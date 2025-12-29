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
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
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

The task will be marked as abandoned, but the git branch and worktree will be preserved
for manual work. You can still access the code at the worktree path.

Examples:
  atlas abandon auth-fix           # Abandon task with confirmation
  atlas abandon auth-fix --force   # Abandon without confirmation`,
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

	// Respect NO_COLOR environment variable
	tui.CheckNoColor()

	out := tui.NewOutput(w, outputFormat)

	// Create workspace store
	wsStore, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Find git repository for worktree runner
	repoPath, err := detectRepoPath()
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("not in a git repository: %w", err))
	}

	//nolint:contextcheck // NewGitWorktreeRunner doesn't take context; it only detects repo root
	wtRunner, err := workspace.NewGitWorktreeRunner(repoPath)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create worktree runner: %w", err))
	}

	wsMgr := workspace.NewManager(wsStore, wtRunner)

	// Get workspace
	ws, err := wsMgr.Get(ctx, workspaceName)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to get workspace: %w", err))
	}

	// Create task store
	taskStore, err := task.NewFileStore(storeBaseDir)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to create task store: %w", err))
	}

	// Get latest task for this workspace
	tasks, err := taskStore.List(ctx, workspaceName)
	if err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("failed to list tasks: %w", err))
	}

	if len(tasks) == 0 {
		return handleAbandonError(outputFormat, w, workspaceName, "", fmt.Errorf("no tasks found in workspace '%s': %w", workspaceName, errors.ErrNoTasksFound))
	}

	// Get the latest task (list returns newest first)
	currentTask := tasks[0]

	logger.Debug().
		Str("workspace_name", workspaceName).
		Str("task_id", currentTask.ID).
		Str("status", string(currentTask.Status)).
		Msg("found task to abandon")

	// Validate task is in abandonable state
	if !task.CanAbandon(currentTask.Status) {
		return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("%w: task status %s cannot be abandoned", errors.ErrInvalidTransition, currentTask.Status))
	}

	// Handle confirmation if needed
	if !force {
		if !terminalCheck() {
			return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
				fmt.Errorf("cannot abandon task: %w", errors.ErrNonInteractiveMode))
		}

		confirmed, err := confirmAbandon(workspaceName)
		if err != nil {
			return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
				fmt.Errorf("failed to get confirmation: %w", err))
		}

		if !confirmed {
			out.Info("Abandonment canceled")
			return nil
		}
	}

	// Create task engine (minimal - we only need store and logger)
	engine := task.NewEngine(taskStore, nil, task.DefaultEngineConfig(), logger)

	// Abandon the task
	reason := "User requested abandonment"
	if err := engine.Abandon(ctx, currentTask, reason); err != nil {
		return handleAbandonError(outputFormat, w, workspaceName, currentTask.ID,
			fmt.Errorf("failed to abandon task: %w", err))
	}

	// Update workspace status to paused
	if err := wsMgr.UpdateStatus(ctx, workspaceName, constants.WorkspaceStatusPaused); err != nil {
		// Log warning but don't fail - task is already abandoned
		logger.Warn().Err(err).Str("workspace_name", workspaceName).Msg("failed to update workspace status to paused")
	}

	// Handle JSON output format
	if outputFormat == OutputJSON {
		return outputAbandonSuccessJSON(w, workspaceName, currentTask.ID, ws.Branch, ws.WorktreePath)
	}

	// Display success
	tui.DisplayAbandonmentSuccess(out, currentTask, ws)

	return nil
}

// confirmAbandon prompts the user for confirmation before abandoning a task.
func confirmAbandon(workspaceName string) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Abandon task in workspace '%s'?", workspaceName)).
				Description("Branch and worktree will be preserved for manual work.").
				Affirmative("Yes, abandon").
				Negative("No, cancel").
				Value(&confirm),
		),
	)

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
