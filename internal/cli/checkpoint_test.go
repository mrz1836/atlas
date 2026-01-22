// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCheckpointTestCmd creates a test command for checkpoint tests.
func createCheckpointTestCmd(outputFormat string) *cobra.Command {
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{Output: outputFormat})
	if outputFormat != "" {
		_ = rootCmd.PersistentFlags().Set("output", outputFormat)
	}
	return rootCmd
}

// TestRunCheckpoint_NoHookFound tests checkpoint when no hook exists.
func TestRunCheckpoint_NoHookFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Set up empty environment with no hooks
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	cmd := createCheckpointTestCmd("text")

	err := runCheckpoint(context.Background(), cmd, &buf, "Test", "manual")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrHookNotFound)
}

// TestRunCheckpoint_NoHookFound_JSON tests checkpoint JSON when no hook exists.
func TestRunCheckpoint_NoHookFound_JSON(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	cmd := createCheckpointTestCmd("json")

	err := runCheckpoint(context.Background(), cmd, &buf, "Test", "manual")
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, false, result["success"])
}

// TestParseTriggerType tests the parseTriggerType function.
func TestParseTriggerType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected domain.CheckpointTrigger
	}{
		{"manual", domain.CheckpointTriggerManual},
		{"git_commit", domain.CheckpointTriggerCommit},
		{"git_push", domain.CheckpointTriggerPush},
		{"pr_created", domain.CheckpointTriggerPR},
		{"validation", domain.CheckpointTriggerValidation},
		{"step_complete", domain.CheckpointTriggerStepComplete},
		{"interval", domain.CheckpointTriggerInterval},
		{"unknown", domain.CheckpointTriggerManual}, // Default fallback
		{"", domain.CheckpointTriggerManual},        // Empty string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := parseTriggerType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDefaultCheckpointDescription tests the defaultCheckpointDescription function.
func TestDefaultCheckpointDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		trigger  domain.CheckpointTrigger
		expected string
	}{
		{domain.CheckpointTriggerManual, "Manual checkpoint"},
		{domain.CheckpointTriggerCommit, "Git commit checkpoint"},
		{domain.CheckpointTriggerPush, "Git push checkpoint"},
		{domain.CheckpointTriggerPR, "Pull request created"},
		{domain.CheckpointTriggerValidation, "Validation passed"},
		{domain.CheckpointTriggerStepComplete, "Step completed"},
		{domain.CheckpointTriggerInterval, "Interval checkpoint"},
	}

	for _, tt := range tests {
		t.Run(string(tt.trigger), func(t *testing.T) {
			t.Parallel()
			result := defaultCheckpointDescription(tt.trigger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAddCheckpointCommand tests that checkpoint command is properly added.
func TestAddCheckpointCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddCheckpointCommand(root)

	// Find the checkpoint command
	checkpointCmd, _, err := root.Find([]string{"checkpoint"})
	require.NoError(t, err)
	require.NotNil(t, checkpointCmd)
	assert.Equal(t, "checkpoint", checkpointCmd.Name())

	// Check --trigger flag exists
	triggerFlag := checkpointCmd.Flags().Lookup("trigger")
	require.NotNil(t, triggerFlag)
	assert.Equal(t, "manual", triggerFlag.DefValue)
}
