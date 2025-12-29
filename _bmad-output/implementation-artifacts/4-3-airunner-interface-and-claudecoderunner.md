# Story 4.3: AIRunner Interface and ClaudeCodeRunner

Status: done

## Story

As a **developer**,
I want **an AIRunner interface with ClaudeCodeRunner implementation**,
So that **ATLAS can invoke Claude Code CLI and capture results**.

## Acceptance Criteria

1. **Given** the domain types exist **When** I implement `internal/ai/runner.go` and `internal/ai/claude.go` **Then** the AIRunner interface provides:
```go
type AIRunner interface {
    Run(ctx context.Context, req *AIRequest) (*AIResult, error)
}
```

2. **Given** AIRunner interface exists **When** ClaudeCodeRunner implementation is complete **Then** ClaudeCodeRunner:
   - Invokes `claude -p --output-format json --model <model>`
   - Passes prompts via stdin or -p flag
   - Supports `--permission-mode plan` for read-only analysis
   - Parses JSON response into AIResult struct
   - Captures session_id, duration_ms, num_turns, total_cost_usd
   - Handles timeouts via context
   - Supports `--max-turns` flag (Note: Not in current CLI - use `--max-budget-usd` instead if needed)
   - Supports `--append-system-prompt` for context injection

3. **Given** ClaudeCodeRunner invocation fails **When** error occurs **Then** errors are wrapped with ErrClaudeInvocation sentinel

4. **Given** transient errors occur **When** retries are configured **Then** retry logic with exponential backoff (3 attempts) is applied

5. **Given** ClaudeCodeRunner is implemented **When** running tests **Then** tests mock subprocess execution

## Tasks / Subtasks

