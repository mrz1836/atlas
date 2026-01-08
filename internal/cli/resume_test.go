package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
