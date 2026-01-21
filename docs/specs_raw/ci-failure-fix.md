# Implementation Plan: AI-Assisted CI Failure Fix

## Overview

Adapt ATLAS's existing validation retry mechanism to automatically fix CI failures. When a CI check fails, ATLAS will:
1. Fetch CI logs using `gh` CLI
2. Extract actionable error context
3. **Always invoke AI** to analyze the failure and decide the appropriate action
4. AI either fixes code OR recommends a re-run (for transient failures)
5. Pause for user approval before pushing (configurable)
6. Push fixes and monitor CI until success or max attempts exceeded

### Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Human Authority** | Configurable (default: pause for approval) | Aligns with ATLAS constitution; opt-in `auto_commit_enabled` for power users |
| **Menu Integration** | Replace when enabled | If `retry_enabled=true`, auto-retry runs first; menu only shows if retry exhausted |
| **Transient Failures** | Always consult AI | AI analyzes logs and decides if re-run is appropriate; no bypass logic |
| **Configuration** | Use existing `operations.ci_failure` | Avoid duplicate config; only add CI-specific fields to `CIConfig` |

---

## Architecture Alignment

### Existing Pattern: Validation Retry

```
ValidationExecutor.Execute() → failure
    ↓
engine.tryValidationRetry()
    ↓
validationRetryHandler.RetryWithAI()
    ↓
ExtractErrorContext() → BuildAIPrompt() → aiRunner.Run() → Re-validate
```

### Proposed Pattern: CI Retry (Parallel Structure)

```
CIExecutor.Execute() → failure
    ↓
engine.tryCIRetry()                          [NEW - mirrors tryValidationRetry]
    ↓
ciRetryHandler.RetryWithAI()                 [NEW - mirrors validationRetryHandler]
    ↓
FetchCILogs() → ExtractCIErrorContext() → BuildCIPrompt() → aiRunner.Run()
    ↓
[AI decides: fix code OR recommend re-run]
    ↓
[auto_commit_enabled?] → NO → AwaitingApproval → [user approves] → Push → Re-poll
                       → YES → Push → Re-poll
```

---

## Task State Machine

```
Running → ci_wait → CI fails
                      ↓
              [retry_enabled?]
                 /        \
               NO          YES
                ↓           ↓
           CIFailed    Fetch CI logs
           (show menu)      ↓
                       AI analyzes (even for transient failures)
                            ↓
                       AI action: fix code OR recommend re-run
                            ↓
                    [auto_commit_enabled?]
                       /            \
                     NO              YES
                      ↓               ↓
              AwaitingApproval   Commit & Push (auto)
                      ↓               ↓
               [user approves?]  Wait for CI
                  /        \          ↓
                NO          YES   [CI passes?]
                 ↓           ↓      /       \
             Rejected   Commit & Push     YES
                             ↓             ↓
                       Wait for CI      Success
                             ↓
                       [CI passes?]
                         /       \
                       NO         YES
                        ↓          ↓
                 [attempts < max?]  Success
                   /        \
                 YES         NO
                  ↓           ↓
            Retry loop    CIFailed (show menu)
```

**Key Flow Rules:**
- When `retry_enabled=true`: Auto-retry runs FIRST, menu only shows if exhausted
- AI is ALWAYS consulted, even for transient failures (AI decides if re-run is appropriate)
- When `auto_commit_enabled=false` (default): Pause at `AwaitingApproval` before push

**Metadata Keys:**
- `ci_retry_attempt` - Current attempt number (1-indexed)
- `ci_retry_started_sha` - HEAD SHA when retry started (for conflict detection)
- `ci_retry_ai_changes` - Files AI modified
- `ci_retry_commit_sha` - Commit SHA of AI fix (if pushed)

---

## File Structure

```
internal/
├── ci/                                      [NEW PACKAGE]
│   ├── retry_handler.go                     # CIRetryHandler implementation
│   ├── retry_handler_test.go                # Comprehensive tests
│   ├── log_fetcher.go                       # gh CLI log fetching
│   ├── log_fetcher_test.go                  # Mock gh responses
│   ├── error_context.go                     # MOVE from task/ci_failure.go + enhance
│   ├── error_context_test.go                # Error parsing tests
│   └── types.go                             # CIRetryConfig, CIRetryResult, etc.
│
├── contracts/
│   ├── git.go                               [NEW] GitOperations interface
│   └── ci_retry.go                          [NEW] CIRetryHandler interface for engine
│
├── task/
│   ├── ci_retry.go                          [NEW] Interface adapter
│   ├── ci_retry_test.go                     [NEW] Interface tests
│   ├── engine_ci_retry.go                   [NEW] Engine integration
│   ├── engine_ci_retry_test.go              [NEW] Engine CI retry tests
│   └── ci_failure.go                        [MODIFY] Backward-compat wrapper for ExtractCIErrorContext
│
├── prompts/
│   ├── prompts.go                           [MODIFY] Register CIRetry prompt
│   ├── types.go                             [MODIFY] Add CIRetryData struct
│   └── templates/
│       └── ci/
│           └── retry.tmpl                   [NEW] CI fix prompt (centralized location)
│
├── git/
│   ├── github.go                            [MODIFY] Extend HubRunner interface
│   ├── github_ci.go                         [MODIFY] Add RunID/JobID to CheckResult
│   └── github_hub_runner.go                 [MODIFY] Implement FetchRunLogs, RerunFailedJobs
│
└── config/
    ├── config.go                            [MODIFY] Add CI retry fields to CIConfig
    └── defaults.go                          [MODIFY] Add CI retry defaults
```

**Note:** Prompts are stored in the centralized `internal/prompts/templates/` directory following ATLAS conventions. The existing `operations.ci_failure` config in `config.go` will be leveraged for agent/model/timeout settings.

---

## Contracts (New Interfaces)

### `contracts/git.go` - Git Operations Interface

**Purpose:** Abstract git operations for testability and loose coupling.

```go
package contracts

import "context"

// GitOperations abstracts git commands for CI retry handler
// This interface enables mocking in tests and loose coupling
type GitOperations interface {
    // StageAll stages all changes in the working directory (git add -A)
    StageAll(ctx context.Context, workDir string) error

    // Commit creates a commit with the given message, returns commit SHA
    Commit(ctx context.Context, workDir string, message string) (sha string, err error)

    // Push pushes to the remote (current branch)
    Push(ctx context.Context, workDir string) error

    // GetHEAD returns the current HEAD commit SHA (for conflict detection)
    GetHEAD(ctx context.Context, workDir string) (sha string, err error)
}
```

### `contracts/ci_retry.go` - CI Retry Handler Interface

**Purpose:** Interface for engine to interact with CI retry handler.

```go
package contracts

import (
    "context"

    "github.com/your-org/atlas/internal/ci"
    "github.com/your-org/atlas/internal/domain"
    "github.com/your-org/atlas/internal/git"
)

// CIRetryHandler defines the interface for AI-assisted CI failure recovery
// Used by task.Engine for dependency injection
type CIRetryHandler interface {
    // RetryWithAI attempts to fix CI failure using AI analysis
    RetryWithAI(
        ctx context.Context,
        prNumber int,
        failedChecks []git.CheckResult,
        workDir string,
        attemptNum int,
        runnerConfig *domain.RunnerConfig,
    ) (*ci.CIRetryResult, error)

    // CanRetry checks if another retry attempt is allowed
    CanRetry(attemptNum int) bool

    // MaxAttempts returns the maximum number of retry attempts
    MaxAttempts() int

    // IsEnabled returns whether CI retry is enabled
    IsEnabled() bool
}
```

---

## Detailed Component Design

### 1. CI Log Fetcher (`internal/ci/log_fetcher.go`)

**Purpose**: Fetch CI failure logs from GitHub Actions using `gh` CLI.

