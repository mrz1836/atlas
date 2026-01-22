package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestDefaultConfig_ReturnsValidConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.NotNil(t, cfg, "DefaultConfig should not return nil")

	// Verify AI defaults
	assert.Equal(t, "sonnet", cfg.AI.Model, "default AI model should be sonnet")
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.GetAPIKeyEnvVar("claude"), "default Claude API key env var")
	assert.Equal(t, "GEMINI_API_KEY", cfg.AI.GetAPIKeyEnvVar("gemini"), "default Gemini API key env var")
	assert.Equal(t, "OPENAI_API_KEY", cfg.AI.GetAPIKeyEnvVar("codex"), "default Codex API key env var")
	assert.Equal(t, constants.DefaultAITimeout, cfg.AI.Timeout, "default AI timeout")
	assert.Equal(t, 10, cfg.AI.MaxTurns, "default max turns")

	// Verify Git defaults
	assert.Equal(t, "main", cfg.Git.BaseBranch, "default base branch")
	assert.True(t, cfg.Git.AutoProceedGit, "default auto proceed git")
	assert.Equal(t, "origin", cfg.Git.Remote, "default remote")

	// Verify CI defaults
	assert.Equal(t, constants.DefaultCITimeout, cfg.CI.Timeout, "default CI timeout")
	assert.Equal(t, constants.CIPollInterval, cfg.CI.PollInterval, "default CI poll interval")

	// Verify Validation defaults
	assert.Equal(t, 5*time.Minute, cfg.Validation.Timeout, "default validation timeout")
	assert.True(t, cfg.Validation.ParallelExecution, "default parallel execution")
	assert.True(t, cfg.Validation.AIRetryEnabled, "default AI retry enabled")
	assert.Equal(t, 3, cfg.Validation.MaxAIRetryAttempts, "default max AI retry attempts")

	// Verify Notifications defaults
	assert.True(t, cfg.Notifications.Bell, "default bell notification")
	assert.Contains(t, cfg.Notifications.Events, "awaiting_approval", "default events")

	// Verify SmartCommit defaults
	assert.Empty(t, cfg.SmartCommit.Model, "default smart commit model should be empty (uses AI.Model)")

	// Verify PRDescription defaults
	assert.Empty(t, cfg.PRDescription.Model, "default PR description model should be empty (uses AI.Model)")

	// Validate the default config passes validation
	err := Validate(cfg)
	assert.NoError(t, err, "default config should pass validation")
}

