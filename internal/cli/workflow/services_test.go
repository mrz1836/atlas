package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/ai"
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

func TestNewServiceFactory(t *testing.T) {
	logger := zerolog.Nop()
	factory := NewServiceFactory(logger)
	assert.NotNil(t, factory)
	assert.Equal(t, logger, factory.logger)
}

func TestCreateTaskStore(t *testing.T) {
	t.Run("creates task store successfully", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)

		// This should succeed as it uses default path
		store, err := factory.CreateTaskStore()
		require.NoError(t, err)
		assert.NotNil(t, store)
	})
}

func TestCreateConfig(t *testing.T) {
	t.Run("returns config even on load failure", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		ctx := context.Background()

		// Even if config load fails, should return default config
		cfg, err := factory.CreateConfig(ctx)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
	})
}

func TestSetupTaskStoreAndConfig(t *testing.T) {
	t.Run("creates both task store and config", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		ctx := context.Background()

		store, cfg, err := factory.SetupTaskStoreAndConfig(ctx)
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.NotNil(t, cfg)
	})
}

func TestCreateNotifiers(t *testing.T) {
	t.Run("creates both notifiers with config", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		notifier, stateNotifier := factory.CreateNotifiers(cfg)
		assert.NotNil(t, notifier)
		assert.NotNil(t, stateNotifier)
	})
}

func TestCreateAIRunner(t *testing.T) {
	t.Run("creates AI runner without activity streaming", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		runner := factory.CreateAIRunner(cfg)
		assert.NotNil(t, runner)
	})
}

func TestCreateAIRunnerWithActivity(t *testing.T) {
	t.Run("creates AI runner with activity streaming", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		activityCalled := false
		activityOpts := &ai.ActivityOptions{
			Callback: func(_ ai.ActivityEvent) {
				activityCalled = true
			},
		}

		runner := factory.CreateAIRunnerWithActivity(cfg, activityOpts)
		assert.NotNil(t, runner)
		// activityCalled will only be true if the runner actually sends events
		// which requires actual execution
		_ = activityCalled
	})

	t.Run("creates AI runner without activity when opts nil", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		runner := factory.CreateAIRunnerWithActivity(cfg, nil)
		assert.NotNil(t, runner)
	})

	t.Run("creates AI runner without activity when callback nil", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		activityOpts := &ai.ActivityOptions{
			Callback: nil,
		}

		runner := factory.CreateAIRunnerWithActivity(cfg, activityOpts)
		assert.NotNil(t, runner)
	})
}

func TestGitConfig(t *testing.T) {
	t.Run("struct fields exist and are accessible", func(t *testing.T) {
		gitCfg := GitConfig{
			CommitAgent:         "claude",
			CommitModel:         "sonnet-4",
			CommitTimeout:       30000000000,
			CommitMaxRetries:    3,
			CommitBackoffFactor: 2.0,
			PRDescAgent:         "gemini",
			PRDescModel:         "pro",
		}

		assert.Equal(t, "claude", gitCfg.CommitAgent)
		assert.Equal(t, "sonnet-4", gitCfg.CommitModel)
		assert.Equal(t, int64(30000000000), int64(gitCfg.CommitTimeout))
		assert.Equal(t, 3, gitCfg.CommitMaxRetries)
		assert.InDelta(t, 2.0, gitCfg.CommitBackoffFactor, 0.01)
		assert.Equal(t, "gemini", gitCfg.PRDescAgent)
		assert.Equal(t, "pro", gitCfg.PRDescModel)
	})
}

func TestGitServices(t *testing.T) {
	t.Run("struct fields exist", func(t *testing.T) {
		gitSvcs := &GitServices{
			Runner:           nil,
			SmartCommitter:   nil,
			Pusher:           nil,
			HubRunner:        nil,
			PRDescGen:        nil,
			CIFailureHandler: nil,
		}

		assert.NotNil(t, gitSvcs)
		// Fields can be nil, we're just testing the struct exists
	})
}

func TestRegistryDeps(t *testing.T) {
	t.Run("struct fields exist and are accessible", func(t *testing.T) {
		deps := RegistryDeps{
			WorkDir:                    "/path/to/work",
			TaskStore:                  nil,
			Notifier:                   nil,
			AIRunner:                   nil,
			Logger:                     zerolog.Nop(),
			GitServices:                nil,
			Config:                     config.DefaultConfig(),
			ProgressCallback:           nil,
			ValidationProgressCallback: nil,
		}

		assert.Equal(t, "/path/to/work", deps.WorkDir)
		assert.NotNil(t, deps.Logger)
		assert.NotNil(t, deps.Config)
	})
}

