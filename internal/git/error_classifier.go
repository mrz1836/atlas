// Package git provides Git operations for ATLAS.
// This file contains generic error classification utilities.
package git

import "strings"

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
	)
)

// MatchesAuthError checks if the error string indicates an authentication error.
func MatchesAuthError(errStr string) bool {
	return authPatterns.MatchesLower(errStr)
}

// MatchesNetworkError checks if the error string indicates a network error.
func MatchesNetworkError(errStr string) bool {
	return networkPatterns.MatchesLower(errStr)
}

// MatchesRateLimitError checks if the error string indicates a rate limit error.
func MatchesRateLimitError(errStr string) bool {
	return rateLimitPatterns.MatchesLower(errStr)
}

// MatchesNotFoundError checks if the error string indicates a not-found error.
func MatchesNotFoundError(errStr string) bool {
	return notFoundPatterns.MatchesLower(errStr)
}

// MatchesNonFastForwardError checks if the error string indicates a non-fast-forward rejection.
func MatchesNonFastForwardError(errStr string) bool {
	return nonFastForwardPatterns.MatchesLower(errStr)
}
