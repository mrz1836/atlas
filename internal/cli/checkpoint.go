// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// AddCheckpointCommand adds the checkpoint command to the root command.
func AddCheckpointCommand(root *cobra.Command) {
	root.AddCommand(newCheckpointCmd())
}

// newCheckpointCmd creates the checkpoint command for manual checkpoint creation.
func newCheckpointCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "checkpoint [description]",
		Short: "Create a manual checkpoint of current task state",
		Long: `Create a manual checkpoint for the current task.

Checkpoints capture the current state of task execution, including:
- Current step progress
- Git branch and commit state
- File snapshots

Manual checkpoints are useful for marking significant milestones
during task execution. They can be used for recovery if needed.

Examples:
  atlas checkpoint "Completed initial analysis"
  atlas checkpoint "Ready for review"

Exit codes:
  0: Checkpoint created successfully
  1: No active task found
  2: Failed to create checkpoint`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := ""
			if len(args) > 0 {
				description = args[0]
			}
			return runCheckpoint(cmd.Context(), cmd, os.Stdout, description)
		},
	}
}

// runCheckpoint executes the checkpoint command.
func runCheckpoint(ctx context.Context, cmd *cobra.Command, w io.Writer, description string) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	// Get base path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	baseDir := filepath.Join(homeDir, constants.AtlasHome)

	// Find active hook
	hookPath, taskID, workspaceID, err := findActiveHookPath(ctx, baseDir)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputCheckpointErrorJSON(w, err.Error())
		}
		return err
	}

	// Create hook store
	hookStore := hook.NewFileStore(baseDir)

	// Get the hook
	h, err := hookStore.Get(ctx, taskID)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputCheckpointErrorJSON(w, fmt.Sprintf("failed to get hook: %v", err))
		}
		return fmt.Errorf("failed to get hook: %w", err)
	}

	// Create the checkpoint
	now := time.Now().UTC()
	checkpointID := "ckpt-" + uuid.New().String()[:8]

	if description == "" {
		description = "Manual checkpoint"
	}

	checkpoint := domain.StepCheckpoint{
		CheckpointID: checkpointID,
		CreatedAt:    now,
		Description:  description,
		Trigger:      domain.CheckpointTriggerManual,
	}

	// Add current step info if available
	if h.CurrentStep != nil {
		checkpoint.StepName = h.CurrentStep.StepName
		checkpoint.StepIndex = h.CurrentStep.StepIndex
	}

	// Add to checkpoints
	h.Checkpoints = append(h.Checkpoints, checkpoint)
	h.UpdatedAt = now

	// Prune if over limit
	if len(h.Checkpoints) > 50 {
		h.Checkpoints = h.Checkpoints[len(h.Checkpoints)-50:]
	}

	// Record in history
	h.History = append(h.History, domain.HookEvent{
		Timestamp: now,
		FromState: h.State,
		ToState:   h.State,
		Trigger:   "manual_checkpoint",
		Details: map[string]any{
			"checkpoint_id": checkpointID,
			"description":   description,
		},
	})

	// Save the hook
	if err := hookStore.Save(ctx, h); err != nil {
		if outputFormat == OutputJSON {
			return outputCheckpointErrorJSON(w, fmt.Sprintf("failed to save checkpoint: %v", err))
		}
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"success":       true,
			"checkpoint_id": checkpointID,
			"description":   description,
			"task_id":       taskID,
			"workspace_id":  workspaceID,
			"created_at":    now.Format(time.RFC3339),
		})
	}

	out.Success(fmt.Sprintf("Checkpoint created: %s", checkpointID))
	out.Info(fmt.Sprintf("  Description: %s", description))
	out.Info(fmt.Sprintf("  Task: %s", taskID))
	out.Info(fmt.Sprintf("  Hook: %s", hookPath))
	return nil
}

// findActiveHookPath finds the path to an active hook and returns relevant IDs.
func findActiveHookPath(ctx context.Context, baseDir string) (hookPath, taskID, workspaceID string, err error) {
	// Get workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Find active workspaces
	workspaces, err := wsStore.List(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Look for hook.json in active workspaces
	for _, ws := range workspaces {
		if ws.Status == "closed" {
			continue
		}

		// Check for hook.json in this workspace's tasks
		tasksDir := filepath.Join(baseDir, "workspaces", ws.Name, "tasks")
		entries, err := os.ReadDir(tasksDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			hp := filepath.Join(tasksDir, entry.Name(), "hook.json")
			if _, statErr := os.Stat(hp); statErr == nil {
				return hp, entry.Name(), ws.Name, nil
			}
		}
	}

	return "", "", "", fmt.Errorf("%w: no active task with hook found", atlaserrors.ErrHookNotFound)
}

// outputCheckpointErrorJSON outputs an error result as JSON.
func outputCheckpointErrorJSON(w io.Writer, errMsg string) error {
	out := tui.NewOutput(w, OutputJSON)
	if err := out.JSON(map[string]any{
		"success": false,
		"error":   errMsg,
	}); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}
