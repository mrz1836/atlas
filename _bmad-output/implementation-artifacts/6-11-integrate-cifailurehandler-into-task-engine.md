# Story 6.11: Integrate CIFailureHandler into Task Engine

Status: draft

## Story

As a **user**,
I want **the task engine to automatically invoke the CI failure handler when CI fails**,
So that **I am presented with options to view logs, retry, fix manually, or abandon**.

## Acceptance Criteria

1. **Given** a step returns `ci_failed` failure type, **When** the task engine processes the result, **Then** the engine invokes `CIFailureHandler` to present options.

2. **Given** the user selects "View logs", **When** the action is processed, **Then** the system opens the GitHub Actions URL in the browser and returns to the options menu.

3. **Given** the user selects "Retry from implement", **When** the action is processed, **Then** the system:
   - Extracts error context from CI result
   - Transitions task to running state
   - Resumes from the implement step with error context

4. **Given** the user selects "Fix manually", **When** the action is processed, **Then** the system:
   - Displays worktree path and instructions
   - Transitions task to awaiting manual fix
   - Instructions include `atlas resume <workspace>` command

5. **Given** the user selects "Abandon task", **When** the action is processed, **Then** the system:
   - Converts PR to draft (if possible)
   - Transitions task to abandoned state
   - Preserves branch and worktree for manual work

6. **Given** a step returns `gh_failed` failure type (push/PR failure), **When** the task engine processes the result, **Then** the engine presents similar options (retry, fix, abandon).

7. **Given** a step returns `ci_timeout` failure type, **When** the task engine processes the result, **Then** the engine presents options including "Continue waiting".

8. **Given** the user fixes CI manually in GitHub, **When** they run `atlas resume <workspace>`, **Then** the system resumes from the ci_wait step and continues monitoring.

## Tasks / Subtasks

- [ ] Task 1: Add CI failure handling to task engine (AC: 1, 6, 7)
  - [ ] 1.1: Add `ciFailureHandler *task.CIFailureHandler` field to TaskEngine
  - [ ] 1.2: Inject handler via engine constructor or option
  - [ ] 1.3: In `processStepResult()`, check for failure_type in result metadata
  - [ ] 1.4: If failure_type is ci_failed, invoke CI failure handling flow
  - [ ] 1.5: If failure_type is gh_failed, invoke GitHub failure handling flow
  - [ ] 1.6: If failure_type is ci_timeout, invoke timeout handling flow

- [ ] Task 2: Implement CI failure handling flow (AC: 1, 2, 3, 4, 5)
  - [ ] 2.1: Create `handleCIFailure(ctx, task, result) error` method on TaskEngine
  - [ ] 2.2: Extract CIWatchResult from step result metadata
  - [ ] 2.3: For now, return awaiting_approval status to trigger menu (Epic 8 will add interactive menu)
  - [ ] 2.4: Store failure context in task for action processing
  - [ ] 2.5: Log failure details for debugging

- [ ] Task 3: Implement action processing (AC: 2, 3, 4, 5)
  - [ ] 3.1: Create `ProcessCIFailureAction(ctx, task, action) error` method
  - [ ] 3.2: Handle ViewLogs: call `CIFailureHandler.HandleCIFailure()` with ViewLogs action
  - [ ] 3.3: Handle RetryFromImplement: update task step to implement, set error context
  - [ ] 3.4: Handle FixManually: update task status, store instructions in result
  - [ ] 3.5: Handle Abandon: call `CIFailureHandler.HandleCIFailure()` with Abandon action

- [ ] Task 4: Implement GitHub failure handling (AC: 6)
  - [ ] 4.1: Create `handleGHFailure(ctx, task, result) error` method
  - [ ] 4.2: Extract error details from step result
  - [ ] 4.3: Present options: Retry now, Fix and retry, Abandon
  - [ ] 4.4: Handle retry: re-execute the failed step (push or PR)
  - [ ] 4.5: Handle abandon: transition to abandoned state

- [ ] Task 5: Implement timeout handling (AC: 7)
  - [ ] 5.1: Create `handleCITimeout(ctx, task, result) error` method
  - [ ] 5.2: Present options: Continue waiting, Retry, Fix manually, Abandon
  - [ ] 5.3: Handle continue_waiting: restart CI monitoring with extended timeout
  - [ ] 5.4: Other options same as CI failure handling

- [ ] Task 6: Implement resume support (AC: 8)
  - [ ] 6.1: Ensure task state is persisted with current step info
  - [ ] 6.2: In `Resume()` method, check if task was in ci_failed/ci_timeout state
  - [ ] 6.3: If resuming from CI failure, restart from ci_wait step
  - [ ] 6.4: Preserve PR number and other context for continued monitoring

