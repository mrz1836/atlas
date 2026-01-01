// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// mockWorkspaceLister implements WorkspaceLister for testing.
type mockWorkspaceLister struct {
	workspaces []*domain.Workspace
	listErr    error
}

func (m *mockWorkspaceLister) List(_ context.Context) ([]*domain.Workspace, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.workspaces, nil
}

// mockTaskLister implements TaskLister for testing.
type mockTaskLister struct {
	tasks   map[string][]*domain.Task
	listErr error
}

func (m *mockTaskLister) List(_ context.Context, workspaceName string) ([]*domain.Task, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if tasks, ok := m.tasks[workspaceName]; ok {
		return tasks, nil
	}
	return []*domain.Task{}, nil
}

// TestNewWatchModel tests WatchModel initialization.
func TestNewWatchModel(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: true,
		Quiet:       false,
	}

	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	assert.NotNil(t, model)
	assert.NotNil(t, model.previousRows)
	assert.Equal(t, 2*time.Second, model.config.Interval)
	assert.True(t, model.config.BellEnabled)
	assert.False(t, model.config.Quiet)
	assert.False(t, model.quitting)
	assert.Equal(t, 80, model.width)  // Default width
	assert.Equal(t, 24, model.height) // Default height
}

// TestDefaultWatchConfig tests default config values.
func TestDefaultWatchConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultWatchConfig()

	assert.Equal(t, 2*time.Second, cfg.Interval)
	assert.True(t, cfg.BellEnabled)
	assert.False(t, cfg.Quiet)
}

// TestWatchModel_Init tests Init returns correct commands.
func TestWatchModel_Init(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	cmd := model.Init()

	// Init should return a batch of commands (refresh + tick)
	assert.NotNil(t, cmd)
}

