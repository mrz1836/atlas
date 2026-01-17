// Package task provides task lifecycle management for ATLAS.
//
// This file implements the TaskEngine, which orchestrates step execution
// through templates. The engine coordinates step executors, state transitions,
// and checkpointing.
//
// # Concurrency Model
//
// Task objects are NOT safe for concurrent modification. The Engine processes
// steps sequentially, updating task state (Steps, StepResults, CurrentStep)
// after each step completes. When parallel step execution is needed, use
// executeStepInternal which does not modify shared task state.
//
// The Engine itself is safe for concurrent use across different tasks -
// each task maintains its own state. However, a single task instance
// must not be processed by multiple goroutines simultaneously.
//
// # Import rules
//
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
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// StepProgressEvent contains information about step execution progress.
// Used by StepProgressCallback to provide UI feedback during task execution.
type StepProgressEvent struct {
	// Type is "start" when step begins, "complete" when step finishes.
	Type string

	// Task information
	TaskID        string
	WorkspaceName string

	// Step information
	StepIndex  int
	TotalSteps int
	StepName   string
	StepType   domain.StepType

	// Agent and model for AI/verify steps (empty for other step types).
	Agent string
	Model string

	// Completion metrics (only populated for "complete" events).
	DurationMs        int64
	NumTurns          int
	FilesChangedCount int
	Status            string
	Output            string // PR URL or other relevant output
}

// StepProgressCallback is called before and after each step execution.
// UI components can use this to show spinners, progress bars, and completion summaries.
type StepProgressCallback func(event StepProgressEvent)

// EngineConfig holds configuration for the TaskEngine.
type EngineConfig struct {
	// AutoProceedGit controls whether git steps proceed automatically.
	// If false, engine pauses after git steps for user confirmation.
	AutoProceedGit bool

	// AutoProceedValidation controls whether validation steps proceed automatically.
	// Default is true (auto-proceed on success).
	AutoProceedValidation bool

	// ProgressCallback is called before and after each step execution.
	// If nil, no progress callbacks are made.
	ProgressCallback StepProgressCallback
}

// DefaultEngineConfig returns sensible defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		AutoProceedGit:        true,
		AutoProceedValidation: true,
	}
}

// HookManager provides an interface for managing task recovery hooks.
// Implementations handle hook lifecycle events: creation, step transitions, and completion.
type HookManager interface {
	// CreateHook initializes a hook for a new task.
	CreateHook(ctx context.Context, task *domain.Task) error

	// TransitionStep updates the hook when entering a step.
	TransitionStep(ctx context.Context, task *domain.Task, stepName string, stepIndex int) error

	// CompleteStep updates the hook when a step completes successfully.
	CompleteStep(ctx context.Context, task *domain.Task, stepName string) error

	// FailStep updates the hook when a step fails.
	FailStep(ctx context.Context, task *domain.Task, stepName string, err error) error

	// CompleteTask finalizes the hook when the task completes.
	CompleteTask(ctx context.Context, task *domain.Task) error

	// FailTask updates the hook when the task fails.
	FailTask(ctx context.Context, task *domain.Task, err error) error
}

