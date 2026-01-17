# Story 6.6: CI Status Monitoring

Status: done

## Story

As a **user**,
I want **ATLAS to wait for CI to pass after creating the PR**,
So that **I only review PRs that have passed automated checks**.

## Acceptance Criteria

1. **Given** a PR has been created, **When** the ci_wait step executes, **Then** the system polls GitHub Actions via `gh pr checks` to check status of configured required workflows

2. **Given** CI is being monitored, **When** polling, **Then** the polling interval is configurable (default: 2 minutes) and timeout is configurable (default: 30 minutes)

3. **Given** CI is running, **When** displaying progress, **Then** the system shows: "Waiting for CI... (5m elapsed, checking: CI, Lint)" with current status of each workflow

4. **Given** CI monitoring is active, **When** all required workflows pass, **Then** the task proceeds to the review step (AwaitingApproval state)

5. **Given** CI monitoring is active, **When** any required workflow fails, **Then** the system transitions to `ci_failed` state with error details

6. **Given** CI monitoring is active, **When** timeout is exceeded, **Then** the system transitions to `ci_timeout` state

7. **Given** CI status changes to pass or fail, **When** the transition occurs, **Then** the system emits terminal bell notification

8. **Given** the CI monitoring system, **When** checking status, **Then** retry logic with exponential backoff handles transient network failures (3 attempts max)

## Tasks / Subtasks

- [x] Task 1: Extend HubRunner interface with CI monitoring methods (AC: 1, 4, 5)
  - [x] 1.1: Add `WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error)` method to HubRunner interface
  - [x] 1.2: Define `CIWatchOptions` struct with PRNumber, Interval, Timeout, RequiredChecks fields
  - [x] 1.3: Define `CIWatchResult` struct with Status, CheckResults, ElapsedTime, Error fields
  - [x] 1.4: Define `CheckResult` struct with Name, State, Conclusion, URL, Duration fields
  - [x] 1.5: Define `CIStatus` type with constants (Pending, Success, Failure, Timeout)

- [x] Task 2: Implement `gh pr checks` execution (AC: 1, 3)
  - [x] 2.1: Create `fetchPRChecks(ctx context.Context, prNumber int) ([]CheckResult, error)` method
  - [x] 2.2: Execute `gh pr checks <number> --json name,state,bucket,completedAt,startedAt,description,workflow`
  - [x] 2.3: Parse JSON output into []CheckResult slice
  - [x] 2.4: Map bucket field to internal CheckState enum (pass, fail, pending, skipping, cancel)
  - [x] 2.5: Calculate individual check duration from startedAt/completedAt

- [x] Task 3: Implement polling loop with configurable interval (AC: 2, 3)
  - [x] 3.1: Create `WatchPRChecks` method in CLIGitHubRunner
  - [x] 3.2: Implement polling loop with context cancellation support
  - [x] 3.3: Apply configurable interval (default 2 minutes) between polls
  - [x] 3.4: Track elapsed time from watch start
  - [x] 3.5: Implement timeout checking against configurable timeout (default 30 minutes)
  - [x] 3.6: Generate progress message: "Waiting for CI... ({elapsed} elapsed, checking: {check_names})" via FormatCIProgressMessage helper

- [x] Task 4: Implement required checks filtering (AC: 1, 4, 5)
  - [x] 4.1: Accept RequiredChecks []string in CIWatchOptions
  - [x] 4.2: If RequiredChecks is empty, monitor all checks
  - [x] 4.3: If RequiredChecks specified, filter to only those checks
  - [x] 4.4: Support wildcard matching for check names (e.g., "CI*" matches "CI" and "CI / lint")

- [x] Task 5: Implement status determination logic (AC: 4, 5, 6)
  - [x] 5.1: Create `determineOverallStatus(checks []CheckResult, requiredChecks []string) CIStatus`
  - [x] 5.2: Return Success only when ALL required checks have bucket="pass"
  - [x] 5.3: Return Failure immediately when ANY required check has bucket="fail" or bucket="cancel"
  - [x] 5.4: Return Pending while any required check has bucket="pending"
  - [x] 5.5: Handle "skipping" bucket appropriately (treat as pass for optional checks)

