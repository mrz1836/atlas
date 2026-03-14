package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestGlobalConfigDir_Success(t *testing.T) {
	dir, err := GlobalConfigDir()
	require.NoError(t, err)

	// Should contain .atlas
	assert.Contains(t, dir, constants.AtlasHome)

	// Should be absolute path
	assert.True(t, filepath.IsAbs(dir))
}

func TestGlobalConfigDir_HomeDirError(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
		}
	}()

	// Unset HOME to trigger error
	require.NoError(t, os.Unsetenv("HOME"))

	// On Unix, UserHomeDir() may still succeed by reading /etc/passwd
	// On some systems this test may not trigger the error path
	// So we verify the contract: if it fails, it returns an error
	dir, err := GlobalConfigDir()

	if err != nil {
		// Error path: dir should be empty
		assert.Empty(t, dir)
		assert.Contains(t, err.Error(), "failed to get home directory")
	} else {
		// Fallback succeeded, dir should be valid
		assert.NotEmpty(t, dir)
		assert.Contains(t, dir, constants.AtlasHome)
	}
}

func TestProjectConfigDir(t *testing.T) {
	dir := ProjectConfigDir()
	assert.Equal(t, constants.AtlasHome, dir)
}

func TestGlobalConfigPath_Success(t *testing.T) {
	path, err := GlobalConfigPath()
	require.NoError(t, err)

	assert.Contains(t, path, constants.AtlasHome)
	assert.Contains(t, path, "config.yaml")
	assert.True(t, filepath.IsAbs(path))
}

func TestGlobalConfigPath_HomeDirError(t *testing.T) {
	// Save original HOME
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalHome != "" {
			_ = os.Setenv("HOME", originalHome)
		}
	}()

	// Unset HOME
	require.NoError(t, os.Unsetenv("HOME"))

	path, err := GlobalConfigPath()

	if err != nil {
		// Error path: path should be empty
		assert.Empty(t, path)
		// Error is propagated from GlobalConfigDir
		assert.Error(t, err)
	} else {
		// Fallback succeeded
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "config.yaml")
	}
}

func TestProjectConfigPath(t *testing.T) {
	path := ProjectConfigPath()

	assert.Equal(t, filepath.Join(constants.AtlasHome, "config.yaml"), path)
	assert.Contains(t, path, ".atlas")
	assert.Contains(t, path, "config.yaml")
}
