# Story 6.10: Wire CIExecutor to HubRunner.WatchPRChecks

Status: ready-for-dev

## Story

As a **user**,
I want **the CI wait step to actually poll GitHub Actions**,
So that **I know when my CI checks pass or fail and can take appropriate action**.

## Acceptance Criteria

1. **Given** a task reaches the `ci_wait` step, **When** the step executes, **Then** the system calls `HubRunner.WatchPRChecks()` to poll GitHub Actions API.

2. **Given** CI is being monitored, **When** polling occurs, **Then** the system:
   - Polls at configurable intervals (default: 2 minutes)
   - Shows progress with check names and statuses
   - Emits bell notification when checks complete

3. **Given** all required checks pass, **When** CI completes successfully, **Then** the system:
   - Transitions to the next step (human review)
   - Saves CI result artifact (ci-result.json)

4. **Given** any required check fails, **When** CI failure is detected, **Then** the system:
   - Transitions task to `ci_failed` state
   - Emits bell notification
   - Invokes `CIFailureHandler` to present options menu

5. **Given** CI monitoring times out, **When** timeout is reached (default: 30 minutes), **Then** the system:
   - Transitions task to `ci_timeout` state
   - Emits bell notification
   - Presents options: continue waiting, retry, fix manually, abandon

6. **Given** the `poll_interval` config is set, **When** CI monitoring runs, **Then** the specified interval is used for polling.

7. **Given** the `timeout` config is set on the step, **When** CI monitoring runs, **Then** the specified timeout is used.

8. **Given** specific workflows are configured, **When** CI monitoring runs, **Then** only the configured workflows are watched (e.g., "CI", "Lint").

## Tasks / Subtasks

- [ ] Task 1: Refactor CIExecutor to use HubRunner (AC: 1, 2)
  - [ ] 1.1: Add `hubRunner git.HubRunner` field to CIExecutor
  - [ ] 1.2: Add `ciFailureHandler *task.CIFailureHandler` field to CIExecutor
  - [ ] 1.3: Create `NewCIExecutor(opts ...CIExecutorOption) *CIExecutor` constructor
  - [ ] 1.4: Implement functional options for dependency injection
  - [ ] 1.5: Remove placeholder polling loop

- [ ] Task 2: Implement CI monitoring execution (AC: 1, 2, 3, 6, 7, 8)
  - [ ] 2.1: Extract `poll_interval` from step config (default: 2 minutes)
  - [ ] 2.2: Extract `timeout` from step config (default: 30 minutes)
  - [ ] 2.3: Extract `workflows` from step config (default: all)
  - [ ] 2.4: Extract `pr_number` from task metadata
  - [ ] 2.5: Build `CIWatchOptions` with extracted config
  - [ ] 2.6: Call `hubRunner.WatchPRChecks(ctx, opts)` with options
  - [ ] 2.7: Handle result based on CIStatus (Success, Failure, Timeout)

- [ ] Task 3: Implement success handling (AC: 3)
  - [ ] 3.1: On CIStatusSuccess, return completed StepResult
  - [ ] 3.2: Save ci-result.json artifact with check details
  - [ ] 3.3: Include elapsed time, check names, and statuses in result

- [ ] Task 4: Implement failure handling (AC: 4)
  - [ ] 4.1: On CIStatusFailure, call `ciFailureHandler.HandleCIFailure()`
  - [ ] 4.2: Pass CIWatchResult to handler for error context extraction
  - [ ] 4.3: Return appropriate StepResult based on handler result:
     - ViewLogs: Return awaiting_approval with browser open
     - RetryFromImplement: Return result with next_step = "implement"
     - FixManually: Return awaiting_approval with instructions
     - Abandon: Return failed with abandoned transition
  - [ ] 4.4: Save ci-result.json artifact with failure details

- [ ] Task 5: Implement timeout handling (AC: 5)
  - [ ] 5.1: On CIStatusTimeout, return awaiting_approval with timeout options
  - [ ] 5.2: Options: continue_waiting, retry, fix_manually, abandon
  - [ ] 5.3: If continue_waiting: restart monitoring with extended timeout
  - [ ] 5.4: Save ci-result.json artifact with timeout details

- [ ] Task 6: Update Execute method (AC: all)
  - [ ] 6.1: Replace placeholder implementation in `Execute()` method
  - [ ] 6.2: Add context cancellation check at entry
  - [ ] 6.3: Extract PR number from task (required)
  - [ ] 6.4: Handle missing PR number gracefully with clear error

- [ ] Task 7: Wire executor in factory (AC: all)
  - [ ] 7.1: Update step executor factory to create CIExecutor with dependencies
  - [ ] 7.2: Inject HubRunner and CIFailureHandler
  - [ ] 7.3: Ensure HubRunner is configured with proper authentication

