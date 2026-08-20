package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// cleanupGitExecutor is a fake git step executor that also implements the
// optional CleanupOnPause hook used by Engine.cleanupOnPause.
type cleanupGitExecutor struct {
	called bool
	path   string
	err    error
}

func (c *cleanupGitExecutor) Execute(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	return &domain.StepResult{Status: constants.StepStatusSuccess}, nil
}

func (c *cleanupGitExecutor) Type() domain.StepType {
	return domain.StepTypeGit
}

func (c *cleanupGitExecutor) CleanupOnPause(_ context.Context, worktreePath string) error {
	c.called = true
	c.path = worktreePath
	return c.err
}

// TestEngine_SetMetadata verifies setMetadata initializes a nil map and
// sets/overwrites individual keys.
func TestEngine_SetMetadata(t *testing.T) {
	t.Parallel()

	newEngine := func() *Engine {
		return NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
	}

	t.Run("initializes_nil_metadata", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t1", Metadata: nil}

		engine.setMetadata(task, "key", "value")

		require.NotNil(t, task.Metadata)
		assert.Equal(t, "value", task.Metadata["key"])
	})

	t.Run("preserves_and_overwrites_existing_metadata", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t2", Metadata: map[string]any{"keep": 1, "key": "old"}}

		engine.setMetadata(task, "key", "new")

		assert.Equal(t, 1, task.Metadata["keep"])
		assert.Equal(t, "new", task.Metadata["key"])
	})

	t.Run("stores_non_string_values", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t3"}

		engine.setMetadata(task, "flag", true)
		engine.setMetadata(task, "count", 7)

		assert.Equal(t, true, task.Metadata["flag"])
		assert.Equal(t, 7, task.Metadata["count"])
	})
}

// TestEngine_SetMetadataMultiple verifies setMetadataMultiple initializes a nil
// map, writes every supplied pair, and preserves unrelated existing keys.
func TestEngine_SetMetadataMultiple(t *testing.T) {
	t.Parallel()

	newEngine := func() *Engine {
		return NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())
	}

	t.Run("initializes_nil_metadata_and_sets_all", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t1", Metadata: nil}

		engine.setMetadataMultiple(task, map[string]any{"a": 1, "b": "two"})

		require.NotNil(t, task.Metadata)
		assert.Equal(t, 1, task.Metadata["a"])
		assert.Equal(t, "two", task.Metadata["b"])
	})

	t.Run("preserves_existing_and_overwrites_collisions", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t2", Metadata: map[string]any{"keep": "me", "a": "old"}}

		engine.setMetadataMultiple(task, map[string]any{"a": "new", "c": 3})

		assert.Equal(t, "me", task.Metadata["keep"])
		assert.Equal(t, "new", task.Metadata["a"])
		assert.Equal(t, 3, task.Metadata["c"])
	})

	t.Run("empty_map_leaves_metadata_initialized_but_unchanged", func(t *testing.T) {
		t.Parallel()
		engine := newEngine()
		task := &domain.Task{ID: "t3", Metadata: nil}

		engine.setMetadataMultiple(task, map[string]any{})

		require.NotNil(t, task.Metadata)
		assert.Empty(t, task.Metadata)
	})
}

// TestEngine_MapStepTypeToErrorStatus_VerifyAndLoop closes the coverage gap for
// the Verify and Loop step types, which map to ValidationFailed alongside AI,
// Human, and SDD.
func TestEngine_MapStepTypeToErrorStatus_VerifyAndLoop(t *testing.T) {
	t.Parallel()

	engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())

	tests := []struct {
		name     string
		stepType domain.StepType
	}{
		{name: "verify_maps_to_validation_failed", stepType: domain.StepTypeVerify},
		{name: "loop_maps_to_validation_failed", stepType: domain.StepTypeLoop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, constants.TaskStatusValidationFailed, engine.mapStepTypeToErrorStatus(tt.stepType))
		})
	}
}

// TestEngine_CleanupOnPause verifies that cleanupOnPause extracts the worktree
// path from task metadata and invokes the git executor's optional CleanupOnPause
// hook, tolerating missing executors, incompatible executors, and hook errors.
func TestEngine_CleanupOnPause(t *testing.T) {
	t.Parallel()

	t.Run("invokes_cleanup_with_worktree_path_from_metadata", func(t *testing.T) {
		t.Parallel()
		gitExec := &cleanupGitExecutor{}
		registry := steps.NewExecutorRegistry()
		registry.Register(gitExec)
		engine := NewEngine(newMockStore(), registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:       "task-cleanup",
			Metadata: map[string]any{"worktree_dir": "/tmp/worktree-xyz"},
		}

		engine.cleanupOnPause(context.Background(), task)

		assert.True(t, gitExec.called, "CleanupOnPause should be called")
		assert.Equal(t, "/tmp/worktree-xyz", gitExec.path)
	})

	t.Run("uses_empty_path_when_metadata_nil", func(t *testing.T) {
		t.Parallel()
		gitExec := &cleanupGitExecutor{}
		registry := steps.NewExecutorRegistry()
		registry.Register(gitExec)
		engine := NewEngine(newMockStore(), registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{ID: "task-cleanup-nometa", Metadata: nil}

		engine.cleanupOnPause(context.Background(), task)

		assert.True(t, gitExec.called)
		assert.Empty(t, gitExec.path)
	})

	t.Run("uses_empty_path_when_worktree_dir_not_a_string", func(t *testing.T) {
		t.Parallel()
		gitExec := &cleanupGitExecutor{}
		registry := steps.NewExecutorRegistry()
		registry.Register(gitExec)
		engine := NewEngine(newMockStore(), registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:       "task-cleanup-badtype",
			Metadata: map[string]any{"worktree_dir": 123},
		}

		engine.cleanupOnPause(context.Background(), task)

		assert.True(t, gitExec.called)
		assert.Empty(t, gitExec.path)
	})

	t.Run("tolerates_cleanup_error", func(t *testing.T) {
		t.Parallel()
		gitExec := &cleanupGitExecutor{err: errTest}
		registry := steps.NewExecutorRegistry()
		registry.Register(gitExec)
		engine := NewEngine(newMockStore(), registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:       "task-cleanup-err",
			Metadata: map[string]any{"worktree_dir": "/tmp/wt"},
		}

		assert.NotPanics(t, func() {
			engine.cleanupOnPause(context.Background(), task)
		})
		assert.True(t, gitExec.called)
	})

	t.Run("no_git_executor_registered_is_a_noop", func(t *testing.T) {
		t.Parallel()
		engine := NewEngine(newMockStore(), steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:       "task-cleanup-noexec",
			Metadata: map[string]any{"worktree_dir": "/tmp/wt"},
		}

		assert.NotPanics(t, func() {
			engine.cleanupOnPause(context.Background(), task)
		})
	})

	t.Run("git_executor_without_cleanup_hook_is_a_noop", func(t *testing.T) {
		t.Parallel()
		registry := steps.NewExecutorRegistry()
		// mockExecutor implements StepExecutor but NOT CleanupOnPause.
		registry.Register(&mockExecutor{stepType: domain.StepTypeGit})
		engine := NewEngine(newMockStore(), registry, DefaultEngineConfig(), testLogger())

		task := &domain.Task{
			ID:       "task-cleanup-nohook",
			Metadata: map[string]any{"worktree_dir": "/tmp/wt"},
		}

		assert.NotPanics(t, func() {
			engine.cleanupOnPause(context.Background(), task)
		})
	})
}
