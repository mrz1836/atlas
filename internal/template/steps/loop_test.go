package steps

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockInnerStepRunner implements InnerStepRunner for testing.
type MockInnerStepRunner struct {
	Results      []*domain.StepResult
	Errors       []error
	ExecuteCalls int
	callIndex    int
}

func (m *MockInnerStepRunner) ExecuteStep(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	m.ExecuteCalls++
	idx := m.callIndex
	m.callIndex++

	if idx < len(m.Errors) && m.Errors[idx] != nil {
		return nil, m.Errors[idx]
	}

	if idx < len(m.Results) {
		return m.Results[idx], nil
	}

	return &domain.StepResult{Status: constants.StepStatusSuccess}, nil
}

// MockLoopStateStore implements LoopStateStore for testing.
type MockLoopStateStore struct {
	SavedState *domain.LoopState
	LoadState  *domain.LoopState
	SaveError  error
	LoadError  error
	SaveCalls  int
	LoadCalls  int
}

func (m *MockLoopStateStore) SaveLoopState(_ context.Context, _ *domain.Task, state *domain.LoopState) error {
	m.SaveCalls++
	if m.SaveError != nil {
		return m.SaveError
	}
	m.SavedState = state
	return nil
}

func (m *MockLoopStateStore) LoadLoopState(_ context.Context, _ *domain.Task, _ string) (*domain.LoopState, error) {
	m.LoadCalls++
	if m.LoadError != nil {
		return nil, m.LoadError
	}
	return m.LoadState, nil
}

// MockExitEvaluator implements ExitEvaluator for testing.
type MockExitEvaluator struct {
	ShouldExit    bool
	ExitReason    string
	ParsedSignal  bool
	EvaluateCalls int
	ParseCalls    int
}

func (m *MockExitEvaluator) Evaluate(_ *domain.IterationResult, _ string) ExitDecision {
	m.EvaluateCalls++
	return ExitDecision{ShouldExit: m.ShouldExit, Reason: m.ExitReason}
}

func (m *MockExitEvaluator) ParseExitSignal(_ string) (bool, error) {
	m.ParseCalls++
	return m.ParsedSignal, nil
}

func (m *MockExitEvaluator) CheckConditions(_ string) bool {
	return m.ShouldExit
}

// MockScratchpad implements ScratchpadWriter for testing.
type MockScratchpad struct {
	Data       *ScratchpadData
	ReadError  error
	WriteError error
	ReadCalls  int
	WriteCalls int
}

func (m *MockScratchpad) Read() (*ScratchpadData, error) {
	m.ReadCalls++
	if m.ReadError != nil {
		return nil, m.ReadError
	}
	if m.Data == nil {
		return &ScratchpadData{}, nil
	}
	return m.Data, nil
}

func (m *MockScratchpad) Write(data *ScratchpadData) error {
	m.WriteCalls++
	if m.WriteError != nil {
		return m.WriteError
	}
	m.Data = data
	return nil
}

func (m *MockScratchpad) AppendIteration(result *IterationSummary) error {
	if m.Data == nil {
		m.Data = &ScratchpadData{}
	}
	m.Data.Iterations = append(m.Data.Iterations, *result)
	return m.Write(m.Data)
}

func TestLoopExecutor_CountBasedExit(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 3, mockRunner.ExecuteCalls) // 3 iterations, 1 step each
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_SignalBasedExit(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: `{"exit": true}`},
		},
	}
	mockStore := &MockLoopStateStore{}
	mockExit := &MockExitEvaluator{ShouldExit: true, ExitReason: "exit signal"}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(mockExit),
	)

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"until_signal":   true,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 1, mockRunner.ExecuteCalls) // Exited after 1 iteration
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
}

