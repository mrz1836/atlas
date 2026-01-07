package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/workspace"
)

func TestWorkspaceCloseCommand_Structure(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find close command
	closeCmd, _, err := rootCmd.Find([]string{"workspace", "close"})
	require.NoError(t, err)
	assert.NotNil(t, closeCmd)
	assert.Equal(t, "close", closeCmd.Name())
}

func TestWorkspaceCloseCommand_HasForceFlag(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find close command
	closeCmd, _, err := rootCmd.Find([]string{"workspace", "close"})
	require.NoError(t, err)

	// Verify --force flag exists
	forceFlag := closeCmd.Flag("force")
	assert.NotNil(t, forceFlag, "--force flag should exist")

	// Verify shorthand -f
	assert.Equal(t, "f", forceFlag.Shorthand, "shorthand should be -f")
}

func TestWorkspaceCloseCommand_RequiresArg(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Execute close without arguments
	rootCmd.SetArgs([]string{"workspace", "close"})
	err := rootCmd.Execute()

	// Should fail because name argument is required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestRunWorkspaceClose_WorkspaceNotFound(t *testing.T) {
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

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close with nonexistent workspace
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "nonexistent", true, tmpDir)

	// Should return an error matching AC5 format with wrapped sentinel
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Workspace 'nonexistent' not found")
	assert.ErrorIs(t, err, errors.ErrWorkspaceNotFound)
}

func TestRunWorkspaceClose_HappyPath(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace with active status (no worktree path for happy path)
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "", // Empty path - tests close without worktree removal
		Branch:       "feat/test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{}, // No running tasks
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

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close with force flag
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "test-ws", true, tmpDir)

	// Should succeed
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "closed")
	assert.Contains(t, buf.String(), "History preserved")

	// Verify workspace is closed but still exists
	ws2, err := store.Get(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusClosed, ws2.Status)
	assert.Empty(t, ws2.WorktreePath)        // Worktree path remains empty
	assert.Equal(t, "feat/test", ws2.Branch) // Branch preserved
}

func TestRunWorkspaceClose_PausedWorkspace(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace with paused status
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "paused-ws",
		WorktreePath: "/tmp/paused-worktree",
		Branch:       "feat/paused",
		Status:       constants.WorkspaceStatusPaused,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "paused-ws", true, tmpDir)

	// Should succeed
	require.NoError(t, err)

	// Verify workspace is closed
	ws2, err := store.Get(context.Background(), "paused-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusClosed, ws2.Status)
}

func TestRunWorkspaceClose_WithRunningTasks(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create workspace store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace FIRST (before task store creates its directory)
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "running-ws",
		WorktreePath: "/tmp/running-worktree",
		Branch:       "feat/running",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{}, // Empty - tasks are in task store
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task store and add a running task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Use valid task ID format (must match task-YYYYMMDD-HHMMSS pattern)
	taskID := task.GenerateTaskID()
	runningTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "running-ws",
		Status:      constants.TaskStatusRunning,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, taskStore.Create(context.Background(), "running-ws", runningTask))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "running-ws", true, tmpDir)

	// Should fail with running tasks error (AC #3)
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrWorkspaceHasRunningTasks)
	// Verify error message format includes workspace name and reason
	assert.Contains(t, err.Error(), "cannot close workspace")
	assert.Contains(t, err.Error(), "running-ws")
	assert.Contains(t, err.Error(), "running")
}

func TestRunWorkspaceClose_AlreadyClosed(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace that is already closed
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "closed-ws",
		WorktreePath: "", // Already cleared
		Branch:       "feat/closed",
		Status:       constants.WorkspaceStatusClosed,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "closed-ws", true, tmpDir)

	// Should succeed with informative message (AC #4)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "already closed")
}

func TestRunWorkspaceClose_JSONOutput(t *testing.T) {
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

	// Execute close with JSON output
	err = runWorkspaceCloseWithOutput(context.Background(), &buf, "json-test", true, tmpDir, OutputJSON)
	require.NoError(t, err)

	// Parse JSON output
	var result closeResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "closed", result.Status)
	assert.Equal(t, "json-test", result.Workspace)
	assert.True(t, result.HistoryPreserved)
}

func TestRunWorkspaceClose_JSONOutput_Error(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Execute close with nonexistent workspace and JSON output
	err := runWorkspaceCloseWithOutput(context.Background(), &buf, "nonexistent", true, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for non-zero exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "nonexistent", result.Workspace)
	assert.Contains(t, result.Error, "not found")
}

