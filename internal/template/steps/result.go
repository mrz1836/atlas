package steps

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// newStepResult creates a StepResult with the common fields populated.
// The returned result can be customized by setting additional fields
// like Output, Error, ArtifactPath, and Metadata.
func newStepResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, status string) *domain.StepResult {
	completedAt := time.Now()
	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      status,
		StartedAt:   startTime,
		CompletedAt: completedAt,
		DurationMs:  completedAt.Sub(startTime).Milliseconds(),
	}
}

// newSuccessResult creates a successful StepResult.
func newSuccessResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time) *domain.StepResult {
	return newStepResult(task, step, startTime, constants.StepStatusSuccess)
}

// newFailedResult creates a failed StepResult with an error message.
func newFailedResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time, errMsg string) *domain.StepResult {
	result := newStepResult(task, step, startTime, constants.StepStatusFailed)
	result.Error = errMsg
	return result
}

// newAwaitingApprovalResult creates a StepResult requiring user approval.
func newAwaitingApprovalResult(task *domain.Task, step *domain.StepDefinition, startTime time.Time) *domain.StepResult {
	return newStepResult(task, step, startTime, constants.StepStatusAwaitingApproval)
}
