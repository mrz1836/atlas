package contracts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestIsAttentionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   constants.TaskStatus
		expected bool
	}{
		// Statuses that require attention (should return true)
		{
			name:     "validation_failed requires attention",
			status:   constants.TaskStatusValidationFailed,
			expected: true,
		},
		{
			name:     "awaiting_approval requires attention",
			status:   constants.TaskStatusAwaitingApproval,
			expected: true,
		},
		{
			name:     "gh_failed requires attention",
			status:   constants.TaskStatusGHFailed,
			expected: true,
		},
		{
			name:     "ci_failed requires attention",
			status:   constants.TaskStatusCIFailed,
			expected: true,
		},
		{
			name:     "ci_timeout requires attention",
			status:   constants.TaskStatusCITimeout,
			expected: true,
		},

		// Statuses that do NOT require attention (should return false)
		{
			name:     "pending does not require attention",
			status:   constants.TaskStatusPending,
			expected: false,
		},
		{
			name:     "running does not require attention",
			status:   constants.TaskStatusRunning,
			expected: false,
		},
		{
			name:     "validating does not require attention",
			status:   constants.TaskStatusValidating,
			expected: false,
		},
		{
			name:     "completed does not require attention",
			status:   constants.TaskStatusCompleted,
			expected: false,
		},
		{
			name:     "rejected does not require attention",
			status:   constants.TaskStatusRejected,
			expected: false,
		},
		{
			name:     "abandoned does not require attention",
			status:   constants.TaskStatusAbandoned,
			expected: false,
		},
		{
			name:     "interrupted does not require attention",
			status:   constants.TaskStatusInterrupted,
			expected: false,
		},

		// Edge case: unknown/invalid status
		{
			name:     "unknown status does not require attention",
			status:   constants.TaskStatus("unknown_status"),
			expected: false,
		},
		{
			name:     "empty status does not require attention",
			status:   constants.TaskStatus(""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAttentionStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsAttentionStatus_AllDefinedStatuses verifies that all task statuses
// defined in constants are handled by IsAttentionStatus.
// This test serves as documentation of which statuses require attention.
func TestIsAttentionStatus_AllDefinedStatuses(t *testing.T) {
	allStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusInterrupted,
	}

	expectedAttentionStatuses := map[constants.TaskStatus]bool{
		constants.TaskStatusValidationFailed: true,
		constants.TaskStatusAwaitingApproval: true,
		constants.TaskStatusGHFailed:         true,
		constants.TaskStatusCIFailed:         true,
		constants.TaskStatusCITimeout:        true,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			expectedAttention := expectedAttentionStatuses[status]
			actualAttention := IsAttentionStatus(status)
			assert.Equal(t, expectedAttention, actualAttention,
				"Status %s should return %v for IsAttentionStatus", status, expectedAttention)
		})
	}
}

// TestIsAttentionStatus_Consistency verifies that the attentionStatuses map
// is consistent with the IsAttentionStatus function behavior.
func TestIsAttentionStatus_Consistency(t *testing.T) {
	// Verify all statuses in the map return true
	for status := range attentionStatuses {
		t.Run("map_entry_"+string(status), func(t *testing.T) {
			assert.True(t, IsAttentionStatus(status),
				"Status %s is in attentionStatuses map but IsAttentionStatus returns false", status)
		})
	}

	// Verify the map has exactly 5 entries (the attention statuses)
	assert.Len(t, attentionStatuses, 5,
		"attentionStatuses map should contain exactly 5 entries")
}
