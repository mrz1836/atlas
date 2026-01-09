package task

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestIsValidTransition_AllValidTransitions tests all valid transitions defined
// in the state machine. Each row in the transitions table is verified.
func TestIsValidTransition_AllValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from constants.TaskStatus
		to   constants.TaskStatus
	}{
		// From Pending
		{"pending to running", constants.TaskStatusPending, constants.TaskStatusRunning},

		// From Running
		{"running to validating", constants.TaskStatusRunning, constants.TaskStatusValidating},
		{"running to gh_failed", constants.TaskStatusRunning, constants.TaskStatusGHFailed},
		{"running to ci_failed", constants.TaskStatusRunning, constants.TaskStatusCIFailed},
		{"running to ci_timeout", constants.TaskStatusRunning, constants.TaskStatusCITimeout},
		{"running to abandoned", constants.TaskStatusRunning, constants.TaskStatusAbandoned},

		// From Validating
		{"validating to awaiting_approval", constants.TaskStatusValidating, constants.TaskStatusAwaitingApproval},
		{"validating to validation_failed", constants.TaskStatusValidating, constants.TaskStatusValidationFailed},

		// From ValidationFailed
		{"validation_failed to running", constants.TaskStatusValidationFailed, constants.TaskStatusRunning},
		{"validation_failed to abandoned", constants.TaskStatusValidationFailed, constants.TaskStatusAbandoned},

		// From AwaitingApproval
		{"awaiting_approval to completed", constants.TaskStatusAwaitingApproval, constants.TaskStatusCompleted},
		{"awaiting_approval to running", constants.TaskStatusAwaitingApproval, constants.TaskStatusRunning},
		{"awaiting_approval to rejected", constants.TaskStatusAwaitingApproval, constants.TaskStatusRejected},

		// From GHFailed
		{"gh_failed to running", constants.TaskStatusGHFailed, constants.TaskStatusRunning},
		{"gh_failed to abandoned", constants.TaskStatusGHFailed, constants.TaskStatusAbandoned},

		// From CIFailed
		{"ci_failed to running", constants.TaskStatusCIFailed, constants.TaskStatusRunning},
		{"ci_failed to abandoned", constants.TaskStatusCIFailed, constants.TaskStatusAbandoned},

		// From CITimeout
		{"ci_timeout to running", constants.TaskStatusCITimeout, constants.TaskStatusRunning},
		{"ci_timeout to abandoned", constants.TaskStatusCITimeout, constants.TaskStatusAbandoned},

		// From Running to Interrupted (user pressed Ctrl+C)
		{"running to interrupted", constants.TaskStatusRunning, constants.TaskStatusInterrupted},

		// From Validating to Interrupted (user pressed Ctrl+C)
		{"validating to interrupted", constants.TaskStatusValidating, constants.TaskStatusInterrupted},

		// From Interrupted
		{"interrupted to running", constants.TaskStatusInterrupted, constants.TaskStatusRunning},
		{"interrupted to abandoned", constants.TaskStatusInterrupted, constants.TaskStatusAbandoned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidTransition(tt.from, tt.to)
			assert.True(t, result, "transition from %s to %s should be valid", tt.from, tt.to)
		})
	}
}

