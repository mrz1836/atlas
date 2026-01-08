package errors_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// testError is a custom error type used to test default branches
// in UserMessage and Actionable without matching any sentinel.
type testError struct {
	msg string
}

func (e testError) Error() string {
	return e.msg
}

func TestSentinelErrors_Existence(t *testing.T) {
	// Verify all sentinel errors exist and are non-nil
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed},
		{"ErrClaudeInvocation", atlaserrors.ErrClaudeInvocation},
		{"ErrGitOperation", atlaserrors.ErrGitOperation},
		{"ErrGitHubOperation", atlaserrors.ErrGitHubOperation},
		{"ErrCIFailed", atlaserrors.ErrCIFailed},
		{"ErrCITimeout", atlaserrors.ErrCITimeout},
		{"ErrUserRejected", atlaserrors.ErrUserRejected},
		{"ErrUserAbandoned", atlaserrors.ErrUserAbandoned},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.err, "%s should not be nil", tc.name)
			assert.NotEmpty(t, tc.err.Error(), "%s should have a message", tc.name)
		})
	}
}

func TestSentinelErrors_Messages(t *testing.T) {
	// Verify all sentinel errors have lowercase messages per Go conventions
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed, "validation failed"},
		{"ErrClaudeInvocation", atlaserrors.ErrClaudeInvocation, "claude invocation failed"},
		{"ErrGitOperation", atlaserrors.ErrGitOperation, "git operation failed"},
		{"ErrGitHubOperation", atlaserrors.ErrGitHubOperation, "github operation failed"},
		{"ErrCIFailed", atlaserrors.ErrCIFailed, "ci workflow failed"},
		{"ErrCITimeout", atlaserrors.ErrCITimeout, "ci polling timeout"},
		{"ErrUserRejected", atlaserrors.ErrUserRejected, "user rejected"},
		{"ErrUserAbandoned", atlaserrors.ErrUserAbandoned, "user abandoned task"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	// Ensure each sentinel error is unique and errors.Is() distinguishes them
	allErrors := []error{
		atlaserrors.ErrValidationFailed,
		atlaserrors.ErrClaudeInvocation,
		atlaserrors.ErrGitOperation,
		atlaserrors.ErrGitHubOperation,
		atlaserrors.ErrCIFailed,
		atlaserrors.ErrCITimeout,
		atlaserrors.ErrUserRejected,
		atlaserrors.ErrUserAbandoned,
	}

	for i, err1 := range allErrors {
		for j, err2 := range allErrors {
			if i == j {
				assert.ErrorIs(t, err1, err2, "error should match itself")
			} else {
				assert.NotErrorIs(t, err1, err2, "different errors should not match")
			}
		}
	}
}

func TestWrap_PreservesErrorChain(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed},
		{"ErrClaudeInvocation", atlaserrors.ErrClaudeInvocation},
		{"ErrGitOperation", atlaserrors.ErrGitOperation},
		{"ErrGitHubOperation", atlaserrors.ErrGitHubOperation},
		{"ErrCIFailed", atlaserrors.ErrCIFailed},
		{"ErrCITimeout", atlaserrors.ErrCITimeout},
		{"ErrUserRejected", atlaserrors.ErrUserRejected},
		{"ErrUserAbandoned", atlaserrors.ErrUserAbandoned},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := atlaserrors.Wrap(tc.sentinel, "context message")

			require.Error(t, wrapped)
			require.ErrorIs(t, wrapped, tc.sentinel,
				"wrapped error should satisfy errors.Is() for %s", tc.name)
			assert.Contains(t, wrapped.Error(), "context message")
			assert.Contains(t, wrapped.Error(), tc.sentinel.Error())
		})
	}
}

func TestWrap_NilError(t *testing.T) {
	result := atlaserrors.Wrap(nil, "should not appear")
	assert.NoError(t, result, "Wrap(nil, msg) should return nil")
}

func TestWrap_MultipleWraps(t *testing.T) {
	// Test that errors.Is() works through multiple wrap levels
	wrapped1 := atlaserrors.Wrap(atlaserrors.ErrGitOperation, "first wrap")
	wrapped2 := atlaserrors.Wrap(wrapped1, "second wrap")
	wrapped3 := atlaserrors.Wrap(wrapped2, "third wrap")

	require.ErrorIs(t, wrapped3, atlaserrors.ErrGitOperation,
		"errors.Is should work through multiple wrap levels")
	assert.Contains(t, wrapped3.Error(), "first wrap")
	assert.Contains(t, wrapped3.Error(), "second wrap")
	assert.Contains(t, wrapped3.Error(), "third wrap")
}

func TestWrap_MessageFormat(t *testing.T) {
	wrapped := atlaserrors.Wrap(atlaserrors.ErrValidationFailed, "build step failed")

	// The format should be "msg: original error"
	expected := "build step failed: validation failed"
	assert.Equal(t, expected, wrapped.Error())
}

