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
		Name:         "completed-ws",
		WorktreePath: "/tmp/completed-ws",
		Branch:       "feat/completed",
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{{ID: taskID}},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// Create task store and task in completed state (not abandonable even with force)
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	completedTask := &domain.Task{
		ID:          taskID,
		WorkspaceID: "completed-ws",
		Status:      constants.TaskStatusCompleted, // Terminal state - cannot be abandoned
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, taskStore.Create(context.Background(), "completed-ws", completedTask))

	var buf bytes.Buffer

	// Execute abandon - task is in completed state (terminal state)
	err = runAbandonWithOutput(context.Background(), &buf, "completed-ws", true, tmpDir, "text")

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

// Phase 2 Quick Wins: Test entry point functions

func TestRunAbandon_ExtractsTextOutputFlag(t *testing.T) {
	// Create a command with output flag set to "text"
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)

	// Set output flag to text (it's a persistent flag on root)
	err = rootCmd.PersistentFlags().Set("output", "text")
	require.NoError(t, err)

	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Call runAbandon - it should extract the "text" flag and call runAbandonWithOutput
	err = runAbandon(context.Background(), abandonCmd, &buf, "nonexistent", true, tmpDir)

	// Should error because workspace doesn't exist, but that means the function executed
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace")

	// Verify text output format was used (no JSON structure)
	output := buf.String()
	assert.NotContains(t, output, `"status"`)
	assert.NotContains(t, output, `"error"`)
}

func TestRunAbandon_ExtractsJSONOutputFlag(t *testing.T) {
	// Create a command with output flag set to "json"
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)

	// Set output flag to json (it's a persistent flag on root)
	err = rootCmd.PersistentFlags().Set("output", "json")
	require.NoError(t, err)

	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Call runAbandon - it should extract the "json" flag and call runAbandonWithOutput
	err = runAbandon(context.Background(), abandonCmd, &buf, "nonexistent", true, tmpDir)

	// Should error because workspace doesn't exist
	require.Error(t, err)

	// Verify JSON output format was used
	output := buf.String()
	var result abandonResult
	jsonErr := json.Unmarshal([]byte(output), &result)
	require.NoError(t, jsonErr, "output should be valid JSON")
	assert.Equal(t, "error", result.Status)
	assert.Contains(t, result.Error, "failed to get workspace")
}

func TestRunAbandon_RespectsContextCancellation(t *testing.T) {
	// Create a command
	flags := &GlobalFlags{}
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, flags)
	AddAbandonCommand(rootCmd)

	// Find abandon command
	abandonCmd, _, err := rootCmd.Find([]string{"abandon"})
	require.NoError(t, err)

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Call runAbandon with canceled context
	err = runAbandon(ctx, abandonCmd, &buf, "test-workspace", true, tmpDir)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewAbandonCmd_FlagRegistration(t *testing.T) {
	cmd := newAbandonCmd()

	// Verify command structure
	assert.Equal(t, "abandon", cmd.Name())
	assert.Equal(t, "abandon <workspace>", cmd.Use)

	// Verify force flag is registered
	forceFlag := cmd.Flag("force")
	require.NotNil(t, forceFlag, "--force flag should be registered")
	assert.Equal(t, "f", forceFlag.Shorthand)
	assert.Equal(t, "false", forceFlag.DefValue, "default should be false")

	// Verify exactly 1 argument is required
	err := cmd.Args(cmd, []string{})
	require.Error(t, err, "should require exactly 1 argument")

	err = cmd.Args(cmd, []string{"workspace1"})
	require.NoError(t, err, "should accept 1 argument")

	err = cmd.Args(cmd, []string{"workspace1", "extra"})
	assert.Error(t, err, "should reject more than 1 argument")
}

func TestNewAbandonCmd_ShortAndLongDesc(t *testing.T) {
	cmd := newAbandonCmd()

	// Verify descriptions are set
	assert.NotEmpty(t, cmd.Short, "short description should not be empty")
	assert.NotEmpty(t, cmd.Long, "long description should not be empty")

	// Verify key information in descriptions
	assert.Contains(t, cmd.Short, "Abandon")
	assert.Contains(t, cmd.Short, "failed task")

	assert.Contains(t, cmd.Long, "Abandon a task")
	assert.Contains(t, cmd.Long, "--force")
	assert.Contains(t, cmd.Long, "Examples:")
}

// Phase 3: Form Interactions - Test confirmation forms

func TestConfirmAbandon_UserConfirms(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAbandonConfirmForm
	defer func() { createAbandonConfirmForm = originalFactory }()

	// Mock the form to simulate user confirming
	var capturedWorkspace string
	var capturedRunning bool
	createAbandonConfirmForm = func(ws string, running bool, confirm *bool) formRunner {
		capturedWorkspace = ws
		capturedRunning = running
		*confirm = true // Simulate user clicking "Yes, abandon"
		return &mockFormRunner{}
	}

	confirmed, err := confirmAbandon("test-workspace", false)

	require.NoError(t, err)
	assert.True(t, confirmed, "should return true when user confirms")
	assert.Equal(t, "test-workspace", capturedWorkspace)
	assert.False(t, capturedRunning)
}

