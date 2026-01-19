package validation

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/contracts"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// RetryConfig holds retry configuration.
type RetryConfig struct {
	// MaxAttempts is the maximum number of AI retry attempts (default: 3).
	MaxAttempts int

	// Enabled indicates whether AI retry is enabled (default: true).
	Enabled bool
}

// DefaultRetryConfig returns sensible defaults for retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		Enabled:     true,
	}
}

// RetryHandler orchestrates validation retry with AI context.
// It extracts error context from failed validation, invokes AI to fix issues,
// and re-runs validation to verify the fix.
type RetryHandler struct {
	aiRunner contracts.AIRunner
	executor *Executor
	config   RetryConfig
	logger   zerolog.Logger
}

// NewRetryHandler creates a retry handler.
func NewRetryHandler(aiRunner contracts.AIRunner, executor *Executor, config RetryConfig, logger zerolog.Logger) *RetryHandler {
	return &RetryHandler{
		aiRunner: aiRunner,
		executor: executor,
		config:   config,
		logger:   logger,
	}
}

// NewRetryHandlerFromConfig creates a RetryHandler using application config.
// This is the recommended way to create a RetryHandler for CLI usage.
func NewRetryHandlerFromConfig(aiRunner contracts.AIRunner, executor *Executor, enabled bool, maxAttempts int, logger zerolog.Logger) *RetryHandler {
	config := RetryConfigFromAppConfig(enabled, maxAttempts)
	return NewRetryHandler(aiRunner, executor, config, logger)
}

// RetryResult contains the outcome of a retry attempt.
type RetryResult struct {
	// Success indicates whether the retry fixed the validation issues.
	Success bool

	// AttemptNumber is which attempt this result is from.
	AttemptNumber int

	// PipelineResult contains the validation results after AI fix.
	PipelineResult *PipelineResult

	// AIResult contains the result from the AI execution.
	AIResult *domain.AIResult
}

// RetryWithAI attempts to fix validation errors using AI.
// It extracts error context, invokes AI to fix, and re-runs validation.
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - result: The failed PipelineResult to retry
//   - workDir: Working directory for AI execution
//   - attemptNum: Current attempt number (1-indexed)
//   - runnerConfig: Configuration for the validation runner (may be nil for defaults)
//   - agent: The AI agent to use (claude, gemini, codex)
//   - model: The specific model to use
//
// Returns:
//   - RetryResult: Contains the retry outcome including new validation results
//   - error: nil if validation passes after AI fix,
//     ErrValidationFailed if validation still fails,
//     ErrMaxRetriesExceeded if max attempts reached,
//     ErrRetryDisabled if retry is disabled
func (h *RetryHandler) RetryWithAI(
	ctx context.Context,
	result *PipelineResult,
	workDir string,
	attemptNum int,
	runnerConfig *RunnerConfig,
	agent domain.Agent,
	model string,
) (*RetryResult, error) {
	// Check if retry is enabled
	if !h.config.Enabled {
		h.logger.Warn().Msg("AI retry is disabled")
		return nil, atlaserrors.ErrRetryDisabled
	}

	// Check if max attempts exceeded
	if attemptNum > h.config.MaxAttempts {
		h.logger.Warn().
			Int("attempt", attemptNum).
			Int("max_attempts", h.config.MaxAttempts).
			Msg("maximum retry attempts exceeded")
		return nil, fmt.Errorf("%w: attempt %d exceeds max %d",
			atlaserrors.ErrMaxRetriesExceeded, attemptNum, h.config.MaxAttempts)
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Pre-flight check: verify workDir exists before attempting AI retry
	// This prevents wasteful AI invocations when the worktree has been deleted
	if workDir == "" {
		h.logger.Error().Msg("work directory is empty for AI retry")
		return nil, fmt.Errorf("work directory is empty: %w", atlaserrors.ErrWorktreeNotFound)
	}
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		h.logger.Error().
			Str("work_dir", workDir).
			Msg("CRITICAL: worktree directory missing before AI retry")
		return nil, fmt.Errorf("worktree directory missing: %s: %w", workDir, atlaserrors.ErrWorktreeNotFound)
	}

	h.logger.Info().
		Str("agent", string(agent)).
		Str("model", model).
		Int("attempt", attemptNum).
		Int("max_attempts", h.config.MaxAttempts).
		Str("failed_step", result.FailedStepName).
		Msg("starting AI-assisted validation retry")

	// Extract error context from the failed result
	retryCtx := ExtractErrorContext(result, attemptNum, h.config.MaxAttempts)

	// Build AI prompt with error context
	prompt := BuildAIPrompt(retryCtx)

	h.logger.Debug().
		Str("failed_step", retryCtx.FailedStep).
		Int("failed_commands_count", len(retryCtx.FailedCommands)).
		Msg("extracted error context for AI")

	// Invoke AI to fix the issues
	aiReq := &domain.AIRequest{
		Agent:      agent,
		Model:      model,
		Prompt:     prompt,
		WorkingDir: workDir,
	}

	h.logger.Debug().
		Str("work_dir", workDir).
		Msg("invoking AI for fix")

	aiResult, err := h.aiRunner.Run(ctx, aiReq)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("agent", string(agent)).
			Str("model", model).
			Msg("AI fix invocation failed")
		return nil, fmt.Errorf("AI fix failed: %w", err)
	}

	h.logger.Info().
		Str("agent", string(agent)).
		Str("model", model).
		Bool("ai_success", aiResult.Success).
		Int("files_changed", len(aiResult.FilesChanged)).
		Msg("AI fix completed, re-running validation")

	// Check context cancellation before re-running validation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Re-run validation using existing Runner
	runner := NewRunner(h.executor, runnerConfig)
	newResult, runErr := runner.Run(ctx, workDir)

	retryResult := &RetryResult{
		AttemptNumber:  attemptNum,
		PipelineResult: newResult,
		AIResult:       aiResult,
	}

	if runErr != nil {
		h.logger.Warn().
			Str("agent", string(agent)).
			Str("model", model).
			Int("attempt", attemptNum).
			Str("failed_step", newResult.FailedStepName).
			Msg("validation still fails after AI fix")

		retryResult.Success = false
		return retryResult, fmt.Errorf("%w: %s (attempt %d)",
			atlaserrors.ErrValidationFailed, newResult.FailedStepName, attemptNum)
	}

	h.logger.Info().
		Str("agent", string(agent)).
		Str("model", model).
		Int("attempt", attemptNum).
		Int64("duration_ms", newResult.DurationMs).
		Msg("validation passed after AI fix")

	retryResult.Success = true
	return retryResult, nil
}

// CanRetry checks if another retry attempt is allowed.
func (h *RetryHandler) CanRetry(attemptNum int) bool {
	return h.config.Enabled && attemptNum <= h.config.MaxAttempts
}

// MaxAttempts returns the maximum retry attempts configured.
func (h *RetryHandler) MaxAttempts() int {
	return h.config.MaxAttempts
}

// IsEnabled returns whether AI retry is enabled.
func (h *RetryHandler) IsEnabled() bool {
	return h.config.Enabled
}

// RetryConfigFromAppConfig creates a RetryConfig from application config values.
// This is a convenience function for integrating with the config package.
func RetryConfigFromAppConfig(enabled bool, maxAttempts int) RetryConfig {
	cfg := RetryConfig{
		Enabled:     enabled,
		MaxAttempts: maxAttempts,
	}
	// Apply sensible defaults if values are invalid
	if cfg.MaxAttempts <= 0 && cfg.Enabled {
		cfg.MaxAttempts = 3
	}
	return cfg
}
