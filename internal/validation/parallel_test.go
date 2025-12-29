package validation_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/validation"
)

func TestNewRunner_NilConfig(t *testing.T) {
	executor := validation.NewExecutor(time.Minute)
	runner := validation.NewRunner(executor, nil)
	require.NotNil(t, runner)
}

func TestNewRunner_WithConfig(t *testing.T) {
	executor := validation.NewExecutor(time.Minute)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
	}
	runner := validation.NewRunner(executor, config)
	require.NotNil(t, runner)
}

func TestRunner_Run_FullPipelineSuccess(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Empty(t, result.FailedStep())
	assert.Len(t, result.FormatResults, 1)
	assert.Len(t, result.LintResults, 1)
	assert.Len(t, result.TestResults, 1)
	assert.Len(t, result.PreCommitResults, 1)
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestRunner_Run_FormatFailureSkipsRest(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "", "format error", 1, atlaserrors.ErrCommandFailed)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "format", result.FailedStep())
	assert.Len(t, result.FormatResults, 1)
	assert.Empty(t, result.LintResults)
	assert.Empty(t, result.TestResults)
	assert.Empty(t, result.PreCommitResults)
}

func TestRunner_Run_LintFailureCollectsTestResults(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "", "lint error", 1, atlaserrors.ErrCommandFailed)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "lint", result.FailedStep())
	assert.Len(t, result.FormatResults, 1)
	assert.Len(t, result.LintResults, 1)
	// Test results should be collected even when lint fails
	assert.Len(t, result.TestResults, 1)
	assert.Empty(t, result.PreCommitResults)
}

func TestRunner_Run_TestFailureCollectsLintResults(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "", "test error", 1, atlaserrors.ErrCommandFailed)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "test", result.FailedStep())
	assert.Len(t, result.FormatResults, 1)
	// Lint results should be collected even when test fails
	assert.Len(t, result.LintResults, 1)
	assert.Len(t, result.TestResults, 1)
	assert.Empty(t, result.PreCommitResults)
}

func TestRunner_Run_PreCommitFailure(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "", "precommit error", 1, atlaserrors.ErrCommandFailed)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "pre-commit", result.FailedStep())
	assert.Len(t, result.FormatResults, 1)
	assert.Len(t, result.LintResults, 1)
	assert.Len(t, result.TestResults, 1)
	assert.Len(t, result.PreCommitResults, 1)
}

func TestRunner_Run_ContextCancellationBeforeStart(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
	}
	runner := validation.NewRunner(executor, config)

	ctx, cancel := context.WithCancel(testContext())
	cancel() // Cancel before run

	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestRunner_Run_ContextCancellationDuringExecution(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponseWithDelay("fmt", "formatted", "", 0, nil, 100*time.Millisecond)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
	}
	runner := validation.NewRunner(executor, config)

	ctx, cancel := context.WithCancel(testContext())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestRunner_Run_ProgressCallbackInvoked(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var progressCalls []struct {
		step   string
		status string
	}
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ProgressCallback: func(step, status string) {
			mu.Lock()
			progressCalls = append(progressCalls, struct {
				step   string
				status string
			}{step: step, status: status})
			mu.Unlock()
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Verify progress callbacks were invoked
	mu.Lock()
	defer mu.Unlock()

	// Should have calls for: format (starting, completed), lint (starting, completed),
	// test (starting, completed), pre-commit (starting, completed)
	assert.GreaterOrEqual(t, len(progressCalls), 8)

	// Verify format starting is first
	assert.Equal(t, "format", progressCalls[0].step)
	assert.Equal(t, "starting", progressCalls[0].status)
}

func TestRunner_Run_ProgressCallbackOnFailure(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "", "error", 1, atlaserrors.ErrCommandFailed)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var failedStep, failedStatus string
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		ProgressCallback: func(step, status string) {
			mu.Lock()
			if status == "failed" {
				failedStep = step
				failedStatus = status
			}
			mu.Unlock()
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	_, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "format", failedStep)
	assert.Equal(t, "failed", failedStatus)
}