func TestConfig_YAMLSerialization(t *testing.T) {
	original := &Config{
		AI: AIConfig{
			Model: "opus",
			APIKeyEnvVars: map[string]string{
				"claude": "MY_API_KEY",
			},
			Timeout:  45 * time.Minute,
			MaxTurns: 20,
		},
		Git: GitConfig{
			BaseBranch:     "develop",
			AutoProceedGit: false,
			Remote:         "upstream",
		},
		Worktree: WorktreeConfig{
			BaseDir:      "/tmp/worktrees",
			NamingSuffix: "-atlas",
		},
		CI: CIConfig{
			Timeout:           60 * time.Minute,
			PollInterval:      5 * time.Minute,
			RequiredWorkflows: []string{"build", "test"},
		},
		Templates: TemplatesConfig{
			DefaultTemplate: "default",
			CustomTemplates: map[string]string{
				"custom": "/path/to/template.yaml",
			},
		},
		Validation: ValidationConfig{
			Commands: ValidationCommands{
				Format:    []string{"magex format:fix"},
				Lint:      []string{"magex lint"},
				Test:      []string{"magex test"},
				PreCommit: []string{"go-pre-commit run --all-files --skip lint"},
			},
			Timeout:            10 * time.Minute,
			ParallelExecution:  false,
			AIRetryEnabled:     true,
			MaxAIRetryAttempts: 5,
		},
		Notifications: NotificationsConfig{
			Bell:   false,
			Events: []string{"error", "task_complete"},
		},
		SmartCommit: SmartCommitConfig{
			Model: "haiku",
		},
		PRDescription: PRDescriptionConfig{
			Model: "sonnet",
		},
	}

	// Serialize to YAML
	data, err := yaml.Marshal(original)
	require.NoError(t, err, "should marshal to YAML")

	// Deserialize back
	var restored Config
	err = yaml.Unmarshal(data, &restored)
	require.NoError(t, err, "should unmarshal from YAML")

	// Verify all fields
	assert.Equal(t, original.AI.Model, restored.AI.Model)
	assert.Equal(t, original.AI.APIKeyEnvVars["claude"], restored.AI.APIKeyEnvVars["claude"])
	assert.Equal(t, original.AI.Timeout, restored.AI.Timeout)
	assert.Equal(t, original.AI.MaxTurns, restored.AI.MaxTurns)

	assert.Equal(t, original.Git.BaseBranch, restored.Git.BaseBranch)
	assert.Equal(t, original.Git.AutoProceedGit, restored.Git.AutoProceedGit)
	assert.Equal(t, original.Git.Remote, restored.Git.Remote)

	assert.Equal(t, original.Worktree.BaseDir, restored.Worktree.BaseDir)
	assert.Equal(t, original.Worktree.NamingSuffix, restored.Worktree.NamingSuffix)

	assert.Equal(t, original.CI.Timeout, restored.CI.Timeout)
	assert.Equal(t, original.CI.PollInterval, restored.CI.PollInterval)
	assert.Equal(t, original.CI.RequiredWorkflows, restored.CI.RequiredWorkflows)

	assert.Equal(t, original.Templates.DefaultTemplate, restored.Templates.DefaultTemplate)
	assert.Equal(t, original.Templates.CustomTemplates, restored.Templates.CustomTemplates)

	assert.Equal(t, original.Validation.Commands, restored.Validation.Commands)
	assert.Equal(t, original.Validation.Timeout, restored.Validation.Timeout)
	assert.Equal(t, original.Validation.ParallelExecution, restored.Validation.ParallelExecution)
	assert.Equal(t, original.Validation.AIRetryEnabled, restored.Validation.AIRetryEnabled)
	assert.Equal(t, original.Validation.MaxAIRetryAttempts, restored.Validation.MaxAIRetryAttempts)

	assert.Equal(t, original.Notifications.Bell, restored.Notifications.Bell)
	assert.Equal(t, original.Notifications.Events, restored.Notifications.Events)

	assert.Equal(t, original.SmartCommit.Model, restored.SmartCommit.Model)

	assert.Equal(t, original.PRDescription.Model, restored.PRDescription.Model)
}

func TestValidate_InvalidValues(t *testing.T) {
	tests := []struct {
		name       string
		modify     func(*Config)
		wantErrMsg string
	}{
		{
			name:       "nil config",
			modify:     nil, // special case handled below
			wantErrMsg: "config is nil",
		},
		{
			name: "negative AI timeout",
			modify: func(c *Config) {
				c.AI.Timeout = -1 * time.Minute
			},
			wantErrMsg: "ai.timeout must be positive",
		},
		{
			name: "zero AI timeout",
			modify: func(c *Config) {
				c.AI.Timeout = 0
			},
			wantErrMsg: "ai.timeout must be positive",
		},
		{
			name: "max turns too low",
			modify: func(c *Config) {
				c.AI.MaxTurns = 0
			},
			wantErrMsg: "ai.max_turns must be between 1 and 100",
		},
		{
			name: "max turns too high",
			modify: func(c *Config) {
				c.AI.MaxTurns = 101
			},
			wantErrMsg: "ai.max_turns must be between 1 and 100",
		},
		{
			name: "empty base branch",
			modify: func(c *Config) {
				c.Git.BaseBranch = ""
			},
			wantErrMsg: "git.base_branch must not be empty",
		},
		{
			name: "negative CI timeout",
			modify: func(c *Config) {
				c.CI.Timeout = -1 * time.Minute
			},
			wantErrMsg: "ci.timeout must be positive",
		},
		{
			name: "poll interval too short",
			modify: func(c *Config) {
				c.CI.PollInterval = 500 * time.Millisecond
			},
			wantErrMsg: "ci.poll_interval must be between",
		},
		{
			name: "poll interval too long",
			modify: func(c *Config) {
				c.CI.PollInterval = 15 * time.Minute
			},
			wantErrMsg: "ci.poll_interval must be between",
		},
		{
			name: "negative validation timeout",
			modify: func(c *Config) {
				c.Validation.Timeout = -1 * time.Minute
			},
			wantErrMsg: "validation.timeout must be positive",
		},
		{
			name: "negative max AI retry attempts",
			modify: func(c *Config) {
				c.Validation.MaxAIRetryAttempts = -1
			},
			wantErrMsg: "validation.max_ai_retry_attempts cannot be negative",
		},
		{
			name: "zero max AI retry attempts when enabled",
			modify: func(c *Config) {
				c.Validation.AIRetryEnabled = true
				c.Validation.MaxAIRetryAttempts = 0
			},
			wantErrMsg: "validation.max_ai_retry_attempts must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *Config
			if tt.modify != nil {
				cfg = DefaultConfig()
				tt.modify(cfg)
			}

			err := Validate(cfg)
			require.Error(t, err, "expected validation to fail")
			assert.Contains(t, err.Error(), tt.wantErrMsg, "error message should contain expected text")
		})
	}
}

