// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddRecoverCommand adds the recover command to the root command.
func AddRecoverCommand(root *cobra.Command) {
	root.AddCommand(newRecoverCmd())
}

// recoverOptions contains all options for the recover command.
type recoverOptions struct {
	workspace         string // Required workspace name
	retry             bool   // For JSON mode: retry with AI fix
	manual            bool   // For JSON mode: fix manually instructions
	abandon           bool   // For JSON mode: abandon task
	continueExecution bool   // For JSON mode: continue waiting (CI timeout only)
}

// newRecoverCmd creates the recover command.
func newRecoverCmd() *cobra.Command {
	opts := &recoverOptions{}

	cmd := &cobra.Command{
		Use:   "recover [workspace]",
		Short: "Recover from task error states",
		Long: `Recover from a task that is in an error state.

This command handles error recovery for tasks in the following states:
  - validation_failed: Validation checks failed
  - gh_failed: GitHub operations (push/PR) failed
  - ci_failed: CI pipeline checks failed
  - ci_timeout: CI pipeline exceeded timeout

Interactive mode:
  atlas recover my-feature

  Presents error-specific recovery options:
  - Retry with AI fix - AI attempts to fix based on error context
  - Fix manually - Edit files in worktree, then resume
  - View errors/logs - See detailed error output
  - Abandon task - End task, preserve branch for later

JSON mode (requires --output json and one action flag):
  atlas recover my-feature --output json --retry     # Retry with AI
  atlas recover my-feature --output json --manual    # Fix manually instructions
  atlas recover my-feature --output json --abandon   # Abandon task
  atlas recover my-feature --output json --continue  # Continue waiting (ci_timeout only)

Examples:
  atlas recover                    # Interactive selection if multiple error tasks
  atlas recover my-feature         # Recover task in my-feature workspace
  atlas recover -o json my-feature --abandon  # JSON output, abandon task`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.workspace = args[0]
			}
			return runRecover(cmd.Context(), cmd, os.Stdout, opts)
		},
	}

	// Add JSON mode flags
	cmd.Flags().BoolVar(&opts.retry, "retry", false, "Retry with AI fix (JSON mode)")
	cmd.Flags().BoolVar(&opts.manual, "manual", false, "Get fix manually instructions (JSON mode)")
	cmd.Flags().BoolVar(&opts.abandon, "abandon", false, "Abandon task (JSON mode)")
	cmd.Flags().BoolVar(&opts.continueExecution, "continue", false, "Continue waiting (JSON mode, ci_timeout only)")

	return cmd
}

// recoverResponse represents the JSON output for recover operations.
type recoverResponse struct {
	Success       bool   `json:"success"`
	Action        string `json:"action"` // "retry", "manual", "abandon", "continue"
	WorkspaceName string `json:"workspace_name"`
	TaskID        string `json:"task_id"`
	ErrorState    string `json:"error_state"`
	WorktreePath  string `json:"worktree_path,omitempty"`
	Instructions  string `json:"instructions,omitempty"`
	GitHubURL     string `json:"github_url,omitempty"`
	Error         string `json:"error,omitempty"`
}

// errorTask holds a workspace and its task in an error state.
type errorTask struct {
	workspace *domain.Workspace
	task      *domain.Task
}

