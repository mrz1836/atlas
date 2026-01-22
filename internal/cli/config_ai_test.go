package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrz1836/atlas/internal/constants"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNewConfigCmd(t *testing.T) {
	t.Parallel()

	cmd := newConfigCmd()

	assert.Equal(t, "config", cmd.Use)
	assert.Contains(t, cmd.Short, "Manage")
	assert.Contains(t, cmd.Long, "configuration")

	// Verify 'ai' subcommand exists
	aiCmd, _, err := cmd.Find([]string{"ai"})
	require.NoError(t, err)
	assert.Equal(t, "ai", aiCmd.Use)
}

func TestNewConfigAICmd(t *testing.T) {
	t.Parallel()

	flags := &ConfigAIFlags{}
	cmd := newConfigAICmd(flags)

	assert.Equal(t, "ai", cmd.Use)
	assert.Contains(t, cmd.Short, "Configure AI")
	assert.Contains(t, cmd.Long, "AI provider settings")

	// Verify --no-interactive flag exists
	noInteractiveFlag := cmd.Flags().Lookup("no-interactive")
	require.NotNil(t, noInteractiveFlag)
	assert.Equal(t, "false", noInteractiveFlag.DefValue)
}

func TestAddConfigCommand(t *testing.T) {
	t.Parallel()

	rootCmd := newRootCmd(&GlobalFlags{}, BuildInfo{})
	AddConfigCommand(rootCmd)

	// Verify config command was added
	configCmd, _, err := rootCmd.Find([]string{"config"})
	require.NoError(t, err)
	assert.Equal(t, "config", configCmd.Use)

	// Verify ai subcommand exists under config
	aiCmd, _, err := configCmd.Find([]string{"ai"})
	require.NoError(t, err)
	assert.Equal(t, "ai", aiCmd.Use)
}

func TestRunConfigAI_NonInteractive_NoConfig(t *testing.T) {
	// Use temp HOME with no config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	flags := &ConfigAIFlags{NoInteractive: true}

	err := runConfigAI(context.Background(), &buf, flags)
	require.NoError(t, err)

	output := buf.String()
	// Should show warning about no config
	assert.Contains(t, output, "No existing configuration found")
	assert.Contains(t, output, "No AI configuration found")
}

func TestRunConfigAI_NonInteractive_WithExistingConfig(t *testing.T) {
	// Create temp HOME with existing config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create config directory and file
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	err := os.MkdirAll(atlasDir, 0o700)
	require.NoError(t, err)

	cfg := AtlasConfig{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "MY_CUSTOM_KEY",
			Timeout:      "1h",
			MaxTurns:     25,
		},
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)
	err = os.WriteFile(configPath, data, 0o600)
	require.NoError(t, err)

	// Set the env var for status check
	t.Setenv("MY_CUSTOM_KEY", "test-value")

	var buf bytes.Buffer
	flags := &ConfigAIFlags{NoInteractive: true}

	err = runConfigAI(context.Background(), &buf, flags)
	require.NoError(t, err)

	output := buf.String()

	// Should display current configuration
	assert.Contains(t, output, "Current AI Configuration")
	assert.Contains(t, output, "opus")
	assert.Contains(t, output, "MY_CUSTOM_KEY")
	assert.Contains(t, output, "1h")
	assert.Contains(t, output, "25")
	assert.Contains(t, output, "Set") // API key status
}

func TestRunConfigAI_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	flags := &ConfigAIFlags{NoInteractive: true}

	err := runConfigAI(ctx, &buf, flags)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestLoadExistingConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, path, err := loadExistingConfig()

	assert.Nil(t, cfg)
	assert.Contains(t, path, constants.GlobalConfigName)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrConfigNotFound)
}

func TestLoadExistingConfig_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create config file
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	err := os.MkdirAll(atlasDir, 0o700)
	require.NoError(t, err)

	expectedCfg := AtlasConfig{
		AI: AIConfig{
			Model:        "haiku",
			APIKeyEnvVar: "TEST_KEY",
			Timeout:      "45m",
			MaxTurns:     15,
		},
	}
	data, err := yaml.Marshal(expectedCfg)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)
	err = os.WriteFile(configPath, data, 0o600)
	require.NoError(t, err)

	cfg, path, err := loadExistingConfig()

	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, configPath, path)
	assert.Equal(t, "haiku", cfg.AI.Model)
	assert.Equal(t, "TEST_KEY", cfg.AI.APIKeyEnvVar)
	assert.Equal(t, "45m", cfg.AI.Timeout)
	assert.Equal(t, 15, cfg.AI.MaxTurns)
}

func TestLoadExistingConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create invalid YAML file
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	err := os.MkdirAll(atlasDir, 0o700)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)
	err = os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0o600)
	require.NoError(t, err)

	cfg, _, err := loadExistingConfig()

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

func TestSaveAtlasConfig_WithConfigAIHeader(t *testing.T) {
	// Use temp HOME directory to test the shared saveAtlasConfig function
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := AtlasConfig{
		AI: AIConfig{
			Model:        "sonnet",
			APIKeyEnvVar: "ANTHROPIC_API_KEY",
			Timeout:      "30m",
			MaxTurns:     10,
		},
	}

	err := saveAtlasConfig(cfg, "Updated by atlas config ai")
	require.NoError(t, err)

	// Verify file was created
	configPath := filepath.Join(tmpDir, constants.AtlasHome, constants.GlobalConfigName)
	assert.FileExists(t, configPath)

	// Verify content
	content, err := os.ReadFile(configPath) //nolint:gosec // Test file
	require.NoError(t, err)
	assert.Contains(t, string(content), "# ATLAS Configuration")
	assert.Contains(t, string(content), "Updated by atlas config ai")
	assert.Contains(t, string(content), "model: sonnet")
	assert.Contains(t, string(content), "api_key_env_var: ANTHROPIC_API_KEY")
}

func TestNewConfigAIStyles(t *testing.T) {
	t.Parallel()

	styles := newConfigAIStyles()

	// Verify all styles are initialized
	assert.NotEmpty(t, styles.header.Render("test"))
	assert.NotEmpty(t, styles.success.Render("test"))
	assert.NotEmpty(t, styles.warning.Render("test"))
	assert.NotEmpty(t, styles.dim.Render("test"))
	assert.NotEmpty(t, styles.key.Render("test"))
	assert.NotEmpty(t, styles.value.Render("test"))
}

func TestDisplayCurrentAIConfig(t *testing.T) {
	t.Parallel()

	cfg := &AtlasConfig{
		AI: AIConfig{
			Model:        "opus",
			APIKeyEnvVar: "TEST_API_KEY",
			Timeout:      "2h",
			MaxTurns:     50,
		},
	}

	var buf bytes.Buffer
	styles := newConfigAIStyles()

	displayCurrentAIConfig(&buf, cfg, styles)

	output := buf.String()

	assert.Contains(t, output, "Current AI Configuration")
	assert.Contains(t, output, "opus")
	assert.Contains(t, output, "TEST_API_KEY")
	assert.Contains(t, output, "2h")
	assert.Contains(t, output, "50")
}

func TestDisplayCurrentAIConfig_APIKeyNotSet(t *testing.T) {
	cfg := &AtlasConfig{
		AI: AIConfig{
			Model:        "sonnet",
			APIKeyEnvVar: "DEFINITELY_NOT_SET_KEY_XYZ",
			Timeout:      "30m",
			MaxTurns:     10,
		},
	}

	var buf bytes.Buffer
	styles := newConfigAIStyles()

	displayCurrentAIConfig(&buf, cfg, styles)

	output := buf.String()
	assert.Contains(t, output, "Not set")
}

func TestDisplayCurrentAIConfig_APIKeySet(t *testing.T) {
	t.Setenv("TEST_SET_API_KEY", "some-value")

	cfg := &AtlasConfig{
		AI: AIConfig{
			Model:        "sonnet",
			APIKeyEnvVar: "TEST_SET_API_KEY",
			Timeout:      "30m",
			MaxTurns:     10,
		},
	}

	var buf bytes.Buffer
	styles := newConfigAIStyles()

	displayCurrentAIConfig(&buf, cfg, styles)

	output := buf.String()
	assert.Contains(t, output, "Set")
}

func TestConfigAIFlags(t *testing.T) {
	t.Parallel()

	flags := &ConfigAIFlags{NoInteractive: true}
	cmd := newConfigAICmd(flags)

	// Test that flag is properly bound
	err := cmd.Flags().Set("no-interactive", "true")
	require.NoError(t, err)
	assert.True(t, flags.NoInteractive)

	err = cmd.Flags().Set("no-interactive", "false")
	require.NoError(t, err)
	assert.False(t, flags.NoInteractive)
}
