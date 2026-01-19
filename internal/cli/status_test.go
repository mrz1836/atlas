// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
)

// mockWorkspaceManager implements workspace.Manager interface for testing.
type mockWorkspaceManager struct {
	workspaces []*domain.Workspace
	listErr    error
}

func (m *mockWorkspaceManager) List(_ context.Context) ([]*domain.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.workspaces, nil
}

// mockTaskStore implements task.Store interface for testing.
type mockTaskStore struct {
	tasks   map[string][]*domain.Task // workspaceName -> tasks
	getErr  error
	listErr error
}

func (m *mockTaskStore) List(_ context.Context, workspaceName string) ([]*domain.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		return tasks, nil
	}
	return []*domain.Task{}, nil
}

// testStatusOpts creates StatusRenderOptions for tests.
func testStatusOpts(output string, quiet, progress bool) StatusRenderOptions {
	return StatusRenderOptions{
		Output:       output,
		Quiet:        quiet,
		ShowProgress: progress,
	}
}

// testStatusDeps creates StatusDeps for tests.
func testStatusDeps(mgr WorkspaceLister, store TaskLister) StatusDeps {
	return StatusDeps{
		WorkspaceMgr: mgr,
		TaskStore:    store,
	}
}

func (m *mockTaskStore) Get(_ context.Context, workspaceName, taskID string) (*domain.Task, error) {
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
	// Return explicit not found error instead of nil, nil
	return nil, errors.ErrTaskNotFound
}

// TestStatusCommand_Basic tests basic status command execution.
func TestStatusCommand_Basic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workspaces []*domain.Workspace
		tasks      map[string][]*domain.Task
		wantOutput []string
	}{
		{
			name:       "empty workspaces shows helpful message",
			workspaces: []*domain.Workspace{},
			tasks:      map[string][]*domain.Task{},
			wantOutput: []string{"No workspaces", "atlas start"},
		},
		{
			name: "single workspace with task",
			workspaces: []*domain.Workspace{
				{
					Name:   "auth",
					Branch: "feat/auth",
					Status: constants.WorkspaceStatusActive,
					Tasks: []domain.TaskRef{
						{ID: "task-1", Status: constants.TaskStatusRunning},
					},
				},
			},
			tasks: map[string][]*domain.Task{
				"auth": {
					{
						ID:          "task-1",
						WorkspaceID: "auth",
						Status:      constants.TaskStatusRunning,
						CurrentStep: 2,
						Steps:       make([]domain.Step, 7),
					},
				},
			},
			wantOutput: []string{"auth", "feat/auth", "running"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			ctx := context.Background()

			// Create mock dependencies
			mockMgr := &mockWorkspaceManager{workspaces: tt.workspaces}
			mockStore := &mockTaskStore{tasks: tt.tasks}

			err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
			require.NoError(t, err)

			output := buf.String()
			for _, want := range tt.wantOutput {
				assert.Contains(t, output, want, "output should contain %q", want)
			}
		})
	}
}

