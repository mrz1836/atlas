package task

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// newCapturingEngine builds an engine whose progress callback records every
// emitted StepProgressEvent into the returned slice pointer.
func newCapturingEngine(t *testing.T) (*Engine, *[]StepProgressEvent) {
	t.Helper()
	events := &[]StepProgressEvent{}
	cfg := DefaultEngineConfig()
	cfg.ProgressCallback = func(e StepProgressEvent) {
		*events = append(*events, e)
	}
	engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), cfg, testLogger())
	return engine, events
}

// TestEngine_NotifyStepStart verifies notifyStepStart emits a start event with
// agent/model populated only for AI/verify steps, and is a no-op without a callback.
func TestEngine_NotifyStepStart(t *testing.T) {
	t.Parallel()

	t.Run("ai_step_populates_agent_and_model", func(t *testing.T) {
		t.Parallel()
		engine, events := newCapturingEngine(t)

		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "ws",
			CurrentStep: 2,
			Config:      domain.TaskConfig{Agent: domain.AgentClaude, Model: "opus"},
		}
		step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}

		engine.notifyStepStart(task, step, 5)

		require.Len(t, *events, 1)
		ev := (*events)[0]
		assert.Equal(t, "start", ev.Type)
		assert.Equal(t, "task-1", ev.TaskID)
		assert.Equal(t, "ws", ev.WorkspaceName)
		assert.Equal(t, 2, ev.StepIndex)
		assert.Equal(t, 5, ev.TotalSteps)
		assert.Equal(t, "implement", ev.StepName)
		assert.Equal(t, domain.StepTypeAI, ev.StepType)
		assert.Equal(t, "claude", ev.Agent)
		assert.Equal(t, "opus", ev.Model)
	})

	t.Run("non_ai_step_leaves_agent_and_model_empty", func(t *testing.T) {
		t.Parallel()
		engine, events := newCapturingEngine(t)

		task := &domain.Task{
			ID:     "task-2",
			Config: domain.TaskConfig{Agent: domain.AgentClaude, Model: "opus"},
		}
		step := &domain.StepDefinition{Name: "push", Type: domain.StepTypeGit}

		engine.notifyStepStart(task, step, 3)

		require.Len(t, *events, 1)
		ev := (*events)[0]
		assert.Empty(t, ev.Agent)
		assert.Empty(t, ev.Model)
	})

	t.Run("no_callback_is_a_noop", func(t *testing.T) {
		t.Parallel()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
		task := &domain.Task{ID: "task-3"}
		step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}

		assert.NotPanics(t, func() {
			engine.notifyStepStart(task, step, 1)
		})
	})
}

// TestEngine_NotifyStepComplete verifies notifyStepComplete emits a complete
// event carrying the result status, output, and completion metrics.
func TestEngine_NotifyStepComplete(t *testing.T) {
	t.Parallel()

	t.Run("ai_step_includes_metrics_and_agent_model", func(t *testing.T) {
		t.Parallel()
		engine, events := newCapturingEngine(t)

		task := &domain.Task{
			ID:          "task-1",
			WorkspaceID: "ws",
			CurrentStep: 1,
			Config:      domain.TaskConfig{Agent: domain.AgentClaude, Model: "opus"},
		}
		step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}
		result := &domain.StepResult{
			Status:       constants.StepStatusSuccess,
			Output:       "done",
			DurationMs:   1234,
			NumTurns:     4,
			FilesChanged: []string{"a.go", "b.go"},
		}

		engine.notifyStepComplete(task, step, result, 3)

		require.Len(t, *events, 1)
		ev := (*events)[0]
		assert.Equal(t, "complete", ev.Type)
		assert.Equal(t, constants.StepStatusSuccess, ev.Status)
		assert.Equal(t, "done", ev.Output)
		assert.Equal(t, int64(1234), ev.DurationMs)
		assert.Equal(t, 4, ev.NumTurns)
		assert.Equal(t, 2, ev.FilesChangedCount)
		assert.Equal(t, "claude", ev.Agent)
		assert.Equal(t, "opus", ev.Model)
	})

	t.Run("non_ai_step_omits_agent_and_model", func(t *testing.T) {
		t.Parallel()
		engine, events := newCapturingEngine(t)

		task := &domain.Task{
			ID:     "task-2",
			Config: domain.TaskConfig{Agent: domain.AgentClaude, Model: "opus"},
		}
		step := &domain.StepDefinition{Name: "push", Type: domain.StepTypeGit}
		result := &domain.StepResult{Status: constants.StepStatusSuccess, Output: "pushed"}

		engine.notifyStepComplete(task, step, result, 3)

		require.Len(t, *events, 1)
		ev := (*events)[0]
		assert.Empty(t, ev.Agent)
		assert.Empty(t, ev.Model)
		assert.Equal(t, "pushed", ev.Output)
		assert.Zero(t, ev.FilesChangedCount)
	})

	t.Run("no_callback_is_a_noop", func(t *testing.T) {
		t.Parallel()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
		task := &domain.Task{ID: "task-3"}
		step := &domain.StepDefinition{Name: "implement", Type: domain.StepTypeAI}
		result := &domain.StepResult{Status: constants.StepStatusSuccess}

		assert.NotPanics(t, func() {
			engine.notifyStepComplete(task, step, result, 1)
		})
	})
}

