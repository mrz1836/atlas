# Story 3.3: Workspace Manager Service

Status: done

## Story

As a **developer**,
I want **a WorkspaceManager service that orchestrates workspace lifecycle**,
So that **workspace operations are coordinated between state and git worktrees**.

## Acceptance Criteria

1. **Given** the store and worktree operations exist **When** I implement `internal/workspace/manager.go` **Then** the manager provides `Create(ctx, name, repoPath, branchType) (*Workspace, error)` - creates workspace + worktree atomically

2. **Given** a workspace exists **When** I call `Get(ctx, name)` **Then** it retrieves the workspace by name

3. **Given** workspaces exist **When** I call `List(ctx)` **Then** it returns all workspaces

4. **Given** a workspace exists **When** I call `Destroy(ctx, name)` **Then** it removes workspace state AND worktree, and deletes the associated branch

5. **Given** a workspace exists **When** I call `Retire(ctx, name)` **Then** it archives state (updates status to retired), removes worktree but keeps the branch

6. **Given** a workspace exists **When** I call `UpdateStatus(ctx, name, status)` **Then** it updates the workspace status

7. **Given** a workspace name **When** I call Create **Then** it validates name uniqueness before creating

8. **Given** workspace state is corrupted **When** I call Destroy **Then** it still removes what it can without error (NFR18 - destroy always succeeds)

9. **Given** a workspace had a worktree **When** I call Destroy **Then** it cleans up orphaned branches

10. **Given** any workspace operation **When** the operation executes **Then** it uses context for cancellation

11. **Given** workspace operations occur **When** operations complete **Then** they are logged with workspace context

12. **Given** multiple workspaces need management **When** I use the Manager **Then** it supports 3+ concurrent workspaces (FR20)

## Tasks / Subtasks

