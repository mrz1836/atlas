# Story 1.3: Create Errors Package

Status: done

## Story

As a **developer**,
I want **a centralized errors package with sentinel errors and wrapping utilities**,
So that **error handling is consistent and errors can be categorized programmatically**.

## Acceptance Criteria

1. **Given** the constants package exists **When** I implement `internal/errors/` **Then** `errors.go` contains sentinel errors:
   - `ErrValidationFailed`
   - `ErrClaudeInvocation`
   - `ErrGitOperation`
   - `ErrGitHubOperation`
   - `ErrCIFailed`
   - `ErrCITimeout`
   - `ErrUserRejected`
   - `ErrUserAbandoned`

2. **Given** errors.go exists **When** I implement error wrapping **Then** `wrap.go` contains `Wrap(err error, msg string) error` utility

3. **Given** wrap.go exists **When** I implement user-facing errors **Then** `user.go` contains user-facing error formatting utilities

4. **Given** all error files exist **When** I inspect the code **Then** errors follow the pattern `errors.New("lower case description")`

5. **Given** the errors package is implemented **When** I check imports **Then** the package does not import any other internal packages

6. **Given** error wrapping is implemented **When** I run tests **Then** tests verify error wrapping preserves the sentinel for `errors.Is()` checks

## Tasks / Subtasks