func TestWrapf_PreservesErrorChain(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		format   string
		args     []any
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed, "task %s failed", []any{"abc123"}},
		{"ErrGitOperation", atlaserrors.ErrGitOperation, "branch %s commit %d", []any{"main", 42}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := atlaserrors.Wrapf(tc.sentinel, tc.format, tc.args...)

			require.Error(t, wrapped)
			require.ErrorIs(t, wrapped, tc.sentinel,
				"wrapped error should satisfy errors.Is() for %s", tc.name)

			// Verify the formatted message is present
			expectedMsg := fmt.Sprintf(tc.format, tc.args...)
			assert.Contains(t, wrapped.Error(), expectedMsg)
		})
	}
}

func TestWrapf_NilError(t *testing.T) {
	result := atlaserrors.Wrapf(nil, "task %s", "abc123")
	assert.NoError(t, result, "Wrapf(nil, ...) should return nil")
}

func TestWrapf_MessageFormat(t *testing.T) {
	wrapped := atlaserrors.Wrapf(atlaserrors.ErrCIFailed, "workflow %s run %d", "build", 42)

	expected := "workflow build run 42: ci workflow failed"
	assert.Equal(t, expected, wrapped.Error())
}

func TestUserMessage_AllSentinels(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed, "Validation failed"},
		{"ErrClaudeInvocation", atlaserrors.ErrClaudeInvocation, "Failed to communicate with Claude"},
		{"ErrGitOperation", atlaserrors.ErrGitOperation, "Git operation failed"},
		{"ErrGitHubOperation", atlaserrors.ErrGitHubOperation, "GitHub operation failed"},
		{"ErrCIFailed", atlaserrors.ErrCIFailed, "CI workflow failed"},
		{"ErrCITimeout", atlaserrors.ErrCITimeout, "CI polling timed out"},
		{"ErrUserRejected", atlaserrors.ErrUserRejected, "Task was rejected"},
		{"ErrUserAbandoned", atlaserrors.ErrUserAbandoned, "Task was abandoned"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := atlaserrors.UserMessage(tc.err)
			assert.Contains(t, msg, tc.contains)
		})
	}
}

func TestUserMessage_WrappedErrors(t *testing.T) {
	// UserMessage should work with wrapped errors too
	wrapped := atlaserrors.Wrap(atlaserrors.ErrGitOperation, "failed to create worktree")
	msg := atlaserrors.UserMessage(wrapped)

	assert.Contains(t, msg, "Git operation failed")
}

func TestUserMessage_NilError(t *testing.T) {
	msg := atlaserrors.UserMessage(nil)
	assert.Empty(t, msg)
}

func TestUserMessage_UnknownError(t *testing.T) {
	// Create an error that doesn't match any sentinel to test the default branch
	unknownErr := testError{msg: "some unexpected error occurred"}
	msg := atlaserrors.UserMessage(unknownErr)

	// Default case returns err.Error() directly
	assert.Equal(t, "some unexpected error occurred", msg)
}

func TestActionable_AllSentinels(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		containsMsg    string
		containsAction string
	}{
		{"ErrValidationFailed", atlaserrors.ErrValidationFailed, "Validation failed", "Fix the issues"},
		{"ErrClaudeInvocation", atlaserrors.ErrClaudeInvocation, "Claude", "ANTHROPIC_API_KEY"},
		{"ErrGitOperation", atlaserrors.ErrGitOperation, "Git operation", "clean git state"},
		{"ErrGitHubOperation", atlaserrors.ErrGitHubOperation, "GitHub operation", "GH_TOKEN"},
		{"ErrCIFailed", atlaserrors.ErrCIFailed, "CI workflow failed", "Review the CI logs"},
		{"ErrCITimeout", atlaserrors.ErrCITimeout, "timed out", "atlas status"},
		{"ErrUserRejected", atlaserrors.ErrUserRejected, "rejected", "retry"},
		{"ErrUserAbandoned", atlaserrors.ErrUserAbandoned, "abandoned", "atlas workspace list"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, action := atlaserrors.Actionable(tc.err)
			assert.Contains(t, msg, tc.containsMsg)
			assert.Contains(t, action, tc.containsAction)
		})
	}
}

func TestActionable_WrappedErrors(t *testing.T) {
	wrapped := atlaserrors.Wrap(atlaserrors.ErrCIFailed, "run 123 failed")
	msg, action := atlaserrors.Actionable(wrapped)

	assert.Contains(t, msg, "CI workflow failed")
	assert.Contains(t, action, "Review the CI logs")
}

func TestActionable_NilError(t *testing.T) {
	msg, action := atlaserrors.Actionable(nil)
	assert.Empty(t, msg)
	assert.Empty(t, action)
}

