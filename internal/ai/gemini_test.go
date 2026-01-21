package ai

// This test suite uses MockExecutor to simulate Gemini CLI subprocess execution.
// IMPORTANT: Tests NEVER make real API calls or use production API keys.
// All AI responses are pre-configured mock data to ensure test isolation and prevent
// accidental API usage. The EnsureNoRealAPIKeys() guard verifies no real API keys are present.

import (
	"context"
	"encoding/json"
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
	"github.com/mrz1836/atlas/internal/testutil"
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
		assert.Equal(t, cfg, runner.base.Config)
		assert.Equal(t, mockExec, runner.base.Executor)
	})

	t.Run("creates runner with default executor when nil provided", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		runner := NewGeminiRunner(cfg, nil)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.IsType(t, &DefaultExecutor{}, runner.base.Executor)
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		assert.Contains(t, cmd.Args, "--yolo") // Auto-approve for non-interactive execution
		assert.Contains(t, cmd.Args, "-m")
		// Model should be resolved to full name
		assert.Contains(t, cmd.Args, "gemini-3-flash-preview")
		// Prompt should be passed as positional argument (last arg)
		assert.Contains(t, cmd.Args, "test prompt here")
	})

	t.Run("uses sandbox AND yolo mode when permission_mode is plan", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:         "verify the implementation",
			Model:          "flash",
			PermissionMode: "plan", // Read-only mode
		}

		cmd := runner.buildCommand(context.Background(), req)

		// Should use BOTH --sandbox (restrict actions) AND --yolo (auto-approve)
		assert.Contains(t, cmd.Args, "--sandbox")
		assert.Contains(t, cmd.Args, "--yolo")
	})

	t.Run("uses only yolo mode when permission_mode is empty", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt:         "implement the feature",
			Model:          "flash",
			PermissionMode: "", // Full access mode
		}

		cmd := runner.buildCommand(context.Background(), req)

		// Should use --yolo for full access mode, NOT --sandbox
		assert.Contains(t, cmd.Args, "--yolo")
		assert.NotContains(t, cmd.Args, "--sandbox")
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
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
		timeSleep = func(_ time.Duration) <-chan time.Time {
			ch := make(chan time.Time)
			close(ch)
			return ch
		}
		defer func() { timeSleep = originalSleep }()

		// Create a mock that always fails with a retryable error
		mockExec := &MockExecutor{
			Err:        fmt.Errorf("connection refused: %w", testutil.ErrMockNetwork),
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

func TestDefaultExecutor_Execute_Gemini(t *testing.T) {
	t.Run("captures stdout and stderr", func(t *testing.T) {
		// This test verifies the DefaultExecutor captures output correctly
		// by testing with a simple echo command
		executor := &DefaultExecutor{}

		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "echo", "test output")
		stdout, stderr, err := executor.Execute(ctx, cmd)

		require.NoError(t, err)
		assert.Equal(t, "test output\n", string(stdout))
		assert.Empty(t, stderr)
	})
}

