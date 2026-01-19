package validation

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
)

// ProgressInfo provides additional context for progress callbacks.
type ProgressInfo struct {
	CurrentStep int   // 1-indexed step number (e.g., 1 for format, 2 for lint)
	TotalSteps  int   // Total number of steps in pipeline (e.g., 4)
	DurationMs  int64 // Duration for completed steps in milliseconds
	ElapsedMs   int64 // Elapsed time for running steps in milliseconds
}

// ProgressCallback is called to report progress during validation pipeline execution.
// The step parameter indicates which step is running (format, lint, test, pre-commit).
// The status parameter is one of: "starting", "completed", "failed", "skipped".
// The info parameter provides additional context like step counts and duration (may be nil for backward compatibility).
type ProgressCallback func(step, status string, info *ProgressInfo)

// ToolChecker is an interface for checking tool availability.
// This allows for dependency injection and testing.
type ToolChecker interface {
	// IsGoPreCommitInstalled checks if go-pre-commit is installed.
	// Returns (installed, version, error).
	IsGoPreCommitInstalled(ctx context.Context) (bool, string, error)
}

// DefaultToolChecker implements ToolChecker using the config package.
type DefaultToolChecker struct{}

// IsGoPreCommitInstalled checks if go-pre-commit is installed using config package.
func (d *DefaultToolChecker) IsGoPreCommitInstalled(ctx context.Context) (bool, string, error) {
	return config.IsGoPreCommitInstalled(ctx)
}

// Stager is an interface for staging modified files after validation.
// This allows for dependency injection and testing.
type Stager interface {
	// StageModifiedFiles stages any files modified during validation.
	StageModifiedFiles(ctx context.Context, workDir string) error
}

// DefaultStager implements Stager using the staging package functions.
type DefaultStager struct{}

// StageModifiedFiles stages modified files using the default implementation.
func (d *DefaultStager) StageModifiedFiles(ctx context.Context, workDir string) error {
	return StageModifiedFiles(ctx, workDir)
}

// RunnerConfig holds configuration for the validation pipeline.
type RunnerConfig struct {
	FormatCommands    []string
	LintCommands      []string
	TestCommands      []string
	PreCommitCommands []string
	ProgressCallback  ProgressCallback
	ToolChecker       ToolChecker // Optional: for checking tool availability. If nil, DefaultToolChecker is used.
	Stager            Stager      // Optional: for staging modified files. If nil, DefaultStager is used.
}

// Runner orchestrates the validation pipeline with parallel execution.
type Runner struct {
	executor   *Executor
	config     *RunnerConfig
	totalSteps int      // Total number of steps in the pipeline
	stepTimes  sync.Map // Tracks start time for each step (step name -> time.Time)
}

// NewRunner creates a validation pipeline runner.
func NewRunner(executor *Executor, config *RunnerConfig) *Runner {
	if config == nil {
		config = &RunnerConfig{}
	}
	return &Runner{
		executor: executor,
		config:   config,
	}
}

// SetProgressCallback sets or updates the progress callback.
func (r *Runner) SetProgressCallback(cb ProgressCallback) {
	r.config.ProgressCallback = cb
}

