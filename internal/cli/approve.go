// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// AddApproveCommand adds the approve command to the root command.
func AddApproveCommand(root *cobra.Command) {
	root.AddCommand(newApproveCmd())
}

// approveOptions contains all options for the approve command.
type approveOptions struct {
	workspace    string // Optional workspace name
	autoApprove  bool   // Skip interactive menu and approve directly
	closeWS      bool   // Also close the workspace after approval
	mergeMessage string // Custom message for approve+merge operations
}

// newApproveCmd creates the approve command.
func newApproveCmd() *cobra.Command {
	var autoApprove bool
	var closeWS bool
	var mergeMessage string

	cmd := &cobra.Command{
		Use:   "approve [workspace]",
		Short: "Approve a completed task",
		Long: `Approve a task that has passed validation and is awaiting approval.

If multiple tasks are awaiting approval, you'll be prompted to select one.
You can also specify a workspace name directly to skip the selection.

The approval flow shows a summary of the task and provides options to:
  - Approve and complete the task
  - Approve and close the workspace (removes worktree, preserves history)
  - Approve, merge PR, and close workspace (all in one)
  - View the git diff of changes
  - View task execution logs
  - Open the PR in your browser
  - Reject the task (redirects to atlas reject)
  - Cancel and return

Non-interactive mode:
  Use --auto-approve to skip the interactive menu and approve directly.
  In non-interactive environments (pipes, CI), --auto-approve is required.

Examples:
  atlas approve              # Interactive selection if multiple tasks
  atlas approve my-feature   # Approve task in my-feature workspace
  atlas approve my-feature --auto-approve  # Approve directly without menu
  atlas approve my-feature --close         # Approve and close workspace
  atlas approve my-feature --message "Merged by CI"  # Custom merge message
  atlas approve -o json my-feature  # Output result as JSON`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := approveOptions{
				autoApprove:  autoApprove,
				closeWS:      closeWS,
				mergeMessage: mergeMessage,
			}
			if len(args) > 0 {
				opts.workspace = args[0]
			}
			return runApprove(cmd.Context(), cmd, os.Stdout, opts)
		},
	}

	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip interactive menu and approve directly")
	cmd.Flags().BoolVar(&closeWS, "close", false, "Also close the workspace after approval (removes worktree, preserves history)")
	cmd.Flags().StringVar(&mergeMessage, "message", "", "Custom message for approve+merge (overrides config)")

	return cmd
}

