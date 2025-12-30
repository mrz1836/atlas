# Story 6.1: GitRunner Implementation

Status: done

## Story

As a **developer**,
I want **a GitRunner that wraps git CLI operations**,
So that **ATLAS can perform git operations reliably with proper error handling**.

## Acceptance Criteria

1. **Given** git is installed, **When** I implement `internal/git/runner.go`, **Then** the GitRunner interface provides:
   - `Status(ctx) (*GitStatus, error)` - get working tree status
   - `Add(ctx, paths []string) error` - stage files
   - `Commit(ctx, message string, trailers map[string]string) error` - commit with trailers
   - `Push(ctx, remote, branch string) error` - push to remote
   - `CurrentBranch(ctx) (string, error)` - get current branch name
   - `CreateBranch(ctx, name string) error` - create and checkout branch
   - `Diff(ctx, cached bool) (string, error)` - get diff output

2. **Given** git operations are called, **When** the GitRunner executes them, **Then** all operations run in the specified working directory

3. **Given** a git operation fails, **When** the error is returned, **Then** errors are wrapped with `ErrGitOperation` sentinel

4. **Given** git command output, **When** processing results, **Then** output is captured for logging and debugging

5. **Given** a context with deadline, **When** git operations run, **Then** operations use context for cancellation

6. **Given** unit tests exist, **When** running tests, **Then** tests use temporary git repositories

## Tasks / Subtasks

- [x] Task 1: Create `internal/git/` package structure (AC: 1)
  - [x] 1.1: Create `internal/git/runner.go` with GitRunner interface
  - [x] 1.2: Create `internal/git/types.go` with GitStatus and related types
  - [x] 1.3: Create `internal/git/errors.go` (thin wrapper importing from internal/errors)

- [x] Task 2: Implement GitRunner interface in `internal/git/git_runner.go` (AC: 1, 2)
  - [x] 2.1: Create `CLIRunner` struct with workDir field
  - [x] 2.2: Implement `NewRunner(workDir string) (*CLIRunner, error)`
  - [x] 2.3: Implement `runGitCommand(ctx, args...)` helper (reuse pattern from workspace/worktree.go:364-386)

- [x] Task 3: Implement Status method (AC: 1, 4)
  - [x] 3.1: Implement `git status --porcelain -uall` parsing
  - [x] 3.2: Create `Status` struct with staged/unstaged/untracked slices
  - [x] 3.3: Parse branch info including ahead/behind counts

- [x] Task 4: Implement Add method (AC: 1, 2)
  - [x] 4.1: Implement `git add <paths>` with path validation
  - [x] 4.2: Handle empty paths slice (stage all changes with -A)
  - [x] 4.3: Capture and log output

- [x] Task 5: Implement Commit method with trailers (AC: 1, 4)
  - [x] 5.1: Implement `git commit -m <message>` with proper quoting
  - [x] 5.2: Append trailers as footer (ATLAS-Task, ATLAS-Template)
  - [x] 5.3: Add `--cleanup=strip` flag per sc.md reference

- [x] Task 6: Implement Push method (AC: 1, 2)
  - [x] 6.1: Implement `git push <remote> <branch>`
  - [x] 6.2: Support `--set-upstream` flag for upstream tracking
  - [x] 6.3: Implement with setUpstream bool parameter

- [x] Task 7: Implement CurrentBranch method (AC: 1)
  - [x] 7.1: Implement `git rev-parse --abbrev-ref HEAD`
  - [x] 7.2: Handle detached HEAD state (return error)

- [x] Task 8: Implement CreateBranch method (AC: 1)
  - [x] 8.1: Implement `git checkout -b <name>`
  - [x] 8.2: Error if branch already exists (ErrBranchExists)

- [x] Task 9: Implement Diff method (AC: 1, 4)
  - [x] 9.1: Implement `git diff` (unstaged changes)
  - [x] 9.2: Implement `git diff --cached` (staged changes)
  - [x] 9.3: Support cached bool parameter for both modes