// Run executes the full validation pipeline in this order:
// 1. Pre-commit (first) - makes auto-fixes like import ordering
// 2. Format (second) - standardizes code after pre-commit changes
// 3. Lint (third) - validation only, no modifications
// 4. Test (last) - tests run after code is validated
//
// Returns a PipelineResult containing all step results regardless of success/failure.
// Returns an error if any step fails, but the PipelineResult will still contain
// all results collected up to and including the failure.
func (r *Runner) Run(ctx context.Context, workDir string) (*PipelineResult, error) {
	log := zerolog.Ctx(ctx)
	result := &PipelineResult{}
	startTime := time.Now()

	// Calculate total steps (pre-commit=1, format=1, lint=1, test=1)
	r.totalSteps = 4

	log.Info().Str("work_dir", workDir).Msg("starting validation pipeline")

	// Check context cancellation before starting
	if err := ctxutil.Canceled(ctx); err != nil {
		return r.finalize(result, startTime), err
	}

	// Phase 1: Pre-commit (first - makes auto-fixes like import ordering)
	preCommitInstalled, preCommitVersion, checkErr := r.getToolChecker().IsGoPreCommitInstalled(ctx)
	if checkErr != nil {
		log.Warn().Err(checkErr).Msg("failed to check go-pre-commit installation status")
		preCommitInstalled = false
	}

	if preCommitInstalled {
		log.Info().Str("version", preCommitVersion).Msg("go-pre-commit detected")
		r.reportProgress("pre-commit", "starting")
		preCommitResults, err := r.runSequentialWithPhase(ctx, r.getPreCommitCommands(), workDir, "pre-commit")
		result.PreCommitResults = preCommitResults
		if err != nil {
			r.reportProgress("pre-commit", "failed")
			result.FailedStepName = "pre-commit"
			log.Error().Err(err).Msg("pre-commit step failed")
			return r.finalize(result, startTime), err
		}
		r.reportProgress("pre-commit", "completed")
	} else {
		r.handlePreCommitSkipped(result, log)
	}

	// Check context cancellation between phases
	if cancelErr := ctxutil.Canceled(ctx); cancelErr != nil {
		return r.finalize(result, startTime), cancelErr
	}

	// Phase 2: Format (cleans up any changes from pre-commit)
	r.reportProgress("format", "starting")
	formatResults, err := r.runSequentialWithPhase(ctx, r.getFormatCommands(), workDir, "format")
	result.FormatResults = formatResults
	if err != nil {
		r.reportProgress("format", "failed")
		result.FailedStepName = "format"
		log.Error().Err(err).Msg("format step failed")
		return r.finalize(result, startTime), err
	}
	r.reportProgress("format", "completed")

	// Stage files after format (format is last step to modify files)
	if stageErr := r.getStager().StageModifiedFiles(ctx, workDir); stageErr != nil {
		log.Warn().Err(stageErr).Msg("failed to stage modified files")
		// Non-fatal - continue with validation
	}

	// Check context cancellation between phases
	if cancelErr := ctxutil.Canceled(ctx); cancelErr != nil {
		return r.finalize(result, startTime), cancelErr
	}

	// Phase 3: Lint (validation only, no modifications)
	r.reportProgress("lint", "starting")
	lintResults, err := r.runSequentialWithPhase(ctx, r.getLintCommands(), workDir, "lint")
	result.LintResults = lintResults
	if err != nil {
		r.reportProgress("lint", "failed")
		result.FailedStepName = "lint"
		log.Error().Err(err).Msg("lint step failed")
		return r.finalize(result, startTime), err
	}
	r.reportProgress("lint", "completed")

	// Check context cancellation between phases
	if cancelErr := ctxutil.Canceled(ctx); cancelErr != nil {
		return r.finalize(result, startTime), cancelErr
	}

	// Phase 4: Test (last)
	if testErr := r.runTestPhase(ctx, result, workDir, log); testErr != nil {
		return r.finalize(result, startTime), testErr
	}

	result.Success = true
	log.Info().Dur("duration_ms", time.Since(startTime)).Msg("validation pipeline completed successfully")
	return r.finalize(result, startTime), nil
}

// runTestPhase handles the test phase (sequential, runs last).
func (r *Runner) runTestPhase(ctx context.Context, result *PipelineResult, workDir string, log *zerolog.Logger) error {
	r.reportProgress("test", "starting")

	testResults, err := r.runSequentialWithPhase(ctx, r.getTestCommands(), workDir, "test")
	result.TestResults = testResults
	if err != nil {
		r.reportProgress("test", "failed")
		result.FailedStepName = "test"
		log.Error().Err(err).Msg("test step failed")
		return err
	}
	r.reportProgress("test", "completed")

	return nil
}

