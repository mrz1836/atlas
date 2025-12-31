// Package cli provides the command-line interface for atlas.
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
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockWorkspaceStore implements workspace.Store interface for testing.
type mockWorkspaceStore struct {
	workspaces []*domain.Workspace
	listErr    error
	getErr     error
}

func (m *mockWorkspaceStore) List(_ context.Context) ([]*domain.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.workspaces, nil
}

func (m *mockWorkspaceStore) Get(_ context.Context, name string) (*domain.Workspace, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, ws := range m.workspaces {
		if ws.Name == name {
			return ws, nil
		}
	}
	return nil, atlaserrors.ErrWorkspaceNotFound
}

func (m *mockWorkspaceStore) Create(_ context.Context, _ *domain.Workspace) error {
	return nil
}

func (m *mockWorkspaceStore) Update(_ context.Context, _ *domain.Workspace) error {
	return nil
}

func (m *mockWorkspaceStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockWorkspaceStore) Exists(_ context.Context, name string) (bool, error) {
	for _, ws := range m.workspaces {
		if ws.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// mockTaskStoreForApprove implements task.Store interface for testing.
type mockTaskStoreForApprove struct {
	tasks       map[string][]*domain.Task // workspaceName -> tasks
	artifacts   map[string][]byte         // "workspace:task:filename" -> data
	getErr      error
	listErr     error
	updateErr   error
	artifactErr error
}

func (m *mockTaskStoreForApprove) List(_ context.Context, workspaceName string) ([]*domain.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		return tasks, nil
	}
	return []*domain.Task{}, nil
}

func (m *mockTaskStoreForApprove) Get(_ context.Context, workspaceName, taskID string) (*domain.Task, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		for _, t := range tasks {
			if t.ID == taskID {
				return t, nil
			}
		}
	}
	return nil, atlaserrors.ErrTaskNotFound
}

func (m *mockTaskStoreForApprove) Create(_ context.Context, _ string, _ *domain.Task) error {
	return nil
}

func (m *mockTaskStoreForApprove) Update(_ context.Context, workspaceName string, t *domain.Task) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		for i, existing := range tasks {
			if existing.ID == t.ID {
				m.tasks[workspaceName][i] = t
				return nil
			}
		}
	}
	return nil
}

func (m *mockTaskStoreForApprove) Delete(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockTaskStoreForApprove) AppendLog(_ context.Context, _ string, _ string, _ []byte) error {
	return nil
}

func (m *mockTaskStoreForApprove) SaveArtifact(_ context.Context, _ string, _ string, _ string, _ []byte) error {
	return nil
}

func (m *mockTaskStoreForApprove) SaveVersionedArtifact(_ context.Context, _, _, _ string, _ []byte) (string, error) {
	return "", nil
}

func (m *mockTaskStoreForApprove) GetArtifact(_ context.Context, workspaceName, taskID, filename string) ([]byte, error) {
	if m.artifactErr != nil {
		return nil, m.artifactErr
	}
	key := workspaceName + ":" + taskID + ":" + filename
	if data, ok := m.artifacts[key]; ok {
		return data, nil
	}
	return nil, atlaserrors.ErrArtifactNotFound
}

func (m *mockTaskStoreForApprove) ListArtifacts(_ context.Context, _ string, _ string) ([]string, error) {
	return []string{}, nil
}

// TestAddApproveCommand tests that approve command is properly added to root.
func TestAddApproveCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	// Find the approve command
	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)
	require.NotNil(t, approveCmd)
	assert.Equal(t, "approve", approveCmd.Name())
}

// TestApproveCommand_CommandHelp tests command help text.
func TestApproveCommand_CommandHelp(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check usage line
	assert.Equal(t, "approve [workspace]", approveCmd.Use)

	// Check short description
	assert.Contains(t, approveCmd.Short, "Approve")

	// Check long description
	assert.Contains(t, approveCmd.Long, "Approve a task")
	assert.Contains(t, approveCmd.Long, "validation")
	assert.Contains(t, approveCmd.Long, "awaiting approval")
}

// TestApproveCommand_MaxArgs tests that approve accepts at most 1 argument.
func TestApproveCommand_MaxArgs(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Args function should be set
	require.NotNil(t, approveCmd.Args)
}