// TestIsValidTransition_InvalidTransitions tests transitions that are NOT allowed.
func TestIsValidTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from constants.TaskStatus
		to   constants.TaskStatus
	}{
		// Cannot skip states
		{"pending to completed", constants.TaskStatusPending, constants.TaskStatusCompleted},
		{"pending to validating", constants.TaskStatusPending, constants.TaskStatusValidating},
		{"pending to awaiting_approval", constants.TaskStatusPending, constants.TaskStatusAwaitingApproval},

		// Terminal states cannot transition
		{"completed to running", constants.TaskStatusCompleted, constants.TaskStatusRunning},
		{"completed to pending", constants.TaskStatusCompleted, constants.TaskStatusPending},
		{"rejected to running", constants.TaskStatusRejected, constants.TaskStatusRunning},
		{"abandoned to running", constants.TaskStatusAbandoned, constants.TaskStatusRunning},

		// Running cannot go directly to most terminal states (abandoned is allowed with force)
		{"running to completed", constants.TaskStatusRunning, constants.TaskStatusCompleted},
		{"running to rejected", constants.TaskStatusRunning, constants.TaskStatusRejected},
		{"running to pending", constants.TaskStatusRunning, constants.TaskStatusPending},

		// Validating cannot go backwards or to wrong states
		{"validating to running", constants.TaskStatusValidating, constants.TaskStatusRunning},
		{"validating to pending", constants.TaskStatusValidating, constants.TaskStatusPending},
		{"validating to completed", constants.TaskStatusValidating, constants.TaskStatusCompleted},

		// Awaiting approval cannot go to error states or abandoned
		{"awaiting_approval to validation_failed", constants.TaskStatusAwaitingApproval, constants.TaskStatusValidationFailed},
		{"awaiting_approval to gh_failed", constants.TaskStatusAwaitingApproval, constants.TaskStatusGHFailed},
		{"awaiting_approval to pending", constants.TaskStatusAwaitingApproval, constants.TaskStatusPending},
		{"awaiting_approval to abandoned", constants.TaskStatusAwaitingApproval, constants.TaskStatusAbandoned},

		// Same status transitions (identity)
		{"pending to pending", constants.TaskStatusPending, constants.TaskStatusPending},
		{"running to running", constants.TaskStatusRunning, constants.TaskStatusRunning},
		{"validating to validating", constants.TaskStatusValidating, constants.TaskStatusValidating},
		{"completed to completed", constants.TaskStatusCompleted, constants.TaskStatusCompleted},

		// Error states cannot transition to wrong states
		{"validation_failed to completed", constants.TaskStatusValidationFailed, constants.TaskStatusCompleted},
		{"gh_failed to completed", constants.TaskStatusGHFailed, constants.TaskStatusCompleted},
		{"ci_failed to completed", constants.TaskStatusCIFailed, constants.TaskStatusCompleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidTransition(tt.from, tt.to)
			assert.False(t, result, "transition from %s to %s should be invalid", tt.from, tt.to)
		})
	}
}

// TestIsValidTransition_UnknownStatus tests behavior with unknown status values.
func TestIsValidTransition_UnknownStatus(t *testing.T) {
	unknownStatus := constants.TaskStatus("unknown_status")

	// Unknown as source should fail
	assert.False(t, IsValidTransition(unknownStatus, constants.TaskStatusRunning))

	// Unknown as target should fail
	assert.False(t, IsValidTransition(constants.TaskStatusPending, unknownStatus))
}

// TestIsTerminalStatus tests the IsTerminalStatus helper function.
func TestIsTerminalStatus(t *testing.T) {
	terminalStatuses := []constants.TaskStatus{
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	nonTerminalStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	for _, status := range terminalStatuses {
		t.Run(status.String()+"_is_terminal", func(t *testing.T) {
			assert.True(t, IsTerminalStatus(status), "%s should be terminal", status)
		})
	}

	for _, status := range nonTerminalStatuses {
		t.Run(status.String()+"_is_not_terminal", func(t *testing.T) {
			assert.False(t, IsTerminalStatus(status), "%s should not be terminal", status)
		})
	}

	// Unknown status should not be terminal
	assert.False(t, IsTerminalStatus(constants.TaskStatus("unknown")))
}

// TestIsErrorStatus tests the IsErrorStatus helper function.
func TestIsErrorStatus(t *testing.T) {
	errorStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	nonErrorStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range errorStatuses {
		t.Run(status.String()+"_is_error", func(t *testing.T) {
			assert.True(t, IsErrorStatus(status), "%s should be error status", status)
		})
	}

	for _, status := range nonErrorStatuses {
		t.Run(status.String()+"_is_not_error", func(t *testing.T) {
			assert.False(t, IsErrorStatus(status), "%s should not be error status", status)
		})
	}

	// Unknown status should not be error
	assert.False(t, IsErrorStatus(constants.TaskStatus("unknown")))
}

// TestCanRetry tests the CanRetry helper function.
func TestCanRetry(t *testing.T) {
	retryableStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	nonRetryableStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range retryableStatuses {
		t.Run(status.String()+"_can_retry", func(t *testing.T) {
			assert.True(t, CanRetry(status), "%s should be retryable", status)
		})
	}

	for _, status := range nonRetryableStatuses {
		t.Run(status.String()+"_cannot_retry", func(t *testing.T) {
			assert.False(t, CanRetry(status), "%s should not be retryable", status)
		})
	}
}

// TestCanAbandon tests the CanAbandon helper function.
func TestCanAbandon(t *testing.T) {
	abandonableStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	nonAbandonableStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned, // Already abandoned
	}

	for _, status := range abandonableStatuses {
		t.Run(status.String()+"_can_abandon", func(t *testing.T) {
			assert.True(t, CanAbandon(status), "%s should be abandonable", status)
		})
	}

	for _, status := range nonAbandonableStatuses {
		t.Run(status.String()+"_cannot_abandon", func(t *testing.T) {
			assert.False(t, CanAbandon(status), "%s should not be abandonable", status)
		})
	}
}

