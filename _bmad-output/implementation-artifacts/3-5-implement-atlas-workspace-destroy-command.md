# Story 3.5: Implement `atlas workspace destroy` Command

Status: done

## Story

As a **user**,
I want **to run `atlas workspace destroy <name>` to fully clean up a workspace**,
So that **I can remove completed or abandoned work without leaving orphaned files**.

## Acceptance Criteria

1. **Given** a workspace exists **When** I run `atlas workspace destroy payment` **Then** the system prompts for confirmation: "Delete workspace 'payment'? This cannot be undone. [y/N]"

2. **Given** confirmation is received **When** destroy proceeds **Then** the system:
   - Removes the git worktree directory
   - Deletes the git branch (if not merged)
   - Removes `~/.atlas/workspaces/payment/` directory
   - Prunes any stale worktree references

3. **Given** destroy succeeds **When** operation completes **Then** displays success: "Workspace 'payment' destroyed"

4. **Given** I run `atlas workspace destroy payment --force` **When** the command executes **Then** confirmation prompt is skipped

5. **Given** workspace doesn't exist **When** I run `atlas workspace destroy nonexistent` **Then** displays clear error: "Workspace 'nonexistent' not found"

6. **Given** worktree is already gone **When** I run destroy **Then** continues with state cleanup without error

7. **Given** state is corrupted **When** I run destroy **Then** still removes what it can (NFR18 - always succeeds)

8. **Given** any state **When** destroy completes **Then** no orphaned directories or branches remain (NFR16, NFR17)

## Tasks / Subtasks

