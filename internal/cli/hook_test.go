// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupHookTestEnv sets up a temporary directory structure for hook tests.
func setupHookTestEnv(t *testing.T, h *domain.Hook) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if h == nil {
		return
	}

	// Create directory structure: ~/.atlas/workspaces/<workspace>/tasks/<task>/hook.json
	atlasDir := filepath.Join(tmpDir, ".atlas")
	wsDir := filepath.Join(atlasDir, "workspaces", h.WorkspaceID)
	taskDir := filepath.Join(wsDir, "tasks", h.TaskID)

	require.NoError(t, os.MkdirAll(taskDir, 0o750))

	// Write hook.json
	hookData, err := json.Marshal(h)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "hook.json"), hookData, 0o600))

	// Create workspace.json so the workspace store can find it
	ws := domain.Workspace{
		Name:   h.WorkspaceID,
		Status: "active",
		Branch: "test-branch",
	}
	wsData, err := json.Marshal(ws)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(wsDir, "workspace.json"), wsData, 0o600))
}

// createTestHook creates a test hook with common defaults.
func createTestHook(taskID, workspaceID string) *domain.Hook {
	now := time.Now().UTC()
	return &domain.Hook{
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		State:       domain.HookStateStepRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
		CurrentStep: &domain.StepContext{
			StepName:    "validation",
			StepIndex:   3,
			Attempt:     1,
			MaxAttempts: 3,
		},
		Checkpoints: []domain.StepCheckpoint{
			{
				CheckpointID: "ckpt-001",
				CreatedAt:    now.Add(-10 * time.Minute),
				Description:  "Initial checkpoint",
				Trigger:      domain.CheckpointTriggerManual,
			},
			{
				CheckpointID: "ckpt-002",
				CreatedAt:    now.Add(-5 * time.Minute),
				Description:  "Git commit checkpoint",
				Trigger:      domain.CheckpointTriggerCommit,
			},
		},
		Receipts: []domain.ValidationReceipt{
			{
				ReceiptID: "rcpt-001",
				StepName:  "lint",
				Command:   "npm run lint",
				ExitCode:  0,
				Duration:  "2.5s",
				Signature: "sig-test-123",
			},
		},
	}
}

// createTestCmd creates a test command with global flags.
func createTestCmd(outputFormat string) *cobra.Command {
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{Output: outputFormat})
	if outputFormat != "" {
		_ = rootCmd.PersistentFlags().Set("output", outputFormat)
	}
	return rootCmd
}

// TestRunHookStatus_TextOutput tests hook status command with text output.
func TestRunHookStatus_TextOutput(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-123", "test-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookStatus(context.Background(), cmd, &buf)
	require.NoError(t, err)

	output := buf.String()

	// Should display hook state info
	assert.Contains(t, output, "step_running", "should show hook state")
	assert.Contains(t, output, "task-123", "should show task ID")
	assert.Contains(t, output, "test-ws", "should show workspace ID")
	assert.Contains(t, output, "validation", "should show current step name")
	assert.Contains(t, output, "ckpt-002", "should show latest checkpoint")
	assert.Contains(t, output, "1 (all valid)", "should show receipt count")
}

// TestRunHookStatus_JSONOutput tests hook status command with JSON output.
func TestRunHookStatus_JSONOutput(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-456", "json-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("json")

	err := runHookStatus(context.Background(), cmd, &buf)
	require.NoError(t, err)

	// Parse JSON output
	var result domain.Hook
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "output should be valid JSON")

	assert.Equal(t, "task-456", result.TaskID)
	assert.Equal(t, "json-ws", result.WorkspaceID)
	assert.Equal(t, domain.HookStateStepRunning, result.State)
	assert.Len(t, result.Checkpoints, 2)
	assert.Len(t, result.Receipts, 1)
}

// TestRunHookStatus_NoHookFound tests hook status when no hook exists.
func TestRunHookStatus_NoHookFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Setup without creating any hook
	setupHookTestEnv(t, nil)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookStatus(context.Background(), cmd, &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrHookNotFound)
}

