package errors

import "errors"

// ErrorInfo holds user-facing message and suggested action for an error.
type ErrorInfo struct {
	// Message is the user-friendly error description.
	Message string
	// Action is a suggested action to resolve the issue (empty if none).
	Action string
}

// errorInfoMap returns the mapping of sentinel errors to their user-facing messages and actions.
// This single source of truth ensures UserMessage and Actionable stay in sync.
func errorInfoMap() []struct {
	err  error
	info ErrorInfo
} {
	return []struct {
		err  error
		info ErrorInfo
	}{
		{
			err: ErrValidationFailed,
			info: ErrorInfo{
				Message: "Validation failed. Check the output above for specific errors.",
				Action:  "Fix the issues reported by the validation commands and retry.",
			},
		},
		{
			err: ErrClaudeInvocation,
			info: ErrorInfo{
				Message: "Failed to communicate with Claude. Check your API key and network.",
				Action:  "Verify ANTHROPIC_API_KEY is set correctly and you have network access.",
			},
		},
		{
			err: ErrGitOperation,
			info: ErrorInfo{
				Message: "Git operation failed. Check your repository state.",
				Action:  "Ensure you have a clean git state and proper permissions.",
			},
		},
		{
			err: ErrGitHubOperation,
			info: ErrorInfo{
				Message: "GitHub operation failed. Check your authentication and permissions.",
				Action:  "Verify GH_TOKEN is set and has required repository permissions.",
			},
		},
		{
			err: ErrCIFailed,
			info: ErrorInfo{
				Message: "CI workflow failed. Check the workflow logs in GitHub Actions.",
				Action:  "Review the CI logs, fix any failing checks, and push a new commit.",
			},
		},
		{
			err: ErrCITimeout,
			info: ErrorInfo{
				Message: "CI polling timed out. Check if CI is running or retry later.",
				Action:  "Run 'atlas status' to check current CI status, or increase timeout.",
			},
		},
		{
			err: ErrUserRejected,
			info: ErrorInfo{
				Message: "Task was rejected. Provide feedback for retry or abandon.",
				Action:  "Choose to provide feedback and retry, or abandon the task.",
			},
		},
		{
			err: ErrUserAbandoned,
			info: ErrorInfo{
				Message: "Task was abandoned. Workspace and branch preserved.",
				Action:  "Run 'atlas workspace list' to see preserved workspaces.",
			},
		},
	}
}

// getErrorInfo looks up the ErrorInfo for a given error.
// Returns an ErrorInfo with the original error message if not found.
func getErrorInfo(err error) ErrorInfo {
	for _, entry := range errorInfoMap() {
		if errors.Is(err, entry.err) {
			return entry.info
		}
	}
	return ErrorInfo{Message: err.Error()}
}

// UserMessage returns a user-friendly message for common errors.
// This function maps sentinel errors to helpful, actionable messages
// that are suitable for display to end users.
//
// For unrecognized errors, it returns the error's original message.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	return getErrorInfo(err).Message
}

// Actionable returns a user-friendly error message along with a suggested
// action the user can take to resolve or work around the issue.
//
// For errors that are not recoverable or have no clear action, the action
// string will be empty.
func Actionable(err error) (message, action string) {
	if err == nil {
		return "", ""
	}
	info := getErrorInfo(err)
	return info.Message, info.Action
}