// TestWatchModel_Update_KeyQuit tests 'q' key quits.
func TestWatchModel_Update_KeyQuit(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate 'q' key press
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	updatedModel, cmd := model.Update(msg)

	watchModel := updatedModel.(*WatchModel)
	assert.True(t, watchModel.quitting)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

// TestWatchModel_Update_KeyCtrlC tests Ctrl+C quits.
func TestWatchModel_Update_KeyCtrlC(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate Ctrl+C key press
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updatedModel, cmd := model.Update(msg)

	watchModel := updatedModel.(*WatchModel)
	assert.True(t, watchModel.quitting)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

// TestWatchModel_Update_WindowResize tests terminal resize handling.
func TestWatchModel_Update_WindowResize(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate window resize
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, cmd := model.Update(msg)

	watchModel := updatedModel.(*WatchModel)
	assert.Equal(t, 120, watchModel.width)
	assert.Equal(t, 40, watchModel.height)
	assert.Nil(t, cmd) // No command on resize
}

// TestWatchModel_Update_TickMsg tests tick message handling.
func TestWatchModel_Update_TickMsg(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate tick message
	msg := TickMsg(time.Now())
	_, cmd := model.Update(msg)

	// TickMsg should trigger a refresh command
	assert.NotNil(t, cmd)
}

// TestWatchModel_Update_RefreshMsg tests refresh data handling.
func TestWatchModel_Update_RefreshMsg(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	testRows := []StatusRow{
		{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
	}

	// Simulate refresh message
	msg := RefreshMsg{Rows: testRows, Err: nil}
	updatedModel, cmd := model.Update(msg)

	watchModel := updatedModel.(*WatchModel)
	assert.Len(t, watchModel.rows, 1)
	assert.Equal(t, "auth", watchModel.rows[0].Workspace)
	assert.False(t, watchModel.lastUpdate.IsZero())
	assert.NotNil(t, cmd) // Should return tick command
}

// TestWatchModel_Update_RefreshMsgError tests error handling in refresh.
func TestWatchModel_Update_RefreshMsgError(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate refresh with error
	msg := RefreshMsg{Rows: nil, Err: assert.AnError}
	updatedModel, cmd := model.Update(msg)

	watchModel := updatedModel.(*WatchModel)
	require.Error(t, watchModel.err)
	assert.NotNil(t, cmd) // Should still return tick command
}

// TestWatchModel_View_Empty tests view rendering with no workspaces.
func TestWatchModel_View_Empty(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	view := model.View()

	// Header uses ASCII art (▄▀█) in wide mode or "ATLAS" text in narrow mode
	assert.True(t, strings.Contains(view, "▄▀█") || strings.Contains(view, "ATLAS"),
		"expected header to contain ASCII art or ATLAS text")
	assert.Contains(t, view, "No workspaces")
	assert.Contains(t, view, "atlas start")
	assert.Contains(t, view, "Press 'q' to quit")
}

// TestWatchModel_View_Quitting tests view when quitting.
func TestWatchModel_View_Quitting(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.quitting = true

	view := model.View()

	assert.Empty(t, view)
}

// TestWatchModel_View_WithData tests view rendering with workspace data.
func TestWatchModel_View_WithData(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.rows = []StatusRow{
		{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		{Workspace: "payment", Branch: "fix/pay", Status: constants.TaskStatusAwaitingApproval, CurrentStep: 5, TotalSteps: 7},
	}
	model.lastUpdate = time.Now()

	view := model.View()

	// Header uses ASCII art (▄▀█) in wide mode or "ATLAS" text in narrow mode
	assert.True(t, strings.Contains(view, "▄▀█") || strings.Contains(view, "ATLAS"),
		"expected header to contain ASCII art or ATLAS text")
	assert.Contains(t, view, "auth")
	assert.Contains(t, view, "payment")
	assert.Contains(t, view, "Last updated:")
	assert.Contains(t, view, "Press 'q' to quit")
	assert.Contains(t, view, "2 workspaces")
}

// TestWatchModel_View_Quiet tests view in quiet mode.
func TestWatchModel_View_Quiet(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: false,
		Quiet:       true,
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.rows = []StatusRow{
		{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
	}
	model.lastUpdate = time.Now()

	view := model.View()

	// Quiet mode should NOT show header or footer
	assert.NotContains(t, view, "ATLAS")
	assert.NotContains(t, view, "workspaces")
	// But should still show quit hint and timestamp
	assert.Contains(t, view, "Press 'q' to quit")
	assert.Contains(t, view, "Last updated:")
}

// TestWatchModel_View_WithError tests view rendering with error.
func TestWatchModel_View_WithError(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.err = assert.AnError

	view := model.View()

	assert.Contains(t, view, "Error:")
}

// TestWatchModel_BellNotification_OnNewAttention tests bell on new attention state.
func TestWatchModel_BellNotification_OnNewAttention(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: true,
		Quiet:       false,
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// First refresh with running status (no attention)
	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusRunning},
	}
	cmd := model.checkForBell()
	assert.Nil(t, cmd, "should not bell for non-attention status")

	// Second refresh transitions to attention status
	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusAwaitingApproval},
	}
	cmd = model.checkForBell()
	assert.NotNil(t, cmd, "should bell on transition to attention status")
}

// TestWatchModel_BellNotification_NoRepeatBell tests no repeat bell for same status.
func TestWatchModel_BellNotification_NoRepeatBell(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: true,
		Quiet:       false,
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Initial attention state - should bell
	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusAwaitingApproval},
	}
	cmd := model.checkForBell()
	assert.NotNil(t, cmd, "first transition should bell")

	// Same attention state again - should NOT bell again
	cmd = model.checkForBell()
	assert.Nil(t, cmd, "repeat attention status should not bell again")
}

// TestWatchModel_BellNotification_Disabled tests bell disabled behavior.
func TestWatchModel_BellNotification_Disabled(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: false, // Disabled
		Quiet:       false,
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusAwaitingApproval},
	}
	cmd := model.checkForBell()

	assert.Nil(t, cmd, "bell disabled should not emit")
}

// TestWatchModel_BellNotification_QuietModeSuppresses tests quiet mode suppresses bell.
func TestWatchModel_BellNotification_QuietModeSuppresses(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: true, // Bell would normally be enabled
		Quiet:       true, // But quiet mode suppresses it
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusAwaitingApproval},
	}
	cmd := model.checkForBell()

	assert.Nil(t, cmd, "quiet mode should suppress bell even when bell is enabled")
}

