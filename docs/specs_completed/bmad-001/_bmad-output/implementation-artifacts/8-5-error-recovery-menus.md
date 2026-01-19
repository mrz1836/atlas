# Story 8.5: Error Recovery Menus

Status: review

> **Note**: The `atlas recover` command mentioned in this document was consolidated into `atlas resume` in commit d8c73f1. All references to `atlas recover` should now use `atlas resume` instead. The resume command provides all recovery functionality through an interactive menu.

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **interactive menus when errors occur**,
So that **I can decide how to proceed without memorizing commands**.

## Acceptance Criteria

1. **Given** a task is in `validation_failed` state
   **When** I run `atlas status` or the error state is displayed
   **Then** an error-specific menu is presented with recovery options

2. **Given** a task is in `gh_failed` state (GitHub operation failed)
   **When** I view the error state
   **Then** a menu is presented with GitHub-specific recovery options

3. **Given** a task is in `ci_failed` state
   **When** I view the error state
   **Then** a menu is presented with CI-specific recovery options

4. **Given** any error recovery menu is displayed
   **When** presenting options
   **Then** all menus follow consistent styling (UX-11, UX-12)

5. **Given** any error recovery menu is displayed
   **When** user navigates
   **Then** escape routes are always available (Cancel, Abandon)

6. **Given** user selects "Retry with AI fix" from validation_failed menu
   **When** processing the selection
   **Then** task transitions to `running` and AI receives error context

7. **Given** user selects "Fix manually" from any error menu
   **When** processing the selection
   **Then** displays worktree path and `atlas resume` instructions

8. **Given** user selects "Abandon task" from any error menu
   **When** processing the selection
   **Then** task transitions to `abandoned` state, preserving branch and worktree

9. **Given** user selects "View workflow logs" from ci_failed menu
   **When** processing the selection
   **Then** opens the GitHub Actions URL in the default browser

10. **Given** `--output json` flag is used
    **When** running error recovery commands
    **Then** outputs structured JSON instead of interactive UI

## Tasks / Subtasks

