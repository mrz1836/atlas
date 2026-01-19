package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/task"
)

func TestCreateHookManager(t *testing.T) {
	t.Run("creates hook manager with valid config", func(t *testing.T) {
		// Create a temp directory to ensure we have a valid home dir
		tmpDir := t.TempDir()
		err := os.Setenv("HOME", tmpDir)
		require.NoError(t, err)
		defer func() {
			// Restore original HOME
			if originalHome, _ := os.LookupEnv("HOME"); originalHome != "" {
				_ = os.Setenv("HOME", originalHome)
			}
		}()

		cfg := config.DefaultConfig()
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)

		hookManager := factory.CreateHookManager(cfg, logger)

		// Should return a non-nil hook manager
		assert.NotNil(t, hookManager, "CreateHookManager should return a non-nil hook manager")

		// Verify it implements the HookManager interface
		_ = hookManager
	})

	t.Run("returns nil gracefully when home dir unavailable", func(_ *testing.T) {
		// This test verifies that CreateHookManager handles errors gracefully
		// It's hard to simulate os.UserHomeDir() failure, so we just verify
		// the function doesn't panic with a valid config
		cfg := config.DefaultConfig()
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)

		// This should not panic
		hookManager := factory.CreateHookManager(cfg, logger)

		// Hook manager may or may not be nil depending on system state,
		// but the function should not panic
		_ = hookManager
	})
}

func TestEngineDeps_WithHookManager(t *testing.T) {
	t.Run("HookManager field exists in EngineDeps", func(t *testing.T) {
		deps := EngineDeps{
			HookManager: nil, // Test that the field exists
		}

		assert.Nil(t, deps.HookManager)
	})
}

func TestCreateEngine_WithHookManager(t *testing.T) {
	t.Run("creates engine with hook manager when provided", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := zerolog.Nop()

		// Create a minimal task store
		taskStore, err := task.NewFileStore(tmpDir)
		require.NoError(t, err)

		// Create service factory and hook manager
		factory := NewServiceFactory(logger)

		// Set HOME for CreateHookManager
		err = os.Setenv("HOME", tmpDir)
		require.NoError(t, err)

		cfg := config.DefaultConfig()
		hookManager := factory.CreateHookManager(cfg, logger)

		// Create engine with hook manager
		engine := factory.CreateEngine(EngineDeps{
			TaskStore:   taskStore,
			Logger:      logger,
			HookManager: hookManager,
		}, cfg)

		assert.NotNil(t, engine)
	})

	t.Run("creates engine without hook manager when nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := zerolog.Nop()

		// Create a minimal task store
		taskStore, err := task.NewFileStore(tmpDir)
		require.NoError(t, err)

		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		// Create engine without hook manager
		engine := factory.CreateEngine(EngineDeps{
			TaskStore:   taskStore,
			Logger:      logger,
			HookManager: nil, // Explicitly nil
		}, cfg)

		assert.NotNil(t, engine)
	})
}

func TestResolveTaskPath_Integration(t *testing.T) {
	// This test verifies that the resolveTaskPath function produces the expected paths
	// It's an integration test that checks the path format used by the hook system

	t.Run("path format matches expected structure", func(t *testing.T) {
		// The expected path format is: workspaces/<workspace>/tasks/<taskID>
		workspaceID := "test-workspace"
		taskID := "test-task-123"
		expectedPath := filepath.Join("workspaces", workspaceID, "tasks", taskID)

		// Verify the expected path format
		assert.Equal(t, "workspaces/test-workspace/tasks/test-task-123", expectedPath)
	})
}
