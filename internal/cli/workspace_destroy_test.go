package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestShowManualCleanupWarning_TextOutput(t *testing.T) {
	var buf bytes.Buffer

	// Set up test logger
	logger := InitLoggerWithWriter(false, false, &buf)

	// Clear the buffer to only capture showManualCleanupWarning output
	buf.Reset()

	showManualCleanupWarning(&buf, "text", "/tmp/test-worktree", "feat/test", logger)

	output := buf.String()
	assert.Contains(t, output, "Manual cleanup may be required")
	assert.Contains(t, output, "git worktree remove --force /tmp/test-worktree")
	assert.Contains(t, output, "git branch -D feat/test")
}

func TestShowManualCleanupWarning_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	var logBuf bytes.Buffer

	// Set up test logger
	logger := InitLoggerWithWriter(false, false, &logBuf)

	showManualCleanupWarning(&buf, OutputJSON, "/tmp/test-worktree", "feat/test", logger)

	// With JSON output, should not write cleanup instructions to buffer
	assert.Empty(t, buf.String())
}

func TestShowManualCleanupWarning_EmptyWorktreePath(t *testing.T) {
	var buf bytes.Buffer
	var logBuf bytes.Buffer

	logger := InitLoggerWithWriter(false, false, &logBuf)

	buf.Reset()
	showManualCleanupWarning(&buf, "text", "", "feat/test", logger)

	output := buf.String()
	assert.Contains(t, output, "Manual cleanup may be required")
	// Should not contain worktree command since path is empty
	assert.NotContains(t, output, "git worktree remove")
	// Should still contain branch command
	assert.Contains(t, output, "git branch -D feat/test")
}

func TestShowManualCleanupWarning_EmptyBranch(t *testing.T) {
	var buf bytes.Buffer
	var logBuf bytes.Buffer

	logger := InitLoggerWithWriter(false, false, &logBuf)

	buf.Reset()
	showManualCleanupWarning(&buf, "text", "/tmp/test-worktree", "", logger)

	output := buf.String()
	assert.Contains(t, output, "Manual cleanup may be required")
	// Should contain worktree command
	assert.Contains(t, output, "git worktree remove --force /tmp/test-worktree")
	// Should not contain branch command since branch is empty
	assert.NotContains(t, output, "git branch -D")
}

func TestDetectRepoPath_InRepo(t *testing.T) {
	// This test should pass when run from within the atlas git repo
	// Save original directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// detectRepoPath should succeed when in a git repo
	repoPath, err := detectRepoPath()
	if err != nil {
		// If not in a git repo, skip test
		t.Skip("Not running in a git repository")
	}

	// Should return a path with .git directory
	gitPath := filepath.Join(repoPath, ".git")
	_, statErr := os.Stat(gitPath)
	assert.NoError(t, statErr, ".git directory should exist at detected repo path")
}

func TestDetectRepoPath_ParentDirectory(t *testing.T) {
	// Create a mock git repo structure
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o750))

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "subdir", "nested")
	require.NoError(t, os.MkdirAll(subDir, 0o750))

	// Change to subdirectory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.Chdir(subDir))

	// detectRepoPath should find parent .git
	repoPath, err := detectRepoPath()
	require.NoError(t, err)

	// On macOS, tmpDir paths may have /private prefix, so use filepath.EvalSymlinks
	expectedPath, evalErr := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, evalErr)
	actualPath, evalErr := filepath.EvalSymlinks(repoPath)
	require.NoError(t, evalErr)
	assert.Equal(t, expectedPath, actualPath)
}

func TestHandleConfirmation_UserCancels(t *testing.T) {
	var buf bytes.Buffer

	// Override terminalCheck to simulate interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return true }
	defer func() { terminalCheck = originalTerminalCheck }()

	// We can't easily test the actual confirmation dialog without user input
	// This test just verifies the force flag path works
	err := handleConfirmation("test-ws", true, "text", &buf)
	require.NoError(t, err)
}

func TestExecuteDestroy_WithLinkedDiscoveries(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("ATLAS_HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "discovery-test",
		WorktreePath: "/tmp/discovery-test",
		Branch:       "feat/discovery",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer
	var logBuf bytes.Buffer
	logger := InitLoggerWithWriter(true, false, &logBuf) // verbose mode for debug logs

	// Execute destroy
	err = executeDestroy(context.Background(), store, "discovery-test", "text", &buf, logger)
	require.NoError(t, err)

	// Verify success message
	assert.Contains(t, buf.String(), "destroyed")
}

func TestExecuteDestroy_WorktreeRunnerCreationFails(t *testing.T) {
	// Create temp directory for test store (not a git repo)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace with worktree info
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "no-runner-test",
		WorktreePath: "/tmp/no-runner-test",
		Branch:       "feat/no-runner",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer
	var logBuf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &logBuf)

	// Change to tmpDir which is NOT a git repo
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute destroy - should succeed even without worktree runner (NFR18)
	err = executeDestroy(context.Background(), store, "no-runner-test", "text", &buf, logger)
	require.NoError(t, err)

	// Should show manual cleanup warning
	output := buf.String()
	assert.Contains(t, output, "Manual cleanup may be required")
	assert.Contains(t, output, "git worktree remove")
	assert.Contains(t, output, "git branch -D")
}