// TestWatchModel_BellNotification_AllAttentionStatuses tests all attention statuses trigger bell.
func TestWatchModel_BellNotification_AllAttentionStatuses(t *testing.T) {
	t.Parallel()

	attentionStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range attentionStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
			mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

			cfg := WatchConfig{
				Interval:    2 * time.Second,
				BellEnabled: true,
				Quiet:       false,
			}
			model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

			model.rows = []StatusRow{
				{Workspace: "test-ws", Status: status},
			}
			cmd := model.checkForBell()

			assert.NotNil(t, cmd, "attention status %s should trigger bell", status)
		})
	}
}

// TestWatchModel_StatusRowBuilding tests building status rows from workspaces.
func TestWatchModel_StatusRowBuilding(t *testing.T) {
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
				Status:      constants.TaskStatusRunning,
				CurrentStep: 2,
				Steps:       make([]domain.Step, 7),
			},
		},
	}

	mockWs := &mockWorkspaceLister{workspaces: workspaces}
	mockTask := &mockTaskLister{tasks: tasks}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	rows := model.buildStatusRows(context.Background(), workspaces)
	require.Len(t, rows, 1)

	assert.Equal(t, "auth", rows[0].Workspace)
	assert.Equal(t, "feat/auth", rows[0].Branch)
	assert.Equal(t, constants.TaskStatusRunning, rows[0].Status)
	assert.Equal(t, 3, rows[0].CurrentStep) // 1-indexed
	assert.Equal(t, 7, rows[0].TotalSteps)
}

// TestWatchModel_StatusRowBuilding_TaskListerError tests graceful handling of TaskLister errors.
func TestWatchModel_StatusRowBuilding_TaskListerError(t *testing.T) {
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

	mockWs := &mockWorkspaceLister{workspaces: workspaces}
	mockTask := &mockTaskLister{
		tasks:   map[string][]*domain.Task{},
		listErr: assert.AnError, // TaskLister returns an error
	}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// buildStatusRows should gracefully handle TaskLister errors
	rows := model.buildStatusRows(context.Background(), workspaces)
	require.Len(t, rows, 1)

	// When TaskLister fails, we should still get a row with default status from workspace
	assert.Equal(t, "auth", rows[0].Workspace)
	assert.Equal(t, "feat/auth", rows[0].Branch)
	// Status falls back to the TaskRef status since full task load failed
	assert.Equal(t, constants.TaskStatusRunning, rows[0].Status)
	// Step info should be zero since we couldn't load the full task
	assert.Equal(t, 0, rows[0].CurrentStep)
	assert.Equal(t, 0, rows[0].TotalSteps)
}

// TestWatchModel_StatusPrioritySorting tests sorting by status priority.
func TestWatchModel_StatusPrioritySorting(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	rows := []StatusRow{
		{Workspace: "completed", Status: constants.TaskStatusCompleted},
		{Workspace: "attention", Status: constants.TaskStatusAwaitingApproval},
		{Workspace: "running", Status: constants.TaskStatusRunning},
		{Workspace: "pending", Status: constants.TaskStatusPending},
	}

	model.sortByStatusPriority(rows)

	assert.Equal(t, "attention", rows[0].Workspace, "attention should be first")
	assert.Equal(t, "running", rows[1].Workspace, "running should be second")
	// Completed and pending have same priority (0)
}

// TestWatchModel_Accessors tests accessor methods.
func TestWatchModel_Accessors(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusRunning},
	}
	model.lastUpdate = time.Now()
	model.err = assert.AnError

	assert.Len(t, model.Rows(), 1)
	assert.False(t, model.LastUpdate().IsZero())
	assert.False(t, model.IsQuitting())
	assert.Error(t, model.Error())
}

// TestWatchModel_Footer tests footer building.
func TestWatchModel_Footer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rows     []StatusRow
		wantSubs []string
	}{
		{
			name:     "empty rows",
			rows:     []StatusRow{},
			wantSubs: []string{"0 workspaces"},
		},
		{
			name: "single workspace no attention",
			rows: []StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusRunning},
			},
			wantSubs: []string{"1 workspace"}, // Singular
		},
		{
			name: "multiple workspaces no attention",
			rows: []StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusRunning},
				{Workspace: "ws2", Status: constants.TaskStatusCompleted},
			},
			wantSubs: []string{"2 workspaces"}, // Plural
		},
		{
			name: "with attention needed singular",
			rows: []StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusAwaitingApproval},
			},
			wantSubs: []string{"1 workspace", "1 needs attention", "atlas approve", "ws1"},
		},
		{
			name: "with attention needed plural",
			rows: []StatusRow{
				{Workspace: "ws1", Status: constants.TaskStatusAwaitingApproval},
				{Workspace: "ws2", Status: constants.TaskStatusCIFailed},
			},
			wantSubs: []string{"2 workspaces", "2 need attention"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
			mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

			cfg := DefaultWatchConfig()
			model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
			model.rows = tt.rows

			footer := model.buildFooter()
			for _, want := range tt.wantSubs {
				assert.Contains(t, footer, want)
			}
		})
	}
}

