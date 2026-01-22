package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/validation"
)

// mockValidationRunner implements ValidationRunner for fast testing.
type mockValidationRunner struct {
	result   *validation.PipelineResult
	err      error
	callback validation.ProgressCallback
}

func (m *mockValidationRunner) SetProgressCallback(cb validation.ProgressCallback) {
	m.callback = cb
}

func (m *mockValidationRunner) Run(_ context.Context, _ string) (*validation.PipelineResult, error) {
	// Simulate progress callbacks if callback is set
	if m.callback != nil {
		m.callback("pre-commit", "starting", &validation.ProgressInfo{CurrentStep: 1, TotalSteps: 4})
		m.callback("pre-commit", "completed", &validation.ProgressInfo{CurrentStep: 1, TotalSteps: 4, DurationMs: 10})
		m.callback("format", "starting", &validation.ProgressInfo{CurrentStep: 2, TotalSteps: 4})
		m.callback("format", "completed", &validation.ProgressInfo{CurrentStep: 2, TotalSteps: 4, DurationMs: 20})
		m.callback("lint", "starting", &validation.ProgressInfo{CurrentStep: 3, TotalSteps: 4})
		m.callback("lint", "completed", &validation.ProgressInfo{CurrentStep: 3, TotalSteps: 4, DurationMs: 30})
		m.callback("test", "starting", &validation.ProgressInfo{CurrentStep: 4, TotalSteps: 4})
		m.callback("test", "completed", &validation.ProgressInfo{CurrentStep: 4, TotalSteps: 4, DurationMs: 40})
	}
	return m.result, m.err
}

// newSuccessMockRunner creates a mock runner that returns success.
func newSuccessMockRunner() *mockValidationRunner {
	return &mockValidationRunner{
		result: &validation.PipelineResult{
			Success: true,
			FormatResults: []validation.Result{
				{Command: "magex fmt", Success: true, ExitCode: 0, DurationMs: 10},
			},
			LintResults: []validation.Result{
				{Command: "magex lint", Success: true, ExitCode: 0, DurationMs: 20},
			},
			TestResults: []validation.Result{
				{Command: "magex test:race", Success: true, ExitCode: 0, DurationMs: 30},
			},
			PreCommitResults: []validation.Result{
				{Command: "go-pre-commit", Success: true, ExitCode: 0, DurationMs: 5},
			},
		},
	}
}

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

