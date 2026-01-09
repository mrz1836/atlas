// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/validation"
)

// CommandRunner is an alias to validation.CommandRunner for backward compatibility.
// Deprecated: Use validation.CommandRunner directly. Will be removed in Epic 7+.
type CommandRunner = validation.CommandRunner

// DefaultCommandRunner is an alias to validation.DefaultCommandRunner for backward compatibility.
// Deprecated: Use validation.DefaultCommandRunner directly. Will be removed in Epic 7+.
type DefaultCommandRunner = validation.DefaultCommandRunner

// ValidationExecutor handles validation steps.
// It runs configured validation commands using the parallel pipeline runner
// and captures their output. Results can optionally be saved as artifacts.
// When retry is configured, failed validation results include retry context.
type ValidationExecutor struct {
	workDir       string
	runner        validation.CommandRunner
	toolChecker   validation.ToolChecker
	artifactSaver ArtifactSaver
	notifier      Notifier
	retryHandler  RetryHandler

	// Validation commands from project config (ordered by execution)
	preCommitCommands []string
	formatCommands    []string
	lintCommands      []string
	testCommands      []string
}

// NewValidationExecutor creates a new validation executor with default settings.
// For more customization, use NewValidationExecutorWithOptions.
func NewValidationExecutor(workDir string) *ValidationExecutor {
	return NewValidationExecutorWithOptions(workDir)
}

// NewValidationExecutorWithOptions creates a validation executor with functional options.
// This is the preferred constructor for creating customized executors.
func NewValidationExecutorWithOptions(workDir string, opts ...ValidationExecutorOption) *ValidationExecutor {
	e := &ValidationExecutor{
		workDir: workDir,
		runner:  &validation.DefaultCommandRunner{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// NewValidationExecutorWithRunner creates a validation executor with a custom command runner.
// Deprecated: Use NewValidationExecutorWithOptions with WithValidationRunner instead.
func NewValidationExecutorWithRunner(workDir string, runner validation.CommandRunner) *ValidationExecutor {
	return NewValidationExecutorWithOptions(workDir, WithValidationRunner(runner))
}

// ValidationCommands holds the validation command configuration from project config.
type ValidationCommands struct {
	Format    []string
	Lint      []string
	Test      []string
	PreCommit []string
}

// ValidationExecutorOption configures a ValidationExecutor.
type ValidationExecutorOption func(*ValidationExecutor)

// WithValidationRunner sets a custom command runner.
func WithValidationRunner(runner validation.CommandRunner) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.runner = runner
	}
}

// WithValidationToolChecker sets a custom tool checker.
func WithValidationToolChecker(tc validation.ToolChecker) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.toolChecker = tc
	}
}

// WithValidationArtifactSaver sets the artifact saver for saving validation results.
func WithValidationArtifactSaver(saver ArtifactSaver) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.artifactSaver = saver
	}
}

// WithValidationNotifier sets the notifier for user notifications.
func WithValidationNotifier(notifier Notifier) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.notifier = notifier
	}
}

// WithValidationRetryHandler sets the retry handler for AI-assisted retries.
func WithValidationRetryHandler(handler RetryHandler) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.retryHandler = handler
	}
}

// WithValidationCommands sets the validation commands from project config.
func WithValidationCommands(cmds ValidationCommands) ValidationExecutorOption {
	return func(e *ValidationExecutor) {
		e.formatCommands = cmds.Format
		e.lintCommands = cmds.Lint
		e.testCommands = cmds.Test
		e.preCommitCommands = cmds.PreCommit
	}
}

// NewValidationExecutorWithDeps creates a validation executor with full dependencies.
// The artifactSaver, notifier, and retryHandler may be nil if those features are not needed.
// Deprecated: Use NewValidationExecutorWithOptions with functional options instead.
func NewValidationExecutorWithDeps(workDir string, artifactSaver ArtifactSaver, notifier Notifier, retryHandler RetryHandler) *ValidationExecutor {
	return NewValidationExecutorWithOptions(workDir,
		WithValidationArtifactSaver(artifactSaver),
		WithValidationNotifier(notifier),
		WithValidationRetryHandler(retryHandler),
	)
}

// NewValidationExecutorFull creates a validation executor with all dependencies including
// validation commands from project config. The commands override the defaults.
// Deprecated: Use NewValidationExecutorWithOptions with functional options instead.
func NewValidationExecutorFull(workDir string, artifactSaver ArtifactSaver, notifier Notifier, retryHandler RetryHandler, commands ValidationCommands) *ValidationExecutor {
	return NewValidationExecutorWithOptions(workDir,
		WithValidationArtifactSaver(artifactSaver),
		WithValidationNotifier(notifier),
		WithValidationRetryHandler(retryHandler),
		WithValidationCommands(commands),
	)
}