- [x] Task 10: Implement context cancellation (AC: 5)
  - [x] 10.1: Use `exec.CommandContext(ctx, ...)` in all commands
  - [x] 10.2: Check `ctx.Done()` at method entry for long operations
  - [x] 10.3: Propagate context error correctly

- [x] Task 11: Implement error wrapping (AC: 3)
  - [x] 11.1: Wrap all errors with `atlaserrors.ErrGitOperation`
  - [x] 11.2: Use action-first error format: `failed to <action>: %w`
  - [x] 11.3: Include stderr in error messages for debugging

- [x] Task 12: Create comprehensive tests (AC: 6)
  - [x] 12.1: Create `internal/git/runner_test.go`
  - [x] 12.2: Add test helper to create temp git repos with testify
  - [x] 12.3: Test each method with success and failure cases
  - [x] 12.4: Test context cancellation behavior
  - [x] 12.5: Test error wrapping with `errors.Is(err, ErrGitOperation)`
  - [x] 12.6: Test coverage at 91.8%

## Dev Notes

### Existing Code to Reuse

**CRITICAL: Reuse patterns from `internal/workspace/worktree.go`**

The worktree package already has excellent git command execution patterns:

```go
// internal/workspace/worktree.go:364-386 - runGitCommand pattern
func runGitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = repoPath
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    // ... error handling with stderr
}
```

**IMPORTANT: Do NOT duplicate this function. Either:**
1. Extract to `internal/git/command.go` and import in both packages
2. Or make worktree import from git package (preferred architecture)

### GitRunner Interface

```go
// internal/git/runner.go
type GitRunner interface {
    Status(ctx context.Context) (*GitStatus, error)
    Add(ctx context.Context, paths []string) error
    Commit(ctx context.Context, message string, trailers map[string]string) error
    Push(ctx context.Context, remote, branch string) error
    CurrentBranch(ctx context.Context) (string, error)
    CreateBranch(ctx context.Context, name string) error
    Diff(ctx context.Context, cached bool) (string, error)
}
```

### GitStatus Type

```go
// internal/git/types.go
type GitStatus struct {
    Staged    []FileChange  // Files staged for commit
    Unstaged  []FileChange  // Modified but not staged
    Untracked []string      // Untracked files
    Branch    string        // Current branch name
    Ahead     int           // Commits ahead of upstream
    Behind    int           // Commits behind upstream
}

type FileChange struct {
    Path   string      // File path relative to repo root
    Status ChangeType  // Added, Modified, Deleted, Renamed, etc.
    OldPath string     // For renamed files, the original path
}

type ChangeType string

const (
    ChangeAdded    ChangeType = "A"
    ChangeModified ChangeType = "M"
    ChangeDeleted  ChangeType = "D"
    ChangeRenamed  ChangeType = "R"
    ChangeCopied   ChangeType = "C"
    ChangeUnmerged ChangeType = "U"
)
```

### Commit with Trailers Pattern

From `epic-6-implementation-notes.md` and `~/.claude/commands/sc.md`:

```go
// Commit message with ATLAS trailers
func (r *GitCLIRunner) Commit(ctx context.Context, msg string, trailers map[string]string) error {
    // Build commit message with trailers in footer
    fullMsg := msg
    if len(trailers) > 0 {
        fullMsg += "\n\n"
        for k, v := range trailers {
            fullMsg += fmt.Sprintf("%s: %s\n", k, v)
        }
    }

    // Use --cleanup=strip to handle formatting
    _, err := r.runGitCommand(ctx, "commit", "-m", fullMsg, "--cleanup=strip")
    return err
}
```

### Error Sentinel from Architecture

```go
// Import from internal/errors
import atlaserrors "github.com/mrz1836/atlas/internal/errors"

// ErrGitOperation is already defined in internal/errors/errors.go
// Use: atlaserrors.ErrGitOperation
```

### Testing Pattern

Follow the pattern from `internal/workspace/worktree_test.go`:

