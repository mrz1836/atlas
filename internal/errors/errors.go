// Package errors provides centralized error handling for ATLAS.
//
// This package defines sentinel errors used for programmatic error categorization
// throughout the application. All error types can be checked using errors.Is().
//
// IMPORTANT: This package MUST NOT import any other internal packages.
// Only standard library imports are allowed.
package errors

import "errors"

// Sentinel errors for error categorization.
// These allow callers to check error types with errors.Is().
// All errors use lowercase descriptions per Go conventions.
var (
	// ErrValidationFailed indicates that one or more validation commands
	// (lint, test, build) failed during task execution.
	ErrValidationFailed = errors.New("validation failed")

	// ErrClaudeInvocation indicates that the Claude Code CLI failed to execute
	// or returned a non-zero exit code.
	ErrClaudeInvocation = errors.New("claude invocation failed")

	// ErrGitOperation indicates that a git command (clone, worktree, commit, etc.)
	// failed during execution.
	ErrGitOperation = errors.New("git operation failed")

	// ErrGitHubOperation indicates that a GitHub API operation (PR creation,
	// CI status check, etc.) failed.
	ErrGitHubOperation = errors.New("github operation failed")

	// ErrCIFailed indicates that the CI workflow completed but one or more
	// checks did not pass.
	ErrCIFailed = errors.New("ci workflow failed")

	// ErrCITimeout indicates that CI status polling exceeded the configured
	// timeout duration.
	ErrCITimeout = errors.New("ci polling timeout")

	// ErrUserRejected indicates that the user explicitly rejected the current
	// task result during the approval step.
	ErrUserRejected = errors.New("user rejected")

	// ErrUserAbandoned indicates that the user chose to abandon the task
	// entirely rather than retry or provide feedback.
	ErrUserAbandoned = errors.New("user abandoned task")

	// ErrConfigNil indicates that a nil config was passed to validation.
	ErrConfigNil = errors.New("config is nil")

	// ErrConfigInvalidAI indicates an invalid AI configuration value.
	ErrConfigInvalidAI = errors.New("invalid AI configuration")

	// ErrConfigInvalidGit indicates an invalid Git configuration value.
	ErrConfigInvalidGit = errors.New("invalid Git configuration")

	// ErrConfigInvalidCI indicates an invalid CI configuration value.
	ErrConfigInvalidCI = errors.New("invalid CI configuration")

	// ErrConfigInvalidValidation indicates an invalid Validation configuration value.
	ErrConfigInvalidValidation = errors.New("invalid Validation configuration")

	// ErrInvalidOutputFormat indicates an invalid output format was specified.
	ErrInvalidOutputFormat = errors.New("invalid output format")

	// ErrCommandNotConfigured indicates that a mock command was not configured in tests.
	ErrCommandNotConfigured = errors.New("command not configured")

	// ErrCommandFailed indicates that a command execution failed.
	ErrCommandFailed = errors.New("command failed")

	// ErrConfigNotFound indicates that the configuration file was not found.
	ErrConfigNotFound = errors.New("config file not found")

	// ErrEmptyValue indicates that a required value was empty.
	ErrEmptyValue = errors.New("value cannot be empty")

	// ErrInvalidEnvVarName indicates that an environment variable name is invalid.
	ErrInvalidEnvVarName = errors.New("invalid environment variable name")

	// ErrInvalidDuration indicates that a duration format is invalid.
	ErrInvalidDuration = errors.New("invalid duration format")

	// ErrValueOutOfRange indicates that a value is outside the allowed range.
	ErrValueOutOfRange = errors.New("value out of range")

	// ErrInvalidModel indicates that an AI model name is invalid.
	ErrInvalidModel = errors.New("invalid model")

	// ErrUnknownTool indicates that an unknown tool name was specified.
	ErrUnknownTool = errors.New("unknown tool")

	// ErrInvalidToolName indicates that an invalid tool name was specified.
	ErrInvalidToolName = errors.New("invalid tool name")

	// ErrMissingRequiredTools indicates that required tools are missing or outdated.
	ErrMissingRequiredTools = errors.New("required tools are missing or outdated")

	// ErrNotInProjectDir indicates that --project flag was used but not in a project directory.
	ErrNotInProjectDir = errors.New("not in a project directory")

	// ErrNotInGitRepo indicates that a git repository is required but not found.
	ErrNotInGitRepo = errors.New("not in a git repository")

	// ErrUnsupportedOutputFormat indicates that an unsupported output format was specified.
	ErrUnsupportedOutputFormat = errors.New("unsupported output format")

	// ErrWorkspaceExists indicates an attempt to create a workspace that already exists.
	ErrWorkspaceExists = errors.New("workspace already exists")

	// ErrWorkspaceNotFound indicates the requested workspace does not exist.
	ErrWorkspaceNotFound = errors.New("workspace not found")

	// ErrWorkspaceCorrupted indicates the workspace state file is corrupted or unreadable.
	ErrWorkspaceCorrupted = errors.New("workspace state corrupted")

	// ErrLockTimeout indicates a file lock could not be acquired within the timeout period.
	ErrLockTimeout = errors.New("lock acquisition timeout")

	// ErrWorktreeExists indicates the worktree path already exists.
	ErrWorktreeExists = errors.New("worktree already exists")

	// ErrNotAWorktree indicates the path is not a valid git worktree.
	ErrNotAWorktree = errors.New("not a git worktree")

	// ErrWorktreeDirty indicates the worktree has uncommitted changes.
	ErrWorktreeDirty = errors.New("worktree has uncommitted changes")

	// ErrBranchExists indicates the branch already exists.
	ErrBranchExists = errors.New("branch already exists")

	// ErrBranchNotFound indicates the specified branch does not exist locally or remotely.
	ErrBranchNotFound = errors.New("branch not found")

	// ErrNotGitRepo indicates the path is not a git repository.
	ErrNotGitRepo = errors.New("not a git repository")

	// ErrWorkspaceHasRunningTasks indicates the workspace has tasks still running.
	ErrWorkspaceHasRunningTasks = errors.New("workspace has running tasks")

	// ErrNonInteractiveMode indicates that an operation requiring confirmation
	// was attempted in non-interactive mode without the force flag.
	ErrNonInteractiveMode = errors.New("use --force in non-interactive mode")

	// ErrJSONErrorOutput indicates that an error has already been output as JSON.
	// This ensures a non-zero exit code while preventing duplicate error messages.
	// Commands should silence cobra's error printing when this is returned.
	ErrJSONErrorOutput = errors.New("error output as JSON")

	// ErrNoTasksFound indicates that no tasks exist for a workspace.
	ErrNoTasksFound = errors.New("no tasks found")

	// ErrTaskNotFound indicates that a specific task was not found in a workspace.
	ErrTaskNotFound = errors.New("task not found")

	// ErrTaskExists indicates an attempt to create a task that already exists.
	ErrTaskExists = errors.New("task already exists")

	// ErrPathTraversal indicates an attempt to use path traversal in a filename.
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrTooManyVersions indicates too many versioned artifacts exist.
	ErrTooManyVersions = errors.New("too many versions")

	// ErrArtifactNotFound indicates the requested artifact file was not found.
	ErrArtifactNotFound = errors.New("artifact not found")

	// ErrInvalidTransition indicates an attempt to make an invalid state transition.
	ErrInvalidTransition = errors.New("invalid state transition")

	// ErrTemplateNotFound indicates the requested template does not exist in the registry.
	ErrTemplateNotFound = errors.New("template not found")

	// ErrTemplateNil indicates a nil template was provided.
	ErrTemplateNil = errors.New("template cannot be nil")

	// ErrTemplateNameEmpty indicates a template has an empty name.
	ErrTemplateNameEmpty = errors.New("template name is required")

	// ErrTemplateDuplicate indicates a template with the same name already exists.
	ErrTemplateDuplicate = errors.New("template already registered")

	// ErrVariableRequired indicates a required template variable was not provided.
	ErrVariableRequired = errors.New("required variable not provided")

	// ErrExecutorNotFound indicates no executor is registered for the given step type.
	ErrExecutorNotFound = errors.New("executor not found for step type")

	// ErrUnknownStepResultStatus indicates an unknown step result status was returned.
	ErrUnknownStepResultStatus = errors.New("unknown step result status")

	// ErrTemplateRequired indicates a template flag is required in non-interactive mode.
	ErrTemplateRequired = errors.New("template flag required in non-interactive mode")

	// ErrTemplateInvalid indicates a template failed validation.
	ErrTemplateInvalid = errors.New("invalid template")

	// ErrTemplateLoadFailed indicates a template file could not be loaded.
	ErrTemplateLoadFailed = errors.New("template load failed")

	// ErrTemplateFileMissing indicates the template file does not exist.
	ErrTemplateFileMissing = errors.New("template file not found")

	// ErrTemplateParseError indicates the template file has invalid YAML/JSON syntax.
	ErrTemplateParseError = errors.New("template parse error")

	// ErrOperationCanceled indicates the user canceled an operation.
	ErrOperationCanceled = errors.New("operation canceled by user")

	// ErrResumeNotImplemented indicates the resume feature is not yet implemented.
	ErrResumeNotImplemented = errors.New("resume not yet implemented")

	// ErrUserInputRequired indicates user input is required but not provided.
	// Commands should exit with code 2 when this error is returned.
	ErrUserInputRequired = errors.New("user input required")

	// ErrCommandTimeout indicates a command exceeded its timeout duration.
	ErrCommandTimeout = errors.New("command timeout exceeded")

	// ErrMaxRetriesExceeded indicates the maximum retry attempts have been reached.
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")

	// ErrRetryDisabled indicates that AI retry is disabled in configuration.
	ErrRetryDisabled = errors.New("AI retry is disabled")

	// ErrAIError indicates that the AI returned an error.
	ErrAIError = errors.New("AI returned error")

	// ErrAIEmptyResponse indicates that the AI returned an empty response.
	ErrAIEmptyResponse = errors.New("AI returned empty response")

	// ErrAIInvalidFormat indicates that the AI response was not in the expected format.
	ErrAIInvalidFormat = errors.New("AI response not in expected format")

	// ErrPushAuthFailed indicates that git push failed due to authentication.
	ErrPushAuthFailed = errors.New("push authentication failed")

	// ErrPushNetworkFailed indicates that git push failed due to network issues.
	ErrPushNetworkFailed = errors.New("push network failed")

	// ErrPRCreationFailed indicates that PR creation failed.
	ErrPRCreationFailed = errors.New("PR creation failed")

	// ErrGHRateLimited indicates that GitHub API rate limit was exceeded.
	ErrGHRateLimited = errors.New("GitHub API rate limited")

	// ErrGHAuthFailed indicates that GitHub authentication failed.
	ErrGHAuthFailed = errors.New("GitHub authentication failed")

	// ErrPRNotFound indicates that the requested PR was not found.
	ErrPRNotFound = errors.New("PR not found")

	// ErrCICheckNotFound indicates that a required CI check was not found.
	ErrCICheckNotFound = errors.New("required CI check not found")

	// ErrUnsupportedOS indicates the current operating system is not supported.
	ErrUnsupportedOS = errors.New("unsupported operating system")

	// ErrInvalidVerificationAction indicates an unknown verification action was specified.
	ErrInvalidVerificationAction = errors.New("invalid verification action")

	// ErrConflictingFlags indicates that mutually exclusive flags were specified.
	ErrConflictingFlags = errors.New("conflicting flags specified")

	// ErrWatchIntervalTooShort indicates that the watch interval is below minimum.
	ErrWatchIntervalTooShort = errors.New("watch interval too short")

	// ErrWatchModeJSONUnsupported indicates that watch mode does not support JSON output.
	ErrWatchModeJSONUnsupported = errors.New("watch mode does not support JSON output")

	// ErrNoMenuOptions indicates that no options were provided to a menu.
	ErrNoMenuOptions = errors.New("no menu options provided")

	// ErrMenuCanceled indicates that the user canceled a menu operation.
	ErrMenuCanceled = errors.New("menu canceled by user")

	// ErrInvalidArgument indicates that an invalid argument was provided.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrInvalidStatus indicates that a task is in an invalid status for the operation.
	ErrInvalidStatus = errors.New("invalid task status")

	// ErrApprovalRequired indicates that approval is required but --auto-approve was not provided.
	ErrApprovalRequired = errors.New("approval required")

	// ErrInteractiveRequired indicates that interactive prompts are required but not available.
	ErrInteractiveRequired = errors.New("interactive prompt required")

	// ErrWorktreeNotFound indicates the requested worktree does not exist.
	ErrWorktreeNotFound = errors.New("worktree not found")

	// ErrRebaseConflict indicates that a rebase operation has conflicts that need manual resolution.
	ErrRebaseConflict = errors.New("rebase has conflicts")
)

// ExitCode2Error wraps an error to indicate exit code 2 should be used.
type ExitCode2Error struct {
	Err error
}

// NewExitCode2Error wraps an error to indicate exit code 2.
func NewExitCode2Error(err error) *ExitCode2Error {
	return &ExitCode2Error{Err: err}
}

// Error implements the error interface.
func (e *ExitCode2Error) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *ExitCode2Error) Unwrap() error {
	return e.Err
}

// IsExitCode2Error checks if an error should result in exit code 2.
func IsExitCode2Error(err error) bool {
	var e *ExitCode2Error
	return errors.As(err, &e)
}
