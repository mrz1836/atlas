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
func concurrentStepTransitioner(ctx context.Context, m *Manager, task *domain.Task, errChan chan<- error) {
	for i := 0; i < 2; i++ {
		if transitionErr := m.TransitionStep(ctx, task, "step", i); transitionErr != nil {
			errChan <- transitionErr
		}
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
	// This test verifies Issue #1 fix: no data races with concurrent access.
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

		// Check for errors
		for err := range errChan {
			t.Errorf("Unexpected error during concurrent access: %v", err)
		}

		// Verify final state is consistent
		finalHook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.NotNil(t, finalHook)

		// Should have receipts and checkpoints
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
		_, err := store.Create(ctx, taskID, "workspace-1")
		require.NoError(t, err)

		err = m.TransitionStep(ctx, task, "implement", 2)
		require.NoError(t, err)

		hook, err := store.Get(ctx, taskID)
		require.NoError(t, err)
		assert.Equal(t, domain.HookStateStepRunning, hook.State)
		assert.Equal(t, "implement", hook.CurrentStep.StepName)
		assert.Equal(t, 2, hook.CurrentStep.StepIndex)
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