func TestActionable_UnknownError(t *testing.T) {
	// Create an error that doesn't match any sentinel to test the default branch
	unknownErr := testError{msg: "unexpected database connection error"}
	msg, action := atlaserrors.Actionable(unknownErr)

	// Default case returns err.Error() for message and empty action
	assert.Equal(t, "unexpected database connection error", msg)
	assert.Empty(t, action, "unknown errors should have no suggested action")
}

func TestExitCode2Error_Creation(t *testing.T) {
	baseErr := atlaserrors.ErrUserRejected
	exitErr := atlaserrors.NewExitCode2Error(baseErr)

	require.NotNil(t, exitErr)
	assert.Equal(t, baseErr.Error(), exitErr.Error())
}

func TestExitCode2Error_Unwrap(t *testing.T) {
	baseErr := atlaserrors.ErrValidationFailed
	exitErr := atlaserrors.NewExitCode2Error(baseErr)

	unwrapped := exitErr.Unwrap()
	assert.Equal(t, baseErr, unwrapped)
}

func TestExitCode2Error_ErrorsIs(t *testing.T) {
	baseErr := atlaserrors.ErrGitOperation
	exitErr := atlaserrors.NewExitCode2Error(baseErr)

	// Should match the base error through unwrap
	require.ErrorIs(t, exitErr, baseErr)
}

func TestIsExitCode2Error_True(t *testing.T) {
	baseErr := atlaserrors.ErrCIFailed
	exitErr := atlaserrors.NewExitCode2Error(baseErr)

	assert.True(t, atlaserrors.IsExitCode2Error(exitErr))
}

func TestIsExitCode2Error_False(t *testing.T) {
	regularErr := atlaserrors.ErrValidationFailed

	assert.False(t, atlaserrors.IsExitCode2Error(regularErr))
}

func TestIsExitCode2Error_WrappedExitCode2(t *testing.T) {
	baseErr := atlaserrors.ErrUserAbandoned
	exitErr := atlaserrors.NewExitCode2Error(baseErr)
	wrappedErr := atlaserrors.Wrap(exitErr, "additional context")

	// Should still detect ExitCode2Error through the wrap chain
	assert.True(t, atlaserrors.IsExitCode2Error(wrappedErr))
}

func TestIsExitCode2Error_Nil(t *testing.T) {
	assert.False(t, atlaserrors.IsExitCode2Error(nil))
}

// TestUserMessage_NewErrorMappings tests the newly added error message mappings.
func TestUserMessage_NewErrorMappings(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		// AI Errors
		{"ErrGeminiInvocation", atlaserrors.ErrGeminiInvocation, "Gemini"},
		{"ErrCodexInvocation", atlaserrors.ErrCodexInvocation, "Codex"},
		{"ErrAgentNotFound", atlaserrors.ErrAgentNotFound, "agent is not available"},
		{"ErrAgentNotInstalled", atlaserrors.ErrAgentNotInstalled, "CLI is not installed"},
		{"ErrAIError", atlaserrors.ErrAIError, "AI returned an error"},
		{"ErrAIEmptyResponse", atlaserrors.ErrAIEmptyResponse, "empty response"},
		{"ErrMaxRetriesExceeded", atlaserrors.ErrMaxRetriesExceeded, "Maximum retry"},

		// Git Errors
		{"ErrNotInGitRepo", atlaserrors.ErrNotInGitRepo, "git repository"},
		{"ErrBranchExists", atlaserrors.ErrBranchExists, "already exists"},
		{"ErrBranchNotFound", atlaserrors.ErrBranchNotFound, "does not exist"},
		{"ErrWorktreeExists", atlaserrors.ErrWorktreeExists, "worktree already exists"},
		{"ErrWorktreeDirty", atlaserrors.ErrWorktreeDirty, "uncommitted changes"},
		{"ErrPushAuthFailed", atlaserrors.ErrPushAuthFailed, "authentication"},
		{"ErrPushNetworkFailed", atlaserrors.ErrPushNetworkFailed, "network"},

		// GitHub Errors
		{"ErrGHAuthFailed", atlaserrors.ErrGHAuthFailed, "authentication failed"},
		{"ErrGHRateLimited", atlaserrors.ErrGHRateLimited, "rate limit"},
		{"ErrPRCreationFailed", atlaserrors.ErrPRCreationFailed, "create pull request"},
		{"ErrPRNotFound", atlaserrors.ErrPRNotFound, "not found"},

		// Workspace/Task Errors
		{"ErrWorkspaceExists", atlaserrors.ErrWorkspaceExists, "already exists"},
		{"ErrWorkspaceNotFound", atlaserrors.ErrWorkspaceNotFound, "not found"},
		{"ErrTaskNotFound", atlaserrors.ErrTaskNotFound, "not found"},
		{"ErrInvalidTransition", atlaserrors.ErrInvalidTransition, "Cannot transition"},
		{"ErrTaskInterrupted", atlaserrors.ErrTaskInterrupted, "interrupted"},

		// Configuration Errors
		{"ErrConfigNotFound", atlaserrors.ErrConfigNotFound, "not found"},
		{"ErrConfigInvalidAI", atlaserrors.ErrConfigInvalidAI, "Invalid AI"},
		{"ErrInvalidModel", atlaserrors.ErrInvalidModel, "Invalid AI model"},
		{"ErrEmptyValue", atlaserrors.ErrEmptyValue, "required value"},

		// Template Errors
		{"ErrTemplateNotFound", atlaserrors.ErrTemplateNotFound, "does not exist"},
		{"ErrTemplateRequired", atlaserrors.ErrTemplateRequired, "must be specified"},
		{"ErrVariableRequired", atlaserrors.ErrVariableRequired, "variable was not provided"},

		// Upgrade Errors
		{"ErrUpgradeNoRelease", atlaserrors.ErrUpgradeNoRelease, "No release found"},
		{"ErrUpgradeDownloadFailed", atlaserrors.ErrUpgradeDownloadFailed, "download"},
		{"ErrUpgradeChecksumMismatch", atlaserrors.ErrUpgradeChecksumMismatch, "checksum"},

		// Misc Errors
		{"ErrUnsupportedOS", atlaserrors.ErrUnsupportedOS, "not supported"},
		{"ErrConflictingFlags", atlaserrors.ErrConflictingFlags, "cannot be used together"},
		{"ErrCommandTimeout", atlaserrors.ErrCommandTimeout, "timed out"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := atlaserrors.UserMessage(tc.err)
			assert.Contains(t, msg, tc.contains,
				"UserMessage for %s should contain %q, got %q", tc.name, tc.contains, msg)
		})
	}
}

