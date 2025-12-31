// Package git provides Git operations for ATLAS.
// This file implements the PushService for pushing commits to remote repositories.
package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
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
	result := &PushResult{}
	delay := p.config.InitialDelay

	for attempt := 1; attempt <= p.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		pushResult := p.attemptPush(ctx, opts, attempt)
		if pushResult.success {
			return p.buildSuccessResult(result, opts, attempt), nil
		}

		result.ErrorType = pushResult.errType
		result.FinalErr = pushResult.err

		// Check if we should stop retrying
		if !p.shouldRetry(pushResult.errType) {
			break
		}

		// Wait before retrying (unless this is the last attempt)
		if attempt < p.config.MaxAttempts {
			if err := p.waitForRetry(ctx, opts, &delay, attempt); err != nil {
				return nil, err
			}
		}
	}

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

// shouldRetry determines if the error type is retryable.
func (p *PushRunner) shouldRetry(errType PushErrorType) bool {
	return errType == PushErrorNetwork || errType == PushErrorTimeout
}

// waitForRetry waits before the next retry attempt.
func (p *PushRunner) waitForRetry(ctx context.Context, opts PushOptions, delay *time.Duration, attempt int) error {
	p.logger.Info().
		Int("next_attempt", attempt+1).
		Dur("delay", *delay).
		Msg("retrying push")

	if opts.ProgressCallback != nil {
		opts.ProgressCallback(fmt.Sprintf("Retrying in %v...", *delay))
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(*delay):
	}

	// Increase delay for next attempt
	*delay = time.Duration(float64(*delay) * p.config.Multiplier)
	if *delay > p.config.MaxDelay {
		*delay = p.config.MaxDelay
	}

	return nil
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
	case PushErrorOther:
		return fmt.Errorf("failed to push: %w", result.FinalErr)
	}
	return fmt.Errorf("failed to push: %w", result.FinalErr)
}

// classifyPushError classifies a push error for retry handling.
func classifyPushError(err error) PushErrorType {
	if err == nil {
		return PushErrorNone
	}

	// Check for timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return PushErrorTimeout
	}

	errStr := strings.ToLower(err.Error())

	if isAuthError(errStr) {
		return PushErrorAuth
	}

	if isNetworkError(errStr) {
		return PushErrorNetwork
	}

	return PushErrorOther
}

// isAuthError checks if the error string indicates an authentication error.
func isAuthError(errStr string) bool {
	authPatterns := []string{
		"authentication failed",
		"could not read username",
		"permission denied",
		"invalid username or password",
		"access denied",
		"fatal: authentication failed",
	}
	for _, pattern := range authPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// isNetworkError checks if the error string indicates a network error.
func isNetworkError(errStr string) bool {
	networkPatterns := []string{
		"could not resolve host",
		"connection refused",
		"network is unreachable",
		"connection timed out",
		"operation timed out",
		"unable to access",
		"no route to host",
		"failed to connect",
	}
	for _, pattern := range networkPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}
