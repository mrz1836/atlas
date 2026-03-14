// Package task provides task lifecycle management for ATLAS.
//
// This file contains step execution logic extracted from engine.go.
// StepRunner methods handle individual step execution, parallel execution,
// skip logic, and step-related logging.
package task

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
)

// executeStepInternal executes a step without modifying task state.
// This is safe for concurrent execution in parallel step groups.
func (e *Engine) executeStepInternal(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	executor, err := e.registry.Get(step.Type)
	if err != nil {
		return nil, err
	}

	e.buildStepLogEvent(task, step, zerolog.InfoLevel, 0).Msg("executing step")

	startTime := time.Now()
	result, err := executor.Execute(ctx, task, step)
	duration := time.Since(startTime)

	if err != nil {
		e.buildStepLogEvent(task, step, zerolog.ErrorLevel, duration.Milliseconds()).
			Err(err).Msg("step execution failed")
		e.recordStepExecuted(task.ID, step.Name, step.Type, duration, false)
		return result, err
	}

	e.buildStepLogEvent(task, step, zerolog.InfoLevel, duration.Milliseconds()).
		Str("status", result.Status).Msg("step completed")
	e.recordStepExecuted(task.ID, step.Name, step.Type, duration, true)

	return result, nil
}

// buildStepLogEvent creates a log event with common step fields.
func (e *Engine) buildStepLogEvent(task *domain.Task, step *domain.StepDefinition, level zerolog.Level, durationMs int64) *zerolog.Event {
	event := e.logger.WithLevel(level). //nolint:zerologlint // event returned for caller to dispatch
						Str("task_id", task.ID).
						Str("step_name", step.Name).
						Str("step_type", string(step.Type))

	if durationMs > 0 {
		event = event.Int64("duration_ms", durationMs)
	}

	if step.Type == domain.StepTypeAI || step.Type == domain.StepTypeVerify {
		agent, model := ResolveStepAgentModel(task, step)
		event = event.Str("agent", string(agent)).Str("model", model)
	}

	return event
}

// ResolveStepAgentModel returns the resolved agent and model for a step,
// applying step-level config overrides to task defaults.
// Exported for use by other packages.
func ResolveStepAgentModel(task *domain.Task, step *domain.StepDefinition) (agent domain.Agent, model string) {
	// Start with task defaults
	agent = task.Config.Agent
	model = task.Config.Model

	// Apply step-level overrides if present
	if step.Config == nil {
		return agent, model
	}

	agentChanged := false
	if stepAgent, ok := step.Config["agent"].(string); ok && stepAgent != "" {
		newAgent := domain.Agent(stepAgent)
		// Only consider it a change if it's actually different
		if newAgent != agent {
			agent = newAgent
			agentChanged = true
		}
	}

	if stepModel, ok := step.Config["model"].(string); ok && stepModel != "" {
		model = stepModel
	} else if agentChanged {
		// Use new agent's default model when agent changed but model wasn't specified
		model = agent.DefaultModel()
	}

	return agent, model
}

// executeCurrentStep executes the step at task.CurrentStep and returns the result.
// It does not modify task state beyond what ExecuteStep does (step status, attempts, timing).
func (e *Engine) executeCurrentStep(ctx context.Context, task *domain.Task, template *domain.Template) (*domain.StepResult, error) {
	step := &template.Steps[task.CurrentStep]
	return e.ExecuteStep(ctx, task, step)
}