```go
func TestGitRunner_Status(t *testing.T) {
    // Create temp directory
    tmpDir := t.TempDir()

    // Initialize git repo
    _, err := exec.Command("git", "init", tmpDir).Output()
    require.NoError(t, err)

    // Create runner
    runner, err := NewGitRunner(tmpDir)
    require.NoError(t, err)

    // Test status on clean repo
    status, err := runner.Status(context.Background())
    require.NoError(t, err)
    assert.Empty(t, status.Staged)
    assert.Empty(t, status.Unstaged)
    assert.Empty(t, status.Untracked)
}
```

### Project Structure Notes

**File Locations:**
- `internal/git/runner.go` - GitRunner interface
- `internal/git/git_runner.go` - GitCLIRunner implementation
- `internal/git/types.go` - GitStatus, FileChange, ChangeType
- `internal/git/runner_test.go` - Unit tests
- `internal/git/integration_test.go` - Integration tests (optional)

**Import Rules (from architecture.md):**
- `internal/git` can import: constants, errors, domain
- `internal/git` cannot import: task, workspace, cli, ai, validation, template, tui

### Context-First Pattern

From `project-context.md`:

```go
// ALWAYS: ctx as first parameter
func (r *GitCLIRunner) Status(ctx context.Context) (*GitStatus, error) {
    // ALWAYS: Check cancellation for long operations
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... implementation
}
```

### Validation Commands Required

**Before marking story complete, run ALL FOUR:**
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

### References

- [Source: epics.md - Story 6.1: GitRunner Implementation]
- [Source: architecture.md - GitRunner Interface section]
- [Source: architecture.md - Error Handling Strategy]
- [Source: project-context.md - Context Handling (CRITICAL)]
- [Source: epic-6-implementation-notes.md - GitRunner Design]
- [Source: epic-6-user-scenarios.md - Scenarios 1, 4, 5]
- [Source: internal/workspace/worktree.go - runGitCommand pattern]
- [Source: internal/errors/errors.go - ErrGitOperation sentinel]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoints 8-9)
- Scenario 4: Multi-File Logical Grouping
- Scenario 5: Feature Workflow with Speckit (checkpoints 14-16)

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No issues encountered during implementation

### Completion Notes List

- Implemented complete GitRunner interface with all 7 methods: Status, Add, Commit, Push, CurrentBranch, CreateBranch, Diff
- Renamed types to follow Go best practices (git.Runner instead of git.GitRunner, git.Status instead of git.GitStatus, git.CLIRunner instead of git.GitCLIRunner)
- Implemented robust git status parsing with --porcelain --branch format including ahead/behind tracking
- Added Push method with setUpstream parameter for --set-upstream flag
- All methods implement context cancellation pattern with ctx.Done() check at entry
- All errors properly wrapped with ErrGitOperation sentinel from internal/errors
- Created comprehensive test suite with 91.8% coverage
- Tests use exec.CommandContext for context-awareness
- All validation commands pass:
  - magex format:fix ✓
  - magex lint ✓ (0 issues)
  - magex test:race ✓
  - go-pre-commit run --all-files ✓ (6 checks pass, including gitleaks)

### File List

- internal/git/runner.go (new) - Runner interface definition
- internal/git/types.go (new) - Status, FileChange, ChangeType types
- internal/git/errors.go (new) - Error sentinel re-exports
- internal/git/command.go (new) - Shared git command execution utility
- internal/git/git_runner.go (new) - CLIRunner implementation
- internal/git/runner_test.go (new) - Comprehensive test suite
- internal/workspace/worktree.go (modified) - Updated to use shared git.RunCommand
- internal/workspace/worktree_test.go (modified) - Updated test to use git.RunCommand

## Change Log

- 2025-12-29: Implemented Story 6.1 - GitRunner with all 7 interface methods, comprehensive tests at 91.8% coverage
- 2025-12-29: Code Review fixes applied:
  - Extracted runGitCommand to shared internal/git/command.go (eliminated duplication with workspace/worktree.go)
  - Fixed potential slice bounds panic in parseBranchLine with proper bounds checking
  - Updated workspace/worktree.go to import and use git.RunCommand
  - Corrected coverage percentage from 92.6% to actual 91.8%
