# Story 6.4: Push to Remote

Status: complete

## Story

As a **user**,
I want **ATLAS to push my branch to remote automatically**,
So that **my work is backed up and ready for PR creation**.

## Acceptance Criteria

1. **Given** commits exist on the local branch, **When** the git_push step executes, **Then** the system runs `git push -u origin <branch>`

2. **Given** a push is executed with setUpstream=true, **When** the push succeeds, **Then** the upstream tracking is set for the branch

3. **Given** push fails due to authentication error, **When** the error is detected, **Then** the task transitions to `gh_failed` state

4. **Given** a transient network failure occurs, **When** the push fails, **Then** retry logic with exponential backoff executes (3 attempts max)

5. **Given** a network timeout occurs, **When** the timeout is detected, **Then** the timeout is handled gracefully with appropriate error message

6. **Given** push progress is available, **When** the push is in progress, **Then** push progress is displayed to the user (if available)

7. **Given** config has `auto_proceed_git: false`, **When** the push step is about to execute, **Then** the system pauses for user confirmation before push

## Tasks / Subtasks

- [x] Task 1: Create `internal/git/push.go` with PushService (AC: 1, 2, 3, 4, 5, 6, 7)
  - [x] 1.1: Define PushService interface:
    - `Push(ctx context.Context, opts PushOptions) (*PushResult, error)`
  - [x] 1.2: Define PushOptions struct with Remote, Branch, SetUpstream, ProgressCallback fields
  - [x] 1.3: Define PushResult struct with Success, Upstream, ErrorType, Attempts fields
  - [x] 1.4: Define PushErrorType enum (None, Auth, Network, Timeout, Other)

- [x] Task 2: Implement PushRunner with retry logic (AC: 1, 4)
  - [x] 2.1: Create PushRunner struct with Runner dependency (from Story 6.1)
  - [x] 2.2: Implement exponential backoff retry logic (3 attempts, 2s initial, 2.0 multiplier)
  - [x] 2.3: Use shared retry utility if available, otherwise implement per architecture pattern
  - [x] 2.4: Log each attempt with attempt number, delay, error reason

- [x] Task 3: Implement error classification (AC: 3, 5)
  - [x] 3.1: Create `classifyPushError(err error) PushErrorType` function
  - [x] 3.2: Detect authentication errors from git output patterns:
    - "Authentication failed"
    - "could not read Username"
    - "Permission denied"
    - "fatal: Authentication failed"
  - [x] 3.3: Detect network errors from patterns:
    - "Could not resolve host"
    - "Connection refused"
    - "Network is unreachable"
    - "Connection timed out"
  - [x] 3.4: Detect timeout errors (context.DeadlineExceeded)
  - [x] 3.5: Return appropriate sentinel error for state machine transition

- [x] Task 4: Implement progress tracking (AC: 6)
  - [x] 4.1: Define ProgressCallback function type
  - [x] 4.2: Explore git push progress options (--progress flag)
  - [x] 4.3: If progress available, stream to callback
  - [x] 4.4: If progress not available, provide indeterminate progress indication

- [x] Task 5: Implement confirmation flow (AC: 7)
  - [x] 5.1: Add `ConfirmBeforePush` option to PushOptions
  - [x] 5.2: Create ConfirmPushCallback function type for TUI integration
  - [x] 5.3: If callback provided and ConfirmBeforePush=true, call callback before push
  - [x] 5.4: Handle callback cancel response (return ErrOperationCanceled)

- [x] Task 6: Add new sentinel errors (AC: 3)
  - [x] 6.1: Add `ErrPushAuthFailed` to internal/errors/errors.go
  - [x] 6.2: Add `ErrPushNetworkFailed` to internal/errors/errors.go
  - [x] 6.3: Document errors with clear descriptions

- [x] Task 7: Create comprehensive tests (AC: 1-7)
  - [x] 7.1: Test successful push
  - [x] 7.2: Test push with setUpstream
  - [x] 7.3: Test auth error classification
  - [x] 7.4: Test network error classification
  - [x] 7.5: Test timeout error classification
  - [x] 7.6: Test retry logic with mock failures
  - [x] 7.7: Test confirmation callback flow
  - [x] 7.8: Test context cancellation
  - [x] 7.9: Target 90%+ coverage

## Dev Notes

### Existing Code to Reuse/Extend

