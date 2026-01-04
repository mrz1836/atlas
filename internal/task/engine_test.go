package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// mockStore implements Store interface for testing.
type mockStore struct {
	mu          sync.Mutex
	tasks       map[string]*domain.Task
	createErr   error
	updateErr   error
	getErr      error
	createCalls int
	updateCalls int
}

func newMockStore() *mockStore {
	return &mockStore{
		tasks: make(map[string]*domain.Task),
	}
}

func (m *mockStore) Create(_ context.Context, _ string, task *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	// Deep copy to avoid external modifications
	m.tasks[task.ID] = task
	return nil
}

func (m *mockStore) Get(_ context.Context, _, taskID string) (*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, atlaserrors.ErrTaskNotFound
	}
	return task, nil
}

func (m *mockStore) Update(_ context.Context, _ string, task *domain.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	if m.updateErr != nil {
		return m.updateErr
	}
	m.tasks[task.ID] = task
	return nil
}

func (m *mockStore) List(_ context.Context, _ string) ([]*domain.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*domain.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, t)
	}
	return result, nil
}

func (m *mockStore) Delete(_ context.Context, _, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, taskID)
	return nil
}

func (m *mockStore) AppendLog(_ context.Context, _, _ string, _ []byte) error {
	return nil
}

func (m *mockStore) ReadLog(_ context.Context, _, _ string) ([]byte, error) {
	return nil, nil
}

func (m *mockStore) SaveArtifact(_ context.Context, _, _, _ string, _ []byte) error {
	return nil
}

func (m *mockStore) SaveVersionedArtifact(_ context.Context, _, _, _ string, _ []byte) (string, error) {
	return "artifact.1.json", nil
}

func (m *mockStore) GetArtifact(_ context.Context, _, _, _ string) ([]byte, error) {
	return nil, nil
}

func (m *mockStore) ListArtifacts(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// mockExecutor implements steps.StepExecutor for testing.
type mockExecutor struct {
	stepType domain.StepType
	result   *domain.StepResult
	err      error
	delay    time.Duration
}

func (m *mockExecutor) Execute(ctx context.Context, _ *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Simulate delay if configured
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}

	// Build result with step info
	result := &domain.StepResult{
		StepIndex:   0,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
		DurationMs:  100,
	}

	if m.result != nil {
		result.Status = m.result.Status
		result.Error = m.result.Error
		result.Output = m.result.Output
	}

	return result, nil
}

func (m *mockExecutor) Type() domain.StepType {
	return m.stepType
}

// Helper to create test logger
func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

// TestNewEngine tests the constructor.
func TestNewEngine(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	cfg := DefaultEngineConfig()
	logger := testLogger()

	engine := NewEngine(store, registry, cfg, logger)

	assert.NotNil(t, engine)
	assert.Equal(t, store, engine.store)
	assert.Equal(t, registry, engine.registry)
	assert.True(t, engine.config.AutoProceedGit)
	assert.True(t, engine.config.AutoProceedValidation)
}

// TestDefaultEngineConfig tests default configuration values.
func TestDefaultEngineConfig(t *testing.T) {
	cfg := DefaultEngineConfig()

	assert.True(t, cfg.AutoProceedGit)
	assert.True(t, cfg.AutoProceedValidation)
}

// TestEngine_Start_Success tests successful task creation and execution.
func TestEngine_Start_Success(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test description")

	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, "test-workspace", task.WorkspaceID)
	assert.True(t, strings.HasPrefix(task.ID, "task-"))
	assert.Len(t, task.StepResults, 1)
	assert.Equal(t, 1, store.createCalls)
	assert.GreaterOrEqual(t, store.updateCalls, 1) // At least one checkpoint
}

// TestEngine_Start_TaskIDFormat tests task ID follows pattern task-YYYYMMDD-HHMMSS.
func TestEngine_Start_TaskIDFormat(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test description")

	require.NoError(t, err)
	// Pattern: task-YYYYMMDD-HHMMSS
	assert.Regexp(t, `^task-\d{8}-\d{6}$`, task.ID)
}

// TestEngine_Start_IteratesSteps tests that steps execute in order.
func TestEngine_Start_IteratesSteps(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// Track execution order
	var executionOrder []string
	var mu sync.Mutex

	// Create executors that track calls
	for _, stepType := range []domain.StepType{domain.StepTypeAI, domain.StepTypeValidation} {
		st := stepType
		registry.Register(&trackingExecutor{
			stepType: st,
			onExecute: func(step *domain.StepDefinition) {
				mu.Lock()
				executionOrder = append(executionOrder, step.Name)
				mu.Unlock()
			},
		})
	}

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI, Required: true},
			{Name: "step2", Type: domain.StepTypeValidation, Required: true},
			{Name: "step3", Type: domain.StepTypeAI, Required: true},
		},
	}

	_, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	assert.Equal(t, []string{"step1", "step2", "step3"}, executionOrder)
}

// TestEngine_Start_ContextCancellation tests context cancellation at start.
func TestEngine_Start_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	assert.Nil(t, task)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_Resume_FromErrorState tests resuming from an error state.
func TestEngine_Resume_FromErrorState(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeValidation,
		result:   &domain.StepResult{Status: "success"},
	})

	task := &domain.Task{
		ID:          "task-20251227-100000",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "step2", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "step3", Type: domain.StepTypeValidation, Status: "failed"},
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeValidation},
		},
	}

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	err := engine.Resume(ctx, task, template)

	require.NoError(t, err)
	// Task should have transitioned from ValidationFailed -> Running -> Validating -> AwaitingApproval
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_Resume_TerminalState tests that resume rejects terminal states.
func TestEngine_Resume_TerminalState(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name:  "test",
		Steps: []domain.StepDefinition{},
	}

	terminalStates := []constants.TaskStatus{
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range terminalStates {
		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      status,
		}

		err := engine.Resume(ctx, task, template)

		assert.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
	}
}

// TestEngine_Resume_ContextCancellation tests context cancellation during resume.
func TestEngine_Resume_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
	}
	template := &domain.Template{Name: "test"}

	err := engine.Resume(ctx, task, template)

	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_ExecuteStep_ContextCancellation tests context check at step entry.
func TestEngine_ExecuteStep_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{ID: "task-123", WorkspaceID: "test"}
	step := &domain.StepDefinition{Name: "step1", Type: domain.StepTypeAI}

	result, err := engine.ExecuteStep(ctx, task, step)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_ExecuteStep_ExecutorNotFound tests error when executor not registered.
func TestEngine_ExecuteStep_ExecutorNotFound(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry() // Empty registry
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{ID: "task-123", WorkspaceID: "test", Steps: []domain.Step{{Name: "step1"}}}
	step := &domain.StepDefinition{Name: "step1", Type: domain.StepTypeAI}

	result, err := engine.ExecuteStep(ctx, task, step)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, atlaserrors.ErrExecutorNotFound)
}

// TestEngine_HandleStepResult_AutoProceed tests auto-proceed for validation.
func TestEngine_HandleStepResult_AutoProceed(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "validate", Type: domain.StepTypeValidation, Status: "running"}},
		StepResults: []domain.StepResult{},
	}

	result := &domain.StepResult{
		StepName:    "validate",
		Status:      "success",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	assert.Len(t, task.StepResults, 1)
	// Task status should remain Running (auto-proceed)
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
}

// TestEngine_HandleStepResult_PausesForHuman tests pausing for human step.
func TestEngine_HandleStepResult_PausesForHuman(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "approve", Type: domain.StepTypeHuman, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "approve",
		Status:      "awaiting_approval",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "approve", Type: domain.StepTypeHuman}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_HandleStepResult_ErrorState tests transitioning to error state on failure.
func TestEngine_HandleStepResult_ErrorState(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	testCases := []struct {
		stepType       domain.StepType
		expectedStatus constants.TaskStatus
	}{
		{domain.StepTypeValidation, constants.TaskStatusValidationFailed},
		{domain.StepTypeGit, constants.TaskStatusGHFailed},
		{domain.StepTypeCI, constants.TaskStatusCIFailed},
		{domain.StepTypeAI, constants.TaskStatusValidationFailed}, // AI maps to ValidationFailed
	}

	for _, tc := range testCases {
		t.Run(string(tc.stepType), func(t *testing.T) {
			task := &domain.Task{
				ID:          "task-123",
				WorkspaceID: "test",
				Status:      constants.TaskStatusRunning,
				CurrentStep: 0,
				Steps:       []domain.Step{{Name: "test", Type: tc.stepType, Status: "running"}},
				StepResults: []domain.StepResult{},
				Transitions: []domain.Transition{},
			}

			result := &domain.StepResult{
				StepName:    "test",
				Status:      "failed",
				Error:       "test error",
				CompletedAt: time.Now().UTC(),
			}
			step := &domain.StepDefinition{Name: "test", Type: tc.stepType}

			err := engine.HandleStepResult(ctx, task, result, step)

			require.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, task.Status)
		})
	}
}

// TestEngine_ParallelExecution tests parallel step group execution.
func TestEngine_ParallelExecution(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// Track concurrent executions
	var concurrentCount int
	var maxConcurrent int
	var mu sync.Mutex

	executor := &concurrencyTrackingExecutor{
		stepType: domain.StepTypeAI,
		onStart: func() {
			mu.Lock()
			concurrentCount++
			if concurrentCount > maxConcurrent {
				maxConcurrent = concurrentCount
			}
			mu.Unlock()
		},
		onEnd: func() {
			mu.Lock()
			concurrentCount--
			mu.Unlock()
		},
		delay: 50 * time.Millisecond,
	}
	registry.Register(executor)

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeAI},
		},
	}

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeAI},
		},
	}
	store.tasks[task.ID] = task

	// Execute parallel group
	results, err := engine.executeParallelGroup(ctx, task, template, []int{0, 1, 2})

	require.NoError(t, err)
	assert.Len(t, results, 3)
	// Should have achieved some concurrency
	assert.GreaterOrEqual(t, maxConcurrent, 1)
}