// Engine orchestrates task execution through template steps.
// It coordinates step executors, manages state transitions, and
// provides checkpointing after each step.
type Engine struct {
	store                  Store
	registry               *steps.ExecutorRegistry
	config                 EngineConfig
	logger                 zerolog.Logger
	ciFailureHandler       *CIFailureHandler
	notifier               *StateChangeNotifier
	validationRetryHandler ValidationRetryHandler
	metrics                Metrics
	hookManager            HookManager
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

// WithValidationRetryHandler sets the validation retry handler for automatic
// AI-assisted fixes when validation fails. When configured, the engine will
// automatically attempt to fix validation failures using AI before transitioning
// to the validation_failed state.
func WithValidationRetryHandler(handler ValidationRetryHandler) EngineOption {
	return func(e *Engine) {
		e.validationRetryHandler = handler
	}
}

// WithMetrics sets the metrics collector for observability.
// When configured, the engine will report task and step execution metrics.
// Use NoopMetrics{} if metrics collection is not needed.
func WithMetrics(m Metrics) EngineOption {
	return func(e *Engine) {
		e.metrics = m
	}
}

// WithHookManager sets the hook manager for crash recovery hooks.
// When configured, the engine will create and update hooks during task execution.
func WithHookManager(hm HookManager) EngineOption {
	return func(e *Engine) {
		e.hookManager = hm
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
func (e *Engine) Start(ctx context.Context, workspaceName, branch, worktreePath string, template *domain.Template, description string) (*domain.Task, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
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
			Status:   constants.StepStatusPending,
			Attempts: 0,
		}
	}

	task := &domain.Task{
		ID:          taskID,
		WorkspaceID: workspaceName,
		TemplateID:  template.Name,
		Description: description,
		Status:      constants.TaskStatusPending,
		CurrentStep: 0,
		Steps:       taskSteps,
		StepResults: make([]domain.StepResult, 0, len(template.Steps)), // Pre-allocate for expected steps
		Transitions: make([]domain.Transition, 0, 8),                   // Pre-allocate for typical transition count
		CreatedAt:   now,
		UpdatedAt:   now,
		Config: domain.TaskConfig{
			Agent:              template.DefaultAgent,
			Model:              template.DefaultModel,
			ValidationCommands: template.ValidationCommands,
		},
		SchemaVersion: constants.TaskSchemaVersion,
		Metadata: map[string]any{
			"branch":       branch,
			"worktree_dir": worktreePath,
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

	// Create hook for crash recovery (if hook manager is configured)
	if e.hookManager != nil {
		if err := e.hookManager.CreateHook(ctx, task); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", taskID).
				Msg("failed to create hook, continuing without crash recovery")
		}
	}

	// Inject logger with task context for step executors
	ctx = e.injectLoggerContext(ctx, workspaceName, taskID)

	// Record task start for metrics
	e.recordTaskStarted(taskID, template.Name)

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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Record start time
	startTime := time.Now()

	// Update task step status (only for sequential execution)
	if task.CurrentStep < len(task.Steps) {
		task.Steps[task.CurrentStep].Status = constants.StepStatusRunning
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Handle nil result - create minimal result for tracking
	if result == nil {
		result = &domain.StepResult{
			StepIndex: task.CurrentStep,
			StepName:  step.Name,
			Status:    constants.StepStatusFailed,
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
		return e.handleSuccessResult(task, step, result)
	case constants.StepStatusNoChanges:
		return e.handleNoChangesResult(task, step)
	case constants.StepStatusAwaitingApproval:
		return e.handleAwaitingApprovalResult(ctx, task)
	case constants.StepStatusFailed:
		return e.handleFailedResult(ctx, task, step, result)
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
// When force is true, running tasks can be abandoned and any tracked processes
// will be terminated using SIGTERM followed by SIGKILL if needed.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - task: The task to abandon (must be in an abandonable state)
//   - reason: Explanation for the abandonment
//   - force: If true, allows abandoning running tasks and kills tracked processes
//
// Returns an error if:
//   - ctx is canceled
//   - task is nil
//   - task is not in an abandonable state (unless force=true for running tasks)
//   - state persistence fails
func (e *Engine) Abandon(ctx context.Context, task *domain.Task, reason string, force bool) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	if task == nil {
		return fmt.Errorf("%w: task is nil", atlaserrors.ErrInvalidTransition)
	}

	log := e.logger.With().
		Str("task_id", task.ID).
		Str("workspace_name", task.WorkspaceID).
		Str("current_status", task.Status.String()).
		Bool("force", force).
		Logger()

	if err := e.validateCanAbandon(task, force, log); err != nil {
		return err
	}

	// Terminate running processes if force-abandoning a running task
	if force && task.Status == constants.TaskStatusRunning {
		e.terminateTrackedProcesses(task, log)
	}

	if err := Transition(ctx, task, constants.TaskStatusAbandoned, reason); err != nil {
		log.Error().Err(err).Msg("failed to transition task to abandoned")
		return err
	}

	if err := e.store.Update(ctx, task.WorkspaceID, task); err != nil {
		log.Error().Err(err).Msg("failed to save abandoned task")
		return fmt.Errorf("failed to save task: %w", err)
	}

	log.Info().Str("reason", reason).Msg("task abandoned successfully")
	return nil
}

// validateCanAbandon checks if a task can be abandoned.
func (e *Engine) validateCanAbandon(task *domain.Task, force bool, log zerolog.Logger) error {
	canAbandon := CanAbandon(task.Status)
	if force {
		canAbandon = CanForceAbandon(task.Status)
	}

	if !canAbandon {
		log.Warn().Msg("task not in abandonable state")
		if !force && CanForceAbandon(task.Status) {
			return fmt.Errorf("%w: task status %s cannot be abandoned without --force",
				atlaserrors.ErrInvalidTransition, task.Status)
		}
		return fmt.Errorf("%w: task status %s cannot be abandoned",
			atlaserrors.ErrInvalidTransition, task.Status)
	}
	return nil
}

// terminateTrackedProcesses kills tracked processes for force-abandonment.
func (e *Engine) terminateTrackedProcesses(task *domain.Task, log zerolog.Logger) {
	log.Warn().
		Ints("tracked_pids", task.RunningProcesses).
		Msg("force-abandoning running task, attempting to terminate processes")

	if len(task.RunningProcesses) == 0 {
		log.Warn().Msg("no processes tracked - task may still be running in background")
		return
	}

	pm := NewProcessManager(log)
	terminated, errs := pm.TerminateProcesses(task.RunningProcesses, constants.ProcessTerminationTimeout)

	log.Info().
		Int("total_processes", len(task.RunningProcesses)).
		Int("terminated", terminated).
		Int("errors", len(errs)).
		Msg("process termination attempted")

	for _, err := range errs {
		log.Warn().Err(err).Msg("failed to terminate process")
	}

	task.RunningProcesses = nil
}

// handleSuccessResult processes a successful step result.
func (e *Engine) handleSuccessResult(task *domain.Task, step *domain.StepDefinition, result *domain.StepResult) error {
	// Check for detect_only validation with no issues - skip fix steps
	if result.Metadata != nil {
		detectOnly, hasDetectOnly := result.Metadata["detect_only"].(bool)
		validationFailed, _ := result.Metadata["validation_failed"].(bool) // defaults to false if missing
		if hasDetectOnly && detectOnly && !validationFailed {
			e.setMetadata(task, "no_issues_detected", true)
			e.logger.Info().
				Str("task_id", task.ID).
				Str("step_name", step.Name).
				Msg("no issues detected in validation, will skip fix steps")
		}
	}
	// Auto-proceed logic handled by caller (runSteps continues)
	return nil
}

// handleNoChangesResult processes a no-changes step result.
func (e *Engine) handleNoChangesResult(task *domain.Task, step *domain.StepDefinition) error {
	// No changes were made (e.g., AI decided no modifications needed)
	// Set metadata flag to skip remaining git steps (push, PR)
	e.setMetadata(task, "skip_git_steps", true)
	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Msg("no changes to commit, will skip remaining git steps")
	return nil
}

// handleAwaitingApprovalResult processes an awaiting-approval step result.
func (e *Engine) handleAwaitingApprovalResult(ctx context.Context, task *domain.Task) error {
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
}

// handleFailedResult processes a failed step result.
func (e *Engine) handleFailedResult(ctx context.Context, task *domain.Task, step *domain.StepDefinition, result *domain.StepResult) error {
	// Store error context for retry (FR25)
	e.setErrorMetadata(task, step.Name, result.Error)

	// Check for specialized failure types (ci_failed, gh_failed, ci_timeout)
	// These have dedicated handlers with user action options
	if handled, err := e.DispatchFailureByType(ctx, task, result); handled {
		return err
	}

	// Map step type to error status with valid transition path
	return e.transitionToErrorState(ctx, task, step.Type, result.Error)
}

// executeStepInternal executes a step without modifying task state.
// This is safe for concurrent execution in parallel step groups.
func (e *Engine) executeStepInternal(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	executor, err := e.registry.Get(step.Type)
	if err != nil {
		return nil, fmt.Errorf("no executor for step type %s: %w", step.Type, err)
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
		agent, model := resolveStepAgentModel(task, step)
		event = event.Str("agent", string(agent)).Str("model", model)
	}

	return event
}

// resolveStepAgentModel returns the resolved agent and model for a step,
// applying step-level config overrides to task defaults.
func resolveStepAgentModel(task *domain.Task, step *domain.StepDefinition) (agent domain.Agent, model string) {
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
	totalSteps := len(template.Steps)

	for task.CurrentStep < totalSteps {
		if err := ctx.Err(); err != nil {
			// Context canceled (likely Ctrl+C) - save state before returning
			e.logger.Info().
				Str("task_id", task.ID).
				Int("current_step", task.CurrentStep).
				Msg("context canceled, saving state before exit")

			// Try to save current state as checkpoint (use context without cancellation since original is canceled)
			if saveErr := e.store.Update(context.WithoutCancel(ctx), task.WorkspaceID, task); saveErr != nil {
				e.logger.Error().Err(saveErr).Msg("failed to save state on cancellation")
			}
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

		// Notify step start for UI feedback
		e.notifyStepStart(task, step, totalSteps)

		// Update hook state to step_running (if hook manager is configured)
		e.transitionHookStep(ctx, task, step.Name, task.CurrentStep)

		result, err := e.executeCurrentStep(ctx, task, template)
		result, err = e.handleStepExecutionResult(ctx, task, step, result, err, totalSteps)
		if err != nil {
			return err
		}

		// Notify step complete for UI feedback
		e.notifyStepComplete(task, step, result, totalSteps)

		if err := e.processStepResult(ctx, task, result, step); err != nil {
			// Update hook on step failure
			e.failHookStep(ctx, task, step.Name, err)
			return err
		}

		// Update hook on step completion
		e.completeHookStep(ctx, task, step.Name)

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

// handleExecutionError handles errors from step execution.
func (e *Engine) handleExecutionError(ctx context.Context, task *domain.Task, step *domain.StepDefinition, result *domain.StepResult, err error) error {
	// Save step result first to preserve output (e.g., validation errors)
	if result != nil {
		task.StepResults = append(task.StepResults, *result)
	}
	return e.handleStepError(ctx, task, step, err)
}

// handleStepExecutionResult processes the result of step execution.
// On error, it notifies completion and attempts validation retry before returning error.
// On success, returns the result with nil error.
func (e *Engine) handleStepExecutionResult(
	ctx context.Context,
	task *domain.Task,
	step *domain.StepDefinition,
	result *domain.StepResult,
	err error,
	totalSteps int,
) (*domain.StepResult, error) {
	if err == nil {
		return result, nil
	}

	// Notify step complete even on error (with error status)
	if result != nil {
		e.notifyStepComplete(task, step, result, totalSteps)
	}

	// Attempt automatic validation retry before error handling
	result, err = e.tryValidationRetry(ctx, task, step, result, err, totalSteps)
	if err != nil {
		return result, e.handleExecutionError(ctx, task, step, result, err)
	}
	return result, nil
}

// tryValidationRetry attempts automatic validation retry for validation step failures.
// If retry succeeds, returns the updated result with nil error.
// Otherwise, returns the original result and error unchanged.
func (e *Engine) tryValidationRetry(
	ctx context.Context,
	task *domain.Task,
	step *domain.StepDefinition,
	result *domain.StepResult,
	err error,
	totalSteps int,
) (*domain.StepResult, error) {
	if step.Type != domain.StepTypeValidation || !e.shouldAttemptValidationRetry(result) {
		return result, err
	}

	retryResult, retryErr := e.attemptValidationRetry(ctx, task, result)
	if retryErr != nil || retryResult == nil || !retryResult.Success {
		return result, err
	}

	// Retry succeeded - update result and notify
	newResult := e.convertRetryResultToStepResult(task, step, retryResult)
	e.notifyStepComplete(task, step, newResult, totalSteps)
	return newResult, nil
}

// shouldSkipStep returns true if the step should be skipped.
// This skips:
// - git push and PR steps when "skip_git_steps" flag is set (no changes to commit)
// - AI and validation steps when "no_issues_detected" flag is set (detect_only found no issues)
func (e *Engine) shouldSkipStep(task *domain.Task, step *domain.StepDefinition) bool {
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
		task.Steps[task.CurrentStep].Status = constants.StepStatusFailed
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
		agent, model := resolveStepAgentModel(task, step)
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
		agent, model := resolveStepAgentModel(task, step)
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
	}
}

// completeHookStep updates the hook when a step completes successfully.
func (e *Engine) completeHookStep(ctx context.Context, task *domain.Task, stepName string) {
	if e.hookManager != nil {
		if err := e.hookManager.CompleteStep(ctx, task, stepName); err != nil {
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
		if err := e.hookManager.FailStep(ctx, task, stepName, stepErr); err != nil {
			e.logger.Warn().Err(err).
				Str("task_id", task.ID).
				Str("step_name", stepName).
				Msg("failed to update hook step failure")
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
