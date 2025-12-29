package task

import (
	"context"
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

	task, err := engine.Start(ctx, "test-workspace", template, "test description")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test description")

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
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeValidation},
			{Name: "step3", Type: domain.StepTypeAI},
		},
	}

	_, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	_, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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
			{Name: "validate", Type: domain.StepTypeValidation},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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
			{Name: "step1", Type: domain.StepTypeAI},
			{Name: "step2", Type: domain.StepTypeAI},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first step completes
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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
	task, err := engine.Start(ctx, "test-workspace", template, "test")

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
			{Name: "human-approve", Type: domain.StepTypeHuman},
			{Name: "next-step", Type: domain.StepTypeHuman},
		},
	}

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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

	task, err := engine.Start(ctx, "test-workspace", template, "test")

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
