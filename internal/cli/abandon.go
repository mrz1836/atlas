// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddAbandonCommand adds the abandon command to the root command.
func AddAbandonCommand(root *cobra.Command) {
	root.AddCommand(newAbandonCmd())
}

// newAbandonCmd creates the abandon command.
func newAbandonCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "abandon <workspace>",
		Short: "Abandon a failed task while preserving the branch and worktree",
		Long: `Abandon a task that is in an error state (validation_failed, gh_failed, ci_failed, ci_timeout).

Use --force to:
  - Skip the confirmation prompt
  - Force-abandon running tasks (terminates tracked processes and marks task as abandoned)

The task will be marked as abandoned, but the git branch and worktree will be preserved
for manual work. You can still access the code at the worktree path.

Examples:
  atlas abandon auth-fix           # Abandon task with confirmation
  atlas abandon auth-fix --force   # Force-abandon without confirmation or force-abandon running task`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runAbandon(cmd.Context(), cmd, os.Stdout, args[0], force, "")
			// If JSON error was already output, silence cobra's error printing
			// but still return error for non-zero exit code
			if stderrors.Is(err, errors.ErrJSONErrorOutput) {
				cmd.SilenceErrors = true
			}
			return err
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

// runAbandon executes the abandon command.
func runAbandon(ctx context.Context, cmd *cobra.Command, w io.Writer, workspaceName string, force bool, storeBaseDir string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get output format from global flags
	outputFormat := cmd.Flag("output").Value.String()

	return runAbandonWithOutput(ctx, w, workspaceName, force, storeBaseDir, outputFormat)
}

// runAbandonWithOutput executes the abandon command with explicit output format.
func runAbandonWithOutput(ctx context.Context, w io.Writer, workspaceName string, force bool, storeBaseDir, outputFormat string) error {
	logger := GetLogger()
	tui.CheckNoColor()

	// Set up workspace and task stores
	wsMgr, ws, err := setupWorkspace(ctx, workspaceName, storeBaseDir, outputFormat, w)
	if err != nil {
		return err
	}

	taskStore, currentTask, err := getLatestTask(ctx, workspaceName, storeBaseDir, outputFormat, w, logger)
	if err != nil {
		return err
	}

	// Validate and confirm abandonment
	if err := validateAbandonability(currentTask.Status, force, outputFormat, w, workspaceName, currentTask.ID); err != nil {
		return err
	}

	if !force {
		if err := confirmAbandonmentInteractive(workspaceName, currentTask, outputFormat, w); err != nil {
			return err
		}
	}

	// Execute abandonment
	return executeAbandon(ctx, w, wsMgr, taskStore, currentTask, ws, workspaceName, force, outputFormat, logger)
}

// setupWorkspace creates workspace manager and retrieves workspace.
func setupWorkspace(ctx context.Context, workspaceName, storeBaseDir, outputFormat string, w io.Writer) (workspace.Manager, *domain.Workspace, error) {
	wsStore, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create workspace store: %w", err))
	}

	repoPath, err := detectRepoPath()
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("not in a git repository: %w", err))
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, repoPath)
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create worktree runner: %w", err))
	}

	wsMgr := workspace.NewManager(wsStore, wtRunner)
	ws, err := wsMgr.Get(ctx, workspaceName)
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to get workspace: %w", err))
	}

	return wsMgr, ws, nil
}

// getLatestTask retrieves the latest task for the workspace.
func getLatestTask(ctx context.Context, workspaceName, storeBaseDir, outputFormat string, w io.Writer, logger zerolog.Logger) (*task.FileStore, *domain.Task, error) {
	taskStore, err := task.NewFileStore(storeBaseDir)
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create task store: %w", err))
	}

	tasks, err := taskStore.List(ctx, workspaceName)
	if err != nil {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to list tasks: %w", err))
	}

	if len(tasks) == 0 {
		return nil, nil, handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("no tasks found in workspace '%s': %w", workspaceName, errors.ErrNoTasksFound))
	}

	currentTask := tasks[0]
	logger.Debug().
		Str("workspace_name", workspaceName).
		Str("task_id", currentTask.ID).
		Str("status", string(currentTask.Status)).
		Msg("found task to abandon")

	return taskStore, currentTask, nil
}

// validateAbandonability checks if the task can be abandoned.
func validateAbandonability(status constants.TaskStatus, force bool, outputFormat string, w io.Writer, workspaceName, taskID string) error {
	if !task.CanAbandon(status) {
		if !force && task.CanForceAbandon(status) {
			return handleAbandonError(outputFormat, w, workspaceName, taskID,
				fmt.Errorf("%w: task status %s cannot be abandoned without --force",
					errors.ErrInvalidTransition, status))
		}
		if !task.CanForceAbandon(status) {
			return handleAbandonError(outputFormat, w, workspaceName, taskID,
				fmt.Errorf("%w: task status %s cannot be abandoned",
					errors.ErrInvalidTransition, status))
		}
	}
	return nil
}

