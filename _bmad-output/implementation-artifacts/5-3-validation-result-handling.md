# Story 5.3: Validation Result Handling

Status: done

## Story

As a **developer**,
I want **validation failures to pause the task and present options**,
So that **users can decide how to proceed when validation fails**.

## Acceptance Criteria

1. **Given** validation has run
   **When** validation fails
   **Then** the task transitions to `validation_failed` state

2. **Given** validation fails
   **When** the ValidationResult is saved
   **Then** the result is saved as artifact (validation.json)

3. **Given** previous validation attempts exist
   **When** saving a new validation result
   **Then** previous validation attempts are preserved (validation.1.json, validation.2.json, etc.)

4. **Given** validation fails
   **When** the system detects the failure
   **Then** the system emits terminal bell notification (FR28)

5. **Given** validation fails
   **When** user runs `atlas status`
   **Then** the task shows as needing attention

6. **Given** validation output is displayed
   **When** user views the failure
   **Then** the output clearly shows:
   - Which command(s) failed
   - The error output from each failure
   - Suggested next actions

7. **Given** validation passes
   **When** all validation commands succeed
   **Then** task auto-proceeds to next step

8. **Given** validation passes
   **When** ValidationResult is saved
   **Then** ValidationResult is saved as artifact (no user intervention required)

## Tasks / Subtasks

