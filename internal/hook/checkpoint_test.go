package hook

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestCheckpointer_CreateCheckpoint(t *testing.T) {
	cfg := &config.HookConfig{
		MaxCheckpoints: 50,
	}
	store := NewFileStore(t.TempDir())
	cp := NewCheckpointer(cfg, store)
	ctx := context.Background()

	t.Run("creates checkpoint with correct fields", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 2,
			},
			Checkpoints: []domain.StepCheckpoint{},
		}

		err := cp.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerManual, "Test checkpoint")
		require.NoError(t, err)

		require.Len(t, hook.Checkpoints, 1)
		checkpoint := hook.Checkpoints[0]

		assert.NotEmpty(t, checkpoint.CheckpointID)
		assert.True(t, strings.HasPrefix(checkpoint.CheckpointID, "ckpt-"))
		assert.Equal(t, "Test checkpoint", checkpoint.Description)
		assert.Equal(t, domain.CheckpointTriggerManual, checkpoint.Trigger)
		assert.Equal(t, "implement", checkpoint.StepName)
		assert.Equal(t, 2, checkpoint.StepIndex)
		assert.NotZero(t, checkpoint.CreatedAt)
	})

	t.Run("creates checkpoint for each trigger type", func(t *testing.T) {
		triggers := []domain.CheckpointTrigger{
			domain.CheckpointTriggerManual,
			domain.CheckpointTriggerCommit,
			domain.CheckpointTriggerPush,
			domain.CheckpointTriggerPR,
			domain.CheckpointTriggerValidation,
			domain.CheckpointTriggerStepComplete,
			domain.CheckpointTriggerInterval,
		}

		for _, trigger := range triggers {
			t.Run(string(trigger), func(t *testing.T) {
				hook := &domain.Hook{
					State:       domain.HookStateStepRunning,
					Checkpoints: []domain.StepCheckpoint{},
				}

				err := cp.CreateCheckpoint(ctx, hook, trigger, "Test")
				require.NoError(t, err)
				assert.Equal(t, trigger, hook.Checkpoints[0].Trigger)
			})
		}
	})

	t.Run("captures file snapshots", func(t *testing.T) {
		// Create a test file
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.go")
		require.NoError(t, os.WriteFile(testFile, []byte("package main"), 0o600))

		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:     "implement",
				FilesTouched: []string{testFile},
			},
			Checkpoints: []domain.StepCheckpoint{},
		}

		err := cp.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerManual, "Test")
		require.NoError(t, err)

		require.Len(t, hook.Checkpoints, 1)
		require.Len(t, hook.Checkpoints[0].FilesSnapshot, 1)

		snapshot := hook.Checkpoints[0].FilesSnapshot[0]
		assert.Equal(t, testFile, snapshot.Path)
		assert.True(t, snapshot.Exists)
		assert.Positive(t, snapshot.Size)
		assert.NotEmpty(t, snapshot.ModTime)
		assert.Len(t, snapshot.SHA256, 16) // First 16 chars
	})

	t.Run("updates current checkpoint ID", func(t *testing.T) {
		hook := &domain.Hook{
			State: domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName: "implement",
			},
			Checkpoints: []domain.StepCheckpoint{},
		}

		err := cp.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerManual, "Test")
		require.NoError(t, err)

		assert.Equal(t, hook.Checkpoints[0].CheckpointID, hook.CurrentStep.CurrentCheckpointID)
	})
}

func TestCheckpointer_Pruning(t *testing.T) {
	cfg := &config.HookConfig{
		MaxCheckpoints: 5, // Small limit for testing
	}
	store := NewFileStore(t.TempDir())
	cp := NewCheckpointer(cfg, store)
	ctx := context.Background()

	t.Run("prunes oldest checkpoints when limit exceeded", func(t *testing.T) {
		hook := &domain.Hook{
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}

		// Create 7 checkpoints (2 over limit)
		for i := 0; i < 7; i++ {
			err := cp.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerInterval, "Checkpoint")
			require.NoError(t, err)
		}

		// Should have only 5 checkpoints (oldest 2 pruned)
		assert.Len(t, hook.Checkpoints, 5)
	})

	t.Run("keeps most recent checkpoints", func(t *testing.T) {
		hook := &domain.Hook{
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}

		// Create checkpoints with unique descriptions
		for i := 0; i < 7; i++ {
			err := cp.CreateCheckpoint(ctx, hook, domain.CheckpointTriggerInterval, "Checkpoint")
			require.NoError(t, err)
		}

		// Verify we have the most recent 5 (IDs should be unique)
		ids := make(map[string]bool)
		for _, cp := range hook.Checkpoints {
			ids[cp.CheckpointID] = true
		}
		assert.Len(t, ids, 5)
	})
}