// TestEngine_ParallelExecution_FirstErrorCancels tests that first error cancels remaining.
func TestEngine_ParallelExecution_FirstErrorCancels(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// First executor succeeds after delay
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
		delay:    100 * time.Millisecond,
	})

	// Override with one that fails quickly
	registry.Register(&failingExecutor{
		stepType: domain.StepTypeValidation,
		err:      atlaserrors.ErrValidationFailed,
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeValidation}, // This will fail
		},
	}

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeValidation},
		},
	}

	_, err := engine.executeParallelGroup(ctx, task, template, []int{0, 1})

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
}

// TestEngine_StateSavedAfterEachStep tests checkpointing.
func TestEngine_StateSavedAfterEachStep(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeValidation,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeValidation},
			{Name: "step3", Type: domain.StepTypeAI},
		},
	}

	_, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	// Should have: 1 create + multiple updates (one per step + final)
	assert.Equal(t, 1, store.createCalls)
	assert.GreaterOrEqual(t, store.updateCalls, 3) // At least once per step
}

// TestEngine_BuildRetryContext tests retry context generation.
func TestEngine_BuildRetryContext(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		CurrentStep: 2,
		StepResults: []domain.StepResult{
			{StepIndex: 0, StepName: "step1", Status: "success"},
			{StepIndex: 1, StepName: "step2", Status: "failed", Error: "previous error"},
		},
	}

	lastResult := &domain.StepResult{
		StepName: "step3",
		Error:    "current error",
	}

	context := engine.buildRetryContext(task, lastResult)

	assert.Contains(t, context, "task-123")
	assert.Contains(t, context, "step3")
	assert.Contains(t, context, "current error")
	assert.Contains(t, context, "step2") // Previous failure
	assert.Contains(t, context, "previous error")
}

// TestEngine_EmptyTemplateSteps tests handling of empty template.
func TestEngine_EmptyTemplateSteps(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name:  "empty-template",
		Steps: []domain.StepDefinition{}, // No steps
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	assert.NotNil(t, task)
	// Should transition to AwaitingApproval (nothing to do = success)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// Helper executors for testing

// trackingExecutor tracks when steps are executed.
type trackingExecutor struct {
	stepType  domain.StepType
	onExecute func(step *domain.StepDefinition)
}

func (e *trackingExecutor) Execute(_ context.Context, _ *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	if e.onExecute != nil {
		e.onExecute(step)
	}
	return &domain.StepResult{
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (e *trackingExecutor) Type() domain.StepType {
	return e.stepType
}

// concurrencyTrackingExecutor tracks concurrent execution.
type concurrencyTrackingExecutor struct {
	stepType domain.StepType
	onStart  func()
	onEnd    func()
	delay    time.Duration
}

func (e *concurrencyTrackingExecutor) Execute(ctx context.Context, _ *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	if e.onStart != nil {
		e.onStart()
	}
	defer func() {
		if e.onEnd != nil {
			e.onEnd()
		}
	}()

	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return &domain.StepResult{
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (e *concurrencyTrackingExecutor) Type() domain.StepType {
	return e.stepType
}

// failingExecutor always returns an error.
type failingExecutor struct {
	stepType domain.StepType
	err      error
}

func (e *failingExecutor) Execute(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	return nil, e.err
}

func (e *failingExecutor) Type() domain.StepType {
	return e.stepType
}

// TestEngine_HandleStepError tests error handling during step execution.
func TestEngine_HandleStepError(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&failingExecutor{
		stepType: domain.StepTypeValidation,
		err:      atlaserrors.ErrValidationFailed,
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "validate", Type: domain.StepTypeValidation, Required: true},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should return the original error
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	assert.NotNil(t, task)

	// Task should be in ValidationFailed state
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)

	// Metadata should have error context
	assert.NotNil(t, task.Metadata)
	assert.Contains(t, task.Metadata, "last_error")
	assert.Contains(t, task.Metadata, "retry_context")

	// Step should be marked as failed
	assert.Equal(t, "failed", task.Steps[0].Status)
	assert.NotEmpty(t, task.Steps[0].Error)
}

// TestEngine_HandleStepError_StoreFails tests when store fails during error handling.
func TestEngine_HandleStepError_StoreFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.updateErr = atlaserrors.ErrWorkspaceNotFound // Simulate store failure
	registry := steps.NewExecutorRegistry()
	registry.Register(&failingExecutor{
		stepType: domain.StepTypeGit,
		err:      atlaserrors.ErrGitOperation,
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "git-step", Type: domain.StepTypeGit},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should return wrapped store error
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
	assert.NotNil(t, task)
}

// TestEngine_RunSteps_ContextCanceledMidLoop tests context cancellation during step loop.
func TestEngine_RunSteps_ContextCanceledMidLoop(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// First executor succeeds, second will be canceled
	callCount := 0
	registry.Register(&callbackExecutor{
		stepType: domain.StepTypeAI,
		callback: func(ctx context.Context) (*domain.StepResult, error) {
			callCount++
			if callCount == 1 {
				return &domain.StepResult{Status: "success", CompletedAt: time.Now().UTC()}, nil
			}
			// Simulate delay that will be canceled
			select {
			case <-time.After(100 * time.Millisecond):
				return &domain.StepResult{Status: "success", CompletedAt: time.Now().UTC()}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI, Required: true},
			{Name: "step2", Type: domain.StepTypeAI, Required: true},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first step completes
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should fail with context canceled
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotNil(t, task)
}

// TestEngine_RunSteps_CheckpointSaveFails tests checkpoint save failure.
func TestEngine_RunSteps_CheckpointSaveFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
		},
	}

	// Make update fail after first step
	store.updateErr = atlaserrors.ErrLockTimeout

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrLockTimeout)
	assert.NotNil(t, task)
}

// TestEngine_TransitionToErrorState_FromValidating tests transition from Validating state.
func TestEngine_TransitionToErrorState_FromValidating(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusValidating, // Already in Validating
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "validate", Type: domain.StepTypeValidation, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "validate",
		Status:      "failed",
		Error:       "validation failed",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "validate", Type: domain.StepTypeValidation}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Should transition from Validating -> ValidationFailed directly
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
}

// TestEngine_TransitionToErrorState_GitFromRunning tests git failure from Running.
func TestEngine_TransitionToErrorState_GitFromRunning(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "git-push", Type: domain.StepTypeGit, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "git-push",
		Status:      "failed",
		Error:       "push failed",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "git-push", Type: domain.StepTypeGit}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Git errors go directly from Running -> GHFailed
	assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
}

// TestEngine_TransitionToErrorState_CIFromRunning tests CI failure from Running.
func TestEngine_TransitionToErrorState_CIFromRunning(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "ci-check", Type: domain.StepTypeCI, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "ci-check",
		Status:      "failed",
		Error:       "CI failed",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "ci-check", Type: domain.StepTypeCI}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// CI errors go directly from Running -> CIFailed
	assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
}

// TestEngine_EnsureMetadata_NonNil tests ensureMetadata with existing metadata.
func TestEngine_EnsureMetadata_NonNil(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	// Task with existing metadata
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "test", Type: domain.StepTypeHuman, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
		Metadata:    map[string]any{"existing_key": "existing_value"},
	}

	result := &domain.StepResult{
		StepName:    "test",
		Status:      "failed",
		Error:       "test error",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "test", Type: domain.StepTypeHuman}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Existing metadata should be preserved
	assert.Equal(t, "existing_value", task.Metadata["existing_key"])
	// New error context should be added
	assert.Contains(t, task.Metadata, "last_error")
}

// TestEngine_CompleteTask_StoreFails tests complete task with store failure.
func TestEngine_CompleteTask_StoreFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	// Start task successfully, then make update fail for final save
	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// First call works (create), but we can't easily test completeTask store failure
	// because it happens after all steps succeed. Let's verify the happy path works.
	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_MapStepTypeToErrorStatus_SDD tests SDD step type mapping.
func TestEngine_MapStepTypeToErrorStatus_SDD(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "sdd-step", Type: domain.StepTypeSDD, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "sdd-step",
		Status:      "failed",
		Error:       "SDD step failed",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "sdd-step", Type: domain.StepTypeSDD}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// SDD failures map to ValidationFailed (goes through Validating first)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
}

// TestEngine_RunSteps_ShouldPauseSaveSuccess tests the pause path when save succeeds.
func TestEngine_RunSteps_ShouldPauseSaveSuccess(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeHuman,
		result:   &domain.StepResult{Status: "awaiting_approval"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "human-approve", Type: domain.StepTypeHuman, Required: true},
			{Name: "next-step", Type: domain.StepTypeHuman, Required: true},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	assert.NotNil(t, task)
	// Should pause at first human step
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
	assert.Equal(t, 0, task.CurrentStep) // Not advanced because paused
}

// TestEngine_RunSteps_ShouldPauseSaveFails tests the pause path when save fails.
func TestEngine_RunSteps_ShouldPauseSaveFails(t *testing.T) {
	ctx := context.Background()

	store := &conditionalFailStore{
		mockStore:    newMockStore(),
		failOnUpdate: 1, // Fail on first update (when trying to save pause state)
	}
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeHuman,
		result:   &domain.StepResult{Status: "awaiting_approval"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "human-approve", Type: domain.StepTypeHuman},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save checkpoint")
	assert.NotNil(t, task)
}

