# Story 6.2: Branch Creation and Naming

Status: done

## Story

As a **user**,
I want **ATLAS to create feature branches with consistent naming**,
So that **my branches follow project conventions and are easy to identify**.

## Acceptance Criteria

1. **Given** a task is starting in a workspace, **When** the system creates a branch, **Then** the branch name follows pattern: `<type>/<workspace-name>`
   - bugfix template → `fix/<workspace-name>`
   - feature template → `feat/<workspace-name>`
   - commit template → `chore/<workspace-name>`

2. **Given** a workspace name contains special characters, **When** the branch is created, **Then** the name is sanitized (lowercase, hyphens, no special chars)

3. **Given** the target branch already exists, **When** the branch is created, **Then** a timestamp suffix is appended: `fix/auth-20251227`

4. **Given** a branch is being created, **When** the system selects a base branch, **Then** it uses the configured base branch (default: main)

5. **Given** a branch creation operation, **When** the branch is created, **Then** the operation is logged with source and target

6. **Given** different templates, **When** branch naming patterns are needed, **Then** patterns are configurable per-template in config

## Tasks / Subtasks

- [x] Task 1: Create `internal/git/branch.go` with BranchManager (AC: 1, 2, 3)
  - [x] 1.1: Define BranchConfig struct with type, baseBranch, namingPattern fields
  - [x] 1.2: Implement `SanitizeBranchName(name string) string` - lowercase, hyphen-replace, trim
  - [x] 1.3: Implement `GenerateBranchName(branchType, workspaceName string) string`
  - [x] 1.4: Implement `GenerateUniqueBranchName(ctx, baseName string) (string, error)` with timestamp fallback

- [x] Task 2: Create BranchCreator service (AC: 1, 3, 4, 5)
  - [x] 2.1: Define BranchCreator interface with `Create(ctx, opts BranchCreateOptions) (*BranchInfo, error)`
  - [x] 2.2: Implement BranchCreateOptions: workspaceName, branchType, baseBranch
  - [x] 2.3: Implement BranchInfo: name, baseBranch, createdAt
  - [x] 2.4: Integrate with existing git.Runner.CreateBranch method

- [x] Task 3: Add per-template branch configuration (AC: 6)
  - [x] 3.1: Add BranchConfig to internal/config/templates.go
  - [x] 3.2: Add default patterns: bugfix→fix, feature→feat, commit→chore
  - [x] 3.3: Support config override via `.atlas/config.yaml`

- [x] Task 4: Integrate with workspace creation flow (AC: 1, 4, 5)
  - [x] 4.1: Update workspace manager to use BranchCreator
  - [x] 4.2: Add logging with zerolog: branch_name, base_branch, workspace_name
  - [x] 4.3: Ensure atomic behavior (cleanup on failure)

- [x] Task 5: Create comprehensive tests (AC: 1-6)
  - [x] 5.1: Test branch name sanitization (special chars, spaces, uppercase)
  - [x] 5.2: Test unique branch generation with timestamp fallback
  - [x] 5.3: Test per-template configuration
  - [x] 5.4: Test integration with git.Runner
  - [x] 5.5: Target 90%+ coverage (achieved: 91.0%)

## Dev Notes

### Existing Code to Reuse/Extend

**CRITICAL: Branch naming logic already exists in `internal/workspace/worktree.go`**

The worktree package already implements branch naming at lines 321-331:

```go
// internal/workspace/worktree.go:321-331
var branchNameRegex = regexp.MustCompile(`[^a-z0-9-]+`)

func generateBranchName(branchType, workspaceName string) string {
    name := strings.ToLower(workspaceName)
    name = branchNameRegex.ReplaceAllString(name, "-")
    name = strings.Trim(name, "-")
    return fmt.Sprintf("%s/%s", branchType, name)
}
```

And unique branch name generation at lines 265-288:

