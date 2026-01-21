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
			// Agent: "claude" is the default AI CLI agent.
			// Users can override to "gemini" for Google's Gemini models or "codex" for OpenAI.
			Agent: "claude",

			// Model: "sonnet" is the recommended balance of capability and cost.
			// Users can override to "opus" for more complex tasks or "haiku" for speed.
			// For gemini, use "flash" (default) or "pro".
			// For codex, use "codex" (default), "max", or "mini".
			Model: "sonnet",

			// APIKeyEnvVars: Maps agent names to their API key environment variables.
			// This keeps API keys out of config files for security.
			// Users can override with custom env var names per provider.
			APIKeyEnvVars: map[string]string{
				"claude": "ANTHROPIC_API_KEY",
				"gemini": "GEMINI_API_KEY",
				"codex":  "OPENAI_API_KEY",
			},

			// Timeout: 30 minutes allows for complex AI operations.
			// Uses the centralized constant for consistency.
			Timeout: constants.DefaultAITimeout,

			// MaxTurns: 10 provides reasonable conversation depth
			// while preventing runaway AI sessions.
			// DEPRECATED: Kept for backward compatibility.
			MaxTurns: 10,

			// MaxBudgetUSD: 0 means unlimited budget.
			// Users can set a positive value to limit AI spending per session.
			MaxBudgetUSD: 0.0,

			// ActivityVerbosity: "medium" is the default verbosity level.
			// Shows phases + file operations + key decisions.
			// Users can set "low" for minimal output or "high" for verbose output.
			ActivityVerbosity: "medium",

			// FallbackEnabled: true enables automatic model fallback.
			// When AI generation fails with format/content errors, the system
			// automatically tries the next model in the fallback chain.
			FallbackEnabled: true,

			// FallbackModels: default model chains per agent.
			// Models are tried in order from fastest/cheapest to most capable.
			// For smart commit (which uses haiku by default), this enables
			// automatic escalation: haiku → sonnet → opus on format errors.
			FallbackModels: map[string][]string{
				"claude": {"haiku", "sonnet", "opus"},
				"gemini": {"flash", "pro"},
				"codex":  {"mini", "codex", "max"},
			},

			// FallbackAgents: empty by default (no cross-agent fallback).
			// Users can enable cross-agent fallback if they have multiple
			// AI providers configured: e.g., ["claude", "gemini"]
			FallbackAgents: nil,

			// FallbackMaxRetriesPerModel: 1 try per model before moving to next.
			// This is intentionally low since format errors typically don't
			// resolve by retrying the same model.
			FallbackMaxRetriesPerModel: 1,
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

			// LockCleanupThreshold: 60 seconds is the default staleness threshold.
			// Lock files older than this are considered stale and safe to remove.
			LockCleanupThreshold: 60 * time.Second,

			// LockCleanupEnabled: true enables automatic cleanup of stale lock files.
			// This prevents errors from crashed git processes.
			LockCleanupEnabled: true,
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

			// GracePeriod: 2 minutes gives CI time to start before polling.
			GracePeriod: constants.CIInitialGracePeriod,

			// RequiredWorkflows: empty means all workflows are considered.
			// Can be set to specific workflow names to check.
			RequiredWorkflows: nil,
		},
		Templates: TemplatesConfig{
			// DefaultTemplate: empty means use built-in default.
			DefaultTemplate: "",

			// CustomTemplates: empty map, users add their own.
			CustomTemplates: nil,

			// BranchPrefixes: default mappings for template types to branch prefixes.
			// These follow conventional commit naming standards.
			BranchPrefixes: map[string]string{
				"bugfix":  "fix",
				"feature": "feat",
				"commit":  "chore",
			},
		},
		Validation: ValidationConfig{
			// Commands: empty means no validation commands by default.
			// Projects should set these in their .atlas/config.yaml.
			Commands: ValidationCommands{},

			// Timeout: 5 minutes is reasonable for individual validation commands.
			// Adjust based on test suite complexity.
			Timeout: 5 * time.Minute,

			// ParallelExecution: true for performance.
			// Commands run concurrently when possible.
			ParallelExecution: true,

			// AIRetryEnabled: true enables AI-assisted validation retry.
			// When validation fails, AI can attempt to fix the issues.
			AIRetryEnabled: true,

			// MaxAIRetryAttempts: 3 is a reasonable default.
			// Allows AI multiple chances to fix issues before requiring manual intervention.
			MaxAIRetryAttempts: 3,
		},
		Notifications: NotificationsConfig{
			// Bell: true enables audio notifications for important events.
			// Users who find this disruptive can disable in their config.
			Bell: true,

			// Events: default events that trigger notifications.
			// Per Story 7.6: all attention states should trigger bells by default.
			// Using granular events for better control (ci_failed, github_failed instead of legacy "error").
			Events: []string{"awaiting_approval", "validation_failed", "ci_failed", "github_failed"},
		},
		SmartCommit: SmartCommitConfig{
			// Model: empty means use AI.Model setting.
			// Can be overridden to use a different model for commit messages,
			// e.g., "haiku" for faster/cheaper commit message generation.
			Model: "",
		},
		PRDescription: PRDescriptionConfig{
			// Model: empty means use AI.Model setting.
			// Can be overridden to use a different model for PR descriptions,
			// e.g., "haiku" for faster/cheaper PR description generation.
			Model: "",
		},
		Hooks: HookConfig{
			// MaxCheckpoints: 50 provides good history without excessive storage.
			MaxCheckpoints: 50,

			// CheckpointInterval: 5 minutes for periodic checkpoints during long steps.
			CheckpointInterval: 5 * time.Minute,

			// StaleThreshold: 5 minutes before considering a hook stale (potential crash).
			StaleThreshold: 5 * time.Minute,

			// Retention: How long to keep hook files per terminal state.
			Retention: RetentionConfig{
				Completed: 720 * time.Hour, // 30 days
				Failed:    168 * time.Hour, // 7 days
				Abandoned: 168 * time.Hour, // 7 days
			},
		},
		Operations: DefaultOperationsConfig(),
	}
}

