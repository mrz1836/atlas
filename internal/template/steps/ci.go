// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Default CI polling configuration.
const (
	defaultPollInterval = 30 * time.Second
	defaultCITimeout    = 30 * time.Minute
)

// CIExecutor handles CI waiting steps.
// This is a placeholder implementation for Epic 4.
// Full implementation will be added in Epic 6 when GitHubRunner is available.
type CIExecutor struct{}

// NewCIExecutor creates a new CI executor.
func NewCIExecutor() *CIExecutor {
	return &CIExecutor{}
}

// Execute polls CI status until completion or timeout.
// Configuration from step.Config:
//   - poll_interval: time.Duration (default: 30s)
//   - timeout: time.Duration (default: 30m)
//
// This is a placeholder implementation. Full functionality
// will be added in Epic 6 when GitHubRunner is implemented.
func (e *CIExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing ci step")

	startTime := time.Now()

	// Get polling configuration from step config
	pollInterval := defaultPollInterval
	timeout := defaultCITimeout

	if step.Config != nil {
		if pi, ok := step.Config["poll_interval"].(time.Duration); ok {
			pollInterval = pi
		}
		if to, ok := step.Config["timeout"].(time.Duration); ok {
			timeout = to
		}
	}

	// Use step.Timeout if set
	if step.Timeout > 0 {
		timeout = step.Timeout
	}

	log.Debug().
		Dur("poll_interval", pollInterval).
		Dur("timeout", timeout).
		Msg("ci polling configuration")

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Placeholder: simulate CI polling
	// In the real implementation, this would:
	// 1. Get the current PR/branch from task metadata
	// 2. Poll GitHub API for CI workflow status
	// 3. Return success when all checks pass
	// 4. Return failure with details if any check fails

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	pollCount := 0
	maxPolls := 3 // Placeholder limit

	for {
		select {
		case <-timeoutCtx.Done():
			elapsed := time.Since(startTime)
			log.Warn().
				Str("task_id", task.ID).
				Str("step_name", step.Name).
				Int("poll_count", pollCount).
				Dur("duration_ms", elapsed).
				Msg("ci step timed out")

			return &domain.StepResult{
				StepIndex:   task.CurrentStep,
				StepName:    step.Name,
				Status:      "failed",
				StartedAt:   startTime,
				CompletedAt: time.Now(),
				DurationMs:  elapsed.Milliseconds(),
				Output:      fmt.Sprintf("CI polling timed out after %d polls", pollCount),
				Error:       "ci polling timeout",
			}, fmt.Errorf("%w: polling exceeded timeout", atlaserrors.ErrCITimeout)

		case <-ticker.C:
			pollCount++
			log.Debug().
				Int("poll_count", pollCount).
				Msg("polling ci status")

			// Placeholder: simulate successful completion after maxPolls
			if pollCount >= maxPolls {
				elapsed := time.Since(startTime)
				log.Info().
					Str("task_id", task.ID).
					Str("step_name", step.Name).
					Int("poll_count", pollCount).
					Dur("duration_ms", elapsed).
					Msg("ci step completed (placeholder)")

				return &domain.StepResult{
					StepIndex:   task.CurrentStep,
					StepName:    step.Name,
					Status:      "success",
					StartedAt:   startTime,
					CompletedAt: time.Now(),
					DurationMs:  elapsed.Milliseconds(),
					Output:      fmt.Sprintf("CI completed successfully after %d polls (placeholder)", pollCount),
				}, nil
			}
		}
	}
}

// Type returns the step type this executor handles.
func (e *CIExecutor) Type() domain.StepType {
	return domain.StepTypeCI
}
