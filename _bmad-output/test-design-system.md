# System-Level Test Design - ATLAS

**Date:** 2025-12-27
**Author:** MrZ (TEA Agent)
**Phase:** 3 (Solutioning - Pre-Implementation Readiness Gate)
**Status:** Draft

---

## Executive Summary

This document provides the system-level testability review for ATLAS (AI Task Lifecycle Automation System), a Go CLI tool orchestrating AI-assisted development workflows. This review evaluates the architecture for testability concerns before the implementation readiness gate check.

**Project Context:**
- **Type:** CLI Tool / Developer Tooling
- **Language:** Go 1.24+
- **Complexity:** Medium (9 major subsystems)
- **Field:** Greenfield (no existing tests)

**Key Findings:**
- Architecture demonstrates strong testability foundations (DI, interfaces, context propagation)
- 52 FRs + 33 NFRs fully mappable to test strategies
- Recommended test split: 70% Unit / 20% Integration / 10% E2E
- 3 testability concerns identified (mitigations provided)

---

## Testability Assessment

### Controllability: PASS

**Definition:** Can we control system state for testing?

| Aspect | Assessment | Evidence |
|--------|------------|----------|
| **Dependency Injection** | Excellent | All services use DI per Architecture (ARCH-13) |
| **Interface Abstractions** | Excellent | `AIRunner`, `GitRunner`, `GitHubRunner` interfaces defined |
| **External Tool Mocking** | Good | Subprocess pattern allows mock implementations |
| **State Seeding** | Good | JSON state files human-readable, manually editable (NFR14) |
| **Error Injection** | Good | Sentinel errors in `internal/errors` enable error path testing |

**Controllability Features:**
- `AIRunner` interface allows mocking Claude Code CLI responses
- `GitRunner` interface allows mocking git operations
- `GitHubRunner` interface allows mocking gh CLI operations
- State machine has explicit transition table for testing state flows
- Config layering allows test-specific overrides

### Observability: PASS

**Definition:** Can we inspect system state and validate behavior?

| Aspect | Assessment | Evidence |
|--------|------------|----------|
| **Structured Logging** | Excellent | Zerolog with JSON-lines format (NFR28-30) |
| **State Files** | Excellent | Human-readable JSON/YAML (NFR13) |
| **Step Progress** | Excellent | Task step tracking with artifacts |
| **Test Output** | Good | `--output json` flag for structured output (NFR46) |
| **Error Context** | Good | Wrapped errors with context at package boundaries |

**Observability Features:**
- Task logs stored per-workspace: `~/.atlas/workspaces/<ws>/tasks/<task-id>/task.log`
- Artifacts directory preserves step outputs (validation.1.json, etc.)
- State transitions logged with timestamps
- Standard fields: `ts`, `level`, `event`, `task_id`, `workspace_name`, `step_name`, `duration_ms`

### Reliability: PASS

**Definition:** Can tests be isolated, deterministic, and reproducible?

| Aspect | Assessment | Evidence |
|--------|------------|----------|
| **Test Isolation** | Excellent | Worktree isolation, unique task IDs |
| **Determinism** | Good | Context-based timeouts, explicit state machine |
| **Reproducibility** | Good | Atomic state saves, checkpoint recovery (NFR11-12) |
| **Parallel Safety** | Good | File locking (flock), per-workspace state |
| **Cleanup** | Excellent | Worktree destruction 100% reliable (NFR16) |

**Reliability Features:**
- Each task gets unique ID: `task-YYYYMMDD-HHMMSS`
- Atomic writes via write-then-rename pattern
- File locking prevents concurrent access corruption
- Worktree cleanup guaranteed even with corrupted state (NFR18)

---

## Architecturally Significant Requirements (ASRs)

ASRs are quality requirements that drive architecture decisions and pose testability challenges.

### ASR-1: Claude Code CLI Integration

**Requirement:** System must invoke Claude Code CLI and parse JSON output (FR21-23)

| Attribute | Value |
|-----------|-------|
| **Category** | TECH |
| **Probability** | 3 (High) |
| **Impact** | 3 (Critical) |
| **Risk Score** | 9 (CRITICAL) |
| **Testability Challenge** | Claude CLI may not behave as expected with `-p` flag |

**Mitigation:**
- AIRunner interface allows mock implementations for unit/integration tests
- Early spike (Week 1) to validate CLI behavior documented in PRD
- Fallback strategies defined: direct API, `--continue` patterns, alternative runners
- Integration tests use recorded responses (HAR-like capture for subprocess)

### ASR-2: Parallel Workspace Management

**Requirement:** System must manage 3+ parallel workspaces simultaneously (FR20)

