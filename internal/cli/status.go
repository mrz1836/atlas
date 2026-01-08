// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
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

// MinWatchInterval is the minimum allowed refresh interval for watch mode.
// Prevents excessive CPU usage from too-frequent refreshes.
const MinWatchInterval = 500 * time.Millisecond

// DefaultWatchInterval is the default refresh interval for watch mode.
const DefaultWatchInterval = 2 * time.Second

// AddStatusCommand adds the status command to the root command.
func AddStatusCommand(parent *cobra.Command) {
	var watchMode bool
	var watchInterval time.Duration
	var showProgress bool

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

Watch mode (-w) enables live updates with automatic refresh.
Progress mode (-p) shows visual progress bars for active tasks.

Examples:
  atlas status              # Display styled status table
  atlas status --output json # Display as JSON array
  atlas status --quiet      # Show table only (no header/footer)
  atlas status --watch      # Live updating dashboard
  atlas status -w --interval 5s # Update every 5 seconds
  atlas status --progress   # Show progress bars for active tasks
  atlas status -w -p        # Watch mode with progress bars`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), cmd, os.Stdout, watchMode, watchInterval, showProgress)
		},
	}

	// Watch mode flags
	cmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Enable watch mode with live updates")
	cmd.Flags().DurationVar(&watchInterval, "interval", DefaultWatchInterval, "Refresh interval in watch mode (minimum 500ms)")
	cmd.Flags().BoolVarP(&showProgress, "progress", "p", false, "Show progress bars for active tasks")

	parent.AddCommand(cmd)
}

// runStatus executes the status command with production dependencies.
func runStatus(ctx context.Context, cmd *cobra.Command, w io.Writer, watchMode bool, watchInterval time.Duration, showProgress bool) error {
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

	logger := Logger()

	// Create production dependencies
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	wsMgr := workspace.NewManager(wsStore, nil, logger)

	taskStore, err := task.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create task store: %w", err)
	}

	// Handle watch mode
	if watchMode {
		// Validate interval
		if watchInterval < MinWatchInterval {
			return fmt.Errorf("%w: minimum is %v", errors.ErrWatchIntervalTooShort, MinWatchInterval)
		}

		// Watch mode doesn't support JSON output
		if output == OutputJSON {
			return errors.ErrWatchModeJSONUnsupported
		}

		return runWatchMode(ctx, wsMgr, taskStore, watchInterval, quiet, showProgress)
	}

	return runStatusWithDeps(ctx, w, output, quiet, showProgress, wsMgr, taskStore)
}

// runStatusWithDeps executes the status command with injected dependencies.
// This enables testing with mock implementations.
func runStatusWithDeps(
	ctx context.Context,
	w io.Writer,
	output string,
	quiet bool,
	showProgress bool,
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
			// Story 7.9: Use structured JSON format for consistency
			emptyOutput := statusJSONOutput{
				Workspaces: []map[string]string{},
			}
			encoder := json.NewEncoder(w)
			encoder.SetIndent("", "  ")
			return encoder.Encode(emptyOutput)
		}
		_, _ = fmt.Fprintln(w, "No workspaces. Run 'atlas start' to create one.")
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

	return outputStatusTable(w, rows, quiet, showProgress)
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

		// Load tasks directly from store (authoritative source)
		tasks, err := taskStore.List(ctx, ws.Name)
		if err == nil && len(tasks) > 0 {
			mostRecent := tasks[0] // Already sorted newest first
			row.Status = mostRecent.Status
			row.CurrentStep = mostRecent.CurrentStep + 1 // 1-indexed for display
			row.TotalSteps = len(mostRecent.Steps)
			// Extract current step name for display
			if mostRecent.CurrentStep >= 0 && mostRecent.CurrentStep < len(mostRecent.Steps) {
				row.StepName = mostRecent.Steps[mostRecent.CurrentStep].Name
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

// statusJSONOutput is the structured JSON output for the status command.
// Includes workspaces and attention_items per Story 7.9.
type statusJSONOutput struct {
	Workspaces     []map[string]string `json:"workspaces"`
	AttentionItems []map[string]string `json:"attention_items,omitempty"`
}

// outputStatusJSON outputs status as JSON with workspaces and attention items.
func outputStatusJSON(w io.Writer, rows []tui.StatusRow) error {
	table := tui.NewStatusTable(rows)
	headers, data := table.ToJSONData()

	// Convert to array of objects with full field names
	workspaces := make([]map[string]string, len(data))
	for i, row := range data {
		obj := make(map[string]string)
		for j, header := range headers {
			// Use lowercase field names for JSON
			key := toLowerCamelCase(header)
			if j < len(row) {
				obj[key] = row[j]
			}
		}
		workspaces[i] = obj
	}

	// Build attention items from StatusFooter (Story 7.9)
	footer := tui.NewStatusFooter(rows)
	attentionItems := footer.ToJSON()

	// Build output structure
	output := statusJSONOutput{
		Workspaces:     workspaces,
		AttentionItems: attentionItems,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
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
func outputStatusTable(w io.Writer, rows []tui.StatusRow, quiet, showProgress bool) error {
	table := tui.NewStatusTable(rows)

	// Header (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w, tui.RenderHeaderAuto())
		_, _ = fmt.Fprintln(w)
	}

	// Table
	if err := table.Render(w); err != nil {
		return err
	}

	// Progress bars (if enabled)
	if showProgress {
		progressRows := buildProgressRows(rows)
		if len(progressRows) > 0 {
			_, _ = fmt.Fprintln(w)
			pd := tui.NewProgressDashboard(progressRows)
			_ = pd.Render(w)
		}
	}

	// Footer summary (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, buildFooter(rows))
	}

	// Action indicators footer (Story 7.9) - shows copy-paste commands
	// Render even in quiet mode since these are actionable commands
	footer := tui.NewStatusFooter(rows)
	if footer.HasItems() {
		_ = footer.Render(w)
	}

	return nil
}

// buildProgressRows converts status rows to progress rows for the dashboard.
// Only includes rows with active tasks (running or validating states).
// Delegates to shared helper in tui package to avoid code duplication.
func buildProgressRows(rows []tui.StatusRow) []tui.ProgressRow {
	return tui.BuildProgressRowsFromStatus(rows)
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

// runWatchMode starts the watch mode TUI with live updates.
func runWatchMode(ctx context.Context, wsMgr tui.WorkspaceLister, taskStore tui.TaskLister, interval time.Duration, quiet, showProgress bool) error {
	// Load config to get bell preference
	cfg, err := config.Load(ctx)
	bellEnabled := true // Default to enabled
	if err == nil {
		bellEnabled = cfg.Notifications.Bell
	}

	// Create watch config
	watchCfg := tui.WatchConfig{
		Interval:     interval,
		BellEnabled:  bellEnabled,
		Quiet:        quiet,
		ShowProgress: showProgress,
	}

	// Create the watch model with context for proper cancellation propagation
	model := tui.NewWatchModel(ctx, wsMgr, taskStore, watchCfg)

	// Create and run the Bubble Tea program with alternate screen and context
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))

	_, err = p.Run()
	return err
}