func TestLoopExecutor_CircuitBreaker_ConsecutiveErrors(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// All steps fail - use a static error from errors package
	errFail := atlaserrors.ErrCommandFailed
	mockRunner := &MockInnerStepRunner{
		Errors: []error{
			errFail,
			errFail,
			errFail,
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 3,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 3, mockRunner.ExecuteCalls)
	assert.Equal(t, "circuit_breaker_errors", result.Metadata["exit_reason"])
}

func TestLoopExecutor_CircuitBreaker_Stagnation(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Steps succeed but no files changed
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"stagnation_iterations": 3,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 3, mockRunner.ExecuteCalls)
	assert.Equal(t, "circuit_breaker_stagnation", result.Metadata["exit_reason"])
}

func TestLoopExecutor_ResumeFromCheckpoint(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// Pre-populate state with 3 completed iterations
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 3,
			MaxIterations:    5,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1},
				{Iteration: 2},
				{Iteration: 3},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 2, mockRunner.ExecuteCalls) // Only 2 more iterations (4 and 5)
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockRunner := &MockInnerStepRunner{}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore)

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, mockRunner.ExecuteCalls)
}

func TestLoopExecutor_ScratchpadIntegration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: "iteration 1 output"},
			{Status: constants.StepStatusSuccess, Output: "iteration 2 output"},
		},
	}
	mockStore := &MockLoopStateStore{}
	mockScratchpad := &MockScratchpad{}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(mockScratchpad),
	)

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  2,
			"scratchpad_file": "progress.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Positive(t, mockScratchpad.WriteCalls)
	assert.Len(t, mockScratchpad.Data.Iterations, 2)
}

func TestLoopExecutor_ParseLoopConfig(t *testing.T) {
	executor := &LoopExecutor{}

	config := map[string]any{
		"max_iterations": 5,
		"until":          "all_tests_pass",
		"until_signal":   true,
		"exit_conditions": []any{
			"all tests passing",
			"no lint errors",
		},
		"fresh_context":   true,
		"scratchpad_file": "progress.json",
		"circuit_breaker": map[string]any{
			"stagnation_iterations": 3,
			"consecutive_errors":    5,
		},
		"steps": []any{
			map[string]any{
				"name": "fix",
				"type": "ai",
			},
		},
	}

	cfg, err := executor.parseLoopConfig(config)
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.MaxIterations)
	assert.Equal(t, "all_tests_pass", cfg.Until)
	assert.True(t, cfg.UntilSignal)
	assert.Len(t, cfg.ExitConditions, 2)
	assert.True(t, cfg.FreshContext)
	assert.Equal(t, "progress.json", cfg.ScratchpadFile)
	assert.Equal(t, 3, cfg.CircuitBreaker.StagnationIterations)
	assert.Equal(t, 5, cfg.CircuitBreaker.ConsecutiveErrors)
	assert.Len(t, cfg.Steps, 1)
	assert.Equal(t, "fix", cfg.Steps[0].Name)
}

func TestLoopExecutor_ParseLoopConfig_FloatConversion(t *testing.T) {
	// JSON unmarshaling produces float64 for numbers
	executor := &LoopExecutor{}

	config := map[string]any{
		"max_iterations": float64(10),
		"circuit_breaker": map[string]any{
			"stagnation_iterations": float64(3),
			"consecutive_errors":    float64(5),
		},
		"steps": []any{
			map[string]any{"name": "fix", "type": "ai"},
		},
	}

	cfg, err := executor.parseLoopConfig(config)
	require.NoError(t, err)

	assert.Equal(t, 10, cfg.MaxIterations)
	assert.Equal(t, 3, cfg.CircuitBreaker.StagnationIterations)
	assert.Equal(t, 5, cfg.CircuitBreaker.ConsecutiveErrors)
}

func TestLoopExecutor_Type(t *testing.T) {
	executor := &LoopExecutor{}
	assert.Equal(t, domain.StepTypeLoop, executor.Type())
}

func TestLoopExecutor_MultipleInnerSteps(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// 2 iterations, 2 steps per iteration = 4 calls
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: "fix output"},
			{Status: constants.StepStatusSuccess, Output: "validate output"},
			{Status: constants.StepStatusSuccess, Output: "fix output 2"},
			{Status: constants.StepStatusSuccess, Output: "validate output 2"},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "fix", "type": "ai"},
				map[string]any{"name": "validate", "type": "validation"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 4, mockRunner.ExecuteCalls) // 2 iterations * 2 steps
}