| Attribute | Value |
|-----------|-------|
| **Category** | TECH |
| **Probability** | 2 (Medium) |
| **Impact** | 2 (Moderate) |
| **Risk Score** | 4 (MEDIUM) |
| **Testability Challenge** | Concurrent state access, worktree conflicts |

**Mitigation:**
- File locking (flock) for state file access
- Unique workspace/task IDs prevent collisions
- Integration tests verify parallel execution with `t.Parallel()`
- Worktree sibling naming with numeric suffixes handles conflicts

### ASR-3: State Persistence & Recovery

**Requirement:** Task state saved after each step; resumable after crash (NFR11-12)

| Attribute | Value |
|-----------|-------|
| **Category** | DATA |
| **Probability** | 2 (Medium) |
| **Impact** | 2 (Moderate) |
| **Risk Score** | 4 (MEDIUM) |
| **Testability Challenge** | Crash recovery scenarios difficult to simulate |

**Mitigation:**
- Atomic writes prevent partial state corruption
- Unit tests verify state serialization/deserialization
- Integration tests inject failures between steps
- Schema versioning enables migration testing

### ASR-4: CLI Responsiveness

**Requirement:** Local operations complete in <1 second (NFR1)

| Attribute | Value |
|-----------|-------|
| **Category** | PERF |
| **Probability** | 1 (Low) |
| **Impact** | 2 (Moderate) |
| **Risk Score** | 2 (LOW) |
| **Testability Challenge** | Performance benchmarks needed |

**Mitigation:**
- Go benchmarks (`go test -bench`) for critical paths
- CI pipeline includes performance regression checks
- Status command specifically measured in integration tests

### ASR-5: Secret Handling

**Requirement:** API keys never logged or displayed (NFR6, NFR9)

| Attribute | Value |
|-----------|-------|
| **Category** | SEC |
| **Probability** | 2 (Medium) |
| **Impact** | 3 (Critical) |
| **Risk Score** | 6 (HIGH) |
| **Testability Challenge** | Must verify absence of secrets in all outputs |

**Mitigation:**
- Unit tests verify config loading uses env var references (not values)
- Integration tests grep log output for known secret patterns
- Structured logging sanitization at zerolog level
- Static analysis for secret patterns in code

### ASR-6: External Tool Error Handling

**Requirement:** Must handle Claude CLI, gh CLI, git errors gracefully (NFR23, FR40)

| Attribute | Value |
|-----------|-------|
| **Category** | TECH |
| **Probability** | 2 (Medium) |
| **Impact** | 2 (Moderate) |
| **Risk Score** | 4 (MEDIUM) |
| **Testability Challenge** | Simulating external tool failures |

**Mitigation:**
- Runner interfaces allow error injection via mocks
- Sentinel errors enable `errors.Is()` checks in tests
- Retry logic with exponential backoff testable with mock delays
- Integration tests use failing subprocess mocks

---

## Test Levels Strategy

Based on the architecture (CLI tool with subprocess integrations), the following test distribution is recommended:

### Recommended Split: 70/20/10

| Level | Percentage | Rationale |
|-------|------------|-----------|
| **Unit** | 70% | Pure Go logic, state machine, config parsing, domain types |
| **Integration** | 20% | Runner interfaces, state persistence, CLI command execution |
| **E2E** | 10% | Critical user journeys (init, start, approve) |

### Unit Tests (70%)

**Target Packages:**
- `internal/constants` - Constant definitions
- `internal/errors` - Error wrapping utilities
- `internal/domain` - Type serialization/deserialization
- `internal/config` - Config parsing, validation, precedence
- `internal/task/state.go` - State machine transitions
- `internal/template/variables.go` - Template expansion
- `internal/workspace/naming.go` - Name generation

**Characteristics:**
- Fast execution (immediate feedback)
- No external dependencies
- 90%+ coverage target per existing standards

**Example Coverage:**
```go
// internal/task/state_test.go
func TestTaskStateTransition(t *testing.T) {
    tests := []struct {
        name        string
        from        TaskStatus
        to          TaskStatus
        wantErr     bool
    }{
        {"pending to running", Pending, Running, false},
        {"running to validating", Running, Validating, false},
        {"pending to completed", Pending, Completed, true}, // Invalid
    }
    // ...
}
```

### Integration Tests (20%)

**Target Packages:**
- `internal/task/store.go` - State file I/O with real filesystem
- `internal/workspace/worktree.go` - Git worktree operations
- `internal/ai/claude.go` - ClaudeCodeRunner (with mocked subprocess)
- `internal/git/runner.go` - GitRunner (with real temp git repos)
- `internal/validation/executor.go` - Command execution

