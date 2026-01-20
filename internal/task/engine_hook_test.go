package task

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/template/steps"
)

var (
	errTest = errors.New("test error")
	errHook = errors.New("hook error")
)

// mockHookManager implements HookManager interface for testing.
type mockHookManager struct {
	mu sync.Mutex

	// Track method calls
	createCalls    int
	completeCalls  int
	failCalls      int
	transitionStep int
	completeStep   int
	failStep       int

	// Store last error passed to FailTask
	lastFailError error

	// Configure errors
	createErr   error
	completeErr error
	failErr     error
}

func newMockHookManager() *mockHookManager {
	return &mockHookManager{}
}

func (m *mockHookManager) CreateHook(_ context.Context, _ *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return m.createErr
}

func (m *mockHookManager) ReadyHook(_ context.Context, _ *domain.Task) error {
	return nil
}

func (m *mockHookManager) CompleteTask(_ context.Context, _ *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeCalls++
	return m.completeErr
}

func (m *mockHookManager) FailTask(_ context.Context, _ *domain.Task, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failCalls++
	m.lastFailError = err
	return m.failErr
}

func (m *mockHookManager) TransitionStep(_ context.Context, _ *domain.Task, _ string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitionStep++
	return nil
}

func (m *mockHookManager) CompleteStep(_ context.Context, _ *domain.Task, _ string, _ []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeStep++
	return nil
}

func (m *mockHookManager) FailStep(_ context.Context, _ *domain.Task, _ string, _ error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failStep++
	return nil
}

func (m *mockHookManager) StartIntervalCheckpointing(_ context.Context, _ *domain.Task) error {
	return nil
}

func (m *mockHookManager) StopIntervalCheckpointing(_ context.Context, _ *domain.Task) error {
	return nil
}

func (m *mockHookManager) CreateValidationReceipt(_ context.Context, _ *domain.Task, _ string, _ *domain.StepResult) error {
	return nil
}

func (m *mockHookManager) getFailCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failCalls
}

func (m *mockHookManager) getLastFailError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastFailError
}

func (m *mockHookManager) getFailStepCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failStep
}

// TestFailHookTask_CallsHookManager tests that failHookTask calls the hook manager
func TestFailHookTask_CallsHookManager(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)

	engine.failHookTask(ctx, task, errTest)

	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Equal(t, errTest, hookManager.getLastFailError())
}

// TestFailHookTask_NilHookManager tests that failHookTask does nothing with nil hook manager
func TestFailHookTask_NilHookManager(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger()) // No hook manager

	task := newTestTask(constants.TaskStatusRunning)

	// Should not panic
	engine.failHookTask(ctx, task, errTest)
}

// TestFailHookTask_LogsWarningOnError tests that errors are logged but not returned
func TestFailHookTask_LogsWarningOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()
	hookManager.failErr = errHook

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)

	// Should not panic even with hook error
	engine.failHookTask(ctx, task, errTest)

	assert.Equal(t, 1, hookManager.getFailCalls())
}

// TestTransitionToErrorState_CallsFailHookStep tests that transitioning to error state calls failHookStep
// (not failHookTask) because these error states are recoverable via resume
func TestTransitionToErrorState_CallsFailHookStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	// Add steps so we can get step name
	task.Steps = []domain.Step{
		{Name: "validate", Type: domain.StepTypeValidation, Status: constants.StepStatusRunning},
	}
	task.CurrentStep = 0

	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
	// Should call failHookStep, not failHookTask (for recoverable errors)
	assert.Equal(t, 1, hookManager.getFailStepCalls(), "should call failHookStep for recoverable error")
	assert.Equal(t, 0, hookManager.getFailCalls(), "should NOT call failHookTask for recoverable error")
}