func TestLoopExecutor_UntilCondition(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	// Task with validation passed
	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
		StepResults: []domain.StepResult{
			{StepName: "validate", Status: "success"},
		},
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"until":          "validation_passed",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should exit immediately because condition is already met
	assert.Equal(t, "condition_met", result.Metadata["exit_reason"])
}

func TestLoopExecutor_FilesChangedAccumulation(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"file1.go"}},
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"file2.go", "file3.go"}},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Len(t, result.FilesChanged, 3)
	assert.Contains(t, result.FilesChanged, "file1.go")
	assert.Contains(t, result.FilesChanged, "file2.go")
	assert.Contains(t, result.FilesChanged, "file3.go")
}

func TestLoopExecutor_CheckpointSaving(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "task-123",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Checkpoint saved after each iteration (3 times)
	assert.Equal(t, 3, mockStore.SaveCalls)
	assert.NotNil(t, mockStore.SavedState)
	assert.Equal(t, 3, mockStore.SavedState.CurrentIteration)
	assert.False(t, mockStore.SavedState.LastCheckpoint.IsZero())
}

func TestLoopExecutor_EmptyConfig(t *testing.T) {
	executor := &LoopExecutor{}
	cfg, err := executor.parseLoopConfig(nil)
	require.NoError(t, err)

	assert.NotNil(t, cfg)
	assert.Equal(t, 0, cfg.MaxIterations)
	assert.Empty(t, cfg.Until)
	assert.False(t, cfg.UntilSignal)
}

func TestLoopExecutor_Duration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 1,
			"steps":          []any{map[string]any{"name": "inner", "type": "ai"}},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.DurationMs, int64(0)) // Duration may be 0 for fast tests
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.True(t, result.CompletedAt.After(result.StartedAt) || result.CompletedAt.Equal(result.StartedAt))
}

func TestLoopExecutor_StagnationResetOnChange(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Pattern: no change, no change, change, no change, no change, no change
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"file.go"}}, // Resets stagnation
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil}, // Triggers stagnation
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"stagnation_iterations": 3,
			},
			"steps": []any{map[string]any{"name": "inner", "type": "ai"}},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should have run 6 iterations: 2 stagnant, 1 change, 3 stagnant (triggering exit)
	assert.Equal(t, 6, mockRunner.ExecuteCalls)
	assert.Equal(t, "circuit_breaker_stagnation", result.Metadata["exit_reason"])
}

func TestLoopState_Fields(t *testing.T) {
	now := time.Now()
	state := domain.LoopState{
		StepName:         "fix_loop",
		CurrentIteration: 5,
		MaxIterations:    10,
		CurrentInnerStep: 2,
		CompletedIterations: []domain.IterationResult{
			{Iteration: 1},
			{Iteration: 2},
		},
		ExitReason:        "max_iterations_reached",
		ScratchpadPath:    "/path/to/scratchpad.json",
		StagnationCount:   3,
		ConsecutiveErrors: 0,
		StartedAt:         now,
		LastCheckpoint:    now,
	}

	assert.Equal(t, "fix_loop", state.StepName)
	assert.Equal(t, 5, state.CurrentIteration)
	assert.Equal(t, 10, state.MaxIterations)
	assert.Len(t, state.CompletedIterations, 2)
	assert.Equal(t, "max_iterations_reached", state.ExitReason)
}

func TestIterationResult_Fields(t *testing.T) {
	now := time.Now()
	result := domain.IterationResult{
		Iteration: 3,
		StepResults: []domain.StepResult{
			{StepName: "fix", Status: "success"},
		},
		FilesChanged: []string{"file1.go", "file2.go"},
		ExitSignal:   true,
		Duration:     5 * time.Second,
		StartedAt:    now,
		CompletedAt:  now.Add(5 * time.Second),
		Error:        "",
	}

	assert.Equal(t, 3, result.Iteration)
	assert.Len(t, result.StepResults, 1)
	assert.Len(t, result.FilesChanged, 2)
	assert.True(t, result.ExitSignal)
	assert.Equal(t, 5*time.Second, result.Duration)
}

