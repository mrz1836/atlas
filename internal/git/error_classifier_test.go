package git

import "testing"

func TestErrorType_String(t *testing.T) {
	tests := []struct {
		errType  ErrorType
		expected string
	}{
		{ErrorTypeUnknown, "unknown"},
		{ErrorTypeAuth, "authentication"},
		{ErrorTypeNetwork, "network"},
		{ErrorTypeRateLimit, "rate_limit"},
		{ErrorTypeNotFound, "not_found"},
		{ErrorTypeNonFastForward, "non_fast_forward"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.errType.String(); got != tt.expected {
				t.Errorf("ErrorType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected ErrorType
	}{
		// Authentication errors
		{"auth - authentication failed", "authentication failed for user", ErrorTypeAuth},
		{"auth - permission denied", "permission denied (publickey)", ErrorTypeAuth},
		{"auth - bad credentials", "Bad credentials", ErrorTypeAuth},
		{"auth - invalid token", "invalid token provided", ErrorTypeAuth},
		{"auth - gh auth login", "To get started with GitHub CLI, please run: gh auth login", ErrorTypeAuth},
		{"auth - token expired", "Token expired, please re-authenticate", ErrorTypeAuth},
		{"auth - case insensitive", "AUTHENTICATION FAILED", ErrorTypeAuth},

		// Network errors
		{"network - could not resolve host", "could not resolve host: github.com", ErrorTypeNetwork},
		{"network - connection refused", "connection refused: 443", ErrorTypeNetwork},
		{"network - connection timed out", "connection timed out after 30s", ErrorTypeNetwork},
		{"network - unable to access", "fatal: unable to access 'https://github.com/...'", ErrorTypeNetwork},
		{"network - timeout", "request timeout", ErrorTypeNetwork},

		// Rate limit errors
		{"rate limit - exceeded", "rate limit exceeded", ErrorTypeRateLimit},
		{"rate limit - api rate limit", "API rate limit exceeded for user", ErrorTypeRateLimit},
		{"rate limit - secondary", "secondary rate limit hit", ErrorTypeRateLimit},
		{"rate limit - too many requests", "too many requests", ErrorTypeRateLimit},
		{"rate limit - abuse detection", "abuse detection mechanism triggered", ErrorTypeRateLimit},

		// Not found errors
		{"not found - basic", "repository not found", ErrorTypeNotFound},
		{"not found - no such", "no such file or directory", ErrorTypeNotFound},
		{"not found - does not exist", "branch 'main' does not exist", ErrorTypeNotFound},

		// Non-fast-forward errors
		{"non-ff - basic", "non-fast-forward update rejected", ErrorTypeNonFastForward},
		{"non-ff - failed to push", "error: failed to push some refs to 'origin'", ErrorTypeNonFastForward},
		{"non-ff - updates rejected", "Updates were rejected because the tip of your current branch is behind", ErrorTypeNonFastForward},
		{"non-ff - fetch first", "hint: Please fetch first", ErrorTypeNonFastForward},

		// Unknown errors
		{"unknown - empty string", "", ErrorTypeUnknown},
		{"unknown - random error", "something went wrong", ErrorTypeUnknown},
		{"unknown - syntax error", "syntax error near unexpected token", ErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.errStr); got != tt.expected {
				t.Errorf("ClassifyError(%q) = %v, want %v", tt.errStr, got, tt.expected)
			}
		})
	}
}

func TestClassifyError_Priority(t *testing.T) {
	// Test that more specific patterns take priority
	tests := []struct {
		name     string
		errStr   string
		expected ErrorType
	}{
		// Rate limit should take priority over auth (both could match "limit exceeded" with "access denied")
		{
			name:     "rate limit over auth",
			errStr:   "rate limit exceeded, access denied",
			expected: ErrorTypeRateLimit,
		},
		// Auth should take priority over network when both could match
		{
			name:     "auth over network",
			errStr:   "authentication failed: connection refused",
			expected: ErrorTypeAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.errStr); got != tt.expected {
				t.Errorf("ClassifyError(%q) = %v, want %v (priority check)", tt.errStr, got, tt.expected)
			}
		})
	}
}

func TestErrorClassifier_Classify(t *testing.T) {
	// Test using a custom classifier
	customClassifier := &ErrorClassifier{
		auth:           NewPatternMatcher("custom-auth-error"),
		network:        NewPatternMatcher("custom-network-error"),
		rateLimit:      NewPatternMatcher("custom-rate-limit"),
		notFound:       NewPatternMatcher("custom-not-found"),
		nonFastForward: NewPatternMatcher("custom-non-ff"),
	}

	tests := []struct {
		errStr   string
		expected ErrorType
	}{
		{"custom-auth-error occurred", ErrorTypeAuth},
		{"custom-network-error occurred", ErrorTypeNetwork},
		{"custom-rate-limit hit", ErrorTypeRateLimit},
		{"custom-not-found returned", ErrorTypeNotFound},
		{"custom-non-ff rejection", ErrorTypeNonFastForward},
		{"standard auth error", ErrorTypeUnknown}, // doesn't match custom patterns
	}

	for _, tt := range tests {
		t.Run(tt.errStr, func(t *testing.T) {
			if got := customClassifier.Classify(tt.errStr); got != tt.expected {
				t.Errorf("customClassifier.Classify(%q) = %v, want %v", tt.errStr, got, tt.expected)
			}
		})
	}
}