```go
// internal/workspace/worktree.go:265-288
func (r *GitWorktreeRunner) generateUniqueBranchName(ctx context.Context, baseName string) (string, error) {
    exists, err := r.BranchExists(ctx, baseName)
    if err != nil {
        return "", err
    }
    if !exists {
        return baseName, nil
    }
    // Append timestamp suffix
    uniqueName := fmt.Sprintf("%s-%s", baseName, time.Now().Format("20060102-150405"))
    // ...
}
```

**IMPORTANT ARCHITECTURAL DECISION:**
- Option A: Extract branch naming logic to `internal/git/branch.go` and have workspace import it
- Option B: Keep in workspace package and add configuration support there
- **Recommendation**: Option A - centralizes git-related logic in git package per architecture.md

### GitRunner Interface (Already Implemented in Story 6.1)

```go
// internal/git/runner.go - already exists
type Runner interface {
    CreateBranch(ctx context.Context, name string) error
    CurrentBranch(ctx context.Context) (string, error)
    // ... other methods
}
```

### BranchConfig Structure (to add)

```go
// internal/git/branch.go (new file)
type BranchConfig struct {
    Type       string // feat, fix, chore
    BaseBranch string // default: main
    Pattern    string // e.g., "{type}/{name}" (optional, default pattern)
}

type BranchCreateOptions struct {
    WorkspaceName string
    BranchType    string
    BaseBranch    string // empty = use default from config
}

type BranchInfo struct {
    Name       string
    BaseBranch string
    CreatedAt  time.Time
}
```

### Template Configuration Integration

From architecture.md, templates config structure:

```go
// internal/config/templates.go
type TemplateConfig struct {
    Name         string
    Model        string
    BranchPrefix string // "feat", "fix", "chore"
    // ... other fields
}
```

Default mappings from epics.md Story 6.2:
- `bugfix` template → `fix/`
- `feature` template → `feat/`
- `commit` template → `chore/`

### Branch Name Sanitization Rules

From acceptance criteria and existing code:
1. Convert to lowercase
2. Replace non-alphanumeric chars with hyphens (`[^a-z0-9-]+` → `-`)
3. Trim leading/trailing hyphens
4. Result: `My Feature!` → `my-feature`

### Timestamp Format for Collision Handling

From existing worktree.go:275:
```go
uniqueName := fmt.Sprintf("%s-%s", baseName, time.Now().Format("20060102-150405"))
// Result: feat/auth-20251229-143022
```

### Logging Requirements

From project-context.md:

```go
log.Info().
    Str("branch_name", branchInfo.Name).
    Str("base_branch", branchInfo.BaseBranch).
    Str("workspace_name", opts.WorkspaceName).
    Msg("branch created")
```

### Project Structure Notes

**File Locations:**
- `internal/git/branch.go` - BranchConfig, BranchCreateOptions, BranchInfo, sanitization
- `internal/git/branch_test.go` - Unit tests
- `internal/config/templates.go` - Add BranchPrefix to TemplateConfig (may already exist)
- `internal/workspace/worktree.go` - Update to use git.GenerateBranchName

**Import Rules (from architecture.md):**
- `internal/git` can import: constants, errors, domain
- `internal/workspace` can import: git, constants, errors, domain, config
- `internal/git` cannot import: workspace, task, cli, ai, validation, template, tui

### Context-First Pattern

From project-context.md:

```go
// ALWAYS: ctx as first parameter
func (c *BranchCreator) Create(ctx context.Context, opts BranchCreateOptions) (*BranchInfo, error) {
    // ALWAYS: Check cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... implementation
}
```

### Error Handling

Use existing sentinel from internal/errors:
- `ErrBranchExists` - already defined and used in worktree.go:284

