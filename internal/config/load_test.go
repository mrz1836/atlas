package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestLoad_ReturnsDefaultsWhenNoConfigFile(t *testing.T) {
	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Clear any ATLAS_ env vars that might interfere
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "ATLAS_" {
			key := env[:len(env)-len(env[len(env)-len(env[len(env)-1:]):])]
			for i := 0; i < len(env); i++ {
				if env[i] == '=' {
					key = env[:i]
					break
				}
			}
			t.Setenv(key, "")
		}
	}

	cfg, err := Load(context.Background())
	require.NoError(t, err, "Load should not fail when no config file exists")
	require.NotNil(t, cfg, "Config should not be nil")

	// Verify defaults are applied
	assert.Equal(t, "sonnet", cfg.AI.Model, "should use default AI model")
	assert.Equal(t, constants.DefaultAITimeout, cfg.AI.Timeout, "should use default AI timeout")
	assert.Equal(t, "main", cfg.Git.BaseBranch, "should use default base branch")
}

func TestLoadFromPaths_ProjectConfigOverridesGlobal(t *testing.T) {
	ctx := context.Background()

	// Create temp directories for configs
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write global config with ai.model = "opus"
	globalConfig := filepath.Join(globalDir, "config.yaml")
	err := os.WriteFile(globalConfig, []byte(`
ai:
  model: opus
  max_turns: 50
git:
  base_branch: master
`), 0o600)
	require.NoError(t, err)

	// Write project config with ai.model = "sonnet"
	projectConfig := filepath.Join(projectDir, "config.yaml")
	err = os.WriteFile(projectConfig, []byte(`
ai:
  model: sonnet
`), 0o600)
	require.NoError(t, err)

	// Load config - project should override global
	cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Project config overrides global for ai.model
	assert.Equal(t, "sonnet", cfg.AI.Model, "project config should override global for ai.model")

	// Global config values that aren't overridden should persist
	assert.Equal(t, 50, cfg.AI.MaxTurns, "global max_turns should be preserved")
	assert.Equal(t, "master", cfg.Git.BaseBranch, "global base_branch should be preserved")
}

func TestLoadFromPaths_GlobalConfigOnly(t *testing.T) {
	ctx := context.Background()

	// Create temp directory for global config
	globalDir := t.TempDir()

	// Write global config
	globalConfig := filepath.Join(globalDir, "config.yaml")
	err := os.WriteFile(globalConfig, []byte(`
ai:
  model: opus
  max_turns: 25
git:
  base_branch: develop
  remote: upstream
`), 0o600)
	require.NoError(t, err)

	// Load with only global config
	cfg, err := LoadFromPaths(ctx, "", globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed with only global config")

	// Verify global config values
	assert.Equal(t, "opus", cfg.AI.Model, "should use global ai.model")
	assert.Equal(t, 25, cfg.AI.MaxTurns, "should use global max_turns")
	assert.Equal(t, "develop", cfg.Git.BaseBranch, "should use global base_branch")
	assert.Equal(t, "upstream", cfg.Git.Remote, "should use global remote")
}

func TestLoad_EnvVarOverridesConfigFile(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with a config file
	tempDir := t.TempDir()
	atlasDir := filepath.Join(tempDir, ".atlas")
	err := os.MkdirAll(atlasDir, 0o750)
	require.NoError(t, err)

	// Write config file with model = "opus"
	configPath := filepath.Join(atlasDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(`
ai:
  model: opus
`), 0o600)
	require.NoError(t, err)

	// Change to the temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Set env var to override (should take precedence)
	t.Setenv("ATLAS_AI_MODEL", "haiku")

	cfg, err := Load(ctx)
	require.NoError(t, err, "Load should succeed")

	// Environment variable should override config file
	assert.Equal(t, "haiku", cfg.AI.Model, "env var should override config file")
}

func TestLoad_EnvVarMapping(t *testing.T) {
	ctx := context.Background()

	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Test various env var mappings
	tests := []struct {
		envVar   string
		value    string
		validate func(*testing.T, *Config)
	}{
		{
			envVar: "ATLAS_AI_MODEL",
			value:  "opus",
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "opus", c.AI.Model)
			},
		},
		{
			envVar: "ATLAS_AI_MAX_TURNS",
			value:  "25",
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 25, c.AI.MaxTurns)
			},
		},
		{
			envVar: "ATLAS_GIT_BASE_BRANCH",
			value:  "develop",
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "develop", c.Git.BaseBranch)
			},
		},
		{
			envVar: "ATLAS_GIT_REMOTE",
			value:  "upstream",
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "upstream", c.Git.Remote)
			},
		},
		{
			envVar: "ATLAS_NOTIFICATIONS_BELL",
			value:  "false",
			validate: func(t *testing.T, c *Config) {
				assert.False(t, c.Notifications.Bell)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.value)

			cfg, err := Load(ctx)
			require.NoError(t, err, "Load should succeed")
			tt.validate(t, cfg)
		})
	}
}

