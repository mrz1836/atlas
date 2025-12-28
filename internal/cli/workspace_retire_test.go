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
	"github.com/mrz1836/atlas/internal/workspace"
)

func TestWorkspaceRetireCommand_Structure(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find retire command
	retireCmd, _, err := rootCmd.Find([]string{"workspace", "retire"})
	require.NoError(t, err)
	assert.NotNil(t, retireCmd)
	assert.Equal(t, "retire", retireCmd.Name())
}

func TestWorkspaceRetireCommand_HasForceFlag(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find retire command
	retireCmd, _, err := rootCmd.Find([]string{"workspace", "retire"})
	require.NoError(t, err)

	// Verify --force flag exists
	forceFlag := retireCmd.Flag("force")
	assert.NotNil(t, forceFlag, "--force flag should exist")

	// Verify shorthand -f
	assert.Equal(t, "f", forceFlag.Shorthand, "shorthand should be -f")
}

func TestWorkspaceRetireCommand_RequiresArg(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Execute retire without arguments
	rootCmd.SetArgs([]string{"workspace", "retire"})
	err := rootCmd.Execute()

	// Should fail because name argument is required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestRunWorkspaceRetire_WorkspaceNotFound(t *testing.T) {
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

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire with nonexistent workspace
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "nonexistent", true, tmpDir)

	// Should return an error matching AC5 format with wrapped sentinel
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Workspace 'nonexistent' not found")
	assert.ErrorIs(t, err, errors.ErrWorkspaceNotFound)
}

func TestRunWorkspaceRetire_HappyPath(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create test workspace with active status
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test-worktree", // Fake path
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

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire with force flag
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "test-ws", true, tmpDir)

	// Should succeed
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "retired")
	assert.Contains(t, buf.String(), "History preserved")

	// Verify workspace is retired but still exists
	ws2, err := store.Get(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusRetired, ws2.Status)
	assert.Empty(t, ws2.WorktreePath)        // Worktree path cleared
	assert.Equal(t, "feat/test", ws2.Branch) // Branch preserved
}

func TestRunWorkspaceRetire_PausedWorkspace(t *testing.T) {
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

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "paused-ws", true, tmpDir)

	// Should succeed
	require.NoError(t, err)

	// Verify workspace is retired
	ws2, err := store.Get(context.Background(), "paused-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusRetired, ws2.Status)
}

func TestRunWorkspaceRetire_WithRunningTasks(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace with a running task
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "running-ws",
		WorktreePath: "/tmp/running-worktree",
		Branch:       "feat/running",
		Status:       constants.WorkspaceStatusActive,
		Tasks: []domain.TaskRef{
			{ID: "task-123", Status: constants.TaskStatusRunning},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "running-ws", true, tmpDir)

	// Should fail with running tasks error (AC #3)
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrWorkspaceHasRunningTasks)
	// Verify error message format includes workspace name and reason
	assert.Contains(t, err.Error(), "cannot retire workspace")
	assert.Contains(t, err.Error(), "running-ws")
	assert.Contains(t, err.Error(), "running tasks")
}

func TestRunWorkspaceRetire_AlreadyRetired(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace that is already retired
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "retired-ws",
		WorktreePath: "", // Already cleared
		Branch:       "feat/retired",
		Status:       constants.WorkspaceStatusRetired,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "retired-ws", true, tmpDir)

	// Should succeed with informative message (AC #4)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "already retired")
}

func TestRunWorkspaceRetire_JSONOutput(t *testing.T) {
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

	// Execute retire with JSON output
	err = runWorkspaceRetireWithOutput(context.Background(), &buf, "json-test", true, tmpDir, OutputJSON)
	require.NoError(t, err)

	// Parse JSON output
	var result retireResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "retired", result.Status)
	assert.Equal(t, "json-test", result.Workspace)
	assert.True(t, result.HistoryPreserved)
}

func TestRunWorkspaceRetire_JSONOutput_Error(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Execute retire with nonexistent workspace and JSON output
	err := runWorkspaceRetireWithOutput(context.Background(), &buf, "nonexistent", true, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for non-zero exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result retireResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "nonexistent", result.Workspace)
	assert.Contains(t, result.Error, "not found")
}

func TestRunWorkspaceRetire_ContextCancellation(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with canceled context
	err := runWorkspaceRetire(ctx, retireCmd, &buf, "test-ws", true, tmpDir)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunWorkspaceRetire_NonInteractiveWithoutForce(t *testing.T) {
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

	// Execute retire WITHOUT --force in non-interactive mode
	err = runWorkspaceRetireWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, "text")

	// Should return ErrNonInteractiveMode
	require.ErrorIs(t, err, errors.ErrNonInteractiveMode)

	// Workspace should still exist (not retired)
	exists, err := store.Exists(context.Background(), "test-ws")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRunWorkspaceRetire_NonInteractiveWithoutForce_JSON(t *testing.T) {
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

	// Execute retire WITHOUT --force in non-interactive mode with JSON output
	err = runWorkspaceRetireWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for proper exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result retireResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "use --force in non-interactive mode")
}

func TestWorkspaceRetireCommand_Help(t *testing.T) {
	// Create root command with workspace subcommand
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddWorkspaceCommand(rootCmd)

	// Find retire command
	retireCmd, _, err := rootCmd.Find([]string{"workspace", "retire"})
	require.NoError(t, err)

	// Verify help text contains key information
	assert.Contains(t, retireCmd.Long, "Archive a completed workspace")
	assert.Contains(t, retireCmd.Long, "--force")
	assert.Contains(t, retireCmd.Short, "Retire a workspace")
}

func TestOutputRetireSuccessJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputRetireSuccessJSON(&buf, "test-ws")
	require.NoError(t, err)

	// Parse JSON output
	var result retireResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "retired", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.True(t, result.HistoryPreserved)
	assert.Empty(t, result.Error)
}

func TestOutputRetireErrorJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputRetireErrorJSON(&buf, "test-ws", "something went wrong")
	require.NoError(t, err)

	// Parse JSON output
	var result retireResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Equal(t, "something went wrong", result.Error)
	assert.False(t, result.HistoryPreserved)
}

func TestRunWorkspaceRetire_SuccessMessage(t *testing.T) {
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
	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "success-test", true, tmpDir)
	require.NoError(t, err)

	// Verify success message format (AC #2)
	output := buf.String()
	assert.Contains(t, output, "Workspace 'success-test' retired")
	assert.Contains(t, output, "History preserved")
	assert.Contains(t, output, "✓")
}

func TestRunWorkspaceRetire_WithValidatingTasks(t *testing.T) {
	// Create temp directory for test store
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create store
	store, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace with a validating task
	now := time.Now()
	ws := &domain.Workspace{
		Name:         "validating-ws",
		WorktreePath: "/tmp/validating-worktree",
		Branch:       "feat/validating",
		Status:       constants.WorkspaceStatusActive,
		Tasks: []domain.TaskRef{
			{ID: "task-456", Status: constants.TaskStatusValidating},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, store.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Create a mock command
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	retireCmd := &cobra.Command{Use: "retire"}
	rootCmd.AddCommand(retireCmd)

	// Execute retire
	err = runWorkspaceRetire(context.Background(), retireCmd, &buf, "validating-ws", true, tmpDir)

	// Should fail with running tasks error (validating also blocks)
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrWorkspaceHasRunningTasks)
}