// TestStatusCommand_StatusPrioritySorting tests that workspaces are sorted by status priority.
func TestStatusCommand_StatusPrioritySorting(t *testing.T) {
	t.Parallel()

	// Create workspaces with different statuses
	workspaces := []*domain.Workspace{
		{
			Name: "completed-ws", Branch: "feat/completed", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-1", Status: constants.TaskStatusCompleted}},
		},
		{
			Name: "attention-ws", Branch: "feat/attention", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-2", Status: constants.TaskStatusAwaitingApproval}},
		},
		{
			Name: "running-ws", Branch: "feat/running", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-3", Status: constants.TaskStatusRunning}},
		},
	}

	tasks := map[string][]*domain.Task{
		"completed-ws": {{ID: "task-1", Status: constants.TaskStatusCompleted, Steps: make([]domain.Step, 5)}},
		"attention-ws": {{ID: "task-2", Status: constants.TaskStatusAwaitingApproval, Steps: make([]domain.Step, 7)}},
		"running-ws":   {{ID: "task-3", Status: constants.TaskStatusRunning, Steps: make([]domain.Step, 6)}},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(output, "\n")

	// Find the data rows (skip header and footer)
	// Each row is a separate line containing the workspace name followed by space
	var dataLines []string
	for _, line := range lines {
		// Match lines that start with a workspace name (data rows, not footer)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "attention-ws") ||
			strings.HasPrefix(trimmed, "running-ws") ||
			strings.HasPrefix(trimmed, "completed-ws") {
			dataLines = append(dataLines, line)
		}
	}

	require.Len(t, dataLines, 3, "should have 3 data rows")

	// Priority order: attention first, then running, then completed
	assert.True(t, strings.HasPrefix(strings.TrimSpace(dataLines[0]), "attention-ws"),
		"attention status should be first, got: %s", dataLines[0])
	assert.True(t, strings.HasPrefix(strings.TrimSpace(dataLines[1]), "running-ws"),
		"running status should be second, got: %s", dataLines[1])
	assert.True(t, strings.HasPrefix(strings.TrimSpace(dataLines[2]), "completed-ws"),
		"completed status should be last, got: %s", dataLines[2])
}

// TestStatusCommand_JSONOutput tests JSON output format.
func TestStatusCommand_JSONOutput(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{
			Name:   "payment",
			Branch: "fix/payment",
			Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-1", Status: constants.TaskStatusAwaitingApproval},
			},
		},
	}

	tasks := map[string][]*domain.Task{
		"payment": {
			{
				ID:          "task-1",
				WorkspaceID: "payment",
				Status:      constants.TaskStatusAwaitingApproval,
				CurrentStep: 5,
				Steps:       make([]domain.Step, 7),
			},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("json", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	// Parse JSON output - now uses hierarchical format with nested tasks
	var result hierarchicalJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "output should be valid JSON")
	require.Len(t, result.Workspaces, 1, "should have one workspace")

	// Check hierarchical JSON structure
	assert.Equal(t, "payment", result.Workspaces[0].Name)
	assert.Equal(t, "fix/payment", result.Workspaces[0].Branch)
	assert.Contains(t, result.Workspaces[0].Status, "awaiting_approval")
	assert.Equal(t, 1, result.Workspaces[0].TotalTasks)
	require.Len(t, result.Workspaces[0].Tasks, 1, "should have one task")
	assert.Equal(t, "6/7", result.Workspaces[0].Tasks[0].Step)

	// Check attention_items
	require.Len(t, result.AttentionItems, 1, "should have one attention item")
	assert.Equal(t, "payment", result.AttentionItems[0].Workspace)
	assert.Equal(t, "task-1", result.AttentionItems[0].TaskID)
	assert.Equal(t, "atlas approve payment", result.AttentionItems[0].Action)
}

// TestStatusCommand_EmptyJSON tests empty state returns valid JSON with empty workspaces array.
func TestStatusCommand_EmptyJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: []*domain.Workspace{}}
	mockStore := &mockTaskStore{tasks: map[string][]*domain.Task{}}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("json", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	// JSON output uses hierarchical format
	var result hierarchicalJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "empty state should return valid JSON")
	assert.Empty(t, result.Workspaces, "workspaces array should be empty")
	assert.Nil(t, result.AttentionItems, "attention_items should be nil/omitted")
}

// TestStatusCommand_QuietMode tests quiet mode output.
func TestStatusCommand_QuietMode(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{
			Name:   "auth",
			Branch: "feat/auth",
			Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-1", Status: constants.TaskStatusRunning},
			},
		},
	}

	tasks := map[string][]*domain.Task{
		"auth": {
			{
				ID:          "task-1",
				WorkspaceID: "auth",
				Status:      constants.TaskStatusRunning,
				CurrentStep: 2,
				Steps:       make([]domain.Step, 7),
			},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", true, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// Quiet mode should NOT have header or footer
	assert.NotContains(t, output, "ATLAS", "quiet mode should not show ATLAS header")
	// The word "workspace" appears in the footer summary (e.g., "1 workspace, 1 task")
	// The hierarchical footer is only shown when not in quiet mode
	assert.NotContains(t, output, "1 workspace,", "quiet mode should not show footer summary")
	assert.NotContains(t, output, "Run:", "quiet mode should not show actionable command")

	// But should have the table data (hierarchical format shows workspace and task rows)
	assert.Contains(t, output, "auth", "quiet mode should still show workspace name")
	assert.Contains(t, output, "task-1", "quiet mode should show task ID")
}

// TestStatusCommand_HeaderAndFooter tests header and footer display.
func TestStatusCommand_HeaderAndFooter(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{
			Name:   "auth",
			Branch: "feat/auth",
			Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-1", Status: constants.TaskStatusRunning},
			},
		},
		{
			Name:   "payment",
			Branch: "fix/payment",
			Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-2", Status: constants.TaskStatusAwaitingApproval},
			},
		},
	}

	tasks := map[string][]*domain.Task{
		"auth": {{ID: "task-1", Status: constants.TaskStatusRunning, Steps: make([]domain.Step, 7)}},
		"payment": {{
			ID: "task-2", Status: constants.TaskStatusAwaitingApproval,
			CurrentStep: 5, Steps: make([]domain.Step, 7),
		}},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// Check header
	assert.Contains(t, output, "ATLAS", "should show ATLAS header")

	// Check footer summary
	assert.Contains(t, output, "2 workspaces", "should show workspace count")
	assert.Contains(t, output, "1 needs attention", "should show attention count")

	// Check actionable command
	assert.Contains(t, output, "atlas approve payment", "should suggest approve command for attention workspace")
}

