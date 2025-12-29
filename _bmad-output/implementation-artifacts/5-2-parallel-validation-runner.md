# Story 5.2: Parallel Validation Runner

Status: ready-for-dev

## Story

As a **developer**,
I want **validation commands to run in optimal order with parallelization**,
So that **validation completes as quickly as possible**.

## Acceptance Criteria

1. **Given** the command executor exists (from Story 5.1)
   **When** I implement `internal/validation/parallel.go`
   **Then** the validation pipeline runs in this order:
   1. **Format** (sequential, first) - `magex format:fix`
   2. **Lint + Test** (parallel) - `magex lint` and `magex test` simultaneously
   3. **Pre-commit** (sequential, last) - `go-pre-commit run --all-files`

2. **Given** format and pre-commit run sequentially
   **When** parallel validation is executed
   **Then** errgroup is used for parallel lint and test execution

3. **Given** format step is first
   **When** format fails
   **Then** subsequent steps (lint, test, pre-commit) are skipped

4. **Given** lint and test run in parallel
   **When** either fails
   **Then** all parallel results are collected before returning error

5. **Given** validation steps produce results
   **When** validation completes (pass or fail)
   **Then** total validation result aggregates all step results

6. **Given** validation is running
   **When** user observes progress
   **Then** progress is reported for each step (starting, completed, failed)

7. **Given** all steps complete successfully
   **When** validation finishes
   **Then** total success is reported with combined results from all steps

## Tasks / Subtasks

