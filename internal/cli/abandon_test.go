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

func TestAbandonCommand_Structure(t *testing.T) {
	// Create root command with abandon command
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)
	assert.NotNil(t, abandonCmd)
	assert.Equal(t, "abandon", abandonCmd.Name())
}

func TestAbandonCommand_HasForceFlag(t *testing.T) {
	// Create root command with abandon command
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)

	// Verify --force flag exists
	forceFlag := abandonCmd.Flag("force")
	assert.NotNil(t, forceFlag, "--force flag should exist")

	// Verify shorthand -f
	assert.Equal(t, "f", forceFlag.Shorthand, "shorthand should be -f")
}

func TestAbandonCommand_RequiresArg(t *testing.T) {
	// Create root command with abandon command
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Execute abandon without arguments
	rootCmd.SetArgs([]string{"abandon"})
	err := rootCmd.Execute()

	// Should fail because name argument is required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestAbandonCommand_Help(t *testing.T) {
	// Create root command with abandon command
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)

	// Verify help text contains key information
	assert.Contains(t, abandonCmd.Long, "Abandon a task")
	assert.Contains(t, abandonCmd.Long, "--force")
	assert.Contains(t, abandonCmd.Short, "Abandon a failed task")
}

func TestRunAbandon_WorkspaceNotFound(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Execute abandon with nonexistent workspace
	err := runAbandonWithOutput(context.Background(), &buf, "nonexistent", true, tmpDir, "text")

	// Should return an error with wrapped sentinel
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")
	assert.ErrorIs(t, err, errors.ErrWorkspaceNotFound)
}

// testTaskID generates a valid task ID for testing.
// Format: task-YYYYMMDD-HHMMSS
func testTaskID(suffix string) string {
	return "task-20251229-" + suffix
}

func TestRunAbandon_NoTasksInWorkspace(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "empty-ws",
		WorktreePath: "/tmp/empty-ws",
		Branch:       "feat/empty",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	var buf bytes.Buffer

	// Execute abandon - workspace exists but has no tasks
	err = runAbandonWithOutput(context.Background(), &buf, "empty-ws", true, tmpDir, "text")

	// Should return ErrNoTasksFound
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrNoTasksFound)
}

func TestRunAbandon_TaskNotAbandonable(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100001")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "running-ws",
		WorktreePath: "/tmp/running-ws",
		Branch:       "feat/running",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task in running state (not abandonable)
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	runningTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "running-ws",
		Status:      constants.TaskStatusRunning, // Not abandonable
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, taskStore.Create(context.Background(), "running-ws", runningTask))

	var buf bytes.Buffer

	// Execute abandon - task is in running state
	err = runAbandonWithOutput(context.Background(), &buf, "running-ws", true, tmpDir, "text")

	// Should return ErrInvalidTransition
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrInvalidTransition)
}

func TestRunAbandon_Success(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("NO_COLOR", "1") // Disable colors for consistent output

	taskID := testTaskID("100002")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "abandon-ws",
		WorktreePath: "/tmp/abandon-ws",
		Branch:       "fix/abandon-test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task in validation_failed state (abandonable)
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	failedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "abandon-ws",
		Status:      constants.TaskStatusValidationFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "abandon-ws", failedTask))

	var buf bytes.Buffer

	// Execute abandon with force flag
	err = runAbandonWithOutput(context.Background(), &buf, "abandon-ws", true, tmpDir, "text")
	require.NoError(t, err)

	// Verify success message contains expected elements
	output := buf.String()
	assert.Contains(t, output, "Task Abandoned")
	assert.Contains(t, output, "fix/abandon-test") // Branch name
	assert.Contains(t, output, "preserved")

	// Verify task was transitioned to abandoned
	updatedTask, err := taskStore.Get(context.Background(), "abandon-ws", taskID)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, updatedTask.Status)

	// Verify workspace status was updated to paused
	updatedWs, err := wsStore.Get(context.Background(), "abandon-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusPaused, updatedWs.Status)
}