// executeParallelGroup runs multiple steps concurrently.
// It uses errgroup for coordinated cancellation - the first error
// cancels remaining steps in the group.
//
// stepIndices are the indices into template.Steps for the parallel group.
func (e *Engine) executeParallelGroup(ctx context.Context, task *domain.Task, template *domain.Template, stepIndices []int) ([]*domain.StepResult, error) {
	e.logger.Info().
		Str("task_id", task.ID).
		Int("parallel_count", len(stepIndices)).
		Msg("executing parallel step group")

	g, gctx := errgroup.WithContext(ctx)
	results := make([]*domain.StepResult, len(stepIndices))
	var mu sync.Mutex

	for i, idx := range stepIndices {
		step := &template.Steps[idx]

		g.Go(func() error {
			// Use internal method to avoid race on task.Steps
			result, err := e.executeStepInternal(gctx, task, step)

			// Always save result first - it may contain useful output even on error
			mu.Lock()
			results[i] = result
			mu.Unlock()

			if err != nil {
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return results, err
	}

	return results, nil
}

// handleSkippedStep marks a step as skipped and advances to the next step.
func (e *Engine) handleSkippedStep(ctx context.Context, task *domain.Task, step *domain.StepDefinition) error {
	// Determine skip reason for logging and output
	reason := e.getSkipReason(task, step)

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("reason", reason).
		Msg("skipping step")

	// Mark step as skipped
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = constants.StepStatusSkipped
	}

	// Record skipped result
	task.StepResults = append(task.StepResults, domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      constants.StepStatusSkipped,
		Output:      "Skipped - " + reason,
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})

	return e.advanceToNextStep(ctx, task)
}

// getSkipReason determines the reason a step is being skipped.
func (e *Engine) getSkipReason(task *domain.Task, step *domain.StepDefinition) string {
	if !step.Required {
		return "optional step not enabled"
	}

	// Check for no issues detected (fix template skipping AI/validation steps)
	if task.Metadata != nil {
		if noIssues, ok := task.Metadata["no_issues_detected"].(bool); ok && noIssues {
			if step.Type == domain.StepTypeAI || step.Type == domain.StepTypeValidation {
				return "no issues to fix"
			}
		}
	}

	return "no changes to push/PR"
}

// shouldSkipStep returns true if the step should be skipped.
// This skips:
// - git push and PR steps when "skip_git_steps" flag is set (no changes to commit)
// - AI and validation steps when "no_issues_detected" flag is set (detect_only found no issues)
// - steps with skip_condition that evaluates to true
func (e *Engine) shouldSkipStep(task *domain.Task, step *domain.StepDefinition) bool {
	// Check skip_condition first (for smart conditional steps)
	if skipCond, ok := step.Config["skip_condition"].(string); ok && skipCond != "" {
		if e.evaluateSkipCondition(task, skipCond) {
			return true
		}
	}

	if !step.Required {
		return true
	}
	if task.Metadata == nil {
		return false
	}
	return e.shouldSkipForNoIssues(task, step) || e.shouldSkipGitSteps(task, step)
}

// shouldSkipForNoIssues checks if step should be skipped when no issues were detected.
// AI steps are always skipped, validation steps are skipped unless they're detect-only.
func (e *Engine) shouldSkipForNoIssues(task *domain.Task, step *domain.StepDefinition) bool {
	noIssues, ok := task.Metadata["no_issues_detected"].(bool)
	if !ok || !noIssues {
		return false
	}

	// AI steps are always skipped when no issues detected
	if step.Type == domain.StepTypeAI {
		return true
	}

	// Validation steps are skipped unless they're detect-only
	if step.Type == domain.StepTypeValidation {
		detectOnly, _ := step.Config["detect_only"].(bool)
		return !detectOnly
	}

	// All other step types (Git, Human, SDD, CI, Verify) are never skipped
	return false
}

// shouldSkipGitSteps checks if git push/PR steps should be skipped (no changes to commit).
func (e *Engine) shouldSkipGitSteps(task *domain.Task, step *domain.StepDefinition) bool {
	skipGit, ok := task.Metadata["skip_git_steps"].(bool)
	if !ok || !skipGit || step.Type != domain.StepTypeGit {
		return false
	}
	return e.isSkippableGitOperation(step)
}

// isSkippableGitOperation returns true if the step is a push or create_pr operation.
func (e *Engine) isSkippableGitOperation(step *domain.StepDefinition) bool {
	op, ok := step.Config["operation"].(string)
	if !ok {
		return false
	}
	// These operation names match GitOpPush and GitOpCreatePR in steps/git.go
	return op == "push" || op == "create_pr"
}

// evaluateSkipCondition evaluates a skip_condition string and returns true if the step should be skipped.
// Supported conditions:
// - "has_description": skip if task has a substantive description (>20 chars)
// - "no_description": skip if task has no substantive description
func (e *Engine) evaluateSkipCondition(task *domain.Task, condition string) bool {
	switch condition {
	case "has_description":
		return e.hasSubstantiveDescription(task)
	case "no_description":
		return !e.hasSubstantiveDescription(task)
	default:
		// Unknown condition - don't skip
		return false
	}
}

// hasSubstantiveDescription returns true if the task has a description longer than 20 characters.
// This threshold distinguishes between short commands like "fix lint" and actual bug descriptions.
func (e *Engine) hasSubstantiveDescription(task *domain.Task) bool {
	desc := strings.TrimSpace(task.Description)
	return len(desc) > 20
}
