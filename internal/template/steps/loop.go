// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// InnerStepRunner executes inner steps within a loop iteration.
// This interface enables mocking inner step execution in tests.
type InnerStepRunner interface {
	ExecuteStep(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error)
}

// LoopStateStore persists loop state for checkpointing.
// This interface enables mocking state persistence in tests.
type LoopStateStore interface {
	SaveLoopState(ctx context.Context, task *domain.Task, state *domain.LoopState) error
	LoadLoopState(ctx context.Context, task *domain.Task, stepName string) (*domain.LoopState, error)
}

// LoopExecutor executes iterative step groups.
// It supports count-based, condition-based, and signal-based termination
// with circuit breakers for safety.
type LoopExecutor struct {
	innerRunner InnerStepRunner  // Mockable: executes inner steps
	stateStore  LoopStateStore   // Mockable: state persistence
	scratchpad  ScratchpadWriter // Mockable: cross-iteration memory
	exitEval    ExitEvaluator    // Mockable: exit condition checking
	artifactDir string           // Directory for scratchpad files
	logger      zerolog.Logger
}

// NewLoopExecutor creates a loop executor with injectable dependencies.
func NewLoopExecutor(
	innerRunner InnerStepRunner,
	stateStore LoopStateStore,
	opts ...LoopExecutorOption,
) *LoopExecutor {
	e := &LoopExecutor{
		innerRunner: innerRunner,
		stateStore:  stateStore,
		logger:      zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// LoopExecutorOption configures a LoopExecutor.
type LoopExecutorOption func(*LoopExecutor)

// WithLoopScratchpad sets the scratchpad writer.
func WithLoopScratchpad(sw ScratchpadWriter) LoopExecutorOption {
	return func(e *LoopExecutor) { e.scratchpad = sw }
}

// WithLoopExitEvaluator sets the exit evaluator.
func WithLoopExitEvaluator(ev ExitEvaluator) LoopExecutorOption {
	return func(e *LoopExecutor) { e.exitEval = ev }
}

// WithLoopLogger sets the logger.
func WithLoopLogger(l zerolog.Logger) LoopExecutorOption {
	return func(e *LoopExecutor) { e.logger = l }
}

// WithLoopArtifactDir sets the artifact directory for scratchpad files.
func WithLoopArtifactDir(dir string) LoopExecutorOption {
	return func(e *LoopExecutor) { e.artifactDir = dir }
}

// Execute runs the loop step, iterating until an exit condition is met.
//
//nolint:gocognit // Loop orchestration inherently requires handling multiple exit conditions and states.
func (e *LoopExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get enriched logger from context (set by Engine.injectLoggerContext)
	// This provides task_id and workspace_name fields automatically.
	logger := zerolog.Ctx(ctx)
	if logger.GetLevel() == zerolog.Disabled {
		logger = &e.logger // Fallback to injected logger
	}

	startTime := time.Now()
	cfg, err := e.parseLoopConfig(step.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid loop configuration: %w", err)
	}

	logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Int("max_iterations", cfg.MaxIterations).
		Bool("until_signal", cfg.UntilSignal).
		Str("until", cfg.Until).
		Msg("starting loop step")

	// Initialize or restore state
	state := e.initOrRestoreState(ctx, task, step, cfg)

	// Set up exit evaluator if not injected
	if e.exitEval == nil && cfg.UntilSignal {
		e.exitEval = NewExitEvaluator(cfg.ExitConditions, e.logger)
	}

	// Set up scratchpad if configured
	if err := e.setupScratchpad(task, step, cfg, state); err != nil {
		logger.Warn().Err(err).Msg("failed to set up scratchpad, continuing without it")
		// Store error in metadata so caller can detect scratchpad failure
		if task.Metadata == nil {
			task.Metadata = make(map[string]any)
		}
		task.Metadata["scratchpad_setup_error"] = err.Error()
	}

	// Main loop
	for !e.shouldExit(ctx, state, cfg, task) {
		state.CurrentIteration++
		state.CurrentInnerStep = 0

		iterStart := time.Now()
		logger.Info().
			Int("iteration", state.CurrentIteration).
			Msg("starting iteration")

		// Execute inner steps
		iterResult, err := e.executeIteration(ctx, task, cfg.Steps, state)
		if err != nil {
			state.ConsecutiveErrors++
			iterResult.Error = err.Error()

			logger.Warn().
				Err(err).
				Int("iteration", state.CurrentIteration).
				Int("consecutive_errors", state.ConsecutiveErrors).
				Msg("iteration failed")

			if e.circuitBreakerTripped(state, cfg) {
				state.ExitReason = "circuit_breaker_errors"
				break
			}
			// Save state and continue to next iteration
			if checkpointErr := e.saveCheckpoint(ctx, task, state); checkpointErr != nil {
				state.ExitReason = "checkpoint_failure"
				return nil, checkpointErr
			}
			continue
		}

		state.ConsecutiveErrors = 0
		iterResult.Duration = time.Since(iterStart)
		iterResult.CompletedAt = time.Now()
		state.CompletedIterations = append(state.CompletedIterations, *iterResult)

		// Update scratchpad
		e.updateScratchpad(iterResult)

		// Check stagnation
		if len(iterResult.FilesChanged) == 0 {
			state.StagnationCount++
		} else {
			state.StagnationCount = 0
		}

		if e.stagnationTripped(state, cfg) {
			state.ExitReason = "circuit_breaker_stagnation"
			break
		}

		// Check exit signal
		if cfg.UntilSignal && iterResult.ExitSignal {
			state.ExitReason = "exit_signal"
			logger.Info().
				Int("iteration", state.CurrentIteration).
				Msg("exit signal received")
			break
		}

		// Checkpoint after each iteration
		if checkpointErr := e.saveCheckpoint(ctx, task, state); checkpointErr != nil {
			state.ExitReason = "checkpoint_failure"
			return nil, checkpointErr
		}
	}

	// Set exit reason if not already set
	if state.ExitReason == "" && state.CurrentIteration >= cfg.MaxIterations && cfg.MaxIterations > 0 {
		state.ExitReason = "max_iterations_reached"
	}

	return e.buildResult(task, step, startTime, state), nil
}

// Type returns the step type this executor handles.
func (e *LoopExecutor) Type() domain.StepType {
	return domain.StepTypeLoop
}

// parseLoopConfig extracts LoopConfig from step config map.
// Returns an error if the configuration contains invalid values.
func (e *LoopExecutor) parseLoopConfig(config map[string]any) (*domain.LoopConfig, error) {
	if config == nil {
		return &domain.LoopConfig{}, nil
	}

	cfg := &domain.LoopConfig{
		MaxIterations:  getIntFromConfig(config, "max_iterations"),
		Until:          getStringFromConfig(config, "until"),
		UntilSignal:    getBoolFromConfig(config, "until_signal"),
		FreshContext:   getBoolFromConfig(config, "fresh_context"),
		ScratchpadFile: getStringFromConfig(config, "scratchpad_file"),
		ExitConditions: getStringSliceFromConfig(config, "exit_conditions"),
		CircuitBreaker: e.parseCircuitBreaker(config),
		Steps:          e.parseInnerSteps(config),
	}

	// Validate configuration
	if err := e.validateLoopConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validateLoopConfig checks for invalid configuration values.
func (e *LoopExecutor) validateLoopConfig(cfg *domain.LoopConfig) error {
	if cfg.MaxIterations < 0 {
		return fmt.Errorf("%w: max_iterations cannot be negative: %d",
			atlaserrors.ErrLoopConfigInvalid, cfg.MaxIterations)
	}

	if cfg.CircuitBreaker.ConsecutiveErrors < 0 {
		return fmt.Errorf("%w: circuit_breaker.consecutive_errors cannot be negative: %d",
			atlaserrors.ErrLoopConfigInvalid, cfg.CircuitBreaker.ConsecutiveErrors)
	}

	if cfg.CircuitBreaker.StagnationIterations < 0 {
		return fmt.Errorf("%w: circuit_breaker.stagnation_iterations cannot be negative: %d",
			atlaserrors.ErrLoopConfigInvalid, cfg.CircuitBreaker.StagnationIterations)
	}

	return nil
}

// getIntFromConfig extracts an int value from config, handling both int and float64.
func getIntFromConfig(config map[string]any, key string) int {
	if v, ok := config[key].(int); ok {
		return v
	}
	if v, ok := config[key].(float64); ok {
		return int(v)
	}
	return 0
}

// getStringFromConfig extracts a string value from config.
func getStringFromConfig(config map[string]any, key string) string {
	if v, ok := config[key].(string); ok {
		return v
	}
	return ""
}

// getBoolFromConfig extracts a bool value from config.
func getBoolFromConfig(config map[string]any, key string) bool {
	if v, ok := config[key].(bool); ok {
		return v
	}
	return false
}

// getStringSliceFromConfig extracts a string slice from config.
func getStringSliceFromConfig(config map[string]any, key string) []string {
	if v, ok := config[key].([]string); ok {
		return v
	}
	if v, ok := config[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// parseCircuitBreaker extracts CircuitBreakerConfig from config.
func (e *LoopExecutor) parseCircuitBreaker(config map[string]any) domain.CircuitBreakerConfig {
	cb, ok := config["circuit_breaker"].(map[string]any)
	if !ok {
		return domain.CircuitBreakerConfig{}
	}
	return domain.CircuitBreakerConfig{
		StagnationIterations: getIntFromConfig(cb, "stagnation_iterations"),
		ConsecutiveErrors:    getIntFromConfig(cb, "consecutive_errors"),
	}
}

// parseInnerSteps extracts inner step definitions from config.
func (e *LoopExecutor) parseInnerSteps(config map[string]any) []domain.StepDefinition {
	steps, ok := config["steps"].([]any)
	if !ok {
		return nil
	}

	result := make([]domain.StepDefinition, 0, len(steps))
	for _, item := range steps {
		if stepMap, ok := item.(map[string]any); ok {
			result = append(result, e.parseStepDefinition(stepMap))
		}
	}
	return result
}

// parseStepDefinition converts a map to a StepDefinition.
func (e *LoopExecutor) parseStepDefinition(m map[string]any) domain.StepDefinition {
	step := domain.StepDefinition{}

	if v, ok := m["name"].(string); ok {
		step.Name = v
	}
	if v, ok := m["type"].(string); ok {
		step.Type = domain.StepType(v)
	}
	if v, ok := m["description"].(string); ok {
		step.Description = v
	}
	if v, ok := m["required"].(bool); ok {
		step.Required = v
	}
	if v, ok := m["config"].(map[string]any); ok {
		step.Config = v
	}

	return step
}

// initOrRestoreState initializes loop state or restores from checkpoint.
func (e *LoopExecutor) initOrRestoreState(ctx context.Context, task *domain.Task, step *domain.StepDefinition, cfg *domain.LoopConfig) *domain.LoopState {
	// Try to restore from checkpoint
	if e.stateStore != nil {
		if existing, err := e.stateStore.LoadLoopState(ctx, task, step.Name); err == nil && existing != nil {
			e.logger.Info().
				Int("iteration", existing.CurrentIteration).
				Msg("restored loop state from checkpoint")
			return existing
		}
	}

	// Create fresh state
	return &domain.LoopState{
		StepName:            step.Name,
		CurrentIteration:    0,
		MaxIterations:       cfg.MaxIterations,
		CurrentInnerStep:    0,
		CompletedIterations: []domain.IterationResult{},
		StartedAt:           time.Now(),
	}
}

// setupScratchpad initializes the scratchpad if configured.
func (e *LoopExecutor) setupScratchpad(task *domain.Task, step *domain.StepDefinition, cfg *domain.LoopConfig, state *domain.LoopState) error {
	if cfg.ScratchpadFile == "" {
		return nil
	}

	// Determine scratchpad path
	var scratchpadPath string
	if e.artifactDir != "" {
		scratchpadPath = filepath.Join(e.artifactDir, cfg.ScratchpadFile)
	} else {
		scratchpadPath = cfg.ScratchpadFile
	}

	state.ScratchpadPath = scratchpadPath

	// Create file-based scratchpad if not injected
	if e.scratchpad == nil {
		e.scratchpad = NewFileScratchpad(scratchpadPath, e.logger)
	}

	// Initialize if this is a fresh start (no completed iterations)
	if len(state.CompletedIterations) == 0 {
		data := &ScratchpadData{
			TaskID:     task.ID,
			LoopName:   step.Name,
			StartedAt:  time.Now(),
			Iterations: []IterationSummary{},
			Metadata:   make(map[string]any),
		}
		if err := e.scratchpad.Write(data); err != nil {
			return err
		}
		e.logger.Debug().Str("path", scratchpadPath).Msg("initialized scratchpad")
	}

	return nil
}

// executeIteration runs all inner steps for one iteration.
func (e *LoopExecutor) executeIteration(ctx context.Context, task *domain.Task, steps []domain.StepDefinition, state *domain.LoopState) (*domain.IterationResult, error) {
	iterResult := &domain.IterationResult{
		Iteration:    state.CurrentIteration,
		StepResults:  []domain.StepResult{},
		FilesChanged: []string{},
		StartedAt:    time.Now(),
	}

	var combinedOutput strings.Builder

	for i := range steps {
		step := &steps[i]
		state.CurrentInnerStep = i

		select {
		case <-ctx.Done():
			return iterResult, ctx.Err()
		default:
		}

		e.logger.Debug().
			Int("iteration", state.CurrentIteration).
			Int("inner_step", i).
			Str("step_name", step.Name).
			Msg("executing inner step")

		result, err := e.innerRunner.ExecuteStep(ctx, task, step)
		if err != nil {
			if result != nil {
				iterResult.StepResults = append(iterResult.StepResults, *result)
			}
			return iterResult, fmt.Errorf("inner step %s failed: %w", step.Name, err)
		}

		iterResult.StepResults = append(iterResult.StepResults, *result)

		// Collect files changed
		iterResult.FilesChanged = append(iterResult.FilesChanged, result.FilesChanged...)

		// Accumulate output for exit signal detection
		if result.Output != "" {
			combinedOutput.WriteString(result.Output)
			combinedOutput.WriteString("\n")
		}
	}

	// Check for exit signal in combined output
	if e.exitEval != nil {
		decision := e.exitEval.Evaluate(iterResult, combinedOutput.String())
		iterResult.ExitSignal = decision.ShouldExit
		if decision.ShouldExit {
			e.logger.Info().
				Str("reason", decision.Reason).
				Msg("exit decision made")
		}
	}

	return iterResult, nil
}

// updateScratchpad appends iteration summary to scratchpad.
func (e *LoopExecutor) updateScratchpad(iterResult *domain.IterationResult) {
	if e.scratchpad == nil {
		return
	}

	summary := &IterationSummary{
		Number:       iterResult.Iteration,
		CompletedAt:  iterResult.CompletedAt,
		FilesChanged: iterResult.FilesChanged,
		ExitSignal:   iterResult.ExitSignal,
		Success:      iterResult.Error == "",
		Error:        iterResult.Error,
	}

	// Build summary text from step results
	var summaryParts []string
	for _, sr := range iterResult.StepResults {
		if sr.Output != "" {
			// Truncate long outputs
			output := sr.Output
			if len(output) > 500 {
				output = output[:500] + "..."
			}
			summaryParts = append(summaryParts, fmt.Sprintf("%s: %s", sr.StepName, output))
		}
	}
	summary.Summary = strings.Join(summaryParts, "; ")

	if err := e.scratchpad.AppendIteration(summary); err != nil {
		e.logger.Warn().Err(err).Msg("failed to update scratchpad")
	}
}

// shouldExit determines if the loop should terminate.
func (e *LoopExecutor) shouldExit(ctx context.Context, state *domain.LoopState, cfg *domain.LoopConfig, task *domain.Task) bool {
	// Check context cancellation
	if ctx.Err() != nil {
		state.ExitReason = "context_canceled"
		return true
	}

	// Check max iterations
	if cfg.MaxIterations > 0 && state.CurrentIteration >= cfg.MaxIterations {
		state.ExitReason = "max_iterations_reached"
		return true
	}

	// Check named condition (e.g., "all_tests_pass")
	if cfg.Until != "" {
		if EvaluateBuiltinCondition(cfg.Until, task) {
			state.ExitReason = "condition_met"
			return true
		}
	}

	return false
}

// circuitBreakerTripped checks if error threshold is exceeded.
func (e *LoopExecutor) circuitBreakerTripped(state *domain.LoopState, cfg *domain.LoopConfig) bool {
	threshold := cfg.CircuitBreaker.ConsecutiveErrors
	if threshold == 0 {
		threshold = 5 // Default threshold
	}
	return state.ConsecutiveErrors >= threshold
}

// stagnationTripped checks if stagnation threshold is exceeded.
func (e *LoopExecutor) stagnationTripped(state *domain.LoopState, cfg *domain.LoopConfig) bool {
	threshold := cfg.CircuitBreaker.StagnationIterations
	if threshold == 0 {
		return false // Stagnation detection disabled
	}
	return state.StagnationCount >= threshold
}

// saveCheckpoint persists the current loop state.
// Returns an error if checkpoint failures exceed threshold (3 consecutive).
func (e *LoopExecutor) saveCheckpoint(ctx context.Context, task *domain.Task, state *domain.LoopState) error {
	if e.stateStore == nil {
		return nil
	}

	state.LastCheckpoint = time.Now()
	if err := e.stateStore.SaveLoopState(ctx, task, state); err != nil {
		state.ConsecutiveCheckpointErrors++
		e.logger.Warn().
			Err(err).
			Int("consecutive_failures", state.ConsecutiveCheckpointErrors).
			Msg("failed to save loop checkpoint")

		// Fail after 3 consecutive checkpoint errors to prevent data loss
		if state.ConsecutiveCheckpointErrors >= 3 {
			return fmt.Errorf("checkpoint persistence failing after %d attempts: %w",
				state.ConsecutiveCheckpointErrors, err)
		}
		return nil
	}

	// Reset counter on success
	state.ConsecutiveCheckpointErrors = 0
	e.logger.Debug().
		Int("iteration", state.CurrentIteration).
		Msg("saved loop checkpoint")
	return nil
}

// buildResult creates the final StepResult for the loop.
func (e *LoopExecutor) buildResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, state *domain.LoopState) *domain.StepResult {
	completedAt := time.Now()

	// Collect all files changed across all iterations
	// Preallocate based on estimated size
	totalFiles := 0
	for _, iter := range state.CompletedIterations {
		totalFiles += len(iter.FilesChanged)
	}
	allFilesChanged := make([]string, 0, totalFiles)
	for _, iter := range state.CompletedIterations {
		allFilesChanged = append(allFilesChanged, iter.FilesChanged...)
	}

	// Build output summary
	output := fmt.Sprintf("Loop completed after %d iteration(s). Exit reason: %s",
		state.CurrentIteration, state.ExitReason)

	// Serialize state for metadata
	var stateJSONStr string
	if stateJSON, err := json.Marshal(state); err == nil {
		stateJSONStr = string(stateJSON)
	}

	result := &domain.StepResult{
		StepIndex:    task.CurrentStep,
		StepName:     step.Name,
		Status:       constants.StepStatusSuccess,
		StartedAt:    startTime,
		CompletedAt:  completedAt,
		DurationMs:   completedAt.Sub(startTime).Milliseconds(),
		Output:       output,
		FilesChanged: allFilesChanged,
		Metadata: map[string]any{
			"exit_reason":          state.ExitReason,
			"iterations_completed": state.CurrentIteration,
			"loop_state":           stateJSONStr,
			"scratchpad_path":      state.ScratchpadPath,
		},
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Int("iterations", state.CurrentIteration).
		Str("exit_reason", state.ExitReason).
		Int("files_changed", len(allFilesChanged)).
		Int64("duration_ms", result.DurationMs).
		Msg("loop step completed")

	return result
}