- [x] Task 1: Create AIRunner interface (AC: #1)
  - [x] 1.1: Create `internal/ai/runner.go` file
  - [x] 1.2: Define `Runner` interface with `Run(ctx, *domain.AIRequest) (*domain.AIResult, error)`
  - [x] 1.3: Add interface documentation following Go conventions

- [x] Task 2: Create ClaudeCodeRunner struct and constructor (AC: #2)
  - [x] 2.1: Create `internal/ai/claude.go` file
  - [x] 2.2: Define `ClaudeCodeRunner` struct with config fields (model, timeout, maxTurns)
  - [x] 2.3: Add `CommandExecutor` interface for subprocess abstraction (testability)
  - [x] 2.4: Implement `NewClaudeCodeRunner(cfg *config.AIConfig, executor CommandExecutor) *ClaudeCodeRunner`
  - [x] 2.5: Implement default `DefaultExecutor` using `exec.CommandContext`

- [x] Task 3: Implement Run method - command building (AC: #2)
  - [x] 3.1: Check context cancellation at function entry
  - [x] 3.2: Build `claude` command with required flags:
    - `-p` (print mode, non-interactive)
    - `--output-format json`
    - `--model <model>` (from request or config)
    - `--permission-mode <mode>` (if specified in request)
    - `--append-system-prompt <prompt>` (if SystemPrompt specified)
  - [x] 3.3: Set working directory from request.WorkingDir
  - [x] 3.4: Create child context with timeout from request

- [x] Task 4: Implement Run method - execution and parsing (AC: #2, #3)
  - [x] 4.1: Pass prompt via stdin (safer for large prompts)
  - [x] 4.2: Execute command and capture stdout/stderr
  - [x] 4.3: Create `internal/ai/response.go` for Claude JSON response parsing
  - [x] 4.4: Define `ClaudeResponse` struct matching Claude's JSON output format
  - [x] 4.5: Parse JSON response into `ClaudeResponse`
  - [x] 4.6: Map `ClaudeResponse` to `domain.AIResult`
  - [x] 4.7: Handle non-zero exit codes appropriately

- [x] Task 5: Implement error handling (AC: #3)
  - [x] 5.1: Wrap all errors with `errors.ErrClaudeInvocation` sentinel
  - [x] 5.2: Include command output in error context for debugging
  - [x] 5.3: Handle specific error cases:
    - Context timeout/cancellation
    - Command not found (claude not installed)
    - JSON parse errors
    - API key missing/invalid
  - [x] 5.4: Create actionable error messages

- [x] Task 6: Implement retry logic (AC: #4)
  - [x] 6.1: Create `internal/ai/retry.go` for retry utilities
  - [x] 6.2: Implement exponential backoff per constants (3 attempts, 1s initial)
  - [x] 6.3: Define retryable vs non-retryable errors
  - [x] 6.4: Apply retry only to transient failures (network, rate limits)
  - [x] 6.5: Do NOT retry on: API key errors, parse errors, context cancellation

- [x] Task 7: Create request builder utilities (AC: #2)
  - [x] 7.1: Create `internal/ai/request.go` for request building helpers
  - [x] 7.2: Implement `NewAIRequest(prompt string, opts ...RequestOption) *domain.AIRequest`
  - [x] 7.3: Add functional options: WithModel, WithTimeout, WithPermissionMode, WithSystemPrompt, WithWorkingDir, WithContext
  - [x] 7.4: Implement defaults from config when options not specified

- [x] Task 8: Write comprehensive tests (AC: #5)
  - [x] 8.1: Create `internal/ai/runner_test.go` for interface tests
  - [x] 8.2: Create `internal/ai/claude_test.go` for ClaudeCodeRunner tests
  - [x] 8.3: Create mock `CommandExecutor` for subprocess testing
  - [x] 8.4: Test successful execution with JSON parsing
  - [x] 8.5: Test context timeout handling
  - [x] 8.6: Test context cancellation handling
  - [x] 8.7: Test error wrapping with ErrClaudeInvocation
  - [x] 8.8: Test retry logic with transient failures
  - [x] 8.9: Test non-retryable errors bypass retry
  - [x] 8.10: Test command building with various options
  - [x] 8.11: Test permission mode flag construction
  - [x] 8.12: Test working directory setting
  - [x] 8.13: Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Dev Notes

### Critical Warnings (READ FIRST)

1. **Domain types already exist**: `AIRequest` and `AIResult` are defined in `internal/domain/ai.go`. Use those types, do NOT redefine.

2. **Use existing errors**: Use `errors.ErrClaudeInvocation` from `internal/errors/errors.go`.

3. **Context as first parameter**: Always check `ctx.Done()` at function entry for long operations.

4. **Claude CLI current flags**: Based on `claude --help` output, the key flags are:
   - `-p, --print` - Print response and exit (non-interactive)
   - `--output-format json` - JSON output format
   - `--model <model>` - Model selection (e.g., "sonnet", "opus", "haiku")
   - `--permission-mode <mode>` - Choices: "acceptEdits", "bypassPermissions", "default", "delegate", "dontAsk", "plan"
   - `--append-system-prompt <prompt>` - Append to system prompt
   - `--max-budget-usd <amount>` - Maximum spend (NOT `--max-turns` as originally expected)

5. **No `--max-turns` flag**: The Claude CLI does NOT have a `--max-turns` flag. Consider using `--max-budget-usd` as an alternative constraint, or simply let the AI run to completion within the timeout.

6. **Subprocess abstraction is critical**: Use a `CommandExecutor` interface so tests can mock the subprocess without actually calling `claude`.

7. **JSON response structure**: Claude's JSON output includes fields like `type`, `subtype`, `is_error`, `result`, `session_id`, etc. - map these to `domain.AIResult`.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/ai/runner.go` | NEW - AIRunner interface definition |
| `internal/ai/claude.go` | NEW - ClaudeCodeRunner implementation |
| `internal/ai/response.go` | NEW - Claude JSON response parsing |
| `internal/ai/request.go` | NEW - Request building utilities |
| `internal/ai/retry.go` | NEW - Retry logic with exponential backoff |
| `internal/ai/runner_test.go` | NEW - Interface tests |
| `internal/ai/claude_test.go` | NEW - ClaudeCodeRunner tests |
| `internal/domain/ai.go` | REFERENCE - AIRequest, AIResult types |
| `internal/config/config.go` | REFERENCE - AIConfig struct |
| `internal/errors/errors.go` | REFERENCE - ErrClaudeInvocation |
| `internal/constants/constants.go` | REFERENCE - Timeout, retry constants |

### Import Rules (CRITICAL)

**`internal/ai/` MAY import:**
- `internal/constants` - for timeout and retry constants
- `internal/domain` - for AIRequest, AIResult types
- `internal/errors` - for ErrClaudeInvocation
- `internal/config` - for AIConfig type
- `context`, `encoding/json`, `fmt`, `io`, `os/exec`, `strings`, `time`

**MUST NOT import:**
- `internal/task` - avoid circular dependencies
- `internal/workspace` - avoid circular dependencies
- `internal/cli` - domain packages don't import CLI

### Claude CLI Command Structure

Based on current Claude CLI help output:

```bash
# Basic invocation with JSON output
claude -p --output-format json --model sonnet "Your prompt here"

# With permission mode for read-only analysis
claude -p --output-format json --model sonnet --permission-mode plan "Analyze this code..."

# With appended system prompt
claude -p --output-format json --model sonnet --append-system-prompt "You are working on..." "Do this task"

# With working directory (use cmd.Dir, not a flag)
# NOTE: Working directory is set via exec.Cmd.Dir, not a CLI flag
```

### Claude JSON Response Structure

From Architecture document:
```go
type ClaudeResponse struct {
    Type      string  `json:"type"`
    Subtype   string  `json:"subtype"`
    IsError   bool    `json:"is_error"`
    Result    string  `json:"result"`
    SessionID string  `json:"session_id"`
    Duration  int     `json:"duration_ms"`
    NumTurns  int     `json:"num_turns"`
    TotalCost float64 `json:"total_cost_usd"`
}
```

**Mapping to domain.AIResult:**
```go
result := &domain.AIResult{
    Success:      !resp.IsError,
    Output:       resp.Result,
    SessionID:    resp.SessionID,
    DurationMs:   resp.Duration,
    NumTurns:     resp.NumTurns,
    TotalCostUSD: resp.TotalCost,
    Error:        "", // Set from stderr if IsError true
    FilesChanged: nil, // Parse from result if available
}
```

### Implementation Patterns

**CommandExecutor Interface (for testability):**
```go
// CommandExecutor abstracts command execution for testing.
type CommandExecutor interface {
    Execute(ctx context.Context, cmd *exec.Cmd) (stdout, stderr []byte, err error)
}

// DefaultExecutor is the production implementation.
type DefaultExecutor struct{}

func (e *DefaultExecutor) Execute(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    err := cmd.Run()
    return stdout.Bytes(), stderr.Bytes(), err
}
```

**ClaudeCodeRunner struct:**
```go
type ClaudeCodeRunner struct {
    config   *config.AIConfig
    executor CommandExecutor
}

func NewClaudeCodeRunner(cfg *config.AIConfig, executor CommandExecutor) *ClaudeCodeRunner {
    if executor == nil {
        executor = &DefaultExecutor{}
    }
    return &ClaudeCodeRunner{
        config:   cfg,
        executor: executor,
    }
}
```

**Run method pattern:**
```go
func (r *ClaudeCodeRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
    // Check cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Create timeout context
    timeout := req.Timeout
    if timeout == 0 {
        timeout = r.config.Timeout
    }
    runCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    // Build and execute with retry
    return r.runWithRetry(runCtx, req)
}
```

**Retry pattern:**
```go
func (r *ClaudeCodeRunner) runWithRetry(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
    var lastErr error
    backoff := constants.InitialBackoff

    for attempt := 1; attempt <= constants.MaxRetryAttempts; attempt++ {
        result, err := r.execute(ctx, req)
        if err == nil {
            return result, nil
        }

        // Don't retry non-retryable errors
        if !isRetryable(err) {
            return nil, err
        }

        lastErr = err
        if attempt < constants.MaxRetryAttempts {
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(backoff):
                backoff *= 2 // Exponential backoff
            }
        }
    }

    return nil, fmt.Errorf("%w: max retries exceeded: %v", atlaserrors.ErrClaudeInvocation, lastErr)
}
```

**Retryable error determination:**
```go
func isRetryable(err error) bool {
    // Context errors are not retryable
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return false
    }

    // Check for specific non-retryable error messages
    errStr := err.Error()
    if strings.Contains(errStr, "authentication") ||
       strings.Contains(errStr, "api key") ||
       strings.Contains(errStr, "invalid json") {
        return false
    }

    // Retry transient failures (network, rate limits)
    return true
}
```

### Previous Story Learnings (from Story 4-2)

From Story 4-2 (Task State Machine):

1. **Use existing errors package** - Add errors to `internal/errors/errors.go`, not locally
2. **Context as first parameter** - Always check `ctx.Done()` at function entry
3. **Action-first error messages** - `"failed to invoke claude: %w"`
4. **Run `magex test:race`** - Race detection is mandatory
5. **Use `//nolint:gochecknoglobals` for lookup tables** - Already established pattern
6. **100% test coverage target** - Aim for comprehensive tests

### Dependencies Between Stories

This story **depends on:**
- **Story 4-1** (Task Data Model and Store) - uses domain types indirectly
- **Story 4-2** (Task State Machine) - uses error patterns

This story **is required for:**
- **Story 4-5** (Step Executor Framework) - uses AIRunner for AIExecutor
- **Story 4-6** (Task Engine Orchestrator) - orchestrates AI execution
- **Story 4-7** (atlas start command) - initiates AI execution

### Edge Cases to Handle

1. **Claude not installed** - Return clear error with install instructions
2. **API key not set** - Detect missing ANTHROPIC_API_KEY and provide actionable error
3. **Large prompts** - Use stdin for prompt delivery to avoid command-line length limits
4. **JSON parse failure** - Capture raw output in error for debugging
5. **Non-zero exit code** - May still have valid JSON output, try parsing first
6. **Empty response** - Handle gracefully with appropriate error
7. **Rate limiting** - Apply retry with backoff
8. **Network timeouts** - Apply retry with backoff

### Performance Considerations

1. **Subprocess overhead** - Minimal compared to AI execution time
2. **JSON parsing** - Single parse of response is efficient
3. **Retry backoff** - Uses exponential backoff to avoid hammering on failures
4. **Context propagation** - Ensures cancellation is responsive

### Security Considerations

1. **No API keys in logs** - Never log API key values
2. **No secrets in error messages** - Sanitize error output
3. **Working directory validation** - Ensure worktree path is valid
4. **Command injection prevention** - Use exec.CommandContext with proper args

### Testing Pattern

```go
// MockExecutor for testing
type MockExecutor struct {
    responses map[string]struct {
        stdout []byte
        stderr []byte
        err    error
    }
}

func (m *MockExecutor) Execute(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
    // Return pre-configured response based on command args
    key := strings.Join(cmd.Args, " ")
    if resp, ok := m.responses[key]; ok {
        return resp.stdout, resp.stderr, resp.err
    }
    return nil, nil, fmt.Errorf("unexpected command: %s", key)
}

func TestClaudeCodeRunner_Run_Success(t *testing.T) {
    mockExec := &MockExecutor{
        responses: map[string]struct {
            stdout, stderr []byte
            err            error
        }{
            "claude -p --output-format json --model sonnet": {
                stdout: []byte(`{"type":"result","is_error":false,"result":"Done","session_id":"abc123","duration_ms":5000,"num_turns":3,"total_cost_usd":0.05}`),
            },
        },
    }

    runner := NewClaudeCodeRunner(&config.AIConfig{
        Model:   "sonnet",
        Timeout: 30 * time.Minute,
    }, mockExec)

    req := &domain.AIRequest{
        Prompt: "Fix the bug",
        Model:  "sonnet",
    }

    result, err := runner.Run(context.Background(), req)
    require.NoError(t, err)
    assert.True(t, result.Success)
    assert.Equal(t, "Done", result.Output)
    assert.Equal(t, "abc123", result.SessionID)
}
```

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.3]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Tool Integration]
- [Source: _bmad-output/planning-artifacts/architecture.md#Core Architectural Decisions]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/domain/ai.go - AIRequest, AIResult types]
- [Source: internal/config/config.go - AIConfig struct]
- [Source: internal/errors/errors.go - ErrClaudeInvocation]
- [Source: internal/constants/constants.go - Timeout and retry constants]
- [Source: _bmad-output/implementation-artifacts/4-2-task-state-machine.md - Previous story patterns]

### Project Structure Notes

- AIRunner is a core interface in `internal/ai/`
- ClaudeCodeRunner is the production implementation
- Uses CommandExecutor interface for subprocess abstraction (testability)
- Follows same patterns as task store for context handling
- Uses constants from `internal/constants/` per architecture

## Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Smoke test (requires claude CLI and API key):
# NOTE: Integration tests should be tagged with //go:build integration
# Unit tests should mock the CommandExecutor

# Manual verification:
# - Review all command building matches Claude CLI help
# - Verify error messages are actionable
# - Ensure retry logic only applies to transient errors
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

No debug sessions required - implementation followed patterns from Story 4-2.

### Completion Notes List

1. **Interface naming:** Kept as `Runner` (not `AIRunner`) per Go naming conventions - `ai.Runner` avoids stuttering that `ai.AIRunner` would cause. Architecture docs refer to "AIRunner" conceptually.
2. **Test coverage:** Added 3 additional test cases to improve coverage from 86.1% to 91.8% (exceeds 90% target).
3. **No `--max-turns` flag:** Confirmed Claude CLI doesn't support `--max-turns`; documented in Dev Notes as expected.
4. **Context handling:** `CommandExecutor.Execute()` receives context for interface consistency, though context is embedded via `exec.CommandContext()`.
5. **timeSleep global:** Uses interface type `interface{ Nanoseconds() int64 }` to accept any duration-like type for test mocking flexibility.

### File List

| File | Purpose | Lines |
|------|---------|-------|
| `internal/ai/runner.go` | AIRunner interface definition | 29 |
| `internal/ai/claude.go` | ClaudeCodeRunner implementation | 243 |
| `internal/ai/request.go` | Request builder with functional options | 92 |
| `internal/ai/response.go` | Claude JSON response parsing and mapping | 73 |
| `internal/ai/retry.go` | Retry logic with exponential backoff | 57 |
| `internal/ai/runner_test.go` | Interface satisfaction tests | 59 |
| `internal/ai/claude_test.go` | ClaudeCodeRunner comprehensive tests | 592 |
| `internal/ai/request_test.go` | Request builder tests | 99 |
| `internal/ai/response_test.go` | Response parsing tests | 146 |
| `internal/ai/retry_test.go` | Retry determination tests | 111 |

