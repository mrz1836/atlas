package ai

import (
	"fmt"
	"strings"
)

// CLIInfo contains provider-specific information for error messages.
type CLIInfo struct {
	Name        string // CLI command name (e.g., "claude", "gemini", "codex")
	InstallHint string // Installation instructions
	ErrType     error  // Sentinel error type for this provider
	EnvVar      string // API key environment variable name
}

// WrapCLIExecutionError wraps an execution error with provider-specific context.
// This is shared logic used by all CLI-based AI runners.
func WrapCLIExecutionError(info CLIInfo, err error, stderr []byte) error {
	stderrStr := strings.TrimSpace(string(stderr))

	// Check for CLI not found
	if strings.Contains(stderrStr, "command not found") ||
		strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("%w: %s CLI not found - %s", info.ErrType, info.Name, info.InstallHint)
	}

	// Check for API key errors
	if strings.Contains(stderrStr, "api key") ||
		strings.Contains(stderrStr, "API key") ||
		strings.Contains(stderrStr, "authentication") ||
		strings.Contains(stderrStr, info.EnvVar) {
		return fmt.Errorf("%w: API key error: %s", info.ErrType, stderrStr)
	}

	// Default error wrapping
	if stderrStr != "" {
		return fmt.Errorf("%w: %s", info.ErrType, stderrStr)
	}

	return fmt.Errorf("%w: %s", info.ErrType, err.Error())
}