// approveResponse represents the JSON output for approve operations.
type approveResponse struct {
	Success         bool          `json:"success"`
	Workspace       workspaceInfo `json:"workspace"`
	Task            taskInfo      `json:"task"`
	PRURL           string        `json:"pr_url,omitempty"`
	WorkspaceClosed bool          `json:"workspace_closed,omitempty"`
	Warning         string        `json:"warning,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// approvalAction represents actions available in the approval menu.
type approvalAction string

const (
	actionApprove           approvalAction = "approve"
	actionApproveAndClose   approvalAction = "approve_and_close"
	actionApproveMergeClose approvalAction = "approve_merge_close"
	actionViewDiff          approvalAction = "view_diff"
	actionViewLogs          approvalAction = "view_logs"
	actionOpenPR            approvalAction = "open_pr"
	actionReject            approvalAction = "reject"
	actionCancel            approvalAction = "cancel"
)

// approveStep represents a step in the approval workflow
type approveStep struct {
	name    string          // Display name (e.g., "Add PR Review")
	execute approveStepFunc // Function to execute
}

// approveStepFunc executes a step and returns an optional message
type approveStepFunc func(ctx context.Context, stepCtx *approveStepContext) (message string, err error)

// approveStepContext contains context needed for executing approval steps
type approveStepContext struct {
	out       tui.Output
	taskStore task.Store
	ws        *domain.Workspace
	t         *domain.Task
	notifier  *tui.Notifier
	hubRunner git.HubRunner
	message   string // PR merge message
}

// approveStepTracker manages step execution and progress display
type approveStepTracker struct {
	steps        []approveStep
	currentStep  int
	totalSteps   int
	out          tui.Output
	outputFormat string // Skip progress in JSON mode
}

// Injection points for testing - these can be overridden in tests
//
//nolint:gochecknoglobals // Test injection points - standard Go testing pattern
var (
	// tuiSelectFunc allows injecting tui.Select for testing
	tuiSelectFunc = tui.Select

	// selectApprovalActionFunc allows injecting selectApprovalAction for testing
	selectApprovalActionFunc = selectApprovalAction

	// execCommandContextFunc allows injecting exec.CommandContext for testing
	execCommandContextFunc = exec.CommandContext
)

// runApprove executes the approve command.
func runApprove(ctx context.Context, cmd *cobra.Command, w io.Writer, opts approveOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := Logger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// Detect non-interactive mode
	isNonInteractive := outputFormat == OutputJSON || !term.IsTerminal(int(os.Stdin.Fd()))

	// JSON mode requires workspace argument (no interactive selection)
	if outputFormat == OutputJSON && opts.workspace == "" {
		return handleApproveError(outputFormat, w, "", fmt.Errorf("workspace argument required with --output json: %w", atlaserrors.ErrInvalidArgument))
	}

	// Non-interactive mode requires --auto-approve or JSON output
	if isNonInteractive && !opts.autoApprove && outputFormat != OutputJSON {
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("use --auto-approve in non-interactive mode: %w", atlaserrors.ErrApprovalRequired))
	}

	// Create stores and find awaiting tasks
	selectedWS, selectedTask, err := findAndSelectTask(ctx, outputFormat, w, out, opts, isNonInteractive)
	if err != nil {
		return fmt.Errorf("find awaiting task: %w", err)
	}
	if selectedWS == nil {
		// No tasks awaiting approval (message already shown)
		return nil
	}

	logger.Debug().
		Str("workspace_name", selectedWS.Name).
		Str("task_id", selectedTask.ID).
		Str("status", string(selectedTask.Status)).
		Msg("selected task for approval")

	// Create task store for updates
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return handleApproveError(outputFormat, w, "", fmt.Errorf("failed to create task store: %w", err))
	}

	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create notifier
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// JSON mode: approve directly without interactive menu
	if outputFormat == OutputJSON {
		return approveAndOutputJSON(ctx, w, taskStore, selectedWS, selectedTask, opts.closeWS)
	}

	// Auto-approve mode: approve directly without interactive menu
	if opts.autoApprove {
		return runAutoApprove(ctx, out, taskStore, selectedWS, selectedTask, notifier, opts.closeWS)
	}

	// Get verbose flag from global flags
	verbose := cmd.Flag("verbose").Value.String() == "true"

	// Interactive approval flow
	return runInteractiveApproval(ctx, out, taskStore, selectedWS, selectedTask, notifier, verbose)
}

// findAndSelectTask finds awaiting tasks and selects one based on options.
func findAndSelectTask(ctx context.Context, outputFormat string, w io.Writer, out tui.Output, opts approveOptions, isNonInteractive bool) (*domain.Workspace, *domain.Task, error) {
	// Create stores
	wsStore, taskStore, err := CreateStores("")
	if err != nil {
		return nil, nil, handleApproveError(outputFormat, w, "", err)
	}

	// Find tasks awaiting approval
	awaitingTasks, err := findAwaitingApprovalTasks(ctx, wsStore, taskStore)
	if err != nil {
		return nil, nil, handleApproveError(outputFormat, w, "", err)
	}

	// Handle case where no tasks are awaiting approval
	if len(awaitingTasks) == 0 {
		if outputFormat == OutputJSON {
			return nil, nil, handleApproveError(outputFormat, w, "", atlaserrors.ErrNoTasksFound)
		}
		out.Info("No tasks awaiting approval.")
		out.Info("Run 'atlas status' to see all workspace statuses.")
		return nil, nil, nil
	}

	// Select the appropriate task
	return selectApprovalTask(outputFormat, w, out, opts, awaitingTasks, isNonInteractive)
}

// selectApprovalTask selects a task from the awaiting tasks based on options.
func selectApprovalTask(outputFormat string, w io.Writer, out tui.Output, opts approveOptions, awaitingTasks []awaitingTask, isNonInteractive bool) (*domain.Workspace, *domain.Task, error) {
	// If workspace provided, find it directly
	if opts.workspace != "" {
		for _, at := range awaitingTasks {
			if at.workspace.Name == opts.workspace {
				return at.workspace, at.task, nil
			}
		}
		return nil, nil, handleApproveError(outputFormat, w, opts.workspace, fmt.Errorf("workspace '%s' not found or not awaiting approval: %w", opts.workspace, atlaserrors.ErrWorkspaceNotFound))
	}

	// Auto-select if only one task
	if len(awaitingTasks) == 1 {
		return awaitingTasks[0].workspace, awaitingTasks[0].task, nil
	}

	// Non-interactive mode requires workspace argument when multiple tasks exist
	if isNonInteractive {
		return nil, nil, atlaserrors.NewExitCode2Error(
			fmt.Errorf("multiple tasks awaiting approval, use workspace argument to specify: %w", atlaserrors.ErrInteractiveRequired))
	}

	// Present selection menu (AC: #1)
	selected, err := selectWorkspaceForApproval(awaitingTasks)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Approval canceled.")
			return nil, nil, nil
		}
		return nil, nil, handleApproveError(outputFormat, w, "", err)
	}

	return selected.workspace, selected.task, nil
}

// awaitingTask holds a workspace and its task awaiting approval.
type awaitingTask struct {
	workspace *domain.Workspace
	task      *domain.Task
}

// GetWorkspaceName returns the workspace name for menu selection.
func (a awaitingTask) GetWorkspaceName() string {
	return a.workspace.Name
}

// GetTaskDescription returns the task description for menu selection.
func (a awaitingTask) GetTaskDescription() string {
	return a.task.Description
}

// findAwaitingApprovalTasks finds all tasks with awaiting_approval status.
func findAwaitingApprovalTasks(ctx context.Context, wsStore workspace.Store, taskStore task.Store) ([]awaitingTask, error) {
	// List all workspaces
	workspaces, err := wsStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var result []awaitingTask

	for _, ws := range workspaces {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// List tasks for this workspace
		tasks, err := taskStore.List(ctx, ws.Name)
		if err != nil {
			// Skip workspaces with no tasks or errors
			continue
		}

		// Find the latest task with awaiting_approval status
		for _, t := range tasks {
			if t.Status == constants.TaskStatusAwaitingApproval {
				result = append(result, awaitingTask{
					workspace: ws,
					task:      t,
				})
				break // Only count the latest task per workspace
			}
		}
	}

	return result, nil
}

// selectWorkspaceForApproval presents a selection menu for multiple awaiting tasks.
func selectWorkspaceForApproval(tasks []awaitingTask) (*awaitingTask, error) {
	idx, err := SelectWorkspaceTask("Select a workspace to approve:", tasks)
	if err != nil {
		return nil, err
	}
	return &tasks[idx], nil
}

// runAutoApprove performs automatic approval without interactive menu.
func runAutoApprove(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, closeWS bool) error {
	// Build step workflow based on whether we're closing workspace
	var steps []approveStep
	if closeWS {
		steps = buildApproveAndCloseSteps()
	} else {
		steps = buildSimpleApproveSteps()
	}

	// Create step context
	stepCtx := &approveStepContext{
		out:       out,
		taskStore: taskStore,
		ws:        ws,
		t:         t,
		notifier:  notifier,
	}

	// Auto-approve mode uses TTY output (not JSON)
	tracker := newApproveStepTracker(steps, out, "")
	if err := tracker.executeSteps(ctx, stepCtx); err != nil {
		return fmt.Errorf("failed to approve task: %w", err)
	}

	// Display summary info
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Task ID:   %s", t.ID))
	if prURL := extractPRURL(t); prURL != "" {
		out.Info(fmt.Sprintf("  PR URL:    %s", prURL))
	}

	notifier.Bell()
	return nil
}

// findCurrentStepResult finds the most recent StepResult for the current step.
// Returns nil if no result exists for the current step.
// This function searches by StepIndex field rather than array position because
// StepResults is a history list where array index may not match step index
// (e.g., after interruptions/resumes or step retries).
func findCurrentStepResult(t *domain.Task) *domain.StepResult {
	if t == nil {
		return nil
	}
	// Search backwards to find most recent result for current step
	for i := len(t.StepResults) - 1; i >= 0; i-- {
		if t.StepResults[i].StepIndex == t.CurrentStep {
			return &t.StepResults[i]
		}
	}
	return nil
}

// hasStepLevelApproval checks if the current step has approval options.
// This indicates the step requires user input before proceeding (e.g., garbage file handling).
func hasStepLevelApproval(t *domain.Task) bool {
	result := findCurrentStepResult(t)
	return result != nil && len(result.ApprovalOptions) > 0
}

// runInteractiveApproval runs the interactive approval flow with action menu.
func runInteractiveApproval(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, verbose bool) error {
	// Check for step-level approval first (e.g., garbage file handling)
	if hasStepLevelApproval(t) {
		return runStepLevelApproval(ctx, out, taskStore, ws, t, notifier)
	}

	// Display approval summary (AC: #2)
	summary := tui.NewApprovalSummary(t, ws)
	_ = out // Mark out as used - printApprovalSummary writes to stdout directly for styled output
	printApprovalSummary(summary, verbose)

	// Action menu loop (AC: #3, #4)
	return runApprovalActionLoop(ctx, out, taskStore, ws, t, notifier)
}

// runStepLevelApproval handles approval for step-specific choices (e.g., garbage file handling).
// It displays the step output and shows a menu with the step's approval options.
// After the user makes a choice, it automatically resumes the task.
func runStepLevelApproval(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	stepResult := findCurrentStepResult(t)
	if stepResult == nil {
		return fmt.Errorf("no result found for current step %d: %w", t.CurrentStep, atlaserrors.ErrUnknownStepResultStatus)
	}

	// Display step output (the garbage warning or other step-specific message)
	out.Warning(stepResult.Output)

	// Build menu from step options
	options := make([]tui.Option, len(stepResult.ApprovalOptions))
	for i, opt := range stepResult.ApprovalOptions {
		label := opt.Label
		if opt.Recommended {
			label += " (recommended)"
		}
		options[i] = tui.Option{Label: label, Description: opt.Description, Value: opt.Key}
	}

	selected, err := tuiSelectFunc("How would you like to proceed?", options)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Approval canceled.")
			return nil
		}
		return fmt.Errorf("select step action: %w", err)
	}

	// Store choice in task metadata for Resume to use
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	t.Metadata["step_approval_choice"] = selected

	// Save the task with the choice
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return fmt.Errorf("failed to save approval choice: %w", err)
	}

	out.Success(fmt.Sprintf("Choice '%s' saved.", selected))

	// Inform user to resume the task
	out.Info(fmt.Sprintf("Run 'atlas resume %s' to continue the task with your choice.", ws.Name))
	notifier.Bell()
	return nil
}

// printApprovalSummaryTo prints the approval summary to the specified writer.
// This function is testable by injecting a custom writer.
func printApprovalSummaryTo(w io.Writer, summary *tui.ApprovalSummary, verbose bool) {
	rendered := tui.RenderApprovalSummaryWithWidth(summary, 0, verbose)
	_, _ = w.Write([]byte(rendered + "\n"))
}

// printApprovalSummary prints the approval summary to stdout.
// This is a convenience wrapper around printApprovalSummaryTo for production use.
func printApprovalSummary(summary *tui.ApprovalSummary, verbose bool) {
	printApprovalSummaryTo(os.Stdout, summary, verbose)
}

// runApprovalActionLoop handles the approval action menu loop.
func runApprovalActionLoop(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	for {
		action, err := selectApprovalActionFunc(t)
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Approval canceled.")
				return nil
			}
			return fmt.Errorf("select approval action: %w", err)
		}

		done, err := executeApprovalAction(ctx, out, taskStore, ws, t, notifier, action)
		if err != nil {
			return fmt.Errorf("execute approval action: %w", err)
		}
		if done {
			return nil
		}
		// Continue loop for view actions
	}
}

// executeApprovalAction executes the selected approval action.
// Returns true if the action loop should exit.
//
//nolint:unparam // error return is kept for future extensibility and consistent interface
func executeApprovalAction(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, action approvalAction) (bool, error) {
	switch action {
	case actionApprove:
		// Build single-step workflow
		steps := buildSimpleApproveSteps()
		stepCtx := &approveStepContext{
			out:       out,
			taskStore: taskStore,
			ws:        ws,
			t:         t,
			notifier:  notifier,
		}
		// Interactive mode always uses TTY output (not JSON)
		tracker := newApproveStepTracker(steps, out, "")
		if err := tracker.executeSteps(ctx, stepCtx); err != nil {
			out.Error(tui.WrapWithSuggestion(err))
			return false, nil // Continue loop on error
		}
		out.Info("PR ready for merge.")
		notifier.Bell()
		return true, nil

	case actionApproveAndClose:
		// Build two-step workflow
		steps := buildApproveAndCloseSteps()
		stepCtx := &approveStepContext{
			out:       out,
			taskStore: taskStore,
			ws:        ws,
			t:         t,
			notifier:  notifier,
		}
		// Interactive mode always uses TTY output (not JSON)
		tracker := newApproveStepTracker(steps, out, "")
		if err := tracker.executeSteps(ctx, stepCtx); err != nil {
			out.Error(tui.WrapWithSuggestion(err))
			return false, nil // Continue loop on error
		}
		out.Info("PR ready for merge.")
		notifier.Bell()
		return true, nil

	case actionApproveMergeClose:
		// Load config to get the default merge message
		cfg, _ := config.Load(ctx)
		message := cfg.Approval.MergeMessage
		// Interactive mode always uses TTY output (not JSON)
		if err := executeApproveMergeClose(ctx, out, taskStore, ws, t, notifier, message, ""); err != nil {
			//nolint:nilerr // Error already displayed to user; continue interactive loop
			return false, nil
		}
		return true, nil

	case actionViewDiff:
		if err := viewDiff(ctx, ws.WorktreePath); err != nil {
			out.Warning(fmt.Sprintf("Could not display diff: %v", err))
		}
		return false, nil

	case actionViewLogs:
		if err := viewLogs(ctx, taskStore, ws.Name, t.ID); err != nil {
			out.Warning(fmt.Sprintf("Could not display logs: %v", err))
		}
		return false, nil

	case actionOpenPR:
		prURL := extractPRURL(t)
		if prURL == "" {
			out.Warning("No PR URL available.")
			return false, nil
		}
		if err := openInBrowser(ctx, prURL); err != nil {
			out.Warning(fmt.Sprintf("Could not open PR: %v", err))
			return false, nil
		}
		out.Info(fmt.Sprintf("Opened %s in browser.", prURL))
		return false, nil

	case actionReject:
		out.Info(fmt.Sprintf("Run 'atlas reject %s' to reject with feedback.", ws.Name))
		return true, nil

	case actionCancel:
		out.Info("Approval canceled.")
		return true, nil
	}

	return false, nil
}

// selectApprovalAction presents the action menu.
func selectApprovalAction(t *domain.Task) (approvalAction, error) {
	options := []tui.Option{
		{Label: "Approve and complete", Description: "Mark task as completed", Value: string(actionApprove)},
		{Label: "Approve and close workspace", Description: "Mark completed and remove worktree", Value: string(actionApproveAndClose)},
	}

	// Only show merge option if PR exists
	if prNumber := extractPRNumber(t); prNumber > 0 {
		options = append(options, tui.Option{
			Label:       "Approve + Merge + Close",
			Description: "Review PR, squash merge, close workspace",
			Value:       string(actionApproveMergeClose),
		})
	}

	options = append(options,
		tui.Option{Label: "View diff", Description: "Show file changes", Value: string(actionViewDiff)},
		tui.Option{Label: "View logs", Description: "Show task execution log", Value: string(actionViewLogs)},
	)

	// Only show Open PR if URL is available
	if prURL := extractPRURL(t); prURL != "" {
		options = append(options, tui.Option{
			Label:       "Open PR in browser",
			Description: "View pull request",
			Value:       string(actionOpenPR),
		})
	}

	options = append(options,
		tui.Option{Label: "Reject", Description: "Run atlas reject for feedback", Value: string(actionReject)},
		tui.Option{Label: "Cancel", Description: "Return without action", Value: string(actionCancel)},
	)

	selected, err := tuiSelectFunc("What would you like to do?", options)
	if err != nil {
		return "", err
	}

	return approvalAction(selected), nil
}

// approveTask transitions the task to completed status.
func approveTask(ctx context.Context, taskStore task.Store, t *domain.Task) error {
	// Transition task to completed (AC: #4)
	if err := task.Transition(ctx, t, constants.TaskStatusCompleted, "User approved"); err != nil {
		return fmt.Errorf("failed to approve task: %w", err)
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return fmt.Errorf("failed to save task: %w", err)
	}

	return nil
}

// viewDiff displays the git diff in a pager.
func viewDiff(ctx context.Context, worktreePath string) error {
	if worktreePath == "" {
		return fmt.Errorf("failed to view diff: %w", atlaserrors.ErrEmptyValue)
	}

	// Get diff of recent changes
	gitCmd := execCommandContextFunc(ctx, "git", "-C", worktreePath, "diff", "HEAD~1")
	gitOutput, err := gitCmd.Output()
	if err != nil {
		// Try without HEAD~1 for new repos
		gitCmd = execCommandContextFunc(ctx, "git", "-C", worktreePath, "diff")
		gitOutput, err = gitCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get diff: %w", err)
		}
	}

	if len(gitOutput) == 0 {
		_, _ = os.Stdout.WriteString("No changes to display.\n")
		return nil
	}

	// Pipe to less with color support
	return pipeToLess(ctx, gitOutput)
}

// viewLogs displays the task log in a pager.
func viewLogs(ctx context.Context, taskStore task.Store, workspaceName, taskID string) error {
	logData, err := taskStore.ReadLog(ctx, workspaceName, taskID)
	if err != nil {
		return fmt.Errorf("no log file found: %w", err)
	}

	if len(logData) == 0 {
		_, _ = os.Stdout.WriteString("Log file is empty.\n")
		return nil
	}

	// Pipe to less
	return pipeToLess(ctx, logData)
}

// pipeToLess pipes data to the less pager.
func pipeToLess(ctx context.Context, data []byte) error {
	lessCmd := execCommandContextFunc(ctx, "less", "-R")
	lessCmd.Stdin = os.Stdin
	lessCmd.Stdout = os.Stdout
	lessCmd.Stderr = os.Stderr

	stdin, pipeErr := lessCmd.StdinPipe()
	if pipeErr != nil {
		// Fallback: print directly (intentionally ignoring pipeErr as we're falling back)
		_, _ = os.Stdout.Write(data)
		return nil //nolint:nilerr // Fallback behavior intentional - print directly when pager unavailable
	}

	if startErr := lessCmd.Start(); startErr != nil {
		// Fallback: print directly (intentionally ignoring startErr as we're falling back)
		_, _ = os.Stdout.Write(data)
		return nil //nolint:nilerr // Fallback behavior intentional - print directly when pager unavailable
	}

	_, _ = stdin.Write(data)
	_ = stdin.Close()

	return lessCmd.Wait()
}

// extractPRURL extracts the PR URL from task metadata.
func extractPRURL(t *domain.Task) string {
	if t == nil || t.Metadata == nil {
		return ""
	}
	if prURL, ok := t.Metadata["pr_url"].(string); ok {
		return prURL
	}
	return ""
}

// extractPRNumber extracts the PR number from task metadata.
func extractPRNumber(t *domain.Task) int {
	if t == nil || t.Metadata == nil {
		return 0
	}

	prNumber, ok := t.Metadata["pr_number"]
	if !ok {
		return 0
	}

	switch v := prNumber.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// createHubRunnerFunc creates a GitHub runner for the given work directory.
// This is a variable to allow test injection.
//
//nolint:gochecknoglobals // Test injection point - standard Go testing pattern
var createHubRunnerFunc = func(workDir string) git.HubRunner {
	return git.NewCLIGitHubRunner(workDir)
}

// executeApproveMergeClose performs the approve+merge+close workflow with step tracking.
// 1. Adds PR review (APPROVE) or falls back to comment
// 2. Merges PR with squash (falls back to admin bypass if needed)
// 3. Approves task
// 4. Closes workspace
func executeApproveMergeClose(
	ctx context.Context,
	out tui.Output,
	taskStore task.Store,
	ws *domain.Workspace,
	t *domain.Task,
	notifier *tui.Notifier,
	message string,
	outputFormat string,
) error {
	// Validate PR exists
	prNumber := extractPRNumber(t)
	if prNumber == 0 {
		err := fmt.Errorf("no PR number found in task metadata: %w", atlaserrors.ErrEmptyValue)
		out.Error(err)
		return err
	}

	// Get the worktree path for GitHub runner
	hubRunner := createHubRunnerFunc(ws.WorktreePath)

	// Build step definitions
	steps := buildApproveMergeCloseSteps()

	// Create step context
	stepCtx := &approveStepContext{
		out:       out,
		taskStore: taskStore,
		ws:        ws,
		t:         t,
		notifier:  notifier,
		hubRunner: hubRunner,
		message:   message,
	}

	// Create tracker and execute steps
	tracker := newApproveStepTracker(steps, out, outputFormat)
	if err := tracker.executeSteps(ctx, stepCtx); err != nil {
		return fmt.Errorf("execute approval steps: %w", err)
	}

	// Ring bell on success
	notifier.Bell()
	return nil
}

// openInBrowser opens a URL in the default browser (macOS).
func openInBrowser(ctx context.Context, url string) error {
	cmd := execCommandContextFunc(ctx, "open", url)
	return cmd.Run()
}

// approveAndOutputJSON approves the task and outputs JSON result.
func approveAndOutputJSON(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task, closeWS bool) error {
	// Approve the task
	if err := task.Transition(ctx, t, constants.TaskStatusCompleted, "User approved via JSON"); err != nil {
		return outputApproveErrorJSON(w, ws.Name, t.ID, err.Error())
	}

	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputApproveErrorJSON(w, ws.Name, t.ID, err.Error())
	}

	// Complete linked backlog discovery if present
	completeLinkedDiscovery(ctx, t)

	// Close workspace if requested
	workspaceClosed := false
	var closeWarning string
	if closeWS {
		warning, err := closeWorkspace(ctx, ws.Name)
		if err == nil {
			workspaceClosed = true
			closeWarning = warning
		}
		// We don't fail the approval if workspace close fails
	}

	// Output success JSON
	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         ws.Name,
			Branch:       ws.Branch,
			WorktreePath: ws.WorktreePath,
			Status:       string(ws.Status),
		},
		Task: taskInfo{
			ID:           t.ID,
			TemplateName: t.TemplateID,
			Description:  t.Description,
			Status:       string(t.Status),
			CurrentStep:  t.CurrentStep,
			TotalSteps:   len(t.Steps),
		},
		PRURL:           extractPRURL(t),
		WorkspaceClosed: workspaceClosed,
		Warning:         closeWarning,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// handleApproveError handles errors based on output format.
func handleApproveError(format string, w io.Writer, workspaceName string, err error) error {
	return HandleCommandError(format, w, approveResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Error: err.Error(),
	}, err)
}

// outputApproveErrorJSON outputs an error result as JSON.
func outputApproveErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	return HandleCommandError(OutputJSON, w, approveResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Task: taskInfo{
			ID: taskID,
		},
		Error: errMsg,
	}, atlaserrors.ErrJSONErrorOutput)
}

// newApproveStepTracker creates a new step tracker
func newApproveStepTracker(steps []approveStep, out tui.Output, outputFormat string) *approveStepTracker {
	return &approveStepTracker{
		steps:        steps,
		currentStep:  0,
		totalSteps:   len(steps),
		out:          out,
		outputFormat: outputFormat,
	}
}

// executeSteps runs all steps in sequence with progress tracking
func (t *approveStepTracker) executeSteps(ctx context.Context, stepCtx *approveStepContext) error {
	for i, step := range t.steps {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		t.currentStep = i

		// Notify step start
		t.notifyStepStart(step)

		// Execute step
		message, err := step.execute(ctx, stepCtx)
		if err != nil {
			t.notifyStepFailed(step, err)
			return err
		}

		// Notify step complete
		t.notifyStepComplete(step, message)
	}

	return nil
}

// notifyStepStart displays the step start message (skip in JSON mode)
func (t *approveStepTracker) notifyStepStart(step approveStep) {
	if t.outputFormat == OutputJSON {
		return
	}

	msg := fmt.Sprintf("Step %d/%d: %s...", t.currentStep+1, t.totalSteps, step.name)
	t.out.Info(msg)
}

// notifyStepComplete displays the step completion message (skip in JSON mode)
func (t *approveStepTracker) notifyStepComplete(step approveStep, message string) {
	if t.outputFormat == OutputJSON {
		return
	}

	msg := fmt.Sprintf("Step %d/%d: %s completed", t.currentStep+1, t.totalSteps, step.name)
	t.out.Success(msg)

	// Display additional message if provided
	if message != "" {
		t.out.Info(fmt.Sprintf("  %s", message))
	}
}

// notifyStepFailed displays the step failure message (skip in JSON mode)
func (t *approveStepTracker) notifyStepFailed(step approveStep, err error) {
	if t.outputFormat == OutputJSON {
		return
	}

	msg := fmt.Sprintf("Step %d/%d: %s failed", t.currentStep+1, t.totalSteps, step.name)
	t.out.Error(fmt.Errorf("%s: %w", msg, err))
}

// buildApproveMergeCloseSteps creates the step definitions for approve+merge+close workflow
func buildApproveMergeCloseSteps() []approveStep {
	return []approveStep{
		{
			name:    "Add PR Review",
			execute: executeAddReviewStep,
		},
		{
			name:    "Merge PR",
			execute: executeMergePRStep,
		},
		{
			name:    "Approve Task",
			execute: executeApproveTaskStep,
		},
		{
			name:    "Close Workspace",
			execute: executeCloseWorkspaceStep,
		},
	}
}

// buildApproveAndCloseSteps creates the step definitions for approve+close workflow
func buildApproveAndCloseSteps() []approveStep {
	return []approveStep{
		{
			name:    "Approve Task",
			execute: executeApproveTaskStep,
		},
		{
			name:    "Close Workspace",
			execute: executeCloseWorkspaceStep,
		},
	}
}

// buildSimpleApproveSteps creates the step definitions for simple approve workflow
func buildSimpleApproveSteps() []approveStep {
	return []approveStep{
		{
			name:    "Approve Task",
			execute: executeApproveTaskStep,
		},
	}
}

// executeAddReviewStep handles adding PR review or comment with fallback
func executeAddReviewStep(ctx context.Context, stepCtx *approveStepContext) (string, error) {
	prNumber := extractPRNumber(stepCtx.t)
	if prNumber == 0 {
		return "", fmt.Errorf("no PR number found in task metadata: %w", atlaserrors.ErrEmptyValue)
	}

	// Try to add review first
	reviewErr := stepCtx.hubRunner.AddPRReview(ctx, prNumber, stepCtx.message, "APPROVE")
	if reviewErr == nil {
		return "PR approved", nil
	}

	// Fallback to comment if review not allowed (own PR)
	if errors.Is(reviewErr, atlaserrors.ErrPRReviewNotAllowed) {
		if commentErr := stepCtx.hubRunner.AddPRComment(ctx, prNumber, stepCtx.message); commentErr != nil {
			return "", fmt.Errorf("could not add comment: %w", commentErr)
		}
		return "Comment added (own PR)", nil
	}

	// Try comment fallback for other errors too
	if commentErr := stepCtx.hubRunner.AddPRComment(ctx, prNumber, stepCtx.message); commentErr != nil {
		return "", fmt.Errorf("could not add review or comment: %w", reviewErr)
	}
	return "Comment added (review failed)", nil
}

// executeMergePRStep handles merging PR with admin bypass fallback
func executeMergePRStep(ctx context.Context, stepCtx *approveStepContext) (string, error) {
	prNumber := extractPRNumber(stepCtx.t)

	// Try standard merge first (deleteBranch=false to preserve branch for workspace cleanup)
	mergeErr := stepCtx.hubRunner.MergePR(ctx, prNumber, "squash", false, false)
	if mergeErr == nil {
		return "PR merged (squash)", nil
	}

	// Try with admin bypass
	mergeErr = stepCtx.hubRunner.MergePR(ctx, prNumber, "squash", true, false)
	if mergeErr != nil {
		return "", fmt.Errorf("merge failed: %w", mergeErr)
	}

	return "PR merged (squash) with admin bypass", nil
}

// executeApproveTaskStep handles task approval
func executeApproveTaskStep(ctx context.Context, stepCtx *approveStepContext) (string, error) {
	if err := approveTask(ctx, stepCtx.taskStore, stepCtx.t); err != nil {
		return "", fmt.Errorf("failed to approve task: %w", err)
	}

	// Complete linked backlog discovery if present
	completeLinkedDiscovery(ctx, stepCtx.t)

	return "Task approved", nil
}

// completeLinkedDiscovery marks a linked backlog discovery as completed.
// This is a best-effort operation - failures are logged but don't fail the approval.
func completeLinkedDiscovery(ctx context.Context, t *domain.Task) {
	if t == nil || t.Metadata == nil {
		return
	}

	backlogID, ok := t.Metadata["from_backlog_id"].(string)
	if !ok || backlogID == "" {
		return
	}

	logger := Logger()
	mgr, err := backlog.NewManager("")
	if err != nil {
		logger.Warn().Err(err).
			Str("discovery_id", backlogID).
			Msg("failed to create backlog manager for discovery completion")
		return
	}

	_, err = mgr.Complete(ctx, backlogID)
	if err != nil {
		logger.Warn().Err(err).
			Str("discovery_id", backlogID).
			Msg("failed to complete backlog discovery")
		return
	}

	logger.Info().
		Str("discovery_id", backlogID).
		Str("task_id", t.ID).
		Msg("backlog discovery marked as completed")
}

// executeCloseWorkspaceStep handles workspace closure
func executeCloseWorkspaceStep(ctx context.Context, stepCtx *approveStepContext) (string, error) {
	warning, err := closeWorkspace(ctx, stepCtx.ws.Name)
	if err != nil {
		return "", fmt.Errorf("failed to close workspace: %w", err)
	}

	msg := fmt.Sprintf("Workspace '%s' closed", stepCtx.ws.Name)
	if warning != "" {
		msg = fmt.Sprintf("%s (warning: %s)", msg, warning)
	}
	return msg, nil
}

// closeWorkspace closes the workspace, removing the worktree but preserving history.
// Returns a warning string if worktree removal failed (workspace is still closed).
func closeWorkspace(ctx context.Context, workspaceName string) (warning string, err error) {
	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return "", fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Get workspace to find worktree path
	ws, err := wsStore.Get(ctx, workspaceName)
	if err != nil {
		// If workspace not found, treat it as already closed
		if errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
			return "workspace already closed or not found", nil
		}
		return "", fmt.Errorf("failed to get workspace: %w", err)
	}

	// Get repo path for worktree runner
	repoPath, err := detectRepoPath()
	if err != nil {
		// If we can't detect repo, worktree operations will fail gracefully
		repoPath = ""
	}

	// Create worktree runner (may be nil if no repo path)
	var wtRunner workspace.WorktreeRunner
	if repoPath != "" {
		wtRunner, err = workspace.NewGitWorktreeRunner(ctx, repoPath, Logger())
		if err != nil {
			// Continue without worktree runner - close should still update state
			wtRunner = nil
		}
	}

	// Create task store to check for running tasks before closing
	// This prevents closing a workspace while tasks are actively running
	var taskLister workspace.TaskLister
	taskStore, taskErr := task.NewFileStore("")
	if taskErr == nil {
		taskLister = taskStore
	}

	// Create manager and close
	mgr := workspace.NewManager(wsStore, wtRunner, Logger())
	result, closeErr := mgr.Close(ctx, ws.Name, taskLister)
	if closeErr != nil {
		return "", fmt.Errorf("failed to close workspace: %w", closeErr)
	}

	// Return any warnings about worktree or branch removal failures
	var warnings []string
	if result != nil {
		if result.WorktreeWarning != "" {
			warnings = append(warnings, result.WorktreeWarning)
		}
		if result.BranchWarning != "" {
			warnings = append(warnings, result.BranchWarning)
		}
	}

	if len(warnings) > 0 {
		return strings.Join(warnings, "; "), nil
	}

	return "", nil
}
