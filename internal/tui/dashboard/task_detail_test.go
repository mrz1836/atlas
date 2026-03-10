package dashboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// makeDetailTask creates a fully-populated TaskInfo for detail panel testing.
func makeDetailTask() dashboard.TaskInfo {
	now := time.Now()
	return dashboard.TaskInfo{
		ID:          "task-abc123",
		Description: "Fix null pointer in auth handler",
		Status:      dashboard.TaskStatusRunning,
		Priority:    "high",
		Template:    "bug-fix",
		Agent:       "claude",
		Model:       "claude-opus-4",
		Branch:      "fix/auth-null-ptr",
		Workspace:   "ws-abc123",
		CurrentStep: "validate",
		StepIndex:   3,
		StepTotal:   7,
		PRURL:       "https://github.com/org/repo/pull/42",
		SubmittedAt: now.Add(-10 * time.Minute),
		StartedAt:   now.Add(-8 * time.Minute),
	}
}

// ── Empty state ───────────────────────────────────────────────────────────────

func TestTaskDetail_View_EmptyState_WhenNoTask(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	view := td.View(80, 20)
	if !strings.Contains(view, "Select a task") {
		t.Errorf("empty state not shown:\n%s", view)
	}
}

func TestTaskDetail_View_HeightExact_WhenEmpty(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	view := td.View(80, 10)
	lines := strings.Split(view, "\n")
	if len(lines) != 10 {
		t.Errorf("empty view has %d lines, want 10", len(lines))
	}
}

// ── Metadata display ──────────────────────────────────────────────────────────

func TestTaskDetail_View_ShowsDescription(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "Fix null pointer") {
		t.Errorf("description not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsTemplate(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "bug-fix") {
		t.Errorf("template not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsAgent(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "claude") {
		t.Errorf("agent not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsModel(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "claude-opus-4") {
		t.Errorf("model not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsBranch(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "fix/auth-null-ptr") {
		t.Errorf("branch not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsWorkspace(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "ws-abc123") {
		t.Errorf("workspace not found in detail view:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsPriority(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "high") {
		t.Errorf("priority not found in detail view:\n%s", view)
	}
}

// ── Step progress rendering ───────────────────────────────────────────────────

func TestTaskDetail_View_ShowsStepProgress(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask() // StepIndex=3, StepTotal=7, CurrentStep="validate"
	td.SetTask(&task)
	view := td.View(100, 40)
	// Should show the step progress section header.
	if !strings.Contains(view, "Steps") {
		t.Errorf("step progress section not found:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsRunningStepIcon(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(100, 40)
	// The running step indicator "▶" should appear for current step.
	if !strings.Contains(view, "▶") {
		t.Errorf("running step icon ▶ not found:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsCompletedStepIcon(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask() // Steps 1,2 are done (StepIndex=3 means 1,2 complete)
	td.SetTask(&task)
	view := td.View(100, 40)
	if !strings.Contains(view, "✓") {
		t.Errorf("completed step icon ✓ not found:\n%s", view)
	}
}

func TestTaskDetail_View_ShowsFutureStepIcon(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask() // Steps 4-7 are pending
	td.SetTask(&task)
	view := td.View(100, 40)
	if !strings.Contains(view, "○") {
		t.Errorf("future step icon ○ not found:\n%s", view)
	}
}

func TestTaskDetail_View_NoStepSection_WhenNoSteps(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	task.StepTotal = 0
	task.StepIndex = 0
	task.CurrentStep = ""
	td.SetTask(&task)
	view := td.View(100, 30)
	if strings.Contains(view, "Steps") {
		t.Errorf("step section shown unexpectedly when no steps:\n%s", view)
	}
}

func TestTaskDetail_View_CurrentStepOnly_WhenNoTotal(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	task.StepTotal = 0
	task.StepIndex = 0
	task.CurrentStep = "lint"
	td.SetTask(&task)
	view := td.View(100, 30)
	if !strings.Contains(view, "lint") {
		t.Errorf("current step 'lint' not found when step total unknown:\n%s", view)
	}
}

// ── Log tail ──────────────────────────────────────────────────────────────────

func TestTaskDetail_View_ShowsLogTail(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	td.SetLogTail([]string{
		"2026-01-01 12:00:00 INFO  starting analysis",
		"2026-01-01 12:00:01 INFO  running linter",
	})
	view := td.View(100, 40)
	if !strings.Contains(view, "Recent Logs") {
		t.Errorf("log tail section header not found:\n%s", view)
	}
}

func TestTaskDetail_View_LogTailContent(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	td.SetLogTail([]string{"running linter"})
	view := td.View(100, 40)
	if !strings.Contains(view, "running linter") {
		t.Errorf("log line content not found:\n%s", view)
	}
}

func TestTaskDetail_View_NoLogSection_WhenEmpty(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	// No log tail set.
	view := td.View(100, 30)
	if strings.Contains(view, "Recent Logs") {
		t.Errorf("log section shown when log tail is empty:\n%s", view)
	}
}

// ── SetTask / Task accessor ───────────────────────────────────────────────────

func TestTaskDetail_Task_ReturnsNilInitially(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	if td.Task() != nil {
		t.Error("Task() should be nil on a new TaskDetail")
	}
}

func TestTaskDetail_SetTask_NilClears(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	td.SetTask(nil)
	view := td.View(80, 10)
	if !strings.Contains(view, "Select a task") {
		t.Errorf("clearing task did not show empty state:\n%s", view)
	}
}

// ── Output dimensions ─────────────────────────────────────────────────────────

func TestTaskDetail_View_HeightExact(t *testing.T) {
	t.Parallel()
	td := dashboard.NewTaskDetail()
	task := makeDetailTask()
	td.SetTask(&task)
	view := td.View(80, 20)
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Errorf("detail view has %d lines, want 20", len(lines))
	}
}