func TestGeminiError_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("unmarshals string error", func(t *testing.T) {
		t.Parallel()
		data := []byte(`"simple error message"`)

		var ge GeminiError
		err := json.Unmarshal(data, &ge)

		require.NoError(t, err)
		assert.Equal(t, "simple error message", ge.RawString)
		assert.Empty(t, ge.Type)
		assert.Empty(t, ge.Message)
	})

	t.Run("unmarshals object error with type and message", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"type":"ApiError","message":"Rate limit exceeded"}`)

		var ge GeminiError
		err := json.Unmarshal(data, &ge)

		require.NoError(t, err)
		assert.Equal(t, "ApiError", ge.Type)
		assert.Equal(t, "Rate limit exceeded", ge.Message)
		assert.Equal(t, 0, ge.Code)
		assert.Empty(t, ge.RawString)
	})

	t.Run("unmarshals object error with code", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"type":"AuthError","message":"Invalid API key","code":401}`)

		var ge GeminiError
		err := json.Unmarshal(data, &ge)

		require.NoError(t, err)
		assert.Equal(t, "AuthError", ge.Type)
		assert.Equal(t, "Invalid API key", ge.Message)
		assert.Equal(t, 401, ge.Code)
		assert.Empty(t, ge.RawString)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{invalid json}`)

		var ge GeminiError
		err := json.Unmarshal(data, &ge)

		require.Error(t, err)
	})
}

func TestGeminiError_String(t *testing.T) {
	t.Parallel()

	t.Run("returns raw string when present", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{
			RawString: "simple error",
		}

		assert.Equal(t, "simple error", ge.String())
	})

	t.Run("returns type and message with code", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{
			Type:    "ApiError",
			Message: "Rate limit exceeded",
			Code:    429,
		}

		result := ge.String()
		assert.Equal(t, "ApiError (code 429): Rate limit exceeded", result)
	})

	t.Run("returns type and message without code", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{
			Type:    "AuthError",
			Message: "Invalid API key",
		}

		assert.Equal(t, "AuthError: Invalid API key", ge.String())
	})

	t.Run("returns only message when type is empty", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{
			Message: "Something went wrong",
		}

		assert.Equal(t, "Something went wrong", ge.String())
	})

	t.Run("returns only type when message is empty", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{
			Type: "UnknownError",
		}

		assert.Equal(t, "UnknownError", ge.String())
	})

	t.Run("returns unknown error when all fields empty", func(t *testing.T) {
		t.Parallel()
		ge := &GeminiError{}

		assert.Equal(t, "unknown error", ge.String())
	})
}

func TestParseGeminiResponse(t *testing.T) {
	t.Parallel()

	t.Run("parses valid success response", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"success":true,"response":"Task completed","session_id":"gem-123","duration_ms":2000,"num_turns":2,"total_cost_usd":0.03}`)

		resp, err := parseGeminiResponse(data)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "Task completed", resp.Response)
		assert.Equal(t, "gem-123", resp.SessionID)
		assert.Equal(t, 2000, resp.DurationMs)
		assert.Equal(t, 2, resp.NumTurns)
		assert.InDelta(t, 0.03, resp.TotalCostUSD, 0.001)
	})

	t.Run("parses error response with structured error", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"success":false,"error":{"type":"ApiError","message":"Rate limit","code":429}}`)

		resp, err := parseGeminiResponse(data)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Equal(t, "ApiError", resp.Error.Type)
		assert.Equal(t, "Rate limit", resp.Error.Message)
		assert.Equal(t, 429, resp.Error.Code)
	})

	t.Run("parses error response with string error", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"success":false,"error":"Simple error message"}`)

		resp, err := parseGeminiResponse(data)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		require.NotNil(t, resp.Error)
		assert.Equal(t, "Simple error message", resp.Error.RawString)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		t.Parallel()
		resp, err := parseGeminiResponse([]byte{})

		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		resp, err := parseGeminiResponse([]byte("not valid json"))

		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "parse json")
	})
}

func TestGeminiResponse_toAIResult(t *testing.T) {
	t.Parallel()

	t.Run("converts success response with response field", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success:      true,
			Response:     "response content",
			SessionID:    "gem-456",
			DurationMs:   3000,
			NumTurns:     3,
			TotalCostUSD: 0.04,
		}

		result := resp.toAIResult("")

		assert.True(t, result.Success)
		assert.Equal(t, "response content", result.Output)
		assert.Equal(t, "gem-456", result.SessionID)
		assert.Equal(t, 3000, result.DurationMs)
		assert.Equal(t, 3, result.NumTurns)
		assert.InDelta(t, 0.04, result.TotalCostUSD, 0.001)
		assert.Empty(t, result.Error)
	})

	t.Run("uses content field when response is empty", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success: true,
			Content: "content field data",
		}

		result := resp.toAIResult("")

		assert.Equal(t, "content field data", result.Output)
	})

	t.Run("uses result field when response and content are empty", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success: true,
			Result:  "result field data",
		}

		result := resp.toAIResult("")

		assert.Equal(t, "result field data", result.Output)
	})

	t.Run("prefers response over content over result", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success:  true,
			Response: "response",
			Content:  "content",
			Result:   "result",
		}

		result := resp.toAIResult("")

		assert.Equal(t, "response", result.Output)
	})

	t.Run("includes error from error object", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success: false,
			Error: &GeminiError{
				Type:    "ApiError",
				Message: "Rate limit exceeded",
				Code:    429,
			},
		}

		result := resp.toAIResult("")

		assert.False(t, result.Success)
		assert.Equal(t, "ApiError (code 429): Rate limit exceeded", result.Error)
	})

	t.Run("uses stderr when error object is nil", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success: false,
		}

		result := resp.toAIResult("stderr error content")

		assert.False(t, result.Success)
		assert.Equal(t, "stderr error content", result.Error)
	})

	t.Run("empty error when success is false but no error info", func(t *testing.T) {
		t.Parallel()
		resp := &GeminiResponse{
			Success: false,
		}

		result := resp.toAIResult("")

		assert.False(t, result.Success)
		assert.Empty(t, result.Error)
	})
}

