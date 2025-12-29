package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/tui"
)

// mockCommandRunner is a test implementation of steps.CommandRunner.
type mockCommandRunner struct {
	responses map[string]mockResponse
}

type mockResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (m *mockCommandRunner) Run(_ context.Context, _, command string) (stdout, stderr string, exitCode int, err error) {
	resp, ok := m.responses[command]
	if !ok {
		return "", "command not found", 1, nil
	}
	return resp.stdout, resp.stderr, resp.exitCode, resp.err
}

func TestRunSingleCommand_Success(t *testing.T) {
	runner := &mockCommandRunner{
		responses: map[string]mockResponse{
			"test-cmd": {stdout: "success", exitCode: 0},
		},
	}

	logger := zerolog.Nop()
	result := runSingleCommand(context.Background(), runner, "/tmp", "test-cmd", logger)

	assert.True(t, result.Success)
	assert.Equal(t, "test-cmd", result.Command)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "success", result.Output)
	assert.Empty(t, result.Error)
	assert.Greater(t, result.DurationMs, int64(-1))
}

func TestRunSingleCommand_Failure(t *testing.T) {
	runner := &mockCommandRunner{
		responses: map[string]mockResponse{
			"fail-cmd": {stderr: "error occurred", exitCode: 1},
		},
	}

	logger := zerolog.Nop()
	result := runSingleCommand(context.Background(), runner, "/tmp", "fail-cmd", logger)

	assert.False(t, result.Success)
	assert.Equal(t, "fail-cmd", result.Command)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, "error occurred", result.Error)
}

func TestRunSingleCommand_WithStderr(t *testing.T) {
	runner := &mockCommandRunner{
		responses: map[string]mockResponse{
			"warn-cmd": {stdout: "output", stderr: "warning message", exitCode: 0},
		},
	}

	logger := zerolog.Nop()
	result := runSingleCommand(context.Background(), runner, "/tmp", "warn-cmd", logger)

	// Command succeeded even with stderr
	assert.True(t, result.Success)
	assert.Equal(t, "output", result.Output)
	// Error should be empty since exit code is 0
	assert.Empty(t, result.Error)
}

func TestRunCommandsWithOutput_AllSuccess(t *testing.T) {
	// Save original output
	var buf bytes.Buffer
	out := tui.NewTTYOutput(&buf)
	logger := zerolog.Nop()

	opts := UtilityOptions{
		Verbose:      false,
		OutputFormat: "text",
		Writer:       &buf,
	}

	// Create a simulated command execution
	commands := []string{"echo hello"}
	err := runCommandsWithOutput(
		context.Background(),
		commands,
		"/tmp",
		"Test",
		out,
		opts,
		logger,
	)

	// This will actually run the command, which should succeed
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Test completed successfully")
}

func TestRunCommandsWithOutput_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	out := tui.NewTTYOutput(&buf)
	logger := zerolog.Nop()

	opts := UtilityOptions{
		Verbose:      false,
		OutputFormat: "text",
		Writer:       &buf,
	}

	err := runCommandsWithOutput(
		ctx,
		[]string{"echo hello"},
		"/tmp",
		"Test",
		out,
		opts,
		logger,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunCommandsWithOutput_JSONOutput_Success(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewJSONOutput(&buf)
	logger := zerolog.Nop()

	opts := UtilityOptions{
		Verbose:      false,
		OutputFormat: "json",
		Writer:       &buf,
	}

	err := runCommandsWithOutput(
		context.Background(),
		[]string{"echo hello"},
		"/tmp",
		"Test",
		out,
		opts,
		logger,
	)

	require.NoError(t, err)

	var resp ValidationResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 1)
	assert.True(t, resp.Results[0].Success)
}

func TestRunCommandsWithOutput_JSONOutput_Failure(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewJSONOutput(&buf)
	logger := zerolog.Nop()

	opts := UtilityOptions{
		Verbose:      false,
		OutputFormat: "json",
		Writer:       &buf,
	}

	// Use a command that will fail
	err := runCommandsWithOutput(
		context.Background(),
		[]string{"nonexistent-command-that-does-not-exist"},
		"/tmp",
		"Test",
		out,
		opts,
		logger,
	)

	// The function returns nil when outputting JSON (the response contains success: false)
	assert.NoError(t, err)

	var resp ValidationResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.False(t, resp.Success)
}