// DefaultOperationsConfig returns sensible defaults for per-operation AI settings.
// These defaults are designed to optimize for quality and cost:
// - opus for analysis (best reasoning for root cause analysis)
// - sonnet for implementation (balance of speed and capability for coding)
// - gemini flash for verification (different AI perspective, fast)
// - sonnet for validation retry (reliable fixes)
// - opus for SDD (complex specification reasoning)
func DefaultOperationsConfig() OperationsConfig {
	return OperationsConfig{
		// Analyze: Deep reasoning for bug analysis, root cause identification.
		// opus provides the best reasoning capability for complex analysis.
		Analyze: OperationAIConfig{
			Agent:          "claude",
			Model:          "opus",
			Timeout:        20 * time.Minute,
			PermissionMode: "plan", // Read-only for analysis
		},

		// Implement: Code generation and modification.
		// sonnet balances speed and capability for coding tasks.
		Implement: OperationAIConfig{
			Agent:          "claude",
			Model:          "sonnet",
			Timeout:        30 * time.Minute,
			PermissionMode: "", // Full access for implementation
		},

		// Verify: Cross-validation using different AI perspective.
		// Using gemini provides a different perspective from claude.
		Verify: OperationAIConfig{
			Agent:          "gemini",
			Model:          "flash",
			Timeout:        5 * time.Minute,
			PermissionMode: "plan", // Read-only for verification
		},

		// ValidationRetry: Fix lint/test/format errors.
		// sonnet is reliable for fixing validation issues.
		ValidationRetry: OperationAIConfig{
			Agent:       "claude",
			Model:       "sonnet",
			Timeout:     15 * time.Minute,
			MaxAttempts: 3, // Override validation.max_ai_retry_attempts
		},

		// SDD: Speckit specification-driven development.
		// opus provides complex reasoning for specifications.
		SDD: OperationAIConfig{
			Agent:   "claude",
			Model:   "opus",
			Timeout: 25 * time.Minute,
		},

		// CIFailure: Analyze and fix CI failures.
		// sonnet is capable of analyzing CI failure logs.
		CIFailure: OperationAIConfig{
			Agent:   "claude",
			Model:   "sonnet",
			Timeout: 10 * time.Minute,
		},
	}
}
