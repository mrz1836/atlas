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

// authConfigErrorMarkers indicate the failure was an authentication,
// account-eligibility, or provider-configuration problem (bad/expired
// credentials, an unsupported client, a discontinued plan/tier, or a
// quota/billing issue). These are actionable by the user and will NOT resolve
// by waiting, so they must never be shown as a transient "outage". They take
// precedence over outageErrorMarkers so that an auth error wrapped in a generic
// "all AI fallback options exhausted" message is classified correctly.
//
//nolint:gochecknoglobals // Read-only pattern list
var authConfigErrorMarkers = []string{
	"ineligibletiererror",
	"unsupported_client",
	"no longer supported",
	"error authenticating",
	"authentication failed",
	"unauthorized",
	"invalid api key",
	"invalid_api_key",
	"api key not found",
	"missing api key",
	"permission denied",
	"forbidden",
	"please migrate",
	"migrate to the",
	"insufficient_quota",
	"exceeded your current quota",
}

// failureKind categorizes a task's last error so the manual-fix display can
// show the most accurate, least-misleading guidance.
type failureKind int

const (
	// failureGeneric is a normal validation/code failure (the default).
	failureGeneric failureKind = iota
	// failureOutage is a transient AI provider outage (5xx / overloaded).
	failureOutage
	// failureAuthConfig is an auth/eligibility/configuration problem the user
	// must fix (credentials, plan/tier, quota, or agent configuration).
	failureAuthConfig
)

// classifyFailure categorizes an error summary. Auth/config problems are
// checked first because their errors are frequently wrapped in a generic
// "all AI fallback options exhausted" message that also matches an outage
// marker — and telling the user to "wait for recovery" on an auth failure is
// exactly the misleading behavior we want to avoid.
func classifyFailure(s string) failureKind {
	if s == "" {
		return failureGeneric
	}
	lower := strings.ToLower(s)
	for _, m := range authConfigErrorMarkers {
		if strings.Contains(lower, m) {
			return failureAuthConfig
		}
	}
	for _, m := range outageErrorMarkers {
		if strings.Contains(lower, m) {
			return failureOutage
		}
	}
	return failureGeneric
}

// isOutageErrorSummary reports whether the error summary string indicates a
// provider outage. Used to swap the manual-fix display for an outage banner.
func isOutageErrorSummary(s string) bool {
	return classifyFailure(s) == failureOutage
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

	switch classifyFailure(info.ErrorSummary) {
	case failureAuthConfig:
		output.Info(renderAuthConfigBlock(info))
		return
	case failureOutage:
		output.Info(renderOutageBlock(info))
		return
	case failureGeneric:
		// Fall through to the standard validation/manual-fix display below.
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

// renderAuthConfigBlock renders the failure display for an AI provider
// authentication, eligibility, or configuration error. Unlike an outage, this
// will not resolve by waiting — the user must fix credentials, their plan/tier,
// or the agent configuration for the failed step.
func renderAuthConfigBlock(info *ManualFixInfo) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("⚠ AI Provider Auth/Config Error (not a code issue — and not an outage)\n")
	sb.WriteString("──────────────────────────────────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "📁 Worktree Path:\n   %s\n", info.WorktreePath)

	if info.FailedStep != "" {
		fmt.Fprintf(&sb, "❌ Failed Step: %s\n", info.FailedStep)
	}

	sb.WriteString("🔑 An AI provider rejected the request due to authentication,\n")
	sb.WriteString("   account eligibility, quota, or configuration. Waiting will NOT\n")
	sb.WriteString("   fix it.\n\n")

	if info.ErrorSummary != "" {
		sb.WriteString("📋 Last Error:\n")
		for _, line := range strings.Split(info.ErrorSummary, "\n") {
			fmt.Fprintf(&sb, "   %s\n", line)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("📝 Next Steps:\n")
	sb.WriteString("   1. Verify the provider CLI is authenticated and your API key/plan is valid\n")
	sb.WriteString("   2. Or switch this step to a working agent (operations.<step>.agent in\n")
	sb.WriteString("      ~/.atlas/config.yaml, or the template's step config)\n")
	sb.WriteString("   3. Re-run the resume command below after fixing\n\n")

	fmt.Fprintf(&sb, "▶ Resume Command:\n   %s\n\n", info.ResumeCommand)

	fmt.Fprintf(&sb, "💡 To abandon the task and preserve the worktree for manual work:\n   atlas abandon %s", info.WorkspaceName)

	return sb.String()
}