```go
// LogFetcher fetches CI logs from GitHub Actions
type LogFetcher interface {
    // FetchFailedLogs returns logs for failed checks on a PR
    FetchFailedLogs(ctx context.Context, prNumber int, checkNames []string) (*CILogs, error)

    // FetchWorkflowLogs returns logs for a specific workflow run
    FetchWorkflowLogs(ctx context.Context, runID int64) (string, error)

    // CanRerun determines if a failure is transient and can be re-run
    CanRerun(ctx context.Context, runID int64) (bool, string, error)
}

// CILogs contains structured CI failure information
type CILogs struct {
    PRNumber     int
    FailedChecks []FailedCheck
    TotalLogs    string        // Combined log output (truncated)
    FetchedAt    time.Time
}

// FailedCheck represents a single failed CI check
type FailedCheck struct {
    Name       string
    Workflow   string
    RunID      int64
    JobID      int64
    Conclusion string    // "failure", "cancelled", "timed_out"
    URL        string
    Logs       string    // Extracted log content
    Duration   time.Duration
}
```

**gh CLI Commands Used**:
```bash
# List failed checks with run IDs
gh pr checks <PR_NUMBER> --json name,state,workflow,runId,jobId,link,conclusion

# Fetch workflow run logs
gh run view <RUN_ID> --log-failed

# Re-run failed jobs only
gh run rerun <RUN_ID> --failed

# Re-run entire workflow
gh run rerun <RUN_ID>
```

**Implementation Details**:

```go
type DefaultLogFetcher struct {
    hubRunner   contracts.HubRunner
    workDir     string
    maxLogSize  int    // Default: 50KB per check, 200KB total
    logger      zerolog.Logger
}

func (f *DefaultLogFetcher) FetchFailedLogs(ctx context.Context, prNumber int, checkNames []string) (*CILogs, error) {
    // 1. Get check details with run IDs
    checks, err := f.fetchPRChecksWithRunIDs(ctx, prNumber)
    if err != nil {
        return nil, fmt.Errorf("fetch checks: %w", err)
    }

    // 2. Filter to failed checks
    failedChecks := filterFailedChecks(checks, checkNames)
    if len(failedChecks) == 0 {
        return nil, ErrNoFailedChecks
    }

    // 3. Fetch logs for each failed check (parallel, with limit)
    logs := f.fetchLogsParallel(ctx, failedChecks, 3) // max 3 concurrent

    // 4. Truncate and combine logs
    return &CILogs{
        PRNumber:     prNumber,
        FailedChecks: logs,
        TotalLogs:    combineLogs(logs, f.maxLogSize),
        FetchedAt:    time.Now(),
    }, nil
}

func (f *DefaultLogFetcher) CanRerun(ctx context.Context, runID int64) (bool, string, error) {
    // Detect transient failures that just need re-run:
    // - "Resource temporarily unavailable"
    // - "rate limit exceeded"
    // - "network error"
    // - "timed out" (without code changes)
    // - "service unavailable"

    logs, err := f.FetchWorkflowLogs(ctx, runID)
    if err != nil {
        return false, "", err
    }

    reason, isTransient := detectTransientFailure(logs)
    return isTransient, reason, nil
}
```

---

### 2. CI Error Context Extractor (`internal/ci/error_context.go`)

**Purpose**: Parse CI logs and extract actionable error context for AI.

**Note**: An `ExtractCIErrorContext()` function already exists in `internal/task/ci_failure.go`. This implementation should be **moved** to `internal/ci/error_context.go` and enhanced with log parsing capabilities. A backward-compatible wrapper should remain in `task/ci_failure.go`.

```go
// CIRetryContext contains extracted error information for AI
type CIRetryContext struct {
    PRNumber       int
    FailedChecks   []string          // Names of failed checks
    ErrorType      CIErrorType       // lint, test, build, other
    ErrorSummary   string            // Concise error description
    ErrorDetails   string            // Full error output (truncated)
    AffectedFiles  []string          // Files mentioned in errors
    AttemptNumber  int
    MaxAttempts    int
    StartedAtSHA   string            // HEAD SHA when retry started (for conflict detection)
}
// Note: Transient failure detection is now handled by AI, not hard-coded patterns.
// AI analyzes logs and outputs {"action": "rerun", "reason": "..."} for transient failures.

// CIErrorType categorizes the CI failure
type CIErrorType string

const (
    CIErrorTypeLint    CIErrorType = "lint"
    CIErrorTypeTest    CIErrorType = "test"
    CIErrorTypeBuild   CIErrorType = "build"
    CIErrorTypeFormat  CIErrorType = "format"
    CIErrorTypeOther   CIErrorType = "other"
)

// ExtractCIErrorContext analyzes CI logs and extracts structured context
func ExtractCIErrorContext(logs *CILogs, attemptNum, maxAttempts int) *CIRetryContext {
    ctx := &CIRetryContext{
        PRNumber:      logs.PRNumber,
        FailedChecks:  extractCheckNames(logs.FailedChecks),
        AttemptNumber: attemptNum,
        MaxAttempts:   maxAttempts,
    }

    // 1. Detect error type from check names and log content
    ctx.ErrorType = detectErrorType(logs)

    // 2. Extract error summary (first meaningful error line)
    ctx.ErrorSummary = extractErrorSummary(logs.TotalLogs)

    // 3. Extract detailed errors (truncated to fit context window)
    ctx.ErrorDetails = extractErrorDetails(logs.TotalLogs, maxDetailsSize)

    // 4. Extract affected files from error messages
    ctx.AffectedFiles = extractAffectedFiles(logs.TotalLogs)

    // 5. Capture HEAD SHA for conflict detection
    ctx.StartedAtSHA = getCurrentHEAD(workDir)

    return ctx
}

// Note: Transient failure detection removed - AI now handles this decision.
// The prompt instructs AI to output {"action": "rerun", "reason": "..."} for transient failures.

// detectErrorType analyzes check names and logs to categorize the failure
func detectErrorType(logs *CILogs) CIErrorType {
    for _, check := range logs.FailedChecks {
        name := strings.ToLower(check.Name)
        switch {
        case strings.Contains(name, "lint"):
            return CIErrorTypeLint
        case strings.Contains(name, "test"):
            return CIErrorTypeTest
        case strings.Contains(name, "build") || strings.Contains(name, "compile"):
            return CIErrorTypeBuild
        case strings.Contains(name, "format") || strings.Contains(name, "fmt"):
            return CIErrorTypeFormat
        }
    }

    // Fall back to log content analysis
    return analyzeLogContent(logs.TotalLogs)
}

// Note: checkTransientFailure() function removed.
// AI now analyzes logs and decides if a failure is transient.
// This approach is more robust and handles edge cases better than hard-coded patterns.
```

---

### 3. CI Retry Handler (`internal/ci/retry_handler.go`)

**Purpose**: Orchestrate the CI retry process, mirroring `validation.RetryHandler`.

