// Package task provides task lifecycle management for ATLAS.
//
// This file contains state management logic extracted from engine.go.
// StateManager methods handle task state transitions, checkpointing,
// metadata management, and error state handling.
package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// advanceToNextStep increments the step counter, updates timestamp, and saves a checkpoint.
func (e *Engine) advanceToNextStep(ctx context.Context, task *domain.Task) error {
	task.CurrentStep++
	task.UpdatedAt = time.Now().UTC()

	// Save checkpoint
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}
	return nil
}

// saveAndPause saves the task state and logs that the task is paused.
// It also performs cleanup of step resources (e.g., git lock files).
func (e *Engine) saveAndPause(ctx context.Context, task *domain.Task) error {
	// Cleanup step resources before pausing (e.g., remove stale git lock files)
	e.cleanupOnPause(ctx, task)

	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}
	e.logger.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Msg("task paused")
	return nil
}

// cleanupOnPause performs cleanup of step-specific resources when task pauses.
// This prevents issues like stale git lock files when resuming.
func (e *Engine) cleanupOnPause(ctx context.Context, task *domain.Task) {
	// Get worktree path from task metadata
	worktreePath := ""
	if task.Metadata != nil {
		if path, ok := task.Metadata["worktree_dir"].(string); ok {
			worktreePath = path
		}
	}

	// Try to cleanup git lock files if we have a git executor
	if gitExec, err := e.registry.Get(domain.StepTypeGit); err == nil {
		// Check if it has CleanupOnPause method via interface type assertion
		if cleaner, ok := gitExec.(interface {
			CleanupOnPause(ctx context.Context, worktreePath string) error
		}); ok {
			if err := cleaner.CleanupOnPause(ctx, worktreePath); err != nil {
				e.logger.Warn().Err(err).Msg("failed to cleanup git resources on pause")
			}
		}
	}
}

// shouldPause returns true if the task should pause execution.
// This happens when waiting for user approval or when in an error state.
func (e *Engine) shouldPause(task *domain.Task) bool {
	return task.Status == constants.TaskStatusAwaitingApproval ||
		IsErrorStatus(task.Status)
}

// setErrorMetadata sets consistent error context in task metadata for retry (FR25).
// This consolidates error metadata setting that was previously duplicated in
// handleStepError and HandleStepResult.
func (e *Engine) setErrorMetadata(task *domain.Task, stepName, errMsg string) {
	e.setMetadataMultiple(task, map[string]any{
		"last_error": errMsg,
		"retry_context": e.buildRetryContext(task, &domain.StepResult{
			StepName: stepName,
			Error:    errMsg,
		}),
	})
}

// ensureMetadata ensures the metadata map is initialized.
func (e *Engine) ensureMetadata(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	return m
}

// setMetadata sets a single metadata key-value pair on the task.
// It ensures the metadata map is initialized before setting the value.
func (e *Engine) setMetadata(task *domain.Task, key string, value any) {
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata[key] = value
}

// setMetadataMultiple sets multiple metadata key-value pairs on the task.
// It ensures the metadata map is initialized once before setting all values.
func (e *Engine) setMetadataMultiple(task *domain.Task, values map[string]any) {
	task.Metadata = e.ensureMetadata(task.Metadata)
	for k, v := range values {
		task.Metadata[k] = v
	}
}

// mapStepTypeToErrorStatus maps a step type to the appropriate error status.
func (e *Engine) mapStepTypeToErrorStatus(stepType domain.StepType) constants.TaskStatus {
	switch stepType {
	case domain.StepTypeValidation:
		return constants.TaskStatusValidationFailed
	case domain.StepTypeGit:
		return constants.TaskStatusGHFailed
	case domain.StepTypeCI:
		return constants.TaskStatusCIFailed
	case domain.StepTypeAI, domain.StepTypeHuman, domain.StepTypeSDD, domain.StepTypeVerify, domain.StepTypeLoop:
		// For AI, human, SDD, verify, and loop failures, use ValidationFailed as general error
		return constants.TaskStatusValidationFailed
	}
	// Unreachable with current step types, but satisfy exhaustive check
	return constants.TaskStatusValidationFailed
}

// requiresValidatingIntermediate returns true if transitioning to ValidationFailed
// requires going through Validating first (from Running state).
func (e *Engine) requiresValidatingIntermediate(currentStatus, targetStatus constants.TaskStatus) bool {
	return currentStatus == constants.TaskStatusRunning &&
		targetStatus == constants.TaskStatusValidationFailed
}

