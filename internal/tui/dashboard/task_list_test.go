package dashboard_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// makeTask creates a minimal TaskInfo for test use.
func makeTask(id, desc string, status dashboard.TaskStatus) dashboard.TaskInfo {
	return dashboard.TaskInfo{
		ID:          id,
		Description: desc,
		Status:      status,
		SubmittedAt: time.Now().Add(-time.Minute),
	}
}

// ── Navigation tests ──────────────────────────────────────────────────────────

func TestTaskList_Cursor_StartsAtZero(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	if tl.Cursor() != 0 {
		t.Errorf("initial cursor = %d, want 0", tl.Cursor())
	}
}

func TestTaskList_MoveDown_IncrementsCursor(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "first", dashboard.TaskStatusQueued),
		makeTask("2", "second", dashboard.TaskStatusRunning),
	}
	tl.SetItems(items)

	updated, _ := tl.Update(tea.KeyPressMsg{Text: "j"})
	if updated.Cursor() != 1 {
		t.Errorf("cursor after j = %d, want 1", updated.Cursor())
	}
}

func TestTaskList_MoveUp_DecrementsCursor(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "first", dashboard.TaskStatusQueued),
		makeTask("2", "second", dashboard.TaskStatusRunning),
	}
	tl.SetItems(items)

	// Move down then up.
	tl, _ = tl.Update(tea.KeyPressMsg{Text: "j"})
	tl, _ = tl.Update(tea.KeyPressMsg{Text: "k"})
	if tl.Cursor() != 0 {
		t.Errorf("cursor after j+k = %d, want 0", tl.Cursor())
	}
}

func TestTaskList_MoveDown_DoesNotExceedBound(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "only", dashboard.TaskStatusQueued),
	}
	tl.SetItems(items)

	tl, _ = tl.Update(tea.KeyPressMsg{Text: "j"})
	if tl.Cursor() != 0 {
		t.Errorf("cursor after j on single item = %d, want 0", tl.Cursor())
	}
}

func TestTaskList_MoveUp_DoesNotGoBelowZero(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "only", dashboard.TaskStatusQueued),
	}
	tl.SetItems(items)

	tl, _ = tl.Update(tea.KeyPressMsg{Text: "k"})
	if tl.Cursor() != 0 {
		t.Errorf("cursor after k at top = %d, want 0", tl.Cursor())
	}
}

func TestTaskList_UpArrow_MovesCursor(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "a", dashboard.TaskStatusQueued),
		makeTask("2", "b", dashboard.TaskStatusRunning),
	}
	tl.SetItems(items)
	tl, _ = tl.Update(tea.KeyPressMsg{Text: "j"})
	tl, _ = tl.Update(tea.KeyPressMsg{Text: "up"})
	if tl.Cursor() != 0 {
		t.Errorf("cursor after up = %d, want 0", tl.Cursor())
	}
}

func TestTaskList_DownArrow_MovesCursor(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("1", "a", dashboard.TaskStatusQueued),
		makeTask("2", "b", dashboard.TaskStatusRunning),
	}
	tl.SetItems(items)
	tl, _ = tl.Update(tea.KeyPressMsg{Text: "down"})
	if tl.Cursor() != 1 {
		t.Errorf("cursor after down = %d, want 1", tl.Cursor())
	}
}

func TestTaskList_NavigationEmitsTaskSelectedMsg(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	items := []dashboard.TaskInfo{
		makeTask("task-1", "first", dashboard.TaskStatusQueued),
		makeTask("task-2", "second", dashboard.TaskStatusRunning),
	}
	tl.SetItems(items)

	_, cmd := tl.Update(tea.KeyPressMsg{Text: "j"})
	if cmd == nil {
		t.Fatal("expected a cmd after navigation, got nil")
	}
	msg := cmd()
	sel, ok := msg.(dashboard.TaskSelectedMsg)
	if !ok {
		t.Fatalf("expected TaskSelectedMsg, got %T", msg)
	}
	if sel.TaskID != "task-2" {
		t.Errorf("TaskSelectedMsg.TaskID = %q, want %q", sel.TaskID, "task-2")
	}
}

