package tui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/tui"
)

func TestRecoveryAction_String(t *testing.T) {
	tests := []struct {
		action   tui.RecoveryAction
		expected string
	}{
		{tui.RecoveryActionRetryAI, "retry_ai"},
		{tui.RecoveryActionFixManually, "fix_manually"},
		{tui.RecoveryActionViewErrors, "view_errors"},
		{tui.RecoveryActionViewLogs, "view_logs"},
		{tui.RecoveryActionContinueWaiting, "continue_waiting"},
		{tui.RecoveryActionAbandon, "abandon"},
		{tui.RecoveryActionRetryGH, "retry_gh"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.action.String())
		})
	}
}

func TestValidationFailedOptions(t *testing.T) {
	options := tui.ValidationFailedOptions()

	require.Len(t, options, 4, "validation_failed should have 4 options")

	// Verify option order and content
	assert.Equal(t, "Retry with AI fix", options[0].Label)
	assert.Equal(t, tui.RecoveryActionRetryAI, options[0].Action)

	assert.Equal(t, "Fix manually", options[1].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[1].Action)

	assert.Equal(t, "View errors", options[2].Label)
	assert.Equal(t, tui.RecoveryActionViewErrors, options[2].Action)

	assert.Equal(t, "Abandon task", options[3].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[3].Action)

	// Verify all options have descriptions
	for _, opt := range options {
		assert.NotEmpty(t, opt.Description, "option %s should have description", opt.Label)
		assert.NotEmpty(t, opt.Value, "option %s should have value", opt.Label)
	}
}

func TestGHFailedOptions(t *testing.T) {
	options := tui.GHFailedOptions()

	require.Len(t, options, 3, "gh_failed should have 3 options")

	// Verify option order and content
	assert.Equal(t, "Retry push/PR", options[0].Label)
	assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)

	assert.Equal(t, "Fix manually", options[1].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[1].Action)

	assert.Equal(t, "Abandon task", options[2].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[2].Action)

	// Verify all options have descriptions
	for _, opt := range options {
		assert.NotEmpty(t, opt.Description, "option %s should have description", opt.Label)
	}
}

func TestCIFailedOptions(t *testing.T) {
	options := tui.CIFailedOptions()

	require.Len(t, options, 4, "ci_failed should have 4 options")

	// Verify option order and content
	assert.Equal(t, "View workflow logs", options[0].Label)
	assert.Equal(t, tui.RecoveryActionViewLogs, options[0].Action)

	assert.Equal(t, "Retry from implement", options[1].Label)
	assert.Equal(t, tui.RecoveryActionRetryAI, options[1].Action)

	assert.Equal(t, "Fix manually", options[2].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[2].Action)

	assert.Equal(t, "Abandon task", options[3].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[3].Action)
}

func TestCITimeoutOptions(t *testing.T) {
	options := tui.CITimeoutOptions()

	require.Len(t, options, 4, "ci_timeout should have 4 options")

	// Verify option order and content
	assert.Equal(t, "Continue waiting", options[0].Label)
	assert.Equal(t, tui.RecoveryActionContinueWaiting, options[0].Action)

	assert.Equal(t, "View workflow logs", options[1].Label)
	assert.Equal(t, tui.RecoveryActionViewLogs, options[1].Action)

	assert.Equal(t, "Fix manually", options[2].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[2].Action)

	assert.Equal(t, "Abandon task", options[3].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[3].Action)
}

func TestGetOptionsForStatus(t *testing.T) {
	tests := []struct {
		status        constants.TaskStatus
		expectedCount int
		description   string
	}{
		{constants.TaskStatusValidationFailed, 4, "validation_failed returns 4 options"},
		{constants.TaskStatusGHFailed, 3, "gh_failed returns 3 options"},
		{constants.TaskStatusCIFailed, 4, "ci_failed returns 4 options"},
		{constants.TaskStatusCITimeout, 4, "ci_timeout returns 4 options"},
		{constants.TaskStatusRunning, 0, "non-error status returns nil"},
		{constants.TaskStatusCompleted, 0, "completed returns nil"},
		{constants.TaskStatusPending, 0, "pending returns nil"},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			options := tui.GetOptionsForStatus(tc.status)
			if tc.expectedCount == 0 {
				assert.Nil(t, options)
			} else {
				assert.Len(t, options, tc.expectedCount)
			}
		})
	}
}