// TestAbandon_CallsFailHookTask tests that Abandon calls failHookTask
func TestAbandon_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusValidationFailed)
	task.Transitions = []domain.Transition{
		{ToStatus: constants.TaskStatusRunning},
		{ToStatus: constants.TaskStatusValidating},
		{ToStatus: constants.TaskStatusValidationFailed},
	}

	err := engine.Abandon(ctx, task, "user requested", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Contains(t, hookManager.getLastFailError().Error(), "task abandoned")
}

// TestRunSteps_ContextCancellation_CallsFailHookStep tests that context cancellation calls failHookStep
// (not failHookTask) because interruptions are recoverable via resume
func TestRunSteps_ContextCancellation_CallsFailHookStep(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	store := newMockStore()
	hookManager := newMockHookManager()
	registry := steps.NewExecutorRegistry()

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	task.CurrentStep = 0 // Start from step 0
	task.Steps = []domain.Step{
		{Name: "step1", Type: domain.StepTypeAI, Status: constants.StepStatusPending},
	}
	template := &domain.Template{
		Name: "test",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	// Cancel context immediately
	cancel()

	err := engine.runSteps(ctx, task, template)

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	// Should call failHookStep, not failHookTask (for recoverable interruption)
	assert.Equal(t, 1, hookManager.getFailStepCalls(), "should call failHookStep for context cancellation")
	assert.Equal(t, 0, hookManager.getFailCalls(), "should NOT call failHookTask for context cancellation")
}

// TestHandleCIFailure_CallsFailHookStep tests that CI failure calls failHookStep
// (not failHookTask) because CI failures are recoverable via resume
func TestHandleCIFailure_CallsFailHookStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		StepName: "ci_watch",
		Error:    "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	// Should call failHookStep, not failHookTask (for recoverable CI failure)
	assert.Equal(t, 1, hookManager.getFailStepCalls(), "should call failHookStep for CI failure")
	assert.Equal(t, 0, hookManager.getFailCalls(), "should NOT call failHookTask for CI failure")
}

// TestHandleGHFailure_CallsFailHookStep tests that GitHub failure calls failHookStep
// (not failHookTask) because GH failures are recoverable via resume
func TestHandleGHFailure_CallsFailHookStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		StepName: "git_push",
		Error:    "gh_failed: non_fast_forward",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	// Should call failHookStep, not failHookTask (for recoverable GH failure)
	assert.Equal(t, 1, hookManager.getFailStepCalls(), "should call failHookStep for GH failure")
	assert.Equal(t, 0, hookManager.getFailCalls(), "should NOT call failHookTask for GH failure")
}

// TestHandleCITimeout_CallsFailHookStep tests that CI timeout calls failHookStep
// (not failHookTask) because CI timeouts are recoverable via resume
func TestHandleCITimeout_CallsFailHookStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)
	result := &domain.StepResult{
		StepName: "ci_watch",
		Error:    "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
	// Should call failHookStep, not failHookTask (for recoverable CI timeout)
	assert.Equal(t, 1, hookManager.getFailStepCalls(), "should call failHookStep for CI timeout")
	assert.Equal(t, 0, hookManager.getFailCalls(), "should NOT call failHookTask for CI timeout")
}

// TestNoHookCalls_WhenNoHookManager tests that no panic occurs without hook manager
func TestNoHookCalls_WhenNoHookManager(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger()) // No hook manager

	task := newTestTask(constants.TaskStatusRunning)
	task.Steps = []domain.Step{
		{Name: "validate", Type: domain.StepTypeValidation, Status: constants.StepStatusRunning},
	}
	task.CurrentStep = 0

	// All of these should not panic without hook manager
	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")
	require.NoError(t, err)

	task = newTestTask(constants.TaskStatusValidationFailed)
	task.Transitions = []domain.Transition{
		{ToStatus: constants.TaskStatusRunning},
		{ToStatus: constants.TaskStatusValidating},
		{ToStatus: constants.TaskStatusValidationFailed},
	}
	err = engine.Abandon(ctx, task, "user requested", false)
	require.NoError(t, err)

	task = newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{StepName: "ci_watch", Error: "CI failed"}
	err = engine.handleCIFailure(ctx, task, result, ciResult)
	require.NoError(t, err)
}

