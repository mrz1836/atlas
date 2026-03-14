package hook

import (
	"context"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// BenchmarkFileStore_Create benchmarks hook creation.
func BenchmarkFileStore_Create(b *testing.B) {
	store := NewFileStore(b.TempDir())
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		taskID := "bench-task-" + string(rune(i%26+'a'))
		_, _ = store.Create(ctx, taskID, "bench-workspace")
	}
}

// BenchmarkFileStore_Get benchmarks hook retrieval.
func BenchmarkFileStore_Get(b *testing.B) {
	store := NewFileStore(b.TempDir())
	ctx := context.Background()

	// Create a hook first
	_, err := store.Create(ctx, "bench-task", "bench-workspace")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, "bench-task")
	}
}

// BenchmarkFileStore_Update benchmarks hook updates.
func BenchmarkFileStore_Update(b *testing.B) {
	store := NewFileStore(b.TempDir())
	ctx := context.Background()

	// Create a hook first
	_, err := store.Create(ctx, "bench-task", "bench-workspace")
	if err != nil {
		b.Fatal(err)
	}

	updateFn := func(h *domain.Hook) error {
		h.UpdatedAt = time.Now()
		h.History = append(h.History, domain.HookEvent{
			Timestamp: time.Now(),
			FromState: h.State,
			ToState:   domain.HookStateStepRunning,
			Trigger:   "benchmark",
		})
		return nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = store.Update(ctx, "bench-task", updateFn)
	}
}

// BenchmarkFileStore_Save benchmarks hook saves.
func BenchmarkFileStore_Save(b *testing.B) {
	store := NewFileStore(b.TempDir())
	ctx := context.Background()

	// Create a hook first to set up the directory
	_, err := store.Create(ctx, "bench-task", "bench-workspace")
	if err != nil {
		b.Fatal(err)
	}

	hook := &domain.Hook{
		Version:     "1.0",
		TaskID:      "bench-task",
		WorkspaceID: "bench-workspace",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		State:       domain.HookStateStepRunning,
		CurrentStep: &domain.StepContext{
			StepName:  "implement",
			StepIndex: 1,
		},
		History: []domain.HookEvent{
			{Timestamp: time.Now(), Trigger: "test"},
		},
		Checkpoints: make([]domain.StepCheckpoint, 0),
		Receipts:    make([]domain.ValidationReceipt, 0),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hook.UpdatedAt = time.Now()
		_ = store.Save(ctx, hook)
	}
}

// BenchmarkFileStore_GetSnapshot benchmarks snapshot creation.
func BenchmarkFileStore_GetSnapshot(b *testing.B) {
	store := NewFileStore(b.TempDir())
	ctx := context.Background()

	// Create a hook with some data
	hook, err := store.Create(ctx, "bench-task", "bench-workspace")
	if err != nil {
		b.Fatal(err)
	}

	// Add some history and checkpoints
	hook.History = make([]domain.HookEvent, 10)
	for i := range hook.History {
		hook.History[i] = domain.HookEvent{
			Timestamp: time.Now(),
			FromState: domain.HookStateStepPending,
			ToState:   domain.HookStateStepRunning,
			Trigger:   "test",
			Details:   map[string]any{"key": "value"},
		}
	}
	hook.Checkpoints = make([]domain.StepCheckpoint, 5)
	for i := range hook.Checkpoints {
		hook.Checkpoints[i] = domain.StepCheckpoint{
			CheckpointID: "ckpt-test",
			CreatedAt:    time.Now(),
			StepName:     "step",
			Description:  "test checkpoint",
		}
	}
	_ = store.Save(ctx, hook)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = store.GetSnapshot(ctx, "bench-task")
	}
}
