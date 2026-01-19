// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// addWorkspaceCloseCmd adds the close subcommand to the workspace command.
func addWorkspaceCloseCmd(parent *cobra.Command) {
	var force bool

	cmd := &cobra.Command{
		Use:   "close <name>",
		Short: "Close a workspace, preserving history",
		Long: `Archive a completed workspace by removing its git worktree
while preserving all task history and the git branch.

Use this when you're done with a workspace but want to keep the history
for reference. The closed workspace will still appear in 'workspace list'.

Examples:
  atlas workspace close auth          # Confirm and close
  atlas workspace close auth --force  # Close without confirmation`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWorkspaceClose(cmd.Context(), cmd, os.Stdout, args[0], force, "")
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

// runWorkspaceClose executes the workspace close command.
func runWorkspaceClose(ctx context.Context, cmd *cobra.Command, w io.Writer, name string, force bool, storeBaseDir string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	return runWorkspaceCloseWithOutput(ctx, w, name, force, storeBaseDir, output)
}

// runWorkspaceCloseWithOutput executes the workspace close command with explicit output format.
func runWorkspaceCloseWithOutput(ctx context.Context, w io.Writer, name string, force bool, storeBaseDir, output string) error {
	logger := Logger()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Create store and check workspace existence
	store, exists, err := checkWorkspaceExistsForClose(ctx, name, storeBaseDir, output, w)
	if err != nil {
		return err
	}
	if !exists {
		return handleCloseWorkspaceNotFound(name, output, w)
	}

	// Check if workspace is already closed
	ws, err := store.Get(ctx, name)
	if err != nil {
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, fmt.Sprintf("failed to get workspace: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to get workspace '%s': %w", name, err)
	}

	if ws.Status == constants.WorkspaceStatusClosed {
		if output == OutputJSON {
			// Return success for already closed (idempotent)
			return outputCloseSuccessJSON(w, name)
		}
		_, _ = fmt.Fprintf(w, "Workspace '%s' is already closed.\n", name)
		return nil
	}

	// Handle confirmation if needed
	if err := handleCloseConfirmation(name, force, output, w); err != nil {
		return err
	}

	// Execute the close operation
	return executeClose(ctx, store, name, storeBaseDir, output, w, logger)
}

// checkWorkspaceExistsForClose creates the store and checks if the workspace exists.
// Returns (store, exists, error). For JSON output errors, returns ErrJSONErrorOutput.
func checkWorkspaceExistsForClose(ctx context.Context, name, storeBaseDir, output string, w io.Writer) (*workspace.FileStore, bool, error) {
	logger := Logger()

	store, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, fmt.Sprintf("failed to create workspace store: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to create workspace store: %w", err)
	}

	exists, err := store.Exists(ctx, name)
	if err != nil {
		logger.Debug().Err(err).Str("workspace", name).Msg("failed to check workspace existence")
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, fmt.Sprintf("failed to check workspace: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to check workspace '%s': %w", name, err)
	}

	return store, exists, nil
}

// handleCloseWorkspaceNotFound handles the case when a workspace is not found.
func handleCloseWorkspaceNotFound(name, output string, w io.Writer) error {
	if output == OutputJSON {
		// Output JSON error and return sentinel so caller knows to silence cobra's error printing
		_ = outputCloseErrorJSON(w, name, "workspace not found")
		return errors.ErrJSONErrorOutput
	}
	// Match AC5 format exactly: "Workspace 'nonexistent' not found"
	// Wrap sentinel for programmatic error checking
	//nolint:staticcheck // ST1005: AC5 requires capitalized error for user-facing message
	return fmt.Errorf("Workspace '%s' not found: %w", name, errors.ErrWorkspaceNotFound)
}

// handleCloseConfirmation handles the user confirmation flow.
// Returns nil if confirmed or force is true, error otherwise.
func handleCloseConfirmation(name string, force bool, output string, w io.Writer) error {
	if force {
		return nil
	}

	if !terminalCheck() {
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, "cannot close workspace: use --force in non-interactive mode")
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("cannot close workspace '%s': %w", name, errors.ErrNonInteractiveMode)
	}

	confirmed, err := confirmClose(name)
	if err != nil {
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, fmt.Sprintf("failed to get confirmation: %v", err))
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

// executeClose performs the actual close operation.
func executeClose(ctx context.Context, store *workspace.FileStore, name, storeBaseDir, output string, w io.Writer, logger zerolog.Logger) error {
	// Get repo path for worktree runner
	repoPath, err := detectRepoPath()
	if err != nil {
		// If we can't detect repo, worktree operations will fail gracefully
		logger.Debug().Err(err).Msg("could not detect repo path, worktree cleanup may be limited")
		repoPath = ""
	}

	// Create worktree runner (may be nil if no repo path)
	var wtRunner workspace.WorktreeRunner
	if repoPath != "" {
		wtRunner, err = workspace.NewGitWorktreeRunner(ctx, repoPath, logger)
		if err != nil {
			// Log but continue - close should still update state
			logger.Debug().Err(err).Msg("could not create worktree runner, worktree cleanup may be limited")
			wtRunner = nil
		}
	}

	// Create task store to check for running tasks before closing
	// This prevents closing a workspace while tasks are actively running
	var taskLister workspace.TaskLister
	taskStore, taskErr := task.NewFileStore(storeBaseDir)
	if taskErr != nil {
		logger.Debug().Err(taskErr).Msg("could not create task store, running task check will be skipped")
	} else {
		taskLister = taskStore
	}

	// Create manager and close
	mgr := workspace.NewManager(store, wtRunner, logger)

	result, closeErr := mgr.Close(ctx, name, taskLister)
	if closeErr != nil {
		return handleCloseError(w, name, output, closeErr)
	}

	// Get warning messages if worktree or branch removal failed
	var warning string
	if result != nil {
		var warnings []string
		if result.WorktreeWarning != "" {
			warnings = append(warnings, result.WorktreeWarning)
		}
		if result.BranchWarning != "" {
			warnings = append(warnings, result.BranchWarning)
		}
		if len(warnings) > 0 {
			warning = strings.Join(warnings, "; ")
		}
	}

	// Output success with optional warning
	if output == OutputJSON {
		return outputCloseSuccessJSONWithWarning(w, name, warning)
	}

	// Text output: success message first, then warning if any
	if err := outputCloseSuccess(w, name, output); err != nil {
		return fmt.Errorf("output close success: %w", err)
	}
	if warning != "" {
		outputCloseWarning(w, warning, output)
	}

	return nil
}

// handleCloseError handles errors from the close operation.
func handleCloseError(w io.Writer, name, output string, closeErr error) error {
	// Check for running tasks error (AC #3)
	if stderrors.Is(closeErr, errors.ErrWorkspaceHasRunningTasks) {
		if output == OutputJSON {
			_ = outputCloseErrorJSON(w, name, "cannot close workspace with running tasks")
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("cannot close workspace '%s' with running tasks: %w", name, closeErr)
	}

	// Other errors
	if output == OutputJSON {
		_ = outputCloseErrorJSON(w, name, closeErr.Error())
		return errors.ErrJSONErrorOutput
	}
	return fmt.Errorf("failed to close workspace '%s': %w", name, closeErr)
}

// outputCloseSuccess outputs success message in appropriate format.
func outputCloseSuccess(w io.Writer, name, output string) error {
	if output == OutputJSON {
		return outputCloseSuccessJSON(w, name)
	}

	// Use lipgloss for styled success message (AC #2)
	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	_, _ = fmt.Fprintf(w, "%s Workspace '%s' closed. History preserved.\n", checkmark, name)

	return nil
}

// outputCloseWarning outputs a warning message about worktree removal failure.
func outputCloseWarning(w io.Writer, warning, output string) {
	if output == OutputJSON {
		// For JSON output, include warning in a separate line
		if err := json.NewEncoder(w).Encode(map[string]any{
			"type":    "warning",
			"message": warning,
		}); err != nil {
			// Best effort - if JSON encoding fails, fall back to stderr
			_, _ = fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
		}
		return
	}

	// Use lipgloss for styled warning message
	warningIcon := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⚠")
	_, _ = fmt.Fprintf(w, "%s Warning: %s\n", warningIcon, warning)
}

// confirmClose prompts the user for confirmation before closing a workspace.
func confirmClose(name string) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Close workspace '%s'?", name)).
				Description("Worktree will be removed but history preserved.").
				Affirmative("Yes, close").
				Negative("No, cancel").
				Value(&confirm),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// closeResult represents the JSON output for close operations.
type closeResult struct {
	Status           string `json:"status"`
	Workspace        string `json:"workspace"`
	HistoryPreserved bool   `json:"history_preserved,omitempty"`
	Warning          string `json:"warning,omitempty"`
	Error            string `json:"error,omitempty"`
}

// outputCloseSuccessJSON outputs a success result as JSON.
func outputCloseSuccessJSON(w io.Writer, name string) error {
	return outputCloseSuccessJSONWithWarning(w, name, "")
}

// outputCloseSuccessJSONWithWarning outputs a success result as JSON with an optional warning.
func outputCloseSuccessJSONWithWarning(w io.Writer, name, warning string) error {
	result := closeResult{
		Status:           "closed",
		Workspace:        name,
		HistoryPreserved: true,
		Warning:          warning,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputCloseErrorJSON outputs an error result as JSON.
// Returns the encoding error if JSON output fails, which callers typically
// ignore with `_ =` since ErrJSONErrorOutput is already being returned.
func outputCloseErrorJSON(w io.Writer, name, errMsg string) error {
	result := closeResult{
		Status:    "error",
		Workspace: name,
		Error:     errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
