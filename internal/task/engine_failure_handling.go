// Package task provides task lifecycle management for ATLAS.
//
// This file implements failure handling methods for the TaskEngine,
// including CI failure, GitHub failure, and timeout handling.
//
// Import rules:
//   - CAN import: internal/constants, internal/domain, internal/errors, internal/git, std lib
//   - MUST NOT import: internal/workspace, internal/cli, internal/tui
package task

import (
	"context"
	"fmt"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// DispatchFailureByType checks the failure_type in result metadata and calls the appropriate handler.
// Returns (handled bool, err error) where handled=true means a specialized handler was invoked.
// If handled=false, the caller should use default failure handling.
func (e *Engine) DispatchFailureByType(ctx context.Context, task *domain.Task, result *domain.StepResult) (bool, error) {
	if result.Metadata == nil {
		return false, nil
	}

	failureType, ok := result.Metadata["failure_type"].(string)
	if !ok || failureType == "" {
		return false, nil
	}

	// Extract CI result if present (for CI-related failures)
	var ciResult *git.CIWatchResult
	if r, ok := result.Metadata["ci_result"].(*git.CIWatchResult); ok {
		ciResult = r
	}

	// Each handler returns error or nil; we always return handled=true when a known type is matched
	switch failureType {
	case "ci_failed":
		return true, e.handleCIFailure(ctx, task, result, ciResult)
	case "ci_timeout":
		return true, e.handleCITimeout(ctx, task, result, ciResult)
	case "gh_failed":
		return true, e.handleGHFailure(ctx, task, result)
	case "ci_fetch_error":
		return true, e.handleCIFetchErrorFailure(ctx, task, result)
	default:
		// Unknown failure type, fall back to default handling
		return false, nil
	}
}

// handleCIFailure handles CI check failures.
// It transitions the task to CIFailed state and stores failure context.
func (e *Engine) handleCIFailure(ctx context.Context, task *domain.Task, result *domain.StepResult, ciResult *git.CIWatchResult) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("CI failure handling canceled: %w", err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("workspace", task.WorkspaceID).
		Msg("handling CI failure")

	oldStatus := task.Status

	// Transition to CIFailed state
	if err := Transition(ctx, task, constants.TaskStatusCIFailed, "CI checks failed"); err != nil {
		return fmt.Errorf("failed to transition to ci_failed: %w", err)
	}

	// Notify on transition to attention state
	e.notifyStateChange(oldStatus, constants.TaskStatusCIFailed)

	// Update hook state to reflect CI failure (recoverable via resume)
	e.failHookStep(ctx, task, result.StepName, fmt.Errorf("%w: %s", atlaserrors.ErrCIFailed, result.Error))

	// Store failure context for action processing
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["ci_failure_result"] = ciResult
	task.Metadata["last_error"] = result.Error

	// Save task state
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// handleGHFailure handles GitHub operation failures (push, PR creation).
// It transitions the task to GHFailed state.
func (e *Engine) handleGHFailure(ctx context.Context, task *domain.Task, result *domain.StepResult) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("GitHub failure handling canceled: %w", err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("workspace", task.WorkspaceID).
		Msg("handling GitHub failure")

	oldStatus := task.Status

	// Transition to GHFailed state
	if err := Transition(ctx, task, constants.TaskStatusGHFailed, result.Error); err != nil {
		return fmt.Errorf("failed to transition to gh_failed: %w", err)
	}

	// Notify on transition to attention state
	e.notifyStateChange(oldStatus, constants.TaskStatusGHFailed)

	// Update hook state to reflect GitHub failure (recoverable via resume)
	e.failHookStep(ctx, task, result.StepName, fmt.Errorf("%w: %s", atlaserrors.ErrGitHubOperation, result.Error))

	// Store error context
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["last_error"] = result.Error

	// Extract and store push error type for recovery UI
	// Error format: "gh_failed: <error_type>" (e.g., "gh_failed: non_fast_forward")
	if result.Error != "" {
		errorType := extractPushErrorType(result.Error)
		if errorType != "" {
			task.Metadata["push_error_type"] = errorType
		}
	}

	// Save task state
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// handleCITimeout handles CI monitoring timeout.
// It transitions the task to CITimeout state.
func (e *Engine) handleCITimeout(ctx context.Context, task *domain.Task, result *domain.StepResult, ciResult *git.CIWatchResult) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("CI timeout handling canceled: %w", err)
	}

	logger := e.logger.Info().
		Str("task_id", task.ID).
		Str("workspace", task.WorkspaceID)
	if ciResult != nil {
		logger = logger.Dur("elapsed", ciResult.ElapsedTime)
	}
	logger.Msg("handling CI timeout")

	oldStatus := task.Status

	// Transition to CITimeout state
	if err := Transition(ctx, task, constants.TaskStatusCITimeout, "CI monitoring timed out"); err != nil {
		return fmt.Errorf("failed to transition to ci_timeout: %w", err)
	}

	// Notify on transition to attention state
	e.notifyStateChange(oldStatus, constants.TaskStatusCITimeout)

	// Update hook state to reflect CI timeout (recoverable via resume)
	e.failHookStep(ctx, task, result.StepName, fmt.Errorf("%w: %s", atlaserrors.ErrCITimeout, result.Error))

	// Store timeout context
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["ci_timeout_result"] = ciResult
	task.Metadata["last_error"] = result.Error

	// Save task state
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// handleCIFetchErrorFailure handles CI status fetch failures (network issues, rate limits).
// Unlike CI failures, this doesn't mean CI failed - we just couldn't verify the status.
// Transitions to AwaitingApproval to allow user to decide how to proceed.
func (e *Engine) handleCIFetchErrorFailure(ctx context.Context, task *domain.Task, result *domain.StepResult) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("CI fetch error handling canceled: %w", err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("workspace", task.WorkspaceID).
		Msg("handling CI fetch error - transitioning to awaiting approval")

	oldStatus := task.Status

	// Transition through Validating to AwaitingApproval (not to a failure state)
	// This allows the user to check GitHub manually and decide
	if task.Status == constants.TaskStatusRunning {
		if err := Transition(ctx, task, constants.TaskStatusValidating, "CI fetch failed - awaiting user decision"); err != nil {
			return fmt.Errorf("failed to transition to validating: %w", err)
		}
	}

	if err := Transition(ctx, task, constants.TaskStatusAwaitingApproval, "CI status could not be verified"); err != nil {
		return fmt.Errorf("failed to transition to awaiting_approval: %w", err)
	}

	// Notify on transition to attention state
	e.notifyStateChange(oldStatus, constants.TaskStatusAwaitingApproval)

	// Store error context for user decision
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["ci_fetch_error"] = true
	if result.Metadata != nil {
		if originalErr, ok := result.Metadata["original_error"].(string); ok {
			task.Metadata["last_error"] = originalErr
		}
	}

	// Save task state
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save task state: %w", err)
	}

	return nil
}

// ProcessCIFailureAction processes user's CI failure action choice.
func (e *Engine) ProcessCIFailureAction(ctx context.Context, task *domain.Task, action CIFailureAction) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("processing CI failure action canceled: %w", err)
	}

	if e.ciFailureHandler == nil {
		return fmt.Errorf("CI failure handler not configured: %w", atlaserrors.ErrExecutorNotFound)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("action", action.String()).
		Msg("processing CI failure action")

	// Get stored CI result
	var ciResult *git.CIWatchResult
	if task.Metadata != nil {
		if r, ok := task.Metadata["ci_failure_result"].(*git.CIWatchResult); ok {
			ciResult = r
		}
	}

	// Get PR number
	prNumber := e.extractPRNumber(task)

	// Get worktree path
	worktreePath := ""
	if task.Metadata != nil {
		if p, ok := task.Metadata["worktree_dir"].(string); ok {
			worktreePath = p
		}
	}

	// Build handler options
	opts := CIFailureOptions{
		Action:        action,
		PRNumber:      prNumber,
		CIResult:      ciResult,
		WorktreePath:  worktreePath,
		WorkspaceName: task.WorkspaceID,
	}

	result, err := e.ciFailureHandler.HandleCIFailure(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to handle CI failure action: %w", err)
	}

	// Process handler result based on action
	return e.processCIFailureResult(ctx, task, action, result)
}

