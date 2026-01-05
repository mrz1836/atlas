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

	// SmartCommit contains settings for smart commit message generation.
	SmartCommit SmartCommitConfig `yaml:"smart_commit" mapstructure:"smart_commit"`
}

// AIConfig contains settings for AI/LLM operations.
// These settings control how ATLAS interacts with Claude Code, Gemini, Codex, and other LLMs.
type AIConfig struct {
	// Agent specifies which AI CLI to use (e.g., "claude", "gemini", "codex").
	// Default: "claude"
	Agent string `yaml:"agent" mapstructure:"agent"`

	// Model specifies the AI model to use (e.g., "sonnet", "opus", "flash", "pro", "codex", "max", "mini").
	// Default: depends on agent ("sonnet" for claude, "flash" for gemini, "codex" for codex)
	Model string `yaml:"model" mapstructure:"model"`

	// APIKeyEnvVars maps agent names to their API key environment variable names.
	// This allows configuring custom API key env vars per provider.
	// Example: {"claude": "MY_ANTHROPIC_KEY", "codex": "WORK_OPENAI_KEY"}
	// If an agent is not in the map, its default env var is used.
	// Defaults: {"claude": "ANTHROPIC_API_KEY", "gemini": "GEMINI_API_KEY", "codex": "OPENAI_API_KEY"}
	APIKeyEnvVars map[string]string `yaml:"api_key_env_vars" mapstructure:"api_key_env_vars"`

	// Timeout is the maximum duration for AI execution operations.
	// Default: 30 minutes
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// MaxTurns is the maximum number of conversation turns with the AI.
	// DEPRECATED: Claude CLI does not support turn limiting.
	// Use MaxBudgetUSD instead. This field will be removed in v2.0.
	// Default: 10, Valid range: 1-100
	MaxTurns int `yaml:"max_turns,omitempty" mapstructure:"max_turns"`

	// MaxBudgetUSD limits the maximum dollar amount for AI operations.
	// Set to 0 for no budget limit.
	// Default: 0 (unlimited)
	MaxBudgetUSD float64 `yaml:"max_budget_usd,omitempty" mapstructure:"max_budget_usd"`
}

// GetAPIKeyEnvVar returns the API key environment variable for the given agent.
// It checks the configured APIKeyEnvVars map first, then falls back to the agent's default.
func (c *AIConfig) GetAPIKeyEnvVar(agent string) string {
	if c.APIKeyEnvVars != nil {
		if envVar, ok := c.APIKeyEnvVars[agent]; ok {
			return envVar
		}
	}
	// Fall back to agent defaults
	switch agent {
	case "claude":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "codex":
		return "OPENAI_API_KEY"
	default:
		return ""
	}
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

	// GracePeriod is the initial grace period before starting to poll.
	// Default: 2 minutes
	GracePeriod time.Duration `yaml:"grace_period" mapstructure:"grace_period"`

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

	// BranchPrefixes maps template types to branch prefixes.
	// Example: {"bugfix": "fix", "feature": "feat", "commit": "chore"}
	// These override the built-in defaults in git.DefaultBranchPrefixes.
	BranchPrefixes map[string]string `yaml:"branch_prefixes,omitempty" mapstructure:"branch_prefixes"`
}

// ValidationCommands holds validation commands organized by category.
// Fields are ordered to match execution order (pre-commit runs first).
type ValidationCommands struct {
	// PreCommit contains commands run before committing (e.g., "go-pre-commit run --all-files").
	PreCommit []string `yaml:"pre_commit" mapstructure:"pre_commit"`
	// Format contains commands that format code (e.g., "magex format:fix").
	Format []string `yaml:"format" mapstructure:"format"`
	// Lint contains commands that lint code (e.g., "magex lint").
	Lint []string `yaml:"lint" mapstructure:"lint"`
	// Test contains commands that run tests (e.g., "magex test").
	Test []string `yaml:"test" mapstructure:"test"`
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

	// AIRetryEnabled enables AI-assisted retry when validation fails.
	// When true and validation fails, the system can invoke AI to fix issues.
	// Default: true
	AIRetryEnabled bool `yaml:"ai_retry_enabled" mapstructure:"ai_retry_enabled"`

	// MaxAIRetryAttempts is the maximum number of AI retry attempts.
	// After this many attempts, the task enters a failed state requiring manual intervention.
	// Default: 3
	MaxAIRetryAttempts int `yaml:"max_ai_retry_attempts" mapstructure:"max_ai_retry_attempts"`
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

// SmartCommitConfig contains settings for smart commit message generation.
// These settings control AI-powered commit message creation.
type SmartCommitConfig struct {
	// Model overrides the AI model for commit message generation.
	// If empty, falls back to AI.Model setting.
	// Common values: "sonnet", "opus", "haiku"
	// Default: "" (uses AI.Model)
	Model string `yaml:"model,omitempty" mapstructure:"model"`
}