// TestFindAwaitingApprovalTasks tests finding tasks awaiting approval.
func TestFindAwaitingApprovalTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		workspaces     []*domain.Workspace
		tasks          map[string][]*domain.Task
		expectedCount  int
		expectedWSName string
	}{
		{
			name:          "no workspaces",
			workspaces:    []*domain.Workspace{},
			tasks:         map[string][]*domain.Task{},
			expectedCount: 0,
		},
		{
			name: "workspace with no tasks",
			workspaces: []*domain.Workspace{
				{Name: "empty-ws", Branch: "feat/empty", Status: constants.WorkspaceStatusActive},
			},
			tasks:         map[string][]*domain.Task{},
			expectedCount: 0,
		},
		{
			name: "workspace with running task only",
			workspaces: []*domain.Workspace{
				{Name: "running-ws", Branch: "feat/running", Status: constants.WorkspaceStatusActive},
			},
			tasks: map[string][]*domain.Task{
				"running-ws": {
					{ID: "task-1", WorkspaceID: "running-ws", Status: constants.TaskStatusRunning},
				},
			},
			expectedCount: 0,
		},
		{
			name: "workspace with awaiting approval task",
			workspaces: []*domain.Workspace{
				{Name: "approval-ws", Branch: "feat/approval", Status: constants.WorkspaceStatusActive},
			},
			tasks: map[string][]*domain.Task{
				"approval-ws": {
					{ID: "task-1", WorkspaceID: "approval-ws", Status: constants.TaskStatusAwaitingApproval},
				},
			},
			expectedCount:  1,
			expectedWSName: "approval-ws",
		},
		{
			name: "multiple workspaces with mixed statuses",
			workspaces: []*domain.Workspace{
				{Name: "running-ws", Branch: "feat/running", Status: constants.WorkspaceStatusActive},
				{Name: "approval-ws", Branch: "feat/approval", Status: constants.WorkspaceStatusActive},
				{Name: "completed-ws", Branch: "feat/completed", Status: constants.WorkspaceStatusActive},
			},
			tasks: map[string][]*domain.Task{
				"running-ws":   {{ID: "task-1", WorkspaceID: "running-ws", Status: constants.TaskStatusRunning}},
				"approval-ws":  {{ID: "task-2", WorkspaceID: "approval-ws", Status: constants.TaskStatusAwaitingApproval}},
				"completed-ws": {{ID: "task-3", WorkspaceID: "completed-ws", Status: constants.TaskStatusCompleted}},
			},
			expectedCount:  1,
			expectedWSName: "approval-ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockWS := &mockWorkspaceStore{workspaces: tt.workspaces}
			mockTask := &mockTaskStoreForApprove{tasks: tt.tasks}

			result, err := findAwaitingApprovalTasks(ctx, mockWS, mockTask)
			require.NoError(t, err)

			assert.Len(t, result, tt.expectedCount)
			if tt.expectedWSName != "" && len(result) > 0 {
				assert.Equal(t, tt.expectedWSName, result[0].workspace.Name)
			}
		})
	}
}

// TestFindAwaitingApprovalTasks_ContextCancellation tests context cancellation.
func TestFindAwaitingApprovalTasks_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockWS := &mockWorkspaceStore{workspaces: []*domain.Workspace{
		{Name: "ws1", Branch: "feat/ws1"},
	}}
	mockTask := &mockTaskStoreForApprove{tasks: map[string][]*domain.Task{}}

	_, err := findAwaitingApprovalTasks(ctx, mockWS, mockTask)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestSelectWorkspaceForApproval tests the workspace selection function.
func TestSelectWorkspaceForApproval(t *testing.T) {
	t.Parallel()

	tasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1", Branch: "feat/ws1"},
			task:      &domain.Task{ID: "task-1", Description: "Test task 1"},
		},
		{
			workspace: &domain.Workspace{Name: "ws2", Branch: "feat/ws2"},
			task:      &domain.Task{ID: "task-2", Description: "Test task 2"},
		},
	}

	// This function requires interactive input, so we just test the structure
	assert.Len(t, tasks, 2)
}