// TestRunHookStatus_NoHookFound_JSON tests hook status with JSON when no hook exists.
func TestRunHookStatus_NoHookFound_JSON(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	setupHookTestEnv(t, nil)

	var buf bytes.Buffer
	cmd := createTestCmd("json")

	err := runHookStatus(context.Background(), cmd, &buf)

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error structure
	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, false, result["success"])
	assert.Contains(t, result["error"], "no active hook")
}

// TestRunHookCheckpoints_TextOutput tests hook checkpoints with text output.
func TestRunHookCheckpoints_TextOutput(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-789", "ckpt-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookCheckpoints(context.Background(), cmd, &buf)
	require.NoError(t, err)

	output := buf.String()

	// Should display checkpoints table
	assert.Contains(t, output, "Checkpoints for task-789", "should show task ID in header")
	assert.Contains(t, output, "Time", "should show table header")
	assert.Contains(t, output, "Trigger", "should show table header")
	assert.Contains(t, output, "Description", "should show table header")
	assert.Contains(t, output, "manual", "should show checkpoint trigger")
	assert.Contains(t, output, "git_commit", "should show checkpoint trigger")
	assert.Contains(t, output, "Initial checkpoint", "should show checkpoint description")
}

// TestRunHookCheckpoints_JSONOutput tests hook checkpoints with JSON output.
func TestRunHookCheckpoints_JSONOutput(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-json-ckpt", "json-ckpt-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("json")

	err := runHookCheckpoints(context.Background(), cmd, &buf)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "output should be valid JSON")

	assert.InEpsilon(t, float64(2), result["count"], 0.0001)
	checkpoints, ok := result["checkpoints"].([]any)
	require.True(t, ok)
	assert.Len(t, checkpoints, 2)
}

// TestRunHookCheckpoints_Empty tests hook checkpoints when no checkpoints exist.
func TestRunHookCheckpoints_Empty(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-empty", "empty-ckpt-ws")
	h.Checkpoints = nil // Clear checkpoints
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookCheckpoints(context.Background(), cmd, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No checkpoints recorded", "should show empty message")
}

// TestRunHookCheckpoints_Empty_JSON tests hook checkpoints JSON when empty.
func TestRunHookCheckpoints_Empty_JSON(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-empty-json", "empty-json-ws")
	h.Checkpoints = nil
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("json")

	err := runHookCheckpoints(context.Background(), cmd, &buf)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	//nolint:testifylint // InEpsilon doesn't work with zero values
	assert.Equal(t, float64(0), result["count"])
}

// TestRunHookCheckpoints_NoHookFound tests checkpoints when no hook exists.
func TestRunHookCheckpoints_NoHookFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	setupHookTestEnv(t, nil)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookCheckpoints(context.Background(), cmd, &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrHookNotFound)
}

// TestRunHookExport tests hook export command.
func TestRunHookExport(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-export", "export-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text") // Export always outputs JSON regardless of format flag

	err := runHookExport(context.Background(), cmd, &buf)
	require.NoError(t, err)

	// Parse exported JSON
	var result domain.Hook
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "export should produce valid JSON")

	assert.Equal(t, "task-export", result.TaskID)
	assert.Equal(t, "export-ws", result.WorkspaceID)
	assert.Equal(t, domain.HookStateStepRunning, result.State)
	assert.NotNil(t, result.CurrentStep)
	assert.Len(t, result.Checkpoints, 2)
	assert.Len(t, result.Receipts, 1)
}

// TestRunHookExport_Indented tests that export output is indented JSON.
func TestRunHookExport_Indented(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-indent", "indent-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookExport(context.Background(), cmd, &buf)
	require.NoError(t, err)

	// Check that output is indented (contains newlines and spaces)
	output := buf.String()
	assert.Contains(t, output, "\n  ", "output should be indented")
}

// TestRunHookExport_NoHookFound tests export when no hook exists.
func TestRunHookExport_NoHookFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	setupHookTestEnv(t, nil)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookExport(context.Background(), cmd, &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrHookNotFound)
}