// processCIFailureResult handles the result of a CI failure action.
func (e *Engine) processCIFailureResult(ctx context.Context, task *domain.Task, action CIFailureAction, result *CIFailureResult) error {
	task.Metadata = e.ensureMetadata(task.Metadata)

	switch action {
	case CIFailureViewLogs:
		// Browser opened, task remains in CIFailed (return to options menu)
		return nil

	case CIFailureRetryImplement:
		// Transition back to running and restart from implement
		implementStep := e.findImplementStep(task)
		task.CurrentStep = implementStep
		task.Metadata["retry_context"] = result.ErrorContext
		if err := Transition(ctx, task, constants.TaskStatusRunning, "retry from implement"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)

	case CIFailureFixManually:
		// Store instructions; task remains in CIFailed
		task.Metadata["manual_fix_instructions"] = result.Message
		return e.store.Update(ctx, task.WorkspaceID, task)

	case CIFailureAbandon:
		// Transition to abandoned
		if err := Transition(ctx, task, constants.TaskStatusAbandoned, "user abandoned after CI failure"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)
	}

	return nil
}

// ProcessGHFailureAction processes user's GitHub failure action choice.
func (e *Engine) ProcessGHFailureAction(ctx context.Context, task *domain.Task, action GHFailureAction) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("processing GitHub failure action canceled: %w", err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("action", action.String()).
		Msg("processing GH failure action")

	task.Metadata = e.ensureMetadata(task.Metadata)

	switch action {
	case GHFailureRetry:
		// Transition back to running to retry current step
		if err := Transition(ctx, task, constants.TaskStatusRunning, "retry GitHub operation"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)

	case GHFailureFixAndRetry:
		// Store instructions; task remains in GHFailed for manual fix
		task.Metadata["awaiting_manual_fix"] = true
		return e.store.Update(ctx, task.WorkspaceID, task)

	case GHFailureAbandon:
		// Transition to abandoned
		if err := Transition(ctx, task, constants.TaskStatusAbandoned, "user abandoned after GitHub failure"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)
	}

	return nil
}

