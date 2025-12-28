# Story 3.1: Workspace Data Model and Store

Status: done

## Story

As a **developer**,
I want **a workspace persistence layer with atomic operations**,
So that **workspace state is reliably saved and can survive crashes**.

## Acceptance Criteria

1. **Given** the domain types exist **When** I implement `internal/workspace/store.go` **Then** the store can create workspace JSON files at `~/.atlas/workspaces/<name>/workspace.json`

2. **Given** a workspace directory exists **When** reading workspace state **Then** proper error handling returns clear errors for missing files, corrupted JSON, and permission issues

3. **Given** workspace state needs updating **When** the store updates workspace state **Then** it uses atomic write (write-then-rename) to prevent partial writes

4. **Given** multiple workspaces exist **When** the store lists all workspaces **Then** it scans the workspaces directory and returns all valid workspace objects

5. **Given** a workspace exists **When** the store deletes workspace state files **Then** the entire workspace directory is removed cleanly

6. **Given** concurrent access to workspace files **When** file operations occur **Then** flock (POSIX file locking) protects against data corruption

7. **Given** workspace state is serialized **When** saving to JSON **Then** all JSON fields use snake_case per Architecture requirement (e.g., `worktree_path`, `created_at`)

8. **Given** any workspace state file **When** reading or writing **Then** `schema_version` is included for future migrations

9. **Given** a corrupted JSON file exists **When** the store attempts to read **Then** it returns a clear, actionable error message

10. **Given** a workspace with atomic write behavior **When** a write operation fails mid-way **Then** no partial writes occur (verified by tests)

## Tasks / Subtasks

