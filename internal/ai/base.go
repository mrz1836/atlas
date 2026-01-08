package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
)

// ExecuteFunc is the function signature for provider-specific command execution.
type ExecuteFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)

// BaseRunner provides common functionality for AI runner implementations.
// Embed this in provider-specific runners to share timeout, retry, and context handling logic.
type BaseRunner struct {
	Config   *config.AIConfig
	Executor CommandExecutor
	ErrType  error // Provider-specific error type for wrapping
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

// runWithRetry executes the AI request with exponential backoff retry logic.
// Only transient errors are retried; non-retryable errors return immediately.
func (b *BaseRunner) runWithRetry(ctx context.Context, req *domain.AIRequest, execute ExecuteFunc) (*domain.AIResult, error) {
	var lastErr error
	backoff := constants.InitialBackoff

	for attempt := 1; attempt <= constants.MaxRetryAttempts; attempt++ {
		result, err := execute(ctx, req)
		if err == nil {
			return result, nil
		}

		// Don't retry non-retryable errors
		if !isRetryable(err) {
			return nil, err
		}

		lastErr = err
		if attempt < constants.MaxRetryAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeSleep(backoff):
				backoff *= constants.BackoffMultiplier
			}
		}
	}

	return nil, fmt.Errorf("%w: max retries exceeded: %w", b.ErrType, lastErr)
}