// TestCanForceAbandon tests the CanForceAbandon helper function.
// CanForceAbandon is more permissive than CanAbandon and includes Running status.
func TestCanForceAbandon(t *testing.T) {
	forceAbandonableStatuses := []constants.TaskStatus{
		constants.TaskStatusRunning, // Key difference from CanAbandon
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	nonForceAbandonableStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusValidating,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned, // Already abandoned
	}

	for _, status := range forceAbandonableStatuses {
		t.Run(status.String()+"_can_force_abandon", func(t *testing.T) {
			assert.True(t, CanForceAbandon(status), "%s should be force-abandonable", status)
		})
	}

	for _, status := range nonForceAbandonableStatuses {
		t.Run(status.String()+"_cannot_force_abandon", func(t *testing.T) {
			assert.False(t, CanForceAbandon(status), "%s should not be force-abandonable", status)
		})
	}
}

// TestCanForceAbandon_IncludesRunning verifies that CanForceAbandon allows Running
// while CanAbandon does not.
func TestCanForceAbandon_IncludesRunning(t *testing.T) {
	assert.False(t, CanAbandon(constants.TaskStatusRunning),
		"CanAbandon should return false for Running")
	assert.True(t, CanForceAbandon(constants.TaskStatusRunning),
		"CanForceAbandon should return true for Running")
}

