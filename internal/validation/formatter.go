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
func FormatResult(result *PipelineResult) string {
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
			sb.WriteString(formatFailedCommand(r))
		}
	}

	sb.WriteString("\n### Suggested Actions\n\n")
	sb.WriteString("1. **Retry with AI fix** - Let AI attempt to fix the issues\n")
	sb.WriteString("2. **Fix manually** - Make changes in worktree, then `atlas resume`\n")
	sb.WriteString("3. **Abandon task** - End task, preserve branch\n")

	return sb.String()
}

// formatFailedCommand formats a single failed command result.
func formatFailedCommand(r Result) string {
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
		if len(r.Stdout) < maxStdoutDisplay {
			sb.WriteString("**Standard output:**\n```\n")
			sb.WriteString(r.Stdout)
			sb.WriteString("\n```\n")
		} else {
			sb.WriteString("**Standard output (truncated):**\n```\n")
			sb.WriteString(r.Stdout[:maxStdoutDisplay])
			sb.WriteString("\n...[truncated, see validation.json artifact for full output]\n```\n")
		}
	}

	sb.WriteString("\n")
	return sb.String()
}