// TestRunHookInstall tests hook install command.
func TestRunHookInstall(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-install", "install-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookInstall(context.Background(), cmd, &buf)
	require.NoError(t, err)

	output := buf.String()

	// Should show script and instructions
	assert.Contains(t, output, "#!/bin/sh", "should show shebang")
	assert.Contains(t, output, "ATLAS_HOOK_WRAPPER", "should show hook marker")
	assert.Contains(t, output, "atlas checkpoint", "should show checkpoint command")
	assert.Contains(t, output, "--trigger git_commit", "should show trigger flag")
	assert.Contains(t, output, "post-commit", "should mention post-commit in instructions")
	assert.Contains(t, output, "chmod +x", "should show chmod instruction")
}

// TestRunHookInstall_JSONOutput tests hook install with JSON output.
func TestRunHookInstall_JSONOutput(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	h := createTestHook("task-install-json", "install-json-ws")
	setupHookTestEnv(t, h)

	var buf bytes.Buffer
	cmd := createTestCmd("json")

	err := runHookInstall(context.Background(), cmd, &buf)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Contains(t, result["script"], "#!/bin/sh")
	assert.Contains(t, result["script"], "atlas checkpoint")
	assert.NotEmpty(t, result["instructions"])
}

// TestRunHookInstall_NoHookFound tests install when no hook exists.
func TestRunHookInstall_NoHookFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	setupHookTestEnv(t, nil)

	var buf bytes.Buffer
	cmd := createTestCmd("text")

	err := runHookInstall(context.Background(), cmd, &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrHookNotFound)
}

// TestAddHookCommand tests that hook command group is properly added.
func TestAddHookCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddHookCommand(root)

	// Find the hook command
	hookCmd, _, err := root.Find([]string{"hook"})
	require.NoError(t, err)
	require.NotNil(t, hookCmd)
	assert.Equal(t, "hook", hookCmd.Name())

	// Check subcommands exist
	subcommands := []string{"status", "checkpoints", "install", "verify-receipt", "regenerate", "export"}
	for _, sub := range subcommands {
		cmd, _, err := hookCmd.Find([]string{sub})
		require.NoError(t, err, "subcommand %q should exist", sub)
		require.NotNil(t, cmd)
		assert.Equal(t, sub, cmd.Name())
	}
}

// TestFormatRelativeTime tests the formatRelativeTime helper.
func TestFormatRelativeTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "just now",
			duration: 30 * time.Second,
			want:     "just now",
		},
		{
			name:     "1 minute ago",
			duration: 90 * time.Second,
			want:     "1 minute ago",
		},
		{
			name:     "multiple minutes ago",
			duration: 5 * time.Minute,
			want:     "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			duration: 90 * time.Minute,
			want:     "1 hour ago",
		},
		{
			name:     "multiple hours ago",
			duration: 4 * time.Hour,
			want:     "4 hours ago",
		},
		{
			name:     "1 day ago",
			duration: 30 * time.Hour,
			want:     "1 day ago",
		},
		{
			name:     "multiple days ago",
			duration: 72 * time.Hour,
			want:     "3 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatRelativeTime(time.Now().Add(-tt.duration))
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDisplayHookStatus tests the displayHookStatus helper.
func TestDisplayHookStatus(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	h := createTestHook("display-task", "display-ws")
	displayHookStatus(out, h)

	output := buf.String()
	assert.Contains(t, output, "step_running")
	assert.Contains(t, output, "display-task")
	assert.Contains(t, output, "validation")
	assert.Contains(t, output, "1/3")
	assert.Contains(t, output, "ckpt-002")
	assert.Contains(t, output, "1 (all valid)")
}

// TestDisplayHookCheckpoints tests the displayHookCheckpoints helper.
func TestDisplayHookCheckpoints(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	h := createTestHook("ckpt-task", "ckpt-ws")
	displayHookCheckpoints(out, h)

	output := buf.String()
	assert.Contains(t, output, "ckpt-task")
	assert.Contains(t, output, "Time")
	assert.Contains(t, output, "Trigger")
	assert.Contains(t, output, "Description")
}

// TestDisplayHookCheckpoints_Empty tests displayHookCheckpoints with no checkpoints.
func TestDisplayHookCheckpoints_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	h := createTestHook("empty-task", "empty-ws")
	h.Checkpoints = nil
	displayHookCheckpoints(out, h)

	output := buf.String()
	assert.Contains(t, output, "No checkpoints recorded")
}
