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
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.APIKeyEnvVar, "default API key env var")
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

	// Verify Notifications defaults
	assert.True(t, cfg.Notifications.Bell, "default bell notification")
	assert.Contains(t, cfg.Notifications.Events, "awaiting_approval", "default events")

	// Validate the default config passes validation
	err := Validate(cfg)
	assert.NoError(t, err, "default config should pass validation")
}

func TestConfig_YAMLSerialization(t *testing.T) {
	original := &Config{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "MY_API_KEY",
			Timeout:      45 * time.Minute,
			MaxTurns:     20,
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
				PreCommit: []string{"go-pre-commit run --all-files"},
			},
			Timeout:           10 * time.Minute,
			ParallelExecution: false,
		},
		Notifications: NotificationsConfig{
			Bell:   false,
			Events: []string{"error", "task_complete"},
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
	assert.Equal(t, original.AI.APIKeyEnvVar, restored.AI.APIKeyEnvVar)
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

	assert.Equal(t, original.Notifications.Bell, restored.Notifications.Bell)
	assert.Equal(t, original.Notifications.Events, restored.Notifications.Events)
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
