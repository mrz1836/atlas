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
