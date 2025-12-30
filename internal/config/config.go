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

// ValidationCommands holds validation commands organized by category.
// This structure allows for clear organization of different validation types.
type ValidationCommands struct {
	// Format contains commands that format code (e.g., "magex format:fix").
	Format []string `yaml:"format" mapstructure:"format"`
	// Lint contains commands that lint code (e.g., "magex lint").
	Lint []string `yaml:"lint" mapstructure:"lint"`
	// Test contains commands that run tests (e.g., "magex test").
	Test []string `yaml:"test" mapstructure:"test"`
	// PreCommit contains commands run before committing (e.g., "go-pre-commit run --all-files").
	PreCommit []string `yaml:"pre_commit" mapstructure:"pre_commit"`
	// CustomPrePR contains custom commands to run before creating a PR.
	CustomPrePR []string `yaml:"custom_pre_pr,omitempty" mapstructure:"custom_pre_pr"`
}

// ValidationConfig contains settings for validation command execution.
// Validation commands are run to verify code quality before proceeding.
type ValidationConfig struct {
	// Commands holds all validation commands organized by category.
	Commands ValidationCommands `yaml:"commands" mapstructure:"commands"`

	// Timeout is the maximum duration for each validation command.
	// Default: 5 minutes
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// ParallelExecution enables running validation commands in parallel.
	// Default: true
	ParallelExecution bool `yaml:"parallel_execution" mapstructure:"parallel_execution"`

	// TemplateOverrides allows per-template validation settings.
	TemplateOverrides map[string]TemplateOverrideConfig `yaml:"template_overrides,omitempty" mapstructure:"template_overrides"`

	// AIRetryEnabled enables AI-assisted retry when validation fails.
	// When true and validation fails, the system can invoke AI to fix issues.
	// Default: true
	AIRetryEnabled bool `yaml:"ai_retry_enabled" mapstructure:"ai_retry_enabled"`

	// MaxAIRetryAttempts is the maximum number of AI retry attempts.
	// After this many attempts, the task enters a failed state requiring manual intervention.
	// Default: 3
	MaxAIRetryAttempts int `yaml:"max_ai_retry_attempts" mapstructure:"max_ai_retry_attempts"`
}

// TemplateOverrideConfig holds per-template validation overrides.
type TemplateOverrideConfig struct {
	// SkipTest indicates whether to skip tests for this template type.
	SkipTest bool `yaml:"skip_test" mapstructure:"skip_test"`
	// SkipLint indicates whether to skip linting for this template type.
	SkipLint bool `yaml:"skip_lint,omitempty" mapstructure:"skip_lint"`
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