func TestGeminiRunner_Streaming(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("uses StreamingExecutor with Gemini provider when activity callback set", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		var receivedEvents []ActivityEvent
		runner := NewGeminiRunner(cfg, nil, WithGeminiActivityCallback(ActivityOptions{
			Callback: func(event ActivityEvent) {
				receivedEvents = append(receivedEvents, event)
			},
			Verbosity: VerbosityHigh,
		}))

		// Verify StreamingExecutor is used
		_, ok := runner.base.Executor.(*StreamingExecutor)
		assert.True(t, ok, "Expected StreamingExecutor when activity callback is set")

		// Verify it's configured with Gemini provider
		if streamExec, ok := runner.base.Executor.(*StreamingExecutor); ok {
			assert.Equal(t, StreamProviderGemini, streamExec.provider)
		}
	})

	t.Run("uses DefaultExecutor when no activity callback", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		runner := NewGeminiRunner(cfg, nil)

		_, ok := runner.base.Executor.(*DefaultExecutor)
		assert.True(t, ok, "Expected DefaultExecutor when no activity callback")
	})

	t.Run("isStreamingEnabled returns true when callback is set", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		runner := NewGeminiRunner(cfg, nil, WithGeminiActivityCallback(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityMedium,
		}))

		assert.True(t, runner.isStreamingEnabled())
	})

	t.Run("isStreamingEnabled returns false when no callback", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}

		runner := NewGeminiRunner(cfg, nil)

		assert.False(t, runner.isStreamingEnabled())
	})
}

func TestGeminiRunner_BuildCommand_Streaming(t *testing.T) {
	t.Run("uses stream-json format when streaming enabled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{}, WithGeminiActivityCallback(ActivityOptions{
			Callback:  func(_ ActivityEvent) {},
			Verbosity: VerbosityMedium,
		}))

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "flash",
		}

		cmd := runner.buildCommand(context.Background(), req)

		// Should use stream-json format
		assert.Contains(t, cmd.Args, "--output-format")
		// Find the index of --output-format and check the next value
		for i, arg := range cmd.Args {
			if arg == "--output-format" && i+1 < len(cmd.Args) {
				assert.Equal(t, "stream-json", cmd.Args[i+1])
				break
			}
		}
	})

	t.Run("uses json format when streaming disabled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "flash",
			Timeout: 30 * time.Minute,
		}
		runner := NewGeminiRunner(cfg, &MockExecutor{})

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "flash",
		}

		cmd := runner.buildCommand(context.Background(), req)

		// Should use json format
		assert.Contains(t, cmd.Args, "--output-format")
		// Find the index of --output-format and check the next value
		for i, arg := range cmd.Args {
			if arg == "--output-format" && i+1 < len(cmd.Args) {
				assert.Equal(t, "json", cmd.Args[i+1])
				break
			}
		}
	})
}

func TestGeminiRunner_StreamResultToGeminiResponse(t *testing.T) {
	t.Parallel()

	cfg := &config.AIConfig{
		Model:   "flash",
		Timeout: 30 * time.Minute,
	}
	runner := NewGeminiRunner(cfg, nil)

	t.Run("converts success result", func(t *testing.T) {
		result := &GeminiStreamResult{
			Success:      true,
			SessionID:    "test-session-123",
			DurationMs:   5419,
			TotalTokens:  16166,
			InputTokens:  15783,
			OutputTokens: 124,
			ToolCalls:    3,
		}

		resp := runner.streamResultToGeminiResponse(result)

		assert.True(t, resp.Success)
		assert.Equal(t, "test-session-123", resp.SessionID)
		assert.Equal(t, 5419, resp.DurationMs)
	})

	t.Run("converts error result", func(t *testing.T) {
		result := &GeminiStreamResult{
			Success:   false,
			SessionID: "error-session",
		}

		resp := runner.streamResultToGeminiResponse(result)

		assert.False(t, resp.Success)
		assert.Equal(t, "error-session", resp.SessionID)
	})
}

// Compile-time check that GeminiRunner implements Runner.
var _ Runner = (*GeminiRunner)(nil)
