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

func TestOptionsForStatus(t *testing.T) {
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
			options := tui.OptionsForStatus(tc.status)
			if tc.expectedCount == 0 {
				assert.Nil(t, options)
			} else {
				assert.Len(t, options, tc.expectedCount)
			}
		})
	}
}

func TestMenuTitleForStatus(t *testing.T) {
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
			title := tui.MenuTitleForStatus(tc.status)
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
			options := tui.OptionsForStatus(status)
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
			options := tui.OptionsForStatus(status)
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
	// Non-error statuses should return ErrMenuCanceled because OptionsForStatus returns nil
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

// TestRecoveryActionRetryCommit_String verifies the new retry_commit action.
func TestRecoveryActionRetryCommit_String(t *testing.T) {
	assert.Equal(t, "retry_commit", tui.RecoveryActionRetryCommit.String())
}

// TestCommitFailedOptions verifies the options for git_commit step failures.
func TestCommitFailedOptions(t *testing.T) {
	options := tui.CommitFailedOptions()

	require.Len(t, options, 3, "commit_failed should have 3 options")

	// Verify option order and content
	assert.Equal(t, "Retry commit", options[0].Label)
	assert.Equal(t, tui.RecoveryActionRetryCommit, options[0].Action)
	assert.Equal(t, "Retry the failed commit operation", options[0].Description)

	assert.Equal(t, "Fix manually", options[1].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[1].Action)
	assert.Equal(t, "Check and fix issues, then resume", options[1].Description)

	assert.Equal(t, "Abandon task", options[2].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[2].Action)
	assert.Equal(t, "End task, keep branch for later", options[2].Description)

	// Verify all options have proper values
	for _, opt := range options {
		assert.NotEmpty(t, opt.Value, "option %s should have value", opt.Label)
		assert.Equal(t, string(opt.Action), opt.Value, "value should match action")
	}
}

// TestPRFailedOptions verifies the options for git_pr step failures.
func TestPRFailedOptions(t *testing.T) {
	options := tui.PRFailedOptions()

	require.Len(t, options, 3, "pr_failed should have 3 options")

	// Verify option order and content
	assert.Equal(t, "Retry PR creation", options[0].Label)
	assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)
	assert.Equal(t, "Retry creating the pull request", options[0].Description)

	assert.Equal(t, "Fix manually", options[1].Label)
	assert.Equal(t, tui.RecoveryActionFixManually, options[1].Action)
	assert.Equal(t, "Check and fix issues, then resume", options[1].Description)

	assert.Equal(t, "Abandon task", options[2].Label)
	assert.Equal(t, tui.RecoveryActionAbandon, options[2].Action)
	assert.Equal(t, "End task, keep branch for later", options[2].Description)

	// Verify all options have proper values
	for _, opt := range options {
		assert.NotEmpty(t, opt.Value, "option %s should have value", opt.Label)
		assert.Equal(t, string(opt.Action), opt.Value, "value should match action")
	}
}

// TestMenuTitleForGHFailedStep verifies step-aware menu titles.
func TestMenuTitleForGHFailedStep(t *testing.T) {
	tests := []struct {
		stepName string
		expected string
	}{
		{"git_commit", "Commit failed. What would you like to do?"},
		{"git_push", "Push failed. What would you like to do?"},
		{"git_pr", "PR creation failed. What would you like to do?"},
		{"unknown_step", "GitHub operation failed. What would you like to do?"},
		{"", "GitHub operation failed. What would you like to do?"},
		{"some_other_step", "GitHub operation failed. What would you like to do?"},
	}

	for _, tc := range tests {
		name := tc.stepName
		if name == "" {
			name = "empty_step"
		}
		t.Run(name, func(t *testing.T) {
			title := tui.MenuTitleForGHFailedStep(tc.stepName)
			assert.Equal(t, tc.expected, title)
		})
	}
}

// TestOptionsForGHFailedStep verifies step-aware recovery options.
func TestOptionsForGHFailedStep(t *testing.T) {
	t.Run("git_commit_returns_commit_options", func(t *testing.T) {
		options := tui.OptionsForGHFailedStep("git_commit")
		require.Len(t, options, 3)

		// Should have "Retry commit" as first option
		assert.Equal(t, "Retry commit", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRetryCommit, options[0].Action)
	})

	t.Run("git_push_returns_gh_failed_options", func(t *testing.T) {
		options := tui.OptionsForGHFailedStep("git_push")
		require.Len(t, options, 3)

		// Should have "Retry push/PR" as first option (standard GHFailedOptions)
		assert.Equal(t, "Retry push/PR", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)
	})

	t.Run("git_pr_returns_pr_options", func(t *testing.T) {
		options := tui.OptionsForGHFailedStep("git_pr")
		require.Len(t, options, 3)

		// Should have "Retry PR creation" as first option
		assert.Equal(t, "Retry PR creation", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)
	})

	t.Run("unknown_step_returns_default_options", func(t *testing.T) {
		options := tui.OptionsForGHFailedStep("unknown_step")
		require.Len(t, options, 3)

		// Should fall back to standard GHFailedOptions
		assert.Equal(t, "Retry push/PR", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)
	})

	t.Run("empty_step_returns_default_options", func(t *testing.T) {
		options := tui.OptionsForGHFailedStep("")
		require.Len(t, options, 3)

		// Should fall back to standard GHFailedOptions
		assert.Equal(t, "Retry push/PR", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRetryGH, options[0].Action)
	})
}

