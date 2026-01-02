// Package task provides task lifecycle management for ATLAS.
//
// This file implements the TaskEngine, which orchestrates step execution
// through templates. The engine coordinates step executors, state transitions,
// and checkpointing.
//
// Import rules:
//   - CAN import: internal/constants, internal/domain, internal/errors, internal/template/steps, std lib
//   - MUST NOT import: internal/workspace, internal/ai, internal/cli
package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// EngineConfig holds configuration for the TaskEngine.
type EngineConfig struct {
	// AutoProceedGit controls whether git steps proceed automatically.
	// If false, engine pauses after git steps for user confirmation.
	AutoProceedGit bool

	// AutoProceedValidation controls whether validation steps proceed automatically.
	// Default is true (auto-proceed on success).
	AutoProceedValidation bool
}

// DefaultEngineConfig returns sensible defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		AutoProceedGit:        true,
		AutoProceedValidation: true,
	}
}

// Engine orchestrates task execution through template steps.
// It coordinates step executors, manages state transitions, and
// provides checkpointing after each step.
type Engine struct {
	store            Store
	registry         *steps.ExecutorRegistry
	config           EngineConfig
	logger           zerolog.Logger
	ciFailureHandler *CIFailureHandler
	notifier         *StateChangeNotifier
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithCIFailureHandler sets the CI failure handler for the engine.
func WithCIFailureHandler(handler *CIFailureHandler) EngineOption {
	return func(e *Engine) {
		e.ciFailureHandler = handler
	}
}

// WithNotifier sets the state change notifier for the engine.
// The notifier emits terminal bell notifications when tasks transition
// to attention-required states.
func WithNotifier(notifier *StateChangeNotifier) EngineOption {
	return func(e *Engine) {
		e.notifier = notifier
	}
}

// NewEngine creates a new task engine with the given dependencies.
// The store is used for task persistence, and the registry provides
// step executors for each step type. Optional EngineOption functions
// can be passed to configure additional features like CI failure handling.
func NewEngine(store Store, registry *steps.ExecutorRegistry, cfg EngineConfig, logger zerolog.Logger, opts ...EngineOption) *Engine {
	e := &Engine{
		store:    store,
		registry: registry,
		config:   cfg,
		logger:   logger,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Start creates and begins execution of a new task.
// It generates a unique task ID, creates the initial task state,
// transitions to Running, and begins step execution.
//
// The workspaceName identifies which workspace this task belongs to.
// The branch is the git branch name for this task (used by git operations).
// The template defines the steps to execute.
// The description provides a human-readable summary of the task.
//
// Returns the created task and any error that occurred during execution.
// Even if execution fails partway through, the task is returned so the
// caller can inspect its state.
func (e *Engine) Start(ctx context.Context, workspaceName, branch string, template *domain.Template, description string) (*domain.Task, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Generate unique task ID
	taskID := GenerateTaskID()

	now := time.Now().UTC()

	// Convert template steps to task steps
	taskSteps := make([]domain.Step, len(template.Steps))
	for i, def := range template.Steps {
		taskSteps[i] = domain.Step{
			Name:     def.Name,
			Type:     def.Type,
			Status:   "pending",
			Attempts: 0,
		}
	}

	task := &domain.Task{
		ID:            taskID,
		WorkspaceID:   workspaceName,
		TemplateID:    template.Name,
		Description:   description,
		Status:        constants.TaskStatusPending,
		CurrentStep:   0,
		Steps:         taskSteps,
		StepResults:   make([]domain.StepResult, 0),
		Transitions:   make([]domain.Transition, 0),
		CreatedAt:     now,
		UpdatedAt:     now,
		Config:        domain.TaskConfig{},
		SchemaVersion: constants.TaskSchemaVersion,
		Metadata: map[string]any{
			"branch": branch,
		},
	}

	e.logger.Info().
		Str("task_id", taskID).
		Str("workspace_name", workspaceName).
		Str("template_name", template.Name).
		Msg("creating new task")

	// Transition to Running
	if err := Transition(ctx, task, constants.TaskStatusRunning, "task started"); err != nil {
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	// Create task in store (initial persistence)
	if err := e.store.Create(ctx, workspaceName, task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	// Inject logger with task context for step executors
	ctx = e.injectLoggerContext(ctx, workspaceName, taskID)

	// Execute steps - pass template for step definitions
	if err := e.runSteps(ctx, task, template); err != nil {
		// Task state is already saved; return error for caller to handle
		return task, err
	}

	return task, nil
}

// Resume continues execution of a paused or failed task.
// It validates the task is in a resumable state, transitions back to Running
// if in an error state, and continues from the current step.
//
// The template must be provided to access step definitions.
//
// Returns an error if the task is in a terminal state (Completed, Rejected, Abandoned).
func (e *Engine) Resume(ctx context.Context, task *domain.Task, template *domain.Template) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Int("current_step", task.CurrentStep).
		Msg("resuming task")

	// Validate task is in resumable state
	if IsTerminalStatus(task.Status) {
		return fmt.Errorf("%w: cannot resume terminal task with status %s",
			atlaserrors.ErrInvalidTransition, task.Status)
	}

	// Transition from error states back to Running
	if IsErrorStatus(task.Status) {
		if err := Transition(ctx, task, constants.TaskStatusRunning, "resumed by user"); err != nil {
			return err
		}
		if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
			return fmt.Errorf("failed to save resumed state: %w", err)
		}
	}

	// Inject logger with task context for step executors
	ctx = e.injectLoggerContext(ctx, task.WorkspaceID, task.ID)

	// Continue from current step
	return e.runSteps(ctx, task, template)
}

// ExecuteStep executes a single step and returns the result.
// It retrieves the executor for the step type, logs timing information,
// and handles context cancellation.
//
// The step definition comes from the template (not the task's Step array).
// This method updates task.Steps[CurrentStep] status and is NOT safe for
// concurrent execution with the same task.
func (e *Engine) ExecuteStep(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Record start time
	startTime := time.Now()

	// Update task step status (only for sequential execution)
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = "running"
		now := startTime
		task.Steps[task.CurrentStep].StartedAt = &now
		task.Steps[task.CurrentStep].Attempts++
	}

	result, err := e.executeStepInternal(ctx, task, step)
	if err != nil {
		// Pass through result - it may contain useful output even on error
		return result, err
	}

	return result, nil
}

// HandleStepResult processes a step result and updates task state.
// It appends the result to history, and transitions the task based on
// the result status.
//
// For success: returns nil, allowing the caller to proceed.
// For awaiting_approval: transitions task to Validating then AwaitingApproval.
// For failed: transitions to the appropriate error state via valid path.
func (e *Engine) HandleStepResult(ctx context.Context, task *domain.Task, result *domain.StepResult, step *domain.StepDefinition) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Handle nil result - create minimal result for tracking
	if result == nil {
		result = &domain.StepResult{
			StepIndex: task.CurrentStep,
			StepName:  step.Name,
			Status:    "failed",
		}
	}

	// Append result to history
	task.StepResults = append(task.StepResults, *result)

	// Update task step status based on result
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = result.Status
		now := result.CompletedAt
		task.Steps[task.CurrentStep].CompletedAt = &now
		if result.Error != "" {
			task.Steps[task.CurrentStep].Error = result.Error
		}
	}

	switch result.Status {
	case constants.StepStatusSuccess:
		// Auto-proceed logic handled by caller (runSteps continues)
		return nil

	case constants.StepStatusNoChanges:
		// No changes were made (e.g., AI decided no modifications needed)
		// Set metadata flag to skip remaining git steps (push, PR)
		task.Metadata = e.ensureMetadata(task.Metadata)
		task.Metadata["skip_git_steps"] = true
		e.logger.Info().
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Msg("no changes to commit, will skip remaining git steps")
		return nil

	case constants.StepStatusAwaitingApproval:
		// For human steps, need to transition through Validating first
		// (Running -> Validating -> AwaitingApproval)
		oldStatus := task.Status
		if task.Status == constants.TaskStatusRunning {
			if err := Transition(ctx, task, constants.TaskStatusValidating, "step requires approval"); err != nil {
				return err
			}
		}
		if err := Transition(ctx, task, constants.TaskStatusAwaitingApproval, "awaiting user approval"); err != nil {
			return err
		}
		// Notify on transition to attention state
		e.notifyStateChange(oldStatus, constants.TaskStatusAwaitingApproval)
		return nil

	case constants.StepStatusFailed:
		// Store error context for retry (FR25)
		e.setErrorMetadata(task, step.Name, result.Error)

		// Check for specialized failure types (ci_failed, gh_failed, ci_timeout)
		// These have dedicated handlers with user action options
		if handled, err := e.DispatchFailureByType(ctx, task, result); handled {
			return err
		}

		// Map step type to error status with valid transition path
		return e.transitionToErrorState(ctx, task, step.Type, result.Error)

	case constants.StepStatusSkipped:
		// Step was intentionally skipped (e.g., CI step when no PR exists)
		// No further action needed, just allow continuation
		return nil

	default:
		return fmt.Errorf("%w: %s", atlaserrors.ErrUnknownStepResultStatus, result.Status)
	}
}

