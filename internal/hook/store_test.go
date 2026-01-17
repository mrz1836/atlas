package hook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestFileStore_Create(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("creates new hook", func(t *testing.T) {
		taskID := "task-001"
		workspaceID := "ws-001"

		hook, err := store.Create(ctx, taskID, workspaceID)
		require.NoError(t, err)
		require.NotNil(t, hook)

		assert.Equal(t, taskID, hook.TaskID)
		assert.Equal(t, workspaceID, hook.WorkspaceID)
		assert.Equal(t, domain.HookStateInitializing, hook.State)
		assert.Equal(t, constants.HookSchemaVersion, hook.Version)
		assert.Equal(t, constants.HookSchemaVersion, hook.SchemaVersion)
		assert.NotZero(t, hook.CreatedAt)
		assert.NotZero(t, hook.UpdatedAt)

		// Verify file was created
		hookPath := filepath.Join(tmpDir, taskID, constants.HookFileName)
		_, err = os.Stat(hookPath)
		require.NoError(t, err)
	})

	t.Run("fails if hook already exists", func(t *testing.T) {
		taskID := "task-002"
		workspaceID := "ws-002"

		// Create first time
		_, err := store.Create(ctx, taskID, workspaceID)
		require.NoError(t, err)

		// Try to create again
		_, err = store.Create(ctx, taskID, workspaceID)
		assert.ErrorIs(t, err, ErrHookExists)
	})
}

func TestFileStore_Get(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("gets existing hook", func(t *testing.T) {
		taskID := "task-get-001"

		// Create hook first
		created, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		// Get it back
		retrieved, err := store.Get(ctx, taskID)
		require.NoError(t, err)

		assert.Equal(t, created.TaskID, retrieved.TaskID)
		assert.Equal(t, created.WorkspaceID, retrieved.WorkspaceID)
		assert.Equal(t, created.State, retrieved.State)
	})

	t.Run("returns error for non-existent hook", func(t *testing.T) {
		_, err := store.Get(ctx, "non-existent-task")
		assert.ErrorIs(t, err, ErrHookNotFound)
	})

	t.Run("returns error for corrupted hook", func(t *testing.T) {
		taskID := "task-corrupted"
		hookDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(hookDir, 0o750))

		hookPath := filepath.Join(hookDir, constants.HookFileName)
		require.NoError(t, os.WriteFile(hookPath, []byte("invalid json"), 0o600))

		_, err := store.Get(ctx, taskID)
		assert.ErrorIs(t, err, ErrInvalidHook)
	})
}

func TestFileStore_Save(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("saves hook state", func(t *testing.T) {
		taskID := "task-save-001"

		// Create hook
		hook, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		// Modify and save
		hook.State = domain.HookStateStepRunning
		hook.CurrentStep = &domain.StepContext{
			StepName:  "implement",
			StepIndex: 2,
			Attempt:   1,
		}

		err = store.Save(ctx, hook)
		require.NoError(t, err)

		// Verify changes were persisted
		retrieved, err := store.Get(ctx, taskID)
		require.NoError(t, err)

		assert.Equal(t, domain.HookStateStepRunning, retrieved.State)
		require.NotNil(t, retrieved.CurrentStep)
		assert.Equal(t, "implement", retrieved.CurrentStep.StepName)
	})

	t.Run("updates timestamp on save", func(t *testing.T) {
		taskID := "task-save-002"

		hook, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		originalUpdatedAt := hook.UpdatedAt
		time.Sleep(10 * time.Millisecond)

		err = store.Save(ctx, hook)
		require.NoError(t, err)

		assert.True(t, hook.UpdatedAt.After(originalUpdatedAt))
	})
}

func TestFileStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("deletes existing hook", func(t *testing.T) {
		taskID := "task-delete-001"

		// Create hook
		_, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		// Verify it exists
		exists, err := store.Exists(ctx, taskID)
		require.NoError(t, err)
		assert.True(t, exists)

		// Delete it
		err = store.Delete(ctx, taskID)
		require.NoError(t, err)

		// Verify it's gone
		exists, err = store.Exists(ctx, taskID)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("succeeds for non-existent hook", func(t *testing.T) {
		err := store.Delete(ctx, "non-existent")
		assert.NoError(t, err)
	})
}

func TestFileStore_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("returns true for existing hook", func(t *testing.T) {
		taskID := "task-exists-001"

		_, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		exists, err := store.Exists(ctx, taskID)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for non-existent hook", func(t *testing.T) {
		exists, err := store.Exists(ctx, "non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestFileStore_ListStale(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("finds stale non-terminal hooks", func(t *testing.T) {
		// Create a stale hook (manually set old timestamp)
		taskID := "task-stale-001"
		hookDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(hookDir, 0o750))

		staleHook := &domain.Hook{
			Version:       "1.0",
			TaskID:        taskID,
			WorkspaceID:   "ws-001",
			CreatedAt:     time.Now().Add(-10 * time.Minute),
			UpdatedAt:     time.Now().Add(-10 * time.Minute), // 10 minutes old
			State:         domain.HookStateStepRunning,
			SchemaVersion: "1.0",
		}

		data, err := json.MarshalIndent(staleHook, "", "  ")
		require.NoError(t, err)

		hookPath := filepath.Join(hookDir, constants.HookFileName)
		require.NoError(t, os.WriteFile(hookPath, data, 0o600))

		// List with 5 minute threshold
		stale, err := store.ListStale(ctx, 5*time.Minute)
		require.NoError(t, err)

		require.Len(t, stale, 1)
		assert.Equal(t, taskID, stale[0].TaskID)
	})

	t.Run("excludes terminal state hooks", func(t *testing.T) {
		taskID := "task-terminal-001"
		hookDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(hookDir, 0o750))

		terminalHook := &domain.Hook{
			Version:       "1.0",
			TaskID:        taskID,
			WorkspaceID:   "ws-001",
			CreatedAt:     time.Now().Add(-10 * time.Minute),
			UpdatedAt:     time.Now().Add(-10 * time.Minute),
			State:         domain.HookStateCompleted, // Terminal state
			SchemaVersion: "1.0",
		}

		data, err := json.MarshalIndent(terminalHook, "", "  ")
		require.NoError(t, err)

		hookPath := filepath.Join(hookDir, constants.HookFileName)
		require.NoError(t, os.WriteFile(hookPath, data, 0o600))

		// List stale - should not include terminal hooks
		stale, err := store.ListStale(ctx, 5*time.Minute)
		require.NoError(t, err)

		for _, h := range stale {
			assert.NotEqual(t, taskID, h.TaskID, "terminal hook should not be listed as stale")
		}
	})

	t.Run("excludes recently updated hooks", func(t *testing.T) {
		taskID := "task-fresh-001"

		// Create fresh hook
		_, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		// List with 5 minute threshold - fresh hook should not be included
		stale, err := store.ListStale(ctx, 5*time.Minute)
		require.NoError(t, err)

		for _, h := range stale {
			assert.NotEqual(t, taskID, h.TaskID, "fresh hook should not be listed as stale")
		}
	})
}

func TestFileStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("atomic write survives crash simulation", func(t *testing.T) {
		taskID := "task-atomic-001"

		// Create hook
		hook, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		// Modify hook
		hook.State = domain.HookStateStepRunning
		hook.History = append(hook.History, domain.HookEvent{
			Timestamp: time.Now(),
			FromState: domain.HookStateInitializing,
			ToState:   domain.HookStateStepRunning,
			Trigger:   "test",
		})

		// Save (atomic write)
		err = store.Save(ctx, hook)
		require.NoError(t, err)

		// Verify file is valid JSON
		hookPath := filepath.Join(tmpDir, taskID, constants.HookFileName)
		data, err := os.ReadFile(hookPath) //nolint:gosec // hookPath is constructed from test fixture paths
		require.NoError(t, err)

		var restored domain.Hook
		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)

		assert.Equal(t, domain.HookStateStepRunning, restored.State)
		assert.Len(t, restored.History, 1)
	})
}

func TestFileStore_WithOptions(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("WithLockTimeout", func(t *testing.T) {
		store := NewFileStore(tmpDir, WithLockTimeout(10*time.Second))
		assert.Equal(t, 10*time.Second, store.lockTimeout)
	})

	t.Run("WithMarkdownGenerator", func(t *testing.T) {
		gen := &mockMarkdownGenerator{}
		store := NewFileStore(tmpDir, WithMarkdownGenerator(gen))
		assert.NotNil(t, store.markdownGenerator)
	})
}

type mockMarkdownGenerator struct{}

func (m *mockMarkdownGenerator) Generate(_ *domain.Hook) ([]byte, error) {
	return []byte("# HOOK.md"), nil
}