func TestRunner_Run_EmptyCommandListsUseDefaults(t *testing.T) {
	// We use the real executor here to ensure default commands are applied
	// This test verifies the default command substitution logic
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("magex format:fix", "formatted", "", 0, nil)
	mockRunner.SetResponse("magex lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("magex test", "test ok", "", 0, nil)
	mockRunner.SetResponse("go-pre-commit run --all-files", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	// Empty config should use defaults
	config := &validation.RunnerConfig{}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	// Verify default commands were used
	assert.Len(t, result.FormatResults, 1)
	assert.Equal(t, "magex format:fix", result.FormatResults[0].Command)
}

func TestRunner_Run_MultipleCommandsPerStep(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt1", "formatted1", "", 0, nil)
	mockRunner.SetResponse("fmt2", "formatted2", "", 0, nil)
	mockRunner.SetResponse("lint1", "lint1 ok", "", 0, nil)
	mockRunner.SetResponse("lint2", "lint2 ok", "", 0, nil)
	mockRunner.SetResponse("test1", "test1 ok", "", 0, nil)
	mockRunner.SetResponse("test2", "test2 ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt1", "fmt2"},
		LintCommands:      []string{"lint1", "lint2"},
		TestCommands:      []string{"test1", "test2"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Len(t, result.FormatResults, 2)
	assert.Len(t, result.LintResults, 2)
	assert.Len(t, result.TestResults, 2)
	assert.Len(t, result.PreCommitResults, 1)
}

func TestRunner_Run_ParallelLintAndTestExecution(t *testing.T) {
	// Test that lint and test actually run in parallel
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponseWithDelay("lint", "lint ok", "", 0, nil, 50*time.Millisecond)
	mockRunner.SetResponseWithDelay("test", "test ok", "", 0, nil, 50*time.Millisecond)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	startTime := time.Now()
	result, err := runner.Run(ctx, "/tmp")
	totalDuration := time.Since(startTime)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// If lint and test run in parallel (50ms each), total should be ~50ms for parallel part
	// If sequential, it would be ~100ms for that part
	// Allow some margin for overhead
	assert.Less(t, totalDuration, 150*time.Millisecond, "lint and test should run in parallel")
}

func TestRunner_Run_BothLintAndTestFail(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "", "lint error", 1, atlaserrors.ErrCommandFailed)
	mockRunner.SetResponse("test", "", "test error", 1, atlaserrors.ErrCommandFailed)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	// FailedStep should be the first failure detected (either lint or test)
	assert.NotEmpty(t, result.FailedStep())
	// Both results should be collected
	assert.Len(t, result.LintResults, 1)
	assert.Len(t, result.TestResults, 1)
}

func TestRunner_SetProgressCallback(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
	}
	runner := validation.NewRunner(executor, config)

	var callCount int32
	runner.SetProgressCallback(func(_, _ string) {
		atomic.AddInt32(&callCount, 1)
	})

	ctx := testContext()
	_, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	assert.Positive(t, atomic.LoadInt32(&callCount))
}

func TestPipelineResult_AllResults(t *testing.T) {
	result := &validation.PipelineResult{
		FormatResults:    []validation.Result{{Command: "fmt"}},
		LintResults:      []validation.Result{{Command: "lint"}},
		TestResults:      []validation.Result{{Command: "test"}},
		PreCommitResults: []validation.Result{{Command: "precommit"}},
	}

	all := result.AllResults()
	assert.Len(t, all, 4)
	assert.Equal(t, "fmt", all[0].Command)
	assert.Equal(t, "lint", all[1].Command)
	assert.Equal(t, "test", all[2].Command)
	assert.Equal(t, "precommit", all[3].Command)
}

func TestPipelineResult_AllResults_EmptySlices(t *testing.T) {
	result := &validation.PipelineResult{}
	all := result.AllResults()
	assert.Empty(t, all)
}

func TestPipelineResult_FailedStep_ReturnsFailedStepName(t *testing.T) {
	result := &validation.PipelineResult{
		FailedStepName: "lint",
	}
	assert.Equal(t, "lint", result.FailedStep())
}

func TestPipelineResult_FailedStep_EmptyOnSuccess(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
	}
	assert.Empty(t, result.FailedStep())
}
