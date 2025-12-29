package validation_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/validation"
)

// safeBufferExec is a thread-safe bytes.Buffer for testing concurrent writes.
type safeBufferExec struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *safeBufferExec) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBufferExec) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

var _ io.Writer = (*safeBufferExec)(nil)

// MockCommandRunner implements CommandRunner for testing.
type MockCommandRunner struct {
	responses map[string]struct {
		stdout   string
		stderr   string
		exitCode int
		err      error
		delay    time.Duration
	}
}

// NewMockCommandRunner creates a new mock command runner.
func NewMockCommandRunner() *MockCommandRunner {
	return &MockCommandRunner{
		responses: make(map[string]struct {
			stdout   string
			stderr   string
			exitCode int
			err      error
			delay    time.Duration
		}),
	}
}

// SetResponse configures the response for a specific command.
func (m *MockCommandRunner) SetResponse(command, stdout, stderr string, exitCode int, err error) {
	m.responses[command] = struct {
		stdout   string
		stderr   string
		exitCode int
		err      error
		delay    time.Duration
	}{
		stdout:   stdout,
		stderr:   stderr,
		exitCode: exitCode,
		err:      err,
	}
}

// SetResponseWithDelay configures a response with an artificial delay.
func (m *MockCommandRunner) SetResponseWithDelay(command, stdout, stderr string, exitCode int, err error, delay time.Duration) {
	m.responses[command] = struct {
		stdout   string
		stderr   string
		exitCode int
		err      error
		delay    time.Duration
	}{
		stdout:   stdout,
		stderr:   stderr,
		exitCode: exitCode,
		err:      err,
		delay:    delay,
	}
}

// Run implements CommandRunner.Run.
func (m *MockCommandRunner) Run(ctx context.Context, _, command string) (stdout, stderr string, exitCode int, err error) {
	resp, ok := m.responses[command]
	if !ok {
		return "", "command not configured", 1, atlaserrors.ErrCommandNotConfigured
	}

	// Simulate delay if configured
	if resp.delay > 0 {
		select {
		case <-ctx.Done():
			return "", "context canceled", 1, ctx.Err()
		case <-time.After(resp.delay):
		}
	}

	return resp.stdout, resp.stderr, resp.exitCode, resp.err
}

// Ensure MockCommandRunner implements CommandRunner.
var _ validation.CommandRunner = (*MockCommandRunner)(nil)

func testContext() context.Context {
	logger := zerolog.Nop()
	return logger.WithContext(context.Background())
}

func TestNewExecutor_DefaultTimeout(t *testing.T) {
	executor := validation.NewExecutor(0)
	require.NotNil(t, executor)
}

func TestNewExecutor_CustomTimeout(t *testing.T) {
	executor := validation.NewExecutor(30 * time.Second)
	require.NotNil(t, executor)
}

func TestNewExecutorWithRunner_DefaultTimeout(t *testing.T) {
	runner := NewMockCommandRunner()
	executor := validation.NewExecutorWithRunner(0, runner)
	require.NotNil(t, executor)
}

