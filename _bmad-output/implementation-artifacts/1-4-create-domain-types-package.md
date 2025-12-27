# Story 1.4: Create Domain Types Package

Status: done

## Story

As a **developer**,
I want **a domain package with all shared types**,
So that **type definitions are centralized and consistent across all packages**.

## Acceptance Criteria

1. **Given** the constants and errors packages exist **When** I implement `internal/domain/` **Then** `task.go` contains:
   - `Task` struct with JSON tags using snake_case
   - `Step` struct
   - `StepResult` struct

2. **Given** the domain package is being implemented **When** I create `workspace.go` **Then** it contains:
   - `Workspace` struct with JSON tags
   - `TaskRef` struct

3. **Given** the domain package is being implemented **When** I create `template.go` **Then** it contains:
   - `Template` struct
   - `StepDefinition` struct

4. **Given** the domain package is being implemented **When** I create `ai.go` **Then** it contains:
   - `AIRequest` struct
   - `AIResult` struct

5. **Given** the domain package is being implemented **When** I create `status.go` **Then** it contains:
   - Re-export of `TaskStatus` type from constants (or type alias)
   - Re-export of `WorkspaceStatus` type from constants (or type alias)

6. **Given** the domain package is complete **When** I inspect all JSON tags **Then** all tags use snake_case per Architecture requirement

7. **Given** the domain package is complete **When** I check imports **Then** the package does not import any other internal packages (except constants/errors)

8. **Given** all types are defined **When** I write tests **Then** tests include example JSON representations verifying serialization

## Tasks / Subtasks