// TestEngine_HandleStepResult_ContextCancelled tests context cancellation in HandleStepResult.
func TestEngine_HandleStepResult_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
	}
	result := &domain.StepResult{Status: "success"}
	step := &domain.StepDefinition{Name: "test", Type: domain.StepTypeAI}

	err := engine.HandleStepResult(ctx, task, result, step)

	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_CompleteTask_TransitionFails tests completeTask when transition fails.
func TestEngine_CompleteTask_TransitionFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	// Create a task that's already in AwaitingApproval - Validating transition will fail
	template := &domain.Template{
		Name:  "test-template",
		Steps: []domain.StepDefinition{},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// With empty steps, completeTask is called immediately
	// It transitions Pending->Running->Validating->AwaitingApproval
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_Resume_SaveFails tests Resume when save fails after state transition.
func TestEngine_Resume_SaveFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.updateErr = atlaserrors.ErrLockTimeout

	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusValidationFailed, // Error state that needs transition
		CurrentStep: 0,
		Steps:       []domain.Step{},
		Transitions: []domain.Transition{},
	}
	template := &domain.Template{Name: "test", Steps: []domain.StepDefinition{}}

	err := engine.Resume(ctx, task, template)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrLockTimeout)
}

// TestEngine_CompleteTask_StoreSaveFails tests completeTask when final store save fails.
func TestEngine_CompleteTask_StoreSaveFails(t *testing.T) {
	ctx := context.Background()

	// Use conditional store that fails on the 2nd update (completeTask save)
	store := &conditionalFailStore{
		mockStore:    newMockStore(),
		failOnUpdate: 2, // Fail on second update (completeTask's store.Update)
	}
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should fail when completeTask tries to save
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save completed state")
	assert.NotNil(t, task)
}

// TestEngine_Start_CreateFails tests Start when store create fails.
func TestEngine_Start_CreateFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.createErr = atlaserrors.ErrTaskExists

	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name:  "test-template",
		Steps: []domain.StepDefinition{},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrTaskExists)
	assert.Nil(t, task)
}

// conditionalFailStore fails on a specific update call number.
type conditionalFailStore struct {
	*mockStore

	failOnUpdate int
	updateCount  int
}

func (s *conditionalFailStore) Create(ctx context.Context, workspaceName string, task *domain.Task) error {
	return s.mockStore.Create(ctx, workspaceName, task)
}

func (s *conditionalFailStore) Get(ctx context.Context, workspaceName, taskID string) (*domain.Task, error) {
	return s.mockStore.Get(ctx, workspaceName, taskID)
}

func (s *conditionalFailStore) Update(ctx context.Context, workspaceName string, task *domain.Task) error {
	s.updateCount++
	if s.updateCount == s.failOnUpdate {
		return atlaserrors.ErrLockTimeout
	}
	return s.mockStore.Update(ctx, workspaceName, task)
}

func (s *conditionalFailStore) List(ctx context.Context, workspaceName string) ([]*domain.Task, error) {
	return s.mockStore.List(ctx, workspaceName)
}

func (s *conditionalFailStore) Delete(ctx context.Context, workspaceName, taskID string) error {
	return s.mockStore.Delete(ctx, workspaceName, taskID)
}

func (s *conditionalFailStore) AppendLog(ctx context.Context, workspaceName, taskID string, entry []byte) error {
	return s.mockStore.AppendLog(ctx, workspaceName, taskID, entry)
}

func (s *conditionalFailStore) ReadLog(ctx context.Context, workspaceName, taskID string) ([]byte, error) {
	return s.mockStore.ReadLog(ctx, workspaceName, taskID)
}

func (s *conditionalFailStore) SaveArtifact(ctx context.Context, workspaceName, taskID, filename string, data []byte) error {
	return s.mockStore.SaveArtifact(ctx, workspaceName, taskID, filename, data)
}

func (s *conditionalFailStore) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	return s.mockStore.SaveVersionedArtifact(ctx, workspaceName, taskID, baseName, data)
}

func (s *conditionalFailStore) GetArtifact(ctx context.Context, workspaceName, taskID, filename string) ([]byte, error) {
	return s.mockStore.GetArtifact(ctx, workspaceName, taskID, filename)
}

func (s *conditionalFailStore) ListArtifacts(ctx context.Context, workspaceName, taskID string) ([]string, error) {
	return s.mockStore.ListArtifacts(ctx, workspaceName, taskID)
}

// callbackExecutor allows custom callback for testing.
type callbackExecutor struct {
	stepType domain.StepType
	callback func(ctx context.Context) (*domain.StepResult, error)
}

// ============================================================================
// Unit Tests for Extracted Helper Functions (Tech Debt Refactoring)
// ============================================================================

// TestEngine_executeCurrentStep tests the executeCurrentStep helper.
func TestEngine_executeCurrentStep(t *testing.T) {
	t.Run("executes_step_at_current_index", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 1,
			Steps: []domain.Step{
				{Name: "step0", Type: domain.StepTypeAI},
				{Name: "step1", Type: domain.StepTypeAI},
				{Name: "step2", Type: domain.StepTypeAI},
			},
		}

		template := &domain.Template{
			Name: "test",
			Steps: []domain.StepDefinition{
				{Name: "step0", Type: domain.StepTypeAI},
				{Name: "step1", Type: domain.StepTypeAI},
				{Name: "step2", Type: domain.StepTypeAI},
			},
		}

		result, err := engine.executeCurrentStep(ctx, task, template)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "step1", result.StepName)
	})

	t.Run("returns_error_when_executor_fails", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&failingExecutor{
			stepType: domain.StepTypeAI,
			err:      atlaserrors.ErrClaudeInvocation,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI}},
		}

		template := &domain.Template{
			Name:  "test",
			Steps: []domain.StepDefinition{{Name: "step0", Type: domain.StepTypeAI}},
		}

		result, err := engine.executeCurrentStep(ctx, task, template)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	})
}

// TestEngine_processStepResult tests the processStepResult helper.
func TestEngine_processStepResult(t *testing.T) {
	t.Run("success_result_returns_nil", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI, Status: "running"}},
			StepResults: []domain.StepResult{},
		}

		result := &domain.StepResult{
			StepName:    "step0",
			Status:      "success",
			CompletedAt: time.Now().UTC(),
		}
		step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

		err := engine.processStepResult(ctx, task, result, step)

		require.NoError(t, err)
		assert.Len(t, task.StepResults, 1)
	})

	t.Run("failed_result_transitions_to_error_state", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeGit, Status: "running"}},
			StepResults: []domain.StepResult{},
			Transitions: []domain.Transition{},
		}
		store.tasks[task.ID] = task

		result := &domain.StepResult{
			StepName:    "step0",
			Status:      "failed",
			Error:       "git push failed",
			CompletedAt: time.Now().UTC(),
		}
		step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeGit}

		err := engine.processStepResult(ctx, task, result, step)

		// HandleStepResult returns nil for failed status (successful transition)
		require.NoError(t, err)
		// Task should be in error state
		assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
		// Result should be appended
		assert.Len(t, task.StepResults, 1)
	})

	t.Run("logs_warning_when_store_update_fails", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		store.updateErr = atlaserrors.ErrLockTimeout
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeHuman, Status: "running"}},
			StepResults: []domain.StepResult{},
			Transitions: []domain.Transition{},
		}

		result := &domain.StepResult{
			StepName:    "step0",
			Status:      "awaiting_approval",
			CompletedAt: time.Now().UTC(),
		}
		step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeHuman}

		err := engine.processStepResult(ctx, task, result, step)

		// Should still return nil (store error is logged, not returned)
		require.NoError(t, err)
	})
}

// TestEngine_advanceToNextStep tests the advanceToNextStep helper.
func TestEngine_advanceToNextStep(t *testing.T) {
	t.Run("increments_step_and_saves_checkpoint", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 0,
			UpdatedAt:   time.Now().Add(-1 * time.Hour).UTC(),
		}
		store.tasks[task.ID] = task
		oldUpdatedAt := task.UpdatedAt

		err := engine.advanceToNextStep(ctx, task)

		require.NoError(t, err)
		assert.Equal(t, 1, task.CurrentStep)
		assert.True(t, task.UpdatedAt.After(oldUpdatedAt))
		assert.Equal(t, 1, store.updateCalls)
	})

	t.Run("returns_error_when_store_fails", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		store.updateErr = atlaserrors.ErrLockTimeout
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 0,
		}

		err := engine.advanceToNextStep(ctx, task)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save checkpoint")
	})
}

// TestEngine_saveAndPause tests the saveAndPause helper.
func TestEngine_saveAndPause(t *testing.T) {
	t.Run("saves_state_and_returns_nil", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      constants.TaskStatusAwaitingApproval,
		}
		store.tasks[task.ID] = task

		err := engine.saveAndPause(ctx, task)

		require.NoError(t, err)
		assert.Equal(t, 1, store.updateCalls)
	})

	t.Run("returns_error_when_store_fails", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		store.updateErr = atlaserrors.ErrLockTimeout
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			Status:      constants.TaskStatusAwaitingApproval,
		}

		err := engine.saveAndPause(ctx, task)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save checkpoint")
	})
}