- [x] Task 6: Implement retry logic for transient failures (AC: 8)
  - [x] 6.1: Wrap gh pr checks calls with retry logic (reuse RetryConfig pattern from push.go)
  - [x] 6.2: Apply exponential backoff (3 attempts, 2s initial, 2.0 multiplier)
  - [x] 6.3: Only retry on network errors; fail immediately on auth/not-found
  - [x] 6.4: Distinguish between poll failures and actual CI failures

- [x] Task 7: Add progress reporting callback (AC: 3)
  - [x] 7.1: Define `CIProgressCallback func(elapsed time.Duration, checks []CheckResult)` type
  - [x] 7.2: Add ProgressCallback field to CIWatchOptions
  - [x] 7.3: Call callback after each poll with current status
  - [x] 7.4: Callback enables TUI to display real-time progress

- [x] Task 8: Implement terminal bell notification (AC: 7)
  - [x] 8.1: Add BellEnabled bool to CIWatchOptions
  - [x] 8.2: Emit "\a" (BEL character) when transitioning from pending to success/failure
  - [x] 8.3: Only emit bell once per watch session (not on every poll)

- [x] Task 9: Add sentinel errors to internal/errors (AC: 5, 6)
  - [x] 9.1: Add `ErrCIFailed` sentinel error (already exists, verify)
  - [x] 9.2: Add `ErrCITimeout` sentinel error (already exists, verify)
  - [x] 9.3: Add `ErrCICheckNotFound` sentinel for when required check doesn't exist

- [x] Task 10: Create comprehensive tests (AC: 1-8)
  - [x] 10.1: Test successful CI pass scenario (all checks pass)
  - [x] 10.2: Test CI failure scenario (one check fails)
  - [x] 10.3: Test timeout scenario (checks remain pending past timeout)
  - [x] 10.4: Test required checks filtering
  - [x] 10.5: Test wildcard matching for check names
  - [x] 10.6: Test retry logic on transient network failure
  - [x] 10.7: Test context cancellation during watch
  - [x] 10.8: Test progress callback invocation
  - [x] 10.9: Test bell notification on status change
  - [x] 10.10: Test empty checks scenario (no CI configured)
  - [x] 10.11: Target 90%+ coverage

## Dev Notes

### Existing Code to Reuse/Extend

**CRITICAL: Build on Story 6.5 HubRunner implementation**

The `internal/git/github.go` already has the foundation:

```go
// From internal/git/github.go - EXTEND THIS

// HubRunner interface - add WatchPRChecks method
type HubRunner interface {
    CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)
    GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)
    // ADD: WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error)
}

// PRStatus already has CI fields (from Story 6.5)
type PRStatus struct {
    Number     int
    State      string
    Mergeable  bool
    ChecksPass bool    // Derived from checks
    CIStatus   string  // pending, success, failure
}

// CLIGitHubRunner - implement WatchPRChecks here
type CLIGitHubRunner struct {
    workDir string
    logger  zerolog.Logger
    config  RetryConfig
    cmdExec CommandExecutor
}
```

**CRITICAL: Use existing RetryConfig pattern**

From `internal/git/push.go`:

```go
// RetryConfig - REUSE THIS
type RetryConfig struct {
    MaxAttempts  int           // Default: 3
    InitialDelay time.Duration // Default: 2s
    MaxDelay     time.Duration // Default: 30s
    Multiplier   float64       // Default: 2.0
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
    return RetryConfig{
        MaxAttempts:  3,
        InitialDelay: 2 * time.Second,
        MaxDelay:     30 * time.Second,
        Multiplier:   2.0,
    }
}
```

### gh pr checks Command Pattern

The most reliable way to monitor CI status:

```bash
# Get all checks as JSON
gh pr checks 42 --json name,state,bucket,completedAt,startedAt,description,workflow

# JSON output format:
[
  {
    "name": "CI / lint",
    "state": "SUCCESS",
    "bucket": "pass",
    "completedAt": "2025-12-30T10:00:00Z",
    "startedAt": "2025-12-30T09:58:00Z",
    "description": "Linting succeeded",
    "workflow": "CI"
  },
  {
    "name": "CI / test",
    "state": "PENDING",
    "bucket": "pending",
    "startedAt": "2025-12-30T09:58:00Z",
    "workflow": "CI"
  }
]

# Get only required checks
gh pr checks 42 --required --json name,state,bucket

# Watch mode (built-in, but we implement custom for better control)
gh pr checks 42 --watch --interval 30
```

