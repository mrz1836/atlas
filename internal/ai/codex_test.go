package ai

// This test suite uses MockExecutor to simulate Codex CLI subprocess execution.
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

// Test error types for Codex execution testing.
var (
	errCodexTestNetwork      = errors.New("network error")
	errCodexTestExecNotFound = errors.New("executable file not found")
	errCodexTestExitStatus1  = errors.New("exit status 1")
	errCodexTestCmdNotFound  = errors.New("exec: command not found")
)

func TestNewCodexRunner(t *testing.T) {
	t.Run("creates runner with provided executor", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "codex",
			Timeout: 30 * time.Minute,
		}
		mockExec := &MockExecutor{}

		runner := NewCodexRunner(cfg, mockExec)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.Equal(t, mockExec, runner.base.Executor)
	})

	t.Run("creates runner with default executor when nil provided", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "codex",
			Timeout: 30 * time.Minute,
		}

		runner := NewCodexRunner(cfg, nil)

		require.NotNil(t, runner)
		assert.Equal(t, cfg, runner.base.Config)
		assert.IsType(t, &DefaultExecutor{}, runner.base.Executor)
	})
}

func TestCodexRunner_Run_ContextCancellation(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("returns error when context is canceled", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "codex",
			Timeout: 30 * time.Minute,
		}
		runner := NewCodexRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "codex",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})

	t.Run("returns error when context deadline exceeded", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model:   "codex",
			Timeout: 30 * time.Minute,
		}
		runner := NewCodexRunner(cfg, &MockExecutor{})

		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		// Allow the deadline to expire
		time.Sleep(time.Millisecond)

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "codex",
		}

		result, err := runner.Run(ctx, req)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, result)
	})
}

func TestCodexRunner_buildCommand(t *testing.T) {
	t.Run("builds command with default flags", func(t *testing.T) {
		cfg := &config.AIConfig{}
		runner := NewCodexRunner(cfg, nil)

		req := &domain.AIRequest{
			Prompt: "test prompt",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "codex", cmd.Path[len(cmd.Path)-5:]) // ends with "codex"
		assert.Contains(t, cmd.Args, "exec")
		assert.Contains(t, cmd.Args, "--json")
		assert.Contains(t, cmd.Args, "--skip-git-repo-check")
		// Default (non-plan) permission mode uses a writable sandbox.
		assert.Contains(t, cmd.Args, "--sandbox")
		assert.Contains(t, cmd.Args, "workspace-write")
	})

	t.Run("plan mode uses read-only sandbox", func(t *testing.T) {
		runner := NewCodexRunner(&config.AIConfig{}, nil)

		cmd := runner.buildCommand(context.Background(), &domain.AIRequest{
			Prompt:         "verify this",
			PermissionMode: "plan",
		})

		assert.Contains(t, cmd.Args, "--sandbox")
		assert.Contains(t, cmd.Args, "read-only")
		assert.NotContains(t, cmd.Args, "workspace-write")
	})

	t.Run("builds command with model from request", func(t *testing.T) {
		cfg := &config.AIConfig{}
		runner := NewCodexRunner(cfg, nil)

		req := &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "codex",
		}

		cmd := runner.buildCommand(context.Background(), req)

		// Model should be resolved and included
		assert.Contains(t, cmd.Args, "-m")
		assert.Contains(t, cmd.Args, "gpt-5.4")
	})

	t.Run("builds command with model from config when request has none", func(t *testing.T) {
		cfg := &config.AIConfig{
			Model: "max",
		}
		runner := NewCodexRunner(cfg, nil)

		req := &domain.AIRequest{
			Prompt: "test prompt",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Contains(t, cmd.Args, "-m")
		assert.Contains(t, cmd.Args, "gpt-5.5")
	})

	t.Run("sets working directory when specified", func(t *testing.T) {
		cfg := &config.AIConfig{}
		runner := NewCodexRunner(cfg, nil)

		req := &domain.AIRequest{
			Prompt:     "test prompt",
			WorkingDir: "/test/dir",
		}

		cmd := runner.buildCommand(context.Background(), req)

		assert.Equal(t, "/test/dir", cmd.Dir)
	})
}