**Characteristics:**
- Uses temp directories for filesystem tests
- Uses temporary git repositories
- Mocks subprocess execution for external tools
- Verifies component boundaries

**Example Coverage:**
```go
// internal/workspace/worktree_test.go (integration)
func TestWorktreeCreate(t *testing.T) {
    // Create temp git repo
    repoDir := t.TempDir()
    initGitRepo(t, repoDir)

    // Test worktree creation
    wt := NewWorktreeManager(repoDir)
    path, err := wt.Create(context.Background(), "test-branch")
    require.NoError(t, err)
    require.DirExists(t, path)
}
```

### E2E Tests (10%)

**Target Commands:**
- `atlas init` - Setup wizard completion
- `atlas start` - Task creation and initial execution
- `atlas status` - Status display
- `atlas approve` - Approval workflow

**Characteristics:**
- Tests compiled binary
- Uses isolated config/state directories
- Mocks external tools at subprocess level
- Validates user journeys end-to-end

**Example Coverage:**
```bash
# E2E test script
./atlas init --no-interactive
./atlas start "fix test" --template bugfix --workspace e2e-test
./atlas status --output json | jq '.workspaces[0].status'
./atlas workspace destroy e2e-test --force
```

### Test Level Decision Matrix for ATLAS

| Scenario | Unit | Integration | E2E |
|----------|------|-------------|-----|
| State machine transitions | Primary | - | - |
| Config precedence logic | Primary | Supplement | - |
| State file read/write | - | Primary | - |
| Git worktree operations | - | Primary | - |
| Claude CLI invocation | - | Primary (mocked) | Supplement |
| Full init wizard | - | - | Primary |
| Full task lifecycle | - | - | Primary |
| TUI rendering | - | Component | - |

---

## NFR Testing Approach

### Security Testing (NFR5-10)

**Approach:** Unit tests + Static analysis + Integration validation

| NFR | Test Strategy |
|-----|---------------|
| NFR5: API keys from env | Unit test config loading verifies env var references |
| NFR6: Keys never logged | Integration test greps log output for patterns |
| NFR7: Keys not committed | Pre-commit hook (go-pre-commit) + secret scanning |
| NFR8: GH auth via gh CLI | Integration test verifies gh CLI delegation |
| NFR9: No secrets in JSON logs | Unit test log sanitization |
| NFR10: No plaintext secrets in config | Config validation tests |

**Tools:**
- Unit tests (testify)
- Static analysis (gosec, staticcheck)
- Pre-commit hooks (go-pre-commit)

### Performance Testing (NFR1-4)

**Approach:** Go benchmarks + Integration timing assertions

| NFR | Test Strategy |
|-----|---------------|
| NFR1: Local ops <1s | Benchmark tests for status, workspace list |
| NFR2: Non-blocking AI ops | Integration test verifies UI responsiveness |
| NFR3: Network timeouts 30s | Unit test timeout configuration |
| NFR4: Progress indicators | TUI component tests |

**Tools:**
- Go benchmarks (`go test -bench`)
- CI pipeline timing checks

### Reliability Testing (NFR11-21)

**Approach:** Integration tests with failure injection

| NFR | Test Strategy |
|-----|---------------|
| NFR11: State saved per step | Integration test verifies checkpoint files |
| NFR12: Resume from checkpoint | Integration test kills process, resumes |
| NFR15: Atomic worktree creation | Integration test with concurrent access |
| NFR16: Reliable worktree destruction | Integration test with corrupted state |
| NFR18: Destroy always succeeds | Unit test handles all error scenarios |
| NFR19: Actionable error messages | Integration test verifies error output |
| NFR20: Timeouts on all external ops | Unit test timeout propagation |

**Tools:**
- Integration tests with temp filesystems
- Failure injection mocks
- Context cancellation tests

### Maintainability Testing (NFR28-33)

**Approach:** CI tools + Code quality gates

| NFR | Test Strategy |
|-----|---------------|
| NFR28: Structured JSON logs | Unit test log format |
| NFR29: Log levels | Config test for log level setting |
| NFR30: Per-workspace logs | Integration test log file paths |
| NFR33: --verbose flag | CLI flag parsing tests |

**Tools:**
- CI coverage reports (90%+ target)
- golangci-lint for code quality
- Code duplication detection

---

## Test Environment Requirements

### Local Development

```
Required:
- Go 1.24+
- Git (for worktree tests)
- testify (testing framework)

Optional:
- golangci-lint (static analysis)
- gosec (security scanning)
```

