# Story 4.5: Step Executor Framework

Status: done

## Story

As a **developer**,
I want **a StepExecutor interface with implementations for each step type**,
So that **the task engine can execute different step types uniformly**.

## Acceptance Criteria

1. **Given** the AIRunner and templates exist **When** I implement `internal/template/steps/` **Then** the StepExecutor interface provides:
   ```go
   type StepExecutor interface {
       Execute(ctx context.Context, task *Task, step *Step) (*StepResult, error)
       Type() StepType
   }
   ```

2. **Given** the interface exists **When** I implement `ai.go` **Then** `AIExecutor` handles AI steps (analyze, implement):
   - Uses `ai.Runner` to execute prompts
   - Passes task context and step configuration
   - Captures session_id, duration_ms, num_turns in result
   - Respects permission_mode from step config

3. **Given** the interface exists **When** I implement `validation.go` **Then** `ValidationExecutor` handles validation steps:
   - Runs configured validation commands in order
   - Captures pass/fail status and output
   - Returns detailed error information on failure

4. **Given** the interface exists **When** I implement `git.go` **Then** `GitExecutor` handles git operations:
   - Supports operations: commit, push, create_pr
   - Uses step config to determine which operation
   - Returns appropriate result for each operation type

5. **Given** the interface exists **When** I implement `human.go` **Then** `HumanExecutor` handles approval checkpoints:
   - Signals that human review is required
   - Returns a special status indicating "awaiting approval"
   - Includes prompt from step config

6. **Given** the interface exists **When** I implement `sdd.go` **Then** `SDDExecutor` handles Speckit SDD steps:
   - Invokes Speckit via AI runner with sdd_command from config
   - Captures specification artifacts
   - Returns path to generated artifacts

7. **Given** the interface exists **When** I implement `ci.go` **Then** `CIExecutor` handles CI waiting:
   - Polls CI status at configured interval
   - Respects timeout from step definition
   - Returns success when all workflows pass
   - Returns failure with workflow details on failure

8. **Given** all executors exist **When** each executor runs **Then**:
   - Logs execution start/end with step context
   - Saves artifacts to task artifacts directory
   - Returns StepResult with output, files_changed, duration
   - Handles context cancellation correctly
   - Step results are persisted after each execution

## Tasks / Subtasks

