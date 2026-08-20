// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// tableStyles holds lipgloss styles for table rendering.
type tableStyles struct {
	header       lipgloss.Style
	cell         lipgloss.Style
	dim          lipgloss.Style
	statusColors map[constants.WorkspaceStatus]tui.AdaptiveColor
}

// newTableStyles creates styles for the workspace list table.
func newTableStyles() *tableStyles {
	return &tableStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.AdaptiveColor{Light: lipgloss.Color("#333333"), Dark: lipgloss.Color("#DDDDDD")}),
		cell: lipgloss.NewStyle(),
		dim: lipgloss.NewStyle().
			Foreground(tui.AdaptiveColor{Light: lipgloss.Color("#666666"), Dark: lipgloss.Color("#888888")}),
		// Semantic colors for workspace statuses (UX-6)
		statusColors: map[constants.WorkspaceStatus]tui.AdaptiveColor{
			constants.WorkspaceStatusActive: {Light: lipgloss.Color("#0087AF"), Dark: lipgloss.Color("#00D7FF")}, // Blue
			constants.WorkspaceStatusPaused: {Light: lipgloss.Color("#585858"), Dark: lipgloss.Color("#6C6C6C")}, // Gray
			constants.WorkspaceStatusClosed: {Light: lipgloss.Color("#585858"), Dark: lipgloss.Color("#6C6C6C")}, // Dim
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

	logger := Logger()

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Detect repo for scoped storage
	repoPath, err := detectRepoPath()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Create store and manager (repo-scoped)
	store, err := workspace.NewRepoScopedFileStore(repoPath)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Create manager (nil worktreeRunner OK for List operation)
	mgr := workspace.NewManager(store, nil, logger)

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

	return outputWorkspacesTable(w, workspaces, repoPath)
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

// countActiveTasks returns the number of non-terminal tasks in the workspace.
func countActiveTasks(ws *domain.Workspace) int {
	count := 0
	for _, taskRef := range ws.Tasks {
		if !task.IsTerminalStatus(taskRef.Status) {
			count++
		}
	}
	return count
}

// countCompletedTasks returns the number of terminal tasks in the workspace.
func countCompletedTasks(ws *domain.Workspace) int {
	count := 0
	for _, taskRef := range ws.Tasks {
		if task.IsTerminalStatus(taskRef.Status) {
			count++
		}
	}
	return count
}

// repoLabel returns a short, human-readable repo name for a workspace,
// derived from the basename of its RepoPath. Returns "-" when the repo path
// is unknown (e.g. legacy workspaces created before RepoPath was recorded).
func repoLabel(ws *domain.Workspace) string {
	if ws.RepoPath == "" {
		return "-"
	}
	return filepath.Base(ws.RepoPath)
}

// outputWorkspacesTable outputs workspaces as a styled table. Columns auto-fit
// to their content so names and branches are never truncated. repoPath is the
// current repo scope, shown as a header line above the table.
func outputWorkspacesTable(w io.Writer, workspaces []*domain.Workspace, repoPath string) error {
	styles := newTableStyles()

	// Scope header: which repo this listing is for.
	if repoPath != "" {
		_, _ = fmt.Fprintln(w, styles.dim.Render(fmt.Sprintf("Repo: %s (%s)", filepath.Base(repoPath), repoPath)))
		_, _ = fmt.Fprintln(w)
	}

	// Compute column widths from content (auto-fit). Text columns size to the
	// widest cell; numeric columns are effectively their header width.
	nameWidth := utf8.RuneCountInString("NAME")
	repoWidth := utf8.RuneCountInString("REPO")
	branchWidth := utf8.RuneCountInString("BRANCH")
	statusWidth := utf8.RuneCountInString("STATUS")
	createdWidth := utf8.RuneCountInString("CREATED")
	const (
		activeWidth    = 6 // "ACTIVE" header
		completedWidth = 9 // "COMPLETED" header
	)

	for _, ws := range workspaces {
		nameWidth = max(nameWidth, utf8.RuneCountInString(ws.Name))
		repoWidth = max(repoWidth, utf8.RuneCountInString(repoLabel(ws)))
		branchWidth = max(branchWidth, utf8.RuneCountInString(ws.Branch))
		statusWidth = max(statusWidth, utf8.RuneCountInString(string(ws.Status)))
		createdWidth = max(createdWidth, utf8.RuneCountInString(tui.RelativeTime(ws.CreatedAt)))
	}

	// Print header
	header := fmt.Sprintf(
		"%-*s %-*s %-*s %-*s %-*s %*s %*s",
		nameWidth, "NAME",
		repoWidth, "REPO",
		branchWidth, "BRANCH",
		statusWidth, "STATUS",
		createdWidth, "CREATED",
		activeWidth, "ACTIVE",
		completedWidth, "COMPLETED",
	)
	_, _ = fmt.Fprintln(w, styles.header.Render(header))

	// Print rows
	for _, ws := range workspaces {
		// Format status with color
		statusStr := string(ws.Status)
		if color, ok := styles.statusColors[ws.Status]; ok {
			statusStyle := lipgloss.NewStyle().Foreground(color)
			statusStr = statusStyle.Render(statusStr)
		}

		// Format created time as relative
		createdStr := tui.RelativeTime(ws.CreatedAt)

		// Count active and completed tasks
		activeCount := countActiveTasks(ws)
		completedCount := countCompletedTasks(ws)

		// Build and print row
		row := fmt.Sprintf(
			"%-*s %-*s %-*s %-*s %-*s %*d %*d",
			nameWidth, ws.Name,
			repoWidth, repoLabel(ws),
			branchWidth, ws.Branch,
			statusWidth+tui.ColorOffset(statusStr, string(ws.Status)), statusStr,
			createdWidth, createdStr,
			activeWidth, activeCount,
			completedWidth, completedCount,
		)
		_, _ = fmt.Fprintln(w, row)
	}

	return nil
}

// getStatusColors returns the semantic color definitions for workspace statuses.
// Exported for testing purposes. Delegates to tui package.
func getStatusColors() map[constants.WorkspaceStatus]tui.AdaptiveColor {
	return tui.StatusColors()
}
