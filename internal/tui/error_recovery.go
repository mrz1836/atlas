// Package tui provides terminal user interface components for ATLAS.
//
// This file implements error recovery menus for handling task error states.
// Error recovery menus provide interactive options for users to recover from
// validation failures, GitHub operation failures, CI failures, and CI timeouts.
//
// # Error States Supported
//
//   - validation_failed: Validation checks failed, can retry with AI or manually
//   - gh_failed: GitHub operations (push/PR) failed
//   - ci_failed: CI pipeline checks failed
//   - ci_timeout: CI pipeline exceeded timeout
//
// # UX Compliance
//
// All menus follow UX-11 (consistent styling) and UX-12 (keyboard navigation)
// guidelines. Menus use the established style system from styles.go.
package tui

import (
	"github.com/mrz1836/atlas/internal/constants"
)

// RecoveryAction represents an action the user can take to recover from an error.
type RecoveryAction string

// Recovery action constants define the available recovery options.
const (
	// RecoveryActionRetryAI instructs AI to attempt fixing the issue.
	RecoveryActionRetryAI RecoveryAction = "retry_ai"

	// RecoveryActionFixManually instructs user to fix manually in worktree.
	RecoveryActionFixManually RecoveryAction = "fix_manually"

	// RecoveryActionViewErrors shows detailed error output.
	RecoveryActionViewErrors RecoveryAction = "view_errors"

	// RecoveryActionViewLogs opens GitHub Actions logs in browser.
	RecoveryActionViewLogs RecoveryAction = "view_logs"

	// RecoveryActionContinueWaiting resumes CI polling with extended timeout.
	RecoveryActionContinueWaiting RecoveryAction = "continue_waiting"

	// RecoveryActionAbandon ends the task, preserving branch for later work.
	RecoveryActionAbandon RecoveryAction = "abandon"

	// RecoveryActionRetryGH retries the failed GitHub operation.
	RecoveryActionRetryGH RecoveryAction = "retry_gh"

	// RecoveryActionRebaseRetry rebases local commits onto remote and retries push.
	RecoveryActionRebaseRetry RecoveryAction = "rebase_retry"
)

// String returns the string representation of the RecoveryAction.
func (a RecoveryAction) String() string {
	return string(a)
}

// ErrorRecoveryOption represents a menu option for error recovery.
// It extends the base Option with the action type for programmatic handling.
type ErrorRecoveryOption struct {
	Option

	Action RecoveryAction
}

// ValidationFailedOptions returns the menu options for validation_failed state.
// From UX spec:
//
//	? Validation failed. What would you like to do?
//	  ❯ Retry with AI fix — Claude attempts to fix based on errors
//	    Fix manually — Edit files in worktree, then resume
//	    View errors — Show detailed validation output
//	    Abandon task — End task, keep branch for later
func ValidationFailedOptions() []ErrorRecoveryOption {
	return []ErrorRecoveryOption{
		{
			Option: Option{
				Label:       "Retry with AI fix",
				Description: "Claude attempts to fix based on errors",
				Value:       string(RecoveryActionRetryAI),
			},
			Action: RecoveryActionRetryAI,
		},
		{
			Option: Option{
				Label:       "Fix manually",
				Description: "Edit files in worktree, then resume",
				Value:       string(RecoveryActionFixManually),
			},
			Action: RecoveryActionFixManually,
		},
		{
			Option: Option{
				Label:       "View errors",
				Description: "Show detailed validation output",
				Value:       string(RecoveryActionViewErrors),
			},
			Action: RecoveryActionViewErrors,
		},
		{
			Option: Option{
				Label:       "Abandon task",
				Description: "End task, keep branch for later",
				Value:       string(RecoveryActionAbandon),
			},
			Action: RecoveryActionAbandon,
		},
	}
}

// GHFailedOptions returns the menu options for gh_failed state.
// From UX spec:
//
//	? GitHub operation failed. What would you like to do?
//	  ❯ Retry push/PR — Retry the failed operation
//	    Fix manually — Check and fix issues, then resume
//	    Abandon task — End task, keep branch for later
func GHFailedOptions() []ErrorRecoveryOption {
	return []ErrorRecoveryOption{
		{
			Option: Option{
				Label:       "Retry push/PR",
				Description: "Retry the failed operation",
				Value:       string(RecoveryActionRetryGH),
			},
			Action: RecoveryActionRetryGH,
		},
		{
			Option: Option{
				Label:       "Fix manually",
				Description: "Check and fix issues, then resume",
				Value:       string(RecoveryActionFixManually),
			},
			Action: RecoveryActionFixManually,
		},
		{
			Option: Option{
				Label:       "Abandon task",
				Description: "End task, keep branch for later",
				Value:       string(RecoveryActionAbandon),
			},
			Action: RecoveryActionAbandon,
		},
	}
}

// GHFailedOptionsForPushError returns context-aware menu options for gh_failed state
// based on the specific push error type. For non-fast-forward errors, this adds
// a "Rebase and retry" option as the first choice.
func GHFailedOptionsForPushError(pushErrorType string) []ErrorRecoveryOption {
	options := []ErrorRecoveryOption{}

	// For non-fast-forward errors, add rebase option as the first choice
	if pushErrorType == "non_fast_forward" {
		options = append(options, ErrorRecoveryOption{
			Option: Option{
				Label:       "Rebase and retry",
				Description: "Integrate remote changes, then push",
				Value:       string(RecoveryActionRebaseRetry),
			},
			Action: RecoveryActionRebaseRetry,
		})
	}

	// Add standard options
	options = append(options, GHFailedOptions()...)
	return options
}

