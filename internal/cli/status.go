// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// statusOptions contains all options for the status command.
// Using a struct instead of individual boolean parameters improves readability
// at call sites and makes it easier to add new options.
type statusOptions struct {
	WatchMode     bool
	WatchInterval time.Duration
	ShowProgress  bool
}

// StatusRenderOptions contains display-related options for status rendering.
// Using a struct reduces parameter count and improves readability.
type StatusRenderOptions struct {
	Output       string
	Quiet        bool
	ShowProgress bool
}

// StatusDeps contains dependencies for status command execution.
// Using a struct enables easier testing with mock implementations.
type StatusDeps struct {
	WorkspaceMgr WorkspaceLister
	TaskStore    TaskLister
}

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
			return runStatus(cmd.Context(), cmd, os.Stdout, statusOptions{
				WatchMode:     watchMode,
				WatchInterval: watchInterval,
				ShowProgress:  showProgress,
			})
		},
	}

	// Watch mode flags
	cmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Enable watch mode with live updates")
	cmd.Flags().DurationVar(&watchInterval, "interval", DefaultWatchInterval, "Refresh interval in watch mode (minimum 500ms)")
	cmd.Flags().BoolVarP(&showProgress, "progress", "p", false, "Show progress bars for active tasks")

	parent.AddCommand(cmd)
}

// runStatus executes the status command with production dependencies.
func runStatus(ctx context.Context, cmd *cobra.Command, w io.Writer, opts statusOptions) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return fmt.Errorf("status command canceled: %w", ctx.Err())
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
	if opts.WatchMode {
		// Validate interval
		if opts.WatchInterval < MinWatchInterval {
			return fmt.Errorf("%w: minimum is %v", errors.ErrWatchIntervalTooShort, MinWatchInterval)
		}

		// Watch mode doesn't support JSON output
		if output == OutputJSON {
			return errors.ErrWatchModeJSONUnsupported
		}

		return runWatchMode(ctx, wsMgr, taskStore, opts.WatchInterval, quiet, opts.ShowProgress)
	}

	renderOpts := StatusRenderOptions{
		Output:       output,
		Quiet:        quiet,
		ShowProgress: opts.ShowProgress,
	}
	deps := StatusDeps{
		WorkspaceMgr: wsMgr,
		TaskStore:    taskStore,
	}
	return runStatusWithDeps(ctx, w, renderOpts, deps)
}

// runStatusWithDeps executes the status command with injected dependencies.
// This enables testing with mock implementations.
func runStatusWithDeps(
	ctx context.Context,
	w io.Writer,
	opts StatusRenderOptions,
	deps StatusDeps,
) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return fmt.Errorf("status check canceled: %w", ctx.Err())
	default:
	}

	// Load workspaces
	workspaces, err := deps.WorkspaceMgr.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Handle empty case
	if len(workspaces) == 0 {
		if opts.Output == OutputJSON {
			// Story 7.9: Use structured JSON format for consistency
			emptyOutput := hierarchicalJSONOutput{
				Workspaces: []tui.HierarchicalJSONWorkspace{},
			}
			encoder := json.NewEncoder(w)
			encoder.SetIndent("", "  ")
			return encoder.Encode(emptyOutput)
		}
		_, _ = fmt.Fprintln(w, "No workspaces. Run 'atlas start' to create one.")
		return nil
	}

	// Build hierarchical workspace groups
	groups, err := buildWorkspaceGroups(ctx, workspaces, deps.TaskStore)
	if err != nil {
		return fmt.Errorf("failed to build workspace groups: %w", err)
	}

	// Sort by status priority (attention first)
	sortGroupsByStatusPriority(groups)

	// Output based on format
	if opts.Output == OutputJSON {
		return outputHierarchicalJSON(w, groups)
	}

	return outputHierarchicalTable(w, groups, opts.Quiet, opts.ShowProgress)
}

// buildWorkspaceGroups builds hierarchical workspace groups from workspaces.
// Each workspace includes all its tasks for nested display.
func buildWorkspaceGroups(
	ctx context.Context,
	workspaces []*domain.Workspace,
	taskStore TaskLister,
) ([]tui.WorkspaceGroup, error) {
	groups := make([]tui.WorkspaceGroup, 0, len(workspaces))

	for _, ws := range workspaces {
		// Check for cancellation during iteration
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("building workspace groups canceled: %w", ctx.Err())
		default:
		}

		group := tui.WorkspaceGroup{
			Name:   ws.Name,
			Branch: ws.Branch,
			Status: constants.TaskStatusPending, // Default
		}

		// Load all tasks for the workspace
		tasks, err := taskStore.List(ctx, ws.Name)
		if err == nil && len(tasks) > 0 {
			group.TotalTasks = len(tasks)
			group.Status = tasks[0].Status // Aggregate status from most recent

			// Build task info list
			group.Tasks = make([]tui.TaskInfo, len(tasks))
			for i, t := range tasks {
				group.Tasks[i] = tui.TaskInfo{
					ID:          t.ID,
					Path:        taskPath(ws.Name, t.ID),
					Template:    t.TemplateID,
					Status:      t.Status,
					CurrentStep: t.CurrentStep + 1, // 1-indexed for display
					TotalSteps:  len(t.Steps),
				}
			}
		}

		groups = append(groups, group)
	}

	return groups, nil
}