// NewValidationExecutorWithAll creates a validation executor with all dependencies including custom runner.
// Deprecated: Use NewValidationExecutorWithOptions with functional options instead.
func NewValidationExecutorWithAll(workDir string, runner validation.CommandRunner, toolChecker validation.ToolChecker, artifactSaver ArtifactSaver, notifier Notifier, retryHandler RetryHandler) *ValidationExecutor {
	return NewValidationExecutorWithOptions(workDir,
		WithValidationRunner(runner),
		WithValidationToolChecker(toolChecker),
		WithValidationArtifactSaver(artifactSaver),
		WithValidationNotifier(notifier),
		WithValidationRetryHandler(retryHandler),
	)
}

// Execute runs validation commands using the parallel pipeline runner.
// Commands are retrieved from task.Config.ValidationCommands.
// If no commands are configured, default commands are used.
//
// The execution order is:
// 1. Format (sequential, first)
// 2. Lint + Test (parallel)
// 3. Pre-commit (sequential, last)
//
// Results are saved as versioned artifacts if an ArtifactSaver is configured.
// Bell notifications are emitted on failure if a Notifier is configured.
func (e *ValidationExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	startTime := time.Now()
	log := zerolog.Ctx(ctx)
	e.logExecutionStart(log, task, step)

	// Run the validation pipeline
	pipelineResult, pipelineErr := e.runPipeline(ctx, task, log)
	elapsed := time.Since(startTime)

	// Save artifact and emit notifications
	artifactPath := e.saveArtifactIfNeeded(ctx, task, pipelineResult, log)

	// Build output and metadata
	output := validation.FormatResultWithArtifact(pipelineResult, artifactPath)
	validationChecks := pipelineResult.BuildChecksAsMap()

	// Early return: detect_only mode always succeeds
	if e.isDetectOnlyMode(step) {
		return e.buildDetectOnlyResult(task, step, startTime, elapsed, output, validationChecks, pipelineResult, artifactPath, log), nil
	}

	// Early return: pipeline error
	if pipelineErr != nil {
		return e.buildErrorResult(task, step, startTime, elapsed, output, validationChecks, pipelineResult, artifactPath, pipelineErr, log), pipelineErr
	}

	// Success case
	return e.buildSuccessResult(task, step, startTime, elapsed, output, validationChecks, pipelineResult, log), nil
}

// Type returns the step type this executor handles.
func (e *ValidationExecutor) Type() domain.StepType {
	return domain.StepTypeValidation
}

// CanRetry checks if the validation executor can perform AI-assisted retry.
// Returns true if retry is configured and within attempt limits.
func (e *ValidationExecutor) CanRetry(attemptNum int) bool {
	if e.retryHandler == nil {
		return false
	}
	return e.retryHandler.CanRetry(attemptNum)
}

// RetryEnabled returns whether AI retry is enabled for this executor.
func (e *ValidationExecutor) RetryEnabled() bool {
	if e.retryHandler == nil {
		return false
	}
	return e.retryHandler.IsEnabled()
}

// MaxRetryAttempts returns the maximum number of retry attempts allowed.
// Returns 0 if retry is not configured.
func (e *ValidationExecutor) MaxRetryAttempts() int {
	if e.retryHandler == nil {
		return 0
	}
	return e.retryHandler.MaxAttempts()
}

// logExecutionStart logs the start of validation step execution.
func (e *ValidationExecutor) logExecutionStart(log *zerolog.Logger, task *domain.Task, step *domain.StepDefinition) {
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing validation step")
}

// runPipeline executes the validation pipeline and returns the result.
func (e *ValidationExecutor) runPipeline(ctx context.Context, task *domain.Task, log *zerolog.Logger) (*validation.PipelineResult, error) {
	config := e.buildRunnerConfig(task)

	log.Debug().
		Strs("format_commands", config.FormatCommands).
		Strs("lint_commands", config.LintCommands).
		Strs("test_commands", config.TestCommands).
		Strs("pre_commit_commands", config.PreCommitCommands).
		Str("work_dir", e.workDir).
		Msg("running validation pipeline")

	executor := validation.NewExecutorWithRunner(validation.DefaultTimeout, e.runner)
	runner := validation.NewRunner(executor, config)
	return runner.Run(ctx, e.workDir)
}

// saveArtifactIfNeeded saves the pipeline result as an artifact if configured.
func (e *ValidationExecutor) saveArtifactIfNeeded(ctx context.Context, task *domain.Task, result *validation.PipelineResult, log *zerolog.Logger) string {
	artifactPath, err := e.handlePipelineResult(ctx, task, result, log)
	if err != nil && !errors.Is(err, atlaserrors.ErrValidationFailed) {
		log.Warn().Err(err).Msg("failed to handle pipeline result (artifact/notification)")
	}
	return artifactPath
}

