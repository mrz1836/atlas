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

// nonRetryablePatterns defines groups of error message patterns that indicate non-retryable errors.
// Each inner slice contains patterns that belong to the same category.
//
//nolint:gochecknoglobals // Read-only pattern configuration
var nonRetryablePatterns = [][]string{
	// Authentication errors - cannot retry without fixing credentials
	{"authentication", "api key", "anthropic_api_key", "gemini_api_key", "openai_api_key"},
	// JSON parse errors - same input will produce same error
	{"invalid json", "failed to parse json"},
	// CLI not found - requires installation, not transient
	{"not found", "executable file not found"},
	// Directory/file system errors - worktree deleted or missing
	{"no such file or directory", "chdir"},
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
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

	// Worktree-specific errors are not retryable
	if errors.Is(err, atlaserrors.ErrWorktreeNotFound) {
		return false
	}

	// Check error message against non-retryable patterns
	errStr := strings.ToLower(err.Error())
	for _, patterns := range nonRetryablePatterns {
		if containsAny(errStr, patterns...) {
			return false
		}
	}

	// All other errors are considered transient and retryable
	// (network errors, rate limits, etc.)
	return true
}
