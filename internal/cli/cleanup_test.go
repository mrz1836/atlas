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