**Bucket values mapping:**
- `pass` → CIStatusSuccess
- `fail` → CIStatusFailure
- `pending` → CIStatusPending
- `skipping` → CIStatusSkipped (treat as pass for optional)
- `cancel` → CIStatusFailure (workflow was cancelled)

### CIWatchOptions Design

```go
// CIWatchOptions configures CI monitoring.
type CIWatchOptions struct {
    // PRNumber is the PR to monitor (required).
    PRNumber int

    // Interval is the polling interval (default: 2 minutes).
    Interval time.Duration

    // Timeout is the maximum time to wait (default: 30 minutes).
    Timeout time.Duration

    // RequiredChecks filters to specific check names.
    // Empty means monitor all checks.
    // Supports wildcards: "CI*" matches "CI / lint", "CI / test"
    RequiredChecks []string

    // ProgressCallback is called after each poll with current status.
    ProgressCallback CIProgressCallback

    // BellEnabled emits terminal bell on status change.
    BellEnabled bool
}

// CIProgressCallback receives progress updates during CI watch.
type CIProgressCallback func(elapsed time.Duration, checks []CheckResult)
```

### CIWatchResult Design

```go
// CIWatchResult contains the outcome of CI monitoring.
type CIWatchResult struct {
    // Status is the final CI status (Success, Failure, Timeout).
    Status CIStatus

    // CheckResults contains individual check outcomes.
    CheckResults []CheckResult

    // ElapsedTime is total time spent monitoring.
    ElapsedTime time.Duration

    // Error contains details if Status is Failure or Timeout.
    Error error
}

// CheckResult contains the outcome of a single CI check.
type CheckResult struct {
    // Name is the check name (e.g., "CI / lint").
    Name string

    // State is the raw GitHub state (SUCCESS, FAILURE, PENDING).
    State string

    // Bucket is the categorized state (pass, fail, pending, skipping, cancel).
    Bucket string

    // Conclusion is the check conclusion if completed.
    Conclusion string

    // URL is the link to the check details.
    URL string

    // Duration is how long the check ran.
    Duration time.Duration

    // Workflow is the parent workflow name.
    Workflow string
}

// CIStatus represents the overall CI status.
type CIStatus int

const (
    CIStatusPending CIStatus = iota
    CIStatusSuccess
    CIStatusFailure
    CIStatusTimeout
)
```

### Implementation Pattern

```go
// WatchPRChecks monitors CI checks until completion or timeout.
func (r *CLIGitHubRunner) WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error) {
    // Check for cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Validate options
    if err := validateCIWatchOptions(&opts); err != nil {
        return nil, err
    }

    // Apply defaults
    if opts.Interval == 0 {
        opts.Interval = 2 * time.Minute
    }
    if opts.Timeout == 0 {
        opts.Timeout = 30 * time.Minute
    }

    result := &CIWatchResult{}
    startTime := time.Now()
    bellEmitted := false

    for {
        // Check timeout
        elapsed := time.Since(startTime)
        if elapsed > opts.Timeout {
            result.Status = CIStatusTimeout
            result.ElapsedTime = elapsed
            result.Error = atlaserrors.ErrCITimeout
            r.emitBellIfEnabled(opts.BellEnabled, &bellEmitted)
            return result, nil
        }

        // Fetch current check status with retry
        checks, err := r.fetchPRChecksWithRetry(ctx, opts.PRNumber)
        if err != nil {
            return nil, err
        }

        // Filter to required checks if specified
        filteredChecks := filterChecks(checks, opts.RequiredChecks)
        result.CheckResults = filteredChecks

        // Determine overall status
        status := determineOverallStatus(filteredChecks)
        result.Status = status
        result.ElapsedTime = elapsed

        // Call progress callback
        if opts.ProgressCallback != nil {
            opts.ProgressCallback(elapsed, filteredChecks)
        }

        // Check for terminal states
        switch status {
        case CIStatusSuccess:
            r.emitBellIfEnabled(opts.BellEnabled, &bellEmitted)
            return result, nil
        case CIStatusFailure:
            result.Error = atlaserrors.ErrCIFailed
            r.emitBellIfEnabled(opts.BellEnabled, &bellEmitted)
            return result, nil
        case CIStatusPending:
            // Wait for next poll
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(opts.Interval):
                // Continue polling
            }
        }
    }
}

func (r *CLIGitHubRunner) fetchPRChecksWithRetry(ctx context.Context, prNumber int) ([]CheckResult, error) {
    var checks []CheckResult
    var lastErr error
    delay := r.config.InitialDelay

    for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
        checks, lastErr = r.fetchPRChecks(ctx, prNumber)
        if lastErr == nil {
            return checks, nil
        }

        errType := classifyGHError(lastErr)
        if !shouldRetryPR(errType) {
            return nil, lastErr
        }

        if attempt < r.config.MaxAttempts {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delay):
            }
            delay = time.Duration(float64(delay) * r.config.Multiplier)
            if delay > r.config.MaxDelay {
                delay = r.config.MaxDelay
            }
        }
    }

    return nil, lastErr
}

func (r *CLIGitHubRunner) fetchPRChecks(ctx context.Context, prNumber int) ([]CheckResult, error) {
    args := []string{
        "pr", "checks", strconv.Itoa(prNumber),
        "--json", "name,state,bucket,completedAt,startedAt,description,workflow,link",
    }

    output, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
    if err != nil {
        return nil, fmt.Errorf("failed to get PR checks: %w", err)
    }

    return parseCheckResults(output)
}

func (r *CLIGitHubRunner) emitBellIfEnabled(enabled bool, emitted *bool) {
    if enabled && !*emitted {
        fmt.Print("\a") // BEL character
        *emitted = true
    }
}
```