// confirmAbandonmentInteractive handles interactive confirmation.
func confirmAbandonmentInteractive(workspaceName string, currentTask *domain.Task, outputFormat string, w io.Writer) error {
	if !terminalCheck() {
		return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("cannot abandon task: %w", errors.ErrNonInteractiveMode))
	}

	confirmed, err := confirmAbandon(workspaceName, currentTask.Status == constants.TaskStatusRunning)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("failed to get confirmation: %w", err))
	}

	if !confirmed {
		out := tui.NewOutput(w, outputFormat)
		out.Info("Abandonment canceled")
		return nil
	}
	return nil
}

// executeAbandon performs the actual abandonment and updates workspace.
func executeAbandon(ctx context.Context, w io.Writer, wsMgr workspace.Manager, taskStore *task.FileStore,
	currentTask *domain.Task, ws *domain.Workspace, workspaceName string, force bool, outputFormat string, logger zerolog.Logger,
) error {
	engine := task.NewEngine(taskStore, nil, task.DefaultEngineConfig(), logger)

	reason := "User requested abandonment"
	if err := engine.Abandon(ctx, currentTask, reason, force); err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("failed to abandon task: %w", err))
	}

	if err := wsMgr.UpdateStatus(ctx, workspaceName, constants.WorkspaceStatusPaused); err != nil {
		logger.Warn().Err(err).Str("workspace_name", workspaceName).Msg("failed to update workspace status to paused")
	}

	if outputFormat == OutputJSON {
		return outputAbandonSuccessJSON(w, workspaceName, currentTask.ID, ws.Branch, ws.WorktreePath)
	}

	out := tui.NewOutput(w, outputFormat)
	tui.DisplayAbandonmentSuccess(out, currentTask, ws)
	return nil
}

// createAbandonConfirmForm is the default factory for creating abandon confirmation forms.
// This variable can be overridden in tests to inject mock forms.
//
//nolint:gochecknoglobals // Test injection point - standard Go testing pattern
var createAbandonConfirmForm = defaultCreateAbandonConfirmForm

// formRunner is an interface that matches huh.Form's Run method.
type formRunner interface {
	Run() error
}

// defaultCreateAbandonConfirmForm creates the actual Charm Huh form for abandon confirmation.
func defaultCreateAbandonConfirmForm(workspaceName string, isRunning bool, confirm *bool) formRunner {
	description := "Branch and worktree will be preserved for manual work."
	if isRunning {
		description = "⚠️  WARNING: Task is currently running. This will attempt to terminate processes and mark the task as abandoned.\n\n" + description
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Abandon task in workspace '%s'?", workspaceName)).
				Description(description).
				Affirmative("Yes, abandon").
				Negative("No, cancel").
				Value(confirm),
		),
	)
}

// confirmAbandon prompts the user for confirmation before abandoning a task.
func confirmAbandon(workspaceName string, isRunning bool) (bool, error) {
	var confirm bool
	form := createAbandonConfirmForm(workspaceName, isRunning, &confirm)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// abandonResult represents the JSON output for abandon operations.
type abandonResult struct {
	Status       string `json:"status"`
	Workspace    string `json:"workspace"`
	TaskID       string `json:"task_id,omitempty"`
	Branch       string `json:"branch,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Error        string `json:"error,omitempty"`
}

// handleAbandonError handles errors based on output format.
func handleAbandonError(format string, w io.Writer, workspaceName, taskID string, err error) error {
	if format == OutputJSON {
		_ = outputAbandonErrorJSON(w, workspaceName, taskID, err.Error())
		return errors.ErrJSONErrorOutput
	}
	return err
}

// outputAbandonSuccessJSON outputs a success result as JSON.
func outputAbandonSuccessJSON(w io.Writer, workspaceName, taskID, branch, worktreePath string) error {
	result := abandonResult{
		Status:       "abandoned",
		Workspace:    workspaceName,
		TaskID:       taskID,
		Branch:       branch,
		WorktreePath: worktreePath,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputAbandonErrorJSON outputs an error result as JSON.
func outputAbandonErrorJSON(w io.Writer, workspaceName, taskID, errMsg string) error {
	result := abandonResult{
		Status:    "error",
		Workspace: workspaceName,
		TaskID:    taskID,
		Error:     errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
