package errors

import "errors"

// UserMessage returns a user-friendly message for common errors.
// This function maps sentinel errors to helpful, actionable messages
// that are suitable for display to end users.
//
// For unrecognized errors, it returns the error's original message.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, ErrValidationFailed):
		return "Validation failed. Check the output above for specific errors."
	case errors.Is(err, ErrClaudeInvocation):
		return "Failed to communicate with Claude. Check your API key and network."
	case errors.Is(err, ErrGitOperation):
		return "Git operation failed. Check your repository state."
	case errors.Is(err, ErrGitHubOperation):
		return "GitHub operation failed. Check your authentication and permissions."
	case errors.Is(err, ErrCIFailed):
		return "CI workflow failed. Check the workflow logs in GitHub Actions."
	case errors.Is(err, ErrCITimeout):
		return "CI polling timed out. Check if CI is running or retry later."
	case errors.Is(err, ErrUserRejected):
		return "Task was rejected. Provide feedback for retry or abandon."
	case errors.Is(err, ErrUserAbandoned):
		return "Task was abandoned. Workspace and branch preserved."
	default:
		return err.Error()
	}
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

	switch {
	case errors.Is(err, ErrValidationFailed):
		return "Validation failed. Check the output above for specific errors.",
			"Fix the issues reported by the validation commands and retry."

	case errors.Is(err, ErrClaudeInvocation):
		return "Failed to communicate with Claude. Check your API key and network.",
			"Verify ANTHROPIC_API_KEY is set correctly and you have network access."

	case errors.Is(err, ErrGitOperation):
		return "Git operation failed. Check your repository state.",
			"Ensure you have a clean git state and proper permissions."

	case errors.Is(err, ErrGitHubOperation):
		return "GitHub operation failed. Check your authentication and permissions.",
			"Verify GH_TOKEN is set and has required repository permissions."

	case errors.Is(err, ErrCIFailed):
		return "CI workflow failed. Check the workflow logs in GitHub Actions.",
			"Review the CI logs, fix any failing checks, and push a new commit."

	case errors.Is(err, ErrCITimeout):
		return "CI polling timed out. Check if CI is running or retry later.",
			"Run 'atlas status' to check current CI status, or increase timeout."

	case errors.Is(err, ErrUserRejected):
		return "Task was rejected. Provide feedback for retry or abandon.",
			"Choose to provide feedback and retry, or abandon the task."

	case errors.Is(err, ErrUserAbandoned):
		return "Task was abandoned. Workspace and branch preserved.",
			"Run 'atlas workspace list' to see preserved workspaces."

	default:
		return err.Error(), ""
	}
}
