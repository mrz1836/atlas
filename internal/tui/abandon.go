// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
)

// AbandonInfo contains information for abandonment success display.
type AbandonInfo struct {
	WorkspaceName string
	BranchName    string
	WorktreePath  string
	TaskID        string
}

// ExtractAbandonInfo extracts abandon information from task and workspace.
func ExtractAbandonInfo(task *domain.Task, workspace *domain.Workspace) *AbandonInfo {
	return &AbandonInfo{
		WorkspaceName: workspace.Name,
		BranchName:    workspace.Branch,
		WorktreePath:  workspace.WorktreePath,
		TaskID:        task.ID,
	}
}

// DisplayAbandonmentSuccess shows the user the abandonment result.
func DisplayAbandonmentSuccess(output Output, task *domain.Task, workspace *domain.Workspace) {
	info := ExtractAbandonInfo(task, workspace)

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("✗ Task Abandoned\n")
	sb.WriteString("───────────────────────────────────────────\n\n")

	sb.WriteString(fmt.Sprintf("📋 Task ID: %s\n", info.TaskID))
	sb.WriteString(fmt.Sprintf("🌿 Branch: %s (preserved)\n", info.BranchName))
	sb.WriteString(fmt.Sprintf("📁 Worktree: %s (preserved)\n\n", info.WorktreePath))

	sb.WriteString("📝 Next Steps:\n")
	sb.WriteString("   • Navigate to the worktree path to continue work manually\n")
	sb.WriteString("   • Run 'atlas start' in the same workspace for a new task\n")
	sb.WriteString(fmt.Sprintf("   • Run 'atlas workspace destroy %s' to clean up later\n", info.WorkspaceName))

	output.Info(sb.String())
}