// CIFailedOptions returns the menu options for ci_failed state.
// From UX spec (Story 6.7):
//
//	? CI workflow "CI" failed. What would you like to do?
//	  ❯ View workflow logs — Open GitHub Actions in browser
//	    Retry from implement — AI tries to fix based on CI output
//	    Fix manually — You fix in worktree, then resume
//	    Abandon task — End task, keep PR as draft
func CIFailedOptions() []ErrorRecoveryOption {
	return []ErrorRecoveryOption{
		{
			Option: Option{
				Label:       "View workflow logs",
				Description: "Open GitHub Actions in browser",
				Value:       string(RecoveryActionViewLogs),
			},
			Action: RecoveryActionViewLogs,
		},
		{
			Option: Option{
				Label:       "Retry from implement",
				Description: "AI tries to fix based on CI output",
				Value:       string(RecoveryActionRetryAI),
			},
			Action: RecoveryActionRetryAI,
		},
		{
			Option: Option{
				Label:       "Fix manually",
				Description: "You fix in worktree, then resume",
				Value:       string(RecoveryActionFixManually),
			},
			Action: RecoveryActionFixManually,
		},
		{
			Option: Option{
				Label:       "Abandon task",
				Description: "End task, keep PR as draft",
				Value:       string(RecoveryActionAbandon),
			},
			Action: RecoveryActionAbandon,
		},
	}
}

// CITimeoutOptions returns the menu options for ci_timeout state.
// From UX spec:
//
//	? CI polling timeout. What would you like to do?
//	  ❯ Continue waiting — Resume polling with extended timeout
//	    View workflow logs — Check CI status in browser
//	    Fix manually — Check CI status and resume when ready
//	    Abandon task — End task
func CITimeoutOptions() []ErrorRecoveryOption {
	return []ErrorRecoveryOption{
		{
			Option: Option{
				Label:       "Continue waiting",
				Description: "Resume polling with extended timeout",
				Value:       string(RecoveryActionContinueWaiting),
			},
			Action: RecoveryActionContinueWaiting,
		},
		{
			Option: Option{
				Label:       "View workflow logs",
				Description: "Check CI status in browser",
				Value:       string(RecoveryActionViewLogs),
			},
			Action: RecoveryActionViewLogs,
		},
		{
			Option: Option{
				Label:       "Fix manually",
				Description: "Check CI status and resume when ready",
				Value:       string(RecoveryActionFixManually),
			},
			Action: RecoveryActionFixManually,
		},
		{
			Option: Option{
				Label:       "Abandon task",
				Description: "End task",
				Value:       string(RecoveryActionAbandon),
			},
			Action: RecoveryActionAbandon,
		},
	}
}

// OptionsForStatus returns the appropriate recovery options for a given task status.
// Returns nil if the status is not an error state.
func OptionsForStatus(status constants.TaskStatus) []ErrorRecoveryOption {
	//nolint:exhaustive // Only error states have recovery options
	switch status {
	case constants.TaskStatusValidationFailed:
		return ValidationFailedOptions()
	case constants.TaskStatusGHFailed:
		return GHFailedOptions()
	case constants.TaskStatusCIFailed:
		return CIFailedOptions()
	case constants.TaskStatusCITimeout:
		return CITimeoutOptions()
	default:
		return nil
	}
}

// MenuTitleForStatus returns the appropriate menu title for a given error status.
func MenuTitleForStatus(status constants.TaskStatus) string {
	//nolint:exhaustive // Only error states have specific titles
	switch status {
	case constants.TaskStatusValidationFailed:
		return "Validation failed. What would you like to do?"
	case constants.TaskStatusGHFailed:
		return "GitHub operation failed. What would you like to do?"
	case constants.TaskStatusCIFailed:
		return "CI workflow failed. What would you like to do?"
	case constants.TaskStatusCITimeout:
		return "CI polling timeout. What would you like to do?"
	default:
		return "Error occurred. What would you like to do?"
	}
}

// SelectErrorRecovery presents an error recovery menu and returns the selected action.
// Uses the established menu system from menus.go with ATLAS styling.
// Returns ErrMenuCanceled if user presses q or Esc.
func SelectErrorRecovery(status constants.TaskStatus) (RecoveryAction, error) {
	options := OptionsForStatus(status)
	if len(options) == 0 {
		return "", ErrMenuCanceled
	}

	// Convert to base Options for the Select function
	baseOptions := make([]Option, len(options))
	for i, opt := range options {
		baseOptions[i] = opt.Option
	}

	title := MenuTitleForStatus(status)
	selected, err := Select(title, baseOptions)
	if err != nil {
		return "", err
	}

	return RecoveryAction(selected), nil
}

// IsViewAction returns true if the action is a "view" action that should return to menu.
// View actions show information but don't change state, so the menu should be re-displayed.
func IsViewAction(action RecoveryAction) bool {
	//nolint:exhaustive // Only view actions return true
	switch action {
	case RecoveryActionViewErrors, RecoveryActionViewLogs:
		return true
	default:
		return false
	}
}

// IsTerminalAction returns true if the action should exit the recovery menu loop.
// Terminal actions change task state or complete the recovery flow.
func IsTerminalAction(action RecoveryAction) bool {
	return !IsViewAction(action)
}
