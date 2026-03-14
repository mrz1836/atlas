package constants

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   TaskStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   TaskStatusPending,
			expected: "pending",
		},
		{
			name:     "running status",
			status:   TaskStatusRunning,
			expected: "running",
		},
		{
			name:     "validating status",
			status:   TaskStatusValidating,
			expected: "validating",
		},
		{
			name:     "validation_failed status",
			status:   TaskStatusValidationFailed,
			expected: "validation_failed",
		},
		{
			name:     "awaiting_approval status",
			status:   TaskStatusAwaitingApproval,
			expected: "awaiting_approval",
		},
		{
			name:     "completed status",
			status:   TaskStatusCompleted,
			expected: "completed",
		},
		{
			name:     "rejected status",
			status:   TaskStatusRejected,
			expected: "rejected",
		},
		{
			name:     "abandoned status",
			status:   TaskStatusAbandoned,
			expected: "abandoned",
		},
		{
			name:     "gh_failed status",
			status:   TaskStatusGHFailed,
			expected: "gh_failed",
		},
		{
			name:     "ci_failed status",
			status:   TaskStatusCIFailed,
			expected: "ci_failed",
		},
		{
			name:     "ci_timeout status",
			status:   TaskStatusCITimeout,
			expected: "ci_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestWorkspaceStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   WorkspaceStatus
		expected string
	}{
		{
			name:     "active status",
			status:   WorkspaceStatusActive,
			expected: "active",
		},
		{
			name:     "paused status",
			status:   WorkspaceStatusPaused,
			expected: "paused",
		},
		{
			name:     "closed status",
			status:   WorkspaceStatusClosed,
			expected: "closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestTaskStatus_JSONSerialization(t *testing.T) {
	type wrapper struct {
		Status TaskStatus `json:"status"`
	}

	tests := []struct {
		name         string
		status       TaskStatus
		expectedJSON string
	}{
		{
			name:         "pending serializes to snake_case",
			status:       TaskStatusPending,
			expectedJSON: `{"status":"pending"}`,
		},
		{
			name:         "validation_failed serializes with underscore",
			status:       TaskStatusValidationFailed,
			expectedJSON: `{"status":"validation_failed"}`,
		},
		{
			name:         "awaiting_approval serializes with underscore",
			status:       TaskStatusAwaitingApproval,
			expectedJSON: `{"status":"awaiting_approval"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := wrapper{Status: tt.status}
			data, err := json.Marshal(w)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedJSON, string(data))
		})
	}
}

func TestWorkspaceStatus_JSONSerialization(t *testing.T) {
	type wrapper struct {
		Status WorkspaceStatus `json:"status"`
	}

	tests := []struct {
		name         string
		status       WorkspaceStatus
		expectedJSON string
	}{
		{
			name:         "active serializes correctly",
			status:       WorkspaceStatusActive,
			expectedJSON: `{"status":"active"}`,
		},
		{
			name:         "paused serializes correctly",
			status:       WorkspaceStatusPaused,
			expectedJSON: `{"status":"paused"}`,
		},
		{
			name:         "closed serializes correctly",
			status:       WorkspaceStatusClosed,
			expectedJSON: `{"status":"closed"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := wrapper{Status: tt.status}
			data, err := json.Marshal(w)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedJSON, string(data))
		})
	}
}

func TestTaskStatus_JSONDeserialization(t *testing.T) {
	type wrapper struct {
		Status TaskStatus `json:"status"`
	}

	tests := []struct {
		name           string
		jsonInput      string
		expectedStatus TaskStatus
	}{
		{
			name:           "deserialize pending",
			jsonInput:      `{"status":"pending"}`,
			expectedStatus: TaskStatusPending,
		},
		{
			name:           "deserialize validation_failed",
			jsonInput:      `{"status":"validation_failed"}`,
			expectedStatus: TaskStatusValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w wrapper
			err := json.Unmarshal([]byte(tt.jsonInput), &w)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, w.Status)
		})
	}
}

func TestWorkspaceStatus_JSONDeserialization(t *testing.T) {
	type wrapper struct {
		Status WorkspaceStatus `json:"status"`
	}

	tests := []struct {
		name           string
		jsonInput      string
		expectedStatus WorkspaceStatus
	}{
		{
			name:           "deserialize active",
			jsonInput:      `{"status":"active"}`,
			expectedStatus: WorkspaceStatusActive,
		},
		{
			name:           "deserialize closed",
			jsonInput:      `{"status":"closed"}`,
			expectedStatus: WorkspaceStatusClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w wrapper
			err := json.Unmarshal([]byte(tt.jsonInput), &w)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, w.Status)
		})
	}
}

func TestValidationProgressStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   ValidationProgressStatus
		expected string
	}{
		{
			name:     "starting status",
			status:   ValidationProgressStarting,
			expected: "starting",
		},
		{
			name:     "completed status",
			status:   ValidationProgressCompleted,
			expected: "completed",
		},
		{
			name:     "failed status",
			status:   ValidationProgressFailed,
			expected: "failed",
		},
		{
			name:     "skipped status",
			status:   ValidationProgressSkipped,
			expected: "skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestStepResultStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   StepResultStatus
		expected string
	}{
		{
			name:     "success status",
			status:   StepResultSuccess,
			expected: "success",
		},
		{
			name:     "failed status",
			status:   StepResultFailed,
			expected: "failed",
		},
		{
			name:     "awaiting_approval status",
			status:   StepResultAwaitingApproval,
			expected: "awaiting_approval",
		},
		{
			name:     "skipped status",
			status:   StepResultSkipped,
			expected: "skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}