func TestDefaultConfig_HasOperationsConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Verify operations defaults are set
	assert.Equal(t, "claude", cfg.Operations.Analyze.Agent)
	assert.Equal(t, "opus", cfg.Operations.Analyze.Model)
	assert.Equal(t, 20*time.Minute, cfg.Operations.Analyze.Timeout)
	assert.Equal(t, "plan", cfg.Operations.Analyze.PermissionMode)

	assert.Equal(t, "claude", cfg.Operations.Implement.Agent)
	assert.Equal(t, "sonnet", cfg.Operations.Implement.Model)
	assert.Equal(t, 30*time.Minute, cfg.Operations.Implement.Timeout)

	assert.Equal(t, "gemini", cfg.Operations.Verify.Agent)
	assert.Equal(t, "flash", cfg.Operations.Verify.Model)
	assert.Equal(t, 5*time.Minute, cfg.Operations.Verify.Timeout)
	assert.Equal(t, "plan", cfg.Operations.Verify.PermissionMode)

	assert.Equal(t, "claude", cfg.Operations.ValidationRetry.Agent)
	assert.Equal(t, "sonnet", cfg.Operations.ValidationRetry.Model)
	assert.Equal(t, 15*time.Minute, cfg.Operations.ValidationRetry.Timeout)
	assert.Equal(t, 3, cfg.Operations.ValidationRetry.MaxAttempts)

	assert.Equal(t, "claude", cfg.Operations.SDD.Agent)
	assert.Equal(t, "opus", cfg.Operations.SDD.Model)
	assert.Equal(t, 25*time.Minute, cfg.Operations.SDD.Timeout)

	assert.Equal(t, "claude", cfg.Operations.CIFailure.Agent)
	assert.Equal(t, "sonnet", cfg.Operations.CIFailure.Model)
	assert.Equal(t, 10*time.Minute, cfg.Operations.CIFailure.Timeout)
}

func TestOperationsConfig_GetForStep(t *testing.T) {
	cfg := DefaultOperationsConfig()

	tests := []struct {
		name          string
		stepName      string
		stepType      string
		expectedAgent string
		expectedModel string
	}{
		{
			name:          "analyze step",
			stepName:      "analyze",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "opus",
		},
		{
			name:          "implement step",
			stepName:      "implement",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "sonnet",
		},
		{
			name:          "verify step by name",
			stepName:      "verify",
			stepType:      "ai",
			expectedAgent: "gemini",
			expectedModel: "flash",
		},
		{
			name:          "verify step by type",
			stepName:      "custom_verify",
			stepType:      "verify",
			expectedAgent: "gemini",
			expectedModel: "flash",
		},
		{
			name:          "validation_retry step",
			stepName:      "validation_retry",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "sonnet",
		},
		{
			name:          "ci_failure step",
			stepName:      "ci_failure",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "sonnet",
		},
		{
			name:          "sdd step by type",
			stepName:      "custom_sdd",
			stepType:      "sdd",
			expectedAgent: "claude",
			expectedModel: "opus",
		},
		{
			name:          "sdd-specify step",
			stepName:      "sdd-specify",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "opus",
		},
		{
			name:          "sdd-implement step",
			stepName:      "sdd-implement",
			stepType:      "ai",
			expectedAgent: "claude",
			expectedModel: "opus",
		},
		{
			name:          "unknown step returns empty",
			stepName:      "unknown",
			stepType:      "unknown",
			expectedAgent: "",
			expectedModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opConfig := cfg.GetForStep(tt.stepName, tt.stepType)
			assert.Equal(t, tt.expectedAgent, opConfig.Agent)
			assert.Equal(t, tt.expectedModel, opConfig.Model)
		})
	}
}