func TestRunAbandon_JSONOutput(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100003")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "json-ws",
		WorktreePath: "/tmp/json-ws",
		Branch:       "fix/json-test",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	failedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "json-ws",
		Status:      constants.TaskStatusGHFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "json-ws", failedTask))

	var buf bytes.Buffer

	// Execute abandon with JSON output
	err = runAbandonWithOutput(context.Background(), &buf, "json-ws", true, tmpDir, OutputJSON)
	require.NoError(t, err)

	// Parse JSON output
	var result abandonResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "abandoned", result.Status)
	assert.Equal(t, "json-ws", result.Workspace)
	assert.Equal(t, taskID, result.TaskID)
	assert.Equal(t, "fix/json-test", result.Branch)
	assert.Equal(t, "/tmp/json-ws", result.WorktreePath)
	assert.Empty(t, result.Error)
}

func TestRunAbandon_JSONOutput_Error(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Execute abandon with nonexistent workspace and JSON output
	err := runAbandonWithOutput(context.Background(), &buf, "nonexistent", true, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for non-zero exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result abandonResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "nonexistent", result.Workspace)
	assert.Contains(t, result.Error, "not found")
}

func TestRunAbandon_ContextCancellation(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with canceled context
	err := runAbandonWithOutput(ctx, &buf, "test-ws", true, tmpDir, "text")

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunAbandon_NonInteractiveWithoutForce(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100004")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "noforce-ws",
		WorktreePath: "/tmp/noforce-ws",
		Branch:       "fix/noforce",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	failedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "noforce-ws",
		Status:      constants.TaskStatusCIFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "noforce-ws", failedTask))

	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Execute abandon WITHOUT --force in non-interactive mode
	err = runAbandonWithOutput(context.Background(), &buf, "noforce-ws", false, tmpDir, "text")

	// Should return ErrNonInteractiveMode
	require.ErrorIs(t, err, errors.ErrNonInteractiveMode)

	// Task should still be in original state (not abandoned)
	unchangedTask, err := taskStore.Get(context.Background(), "noforce-ws", taskID)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, unchangedTask.Status)
}

func TestRunAbandon_NonInteractiveWithoutForce_JSON(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100005")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "noforce-json-ws",
		WorktreePath: "/tmp/noforce-json-ws",
		Branch:       "fix/noforce-json",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	failedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "noforce-json-ws",
		Status:      constants.TaskStatusCITimeout,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "noforce-json-ws", failedTask))

	var buf bytes.Buffer

	// Override terminalCheck to simulate non-interactive mode
	originalTerminalCheck := terminalCheck
	terminalCheck = func() bool { return false }
	defer func() { terminalCheck = originalTerminalCheck }()

	// Execute abandon WITHOUT --force in non-interactive mode with JSON output
	err = runAbandonWithOutput(context.Background(), &buf, "noforce-json-ws", false, tmpDir, OutputJSON)

	// Should return ErrJSONErrorOutput for proper exit code
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	// Parse JSON output
	var result abandonResult
	unmarshalErr := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "use --force in non-interactive mode")
}

func TestRunAbandon_FromCITimeout(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100006")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "timeout-ws",
		WorktreePath: "/tmp/timeout-ws",
		Branch:       "fix/timeout",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task in ci_timeout state
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	timeoutTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "timeout-ws",
		Status:      constants.TaskStatusCITimeout,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "timeout-ws", timeoutTask))

	var buf bytes.Buffer

	// Execute abandon
	err = runAbandonWithOutput(context.Background(), &buf, "timeout-ws", true, tmpDir, "text")
	require.NoError(t, err)

	// Verify task was transitioned to abandoned
	updatedTask, err := taskStore.Get(context.Background(), "timeout-ws", taskID)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, updatedTask.Status)
}

func TestOutputAbandonSuccessJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputAbandonSuccessJSON(&buf, "test-ws", "task-abc", "fix/test", "/path/to/worktree")
	require.NoError(t, err)

	// Parse JSON output
	var result abandonResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "abandoned", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Equal(t, "task-abc", result.TaskID)
	assert.Equal(t, "fix/test", result.Branch)
	assert.Equal(t, "/path/to/worktree", result.WorktreePath)
	assert.Empty(t, result.Error)
}

func TestOutputAbandonErrorJSON(t *testing.T) {
	var buf bytes.Buffer

	err := outputAbandonErrorJSON(&buf, "test-ws", "task-abc", "something went wrong")
	require.NoError(t, err)

	// Parse JSON output
	var result abandonResult
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result.Status)
	assert.Equal(t, "test-ws", result.Workspace)
	assert.Equal(t, "task-abc", result.TaskID)
	assert.Equal(t, "something went wrong", result.Error)
}

func TestRunAbandon_WorkspaceStatusUpdatedToPaused(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100007")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "paused-ws",
		WorktreePath: "/tmp/paused-ws",
		Branch:       "fix/paused-test",
		Status:       constants.WorkspaceStatusActive, // Start as active
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	failedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "paused-ws",
		Status:      constants.TaskStatusValidationFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
	}
	require.NoError(t, taskStore.Create(context.Background(), "paused-ws", failedTask))

	var buf bytes.Buffer

	// Execute abandon
	err = runAbandonWithOutput(context.Background(), &buf, "paused-ws", true, tmpDir, "text")
	require.NoError(t, err)

	// Verify workspace status is now paused (AC #6)
	updatedWs, err := wsStore.Get(context.Background(), "paused-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusPaused, updatedWs.Status, "workspace should be in paused state after abandonment")

	// Verify worktree path and branch are preserved (AC #2, #3)
	assert.Equal(t, "/tmp/paused-ws", updatedWs.WorktreePath, "worktree path should be preserved")
	assert.Equal(t, "fix/paused-test", updatedWs.Branch, "branch should be preserved")
}

func TestRunAbandon_TaskArtifactsPreserved(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	taskID := testTaskID("100008")

	// Create workspace store and workspace
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "artifacts-ws",
		WorktreePath: "/tmp/artifacts-ws",
		Branch:       "fix/artifacts",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task with metadata
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	taskWithMeta := &domain.Task{
		ID:          taskID,
		WorkspaceID: "artifacts-ws",
		Status:      constants.TaskStatusValidationFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
		Transitions: []domain.Transition{},
		Steps: []domain.Step{
			{Name: "step1", Status: "completed"},
			{Name: "step2", Status: "failed"},
		},
		Metadata: map[string]any{
			"last_error":    "lint failed",
			"retry_context": "Previous attempts: 2",
		},
	}
	require.NoError(t, taskStore.Create(context.Background(), "artifacts-ws", taskWithMeta))

	var buf bytes.Buffer

	// Execute abandon
	err = runAbandonWithOutput(context.Background(), &buf, "artifacts-ws", true, tmpDir, "text")
	require.NoError(t, err)

	// Verify task metadata is preserved (AC #4)
	updatedTask, err := taskStore.Get(context.Background(), "artifacts-ws", taskID)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, updatedTask.Status)
	assert.Equal(t, "lint failed", updatedTask.Metadata["last_error"])
	assert.Equal(t, "Previous attempts: 2", updatedTask.Metadata["retry_context"])
	assert.Len(t, updatedTask.Steps, 2)
}

func TestConfirmAbandon_FormStructure(_ *testing.T) {
	// This test verifies the confirmAbandon function exists and has the expected signature
	// We can't easily test the interactive form, but we can verify the function is defined
	// The actual form testing would require mocking huh forms

	// Verify confirmAbandon is a function with expected signature
	_ = confirmAbandon // Function exists
}
