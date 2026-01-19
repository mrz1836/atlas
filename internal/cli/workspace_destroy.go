// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// addWorkspaceDestroyCmd adds the destroy subcommand to the workspace command.
func addWorkspaceDestroyCmd(parent *cobra.Command) {
	var force bool

	cmd := &cobra.Command{
		Use:   "destroy <name>",
		Short: "Destroy a workspace and its worktree",
		Long: `Completely remove a workspace including its git worktree,
branch, and all associated state files.

This operation cannot be undone. Use --force to skip confirmation.

Examples:
  atlas workspace destroy payment           # Confirm and destroy
  atlas workspace destroy payment --force   # Destroy without confirmation`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWorkspaceDestroy(cmd.Context(), cmd, os.Stdout, args[0], force, "")
			// If JSON error was already output, silence cobra's error printing
			// but still return error for non-zero exit code
			if stderrors.Is(err, errors.ErrJSONErrorOutput) {
				cmd.SilenceErrors = true
			}
			return err
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	parent.AddCommand(cmd)
}

// runWorkspaceDestroy executes the workspace destroy command.
func runWorkspaceDestroy(ctx context.Context, cmd *cobra.Command, w io.Writer, name string, force bool, storeBaseDir string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	return runWorkspaceDestroyWithOutput(ctx, w, name, force, storeBaseDir, output)
}

// runWorkspaceDestroyWithOutput executes the workspace destroy command with explicit output format.
func runWorkspaceDestroyWithOutput(ctx context.Context, w io.Writer, name string, force bool, storeBaseDir, output string) error {
	logger := Logger()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Create store and check workspace existence
	store, exists, err := checkWorkspaceExists(ctx, name, storeBaseDir, output, w)
	if err != nil {
		return err
	}
	if !exists {
		return handleWorkspaceNotFound(name, output, w)
	}

	// Handle confirmation if needed
	if err := handleConfirmation(name, force, output, w); err != nil {
		return err
	}

	// Execute the destroy operation
	return executeDestroy(ctx, store, name, output, w, logger)
}

// checkWorkspaceExists creates the store and checks if the workspace exists.
// Returns (store, exists, error). For JSON output errors, returns ErrJSONErrorOutput.
func checkWorkspaceExists(ctx context.Context, name, storeBaseDir, output string, w io.Writer) (*workspace.FileStore, bool, error) {
	logger := Logger()

	store, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		if output == OutputJSON {
			_ = outputDestroyErrorJSON(w, name, fmt.Sprintf("failed to create workspace store: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to create workspace store: %w", err)
	}

	exists, err := store.Exists(ctx, name)
	if err != nil {
		logger.Debug().Err(err).Str("workspace", name).Msg("failed to check workspace existence")
		if output == OutputJSON {
			_ = outputDestroyErrorJSON(w, name, fmt.Sprintf("failed to check workspace: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to check workspace '%s': %w", name, err)
	}

	return store, exists, nil
}

// handleWorkspaceNotFound handles the case when a workspace is not found.
func handleWorkspaceNotFound(name, output string, w io.Writer) error {
	if output == OutputJSON {
		// Output JSON error and return sentinel so caller knows to silence cobra's error printing
		_ = outputDestroyErrorJSON(w, name, "workspace not found")
		return errors.ErrJSONErrorOutput
	}
	// Match AC5 format exactly: "Workspace 'nonexistent' not found"
	// Wrap sentinel for programmatic error checking
	//nolint:staticcheck // ST1005: AC5 requires capitalized error for user-facing message
	return fmt.Errorf("Workspace '%s' not found: %w", name, errors.ErrWorkspaceNotFound)
}

// handleConfirmation handles the user confirmation flow.
// Returns nil if confirmed or force is true, error otherwise.
func handleConfirmation(name string, force bool, output string, w io.Writer) error {
	if force {
		return nil
	}

	if !terminalCheck() {
		if output == OutputJSON {
			_ = outputDestroyErrorJSON(w, name, "cannot destroy workspace: use --force in non-interactive mode")
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("cannot destroy workspace '%s': %w", name, errors.ErrNonInteractiveMode)
	}

	confirmed, err := confirmDestroy(name)
	if err != nil {
		if output == OutputJSON {
			_ = outputDestroyErrorJSON(w, name, fmt.Sprintf("failed to get confirmation: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to get confirmation: %w", err)
	}

	if !confirmed {
		_, _ = fmt.Fprintln(w, "Operation canceled.")
		return nil
	}

	return nil
}

// executeDestroy performs the actual destroy operation.
func executeDestroy(ctx context.Context, store *workspace.FileStore, name, output string, w io.Writer, logger zerolog.Logger) error {
	// Get workspace first to store path/branch info for better error reporting
	ws, wsErr := store.Get(ctx, name)
	var worktreePath, branch string
	if wsErr == nil && ws != nil {
		worktreePath = ws.WorktreePath
		branch = ws.Branch
	}

	// Delete linked backlog discoveries before destroying workspace (best-effort)
	deleteLinkedDiscoveries(ctx, name, logger)

	// Get repo path for worktree runner
	repoPath, err := detectRepoPath()
	if err != nil {
		logger.Warn().Err(err).Msg("could not detect repo path, worktree cleanup will be skipped")
		// Show warning to user if we have worktree info
		if worktreePath != "" {
			logger.Warn().
				Str("worktree_path", worktreePath).
				Str("branch", branch).
				Msg("manual cleanup may be required: run 'git worktree remove --force <path>' and 'git branch -D <branch>'")
		}
		repoPath = ""
	}

	// Create worktree runner (may be nil if no repo path)
	var wtRunner workspace.WorktreeRunner
	if repoPath != "" {
		wtRunner, err = workspace.NewGitWorktreeRunner(ctx, repoPath, logger)
		if err != nil {
			// Log but continue - destroy should still clean up state
			logger.Warn().Err(err).Msg("could not create worktree runner, worktree cleanup will be limited")
			wtRunner = nil
		}
	}

	// Create manager and destroy
	mgr := workspace.NewManager(store, wtRunner, logger)

	if destroyErr := mgr.Destroy(ctx, name); destroyErr != nil {
		// This should never happen per NFR18, but handle just in case
		if output == OutputJSON {
			return outputDestroyErrorJSON(w, name, destroyErr.Error())
		}
		return fmt.Errorf("failed to destroy workspace '%s': %w", name, destroyErr)
	}

	// If we couldn't create worktree runner, add helpful message
	if wtRunner == nil && worktreePath != "" {
		showManualCleanupWarning(w, output, worktreePath, branch, logger)
	}

	// Output success
	if output == OutputJSON {
		return outputDestroySuccessJSON(w, name)
	}

	// Use lipgloss for styled success message
	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	_, _ = fmt.Fprintf(w, "%s Workspace '%s' destroyed\n", checkmark, name)

	return nil
}

// showManualCleanupWarning displays manual cleanup instructions when worktree runner is unavailable.
func showManualCleanupWarning(w io.Writer, output, worktreePath, branch string, logger zerolog.Logger) {
	logger.Warn().Msg("workspace state deleted, but worktree cleanup was limited")
	if output == OutputJSON {
		return
	}

	_, _ = fmt.Fprintf(w, "\n⚠️  Manual cleanup may be required:\n")
	if worktreePath != "" {
		_, _ = fmt.Fprintf(w, "   git worktree remove --force %s\n", worktreePath)
	}
	if branch != "" {
		_, _ = fmt.Fprintf(w, "   git branch -D %s\n", branch)
	}
	_, _ = fmt.Fprintf(w, "\n")
}

// confirmDestroy prompts the user for confirmation before destroying a workspace.
func confirmDestroy(name string) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete workspace '%s'?", name)).
				Description("This cannot be undone.").
				Affirmative("Yes, delete").
				Negative("No, cancel").
				Value(&confirm),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// terminalCheck is a variable for the terminal check function, allowing tests to override it.
//
//nolint:gochecknoglobals // Required for test injection of terminal detection
var terminalCheck = isTerminal

// isTerminal returns true if stdin is a terminal.
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// detectRepoPath finds the git repository root from the current working directory.
func detectRepoPath() (string, error) {
	// Try current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if it's a git repo by looking for .git
	// Walk up directory tree
	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return "", errors.ErrNotGitRepo
}

// destroyResult represents the JSON output for destroy operations.
type destroyResult struct {
	Status    string `json:"status"`
	Workspace string `json:"workspace"`
	Error     string `json:"error,omitempty"`
}

// outputDestroySuccessJSON outputs a success result as JSON.
func outputDestroySuccessJSON(w io.Writer, name string) error {
	result := destroyResult{
		Status:    "destroyed",
		Workspace: name,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputDestroyErrorJSON outputs an error result as JSON.
// Returns the encoding error if JSON output fails, which callers typically
// ignore with `_ =` since ErrJSONErrorOutput is already being returned.
// This is intentional: if we can't write JSON, there's no useful fallback,
// and the caller's return of ErrJSONErrorOutput signals to cobra to suppress
// its own error printing regardless of whether our JSON succeeded.
func outputDestroyErrorJSON(w io.Writer, name, errMsg string) error {
	result := destroyResult{
		Status:    "error",
		Workspace: name,
		Error:     errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// deleteLinkedDiscoveries removes backlog discovery files linked to tasks in the workspace.
// This is a best-effort operation - failures are logged but don't prevent workspace destruction.
// Git history provides the audit trail for deleted discoveries.
func deleteLinkedDiscoveries(ctx context.Context, workspaceName string, logger zerolog.Logger) {
	// Create task store to list tasks for the workspace
	taskStore, err := task.NewFileStore("")
	if err != nil {
		logger.Debug().Err(err).Msg("could not create task store for discovery cleanup")
		return
	}

	// List all tasks in the workspace
	tasks, err := taskStore.List(ctx, workspaceName)
	if err != nil {
		logger.Debug().Err(err).
			Str("workspace_name", workspaceName).
			Msg("could not list tasks for discovery cleanup")
		return
	}

	// Create backlog manager
	backlogMgr, err := backlog.NewManager("")
	if err != nil {
		logger.Debug().Err(err).Msg("could not create backlog manager for discovery cleanup")
		return
	}

	// Check each task for linked discoveries and delete them
	for _, t := range tasks {
		if t.Metadata == nil {
			continue
		}

		backlogID, ok := t.Metadata["from_backlog_id"].(string)
		if !ok || backlogID == "" {
			continue
		}

		// Delete the discovery file (best-effort)
		if deleteErr := backlogMgr.Delete(ctx, backlogID); deleteErr != nil {
			logger.Debug().Err(deleteErr).
				Str("discovery_id", backlogID).
				Str("task_id", t.ID).
				Msg("could not delete linked discovery")
		} else {
			logger.Info().
				Str("discovery_id", backlogID).
				Str("task_id", t.ID).
				Str("workspace_name", workspaceName).
				Msg("deleted linked backlog discovery")
		}
	}
}