// TestStatusCommand_Performance tests that status command completes quickly.
func TestStatusCommand_Performance(t *testing.T) {
	t.Parallel()

	// Create 10+ workspaces to test performance
	workspaces := make([]*domain.Workspace, 15)
	tasks := make(map[string][]*domain.Task)

	for i := 0; i < 15; i++ {
		name := "workspace-" + string(rune('a'+i))
		workspaces[i] = &domain.Workspace{
			Name:   name,
			Branch: "feat/" + name,
			Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{
				{ID: "task-" + name, Status: constants.TaskStatusRunning},
			},
		}
		tasks[name] = []*domain.Task{
			{
				ID:          "task-" + name,
				WorkspaceID: name,
				Status:      constants.TaskStatusRunning,
				CurrentStep: 3,
				Steps:       make([]domain.Step, 7),
			},
		}
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	start := time.Now()
	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, duration, time.Second, "status command should complete in < 1 second (NFR1)")
}

// TestStatusCommand_ContextCancellation tests context cancellation handling.
func TestStatusCommand_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	mockMgr := &mockWorkspaceManager{workspaces: []*domain.Workspace{}}
	mockStore := &mockTaskStore{tasks: map[string][]*domain.Task{}}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	assert.ErrorIs(t, err, context.Canceled)
}

// TestAddStatusCommand tests that status command is properly added to root.
func TestAddStatusCommand(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddStatusCommand(root)

	// Find the status command
	statusCmd, _, err := root.Find([]string{"status"})
	require.NoError(t, err)
	require.NotNil(t, statusCmd)
	assert.Equal(t, "status", statusCmd.Name())
}

// TestRunStatus_EmptyWorkspaces tests runStatus with production dependencies (empty state).
func TestRunStatus_EmptyWorkspaces(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command with proper flags
	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	statusCmd := &cobra.Command{Use: "status"}
	rootCmd.AddCommand(statusCmd)

	// Execute with buffer - tests the production code path (no watch mode)
	err := runStatus(context.Background(), statusCmd, &buf, statusOptions{WatchMode: false, WatchInterval: DefaultWatchInterval, ShowProgress: false})
	require.NoError(t, err)

	// Verify empty message
	assert.Contains(t, buf.String(), "No workspaces")
}

// TestRunStatus_JSONOutput tests runStatus with JSON output format.
func TestRunStatus_JSONOutput(t *testing.T) {
	// Set up a temporary atlas directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	// Create a mock command with output flag set to json
	rootCmd := &cobra.Command{Use: "atlas"}
	flags := &GlobalFlags{Output: OutputJSON}
	AddGlobalFlags(rootCmd, flags)

	statusCmd := &cobra.Command{Use: "status"}
	rootCmd.AddCommand(statusCmd)

	// Set the output flag value on the persistent flags
	_ = rootCmd.PersistentFlags().Set("output", "json")

	// Execute with buffer (no watch mode)
	err := runStatus(context.Background(), statusCmd, &buf, statusOptions{WatchMode: false, WatchInterval: DefaultWatchInterval, ShowProgress: false})
	require.NoError(t, err)

	// Story 7.9: Should output empty structured JSON object
	var result statusJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "output should be valid JSON")
	assert.Empty(t, result.Workspaces)
}

