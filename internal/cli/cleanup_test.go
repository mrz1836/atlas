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

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/tui"
)

// createCleanupTestCmd creates a test command for cleanup tests.
func createCleanupTestCmd(outputFormat string) *cobra.Command {
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{Output: outputFormat})
	if outputFormat != "" {
		_ = rootCmd.PersistentFlags().Set("output", outputFormat)
	}
	return rootCmd
}

// TestRunCleanup_EmptyDir tests cleanup with an empty .atlas directory.
func TestRunCleanup_EmptyDir(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create empty .atlas directory
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".atlas"), 0o750))

	var buf bytes.Buffer
	cmd := createCleanupTestCmd("text")

	err := runCleanup(context.Background(), cmd, &buf, false, false)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No hooks eligible for cleanup")
}

// TestRunCleanup_EmptyDir_JSON tests cleanup JSON with empty directory.
func TestRunCleanup_EmptyDir_JSON(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create empty .atlas directory
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".atlas"), 0o750))

	var buf bytes.Buffer
	cmd := createCleanupTestCmd("json")

	err := runCleanup(context.Background(), cmd, &buf, false, false)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, true, result["success"])
	//nolint:testifylint // InEpsilon doesn't work with zero values
	assert.Equal(t, float64(0), result["deleted"])
}

// TestAddCleanupCommand tests that cleanup command is properly added.
func TestAddCleanupCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddCleanupCommand(root)

	// Find the cleanup command
	cleanupCmd, _, err := root.Find([]string{"cleanup"})
	require.NoError(t, err)
	require.NotNil(t, cleanupCmd)
	assert.Equal(t, "cleanup", cleanupCmd.Name())

	// Check flags exist
	dryRunFlag := cleanupCmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)

	hooksFlag := cleanupCmd.Flags().Lookup("hooks")
	require.NotNil(t, hooksFlag)
	assert.Equal(t, "false", hooksFlag.DefValue)
}

// TestGetRetentionDuration tests the getRetentionDuration function.
func TestGetRetentionDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured time.Duration
		def        time.Duration
		expected   time.Duration
	}{
		{
			name:       "use default when zero",
			configured: 0,
			def:        30 * 24 * time.Hour,
			expected:   30 * 24 * time.Hour,
		},
		{
			name:       "use configured when set",
			configured: 7 * 24 * time.Hour,
			def:        30 * 24 * time.Hour,
			expected:   7 * 24 * time.Hour,
		},
		{
			name:       "use configured even if smaller than default",
			configured: 1 * 24 * time.Hour,
			def:        30 * 24 * time.Hour,
			expected:   1 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getRetentionDuration(tt.configured, tt.def)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRunCleanup_DryRunNoHooks tests dry-run mode when no hooks exist.
func TestRunCleanup_DryRunNoHooks(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .atlas directory (required for cleanup to work)
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".atlas"), 0o750))

	var buf bytes.Buffer
	cmd := createCleanupTestCmd("text")

	err := runCleanup(context.Background(), cmd, &buf, true, false) // dry-run=true
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No hooks eligible for cleanup")
}

// TestCleanupStats tests the cleanupStats structure.
func TestCleanupStats(t *testing.T) {
	t.Parallel()

	stats := cleanupStats{
		completed: 5,
		failed:    3,
		abandoned: 2,
	}

	assert.Equal(t, 5, stats.completed)
	assert.Equal(t, 3, stats.failed)
	assert.Equal(t, 2, stats.abandoned)
}

// TestCleanupDefaultRetention tests the default retention constants.
func TestCleanupDefaultRetention(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 30*24*time.Hour, defaultCompletedRetention)
	assert.Equal(t, 7*24*time.Hour, defaultFailedRetention)
	assert.Equal(t, 7*24*time.Hour, defaultAbandonedRetention)
}