// TestWatchModel_CleanupRemovedWorkspaces tests that removed workspaces are cleaned from tracking.
func TestWatchModel_CleanupRemovedWorkspaces(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := WatchConfig{
		Interval:    2 * time.Second,
		BellEnabled: true,
		Quiet:       false,
	}
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// First, track a workspace
	model.rows = []StatusRow{
		{Workspace: "old-ws", Status: constants.TaskStatusRunning},
		{Workspace: "keep-ws", Status: constants.TaskStatusRunning},
	}
	model.checkForBell() // This populates previousRows

	// Verify both are tracked
	_, oldExists := model.previousRows["old-ws"]
	_, keepExists := model.previousRows["keep-ws"]
	assert.True(t, oldExists)
	assert.True(t, keepExists)

	// Now workspace is removed
	model.rows = []StatusRow{
		{Workspace: "keep-ws", Status: constants.TaskStatusRunning},
	}
	model.checkForBell() // Should clean up old-ws

	// Verify old-ws is removed from tracking
	_, oldExists = model.previousRows["old-ws"]
	_, keepExists = model.previousRows["keep-ws"]
	assert.False(t, oldExists, "removed workspace should be cleaned from tracking")
	assert.True(t, keepExists, "remaining workspace should still be tracked")
}

// TestWatchModel_RefreshData tests the refresh data command.
func TestWatchModel_RefreshData(t *testing.T) {
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
				Status:      constants.TaskStatusRunning,
				CurrentStep: 2,
				Steps:       make([]domain.Step, 7),
			},
		},
	}

	mockWs := &mockWorkspaceLister{workspaces: workspaces}
	mockTask := &mockTaskLister{tasks: tasks}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	cmd := model.refreshData()
	require.NotNil(t, cmd)

	// Execute the command to get the message
	msg := cmd()
	require.NotNil(t, msg)

	refreshMsg, ok := msg.(RefreshMsg)
	require.True(t, ok, "should return RefreshMsg")
	require.NoError(t, refreshMsg.Err)
	require.Len(t, refreshMsg.Rows, 1)
	assert.Equal(t, "auth", refreshMsg.Rows[0].Workspace)
}

// TestWatchModel_RefreshDataError tests refresh data with error.
func TestWatchModel_RefreshDataError(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: nil, listErr: assert.AnError}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	cmd := model.refreshData()
	require.NotNil(t, cmd)

	// Execute the command to get the message
	msg := cmd()
	require.NotNil(t, msg)

	refreshMsg, ok := msg.(RefreshMsg)
	require.True(t, ok, "should return RefreshMsg")
	require.Error(t, refreshMsg.Err)
	assert.Contains(t, refreshMsg.Err.Error(), "failed to list workspaces")
}

// TestEmitBell tests the emitBell command.
func TestEmitBell(t *testing.T) {
	t.Parallel()

	cmd := emitBell()
	require.NotNil(t, cmd)

	// Execute the command - it should return BellMsg
	msg := cmd()
	_, ok := msg.(BellMsg)
	assert.True(t, ok, "emitBell should return BellMsg")
}

// TestWatchModel_ViewContainsTimestamp tests that view shows last update timestamp.
func TestWatchModel_ViewContainsTimestamp(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.rows = []StatusRow{
		{Workspace: "auth", Status: constants.TaskStatusRunning},
	}

	testTime := time.Date(2025, 12, 31, 14, 30, 45, 0, time.UTC)
	model.lastUpdate = testTime

	view := model.View()

	assert.Contains(t, view, "Last updated: 14:30:45")
}

