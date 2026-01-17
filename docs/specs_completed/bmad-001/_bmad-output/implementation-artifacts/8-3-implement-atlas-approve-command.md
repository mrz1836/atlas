# Story 8.3: Implement `atlas approve` Command

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **to run `atlas approve` to approve completed work**,
So that **I can confirm the task is complete and ready to merge**.

## Acceptance Criteria

1. **Given** tasks are awaiting approval
   **When** I run `atlas approve`
   **Then** if multiple tasks pending, shows selection menu

2. **Given** user selects a task (or only one task exists)
   **When** displaying the approval screen
   **Then** shows approval summary (using ApprovalSummary component from Story 8.2)

3. **Given** approval summary is displayed
   **When** presenting action menu
   **Then** presents action menu with options: Approve, View diff, View logs, Open PR, Reject, Cancel

4. **Given** user selects "Approve and continue"
   **When** processing approval
   **Then** transitions task to `completed` state

5. **Given** user selects "View diff"
   **When** processing action
   **Then** shows full git diff in pager (using `less` or similar)

6. **Given** user selects "View logs"
   **When** processing action
   **Then** shows task execution log

7. **Given** user selects "Open PR in browser"
   **When** processing action
   **Then** opens PR URL via `open` command (macOS)

8. **Given** workspace name is provided directly
   **When** running `atlas approve <workspace>`
   **Then** skips selection menu and goes directly to approval flow

9. **Given** approval is successful
   **When** displaying result
   **Then** displays success message: "✓ Task approved. PR ready for merge."

10. **Given** `--output json` flag is used
    **When** running approve command
    **Then** outputs structured JSON instead of interactive UI

## Tasks / Subtasks