Action-first error format:
```go
return nil, fmt.Errorf("failed to create branch: %w", err)
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

- [Source: epics.md - Story 6.2: Branch Creation and Naming]
- [Source: architecture.md - GitRunner Interface section]
- [Source: project-context.md - Context Handling (CRITICAL)]
- [Source: internal/workspace/worktree.go - existing branch naming logic lines 265-331]
- [Source: internal/git/runner.go - CreateBranch interface]
- [Source: epic-6-implementation-notes.md - GitRunner Design]
- [Source: epic-6-user-scenarios.md - Scenarios 1, 5 (workspace & branch creation)]
- [Source: 6-1-gitrunner-implementation.md - Previous story learnings]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoint 3 - workspace/branch creation)
- Scenario 5: Feature Workflow with Speckit (checkpoint 2 - workspace/branch creation)

### Previous Story Intelligence (Story 6.1)

From `6-1-gitrunner-implementation.md`:
- GitRunner renamed to follow Go best practices: `git.Runner` not `git.GitRunner`
- Shared `git.RunCommand` utility in `internal/git/command.go`
- Tests use `t.TempDir()` with temp git repos
- Test coverage at 91.8%

Key patterns established:
- Context cancellation check at method entry
- Errors wrapped with `atlaserrors.ErrGitOperation`
- Action-first error format: `failed to <action>: %w`

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

None

### Completion Notes List

- Created `internal/git/branch.go` with centralized branch naming logic following Option A from Dev Notes
- Implemented `BranchConfig`, `BranchCreateOptions`, and `BranchInfo` structs
- Implemented `SanitizeBranchName()` - converts to lowercase, replaces non-alphanumeric with hyphens, trims, collapses consecutive hyphens
- Implemented `GenerateBranchName()` - creates `{type}/{sanitized-name}` format
- Implemented `ResolveBranchPrefix()` and `ResolveBranchPrefixWithConfig()` - maps template types to branch prefixes with custom config override support
- Created `BranchCreator` service with `Create()` method that integrates with git.Runner
- Added `BranchExists()` to git.Runner interface and implemented in CLIRunner
- Added `BranchPrefixes` map to `TemplatesConfig` in `internal/config/config.go`
- Added default branch prefixes (bugfix→fix, feature→feat, commit→chore) to `DefaultConfig()`
- Updated `internal/workspace/worktree.go` to use centralized `git.GenerateBranchName()`
- Added zerolog logging for branch creation (success and failure)
- Atomic behavior preserved - cleanup on failure already existed
- Created comprehensive tests in `internal/git/branch_test.go` (91.0% coverage)
- All validation commands pass: format, lint, test:race, pre-commit

**Code Review Fixes (2025-12-30):**
- Fixed AC4 violation: `CreateBranch()` now accepts and uses `baseBranch` parameter to create branches from specified base (was ignoring it)
- Added `BranchCreatorService` interface per Task 2.1 (was missing - only struct existed)
- Removed code duplication: Created shared `GenerateUniqueBranchNameWithChecker()` function used by both `BranchCreator` and `GitWorktreeRunner`
- Added `BranchExistsChecker` interface for shared unique name generation logic
- Added new test case for `CreateBranch` from specified base branch
- Test coverage improved to 91.2%

### Change Log

- 2025-12-30: Implemented Story 6.2 - Branch Creation and Naming
- 2025-12-30: Code Review - Fixed 3 issues: BaseBranch now used in branch creation, added BranchCreatorService interface, removed code duplication

### File List

**New Files:**
- `internal/git/branch.go` - BranchConfig, BranchCreateOptions, BranchInfo, BranchCreatorService interface, BranchExistsChecker interface, SanitizeBranchName, GenerateBranchName, GenerateUniqueBranchNameWithChecker, ResolveBranchPrefix, BranchCreator
- `internal/git/branch_test.go` - Comprehensive tests (91.2% coverage)

**Modified Files:**
- `internal/git/runner.go` - Added BranchExists() to Runner interface, updated CreateBranch() signature to include baseBranch parameter
- `internal/git/git_runner.go` - Made branchExists() public as BranchExists(), CreateBranch() now supports baseBranch parameter, added context cancellation check
- `internal/git/runner_test.go` - Added test for CreateBranch from specified base branch
- `internal/config/config.go` - Added BranchPrefixes to TemplatesConfig
- `internal/config/defaults.go` - Added default branch prefixes
- `internal/workspace/worktree.go` - Updated to use git.GenerateBranchName() and git.GenerateUniqueBranchNameWithChecker(), added zerolog logging
