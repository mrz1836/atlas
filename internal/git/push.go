// Package git provides Git operations for ATLAS.
// This file implements the PushService for pushing commits to remote repositories.
package git

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ctxutil"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// PushErrorType classifies push failures for appropriate handling.
type PushErrorType int

const (
	// PushErrorNone indicates no error occurred.
	PushErrorNone PushErrorType = iota
	// PushErrorAuth indicates authentication failed - don't retry.
	PushErrorAuth
	// PushErrorNetwork indicates a network issue - retry with backoff.
	PushErrorNetwork
	// PushErrorTimeout indicates a timeout - retry with backoff.
	PushErrorTimeout
	// PushErrorNonFastForward indicates remote has commits local doesn't - needs rebase.
	PushErrorNonFastForward
	// PushErrorOther indicates an unknown error - don't retry.
	PushErrorOther
)

// String returns a string representation of the error type.
func (t PushErrorType) String() string {
	switch t {
	case PushErrorNone:
		return "none"
	case PushErrorAuth:
		return "auth"
	case PushErrorNetwork:
		return "network"
	case PushErrorTimeout:
		return "timeout"
	case PushErrorNonFastForward:
		return "non_fast_forward"
	case PushErrorOther:
		return "other"
	}
	return "other"
}

// PushOptions configures the push operation.
type PushOptions struct {
	// Remote is the remote to push to (default: "origin").
	Remote string
	// Branch is the branch to push.
	Branch string
	// SetUpstream sets the upstream tracking reference if true.
	SetUpstream bool
	// ConfirmBeforePush requires confirmation before pushing if true.
	ConfirmBeforePush bool
	// ConfirmCallback is called before push if ConfirmBeforePush is true.
	// Returns true to proceed, false to cancel.
	ConfirmCallback func(remote, branch string) (bool, error)
	// ProgressCallback receives progress updates during push.
	ProgressCallback func(progress string)
}

// PushResult contains the outcome of a push operation.
type PushResult struct {
	// Success indicates whether the push succeeded.
	Success bool
	// Upstream is the upstream tracking reference (e.g., "origin/feat/new-feature").
	Upstream string
	// ErrorType classifies the error if push failed.
	ErrorType PushErrorType
	// Attempts is the number of push attempts made.
	Attempts int
	// FinalErr is the final error if push failed.
	FinalErr error
}

// RetryConfig configures retry behavior for operations.
// This type is currently local to the git package. If other packages need
// retry configuration (e.g., AI calls, CI polling), consider moving to internal/domain.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (default: 3).
	MaxAttempts int
	// InitialDelay is the initial delay between retries (default: 2s).
	InitialDelay time.Duration
	// MaxDelay is the maximum delay cap (default: 30s).
	MaxDelay time.Duration
	// Multiplier is the delay multiplier per attempt (default: 2.0).
	Multiplier float64
}

// DefaultRetryConfig returns the default retry configuration for push operations.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// PushService provides high-level push operations with retry.
type PushService interface {
	// Push pushes commits to the remote repository with retry logic.
	Push(ctx context.Context, opts PushOptions) (*PushResult, error)
}

// Compile-time interface check.
var _ PushService = (*PushRunner)(nil)

// PushRunner implements PushService using the git Runner.
type PushRunner struct {
	runner Runner
	logger zerolog.Logger
	config RetryConfig
}

// PushRunnerOption configures a PushRunner.
type PushRunnerOption func(*PushRunner)

// NewPushRunner creates a PushRunner with the given git runner.
func NewPushRunner(runner Runner, opts ...PushRunnerOption) *PushRunner {
	pr := &PushRunner{
		runner: runner,
		logger: zerolog.Nop(),
		config: DefaultRetryConfig(),
	}
	for _, opt := range opts {
		opt(pr)
	}
	return pr
}

// WithPushLogger sets the logger for push operations.
func WithPushLogger(logger zerolog.Logger) PushRunnerOption {
	return func(pr *PushRunner) {
		pr.logger = logger
	}
}

// WithPushRetryConfig sets custom retry configuration.
func WithPushRetryConfig(config RetryConfig) PushRunnerOption {
	return func(pr *PushRunner) {
		pr.config = config
	}
}

// Push pushes commits to the remote repository with retry logic.
func (p *PushRunner) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate and normalize options
	if err := p.validateAndNormalizeOpts(&opts); err != nil {
		return nil, err
	}

	// Handle confirmation callback
	if err := p.handleConfirmation(opts); err != nil {
		return nil, err
	}

	// Execute push with retry
	return p.executePushWithRetry(ctx, opts)
}

