package ai

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ExecuteFunc is the function signature for provider-specific command execution.
type ExecuteFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)

// BaseRunner provides common functionality for AI runner implementations.
// Embed this in provider-specific runners to share timeout, retry, and context handling logic.
type BaseRunner struct {
	Config        *config.AIConfig
	Executor      CommandExecutor
	ErrType       error          // Provider-specific error type for wrapping
	Logger        zerolog.Logger // Logger for retry/diagnostic logging (optional, uses nop if not set)
	ProviderName  string         // Display name for retry/outage log messages (e.g., "claude", "gemini")
	StatusPageURL string         // Provider status page surfaced in outage log messages
}

// outage backoff bounds applied when runWithRetry detects a provider outage
// (5xx, overloaded). These widen the wait window vs. regular transient errors,
// since outages typically last seconds-to-minutes.
const (
	outageMinBackoff = 30 * time.Second
	outageMaxBackoff = 60 * time.Second
)

// ValidateWorkingDir checks if the working directory exists.
// Returns nil if the directory exists or is empty (current dir).
// This prevents wasteful retry attempts when the worktree has been deleted.
func (b *BaseRunner) ValidateWorkingDir(workingDir string) error {
	if workingDir == "" {
		return nil
	}
	if _, err := os.Stat(workingDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory missing: %s: %w",
			workingDir, atlaserrors.ErrWorktreeNotFound)
	}
	return nil
}

// ResolveTimeout determines the timeout to use for a request.
// Priority: request timeout > config timeout > default timeout.
func (b *BaseRunner) ResolveTimeout(req *domain.AIRequest) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	if b.Config != nil && b.Config.Timeout > 0 {
		return b.Config.Timeout
	}
	return constants.DefaultAITimeout
}

// RunWithTimeout executes an AI request with proper timeout and retry handling.
// The execute function is provider-specific and handles command building and response parsing.
func (b *BaseRunner) RunWithTimeout(ctx context.Context, req *domain.AIRequest, execute ExecuteFunc) (*domain.AIResult, error) {
	// Check cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Create child context with timeout
	timeout := b.ResolveTimeout(req)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retry logic
	return b.runWithRetry(runCtx, req, execute)
}

// HandleExecutionError processes errors from command execution.
// It checks for context cancellation and attempts to parse error responses.
func (b *BaseRunner) HandleExecutionError(ctx context.Context, err error, tryParse func() (*domain.AIResult, bool), wrapErr func(error) error) (*domain.AIResult, error) {
	// Check if context was canceled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Try to parse response even on error (may have valid JSON with error info)
	if tryParse != nil {
		if result, handled := tryParse(); handled {
			return result, nil
		}
	}

	return nil, wrapErr(err)
}

// HandleProviderExecutionError is a convenience method for provider-specific error handling.
// It simplifies the common pattern of wrapping CLI errors with provider info.
// This reduces boilerplate in individual runner implementations.
func (b *BaseRunner) HandleProviderExecutionError(
	ctx context.Context,
	info CLIInfo,
	err error,
	stderr []byte,
	tryParse func() (*domain.AIResult, bool),
) (*domain.AIResult, error) {
	return b.HandleExecutionError(ctx, err, tryParse, func(e error) error {
		return WrapCLIExecutionError(info, e, stderr)
	})
}

// ProcessTerminator is an interface for executors that support process termination.
type ProcessTerminator interface {
	TerminateProcess() error
}

// TerminateRunningProcess terminates any running subprocess.
// This is used to clean up AI processes during Ctrl+C interruption.
// Returns nil if the executor doesn't support termination or if no process is running.
func (b *BaseRunner) TerminateRunningProcess() error {
	// Check if executor supports termination (StreamingExecutor or DefaultExecutor)
	if terminator, ok := b.Executor.(ProcessTerminator); ok {
		return terminator.TerminateProcess()
	}
	return nil // Executor doesn't support process termination
}