- [x] Task 1: Create Error Recovery Menu Component (AC: #1, #4)
  - [x] 1.1: Create `internal/tui/error_recovery.go` with base error menu component
  - [x] 1.2: Define `ErrorRecoveryOption` type with action type
  - [x] 1.3: Create `ErrorRecoveryOption` struct matching `tui.Option` pattern
  - [x] 1.4: Apply established style system from `styles.go` (UX-11, UX-12)
  - [x] 1.5: Add `SelectErrorRecovery(status) (RecoveryAction, error)` function

- [x] Task 2: Implement Validation Failed Menu (AC: #1, #6, #7, #8)
  - [x] 2.1: Create `ValidationFailedOptions()` returning menu options
  - [x] 2.2: Implement recovery actions for each choice
  - [x] 2.3: Display worktree path and resume instructions for "Fix manually"
  - [x] 2.4: Transition to `abandoned` state for "Abandon task"

- [x] Task 3: Implement GitHub Failed Menu (AC: #2, #7, #8)
  - [x] 3.1: Create `GHFailedOptions()` returning menu options
  - [x] 3.2: Handle both push and PR creation failures

- [x] Task 4: Implement CI Failed Menu (AC: #3, #7, #8, #9)
  - [x] 4.1: Create `CIFailedOptions()` returning menu options
  - [x] 4.2: Open GitHub Actions URL in browser for "View workflow logs"

- [x] Task 5: Implement CI Timeout Menu (AC: #3, #7, #8)
  - [x] 5.1: Create `CITimeoutOptions()` returning menu options
  - [x] 5.2: Support "Continue waiting" to resume CI polling

- [x] Task 6: Create Error Recovery CLI Command (AC: #1-#10)
  - [x] 6.1: Create `internal/cli/recover.go` with `newRecoverCmd()` function
  - [x] 6.2: Add `AddRecoverCommand(root *cobra.Command)` function
  - [x] 6.3: Register command in `internal/cli/root.go`
  - [x] 6.4: Load workspace and task, validate error state
  - [x] 6.5: Route to appropriate error menu based on task status
  - [x] 6.6: Support `--output json` flag for non-interactive mode

- [x] Task 7: Integrate with Status Command (AC: #1, #4)
  - [x] 7.1: Updated `SuggestedAction()` to return "atlas recover" for error states
  - [x] 7.2: Display "Run: atlas recover <workspace>" in ACTION column for error states

- [x] Task 8: Implement Menu Actions (AC: #6, #7, #8, #9)
  - [x] 8.1: Create `handleRetryAction()` - transitions to running
  - [x] 8.2: Create `handleFixManually()` - displays path and instructions
  - [x] 8.3: Create `handleAbandon()` - transitions to abandoned
  - [x] 8.4: Create `handleViewLogs()` - opens GitHub Actions URL
  - [x] 8.5: Create `handleContinueWaiting()` - resumes CI polling

- [x] Task 9: Implement JSON Output Mode (AC: #10)
  - [x] 9.1: Create `recoverResponse` struct for JSON output
  - [x] 9.2: Require explicit action flag when using `--output json`:
    - `--retry` for retry with AI fix
    - `--manual` for fix manually instructions
    - `--abandon` for abandon task
    - `--continue` for continue waiting (CI timeout only)
  - [x] 9.3: Return appropriate JSON for each action

- [x] Task 10: Create Test Suite (AC: #1-#10)
  - [x] 10.1: Test menu option generation for each error state
  - [x] 10.2: Test choice handling and state transitions
  - [x] 10.3: Test validation_failed → running transition
  - [x] 10.4: Test validation_failed → abandoned transition
  - [x] 10.5: Test gh_failed recovery options
  - [x] 10.6: Test ci_failed recovery options
  - [x] 10.7: Test ci_timeout recovery options
  - [x] 10.8: Test JSON output mode with all flag combinations
  - [x] 10.9: Test ErrMenuCanceled handling for all menus
  - [x] 10.10: Mock workspace store, task store for unit tests

- [x] Task 11: Validate and Finalize
  - [x] 11.1: All tests pass
  - [x] 11.2: Lint passes (golangci-lint)
  - [x] 11.3: Pre-commit checks pass (go-pre-commit run --all-files)

## Dev Notes

### Existing CLI Infrastructure

**DO NOT recreate - reuse these patterns from existing CLI commands:**

**From `internal/cli/reject.go` (reference implementation - Story 8.4):**
```go
// Command structure pattern
func AddRejectCommand(root *cobra.Command) {
    root.AddCommand(newRejectCmd())
}

type rejectOptions struct {
    // options struct
}

func newRejectCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "reject [workspace]",
        Short: "Reject work with feedback",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return runReject(cmd.Context(), cmd, os.Stdout, opts, args)
        },
    }
    return cmd
}

// Decision flow pattern
decision, err := tui.Select("How would you like to proceed?", []tui.Option{
    {Label: "Reject and retry", Description: "AI will retry with your feedback", Value: "retry"},
    {Label: "Reject (done)", Description: "End task, preserve branch for manual work", Value: "done"},
})
```

**From `internal/cli/approve.go` (reference implementation - Story 8.3):**
```go
// Shared infrastructure
tui.CheckNoColor()
out := tui.NewOutput(w, outputFormat)
logger := GetLogger()
```

### TUI Components (Story 8.1, 8.2)

**From `internal/tui/menus.go`:**
```go
// Selection menu - USE THIS for error recovery menus
tui.Select(title string, options []tui.Option) (string, error)

type Option struct {
    Label       string
    Description string
    Value       string
}

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
```

### Error States and Task Status

**From `internal/constants/status.go`:**
```go
const (
    TaskStatusValidationFailed constants.TaskStatus = "validation_failed"
    TaskStatusGHFailed         constants.TaskStatus = "gh_failed"
    TaskStatusCIFailed         constants.TaskStatus = "ci_failed"
    TaskStatusCITimeout        constants.TaskStatus = "ci_timeout"
    TaskStatusRunning          constants.TaskStatus = "running"
    TaskStatusAbandoned        constants.TaskStatus = "abandoned"
)
```

**From `internal/task/state.go`:**
```go
// Valid transitions for error recovery:
// validation_failed → running    (retry with AI)
// validation_failed → abandoned  (abandon task)
// gh_failed → running            (retry GitHub op)
// gh_failed → abandoned          (abandon task)
// ci_failed → running            (retry from implement)
// ci_failed → abandoned          (abandon task)
// ci_timeout → running           (continue waiting)
// ci_timeout → abandoned         (abandon task)
var validTransitions = map[constants.TaskStatus][]constants.TaskStatus{
    constants.TaskStatusValidationFailed: {
        constants.TaskStatusRunning,
        constants.TaskStatusAbandoned,
    },
    constants.TaskStatusGHFailed: {
        constants.TaskStatusRunning,
        constants.TaskStatusAbandoned,
    },
    constants.TaskStatusCIFailed: {
        constants.TaskStatusRunning,
        constants.TaskStatusAbandoned,
    },
    constants.TaskStatusCITimeout: {
        constants.TaskStatusRunning,
        constants.TaskStatusAbandoned,
    },
}
```

### Error Recovery UX Design

**From UX specification (ux-design-specification.md):**

**Validation Failed Menu:**
```
? Validation failed. What would you like to do?
  ❯ Retry with AI fix — Claude attempts to fix based on errors
    Fix manually — Edit files in worktree, then resume
    View errors — Show detailed validation output
    Abandon task — End task, keep branch for later
```

**GitHub Operation Failed Menu:**
```
? GitHub operation failed. What would you like to do?
  ❯ Retry push/PR — Retry the failed operation
    Fix manually — Check and fix issues, then resume
    Abandon task — End task, keep branch for later
```

**CI Failed Menu (from Story 6.7):**
```
? CI workflow "CI" failed. What would you like to do?
  ❯ View workflow logs — Open GitHub Actions in browser
    Retry from implement — AI tries to fix based on CI output
    Fix manually — You fix in worktree, then resume
    Abandon task — End task, keep PR as draft
```

**CI Timeout Menu:**
```
? CI polling timeout. What would you like to do?
  ❯ Continue waiting — Resume polling with extended timeout
    View workflow logs — Check CI status in browser
    Fix manually — Check CI status and resume when ready
    Abandon task — End task
```

### Menu Interaction Pattern (UX-11, UX-12)

From UX design:
```
[↑↓] Navigate  [enter] Select  [q] Cancel
```

All menus must:
- Use established style system (colors, icons from `styles.go`)
- Support keyboard navigation (arrow keys + enter)
- Have escape route (q/Esc returns `ErrMenuCanceled`)
- Follow consistent option format (Label + Description)

### Browser Launch Pattern

**For opening GitHub Actions URL:**
```go
import "os/exec"

func openBrowser(url string) error {
    // macOS-specific (ATLAS v1 is macOS-only)
    return exec.Command("open", url).Run()
}
```

### Error Context Extraction

**Validation errors from artifacts:**
```go
// Load validation result artifact
artifactPath := filepath.Join(
    task.ArtifactsDir(),
    "validation.json", // or validation.N.json for versioned
)

// Parse and extract error messages for AI context
type ValidationResult struct {
    Command  string `json:"command"`
    ExitCode int    `json:"exit_code"`
    Stdout   string `json:"stdout"`
    Stderr   string `json:"stderr"`
    Passed   bool   `json:"passed"`
}
```

**CI errors from artifacts:**
```go
// Load CI result artifact
artifactPath := filepath.Join(
    task.ArtifactsDir(),
    "ci-result.json",
)

type CIResult struct {
    Workflow    string `json:"workflow"`
    Status      string `json:"status"`
    URL         string `json:"url"`         // GitHub Actions URL
    ErrorOutput string `json:"error_output"`
}
```

### JSON Output Mode

**When `--output json` is used, require explicit action flags:**
```bash
# Retry with AI
atlas recover payment --output json --retry

# Fix manually (returns worktree path and instructions)
atlas recover payment --output json --manual

# Abandon task
atlas recover payment --output json --abandon

# Continue waiting (CI timeout only)
atlas recover payment --output json --continue
```

**JSON Response Schema:**
```go
type errorRecoveryResponse struct {
    Success       bool   `json:"success"`
    Action        string `json:"action"`          // "retry", "manual", "abandon", "continue"
    WorkspaceName string `json:"workspace_name"`
    TaskID        string `json:"task_id"`
    ErrorState    string `json:"error_state"`     // validation_failed, gh_failed, ci_failed, ci_timeout
    WorktreePath  string `json:"worktree_path,omitempty"`
    Instructions  string `json:"instructions,omitempty"`
    GitHubURL     string `json:"github_url,omitempty"`
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
- `internal/tui/error_recovery.go` - Error recovery menu component
- `internal/tui/error_recovery_test.go` - Component tests
- `internal/cli/recover.go` - Error recovery CLI command
- `internal/cli/recover_test.go` - CLI tests

**Files to modify:**
- `internal/cli/root.go` - Add `AddRecoverCommand(cmd)` registration
- `internal/cli/status.go` - Add ACTION column hint for error states

**Alignment with project structure (from architecture.md):**
```
internal/
├── cli/
│   ├── recover.go       # ← NEW: atlas recover
│   ├── recover_test.go  # ← NEW: tests
│   ├── status.go        # ← MODIFY: add error state hints
│   ├── reject.go        # Reference pattern
│   └── approve.go       # Reference pattern
├── tui/
│   ├── error_recovery.go      # ← NEW: error recovery component
│   ├── error_recovery_test.go # ← NEW: component tests
│   ├── menus.go         # Reference - Select, Option, ErrMenuCanceled
│   └── output.go        # Reference - Output interface
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
- `internal/tui` → can import styles, domain, constants, errors

### Previous Story Learnings (Story 8.3, 8.4)

From Story 8.4 (reject command) implementation:
1. **Use Output interface** - `tui.NewOutput(w, format)` for consistent output
2. **Handle ErrMenuCanceled** - Use `errors.Is(err, tui.ErrMenuCanceled)` for cancel detection
3. **Check context cancellation** - Check at function entry for long operations
4. **Call CheckNoColor()** - At function entry for NO_COLOR compliance
5. **State transitions** - Use `task.Transition()` with proper state machine
6. **JSON mode** - Require explicit action flags, skip interactive UI
7. **Iterative input loops** - Use loops instead of recursion for re-prompting

From Story 8.3 (approve command):
1. **Menu loop pattern** - View actions return to menu, terminal actions exit
2. **Option struct** - Use Label, Description, Value consistently

From Story 8.1 (menus):
1. **AtlasTheme()** - Use for consistent Huh styling
2. **adaptWidth()** - Respect terminal width
3. **KeyHints** - Display navigation hints

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
- NEVER use numeric suffixes in test values: `_12345`, `_123`, `_98765`
- DO use semantic names: `ATLAS_TEST_WORKSPACE_RECOVER`, `test_error_message`

### Git Commit Pattern

Expected commit format based on recent Epic 8 commits:
```
feat(tui): add error recovery menu component
feat(cli): add recover command for error recovery workflow
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md#epic-8 - Story 8.5 acceptance criteria]
- [Source: _bmad-output/planning-artifacts/architecture.md - CLI/TUI package structure, import rules]
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md - Error recovery UX, UX-11/UX-12]
- [Source: _bmad-output/project-context.md - Validation commands, error handling, coding standards]
- [Source: internal/cli/reject.go - CLI command pattern reference]
- [Source: internal/cli/approve.go - CLI command pattern reference]
- [Source: internal/tui/menus.go - Select, Option types, ErrMenuCanceled, AtlasTheme]
- [Source: internal/tui/output.go - Output interface, NewOutput]
- [Source: internal/task/state.go - State machine, valid transitions]
- [Source: internal/constants/status.go - TaskStatus constants]
- [Source: _bmad-output/implementation-artifacts/8-4-implement-atlas-reject-command.md - Reject command learnings]
- [Source: _bmad-output/implementation-artifacts/8-3-implement-atlas-approve-command.md - Approve command learnings]
- [Source: _bmad-output/implementation-artifacts/8-1-interactive-menu-system.md - Menu system learnings]
- [Source: _bmad-output/planning-artifacts/epics.md#story-67 - CI failure handling reference]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - Implementation completed without debug logging

### Completion Notes List

1. Created `internal/tui/error_recovery.go` - Core error recovery menu component with:
   - `RecoveryAction` type and constants for all recovery actions
   - `ErrorRecoveryOption` struct extending base `Option`
   - Functions for each error state: `ValidationFailedOptions()`, `GHFailedOptions()`, `CIFailedOptions()`, `CITimeoutOptions()`
   - `GetOptionsForStatus()` to get options for any error state
   - `GetMenuTitleForStatus()` for consistent menu titles
   - `SelectErrorRecovery()` for interactive menu selection
   - `IsViewAction()` and `IsTerminalAction()` helpers for menu loop logic

2. Created `internal/cli/recover.go` - CLI command implementation with:
   - `atlas recover [workspace]` command
   - JSON mode with explicit action flags: `--retry`, `--manual`, `--abandon`, `--continue`
   - Interactive menu loop where view actions return to menu
   - State transitions via `task.Transition()`
   - Error context extraction from task artifacts

3. Updated `internal/tui/styles.go` - Changed `SuggestedAction()` to return "atlas recover" for all error states (validation_failed, gh_failed, ci_failed, ci_timeout)

4. All tests pass, lint passes, pre-commit checks pass

### File List

**New Files:**
- `internal/tui/error_recovery.go` - Error recovery menu component
- `internal/tui/error_recovery_test.go` - Component tests
- `internal/cli/recover.go` - CLI recover command
- `internal/cli/recover_test.go` - CLI tests

**Modified Files:**
- `internal/cli/root.go` - Added `AddRecoverCommand(cmd)` registration
- `internal/tui/styles.go` - Updated `SuggestedAction()` for error states
- `internal/tui/styles_test.go` - Updated test expectations
- `internal/tui/table_test.go` - Updated test expectations
- `internal/tui/watch_test.go` - Updated test expectations
- `internal/tui/footer_test.go` - Updated test expectations
- `internal/errors/errors.go` - Added `ErrInvalidStatus` sentinel error

## Code Review Record

### Review Date
2026-01-01

### Reviewer Model
Claude Opus 4.5 (claude-opus-4-5-20251101)

### Issues Found and Fixed

**HIGH SEVERITY (3 issues - all fixed):**
1. H1: Missing test for `SelectErrorRecovery` non-error status path - Added `TestSelectErrorRecovery_NonErrorStatus`
2. H2: Missing tests for interactive handler functions - Added tests for `handleRetryAction`, `handleFixManually`, `handleAbandon`, `handleContinueWaiting`, `handleViewErrors`
3. H3: Missing test for `handleViewLogs` - Added `TestHandleViewLogs` (no-URL case only to avoid browser side effects)

**MEDIUM SEVERITY (5 issues - all fixed):**
1. M1: Context cancellation not tested in `findErrorTasks` - Added `TestFindErrorTasks_ContextCancellation`
2. M2: `handleViewErrors` wrote directly to `os.Stdout` - Refactored to use `out.Info()` interface
3. M3: No test for workspace selection menu - Added `TestSelectWorkspaceForRecovery`
4. M4: Missing validation for empty worktree path in `handleFixManually` - Added placeholder message for empty path
5. M5: Missing validation for empty worktree path in `processJSONManual` - Added fallback instructions

### Verification Results
- All tests pass with race detection (`magex test:race`)
- All 62 linters pass (`magex lint`)
- All pre-commit checks pass (`go-pre-commit run --all-files`)

### Files Modified During Review
- `internal/cli/recover.go` - M2, M4, M5 fixes
- `internal/cli/recover_test.go` - H2, H3, M1, M3 tests added
- `internal/tui/error_recovery_test.go` - H1 test added
