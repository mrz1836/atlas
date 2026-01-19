package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/backlog"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
)

// newBacklogPromoteCmd creates the backlog promote command.
func newBacklogPromoteCmd() *cobra.Command {
	var (
		taskID     string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "promote <id>",
		Short: "Promote a discovery to a task",
		Long: `Promote a discovery to a task by recording the task ID.

Only pending discoveries can be promoted. This command records
the task ID in the discovery's lifecycle metadata but does not
create the actual task.

Examples:
  atlas backlog promote disc-abc123 --task-id T001
  atlas backlog promote disc-abc123 --task-id task-20260118-150000

Exit codes:
  0: Success
  1: Discovery not found or error
  2: Invalid input (discovery not pending, missing task-id)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogPromote(cmd.Context(), cmd, cmd.OutOrStdout(), args[0], taskID, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&taskID, "task-id", "", "ATLAS task ID to link (required)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	_ = cmd.MarkFlagRequired("task-id")

	return cmd
}

// runBacklogPromote executes the backlog promote command.
func runBacklogPromote(ctx context.Context, cmd *cobra.Command, w io.Writer, id, taskID string, jsonOutput bool) error {
	outputFormat := getOutputFormat(cmd, jsonOutput)
	out := tui.NewOutput(w, outputFormat)

	// Validate task ID
	if taskID == "" {
		return atlaserrors.NewExitCode2Error(
			fmt.Errorf("%w: --task-id flag is required", atlaserrors.ErrUserInputRequired))
	}

	// Create manager
	mgr, err := backlog.NewManager("")
	if err != nil {
		return outputBacklogError(w, outputFormat, "promote", err)
	}

	// Promote the discovery
	d, err := mgr.Promote(ctx, id, taskID)
	if err != nil {
		// Check if this is an invalid transition error
		if atlaserrors.IsExitCode2Error(err) {
			return err
		}
		return outputBacklogError(w, outputFormat, "promote", err)
	}

	// Output results
	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"success":   true,
			"id":        d.ID,
			"status":    d.Status,
			"task_id":   d.Lifecycle.PromotedToTask,
			"discovery": d,
		})
	}

	displayBacklogPromoteSuccess(out, d)
	return nil
}

// displayBacklogPromoteSuccess displays the success message for promote command.
func displayBacklogPromoteSuccess(out tui.Output, d *backlog.Discovery) {
	out.Success(fmt.Sprintf("Promoted discovery %s", d.ID))
	out.Info(fmt.Sprintf("  Linked to task: %s", d.Lifecycle.PromotedToTask))
}