func TestCheckpointer_GetLatestCheckpoint(t *testing.T) {
	cfg := &config.HookConfig{}
	store := NewFileStore(t.TempDir())
	cp := NewCheckpointer(cfg, store)

	t.Run("returns nil for empty checkpoints", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: []domain.StepCheckpoint{},
		}

		latest := cp.GetLatestCheckpoint(hook)
		assert.Nil(t, latest)
	})

	t.Run("returns most recent checkpoint", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: []domain.StepCheckpoint{
				{CheckpointID: "ckpt-001", Description: "First"},
				{CheckpointID: "ckpt-002", Description: "Second"},
				{CheckpointID: "ckpt-003", Description: "Third"},
			},
		}

		latest := cp.GetLatestCheckpoint(hook)
		require.NotNil(t, latest)
		assert.Equal(t, "ckpt-003", latest.CheckpointID)
		assert.Equal(t, "Third", latest.Description)
	})
}

func TestCheckpointer_GetCheckpointByID(t *testing.T) {
	cfg := &config.HookConfig{}
	store := NewFileStore(t.TempDir())
	cp := NewCheckpointer(cfg, store)

	hook := &domain.Hook{
		Checkpoints: []domain.StepCheckpoint{
			{CheckpointID: "ckpt-001", Description: "First"},
			{CheckpointID: "ckpt-002", Description: "Second"},
			{CheckpointID: "ckpt-003", Description: "Third"},
		},
	}

	t.Run("finds existing checkpoint", func(t *testing.T) {
		found := cp.GetCheckpointByID(hook, "ckpt-002")
		require.NotNil(t, found)
		assert.Equal(t, "Second", found.Description)
	})

	t.Run("returns nil for non-existent ID", func(t *testing.T) {
		found := cp.GetCheckpointByID(hook, "ckpt-999")
		assert.Nil(t, found)
	})
}