// runRecover executes the recover command.
func runRecover(ctx context.Context, cmd *cobra.Command, w io.Writer, opts *recoverOptions) error {
	// Check context cancellation at entry
	select {
	case <-ctx.Done():
		return fmt.Errorf("recover command canceled: %w", ctx.Err())
	default:
	}

	logger := Logger()
	outputFormat := cmd.Flag("output").Value.String()

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// JSON mode requires workspace argument
	if outputFormat == OutputJSON && opts.workspace == "" {
		return handleRecoverError(outputFormat, w, "", fmt.Errorf("workspace argument required with --output json: %w", atlaserrors.ErrInvalidArgument))
	}

	// JSON mode requires exactly one action flag
	actionCount := countBool(opts.retry, opts.manual, opts.abandon, opts.continueExecution)
	if outputFormat == OutputJSON {
		if actionCount == 0 {
			return handleRecoverError(outputFormat, w, opts.workspace, fmt.Errorf("one of --retry, --manual, --abandon, or --continue required with --output json: %w", atlaserrors.ErrInvalidArgument))
		}
		if actionCount > 1 {
			return handleRecoverError(outputFormat, w, opts.workspace, fmt.Errorf("only one action flag allowed: %w", atlaserrors.ErrInvalidArgument))
		}
	}

	// Create stores
	wsStore, taskStore, err := CreateStores("")
	if err != nil {
		return handleRecoverError(outputFormat, w, "", err)
	}

	// Find and select task
	selectedWS, selectedTask, err := findAndSelectErrorTask(ctx, outputFormat, w, out, opts, wsStore, taskStore)
	if err != nil {
		return err
	}
	if selectedWS == nil {
		// No error tasks found (message already shown)
		return nil
	}

	logger.Debug().
		Str("workspace_name", selectedWS.Name).
		Str("task_id", selectedTask.ID).
		Str("status", string(selectedTask.Status)).
		Msg("selected task for recovery")

	// Validate task is in an error state
	if !task.IsErrorStatus(selectedTask.Status) {
		return handleRecoverError(outputFormat, w, selectedWS.Name, fmt.Errorf("task is not in an error state (status: %s): %w", selectedTask.Status, atlaserrors.ErrInvalidStatus))
	}

	// Validate --continue flag is only used with ci_timeout
	if opts.continueExecution && selectedTask.Status != constants.TaskStatusCITimeout {
		return handleRecoverError(outputFormat, w, selectedWS.Name, fmt.Errorf("--continue flag only valid for ci_timeout state (current: %s): %w", selectedTask.Status, atlaserrors.ErrInvalidStatus))
	}

	// Load config for notification settings
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load config, using default notification settings")
		cfg = config.DefaultConfig()
	}

	// Create notifier
	notifier := tui.NewNotifier(cfg.Notifications.Bell, false)

	// JSON mode: process directly without interactive menu
	if outputFormat == OutputJSON {
		return processJSONRecover(ctx, w, taskStore, selectedWS, selectedTask, opts)
	}

	// Interactive recovery flow
	return runInteractiveRecover(ctx, out, taskStore, selectedWS, selectedTask, notifier)
}

// countBool counts how many booleans are true.
func countBool(bools ...bool) int {
	count := 0
	for _, b := range bools {
		if b {
			count++
		}
	}
	return count
}

// findAndSelectErrorTask finds error tasks and selects one based on options.
func findAndSelectErrorTask(ctx context.Context, outputFormat string, w io.Writer, out tui.Output, opts *recoverOptions, wsStore workspace.Store, taskStore task.Store) (*domain.Workspace, *domain.Task, error) {
	// Find tasks in error states
	errorTasks, err := findErrorTasks(ctx, wsStore, taskStore)
	if err != nil {
		return nil, nil, handleRecoverError(outputFormat, w, "", err)
	}

	// Handle case where no tasks are in error states
	if len(errorTasks) == 0 {
		return handleNoErrorTasks(ctx, outputFormat, w, out, wsStore, taskStore)
	}

	// Select or find the appropriate error task
	return selectErrorTask(errorTasks, outputFormat, w, out, opts)
}

// handleNoErrorTasks handles the case when no tasks are in error states.
func handleNoErrorTasks(ctx context.Context, outputFormat string, w io.Writer, out tui.Output, wsStore workspace.Store, taskStore task.Store) (*domain.Workspace, *domain.Task, error) {
	if outputFormat == OutputJSON {
		return nil, nil, handleRecoverError(outputFormat, w, "", atlaserrors.ErrNoTasksFound)
	}

	out.Info("No tasks in error states.")
	displayRunningTasksHint(ctx, out, wsStore, taskStore)
	out.Info("Run 'atlas status' to see all workspace statuses.")

	return nil, nil, nil
}

