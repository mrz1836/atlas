# Story 3.4: Implement `atlas workspace list` Command

Status: done

## Story

As a **user**,
I want **to run `atlas workspace list` to see all my workspaces**,
So that **I can see what tasks are running and their current status**.

## Acceptance Criteria

1. **Given** workspaces exist **When** I run `atlas workspace list` **Then** a table displays with columns: NAME, BRANCH, STATUS, CREATED, TASKS

2. **Given** workspaces exist **When** the table is rendered **Then** it uses Lip Gloss styling with semantic colors:
   - status "active" → blue
   - status "paused" → gray
   - status "retired" → dim

3. **Given** I run `atlas workspace list --output json` **When** workspaces exist **Then** it returns a structured JSON array of workspaces

4. **Given** no workspaces exist **When** I run `atlas workspace list` **Then** it displays: "No workspaces. Run 'atlas start' to create one."

5. **Given** any state **When** I run `atlas workspace list` **Then** the command completes in <1 second (NFR1)

6. **Given** workspaces exist with tasks **When** the table renders **Then** the TASKS column shows the count of tasks in each workspace

7. **Given** workspaces exist **When** the table renders **Then** the CREATED column shows human-readable relative time (e.g., "2 hours ago")

## Tasks / Subtasks

- [x] Task 1: Create workspace subcommand structure (AC: #1)
  - [x] 1.1: Create `internal/cli/workspace.go` with `workspace` parent command
  - [x] 1.2: Create `internal/cli/workspace_list.go` for the `list` subcommand
  - [x] 1.3: Create `internal/cli/workspace_list_test.go` for tests
  - [x] 1.4: Add `AddWorkspaceCommand(parent *cobra.Command)` to register the command
  - [x] 1.5: Register workspace command in `root.go`

- [x] Task 2: Create TUI output infrastructure (AC: #2, #3)
  - [x] 2.1: Create `internal/tui/styles.go` with semantic color definitions (AdaptiveColor for light/dark)
  - [x] 2.2: Create `internal/tui/output.go` with Output interface (Success, Error, Warning, Info, Table, JSON)
  - [x] 2.3: Create `internal/tui/table.go` with table rendering component
  - [x] 2.4: Create `internal/tui/time.go` with relative time formatting ("2 hours ago", "1 day ago")
  - [x] 2.5: Create tests for TUI components

- [x] Task 3: Implement workspace list command logic (AC: #1, #4, #5, #6, #7)
  - [x] 3.1: Implement `runWorkspaceList(cmd *cobra.Command, args []string) error`
  - [x] 3.2: Get output format from global flags (`--output json|text`)
  - [x] 3.3: Create WorkspaceManager with FileStore (using default ~/.atlas path)
  - [x] 3.4: Call `manager.List(ctx)` to get all workspaces
  - [x] 3.5: Handle empty list case - display helpful message
  - [x] 3.6: Build table rows with: Name, Branch, Status, Created (relative), Tasks count
  - [x] 3.7: Render table using tui.Output interface

- [x] Task 4: Implement styled table output (AC: #2)
  - [x] 4.1: Define status color mapping in tui/styles.go
  - [x] 4.2: Apply semantic colors to STATUS column cells
  - [x] 4.3: Implement NO_COLOR environment variable support (UX-7)
  - [x] 4.4: Ensure table renders correctly in different terminal widths

- [x] Task 5: Implement JSON output mode (AC: #3)
  - [x] 5.1: Detect `--output json` flag from global flags
  - [x] 5.2: Marshal workspaces to JSON and write to stdout
  - [x] 5.3: Include all workspace fields in JSON output
  - [x] 5.4: Test JSON output structure matches expected format

- [x] Task 6: Write comprehensive tests (AC: all)
  - [x] 6.1: Test list with multiple workspaces (happy path)
  - [x] 6.2: Test list with empty workspaces (empty message)
  - [x] 6.3: Test list with --output json flag
  - [x] 6.4: Test relative time formatting
  - [x] 6.5: Test status color application
  - [x] 6.6: Test NO_COLOR environment variable support
  - [x] 6.7: Run `magex format:fix && magex lint && magex test:race`

## Dev Notes

### Critical Warnings (READ FIRST)

1. **MUST use Manager, NOT Store directly** - The CLI should use `workspace.Manager` interface, not `workspace.Store`
2. **MUST respect --output flag** - Check `GlobalFlags.Output` for "json" vs "text" mode
3. **MUST handle empty case gracefully** - No error, just helpful message
4. **MUST complete <1 second** - This is NFR1, keep it fast
5. **MUST support NO_COLOR** - Environment variable disables all colors (UX-7)
6. **MUST use AdaptiveColor** - For light/dark terminal support (UX-6)

### Package Locations

| File | Purpose |
|------|---------|
| `internal/cli/workspace.go` | NEW - Parent workspace command |
| `internal/cli/workspace_list.go` | NEW - List subcommand implementation |
| `internal/cli/workspace_list_test.go` | NEW - Tests for list command |
| `internal/tui/styles.go` | NEW - Lip Gloss semantic color system |
| `internal/tui/output.go` | NEW - Output interface (TTY/JSON) |
| `internal/tui/table.go` | NEW - Table rendering component |
| `internal/tui/time.go` | NEW - Relative time formatting |
| `internal/workspace/manager.go` | EXISTS - Use Manager.List() |
| `internal/workspace/store.go` | EXISTS - Store interface (dependency of Manager) |

### Import Rules (CRITICAL)

**`internal/cli/workspace_list.go` MAY import:**
- `internal/workspace` - for Manager, NewManager, NewFileStore
- `internal/tui` - for Output, Table, Styles
- `internal/constants` - for workspace status values
- `context`, `fmt`, `os`
- `github.com/spf13/cobra`

**`internal/tui/` packages MAY import:**
- `github.com/charmbracelet/lipgloss` - for styling
- `internal/constants` - for status values
- `os`, `fmt`, `time`, `encoding/json`, `io`

**MUST NOT import:**
- `internal/task` - workspace command doesn't need task engine
- `internal/ai` - no AI operations
- `internal/git` - no git operations
- `internal/config` - use global flags, not config directly

### Table Output Format (from Epics)

```
NAME        BRANCH          STATUS    CREATED         TASKS
auth        feat/auth       active    2 hours ago     2
payment     fix/payment     paused    1 day ago       1
old-feat    feat/old        retired   3 days ago      3
```

### Color System (from Architecture UX-4, UX-6)

```go
// Semantic colors with AdaptiveColor for light/dark terminals
var StatusColors = map[constants.WorkspaceStatus]lipgloss.AdaptiveColor{
    constants.WorkspaceStatusActive:  {Light: "#0087AF", Dark: "#00D7FF"},  // Blue
    constants.WorkspaceStatusPaused:  {Light: "#585858", Dark: "#6C6C6C"},  // Gray
    constants.WorkspaceStatusRetired: {Light: "#585858", Dark: "#6C6C6C"},  // Dim
}
```

### Empty State Message

```
No workspaces. Run 'atlas start' to create one.
```

### JSON Output Format

```json
[
  {
    "name": "auth",
    "path": "~/.atlas/workspaces/auth/",
    "worktree_path": "../repo-auth/",
    "branch": "feat/auth",
    "status": "active",
    "tasks": [...],
    "created_at": "2025-12-27T10:00:00Z",
    "updated_at": "2025-12-27T12:00:00Z",
    "schema_version": 1
  }
]
```

### Workspace Command Structure Pattern (follow existing CLI patterns)

```go
// internal/cli/workspace.go
func AddWorkspaceCommand(parent *cobra.Command) {
    workspaceCmd := &cobra.Command{
        Use:   "workspace",
        Short: "Manage ATLAS workspaces",
        Long:  `Commands for managing ATLAS workspaces including listing,
destroying, and retiring workspaces.`,
    }

    // Add subcommands
    addWorkspaceListCmd(workspaceCmd)
    // Future: addWorkspaceDestroyCmd(workspaceCmd)
    // Future: addWorkspaceRetireCmd(workspaceCmd)
    // Future: addWorkspaceLogsCmd(workspaceCmd)

    parent.AddCommand(workspaceCmd)
}
```

### List Command Implementation Pattern

```go
// internal/cli/workspace_list.go
func addWorkspaceListCmd(parent *cobra.Command) {
    cmd := &cobra.Command{
        Use:   "list",
        Short: "List all workspaces",
        Long:  `Display a table of all ATLAS workspaces with their status,
branch, creation time, and task count.`,
        Aliases: []string{"ls"},
        RunE: func(cmd *cobra.Command, args []string) error {
            return runWorkspaceList(cmd, args)
        },
    }
    parent.AddCommand(cmd)
}

func runWorkspaceList(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()
    logger := GetLogger()

    // Get output format from global flags
    output := cmd.Flag("output").Value.String()

    // Create store and manager
    store, err := workspace.NewFileStore("")
    if err != nil {
        return fmt.Errorf("failed to create workspace store: %w", err)
    }

    // Create worktree runner (needed for manager, but list doesn't use it)
    // Use nil worktreeRunner since List doesn't need it
    mgr := workspace.NewManager(store, nil)

    // Get all workspaces
    workspaces, err := mgr.List(ctx)
    if err != nil {
        return fmt.Errorf("failed to list workspaces: %w", err)
    }

    // Handle empty case
    if len(workspaces) == 0 {
        if output == "json" {
            fmt.Println("[]")
        } else {
            fmt.Println("No workspaces. Run 'atlas start' to create one.")
        }
        return nil
    }

    // Output based on format
    if output == "json" {
        return outputWorkspacesJSON(workspaces)
    }

    return outputWorkspacesTable(workspaces)
}
```

### Relative Time Formatting Pattern

```go
// internal/tui/time.go
func RelativeTime(t time.Time) string {
    now := time.Now()
    diff := now.Sub(t)

    switch {
    case diff < time.Minute:
        return "just now"
    case diff < time.Hour:
        mins := int(diff.Minutes())
        if mins == 1 {
            return "1 minute ago"
        }
        return fmt.Sprintf("%d minutes ago", mins)
    case diff < 24*time.Hour:
        hours := int(diff.Hours())
        if hours == 1 {
            return "1 hour ago"
        }
        return fmt.Sprintf("%d hours ago", hours)
    case diff < 7*24*time.Hour:
        days := int(diff.Hours() / 24)
        if days == 1 {
            return "1 day ago"
        }
        return fmt.Sprintf("%d days ago", days)
    default:
        weeks := int(diff.Hours() / 24 / 7)
        if weeks == 1 {
            return "1 week ago"
        }
        return fmt.Sprintf("%d weeks ago", weeks)
    }
}
```

### TUI Output Interface Pattern (from Architecture)

```go
// internal/tui/output.go
type Output interface {
    Success(msg string)
    Error(err error)
    Warning(msg string)
    Info(msg string)
    Table(headers []string, rows [][]string)
    JSON(v interface{})
}

func NewOutput(w io.Writer, format string) Output {
    if format == "json" {
        return &JSONOutput{w: w}
    }
    return &TTYOutput{w: w}
}
```

### NO_COLOR Support Pattern

```go
// internal/tui/styles.go
func init() {
    // Respect NO_COLOR environment variable (UX-7)
    if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
        lipgloss.SetColorProfile(termenv.Ascii)
    }
}
```

### Context Usage Pattern (from previous stories)

```go
func runWorkspaceList(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()

    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // ... rest of implementation
}
```

### Testing Pattern (mock the Manager)

```go
func TestRunWorkspaceList_WithWorkspaces(t *testing.T) {
    // Create temp directory for test store
    tmpDir := t.TempDir()

    // Create and populate test workspaces
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    now := time.Now()
    ws := &domain.Workspace{
        Name:         "test-ws",
        WorktreePath: "/tmp/test",
        Branch:       "feat/test",
        Status:       constants.WorkspaceStatusActive,
        Tasks:        []domain.TaskRef{},
        CreatedAt:    now.Add(-2 * time.Hour),
        UpdatedAt:    now,
    }
    require.NoError(t, store.Create(context.Background(), ws))

    // Create command and run
    // ... assert table output contains expected rows
}

func TestRunWorkspaceList_EmptyState(t *testing.T) {
    tmpDir := t.TempDir()
    store, err := workspace.NewFileStore(tmpDir)
    require.NoError(t, err)

    mgr := workspace.NewManager(store, nil)
    workspaces, err := mgr.List(context.Background())
    require.NoError(t, err)
    assert.Empty(t, workspaces)
    // Assert empty message is displayed
}

func TestRunWorkspaceList_JSONOutput(t *testing.T) {
    // Test that --output json produces valid JSON array
}
```

### Previous Story Learnings (from Story 3-1, 3-2, 3-3)

1. **Context as first parameter** - Always check `ctx.Done()` at entry
2. **Action-first error messages** - `"failed to list workspaces: %w"`
3. **Use constants package** - Never inline magic strings for status
4. **Use errors package** - Never define local sentinel errors
5. **Run `magex test:race`** - Race detection is mandatory
6. **Test empty states** - Edge cases are important

### Epic 2 Retro Learnings

1. **Run `magex test:race`** - Race detection is mandatory
2. **Test early, test manually** - Build and run actual commands
3. **Integration tests required** - CLI commands need thorough testing
4. **Smoke test validation** - Verify end-to-end flows work

### File Structure After This Story

```
internal/
├── cli/
│   ├── root.go              # EXISTS - add AddWorkspaceCommand
│   ├── workspace.go         # NEW - parent workspace command
│   ├── workspace_list.go    # NEW - list subcommand
│   └── workspace_list_test.go # NEW - list tests
├── tui/
│   ├── styles.go            # NEW - Lip Gloss color system
│   ├── output.go            # NEW - Output interface
│   ├── table.go             # NEW - Table component
│   ├── time.go              # NEW - Relative time formatting
│   └── *_test.go            # NEW - TUI tests
└── workspace/
    ├── manager.go           # EXISTS - use Manager.List()
    └── store.go             # EXISTS - FileStore used by Manager
```

### Dependencies Between Stories

This story builds on:
- **Story 3-1** (Workspace Data Model and Store) - uses Store via Manager
- **Story 3-2** (Git Worktree Operations) - Manager dependency (nil OK for List)
- **Story 3-3** (Workspace Manager Service) - uses Manager.List()

This story is required by:
- **Story 7-3** (Status Table Component) - will reuse tui/table.go
- **Story 7-4** (atlas status command) - will reuse tui patterns

### Security Considerations

1. **No secrets in output** - Don't expose API keys or tokens
2. **Path sanitization** - Workspace names already validated by Store
3. **File permissions** - Inherited from Store (0o750/0o600)

### Performance Considerations

1. **NFR1: <1 second** - List should be fast, just file reads
2. **No network calls** - Pure local file operations
3. **Lazy loading** - Don't load task details, just counts

### Edge Cases to Handle

1. **No ~/.atlas directory** - Return empty list, don't error
2. **Corrupted workspace files** - Skip with warning (List already handles this in Store)
3. **Context cancelled** - Return ctx.Err() cleanly
4. **Terminal too narrow** - Truncate or wrap gracefully
5. **Very old timestamps** - Show weeks/months properly

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.4]
- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure & Boundaries]
- [Source: _bmad-output/planning-artifacts/architecture.md#UX-4 to UX-10]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/workspace/manager.go - Manager.List() interface]
- [Source: internal/cli/root.go - CLI command patterns]
- [Source: _bmad-output/implementation-artifacts/3-3-workspace-manager-service.md - Previous story patterns]

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test:
go run ./cmd/atlas workspace list              # Should show empty message or table
go run ./cmd/atlas workspace list --output json # Should output JSON array

# Manual test with actual workspaces (if any exist):
atlas workspace list
atlas workspace list --output json
NO_COLOR=1 atlas workspace list  # Should be colorless
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No debug issues encountered

### Completion Notes List

- Implemented `atlas workspace list` command with table and JSON output formats
- Created reusable TUI package with styles, output, table, and time components
- All acceptance criteria satisfied:
  - AC1: Table displays NAME, BRANCH, STATUS, CREATED, TASKS columns ✅
  - AC2: Lip Gloss styling with AdaptiveColor for status colors ✅
  - AC3: `--output json` returns structured JSON array ✅
  - AC4: Empty state displays helpful message ✅
  - AC5: Command completes in <1 second (local file operations only) ✅
  - AC6: TASKS column shows task count per workspace ✅
  - AC7: CREATED shows relative time (e.g., "2 hours ago") ✅
- NO_COLOR environment variable support implemented (UX-7)
- Context cancellation handling at entry points
- Comprehensive test coverage with race detection

### Change Log

- 2025-12-28: Implemented Story 3.4 - atlas workspace list command
  - Created internal/cli/workspace.go - parent workspace command
  - Created internal/cli/workspace_list.go - list subcommand with table/JSON output
  - Created internal/cli/workspace_list_test.go - comprehensive tests
  - Created internal/tui/styles.go - semantic color system with AdaptiveColor
  - Created internal/tui/output.go - Output interface (TTY/JSON modes)
  - Created internal/tui/table.go - table rendering component
  - Created internal/tui/time.go - relative time formatting
  - Created internal/tui/*_test.go - TUI component tests
  - Modified internal/cli/root.go - registered workspace command
  - Fixed internal/workspace/manager_test.go - linting issue with copy builtin

- 2025-12-28: Code Review Fixes (AI)
  - Refactored workspace_list.go to use tui package (eliminated code duplication)
  - Changed to use tui.RelativeTime() instead of local duplicate function
  - Changed to use tui.ColorOffset() instead of local duplicate function
  - Changed to use tui.CheckNoColor() instead of inline NO_COLOR handling
  - Updated getStatusColors() to delegate to tui.StatusColors()
  - Added TestRunWorkspaceList_ContextCancellation test
  - Updated test file to use tui package functions
  - Added missing errors.go to File List

### File List

- internal/cli/workspace.go (NEW)
- internal/cli/workspace_list.go (NEW)
- internal/cli/workspace_list_test.go (NEW)
- internal/cli/root.go (MODIFIED - added AddWorkspaceCommand)
- internal/tui/styles.go (NEW)
- internal/tui/output.go (NEW)
- internal/tui/table.go (NEW)
- internal/tui/time.go (NEW)
- internal/tui/styles_test.go (NEW)
- internal/tui/output_test.go (NEW)
- internal/tui/table_test.go (NEW)
- internal/tui/time_test.go (NEW)
- internal/errors/errors.go (MODIFIED - added ErrWorkspaceHasRunningTasks)
- internal/workspace/manager_test.go (MODIFIED - fixed linting issue)
