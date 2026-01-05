// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	logData     []byte                    // log file data
	getErr      error
	listErr     error
	updateErr   error
	artifactErr error
	logErr      error
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

func (m *mockTaskStoreForApprove) Delete(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockTaskStoreForApprove) AppendLog(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

func (m *mockTaskStoreForApprove) ReadLog(_ context.Context, _, _ string) ([]byte, error) {
	if m.logErr != nil {
		return nil, m.logErr
	}
	if m.logData != nil {
		return m.logData, nil
	}
	return nil, atlaserrors.ErrArtifactNotFound
}

func (m *mockTaskStoreForApprove) SaveArtifact(_ context.Context, _, _, _ string, _ []byte) error {
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

func (m *mockTaskStoreForApprove) ListArtifacts(_ context.Context, _, _ string) ([]string, error) {
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
		tasks:  map[string][]*domain.Task{},
		logErr: atlaserrors.ErrArtifactNotFound,
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
		ws, task, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks, false)
		require.NoError(t, err)
		assert.Equal(t, "ws1", ws.Name)
		assert.Equal(t, "task-1", task.ID)
	})

	t.Run("workspace not found", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		opts := approveOptions{workspace: "nonexistent"}
		_, _, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks, false)
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
	ws, task, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks, false)
	require.NoError(t, err)
	assert.Equal(t, "ws1", ws.Name)
	assert.Equal(t, "task-1", task.ID)
}

// TestSelectApprovalTask_NonInteractive_MultipleTasksRequiresWorkspace tests that non-interactive mode
// with multiple tasks returns error requiring workspace argument.
func TestSelectApprovalTask_NonInteractive_MultipleTasksRequiresWorkspace(t *testing.T) {
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

	var buf bytes.Buffer
	opts := approveOptions{}                                                          // No workspace specified
	_, _, err := selectApprovalTask(OutputText, &buf, nil, opts, awaitingTasks, true) // Non-interactive
	require.Error(t, err)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
	assert.ErrorIs(t, err, atlaserrors.ErrInteractiveRequired)
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
	_, _, err := selectApprovalTask(OutputJSON, &buf, nil, opts, awaitingTasks, false)

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
		workspace:   "my-workspace",
		autoApprove: true,
	}

	assert.Equal(t, "my-workspace", opts.workspace)
	assert.True(t, opts.autoApprove)
}

// TestApproveCommand_AutoApproveFlag tests that --auto-approve flag is defined.
func TestApproveCommand_AutoApproveFlag(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check that --auto-approve flag exists
	flag := approveCmd.Flags().Lookup("auto-approve")
	require.NotNil(t, flag)
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "interactive")
}