func TestCodexRunner_execute(t *testing.T) {
	EnsureNoRealAPIKeys(t)

	t.Run("executes successfully and parses response", func(t *testing.T) {
		mockExec := &MockExecutor{
			StdoutData: []byte(`{"type":"thread.started","thread_id":"test-session"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"test output"}}
{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`),
		}
		runner := NewCodexRunner(&config.AIConfig{}, mockExec)

		result, err := runner.execute(context.Background(), &domain.AIRequest{
			Prompt: "test prompt",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "test output", result.Output)
		assert.Equal(t, "test-session", result.SessionID)
		assert.Equal(t, 1, result.NumTurns)
	})

	t.Run("returns error on execution failure without valid JSON", func(t *testing.T) {
		mockExec := &MockExecutor{
			Err:        errCodexTestNetwork,
			StderrData: []byte("connection refused"),
		}
		runner := NewCodexRunner(&config.AIConfig{}, mockExec)

		result, err := runner.execute(context.Background(), &domain.AIRequest{
			Prompt: "test prompt",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
	})

	t.Run("returns error with CLI not found message", func(t *testing.T) {
		mockExec := &MockExecutor{
			Err:        errCodexTestExecNotFound,
			StderrData: []byte("executable file not found"),
		}
		runner := NewCodexRunner(&config.AIConfig{}, mockExec)

		result, err := runner.execute(context.Background(), &domain.AIRequest{
			Prompt: "test prompt",
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "npm install -g @openai/codex")
	})

	t.Run("returns parsed error response on execution failure with valid JSON", func(t *testing.T) {
		mockExec := &MockExecutor{
			Err: errCodexTestExitStatus1,
			StdoutData: []byte(`{"type":"thread.started","thread_id":"s"}
{"type":"turn.failed","message":"API rate limit exceeded"}`),
		}
		runner := NewCodexRunner(&config.AIConfig{}, mockExec)

		result, err := runner.execute(context.Background(), &domain.AIRequest{
			Prompt: "test prompt",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Contains(t, result.Error, "exit status 1")
	})
}

func TestWrapCodexExecutionError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		stderr         []byte
		expectedSubstr string
	}{
		{
			name:           "command not found",
			err:            errCodexTestCmdNotFound,
			stderr:         []byte("command not found"),
			expectedSubstr: "npm install -g @openai/codex",
		},
		{
			name:           "executable not found",
			err:            errCodexTestExecNotFound,
			stderr:         []byte{},
			expectedSubstr: "npm install -g @openai/codex",
		},
		{
			name:           "API key error in stderr",
			err:            errCodexTestExitStatus1,
			stderr:         []byte("OPENAI_API_KEY not set"),
			expectedSubstr: "API key error",
		},
		{
			name:           "generic error with stderr",
			err:            errCodexTestExitStatus1,
			stderr:         []byte("some error message"),
			expectedSubstr: "some error message",
		},
		{
			name:           "generic error without stderr",
			err:            errCodexTestNetwork,
			stderr:         []byte{},
			expectedSubstr: "network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WrapCLIExecutionError(codexCLIInfo, tt.err, tt.stderr)
			require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
			assert.Contains(t, err.Error(), tt.expectedSubstr)
		})
	}
}

func TestParseCodexResponse(t *testing.T) {
	t.Run("parses agent message from event stream", func(t *testing.T) {
		data := []byte(`{"type":"thread.started","thread_id":"sess-123"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Hello world"}}
{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":5}}`)

		resp, err := parseCodexResponse(data)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "Hello world", resp.Content)
		assert.Equal(t, "sess-123", resp.SessionID)
		assert.Equal(t, 1, resp.NumTurns)
		assert.Equal(t, 100, resp.Usage.InputTokens)
		assert.Equal(t, 5, resp.Usage.OutputTokens)
	})

	t.Run("ignores non-fatal warning item when answer is present", func(t *testing.T) {
		data := []byte(`{"type":"item.completed","item":{"type":"error","message":"Model metadata for gpt-5.6 not found. Defaulting to fallback metadata."}}
{"type":"item.completed","item":{"type":"agent_message","text":"OK"}}
{"type":"turn.completed"}`)

		resp, err := parseCodexResponse(data)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "OK", resp.Content)
		assert.Empty(t, resp.Error)
	})

	t.Run("surfaces turn.failed as error", func(t *testing.T) {
		data := []byte(`{"type":"thread.started","thread_id":"s"}
{"type":"turn.failed","message":"Rate limit exceeded"}`)

		resp, err := parseCodexResponse(data)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "Rate limit exceeded", resp.Error)
	})

	t.Run("surfaces terminal error item when no answer", func(t *testing.T) {
		data := []byte(`{"type":"item.completed","item":{"type":"error","message":"stream disconnected"}}`)

		resp, err := parseCodexResponse(data)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "stream disconnected", resp.Error)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		resp, err := parseCodexResponse([]byte{})

		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("returns error when no json events present", func(t *testing.T) {
		resp, err := parseCodexResponse([]byte("not json at all"))

		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
		assert.Contains(t, err.Error(), "no json events")
	})
}

func TestCodexResponse_toAIResult(t *testing.T) {
	t.Run("converts success response", func(t *testing.T) {
		resp := &CodexResponse{
			Success:   true,
			Content:   "output content",
			SessionID: "sess-456",
			NumTurns:  2,
		}

		result := resp.toAIResult("")

		assert.True(t, result.Success)
		assert.Equal(t, "output content", result.Output)
		assert.Equal(t, "sess-456", result.SessionID)
		assert.Equal(t, 2, result.NumTurns)
		assert.Empty(t, result.Error)
	})

	t.Run("includes error from response", func(t *testing.T) {
		resp := &CodexResponse{
			Success: false,
			Error:   "API error",
		}

		result := resp.toAIResult("")

		assert.False(t, result.Success)
		assert.Equal(t, "API error", result.Error)
	})

	t.Run("uses stderr when response error is empty", func(t *testing.T) {
		resp := &CodexResponse{
			Success: false,
		}

		result := resp.toAIResult("stderr content")

		assert.False(t, result.Success)
		assert.Equal(t, "stderr content", result.Error)
	})
}

func TestDefaultExecutor_Execute_Codex(t *testing.T) {
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

// MockExecutor for Codex tests (uses the same mock from claude_test.go)
// If needed, define additional mock behavior here.
