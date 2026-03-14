package steps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ====================
// Phase 6: Integration Tests
// ====================

// IntegrationMockRunner provides more realistic mock behavior for integration tests.
type IntegrationMockRunner struct {
	Sequence []IntegrationStepResponse
	callIdx  int
}

type IntegrationStepResponse struct {
	Result *domain.StepResult
	Err    error
	Delay  time.Duration // Optional delay to simulate work
}

func (m *IntegrationMockRunner) ExecuteStep(ctx context.Context, _ *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.callIdx >= len(m.Sequence) {
		return &domain.StepResult{
			Status:   constants.StepStatusSuccess,
			StepName: step.Name,
		}, nil
	}

	resp := m.Sequence[m.callIdx]
	m.callIdx++

	if resp.Delay > 0 {
		select {
		case <-time.After(resp.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if resp.Result != nil {
		resp.Result.StepName = step.Name
	}
	return resp.Result, resp.Err
}

// FileStateStore implements LoopStateStore using the filesystem for integration testing.
type FileStateStore struct {
	dir string
}

func NewFileStateStore(dir string) *FileStateStore {
	return &FileStateStore{dir: dir}
}

func (s *FileStateStore) SaveLoopState(_ context.Context, task *domain.Task, state *domain.LoopState) error {
	path := filepath.Join(s.dir, task.ID+"_"+state.StepName+".json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *FileStateStore) LoadLoopState(_ context.Context, task *domain.Task, stepName string) (*domain.LoopState, error) {
	path := filepath.Join(s.dir, task.ID+"_"+stepName+".json")
	data, err := os.ReadFile(path) // #nosec G304 -- test code with controlled file paths
	if os.IsNotExist(err) {
		return nil, nil //nolint:nilnil // returning nil state when file doesn't exist is expected behavior
	}
	if err != nil {
		return nil, err
	}
	var state domain.LoopState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func TestLoopIntegration_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// Simulate AI + validation loop
	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			// Iteration 1: AI fix
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				Output:       "Fixed issue in file1.go",
				FilesChanged: []string{"file1.go"},
			}},
			// Iteration 1: Validation
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: "Tests passing: 5/10",
			}},
			// Iteration 2: AI fix
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				Output:       "Fixed issue in file2.go",
				FilesChanged: []string{"file2.go"},
			}},
			// Iteration 2: Validation
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: "Tests passing: 8/10",
			}},
			// Iteration 3: AI fix
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				Output:       `Fixed final issues {"exit": true}`,
				FilesChanged: []string{"file3.go"},
			}},
			// Iteration 3: Validation
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: "all tests passing - 10/10",
			}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)
	scratchpadPath := filepath.Join(tmpDir, "scratchpad.json")
	scratchpad := NewFileScratchpad(scratchpadPath, logger)

	exitEval := NewExitEvaluator([]string{"all tests passing"}, logger)

	executor := NewLoopExecutor(mockRunner, stateStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(scratchpad),
		WithLoopExitEvaluator(exitEval),
	)

	task := &domain.Task{
		ID:          "integration-test-1",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "fix_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  10,
			"until_signal":    true,
			"exit_conditions": []any{"all tests passing"},
			"scratchpad_file": "scratchpad.json",
			"steps": []any{
				map[string]any{"name": "fix", "type": "ai"},
				map[string]any{"name": "validate", "type": "validation"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
	assert.Len(t, result.FilesChanged, 3)
	assert.Contains(t, result.FilesChanged, "file1.go")
	assert.Contains(t, result.FilesChanged, "file2.go")
	assert.Contains(t, result.FilesChanged, "file3.go")

	// Verify scratchpad was written
	scratchData, err := scratchpad.Read()
	require.NoError(t, err)
	assert.Len(t, scratchData.Iterations, 3)
}

func TestLoopIntegration_ResumeFullWorkflow(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// Pre-populate state file (simulating interrupted execution)
	stateStore := NewFileStateStore(tmpDir)
	initialState := &domain.LoopState{
		StepName:         "fix_loop",
		CurrentIteration: 2,
		MaxIterations:    5,
		CompletedIterations: []domain.IterationResult{
			{Iteration: 1, FilesChanged: []string{"a.go"}},
			{Iteration: 2, FilesChanged: []string{"b.go"}},
		},
		StartedAt: time.Now().Add(-5 * time.Minute),
	}
	task := &domain.Task{
		ID:          "resume-test-1",
		CurrentStep: 0,
	}
	err := stateStore.SaveLoopState(ctx, task, initialState)
	require.NoError(t, err)

	// Runner for remaining iterations
	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			// Iteration 3
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"c.go"},
			}},
			// Iteration 4
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"d.go"},
			}},
			// Iteration 5
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"e.go"},
			}},
		},
	}

	executor := NewLoopExecutor(mockRunner, stateStore, WithLoopLogger(logger))

	step := &domain.StepDefinition{
		Name: "fix_loop",
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
	assert.Equal(t, 5, result.Metadata["iterations_completed"])
	// Should include files from all iterations (prior + new)
	assert.Contains(t, result.FilesChanged, "a.go")
	assert.Contains(t, result.FilesChanged, "b.go")
	assert.Contains(t, result.FilesChanged, "c.go")
	assert.Contains(t, result.FilesChanged, "d.go")
	assert.Contains(t, result.FilesChanged, "e.go")
}