// taskPath computes the full file system path to a task directory.
// Used for generating clickable hyperlinks in terminals that support OSC 8.
// Task data is stored in ~/.atlas/, not the project directory.
func taskPath(workspaceName, taskID string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".atlas", constants.WorkspacesDir, workspaceName, constants.TasksDir, taskID)
}

// sortByStatusPriority sorts rows by status priority (attention first, then running).
func sortByStatusPriority(rows []tui.StatusRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		return statusPriority(rows[i].Status) > statusPriority(rows[j].Status)
	})
}

// sortGroupsByStatusPriority sorts workspace groups by status priority.
func sortGroupsByStatusPriority(groups []tui.WorkspaceGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		return statusPriority(groups[i].Status) > statusPriority(groups[j].Status)
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

// hierarchicalJSONOutput is the structured JSON output with nested tasks.
type hierarchicalJSONOutput struct {
	Workspaces     []tui.HierarchicalJSONWorkspace `json:"workspaces"`
	AttentionItems []attentionItem                 `json:"attention_items,omitempty"`
}

// attentionItem represents an item that needs user attention.
type attentionItem struct {
	Workspace string `json:"workspace"`
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Action    string `json:"action"`
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

// outputHierarchicalJSON outputs status as hierarchical JSON with nested tasks.
func outputHierarchicalJSON(w io.Writer, groups []tui.WorkspaceGroup) error {
	table := tui.NewHierarchicalStatusTable(groups)

	// Build attention items
	var attention []attentionItem
	for _, group := range groups {
		for _, task := range group.Tasks {
			if tui.IsAttentionStatus(task.Status) {
				action := tui.SuggestedAction(task.Status)
				if action != "" {
					attention = append(attention, attentionItem{
						Workspace: group.Name,
						TaskID:    task.ID,
						Status:    string(task.Status),
						Action:    fmt.Sprintf("%s %s", action, group.Name),
					})
				}
			}
		}
	}

	output := hierarchicalJSONOutput{
		Workspaces:     table.ToJSONData(),
		AttentionItems: attention,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// outputHierarchicalTable outputs status as hierarchical table with nested tasks.
func outputHierarchicalTable(w io.Writer, groups []tui.WorkspaceGroup, quiet, showProgress bool) error {
	table := tui.NewHierarchicalStatusTable(groups)

	// Header (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w, tui.RenderHeaderAuto())
		_, _ = fmt.Fprintln(w)
	}

	// Hierarchical table
	if err := table.Render(w); err != nil {
		return fmt.Errorf("render hierarchical table: %w", err)
	}

	// Progress bars (if enabled)
	if showProgress {
		progressRows := buildProgressRowsFromGroups(groups)
		if len(progressRows) > 0 {
			_, _ = fmt.Fprintln(w)
			pd := tui.NewProgressDashboard(progressRows)
			_ = pd.Render(w)
		}
	}

	// Footer summary (unless quiet)
	if !quiet {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, buildHierarchicalFooter(groups))
	}

	return nil
}

// buildProgressRowsFromGroups converts workspace groups to progress rows.
func buildProgressRowsFromGroups(groups []tui.WorkspaceGroup) []tui.ProgressRow {
	var progressRows []tui.ProgressRow
	for _, group := range groups {
		for _, task := range group.Tasks {
			if task.Status == constants.TaskStatusRunning || task.Status == constants.TaskStatusValidating {
				percent := 0.0
				if task.TotalSteps > 0 {
					percent = float64(task.CurrentStep) / float64(task.TotalSteps)
				}
				progressRows = append(progressRows, tui.ProgressRow{
					Name:        fmt.Sprintf("%s/%s", group.Name, task.ID),
					Percent:     percent,
					CurrentStep: task.CurrentStep,
					TotalSteps:  task.TotalSteps,
					StepName:    task.Template,
				})
			}
		}
	}
	return progressRows
}

// buildHierarchicalFooter creates the footer summary for hierarchical display.
func buildHierarchicalFooter(groups []tui.WorkspaceGroup) string {
	attentionCount := 0
	var firstAttention *tui.WorkspaceGroup

	for i := range groups {
		if tui.IsAttentionStatus(groups[i].Status) {
			attentionCount++
			if firstAttention == nil {
				firstAttention = &groups[i]
			}
		}
	}

	// Count total tasks
	totalTasks := 0
	for _, group := range groups {
		totalTasks += group.TotalTasks
	}

	// Summary line
	workspaceWord := "workspaces"
	if len(groups) == 1 {
		workspaceWord = "workspace"
	}
	taskWord := "tasks"
	if totalTasks == 1 {
		taskWord = "task"
	}
	summary := fmt.Sprintf("%d %s, %d %s", len(groups), workspaceWord, totalTasks, taskWord)

	if attentionCount > 0 {
		needWord := "need"
		if attentionCount == 1 {
			needWord = "needs"
		}
		summary += fmt.Sprintf(", %d %s attention", attentionCount, needWord)
	}

	// Actionable command
	if firstAttention != nil {
		action := tui.SuggestedAction(firstAttention.Status)
		if action != "" {
			summary += fmt.Sprintf("\nRun: %s %s", action, firstAttention.Name)
		}
	}

	return summary
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
	return fmt.Sprintf("\nRun: %s %s", action, row.Workspace)
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
	if err != nil {
		return fmt.Errorf("run watch mode: %w", err)
	}
	return nil
}