// ====================
// Phase 2: Resume/Restart Scenarios
// ====================

// SequencedMockRunner allows programming a sequence of responses for inner steps.
type SequencedMockRunner struct {
	Responses []struct {
		Result *domain.StepResult
		Err    error
	}
	current      int
	ExecuteCalls int
}

func (m *SequencedMockRunner) ExecuteStep(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	m.ExecuteCalls++
	if m.current >= len(m.Responses) {
		return &domain.StepResult{Status: constants.StepStatusSuccess}, nil
	}
	resp := m.Responses[m.current]
	m.current++
	return resp.Result, resp.Err
}

func TestLoopExecutor_ResumePartialIteration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Runner will succeed for remaining steps
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State shows we were in iteration 2, inner step 1 (second step)
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 2,
			MaxIterations:    3,
			CurrentInnerStep: 1,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1, StepResults: []domain.StepResult{{Status: constants.StepStatusSuccess}}},
				{Iteration: 2, StepResults: []domain.StepResult{{Status: constants.StepStatusSuccess}}},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "fix", "type": "ai"},
				map[string]any{"name": "validate", "type": "validation"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	// Should only run iteration 3 (2 steps)
	assert.Equal(t, 2, mockRunner.ExecuteCalls)
}

func TestLoopExecutor_ResumeWithModifiedMaxIterations(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State shows 3 completed, but max was 3
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 3,
			MaxIterations:    3, // Old max
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1},
				{Iteration: 2},
				{Iteration: 3},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 6, // New max - increased
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should continue to new max (iterations 4, 5, 6)
	assert.Equal(t, 3, mockRunner.ExecuteCalls)
	assert.Equal(t, 6, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ResumeWithDecreasedMaxIterations(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{}

	// State shows 5 completed, old max was 10
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 5,
			MaxIterations:    10,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1},
				{Iteration: 2},
				{Iteration: 3},
				{Iteration: 4},
				{Iteration: 5},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3, // Decreased below current
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should exit immediately - already past new max
	assert.Equal(t, 0, mockRunner.ExecuteCalls)
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
}

func TestLoopExecutor_ResumeFromFullyCompletedLoop(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{}

	// State shows loop already completed with exit signal
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 5,
			MaxIterations:    5,
			ExitReason:       "max_iterations_reached",
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1},
				{Iteration: 2},
				{Iteration: 3},
				{Iteration: 4},
				{Iteration: 5},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should not run any additional iterations
	assert.Equal(t, 0, mockRunner.ExecuteCalls)
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ResumeWithCorruptedState(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State store returns error (corrupted state)
	mockStore := &MockLoopStateStore{
		LoadError: atlaserrors.ErrWorkspaceCorrupted,
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should start fresh after failed state load
	assert.Equal(t, 3, mockRunner.ExecuteCalls)
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ResumePreservesIterationResults(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"new.go"}},
		},
	}

	// State with prior iterations
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 2,
			MaxIterations:    3,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1, FilesChanged: []string{"old1.go"}},
				{Iteration: 2, FilesChanged: []string{"old2.go"}},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should include files from all iterations (prior + new)
	assert.Contains(t, result.FilesChanged, "old1.go")
	assert.Contains(t, result.FilesChanged, "old2.go")
	assert.Contains(t, result.FilesChanged, "new.go")
}