// transitionToErrorState transitions the task to the appropriate error state
// following valid state machine paths.
func (e *Engine) transitionToErrorState(ctx context.Context, task *domain.Task, stepType domain.StepType, reason string) error {
	targetStatus := e.mapStepTypeToErrorStatus(stepType)
	oldStatus := task.Status

	// From Running, ValidationFailed requires intermediate Validating state
	if e.requiresValidatingIntermediate(task.Status, targetStatus) {
		if err := Transition(ctx, task, constants.TaskStatusValidating, "step failed"); err != nil {
			return err
		}
	}

	if err := Transition(ctx, task, targetStatus, reason); err != nil {
		return err
	}

	// Notify on transition to attention/error state
	e.notifyStateChange(oldStatus, targetStatus)

	// Update hook state to reflect step failure (awaiting_human, recoverable)
	// We use failHookStep instead of failHookTask because these error states
	// (gh_failed, ci_failed, validation_failed) are recoverable via resume
	stepName := ""
	if task.CurrentStep >= 0 && task.CurrentStep < len(task.Steps) {
		stepName = task.Steps[task.CurrentStep].Name
	}
	e.failHookStep(ctx, task, stepName, fmt.Errorf("%w: %s", atlaserrors.ErrTaskFailed, reason))

	return nil
}

// completeTask transitions the task to the appropriate final state.
// For most templates, this transitions through Validating to AwaitingApproval.
func (e *Engine) completeTask(ctx context.Context, task *domain.Task) error {
	e.logger.Info().
		Str("task_id", task.ID).
		Msg("all steps completed, transitioning to validation")

	oldStatus := task.Status

	// Transition to Validating
	if err := Transition(ctx, task, constants.TaskStatusValidating, "all steps completed"); err != nil {
		return err
	}

	// Then to AwaitingApproval (validation passed)
	if err := Transition(ctx, task, constants.TaskStatusAwaitingApproval, "validation passed"); err != nil {
		return err
	}

	// Notify on transition to attention state
	e.notifyStateChange(oldStatus, constants.TaskStatusAwaitingApproval)

	// Save final state
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save completed state: %w", err)
	}

	// Finalize hook on task completion
	e.completeHookTask(ctx, task)

	// Record task completion for metrics
	e.recordTaskCompleted(task.ID, time.Since(task.CreatedAt), string(task.Status))

	e.logger.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Msg("task awaiting approval")

	return nil
}

// applyStepApprovalChoice applies the user's step-level approval choice.
// This is called by Resume when the user has made a choice during step-level approval.
func (e *Engine) applyStepApprovalChoice(ctx context.Context, task *domain.Task, template *domain.Template, choice string) error {
	if task.CurrentStep >= len(template.Steps) {
		return fmt.Errorf("invalid current step index %d: %w", task.CurrentStep, atlaserrors.ErrInvalidArgument)
	}

	step := &template.Steps[task.CurrentStep]

	// Handle git commit garbage choices
	if step.Type == domain.StepTypeGit {
		operation, _ := step.Config["operation"].(string)
		if operation == "" || operation == "commit" {
			return e.applyGitGarbageChoice(ctx, task, choice)
		}
	}

	return nil
}

// applyGitGarbageChoice handles garbage file choices for git commit operations.
func (e *Engine) applyGitGarbageChoice(_ context.Context, task *domain.Task, choice string) error {
	switch choice {
	case "r":
		// Remove garbage files from staging - set metadata flag for commit to skip garbage
		e.setMetadata(task, "garbage_action", "remove")
		e.logger.Info().Str("task_id", task.ID).Msg("garbage files will be removed from staging")

	case "i":
		// Include garbage files anyway - set metadata flag for commit to include them
		e.setMetadata(task, "garbage_action", "include")
		e.logger.Info().Str("task_id", task.ID).Msg("garbage files will be included in commit")

	case "a":
		// Abort - transition to an error state
		e.logger.Info().Str("task_id", task.ID).Msg("user chose to abort garbage handling")
		return fmt.Errorf("user aborted: garbage files require manual intervention: %w", atlaserrors.ErrOperationCanceled)

	default:
		return fmt.Errorf("unknown garbage handling choice %q: %w", choice, atlaserrors.ErrInvalidArgument)
	}

	return nil
}

// buildRetryContext creates a human-readable error summary for AI retry (FR25).
func (e *Engine) buildRetryContext(task *domain.Task, lastResult *domain.StepResult) string {
	var sb strings.Builder

	sb.WriteString("## Retry Context\n\n")
	fmt.Fprintf(&sb, "**Task ID:** %s\n", task.ID)
	fmt.Fprintf(&sb, "**Current Step:** %d\n", task.CurrentStep)

	if lastResult != nil {
		fmt.Fprintf(&sb, "**Failed Step:** %s\n", lastResult.StepName)
		if lastResult.Error != "" {
			fmt.Fprintf(&sb, "**Error:** %s\n", lastResult.Error)
		}
	}

	sb.WriteString("\n### Previous Attempts\n\n")
	for i, result := range task.StepResults {
		if result.Status == "failed" {
			fmt.Fprintf(&sb, "- Step %d (%s): %s\n", i, result.StepName, result.Error)
		}
	}

	return sb.String()
}