// TestRunStatus_ContextCancellation tests runStatus respects context cancellation.
func TestRunStatus_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	statusCmd := &cobra.Command{Use: "status"}
	rootCmd.AddCommand(statusCmd)

	// Execute with canceled context (no watch mode)
	err := runStatus(ctx, statusCmd, &buf, statusOptions{WatchMode: false, WatchInterval: DefaultWatchInterval, ShowProgress: false})

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestStatusCommand_MultipleAttentionStates tests handling of multiple attention states.
func TestStatusCommand_MultipleAttentionStates(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{
			Name: "ws-validation-failed", Branch: "feat/a", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-1", Status: constants.TaskStatusValidationFailed}},
		},
		{
			Name: "ws-ci-failed", Branch: "feat/b", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-2", Status: constants.TaskStatusCIFailed}},
		},
		{
			Name: "ws-awaiting", Branch: "feat/c", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-3", Status: constants.TaskStatusAwaitingApproval}},
		},
	}

	tasks := map[string][]*domain.Task{
		"ws-validation-failed": {{ID: "task-1", Status: constants.TaskStatusValidationFailed, Steps: make([]domain.Step, 5)}},
		"ws-ci-failed":         {{ID: "task-2", Status: constants.TaskStatusCIFailed, Steps: make([]domain.Step, 5)}},
		"ws-awaiting":          {{ID: "task-3", Status: constants.TaskStatusAwaitingApproval, Steps: make([]domain.Step, 5)}},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// All three should be marked as needing attention (plural grammar)
	assert.Contains(t, output, "3 need attention", "footer should show 3 attention with plural grammar")
}

// TestStatusCommand_WorkspacesWithNoTasks tests workspaces without any tasks.
func TestStatusCommand_WorkspacesWithNoTasks(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{Name: "empty-ws", Branch: "feat/empty", Status: constants.WorkspaceStatusActive, Tasks: nil},
	}

	tasks := map[string][]*domain.Task{}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// Should show workspace with pending status
	assert.Contains(t, output, "empty-ws")
	assert.Contains(t, output, "pending")
}

// TestStatusCommand_EmptyTaskRefsWithTasksInStore tests the fix for the bug where
// ws.Tasks is empty but tasks exist in the store. This was the root cause of
// status showing 0/0 and "pending" instead of actual values.
func TestStatusCommand_EmptyTaskRefsWithTasksInStore(t *testing.T) {
	t.Parallel()

	// Workspace with EMPTY Tasks slice (simulates real bug scenario)
	workspaces := []*domain.Workspace{
		{
			Name:   "task-workspace",
			Branch: "task/task-workspace",
			Status: constants.WorkspaceStatusActive,
			Tasks:  []domain.TaskRef{}, // Empty! This is the bug scenario
		},
	}

	// But the task store HAS tasks for this workspace
	tasks := map[string][]*domain.Task{
		"task-workspace": {
			{
				ID:          "task-20260102-152211",
				WorkspaceID: "task-workspace",
				Status:      constants.TaskStatusAwaitingApproval,
				CurrentStep: 7, // 0-indexed, should display as 8
				Steps:       make([]domain.Step, 8),
			},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// Should show correct status from store, NOT "pending"
	assert.Contains(t, output, "awaiting_approval", "should show actual status from task store")
	assert.NotContains(t, output, "0/0", "should NOT show 0/0 step count")
	assert.Contains(t, output, "8/8", "should show correct step count (8/8)")
}

// TestStatusCommand_EmptyTaskRefsWithTasksInStore_JSON tests JSON output for the same bug scenario.
func TestStatusCommand_EmptyTaskRefsWithTasksInStore_JSON(t *testing.T) {
	t.Parallel()

	workspaces := []*domain.Workspace{
		{
			Name:   "task-workspace",
			Branch: "task/task-workspace",
			Status: constants.WorkspaceStatusActive,
			Tasks:  []domain.TaskRef{}, // Empty!
		},
	}

	tasks := map[string][]*domain.Task{
		"task-workspace": {
			{
				ID:          "task-20260102-152211",
				WorkspaceID: "task-workspace",
				Status:      constants.TaskStatusAwaitingApproval,
				CurrentStep: 7,
				Steps:       make([]domain.Step, 8),
			},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	err := runStatusWithDeps(ctx, &buf, testStatusOpts("json", false, false), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	var result hierarchicalJSONOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	require.Len(t, result.Workspaces, 1)

	// Verify correct values in hierarchical JSON format
	assert.Contains(t, result.Workspaces[0].Status, "awaiting_approval")
	require.Len(t, result.Workspaces[0].Tasks, 1, "should have one task")
	assert.Equal(t, "8/8", result.Workspaces[0].Tasks[0].Step, "JSON should show correct step count")
}

// TestStatusCommand_AllStatuses tests all possible task statuses are handled.
func TestStatusCommand_AllStatuses(t *testing.T) {
	t.Parallel()

	statuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			workspaces := []*domain.Workspace{
				{
					Name: "test-ws", Branch: "feat/test", Status: constants.WorkspaceStatusActive,
					Tasks: []domain.TaskRef{{ID: "task-1", Status: status}},
				},
			}

			tasks := map[string][]*domain.Task{
				"test-ws": {{ID: "task-1", Status: status, Steps: make([]domain.Step, 5)}},
			}

			var buf bytes.Buffer
			ctx := context.Background()

			mockMgr := &mockWorkspaceManager{workspaces: workspaces}
			mockStore := &mockTaskStore{tasks: tasks}

			err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, false), testStatusDeps(mockMgr, mockStore))
			require.NoError(t, err, "status %s should not cause error", status)

			output := buf.String()
			assert.Contains(t, output, "test-ws", "should show workspace name")
			assert.Contains(t, output, string(status), "should show status")
		})
	}
}