**CRITICAL: Reuse GitRunner.Push from Story 6.1**

The git package already has the basic Push method:

```go
// internal/git/runner.go - already implemented
type Runner interface {
    Push(ctx context.Context, remote, branch string, setUpstream bool) error
    // ... other methods
}

// internal/git/git_runner.go:112-133 - already implemented
func (r *CLIRunner) Push(ctx context.Context, remote, branch string, setUpstream bool) error {
    // Basic implementation exists - needs wrapping with retry
}
```

**CRITICAL: Use retry pattern from architecture**

From `architecture.md`:

```go
type RetryConfig struct {
    MaxAttempts  int           // Default: 3
    InitialDelay time.Duration // Default: 1s (use 2s for push)
    MaxDelay     time.Duration // Default: 30s
    Multiplier   float64       // Default: 2.0
}
```

### Push Error Patterns to Detect

Git push can fail in several ways. Error classification should check stderr output:

**Authentication Errors (transition to gh_failed):**
```
remote: Invalid username or password.
fatal: Authentication failed for 'https://github.com/...'

fatal: could not read Username for 'https://github.com': terminal prompts disabled

Permission denied (publickey).
fatal: Could not read from remote repository.
```

**Network Errors (retry with backoff):**
```
fatal: unable to access 'https://github.com/...': Could not resolve host: github.com

fatal: unable to access 'https://github.com/...': Failed to connect to github.com port 443: Connection refused

ssh: connect to host github.com port 22: Network is unreachable

fatal: unable to access 'https://github.com/...': Operation timed out after 30001 milliseconds
```

### Implementation Pattern

```go
// internal/git/push.go

package git

import (
    "context"
    "fmt"
    "strings"
    "time"

    atlaserrors "github.com/mrz1836/atlas/internal/errors"
    "github.com/rs/zerolog"
)

// PushErrorType classifies push failures for appropriate handling.
type PushErrorType int

const (
    PushErrorNone PushErrorType = iota
    PushErrorAuth     // Authentication failed - don't retry
    PushErrorNetwork  // Network issue - retry with backoff
    PushErrorTimeout  // Timeout - retry with backoff
    PushErrorOther    // Other error - don't retry
)

// PushOptions configures the push operation.
type PushOptions struct {
    Remote            string
    Branch            string
    SetUpstream       bool
    ConfirmBeforePush bool
    ConfirmCallback   func(remote, branch string) (bool, error)
    ProgressCallback  func(progress string)
}

// PushResult contains the outcome of a push operation.
type PushResult struct {
    Success   bool
    Upstream  string // e.g., "origin/feat/new-feature"
    ErrorType PushErrorType
    Attempts  int
    FinalErr  error
}

// PushService provides high-level push operations with retry.
type PushService interface {
    Push(ctx context.Context, opts PushOptions) (*PushResult, error)
}

// Compile-time interface check
var _ PushService = (*PushRunner)(nil)

// PushRunner implements PushService using the git Runner.
type PushRunner struct {
    runner Runner
    logger zerolog.Logger
    config RetryConfig
}

// NewPushRunner creates a PushRunner with the given git runner.
func NewPushRunner(runner Runner, opts ...PushRunnerOption) *PushRunner {
    pr := &PushRunner{
        runner: runner,
        logger: zerolog.Nop(),
        config: RetryConfig{
            MaxAttempts:  3,
            InitialDelay: 2 * time.Second,
            MaxDelay:     30 * time.Second,
            Multiplier:   2.0,
        },
    }
    for _, opt := range opts {
        opt(pr)
    }
    return pr
}

// PushRunnerOption configures a PushRunner.
type PushRunnerOption func(*PushRunner)

// WithLogger sets the logger for push operations.
func WithPushLogger(logger zerolog.Logger) PushRunnerOption {
    return func(pr *PushRunner) {
        pr.logger = logger
    }
}

// WithRetryConfig sets custom retry configuration.
func WithPushRetryConfig(config RetryConfig) PushRunnerOption {
    return func(pr *PushRunner) {
        pr.config = config
    }
}
```

### Retry Logic Pattern

