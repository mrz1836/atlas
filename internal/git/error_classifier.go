// Package git provides Git operations for ATLAS.
// This file contains generic error classification utilities.
package git

import "strings"

// ErrorType represents the classification of a git/github error.
type ErrorType int

const (
	// ErrorTypeUnknown indicates the error could not be classified.
	ErrorTypeUnknown ErrorType = iota
	// ErrorTypeAuth indicates an authentication error.
	ErrorTypeAuth
	// ErrorTypeNetwork indicates a network connectivity error.
	ErrorTypeNetwork
	// ErrorTypeRateLimit indicates an API rate limit error.
	ErrorTypeRateLimit
	// ErrorTypeNotFound indicates a resource not found error.
	ErrorTypeNotFound
	// ErrorTypeNonFastForward indicates a non-fast-forward push rejection.
	ErrorTypeNonFastForward
)

// String returns a human-readable name for the error type.
func (e ErrorType) String() string {
	switch e {
	case ErrorTypeUnknown:
		return "unknown"
	case ErrorTypeAuth:
		return "authentication"
	case ErrorTypeNetwork:
		return "network"
	case ErrorTypeRateLimit:
		return "rate_limit"
	case ErrorTypeNotFound:
		return "not_found"
	case ErrorTypeNonFastForward:
		return "non_fast_forward"
	default:
		return "unknown"
	}
}

// PatternMatcher checks if a string contains any of a list of patterns.
// It performs case-insensitive matching on the lowercased input.
type PatternMatcher struct {
	patterns []string
}

// NewPatternMatcher creates a new PatternMatcher with the given patterns.
// All patterns should be lowercase for consistent matching.
func NewPatternMatcher(patterns ...string) *PatternMatcher {
	return &PatternMatcher{patterns: patterns}
}

// Matches returns true if the input string contains any of the patterns.
// The input is lowercased before matching.
func (m *PatternMatcher) Matches(s string) bool {
	lower := strings.ToLower(s)
	for _, pattern := range m.patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// MatchesLower checks if an already-lowercased string matches any pattern.
// Use this when you've already lowercased the input for better performance.
func (m *PatternMatcher) MatchesLower(lower string) bool {
	for _, pattern := range m.patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// Common error pattern matchers for reuse across the package.
//
//nolint:gochecknoglobals // Package-level immutable pattern matchers for performance
var (
	// authPatterns matches authentication-related errors.
	authPatterns = NewPatternMatcher(
		"authentication failed",
		"could not read username",
		"permission denied",
		"invalid username or password",
		"access denied",
		"fatal: authentication failed",
		"authentication required",
		"bad credentials",
		"not logged into",
		"must be authenticated",
		"gh auth login",
		"invalid token",
		"token expired",
	)

	// networkPatterns matches network-related errors.
	networkPatterns = NewPatternMatcher(
		"could not resolve host",
		"connection refused",
		"network is unreachable",
		"connection timed out",
		"operation timed out",
		"unable to access",
		"no route to host",
		"failed to connect",
		"timeout",
	)

	// rateLimitPatterns matches rate limiting errors.
	rateLimitPatterns = NewPatternMatcher(
		"rate limit exceeded",
		"api rate limit",
		"secondary rate limit",
		"abuse detection",
		"too many requests",
	)

	// notFoundPatterns matches not-found errors.
	notFoundPatterns = NewPatternMatcher(
		"not found",
		"no such",
		"repository not found",
		"does not exist",
	)

	// nonFastForwardPatterns matches non-fast-forward push rejections.
	nonFastForwardPatterns = NewPatternMatcher(
		"non-fast-forward",
		"rejected",
		"failed to push some refs",
		"updates were rejected",
		"fetch first",
		"tip of your current branch is behind",
		"rejected because the remote contains work",
	)
)

// ErrorClassifier provides a unified interface for classifying git errors.
// It consolidates all pattern matchers into a single struct for easier testing
// and extension.
type ErrorClassifier struct {
	auth           *PatternMatcher
	network        *PatternMatcher
	rateLimit      *PatternMatcher
	notFound       *PatternMatcher
	nonFastForward *PatternMatcher
}

// defaultClassifier is the package-level classifier using standard patterns.
//
//nolint:gochecknoglobals // Singleton classifier for package use
var defaultClassifier = &ErrorClassifier{
	auth:           authPatterns,
	network:        networkPatterns,
	rateLimit:      rateLimitPatterns,
	notFound:       notFoundPatterns,
	nonFastForward: nonFastForwardPatterns,
}

// ClassifyError determines the error type from an error string.
// The string is lowercased before matching. Returns ErrorTypeUnknown if
// the error doesn't match any known pattern.
//
// Classification priority (first match wins):
// 1. Rate limit (most specific, usually indicates temporary failure)
// 2. Authentication (actionable - user can fix credentials)
// 3. Network (often transient, retry may help)
// 4. Non-fast-forward (git-specific, requires pull)
// 5. Not found (general, last resort)
func ClassifyError(errStr string) ErrorType {
	return defaultClassifier.Classify(errStr)
}

// Classify determines the error type from an error string.
// See ClassifyError for classification priority.
func (c *ErrorClassifier) Classify(errStr string) ErrorType {
	lower := strings.ToLower(errStr)
	return c.classifyLower(lower)
}

// classifyLower performs classification on an already-lowercased string.
func (c *ErrorClassifier) classifyLower(lower string) ErrorType {
	// Order matters: more specific patterns first
	if c.rateLimit.MatchesLower(lower) {
		return ErrorTypeRateLimit
	}
	if c.auth.MatchesLower(lower) {
		return ErrorTypeAuth
	}
	if c.network.MatchesLower(lower) {
		return ErrorTypeNetwork
	}
	if c.nonFastForward.MatchesLower(lower) {
		return ErrorTypeNonFastForward
	}
	if c.notFound.MatchesLower(lower) {
		return ErrorTypeNotFound
	}
	return ErrorTypeUnknown
}

// MatchesAuthError checks if the error string indicates an authentication error.
// The input string is lowercased before matching.
func MatchesAuthError(errStr string) bool {
	return authPatterns.Matches(errStr)
}

// MatchesNetworkError checks if the error string indicates a network error.
// The input string is lowercased before matching.
func MatchesNetworkError(errStr string) bool {
	return networkPatterns.Matches(errStr)
}

// MatchesRateLimitError checks if the error string indicates a rate limit error.
// The input string is lowercased before matching.
func MatchesRateLimitError(errStr string) bool {
	return rateLimitPatterns.Matches(errStr)
}

// MatchesNotFoundError checks if the error string indicates a not-found error.
// The input string is lowercased before matching.
func MatchesNotFoundError(errStr string) bool {
	return notFoundPatterns.Matches(errStr)
}

// MatchesNonFastForwardError checks if the error string indicates a non-fast-forward rejection.
// The input string is lowercased before matching.
func MatchesNonFastForwardError(errStr string) bool {
	return nonFastForwardPatterns.Matches(errStr)
}
