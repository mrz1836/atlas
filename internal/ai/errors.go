package ai

import (
	"fmt"
	"strings"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// CLIInfo contains provider-specific information for error messages.
type CLIInfo struct {
	Name          string // CLI command name (e.g., "claude", "gemini", "codex")
	InstallHint   string // Installation instructions
	ErrType       error  // Sentinel error type for this provider
	EnvVar        string // API key environment variable name
	StatusPageURL string // Provider status page (shown to user on outage)
}

// WrapCLIExecutionError wraps an execution error with provider-specific context.
// This is shared logic used by all CLI-based AI runners.
func WrapCLIExecutionError(info CLIInfo, err error, stderr []byte) error {
	return WrapCLIExecutionErrorWithOp(info, "", err, stderr)
}

// WrapCLIExecutionErrorWithOp wraps an execution error with provider-specific context
// and operation information for better debugging.
func WrapCLIExecutionErrorWithOp(info CLIInfo, operation string, err error, stderr []byte) error {
	stderrStr := strings.TrimSpace(string(stderr))
	opContext := formatOpContext(operation)

	// Check for CLI not found
	if strings.Contains(stderrStr, "command not found") ||
		strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("%w: %s CLI not found%s - %s", info.ErrType, info.Name, opContext, info.InstallHint)
	}

	// Check for API key errors
	if strings.Contains(stderrStr, "api key") ||
		strings.Contains(stderrStr, "API key") ||
		strings.Contains(stderrStr, "authentication") ||
		strings.Contains(stderrStr, info.EnvVar) {
		return fmt.Errorf("%w: API key error%s: %s", info.ErrType, opContext, stderrStr)
	}

	// Check for provider outage (5xx, overloaded). When matched, wrap with
	// ErrProviderOutage so downstream code (FallbackRunner, retry backoff,
	// TUI display) can detect it via errors.Is.
	if isOutageStderr(stderrStr) || isOutageStderr(err.Error()) {
		detail := stderrStr
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%w: %w%s: %s", info.ErrType, atlaserrors.ErrProviderOutage, opContext, detail)
	}

	// Default error wrapping
	if stderrStr != "" {
		return fmt.Errorf("%w%s: %s", info.ErrType, opContext, stderrStr)
	}

	return fmt.Errorf("%w%s: %s", info.ErrType, opContext, err.Error())
}

// isOutageStderr reports whether the given output text contains markers of an
// AI provider outage (5xx, overloaded). Matched case-insensitively against the
// shared providerOutagePatterns list defined in retry.go.
func isOutageStderr(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, p := range providerOutagePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// formatOpContext formats the operation context for error messages.
// Returns empty string if operation is empty.
func formatOpContext(operation string) string {
	if operation == "" {
		return ""
	}
	return " while " + operation
}