func TestGetMenuTitleForStatus(t *testing.T) {
	tests := []struct {
		status   constants.TaskStatus
		contains string
	}{
		{constants.TaskStatusValidationFailed, "Validation failed"},
		{constants.TaskStatusGHFailed, "GitHub operation failed"},
		{constants.TaskStatusCIFailed, "CI workflow failed"},
		{constants.TaskStatusCITimeout, "CI polling timeout"},
		{constants.TaskStatusRunning, "Error occurred"}, // Default case
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			title := tui.GetMenuTitleForStatus(tc.status)
			assert.Contains(t, title, tc.contains)
			assert.Contains(t, title, "What would you like to do?")
		})
	}
}

func TestIsViewAction(t *testing.T) {
	viewActions := []tui.RecoveryAction{
		tui.RecoveryActionViewErrors,
		tui.RecoveryActionViewLogs,
	}

	nonViewActions := []tui.RecoveryAction{
		tui.RecoveryActionRetryAI,
		tui.RecoveryActionFixManually,
		tui.RecoveryActionAbandon,
		tui.RecoveryActionRetryGH,
		tui.RecoveryActionContinueWaiting,
	}

	for _, action := range viewActions {
		t.Run(action.String()+"_is_view", func(t *testing.T) {
			assert.True(t, tui.IsViewAction(action), "%s should be a view action", action)
		})
	}

	for _, action := range nonViewActions {
		t.Run(action.String()+"_is_not_view", func(t *testing.T) {
			assert.False(t, tui.IsViewAction(action), "%s should not be a view action", action)
		})
	}
}

func TestIsTerminalAction(t *testing.T) {
	terminalActions := []tui.RecoveryAction{
		tui.RecoveryActionRetryAI,
		tui.RecoveryActionFixManually,
		tui.RecoveryActionAbandon,
		tui.RecoveryActionRetryGH,
		tui.RecoveryActionContinueWaiting,
	}

	nonTerminalActions := []tui.RecoveryAction{
		tui.RecoveryActionViewErrors,
		tui.RecoveryActionViewLogs,
	}

	for _, action := range terminalActions {
		t.Run(action.String()+"_is_terminal", func(t *testing.T) {
			assert.True(t, tui.IsTerminalAction(action), "%s should be a terminal action", action)
		})
	}

	for _, action := range nonTerminalActions {
		t.Run(action.String()+"_is_not_terminal", func(t *testing.T) {
			assert.False(t, tui.IsTerminalAction(action), "%s should not be a terminal action", action)
		})
	}
}

func TestErrorRecoveryOption_HasBaseOption(t *testing.T) {
	// Verify that ErrorRecoveryOption properly embeds Option
	options := tui.ValidationFailedOptions()
	require.NotEmpty(t, options)

	// Should be able to access Option fields directly
	opt := options[0]
	assert.Equal(t, "Retry with AI fix", opt.Label)
	assert.NotEmpty(t, opt.Description)
	assert.Equal(t, "retry_ai", opt.Value)
}

func TestAllOptionsHaveEscapeRoute(t *testing.T) {
	// Per AC #5: All menus must have escape routes (Cancel via q/Esc, Abandon option)
	statuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			options := tui.GetOptionsForStatus(status)
			require.NotNil(t, options)

			// Check that "Abandon task" option exists
			hasAbandon := false
			for _, opt := range options {
				if opt.Action == tui.RecoveryActionAbandon {
					hasAbandon = true
					break
				}
			}
			assert.True(t, hasAbandon, "menu for %s should have Abandon option", status)
		})
	}
}

func TestAllOptionsHaveConsistentFormat(t *testing.T) {
	// Per AC #4: All menus follow consistent styling
	statuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			options := tui.GetOptionsForStatus(status)
			require.NotNil(t, options)

			for _, opt := range options {
				// Each option should have Label, Description, Value, Action
				assert.NotEmpty(t, opt.Label, "option should have label")
				assert.NotEmpty(t, opt.Description, "option should have description")
				assert.NotEmpty(t, opt.Value, "option should have value")
				assert.NotEmpty(t, opt.Action, "option should have action")

				// Value should match Action string
				assert.Equal(t, string(opt.Action), opt.Value,
					"option value should match action string")
			}
		})
	}
}

// TestSelectErrorRecovery_NonErrorStatus verifies that SelectErrorRecovery
// returns ErrMenuCanceled when called with a non-error status (H1 fix).
func TestSelectErrorRecovery_NonErrorStatus(t *testing.T) {
	// Non-error statuses should return ErrMenuCanceled because GetOptionsForStatus returns nil
	nonErrorStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range nonErrorStatuses {
		t.Run(string(status), func(t *testing.T) {
			action, err := tui.SelectErrorRecovery(status)
			require.ErrorIs(t, err, tui.ErrMenuCanceled)
			assert.Empty(t, action)
		})
	}
}