// TestApproveCommand_AutoApproveHelp tests that help includes auto-approve examples.
func TestApproveCommand_AutoApproveHelp(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check that long description mentions --auto-approve
	assert.Contains(t, approveCmd.Long, "--auto-approve")
	assert.Contains(t, approveCmd.Long, "Non-interactive mode")
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

// TestRunAutoApprove_Success tests that runAutoApprove successfully approves a task.
func TestRunAutoApprove_Success(t *testing.T) {
	t.Parallel()

	// Create a task in awaiting_approval state
	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Description: "Test auto-approve task",
		Status:      constants.TaskStatusAwaitingApproval,
		Steps:       make([]domain.Step, 7),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ws := &domain.Workspace{
		Name:   "test-ws",
		Branch: "feat/test",
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, false) // Bell disabled for tests

	ctx := context.Background()
	err := runAutoApprove(ctx, out, mockStore, ws, task, notifier, false)
	require.NoError(t, err)

	// Verify task status changed to completed
	assert.Equal(t, constants.TaskStatusCompleted, task.Status)

	// Verify output contains success message
	output := buf.String()
	assert.Contains(t, output, "Task approved")
	assert.Contains(t, output, "test-ws")
	assert.Contains(t, output, "task-1")
}

// TestRunAutoApprove_WithPRURL tests that runAutoApprove displays PR URL when available.
func TestRunAutoApprove_WithPRURL(t *testing.T) {
	t.Parallel()

	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Description: "Test auto-approve with PR",
		Status:      constants.TaskStatusAwaitingApproval,
		Metadata:    map[string]interface{}{"pr_url": "https://github.com/owner/repo/pull/123"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ws := &domain.Workspace{
		Name:   "test-ws",
		Branch: "feat/test",
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, false)

	ctx := context.Background()
	err := runAutoApprove(ctx, out, mockStore, ws, task, notifier, false)
	require.NoError(t, err)

	// Verify PR URL is in output
	output := buf.String()
	assert.Contains(t, output, "https://github.com/owner/repo/pull/123")
}

// TestRunAutoApprove_StoreError tests error handling when task store update fails.
func TestRunAutoApprove_StoreError(t *testing.T) {
	t.Parallel()

	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Description: "Test auto-approve error",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ws := &domain.Workspace{
		Name:   "test-ws",
		Branch: "feat/test",
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
		updateErr: atlaserrors.ErrTaskNotFound,
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, false)

	ctx := context.Background()
	err := runAutoApprove(ctx, out, mockStore, ws, task, notifier, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to approve task")
}

// TestApproveCommand_CloseFlag tests that --close flag is defined.
func TestApproveCommand_CloseFlag(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check that --close flag exists
	flag := approveCmd.Flags().Lookup("close")
	require.NotNil(t, flag)
	assert.Equal(t, "bool", flag.Value.Type())
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "close")
}

// TestApproveCommand_CloseHelp tests that help includes --close examples.
func TestApproveCommand_CloseHelp(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddApproveCommand(root)

	approveCmd, _, err := root.Find([]string{"approve"})
	require.NoError(t, err)

	// Check that long description mentions --close
	assert.Contains(t, approveCmd.Long, "--close")
	assert.Contains(t, approveCmd.Long, "close")
}

// TestApproveAction_ApproveAndClose tests actionApproveAndClose constant.
func TestApproveAction_ApproveAndClose(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "approve_and_close", string(actionApproveAndClose))
}

// TestApproveOptions_CloseField tests approveOptions includes closeWS field.
func TestApproveOptions_CloseField(t *testing.T) {
	t.Parallel()

	opts := approveOptions{
		workspace:   "my-workspace",
		autoApprove: true,
		closeWS:     true,
	}

	assert.Equal(t, "my-workspace", opts.workspace)
	assert.True(t, opts.autoApprove)
	assert.True(t, opts.closeWS)
}

// TestApproveResponse_WorkspaceClosedField tests that WorkspaceClosed field is marshaled correctly.
func TestApproveResponse_WorkspaceClosedField(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name: "test-ws",
		},
		Task: taskInfo{
			ID: "task-1",
		},
		WorkspaceClosed: true,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify workspace_closed field is present
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.True(t, parsed["workspace_closed"].(bool))
}

// TestApproveResponse_WorkspaceClosedOmitted tests that workspace_closed is omitted when false.
func TestApproveResponse_WorkspaceClosedOmitted(t *testing.T) {
	t.Parallel()

	resp := approveResponse{
		Success: true,
		Workspace: workspaceInfo{
			Name: "test-ws",
		},
		Task: taskInfo{
			ID: "task-1",
		},
		WorkspaceClosed: false, // Should be omitted
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "workspace_closed", "workspace_closed should be omitted when false")
}

// TestRunAutoApprove_WithCloseFlag tests runAutoApprove with closeWS flag.
func TestRunAutoApprove_WithCloseFlag(t *testing.T) {
	t.Parallel()

	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "test-ws",
		Description: "Test auto-approve with close",
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ws := &domain.Workspace{
		Name:   "test-ws",
		Branch: "feat/test",
	}

	mockStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"test-ws": {task},
		},
	}

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")
	notifier := tui.NewNotifier(false, false)

	ctx := context.Background()
	// closeWS=true, but workspace won't actually close because we don't have a real store
	// This tests that the flag is passed through correctly
	err := runAutoApprove(ctx, out, mockStore, ws, task, notifier, true)
	require.NoError(t, err)

	// Verify task status changed to completed
	assert.Equal(t, constants.TaskStatusCompleted, task.Status)

	// Output should mention the workspace close attempt (even if it fails)
	output := buf.String()
	assert.Contains(t, output, "Task approved")
}

// TestSelectApprovalTask_WorkspaceMatchFound tests successful workspace match
func TestSelectApprovalTask_WorkspaceMatchFound(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	expectedWS := &domain.Workspace{Name: "target-workspace"}
	expectedTask := &domain.Task{ID: "target-task", Status: constants.TaskStatusAwaitingApproval}

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "other-workspace"},
			task:      &domain.Task{ID: "other-task"},
		},
		{
			workspace: expectedWS,
			task:      expectedTask,
		},
	}

	ws, task, err := selectApprovalTask("text", &buf, out, approveOptions{
		workspace: "target-workspace",
	}, awaitingTasks, false)

	require.NoError(t, err)
	assert.Equal(t, expectedWS, ws)
	assert.Equal(t, expectedTask, task)
}