func TestIntervalCheckpointer(t *testing.T) {
	cfg := &config.HookConfig{
		MaxCheckpoints: 50,
	}
	tmpDir := t.TempDir()
	store := NewFileStore(tmpDir)
	cp := NewCheckpointer(cfg, store)
	ctx := context.Background()

	t.Run("concurrent Stop calls do not deadlock", func(t *testing.T) {
		// This test verifies the fix for the deadlock pattern where Stop()
		// would hold the mutex while waiting on the done channel.
		taskID := "test-concurrent-stop"

		// Create and save hook to disk
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		// Create interval checkpointer
		ic := NewIntervalCheckpointer(cp, taskID, store, 50*time.Millisecond, zerolog.Nop())

		ic.Start(ctx)
		time.Sleep(10 * time.Millisecond) // Let it start

		// Call Stop() from multiple goroutines concurrently
		// This would deadlock with the old implementation if the goroutine
		// tried to acquire the mutex while Stop() was holding it and waiting.
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ic.Stop()
			}()
		}

		// Use a timeout to detect deadlock
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("Deadlock detected: concurrent Stop() calls did not complete")
		}
	})

	t.Run("Stop is idempotent", func(t *testing.T) {
		// Verify that calling Stop() multiple times is safe
		taskID := "test-idempotent-stop"

		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		ic := NewIntervalCheckpointer(cp, taskID, store, 50*time.Millisecond, zerolog.Nop())

		ic.Start(ctx)
		time.Sleep(10 * time.Millisecond)

		// Call Stop multiple times - should not panic or hang
		ic.Stop()
		ic.Stop()
		ic.Stop()
	})

	t.Run("Stop on never-started checkpointer", func(_ *testing.T) {
		// Calling Stop() without Start() should be a no-op
		taskID := "test-stop-never-started"

		ic := NewIntervalCheckpointer(cp, taskID, store, 50*time.Millisecond, zerolog.Nop())

		// Should not panic or hang
		ic.Stop()
		ic.Stop()
	})

	t.Run("starts and stops cleanly", func(t *testing.T) {
		taskID := "test-task-start-stop"

		// Create and save hook to disk first (required by new design)
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		// Now create interval checkpointer with taskID (not hook pointer)
		ic := NewIntervalCheckpointer(cp, taskID, store, 50*time.Millisecond, zerolog.Nop())

		ic.Start(ctx)
		time.Sleep(10 * time.Millisecond) // Let it start
		ic.Stop()

		// Should complete without hanging
	})

	t.Run("creates checkpoints at interval", func(t *testing.T) {
		taskID := "interval-test"

		// Create and save hook to disk
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		// Create interval checkpointer with taskID
		ic := NewIntervalCheckpointer(cp, taskID, store, 20*time.Millisecond, zerolog.Nop())

		ic.Start(ctx)
		time.Sleep(100 * time.Millisecond) // Allow multiple intervals
		ic.Stop()

		// Reload hook from store to see checkpoints
		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)

		// Should have created some checkpoints.
		// The timing is approximate, so we verify at least one checkpoint was created.
		assert.GreaterOrEqual(t, len(updatedHook.Checkpoints), 1)
	})

	t.Run("only creates checkpoints when step_running", func(t *testing.T) {
		taskID := "state-test"

		// Create and save hook in non-running state
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepPending, // Not running
			Checkpoints: []domain.StepCheckpoint{},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		// Create interval checkpointer with taskID
		ic := NewIntervalCheckpointer(cp, taskID, store, 20*time.Millisecond, zerolog.Nop())

		ic.Start(ctx)
		time.Sleep(100 * time.Millisecond)
		ic.Stop()

		// Reload hook from store
		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)

		// Should not have created any checkpoints
		assert.Empty(t, updatedHook.Checkpoints)
	})

	t.Run("handles concurrent state changes safely", func(t *testing.T) {
		// This test verifies thread-safe concurrent access to hook state.
		// Both operations use Update() for atomic read-modify-write, ensuring
		// that modifications are not lost even when running concurrently.
		taskID := "race-test"

		// Create and save initial hook
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 1,
			},
		}
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		require.NoError(t, store.Save(ctx, hook))

		// Start interval checkpointer with longer interval to reduce lock contention
		// during test (git commands can be slow under race detector)
		ic := NewIntervalCheckpointer(cp, taskID, store, 200*time.Millisecond, zerolog.Nop())
		ic.Start(ctx)

		// Give the interval checkpointer time to create at least one checkpoint
		time.Sleep(250 * time.Millisecond)

		// Now stop the interval checkpointer before making concurrent modifications
		// This ensures we can reliably test the state change without lock contention
		ic.Stop()

		// Verify at least one checkpoint was created
		midHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		initialCheckpoints := len(midHook.Checkpoints)
		require.GreaterOrEqual(t, initialCheckpoints, 1, "Should have at least one checkpoint")

		// Now test concurrent modifications using Update() (both should succeed)
		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Change state to step_pending
		go func() {
			defer wg.Done()
			updateErr := store.Update(ctx, taskID, func(h *domain.Hook) error {
				h.State = domain.HookStateStepPending
				return nil
			})
			assert.NoError(t, updateErr)
		}()

		// Goroutine 2: Add a history event
		go func() {
			defer wg.Done()
			updateErr := store.Update(ctx, taskID, func(h *domain.Hook) error {
				h.History = append(h.History, domain.HookEvent{
					Timestamp: time.Now().UTC(),
					FromState: domain.HookStateStepRunning,
					ToState:   domain.HookStateStepPending,
					Trigger:   "test_concurrent",
					Details:   map[string]any{"note": "Concurrent modification test"},
				})
				return nil
			})
			assert.NoError(t, updateErr)
		}()

		wg.Wait()

		// Verify both modifications were applied (no data loss)
		finalHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepPending, finalHook.State,
			"State change should be preserved")
		assert.GreaterOrEqual(t, len(finalHook.History), 1,
			"History event should be preserved")
		assert.GreaterOrEqual(t, len(finalHook.Checkpoints), initialCheckpoints,
			"Checkpoints should be preserved during concurrent modification")
	})
}