// TestAllStepOptionsHaveEscapeRoute verifies all step-specific menus have abandon option.
func TestAllStepOptionsHaveEscapeRoute(t *testing.T) {
	stepNames := []string{"git_commit", "git_push", "git_pr", "unknown_step", ""}

	for _, stepName := range stepNames {
		name := stepName
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			options := tui.OptionsForGHFailedStep(stepName)
			require.NotEmpty(t, options)

			hasAbandon := false
			for _, opt := range options {
				if opt.Action == tui.RecoveryActionAbandon {
					hasAbandon = true
					break
				}
			}
			assert.True(t, hasAbandon, "step %q options should have Abandon option", stepName)
		})
	}
}

// TestAllStepOptionsHaveFixManually verifies all step-specific menus have fix manually option.
func TestAllStepOptionsHaveFixManually(t *testing.T) {
	stepNames := []string{"git_commit", "git_push", "git_pr", "unknown_step", ""}

	for _, stepName := range stepNames {
		name := stepName
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			options := tui.OptionsForGHFailedStep(stepName)
			require.NotEmpty(t, options)

			hasFixManually := false
			for _, opt := range options {
				if opt.Action == tui.RecoveryActionFixManually {
					hasFixManually = true
					break
				}
			}
			assert.True(t, hasFixManually, "step %q options should have Fix manually option", stepName)
		})
	}
}

// TestStepAwareOptionsConsistentFormat verifies all step-specific options have consistent format.
func TestStepAwareOptionsConsistentFormat(t *testing.T) {
	stepNames := []string{"git_commit", "git_push", "git_pr", "unknown_step", ""}

	for _, stepName := range stepNames {
		name := stepName
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			options := tui.OptionsForGHFailedStep(stepName)
			require.NotEmpty(t, options)

			for _, opt := range options {
				assert.NotEmpty(t, opt.Label, "option should have label")
				assert.NotEmpty(t, opt.Description, "option should have description")
				assert.NotEmpty(t, opt.Value, "option should have value")
				assert.NotEmpty(t, opt.Action, "option should have action")
				assert.Equal(t, string(opt.Action), opt.Value, "value should match action string")
			}
		})
	}
}

// TestRetryCommitIsTerminalAction verifies RecoveryActionRetryCommit is a terminal action.
func TestRetryCommitIsTerminalAction(t *testing.T) {
	assert.True(t, tui.IsTerminalAction(tui.RecoveryActionRetryCommit),
		"RecoveryActionRetryCommit should be a terminal action")
	assert.False(t, tui.IsViewAction(tui.RecoveryActionRetryCommit),
		"RecoveryActionRetryCommit should not be a view action")
}

// TestRebaseRetryConstant verifies the existing rebase retry constant.
func TestRebaseRetryConstant(t *testing.T) {
	assert.Equal(t, "rebase_retry", tui.RecoveryActionRebaseRetry.String())
	assert.True(t, tui.IsTerminalAction(tui.RecoveryActionRebaseRetry),
		"RecoveryActionRebaseRetry should be a terminal action")
}

// TestGHFailedOptionsForPushError_WithStepContext tests that push error options work correctly.
func TestGHFailedOptionsForPushError_WithStepContext(t *testing.T) {
	t.Run("non_fast_forward_adds_rebase_option", func(t *testing.T) {
		options := tui.GHFailedOptionsForPushError("non_fast_forward")
		require.GreaterOrEqual(t, len(options), 4, "should have rebase option + standard options")

		// First option should be "Rebase and retry"
		assert.Equal(t, "Rebase and retry", options[0].Label)
		assert.Equal(t, tui.RecoveryActionRebaseRetry, options[0].Action)
		assert.Equal(t, "Integrate remote changes, then push", options[0].Description)
	})

	t.Run("empty_error_type_returns_standard_options", func(t *testing.T) {
		options := tui.GHFailedOptionsForPushError("")
		require.Len(t, options, 3, "should return standard 3 options")

		// Should be standard GHFailedOptions
		assert.Equal(t, "Retry push/PR", options[0].Label)
	})

	t.Run("other_error_type_returns_standard_options", func(t *testing.T) {
		options := tui.GHFailedOptionsForPushError("permission_denied")
		require.Len(t, options, 3, "should return standard 3 options for unknown error type")

		// Should be standard GHFailedOptions
		assert.Equal(t, "Retry push/PR", options[0].Label)
	})
}

// TestMenuTitleAndOptionsMatch verifies titles and options are consistent for each step.
func TestMenuTitleAndOptionsMatch(t *testing.T) {
	tests := []struct {
		stepName        string
		expectedTitle   string
		expectedFirstOp string
	}{
		{"git_commit", "Commit failed", "Retry commit"},
		{"git_push", "Push failed", "Retry push/PR"},
		{"git_pr", "PR creation failed", "Retry PR creation"},
	}

	for _, tc := range tests {
		t.Run(tc.stepName, func(t *testing.T) {
			title := tui.MenuTitleForGHFailedStep(tc.stepName)
			options := tui.OptionsForGHFailedStep(tc.stepName)

			assert.Contains(t, title, tc.expectedTitle, "title should mention the failed step")
			assert.Equal(t, tc.expectedFirstOp, options[0].Label, "first option should match step type")
		})
	}
}