func TestLoopIntegration_ScratchpadPersistence(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				Output:       "First iteration work",
				FilesChanged: []string{"file1.go"},
			}},
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				Output:       "Second iteration work",
				FilesChanged: []string{"file2.go"},
			}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)
	scratchpadPath := filepath.Join(tmpDir, "artifacts", "scratchpad.json")
	scratchpad := NewFileScratchpad(scratchpadPath, logger)

	executor := NewLoopExecutor(mockRunner, stateStore,
		WithLoopLogger(logger),
		WithLoopScratchpad(scratchpad),
	)

	task := &domain.Task{
		ID:          "scratchpad-test-1",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  2,
			"scratchpad_file": "scratchpad.json",
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)

	// Verify scratchpad file contents
	content, err := os.ReadFile(scratchpadPath) // #nosec G304 -- test code with controlled file paths
	require.NoError(t, err)

	var scratchData ScratchpadData
	err = json.Unmarshal(content, &scratchData)
	require.NoError(t, err)

	assert.Equal(t, "scratchpad-test-1", scratchData.TaskID)
	assert.Equal(t, "test_loop", scratchData.LoopName)
	assert.Len(t, scratchData.Iterations, 2)
	assert.Equal(t, 1, scratchData.Iterations[0].Number)
	assert.Equal(t, 2, scratchData.Iterations[1].Number)
}

func TestLoopIntegration_StateCheckpointPersistence(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}},
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}},
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)

	executor := NewLoopExecutor(mockRunner, stateStore, WithLoopLogger(logger))

	task := &domain.Task{
		ID:          "checkpoint-test-1",
		CurrentStep: 0,
	}
	step := &domain.StepDefinition{
		Name: "checkpoint_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	_, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)

	// Verify state file was created
	statePath := filepath.Join(tmpDir, "checkpoint-test-1_checkpoint_loop.json")
	content, err := os.ReadFile(statePath) // #nosec G304 -- test code with controlled file paths
	require.NoError(t, err)

	var state domain.LoopState
	err = json.Unmarshal(content, &state)
	require.NoError(t, err)

	assert.Equal(t, "checkpoint_loop", state.StepName)
	assert.Equal(t, 3, state.CurrentIteration)
	assert.Len(t, state.CompletedIterations, 3)
}

func TestLoopIntegration_ExitSignalWithConditions(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// Exit signal present but condition not met on first tries
	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			// Iteration 1: Signal but no condition
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: `{"exit": true} - tests still failing`,
			}},
			// Iteration 2: Signal but no condition
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: `{"exit": true} - some tests failing`,
			}},
			// Iteration 3: Signal AND condition
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: `{"exit": true} - all tests passing`,
			}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)
	exitEval := NewExitEvaluator([]string{"all tests passing"}, logger)

	executor := NewLoopExecutor(mockRunner, stateStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(exitEval),
	)

	task := &domain.Task{ID: "dual-gate-test-1", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "dual_gate_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  10,
			"until_signal":    true,
			"exit_conditions": []any{"all tests passing"},
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
}

func TestLoopIntegration_CircuitBreakerRecovery(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// First execution: Hit circuit breaker
	firstRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Err: atlaserrors.ErrCommandFailed},
			{Err: atlaserrors.ErrCommandFailed},
			{Err: atlaserrors.ErrCommandFailed},
		},
	}

	stateStore := NewFileStateStore(tmpDir)

	executor1 := NewLoopExecutor(firstRunner, stateStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "circuit-breaker-1", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "recovery_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"circuit_breaker": map[string]any{
				"consecutive_errors": 3,
			},
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	result1, err := executor1.Execute(ctx, task, step)
	require.NoError(t, err)
	assert.Equal(t, "circuit_breaker_errors", result1.Metadata["exit_reason"])

	// Second execution: Recover (after "fixing" the issue)
	secondRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}},
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}},
		},
	}

	// Load existing state - it should have ConsecutiveErrors = 3
	existingState, err := stateStore.LoadLoopState(ctx, task, "recovery_loop")
	require.NoError(t, err)
	require.NotNil(t, existingState)

	// Reset the error count (simulating a new attempt after fixing)
	existingState.ConsecutiveErrors = 0
	existingState.ExitReason = "" // Clear exit reason
	err = stateStore.SaveLoopState(ctx, task, existingState)
	require.NoError(t, err)

	// Update max iterations config
	step.Config["max_iterations"] = 5

	executor2 := NewLoopExecutor(secondRunner, stateStore, WithLoopLogger(logger))
	result2, err := executor2.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "max_iterations_reached", result2.Metadata["exit_reason"])
	assert.Equal(t, 5, result2.Metadata["iterations_completed"])
}

