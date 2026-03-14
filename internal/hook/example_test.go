package hook_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/hook"
)

// ExampleNewManager demonstrates creating a hook Manager for task recovery.
func ExampleNewManager() {
	// Create a temporary directory for the store
	tmpDir, _ := os.MkdirTemp("", "hook-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create the file store and manager
	store := hook.NewFileStore(tmpDir)
	cfg := &config.HookConfig{
		MaxCheckpoints: 50,
	}
	manager := hook.NewManager(store, cfg)

	// Create a hook for a task
	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "my-workspace",
	}
	_ = os.MkdirAll(filepath.Join(tmpDir, task.ID), 0o750)

	ctx := context.Background()
	if err := manager.CreateHook(ctx, task); err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Hook created successfully")
	// Output: Hook created successfully
}

// ExampleFileStore_Create demonstrates creating a new hook via the FileStore.
func ExampleFileStore_Create() {
	// Create a temporary directory
	tmpDir, _ := os.MkdirTemp("", "store-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	store := hook.NewFileStore(tmpDir)
	ctx := context.Background()

	// Create a new hook
	h, err := store.Create(ctx, "task-456", "my-workspace")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("TaskID:", h.TaskID)
	fmt.Println("State:", h.State)
	// Output:
	// TaskID: task-456
	// State: initializing
}

// ExampleFileStore_Update demonstrates updating a hook atomically.
func ExampleFileStore_Update() {
	// Create a temporary directory
	tmpDir, _ := os.MkdirTemp("", "update-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	store := hook.NewFileStore(tmpDir)
	ctx := context.Background()

	// Create a hook first
	_, _ = store.Create(ctx, "task-789", "my-workspace")

	// Update the hook state atomically
	err := store.Update(ctx, "task-789", func(h *domain.Hook) error {
		h.State = domain.HookStateStepRunning
		h.CurrentStep = &domain.StepContext{
			StepName:  "implement",
			StepIndex: 0,
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Verify the update
	h, _ := store.Get(ctx, "task-789")
	fmt.Println("Updated state:", h.State)
	// Output: Updated state: step_running
}

// ExampleCheckpointer_CreateCheckpoint demonstrates creating checkpoints.
func ExampleCheckpointer_CreateCheckpoint() {
	// Create a temporary directory
	tmpDir, _ := os.MkdirTemp("", "checkpoint-example")
	defer func() { _ = os.RemoveAll(tmpDir) }()

	store := hook.NewFileStore(tmpDir)
	cfg := &config.HookConfig{MaxCheckpoints: 50}
	checkpointer := hook.NewCheckpointer(cfg, store)

	// Create a hook with current step context
	h := &domain.Hook{
		TaskID:      "task-abc",
		WorkspaceID: "my-workspace",
		State:       domain.HookStateStepRunning,
		CurrentStep: &domain.StepContext{
			StepName:     "implement",
			StepIndex:    1,
			FilesTouched: []string{"main.go", "utils.go"},
		},
		Checkpoints: []domain.StepCheckpoint{},
	}

	// Create the task directory and save initial hook
	_ = os.MkdirAll(filepath.Join(tmpDir, h.TaskID), 0o750)
	_ = store.Save(context.Background(), h)

	// Create a checkpoint
	ctx := context.Background()
	err := checkpointer.CreateCheckpoint(ctx, h, domain.CheckpointTriggerManual, "Before refactoring")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Checkpoints:", len(h.Checkpoints))
	fmt.Println("Trigger:", h.Checkpoints[0].Trigger)
	// Output:
	// Checkpoints: 1
	// Trigger: manual
}
