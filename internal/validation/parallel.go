package validation

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
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
// 1. Format (sequential, first) - formatting must complete before other steps
// 2. Lint + Pre-commit (parallel) - run simultaneously for efficiency
// 3. Test (sequential, last) - tests run after code is validated
//
// Returns a PipelineResult containing all step results regardless of success/failure.
// Returns an error if any step fails, but the PipelineResult will still contain
// all results collected up to and including the failure.
func (r *Runner) Run(ctx context.Context, workDir string) (*PipelineResult, error) {
	log := zerolog.Ctx(ctx)
	result := &PipelineResult{}
	startTime := time.Now()

	// Calculate total steps (format=1, lint=1, test=1, pre-commit=1)
	r.totalSteps = 4

	log.Info().Str("work_dir", workDir).Msg("starting validation pipeline")

	// Check context cancellation before starting
	select {
	case <-ctx.Done():
		return r.finalize(result, startTime), ctx.Err()
	default:
	}

	// Phase 1: Format (sequential, first)
	r.reportProgress("format", "starting")
	formatResults, err := r.runSequential(ctx, r.getFormatCommands(), workDir)
	result.FormatResults = formatResults
	if err != nil {
		r.reportProgress("format", "failed")
		result.FailedStepName = "format"
		log.Error().Err(err).Msg("format step failed")
		return r.finalize(result, startTime), err
	}
	r.reportProgress("format", "completed")

	// Check context cancellation between phases
	select {
	case <-ctx.Done():
		return r.finalize(result, startTime), ctx.Err()
	default:
	}

	// Check pre-commit availability before parallel phase
	preCommitInstalled, preCommitVersion, checkErr := r.getToolChecker().IsGoPreCommitInstalled(ctx)
	if checkErr != nil {
		log.Warn().Err(checkErr).Msg("failed to check go-pre-commit installation status")
		preCommitInstalled = false
	}

	// Phase 2: Lint + Pre-commit (parallel)
	r.reportProgress("lint", "starting")
	if preCommitInstalled {
		r.reportProgress("pre-commit", "starting")
	}
	lintResults, preCommitResults, parallelErr := r.runParallelLintPreCommit(ctx, workDir, preCommitInstalled, preCommitVersion, log)
	result.LintResults = lintResults
	result.PreCommitResults = preCommitResults
	if !preCommitInstalled {
		r.handlePreCommitSkipped(result, log)
	}
	if parallelErr != nil {
		r.handleParallelFailure(result, lintResults, preCommitResults, parallelErr, log)
		return r.finalize(result, startTime), parallelErr
	}
	r.reportProgress("lint", "completed")
	if preCommitInstalled {
		r.reportProgress("pre-commit", "completed")
	}

	// Check context cancellation between phases
	select {
	case <-ctx.Done():
		return r.finalize(result, startTime), ctx.Err()
	default:
	}

	// Phase 3: Test (sequential, last)
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

	testResults, err := r.runSequential(ctx, r.getTestCommands(), workDir)
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
	if result.SkippedSteps == nil {
		result.SkippedSteps = make([]string, 0, 1)
	}
	if result.SkipReasons == nil {
		result.SkipReasons = make(map[string]string)
	}
	result.SkippedSteps = append(result.SkippedSteps, "pre-commit")
	result.SkipReasons["pre-commit"] = "go-pre-commit not installed"
}

// runSequential executes commands in sequence, stopping on first failure.
func (r *Runner) runSequential(ctx context.Context, commands []string, workDir string) ([]Result, error) {
	if len(commands) == 0 {
		return nil, nil
	}
	return r.executor.Run(ctx, commands, workDir)
}