- [x] Task 1: Create destroy subcommand structure (AC: #1, #4, #5)
  - [x] 1.1: Create `internal/cli/workspace_destroy.go` with the `destroy` subcommand
  - [x] 1.2: Create `internal/cli/workspace_destroy_test.go` for tests
  - [x] 1.3: Add `addWorkspaceDestroyCmd(parent *cobra.Command)` function
  - [x] 1.4: Register destroy command in `workspace.go`

- [x] Task 2: Implement confirmation prompt (AC: #1, #4)
  - [x] 2.1: Create confirmation prompt using charmbracelet/huh
  - [x] 2.2: Add `--force` flag to skip confirmation
  - [x] 2.3: Handle non-interactive mode (error if no --force and not TTY)
  - [x] 2.4: Test confirmation flow with various inputs

- [x] Task 3: Implement destroy command logic (AC: #2, #3, #6, #7, #8)
  - [x] 3.1: Implement `runWorkspaceDestroy(ctx, cmd, name string) error`
  - [x] 3.2: Create WorktreeRunner from repo path (detect from cwd or config)
  - [x] 3.3: Create Manager with store and worktreeRunner
  - [x] 3.4: Call `manager.Destroy(ctx, name)` - already implements NFR18
  - [x] 3.5: Handle and display success message with checkmark icon
  - [x] 3.6: Handle workspace not found error with clear message

- [x] Task 4: Implement JSON output support (AC: #3)
  - [x] 4.1: Detect `--output json` flag from global flags
  - [x] 4.2: Output JSON result: `{"status": "destroyed", "workspace": "name"}`
  - [x] 4.3: On error with JSON output, output JSON error object

- [x] Task 5: Write comprehensive tests (AC: all)
  - [x] 5.1: Test destroy with confirmation (happy path)
  - [x] 5.2: Test destroy with --force flag (skip confirmation)
  - [x] 5.3: Test destroy with non-existent workspace (error)
  - [x] 5.4: Test destroy with corrupted state (still succeeds - NFR18)
  - [x] 5.5: Test destroy with --output json flag
  - [x] 5.6: Test destroy in non-interactive mode without --force (error)
  - [x] 5.7: Test context cancellation
  - [x] 5.8: Run `magex format:fix && magex lint && magex test:race`

## Dev Notes

### Critical Warnings (READ FIRST)

1. **Manager.Destroy() already implements NFR18** - It ALWAYS succeeds, collecting warnings but returning nil. The CLI just needs to call it and display success.
2. **MUST get worktreeRunner from actual repo** - Unlike `list` which passes nil, destroy NEEDS a real WorktreeRunner to remove worktrees and branches.
3. **MUST detect git repo path** - Either from cwd, config, or command flag.
4. **MUST handle non-interactive mode** - If not TTY and no --force, return clear error.
5. **MUST respect --output flag** - JSON vs text output modes.
6. **MUST use huh for confirmation** - Not fmt.Scanln or manual prompt.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/cli/workspace.go` | EXISTS - Add addWorkspaceDestroyCmd call |
| `internal/cli/workspace_destroy.go` | NEW - Destroy subcommand implementation |
| `internal/cli/workspace_destroy_test.go` | NEW - Tests for destroy command |
| `internal/workspace/manager.go` | EXISTS - Manager.Destroy() already implemented |
| `internal/workspace/worktree.go` | EXISTS - WorktreeRunner interface |
| `internal/tui/output.go` | EXISTS - Output interface for styled messages |

### Import Rules (CRITICAL)

**`internal/cli/workspace_destroy.go` MAY import:**
- `internal/workspace` - for Manager, NewManager, NewFileStore, NewWorktreeRunner
- `internal/tui` - for Output interface, Success/Error methods
- `internal/constants` - for any constants needed
- `internal/errors` - for error checking with errors.Is()
- `context`, `fmt`, `os`
- `github.com/spf13/cobra`
- `github.com/charmbracelet/huh` - for confirmation prompt
- `golang.org/x/term` - for TTY detection

**MUST NOT import:**
- `internal/task` - destroy doesn't need task engine
- `internal/ai` - no AI operations
- `internal/git` - use workspace.WorktreeRunner, not git package directly
- `internal/config` - may use if needed for repo path detection

### Confirmation Prompt Pattern (using huh)

```go
import "github.com/charmbracelet/huh"

func confirmDestroy(name string) (bool, error) {
    var confirm bool

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title(fmt.Sprintf("Delete workspace '%s'?", name)).
                Description("This cannot be undone.").
                Affirmative("Yes, delete").
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

### TTY Detection Pattern

```go
import "golang.org/x/term"

func isTerminal() bool {
    return term.IsTerminal(int(os.Stdin.Fd()))
}

// In command handler:
if !forceFlag && !isTerminal() {
    return fmt.Errorf("cannot destroy workspace: use --force in non-interactive mode")
}
```

### Command Structure Pattern (from workspace_list.go)

```go
// internal/cli/workspace_destroy.go
func addWorkspaceDestroyCmd(parent *cobra.Command) {
    var force bool

    cmd := &cobra.Command{
        Use:   "destroy <name>",
        Short: "Destroy a workspace and its worktree",
        Long: `Completely remove a workspace including its git worktree,
branch, and all associated state files.

This operation cannot be undone. Use --force to skip confirmation.

Examples:
  atlas workspace destroy payment           # Confirm and destroy
  atlas workspace destroy payment --force   # Destroy without confirmation`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return runWorkspaceDestroy(cmd.Context(), cmd, args[0], force)
        },
    }

    cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

    parent.AddCommand(cmd)
}
```

### Destroy Command Implementation Pattern

```go
func runWorkspaceDestroy(ctx context.Context, cmd *cobra.Command, name string, force bool) error {
    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    logger := GetLogger()
    output := cmd.Flag("output").Value.String()

    // Respect NO_COLOR
    tui.CheckNoColor()

    // Check if workspace exists first
    store, err := workspace.NewFileStore("")
    if err != nil {
        return fmt.Errorf("failed to create workspace store: %w", err)
    }

    exists, err := store.Exists(ctx, name)
    if err != nil {
        return fmt.Errorf("failed to check workspace: %w", err)
    }
    if !exists {
        if output == OutputJSON {
            return outputDestroyErrorJSON(os.Stdout, name, "workspace not found")
        }
        return fmt.Errorf("workspace '%s' not found", name)
    }

    // Handle confirmation
    if !force {
        if !isTerminal() {
            return fmt.Errorf("cannot destroy workspace '%s': use --force in non-interactive mode", name)
        }

        confirmed, err := confirmDestroy(name)
        if err != nil {
            return fmt.Errorf("failed to get confirmation: %w", err)
        }
        if !confirmed {
            fmt.Fprintln(os.Stdout, "Operation cancelled.")
            return nil
        }
    }

    // Get repo path for worktree runner
    repoPath, err := detectRepoPath()
    if err != nil {
        // If we can't detect repo, worktree operations will fail gracefully
        // Manager.Destroy() handles this via NFR18
        logger.Debug().Err(err).Msg("could not detect repo path, worktree cleanup may be limited")
        repoPath = ""
    }

    // Create worktree runner (may be nil if no repo path)
    var wtRunner workspace.WorktreeRunner
    if repoPath != "" {
        wtRunner = workspace.NewWorktreeRunner(repoPath)
    }

    // Create manager and destroy
    mgr := workspace.NewManager(store, wtRunner)

    if err := mgr.Destroy(ctx, name); err != nil {
        // This should never happen per NFR18, but handle just in case
        logger.Error().Err(err).Str("workspace", name).Msg("destroy failed")
        if output == OutputJSON {
            return outputDestroyErrorJSON(os.Stdout, name, err.Error())
        }
        return fmt.Errorf("failed to destroy workspace '%s': %w", name, err)
    }

    // Output success
    if output == OutputJSON {
        return outputDestroySuccessJSON(os.Stdout, name)
    }

    // Use tui for styled success message
    fmt.Fprintf(os.Stdout, "%s Workspace '%s' destroyed\n",
        lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓"),
        name)

    return nil
}

func detectRepoPath() (string, error) {
    // Try current working directory
    cwd, err := os.Getwd()
    if err != nil {
        return "", err
    }

    // Check if it's a git repo by looking for .git
    // Walk up directory tree
    dir := cwd
    for {
        gitPath := filepath.Join(dir, ".git")
        if _, err := os.Stat(gitPath); err == nil {
            return dir, nil
        }

        parent := filepath.Dir(dir)
        if parent == dir {
            // Reached root
            break
        }
        dir = parent
    }

    return "", fmt.Errorf("not in a git repository")
}
```

### JSON Output Format

**Success:**
```json
{
  "status": "destroyed",
  "workspace": "payment"
}
```

**Error:**
```json
{
  "status": "error",
  "workspace": "payment",
  "error": "workspace not found"
}
```

### Testing Pattern

```go
func TestRunWorkspaceDestroy_WithForce(t *testing.T) {
    // Create temp directory for test store
    tmpDir := t.TempDir()

    // Create test workspace
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    ws := &domain.Workspace{
        Name:         "test-ws",
        WorktreePath: "/tmp/test-worktree", // Fake path
        Branch:       "feat/test",
        Status:       constants.WorkspaceStatusActive,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }
    require.NoError(t, store.Create(context.Background(), ws))

    // Create mock worktree runner that expects destroy calls
    mockRunner := &MockWorktreeRunner{
        RemoveFunc: func(ctx context.Context, path string, force bool) error {
            return nil // Success
        },
        DeleteBranchFunc: func(ctx context.Context, branch string, force bool) error {
            return nil
        },
        PruneFunc: func(ctx context.Context) error {
            return nil
        },
    }

    // Create manager
    mgr := workspace.NewManager(store, mockRunner)

    // Destroy should succeed
    err = mgr.Destroy(context.Background(), "test-ws")
    require.NoError(t, err)

    // Verify workspace is gone
    exists, err := store.Exists(context.Background(), "test-ws")
    require.NoError(t, err)
    assert.False(t, exists)
}

func TestRunWorkspaceDestroy_NonInteractiveWithoutForce(t *testing.T) {
    // Test that destroy fails in non-interactive mode without --force
    // This requires mocking isTerminal() or using test flags
}

func TestRunWorkspaceDestroy_WorkspaceNotFound(t *testing.T) {
    tmpDir := t.TempDir()
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    // Don't create workspace, just try to destroy
    mgr := workspace.NewManager(store, nil)

    // Destroy should succeed per NFR18 even if workspace doesn't exist
    // But our CLI should check existence first and show clear error
    exists, err := store.Exists(context.Background(), "nonexistent")
    require.NoError(t, err)
    assert.False(t, exists)
}
```

### Previous Story Learnings (from Story 3-4)

1. **Context as first parameter** - Always check `ctx.Done()` at entry
2. **Action-first error messages** - `"failed to destroy workspace: %w"`
3. **Use constants package** - Never inline magic strings for status
4. **Use errors package** - Never define local sentinel errors
5. **Use tui package** - For RelativeTime, ColorOffset, CheckNoColor
6. **Run `magex test:race`** - Race detection is mandatory
7. **Test empty/error states** - Edge cases are important

### Manager.Destroy() Already Implemented

The `Manager.Destroy()` method in `internal/workspace/manager.go` already:
- Collects warnings but ALWAYS returns nil (NFR18)
- Tries to load workspace (handles corrupted state)
- Removes worktree if path known
- Deletes branch if known
- Prunes stale worktrees
- Deletes workspace state

The CLI command just needs to:
1. Check workspace exists (for user-friendly error message)
2. Get confirmation (or --force)
3. Create manager with real WorktreeRunner
4. Call manager.Destroy()
5. Display success/error message

### File Structure After This Story

```
internal/
├── cli/
│   ├── root.go              # EXISTS
│   ├── workspace.go         # MODIFY - add addWorkspaceDestroyCmd
│   ├── workspace_list.go    # EXISTS
│   ├── workspace_list_test.go # EXISTS
│   ├── workspace_destroy.go # NEW - destroy subcommand
│   └── workspace_destroy_test.go # NEW - destroy tests
└── workspace/
    ├── manager.go           # EXISTS - Manager.Destroy() already implemented
    ├── store.go             # EXISTS
    └── worktree.go          # EXISTS - WorktreeRunner interface
```

### Dependencies Between Stories

This story builds on:
- **Story 3-1** (Workspace Data Model and Store) - uses Store.Exists(), Store.Delete()
- **Story 3-2** (Git Worktree Operations) - uses WorktreeRunner.Remove(), DeleteBranch(), Prune()
- **Story 3-3** (Workspace Manager Service) - uses Manager.Destroy() (already implemented!)
- **Story 3-4** (Workspace List Command) - patterns for CLI commands

This story is required by:
- **Story 3-6** (`atlas workspace retire`) - similar command structure
- **Story 5-6** (Task Abandonment Flow) - may call workspace destroy

### Security Considerations

1. **Confirmation required by default** - Destructive operation needs explicit consent
2. **--force flag documented clearly** - User understands they're bypassing safety
3. **No secrets exposed** - Workspace paths are safe to display
4. **File permissions inherited** - From store (0o750/0o600)

### Performance Considerations

1. **NFR1: <1 second** - Destroy should be fast (file deletions + git commands)
2. **No network calls** - Pure local operations
3. **Graceful degradation** - If git repo not found, still cleans state

### Edge Cases to Handle

1. **Workspace doesn't exist** - Clear error message before confirmation
2. **Worktree already removed manually** - Continue with state cleanup
3. **Branch already deleted** - Continue without error
4. **Context cancelled during confirm** - Return ctx.Err()
5. **Non-TTY without --force** - Error with clear instructions
6. **Corrupted workspace.json** - Manager.Destroy() handles this (NFR18)

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.5]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure & Boundaries]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/workspace/manager.go - Manager.Destroy() implementation]
- [Source: internal/cli/workspace_list.go - CLI command patterns]
- [Source: internal/cli/workspace.go - Parent command structure]
- [Source: _bmad-output/implementation-artifacts/3-4-implement-atlas-workspace-list-command.md - Previous story patterns]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test:
go run ./cmd/atlas workspace destroy nonexistent        # Should show "not found" error
go run ./cmd/atlas workspace destroy --help             # Should show help with --force flag

# Integration test (requires actual workspace):
# atlas workspace list                                  # Verify workspace exists
# atlas workspace destroy <name>                        # Test confirmation prompt
# atlas workspace destroy <name> --force                # Test force flag
# atlas workspace destroy <name> --output json --force  # Test JSON output
```

## Dev Agent Record

### Agent Model Used

claude-opus-4-5-20251101

### Debug Log References

N/A

### Completion Notes List

1. **Implementation Complete**: All 5 tasks and their subtasks completed successfully
2. **All Tests Pass**: 18 tests covering destroy command functionality
3. **Validation Passed**:
   - `magex format:fix` - Code formatted
   - `magex lint` - No lint errors
   - `magex test:race` - All tests pass with race detection
   - `go build ./...` - Successful compilation
4. **NFR18 Compliance**: Destroy always succeeds by cleaning up what it can, even with corrupted state
5. **Added sentinel errors**: `ErrNonInteractiveMode` and `ErrJSONErrorOutput` to internal/errors/errors.go

### Change Log

| Date | Change | Files |
|------|--------|-------|
| 2025-12-28 | Create workspace destroy command | internal/cli/workspace_destroy.go |
| 2025-12-28 | Create comprehensive test suite | internal/cli/workspace_destroy_test.go |
| 2025-12-28 | Register destroy in workspace cmd | internal/cli/workspace.go |
| 2025-12-28 | Add ErrNonInteractiveMode error | internal/errors/errors.go |
| 2025-12-28 | Code review fixes: AC5 error format, JSON exit codes, tests | internal/cli/workspace_destroy.go, internal/cli/workspace_destroy_test.go, internal/errors/errors.go |

### Senior Developer Review (AI)

**Reviewer:** claude-opus-4-5-20251101
**Date:** 2025-12-28
**Outcome:** Approved with fixes applied

**Issues Found & Fixed:**
- **H1**: AC5 error message format - Fixed to match "Workspace 'name' not found" format
- **H2**: Missing non-interactive mode test - Added `TestRunWorkspaceDestroy_NonInteractiveWithoutForce` and JSON variant
- **H3**: JSON errors returning exit code 0 - Added `ErrJSONErrorOutput` sentinel, now returns non-zero exit code
- **M2**: Duplicate context cancellation check - Removed redundant check from `runWorkspaceDestroyWithOutput`
- **M4**: TestIsTerminal no-op - Made useful with proper assertion
- **M5**: Added injectable `terminalCheck` for test control

**Verification:**
- `magex lint` ✓
- `magex test:race` ✓ (18 tests pass)
- `go build ./...` ✓

### File List

**New Files:**
- `internal/cli/workspace_destroy.go` - Destroy subcommand implementation
- `internal/cli/workspace_destroy_test.go` - Comprehensive test suite (18 tests)

**Modified Files:**
- `internal/cli/workspace.go` - Added `addWorkspaceDestroyCmd(cmd)` call
- `internal/errors/errors.go` - Added `ErrNonInteractiveMode` and `ErrJSONErrorOutput` sentinel errors
