// Package constants provides centralized constant values used throughout ATLAS.
// This package is the single source of truth for all shared constants and MUST NOT
// import any other internal packages.
package constants

import "time"

// File names used by ATLAS for state persistence.
const (
	// TaskFileName is the name of the JSON file that stores task state within a workspace.
	TaskFileName = "task.json"

	// WorkspaceFileName is the name of the JSON file that stores workspace metadata.
	WorkspaceFileName = "workspace.json"
)

// Directory names and paths used by ATLAS for organizing data.
const (
	// AtlasHome is the hidden directory name where ATLAS stores all its data.
	// This directory is created in the user's home directory.
	AtlasHome = ".atlas"

	// WorkspacesDir is the directory name where workspace data is stored.
	WorkspacesDir = "workspaces"

	// TasksDir is the directory name where task-related files are stored.
	TasksDir = "tasks"

	// ArtifactsDir is the directory name where build and execution artifacts are stored.
	ArtifactsDir = "artifacts"

	// LogsDir is the directory name where log files are stored.
	LogsDir = "logs"
)

// Timeout configurations for various operations.
const (
	// DefaultAITimeout is the default maximum duration for AI execution operations.
	// AI operations include Claude Code execution and other LLM interactions.
	DefaultAITimeout = 30 * time.Minute

	// DefaultCITimeout is the default maximum duration for CI pipeline operations.
	// This includes waiting for CI builds, tests, and deployments to complete.
	DefaultCITimeout = 30 * time.Minute

	// CIPollInterval is the interval at which ATLAS polls CI status.
	CIPollInterval = 2 * time.Minute

	// CIInitialGracePeriod is the time to wait for CI checks to appear after PR creation.
	// GitHub Actions typically start within 30 seconds but can take up to 2 minutes.
	CIInitialGracePeriod = 2 * time.Minute

	// CIGracePollInterval is the polling interval during the initial grace period.
	// More frequent than normal polling since we're waiting for checks to appear.
	CIGracePollInterval = 10 * time.Second
)

// Retry configuration defaults for recoverable operations.
const (
	// MaxRetryAttempts is the maximum number of retry attempts for recoverable errors.
	MaxRetryAttempts = 3

	// InitialBackoff is the initial backoff duration before the first retry.
	// Subsequent retries may use exponential backoff based on this value.
	InitialBackoff = 1 * time.Second
)

// Schema version constants for data migration support.
const (
	// TaskSchemaVersion is the current version of the task JSON schema.
	// This enables forward-compatible schema migrations.
	TaskSchemaVersion = "1.0"
)

// Default validation commands used when no configuration is provided.
const (
	// DefaultFormatCommand is the default command for code formatting.
	DefaultFormatCommand = "magex format:fix"

	// DefaultLintCommand is the default command for linting.
	DefaultLintCommand = "magex lint"

	// DefaultTestCommand is the default command for running tests.
	DefaultTestCommand = "magex test"

	// DefaultPreCommitCommand is the default command for pre-commit hooks.
	DefaultPreCommitCommand = "go-pre-commit run --all-files"
)

// Log rotation configuration constants.
const (
	// LogMaxSizeMB is the maximum size in megabytes of the log file before it gets rotated.
	LogMaxSizeMB = 10

	// LogMaxBackups is the maximum number of old log files to retain.
	LogMaxBackups = 5

	// LogMaxAgeDays is the maximum number of days to retain old log files.
	LogMaxAgeDays = 30

	// LogCompress indicates whether the rotated log files should be compressed using gzip.
	LogCompress = true
)

// Step result status constants used by step executors.
const (
	// StepStatusSuccess indicates the step completed successfully.
	StepStatusSuccess = "success"

	// StepStatusFailed indicates the step failed.
	StepStatusFailed = "failed"

	// StepStatusPending indicates the step has not started yet.
	StepStatusPending = "pending"

	// StepStatusRunning indicates the step is currently executing.
	StepStatusRunning = "running"

	// StepStatusAwaitingApproval indicates the step requires user approval.
	StepStatusAwaitingApproval = "awaiting_approval"

	// StepStatusNoChanges indicates the step completed but made no changes.
	// This is used by git commit when there are no files to commit.
	// The engine should skip subsequent git push/PR steps when this status is returned.
	StepStatusNoChanges = "no_changes"

	// StepStatusSkipped indicates the step was skipped.
	// This is used when a step is intentionally not executed (e.g., git push/PR when no changes).
	StepStatusSkipped = "skipped"
)