### Wildcard Matching for Check Names

```go
import "path/filepath"

// filterChecks filters checks by required check names with wildcard support.
func filterChecks(checks []CheckResult, required []string) []CheckResult {
    if len(required) == 0 {
        return checks // No filter, return all
    }

    var filtered []CheckResult
    for _, check := range checks {
        if matchesAnyPattern(check.Name, required) {
            filtered = append(filtered, check)
        }
    }
    return filtered
}

// matchesAnyPattern checks if name matches any of the patterns.
// Supports glob-style wildcards: "CI*" matches "CI / lint"
func matchesAnyPattern(name string, patterns []string) bool {
    for _, pattern := range patterns {
        matched, _ := filepath.Match(pattern, name)
        if matched {
            return true
        }
        // Also try prefix matching for patterns ending in *
        if strings.HasSuffix(pattern, "*") {
            prefix := strings.TrimSuffix(pattern, "*")
            if strings.HasPrefix(name, prefix) {
                return true
            }
        }
    }
    return false
}
```

### Project Structure Notes

**File Locations:**
- `internal/git/github.go` - Extend with WatchPRChecks method and CI types
- `internal/git/github_test.go` - Add comprehensive tests for CI monitoring
- `internal/errors/errors.go` - Verify ErrCIFailed, ErrCITimeout exist; add ErrCICheckNotFound if needed

**Import Rules (from architecture.md):**
- `internal/git` can import: constants, errors, domain
- `internal/git` cannot import: task, workspace, cli, validation, template, tui

### Context-First Pattern

From project-context.md:

```go
// ALWAYS: ctx as first parameter
func (r *CLIGitHubRunner) WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error) {
    // ALWAYS: Check cancellation at entry
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... implementation
}

// ALWAYS: Check context during long waits
select {
case <-ctx.Done():
    return nil, ctx.Err()
case <-time.After(opts.Interval):
    // Continue polling
}
```

### Error Handling

Use existing sentinels and action-first format:

