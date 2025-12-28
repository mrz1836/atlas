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
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// addWorkspaceRetireCmd adds the retire subcommand to the workspace command.
func addWorkspaceRetireCmd(parent *cobra.Command) {
	var force bool

	cmd := &cobra.Command{
		Use:   "retire <name>",
		Short: "Retire a workspace, preserving history",
		Long: `Archive a completed workspace by removing its git worktree
while preserving all task history and the git branch.

Use this when you're done with a workspace but want to keep the history
for reference. The retired workspace will still appear in 'workspace list'.

Examples:
  atlas workspace retire auth          # Confirm and retire
  atlas workspace retire auth --force  # Retire without confirmation`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWorkspaceRetire(cmd.Context(), cmd, os.Stdout, args[0], force, "")
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

// runWorkspaceRetire executes the workspace retire command.
func runWorkspaceRetire(ctx context.Context, cmd *cobra.Command, w io.Writer, name string, force bool, storeBaseDir string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	return runWorkspaceRetireWithOutput(ctx, w, name, force, storeBaseDir, output)
}

// runWorkspaceRetireWithOutput executes the workspace retire command with explicit output format.
func runWorkspaceRetireWithOutput(ctx context.Context, w io.Writer, name string, force bool, storeBaseDir, output string) error {
	logger := GetLogger()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Create store and check workspace existence
	store, exists, err := checkWorkspaceExistsForRetire(ctx, name, storeBaseDir, output, w)
	if err != nil {
		return err
	}
	if !exists {
		return handleRetireWorkspaceNotFound(name, output, w)
	}

	// Check if workspace is already retired
	ws, err := store.Get(ctx, name)
	if err != nil {
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, fmt.Sprintf("failed to get workspace: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to get workspace '%s': %w", name, err)
	}

	if ws.Status == constants.WorkspaceStatusRetired {
		if output == OutputJSON {
			// Return success for already retired (idempotent)
			return outputRetireSuccessJSON(w, name)
		}
		_, _ = fmt.Fprintf(w, "Workspace '%s' is already retired.\n", name)
		return nil
	}

	// Handle confirmation if needed
	if err := handleRetireConfirmation(name, force, output, w); err != nil {
		return err
	}

	// Execute the retire operation
	return executeRetire(ctx, store, name, output, w, logger)
}

// checkWorkspaceExistsForRetire creates the store and checks if the workspace exists.
// Returns (store, exists, error). For JSON output errors, returns ErrJSONErrorOutput.
func checkWorkspaceExistsForRetire(ctx context.Context, name, storeBaseDir, output string, w io.Writer) (*workspace.FileStore, bool, error) {
	logger := GetLogger()

	store, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, fmt.Sprintf("failed to create workspace store: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to create workspace store: %w", err)
	}

	exists, err := store.Exists(ctx, name)
	if err != nil {
		logger.Debug().Err(err).Str("workspace", name).Msg("failed to check workspace existence")
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, fmt.Sprintf("failed to check workspace: %v", err))
			return nil, false, errors.ErrJSONErrorOutput
		}
		return nil, false, fmt.Errorf("failed to check workspace '%s': %w", name, err)
	}

	return store, exists, nil
}

// handleRetireWorkspaceNotFound handles the case when a workspace is not found.
func handleRetireWorkspaceNotFound(name, output string, w io.Writer) error {
	if output == OutputJSON {
		// Output JSON error and return sentinel so caller knows to silence cobra's error printing
		_ = outputRetireErrorJSON(w, name, "workspace not found")
		return errors.ErrJSONErrorOutput
	}
	// Match AC5 format exactly: "Workspace 'nonexistent' not found"
	// Wrap sentinel for programmatic error checking
	//nolint:staticcheck // ST1005: AC5 requires capitalized error for user-facing message
	return fmt.Errorf("Workspace '%s' not found: %w", name, errors.ErrWorkspaceNotFound)
}

// handleRetireConfirmation handles the user confirmation flow.
// Returns nil if confirmed or force is true, error otherwise.
func handleRetireConfirmation(name string, force bool, output string, w io.Writer) error {
	if force {
		return nil
	}

	if !terminalCheck() {
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, "cannot retire workspace: use --force in non-interactive mode")
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("cannot retire workspace '%s': %w", name, errors.ErrNonInteractiveMode)
	}

	confirmed, err := confirmRetire(name)
	if err != nil {
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, fmt.Sprintf("failed to get confirmation: %v", err))
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

// executeRetire performs the actual retire operation.
func executeRetire(ctx context.Context, store *workspace.FileStore, name, output string, w io.Writer, logger zerolog.Logger) error {
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
		//nolint:contextcheck // NewGitWorktreeRunner doesn't take context; it only detects repo root
		wtRunner, err = workspace.NewGitWorktreeRunner(repoPath)
		if err != nil {
			// Log but continue - retire should still update state
			logger.Debug().Err(err).Msg("could not create worktree runner, worktree cleanup may be limited")
			wtRunner = nil
		}
	}

	// Create manager and retire
	mgr := workspace.NewManager(store, wtRunner)

	if retireErr := mgr.Retire(ctx, name); retireErr != nil {
		return handleRetireError(w, name, output, retireErr)
	}

	// Output success
	return outputRetireSuccess(w, name, output)
}

// handleRetireError handles errors from the retire operation.
func handleRetireError(w io.Writer, name, output string, retireErr error) error {
	// Check for running tasks error (AC #3)
	if stderrors.Is(retireErr, errors.ErrWorkspaceHasRunningTasks) {
		if output == OutputJSON {
			_ = outputRetireErrorJSON(w, name, "cannot retire workspace with running tasks")
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("cannot retire workspace '%s' with running tasks: %w", name, retireErr)
	}

	// Other errors
	if output == OutputJSON {
		_ = outputRetireErrorJSON(w, name, retireErr.Error())
		return errors.ErrJSONErrorOutput
	}
	return fmt.Errorf("failed to retire workspace '%s': %w", name, retireErr)
}

// outputRetireSuccess outputs success message in appropriate format.
func outputRetireSuccess(w io.Writer, name, output string) error {
	if output == OutputJSON {
		return outputRetireSuccessJSON(w, name)
	}

	// Use lipgloss for styled success message (AC #2)
	checkmark := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")
	_, _ = fmt.Fprintf(w, "%s Workspace '%s' retired. History preserved.\n", checkmark, name)

	return nil
}

// confirmRetire prompts the user for confirmation before retiring a workspace.
func confirmRetire(name string) (bool, error) {
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Retire workspace '%s'?", name)).
				Description("Worktree will be removed but history preserved.").
				Affirmative("Yes, retire").
				Negative("No, cancel").
				Value(&confirm),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// retireResult represents the JSON output for retire operations.
type retireResult struct {
	Status           string `json:"status"`
	Workspace        string `json:"workspace"`
	HistoryPreserved bool   `json:"history_preserved,omitempty"`
	Error            string `json:"error,omitempty"`
}

// outputRetireSuccessJSON outputs a success result as JSON.
func outputRetireSuccessJSON(w io.Writer, name string) error {
	result := retireResult{
		Status:           "retired",
		Workspace:        name,
		HistoryPreserved: true,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputRetireErrorJSON outputs an error result as JSON.
// Returns nil because the error is encoded in the JSON response.
func outputRetireErrorJSON(w io.Writer, name, errMsg string) error {
	result := retireResult{
		Status:    "error",
		Workspace: name,
		Error:     errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