func TestHandleValidationFailure_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewJSONOutput(&buf)

	results := []CommandResult{
		{Command: "cmd1", Success: true, ExitCode: 0, DurationMs: 100},
		{Command: "cmd2", Success: false, ExitCode: 1, Error: "failed", DurationMs: 50},
	}

	err := handleValidationFailure(out, "json", results)
	require.NoError(t, err)

	var resp ValidationResponse
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Len(t, resp.Results, 2)
}

func TestHandleValidationFailure_TextOutput(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewTTYOutput(&buf)

	results := []CommandResult{
		{Command: "cmd1", Success: true, ExitCode: 0},
		{Command: "cmd2", Success: false, ExitCode: 1, Error: "something went wrong"},
	}

	err := handleValidationFailure(out, "text", results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, buf.String(), "cmd2")
	assert.Contains(t, buf.String(), "something went wrong")
}

func TestCommandResult_JSONStructure(t *testing.T) {
	result := CommandResult{
		Command:    "test-cmd",
		Success:    true,
		ExitCode:   0,
		Output:     "output text",
		DurationMs: 1234,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, "test-cmd", parsed["command"])
	assert.True(t, parsed["success"].(bool))
	assert.InDelta(t, 0, parsed["exit_code"].(float64), 0.001)
	assert.Equal(t, "output text", parsed["output"])
	assert.InDelta(t, 1234, parsed["duration_ms"].(float64), 0.001)
}

func TestCommandResult_JSONOmitEmpty(t *testing.T) {
	// Test that empty fields are omitted from JSON
	result := CommandResult{
		Command:    "test-cmd",
		Success:    true,
		ExitCode:   0,
		DurationMs: 100,
		// Output and Error are empty
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	_, hasOutput := parsed["output"]
	_, hasError := parsed["error"]
	assert.False(t, hasOutput, "empty output should be omitted")
	assert.False(t, hasError, "empty error should be omitted")
}

func TestValidationResponse_JSONStructure(t *testing.T) {
	resp := ValidationResponse{
		Success: true,
		Results: []CommandResult{
			{Command: "cmd1", Success: true, ExitCode: 0, DurationMs: 100},
			{Command: "cmd2", Success: true, ExitCode: 0, DurationMs: 200},
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.True(t, parsed["success"].(bool))
	results := parsed["results"].([]any)
	assert.Len(t, results, 2)
}

func TestRunSingleCommand_ContextTimeout(t *testing.T) {
	// Create a context that has already expired
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	runner := &mockCommandRunner{
		responses: map[string]mockResponse{
			"slow-cmd": {stdout: "output", exitCode: 0},
		},
	}

	logger := zerolog.Nop()
	result := runSingleCommand(ctx, runner, "/tmp", "slow-cmd", logger)

	// The mock doesn't actually check context, so it will return the mocked response
	// In real usage, the command would respect context cancellation
	assert.Equal(t, "slow-cmd", result.Command)
}

func TestRunCommandsWithOutput_VerboseMode(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewTTYOutput(&buf)
	logger := zerolog.Nop()

	opts := UtilityOptions{
		Verbose:      true,
		OutputFormat: "text",
		Writer:       &buf,
	}

	err := runCommandsWithOutput(
		context.Background(),
		[]string{"echo hello"},
		"/tmp",
		"Test",
		out,
		opts,
		logger,
	)

	require.NoError(t, err)
	// Verbose mode should show "Running:" prefix
	assert.Contains(t, buf.String(), "Running:")
	// Should also show the output
	assert.Contains(t, buf.String(), "hello")
}

func TestGetDefaultCommands(t *testing.T) {
	tests := []struct {
		name     string
		category string
		expected []string
	}{
		{
			name:     "format defaults",
			category: "format",
			expected: []string{"magex format:fix"},
		},
		{
			name:     "lint defaults",
			category: "lint",
			expected: []string{"magex lint"},
		},
		{
			name:     "test defaults",
			category: "test",
			expected: []string{"magex test"},
		},
		{
			name:     "pre-commit defaults",
			category: "pre-commit",
			expected: []string{"go-pre-commit run --all-files"},
		},
		{
			name:     "unknown category",
			category: "unknown",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getDefaultCommands(tc.category)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUtilityOptions_Structure(t *testing.T) {
	opts := UtilityOptions{
		Verbose:      true,
		OutputFormat: "json",
		Writer:       nil,
	}

	assert.True(t, opts.Verbose)
	assert.Equal(t, "json", opts.OutputFormat)
}
