package hook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
)

var (
	errKeyNotAvailable   = errors.New("key not available")
	errCompilationFailed = errors.New("compilation failed")
	errFatalError        = errors.New("fatal error")
)

func TestNewManager(t *testing.T) {
	t.Run("creates manager with defaults", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		cfg := &config.HookConfig{}

		m := NewManager(store, cfg)

		require.NotNil(t, m)
		assert.Nil(t, m.signer) // No signer by default
		assert.NotNil(t, m.intervalCheckers)
	})

	t.Run("creates manager with signer option", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		cfg := &config.HookConfig{}
		signer := NewMockSigner()

		m := NewManager(store, cfg, WithReceiptSigner(signer))

		require.NotNil(t, m)
		assert.Equal(t, signer, m.signer)
	})
}

func TestManager_CreateValidationReceipt(t *testing.T) {
	ctx := context.Background()

	t.Run("creates receipt without signer", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg) // No signer

		// Create task and hook
		taskID := "test-task-no-sign"
		task := &domain.Task{ID: taskID}

		// Create hook directory and hook
		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		_, err := store.Create(ctx, taskID, "workspace-1")
		require.NoError(t, err)

		// Create validation result
		result := &domain.StepResult{
			Output:      "All tests passed",
			StartedAt:   time.Now().Add(-10 * time.Second),
			CompletedAt: time.Now(),
			DurationMs:  10000,
			Metadata: map[string]any{
				"command":   "go test ./...",
				"exit_code": 0,
			},
		}

		// Create receipt
		err = m.CreateValidationReceipt(ctx, task, "validate", result)
		require.NoError(t, err)

		// Verify receipt was created but without signature
		hook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		require.Len(t, hook.Receipts, 1)

		receipt := hook.Receipts[0]
		assert.Contains(t, receipt.ReceiptID, "rcpt-")
		assert.Equal(t, "validate", receipt.StepName)
		assert.Equal(t, "go test ./...", receipt.Command)
		assert.Equal(t, 0, receipt.ExitCode)
		assert.Empty(t, receipt.Signature) // No signer = no signature
	})

	t.Run("creates and signs receipt with signer", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		signer := NewMockSigner()
		m := NewManager(store, cfg, WithReceiptSigner(signer))

		// Create task and hook
		taskID := "test-task-signed"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		_, err := store.Create(ctx, taskID, "workspace-1")
		require.NoError(t, err)

		// Create validation result
		result := &domain.StepResult{
			Output:      "Lint passed",
			StartedAt:   time.Now().Add(-5 * time.Second),
			CompletedAt: time.Now(),
			DurationMs:  5000,
			Metadata: map[string]any{
				"command":   "magex lint",
				"exit_code": 0,
			},
		}

		// Create receipt
		err = m.CreateValidationReceipt(ctx, task, "lint", result)
		require.NoError(t, err)

		// Verify receipt was created WITH signature
		hook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		require.Len(t, hook.Receipts, 1)

		receipt := hook.Receipts[0]
		assert.NotEmpty(t, receipt.Signature) // Signer populated signature
		assert.Equal(t, "lint", receipt.StepName)

		// Verify signer was called
		require.Len(t, signer.SignedReceipts, 1)
		assert.Equal(t, receipt.ReceiptID, signer.SignedReceipts[0].ReceiptID)
	})

	t.Run("continues without signature on signer error", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}

		// Create signer that fails
		signer := NewMockSigner()
		signer.SignFunc = func(_ context.Context, _ []byte) ([]byte, error) {
			return nil, errKeyNotAvailable
		}
		m := NewManager(store, cfg, WithReceiptSigner(signer))

		// Create task and hook
		taskID := "test-task-sign-fail"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		_, err := store.Create(ctx, taskID, "workspace-1")
		require.NoError(t, err)

		// Create validation result
		result := &domain.StepResult{
			Output:      "Tests passed",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			DurationMs:  1000,
			Metadata: map[string]any{
				"command":   "go test",
				"exit_code": 0,
			},
		}

		// Create receipt - should succeed despite signing failure
		err = m.CreateValidationReceipt(ctx, task, "test", result)
		require.NoError(t, err)

		// Verify receipt was created but without signature
		hook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		require.Len(t, hook.Receipts, 1)
		assert.Empty(t, hook.Receipts[0].Signature) // Signing failed, but receipt saved
	})

	t.Run("increments task index for each receipt", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}

		// Track task indices used for signing
		var taskIndices []uint32
		signer := NewMockSigner()
		originalSignReceipt := signer.SignReceipt
		signer.SignFunc = func(_ context.Context, _ []byte) ([]byte, error) {
			return []byte("sig"), nil
		}
		// Override SignReceipt to capture taskIndex
		m := NewManager(store, cfg, WithReceiptSigner(&taskIndexTrackingSigner{
			MockSigner:  signer,
			taskIndices: &taskIndices,
		}))

		// Create task and hook
		taskID := "test-task-multi-receipt"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))
		_, err := store.Create(ctx, taskID, "workspace-1")
		require.NoError(t, err)

		// Create multiple receipts
		for i := 0; i < 3; i++ {
			result := &domain.StepResult{
				Output:      "passed",
				StartedAt:   time.Now(),
				CompletedAt: time.Now(),
				DurationMs:  1000,
			}
			err := m.CreateValidationReceipt(ctx, task, "step", result)
			require.NoError(t, err)
		}

		// Verify task indices were 0, 1, 2
		assert.Equal(t, []uint32{0, 1, 2}, taskIndices)

		// Discard the original function reference to avoid unused warning
		_ = originalSignReceipt
	})
}

