// Package logging provides logging utilities including sensitive data filtering.
// This package contains hooks and utilities for zerolog that help ensure
// sensitive data is never written to log files.
package logging

import (
	"io"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
)

// RedactedValue is the replacement string for sensitive data.
const RedactedValue = "[REDACTED]"

// sensitivePatterns contains compiled regular expressions for detecting sensitive values.
// These patterns match common API key, token, and credential formats.
var sensitivePatterns = []*regexp.Regexp{ //nolint:gochecknoglobals // Package-level patterns for reuse
	// Anthropic API keys (sk-ant-api...)
	regexp.MustCompile(`sk-ant-api[a-zA-Z0-9_-]+`),

	// OpenAI API keys (sk-...)
	regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),

	// GitHub tokens (ghp_, gho_, ghu_, ghs_, ghr_)
	regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{20,}`),

	// Generic API keys (any string with api_key, apikey, api-key followed by value)
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["']?([a-zA-Z0-9_-]{16,})["']?`),

	// Bearer tokens
	regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_-]{20,}`),

	// Authorization headers with tokens
	regexp.MustCompile(`(?i)authorization\s*[:=]\s*["']?[a-zA-Z0-9_-]{20,}["']?`),

	// Generic secret patterns (secret, password, credential, token with values)
	regexp.MustCompile(`(?i)(secret|password|credential|passwd|pwd)\s*[:=]\s*["']?[^\s"']{8,}["']?`),

	// SSH private keys (starts with -----)
	regexp.MustCompile(`(?i)-----BEGIN[A-Z\s]+PRIVATE KEY-----`),

	// Base64-encoded secrets that look like tokens (long alphanumeric strings)
	regexp.MustCompile(`(?i)(token|auth)\s*[:=]\s*["']?[a-zA-Z0-9+/=]{32,}["']?`),
}

// sensitiveFieldNames contains field names that should always have their values redacted.
// Case-insensitive matching is performed.
var sensitiveFieldNames = []string{ //nolint:gochecknoglobals // Package-level patterns for reuse
	"api_key",
	"apikey",
	"api-key",
	"auth_token",
	"authtoken",
	"auth-token",
	"password",
	"passwd",
	"secret",
	"credential",
	"credentials",
	"private_key",
	"privatekey",
	"private-key",
	"access_token",
	"accesstoken",
	"access-token",
	"refresh_token",
	"refreshtoken",
	"refresh-token",
	"bearer",
	"authorization",
	"anthropic_api_key",
	"github_token",
	"openai_api_key",
}

// SensitiveDataHook is a zerolog hook that filters sensitive data from log entries.
// It examines string values in log events and redacts any content that matches
// known sensitive patterns or field names.
type SensitiveDataHook struct{}

// NewSensitiveDataHook creates a new SensitiveDataHook for filtering sensitive data.
func NewSensitiveDataHook() *SensitiveDataHook {
	return &SensitiveDataHook{}
}

// Run implements the zerolog.Hook interface.
// It examines the log event and redacts sensitive data.
// Zerolog hooks have limited access to event data. This hook primarily
// works by filtering the message string. For field-level filtering,
// use FilterSensitiveValue when constructing log entries.
func (h *SensitiveDataHook) Run(e *zerolog.Event, _ zerolog.Level, msg string) {
	// The zerolog.Event doesn't expose a way to modify fields directly,
	// but we can add context that indicates filtering was applied.
	// The main filtering happens via FilterSensitiveValue used at log call sites.

	// Filter the message if it contains sensitive data
	if ContainsSensitiveData(msg) {
		// Unfortunately, zerolog doesn't allow modifying the message in a hook.
		// The message filtering must be done at the call site.
		// This hook serves as a fallback to at least flag potentially sensitive logs.
		e.Bool("contains_filtered_data", true)
	}
}

// ContainsSensitiveData checks if a string contains any sensitive data patterns.
// Returns true if any sensitive pattern is found.
func ContainsSensitiveData(s string) bool {
	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(s) {
			return true
		}
	}
	return false
}

// FilterSensitiveValue filters sensitive data from a string value.
// It replaces any matches of sensitive patterns with [REDACTED].
// This function should be used when logging potentially sensitive values.
func FilterSensitiveValue(value string) string {
	result := value
	for _, pattern := range sensitivePatterns {
		result = pattern.ReplaceAllString(result, RedactedValue)
	}
	return result
}

// IsSensitiveFieldName checks if a field name indicates sensitive data.
// Returns true if the field name matches any known sensitive field name patterns.
func IsSensitiveFieldName(fieldName string) bool {
	lowerName := strings.ToLower(fieldName)
	for _, sensitive := range sensitiveFieldNames {
		if lowerName == sensitive || strings.Contains(lowerName, sensitive) {
			return true
		}
	}
	return false
}

// RedactIfSensitive returns [REDACTED] if the field name indicates sensitive data,
// otherwise returns the original value.
// Use this when logging field values that might be sensitive.
func RedactIfSensitive(fieldName, value string) string {
	if IsSensitiveFieldName(fieldName) {
		return RedactedValue
	}
	return FilterSensitiveValue(value)
}

// SafeValue returns a filtered value for a field, redacting sensitive data.
// This is a convenience wrapper for adding filtered string fields to log events.
//
// Usage:
//
//	log.Info().Str("config", logging.SafeValue("config", configValue)).Msg("loaded config")
func SafeValue(fieldName, value string) string {
	return RedactIfSensitive(fieldName, value)
}

// FilteringWriter wraps an io.Writer and filters sensitive data from output.
// This is used to wrap log file writers to ensure sensitive data is never
// written to disk, even if it appears in log messages or field values.
type FilteringWriter struct {
	w io.Writer
}

// NewFilteringWriter creates a new FilteringWriter that wraps the given writer.
// All data written through this writer will have sensitive patterns redacted.
func NewFilteringWriter(w io.Writer) *FilteringWriter {
	return &FilteringWriter{w: w}
}

// Write implements io.Writer, filtering sensitive data before writing.
func (fw *FilteringWriter) Write(p []byte) (n int, err error) {
	// Filter the data before writing
	filtered := FilterSensitiveValue(string(p))
	// Write the filtered data, but return original length to satisfy io.Writer contract
	_, err = fw.w.Write([]byte(filtered))
	if err != nil {
		return 0, err
	}
	// Return original length so callers don't think there was a short write
	return len(p), nil
}
