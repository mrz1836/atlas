package validation

import (
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/prompts"
)

// RetryContext holds context for AI-assisted retry.
// It captures information about failed validation steps to provide
// meaningful context to the AI for fixing issues.
type RetryContext struct {
	// FailedStep is which step failed (format, lint, test, pre-commit).
	FailedStep string

	// FailedCommands lists the commands that failed.
	FailedCommands []string

	// ErrorOutput is the combined error output (truncated if needed).
	ErrorOutput string

	// AttemptNumber is the current retry attempt (1-indexed).
	AttemptNumber int

	// MaxAttempts is the maximum allowed attempts.
	MaxAttempts int
}

// MaxErrorOutputLength limits error output included in AI retry prompts.
// This prevents exceeding AI model token limits while providing sufficient
// context for fixing issues. Typical validation error output is 500-2000 chars;
// 4000 provides a safety margin for complex multi-command failures.
const MaxErrorOutputLength = 4000

// ExtractErrorContext creates a RetryContext from a failed PipelineResult.
// It extracts failed commands, exit codes, and relevant error output.
func ExtractErrorContext(result *PipelineResult, attemptNum, maxAttempts int) *RetryContext {
	if result == nil {
		return &RetryContext{
			AttemptNumber: attemptNum,
			MaxAttempts:   maxAttempts,
		}
	}

	ctx := &RetryContext{
		FailedStep:    result.FailedStepName,
		AttemptNumber: attemptNum,
		MaxAttempts:   maxAttempts,
	}

	// Collect failed commands and their output
	var failedCommands []string
	var errorParts []string

	for _, r := range result.AllResults() {
		if !r.Success {
			failedCommands = append(failedCommands, r.Command)

			// Build error detail for this command
			var errDetail strings.Builder
			fmt.Fprintf(&errDetail, "Command: %s\n", r.Command)
			fmt.Fprintf(&errDetail, "Exit code: %d\n", r.ExitCode)

			if r.Stderr != "" {
				fmt.Fprintf(&errDetail, "Error output:\n%s", r.Stderr)
			} else if r.Stdout != "" {
				// Some tools output errors to stdout
				fmt.Fprintf(&errDetail, "Output:\n%s", r.Stdout)
			}

			if r.Error != "" {
				fmt.Fprintf(&errDetail, "\nError: %s", r.Error)
			}

			errorParts = append(errorParts, errDetail.String())
		}
	}

	ctx.FailedCommands = failedCommands
	ctx.ErrorOutput = truncateOutput(strings.Join(errorParts, "\n\n---\n\n"), MaxErrorOutputLength)

	return ctx
}

// BuildAIPrompt constructs the prompt for AI to fix validation errors.
// The prompt provides structured context about the failures to help the AI
// understand and fix the issues.
func BuildAIPrompt(ctx *RetryContext) string {
	if ctx == nil {
		return "Please fix the validation errors."
	}

	data := prompts.ValidationRetryData{
		FailedStep:     ctx.FailedStep,
		FailedCommands: ctx.FailedCommands,
		ErrorOutput:    ctx.ErrorOutput,
		AttemptNumber:  ctx.AttemptNumber,
		MaxAttempts:    ctx.MaxAttempts,
	}

	return prompts.MustRender(prompts.ValidationRetry, data)
}

// truncateOutput truncates a string to maxLen, adding an indicator if truncated.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	// Leave room for truncation message
	truncationMsg := "\n\n... (output truncated)"
	cutoffLen := maxLen - len(truncationMsg)
	if cutoffLen < 0 {
		cutoffLen = 0
	}

	return s[:cutoffLen] + truncationMsg
}