// displayRunningTasksHint checks for and displays hints about potentially stuck running tasks.
func displayRunningTasksHint(ctx context.Context, out tui.Output, wsStore workspace.Store, taskStore task.Store) {
	runningTasks, findErr := findRunningTasks(ctx, wsStore, taskStore)
	if findErr != nil || len(runningTasks) == 0 {
		return
	}

	out.Info("")
	if len(runningTasks) == 1 {
		out.Info("Found 1 workspace with a running task that may be stuck:")
	} else {
		out.Info(fmt.Sprintf("Found %d workspaces with running tasks that may be stuck:", len(runningTasks)))
	}

	for _, rt := range runningTasks {
		stepInfo := fmt.Sprintf("step %d/%d", rt.task.CurrentStep+1, len(rt.task.Steps))
		out.Info(fmt.Sprintf("  %s: running at %s", rt.workspace.Name, stepInfo))
	}

	out.Info("")
	out.Info("If a task is stuck, you can force-abandon it:")
	for _, rt := range runningTasks {
		out.Info(fmt.Sprintf("  atlas abandon %s --force", rt.workspace.Name))
	}
	out.Info("")
}

// selectErrorTask selects an error task based on provided options or user input.
func selectErrorTask(errorTasks []errorTask, outputFormat string, w io.Writer, out tui.Output, opts *recoverOptions) (*domain.Workspace, *domain.Task, error) {
	// If workspace provided, find it directly
	if opts.workspace != "" {
		return findErrorTaskByName(errorTasks, opts.workspace, outputFormat, w)
	}

	// Auto-select if only one task
	if len(errorTasks) == 1 {
		return errorTasks[0].workspace, errorTasks[0].task, nil
	}

	// Present selection menu
	return selectErrorTaskFromMenu(errorTasks, outputFormat, w, out)
}

// findErrorTaskByName finds an error task by workspace name.
func findErrorTaskByName(errorTasks []errorTask, workspaceName, outputFormat string, w io.Writer) (*domain.Workspace, *domain.Task, error) {
	for _, et := range errorTasks {
		if et.workspace.Name == workspaceName {
			return et.workspace, et.task, nil
		}
	}
	return nil, nil, handleRecoverError(outputFormat, w, workspaceName, fmt.Errorf("workspace '%s' not found or not in error state: %w", workspaceName, atlaserrors.ErrWorkspaceNotFound))
}

// selectErrorTaskFromMenu presents a menu for the user to select an error task.
func selectErrorTaskFromMenu(errorTasks []errorTask, outputFormat string, w io.Writer, out tui.Output) (*domain.Workspace, *domain.Task, error) {
	selected, err := selectWorkspaceForRecovery(errorTasks)
	if err != nil {
		if errors.Is(err, tui.ErrMenuCanceled) {
			out.Info("Recovery canceled.")
			return nil, nil, nil
		}
		return nil, nil, handleRecoverError(outputFormat, w, "", err)
	}

	return selected.workspace, selected.task, nil
}

// findErrorTasks finds all tasks in error states.
func findErrorTasks(ctx context.Context, wsStore workspace.Store, taskStore task.Store) ([]errorTask, error) {
	// List all workspaces
	workspaces, err := wsStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var result []errorTask

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

		// Find the latest task in an error state
		for _, t := range tasks {
			if task.IsErrorStatus(t.Status) {
				result = append(result, errorTask{
					workspace: ws,
					task:      t,
				})
				break // Only count the latest task per workspace
			}
		}
	}

	return result, nil
}

// runningTask holds a workspace and its task in running state.
type runningTask struct {
	workspace *domain.Workspace
	task      *domain.Task
}

// findRunningTasks finds all tasks in running state.
// These tasks may be stuck if the process executing them crashed.
func findRunningTasks(ctx context.Context, wsStore workspace.Store, taskStore task.Store) ([]runningTask, error) {
	// List all workspaces
	workspaces, err := wsStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var result []runningTask

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

		// Find the latest task in running state
		for _, t := range tasks {
			if t.Status == constants.TaskStatusRunning {
				result = append(result, runningTask{
					workspace: ws,
					task:      t,
				})
				break // Only count the latest task per workspace
			}
		}
	}

	return result, nil
}