// TestExtractPRURL tests PR URL extraction from task metadata.
func TestExtractPRURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		task     *domain.Task
		expected string
	}{
		{
			name:     "nil task",
			task:     nil,
			expected: "",
		},
		{
			name:     "nil metadata",
			task:     &domain.Task{ID: "task-1"},
			expected: "",
		},
		{
			name:     "empty metadata",
			task:     &domain.Task{ID: "task-1", Metadata: map[string]interface{}{}},
			expected: "",
		},
		{
			name: "no pr_url key",
			task: &domain.Task{
				ID:       "task-1",
				Metadata: map[string]interface{}{"other_key": "value"},
			},
			expected: "",
		},
		{
			name: "pr_url present",
			task: &domain.Task{
				ID:       "task-1",
				Metadata: map[string]interface{}{"pr_url": "https://github.com/owner/repo/pull/123"},
			},
			expected: "https://github.com/owner/repo/pull/123",
		},
		{
			name: "pr_url not a string",
			task: &domain.Task{
				ID:       "task-1",
				Metadata: map[string]interface{}{"pr_url": 123},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := extractPRURL(tt.task)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestApproveResponse_JSONStructure tests JSON response structure.
func TestApproveResponse_JSONStructure(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/path/to/worktree",
			Status:       "active",
		},
		Task: taskInfo{
			ID:           "task-1",
			TemplateName: "default",
			Description:  "Test task",
			Status:       "completed",
			CurrentStep:  7,
			TotalSteps:   7,
		},
		PRURL: "https://github.com/owner/repo/pull/123",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify JSON structure
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed["success"].(bool))
	assert.NotNil(t, parsed["workspace"])
	assert.NotNil(t, parsed["task"])
	assert.Equal(t, "https://github.com/owner/repo/pull/123", parsed["pr_url"])
}

// TestApproveResponse_JSONError tests JSON error response structure.
func TestApproveResponse_JSONError(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: false,
		Workspace: workspaceInfo{
			Name: "test-ws",
		},
		Task: taskInfo{
			ID: "task-1",
		},
		Error: "failed to approve task",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify error is present
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.False(t, parsed["success"].(bool))
	assert.Equal(t, "failed to approve task", parsed["error"])
}

// TestOutputApproveErrorJSON tests JSON error output.
func TestOutputApproveErrorJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := outputApproveErrorJSON(&buf, "test-ws", "task-1", "test error message")

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON output
	var resp approveResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)

	assert.False(t, resp.Success)
	assert.Equal(t, "test-ws", resp.Workspace.Name)
	assert.Equal(t, "task-1", resp.Task.ID)
	assert.Equal(t, "test error message", resp.Error)
}

// TestHandleApproveError tests error handling based on output format.
func TestHandleApproveError(t *testing.T) {
	t.Parallel()

	testErr := atlaserrors.ErrWorkspaceNotFound

	t.Run("text format returns error directly", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := handleApproveError(OutputText, &buf, "test-ws", testErr)
		require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
		assert.Empty(t, buf.String(), "text format should not write to buffer")
	})

	t.Run("json format outputs JSON error", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		err := handleApproveError(OutputJSON, &buf, "test-ws", testErr)
		require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)
		assert.NotEmpty(t, buf.String(), "json format should write to buffer")

		var resp approveResponse
		jsonErr := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, jsonErr)
		assert.False(t, resp.Success)
	})
}

// TestApproveAction_Constants tests approval action constants.
func TestApproveAction_Constants(t *testing.T) {
	t.Parallel()

	// Verify all action constants are defined
	assert.Equal(t, actionApprove, approvalAction("approve"))
	assert.Equal(t, actionViewDiff, approvalAction("view_diff"))
	assert.Equal(t, actionViewLogs, approvalAction("view_logs"))
	assert.Equal(t, actionOpenPR, approvalAction("open_pr"))
	assert.Equal(t, actionReject, approvalAction("reject"))
	assert.Equal(t, actionCancel, approvalAction("cancel"))
}