```go
func (p *PushRunner) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Handle confirmation callback
    if opts.ConfirmBeforePush && opts.ConfirmCallback != nil {
        confirmed, err := opts.ConfirmCallback(opts.Remote, opts.Branch)
        if err != nil {
            return nil, fmt.Errorf("failed to confirm push: %w", err)
        }
        if !confirmed {
            return nil, atlaserrors.ErrOperationCanceled
        }
    }

    result := &PushResult{Remote: opts.Remote, Branch: opts.Branch}
    delay := p.config.InitialDelay

    for attempt := 1; attempt <= p.config.MaxAttempts; attempt++ {
        result.Attempts = attempt

        p.logger.Info().
            Int("attempt", attempt).
            Str("remote", opts.Remote).
            Str("branch", opts.Branch).
            Msg("pushing to remote")

        err := p.runner.Push(ctx, opts.Remote, opts.Branch, opts.SetUpstream)
        if err == nil {
            result.Success = true
            if opts.SetUpstream {
                result.Upstream = fmt.Sprintf("%s/%s", opts.Remote, opts.Branch)
            }
            p.logger.Info().
                Int("attempts", attempt).
                Str("upstream", result.Upstream).
                Msg("push succeeded")
            return result, nil
        }

        // Classify the error
        errType := classifyPushError(err)
        result.ErrorType = errType
        result.FinalErr = err

        p.logger.Warn().
            Err(err).
            Int("attempt", attempt).
            Str("error_type", errType.String()).
            Msg("push failed")

        // Don't retry auth errors or unknown errors
        if errType == PushErrorAuth || errType == PushErrorOther {
            break
        }

        // Check if we should retry
        if attempt < p.config.MaxAttempts {
            // Check context before sleeping
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
            }

            // Increase delay for next attempt
            delay = time.Duration(float64(delay) * p.config.Multiplier)
            if delay > p.config.MaxDelay {
                delay = p.config.MaxDelay
            }
        }
    }

    // All retries exhausted or non-retryable error
    switch result.ErrorType {
    case PushErrorAuth:
        return result, fmt.Errorf("authentication failed: %w", atlaserrors.ErrPushAuthFailed)
    case PushErrorNetwork, PushErrorTimeout:
        return result, fmt.Errorf("push failed after %d attempts: %w", result.Attempts, atlaserrors.ErrMaxRetriesExceeded)
    default:
        return result, fmt.Errorf("push failed: %w", result.FinalErr)
    }
}
```

### Error Classification

```go
func classifyPushError(err error) PushErrorType {
    if err == nil {
        return PushErrorNone
    }

    // Check for timeout
    if errors.Is(err, context.DeadlineExceeded) {
        return PushErrorTimeout
    }

    errStr := strings.ToLower(err.Error())

    // Authentication errors
    authPatterns := []string{
        "authentication failed",
        "could not read username",
        "permission denied",
        "invalid username or password",
        "access denied",
    }
    for _, pattern := range authPatterns {
        if strings.Contains(errStr, pattern) {
            return PushErrorAuth
        }
    }

    // Network errors
    networkPatterns := []string{
        "could not resolve host",
        "connection refused",
        "network is unreachable",
        "connection timed out",
        "operation timed out",
        "unable to access",
        "no route to host",
    }
    for _, pattern := range networkPatterns {
        if strings.Contains(errStr, pattern) {
            return PushErrorNetwork
        }
    }

    return PushErrorOther
}

func (t PushErrorType) String() string {
    switch t {
    case PushErrorNone:
        return "none"
    case PushErrorAuth:
        return "auth"
    case PushErrorNetwork:
        return "network"
    case PushErrorTimeout:
        return "timeout"
    default:
        return "other"
    }
}
```

### Project Structure Notes

**File Locations:**
- `internal/git/push.go` - PushService interface, PushRunner implementation
- `internal/git/push_test.go` - Comprehensive tests for push service
- `internal/errors/errors.go` - Add ErrPushAuthFailed, ErrPushNetworkFailed

**Import Rules (from architecture.md):**
- `internal/git` can import: constants, errors, domain
- `internal/git` cannot import: task, workspace, cli, validation, template, tui

### Context-First Pattern

From project-context.md:

```go
// ALWAYS: ctx as first parameter
func (p *PushRunner) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
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

Use existing sentinels and action-first format:

```go
import atlaserrors "github.com/mrz1836/atlas/internal/errors"

// Action-first error format
return nil, fmt.Errorf("failed to push to remote: %w", err)

