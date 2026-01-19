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

	// PRDescription contains settings for PR description generation.
	PRDescription PRDescriptionConfig `yaml:"pr_description" mapstructure:"pr_description"`

	// Approval contains settings for approval operations (approve + merge + close).
	Approval ApprovalConfig `yaml:"approval" mapstructure:"approval"`

	// Hooks contains settings for the hook system (crash recovery & context persistence).
	Hooks HookConfig `yaml:"hooks" mapstructure:"hooks"`
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

	// PR contains default settings for pull request operations.
	// These defaults are used by git steps and can be overridden per-step in templates.
	PR PRConfig `yaml:"pr,omitempty" mapstructure:"pr"`
}

// PRConfig contains default settings for PR operations.
// These settings control the default behavior for merge_pr, add_pr_review,
// and add_pr_comment git operations.
type PRConfig struct {
	// MergeMethod is the default merge method for PRs.
	// Valid values: "squash", "merge", "rebase"
	// Default: "squash"
	MergeMethod string `yaml:"merge_method,omitempty" mapstructure:"merge_method"`

	// DeleteBranch controls whether to delete the source branch after merging.
	// Default: false (keep branch for reference)
	DeleteBranch bool `yaml:"delete_branch,omitempty" mapstructure:"delete_branch"`

	// AdminBypass allows merging PRs even when branch protection checks haven't passed.
	// Use with caution - this bypasses required status checks.
	// Default: false
	AdminBypass bool `yaml:"admin_bypass,omitempty" mapstructure:"admin_bypass"`

	// ReviewEvent is the default review event type for add_pr_review operations.
	// Valid values: "APPROVE", "REQUEST_CHANGES", "COMMENT"
	// Default: "APPROVE"
	ReviewEvent string `yaml:"review_event,omitempty" mapstructure:"review_event"`
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
	// Supported events: "awaiting_approval", "validation_failed", "ci_failed", "github_failed"
	Events []string `yaml:"events" mapstructure:"events"`
}

// SmartCommitConfig contains settings for smart commit message generation.
// These settings control AI-powered commit message creation.
type SmartCommitConfig struct {
	// Agent overrides the AI agent for commit message generation.
	// If empty, falls back to AI.Agent setting.
	// Valid values: "claude", "gemini", "codex"
	// Default: "" (uses AI.Agent)
	Agent string `yaml:"agent,omitempty" mapstructure:"agent"`

	// Model overrides the AI model for commit message generation.
	// If empty, falls back to AI.Model setting.
	// Common values: "sonnet", "opus", "haiku"
	// Default: "" (uses AI.Model)
	Model string `yaml:"model,omitempty" mapstructure:"model"`

	// Timeout is the maximum duration for AI commit message generation.
	// If the AI takes longer than this, it will timeout and fall back to template generation.
	// Default: 30 seconds
	Timeout time.Duration `yaml:"timeout,omitempty" mapstructure:"timeout"`

	// MaxRetries is the maximum number of retry attempts for AI generation.
	// Retries use exponential backoff to handle transient failures.
	// Default: 2
	MaxRetries int `yaml:"max_retries,omitempty" mapstructure:"max_retries"`

	// RetryBackoffFactor is the multiplier for exponential backoff between retries.
	// Each retry timeout = previous_timeout * retry_backoff_factor.
	// Example: With timeout=30s and factor=1.5, retries use 30s, 45s, 67.5s...
	// Default: 1.5
	RetryBackoffFactor float64 `yaml:"retry_backoff_factor,omitempty" mapstructure:"retry_backoff_factor"`
}

// PRDescriptionConfig contains settings for PR description generation.
// These settings control AI-powered PR title and body creation.
type PRDescriptionConfig struct {
	// Agent overrides the AI agent for PR description generation.
	// If empty, falls back to AI.Agent setting.
	// Valid values: "claude", "gemini", "codex"
	// Default: "" (uses AI.Agent)
	Agent string `yaml:"agent,omitempty" mapstructure:"agent"`

	// Model overrides the AI model for PR description generation.
	// If empty, falls back to AI.Model setting.
	// Common values: "sonnet", "opus", "haiku"
	// Default: "" (uses AI.Model)
	Model string `yaml:"model,omitempty" mapstructure:"model"`
}

// ApprovalConfig contains settings for approval operations.
// These settings control the approve + merge + close workflow.
type ApprovalConfig struct {
	// MergeMessage is the default message used when approving and merging PRs.
	// This message is used for both the PR review and comment (fallback).
	// Default: "Approved and Merged by ATLAS"
	MergeMessage string `yaml:"merge_message" mapstructure:"merge_message"`
}

// HookConfig contains all configurable settings for the hook system.
// The hook system provides crash recovery and context persistence for ATLAS tasks.
type HookConfig struct {
	// MaxCheckpoints is the maximum number of checkpoints per task.
	// Oldest checkpoints are pruned when this limit is exceeded.
	// Default: 50
	MaxCheckpoints int `yaml:"max_checkpoints" mapstructure:"max_checkpoints"`

	// CheckpointInterval is the interval for periodic checkpoints during long-running steps.
	// Set to 0 to disable interval checkpoints.
	// Default: 5m
	CheckpointInterval time.Duration `yaml:"checkpoint_interval" mapstructure:"checkpoint_interval"`

	// StaleThreshold is the time after which a hook is considered stale (potential crash).
	// Default: 5m
	StaleThreshold time.Duration `yaml:"stale_threshold" mapstructure:"stale_threshold"`

	// MaxStepAttempts is the maximum number of retry attempts for a failing step.
	// Default: 3
	MaxStepAttempts int `yaml:"max_step_attempts" mapstructure:"max_step_attempts"`

	// Retention specifies how long to keep hook files per terminal state.
	Retention RetentionConfig `yaml:"retention" mapstructure:"retention"`

	// Crypto holds cryptographic configuration.
	Crypto CryptoConfig `yaml:"crypto" mapstructure:"crypto"`
}

// RetentionConfig specifies how long to keep hook files per terminal state.
type RetentionConfig struct {
	// Completed is the retention period for completed task hooks.
	// Default: 720h (30 days)
	Completed time.Duration `yaml:"completed" mapstructure:"completed"`

	// Failed is the retention period for failed task hooks.
	// Default: 168h (7 days)
	Failed time.Duration `yaml:"failed" mapstructure:"failed"`

	// Abandoned is the retention period for abandoned task hooks.
	// Default: 168h (7 days)
	Abandoned time.Duration `yaml:"abandoned" mapstructure:"abandoned"`
}