- [x] Task 1: Create Approve Command Structure (AC: #1, #8, #10)
  - [x] 1.1: Create `internal/cli/approve.go` with `newApproveCmd()` function
  - [x] 1.2: Create `approveOptions` struct (workspace name optional arg)
  - [x] 1.3: Add `AddApproveCommand(root *cobra.Command)` function
  - [x] 1.4: Register command in `internal/cli/root.go`
  - [x] 1.5: Support `--output json` flag through shared flags system

- [x] Task 2: Implement Task Selection Menu (AC: #1, #8)
  - [x] 2.1: Create `findAwaitingApprovalTasks()` function to list all tasks with `awaiting_approval` status
  - [x] 2.2: If workspace arg provided, skip selection and load that workspace's task
  - [x] 2.3: If no tasks awaiting approval, display helpful message and exit
  - [x] 2.4: If only one task, auto-select it (no menu needed)
  - [x] 2.5: If multiple tasks, use `tui.Select()` from menus.go to present workspace selection
  - [x] 2.6: Create `Option` structs with workspace name as label, status details as description

- [x] Task 3: Implement Approval Summary Display (AC: #2)
  - [x] 3.1: Load task and workspace for selected workspace
  - [x] 3.2: Create `ApprovalSummary` using `tui.NewApprovalSummary(task, workspace)`
  - [x] 3.3: Optionally call `SetFileStats()` with git diff stats if available
  - [x] 3.4: Render summary using `tui.RenderApprovalSummary(summary)`
  - [x] 3.5: Display summary to user before action menu

- [x] Task 4: Implement Action Menu (AC: #3)
  - [x] 4.1: Create action menu options: Approve, View diff, View logs, Open PR, Reject, Cancel
  - [x] 4.2: Use `tui.Select()` for action menu with appropriate styling
  - [x] 4.3: Map menu selections to action handlers
  - [x] 4.4: Implement menu loop (return to menu after View actions)
  - [x] 4.5: Handle `tui.ErrMenuCanceled` gracefully

- [x] Task 5: Implement "Approve" Action (AC: #4, #9)
  - [x] 5.1: Transition task status to `completed` using state machine
  - [x] 5.2: Update workspace status if needed
  - [x] 5.3: Save task state to file store
  - [x] 5.4: Display success message: "✓ Task approved. PR ready for merge."
  - [x] 5.5: Emit terminal bell notification for completion

- [x] Task 6: Implement "View diff" Action (AC: #5)
  - [x] 6.1: Get worktree path from workspace
  - [x] 6.2: Run `git diff HEAD~1` in worktree to get changes
  - [x] 6.3: Pipe output to `less -R` (or fallback to direct output if less unavailable)
  - [x] 6.4: Return to action menu after viewing
  - [x] 6.5: Handle case where diff is empty or command fails

- [x] Task 7: Implement "View logs" Action (AC: #6)
  - [x] 7.1: Load task log file from task store (`task.log` in task directory)
  - [x] 7.2: Display log content using pager
  - [x] 7.3: Return to action menu after viewing
  - [x] 7.4: Handle case where log file doesn't exist

- [x] Task 8: Implement "Open PR in browser" Action (AC: #7)
  - [x] 8.1: Extract PR URL from task metadata (`task.Metadata["pr_url"]`)
  - [x] 8.2: If no PR URL, display message "No PR URL available"
  - [x] 8.3: Use `exec.Command("open", prURL)` for macOS
  - [x] 8.4: Return to action menu after opening
  - [x] 8.5: Handle `open` command failure gracefully

- [x] Task 9: Implement "Reject" Action (AC: #3)
  - [x] 9.1: Display note that reject command is separate: "Run `atlas reject <workspace>` to reject with feedback"
  - [x] 9.2: Alternative: Transition to reject flow inline (Story 8.4 dependency)
  - [x] 9.3: For now, just display the redirect message and return to menu

- [x] Task 10: Implement JSON Output Mode (AC: #10)
  - [x] 10.1: Create `approveResponse` struct for JSON output
  - [x] 10.2: Include: success, workspace info, task info, pr_url, error (if any)
  - [x] 10.3: Skip interactive menus in JSON mode
  - [x] 10.4: Require workspace arg when using `--output json` (no interactive selection)
  - [x] 10.5: Return appropriate JSON for success/failure cases

- [x] Task 11: Create Test Suite (AC: #1-#10)
  - [x] 11.1: Test command registration and flag parsing
  - [x] 11.2: Test workspace selection with multiple tasks
  - [x] 11.3: Test direct workspace argument skipping selection
  - [x] 11.4: Test approval flow with task state transition
  - [x] 11.5: Test JSON output mode
  - [x] 11.6: Test error cases (no tasks, workspace not found, invalid status)
  - [x] 11.7: Mock workspace store and task store for unit tests

- [x] Task 12: Validate and Finalize
  - [x] 12.1: Run `magex format:fix` - must pass
  - [x] 12.2: Run `magex lint` - must pass
  - [x] 12.3: Run `magex test:race` - must pass
  - [x] 12.4: Run `go-pre-commit run --all-files` - must pass

## Dev Notes

### Existing CLI Infrastructure

**DO NOT recreate - reuse these patterns from existing CLI commands:**

**From `internal/cli/resume.go` (reference implementation):**
```go
// Command structure pattern
func AddResumeCommand(root *cobra.Command) {
    root.AddCommand(newResumeCmd())
}

type resumeOptions struct {
    aiFix bool
}

func newResumeCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "resume <workspace>",
        Short: "Resume a paused or failed task",
        Args:  cobra.ExactArgs(1), // Or cobra.MaximumNArgs(1) for optional
        RunE: func(cmd *cobra.Command, args []string) error {
            return runResume(cmd.Context(), cmd, os.Stdout, args[0], opts)
        },
    }
    return cmd
}

// Shared infrastructure
tui.CheckNoColor()
out := tui.NewOutput(w, outputFormat)
logger := GetLogger()
```

**From `internal/cli/flags.go`:**
```go
// Output format handling
const (
    OutputJSON = "json"
    OutputText = "text"
)

// Use cmd.Flag("output").Value.String() for format detection
```

### TUI Components (Story 8.1, 8.2)

**From `internal/tui/menus.go`:**
```go
// Selection menu - USE THIS for workspace/action selection
tui.Select(title string, options []tui.Option) (string, error)

type Option struct {
    Label       string
    Description string
    Value       string
}

// Confirmation dialog
tui.Confirm(message string, defaultYes bool) (bool, error)

// Cancel detection
tui.ErrMenuCanceled // Returned when user presses q/Esc
```

**From `internal/tui/approval.go` (Story 8.2):**
```go
// Approval summary - USE THIS for displaying task context
type ApprovalSummary struct {
    TaskID, WorkspaceName, Description string
    Status                              constants.TaskStatus
    CurrentStep, TotalSteps             int
    BranchName, PRURL                   string
    FileChanges                         []FileChange
    TotalInsertions, TotalDeletions     int
    Validation                          *ValidationSummary
}

// Constructor
summary := tui.NewApprovalSummary(task, workspace)

// Optional: Add file stats
summary.SetFileStats(stats map[string]tui.FileChange)

// Render
output := tui.RenderApprovalSummary(summary)
```

**From `internal/tui/output.go`:**
```go
type Output interface {
    Success(msg string)
    Error(err error)
    Warning(msg string)
    Info(msg string)
    JSON(v interface{}) error
}

out := tui.NewOutput(w, format)
```

### Domain Types

**From `internal/domain/task.go`:**
```go
type Task struct {
    ID            string                  `json:"id"`
    WorkspaceID   string                  `json:"workspace_id"`
    Description   string                  `json:"description"`
    Status        constants.TaskStatus    `json:"status"`
    CurrentStep   int                     `json:"current_step"`
    Steps         []Step                  `json:"steps"`
    StepResults   []StepResult            `json:"step_results,omitempty"`
    Metadata      map[string]any          `json:"metadata,omitempty"`
}
```

**Task Status Constants (from `internal/constants/status.go`):**
```go
const (
    TaskStatusAwaitingApproval constants.TaskStatus = "awaiting_approval"
    TaskStatusCompleted        constants.TaskStatus = "completed"
    TaskStatusRejected         constants.TaskStatus = "rejected"
)
```

### State Machine

**From `internal/task/state.go`:**
```go
// Valid transition: awaiting_approval → completed
var validTransitions = map[constants.TaskStatus][]constants.TaskStatus{
    constants.TaskStatusAwaitingApproval: {
        constants.TaskStatusCompleted,
        constants.TaskStatusRunning,    // For reject/retry
        constants.TaskStatusRejected,
    },
}

// Use task engine or direct state machine for transitions
func (sm *StateMachine) Transition(ctx context.Context, task *domain.Task, newStatus constants.TaskStatus) error
```

### Action Menu Options

Based on UX specification (ux-design-specification.md):

```
? What would you like to do?
  ❯ Approve and create PR ready state
    View diff — Show file changes
    View logs — Show task execution log
    Open PR in browser — View pull request
    Reject — Run atlas reject for feedback
    Cancel
```

**Keyboard shortcuts pattern from UX spec:**
```
[a] Approve    [d] Diff    [l] Logs    [p] Open PR    [r] Reject    [q] Cancel
```

### External Commands

**View diff:**
```bash
# Get diff of changes in worktree
git -C <worktree_path> diff HEAD~1 | less -R
```

**View logs:**
```bash
# Task log location
~/.atlas/workspaces/<workspace>/tasks/<task-id>/task.log
```

**Open PR:**
```bash
# macOS only
open <pr_url>
```

### Git Intelligence

Recent commit patterns from this epic:
```
feat(tui): add approval summary component
feat(tui): add interactive menu system using Charm Huh
```

Expected commit format:
```
feat(cli): add atlas approve command
```

### Validation Commands

```bash
# MUST run ALL FOUR before marking complete
magex format:fix
magex lint
magex test:race
go-pre-commit run --all-files
```

### Project Structure Notes

**Files to create:**
- `internal/cli/approve.go` - Main approve command implementation
- `internal/cli/approve_test.go` - Comprehensive test suite

**Files to modify:**
- `internal/cli/root.go` - Add `AddApproveCommand(rootCmd)` registration

**Alignment with project structure (from architecture.md):**
```
internal/
├── cli/
│   ├── approve.go       # ← NEW: atlas approve
│   ├── approve_test.go  # ← NEW: tests
│   ├── resume.go        # Reference pattern
│   └── ...
```

### Architecture Compliance

**From architecture.md - CLI Package:**
- One file per command
- Use Cobra command pattern
- Context-first design (ctx as first parameter)
- Use tui package for output, not direct fmt.Print
- Support --output json flag

**From architecture.md - Import Rules:**
- `internal/cli` → can import task, workspace, tui, config, domain, constants, errors
- Must NOT be imported by other packages

### Previous Story Learnings (Story 8.2)

From Story 8.2 code review:
1. **Use AdaptiveColor directly** - Don't hardcode `.Dark` values
2. **Call CheckNoColor() at function entry** - Essential for NO_COLOR compliance
3. **Use existing style constants** - Import from styles.go
4. **Test width adaptation** - Test at 60, 80, 120 column widths
5. **Handle nil inputs gracefully** - Return empty/error early

From Story 8.1 (menus):
1. **Handle ErrMenuCanceled** - Use `errors.Is(err, tui.ErrMenuCanceled)`
2. **Menu loop pattern** - View actions return to menu, terminal actions exit

### Error Handling Patterns

From project-context.md:
```go
// Action-first format
return fmt.Errorf("failed to load task: %w", err)

// Use sentinel errors from internal/errors
atlaserrors.ErrTaskNotFound
atlaserrors.ErrInvalidTransition
atlaserrors.ErrNoTasksFound
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md#epic-8 - Story 8.3 acceptance criteria]
- [Source: _bmad-output/planning-artifacts/architecture.md - CLI package structure, import rules]
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md - Approval flow, action menu patterns, UX-11/UX-12]
- [Source: _bmad-output/project-context.md - Validation commands, error handling, coding standards]
- [Source: internal/cli/resume.go - CLI command pattern reference]
- [Source: internal/tui/menus.go - Select, Confirm, Option types, ErrMenuCanceled]
- [Source: internal/tui/approval.go - ApprovalSummary, RenderApprovalSummary]
- [Source: internal/tui/output.go - Output interface, NewOutput]
- [Source: internal/task/state.go - State machine, valid transitions]
- [Source: internal/constants/status.go - TaskStatus constants]
- [Source: _bmad-output/implementation-artifacts/8-1-interactive-menu-system.md - Menu system learnings]
- [Source: _bmad-output/implementation-artifacts/8-2-approval-summary-component.md - Approval summary learnings]

## Dev Agent Record

### Agent Model Used

claude-opus-4-5-20251101

### Debug Log References

N/A

### Completion Notes List

1. **Implementation completed** - All 12 tasks and subtasks successfully implemented
2. **All acceptance criteria met** - AC #1-#10 verified through implementation and testing
3. **Validation passes** - All four required validation commands pass:
   - `magex format:fix` - ✅
   - `magex lint` - ✅
   - `magex test:race` - ✅
   - `go-pre-commit run --all-files` - ✅ (6/6 checks passed)
4. **Code follows established patterns** - Uses same patterns as resume.go, status.go
5. **Test coverage** - Comprehensive test suite with 30+ test cases covering:
   - Command registration and flags
   - Task selection and filtering
   - State transitions
   - JSON output mode
   - Error handling
   - Context cancellation

### File List

**Created:**
- `internal/cli/approve.go` - Main approve command implementation (577 lines)
- `internal/cli/approve_test.go` - Comprehensive test suite (879 lines)

**Modified:**
- `internal/cli/root.go` - Added `AddApproveCommand(cmd)` registration
- `internal/errors/errors.go` - Added `ErrInvalidArgument` sentinel error

**Unrelated (bundled in working tree):**
- `docs/external/vision.md` - Documentation updates (ecosystem architecture, mage-x clarifications) - should be committed separately

### Change Log

| Date | Author | Change |
|------|--------|--------|
| 2025-12-31 | Dev Agent | Initial implementation completed |
| 2025-12-31 | Code Review | Adversarial review passed - all ACs verified |

## Senior Developer Review (AI)

**Reviewer:** claude-opus-4-5-20251101
**Date:** 2025-12-31
**Outcome:** ✅ APPROVED

### Review Summary

| Category | Status |
|----------|--------|
| All Tasks [x] Verified | ✅ 12/12 tasks confirmed done |
| All ACs Implemented | ✅ 10/10 acceptance criteria verified |
| Validation Commands | ✅ All 4 pass (format, lint, test:race, pre-commit) |
| Architecture Compliance | ✅ Import rules, context-first, error handling correct |
| Test Coverage | ✅ 30+ test cases in approve_test.go |
| Git vs Story Sync | ⚠️ 1 unrelated file (vision.md) - documented above |

### Issues Found & Resolution

**Medium Issues (2):**
1. `docs/external/vision.md` modified but not story-related → Documented in File List as unrelated
2. `printApprovalSummary` bypasses Output interface for styled output → Acceptable pattern (consistent with existing commands)

**Low Issues (1):**
1. Vision.md changes should be separate commit → Documented for awareness

### Verification Details

- State transitions use `task.Transition()` correctly (approve.go:412)
- ErrMenuCanceled handled with `errors.Is()` (approve.go:207, 311)
- JSON output uses proper struct with snake_case fields (approve.go:69-76)
- Context cancellation checked at entry (approve.go:93-97)
- NO_COLOR respected via `tui.CheckNoColor()` (approve.go:103)

### Recommendation

Story is ready for merge. The unrelated vision.md changes should ideally be committed separately to maintain clean git history.