- [x] Task 1: Create StepExecutor interface and registry (AC: #1)
  - [x] 1.1: Create `internal/template/steps/executor.go` with StepExecutor interface
  - [x] 1.2: Define `ExecutorRegistry` to map StepType → StepExecutor
  - [x] 1.3: Implement `NewExecutorRegistry()` constructor
  - [x] 1.4: Add `Get(stepType StepType) (StepExecutor, error)` method
  - [x] 1.5: Add `ErrExecutorNotFound` to `internal/errors/errors.go`

- [x] Task 2: Implement AIExecutor (AC: #2, #8)
  - [x] 2.1: Create `internal/template/steps/ai.go`
  - [x] 2.2: Define `AIExecutor` struct with `ai.Runner` dependency
  - [x] 2.3: Implement `Execute(ctx, task, step)` method
  - [x] 2.4: Build `AIRequest` from step config (prompt_template, permission_mode)
  - [x] 2.5: Call `runner.Run(ctx, req)` with derived timeout context
  - [x] 2.6: Map `AIResult` to `StepResult` (session_id, duration_ms, output)
  - [x] 2.7: Implement `Type() StepType` returning `domain.StepTypeAI`
  - [x] 2.8: Log execution start/end with zerolog

- [x] Task 3: Implement ValidationExecutor (AC: #3, #8)
  - [x] 3.1: Create `internal/template/steps/validation.go`
  - [x] 3.2: Define `ValidationExecutor` struct with command runner dependency
  - [x] 3.3: Implement `Execute(ctx, task, step)` method
  - [x] 3.4: Get validation commands from task.Config.ValidationCommands
  - [x] 3.5: Execute each command in order using exec.CommandContext
  - [x] 3.6: Capture stdout, stderr, exit code for each command
  - [x] 3.7: Return StepResult with aggregated output
  - [x] 3.8: Return early on first failure with detailed error
  - [x] 3.9: Implement `Type() StepType` returning `domain.StepTypeValidation`

- [x] Task 4: Implement GitExecutor (AC: #4, #8)
  - [x] 4.1: Create `internal/template/steps/git.go`
  - [x] 4.2: Define `GitExecutor` struct (placeholder for GitRunner dependency)
  - [x] 4.3: Implement `Execute(ctx, task, step)` method
  - [x] 4.4: Read `operation` from step.Config to determine action
  - [x] 4.5: Implement operation dispatch: commit, push, create_pr, smart_commit
  - [x] 4.6: Return StepResult with operation-specific output
  - [x] 4.7: Implement `Type() StepType` returning `domain.StepTypeGit`
  - [x] 4.8: For now, return placeholder results (full implementation in Epic 6)

- [x] Task 5: Implement HumanExecutor (AC: #5, #8)
  - [x] 5.1: Create `internal/template/steps/human.go`
  - [x] 5.2: Define `HumanExecutor` struct
  - [x] 5.3: Implement `Execute(ctx, task, step)` method
  - [x] 5.4: Read `prompt` from step.Config
  - [x] 5.5: Return StepResult with status "awaiting_approval" and prompt in output
  - [x] 5.6: Implement `Type() StepType` returning `domain.StepTypeHuman`

- [x] Task 6: Implement SDDExecutor (AC: #6, #8)
  - [x] 6.1: Create `internal/template/steps/sdd.go`
  - [x] 6.2: Define `SDDExecutor` struct with ai.Runner dependency
  - [x] 6.3: Implement `Execute(ctx, task, step)` method
  - [x] 6.4: Read `sdd_command` from step.Config (specify, plan, tasks, checklist)
  - [x] 6.5: Construct prompt for Speckit invocation via AI
  - [x] 6.6: Save generated artifact to task artifacts directory
  - [x] 6.7: Return StepResult with artifact_path set
  - [x] 6.8: Implement `Type() StepType` returning `domain.StepTypeSDD`

- [x] Task 7: Implement CIExecutor (AC: #7, #8)
  - [x] 7.1: Create `internal/template/steps/ci.go`
  - [x] 7.2: Define `CIExecutor` struct (placeholder for GitHubRunner dependency)
  - [x] 7.3: Implement `Execute(ctx, task, step)` method
  - [x] 7.4: Read `poll_interval` from step.Config
  - [x] 7.5: Implement polling loop with context cancellation check
  - [x] 7.6: Return success when workflows complete
  - [x] 7.7: Return failure with workflow details on timeout or failure
  - [x] 7.8: Implement `Type() StepType` returning `domain.StepTypeCI`
  - [x] 7.9: For now, return placeholder results (full implementation in Epic 6)

- [x] Task 8: Create default executor registry (AC: all)
  - [x] 8.1: Create `internal/template/steps/defaults.go`
  - [x] 8.2: Implement `NewDefaultRegistry(deps ExecutorDeps) *ExecutorRegistry`
  - [x] 8.3: Define `ExecutorDeps` struct for injecting ai.Runner and other deps
  - [x] 8.4: Register all executor implementations
  - [x] 8.5: Ensure all step types are covered

- [x] Task 9: Write comprehensive tests (AC: all)
  - [x] 9.1: Create `internal/template/steps/executor_test.go`
  - [x] 9.2: Test registry Get returns correct executor
  - [x] 9.3: Test registry Get returns ErrExecutorNotFound for unknown type
  - [x] 9.4: Create `internal/template/steps/ai_test.go`
  - [x] 9.5: Test AIExecutor passes correct request to runner
  - [x] 9.6: Test AIExecutor handles timeout via context
  - [x] 9.7: Test AIExecutor handles runner errors
  - [x] 9.8: Create `internal/template/steps/validation_test.go`
  - [x] 9.9: Test ValidationExecutor runs all commands
  - [x] 9.10: Test ValidationExecutor stops on first failure
  - [x] 9.11: Test ValidationExecutor captures output correctly
  - [x] 9.12: Create `internal/template/steps/git_test.go`
  - [x] 9.13: Test GitExecutor dispatches based on operation config
  - [x] 9.14: Create `internal/template/steps/human_test.go`
  - [x] 9.15: Test HumanExecutor returns awaiting_approval status
  - [x] 9.16: Test HumanExecutor includes prompt in output
  - [x] 9.17: Create `internal/template/steps/sdd_test.go`
  - [x] 9.18: Test SDDExecutor invokes correct SDD command
  - [x] 9.19: Create `internal/template/steps/ci_test.go`
  - [x] 9.20: Test CIExecutor polling behavior
  - [x] 9.21: Test CIExecutor timeout handling
  - [x] 9.22: Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Dev Notes

### Critical Warnings (READ FIRST)

1. **Domain types already exist**: `Step`, `StepResult`, `StepType` are defined in `internal/domain/`. Use those types, DO NOT redefine them.

2. **AIRunner exists as ai.Runner**: The interface is `ai.Runner` (not `AIRunner`). Import from `internal/ai`.

3. **Context as first parameter ALWAYS**: Every `Execute` method takes `ctx context.Context` as the first parameter. Check `ctx.Done()` for long operations.

4. **Package location is `internal/template/steps/`**: This is a NEW subpackage under `internal/template/`. The architecture doc specifies this location.

5. **Use dependency injection**: Pass `ai.Runner` and other dependencies into executor constructors, never create them internally.

6. **Error sentinel in internal/errors**: Add `ErrExecutorNotFound` to `internal/errors/errors.go` following existing patterns.

7. **Placeholder implementations for Epic 6 dependencies**: GitRunner and GitHubRunner don't exist yet. Implement GitExecutor and CIExecutor as stubs that return placeholder results. They'll be fully implemented in Epic 6.

### Package Locations

| File | Purpose |
|------|---------|
| `internal/template/steps/executor.go` | NEW - StepExecutor interface & ExecutorRegistry |
| `internal/template/steps/ai.go` | NEW - AIExecutor implementation |
| `internal/template/steps/validation.go` | NEW - ValidationExecutor implementation |
| `internal/template/steps/git.go` | NEW - GitExecutor implementation (placeholder) |
| `internal/template/steps/human.go` | NEW - HumanExecutor implementation |
| `internal/template/steps/sdd.go` | NEW - SDDExecutor implementation |
| `internal/template/steps/ci.go` | NEW - CIExecutor implementation (placeholder) |
| `internal/template/steps/defaults.go` | NEW - Default registry factory |
| `internal/template/steps/*_test.go` | NEW - Tests for each executor |
| `internal/domain/task.go` | REFERENCE - Step, StepResult types |
| `internal/domain/template.go` | REFERENCE - StepType constants |
| `internal/ai/runner.go` | REFERENCE - ai.Runner interface |
| `internal/errors/errors.go` | MODIFY - Add ErrExecutorNotFound |

### Import Rules (CRITICAL)

**`internal/template/steps/` MAY import:**
- `internal/constants` - for timeout constants
- `internal/domain` - for Task, Step, StepResult, StepType
- `internal/errors` - for ErrExecutorNotFound
- `internal/ai` - for ai.Runner interface
- `context`, `fmt`, `os/exec`, `sync`, `time`
- `github.com/rs/zerolog` - for structured logging

**MUST NOT import:**
- `internal/task` - avoid circular dependencies (task imports steps, not vice versa)
- `internal/workspace` - not needed here
- `internal/cli` - domain packages don't import CLI
- `internal/template` (parent package) - avoid circular import

### StepExecutor Interface Pattern

```go
// internal/template/steps/executor.go

package steps

import (
    "context"

    "github.com/mrz1836/atlas/internal/domain"
)

// StepExecutor defines the interface for executing a single step type.
// Implementations handle specific step types (AI, validation, git, etc.)
// and return structured results.
//
// Architecture doc refers to this as "StepExecutor interface" with
// implementations for each StepType.
type StepExecutor interface {
    // Execute runs the step and returns its result.
    // The context controls timeout and cancellation.
    // task provides the full task context for step execution.
    // step is the specific step being executed.
    Execute(ctx context.Context, task *domain.Task, step *domain.Step) (*domain.StepResult, error)

    // Type returns the StepType this executor handles.
    Type() domain.StepType
}

// ExecutorRegistry maps step types to their executors.
type ExecutorRegistry struct {
    executors map[domain.StepType]StepExecutor
}

// NewExecutorRegistry creates a new empty executor registry.
func NewExecutorRegistry() *ExecutorRegistry {
    return &ExecutorRegistry{
        executors: make(map[domain.StepType]StepExecutor),
    }
}

// Register adds an executor to the registry.
func (r *ExecutorRegistry) Register(e StepExecutor) {
    r.executors[e.Type()] = e
}

// Get retrieves the executor for a step type.
// Returns ErrExecutorNotFound if no executor is registered.
func (r *ExecutorRegistry) Get(stepType domain.StepType) (StepExecutor, error) {
    e, ok := r.executors[stepType]
    if !ok {
        return nil, fmt.Errorf("%w: %s", atlaserrors.ErrExecutorNotFound, stepType)
    }
    return e, nil
}
```

### AIExecutor Implementation Pattern

```go
// internal/template/steps/ai.go

package steps

import (
    "context"
    "time"

    "github.com/mrz1836/atlas/internal/ai"
    "github.com/mrz1836/atlas/internal/domain"
    "github.com/rs/zerolog"
)

// AIExecutor handles AI steps (analyze, implement).
type AIExecutor struct {
    runner ai.Runner
}

// NewAIExecutor creates a new AI executor with the given runner.
func NewAIExecutor(runner ai.Runner) *AIExecutor {
    return &AIExecutor{runner: runner}
}

// Execute runs an AI step using Claude Code.
func (e *AIExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.Step) (*domain.StepResult, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    log := zerolog.Ctx(ctx)
    log.Info().
        Str("task_id", task.ID).
        Str("step_name", step.Name).
        Msg("executing ai step")

    startTime := time.Now()

    // Build AI request from step config
    req := e.buildRequest(task, step)

    // Execute with timeout from step definition
    result, err := e.runner.Run(ctx, req)
    if err != nil {
        return nil, err
    }

    elapsed := time.Since(startTime)

    log.Info().
        Str("task_id", task.ID).
        Str("step_name", step.Name).
        Dur("duration_ms", elapsed).
        Msg("ai step completed")

    return &domain.StepResult{
        StepIndex:   task.CurrentStep,
        StepName:    step.Name,
        Status:      "success",
        StartedAt:   startTime,
        CompletedAt: time.Now(),
        DurationMs:  elapsed.Milliseconds(),
        Output:      result.Response,
        // SessionID can be stored in metadata if needed
    }, nil
}

// Type returns the step type this executor handles.
func (e *AIExecutor) Type() domain.StepType {
    return domain.StepTypeAI
}

func (e *AIExecutor) buildRequest(task *domain.Task, step *domain.Step) *domain.AIRequest {
    req := &domain.AIRequest{
        Prompt: task.Description, // Base prompt from task
        Model:  task.Config.Model,
    }

    // Apply step-specific config
    if step.Config != nil {
        if pm, ok := step.Config["permission_mode"].(string); ok {
            req.PermissionMode = pm
        }
        if pt, ok := step.Config["prompt_template"].(string); ok {
            // In production, would expand template
            req.Prompt = fmt.Sprintf("%s: %s", pt, task.Description)
        }
    }

    return req
}
```

### ValidationExecutor Implementation Pattern

```go
// internal/template/steps/validation.go

package steps

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
    "time"

    "github.com/mrz1836/atlas/internal/domain"
    atlaserrors "github.com/mrz1836/atlas/internal/errors"
    "github.com/rs/zerolog"
)

// ValidationExecutor handles validation steps.
type ValidationExecutor struct {
    workDir string // Working directory for commands
}

// NewValidationExecutor creates a new validation executor.
func NewValidationExecutor(workDir string) *ValidationExecutor {
    return &ValidationExecutor{workDir: workDir}
}

// Execute runs validation commands.
func (e *ValidationExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.Step) (*domain.StepResult, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    log := zerolog.Ctx(ctx)
    log.Info().
        Str("task_id", task.ID).
        Str("step_name", step.Name).
        Msg("executing validation step")

    startTime := time.Now()
    var output bytes.Buffer

    commands := task.Config.ValidationCommands
    if len(commands) == 0 {
        commands = []string{"magex format:fix", "magex lint", "magex test"}
    }

    for _, cmdStr := range commands {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }

        cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
        cmd.Dir = e.workDir

        var cmdOut, cmdErr bytes.Buffer
        cmd.Stdout = &cmdOut
        cmd.Stderr = &cmdErr

        log.Debug().Str("command", cmdStr).Msg("running validation command")

        if err := cmd.Run(); err != nil {
            output.WriteString(fmt.Sprintf("Command failed: %s\n%s\n%s\n",
                cmdStr, cmdOut.String(), cmdErr.String()))

            return &domain.StepResult{
                StepIndex:   task.CurrentStep,
                StepName:    step.Name,
                Status:      "failed",
                StartedAt:   startTime,
                CompletedAt: time.Now(),
                DurationMs:  time.Since(startTime).Milliseconds(),
                Output:      output.String(),
                Error:       fmt.Sprintf("validation command failed: %s", cmdStr),
            }, fmt.Errorf("%w: %s", atlaserrors.ErrValidationFailed, cmdStr)
        }

        output.WriteString(fmt.Sprintf("✓ %s\n", cmdStr))
    }

    return &domain.StepResult{
        StepIndex:   task.CurrentStep,
        StepName:    step.Name,
        Status:      "success",
        StartedAt:   startTime,
        CompletedAt: time.Now(),
        DurationMs:  time.Since(startTime).Milliseconds(),
        Output:      output.String(),
    }, nil
}

// Type returns the step type this executor handles.
func (e *ValidationExecutor) Type() domain.StepType {
    return domain.StepTypeValidation
}
```

### HumanExecutor Implementation Pattern

```go
// internal/template/steps/human.go

package steps

import (
    "context"
    "time"

    "github.com/mrz1836/atlas/internal/domain"
)

// HumanExecutor handles steps requiring human intervention.
type HumanExecutor struct{}

// NewHumanExecutor creates a new human executor.
func NewHumanExecutor() *HumanExecutor {
    return &HumanExecutor{}
}

// Execute signals that human review is required.
// Returns a special result indicating awaiting approval.
func (e *HumanExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.Step) (*domain.StepResult, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    now := time.Now()
    prompt := "Review required"
    if step.Config != nil {
        if p, ok := step.Config["prompt"].(string); ok {
            prompt = p
        }
    }

    return &domain.StepResult{
        StepIndex:   task.CurrentStep,
        StepName:    step.Name,
        Status:      "awaiting_approval",
        StartedAt:   now,
        CompletedAt: now,
        DurationMs:  0,
        Output:      prompt,
    }, nil
}

// Type returns the step type this executor handles.
func (e *HumanExecutor) Type() domain.StepType {
    return domain.StepTypeHuman
}
```

### Dependency Injection Pattern

```go
// internal/template/steps/defaults.go

package steps

import (
    "github.com/mrz1836/atlas/internal/ai"
)

// ExecutorDeps holds dependencies for creating executors.
type ExecutorDeps struct {
    AIRunner ai.Runner
    WorkDir  string
}

// NewDefaultRegistry creates a registry with all built-in executors.
func NewDefaultRegistry(deps ExecutorDeps) *ExecutorRegistry {
    r := NewExecutorRegistry()

    // Register all executors
    r.Register(NewAIExecutor(deps.AIRunner))
    r.Register(NewValidationExecutor(deps.WorkDir))
    r.Register(NewGitExecutor())       // Placeholder for Epic 6
    r.Register(NewHumanExecutor())
    r.Register(NewSDDExecutor(deps.AIRunner))
    r.Register(NewCIExecutor())        // Placeholder for Epic 6

    return r
}
```

### Previous Story Learnings (from Story 4-4)

From Story 4-4 (Template Registry and Definitions):

1. **Interface naming**: Use simple names to avoid stuttering. `StepExecutor` is good, not `steps.StepExecutorInterface`.
2. **Test helper patterns**: Use shared test helpers in `helpers_test.go` for common assertions.
3. **Thread safety**: If registry is accessed concurrently, use `sync.RWMutex`.
4. **Error sentinel placement**: Add `ErrExecutorNotFound` to `internal/errors/errors.go`.
5. **Run `magex test:race`**: Race detection is mandatory.
6. **Target 90%+ coverage**: All executors need comprehensive tests.

### Dependencies Between Stories

This story **depends on:**
- **Story 4-4** (Template Registry) - templates define step types
- **Story 4-3** (AIRunner Interface) - used by AIExecutor and SDDExecutor
- **Story 4-2** (Task State Machine) - task states affect execution
- **Story 4-1** (Task Data Model) - Step, StepResult types

This story **is required for:**
- **Story 4-6** (Task Engine Orchestrator) - engine uses executors
- **Story 4-7** (atlas start command) - start triggers execution
- **Story 4-8** (Utility Commands) - uses ValidationExecutor

### Edge Cases to Handle

1. **Context cancellation during execution** - All executors must check `ctx.Done()` and return `ctx.Err()` promptly
2. **Unknown step type** - Return `ErrExecutorNotFound` with step type in error message
3. **Nil step config** - Handle nil Config map gracefully (use defaults)
4. **Empty validation commands** - Use default commands if none configured
5. **AI runner returns error** - Wrap with `ErrClaudeInvocation` and include step context
6. **Command execution failure** - Capture both stdout and stderr in error output

### Performance Considerations

1. **Registry access is read-heavy** - Can use simple map without mutex if only populated at startup
2. **Validation commands run sequentially** - Consider parallel execution in future (Story 5-2)
3. **AI steps can be slow** - Always use context timeout from step definition
4. **CI polling has delays** - Use appropriate poll interval from config

### Testing Pattern

```go
func TestAIExecutor_Execute_Success(t *testing.T) {
    ctx := context.Background()

    mockRunner := &mockAIRunner{
        result: &domain.AIResult{
            Response:  "Implementation complete",
            SessionID: "test-session",
            Duration:  5000,
        },
    }

    executor := NewAIExecutor(mockRunner)

    task := &domain.Task{
        ID:          "task-123",
        CurrentStep: 0,
        Config:      domain.TaskConfig{Model: "sonnet"},
    }
    step := &domain.Step{Name: "implement", Type: domain.StepTypeAI}

    result, err := executor.Execute(ctx, task, step)

    require.NoError(t, err)
    assert.Equal(t, "success", result.Status)
    assert.Equal(t, "implement", result.StepName)
    assert.Contains(t, result.Output, "Implementation complete")
}

func TestAIExecutor_Execute_ContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // Cancel immediately

    executor := NewAIExecutor(&mockAIRunner{})
    task := &domain.Task{ID: "task-123"}
    step := &domain.Step{Name: "implement", Type: domain.StepTypeAI}

    _, err := executor.Execute(ctx, task, step)

    assert.ErrorIs(t, err, context.Canceled)
}

func TestValidationExecutor_Execute_FailsOnFirstError(t *testing.T) {
    ctx := context.Background()
    tmpDir := t.TempDir()

    executor := NewValidationExecutor(tmpDir)

    task := &domain.Task{
        ID:          "task-123",
        CurrentStep: 0,
        Config: domain.TaskConfig{
            ValidationCommands: []string{
                "echo 'ok'",
                "exit 1",  // This will fail
                "echo 'should not run'",
            },
        },
    }
    step := &domain.Step{Name: "validate", Type: domain.StepTypeValidation}

    result, err := executor.Execute(ctx, task, step)

    assert.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
    assert.Equal(t, "failed", result.Status)
    assert.NotContains(t, result.Output, "should not run")
}
```

### Project Structure Notes

- Step executors live in `internal/template/steps/` - a NEW subpackage
- Uses `internal/domain` for Task, Step, StepResult, StepType
- Uses `internal/ai` for ai.Runner interface
- GitRunner and GitHubRunner don't exist yet - use placeholders
- All executors follow the same interface pattern for uniformity

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.5]
- [Source: _bmad-output/planning-artifacts/architecture.md#StepExecutor Interface]
- [Source: _bmad-output/planning-artifacts/architecture.md#Task Engine Architecture]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/domain/task.go - Step, StepResult types]
- [Source: internal/domain/template.go - StepType constants]
- [Source: internal/ai/runner.go - ai.Runner interface]
- [Source: _bmad-output/implementation-artifacts/4-4-template-registry-and-definitions.md - Previous story patterns]
- [Source: _bmad-output/implementation-artifacts/4-3-airunner-interface-and-claudecoderunner.md - AIRunner implementation patterns]

### Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Manual verification:
# - Verify all 6 executor types are implemented
# - Verify registry maps all StepType values
# - Verify context cancellation works in all executors
# - Ensure 90%+ test coverage for new code
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

1. **StepDefinition vs Step**: The AC shows `step *Step` but implementation uses `*domain.StepDefinition` - this is correct per the actual domain types defined in Story 4-1. The story AC used simplified notation.
2. **Placeholder implementations**: GitExecutor and CIExecutor are intentionally placeholder implementations per Dev Notes - full implementation deferred to Epic 6 when GitRunner and GitHubRunner are available.
3. **Thread-safe registry**: Added sync.RWMutex to ExecutorRegistry for safe concurrent access.
4. **SessionID/NumTurns**: Added to StepResult struct during code review to properly capture AI step metadata per AC #2.

### Change Log

| Date | Change | Author |
|------|--------|--------|
| 2025-12-28 | Initial implementation of all 8 executor types and registry | Dev Agent |
| 2025-12-28 | Code review fixes: lint error, added SessionID/NumTurns to StepResult, updated tests | Code Review Agent |

### File List

**New Files:**
- `internal/template/steps/executor.go` - StepExecutor interface and ExecutorRegistry
- `internal/template/steps/ai.go` - AIExecutor implementation
- `internal/template/steps/validation.go` - ValidationExecutor with CommandRunner interface
- `internal/template/steps/git.go` - GitExecutor placeholder for Epic 6
- `internal/template/steps/human.go` - HumanExecutor for approval checkpoints
- `internal/template/steps/sdd.go` - SDDExecutor for Speckit integration
- `internal/template/steps/ci.go` - CIExecutor placeholder with polling logic
- `internal/template/steps/defaults.go` - NewDefaultRegistry and NewMinimalRegistry factories
- `internal/template/steps/executor_test.go` - Registry tests
- `internal/template/steps/ai_test.go` - AIExecutor tests with mock runner
- `internal/template/steps/validation_test.go` - ValidationExecutor tests with mock command runner
- `internal/template/steps/git_test.go` - GitExecutor operation dispatch tests
- `internal/template/steps/human_test.go` - HumanExecutor approval flow tests
- `internal/template/steps/sdd_test.go` - SDDExecutor artifact generation tests
- `internal/template/steps/ci_test.go` - CIExecutor polling and timeout tests
- `internal/template/steps/defaults_test.go` - Registry factory tests

**Modified Files:**
- `internal/errors/errors.go` - Added ErrExecutorNotFound sentinel error (line 183-184)
- `internal/domain/task.go` - Added SessionID and NumTurns fields to StepResult (lines 191-195)

