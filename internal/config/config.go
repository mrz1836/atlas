// Package config provides configuration management for ATLAS with layered precedence.
//
// Configuration sources are loaded in the following order (highest precedence first):
//  1. CLI flags (passed via LoadWithOverrides)
//  2. Environment variables (ATLAS_* prefix)
//  3. Project config (.atlas/config.yaml)
//  4. Global config (~/.atlas/config.yaml)
//  5. Built-in defaults
//
// Each higher level completely overrides the lower level for the same key.
//
// IMPORTANT: This package may import internal/constants and internal/errors,
// but MUST NOT import internal/domain or other internal packages.
package config

import "time"

// Config is the root configuration structure for ATLAS.
// It contains all configuration sections for the application.
type Config struct {
	// AI contains settings for AI/LLM operations including Claude Code execution.
	AI AIConfig `yaml:"ai" mapstructure:"ai"`

	// Git contains settings for git operations and repository management.
	Git GitConfig `yaml:"git" mapstructure:"git"`

	// Worktree contains settings for git worktree management.
	Worktree WorktreeConfig `yaml:"worktree" mapstructure:"worktree"`

	// CI contains settings for CI/CD pipeline monitoring and integration.
	CI CIConfig `yaml:"ci" mapstructure:"ci"`

	// Templates contains settings for task templates.
	Templates TemplatesConfig `yaml:"templates" mapstructure:"templates"`

	// Validation contains settings for validation command execution.
	Validation ValidationConfig `yaml:"validation" mapstructure:"validation"`

	// Notifications contains settings for user notifications.
	Notifications NotificationsConfig `yaml:"notifications" mapstructure:"notifications"`
}

// AIConfig contains settings for AI/LLM operations.
// These settings control how ATLAS interacts with Claude Code and other LLMs.
type AIConfig struct {
	// Model specifies the AI model to use (e.g., "sonnet", "opus", "haiku").
	// Default: "sonnet"
	Model string `yaml:"model" mapstructure:"model"`

	// APIKeyEnvVar is the name of the environment variable containing the API key.
	// Default: "ANTHROPIC_API_KEY"
	APIKeyEnvVar string `yaml:"api_key_env_var" mapstructure:"api_key_env_var"`

	// Timeout is the maximum duration for AI execution operations.
	// Default: 30 minutes
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// MaxTurns is the maximum number of conversation turns with the AI.
	// This prevents runaway AI sessions.
	// Default: 10, Valid range: 1-100
	MaxTurns int `yaml:"max_turns" mapstructure:"max_turns"`
}

// GitConfig contains settings for git operations.
// These settings control how ATLAS manages git repositories and branches.
type GitConfig struct {
	// BaseBranch is the default base branch for creating feature branches.
	// Default: "main"
	BaseBranch string `yaml:"base_branch" mapstructure:"base_branch"`

	// AutoProceedGit enables automatic git operations without user confirmation.
	// When true, commits and pushes proceed automatically.
	// Default: true
	AutoProceedGit bool `yaml:"auto_proceed_git" mapstructure:"auto_proceed_git"`

	// Remote is the name of the remote repository.
	// Default: "origin"
	Remote string `yaml:"remote" mapstructure:"remote"`
}

// WorktreeConfig contains settings for git worktree management.
// Worktrees allow ATLAS to work on tasks in isolated directories.
type WorktreeConfig struct {
	// BaseDir is the base directory where worktrees are created.
	// If empty, worktrees are created in the default location.
	BaseDir string `yaml:"base_dir" mapstructure:"base_dir"`

	// NamingSuffix is appended to worktree directory names.
	// Useful for identifying ATLAS-managed worktrees.
	NamingSuffix string `yaml:"naming_suffix" mapstructure:"naming_suffix"`
}

// CIConfig contains settings for CI/CD integration.
// These settings control how ATLAS monitors and interacts with CI pipelines.
type CIConfig struct {
	// Timeout is the maximum duration to wait for CI completion.
	// Default: 30 minutes
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// PollInterval is how often to check CI status.
	// Default: 2 minutes, Valid range: 1 second to 10 minutes
	PollInterval time.Duration `yaml:"poll_interval" mapstructure:"poll_interval"`

	// RequiredWorkflows is the list of CI workflow names that must pass.
	// If empty, all workflows are considered.
	RequiredWorkflows []string `yaml:"required_workflows" mapstructure:"required_workflows"`
}

// TemplatesConfig contains settings for task templates.
// Templates define the structure and steps for automated tasks.
type TemplatesConfig struct {
	// DefaultTemplate is the name of the default template to use when none is specified.
	DefaultTemplate string `yaml:"default_template" mapstructure:"default_template"`

	// CustomTemplates is a map of custom template names to their file paths.
	// These templates are merged with built-in templates.
	CustomTemplates map[string]string `yaml:"custom_templates" mapstructure:"custom_templates"`
}

// ValidationConfig contains settings for validation command execution.
// Validation commands are run to verify code quality before proceeding.
type ValidationConfig struct {
	// Commands is the list of validation commands to run.
	// Example: ["magex format:fix", "magex lint", "magex test"]
	Commands []string `yaml:"commands" mapstructure:"commands"`

	// Timeout is the maximum duration for each validation command.
	// Default: 5 minutes
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// ParallelExecution enables running validation commands in parallel.
	// Default: true
	ParallelExecution bool `yaml:"parallel_execution" mapstructure:"parallel_execution"`
}

// NotificationsConfig contains settings for user notifications.
// Notifications alert users to important events during task execution.
type NotificationsConfig struct {
	// Bell enables terminal bell notifications.
	// When true, a bell sound is played for important events.
	// Default: true
	Bell bool `yaml:"bell" mapstructure:"bell"`

	// Events is the list of event types that trigger notifications.
	// Supported events: "awaiting_approval", "validation_failed", "task_complete", "error"
	Events []string `yaml:"events" mapstructure:"events"`
}