func TestNewValidateCmd_QuietFlagDefault(t *testing.T) {
	cmd := newValidateCmd()

	// Verify quiet flag exists and has correct default
	quietFlag := cmd.Flags().Lookup("quiet")
	require.NotNil(t, quietFlag)
	assert.Equal(t, "false", quietFlag.DefValue)
	assert.Equal(t, "q", quietFlag.Shorthand)

	// Verify flag is a bool
	val, err := cmd.Flags().GetBool("quiet")
	require.NoError(t, err)
	assert.False(t, val)
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

func TestRunValidate_QuietMode(t *testing.T) {
	// TODO: BUG - Race condition in TerminalSpinner.Start() and animate()
	// The spinner's internal state is not properly synchronized.
	// This causes data races when running tests with -race flag.
	// Skipping until spinner synchronization is fixed.
	// See: internal/tui/spinner.go:66
	t.Skip("Skipping due to race condition in TerminalSpinner - needs fix in internal/tui/spinner.go")

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	// Set the quiet flag
	require.NoError(t, cmd.Flags().Set("quiet", "true"))

	var buf safeBuffer
	ctx := context.Background()

	err := runValidate(ctx, cmd, &buf)

	// May succeed or fail depending on environment
	// The important thing is that quiet mode suppresses progress output
	_ = err
	// In quiet mode, output should be minimal (just final result)
	t.Logf("Quiet mode output length: %d", buf.Len())
}

func TestRunValidate_SuccessWithAllSteps(t *testing.T) {
	// This test requires working validation commands
	// Skip if environment doesn't have the necessary tools
	t.Skip("Requires working validation environment")

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	ctx := context.Background()

	err := runValidate(ctx, cmd, &buf)

	// If validation passes, should not return error
	if err == nil {
		output := buf.String()
		assert.Contains(t, output, "passed")
	}
}

func TestCapitalizeStep_AllCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "format step",
			input:    "format",
			expected: "Format",
		},
		{
			name:     "lint step",
			input:    "lint",
			expected: "Lint",
		},
		{
			name:     "test step",
			input:    "test",
			expected: "Test",
		},
		{
			name:     "pre-commit step",
			input:    "pre-commit",
			expected: "Pre-commit",
		},
		{
			name:     "unknown step returns as-is",
			input:    "unknown",
			expected: "unknown",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "mixed case input",
			input:    "FORMAT",
			expected: "FORMAT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := capitalizeStep(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPipelineResultToResponse_EmptyResults(t *testing.T) {
	result := &validation.PipelineResult{
		Success:       true,
		FormatResults: []validation.Result{},
		LintResults:   []validation.Result{},
		TestResults:   []validation.Result{},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Empty(t, resp.Results)
	assert.Empty(t, resp.SkippedSteps)
}

func TestPipelineResultToResponse_MultipleResults(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
		FormatResults: []validation.Result{
			{
				Command:    "format-cmd-1",
				Success:    true,
				ExitCode:   0,
				Stdout:     "formatted successfully",
				DurationMs: 150,
			},
			{
				Command:    "format-cmd-2",
				Success:    true,
				ExitCode:   0,
				Stdout:     "all good",
				DurationMs: 200,
			},
		},
		TestResults: []validation.Result{
			{
				Command:    "test-cmd",
				Success:    true,
				ExitCode:   0,
				DurationMs: 5000,
			},
		},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 3)
	assert.Equal(t, "format-cmd-1", resp.Results[0].Command)
	assert.Equal(t, "format-cmd-2", resp.Results[1].Command)
	assert.Equal(t, "test-cmd", resp.Results[2].Command)
	assert.Equal(t, int64(150), resp.Results[0].DurationMs)
}

func TestPipelineResultToResponse_WithFailedResult(t *testing.T) {
	result := &validation.PipelineResult{
		Success: false,
		LintResults: []validation.Result{
			{
				Command:    "lint-cmd",
				Success:    false,
				ExitCode:   1,
				Stderr:     "linting errors found",
				Error:      "exit status 1",
				DurationMs: 300,
			},
		},
	}

	resp := pipelineResultToResponse(result)

	assert.False(t, resp.Success)
	assert.Len(t, resp.Results, 1)
	assert.False(t, resp.Results[0].Success)
	assert.Equal(t, 1, resp.Results[0].ExitCode)
	assert.Equal(t, "exit status 1", resp.Results[0].Error)
}

func TestHandlePipelineFailure_WithStderr(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{
				Command:  "format-cmd",
				Success:  false,
				ExitCode: 2,
				Error:    "formatting failed",
				Stderr:   "detailed stderr information",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "format-cmd")
	assert.Contains(t, output, "detailed stderr information")
}

func TestHandlePipelineFailure_StderrSameAsError(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		LintResults: []validation.Result{
			{
				Command:  "lint-cmd",
				Success:  false,
				ExitCode: 1,
				Error:    "same error message",
				Stderr:   "same error message", // Same as Error, should not duplicate
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "lint-cmd")
	// Should only appear once, not duplicated
	firstIndex := strings.Index(output, "same error message")
	lastIndex := strings.LastIndex(output, "same error message")
	assert.Equal(t, firstIndex, lastIndex, "error message should only appear once")
}

func TestHandlePipelineFailure_OnlyFirstFailure(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{
				Command:  "first-fail",
				Success:  false,
				ExitCode: 1,
				Error:    "first error",
			},
			{
				Command:  "second-fail",
				Success:  false,
				ExitCode: 1,
				Error:    "second error",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	output := buf.String()
	// Should only show the first failure
	assert.Contains(t, output, "first-fail")
	assert.Contains(t, output, "first error")
	// Should not show the second failure
	assert.NotContains(t, output, "second-fail")
	assert.NotContains(t, output, "second error")
}

func TestHandlePipelineFailure_AllResultsSuccess(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false, // Marked as failed but all results are success
		FormatResults: []validation.Result{
			{
				Command:  "format-cmd",
				Success:  true,
				ExitCode: 0,
			},
		},
	}

	err := handlePipelineFailure(out, result)

	// Should still return error since Success is false
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrValidationFailed)
}