// Abandon terminates a task that is in an error state.
// The task is transitioned to Abandoned status, preserving all artifacts
// and the workspace worktree for manual work.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - task: The task to abandon (must be in an abandonable state)
//   - reason: Explanation for the abandonment
//
// Returns an error if:
//   - ctx is canceled
//   - task is nil
//   - task is not in an abandonable state
//   - state persistence fails
func (e *Engine) Abandon(ctx context.Context, task *domain.Task, reason string) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate task is not nil
	if task == nil {
		return fmt.Errorf("%w: task is nil", atlaserrors.ErrInvalidTransition)
	}

	log := e.logger.With().
		Str("task_id", task.ID).
		Str("workspace_name", task.WorkspaceID).
		Str("current_status", task.Status.String()).
		Logger()

	// Validate task can be abandoned
	if !CanAbandon(task.Status) {
		log.Warn().Msg("task not in abandonable state")
		return fmt.Errorf("%w: task status %s cannot be abandoned",
			atlaserrors.ErrInvalidTransition, task.Status)
	}

	// Transition to abandoned
	if err := Transition(ctx, task, constants.TaskStatusAbandoned, reason); err != nil {
		log.Error().Err(err).Msg("failed to transition task to abandoned")
		return err
	}

	// Save task state (artifacts and logs are preserved by default - we just save, never delete)
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		log.Error().Err(err).Msg("failed to save abandoned task")
		return fmt.Errorf("failed to save task: %w", err)
	}

	log.Info().
		Str("reason", reason).
		Msg("task abandoned successfully")

	return nil
}