- [x] Task 1: Create validation result handler service (AC: #1, #2, #3, #7, #8)
  - [x] 1.1: Create `internal/validation/handler.go` with `ResultHandler` struct
  - [x] 1.2: Implement `Handle(ctx, task, result *PipelineResult) error` method
  - [x] 1.3: On failure: save result as artifact, transition task to validation_failed
  - [x] 1.4: On success: save result as artifact, return nil (auto-proceed)
  - [x] 1.5: Use `SaveVersionedArtifact` for validation.json to preserve history

- [x] Task 2: Integrate with Task Engine (AC: #1, #7)
  - [x] 2.1: Add validation step executor that uses `validation.Runner` and `ResultHandler`
  - [x] 2.2: Ensure task engine transitions through proper state machine path (Running → Validating → ValidationFailed)
  - [x] 2.3: Ensure successful validation allows task to auto-proceed

- [x] Task 3: Implement terminal bell notification (AC: #4)
  - [x] 3.1: Create `internal/tui/notification.go` with `NotifyBell()` function
  - [x] 3.2: Bell emits `\a` (BEL character) when validation fails
  - [x] 3.3: Check config for `notifications.bell: true/false` setting
  - [x] 3.4: Respect `--quiet` flag to suppress bell

- [x] Task 4: Format validation output for user (AC: #6)
  - [x] 4.1: Create `internal/validation/formatter.go` with `FormatResult(result *PipelineResult) string`
  - [x] 4.2: Format failed commands with clear headers
  - [x] 4.3: Include stderr/stdout from failed commands
  - [x] 4.4: Add suggested next actions (retry with AI fix, fix manually, abandon)
  - [x] 4.5: Use semantic colors for pass/fail indication

- [x] Task 5: Prepare TUI helpers for `atlas status` attention display (AC: #5 prep - command in Epic 7)
  - [x] 5.1: Add `TaskStatusIcon()` returning ⚠ for validation_failed
  - [x] 5.2: Add `SuggestedAction()` returning suggested CLI command
  - [x] 5.3: Add `IsAttentionStatus()` for sorting failed tasks to top

- [x] Task 6: Write comprehensive tests (AC: all)
  - [x] 6.1: Create `internal/validation/handler_test.go`
  - [x] 6.2: Test failure saves versioned artifact and transitions state
  - [x] 6.3: Test success saves artifact and returns nil
  - [x] 6.4: Test previous attempts are preserved
  - [x] 6.5: Test formatter output format
  - [x] 6.6: Test bell notification respects config
  - [x] 6.7: Run tests with `-race` flag

## Dev Notes

### CRITICAL: Build on Existing Code

**DO NOT reinvent - EXTEND existing validation code:**

Story 5.1 created `internal/validation/executor.go` with `Executor` and `Result` types.
Story 5.2 created `internal/validation/parallel.go` with `Runner` and `PipelineResult` types.

Your job is to CREATE the result handling layer that:
1. Takes a `PipelineResult` from `Runner.Run()`
2. Saves it as an artifact
3. Transitions task state appropriately
4. Emits notifications

### Existing Types to Use

From `internal/validation/result.go`:
```go
type Result struct {
    Command     string    `json:"command"`
    Success     bool      `json:"success"`
    ExitCode    int       `json:"exit_code"`
    Stdout      string    `json:"stdout"`
    Stderr      string    `json:"stderr"`
    DurationMs  int64     `json:"duration_ms"`
    Error       string    `json:"error,omitempty"`
    StartedAt   time.Time `json:"started_at"`
    CompletedAt time.Time `json:"completed_at"`
}

type PipelineResult struct {
    Success          bool     `json:"success"`
    FormatResults    []Result `json:"format_results"`
    LintResults      []Result `json:"lint_results"`
    TestResults      []Result `json:"test_results"`
    PreCommitResults []Result `json:"pre_commit_results"`
    DurationMs       int64    `json:"duration_ms"`
    FailedStepName   string   `json:"failed_step,omitempty"`
}
```

### Architecture Compliance

**Package Boundaries (from Architecture):**
- `internal/validation` → can import: constants, errors, config, domain
- `internal/validation` → must NOT import: cli, task, workspace, ai, git, template, tui

**Exception:** The handler needs to work with task store. Two approaches:
1. **Interface pattern** (RECOMMENDED): Handler takes a `ArtifactSaver` interface, task package implements
2. **Service boundary**: Handler returns action, caller (task engine) performs state change

**Recommended Interface Pattern:**
```go
// ArtifactSaver abstracts artifact persistence.
type ArtifactSaver interface {
    SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
}

// ResultHandler handles validation pipeline results.
type ResultHandler struct {
    artifactSaver ArtifactSaver
    notifier      Notifier
    logger        zerolog.Logger
}
```

### Task Store Integration

From `internal/task/store.go`, use:
```go
// SaveVersionedArtifact saves an artifact with version suffix (e.g., validation.1.json).
SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
```

This automatically handles:
- Creating artifacts directory
- Finding next available version number
- Atomic write

### State Machine Integration

From `internal/task/state.go`, the valid transition is:
```
Running → Validating → ValidationFailed
```

The task engine already handles this via:
```go
func (e *Engine) transitionToErrorState(ctx context.Context, task *domain.Task, stepType domain.StepType, reason string) error
```

For validation steps (`domain.StepTypeValidation`), this maps to `constants.TaskStatusValidationFailed`.

### Result Handler Design

```go
package validation

import (
    "context"
    "encoding/json"

    "github.com/rs/zerolog"
)

// ArtifactSaver abstracts artifact persistence.
type ArtifactSaver interface {
    SaveVersionedArtifact(ctx context.Context, workspaceName, taskID, baseName string, data []byte) (string, error)
}

// Notifier abstracts notification delivery.
type Notifier interface {
    Bell() // Emit terminal bell
}

// ResultHandler handles validation pipeline results.
type ResultHandler struct {
    saver    ArtifactSaver
    notifier Notifier
    logger   zerolog.Logger
}

// NewResultHandler creates a result handler.
func NewResultHandler(saver ArtifactSaver, notifier Notifier, logger zerolog.Logger) *ResultHandler {
    return &ResultHandler{
        saver:    saver,
        notifier: notifier,
        logger:   logger,
    }
}

// HandleResult processes a validation pipeline result.
// Returns nil if validation passed (task should auto-proceed).
// Returns ErrValidationFailed if validation failed (task should pause).
func (h *ResultHandler) HandleResult(ctx context.Context, workspaceName, taskID string, result *PipelineResult) error {
    // Save result as artifact
    data, err := json.MarshalIndent(result, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal validation result: %w", err)
    }

    filename, err := h.saver.SaveVersionedArtifact(ctx, workspaceName, taskID, "validation.json", data)
    if err != nil {
        return fmt.Errorf("failed to save validation artifact: %w", err)
    }

    h.logger.Info().
        Str("task_id", taskID).
        Str("artifact", filename).
        Bool("success", result.Success).
        Msg("saved validation result")

    if !result.Success {
        // Emit bell notification
        if h.notifier != nil {
            h.notifier.Bell()
        }
        return fmt.Errorf("%w: %s", atlaserrors.ErrValidationFailed, result.FailedStepName)
    }

    return nil
}
```

### Notification Design

```go
package tui

import (
    "fmt"
    "os"
)

// Notifier handles user notifications.
type Notifier struct {
    bellEnabled bool
    quiet       bool
    writer      io.Writer
}

// NewNotifier creates a notifier with the given settings.
func NewNotifier(bellEnabled bool, quiet bool) *Notifier {
    return &Notifier{
        bellEnabled: bellEnabled,
        quiet:       quiet,
        writer:      os.Stdout,
    }
}

// Bell emits a terminal bell if enabled.
func (n *Notifier) Bell() {
    if n.bellEnabled && !n.quiet {
        fmt.Fprint(n.writer, "\a")
    }
}
```

### Formatter Design

```go
package validation

import (
    "fmt"
    "strings"
)

// FormatResult formats a PipelineResult for human-readable display.
func FormatResult(result *PipelineResult) string {
    var sb strings.Builder

    if result.Success {
        sb.WriteString("✓ All validations passed\n")
        sb.WriteString(fmt.Sprintf("  Duration: %dms\n", result.DurationMs))
        return sb.String()
    }

    sb.WriteString(fmt.Sprintf("✗ Validation failed at: %s\n\n", result.FailedStepName))

    // Format each failed result
    for _, r := range result.AllResults() {
        if !r.Success {
            sb.WriteString(formatFailedCommand(r))
        }
    }

    sb.WriteString("\n### Suggested Actions\n\n")
    sb.WriteString("1. **Retry with AI fix** - Let AI attempt to fix the issues\n")
    sb.WriteString("2. **Fix manually** - Make changes in worktree, then `atlas resume`\n")
    sb.WriteString("3. **Abandon task** - End task, preserve branch\n")

    return sb.String()
}

func formatFailedCommand(r Result) string {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("### Command: %s\n", r.Command))
    sb.WriteString(fmt.Sprintf("Exit code: %d\n", r.ExitCode))
    if r.Stderr != "" {
        sb.WriteString("**Error output:**\n```\n")
        sb.WriteString(r.Stderr)
        sb.WriteString("\n```\n")
    }
    if r.Stdout != "" && len(r.Stdout) < 1000 {
        sb.WriteString("**Standard output:**\n```\n")
        sb.WriteString(r.Stdout)
        sb.WriteString("\n```\n")
    }
    return sb.String()
}
```

### Test Patterns

```go
func TestResultHandler_HandleResult_FailureSavesArtifactAndNotifies(t *testing.T) {
    mockSaver := &MockArtifactSaver{
        SaveVersionedArtifactFn: func(ctx context.Context, ws, task, base string, data []byte) (string, error) {
            assert.Equal(t, "validation.json", base)
            return "validation.1.json", nil
        },
    }
    mockNotifier := &MockNotifier{}
    logger := zerolog.Nop()

    handler := NewResultHandler(mockSaver, mockNotifier, logger)

    result := &PipelineResult{
        Success:        false,
        FailedStepName: "lint",
        LintResults:    []Result{{Command: "magex lint", Success: false, ExitCode: 1}},
    }

    err := handler.HandleResult(context.Background(), "ws", "task-123", result)

    assert.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
    assert.True(t, mockNotifier.BellCalled)
}

func TestResultHandler_HandleResult_SuccessAutoProceeds(t *testing.T) {
    mockSaver := &MockArtifactSaver{
        SaveVersionedArtifactFn: func(ctx context.Context, ws, task, base string, data []byte) (string, error) {
            return "validation.1.json", nil
        },
    }
    logger := zerolog.Nop()

    handler := NewResultHandler(mockSaver, nil, logger)

    result := &PipelineResult{Success: true}

    err := handler.HandleResult(context.Background(), "ws", "task-123", result)

    assert.NoError(t, err)
}
```

### Project Structure Notes

**Files to Create:**
```
internal/validation/
├── handler.go         # ResultHandler struct, HandleResult method
├── handler_test.go    # Tests for result handler
├── formatter.go       # FormatResult function for human output
├── formatter_test.go  # Tests for formatter

internal/tui/
├── notification.go    # Notifier struct with Bell() method
├── notification_test.go
```

**Files to Modify:**
- `internal/template/steps/validation.go` - Use ResultHandler after Runner.Run()
- `internal/cli/status.go` - Ensure validation_failed shows with attention icon

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns & Consistency Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.3]
- [Source: _bmad-output/planning-artifacts/prd.md#FR27-FR33]
- [Source: internal/validation/result.go] - Existing Result and PipelineResult types
- [Source: internal/validation/parallel.go] - Existing Runner
- [Source: internal/task/store.go] - SaveVersionedArtifact for artifact persistence
- [Source: internal/task/state.go] - Task state transitions
- [Source: internal/task/engine.go] - Task engine integration point

### Previous Story Intelligence (5.2)

**Patterns to Follow:**
1. **Interface-driven design** - ArtifactSaver interface enables mocking and package boundary respect
2. **Context propagation** - Always check ctx.Done() at operation boundaries
3. **Error wrapping** - Use `fmt.Errorf("%w: ...", atlaserrors.ErrValidationFailed, ...)`
4. **Test coverage target** - 90%+ on critical paths
5. **Run tests with `-race`** - Required for all concurrent code

**Code Patterns from 5.2:**
- `zerolog.Ctx(ctx)` for logger access
- `DurationMs int64` instead of `time.Duration` for JSON serialization
- Existing `PipelineResult.FailedStep()` and `AllResults()` methods

### Git Intelligence Summary

**Recent commits show:**
- Story 5.2 just completed with parallel validation runner
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

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

1. Created `internal/validation/handler.go` - ResultHandler with ArtifactSaver interface pattern
2. Created `internal/validation/handler_test.go` - Comprehensive tests for handler
3. Created `internal/validation/formatter.go` - FormatResult function for human output
4. Created `internal/validation/formatter_test.go` - Tests for formatter
5. Created `internal/tui/notification.go` - Notifier with Bell() method
6. Created `internal/tui/notification_test.go` - Tests for notifier
7. Added task status styling functions to `internal/tui/styles.go`
8. Updated `internal/template/steps/validation.go` to use parallel Runner and ResultHandler
9. Updated `internal/template/steps/defaults.go` to include ArtifactSaver and Notifier in ExecutorDeps
10. All tests pass with `-race` flag
11. All lint checks pass
12. **[Code Review Fix]** Updated `internal/cli/start.go` to wire up ArtifactSaver (taskStore) and Notifier via `NewDefaultRegistry` - enables AC#2, AC#3, AC#4 to work in production

### Change Log

- 2025-12-29: Story completed - implemented validation result handling with artifact persistence, bell notifications, and task engine integration
- 2025-12-29: **[Code Review]** Fixed critical integration bug - wired up ArtifactSaver and Notifier in start.go via NewDefaultRegistry

### File List

**New Files:**
- `internal/validation/handler.go`
- `internal/validation/handler_test.go`
- `internal/validation/formatter.go`
- `internal/validation/formatter_test.go`
- `internal/tui/notification.go`
- `internal/tui/notification_test.go`

**Modified Files:**
- `internal/tui/styles.go` - Added TaskStatusColors, TaskStatusIcon, IsAttentionStatus, SuggestedAction
- `internal/tui/styles_test.go` - Added tests for task status functions
- `internal/template/steps/validation.go` - Updated to use parallel Runner and ResultHandler
- `internal/template/steps/validation_test.go` - Updated tests for new parallel behavior
- `internal/template/steps/defaults.go` - Added ArtifactSaver and Notifier interfaces and ExecutorDeps fields
- `internal/cli/start.go` - Wired up ArtifactSaver and Notifier via NewDefaultRegistry (code review fix)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` - Sprint tracking updates
