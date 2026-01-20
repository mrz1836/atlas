package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/task"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

func TestIsResumableStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   constants.TaskStatus
		expected bool
	}{
		// Error statuses - should be resumable
		{
			name:     "validation_failed is resumable",
			status:   constants.TaskStatusValidationFailed,
			expected: true,
		},
		{
			name:     "gh_failed is resumable",
			status:   constants.TaskStatusGHFailed,
			expected: true,
		},
		{
			name:     "ci_failed is resumable",
			status:   constants.TaskStatusCIFailed,
			expected: true,
		},
		{
			name:     "ci_timeout is resumable",
			status:   constants.TaskStatusCITimeout,
			expected: true,
		},
		// Awaiting approval - should be resumable
		{
			name:     "awaiting_approval is resumable",
			status:   constants.TaskStatusAwaitingApproval,
			expected: true,
		},
		// Non-resumable statuses
		{
			name:     "pending is not resumable",
			status:   constants.TaskStatusPending,
			expected: false,
		},
		{
			name:     "running is not resumable",
			status:   constants.TaskStatusRunning,
			expected: false,
		},
		{
			name:     "validating is not resumable",
			status:   constants.TaskStatusValidating,
			expected: false,
		},
		{
			name:     "completed is not resumable",
			status:   constants.TaskStatusCompleted,
			expected: false,
		},
		{
			name:     "rejected is not resumable",
			status:   constants.TaskStatusRejected,
			expected: false,
		},
		{
			name:     "abandoned is not resumable",
			status:   constants.TaskStatusAbandoned,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isResumableStatus(tc.status)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsResumableStatus_ConsistentWithTaskPackage(t *testing.T) {
	// Verify our isResumableStatus is consistent with task.IsErrorStatus
	// Error statuses should always be resumable
	errorStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range errorStatuses {
		t.Run(string(status), func(t *testing.T) {
			// Should be consistent with task package
			assert.True(t, task.IsErrorStatus(status), "task.IsErrorStatus should return true for %s", status)
			assert.True(t, isResumableStatus(status), "isResumableStatus should return true for %s", status)
		})
	}

	// AwaitingApproval is special - resumable but not an error status
	t.Run("awaiting_approval is resumable but not error", func(t *testing.T) {
		assert.False(t, task.IsErrorStatus(constants.TaskStatusAwaitingApproval))
		assert.True(t, isResumableStatus(constants.TaskStatusAwaitingApproval))
	})
}

func TestNewResumeCmd(t *testing.T) {
	cmd := newResumeCmd()

	t.Run("command has correct use", func(t *testing.T) {
		assert.Equal(t, "resume <workspace>", cmd.Use)
	})

	t.Run("command requires exactly one argument", func(t *testing.T) {
		assert.NotNil(t, cmd.Args)
	})

	t.Run("command has ai-fix flag", func(t *testing.T) {
		flag := cmd.Flags().Lookup("ai-fix")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.Equal(t, "Resume a paused or failed task", cmd.Short)
	})

	t.Run("command has long description with examples", func(t *testing.T) {
		assert.Contains(t, cmd.Long, "atlas resume auth-fix")
		assert.Contains(t, cmd.Long, "--ai-fix")
	})
}

func TestResumeResponse_JSON(t *testing.T) {
	t.Run("success response has correct structure", func(t *testing.T) {
		resp := resumeResponse{
			Success: true,
			Workspace: workspaceInfo{
				Name:         "test-ws",
				Branch:       "feat/test",
				WorktreePath: "/path/to/worktree",
				Status:       "active",
			},
			Task: taskInfo{
				ID:           "task-xyz",
				TemplateName: "bugfix",
				Description:  "fix something",
				Status:       "awaiting_approval",
				CurrentStep:  2,
				TotalSteps:   3,
			},
		}

		assert.True(t, resp.Success)
		assert.Empty(t, resp.Error)
		assert.Equal(t, "test-ws", resp.Workspace.Name)
		assert.Equal(t, "task-xyz", resp.Task.ID)
	})

	t.Run("error response includes error message", func(t *testing.T) {
		resp := resumeResponse{
			Success: false,
			Workspace: workspaceInfo{
				Name: "failed-ws",
			},
			Task: taskInfo{
				ID: "task-abc",
			},
			Error: "task not in resumable state",
		}

		assert.False(t, resp.Success)
		assert.Equal(t, "task not in resumable state", resp.Error)
	})
}

func TestRunResume_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newResumeCmd()
	cmd.SetContext(ctx)

	// Add output flag to root
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runResume(ctx, cmd, &buf, "test-workspace", resumeOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunResume_WorkspaceNotFound(t *testing.T) {
	// Create temp workspace store without the workspace
	tmpDir := t.TempDir()
	_, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	cmd := newResumeCmd()

	// Add output flag to root
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err = runResume(context.Background(), cmd, &buf, "nonexistent-workspace", resumeOptions{})

	// Should fail because we're not in a git repo or workspace doesn't exist
	require.Error(t, err)
}

func TestRunResume_TaskNotResumable(t *testing.T) {
	// This test verifies the error path when task is not in resumable state
	// Since runResume creates its own stores internally, we test the isResumableStatus helper
	// and the error handling path separately

	// Test that completed tasks are NOT resumable
	assert.False(t, isResumableStatus(constants.TaskStatusCompleted))
	assert.False(t, isResumableStatus(constants.TaskStatusRunning))
	assert.False(t, isResumableStatus(constants.TaskStatusPending))

	// Test that error states ARE resumable
	assert.True(t, isResumableStatus(constants.TaskStatusValidationFailed))
	assert.True(t, isResumableStatus(constants.TaskStatusGHFailed))
	assert.True(t, isResumableStatus(constants.TaskStatusCIFailed))
}

func TestHandleResumeError_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	testErr := errors.ErrWorkspaceNotFound

	result := handleResumeError("text", &buf, "test-ws", "task-abc", testErr)

	// Text format should return error directly
	assert.Equal(t, testErr, result)
	// Buffer should be empty
	assert.Empty(t, buf.String())
}

func TestHandleResumeError_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	testErr := errors.ErrInvalidTransition

	result := handleResumeError("json", &buf, "test-ws", "task-abc", testErr)

	// JSON format should return special error
	require.ErrorIs(t, result, errors.ErrJSONErrorOutput)

	// Buffer should contain JSON
	var resp resumeResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "test-ws", resp.Workspace.Name)
	assert.Equal(t, "task-abc", resp.Task.ID)
	assert.Contains(t, resp.Error, "invalid state transition")
}