```go
// CIRetryConfig configures CI retry behavior
// Note: Agent/model/timeout come from existing operations.ci_failure config
type CIRetryConfig struct {
    Enabled           bool          // Default: true (from ci.retry_enabled)
    AutoCommitEnabled bool          // Default: false - require approval before push
    LogFetchTimeout   time.Duration // Default: 30s (from ci.log_fetch_timeout)
    // MaxAttempts comes from operations.ci_failure.max_attempts
}

// CIRetryResult captures the outcome of a CI retry attempt
type CIRetryResult struct {
    Success          bool
    AttemptNumber    int
    Action           CIRetryAction  // "fix", "rerun", "none"
    AIResult         *domain.AIResult
    FilesChanged     []string
    ProposedChanges  []string       // Files AI wants to change (before approval)
    CommitSHA        string         // New commit if code was changed
    RerunTriggered   bool
    RequiresApproval bool           // True unless auto_commit_enabled=true
    CIStatus         CIStatus       // Final CI status after fix/rerun
    Error            error
}

// CIRetryAction describes what action was taken
type CIRetryAction string

const (
    CIRetryActionFix   CIRetryAction = "fix"   // AI fixed code
    CIRetryActionRerun CIRetryAction = "rerun" // Re-ran without changes
    CIRetryActionNone  CIRetryAction = "none"  // No action taken
)

// CIRetryHandler handles AI-assisted CI failure recovery
type CIRetryHandler struct {
    logFetcher       LogFetcher
    aiRunner         contracts.AIRunner
    gitOps           contracts.GitOperations       // NEW interface (see contracts/git.go)
    hubRunner        git.HubRunner                 // Existing interface - includes WatchPRChecks
    config           CIRetryConfig
    operationsConfig *config.OperationsConfig      // For agent/model/timeout from operations.ci_failure
    logger           zerolog.Logger
}

func NewCIRetryHandler(
    logFetcher LogFetcher,
    aiRunner contracts.AIRunner,
    gitOps contracts.GitOperations,
    hubRunner git.HubRunner,
    config CIRetryConfig,
    logger zerolog.Logger,
) *CIRetryHandler {
    return &CIRetryHandler{
        logFetcher: logFetcher,
        aiRunner:   aiRunner,
        gitOps:     gitOps,
        hubRunner:  hubRunner,
        config:     config,
        logger:     logger,
    }
}

// SetOperationsConfig allows lazy injection of per-operation AI settings
// (mirrors validation.RetryHandler pattern)
func (h *CIRetryHandler) SetOperationsConfig(cfg *config.OperationsConfig) {
    h.operationsConfig = cfg
}

// RetryWithAI attempts to fix CI failure using AI
func (h *CIRetryHandler) RetryWithAI(
    ctx context.Context,
    prNumber int,
    failedChecks []git.CheckResult,
    workDir string,
    attemptNum int,
    runnerConfig *domain.RunnerConfig,
) (*CIRetryResult, error) {
    // 1. Validate retry is allowed
    if !h.config.Enabled {
        return nil, ErrCIRetryDisabled
    }

    maxAttempts := h.getMaxAttempts()
    if attemptNum > maxAttempts {
        return nil, ErrMaxCIRetriesExceeded
    }

    // 2. Fetch CI logs
    h.logger.Info().Int("pr", prNumber).Msg("Fetching CI failure logs")
    checkNames := extractCheckNames(failedChecks)

    fetchCtx, cancel := context.WithTimeout(ctx, h.config.LogFetchTimeout)
    defer cancel()

    logs, err := h.logFetcher.FetchFailedLogs(fetchCtx, prNumber, checkNames)
    if err != nil {
        return nil, fmt.Errorf("fetch CI logs: %w", err)
    }

    // 3. Extract error context (includes HEAD SHA for conflict detection)
    retryCtx := ExtractCIErrorContext(logs, attemptNum, maxAttempts, workDir)

    // 4. Resolve agent/model from operations.ci_failure config
    agent, model := h.getAgentModel()

    // 5. Build AI prompt - AI will decide if fix or re-run is needed
    prompt, err := prompts.Render(prompts.CIRetry, prompts.CIRetryData{
        PRNumber:      retryCtx.PRNumber,
        FailedChecks:  retryCtx.FailedChecks,
        ErrorType:     string(retryCtx.ErrorType),
        ErrorSummary:  retryCtx.ErrorSummary,
        ErrorDetails:  retryCtx.ErrorDetails,
        AffectedFiles: retryCtx.AffectedFiles,
        AttemptNumber: retryCtx.AttemptNumber,
        MaxAttempts:   retryCtx.MaxAttempts,
    })
    if err != nil {
        return nil, fmt.Errorf("build CI retry prompt: %w", err)
    }

    aiReq := &domain.AIRequest{
        Agent:        agent,
        Prompt:       prompt,
        Model:        model,
        WorkingDir:   workDir,
        RunnerConfig: runnerConfig,
    }

    h.logger.Info().
        Str("error_type", string(retryCtx.ErrorType)).
        Int("attempt", attemptNum).
        Msg("Invoking AI to analyze CI failure")

    aiResult, err := h.aiRunner.Run(ctx, aiReq)
    if err != nil {
        return nil, fmt.Errorf("AI execution failed: %w", err)
    }

    // 6. Parse AI response - check if AI recommends re-run (transient failure)
    if action := parseAIAction(aiResult.Output); action.Type == "rerun" {
        h.logger.Info().Str("reason", action.Reason).Msg("AI recommends re-run for transient failure")
        return h.triggerRerun(ctx, logs, retryCtx, action.Reason)
    }

    // 7. AI made code changes - check if any files changed
    if !aiResult.Success || len(aiResult.FilesChanged) == 0 {
        return &CIRetryResult{
            Success:       false,
            AttemptNumber: attemptNum,
            Action:        CIRetryActionNone,
            AIResult:      aiResult,
            Error:         ErrAINoChanges,
        }, nil
    }

    // 8. Human authority checkpoint - pause for approval unless auto_commit_enabled
    if !h.config.AutoCommitEnabled {
        return &CIRetryResult{
            Success:          false, // Not yet - waiting for approval
            AttemptNumber:    attemptNum,
            Action:           CIRetryActionFix,
            AIResult:         aiResult,
            ProposedChanges:  aiResult.FilesChanged,
            RequiresApproval: true,
        }, nil
    }

    // 9. Auto-commit enabled - commit and push changes
    commitSHA, err := h.commitAndPush(ctx, workDir, retryCtx)
    if err != nil {
        return nil, fmt.Errorf("commit and push: %w", err)
    }

    // 10. Wait for CI to complete
    ciResult, err := h.waitForCI(ctx, prNumber)
    if err != nil {
        return nil, fmt.Errorf("wait for CI: %w", err)
    }

    return &CIRetryResult{
        Success:       ciResult.Status == git.CIStatusSuccess,
        AttemptNumber: attemptNum,
        Action:        CIRetryActionFix,
        AIResult:      aiResult,
        FilesChanged:  aiResult.FilesChanged,
        CommitSHA:     commitSHA,
        CIStatus:      ciResult.Status,
    }, nil
}

// getAgentModel resolves agent/model from operations.ci_failure config
func (h *CIRetryHandler) getAgentModel() (domain.Agent, string) {
    if h.operationsConfig == nil {
        return domain.AgentClaude, "sonnet" // Fallback defaults
    }
    opConfig := h.operationsConfig.CIFailure
    agent := domain.Agent(opConfig.Agent)
    if agent == "" {
        agent = domain.AgentClaude
    }
    model := opConfig.Model
    if model == "" {
        model = agent.DefaultModel()
    }
    return agent, model
}

// getMaxAttempts returns max attempts from operations.ci_failure config
func (h *CIRetryHandler) getMaxAttempts() int {
    if h.operationsConfig != nil && h.operationsConfig.CIFailure.MaxAttempts > 0 {
        return h.operationsConfig.CIFailure.MaxAttempts
    }
    return 2 // Default
}

// parseAIAction parses AI output for re-run recommendation
type AIAction struct {
    Type   string // "rerun" or ""
    Reason string
}

func parseAIAction(output string) AIAction {
    // Look for {"action": "rerun", "reason": "..."} in AI output
    // Returns empty AIAction if not found (AI fixed code instead)
    var action AIAction
    if idx := strings.Index(output, `"action"`); idx != -1 {
        // Parse JSON action from output
        // Implementation extracts action type and reason
    }
    return action
}

// triggerRerun triggers a CI re-run without code changes (AI determined failure is transient)
func (h *CIRetryHandler) triggerRerun(
    ctx context.Context,
    logs *CILogs,
    retryCtx *CIRetryContext,
    reason string,
) (*CIRetryResult, error) {
    h.logger.Info().
        Str("reason", reason).
        Msg("AI recommends re-run for transient CI failure")

    // Get the run ID from the first failed check
    if len(logs.FailedChecks) == 0 {
        return nil, ErrNoFailedChecks
    }

    runID := logs.FailedChecks[0].RunID

    // Trigger re-run of failed jobs only
    err := h.hubRunner.RerunFailedJobs(ctx, runID)
    if err != nil {
        return nil, fmt.Errorf("trigger re-run: %w", err)
    }

    // Wait for CI to complete
    ciResult, err := h.waitForCI(ctx, retryCtx.PRNumber)
    if err != nil {
        return nil, fmt.Errorf("wait for CI after re-run: %w", err)
    }

    return &CIRetryResult{
        Success:        ciResult.Status == git.CIStatusSuccess,
        AttemptNumber:  retryCtx.AttemptNumber,
        Action:         CIRetryActionRerun,
        RerunTriggered: true,
        CIStatus:       ciResult.Status,
    }, nil
}

// commitAndPush commits AI changes and pushes to remote
// Includes concurrent edit detection to prevent conflicts if user made manual changes
func (h *CIRetryHandler) commitAndPush(
    ctx context.Context,
    workDir string,
    retryCtx *CIRetryContext,
) (string, error) {
    // CONCURRENT EDIT DETECTION: Check if HEAD changed since retry started
    // This prevents overwriting manual fixes the user may have made
    currentHEAD, err := h.gitOps.GetHEAD(ctx, workDir)
    if err != nil {
        return "", fmt.Errorf("get HEAD: %w", err)
    }

    if currentHEAD != retryCtx.StartedAtSHA {
        h.logger.Warn().
            Str("expected", retryCtx.StartedAtSHA).
            Str("actual", currentHEAD).
            Msg("HEAD changed during CI retry - aborting to prevent conflicts")
        return "", ErrHeadChanged
    }

    // Stage all changes
    if err := h.gitOps.StageAll(ctx, workDir); err != nil {
        return "", fmt.Errorf("stage changes: %w", err)
    }

    // Create commit with descriptive message
    msg := fmt.Sprintf("fix(ci): %s [attempt %d/%d]\n\nAuto-fix for CI failure: %s",
        retryCtx.ErrorSummary,
        retryCtx.AttemptNumber,
        retryCtx.MaxAttempts,
        strings.Join(retryCtx.FailedChecks, ", "))

    sha, err := h.gitOps.Commit(ctx, workDir, msg)
    if err != nil {
        return "", fmt.Errorf("commit: %w", err)
    }

    // Push to remote
    if err := h.gitOps.Push(ctx, workDir); err != nil {
        return "", fmt.Errorf("push: %w", err)
    }

    return sha, nil
}

// waitForCI polls CI status until completion
func (h *CIRetryHandler) waitForCI(ctx context.Context, prNumber int) (*git.CIWatchResult, error) {
    // Use existing CI watcher with reasonable timeout
    return h.ciWatcher.WatchPRChecks(ctx, prNumber, git.CIWatchConfig{
        PollInterval: 30 * time.Second,
        Timeout:      15 * time.Minute,
        GracePeriod:  30 * time.Second,
    })
}
```

