// Package task provides task execution and lifecycle management.
package task

import (
	"context"

	"github.com/mrz1836/atlas/internal/validation"
)

// ValidationRetryHandler defines the interface for AI-assisted validation retry.
// This interface is implemented by *validation.RetryHandler and allows the task
// engine to automatically retry failed validation steps using AI.
type ValidationRetryHandler interface {
	// RetryWithAI attempts to fix validation errors using AI.
	// It extracts error context, invokes AI to fix the issues, and re-runs validation.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - result: The failed PipelineResult to retry
	//   - workDir: Working directory for AI execution
	//   - attemptNum: Current attempt number (1-indexed)
	//   - runnerConfig: Configuration for the validation runner (may be nil for defaults)
	//
	// Returns:
	//   - RetryResult: Contains the retry outcome including new validation results
	//   - error: nil if validation passes after AI fix
	RetryWithAI(
		ctx context.Context,
		result *validation.PipelineResult,
		workDir string,
		attemptNum int,
		runnerConfig *validation.RunnerConfig,
	) (*validation.RetryResult, error)

	// CanRetry checks if another retry attempt is allowed.
	CanRetry(attemptNum int) bool

	// MaxAttempts returns the maximum retry attempts configured.
	MaxAttempts() int

	// IsEnabled returns whether AI retry is enabled.
	IsEnabled() bool
}