func TestExecutor_Run_SuccessfulCommands(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("command1", "output1", "", 0, nil)
	runner.SetResponse("command2", "output2", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	results, err := executor.Run(ctx, []string{"command1", "command2"}, "/tmp")

	require.NoError(t, err)
	assert.Len(t, results, 2)

	assert.Equal(t, "command1", results[0].Command)
	assert.True(t, results[0].Success)
	assert.Equal(t, "output1", results[0].Stdout)

	assert.Equal(t, "command2", results[1].Command)
	assert.True(t, results[1].Success)
	assert.Equal(t, "output2", results[1].Stdout)
}

func TestExecutor_Run_StopsOnFailure(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("command1", "output1", "", 0, nil)
	runner.SetResponse("command2", "", "error output", 1, atlaserrors.ErrCommandFailed)
	runner.SetResponse("command3", "output3", "", 0, nil) // Should not run

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	results, err := executor.Run(ctx, []string{"command1", "command2", "command3"}, "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	assert.Len(t, results, 2) // Only first two commands ran

	assert.True(t, results[0].Success)
	assert.False(t, results[1].Success)
}

func TestExecutor_Run_EmptyCommands(t *testing.T) {
	runner := NewMockCommandRunner()
	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	results, err := executor.Run(ctx, []string{}, "/tmp")

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExecutor_Run_ContextCancellation(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("command1", "output1", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx, cancel := context.WithCancel(testContext())
	cancel() // Cancel immediately

	results, err := executor.Run(ctx, []string{"command1"}, "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, results)
}

func TestExecutor_RunSingle_Timeout(t *testing.T) {
	runner := NewMockCommandRunner()
	// Command takes 2 seconds but timeout is 100ms
	runner.SetResponseWithDelay("slow_command", "output", "", 0, nil, 2*time.Second)

	executor := validation.NewExecutorWithRunner(100*time.Millisecond, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "slow_command", "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrCommandTimeout)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, "command timed out", result.Error)
}

func TestExecutor_RunSingle_SuccessWithOutput(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("echo test", "test output\n", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "echo test", "/tmp")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "echo test", result.Command)
	assert.Equal(t, "test output\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Error)
	assert.False(t, result.StartedAt.IsZero())
	assert.False(t, result.CompletedAt.IsZero())
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestExecutor_RunSingle_FailureWithExitCode(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("failing_command", "", "error output", 1, atlaserrors.ErrCommandFailed)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "failing_command", "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "error output", result.Stderr)
	assert.NotEmpty(t, result.Error)
}

func TestExecutor_RunSingle_FailureWithNonZeroExitCode(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("failing_command", "", "error output", 42, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "failing_command", "/tmp")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValidationFailed)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, 42, result.ExitCode)
	assert.Equal(t, "exit code 42", result.Error)
}

func TestExecutor_RunSingle_ContextCancellationDuringCommand(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponseWithDelay("slow_command", "", "", 0, nil, 5*time.Second)

	executor := validation.NewExecutorWithRunner(10*time.Second, runner)
	ctx, cancel := context.WithCancel(testContext())

	// Cancel context after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := executor.RunSingle(ctx, "slow_command", "/tmp")

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestExecutor_RunSingle_CapturesBothStdoutAndStderr(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("mixed_output", "stdout content", "stderr content", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "mixed_output", "/tmp")

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, "stdout content", result.Stdout)
	assert.Equal(t, "stderr content", result.Stderr)
}

func TestExecutor_RunSingle_DurationTracking(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponseWithDelay("timed_command", "", "", 0, nil, 50*time.Millisecond)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	result, err := executor.RunSingle(ctx, "timed_command", "/tmp")

	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.DurationMs, int64(50))
	assert.True(t, result.CompletedAt.After(result.StartedAt))
}

func TestExecutor_Run_WorkingDirectoryPassedThrough(t *testing.T) {
	// This test verifies that workDir is passed correctly by using the real runner
	executor := validation.NewExecutor(time.Minute)
	ctx := testContext()

	tmpDir := t.TempDir()
	results, err := executor.Run(ctx, []string{"pwd"}, tmpDir)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Stdout, tmpDir)
}

func TestExecutor_Run_EnvironmentInherited(t *testing.T) {
	// Set an environment variable
	testEnvKey := "ATLAS_TEST_ENV_INHERITED"
	testEnvValue := "inherited_env_value"
	t.Setenv(testEnvKey, testEnvValue)

	executor := validation.NewExecutor(time.Minute)
	ctx := testContext()

	tmpDir := t.TempDir()
	results, err := executor.Run(ctx, []string{"echo $" + testEnvKey}, tmpDir)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Stdout, testEnvValue)
}

func TestExecutor_SetLiveOutput(t *testing.T) {
	executor := validation.NewExecutor(time.Minute)
	ctx := testContext()
	tmpDir := t.TempDir()

	liveOutput := &safeBufferExec{}
	executor.SetLiveOutput(liveOutput)

	results, err := executor.Run(ctx, []string{"echo live_test_output"}, tmpDir)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Success)

	// Live output should contain the output
	assert.Contains(t, liveOutput.String(), "live_test_output")
}

func TestExecutor_Run_SequentialExecutionOrder(t *testing.T) {
	runner := NewMockCommandRunner()
	runner.SetResponse("cmd1", "1", "", 0, nil)
	runner.SetResponse("cmd2", "2", "", 0, nil)
	runner.SetResponse("cmd3", "3", "", 0, nil)

	executor := validation.NewExecutorWithRunner(time.Minute, runner)
	ctx := testContext()

	results, err := executor.Run(ctx, []string{"cmd1", "cmd2", "cmd3"}, "/tmp")

	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify order
	assert.Equal(t, "cmd1", results[0].Command)
	assert.Equal(t, "cmd2", results[1].Command)
	assert.Equal(t, "cmd3", results[2].Command)

	// Verify sequential timing (each should complete after previous)
	for i := 1; i < len(results); i++ {
		assert.True(t, results[i].StartedAt.After(results[i-1].StartedAt) ||
			results[i].StartedAt.Equal(results[i-1].StartedAt))
	}
}