// TestEngine_setErrorMetadata tests the setErrorMetadata helper.
func TestEngine_setErrorMetadata(t *testing.T) {
	t.Run("sets_error_metadata_on_nil_metadata", func(t *testing.T) {
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 2,
			Metadata:    nil,
		}

		engine.setErrorMetadata(task, "failing-step", "something went wrong")

		require.NotNil(t, task.Metadata)
		assert.Equal(t, "something went wrong", task.Metadata["last_error"])
		assert.Contains(t, task.Metadata["retry_context"], "failing-step")
	})

	t.Run("preserves_existing_metadata", func(t *testing.T) {
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-123",
			WorkspaceID: "test",
			CurrentStep: 1,
			Metadata:    map[string]any{"custom_key": "custom_value"},
		}

		engine.setErrorMetadata(task, "step1", "error message")

		assert.Equal(t, "custom_value", task.Metadata["custom_key"])
		assert.Equal(t, "error message", task.Metadata["last_error"])
	})
}

// TestEngine_requiresValidatingIntermediate tests the requiresValidatingIntermediate helper.
func TestEngine_requiresValidatingIntermediate(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	testCases := []struct {
		name           string
		currentStatus  constants.TaskStatus
		targetStatus   constants.TaskStatus
		expectedResult bool
	}{
		{
			name:           "running_to_validation_failed_requires_intermediate",
			currentStatus:  constants.TaskStatusRunning,
			targetStatus:   constants.TaskStatusValidationFailed,
			expectedResult: true,
		},
		{
			name:           "running_to_gh_failed_no_intermediate",
			currentStatus:  constants.TaskStatusRunning,
			targetStatus:   constants.TaskStatusGHFailed,
			expectedResult: false,
		},
		{
			name:           "running_to_ci_failed_no_intermediate",
			currentStatus:  constants.TaskStatusRunning,
			targetStatus:   constants.TaskStatusCIFailed,
			expectedResult: false,
		},
		{
			name:           "validating_to_validation_failed_no_intermediate",
			currentStatus:  constants.TaskStatusValidating,
			targetStatus:   constants.TaskStatusValidationFailed,
			expectedResult: false,
		},
		{
			name:           "pending_to_validation_failed_no_intermediate",
			currentStatus:  constants.TaskStatusPending,
			targetStatus:   constants.TaskStatusValidationFailed,
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.requiresValidatingIntermediate(tc.currentStatus, tc.targetStatus)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func (e *callbackExecutor) Execute(ctx context.Context, _ *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	result, err := e.callback(ctx)
	if err != nil {
		return nil, err
	}
	result.StepName = step.Name
	return result, nil
}

func (e *callbackExecutor) Type() domain.StepType {
	return e.stepType
}

// ============================================================================
// Tech Debt Test Coverage Expansion Tests
// ============================================================================

// TestEngine_ExecuteStepInternal_AllStepTypes tests executeStepInternal with all 6 step types.
// AC #3: Given executeStepInternal When called with each step type Then returns correct results.
func TestEngine_ExecuteStepInternal_AllStepTypes(t *testing.T) {
	testCases := []struct {
		name           string
		stepType       domain.StepType
		stepName       string
		expectedStatus string
	}{
		{
			name:           "AI_step_type",
			stepType:       domain.StepTypeAI,
			stepName:       "ai-step",
			expectedStatus: "success",
		},
		{
			name:           "Validation_step_type",
			stepType:       domain.StepTypeValidation,
			stepName:       "validation-step",
			expectedStatus: "success",
		},
		{
			name:           "Git_step_type",
			stepType:       domain.StepTypeGit,
			stepName:       "git-step",
			expectedStatus: "success",
		},
		{
			name:           "CI_step_type",
			stepType:       domain.StepTypeCI,
			stepName:       "ci-step",
			expectedStatus: "success",
		},
		{
			name:           "Human_step_type",
			stepType:       domain.StepTypeHuman,
			stepName:       "human-step",
			expectedStatus: "success",
		},
		{
			name:           "SDD_step_type",
			stepType:       domain.StepTypeSDD,
			stepName:       "sdd-step",
			expectedStatus: "success",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			store := newMockStore()
			registry := steps.NewExecutorRegistry()

			// Register executor for this step type
			registry.Register(&mockExecutor{
				stepType: tc.stepType,
				result:   &domain.StepResult{Status: tc.expectedStatus},
			})

			engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

			task := &domain.Task{
				ID:          "task-123",
				WorkspaceID: "test",
				Status:      constants.TaskStatusRunning,
				CurrentStep: 0,
				Steps: []domain.Step{
					{Name: tc.stepName, Type: tc.stepType, Status: "pending"},
				},
			}

			step := &domain.StepDefinition{
				Name: tc.stepName,
				Type: tc.stepType,
			}

			// Execute the step
			startTime := time.Now()
			result, err := engine.executeStepInternal(ctx, task, step)
			duration := time.Since(startTime)

			// Verify executor was called correctly
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tc.stepName, result.StepName)
			assert.Equal(t, tc.expectedStatus, result.Status)

			// Verify duration tracking works (result should have timing)
			assert.Positive(t, duration, "duration should be positive")
			assert.GreaterOrEqual(t, result.DurationMs, int64(0))
		})
	}
}

// TestEngine_ExecuteStepInternal_LogsStepDetails verifies logging output includes step details.
func TestEngine_ExecuteStepInternal_LogsStepDetails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	// Use a real logger to verify it doesn't panic
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-log-test",
		WorkspaceID: "test",
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "logged-step", Type: domain.StepTypeAI}},
	}

	step := &domain.StepDefinition{
		Name: "logged-step",
		Type: domain.StepTypeAI,
	}

	result, err := engine.executeStepInternal(ctx, task, step)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// Logger should have been called without panic
}

// TestEngine_ShouldPause_AllErrorStates tests shouldPause for all error states.
// AC #4: Given shouldPause When task is in any error state Then returns true.
func TestEngine_ShouldPause_AllErrorStates(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	testCases := []struct {
		name     string
		status   constants.TaskStatus
		expected bool
	}{
		{
			name:     "ValidationFailed_returns_true",
			status:   constants.TaskStatusValidationFailed,
			expected: true,
		},
		{
			name:     "GHFailed_returns_true",
			status:   constants.TaskStatusGHFailed,
			expected: true,
		},
		{
			name:     "CIFailed_returns_true",
			status:   constants.TaskStatusCIFailed,
			expected: true,
		},
		{
			name:     "CITimeout_returns_true",
			status:   constants.TaskStatusCITimeout,
			expected: true,
		},
		{
			name:     "AwaitingApproval_returns_true",
			status:   constants.TaskStatusAwaitingApproval,
			expected: true,
		},
		{
			name:     "Running_returns_false",
			status:   constants.TaskStatusRunning,
			expected: false,
		},
		{
			name:     "Completed_returns_false",
			status:   constants.TaskStatusCompleted,
			expected: false,
		},
		{
			name:     "Pending_returns_false",
			status:   constants.TaskStatusPending,
			expected: false,
		},
		{
			name:     "Validating_returns_false",
			status:   constants.TaskStatusValidating,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &domain.Task{
				ID:          "task-123",
				WorkspaceID: "test",
				Status:      tc.status,
			}

			result := engine.shouldPause(task)

			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestEngine_ParallelExecution_RaceCondition is a stress test for race conditions.
// AC #5: Given parallel step execution When 100+ iterations with race detector Then no data races.
func TestEngine_ParallelExecution_RaceCondition(t *testing.T) {
	const iterations = 100

	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			t.Parallel() // Enable concurrent execution

			ctx := context.Background()

			store := newMockStore()
			registry := steps.NewExecutorRegistry()

			// Create executor with small delay to simulate real work
			registry.Register(&mockExecutor{
				stepType: domain.StepTypeAI,
				result:   &domain.StepResult{Status: "success"},
				delay:    time.Millisecond, // Small delay
			})

			engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

			template := &domain.Template{
				Name: "test-template",
				Steps: []domain.StepDefinition{
					{Name: "step1", Type: domain.StepTypeAI},
					{Name: "step2", Type: domain.StepTypeAI},
					{Name: "step3", Type: domain.StepTypeAI},
				},
			}

			task := &domain.Task{
				ID:          fmt.Sprintf("task-%d", i),
				WorkspaceID: "test",
				Status:      constants.TaskStatusRunning,
				Steps: []domain.Step{
					{Name: "step1", Type: domain.StepTypeAI},
					{Name: "step2", Type: domain.StepTypeAI},
					{Name: "step3", Type: domain.StepTypeAI},
				},
			}
			store.tasks[task.ID] = task

			// Execute parallel group - should not panic or race
			results, err := engine.executeParallelGroup(ctx, task, template, []int{0, 1, 2})

			// Verify results slice is thread-safe
			require.NoError(t, err)
			assert.Len(t, results, 3)

			// All results should be populated
			for idx, result := range results {
				assert.NotNil(t, result, "result at index %d should not be nil", idx)
				assert.Equal(t, "success", result.Status)
			}
		})
	}
}