---

### 4. Engine Integration (`internal/task/engine_ci_retry.go`)

**Purpose**: Integrate CI retry into the task engine, mirroring `engine_validation_retry.go`.

```go
// CIRetryHandler interface for dependency injection
type CIRetryHandler interface {
    RetryWithAI(ctx context.Context, prNumber int, failedChecks []git.CheckResult,
        workDir string, attemptNum int, runnerConfig *domain.RunnerConfig,
        agent, model string) (*ci.CIRetryResult, error)
    CanRetry(attemptNum int) bool
    MaxAttempts() int
    IsEnabled() bool
}

// shouldAttemptCIRetry determines if automatic CI retry should be attempted
func (e *Engine) shouldAttemptCIRetry(result *domain.StepResult) bool {
    // 1. Check handler exists and is enabled
    if e.ciRetryHandler == nil || !e.ciRetryHandler.IsEnabled() {
        return false
    }

    // 2. Check result contains CI failure metadata
    if result.Metadata == nil {
        return false
    }

    ciResult, ok := result.Metadata["ci_result"].(*git.CIWatchResult)
    if !ok || ciResult == nil {
        return false
    }

    // 3. Only retry actual failures, not timeouts or fetch errors
    return ciResult.Status == git.CIStatusFailure
}

// tryCIRetry attempts to retry a failed CI step with AI assistance
func (e *Engine) tryCIRetry(
    ctx context.Context,
    task *domain.Task,
    result *domain.StepResult,
) (*domain.StepResult, error) {
    if !e.shouldAttemptCIRetry(result) {
        return nil, nil // No retry attempted
    }

    retryResult, err := e.attemptCIRetry(ctx, task, result)
    if err != nil {
        e.logger.Error().Err(err).Msg("CI retry failed")
        return nil, err
    }

    if retryResult != nil && retryResult.Success {
        return e.convertCIRetryResultToStepResult(retryResult, result), nil
    }

    return nil, nil
}

// attemptCIRetry executes the CI retry loop
func (e *Engine) attemptCIRetry(
    ctx context.Context,
    task *domain.Task,
    result *domain.StepResult,
) (*ci.CIRetryResult, error) {
    // Extract CI result from metadata
    ciResult := result.Metadata["ci_result"].(*git.CIWatchResult)
    prNumber := extractPRNumber(task.Metadata)
    workDir := extractWorkDir(task.Metadata)

    if prNumber == 0 {
        return nil, ErrNoPRNumber
    }
    if workDir == "" {
        return nil, ErrNoWorkDir
    }

    // Get current attempt from task metadata
    currentAttempt := 1
    if attempt, ok := task.Metadata["ci_retry_attempt"].(int); ok {
        currentAttempt = attempt
    }

    maxAttempts := e.ciRetryHandler.MaxAttempts()
    var lastResult *ci.CIRetryResult
    var lastErr error

    for attempt := currentAttempt; attempt <= maxAttempts; attempt++ {
        if !e.ciRetryHandler.CanRetry(attempt) {
            break
        }

        // Update attempt counter
        task.Metadata["ci_retry_attempt"] = attempt

        // Notify progress
        e.notifyCIRetryAttempt(task, attempt, maxAttempts)

        // Get AI config from task
        agent := extractAgent(task.Metadata)
        model := extractModel(task.Metadata)
        runnerConfig := extractRunnerConfig(task.Metadata)

        // Attempt retry
        e.notifyCIRetryAIStart(task, attempt, maxAttempts)

        lastResult, lastErr = e.ciRetryHandler.RetryWithAI(
            ctx,
            prNumber,
            ciResult.CheckResults,
            workDir,
            attempt,
            runnerConfig,
            agent,
            model,
        )

        e.notifyCIRetryAIComplete(task, attempt, lastResult)

        if lastErr == nil && lastResult != nil && lastResult.Success {
            return lastResult, nil
        }

        // Update ciResult for next iteration if we got new results
        if lastResult != nil && lastResult.CIStatus != "" {
            ciResult.Status = lastResult.CIStatus
        }
    }

    return lastResult, lastErr
}

// Progress notifications (mirrors validation retry)
func (e *Engine) notifyCIRetryAttempt(task *domain.Task, attempt, max int) {
    if e.progressCallback != nil {
        e.progressCallback(StepProgressEvent{
            TaskID:    task.ID,
            StepIndex: task.CurrentStep,
            Event:     "ci_retry",
            Message:   fmt.Sprintf("CI retry attempt %d/%d", attempt, max),
        })
    }
}

func (e *Engine) notifyCIRetryAIStart(task *domain.Task, attempt, max int) {
    if e.progressCallback != nil {
        e.progressCallback(StepProgressEvent{
            TaskID:    task.ID,
            StepIndex: task.CurrentStep,
            Event:     "ci_retry_ai_start",
            Message:   fmt.Sprintf("CI retry %d/%d: AI analyzing failure", attempt, max),
        })
    }
}

func (e *Engine) notifyCIRetryAIComplete(task *domain.Task, attempt int, result *ci.CIRetryResult) {
    if e.progressCallback != nil {
        metadata := map[string]any{
            "attempt": attempt,
            "action":  string(result.Action),
        }
        if result.AIResult != nil {
            metadata["duration_ms"] = result.AIResult.DurationMs
            metadata["files_changed"] = len(result.FilesChanged)
        }

        e.progressCallback(StepProgressEvent{
            TaskID:    task.ID,
            StepIndex: task.CurrentStep,
            Event:     "ci_retry_ai_complete",
            Message:   fmt.Sprintf("CI retry %d: %s", attempt, result.Action),
            Metadata:  metadata,
        })
    }
}
```

---

### 5. Prompt Template (`internal/prompts/templates/ci/retry.tmpl`)

**Note:** This prompt enables AI to analyze ALL CI failures and decide the appropriate action - either fix the code OR recommend a re-run for transient failures.