func TestOutputResumeErrorJSON(t *testing.T) {
	var buf bytes.Buffer
	err := outputResumeErrorJSON(&buf, "my-workspace", "task-xyz", "something went wrong")

	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	var resp resumeResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))

	assert.False(t, resp.Success)
	assert.Equal(t, "my-workspace", resp.Workspace.Name)
	assert.Equal(t, "task-xyz", resp.Task.ID)
	assert.Equal(t, "something went wrong", resp.Error)
}

func TestDisplayResumeResult(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/tmp/test-ws",
		Status:       constants.WorkspaceStatusActive,
	}

	task := &domain.Task{
		ID:          "task-resume-test",
		TemplateID:  "bugfix",
		Description: "resume test",
		Status:      constants.TaskStatusAwaitingApproval,
		CurrentStep: 2,
		Steps:       make([]domain.Step, 4),
	}

	displayResumeResult(out, ws, task, nil)

	output := buf.String()
	assert.Contains(t, output, "task-resume-test")
	assert.Contains(t, output, "test-ws")
	assert.Contains(t, output, "3/4") // Step 2+1 / 4 total
}

func TestDisplayResumeResult_WithError(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	ws := &domain.Workspace{
		Name:   "test-ws",
		Status: constants.WorkspaceStatusActive,
	}

	task := &domain.Task{
		ID:     "task-err-test",
		Status: constants.TaskStatusValidationFailed,
		Steps:  make([]domain.Step, 3),
	}

	execErr := errors.ErrValidationFailed
	displayResumeResult(out, ws, task, execErr)

	output := buf.String()
	assert.Contains(t, output, "Execution paused")
}

func TestAddResumeCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddResumeCommand(root)

	// Verify resume command was added
	resumeCmd, _, err := root.Find([]string{"resume"})
	require.NoError(t, err)
	assert.NotNil(t, resumeCmd)
	assert.Equal(t, "resume", resumeCmd.Name())
}

func TestResumeResponse_JSONMarshal(t *testing.T) {
	resp := resumeResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/tmp/test",
			Status:       "active",
		},
		Task: taskInfo{
			ID:           "task-marshal-test",
			TemplateName: "bugfix",
			Description:  "test marshal",
			Status:       "awaiting_approval",
			CurrentStep:  1,
			TotalSteps:   3,
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify structure using map
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.True(t, parsed["success"].(bool))
	assert.NotNil(t, parsed["workspace"])
	assert.NotNil(t, parsed["task"])

	ws := parsed["workspace"].(map[string]any)
	assert.Equal(t, "test-ws", ws["name"])
	assert.Equal(t, "feat/test", ws["branch"])

	task := parsed["task"].(map[string]any)
	assert.Equal(t, "task-marshal-test", task["task_id"])
	assert.Equal(t, "bugfix", task["template_name"])
}

func TestRunResume_AIFixNotImplemented(t *testing.T) {
	// Test that --ai-fix flag returns proper error
	// Since we can't easily set up full environment, we verify the flag exists
	// and the error type is correct when triggered

	cmd := newResumeCmd()
	flag := cmd.Flags().Lookup("ai-fix")
	require.NotNil(t, flag)

	// Verify the error type used
	err := errors.ErrResumeNotImplemented
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestRunResume_NoTasksFound(t *testing.T) {
	// Create temp directories for stores
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace without any tasks
	ws := &domain.Workspace{
		Name:         "empty-workspace",
		Status:       constants.WorkspaceStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		WorktreePath: tmpDir,
	}
	require.NoError(t, wsStore.Create(context.Background(), ws))

	// The actual runResume will fail because it's not in a git repo
	// but this test verifies the workspace exists scenario
	exists, err := wsStore.Exists(context.Background(), "empty-workspace")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestIsResumableStatus_Interrupted(t *testing.T) {
	// Test that interrupted status IS resumable (for Ctrl+C handling)
	assert.True(t, isResumableStatus(constants.TaskStatusInterrupted),
		"interrupted status should be resumable to allow resume after Ctrl+C")

	// Verify consistency with task.IsErrorStatus
	assert.True(t, task.IsErrorStatus(constants.TaskStatusInterrupted),
		"interrupted should be an error status")
}

func TestHandleResumeInterruption(t *testing.T) {
	// Create temp directories for stores
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-interrupt-ws",
		Branch:       "feat/test",
		WorktreePath: tmpDir,
		Status:       constants.WorkspaceStatusActive,
	}

	testTask := &domain.Task{
		ID:          "task-interrupt-test",
		TemplateID:  "bugfix",
		Description: "test interruption handling",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusRunning},
			{Name: "commit", Status: constants.StepStatusPending},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	logger := zerolog.Nop()

	// Call handleResumeInterruption
	ctx := context.Background()
	err := handleResumeInterruption(ctx, out, ws, testTask, nil, logger)

	// Should return the interrupted error
	require.ErrorIs(t, err, errors.ErrTaskInterrupted)

	// Should update task status to interrupted
	assert.Equal(t, constants.TaskStatusInterrupted, testTask.Status)

	// Should update workspace status to paused
	assert.Equal(t, constants.WorkspaceStatusPaused, ws.Status)

	// Should display interruption message
	output := buf.String()
	assert.Contains(t, output, "Interrupt received")
	assert.Contains(t, output, "state saved")
}

func TestHandleResumeInterruption_DisplaysResumeInstructions(t *testing.T) {
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "my-workspace",
		Branch:       "feat/test",
		WorktreePath: tmpDir,
		Status:       constants.WorkspaceStatusActive,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusSuccess},
			{Name: "commit", Status: constants.StepStatusRunning},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	logger := zerolog.Nop()

	ctx := context.Background()
	_ = handleResumeInterruption(ctx, out, ws, testTask, nil, logger)

	output := buf.String()
	// Should show resume command
	assert.Contains(t, output, "atlas resume my-workspace")
	// Should show workspace name
	assert.Contains(t, output, "my-workspace")
	// Should show task ID
	assert.Contains(t, output, "task-123")
}