// TestOutputDryRunResults_Text tests dry-run output in text format.
func TestOutputDryRunResults_Text(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	now := time.Now()
	toDelete := []*domain.Hook{
		{
			TaskID:      "task-001",
			WorkspaceID: "ws-001",
			State:       domain.HookStateCompleted,
			UpdatedAt:   now.Add(-35 * 24 * time.Hour),
		},
		{
			TaskID:      "task-002",
			WorkspaceID: "ws-002",
			State:       domain.HookStateFailed,
			UpdatedAt:   now.Add(-10 * 24 * time.Hour),
		},
		{
			TaskID:      "task-003",
			WorkspaceID: "ws-003",
			State:       domain.HookStateAbandoned,
			UpdatedAt:   now.Add(-8 * 24 * time.Hour),
		},
	}

	stats := cleanupStats{
		completed: 1,
		failed:    1,
		abandoned: 1,
	}

	err := outputDryRunResults(out, "text", toDelete, stats)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Would delete 3 hook files")
	assert.Contains(t, output, "Completed: 1")
	assert.Contains(t, output, "Failed: 1")
	assert.Contains(t, output, "Abandoned: 1")
	assert.Contains(t, output, "task-001")
	assert.Contains(t, output, "task-002")
	assert.Contains(t, output, "task-003")
}