// selectWorkspaceForRecovery presents a selection menu for multiple error tasks.
func selectWorkspaceForRecovery(tasks []errorTask) (*errorTask, error) {
	options := make([]tui.Option, len(tasks))
	for i, et := range tasks {
		options[i] = tui.Option{
			Label:       et.workspace.Name,
			Description: fmt.Sprintf("%s - %s", et.task.Status, et.task.Description),
			Value:       et.workspace.Name,
		}
	}

	selected, err := tui.Select("Select a workspace to recover:", options)
	if err != nil {
		return nil, err
	}

	// Find the selected task
	for i, et := range tasks {
		if et.workspace.Name == selected {
			return &tasks[i], nil
		}
	}

	return nil, fmt.Errorf("selected workspace not found: %w", atlaserrors.ErrWorkspaceNotFound)
}

// runInteractiveRecover runs the interactive recovery flow.
func runInteractiveRecover(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	// Display error context
	displayErrorContext(out, ws, t)

	// Action menu loop - view actions return to menu
	return runRecoveryActionLoop(ctx, out, taskStore, ws, t, notifier)
}

// displayErrorContext shows the error state and relevant information.
func displayErrorContext(out tui.Output, ws *domain.Workspace, t *domain.Task) {
	out.Info(fmt.Sprintf("Workspace: %s", ws.Name))
	out.Info(fmt.Sprintf("Task: %s", t.Description))
	out.Info(fmt.Sprintf("Status: %s", tui.TaskStatusIcon(t.Status)+" "+string(t.Status)))
	out.Info("")
}

// runRecoveryActionLoop handles the recovery action menu loop.
func runRecoveryActionLoop(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) error {
	for {
		action, err := selectRecoveryAction(t)
		if err != nil {
			if errors.Is(err, tui.ErrMenuCanceled) {
				out.Info("Recovery canceled.")
				return nil
			}
			return err
		}

		done, err := executeRecoveryAction(ctx, out, taskStore, ws, t, notifier, action)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		// Continue loop for view actions
	}
}

// selectRecoveryAction selects the appropriate recovery menu based on task state.
// For GH failed state with specific error types, shows context-aware options.
func selectRecoveryAction(t *domain.Task) (tui.RecoveryAction, error) {
	// For GH failed state, check for specific push error type
	if action, ok := tryGHFailedRecovery(t); ok {
		return action, nil
	}

	// Default: use standard recovery menu
	return tui.SelectErrorRecovery(t.Status)
}

// tryGHFailedRecovery attempts to show GH-specific recovery options.
// Returns the selected action and true if successful, or empty action and false otherwise.
func tryGHFailedRecovery(t *domain.Task) (tui.RecoveryAction, bool) {
	if t.Status != constants.TaskStatusGHFailed || t.Metadata == nil {
		return "", false
	}

	pushErrorType, ok := t.Metadata["push_error_type"].(string)
	if !ok || pushErrorType == "" {
		return "", false
	}

	options := tui.GHFailedOptionsForPushError(pushErrorType)
	if len(options) == 0 {
		return "", false
	}

	baseOptions := make([]tui.Option, len(options))
	for i, opt := range options {
		baseOptions[i] = opt.Option
	}

	title := tui.MenuTitleForStatus(t.Status)
	selected, err := tui.Select(title, baseOptions)
	if err != nil {
		return "", false
	}

	return tui.RecoveryAction(selected), true
}

// executeRecoveryAction executes the selected recovery action.
// Returns true if the action loop should exit.
func executeRecoveryAction(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier, action tui.RecoveryAction) (bool, error) {
	switch action {
	case tui.RecoveryActionRetryAI, tui.RecoveryActionRetryGH:
		return handleRetryAction(ctx, out, taskStore, t, notifier)

	case tui.RecoveryActionRebaseRetry:
		return handleRebaseRetry(ctx, out, taskStore, ws, t, notifier)

	case tui.RecoveryActionFixManually:
		return handleFixManually(out, ws, notifier)

	case tui.RecoveryActionViewErrors:
		return handleViewErrors(ctx, out, taskStore, ws.Name, t.ID)

	case tui.RecoveryActionViewLogs:
		return handleViewLogs(ctx, out, ws, t)

	case tui.RecoveryActionContinueWaiting:
		return handleContinueWaiting(ctx, out, taskStore, t, notifier)

	case tui.RecoveryActionAbandon:
		return handleAbandon(ctx, out, taskStore, ws, t, notifier)
	}

	return false, nil
}