// TestEngine_RecordMetrics verifies the record* helpers forward to a configured
// metrics collector and are safe no-ops when no collector is set.
func TestEngine_RecordMetrics(t *testing.T) {
	t.Parallel()

	t.Run("forwards_to_configured_metrics", func(t *testing.T) {
		t.Parallel()
		m := &mockMetrics{}
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger(), WithMetrics(m))

		engine.recordTaskStarted("task-1", "bugfix")
		engine.recordTaskCompleted("task-1", 5*time.Second, "completed")
		engine.recordStepExecuted("task-1", "implement", domain.StepTypeAI, 2*time.Second, true)

		require.Len(t, m.taskStartedCalls, 1)
		assert.Equal(t, "task-1", m.taskStartedCalls[0].taskID)
		assert.Equal(t, "bugfix", m.taskStartedCalls[0].templateName)

		require.Len(t, m.taskCompletedCalls, 1)
		assert.Equal(t, 5*time.Second, m.taskCompletedCalls[0].duration)
		assert.Equal(t, "completed", m.taskCompletedCalls[0].status)

		require.Len(t, m.stepExecutedCalls, 1)
		assert.Equal(t, "implement", m.stepExecutedCalls[0].stepName)
		assert.Equal(t, domain.StepTypeAI, m.stepExecutedCalls[0].stepType)
		assert.True(t, m.stepExecutedCalls[0].success)
	})

	t.Run("nil_metrics_is_a_noop", func(t *testing.T) {
		t.Parallel()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())

		assert.NotPanics(t, func() {
			engine.recordTaskStarted("task-1", "bugfix")
			engine.recordTaskCompleted("task-1", time.Second, "completed")
			engine.recordStepExecuted("task-1", "implement", domain.StepTypeAI, time.Second, false)
		})
	})
}

// TestEngine_HookStepNotifications verifies the hook step-transition helpers
// forward to a configured hook manager and are safe no-ops when none is set.
func TestEngine_HookStepNotifications(t *testing.T) {
	t.Parallel()

	t.Run("forwards_to_configured_hook_manager", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		hm := newMockHookManager()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger(), WithHookManager(hm))

		task := &domain.Task{ID: "task-hooks", WorkspaceID: "ws"}

		engine.transitionHookStep(ctx, task, "implement", 0)
		engine.completeHookStep(ctx, task, "implement", []string{"a.go"})
		engine.completeHookTask(ctx, task)

		assert.Equal(t, 1, hm.transitionStep)
		assert.Equal(t, 1, hm.completeStep)
		assert.Equal(t, 1, hm.completeCalls)
	})

	t.Run("nil_hook_manager_is_a_noop", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
		task := &domain.Task{ID: "task-nohooks"}

		assert.NotPanics(t, func() {
			engine.transitionHookStep(ctx, task, "implement", 0)
			engine.completeHookStep(ctx, task, "implement", nil)
			engine.completeHookTask(ctx, task)
		})
	})
}