// TestStatusPriority tests the statusPriority function.
func TestStatusPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   constants.TaskStatus
		expected int
	}{
		{constants.TaskStatusAwaitingApproval, 2},
		{constants.TaskStatusValidationFailed, 2},
		{constants.TaskStatusCIFailed, 2},
		{constants.TaskStatusGHFailed, 2},
		{constants.TaskStatusCITimeout, 2},
		{constants.TaskStatusRunning, 1},
		{constants.TaskStatusValidating, 1},
		{constants.TaskStatusPending, 0},
		{constants.TaskStatusCompleted, 0},
		{constants.TaskStatusRejected, 0},
		{constants.TaskStatusAbandoned, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, statusPriority(tt.status))
		})
	}
}

// TestBuildFooter tests the buildFooter function.
func TestBuildFooter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rows     []tui.StatusRow
		wantSubs []string
	}{
		{
			name:     "empty rows",
			rows:     []tui.StatusRow{},
			wantSubs: []string{"0 workspaces"},
		},
		{
			name: "no attention needed",
			rows: []tui.StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusRunning},
				{Workspace: "ws2", Status: constants.TaskStatusCompleted},
			},
			wantSubs: []string{"2 workspaces"},
		},
		{
			name: "with attention needed",
			rows: []tui.StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusAwaitingApproval},
			},
			wantSubs: []string{"1 workspace", "1 needs attention", "atlas approve", "ws1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			footer := buildFooter(tt.rows)
			for _, want := range tt.wantSubs {
				assert.Contains(t, footer, want)
			}
		})
	}
}

// TestToLowerCamelCase tests the toLowerCamelCase function.
func TestToLowerCamelCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"WORKSPACE", "workspace"},
		{"BRANCH", "branch"},
		{"STATUS", "status"},
		{"STEP", "step"},
		{"ACTION", "action"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, toLowerCamelCase(tt.input))
		})
	}
}

// TestSortByStatusPriority tests the sorting function.
func TestSortByStatusPriority(t *testing.T) {
	t.Parallel()

	rows := []tui.StatusRow{
		{Workspace: "completed", Status: constants.TaskStatusCompleted},
		{Workspace: "attention", Status: constants.TaskStatusAwaitingApproval},
		{Workspace: "running", Status: constants.TaskStatusRunning},
		{Workspace: "pending", Status: constants.TaskStatusPending},
	}

	sortByStatusPriority(rows)

	assert.Equal(t, "attention", rows[0].Workspace, "attention should be first")
	assert.Equal(t, "running", rows[1].Workspace, "running should be second")
	// Completed and pending have same priority, order is stable
}

// TestAddStatusCommand_WatchFlags tests that watch mode flags are registered.
func TestAddStatusCommand_WatchFlags(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{Use: "atlas"}
	AddStatusCommand(root)

	// Find the status command
	statusCmd, _, err := root.Find([]string{"status"})
	require.NoError(t, err)
	require.NotNil(t, statusCmd)

	// Check --watch flag exists
	watchFlag := statusCmd.Flags().Lookup("watch")
	require.NotNil(t, watchFlag, "--watch flag should exist")
	assert.Equal(t, "w", watchFlag.Shorthand, "-w shorthand should be registered")

	// Check --interval flag exists
	intervalFlag := statusCmd.Flags().Lookup("interval")
	require.NotNil(t, intervalFlag, "--interval flag should exist")
	assert.Equal(t, "2s", intervalFlag.DefValue, "default interval should be 2s")

	// Check --progress flag exists (Story 7.8)
	progressFlag := statusCmd.Flags().Lookup("progress")
	require.NotNil(t, progressFlag, "--progress flag should exist")
	assert.Equal(t, "p", progressFlag.Shorthand, "-p shorthand should be registered")
}