// TestGetValidTargetStatuses tests the GetValidTargetStatuses helper function.
func TestGetValidTargetStatuses(t *testing.T) {
	tests := []struct {
		name     string
		from     constants.TaskStatus
		expected []constants.TaskStatus
	}{
		{
			name:     "pending targets",
			from:     constants.TaskStatusPending,
			expected: []constants.TaskStatus{constants.TaskStatusRunning},
		},
		{
			name: "running targets",
			from: constants.TaskStatusRunning,
			expected: []constants.TaskStatus{
				constants.TaskStatusValidating,
				constants.TaskStatusGHFailed,
				constants.TaskStatusCIFailed,
				constants.TaskStatusCITimeout,
				constants.TaskStatusInterrupted,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "validating targets",
			from: constants.TaskStatusValidating,
			expected: []constants.TaskStatus{
				constants.TaskStatusAwaitingApproval,
				constants.TaskStatusValidationFailed,
				constants.TaskStatusInterrupted,
			},
		},
		{
			name: "validation_failed targets",
			from: constants.TaskStatusValidationFailed,
			expected: []constants.TaskStatus{
				constants.TaskStatusRunning,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "gh_failed targets",
			from: constants.TaskStatusGHFailed,
			expected: []constants.TaskStatus{
				constants.TaskStatusRunning,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "ci_failed targets",
			from: constants.TaskStatusCIFailed,
			expected: []constants.TaskStatus{
				constants.TaskStatusRunning,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "ci_timeout targets",
			from: constants.TaskStatusCITimeout,
			expected: []constants.TaskStatus{
				constants.TaskStatusRunning,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "interrupted targets",
			from: constants.TaskStatusInterrupted,
			expected: []constants.TaskStatus{
				constants.TaskStatusRunning,
				constants.TaskStatusAbandoned,
			},
		},
		{
			name: "awaiting_approval targets",
			from: constants.TaskStatusAwaitingApproval,
			expected: []constants.TaskStatus{
				constants.TaskStatusCompleted,
				constants.TaskStatusRunning,
				constants.TaskStatusRejected,
			},
		},
		{
			name:     "completed targets (terminal)",
			from:     constants.TaskStatusCompleted,
			expected: nil,
		},
		{
			name:     "rejected targets (terminal)",
			from:     constants.TaskStatusRejected,
			expected: nil,
		},
		{
			name:     "abandoned targets (terminal)",
			from:     constants.TaskStatusAbandoned,
			expected: nil,
		},
		{
			name:     "unknown status targets",
			from:     constants.TaskStatus("unknown"),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetValidTargetStatuses(tt.from)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetValidTargetStatuses_ReturnsCopy verifies that the returned slice is a copy.
func TestGetValidTargetStatuses_ReturnsCopy(t *testing.T) {
	targets1 := GetValidTargetStatuses(constants.TaskStatusPending)
	targets2 := GetValidTargetStatuses(constants.TaskStatusPending)

	require.NotNil(t, targets1)
	require.NotNil(t, targets2)

	// Modify the first result
	targets1[0] = constants.TaskStatusCompleted

	// Second result should be unaffected
	assert.Equal(t, constants.TaskStatusRunning, targets2[0])

	// Original ValidTransitions should be unaffected
	assert.Equal(t, constants.TaskStatusRunning, ValidTransitions[constants.TaskStatusPending][0])
}

// TestTransition_ValidTransitions tests all valid transitions using the Transition function.
func TestTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name   string
		from   constants.TaskStatus
		to     constants.TaskStatus
		reason string
	}{
		{"pending to running", constants.TaskStatusPending, constants.TaskStatusRunning, "task started"},
		{"running to validating", constants.TaskStatusRunning, constants.TaskStatusValidating, "AI completed"},
		{"running to gh_failed", constants.TaskStatusRunning, constants.TaskStatusGHFailed, "GitHub API error"},
		{"running to ci_failed", constants.TaskStatusRunning, constants.TaskStatusCIFailed, "CI tests failed"},
		{"running to ci_timeout", constants.TaskStatusRunning, constants.TaskStatusCITimeout, "CI polling timeout"},
		{"validating to awaiting_approval", constants.TaskStatusValidating, constants.TaskStatusAwaitingApproval, "validation passed"},
		{"validating to validation_failed", constants.TaskStatusValidating, constants.TaskStatusValidationFailed, "lint failed"},
		{"validation_failed to running", constants.TaskStatusValidationFailed, constants.TaskStatusRunning, "retrying"},
		{"validation_failed to abandoned", constants.TaskStatusValidationFailed, constants.TaskStatusAbandoned, "user abandoned"},
		{"awaiting_approval to completed", constants.TaskStatusAwaitingApproval, constants.TaskStatusCompleted, "user approved"},
		{"awaiting_approval to running", constants.TaskStatusAwaitingApproval, constants.TaskStatusRunning, "user requested changes"},
		{"awaiting_approval to rejected", constants.TaskStatusAwaitingApproval, constants.TaskStatusRejected, "user rejected"},
		{"gh_failed to running", constants.TaskStatusGHFailed, constants.TaskStatusRunning, "retrying GitHub"},
		{"gh_failed to abandoned", constants.TaskStatusGHFailed, constants.TaskStatusAbandoned, "user abandoned"},
		{"ci_failed to running", constants.TaskStatusCIFailed, constants.TaskStatusRunning, "retrying CI"},
		{"ci_failed to abandoned", constants.TaskStatusCIFailed, constants.TaskStatusAbandoned, "user abandoned"},
		{"ci_timeout to running", constants.TaskStatusCITimeout, constants.TaskStatusRunning, "retrying CI"},
		{"ci_timeout to abandoned", constants.TaskStatusCITimeout, constants.TaskStatusAbandoned, "user abandoned"},
		// Interrupted status transitions
		{"running to interrupted", constants.TaskStatusRunning, constants.TaskStatusInterrupted, "user pressed Ctrl+C"},
		{"validating to interrupted", constants.TaskStatusValidating, constants.TaskStatusInterrupted, "user pressed Ctrl+C"},
		{"interrupted to running", constants.TaskStatusInterrupted, constants.TaskStatusRunning, "resuming task"},
		{"interrupted to abandoned", constants.TaskStatusInterrupted, constants.TaskStatusAbandoned, "user abandoned"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:        "task-00000000-0000-4000-8000-000000000000",
				Status:    tt.from,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			originalUpdatedAt := task.UpdatedAt

			err := Transition(context.Background(), task, tt.to, tt.reason)

			require.NoError(t, err)
			assert.Equal(t, tt.to, task.Status, "status should be updated")
			assert.Len(t, task.Transitions, 1, "should have one transition")
			assert.Equal(t, tt.from, task.Transitions[0].FromStatus)
			assert.Equal(t, tt.to, task.Transitions[0].ToStatus)
			assert.Equal(t, tt.reason, task.Transitions[0].Reason)
			assert.False(t, task.Transitions[0].Timestamp.IsZero())
			assert.True(t, task.UpdatedAt.After(originalUpdatedAt) || task.UpdatedAt.Equal(originalUpdatedAt))
		})
	}
}

// TestTransition_InvalidTransitions tests that invalid transitions return errors.
func TestTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from constants.TaskStatus
		to   constants.TaskStatus
	}{
		{"pending to completed", constants.TaskStatusPending, constants.TaskStatusCompleted},
		{"completed to running", constants.TaskStatusCompleted, constants.TaskStatusRunning},
		{"running to pending", constants.TaskStatusRunning, constants.TaskStatusPending},
		{"running to completed", constants.TaskStatusRunning, constants.TaskStatusCompleted},
		{"pending to pending", constants.TaskStatusPending, constants.TaskStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:     "task-00000000-0000-4000-8000-000000000000",
				Status: tt.from,
			}

			err := Transition(context.Background(), task, tt.to, "test")

			require.Error(t, err)
			require.ErrorIs(t, err, atlaserrors.ErrInvalidTransition,
				"error should be ErrInvalidTransition, got: %v", err)
			assert.Contains(t, err.Error(), tt.from.String())
			assert.Contains(t, err.Error(), tt.to.String())

			// Status should be unchanged
			assert.Equal(t, tt.from, task.Status, "status should not change on invalid transition")

			// No transition recorded
			assert.Empty(t, task.Transitions, "no transition should be recorded on failure")
		})
	}
}

// TestTransition_SetsCompletedAt tests that CompletedAt is set for terminal states.
func TestTransition_SetsCompletedAt(t *testing.T) {
	tests := []struct {
		name     string
		from     constants.TaskStatus
		terminal constants.TaskStatus
	}{
		{"completed", constants.TaskStatusAwaitingApproval, constants.TaskStatusCompleted},
		{"rejected", constants.TaskStatusAwaitingApproval, constants.TaskStatusRejected},
		{"abandoned from validation_failed", constants.TaskStatusValidationFailed, constants.TaskStatusAbandoned},
		{"abandoned from gh_failed", constants.TaskStatusGHFailed, constants.TaskStatusAbandoned},
		{"abandoned from ci_failed", constants.TaskStatusCIFailed, constants.TaskStatusAbandoned},
		{"abandoned from ci_timeout", constants.TaskStatusCITimeout, constants.TaskStatusAbandoned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:     "task-00000000-0000-4000-8000-000000000000",
				Status: tt.from,
			}

			require.Nil(t, task.CompletedAt, "CompletedAt should be nil before transition")

			err := Transition(context.Background(), task, tt.terminal, "test")

			require.NoError(t, err)
			require.NotNil(t, task.CompletedAt, "CompletedAt should be set for terminal state")
			assert.False(t, task.CompletedAt.IsZero())
		})
	}
}

