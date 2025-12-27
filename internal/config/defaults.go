package config

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
)

// DefaultConfig returns a new Config with sensible default values.
// These defaults are used as the base layer that can be overridden by
// config files, environment variables, and CLI flags.
//
// Default values are chosen to provide a working configuration out of the box
// while following best practices for security and performance.
func DefaultConfig() *Config {
	return &Config{
		AI: AIConfig{
			// Model: "sonnet" is the recommended balance of capability and cost.
			// Users can override to "opus" for more complex tasks or "haiku" for speed.
			Model: "sonnet",

			// APIKeyEnvVar: Standard Anthropic API key environment variable.
			// This keeps API keys out of config files for security.
			APIKeyEnvVar: "ANTHROPIC_API_KEY",

			// Timeout: 30 minutes allows for complex AI operations.
			// Uses the centralized constant for consistency.
			Timeout: constants.DefaultAITimeout,

			// MaxTurns: 10 provides reasonable conversation depth
			// while preventing runaway AI sessions.
			MaxTurns: 10,
		},
		Git: GitConfig{
			// BaseBranch: "main" is the modern Git default.
			// Projects using "master" should override in their config.
			BaseBranch: "main",

			// AutoProceedGit: true enables automation workflows.
			// Set to false if manual confirmation is preferred.
			AutoProceedGit: true,

			// Remote: "origin" is the standard Git remote name.
			Remote: "origin",
		},
		Worktree: WorktreeConfig{
			// BaseDir: empty means use default location.
			// Can be set to a specific path for custom worktree organization.
			BaseDir: "",

			// NamingSuffix: empty means no suffix.
			// Can be set to identify ATLAS-managed worktrees.
			NamingSuffix: "",
		},
		CI: CIConfig{
			// Timeout: 30 minutes for CI operations.
			// CI pipelines often run tests and builds that take significant time.
			Timeout: constants.DefaultCITimeout,

			// PollInterval: 2 minutes is a reasonable balance between
			// responsiveness and API rate limiting.
			PollInterval: constants.CIPollInterval,

			// RequiredWorkflows: empty means all workflows are considered.
			// Can be set to specific workflow names to check.
			RequiredWorkflows: nil,
		},
		Templates: TemplatesConfig{
			// DefaultTemplate: empty means use built-in default.
			DefaultTemplate: "",

			// CustomTemplates: empty map, users add their own.
			CustomTemplates: nil,
		},
		Validation: ValidationConfig{
			// Commands: empty means no validation commands by default.
			// Projects should set these in their .atlas/config.yaml.
			Commands: nil,

			// Timeout: 5 minutes is reasonable for individual validation commands.
			// Adjust based on test suite complexity.
			Timeout: 5 * time.Minute,

			// ParallelExecution: true for performance.
			// Commands run concurrently when possible.
			ParallelExecution: true,
		},
		Notifications: NotificationsConfig{
			// Bell: true enables audio notifications for important events.
			// Users who find this disruptive can disable in their config.
			Bell: true,

			// Events: default events that trigger notifications.
			Events: []string{"awaiting_approval", "validation_failed"},
		},
	}
}