- [ ] Task 8: Create comprehensive tests (AC: 1-8)
  - [ ] 8.1: Test Execute with successful CI (all checks pass)
  - [ ] 8.2: Test Execute with CI failure (one check fails)
  - [ ] 8.3: Test Execute with CI timeout
  - [ ] 8.4: Test poll interval configuration
  - [ ] 8.5: Test timeout configuration
  - [ ] 8.6: Test workflow filtering
  - [ ] 8.7: Test CI failure handler integration (ViewLogs action)
  - [ ] 8.8: Test CI failure handler integration (RetryFromImplement action)
  - [ ] 8.9: Test CI failure handler integration (FixManually action)
  - [ ] 8.10: Test CI failure handler integration (Abandon action)
  - [ ] 8.11: Test artifact saving
  - [ ] 8.12: Test missing PR number error
  - [ ] 8.13: Target 90%+ coverage for new code

## Dev Notes

### Current Placeholder Code to Replace

```go
// internal/template/steps/ci.go - CURRENT (placeholder)
func (e *CIExecutor) Execute(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
    // This is a placeholder implementation for Epic 4.
    // Full implementation will be added in Epic 6 when GitHubRunner is available.

    // Fake polling loop that returns success after 3 iterations
    for i := 0; i < 3; i++ {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(time.Second):
            // Simulated poll
        }
    }

    return &domain.StepResult{
        Status: "completed",
        Output: "CI checks passed (placeholder)",
    }, nil
}
```

### New CIExecutor Design

```go
// CIExecutor handles CI wait steps by monitoring GitHub Actions.
type CIExecutor struct {
    hubRunner        git.HubRunner
    ciFailureHandler *task.CIFailureHandler
    logger           zerolog.Logger
}

// CIExecutorOption configures CIExecutor.
type CIExecutorOption func(*CIExecutor)

// NewCIExecutor creates a CIExecutor with dependencies.
func NewCIExecutor(opts ...CIExecutorOption) *CIExecutor {
    e := &CIExecutor{
        logger: zerolog.Nop(),
    }
    for _, opt := range opts {
        opt(e)
    }
    return e
}

// WithHubRunner sets the GitHub runner for CI monitoring.
func WithHubRunner(runner git.HubRunner) CIExecutorOption {
    return func(e *CIExecutor) {
        e.hubRunner = runner
    }
}

// WithCIFailureHandler sets the CI failure handler.
func WithCIFailureHandler(handler *task.CIFailureHandler) CIExecutorOption {
    return func(e *CIExecutor) {
        e.ciFailureHandler = handler
    }
}
```

### Execute Method Implementation

```go
func (e *CIExecutor) Execute(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Validate dependencies
    if e.hubRunner == nil {
        return nil, fmt.Errorf("CI executor missing HubRunner: %w", atlaserrors.ErrInvalidConfig)
    }

    // Extract PR number from task metadata
    prNumber, ok := task.Metadata["pr_number"].(int)
    if !ok || prNumber <= 0 {
        return nil, fmt.Errorf("CI wait step requires pr_number in task metadata: %w", atlaserrors.ErrInvalidConfig)
    }

    // Extract configuration from step
    pollInterval := extractDuration(step.Config, "poll_interval", constants.CIPollInterval)
    timeout := step.Timeout
    if timeout == 0 {
        timeout = constants.DefaultCITimeout
    }
    workflows := extractStringSlice(step.Config, "workflows")

    // Build watch options
    watchOpts := git.CIWatchOptions{
        PRNumber:     prNumber,
        PollInterval: pollInterval,
        Timeout:      timeout,
        Workflows:    workflows,
    }

    // Execute CI monitoring
    e.logger.Info().
        Int("pr_number", prNumber).
        Dur("poll_interval", pollInterval).
        Dur("timeout", timeout).
        Strs("workflows", workflows).
        Msg("starting CI monitoring")

    result, err := e.hubRunner.WatchPRChecks(ctx, watchOpts)
    if err != nil {
        return nil, fmt.Errorf("failed to watch PR checks: %w", err)
    }

    // Save CI result artifact
    artifactDir := filepath.Join(task.ArtifactDir, step.Name)
    artifactPath := e.saveCIArtifact(result, artifactDir)

    // Handle result based on status
    switch result.Status {
    case git.CIStatusSuccess:
        return e.handleSuccess(result, artifactPath)
    case git.CIStatusFailure:
        return e.handleFailure(ctx, result, task, artifactPath)
    case git.CIStatusTimeout:
        return e.handleTimeout(result, artifactPath)
    default:
        return nil, fmt.Errorf("unexpected CI status %v: %w", result.Status, atlaserrors.ErrUnexpectedState)
    }
}
```

### Success Handling

```go
func (e *CIExecutor) handleSuccess(result *git.CIWatchResult, artifactPath string) (*domain.StepResult, error) {
    return &domain.StepResult{
        Status: domain.StepStatusCompleted,
        Output: fmt.Sprintf("CI passed in %s", result.ElapsedTime.Round(time.Second)),
        Metadata: map[string]any{
            "elapsed_time":   result.ElapsedTime.String(),
            "checks_passed":  len(result.CheckResults),
        },
        ArtifactPaths: []string{artifactPath},
    }, nil
}
```

### Failure Handling with CIFailureHandler

