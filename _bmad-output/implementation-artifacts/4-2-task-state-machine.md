# Story 4.2: Task State Machine

Status: done

## Story

As a **developer**,
I want **an explicit state machine for task lifecycle**,
So that **state transitions are validated and auditable**.

## Acceptance Criteria

1. **Given** the task store exists **When** I implement `internal/task/state.go` **Then** the state machine enforces valid transitions:
```
Pending → Running
Running → Validating, GHFailed, CIFailed, CITimeout
Validating → AwaitingApproval, ValidationFailed
ValidationFailed → Running, Abandoned
AwaitingApproval → Completed, Running, Rejected
GHFailed → Running, Abandoned
CIFailed → Running, Abandoned
CITimeout → Running, Abandoned
```

2. **Given** invalid transition attempted **When** Transition is called **Then** invalid transitions return clear errors

3. **Given** transition occurs **When** state changes **Then** each transition is logged with timestamp

4. **Given** task state changes **When** Transition function called **Then** `Transition(ctx, task, newStatus) error` validates and applies transition

5. **Given** transitions occur **When** state changes **Then** transition history is stored in task.json

6. **Given** state machine implemented **When** running tests **Then** tests verify all valid and invalid transition combinations

## Tasks / Subtasks

- [x] Task 1: Define state machine types and transitions map (AC: #1)
  - [x] 1.1: Create `internal/task/state.go` file
  - [x] 1.2: Define `validTransitions` map with all valid state transitions
  - [x] 1.3: Export map as `ValidTransitions` for testing visibility
  - [x] 1.4: Add helper function `IsValidTransition(from, to TaskStatus) bool`

- [x] Task 2: Add sentinel error for invalid transitions (AC: #2)
  - [x] 2.1: Add `ErrInvalidTransition` to `internal/errors/errors.go`
  - [x] 2.2: Error message should include from/to status for debugging

- [x] Task 3: Implement Transition function (AC: #3, #4, #5)
  - [x] 3.1: Define `Transition(ctx context.Context, task *domain.Task, to constants.TaskStatus, reason string) error`
  - [x] 3.2: Check context cancellation at function entry
  - [x] 3.3: Validate transition is allowed using `IsValidTransition`
  - [x] 3.4: Return wrapped `ErrInvalidTransition` with from/to details if invalid
  - [x] 3.5: Create `domain.Transition` record with timestamp
  - [x] 3.6: Append transition to `task.Transitions` slice
  - [x] 3.7: Update `task.Status` to new status
  - [x] 3.8: Update `task.UpdatedAt` timestamp
  - [x] 3.9: If transitioning to terminal state (Completed, Rejected, Abandoned), set `task.CompletedAt`

- [x] Task 4: Add helper functions for state queries (AC: #1)
  - [x] 4.1: Implement `IsTerminalStatus(status TaskStatus) bool` - returns true for Completed, Rejected, Abandoned
  - [x] 4.2: Implement `IsErrorStatus(status TaskStatus) bool` - returns true for ValidationFailed, GHFailed, CIFailed, CITimeout
  - [x] 4.3: Implement `CanRetry(status TaskStatus) bool` - returns true for error states that can transition to Running
  - [x] 4.4: Implement `CanAbandon(status TaskStatus) bool` - returns true for states that can transition to Abandoned
  - [x] 4.5: Implement `GetValidTargetStatuses(from TaskStatus) []TaskStatus` - returns all valid target statuses

- [x] Task 5: Write comprehensive tests (AC: #6)
  - [x] 5.1: Create `internal/task/state_test.go`
  - [x] 5.2: Test all valid transitions (positive tests for each row in transitions table)
  - [x] 5.3: Test all invalid transitions (negative tests for disallowed transitions)
  - [x] 5.4: Test `IsValidTransition` function
  - [x] 5.5: Test `IsTerminalStatus` function
  - [x] 5.6: Test `IsErrorStatus` function
  - [x] 5.7: Test `CanRetry` function
  - [x] 5.8: Test `CanAbandon` function
  - [x] 5.9: Test `GetValidTargetStatuses` function
  - [x] 5.10: Test transition history is correctly recorded
  - [x] 5.11: Test `CompletedAt` is set on terminal transitions
  - [x] 5.12: Test context cancellation is respected
  - [x] 5.13: Run `magex format:fix && magex lint && magex test:unit` - ALL PASS

## Dev Notes

### Critical Warnings (READ FIRST)

1. **TaskStatus type is in constants package**: Use `constants.TaskStatus` NOT `domain.TaskStatus`. The status type is already defined in `internal/constants/status.go`.

2. **Transition type is in domain package**: Use `domain.Transition` for transition records. Already defined in `internal/domain/task.go`.

3. **Task already has Transitions field**: The `domain.Task` struct already has `Transitions []Transition` field. No struct modifications needed.

4. **Use existing errors pattern**: Add new error to `internal/errors/errors.go`, not locally.

5. **Context as first parameter**: Always check `ctx.Done()` at function entry.

6. **No database or persistence in this story**: This story only defines the state machine logic. Saving the task with updated transitions is done by the caller using `store.Update()`.

7. **This is a pure validation/logic layer**: The Transition function modifies the task in-memory. The caller is responsible for persisting.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/task/state.go` | NEW - State machine implementation |
| `internal/task/state_test.go` | NEW - Comprehensive state machine tests |
| `internal/errors/errors.go` | MODIFY - Add ErrInvalidTransition |
| `internal/constants/status.go` | REFERENCE - TaskStatus type and constants |
| `internal/domain/task.go` | REFERENCE - Task and Transition types |

### Import Rules (CRITICAL)

**`internal/task/state.go` MAY import:**
- `internal/constants` - for TaskStatus type and constants
- `internal/domain` - for Task and Transition types
- `internal/errors` - for ErrInvalidTransition
- `context`, `fmt`, `time`

**MUST NOT import:**
- `internal/workspace` - avoid circular dependencies
- `internal/ai` - not implemented yet
- `internal/cli` - domain packages don't import CLI

### State Machine Transitions Table

This is the authoritative transitions table from the Architecture document:

```go
// validTransitions defines all allowed state transitions.
// Format: from_status -> []to_statuses
var validTransitions = map[constants.TaskStatus][]constants.TaskStatus{
    constants.TaskStatusPending:          {constants.TaskStatusRunning},
    constants.TaskStatusRunning:          {constants.TaskStatusValidating, constants.TaskStatusGHFailed, constants.TaskStatusCIFailed, constants.TaskStatusCITimeout},
    constants.TaskStatusValidating:       {constants.TaskStatusAwaitingApproval, constants.TaskStatusValidationFailed},
    constants.TaskStatusValidationFailed: {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusAwaitingApproval: {constants.TaskStatusCompleted, constants.TaskStatusRunning, constants.TaskStatusRejected},
    constants.TaskStatusGHFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusCIFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusCITimeout:        {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
}
```

### Terminal vs Error States

**Terminal States** (no further transitions allowed):
- `Completed` - Task successfully finished
- `Rejected` - User rejected during approval
- `Abandoned` - User chose to abandon

**Error States** (can retry or abandon):
- `ValidationFailed` - Lint/test/format failed
- `GHFailed` - GitHub operation failed
- `CIFailed` - CI workflow failed
- `CITimeout` - CI polling timed out

### Implementation Pattern

```go
// internal/task/state.go

package task

import (
    "context"
    "fmt"
    "time"

    "github.com/mrz1836/atlas/internal/constants"
    "github.com/mrz1836/atlas/internal/domain"
    atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ValidTransitions defines all allowed state transitions.
var ValidTransitions = map[constants.TaskStatus][]constants.TaskStatus{
    constants.TaskStatusPending:          {constants.TaskStatusRunning},
    constants.TaskStatusRunning:          {constants.TaskStatusValidating, constants.TaskStatusGHFailed, constants.TaskStatusCIFailed, constants.TaskStatusCITimeout},
    constants.TaskStatusValidating:       {constants.TaskStatusAwaitingApproval, constants.TaskStatusValidationFailed},
    constants.TaskStatusValidationFailed: {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusAwaitingApproval: {constants.TaskStatusCompleted, constants.TaskStatusRunning, constants.TaskStatusRejected},
    constants.TaskStatusGHFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusCIFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
    constants.TaskStatusCITimeout:        {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
}

// IsValidTransition checks if a transition from one status to another is allowed.
func IsValidTransition(from, to constants.TaskStatus) bool {
    validTargets, exists := ValidTransitions[from]
    if !exists {
        return false // Terminal state or unknown state
    }
    for _, target := range validTargets {
        if target == to {
            return true
        }
    }
    return false
}

// Transition validates and applies a state transition to the task.
// It records the transition in the task's history and updates timestamps.
// The caller is responsible for persisting the updated task.
func Transition(ctx context.Context, task *domain.Task, to constants.TaskStatus, reason string) error {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    from := task.Status

    // Validate transition
    if !IsValidTransition(from, to) {
        return fmt.Errorf("%w: cannot transition from %s to %s",
            atlaserrors.ErrInvalidTransition, from, to)
    }

    now := time.Now().UTC()

    // Record transition in history
    transition := domain.Transition{
        FromStatus: from,
        ToStatus:   to,
        Timestamp:  now,
        Reason:     reason,
    }
    task.Transitions = append(task.Transitions, transition)

    // Update task status
    task.Status = to
    task.UpdatedAt = now

    // Set CompletedAt for terminal states
    if IsTerminalStatus(to) {
        task.CompletedAt = &now
    }

    return nil
}

// Terminal status helpers - implement these
func IsTerminalStatus(status constants.TaskStatus) bool { ... }
func IsErrorStatus(status constants.TaskStatus) bool { ... }
func CanRetry(status constants.TaskStatus) bool { ... }
func CanAbandon(status constants.TaskStatus) bool { ... }
func GetValidTargetStatuses(from constants.TaskStatus) []constants.TaskStatus { ... }
```

### Error Message Pattern

```go
// In internal/errors/errors.go, add:

// ErrInvalidTransition indicates an attempt to make an invalid state transition.
ErrInvalidTransition = errors.New("invalid state transition")
```

When wrapping the error in Transition():
```go
return fmt.Errorf("%w: cannot transition from %s to %s",
    atlaserrors.ErrInvalidTransition, from, to)
```

This allows callers to check with `errors.Is(err, atlaserrors.ErrInvalidTransition)` while also getting the specific from/to details.

### Testing Pattern

```go
func TestTransition_ValidTransitions(t *testing.T) {
    tests := []struct {
        name   string
        from   constants.TaskStatus
        to     constants.TaskStatus
        reason string
    }{
        {"pending to running", constants.TaskStatusPending, constants.TaskStatusRunning, "task started"},
        {"running to validating", constants.TaskStatusRunning, constants.TaskStatusValidating, "AI completed"},
        // ... all valid transitions
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            task := &domain.Task{
                ID:     "task-20251228-100000",
                Status: tt.from,
            }

            err := Transition(context.Background(), task, tt.to, tt.reason)
            require.NoError(t, err)
            assert.Equal(t, tt.to, task.Status)
            assert.Len(t, task.Transitions, 1)
            assert.Equal(t, tt.from, task.Transitions[0].FromStatus)
            assert.Equal(t, tt.to, task.Transitions[0].ToStatus)
        })
    }
}

func TestTransition_InvalidTransitions(t *testing.T) {
    tests := []struct {
        name string
        from constants.TaskStatus
        to   constants.TaskStatus
    }{
        {"pending to completed", constants.TaskStatusPending, constants.TaskStatusCompleted},
        {"completed to running", constants.TaskStatusCompleted, constants.TaskStatusRunning},
        // ... all invalid transitions
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            task := &domain.Task{
                ID:     "task-20251228-100000",
                Status: tt.from,
            }

            err := Transition(context.Background(), task, tt.to, "test")
            require.Error(t, err)
            assert.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
            // Status should be unchanged
            assert.Equal(t, tt.from, task.Status)
            // No transition recorded
            assert.Empty(t, task.Transitions)
        })
    }
}

func TestTransition_SetsCompletedAt(t *testing.T) {
    terminalStates := []constants.TaskStatus{
        constants.TaskStatusCompleted,
        constants.TaskStatusRejected,
        constants.TaskStatusAbandoned,
    }

    for _, terminal := range terminalStates {
        t.Run(terminal.String(), func(t *testing.T) {
            // Create task in appropriate pre-terminal state
            var from constants.TaskStatus
            switch terminal {
            case constants.TaskStatusCompleted, constants.TaskStatusRejected:
                from = constants.TaskStatusAwaitingApproval
            case constants.TaskStatusAbandoned:
                from = constants.TaskStatusValidationFailed
            }

            task := &domain.Task{
                ID:     "task-20251228-100000",
                Status: from,
            }

            err := Transition(context.Background(), task, terminal, "test")
            require.NoError(t, err)
            assert.NotNil(t, task.CompletedAt)
        })
    }
}
```

### Previous Story Learnings (from Story 4-1)

From Story 4-1 (Task Data Model and Store):

1. **Use syscall.Flock pattern** - Established in task store, but not needed here (no persistence)
2. **Constants package for status** - TaskStatus is in `internal/constants/status.go`
3. **Transition struct ready** - Already defined in `internal/domain/task.go`
4. **Run `magex test:race`** - Race detection is mandatory for all tests
5. **Action-first error messages** - `"cannot transition from %s to %s"`

### Dependencies Between Stories

This story **depends on:**
- **Story 4-1** (Task Data Model and Store) - uses Task, Transition types ✓ DONE

This story **is required for:**
- **Story 4-6** (Task Engine Orchestrator) - uses Transition() for state management
- **Story 5.3** (Validation Result Handling) - uses state machine for validation_failed
- All subsequent stories that modify task state

### Edge Cases to Handle

1. **Unknown status** - Task has status not in validTransitions map → treat as terminal (no valid targets)
2. **Same status transition** - from == to → should return error (not a valid transition)
3. **Nil task** - Return error early with clear message
4. **Context cancelled** - Return ctx.Err() immediately
5. **Empty reason** - Allow empty reason string (optional field)
6. **Multiple sequential transitions** - Each should append to Transitions slice

### Performance Considerations

1. **Map lookup is O(n) for targets** - Acceptable for small target lists (max 4 targets)
2. **No allocations in hot path** - except for the Transition record itself
3. **Transition history grows** - Long-running tasks may have many transitions; consider limit in future

### Security Considerations

1. **No sensitive data in reasons** - Don't include API keys, tokens, etc. in transition reasons
2. **Audit trail** - Transitions provide audit trail for debugging and compliance

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.2]
- [Source: _bmad-output/planning-artifacts/architecture.md#Task Engine Architecture]
- [Source: _bmad-output/planning-artifacts/architecture.md#State Transitions Table]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/constants/status.go - TaskStatus type and constants]
- [Source: internal/domain/task.go - Task and Transition types]
- [Source: internal/task/store.go - Pattern for context handling]
- [Source: _bmad-output/implementation-artifacts/4-1-task-data-model-and-store.md - Previous story patterns]

### Project Structure Notes

- State machine is a pure logic layer in `internal/task/`
- Follows same patterns as task store for context handling
- Uses constants from `internal/constants/` per architecture
- Error sentinel in `internal/errors/` per project standards

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test (state machine is internal - test via unit tests):
go test -v ./internal/task/... -run TestTransition

# Manual verification:
# Review all transitions in tests match architecture document
# Verify error messages are actionable and include from/to status
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No blocking issues encountered during implementation.

### Completion Notes List

- Implemented complete task state machine in `internal/task/state.go`
- Added `ErrInvalidTransition` sentinel error to `internal/errors/errors.go`
- Created `ValidTransitions` map with all allowed state transitions per architecture spec
- Implemented `Transition()` function with context cancellation, validation, and history recording
- Implemented helper functions: `IsValidTransition`, `IsTerminalStatus`, `IsErrorStatus`, `CanRetry`, `CanAbandon`, `GetValidTargetStatuses`
- Created comprehensive test suite with 20+ test functions covering all valid/invalid transitions, edge cases, context handling
- All tests pass with race detection (`magex test:race`)
- All linting passes (`magex lint`)
- Code formatted (`magex format:fix`)

### Implementation Notes

- Used `//nolint:gochecknoglobals` for lookup table maps per project patterns
- `GetValidTargetStatuses` returns a copy of the slice to prevent modification of the original
- Error messages follow action-first format: "cannot transition from X to Y"
- Same-status transitions (e.g., pending -> pending) are explicitly rejected

### File List

- `internal/task/state.go` (NEW) - State machine implementation
- `internal/task/state_test.go` (NEW) - Comprehensive state machine tests
- `internal/errors/errors.go` (MODIFIED) - Added ErrInvalidTransition

### Change Log

- 2025-12-28: Implemented Story 4.2 Task State Machine - all tasks and subtasks completed
- 2025-12-28: Code review completed - 4 medium issues fixed, 3 low issues addressed

## Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5 (Adversarial Code Review)
**Date:** 2025-12-28
**Outcome:** APPROVED (after fixes)

### Review Summary

All Acceptance Criteria verified as implemented. All tasks marked `[x]` confirmed complete.
Git File List matches story claims exactly. 100% test coverage on state.go.

### Issues Found and Fixed

| Severity | Issue | Resolution |
|----------|-------|------------|
| MEDIUM | Redundant lookup tables lacked maintenance docs | Added MAINTENANCE comments explaining intentional duplication for O(1) performance |
| MEDIUM | Missing test for AwaitingApproval → Abandoned | Added explicit test case to `TestIsValidTransition_InvalidTransitions` |
| MEDIUM | CanRetry comment misleading about AwaitingApproval | Clarified that AwaitingApproval → Running is workflow choice, not retry |
| MEDIUM | Missing error state tests in GetValidTargetStatuses | Added tests for GHFailed, CIFailed, CITimeout target statuses |
| LOW | Line 38 exceeded 131 characters | Reformatted ValidTransitions map to multi-line for TaskStatusRunning targets |
| LOW | godox lint error on "Note:" | Removed "Note:" prefix from comment |

### Verification

```bash
magex format:fix   # PASS
magex lint         # PASS (0 issues)
go test -race      # PASS (1.956s)
go test -cover     # state.go: 100% coverage
```

### Files Modified in Review

- `internal/task/state.go` - Documentation improvements, line length fix
- `internal/task/state_test.go` - Added 4 new test cases

