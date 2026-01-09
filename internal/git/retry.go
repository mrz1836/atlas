// Package git provides Git operations for ATLAS.
// This file implements shared retry logic for git operations.
package git

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// RetryableOperation defines the interface for operations that can be retried.
// Implementations provide the attempt logic and retry decision making.
type RetryableOperation[R any] interface {
	// Attempt performs a single attempt and returns the result.
	// success indicates if the attempt succeeded.
	// err is any error that occurred (may be non-nil even on success for logging).
	Attempt(ctx context.Context, attempt int) (result R, success bool, err error)

	// ShouldRetry returns true if the operation should be retried given the error.
	ShouldRetry(err error) bool

	// OnRetryWait is called before waiting for the next retry (optional logging/progress).
	OnRetryWait(attempt int, delay time.Duration)
}

// ExecuteWithRetry executes an operation with retry logic based on the provided config.
// Returns the result, total attempts made, and any final error.
func ExecuteWithRetry[R any](
	ctx context.Context,
	config RetryConfig,
	op RetryableOperation[R],
	_ zerolog.Logger,
) (result R, attempts int, finalErr error) {
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		attempts = attempt

		res, success, err := op.Attempt(ctx, attempt)
		if success {
			return res, attempts, nil
		}

		// Store both the result and error from the failed attempt
		result = res
		finalErr = err

		// Check if we should stop retrying
		if !op.ShouldRetry(err) {
			break
		}

		// Wait before retrying (unless this is the last attempt)
		if attempt < config.MaxAttempts {
			op.OnRetryWait(attempt, delay)

			select {
			case <-ctx.Done():
				return result, attempts, ctx.Err()
			case <-time.After(delay):
			}

			// Increase delay for next attempt with exponential backoff
			delay = time.Duration(float64(delay) * config.Multiplier)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
	}

	return result, attempts, finalErr
}

// SimpleRetryOperation provides a simplified implementation for common cases.
// Use this when you have straightforward attempt and retry logic.
type SimpleRetryOperation[R any] struct {
	AttemptFunc     func(ctx context.Context, attempt int) (R, bool, error)
	ShouldRetryFunc func(err error) bool
	OnRetryWaitFunc func(attempt int, delay time.Duration)
}

// Attempt implements RetryableOperation.
func (s *SimpleRetryOperation[R]) Attempt(ctx context.Context, attempt int) (R, bool, error) {
	return s.AttemptFunc(ctx, attempt)
}

// ShouldRetry implements RetryableOperation.
func (s *SimpleRetryOperation[R]) ShouldRetry(err error) bool {
	if s.ShouldRetryFunc == nil {
		return false
	}
	return s.ShouldRetryFunc(err)
}

// OnRetryWait implements RetryableOperation.
func (s *SimpleRetryOperation[R]) OnRetryWait(attempt int, delay time.Duration) {
	if s.OnRetryWaitFunc != nil {
		s.OnRetryWaitFunc(attempt, delay)
	}
}

// Compile-time interface check.
var _ RetryableOperation[any] = (*SimpleRetryOperation[any])(nil)