```
You are analyzing a CI/CD pipeline failure for a Go project.

## PR Information
- PR Number: #{{.PRNumber}}
- Retry Attempt: {{.AttemptNumber}} of {{.MaxAttempts}}

## Failed CI Checks
{{range .FailedChecks}}- {{.}}
{{end}}

## Error Type: {{.ErrorType}}

## Error Summary
{{.ErrorSummary}}

## Detailed Error Output
` + "```" + `
{{.ErrorDetails}}
` + "```" + `

{{if .AffectedFiles}}
## Potentially Affected Files
{{range .AffectedFiles}}- {{.}}
{{end}}
{{end}}

## Your Task

**First, determine the failure type:**

### 1. Transient Failure (infrastructure issue, not code)

If the failure is due to infrastructure or temporary issues, respond with ONLY this JSON:

` + "```json" + `
{"action": "rerun", "reason": "<brief explanation>"}
` + "```" + `

Common transient failures:
- Rate limiting ("API rate limit exceeded")
- Network errors ("ECONNRESET", "connection reset", "timeout")
- Service unavailable ("503", "service temporarily unavailable")
- Resource temporarily unavailable
- Flaky test (passes locally, random CI failures with no code changes)
- Download/dependency fetch failures

### 2. Code Failure (requires fix)

If the failure is due to actual code issues, fix the code directly. Do NOT output the JSON action - just make the fixes.

Code failures include:
- Lint errors, test failures, build errors
- Type mismatches, missing imports
- Logic bugs caught by tests
- Formatting issues

## Guidelines for Code Fixes

- Make **minimal, targeted changes** - don't refactor unrelated code
- Follow the project's existing code style and conventions
- If multiple files are affected, fix them all
- If the error is ambiguous, check similar code in the project for patterns
- Do NOT add excessive comments or documentation
- Do NOT modify test assertions to make them pass - fix the actual code
- Run local validation after your fix

{{if eq .ErrorType "lint"}}
## Lint-Specific Guidance
- Focus on the specific linter rules mentioned in the error
- Common issues: unused variables, missing error checks, formatting
- Run ` + "`magex lint`" + ` locally after your fix to verify
{{end}}

{{if eq .ErrorType "test"}}
## Test-Specific Guidance
- Read the test failure carefully to understand expected vs actual behavior
- The test is likely correct - fix the implementation, not the test
- Check for race conditions, timing issues, or environment assumptions
- Run ` + "`magex test`" + ` locally after your fix to verify
{{end}}

{{if eq .ErrorType "build"}}
## Build-Specific Guidance
- Check for type errors, missing imports, or syntax issues
- Ensure all dependencies are properly imported
- Run ` + "`go build ./...`" + ` locally after your fix to verify
{{end}}

Analyze the failure and take the appropriate action now.
```

**Prompt Data Struct:**

```go
// In internal/prompts/types.go
type CIRetryData struct {
    PRNumber      int
    FailedChecks  []string
    ErrorType     string
    ErrorSummary  string
    ErrorDetails  string
    AffectedFiles []string
    AttemptNumber int
    MaxAttempts   int
}
```

---

### 6. Configuration

**IMPORTANT:** Leverages existing `operations.ci_failure` config (see [config/defaults.go](internal/config/defaults.go)). Only add CI-specific fields to `CIConfig`.

#### Existing Config (Already Present)

```go
// In internal/config/config.go - ALREADY EXISTS
type OperationsConfig struct {
    // ... other operations
    CIFailure OperationAIConfig `yaml:"ci_failure" json:"ci_failure"`
}

type OperationAIConfig struct {
    Agent       string        `yaml:"agent" json:"agent"`
    Model       string        `yaml:"model" json:"model"`
    Timeout     time.Duration `yaml:"timeout" json:"timeout"`
    MaxAttempts int           `yaml:"max_attempts" json:"max_attempts"` // Add if not present
}
```

#### New Config Fields (Add to CIConfig)

```go
// In internal/config/config.go - ADD these fields to existing CIConfig
type CIConfig struct {
    // Existing fields (keep as-is)...
    PollInterval      time.Duration `yaml:"poll_interval" json:"poll_interval"`
    GracePeriod       time.Duration `yaml:"grace_period" json:"grace_period"`
    Timeout           time.Duration `yaml:"timeout" json:"timeout"`
    RequiredWorkflows []string      `yaml:"required_workflows" json:"required_workflows"`

    // NEW: CI Retry settings (AI agent/model/timeout come from operations.ci_failure)
    RetryEnabled      bool          `yaml:"retry_enabled" json:"retry_enabled"`           // Default: true
    AutoCommitEnabled bool          `yaml:"auto_commit_enabled" json:"auto_commit_enabled"` // Default: false (require approval)
    LogFetchTimeout   time.Duration `yaml:"log_fetch_timeout" json:"log_fetch_timeout"`   // Default: 30s
}
```

#### Config File Example

```yaml
# Simplified config - no duplicate agent/model/timeout settings
ci:
  retry_enabled: true           # Enable AI-assisted CI retry (default: true)
  auto_commit_enabled: false    # Auto-push without approval (default: false for safety)
  log_fetch_timeout: 30s        # Timeout for fetching CI logs

operations:
  ci_failure:                   # EXISTING - reuse for AI settings
    agent: claude
    model: sonnet
    timeout: 10m
    max_attempts: 2             # Maximum retry attempts
```

**Why this approach:**
- **Avoids duplicate config:** `operations.ci_failure` already defines agent/model/timeout
- **Follows ATLAS patterns:** Other operations (validation, etc.) use `OperationsConfig`
- **Simpler UX:** Users don't need to configure AI settings in multiple places
- **AI always consulted:** No `auto_rerun_enabled` - AI analyzes all failures and decides action

---

### 7. Hub Runner Extensions (`internal/git/hub_runner.go`)

Add methods to existing `HubRunner` interface:

```go
type HubRunner interface {
    // Existing methods...

    // NEW: CI log and re-run methods
    FetchRunLogs(ctx context.Context, runID int64) (string, error)
    RerunFailedJobs(ctx context.Context, runID int64) error
    RerunWorkflow(ctx context.Context, runID int64) error
}

// Implementation
func (h *DefaultHubRunner) FetchRunLogs(ctx context.Context, runID int64) (string, error) {
    args := []string{"run", "view", fmt.Sprintf("%d", runID), "--log-failed"}
    output, err := h.execute(ctx, args...)
    if err != nil {
        return "", fmt.Errorf("fetch run logs: %w", err)
    }
    return string(output), nil
}

func (h *DefaultHubRunner) RerunFailedJobs(ctx context.Context, runID int64) error {
    args := []string{"run", "rerun", fmt.Sprintf("%d", runID), "--failed"}
    _, err := h.execute(ctx, args...)
    return err
}

func (h *DefaultHubRunner) RerunWorkflow(ctx context.Context, runID int64) error {
    args := []string{"run", "rerun", fmt.Sprintf("%d", runID)}
    _, err := h.execute(ctx, args...)
    return err
}
```

---

## Test Coverage Plan

### 1. Unit Tests: Log Fetcher (`internal/ci/log_fetcher_test.go`)

```go
func TestLogFetcher_FetchFailedLogs(t *testing.T) {
    tests := []struct {
        name           string
        prNumber       int
        mockResponse   string
        mockError      error
        expectedChecks int
        expectError    bool
    }{
        {
            name:     "successful_fetch_multiple_failures",
            prNumber: 123,
            mockResponse: `[
                {"name":"CI / lint","state":"FAILURE","runId":1001},
                {"name":"CI / test","state":"FAILURE","runId":1002},
                {"name":"CI / build","state":"SUCCESS","runId":1003}
            ]`,
            expectedChecks: 2,
        },
        {
            name:         "no_failed_checks",
            prNumber:     123,
            mockResponse: `[{"name":"CI / lint","state":"SUCCESS","runId":1001}]`,
            expectError:  true, // ErrNoFailedChecks
        },
        {
            name:        "gh_cli_error",
            prNumber:    123,
            mockError:   errors.New("gh: not authenticated"),
            expectError: true,
        },
        {
            name:        "invalid_pr_number",
            prNumber:    0,
            expectError: true,
        },
        {
            name:     "log_truncation",
            prNumber: 123,
            // Test that logs exceeding maxLogSize are truncated
        },
    }
    // ... implementation
}

func TestLogFetcher_CanRerun(t *testing.T) {
    tests := []struct {
        name           string
        logs           string
        expectedRerun  bool
        expectedReason string
    }{
        {
            name:           "rate_limit_error",
            logs:           "Error: API rate limit exceeded",
            expectedRerun:  true,
            expectedReason: "GitHub API rate limit exceeded",
        },
        {
            name:           "network_error",
            logs:           "ECONNRESET: connection reset by peer",
            expectedRerun:  true,
            expectedReason: "Connection reset/timeout",
        },
        {
            name:          "actual_code_error",
            logs:          "main.go:47: undefined: foo",
            expectedRerun: false,
        },
        {
            name:          "test_failure",
            logs:          "FAIL: TestSomething expected 1 got 2",
            expectedRerun: false,
        },
    }
    // ... implementation
}
```

