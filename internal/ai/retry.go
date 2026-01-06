package ai

import (
	"context"
	"errors"
	"strings"
	"time"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// timeSleep is a wrapper for time.After that can be overridden in tests.
// It returns a channel that receives after the given duration.
//
// The interface type `interface{ Nanoseconds() int64 }` is used instead of
// time.Duration to accept any duration-like type, providing flexibility for
// test mocking while maintaining type safety.
//
//nolint:gochecknoglobals // Required for test mocking
var timeSleep = func(d interface{ Nanoseconds() int64 }) <-chan time.Time {
	return time.After(time.Duration(d.Nanoseconds()))
}

// isRetryable determines whether an error should be retried.
// Returns false for non-retryable errors (context errors, auth errors, parse errors).
// Returns true for transient errors (network, rate limits).
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for specific non-retryable error messages
	errStr := strings.ToLower(err.Error())

	// Authentication errors are not retryable
	if strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "api key") ||
		strings.Contains(errStr, "anthropic_api_key") ||
		strings.Contains(errStr, "gemini_api_key") ||
		strings.Contains(errStr, "openai_api_key") {
		return false
	}

	// JSON parse errors are not retryable
	if strings.Contains(errStr, "invalid json") ||
		strings.Contains(errStr, "failed to parse json") {
		return false
	}

	// CLI not found is not retryable
	if strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "executable file not found") {
		return false
	}

	// Directory/file system errors are not retryable (worktree deleted/missing)
	// These occur when cmd.Dir points to a non-existent directory
	if strings.Contains(errStr, "no such file or directory") ||
		strings.Contains(errStr, "chdir") {
		return false
	}

	// Worktree-specific errors are not retryable
	if errors.Is(err, atlaserrors.ErrWorktreeNotFound) {
		return false
	}

	// All other errors are considered transient and retryable
	// (network errors, rate limits, etc.)
	return true
}
