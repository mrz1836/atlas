// Package git provides Git operations for ATLAS.
// This file implements retry logic for git lock file errors.
package git

import (
	"context"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// LockRetryConfig configures retry behavior for lock file errors.
type LockRetryConfig struct {
	// MaxAttempts is the maximum number of attempts (default: 5).
	MaxAttempts int
	// InitialDelay is the initial delay between retries (default: 100ms).
	InitialDelay time.Duration
	// MaxDelay is the maximum delay cap (default: 2s).
	MaxDelay time.Duration
	// Multiplier is the delay multiplier per attempt (default: 2.0).
	Multiplier float64
	// CleanupOnRetry enables stale lock file cleanup before each retry attempt.
	// When enabled, the retry loop will attempt to remove stale lock files
	// that may have been left behind by crashed/interrupted processes.
	CleanupOnRetry bool
	// CleanupWorkDir is the git working directory for lock cleanup.
	// Required when CleanupOnRetry is true.
	CleanupWorkDir string
}

// DefaultLockRetryConfig returns sensible defaults for lock file retry.
// Uses shorter delays than network retries since lock files are typically
// released quickly.
func DefaultLockRetryConfig() LockRetryConfig {
	return LockRetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
	}
}

// DefaultLockRetryConfigWithCleanup returns a lock retry config with stale lock cleanup enabled.
// The workDir should be the git working directory (the directory containing .git).
// This config will attempt to clean up stale lock files before each retry,
// which helps recover from locks left behind by crashed/interrupted processes.
func DefaultLockRetryConfigWithCleanup(workDir string) LockRetryConfig {
	config := DefaultLockRetryConfig()
	config.CleanupOnRetry = true
	config.CleanupWorkDir = workDir
	return config
}

// RunWithLockRetry executes a git operation with retry logic for lock file errors.
// If the operation fails with a lock file error, it will retry up to MaxAttempts times
// with exponential backoff. Non-lock-file errors are returned immediately.
//
// The generic type R allows returning any result type from the operation.
func RunWithLockRetry[R any](
	ctx context.Context,
	config LockRetryConfig,
	logger zerolog.Logger,
	operation func(ctx context.Context) (R, error),
) (R, error) {
	var zero R
	var lastErr error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check for context cancellation before attempting
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := operation(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if this is a lock file error
		if !MatchesLockFileError(err.Error()) {
			// Not a lock file error, don't retry
			return zero, err
		}

		// Log the retry attempt
		logger.Debug().
			Int("attempt", attempt).
			Int("max_attempts", config.MaxAttempts).
			Dur("delay", delay).
			Err(err).
			Msg("git lock file error, retrying")

		// Don't wait after the last attempt
		if attempt >= config.MaxAttempts {
			break
		}

		// Attempt to clean up stale lock files before retrying
		// This helps recover from locks left behind by crashed/interrupted processes
		if config.CleanupOnRetry && config.CleanupWorkDir != "" {
			gitDir := filepath.Join(config.CleanupWorkDir, ".git")
			if cleanupErr := CleanupStaleLockFiles(ctx, gitDir, DefaultLockStalenessThreshold, logger); cleanupErr != nil {
				logger.Debug().Err(cleanupErr).Msg("lock cleanup during retry failed")
			}
		}

		// Wait before retrying
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}

		// Increase delay for next attempt with exponential backoff
		delay = time.Duration(float64(delay) * config.Multiplier)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	logger.Warn().
		Int("attempts", config.MaxAttempts).
		Err(lastErr).
		Msg("git lock file retry exhausted")

	return zero, lastErr
}

// RunWithLockRetryVoid is a convenience wrapper for operations that don't return a value.
// It wraps the operation to return struct{}{} and discards the result.
func RunWithLockRetryVoid(
	ctx context.Context,
	config LockRetryConfig,
	logger zerolog.Logger,
	operation func(ctx context.Context) error,
) error {
	_, err := RunWithLockRetry(ctx, config, logger, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, operation(ctx)
	})
	return err
}