### 2. Unit Tests: Error Context (`internal/ci/error_context_test.go`)

```go
func TestExtractCIErrorContext(t *testing.T) {
    tests := []struct {
        name              string
        logs              *CILogs
        expectedType      CIErrorType
        expectedFiles     []string
        expectedTransient bool
    }{
        {
            name: "lint_error_detection",
            logs: &CILogs{
                FailedChecks: []FailedCheck{{Name: "CI / lint", Logs: "main.go:47: exported function Foo should have comment"}},
            },
            expectedType:  CIErrorTypeLint,
            expectedFiles: []string{"main.go"},
        },
        {
            name: "test_error_detection",
            logs: &CILogs{
                FailedChecks: []FailedCheck{{Name: "CI / test", Logs: "--- FAIL: TestParser (0.01s)\n    parser_test.go:23: expected nil, got error"}},
            },
            expectedType:  CIErrorTypeTest,
            expectedFiles: []string{"parser_test.go"},
        },
        {
            name: "build_error_detection",
            logs: &CILogs{
                FailedChecks: []FailedCheck{{Name: "CI / build", Logs: "internal/foo/bar.go:12:5: undefined: SomeFunc"}},
            },
            expectedType:  CIErrorTypeBuild,
            expectedFiles: []string{"internal/foo/bar.go"},
        },
        {
            name: "transient_rate_limit",
            logs: &CILogs{
                FailedChecks: []FailedCheck{{Logs: "Error: API rate limit exceeded for installation"}},
            },
            expectedTransient: true,
        },
    }
    // ... implementation
}

func TestExtractAffectedFiles(t *testing.T) {
    tests := []struct {
        name          string
        logContent    string
        expectedFiles []string
    }{
        {
            name:          "go_compiler_error",
            logContent:    "internal/task/engine.go:123: undefined: foo",
            expectedFiles: []string{"internal/task/engine.go"},
        },
        {
            name:          "multiple_files",
            logContent:    "cmd/main.go:10: error\ninternal/pkg/util.go:20: error",
            expectedFiles: []string{"cmd/main.go", "internal/pkg/util.go"},
        },
        {
            name:          "golangci_lint_format",
            logContent:    "internal/ai/claude.go:45:12: ineffectual assignment to err",
            expectedFiles: []string{"internal/ai/claude.go"},
        },
        {
            name:          "no_files",
            logContent:    "Error: something went wrong",
            expectedFiles: []string{},
        },
    }
    // ... implementation
}
```

### 3. Unit Tests: Retry Handler (`internal/ci/retry_handler_test.go`)

```go
func TestCIRetryHandler_RetryWithAI(t *testing.T) {
    tests := []struct {
        name           string
        setupMocks     func(*mockLogFetcher, *mockAIRunner, *mockGitOps, *mockCIWatcher)
        prNumber       int
        attemptNum     int
        expectedAction CIRetryAction
        expectedErr    error
    }{
        {
            name: "successful_fix",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {
                lf.logs = &CILogs{FailedChecks: []FailedCheck{{Name: "lint"}}}
                ai.result = &domain.AIResult{Success: true, FilesChanged: []string{"main.go"}}
                git.commitSHA = "abc123"
                ci.result = &git.CIWatchResult{Status: git.CIStatusSuccess}
            },
            prNumber:       123,
            attemptNum:     1,
            expectedAction: CIRetryActionFix,
        },
        {
            name: "transient_failure_rerun",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {
                lf.logs = &CILogs{
                    FailedChecks: []FailedCheck{{Name: "test", RunID: 1001, Logs: "rate limit exceeded"}},
                }
                ci.result = &git.CIWatchResult{Status: git.CIStatusSuccess}
            },
            prNumber:       123,
            attemptNum:     1,
            expectedAction: CIRetryActionRerun,
        },
        {
            name: "ai_makes_no_changes",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {
                lf.logs = &CILogs{FailedChecks: []FailedCheck{{Name: "test"}}}
                ai.result = &domain.AIResult{Success: true, FilesChanged: []string{}} // No changes
            },
            prNumber:       123,
            attemptNum:     1,
            expectedAction: CIRetryActionNone,
            expectedErr:    ErrAINoChanges,
        },
        {
            name: "max_attempts_exceeded",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {},
            prNumber:       123,
            attemptNum:     10, // Exceeds max
            expectedErr:    ErrMaxCIRetriesExceeded,
        },
        {
            name: "log_fetch_timeout",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {
                lf.err = context.DeadlineExceeded
            },
            prNumber:    123,
            attemptNum:  1,
            expectedErr: context.DeadlineExceeded,
        },
        {
            name: "push_failure",
            setupMocks: func(lf *mockLogFetcher, ai *mockAIRunner, git *mockGitOps, ci *mockCIWatcher) {
                lf.logs = &CILogs{FailedChecks: []FailedCheck{{Name: "lint"}}}
                ai.result = &domain.AIResult{Success: true, FilesChanged: []string{"main.go"}}
                git.pushErr = errors.New("push rejected: non-fast-forward")
            },
            prNumber:    123,
            attemptNum:  1,
            expectedErr: errors.New("push rejected"),
        },
    }
    // ... implementation
}

func TestCIRetryHandler_DisabledConfig(t *testing.T) {
    handler := NewCIRetryHandler(nil, nil, nil, nil, nil, CIRetryConfig{Enabled: false}, zerolog.Nop())

    _, err := handler.RetryWithAI(context.Background(), 123, nil, "/tmp", 1, nil, "claude", "sonnet")

    require.ErrorIs(t, err, ErrCIRetryDisabled)
}
```

### 4. Integration Tests: Engine CI Retry (`internal/task/engine_ci_retry_test.go`)

```go
func TestEngine_tryCIRetry_Integration(t *testing.T) {
    tests := []struct {
        name           string
        setupTask      func() *domain.Task
        setupResult    func() *domain.StepResult
        setupHandler   func() *mockCIRetryHandler
        expectRetry    bool
        expectSuccess  bool
    }{
        {
            name: "retries_on_ci_failure",
            setupTask: func() *domain.Task {
                return &domain.Task{
                    ID:     "task-123",
                    Status: constants.TaskStatusRunning,
                    Metadata: map[string]any{
                        "pr_number":    123,
                        "worktree_dir": "/tmp/worktree",
                        "agent":        "claude",
                        "model":        "sonnet",
                    },
                }
            },
            setupResult: func() *domain.StepResult {
                return &domain.StepResult{
                    Success: false,
                    Metadata: map[string]any{
                        "ci_result": &git.CIWatchResult{
                            Status: git.CIStatusFailure,
                            CheckResults: []git.CheckResult{
                                {Name: "CI / lint", State: "FAILURE"},
                            },
                        },
                    },
                }
            },
            setupHandler: func() *mockCIRetryHandler {
                return &mockCIRetryHandler{
                    enabled:     true,
                    maxAttempts: 2,
                    result: &ci.CIRetryResult{
                        Success: true,
                        Action:  ci.CIRetryActionFix,
                    },
                }
            },
            expectRetry:   true,
            expectSuccess: true,
        },
        {
            name: "skips_retry_on_timeout",
            setupResult: func() *domain.StepResult {
                return &domain.StepResult{
                    Metadata: map[string]any{
                        "ci_result": &git.CIWatchResult{
                            Status: git.CIStatusTimeout, // Not a failure
                        },
                    },
                }
            },
            expectRetry: false,
        },
        {
            name: "skips_retry_when_disabled",
            setupHandler: func() *mockCIRetryHandler {
                return &mockCIRetryHandler{enabled: false}
            },
            expectRetry: false,
        },
    }
    // ... implementation
}

func TestEngine_shouldAttemptCIRetry(t *testing.T) {
    tests := []struct {
        name     string
        handler  CIRetryHandler
        result   *domain.StepResult
        expected bool
    }{
        {
            name:     "nil_handler",
            handler:  nil,
            result:   &domain.StepResult{},
            expected: false,
        },
        {
            name:     "handler_disabled",
            handler:  &mockCIRetryHandler{enabled: false},
            result:   &domain.StepResult{},
            expected: false,
        },
        {
            name:    "nil_metadata",
            handler: &mockCIRetryHandler{enabled: true},
            result: &domain.StepResult{
                Metadata: nil,
            },
            expected: false,
        },
        {
            name:    "missing_ci_result",
            handler: &mockCIRetryHandler{enabled: true},
            result: &domain.StepResult{
                Metadata: map[string]any{},
            },
            expected: false,
        },
        {
            name:    "ci_success_no_retry",
            handler: &mockCIRetryHandler{enabled: true},
            result: &domain.StepResult{
                Metadata: map[string]any{
                    "ci_result": &git.CIWatchResult{Status: git.CIStatusSuccess},
                },
            },
            expected: false,
        },
        {
            name:    "ci_failure_should_retry",
            handler: &mockCIRetryHandler{enabled: true},
            result: &domain.StepResult{
                Metadata: map[string]any{
                    "ci_result": &git.CIWatchResult{Status: git.CIStatusFailure},
                },
            },
            expected: true,
        },
    }
    // ... implementation
}
```