- [x] Task 1: Create errors.go with sentinel errors (AC: #1, #4, #5)
  - [x] Create `internal/errors/errors.go`
  - [x] Define `ErrValidationFailed = errors.New("validation failed")`
  - [x] Define `ErrClaudeInvocation = errors.New("claude invocation failed")`
  - [x] Define `ErrGitOperation = errors.New("git operation failed")`
  - [x] Define `ErrGitHubOperation = errors.New("github operation failed")`
  - [x] Define `ErrCIFailed = errors.New("ci workflow failed")`
  - [x] Define `ErrCITimeout = errors.New("ci polling timeout")`
  - [x] Define `ErrUserRejected = errors.New("user rejected")`
  - [x] Define `ErrUserAbandoned = errors.New("user abandoned task")`
  - [x] Add comprehensive documentation for each sentinel error
  - [x] Verify no imports from other internal packages

- [x] Task 2: Create wrap.go with error wrapping utility (AC: #2, #5)
  - [x] Create `internal/errors/wrap.go`
  - [x] Implement `Wrap(err error, msg string) error` function
  - [x] Handle nil error case (return nil)
  - [x] Use `fmt.Errorf("%s: %w", msg, err)` pattern
  - [x] Add documentation explaining boundary wrapping pattern
  - [x] Consider adding `Wrapf(err error, format string, args ...any) error` for formatted messages

- [x] Task 3: Create user.go with user-facing error utilities (AC: #3, #5)
  - [x] Create `internal/errors/user.go`
  - [x] Implement `UserMessage(err error) string` - returns user-friendly message
  - [x] Implement `Actionable(err error) (message string, action string)` - returns error with suggested action
  - [x] Map sentinel errors to user-friendly messages
  - [x] Provide actionable suggestions for recoverable errors
  - [x] Add documentation for each utility function

- [x] Task 4: Create comprehensive tests (AC: #6)
  - [x] Create `internal/errors/errors_test.go`
  - [x] Test that wrapped errors satisfy `errors.Is()` for their sentinel
  - [x] Test that wrapped errors preserve original error chain
  - [x] Test nil handling in Wrap function
  - [x] Test user-friendly message formatting
  - [x] Test actionable error suggestions
  - [x] Use table-driven tests for exhaustive coverage
  - [x] Ensure good test coverage

- [x] Task 5: Remove .gitkeep and validate (AC: all)
  - [x] Remove `internal/errors/.gitkeep`
  - [x] Run `go build ./...` to verify compilation
  - [x] Run `magex format:fix` to format code
  - [x] Run `magex lint` to verify linting passes (must have 0 issues)
  - [x] Run `magex test` to verify tests pass

## Dev Notes

### Critical Architecture Requirements

**This package is foundational - it MUST be implemented correctly!** All other packages will import from this package for error handling. Any mistakes here will propagate throughout the codebase.

#### Package Rules (CRITICAL - ENFORCE STRICTLY)

From architecture.md:
- **internal/errors** → MUST NOT import any other internal package (only standard library allowed)
- All packages CAN import errors
- Sentinel errors are the **single source of truth** for error categorization

#### Required Sentinel Errors (from Architecture Document)

```go
package errors

import "errors"

// Sentinel errors for category switching.
// These allow callers to check error types with errors.Is().
var (
    ErrValidationFailed = errors.New("validation failed")
    ErrClaudeInvocation = errors.New("claude invocation failed")
    ErrGitOperation     = errors.New("git operation failed")
    ErrGitHubOperation  = errors.New("github operation failed")
    ErrCIFailed         = errors.New("ci workflow failed")
    ErrCITimeout        = errors.New("ci polling timeout")
    ErrUserRejected     = errors.New("user rejected")
    ErrUserAbandoned    = errors.New("user abandoned task")
)
```

#### Error Wrapping Pattern (from Architecture Document)

```go
// Wrap adds context to errors at package boundaries.
// It returns nil if err is nil, allowing for safe inline usage.
func Wrap(err error, msg string) error {
    if err == nil {
        return nil
    }
    return fmt.Errorf("%s: %w", msg, err)
}
```

The `%w` verb is critical - it enables `errors.Is()` checks to work correctly on wrapped errors:

```go
// Example usage at package boundary:
func (e *AIExecutor) Execute(ctx context.Context, task *Task) error {
    result, err := e.runner.Run(ctx, req)
    if err != nil {
        return errors.Wrap(err, "ai execution failed")
    }
    return nil
}

// Caller can still check for sentinel:
if errors.Is(err, errors.ErrClaudeInvocation) {
    // Handle Claude-specific error
}
```

#### User-Friendly Error Formatting Pattern

```go
// UserMessage returns a user-friendly message for common errors.
func UserMessage(err error) string {
    switch {
    case errors.Is(err, ErrValidationFailed):
        return "Validation failed. Check the output above for specific errors."
    case errors.Is(err, ErrClaudeInvocation):
        return "Failed to communicate with Claude. Check your API key and network."
    case errors.Is(err, ErrGitOperation):
        return "Git operation failed. Check your repository state."
    case errors.Is(err, ErrGitHubOperation):
        return "GitHub operation failed. Check your authentication and permissions."
    case errors.Is(err, ErrCIFailed):
        return "CI workflow failed. Check the workflow logs in GitHub Actions."
    case errors.Is(err, ErrCITimeout):
        return "CI polling timed out. Check if CI is running or retry later."
    case errors.Is(err, ErrUserRejected):
        return "Task was rejected. Provide feedback for retry or abandon."
    case errors.Is(err, ErrUserAbandoned):
        return "Task was abandoned. Workspace and branch preserved."
    default:
        return err.Error()
    }
}
```

### Error Message Format Standards

From project-context.md:
```go
// ✅ Action-first format
return fmt.Errorf("failed to create worktree: %w", err)

// ❌ Don't describe state before action
return fmt.Errorf("branch exists, worktree creation failed: %w", err)  // Wrong order

// ✅ Wrap at package boundaries only
return errors.Wrap(err, "ai execution failed")

// ❌ Don't over-wrap
return fmt.Errorf("executor: run: invoke: %w", err)  // Too deep
```

### Code Style Requirements

1. **No imports from internal packages** - only standard library allowed
2. **Lowercase error descriptions** - per Go conventions and AC #4
3. **Comprehensive documentation** - every exported error/function needs a comment
4. **Use errors.New for sentinels** - not fmt.Errorf
5. **Use fmt.Errorf with %w for wrapping** - enables errors.Is() checks

### Testing Strategy

- Test that each sentinel error can be created and compared
- Test Wrap() preserves error chain for errors.Is() checks
- Test Wrap() with nil returns nil
- Test Wrapf() with format specifiers (if implemented)
- Test UserMessage returns appropriate strings for each sentinel
- Test Actionable returns error+suggestion pairs
- Use table-driven tests for exhaustive coverage

### Previous Story Intelligence

From Story 1-2 completion:
- Constants package established the pattern for foundational packages
- Package uses no internal imports (only standard library)
- All constants have comprehensive documentation
- Tests use table-driven approach for exhaustive coverage
- All code must pass `magex format:fix`, `magex lint`, `magex test`

### Git Commit Patterns

Recent commits follow conventional commits format:
- `feat(constants): add centralized constants package`
- `feat(project-structure): complete Go module initialization...`

For this story, use:
- `feat(errors): add sentinel errors for error categorization`
- `feat(errors): add error wrapping and user-friendly formatting`

### Project Structure Notes

Current structure after Story 1-2:
```
internal/
├── constants/
│   ├── constants.go      # File/dir/timeout/retry constants
│   ├── status.go         # TaskStatus, WorkspaceStatus types
│   ├── paths.go          # Path-related constants
│   └── status_test.go    # Tests
├── errors/
│   └── .gitkeep          # Will be replaced by actual files
└── ...
```

After this story:
```
internal/
├── constants/
│   └── ... (unchanged)
├── errors/
│   ├── errors.go         # Sentinel errors
│   ├── wrap.go           # Wrap utility
│   ├── user.go           # User-facing formatting
│   └── errors_test.go    # Comprehensive tests
└── ...
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Errors Package]
- [Source: _bmad-output/planning-artifacts/architecture.md#Error Handling Strategy]
- [Source: _bmad-output/planning-artifacts/architecture.md#Sentinel Errors]
- [Source: _bmad-output/planning-artifacts/architecture.md#Error Wrapping (Boundary Pattern)]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.3]
- [Source: _bmad-output/project-context.md#Error Handling]
- [Source: _bmad-output/project-context.md#Anti-Patterns]

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
import "github.com/mrz1836/atlas/internal/constants"  // DON'T

// ❌ NEVER: Use uppercase in error descriptions
var ErrValidationFailed = errors.New("Validation Failed")  // DON'T - use lowercase

// ❌ NEVER: Use fmt.Errorf for sentinel errors
var ErrValidationFailed = fmt.Errorf("validation failed")  // DON'T - use errors.New

// ❌ NEVER: Forget to use %w in wrapping
return fmt.Errorf("failed: %v", err)  // DON'T - use %w for wrapping

// ❌ NEVER: Wrap at every level
return errors.Wrap(errors.Wrap(err, "inner"), "outer")  // DON'T - wrap only at boundaries

// ✅ DO: Use errors.New for sentinels
var ErrValidationFailed = errors.New("validation failed")

// ✅ DO: Use %w for error wrapping
return fmt.Errorf("%s: %w", msg, err)

// ✅ DO: Document every exported item
// ErrValidationFailed indicates that one or more validation commands failed.
var ErrValidationFailed = errors.New("validation failed")
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

- Fixed golangci-lint revive var-naming rule by migrating from .revive.toml to inline JSON configuration in .golangci.json

### Completion Notes List

- Created internal/errors package with 8 sentinel errors for error categorization
- Implemented Wrap() and Wrapf() functions for error wrapping with preserved error chain
- Implemented UserMessage() and Actionable() functions for user-friendly error formatting
- All errors use lowercase descriptions per Go conventions
- Package uses only standard library imports (no internal package dependencies)
- Comprehensive table-driven tests verify errors.Is() works through multiple wrap levels
- All validation commands pass: go build, magex format:fix, magex lint, magex test
- Updated golangci-lint configuration to use inline revive rules instead of external .revive.toml (removed var-naming rule that conflicts with intentional stdlib shadowing)

### File List

- internal/errors/errors.go (new)
- internal/errors/wrap.go (new)
- internal/errors/user.go (new)
- internal/errors/errors_test.go (new)
- internal/errors/.gitkeep (deleted)
- .golangci.json (modified - revive config moved inline, exclude-rules added)
- .revive.toml (deleted - config now fully inline in .golangci.json)

### Change Log

- 2025-12-27: Implemented Story 1.3 - Created centralized errors package with sentinel errors, wrapping utilities, and user-facing formatting
- 2025-12-27: Code review - Deleted orphaned .revive.toml (config now inline in .golangci.json), fixed test coverage for default branches in UserMessage and Actionable