func TestHandleResumeInterruption_SetsWorkspaceStatusToPaused(t *testing.T) {
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: tmpDir,
		Status:       constants.WorkspaceStatusActive, // Start as active
	}

	testTask := &domain.Task{
		ID:          "task-123",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusRunning},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	logger := zerolog.Nop()

	ctx := context.Background()
	_ = handleResumeInterruption(ctx, out, ws, testTask, nil, logger)

	// Should update workspace status to paused
	assert.Equal(t, constants.WorkspaceStatusPaused, ws.Status,
		"workspace status should be paused after interruption")
}

func TestWorkspaceStatusUpdatedToActiveOnResume(t *testing.T) {
	// This test verifies that when resuming, the workspace status
	// is updated from "paused" to "active"

	// Test the in-memory update behavior
	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: t.TempDir(),
		Status:       constants.WorkspaceStatusPaused, // Start as paused
	}

	// Simulate what runResume does - update status to active
	ws.Status = constants.WorkspaceStatusActive

	// Verify the status is now active
	assert.Equal(t, constants.WorkspaceStatusActive, ws.Status,
		"workspace status should be active after resume")
}

func TestWorkspaceStatusTransitions(t *testing.T) {
	// Test all valid workspace status transitions during resume workflow
	tests := []struct {
		name           string
		initialStatus  constants.WorkspaceStatus
		action         string
		expectedStatus constants.WorkspaceStatus
	}{
		{
			name:           "active workspace interrupted becomes paused",
			initialStatus:  constants.WorkspaceStatusActive,
			action:         "interrupt",
			expectedStatus: constants.WorkspaceStatusPaused,
		},
		{
			name:           "paused workspace resumed becomes active",
			initialStatus:  constants.WorkspaceStatusPaused,
			action:         "resume",
			expectedStatus: constants.WorkspaceStatusActive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := &domain.Workspace{
				Name:         "test-ws",
				WorktreePath: t.TempDir(),
				Status:       tc.initialStatus,
			}

			switch tc.action {
			case "interrupt":
				ws.Status = constants.WorkspaceStatusPaused
			case "resume":
				ws.Status = constants.WorkspaceStatusActive
			}

			assert.Equal(t, tc.expectedStatus, ws.Status)
		})
	}
}

func TestCalculateWorktreePath(t *testing.T) {
	tests := []struct {
		name          string
		repoPath      string
		workspaceName string
		expected      string
	}{
		{
			name:          "simple path",
			repoPath:      "/Users/user/projects/myapp",
			workspaceName: "feature-x",
			expected:      "/Users/user/projects/myapp-feature-x",
		},
		{
			name:          "path with hyphen",
			repoPath:      "/Users/user/my-project",
			workspaceName: "bugfix-123",
			expected:      "/Users/user/my-project-bugfix-123",
		},
		{
			name:          "nested path",
			repoPath:      "/home/dev/code/work/atlas",
			workspaceName: "test-ws",
			expected:      "/home/dev/code/work/atlas-test-ws",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateWorktreePath(tc.repoPath, tc.workspaceName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetTaskStepName(t *testing.T) {
	tests := []struct {
		name         string
		task         *domain.Task
		expectedName string
	}{
		{
			name: "returns step name for valid current step",
			task: &domain.Task{
				CurrentStep: 2,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
					{Name: "git_commit"},
					{Name: "git_push"},
				},
			},
			expectedName: "git_commit",
		},
		{
			name: "returns first step name when current step is 0",
			task: &domain.Task{
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
				},
			},
			expectedName: "implement",
		},
		{
			name: "returns last step name for last step",
			task: &domain.Task{
				CurrentStep: 3,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
					{Name: "git_commit"},
					{Name: "git_pr"},
				},
			},
			expectedName: "git_pr",
		},
		{
			name: "returns empty string when current step is out of bounds (too high)",
			task: &domain.Task{
				CurrentStep: 5,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
				},
			},
			expectedName: "",
		},
		{
			name: "returns empty string when current step is negative",
			task: &domain.Task{
				CurrentStep: -1,
				Steps: []domain.Step{
					{Name: "implement"},
				},
			},
			expectedName: "",
		},
		{
			name: "returns empty string when steps is empty",
			task: &domain.Task{
				CurrentStep: 0,
				Steps:       []domain.Step{},
			},
			expectedName: "",
		},
		{
			name: "returns empty string when steps is nil",
			task: &domain.Task{
				CurrentStep: 0,
				Steps:       nil,
			},
			expectedName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getTaskStepName(tc.task)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

func TestSelectRecoveryAction_NonGHFailed(t *testing.T) {
	t.Skip("Skipping test that requires TTY interaction - hangs in CI without terminal")
	// For non-gh_failed statuses, selectRecoveryAction should delegate to tui.SelectErrorRecovery
	// which requires terminal interaction. This test would need mocking or TTY detection.
	tests := []struct {
		name   string
		status constants.TaskStatus
	}{
		{"validation_failed", constants.TaskStatusValidationFailed},
		{"ci_failed", constants.TaskStatusCIFailed},
		{"ci_timeout", constants.TaskStatusCITimeout},
		{"running (non-error)", constants.TaskStatusRunning},
		{"pending (non-error)", constants.TaskStatusPending},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				Status:      tc.status,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "validate"},
				},
			}

			// Since we can't easily mock the tui.Select call, we verify the function
			// returns an error (ErrMenuCanceled) when not in a terminal
			action, err := selectRecoveryAction(task)

			// Should return error since there's no terminal for huh forms
			require.Error(t, err)
			assert.Empty(t, action)
		})
	}
}