func TestPatternMatcher_Matches(t *testing.T) {
	matcher := NewPatternMatcher("error", "fail", "timeout")

	tests := []struct {
		input    string
		expected bool
	}{
		{"an error occurred", true},
		{"operation failed", true},
		{"request timeout", true},
		{"success", false},
		{"", false},
		{"ERROR in uppercase", true}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := matcher.Matches(tt.input); got != tt.expected {
				t.Errorf("Matches(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPatternMatcher_MatchesLower(t *testing.T) {
	matcher := NewPatternMatcher("error", "fail")

	tests := []struct {
		input    string
		expected bool
	}{
		{"an error occurred", true},
		{"operation failed", true},
		{"success", false},
		// MatchesLower expects pre-lowercased input
		{"ERROR", false}, // uppercase won't match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := matcher.MatchesLower(tt.input); got != tt.expected {
				t.Errorf("MatchesLower(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// Test backward compatibility with existing Matches* functions
func TestMatchesFunctions_BackwardCompatibility(t *testing.T) {
	t.Run("MatchesAuthError", func(t *testing.T) {
		if !MatchesAuthError("authentication failed") {
			t.Error("expected MatchesAuthError to match 'authentication failed'")
		}
		if MatchesAuthError("success") {
			t.Error("expected MatchesAuthError not to match 'success'")
		}
	})

	t.Run("MatchesNetworkError", func(t *testing.T) {
		if !MatchesNetworkError("connection refused") {
			t.Error("expected MatchesNetworkError to match 'connection refused'")
		}
		if MatchesNetworkError("success") {
			t.Error("expected MatchesNetworkError not to match 'success'")
		}
	})

	t.Run("MatchesRateLimitError", func(t *testing.T) {
		if !MatchesRateLimitError("rate limit exceeded") {
			t.Error("expected MatchesRateLimitError to match 'rate limit exceeded'")
		}
		if MatchesRateLimitError("success") {
			t.Error("expected MatchesRateLimitError not to match 'success'")
		}
	})

	t.Run("MatchesNotFoundError", func(t *testing.T) {
		if !MatchesNotFoundError("not found") {
			t.Error("expected MatchesNotFoundError to match 'not found'")
		}
		if MatchesNotFoundError("success") {
			t.Error("expected MatchesNotFoundError not to match 'success'")
		}
	})

	t.Run("MatchesNonFastForwardError", func(t *testing.T) {
		if !MatchesNonFastForwardError("non-fast-forward") {
			t.Error("expected MatchesNonFastForwardError to match 'non-fast-forward'")
		}
		if MatchesNonFastForwardError("success") {
			t.Error("expected MatchesNonFastForwardError not to match 'success'")
		}
	})
}

// TestMatchesFunctions_CaseInsensitive verifies that all Matches* functions
// perform case-insensitive matching. This tests the fix for the bug where
// MatchesLower was called without lowercasing the input first.
func TestMatchesFunctions_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(string) bool
		inputs   []string
		expected bool
	}{
		{
			name:     "MatchesAuthError uppercase",
			fn:       MatchesAuthError,
			inputs:   []string{"AUTHENTICATION FAILED", "Authentication Failed", "PERMISSION DENIED"},
			expected: true,
		},
		{
			name:     "MatchesNetworkError uppercase",
			fn:       MatchesNetworkError,
			inputs:   []string{"CONNECTION REFUSED", "Connection Timed Out", "COULD NOT RESOLVE HOST"},
			expected: true,
		},
		{
			name:     "MatchesRateLimitError uppercase",
			fn:       MatchesRateLimitError,
			inputs:   []string{"RATE LIMIT EXCEEDED", "Rate Limit Exceeded", "TOO MANY REQUESTS"},
			expected: true,
		},
		{
			name:     "MatchesNotFoundError uppercase",
			fn:       MatchesNotFoundError,
			inputs:   []string{"NOT FOUND", "Not Found", "REPOSITORY NOT FOUND"},
			expected: true,
		},
		{
			name:     "MatchesNonFastForwardError uppercase",
			fn:       MatchesNonFastForwardError,
			inputs:   []string{"NON-FAST-FORWARD", "Non-Fast-Forward", "UPDATES WERE REJECTED"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, input := range tt.inputs {
				if got := tt.fn(input); got != tt.expected {
					t.Errorf("%s(%q) = %v, want %v (case-insensitive matching)", tt.name, input, got, tt.expected)
				}
			}
		})
	}
}

// TestMatchesFunctions_MixedCase tests edge cases with mixed case patterns.
func TestMatchesFunctions_MixedCase(t *testing.T) {
	// Real-world error messages often have mixed casing
	realWorldErrors := []struct {
		errStr   string
		matchFn  func(string) bool
		fnName   string
		expected bool
	}{
		{"Error: Authentication Failed for user@example.com", MatchesAuthError, "MatchesAuthError", true},
		{"FATAL: Could not resolve host: github.com", MatchesNetworkError, "MatchesNetworkError", true},
		{"GitHub API Rate Limit Exceeded - try again later", MatchesRateLimitError, "MatchesRateLimitError", true},
		{"Error 404: Repository Not Found", MatchesNotFoundError, "MatchesNotFoundError", true},
		{"ERROR: Updates Were Rejected because the tip is behind", MatchesNonFastForwardError, "MatchesNonFastForwardError", true},
	}

	for _, tt := range realWorldErrors {
		t.Run(tt.fnName+"_realworld", func(t *testing.T) {
			if got := tt.matchFn(tt.errStr); got != tt.expected {
				t.Errorf("%s(%q) = %v, want %v", tt.fnName, tt.errStr, got, tt.expected)
			}
		})
	}
}
