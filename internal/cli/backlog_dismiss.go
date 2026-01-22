package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/mrz1836/atlas/internal/backlog"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/spf13/cobra"
)

// backlogDismissFlags holds the flags for the dismiss command.
type backlogDismissFlags struct {
	reason      string
	json        bool
	projectRoot string // used for testing
}

// newBacklogDismissCmd creates the backlog dismiss command.
func newBacklogDismissCmd() *cobra.Command {
	flags := &backlogDismissFlags{}

	cmd := &cobra.Command{
		Use:   "dismiss <id>",
		Short: "Dismiss a discovery with a reason",
		Long: `Dismiss a discovery with a reason.

Only pending discoveries can be dismissed. The reason is required
to document why the discovery was not addressed.

Examples:
  atlas backlog dismiss disc-abc123 --reason "duplicate of disc-xyz789"
  atlas backlog dismiss disc-abc123 --reason "won't fix"
  atlas backlog dismiss disc-abc123 --reason "already fixed in PR #123"

Exit codes:
  0: Success
  1: Discovery not found or error
  2: Invalid input (discovery not pending, missing reason)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogDismiss(cmd.Context(), cmd, cmd.OutOrStdout(), args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for dismissal (required)")
	cmd.Flags().BoolVar(&flags.json, "json", false, "Output as JSON")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

// runBacklogDismiss executes the backlog dismiss command.
func runBacklogDismiss(ctx context.Context, cmd *cobra.Command, w io.Writer, id string, flags *backlogDismissFlags) error {
	outputFormat := getOutputFormat(cmd, flags.json)
	out := tui.NewOutput(w, outputFormat)

	// Validate reason
	if flags.reason == "" {
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: --reason flag is required", atlaserrors.ErrUserInputRequired))
	}

	// Create manager
	mgr, err := backlog.NewManager(flags.projectRoot)
	if err != nil {
		return outputBacklogError(w, outputFormat, "dismiss", err)
	}

	// Dismiss the discovery
	d, err := mgr.Dismiss(ctx, id, flags.reason)
	if err != nil {
		// Check if this is an invalid transition error
		if atlaserrors.IsExitCode2Error(err) {
			return err
		}
		return outputBacklogError(w, outputFormat, "dismiss", err)
	}

	// Output results
	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"success":   true,
			"id":        d.ID,
			"status":    d.Status,
			"reason":    d.Lifecycle.DismissedReason,
			"discovery": d,
		})
	}

	displayBacklogDismissSuccess(out, d)
	return nil
}

// displayBacklogDismissSuccess displays the success message for dismiss command.
func displayBacklogDismissSuccess(out tui.Output, d *backlog.Discovery) {
	out.Success(fmt.Sprintf("Dismissed discovery %s", d.ID))
	out.Info(fmt.Sprintf("  Reason: %s", d.Lifecycle.DismissedReason))
}