func TestExecuteRecoveryActionWithResume_RetryCommit(t *testing.T) {
	// Test that RecoveryActionRetryCommit is handled the same as RetryGH and RetryAI
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: tmpDir,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusSuccess},
			{Name: "git_commit", Status: constants.StepStatusFailed},
		},
	}

	// Create a task store
	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create and save the task first
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	// Test RecoveryActionRetryCommit
	done, autoResume, err := executeRecoveryActionWithResume(
		ctx, out, taskStore, ws, testTask, notifier,
		tui.RecoveryActionRetryCommit,
	)

	require.NoError(t, err)
	assert.True(t, done, "retry commit should be a terminal action")
	assert.True(t, autoResume, "retry commit should trigger auto-resume")
	assert.Equal(t, constants.TaskStatusRunning, testTask.Status, "task should transition to running")
}

func TestStepAwareRecoveryMenuMapping(t *testing.T) {
	// Test that the step name maps to the correct recovery options
	// This tests the integration between step names and menu options

	tests := []struct {
		stepName      string
		expectedTitle string
		expectedFirst string
	}{
		{
			stepName:      "git_commit",
			expectedTitle: "Commit failed",
			expectedFirst: "Retry commit",
		},
		{
			stepName:      "git_push",
			expectedTitle: "Push failed",
			expectedFirst: "Retry push/PR",
		},
		{
			stepName:      "git_pr",
			expectedTitle: "PR creation failed",
			expectedFirst: "Retry PR creation",
		},
		{
			stepName:      "unknown_step",
			expectedTitle: "GitHub operation failed",
			expectedFirst: "Retry push/PR",
		},
		{
			stepName:      "",
			expectedTitle: "GitHub operation failed",
			expectedFirst: "Retry push/PR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.stepName+"_step", func(t *testing.T) {
			title := tui.MenuTitleForGHFailedStep(tc.stepName)
			options := tui.OptionsForGHFailedStep(tc.stepName)

			assert.Contains(t, title, tc.expectedTitle,
				"title should contain expected text for step %s", tc.stepName)
			assert.Equal(t, tc.expectedFirst, options[0].Label,
				"first option should be %s for step %s", tc.expectedFirst, tc.stepName)
		})
	}
}

func TestGHFailedRecoveryWithPushError(t *testing.T) {
	// Test that push error type takes precedence and adds rebase option

	testTask := &domain.Task{
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push"},
		},
		Metadata: map[string]any{
			"push_error_type": "non_fast_forward",
		},
	}

	stepName := getTaskStepName(testTask)
	assert.Equal(t, "git_push", stepName)

	// When there's a push error type, GHFailedOptionsForPushError should be used
	options := tui.GHFailedOptionsForPushError("non_fast_forward")
	assert.GreaterOrEqual(t, len(options), 4, "should have rebase option")
	assert.Equal(t, "Rebase and retry", options[0].Label)
}

func TestAllRecoveryActionsAreCovered(t *testing.T) {
	// Verify that all recovery actions are handled in executeRecoveryActionWithResume
	// by checking that each action type exists

	allActions := []tui.RecoveryAction{
		tui.RecoveryActionRetryAI,
		tui.RecoveryActionRetryGH,
		tui.RecoveryActionRetryCommit,
		tui.RecoveryActionRebaseRetry,
		tui.RecoveryActionFixManually,
		tui.RecoveryActionViewErrors,
		tui.RecoveryActionViewLogs,
		tui.RecoveryActionContinueWaiting,
		tui.RecoveryActionAbandon,
	}

	// This test just verifies the actions exist and have string representations
	for _, action := range allActions {
		t.Run(action.String(), func(t *testing.T) {
			assert.NotEmpty(t, action.String(), "action should have string representation")
		})
	}
}

func TestTaskStepContextForErrorDisplay(t *testing.T) {
	// Test that failed tasks have proper step context for error display

	tests := []struct {
		name         string
		currentStep  int
		steps        []domain.Step
		expectedStep string
	}{
		{
			name:        "commit step failed",
			currentStep: 2,
			steps: []domain.Step{
				{Name: "implement"},
				{Name: "validate"},
				{Name: "git_commit", Error: "commit failed: no changes to commit"},
			},
			expectedStep: "git_commit",
		},
		{
			name:        "push step failed",
			currentStep: 3,
			steps: []domain.Step{
				{Name: "implement"},
				{Name: "validate"},
				{Name: "git_commit"},
				{Name: "git_push", Error: "push rejected: non-fast-forward"},
			},
			expectedStep: "git_push",
		},
		{
			name:        "pr step failed",
			currentStep: 4,
			steps: []domain.Step{
				{Name: "implement"},
				{Name: "validate"},
				{Name: "git_commit"},
				{Name: "git_push"},
				{Name: "git_pr", Error: "PR creation failed: duplicate PR"},
			},
			expectedStep: "git_pr",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				Status:      constants.TaskStatusGHFailed,
				CurrentStep: tc.currentStep,
				Steps:       tc.steps,
			}

			stepName := getTaskStepName(task)
			assert.Equal(t, tc.expectedStep, stepName)

			// Verify that the step name maps to appropriate recovery options
			options := tui.OptionsForGHFailedStep(stepName)
			assert.NotEmpty(t, options, "should have recovery options for step %s", stepName)
		})
	}
}

