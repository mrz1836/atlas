# Story 5.6: Task Abandonment Flow

Status: done

## Story

As a **user**,
I want **to abandon a task while preserving the branch and worktree**,
So that **I can take over manually or revisit later**.

## Acceptance Criteria

1. **Given** a task is in `validation_failed` state (or any error state)
   **When** I select "Abandon task" option
   **Then** the system transitions task to `abandoned` state

2. **Given** a task is being abandoned
   **When** the abandonment completes
   **Then** the git branch with all commits is preserved

3. **Given** a task is being abandoned
   **When** the abandonment completes
   **Then** the worktree directory is preserved

4. **Given** a task is being abandoned
   **When** the abandonment completes
   **Then** all task artifacts and logs are preserved

5. **Given** a task is abandoned
   **When** the process completes
   **Then** the system displays: "Task abandoned. Branch '<branch>' preserved at '<worktree-path>'"

6. **Given** a task is abandoned
   **When** the workspace status is checked
   **Then** the workspace remains in `paused` state (not retired)

7. **Given** a task has been abandoned
   **When** I want to continue working
   **Then** I can start a new task in the same workspace if desired

8. **Given** a task has been abandoned
   **When** checking task history
   **Then** abandoned tasks appear in task history

9. **Given** a task has been abandoned
   **When** I want to clean up later
   **Then** `atlas workspace destroy` can still clean up the workspace

## Tasks / Subtasks

- [x] Task 1: Implement `atlas abandon` CLI command (AC: #1, #5, #6)
  - [x] 1.1: Create `internal/cli/abandon.go` with `abandonCmd` Cobra command
  - [x] 1.2: Add positional arg for workspace name
  - [x] 1.3: Add `--force` flag to skip confirmation prompt
  - [x] 1.4: Load workspace and current task using workspace manager
  - [x] 1.5: Validate task is in abandonable state using `task.CanAbandon()`
  - [x] 1.6: Prompt for confirmation unless `--force` is used
  - [x] 1.7: Call task engine's `Abandon()` method
  - [x] 1.8: Update workspace status to `paused`
  - [x] 1.9: Display success message with branch and worktree path
  - [x] 1.10: Register command in root.go

- [x] Task 2: Implement `Engine.Abandon()` method (AC: #1, #2, #3, #4)
  - [x] 2.1: Add `Abandon(ctx context.Context, task *domain.Task, reason string) error` to `internal/task/engine.go`
  - [x] 2.2: Validate task is in abandonable state using `task.CanAbandon()`
  - [x] 2.3: Call `task.Transition()` to move task to `abandoned` status with reason
  - [x] 2.4: Preserve task artifacts (do NOT delete artifacts directory)
  - [x] 2.5: Preserve task log file (do NOT delete task.log)
  - [x] 2.6: Save final task state to task.json
  - [x] 2.7: Log abandonment with task_id, workspace_name, and reason

- [x] Task 3: Create abandonment info display (AC: #5)
  - [x] 3.1: Create `internal/tui/abandon.go` with `AbandonInfo` struct
  - [x] 3.2: Implement `DisplayAbandonmentSuccess(task, workspace)` function
  - [x] 3.3: Display preserved branch name
  - [x] 3.4: Display preserved worktree path
  - [x] 3.5: Display suggestion for next steps (manual work or workspace destroy)
  - [x] 3.6: Use established TUI styling (styles.go patterns)

- [x] Task 4: Update workspace manager for abandon flow (AC: #6, #7)
  - [x] 4.1: Use existing `UpdateStatus()` method to set workspace status to `paused`
  - [x] 4.2: Verified workspace `paused` status preserves worktree and branch
  - [x] 4.3: Verified paused workspace can accept new tasks
  - [x] 4.4: Verified `atlas workspace destroy` works on paused workspace

- [x] Task 5: Integrate abandon option into error recovery menus (AC: #1)
  - [x] 5.1: Update `internal/tui/manual_fix.go` to suggest abandon option on validation failure
  - [x] 5.2: Added abandon suggestion to manual fix instructions display
  - [x] 5.3: Abandon command available from CLI at any time during error state

- [x] Task 6: Write comprehensive tests (AC: all)
  - [x] 6.1: Created `internal/tui/abandon_test.go` with ExtractAbandonInfo and DisplayAbandonmentSuccess tests
  - [x] 6.2: Test abandon from validation_failed state
  - [x] 6.3: Test abandon from gh_failed state
  - [x] 6.4: Test abandon from ci_failed state
  - [x] 6.5: Test abandon from ci_timeout state
  - [x] 6.6: Test rejection of non-abandonable states (running, pending, validating, etc.)
  - [x] 6.7: Test `Engine.Abandon()` transitions task to abandoned
  - [x] 6.8: Test `Engine.Abandon()` preserves artifacts and logs (metadata preserved)
  - [x] 6.9: Test nil task handling
  - [x] 6.10: Test context cancellation handling
  - [x] 6.11: Test store failure handling
  - [x] 6.12: Run tests with `-race` flag - all tests pass

## Dev Agent Record

### Agent Model Used

claude-opus-4-5-20251101

### Debug Log References

N/A

### Completion Notes List

1. **Implementation Complete**: All 6 tasks implemented successfully
2. **Files Created**:
   - `internal/cli/abandon.go` - Abandon CLI command
   - `internal/tui/abandon.go` - AbandonInfo struct and DisplayAbandonmentSuccess function
   - `internal/tui/abandon_test.go` - Tests for abandon TUI components
3. **Files Modified**:
   - `internal/cli/root.go` - Registered abandon command
   - `internal/task/engine.go` - Added Engine.Abandon() method
   - `internal/task/engine_test.go` - Added comprehensive tests for Abandon()
   - `internal/tui/manual_fix.go` - Added abandon suggestion to instructions
4. **Validation**: All linting passes, all tests pass (with race detection)
5. **Notes**:
   - Abandon command follows exact patterns from resume.go and workspace_destroy.go
   - Engine.Abandon() is placed correctly in engine.go (after HandleStepResult, before private methods)
   - Manual fix instructions now include abandon suggestion for better discoverability

### Code Review Notes

**Reviewed by:** claude-opus-4-5-20251101
**Date:** 2025-12-29

**Issues Found & Fixed:**
1. **HIGH - Missing CLI Tests**: Created `internal/cli/abandon_test.go` with 18 comprehensive tests covering:
   - Command structure and flags
   - Workspace/task not found errors
   - Task not in abandonable state
   - Success scenarios (text and JSON output)
   - Context cancellation
   - Non-interactive mode handling
   - Workspace status updates
   - Task artifact preservation

2. **MEDIUM - Error Message Duplication**: Fixed redundant error message in non-interactive mode check (was including "use --force" twice)

**All tests pass, lint clean.**

### File List

Created:
- `internal/cli/abandon.go`
- `internal/cli/abandon_test.go` (added during code review)
- `internal/tui/abandon.go`
- `internal/tui/abandon_test.go`

Modified:
- `internal/cli/root.go`
- `internal/task/engine.go`
- `internal/task/engine_test.go`
- `internal/tui/manual_fix.go`