// ProcessCITimeoutAction processes user's CI timeout action choice.
func (e *Engine) ProcessCITimeoutAction(ctx context.Context, task *domain.Task, action CITimeoutAction) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return fmt.Errorf("processing CI timeout action canceled: %w", err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("action", action.String()).
		Msg("processing CI timeout action")

	task.Metadata = e.ensureMetadata(task.Metadata)

	switch action {
	case CITimeoutContinueWaiting:
		// Transition back to running; flag extended timeout
		task.Metadata["extended_ci_timeout"] = true
		if err := Transition(ctx, task, constants.TaskStatusRunning, "continue waiting for CI"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)

	case CITimeoutRetry:
		// Transition back to running and restart from implement
		implementStep := e.findImplementStep(task)
		task.CurrentStep = implementStep
		if err := Transition(ctx, task, constants.TaskStatusRunning, "retry from implement after timeout"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)

	case CITimeoutFixManually:
		// Store instructions; task remains in CITimeout for manual fix
		task.Metadata["awaiting_manual_fix"] = true
		return e.store.Update(ctx, task.WorkspaceID, task)

	case CITimeoutAbandon:
		// Transition to abandoned
		if err := Transition(ctx, task, constants.TaskStatusAbandoned, "user abandoned after CI timeout"); err != nil {
			return err
		}
		return e.store.Update(ctx, task.WorkspaceID, task)
	}

	return nil
}

// findImplementStep finds the index of the "implement" step in the task.
// Falls back to first AI step if no explicit "implement" step exists.
func (e *Engine) findImplementStep(task *domain.Task) int {
	// First, look for a step named "implement"
	for i, step := range task.Steps {
		if step.Name == "implement" {
			return i
		}
	}

	// Fall back to first AI step
	for i, step := range task.Steps {
		if step.Type == domain.StepTypeAI {
			return i
		}
	}

	// Default to 0 if nothing found
	return 0
}

// extractPRNumber extracts the PR number from task metadata.
func (e *Engine) extractPRNumber(task *domain.Task) int {
	if task.Metadata == nil {
		return 0
	}

	prNumber, ok := task.Metadata["pr_number"]
	if !ok {
		return 0
	}

	switch v := prNumber.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// extractPushErrorType extracts the push error type from an error string.
// Expected format: "gh_failed: <error_type>" (e.g., "gh_failed: non_fast_forward")
// Returns the error type (e.g., "non_fast_forward") or empty string if not found.
func extractPushErrorType(errStr string) string {
	const prefix = "gh_failed: "
	if len(errStr) > len(prefix) && errStr[:len(prefix)] == prefix {
		return errStr[len(prefix):]
	}
	return ""
}
