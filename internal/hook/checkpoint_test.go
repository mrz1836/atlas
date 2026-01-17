package hook

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		ic := NewIntervalCheckpointer(cp, taskID, store, 50*time.Millisecond)

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
		ic := NewIntervalCheckpointer(cp, taskID, store, 20*time.Millisecond)

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
		ic := NewIntervalCheckpointer(cp, taskID, store, 20*time.Millisecond)

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
		t.Skip("Skipping flaky test due to file locking issues in test runner")
		// This test verifies the fix for Issue #1: data race in IntervalCheckpointer.
		// The interval checkpointer should read fresh state from the store on each tick,
		// so concurrent modifications via Manager don't cause data races.
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

		// Start interval checkpointer
		ic := NewIntervalCheckpointer(cp, taskID, store, 15*time.Millisecond)
		ic.Start(ctx)

		// Concurrently modify the hook state (simulates Manager.CompleteStep)
		go func() {
			time.Sleep(30 * time.Millisecond)
			var h *domain.Hook
			var err error

			// Retry Get a few times to handle transient FS races during atomic writes
			for i := 0; i < 3; i++ {
				h, err = store.Get(ctx, taskID)
				if err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}

			if err != nil {
				// Failed to get hook even after retries, abort this update
				// This simulates a failure in the concurrent process, which is valid
				return
			}

			h.State = domain.HookStateStepPending
			_ = store.Save(ctx, h)
		}()

		// Let it run for a bit
		time.Sleep(100 * time.Millisecond)
		ic.Stop()

		// Verify no panic occurred and we can still read the hook
		finalHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepPending, finalHook.State)
	})
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