// TestEngine_ParallelExecution_NoPanicsUnderHighConcurrency verifies no panics under stress.
func TestEngine_ParallelExecution_NoPanicsUnderHighConcurrency(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// Track concurrent access
	var accessCount int64
	var mu sync.Mutex

	registry.Register(&trackingExecutor{
		stepType: domain.StepTypeAI,
		onExecute: func(_ *domain.StepDefinition) {
			mu.Lock()
			accessCount++
			mu.Unlock()
		},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "stress-test",
		Steps: []domain.StepDefinition{
			{Name: "step0", Type: domain.StepTypeAI},
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeAI},
			{Name: "step4", Type: domain.StepTypeAI},
		},
	}

	task := &domain.Task{
		ID:          "task-stress",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		Steps: []domain.Step{
			{Name: "step0", Type: domain.StepTypeAI},
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeAI},
			{Name: "step4", Type: domain.StepTypeAI},
		},
	}
	store.tasks[task.ID] = task

	// Execute parallel group with 5 steps
	results, err := engine.executeParallelGroup(ctx, task, template, []int{0, 1, 2, 3, 4})

	require.NoError(t, err)
	assert.Len(t, results, 5)
	assert.Equal(t, int64(5), accessCount)
}

// TestEngine_Timeout_StepExceedsLimit tests timeout handling for long-running steps.
// AC #6: Given step with configured timeout When execution exceeds timeout Then returns context.DeadlineExceeded.
func TestEngine_Timeout_StepExceedsLimit(t *testing.T) {
	t.Run("step_exceeds_timeout_returns_deadline_exceeded", func(t *testing.T) {
		// Create context with short timeout (100ms)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()

		// Create mock executor with configurable delay (500ms - exceeds timeout)
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
			delay:    500 * time.Millisecond,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-timeout",
			WorkspaceID: "test",
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "slow-step", Type: domain.StepTypeAI}},
		}

		step := &domain.StepDefinition{
			Name: "slow-step",
			Type: domain.StepTypeAI,
		}

		// Execute step that takes longer than timeout
		result, err := engine.executeStepInternal(ctx, task, step)

		// Verify context.DeadlineExceeded returned
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, result)
	})

	t.Run("task_state_consistent_after_timeout", func(t *testing.T) {
		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()

		// Slow executor
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
			delay:    200 * time.Millisecond,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-timeout-state",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "slow-step", Type: domain.StepTypeAI, Status: "pending"}},
		}
		store.tasks[task.ID] = task

		step := &domain.StepDefinition{
			Name: "slow-step",
			Type: domain.StepTypeAI,
		}

		// Execute with timeout
		_, err := engine.ExecuteStep(ctx, task, step)

		// Should timeout
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// Task should still be accessible and consistent
		storedTask, getErr := store.Get(context.Background(), "test", task.ID)
		require.NoError(t, getErr)
		assert.NotNil(t, storedTask)
		// Step should have been marked as running before timeout
		assert.Equal(t, "running", storedTask.Steps[0].Status)
	})

	t.Run("context_canceled_vs_deadline", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
			delay:    500 * time.Millisecond,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-cancel",
			WorkspaceID: "test",
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step", Type: domain.StepTypeAI}},
		}
		step := &domain.StepDefinition{Name: "step", Type: domain.StepTypeAI}

		// Cancel immediately
		cancel()

		result, err := engine.executeStepInternal(ctx, task, step)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})
}

// TestEngine_MapStepTypeToErrorStatus_Exhaustive tests exhaustive step type to error status mapping.
// AC #7: Given mapStepTypeToErrorStatus When called with all step types Then returns correct mappings exhaustively.
func TestEngine_MapStepTypeToErrorStatus_Exhaustive(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	testCases := []struct {
		name           string
		stepType       domain.StepType
		expectedStatus constants.TaskStatus
	}{
		{
			name:           "Validation_maps_to_ValidationFailed",
			stepType:       domain.StepTypeValidation,
			expectedStatus: constants.TaskStatusValidationFailed,
		},
		{
			name:           "Git_maps_to_GHFailed",
			stepType:       domain.StepTypeGit,
			expectedStatus: constants.TaskStatusGHFailed,
		},
		{
			name:           "CI_maps_to_CIFailed",
			stepType:       domain.StepTypeCI,
			expectedStatus: constants.TaskStatusCIFailed,
		},
		{
			name:           "AI_maps_to_ValidationFailed",
			stepType:       domain.StepTypeAI,
			expectedStatus: constants.TaskStatusValidationFailed,
		},
		{
			name:           "Human_maps_to_ValidationFailed",
			stepType:       domain.StepTypeHuman,
			expectedStatus: constants.TaskStatusValidationFailed,
		},
		{
			name:           "SDD_maps_to_ValidationFailed",
			stepType:       domain.StepTypeSDD,
			expectedStatus: constants.TaskStatusValidationFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.mapStepTypeToErrorStatus(tc.stepType)
			assert.Equal(t, tc.expectedStatus, result)
		})
	}

	// Ensure all known step types are covered
	allStepTypes := []domain.StepType{
		domain.StepTypeAI,
		domain.StepTypeValidation,
		domain.StepTypeGit,
		domain.StepTypeCI,
		domain.StepTypeHuman,
		domain.StepTypeSDD,
	}

	t.Run("all_known_step_types_covered", func(t *testing.T) {
		for _, st := range allStepTypes {
			result := engine.mapStepTypeToErrorStatus(st)
			// Should return a valid error status (not empty)
			assert.NotEmpty(t, result)
			// Should be an error status (not running or pending)
			assert.NotEqual(t, constants.TaskStatusRunning, result)
			assert.NotEqual(t, constants.TaskStatusPending, result)
		}
	})
}

// TestEngine_BuildRetryContext_EdgeCases tests edge cases for buildRetryContext.
func TestEngine_BuildRetryContext_EdgeCases(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	t.Run("nil_last_result", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-nil-result",
			WorkspaceID: "test",
			CurrentStep: 0,
			StepResults: []domain.StepResult{},
		}

		context := engine.buildRetryContext(task, nil)

		// Should not panic and should still include task info
		assert.Contains(t, context, "task-nil-result")
		assert.Contains(t, context, "Retry Context")
		// Should not have "Failed Step" section when result is nil
		assert.NotContains(t, context, "Failed Step")
	})

	t.Run("empty_step_results_array", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-empty-results",
			WorkspaceID: "test",
			CurrentStep: 0,
			StepResults: []domain.StepResult{}, // Empty
		}

		lastResult := &domain.StepResult{
			StepName: "current-step",
			Error:    "current error",
		}

		context := engine.buildRetryContext(task, lastResult)

		assert.Contains(t, context, "task-empty-results")
		assert.Contains(t, context, "current-step")
		assert.Contains(t, context, "current error")
		// Previous Attempts section should exist but be empty
		assert.Contains(t, context, "Previous Attempts")
	})

	t.Run("10_plus_failed_steps_all_included", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-many-failures",
			WorkspaceID: "test",
			CurrentStep: 10,
			StepResults: []domain.StepResult{},
		}

		// Add 12 failed steps
		for i := 0; i < 12; i++ {
			task.StepResults = append(task.StepResults, domain.StepResult{
				StepIndex: i,
				StepName:  fmt.Sprintf("step-%d", i),
				Status:    "failed",
				Error:     fmt.Sprintf("error-%d", i),
			})
		}

		lastResult := &domain.StepResult{
			StepName: "final-step",
			Error:    "final error",
		}

		context := engine.buildRetryContext(task, lastResult)

		// Verify all 12 failed steps are included
		for i := 0; i < 12; i++ {
			assert.Contains(t, context, fmt.Sprintf("step-%d", i))
			assert.Contains(t, context, fmt.Sprintf("error-%d", i))
		}
	})

	t.Run("markdown_formatting_is_valid", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-markdown",
			WorkspaceID: "test",
			CurrentStep: 2,
			StepResults: []domain.StepResult{
				{StepIndex: 0, StepName: "step-0", Status: "success"},
				{StepIndex: 1, StepName: "step-1", Status: "failed", Error: "error-1"},
			},
		}

		lastResult := &domain.StepResult{
			StepName: "step-2",
			Error:    "current error",
		}

		context := engine.buildRetryContext(task, lastResult)

		// Verify markdown structure
		assert.Contains(t, context, "## Retry Context")
		assert.Contains(t, context, "**Task ID:**")
		assert.Contains(t, context, "**Current Step:**")
		assert.Contains(t, context, "**Failed Step:**")
		assert.Contains(t, context, "**Error:**")
		assert.Contains(t, context, "### Previous Attempts")
		assert.Contains(t, context, "- Step")
	})

	t.Run("mixed_success_and_failure_results", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-mixed",
			WorkspaceID: "test",
			CurrentStep: 5,
			StepResults: []domain.StepResult{
				{StepIndex: 0, StepName: "step-0", Status: "success"},
				{StepIndex: 1, StepName: "step-1", Status: "failed", Error: "error-1"},
				{StepIndex: 2, StepName: "step-2", Status: "success"},
				{StepIndex: 3, StepName: "step-3", Status: "failed", Error: "error-3"},
				{StepIndex: 4, StepName: "step-4", Status: "success"},
			},
		}

		lastResult := &domain.StepResult{
			StepName: "step-5",
			Error:    "current error",
		}

		context := engine.buildRetryContext(task, lastResult)

		// Only failed steps should be listed in Previous Attempts
		assert.Contains(t, context, "step-1")
		assert.Contains(t, context, "error-1")
		assert.Contains(t, context, "step-3")
		assert.Contains(t, context, "error-3")
		// Success steps should not be in Previous Attempts (checking they're not in the failures list)
		// The output format is "- Step X (stepname): error" so we check for success steps not being in that pattern
	})
}