// taskIndexTrackingSigner wraps MockSigner to track taskIndex values
type taskIndexTrackingSigner struct {
	*MockSigner

	taskIndices *[]uint32
}

func (s *taskIndexTrackingSigner) SignReceipt(ctx context.Context, receipt *domain.ValidationReceipt, taskIndex uint32) error {
	*s.taskIndices = append(*s.taskIndices, taskIndex)
	return s.MockSigner.SignReceipt(ctx, receipt, taskIndex)
}

func TestManager_StartIntervalCheckpointing(t *testing.T) {
	ctx := context.Background()

	t.Run("starts interval checkpointer for task", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{
			CheckpointInterval: 50 * time.Millisecond,
		}
		m := NewManager(store, cfg)

		// Create task and hook
		taskID := "test-interval-start"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Start interval checkpointing
		err := m.StartIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		// Verify checkpointer was started
		m.checkpointsMu.Lock()
		_, exists := m.intervalCheckers[taskID]
		m.checkpointsMu.Unlock()
		assert.True(t, exists)

		// Clean up
		_ = m.StopIntervalCheckpointing(ctx, task)
	})

	t.Run("stops existing checkpointer before starting new one", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{
			CheckpointInterval: 100 * time.Millisecond,
		}
		m := NewManager(store, cfg)

		taskID := "test-interval-restart"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Start twice - should not leak goroutines
		err := m.StartIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		err = m.StartIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		// Clean up
		_ = m.StopIntervalCheckpointing(ctx, task)
	})

	t.Run("returns error if hook does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		task := &domain.Task{ID: "non-existent-task"}

		err := m.StartIntervalCheckpointing(ctx, task)
		assert.Error(t, err)
	})
}

func TestManager_StopIntervalCheckpointing(t *testing.T) {
	ctx := context.Background()

	t.Run("stops running checkpointer", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{
			CheckpointInterval: 50 * time.Millisecond,
		}
		m := NewManager(store, cfg)

		taskID := "test-interval-stop"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Start and then stop
		err := m.StartIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		err = m.StopIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		// Verify checkpointer was removed
		m.checkpointsMu.Lock()
		_, exists := m.intervalCheckers[taskID]
		m.checkpointsMu.Unlock()
		assert.False(t, exists)
	})

	t.Run("no-op if checkpointer not running", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		task := &domain.Task{ID: "not-running"}

		// Should not error
		err := m.StopIntervalCheckpointing(ctx, task)
		assert.NoError(t, err)
	})
}