```go
import atlaserrors "github.com/mrz1836/atlas/internal/errors"

// Action-first error format
return nil, fmt.Errorf("failed to fetch PR checks: %w", err)

// Use appropriate sentinel
return result, atlaserrors.ErrCIFailed
return result, atlaserrors.ErrCITimeout

// New sentinel for missing required check
return nil, fmt.Errorf("required check %q not found: %w", checkName, atlaserrors.ErrCICheckNotFound)
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
- Use semantic names: `ATLAS_TEST_CI_STATUS`
- Avoid numeric suffixes that look like keys: `_12345`
- Use `mock_value_for_test` patterns

### References

- [Source: epics.md - Story 6.6: CI Status Monitoring]
- [Source: architecture.md - GitHubRunner Interface section]
- [Source: architecture.md - Retry Strategy section]
- [Source: project-context.md - Context Handling (CRITICAL)]
- [Source: epic-6-implementation-notes.md - GitHubRunner Design]
- [Source: epic-6-user-scenarios.md - Scenario 1 step 11, Scenario 5 step 19]
- [Source: internal/git/github.go - HubRunner interface and CLIGitHubRunner]
- [Source: internal/git/push.go - RetryConfig pattern to follow]
- [Source: internal/errors/errors.go - Existing error sentinels]
- [Source: 6-5-githubrunner-and-pr-creation.md - Story 6.5 learnings]
- [Source: gh pr checks CLI documentation - https://cli.github.com/manual/gh_pr_checks]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (checkpoint 11 - CI wait step)
- Scenario 5: Feature Workflow with Speckit SDD (checkpoint 19 - CI wait step)

Specific validation checkpoints from scenarios:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| Polling | Poll GitHub Actions API via gh pr checks | AC1 |
| Interval | 2 minute default, configurable | AC2 |
| Progress display | "Waiting for CI... (5m elapsed, checking: CI, Lint)" | AC3 |
| All pass | Task proceeds to review step | AC4 |
| Any fail | Transitions to ci_failed state | AC5 |
| Timeout | Transitions to ci_timeout state | AC6 |
| Bell notification | Terminal bell on pass/fail | AC7 |
| Retry logic | 3 attempts with exponential backoff | AC8 |

### Previous Story Intelligence

**From Story 6.5 (GitHubRunner and PR Creation):**
- HubRunner interface established (not GitHubRunner to avoid stutter)
- CLIGitHubRunner with CommandExecutor for testability
- PRStatus struct already has ChecksPass and CIStatus fields (stub implementation)
- GetPRStatus method exists but uses gh pr view (should use gh pr checks for richer data)
- RetryConfig pattern in place
- Error classification with classifyGHError()
- Comprehensive test patterns using mock CommandExecutor
- 94.2% test coverage achieved

**Key Learning from 6.5:**
- The GetPRStatus implementation uses `gh pr view --json statusCheckRollup` which is less detailed
- For CI monitoring, `gh pr checks --json` provides richer data including bucket, workflow, timing
- Consider whether to enhance GetPRStatus or create separate WatchPRChecks method
- Decision: Create WatchPRChecks as a dedicated method for active monitoring with callbacks

### Git Intelligence (Recent Commits)

Recent commits in Epic 6 branch show patterns to follow:
- `feat(git): improve PR scope detection and description formatting` - PR description enhancement
- `feat(git): implement GitHub PR creation with description generation` - HubRunner pattern
- `feat(git): implement push service with retry and error handling` - RetryConfig pattern

File patterns established:
- Implementation: `internal/git/<feature>.go`
- Tests: `internal/git/<feature>_test.go`
- Extend existing files when adding methods to existing interfaces

### Testing Strategy

**Unit Tests (mock gh CLI execution):**

```go
func TestCLIGitHubRunner_WatchPRChecks_Success(t *testing.T) {
    callCount := 0
    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            callCount++
            if callCount == 1 {
                // First poll: pending
                return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
            }
            // Second poll: success
            return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
        PRNumber: 42,
        Interval: 10 * time.Millisecond, // Fast for tests
        Timeout:  1 * time.Second,
    })

    require.NoError(t, err)
    assert.Equal(t, CIStatusSuccess, result.Status)
    assert.Equal(t, 2, callCount)
}

func TestCLIGitHubRunner_WatchPRChecks_Failure(t *testing.T) {
    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            return []byte(`[{"name":"CI","state":"FAILURE","bucket":"fail"}]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
        PRNumber: 42,
    })

    require.NoError(t, err)
    assert.Equal(t, CIStatusFailure, result.Status)
    assert.ErrorIs(t, result.Error, atlaserrors.ErrCIFailed)
}

func TestCLIGitHubRunner_WatchPRChecks_Timeout(t *testing.T) {
    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            // Always pending
            return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
        PRNumber: 42,
        Interval: 10 * time.Millisecond,
        Timeout:  50 * time.Millisecond,
    })

    require.NoError(t, err)
    assert.Equal(t, CIStatusTimeout, result.Status)
    assert.ErrorIs(t, result.Error, atlaserrors.ErrCITimeout)
}

func TestCLIGitHubRunner_WatchPRChecks_RequiredChecksFilter(t *testing.T) {
    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            return []byte(`[
                {"name":"CI / lint","state":"SUCCESS","bucket":"pass"},
                {"name":"CI / test","state":"FAILURE","bucket":"fail"},
                {"name":"Optional","state":"SUCCESS","bucket":"pass"}
            ]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    // Only require CI checks - should fail
    result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
        PRNumber:       42,
        RequiredChecks: []string{"CI*"},
    })

    require.NoError(t, err)
    assert.Equal(t, CIStatusFailure, result.Status)
    assert.Len(t, result.CheckResults, 2) // Only CI checks
}