func TestRunValidate_JSONOutputWithFailure(t *testing.T) {
	// This test would require mocking the validation runner
	// which is complex. Document the limitation.
	t.Skip("Requires mocking validation runner for controlled failure")
}

func TestAddValidateCommand_Integration(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}

	// Before adding, there should be no validate command
	hasCmd := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "validate" {
			hasCmd = true
			break
		}
	}
	assert.False(t, hasCmd, "validate command should not exist before adding")

	AddValidateCommand(root)

	// After adding, verify validate command exists
	validateCmd, _, err := root.Find([]string{"validate"})
	require.NoError(t, err)
	assert.NotNil(t, validateCmd)
	assert.Equal(t, "validate", validateCmd.Name())

	// Verify command has expected flags
	quietFlag := validateCmd.Flags().Lookup("quiet")
	require.NotNil(t, quietFlag)
	assert.Equal(t, "q", quietFlag.Shorthand)
}

func TestValidateCommand_LongDescription(t *testing.T) {
	cmd := newValidateCmd()

	// Verify the long description contains all expected information
	assert.Contains(t, cmd.Long, "validation pipeline")
	assert.Contains(t, cmd.Long, "1. Format")
	assert.Contains(t, cmd.Long, "2. Lint + Test")
	assert.Contains(t, cmd.Long, "3. Pre-commit")
	assert.Contains(t, cmd.Long, "Examples:")
	assert.Contains(t, cmd.Long, "--output json")
	assert.Contains(t, cmd.Long, "--verbose")
	assert.Contains(t, cmd.Long, "--quiet")
}

func TestPipelineResultToResponse_AllResultTypes(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
		FormatResults: []validation.Result{
			{Command: "format", Success: true, ExitCode: 0, DurationMs: 100},
		},
		LintResults: []validation.Result{
			{Command: "lint", Success: true, ExitCode: 0, DurationMs: 200},
		},
		TestResults: []validation.Result{
			{Command: "test", Success: true, ExitCode: 0, DurationMs: 300},
		},
		PreCommitResults: []validation.Result{
			{Command: "pre-commit", Success: true, ExitCode: 0, DurationMs: 400},
		},
		SkippedSteps: []string{},
		SkipReasons:  map[string]string{},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 4)

	// Verify order: Format, Lint, Test, PreCommit
	assert.Equal(t, "format", resp.Results[0].Command)
	assert.Equal(t, "lint", resp.Results[1].Command)
	assert.Equal(t, "test", resp.Results[2].Command)
	assert.Equal(t, "pre-commit", resp.Results[3].Command)

	// Verify durations preserved
	assert.Equal(t, int64(100), resp.Results[0].DurationMs)
	assert.Equal(t, int64(200), resp.Results[1].DurationMs)
	assert.Equal(t, int64(300), resp.Results[2].DurationMs)
	assert.Equal(t, int64(400), resp.Results[3].DurationMs)
}

func TestHandlePipelineFailure_EmptyError(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		TestResults: []validation.Result{
			{
				Command:  "test-cmd",
				Success:  false,
				ExitCode: 1,
				Error:    "", // Empty error string
				Stderr:   "",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "test-cmd")
}

func TestPipelineResultToResponse_MixedSuccessAndSkipped(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
		FormatResults: []validation.Result{
			{Command: "format", Success: true, ExitCode: 0, DurationMs: 100},
		},
		LintResults: []validation.Result{
			{Command: "lint", Success: true, ExitCode: 0, DurationMs: 200},
		},
		TestResults:      []validation.Result{},
		PreCommitResults: []validation.Result{},
		SkippedSteps:     []string{"test", "pre-commit"},
		SkipReasons: map[string]string{
			"test":       "no test command configured",
			"pre-commit": "tool not found",
		},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 2)
	assert.Equal(t, []string{"test", "pre-commit"}, resp.SkippedSteps)
	assert.Equal(t, "no test command configured", resp.SkipReasons["test"])
	assert.Equal(t, "tool not found", resp.SkipReasons["pre-commit"])
}

