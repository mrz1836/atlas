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

	// GitCommitTimeout is the timeout for git commit operations.
	GitCommitTimeout = 1 * time.Minute

	// GitPushTimeout is the timeout for git push operations.
	GitPushTimeout = 2 * time.Minute

	// GitPRTimeout is the timeout for git PR creation operations.
	GitPRTimeout = 2 * time.Minute

	// ValidationStepTimeout is the timeout for validation step operations (format, lint, test).
	ValidationStepTimeout = 10 * time.Minute

	// WorkspaceLockTimeout is the maximum duration to wait for acquiring a workspace file lock.
	WorkspaceLockTimeout = 5 * time.Second
)

// Retry configuration defaults for recoverable operations.
const (
	// MaxRetryAttempts is the maximum number of retry attempts for recoverable errors.
	MaxRetryAttempts = 3

	// InitialBackoff is the initial backoff duration before the first retry.
	// Subsequent retries may use exponential backoff based on this value.
	InitialBackoff = 1 * time.Second

	// BackoffMultiplier is the factor by which backoff increases between retry attempts.
	// Used for exponential backoff: backoff *= BackoffMultiplier after each attempt.
	BackoffMultiplier = 2
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

// Git remote configuration.
const (
	// DefaultRemote is the default git remote name used for push/fetch operations.
	DefaultRemote = "origin"
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

// File permission constants for workspace and task files.
const (
	// WorkspaceDirPerm is the permission mode for workspace directories.
	// Owner has read/write/execute, group has read/execute.
	WorkspaceDirPerm = 0o750

	// WorkspaceFilePerm is the permission mode for workspace metadata files.
	// Owner has read/write only.
	WorkspaceFilePerm = 0o600
)

// Workspace name validation constants.
const (
	// MaxWorkspaceNameLength is the maximum allowed length for workspace names.
	MaxWorkspaceNameLength = 255
)

// Artifact filename constants for git operation results.
const (
	// ArtifactCommitResult is the filename for commit operation results.
	ArtifactCommitResult = "commit-result.json"

	// ArtifactPushResult is the filename for push operation results.
	ArtifactPushResult = "push-result.json"

	// ArtifactPRResult is the filename for pull request creation results.
	ArtifactPRResult = "pr-result.json"

	// ArtifactPRDescription is the filename for pull request description markdown.
	ArtifactPRDescription = "pr-description.md"

	// ArtifactMergeResult is the filename for merge operation results.
	ArtifactMergeResult = "merge-result.json"

	// ArtifactReviewResult is the filename for review operation results.
	ArtifactReviewResult = "review-result.json"

	// ArtifactCommentResult is the filename for comment operation results.
	ArtifactCommentResult = "comment-result.json"
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