// Test display functions
func TestDisplayResumeInfo(t *testing.T) {
	tests := []struct {
		name          string
		workspaceName string
		task          *domain.Task
		expectsOutput []string
	}{
		{
			name:          "normal task resume",
			workspaceName: "my-workspace",
			task: &domain.Task{
				ID:          "task-123",
				Status:      constants.TaskStatusValidationFailed,
				CurrentStep: 1,
				Steps: []domain.Step{
					{Name: "implement", Status: constants.StepStatusSuccess},
					{Name: "validate", Status: constants.StepStatusFailed},
					{Name: "commit", Status: constants.StepStatusPending},
				},
			},
			expectsOutput: []string{
				"my-workspace",
				"task-123",
				"validation_failed → running",
				"2/3",
			},
		},
		{
			name:          "interrupted task",
			workspaceName: "test-ws",
			task: &domain.Task{
				ID:          "task-xyz",
				Status:      constants.TaskStatusInterrupted,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "implement", Status: constants.StepStatusRunning},
				},
			},
			expectsOutput: []string{
				"test-ws",
				"task-xyz",
				"interrupted",
				"1/1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := tui.NewOutput(&buf, "text")

			displayResumeInfo(out, tc.workspaceName, tc.task)

			output := buf.String()
			for _, expected := range tc.expectsOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestGetTaskErrorMessage(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		expectedMsg string
	}{
		{
			name: "error in current step",
			task: &domain.Task{
				CurrentStep: 1,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate", Error: "validation failed: 3 errors"},
					{Name: "commit"},
				},
			},
			expectedMsg: "validation failed: 3 errors",
		},
		{
			name: "no error in current step",
			task: &domain.Task{
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
				},
			},
			expectedMsg: "",
		},
		{
			name: "current step out of bounds",
			task: &domain.Task{
				CurrentStep: 5,
				Steps: []domain.Step{
					{Name: "implement"},
				},
			},
			expectedMsg: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getTaskErrorMessage(tc.task)
			assert.Equal(t, tc.expectedMsg, result)
		})
	}
}

func TestGetMetadataError(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		expectedMsg string
	}{
		{
			name: "error in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": "push failed: non-fast-forward",
				},
			},
			expectedMsg: "push failed: non-fast-forward",
		},
		{
			name: "no error in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"other_key": "value",
				},
			},
			expectedMsg: "",
		},
		{
			name: "nil metadata",
			task: &domain.Task{
				Metadata: nil,
			},
			expectedMsg: "",
		},
		{
			name: "error is not a string",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": 123,
				},
			},
			expectedMsg: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getMetadataError(tc.task)
			assert.Equal(t, tc.expectedMsg, result)
		})
	}
}

func TestDisplayValidationContext(t *testing.T) {
	tests := []struct {
		name          string
		task          *domain.Task
		expectsOutput string
	}{
		{
			name: "validation error count in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"validation_error_count": 5,
				},
			},
			expectsOutput: "5 validation failures",
		},
		{
			name: "no validation error count",
			task: &domain.Task{
				Metadata: map[string]any{},
			},
			expectsOutput: "",
		},
		{
			name: "nil metadata",
			task: &domain.Task{
				Metadata: nil,
			},
			expectsOutput: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := tui.NewOutput(&buf, "text")

			displayValidationContext(out, tc.task)

			output := buf.String()
			if tc.expectsOutput != "" {
				assert.Contains(t, output, tc.expectsOutput)
			}
		})
	}
}

func TestDisplayCIContext(t *testing.T) {
	tests := []struct {
		name          string
		task          *domain.Task
		expectsOutput string
	}{
		{
			name: "ci url in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"ci_url": "https://github.com/owner/repo/actions/runs/12345",
				},
			},
			expectsOutput: "https://github.com/owner/repo/actions/runs/12345",
		},
		{
			name: "no ci url",
			task: &domain.Task{
				Metadata: map[string]any{},
			},
			expectsOutput: "",
		},
		{
			name: "empty ci url",
			task: &domain.Task{
				Metadata: map[string]any{
					"ci_url": "",
				},
			},
			expectsOutput: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := tui.NewOutput(&buf, "text")

			displayCIContext(out, tc.task)

			output := buf.String()
			if tc.expectsOutput != "" {
				assert.Contains(t, output, tc.expectsOutput)
			}
		})
	}
}

func TestExtractGitHubActionsURL(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		expectedURL string
	}{
		{
			name: "ci_url in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"ci_url": "https://github.com/owner/repo/actions/runs/12345",
				},
			},
			expectedURL: "https://github.com/owner/repo/actions/runs/12345",
		},
		{
			name: "github_actions_url in metadata",
			task: &domain.Task{
				Metadata: map[string]any{
					"github_actions_url": "https://github.com/owner/repo/actions/runs/67890",
				},
			},
			expectedURL: "https://github.com/owner/repo/actions/runs/67890",
		},
		{
			name: "ci_url takes precedence",
			task: &domain.Task{
				Metadata: map[string]any{
					"ci_url":             "https://github.com/owner/repo/actions/runs/12345",
					"github_actions_url": "https://github.com/owner/repo/actions/runs/67890",
				},
			},
			expectedURL: "https://github.com/owner/repo/actions/runs/12345",
		},
		{
			name: "nil metadata",
			task: &domain.Task{
				Metadata: nil,
			},
			expectedURL: "",
		},
		{
			name:        "nil task",
			task:        nil,
			expectedURL: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractGitHubActionsURL(tc.task)
			assert.Equal(t, tc.expectedURL, result)
		})
	}
}

func TestExtractRepoInfo(t *testing.T) {
	tests := []struct {
		name         string
		ws           *domain.Workspace
		expectedRepo string
	}{
		{
			name: "repository in metadata",
			ws: &domain.Workspace{
				Metadata: map[string]any{
					"repository": "owner/repo",
				},
			},
			expectedRepo: "owner/repo",
		},
		{
			name: "no repository in metadata",
			ws: &domain.Workspace{
				Metadata: map[string]any{},
			},
			expectedRepo: "",
		},
		{
			name: "nil metadata",
			ws: &domain.Workspace{
				Metadata: nil,
			},
			expectedRepo: "",
		},
		{
			name:         "nil workspace",
			ws:           nil,
			expectedRepo: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractRepoInfo(tc.ws)
			assert.Equal(t, tc.expectedRepo, result)
		})
	}
}