// handleRetryAction handles retry with AI fix actions.
func handleRetryAction(ctx context.Context, out tui.Output, taskStore task.Store, t *domain.Task, notifier *tui.Notifier) (bool, error) {
	// Transition task back to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested retry"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return false, nil // Continue loop on error
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return false, nil
	}

	out.Success("Task transitioned to running. AI will retry with error context.")
	notifier.Bell()
	return true, nil
}

// handleFixManually shows worktree path and resume instructions.
func handleFixManually(out tui.Output, ws *domain.Workspace, notifier *tui.Notifier) (bool, error) {
	out.Info("Fix the issue in the worktree, then resume:")
	out.Info("")
	// M4 fix: Validate worktree path is not empty
	if ws.WorktreePath != "" {
		out.Info(fmt.Sprintf("  cd %s", ws.WorktreePath))
	} else {
		out.Info(fmt.Sprintf("  cd <worktree for %s>", ws.Name))
	}
	out.Info("  # Make your fixes")
	out.Info(fmt.Sprintf("  atlas resume %s", ws.Name))
	out.Info("")
	notifier.Bell()
	return true, nil
}

// handleRebaseRetry handles the "Rebase and retry" action for non-fast-forward push failures.
// It fetches from remote, rebases local commits onto the remote branch, and transitions
// the task back to running to retry the push.
func handleRebaseRetry(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) (bool, error) {
	// Validate worktree path
	if ws.WorktreePath == "" {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("worktree path not available: %w", atlaserrors.ErrWorktreeNotFound)))
		return false, nil
	}

	// Get branch name
	branch := ws.Branch
	if branch == "" {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("branch name not available: %w", atlaserrors.ErrEmptyValue)))
		return false, nil
	}
	remote := "origin"

	// Create git runner for worktree
	runner, err := git.NewRunner(ctx, ws.WorktreePath)
	if err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to create git runner: %w", err)))
		return false, nil
	}

	// Fetch latest from remote
	out.Info(fmt.Sprintf("Fetching latest from %s...", remote))
	if err := runner.Fetch(ctx, remote); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("fetch failed: %w", err)))
		return false, nil
	}

	// Attempt rebase
	rebaseTarget := fmt.Sprintf("%s/%s", remote, branch)
	out.Info(fmt.Sprintf("Rebasing onto %s...", rebaseTarget))
	if err := runner.Rebase(ctx, rebaseTarget); err != nil {
		// Check for conflicts
		if errors.Is(err, atlaserrors.ErrRebaseConflict) {
			// Abort the failed rebase
			_ = runner.RebaseAbort(ctx)

			out.Warning("Rebase has conflicts that require manual resolution:")
			out.Info("")
			out.Info(fmt.Sprintf("  cd %s", ws.WorktreePath))
			out.Info(fmt.Sprintf("  git fetch %s", remote))
			out.Info(fmt.Sprintf("  git rebase %s", rebaseTarget))
			out.Info("  # Resolve conflicts in your editor")
			out.Info("  git add <resolved-files>")
			out.Info("  git rebase --continue")
			out.Info(fmt.Sprintf("  atlas resume %s", ws.Name))
			out.Info("")
			notifier.Bell()
			return true, nil
		}

		out.Error(tui.WrapWithSuggestion(fmt.Errorf("rebase failed: %w", err)))
		return false, nil
	}

	// Rebase succeeded, transition task back to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "Rebased and retrying push"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return false, nil
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return false, nil
	}

	out.Success("Rebase successful. Task will retry push on next resume.")
	out.Info(fmt.Sprintf("Run: atlas resume %s", ws.Name))
	notifier.Bell()
	return true, nil
}

