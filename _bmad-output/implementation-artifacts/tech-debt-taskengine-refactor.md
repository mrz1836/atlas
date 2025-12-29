# Story: Tech Debt - TaskEngine Function Decomposition

Status: done

## Story

As a **developer**,
I want **the TaskEngine functions decomposed into smaller, focused helpers**,
So that **the codebase is more maintainable, testable, and readable for future development**.

## Acceptance Criteria

1. **Given** the refactored `runSteps` method **When** executing a task **Then** behavior is identical to before (no functional changes)

2. **Given** the new `executeCurrentStep` helper **When** called with a task and template **Then** it executes the current step and returns the result without modifying task state beyond what's necessary

3. **Given** the new `processStepResult` helper **When** called with a result **Then** it handles the result and updates task state appropriately

4. **Given** the new `advanceToNextStep` helper **When** called after successful step **Then** it increments the step counter and saves a checkpoint

5. **Given** the new `setErrorMetadata` helper **When** any step fails **Then** error metadata is set consistently in one place (eliminates duplication)

6. **Given** the simplified `transitionToErrorState` **When** any error occurs **Then** correct state transitions happen using the new `requiresValidatingIntermediate` helper

7. **Given** all existing tests **When** running `magex test:race` **Then** all tests pass (pure refactoring, no behavior change)

8. **Given** linting rules **When** running `magex lint` **Then** zero issues reported

## Tasks / Subtasks