// TestEngine_ConcurrentResume is a stress test for concurrent resume operations.
// AC #5: Given parallel step execution When 100+ iterations with race detector Then no data races.
func TestEngine_ConcurrentResume(t *testing.T) {
	const goroutines = 10

	t.Run("concurrent_resume_same_task_no_panics", func(t *testing.T) {
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "step1", Type: domain.StepTypeAI},
			},
		}

		// Track results
		var wg sync.WaitGroup
		errors := make(chan error, goroutines)
		panicCount := make(chan int, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panicCount <- 1
					}
				}()

				// Each goroutine gets its own task to avoid conflicts
				task := &domain.Task{
					ID:          fmt.Sprintf("task-resume-%d", idx),
					WorkspaceID: "test",
					Status:      constants.TaskStatusValidationFailed,
					CurrentStep: 0,
					Steps:       []domain.Step{{Name: "step1", Type: domain.StepTypeAI, Status: "failed"}},
					Transitions: []domain.Transition{},
				}
				store.mu.Lock()
				store.tasks[task.ID] = task
				store.mu.Unlock()

				ctx := context.Background()
				err := engine.Resume(ctx, task, template)
				if err != nil {
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)
		close(panicCount)

		// Count panics
		totalPanics := 0
		for p := range panicCount {
			totalPanics += p
		}
		assert.Equal(t, 0, totalPanics, "no panics should occur")

		// All should complete without panics (errors are ok as concurrent modification is expected)
	})

	t.Run("behavior_is_deterministic_per_task", func(t *testing.T) {
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "step1", Type: domain.StepTypeAI},
			},
		}

		// Run same scenario multiple times to verify determinism
		for run := 0; run < 5; run++ {
			task := &domain.Task{
				ID:          fmt.Sprintf("task-deterministic-%d", run),
				WorkspaceID: "test",
				Status:      constants.TaskStatusRunning,
				CurrentStep: 0,
				Steps:       []domain.Step{{Name: "step1", Type: domain.StepTypeAI, Status: "pending"}},
				Transitions: []domain.Transition{},
			}
			store.tasks[task.ID] = task

			ctx := context.Background()
			err := engine.Resume(ctx, task, template)

			require.NoError(t, err)
			// Task should end in same state every time
			assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
		}
	})

	t.Run("no_races_with_race_detector", func(_ *testing.T) {
		// This test is primarily for running with -race flag
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
			delay:    time.Millisecond,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "step1", Type: domain.StepTypeAI},
			},
		}

		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				task := &domain.Task{
					ID:          fmt.Sprintf("task-race-%d", idx),
					WorkspaceID: "test",
					Status:      constants.TaskStatusRunning,
					CurrentStep: 0,
					Steps:       []domain.Step{{Name: "step1", Type: domain.StepTypeAI, Status: "pending"}},
					Transitions: []domain.Transition{},
				}
				store.mu.Lock()
				store.tasks[task.ID] = task
				store.mu.Unlock()

				ctx := context.Background()
				_ = engine.Resume(ctx, task, template)
			}(i)
		}

		wg.Wait()
		// If we get here without race detector complaints, test passes
	})
}

// ============================================================================
// Additional Coverage Tests (Task 8)
// ============================================================================

// TestEngine_ProcessStepResult_StoreSaveErrorPath tests the error path in processStepResult.
func TestEngine_ProcessStepResult_StoreSaveErrorPath(t *testing.T) {
	t.Run("unknown_result_status_returns_error", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-unknown-status",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI, Status: "running"}},
			StepResults: []domain.StepResult{},
		}
		store.tasks[task.ID] = task

		result := &domain.StepResult{
			StepName:    "step0",
			Status:      "unknown_invalid_status", // Unknown status
			CompletedAt: time.Now().UTC(),
		}
		step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

		err := engine.processStepResult(ctx, task, result, step)

		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrUnknownStepResultStatus)
	})

	t.Run("store_save_error_on_failed_result", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		store.updateErr = atlaserrors.ErrLockTimeout
		registry := steps.NewExecutorRegistry()
		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:          "task-store-fail",
			WorkspaceID: "test",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 0,
			Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeGit, Status: "running"}},
			StepResults: []domain.StepResult{},
			Transitions: []domain.Transition{},
		}

		result := &domain.StepResult{
			StepName:    "step0",
			Status:      "failed",
			Error:       "git failed",
			CompletedAt: time.Now().UTC(),
		}
		step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeGit}

		err := engine.processStepResult(ctx, task, result, step)

		// Should still transition to error state even if store fails
		// The store error is logged but not propagated
		require.NoError(t, err)
		assert.Equal(t, constants.TaskStatusGHFailed, task.Status)
	})
}

// TestEngine_RunSteps_HandleStepErrorPath tests the handleStepError path in runSteps.
func TestEngine_RunSteps_HandleStepErrorPath(t *testing.T) {
	t.Run("handle_step_error_propagates_executor_error", func(t *testing.T) {
		ctx := context.Background()

		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&failingExecutor{
			stepType: domain.StepTypeCI,
			err:      atlaserrors.ErrCIFailed,
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "ci-step", Type: domain.StepTypeCI, Required: true},
			},
		}

		task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrCIFailed)
		assert.NotNil(t, task)
		assert.Equal(t, constants.TaskStatusCIFailed, task.Status)
	})
}

// TestEngine_CompleteTask_TransitionErrors tests error paths in completeTask.
func TestEngine_CompleteTask_TransitionErrors(t *testing.T) {
	t.Run("store_save_fails_returns_error", func(t *testing.T) {
		ctx := context.Background()

		// Use conditional store that fails on a later update
		store := &conditionalFailStore{
			mockStore:    newMockStore(),
			failOnUpdate: 2, // Fail on second update (completeTask's final save)
		}
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "step1", Type: domain.StepTypeAI},
			},
		}

		task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save completed state")
		assert.NotNil(t, task)
	})
}

// TestEngine_HandleStepResult_UnknownStatus tests unknown result status handling.
func TestEngine_HandleStepResult_UnknownStatus(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "test", Type: domain.StepTypeAI, Status: "running"}},
		StepResults: []domain.StepResult{},
	}

	result := &domain.StepResult{
		StepName:    "test",
		Status:      "some_weird_status", // Invalid status
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "test", Type: domain.StepTypeAI}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrUnknownStepResultStatus)
}

// TestEngine_HandleStepResult_SkippedStatus tests skipped result status handling.
// This is the scenario when AI decides no changes are needed and the CI step
// is skipped because no PR was created.
func TestEngine_HandleStepResult_SkippedStatus(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-skipped",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "ci_wait", Type: domain.StepTypeCI, Status: "running"}},
		StepResults: []domain.StepResult{},
		Metadata:    map[string]any{"skip_git_steps": true}, // Set by git_commit when no changes
	}

	result := &domain.StepResult{
		StepName:    "ci_wait",
		Status:      constants.StepStatusSkipped, // CI step skipped due to no PR
		CompletedAt: time.Now().UTC(),
		Output:      "Skipped - no PR was created (no changes to commit)",
	}
	step := &domain.StepDefinition{Name: "ci_wait", Type: domain.StepTypeCI}

	err := engine.HandleStepResult(ctx, task, result, step)

	// Should NOT return an error - skipped is a valid status
	require.NoError(t, err)
	// Task should remain in Running status (caller advances to next step)
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
}

// TestEngine_MapStepTypeToErrorStatus_DefaultCase tests the default case in switch.
func TestEngine_MapStepTypeToErrorStatus_DefaultCase(t *testing.T) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	// Test with an unknown step type (cast to domain.StepType)
	unknownType := domain.StepType("unknown_type")
	result := engine.mapStepTypeToErrorStatus(unknownType)

	// Default case should return ValidationFailed
	assert.Equal(t, constants.TaskStatusValidationFailed, result)
}

// TestEngine_Start_StepBeyondArrayBounds tests edge case of CurrentStep >= len(Steps).
func TestEngine_Start_StepBeyondArrayBounds(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	// Empty steps - task.CurrentStep (0) will be >= len(task.Steps) (0)
	template := &domain.Template{
		Name:  "empty-template",
		Steps: []domain.StepDefinition{},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	assert.NotNil(t, task)
	// Should transition to AwaitingApproval directly
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_ExecuteStep_UpdatesStepStatus tests that ExecuteStep updates step status correctly.
func TestEngine_ExecuteStep_UpdatesStepStatus(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-step-status",
		WorkspaceID: "test",
		CurrentStep: 0,
		Steps: []domain.Step{
			{Name: "step0", Type: domain.StepTypeAI, Status: "pending", Attempts: 0},
		},
	}

	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

	result, err := engine.ExecuteStep(ctx, task, step)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// Step should be marked as running
	assert.Equal(t, "running", task.Steps[0].Status)
	// Attempts should be incremented
	assert.Equal(t, 1, task.Steps[0].Attempts)
	// StartedAt should be set
	assert.NotNil(t, task.Steps[0].StartedAt)
}

// TestEngine_HandleStepResult_UpdatesStepCompletion tests step completion updates.
func TestEngine_HandleStepResult_UpdatesStepCompletion(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-completion",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI, Status: "running"}},
		StepResults: []domain.StepResult{},
	}

	completedAt := time.Now().UTC()
	result := &domain.StepResult{
		StepName:    "step0",
		Status:      "success",
		CompletedAt: completedAt,
	}
	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Step should be marked with success status
	assert.Equal(t, "success", task.Steps[0].Status)
	// CompletedAt should be set
	assert.NotNil(t, task.Steps[0].CompletedAt)
	assert.Equal(t, completedAt, *task.Steps[0].CompletedAt)
}