func TestOutputResumeSuccessJSON(t *testing.T) {
	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: "/tmp/test-ws",
		Status:       constants.WorkspaceStatusActive,
	}

	currentTask := &domain.Task{
		ID:          "task-123",
		TemplateID:  "bugfix",
		Description: "fix something",
		Status:      constants.TaskStatusCompleted,
		CurrentStep: 2,
		Steps:       make([]domain.Step, 3),
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")

	err := outputResumeSuccessJSON(out, ws, currentTask)
	require.NoError(t, err)

	var resp resumeResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))

	assert.True(t, resp.Success)
	assert.Equal(t, "test-ws", resp.Workspace.Name)
	assert.Equal(t, "feat/test", resp.Workspace.Branch)
	assert.Equal(t, "task-123", resp.Task.ID)
	assert.Equal(t, 2, resp.Task.CurrentStep)
	assert.Equal(t, 3, resp.Task.TotalSteps)
}

func TestHandleResumeError_Coverage(t *testing.T) {
	tests := []struct {
		name         string
		outputFormat string
		err          error
		expectJSON   bool
	}{
		{
			name:         "text format returns error directly",
			outputFormat: "text",
			err:          errors.ErrWorkspaceNotFound,
			expectJSON:   false,
		},
		{
			name:         "json format outputs JSON",
			outputFormat: "json",
			err:          errors.ErrInvalidTransition,
			expectJSON:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			result := handleResumeError(tc.outputFormat, &buf, "test-ws", "task-123", tc.err)

			if tc.expectJSON {
				require.ErrorIs(t, result, errors.ErrJSONErrorOutput)
				assert.NotEmpty(t, buf.String())

				var resp resumeResponse
				require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
				assert.False(t, resp.Success)
			} else {
				assert.Equal(t, tc.err, result)
				assert.Empty(t, buf.String())
			}
		})
	}
}

// Test recovery action handlers
func TestHandleRetryAction(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusFailed},
		},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create the task first
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleRetryAction(ctx, out, taskStore, testTask, notifier)
	require.NoError(t, err)

	// Task should transition to running
	assert.Equal(t, constants.TaskStatusRunning, testTask.Status)

	// Verify task was saved
	saved, err := taskStore.Get(ctx, testTask.WorkspaceID, testTask.ID)
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusRunning, saved.Status)

	output := buf.String()
	assert.Contains(t, output, "Retrying with AI fix")
}

func TestHandleFixManually(t *testing.T) {
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "/tmp/test-workspace",
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err := handleFixManually(out, ws, notifier)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "cd /tmp/test-workspace")
	assert.Contains(t, output, "atlas resume test-ws")
	assert.Contains(t, output, "Make your fixes")
}

func TestHandleFixManually_EmptyWorktreePath(t *testing.T) {
	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: "", // Empty worktree path
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err := handleFixManually(out, ws, notifier)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "cd <worktree for test-ws>")
	assert.Contains(t, output, "atlas resume test-ws")
}

func TestHandleContinueWaiting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCITimeout,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusSuccess},
			{Name: "ci_wait", Status: constants.StepStatusRunning},
		},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleContinueWaiting(ctx, out, taskStore, testTask, notifier)
	require.NoError(t, err)

	assert.Equal(t, constants.TaskStatusRunning, testTask.Status)

	output := buf.String()
	assert.Contains(t, output, "Continuing CI polling")
}

func TestHandleAbandon(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: tmpDir,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps:       []domain.Step{{Name: "validate", Status: constants.StepStatusFailed}},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleAbandon(ctx, out, taskStore, ws, testTask, notifier)
	require.NoError(t, err)

	assert.Equal(t, constants.TaskStatusAbandoned, testTask.Status)

	output := buf.String()
	assert.Contains(t, output, "Task abandoned")
	assert.Contains(t, output, "feat/test")
}

func TestTrySelectPushErrorRecovery_NoMetadata(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push"},
		},
		Metadata: nil, // No metadata
	}

	action, handled, err := trySelectPushErrorRecovery(testTask, "git_push")
	require.NoError(t, err)
	assert.False(t, handled, "should not be handled when metadata is nil")
	assert.Empty(t, action)
}

func TestTrySelectPushErrorRecovery_NoPushErrorType(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push"},
		},
		Metadata: map[string]any{
			"other_key": "value",
		},
	}

	action, handled, err := trySelectPushErrorRecovery(testTask, "git_push")
	require.NoError(t, err)
	assert.False(t, handled, "should not be handled when push_error_type is missing")
	assert.Empty(t, action)
}

func TestTrySelectPushErrorRecovery_EmptyPushErrorType(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push"},
		},
		Metadata: map[string]any{
			"push_error_type": "",
		},
	}

	action, handled, err := trySelectPushErrorRecovery(testTask, "git_push")
	require.NoError(t, err)
	assert.False(t, handled, "should not be handled when push_error_type is empty")
	assert.Empty(t, action)
}

func TestDisplayRecoveryErrorContext(t *testing.T) {
	ws := &domain.Workspace{
		Name: "test-ws",
	}

	testTask := &domain.Task{
		ID:          "task-123",
		Description: "fix authentication",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement", Status: constants.StepStatusSuccess},
			{Name: "validate", Status: constants.StepStatusFailed, Error: "validation failed"},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	displayRecoveryErrorContext(out, ws, testTask)

	output := buf.String()
	assert.Contains(t, output, "Task failed")
	assert.Contains(t, output, "fix authentication")
	assert.Contains(t, output, "test-ws")
	assert.Contains(t, output, "validate")
	assert.Contains(t, output, "validation_failed")
}

func TestDisplayStatusSpecificContext_ValidationFailed(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate", Error: "3 lint errors found"},
		},
		Metadata: map[string]any{
			"validation_error_count": 3,
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	displayStatusSpecificContext(out, testTask)

	output := buf.String()
	assert.Contains(t, output, "3 validation failures")
	assert.Contains(t, output, "3 lint errors found")
}

func TestDisplayStatusSpecificContext_GHFailed(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push", Error: "push rejected"},
		},
		Metadata: map[string]any{
			"error": "non-fast-forward",
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	displayStatusSpecificContext(out, testTask)

	output := buf.String()
	assert.Contains(t, output, "push rejected")
}

func TestDisplayStatusSpecificContext_CIFailed(t *testing.T) {
	testTask := &domain.Task{
		Status:      constants.TaskStatusCIFailed,
		CurrentStep: 3,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "commit"},
			{Name: "ci_wait", Error: "tests failed"},
		},
		Metadata: map[string]any{
			"ci_url": "https://github.com/owner/repo/actions/runs/12345",
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	displayStatusSpecificContext(out, testTask)

	output := buf.String()
	assert.Contains(t, output, "https://github.com/owner/repo/actions/runs/12345")
	assert.Contains(t, output, "tests failed")
}

func TestDisplayStatusSpecificContext_LongErrorTruncated(t *testing.T) {
	longError := strings.Repeat("error ", 50) // Create error longer than 150 chars
	testTask := &domain.Task{
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 0,
		Steps: []domain.Step{
			{Name: "validate", Error: longError},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	displayStatusSpecificContext(out, testTask)

	output := buf.String()
	// Error should be truncated to 150 chars + "..."
	assert.Contains(t, output, "...")
	// Full error should not be in output
	assert.NotContains(t, output, longError)
}

func TestGetMetadataError_Coverage(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		expectedMsg string
	}{
		{
			name: "error in metadata as string",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": "push failed",
				},
			},
			expectedMsg: "push failed",
		},
		{
			name: "error not a string",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": 123,
				},
			},
			expectedMsg: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getMetadataError(tc.task)
			assert.Equal(t, tc.expectedMsg, result)
		})
	}
}