- [ ] Task 7: Update state machine transitions (AC: all)
  - [ ] 7.1: Verify transition: Running → CIFailed (on CI failure)
  - [ ] 7.2: Verify transition: CIFailed → Running (on retry from implement)
  - [ ] 7.3: Verify transition: CIFailed → Running (on resume after manual fix)
  - [ ] 7.4: Verify transition: CIFailed → Abandoned (on abandon)
  - [ ] 7.5: Verify transition: Running → GHFailed (on GitHub operation failure)
  - [ ] 7.6: Verify transition: GHFailed → Running (on retry)
  - [ ] 7.7: Verify transition: Running → CITimeout (on CI timeout)

- [ ] Task 8: Create comprehensive tests (AC: 1-8)
  - [ ] 8.1: Test CI failure triggers handler invocation
  - [ ] 8.2: Test ViewLogs action opens browser
  - [ ] 8.3: Test RetryFromImplement returns to implement step
  - [ ] 8.4: Test FixManually sets correct status and instructions
  - [ ] 8.5: Test Abandon transitions to abandoned state
  - [ ] 8.6: Test GH failure handling (push failure)
  - [ ] 8.7: Test GH failure handling (PR creation failure)
  - [ ] 8.8: Test CI timeout handling
  - [ ] 8.9: Test continue waiting extends timeout
  - [ ] 8.10: Test resume from ci_failed state
  - [ ] 8.11: Test state machine transitions
  - [ ] 8.12: Target 90%+ coverage for new code

## Dev Notes

### Task Engine Integration Points

The task engine (`internal/task/engine.go`) needs to be updated to handle failure types from step results.

```go
// TaskEngine orchestrates task execution.
type TaskEngine struct {
    // ... existing fields ...
    ciFailureHandler *CIFailureHandler
}

// NewTaskEngine creates a task engine with dependencies.
func NewTaskEngine(opts ...TaskEngineOption) *TaskEngine {
    // ... existing code ...
}

// WithCIFailureHandler sets the CI failure handler.
func WithCIFailureHandler(handler *CIFailureHandler) TaskEngineOption {
    return func(e *TaskEngine) {
        e.ciFailureHandler = handler
    }
}
```

### Step Result Processing

```go
func (e *TaskEngine) processStepResult(ctx context.Context, task *domain.Task, result *domain.StepResult) error {
    // Check for failure types that need special handling
    if result.Status == domain.StepStatusFailed {
        failureType, _ := result.Metadata["failure_type"].(string)

        switch failureType {
        case "ci_failed":
            return e.handleCIFailure(ctx, task, result)
        case "ci_timeout":
            return e.handleCITimeout(ctx, task, result)
        case "gh_failed":
            return e.handleGHFailure(ctx, task, result)
        default:
            // Standard failure handling
            return e.handleStandardFailure(ctx, task, result)
        }
    }

    // Check for awaiting_approval that needs action
    if result.Status == domain.StepStatusAwaitingApproval {
        actionRequired, _ := result.Metadata["action_required"].(string)

        switch actionRequired {
        case "ci_failure_handling":
            return e.presentCIFailureOptions(ctx, task, result)
        case "ci_timeout_handling":
            return e.presentCITimeoutOptions(ctx, task, result)
        case "garbage_handling":
            return e.presentGarbageOptions(ctx, task, result)
        default:
            // Standard approval handling (human review)
            return e.handleAwaitingApproval(ctx, task, result)
        }
    }

    // ... rest of existing processing ...
}
```

### CI Failure Handling

```go
func (e *TaskEngine) handleCIFailure(ctx context.Context, task *domain.Task, result *domain.StepResult) error {
    e.logger.Info().
        Str("task_id", task.ID).
        Str("workspace", task.WorkspaceName).
        Msg("handling CI failure")

    // Update task state
    if err := e.transitionState(ctx, task, domain.TaskStatusCIFailed); err != nil {
        return fmt.Errorf("failed to transition to ci_failed: %w", err)
    }

    // Store failure context for action processing
    task.Metadata["ci_failure_result"] = result.Metadata["ci_result"]

    // Save task state
    if err := e.taskStore.Update(ctx, task); err != nil {
        return fmt.Errorf("failed to save task state: %w", err)
    }

    // For Epic 6, return awaiting approval to trigger menu display
    // Epic 8 will add interactive TUI menu
    return nil
}

func (e *TaskEngine) presentCIFailureOptions(ctx context.Context, task *domain.Task, result *domain.StepResult) error {
    // Extract CI result from metadata
    ciResult, ok := result.Metadata["ci_result"].(*git.CIWatchResult)
    if !ok {
        e.logger.Warn().Msg("CI result not found in metadata")
    }

    // Format options for user (text-based for Epic 6, TUI in Epic 8)
    options := formatCIFailureOptions(task, ciResult)

    // Store options in task for action processing
    task.Metadata["pending_action"] = "ci_failure"
    task.Metadata["action_options"] = options

    // Save and wait for user input
    return e.taskStore.Update(ctx, task)
}
```

### Action Processing

