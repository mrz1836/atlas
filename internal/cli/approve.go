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

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddApproveCommand adds the approve command to the root command.
func AddApproveCommand(root *cobra.Command) {
	root.AddCommand(newApproveCmd())
}

// approveOptions contains all options for the approve command.
type approveOptions struct {
	workspace   string // Optional workspace name
	autoApprove bool   // Skip interactive menu and approve directly
	closeWS     bool   // Also close the workspace after approval
}

// newApproveCmd creates the approve command.
func newApproveCmd() *cobra.Command {
	var autoApprove bool
	var closeWS bool

	cmd := &cobra.Command{
		Use:   "approve [workspace]",
		Short: "Approve a completed task",
		Long: `Approve a task that has passed validation and is awaiting approval.

If multiple tasks are awaiting approval, you'll be prompted to select one.
You can also specify a workspace name directly to skip the selection.

The approval flow shows a summary of the task and provides options to:
  - Approve and complete the task
  - Approve and close the workspace (removes worktree, preserves history)
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
  atlas approve -o json my-feature  # Output result as JSON`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := approveOptions{
				autoApprove: autoApprove,
				closeWS:     closeWS,
			}
			if len(args) > 0 {
				opts.workspace = args[0]
			}
			return runApprove(cmd.Context(), cmd, os.Stdout, opts)
		},
	}

	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip interactive menu and approve directly")
	cmd.Flags().BoolVar(&closeWS, "close", false, "Also close the workspace after approval (removes worktree, preserves history)")

	return cmd
}