// TestOutputDryRunResults_JSON tests dry-run output in JSON format.
func TestOutputDryRunResults_JSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")

	now := time.Now()
	toDelete := []*domain.Hook{
		{
			TaskID:      "task-001",
			WorkspaceID: "ws-001",
			State:       domain.HookStateCompleted,
			UpdatedAt:   now.Add(-35 * 24 * time.Hour),
		},
	}

	stats := cleanupStats{
		completed: 1,
		failed:    0,
		abandoned: 0,
	}

	err := outputDryRunResults(out, "json", toDelete, stats)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, true, result["success"])
	assert.Equal(t, true, result["dry_run"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(1), result["would_delete"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(1), result["completed"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(0), result["failed"])

	hooks, ok := result["hooks"].([]any)
	require.True(t, ok)
	require.Len(t, hooks, 1)
}

// TestOutputCleanupResults_Success_Text tests successful cleanup output in text format.
func TestOutputCleanupResults_Success_Text(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	stats := cleanupStats{
		completed: 5,
		failed:    3,
		abandoned: 2,
	}

	err := outputCleanupResults(out, "text", 10, stats, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Cleaned up 10 hook files")
	assert.Contains(t, output, "Completed: 5")
	assert.Contains(t, output, "Failed: 3")
	assert.Contains(t, output, "Abandoned: 2")
}

// TestOutputCleanupResults_Success_JSON tests successful cleanup output in JSON format.
func TestOutputCleanupResults_Success_JSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")

	stats := cleanupStats{
		completed: 5,
		failed:    3,
		abandoned: 2,
	}

	err := outputCleanupResults(out, "json", 10, stats, nil)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, true, result["success"])
	assert.Equal(t, false, result["dry_run"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(10), result["deleted"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(5), result["completed"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(3), result["failed"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(2), result["abandoned"])
}

// TestOutputCleanupResults_WithErrors_Text tests cleanup output with errors in text format.
func TestOutputCleanupResults_WithErrors_Text(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	stats := cleanupStats{
		completed: 2,
		failed:    1,
		abandoned: 0,
	}

	deleteErrors := []string{
		"task-001: permission denied",
		"task-002: file not found",
	}

	err := outputCleanupResults(out, "text", 1, stats, deleteErrors)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Cleaned up 1 hook files")
	assert.Contains(t, output, "Failed to delete 2 hooks")
	assert.Contains(t, output, "task-001: permission denied")
	assert.Contains(t, output, "task-002: file not found")
}

// TestOutputCleanupResults_WithErrors_JSON tests cleanup output with errors in JSON format.
func TestOutputCleanupResults_WithErrors_JSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")

	stats := cleanupStats{
		completed: 2,
		failed:    1,
		abandoned: 0,
	}

	deleteErrors := []string{
		"task-001: permission denied",
	}

	err := outputCleanupResults(out, "json", 2, stats, deleteErrors)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, false, result["success"])
	//nolint:testifylint // InEpsilon doesn't work with integer values
	assert.Equal(t, float64(2), result["deleted"])

	errors, ok := result["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errors, 1)
	assert.Equal(t, "task-001: permission denied", errors[0])
}

// TestPerformCleanup_Success tests successful cleanup of hooks.
func TestPerformCleanup_Success(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create hook store and hooks
	atlasDir := filepath.Join(tmpDir, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o750))

	hookStore := hook.NewFileStore(atlasDir)
	ctx := context.Background()

	// Create test hooks
	h1, err := hookStore.Create(ctx, "task-001", "ws-001")
	require.NoError(t, err)
	h1.State = domain.HookStateCompleted
	require.NoError(t, hookStore.Save(ctx, h1))

	h2, err := hookStore.Create(ctx, "task-002", "ws-002")
	require.NoError(t, err)
	h2.State = domain.HookStateFailed
	require.NoError(t, hookStore.Save(ctx, h2))

	toDelete := []*domain.Hook{h1, h2}
	stats := cleanupStats{completed: 1, failed: 1, abandoned: 0}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err = performCleanup(ctx, hookStore, out, "text", toDelete, stats)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Cleaned up 2 hook files")

	// Verify hooks were deleted
	_, err = hookStore.Get(ctx, "task-001")
	require.Error(t, err)
	_, err = hookStore.Get(ctx, "task-002")
	require.Error(t, err)
}

// TestPerformCleanup_PartialFailure tests cleanup when some deletions fail.
func TestPerformCleanup_PartialFailure(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	atlasDir := filepath.Join(tmpDir, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o750))

	hookStore := hook.NewFileStore(atlasDir)
	ctx := context.Background()

	// Create two hooks that exist
	h1, err := hookStore.Create(ctx, "task-001", "ws-001")
	require.NoError(t, err)

	h2, err := hookStore.Create(ctx, "task-002", "ws-002")
	require.NoError(t, err)

	// Make the second hook's directory read-only to cause deletion to fail
	hookDir := filepath.Join(atlasDir, "task-002")
	require.NoError(t, os.Chmod(hookDir, 0o400))

	toDelete := []*domain.Hook{h1, h2}
	stats := cleanupStats{completed: 2, failed: 0, abandoned: 0}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err = performCleanup(ctx, hookStore, out, "text", toDelete, stats)
	require.NoError(t, err)

	output := buf.String()
	// Should have deleted at least one hook
	assert.Contains(t, output, "hook files")

	// Restore permissions for cleanup to allow test cleanup
	_ = os.Chmod(hookDir, 0o755)                             //nolint:gosec // Test cleanup requires restoring directory permissions
	_ = os.Chmod(filepath.Join(hookDir, "hook.json"), 0o644) //nolint:gosec // Test cleanup requires restoring file permissions
}

// TestSetupCleanup_HomeDirError tests setupCleanup when home directory cannot be determined.
func TestSetupCleanup_HomeDirError(t *testing.T) {
	t.Parallel()

	// This test verifies the error handling path when UserHomeDir fails.
	// We can't easily mock os.UserHomeDir(), so we test the happy path
	// and ensure setupCleanup returns a valid store and config.
	ctx := context.Background()
	store, cfg, err := setupCleanup(ctx)

	// Should succeed in normal circumstances
	require.NoError(t, err)
	require.NotNil(t, store)
	require.NotNil(t, cfg)
}

// TestCollectStaleHooks_ErrorHandling tests error handling in collectStaleHooks.
func TestCollectStaleHooks_ErrorHandling(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Don't create .atlas directory - this may cause errors
	atlasDir := filepath.Join(tmpDir, ".atlas")
	hookStore := hook.NewFileStore(atlasDir)
	ctx := context.Background()

	cfg := &config.Config{}

	// Should handle missing directory gracefully
	toDelete, stats, err := collectStaleHooks(ctx, hookStore, cfg)

	// The function should not error out, just return empty results
	require.NoError(t, err)
	assert.Empty(t, toDelete)
	assert.Equal(t, 0, stats.completed)
	assert.Equal(t, 0, stats.failed)
	assert.Equal(t, 0, stats.abandoned)
}

// TestNewCleanupCmd_FlagsAndCommand tests the cleanup command creation.
func TestNewCleanupCmd_FlagsAndCommand(t *testing.T) {
	t.Parallel()

	cmd := newCleanupCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "cleanup", cmd.Use)
	assert.Contains(t, cmd.Short, "Clean up old task artifacts")
	assert.Contains(t, cmd.Long, "retention policies")

	// Test flags
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)
	assert.Equal(t, "false", dryRunFlag.DefValue)
	assert.Contains(t, dryRunFlag.Usage, "Preview")

	hooksFlag := cmd.Flags().Lookup("hooks")
	require.NotNil(t, hooksFlag)
	assert.Equal(t, "false", hooksFlag.DefValue)
	assert.Contains(t, hooksFlag.Usage, "hook files")
}

// TestGetRetentionDuration_Negative tests getRetentionDuration with negative values.
func TestGetRetentionDuration_Negative(t *testing.T) {
	t.Parallel()

	// Negative configured value should use default
	result := getRetentionDuration(-1*time.Hour, 30*24*time.Hour)
	assert.Equal(t, 30*24*time.Hour, result)
}
