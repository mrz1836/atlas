package validation

import (
	"fmt"
	"strings"
)

// maxStdoutDisplay is the maximum length of stdout to display in formatted output.
// Longer output is truncated to prevent overwhelming the user.
const maxStdoutDisplay = 1000

// FormatResult formats a PipelineResult for human-readable display.
// It provides a clear summary of validation results with emphasis on failures.
// The artifactPath parameter is optional and will be included in truncated output messages.
func FormatResult(result *PipelineResult) string {
	return FormatResultWithArtifact(result, "")
}

// FormatResultWithArtifact formats a PipelineResult with an optional artifact path.
// When artifactPath is provided, it will be shown in truncated output messages
// to help users find the full validation output.
// Output is plain text suitable for terminal display.
func FormatResultWithArtifact(result *PipelineResult, artifactPath string) string {
	var sb strings.Builder

	if result.Success {
		sb.WriteString("✓ All validations passed\n")
		sb.WriteString(fmt.Sprintf("  Duration: %dms\n", result.DurationMs))
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("✗ Validation failed at: %s\n\n", result.FailedStepName))

	// Format each failed result
	for _, r := range result.AllResults() {
		if !r.Success {
			sb.WriteString(formatFailedCommand(r, artifactPath))
		}
	}

	return sb.String()
}

// formatFailedCommand formats a single failed command result.
// The artifactPath is included in truncation messages when provided.
// Output is plain text suitable for terminal display.
func formatFailedCommand(r Result, artifactPath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Command: %s\n", r.Command))
	sb.WriteString(fmt.Sprintf("Exit code: %d\n", r.ExitCode))

	if r.Stderr != "" {
		sb.WriteString("Error output:\n")
		// Indent stderr for readability
		for _, line := range strings.Split(r.Stderr, "\n") {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	// Show stdout, truncating if necessary
	if r.Stdout != "" {
		sb.WriteString(formatStdout(r.Stdout, artifactPath))
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatStdout formats stdout output, truncating if necessary.
// Output is plain text suitable for terminal display.
func formatStdout(stdout, artifactPath string) string {
	var sb strings.Builder

	if len(stdout) < maxStdoutDisplay {
		sb.WriteString("\nStandard output:\n")
		// Indent stdout for readability
		for _, line := range strings.Split(stdout, "\n") {
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
		return sb.String()
	}

	// Truncated output
	sb.WriteString("\nStandard output (truncated):\n")
	for _, line := range strings.Split(stdout[:maxStdoutDisplay], "\n") {
		sb.WriteString(fmt.Sprintf("  %s\n", line))
	}
	sb.WriteString("  ...[truncated]\n")
	if artifactPath != "" {
		sb.WriteString(fmt.Sprintf("📄 Full output saved to: %s\n", artifactPath))
	}
	return sb.String()
}