- [x] Task 1: Create Manager interface and struct (AC: #1, #2, #3, #4, #5, #6)
  - [x] 1.1: Create `internal/workspace/manager.go` with package declaration
  - [x] 1.2: Create `internal/workspace/manager_test.go` for tests
  - [x] 1.3: Define `Manager` interface with all operations
  - [x] 1.4: Define `DefaultManager` struct implementing the interface
  - [x] 1.5: Implement `NewManager(store Store, worktreeRunner WorktreeRunner) *DefaultManager`

- [x] Task 2: Implement Create operation (AC: #1, #7, #10, #11)
  - [x] 2.1: Implement `Create(ctx context.Context, name string, repoPath string, branchType string) (*domain.Workspace, error)`
  - [x] 2.2: Validate name is not empty and doesn't already exist
  - [x] 2.3: Call worktreeRunner.Create() to create worktree and branch
  - [x] 2.4: Build domain.Workspace with all fields populated
  - [x] 2.5: Call store.Create() to persist workspace
  - [x] 2.6: If store.Create fails, rollback worktree (atomic operation)
  - [x] 2.7: Return the complete Workspace object

- [x] Task 3: Implement Get operation (AC: #2, #10)
  - [x] 3.1: Implement `Get(ctx context.Context, name string) (*domain.Workspace, error)`
  - [x] 3.2: Delegate to store.Get() and return result
  - [x] 3.3: Check context cancellation at entry

- [x] Task 4: Implement List operation (AC: #3, #10, #12)
  - [x] 4.1: Implement `List(ctx context.Context) ([]*domain.Workspace, error)`
  - [x] 4.2: Delegate to store.List() and return result
  - [x] 4.3: Check context cancellation at entry

- [x] Task 5: Implement Destroy operation (AC: #4, #8, #9, #10, #11)
  - [x] 5.1: Implement `Destroy(ctx context.Context, name string) error`
  - [x] 5.2: Try to load workspace from store (continue even if corrupted)
  - [x] 5.3: Try to remove worktree via worktreeRunner.Remove(force=true)
  - [x] 5.4: Try to delete the branch via worktreeRunner.DeleteBranch(force=true)
  - [x] 5.5: Call worktreeRunner.Prune() to clean up stale entries
  - [x] 5.6: Delete workspace state via store.Delete()
  - [x] 5.7: CRITICAL: Always succeed even if some operations fail (NFR18)
  - [x] 5.8: Log all operation results and continue on partial failures

- [x] Task 6: Implement Retire operation (AC: #5, #10, #11)
  - [x] 6.1: Implement `Retire(ctx context.Context, name string) error`
  - [x] 6.2: Load workspace via store.Get()
  - [x] 6.3: Check if workspace has running tasks (return error if so)
  - [x] 6.4: Remove worktree via worktreeRunner.Remove() but keep branch
  - [x] 6.5: Update workspace status to constants.WorkspaceStatusRetired
  - [x] 6.6: Clear WorktreePath field (worktree no longer exists)
  - [x] 6.7: Persist via store.Update()

- [x] Task 7: Implement UpdateStatus operation (AC: #6, #10)
  - [x] 7.1: Implement `UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error`
  - [x] 7.2: Load workspace via store.Get()
  - [x] 7.3: Update Status field
  - [x] 7.4: Persist via store.Update()

- [x] Task 8: Implement Exists operation (AC: #7)
  - [x] 8.1: Implement `Exists(ctx context.Context, name string) (bool, error)`
  - [x] 8.2: Delegate to store.Exists() and return result

- [x] Task 9: Write comprehensive tests (AC: all)
  - [x] 9.1: Test Create with new workspace (happy path)
  - [x] 9.2: Test Create validates name uniqueness
  - [x] 9.3: Test Create rolls back worktree on store failure
  - [x] 9.4: Test Get with existing workspace
  - [x] 9.5: Test Get with non-existent workspace
  - [x] 9.6: Test List with multiple workspaces
  - [x] 9.7: Test List with empty store
  - [x] 9.8: Test Destroy with clean workspace
  - [x] 9.9: Test Destroy with corrupted state (still succeeds - NFR18)
  - [x] 9.10: Test Destroy cleans up branches
  - [x] 9.11: Test Retire with clean workspace
  - [x] 9.12: Test Retire with running tasks (returns error)
  - [x] 9.13: Test UpdateStatus
  - [x] 9.14: Test context cancellation in all operations
  - [x] 9.15: Run `magex format:fix && magex lint && magex test:race`

## Dev Notes

### Critical Warnings (READ FIRST)

1. **MUST be atomic** - Create operation either fully succeeds or fully rolls back
2. **MUST handle partial failures in Destroy** - NFR18 requires destroy to ALWAYS succeed
3. **MUST import from constants** - Use `constants.WorkspaceStatus*` values, never inline strings
4. **MUST import from errors** - Use sentinel errors from centralized errors package
5. **MUST check context cancellation** - All methods check `ctx.Done()` at entry

### Package Locations

| File | Purpose |
|------|---------|
| `internal/workspace/manager.go` | NEW - Manager interface and DefaultManager implementation |
| `internal/workspace/manager_test.go` | NEW - Comprehensive tests with mocks |
| `internal/workspace/store.go` | EXISTS - Store interface (use as dependency) |
| `internal/workspace/worktree.go` | EXISTS - WorktreeRunner interface (use as dependency) |
| `internal/domain/workspace.go` | EXISTS - Workspace type (use as-is) |
| `internal/constants/status.go` | EXISTS - WorkspaceStatus constants (use as-is) |

### Import Rules (CRITICAL)

**`internal/workspace/manager.go` MAY import:**
- `internal/constants` - for status values
- `internal/errors` - for sentinel errors
- `internal/domain` - for Workspace type
- `context`, `fmt`, `time`

**`internal/workspace/manager.go` MUST NOT import:**
- `internal/cli` - no CLI dependencies
- `internal/config` - manager doesn't need config
- `internal/task` - workspace doesn't depend on task
- `os`, `path/filepath` - use Store/WorktreeRunner instead

### Manager Interface Design

```go
// Manager orchestrates workspace lifecycle operations.
type Manager interface {
    // Create creates a new workspace with a git worktree.
    // Returns ErrWorkspaceExists if workspace already exists.
    Create(ctx context.Context, name string, repoPath string, branchType string) (*domain.Workspace, error)

    // Get retrieves a workspace by name.
    // Returns ErrWorkspaceNotFound if not found.
    Get(ctx context.Context, name string) (*domain.Workspace, error)

    // List returns all workspaces.
    // Returns empty slice if none exist.
    List(ctx context.Context) ([]*domain.Workspace, error)

    // Destroy removes a workspace and its worktree.
    // ALWAYS succeeds even if state is corrupted (NFR18).
    Destroy(ctx context.Context, name string) error

    // Retire archives a workspace, removing worktree but keeping state.
    // Returns error if tasks are running.
    Retire(ctx context.Context, name string) error

    // UpdateStatus updates the status of a workspace.
    UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error

    // Exists returns true if a workspace exists.
    Exists(ctx context.Context, name string) (bool, error)
}
```

### DefaultManager Implementation Pattern

```go
// DefaultManager implements Manager using Store and WorktreeRunner.
type DefaultManager struct {
    store          Store
    worktreeRunner WorktreeRunner
}

// NewManager creates a new DefaultManager.
func NewManager(store Store, worktreeRunner WorktreeRunner) *DefaultManager {
    return &DefaultManager{
        store:          store,
        worktreeRunner: worktreeRunner,
    }
}
```

### Create Operation Pattern (ATOMIC)

```go
func (m *DefaultManager) Create(ctx context.Context, name string, repoPath string, branchType string) (*domain.Workspace, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Check if workspace already exists
    exists, err := m.store.Exists(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("failed to check workspace existence: %w", err)
    }
    if exists {
        return nil, fmt.Errorf("failed to create workspace '%s': %w", name, atlaserrors.ErrWorkspaceExists)
    }

    // Create worktree
    wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
        RepoPath:      repoPath,
        WorkspaceName: name,
        BranchType:    branchType,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create worktree: %w", err)
    }

    // Build workspace
    now := time.Now()
    ws := &domain.Workspace{
        Name:         name,
        WorktreePath: wtInfo.Path,
        Branch:       wtInfo.Branch,
        Status:       constants.WorkspaceStatusActive,
        Tasks:        []domain.TaskRef{},
        CreatedAt:    now,
        UpdatedAt:    now,
    }

    // Persist to store
    if err := m.store.Create(ctx, ws); err != nil {
        // CRITICAL: Rollback worktree on store failure
        _ = m.worktreeRunner.Remove(ctx, wtInfo.Path, true)
        return nil, fmt.Errorf("failed to persist workspace: %w", err)
    }

    return ws, nil
}
```

### Destroy Operation Pattern (ALWAYS SUCCEEDS - NFR18)

```go
func (m *DefaultManager) Destroy(ctx context.Context, name string) error {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    var errs []error

    // Try to load workspace (may be corrupted)
    ws, err := m.store.Get(ctx, name)
    if err != nil && !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
        // Log warning but continue - state might be corrupted
        errs = append(errs, fmt.Errorf("warning: failed to load workspace: %w", err))
    }

    // Try to remove worktree if we know the path
    if ws != nil && ws.WorktreePath != "" {
        if err := m.worktreeRunner.Remove(ctx, ws.WorktreePath, true); err != nil {
            // Log warning but continue
            errs = append(errs, fmt.Errorf("warning: failed to remove worktree: %w", err))
        }
    }

    // Try to delete branch if we know it
    if ws != nil && ws.Branch != "" {
        if err := m.worktreeRunner.DeleteBranch(ctx, ws.Branch, true); err != nil {
            // Log warning but continue - branch might already be deleted
            errs = append(errs, fmt.Errorf("warning: failed to delete branch: %w", err))
        }
    }

    // Prune stale worktrees
    if err := m.worktreeRunner.Prune(ctx); err != nil {
        errs = append(errs, fmt.Errorf("warning: failed to prune worktrees: %w", err))
    }

    // Delete workspace state
    if err := m.store.Delete(ctx, name); err != nil {
        if !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
            errs = append(errs, fmt.Errorf("warning: failed to delete workspace state: %w", err))
        }
    }

    // NFR18: ALWAYS succeed - log warnings but don't return error
    // In production, these would be logged via zerolog
    _ = errs // Warnings collected for logging

    return nil
}
```

### Retire Operation Pattern

```go
func (m *DefaultManager) Retire(ctx context.Context, name string) error {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Load workspace
    ws, err := m.store.Get(ctx, name)
    if err != nil {
        return fmt.Errorf("failed to retire workspace '%s': %w", name, err)
    }

    // Check for running tasks
    for _, task := range ws.Tasks {
        if task.Status == constants.TaskStatusRunning ||
           task.Status == constants.TaskStatusValidating {
            return fmt.Errorf("cannot retire workspace '%s': task '%s' is still running", name, task.ID)
        }
    }

    // Remove worktree (but keep branch!)
    if ws.WorktreePath != "" {
        if err := m.worktreeRunner.Remove(ctx, ws.WorktreePath, false); err != nil {
            // If worktree is dirty, force remove
            if err := m.worktreeRunner.Remove(ctx, ws.WorktreePath, true); err != nil {
                return fmt.Errorf("failed to remove worktree: %w", err)
            }
        }
    }

    // Update status
    ws.Status = constants.WorkspaceStatusRetired
    ws.WorktreePath = "" // Worktree no longer exists

    // Persist
    if err := m.store.Update(ctx, ws); err != nil {
        return fmt.Errorf("failed to update workspace status: %w", err)
    }

    return nil
}
```

### Context Usage Pattern (from Story 3-1 and 3-2)

```go
func (m *DefaultManager) Get(ctx context.Context, name string) (*domain.Workspace, error) {
    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    return m.store.Get(ctx, name)
}
```

### Error Handling Pattern

```go
// Action-first error messages
return fmt.Errorf("failed to create workspace '%s': %w", name, err)
return fmt.Errorf("failed to retire workspace '%s': %w", name, err)
return fmt.Errorf("failed to destroy workspace '%s': %w", name, err)

// Use sentinel errors from internal/errors
return fmt.Errorf("failed to create workspace '%s': %w", name, atlaserrors.ErrWorkspaceExists)
```

### Testing Pattern (Mock Dependencies)

```go
// MockStore implements Store for testing
type MockStore struct {
    workspaces map[string]*domain.Workspace
    createErr  error
    getErr     error
    updateErr  error
    deleteErr  error
}

// MockWorktreeRunner implements WorktreeRunner for testing
type MockWorktreeRunner struct {
    createResult *WorktreeInfo
    createErr    error
    removeErr    error
    pruneErr     error
    deleteBranchErr error
}

func TestDefaultManager_Create_Success(t *testing.T) {
    store := &MockStore{workspaces: make(map[string]*domain.Workspace)}
    runner := &MockWorktreeRunner{
        createResult: &WorktreeInfo{
            Path:   "/tmp/repo-test",
            Branch: "feat/test",
        },
    }

    mgr := NewManager(store, runner)
    ws, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat")

    require.NoError(t, err)
    assert.Equal(t, "test", ws.Name)
    assert.Equal(t, "/tmp/repo-test", ws.WorktreePath)
    assert.Equal(t, "feat/test", ws.Branch)
    assert.Equal(t, constants.WorkspaceStatusActive, ws.Status)
}

func TestDefaultManager_Destroy_SucceedsEvenIfCorrupted(t *testing.T) {
    // NFR18: Destroy ALWAYS succeeds
    store := &MockStore{
        getErr: atlaserrors.ErrWorkspaceCorrupted,
    }
    runner := &MockWorktreeRunner{}

    mgr := NewManager(store, runner)
    err := mgr.Destroy(context.Background(), "corrupted")

    // MUST succeed even with corrupted state
    assert.NoError(t, err)
}

func TestDefaultManager_Create_RollsBackOnStoreFailure(t *testing.T) {
    removeCallCount := 0
    store := &MockStore{
        createErr: errors.New("store failure"),
    }
    runner := &MockWorktreeRunner{
        createResult: &WorktreeInfo{Path: "/tmp/test", Branch: "feat/test"},
    }

    // Track Remove calls
    originalRemove := runner.Remove
    runner.Remove = func(ctx context.Context, path string, force bool) error {
        removeCallCount++
        return nil
    }

    mgr := NewManager(store, runner)
    _, err := mgr.Create(context.Background(), "test", "/tmp/repo", "feat")

    assert.Error(t, err)
    assert.Equal(t, 1, removeCallCount) // Verify rollback happened
}
```

### Previous Story Learnings (from Story 3-1 and 3-2)

1. **Context parameter in all methods** - Always check `ctx.Done()` at entry
2. **Action-first error messages** - `"failed to create workspace: %w"`
3. **Atomic operations** - Clean up on any failure
4. **Comprehensive tests** - Cover happy path, error cases, edge cases
5. **Use constants package** - Never inline magic strings
6. **Use errors package** - Never define local sentinel errors

### Epic 2 Retro Learnings

1. **Run `magex test:race`** - Race detection is mandatory
2. **Test early, test manually** - Build and run actual commands
3. **Integration tests required** - Manager orchestration needs thorough testing
4. **Smoke test validation** - Verify end-to-end flows work

### File Structure After This Story

```
internal/
├── workspace/
│   ├── store.go           # EXISTS - Workspace persistence (Story 3-1)
│   ├── store_test.go      # EXISTS - Store tests
│   ├── worktree.go        # EXISTS - WorktreeRunner (Story 3-2)
│   ├── worktree_test.go   # EXISTS - Worktree tests
│   ├── manager.go         # NEW - Manager interface + DefaultManager
│   └── manager_test.go    # NEW - Manager tests with mocks
├── errors/
│   └── errors.go          # EXISTS - Use workspace sentinel errors
├── domain/
│   └── workspace.go       # EXISTS - Workspace type
└── constants/
    └── status.go          # EXISTS - WorkspaceStatus constants
```

### Dependencies Between Stories

This story builds on:
- **Story 3-1** (Workspace Data Model and Store) - uses Store interface
- **Story 3-2** (Git Worktree Operations) - uses WorktreeRunner interface

This story is required by:
- **Story 3-4** (atlas workspace list) - will use Manager.List()
- **Story 3-5** (atlas workspace destroy) - will use Manager.Destroy()
- **Story 3-6** (atlas workspace retire) - will use Manager.Retire()
- **Story 3-7** (atlas workspace logs) - will use Manager.Get()
- **Story 4-7** (atlas start) - will use Manager.Create() for task workspaces

### WorkspaceStatus Constants (from internal/constants/status.go)

```go
type WorkspaceStatus string

const (
    WorkspaceStatusActive  WorkspaceStatus = "active"
    WorkspaceStatusPaused  WorkspaceStatus = "paused"
    WorkspaceStatusRetired WorkspaceStatus = "retired"
)
```

### TaskStatus Constants (for Retire validation)

```go
type TaskStatus string

const (
    TaskStatusPending          TaskStatus = "pending"
    TaskStatusRunning          TaskStatus = "running"
    TaskStatusValidating       TaskStatus = "validating"
    TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"
    TaskStatusCompleted        TaskStatus = "completed"
    // ... other statuses
)
```

### Security Considerations

1. **Name validation** - Manager should reject names that could cause path traversal (handled by Store)
2. **No secrets** - Manager never handles credentials or API keys
3. **File permissions** - Delegated to Store (uses secure 0o750/0o600)

### Performance Considerations

1. **Lazy loading** - List doesn't need to load worktree info, just state
2. **Context timeouts** - All operations respect context for cancellation
3. **Destroy is resilient** - Continues on errors, doesn't block on failures

### Edge Cases to Handle

1. **Empty workspace name** - Validate and return error
2. **Workspace already exists** - Return ErrWorkspaceExists
3. **Worktree creation fails** - Don't create workspace state
4. **Store creation fails** - Rollback worktree creation
5. **Corrupted workspace state** - Destroy still succeeds (NFR18)
6. **Worktree already deleted** - Destroy continues without error
7. **Branch already deleted** - Destroy continues without error
8. **Running tasks on retire** - Return error, don't allow retire
9. **Context cancelled mid-operation** - Return ctx.Err() cleanly

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.3]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Tool Integration]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/workspace/store.go - Store interface patterns]
- [Source: internal/workspace/worktree.go - WorktreeRunner interface patterns]
- [Source: _bmad-output/implementation-artifacts/3-1-workspace-data-model-and-store.md - Previous story patterns]
- [Source: _bmad-output/implementation-artifacts/3-2-git-worktree-operations.md - Previous story patterns]
- [Source: _bmad-output/implementation-artifacts/epic-2-retro-2025-12-28.md - Epic 2 learnings]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test (manager tests with mocks):
go test -v -run TestDefaultManager ./internal/workspace/

# Integration test (optional but recommended):
# Create a test that uses real Store and WorktreeRunner in temp directories
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No debug issues encountered

### Completion Notes List

- Implemented Manager interface with 7 operations: Create, Get, List, Destroy, Retire, UpdateStatus, Exists
- Create operation is atomic: worktree is rolled back if store persistence fails
- Destroy operation always succeeds (NFR18): collects warnings but never returns error
- Retire operation validates no running tasks before archiving
- All operations check context cancellation at entry
- Added ErrWorkspaceHasRunningTasks sentinel error to internal/errors
- Comprehensive test coverage: 38 tests covering all operations, error cases, and edge cases
- All tests pass with race detection enabled
- Code passes golangci-lint with 62 enabled linters

### Code Review (2025-12-28)

**Reviewer:** Claude Opus 4.5 (Adversarial Code Review)

**Issues Found:** 4 HIGH, 6 MEDIUM, 4 LOW

**Issues Fixed:**
- H1: Retire now updates store FIRST (before worktree removal) for data consistency
- H2: Manager consistently sets UpdatedAt before store operations
- H3: Added test for Retire with store.Update failure
- H4: Added test for UpdateStatus with store.Update failure
- M1: Retire preserves error context from both remove attempts
- M2: Fixed test to use store-related error (ErrLockTimeout) instead of ErrGitOperation
- M3: Clarified misleading comment about logging warnings
- M4: Added input validation for repoPath and branchType in Create
- M5: Added test for Exists with store error
- M6: Clarified concurrent test documentation

**Remaining LOW Issues (acceptable):**
- L1: forceRemoveMockRunner shadows removeCallCount (working, just confusing)
- L2: No integration test with real Store/WorktreeRunner (optional per story)
- L3: sprint-status.yaml not in File List (expected for story updates)
- L4: UpdateStatus local ws object has stale UpdatedAt (caller should reload)

### Change Log

- 2025-12-28: Story 3.3 implementation complete - all tasks and subtasks finished
- Created Manager interface and DefaultManager implementation
- Added ErrWorkspaceHasRunningTasks sentinel error
- Created comprehensive test suite with mock dependencies
- 2025-12-28: Code review completed - fixed 10 issues (4 HIGH, 6 MEDIUM)
- Improved Retire operation order (store update before worktree removal)
- Added input validation for Create parameters
- Added 5 new tests for error handling edge cases
- MockStore.Get now returns copy (matches real FileStore behavior)

### File List

- internal/workspace/manager.go (NEW)
- internal/workspace/manager_test.go (NEW)
- internal/errors/errors.go (MODIFIED - added ErrWorkspaceHasRunningTasks)