// runParallelLintPreCommit runs lint and pre-commit commands concurrently using errgroup.
// It collects results from both even if one fails, ensuring complete result data.
// IMPORTANT: We use the original ctx (not errgroup's derived context) and return nil
// from goroutines to prevent context cancellation when one fails - this ensures
// both results are always collected.
func (r *Runner) runParallelLintPreCommit(ctx context.Context, workDir string, preCommitInstalled bool, preCommitVersion string, log *zerolog.Logger) ([]Result, []Result, error) {
	var g errgroup.Group

	var lintResults, preCommitResults []Result
	var lintMu, preCommitMu sync.Mutex
	var lintErr, preCommitErr error

	lintCommands := r.getLintCommands()

	// Run lint commands - use original ctx to avoid cancellation from other goroutine
	g.Go(func() error {
		results, err := r.runCommandGroup(ctx, lintCommands, workDir)
		lintMu.Lock()
		lintResults = results
		lintErr = err
		lintMu.Unlock()
		return nil // Don't return error - we manage errors separately to collect all results
	})

	// Run pre-commit commands if installed - use original ctx to avoid cancellation from other goroutine
	if preCommitInstalled {
		g.Go(func() error {
			log.Info().Str("version", preCommitVersion).Msg("go-pre-commit detected")
			preCommitCommands := r.getPreCommitCommands()
			results, err := r.runCommandGroup(ctx, preCommitCommands, workDir)
			preCommitMu.Lock()
			preCommitResults = results
			preCommitErr = err
			preCommitMu.Unlock()

			// Stage any files modified by pre-commit hooks (auto-fixes)
			if err == nil {
				if stageErr := r.getStager().StageModifiedFiles(ctx, workDir); stageErr != nil {
					log.Warn().Err(stageErr).Msg("failed to stage pre-commit modified files")
					// Non-fatal - continue with validation
				}
			}
			return nil // Don't return error - we manage errors separately to collect all results
		})
	}

	// Wait for both to complete - will always return nil since goroutines return nil
	_ = g.Wait()

	// Return all collected results even on error
	// Use the first non-nil error (prefer lintErr if both failed)
	var returnErr error
	if lintErr != nil {
		returnErr = lintErr
	} else if preCommitErr != nil {
		returnErr = preCommitErr
	}

	return lintResults, preCommitResults, returnErr
}

// runCommandGroup executes a group of commands, used by parallel executor.
func (r *Runner) runCommandGroup(ctx context.Context, commands []string, workDir string) ([]Result, error) {
	if len(commands) == 0 {
		return nil, nil
	}
	return r.executor.Run(ctx, commands, workDir)
}

// stepNumber returns the 1-indexed step number for a given step name.
func stepNumber(step string) int {
	switch step {
	case "format":
		return 1
	case "lint":
		return 2
	case "pre-commit":
		return 3
	case "test":
		return 4
	default:
		return 0
	}
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
	case "completed", "failed":
		if startTime, ok := r.stepTimes.Load(step); ok {
			info.DurationMs = time.Since(startTime.(time.Time)).Milliseconds()
			r.stepTimes.Delete(step)
		}
	}

	// ElapsedMs is 0 for starting status (step just began)
	// For completed/failed, DurationMs contains the total time
	if status == "starting" {
		info.ElapsedMs = 0
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

// hasFailedResult checks if any result in the slice indicates failure.
func hasFailedResult(results []Result) bool {
	for _, r := range results {
		if !r.Success {
			return true
		}
	}
	return false
}

// handleParallelFailure handles the failure reporting for the parallel lint+pre-commit phase.
func (r *Runner) handleParallelFailure(result *PipelineResult, lintResults, preCommitResults []Result, _ error, log *zerolog.Logger) {
	lintFailed := hasFailedResult(lintResults)
	preCommitFailed := hasFailedResult(preCommitResults)

	reportStatus := func(step string, failed bool) {
		if failed {
			r.reportProgress(step, "failed")
		} else {
			r.reportProgress(step, "completed")
		}
	}

	reportStatus("lint", lintFailed)
	// Only report pre-commit status if it actually ran (has results)
	if len(preCommitResults) > 0 {
		reportStatus("pre-commit", preCommitFailed)
	}

	// Set failed step name to first failure
	switch {
	case lintFailed:
		result.FailedStepName = "lint"
	case preCommitFailed:
		result.FailedStepName = "pre-commit"
	}
	log.Error().Msg("parallel phase failed")
}