// approveResponse represents the JSON output for approve operations.
type approveResponse struct {
	Success         bool          `json:"success"`
	Workspace       workspaceInfo `json:"workspace"`
	Task            taskInfo      `json:"task"`
	PRURL           string        `json:"pr_url,omitempty"`
	WorkspaceClosed bool          `json:"workspace_closed,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// approvalAction represents actions available in the approval menu.
type approvalAction string

const (
	actionApprove         approvalAction = "approve"
	actionApproveAndClose approvalAction = "approve_and_close"
	actionViewDiff        approvalAction = "view_diff"
	actionViewLogs        approvalAction = "view_logs"
	actionOpenPR          approvalAction = "open_pr"
	actionReject          approvalAction = "reject"
	actionCancel          approvalAction = "cancel"
)

// runApprove executes the approve command.
func runApprove(ctx context.Context, cmd *cobra.Command, w io.Writer, opts approveOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	logger := GetLogger()
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
		return err
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
	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return nil, nil, handleApproveError(outputFormat, w, "", fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Create task store
	taskStore, err := task.NewFileStore("")
	if err != nil {
		return nil, nil, handleApproveError(outputFormat, w, "", fmt.Errorf("failed to create task store: %w", err))
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
	options := make([]tui.Option, len(tasks))
	for i, at := range tasks {
		options[i] = tui.Option{
			Label:       at.workspace.Name,
			Description: at.task.Description,
			Value:       at.workspace.Name,
		}
	}

	selected, err := tui.Select("Select a workspace to approve:", options)
	if err != nil {
		return nil, err
	}

	// Find the selected task
	for i, at := range tasks {
		if at.workspace.Name == selected {
			return &tasks[i], nil
		}
	}

	return nil, fmt.Errorf("selected workspace not found: %w", atlaserrors.ErrWorkspaceNotFound)
}

// runAutoApprove performs automatic approval without interactive menu.
func runAutoApprove(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, closeWS bool) error {
	// Approve the task directly
	if err := approveTask(ctx, taskStore, t); err != nil {
		return fmt.Errorf("failed to approve task: %w", err)
	}

	out.Success(fmt.Sprintf("Task approved: %s", t.Description))
	out.Info(fmt.Sprintf("  Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("  Task ID:   %s", t.ID))
	if prURL := extractPRURL(t); prURL != "" {
		out.Info(fmt.Sprintf("  PR URL:    %s", prURL))
	}

	// Close workspace if requested
	if closeWS {
		if err := closeWorkspace(ctx, ws.Name); err != nil {
			out.Warning(fmt.Sprintf("Failed to close workspace: %s", err.Error()))
		} else {
			out.Success(fmt.Sprintf("Workspace '%s' closed. History preserved.", ws.Name))
		}
	}

	notifier.Bell()
	return nil
}

// runInteractiveApproval runs the interactive approval flow with action menu.
func runInteractiveApproval(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, verbose bool) error {
	// Display approval summary (AC: #2)
	summary := tui.NewApprovalSummary(t, ws)
	_ = out // Mark out as used - printApprovalSummary writes to stdout directly for styled output
	printApprovalSummary(summary, verbose)

	// Action menu loop (AC: #3, #4)
	return runApprovalActionLoop(ctx, out, taskStore, ws, t, notifier)
}

// printApprovalSummary prints the approval summary to stdout.
func printApprovalSummary(summary *tui.ApprovalSummary, verbose bool) {
	rendered := tui.RenderApprovalSummaryWithWidth(summary, 0, verbose)
	_, _ = os.Stdout.WriteString(rendered + "\n")
}

// runApprovalActionLoop handles the approval action menu loop.
func runApprovalActionLoop(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	for {
		action, err := selectApprovalAction(t)
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Approval canceled.")
				return nil
			}
			return err
		}

		done, err := executeApprovalAction(ctx, out, taskStore, ws, t, notifier, action)
		if err != nil {
			return err
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
		if err := approveTask(ctx, taskStore, t); err != nil {
			out.Error(tui.WrapWithSuggestion(err))
			return false, nil // Continue loop on error
		}
		out.Success("Task approved. PR ready for merge.")
		notifier.Bell()
		return true, nil

	case actionApproveAndClose:
		if err := approveTask(ctx, taskStore, t); err != nil {
			out.Error(tui.WrapWithSuggestion(err))
			return false, nil // Continue loop on error
		}
		out.Success("Task approved. PR ready for merge.")
		if err := closeWorkspace(ctx, ws.Name); err != nil {
			out.Warning(fmt.Sprintf("Failed to close workspace: %s", err.Error()))
		} else {
			out.Success(fmt.Sprintf("Workspace '%s' closed. History preserved.", ws.Name))
		}
		notifier.Bell()
		return true, nil

	case actionViewDiff:
		if err := viewDiff(ctx, ws.WorktreePath); err != nil {
			out.Warning("Could not display diff: " + err.Error())
		}
		return false, nil

	case actionViewLogs:
		if err := viewLogs(ctx, taskStore, ws.Name, t.ID); err != nil {
			out.Warning("Could not display logs: " + err.Error())
		}
		return false, nil

	case actionOpenPR:
		prURL := extractPRURL(t)
		if prURL == "" {
			out.Warning("No PR URL available.")
		} else if err := openInBrowser(ctx, prURL); err != nil {
			out.Warning("Could not open PR: " + err.Error())
		} else {
			out.Info("Opened " + prURL + " in browser.")
		}
		return false, nil

	case actionReject:
		out.Info("Run 'atlas reject " + ws.Name + "' to reject with feedback.")
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
		{Label: "View diff", Description: "Show file changes", Value: string(actionViewDiff)},
		{Label: "View logs", Description: "Show task execution log", Value: string(actionViewLogs)},
	}

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

	selected, err := tui.Select("What would you like to do?", options)
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
	gitCmd := exec.CommandContext(ctx, "git", "-C", worktreePath, "diff", "HEAD~1")
	gitOutput, err := gitCmd.Output()
	if err != nil {
		// Try without HEAD~1 for new repos
		gitCmd = exec.CommandContext(ctx, "git", "-C", worktreePath, "diff")
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
	lessCmd := exec.CommandContext(ctx, "less", "-R")
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

// openInBrowser opens a URL in the default browser (macOS).
func openInBrowser(ctx context.Context, url string) error {
	cmd := exec.CommandContext(ctx, "open", url)
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

	// Close workspace if requested
	workspaceClosed := false
	if closeWS {
		if err := closeWorkspace(ctx, ws.Name); err == nil {
			workspaceClosed = true
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
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// handleApproveError handles errors based on output format.
func handleApproveError(format string, w io.Writer, workspaceName string, err error) error {
	if format == OutputJSON {
		return outputApproveErrorJSON(w, workspaceName, "", err.Error())
	}
	return err
}

// outputApproveErrorJSON outputs an error result as JSON.
func outputApproveErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	resp := approveResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: workspaceName,
		},
		Task: taskInfo{
			ID: taskID,
		},
		Error: errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(resp); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}

// closeWorkspace closes the workspace, removing the worktree but preserving history.
func closeWorkspace(ctx context.Context, workspaceName string) error {
	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Get workspace to find worktree path
	ws, err := wsStore.Get(ctx, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
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
		//nolint:contextcheck // NewGitWorktreeRunner doesn't take context; it only detects repo root
		wtRunner, err = workspace.NewGitWorktreeRunner(repoPath)
		if err != nil {
			// Continue without worktree runner - close should still update state
			wtRunner = nil
		}
	}

	// Create manager and close
	mgr := workspace.NewManager(wsStore, wtRunner)
	if err := mgr.Close(ctx, ws.Name); err != nil {
		return fmt.Errorf("failed to close workspace: %w", err)
	}

	return nil
}