// TestWatchModel_NoTimestampBeforeFirstRefresh tests no timestamp before first refresh.
func TestWatchModel_NoTimestampBeforeFirstRefresh(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
	mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	view := model.View()

	// Before first refresh, lastUpdate is zero, so no timestamp
	assert.NotContains(t, view, "Last updated:")
}

// TestWatchModel_ActionableSuggestion tests actionable command suggestion in footer.
func TestWatchModel_ActionableSuggestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    constants.TaskStatus
		wantCmd   string
		wantInCmd bool
	}{
		{
			name:      "awaiting approval suggests approve",
			status:    constants.TaskStatusAwaitingApproval,
			wantCmd:   "atlas approve",
			wantInCmd: true,
		},
		{
			name:      "validation failed suggests recover",
			status:    constants.TaskStatusValidationFailed,
			wantCmd:   "atlas recover",
			wantInCmd: true,
		},
		{
			name:      "CI failed suggests recover",
			status:    constants.TaskStatusCIFailed,
			wantCmd:   "atlas recover",
			wantInCmd: true,
		},
		{
			name:      "running has no suggestion",
			status:    constants.TaskStatusRunning,
			wantCmd:   "Run:",
			wantInCmd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockWs := &mockWorkspaceLister{workspaces: []*domain.Workspace{}}
			mockTask := &mockTaskLister{tasks: map[string][]*domain.Task{}}

			cfg := DefaultWatchConfig()
			model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
			model.rows = []StatusRow{
				{Workspace: "test-ws", Status: tt.status},
			}

			footer := model.buildFooter()
			if tt.wantInCmd {
				assert.Contains(t, footer, tt.wantCmd)
				assert.Contains(t, footer, "test-ws")
			} else {
				assert.NotContains(t, footer, tt.wantCmd)
			}
		})
	}
}

// TestWatchModel_MultipleRefreshes tests multiple refresh cycles.
func TestWatchModel_MultipleRefreshes(t *testing.T) {
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
		"auth": {{ID: "task-1", Status: constants.TaskStatusRunning, Steps: make([]domain.Step, 7)}},
	}

	mockWs := &mockWorkspaceLister{workspaces: workspaces}
	mockTask := &mockTaskLister{tasks: tasks}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)

	// Simulate first refresh
	msg1 := RefreshMsg{Rows: []StatusRow{{Workspace: "auth", Status: constants.TaskStatusRunning}}}
	updatedModel1, _ := model.Update(msg1)
	watchModel1 := updatedModel1.(*WatchModel)

	firstUpdate := watchModel1.lastUpdate

	// Wait a tiny bit and simulate second refresh
	time.Sleep(10 * time.Millisecond)
	msg2 := RefreshMsg{Rows: []StatusRow{{Workspace: "auth", Status: constants.TaskStatusCompleted}}}
	updatedModel2, _ := watchModel1.Update(msg2)
	watchModel2 := updatedModel2.(*WatchModel)

	secondUpdate := watchModel2.lastUpdate

	// Verify timestamp was updated
	assert.True(t, secondUpdate.After(firstUpdate), "second refresh should have later timestamp")
	assert.Equal(t, constants.TaskStatusCompleted, watchModel2.rows[0].Status)
}

// TestWatchModel_StatusPriority tests the statusPriority helper.
func TestWatchModel_StatusPriority(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{}
	mockTask := &mockTaskLister{}
	model := NewWatchModel(context.Background(), mockWs, mockTask, DefaultWatchConfig())

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
			assert.Equal(t, tt.expected, model.statusPriority(tt.status))
		})
	}
}

// TestWatchModel_TableRendering tests that table is rendered correctly in view.
func TestWatchModel_TableRendering(t *testing.T) {
	t.Parallel()

	mockWs := &mockWorkspaceLister{}
	mockTask := &mockTaskLister{}

	cfg := DefaultWatchConfig()
	model := NewWatchModel(context.Background(), mockWs, mockTask, cfg)
	model.rows = []StatusRow{
		{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
	}
	model.lastUpdate = time.Now()
	model.width = 120 // Set a wide terminal

	view := model.View()

	// Verify table content is present
	assert.True(t, strings.Contains(view, "auth") && strings.Contains(view, "feat/auth"),
		"view should contain workspace and branch")
	assert.Contains(t, view, "running")
	assert.Contains(t, view, "3/7")
}