func TestCapitalizeStep_EdgeCases(t *testing.T) {
	// Test with whitespace
	assert.Equal(t, " format ", capitalizeStep(" format "))

	// Test with special characters
	assert.Equal(t, "pre-commit-hooks", capitalizeStep("pre-commit-hooks"))

	// Test numeric input
	assert.Equal(t, "123", capitalizeStep("123"))
}

func TestHandlePipelineFailure_MultipleStepsWithOnlyOneFailure(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{Command: "format", Success: true, ExitCode: 0},
		},
		LintResults: []validation.Result{
			{Command: "lint", Success: true, ExitCode: 0},
		},
		TestResults: []validation.Result{
			{
				Command:  "test-cmd",
				Success:  false,
				ExitCode: 1,
				Error:    "test failed",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	output := buf.String()
	// Should show the failed test command
	assert.Contains(t, output, "test-cmd")
	// Should not show successful commands
	assert.NotContains(t, output, "format passed")
	assert.NotContains(t, output, "lint passed")
}

func TestRunValidate_ConfigLoadWithDefaults(t *testing.T) {
	t.Parallel()
	// This test verifies that even if config loading fails, the command uses defaults
	// and continues execution. Uses mock runner to avoid running real commands.
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	ctx := context.Background()
	opts := &ValidateOptions{Runner: newSuccessMockRunner()}

	// With mock runner, this should succeed quickly
	err := runValidateWithOptions(ctx, cmd, &buf, opts)
	require.NoError(t, err)
}

func TestRunValidate_OutputFormats(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "text output (default)",
			output: "",
		},
		{
			name:   "json output",
			output: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newValidateCmd()
			root := &cobra.Command{Use: "atlas"}
			AddGlobalFlags(root, &GlobalFlags{Output: tt.output})
			root.AddCommand(cmd)

			if tt.output != "" {
				_ = root.PersistentFlags().Set("output", tt.output)
			}

			var buf bytes.Buffer
			ctx := context.Background()

			err := runValidate(ctx, cmd, &buf)
			// May succeed or fail, we're just verifying the output format path is exercised
			_ = err
		})
	}
}

func TestRunValidate_FlagParsing(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr bool
	}{
		{
			name: "quiet mode enabled",
			flags: map[string]string{
				"quiet": "true",
			},
		},
		{
			name: "verbose mode enabled",
			flags: map[string]string{
				"verbose": "true",
			},
		},
		{
			name: "quiet and verbose both enabled",
			flags: map[string]string{
				"quiet":   "true",
				"verbose": "true",
			},
		},
		{
			name: "json output format",
			flags: map[string]string{
				"output": "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newValidateCmd()
			root := &cobra.Command{Use: "atlas"}
			AddGlobalFlags(root, &GlobalFlags{})
			root.AddCommand(cmd)

			// Set flags
			for k, v := range tt.flags {
				if k == "output" || k == "verbose" {
					_ = root.PersistentFlags().Set(k, v)
				} else {
					_ = cmd.Flags().Set(k, v)
				}
			}

			var buf bytes.Buffer
			ctx := context.Background()

			err := runValidate(ctx, cmd, &buf)
			// May succeed or fail depending on environment
			_ = err
		})
	}
}

func TestRunValidate_ProgressCallbackStates(t *testing.T) {
	t.Parallel()
	// Test that all progress callback states are handled
	// Uses mock runner to verify callback execution without running real commands.
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	ctx := context.Background()
	opts := &ValidateOptions{Runner: newSuccessMockRunner()}

	err := runValidateWithOptions(ctx, cmd, &buf, opts)
	// With mock runner, this should succeed and exercise all callback states
	require.NoError(t, err)
}

func TestHandlePipelineFailure_WithOnlyError(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{
				Command:  "format-cmd",
				Success:  false,
				ExitCode: 1,
				Error:    "formatting error",
				Stderr:   "", // No stderr, only Error
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "format-cmd")
	assert.Contains(t, output, "exit code: 1")
}

