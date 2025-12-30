# Story 5.7: Pre-commit Hook Integration

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a **user**,
I want **pre-commit hooks to run as part of validation**,
So that **all project quality checks pass before committing**.

## Acceptance Criteria

1. **Given** go-pre-commit is installed
   **When** validation reaches the pre-commit step
   **Then** the system runs `go-pre-commit run --all-files`

2. **Given** validation is running
   **When** the pipeline executes
   **Then** pre-commit runs after format, lint, and test (Phase 3, last)

3. **Given** pre-commit hook execution completes
   **When** pre-commit fails
   **Then** the failure is handled like other validation failures (task transitions to `validation_failed`)

4. **Given** pre-commit modifies files (auto-fix)
   **When** validation completes with modifications
   **Then** those changes are staged for the subsequent commit step

5. **Given** go-pre-commit is not installed
   **When** validation pipeline runs
   **Then** the pre-commit step is skipped with a warning message

6. **Given** the configuration file has custom pre-commit commands
   **When** validation runs
   **Then** custom pre-commit commands are used instead of the default

7. **Given** pre-commit runs
   **When** it completes (success or failure)
   **Then** pre-commit output is captured in validation results

## Tasks / Subtasks

- [x] Task 1: Implement go-pre-commit tool detection (AC: #5)
  - [x] 1.1: Add `IsGoPreCommitInstalled(ctx context.Context) (bool, string, error)` to `internal/config/tools.go`
  - [x] 1.2: Use `exec.LookPath("go-pre-commit")` to detect installation
  - [x] 1.3: If found, run `go-pre-commit --version` to get version string
  - [x] 1.4: Return (installed bool, version string, error)
  - [x] 1.5: Add unit tests with mocked command execution

- [x] Task 2: Implement pre-commit skip logic in validation runner (AC: #5)
  - [x] 2.1: Add `ToolChecker` interface to `RunnerConfig` struct for testability
  - [x] 2.2: Before Phase 3 in `Runner.Run()`, check if go-pre-commit is installed
  - [x] 2.3: If not installed, skip Phase 3 with warning
  - [x] 2.4: Log warning: "go-pre-commit not installed, skipping pre-commit validation"
  - [x] 2.5: Set result note/warning in PipelineResult indicating pre-commit was skipped
  - [x] 2.6: Add tests for skip behavior

- [x] Task 3: Implement auto-fix file staging (AC: #4)
  - [x] 3.1: Create `internal/validation/staging.go` with `StageModifiedFiles(ctx, workDir string) error`
  - [x] 3.2: Implement git status check: `git status --porcelain` to detect modified files
  - [x] 3.3: If modified files detected after pre-commit, run `git add` to stage them
  - [x] 3.4: Log staged files for transparency
  - [x] 3.5: Call `StageModifiedFiles` after successful pre-commit in `Runner.Run()`
  - [x] 3.6: Add tests for staging behavior (mock git commands)

- [x] Task 4: Add pre-commit skip warning to result handling (AC: #5, #7)
  - [x] 4.1: Add `SkippedSteps []string` field to `PipelineResult` struct in `result.go`
  - [x] 4.2: Add `SkipReasons map[string]string` field for skip reasons
  - [x] 4.3: When pre-commit is skipped, add "pre-commit" to SkippedSteps with reason
  - [x] 4.4: Update `ResultHandler` to log/display skipped steps with warnings
  - [x] 4.5: Update artifact saving to include skipped step info
  - [x] 4.6: Add tests for skip result handling

- [x] Task 5: Ensure custom pre-commit commands work (AC: #6)
  - [x] 5.1: Verify `RunnerConfig.PreCommitCommands` accepts custom commands from config
  - [x] 5.2: Verify `getPreCommitCommands()` in parallel.go returns custom commands when configured
  - [x] 5.3: Add integration test: custom pre-commit command execution
  - [x] 5.4: Add test: multiple custom pre-commit commands execute in order
  - [x] 5.5: Document in config: config already supports custom pre-commit commands

- [x] Task 6: Add TUI progress reporting for pre-commit (AC: #7)
  - [x] 6.1: Ensure `ProgressCallback` reports "pre-commit" step correctly (already implemented)
  - [x] 6.2: Add "skipped" status handling in CLI validate.go with Warning output
  - [x] 6.3: Add SkippedSteps and SkipReasons to ValidationResponse for JSON output
  - [x] 6.4: Add test for progress callback during pre-commit

- [x] Task 7: Write comprehensive tests (AC: all)
  - [x] 7.1: Test pre-commit runs after format/lint/test (order verification) - existing tests
  - [x] 7.2: Test pre-commit failure transitions task to validation_failed - TestRunner_Run_PreCommitFailure
  - [x] 7.3: Test pre-commit skip when not installed (with warning) - TestRunner_Run_PreCommitSkippedWhenNotInstalled
  - [x] 7.4: Test pre-commit auto-fix stages files - TestStageModifiedFiles_*
  - [x] 7.5: Test custom pre-commit commands override default - TestRunner_Run_CustomPreCommitCommandsOverrideDefault
  - [x] 7.6: Test pre-commit output captured in results - existing tests
  - [x] 7.7: Run all tests with `-race` flag - verified

## Dev Notes

### CRITICAL: Build on Existing Code

**DO NOT reinvent - EXTEND existing patterns:**

The validation pipeline is already fully implemented in Epic 5 stories 5.1-5.6:

- `internal/validation/parallel.go` - **Already has Phase 3 pre-commit execution**
- `internal/validation/executor.go` - Command execution with timeout
- `internal/validation/result.go` - PipelineResult struct
- `internal/validation/handler.go` - Result handling and artifacts
- `internal/constants/constants.go` - `DefaultPreCommitCommand = "go-pre-commit run --all-files"`

**The pre-commit execution is ALREADY IMPLEMENTED.** Your job is to ADD:
1. Tool detection (skip gracefully if go-pre-commit not installed)
2. Auto-fix file staging after pre-commit
3. Skip warnings in result handling

### Existing Implementation Analysis

From `internal/validation/parallel.go` (lines 111-121):
```go
// Phase 3: Pre-commit (sequential, last)
r.reportProgress("pre-commit", "starting")
preCommitResults, err := r.runSequential(ctx, r.getPreCommitCommands(), workDir)
result.PreCommitResults = preCommitResults
if err != nil {
    r.reportProgress("pre-commit", "failed")
    result.FailedStepName = "pre-commit"
    log.Error().Err(err).Msg("pre-commit step failed")
    return r.finalize(result, startTime), err
}
r.reportProgress("pre-commit", "completed")
```

This already:
- Runs pre-commit as Phase 3 (after format, lint, test)
- Captures results in `PreCommitResults`
- Sets `FailedStepName = "pre-commit"` on failure
- Reports progress via callback

**What's MISSING for this story:**
1. **Tool detection** - Skip gracefully if go-pre-commit isn't installed
2. **Auto-staging** - Stage files modified by pre-commit auto-fixes
3. **Skip warnings** - Track and display when pre-commit is skipped

### Architecture Compliance

**Package Boundaries:**
- `internal/validation` → can import: constants, errors, domain
- `internal/validation` → must NOT import: cli, tui, task, workspace, ai
- `internal/config/tools.go` → tool detection belongs here (existing pattern)

**Command Execution Pattern:**
```go
// Use existing CommandRunner interface from validation package
type CommandRunner interface {
    Run(ctx context.Context, workDir, command string) (stdout, stderr string, exitCode int, err error)
}
```

### Tool Detection Pattern

Follow existing tool detection in `internal/config/tools.go`:

```go
// IsGoPreCommitInstalled checks if go-pre-commit is available.
func IsGoPreCommitInstalled(ctx context.Context) (bool, string, error) {
    path, err := exec.LookPath("go-pre-commit")
    if err != nil {
        return false, "", nil // Not installed, not an error
    }

    // Get version
    cmd := exec.CommandContext(ctx, path, "--version")
    output, err := cmd.Output()
    if err != nil {
        return true, "unknown", nil // Installed but version check failed
    }

    return true, strings.TrimSpace(string(output)), nil
}
```

### Auto-Fix File Staging

Pre-commit hooks like gitleaks may not modify files, but formatters might. The staging logic:

```go
// internal/validation/staging.go
package validation

import (
    "context"
    "os/exec"
    "strings"

    "github.com/rs/zerolog"
)

// StageModifiedFiles stages any files modified during validation.
// This handles auto-fixes from pre-commit hooks.
func StageModifiedFiles(ctx context.Context, workDir string) error {
    log := zerolog.Ctx(ctx)

    // Check for modified files
    cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
    cmd.Dir = workDir
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("failed to check git status: %w", err)
    }

    // Parse modified files (lines starting with " M" are modified but unstaged)
    modified := parseModifiedFiles(string(output))
    if len(modified) == 0 {
        return nil // Nothing to stage
    }

    log.Info().
        Int("file_count", len(modified)).
        Strs("files", modified).
        Msg("staging files modified by pre-commit hooks")

    // Stage modified files
    args := append([]string{"add"}, modified...)
    stageCmd := exec.CommandContext(ctx, "git", args...)
    stageCmd.Dir = workDir
    if err := stageCmd.Run(); err != nil {
        return fmt.Errorf("failed to stage modified files: %w", err)
    }

    return nil
}

// parseModifiedFiles extracts unstaged modified files from git status output.
func parseModifiedFiles(statusOutput string) []string {
    var files []string
    for _, line := range strings.Split(statusOutput, "\n") {
        if len(line) < 3 {
            continue
        }
        // " M file.go" = modified but not staged
        // "M  file.go" = modified and staged (skip - already staged)
        if line[0] == ' ' && line[1] == 'M' {
            files = append(files, strings.TrimSpace(line[2:]))
        }
    }
    return files
}
```

### PipelineResult Enhancement

```go
// In internal/validation/result.go, add to PipelineResult:
type PipelineResult struct {
    // ... existing fields ...

    // SkippedSteps contains steps that were skipped (e.g., "pre-commit").
    SkippedSteps []string `json:"skipped_steps,omitempty"`

    // SkipReasons maps skipped step names to reasons.
    SkipReasons map[string]string `json:"skip_reasons,omitempty"`
}
```

### Integration Point in Runner.Run()

Modify Phase 3 in `internal/validation/parallel.go`:

```go
// Phase 3: Pre-commit (sequential, last)
// Check if go-pre-commit is installed
installed, version, checkErr := config.IsGoPreCommitInstalled(ctx)
if checkErr != nil {
    log.Warn().Err(checkErr).Msg("failed to check go-pre-commit installation")
}

if !installed {
    log.Warn().Msg("go-pre-commit not installed, skipping pre-commit validation")
    r.reportProgress("pre-commit", "skipped")
    if result.SkipReasons == nil {
        result.SkipReasons = make(map[string]string)
    }
    result.SkippedSteps = append(result.SkippedSteps, "pre-commit")
    result.SkipReasons["pre-commit"] = "go-pre-commit not installed"
} else {
    log.Info().Str("version", version).Msg("go-pre-commit detected")
    r.reportProgress("pre-commit", "starting")
    preCommitResults, err := r.runSequential(ctx, r.getPreCommitCommands(), workDir)
    result.PreCommitResults = preCommitResults
    if err != nil {
        r.reportProgress("pre-commit", "failed")
        result.FailedStepName = "pre-commit"
        log.Error().Err(err).Msg("pre-commit step failed")
        return r.finalize(result, startTime), err
    }
    r.reportProgress("pre-commit", "completed")

    // Stage any files modified by pre-commit hooks (auto-fixes)
    if err := StageModifiedFiles(ctx, workDir); err != nil {
        log.Warn().Err(err).Msg("failed to stage pre-commit modified files")
        // Non-fatal - continue with validation success
    }
}
```

### Test Patterns

```go
func TestRunner_PreCommitSkippedWhenNotInstalled(t *testing.T) {
    // Mock tool detection to return not installed
    executor := NewExecutorWithRunner(time.Minute, &mockRunner{})
    runner := NewRunner(executor, &RunnerConfig{
        FormatCommands: []string{"echo format"},
        LintCommands:   []string{"echo lint"},
        TestCommands:   []string{"echo test"},
        // PreCommit will use default, but tool check will skip it
    })

    result, err := runner.Run(context.Background(), "/tmp/test")

    require.NoError(t, err)
    assert.True(t, result.Success)
    assert.Contains(t, result.SkippedSteps, "pre-commit")
    assert.Equal(t, "go-pre-commit not installed", result.SkipReasons["pre-commit"])
}

func TestStageModifiedFiles_StagesUnstagedChanges(t *testing.T) {
    // Setup git repo with modified file
    tmpDir := t.TempDir()
    // ... setup git repo and modify a file ...

    err := StageModifiedFiles(context.Background(), tmpDir)

    require.NoError(t, err)
    // Verify file is now staged
    // ... check git status ...
}
```

### Project Structure Notes

**Files to Create:**
```
internal/validation/
├── staging.go           # StageModifiedFiles function
├── staging_test.go      # Tests for staging

internal/config/
└── tools.go            # Add IsGoPreCommitInstalled (may already exist for other tools)
```

**Files to Modify:**
- `internal/validation/parallel.go` - Add tool check and staging call in Phase 3
- `internal/validation/result.go` - Add SkippedSteps and SkipReasons fields
- `internal/validation/handler.go` - Handle skipped steps in result display

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Implementation Patterns & Consistency Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 5.7]
- [Source: _bmad-output/planning-artifacts/prd.md#FR33]
- [Source: internal/validation/parallel.go] - Existing Phase 3 pre-commit execution
- [Source: internal/constants/constants.go:78-79] - DefaultPreCommitCommand definition
- [Source: internal/config/config.go:122-133] - ValidationCommands.PreCommit configuration

### Previous Story Intelligence (5.6)

**Patterns Established in Story 5.6:**
1. **CLI command structure** - Follow `abandon.go` pattern for any new commands
2. **TUI display patterns** - Use `internal/tui/abandon.go` as reference for info display
3. **Engine integration** - Task engine already handles validation_failed state transitions
4. **Test coverage target** - 90%+ on critical paths with `-race` flag

**Files from 5.6:**
- `internal/cli/abandon.go` - CLI command pattern
- `internal/tui/abandon.go` - TUI display pattern
- `internal/task/engine.go` - Engine.Abandon() method location

### Git Intelligence Summary

**Recent commits show:**
- Story 5.6 completed: `feat(cli): add task abandonment flow with branch preservation`
- Story 5.5: `feat(cli): add manual fix and resume flow for validation failures`
- Story 5.4: `feat(validation): add AI-assisted retry for failed validation`

**Commit message pattern:**
```
feat(validation): add pre-commit hook integration with graceful skip

- Add go-pre-commit tool detection with version check
- Skip pre-commit gracefully when tool not installed (warning)
- Stage files modified by pre-commit auto-fixes
- Add skipped step tracking to PipelineResult
- Add comprehensive tests

Story 5.7 complete - pre-commit hook integration
```

### Validation Commands

Run before committing (ALL FOUR REQUIRED):
```bash
magex format:fix                # Format code
magex lint                      # Run linters (must pass)
magex test:race                 # Run tests with race detection (must pass)
go-pre-commit run --all-files   # CRITICAL: Runs gitleaks security scan!
```

**GITLEAKS WARNING:** Test values must not look like secrets. Avoid numeric suffixes like `_12345`.

### Context Propagation Requirements

```go
// ✅ ALWAYS: ctx as first parameter
func StageModifiedFiles(ctx context.Context, workDir string) error

// ✅ ALWAYS: Check cancellation at function entry
select {
case <-ctx.Done():
    return ctx.Err()
default:
}

// ✅ ALWAYS: Pass context to exec.Command
cmd := exec.CommandContext(ctx, "git", args...)
```

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

None required - implementation was clean.

### Completion Notes List

1. **Tool Detection**: Added `IsGoPreCommitInstalled()` and `IsGoPreCommitInstalledWithExecutor()` functions to `internal/config/tools.go` for checking go-pre-commit availability.

2. **Skip Logic**: Implemented `ToolChecker` interface and `DefaultToolChecker` in `parallel.go` for dependency injection. Refactored Phase 3 into `runPreCommitPhase()`, `handlePreCommitSkipped()`, and `executePreCommit()` methods to reduce nesting complexity.

3. **Auto-Fix Staging**: Created `internal/validation/staging.go` with `StageModifiedFiles()` and `GitRunner` interface for staging files modified by pre-commit hooks.

4. **Result Handling**: Added `SkippedSteps []string` and `SkipReasons map[string]string` fields to `PipelineResult` struct. Updated `ResultHandler.HandleResult()` to log skipped steps with warnings.

5. **TUI Integration**: Updated `internal/cli/validate.go` progress callback to handle "skipped" status. Added `SkippedSteps` and `SkipReasons` fields to `ValidationResponse` for JSON output.

6. **All tests pass with `-race` flag**: 100% of validation package tests pass, including new tests for tool detection, skip behavior, staging, and custom commands.

### File List

**Files Created:**
- `internal/validation/staging.go` - Auto-fix file staging implementation
- `internal/validation/staging_test.go` - Staging tests

**Files Modified:**
- `internal/config/tools.go` - Added IsGoPreCommitInstalled functions
- `internal/config/tools_test.go` - Added tool detection tests
- `internal/validation/parallel.go` - Added ToolChecker interface, Stager interface, and pre-commit phase handling
- `internal/validation/parallel_test.go` - Added MockToolChecker, MockStager, and skip/custom command/staging tests
- `internal/validation/result.go` - Added SkippedSteps and SkipReasons fields
- `internal/validation/handler.go` - Added skipped step logging
- `internal/validation/handler_test.go` - Added skipped step tests
- `internal/cli/validate.go` - Added skipped status handling and response fields
- `internal/cli/utility.go` - Added SkippedSteps and SkipReasons to ValidationResponse
- `internal/validation/staging.go` - Added untracked file handling to parseModifiedFiles

## Senior Developer Review (AI)

### Review Date
2025-12-29

### Reviewer
Claude Opus 4.5 (Adversarial Code Review)

### Review Outcome
**APPROVED** - All issues identified and fixed

### Issues Found and Resolved

| # | Severity | Issue | Resolution |
|---|----------|-------|------------|
| 1 | HIGH | Missing test verifying StageModifiedFiles is called during Runner.Run() | Added `Stager` interface for dependency injection with tests: `TestRunner_Run_StagerCalledAfterPreCommit`, `TestRunner_Run_StagerNotCalledWhenPreCommitSkipped`, `TestRunner_Run_StagerErrorNonFatal` |
| 2 | MEDIUM | parseModifiedFiles didn't handle untracked files (`??`) | Added handling for untracked files in `parseModifiedFiles()` with tests: `TestStageModifiedFiles_StagesUntrackedFiles`, `TestStageModifiedFiles_HandlesMixedStatusTypes` |
| 3 | MEDIUM | DefaultToolChecker had no direct tests | Added `TestDefaultToolChecker_IsGoPreCommitInstalled` and `TestDefaultStager_StageModifiedFiles` |
| 4 | MEDIUM | Inconsistent initialization of SkippedSteps vs SkipReasons | Added explicit initialization of both slice and map in `handlePreCommitSkipped()` |

### Acceptance Criteria Verification

| AC | Status | Evidence |
|----|--------|----------|
| AC1: go-pre-commit runs `go-pre-commit run --all-files` | ✅ PASS | `constants.DefaultPreCommitCommand` and `getPreCommitCommands()` in parallel.go |
| AC2: pre-commit runs after format/lint/test (Phase 3) | ✅ PASS | Runner.Run() structure in parallel.go:76-136 |
| AC3: pre-commit failure transitions to validation_failed | ✅ PASS | `TestRunner_Run_PreCommitFailure` test passes |
| AC4: modified files are staged for commit | ✅ PASS | `StageModifiedFiles()` in staging.go, now with Stager interface tests |
| AC5: skip with warning if go-pre-commit not installed | ✅ PASS | `TestRunner_Run_PreCommitSkippedWhenNotInstalled` test passes |
| AC6: custom pre-commit commands override default | ✅ PASS | `TestRunner_Run_CustomPreCommitCommandsOverrideDefault` test passes |
| AC7: output captured in validation results | ✅ PASS | `PipelineResult.PreCommitResults` populated in executePreCommit() |

### Validation Results
```
✅ magex format:fix - PASS
✅ magex lint - PASS (0 issues)
✅ magex test:race - PASS (all packages)
✅ go-pre-commit run --all-files - PASS (6 checks on 322 files)
```

### Files Changed in Review
- `internal/validation/parallel.go` - Added Stager interface, DefaultStager, getStager() method
- `internal/validation/parallel_test.go` - Added MockStager, 3 new staging tests, 2 default checker tests
- `internal/validation/staging.go` - Added untracked file handling in parseModifiedFiles()
- `internal/validation/staging_test.go` - Added 2 new tests for untracked file handling