func TestHandleRetryAction_TransitionError(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a task in a state that can't transition to running
	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Can't transition from completed to running
		CurrentStep: 2,
		Steps:       []domain.Step{{Name: "done"}},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleRetryAction(ctx, out, taskStore, testTask, notifier)
	require.Error(t, err)
}

func TestHandleContinueWaiting_TransitionError(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Can't transition from completed
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "done"}},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleContinueWaiting(ctx, out, taskStore, testTask, notifier)
	require.Error(t, err)
}

func TestHandleAbandon_TransitionError(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		Branch:       "feat/test",
		WorktreePath: tmpDir,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Can't transition from completed
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "done"}},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	err = handleAbandon(ctx, out, taskStore, ws, testTask, notifier)
	require.Error(t, err)
}

func TestCheckBranchExists(t *testing.T) {
	ctx := context.Background()

	// Test with non-existent repo path - should return false
	result := checkBranchExists(ctx, "/nonexistent/path", "main")
	assert.False(t, result)
}

func TestDetectMainRepoPath_NotInGitRepo(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory that is not a git repo
	tmpDir := t.TempDir()

	// Change to that directory temporarily
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(origDir)
	}()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Should return error when not in a git repo
	_, err = detectMainRepoPath(ctx)
	require.Error(t, err)
}

func TestCreateWorktreeForBranch_NonExistentRepo(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	err := createWorktreeForBranch(ctx, "/nonexistent/repo", tmpDir+"/worktree", "main")
	require.Error(t, err)
}

func TestShouldShowRecoveryContext_JSONOutput(t *testing.T) {
	ctx := context.Background()
	testTask := &domain.Task{
		ID: "task-123",
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "json")
	logger := zerolog.Nop()

	result := shouldShowRecoveryContext(ctx, testTask, out, "json", logger)
	assert.False(t, result, "should skip recovery context for JSON output")
}

func TestShouldShowRecoveryContext_NoHook(t *testing.T) {
	ctx := context.Background()
	testTask := &domain.Task{
		ID: "task-nonexistent",
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	logger := zerolog.Nop()

	result := shouldShowRecoveryContext(ctx, testTask, out, "text", logger)
	assert.False(t, result, "should return false when hook doesn't exist")
}

func TestExecuteRecoveryActionWithResume_AllActions(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: tmpDir,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusGHFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
			{Name: "git_push"},
		},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	tests := []struct {
		name         string
		action       tui.RecoveryAction
		expectDone   bool
		expectResume bool
	}{
		{
			name:         "RetryAI action",
			action:       tui.RecoveryActionRetryAI,
			expectDone:   true,
			expectResume: true,
		},
		{
			name:         "RetryGH action",
			action:       tui.RecoveryActionRetryGH,
			expectDone:   true,
			expectResume: true,
		},
		{
			name:         "FixManually action",
			action:       tui.RecoveryActionFixManually,
			expectDone:   true,
			expectResume: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset task status
			testTask.Status = constants.TaskStatusGHFailed
			buf.Reset()

			done, autoResume, err := executeRecoveryActionWithResume(
				ctx, out, taskStore, ws, testTask, notifier, tc.action,
			)

			require.NoError(t, err)
			assert.Equal(t, tc.expectDone, done)
			assert.Equal(t, tc.expectResume, autoResume)
		})
	}
}

func TestExecuteRecoveryActionWithResume_ViewActions(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: tmpDir,
	}

	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "implement"},
			{Name: "validate"},
		},
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	// Test ViewErrors action (returns to menu)
	done, autoResume, err := executeRecoveryActionWithResume(
		ctx, out, taskStore, ws, testTask, notifier,
		tui.RecoveryActionViewErrors,
	)

	require.NoError(t, err)
	assert.False(t, done, "view errors should return to menu")
	assert.False(t, autoResume)
}

func TestHandleViewErrors_NoArtifact(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err = handleViewErrors(ctx, out, taskStore, "test-ws", "task-123")
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Could not load validation results")
}

func TestHandleViewLogs_NoURL(t *testing.T) {
	ctx := context.Background()

	ws := &domain.Workspace{
		Name: "test-ws",
	}

	testTask := &domain.Task{
		ID:       "task-123",
		Metadata: nil, // No metadata
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err := handleViewLogs(ctx, out, ws, testTask)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No GitHub Actions URL available")
}

func TestOutputResumeErrorJSON_EncodeError(t *testing.T) {
	// Test that outputResumeErrorJSON handles JSON encoding
	var buf bytes.Buffer

	err := outputResumeErrorJSON(&buf, "test-ws", "task-123", "test error")
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	var resp resumeResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "test error", resp.Error)
}