// TestTransition_DoesNotSetCompletedAtForNonTerminal verifies CompletedAt is not set
// for non-terminal transitions.
func TestTransition_DoesNotSetCompletedAtForNonTerminal(t *testing.T) {
	tests := []struct {
		name string
		from constants.TaskStatus
		to   constants.TaskStatus
	}{
		{"pending to running", constants.TaskStatusPending, constants.TaskStatusRunning},
		{"running to validating", constants.TaskStatusRunning, constants.TaskStatusValidating},
		{"validating to awaiting_approval", constants.TaskStatusValidating, constants.TaskStatusAwaitingApproval},
		{"validation_failed to running", constants.TaskStatusValidationFailed, constants.TaskStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.Task{
				ID:     "task-00000000-0000-4000-8000-000000000000",
				Status: tt.from,
			}

			err := Transition(context.Background(), task, tt.to, "test")

			require.NoError(t, err)
			assert.Nil(t, task.CompletedAt, "CompletedAt should remain nil for non-terminal state")
		})
	}
}

// TestTransition_ContextCancellation tests that the function respects context cancellation.
func TestTransition_ContextCancellation(t *testing.T) {
	task := &domain.Task{
		ID:     "task-00000000-0000-4000-8000-000000000000",
		Status: constants.TaskStatusPending,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := Transition(ctx, task, constants.TaskStatusRunning, "test")

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, constants.TaskStatusPending, task.Status, "status should not change")
	assert.Empty(t, task.Transitions, "no transition should be recorded")
}

