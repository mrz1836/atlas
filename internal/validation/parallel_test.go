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

// MockToolChecker implements ToolChecker for testing.
type MockToolChecker struct {
	Installed bool
	Version   string
	Err       error
}

// IsGoPreCommitInstalled implements validation.ToolChecker.
func (m *MockToolChecker) IsGoPreCommitInstalled(_ context.Context) (bool, string, error) {
	return m.Installed, m.Version, m.Err
}

// Ensure MockToolChecker implements ToolChecker.
var _ validation.ToolChecker = (*MockToolChecker)(nil)

// MockStager implements Stager for testing.
type MockStager struct {
	Called  bool
	WorkDir string
	Err     error
}

// StageModifiedFiles implements validation.Stager.
func (m *MockStager) StageModifiedFiles(_ context.Context, workDir string) error {
	m.Called = true
	m.WorkDir = workDir
	return m.Err
}

// Ensure MockStager implements Stager.
var _ validation.Stager = (*MockStager)(nil)

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
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
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
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
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
		info   *validation.ProgressInfo
	}
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
		ProgressCallback: func(step, status string, info *validation.ProgressInfo) {
			mu.Lock()
			progressCalls = append(progressCalls, struct {
				step   string
				status string
				info   *validation.ProgressInfo
			}{step: step, status: status, info: info})
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

func TestRunner_Run_ProgressCallbackIncludesStepCounts(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var progressCalls []struct {
		step   string
		status string
		info   *validation.ProgressInfo
	}
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ProgressCallback: func(step, status string, info *validation.ProgressInfo) {
			mu.Lock()
			progressCalls = append(progressCalls, struct {
				step   string
				status string
				info   *validation.ProgressInfo
			}{step: step, status: status, info: info})
			mu.Unlock()
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	mu.Lock()
	defer mu.Unlock()

	// Verify step counts are correct
	// First call should be format starting with step 1/4
	assert.Equal(t, "format", progressCalls[0].step)
	assert.Equal(t, "starting", progressCalls[0].status)
	require.NotNil(t, progressCalls[0].info)
	assert.Equal(t, 1, progressCalls[0].info.CurrentStep)
	assert.Equal(t, 4, progressCalls[0].info.TotalSteps)

	// Find format completed call and verify it has duration
	for _, call := range progressCalls {
		if call.step == "format" && call.status == "completed" {
			require.NotNil(t, call.info)
			assert.Equal(t, 1, call.info.CurrentStep)
			assert.Equal(t, 4, call.info.TotalSteps)
			// Duration should be >= 0 (may be very small in tests)
			assert.GreaterOrEqual(t, call.info.DurationMs, int64(0))
			break
		}
	}

	// Find lint starting call and verify step number
	for _, call := range progressCalls {
		if call.step == "lint" && call.status == "starting" {
			require.NotNil(t, call.info)
			assert.Equal(t, 2, call.info.CurrentStep)
			assert.Equal(t, 4, call.info.TotalSteps)
			break
		}
	}

	// Find test starting call and verify step number
	for _, call := range progressCalls {
		if call.step == "test" && call.status == "starting" {
			require.NotNil(t, call.info)
			assert.Equal(t, 3, call.info.CurrentStep)
			assert.Equal(t, 4, call.info.TotalSteps)
			break
		}
	}

	// Find pre-commit starting call and verify step number
	for _, call := range progressCalls {
		if call.step == "pre-commit" && call.status == "starting" {
			require.NotNil(t, call.info)
			assert.Equal(t, 4, call.info.CurrentStep)
			assert.Equal(t, 4, call.info.TotalSteps)
			break
		}
	}
}

func TestRunner_Run_ProgressCallbackIncludesDuration(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	// Add small delay to ensure measurable duration
	mockRunner.SetResponseWithDelay("fmt", "formatted", "", 0, nil, 10*time.Millisecond)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var formatCompletedInfo *validation.ProgressInfo
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ProgressCallback: func(step, status string, info *validation.ProgressInfo) {
			mu.Lock()
			if step == "format" && status == "completed" {
				// Make a copy of the info
				if info != nil {
					copyInfo := *info
					formatCompletedInfo = &copyInfo
				}
			}
			mu.Unlock()
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	mu.Lock()
	defer mu.Unlock()

	// Format should have duration >= 10ms due to the delay
	require.NotNil(t, formatCompletedInfo)
	assert.GreaterOrEqual(t, formatCompletedInfo.DurationMs, int64(10))
}

func TestRunner_Run_ProgressCallbackOnFailure(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "", "error", 1, atlaserrors.ErrCommandFailed)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var failedStep, failedStatus string
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		ProgressCallback: func(step, status string, _ *validation.ProgressInfo) {
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
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
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
	runner.SetProgressCallback(func(_, _ string, _ *validation.ProgressInfo) {
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

func TestRunner_Run_PreCommitSkippedWhenNotInstalled(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
		ToolChecker: &MockToolChecker{
			Installed: false,
			Version:   "",
			Err:       nil,
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Empty(t, result.PreCommitResults, "pre-commit should not run when not installed")
	assert.Contains(t, result.SkippedSteps, "pre-commit")
	assert.Equal(t, "go-pre-commit not installed", result.SkipReasons["pre-commit"])
}

func TestRunner_Run_PreCommitRunsWhenInstalled(t *testing.T) {
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
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
			Err:       nil,
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Len(t, result.PreCommitResults, 1, "pre-commit should run when installed")
	assert.Empty(t, result.SkippedSteps)
}

func TestRunner_Run_PreCommitSkippedOnCheckError(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
		ToolChecker: &MockToolChecker{
			Installed: false,
			Version:   "",
			Err:       atlaserrors.ErrCommandFailed, // Error checking tool
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	// Should skip pre-commit when check fails (treated as not installed)
	assert.Contains(t, result.SkippedSteps, "pre-commit")
}

func TestRunner_Run_ProgressCallbackSkippedStatus(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)

	var preCommitStatus string
	var mu sync.Mutex

	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
		ToolChecker: &MockToolChecker{
			Installed: false,
		},
		ProgressCallback: func(step, status string, _ *validation.ProgressInfo) {
			mu.Lock()
			if step == "pre-commit" {
				preCommitStatus = status
			}
			mu.Unlock()
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	_, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "skipped", preCommitStatus, "pre-commit should report skipped status")
}

func TestPipelineResult_SkipFields(t *testing.T) {
	result := &validation.PipelineResult{
		Success:      true,
		SkippedSteps: []string{"pre-commit"},
		SkipReasons: map[string]string{
			"pre-commit": "go-pre-commit not installed",
		},
	}

	assert.Len(t, result.SkippedSteps, 1)
	assert.Equal(t, "pre-commit", result.SkippedSteps[0])
	assert.Equal(t, "go-pre-commit not installed", result.SkipReasons["pre-commit"])
}

func TestRunner_Run_CustomPreCommitCommandsOverrideDefault(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("custom-precommit-1", "ok", "", 0, nil)
	mockRunner.SetResponse("custom-precommit-2", "ok", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
		// Custom pre-commit commands (not using default go-pre-commit)
		PreCommitCommands: []string{"custom-precommit-1", "custom-precommit-2"},
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Len(t, result.PreCommitResults, 2, "both custom pre-commit commands should run")
	assert.Equal(t, "custom-precommit-1", result.PreCommitResults[0].Command)
	assert.Equal(t, "custom-precommit-2", result.PreCommitResults[1].Command)
}

func TestRunner_Run_MultipleCustomPreCommitCommandsExecuteInOrder(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("first-hook", "first", "", 0, nil)
	mockRunner.SetResponse("second-hook", "second", "", 0, nil)
	mockRunner.SetResponse("third-hook", "third", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"first-hook", "second-hook", "third-hook"},
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	require.Len(t, result.PreCommitResults, 3)

	// Verify order
	assert.Equal(t, "first-hook", result.PreCommitResults[0].Command)
	assert.Equal(t, "second-hook", result.PreCommitResults[1].Command)
	assert.Equal(t, "third-hook", result.PreCommitResults[2].Command)
}

func TestRunner_Run_StagerCalledAfterPreCommit(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	mockStager := &MockStager{}

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
		Stager: mockStager,
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/test/work/dir")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Verify stager was called with correct work directory
	assert.True(t, mockStager.Called, "Stager.StageModifiedFiles should be called after pre-commit")
	assert.Equal(t, "/test/work/dir", mockStager.WorkDir)
}

func TestRunner_Run_StagerNotCalledWhenPreCommitSkipped(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)

	mockStager := &MockStager{}

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands: []string{"fmt"},
		LintCommands:   []string{"lint"},
		TestCommands:   []string{"test"},
		ToolChecker: &MockToolChecker{
			Installed: false, // Not installed = skip pre-commit
		},
		Stager: mockStager,
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Stager should NOT be called when pre-commit is skipped
	assert.False(t, mockStager.Called, "Stager should not be called when pre-commit is skipped")
}

func TestRunner_Run_StagerErrorNonFatal(t *testing.T) {
	mockRunner := NewMockCommandRunner()
	mockRunner.SetResponse("fmt", "formatted", "", 0, nil)
	mockRunner.SetResponse("lint", "lint ok", "", 0, nil)
	mockRunner.SetResponse("test", "test ok", "", 0, nil)
	mockRunner.SetResponse("precommit", "precommit ok", "", 0, nil)

	mockStager := &MockStager{
		Err: atlaserrors.ErrCommandFailed, // Simulate staging error
	}

	executor := validation.NewExecutorWithRunner(time.Minute, mockRunner)
	config := &validation.RunnerConfig{
		FormatCommands:    []string{"fmt"},
		LintCommands:      []string{"lint"},
		TestCommands:      []string{"test"},
		PreCommitCommands: []string{"precommit"},
		ToolChecker: &MockToolChecker{
			Installed: true,
			Version:   "1.0.0",
		},
		Stager: mockStager,
	}
	runner := validation.NewRunner(executor, config)

	ctx := testContext()
	result, err := runner.Run(ctx, "/tmp")

	// Staging error should be non-fatal
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success, "validation should still succeed despite staging error")
	assert.True(t, mockStager.Called)
}

func TestDefaultToolChecker_IsGoPreCommitInstalled(t *testing.T) {
	t.Parallel()
	// This test verifies DefaultToolChecker delegates to config package
	// It's an integration test that may pass or fail depending on system state
	checker := &validation.DefaultToolChecker{}
	ctx := testContext()

	// Just verify it doesn't panic and returns valid types
	installed, version, err := checker.IsGoPreCommitInstalled(ctx)

	// We can't assert specific values since it depends on system state,
	// but we can verify the return types are valid
	_ = installed // bool
	_ = version   // string
	_ = err       // error or nil
}

func TestDefaultStager_StageModifiedFiles(t *testing.T) {
	t.Parallel()
	// This test verifies DefaultStager delegates to StageModifiedFiles
	// Using a temp directory without git will cause an error, which is fine
	stager := &validation.DefaultStager{}
	ctx := testContext()
	tmpDir := t.TempDir()

	// Should return an error since tmpDir is not a git repo
	err := stager.StageModifiedFiles(ctx, tmpDir)

	// We expect an error because tmpDir is not a git repo
	// This verifies the function is called and returns the expected error type
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check git status")
}