func TestLoopIntegration_MultipleLoopsSequential(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// First loop runner
	firstLoopRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"analyze.go"},
			}},
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"analyze2.go"},
			}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)

	// Execute first loop
	executor1 := NewLoopExecutor(firstLoopRunner, stateStore, WithLoopLogger(logger))

	task := &domain.Task{ID: "multi-loop-task-1", CurrentStep: 0}
	step1 := &domain.StepDefinition{
		Name: "analyze_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 2,
			"steps": []any{
				map[string]any{"name": "analyze", "type": "ai"},
			},
		},
	}

	result1, err := executor1.Execute(ctx, task, step1)
	require.NoError(t, err)
	assert.Equal(t, 2, result1.Metadata["iterations_completed"])

	// Second loop runner
	secondLoopRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"fix.go"},
			}},
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"fix2.go"},
			}},
			{Result: &domain.StepResult{
				Status:       constants.StepStatusSuccess,
				FilesChanged: []string{"fix3.go"},
			}},
		},
	}

	// Execute second loop (same task, different step)
	executor2 := NewLoopExecutor(secondLoopRunner, stateStore, WithLoopLogger(logger))

	task.CurrentStep = 1 // Move to next step
	step2 := &domain.StepDefinition{
		Name: "fix_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 3,
			"steps": []any{
				map[string]any{"name": "fix", "type": "ai"},
			},
		},
	}

	result2, err := executor2.Execute(ctx, task, step2)
	require.NoError(t, err)
	assert.Equal(t, 3, result2.Metadata["iterations_completed"])

	// Verify both state files exist independently
	state1, err := stateStore.LoadLoopState(ctx, task, "analyze_loop")
	require.NoError(t, err)
	assert.Equal(t, 2, state1.CurrentIteration)

	state2, err := stateStore.LoadLoopState(ctx, task, "fix_loop")
	require.NoError(t, err)
	assert.Equal(t, 3, state2.CurrentIteration)
}

func TestLoopIntegration_ContextTimeoutMidExecution(t *testing.T) {
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	// Runner with delays
	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}, Delay: 10 * time.Millisecond},
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}, Delay: 10 * time.Millisecond},
			{Result: &domain.StepResult{Status: constants.StepStatusSuccess}, Delay: 100 * time.Millisecond}, // This will be interrupted
		},
	}

	stateStore := NewFileStateStore(tmpDir)

	executor := NewLoopExecutor(mockRunner, stateStore, WithLoopLogger(logger))

	// Context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	task := &domain.Task{ID: "timeout-test-1", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "timeout_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations": 10,
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	// Should either error or have partial results
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	} else {
		// If no error, should have partial completion
		assert.Less(t, result.Metadata["iterations_completed"].(int), 10)
	}
}

func TestLoopIntegration_RealExitConditionEvaluator(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()
	tmpDir := t.TempDir()

	mockRunner := &IntegrationMockRunner{
		Sequence: []IntegrationStepResponse{
			// No exit signal
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: "Working on it...",
			}},
			// Exit signal but condition not met
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: `{"exit": true} but tests failing`,
			}},
			// Exit signal and one condition met
			{Result: &domain.StepResult{
				Status: constants.StepStatusSuccess,
				Output: `{"exit": true} all tests passing`,
			}},
		},
	}

	stateStore := NewFileStateStore(tmpDir)
	exitEval := NewExitEvaluator([]string{"all tests passing"}, logger)

	executor := NewLoopExecutor(mockRunner, stateStore,
		WithLoopLogger(logger),
		WithLoopExitEvaluator(exitEval),
	)

	task := &domain.Task{ID: "real-exit-test-1", CurrentStep: 0}
	step := &domain.StepDefinition{
		Name: "exit_test_loop",
		Type: domain.StepTypeLoop,
		Config: map[string]any{
			"max_iterations":  10,
			"until_signal":    true,
			"exit_conditions": []any{"all tests passing"},
			"steps": []any{
				map[string]any{"name": "work", "type": "ai"},
			},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "exit_signal", result.Metadata["exit_reason"])
	assert.Equal(t, 3, result.Metadata["iterations_completed"])
}
