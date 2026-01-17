# Story 4.9: Speckit SDD Integration

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **developer**,
I want **Speckit SDD workflows abstracted behind the feature template**,
So that **users get spec-driven development without learning Speckit commands**.

## Acceptance Criteria

1. **Given** the SDDExecutor exists **When** the feature template executes SDD steps **Then** the SDDExecutor invokes Speckit via Claude Code:
   - `/speckit.specify` → generates spec.md artifact
   - `/speckit.plan` → generates plan.md artifact
   - `/speckit.tasks` → generates tasks.md artifact
   - `/speckit.implement` → executes implementation
   - `/speckit.checklist` → generates checklist.md artifact

2. **Given** an SDD step executes **When** the AI runner returns output **Then**:
   - Output is captured as artifact in task directory
   - Artifact filename uses semantic naming: `spec.md`, `plan.md`, `tasks.md`, `checklist.md`
   - Artifact is versioned if multiple attempts (spec.1.md, spec.2.md, etc.)

3. **Given** the specify step completes **When** user interaction is required **Then**:
   - Spec is presented for human review after specify step (FR12)
   - Human step follows the specify step (already in template)
   - User can approve, request changes, or reject

4. **Given** Speckit is not installed **When** an SDD step is executed **Then**:
   - Clear error message is displayed with install instructions
   - Error includes: `Speckit not installed. Install with: uv tool install specify-cli --from git+https://github.com/github/spec-kit.git`
   - Task transitions to appropriate error state

5. **Given** a worktree exists **When** Speckit invocation occurs **Then**:
   - Speckit uses the worktree's working directory
   - Speckit looks for .speckit/ directory in worktree
   - If .speckit/ doesn't exist, Speckit creates it (or uses project defaults)

6. **Given** errors occur during Speckit execution **When** the AIRunner returns an error **Then**:
   - Errors are wrapped with appropriate sentinel (ErrClaudeInvocation)
   - Error context includes the SDD command that failed
   - Retry logic applies (existing in AIRunner)

7. **Given** `--max-turns` is configured **When** SDD steps execute **Then**:
   - MaxTurns configuration is passed to AI runner
   - Respects task-level and step-level timeout settings

8. **Given** context cancellation occurs **When** an SDD step is running **Then**:
   - Context cancellation is checked at function entry
   - Context is passed to AI runner
   - Cleanup occurs appropriately

## Tasks / Subtasks

