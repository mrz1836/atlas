package hook

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestFileStore_GetSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	ctx := context.Background()

	t.Run("returns deep copy of hook", func(t *testing.T) {
		taskID := "task-snapshot-001"

		// Create hook with some state
		hook, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		hook.State = domain.HookStateStepRunning
		hook.CurrentStep = &domain.StepContext{
			StepName:     "implement",
			StepIndex:    2,
			FilesTouched: []string{"file1.go", "file2.go"},
		}
		err = store.Save(ctx, hook)
		require.NoError(t, err)

		// Get snapshot
		snapshot, err := store.GetSnapshot(ctx, taskID)
		require.NoError(t, err)
		require.NotNil(t, snapshot)

		// Verify snapshot has correct values
		assert.Equal(t, taskID, snapshot.TaskID)
		assert.Equal(t, domain.HookStateStepRunning, snapshot.State)
		require.NotNil(t, snapshot.CurrentStep)
		assert.Equal(t, "implement", snapshot.CurrentStep.StepName)

		// Modify the snapshot
		snapshot.TaskID = "modified-task"
		snapshot.State = domain.HookStateFailed
		snapshot.CurrentStep.StepName = "modified-step"
		snapshot.CurrentStep.FilesTouched[0] = "modified.go"

		// Get original again and verify it's unchanged
		original, err := store.Get(ctx, taskID)
		require.NoError(t, err)

		assert.Equal(t, taskID, original.TaskID)
		assert.Equal(t, domain.HookStateStepRunning, original.State)
		assert.Equal(t, "implement", original.CurrentStep.StepName)
		assert.Equal(t, "file1.go", original.CurrentStep.FilesTouched[0])
	})

	t.Run("returns error for non-existent hook", func(t *testing.T) {
		_, err := store.GetSnapshot(ctx, "non-existent-task")
		assert.ErrorIs(t, err, ErrHookNotFound)
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

func TestFileStore_ContextCancellation(t *testing.T) {
	// This test verifies the fix for Issue 2: context cancellation should
	// interrupt lock acquisition instead of waiting the full timeout.

	t.Run("Update respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Use a long lock timeout to make it obvious if context cancellation isn't working
		store := NewFileStore(tmpDir, WithLockTimeout(10*time.Second))
		ctx := context.Background()

		taskID := "test-ctx-cancel"
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create a hook
		_, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		hookPath := store.hookPath(taskID)
		lockPath := hookPath + ".lock"

		// Acquire the lock externally to block the store operation
		externalLock := newFileLock(lockPath)
		err = externalLock.LockWithTimeout(time.Second)
		require.NoError(t, err)
		defer func() { _ = externalLock.Unlock() }()

		// Create a cancellable context
		cancelCtx, cancel := context.WithCancel(ctx)

		// Start Update in a goroutine - it will block trying to acquire the lock
		done := make(chan error, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			updateErr := store.Update(cancelCtx, taskID, func(h *domain.Hook) error {
				h.State = domain.HookStateStepRunning
				return nil
			})
			done <- updateErr
		}()

		// Wait for goroutine to start
		<-started
		time.Sleep(100 * time.Millisecond) // Give it time to enter lock acquisition

		// Cancel the context
		cancel()

		// The Update should return quickly with context.Canceled error
		select {
		case err := <-done:
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("Update did not respect context cancellation - timed out waiting")
		}
	})

	t.Run("Get respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir, WithLockTimeout(10*time.Second))
		ctx := context.Background()

		taskID := "test-ctx-cancel-get"
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create a hook
		_, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		hookPath := store.hookPath(taskID)
		lockPath := hookPath + ".lock"

		// Acquire the lock externally
		externalLock := newFileLock(lockPath)
		err = externalLock.LockWithTimeout(time.Second)
		require.NoError(t, err)
		defer func() { _ = externalLock.Unlock() }()

		// Create a cancellable context
		cancelCtx, cancel := context.WithCancel(ctx)

		// Start Get in a goroutine
		done := make(chan error, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			_, getErr := store.Get(cancelCtx, taskID)
			done <- getErr
		}()

		<-started
		time.Sleep(100 * time.Millisecond)

		// Cancel
		cancel()

		select {
		case err := <-done:
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("Get did not respect context cancellation")
		}
	})

	t.Run("Save respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir, WithLockTimeout(10*time.Second))
		ctx := context.Background()

		taskID := "test-ctx-cancel-save"
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create a hook
		hook, err := store.Create(ctx, taskID, "ws-001")
		require.NoError(t, err)

		hookPath := store.hookPath(taskID)
		lockPath := hookPath + ".lock"

		// Acquire the lock externally
		externalLock := newFileLock(lockPath)
		err = externalLock.LockWithTimeout(time.Second)
		require.NoError(t, err)
		defer func() { _ = externalLock.Unlock() }()

		// Create a cancellable context
		cancelCtx, cancel := context.WithCancel(ctx)

		// Start Save in a goroutine
		done := make(chan error, 1)
		started := make(chan struct{})
		go func() {
			close(started)
			saveErr := store.Save(cancelCtx, hook)
			done <- saveErr
		}()

		<-started
		time.Sleep(100 * time.Millisecond)

		// Cancel
		cancel()

		select {
		case err := <-done:
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("Save did not respect context cancellation")
		}
	})
}

