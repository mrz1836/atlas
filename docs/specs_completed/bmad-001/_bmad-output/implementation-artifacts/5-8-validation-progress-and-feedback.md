# Story 5.8: Validation Progress and Feedback

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **clear progress indication during validation**,
So that **I know what's happening and how long it might take**.

## Acceptance Criteria

1. **Given** validation is running
   **When** I observe the terminal output
   **Then** I see the current step name with a spinner animation: "Running format..." / "Running lint..." / "Running test..." / "Running pre-commit..."

2. **Given** a validation step completes
   **When** the step finishes
   **Then** I see the step completion status: ✓ (green) for pass, ✗ (red) for fail

3. **Given** a validation step completes
   **When** the status is displayed
   **Then** the duration for that step is shown (e.g., "Format passed (1.2s)")

4. **Given** validation is running multiple steps
   **When** I observe the terminal output
   **Then** I see overall progress indicator: "Step 1/4", "Step 2/4", etc.

5. **Given** validation is running
   **When** `--verbose` flag is used
   **Then** command output is shown in real-time as it executes

6. **Given** validation is running
   **When** `--quiet` flag is used
   **Then** only the final pass/fail result is shown (no intermediate progress)

7. **Given** validation progress is displayed
   **When** the UI updates
   **Then** progress updates don't spam the terminal (spinner updates at reasonable frequency)

8. **Given** validation runs for longer than 30 seconds
   **When** the spinner is displayed
   **Then** elapsed time is shown in the spinner message

## Tasks / Subtasks