func TestLoopExecutor_ResumeWithScratchpadData(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}

	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 2,
			MaxIterations:    3,
			ScratchpadPath:   "/tmp/scratchpad.json",
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1}, {Iteration: 2},
			},
		},
	}

	mockScratchpad := &MockScratchpad{
		Data: &ScratchpadData{
			TaskID: "task-123",
			Iterations: []IterationSummary{
				{Number: 1, Summary: "First iteration"},
				{Number: 2, Summary: "Second iteration"},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(mockScratchpad),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  3,
			"scratchpad_file": "progress.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	// Scratchpad should have all 3 iterations
	assert.Len(t, mockScratchpad.Data.Iterations, 3)
}

func TestLoopExecutor_ResumeAfterErrorIteration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State shows 2 consecutive errors occurred, resuming from iteration 2
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:          "test_loop",
			CurrentIteration:  2,
			MaxIterations:     5,
			ConsecutiveErrors: 2,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1, Error: "failed"},
				{Iteration: 2, Error: "failed again"},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 5,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should continue from iteration 3 to 5 (3 more iterations)
	assert.Equal(t, 3, mockRunner.ExecuteCalls)
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ResumeAfterStagnation(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Return files changed to avoid stagnation
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"fixed.go"}},
		},
	}

	// State shows 2 stagnation iterations occurred
	mockStore := &MockLoopStateStore{
		LoadState: &domain.LoopState{
			StepName:         "test_loop",
			CurrentIteration: 2,
			MaxIterations:    5,
			StagnationCount:  2,
			CompletedIterations: []domain.IterationResult{
				{Iteration: 1, FilesChanged: nil},
				{Iteration: 2, FilesChanged: nil},
			},
		},
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"circuit_breaker": map[string]any{
				"stagnation_iterations": 3,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Making a change resets stagnation, should continue to max
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
}

// ====================
// Phase 3: Error Recovery Scenarios
// ====================

func TestLoopExecutor_StateStoreLoadError(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State store fails to load
	mockStore := &MockLoopStateStore{
		LoadError: atlaserrors.ErrTaskNotFound,
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should start fresh on load error
	assert.Equal(t, 2, mockRunner.ExecuteCalls)
	assert.Equal(t, 2, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_StateStoreSaveError(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// State store fails to save
	mockStore := &MockLoopStateStore{
		SaveError: atlaserrors.ErrWorkspaceCorrupted,
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should continue despite save errors
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 2, mockRunner.ExecuteCalls)
}

func TestLoopExecutor_ScratchpadReadError(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	mockScratchpad := &MockScratchpad{
		ReadError: atlaserrors.ErrArtifactNotFound,
	}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(mockScratchpad),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  1,
			"scratchpad_file": "progress.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should continue without scratchpad
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
}

func TestLoopExecutor_ScratchpadWriteError(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	mockScratchpad := &MockScratchpad{
		WriteError: atlaserrors.ErrPathTraversal,
	}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(mockScratchpad),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  1,
			"scratchpad_file": "progress.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should continue despite write error
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
}

func TestLoopExecutor_PartialInnerStepFailure(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// First step succeeds, second fails
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess}, // First inner step
		},
		Errors: []error{
			nil,                          // First step succeeds
			atlaserrors.ErrCommandFailed, // Second step fails
			nil,                          // Iteration 2 first step succeeds
			atlaserrors.ErrCommandFailed, // Iteration 2 second step fails
			nil,                          // Iteration 3 first step succeeds
			atlaserrors.ErrCommandFailed, // Iteration 3 second step fails
			nil,                          // Etc...
			atlaserrors.ErrCommandFailed,
			nil,
			atlaserrors.ErrCommandFailed, // This should trigger circuit breaker
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 5,
			},
			"steps": []any{
				map[string]any{"name": "fix", "type": "ai"},
				map[string]any{"name": "validate", "type": "validation"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Circuit breaker should trigger after 5 consecutive errors
	assert.Equal(t, "circuit_breaker_errors", result.Metadata["exit_reason"])
}

func TestLoopExecutor_RecoveryAfterErrors(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// 2 errors then success
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			nil,                                   // Error
			nil,                                   // Error
			{Status: constants.StepStatusSuccess}, // Success - resets error count
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
		Errors: []error{
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			nil,
			nil,
			nil,
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 3, // Threshold is 3
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should complete all 5 iterations
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ErrorThenStagnation(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Error, success (no files), success (no files), success (no files) - stagnation
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			nil, // Error
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
		},
		Errors: []error{
			atlaserrors.ErrCommandFailed,
			nil,
			nil,
			nil,
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"consecutive_errors":    5,
				"stagnation_iterations": 3,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Should exit due to stagnation (error doesn't count toward stagnation)
	assert.Equal(t, "circuit_breaker_stagnation", result.Metadata["exit_reason"])
}

func TestLoopExecutor_AllInnerStepsFail(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Errors: []error{
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 5,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "circuit_breaker_errors", result.Metadata["exit_reason"])
	assert.Equal(t, 5, mockRunner.ExecuteCalls)
}

// ====================
// Phase 5: Edge Cases and Boundary Tests
// ====================

func TestLoopExecutor_ZeroMaxIterationsWithUntilSignal(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Signal exit on first iteration
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: `{"exit": true}`},
		},
	}
	mockStore := &MockLoopStateStore{}
	mockExit := &MockExitEvaluator{ShouldExit: true, ExitReason: "exit signal"}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(mockExit),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 0, // Unlimited iterations
			"until_signal":   true,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
	assert.Equal(t, 1, mockRunner.ExecuteCalls)
}

func TestLoopExecutor_OneIteration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 1,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 1, mockRunner.ExecuteCalls)
	assert.Equal(t, 1, result.Metadata["iterations_completed"])
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
}

func TestLoopExecutor_LargeMaxIterations(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Exit early via signal
	callCount := 0
	mockRunner := &MockInnerStepRunner{
		Results: make([]*domain.StepResult, 100),
	}
	for i := range mockRunner.Results {
		mockRunner.Results[i] = &domain.StepResult{Status: constants.StepStatusSuccess}
	}

	mockStore := &MockLoopStateStore{}
	mockExit := &MockExitEvaluator{ShouldExit: false}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(mockExit),
	)

	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100, // Large but reasonable
			"until_signal":   true,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 100, result.Metadata["iterations_completed"])
	_ = callCount // Suppress unused warning
}