// isDetectOnlyMode checks if the step is configured for detect-only mode.
func (e *ValidationExecutor) isDetectOnlyMode(step *domain.StepDefinition) bool {
	d, ok := step.Config["detect_only"].(bool)
	return ok && d
}

// buildMetadataWithArtifact creates metadata map with optional artifact path.
func (e *ValidationExecutor) buildMetadataWithArtifact(validationChecks []map[string]any, pipelineResult *validation.PipelineResult, artifactPath string, extra map[string]any) map[string]any {
	metadata := map[string]any{
		"validation_checks": validationChecks,
		"pipeline_result":   pipelineResult,
	}
	for k, v := range extra {
		metadata[k] = v
	}
	if artifactPath != "" {
		metadata["artifact_path"] = artifactPath
	}
	return metadata
}

// buildDetectOnlyResult builds the result for detect_only mode.
func (e *ValidationExecutor) buildDetectOnlyResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, elapsed time.Duration, output string, validationChecks []map[string]any, pipelineResult *validation.PipelineResult, artifactPath string, log *zerolog.Logger) *domain.StepResult {
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Bool("validation_passed", pipelineResult.Success).
		Msg("validation step completed in detect_only mode")

	metadata := e.buildMetadataWithArtifact(validationChecks, pipelineResult, artifactPath, map[string]any{
		"validation_failed": !pipelineResult.Success,
		"detect_only":       true,
	})

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output,
		Metadata:    metadata,
	}
}

// buildErrorResult builds the result for a failed validation.
func (e *ValidationExecutor) buildErrorResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, elapsed time.Duration, output string, validationChecks []map[string]any, pipelineResult *validation.PipelineResult, artifactPath string, pipelineErr error, log *zerolog.Logger) *domain.StepResult {
	log.Error().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("failed_step", pipelineResult.FailedStepName).
		Err(pipelineErr).
		Dur("duration_ms", elapsed).
		Msg("validation step failed")

	metadata := e.buildMetadataWithArtifact(validationChecks, pipelineResult, artifactPath, nil)

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "failed",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output,
		Error:       pipelineErr.Error(),
		Metadata:    metadata,
	}
}

// buildSuccessResult builds the result for a successful validation.
func (e *ValidationExecutor) buildSuccessResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, elapsed time.Duration, output string, validationChecks []map[string]any, pipelineResult *validation.PipelineResult, log *zerolog.Logger) *domain.StepResult {
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Int64("pipeline_duration_ms", pipelineResult.DurationMs).
		Dur("duration_ms", elapsed).
		Msg("validation step completed")

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output,
		Metadata: map[string]any{
			"validation_checks": validationChecks,
			"pipeline_result":   pipelineResult,
		},
	}
}

// buildRunnerConfig creates a RunnerConfig from task config and executor's stored commands.
func (e *ValidationExecutor) buildRunnerConfig(_ *domain.Task) *validation.RunnerConfig {
	return &validation.RunnerConfig{
		ToolChecker:       e.toolChecker,
		FormatCommands:    e.formatCommands,
		LintCommands:      e.lintCommands,
		TestCommands:      e.testCommands,
		PreCommitCommands: e.preCommitCommands,
	}
}

// handlePipelineResult saves the result as an artifact and emits notifications.
// Returns the artifact path (if saved successfully) and any error.
func (e *ValidationExecutor) handlePipelineResult(ctx context.Context, task *domain.Task, result *validation.PipelineResult, log *zerolog.Logger) (string, error) {
	if e.artifactSaver == nil {
		return "", nil
	}

	// Create result handler with our dependencies
	// The validation.ResultHandler accepts validation.ArtifactSaver and validation.Notifier
	// Our ArtifactSaver and Notifier interfaces have the same signatures, so we can adapt them
	handler := validation.NewResultHandler(
		&artifactSaverAdapter{e.artifactSaver},
		&notifierAdapter{e.notifier},
		*log,
	)

	return handler.HandleResult(ctx, task.WorkspaceID, task.ID, result)
}

// artifactSaverAdapter adapts steps.ArtifactSaver to validation.ArtifactSaver.
type artifactSaverAdapter struct {
	saver ArtifactSaver
}

// SaveVersionedArtifact implements validation.ArtifactSaver.
func (a *artifactSaverAdapter) SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error) {
	if a.saver == nil {
		return "", nil
	}
	return a.saver.SaveVersionedArtifact(ctx, workspaceName, taskID, baseName, data)
}

// notifierAdapter adapts steps.Notifier to validation.Notifier.
type notifierAdapter struct {
	notifier Notifier
}

// Bell implements validation.Notifier.
func (n *notifierAdapter) Bell() {
	if n.notifier != nil {
		n.notifier.Bell()
	}
}
