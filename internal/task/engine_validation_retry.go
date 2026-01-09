// Package task provides task execution and lifecycle management.
package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/validation"
)

var (
	// ErrPipelineResultNotFound is returned when pipeline_result is not found in metadata.
	ErrPipelineResultNotFound = errors.New("pipeline_result not found in metadata")

	// ErrWorkDirNotFound is returned when work directory is not found for validation retry.
	ErrWorkDirNotFound = errors.New("work directory not found for validation retry")
)

// shouldAttemptValidationRetry checks if automatic retry should be attempted.
// Returns true if:
// - A validation retry handler is configured and enabled
// - The step result contains pipeline_result metadata for retry
func (e *Engine) shouldAttemptValidationRetry(result *domain.StepResult) bool {
	if e.validationRetryHandler == nil {
		e.logger.Debug().Msg("validation retry skipped: handler is nil")
		return false
	}
	if !e.validationRetryHandler.IsEnabled() {
		e.logger.Debug().Msg("validation retry skipped: handler is disabled")
		return false
	}
	if result == nil || result.Metadata == nil {
		e.logger.Debug().
			Bool("result_nil", result == nil).
			Msg("validation retry skipped: result or metadata is nil")
		return false
	}
	// Check if pipeline_result is available
	pipelineResult, ok := result.Metadata["pipeline_result"].(*validation.PipelineResult)
	if !ok {
		// Log the actual type for debugging
		actualValue := result.Metadata["pipeline_result"]
		actualType := "<nil>"
		if actualValue != nil {
			actualType = reflect.TypeOf(actualValue).String()
		}
		e.logger.Debug().
			Str("expected_type", "*validation.PipelineResult").
			Str("actual_type", actualType).
			Bool("value_nil", actualValue == nil).
			Msg("validation retry skipped: pipeline_result type assertion failed")
		return false
	}
	e.logger.Debug().
		Str("failed_step", pipelineResult.FailedStepName).
		Msg("validation retry conditions met")
	return true
}

// attemptValidationRetry performs the automatic validation retry loop.
// It extracts the PipelineResult from the failed step result's metadata,
// and invokes AI to fix the issues, retrying validation until success
// or max attempts are exhausted.
func (e *Engine) attemptValidationRetry(
	ctx context.Context,
	task *domain.Task,
	failedResult *domain.StepResult,
) (*validation.RetryResult, error) {
	// Extract pipeline result from metadata
	pipelineResult, ok := failedResult.Metadata["pipeline_result"].(*validation.PipelineResult)
	if !ok {
		e.logger.Debug().
			Str("task_id", task.ID).
			Msg("validation retry failed: pipeline_result not found in metadata")
		return nil, fmt.Errorf("task %s: %w", task.ID, ErrPipelineResultNotFound)
	}

	// Get work directory from task metadata
	workDir := e.getValidationWorkDir(task)
	if workDir == "" {
		e.logger.Debug().
			Str("task_id", task.ID).
			Msg("validation retry failed: worktree_dir not set in task metadata")
		return nil, fmt.Errorf("task %s: %w", task.ID, ErrWorkDirNotFound)
	}

	// Pre-flight check: verify worktree directory exists before attempting retry
	// This catches cases where the worktree was deleted (externally or by race condition)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		e.logger.Error().
			Str("task_id", task.ID).
			Str("work_dir", workDir).
			Msg("CRITICAL: worktree directory missing - cannot perform AI retry")
		return nil, fmt.Errorf("worktree directory missing: %s: %w", workDir, ErrWorkDirNotFound)
	}

	// Build runner config from task (nil uses defaults)
	runnerConfig := e.buildValidationRunnerConfig(task)

	maxAttempts := e.validationRetryHandler.MaxAttempts()
	currentAttempt := e.getValidationAttemptNumber(task)

	e.logger.Info().
		Str("task_id", task.ID).
		Int("current_attempt", currentAttempt).
		Int("max_attempts", maxAttempts).
		Str("failed_step", pipelineResult.FailedStepName).
		Msg("starting automatic validation retry")

	var lastResult *validation.RetryResult
	var lastErr error

	for attempt := currentAttempt; attempt <= maxAttempts; attempt++ {
		if err := ctxutil.Canceled(ctx); err != nil {
			return nil, err
		}

		if !e.validationRetryHandler.CanRetry(attempt) {
			break
		}

		e.logger.Info().
			Str("task_id", task.ID).
			Str("agent", string(task.Config.Agent)).
			Str("model", task.Config.Model).
			Int("attempt", attempt).
			Int("max_attempts", maxAttempts).
			Msg("attempting AI-assisted validation fix")

		// Notify progress callback about retry
		e.notifyRetryAttempt(task, attempt, maxAttempts)

		lastResult, lastErr = e.validationRetryHandler.RetryWithAI(
			ctx,
			pipelineResult,
			workDir,
			attempt,
			runnerConfig,
			task.Config.Agent,
			task.Config.Model,
		)

		if lastErr == nil && lastResult != nil && lastResult.Success {
			e.logger.Info().
				Str("task_id", task.ID).
				Str("agent", string(task.Config.Agent)).
				Str("model", task.Config.Model).
				Int("attempt", attempt).
				Int("files_changed", len(lastResult.AIResult.FilesChanged)).
				Msg("validation retry succeeded")

			// Update attempt counter in task metadata
			e.setValidationAttemptNumber(task, attempt)
			return lastResult, nil
		}

		// Update pipeline result for next iteration
		if lastResult != nil && lastResult.PipelineResult != nil {
			pipelineResult = lastResult.PipelineResult
		}

		e.logger.Warn().
			Str("task_id", task.ID).
			Str("agent", string(task.Config.Agent)).
			Str("model", task.Config.Model).
			Int("attempt", attempt).
			Err(lastErr).
			Msg("validation retry attempt failed")
	}

	e.logger.Error().
		Str("task_id", task.ID).
		Str("agent", string(task.Config.Agent)).
		Str("model", task.Config.Model).
		Int("attempts", maxAttempts).
		Msg("all validation retry attempts exhausted")

	return lastResult, lastErr
}