- [x] Task 1: Enhance SDDExecutor to use proper Speckit slash commands (AC: #1, #6, #7, #8)
  - [x] 1.1: Update `buildPrompt()` to use `/speckit.specify`, `/speckit.plan`, `/speckit.tasks`, `/speckit.checklist` format
  - [x] 1.2: Add `/speckit.implement` support for the implement command
  - [x] 1.3: Ensure proper context propagation and cancellation handling
  - [x] 1.4: Verify MaxTurns and timeout settings are passed correctly

- [x] Task 2: Add Speckit installation detection (AC: #4)
  - [x] 2.1: Create `checkSpeckitInstalled()` function to detect if specify-cli is available
  - [x] 2.2: Add clear error message with install instructions when not found
  - [x] 2.3: Consider caching detection result for performance

- [x] Task 3: Improve artifact versioning (AC: #2)
  - [x] 3.1: Update `saveArtifact()` to use semantic naming: `spec.md`, `plan.md`, `tasks.md`, `checklist.md`
  - [x] 3.2: Implement artifact versioning for retry attempts (spec.1.md, spec.2.md)
  - [x] 3.3: Ensure proper file permissions (0600 for artifacts)

- [x] Task 4: Add worktree support (AC: #5)
  - [x] 4.1: Update SDDExecutor to accept working directory parameter
  - [x] 4.2: Pass worktree path to AI request's WorkingDir field
  - [x] 4.3: Verify .speckit/ directory handling in worktree context

- [x] Task 5: Enhance error handling and messaging (AC: #4, #6)
  - [x] 5.1: Add Speckit-specific error detection (command not found, constitution missing, etc.)
  - [x] 5.2: Wrap errors with ErrClaudeInvocation and include SDD command context
  - [x] 5.3: Provide actionable error messages for common failure scenarios

- [x] Task 6: Write comprehensive tests (AC: all)
  - [x] 6.1: Update existing `sdd_test.go` with new slash command format tests
  - [x] 6.2: Test Speckit installation detection
  - [x] 6.3: Test artifact versioning
  - [x] 6.4: Test worktree context passing
  - [x] 6.5: Test error handling for various failure scenarios
  - [x] 6.6: Run `magex format:fix && magex lint && magex test:race` - ALL PASS

- [x] Task 7: Update feature template if needed (AC: #1, #3)
  - [x] 7.1: Verify feature template steps align with new implementation
  - [x] 7.2: Ensure review_spec step follows specify step (already done)
  - [x] 7.3: Add any missing SDD step configuration

## Dev Notes

### Critical Warnings (READ FIRST)

1. **SDDExecutor already exists**: `internal/template/steps/sdd.go` has a working implementation. ENHANCE it, DO NOT recreate.

2. **AIRunner integration**: The SDDExecutor uses `ai.Runner` interface. The ClaudeCodeRunner already handles subprocess execution, retries, and JSON parsing.

3. **Feature template exists**: `internal/template/feature.go` already defines the SDD workflow with specify → review_spec → plan → tasks → implement → validate → checklist steps.

4. **Prompt format matters**: Use `/speckit.specify` format (with the dot), not `/speckit specify`. This is how Claude Code slash commands work.

5. **Context as first parameter ALWAYS**: Every method takes `ctx context.Context` as first parameter.

6. **Use existing patterns**: Follow patterns from previous story (4-8) and existing executor implementations.

### Speckit/Spec Kit Background

GitHub's Spec Kit (also called Speckit) is a Spec-Driven Development toolkit. Key facts:

- **Installation**: `uv tool install specify-cli --from git+https://github.com/github/spec-kit.git`
- **Slash commands**: `/speckit.specify`, `/speckit.plan`, `/speckit.tasks`, `/speckit.implement`, `/speckit.checklist`, `/speckit.clarify`
- **Constitution file**: `.speckit/constitution.md` defines project principles
- **Spec file**: `.speckit/spec.md` is the specification output
- **Current version**: 0.0.30+ (rapidly evolving)

### Package Locations

| File | Purpose |
|------|---------|
| `internal/template/steps/sdd.go` | MODIFY - Enhance SDDExecutor with proper Speckit integration |
| `internal/template/steps/sdd_test.go` | MODIFY - Update tests for new functionality |
| `internal/template/feature.go` | REFERENCE - Feature template with SDD steps |
| `internal/ai/claude.go` | REFERENCE - ClaudeCodeRunner implementation |
| `internal/ai/runner.go` | REFERENCE - AIRunner interface |
| `internal/domain/ai.go` | REFERENCE - AIRequest/AIResult types |
| `internal/constants/constants.go` | MAY MODIFY - Add Speckit-related constants if needed |

### Import Rules (CRITICAL)

**`internal/template/steps/sdd.go` MAY import:**
- `internal/ai` - for AIRunner interface
- `internal/constants` - for constants
- `internal/domain` - for Task, StepDefinition, StepResult, AIRequest
- `internal/errors` - for sentinel errors
- `github.com/rs/zerolog` - structured logging
- Standard library: context, fmt, os, path/filepath, time, os/exec

**MUST NOT import:**
- `internal/cli` - steps don't import CLI
- `internal/workspace` - steps don't import workspace directly
- `internal/tui` - steps don't import TUI

### Current SDDExecutor Analysis

The existing `internal/template/steps/sdd.go` implementation:

**Strengths:**
- Context cancellation handling ✅
- Logging with zerolog ✅
- Artifact saving to task directory ✅
- Step timeout support ✅
- Returns proper StepResult ✅

**Needs Enhancement:**
- Prompts should use `/speckit.specify` format instead of prose descriptions
- Missing Speckit installation check
- Artifact naming could be more semantic (spec.md vs sdd-specify-timestamp.md)
- Missing worktree path handling
- Missing implement command support

### Proper Slash Command Format

```go
// CURRENT (needs update):
func (e *SDDExecutor) buildPrompt(task *domain.Task, cmd SDDCommand) string {
    switch cmd {
    case SDDCmdSpecify:
        return fmt.Sprintf("Using Speckit SDD, generate a specification for: %s", task.Description)
    // ...
    }
}

// SHOULD BE:
func (e *SDDExecutor) buildPrompt(task *domain.Task, cmd SDDCommand) string {
    switch cmd {
    case SDDCmdSpecify:
        return fmt.Sprintf("/speckit.specify %s", task.Description)
    case SDDCmdPlan:
        return "/speckit.plan"  // Uses existing spec.md
    case SDDCmdTasks:
        return "/speckit.tasks" // Uses existing spec.md and plan.md
    case SDDCmdImplement:
        return "/speckit.implement" // Implements from tasks
    case SDDCmdChecklist:
        return "/speckit.checklist" // Generates review checklist
    default:
        return fmt.Sprintf("/speckit.%s", cmd)
    }
}
```

### Speckit Installation Detection

```go
// Check if Speckit is installed
func checkSpeckitInstalled() error {
    _, err := exec.LookPath("specify")
    if err != nil {
        return fmt.Errorf("Speckit not installed. Install with: uv tool install specify-cli --from git+https://github.com/github/spec-kit.git")
    }
    return nil
}
```

### Artifact Naming Strategy

Current: `sdd-specify-1735123456.md`
Better: `spec.md`, `plan.md`, `tasks.md`, `checklist.md`

For retries: `spec.1.md`, `spec.2.md` (versioned by attempt number)

### Previous Story Learnings (from Story 4-8)

From Story 4-8 (utility commands):

1. **Constants usage**: Use constants from `internal/constants` instead of magic strings
2. **Config loading pattern**: Load config with fallback to defaults
3. **Context check pattern**: Always check context at function entry and between operations
4. **Error wrapping**: Wrap at package boundaries only
5. **Testing**: Run `magex format:fix && magex lint && magex test:race` before marking done

### Git Intelligence (Recent Commits)

Recent commits show the following patterns:
- `02054de feat(cli): implement utility commands for standalone validation`
- `3d96571 feat(cli): implement atlas start command for task execution`
- `ac37697 feat(task): implement task engine orchestrator with step execution`
- `fd221de feat(steps): implement step executor framework with all executor types`
- `8fb8e06 feat(template): implement template registry and built-in definitions`

Key observations:
- All step executors follow the same interface pattern
- SDD executor was implemented in fd221de along with other executors
- Template registry uses Go code compiled into binary

### Edge Cases to Handle

1. **Speckit not installed** - Clear error with install instructions
2. **No .speckit/ directory** - Let Speckit create it or use defaults
3. **constitution.md missing** - Speckit handles this, but provide helpful error
4. **AI timeout during long specification** - Use step timeout configuration
5. **spec.md already exists** - Overwrite or version based on retry count
6. **Empty output from Speckit** - Treat as error, don't save empty artifact
7. **Worktree path with spaces** - Properly quote in WorkingDir

### Performance Considerations

1. **Single installation check**: Cache Speckit detection result at startup or first use
2. **Efficient artifact writes**: Use atomic write pattern (write temp, rename)
3. **Reasonable timeouts**: Specify step: 20min, Plan: 15min, Tasks: 15min, Checklist: 10min (already in template)

### Project Structure Notes

- SDDExecutor lives in `internal/template/steps/`
- Uses `ai.Runner` interface for Claude Code invocation
- Artifacts stored in task-specific directories
- Feature template already configured with SDD steps

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.9]
- [Source: _bmad-output/planning-artifacts/architecture.md#External Tool Integration]
- [Source: _bmad-output/planning-artifacts/prd.md#FR12, FR24]
- [Source: _bmad-output/project-context.md#Critical Implementation Rules]
- [Source: internal/template/steps/sdd.go - Current implementation]
- [Source: internal/template/feature.go - Feature template with SDD steps]
- [Source: internal/ai/claude.go - ClaudeCodeRunner implementation]
- [Source: _bmad-output/implementation-artifacts/4-8-implement-utility-commands.md - Previous story patterns]
- [External: GitHub Spec Kit](https://github.com/github/spec-kit)
- [External: Spec Kit Official Site](https://speckit.org/)
- [External: Martin Fowler - Understanding SDD Tools](https://martinfowler.com/articles/exploring-gen-ai/sdd-3-tools.html)

### Validation Commands

```bash
# REQUIRED before marking done:
magex format:fix      # Format code
magex lint            # Lint code (must pass)
magex test:race       # Run tests WITH race detection (CRITICAL)
go build ./...        # Verify compilation

# Manual verification:
# - Verify SDDExecutor uses /speckit.specify format
# - Verify Speckit installation detection works
# - Verify artifact versioning for retries
# - Verify worktree path is passed correctly
# - Ensure 90%+ test coverage for modified code
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

N/A

### Completion Notes List

1. Updated `buildPrompt()` to use proper Speckit slash command format (`/speckit.specify`, `/speckit.plan`, etc.)
2. Added Speckit installation detection with cached check using `exec.LookPath("specify")`
3. Implemented semantic artifact naming (`spec.md`, `plan.md`, `tasks.md`, `checklist.md`)
4. Added artifact versioning for retries (`spec.1.md`, `spec.2.md`, etc.)
5. Added worktree support via `NewSDDExecutorWithWorkingDir()` and `SetWorkingDir()` methods
6. Enhanced error handling with proper error wrapping and actionable messages
7. Updated feature template to use SDD type for implement step
8. Wrote comprehensive tests covering all acceptance criteria
9. All validation passes: `magex format:fix && magex lint && magex test:race`

### Code Review Fixes (2025-12-28)

1. Updated AC#2 to reflect actual implementation (semantic naming vs timestamp-based naming)
2. Improved `TestResetSpeckitCheck` with actual assertions instead of placeholder test
3. Added `TestGetArtifactFilename_ImplementReturnsEmpty` test
4. Added `TestGetArtifactFilename_AllCommands` comprehensive test
5. Added `TestSDDExecutor_saveArtifact_InvalidDirectory` error path test
6. Added `TestSDDExecutor_Execute_StepTimeout` test
7. Added `TestSDDExecutor_Execute_ContextCanceledDuringExecution` test
8. Test coverage improved from 89.7% to 91.4% (above 90% target)
9. All validation passes: `magex format:fix && magex lint && magex test:race`

### File List

- `internal/template/steps/sdd.go` - Enhanced SDDExecutor with Speckit integration
- `internal/template/steps/sdd_test.go` - Comprehensive tests for SDDExecutor
- `internal/template/feature.go` - Updated implement step to use SDD type
- `internal/template/feature_test.go` - Updated tests to reflect SDD implement step
