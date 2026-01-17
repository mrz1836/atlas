# Story 1.2: Create Constants Package

Status: done

## Story

As a **developer**,
I want **a centralized constants package with all shared values**,
So that **I never use magic strings and all AI agents use consistent values**.

## Acceptance Criteria

1. **Given** the project structure exists **When** I implement `internal/constants/` **Then** `constants.go` contains:
   - File names: `TaskFileName = "task.json"`, `WorkspaceFileName = "workspace.json"`
   - Directories: `AtlasHome = ".atlas"`, `WorkspacesDir = "workspaces"`, `TasksDir = "tasks"`, `ArtifactsDir = "artifacts"`

2. **Given** constants.go exists **When** I implement status constants **Then** `status.go` contains task and workspace status constants

3. **Given** status.go exists **When** I implement path constants **Then** `paths.go` contains path-related constants

4. **Given** all constant files exist **When** I inspect them **Then** all constants are exported and documented

5. **Given** constants are documented **When** I run tests **Then** the package has 100% test coverage for any helper functions

6. **Given** the constants package is complete **When** I search the codebase **Then** no other package in the codebase defines inline magic strings for these values

## Tasks / Subtasks

- [x] Task 1: Create constants.go with file and directory constants (AC: #1, #4)
  - [x] Create `internal/constants/constants.go`
  - [x] Define file name constants (TaskFileName, WorkspaceFileName)
  - [x] Define directory constants (AtlasHome, WorkspacesDir, TasksDir, ArtifactsDir)
  - [x] Define timeout constants (DefaultAITimeout, DefaultCITimeout, CIPollInterval)
  - [x] Define retry constants (MaxRetryAttempts, InitialBackoff)
  - [x] Add comprehensive documentation for all constants

- [x] Task 2: Create status.go with task and workspace status constants (AC: #2, #4)
  - [x] Create `internal/constants/status.go`
  - [x] Define TaskStatus type and constants (Pending, Running, Validating, ValidationFailed, AwaitingApproval, Completed, Rejected, Abandoned, GHFailed, CIFailed, CITimeout)
  - [x] Define WorkspaceStatus type and constants (Active, Paused, Retired)
  - [x] Add String() methods for type safety and debugging
  - [x] Add comprehensive documentation

- [x] Task 3: Create paths.go with path-related constants (AC: #3, #4)
  - [x] Create `internal/constants/paths.go`
  - [x] Define log file paths and patterns
  - [x] Define config file names (GlobalConfigName, ProjectConfigName)
  - [x] Define branch prefix patterns (BranchPrefixFix, BranchPrefixFeat, BranchPrefixChore)
  - [x] Add comprehensive documentation

- [x] Task 4: Create test file(s) for helper functions (AC: #5)
  - [x] Create `internal/constants/constants_test.go` or `internal/constants/status_test.go`
  - [x] Test String() methods on status types
  - [x] Test any helper functions added
  - [x] Ensure 100% coverage of any non-trivial code

- [x] Task 5: Remove .gitkeep and verify (AC: #6)
  - [x] Remove `internal/constants/.gitkeep`
  - [x] Run `go build ./...` to verify compilation
  - [x] Run `magex lint` to verify linting passes
  - [x] Run `magex test` to verify tests pass

## Dev Notes

### Critical Architecture Requirements

**This package is foundational - it MUST be implemented correctly!** All other packages will import from this package. Any mistakes here will propagate throughout the codebase.

#### Package Rules (CRITICAL - ENFORCE STRICTLY)

From architecture.md:
- **internal/constants** → MUST NOT import any other package (only standard library allowed)
- All packages CAN import constants
- Constants must be the **single source of truth** for shared values

#### Required Constants (from Architecture Document)

```go
package constants

import "time"

// File names used by ATLAS for state persistence
const (
    TaskFileName      = "task.json"
    WorkspaceFileName = "workspace.json"
)

// Directory names and paths
const (
    AtlasHome     = ".atlas"
    WorkspacesDir = "workspaces"
    TasksDir      = "tasks"
    ArtifactsDir  = "artifacts"
    LogsDir       = "logs"
)

// Timeouts for various operations
const (
    DefaultAITimeout = 30 * time.Minute
    DefaultCITimeout = 30 * time.Minute
    CIPollInterval   = 2 * time.Minute
)

// Retry configuration defaults
const (
    MaxRetryAttempts = 3
    InitialBackoff   = 1 * time.Second
)
```

#### Status Constants (from Architecture Document)

```go
// TaskStatus represents the state of a task in the state machine
type TaskStatus string

const (
    TaskStatusPending          TaskStatus = "pending"
    TaskStatusRunning          TaskStatus = "running"
    TaskStatusValidating       TaskStatus = "validating"
    TaskStatusValidationFailed TaskStatus = "validation_failed"
    TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"
    TaskStatusCompleted        TaskStatus = "completed"
    TaskStatusRejected         TaskStatus = "rejected"
    TaskStatusAbandoned        TaskStatus = "abandoned"
    TaskStatusGHFailed         TaskStatus = "gh_failed"
    TaskStatusCIFailed         TaskStatus = "ci_failed"
    TaskStatusCITimeout        TaskStatus = "ci_timeout"
)

// WorkspaceStatus represents the state of a workspace
type WorkspaceStatus string

const (
    WorkspaceStatusActive  WorkspaceStatus = "active"
    WorkspaceStatusPaused  WorkspaceStatus = "paused"
    WorkspaceStatusRetired WorkspaceStatus = "retired"
)
```

#### State Transitions Reference (for documentation)

Valid task state transitions from architecture.md:
```
Pending → Running
Running → Validating, GHFailed, CIFailed, CITimeout
Validating → AwaitingApproval, ValidationFailed
ValidationFailed → Running, Abandoned
AwaitingApproval → Completed, Running, Rejected
GHFailed → Running, Abandoned
CIFailed → Running, Abandoned
CITimeout → Running, Abandoned
```

### Code Style Requirements

1. **No imports from internal packages** - only standard library allowed
2. **snake_case for status values** - these will be used in JSON serialization
3. **Comprehensive documentation** - every exported constant/type needs a comment
4. **Type safety** - use typed constants (TaskStatus, WorkspaceStatus) not raw strings

### Testing Strategy

- Test String() methods return expected values
- Test that status constants serialize correctly to JSON (lowercase snake_case)
- Consider table-driven tests for exhaustive coverage

### Previous Story Intelligence

From Story 1-1 completion:
- Project structure is in place with `internal/constants/.gitkeep`
- Go 1.24 is configured
- golangci-lint is configured with strict rules (gochecknoglobals, revive)
- Use function-based patterns to avoid global variable linter warnings
- All code must pass `magex lint` before completion

### Git Commit Patterns

Recent commits follow conventional commits format:
- `feat(project-structure): complete Go module initialization...`
- `feat(sprint-status): update epic and story statuses...`

For this story, use:
- `feat(constants): add constants package with file and directory constants`
- `feat(constants): add task and workspace status constants`

### Project Structure Notes

Current structure:
```
internal/
├── constants/
│   └── .gitkeep    # Will be replaced by actual files
├── cli/
│   └── root.go     # Existing CLI implementation
├── deps/
│   └── deps.go     # Temporary dependency preservation
└── ... (other empty packages with .gitkeep)
```

After this story:
```
internal/
├── constants/
│   ├── constants.go      # File/dir/timeout/retry constants
│   ├── status.go         # TaskStatus, WorkspaceStatus types
│   ├── paths.go          # Path-related constants
│   └── constants_test.go # Tests for helper functions
└── ...
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Constants Package]
- [Source: _bmad-output/planning-artifacts/architecture.md#Task Engine Architecture]
- [Source: _bmad-output/planning-artifacts/architecture.md#State Transitions Table]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.2]
- [Source: _bmad-output/project-context.md#Package Import Rules]
- [Source: _bmad-output/project-context.md#JSON & Logging Conventions]

### Validation Commands (MUST RUN BEFORE COMPLETION)

```bash
magex format:fix    # Format code
magex lint          # Must pass with 0 issues
magex test          # Must pass all tests
go build ./...      # Must compile
```

### Anti-Patterns to Avoid

```go
// ❌ NEVER: Import other internal packages
import "github.com/mrz1836/atlas/internal/errors"  // DON'T

// ❌ NEVER: Use untyped string constants for status
const StatusRunning = "running"  // DON'T - use typed TaskStatus

// ❌ NEVER: Use camelCase in values that will be JSON-serialized
const TaskStatusRunning TaskStatus = "Running"  // DON'T - use "running"

// ❌ NEVER: Leave constants undocumented
const Foo = "bar"  // DON'T - add documentation

// ✅ DO: Use typed constants
const TaskStatusRunning TaskStatus = "running"  // Correct

// ✅ DO: Document everything
// TaskFileName is the name of the file that stores task state
const TaskFileName = "task.json"
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A - No debug issues encountered

### Completion Notes List

- Created `constants.go` with file names (TaskFileName, WorkspaceFileName), directory names (AtlasHome, WorkspacesDir, TasksDir, ArtifactsDir, LogsDir), timeout constants (DefaultAITimeout, DefaultCITimeout, CIPollInterval), and retry constants (MaxRetryAttempts, InitialBackoff)
- Created `status.go` with typed TaskStatus and WorkspaceStatus enums, including all 11 task statuses and 3 workspace statuses per architecture spec
- Added String() methods to both status types for fmt.Stringer compatibility
- Created `paths.go` with log file names, config file names, and branch prefix patterns
- Created comprehensive `status_test.go` with table-driven tests for String() methods and JSON serialization/deserialization
- Achieved 100% test coverage for helper functions
- All validation commands pass: `magex format:fix`, `magex lint`, `magex test`, `go build ./...`
- Package has no imports from other internal packages (only standard library)
- All constants use snake_case values for JSON compatibility

### File List

- `internal/constants/constants.go` (new)
- `internal/constants/status.go` (new)
- `internal/constants/paths.go` (new)
- `internal/constants/status_test.go` (new)
- `internal/constants/.gitkeep` (deleted)
- `internal/deps/deps.go` (modified - import reordering by format:fix)

## Change Log

- 2025-12-27: Story completed - created constants package with file/directory constants, status types with String() methods, path constants, and comprehensive tests (100% coverage)

