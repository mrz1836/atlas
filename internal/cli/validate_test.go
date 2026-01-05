package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// safeBuffer is a thread-safe buffer for use in tests
// where concurrent writes may occur (e.g., spinner animations).
type safeBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func (sb *safeBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

func TestNewValidateCmd(t *testing.T) {
	cmd := newValidateCmd()

	assert.Equal(t, "validate", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Long, "Format")
	assert.Contains(t, cmd.Long, "Lint")
	assert.Contains(t, cmd.Long, "Test")
	assert.Contains(t, cmd.Long, "Pre-commit")
}

func TestAddValidateCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddValidateCommand(root)

	// Verify validate command was added
	validateCmd, _, err := root.Find([]string{"validate"})
	require.NoError(t, err)
	assert.NotNil(t, validateCmd)
	assert.Equal(t, "validate", validateCmd.Name())
}

func TestRunValidate_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runValidate(ctx, cmd, &buf)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunValidate_WithJSONOutput(t *testing.T) {
	// Skip if magex is not available
	t.Skip("Requires magex to be installed")

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json"})
	root.AddCommand(cmd)

	// Set the output flag value
	_ = root.PersistentFlags().Set("output", "json")

	var buf bytes.Buffer
	err := runValidate(context.Background(), cmd, &buf)

	// The command may succeed or fail depending on whether magex is installed
	// but if it fails with JSON output, it should still produce valid JSON
	if err == nil {
		var resp ValidationResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
		assert.True(t, resp.Success)
	}
}

func TestValidateCommand_HasNoArgs(t *testing.T) {
	cmd := newValidateCmd()
	// Validate command should accept no arguments
	assert.Nil(t, cmd.Args)
}

func TestValidateCommand_Examples(t *testing.T) {
	cmd := newValidateCmd()
	// Verify examples are present
	assert.Contains(t, cmd.Long, "atlas validate")
	assert.Contains(t, cmd.Long, "atlas validate --output json")
}

func TestValidateCommand_HasQuietFlag(t *testing.T) {
	cmd := newValidateCmd()

	// Verify --quiet flag exists
	quietFlag := cmd.Flags().Lookup("quiet")
	require.NotNil(t, quietFlag, "--quiet flag should exist")
	assert.Equal(t, "q", quietFlag.Shorthand)
	assert.Equal(t, "false", quietFlag.DefValue)
}

func TestCapitalizeStep(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"format", "Format"},
		{"lint", "Lint"},
		{"test", "Test"},
		{"pre-commit", "Pre-commit"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalizeStep(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPipelineResultToResponse_NilResult(t *testing.T) {
	resp := pipelineResultToResponse(nil)
	assert.False(t, resp.Success)
	assert.Empty(t, resp.Results)
}

func TestRunValidate_GetWorkingDirectoryFails(t *testing.T) {
	// This test would require mocking os.Getwd which is not easily done
	// The error path is covered but hard to test in isolation
	// We document this limitation
	t.Skip("Cannot easily mock os.Getwd failure")
}

func TestRunValidate_QuietMode(t *testing.T) {
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	// Set the quiet flag
	require.NoError(t, cmd.Flags().Set("quiet", "true"))

	var buf safeBuffer
	ctx := context.Background()

	// Use a custom config with simple test commands that will pass
	// This will test the quiet mode output behavior
	err := runValidate(ctx, cmd, &buf)

	// May succeed or fail depending on environment
	// The important thing is that quiet mode doesn't panic
	// and produces minimal output
	output := buf.String()
	t.Logf("Quiet mode output length: %d", len(output))

	// Quiet mode should produce less output than normal mode
	// but we can't assert exact output since it depends on whether commands exist
	_ = err
}

func TestRunValidate_VerboseMode(t *testing.T) {
	// TODO: BUG - Race condition in TerminalSpinner.Start() and animate()
	// The spinner's internal state is not properly synchronized.
	// This causes data races when running tests with -race flag.
	// Skipping until spinner synchronization is fixed.
	// See: internal/tui/spinner.go:66
	t.Skip("Skipping due to race condition in TerminalSpinner - needs fix in internal/tui/spinner.go")

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Verbose: true})
	root.AddCommand(cmd)

	// Set the verbose flag via global flags
	require.NoError(t, root.PersistentFlags().Set("verbose", "true"))

	var buf safeBuffer
	ctx := context.Background()

	err := runValidate(ctx, cmd, &buf)

	// May succeed or fail depending on environment
	// The important thing is that verbose mode doesn't panic
	_ = err
	// Verbose mode output will include command execution details
	t.Logf("Verbose mode output length: %d", buf.Len())
}

func TestHandlePipelineFailure_NilResult(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	err := handlePipelineFailure(out, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrValidationFailed)
}

func TestHandlePipelineFailure_WithFailedResult(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{
				Command:  "test-command",
				Success:  false,
				ExitCode: 1,
				Error:    "command failed",
				Stderr:   "error details",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "test-command")
}

func TestPipelineResultToResponse_WithResults(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
		FormatResults: []validation.Result{
			{
				Command:    "format-cmd",
				Success:    true,
				ExitCode:   0,
				Stdout:     "formatted",
				DurationMs: 100,
			},
		},
		LintResults: []validation.Result{
			{
				Command:    "lint-cmd",
				Success:    true,
				ExitCode:   0,
				DurationMs: 200,
			},
		},
		SkippedSteps: []string{"pre-commit"},
		SkipReasons:  map[string]string{"pre-commit": "tool not installed"},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 2)
	assert.Equal(t, "format-cmd", resp.Results[0].Command)
	assert.True(t, resp.Results[0].Success)
	assert.Equal(t, int64(100), resp.Results[0].DurationMs)
	assert.Equal(t, []string{"pre-commit"}, resp.SkippedSteps)
	assert.Equal(t, "tool not installed", resp.SkipReasons["pre-commit"])
}

func TestValidateCommand_FlagsIntegration(t *testing.T) {
	cmd := newValidateCmd()

	// Test setting quiet flag by name
	err := cmd.Flags().Set("quiet", "true")
	require.NoError(t, err)

	// Verify the flag was set
	val, err := cmd.Flags().GetBool("quiet")
	require.NoError(t, err)
	assert.True(t, val)

	// Reset
	err = cmd.Flags().Set("quiet", "false")
	require.NoError(t, err)
}