func TestOperationAIConfig_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		config   OperationAIConfig
		expected bool
	}{
		{
			name:     "completely empty",
			config:   OperationAIConfig{},
			expected: true,
		},
		{
			name:     "only agent set",
			config:   OperationAIConfig{Agent: "claude"},
			expected: false,
		},
		{
			name:     "only model set",
			config:   OperationAIConfig{Model: "sonnet"},
			expected: false,
		},
		{
			name:     "only timeout set",
			config:   OperationAIConfig{Timeout: 5 * time.Minute},
			expected: false,
		},
		{
			name:     "only permission_mode set",
			config:   OperationAIConfig{PermissionMode: "plan"},
			expected: false,
		},
		{
			name:     "only max_attempts set",
			config:   OperationAIConfig{MaxAttempts: 3},
			expected: false,
		},
		{
			name: "fully populated",
			config: OperationAIConfig{
				Agent:          "claude",
				Model:          "sonnet",
				Timeout:        30 * time.Minute,
				PermissionMode: "default",
				MaxAttempts:    5,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsEmpty())
		})
	}
}

func TestOperationsConfig_YAMLSerialization(t *testing.T) {
	original := OperationsConfig{
		Analyze: OperationAIConfig{
			Agent:          "claude",
			Model:          "opus",
			Timeout:        20 * time.Minute,
			PermissionMode: "plan",
		},
		Implement: OperationAIConfig{
			Agent:   "claude",
			Model:   "sonnet",
			Timeout: 30 * time.Minute,
		},
		Verify: OperationAIConfig{
			Agent:          "gemini",
			Model:          "flash",
			Timeout:        5 * time.Minute,
			PermissionMode: "plan",
		},
		ValidationRetry: OperationAIConfig{
			Agent:       "claude",
			Model:       "sonnet",
			Timeout:     15 * time.Minute,
			MaxAttempts: 3,
		},
		SDD: OperationAIConfig{
			Agent:   "claude",
			Model:   "opus",
			Timeout: 25 * time.Minute,
		},
		CIFailure: OperationAIConfig{
			Agent:   "claude",
			Model:   "sonnet",
			Timeout: 10 * time.Minute,
		},
	}

	// Serialize to YAML
	data, err := yaml.Marshal(original)
	require.NoError(t, err, "should marshal to YAML")

	// Deserialize back
	var restored OperationsConfig
	err = yaml.Unmarshal(data, &restored)
	require.NoError(t, err, "should unmarshal from YAML")

	// Verify all fields
	assert.Equal(t, original.Analyze, restored.Analyze)
	assert.Equal(t, original.Implement, restored.Implement)
	assert.Equal(t, original.Verify, restored.Verify)
	assert.Equal(t, original.ValidationRetry, restored.ValidationRetry)
	assert.Equal(t, original.SDD, restored.SDD)
	assert.Equal(t, original.CIFailure, restored.CIFailure)
}

func TestValidate_ValidConfig(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{
			name:   "default config",
			modify: nil,
		},
		{
			name: "min valid values",
			modify: func(c *Config) {
				c.AI.MaxTurns = 1
				c.CI.PollInterval = 1 * time.Second
			},
		},
		{
			name: "max valid values",
			modify: func(c *Config) {
				c.AI.MaxTurns = 100
				c.CI.PollInterval = 10 * time.Minute
			},
		},
		{
			name: "custom model",
			modify: func(c *Config) {
				c.AI.Model = "haiku"
			},
		},
		{
			name: "custom base branch",
			modify: func(c *Config) {
				c.Git.BaseBranch = "master"
			},
		},
		{
			name: "AI retry disabled with zero attempts allowed",
			modify: func(c *Config) {
				c.Validation.AIRetryEnabled = false
				c.Validation.MaxAIRetryAttempts = 0
			},
		},
		{
			name: "AI retry with custom max attempts",
			modify: func(c *Config) {
				c.Validation.AIRetryEnabled = true
				c.Validation.MaxAIRetryAttempts = 5
			},
		},
		{
			name: "smart commit with custom model",
			modify: func(c *Config) {
				c.SmartCommit.Model = "haiku"
			},
		},
		{
			name: "smart commit with empty model (uses AI.Model)",
			modify: func(c *Config) {
				c.SmartCommit.Model = ""
			},
		},
		{
			name: "pr description with custom model",
			modify: func(c *Config) {
				c.PRDescription.Model = "sonnet"
			},
		},
		{
			name: "pr description with empty model (uses AI.Model)",
			modify: func(c *Config) {
				c.PRDescription.Model = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			if tt.modify != nil {
				tt.modify(cfg)
			}

			err := Validate(cfg)
			assert.NoError(t, err, "expected validation to pass")
		})
	}
}