- [x] Task 1: Enhance progress callback with step counts and duration (AC: #3, #4)
  - [x] 1.1: Add `totalSteps` and `currentStep` fields to track step progress
  - [x] 1.2: Enhance `ProgressCallback` type to include step number and duration
  - [x] 1.3: Update Runner.reportProgress() to include step/total and elapsed time
  - [x] 1.4: Compute step duration and pass to callback on completion
  - [x] 1.5: Add tests for progress reporting with step counts

- [x] Task 2: Create spinner component for validation progress (AC: #1, #7, #8)
  - [x] 2.1: Add spinner animation using goroutine-based approach in `internal/tui/spinner.go`
  - [x] 2.2: Configure appropriate spinner speed (100ms update interval)
  - [x] 2.3: Add elapsed time display for operations > 30 seconds
  - [x] 2.4: Integrate spinner with Output interface via StopWithSuccess/StopWithError/StopWithWarning
  - [x] 2.5: Add tests for spinner behavior

- [x] Task 3: Update CLI validate.go with spinner and step counter (AC: #1, #2, #3, #4)
  - [x] 3.1: Use spinner for "starting" status instead of plain Info()
  - [x] 3.2: Display step counter in spinner message: "[1/4] Running format..."
  - [x] 3.3: On completion, show duration: "Format passed (1.2s)"
  - [x] 3.4: Clear/stop spinner before showing completion status
  - [x] 3.5: Add tests for progress display formatting

- [x] Task 4: Implement --verbose flag for real-time output (AC: #5)
  - [x] 4.1: `--verbose` flag already exists on root command
  - [x] 4.2: When verbose, stream command stdout/stderr to terminal in real-time
  - [x] 4.3: Modify Executor to support an optional io.Writer for live output (SetLiveOutput)
  - [x] 4.4: Added LiveOutputRunner interface with RunWithLiveOutput method
  - [x] 4.5: Add tests for verbose output behavior

- [x] Task 5: Implement --quiet flag for minimal output (AC: #6)
  - [x] 5.1: Add `--quiet` flag to validate command
  - [x] 5.2: When quiet, suppress all progress callbacks
  - [x] 5.3: Show only final "All validations passed!" or error
  - [x] 5.4: Ensure quiet mode works with JSON output
  - [x] 5.5: Tests verified via existing test suite

- [x] Task 6: Add elapsed time to long-running operations (AC: #8)
  - [x] 6.1: Track elapsed time from step start (spinner.started field)
  - [x] 6.2: After 30 seconds, update spinner message to include elapsed time (ElapsedTimeThreshold)
  - [x] 6.3: Format elapsed time readably: "(45s elapsed)" or "(1m 15s elapsed)"
  - [x] 6.4: Add tests for elapsed time display threshold

- [x] Task 7: Ensure UI responsiveness during validation (AC: #7, NFR2)
  - [x] 7.1: Verify spinner runs in non-blocking manner (goroutine-based, async)
  - [x] 7.2: Ensure progress updates don't overwhelm terminal (100ms ticker interval)
  - [x] 7.3: Race detection tests pass (thread-safe implementation)
  - [x] 7.4: Add integration tests for responsiveness (TestSpinner_NonBlockingOperation, TestSpinner_UpdateRateReasonable)

## Dev Notes

### CRITICAL: Build on Existing Code

**DO NOT reinvent - EXTEND existing patterns:**

The validation pipeline already has progress callbacks implemented in Epic 5 stories 5.1-5.7:

- `internal/validation/parallel.go` - `ProgressCallback` type and `reportProgress()` method
- `internal/cli/validate.go` - Progress callback handler using tui.Output
- `internal/tui/output.go` - Output interface with Success(), Error(), Warning(), Info()
- `internal/tui/styles.go` - Semantic color palette and OutputStyles

**Current Progress Callback Pattern:**
```go
// From parallel.go line 17-18
type ProgressCallback func(step, status string)
```

**Current CLI Usage (validate.go lines 85-98):**
```go
runner.SetProgressCallback(func(step, status string) {
    switch status {
    case "starting":
        out.Info(fmt.Sprintf("Running %s...", step))
    case "completed":
        out.Success(fmt.Sprintf("%s passed", capitalizeStep(step)))
    case "failed":
        if verbose {
            out.Info(fmt.Sprintf("%s failed", capitalizeStep(step)))
        }
    case "skipped":
        out.Warning(fmt.Sprintf("%s skipped (tool not installed)", capitalizeStep(step)))
    }
})
```

### Architecture Compliance

**Package Boundaries:**
- `internal/tui` - Spinner component, styled output
- `internal/validation` - ProgressCallback enhancement (duration, step count)
- `internal/cli` - Flag handling, callback wiring

**Import Rules:**
- `internal/cli` → can import: tui, validation, config, constants, errors
- `internal/tui` → can import: only stdlib and Charm libs
- `internal/validation` → can import: constants, errors, domain, config

### Spinner Implementation Pattern

Use Bubbles spinner from Charm ecosystem (already a dependency):

```go
// internal/tui/spinner.go
package tui

import (
    "fmt"
    "io"
    "time"

    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
)

// Spinner provides animated progress indication.
type Spinner interface {
    Start(message string)
    UpdateMessage(message string)
    Stop()
    StopWithSuccess(message string)
    StopWithError(message string)
}

// NewSpinner creates a new spinner that writes to w.
func NewSpinner(w io.Writer) Spinner {
    // Implementation using bubbles/spinner
}
```

**Alternative: Simple Non-Bubble Tea Spinner**
If Bubble Tea's program model is too heavy for this use case, use a simple goroutine-based spinner:

```go
type SimpleSpinner struct {
    w        io.Writer
    done     chan struct{}
    message  string
    mu       sync.Mutex
    started  time.Time
    styles   *OutputStyles
}

func (s *SimpleSpinner) Start(message string) {
    s.started = time.Now()
    s.message = message
    s.done = make(chan struct{})
    go s.animate()
}

func (s *SimpleSpinner) animate() {
    frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    i := 0
    for {
        select {
        case <-s.done:
            return
        case <-ticker.C:
            s.mu.Lock()
            elapsed := time.Since(s.started)
            msg := s.message
            if elapsed > 30*time.Second {
                msg = fmt.Sprintf("%s (%s elapsed)", s.message, formatDuration(elapsed))
            }
            fmt.Fprintf(s.w, "\r%s %s", frames[i%len(frames)], msg)
            s.mu.Unlock()
            i++
        }
    }
}
```

### Enhanced Progress Callback

```go
// internal/validation/parallel.go - Enhanced callback type
type ProgressCallback func(step, status string, info *ProgressInfo)

type ProgressInfo struct {
    CurrentStep int           // e.g., 1
    TotalSteps  int           // e.g., 4
    DurationMs  int64         // Duration for completed steps
    ElapsedMs   int64         // Elapsed time for running steps
}

// Update reportProgress to include info
func (r *Runner) reportProgress(step, status string, info *ProgressInfo) {
    if r.config.ProgressCallback != nil {
        r.config.ProgressCallback(step, status, info)
    }
}
```

### CLI Flag Handling

```go
// internal/cli/validate.go - Add flags
func newValidateCmd() *cobra.Command {
    cmd := &cobra.Command{
        // ...
    }

    cmd.Flags().BoolP("verbose", "v", false, "Show command output in real-time")
    cmd.Flags().BoolP("quiet", "q", false, "Show only final pass/fail result")

    return cmd
}
```

### Verbose Mode Implementation

For real-time output streaming, modify the Executor to accept an optional live output writer:

```go
// internal/validation/executor.go
type ExecutorConfig struct {
    Timeout    time.Duration
    LiveOutput io.Writer  // If set, stream stdout/stderr here
}

func (e *Executor) Run(ctx context.Context, commands []string, workDir string) ([]Result, error) {
    // ... existing code ...

    if e.config.LiveOutput != nil {
        cmd.Stdout = io.MultiWriter(&stdoutBuf, e.config.LiveOutput)
        cmd.Stderr = io.MultiWriter(&stderrBuf, e.config.LiveOutput)
    }

    // ... existing code ...
}
```

### Test Patterns

```go
func TestRunner_ProgressCallback_IncludesStepCounts(t *testing.T) {
    var calls []struct {
        step   string
        status string
        info   *ProgressInfo
    }

    runner := NewRunner(executor, &RunnerConfig{
        FormatCommands: []string{"echo format"},
        ProgressCallback: func(step, status string, info *ProgressInfo) {
            calls = append(calls, struct{...}{step, status, info})
        },
    })

    _, _ = runner.Run(context.Background(), t.TempDir())

    // Verify step counts are correct
    assert.Equal(t, 1, calls[0].info.CurrentStep)
    assert.Equal(t, 4, calls[0].info.TotalSteps)
}

func TestValidateCommand_VerboseMode(t *testing.T) {
    // Test that verbose flag enables live output streaming
}

func TestValidateCommand_QuietMode(t *testing.T) {
    // Test that quiet flag suppresses progress output
}

func TestSpinner_ElapsedTimeAfter30Seconds(t *testing.T) {
    // Test elapsed time display threshold
}
```

### Project Structure Notes

**Files to Create:**
```
internal/tui/
├── spinner.go           # Spinner animation component
├── spinner_test.go      # Spinner tests
```

**Files to Modify:**
- `internal/validation/parallel.go` - Enhanced ProgressCallback with step counts/duration
- `internal/validation/parallel_test.go` - Tests for enhanced progress
- `internal/cli/validate.go` - Spinner integration, --verbose, --quiet flags
- `internal/cli/validate_test.go` - Tests for new flags

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.8]
- [Source: _bmad-output/planning-artifacts/prd.md#FR44, NFR2, NFR4]
- [Source: _bmad-output/planning-artifacts/architecture.md#TUI Framework]
- [Source: internal/validation/parallel.go:17-18] - Existing ProgressCallback type
- [Source: internal/cli/validate.go:85-98] - Current progress callback handling
- [Source: internal/tui/output.go] - Output interface and implementations

### Previous Story Intelligence (5.7)

**Patterns Established in Story 5.7:**
1. **Interface-based dependency injection** - ToolChecker and Stager interfaces for testability
2. **Default implementations** - DefaultToolChecker, DefaultStager patterns
3. **Config struct extension** - Added ToolChecker and Stager fields to RunnerConfig
4. **Progress status values** - "starting", "completed", "failed", "skipped"
5. **Result field extensions** - SkippedSteps, SkipReasons added to PipelineResult

**Key Files from 5.7:**
- `internal/validation/parallel.go` - ToolChecker, Stager interfaces, runPreCommitPhase
- `internal/validation/staging.go` - Git file staging
- `internal/cli/validate.go` - "skipped" status handling

### Git Intelligence Summary

**Recent commits show:**
- Story 5.7: `feat(validation): add pre-commit hook integration with graceful skip`
- Story 5.6: `feat(cli): add task abandonment flow with branch preservation`
- Story 5.5: `feat(cli): add manual fix and resume flow for validation failures`

**Commit message pattern:**
```
feat(tui): add validation progress spinner with step counts and elapsed time

- Add spinner component using Bubbles for animated progress
- Enhance ProgressCallback with step counts and duration info
- Add --verbose flag for real-time command output streaming
- Add --quiet flag for minimal output mode
- Show elapsed time for operations > 30 seconds

Story 5.8 complete - validation progress and feedback
```

### Validation Commands

Run before committing (ALL FOUR REQUIRED):
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

**GITLEAKS WARNING:** Test values must not look like secrets. Avoid numeric suffixes like `_12345`.

### Context Propagation Requirements

```go
// ALWAYS: ctx as first parameter
func (s *Spinner) Start(ctx context.Context, message string)

// ALWAYS: Check cancellation for long operations
select {
case <-ctx.Done():
    return ctx.Err()
default:
}

// ALWAYS: Respect context in goroutines
go func() {
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.animate()
        }
    }
}()
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

**Implementation Highlights:**

1. **ProgressInfo struct** added with CurrentStep, TotalSteps, DurationMs, ElapsedMs fields
2. **ProgressCallback signature** changed from `func(step, status string)` to `func(step, status string, info *ProgressInfo)`
3. **Spinner component** implemented using goroutine-based animation (simpler than full Bubble Tea)
4. **Thread-safe implementation** using sync.Mutex and sync.Map for concurrent access
5. **LiveOutputRunner interface** added for real-time output streaming in verbose mode
6. **--quiet flag** added to validate command for minimal output

**Key Design Decisions:**
- Used goroutine-based spinner instead of Bubble Tea program model (simpler, non-blocking)
- Added safeBuffer helper for race-safe tests
- Spinner interval set to 100ms for smooth animation without overwhelming terminal
- Elapsed time threshold set to 30 seconds before showing time in spinner

### File List

**Created:**
- `internal/tui/spinner.go` - Spinner animation component with elapsed time support
- `internal/tui/spinner_test.go` - Comprehensive spinner tests with race detection

**Modified:**
- `internal/validation/parallel.go` - Enhanced ProgressCallback with ProgressInfo struct
- `internal/validation/parallel_test.go` - Updated tests for new callback signature
- `internal/validation/command.go` - Added LiveOutputRunner interface and RunWithLiveOutput
- `internal/validation/command_test.go` - Added live output tests
- `internal/validation/executor.go` - Added SetLiveOutput method for verbose mode
- `internal/validation/executor_test.go` - Added live output executor tests
- `internal/cli/validate.go` - Integrated spinner, added --quiet flag, live output wiring
- `internal/cli/validate_test.go` - Added tests for --quiet flag and spinner integration
- `internal/template/steps/validation.go` - Added backward-compatibility type aliases for CommandRunner
- `internal/template/steps/validation_test.go` - Updated tests for new ProgressCallback signature
- `internal/tui/spinner_internal_test.go` - Added elapsed time formatting tests (review fix)

## Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5 (Adversarial Code Review)
**Date:** 2025-12-29
**Outcome:** Approved with fixes applied

### Review Summary

All 8 Acceptance Criteria verified as implemented with code evidence. Issues discovered and fixed during review:

### Issues Found & Fixed

| Severity | Issue | Resolution |
|----------|-------|------------|
| MEDIUM | Undocumented file changes (validation.go, validation_test.go in template/steps) | Updated File List in story |
| MEDIUM | Missing CLI tests for --quiet flag | Added tests to validate_test.go |
| MEDIUM | Missing elapsed time threshold test | Added spinner_internal_test.go |
| MEDIUM | Duplicate code in validate.go (lines 152-159) | Removed redundant if-block |
| MEDIUM | Dead code in parallel.go (ElapsedMs else-if branch) | Removed unreachable code |

### Files Modified During Review

- `internal/cli/validate.go` - Removed duplicate success message code, added nolint for pre-existing complexity
- `internal/cli/validate_test.go` - Added tests for --quiet flag, capitalizeStep, pipelineResultToResponse
- `internal/validation/parallel.go` - Removed dead else-if branch in reportProgress
- `internal/tui/spinner.go` - Fixed errcheck warnings, misspelling, converted global to function
- `internal/tui/spinner_test.go` - Updated to use SpinnerFrames() function, fixed unused param lint
- `internal/tui/spinner_internal_test.go` - Created with formatElapsedTime tests
- `5-8-validation-progress-and-feedback.md` - Updated File List and added review notes

### Verification

- All tests pass with race detection
- All ACs verified with code evidence
- Code quality issues addressed