- [x] Task 1: Create task.go with Task, Step, and StepResult types (AC: #1, #6, #7)
  - [x] Create `internal/domain/task.go`
  - [x] Define `Task` struct with fields: ID, WorkspaceID, TemplateID, Description, Status, CurrentStep, Steps, CreatedAt, UpdatedAt, CompletedAt, Config, Metadata
  - [x] Define `Step` struct with fields: Name, Type, Status, StartedAt, CompletedAt, Error, Attempts
  - [x] Define `StepResult` struct with fields: StepName, Success, Output, Error, Duration, FilesChanged, ArtifactPath
  - [x] Use snake_case for ALL JSON tags
  - [x] Add comprehensive documentation for each type and field
  - [x] Verify imports only include constants, errors, and std lib

- [x] Task 2: Create workspace.go with Workspace and TaskRef types (AC: #2, #6, #7)
  - [x] Create `internal/domain/workspace.go`
  - [x] Define `Workspace` struct with fields: Name, Path, WorktreePath, Branch, Status, Tasks, CreatedAt, UpdatedAt, Metadata
  - [x] Define `TaskRef` struct with fields: ID, Status, StartedAt, CompletedAt
  - [x] Use snake_case for ALL JSON tags
  - [x] Add comprehensive documentation

- [x] Task 3: Create template.go with Template and StepDefinition types (AC: #3, #6, #7)
  - [x] Create `internal/domain/template.go`
  - [x] Define `Template` struct with fields: Name, Description, BranchPrefix, DefaultModel, Steps, ValidationCommands, Variables
  - [x] Define `StepDefinition` struct with fields: Name, Type, Description, Required, Timeout, RetryCount, Config
  - [x] Define `StepType` enum with values: AI, Validation, Git, Human, SDD, CI
  - [x] Use snake_case for ALL JSON tags
  - [x] Add comprehensive documentation

- [x] Task 4: Create ai.go with AIRequest and AIResult types (AC: #4, #6, #7)
  - [x] Create `internal/domain/ai.go`
  - [x] Define `AIRequest` struct with fields: Prompt, Context, Model, MaxTurns, Timeout, PermissionMode, SystemPrompt, WorkingDir
  - [x] Define `AIResult` struct with fields: Success, Output, SessionID, DurationMs, NumTurns, TotalCostUSD, Error, FilesChanged
  - [x] Use snake_case for ALL JSON tags
  - [x] Add comprehensive documentation

- [x] Task 5: Create status.go with type re-exports (AC: #5, #7)
  - [x] Create `internal/domain/status.go`
  - [x] Create type aliases: `type TaskStatus = constants.TaskStatus` and `type WorkspaceStatus = constants.WorkspaceStatus`
  - [x] Re-export all status constants for convenience
  - [x] Add documentation explaining the re-export pattern

- [x] Task 6: Create comprehensive tests (AC: #8)
  - [x] Create `internal/domain/domain_test.go`
  - [x] Test JSON serialization for Task struct (verify snake_case)
  - [x] Test JSON serialization for Workspace struct
  - [x] Test JSON serialization for Template struct
  - [x] Test JSON serialization for AIRequest and AIResult structs
  - [x] Test JSON deserialization (round-trip tests)
  - [x] Use table-driven tests for multiple scenarios
  - [x] Include example JSON in test documentation

- [x] Task 7: Remove .gitkeep and validate (AC: all)
  - [x] Remove `internal/domain/.gitkeep`
  - [x] Run `go build ./...` to verify compilation
  - [x] Run `magex format:fix` to format code
  - [x] Run `magex lint` to verify linting passes (must have 0 issues)
  - [x] Run `magex test` to verify tests pass

## Dev Notes

### Critical Architecture Requirements

**This package is foundational - it MUST be implemented correctly!** All other packages will import domain types. Any mistakes here will propagate throughout the codebase.

#### Package Rules (CRITICAL - ENFORCE STRICTLY)

From architecture.md:
- **internal/domain** → CAN import `internal/constants`, `internal/errors`, and standard library ONLY
- **internal/domain** → MUST NOT import any other internal packages
- All packages CAN import domain

#### JSON Naming Convention (CRITICAL)

From Architecture Document - ALL JSON fields MUST use snake_case:

```go
// ✅ CORRECT - snake_case
type Task struct {
    TaskID     string `json:"task_id"`
    CreatedAt  string `json:"created_at"`
    CurrentStep int   `json:"current_step"`
}

// ❌ WRONG - camelCase
type Task struct {
    TaskID     string `json:"taskId"`      // DON'T
    CreatedAt  string `json:"createdAt"`   // DON'T
}
```

#### Required Types from Architecture Document

**Task struct:**
```go
type Task struct {
    ID            string              `json:"id"`             // Format: task-YYYYMMDD-HHMMSS
    WorkspaceID   string              `json:"workspace_id"`
    TemplateID    string              `json:"template_id"`
    Description   string              `json:"description"`
    Status        TaskStatus          `json:"status"`
    CurrentStep   int                 `json:"current_step"`
    Steps         []Step              `json:"steps"`
    CreatedAt     time.Time           `json:"created_at"`
    UpdatedAt     time.Time           `json:"updated_at"`
    CompletedAt   *time.Time          `json:"completed_at,omitempty"`
    Config        TaskConfig          `json:"config"`
    Metadata      map[string]any      `json:"metadata,omitempty"`
    SchemaVersion int                 `json:"schema_version"` // Always included per architecture
}
```

**Step struct:**
```go
type Step struct {
    Name       string     `json:"name"`
    Type       StepType   `json:"type"`
    Status     string     `json:"status"`    // pending, running, completed, failed, skipped
    StartedAt  *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
    Error      string     `json:"error,omitempty"`
    Attempts   int        `json:"attempts"`
}
```

**Workspace struct:**
```go
type Workspace struct {
    Name         string           `json:"name"`
    Path         string           `json:"path"`           // ~/.atlas/workspaces/<name>/
    WorktreePath string           `json:"worktree_path"`  // ../repo-<name>/
    Branch       string           `json:"branch"`
    Status       WorkspaceStatus  `json:"status"`
    Tasks        []TaskRef        `json:"tasks"`
    CreatedAt    time.Time        `json:"created_at"`
    UpdatedAt    time.Time        `json:"updated_at"`
    Metadata     map[string]any   `json:"metadata,omitempty"`
    SchemaVersion int             `json:"schema_version"`
}
```

**AIRequest and AIResult from Architecture:**
```go
type AIRequest struct {
    Prompt         string        `json:"prompt"`
    Context        string        `json:"context,omitempty"`
    Model          string        `json:"model"`
    MaxTurns       int           `json:"max_turns"`
    Timeout        time.Duration `json:"timeout"`
    PermissionMode string        `json:"permission_mode"` // "", "plan"
    SystemPrompt   string        `json:"system_prompt,omitempty"`
    WorkingDir     string        `json:"working_dir"`
}

type AIResult struct {
    Success      bool     `json:"success"`
    Output       string   `json:"output"`
    SessionID    string   `json:"session_id"`
    DurationMs   int      `json:"duration_ms"`
    NumTurns     int      `json:"num_turns"`
    TotalCostUSD float64  `json:"total_cost_usd"`
    Error        string   `json:"error,omitempty"`
    FilesChanged []string `json:"files_changed,omitempty"`
}
```

### StepType Enum Pattern

Follow the same pattern as TaskStatus in constants:

```go
type StepType string

const (
    StepTypeAI         StepType = "ai"
    StepTypeValidation StepType = "validation"
    StepTypeGit        StepType = "git"
    StepTypeHuman      StepType = "human"
    StepTypeSDD        StepType = "sdd"
    StepTypeCI         StepType = "ci"
)

func (s StepType) String() string {
    return string(s)
}
```

### Type Re-export Pattern

The domain package should re-export status types from constants for convenience:

```go
// status.go
package domain

import "github.com/mrz1836/atlas/internal/constants"

// Re-export TaskStatus and WorkspaceStatus from constants package.
// This allows consumers to import domain types and status types together.
type (
    TaskStatus      = constants.TaskStatus
    WorkspaceStatus = constants.WorkspaceStatus
)

// Re-export status constants for convenience.
const (
    TaskStatusPending          = constants.TaskStatusPending
    TaskStatusRunning          = constants.TaskStatusRunning
    TaskStatusValidating       = constants.TaskStatusValidating
    TaskStatusValidationFailed = constants.TaskStatusValidationFailed
    TaskStatusAwaitingApproval = constants.TaskStatusAwaitingApproval
    TaskStatusCompleted        = constants.TaskStatusCompleted
    TaskStatusRejected         = constants.TaskStatusRejected
    TaskStatusAbandoned        = constants.TaskStatusAbandoned
    TaskStatusGHFailed         = constants.TaskStatusGHFailed
    TaskStatusCIFailed         = constants.TaskStatusCIFailed
    TaskStatusCITimeout        = constants.TaskStatusCITimeout

    WorkspaceStatusActive  = constants.WorkspaceStatusActive
    WorkspaceStatusPaused  = constants.WorkspaceStatusPaused
    WorkspaceStatusRetired = constants.WorkspaceStatusRetired
)
```

### Testing Strategy

**JSON Round-Trip Tests (CRITICAL):**
```go
func TestTask_JSONSerialization(t *testing.T) {
    task := Task{
        ID:          "task-20251227-100000",
        WorkspaceID: "auth-workspace",
        // ... all fields
    }

    // Marshal to JSON
    data, err := json.Marshal(task)
    require.NoError(t, err)

    // Verify snake_case keys are present
    assert.Contains(t, string(data), `"task_id"`)  // NOT "taskId"
    assert.Contains(t, string(data), `"workspace_id"`)
    assert.Contains(t, string(data), `"current_step"`)

    // Unmarshal back
    var decoded Task
    err = json.Unmarshal(data, &decoded)
    require.NoError(t, err)

    assert.Equal(t, task.ID, decoded.ID)
}
```

**Example JSON Document (for test documentation):**
```json
{
    "id": "task-20251227-100000",
    "workspace_id": "auth-workspace",
    "template_id": "bugfix",
    "description": "Fix null pointer in parseConfig",
    "status": "running",
    "current_step": 2,
    "steps": [
        {
            "name": "analyze",
            "type": "ai",
            "status": "completed",
            "started_at": "2025-12-27T10:00:00Z",
            "completed_at": "2025-12-27T10:05:00Z",
            "attempts": 1
        }
    ],
    "created_at": "2025-12-27T10:00:00Z",
    "updated_at": "2025-12-27T10:05:00Z",
    "schema_version": 1
}
```

### Previous Story Intelligence

From Story 1-2 (constants) and Story 1-3 (errors) completion:
- Constants package established `TaskStatus` and `WorkspaceStatus` types with String() methods
- All code must pass `magex format:fix`, `magex lint`, `magex test`
- Table-driven tests are the preferred pattern
- Comprehensive documentation is required for all exported items
- Package structure uses lowercase filenames

### Git Commit Patterns

Recent commits follow conventional commits format:
- `feat(errors): add centralized error handling package`
- `feat(constants): add centralized constants package`

For this story, use:
- `feat(domain): add Task, Step, and StepResult types`
- `feat(domain): add Workspace and TaskRef types`
- `feat(domain): add Template and StepDefinition types`
- `feat(domain): add AIRequest and AIResult types`

Or a single commit:
- `feat(domain): add centralized domain types package`

### Project Structure Notes

Current structure after Story 1-3:
```
internal/
├── constants/
│   ├── constants.go      # File/dir/timeout/retry constants
│   ├── status.go         # TaskStatus, WorkspaceStatus types
│   ├── paths.go          # Path-related constants
│   └── status_test.go    # Tests
├── errors/
│   ├── errors.go         # Sentinel errors
│   ├── wrap.go           # Wrap utility
│   ├── user.go           # User-facing formatting
│   └── errors_test.go    # Tests
├── domain/
│   └── .gitkeep          # Will be replaced by actual files
└── ...
```

After this story:
```
internal/
├── constants/
│   └── ... (unchanged)
├── errors/
│   └── ... (unchanged)
├── domain/
│   ├── task.go           # Task, Step, StepResult
│   ├── workspace.go      # Workspace, TaskRef
│   ├── template.go       # Template, StepDefinition, StepType
│   ├── ai.go             # AIRequest, AIResult
│   ├── status.go         # Re-exports from constants
│   └── domain_test.go    # JSON serialization tests
└── ...
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure]
- [Source: _bmad-output/planning-artifacts/architecture.md#JSON Field Naming]
- [Source: _bmad-output/planning-artifacts/architecture.md#Domain Package]
- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns & Consistency Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.4]
- [Source: _bmad-output/project-context.md#JSON & Logging Conventions]
- [Source: _bmad-output/project-context.md#Package Import Rules]

### Validation Commands (MUST RUN BEFORE COMPLETION)

```bash
magex format:fix    # Format code
magex lint          # Must pass with 0 issues
magex test          # Must pass all tests
go build ./...      # Must compile
```

### Anti-Patterns to Avoid

```go
// ❌ NEVER: Import other internal packages besides constants/errors
import "github.com/mrz1836/atlas/internal/config"    // DON'T
import "github.com/mrz1836/atlas/internal/task"      // DON'T

// ❌ NEVER: Use camelCase in JSON tags
`json:"taskId"`          // DON'T - use "task_id"
`json:"createdAt"`       // DON'T - use "created_at"
`json:"workspaceId"`     // DON'T - use "workspace_id"

// ❌ NEVER: Forget schema_version field
type Task struct {
    ID string `json:"id"`
    // Missing: SchemaVersion int `json:"schema_version"`
}

// ❌ NEVER: Use pointer receivers for simple value types
func (s *StepType) String() string  // DON'T - use value receiver

// ❌ NEVER: Forget omitempty for optional fields
CompletedAt time.Time `json:"completed_at"`  // DON'T - use omitempty for nullable

// ✅ DO: Use snake_case for all JSON fields
`json:"task_id"`
`json:"created_at"`
`json:"workspace_id"`

// ✅ DO: Include schema_version in all state structs
SchemaVersion int `json:"schema_version"`

// ✅ DO: Use omitempty for optional/nullable fields
CompletedAt *time.Time `json:"completed_at,omitempty"`
Error       string     `json:"error,omitempty"`

// ✅ DO: Use value receivers for String() methods
func (s StepType) String() string { return string(s) }
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

- Initial lint failure: float comparison in test required InDelta instead of Equal

### Completion Notes List

- ✅ Created `internal/domain/task.go` with Task, Step, StepResult, and TaskConfig types
- ✅ Created `internal/domain/workspace.go` with Workspace and TaskRef types
- ✅ Created `internal/domain/template.go` with Template, StepDefinition, StepType, and TemplateVariable types
- ✅ Created `internal/domain/ai.go` with AIRequest and AIResult types
- ✅ Created `internal/domain/status.go` with type aliases and constant re-exports from constants package
- ✅ Created `internal/domain/domain_test.go` with comprehensive JSON serialization tests including:
  - Round-trip tests for all major types
  - snake_case verification (ensuring no camelCase in JSON output)
  - omitempty field behavior verification
  - Example JSON parsing tests
  - Status re-export verification
  - StepType.String() method tests
- ✅ All JSON tags use snake_case per architecture requirement
- ✅ All types include SchemaVersion field where required
- ✅ Removed `.gitkeep` placeholder file
- ✅ All validation commands pass: `go build ./...`, `magex format:fix`, `magex lint`, `magex test`

### File List

- internal/domain/task.go (new)
- internal/domain/workspace.go (new)
- internal/domain/template.go (new)
- internal/domain/ai.go (new)
- internal/domain/status.go (new)
- internal/domain/domain_test.go (new)
- internal/domain/.gitkeep (deleted)
- _bmad-output/implementation-artifacts/sprint-status.yaml (modified)

### Senior Developer Review (AI)

**Reviewer:** Claude Opus 4.5
**Date:** 2025-12-27
**Outcome:** ✅ APPROVED

**Review Summary:**
- All 8 Acceptance Criteria verified as FULLY IMPLEMENTED
- All 7 Tasks verified as ACTUALLY COMPLETED
- Validation commands pass: `go build`, `magex lint` (0 issues), `magex test` (100% coverage)
- No security vulnerabilities, no forbidden imports, all JSON tags use snake_case

**Issues Found & Fixed:**
1. [MEDIUM] Story File List was missing sprint-status.yaml modification → FIXED
2. [MEDIUM] Missing TaskConfig JSON serialization tests → FIXED (added TestTaskConfig_JSONSerialization, TestTaskConfig_OmitemptyFields)
3. [MEDIUM] Missing TemplateVariable JSON serialization tests → FIXED (added TestTemplateVariable_JSONSerialization, TestTemplateVariable_OmitemptyFields)

**Files Modified During Review:**
- internal/domain/domain_test.go (added 4 new test functions)
- _bmad-output/implementation-artifacts/1-4-create-domain-types-package.md (updated File List, added review notes)

### Change Log

- 2025-12-27: Code review completed - 3 medium issues fixed, status updated to done
- 2025-12-27: Implemented complete domain types package with all required structs, comprehensive tests, and documentation