// logOutageWarn emits a user-facing warn log when a provider outage is detected
// during retry. It prefers the provider's display name and status page URL when
// configured, falling back to neutral language otherwise.
func (b *BaseRunner) logOutageWarn(err error, attempt int, backoff time.Duration) {
	provider := b.ProviderName
	if provider == "" {
		provider = "AI provider"
	}
	statusPage := b.StatusPageURL
	if statusPage == "" {
		statusPage = "(provider status page not configured)"
	}
	b.Logger.Warn().
		Err(err).
		Str("provider", provider).
		Str("status_page", statusPage).
		Int("attempt", attempt).
		Int("max_attempts", constants.MaxRetryAttempts).
		Dur("backoff", backoff).
		Msgf("%s API appears degraded (5xx/overloaded) — retrying in %s. Status: %s",
			provider, backoff, statusPage)
}

// computeRetryBackoff returns the sleep duration to use before the next retry
// and the new value of the running backoff counter. Provider outages clamp the
// sleep to [outageMinBackoff, outageMaxBackoff] and pin subsequent backoffs to
// the ceiling; other transient errors use normal exponential growth.
func computeRetryBackoff(err error, current time.Duration) (sleep, next time.Duration, outage bool) {
	if isProviderOutage(err) {
		sleep = current
		if sleep < outageMinBackoff {
			sleep = outageMinBackoff
		}
		if sleep > outageMaxBackoff {
			sleep = outageMaxBackoff
		}
		return sleep, outageMaxBackoff, true
	}
	return current, current * constants.BackoffMultiplier, false
}

// logRetryDecision emits the appropriate log message for a retry attempt. An
// outage gets a user-facing warn pointing at the status page; other transient
// errors get the standard "will retry after backoff" warn.
func (b *BaseRunner) logRetryDecision(err error, attempt int, sleep time.Duration, outage bool) {
	if outage {
		b.logOutageWarn(err, attempt, sleep)
		return
	}
	b.Logger.Warn().
		Err(err).
		Int("attempt", attempt).
		Int("max_attempts", constants.MaxRetryAttempts).
		Dur("backoff", sleep).
		Msg("AI request failed, will retry after backoff")
}

// runWithRetry executes the AI request with exponential backoff retry logic.
// Only transient errors are retried; non-retryable errors return immediately.
func (b *BaseRunner) runWithRetry(ctx context.Context, req *domain.AIRequest, execute ExecuteFunc) (*domain.AIResult, error) {
	var lastErr error
	backoff := constants.InitialBackoff

	for attempt := 1; attempt <= constants.MaxRetryAttempts; attempt++ {
		if attempt > 1 {
			b.Logger.Debug().
				Int("attempt", attempt).
				Int("max_attempts", constants.MaxRetryAttempts).
				Msg("retrying AI request")
		}

		result, err := execute(ctx, req)
		if err == nil {
			if attempt > 1 {
				b.Logger.Info().
					Int("attempt", attempt).
					Msg("AI request succeeded after retry")
			}
			return result, nil
		}

		// Don't retry non-retryable errors
		if !isRetryable(err) {
			b.Logger.Debug().
				Err(err).
				Int("attempt", attempt).
				Msg("AI request failed with non-retryable error")
			return nil, err
		}

		lastErr = err
		if attempt >= constants.MaxRetryAttempts {
			continue
		}

		sleep, next, outage := computeRetryBackoff(err, backoff)
		b.logRetryDecision(err, attempt, sleep, outage)

		select {
		case <-ctx.Done():
			// Terminate any running process from previous attempt before returning
			_ = b.TerminateRunningProcess()
			return nil, ctx.Err()
		case <-timeSleep(sleep):
			backoff = next
		}
	}

	b.Logger.Error().
		Err(lastErr).
		Int("max_attempts", constants.MaxRetryAttempts).
		Msg("AI request failed after max retries")

	return nil, fmt.Errorf("%w: max retries exceeded: %w", b.ErrType, lastErr)
}