func TestFilterChecks_WildcardMatching(t *testing.T) {
    checks := []CheckResult{
        {Name: "CI / lint"},
        {Name: "CI / test"},
        {Name: "Security Scan"},
    }

    tests := []struct {
        name     string
        patterns []string
        expected int
    }{
        {"all", nil, 3},
        {"exact match", []string{"CI / lint"}, 1},
        {"wildcard", []string{"CI*"}, 2},
        {"multiple patterns", []string{"CI / lint", "Security*"}, 2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            filtered := filterChecks(checks, tt.patterns)
            assert.Len(t, filtered, tt.expected)
        })
    }
}

func TestCLIGitHubRunner_WatchPRChecks_ProgressCallback(t *testing.T) {
    callCount := 0
    progressCalls := 0

    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            callCount++
            if callCount < 3 {
                return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
            }
            return []byte(`[{"name":"CI","state":"SUCCESS","bucket":"pass"}]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    result, err := runner.WatchPRChecks(context.Background(), CIWatchOptions{
        PRNumber: 42,
        Interval: 10 * time.Millisecond,
        ProgressCallback: func(elapsed time.Duration, checks []CheckResult) {
            progressCalls++
        },
    })

    require.NoError(t, err)
    assert.Equal(t, CIStatusSuccess, result.Status)
    assert.Equal(t, 3, progressCalls) // Called on each poll
}

func TestCLIGitHubRunner_WatchPRChecks_ContextCancellation(t *testing.T) {
    mockCmd := &MockCommandExecutor{
        ExecuteFunc: func(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
            return []byte(`[{"name":"CI","state":"PENDING","bucket":"pending"}]`), nil
        },
    }
    runner := NewCLIGitHubRunner("/tmp/test", WithGHCommandExecutor(mockCmd))

    ctx, cancel := context.WithCancel(context.Background())

    go func() {
        time.Sleep(30 * time.Millisecond)
        cancel()
    }()

    _, err := runner.WatchPRChecks(ctx, CIWatchOptions{
        PRNumber: 42,
        Interval: 100 * time.Millisecond,
    })

    assert.ErrorIs(t, err, context.Canceled)
}
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

- Initial implementation complete with all ACs implemented
- Code review identified and fixed bug: ErrCICheckNotFound was defined but not used
- Added FormatCIProgressMessage helper function for AC3 compliance
- Added tests for RequiredChecks not found scenario
- Added tests for FormatCIProgressMessage and formatDuration helpers

### File List

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/git/github.go` | Modified | Extended HubRunner with WatchPRChecks method, added CI monitoring types (CIStatus, CIWatchOptions, CIWatchResult, CheckResult), implemented polling loop with retry, filtering, progress callback, and bell notification. Added FormatCIProgressMessage helper. |
| `internal/git/github_test.go` | Modified | Added 30+ tests for CI monitoring: success/failure/timeout scenarios, required checks filtering, wildcard matching, progress callback, context cancellation, retry logic, edge cases. Added tests for FormatCIProgressMessage. |
| `internal/errors/errors.go` | Modified | Added ErrCICheckNotFound sentinel error for when required CI checks are not found |

### Senior Developer Review (AI)

**Reviewed by:** Claude Opus 4.5
**Date:** 2025-12-30
**Outcome:** Approved with fixes applied

**Issues Found and Fixed:**
1. CRIT-3: ErrCICheckNotFound was defined but not used - Now validates required checks exist
2. MED-1: AC3 progress message format missing - Added FormatCIProgressMessage helper
3. MED-2: Missing test for required checks not found - Added TestCLIGitHubRunner_WatchPRChecks_RequiredChecksNotFound

**Validation Status:** ✅ All passed (format, lint, test:race, pre-commit)

