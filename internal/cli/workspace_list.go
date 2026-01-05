// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
	"github.com/spf13/cobra"
)

// tableStyles holds lipgloss styles for table rendering.
type tableStyles struct {
	header       lipgloss.Style
	cell         lipgloss.Style
	dim          lipgloss.Style
	statusColors map[constants.WorkspaceStatus]lipgloss.AdaptiveColor
}

// newTableStyles creates styles for the workspace list table.
func newTableStyles() *tableStyles {
	return &tableStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"}),
		cell: lipgloss.NewStyle(),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}),
		// Semantic colors for workspace statuses (UX-6)
		statusColors: map[constants.WorkspaceStatus]lipgloss.AdaptiveColor{
			constants.WorkspaceStatusActive: {Light: "#0087AF", Dark: "#00D7FF"}, // Blue
			constants.WorkspaceStatusPaused: {Light: "#585858", Dark: "#6C6C6C"}, // Gray
			constants.WorkspaceStatusClosed: {Light: "#585858", Dark: "#6C6C6C"}, // Dim
		},
	}
}

// addWorkspaceListCmd adds the list subcommand to the workspace command.
func addWorkspaceListCmd(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all workspaces",
		Long: `Display a table of all ATLAS workspaces with their status,
branch, creation time, and task count.

Examples:
  atlas workspace list              # Display as styled table
  atlas workspace list --output json # Display as JSON array
  atlas workspace ls                 # Alias for list`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkspaceList(cmd.Context(), cmd, os.Stdout)
		},
	}
	parent.AddCommand(cmd)
}

// runWorkspaceList executes the workspace list command.
func runWorkspaceList(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := GetLogger()

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Create store and manager
	store, err := workspace.NewFileStore("")
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Create manager (nil worktreeRunner OK for List operation)
	mgr := workspace.NewManager(store, nil)

	// Get all workspaces
	workspaces, err := mgr.List(ctx)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to list workspaces")
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Handle empty case
	if len(workspaces) == 0 {
		if output == OutputJSON {
			_, _ = fmt.Fprintln(w, "[]")
		} else {
			_, _ = fmt.Fprintln(w, "No workspaces. Run 'atlas start' to create one.")
		}
		return nil
	}

	// Output based on format
	if output == OutputJSON {
		return outputWorkspacesJSON(w, workspaces)
	}

	return outputWorkspacesTable(w, workspaces)
}

// outputWorkspacesJSON outputs workspaces as JSON array.
func outputWorkspacesJSON(w io.Writer, workspaces []*domain.Workspace) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(workspaces); err != nil {
		return fmt.Errorf("failed to encode workspaces to JSON: %w", err)
	}
	return nil
}

// outputWorkspacesTable outputs workspaces as a styled table.
func outputWorkspacesTable(w io.Writer, workspaces []*domain.Workspace) error {
	styles := newTableStyles()

	// Define column widths
	const (
		nameWidth    = 12
		branchWidth  = 20
		statusWidth  = 10
		createdWidth = 15
		tasksWidth   = 5
	)

	// Print header
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %*s",
		nameWidth, "NAME",
		branchWidth, "BRANCH",
		statusWidth, "STATUS",
		createdWidth, "CREATED",
		tasksWidth, "TASKS",
	)
	_, _ = fmt.Fprintln(w, styles.header.Render(header))

	// Print rows
	for _, ws := range workspaces {
		// Format name (truncate if needed)
		name := ws.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		// Format branch (truncate if needed)
		branch := ws.Branch
		if len(branch) > branchWidth {
			branch = branch[:branchWidth-1] + "…"
		}

		// Format status with color
		statusStr := string(ws.Status)
		if color, ok := styles.statusColors[ws.Status]; ok {
			statusStyle := lipgloss.NewStyle().Foreground(color)
			statusStr = statusStyle.Render(statusStr)
		}

		// Format created time as relative
		createdStr := tui.RelativeTime(ws.CreatedAt)

		// Count tasks
		taskCount := len(ws.Tasks)

		// Build and print row
		row := fmt.Sprintf("%-*s %-*s %-*s %-*s %*d",
			nameWidth, name,
			branchWidth, branch,
			statusWidth+tui.ColorOffset(statusStr, string(ws.Status)), statusStr,
			createdWidth, createdStr,
			tasksWidth, taskCount,
		)
		_, _ = fmt.Fprintln(w, row)
	}

	return nil
}

// getStatusColors returns the semantic color definitions for workspace statuses.
// Exported for testing purposes. Delegates to tui package.
func getStatusColors() map[constants.WorkspaceStatus]lipgloss.AdaptiveColor {
	return tui.StatusColors()
}