func TestLoadWithOverrides_AppliesCLIOverrides(t *testing.T) {
	ctx := context.Background()

	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	overrides := &Config{
		AI: AIConfig{
			Model:    "opus",
			MaxTurns: 50,
		},
		Git: GitConfig{
			BaseBranch: "develop",
		},
	}

	cfg, err := LoadWithOverrides(ctx, overrides)
	require.NoError(t, err, "LoadWithOverrides should succeed")

	// Verify overrides are applied
	assert.Equal(t, "opus", cfg.AI.Model, "override AI model")
	assert.Equal(t, 50, cfg.AI.MaxTurns, "override max turns")
	assert.Equal(t, "develop", cfg.Git.BaseBranch, "override base branch")

	// Verify non-overridden values keep defaults
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.APIKeyEnvVar, "default API key env var")
	assert.Equal(t, "origin", cfg.Git.Remote, "default remote")
}

func TestLoadWithOverrides_NilOverrides(t *testing.T) {
	ctx := context.Background()

	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	cfg, err := LoadWithOverrides(ctx, nil)
	require.NoError(t, err, "LoadWithOverrides with nil should succeed")

	// Verify defaults are used
	assert.Equal(t, "sonnet", cfg.AI.Model, "should use default model")
}

func TestLoadFromPaths_DurationParsing(t *testing.T) {
	ctx := context.Background()

	// Create temp directory for config
	tempDir := t.TempDir()

	// Write config with duration strings
	configPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(`
ai:
  timeout: 45m
ci:
  timeout: 1h
  poll_interval: 3m
validation:
  timeout: 10m
`), 0o600)
	require.NoError(t, err)

	cfg, err := LoadFromPaths(ctx, configPath, "")
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Verify durations are parsed correctly
	assert.Equal(t, 45*time.Minute, cfg.AI.Timeout, "AI timeout should be 45m")
	assert.Equal(t, 1*time.Hour, cfg.CI.Timeout, "CI timeout should be 1h")
	assert.Equal(t, 3*time.Minute, cfg.CI.PollInterval, "CI poll interval should be 3m")
	assert.Equal(t, 10*time.Minute, cfg.Validation.Timeout, "Validation timeout should be 10m")
}

func TestLoadFromPaths_InvalidConfigFile(t *testing.T) {
	ctx := context.Background()

	// Create temp directory for config
	tempDir := t.TempDir()

	// Write invalid YAML
	configPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(`
ai:
  model: opus
  invalid yaml here: [
`), 0o600)
	require.NoError(t, err)

	_, err = LoadFromPaths(ctx, configPath, "")
	require.Error(t, err, "LoadFromPaths should fail with invalid YAML")
	assert.Contains(t, err.Error(), "failed to read project config", "error should mention reading config")
}