// handlePreCommitSkipped records that pre-commit was skipped due to missing tool.
func (r *Runner) handlePreCommitSkipped(result *PipelineResult, log *zerolog.Logger) {
	log.Warn().Msg("go-pre-commit not installed, skipping pre-commit validation")
	r.reportProgress("pre-commit", "skipped")

	// Track skipped step - initialize both slice and map for consistency
	// Capacity 4 covers all pipeline steps: pre-commit, format, lint, test
	if result.SkippedSteps == nil {
		result.SkippedSteps = make([]string, 0, 4)
	}
	if result.SkipReasons == nil {
		result.SkipReasons = make(map[string]string, 4)
	}
	result.SkippedSteps = append(result.SkippedSteps, "pre-commit")
	result.SkipReasons["pre-commit"] = "go-pre-commit not installed"
}

// runSequentialWithPhase executes commands in sequence with phase context for logging.
// The phase parameter is used for clearer log messages (pre-commit, format, lint, test).
func (r *Runner) runSequentialWithPhase(ctx context.Context, commands []string, workDir, phase string) ([]Result, error) {
	if len(commands) == 0 {
		return nil, nil
	}
	return r.executor.RunWithPhase(ctx, commands, workDir, phase)
}

// stepNumbers maps step names to their 1-indexed position in the pipeline.
//
//nolint:gochecknoglobals // Package-level constant-like mapping for step ordering
var stepNumbers = map[string]int{
	"pre-commit": 1,
	"format":     2,
	"lint":       3,
	"test":       4,
}

// stepNumber returns the 1-indexed step number for a given step name.
// Returns 0 for unknown step names.
func stepNumber(step string) int {
	return stepNumbers[step]
}

// reportProgress calls the progress callback if configured.
// It tracks step start times and calculates durations.
func (r *Runner) reportProgress(step, status string) {
	if r.config.ProgressCallback == nil {
		return
	}

	info := &ProgressInfo{
		CurrentStep: stepNumber(step),
		TotalSteps:  r.totalSteps,
	}

	switch status {
	case "starting":
		r.stepTimes.Store(step, time.Now())
		// ElapsedMs defaults to 0 for starting status (step just began)
	case "completed", "failed":
		if startTimeVal, ok := r.stepTimes.Load(step); ok {
			if startTime, ok := startTimeVal.(time.Time); ok {
				info.DurationMs = time.Since(startTime).Milliseconds()
			}
			r.stepTimes.Delete(step)
		}
	}

	r.config.ProgressCallback(step, status, info)
}

// finalize sets the duration and returns the result.
func (r *Runner) finalize(result *PipelineResult, startTime time.Time) *PipelineResult {
	result.DurationMs = time.Since(startTime).Milliseconds()
	return result
}

// getFormatCommands returns format commands with default fallback.
func (r *Runner) getFormatCommands() []string {
	return applyDefaults(r.config.FormatCommands, constants.DefaultFormatCommand)
}

// getLintCommands returns lint commands with default fallback.
func (r *Runner) getLintCommands() []string {
	return applyDefaults(r.config.LintCommands, constants.DefaultLintCommand)
}

// getTestCommands returns test commands with default fallback.
func (r *Runner) getTestCommands() []string {
	return applyDefaults(r.config.TestCommands, constants.DefaultTestCommand)
}

// getPreCommitCommands returns pre-commit commands with default fallback.
func (r *Runner) getPreCommitCommands() []string {
	return applyDefaults(r.config.PreCommitCommands, constants.DefaultPreCommitCommand)
}

// getToolChecker returns the configured tool checker or the default.
func (r *Runner) getToolChecker() ToolChecker {
	if r.config.ToolChecker != nil {
		return r.config.ToolChecker
	}
	return &DefaultToolChecker{}
}

// getStager returns the configured stager or the default.
func (r *Runner) getStager() Stager {
	if r.config.Stager != nil {
		return r.config.Stager
	}
	return &DefaultStager{}
}

// applyDefaults returns commands or a slice with the default if empty.
func applyDefaults(cmds []string, defaultCmd string) []string {
	if len(cmds) == 0 {
		return []string{defaultCmd}
	}
	return cmds
}