// handleViewErrors displays the validation errors.
func handleViewErrors(ctx context.Context, out tui.Output, taskStore task.Store, workspaceName, taskID string) (bool, error) {
	// Try to get validation artifact
	data, err := taskStore.GetArtifact(ctx, workspaceName, taskID, "validation.json")
	if err != nil {
		// Try alternate filename
		data, err = taskStore.GetArtifact(ctx, workspaceName, taskID, "validation-result.json")
		if err != nil {
			out.Warning(fmt.Sprintf("Could not load validation results: %v", err))
			return false, nil
		}
	}

	if len(data) == 0 {
		out.Info("No validation errors recorded.")
		return false, nil
	}

	// Display the validation output using the Output interface (M2 fix)
	out.Info("")
	out.Info("--- Validation Output ---")
	out.Info(string(data))
	out.Info("-------------------------")
	out.Info("")

	return false, nil // Return to menu
}

// handleViewLogs opens GitHub Actions in browser for CI states.
func handleViewLogs(ctx context.Context, out tui.Output, ws *domain.Workspace, t *domain.Task) (bool, error) {
	// Extract GitHub Actions URL from task metadata or PR URL
	ghURL := extractGitHubActionsURL(t)
	if ghURL == "" {
		// Fall back to PR URL if available
		prURL := extractPRURL(t)
		if prURL != "" {
			ghURL = prURL + "/checks"
		}
	}

	if ghURL == "" {
		out.Warning("No GitHub Actions URL available.")
		out.Info(fmt.Sprintf("You can manually check: https://github.com/%s/actions", extractRepoInfo(ws)))
		return false, nil
	}

	// Open in browser
	if err := openInBrowser(ctx, ghURL); err != nil {
		out.Warning(fmt.Sprintf("Could not open browser: %v", err))
		out.Info(fmt.Sprintf("URL: %s", ghURL))
	} else {
		out.Info(fmt.Sprintf("Opened %s in browser.", ghURL))
	}

	return false, nil // Return to menu
}

// handleContinueWaiting resumes CI polling.
func handleContinueWaiting(ctx context.Context, out tui.Output, taskStore task.Store, t *domain.Task, notifier *tui.Notifier) (bool, error) {
	// Transition task back to running to continue CI polling
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested to continue waiting"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return false, nil
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return false, nil
	}

	out.Success("Task transitioned to running. CI polling will resume with extended timeout.")
	notifier.Bell()
	return true, nil
}

// handleAbandon transitions task to abandoned state.
func handleAbandon(ctx context.Context, out tui.Output, taskStore task.Store, ws *domain.Workspace, t *domain.Task, notifier *tui.Notifier) (bool, error) {
	// Transition task to abandoned
	if err := task.Transition(ctx, t, constants.TaskStatusAbandoned, "User abandoned from error recovery"); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to transition task: %w", err)))
		return false, nil
	}

	// Save updated task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		out.Error(tui.WrapWithSuggestion(fmt.Errorf("failed to save task: %w", err)))
		return false, nil
	}

	out.Info(fmt.Sprintf("Task abandoned. Branch '%s' preserved at '%s'", ws.Branch, ws.WorktreePath))
	out.Info("You can work on the code manually or destroy the workspace later.")
	notifier.Bell()
	return true, nil
}

// extractGitHubActionsURL extracts the GitHub Actions URL from task metadata.
func extractGitHubActionsURL(t *domain.Task) string {
	if t == nil || t.Metadata == nil {
		return ""
	}
	if url, ok := t.Metadata["ci_url"].(string); ok {
		return url
	}
	if url, ok := t.Metadata["github_actions_url"].(string); ok {
		return url
	}
	return ""
}

// extractRepoInfo extracts repository info from workspace for manual URL construction.
func extractRepoInfo(ws *domain.Workspace) string {
	if ws == nil || ws.Metadata == nil {
		return ""
	}
	if repo, ok := ws.Metadata["repository"].(string); ok {
		return repo
	}
	return ""
}