// concurrentReceiptCreator creates validation receipts concurrently
func concurrentReceiptCreator(ctx context.Context, m *Manager, task *domain.Task, errChan chan<- error) {
	for i := 0; i < 3; i++ {
		result := &domain.StepResult{
			Output:      "passed",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			DurationMs:  100,
			Metadata: map[string]any{
				"command":   "test",
				"exit_code": 0,
			},
		}
		if receiptErr := m.CreateValidationReceipt(ctx, task, "validate", result); receiptErr != nil {
			errChan <- receiptErr
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// concurrentStepTransitioner transitions steps concurrently
// After state validation, transitions from step_running to step_running are invalid.
// This is expected - we just check that concurrent attempts don't cause races.
func concurrentStepTransitioner(ctx context.Context, m *Manager, task *domain.Task, _ chan<- error) {
	for i := 0; i < 2; i++ {
		// Errors are expected after state validation (can't transition step_running -> step_running)
		// We only report unexpected panics or data races (which the race detector catches)
		_ = m.TransitionStep(ctx, task, "step", i)
		time.Sleep(20 * time.Millisecond)
	}
}

// concurrentStepCompleter completes steps concurrently
func concurrentStepCompleter(ctx context.Context, m *Manager, task *domain.Task, errChan chan<- error) {
	time.Sleep(30 * time.Millisecond) // Wait for some transitions
	if completeErr := m.CompleteStep(ctx, task, "step", []string{"file.go"}); completeErr != nil {
		errChan <- completeErr
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	// This test verifies no data races with concurrent access.
	// Run with: go test -race ./internal/hook/...
	ctx := context.Background()

	t.Run("concurrent manager operations are safe", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Use longer lock timeout for concurrent test to avoid timeouts under heavy contention
		store := NewFileStore(tmpDir, WithLockTimeout(30*time.Second))
		cfg := &config.HookConfig{
			CheckpointInterval: 100 * time.Millisecond, // Reduce checkpoint frequency to lower contention
			MaxCheckpoints:     50,
		}
		signer := NewMockSigner()
		m := NewManager(store, cfg, WithReceiptSigner(signer))

		taskID := "concurrent-test"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create initial hook
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepRunning,
			Checkpoints: []domain.StepCheckpoint{},
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 1,
			},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Start interval checkpointing
		err := m.StartIntervalCheckpointing(ctx, task)
		require.NoError(t, err)

		// Run concurrent operations
		// After state validation, some operations may return errors
		// (e.g., transitioning from step_running to step_running is invalid).
		// The key goal here is to verify no data races (detected by -race flag).
		var wg sync.WaitGroup
		errChan := make(chan error, 10)

		wg.Add(3)
		go func() { defer wg.Done(); concurrentReceiptCreator(ctx, m, task, errChan) }()
		go func() { defer wg.Done(); concurrentStepTransitioner(ctx, m, task, errChan) }()
		go func() { defer wg.Done(); concurrentStepCompleter(ctx, m, task, errChan) }()

		// Wait for all goroutines
		wg.Wait()
		close(errChan)

		// Stop interval checkpointing
		_ = m.StopIntervalCheckpointing(ctx, task)

		// Verify final state is consistent (no panics, no corruption)
		finalHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.NotNil(t, finalHook)

		// Should have receipts
		assert.GreaterOrEqual(t, len(finalHook.Receipts), 1)
	})
}

func TestManager_TransitionStep(t *testing.T) {
	ctx := context.Background()

	t.Run("transitions hook to step_running", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-transition"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in step_pending state (valid source for step_running)
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepPending,
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		err := m.TransitionStep(ctx, task, "implement", 2)
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepRunning, updatedHook.State)
		assert.Equal(t, "implement", updatedHook.CurrentStep.StepName)
		assert.Equal(t, 2, updatedHook.CurrentStep.StepIndex)
	})
}

