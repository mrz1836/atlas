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

// TestTransitionToErrorState_CallsFailHookTask tests that transitioning to error state calls failHookTask
func TestTransitionToErrorState_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)

	err := engine.transitionToErrorState(ctx, task, domain.StepTypeValidation, "validation failed")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Contains(t, hookManager.getLastFailError().Error(), "validation failed")
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

// TestRunSteps_ContextCancellation_CallsFailHookTask tests that context cancellation calls failHookTask
func TestRunSteps_ContextCancellation_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	store := newMockStore()
	hookManager := newMockHookManager()
	registry := steps.NewExecutorRegistry()

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	task.CurrentStep = 0 // Start from step 0
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
	assert.Equal(t, 1, hookManager.getFailCalls())
}

// TestHandleCIFailure_CallsFailHookTask tests that CI failure calls failHookTask
func TestHandleCIFailure_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusFailure)
	result := &domain.StepResult{
		Error: "CI checks failed",
	}

	err := engine.handleCIFailure(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Contains(t, hookManager.getLastFailError().Error(), "ci workflow failed")
}

// TestHandleGHFailure_CallsFailHookTask tests that GitHub failure calls failHookTask
func TestHandleGHFailure_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	result := &domain.StepResult{
		Error: "gh_failed: non_fast_forward",
	}

	err := engine.handleGHFailure(ctx, task, result)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Contains(t, hookManager.getLastFailError().Error(), "github operation failed")
}

// TestHandleCITimeout_CallsFailHookTask tests that CI timeout calls failHookTask
func TestHandleCITimeout_CallsFailHookTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()
	hookManager := newMockHookManager()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger(), WithHookManager(hookManager))

	task := newTestTask(constants.TaskStatusRunning)
	ciResult := newTestCIResult(git.CIStatusTimeout)
	result := &domain.StepResult{
		Error: "CI monitoring timed out",
	}

	err := engine.handleCITimeout(ctx, task, result, ciResult)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusCITimeout, task.Status)
	assert.Equal(t, 1, hookManager.getFailCalls())
	assert.Contains(t, hookManager.getLastFailError().Error(), "ci polling timeout")
}

// TestNoFailHookTask_WhenNoHookManager tests that no panic occurs without hook manager
func TestNoFailHookTask_WhenNoHookManager(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMockStore()

	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger()) // No hook manager

	task := newTestTask(constants.TaskStatusRunning)

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
	result := &domain.StepResult{Error: "CI failed"}
	err = engine.handleCIFailure(ctx, task, result, ciResult)
	require.NoError(t, err)
}
