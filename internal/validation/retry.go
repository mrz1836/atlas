package validation

import (
	"fmt"
	"strings"
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

// MaxErrorOutputLength is the maximum length for error output in the prompt.
// This prevents overly long prompts while providing sufficient context.
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
			errDetail.WriteString(fmt.Sprintf("Command: %s\n", r.Command))
			errDetail.WriteString(fmt.Sprintf("Exit code: %d\n", r.ExitCode))

			if r.Stderr != "" {
				errDetail.WriteString(fmt.Sprintf("Error output:\n%s", r.Stderr))
			} else if r.Stdout != "" {
				// Some tools output errors to stdout
				errDetail.WriteString(fmt.Sprintf("Output:\n%s", r.Stdout))
			}

			if r.Error != "" {
				errDetail.WriteString(fmt.Sprintf("\nError: %s", r.Error))
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

	var prompt strings.Builder

	prompt.WriteString("Previous validation failed")
	if ctx.FailedStep != "" {
		prompt.WriteString(fmt.Sprintf(" at step: %s", ctx.FailedStep))
	}
	prompt.WriteString("\n\n")

	if len(ctx.FailedCommands) > 0 {
		prompt.WriteString(fmt.Sprintf("Failed commands: %s\n\n", strings.Join(ctx.FailedCommands, ", ")))
	}

	if ctx.ErrorOutput != "" {
		prompt.WriteString("Error output:\n")
		prompt.WriteString(ctx.ErrorOutput)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("Please analyze these errors and fix the issues in the code. Focus on:\n")
	prompt.WriteString("1. Fixing the specific errors shown above\n")
	prompt.WriteString("2. Not introducing new issues\n")
	prompt.WriteString("3. Following project conventions\n")

	if ctx.AttemptNumber > 0 && ctx.MaxAttempts > 0 {
		prompt.WriteString(fmt.Sprintf("\nAttempt %d of %d.", ctx.AttemptNumber, ctx.MaxAttempts))
	}

	return prompt.String()
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