// validateAndNormalizeOpts validates options and sets defaults.
func (p *PushRunner) validateAndNormalizeOpts(opts *PushOptions) error {
	if opts.Remote == "" {
		opts.Remote = "origin"
		p.logger.Debug().Msg("using default remote 'origin'")
	}
	if opts.Branch == "" {
		return fmt.Errorf("branch name cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	return nil
}

// handleConfirmation handles the confirmation callback if configured.
func (p *PushRunner) handleConfirmation(opts PushOptions) error {
	if !opts.ConfirmBeforePush || opts.ConfirmCallback == nil {
		return nil
	}

	confirmed, err := opts.ConfirmCallback(opts.Remote, opts.Branch)
	if err != nil {
		return fmt.Errorf("failed to confirm push: %w", err)
	}
	if !confirmed {
		return atlaserrors.ErrOperationCanceled
	}
	return nil
}

// executePushWithRetry executes the push operation with retry logic.
func (p *PushRunner) executePushWithRetry(ctx context.Context, opts PushOptions) (*PushResult, error) {
	op := &SimpleRetryOperation[pushAttemptResult]{
		AttemptFunc: func(ctx context.Context, attempt int) (pushAttemptResult, bool, error) {
			result := p.attemptPush(ctx, opts, attempt)
			return result, result.success, result.err
		},
		ShouldRetryFunc: func(err error) bool {
			errType := classifyPushError(err)
			return errType == PushErrorNetwork || errType == PushErrorTimeout
		},
		OnRetryWaitFunc: func(attempt int, delay time.Duration) {
			p.logger.Info().
				Int("next_attempt", attempt+1).
				Dur("delay", delay).
				Msg("retrying push")
			if opts.ProgressCallback != nil {
				opts.ProgressCallback(fmt.Sprintf("Retrying in %v...", delay))
			}
		},
	}

	attemptResult, attempts, err := ExecuteWithRetry(ctx, p.config, op, p.logger)

	result := &PushResult{Attempts: attempts}
	if err == nil && attemptResult.success {
		return p.buildSuccessResult(result, opts, attempts), nil
	}

	// Handle context cancellation directly without wrapping.
	// Check ctx.Err() to distinguish parent context cancellation from operation timeout:
	// - If ctx.Err() != nil, the parent context was canceled/timed out
	// - If ctx.Err() == nil but err is DeadlineExceeded, the operation itself timed out
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	result.ErrorType = attemptResult.errType
	result.FinalErr = attemptResult.err

	return result, p.buildFinalError(result)
}

// pushAttemptResult holds the result of a single push attempt.
type pushAttemptResult struct {
	success bool
	errType PushErrorType
	err     error
}

// attemptPush performs a single push attempt.
func (p *PushRunner) attemptPush(ctx context.Context, opts PushOptions, attempt int) pushAttemptResult {
	p.logger.Info().
		Int("attempt", attempt).
		Str("remote", opts.Remote).
		Str("branch", opts.Branch).
		Bool("set_upstream", opts.SetUpstream).
		Msg("pushing to remote")

	if opts.ProgressCallback != nil {
		opts.ProgressCallback(fmt.Sprintf("Push attempt %d/%d...", attempt, p.config.MaxAttempts))
	}

	err := p.runner.Push(ctx, opts.Remote, opts.Branch, opts.SetUpstream)
	if err == nil {
		return pushAttemptResult{success: true}
	}

	errType := classifyPushError(err)
	p.logger.Warn().
		Err(err).
		Int("attempt", attempt).
		Str("error_type", errType.String()).
		Msg("push failed")

	return pushAttemptResult{success: false, errType: errType, err: err}
}

// buildSuccessResult builds the success result.
func (p *PushRunner) buildSuccessResult(result *PushResult, opts PushOptions, attempts int) *PushResult {
	result.Success = true
	if opts.SetUpstream {
		result.Upstream = fmt.Sprintf("%s/%s", opts.Remote, opts.Branch)
	}

	p.logger.Info().
		Int("attempts", attempts).
		Str("upstream", result.Upstream).
		Msg("push succeeded")

	if opts.ProgressCallback != nil {
		opts.ProgressCallback("Push completed successfully")
	}

	return result
}

// buildFinalError builds the appropriate error based on the error type.
// This function is only called when retry logic exhausts or a non-retryable error occurs.
func (p *PushRunner) buildFinalError(result *PushResult) error {
	switch result.ErrorType {
	case PushErrorNone:
		// Defensive: should not be called with PushErrorNone, but handle gracefully
		return nil
	case PushErrorAuth:
		return fmt.Errorf("authentication failed: %w", atlaserrors.ErrPushAuthFailed)
	case PushErrorNetwork, PushErrorTimeout:
		return fmt.Errorf("push failed after %d attempts: %w", result.Attempts, atlaserrors.ErrPushNetworkFailed)
	case PushErrorNonFastForward:
		return fmt.Errorf("push rejected (non-fast-forward): %w", result.FinalErr)
	case PushErrorOther:
		return fmt.Errorf("failed to push: %w", result.FinalErr)
	}
	return fmt.Errorf("failed to push: %w", result.FinalErr)
}

// classifyPushError classifies a push error for retry handling.
// Uses shared pattern matchers from error_classifier.go.
func classifyPushError(err error) PushErrorType {
	if err == nil {
		return PushErrorNone
	}

	// Check for timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return PushErrorTimeout
	}

	errStr := err.Error()

	// Use shared pattern matchers directly (they handle lowercasing)
	if MatchesAuthError(errStr) {
		return PushErrorAuth
	}
	if MatchesNetworkError(errStr) {
		return PushErrorNetwork
	}
	if MatchesNonFastForwardError(errStr) {
		return PushErrorNonFastForward
	}

	return PushErrorOther
}
