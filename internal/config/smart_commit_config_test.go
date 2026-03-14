package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartCommitConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFromPaths(context.Background(), "", "")
	require.NoError(t, err)

	// Verify smart_commit defaults
	assert.Equal(t, 30*time.Second, cfg.SmartCommit.Timeout, "default timeout should be 30s")
	assert.Equal(t, 2, cfg.SmartCommit.MaxRetries, "default max_retries should be 2")
	assert.InEpsilon(t, 1.5, cfg.SmartCommit.RetryBackoffFactor, 0.001, "default retry_backoff_factor should be 1.5")
}

func TestSmartCommitConfig_LoadFromFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a config file with custom smart_commit settings
	configContent := `
ai:
  agent: claude
  model: sonnet

smart_commit:
  agent: claude
  model: haiku
  timeout: 45s
  max_retries: 3
  retry_backoff_factor: 2.0
`

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	// Verify smart_commit config was loaded
	assert.Equal(t, "claude", cfg.SmartCommit.Agent)
	assert.Equal(t, "haiku", cfg.SmartCommit.Model)
	assert.Equal(t, 45*time.Second, cfg.SmartCommit.Timeout)
	assert.Equal(t, 3, cfg.SmartCommit.MaxRetries)
	assert.InEpsilon(t, 2.0, cfg.SmartCommit.RetryBackoffFactor, 0.001)
}

func TestSmartCommitConfig_TimeoutParsing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		timeoutStr     string
		expectedResult time.Duration
	}{
		{"seconds", "30s", 30 * time.Second},
		{"minutes", "2m", 2 * time.Minute},
		{"mixed", "1m30s", 90 * time.Second},
		{"hours", "1h", 1 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()

			configContent := `
smart_commit:
  timeout: ` + tt.timeoutStr + `
`

			configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

			cfg, err := LoadFromPaths(context.Background(), configPath, "")
			require.NoError(t, err)

			assert.Equal(t, tt.expectedResult, cfg.SmartCommit.Timeout)
		})
	}
}

func TestSmartCommitConfig_PartialConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Only specify some smart_commit fields
	configContent := `
smart_commit:
  timeout: 60s
  # max_retries and retry_backoff_factor use defaults
`

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	// Custom value
	assert.Equal(t, 60*time.Second, cfg.SmartCommit.Timeout)

	// Defaults
	assert.Equal(t, 2, cfg.SmartCommit.MaxRetries)
	assert.InEpsilon(t, 1.5, cfg.SmartCommit.RetryBackoffFactor, 0.001)
}

func TestSmartCommitConfig_ZeroValues(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Explicitly set to zero/empty values
	configContent := `
smart_commit:
  timeout: 0s
  max_retries: 0
  retry_backoff_factor: 0
`

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	// Zero values should be preserved (not replaced with defaults)
	assert.Equal(t, time.Duration(0), cfg.SmartCommit.Timeout)
	assert.Equal(t, 0, cfg.SmartCommit.MaxRetries)
	assert.InDelta(t, 0.0, cfg.SmartCommit.RetryBackoffFactor, 0.001)
}

func TestSmartCommitConfig_AgentModelFallback(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// smart_commit.agent and smart_commit.model are empty, should use ai.agent and ai.model
	configContent := `
ai:
  agent: claude
  model: sonnet

smart_commit:
  timeout: 30s
  # agent and model not specified, should fall back to ai.* in CLI code
`

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	// SmartCommit should have empty agent/model (fallback happens in CLI layer)
	assert.Empty(t, cfg.SmartCommit.Agent)
	assert.Empty(t, cfg.SmartCommit.Model)

	// AI config should have values
	assert.Equal(t, "claude", cfg.AI.Agent)
	assert.Equal(t, "sonnet", cfg.AI.Model)

	// Timeout should be loaded
	assert.Equal(t, 30*time.Second, cfg.SmartCommit.Timeout)
}

func TestSmartCommitConfig_ExtremeValues(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Test with extreme but valid values
	configContent := `
smart_commit:
  timeout: 10m
  max_retries: 10
  retry_backoff_factor: 5.0
`

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	assert.Equal(t, 10*time.Minute, cfg.SmartCommit.Timeout)
	assert.Equal(t, 10, cfg.SmartCommit.MaxRetries)
	assert.InEpsilon(t, 5.0, cfg.SmartCommit.RetryBackoffFactor, 0.001)
}

func TestSmartCommitConfig_EmptyConfigFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Empty config file - should use all defaults
	configContent := ``

	configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	cfg, err := LoadFromPaths(context.Background(), configPath, "")
	require.NoError(t, err)

	// All should be defaults
	assert.Equal(t, 30*time.Second, cfg.SmartCommit.Timeout)
	assert.Equal(t, 2, cfg.SmartCommit.MaxRetries)
	assert.InEpsilon(t, 1.5, cfg.SmartCommit.RetryBackoffFactor, 0.001)
}

func TestSmartCommitConfig_DecimalBackoffFactor(t *testing.T) {
	t.Parallel()

	// Test various decimal values
	tests := []struct {
		factor   string
		expected float64
	}{
		{"1.0", 1.0},
		{"1.5", 1.5},
		{"2.0", 2.0},
		{"2.5", 2.5},
		{"1.25", 1.25},
		{"3.75", 3.75},
	}

	for _, tt := range tests {
		t.Run(tt.factor, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()

			configContent := `
smart_commit:
  retry_backoff_factor: ` + tt.factor + `
`

			configPath := filepath.Join(tmpDir, ".atlas", "config.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0o750))
			require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

			cfg, err := LoadFromPaths(context.Background(), configPath, "")
			require.NoError(t, err)

			assert.InDelta(t, tt.expected, cfg.SmartCommit.RetryBackoffFactor, 0.001)
		})
	}
}