// TestRunStatus_WatchModeMinInterval tests minimum interval validation.
func TestRunStatus_WatchModeMinInterval(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})

	statusCmd := &cobra.Command{Use: "status"}
	rootCmd.AddCommand(statusCmd)

	// Try with interval below minimum (500ms)
	err := runStatus(context.Background(), statusCmd, &buf, statusOptions{WatchMode: true, WatchInterval: 100 * time.Millisecond, ShowProgress: false})

	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrWatchIntervalTooShort)
}

// TestRunStatus_WatchModeJSONError tests watch mode rejects JSON output.
func TestRunStatus_WatchModeJSONError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{Output: OutputJSON})

	statusCmd := &cobra.Command{Use: "status"}
	rootCmd.AddCommand(statusCmd)

	// Set the output flag value on the persistent flags
	_ = rootCmd.PersistentFlags().Set("output", "json")

	// Try watch mode with JSON output
	err := runStatus(context.Background(), statusCmd, &buf, statusOptions{WatchMode: true, WatchInterval: 2 * time.Second, ShowProgress: false})

	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrWatchModeJSONUnsupported)
}

// TestMinWatchInterval tests the constant values.
func TestMinWatchInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 500*time.Millisecond, MinWatchInterval)
	assert.Equal(t, 2*time.Second, DefaultWatchInterval)
}

// TestStatusCommand_ProgressFlag tests progress bar display (Story 7.8).
func TestStatusCommand_ProgressFlag(t *testing.T) {
	t.Parallel()

	// Create workspaces with running tasks
	workspaces := []*domain.Workspace{
		{
			Name: "auth", Branch: "feat/auth", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-1", Status: constants.TaskStatusRunning}},
		},
		{
			Name: "payment", Branch: "fix/payment", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-2", Status: constants.TaskStatusValidating}},
		},
		{
			Name: "completed-ws", Branch: "feat/done", Status: constants.WorkspaceStatusActive,
			Tasks: []domain.TaskRef{{ID: "task-3", Status: constants.TaskStatusCompleted}},
		},
	}

	tasks := map[string][]*domain.Task{
		"auth":         {{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 3, Steps: make([]domain.Step, 7)}},
		"payment":      {{ID: "task-2", Status: constants.TaskStatusValidating, CurrentStep: 5, Steps: make([]domain.Step, 7)}},
		"completed-ws": {{ID: "task-3", Status: constants.TaskStatusCompleted, CurrentStep: 7, Steps: make([]domain.Step, 7)}},
	}

	var buf bytes.Buffer
	ctx := context.Background()

	mockMgr := &mockWorkspaceManager{workspaces: workspaces}
	mockStore := &mockTaskStore{tasks: tasks}

	// Run with showProgress=true
	err := runStatusWithDeps(ctx, &buf, testStatusOpts("text", false, true), testStatusDeps(mockMgr, mockStore))
	require.NoError(t, err)

	output := buf.String()

	// Should include table data
	assert.Contains(t, output, "auth")
	assert.Contains(t, output, "payment")

	// Progress bars should be rendered for running/validating tasks
	// (Output will include progress bar characters like █ or ░)
	// The output should be longer than without progress
	assert.Greater(t, len(output), 100, "output with progress should have substantial content")
}

// TestBuildProgressRows tests the buildProgressRows function.
func TestBuildProgressRows(t *testing.T) {
	t.Parallel()

	rows := []tui.StatusRow{
		{Workspace: "running-ws", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		{Workspace: "validating-ws", Status: constants.TaskStatusValidating, CurrentStep: 5, TotalSteps: 7},
		{Workspace: "completed-ws", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7},
		{Workspace: "pending-ws", Status: constants.TaskStatusPending, CurrentStep: 0, TotalSteps: 5},
	}

	progressRows := buildProgressRows(rows)

	// Only running and validating should be included
	assert.Len(t, progressRows, 2, "only active tasks should be included")
	assert.Equal(t, "running-ws", progressRows[0].Name)
	assert.Equal(t, "validating-ws", progressRows[1].Name)

	// Check progress calculations
	assert.InDelta(t, 3.0/7.0, progressRows[0].Percent, 0.01)
	assert.InDelta(t, 5.0/7.0, progressRows[1].Percent, 0.01)
}