func TestLoadFromPaths_ValidationFailure(t *testing.T) {
	ctx := context.Background()

	// Create temp directory for config
	tempDir := t.TempDir()

	// Write config with invalid values
	configPath := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(`
ai:
  max_turns: 200
`), 0o600)
	require.NoError(t, err)

	_, err = LoadFromPaths(ctx, configPath, "")
	require.Error(t, err, "LoadFromPaths should fail validation")
	assert.Contains(t, err.Error(), "max_turns must be between", "error should mention validation issue")
}

func TestLoad_MergesGlobalAndProjectConfigs(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory structure simulating a project with both configs
	tempDir := t.TempDir()

	// Create fake home directory with global config
	fakeHome := filepath.Join(tempDir, "home")
	globalAtlasDir := filepath.Join(fakeHome, ".atlas")
	err := os.MkdirAll(globalAtlasDir, 0o750)
	require.NoError(t, err)

	// Write global config with multiple settings
	globalConfig := filepath.Join(globalAtlasDir, "config.yaml")
	err = os.WriteFile(globalConfig, []byte(`
ai:
  model: opus
  max_turns: 50
git:
  base_branch: develop
  remote: upstream
`), 0o600)
	require.NoError(t, err)

	// Create project directory with project config
	projectDir := filepath.Join(tempDir, "project")
	projectAtlasDir := filepath.Join(projectDir, ".atlas")
	err = os.MkdirAll(projectAtlasDir, 0o750)
	require.NoError(t, err)

	// Write project config that only overrides ai.model
	projectConfig := filepath.Join(projectAtlasDir, "config.yaml")
	err = os.WriteFile(projectConfig, []byte(`
ai:
  model: sonnet
`), 0o600)
	require.NoError(t, err)

	// Use LoadFromPaths to test the merging behavior
	// (Load() uses the real home dir which we can't easily mock)
	cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Project config should override global for ai.model
	assert.Equal(t, "sonnet", cfg.AI.Model, "project should override global ai.model")

	// Global config values that aren't overridden should be preserved
	assert.Equal(t, 50, cfg.AI.MaxTurns, "global max_turns should be preserved")
	assert.Equal(t, "develop", cfg.Git.BaseBranch, "global base_branch should be preserved")
	assert.Equal(t, "upstream", cfg.Git.Remote, "global remote should be preserved")
}

func TestPaths(t *testing.T) {
	// Test ProjectConfigDir
	assert.Equal(t, ".atlas", ProjectConfigDir(), "project config dir should be .atlas")

	// Test ProjectConfigPath
	assert.Equal(t, ".atlas/config.yaml", ProjectConfigPath(), "project config path")

	// Test GlobalConfigDir
	globalDir, err := GlobalConfigDir()
	require.NoError(t, err, "GlobalConfigDir should succeed")
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".atlas"), globalDir, "global config dir")

	// Test GlobalConfigPath
	globalPath, err := GlobalConfigPath()
	require.NoError(t, err, "GlobalConfigPath should succeed")
	assert.Equal(t, filepath.Join(home, ".atlas", "config.yaml"), globalPath, "global config path")
}

