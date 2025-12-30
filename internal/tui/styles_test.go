package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestStatusColors(t *testing.T) {
	colors := StatusColors()

	// Verify all workspace statuses have colors defined
	statuses := []constants.WorkspaceStatus{
		constants.WorkspaceStatusActive,
		constants.WorkspaceStatusPaused,
		constants.WorkspaceStatusRetired,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}

func TestNewTableStyles(t *testing.T) {
	styles := NewTableStyles()
	assert.NotNil(t, styles)
	assert.NotNil(t, styles.StatusColors)
}

func TestNewOutputStyles(t *testing.T) {
	styles := NewOutputStyles()
	assert.NotNil(t, styles)
}

func TestTaskStatusColors(t *testing.T) {
	colors := TaskStatusColors()

	// Verify all task statuses have colors defined
	statuses := []constants.TaskStatus{
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
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}

func TestTaskStatusIcon(t *testing.T) {
	tests := []struct {
		status       constants.TaskStatus
		expectedIcon string
	}{
		{constants.TaskStatusPending, "○"},
		{constants.TaskStatusRunning, "▶"},
		{constants.TaskStatusValidating, "◐"},
		{constants.TaskStatusValidationFailed, "⚠"},
		{constants.TaskStatusAwaitingApproval, "◉"},
		{constants.TaskStatusCompleted, "✓"},
		{constants.TaskStatusRejected, "✗"},
		{constants.TaskStatusAbandoned, "⊘"},
		{constants.TaskStatusGHFailed, "⚠"},
		{constants.TaskStatusCIFailed, "⚠"},
		{constants.TaskStatusCITimeout, "⏱"},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			icon := TaskStatusIcon(tc.status)
			assert.Equal(t, tc.expectedIcon, icon)
		})
	}
}

func TestIsAttentionStatus(t *testing.T) {
	attentionStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	nonAttentionStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range attentionStatuses {
		t.Run(string(status)+"_needs_attention", func(t *testing.T) {
			assert.True(t, IsAttentionStatus(status))
		})
	}

	for _, status := range nonAttentionStatuses {
		t.Run(string(status)+"_no_attention", func(t *testing.T) {
			assert.False(t, IsAttentionStatus(status))
		})
	}
}

func TestSuggestedAction(t *testing.T) {
	tests := []struct {
		status         constants.TaskStatus
		expectedAction string
	}{
		{constants.TaskStatusValidationFailed, "atlas resume"},
		{constants.TaskStatusAwaitingApproval, "atlas approve"},
		{constants.TaskStatusGHFailed, "atlas retry"},
		{constants.TaskStatusCIFailed, "atlas retry"},
		{constants.TaskStatusCITimeout, "atlas retry"},
		{constants.TaskStatusRunning, ""},
		{constants.TaskStatusCompleted, ""},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			action := SuggestedAction(tc.status)
			assert.Equal(t, tc.expectedAction, action)
		})
	}
}