- [ ] Task 1: Create `internal/validation/parallel.go` (AC: #1, #2)
  - [ ] 1.1: Define `Runner` struct with `Executor` and config dependencies
  - [ ] 1.2: Define `RunnerConfig` struct with command lists for format, lint, test, pre-commit
  - [ ] 1.3: Create `NewRunner(executor *Executor, config *RunnerConfig) *Runner`
  - [ ] 1.4: Define `PipelineResult` struct aggregating step results

- [ ] Task 2: Implement `Runner.Run()` method (AC: #1, #3, #4, #5)
  - [ ] 2.1: Accept `ctx context.Context, workDir string` parameters
  - [ ] 2.2: Implement sequential format step first with early exit on failure
  - [ ] 2.3: Implement parallel lint + test using `errgroup` and `sync.WaitGroup`
  - [ ] 2.4: Implement sequential pre-commit step last
  - [ ] 2.5: Collect all results into `PipelineResult` regardless of success/failure
  - [ ] 2.6: Check context cancellation between major phases

- [ ] Task 3: Implement step execution helpers (AC: #1, #3, #4)
  - [ ] 3.1: Create `runSequential(ctx, commands, workDir) ([]Result, error)` helper
  - [ ] 3.2: Create `runParallel(ctx, lintCmds, testCmds, workDir) ([]Result, []Result, error)` helper
  - [ ] 3.3: Ensure parallel runner collects all results even when one fails

- [ ] Task 4: Implement progress reporting (AC: #6, #7)
  - [ ] 4.1: Add `ProgressCallback func(step string, status string)` to `RunnerConfig`
  - [ ] 4.2: Report "starting" for each step before execution
  - [ ] 4.3: Report "completed" or "failed" for each step after execution
  - [ ] 4.4: Log step transitions using zerolog with structured fields

- [ ] Task 5: Define result types (AC: #5, #7)
  - [ ] 5.1: Create `PipelineResult` struct in `internal/validation/result.go`
  - [ ] 5.2: Include: Success, FormatResults, LintResults, TestResults, PreCommitResults, Duration
  - [ ] 5.3: Add `AllResults() []Result` method for flat result list
  - [ ] 5.4: Add `FailedStep() string` method returning first failed step name

- [ ] Task 6: Write comprehensive tests (AC: all)
  - [ ] 6.1: Create `internal/validation/parallel_test.go`
  - [ ] 6.2: Test successful full pipeline (format → lint+test → pre-commit)
  - [ ] 6.3: Test format failure skips subsequent steps
  - [ ] 6.4: Test lint failure during parallel phase collects test results
  - [ ] 6.5: Test test failure during parallel phase collects lint results
  - [ ] 6.6: Test context cancellation stops pipeline
  - [ ] 6.7: Test progress callback is invoked for each step
  - [ ] 6.8: Test empty command lists use defaults
  - [ ] 6.9: Run all tests with `-race` flag

- [ ] Task 7: Integrate with CLI validate command (AC: #1)
  - [ ] 7.1: Update `internal/cli/validate.go` to use `validation.Runner`
  - [ ] 7.2: Simplify CLI validate to delegate to `Runner.Run()`
  - [ ] 7.3: Wire progress callback to TUI output
  - [ ] 7.4: Ensure backward compatibility with existing behavior

## Dev Notes

### CRITICAL: Build on Existing Code

**DO NOT reinvent - REFACTOR existing code in `internal/cli/validate.go`:**

The CLI validate command already implements the parallel validation logic:
- Format runs first sequentially
- Lint and test run in parallel using `errgroup`
- Pre-commit runs last sequentially

**Your job is to EXTRACT this logic into `internal/validation/parallel.go`**, making it reusable by both CLI and the Task Engine.

### Existing Parallel Implementation (from `internal/cli/validate.go`)

```go
// Current pattern in CLI - extract and generalize this:
g, gCtx := errgroup.WithContext(ctx)
var lintResults, testResults []CommandResult
var lintMu, testMu sync.Mutex

g.Go(func() error {
    return runParallelCommands(gCtx, lintCmds, "lint", workDir, runner, ...)
})

g.Go(func() error {
    return runParallelCommands(gCtx, testCmds, "test", workDir, runner, ...)
})

if err := g.Wait(); err != nil {
    // Collect all results even on failure
}
```

### Architecture Compliance

**Package Boundaries (from Architecture):**
- `internal/validation` → can import: constants, errors, config, domain
- `internal/validation` → must NOT import: cli, task, workspace, ai, git, template, tui

**Import Pattern:**
```go
import (
    "context"
    "fmt"
    "sync"
    "time"

    "golang.org/x/sync/errgroup"
    "github.com/rs/zerolog"

    "github.com/mrz1836/atlas/internal/constants"
    atlaserrors "github.com/mrz1836/atlas/internal/errors"
)
```

### Runner Interface Design

```go
// RunnerConfig holds configuration for the validation pipeline.
type RunnerConfig struct {
    FormatCommands    []string
    LintCommands      []string
    TestCommands      []string
    PreCommitCommands []string
    ProgressCallback  func(step, status string) // Optional callback
}

// Runner orchestrates the validation pipeline with parallel execution.
type Runner struct {
    executor *Executor
    config   *RunnerConfig
}

// NewRunner creates a validation pipeline runner.
func NewRunner(executor *Executor, config *RunnerConfig) *Runner

// NewRunnerFromConfig creates a runner from atlas config.
func NewRunnerFromConfig(executor *Executor, cfg *config.Config) *Runner

// Run executes the full validation pipeline.
func (r *Runner) Run(ctx context.Context, workDir string) (*PipelineResult, error)
```

### PipelineResult Structure

```go
// PipelineResult aggregates results from all validation steps.
type PipelineResult struct {
    Success           bool          `json:"success"`
    FormatResults     []Result      `json:"format_results"`
    LintResults       []Result      `json:"lint_results"`
    TestResults       []Result      `json:"test_results"`
    PreCommitResults  []Result      `json:"pre_commit_results"`
    Duration          time.Duration `json:"duration_ms"`
    FailedStepName    string        `json:"failed_step,omitempty"`
}

// AllResults returns a flat list of all results.
func (p *PipelineResult) AllResults() []Result

// FailedStep returns the name of the first failed step.
func (p *PipelineResult) FailedStep() string
```

### Parallel Execution Pattern

```go
func (r *Runner) runParallelLintTest(ctx context.Context, workDir string) ([]Result, []Result, error) {
    g, gCtx := errgroup.WithContext(ctx)

    var lintResults, testResults []Result
    var lintMu, testMu sync.Mutex
    var lintErr, testErr error

    // Run lint
    g.Go(func() error {
        results, err := r.runCommandGroup(gCtx, r.config.LintCommands, workDir)
        lintMu.Lock()
        lintResults = results
        lintErr = err
        lintMu.Unlock()
        return err
    })

    // Run test
    g.Go(func() error {
        results, err := r.runCommandGroup(gCtx, r.config.TestCommands, workDir)
        testMu.Lock()
        testResults = results
        testErr = err
        testMu.Unlock()
        return err
    })

    // Wait for both - errgroup.Wait() returns first error
    err := g.Wait()

    // Return all collected results even on error
    return lintResults, testResults, err
}
```

### Progress Callback Pattern

```go
func (r *Runner) Run(ctx context.Context, workDir string) (*PipelineResult, error) {
    result := &PipelineResult{}
    startTime := time.Now()

    // Format
    r.reportProgress("format", "starting")
    formatResults, err := r.runSequential(ctx, r.config.FormatCommands, workDir)
    result.FormatResults = formatResults
    if err != nil {
        r.reportProgress("format", "failed")
        result.FailedStepName = "format"
        return r.finalize(result, startTime), err
    }
    r.reportProgress("format", "completed")

    // Lint + Test (parallel)
    r.reportProgress("lint", "starting")
    r.reportProgress("test", "starting")
    lintResults, testResults, err := r.runParallelLintTest(ctx, workDir)
    result.LintResults = lintResults
    result.TestResults = testResults
    if err != nil {
        // Determine which failed
        r.reportProgress("lint", lintStatus)
        r.reportProgress("test", testStatus)
        return r.finalize(result, startTime), err
    }
    r.reportProgress("lint", "completed")
    r.reportProgress("test", "completed")

    // Pre-commit
    r.reportProgress("pre-commit", "starting")
    preCommitResults, err := r.runSequential(ctx, r.config.PreCommitCommands, workDir)
    result.PreCommitResults = preCommitResults
    if err != nil {
        r.reportProgress("pre-commit", "failed")
        result.FailedStepName = "pre-commit"
        return r.finalize(result, startTime), err
    }
    r.reportProgress("pre-commit", "completed")

    result.Success = true
    return r.finalize(result, startTime), nil
}
```

### Default Commands from Constants

Use existing constants from `internal/constants/constants.go`:

```go
const (
    DefaultFormatCommand    = "magex format:fix"
    DefaultLintCommand      = "magex lint"
    DefaultTestCommand      = "magex test"
    DefaultPreCommitCommand = "go-pre-commit run --all-files"
)
```

**Action:** Use these when command lists are empty:
```go
func applyDefaults(cmds []string, defaultCmd string) []string {
    if len(cmds) == 0 {
        return []string{defaultCmd}
    }
    return cmds
}
```

### CLI Integration Pattern

Update `internal/cli/validate.go` to use the new Runner:

```go
func runValidate(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
    cfg, err := config.Load(ctx)
    if err != nil {
        cfg = config.DefaultConfig()
    }

    workDir, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("failed to get working directory: %w", err)
    }

    executor := validation.NewExecutor(cfg.Validation.Timeout)
    runner := validation.NewRunnerFromConfig(executor, cfg)

    // Set up progress callback for TUI
    runner.SetProgressCallback(func(step, status string) {
        switch status {
        case "starting":
            out.Info(fmt.Sprintf("Running %s...", step))
        case "completed":
            out.Success(fmt.Sprintf("%s passed", step))
        case "failed":
            out.Error(fmt.Errorf("%s failed", step))
        }
    })

    result, err := runner.Run(ctx, workDir)

    if outputFormat == OutputJSON {
        return out.JSON(result)
    }

    if err != nil {
        return handleValidationFailure(out, result)
    }

    out.Success("All validations passed!")
    return nil
}
```

### Test Patterns

**MockExecutor for testing:**

```go
type MockExecutor struct {
    Results map[string]*Result // command -> result
}

func (m *MockExecutor) RunSingle(ctx context.Context, cmd, workDir string) (*Result, error) {
    if result, ok := m.Results[cmd]; ok {
        if !result.Success {
            return result, errors.ErrValidationFailed
        }
        return result, nil
    }
    return &Result{Command: cmd, Success: true}, nil
}
```

**Test naming convention:**
```go
func TestRunner_Run_FullPipelineSuccess(t *testing.T)
func TestRunner_Run_FormatFailureSkipsRest(t *testing.T)
func TestRunner_Run_ParallelCollectsAllResults(t *testing.T)
func TestRunner_Run_ContextCancellation(t *testing.T)
func TestRunner_Run_ProgressCallbackInvoked(t *testing.T)
```

### Project Structure Notes

**Files to Create:**
```
internal/validation/
├── parallel.go         # Runner struct, Run method, parallel execution
├── parallel_test.go    # Comprehensive tests for parallel runner
```

**Files to Modify:**
- `internal/validation/result.go` - Add PipelineResult type
- `internal/cli/validate.go` - Simplify to use validation.Runner

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns & Consistency Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.2]
- [Source: _bmad-output/planning-artifacts/prd.md#FR27-FR33]
- [Source: internal/validation/executor.go] - Existing Executor
- [Source: internal/cli/validate.go] - Existing parallel implementation to extract
- [Source: internal/constants/constants.go] - Default validation commands

### Previous Story Intelligence (5.1)

**Patterns to Follow:**
1. **Interface-driven design** - CommandRunner interface enables mocking
2. **Context propagation** - Always check ctx.Done() at operation boundaries
3. **Atomic writes** - Result files should use write-then-rename pattern
4. **Test coverage target** - 90%+ on critical paths
5. **Run tests with `-race`** - Required for all concurrent code
6. **Error wrapping** - Use `fmt.Errorf("%w: ...", atlaserrors.ErrValidationFailed, ...)`

**Code Patterns from 5.1:**
- `zerolog.Ctx(ctx)` for logger access
- `DurationMs int64` instead of `time.Duration` for JSON serialization
- `select { case <-ctx.Done(): return ctx.Err() default: }` for cancellation checks

### Git Intelligence Summary

**Recent commits show:**
- Story 5.1 just completed with validation command executor
- Pattern: Feature commits use `feat(scope): description` format
- All Epics 1-4 are done, Epic 5 is in-progress

### Validation Commands

Run before committing (ALL FOUR REQUIRED):
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

**GITLEAKS WARNING:** Test values must not look like secrets. Avoid numeric suffixes like `_12345`.

## Dev Agent Record

### Agent Model Used

{{agent_model_name_version}}

### Debug Log References

### Completion Notes List

### File List