// TestTransition_ContextDeadlineExceeded tests deadline exceeded behavior.
func TestTransition_ContextDeadlineExceeded(t *testing.T) {
	task := &domain.Task{
		ID:     "task-00000000-0000-4000-8000-000000000000",
		Status: constants.TaskStatusPending,
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err := Transition(ctx, task, constants.TaskStatusRunning, "test")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestTransition_NilTask tests that nil task returns an error.
func TestTransition_NilTask(t *testing.T) {
	err := Transition(context.Background(), nil, constants.TaskStatusRunning, "test")

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrInvalidTransition)
	assert.Contains(t, err.Error(), "nil")
}

// TestTransition_EmptyReason tests that empty reason is allowed.
func TestTransition_EmptyReason(t *testing.T) {
	task := &domain.Task{
		ID:     "task-00000000-0000-4000-8000-000000000000",
		Status: constants.TaskStatusPending,
	}

	err := Transition(context.Background(), task, constants.TaskStatusRunning, "")

	require.NoError(t, err)
	assert.Equal(t, constants.TaskStatusRunning, task.Status)
	assert.Empty(t, task.Transitions[0].Reason)
}

// TestTransition_MultipleSequentialTransitions tests that multiple transitions
// are appended to the history.
func TestTransition_MultipleSequentialTransitions(t *testing.T) {
	task := &domain.Task{
		ID:     "task-00000000-0000-4000-8000-000000000000",
		Status: constants.TaskStatusPending,
	}

	transitions := []struct {
		to     constants.TaskStatus
		reason string
	}{
		{constants.TaskStatusRunning, "started"},
		{constants.TaskStatusValidating, "AI completed"},
		{constants.TaskStatusAwaitingApproval, "validation passed"},
		{constants.TaskStatusCompleted, "approved"},
	}

	ctx := context.Background()
	for i, tr := range transitions {
		err := Transition(ctx, task, tr.to, tr.reason)
		require.NoError(t, err, "transition %d should succeed", i)
	}

	// Verify all transitions are recorded
	assert.Len(t, task.Transitions, len(transitions))

	// Verify transition chain
	expectedChain := []struct {
		from   constants.TaskStatus
		to     constants.TaskStatus
		reason string
	}{
		{constants.TaskStatusPending, constants.TaskStatusRunning, "started"},
		{constants.TaskStatusRunning, constants.TaskStatusValidating, "AI completed"},
		{constants.TaskStatusValidating, constants.TaskStatusAwaitingApproval, "validation passed"},
		{constants.TaskStatusAwaitingApproval, constants.TaskStatusCompleted, "approved"},
	}

	for i, expected := range expectedChain {
		assert.Equal(t, expected.from, task.Transitions[i].FromStatus, "transition %d from", i)
		assert.Equal(t, expected.to, task.Transitions[i].ToStatus, "transition %d to", i)
		assert.Equal(t, expected.reason, task.Transitions[i].Reason, "transition %d reason", i)
	}

	// Final status should be completed
	assert.Equal(t, constants.TaskStatusCompleted, task.Status)
	assert.NotNil(t, task.CompletedAt)
}

// TestTransition_TransitionHistoryTimestamps verifies timestamps are recorded correctly.
func TestTransition_TransitionHistoryTimestamps(t *testing.T) {
	task := &domain.Task{
		ID:     "task-00000000-0000-4000-8000-000000000000",
		Status: constants.TaskStatusPending,
	}

	before := time.Now().UTC()
	err := Transition(context.Background(), task, constants.TaskStatusRunning, "test")
	after := time.Now().UTC()

	require.NoError(t, err)
	require.Len(t, task.Transitions, 1)

	timestamp := task.Transitions[0].Timestamp
	assert.False(t, timestamp.Before(before), "timestamp should be >= before")
	assert.False(t, timestamp.After(after), "timestamp should be <= after")

	// UpdatedAt should also be in range
	assert.False(t, task.UpdatedAt.Before(before), "UpdatedAt should be >= before")
}

// TestValidTransitions_Completeness verifies all expected statuses are in the map.
func TestValidTransitions_Completeness(t *testing.T) {
	expectedNonTerminalStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	for _, status := range expectedNonTerminalStatuses {
		t.Run(status.String()+"_has_transitions", func(t *testing.T) {
			targets, exists := ValidTransitions[status]
			assert.True(t, exists, "%s should be in ValidTransitions map", status)
			assert.NotEmpty(t, targets, "%s should have at least one valid target", status)
		})
	}

	// Terminal statuses should NOT be in the map
	terminalStatuses := []constants.TaskStatus{
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range terminalStatuses {
		t.Run(status.String()+"_has_no_transitions", func(t *testing.T) {
			_, exists := ValidTransitions[status]
			assert.False(t, exists, "%s should NOT be in ValidTransitions map", status)
		})
	}
}

// TestGeneratedStateMaps verifies that the auto-generated terminalStatuses and errorStatuses
// maps contain the expected values. This ensures the init() function correctly derives
// these maps from ValidTransitions.
func TestGeneratedStateMaps(t *testing.T) {
	t.Run("terminalStatuses contains expected statuses", func(t *testing.T) {
		expectedTerminal := []constants.TaskStatus{
			constants.TaskStatusCompleted,
			constants.TaskStatusRejected,
			constants.TaskStatusAbandoned,
		}

		// Verify expected statuses are present
		for _, status := range expectedTerminal {
			assert.True(t, terminalStatuses[status], "%s should be a terminal status", status)
		}

		// Verify count matches expectations
		assert.Len(t, terminalStatuses, len(expectedTerminal),
			"terminalStatuses should have exactly %d entries", len(expectedTerminal))
	})

	t.Run("errorStatuses contains expected statuses", func(t *testing.T) {
		expectedError := []constants.TaskStatus{
			constants.TaskStatusValidationFailed,
			constants.TaskStatusGHFailed,
			constants.TaskStatusCIFailed,
			constants.TaskStatusCITimeout,
			constants.TaskStatusInterrupted,
		}

		// Verify expected statuses are present
		for _, status := range expectedError {
			assert.True(t, errorStatuses[status], "%s should be an error status", status)
		}

		// Verify count matches expectations
		assert.Len(t, errorStatuses, len(expectedError),
			"errorStatuses should have exactly %d entries", len(expectedError))
	})

	t.Run("terminalStatuses and errorStatuses are disjoint", func(t *testing.T) {
		for status := range terminalStatuses {
			assert.False(t, errorStatuses[status],
				"%s cannot be both terminal and error status", status)
		}
	})

	t.Run("error statuses can transition to Running and Abandoned", func(t *testing.T) {
		for status := range errorStatuses {
			targets := ValidTransitions[status]
			require.NotNil(t, targets, "error status %s should have transitions", status)

			hasRunning := false
			hasAbandoned := false
			for _, target := range targets {
				if target == constants.TaskStatusRunning {
					hasRunning = true
				}
				if target == constants.TaskStatusAbandoned {
					hasAbandoned = true
				}
			}
			assert.True(t, hasRunning, "error status %s should be able to transition to Running", status)
			assert.True(t, hasAbandoned, "error status %s should be able to transition to Abandoned", status)
		}
	})
}
