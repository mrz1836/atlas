package ai

// This test suite uses MockExecutor to simulate Gemini CLI subprocess execution.
// IMPORTANT: Tests NEVER make real API calls or use production API keys.
// All AI responses are pre-configured mock data to ensure test isolation and prevent
// accidental API usage. The EnsureNoRealAPIKeys() guard verifies no real API keys are present.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test error types for Gemini execution testing.
var (
	errGeminiTestNetwork      = errors.New("network error")
	errGeminiTestExecNotFound = errors.New("executable file not found")
	errGeminiTestExitStatus1  = errors.New("exit status 1")
)

func TestNewGeminiRunner(t *testing.T) {
	t.Run("creates runner with provided executor", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		mockExec := &MockExecutor{}

		runner := NewGeminiRunner(cfg, mockExec)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.config)
		assert.Equal(t, mockExec, runner.executor)
	})

	t.Run("creates runner with default executor when nil provided", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		runner := NewGeminiRunner(cfg, nil)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.config)
		assert.IsType(t, &DefaultExecutor{}, runner.executor)
	})
}

func TestGeminiRunner_Run_ContextCancellation(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("returns error when context is canceled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "flash",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})

	t.Run("returns error when context deadline exceeded", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		// Allow the deadline to expire
		time.Sleep(time.Millisecond)

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "flash",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, result)
	})
}

func TestGeminiRunner_Run_Success(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("successful execution with JSON parsing", func(t *testing.T) {
		// Override timeSleep to not actually sleep in tests
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(`{"success":true,"content":"Task completed","session_id":"gem123","duration_ms":5000,"num_turns":3,"total_cost_usd":0.02}`),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "Fix the bug",
			Model:  "flash",
		}

		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "Task completed", result.Output)
		assert.Equal(t, "gem123", result.SessionID)
		assert.Equal(t, 5000, result.DurationMs)
		assert.Equal(t, 3, result.NumTurns)
		assert.InEpsilon(t, 0.02, result.TotalCostUSD, 0.0001)
	})

	t.Run("handles error response in JSON", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(`{"success":false,"content":"An error occurred","error":"Internal error","session_id":"err123","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`),
			StderrData: []byte("Error details from stderr"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "Do something",
			Model:  "flash",
		}

		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "An error occurred", result.Output)
		// Error field is set to the "error" field from JSON when present (not stderr)
		assert.Equal(t, "Internal error", result.Error)
	})
}

func TestGeminiRunner_BuildCommand(t *testing.T) {
	t.Run("builds command with basic flags", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test prompt here",
			Model:  "flash",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "gemini", cmd.Path[len(cmd.Path)-6:]) // Ends with "gemini"
		assert.Contains(t, cmd.Args, "--output-format")
		assert.Contains(t, cmd.Args, "json")
		assert.Contains(t, cmd.Args, "-m")
		// Model should be resolved to full name
		assert.Contains(t, cmd.Args, "gemini-3-flash-preview")
		// Prompt should be passed as positional argument (last arg)
		assert.Contains(t, cmd.Args, "test prompt here")
	})

	t.Run("resolves model alias to full name", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "pro",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "pro",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "gemini-3-pro-preview")
	})

	t.Run("sets working directory", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:     "test",
			Model:      "flash",
			WorkingDir: "/tmp/workdir",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "/tmp/workdir", cmd.Dir)
	})

	t.Run("uses config model when request model is empty", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "pro",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "", // Empty - should fall back to config
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "-m")
		assert.Contains(t, cmd.Args, "gemini-3-pro-preview")
	})
}

func TestGeminiRunner_ErrorHandling(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("handles execution error with valid error JSON response", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Command fails but returns valid JSON with success: false
		mockExec := &MockExecutor{
			StdoutData: []byte(`{"success":false,"content":"Rate limit exceeded","session_id":"err456","duration_ms":100,"num_turns":1,"total_cost_usd":0.001}`),
			StderrData: []byte("API rate limit hit"),
			Err:        errGeminiTestExitStatus1,
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		result, err := runner.Run(context.Background(), req)

		// Should succeed because we parsed the error JSON successfully
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "Rate limit exceeded", result.Output)
		assert.Contains(t, result.Error, "exit status 1")
	})

	t.Run("wraps execution error with ErrGeminiInvocation", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        errGeminiTestNetwork,
			StderrData: []byte("Connection refused"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		// After max retries, error should be wrapped
		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
	})

	t.Run("handles gemini not found error", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err: errGeminiTestExecNotFound,
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("handles empty response", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte(""),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("handles invalid JSON response", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			StdoutData: []byte("not valid json"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "parse json")
	})

	t.Run("handles API key error in stderr", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        errGeminiTestExitStatus1,
			StderrData: []byte("Error: GEMINI_API_KEY environment variable not set"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("handles command not found via stderr", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		mockExec := &MockExecutor{
			Err:        fmt.Errorf("exit status 127: %w", atlaserrors.ErrAgentNotInstalled),
			StderrData: []byte("bash: gemini: command not found"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGeminiRunner_RetryLogic(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("retries transient errors", func(t *testing.T) {
		// Override timeSleep to not actually sleep in tests
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		// Create a custom executor that fails twice then succeeds
		retryMockExec := &RetryMockExecutor{
			failuresBeforeSuccess: 2,
			successResponse:       []byte(`{"success":true,"content":"Success after retries","session_id":"retry123","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`),
		}

		runner := NewGeminiRunner(cfg, retryMockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
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
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Non-retryable error (executable not found)
		mockExec := &MockExecutor{
			Err: errGeminiTestExecNotFound,
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		// Should have only tried once since error is not retryable
	})

	t.Run("respects context cancellation during retry", func(t *testing.T) {
		originalSleep := timeSleep
		timeSleep = func(_ interface{ Nanoseconds() int64 }) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Create a mock that always fails with a retryable error
		mockExec := &MockExecutor{
			Err:        fmt.Errorf("connection refused: %w", atlaserrors.ErrMockNetwork),
			StderrData: []byte("connection refused"),
		}
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, mockExec)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &domain.AIRequest{
			Prompt: "test",
			Model:  "flash",
		}

		_, err := runner.Run(ctx, req)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestGeminiExecutor_Execute(t *testing.T) {
	t.Run("captures stdout and stderr", func(t *testing.T) {
		// This test verifies the executor captures output correctly
		// by testing with a simple echo command
		executor := &GeminiExecutor{}

		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "echo", "test output")
		stdout, stderr, err := executor.Execute(ctx, cmd)

		require.NoError(t, err)
		assert.Equal(t, "test output\n", string(stdout))
		assert.Empty(t, stderr)
	})
}

// Compile-time check that GeminiRunner implements Runner.
var _ Runner = (*GeminiRunner)(nil)
