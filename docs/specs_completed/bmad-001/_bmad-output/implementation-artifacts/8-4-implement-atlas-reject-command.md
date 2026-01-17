# Story 8.4: Implement `atlas reject` Command

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **to run `atlas reject` to reject work with feedback**,
So that **ATLAS can retry with my guidance**.

## Acceptance Criteria

1. **Given** a task is awaiting approval
   **When** I run `atlas reject payment`
   **Then** the system presents a decision flow asking how to proceed

2. **Given** the decision flow is displayed
   **When** presenting options
   **Then** shows choices: "Reject and retry" or "Reject (done)"

3. **Given** user selects "Reject and retry"
   **When** processing the selection
   **Then** shows feedback form for rejection reason

4. **Given** user provides feedback
   **When** processing rejection with retry
   **Then** presents step selection menu to choose where to resume

5. **Given** step selection is complete
   **When** finalizing rejection
   **Then** feedback is saved to task artifacts

6. **Given** rejection with retry is complete
   **When** task state changes
   **Then** task returns to `running` state at the specified step

7. **Given** task is in running state after rejection
   **When** AI resumes work
   **Then** AI receives the feedback as context for retry

8. **Given** user selects "Reject (done)"
   **When** processing final rejection
   **Then** task transitions to `rejected` state

9. **Given** task is rejected (done)
   **When** checking workspace state
   **Then** rejected tasks preserve branch and worktree for manual work

10. **Given** `--output json` flag is used
    **When** running reject command
    **Then** outputs structured JSON instead of interactive UI

## Tasks / Subtasks

