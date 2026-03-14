package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatResult_Success(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:    true,
		DurationMs: 5432,
	}

	output := FormatResult(result)

	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "All validations passed")
	assert.Contains(t, output, "5432")
}

func TestFormatResult_Failure_ShowsFailedStep(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		DurationMs:     1234,
	}

	output := FormatResult(result)

	assert.Contains(t, output, "✗")
	assert.Contains(t, output, "lint")
}

func TestFormatResult_Failure_ShowsFailedCommands(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		LintResults: []Result{
			{
				Command:  "golangci-lint run",
				Success:  false,
				ExitCode: 1,
				Stderr:   "error: undefined variable 'foo'",
				Stdout:   "Checking files...",
			},
		},
	}

	output := FormatResult(result)

	assert.Contains(t, output, "golangci-lint run")
	assert.Contains(t, output, "Exit code: 1")
	assert.Contains(t, output, "undefined variable 'foo'")
}

func TestFormatResult_Failure_PlainTextOutput(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
		TestResults: []Result{
			{Command: "go test", Success: false, ExitCode: 1},
		},
	}

	output := FormatResult(result)

	// Should show failure info in plain text (no markdown)
	assert.Contains(t, output, "Command: go test")
	assert.Contains(t, output, "Exit code: 1")
	// Should NOT contain markdown syntax
	assert.NotContains(t, output, "###")
	assert.NotContains(t, output, "**")
	assert.NotContains(t, output, "```")
}

func TestFormatResult_MultipleFailures(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		LintResults: []Result{
			{Command: "golangci-lint run", Success: false, ExitCode: 1, Stderr: "lint error 1"},
		},
		TestResults: []Result{
			{Command: "go test ./...", Success: false, ExitCode: 1, Stderr: "test error"},
		},
	}

	output := FormatResult(result)

	// Should show both failed commands
	assert.Contains(t, output, "golangci-lint run")
	assert.Contains(t, output, "lint error 1")
	assert.Contains(t, output, "go test")
	assert.Contains(t, output, "test error")
}

func TestFormatResult_TruncatesLongOutput(t *testing.T) {
	t.Parallel()

	longOutput := strings.Repeat("a", 2000)
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
		TestResults: []Result{
			{Command: "go test", Success: false, ExitCode: 1, Stdout: longOutput},
		},
	}

	output := FormatResult(result)

	// Should not include the full long output
	assert.NotContains(t, output, longOutput)
}

func TestFormatResult_TruncatesAtLineBoundary(t *testing.T) {
	t.Parallel()

	// Create output with clear line boundaries
	// Each line is ~50 chars, so 25 lines = ~1250 chars (over maxStdoutDisplay=1000)
	var lines []string
	for i := 0; i < 25; i++ {
		lines = append(lines, strings.Repeat("x", 45)+"-end")
	}
	longOutput := strings.Join(lines, "\n")

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
		TestResults: []Result{
			{Command: "go test", Success: false, ExitCode: 1, Stdout: longOutput},
		},
	}

	output := FormatResult(result)

	// Should show truncation indicator
	assert.Contains(t, output, "...[truncated]")

	// Should not contain partial lines (lines should end with "-end" or be fully truncated)
	// The key assertion: we should NOT see a line that's been cut in the middle
	// which would appear as a line without proper ending before the truncation
	lines = strings.Split(output, "\n")
	foundTruncated := false
	for i, line := range lines {
		if strings.Contains(line, "...[truncated]") {
			foundTruncated = true
			// The line before truncation should be a complete line (ends with "-end")
			// or be the truncation indicator itself
			if i > 0 {
				prevLine := strings.TrimSpace(lines[i-1])
				// Previous line should either be empty or end with "-end"
				// (allowing for the "  " indent prefix)
				if prevLine != "" && !strings.HasSuffix(prevLine, "-end") {
					t.Errorf("Line before truncation appears to be cut mid-line: %q", prevLine)
				}
			}
			break
		}
	}
	assert.True(t, foundTruncated, "Should contain truncation indicator")
}

func TestFormatResult_SkipsPassingCommands(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		FormatResults: []Result{
			{Command: "magex format", Success: true, ExitCode: 0},
		},
		LintResults: []Result{
			{Command: "golangci-lint run", Success: false, ExitCode: 1, Stderr: "error"},
		},
	}

	output := FormatResult(result)

	// Should NOT show the passing format command in detail
	assert.NotContains(t, output, "magex format")
	// Should show the failing lint command
	assert.Contains(t, output, "golangci-lint run")
}

func TestFormatResult_EmptyResults(t *testing.T) {
	t.Parallel()

	result := &PipelineResult{
		Success:        false,
		FailedStepName: "format",
	}

	output := FormatResult(result)

	// Should still show failure message
	assert.Contains(t, output, "✗")
	assert.Contains(t, output, "format")
}
