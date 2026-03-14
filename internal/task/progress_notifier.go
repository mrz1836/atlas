// Package task provides task lifecycle management for ATLAS.
//
// This file contains progress notification and metrics logic extracted from engine.go.
// ProgressNotifier methods handle progress callbacks, metrics recording,
// and hook manager notifications.
package task

import (
	"context"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// notifyStateChange emits a bell notification if the state transition warrants it.
// This is called after successful state transitions to attention-required states.
func (e *Engine) notifyStateChange(oldStatus, newStatus constants.TaskStatus) {
	if e.notifier != nil {
		e.notifier.NotifyStateChange(oldStatus, newStatus)
	}
}

// notifyStepStart calls the progress callback with a "start" event if configured.
func (e *Engine) notifyStepStart(task *domain.Task, step *domain.StepDefinition, totalSteps int) {
	if e.config.ProgressCallback == nil {
		return
	}

	event := StepProgressEvent{
		Type:          "start",
		TaskID:        task.ID,
		WorkspaceName: task.WorkspaceID,
		StepIndex:     task.CurrentStep,
		TotalSteps:    totalSteps,
		StepName:      step.Name,
		StepType:      step.Type,
	}

	// Add agent/model for AI steps
	if step.Type == domain.StepTypeAI || step.Type == domain.StepTypeVerify {
		agent, model := ResolveStepAgentModel(task, step)
		event.Agent = string(agent)
		event.Model = model
	}

	e.config.ProgressCallback(event)
}

// notifyStepComplete calls the progress callback with a "complete" event if configured.
func (e *Engine) notifyStepComplete(task *domain.Task, step *domain.StepDefinition, result *domain.StepResult, totalSteps int) {
	if e.config.ProgressCallback == nil {
		return
	}

	event := StepProgressEvent{
		Type:          "complete",
		TaskID:        task.ID,
		WorkspaceName: task.WorkspaceID,
		StepIndex:     task.CurrentStep,
		TotalSteps:    totalSteps,
		StepName:      step.Name,
		StepType:      step.Type,
		Status:        result.Status,
		Output:        result.Output,
	}

	// Add agent/model for AI steps
	if step.Type == domain.StepTypeAI || step.Type == domain.StepTypeVerify {
		agent, model := ResolveStepAgentModel(task, step)
		event.Agent = string(agent)
		event.Model = model
	}

	// Add completion metrics
	event.DurationMs = result.DurationMs
	event.NumTurns = result.NumTurns
	event.FilesChangedCount = len(result.FilesChanged)

	e.config.ProgressCallback(event)
}

// recordTaskStarted reports task start to metrics collector if configured.
func (e *Engine) recordTaskStarted(taskID, templateName string) {
	if e.metrics != nil {
		e.metrics.TaskStarted(taskID, templateName)
	}
}

// recordTaskCompleted reports task completion to metrics collector if configured.
func (e *Engine) recordTaskCompleted(taskID string, duration time.Duration, status string) {
	if e.metrics != nil {
		e.metrics.TaskCompleted(taskID, duration, status)
	}
}

// recordStepExecuted reports step execution to metrics collector if configured.
func (e *Engine) recordStepExecuted(taskID, stepName string, stepType domain.StepType, duration time.Duration, success bool) {
	if e.metrics != nil {
		e.metrics.StepExecuted(taskID, stepName, stepType, duration, success)
	}
}

// transitionHookStep updates the hook when entering a step.
func (e *Engine) transitionHookStep(ctx context.Context, task *domain.Task, stepName string, stepIndex int) {
	if e.hookManager != nil {
		if err := e.hookManager.TransitionStep(ctx, task, stepName, stepIndex); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to update hook step state")
		}

		// Start interval checkpointing for long-running steps
		if err := e.hookManager.StartIntervalCheckpointing(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to start interval checkpointing")
		}
	}
}

// completeHookStep updates the hook when a step completes successfully.
func (e *Engine) completeHookStep(ctx context.Context, task *domain.Task, stepName string, filesChanged []string) {
	if e.hookManager != nil {
		// Stop interval checkpointing
		if err := e.hookManager.StopIntervalCheckpointing(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Msg("failed to stop interval checkpointing")
		}

		if err := e.hookManager.CompleteStep(ctx, task, stepName, filesChanged); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to update hook step completion")
		}
	}
}

// failHookStep updates the hook when a step fails.
func (e *Engine) failHookStep(ctx context.Context, task *domain.Task, stepName string, stepErr error) {
	if e.hookManager != nil {
		// Stop interval checkpointing
		if err := e.hookManager.StopIntervalCheckpointing(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Msg("failed to stop interval checkpointing")
		}

		if err := e.hookManager.FailStep(ctx, task, stepName, stepErr); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to update hook step failure")
		}
	}
}

// interruptHookStep updates the hook when a step is interrupted by user (Ctrl+C).
// This transitions the hook to awaiting_human state so resume can work properly.
func (e *Engine) interruptHookStep(ctx context.Context, task *domain.Task, stepName string) {
	if e.hookManager != nil {
		// Stop interval checkpointing
		if err := e.hookManager.StopIntervalCheckpointing(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Msg("failed to stop interval checkpointing")
		}

		if err := e.hookManager.InterruptStep(ctx, task, stepName); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to update hook interrupt state")
		}
	}
}

// completeHookTask finalizes the hook when the task completes.
func (e *Engine) completeHookTask(ctx context.Context, task *domain.Task) {
	if e.hookManager != nil {
		if err := e.hookManager.CompleteTask(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Msg("failed to finalize hook on task completion")
		}
	}
}

// failHookTask updates the hook when the task fails or is interrupted.
func (e *Engine) failHookTask(ctx context.Context, task *domain.Task, taskErr error) {
	if e.hookManager == nil {
		return
	}
	if err := e.hookManager.FailTask(ctx, task, taskErr); err != nil {
		e.logger.Warn().Err(err).
			Str("task_id", task.ID).
			Msg("failed to update hook task failure")
	}
}