// TestActionable_NewErrorMappings tests actionable messages for new error mappings.
func TestActionable_NewErrorMappings(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		containsAction string
	}{
		// AI Errors with specific actions
		{"ErrAgentNotInstalled", atlaserrors.ErrAgentNotInstalled, "Install"},
		{"ErrMaxRetriesExceeded", atlaserrors.ErrMaxRetriesExceeded, "fix issues manually"},

		// Git Errors with specific actions
		{"ErrNotInGitRepo", atlaserrors.ErrNotInGitRepo, "git init"},
		{"ErrBranchExists", atlaserrors.ErrBranchExists, "different branch name"},
		{"ErrWorktreeDirty", atlaserrors.ErrWorktreeDirty, "Commit or stash"},
		{"ErrRebaseConflict", atlaserrors.ErrRebaseConflict, "Resolve conflicts"},

		// GitHub Errors with specific actions
		{"ErrGHAuthFailed", atlaserrors.ErrGHAuthFailed, "gh auth login"},
		{"ErrGHRateLimited", atlaserrors.ErrGHRateLimited, "Wait"},

		// Workspace/Task Errors with specific actions
		{"ErrWorkspaceNotFound", atlaserrors.ErrWorkspaceNotFound, "atlas workspace list"},
		{"ErrTaskNotFound", atlaserrors.ErrTaskNotFound, "atlas status"},
		{"ErrTaskInterrupted", atlaserrors.ErrTaskInterrupted, "atlas resume"},
		{"ErrLockTimeout", atlaserrors.ErrLockTimeout, "Wait and try again"},

		// Configuration Errors with specific actions
		{"ErrConfigNotFound", atlaserrors.ErrConfigNotFound, "atlas.yaml"},
		{"ErrInvalidDuration", atlaserrors.ErrInvalidDuration, "30s"},

		// Template Errors with specific actions
		{"ErrTemplateNotFound", atlaserrors.ErrTemplateNotFound, "atlas template list"},
		{"ErrTemplateRequired", atlaserrors.ErrTemplateRequired, "--template"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, action := atlaserrors.Actionable(tc.err)
			assert.Contains(t, action, tc.containsAction,
				"Action for %s should contain %q, got %q", tc.name, tc.containsAction, action)
		})
	}
}

// TestActionable_CanceledErrorsHaveNoAction verifies canceled errors have empty actions.
func TestActionable_CanceledErrorsHaveNoAction(t *testing.T) {
	canceledErrors := []error{
		atlaserrors.ErrOperationCanceled,
		atlaserrors.ErrMenuCanceled,
	}

	for _, err := range canceledErrors {
		t.Run(err.Error(), func(t *testing.T) {
			_, action := atlaserrors.Actionable(err)
			assert.Empty(t, action, "Canceled errors should have no suggested action")
		})
	}
}