### 5. Mock Definitions

```go
// mockLogFetcher for testing
type mockLogFetcher struct {
    logs          *CILogs
    err           error
    canRerun      bool
    rerunReason   string
    fetchCalls    int
    canRerunCalls int
}

func (m *mockLogFetcher) FetchFailedLogs(ctx context.Context, prNumber int, checkNames []string) (*CILogs, error) {
    m.fetchCalls++
    return m.logs, m.err
}

func (m *mockLogFetcher) CanRerun(ctx context.Context, runID int64) (bool, string, error) {
    m.canRerunCalls++
    return m.canRerun, m.rerunReason, m.err
}

// mockCIRetryHandler for engine tests
type mockCIRetryHandler struct {
    enabled       bool
    maxAttempts   int
    result        *ci.CIRetryResult
    err           error
    retryCalls    int
    lastPRNumber  int
    lastAttempt   int
}

func (m *mockCIRetryHandler) RetryWithAI(ctx context.Context, prNumber int, failedChecks []git.CheckResult, workDir string, attemptNum int, runnerConfig *domain.RunnerConfig, agent, model string) (*ci.CIRetryResult, error) {
    m.retryCalls++
    m.lastPRNumber = prNumber
    m.lastAttempt = attemptNum
    return m.result, m.err
}

func (m *mockCIRetryHandler) CanRetry(attemptNum int) bool {
    return attemptNum <= m.maxAttempts
}

func (m *mockCIRetryHandler) MaxAttempts() int { return m.maxAttempts }
func (m *mockCIRetryHandler) IsEnabled() bool  { return m.enabled }
```

---

## Verification Plan

### Manual Testing Checklist

1. **Happy Path: Lint Failure**
   ```bash
   # Introduce lint error, push, wait for CI failure
   atlas start "fix lint" -t task
   # Verify: AI fetches logs, identifies lint error, fixes, pushes, CI passes
   ```

2. **Happy Path: Test Failure**
   ```bash
   # Break a test, push
   atlas start "fix test" -t task
   # Verify: AI identifies test failure, fixes implementation, CI passes
   ```

3. **Transient Failure Re-run**
   ```bash
   # Simulate rate limit (may need to mock)
   # Verify: System detects transient, triggers re-run, no code changes
   ```

4. **Max Attempts Exhausted**
   ```bash
   # Create unfixable error
   # Verify: System tries max attempts, then falls back to ci_failed state
   ```

5. **Disabled via Config**
   ```yaml
   ci:
     retry_enabled: false
   ```
   ```bash
   # Verify: CI failures go directly to ci_failed without retry attempt
   ```

### Automated Test Commands

```bash
# Run all CI retry tests
go test ./internal/ci/... -v

# Run engine integration tests
go test ./internal/task/... -run CIRetry -v

# Run with race detection
go test ./internal/ci/... ./internal/task/... -race

# Coverage report
go test ./internal/ci/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## Artifact Saving

**Purpose:** Save CI retry artifacts for debugging, audit trail, and state recovery (mirrors validation retry pattern).

### Artifact Structure

```go
// CIRetryArtifact captures retry attempt data for persistence
type CIRetryArtifact struct {
    Attempt       int       `json:"attempt"`
    Timestamp     string    `json:"timestamp"`
    PRNumber      int       `json:"pr_number"`
    FailedChecks  []string  `json:"failed_checks"`
    ErrorType     string    `json:"error_type"`
    LogsFetched   bool      `json:"logs_fetched"`
    ErrorContext  string    `json:"error_context"`
    AIPrompt      string    `json:"ai_prompt"`       // For debugging
    AIAction      string    `json:"ai_action"`       // "fix" or "rerun"
    FilesChanged  []string  `json:"files_changed"`
    CommitSHA     string    `json:"commit_sha,omitempty"`
    CIStatus      string    `json:"ci_status"`
    Success       bool      `json:"success"`
    Error         string    `json:"error,omitempty"`
}
```

### Storage Location

Artifacts are stored in the task artifact directory:

```
.atlas/tasks/{task-id}/artifacts/
└── {step-name}/
    ├── ci-retry-1.json       # First attempt
    ├── ci-retry-2.json       # Second attempt (if needed)
    └── ci-logs-{attempt}.txt # Raw CI logs (optional, for debugging)
```

### Saving Artifacts

```go
func (h *CIRetryHandler) saveArtifact(
    artifactDir string,
    attempt int,
    retryCtx *CIRetryContext,
    result *CIRetryResult,
    prompt string,
) error {
    artifact := CIRetryArtifact{
        Attempt:      attempt,
        Timestamp:    time.Now().Format(time.RFC3339),
        PRNumber:     retryCtx.PRNumber,
        FailedChecks: retryCtx.FailedChecks,
        ErrorType:    string(retryCtx.ErrorType),
        LogsFetched:  true,
        ErrorContext: retryCtx.ErrorSummary,
        AIPrompt:     prompt,
        AIAction:     string(result.Action),
        FilesChanged: result.FilesChanged,
        CommitSHA:    result.CommitSHA,
        CIStatus:     string(result.CIStatus),
        Success:      result.Success,
    }

    if result.Error != nil {
        artifact.Error = result.Error.Error()
    }

    filename := fmt.Sprintf("ci-retry-%d.json", attempt)
    return saveJSON(filepath.Join(artifactDir, filename), artifact)
}
```

### Benefits

1. **Debugging:** Full context when things go wrong
2. **Audit trail:** Record of all AI actions and decisions
3. **State recovery:** Resume from specific retry attempt after crash
4. **Metrics:** Collect data on retry success rates, common error types

---

## Integration Tests

**Purpose:** End-to-end tests that verify complete CI retry flows.

### Test Scenarios

```go
// E2E flow tests (use testcontainers or mock gh CLI)
func TestCIRetry_EndToEnd_LintFailure_AIFixes(t *testing.T) {
    // 1. Create task with failing lint CI
    // 2. Verify handler fetches logs
    // 3. Verify AI is invoked with correct prompt
    // 4. Verify AI changes are committed (or approval requested)
    // 5. Verify CI is re-polled
    // 6. Verify success result
}

func TestCIRetry_EndToEnd_TransientFailure_AIRecommendsRerun(t *testing.T) {
    // 1. Create task with rate-limit failure logs
    // 2. Verify AI analyzes and outputs {"action": "rerun", ...}
    // 3. Verify re-run is triggered (no code changes)
    // 4. Verify CI is re-polled
}

func TestCIRetry_EndToEnd_MaxAttemptsExhausted(t *testing.T) {
    // 1. Create unfixable error
    // 2. Verify all attempts are made
    // 3. Verify transition to CIFailed state
    // 4. Verify menu is shown to user
}