func TestManager_CompleteStep(t *testing.T) {
	ctx := context.Background()

	t.Run("transitions hook to step_pending and creates checkpoint", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-complete"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "analyze",
				StepIndex: 0,
			},
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		err := m.CompleteStep(ctx, task, "analyze", []string{"file1.go", "file2.go"})
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepPending, updatedHook.State)
		assert.Len(t, updatedHook.Checkpoints, 1)
		assert.Equal(t, domain.CheckpointTriggerStepComplete, updatedHook.Checkpoints[0].Trigger)
	})

	t.Run("persists FilesSnapshot in checkpoint", func(t *testing.T) {
		// This test verifies the fix for Issue 1: FilesSnapshot was being set
		// AFTER append, which means the modification was lost (struct copied on append).
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-complete-snapshot"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create actual test files so snapshotFiles can stat them
		testFile1 := filepath.Join(tmpDir, "changed1.go")
		testFile2 := filepath.Join(tmpDir, "changed2.go")
		require.NoError(t, os.WriteFile(testFile1, []byte("package test1"), 0o600))
		require.NoError(t, os.WriteFile(testFile2, []byte("package test2"), 0o600))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 1,
			},
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Complete step with files changed
		err := m.CompleteStep(ctx, task, "implement", []string{testFile1, testFile2})
		require.NoError(t, err)

		// Verify FilesSnapshot is populated in the persisted checkpoint
		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		require.Len(t, updatedHook.Checkpoints, 1)

		checkpoint := updatedHook.Checkpoints[0]
		assert.NotEmpty(t, checkpoint.FilesSnapshot, "FilesSnapshot should NOT be empty after CompleteStep")
		assert.Len(t, checkpoint.FilesSnapshot, 2)

		// Verify snapshot details
		assert.Equal(t, testFile1, checkpoint.FilesSnapshot[0].Path)
		assert.True(t, checkpoint.FilesSnapshot[0].Exists)
		assert.Equal(t, testFile2, checkpoint.FilesSnapshot[1].Path)
		assert.True(t, checkpoint.FilesSnapshot[1].Exists)
	})

	t.Run("falls back to FilesTouched when filesChanged is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-complete-fallback"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create test file
		testFile := filepath.Join(tmpDir, "touched.go")
		require.NoError(t, os.WriteFile(testFile, []byte("package touched"), 0o600))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:     "implement",
				StepIndex:    1,
				FilesTouched: []string{testFile}, // Pre-existing files touched
			},
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Complete step with empty filesChanged - should fall back to FilesTouched
		err := m.CompleteStep(ctx, task, "implement", nil)
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		require.Len(t, updatedHook.Checkpoints, 1)

		checkpoint := updatedHook.Checkpoints[0]
		assert.NotEmpty(t, checkpoint.FilesSnapshot, "FilesSnapshot should use fallback FilesTouched")
		assert.Len(t, checkpoint.FilesSnapshot, 1)
		assert.Equal(t, testFile, checkpoint.FilesSnapshot[0].Path)
	})
}

func TestManager_CompleteStep_NilCurrentStep(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when CurrentStep is nil", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-nil-current-step"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook WITHOUT CurrentStep set
		hook := &domain.Hook{
			TaskID:      taskID,
			State:       domain.HookStateStepPending, // No current step context
			CurrentStep: nil,                         // Explicitly nil
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to complete step - should fail
		err := m.CompleteStep(ctx, task, "analyze", []string{"file.go"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no current step context")
		assert.Contains(t, err.Error(), "analyze")
	})
}

func TestManager_CompleteStep_StepNameMismatch(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when stepName does not match CurrentStep", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-step-mismatch"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook with CurrentStep set to "analyze"
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "analyze",
				StepIndex: 0,
				StartedAt: time.Now(),
			},
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to complete with wrong step name - should fail
		err := m.CompleteStep(ctx, task, "implement", []string{"file.go"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot complete step")
		assert.Contains(t, err.Error(), "implement")
		assert.Contains(t, err.Error(), "current step is")
		assert.Contains(t, err.Error(), "analyze")
	})

	t.Run("succeeds when stepName matches CurrentStep", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-step-match"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook with CurrentStep set to "analyze"
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
			CurrentStep: &domain.StepContext{
				StepName:  "analyze",
				StepIndex: 0,
				StartedAt: time.Now(),
			},
			Checkpoints: []domain.StepCheckpoint{},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Complete with matching step name - should succeed
		err := m.CompleteStep(ctx, task, "analyze", []string{"file.go"})
		require.NoError(t, err)

		// Verify state transition
		updated, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepPending, updated.State)
	})
}