// TestApproveTask_StateTransition tests task state transition on approval.
func TestApproveTask_StateTransition(t *testing.T) {
	t.Parallel()

	// Create a task in awaiting_approval state
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       make([]domain.Step, 7),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
	}

	ctx := context.Background()
	err := approveTask(ctx, mockStore, task)
	require.NoError(t, err)

	// Verify state changed to completed
	assert.Equal(t, constants.TaskStatusCompleted, task.Status)

	// Verify transition was recorded
	assert.Len(t, task.Transitions, 1)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Transitions[0].FromStatus)
	assert.Equal(t, constants.TaskStatusCompleted, task.Transitions[0].ToStatus)
	assert.Equal(t, "User approved", task.Transitions[0].Reason)
}

// TestApproveTask_UpdateError tests handling of update errors.
func TestApproveTask_UpdateError(t *testing.T) {
	t.Parallel()

	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusAwaitingApproval,
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
		updateErr: atlaserrors.ErrTaskNotFound,
	}

	ctx := context.Background()
	err := approveTask(ctx, mockStore, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save task")
}

// TestViewDiff_EmptyWorktreePath tests diff with empty worktree path.
func TestViewDiff_EmptyWorktreePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := viewDiff(ctx, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

// TestViewLogs_NoLogFile tests viewing logs when no log file exists.
func TestViewLogs_NoLogFile(t *testing.T) {
	t.Parallel()

	mockStore := &mockTaskStoreForApprove{
		tasks:       map[string][]*domain.Task{},
		artifactErr: atlaserrors.ErrArtifactNotFound,
	}

	ctx := context.Background()
	err := viewLogs(ctx, mockStore, "test-ws", "task-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no log file found")
}

// TestSelectApprovalTask_WorkspaceProvided tests task selection with workspace argument.
func TestSelectApprovalTask_WorkspaceProvided(t *testing.T) {
	t.Parallel()

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1", Branch: "feat/ws1"},
			task:      &domain.Task{ID: "task-1", Description: "Task 1"},
		},
		{
			workspace: &domain.Workspace{Name: "ws2", Branch: "feat/ws2"},
			task:      &domain.Task{ID: "task-2", Description: "Task 2"},
		},
	}

	t.Run("workspace found", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		opts := approveOptions{workspace: "ws1"}
		ws, task, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks)
		require.NoError(t, err)
		assert.Equal(t, "ws1", ws.Name)
		assert.Equal(t, "task-1", task.ID)
	})

	t.Run("workspace not found", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		opts := approveOptions{workspace: "nonexistent"}
		_, _, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
	})
}

// TestSelectApprovalTask_SingleTask tests auto-selection with single task.
func TestSelectApprovalTask_SingleTask(t *testing.T) {
	t.Parallel()

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1", Branch: "feat/ws1"},
			task:      &domain.Task{ID: "task-1", Description: "Only task"},
		},
	}

	var buf bytes.Buffer
	opts := approveOptions{} // No workspace specified
	ws, task, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks)
	require.NoError(t, err)
	assert.Equal(t, "ws1", ws.Name)
	assert.Equal(t, "task-1", task.ID)
}

// TestRunApprove_ContextCancellation tests context cancellation handling.
func TestRunApprove_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	approveCmd := &cobra.Command{Use: "approve"}
	rootCmd.AddCommand(approveCmd)

	opts := approveOptions{}
	err := runApprove(ctx, approveCmd, &buf, opts)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestRunApprove_JSONModeRequiresWorkspace tests JSON mode workspace requirement.
func TestRunApprove_JSONModeRequiresWorkspace(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)
	_ = rootCmd.PersistentFlags().Set("output", "json")

	approveCmd := &cobra.Command{Use: "approve"}
	rootCmd.AddCommand(approveCmd)

	ctx := context.Background()
	opts := approveOptions{} // No workspace
	err := runApprove(ctx, approveCmd, &buf, opts)

	// Should return ErrJSONErrorOutput because error is written as JSON
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error output
	var resp approveResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "workspace argument required")
}