func TestCIRetry_EndToEnd_ApprovalFlow(t *testing.T) {
    // 1. Create task with auto_commit_enabled=false
    // 2. Verify AI makes changes
    // 3. Verify RequiresApproval=true in result
    // 4. Verify no push until approved
}

// State persistence tests
func TestCIRetry_TaskResume_AfterCrash(t *testing.T) {
    // 1. Start retry, save artifact at attempt 1
    // 2. Simulate crash
    // 3. Resume task
    // 4. Verify retry continues from attempt 2
}

func TestCIRetry_HookState_TransitionsDuringRetry(t *testing.T) {
    // 1. Verify hook state during retry flow
    // 2. Verify metadata keys are updated correctly
}

// Edge case tests
func TestCIRetry_ConcurrentManualFix_DetectsHeadChange(t *testing.T) {
    // 1. Start retry
    // 2. Simulate user pushing manual fix (HEAD changes)
    // 3. Verify ErrHeadChanged is returned
    // 4. Verify no conflicting push
}

func TestCIRetry_LogsExpired_GracefulDegradation(t *testing.T) {
    // 1. Simulate gh returning "logs expired" error
    // 2. Verify graceful error handling
    // 3. Verify fallback to CIFailed state
}

func TestCIRetry_BranchProtection_HandlesRejection(t *testing.T) {
    // 1. Simulate push rejected due to branch protection
    // 2. Verify error is surfaced clearly
    // 3. Verify retry doesn't loop infinitely
}
```

### Test Utilities

```go
// MockGHRunner simulates gh CLI responses
type MockGHRunner struct {
    PRChecksResponse string
    RunLogsResponse  string
    RerunError       error
}

func (m *MockGHRunner) Execute(args ...string) ([]byte, error) {
    switch args[0] {
    case "pr":
        return []byte(m.PRChecksResponse), nil
    case "run":
        if args[1] == "view" {
            return []byte(m.RunLogsResponse), nil
        }
        if args[1] == "rerun" {
            return nil, m.RerunError
        }
    }
    return nil, fmt.Errorf("unexpected command: %v", args)
}
```

---

## Implementation Order

1. **Phase 1: Types & Interfaces** (internal/ci/types.go, internal/task/ci_retry.go)
   - Define all types, interfaces, errors
   - No dependencies on implementation

2. **Phase 2: Log Fetcher** (internal/ci/log_fetcher.go)
   - Implement gh CLI integration
   - Full test coverage

3. **Phase 3: Error Context** (internal/ci/error_context.go)
   - Implement error parsing and classification
   - Full test coverage

4. **Phase 4: Prompt Template** (internal/prompts/templates/ci/retry.tmpl)
   - Create and test prompt rendering

5. **Phase 5: Retry Handler** (internal/ci/retry_handler.go)
   - Implement core retry logic
   - Full test coverage with mocks

6. **Phase 6: Engine Integration** (internal/task/engine_ci_retry.go)
   - Wire handler into engine
   - Integration tests

7. **Phase 7: Configuration** (internal/domain/config.go)
   - Add config options
   - Update config loading

8. **Phase 8: Hub Runner Extensions** (internal/git/hub_runner.go)
   - Add re-run methods
   - Test coverage

---

## Critical Files to Modify

| File | Changes |
|------|---------|
| **New Files** | |
| `internal/ci/types.go` | NEW - All CI retry types (CIRetryConfig, CIRetryResult, etc.) |
| `internal/ci/log_fetcher.go` | NEW - Log fetching via gh CLI |
| `internal/ci/error_context.go` | NEW - MOVE from task/ci_failure.go + enhance |
| `internal/ci/retry_handler.go` | NEW - Retry orchestration |
| `internal/contracts/git.go` | NEW - GitOperations interface |
| `internal/contracts/ci_retry.go` | NEW - CIRetryHandler interface |
| `internal/task/ci_retry.go` | NEW - Interface adapter for engine |
| `internal/task/engine_ci_retry.go` | NEW - Engine integration (tryCIRetry, shouldAttemptCIRetry) |
| `internal/prompts/templates/ci/retry.tmpl` | NEW - Prompt template (centralized location) |
| **Modified Files** | |
| `internal/task/engine.go` | MODIFY - Wire CI retry handler |
| `internal/task/ci_failure.go` | MODIFY - Backward-compat wrapper for ExtractCIErrorContext |
| `internal/template/steps/ci.go` | MODIFY - Store CI result in metadata |
| `internal/git/hub_runner.go` | MODIFY - Add FetchRunLogs, RerunFailedJobs, RerunWorkflow |
| `internal/git/github_ci.go` | MODIFY - Add RunID, JobID to CheckResult |
| `internal/config/config.go` | MODIFY - Add CI retry fields to CIConfig |
| `internal/config/defaults.go` | MODIFY - Add CI retry defaults, ensure max_attempts in OperationAIConfig |
| `internal/prompts/prompts.go` | MODIFY - Register CIRetry prompt |
| `internal/prompts/types.go` | MODIFY - Add CIRetryData struct |

---

## Success Criteria

1. **Zero manual intervention** for fixable CI failures (with `auto_commit_enabled=true`)
2. **Human authority preserved** - Default pause for approval before push (`auto_commit_enabled=false`)
3. **AI always consulted** - Even for transient failures, AI analyzes and decides action
4. **Graceful degradation** to `ci_failed` state (showing menu) when max attempts exceeded
5. **Concurrent edit safety** - Detect HEAD changes and abort to prevent conflicts
6. **Full test coverage** (>90%) for all new code
7. **No regressions** in existing CI monitoring behavior
8. **Configuration simplicity** - Reuse existing `operations.ci_failure` for agent/model/timeout
9. **Clear audit trail** via artifacts, progress events, and logs
10. **State recovery** - Resume from specific retry attempt after crash

---

## Implementation Requirements

### Documentation Updates

Update [docs/internal/quick-start.md](docs/internal/quick-start.md) with:

1. **CI Configuration section** - Add new CI retry config fields:
   ```yaml
   ci:
     # ... existing fields ...
     retry_enabled: true           # Enable AI-assisted CI retry (default: true)
     auto_commit_enabled: false    # Auto-push without approval (default: false)
     log_fetch_timeout: 30s        # Timeout for fetching CI logs
   ```

2. **Operations config section** - Document `max_attempts` field for `operations.ci_failure`:
   ```yaml
   operations:
     ci_failure:
       agent: claude
       model: sonnet
       timeout: 10m
       max_attempts: 2             # Maximum CI retry attempts
   ```

3. **Task States section** - Add `AwaitingApproval` checkpoint for CI retry flow

4. **Troubleshooting section** - Add common CI retry issues:
   - `ci_retry_disabled` - CI retry is disabled in config
   - `max_ci_retries_exceeded` - Maximum retry attempts reached
   - `head_changed` - Manual fix detected, retry aborted

### Validation During Implementation

Run validation after each phase to catch issues early:

```bash
# After each implementation phase:
magex format:fix && magex lint && magex test

# Or use atlas validate for full suite:
atlas validate
```

### Project Config Updates

Update [.atlas/config.yaml](.atlas/config.yaml) with new defaults:

```yaml
ci:
  # ... existing fields ...

  # NEW: CI Retry settings
  retry_enabled: true           # Enable AI-assisted CI retry
  auto_commit_enabled: false    # Require approval before push (safer default)
  log_fetch_timeout: 30s        # Timeout for fetching CI logs

operations:
  # ... existing operations ...

  ci_failure:
    agent: claude
    model: sonnet
    timeout: 10m
    max_attempts: 2             # Add if not present
```

### Checklist

Before marking implementation complete:

- [ ] All unit tests pass: `go test ./internal/ci/... -v`
- [ ] All integration tests pass: `go test ./internal/task/... -run CIRetry -v`
- [ ] Full validation passes: `magex format:fix && magex lint && magex test`
- [ ] Documentation updated: [docs/internal/quick-start.md](docs/internal/quick-start.md)
- [ ] Config updated: [.atlas/config.yaml](.atlas/config.yaml)
- [ ] No regressions in existing CI monitoring
- [ ] Manual test: Create failing CI, verify retry flow works