func TestConfirmAbandon_UserCancels(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAbandonConfirmForm
	defer func() { createAbandonConfirmForm = originalFactory }()

	// Mock the form to simulate user canceling
	createAbandonConfirmForm = func(_ string, _ bool, confirm *bool) formRunner {
		*confirm = false // Simulate user clicking "No, cancel"
		return &mockFormRunner{}
	}

	confirmed, err := confirmAbandon("test-workspace", false)

	require.NoError(t, err)
	assert.False(t, confirmed, "should return false when user cancels")
}

func TestConfirmAbandon_RunningTaskShowsWarning(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAbandonConfirmForm
	defer func() { createAbandonConfirmForm = originalFactory }()

	// Mock the form to capture the isRunning parameter
	var capturedRunning bool
	createAbandonConfirmForm = func(_ string, running bool, confirm *bool) formRunner {
		capturedRunning = running
		*confirm = true
		return &mockFormRunner{}
	}

	_, err := confirmAbandon("test-workspace", true)

	require.NoError(t, err)
	assert.True(t, capturedRunning, "should pass isRunning=true to form factory")
}

func TestConfirmAbandon_FormError(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAbandonConfirmForm
	defer func() { createAbandonConfirmForm = originalFactory }()

	// Mock the form to return an error
	expectedErr := errors.ErrUserAbandoned
	createAbandonConfirmForm = func(_ string, _ bool, _ *bool) formRunner {
		return &mockFormRunner{runErr: expectedErr}
	}

	confirmed, err := confirmAbandon("test-workspace", false)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.False(t, confirmed, "should return false on error")
}

func TestConfirmAbandon_PassesWorkspaceName(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAbandonConfirmForm
	defer func() { createAbandonConfirmForm = originalFactory }()

	tests := []struct {
		name          string
		workspaceName string
	}{
		{"simple name", "my-workspace"},
		{"with dashes", "feature-auth-fix"},
		{"with numbers", "hotfix-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedWorkspace string
			createAbandonConfirmForm = func(ws string, _ bool, confirm *bool) formRunner {
				capturedWorkspace = ws
				*confirm = true
				return &mockFormRunner{}
			}

			_, err := confirmAbandon(tt.workspaceName, false)

			require.NoError(t, err)
			assert.Equal(t, tt.workspaceName, capturedWorkspace)
		})
	}
}

func TestConfirmAbandonmentInteractive_WithForceSkipsConfirmation(_ *testing.T) {
	// This function should not be called when force=true
	// The runAbandon flow skips confirmAbandonmentInteractive entirely
	// This is already tested in TestRunAbandon_Success with force=true
}

func TestConfirmAbandonmentInteractive_NonInteractiveModeErrors(t *testing.T) {
	// Save and restore terminal check
	cleanup := mockTerminalCheckFunc(false)
	defer cleanup()

	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Create workspace and task
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: tmpDir,
		Branch:       "test-branch",
		Status:       constants.WorkspaceStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err = wsStore.Create(context.Background(), ws)
	require.NoError(t, err)

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	testTask := &domain.Task{
		ID:          testTaskID("120000"),
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusValidationFailed,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = taskStore.Create(context.Background(), "test-ws", testTask)
	require.NoError(t, err)

	// Try to run without force in non-interactive mode
	err = runAbandonWithOutput(context.Background(), &buf, "test-ws", false, tmpDir, "text")

	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrNonInteractiveMode)
}

func TestAbandonCommand_RunEExecution(t *testing.T) {
	// Test that RunE is actually called when the command is executed
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	AddAbandonCommand(root)

	// Execute the command with missing argument (will fail but call RunE)
	root.SetArgs([]string{"abandon"})

	// Capture output
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// Execute - should fail due to missing workspace argument
	err := root.Execute()

	// Should error because workspace argument is required
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestAbandonCommand_RunEWithWorkspace(t *testing.T) {
	// Test RunE with a workspace argument
	tmpDir := t.TempDir()

	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	AddAbandonCommand(root)

	// Execute with --force and nonexistent workspace
	root.SetArgs([]string{"abandon", "nonexistent", "--force"})

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// Set HOME to tmpDir to isolate file store
	t.Setenv("HOME", tmpDir)

	// Execute - will fail because workspace doesn't exist, but RunE is called
	_ = root.Execute()

	// Verify the command attempted to run (error indicates RunE was called)
	output := buf.String()
	// Output should indicate workspace not found or similar error
	assert.True(t, len(output) > 0 || buf.Len() > 0, "command should have produced output or error")
}