func TestTaskList_NoNavigation_NilCmd(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "a", dashboard.TaskStatusRunning)})

	// j at bottom — no change, no cmd.
	_, cmd := tl.Update(tea.KeyPressMsg{Text: "j"})
	if cmd != nil {
		t.Error("expected nil cmd when cursor does not move")
	}
}

// ── Status icon tests ─────────────────────────────────────────────────────────

func TestTaskList_View_ContainsRunningIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusRunning)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "●") {
		t.Errorf("running icon ● not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsQueuedIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusQueued)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "○") {
		t.Errorf("queued icon ○ not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsApprovalIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusAwaitingApproval)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "◉") {
		t.Errorf("approval icon ◉ not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsCompletedIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusCompleted)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "✓") {
		t.Errorf("completed icon ✓ not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsFailedIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusFailed)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "✗") {
		t.Errorf("failed icon ✗ not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsPausedIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusPaused)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "⏸") {
		t.Errorf("paused icon ⏸ not found in view:\n%s", view)
	}
}

func TestTaskList_View_ContainsAbandonedIcon(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("1", "mytask", dashboard.TaskStatusAbandoned)})
	view := tl.View(80, 10)
	if !strings.Contains(view, "⊘") {
		t.Errorf("abandoned icon ⊘ not found in view:\n%s", view)
	}
}

// ── Auto-scroll tests ─────────────────────────────────────────────────────────

func TestTaskList_View_AutoScroll_ShowsSelection(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	// Create more items than fit in the view (height=3).
	items := make([]dashboard.TaskInfo, 10)
	for i := range items {
		items[i] = makeTask(
			string(rune('A'+i)),
			string(rune('A'+i))+"-task",
			dashboard.TaskStatusQueued,
		)
	}
	tl.SetItems(items)

	// Navigate to the last item (index 9), which is out of the initial 3-row window.
	for range 9 {
		tl, _ = tl.Update(tea.KeyPressMsg{Text: "j"})
	}

	view := tl.View(40, 3)
	// The selected task description should appear somewhere in the view.
	if !strings.Contains(view, "J-task") {
		t.Errorf("selected task J-task not visible in view after auto-scroll:\n%s", view)
	}
}

func TestTaskList_View_HeightMatchesExactly(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{
		makeTask("1", "one", dashboard.TaskStatusQueued),
		makeTask("2", "two", dashboard.TaskStatusRunning),
	})
	view := tl.View(40, 5)
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Errorf("View returned %d lines, want 5", len(lines))
	}
}

func TestTaskList_View_EmptyState(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	view := tl.View(40, 5)
	if !strings.Contains(view, "No tasks") {
		t.Errorf("empty state not shown:\n%s", view)
	}
}

func TestTaskList_SetItems_ClampsOvershotCursor(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	// Start with 5 items, navigate to last.
	items5 := make([]dashboard.TaskInfo, 5)
	for i := range items5 {
		items5[i] = makeTask(string(rune('A'+i)), "task", dashboard.TaskStatusQueued)
	}
	tl.SetItems(items5)
	for range 4 {
		tl, _ = tl.Update(tea.KeyPressMsg{Text: "j"})
	}
	if tl.Cursor() != 4 {
		t.Fatalf("pre-condition: cursor = %d, want 4", tl.Cursor())
	}

	// Shrink to 2 items — cursor must clamp.
	items2 := items5[:2]
	tl.SetItems(items2)
	if tl.Cursor() >= 2 {
		t.Errorf("cursor not clamped after SetItems: cursor = %d, want < 2", tl.Cursor())
	}
}

func TestTaskList_SelectedID_Empty(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	if id := tl.SelectedID(); id != "" {
		t.Errorf("SelectedID on empty list = %q, want \"\"", id)
	}
}

func TestTaskList_SelectedID_AfterSet(t *testing.T) {
	t.Parallel()
	tl := dashboard.NewTaskList()
	tl.SetItems([]dashboard.TaskInfo{makeTask("abc", "desc", dashboard.TaskStatusRunning)})
	if id := tl.SelectedID(); id != "abc" {
		t.Errorf("SelectedID = %q, want \"abc\"", id)
	}
}
