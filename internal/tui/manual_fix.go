// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
)

// outageErrorMarkers are substrings that indicate the last error was an AI
// provider outage (5xx / overloaded) rather than a code/validation problem.
// Matched case-insensitively against the task's last error string.
//
//nolint:gochecknoglobals // Read-only pattern list
var outageErrorMarkers = []string{
	"ai provider outage",
	"all ai fallback options exhausted",
	"overloaded_error",
	"overloaded",
	"service unavailable",
	"internal server error",
	"bad gateway",
	"gateway timeout",
	"upstream connect error",
	"upstream connection error",
	"api error: 5",
	"http 5",
	"529 ",
	"529:",
	"529}",
	"503 ",
	"503:",
}

// isOutageErrorSummary reports whether the error summary string indicates a
// provider outage. Used to swap the manual-fix display for an outage banner.
func isOutageErrorSummary(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for _, m := range outageErrorMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// ManualFixInfo contains information for manual fix display.
type ManualFixInfo struct {
	WorkspaceName    string
	WorktreePath     string
	ErrorSummary     string
	FailedStep       string
	ResumeCommand    string
	ValidationOutput string // Full output from failed validation step
	ArtifactPath     string // Path to validation artifact with full output
}

// ExtractManualFixInfo extracts manual fix information from task and workspace.
func ExtractManualFixInfo(task *domain.Task, workspace *domain.Workspace) *ManualFixInfo {
	worktreePath := workspace.WorktreePath
	if worktreePath == "" {
		worktreePath = "(workspace closed - worktree not available)"
	}

	info := &ManualFixInfo{
		WorkspaceName: workspace.Name,
		WorktreePath:  worktreePath,
		ResumeCommand: fmt.Sprintf("atlas resume %s", workspace.Name),
	}

	// Extract error info from task metadata
	if task.Metadata != nil {
		if lastErr, ok := task.Metadata["last_error"].(string); ok {
			info.ErrorSummary = lastErr
		}
	}

	// Get failed step name from current step
	if task.CurrentStep < len(task.Steps) {
		info.FailedStep = task.Steps[task.CurrentStep].Name
	}

	// Extract validation output and artifact path from step results
	for _, sr := range task.StepResults {
		if sr.Status == "failed" && sr.Output != "" {
			info.ValidationOutput = sr.Output
			// Extract artifact path from metadata if available
			if sr.Metadata != nil {
				if artifactPath, ok := sr.Metadata["artifact_path"].(string); ok {
					info.ArtifactPath = artifactPath
				}
			}
			break // Use the first failed step's output
		}
	}

	return info
}

// DisplayManualFixInstructions shows the user how to fix issues manually.
func DisplayManualFixInstructions(output Output, task *domain.Task, workspace *domain.Workspace) {
	info := ExtractManualFixInfo(task, workspace)

	if isOutageErrorSummary(info.ErrorSummary) {
		output.Info(renderOutageBlock(info))
		return
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("⚠ Validation Failed - Manual Fix Required\n")
	sb.WriteString("─────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "📁 Worktree Path:\n   %s\n", info.WorktreePath)

	if info.FailedStep != "" {
		fmt.Fprintf(&sb, "❌ Failed Step: %s\n", info.FailedStep)
	}

	// Show validation output if available, otherwise fall back to error summary
	// ValidationOutput already contains properly formatted markdown from formatter.go
	// with code blocks for stderr/stdout, so we don't wrap it in additional code blocks
	if info.ValidationOutput != "" {
		sb.WriteString("📋 Validation Output:\n")
		sb.WriteString(info.ValidationOutput)
		sb.WriteString("\n")
	} else if info.ErrorSummary != "" {
		sb.WriteString("📋 Error Details:\n")
		// Indent error output
		for _, line := range strings.Split(info.ErrorSummary, "\n") {
			fmt.Fprintf(&sb, "   %s\n", line)
		}
		sb.WriteString("\n")
	}

	// Show artifact path prominently if available
	if info.ArtifactPath != "" {
		fmt.Fprintf(&sb, "📄 Full Validation Log:\n   %s\n\n", info.ArtifactPath)
	}

	sb.WriteString("📝 Next Steps:\n")
	sb.WriteString("   1. Navigate to the worktree path above\n")
	sb.WriteString("   2. Fix the validation errors shown\n")
	sb.WriteString("   3. Run the resume command below\n\n")

	fmt.Fprintf(&sb, "▶ Resume Command:\n   %s\n\n", info.ResumeCommand)

	fmt.Fprintf(&sb, "💡 Alternatively, to abandon the task and preserve the worktree for manual work:\n   atlas abandon %s", info.WorkspaceName)

	output.Info(sb.String())
}

// renderOutageBlock renders the failure display for an AI provider outage. The
// goal is to make it unambiguous to the user that the failure was not caused by
// their code, and to point them at provider status pages.
func renderOutageBlock(info *ManualFixInfo) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("⚠ AI Provider Outage Detected (not a code issue)\n")
	sb.WriteString("─────────────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "📁 Worktree Path:\n   %s\n", info.WorktreePath)

	if info.FailedStep != "" {
		fmt.Fprintf(&sb, "❌ Failed Step: %s\n", info.FailedStep)
	}

	sb.WriteString("🌐 All configured AI providers returned outage errors.\n")
	sb.WriteString("   Check status pages:\n")
	sb.WriteString("     • https://status.anthropic.com   (claude)\n")
	sb.WriteString("     • https://status.openai.com      (codex)\n")
	sb.WriteString("     • https://status.cloud.google.com (gemini)\n\n")

	if info.ErrorSummary != "" {
		sb.WriteString("📋 Last Error:\n")
		for _, line := range strings.Split(info.ErrorSummary, "\n") {
			fmt.Fprintf(&sb, "   %s\n", line)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("📝 Next Steps:\n")
	sb.WriteString("   1. Wait a few minutes for provider recovery\n")
	sb.WriteString("   2. Run the resume command below\n\n")

	fmt.Fprintf(&sb, "▶ Resume Command:\n   %s\n\n", info.ResumeCommand)

	fmt.Fprintf(&sb, "💡 To abandon the task and preserve the worktree for manual work:\n   atlas abandon %s", info.WorkspaceName)

	return sb.String()
}