- [x] Task 1: Create Reject Command Structure (AC: #1, #10)
  - [x] 1.1: Create `internal/cli/reject.go` with `newRejectCmd()` function
  - [x] 1.2: Create `rejectOptions` struct with workspace name required arg
  - [x] 1.3: Add `AddRejectCommand(root *cobra.Command)` function
  - [x] 1.4: Register command in `internal/cli/root.go`
  - [x] 1.5: Support `--output json` flag through shared flags system

- [x] Task 2: Implement Task Loading and Validation (AC: #1)
  - [x] 2.1: Load workspace by name from workspace store
  - [x] 2.2: Load active task for workspace
  - [x] 2.3: Validate task is in `awaiting_approval` status
  - [x] 2.4: If not awaiting approval, display error with current status
  - [x] 2.5: Display task summary before decision flow

- [x] Task 3: Implement Decision Flow Menu (AC: #2)
  - [x] 3.1: Use `tui.Select()` for decision menu
  - [x] 3.2: Create options: "Reject and retry" (with description), "Reject (done)" (with description)
  - [x] 3.3: Handle `tui.ErrMenuCanceled` gracefully
  - [x] 3.4: Map menu selection to appropriate handlers

- [x] Task 4: Implement Feedback Form (AC: #3, #5)
  - [x] 4.1: Use `tui.TextArea()` for multi-line feedback input
  - [x] 4.2: Prompt: "What should be changed or fixed?"
  - [x] 4.3: Validate feedback is not empty (require at least some input)
  - [x] 4.4: Store feedback text for artifact saving

- [x] Task 5: Implement Step Selection (AC: #4)
  - [x] 5.1: Load task step definitions from task store
  - [x] 5.2: Build options list from steps (step number + name)
  - [x] 5.3: Use `tui.Select()` for step selection
  - [x] 5.4: Default to earliest relevant step (e.g., "implement" step)
  - [x] 5.5: Map selection back to step index

- [x] Task 6: Implement "Reject and Retry" Action (AC: #5, #6, #7)
  - [x] 6.1: Save feedback as artifact (`rejection-feedback.md` in task artifacts directory)
  - [x] 6.2: Update task metadata with `rejection_feedback` and `resume_from_step`
  - [x] 6.3: Reset current step to selected step index
  - [x] 6.4: Transition task status to `running` using state machine
  - [x] 6.5: Save updated task state
  - [x] 6.6: Display success: "Task rejected with feedback. Resuming from step N."

- [x] Task 7: Implement "Reject (done)" Action (AC: #8, #9)
  - [x] 7.1: Transition task status to `rejected` using state machine
  - [x] 7.2: Preserve workspace status (do not retire or destroy)
  - [x] 7.3: Save updated task state
  - [x] 7.4: Display info: "Task rejected. Branch '<branch>' preserved at '<worktree-path>'"
  - [x] 7.5: Display suggestion: "You can work on the code manually or destroy the workspace later"

- [x] Task 8: Implement JSON Output Mode (AC: #10)
  - [x] 8.1: Create `rejectResponse` struct for JSON output
  - [x] 8.2: Include: success, action (retry/done), workspace info, task info, feedback (if retry), error (if any)
  - [x] 8.3: Skip interactive menus in JSON mode
  - [x] 8.4: Require `--retry` or `--done` flag when using `--output json`
  - [x] 8.5: Accept `--feedback "text"` flag for JSON mode with retry
  - [x] 8.6: Accept `--step N` flag for JSON mode with retry
  - [x] 8.7: Return appropriate JSON for success/failure cases

- [x] Task 9: Create Test Suite (AC: #1-#10)
  - [x] 9.1: Test command registration and flag parsing
  - [x] 9.2: Test task loading and status validation
  - [x] 9.3: Test reject with retry flow
  - [x] 9.4: Test reject done flow
  - [x] 9.5: Test feedback saving as artifact
  - [x] 9.6: Test state transitions (awaiting_approval → running, awaiting_approval → rejected)
  - [x] 9.7: Test JSON output mode with all flag combinations
  - [x] 9.8: Test error cases (workspace not found, invalid status, missing args)
  - [x] 9.9: Mock workspace store and task store for unit tests

- [x] Task 10: Validate and Finalize
  - [x] 10.1: Run `magex format:fix` - must pass
  - [x] 10.2: Run `magex lint` - must pass
  - [x] 10.3: Run `magex test:race` - must pass
  - [x] 10.4: Run `go-pre-commit run --all-files` - must pass

## Dev Notes

### Existing CLI Infrastructure

**DO NOT recreate - reuse these patterns from existing CLI commands:**

**From `internal/cli/approve.go` (reference implementation - Story 8.3):**
```go
// Command structure pattern
func AddApproveCommand(root *cobra.Command) {
    root.AddCommand(newApproveCmd())
}

type approveOptions struct {
    // options struct
}

func newApproveCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "approve [workspace]",
        Short: "Approve completed work and mark ready for merge",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return runApprove(cmd.Context(), cmd, os.Stdout, opts, args)
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
// Selection menu - USE THIS for decision/step selection
tui.Select(title string, options []tui.Option) (string, error)

type Option struct {
    Label       string
    Description string
    Value       string
}

// Multi-line text input - USE THIS for feedback form
tui.TextArea(prompt string, placeholder string) (string, error)

// Cancel detection
tui.ErrMenuCanceled // Returned when user presses q/Esc
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
    TaskStatusRunning          constants.TaskStatus = "running"
    TaskStatusRejected         constants.TaskStatus = "rejected"
)
```

### State Machine

**From `internal/task/state.go`:**
```go
// Valid transitions for reject:
// awaiting_approval → running    (reject with retry)
// awaiting_approval → rejected   (reject done)
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

### Decision Flow UI

Based on UX specification (ux-design-specification.md) and epics.md:

```
? How would you like to proceed?
  ❯ Reject and retry — AI will retry with your feedback
    Reject (done) — End task, preserve branch for manual work
```

**Feedback Form:**
```
What should be changed or fixed?
┌────────────────────────────────────────────────────────────────┐
│ The authentication flow should use OAuth2 instead of basic    │
│ auth. Please also add proper error handling for network       │
│ timeouts.                                                     │
│                                                                │
│                                                                │
└────────────────────────────────────────────────────────────────┘
[enter] Submit  [esc] Cancel
```

**Step Selection:**
```
? Resume from which step?
  ❯ Step 3: implement — Code implementation
    Step 4: validate — Run validation
    Step 5: git_commit — Create commit
    Step 2: plan — Create implementation plan
```

### Artifact Storage

**Feedback artifact location:**
```
~/.atlas/workspaces/<workspace>/tasks/<task-id>/artifacts/rejection-feedback.md
```

**Feedback artifact format:**
```markdown
# Rejection Feedback

Date: 2025-12-31T10:00:00Z
Resume From: Step 3 (implement)

## Feedback

The authentication flow should use OAuth2 instead of basic auth.
Please also add proper error handling for network timeouts.
```

### Git Intelligence

Recent commit patterns from this epic:
```
feat(cli): add approve command for task approval workflow
feat(tui): add approval summary component
feat(tui): add interactive menu system using Charm Huh
```

Expected commit format:
```
feat(cli): add atlas reject command for task rejection workflow
```

### JSON Output Mode

When `--output json` is used, require explicit flags:
```bash
# Reject with retry
atlas reject payment --output json --retry --feedback "Fix the auth flow" --step 3

# Reject done
atlas reject payment --output json --done
```

**JSON Response Schema:**
```go
type rejectResponse struct {
    Success       bool   `json:"success"`
    Action        string `json:"action"`        // "retry" or "done"
    WorkspaceName string `json:"workspace_name"`
    TaskID        string `json:"task_id"`
    Feedback      string `json:"feedback,omitempty"`
    ResumeStep    int    `json:"resume_step,omitempty"`
    BranchName    string `json:"branch_name,omitempty"`
    WorktreePath  string `json:"worktree_path,omitempty"`
    Error         string `json:"error,omitempty"`
}
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
- `internal/cli/reject.go` - Main reject command implementation
- `internal/cli/reject_test.go` - Comprehensive test suite

**Files to modify:**
- `internal/cli/root.go` - Add `AddRejectCommand(rootCmd)` registration

**Alignment with project structure (from architecture.md):**
```
internal/
├── cli/
│   ├── reject.go       # ← NEW: atlas reject
│   ├── reject_test.go  # ← NEW: tests
│   ├── approve.go      # Reference pattern
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

### Previous Story Learnings (Story 8.3)

From Story 8.3 (approve command) implementation:
1. **Use Output interface** - `tui.NewOutput(w, format)` for consistent output
2. **Handle ErrMenuCanceled** - Use `errors.Is(err, tui.ErrMenuCanceled)` for cancel detection
3. **Check context cancellation** - Check at function entry for long operations
4. **Call CheckNoColor()** - At function entry for NO_COLOR compliance
5. **State transitions** - Use `task.Transition()` with proper state machine
6. **JSON mode** - Require explicit args, skip interactive UI

From Story 8.2 (approval summary):
1. **Use AdaptiveColor directly** - Don't hardcode color values
2. **Test width adaptation** - Handle different terminal widths

From Story 8.1 (menus):
1. **Menu loop pattern** - View actions return to menu, terminal actions exit
2. **Option struct** - Use Label, Description, Value consistently

### Error Handling Patterns

From project-context.md:
```go
// Action-first format
return fmt.Errorf("failed to load task: %w", err)

// Use sentinel errors from internal/errors
atlaserrors.ErrTaskNotFound
atlaserrors.ErrInvalidTransition
atlaserrors.ErrInvalidStatus
```

### Gitleaks Compliance

From project-context.md:
- ❌ NEVER use numeric suffixes in test values: `_12345`, `_123`, `_98765`
- ✅ DO use semantic names: `ATLAS_TEST_WORKSPACE_REJECT`, `test_feedback_message`

### References

- [Source: _bmad-output/planning-artifacts/epics.md#epic-8 - Story 8.4 acceptance criteria]
- [Source: _bmad-output/planning-artifacts/architecture.md - CLI package structure, import rules]
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md - Rejection flow, UX-11/UX-12]
- [Source: _bmad-output/project-context.md - Validation commands, error handling, coding standards]
- [Source: internal/cli/approve.go - CLI command pattern reference for reject]
- [Source: internal/tui/menus.go - Select, TextArea, Option types, ErrMenuCanceled]
- [Source: internal/tui/output.go - Output interface, NewOutput]
- [Source: internal/task/state.go - State machine, valid transitions]
- [Source: internal/constants/status.go - TaskStatus constants]
- [Source: _bmad-output/implementation-artifacts/8-3-implement-atlas-approve-command.md - Approve command learnings]
- [Source: _bmad-output/implementation-artifacts/8-2-approval-summary-component.md - Approval summary learnings]
- [Source: _bmad-output/implementation-artifacts/8-1-interactive-menu-system.md - Menu system learnings]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

- No debug issues encountered

### Completion Notes List

- Implemented `atlas reject` command following patterns from `atlas approve`
- Created two rejection flows: "Reject and retry" (transitions to running) and "Reject (done)" (transitions to rejected)
- Feedback form uses `tui.TextArea()` with validation for non-empty input (loop-based, not recursive)
- Step selection menu defaults to earliest "implement" step using intelligent detection
- Rejection feedback saved as `rejection-feedback.md` artifact with date and resume step info
- JSON mode requires explicit `--retry` or `--done` flag, plus `--feedback` and `--step` for retry
- JSON mode step input is 1-indexed (0 = auto-select), consistent with UI display
- All state transitions use `task.Transition()` with proper state machine validation
- Comprehensive test suite with 45+ test cases covering all acceptance criteria
- All validation commands pass: `magex format:fix`, `magex lint`, `magex test:race`, `go-pre-commit run --all-files`

### File List

- `internal/cli/reject.go` (NEW) - Main reject command implementation (~600 lines)
- `internal/cli/reject_test.go` (NEW) - Comprehensive test suite (~1000 lines)
- `internal/cli/root.go` (MODIFIED) - Added `AddRejectCommand(cmd)` registration
- `internal/cli/approve_test.go` (MODIFIED) - Mock interface signature formatting fixes

### Change Log

- 2025-12-31: Implemented Story 8.4 - `atlas reject` command with interactive and JSON modes
- 2025-12-31: Code Review - Fixed 5 issues: recursive feedback loop → iterative, step indexing consistency (1-indexed), placeholder tests replaced with meaningful tests, documented approve_test.go changes

