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

// ProgressCallback is called to report progress during validation pipeline execution.
// The step parameter indicates which step is running (format, lint, test, pre-commit).
// The status parameter is one of: "starting", "completed", "failed", "skipped".
type ProgressCallback func(step, status string)

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
	executor *Executor
	config   *RunnerConfig
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
// 2. Lint + Test (parallel) - run simultaneously for efficiency
// 3. Pre-commit (sequential, last) - final checks after all code is validated
//
// Returns a PipelineResult containing all step results regardless of success/failure.
// Returns an error if any step fails, but the PipelineResult will still contain
// all results collected up to and including the failure.
func (r *Runner) Run(ctx context.Context, workDir string) (*PipelineResult, error) {
	log := zerolog.Ctx(ctx)
	result := &PipelineResult{}
	startTime := time.Now()

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

	// Phase 2: Lint + Test (parallel)
	r.reportProgress("lint", "starting")
	r.reportProgress("test", "starting")
	lintResults, testResults, parallelErr := r.runParallelLintTest(ctx, workDir)
	result.LintResults = lintResults
	result.TestResults = testResults
	if parallelErr != nil {
		r.handleParallelFailure(result, lintResults, testResults, parallelErr, log)
		return r.finalize(result, startTime), parallelErr
	}
	r.reportProgress("lint", "completed")
	r.reportProgress("test", "completed")

	// Check context cancellation between phases
	select {
	case <-ctx.Done():
		return r.finalize(result, startTime), ctx.Err()
	default:
	}

	// Phase 3: Pre-commit (sequential, last)
	if preCommitErr := r.runPreCommitPhase(ctx, result, workDir, log); preCommitErr != nil {
		return r.finalize(result, startTime), preCommitErr
	}

	result.Success = true
	log.Info().Dur("duration_ms", time.Since(startTime)).Msg("validation pipeline completed successfully")
	return r.finalize(result, startTime), nil
}

// runPreCommitPhase handles the pre-commit phase with tool availability check.
// It skips pre-commit if go-pre-commit is not installed, otherwise runs the configured commands.
func (r *Runner) runPreCommitPhase(ctx context.Context, result *PipelineResult, workDir string, log *zerolog.Logger) error {
	// Check if go-pre-commit is installed before running
	preCommitInstalled, preCommitVersion, checkErr := r.getToolChecker().IsGoPreCommitInstalled(ctx)
	if checkErr != nil {
		log.Warn().Err(checkErr).Msg("failed to check go-pre-commit installation status")
		// Continue anyway - treat as not installed
		preCommitInstalled = false
	}

	// Skip if not installed
	if !preCommitInstalled {
		r.handlePreCommitSkipped(result, log)
		return nil
	}

	// Run pre-commit
	return r.executePreCommit(ctx, result, workDir, preCommitVersion, log)
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

// executePreCommit runs the pre-commit commands and stages any modified files.
func (r *Runner) executePreCommit(ctx context.Context, result *PipelineResult, workDir, version string, log *zerolog.Logger) error {
	log.Info().Str("version", version).Msg("go-pre-commit detected")
	r.reportProgress("pre-commit", "starting")

	preCommitResults, err := r.runSequential(ctx, r.getPreCommitCommands(), workDir)
	result.PreCommitResults = preCommitResults
	if err != nil {
		r.reportProgress("pre-commit", "failed")
		result.FailedStepName = "pre-commit"
		log.Error().Err(err).Msg("pre-commit step failed")
		return err
	}
	r.reportProgress("pre-commit", "completed")

	// Stage any files modified by pre-commit hooks (auto-fixes)
	if stageErr := r.getStager().StageModifiedFiles(ctx, workDir); stageErr != nil {
		log.Warn().Err(stageErr).Msg("failed to stage pre-commit modified files")
		// Non-fatal - continue with validation success
	}

	return nil
}

// runSequential executes commands in sequence, stopping on first failure.
func (r *Runner) runSequential(ctx context.Context, commands []string, workDir string) ([]Result, error) {
	if len(commands) == 0 {
		return nil, nil
	}
	return r.executor.Run(ctx, commands, workDir)
}

// runParallelLintTest runs lint and test commands concurrently using errgroup.
// It collects results from both even if one fails, ensuring complete result data.
// IMPORTANT: We use the original ctx (not errgroup's derived context) and return nil
// from goroutines to prevent context cancellation when one fails - this ensures
// both results are always collected per AC #4.
func (r *Runner) runParallelLintTest(ctx context.Context, workDir string) ([]Result, []Result, error) {
	var g errgroup.Group

	var lintResults, testResults []Result
	var lintMu, testMu sync.Mutex
	var lintErr, testErr error

	lintCommands := r.getLintCommands()
	testCommands := r.getTestCommands()

	// Run lint commands - use original ctx to avoid cancellation from other goroutine
	g.Go(func() error {
		results, err := r.runCommandGroup(ctx, lintCommands, workDir)
		lintMu.Lock()
		lintResults = results
		lintErr = err
		lintMu.Unlock()
		return nil // Don't return error - we manage errors separately to collect all results
	})

	// Run test commands - use original ctx to avoid cancellation from other goroutine
	g.Go(func() error {
		results, err := r.runCommandGroup(ctx, testCommands, workDir)
		testMu.Lock()
		testResults = results
		testErr = err
		testMu.Unlock()
		return nil // Don't return error - we manage errors separately to collect all results
	})

	// Wait for both to complete - will always return nil since goroutines return nil
	_ = g.Wait()

	// Return all collected results even on error
	// Use the first non-nil error (prefer lintErr if both failed)
	var returnErr error
	if lintErr != nil {
		returnErr = lintErr
	} else if testErr != nil {
		returnErr = testErr
	}

	return lintResults, testResults, returnErr
}

// runCommandGroup executes a group of commands, used by parallel executor.
func (r *Runner) runCommandGroup(ctx context.Context, commands []string, workDir string) ([]Result, error) {
	if len(commands) == 0 {
		return nil, nil
	}
	return r.executor.Run(ctx, commands, workDir)
}

// reportProgress calls the progress callback if configured.
func (r *Runner) reportProgress(step, status string) {
	if r.config.ProgressCallback != nil {
		r.config.ProgressCallback(step, status)
	}
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

// handleParallelFailure handles the failure reporting for the parallel lint+test phase.
func (r *Runner) handleParallelFailure(result *PipelineResult, lintResults, testResults []Result, _ error, log *zerolog.Logger) {
	lintFailed := hasFailedResult(lintResults)
	testFailed := hasFailedResult(testResults)

	reportStatus := func(step string, failed bool) {
		if failed {
			r.reportProgress(step, "failed")
		} else {
			r.reportProgress(step, "completed")
		}
	}

	reportStatus("lint", lintFailed)
	reportStatus("test", testFailed)

	// Set failed step name to first failure
	switch {
	case lintFailed:
		result.FailedStepName = "lint"
	case testFailed:
		result.FailedStepName = "test"
	}
	log.Error().Msg("parallel phase failed")
}