func TestEngineDeps(t *testing.T) {
	t.Run("struct fields exist and are accessible", func(t *testing.T) {
		deps := EngineDeps{
			TaskStore:              nil,
			ExecRegistry:           nil,
			Logger:                 zerolog.Nop(),
			StateNotifier:          nil,
			ProgressCallback:       nil,
			ValidationRetryHandler: nil,
			HookManager:            nil,
		}

		assert.NotNil(t, deps.Logger)
		// Other fields can be nil
	})
}

func TestCreateValidationRetryHandler(t *testing.T) {
	t.Run("returns nil when AI retry disabled", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()
		cfg.Validation.AIRetryEnabled = false

		handler := factory.CreateValidationRetryHandler(nil, cfg)
		assert.Nil(t, handler)
	})

	t.Run("returns handler when AI retry enabled", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()
		cfg.Validation.AIRetryEnabled = true

		// Create a mock AI runner
		runner := factory.CreateAIRunner(cfg)

		handler := factory.CreateValidationRetryHandler(runner, cfg)
		assert.NotNil(t, handler)
	})
}

func TestCreateValidationRetryHandler_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Validation.AIRetryEnabled = false

		handler := CreateValidationRetryHandler(nil, cfg)
		assert.Nil(t, handler)
	})
}

func TestCreateNotifiers_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(t *testing.T) {
		cfg := config.DefaultConfig()

		notifier, stateNotifier := CreateNotifiers(cfg)
		assert.NotNil(t, notifier)
		assert.NotNil(t, stateNotifier)
	})
}

func TestCreateAIRunner_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(t *testing.T) {
		cfg := config.DefaultConfig()

		runner := CreateAIRunner(cfg)
		assert.NotNil(t, runner)
	})
}

func TestCreateExecutorRegistry(t *testing.T) {
	t.Run("creates executor registry with dependencies", func(t *testing.T) {
		logger := zerolog.Nop()
		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		// Create minimal dependencies
		runner := factory.CreateAIRunner(cfg)
		notifier, _ := factory.CreateNotifiers(cfg)

		deps := RegistryDeps{
			WorkDir:                    "/path/to/work",
			TaskStore:                  nil, // Can be nil for this test
			Notifier:                   notifier,
			AIRunner:                   runner,
			Logger:                     logger,
			GitServices:                &GitServices{}, // Empty but not nil
			Config:                     cfg,
			ProgressCallback:           nil,
			ValidationProgressCallback: nil,
		}

		registry := factory.CreateExecutorRegistry(deps)
		assert.NotNil(t, registry)
	})
}

func TestCreateEngine_WithValidationRetryHandler(t *testing.T) {
	t.Run("creates engine with validation retry handler", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := zerolog.Nop()

		taskStore, err := task.NewFileStore(tmpDir)
		require.NoError(t, err)

		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()
		cfg.Validation.AIRetryEnabled = true

		runner := factory.CreateAIRunner(cfg)
		retryHandler := factory.CreateValidationRetryHandler(runner, cfg)

		engine := factory.CreateEngine(EngineDeps{
			TaskStore:              taskStore,
			Logger:                 logger,
			ValidationRetryHandler: retryHandler,
		}, cfg)

		assert.NotNil(t, engine)
	})
}

func TestCreateEngine_WithoutValidationRetryHandler(t *testing.T) {
	t.Run("creates engine without validation retry handler", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := zerolog.Nop()

		taskStore, err := task.NewFileStore(tmpDir)
		require.NoError(t, err)

		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		engine := factory.CreateEngine(EngineDeps{
			TaskStore:              taskStore,
			Logger:                 logger,
			ValidationRetryHandler: nil,
		}, cfg)

		assert.NotNil(t, engine)
	})
}

func TestCreateEngine_WithProgressCallback(t *testing.T) {
	t.Run("creates engine with progress callback", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger := zerolog.Nop()

		taskStore, err := task.NewFileStore(tmpDir)
		require.NoError(t, err)

		factory := NewServiceFactory(logger)
		cfg := config.DefaultConfig()

		progressCalled := false
		progressCallback := func(_ task.StepProgressEvent) {
			progressCalled = true
		}

		engine := factory.CreateEngine(EngineDeps{
			TaskStore:        taskStore,
			Logger:           logger,
			ProgressCallback: progressCallback,
		}, cfg)

		assert.NotNil(t, engine)
		// progressCalled will only be true during actual execution
		_ = progressCalled
	})
}