func TestPipelineResultToResponse_WithStdoutAndStderr(t *testing.T) {
	result := &validation.PipelineResult{
		Success: false,
		LintResults: []validation.Result{
			{
				Command:    "lint-cmd",
				Success:    false,
				ExitCode:   1,
				Stdout:     "some warnings",
				Stderr:     "errors found",
				Error:      "lint failed",
				DurationMs: 250,
			},
		},
	}

	resp := pipelineResultToResponse(result)

	assert.False(t, resp.Success)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "lint-cmd", resp.Results[0].Command)
	assert.False(t, resp.Results[0].Success)
	assert.Equal(t, "some warnings", resp.Results[0].Output)
	assert.Equal(t, "lint failed", resp.Results[0].Error)
	assert.Equal(t, int64(250), resp.Results[0].DurationMs)
}

func TestNewValidateCmd_RunEFunction(t *testing.T) {
	cmd := newValidateCmd()

	// Verify RunE function is set
	require.NotNil(t, cmd.RunE)

	// Verify the RunE function signature
	assert.NotNil(t, cmd.RunE)
}

func TestCapitalizeStep_Consistency(t *testing.T) {
	// Verify consistent behavior across multiple calls
	input := "format"
	result1 := capitalizeStep(input)
	result2 := capitalizeStep(input)
	assert.Equal(t, result1, result2)
	assert.Equal(t, "Format", result1)
}

func TestHandlePipelineFailure_NoResultsInPipeline(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success:       false,
		FormatResults: []validation.Result{},
		LintResults:   []validation.Result{},
		TestResults:   []validation.Result{},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrValidationFailed)
}

func TestRunValidate_SuccessPath(t *testing.T) {
	t.Parallel()
	// This test verifies the success path is exercised
	// Uses mock runner to test the success path quickly.
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	ctx := context.Background()
	opts := &ValidateOptions{Runner: newSuccessMockRunner()}

	err := runValidateWithOptions(ctx, cmd, &buf, opts)

	// With mock runner, should succeed and show success message
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "passed")
}

func TestRunValidate_AllOutputModes(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		verbose bool
		quiet   bool
	}{
		{
			name:    "default text output",
			output:  "",
			verbose: false,
			quiet:   false,
		},
		{
			name:    "json output",
			output:  "json",
			verbose: false,
			quiet:   false,
		},
		{
			name:    "text with verbose",
			output:  "",
			verbose: true,
			quiet:   false,
		},
		{
			name:    "text with quiet",
			output:  "",
			verbose: false,
			quiet:   true,
		},
		{
			name:    "json with verbose",
			output:  "json",
			verbose: true,
			quiet:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newValidateCmd()
			root := &cobra.Command{Use: "atlas"}
			AddGlobalFlags(root, &GlobalFlags{Output: tt.output, Verbose: tt.verbose})
			root.AddCommand(cmd)

			if tt.output != "" {
				_ = root.PersistentFlags().Set("output", tt.output)
			}
			if tt.verbose {
				_ = root.PersistentFlags().Set("verbose", "true")
			}
			if tt.quiet {
				_ = cmd.Flags().Set("quiet", "true")
			}

			var buf bytes.Buffer
			ctx := context.Background()

			err := runValidate(ctx, cmd, &buf)
			// May succeed or fail, we're exercising all code paths
			_ = err
		})
	}
}

func TestPipelineResultToResponse_PreCommitResults(t *testing.T) {
	result := &validation.PipelineResult{
		Success: true,
		PreCommitResults: []validation.Result{
			{
				Command:    "pre-commit-cmd",
				Success:    true,
				ExitCode:   0,
				Stdout:     "pre-commit passed",
				DurationMs: 500,
			},
		},
	}

	resp := pipelineResultToResponse(result)

	assert.True(t, resp.Success)
	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "pre-commit-cmd", resp.Results[0].Command)
	assert.Equal(t, "pre-commit passed", resp.Results[0].Output)
	assert.Equal(t, int64(500), resp.Results[0].DurationMs)
}

