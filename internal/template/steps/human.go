// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// HumanExecutor handles steps requiring human intervention.
// It signals that human review is required by returning a special status.
type HumanExecutor struct{}

// NewHumanExecutor creates a new human executor.
func NewHumanExecutor() *HumanExecutor {
	return &HumanExecutor{}
}

// Execute signals that human review is required.
// Returns a StepResult with status "awaiting_approval" and the prompt in output.
// The prompt is read from step.Config["prompt"] if present.
func (e *HumanExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing human step")

	now := time.Now()

	// Get prompt from step config or use default
	prompt := "Review required"
	if step.Config != nil {
		if p, ok := step.Config["prompt"].(string); ok && p != "" {
			prompt = p
		}
	}

	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("prompt", prompt).
		Msg("awaiting human approval")

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      constants.StepStatusAwaitingApproval,
		StartedAt:   now,
		CompletedAt: now,
		DurationMs:  0,
		Output:      prompt,
	}, nil
}

// Type returns the step type this executor handles.
func (e *HumanExecutor) Type() domain.StepType {
	return domain.StepTypeHuman
}