func TestLoopExecutor_SingleInnerStep(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: "output1"},
			{Status: constants.StepStatusSuccess, Output: "output2"},
			{Status: constants.StepStatusSuccess, Output: "output3"},
			{Status: constants.StepStatusSuccess, Output: "output4"},
			{Status: constants.StepStatusSuccess, Output: "output5"},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"steps": []any{
				map[string]any{"name": "single", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 5, mockRunner.ExecuteCalls)
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ManyInnerSteps(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// 3 iterations * 10 inner steps = 30 calls
	results := make([]*domain.StepResult, 30)
	for i := range results {
		results[i] = &domain.StepResult{Status: constants.StepStatusSuccess}
	}

	mockRunner := &MockInnerStepRunner{Results: results}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}

	// 10 inner steps
	innerSteps := make([]any, 10)
	for i := range innerSteps {
		innerSteps[i] = map[string]any{"name": fmt.Sprintf("step_%d", i), "type": "ai"}
	}

	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps":          innerSteps,
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 30, mockRunner.ExecuteCalls)
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
}

func TestLoopExecutor_ExitOnFirstIteration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, Output: `{"exit": true}`},
		},
	}
	mockStore := &MockLoopStateStore{}
	mockExit := &MockExitEvaluator{ShouldExit: true, ExitReason: "immediate exit"}

	executor := NewLoopExecutor(mockRunner, mockStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(mockExit),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			"until_signal":   true,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 1, mockRunner.ExecuteCalls)
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
}

func TestLoopExecutor_ExitOnLastIteration(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess, Output: `{"exit": true}`},
		},
	}
	mockStore := &MockLoopStateStore{}

	// Use max iterations to exit on 5th
	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, 5, mockRunner.ExecuteCalls)
	assert.Equal(t, "max_iterations_reached", result.Metadata["exit_reason"])
}

