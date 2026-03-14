package task

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// BenchmarkEngineExecuteStep benchmarks step execution overhead.
func BenchmarkEngineExecuteStep(b *testing.B) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	registry.Register(&mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: constants.StepStatusSuccess},
	})

	engine := NewEngine(store, registry, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		ID:          "benchmark-task",
		WorkspaceID: "benchmark-ws",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI, Status: constants.StepStatusPending},
		},
		StepResults: make([]domain.StepResult, 0, 10),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	step := &domain.StepDefinition{
		Name:     "step1",
		Type:     domain.StepTypeAI,
		Required: true,
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = engine.ExecuteStep(ctx, task, step)
		// Reset step status for next iteration
		task.Steps[0].Status = constants.StepStatusPending
		task.Steps[0].Attempts = 0
	}
}

// BenchmarkEngineHandleStepResult benchmarks result processing overhead.
func BenchmarkEngineHandleStepResult(b *testing.B) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		ID:          "benchmark-task",
		WorkspaceID: "benchmark-ws",
		Status:      constants.TaskStatusRunning,
		CurrentStep: 0,
		Steps: []domain.Step{
			{Name: "step1", Type: domain.StepTypeAI, Status: constants.StepStatusRunning},
		},
		StepResults: make([]domain.StepResult, 0, 100),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	step := &domain.StepDefinition{
		Name:     "step1",
		Type:     domain.StepTypeAI,
		Required: true,
	}

	result := &domain.StepResult{
		StepIndex:   0,
		StepName:    "step1",
		Status:      constants.StepStatusSuccess,
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
		DurationMs:  100,
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset task state for each iteration
		task.StepResults = task.StepResults[:0]
		task.Status = constants.TaskStatusRunning
		task.Steps[0].Status = constants.StepStatusRunning

		_ = engine.HandleStepResult(ctx, task, result, step)
	}
}

// BenchmarkStateTransition benchmarks task state transitions.
func BenchmarkStateTransition(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		task := &domain.Task{
			ID:          "benchmark-task",
			Status:      constants.TaskStatusPending,
			Transitions: make([]domain.Transition, 0, 8),
		}

		_ = Transition(ctx, task, constants.TaskStatusRunning, "test")
	}
}

// BenchmarkBuildRetryContext benchmarks retry context generation.
func BenchmarkBuildRetryContext(b *testing.B) {
	store := newMockStore()
	registry := steps.NewExecutorRegistry()
	engine := NewEngine(store, registry, DefaultEngineConfig(), zerolog.Nop())

	task := &domain.Task{
		ID:          "benchmark-task",
		CurrentStep: 5,
		StepResults: make([]domain.StepResult, 10),
	}

	// Add some failed results
	for i := range task.StepResults {
		task.StepResults[i] = domain.StepResult{
			StepName: "step",
			Status:   "failed",
			Error:    "test error",
		}
	}

	lastResult := &domain.StepResult{
		StepName: "current-step",
		Error:    "current error",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = engine.buildRetryContext(task, lastResult)
	}
}
