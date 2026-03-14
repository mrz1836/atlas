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
//nolint:gochecknoglobals // Required for test mocking
var timeSleep = func(d time.Duration) <-chan time.Time {
	return time.After(d)
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

// isFallbackTrigger determines whether an error should trigger a model fallback.
// Returns true for format and content errors that might succeed with a different model.
// Returns false for transient errors (network, rate limits) which should be retried on same model.
func isFallbackTrigger(err error) bool {
	if err == nil {
		return false
	}

	// Format/content errors - a different model might produce valid output
	if errors.Is(err, atlaserrors.ErrAIInvalidFormat) ||
		errors.Is(err, atlaserrors.ErrAIEmptyResponse) {
		return true
	}

	// Check error message for format-related patterns
	errStr := strings.ToLower(err.Error())
	formatPatterns := []string{
		"invalid format",
		"unexpected format",
		"parse error",
		"malformed response",
		"not in expected format",
	}
	for _, pattern := range formatPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isNonRecoverableError determines whether an error should stop all retry/fallback attempts.
// Returns true for errors that cannot be recovered by retrying or switching models.
func isNonRecoverableError(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation - user requested stop
	if errors.Is(err, context.Canceled) {
		return true
	}

	// Worktree deleted/missing - nothing can be done
	if errors.Is(err, atlaserrors.ErrWorktreeNotFound) {
		return true
	}

	// Check for authentication errors in message
	errStr := strings.ToLower(err.Error())
	authPatterns := []string{
		"authentication",
		"api key",
		"unauthorized",
		"forbidden",
		"invalid credentials",
	}
	for _, pattern := range authPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// IsFallbackTrigger is the exported version of isFallbackTrigger.
// It determines whether an error should trigger a model fallback.
func IsFallbackTrigger(err error) bool {
	return isFallbackTrigger(err)
}

// IsNonRecoverable is the exported version of isNonRecoverableError.
// It determines whether an error should stop all retry/fallback attempts.
func IsNonRecoverable(err error) bool {
	return isNonRecoverableError(err)
}