// TestConfig_Precedence_FullChain tests the complete precedence order:
// CLI > env > project > global > defaults
func TestConfig_Precedence_FullChain(t *testing.T) {
	ctx := context.Background()

	// Create temp directories for configs
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write global config - lowest precedence file
	globalConfig := filepath.Join(globalDir, "config.yaml")
	err := os.WriteFile(globalConfig, []byte(`
ai:
  model: global-model
  max_turns: 100
  timeout: 1h
git:
  base_branch: global-branch
  remote: global-remote
`), 0o600)
	require.NoError(t, err)

	// Write project config - overrides global
	projectConfig := filepath.Join(projectDir, "config.yaml")
	err = os.WriteFile(projectConfig, []byte(`
ai:
  model: project-model
  max_turns: 50
git:
  base_branch: project-branch
`), 0o600)
	require.NoError(t, err)

	// Set env var - overrides project config
	t.Setenv("ATLAS_AI_MODEL", "env-model")

	// Load config - project should override global, env should override project
	cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Verify precedence:
	// - ai.model: env-model (from env var, highest precedence)
	assert.Equal(t, "env-model", cfg.AI.Model, "env var should override project config")

	// - ai.max_turns: 50 (from project, project > global)
	assert.Equal(t, 50, cfg.AI.MaxTurns, "project config should override global")

	// - ai.timeout: 1h (from global, not overridden)
	assert.Equal(t, 1*time.Hour, cfg.AI.Timeout, "global config should be preserved when not overridden")

	// - git.base_branch: project-branch (from project, project > global)
	assert.Equal(t, "project-branch", cfg.Git.BaseBranch, "project config should override global")

	// - git.remote: global-remote (from global, not overridden in project)
	assert.Equal(t, "global-remote", cfg.Git.Remote, "global config should be preserved when not overridden")
}

// TestConfig_Precedence_EnvVarOverridesAllConfigFiles tests that env vars override both
// project and global config files.
func TestConfig_Precedence_EnvVarOverridesAllConfigFiles(t *testing.T) {
	ctx := context.Background()

	// Create temp directories for configs
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write global config
	globalConfig := filepath.Join(globalDir, "config.yaml")
	err := os.WriteFile(globalConfig, []byte(`
ai:
  model: global-model
`), 0o600)
	require.NoError(t, err)

	// Write project config
	projectConfig := filepath.Join(projectDir, "config.yaml")
	err = os.WriteFile(projectConfig, []byte(`
ai:
  model: project-model
`), 0o600)
	require.NoError(t, err)

	// Set env var to override
	t.Setenv("ATLAS_AI_MODEL", "env-model")

	// Load config
	cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Env var should win over both project and global
	assert.Equal(t, "env-model", cfg.AI.Model, "env var should override both project and global config")
}

// TestConfig_Precedence_CLIOverridesAll tests that CLI overrides (via LoadWithOverrides)
// override environment variables, project config, and global config.
func TestConfig_Precedence_CLIOverridesAll(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with project config
	tempDir := t.TempDir()
	atlasDir := filepath.Join(tempDir, ".atlas")
	err := os.MkdirAll(atlasDir, 0o750)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(`
ai:
  model: config-model
`), 0o600)
	require.NoError(t, err)

	// Change to the temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Set env var
	t.Setenv("ATLAS_AI_MODEL", "env-model")

	// Apply CLI override (highest precedence)
	overrides := &Config{
		AI: AIConfig{
			Model: "cli-model",
		},
	}

	cfg, err := LoadWithOverrides(ctx, overrides)
	require.NoError(t, err, "LoadWithOverrides should succeed")

	// CLI override should win over env var
	assert.Equal(t, "cli-model", cfg.AI.Model, "CLI override should have highest precedence")
}