// processJSONRecover handles JSON mode recovery.
func processJSONRecover(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task, opts *recoverOptions) error {
	switch {
	case opts.retry:
		return processJSONRetry(ctx, w, taskStore, ws, t)
	case opts.manual:
		return processJSONManual(w, ws, t)
	case opts.abandon:
		return processJSONAbandon(ctx, w, taskStore, ws, t)
	case opts.continueExecution:
		return processJSONContinue(ctx, w, taskStore, ws, t)
	}
	return handleRecoverError(OutputJSON, w, ws.Name, fmt.Errorf("no action specified: %w", atlaserrors.ErrInvalidArgument))
}

// processJSONRetry handles JSON mode retry action.
func processJSONRetry(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task) error {
	// Capture original error state before transition
	originalState := string(t.Status)

	// Transition task to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested retry (JSON mode)"); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to transition task: %v", err))
	}

	// Save task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to save task: %v", err))
	}

	resp := recoverResponse{
		Success:       true,
		Action:        "retry",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		ErrorState:    originalState,
		WorktreePath:  ws.WorktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// processJSONManual handles JSON mode fix manually action.
func processJSONManual(w io.Writer, ws *domain.Workspace, t *domain.Task) error {
	// M5 fix: Handle empty worktree path
	worktreePath := ws.WorktreePath
	var instructions string
	if worktreePath != "" {
		instructions = fmt.Sprintf("cd %s && # make fixes && atlas resume %s", worktreePath, ws.Name)
	} else {
		instructions = fmt.Sprintf("# locate worktree for %s && # make fixes && atlas resume %s", ws.Name, ws.Name)
	}

	resp := recoverResponse{
		Success:       true,
		Action:        "manual",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		ErrorState:    string(t.Status),
		WorktreePath:  worktreePath,
		Instructions:  instructions,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// processJSONAbandon handles JSON mode abandon action.
func processJSONAbandon(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task) error {
	// Capture original error state before transition
	originalState := string(t.Status)

	// Transition task to abandoned
	if err := task.Transition(ctx, t, constants.TaskStatusAbandoned, "User abandoned task (JSON mode)"); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to transition task: %v", err))
	}

	// Save task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to save task: %v", err))
	}

	resp := recoverResponse{
		Success:       true,
		Action:        "abandon",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		ErrorState:    originalState,
		WorktreePath:  ws.WorktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// processJSONContinue handles JSON mode continue waiting action.
func processJSONContinue(ctx context.Context, w io.Writer, taskStore task.Store, ws *domain.Workspace, t *domain.Task) error {
	// Capture original error state before transition
	originalState := string(t.Status)

	// Validate status is ci_timeout
	if t.Status != constants.TaskStatusCITimeout {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("--continue only valid for ci_timeout state, got: %s", t.Status))
	}

	// Transition task to running
	if err := task.Transition(ctx, t, constants.TaskStatusRunning, "User requested to continue waiting (JSON mode)"); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to transition task: %v", err))
	}

	// Save task
	if err := taskStore.Update(ctx, t.WorkspaceID, t); err != nil {
		return outputRecoverErrorJSON(w, ws.Name, t.ID, originalState, fmt.Sprintf("failed to save task: %v", err))
	}

	resp := recoverResponse{
		Success:       true,
		Action:        "continue",
		WorkspaceName: ws.Name,
		TaskID:        t.ID,
		ErrorState:    originalState,
		WorktreePath:  ws.WorktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(resp)
}

// handleRecoverError handles errors based on output format.
func handleRecoverError(format string, w io.Writer, workspaceName string, err error) error {
	if format == OutputJSON {
		return outputRecoverErrorJSON(w, workspaceName, "", "", err.Error())
	}
	return err
}

// outputRecoverErrorJSON outputs an error result as JSON.
func outputRecoverErrorJSON(w io.Writer, workspaceName, taskID, errorState, errMsg string) error {
	resp := recoverResponse{
		Success:       false,
		WorkspaceName: workspaceName,
		TaskID:        taskID,
		ErrorState:    errorState,
		Error:         errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(resp); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}
