# Tech Debt: TaskEngine Test Coverage Expansion

Status: pending

## Overview

The TaskEngine has 82% test coverage. This task adds targeted tests to reach 90%+ and cover edge cases identified during the Epic 4 retrospective.

## Current Coverage Analysis

| Area | Status | Gap |
|------|--------|-----|
| `Start` happy path | ✅ Covered | - |
| `Resume` from error states | ✅ Covered | - |
| `ExecuteStep` context cancellation | ✅ Covered | - |
| `HandleStepResult` all branches | ✅ Covered | - |
| `executeStepInternal` directly | ⚠️ Partial | Only via `executeParallelGroup` |
| `shouldPause` edge cases | ⚠️ Partial | Missing error state variations |
| Timeout scenarios | ⚠️ Weak | Only basic timeout test |
| Concurrent modification | ⚠️ Weak | Basic concurrency test exists |
| `mapStepTypeToErrorStatus` exhaustive | ⚠️ Partial | Missing all step types |

## New Tests to Add

### Test 1: `TestEngine_ExecuteStepInternal_AllStepTypes`

```go
func TestEngine_ExecuteStepInternal_AllStepTypes(t *testing.T) {
    stepTypes := []domain.StepType{
        domain.StepTypeAI,
        domain.StepTypeValidation,
        domain.StepTypeGit,
        domain.StepTypeCI,
        domain.StepTypeHuman,
        domain.StepTypeSDD,
    }

    for _, stepType := range stepTypes {
        t.Run(string(stepType), func(t *testing.T) {
            // Setup registry with executor for this type
            // Call executeStepInternal directly
            // Verify result and logging
        })
    }
}
```

### Test 2: `TestEngine_ShouldPause_AllErrorStates`

```go
func TestEngine_ShouldPause_AllErrorStates(t *testing.T) {
    errorStates := []constants.TaskStatus{
        constants.TaskStatusValidationFailed,
        constants.TaskStatusGHFailed,
        constants.TaskStatusCIFailed,
        constants.TaskStatusCITimeout,
    }

    for _, status := range errorStates {
        t.Run(string(status), func(t *testing.T) {
            task := &domain.Task{Status: status}
            engine := NewEngine(...)

            assert.True(t, engine.shouldPause(task))
        })
    }
}
```

### Test 3: `TestEngine_ParallelExecution_RaceCondition`

```go
func TestEngine_ParallelExecution_RaceCondition(t *testing.T) {
    // Run with -race flag
    // Execute many parallel groups concurrently
    // Verify no data races on results slice

    const iterations = 100
    for i := 0; i < iterations; i++ {
        t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
            t.Parallel()
            // Execute parallel group
            // Verify all results collected correctly
        })
    }
}
```

### Test 4: `TestEngine_Timeout_StepExceedsLimit`

```go
func TestEngine_Timeout_StepExceedsLimit(t *testing.T) {
    // Create executor that takes longer than timeout
    executor := &mockExecutor{
        delay: 5 * time.Second,
    }

    // Create context with short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    // Execute step
    result, err := engine.ExecuteStep(ctx, task, step)

    // Verify timeout error
    assert.ErrorIs(t, err, context.DeadlineExceeded)
}
```

### Test 5: `TestEngine_MapStepTypeToErrorStatus_Exhaustive`

```go
func TestEngine_MapStepTypeToErrorStatus_Exhaustive(t *testing.T) {
    testCases := []struct {
        stepType       domain.StepType
        expectedStatus constants.TaskStatus
    }{
        {domain.StepTypeValidation, constants.TaskStatusValidationFailed},
        {domain.StepTypeGit, constants.TaskStatusGHFailed},
        {domain.StepTypeCI, constants.TaskStatusCIFailed},
        {domain.StepTypeAI, constants.TaskStatusValidationFailed},
        {domain.StepTypeHuman, constants.TaskStatusValidationFailed},
        {domain.StepTypeSDD, constants.TaskStatusValidationFailed},
    }

    for _, tc := range testCases {
        t.Run(string(tc.stepType), func(t *testing.T) {
            status := engine.mapStepTypeToErrorStatus(tc.stepType)
            assert.Equal(t, tc.expectedStatus, status)
        })
    }
}
```

### Test 6: `TestEngine_BuildRetryContext_EdgeCases`

```go
func TestEngine_BuildRetryContext_EdgeCases(t *testing.T) {
    t.Run("nil_result", func(t *testing.T) {
        task := &domain.Task{ID: "test"}
        context := engine.buildRetryContext(task, nil)
        assert.Contains(t, context, "test")
    })

    t.Run("empty_step_results", func(t *testing.T) {
        task := &domain.Task{
            ID:          "test",
            StepResults: []domain.StepResult{},
        }
        context := engine.buildRetryContext(task, &domain.StepResult{})
        assert.Contains(t, context, "Previous Attempts")
    })

    t.Run("many_failed_steps", func(t *testing.T) {
        // Create task with 10 failed steps
        // Verify all are included in context
    })
}
```

### Test 7: `TestEngine_ConcurrentResume`

```go
func TestEngine_ConcurrentResume(t *testing.T) {
    // Create multiple goroutines trying to resume same task
    // Verify only one succeeds or proper error handling

    var wg sync.WaitGroup
    errors := make([]error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            errors[idx] = engine.Resume(ctx, task, template)
        }(i)
    }

    wg.Wait()
    // Verify consistent behavior
}
```

## Acceptance Criteria

1. **Given** all new tests **When** running with `-race` **Then** no race conditions detected
2. **Given** `executeStepInternal` **When** called directly **Then** all step types work correctly
3. **Given** `shouldPause` **When** in any error state **Then** returns true
4. **Given** timeout scenario **When** step exceeds limit **Then** proper error returned
5. Test coverage reaches 90%+ on `internal/task/engine.go`
6. Run `magex format:fix && magex lint && magex test:race` - ALL PASS

## Validation Commands

```bash
# Check current coverage
go test -cover ./internal/task/...

# Detailed coverage report
go test -coverprofile=coverage.out ./internal/task/...
go tool cover -html=coverage.out -o coverage.html

# Verify with race detector
go test -race ./internal/task/... -count=1

# Full validation
magex format:fix && magex lint && magex test:race
```

## Priority

P1 - Important. Higher test coverage increases confidence for Epic 5.

## Estimated Effort

Small-Medium - 2-3 focused sessions

## Files to Modify

- `internal/task/engine_test.go` - Add new test functions

## Notes

- Focus on edge cases and error paths
- Each test should be independent and idempotent
- Use table-driven tests where appropriate
- Consider adding benchmark tests for parallel execution
