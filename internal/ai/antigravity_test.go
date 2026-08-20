package ai

// This test suite uses MockExecutor to simulate Antigravity CLI (agy) subprocess
// execution. IMPORTANT: Tests NEVER make real API calls or trigger sign-in.
// All AI responses are pre-configured mock data to ensure test isolation.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test error types for Antigravity execution testing.
var (
	errAntigravityTestExecNotFound = errors.New("executable file not found")
	errAntigravityTestExitStatus1  = errors.New("exit status 1")
)

func TestNewAntigravityRunner(t *testing.T) {
	t.Run("creates runner with provided executor", func(t *testing.T) {
		cfg := &config.AIConfig{Model: "pro", Timeout: 30 * time.Minute}
		mockExec := &MockExecutor{}

		runner := NewAntigravityRunner(cfg, mockExec)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.Equal(t, mockExec, runner.base.Executor)
		assert.Equal(t, "agy", runner.base.ProviderName)
	})

	t.Run("creates runner with default executor when nil provided", func(t *testing.T) {
		cfg := &config.AIConfig{Model: "pro", Timeout: 30 * time.Minute}

		runner := NewAntigravityRunner(cfg, nil)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.IsType(t, &DefaultExecutor{}, runner.base.Executor)
	})
}

func TestAntigravityRunner_buildCommand(t *testing.T) {
	t.Run("uses agy binary with print and json output", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{Prompt: "hi"})

		assert.Equal(t, "agy", cmd.Args[0])
		assert.Contains(t, cmd.Args, "-p")
		assert.Contains(t, cmd.Args, "--output-format")
		assert.Contains(t, cmd.Args, "json")
		// Slash/skill expansion must be disabled in non-interactive print mode.
		assert.Contains(t, cmd.Args, "--disable-slash-commands")
	})

	t.Run("implementation mode auto-approves tool use", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{Prompt: "do it"})

		assert.Contains(t, cmd.Args, "--dangerously-skip-permissions")
		assert.NotContains(t, cmd.Args, "plan")
	})

	t.Run("plan mode runs read-only", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt:         "verify",
			PermissionMode: "plan",
		})

		assert.Contains(t, cmd.Args, "--mode")
		assert.Contains(t, cmd.Args, "plan")
		// Plan mode forbids edits, but tool requests are still auto-approved so the
		// non-interactive verify run can read files without stalling on a prompt.
		assert.Contains(t, cmd.Args, "--dangerously-skip-permissions")
	})

	t.Run("resolves pro alias to full model id", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt: "hi",
			Model:  "pro",
		})

		assert.Contains(t, cmd.Args, "--model")
		assert.Contains(t, cmd.Args, "gemini-3.1-pro-high")
	})

	t.Run("resolves flash alias to full model id", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt: "hi",
			Model:  "flash",
		})

		assert.Contains(t, cmd.Args, "gemini-3.7-flash-medium")
	})

	t.Run("passes through full model id unchanged", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt: "hi",
			Model:  "claude-sonnet-4-6",
		})

		assert.Contains(t, cmd.Args, "claude-sonnet-4-6")
	})

	t.Run("falls back to config model when request model empty", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{Model: "pro"}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{Prompt: "hi"})

		assert.Contains(t, cmd.Args, "gemini-3.1-pro-high")
	})

	t.Run("sets working directory when provided", func(t *testing.T) {
		runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt:     "hi",
			WorkingDir: "/tmp/work",
		})

		assert.Equal(t, "/tmp/work", cmd.Dir)
	})
}

func TestAntigravityRunner_Run_Success(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	// Real agy --output-format json schema (not Claude's).
	stdout := `{"conversation_id":"sess-1","status":"SUCCESS",` +
		`"response":"Hello from antigravity","duration_seconds":1.2,"num_turns":2,` +
		`"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`

	mockExec := &MockExecutor{StdoutData: []byte(stdout)}
	runner := NewAntigravityRunner(&config.AIConfig{Model: "pro", Timeout: time.Minute}, mockExec)

	result, err := runner.Run(context.Background(), &domain.AIRequest{Prompt: "hi"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "Hello from antigravity", result.Output)
	assert.Equal(t, "sess-1", result.SessionID)
	assert.Equal(t, 1200, result.DurationMs) // duration_seconds 1.2 -> 1200ms
	assert.Equal(t, 2, result.NumTurns)
	// Prompt is delivered via stdin.
	require.NotNil(t, mockExec.CapturedCmd)
	assert.NotNil(t, mockExec.CapturedCmd.Stdin)
}

func TestAntigravityRunner_Run_ContextCancellation(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	runner := NewAntigravityRunner(&config.AIConfig{Timeout: time.Minute}, &MockExecutor{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runner.Run(ctx, &domain.AIRequest{Prompt: "hi"})

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

func TestAntigravityRunner_Run_CLINotFound(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	mockExec := &MockExecutor{Err: errAntigravityTestExecNotFound}
	runner := NewAntigravityRunner(&config.AIConfig{Timeout: time.Minute}, mockExec)

	result, err := runner.Run(context.Background(), &domain.AIRequest{Prompt: "hi"})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
}

func TestAntigravityRunner_Run_ErrorResponse(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	// Non-SUCCESS status with an explicit message field.
	stdout := `{"conversation_id":"sess-2","status":"ERROR","response":"","message":"boom"}`
	mockExec := &MockExecutor{
		StdoutData: []byte(stdout),
		StderrData: []byte("something failed"),
		Err:        errAntigravityTestExitStatus1,
	}
	runner := NewAntigravityRunner(&config.AIConfig{Timeout: time.Minute}, mockExec)

	result, err := runner.Run(context.Background(), &domain.AIRequest{Prompt: "hi"})

	// Error response is parsed into a result rather than returned as an error.
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "boom")
}

func TestAntigravityRunner_Run_ErrorFallsBackToStderr(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	// Non-SUCCESS status with no message/response: error text comes from stderr.
	stdout := `{"conversation_id":"sess-3","status":"ERROR"}`
	mockExec := &MockExecutor{
		StdoutData: []byte(stdout),
		StderrData: []byte("something failed"),
		Err:        errAntigravityTestExitStatus1,
	}
	runner := NewAntigravityRunner(&config.AIConfig{Timeout: time.Minute}, mockExec)

	result, err := runner.Run(context.Background(), &domain.AIRequest{Prompt: "hi"})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "something failed")
}

func TestParseAntigravityResponse(t *testing.T) {
	t.Run("parses valid response", func(t *testing.T) {
		resp, err := parseAntigravityResponse([]byte(`{"status":"SUCCESS","response":"ok"}`))
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Response)
		assert.True(t, resp.isSuccess())
	})

	t.Run("empty input returns error", func(t *testing.T) {
		_, err := parseAntigravityResponse(nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
	})

	t.Run("invalid json returns antigravity error", func(t *testing.T) {
		_, err := parseAntigravityResponse([]byte(`not json`))
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
	})
}

func TestAntigravityRunner_TerminateRunningProcess(t *testing.T) {
	runner := NewAntigravityRunner(&config.AIConfig{}, &MockExecutor{})
	// MockExecutor does not implement ProcessTerminator, so this is a no-op.
	assert.NoError(t, runner.TerminateRunningProcess())
}