// executeStepInternal executes a step without modifying task state.
// This is safe for concurrent execution in parallel step groups.
func (e *Engine) executeStepInternal(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get executor from registry
	executor, err := e.registry.Get(step.Type)
	if err != nil {
		return nil, fmt.Errorf("no executor for step type %s: %w", step.Type, err)
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing step")

	// Record start time
	startTime := time.Now()

	// Execute step via executor
	result, err := executor.Execute(ctx, task, step)

	// Calculate duration
	duration := time.Since(startTime)

	if err != nil {
		e.logger.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Int64("duration_ms", duration.Milliseconds()).
			Msg("step execution failed")
		// Return result WITH error - result may contain useful output (e.g., validation errors)
		return result, err
	}

	// Log completion
	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("status", result.Status).
		Int64("duration_ms", duration.Milliseconds()).
		Msg("step completed")

	return result, nil
}

// executeCurrentStep executes the step at task.CurrentStep and returns the result.
// It does not modify task state beyond what ExecuteStep does (step status, attempts, timing).
func (e *Engine) executeCurrentStep(ctx context.Context, task *domain.Task, template *domain.Template) (*domain.StepResult, error) {
	step := &template.Steps[task.CurrentStep]
	return e.ExecuteStep(ctx, task, step)
}

// processStepResult handles the result of a step execution.
// It delegates to HandleStepResult and saves state on error.
func (e *Engine) processStepResult(ctx context.Context, task *domain.Task, result *domain.StepResult, step *domain.StepDefinition) error {
	if err := e.HandleStepResult(ctx, task, result, step); err != nil {
		// Save state before returning error (best-effort, log if fails)
		if saveErr := e.store.Update(ctx, task.WorkspaceID, task); saveErr != nil {
			e.logger.Warn().
				Err(saveErr).
				Str("task_id", task.ID).
				Msg("failed to save task state during error handling")
		}
		return err
	}
	return nil
}

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
func (e *Engine) saveAndPause(ctx context.Context, task *domain.Task) error {
	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}
	e.logger.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Msg("task paused")
	return nil
}

// runSteps executes template steps in order, saving state after each.
// It checks for context cancellation between steps and pauses on
// awaiting approval or error states.
//
// This is a simple orchestration loop that delegates to focused helpers:
// - executeCurrentStep: executes the current step
// - processStepResult: handles the step result
// - saveAndPause: saves state when pausing
// - advanceToNextStep: increments step counter and checkpoints
func (e *Engine) runSteps(ctx context.Context, task *domain.Task, template *domain.Template) error {
	for task.CurrentStep < len(template.Steps) {
		if err := ctx.Err(); err != nil {
			return err
		}

		step := &template.Steps[task.CurrentStep]

		// Check if this step should be skipped (e.g., git push/PR when no changes)
		if e.shouldSkipStep(task, step) {
			if err := e.handleSkippedStep(ctx, task, step); err != nil {
				return err
			}
			continue
		}

		result, err := e.executeCurrentStep(ctx, task, template)
		if err != nil {
			return e.handleExecutionError(ctx, task, step, result, err)
		}

		if err := e.processStepResult(ctx, task, result, step); err != nil {
			return err
		}

		if e.shouldPause(task) {
			return e.saveAndPause(ctx, task)
		}

		if err := e.advanceToNextStep(ctx, task); err != nil {
			return err
		}
	}

	return e.completeTask(ctx, task)
}