// getValidationAttemptNumber returns the current validation attempt number from task metadata.
func (e *Engine) getValidationAttemptNumber(task *domain.Task) int {
	if task.Metadata == nil {
		return 1
	}
	if attempt, ok := task.Metadata["validation_attempt"].(int); ok {
		return attempt + 1 // Next attempt
	}
	return 1
}

// setValidationAttemptNumber stores the validation attempt number in task metadata.
func (e *Engine) setValidationAttemptNumber(task *domain.Task, attempt int) {
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["validation_attempt"] = attempt
}

// getValidationWorkDir extracts the working directory for validation.
func (e *Engine) getValidationWorkDir(task *domain.Task) string {
	if task.Metadata != nil {
		if workDir, ok := task.Metadata["worktree_dir"].(string); ok {
			return workDir
		}
	}
	return ""
}

// buildValidationRunnerConfig creates runner config from task config.
// Returns nil to use defaults - the validation executor already has the commands.
func (e *Engine) buildValidationRunnerConfig(_ *domain.Task) *validation.RunnerConfig {
	return nil
}

// convertRetryResultToStepResult converts a successful retry result to a StepResult.
func (e *Engine) convertRetryResultToStepResult(
	task *domain.Task,
	step *domain.StepDefinition,
	retryResult *validation.RetryResult,
) *domain.StepResult {
	now := time.Now()
	startTime := now.Add(-time.Duration(retryResult.PipelineResult.DurationMs) * time.Millisecond)

	filesChanged := 0
	if retryResult.AIResult != nil {
		filesChanged = len(retryResult.AIResult.FilesChanged)
	}

	// Build validation checks from pipeline result for display in approval summary
	validationChecks := retryResult.PipelineResult.BuildChecksAsMap()

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: now,
		DurationMs:  retryResult.PipelineResult.DurationMs,
		Output:      fmt.Sprintf("Validation passed after AI retry (attempt %d, %d files changed)", retryResult.AttemptNumber, filesChanged),
		Metadata: map[string]any{
			"validation_checks": validationChecks,
			"pipeline_result":   retryResult.PipelineResult,
			"retry_attempt":     retryResult.AttemptNumber,
			"ai_files_changed":  filesChanged,
		},
	}
}

// notifyRetryAttempt sends a progress notification for retry attempts.
func (e *Engine) notifyRetryAttempt(task *domain.Task, attempt, maxAttempts int) {
	if e.config.ProgressCallback != nil {
		e.config.ProgressCallback(StepProgressEvent{
			Type:          "retry",
			TaskID:        task.ID,
			WorkspaceName: task.WorkspaceID,
			StepIndex:     task.CurrentStep,
			Status:        fmt.Sprintf("Retry attempt %d/%d", attempt, maxAttempts),
		})
	}
}