func TestLoopExecutor_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Give time for context to expire
	time.Sleep(5 * time.Millisecond)

	mockRunner := &MockInnerStepRunner{}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore)

	task := &domain.Task{ID: "task-123"}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLoopExecutor_ContextCancelDuringInnerStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := zerolog.Nop()

	// Cancel during first inner step execution
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}

	// Custom runner that cancels context
	cancellingRunner := &cancelOnCallRunner{
		cancel:    cancel,
		cancelAt:  2, // Cancel on second call
		results:   mockRunner.Results,
		callCount: 0,
	}

	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(cancellingRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	// Should handle cancellation gracefully
	if err != nil {
		assert.ErrorIs(t, err, context.Canceled)
	} else {
		// Or should have exited early
		assert.LessOrEqual(t, cancellingRunner.callCount, 3)
		assert.NotNil(t, result)
	}
}

// cancelOnCallRunner cancels context on a specific call
type cancelOnCallRunner struct {
	cancel    context.CancelFunc
	cancelAt  int
	results   []*domain.StepResult
	callCount int
}

func (r *cancelOnCallRunner) ExecuteStep(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	r.callCount++
	if r.callCount == r.cancelAt {
		r.cancel()
	}
	if r.callCount <= len(r.results) {
		return r.results[r.callCount-1], nil
	}
	return &domain.StepResult{Status: constants.StepStatusSuccess}, nil
}

func TestLoopExecutor_EmptyFilesChanged(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// All iterations return no files changed
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: []string{}},
			{Status: constants.StepStatusSuccess, FilesChanged: nil},
			{Status: constants.StepStatusSuccess, FilesChanged: []string{}},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"circuit_breaker": map[string]any{
				"stagnation_iterations": 3,
			},
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "circuit_breaker_stagnation", result.Metadata["exit_reason"])
	assert.Empty(t, result.FilesChanged)
}

func TestLoopExecutor_DuplicateFilesChanged(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Same file modified in multiple iterations
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"common.go", "a.go"}},
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"common.go", "b.go"}},
			{Status: constants.StepStatusSuccess, FilesChanged: []string{"common.go", "c.go"}},
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Duplicates are included (not deduplicated)
	assert.Contains(t, result.FilesChanged, "common.go")
	assert.Contains(t, result.FilesChanged, "a.go")
	assert.Contains(t, result.FilesChanged, "b.go")
	assert.Contains(t, result.FilesChanged, "c.go")
}

func TestLoopExecutor_NilStateStore(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// No state store - should still work
	executor := NewLoopExecutor(mockRunner, nil, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, 2, mockRunner.ExecuteCalls)
}

func TestLoopExecutor_DefaultCircuitBreakerThreshold(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Default threshold is 5 consecutive errors
	mockRunner := &MockInnerStepRunner{
		Errors: []error{
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
			atlaserrors.ErrCommandFailed,
		},
	}
	mockStore := &MockLoopStateStore{}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 100,
			// No circuit_breaker config - uses default of 5
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "circuit_breaker_errors", result.Metadata["exit_reason"])
	assert.Equal(t, 5, mockRunner.ExecuteCalls)
}

// ============================================================================
// Tests for QW-1, QW-2, QW-3: Error handling improvements
// ============================================================================

func TestLoopExecutor_ScratchpadErrorStoredInMetadata(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
		},
	}
	mockStore := &MockLoopStateStore{}

	// Create a scratchpad that fails on write
	mockScratchpad := &MockScratchpad{
		WriteError: atlaserrors.Wrap(atlaserrors.ErrCommandFailed, "scratchpad write failed"),
	}

	executor := NewLoopExecutor(
		mockRunner,
		mockStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(mockScratchpad),
	)

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  1,
			"scratchpad_file": "test_scratchpad.json",
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err) // Scratchpad error doesn't fail execution
	// But the error should be stored in metadata
	assert.NotNil(t, task.Metadata)
	assert.Contains(t, task.Metadata, "scratchpad_setup_error")
}

func TestLoopExecutor_CheckpointFailureAfterThreshold(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Create a runner that succeeds
	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// Create a store that always fails on save
	mockStore := &MockLoopStateStore{
		SaveError: atlaserrors.Wrap(atlaserrors.ErrCommandFailed, "checkpoint save failed"),
	}

	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)

	// Should fail after 3 consecutive checkpoint failures
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint persistence failing")
}

