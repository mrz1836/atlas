# Tech Debt: TaskEngine Function Decomposition

Status: pending

## Overview

The TaskEngine in `internal/task/engine.go` has several large functions that would benefit from decomposition into smaller, more focused functions. This improves testability, readability, and maintainability.

## Scope

### Task 1: Extract Step Execution Helpers from `runSteps`

**Current State:** `runSteps` (lines 327-374, 47 lines) does 5 things:
1. Loop through steps
2. Execute each step
3. Handle step results
4. Check if should pause
5. Save checkpoint

**Refactoring Plan:**

```go
// BEFORE: runSteps does everything
func (e *Engine) runSteps(ctx context.Context, task *domain.Task, template *domain.Template) error {
    for task.CurrentStep < len(template.Steps) {
        // ... 47 lines of mixed concerns
    }
}

// AFTER: Extract focused helpers

// executeCurrentStep executes the current step and returns result
func (e *Engine) executeCurrentStep(ctx context.Context, task *domain.Task, template *domain.Template) (*domain.StepResult, error)

// processStepResult handles the result and updates task state
func (e *Engine) processStepResult(ctx context.Context, task *domain.Task, result *domain.StepResult, step *domain.StepDefinition) error

// advanceToNextStep increments step counter and saves checkpoint
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

### Task 2: Extract Error Metadata Helper

**Current State:** Error metadata setting is duplicated in `handleStepError` and `HandleStepResult`:

```go
// Appears in both places:
task.Metadata = e.ensureMetadata(task.Metadata)
task.Metadata["last_error"] = result.Error
task.Metadata["retry_context"] = e.buildRetryContext(task, result)
```

**Refactoring Plan:**

```go
// Extract to a helper function
func (e *Engine) setErrorMetadata(task *domain.Task, stepName string, err error) {
    task.Metadata = e.ensureMetadata(task.Metadata)
    task.Metadata["last_error"] = err.Error()
    task.Metadata["failed_step"] = stepName
    task.Metadata["retry_context"] = e.buildRetryContext(task, &domain.StepResult{
        StepName: stepName,
        Error:    err.Error(),
    })
}
```

### Task 3: Simplify `transitionToErrorState`

**Current State:** Complex switch with redundant ValidationFailed logic:

```go
func (e *Engine) transitionToErrorState(...) error {
    switch task.Status {
    case constants.TaskStatusRunning:
        if targetStatus == constants.TaskStatusValidationFailed {
            // Running -> Validating -> ValidationFailed (2 steps)
        }
        // else direct transition
    case constants.TaskStatusValidating:
        if targetStatus == constants.TaskStatusValidationFailed {
            // Direct to ValidationFailed (1 step)
        }
    // ... many cases
    }
}
```

**Refactoring Plan:**

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

## Acceptance Criteria

1. **Given** the refactored `runSteps` **When** executing a task **Then** behavior is identical to before
2. **Given** error metadata helper **When** any step fails **Then** metadata is set consistently
3. **Given** simplified `transitionToErrorState` **When** any error occurs **Then** correct state transitions happen
4. All existing tests continue to pass
5. No new behavior introduced - pure refactoring
6. Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Validation Commands

```bash
# Before refactoring - capture baseline
go test -v ./internal/task/... -count=1 > /tmp/baseline.txt

# After refactoring - compare
go test -v ./internal/task/... -count=1 > /tmp/after.txt
diff /tmp/baseline.txt /tmp/after.txt  # Should only differ in timing

# Full validation
magex format:fix && magex lint && magex test:race
```

## Priority

P2 - Nice to have. Can be done during Epic 5 or as prep work.

## Estimated Effort

Small - 1-2 focused sessions

## Files to Modify

- `internal/task/engine.go` - Refactor functions
- `internal/task/engine_test.go` - Add tests for new helpers (optional, existing coverage should work)
