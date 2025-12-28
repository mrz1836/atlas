# Story 3.6: Implement `atlas workspace retire` Command

Status: done

## Story

As a **user**,
I want **to run `atlas workspace retire <name>` to archive a completed workspace**,
So that **I preserve the task history while freeing up disk space**.

## Acceptance Criteria

1. **Given** a workspace exists with status "active" or "paused" **When** I run `atlas workspace retire auth` **Then** the system:
   - Verifies no tasks are currently running
   - Updates workspace status to "retired"
   - Removes the git worktree (but keeps the branch)
   - Preserves `~/.atlas/workspaces/auth/` with all task history

2. **Given** retire succeeds **When** operation completes **Then** displays success: "Workspace 'auth' retired. History preserved."

3. **Given** tasks are running in the workspace **When** I run `atlas workspace retire auth` **Then** displays error: "Cannot retire workspace with running tasks"

4. **Given** workspace is already retired **When** I run `atlas workspace retire auth` **Then** displays appropriate message (already retired or error)

5. **Given** workspace doesn't exist **When** I run `atlas workspace retire nonexistent` **Then** displays clear error: "Workspace 'nonexistent' not found"

6. **Given** any state **When** retire completes **Then** retired workspaces still appear in `workspace list` with "retired" status

7. **Given** any state **When** retire completes **Then** retired workspaces can be referenced for log viewing (future story 3-7)

8. **Given** `--output json` flag **When** running retire **Then** outputs structured JSON result

## Tasks / Subtasks

