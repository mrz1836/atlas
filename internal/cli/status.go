// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// WorkspaceLister defines the interface for listing workspaces.
// Used for dependency injection in tests.
type WorkspaceLister interface {
	List(ctx context.Context) ([]*domain.Workspace, error)
}

// TaskLister defines the interface for listing tasks.
// Used for dependency injection in tests.
type TaskLister interface {
	List(ctx context.Context, workspaceName string) ([]*domain.Task, error)
}

// AddStatusCommand adds the status command to the root command.
func AddStatusCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show workspace status dashboard",
		Long: `Display status of all ATLAS workspaces with their current task state,
showing which workspaces need attention, are running, or completed.

The status table shows:
  • WORKSPACE - Name of the workspace
  • BRANCH    - Git branch being worked on
  • STATUS    - Current task status with icon
  • STEP      - Progress as current/total steps
  • ACTION    - Suggested next action if any

Workspaces are sorted by priority: attention-required states first,
then running states, then others.

Examples:
  atlas status              # Display styled status table
  atlas status --output json # Display as JSON array
  atlas status --quiet      # Show table only (no header/footer)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), cmd, os.Stdout)
		},
	}
	parent.AddCommand(cmd)
}

// runStatus executes the status command with production dependencies.
func runStatus(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get flags
	output := cmd.Flag("output").Value.String()
	quiet := cmd.Flag("quiet").Value.String() == "true"

	// Respect NO_COLOR
	tui.CheckNoColor()

	// Create production dependencies
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	wsMgr := workspace.NewManager(wsStore, nil)

	taskStore, err := task.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create task store: %w", err)
	}

	return runStatusWithDeps(ctx, w, output, quiet, wsMgr, taskStore)
}

// runStatusWithDeps executes the status command with injected dependencies.
// This enables testing with mock implementations.
func runStatusWithDeps(
	ctx context.Context,
	w io.Writer,
	output string,
	quiet bool,
	wsMgr WorkspaceLister,
	taskStore TaskLister,
) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Load workspaces
	workspaces, err := wsMgr.List(ctx)
	if err != nil {
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

	// Build status rows from workspaces
	rows, err := buildStatusRows(ctx, workspaces, taskStore)
	if err != nil {
		return fmt.Errorf("failed to build status rows: %w", err)
	}

	// Sort by status priority (attention first)
	sortByStatusPriority(rows)

	// Output based on format
	if output == OutputJSON {
		return outputStatusJSON(w, rows)
	}

	return outputStatusTable(w, rows, quiet)
}

// buildStatusRows builds StatusRow slice from workspaces.
func buildStatusRows(
	ctx context.Context,
	workspaces []*domain.Workspace,
	taskStore TaskLister,
) ([]tui.StatusRow, error) {
	rows := make([]tui.StatusRow, 0, len(workspaces))

	for _, ws := range workspaces {
		// Check for cancellation during iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		row := tui.StatusRow{
			Workspace: ws.Name,
			Branch:    ws.Branch,
			Status:    constants.TaskStatusPending, // Default
		}

		// Get most recent task status
		if len(ws.Tasks) > 0 {
			row.Status = ws.Tasks[0].Status

			// Load full task to get step info
			tasks, err := taskStore.List(ctx, ws.Name)
			if err == nil && len(tasks) > 0 {
				mostRecent := tasks[0] // Already sorted newest first
				row.Status = mostRecent.Status
				row.CurrentStep = mostRecent.CurrentStep + 1 // 1-indexed for display
				row.TotalSteps = len(mostRecent.Steps)
			}
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// sortByStatusPriority sorts rows by status priority (attention first, then running).
func sortByStatusPriority(rows []tui.StatusRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		return statusPriority(rows[i].Status) > statusPriority(rows[j].Status)
	})
}

// statusPriority returns the priority level for a task status.
// Higher values = higher priority (shown first).
func statusPriority(status constants.TaskStatus) int {
	if tui.IsAttentionStatus(status) {
		return 2 // Highest priority
	}
	if status == constants.TaskStatusRunning || status == constants.TaskStatusValidating {
		return 1 // Middle priority
	}
	return 0 // Lowest priority
}

// outputStatusJSON outputs status as JSON array.
func outputStatusJSON(w io.Writer, rows []tui.StatusRow) error {
	table := tui.NewStatusTable(rows)
	headers, data := table.ToJSONData()

	// Convert to array of objects with full field names
	result := make([]map[string]string, len(data))
	for i, row := range data {
		obj := make(map[string]string)
		for j, header := range headers {
			// Use lowercase field names for JSON
			key := toLowerCamelCase(header)
			if j < len(row) {
				obj[key] = row[j]
			}
		}
		result[i] = obj
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// toLowerCamelCase converts UPPER_CASE to lowercase.
func toLowerCamelCase(s string) string {
	switch s {
	case "WORKSPACE":
		return "workspace"
	case "BRANCH":
		return "branch"
	case "STATUS":
		return "status"
	case "STEP":
		return "step"
	case "ACTION":
		return "action"
	default:
		return s
	}
}

// outputStatusTable outputs status as styled table with header and footer.
func outputStatusTable(w io.Writer, rows []tui.StatusRow, quiet bool) error {
	table := tui.NewStatusTable(rows)

	// Header (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w, "═══ ATLAS ═══")
		_, _ = fmt.Fprintln(w)
	}

	// Table
	if err := table.Render(w); err != nil {
		return err
	}

	// Footer (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, buildFooter(rows))
	}

	return nil
}

// buildFooter creates the footer summary and actionable command.
func buildFooter(rows []tui.StatusRow) string {
	attentionCount, firstAttention := countAttention(rows)

	// Summary line with proper singular/plural grammar
	workspaceWord := "workspaces"
	if len(rows) == 1 {
		workspaceWord = "workspace"
	}
	summary := fmt.Sprintf("%d %s", len(rows), workspaceWord)

	if attentionCount > 0 {
		needWord := "need"
		if attentionCount == 1 {
			needWord = "needs"
		}
		summary += fmt.Sprintf(", %d %s attention", attentionCount, needWord)
	}

	// Actionable command
	if firstAttention != nil {
		summary += buildActionableSuggestion(firstAttention)
	}

	return summary
}

// countAttention counts workspaces needing attention and returns the first one.
func countAttention(rows []tui.StatusRow) (int, *tui.StatusRow) {
	var count int
	var first *tui.StatusRow

	for i := range rows {
		if tui.IsAttentionStatus(rows[i].Status) {
			count++
			if first == nil {
				first = &rows[i]
			}
		}
	}

	return count, first
}

// buildActionableSuggestion builds the "Run: ..." suggestion for a workspace.
// Always includes the workspace name for consistency.
func buildActionableSuggestion(row *tui.StatusRow) string {
	action := tui.SuggestedAction(row.Status)
	if action == "" {
		return ""
	}

	// All actions include the workspace name for consistency
	return "\nRun: " + action + " " + row.Workspace
}