func TestRunWorkspaceClose_ContextCancellation(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with canceled context
	err := runWorkspaceClose(ctx, closeCmd, &buf, "test-ws", true, tmpDir)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunWorkspaceClose_NonInteractiveWithoutForce(t *testing.T) {
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

	// Execute close WITHOUT --force in non-interactive mode
	err = runWorkspaceCloseWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, "text")

	// Should return ErrNonInteractiveMode
	require.ErrorIs(t, err, errors.ErrNonInteractiveMode)

	// Workspace should still exist (not closed)
	exists, err := store.Exists(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRunWorkspaceClose_NonInteractiveWithoutForce_JSON(t *testing.T) {
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

	// Execute close WITHOUT --force in non-interactive mode with JSON output
	err = runWorkspaceCloseWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for proper exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "use --force in non-interactive mode")
}

func TestWorkspaceCloseCommand_Help(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find close command
	closeCmd, _, err := rootCmd.Find([]string{"workspace", "close"})
	require.NoError(t, err)

	// Verify help text contains key information
	assert.Contains(t, closeCmd.Long, "Archive a completed workspace")
	assert.Contains(t, closeCmd.Long, "--force")
	assert.Contains(t, closeCmd.Short, "Close a workspace")
}

func TestOutputCloseSuccessJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputCloseSuccessJSON(&buf, "test-ws")
	require.NoError(t, err)

	// Parse JSON output
	var result closeResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "closed", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.True(t, result.HistoryPreserved)
	assert.Empty(t, result.Error)
}

func TestOutputCloseErrorJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputCloseErrorJSON(&buf, "test-ws", "something went wrong")
	require.NoError(t, err)

	// Parse JSON output
	var result closeResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Equal(t, "something went wrong", result.Error)
	assert.False(t, result.HistoryPreserved)
}

func TestRunWorkspaceClose_SuccessMessage(t *testing.T) {
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
	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "success-test", true, tmpDir)
	require.NoError(t, err)

	// Verify success message format (AC #2)
	output := buf.String()
	assert.Contains(t, output, "Workspace 'success-test' closed")
	assert.Contains(t, output, "History preserved")
	assert.Contains(t, output, "✓")
}

func TestRunWorkspaceClose_WithValidatingTasks(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create workspace store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace FIRST (before task store creates its directory)
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "validating-ws",
		WorktreePath: "/tmp/validating-worktree",
		Branch:       "feat/validating",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{}, // Empty - tasks are in task store
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	// Create task store and add a validating task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Use valid task ID format (must match task-YYYYMMDD-HHMMSS pattern)
	taskID := task.GenerateTaskID()
	validatingTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "validating-ws",
		Status:      constants.TaskStatusValidating,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, taskStore.Create(context.Background(), "validating-ws", validatingTask))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	closeCmd := &cobra.Command{Use: "close"}
	rootCmd.AddCommand(closeCmd)

	// Execute close
	err = runWorkspaceClose(context.Background(), closeCmd, &buf, "validating-ws", true, tmpDir)

	// Should fail with running tasks error (validating also blocks)
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrWorkspaceHasRunningTasks)
}

func TestCheckWorkspaceExistsForClose_ExistsCheckError(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Use context cancellation to trigger error in store.Exists()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, exists, err := checkWorkspaceExistsForClose(
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

func TestCheckWorkspaceExistsForClose_ExistsCheckError_JSON(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Use context cancellation to trigger error in store.Exists() with JSON output
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, exists, err := checkWorkspaceExistsForClose(
		ctx,
		"test-ws",
		tmpDir,
		OutputJSON,
		&buf,
	)

	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)
	assert.False(t, exists)

	// Verify JSON error output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "failed to check workspace")
}

func TestHandleCloseConfirmation_ForceFlag(t *testing.T) {
	var buf bytes.Buffer

	// With force flag, should return nil immediately
	err := handleCloseConfirmation("test-ws", true, "text", &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestHandleCloseConfirmation_NonInteractive_Text(t *testing.T) {
	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Without force flag in non-interactive mode (text output)
	err := handleCloseConfirmation("test-ws", false, "text", &buf)
	require.ErrorIs(t, err, errors.ErrNonInteractiveMode)
}

func TestHandleCloseConfirmation_NonInteractive_JSON(t *testing.T) {
	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Without force flag in non-interactive mode (JSON output)
	err := handleCloseConfirmation("test-ws", false, OutputJSON, &buf)
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Verify JSON error output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "use --force in non-interactive mode")
}

func TestHandleCloseError_GenericError(t *testing.T) {
	var buf bytes.Buffer

	// Test with a generic error (not running tasks)
	genericErr := errors.ErrGitOperation
	err := handleCloseError(&buf, "test-ws", "text", genericErr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to close workspace 'test-ws'")
}

func TestHandleCloseError_GenericError_JSON(t *testing.T) {
	var buf bytes.Buffer

	// Test with a generic error in JSON mode
	genericErr := errors.ErrGitOperation
	err := handleCloseError(&buf, "test-ws", OutputJSON, genericErr)

	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Verify JSON error output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "git operation failed")
}

func TestHandleCloseError_RunningTasksError_JSON(t *testing.T) {
	var buf bytes.Buffer

	// Test with running tasks error in JSON mode
	err := handleCloseError(&buf, "test-ws", OutputJSON, errors.ErrWorkspaceHasRunningTasks)

	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Verify JSON error output
	var result closeResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "cannot close workspace with running tasks")
}
