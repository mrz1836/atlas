// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
)

// GitOperation defines the supported git operations.
type GitOperation string

// Git operation constants.
const (
	GitOpCommit      GitOperation = "commit"
	GitOpPush        GitOperation = "push"
	GitOpCreatePR    GitOperation = "create_pr"
	GitOpSmartCommit GitOperation = "smart_commit"
)

// GitExecutor handles git operations.
// This is a placeholder implementation for Epic 4.
// Full implementation will be added in Epic 6 when GitRunner is available.
type GitExecutor struct {
	workDir string
}

// NewGitExecutor creates a new git executor.
func NewGitExecutor(workDir string) *GitExecutor {
	return &GitExecutor{workDir: workDir}
}

// Execute runs a git operation.
// The operation type is read from step.Config["operation"].
// Supported operations: commit, push, create_pr, smart_commit
//
// This is a placeholder implementation. Full functionality
// will be added in Epic 6 when GitRunner is implemented.
func (e *GitExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
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
		Msg("executing git step")

	startTime := time.Now()

	// Get operation from step config
	operation := GitOpCommit // default
	if step.Config != nil {
		if op, ok := step.Config["operation"].(string); ok {
			operation = GitOperation(op)
		}
	}

	log.Debug().
		Str("operation", string(operation)).
		Str("work_dir", e.workDir).
		Msg("git operation")

	// Placeholder implementation - returns success with operation info
	var output string
	switch operation {
	case GitOpCommit:
		output = fmt.Sprintf("Git commit placeholder (work_dir: %s)", e.workDir)
	case GitOpPush:
		output = fmt.Sprintf("Git push placeholder (work_dir: %s)", e.workDir)
	case GitOpCreatePR:
		output = fmt.Sprintf("Git create_pr placeholder (work_dir: %s)", e.workDir)
	case GitOpSmartCommit:
		output = fmt.Sprintf("Git smart_commit placeholder (work_dir: %s)", e.workDir)
	default:
		output = fmt.Sprintf("Unknown git operation: %s", operation)
	}

	elapsed := time.Since(startTime)
	log.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("operation", string(operation)).
		Dur("duration_ms", elapsed).
		Msg("git step completed (placeholder)")

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output,
	}, nil
}

// Type returns the step type this executor handles.
func (e *GitExecutor) Type() domain.StepType {
	return domain.StepTypeGit
}