// For push-specific errors, wrap with appropriate sentinel
return nil, fmt.Errorf("authentication failed: %w", atlaserrors.ErrPushAuthFailed)
```

### RetryConfig Type

Create or reuse from existing codebase:

```go
// RetryConfig configures retry behavior for operations.
type RetryConfig struct {
    MaxAttempts  int           // Maximum number of attempts (default: 3)
    InitialDelay time.Duration // Initial delay between retries (default: 2s)
    MaxDelay     time.Duration // Maximum delay cap (default: 30s)
    Multiplier   float64       // Delay multiplier per attempt (default: 2.0)
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

### Gitleaks Compliance (CRITICAL)

**Test values MUST NOT look like secrets:**
- Use semantic names: `ATLAS_TEST_PUSH_PATTERN`
- Avoid numeric suffixes that look like keys: `_12345`
- Use `mock_value_for_test` patterns

### References

- [Source: epics.md - Story 6.4: Push to Remote]
- [Source: architecture.md - Retry Strategy section]
- [Source: architecture.md - GitRunner Interface section]
- [Source: project-context.md - Context Handling (CRITICAL)]
- [Source: epic-6-implementation-notes.md - GitRunner Design]
- [Source: epic-6-user-scenarios.md - Scenario 1 steps 9, Scenario 5 step 17]
- [Source: internal/git/runner.go - Existing Runner interface]
- [Source: internal/git/git_runner.go - Existing Push implementation]
- [Source: internal/errors/errors.go - Existing error sentinels]
- [Source: 6-1-gitrunner-implementation.md - Story 6.1 learnings]
- [Source: 6-2-branch-creation-and-naming.md - Story 6.2 learnings]
- [Source: 6-3-smart-commit-system.md - Story 6.3 learnings]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoint 9 - push step)
- Scenario 5: Feature Workflow with Speckit SDD (checkpoint 17 - push step)

Specific validation checkpoints from scenarios:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| Push command | `git push -u origin <branch>` executed | AC1 |
| Upstream set | Tracking reference established | AC2 |
| Auth failure | Transition to gh_failed state | AC3 |
| Retry on network | 3 attempts with exponential backoff | AC4 |
| Timeout handling | Graceful error with clear message | AC5 |
| Progress display | Show push progress if available | AC6 |
| Confirmation | Pause if auto_proceed_git=false | AC7 |

### Previous Story Intelligence

**From Story 6.1 (GitRunner Implementation):**
- Runner renamed to follow Go best practices: `git.Runner` not `git.GitRunner`
- Shared `git.RunCommand` utility in `internal/git/command.go`
- Tests use `t.TempDir()` with temp git repos
- Test coverage at 91.8%
- Context cancellation check at method entry
- Errors wrapped with `atlaserrors.ErrGitOperation`
- Action-first error format: `failed to <action>: %w`

**From Story 6.2 (Branch Creation and Naming):**
- BranchCreatorService interface pattern established
- Shared `GenerateUniqueBranchNameWithChecker()` for code reuse
- Tests at 91.2% coverage
- zerolog logging for operations (success and failure)

**From Story 6.3 (Smart Commit System):**
- Functional options pattern: `WithTaskID`, `WithTemplateName`, etc.
- AI integration patterns if needed for progress feedback
- Comprehensive error sentinel additions to errors.go
- Tests at 92.7% coverage after code review fixes

### Git Intelligence (Recent Commits)

Recent commits in Epic 6 branch show patterns to follow:
- `feat(git): implement smart commit system with garbage detection` - SmartCommitRunner pattern
- `feat(git): add branch creation and naming system` - BranchCreatorService pattern
- `feat(git): implement GitRunner for git CLI operations` - Base Runner interface

File patterns established:
- Implementation: `internal/git/<feature>.go`
- Tests: `internal/git/<feature>_test.go`
- Interface + types in same file when small, separate when large

### Testing Strategy

**Unit Tests (mock git.Runner):**
```go
func TestPushRunner_Push_Success(t *testing.T) {
    mockRunner := &MockRunner{
        PushFunc: func(ctx context.Context, remote, branch string, setUpstream bool) error {
            return nil
        },
    }
    pr := NewPushRunner(mockRunner)
    result, err := pr.Push(context.Background(), PushOptions{
        Remote:      "origin",
        Branch:      "feat/test",
        SetUpstream: true,
    })
    require.NoError(t, err)
    assert.True(t, result.Success)
    assert.Equal(t, "origin/feat/test", result.Upstream)
}
```