```go
// ProcessCIFailureAction processes user's CI failure action choice.
func (e *TaskEngine) ProcessCIFailureAction(ctx context.Context, task *domain.Task, action task.CIFailureAction) error {
    if e.ciFailureHandler == nil {
        return fmt.Errorf("CI failure handler not configured: %w", atlaserrors.ErrInvalidConfig)
    }

    // Get stored CI result
    ciResult, _ := task.Metadata["ci_failure_result"].(*git.CIWatchResult)

    opts := task.CIFailureOptions{
        Action:        action,
        PRNumber:      task.PRNumber,
        CIResult:      ciResult,
        WorktreePath:  task.WorktreePath,
        WorkspaceName: task.WorkspaceName,
        ArtifactDir:   task.ArtifactDir,
    }

    result, err := e.ciFailureHandler.HandleCIFailure(ctx, opts)
    if err != nil {
        return fmt.Errorf("failed to handle CI failure action: %w", err)
    }

    // Process handler result
    switch action {
    case task.CIFailureViewLogs:
        // Browser opened, return to options
        return nil

    case task.CIFailureRetryImplement:
        // Transition back to running and restart from implement
        task.CurrentStep = "implement"
        task.Metadata["retry_context"] = result.ErrorContext
        return e.transitionState(ctx, task, domain.TaskStatusRunning)

    case task.CIFailureFixManually:
        // Update status with instructions
        task.Metadata["manual_fix_instructions"] = result.Message
        return e.taskStore.Update(ctx, task)

    case task.CIFailureAbandon:
        // Transition to abandoned
        return e.transitionState(ctx, task, domain.TaskStatusAbandoned)
    }

    return nil
}
```

### Resume Support

```go
func (e *TaskEngine) Resume(ctx context.Context, workspaceName string) error {
    task, err := e.taskStore.GetByWorkspace(ctx, workspaceName)
    if err != nil {
        return fmt.Errorf("failed to get task: %w", err)
    }

    // Check if resuming from CI failure states
    switch task.Status {
    case domain.TaskStatusCIFailed, domain.TaskStatusCITimeout:
        e.logger.Info().
            Str("task_id", task.ID).
            Str("status", string(task.Status)).
            Msg("resuming from CI failure state")

        // User fixed the issue, restart CI monitoring
        task.CurrentStep = "ci_wait"
        if err := e.transitionState(ctx, task, domain.TaskStatusRunning); err != nil {
            return err
        }

    case domain.TaskStatusGHFailed:
        e.logger.Info().
            Str("task_id", task.ID).
            Msg("resuming from GitHub failure state")

        // Retry the failed GitHub operation
        // Current step should still be the failed step (push or pr)
        if err := e.transitionState(ctx, task, domain.TaskStatusRunning); err != nil {
            return err
        }
    }

    // Continue normal execution
    return e.runSteps(ctx, task)
}
```

### State Machine Updates

Verify these transitions exist in `internal/task/state.go`:

```go
var validTransitions = map[TaskStatus][]TaskStatus{
    // ... existing transitions ...
    TaskStatusRunning: {
        TaskStatusCompleted,
        TaskStatusValidating,
        TaskStatusAwaitingApproval,
        TaskStatusValidationFailed,
        TaskStatusCIFailed,      // On CI check failure
        TaskStatusCITimeout,     // On CI monitoring timeout
        TaskStatusGHFailed,      // On GitHub operation failure
        TaskStatusAbandoned,
    },
    TaskStatusCIFailed: {
        TaskStatusRunning,       // Retry from implement or resume after fix
        TaskStatusAbandoned,     // User abandons
    },
    TaskStatusCITimeout: {
        TaskStatusRunning,       // Continue waiting or retry
        TaskStatusAbandoned,     // User abandons
    },
    TaskStatusGHFailed: {
        TaskStatusRunning,       // Retry operation
        TaskStatusAbandoned,     // User abandons
    },
    // ... rest of transitions ...
}
```

### Validation Commands Required

**Before marking story complete, run ALL FOUR:**
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

### References

- [Source: epic-6-traceability-matrix.md - GAP 3]
- [Source: internal/task/engine.go - Task engine to modify]
- [Source: internal/task/ci_failure.go - CIFailureHandler to integrate]
- [Source: internal/task/state.go - State machine transitions]
- [Source: 6-7-ci-failure-handling.md - CIFailureHandler design]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (Step 11 - CI failure handling)
- Scenario 3: PR Creation with Rate Limit (GitHub failure handling)
- Scenario 5: Feature Workflow with Speckit SDD (Step 19 - CI failure handling)

Specific validation checkpoints:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| CI failure detection | Handler invoked | AC1 |
| View logs | Opens browser | AC2 |
| Retry from implement | Returns to implement step | AC3 |
| Fix manually | Shows instructions | AC4 |
| Abandon | PR to draft, task abandoned | AC5 |
| GH failure | Similar options presented | AC6 |
| CI timeout | Includes "Continue waiting" | AC7 |
| Resume after fix | Continues from ci_wait | AC8 |