// TestConfig_Precedence_NestedKeyMerging tests that nested keys are properly merged.
// For example: global has ai.model and ai.timeout, project has only ai.model.
// Result should have project's ai.model and global's ai.timeout.
func TestConfig_Precedence_NestedKeyMerging(t *testing.T) {
	ctx := context.Background()

	// Create temp directories for configs
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write global config with multiple nested keys
	globalConfig := filepath.Join(globalDir, "config.yaml")
	err := os.WriteFile(globalConfig, []byte(`
ai:
  model: opus
  timeout: 1h
  max_turns: 20
git:
  base_branch: main
  remote: origin
ci:
  timeout: 30m
  poll_interval: 2m
`), 0o600)
	require.NoError(t, err)

	// Write project config that overrides only some keys
	projectConfig := filepath.Join(projectDir, "config.yaml")
	err = os.WriteFile(projectConfig, []byte(`
ai:
  model: sonnet
git:
  base_branch: develop
`), 0o600)
	require.NoError(t, err)

	// Load config
	cfg, err := LoadFromPaths(ctx, projectConfig, globalConfig)
	require.NoError(t, err, "LoadFromPaths should succeed")

	// Verify project values override
	assert.Equal(t, "sonnet", cfg.AI.Model, "project should override ai.model")
	assert.Equal(t, "develop", cfg.Git.BaseBranch, "project should override git.base_branch")

	// Verify global values are preserved when not overridden
	assert.Equal(t, 1*time.Hour, cfg.AI.Timeout, "global ai.timeout should be preserved")
	assert.Equal(t, 20, cfg.AI.MaxTurns, "global ai.max_turns should be preserved")
	assert.Equal(t, "origin", cfg.Git.Remote, "global git.remote should be preserved")
	assert.Equal(t, 30*time.Minute, cfg.CI.Timeout, "global ci.timeout should be preserved")
	assert.Equal(t, 2*time.Minute, cfg.CI.PollInterval, "global ci.poll_interval should be preserved")
}

// TestConfig_Precedence_Documentation validates that the precedence order
// documented in Load() is correct.
func TestConfig_Precedence_Documentation(t *testing.T) {
	// This test documents the expected precedence order as stated in load.go:
	// 1. CLI flags (via LoadWithOverrides) - highest precedence
	// 2. Environment variables (ATLAS_* prefix)
	// 3. Project config (.atlas/config.yaml)
	// 4. Global config (~/.atlas/config.yaml)
	// 5. Built-in defaults - lowest precedence
	//
	// Each level is tested independently in other tests.
	// This test serves as documentation and a sanity check.

	ctx := context.Background()

	// Create temp directory with no config
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Load with defaults only
	cfg, err := Load(ctx)
	require.NoError(t, err)

	// Verify defaults are applied (level 5)
	assert.Equal(t, "sonnet", cfg.AI.Model, "default model should be sonnet")
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.APIKeyEnvVar, "default API key env var")
	assert.Equal(t, constants.DefaultAITimeout, cfg.AI.Timeout, "default AI timeout")
	assert.Equal(t, 10, cfg.AI.MaxTurns, "default max turns")
	assert.Equal(t, "main", cfg.Git.BaseBranch, "default base branch")
	assert.True(t, cfg.Git.AutoProceedGit, "default auto proceed git")
	assert.Equal(t, "origin", cfg.Git.Remote, "default remote")
	assert.True(t, cfg.Notifications.Bell, "default bell enabled")
}