func TestManager_FailStep(t *testing.T) {
	ctx := context.Background()

	t.Run("transitions hook to awaiting_human", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-fail"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
		}
		require.NoError(t, store.Save(ctx, hook))

		err := m.FailStep(ctx, task, "implement", errCompilationFailed)
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateAwaitingHuman, updatedHook.State)
		assert.Len(t, updatedHook.History, 1)
		assert.Equal(t, "compilation failed", updatedHook.History[0].Details["error"])
	})
}

func TestManager_CompleteTask(t *testing.T) {
	ctx := context.Background()

	t.Run("transitions hook to completed", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-complete-task"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepPending,
			CurrentStep: &domain.StepContext{
				StepName: "final",
			},
		}
		require.NoError(t, store.Save(ctx, hook))

		err := m.CompleteTask(ctx, task)
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateCompleted, updatedHook.State)
		assert.Nil(t, updatedHook.CurrentStep)
	})
}

func TestManager_FailTask(t *testing.T) {
	ctx := context.Background()

	t.Run("transitions hook to failed", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-fail-task"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
		}
		require.NoError(t, store.Save(ctx, hook))

		err := m.FailTask(ctx, task, errFatalError)
		require.NoError(t, err)

		updatedHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateFailed, updatedHook.State)
		assert.Len(t, updatedHook.History, 1)
		assert.Equal(t, "fatal error", updatedHook.History[0].Details["error"])
	})
}

func TestManager_InvalidStateTransitions(t *testing.T) {
	ctx := context.Background()

	t.Run("TransitionStep rejects transition from terminal state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-transition-step"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in completed (terminal) state
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateCompleted,
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to transition - should fail
		err := m.TransitionStep(ctx, task, "implement", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
	})

	t.Run("CompleteStep rejects transition from terminal state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-complete-step"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in failed (terminal) state with a CurrentStep to pass the nil check
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateFailed,
			CurrentStep: &domain.StepContext{
				StepName:  "implement",
				StepIndex: 0,
			},
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to complete - should fail due to state validation
		err := m.CompleteStep(ctx, task, "implement", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
	})

	t.Run("FailStep rejects transition from terminal state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-fail-step"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in completed (terminal) state
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateCompleted,
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to fail step - should fail
		err := m.FailStep(ctx, task, "implement", errCompilationFailed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
	})

	t.Run("CompleteTask rejects transition from terminal state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-complete-task"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in already completed state
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateCompleted,
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to complete again - should fail
		err := m.CompleteTask(ctx, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
	})

	t.Run("FailTask rejects transition from terminal state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-fail-task"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in already failed state
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateFailed,
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to fail again - should fail
		err := m.FailTask(ctx, task, errFatalError)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminal state")
	})

	t.Run("CompleteTask rejects invalid non-terminal transition", func(t *testing.T) {
		tmpDir := t.TempDir()
		store := NewFileStore(tmpDir)
		cfg := &config.HookConfig{}
		m := NewManager(store, cfg)

		taskID := "test-invalid-complete-from-running"
		task := &domain.Task{ID: taskID}

		taskDir := filepath.Join(tmpDir, taskID)
		require.NoError(t, os.MkdirAll(taskDir, 0o750))

		// Create hook in step_running state (cannot transition directly to completed)
		hook := &domain.Hook{
			TaskID: taskID,
			State:  domain.HookStateStepRunning,
		}
		require.NoError(t, store.Save(ctx, hook))

		// Attempt to complete task - should fail (must go through step_pending first)
		err := m.CompleteTask(ctx, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid state transition")
	})
}