- [x] Task 1: Create workspace package structure (AC: #1, #7, #8)
  - [x] 1.1: Create `internal/workspace/store.go` with package declaration and imports
  - [x] 1.2: Create `internal/workspace/store_test.go` for co-located tests
  - [x] 1.3: Define `Store` interface with all CRUD operations
  - [x] 1.4: Define `FileStore` struct implementing the Store interface

- [x] Task 2: Implement workspace path helpers (AC: #1)
  - [x] 2.1: Create `atlasHomeDir()` - returns `~/.atlas` (use `os.UserHomeDir()`)
  - [x] 2.2: Create `workspacesDir()` - returns `~/.atlas/workspaces`
  - [x] 2.3: Create `workspacePath(name string)` - returns `~/.atlas/workspaces/<name>/`
  - [x] 2.4: Create `workspaceFilePath(name string)` - returns full path to workspace.json
  - [x] 2.5: Use constants from `internal/constants` for directory names (MUST NOT inline strings)

- [x] Task 3: Implement Create operation (AC: #1, #6, #7, #8)
  - [x] 3.1: Implement `Create(ctx context.Context, ws *domain.Workspace) error`
  - [x] 3.2: Validate workspace name is not empty and contains valid characters
  - [x] 3.3: Check if workspace already exists (return `ErrWorkspaceExists`)
  - [x] 3.4: Create workspace directory with `os.MkdirAll(path, 0755)`
  - [x] 3.5: Set `schema_version` to current version (1) before saving
  - [x] 3.6: Use atomic write for initial creation
  - [x] 3.7: Apply flock during write operation

- [x] Task 4: Implement Read operation (AC: #2, #8, #9)
  - [x] 4.1: Implement `Get(ctx context.Context, name string) (*domain.Workspace, error)`
  - [x] 4.2: Check if workspace directory exists (return `ErrWorkspaceNotFound`)
  - [x] 4.3: Read workspace.json with flock protection
  - [x] 4.4: Parse JSON with proper error handling
  - [x] 4.5: Handle corrupted JSON with clear error: "workspace '<name>' has corrupted state file: <details>"
  - [x] 4.6: Validate schema_version for potential migrations (log warning if newer than supported)

- [x] Task 5: Implement Update operation (AC: #3, #6, #7)
  - [x] 5.1: Implement `Update(ctx context.Context, ws *domain.Workspace) error`
  - [x] 5.2: Verify workspace exists before updating (return `ErrWorkspaceNotFound`)
  - [x] 5.3: Update `UpdatedAt` timestamp before saving
  - [x] 5.4: Use atomic write-then-rename pattern:
    - Write to `workspace.json.tmp`
    - Sync to disk (`file.Sync()`)
    - Rename to `workspace.json`
  - [x] 5.5: Apply flock during entire write operation
  - [x] 5.6: Clean up temp file on any error

- [x] Task 6: Implement List operation (AC: #4)
  - [x] 6.1: Implement `List(ctx context.Context) ([]*domain.Workspace, error)`
  - [x] 6.2: Read workspaces directory entries with `os.ReadDir`
  - [x] 6.3: Filter to directories only (skip files)
  - [x] 6.4: For each directory, attempt to read workspace.json
  - [x] 6.5: Skip directories without valid workspace.json (log warning)
  - [x] 6.6: Return all successfully loaded workspaces
  - [x] 6.7: Return empty slice (not nil) when no workspaces exist

- [x] Task 7: Implement Delete operation (AC: #5)
  - [x] 7.1: Implement `Delete(ctx context.Context, name string) error`
  - [x] 7.2: Check if workspace exists (return `ErrWorkspaceNotFound`)
  - [x] 7.3: Use `os.RemoveAll` to delete entire workspace directory
  - [x] 7.4: Handle partial deletion gracefully (log warning, continue)

- [x] Task 8: Implement file locking (AC: #6)
  - [x] 8.1: Create file locking utilities in store.go (integrated, not separate file)
  - [x] 8.2: Implement `acquireLock(path string) (*os.File, error)` using `syscall.Flock`
  - [x] 8.3: Implement `releaseLock(f *os.File) error`
  - [x] 8.4: Create lock helper integrated into store methods
  - [x] 8.5: Handle lock timeout (don't block forever) - 5 second timeout with non-blocking retry
  - [x] 8.6: Ensure lock file cleanup on context cancellation

- [x] Task 9: Implement atomic write helper (AC: #3, #10)
  - [x] 9.1: Create `atomicWrite(path string, data []byte) error` function
  - [x] 9.2: Write to temporary file with `.tmp` suffix
  - [x] 9.3: Call `file.Sync()` to ensure data is flushed to disk
  - [x] 9.4: Use `os.Rename` to atomically replace target file
  - [x] 9.5: Clean up temp file on any error path
  - [x] 9.6: Preserve file permissions on rename

- [x] Task 10: Add sentinel errors (AC: #2, #9)
  - [x] 10.1: Add `ErrWorkspaceExists` to `internal/errors/errors.go`
  - [x] 10.2: Add `ErrWorkspaceNotFound` to `internal/errors/errors.go`
  - [x] 10.3: Add `ErrWorkspaceCorrupted` to `internal/errors/errors.go`
  - [x] 10.4: Add `ErrLockTimeout` to `internal/errors/errors.go`
  - [x] 10.5: Use action-first error wrapping: `fmt.Errorf("failed to create workspace: %w", err)`

- [x] Task 11: Write comprehensive tests (AC: all)
  - [x] 11.1: Test Create with new workspace (happy path)
  - [x] 11.2: Test Create with existing workspace (returns ErrWorkspaceExists)
  - [x] 11.3: Test Create validates name (empty name, invalid characters)
  - [x] 11.4: Test Get with existing workspace (happy path)
  - [x] 11.5: Test Get with non-existent workspace (returns ErrWorkspaceNotFound)
  - [x] 11.6: Test Get with corrupted JSON (returns ErrWorkspaceCorrupted)
  - [x] 11.7: Test Update with existing workspace (happy path)
  - [x] 11.8: Test Update with non-existent workspace (returns ErrWorkspaceNotFound)
  - [x] 11.9: Test List with multiple workspaces
  - [x] 11.10: Test List with empty directory (returns empty slice)
  - [x] 11.11: Test List with mixed valid/invalid workspace directories
  - [x] 11.12: Test Delete with existing workspace
  - [x] 11.13: Test Delete with non-existent workspace
  - [x] 11.14: Test atomic write behavior (simulate failure mid-write)
  - [x] 11.15: Test JSON field naming (verify snake_case)
  - [x] 11.16: Test schema_version is set correctly
  - [x] 11.17: Run `magex format:fix && magex lint && magex test:race`

## Dev Notes

### Critical Warnings (READ FIRST)

1. **MUST use atomic writes** - Never write directly to workspace.json; always use write-then-rename
2. **MUST use flock** - Concurrent access without locking causes data corruption
3. **MUST use snake_case** - All JSON fields must be snake_case per Architecture
4. **MUST import from constants** - Never inline magic strings for paths/filenames
5. **MUST import from errors** - Add sentinel errors to centralized errors package, not locally

### Package Locations

| File | Purpose |
|------|---------|
| `internal/workspace/store.go` | NEW - Store interface, FileStore implementation, and integrated file locking |
| `internal/workspace/store_test.go` | NEW - Comprehensive tests for store operations |
| `internal/errors/errors.go` | MODIFY - Add workspace-related sentinel errors |
| `internal/domain/workspace.go` | EXISTS - Workspace and TaskRef types (use as-is) |
| `internal/constants/constants.go` | EXISTS - File and directory name constants |

### Import Rules (CRITICAL)

**`internal/workspace/store.go` MAY import:**
- `internal/constants` - for directory names
- `internal/errors` - for sentinel errors
- `internal/domain` - for Workspace type
- `context`, `encoding/json`, `os`, `path/filepath`, `syscall`, `time`

**`internal/workspace/store.go` MUST NOT import:**
- `internal/cli` - no CLI dependencies
- `internal/config` - store does not need config
- `internal/task` - workspace does not depend on task

### Existing Domain Types (Use As-Is)

The `domain.Workspace` struct already exists in `internal/domain/workspace.go`:

```go
type Workspace struct {
    Name          string                    `json:"name"`
    Path          string                    `json:"path"`
    WorktreePath  string                    `json:"worktree_path"`
    Branch        string                    `json:"branch"`
    Status        constants.WorkspaceStatus `json:"status"`
    Tasks         []TaskRef                 `json:"tasks"`
    CreatedAt     time.Time                 `json:"created_at"`
    UpdatedAt     time.Time                 `json:"updated_at"`
    Metadata      map[string]any            `json:"metadata,omitempty"`
    SchemaVersion int                       `json:"schema_version"`
}
```

### Existing Constants (Use These)

From `internal/constants/constants.go`:
```go
const (
    TaskFileName      = "task.json"
    WorkspaceFileName = "workspace.json"
    AtlasHome         = ".atlas"
    WorkspacesDir     = "workspaces"
    TasksDir          = "tasks"
    ArtifactsDir      = "artifacts"
    LogsDir           = "logs"
)
```

### Store Interface Design

```go
// Store defines the interface for workspace persistence operations.
type Store interface {
    // Create persists a new workspace. Returns ErrWorkspaceExists if workspace already exists.
    Create(ctx context.Context, ws *domain.Workspace) error

    // Get retrieves a workspace by name. Returns ErrWorkspaceNotFound if not found.
    Get(ctx context.Context, name string) (*domain.Workspace, error)

    // Update persists changes to an existing workspace. Returns ErrWorkspaceNotFound if not found.
    Update(ctx context.Context, ws *domain.Workspace) error

    // List returns all workspaces. Returns empty slice if none exist.
    List(ctx context.Context) ([]*domain.Workspace, error)

    // Delete removes a workspace and its data. Returns ErrWorkspaceNotFound if not found.
    Delete(ctx context.Context, name string) error

    // Exists returns true if a workspace with the given name exists.
    Exists(ctx context.Context, name string) (bool, error)
}
```

### FileStore Implementation Pattern

```go
// FileStore implements Store using the local filesystem.
type FileStore struct {
    baseDir string // Usually ~/.atlas
}

// NewFileStore creates a new FileStore with the given base directory.
// If baseDir is empty, uses the default ~/.atlas directory.
func NewFileStore(baseDir string) (*FileStore, error) {
    if baseDir == "" {
        home, err := os.UserHomeDir()
        if err != nil {
            return nil, fmt.Errorf("failed to get user home directory: %w", err)
        }
        baseDir = filepath.Join(home, constants.AtlasHome)
    }
    return &FileStore{baseDir: baseDir}, nil
}
```

### Atomic Write Pattern (CRITICAL)

```go
func atomicWrite(path string, data []byte, perm os.FileMode) error {
    // 1. Write to temp file
    tmpPath := path + ".tmp"
    f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
    if err != nil {
        return fmt.Errorf("failed to create temp file: %w", err)
    }

    // 2. Write data
    if _, err := f.Write(data); err != nil {
        f.Close()
        os.Remove(tmpPath)
        return fmt.Errorf("failed to write data: %w", err)
    }

    // 3. Sync to disk (ensure data is persisted before rename)
    if err := f.Sync(); err != nil {
        f.Close()
        os.Remove(tmpPath)
        return fmt.Errorf("failed to sync file: %w", err)
    }

    // 4. Close file before rename
    if err := f.Close(); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("failed to close file: %w", err)
    }

    // 5. Atomic rename
    if err := os.Rename(tmpPath, path); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("failed to rename file: %w", err)
    }

    return nil
}
```

### File Locking Pattern (POSIX flock)

```go
import "syscall"

func acquireLock(path string) (*os.File, error) {
    f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return nil, fmt.Errorf("failed to open lock file: %w", err)
    }

    // LOCK_EX = exclusive lock, LOCK_NB = non-blocking
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        f.Close()
        return nil, fmt.Errorf("failed to acquire lock: %w", err)
    }

    return f, nil
}

func releaseLock(f *os.File) error {
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
        return fmt.Errorf("failed to release lock: %w", err)
    }
    return f.Close()
}
```

### Error Messages Pattern

```go
// Action-first error wrapping at package boundaries
return fmt.Errorf("failed to create workspace '%s': %w", name, err)
return fmt.Errorf("failed to read workspace '%s': %w", name, err)
return fmt.Errorf("failed to update workspace '%s': %w", name, err)
return fmt.Errorf("failed to delete workspace '%s': %w", name, err)

// Corrupted state with actionable message
return fmt.Errorf("workspace '%s' has corrupted state file: %w. Consider deleting ~/.atlas/workspaces/%s/", name, err, name)
```

### Testing Patterns

```go
func TestFileStore_Create_Success(t *testing.T) {
    // Use t.TempDir() for isolated test directory
    tmpDir := t.TempDir()
    store, err := NewFileStore(tmpDir)
    require.NoError(t, err)

    ws := &domain.Workspace{
        Name:      "test-workspace",
        Status:    constants.WorkspaceStatusActive,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err = store.Create(context.Background(), ws)
    require.NoError(t, err)

    // Verify file exists
    path := filepath.Join(tmpDir, constants.WorkspacesDir, "test-workspace", constants.WorkspaceFileName)
    assert.FileExists(t, path)

    // Verify content
    data, err := os.ReadFile(path)
    require.NoError(t, err)

    var loaded domain.Workspace
    err = json.Unmarshal(data, &loaded)
    require.NoError(t, err)

    assert.Equal(t, "test-workspace", loaded.Name)
    assert.Equal(t, 1, loaded.SchemaVersion)
}

func TestFileStore_Get_CorruptedJSON(t *testing.T) {
    tmpDir := t.TempDir()
    store, err := NewFileStore(tmpDir)
    require.NoError(t, err)

    // Create corrupted workspace.json
    wsDir := filepath.Join(tmpDir, constants.WorkspacesDir, "corrupted")
    err = os.MkdirAll(wsDir, 0755)
    require.NoError(t, err)

    wsFile := filepath.Join(wsDir, constants.WorkspaceFileName)
    err = os.WriteFile(wsFile, []byte("{invalid json"), 0644)
    require.NoError(t, err)

    // Attempt to read
    _, err = store.Get(context.Background(), "corrupted")
    require.Error(t, err)
    assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceCorrupted)
    assert.Contains(t, err.Error(), "corrupted state file")
}

func TestFileStore_JSONFieldNaming(t *testing.T) {
    tmpDir := t.TempDir()
    store, err := NewFileStore(tmpDir)
    require.NoError(t, err)

    ws := &domain.Workspace{
        Name:         "test",
        WorktreePath: "/some/path",
        Status:       constants.WorkspaceStatusActive,
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }

    err = store.Create(context.Background(), ws)
    require.NoError(t, err)

    // Read raw JSON and verify snake_case
    path := filepath.Join(tmpDir, constants.WorkspacesDir, "test", constants.WorkspaceFileName)
    data, err := os.ReadFile(path)
    require.NoError(t, err)

    // Must contain snake_case, not camelCase
    assert.Contains(t, string(data), "worktree_path")
    assert.Contains(t, string(data), "created_at")
    assert.Contains(t, string(data), "updated_at")
    assert.Contains(t, string(data), "schema_version")
    assert.NotContains(t, string(data), "worktreePath")
    assert.NotContains(t, string(data), "createdAt")
}
```

### Context Usage Pattern

```go
func (s *FileStore) Get(ctx context.Context, name string) (*domain.Workspace, error) {
    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // ... implementation
}
```

### Previous Epic Learnings (from Epic 2 Retro)

1. **Run `magex test:race`** - Race detection is mandatory
2. **Test early, test manually** - Build and run actual commands
3. **Integration tests required** - Stories that produce/consume files need integration tests
4. **Smoke test validation** - Run actual CLI commands before marking complete

### Sentinel Errors to Add

Add these to `internal/errors/errors.go`:

```go
// Workspace-related errors
var (
    // ErrWorkspaceExists indicates an attempt to create a workspace that already exists.
    ErrWorkspaceExists = errors.New("workspace already exists")

    // ErrWorkspaceNotFound indicates the requested workspace does not exist.
    ErrWorkspaceNotFound = errors.New("workspace not found")

    // ErrWorkspaceCorrupted indicates the workspace state file is corrupted or unreadable.
    ErrWorkspaceCorrupted = errors.New("workspace state corrupted")

    // ErrLockTimeout indicates a file lock could not be acquired within the timeout period.
    ErrLockTimeout = errors.New("lock acquisition timeout")
)
```

### File Structure After This Story

```
internal/
├── workspace/
│   ├── store.go           # NEW: Store interface + FileStore + integrated flock
│   └── store_test.go      # NEW: Comprehensive store tests (32 tests)
├── errors/
│   └── errors.go          # MODIFIED: Add workspace sentinel errors
├── domain/
│   └── workspace.go       # EXISTS: Use as-is
└── constants/
    └── constants.go       # EXISTS: Use as-is
```

### Dependencies Between Epics

This story is foundational for Epic 3:
- Story 3-2 (Git Worktree Operations) will use Store for persistence
- Story 3-3 (Workspace Manager Service) will depend on Store interface
- Stories 3-4 through 3-7 (CLI commands) will use the Manager which uses Store

### Security Considerations

1. **File permissions** - Create workspace directories with 0755, files with 0644
2. **Path traversal** - Validate workspace names to prevent path traversal attacks
3. **No secrets in state** - Workspace.json must never contain API keys or sensitive data

### Performance Considerations

1. **List operation** - Reading all workspaces may be slow with many workspaces; consider caching
2. **Lock contention** - Use non-blocking locks with reasonable timeout
3. **Atomic writes** - Sync to disk adds latency but is necessary for reliability

### Edge Cases to Handle

1. **Empty workspace name** - Return validation error
2. **Invalid characters in name** - Only allow alphanumeric, dash, underscore
3. **Very long workspace names** - Consider filesystem limits
4. **Concurrent access** - flock handles this
5. **Disk full** - Return clear error on write failure
6. **Missing .atlas directory** - Create it on first use
7. **Permission denied** - Return actionable error

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.1]
- [Source: _bmad-output/planning-artifacts/architecture.md#State & Persistence Architecture]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/domain/workspace.go - Workspace struct definition]
- [Source: internal/constants/constants.go - Directory and file name constants]
- [Source: _bmad-output/implementation-artifacts/epic-2-retro-2025-12-28.md - Epic 2 learnings]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test (build and verify imports):
go build -o /tmp/atlas ./cmd/atlas && echo "Build OK"
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

None - implementation proceeded without issues.

### Completion Notes List

- Implemented complete workspace persistence layer with Store interface and FileStore implementation
- All CRUD operations (Create, Get, Update, List, Delete, Exists) implemented with full test coverage
- Atomic write pattern implemented with write-then-rename for crash safety
- POSIX file locking (flock) with 5-second timeout prevents concurrent access corruption
- Workspace name validation prevents path traversal attacks and invalid characters
- All JSON fields use snake_case as required by architecture
- Schema version tracking (v1) enabled for future migrations
- 32 comprehensive tests covering all acceptance criteria (updated from 28 after review)
- All tests pass with race detection (`magex test:race`)
- Code follows project patterns: context handling, error wrapping, import rules
- Security: directories use 0o750, files use 0o600 permissions

**Review Fixes Applied (2025-12-28):**
- Added context parameter to acquireLock() and context checking in lock retry loop
- Fixed incorrect error type in validateName (ErrEmptyValue → ErrValueOutOfRange)
- Added 5 new tests: atomic write preservation, lock timeout, concurrent access, context cancellation during lock
- Fixed documentation inconsistency (removed references to non-existent flock.go/flock_test.go files)

### Change Log

- 2025-12-28: Implemented Story 3.1 - Workspace Data Model and Store
  - Created `internal/workspace/store.go` with Store interface and FileStore implementation
  - Created `internal/workspace/store_test.go` with 28 comprehensive tests
  - Added workspace-related sentinel errors to `internal/errors/errors.go`:
    - ErrWorkspaceExists
    - ErrWorkspaceNotFound
    - ErrWorkspaceCorrupted
    - ErrLockTimeout
- 2025-12-28: Code Review Fixes Applied
  - Added context parameter to acquireLock() for proper cancellation support
  - Added context checking in lock acquisition retry loop
  - Fixed error types in validateName (ErrEmptyValue → ErrValueOutOfRange)
  - Added 5 new tests (32 total): atomic write, lock timeout, concurrent access, context during lock
  - Fixed documentation: removed references to non-existent flock.go/flock_test.go files

### File List

- `internal/workspace/store.go` (NEW) - Store interface and FileStore implementation with atomic writes and flock
- `internal/workspace/store_test.go` (NEW) - Comprehensive test suite (32 tests)
- `internal/errors/errors.go` (MODIFIED) - Added 4 workspace-related sentinel errors