func TestGitCommandTimeout(t *testing.T) {
	// Verify that gitCommandTimeout is set to a reasonable value (5 seconds)
	// This ensures git commands won't block the checkpointer indefinitely
	assert.Equal(t, 5*time.Second, gitCommandTimeout,
		"gitCommandTimeout should be 5 seconds to prevent infinite hangs")
}

func TestCaptureGitState_ReturnsWithinTimeout(t *testing.T) {
	// This test verifies that captureGitState returns within a reasonable time
	// even when called in a non-git directory (which should fail fast)
	cfg := &config.HookConfig{}
	store := NewFileStore(t.TempDir())
	cp := NewCheckpointer(cfg, store)

	// Use a context with a timeout longer than gitCommandTimeout
	// to ensure we're not relying on the parent context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	branch, commit, dirty := cp.captureGitState(ctx)
	elapsed := time.Since(start)

	// The function should return within gitCommandTimeout + small buffer
	// In a non-git directory, git commands fail immediately
	assert.Less(t, elapsed, gitCommandTimeout+time.Second,
		"captureGitState should return within timeout")

	// In a non-git directory, we expect empty/default values
	// (or actual values if we're in the atlas repo)
	_ = branch
	_ = commit
	_ = dirty
}

func TestCaptureFileSnapshot(t *testing.T) {
	t.Run("captures existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		content := []byte("Hello, World!")
		require.NoError(t, os.WriteFile(testFile, content, 0o600))

		snapshot := CaptureFileSnapshot(testFile)

		assert.Equal(t, testFile, snapshot.Path)
		assert.True(t, snapshot.Exists)
		assert.Equal(t, int64(len(content)), snapshot.Size)
		assert.NotEmpty(t, snapshot.ModTime)
		assert.Len(t, snapshot.SHA256, 16)
	})

	t.Run("handles non-existent file", func(t *testing.T) {
		snapshot := CaptureFileSnapshot("/non/existent/file.txt")

		assert.Equal(t, "/non/existent/file.txt", snapshot.Path)
		assert.False(t, snapshot.Exists)
		assert.Equal(t, int64(0), snapshot.Size)
	})
}

func TestCaptureFileSnapshot_LargeFile(t *testing.T) {
	// This test verifies the fix for unbounded file reads.
	// Large files should be captured but without SHA256 hash to prevent OOM.

	t.Run("skips hash for files over size limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		largeFile := filepath.Join(tmpDir, "large.bin")

		// Create a file larger than maxFileSizeForHash (10MB)
		// We use a sparse file approach: create file and seek to make it large
		f, err := os.Create(largeFile) // #nosec G304 -- Test file in temp directory
		require.NoError(t, err)

		// Write a small header so the file has some content
		_, err = f.WriteString("header data")
		require.NoError(t, err)

		// Seek to 11MB and write a byte to create a "large" file
		// This creates a sparse file that doesn't actually use 11MB of disk
		_, err = f.Seek(11*1024*1024, 0)
		require.NoError(t, err)
		_, err = f.WriteString("x")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		snapshot := CaptureFileSnapshot(largeFile)

		assert.True(t, snapshot.Exists)
		assert.Greater(t, snapshot.Size, int64(10*1024*1024), "File should be larger than 10MB")
		assert.Empty(t, snapshot.SHA256, "Large files should not have SHA256 computed")
		assert.NotEmpty(t, snapshot.ModTime, "ModTime should still be captured")
	})

	t.Run("computes hash for files at size limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "limit.bin")

		// Create a file exactly at the limit (10MB)
		f, err := os.Create(testFile) // #nosec G304 -- Test file in temp directory
		require.NoError(t, err)

		// Write exactly 10MB of data
		data := make([]byte, 1024) // 1KB block
		for i := 0; i < 10*1024; i++ {
			_, err = f.Write(data)
			require.NoError(t, err)
		}
		require.NoError(t, f.Close())

		snapshot := CaptureFileSnapshot(testFile)

		assert.True(t, snapshot.Exists)
		assert.Equal(t, int64(10*1024*1024), snapshot.Size)
		assert.Len(t, snapshot.SHA256, 16, "Files at the limit should have SHA256 computed")
	})
}

