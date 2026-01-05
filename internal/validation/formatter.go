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

	sb.WriteString("\n### Suggested Actions\n\n")
	sb.WriteString("1. **Retry with AI fix** - Let AI attempt to fix the issues\n")
	sb.WriteString("2. **Fix manually** - Make changes in worktree, then `atlas resume`\n")
	sb.WriteString("3. **Abandon task** - End task, preserve branch\n")

	return sb.String()
}

// formatFailedCommand formats a single failed command result.
// The artifactPath is included in truncation messages when provided.
func formatFailedCommand(r Result, artifactPath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("### Command: %s\n", r.Command))
	sb.WriteString(fmt.Sprintf("Exit code: %d\n", r.ExitCode))

	if r.Stderr != "" {
		sb.WriteString("**Error output:**\n```\n")
		sb.WriteString(r.Stderr)
		sb.WriteString("\n```\n")
	}

	// Show stdout, truncating if necessary
	if r.Stdout != "" {
		sb.WriteString(formatStdout(r.Stdout, artifactPath))
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatStdout formats stdout output, truncating if necessary.
func formatStdout(stdout, artifactPath string) string {
	if len(stdout) < maxStdoutDisplay {
		return fmt.Sprintf("**Standard output:**\n```\n%s\n```\n", stdout)
	}

	// Truncated output
	truncatedMsg := "\n...[truncated, see validation.json artifact for full output]\n```\n"
	if artifactPath != "" {
		truncatedMsg = fmt.Sprintf("\n...[truncated, see validation.json artifact for full output]\n\n📄 Full output saved to: %s\n```\n", artifactPath)
	}
	return fmt.Sprintf("**Standard output (truncated):**\n```\n%s%s", stdout[:maxStdoutDisplay], truncatedMsg)
}
