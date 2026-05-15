package ai

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

var (
	errOriginal           = errors.New("original error")
	errOutageInStderrText = errors.New("anthropic api returned 529 overloaded_error")
)

func TestWrapCLIExecutionError(t *testing.T) {
	t.Parallel()

	claudeInfo := CLIInfo{
		Name:        "claude",
		InstallHint: "please install claude code",
		ErrType:     atlaserrors.ErrClaudeInvocation,
		EnvVar:      "ANTHROPIC_API_KEY",
	}

	geminiInfo := CLIInfo{
		Name:        "gemini",
		InstallHint: "install with: npm install -g @google/gemini-cli",
		ErrType:     atlaserrors.ErrGeminiInvocation,
		EnvVar:      "GEMINI_API_KEY",
	}

	t.Run("wraps command not found error for claude", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus127, []byte("command not found"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "claude CLI not found")
		assert.Contains(t, err.Error(), "please install claude code")
	})

	t.Run("wraps command not found error for gemini", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(geminiInfo, errTestExitStatus127, []byte("command not found"))
		require.ErrorIs(t, err, atlaserrors.ErrGeminiInvocation)
		assert.Contains(t, err.Error(), "gemini CLI not found")
		assert.Contains(t, err.Error(), "npm install")
	})

	t.Run("wraps executable file not found error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExecNotFound, []byte(""))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "CLI not found")
	})

	t.Run("wraps API key error from stderr", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus1, []byte("Invalid API key"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("wraps environment variable error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus1, []byte("ANTHROPIC_API_KEY not set"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("wraps authentication error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus1, []byte("authentication failed"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("wraps stderr when present", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus1, []byte("some error message"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "some error message")
	})

	t.Run("wraps original error when no stderr", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errOriginal, []byte(""))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "original error")
	})

	t.Run("trims whitespace from stderr", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionError(claudeInfo, errTestExitStatus1, []byte("  error with whitespace  \n"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "error with whitespace")
		assert.NotContains(t, err.Error(), "  error") // No leading whitespace
	})
}

func TestWrapCLIExecutionErrorWithOp(t *testing.T) {
	t.Parallel()

	claudeInfo := CLIInfo{
		Name:        "claude",
		InstallHint: "please install claude code",
		ErrType:     atlaserrors.ErrClaudeInvocation,
		EnvVar:      "ANTHROPIC_API_KEY",
	}

	t.Run("includes operation context in CLI not found error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "executing prompt", errTestExitStatus127, []byte("command not found"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "while executing prompt")
		assert.Contains(t, err.Error(), "CLI not found")
	})

	t.Run("includes operation context in API key error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "sending request", errTestExitStatus1, []byte("Invalid API key"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "while sending request")
		assert.Contains(t, err.Error(), "API key error")
	})

	t.Run("includes operation context in stderr error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "processing response", errTestExitStatus1, []byte("some error"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "while processing response")
		assert.Contains(t, err.Error(), "some error")
	})

	t.Run("includes operation context in original error", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "running AI", errOriginal, []byte(""))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "while running AI")
		assert.Contains(t, err.Error(), "original error")
	})

	t.Run("empty operation produces no context", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "", errOriginal, []byte(""))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.NotContains(t, err.Error(), "while")
	})
}

func TestWrapCLIExecutionError_OutageDetection(t *testing.T) {
	t.Parallel()

	claudeInfo := CLIInfo{
		Name:          "claude",
		InstallHint:   "please install claude code",
		ErrType:       atlaserrors.ErrClaudeInvocation,
		EnvVar:        "ANTHROPIC_API_KEY",
		StatusPageURL: "https://status.anthropic.com",
	}

	cases := []struct {
		name   string
		stderr string
	}{
		{name: "overloaded_error JSON from claude", stderr: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`},
		{name: "529 status code", stderr: "API Error: 529 overloaded_error"},
		{name: "503 service unavailable", stderr: "HTTP 503 Service Unavailable"},
		{name: "500 internal server error", stderr: "Internal Server Error"},
		{name: "upstream connect error", stderr: "upstream connect error or disconnect/reset before headers"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := WrapCLIExecutionErrorWithOp(claudeInfo, "analyze", errTestExitStatus1, []byte(tc.stderr))
			require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation, "must preserve provider sentinel")
			require.ErrorIs(t, err, atlaserrors.ErrProviderOutage, "must mark as provider outage")
			assert.Contains(t, err.Error(), "while analyze")
		})
	}

	t.Run("non-outage stderr does not get outage sentinel", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "analyze", errTestExitStatus1, []byte("Bad Request: invalid prompt"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.NotErrorIs(t, err, atlaserrors.ErrProviderOutage)
	})

	t.Run("api key takes precedence over outage markers", func(t *testing.T) {
		// If both API key and 5xx markers appear, treat as API key error so
		// the user fixes their credentials rather than waiting for an outage.
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "analyze", errTestExitStatus1, []byte("Invalid API key, 503 returned"))
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "API key error")
		assert.NotErrorIs(t, err, atlaserrors.ErrProviderOutage)
	})

	t.Run("outage detected from err string when stderr empty", func(t *testing.T) {
		t.Parallel()
		err := WrapCLIExecutionErrorWithOp(claudeInfo, "analyze", errOutageInStderrText, nil)
		require.ErrorIs(t, err, atlaserrors.ErrProviderOutage)
	})
}

func TestIsOutageStderr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "overloaded_error", input: "overloaded_error encountered", want: true},
		{name: "529", input: "got 529 response", want: true},
		{name: "503 Service Unavailable", input: "503 service unavailable", want: true},
		{name: "uppercase outage marker", input: "INTERNAL SERVER ERROR", want: true},
		{name: "non-outage 4xx", input: "bad request: 400", want: false},
		{name: "neutral text", input: "all good", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, isOutageStderr(tc.input))
		})
	}
}

func TestFormatOpContext(t *testing.T) {
	t.Parallel()

	t.Run("returns empty for empty operation", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, formatOpContext(""))
	})

	t.Run("formats non-empty operation", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, " while executing", formatOpContext("executing"))
	})

	t.Run("formats multi-word operation", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, " while processing AI response", formatOpContext("processing AI response"))
	})
}