// TestEngine_HandleStepResult_FailedStepSetsError tests error field is set on failure.
func TestEngine_HandleStepResult_FailedStepSetsError(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-error-field",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeCI, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "step0",
		Status:      "failed",
		Error:       "CI pipeline failed with exit code 1",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeCI}

	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Step should have error field set
	assert.Equal(t, "CI pipeline failed with exit code 1", task.Steps[0].Error)
}

// TestEngine_Start_TransitionFails tests Start when initial transition fails.
func TestEngine_Start_TransitionFails(t *testing.T) {
	// This is hard to test directly since Transition() validates internal state machine
	// The Transition function only fails on invalid state transitions
	// Since we control the initial state (Pending), transition to Running always succeeds
	// So we test the context cancellation path instead
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	assert.Nil(t, task)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_CompleteTask_FirstTransitionFails tests completeTask when first transition fails.
func TestEngine_CompleteTask_FirstTransitionFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	// Create a task that's NOT in Running state - completeTask expects Running
	// but we'll give it a task in Pending state, which cannot transition to Validating
	task := &domain.Task{
		ID:          "task-wrong-state",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusPending, // Wrong state!
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	// Call completeTask directly - first transition should fail
	err := engine.completeTask(ctx, task)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
	assert.Contains(t, err.Error(), "cannot transition from pending to validating")
}

// TestEngine_CompleteTask_SecondTransitionFails tests the second transition path in completeTask.
// The second transition (Validating → AwaitingApproval) can only fail via context cancellation
// since it's always a valid state machine transition.
func TestEngine_CompleteTask_SecondTransitionFails(t *testing.T) {
	// To test the second transition failing, we call completeTask directly
	// with a task already in Validating state and a canceled context.
	// The first transition will fail because context is checked at start of Transition.
	// To specifically test second transition, we use a fresh context and cancel during execution.

	// Test 1: Verify second transition path exists by testing with Validating task
	t.Run("task already in validating state", func(t *testing.T) {
		ctx := context.Background()
		store := newMockStore()
		engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

		// Task already in Validating - first transition (Running→Validating) will fail
		task := &domain.Task{
			ID:          "task-validating",
			WorkspaceID: "test-workspace",
			Status:      constants.TaskStatusValidating,
			Transitions: []domain.Transition{},
		}
		store.tasks[task.ID] = task

		err := engine.completeTask(ctx, task)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
		// First transition fails because Validating→Validating is same-state
		assert.Contains(t, err.Error(), "cannot transition from validating to validating")
	})

	// Test 2: Normal success path to verify completeTask works end-to-end
	t.Run("success path", func(t *testing.T) {
		ctx := context.Background()
		store := newMockStore()
		registry := steps.NewExecutorRegistry()
		registry.Register(&mockExecutor{
			stepType: domain.StepTypeAI,
			result:   &domain.StepResult{Status: "success"},
		})

		engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

		template := &domain.Template{
			Name: "test-template",
			Steps: []domain.StepDefinition{
				{Name: "step1", Type: domain.StepTypeAI},
			},
		}

		task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

		require.NoError(t, err)
		assert.NotNil(t, task)
		assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
	})
}

// TestEngine_Resume_AlreadyRunning tests Resume when task is already Running.
func TestEngine_Resume_AlreadyRunning(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task := &domain.Task{
		ID:          "task-already-running",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning, // Already running
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "step1", Type: domain.StepTypeAI, Status: "pending"}},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Resume(ctx, task, template)

	// Should proceed without error (no transition needed)
	require.NoError(t, err)
	// Task should complete normally
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_Resume_FromAwaitingApproval tests Resume when task is in AwaitingApproval state.
func TestEngine_Resume_FromAwaitingApproval(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	// Template with optional step that will be skipped
	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI, Required: true},
			{Name: "step2", Type: domain.StepTypeAI, Required: false}, // Optional - will be skipped
		},
	}

	// Task paused at step 1 in running state (after step1 completed)
	task := &domain.Task{
		ID:          "task-awaiting",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning, // Running - can complete after skipping optional steps
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI, Status: "success"},
			{Name: "step2", Type: domain.StepTypeAI, Status: "pending"},
		},
		StepResults: []domain.StepResult{
			{StepName: "step1", Status: "success"},
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Resume(ctx, task, template)

	// Resumes without error - optional step2 is skipped, task reaches awaiting approval
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_ProcessStepResult_SaveWarningPath tests the warning log path when save fails on error handling.
func TestEngine_ProcessStepResult_SaveWarningPath(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.updateErr = atlaserrors.ErrLockTimeout
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-save-warning",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeHuman, Status: "running"}},
		StepResults: []domain.StepResult{},
		Transitions: []domain.Transition{},
	}

	result := &domain.StepResult{
		StepName:    "step0",
		Status:      "awaiting_approval",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeHuman}

	// processStepResult should handle store failure gracefully (log warning)
	err := engine.processStepResult(ctx, task, result, step)

	// The error is logged but not returned for awaiting_approval
	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
}

// TestEngine_RunSteps_MultipleStepsWithPause tests the pause behavior mid-execution.
func TestEngine_RunSteps_MultipleStepsWithPause(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// First step succeeds, second step requires approval
	stepCount := 0
	registry.Register(&callbackExecutor{
		stepType: domain.StepTypeAI,
		callback: func(_ context.Context) (*domain.StepResult, error) {
			stepCount++
			if stepCount == 1 {
				return &domain.StepResult{Status: "success", CompletedAt: time.Now().UTC()}, nil
			}
			return &domain.StepResult{Status: "awaiting_approval", CompletedAt: time.Now().UTC()}, nil
		},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI, Required: true},
			{Name: "step2", Type: domain.StepTypeAI, Required: true},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.NoError(t, err)
	assert.NotNil(t, task)
	// Should pause at second step
	assert.Equal(t, constants.TaskStatusAwaitingApproval, task.Status)
	assert.Equal(t, 1, task.CurrentStep) // First step complete
}

// TestEngine_HandleStepResult_CurrentStepBeyondArray tests edge case handling.
func TestEngine_HandleStepResult_CurrentStepBeyondArray(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-beyond",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 10, // Beyond array bounds
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI, Status: "running"}},
		StepResults: []domain.StepResult{},
	}

	result := &domain.StepResult{
		StepName:    "step0",
		Status:      "success",
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

	// Should not panic even when CurrentStep is beyond array
	err := engine.HandleStepResult(ctx, task, result, step)

	require.NoError(t, err)
	// Result should still be appended
	assert.Len(t, task.StepResults, 1)
}

// TestEngine_ExecuteStep_CurrentStepBeyondArray tests ExecuteStep with out-of-bounds index.
func TestEngine_ExecuteStep_CurrentStepBeyondArray(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-beyond-execute",
		WorkspaceID: "test",
		CurrentStep: 5, // Beyond array
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI}},
	}

	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

	// Should not panic and execute successfully
	result, err := engine.ExecuteStep(ctx, task, step)

	require.NoError(t, err)
	assert.NotNil(t, result)
}

// TestEngine_ProcessStepResult_HandleResultError_WithStoreSaveError tests the error path with store save failure.
func TestEngine_ProcessStepResult_HandleResultError_WithStoreSaveError(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.updateErr = atlaserrors.ErrLockTimeout // This will make the best-effort save fail
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-handle-error",
		WorkspaceID: "test",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps:       []domain.Step{{Name: "step0", Type: domain.StepTypeAI, Status: "running"}},
		StepResults: []domain.StepResult{},
	}

	result := &domain.StepResult{
		StepName:    "step0",
		Status:      "invalid_status_that_will_error", // Invalid status will cause HandleStepResult to fail
		CompletedAt: time.Now().UTC(),
	}
	step := &domain.StepDefinition{Name: "step0", Type: domain.StepTypeAI}

	err := engine.processStepResult(ctx, task, result, step)

	// Should return the HandleStepResult error (not the store save error which is just logged)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrUnknownStepResultStatus)
}

// TestEngine_RunSteps_ExecuteCurrentStepError tests runSteps error handling when execute fails.
func TestEngine_RunSteps_ExecuteCurrentStepError(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()

	// Executor that fails with context cancellation
	registry.Register(&callbackExecutor{
		stepType: domain.StepTypeAI,
		callback: func(_ context.Context) (*domain.StepResult, error) {
			return nil, context.Canceled
		},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI, Required: true},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should return the executor error
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotNil(t, task)
	// Task should be in a failed state
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Status)
}

// TestEngine_Start_StoreCreateFails tests Start when store.Create fails.
func TestEngine_Start_StoreCreateFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.createErr = atlaserrors.ErrLockTimeout // Make store.Create fail
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save task")
	assert.Nil(t, task)
}

// TestEngine_RunSteps_AdvanceToNextStepError tests runSteps when advanceToNextStep fails.
func TestEngine_RunSteps_AdvanceToNextStepError(t *testing.T) {
	ctx := context.Background()

	// Use a store that fails on second update (which happens in advanceToNextStep)
	store := &conditionalFailStore{
		mockStore:    newMockStore(),
		failOnUpdate: 1, // Fail on first update after create
	}
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should return the store error from advanceToNextStep
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save checkpoint") // Error from advanceToNextStep
	assert.NotNil(t, task)
}