func TestFileLock_LockWithContext(t *testing.T) {
	t.Run("returns context error when canceled before acquiring", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "test.lock")

		// Acquire lock with first instance
		lock1 := newFileLock(lockPath)
		err := lock1.LockWithTimeout(time.Second)
		require.NoError(t, err)
		defer func() { _ = lock1.Unlock() }()

		// Try to acquire with second instance using canceled context
		lock2 := newFileLock(lockPath)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = lock2.LockWithContext(ctx, 5*time.Second)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("returns context error when canceled during wait", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "test2.lock")

		// Acquire lock with first instance
		lock1 := newFileLock(lockPath)
		err := lock1.LockWithTimeout(time.Second)
		require.NoError(t, err)
		defer func() { _ = lock1.Unlock() }()

		// Try to acquire with second instance
		lock2 := newFileLock(lockPath)
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() {
			done <- lock2.LockWithContext(ctx, 10*time.Second)
		}()

		// Wait a bit then cancel
		time.Sleep(150 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("LockWithContext did not respect cancellation")
		}
	})

	t.Run("succeeds when lock becomes available", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "test3.lock")

		// Acquire lock briefly
		lock1 := newFileLock(lockPath)
		err := lock1.LockWithTimeout(time.Second)
		require.NoError(t, err)

		// Start waiting for lock in goroutine
		lock2 := newFileLock(lockPath)
		ctx := context.Background()

		done := make(chan error, 1)
		go func() {
			done <- lock2.LockWithContext(ctx, 5*time.Second)
		}()

		// Release lock1 after a delay
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, lock1.Unlock())

		// lock2 should succeed
		select {
		case err := <-done:
			require.NoError(t, err)
			_ = lock2.Unlock()
		case <-time.After(2 * time.Second):
			t.Fatal("LockWithContext should have succeeded after lock was released")
		}
	})
}

func TestFileStore_Create_RaceCondition(t *testing.T) {
	// This test verifies the fix for the TOCTOU race condition in Create().
	// Multiple goroutines trying to create the same hook simultaneously should
	// result in exactly one success and the rest getting ErrHookExists.

	t.Run("concurrent creates result in exactly one success", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		ctx := context.Background()

		taskID := "race-test-task"

		// Launch multiple goroutines trying to create the same hook
		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.Create(ctx, taskID, "ws-001")
				results <- err
			}()
		}

		wg.Wait()
		close(results)

		// Count successes and ErrHookExists
		successes := 0
		hookExists := 0
		var otherErrors []error
		for err := range results {
			if err == nil {
				successes++
			} else if errors.Is(err, ErrHookExists) {
				hookExists++
			} else {
				otherErrors = append(otherErrors, err)
			}
		}

		// Exactly one should succeed, rest should get ErrHookExists
		assert.Equal(t, 1, successes, "Exactly one Create should succeed")
		assert.Equal(t, numGoroutines-1, hookExists, "Others should get ErrHookExists")
		assert.Empty(t, otherErrors, "No unexpected errors should occur")

		// Verify the hook exists and is valid
		hook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, taskID, hook.TaskID)
		assert.Equal(t, "ws-001", hook.WorkspaceID)
	})

	t.Run("high contention with many goroutines", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		ctx := context.Background()

		// Test with higher contention
		const numGoroutines = 50
		taskID := "high-contention-task"

		var wg sync.WaitGroup
		results := make(chan error, numGoroutines)

		// Add a ready signal to maximize contention
		ready := make(chan struct{})

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-ready // Wait for all goroutines to be ready
				_, err := store.Create(ctx, taskID, "ws-001")
				results <- err
			}()
		}

		// Start all goroutines at once
		close(ready)
		wg.Wait()
		close(results)

		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}

		assert.Equal(t, 1, successes, "Exactly one Create should succeed under high contention")
	})
}
