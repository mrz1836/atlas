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