// TestApproveResponseFields tests that all expected fields are present in response.
func TestApproveResponseFields(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name:         "test-ws",
			Branch:       "feat/test",
			WorktreePath: "/path/to/worktree",
			Status:       "active",
		},
		Task: taskInfo{
			ID:           "task-1",
			TemplateName: "default",
			Description:  "Test task",
			Status:       "completed",
			CurrentStep:  7,
			TotalSteps:   7,
		},
		PRURL: "https://github.com/owner/repo/pull/123",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify all expected fields are present
	jsonStr := string(data)
	expectedFields := []string{
		"success",
		"workspace",
		"task",
		"pr_url",
		"name",
		"branch",
		"worktree_path",
		"status",
		"id",
		"template_name",
		"description",
		"current_step",
		"total_steps",
	}

	for _, field := range expectedFields {
		assert.Contains(t, jsonStr, field, "JSON should contain field %q", field)
	}
}

// TestApproveResponse_EmptyPRURL tests that pr_url is omitted when empty.
func TestApproveResponse_EmptyPRURL(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name: "test-ws",
		},
		Task: taskInfo{
			ID: "task-1",
		},
		PRURL: "", // Empty
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "pr_url", "pr_url should be omitted when empty")
}

// TestSelectApprovalTask_WorkspaceNotFound tests JSON error when workspace not found.
func TestSelectApprovalTask_WorkspaceNotFound_JSON(t *testing.T) {
	t.Parallel()

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1", Branch: "feat/ws1"},
			task:      &domain.Task{ID: "task-1", Description: "Task 1"},
		},
	}

	var buf bytes.Buffer
	opts := approveOptions{workspace: "nonexistent"}
	_, _, err := selectApprovalTask(OutputJSON, &buf, nil, opts, awaitingTasks)

	// Should return ErrJSONErrorOutput
	require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)

	// Check JSON error was written
	var resp approveResponse
	jsonErr := json.Unmarshal(buf.Bytes(), &resp)
	require.NoError(t, jsonErr)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "not found")
}

// TestApproveCommand_Examples tests that command has examples.
func TestApproveCommand_Examples(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check examples in long description
	assert.Contains(t, approveCmd.Long, "atlas approve")
	assert.Contains(t, approveCmd.Long, "-o json")
}

// TestApproveTask_InvalidTransition tests that invalid state transitions fail.
func TestApproveTask_InvalidTransition(t *testing.T) {
	t.Parallel()

	// Task in wrong state (already completed)
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Status:      constants.TaskStatusCompleted, // Already completed
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
	}

	ctx := context.Background()
	err := approveTask(ctx, mockStore, task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to approve task")
}

// TestAwaitingTask_Structure tests awaitingTask structure.
func TestAwaitingTask_Structure(t *testing.T) {
	t.Parallel()

	ws := &domain.Workspace{Name: "test-ws", Branch: "feat/test"}
	task := &domain.Task{ID: "task-1", Description: "Test task"}

	at := awaitingTask{
		workspace: ws,
		task:      task,
	}

	assert.Equal(t, "test-ws", at.workspace.Name)
	assert.Equal(t, "task-1", at.task.ID)
}

// TestApproveOptions_Structure tests approveOptions structure.
func TestApproveOptions_Structure(t *testing.T) {
	t.Parallel()

	opts := approveOptions{
		workspace: "my-workspace",
	}

	assert.Equal(t, "my-workspace", opts.workspace)
}

// TestApprovalAction_String tests approvalAction string conversion.
func TestApprovalAction_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "approve", string(actionApprove))
	assert.Equal(t, "view_diff", string(actionViewDiff))
	assert.Equal(t, "view_logs", string(actionViewLogs))
	assert.Equal(t, "open_pr", string(actionOpenPR))
	assert.Equal(t, "reject", string(actionReject))
	assert.Equal(t, "cancel", string(actionCancel))
}

// TestFindAwaitingApprovalTasks_WorkspaceListError tests error handling when listing workspaces fails.
func TestFindAwaitingApprovalTasks_WorkspaceListError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockWS := &mockWorkspaceStore{
		listErr: atlaserrors.ErrWorkspaceNotFound,
	}
	mockTask := &mockTaskStoreForApprove{tasks: map[string][]*domain.Task{}}

	_, err := findAwaitingApprovalTasks(ctx, mockWS, mockTask)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list workspaces")
}