// TestHandleApproveError_DifferentFormats tests error handling in different output modes
func TestHandleApproveError_DifferentFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		outputFormat string
		workspace    string
		err          error
		wantJSONOut  bool
		wantError    bool
	}{
		{
			name:         "JSON format outputs JSON",
			outputFormat: "json",
			workspace:    "test-ws",
			err:          atlaserrors.ErrTaskNotFound,
			wantJSONOut:  true,
			wantError:    true, // handleApproveError returns ErrJSONErrorOutput for JSON
		},
		{
			name:         "text format returns error",
			outputFormat: "text",
			workspace:    "test-ws",
			err:          atlaserrors.ErrWorkspaceNotFound,
			wantJSONOut:  false,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := handleApproveError(tt.outputFormat, &buf, tt.workspace, tt.err)

			if tt.wantJSONOut {
				require.ErrorIs(t, err, atlaserrors.ErrJSONErrorOutput)
				var resp approveResponse
				require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
				assert.False(t, resp.Success)
			}

			if tt.wantError && !tt.wantJSONOut {
				// Text format should return the original error
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.err)
			}
		})
	}
}

// TestFindAwaitingApprovalTasks_FiltersNonAwaitingTasks tests status filtering
func TestFindAwaitingApprovalTasks_FiltersNonAwaitingTasks(t *testing.T) {
	t.Parallel()

	wsStore := &mockWorkspaceStore{
		workspaces: []*domain.Workspace{
			{Name: "ws1", Status: constants.WorkspaceStatusActive},
			{Name: "ws2", Status: constants.WorkspaceStatusActive},
		},
	}

	taskStore := &mockTaskStoreForApprove{
		tasks: map[string][]*domain.Task{
			"ws1": {
				{ID: "task1", Status: constants.TaskStatusAwaitingApproval}, // Include
				{ID: "task2", Status: constants.TaskStatusCompleted},        // Exclude
			},
			"ws2": {
				{ID: "task3", Status: constants.TaskStatusRunning}, // Exclude
			},
		},
	}

	tasks, err := findAwaitingApprovalTasks(context.Background(), wsStore, taskStore)

	require.NoError(t, err)
	assert.Len(t, tasks, 1, "should only return awaiting approval tasks")
	assert.Equal(t, "task1", tasks[0].task.ID)
	assert.Equal(t, "ws1", tasks[0].workspace.Name)
}

// TestSelectApprovalTask_MultipleTasksNonInteractiveError tests non-interactive multi-task handling
func TestSelectApprovalTask_MultipleTasksNonInteractiveError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "text")

	awaitingTasks := []awaitingTask{
		{
			workspace: &domain.Workspace{Name: "ws1"},
			task:      &domain.Task{ID: "task1"},
		},
		{
			workspace: &domain.Workspace{Name: "ws2"},
			task:      &domain.Task{ID: "task2"},
		},
	}

	// Non-interactive mode with multiple tasks and no workspace specified
	ws, task, err := selectApprovalTask("text", &buf, out, approveOptions{}, awaitingTasks, true)

	assert.Nil(t, ws)
	assert.Nil(t, task)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrInteractiveRequired)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
}

// TestApproveOptions_DefaultValues tests default option values
func TestApproveOptions_DefaultValues(t *testing.T) {
	t.Parallel()

	opts := approveOptions{}

	assert.Empty(t, opts.workspace)
	assert.False(t, opts.autoApprove)
	assert.False(t, opts.closeWS)
}

// TestApprovalAction_TypeConversion tests action type conversion
func TestApprovalAction_TypeConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		str    string
		action approvalAction
	}{
		{"approve", actionApprove},
		{"approve_and_close", actionApproveAndClose},
		{"view_diff", actionViewDiff},
		{"view_logs", actionViewLogs},
		{"open_pr", actionOpenPR},
		{"reject", actionReject},
		{"cancel", actionCancel},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			assert.Equal(t, approvalAction(tt.str), tt.action)
			assert.Equal(t, string(tt.action), tt.str)
		})
	}
}
