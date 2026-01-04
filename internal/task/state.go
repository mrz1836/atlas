// Package task provides task lifecycle management for ATLAS.
//
// This file implements the task state machine, which enforces valid state
// transitions and maintains an audit trail of all status changes.
//
// Import rules:
//   - CAN import: internal/constants, internal/domain, internal/errors, std lib
//   - MUST NOT import: internal/workspace, internal/ai, internal/cli
package task

import (
	"context"
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ValidTransitions defines all allowed state transitions in the task lifecycle.
// Format: from_status -> []to_statuses
//
// The state machine follows this flow:
//
//	Pending → Running
//	Running → Validating, GHFailed, CIFailed, CITimeout, Abandoned
//	Validating → AwaitingApproval, ValidationFailed
//	ValidationFailed → Running, Abandoned
//	AwaitingApproval → Completed, Running, Rejected
//	GHFailed → Running, Abandoned
//	CIFailed → Running, Abandoned
//	CITimeout → Running, Abandoned
//
//nolint:gochecknoglobals // Exported for testing and read-only lookup table
var ValidTransitions = map[constants.TaskStatus][]constants.TaskStatus{
	constants.TaskStatusPending: {constants.TaskStatusRunning},
	constants.TaskStatusRunning: {
		constants.TaskStatusValidating,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusAbandoned, // Allow force-abandon
	},
	constants.TaskStatusValidating:       {constants.TaskStatusAwaitingApproval, constants.TaskStatusValidationFailed},
	constants.TaskStatusValidationFailed: {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
	constants.TaskStatusAwaitingApproval: {constants.TaskStatusCompleted, constants.TaskStatusRunning, constants.TaskStatusRejected},
	constants.TaskStatusGHFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
	constants.TaskStatusCIFailed:         {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
	constants.TaskStatusCITimeout:        {constants.TaskStatusRunning, constants.TaskStatusAbandoned},
}

// terminalStatuses defines states where no further transitions are allowed.
// These are intentionally duplicated from ValidTransitions for O(1) lookup performance.
// Terminal states are those NOT present as keys in ValidTransitions.
// MAINTENANCE: When adding new statuses, update both ValidTransitions and this map.
//
//nolint:gochecknoglobals // Read-only lookup table for terminal state checks
var terminalStatuses = map[constants.TaskStatus]bool{
	constants.TaskStatusCompleted: true,
	constants.TaskStatusRejected:  true,
	constants.TaskStatusAbandoned: true,
}

// errorStatuses defines states that indicate a recoverable error condition.
// These states can transition to Running (retry) or Abandoned (give up).
// Intentionally duplicated from ValidTransitions for O(1) lookup performance.
// MAINTENANCE: When adding new error statuses, update both ValidTransitions and this map.
//
//nolint:gochecknoglobals // Read-only lookup table for error state checks
var errorStatuses = map[constants.TaskStatus]bool{
	constants.TaskStatusValidationFailed: true,
	constants.TaskStatusGHFailed:         true,
	constants.TaskStatusCIFailed:         true,
	constants.TaskStatusCITimeout:        true,
}

// IsValidTransition checks if a transition from one status to another is allowed.
// Returns false for transitions from terminal states or to the same state.
func IsValidTransition(from, to constants.TaskStatus) bool {
	// Same status is not a valid transition
	if from == to {
		return false
	}

	validTargets, exists := ValidTransitions[from]
	if !exists {
		return false // Terminal state or unknown state
	}
	for _, target := range validTargets {
		if target == to {
			return true
		}
	}
	return false
}

// IsTerminalStatus returns true for states where no further transitions are allowed.
// Terminal states: Completed, Rejected, Abandoned
func IsTerminalStatus(status constants.TaskStatus) bool {
	return terminalStatuses[status]
}

// IsErrorStatus returns true for states that indicate an error condition.
// Error states: ValidationFailed, GHFailed, CIFailed, CITimeout
func IsErrorStatus(status constants.TaskStatus) bool {
	return errorStatuses[status]
}

// CanRetry returns true for error states that can transition back to Running.
// This allows retrying failed operations after addressing the underlying issue.
// AwaitingApproval can also transition to Running (for "requested changes"),
// but that is not considered a "retry" - it's a deliberate workflow choice.
func CanRetry(status constants.TaskStatus) bool {
	return errorStatuses[status]
}

// CanAbandon returns true for states that can transition to Abandoned.
// This includes all error states that support the abandon path.
// Running status is NOT included here - use CanForceAbandon for that.
func CanAbandon(status constants.TaskStatus) bool {
	// Only allow abandoning error states (not Running)
	return errorStatuses[status]
}

// CanForceAbandon returns true for states that can be force-abandoned.
// This is more permissive than CanAbandon and includes Running status.
// Use this when the --force flag is provided to allow abandoning running tasks.
func CanForceAbandon(status constants.TaskStatus) bool {
	// Check if Abandoned is a valid target from this status
	validTargets, exists := ValidTransitions[status]
	if !exists {
		return false
	}
	for _, target := range validTargets {
		if target == constants.TaskStatusAbandoned {
			return true
		}
	}
	return false
}

// GetValidTargetStatuses returns all valid target statuses for a given status.
// Returns nil for terminal states or unknown statuses.
func GetValidTargetStatuses(from constants.TaskStatus) []constants.TaskStatus {
	targets, exists := ValidTransitions[from]
	if !exists {
		return nil
	}
	// Return a copy to prevent modification of the original slice
	result := make([]constants.TaskStatus, len(targets))
	copy(result, targets)
	return result
}

// Transition validates and applies a state transition to the task.
// It records the transition in the task's history and updates timestamps.
// The caller is responsible for persisting the updated task.
//
// Parameters:
//   - ctx: Context for cancellation support
//   - task: The task to transition (modified in place)
//   - to: The target status
//   - reason: Optional explanation for the transition (can be empty)
//
// Returns an error if:
//   - ctx is canceled
//   - task is nil
//   - The transition is invalid (returns wrapped ErrInvalidTransition)
func Transition(ctx context.Context, task *domain.Task, to constants.TaskStatus, reason string) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate task is not nil
	if task == nil {
		return fmt.Errorf("%w: task is nil", atlaserrors.ErrInvalidTransition)
	}

	from := task.Status

	// Validate transition
	if !IsValidTransition(from, to) {
		return fmt.Errorf("%w: cannot transition from %s to %s",
			atlaserrors.ErrInvalidTransition, from, to)
	}

	now := time.Now().UTC()

	// Record transition in history
	transition := domain.Transition{
		FromStatus: from,
		ToStatus:   to,
		Timestamp:  now,
		Reason:     reason,
	}
	task.Transitions = append(task.Transitions, transition)

	// Update task status
	task.Status = to
	task.UpdatedAt = now

	// Set CompletedAt for terminal states
	if IsTerminalStatus(to) {
		task.CompletedAt = &now
	}

	return nil
}
