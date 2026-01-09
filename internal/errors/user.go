package errors

import "errors"

// ErrorInfo holds user-facing message and suggested action for an error.
type ErrorInfo struct {
	// Message is the user-friendly error description.
	Message string
	// Action is a suggested action to resolve the issue (empty if none).
	Action string
}

// errorEntry pairs a sentinel error with its user-facing info.
type errorEntry struct {
	err  error
	info ErrorInfo
}

// errorInfoEntries is the pre-built mapping of sentinel errors to their user-facing messages.
// This single source of truth ensures UserMessage and Actionable stay in sync.
// Using a slice (not a map) because errors.Is() requires proper error chain traversal.
//
//nolint:gochecknoglobals // Pre-built mapping for efficiency
var errorInfoEntries = []errorEntry{
	// ===================
	// Validation & CI
	// ===================
	{
		err: ErrValidationFailed,
		info: ErrorInfo{
			Message: "Validation failed. Check the output above for specific errors.",
			Action:  "Fix the issues reported by the validation commands and retry.",
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
		err: ErrCIFetchFailed,
		info: ErrorInfo{
			Message: "Could not fetch CI status from GitHub.",
			Action:  "Check your network connection and GH_TOKEN permissions.",
		},
	},
	{
		err: ErrCICheckNotFound,
		info: ErrorInfo{
			Message: "Required CI check was not found.",
			Action:  "Verify the workflow is configured in .github/workflows/ and the check name matches.",
		},
	},

	// ===================
	// AI Invocation
	// ===================
	{
		err: ErrClaudeInvocation,
		info: ErrorInfo{
			Message: "Failed to communicate with Claude. Check your API key and network.",
			Action:  "Verify ANTHROPIC_API_KEY is set correctly and you have network access.",
		},
	},
	{
		err: ErrGeminiInvocation,
		info: ErrorInfo{
			Message: "Failed to communicate with Gemini. Check your API key and network.",
			Action:  "Verify GOOGLE_API_KEY is set correctly and you have network access.",
		},
	},
	{
		err: ErrCodexInvocation,
		info: ErrorInfo{
			Message: "Failed to communicate with Codex. Check your API key and network.",
			Action:  "Verify OPENAI_API_KEY is set correctly and you have network access.",
		},
	},
	{
		err: ErrAgentNotFound,
		info: ErrorInfo{
			Message: "The specified AI agent is not available.",
			Action:  "Check available agents with 'atlas config ai' or use --agent flag.",
		},
	},
	{
		err: ErrAgentNotInstalled,
		info: ErrorInfo{
			Message: "The AI agent CLI is not installed.",
			Action:  "Install the required CLI tool (e.g., 'npm install -g @anthropic-ai/claude-code').",
		},
	},
	{
		err: ErrAIError,
		info: ErrorInfo{
			Message: "The AI returned an error response.",
			Action:  "Review the error details and try again. Consider simplifying the request.",
		},
	},
	{
		err: ErrAIEmptyResponse,
		info: ErrorInfo{
			Message: "The AI returned an empty response.",
			Action:  "Try again. If the issue persists, check your API key and quota.",
		},
	},
	{
		err: ErrAIInvalidFormat,
		info: ErrorInfo{
			Message: "The AI response was not in the expected format.",
			Action:  "This may be a temporary issue. Try again.",
		},
	},
	{
		err: ErrMaxRetriesExceeded,
		info: ErrorInfo{
			Message: "Maximum retry attempts reached.",
			Action:  "Review the errors, fix issues manually, or increase retry limit in config.",
		},
	},
	{
		err: ErrRetryDisabled,
		info: ErrorInfo{
			Message: "AI retry is disabled in configuration.",
			Action:  "Enable retry with 'atlas config ai --retry-enabled true'.",
		},
	},

	// ===================
	// Git Operations
	// ===================
	{
		err: ErrGitOperation,
		info: ErrorInfo{
			Message: "Git operation failed. Check your repository state.",
			Action:  "Ensure you have a clean git state and proper permissions.",
		},
	},
	{
		err: ErrNotInGitRepo,
		info: ErrorInfo{
			Message: "This command must be run from within a git repository.",
			Action:  "Navigate to a git repository or run 'git init' to create one.",
		},
	},
	{
		err: ErrNotGitRepo,
		info: ErrorInfo{
			Message: "The specified path is not a git repository.",
			Action:  "Ensure the path is correct and contains a .git directory.",
		},
	},
	{
		err: ErrBranchExists,
		info: ErrorInfo{
			Message: "A branch with this name already exists.",
			Action:  "Choose a different branch name or delete the existing branch first.",
		},
	},
	{
		err: ErrBranchNotFound,
		info: ErrorInfo{
			Message: "The specified branch does not exist.",
			Action:  "Check the branch name with 'git branch -a' or create it first.",
		},
	},
	{
		err: ErrWorktreeExists,
		info: ErrorInfo{
			Message: "A worktree already exists at this path.",
			Action:  "Remove the existing worktree with 'git worktree remove <path>'.",
		},
	},
	{
		err: ErrWorktreeNotFound,
		info: ErrorInfo{
			Message: "The specified worktree does not exist.",
			Action:  "Check available worktrees with 'git worktree list'.",
		},
	},
	{
		err: ErrNotAWorktree,
		info: ErrorInfo{
			Message: "The specified path is not a git worktree.",
			Action:  "Ensure the path points to a valid git worktree directory.",
		},
	},
	{
		err: ErrWorktreeDirty,
		info: ErrorInfo{
			Message: "The worktree has uncommitted changes.",
			Action:  "Commit or stash your changes before proceeding.",
		},
	},
	{
		err: ErrRebaseConflict,
		info: ErrorInfo{
			Message: "Rebase has conflicts that need manual resolution.",
			Action:  "Resolve conflicts in the worktree, then 'git rebase --continue'.",
		},
	},
	{
		err: ErrPushAuthFailed,
		info: ErrorInfo{
			Message: "Git push failed due to authentication error.",
			Action:  "Check your SSH keys or GH_TOKEN permissions for the repository.",
		},
	},
	{
		err: ErrPushNetworkFailed,
		info: ErrorInfo{
			Message: "Git push failed due to network error.",
			Action:  "Check your internet connection and try again.",
		},
	},

	// ===================
	// GitHub Operations
	// ===================
	{
		err: ErrGitHubOperation,
		info: ErrorInfo{
			Message: "GitHub operation failed. Check your authentication and permissions.",
			Action:  "Verify GH_TOKEN is set and has required repository permissions.",
		},
	},
	{
		err: ErrGHAuthFailed,
		info: ErrorInfo{
			Message: "GitHub authentication failed.",
			Action:  "Run 'gh auth login' or set GH_TOKEN environment variable.",
		},
	},
	{
		err: ErrGHRateLimited,
		info: ErrorInfo{
			Message: "GitHub API rate limit exceeded.",
			Action:  "Wait a few minutes and try again, or use authenticated requests.",
		},
	},
	{
		err: ErrPRCreationFailed,
		info: ErrorInfo{
			Message: "Failed to create pull request.",
			Action:  "Check that the branch is pushed and you have repo permissions.",
		},
	},
	{
		err: ErrPRNotFound,
		info: ErrorInfo{
			Message: "The specified pull request was not found.",
			Action:  "Verify the PR number and repository.",
		},
	},
	{
		err: ErrPRReviewNotAllowed,
		info: ErrorInfo{
			Message: "Cannot add a review to this pull request.",
			Action:  "You cannot review your own PR. Ask a teammate to review.",
		},
	},
	{
		err: ErrPRMergeFailed,
		info: ErrorInfo{
			Message: "Failed to merge the pull request.",
			Action:  "Ensure all required checks pass and there are no merge conflicts.",
		},
	},

	// ===================
	// Workspace & Task
	// ===================
	{
		err: ErrWorkspaceExists,
		info: ErrorInfo{
			Message: "A workspace with this name already exists.",
			Action:  "Use a different name or resume the existing workspace.",
		},
	},
	{
		err: ErrWorkspaceNotFound,
		info: ErrorInfo{
			Message: "The specified workspace was not found.",
			Action:  "Run 'atlas workspace list' to see available workspaces.",
		},
	},
	{
		err: ErrWorkspaceCorrupted,
		info: ErrorInfo{
			Message: "The workspace state file is corrupted.",
			Action:  "Delete the workspace with 'atlas workspace delete <name> --force'.",
		},
	},
	{
		err: ErrWorkspaceHasRunningTasks,
		info: ErrorInfo{
			Message: "The workspace has tasks still running.",
			Action:  "Wait for tasks to complete or use 'atlas workspace delete --force'.",
		},
	},
	{
		err: ErrTaskNotFound,
		info: ErrorInfo{
			Message: "The specified task was not found.",
			Action:  "Run 'atlas status' to see current tasks.",
		},
	},
	{
		err: ErrNoTasksFound,
		info: ErrorInfo{
			Message: "No tasks found for this workspace.",
			Action:  "Start a new task with 'atlas start'.",
		},
	},
	{
		err: ErrTaskExists,
		info: ErrorInfo{
			Message: "A task with this ID already exists.",
			Action:  "Use 'atlas resume' to continue the existing task.",
		},
	},
	{
		err: ErrInvalidTransition,
		info: ErrorInfo{
			Message: "Cannot transition task to this state.",
			Action:  "Run 'atlas status' to see current task state.",
		},
	},
	{
		err: ErrInvalidStatus,
		info: ErrorInfo{
			Message: "The task is in an invalid status for this operation.",
			Action:  "Check task status with 'atlas status' and try again.",
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
	{
		err: ErrTaskInterrupted,
		info: ErrorInfo{
			Message: "Task was interrupted. State has been saved.",
			Action:  "Resume with 'atlas resume <workspace>' when ready.",
		},
	},
	{
		err: ErrApprovalRequired,
		info: ErrorInfo{
			Message: "This operation requires approval.",
			Action:  "Run interactively or use --auto-approve flag.",
		},
	},
	{
		err: ErrLockTimeout,
		info: ErrorInfo{
			Message: "Could not acquire lock. Another process may be using the resource.",
			Action:  "Wait and try again, or check for stuck processes.",
		},
	},

	// ===================
	// Configuration
	// ===================
	{
		err: ErrConfigNotFound,
		info: ErrorInfo{
			Message: "Configuration file not found.",
			Action:  "Create an atlas.yaml file or run 'atlas init' in your project.",
		},
	},
	{
		err: ErrConfigNil,
		info: ErrorInfo{
			Message: "Configuration is not loaded.",
			Action:  "Ensure atlas.yaml exists and is valid YAML.",
		},
	},
	{
		err: ErrConfigInvalidAI,
		info: ErrorInfo{
			Message: "Invalid AI configuration.",
			Action:  "Check the 'ai' section in atlas.yaml for invalid values.",
		},
	},
	{
		err: ErrConfigInvalidGit,
		info: ErrorInfo{
			Message: "Invalid Git configuration.",
			Action:  "Check the 'git' section in atlas.yaml for invalid values.",
		},
	},
	{
		err: ErrConfigInvalidCI,
		info: ErrorInfo{
			Message: "Invalid CI configuration.",
			Action:  "Check the 'ci' section in atlas.yaml for invalid values.",
		},
	},
	{
		err: ErrConfigInvalidValidation,
		info: ErrorInfo{
			Message: "Invalid validation configuration.",
			Action:  "Check the 'validation' section in atlas.yaml for invalid values.",
		},
	},
	{
		err: ErrInvalidModel,
		info: ErrorInfo{
			Message: "Invalid AI model specified.",
			Action:  "Check available models for your AI provider.",
		},
	},
	{
		err: ErrInvalidDuration,
		info: ErrorInfo{
			Message: "Invalid duration format.",
			Action:  "Use formats like '30s', '5m', '1h' for durations.",
		},
	},
	{
		err: ErrValueOutOfRange,
		info: ErrorInfo{
			Message: "Value is outside the allowed range.",
			Action:  "Check the documentation for valid value ranges.",
		},
	},
	{
		err: ErrEmptyValue,
		info: ErrorInfo{
			Message: "A required value was not provided.",
			Action:  "Provide the required value and try again.",
		},
	},

	// ===================
	// Templates
	// ===================
	{
		err: ErrTemplateNotFound,
		info: ErrorInfo{
			Message: "The specified template does not exist.",
			Action:  "Run 'atlas template list' to see available templates.",
		},
	},
	{
		err: ErrTemplateRequired,
		info: ErrorInfo{
			Message: "A template must be specified in non-interactive mode.",
			Action:  "Use --template flag to specify a template.",
		},
	},
	{
		err: ErrTemplateInvalid,
		info: ErrorInfo{
			Message: "The template failed validation.",
			Action:  "Check the template file for syntax errors.",
		},
	},
	{
		err: ErrTemplateFileMissing,
		info: ErrorInfo{
			Message: "The template file does not exist.",
			Action:  "Check the file path and ensure the template file exists.",
		},
	},
	{
		err: ErrTemplateParseError,
		info: ErrorInfo{
			Message: "The template file has invalid YAML syntax.",
			Action:  "Check the template file for YAML syntax errors.",
		},
	},
	{
		err: ErrVariableRequired,
		info: ErrorInfo{
			Message: "A required template variable was not provided.",
			Action:  "Provide all required variables for the template.",
		},
	},

	// ===================
	// User Interaction
	// ===================
	{
		err: ErrOperationCanceled,
		info: ErrorInfo{
			Message: "Operation was canceled.",
			Action:  "",
		},
	},
	{
		err: ErrMenuCanceled,
		info: ErrorInfo{
			Message: "Menu selection was canceled.",
			Action:  "",
		},
	},
	{
		err: ErrUserInputRequired,
		info: ErrorInfo{
			Message: "This operation requires user input.",
			Action:  "Run in an interactive terminal or provide required flags.",
		},
	},
	{
		err: ErrInteractiveRequired,
		info: ErrorInfo{
			Message: "This operation requires an interactive terminal.",
			Action:  "Run in an interactive terminal, not in a script.",
		},
	},
	{
		err: ErrNonInteractiveMode,
		info: ErrorInfo{
			Message: "This operation requires confirmation in non-interactive mode.",
			Action:  "Use --force flag to skip confirmation.",
		},
	},
	{
		err: ErrCommandTimeout,
		info: ErrorInfo{
			Message: "Command execution timed out.",
			Action:  "Increase the timeout or check if the command is stuck.",
		},
	},

	// ===================
	// Upgrade
	// ===================
	{
		err: ErrUpgradeNoRelease,
		info: ErrorInfo{
			Message: "No release found for upgrade.",
			Action:  "Check GitHub releases or try again later.",
		},
	},
	{
		err: ErrUpgradeDownloadFailed,
		info: ErrorInfo{
			Message: "Failed to download the upgrade binary.",
			Action:  "Check your internet connection and try again.",
		},
	},
	{
		err: ErrUpgradeChecksumMismatch,
		info: ErrorInfo{
			Message: "Upgrade checksum verification failed.",
			Action:  "The download may be corrupted. Try again.",
		},
	},
	{
		err: ErrUpgradeReplaceFailed,
		info: ErrorInfo{
			Message: "Failed to replace the binary during upgrade.",
			Action:  "Ensure you have write permissions to the binary location.",
		},
	},
	{
		err: ErrUpgradeAssetNotFound,
		info: ErrorInfo{
			Message: "No binary available for your platform.",
			Action:  "Build from source or check for platform-specific releases.",
		},
	},

	// ===================
	// Misc
	// ===================
	{
		err: ErrUnsupportedOS,
		info: ErrorInfo{
			Message: "Your operating system is not supported for this operation.",
			Action:  "Atlas supports macOS, Linux, and Windows.",
		},
	},
	{
		err: ErrConflictingFlags,
		info: ErrorInfo{
			Message: "The specified flags cannot be used together.",
			Action:  "Check the command help for valid flag combinations.",
		},
	},
	{
		err: ErrInvalidArgument,
		info: ErrorInfo{
			Message: "An invalid argument was provided.",
			Action:  "Check the command help for valid arguments.",
		},
	},
	{
		err: ErrNotInProjectDir,
		info: ErrorInfo{
			Message: "This command must be run from a project directory.",
			Action:  "Navigate to a project directory or use --project flag.",
		},
	},
	{
		err: ErrMissingRequiredTools,
		info: ErrorInfo{
			Message: "Required tools are missing or outdated.",
			Action:  "Run 'atlas doctor' to check and install required tools.",
		},
	},
}

// errorInfoMap provides O(1) lookup for direct sentinel error matches.
// Built once from errorInfoEntries during package initialization.
//
//nolint:gochecknoglobals // Pre-built mapping for O(1) lookup performance
var errorInfoMap = buildErrorInfoMap()

// buildErrorInfoMap creates a map from the errorInfoEntries slice.
// This is called once during package init for O(1) direct lookups.
func buildErrorInfoMap() map[error]ErrorInfo {
	m := make(map[error]ErrorInfo, len(errorInfoEntries))
	for _, entry := range errorInfoEntries {
		m[entry.err] = entry.info
	}
	return m
}

// getErrorInfo looks up the ErrorInfo for a given error.
// It first tries O(1) direct map lookup for unwrapped sentinel errors,
// then falls back to errors.Is() traversal for wrapped errors.
// Returns an ErrorInfo with the original error message if not found.
func getErrorInfo(err error) ErrorInfo {
	// Fast path: O(1) lookup for direct sentinel errors
	if info, ok := errorInfoMap[err]; ok {
		return info
	}

	// Slow path: errors.Is() for wrapped errors
	for _, entry := range errorInfoEntries {
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