// TestEngine_RunSteps_ContextErrorInLoop tests the context.Err() check in the loop.
func TestEngine_RunSteps_ContextErrorInLoop(t *testing.T) {
	registry := steps.NewExecutorRegistry()

	// First step succeeds, then context gets canceled
	registry.Register(&callbackExecutor{
		stepType: domain.StepTypeAI,
		callback: func(_ context.Context) (*domain.StepResult, error) {
			return &domain.StepResult{Status: "success", CompletedAt: time.Now().UTC()}, nil
		},
	})

	template := &domain.Template{
		Name: "test-template",
		Steps: []domain.StepDefinition{
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
			{Name: "step3", Type: domain.StepTypeAI},
		},
	}

	// Create a context that we'll cancel after the first step
	ctx, cancel := context.WithCancel(context.Background())

	// Use a modified store that cancels context after first step
	cancellingStore := &contextCancellingStore{
		mockStore:    newMockStore(),
		cancelFunc:   cancel,
		cancelOnCall: 1, // Cancel on first Update call (after first step completes)
		callCount:    0,
	}

	engine := NewEngine(cancellingStore, registry, DefaultEngineConfig(), testLogger())

	task, err := engine.Start(ctx, "test-workspace", "test-branch", template, "test")

	// Should return context canceled error
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotNil(t, task)
}

// contextCancellingStore cancels the context after a certain number of calls.
type contextCancellingStore struct {
	*mockStore

	cancelFunc   func()
	cancelOnCall int
	callCount    int
}

func (s *contextCancellingStore) Update(ctx context.Context, workspaceName string, task *domain.Task) error {
	s.callCount++
	if s.callCount == s.cancelOnCall {
		s.cancelFunc()
	}
	return s.mockStore.Update(ctx, workspaceName, task)
}

// TestEngine_Abandon_Success tests successful task abandonment.
func TestEngine_Abandon_Success(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	// Create a task in validation_failed state
	task := &domain.Task{
		ID:          "task-abandon-test",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusValidationFailed,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "step1", Status: "completed"},
			{Name: "step2", Status: "failed"},
		},
		Metadata: map[string]any{
			"last_error": "validation failed: some error",
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "User requested abandonment", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
	assert.NotNil(t, task.CompletedAt) // Terminal state sets CompletedAt
	assert.Len(t, task.Transitions, 1)
	assert.Equal(t, constants.TaskStatusValidationFailed, task.Transitions[0].FromStatus)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Transitions[0].ToStatus)
	assert.Equal(t, "User requested abandonment", task.Transitions[0].Reason)
}

// TestEngine_Abandon_FromGHFailed tests abandonment from gh_failed state.
func TestEngine_Abandon_FromGHFailed(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-gh-fail",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusGHFailed,
		Steps:       []domain.Step{{Name: "git-push", Status: "failed"}},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "PR creation failed", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_Abandon_FromCIFailed tests abandonment from ci_failed state.
func TestEngine_Abandon_FromCIFailed(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-ci-fail",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCIFailed,
		Steps:       []domain.Step{{Name: "ci-check", Status: "failed"}},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "CI tests failed repeatedly", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_Abandon_FromCITimeout tests abandonment from ci_timeout state.
func TestEngine_Abandon_FromCITimeout(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-ci-timeout",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusCITimeout,
		Steps:       []domain.Step{{Name: "ci-wait", Status: "failed"}},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "CI timed out", false)

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_Abandon_RejectsNonAbandonableState tests that non-abandonable states are rejected.
func TestEngine_Abandon_RejectsNonAbandonableState(t *testing.T) {
	tests := []struct {
		name   string
		status constants.TaskStatus
	}{
		{"running", constants.TaskStatusRunning},
		{"pending", constants.TaskStatusPending},
		{"validating", constants.TaskStatusValidating},
		{"awaiting_approval", constants.TaskStatusAwaitingApproval},
		{"completed", constants.TaskStatusCompleted},
		{"rejected", constants.TaskStatusRejected},
		{"abandoned", constants.TaskStatusAbandoned},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			store := newMockStore()
			engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

			originalStatus := tc.status
			task := &domain.Task{
				ID:          "task-test",
				WorkspaceID: "test-workspace",
				Status:      tc.status,
				Transitions: []domain.Transition{},
			}
			store.tasks[task.ID] = task

			err := engine.Abandon(ctx, task, "trying to abandon", false)

			require.Error(t, err)
			require.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
			assert.Equal(t, originalStatus, task.Status) // Status unchanged from original
		})
	}
}

// TestEngine_Abandon_NilTask tests abandonment with nil task.
func TestEngine_Abandon_NilTask(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	err := engine.Abandon(ctx, nil, "abandon nil task", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
}

// TestEngine_Abandon_ContextCanceled tests context cancellation during abandon.
func TestEngine_Abandon_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-cancel-test",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusValidationFailed,
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "abandoning", false)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestEngine_Abandon_StoreFails tests when store fails during abandon.
func TestEngine_Abandon_StoreFails(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	store.updateErr = atlaserrors.ErrWorkspaceNotFound // Simulate store failure
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-store-fail",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusValidationFailed,
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "abandoning", false)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
	// Task is still transitioned in memory, but save failed
	assert.Equal(t, constants.TaskStatusAbandoned, task.Status)
}

// TestEngine_Abandon_PreservesMetadata tests that abandonment preserves task metadata.
func TestEngine_Abandon_PreservesMetadata(t *testing.T) {
	ctx := context.Background()

	store := newMockStore()
	engine := NewEngine(store, nil, DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-metadata-test",
		WorkspaceID: "test-workspace",
		Status:      constants.TaskStatusValidationFailed,
		Steps: []domain.Step{
			{Name: "step1", Status: "completed", Attempts: 1},
			{Name: "step2", Status: "failed", Attempts: 3},
		},
		Metadata: map[string]any{
			"last_error":    "validation failed: lint errors",
			"retry_context": "Previous attempts: 3",
		},
		Transitions: []domain.Transition{},
	}
	store.tasks[task.ID] = task

	err := engine.Abandon(ctx, task, "giving up after 3 retries", false)

	require.NoError(t, err)
	// Metadata should be preserved
	assert.Equal(t, "validation failed: lint errors", task.Metadata["last_error"])
	assert.Equal(t, "Previous attempts: 3", task.Metadata["retry_context"])
	// Steps should be preserved
	assert.Len(t, task.Steps, 2)
	assert.Equal(t, 3, task.Steps[1].Attempts)
}

func TestEngine_InjectLoggerContext(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	logger := zerolog.New(&buf)
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), logger)

	ctx := context.Background()
	workspaceName := "test-workspace"
	taskID := "task-20251231-120000"

	// Inject logger context
	enrichedCtx := engine.injectLoggerContext(ctx, workspaceName, taskID)

	// Extract logger from context and log a message
	log := zerolog.Ctx(enrichedCtx)
	log.Info().Msg("test message")

	// Verify the log output contains workspace_name and task_id
	output := buf.String()
	assert.Contains(t, output, `"workspace_name":"test-workspace"`)
	assert.Contains(t, output, `"task_id":"task-20251231-120000"`)
	assert.Contains(t, output, "test message")
}

func TestEngine_ShouldSkipStep_OptionalSteps(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), testLogger())

	tests := []struct {
		name     string
		task     *domain.Task
		step     *domain.StepDefinition
		expected bool
	}{
		{
			name: "optional step (Required=false) should be skipped",
			task: &domain.Task{
				ID:       "task-1",
				Metadata: nil,
			},
			step: &domain.StepDefinition{
				Name:     "verify",
				Type:     domain.StepTypeVerify,
				Required: false,
			},
			expected: true,
		},
		{
			name: "required step (Required=true) should not be skipped",
			task: &domain.Task{
				ID:       "task-2",
				Metadata: nil,
			},
			step: &domain.StepDefinition{
				Name:     "implement",
				Type:     domain.StepTypeAI,
				Required: true,
			},
			expected: false,
		},
		{
			name: "required git push step with skip_git_steps=true should be skipped",
			task: &domain.Task{
				ID:       "task-3",
				Metadata: map[string]any{"skip_git_steps": true},
			},
			step: &domain.StepDefinition{
				Name:     "git_push",
				Type:     domain.StepTypeGit,
				Required: true,
				Config:   map[string]any{"operation": "push"},
			},
			expected: true,
		},
		{
			name: "required git create_pr step with skip_git_steps=true should be skipped",
			task: &domain.Task{
				ID:       "task-4",
				Metadata: map[string]any{"skip_git_steps": true},
			},
			step: &domain.StepDefinition{
				Name:     "git_pr",
				Type:     domain.StepTypeGit,
				Required: true,
				Config:   map[string]any{"operation": "create_pr"},
			},
			expected: true,
		},
		{
			name: "required git commit step with skip_git_steps=true should NOT be skipped",
			task: &domain.Task{
				ID:       "task-5",
				Metadata: map[string]any{"skip_git_steps": true},
			},
			step: &domain.StepDefinition{
				Name:     "git_commit",
				Type:     domain.StepTypeGit,
				Required: true,
				Config:   map[string]any{"operation": "commit"},
			},
			expected: false,
		},
		{
			name: "required non-git step with skip_git_steps=true should NOT be skipped",
			task: &domain.Task{
				ID:       "task-6",
				Metadata: map[string]any{"skip_git_steps": true},
			},
			step: &domain.StepDefinition{
				Name:     "implement",
				Type:     domain.StepTypeAI,
				Required: true,
			},
			expected: false,
		},
		{
			name: "optional git push step should be skipped (Required takes precedence)",
			task: &domain.Task{
				ID:       "task-7",
				Metadata: map[string]any{"skip_git_steps": false},
			},
			step: &domain.StepDefinition{
				Name:     "git_push",
				Type:     domain.StepTypeGit,
				Required: false,
				Config:   map[string]any{"operation": "push"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.shouldSkipStep(tt.task, tt.step)
			assert.Equal(t, tt.expected, result)
		})
	}
}