// handleSkippedStep marks a step as skipped and advances to the next step.
func (e *Engine) handleSkippedStep(ctx context.Context, task *domain.Task, step *domain.StepDefinition) error {
	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Msg("skipping step - no changes to push/PR")

	// Mark step as skipped
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = constants.StepStatusSkipped
	}

	// Record skipped result
	task.StepResults = append(task.StepResults, domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      constants.StepStatusSkipped,
		Output:      "Skipped - no changes were made",
		StartedAt:   time.Now().UTC(),
		CompletedAt: time.Now().UTC(),
	})

	return e.advanceToNextStep(ctx, task)
}

// handleExecutionError handles errors from step execution.
func (e *Engine) handleExecutionError(ctx context.Context, task *domain.Task, step *domain.StepDefinition, result *domain.StepResult, err error) error {
	// Save step result first to preserve output (e.g., validation errors)
	if result != nil {
		task.StepResults = append(task.StepResults, *result)
	}
	return e.handleStepError(ctx, task, step, err)
}

// shouldSkipStep returns true if the step should be skipped.
// Currently, this skips git push and PR steps when the "skip_git_steps" flag is set,
// which happens when the commit step returns "no_changes" (AI made no modifications).
func (e *Engine) shouldSkipStep(task *domain.Task, step *domain.StepDefinition) bool {
	// Early return if no metadata
	if task.Metadata == nil {
		return false
	}

	// Check if git steps should be skipped
	skipGit, ok := task.Metadata["skip_git_steps"].(bool)
	if !ok || !skipGit {
		return false
	}

	// Only skip git push and PR operations
	if step.Type != domain.StepTypeGit {
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
	task.Metadata = e.ensureMetadata(task.Metadata)
	task.Metadata["last_error"] = errMsg
	task.Metadata["retry_context"] = e.buildRetryContext(task, &domain.StepResult{
		StepName: stepName,
		Error:    errMsg,
	})
}

// handleStepError handles an error from step execution.
// It transitions the task to the appropriate error state and saves.
func (e *Engine) handleStepError(ctx context.Context, task *domain.Task, step *domain.StepDefinition, err error) error {
	e.logger.Error().
		Err(err).
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Msg("step execution error")

	// Update task step status
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = "failed"
		task.Steps[task.CurrentStep].Error = err.Error()
		now := time.Now().UTC()
		task.Steps[task.CurrentStep].CompletedAt = &now
	}

	// Store error context for retry (FR25)
	e.setErrorMetadata(task, step.Name, err.Error())

	// Transition to error state following valid path
	if transErr := e.transitionToErrorState(ctx, task, step.Type, err.Error()); transErr != nil {
		return transErr
	}

	// Save state before returning
	if saveErr := e.store.Update(ctx, task.WorkspaceID, task); saveErr != nil {
		return fmt.Errorf("failed to save error state: %w", saveErr)
	}

	return err
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

	e.logger.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Msg("task awaiting approval")

	return nil
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
	case domain.StepTypeAI, domain.StepTypeHuman, domain.StepTypeSDD, domain.StepTypeVerify:
		// For AI, human, SDD, and verify failures, use ValidationFailed as general error
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
	return nil
}

// buildRetryContext creates a human-readable error summary for AI retry (FR25).
func (e *Engine) buildRetryContext(task *domain.Task, lastResult *domain.StepResult) string {
	var sb strings.Builder

	sb.WriteString("## Retry Context\n\n")
	sb.WriteString(fmt.Sprintf("**Task ID:** %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("**Current Step:** %d\n", task.CurrentStep))

	if lastResult != nil {
		sb.WriteString(fmt.Sprintf("**Failed Step:** %s\n", lastResult.StepName))
		if lastResult.Error != "" {
			sb.WriteString(fmt.Sprintf("**Error:** %s\n", lastResult.Error))
		}
	}

	sb.WriteString("\n### Previous Attempts\n\n")
	for i, result := range task.StepResults {
		if result.Status == "failed" {
			sb.WriteString(fmt.Sprintf("- Step %d (%s): %s\n", i, result.StepName, result.Error))
		}
	}

	return sb.String()
}

// ensureMetadata ensures the metadata map is initialized.
func (e *Engine) ensureMetadata(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	return m
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

// notifyStateChange emits a bell notification if the state transition warrants it.
// This is called after successful state transitions to attention-required states.
func (e *Engine) notifyStateChange(oldStatus, newStatus constants.TaskStatus) {
	if e.notifier != nil {
		e.notifier.NotifyStateChange(oldStatus, newStatus)
	}
}

// injectLoggerContext creates a context with an enriched logger containing
// workspace_name and task_id fields. Step executors can retrieve this logger
// using zerolog.Ctx(ctx) to automatically include these fields in all log entries.
func (e *Engine) injectLoggerContext(ctx context.Context, workspaceName, taskID string) context.Context {
	logger := e.logger.With().
		Str("workspace_name", workspaceName).
		Str("task_id", taskID).
		Logger()
	return logger.WithContext(ctx)
}