func TestMaxFileSizeForHash(t *testing.T) {
	// Verify the constant is set to 10MB as specified
	assert.Equal(t, int64(10*1024*1024), int64(maxFileSizeForHash),
		"maxFileSizeForHash should be 10MB")
}

func TestHashFileStreaming(t *testing.T) {
	t.Run("computes correct hash", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		content := []byte("Hello, World!")
		require.NoError(t, os.WriteFile(testFile, content, 0o600))

		hash, err := hashFileStreaming(testFile)
		require.NoError(t, err)

		// Verify hash length (SHA256 = 64 hex chars)
		assert.Len(t, hash, 64)

		// Verify hash is consistent
		hash2, err := hashFileStreaming(testFile)
		require.NoError(t, err)
		assert.Equal(t, hash, hash2)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := hashFileStreaming("/non/existent/file.txt")
		assert.Error(t, err)
	})

	t.Run("handles empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty.txt")
		require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0o600))

		hash, err := hashFileStreaming(emptyFile)
		require.NoError(t, err)
		assert.Len(t, hash, 64) // SHA256 of empty = e3b0c44...
	})
}

func TestGetCheckpointsForStep(t *testing.T) {
	hook := &domain.Hook{
		Checkpoints: []domain.StepCheckpoint{
			{CheckpointID: "ckpt-001", StepName: "analyze"},
			{CheckpointID: "ckpt-002", StepName: "implement"},
			{CheckpointID: "ckpt-003", StepName: "implement"},
			{CheckpointID: "ckpt-004", StepName: "validate"},
		},
	}

	t.Run("finds checkpoints for step", func(t *testing.T) {
		cps := GetCheckpointsForStep(hook, "implement")
		assert.Len(t, cps, 2)
		assert.Equal(t, "ckpt-002", cps[0].CheckpointID)
		assert.Equal(t, "ckpt-003", cps[1].CheckpointID)
	})

	t.Run("returns empty for unknown step", func(t *testing.T) {
		cps := GetCheckpointsForStep(hook, "unknown")
		assert.Empty(t, cps)
	})
}

func TestGetCheckpointsSince(t *testing.T) {
	now := time.Now()
	hook := &domain.Hook{
		Checkpoints: []domain.StepCheckpoint{
			{CheckpointID: "ckpt-001", CreatedAt: now.Add(-30 * time.Minute)},
			{CheckpointID: "ckpt-002", CreatedAt: now.Add(-10 * time.Minute)},
			{CheckpointID: "ckpt-003", CreatedAt: now.Add(-5 * time.Minute)},
		},
	}

	t.Run("finds checkpoints after time", func(t *testing.T) {
		cps := GetCheckpointsSince(hook, now.Add(-15*time.Minute))
		assert.Len(t, cps, 2)
		assert.Equal(t, "ckpt-002", cps[0].CheckpointID)
		assert.Equal(t, "ckpt-003", cps[1].CheckpointID)
	})
}

func TestCountCheckpointsByTrigger(t *testing.T) {
	hook := &domain.Hook{
		Checkpoints: []domain.StepCheckpoint{
			{Trigger: domain.CheckpointTriggerCommit},
			{Trigger: domain.CheckpointTriggerCommit},
			{Trigger: domain.CheckpointTriggerManual},
			{Trigger: domain.CheckpointTriggerInterval},
			{Trigger: domain.CheckpointTriggerInterval},
			{Trigger: domain.CheckpointTriggerInterval},
		},
	}

	counts := CountCheckpointsByTrigger(hook)

	assert.Equal(t, 2, counts[domain.CheckpointTriggerCommit])
	assert.Equal(t, 1, counts[domain.CheckpointTriggerManual])
	assert.Equal(t, 3, counts[domain.CheckpointTriggerInterval])
}

