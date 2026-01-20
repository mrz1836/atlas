package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test error types for isRetryable testing.
var (
	errAuthFailed        = errors.New("authentication failed")
	errInvalidAPIKey     = errors.New("invalid api key")
	errAPIKeyNotSet      = errors.New("ANTHROPIC_API_KEY not set")
	errInvalidJSON       = errors.New("invalid json response")
	errParseJSON         = errors.New("failed to parse json")
	errCommandNotFound   = errors.New("claude: command not found")
	errExecNotFound      = errors.New("executable file not found")
	errNetworkReset      = errors.New("network connection reset")
	errRateLimit         = errors.New("rate limit exceeded")
	errGeneric           = errors.New("something went wrong")
	errConnectionTimeout = errors.New("connection timeout")
	errNoSuchFile        = errors.New("chdir /path/to/worktree: no such file or directory")
	errChdirFailed       = errors.New("chdir to working directory failed")

	// Test error types for isFallbackTrigger and isNonRecoverableError testing.
	errInvalidFormatMissingPrefix = errors.New("invalid format: missing type prefix")
	errUnexpectedFormat           = errors.New("unexpected format from AI")
	errParseExpectedConventional  = errors.New("parse error: expected conventional commit")
	errMalformedResponse          = errors.New("malformed response from model")
	errNotInExpectedFormat        = errors.New("AI response not in expected format")
	errAuthFailedForAPI           = errors.New("authentication failed for API")
	errInvalidAPIKeyProvided      = errors.New("invalid api key provided")
	errUnauthorized               = errors.New("request unauthorized")
	errForbidden                  = errors.New("access forbidden")
	errInvalidCreds               = errors.New("invalid credentials")
)

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		substrs []string
		want    bool
	}{
		{
			name:    "empty string and empty substrs",
			s:       "",
			substrs: []string{},
			want:    false,
		},
		{
			name:    "empty substrs",
			s:       "some error",
			substrs: []string{},
			want:    false,
		},
		{
			name:    "contains first substr",
			s:       "authentication failed",
			substrs: []string{"authentication", "api key"},
			want:    true,
		},
		{
			name:    "contains second substr",
			s:       "invalid api key",
			substrs: []string{"authentication", "api key"},
			want:    true,
		},
		{
			name:    "contains none",
			s:       "network timeout",
			substrs: []string{"authentication", "api key"},
			want:    false,
		},
		{
			name:    "case sensitive - no match",
			s:       "AUTHENTICATION",
			substrs: []string{"authentication"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsAny(tt.s, tt.substrs...); got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not retryable",
			err:      nil,
			expected: false,
		},
		{
			name:     "context canceled is not retryable",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "context deadline exceeded is not retryable",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "authentication error is not retryable",
			err:      errAuthFailed,
			expected: false,
		},
		{
			name:     "api key error is not retryable",
			err:      errInvalidAPIKey,
			expected: false,
		},
		{
			name:     "ANTHROPIC_API_KEY error is not retryable",
			err:      errAPIKeyNotSet,
			expected: false,
		},
		{
			name:     "invalid json error is not retryable",
			err:      errInvalidJSON,
			expected: false,
		},
		{
			name:     "failed to parse json is not retryable",
			err:      errParseJSON,
			expected: false,
		},
		{
			name:     "command not found is not retryable",
			err:      errCommandNotFound,
			expected: false,
		},
		{
			name:     "executable file not found is not retryable",
			err:      errExecNotFound,
			expected: false,
		},
		{
			name:     "network error is retryable",
			err:      errNetworkReset,
			expected: true,
		},
		{
			name:     "rate limit error is retryable",
			err:      errRateLimit,
			expected: true,
		},
		{
			name:     "generic error is retryable",
			err:      errGeneric,
			expected: true,
		},
		{
			name:     "timeout error is retryable",
			err:      errConnectionTimeout,
			expected: true,
		},
		// Directory/filesystem errors should NOT be retryable
		{
			name:     "no such file or directory is not retryable",
			err:      errNoSuchFile,
			expected: false,
		},
		{
			name:     "chdir error is not retryable",
			err:      errChdirFailed,
			expected: false,
		},
		{
			name:     "ErrWorktreeNotFound is not retryable",
			err:      atlaserrors.ErrWorktreeNotFound,
			expected: false,
		},
		{
			name:     "wrapped ErrWorktreeNotFound is not retryable",
			err:      fmt.Errorf("working directory missing: /path: %w", atlaserrors.ErrWorktreeNotFound),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsFallbackTrigger(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not fallback trigger",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrAIInvalidFormat is fallback trigger",
			err:      atlaserrors.ErrAIInvalidFormat,
			expected: true,
		},
		{
			name:     "ErrAIEmptyResponse is fallback trigger",
			err:      atlaserrors.ErrAIEmptyResponse,
			expected: true,
		},
		{
			name:     "wrapped ErrAIInvalidFormat is fallback trigger",
			err:      fmt.Errorf("generation failed: %w", atlaserrors.ErrAIInvalidFormat),
			expected: true,
		},
		{
			name:     "wrapped ErrAIEmptyResponse is fallback trigger",
			err:      fmt.Errorf("no response: %w", atlaserrors.ErrAIEmptyResponse),
			expected: true,
		},
		{
			name:     "invalid format in message is fallback trigger",
			err:      errInvalidFormatMissingPrefix,
			expected: true,
		},
		{
			name:     "unexpected format in message is fallback trigger",
			err:      errUnexpectedFormat,
			expected: true,
		},
		{
			name:     "parse error in message is fallback trigger",
			err:      errParseExpectedConventional,
			expected: true,
		},
		{
			name:     "malformed response in message is fallback trigger",
			err:      errMalformedResponse,
			expected: true,
		},
		{
			name:     "not in expected format in message is fallback trigger",
			err:      errNotInExpectedFormat,
			expected: true,
		},
		{
			name:     "network error is not fallback trigger",
			err:      errNetworkReset,
			expected: false,
		},
		{
			name:     "rate limit error is not fallback trigger",
			err:      errRateLimit,
			expected: false,
		},
		{
			name:     "context canceled is not fallback trigger",
			err:      context.Canceled,
			expected: false,
		},
		{
			name:     "context deadline exceeded is not fallback trigger",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "generic error is not fallback trigger",
			err:      errGeneric,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFallbackTrigger(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNonRecoverableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not non-recoverable",
			err:      nil,
			expected: false,
		},
		{
			name:     "context canceled is non-recoverable",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "wrapped context canceled is non-recoverable",
			err:      fmt.Errorf("operation stopped: %w", context.Canceled),
			expected: true,
		},
		{
			name:     "ErrWorktreeNotFound is non-recoverable",
			err:      atlaserrors.ErrWorktreeNotFound,
			expected: true,
		},
		{
			name:     "wrapped ErrWorktreeNotFound is non-recoverable",
			err:      fmt.Errorf("worktree missing: %w", atlaserrors.ErrWorktreeNotFound),
			expected: true,
		},
		{
			name:     "authentication in message is non-recoverable",
			err:      errAuthFailedForAPI,
			expected: true,
		},
		{
			name:     "api key in message is non-recoverable",
			err:      errInvalidAPIKeyProvided,
			expected: true,
		},
		{
			name:     "unauthorized in message is non-recoverable",
			err:      errUnauthorized,
			expected: true,
		},
		{
			name:     "forbidden in message is non-recoverable",
			err:      errForbidden,
			expected: true,
		},
		{
			name:     "invalid credentials in message is non-recoverable",
			err:      errInvalidCreds,
			expected: true,
		},
		{
			name:     "context deadline exceeded is not non-recoverable",
			err:      context.DeadlineExceeded,
			expected: false,
		},
		{
			name:     "network error is not non-recoverable",
			err:      errNetworkReset,
			expected: false,
		},
		{
			name:     "rate limit error is not non-recoverable",
			err:      errRateLimit,
			expected: false,
		},
		{
			name:     "ErrAIInvalidFormat is not non-recoverable",
			err:      atlaserrors.ErrAIInvalidFormat,
			expected: false,
		},
		{
			name:     "generic error is not non-recoverable",
			err:      errGeneric,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNonRecoverableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