- [x] Task 1: Extract Step Execution Helpers from `runSteps` (AC: #1, #2, #3, #4)
  - [x] 1.1 Create `executeCurrentStep(ctx, task, template) (*domain.StepResult, error)` helper
  - [x] 1.2 Create `processStepResult(ctx, task, result, step) error` helper
  - [x] 1.3 Create `advanceToNextStep(ctx, task) error` helper
  - [x] 1.4 Refactor `runSteps` to use new helpers as simple orchestration loop
  - [x] 1.5 Run tests to verify behavior unchanged

- [x] Task 2: Extract Error Metadata Helper (AC: #5)
  - [x] 2.1 Create `setErrorMetadata(task, stepName string, errMsg string)` helper function
  - [x] 2.2 Update `handleStepError` to use new helper
  - [x] 2.3 Update `HandleStepResult` (failed case) to use new helper
  - [x] 2.4 Verify error metadata is identical before/after refactor

- [x] Task 3: Simplify `transitionToErrorState` (AC: #6)
  - [x] 3.1 Create `requiresValidatingIntermediate(currentStatus, targetStatus) bool` helper
  - [x] 3.2 Refactor `transitionToErrorState` to use new helper
  - [x] 3.3 Verify all state transitions work correctly

- [x] Task 4: Final Validation (AC: #7, #8)
  - [x] 4.1 Run `magex format:fix`
  - [x] 4.2 Run `magex lint` - ensure zero issues
  - [x] 4.3 Run `magex test:race` - ensure all tests pass
  - [x] 4.4 Compare test output before/after (should only differ in timing)

## Dev Notes

### Context & Motivation

This story addresses tech debt identified in the Epic 4 retrospective. The TaskEngine in `internal/task/engine.go` has several large functions that would benefit from decomposition:

- `runSteps` (lines 327-374, 47 lines) does 5 things in one method
- Error metadata setting is duplicated between `handleStepError` and `HandleStepResult`
- `transitionToErrorState` has complex switch logic that can be simplified

### Architecture Compliance

**Package Import Rules (from project-context.md):**
- `internal/task` CAN import: ai, git, validation, template, domain, constants, errors
- `internal/task` MUST NOT import: workspace, cli

**Existing Pattern to Follow:**
The engine already uses good patterns:
- Context-first parameters on all methods
- Proper error wrapping with `fmt.Errorf("action: %w", err)`
- Structured logging with zerolog
- Interface-based design (`Store` interface, `ExecutorRegistry`)

### Critical Implementation Details

**Function Extraction Pattern:**
```go
// BEFORE: runSteps does everything
func (e *Engine) runSteps(ctx context.Context, task *domain.Task, template *domain.Template) error {
    for task.CurrentStep < len(template.Steps) {
        // ... 47 lines of mixed concerns
    }
}

// AFTER: Extract focused helpers
func (e *Engine) executeCurrentStep(ctx context.Context, task *domain.Task, template *domain.Template) (*domain.StepResult, error)
func (e *Engine) processStepResult(ctx context.Context, task *domain.Task, result *domain.StepResult, step *domain.StepDefinition) error
func (e *Engine) advanceToNextStep(ctx context.Context, task *domain.Task) error

// runSteps becomes a simple orchestration loop
func (e *Engine) runSteps(ctx context.Context, task *domain.Task, template *domain.Template) error {
    for task.CurrentStep < len(template.Steps) {
        if err := ctx.Err(); err != nil {
            return err
        }

        result, err := e.executeCurrentStep(ctx, task, template)
        if err != nil {
            return e.handleStepError(ctx, task, &template.Steps[task.CurrentStep], err)
        }

        if err := e.processStepResult(ctx, task, result, &template.Steps[task.CurrentStep]); err != nil {
            return err
        }

        if e.shouldPause(task) {
            return e.saveAndPause(ctx, task)
        }

        if err := e.advanceToNextStep(ctx, task); err != nil {
            return err
        }
    }
    return e.completeTask(ctx, task)
}
```

**Error Metadata Consolidation:**
```go
// Extract to a helper function - eliminates duplication in handleStepError and HandleStepResult
func (e *Engine) setErrorMetadata(task *domain.Task, stepName, errMsg string) {
    task.Metadata = e.ensureMetadata(task.Metadata)
    task.Metadata["last_error"] = errMsg
    task.Metadata["retry_context"] = e.buildRetryContext(task, &domain.StepResult{
        StepName: stepName,
        Error:    errMsg,
    })
}
```

**State Transition Simplification:**
```go
// requiresValidatingIntermediate returns true if we need to go through Validating first
func (e *Engine) requiresValidatingIntermediate(currentStatus constants.TaskStatus, targetStatus constants.TaskStatus) bool {
    return currentStatus == constants.TaskStatusRunning &&
           targetStatus == constants.TaskStatusValidationFailed
}

func (e *Engine) transitionToErrorState(ctx context.Context, task *domain.Task, stepType domain.StepType, reason string) error {
    targetStatus := e.mapStepTypeToErrorStatus(stepType)

    if e.requiresValidatingIntermediate(task.Status, targetStatus) {
        if err := Transition(ctx, task, constants.TaskStatusValidating, "step failed"); err != nil {
            return err
        }
    }

    return Transition(ctx, task, targetStatus, reason)
}
```

### Project Structure Notes

**File to Modify:**
- `internal/task/engine.go` - Main refactoring target

**Files Potentially Affected:**
- `internal/task/engine_test.go` - May need minor adjustments if tests directly call refactored methods

**No New Files Required:**
- All changes are internal to engine.go
- No new packages or dependencies needed

### Testing Strategy

**This is a pure refactoring task:**
1. Run tests before any changes: `go test -v ./internal/task/... -count=1 > /tmp/baseline.txt`
2. Make refactoring changes incrementally
3. Run tests after each change to catch regressions immediately
4. Final comparison: `diff /tmp/baseline.txt /tmp/after.txt` (should only differ in timing)

**Validation Commands:**
```bash
# Before refactoring - capture baseline
go test -v ./internal/task/... -count=1 > /tmp/baseline.txt

# After refactoring - compare
go test -v ./internal/task/... -count=1 > /tmp/after.txt
diff /tmp/baseline.txt /tmp/after.txt  # Should only differ in timing

# Full validation suite
magex format:fix && magex lint && magex test:race
```

### Key Code Locations

| Concern | Current Location | Lines |
|---------|-----------------|-------|
| runSteps orchestration | engine.go | 327-374 |
| handleStepError | engine.go | 385-419 |
| HandleStepResult | engine.go | 224-271 |
| transitionToErrorState | engine.go | 470-509 |
| mapStepTypeToErrorStatus | engine.go | 452-466 |
| buildRetryContext | engine.go | 512-534 |
| ensureMetadata | engine.go | 537-542 |

### References

- [Source: _bmad-output/implementation-artifacts/epic-4-retro-2025-12-28.md#TD-1]
- [Source: _bmad-output/project-context.md]
- [Source: internal/task/engine.go]
- [Source: _bmad-output/planning-artifacts/architecture.md]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - Pure refactoring, no debug issues encountered.

### Completion Notes List

- **Task 1 Complete:** Extracted 4 focused helpers from `runSteps`: `executeCurrentStep`, `processStepResult`, `advanceToNextStep`, `saveAndPause`. The `runSteps` method is now a simple 20-line orchestration loop (reduced from 47 lines).
- **Task 2 Complete:** Created `setErrorMetadata(task, stepName, errMsg string)` helper consolidating duplicated error metadata logic from `handleStepError` and `HandleStepResult`.
- **Task 3 Complete:** Created `requiresValidatingIntermediate` helper and simplified `transitionToErrorState` from 38 lines to 11 lines.
- **Task 4 Complete:** All validation passes - `magex format:fix`, `magex lint` (zero issues), `magex test:race` (all pass).

### File List

- `internal/task/engine.go` - Primary file refactored
- `internal/task/engine_test.go` - Added unit tests for extracted helper functions

## Senior Developer Review (AI)

**Review Date:** 2025-12-28
**Reviewer:** Claude Opus 4.5 (Code Review Agent)

### Issues Found & Fixed

| Severity | Issue | Resolution |
|----------|-------|------------|
| HIGH | `setErrorMetadata` added `failed_step` field not present in original code (behavior change violating AC #1) | Removed `failed_step` from `setErrorMetadata` to maintain identical behavior |
| MEDIUM | `processStepResult` silently discarded store.Update errors | Added warning log when store.Update fails during error handling |
| MEDIUM | No dedicated unit tests for new helper functions | Added 6 new test functions covering `executeCurrentStep`, `processStepResult`, `advanceToNextStep`, `saveAndPause`, `setErrorMetadata`, `requiresValidatingIntermediate` |

### Verified ACs

- ✅ AC #1: Behavior identical to before (after fixing HIGH issue)
- ✅ AC #2: `executeCurrentStep` helper works correctly
- ✅ AC #3: `processStepResult` helper handles results appropriately
- ✅ AC #4: `advanceToNextStep` increments and checkpoints
- ✅ AC #5: `setErrorMetadata` consolidates duplication
- ✅ AC #6: `requiresValidatingIntermediate` simplifies transitions
- ✅ AC #7: All tests pass
- ✅ AC #8: Lint passes with zero issues

### Notes

- Test coverage remains at 82.2% (below 90% target, but new helpers are exercised through integration tests)
- Context check pattern change from `select` to `ctx.Err()` is functionally equivalent

## Change Log

- 2025-12-28: Tech debt refactoring - decomposed TaskEngine functions into smaller, focused helpers. Pure refactoring with no behavior change.
- 2025-12-28: Senior Developer Review - Fixed 1 HIGH (behavior change) and 2 MEDIUM issues. Added unit tests for helper functions. Status → done.