// TestTransitionToErrorState_EmptyStepsArray tests that transitionToErrorState handles empty steps gracefully
func TestTransitionToErrorState_EmptyStepsArray(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	// Task has no steps - edge case
	task.Steps = nil
	task.CurrentStep = 0

	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
	// Should still call failHookStep with empty step name
	assert.Equal(t, 1, hookManager.getFailStepCalls())
}

// TestTransitionToErrorState_InvalidCurrentStep tests that transitionToErrorState handles out-of-bounds step index
func TestTransitionToErrorState_InvalidCurrentStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	task.Steps = []domain.Step{
		{Name: "step1", Type: domain.StepTypeValidation},
	}
	task.CurrentStep = 10 // Out of bounds

	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
	// Should still call failHookStep with empty step name (bounds check prevents panic)
	assert.Equal(t, 1, hookManager.getFailStepCalls())
}

// TestTransitionToErrorState_NegativeCurrentStep tests that transitionToErrorState handles negative step index
func TestTransitionToErrorState_NegativeCurrentStep(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	task.Steps = []domain.Step{
		{Name: "step1", Type: domain.StepTypeValidation},
	}
	task.CurrentStep = -1 // Negative index

	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
	// Should still call failHookStep with empty step name (bounds check prevents panic)
	assert.Equal(t, 1, hookManager.getFailStepCalls())
}

// TestRecoverableErrorsUseFailHookStep verifies that all recoverable error states
// use failHookStep (not failHookTask) to allow resume functionality
func TestRecoverableErrorsUseFailHookStep(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		setup    func(*Engine, *domain.Task) error
		expected constants.TaskStatus
	}{
		{
			name: "validation_failed",
			setup: func(e *Engine, task *domain.Task) error {
				return e.transitionToErrorState(context.Background(), task, domain.StepTypeValidation, "test error")
			},
			expected: constants.TaskStatusValidationFailed,
		},
		{
			name: "gh_failed",
			setup: func(e *Engine, task *domain.Task) error {
				return e.transitionToErrorState(context.Background(), task, domain.StepTypeGit, "test error")
			},
			expected: constants.TaskStatusGHFailed,
		},
		{
			name: "ci_failed",
			setup: func(e *Engine, task *domain.Task) error {
				return e.transitionToErrorState(context.Background(), task, domain.StepTypeCI, "test error")
			},
			expected: constants.TaskStatusCIFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newMockStore()
			hookManager := newMockHookManager()
			engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

			task := newTestTask(constants.TaskStatusRunning)
			task.Steps = []domain.Step{{Name: "test_step", Type: domain.StepTypeValidation}}
			task.CurrentStep = 0

			err := tc.setup(engine, task)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, task.Status)
			// Key assertion: recoverable errors use failHookStep, NOT failHookTask
			assert.Equal(t, 1, hookManager.getFailStepCalls(), "recoverable error %s should call failHookStep", tc.name)
			assert.Equal(t, 0, hookManager.getFailCalls(), "recoverable error %s should NOT call failHookTask", tc.name)
		})
	}
}

// TestAbandon_UsesFailHookTask verifies that Abandon (terminal state) still uses failHookTask
func TestAbandon_UsesFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusGHFailed) // Start from a recoverable error state
	task.Transitions = []domain.Transition{
		{ToStatus: constants.TaskStatusRunning},
		{ToStatus: constants.TaskStatusGHFailed},
	}

	err := engine.Abandon(ctx, task, "user explicitly abandoned", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
	// Key assertion: Abandon is terminal, so it should use failHookTask (not failHookStep)
	assert.Equal(t, 1, hookManager.getFailCalls(), "Abandon should call failHookTask for terminal state")
	assert.Equal(t, 0, hookManager.getFailStepCalls(), "Abandon should NOT call failHookStep")
}
