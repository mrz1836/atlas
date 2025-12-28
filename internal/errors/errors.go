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

	// ErrNotGitRepo indicates the path is not a git repository.
	ErrNotGitRepo = errors.New("not a git repository")
)
