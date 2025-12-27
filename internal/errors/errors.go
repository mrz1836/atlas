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
)
