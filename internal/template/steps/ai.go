// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/validation"
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

	// Debug log for verbose mode - shows exact request configuration
	log.Debug().
		Str("task_id", task.ID).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Str("permission_mode", req.PermissionMode).
		Str("working_dir", req.WorkingDir).
		Dur("timeout", req.Timeout).
		Msg("AI request details")

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

	// Check for include_previous_errors config (used by fix template)
	// This injects validation errors from a previous detect_only validation step
	if includePrevErrors, ok := step.Config["include_previous_errors"].(bool); ok && includePrevErrors {
		e.injectPreviousValidationErrors(req, task)
	}

	return req
}

// injectPreviousValidationErrors finds the most recent failed validation result
// and appends error context to the AI prompt. This is used by the fix template
// to pass validation errors to the AI step for fixing.
func (e *AIExecutor) injectPreviousValidationErrors(req *domain.AIRequest, task *domain.Task) {
	// Find the most recent validation step result with errors
	for i := len(task.StepResults) - 1; i >= 0; i-- {
		result := task.StepResults[i]

		// Check if this step has validation failed flag (from detect_only mode)
		if validationFailed, ok := result.Metadata["validation_failed"].(bool); ok && validationFailed {
			// Try to get the pipeline result for error context
			if pipelineResult, ok := result.Metadata["pipeline_result"].(*validation.PipelineResult); ok {
				// Extract error context and build AI prompt
				errorCtx := validation.ExtractErrorContext(pipelineResult, 1, 1)
				errorPrompt := validation.BuildAIPrompt(errorCtx)

				// Append validation errors to the prompt
				req.Prompt = fmt.Sprintf("%s\n\n--- Validation Errors to Fix ---\n%s", req.Prompt, errorPrompt)
				return
			}
		}
	}
}

// applyStepConfig applies step-specific configuration overrides to the request.
func (e *AIExecutor) applyStepConfig(req *domain.AIRequest, description string, config map[string]any) {
	if config == nil {
		return
	}

	// Agent override for this step
	agentChanged := false
	if agent, ok := config["agent"].(string); ok && agent != "" {
		newAgent := domain.Agent(agent)
		// Only consider it a change if it's actually different
		if newAgent != req.Agent {
			req.Agent = newAgent
			agentChanged = true
		}
	}
	if pm, ok := config["permission_mode"].(string); ok {
		req.PermissionMode = pm
	}
	if pt, ok := config["prompt_template"].(string); ok {
		req.Prompt = fmt.Sprintf("%s: %s", pt, description)
	}
	if model, ok := config["model"].(string); ok {
		req.Model = model
	} else if agentChanged {
		// Use new agent's default model when agent changed but model wasn't specified
		req.Model = req.Agent.DefaultModel()
	}
	if timeout, ok := config["timeout"].(time.Duration); ok {
		req.Timeout = timeout
	}
}