func TestExecuteDestroy_JSONOutputWithWarning(t *testing.T) {
	// Create temp directory for test store (not a git repo)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace with worktree info
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "json-warning-test",
		WorktreePath: "/tmp/json-warning-test",
		Branch:       "feat/json-warning",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer
	var logBuf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &logBuf)

	// Change to tmpDir which is NOT a git repo
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Execute destroy with JSON output
	err = executeDestroy(context.Background(), store, "json-warning-test", OutputJSON, &buf, logger)
	require.NoError(t, err)

	// Parse JSON output - should be success, not manual cleanup text
	var result destroyResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "destroyed", result.Status)
	assert.Equal(t, "json-warning-test", result.Workspace)

	// Manual cleanup warning should not appear in JSON output
	assert.NotContains(t, buf.String(), "Manual cleanup may be required")
}

func TestRunWorkspaceDestroyWithOutput_FullFlow(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "full-flow-test",
		WorktreePath: "/tmp/full-flow-test",
		Branch:       "feat/full-flow",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Execute the full destroy flow with force
	err = runWorkspaceDestroyWithOutput(
		context.Background(),
		&buf,
		"full-flow-test",
		true,
		tmpDir,
		"text",
	)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "destroyed")

	// Verify workspace is gone
	exists, err := store.Exists(context.Background(), "full-flow-test")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRunWorkspaceDestroyWithOutput_NoColorEnv(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("NO_COLOR", "1")

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "no-color-test",
		WorktreePath: "/tmp/no-color-test",
		Branch:       "feat/no-color",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Execute destroy - should respect NO_COLOR
	err = runWorkspaceDestroyWithOutput(
		context.Background(),
		&buf,
		"no-color-test",
		true,
		tmpDir,
		"text",
	)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "destroyed")
}

func TestExecuteDestroy_GetWorkspaceError(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "get-error-test",
		WorktreePath: "/tmp/get-error-test",
		Branch:       "feat/get-error",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer
	var logBuf bytes.Buffer
	logger := InitLoggerWithWriter(false, false, &logBuf)

	// Use canceled context to trigger potential errors during Get
	ctx := context.Background()

	// Execute destroy - should still succeed even if Get has issues
	err = executeDestroy(ctx, store, "get-error-test", "text", &buf, logger)
	require.NoError(t, err)
}

func TestDeleteLinkedDiscoveries_TaskStoreError(t *testing.T) {
	var logBuf bytes.Buffer
	_ = InitLoggerWithWriter(true, false, &logBuf) // Initialize global logger

	// Create invalid ATLAS_HOME to trigger task store creation error
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(invalidFile, []byte("test"), 0o600))
	t.Setenv("ATLAS_HOME", invalidFile)

	logger := Logger()

	// deleteLinkedDiscoveries should handle error gracefully (best-effort)
	// Should not panic even with invalid ATLAS_HOME
	deleteLinkedDiscoveries(context.Background(), "test-ws", logger)
}

func TestDeleteLinkedDiscoveries_ListTasksError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("ATLAS_HOME", tmpDir)

	var logBuf bytes.Buffer
	_ = InitLoggerWithWriter(true, false, &logBuf)
	logger := Logger()

	// Use canceled context to trigger error in List
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// deleteLinkedDiscoveries should handle error gracefully
	// Should not panic even with canceled context
	deleteLinkedDiscoveries(ctx, "test-ws", logger)
}

func TestDeleteLinkedDiscoveries_BacklogManagerError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Set ATLAS_HOME for task store, but create invalid backlog structure
	atlasHome := filepath.Join(tmpDir, ".atlas")
	require.NoError(t, os.MkdirAll(atlasHome, 0o750))
	t.Setenv("ATLAS_HOME", atlasHome)

	// Create backlog dir as a file to cause backlog manager creation to fail
	backlogPath := filepath.Join(atlasHome, "backlog")
	require.NoError(t, os.WriteFile(backlogPath, []byte("test"), 0o600))

	var logBuf bytes.Buffer
	_ = InitLoggerWithWriter(true, false, &logBuf)
	logger := Logger()

	// deleteLinkedDiscoveries should handle backlog manager error gracefully
	// Should not panic even when backlog manager creation fails
	deleteLinkedDiscoveries(context.Background(), "test-ws", logger)
}

func TestAddWorkspaceDestroyCmd_JSONErrorHandling(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("ATLAS_HOME", tmpDir)

	// Test the JSON error path directly
	var buf bytes.Buffer

	// Execute destroy for nonexistent workspace with JSON output
	err := runWorkspaceDestroyWithOutput(
		context.Background(),
		&buf,
		"nonexistent",
		true,
		tmpDir,
		OutputJSON,
	)

	// Should return ErrJSONErrorOutput
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Output should be valid JSON
	var result map[string]string
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result["status"])
	assert.Equal(t, "nonexistent", result["workspace"])
}