func TestFindFilesInCheckpoints(t *testing.T) {
	hook := &domain.Hook{
		Checkpoints: []domain.StepCheckpoint{
			{
				FilesSnapshot: []domain.FileSnapshot{
					{Path: "file1.go"},
					{Path: "file2.go"},
				},
			},
			{
				FilesSnapshot: []domain.FileSnapshot{
					{Path: "file2.go"}, // Duplicate
					{Path: "file3.go"},
				},
			},
		},
	}

	files := FindFilesInCheckpoints(hook)

	assert.Len(t, files, 3) // Unique files only
	assert.Contains(t, files, "file1.go")
	assert.Contains(t, files, "file2.go")
	assert.Contains(t, files, "file3.go")
}

func TestPruneCheckpoints(t *testing.T) {
	t.Run("prunes when over custom limit", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: make([]domain.StepCheckpoint, 60),
		}
		for i := 0; i < 60; i++ {
			hook.Checkpoints[i] = domain.StepCheckpoint{
				CheckpointID: GenerateCheckpointID(),
				Description:  "checkpoint " + string(rune('A'+i%26)),
			}
		}

		// Prune with custom limit of 30
		PruneCheckpoints(hook, 30)

		assert.Len(t, hook.Checkpoints, 30)
	})

	t.Run("uses default when maxCheckpoints is 0", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: make([]domain.StepCheckpoint, 60),
		}
		for i := 0; i < 60; i++ {
			hook.Checkpoints[i] = domain.StepCheckpoint{
				CheckpointID: GenerateCheckpointID(),
			}
		}

		// Prune with 0 should use DefaultMaxCheckpoints
		PruneCheckpoints(hook, 0)

		assert.Len(t, hook.Checkpoints, DefaultMaxCheckpoints)
	})

	t.Run("uses default when maxCheckpoints is negative", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: make([]domain.StepCheckpoint, 60),
		}
		for i := 0; i < 60; i++ {
			hook.Checkpoints[i] = domain.StepCheckpoint{
				CheckpointID: GenerateCheckpointID(),
			}
		}

		// Prune with negative should use DefaultMaxCheckpoints
		PruneCheckpoints(hook, -5)

		assert.Len(t, hook.Checkpoints, DefaultMaxCheckpoints)
	})

	t.Run("keeps most recent checkpoints", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: make([]domain.StepCheckpoint, 60),
		}
		// Create checkpoints with identifiable IDs
		for i := 0; i < 60; i++ {
			hook.Checkpoints[i] = domain.StepCheckpoint{
				CheckpointID: "ckpt-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
				Description:  "checkpoint-" + string(rune('A'+i%26)),
			}
		}

		// Remember the last 30 checkpoints
		expected := make([]string, 30)
		for i := 0; i < 30; i++ {
			expected[i] = hook.Checkpoints[30+i].CheckpointID
		}

		PruneCheckpoints(hook, 30)

		assert.Len(t, hook.Checkpoints, 30)
		// Verify we kept the most recent ones
		for i, cp := range hook.Checkpoints {
			assert.Equal(t, expected[i], cp.CheckpointID)
		}
	})

	t.Run("does nothing when under limit", func(t *testing.T) {
		hook := &domain.Hook{
			Checkpoints: make([]domain.StepCheckpoint, 10),
		}
		for i := 0; i < 10; i++ {
			hook.Checkpoints[i] = domain.StepCheckpoint{
				CheckpointID: GenerateCheckpointID(),
			}
		}

		PruneCheckpoints(hook, 50)

		assert.Len(t, hook.Checkpoints, 10)
	})
}

func TestDefaultMaxCheckpoints(t *testing.T) {
	t.Run("constant has expected value", func(t *testing.T) {
		assert.Equal(t, 50, DefaultMaxCheckpoints)
	})
}

func TestGenerateCheckpointID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateCheckpointID()
			assert.False(t, ids[id], "duplicate ID generated: %s", id)
			ids[id] = true
		}
	})

	t.Run("has correct format", func(t *testing.T) {
		id := GenerateCheckpointID()
		assert.True(t, strings.HasPrefix(id, "ckpt-"))
		assert.Len(t, id, 13) // "ckpt-" + 8 chars
	})
}
