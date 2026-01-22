package ai

// This test suite uses MockExecutor to simulate Claude CLI subprocess execution.
// IMPORTANT: Tests NEVER make real API calls or use production API keys.
// All AI responses are pre-configured mock data to ensure test isolation and prevent
// accidental API usage. The EnsureNoRealAPIKeys() guard verifies no real API keys are present.

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test error types for execution testing.
var (
	errTestNetwork       = errors.New("network error")
	errTestExecNotFound  = errors.New("executable file not found")
	errTestRetryNetwork  = errors.New("network error")
	errTestExitStatus1   = errors.New("exit status 1")
	errTestExitStatus127 = errors.New("exit status 127")
)

// MockExecutor is a test implementation of CommandExecutor that simulates subprocess execution.
// It returns pre-configured responses without actually running the Claude CLI or making API calls.
// This ensures tests are fast, deterministic, and never incur API costs or require network access.
type MockExecutor struct {
	StdoutData []byte
	StderrData []byte
	Err        error
	// CapturedCmd stores the last executed command for verification.
	CapturedCmd *exec.Cmd
}

func (m *MockExecutor) Execute(_ context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	m.CapturedCmd = cmd
	return m.StdoutData, m.StderrData, m.Err
}

func TestNewClaudeCodeRunner(t *testing.T) {
	t.Run("creates runner with provided executor", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		mockExec := &MockExecutor{}

		runner := NewClaudeCodeRunner(cfg, mockExec)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.Equal(t, mockExec, runner.base.Executor)
	})

	t.Run("creates runner with default executor when nil provided", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		runner := NewClaudeCodeRunner(cfg, nil)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.IsType(t, &DefaultExecutor{}, runner.base.Executor)
	})
}

func TestClaudeCodeRunner_Run_ContextCancellation(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("returns error when context is canceled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "sonnet",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})

	t.Run("returns error when context deadline exceeded", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		// Allow the deadline to expire
		time.Sleep(time.Millisecond)

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "sonnet",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, result)
	})
}

func TestClaudeCodeRunner_Run_Success(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("successful execution with JSON parsing", func(t *testing.T) {
		// Override timeSleep to not actually sleep in tests
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(`{"type":"result","is_error":false,"result":"Task completed","session_id":"abc123","duration_ms":5000,"num_turns":3,"total_cost_usd":0.05}`),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "Fix the bug",
			Model:  "sonnet",
		}

		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "Task completed", result.Output)
		assert.Equal(t, "abc123", result.SessionID)
		assert.Equal(t, 5000, result.DurationMs)
		assert.Equal(t, 3, result.NumTurns)
		assert.InEpsilon(t, 0.05, result.TotalCostUSD, 0.0001)
	})

	t.Run("handles error response in JSON", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(`{"type":"result","is_error":true,"result":"An error occurred","session_id":"err123","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`),
			StderrData: []byte("Error details from stderr"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "Do something",
			Model:  "sonnet",
		}

		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "An error occurred", result.Output)
		assert.Equal(t, "Error details from stderr", result.Error)
	})
}

func TestClaudeCodeRunner_BuildCommand(t *testing.T) {
	t.Run("builds command with basic flags", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "claude", cmd.Path[len(cmd.Path)-6:]) // Ends with "claude"
		assert.Contains(t, cmd.Args, "-p")
		assert.Contains(t, cmd.Args, "--output-format")
		assert.Contains(t, cmd.Args, "json")
		assert.Contains(t, cmd.Args, "--model")
		assert.Contains(t, cmd.Args, "sonnet")
	})

	t.Run("uses stream-json format when activity streaming enabled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		// Create runner with activity callback to enable streaming
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{}, WithClaudeActivityCallback(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityMedium,
		}))

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "--output-format")
		assert.Contains(t, cmd.Args, "stream-json")
		// stream-json requires --verbose flag
		assert.Contains(t, cmd.Args, "--verbose")

		// Verify the exact position of output format value
		for i, arg := range cmd.Args {
			if arg == "--output-format" && i+1 < len(cmd.Args) {
				assert.Equal(t, "stream-json", cmd.Args[i+1], "output-format should be stream-json")
				break
			}
		}
	})

	t.Run("does not use verbose flag without activity streaming", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		// No activity callback - should use regular json format
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "--output-format")
		assert.Contains(t, cmd.Args, "json")
		assert.NotContains(t, cmd.Args, "--verbose")
		assert.NotContains(t, cmd.Args, "stream-json")
	})

	t.Run("builds command with permission mode", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:         "test",
			Model:          "sonnet",
			PermissionMode: "plan",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "--permission-mode")
		assert.Contains(t, cmd.Args, "plan")
	})

	t.Run("builds command with system prompt", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:       "test",
			Model:        "sonnet",
			SystemPrompt: "You are a helpful assistant",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "--append-system-prompt")
		assert.Contains(t, cmd.Args, "You are a helpful assistant")
	})

	t.Run("sets working directory", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:     "test",
			Model:      "sonnet",
			WorkingDir: "/tmp/workdir",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "/tmp/workdir", cmd.Dir)
	})

	t.Run("uses config model when request model is empty", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "opus",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "", // Empty - should fall back to config
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "--model")
		assert.Contains(t, cmd.Args, "opus")
	})
}