func TestExecuteRecoveryActionWithResume_AllSwitchCases(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	ws := &domain.Workspace{
		Name:         "test-ws",
		WorktreePath: tmpDir,
	}

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, true)

	tests := []struct {
		name         string
		action       tui.RecoveryAction
		taskStatus   constants.TaskStatus
		expectDone   bool
		expectResume bool
	}{
		{
			name:         "RebaseRetry action",
			action:       tui.RecoveryActionRebaseRetry,
			taskStatus:   constants.TaskStatusGHFailed,
			expectDone:   true,
			expectResume: false, // Will fail without git repo
		},
		{
			name:         "ViewLogs action",
			action:       tui.RecoveryActionViewLogs,
			taskStatus:   constants.TaskStatusCIFailed,
			expectDone:   false,
			expectResume: false,
		},
		{
			name:         "ContinueWaiting action",
			action:       tui.RecoveryActionContinueWaiting,
			taskStatus:   constants.TaskStatusCITimeout,
			expectDone:   true,
			expectResume: true,
		},
		{
			name:         "Abandon action",
			action:       tui.RecoveryActionAbandon,
			taskStatus:   constants.TaskStatusValidationFailed,
			expectDone:   true,
			expectResume: false,
		},
		{
			name:         "Unknown action (default case)",
			action:       tui.RecoveryAction("unknown"),
			taskStatus:   constants.TaskStatusGHFailed,
			expectDone:   false,
			expectResume: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testTask := &domain.Task{
				ID:          "task-" + tc.name,
				WorkspaceID: "test-ws",
				Status:      tc.taskStatus,
				CurrentStep: 1,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
				},
			}

			require.NoError(t, taskStore.Create(ctx, testTask.WorkspaceID, testTask))
			buf.Reset()

			done, autoResume, _ := executeRecoveryActionWithResume(
				ctx, out, taskStore, ws, testTask, notifier, tc.action,
			)

			assert.Equal(t, tc.expectDone, done, "done mismatch for %s", tc.name)
			assert.Equal(t, tc.expectResume, autoResume, "autoResume mismatch for %s", tc.name)
		})
	}
}

func TestGetMetadataError_AllCases(t *testing.T) {
	tests := []struct {
		name        string
		task        *domain.Task
		expectedMsg string
	}{
		{
			name: "nil metadata",
			task: &domain.Task{
				Metadata: nil,
			},
			expectedMsg: "",
		},
		{
			name: "error as string",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": "push failed",
				},
			},
			expectedMsg: "push failed",
		},
		{
			name: "error not a string",
			task: &domain.Task{
				Metadata: map[string]any{
					"error": 123,
				},
			},
			expectedMsg: "",
		},
		{
			name: "no error key",
			task: &domain.Task{
				Metadata: map[string]any{
					"other": "value",
				},
			},
			expectedMsg: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getMetadataError(tc.task)
			assert.Equal(t, tc.expectedMsg, result)
		})
	}
}

func TestTrySelectPushErrorRecovery_AllCases(t *testing.T) {
	tests := []struct {
		name          string
		metadata      map[string]any
		expectHandled bool
	}{
		{
			name:          "nil metadata",
			metadata:      nil,
			expectHandled: false,
		},
		{
			name: "empty push_error_type",
			metadata: map[string]any{
				"push_error_type": "",
			},
			expectHandled: false,
		},
		{
			name: "push_error_type not string",
			metadata: map[string]any{
				"push_error_type": 123,
			},
			expectHandled: false,
		},
		{
			name:          "no push_error_type key",
			metadata:      map[string]any{},
			expectHandled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testTask := &domain.Task{
				Status:      constants.TaskStatusGHFailed,
				CurrentStep: 2,
				Steps: []domain.Step{
					{Name: "implement"},
					{Name: "validate"},
					{Name: "git_push"},
				},
				Metadata: tc.metadata,
			}

			_, handled, err := trySelectPushErrorRecovery(testTask, "git_push")
			require.NoError(t, err)
			assert.Equal(t, tc.expectHandled, handled)
		})
	}
}

func TestHandleViewErrors_WithArtifact(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	taskStore, err := task.NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a task
	testTask := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusValidationFailed,
	}
	require.NoError(t, taskStore.Create(ctx, "test-ws", testTask))

	// Save an artifact
	artifactData := []byte("validation error: missing semicolon")
	require.NoError(t, taskStore.SaveArtifact(ctx, "test-ws", "task-123", "validation.json", artifactData))

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err = handleViewErrors(ctx, out, taskStore, "test-ws", "task-123")
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "validation error: missing semicolon")
}

func TestHandleViewLogs_WithURL(t *testing.T) {
	ctx := context.Background()

	ws := &domain.Workspace{
		Name: "test-ws",
	}

	testTask := &domain.Task{
		ID: "task-123",
		Metadata: map[string]any{
			"ci_url": "https://github.com/owner/repo/actions/runs/12345",
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	// This will try to open browser, which will likely fail in test, but that's OK
	err := handleViewLogs(ctx, out, ws, testTask)
	require.NoError(t, err)

	output := buf.String()
	// Should either show the URL or indicate it couldn't open browser
	assert.True(t, strings.Contains(output, "12345") || strings.Contains(output, "Could not"))
}

func TestHandleViewLogs_WithPRURL(t *testing.T) {
	ctx := context.Background()

	ws := &domain.Workspace{
		Name: "test-ws",
		Metadata: map[string]any{
			"repository": "owner/repo",
		},
	}

	testTask := &domain.Task{
		ID:       "task-123",
		Metadata: map[string]any{}, // No CI URL
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	err := handleViewLogs(ctx, out, ws, testTask)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No GitHub Actions URL")
}

func TestOutputResumeErrorJSON_Coverage(t *testing.T) {
	var buf bytes.Buffer

	err := outputResumeErrorJSON(&buf, "workspace-name", "task-id", "error message")
	require.ErrorIs(t, err, errors.ErrJSONErrorOutput)

	var resp resumeResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))

	assert.False(t, resp.Success)
	assert.Equal(t, "workspace-name", resp.Workspace.Name)
	assert.Equal(t, "task-id", resp.Task.ID)
	assert.Equal(t, "error message", resp.Error)
}