```go
func (e *CIExecutor) handleFailure(ctx context.Context, result *git.CIWatchResult, task *domain.Task, artifactPath string) (*domain.StepResult, error) {
    // If no failure handler, return simple failure
    if e.ciFailureHandler == nil {
        return &domain.StepResult{
            Status: domain.StepStatusFailed,
            Output: "CI checks failed",
            Error:  atlaserrors.ErrCIFailed,
            Metadata: map[string]any{
                "failure_type": "ci_failed",
            },
            ArtifactPaths: []string{artifactPath},
        }, nil
    }

    // Present failure options via handler
    // For now, return awaiting_approval to trigger menu display
    // The actual action handling happens when user responds
    return &domain.StepResult{
        Status: domain.StepStatusAwaitingApproval,
        Output: formatCIFailureMessage(result),
        Metadata: map[string]any{
            "action_required": "ci_failure_handling",
            "ci_result":       result,
            "failure_type":    "ci_failed",
        },
        ArtifactPaths: []string{artifactPath},
    }, nil
}

func formatCIFailureMessage(result *git.CIWatchResult) string {
    var sb strings.Builder
    sb.WriteString("CI checks failed:\n\n")

    for _, check := range result.CheckResults {
        if check.Bucket == "fail" {
            sb.WriteString(fmt.Sprintf("  - %s: %s\n", check.Name, check.Bucket))
            if check.URL != "" {
                sb.WriteString(fmt.Sprintf("    Logs: %s\n", check.URL))
            }
        }
    }

    return sb.String()
}
```

### Timeout Handling

```go
func (e *CIExecutor) handleTimeout(result *git.CIWatchResult, artifactPath string) (*domain.StepResult, error) {
    return &domain.StepResult{
        Status: domain.StepStatusAwaitingApproval,
        Output: fmt.Sprintf("CI monitoring timed out after %s", result.ElapsedTime.Round(time.Second)),
        Metadata: map[string]any{
            "action_required": "ci_timeout_handling",
            "ci_result":       result,
            "failure_type":    "ci_timeout",
        },
        ArtifactPaths: []string{artifactPath},
    }, nil
}
```

### Artifact Saving

```go
func (e *CIExecutor) saveCIArtifact(result *git.CIWatchResult, artifactDir string) string {
    if err := os.MkdirAll(artifactDir, 0755); err != nil {
        e.logger.Warn().Err(err).Msg("failed to create artifact directory")
        return ""
    }

    artifact := task.CIResultArtifact{
        Status:      result.Status.String(),
        ElapsedTime: result.ElapsedTime.String(),
        AllChecks:   convertChecksToArtifact(result.CheckResults),
        FailedChecks: filterFailedChecks(result.CheckResults),
        Timestamp:   time.Now().Format(time.RFC3339),
    }

    if result.Error != nil {
        artifact.ErrorMessage = result.Error.Error()
    }

    path := filepath.Join(artifactDir, "ci-result.json")
    data, _ := json.MarshalIndent(artifact, "", "  ")
    _ = os.WriteFile(path, data, 0644)

    return path
}
```

### Helper Functions

```go
func extractDuration(config map[string]any, key string, defaultVal time.Duration) time.Duration {
    if val, ok := config[key]; ok {
        switch v := val.(type) {
        case time.Duration:
            return v
        case string:
            if d, err := time.ParseDuration(v); err == nil {
                return d
            }
        case int:
            return time.Duration(v) * time.Second
        }
    }
    return defaultVal
}

func extractStringSlice(config map[string]any, key string) []string {
    if val, ok := config[key]; ok {
        switch v := val.(type) {
        case []string:
            return v
        case []any:
            result := make([]string, 0, len(v))
            for _, item := range v {
                if s, ok := item.(string); ok {
                    result = append(result, s)
                }
            }
            return result
        }
    }
    return nil
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

- [Source: epic-6-traceability-matrix.md - GAP 2]
- [Source: internal/git/github.go - HubRunner.WatchPRChecks() to wire]
- [Source: internal/task/ci_failure.go - CIFailureHandler to integrate]
- [Source: internal/template/steps/ci.go - Current placeholder to replace]
- [Source: 6-6-ci-status-monitoring.md - CIWatchResult types]
- [Source: 6-7-ci-failure-handling.md - CIFailureHandler design]

### User Scenario Validation

This story is validated by the following scenarios from `epic-6-user-scenarios.md`:
- Scenario 1: Bugfix Workflow (Step 11 - CI Wait)
- Scenario 3: PR Creation with Rate Limit (CI timeout handling)
- Scenario 5: Feature Workflow with Speckit SDD (Step 19 - CI Wait)

Specific validation checkpoints:
| Checkpoint | Expected Behavior | AC |
|------------|-------------------|-----|
| CI polling | Calls WatchPRChecks | AC1 |
| Poll interval | Configurable (default 2min) | AC2, AC6 |
| Progress display | Shows check names/statuses | AC2 |
| Bell notification | Emits on completion | AC2 |
| CI success | Transitions to next step | AC3 |
| CI failure | Invokes CIFailureHandler | AC4 |
| CI timeout | Presents timeout options | AC5 |
| Workflow filter | Only watches configured workflows | AC8 |