func TestLoopExecutor_ParseLoopConfigValidation(t *testing.T) {
	logger := zerolog.Nop()

	tests := []struct {
		name        string
		config      map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "negative max_iterations",
			config: map[string]any{
				"max_iterations": -1,
				"steps":          []any{},
			},
			expectError: true,
			errorMsg:    "max_iterations cannot be negative",
		},
		{
			name: "negative consecutive_errors",
			config: map[string]any{
				"max_iterations": 1,
				"circuit_breaker": map[string]any{
					"consecutive_errors": -5,
				},
				"steps": []any{},
			},
			expectError: true,
			errorMsg:    "consecutive_errors cannot be negative",
		},
		{
			name: "negative stagnation_iterations",
			config: map[string]any{
				"max_iterations": 1,
				"circuit_breaker": map[string]any{
					"stagnation_iterations": -3,
				},
				"steps": []any{},
			},
			expectError: true,
			errorMsg:    "stagnation_iterations cannot be negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create fresh executor for each test
			mockRunner := &MockInnerStepRunner{}
			mockStore := &MockLoopStateStore{}
			executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

			// Use timeout context to prevent hanging
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			task := &domain.Task{ID: "task-123", CurrentStep: 0}
			step := &domain.StepDefinition{
				Name:   "test_loop",
				Type:   domain.StepTypeLoop,
				Config: tc.config,
			}

			_, err := executor.Execute(ctx, task, step)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorMsg)
		})
	}
}

func TestLoopExecutor_ParseLoopConfigValid(t *testing.T) {
	logger := zerolog.Nop()

	// Create fresh executor for this test
	mockRunner := &MockInnerStepRunner{}
	mockStore := &MockLoopStateStore{}
	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	// Use timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"circuit_breaker": map[string]any{
				"consecutive_errors":    3,
				"stagnation_iterations": 2,
			},
			"steps": []any{}, // Empty steps - should complete quickly
		},
	}

	// With empty steps and max_iterations: 2, should complete immediately
	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)
}

func TestLoopExecutor_ParseLoopConfigNil(t *testing.T) {
	logger := zerolog.Nop()

	// Create fresh executor for this test
	mockRunner := &MockInnerStepRunner{}
	mockStore := &MockLoopStateStore{}
	executor := NewLoopExecutor(mockRunner, mockStore, WithLoopLogger(logger))

	// Use timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name:   "test_loop",
		Type:   domain.StepTypeLoop,
		Config: nil,
	}

	// Nil config should use defaults and complete
	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)
}

func TestLoopExecutor_ConsecutiveCheckpointErrorReset(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	mockRunner := &MockInnerStepRunner{
		Results: []*domain.StepResult{
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
			{Status: constants.StepStatusSuccess},
		},
	}

	// Override to fail first 2 saves, then succeed
	customStore := &failThenSucceedStore{
		failUntil: 2,
	}

	executor := NewLoopExecutor(mockRunner, customStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "task-123", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 5,
			"steps": []any{
				map[string]any{"name": "inner", "type": "ai"},
			},
		},
	}

	// Should succeed because consecutive errors reset after a successful save
	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)
}

// failThenSucceedStore fails saves until failUntil count, then succeeds.
type failThenSucceedStore struct {
	failUntil  int
	callCount  int
	savedState *domain.LoopState
}

func (s *failThenSucceedStore) SaveLoopState(_ context.Context, _ *domain.Task, state *domain.LoopState) error {
	s.callCount++
	if s.callCount <= s.failUntil {
		return atlaserrors.Wrapf(atlaserrors.ErrCommandFailed, "simulated checkpoint failure %d", s.callCount)
	}
	s.savedState = state
	return nil
}

func (s *failThenSucceedStore) LoadLoopState(_ context.Context, _ *domain.Task, _ string) (*domain.LoopState, error) {
	// Return nil state to indicate no saved state exists (not an error condition for resumption)
	return nil, nil //nolint:nilnil // nil state means no state to resume from, which is valid
}
