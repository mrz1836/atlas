package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/workspace"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceDestroyCommand_Structure(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find destroy command
	destroyCmd, _, err := rootCmd.Find([]string{"workspace", "destroy"})
	require.NoError(t, err)
	assert.NotNil(t, destroyCmd)
	assert.Equal(t, "destroy", destroyCmd.Name())
}

func TestWorkspaceDestroyCommand_HasForceFlag(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find destroy command
	destroyCmd, _, err := rootCmd.Find([]string{"workspace", "destroy"})
	require.NoError(t, err)

	// Verify --force flag exists
	forceFlag := destroyCmd.Flag("force")
	assert.NotNil(t, forceFlag, "--force flag should exist")

	// Verify shorthand -f
	assert.Equal(t, "f", forceFlag.Shorthand, "shorthand should be -f")
}

func TestWorkspaceDestroyCommand_RequiresArg(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Execute destroy without arguments
	rootCmd.SetArgs([]string{"workspace", "destroy"})
	err := rootCmd.Execute()

	// Should fail because name argument is required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestRunWorkspaceDestroy_WorkspaceNotFound(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create store (empty - no workspaces)
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Verify workspace doesn't exist
	exists, err := store.Exists(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Execute destroy with nonexistent workspace
	err = runWorkspaceDestroy(context.Background(), destroyCmd, &buf, "nonexistent", true, tmpDir)

	// Should return an error matching AC5 format with wrapped sentinel
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Workspace 'nonexistent' not found")
	assert.ErrorIs(t, err, errors.ErrWorkspaceNotFound)
}

func TestRunWorkspaceDestroy_WithForceFlag(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test-worktree", // Fake path
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Verify workspace exists
	exists, err := store.Exists(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.True(t, exists)

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Execute destroy with force flag
	// This will attempt worktree operations which may fail, but per NFR18
	// the destroy should still succeed (cleanup state)
	err = runWorkspaceDestroy(context.Background(), destroyCmd, &buf, "test-ws", true, tmpDir)

	// Should succeed (NFR18 - destroy always succeeds)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "destroyed")

	// Verify workspace is gone from store
	exists, err = store.Exists(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRunWorkspaceDestroy_JSONOutput(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "json-test",
		WorktreePath: "/tmp/json-worktree",
		Branch:       "feat/json",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Execute destroy with JSON output
	err = runWorkspaceDestroyWithOutput(context.Background(), &buf, "json-test", true, tmpDir, OutputJSON)
	require.NoError(t, err)

	// Parse JSON output
	var result map[string]string
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "destroyed", result["status"])
	assert.Equal(t, "json-test", result["workspace"])
}

func TestRunWorkspaceDestroy_JSONOutput_Error(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Execute destroy with nonexistent workspace and JSON output
	err := runWorkspaceDestroyWithOutput(context.Background(), &buf, "nonexistent", true, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for non-zero exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output - error was already written to buffer
	var result map[string]string
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "nonexistent", result["workspace"])
	assert.Contains(t, result["error"], "not found")
}

func TestRunWorkspaceDestroy_ContextCancellation(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with canceled context
	err := runWorkspaceDestroy(ctx, destroyCmd, &buf, "test-ws", true, tmpDir)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunWorkspaceDestroy_CorruptedState(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "corrupt-test",
		WorktreePath: "/nonexistent/path/that/doesnt/exist",
		Branch:       "feat/corrupt",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Execute destroy - worktree path doesn't exist, but should still succeed per NFR18
	err = runWorkspaceDestroy(context.Background(), destroyCmd, &buf, "corrupt-test", true, tmpDir)

	// Should succeed even with corrupted/missing worktree (NFR18)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "destroyed")

	// Verify workspace state is cleaned up
	exists, err := store.Exists(context.Background(), "corrupt-test")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestWorkspaceDestroyCommand_Help(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find destroy command
	destroyCmd, _, err := rootCmd.Find([]string{"workspace", "destroy"})
	require.NoError(t, err)

	// Verify help text contains key information
	assert.Contains(t, destroyCmd.Long, "Completely remove a workspace")
	assert.Contains(t, destroyCmd.Long, "--force")
	assert.Contains(t, destroyCmd.Short, "Destroy a workspace")
}

func TestOutputDestroySuccessJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputDestroySuccessJSON(&buf, "test-ws")
	require.NoError(t, err)

	// Parse JSON output
	var result destroyResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "destroyed", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Empty(t, result.Error)
}

func TestOutputDestroyErrorJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputDestroyErrorJSON(&buf, "test-ws", "something went wrong")
	require.NoError(t, err)

	// Parse JSON output
	var result destroyResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Equal(t, "something went wrong", result.Error)
}

func TestRunWorkspaceDestroy_NonInteractiveWithoutForce(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store with a workspace
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test-worktree",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Execute destroy WITHOUT --force in non-interactive mode
	err = runWorkspaceDestroyWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, "text")

	// Should return ErrNonInteractiveMode
	require.ErrorIs(t, err, errors.ErrNonInteractiveMode)
	assert.Contains(t, err.Error(), "use --force in non-interactive mode")

	// Workspace should still exist (not destroyed)
	exists, err := store.Exists(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRunWorkspaceDestroy_NonInteractiveWithoutForce_JSON(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store with a workspace
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test-worktree",
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Execute destroy WITHOUT --force in non-interactive mode with JSON output
	err = runWorkspaceDestroyWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for proper exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result map[string]string
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result["status"])
	assert.Contains(t, result["error"], "use --force in non-interactive mode")
}

func TestIsTerminal(t *testing.T) {
	// Verify isTerminal returns a boolean without panicking
	// In CI/test environments, stdin is typically not a terminal
	result := isTerminal()

	// We can only verify it returns a boolean and doesn't panic
	// The actual value depends on the test environment
	assert.IsType(t, true, result)
}

func TestDetectRepoPath_NotInRepo(t *testing.T) {
	// Create a temporary directory that is not a git repo
	tmpDir := t.TempDir()

	// Change to tmp dir (save original for restore)
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	// detectRepoPath should return error when not in a git repo
	_, err = detectRepoPath()
	require.Error(t, err)
	// Uses sentinel error from errors package
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRunWorkspaceDestroy_MultipleWorkspaces(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()

	// Create multiple workspaces
	workspaces := []string{"ws-1", "ws-2", "ws-3"}
	for _, name := range workspaces {
		ws := &domain.Workspace{
			Name:         name,
			WorktreePath: "/tmp/" + name,
			Branch:       "feat/" + name,
			Status:       constants.WorkspaceStatusActive,
			Tasks:        []domain.TaskRef{},
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		require.NoError(t, store.Create(context.Background(), ws))
	}

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})
	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Destroy ws-2
	var buf bytes.Buffer
	err = runWorkspaceDestroy(context.Background(), destroyCmd, &buf, "ws-2", true, tmpDir)
	require.NoError(t, err)

	// Verify ws-2 is gone
	exists, err := store.Exists(context.Background(), "ws-2")
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify ws-1 and ws-3 still exist
	exists, err = store.Exists(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = store.Exists(context.Background(), "ws-3")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRunWorkspaceDestroy_SuccessMessage(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("NO_COLOR", "1") // Disable colors for consistent output

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "success-test",
		WorktreePath: "/tmp/success-test",
		Branch:       "feat/success",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})
	destroyCmd := &cobra.Command{Use: "destroy"}
	rootCmd.AddCommand(destroyCmd)

	// Execute destroy
	err = runWorkspaceDestroy(context.Background(), destroyCmd, &buf, "success-test", true, tmpDir)
	require.NoError(t, err)

	// Verify success message format
	output := buf.String()
	assert.Contains(t, output, "Workspace 'success-test' destroyed")
	assert.Contains(t, output, "✓")
}

func TestCheckWorkspaceExists_ExistsCheckError(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Use context cancellation to trigger error in store.Exists()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, exists, err := checkWorkspaceExists(
		ctx,
		"test-ws",
		tmpDir,
		"text",
		&buf,
	)

	require.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestCheckWorkspaceExists_ExistsCheckError_JSON(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Use context cancellation to trigger error in store.Exists() with JSON output
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, exists, err := checkWorkspaceExists(
		ctx,
		"test-ws",
		tmpDir,
		OutputJSON,
		&buf,
	)

	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)
	assert.False(t, exists)

	// Verify JSON error output
	var result destroyResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "failed to check workspace")
}

func TestHandleConfirmation_ForceFlag(t *testing.T) {
	var buf bytes.Buffer

	// With force flag, should return nil immediately
	err := handleConfirmation("test-ws", true, "text", &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