### CI Pipeline

```yaml
# Suggested CI stages
stages:
  - test:unit
  - test:integration
  - test:e2e
  - quality:lint
  - quality:security
  - quality:coverage

test:unit:
  script: go test -v -race -coverprofile=coverage.out ./...
  coverage: 90%

test:integration:
  script: go test -v -tags=integration ./...
  timeout: 10m

test:e2e:
  script: ./scripts/e2e-tests.sh
  timeout: 15m
```

### Test Data Strategy

| Data Type | Strategy |
|-----------|----------|
| State files | Generated via factories, auto-cleaned |
| Git repos | Temp directories with `t.TempDir()` |
| Config files | In-memory or temp files |
| AI responses | Recorded/mocked subprocess output |
| CLI output | Captured via subprocess exec |

---

## Testability Concerns

### Concern 1: Claude Code CLI Behavior (CRITICAL)

**Issue:** The `-p` flag behavior for custom slash commands is uncertain (documented risk in PRD).

**Impact:** Core AI integration may not work as expected, requiring alternative implementation.

**Recommendation:**
1. Week 1 spike to validate CLI behavior before deep implementation
2. Design AIRunner interface to support multiple implementations
3. Prepare fallback: direct API calls, `--continue` patterns
4. Create mock runner for testing that doesn't require real Claude CLI

**Status:** Documented risk with mitigation strategy

### Concern 2: Subprocess Testing Complexity

**Issue:** External tool integrations (claude, gh, git) require subprocess mocking.

**Impact:** Integration tests may be fragile or incomplete without proper mocking infrastructure.

**Recommendation:**
1. Create `internal/testutil/subprocess.go` with mock command execution
2. Use interface-based design (already present) for dependency injection
3. Record expected outputs for replay in tests
4. Consider `github.com/rogpeppe/go-internal/testscript` for CLI testing

**Status:** Addressed by architecture (interfaces defined)

### Concern 3: TUI Testing

**Issue:** Bubble Tea TUI components may be difficult to test in headless CI.

**Impact:** TUI bugs may slip through if not properly tested.

**Recommendation:**
1. Use Bubble Tea's `teatest` package for TUI testing
2. Separate TUI logic from business logic (already architected)
3. Test TUI components in isolation with simulated input
4. E2E tests capture final output strings, not visual rendering

**Status:** Moderate concern, mitigated by architecture separation

---

## Recommendations for Sprint 0

### Framework Setup Tasks

1. **Test Utilities Package**
   - Create `internal/testutil/` with fixtures, mocks, helpers
   - Implement subprocess mock for external tools
   - Create state file factories for test data generation

2. **CI Pipeline Configuration**
   - Unit tests with race detection and coverage
   - Integration tests with timeout
   - E2E test harness script
   - golangci-lint with project configuration

3. **Test Infrastructure**
   - Temp directory helpers for filesystem tests
   - Git repo initialization helpers
   - Mock implementations for all Runner interfaces

### Quality Gates

| Gate | Threshold |
|------|-----------|
| Unit test coverage | 90% |
| Integration tests passing | 100% |
| E2E critical paths | 100% |
| golangci-lint | 0 errors |
| gosec | 0 high/critical |

### First Stories Test Focus

| Story | Primary Test Type | Coverage Target |
|-------|-------------------|-----------------|
| 1.1 Project Structure | Build verification | - |
| 1.2 Constants Package | Unit | 100% |
| 1.3 Errors Package | Unit | 100% |
| 1.4 Domain Types | Unit | 100% |
| 1.5 Config Framework | Unit + Integration | 90% |
| 1.6 CLI Root Command | Integration + E2E | 80% |

---

## Validation Checklist

- [x] Architecture analyzed for testability
- [x] Controllability assessed (PASS)
- [x] Observability assessed (PASS)
- [x] Reliability assessed (PASS)
- [x] ASRs identified and risk-scored (6 ASRs)
- [x] Test levels strategy defined (70/20/10)
- [x] NFR testing approaches documented
- [x] Test environment requirements specified
- [x] Testability concerns flagged (3 concerns)
- [x] Sprint 0 recommendations provided

---

## Next Steps

1. **Review with team** - Validate ASR risk scores and mitigation strategies
2. **Proceed to implementation readiness gate** - This document supports the gate check
3. **Sprint 0 setup** - Implement test framework and CI pipeline
4. **Per-epic test planning** - Run `*test-design` in epic-level mode during Phase 4

---

**Generated by:** BMad TEA Agent - Test Architect Module
**Workflow:** `_bmad/bmm/testarch/test-design`
**Version:** 4.0 (BMad v6)