- [x] Task 1: Create retire subcommand structure (AC: #1, #5, #8)
  - [x] 1.1: Create `internal/cli/workspace_retire.go` with the `retire` subcommand
  - [x] 1.2: Create `internal/cli/workspace_retire_test.go` for tests
  - [x] 1.3: Add `addWorkspaceRetireCmd(parent *cobra.Command)` function
  - [x] 1.4: Register retire command in `workspace.go` (replace Future comment)

- [x] Task 2: Implement confirmation prompt (AC: #1)
  - [x] 2.1: Create confirmation prompt using charmbracelet/huh (similar to destroy)
  - [x] 2.2: Add `--force` flag to skip confirmation
  - [x] 2.3: Handle non-interactive mode (error if no --force and not TTY)
  - [x] 2.4: Use different confirmation message: "Retire workspace 'name'? Worktree will be removed but history preserved."

- [x] Task 3: Implement retire command logic (AC: #1, #2, #3, #4)
  - [x] 3.1: Implement `runWorkspaceRetire(ctx, cmd, name string, force bool) error`
  - [x] 3.2: Check workspace exists first (clear error if not)
  - [x] 3.3: Check workspace status - if already retired, display appropriate message
  - [x] 3.4: Create WorktreeRunner from repo path (detect from cwd)
  - [x] 3.5: Create Manager with store and worktreeRunner
  - [x] 3.6: Call `manager.Retire(ctx, name)` - already implements running task check!
  - [x] 3.7: Handle ErrWorkspaceHasRunningTasks with user-friendly message (AC #3)
  - [x] 3.8: Handle and display success message with checkmark icon

- [x] Task 4: Implement JSON output support (AC: #8)
  - [x] 4.1: Detect `--output json` flag from global flags
  - [x] 4.2: Output JSON success: `{"status": "retired", "workspace": "name", "history_preserved": true}`
  - [x] 4.3: On error with JSON output, output JSON error object
  - [x] 4.4: Use `ErrJSONErrorOutput` pattern from destroy command for exit codes

- [x] Task 5: Write comprehensive tests (AC: all)
  - [x] 5.1: Test retire happy path (active workspace)
  - [x] 5.2: Test retire with paused workspace
  - [x] 5.3: Test retire with running tasks (error - AC #3)
  - [x] 5.4: Test retire already retired workspace (AC #4)
  - [x] 5.5: Test retire with non-existent workspace (AC #5)
  - [x] 5.6: Test retire with --force flag (skip confirmation)
  - [x] 5.7: Test retire with --output json flag (AC #8)
  - [x] 5.8: Test non-interactive mode without --force (error)
  - [x] 5.9: Test context cancellation
  - [x] 5.10: Run `magex format:fix && magex lint && magex test:race`

## Dev Notes

### Critical Warnings (READ FIRST)

1. **Manager.Retire() already implements the core logic** - It checks for running tasks, updates status, removes worktree. The CLI just needs to call it and display results.
2. **Manager.Retire() returns ErrWorkspaceHasRunningTasks** - CLI must catch this and display user-friendly error for AC #3.
3. **Manager.Retire() does NOT delete the branch** - Unlike Destroy, Retire keeps the branch (per AC #1).
4. **MUST get worktreeRunner from actual repo** - Unlike `list` which passes nil, retire NEEDS a real WorktreeRunner to remove worktrees.
5. **MUST use huh for confirmation** - Not fmt.Scanln or manual prompt. Follow destroy command pattern.
6. **MUST respect --output flag** - JSON vs text output modes.
7. **Copy patterns from workspace_destroy.go** - Similar structure, confirmation flow, error handling.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/cli/workspace.go` | EXISTS - Update to add addWorkspaceRetireCmd call |
| `internal/cli/workspace_retire.go` | NEW - Retire subcommand implementation |
| `internal/cli/workspace_retire_test.go` | NEW - Tests for retire command |
| `internal/cli/workspace_destroy.go` | EXISTS - Reference for patterns |
| `internal/workspace/manager.go` | EXISTS - Manager.Retire() already implemented |
| `internal/workspace/worktree.go` | EXISTS - WorktreeRunner interface |
| `internal/tui/output.go` | EXISTS - Output interface for styled messages |
| `internal/constants/status.go` | EXISTS - WorkspaceStatusRetired constant |

### Import Rules (CRITICAL)

**`internal/cli/workspace_retire.go` MAY import:**
- `internal/workspace` - for Manager, NewManager, NewFileStore, NewGitWorktreeRunner
- `internal/tui` - for CheckNoColor
- `internal/constants` - for WorkspaceStatusRetired
- `internal/errors` - for ErrWorkspaceHasRunningTasks, ErrWorkspaceNotFound, ErrJSONErrorOutput, ErrNonInteractiveMode, ErrNotGitRepo
- `context`, `fmt`, `io`, `os`
- `encoding/json`
- `github.com/spf13/cobra`
- `github.com/charmbracelet/huh` - for confirmation prompt
- `github.com/charmbracelet/lipgloss` - for styled output
- `golang.org/x/term` - for TTY detection

**MUST NOT import:**
- `internal/task` - retire doesn't need task engine
- `internal/ai` - no AI operations
- `internal/git` - use workspace.WorktreeRunner, not git package directly

### Manager.Retire() Already Implemented

The `Manager.Retire()` method in `internal/workspace/manager.go` (lines 200-253) already:
1. Checks for context cancellation
2. Loads the workspace (returns error if not found)
3. Checks for running tasks (returns `ErrWorkspaceHasRunningTasks` if any)
4. Updates status to `WorkspaceStatusRetired`
5. Clears `WorktreePath` in state
6. Persists the state change
7. Removes the worktree (with force fallback)
8. Does NOT delete the branch (intentional per requirements)

The CLI command needs to:
1. Check workspace exists (for user-friendly error message)
2. Check if already retired (optional - give informative message)
3. Get confirmation (or --force)
4. Create manager with real WorktreeRunner
5. Call manager.Retire()
6. Handle ErrWorkspaceHasRunningTasks specially
7. Display success/error message

### Command Structure Pattern (from workspace_destroy.go)

```go
// internal/cli/workspace_retire.go
func addWorkspaceRetireCmd(parent *cobra.Command) {
    var force bool

    cmd := &cobra.Command{
        Use:   "retire <name>",
        Short: "Retire a workspace, preserving history",
        Long: `Archive a completed workspace by removing its git worktree
while preserving all task history and the git branch.

Use this when you're done with a workspace but want to keep the history
for reference. The retired workspace will still appear in 'workspace list'.

Examples:
  atlas workspace retire auth          # Confirm and retire
  atlas workspace retire auth --force  # Retire without confirmation`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            err := runWorkspaceRetire(cmd.Context(), cmd, os.Stdout, args[0], force, "")
            if stderrors.Is(err, errors.ErrJSONErrorOutput) {
                cmd.SilenceErrors = true
            }
            return err
        },
    }

    cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

    parent.AddCommand(cmd)
}
```

### Confirmation Prompt Pattern

```go
func confirmRetire(name string) (bool, error) {
    var confirm bool

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title(fmt.Sprintf("Retire workspace '%s'?", name)).
                Description("Worktree will be removed but history preserved.").
                Affirmative("Yes, retire").
                Negative("No, cancel").
                Value(&confirm),
        ),
    )

    if err := form.Run(); err != nil {
        return false, err
    }

    return confirm, nil
}
```

### JSON Output Format

**Success:**
```json
{
  "status": "retired",
  "workspace": "auth",
  "history_preserved": true
}
```

**Error (running tasks):**
```json
{
  "status": "error",
  "workspace": "auth",
  "error": "cannot retire workspace: task 'task-123' is still running"
}
```

**Error (not found):**
```json
{
  "status": "error",
  "workspace": "nonexistent",
  "error": "workspace not found"
}
```

### Retire Result Type

```go
type retireResult struct {
    Status           string `json:"status"`
    Workspace        string `json:"workspace"`
    HistoryPreserved bool   `json:"history_preserved,omitempty"`
    Error            string `json:"error,omitempty"`
}
```

### Error Handling Pattern

```go
// Handle specific errors from Manager.Retire()
if err := mgr.Retire(ctx, name); err != nil {
    // Check for running tasks error
    if stderrors.Is(err, errors.ErrWorkspaceHasRunningTasks) {
        if output == OutputJSON {
            _ = outputRetireErrorJSON(w, name, "cannot retire workspace with running tasks")
            return errors.ErrJSONErrorOutput
        }
        return fmt.Errorf("Cannot retire workspace '%s' with running tasks: %w", name, err)
    }

    // Other errors
    if output == OutputJSON {
        _ = outputRetireErrorJSON(w, name, err.Error())
        return errors.ErrJSONErrorOutput
    }
    return fmt.Errorf("failed to retire workspace '%s': %w", name, err)
}
```

### Handle Already Retired Case

```go
// Check if workspace is already retired before confirmation
ws, err := store.Get(ctx, name)
if err != nil {
    // Handle error
}

if ws.Status == constants.WorkspaceStatusRetired {
    if output == OutputJSON {
        _ = outputRetireSuccessJSON(w, name) // Still success, just already done
        return nil
    }
    fmt.Fprintf(w, "Workspace '%s' is already retired.\n", name)
    return nil
}
```

### Re-use Helper Functions from workspace_destroy.go

These functions can be reused or copied with modifications:
- `terminalCheck` / `isTerminal()` - Use the existing global variable pattern
- `detectRepoPath()` - Reuse directly (it's not workspace-specific)
- Pattern for `checkWorkspaceExists()` - Adapt for retire

### Testing Pattern

```go
func TestRunWorkspaceRetire_HappyPath(t *testing.T) {
    tmpDir := t.TempDir()

    // Create test workspace with active status
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    ws := &domain.Workspace{
        Name:         "test-ws",
        WorktreePath: "/tmp/test-worktree",
        Branch:       "feat/test",
        Status:       constants.WorkspaceStatusActive,
        Tasks:        []domain.TaskRef{}, // No running tasks
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }
    require.NoError(t, store.Create(context.Background(), ws))

    // Create mock worktree runner
    mockRunner := &MockWorktreeRunner{
        RemoveFunc: func(ctx context.Context, path string, force bool) error {
            return nil
        },
        // Note: Retire does NOT call DeleteBranch or Prune
    }

    // Create manager and retire
    mgr := workspace.NewManager(store, mockRunner)
    err = mgr.Retire(context.Background(), "test-ws")
    require.NoError(t, err)

    // Verify workspace is retired but still exists
    ws2, err := store.Get(context.Background(), "test-ws")
    require.NoError(t, err)
    assert.Equal(t, constants.WorkspaceStatusRetired, ws2.Status)
    assert.Empty(t, ws2.WorktreePath) // Worktree path cleared
    assert.Equal(t, "feat/test", ws2.Branch) // Branch preserved
}

func TestRunWorkspaceRetire_WithRunningTasks(t *testing.T) {
    tmpDir := t.TempDir()
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    // Create workspace with a running task
    ws := &domain.Workspace{
        Name:   "test-ws",
        Status: constants.WorkspaceStatusActive,
        Tasks: []domain.TaskRef{
            {ID: "task-123", Status: constants.TaskStatusRunning},
        },
    }
    require.NoError(t, store.Create(context.Background(), ws))

    mgr := workspace.NewManager(store, nil)
    err = mgr.Retire(context.Background(), "test-ws")

    // Should fail with running tasks error
    assert.Error(t, err)
    assert.ErrorIs(t, err, errors.ErrWorkspaceHasRunningTasks)
}
```

### Previous Story Learnings (from Story 3-5)

1. **Context as first parameter** - Always check `ctx.Done()` at entry
2. **Action-first error messages** - `"failed to retire workspace: %w"`
3. **Use constants package** - Never inline magic strings for status
4. **Use errors package** - Never define local sentinel errors, use existing ones
5. **Use tui.CheckNoColor()** - Respect NO_COLOR environment variable (UX-7)
6. **Run `magex test:race`** - Race detection is mandatory
7. **Test empty/error states** - Edge cases are important
8. **Use injectable terminalCheck** - For testing non-interactive mode
9. **Use ErrJSONErrorOutput pattern** - For correct exit codes with JSON output

### Differences from Destroy Command

| Aspect | Destroy | Retire |
|--------|---------|--------|
| Worktree | Removed | Removed |
| Branch | Deleted | **Preserved** |
| State files | Deleted | **Preserved** |
| Task history | Lost | **Preserved** |
| Workspace.json | Deleted | Updated (status=retired) |
| Can be undone | No | Partially (recreate worktree manually) |
| Running tasks | Allowed (NFR18) | **Error returned** |
| Final status | Gone | Shows as "retired" in list |

### File Structure After This Story

```
internal/
├── cli/
│   ├── root.go              # EXISTS
│   ├── workspace.go         # MODIFY - add addWorkspaceRetireCmd
│   ├── workspace_list.go    # EXISTS
│   ├── workspace_list_test.go # EXISTS
│   ├── workspace_destroy.go # EXISTS - reference for patterns
│   ├── workspace_destroy_test.go # EXISTS
│   ├── workspace_retire.go  # NEW - retire subcommand
│   └── workspace_retire_test.go # NEW - retire tests
└── workspace/
    ├── manager.go           # EXISTS - Manager.Retire() already implemented
    ├── store.go             # EXISTS
    └── worktree.go          # EXISTS - WorktreeRunner interface
```

### Dependencies Between Stories

This story builds on:
- **Story 3-1** (Workspace Data Model and Store) - uses Store.Get(), Store.Update()
- **Story 3-2** (Git Worktree Operations) - uses WorktreeRunner.Remove()
- **Story 3-3** (Workspace Manager Service) - uses Manager.Retire() (already implemented!)
- **Story 3-4** (Workspace List Command) - patterns for CLI commands
- **Story 3-5** (Workspace Destroy Command) - similar patterns, copy/adapt code

This story is required by:
- **Story 3-7** (`atlas workspace logs`) - retired workspaces should be viewable

### Security Considerations

1. **Confirmation required by default** - Non-destructive but still significant operation
2. **--force flag documented clearly** - User understands they're bypassing safety
3. **No secrets exposed** - Workspace paths are safe to display
4. **File permissions inherited** - From store (0o750/0o600)
5. **Running task protection** - Cannot accidentally retire active work

### Performance Considerations

1. **NFR1: <1 second** - Retire should be fast (status update + worktree removal)
2. **No network calls** - Pure local operations
3. **State update before worktree removal** - Ensures consistency even if worktree removal fails

### Edge Cases to Handle

1. **Workspace doesn't exist** - Clear error message before confirmation
2. **Workspace already retired** - Informative message (not an error)
3. **Worktree already removed manually** - Retire should still succeed (update state)
4. **Running tasks** - Block retire with clear error
5. **Validating tasks** - Also block retire (TaskStatusValidating)
6. **Context cancelled during confirm** - Return ctx.Err()
7. **Non-TTY without --force** - Error with clear instructions

### Task Status Values That Block Retire

From `internal/workspace/manager.go` lines 216-220:
```go
if task.Status == constants.TaskStatusRunning ||
    task.Status == constants.TaskStatusValidating {
    return fmt.Errorf("cannot retire workspace '%s': task '%s' is still running: %w",
        name, task.ID, atlaserrors.ErrWorkspaceHasRunningTasks)
}
```

So only `Running` and `Validating` block retire. Other statuses like `Pending`, `AwaitingApproval`, `Completed`, etc. are OK.

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.6]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure & Boundaries]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/workspace/manager.go:200-253 - Manager.Retire() implementation]
- [Source: internal/cli/workspace_destroy.go - CLI command patterns to follow]
- [Source: internal/cli/workspace.go - Parent command structure]
- [Source: _bmad-output/implementation-artifacts/3-5-implement-atlas-workspace-destroy-command.md - Previous story patterns]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test:
go run ./cmd/atlas workspace retire nonexistent        # Should show "not found" error
go run ./cmd/atlas workspace retire --help             # Should show help with --force flag

# Integration test (requires actual workspace):
# atlas workspace list                                  # Verify workspace exists
# atlas workspace retire <name>                         # Test confirmation prompt
# atlas workspace retire <name> --force                 # Test force flag
# atlas workspace list                                  # Verify shows as "retired"
# atlas workspace retire <name> --output json --force   # Test JSON output
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

None - implementation proceeded without errors.

### Completion Notes List

- Implemented `atlas workspace retire` command following patterns from `workspace_destroy.go`
- Created confirmation prompt using charmbracelet/huh with descriptive message about history preservation
- Implemented JSON output support with `retireResult` struct containing `history_preserved` field
- Added comprehensive test coverage (16 test cases) covering:
  - Happy path (active and paused workspaces)
  - Running/validating tasks error handling (AC #3)
  - Already retired workspace handling (AC #4)
  - Workspace not found error (AC #5)
  - Force flag, JSON output, context cancellation, non-interactive mode
- Used `handleRetireError` and `outputRetireSuccess` helper functions to reduce cyclomatic complexity
- All validation commands pass: `magex format:fix`, `magex lint`, `magex test:race`
- Smoke tests verified: `--help` displays correct usage, nonexistent workspace shows proper error

### File List

- internal/cli/workspace_retire.go (NEW) - Retire subcommand implementation
- internal/cli/workspace_retire_test.go (NEW) - Comprehensive test suite
- internal/cli/workspace.go (MODIFIED) - Added `addWorkspaceRetireCmd(cmd)` call
- _bmad-output/implementation-artifacts/sprint-status.yaml (MODIFIED) - Updated story status to "review"

## Senior Developer Review (AI)

**Review Date:** 2025-12-28
**Reviewer:** Claude Opus 4.5 (Adversarial Code Review)
**Outcome:** APPROVED with fixes applied

### Issues Found & Fixed

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| H1 | HIGH | sprint-status.yaml modified but not in File List | ✅ Fixed |
| H2 | HIGH | Test didn't verify AC3 error message format | ✅ Fixed |
| M1 | MEDIUM | Logger injection inconsistent with destroy command | ✅ Fixed |
| M2 | MEDIUM | AC3 message format differs (enhanced with workspace name) | Accepted as-is (more informative) |
| L1 | LOW | No test for user canceling confirmation dialog | Noted (hard to test terminal interaction) |
| L2 | LOW | Low-probability code path for confirmation error | Noted (code is correct) |

### Fixes Applied

1. **H1:** Added `sprint-status.yaml (MODIFIED)` to File List
2. **H2:** Added message assertions to `TestRunWorkspaceRetire_WithRunningTasks`:
   - `assert.Contains(t, err.Error(), "cannot retire workspace")`
   - `assert.Contains(t, err.Error(), "running-ws")`
   - `assert.Contains(t, err.Error(), "running tasks")`
3. **M1:** Updated `executeRetire()` to take `logger zerolog.Logger` parameter, matching `executeDestroy()` pattern

### Validation

- `magex format:fix` ✅
- `magex lint` ✅ (0 issues)
- `magex test:race` ✅ (all tests pass)

### Files Modified in Review

- `internal/cli/workspace_retire.go` - Added zerolog import, updated executeRetire signature
- `internal/cli/workspace_retire_test.go` - Added AC3 message format assertions
- `_bmad-output/implementation-artifacts/3-6-implement-atlas-workspace-retire-command.md` - Added sprint-status.yaml to File List, added review notes

## Change Log

- 2025-12-28: **Code Review** - Fixed 3 issues (H1, H2, M1). Added logger parameter to executeRetire for consistency, enhanced test assertions for AC3 message format, documented sprint-status.yaml in File List.
- 2025-12-28: Implemented `atlas workspace retire` command with confirmation prompt, JSON output support, and comprehensive test coverage. All acceptance criteria satisfied.
