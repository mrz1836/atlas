package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractErrorContext_CapturesFailedCommands(t *testing.T) {
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "lint",
		LintResults: []Result{
			{
				Command:  "golangci-lint run",
				Success:  false,
				ExitCode: 1,
				Stderr:   "error: undefined variable 'foo'",
			},
		},
	}

	ctx := ExtractErrorContext(result, 1, 3)

	assert.Equal(t, "lint", ctx.FailedStep)
	assert.Contains(t, ctx.FailedCommands, "golangci-lint run")
	assert.Contains(t, ctx.ErrorOutput, "undefined variable")
	assert.Equal(t, 1, ctx.AttemptNumber)
	assert.Equal(t, 3, ctx.MaxAttempts)
}

func TestExtractErrorContext_CapturesMultipleFailures(t *testing.T) {
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
		LintResults: []Result{
			{Command: "golangci-lint run", Success: true, ExitCode: 0},
		},
		TestResults: []Result{
			{
				Command:  "go test ./...",
				Success:  false,
				ExitCode: 1,
				Stderr:   "FAIL: TestFoo expected 1, got 2",
			},
			{
				Command:  "go test -race ./...",
				Success:  false,
				ExitCode: 1,
				Stderr:   "data race detected",
			},
		},
	}

	ctx := ExtractErrorContext(result, 2, 3)

	assert.Equal(t, "test", ctx.FailedStep)
	assert.Len(t, ctx.FailedCommands, 2)
	assert.Contains(t, ctx.FailedCommands, "go test ./...")
	assert.Contains(t, ctx.FailedCommands, "go test -race ./...")
	assert.Contains(t, ctx.ErrorOutput, "expected 1, got 2")
	assert.Contains(t, ctx.ErrorOutput, "data race detected")
	assert.Equal(t, 2, ctx.AttemptNumber)
}

func TestExtractErrorContext_NilResult(t *testing.T) {
	ctx := ExtractErrorContext(nil, 1, 3)

	assert.Empty(t, ctx.FailedStep)
	assert.Empty(t, ctx.FailedCommands)
	assert.Empty(t, ctx.ErrorOutput)
	assert.Equal(t, 1, ctx.AttemptNumber)
	assert.Equal(t, 3, ctx.MaxAttempts)
}

func TestExtractErrorContext_EmptyResult(t *testing.T) {
	result := &PipelineResult{
		Success: true,
	}

	ctx := ExtractErrorContext(result, 1, 3)

	assert.Empty(t, ctx.FailedStep)
	assert.Empty(t, ctx.FailedCommands)
	assert.Empty(t, ctx.ErrorOutput)
}

func TestExtractErrorContext_UsesStdoutIfNoStderr(t *testing.T) {
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "format",
		FormatResults: []Result{
			{
				Command:  "magex format:fix",
				Success:  false,
				ExitCode: 1,
				Stdout:   "formatting failed: invalid syntax",
				Stderr:   "",
			},
		},
	}

	ctx := ExtractErrorContext(result, 1, 3)

	assert.Contains(t, ctx.ErrorOutput, "formatting failed: invalid syntax")
}

func TestExtractErrorContext_IncludesErrorField(t *testing.T) {
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "pre-commit",
		PreCommitResults: []Result{
			{
				Command:  "go-pre-commit run --all-files",
				Success:  false,
				ExitCode: 1,
				Error:    "command timed out",
			},
		},
	}

	ctx := ExtractErrorContext(result, 1, 3)

	assert.Contains(t, ctx.ErrorOutput, "command timed out")
}

func TestBuildAIPrompt_IncludesAllContext(t *testing.T) {
	ctx := &RetryContext{
		FailedStep:     "test",
		FailedCommands: []string{"go test ./..."},
		ErrorOutput:    "FAIL: TestFoo expected 1, got 2",
		AttemptNumber:  2,
		MaxAttempts:    3,
	}

	prompt := BuildAIPrompt(ctx)

	assert.Contains(t, prompt, "step: test")
	assert.Contains(t, prompt, "go test ./...")
	assert.Contains(t, prompt, "expected 1, got 2")
	assert.Contains(t, prompt, "Attempt 2 of 3")
	assert.Contains(t, prompt, "Please analyze these errors")
	assert.Contains(t, prompt, "fix the issues")
}

func TestBuildAIPrompt_NilContext(t *testing.T) {
	prompt := BuildAIPrompt(nil)

	assert.Contains(t, prompt, "fix the validation errors")
}

func TestBuildAIPrompt_EmptyContext(t *testing.T) {
	ctx := &RetryContext{}

	prompt := BuildAIPrompt(ctx)

	assert.Contains(t, prompt, "Previous validation failed")
	assert.Contains(t, prompt, "Please analyze these errors")
}

func TestBuildAIPrompt_MultipleFailedCommands(t *testing.T) {
	ctx := &RetryContext{
		FailedStep:     "lint",
		FailedCommands: []string{"golangci-lint run", "staticcheck ./..."},
		ErrorOutput:    "lint errors",
		AttemptNumber:  1,
		MaxAttempts:    3,
	}

	prompt := BuildAIPrompt(ctx)

	assert.Contains(t, prompt, "golangci-lint run, staticcheck ./...")
}

func TestTruncateOutput_ShortString(t *testing.T) {
	input := "short string"
	result := truncateOutput(input, 100)
	assert.Equal(t, input, result)
}

func TestTruncateOutput_LongString(t *testing.T) {
	input := strings.Repeat("a", 5000)
	result := truncateOutput(input, 100)

	assert.LessOrEqual(t, len(result), 100)
	assert.Contains(t, result, "... (output truncated)")
}

func TestTruncateOutput_ExactLength(t *testing.T) {
	input := strings.Repeat("a", 100)
	result := truncateOutput(input, 100)
	assert.Equal(t, input, result)
}

func TestExtractErrorContext_TruncatesLongOutput(t *testing.T) {
	longError := strings.Repeat("x", 10000)
	result := &PipelineResult{
		Success:        false,
		FailedStepName: "test",
		TestResults: []Result{
			{
				Command:  "go test ./...",
				Success:  false,
				ExitCode: 1,
				Stderr:   longError,
			},
		},
	}

	ctx := ExtractErrorContext(result, 1, 3)

	require.LessOrEqual(t, len(ctx.ErrorOutput), MaxErrorOutputLength)
	assert.Contains(t, ctx.ErrorOutput, "... (output truncated)")
}