**Integration Tests (real git repo):**
- Create temp repo with remote
- Push actual commits
- Verify branch tracking

**Error Classification Tests:**
```go
func TestClassifyPushError(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected PushErrorType
    }{
        {
            name:     "auth error",
            err:      errors.New("fatal: Authentication failed for 'https://github.com'"),
            expected: PushErrorAuth,
        },
        {
            name:     "network error",
            err:      errors.New("fatal: Could not resolve host: github.com"),
            expected: PushErrorNetwork,
        },
        {
            name:     "timeout",
            err:      context.DeadlineExceeded,
            expected: PushErrorTimeout,
        },
    }
    // ... test implementation
}
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

1. **Implementation Complete**: Created `internal/git/push.go` with full PushService implementation including:
   - `PushService` interface with `Push(ctx, opts)` method
   - `PushRunner` struct implementing retry logic with exponential backoff
   - `PushOptions` struct with Remote, Branch, SetUpstream, ConfirmBeforePush, ConfirmCallback, ProgressCallback
   - `PushResult` struct with Success, Upstream, ErrorType, Attempts, FinalErr
   - `PushErrorType` enum (None, Auth, Network, Timeout, Other)
   - Error classification via `classifyPushError()` with patterns for auth/network/timeout
   - Helper functions: `isAuthError()`, `isNetworkError()` for clean code organization

2. **Retry Logic**: Implemented exponential backoff with:
   - Default 3 max attempts
   - 2s initial delay
   - 2.0 multiplier
   - 30s max delay cap
   - Proper context cancellation handling during wait

3. **Error Classification**: Detects:
   - Auth errors: authentication failed, could not read username, permission denied, invalid username, access denied
   - Network errors: could not resolve host, connection refused, network unreachable, connection/operation timed out, unable to access, no route to host, failed to connect
   - Timeout: context.DeadlineExceeded

4. **Sentinel Errors Added**: `ErrPushAuthFailed` and `ErrPushNetworkFailed` in internal/errors/errors.go

5. **Tests**: Comprehensive test coverage including:
   - Success scenarios (with/without upstream)
   - Error classification tests for all error types
   - Retry logic with mock failures
   - Confirmation callback flow (approved/denied/error)
   - Progress callback verification
   - Context cancellation
   - Exponential backoff timing verification
   - MaxDelay cap verification

6. **Code Refactoring**: Broke down large Push function into smaller methods to reduce cognitive complexity:
   - `validateAndNormalizeOpts()`
   - `handleConfirmation()`
   - `executePushWithRetry()`
   - `attemptPush()`
   - `buildSuccessResult()`
   - `shouldRetry()`
   - `waitForRetry()`
   - `buildFinalError()`

7. **Validation**: All commands pass:
   - `magex format:fix` - Code formatted
   - `magex lint` - 0 issues
   - `magex test:race` - All tests pass
   - `go-pre-commit run --all-files` - All checks pass

### File List

1. `internal/git/push.go` - PushService interface and PushRunner implementation (394 lines)
2. `internal/git/push_test.go` - Comprehensive tests (941 lines)
3. `internal/git/integration_test.go` - Integration tests for real git push operations (added in code review)
4. `internal/errors/errors.go` - Added ErrPushAuthFailed and ErrPushNetworkFailed sentinels

### Code Review Notes (2025-12-30)

**Reviewer:** Claude Opus 4.5 (Adversarial Code Review Workflow)

**Issues Found:** 0 High, 3 Medium, 2 Low

**Fixes Applied:**
1. Removed dead code in `buildFinalError` - added defensive handling for `PushErrorNone` case to satisfy exhaustive linter
2. Added integration tests (`internal/git/integration_test.go`) for real git push operations with bare remote
3. Added documentation comment on `RetryConfig` for potential future refactoring to `internal/domain`
4. Added debug log when using default remote "origin"
5. Verified all Acceptance Criteria are properly implemented

**Validation Post-Review:**
- `magex format:fix` - Pass
- `magex lint` - 0 issues
- `magex test:race` - All tests pass (including integration tests)
- `go-pre-commit run --all-files` - All checks pass

**Coverage:** 93.4% overall for git package

