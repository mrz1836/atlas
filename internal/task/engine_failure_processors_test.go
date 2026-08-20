package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// These tests cover the exported failure-recovery action processors, which
// mutation testing found were exercised only indirectly: the state transition
// and — crucially — the store.Update persistence of each recovery decision were
// never asserted. A regression that dropped the persistence (e.g. a flipped
// error check that returned before store.Update) would have gone unnoticed,
// leaving a recovery choice unsaved.

func newFailureProcessorEngine(t *testing.T) (*Engine, *mockStore) {
	t.Helper()
	store := newMockStore()
	engine := NewEngine(store, steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
	return engine, store
}

func TestProcessGHFailureAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     GHFailureAction
		wantStatus constants.TaskStatus
		wantMeta   string
	}{
		{"retry_transitions_to_running", GHFailureRetry, constants.TaskStatusRunning, ""},
		{"fix_and_retry_stays_gh_failed", GHFailureFixAndRetry, constants.TaskStatusGHFailed, "awaiting_manual_fix"},
		{"abandon_transitions_to_abandoned", GHFailureAbandon, constants.TaskStatusAbandoned, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine, store := newFailureProcessorEngine(t)
			task := &domain.Task{ID: "gh-1", WorkspaceID: "ws", Status: constants.TaskStatusGHFailed}

			before := store.updateCalls
			require.NoError(t, engine.ProcessGHFailureAction(context.Background(), task, tt.action))

			assert.Equal(t, tt.wantStatus, task.Status, "action must leave the task in the expected state")
			assert.Greater(t, store.updateCalls, before, "recovery decision must be persisted via store.Update")
			if tt.wantMeta != "" {
				assert.Contains(t, task.Metadata, tt.wantMeta)
			}
		})
	}
}

func TestProcessGHFailureAction_ContextCanceled(t *testing.T) {
	t.Parallel()
	engine, store := newFailureProcessorEngine(t)
	task := &domain.Task{ID: "gh-2", WorkspaceID: "ws", Status: constants.TaskStatusGHFailed}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := engine.ProcessGHFailureAction(ctx, task, GHFailureRetry)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, store.updateCalls, "a canceled action must not persist anything")
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status, "status must be untouched on cancellation")
}

func TestProcessCITimeoutAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     CITimeoutAction
		wantStatus constants.TaskStatus
		wantMeta   string
	}{
		{"continue_waiting_transitions_to_running", CITimeoutContinueWaiting, constants.TaskStatusRunning, "extended_ci_timeout"},
		{"retry_transitions_to_running", CITimeoutRetry, constants.TaskStatusRunning, ""},
		{"fix_manually_stays_ci_timeout", CITimeoutFixManually, constants.TaskStatusCITimeout, "awaiting_manual_fix"},
		{"abandon_transitions_to_abandoned", CITimeoutAbandon, constants.TaskStatusAbandoned, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine, store := newFailureProcessorEngine(t)
			task := &domain.Task{ID: "ci-to-1", WorkspaceID: "ws", Status: constants.TaskStatusCITimeout}

			before := store.updateCalls
			require.NoError(t, engine.ProcessCITimeoutAction(context.Background(), task, tt.action))

			assert.Equal(t, tt.wantStatus, task.Status)
			assert.Greater(t, store.updateCalls, before, "recovery decision must be persisted via store.Update")
			if tt.wantMeta != "" {
				assert.Contains(t, task.Metadata, tt.wantMeta)
			}
		})
	}
}

func TestProcessCIFailureResult(t *testing.T) {
	t.Parallel()

	t.Run("view_logs_is_noop_no_persist", func(t *testing.T) {
		t.Parallel()
		engine, store := newFailureProcessorEngine(t)
		task := &domain.Task{ID: "ci-1", WorkspaceID: "ws", Status: constants.TaskStatusCIFailed}

		require.NoError(t, engine.processCIFailureResult(context.Background(), task, CIFailureViewLogs, &CIFailureResult{}))
		assert.Equal(t, constants.TaskStatusCIFailed, task.Status, "view logs keeps the task in CIFailed")
		assert.Equal(t, 0, store.updateCalls, "view logs must not persist")
	})

	tests := []struct {
		name       string
		action     CIFailureAction
		wantStatus constants.TaskStatus
	}{
		{"retry_implement_transitions_to_running", CIFailureRetryImplement, constants.TaskStatusRunning},
		{"fix_manually_stays_ci_failed", CIFailureFixManually, constants.TaskStatusCIFailed},
		{"abandon_transitions_to_abandoned", CIFailureAbandon, constants.TaskStatusAbandoned},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine, store := newFailureProcessorEngine(t)
			task := &domain.Task{ID: "ci-2", WorkspaceID: "ws", Status: constants.TaskStatusCIFailed}

			before := store.updateCalls
			require.NoError(t, engine.processCIFailureResult(context.Background(), task, tt.action, &CIFailureResult{ErrorContext: "boom", Message: "fix it"}))

			assert.Equal(t, tt.wantStatus, task.Status)
			assert.Greater(t, store.updateCalls, before, "recovery decision must be persisted via store.Update")
		})
	}
}