// TestApplyOverrides_AllFields tests that all override fields are properly applied.
func TestApplyOverrides_AllFields(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	overrides := &Config{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "MY_API_KEY",
			Timeout:      45 * time.Minute,
			MaxTurns:     25,
		},
		Git: GitConfig{
			BaseBranch: "develop",
			Remote:     "upstream",
		},
		Worktree: WorktreeConfig{
			BaseDir:      "/custom/worktree",
			NamingSuffix: "-custom",
		},
		CI: CIConfig{
			Timeout:           2 * time.Hour,
			PollInterval:      5 * time.Minute,
			RequiredWorkflows: []string{"ci", "lint"},
		},
		Templates: TemplatesConfig{
			DefaultTemplate: "feature",
			CustomTemplates: map[string]string{"custom": "path/to/template"},
		},
		Validation: ValidationConfig{
			Commands: ValidationCommands{
				Format:      []string{"gofmt -w ."},
				Lint:        []string{"golangci-lint run"},
				Test:        []string{"go test ./..."},
				PreCommit:   []string{"pre-commit run"},
				CustomPrePR: []string{"custom-check"},
			},
			Timeout: 10 * time.Minute,
		},
		Notifications: NotificationsConfig{
			Events: []string{"completed", "failed"},
		},
	}

	cfg, err := LoadWithOverrides(ctx, overrides)
	require.NoError(t, err, "LoadWithOverrides should succeed")

	// Verify all AI overrides
	assert.Equal(t, "opus", cfg.AI.Model)
	assert.Equal(t, "MY_API_KEY", cfg.AI.APIKeyEnvVar)
	assert.Equal(t, 45*time.Minute, cfg.AI.Timeout)
	assert.Equal(t, 25, cfg.AI.MaxTurns)

	// Verify all Git overrides
	assert.Equal(t, "develop", cfg.Git.BaseBranch)
	assert.Equal(t, "upstream", cfg.Git.Remote)

	// Verify all Worktree overrides
	assert.Equal(t, "/custom/worktree", cfg.Worktree.BaseDir)
	assert.Equal(t, "-custom", cfg.Worktree.NamingSuffix)

	// Verify all CI overrides
	assert.Equal(t, 2*time.Hour, cfg.CI.Timeout)
	assert.Equal(t, 5*time.Minute, cfg.CI.PollInterval)
	assert.Equal(t, []string{"ci", "lint"}, cfg.CI.RequiredWorkflows)

	// Verify all Templates overrides
	assert.Equal(t, "feature", cfg.Templates.DefaultTemplate)
	assert.Equal(t, "path/to/template", cfg.Templates.CustomTemplates["custom"])

	// Verify all Validation overrides
	assert.Equal(t, []string{"gofmt -w ."}, cfg.Validation.Commands.Format)
	assert.Equal(t, []string{"golangci-lint run"}, cfg.Validation.Commands.Lint)
	assert.Equal(t, []string{"go test ./..."}, cfg.Validation.Commands.Test)
	assert.Equal(t, []string{"pre-commit run"}, cfg.Validation.Commands.PreCommit)
	assert.Equal(t, []string{"custom-check"}, cfg.Validation.Commands.CustomPrePR)
	assert.Equal(t, 10*time.Minute, cfg.Validation.Timeout)

	// Verify Notifications overrides
	assert.Equal(t, []string{"completed", "failed"}, cfg.Notifications.Events)
}

// TestApplyOverrides_PartialOverrides tests that only non-zero values are applied.
func TestApplyOverrides_PartialOverrides(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Only override AI.Model, leave everything else as zero values
	overrides := &Config{
		AI: AIConfig{
			Model: "opus",
		},
	}

	cfg, err := LoadWithOverrides(ctx, overrides)
	require.NoError(t, err)

	// Only Model should be overridden
	assert.Equal(t, "opus", cfg.AI.Model)

	// Other values should retain defaults
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.AI.APIKeyEnvVar)
	assert.Equal(t, constants.DefaultAITimeout, cfg.AI.Timeout)
	assert.Equal(t, 10, cfg.AI.MaxTurns)
	assert.Equal(t, "main", cfg.Git.BaseBranch)
	assert.Equal(t, "origin", cfg.Git.Remote)
}

// TestApplyOverrides_MergesCustomTemplates tests that custom templates are merged, not replaced.
func TestApplyOverrides_MergesCustomTemplates(t *testing.T) {
	ctx := context.Background()

	// Create temp directory with config that has custom templates
	tempDir := t.TempDir()
	atlasDir := filepath.Join(tempDir, ".atlas")
	err := os.MkdirAll(atlasDir, 0o750)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(`
templates:
  custom_templates:
    existing: path/to/existing
`), 0o600)
	require.NoError(t, err)

	// Change to the temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	overrides := &Config{
		Templates: TemplatesConfig{
			CustomTemplates: map[string]string{"new": "path/to/new"},
		},
	}

	cfg, err := LoadWithOverrides(ctx, overrides)
	require.NoError(t, err)

	// Both templates should be present (merged)
	assert.Equal(t, "path/to/existing", cfg.Templates.CustomTemplates["existing"])
	assert.Equal(t, "path/to/new", cfg.Templates.CustomTemplates["new"])
}
