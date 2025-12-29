# Story 5.9: Quality Test Coverage Expansion

Status: backlog

## Story

As a **developer**,
I want **comprehensive quality tests covering Epics 1-5 functionality**,
So that **we have high confidence in the foundation before building advanced features**.

## Acceptance Criteria

1. **Given** Epics 1-5 are complete **When** running the full test suite **Then**:
   - All packages achieve 85%+ test coverage minimum
   - Critical paths (task engine, state machine, AI runner) achieve 90%+
   - Integration tests verify end-to-end workflows

2. **Given** the test suite exists **When** running with race detection **Then**:
   - `go test -race ./...` passes with zero race conditions
   - Concurrent access patterns are properly tested
   - Parallel step execution is stress-tested

3. **Given** error scenarios **When** testing failure paths **Then**:
   - All sentinel errors in `internal/errors` have corresponding tests
   - Error wrapping preserves context through call chain
   - Recovery paths are tested (retry, resume, abandon)

4. **Given** configuration edge cases **When** testing config loading **Then**:
   - Missing config files handled gracefully
   - Invalid config values produce clear errors
   - Config override precedence is verified

5. **Given** CLI commands **When** testing user interactions **Then**:
   - All flag combinations are tested
   - JSON output mode produces valid JSON
   - Exit codes match expectations (0 success, 1 error, 2 user input error)

6. **Given** workspace and task lifecycle **When** testing state transitions **Then**:
   - All valid transitions are tested
   - Invalid transitions are properly rejected
   - State persistence survives process restart

## Tasks / Subtasks

- [ ] Task 1: Audit current test coverage across all packages
  - [ ] 1.1: Generate coverage report for each package
  - [ ] 1.2: Identify packages below 85% coverage
  - [ ] 1.3: Prioritize critical path packages for improvement

- [ ] Task 2: Expand Epic 1-2 test coverage (Foundation & CLI)
  - [ ] 2.1: Add tests for config edge cases (invalid YAML, missing fields)
  - [ ] 2.2: Add tests for CLI flag combinations
  - [ ] 2.3: Add tests for tool detection edge cases
  - [ ] 2.4: Add tests for upgrade command scenarios

- [ ] Task 3: Expand Epic 3 test coverage (Workspace Management)
  - [ ] 3.1: Add integration tests for workspace lifecycle
  - [ ] 3.2: Add concurrent access tests for workspace store
  - [ ] 3.3: Add tests for worktree edge cases (permission errors, disk full)
  - [ ] 3.4: Add tests for workspace commands with various states

- [ ] Task 4: Expand Epic 4 test coverage (Task Engine)
  - [ ] 4.1: Add TaskEngine tests per tech-debt-taskengine-test-coverage.md
  - [ ] 4.2: Add tests for all step executor types
  - [ ] 4.3: Add tests for template parsing edge cases
  - [ ] 4.4: Add tests for AIRunner timeout and retry scenarios
  - [ ] 4.5: Add tests for SDD executor Speckit integration

- [ ] Task 5: Add integration tests for end-to-end workflows
  - [ ] 5.1: Test: init → start → validate → complete workflow
  - [ ] 5.2: Test: start → validation_failed → retry → success workflow
  - [ ] 5.3: Test: start → validation_failed → manual fix → resume workflow
  - [ ] 5.4: Test: start → abandon → workspace preserved workflow

- [ ] Task 6: Add stress tests for concurrent operations
  - [ ] 6.1: Concurrent workspace creation/destruction
  - [ ] 6.2: Concurrent task execution in parallel step groups
  - [ ] 6.3: Concurrent file store access with locking
  - [ ] 6.4: Run with `-race` flag, fix any detected issues

- [ ] Task 7: Validate and document test suite
  - [ ] 7.1: Generate final coverage report
  - [ ] 7.2: Document test patterns and conventions
  - [ ] 7.3: Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Dev Notes

### Coverage Targets by Package

| Package | Current Est. | Target | Priority |
|---------|--------------|--------|----------|
| internal/task | 82% | 90%+ | P0 - Critical |
| internal/workspace | 85% | 90% | P1 |
| internal/ai | 90% | 90% | Maintain |
| internal/template | 90% | 90% | Maintain |
| internal/cli | 80% | 85% | P2 |
| internal/config | 85% | 85% | Maintain |
| internal/errors | 100% | 100% | Maintain |
| internal/constants | N/A | N/A | No logic |

### Test Patterns to Follow

1. **Table-driven tests** for all cases with multiple inputs
2. **Mock interfaces** for external dependencies (Store, Runner, etc.)
3. **Context cancellation** tests for all long-running operations
4. **Parallel tests** where safe (use `t.Parallel()`)
5. **Cleanup functions** with `t.Cleanup()` for resources

### Related Tech Debt Tasks

- `tech-debt-taskengine-test-coverage.md` - Specific TaskEngine tests (can be merged into this story)
- `tech-debt-taskengine-refactor.md` - Refactoring that may help testability

### Integration Test Setup

```go
// Example integration test pattern
func TestIntegration_StartToComplete(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Setup: Create temp directory, init config
    tmpDir := t.TempDir()
    // ... full workflow test
}
```

### Validation Commands

```bash
# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Race detection
go test -race ./... -count=1

# Full validation
magex format:fix && magex lint && magex test:race
```

## Priority

P1 - Important. Solid test coverage increases confidence for Epics 6-8.

## Estimated Effort

Medium - 3-5 focused sessions
