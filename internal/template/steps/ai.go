// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
)

// AIExecutor handles AI steps (analyze, implement).
// It uses the ai.Runner interface to execute prompts via Claude Code CLI.
type AIExecutor struct {
	runner     ai.Runner
	workingDir string
}

// NewAIExecutor creates a new AI executor with the given runner.
func NewAIExecutor(runner ai.Runner) *AIExecutor {
	return &AIExecutor{runner: runner}
}

// NewAIExecutorWithWorkingDir creates an AI executor with a working directory.
// The working directory is used to set the Claude CLI's working directory,
// ensuring file operations happen in the correct location (e.g., worktree).
func NewAIExecutorWithWorkingDir(runner ai.Runner, workingDir string) *AIExecutor {
	return &AIExecutor{runner: runner, workingDir: workingDir}
}

// Execute runs an AI step using Claude Code.
// The step config may contain:
//   - permission_mode: string controlling AI permissions ("plan" or empty)
//   - prompt_template: string template for building the prompt
func (e *AIExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	startTime := time.Now()

	// Build AI request from task and step config first so we can log resolved agent/model
	req := e.buildRequest(task, step)

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Msg("executing ai step")

	// Execute with timeout from step definition if set
	execCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Run the AI
	result, err := e.runner.Run(execCtx, req)
	if err != nil {
		elapsed := time.Since(startTime)
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Str("agent", string(req.Agent)).
			Str("model", req.Model).
			Dur("duration_ms", elapsed).
			Msg("ai step failed")

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      "failed",
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  elapsed.Milliseconds(),
			Error:       err.Error(),
		}, err
	}

	elapsed := time.Since(startTime)

	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Str("session_id", result.SessionID).
		Int("num_turns", result.NumTurns).
		Dur("duration_ms", elapsed).
		Msg("ai step completed")

	return &domain.StepResult{
		StepIndex:    task.CurrentStep,
		StepName:     step.Name,
		Status:       "success",
		StartedAt:    startTime,
		CompletedAt:  time.Now(),
		DurationMs:   elapsed.Milliseconds(),
		Output:       result.Output,
		FilesChanged: result.FilesChanged,
		SessionID:    result.SessionID,
		NumTurns:     result.NumTurns,
	}, nil
}

// Type returns the step type this executor handles.
func (e *AIExecutor) Type() domain.StepType {
	return domain.StepTypeAI
}

// buildRequest constructs an AIRequest from task and step configuration.
func (e *AIExecutor) buildRequest(task *domain.Task, step *domain.StepDefinition) *domain.AIRequest {
	req := &domain.AIRequest{
		Agent:      task.Config.Agent,
		Prompt:     task.Description,
		Model:      task.Config.Model,
		MaxTurns:   task.Config.MaxTurns,
		Timeout:    task.Config.Timeout,
		WorkingDir: e.workingDir,
	}

	// Apply permission mode from task config
	if task.Config.PermissionMode != "" {
		req.PermissionMode = task.Config.PermissionMode
	}

	// Override with step-specific config if present
	e.applyStepConfig(req, task.Description, step.Config)

	return req
}

// applyStepConfig applies step-specific configuration overrides to the request.
func (e *AIExecutor) applyStepConfig(req *domain.AIRequest, description string, config map[string]any) {
	if config == nil {
		return
	}

	// Agent override for this step
	if agent, ok := config["agent"].(string); ok && agent != "" {
		req.Agent = domain.Agent(agent)
	}
	if pm, ok := config["permission_mode"].(string); ok {
		req.PermissionMode = pm
	}
	if pt, ok := config["prompt_template"].(string); ok {
		req.Prompt = fmt.Sprintf("%s: %s", pt, description)
	}
	if model, ok := config["model"].(string); ok {
		req.Model = model
	}
	if timeout, ok := config["timeout"].(time.Duration); ok {
		req.Timeout = timeout
	}
}