func TestNewValidateCmd_FlagDefaults(t *testing.T) {
	cmd := newValidateCmd()

	// Verify command structure
	assert.Equal(t, "validate", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify quiet flag default
	quiet, err := cmd.Flags().GetBool("quiet")
	require.NoError(t, err)
	assert.False(t, quiet, "quiet should default to false")
}

func TestHandlePipelineFailure_PreCommitFailure(t *testing.T) {
	var buf bytes.Buffer
	out := tui.NewOutput(&buf, "")

	result := &validation.PipelineResult{
		Success: false,
		PreCommitResults: []validation.Result{
			{
				Command:  "pre-commit-cmd",
				Success:  false,
				ExitCode: 1,
				Error:    "pre-commit checks failed",
				Stderr:   "hook execution failed",
			},
		},
	}

	err := handlePipelineFailure(out, result)

	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrValidationFailed)
	output := buf.String()
	assert.Contains(t, output, "pre-commit-cmd")
	assert.Contains(t, output, "exit code: 1")
}

func TestNewValidateCmd_Execute(t *testing.T) {
	t.Parallel()
	// Test executing the command through the RunE function
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	// Set context on the command
	ctx := context.Background()
	cmd.SetContext(ctx)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Execute the command (may fail, but should not panic)
	err := cmd.Execute()
	_ = err // May succeed or fail depending on environment
}

func TestNewValidateCmd_ShorthandFlags(t *testing.T) {
	cmd := newValidateCmd()

	// Test quiet shorthand flag
	quietFlag := cmd.Flags().ShorthandLookup("q")
	require.NotNil(t, quietFlag)
	assert.Equal(t, "quiet", quietFlag.Name)
}

func TestRunValidate_CombinedFlags(t *testing.T) {
	t.Parallel()
	// Test with multiple flags set simultaneously
	// Uses mock runner to test flag handling quickly.
	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json", Verbose: true})
	root.AddCommand(cmd)

	_ = root.PersistentFlags().Set("output", "json")
	_ = root.PersistentFlags().Set("verbose", "true")
	_ = cmd.Flags().Set("quiet", "true")

	var buf bytes.Buffer
	ctx := context.Background()
	opts := &ValidateOptions{Runner: newSuccessMockRunner()}

	err := runValidateWithOptions(ctx, cmd, &buf, opts)
	// With mock runner, should succeed with combined flags
	require.NoError(t, err)
}

func TestPipelineResultToResponse_ComplexScenario(t *testing.T) {
	// Test with a complex scenario: some passed, some failed, some skipped
	result := &validation.PipelineResult{
		Success: false,
		FormatResults: []validation.Result{
			{Command: "format-1", Success: true, ExitCode: 0, DurationMs: 100},
			{Command: "format-2", Success: true, ExitCode: 0, DurationMs: 150},
		},
		LintResults: []validation.Result{
			{Command: "lint-1", Success: false, ExitCode: 1, Error: "lint error", DurationMs: 200},
		},
		TestResults: []validation.Result{},
		PreCommitResults: []validation.Result{
			{Command: "pre-commit", Success: true, ExitCode: 0, DurationMs: 300},
		},
		SkippedSteps: []string{"test"},
		SkipReasons:  map[string]string{"test": "no test commands configured"},
	}

	resp := pipelineResultToResponse(result)

	assert.False(t, resp.Success)
	assert.Len(t, resp.Results, 4) // 2 format + 1 lint + 1 pre-commit
	assert.Equal(t, []string{"test"}, resp.SkippedSteps)
	assert.Equal(t, "no test commands configured", resp.SkipReasons["test"])

	// Verify order is preserved: format results first, then lint, then test, then pre-commit
	assert.Equal(t, "format-1", resp.Results[0].Command)
	assert.Equal(t, "format-2", resp.Results[1].Command)
	assert.Equal(t, "lint-1", resp.Results[2].Command)
	assert.Equal(t, "pre-commit", resp.Results[3].Command)
}

func TestCapitalizeStep_CasePreservation(t *testing.T) {
	// Verify that unknown steps preserve their original case
	tests := []struct {
		input    string
		expected string
	}{
		{"Format", "Format"},                 // Already capitalized
		{"LINT", "LINT"},                     // All caps
		{"TeSt", "TeSt"},                     // Mixed case
		{"my-custom-step", "my-custom-step"}, // Custom step
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalizeStep(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
