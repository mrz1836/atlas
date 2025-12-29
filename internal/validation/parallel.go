package validation

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/constants"
)

// ProgressCallback is called to report progress during validation pipeline execution.
// The step parameter indicates which step is running (format, lint, test, pre-commit).
// The status parameter is one of: "starting", "completed", "failed".
type ProgressCallback func(step, status string)

// RunnerConfig holds configuration for the validation pipeline.
type RunnerConfig struct {
	FormatCommands    []string
	LintCommands      []string
	TestCommands      []string
	PreCommitCommands []string
	ProgressCallback  ProgressCallback
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
	r.reportProgress("pre-commit", "starting")
	preCommitResults, err := r.runSequential(ctx, r.getPreCommitCommands(), workDir)
	result.PreCommitResults = preCommitResults
	if err != nil {
		r.reportProgress("pre-commit", "failed")
		result.FailedStepName = "pre-commit"
		log.Error().Err(err).Msg("pre-commit step failed")
		return r.finalize(result, startTime), err
	}
	r.reportProgress("pre-commit", "completed")

	result.Success = true
	log.Info().Dur("duration_ms", time.Since(startTime)).Msg("validation pipeline completed successfully")
	return r.finalize(result, startTime), nil
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