func TestClaudeCodeRunner_ErrorHandling(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("handles execution error with valid error JSON response", func(t *testing.T) {
		// This tests the tryParseErrorResponse path where command fails but returns valid error JSON
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Command fails but returns valid JSON with is_error: true
		mockExec := &MockExecutor{
			StdoutData: []byte(`{"type":"result","is_error":true,"result":"Rate limit exceeded","session_id":"err456","duration_ms":100,"num_turns":1,"total_cost_usd":0.001}`),
			StderrData: []byte("API rate limit hit"),
			Err:        errTestExitStatus1,
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		result, err := runner.Run(context.Background(), req)

		// Should succeed because we parsed the error JSON successfully
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "Rate limit exceeded", result.Output)
		assert.Contains(t, result.Error, "exit status 1")
	})

	t.Run("wraps execution error with ErrClaudeInvocation", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        errTestNetwork,
			StderrData: []byte("Connection refused"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		// After max retries, error should be wrapped
		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
	})

	t.Run("handles claude not found error", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err: errTestExecNotFound,
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("handles empty response", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(""),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("handles invalid JSON response", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte("not valid json"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "parse json")
	})

	t.Run("handles API key error in stderr", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        errTestExitStatus1,
			StderrData: []byte("Error: ANTHROPIC_API_KEY environment variable not set"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("handles command not found via stderr", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        errTestExitStatus127,
			StderrData: []byte("bash: claude: command not found"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestClaudeCodeRunner_RetryLogic(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("retries transient errors", func(t *testing.T) {
		// Override timeSleep to not actually sleep in tests
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		// Create a custom executor that fails twice then succeeds
		retryMockExec := &RetryMockExecutor{
			failuresBeforeSuccess: 2,
			successResponse:       []byte(`{"type":"result","is_error":false,"result":"Success after retries","session_id":"retry123","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`),
		}

		runner := NewClaudeCodeRunner(cfg, retryMockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "Success after retries", result.Output)
		assert.Equal(t, 3, retryMockExec.callCount) // 2 failures + 1 success
	})

	t.Run("does not retry non-retryable errors", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Non-retryable error (executable not found)
		mockExec := &MockExecutor{
			Err: errTestExecNotFound,
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		// Should have only tried once since error is not retryable
	})

	t.Run("respects context cancellation during retry", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Create a mock that always fails with a retryable error
		mockExec := &MockExecutor{
			Err:        errTestRetryNetwork,
			StderrData: []byte("connection refused"),
		}
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}
		runner := NewClaudeCodeRunner(cfg, mockExec)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		}

		_, err := runner.Run(ctx, req)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})
}

// RetryMockExecutor is a mock that fails a specified number of times before succeeding.
type RetryMockExecutor struct {
	failuresBeforeSuccess int
	callCount             int
	successResponse       []byte
}

// errRetryMockNetwork is a static error for RetryMockExecutor.
var errRetryMockNetwork = errors.New("network error")

func (m *RetryMockExecutor) Execute(_ context.Context, _ *exec.Cmd) ([]byte, []byte, error) {
	m.callCount++
	if m.callCount <= m.failuresBeforeSuccess {
		return nil, []byte("network error"), errRetryMockNetwork
	}
	return m.successResponse, nil, nil
}

// StreamingMockExecutor is a mock that simulates Claude Code's stream-json output.
type StreamingMockExecutor struct {
	StreamOutput []byte // Simulated stream-json output
	Stderr       []byte
	Err          error
}

func (m *StreamingMockExecutor) Execute(_ context.Context, _ *exec.Cmd) ([]byte, []byte, error) {
	if m.Err != nil {
		return nil, m.Stderr, m.Err
	}
	return m.StreamOutput, m.Stderr, nil
}

func TestClaudeCodeRunner_StreamingExecution(t *testing.T) {
	t.Run("parses json output without streaming", func(t *testing.T) {
		// When not using StreamingExecutor, Claude returns single JSON (not NDJSON)
		jsonOutput := []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Analysis complete.","session_id":"test-session","duration_ms":2500,"num_turns":2,"total_cost_usd":0.025}`)

		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		mockExec := &StreamingMockExecutor{
			StreamOutput: jsonOutput,
		}

		// Create runner with mock executor (not StreamingExecutor)
		runner := NewClaudeCodeRunner(cfg, mockExec)

		result, err := runner.Run(context.Background(), &domain.AIRequest{
			Prompt: "Analyze main.go",
			Model:  "sonnet",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Analysis complete.", result.Output)
		assert.Equal(t, "test-session", result.SessionID)
		assert.True(t, result.Success)
	})

	t.Run("handles error in json output", func(t *testing.T) {
		// Simulate an error result from Claude Code
		jsonOutput := []byte(`{"type":"result","subtype":"error","is_error":true,"result":"Rate limit exceeded","session_id":"error-session"}`)

		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		mockExec := &StreamingMockExecutor{
			StreamOutput: jsonOutput,
		}

		runner := NewClaudeCodeRunner(cfg, mockExec)

		result, err := runner.Run(context.Background(), &domain.AIRequest{
			Prompt: "test",
			Model:  "sonnet",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "Rate limit exceeded", result.Output)
	})
}

func TestClaudeCodeRunner_ParseResponse(t *testing.T) {
	t.Run("uses streaming executor result when available", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		// Create runner with streaming enabled
		runner := NewClaudeCodeRunner(cfg, nil, WithClaudeActivityCallback(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityMedium,
		}))

		// The streaming executor should be used
		streamExec, ok := runner.base.Executor.(*StreamingExecutor)
		require.True(t, ok, "Expected StreamingExecutor when activity callback is set")

		// Manually set a result on the streaming executor
		streamExec.lastClaudeResult = &ClaudeResponse{
			Type:      "result",
			Result:    "Streaming result",
			SessionID: "stream-session",
		}

		// parseResponse should return the streaming result
		resp, err := runner.parseResponse([]byte(`{"type":"result","result":"Fallback"}`))
		require.NoError(t, err)
		assert.Equal(t, "Streaming result", resp.Result)
		assert.Equal(t, "stream-session", resp.SessionID)
	})

	t.Run("falls back to stdout parsing when streaming result is nil", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		// Create runner with streaming enabled
		runner := NewClaudeCodeRunner(cfg, nil, WithClaudeActivityCallback(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityMedium,
		}))

		// The streaming executor's lastResult is nil by default

		// parseResponse should fall back to parsing stdout
		resp, err := runner.parseResponse([]byte(`{"type":"result","result":"Fallback result","session_id":"fallback-session"}`))
		require.NoError(t, err)
		assert.Equal(t, "Fallback result", resp.Result)
		assert.Equal(t, "fallback-session", resp.SessionID)
	})

	t.Run("parses stdout directly when streaming is disabled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		}

		// Create runner without streaming
		runner := NewClaudeCodeRunner(cfg, &MockExecutor{})

		// parseResponse should parse stdout directly
		resp, err := runner.parseResponse([]byte(`{"type":"result","result":"Direct result","session_id":"direct-session"}`))
		require.NoError(t, err)
		assert.Equal(t, "Direct result", resp.Result)
		assert.Equal(t, "direct-session", resp.SessionID)
	})
}

// Compile-time check that ClaudeCodeRunner implements Runner.
var _ Runner = (*ClaudeCodeRunner)(nil)

// Compile-time check that DefaultExecutor implements ProcessTerminator.
var _ ProcessTerminator = (*DefaultExecutor)(nil)

func TestDefaultExecutor_TerminateProcess_NoProcess(t *testing.T) {
	t.Parallel()

	executor := &DefaultExecutor{}

	// Terminate with no running process should return nil
	err := executor.TerminateProcess()
	assert.NoError(t, err)
}

func TestDefaultExecutor_TerminateProcess_RunningProcess(t *testing.T) {
	// Skip in short mode as this involves process management
	if testing.Short() {
		t.Skip("Skipping process termination test in short mode")
	}

	t.Parallel()

	executor := &DefaultExecutor{}

	// Start a long-running command in a goroutine
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sleep", "30")

	// Start execution in goroutine
	done := make(chan error, 1)
	go func() {
		_, _, err := executor.Execute(ctx, cmd)
		done <- err
	}()

	// Wait a bit for the process to start
	time.Sleep(100 * time.Millisecond)

	// Terminate the process
	err := executor.TerminateProcess()
	require.NoError(t, err)

	// Wait for execution to complete (should be quick after termination)
	select {
	case <-done:
		// Process was terminated successfully
	case <-time.After(5 * time.Second):
		t.Error("Process termination took too long")
	}
}

func TestDefaultExecutor_TerminateProcess_AlreadyExited(t *testing.T) {
	t.Parallel()

	executor := &DefaultExecutor{}

	// Execute a quick command
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "echo", "hello")

	_, _, err := executor.Execute(ctx, cmd)
	require.NoError(t, err)

	// Process already exited, terminate should return nil
	err = executor.TerminateProcess()
	assert.NoError(t, err)
}

func TestDefaultExecutor_TracksRunningProcess(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping process tracking test in short mode")
	}

	t.Parallel()

	executor := &DefaultExecutor{}

	// Before execution, no process should be tracked
	executor.processMu.Lock()
	assert.Nil(t, executor.runningProcess)
	executor.processMu.Unlock()

	// Start a command in a goroutine
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sleep", "1")

	done := make(chan error, 1)
	go func() {
		_, _, err := executor.Execute(ctx, cmd)
		done <- err
	}()

	// Wait for process to start
	time.Sleep(100 * time.Millisecond)

	// During execution, process should be tracked
	executor.processMu.Lock()
	assert.NotNil(t, executor.runningProcess)
	pid := executor.runningProcess.Pid
	executor.processMu.Unlock()
	assert.Positive(t, pid)

	// Wait for completion
	<-done

	// After completion, process should be cleared
	executor.processMu.Lock()
	assert.Nil(t, executor.runningProcess)
	executor.processMu.Unlock()
}
