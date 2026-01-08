package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// TestIntegration_TaskLifecycle tests the full task lifecycle:
// create -> update status -> add artifacts -> complete
func TestIntegration_TaskLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create workspace directory structure
	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	// Create store
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	var task *domain.Task
	var artifactContent []byte

	t.Run("1_create_task", func(t *testing.T) {
		task = &domain.Task{
			ID:          "task-11111111-1111-4111-8111-111111111111",
			WorkspaceID: "test-ws",
			TemplateID:  "bugfix",
			Description: "Integration test task",
			Status:      domain.TaskStatusPending,
			CurrentStep: 0,
			Steps: []domain.Step{
				{Name: "analyze", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "implement", Type: domain.StepTypeAI, Status: "pending"},
				{Name: "validate", Type: domain.StepTypeValidation, Status: "pending"},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Config:    domain.TaskConfig{Model: "sonnet"},
		}

		err = store.Create(ctx, "test-ws", task)
		require.NoError(t, err, "should create task")
	})

	t.Run("2_get_and_verify_task", func(t *testing.T) {
		var retrieved *domain.Task
		retrieved, err = store.Get(ctx, "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, task.ID, retrieved.ID)
		assert.Equal(t, domain.TaskStatusPending, retrieved.Status)
		assert.Len(t, retrieved.Steps, 3)
	})

	t.Run("3_update_task_status", func(t *testing.T) {
		var retrieved *domain.Task
		retrieved, err = store.Get(ctx, "test-ws", task.ID)
		require.NoError(t, err)
		retrieved.Status = domain.TaskStatusRunning
		retrieved.CurrentStep = 1
		retrieved.Steps[0].Status = "completed"
		err = store.Update(ctx, "test-ws", retrieved)
		require.NoError(t, err)
	})

	t.Run("4_add_log_entry", func(t *testing.T) {
		logEntry := []byte(`{"step":"analyze","status":"completed","duration":"5s"}` + "\n")
		err = store.AppendLog(ctx, "test-ws", task.ID, logEntry)
		require.NoError(t, err)
	})

	t.Run("5_save_artifact", func(t *testing.T) {
		artifactContent = []byte("// Generated code from AI\nfunc Fix() {}")
		err = store.SaveArtifact(ctx, "test-ws", task.ID, "fix.go", artifactContent)
		require.NoError(t, err)
	})

	t.Run("6_list_artifacts", func(t *testing.T) {
		artifacts, err := store.ListArtifacts(ctx, "test-ws", task.ID)
		require.NoError(t, err)
		assert.Contains(t, artifacts, "fix.go")
	})

	t.Run("7_get_artifact", func(t *testing.T) {
		content, err := store.GetArtifact(ctx, "test-ws", task.ID, "fix.go")
		require.NoError(t, err)
		assert.Equal(t, artifactContent, content)
	})

	t.Run("8_complete_task", func(t *testing.T) {
		final, err := store.Get(ctx, "test-ws", task.ID)
		require.NoError(t, err)
		final.Status = domain.TaskStatusCompleted
		final.CurrentStep = 3
		for i := range final.Steps {
			final.Steps[i].Status = "completed"
		}
		err = store.Update(ctx, "test-ws", final)
		require.NoError(t, err)
	})

	t.Run("9_verify_final_state", func(t *testing.T) {
		completed, err := store.Get(ctx, "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusCompleted, completed.Status)
	})

	t.Run("10_list_tasks", func(t *testing.T) {
		tasks, err := store.List(ctx, "test-ws")
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
	})
}

// TestIntegration_TaskWithVersionedArtifacts tests saving multiple versions of artifacts.
func TestIntegration_TaskWithVersionedArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create workspace directory structure
	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create task
	task := &domain.Task{
		ID:          "task-11111111-1111-4111-8111-111111111111",
		WorkspaceID: "test-ws",
		TemplateID:  "feature",
		Description: "Versioned artifact test",
		Status:      domain.TaskStatusRunning,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = store.Create(ctx, "test-ws", task)
	require.NoError(t, err)

	// Save multiple versions of same artifact
	versions := []string{"v1 content", "v2 content", "v3 content"}
	paths := make([]string, len(versions))

	for i, content := range versions {
		var path string
		path, err = store.SaveVersionedArtifact(ctx, "test-ws", task.ID, "output.txt", []byte(content))
		require.NoError(t, err)
		paths[i] = path
	}

	// All paths should be different
	assert.NotEqual(t, paths[0], paths[1])
	assert.NotEqual(t, paths[1], paths[2])

	// List artifacts - should have multiple files
	artifacts, err := store.ListArtifacts(ctx, "test-ws", task.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(artifacts), 3, "should have at least 3 versioned artifacts")
}

// TestIntegration_TaskStateTransitions tests valid state transitions.
func TestIntegration_TaskStateTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Test valid transitions: pending -> running -> completed
	task := &domain.Task{
		ID:          "task-11111111-1111-4111-8111-111111111111",
		WorkspaceID: "test-ws",
		Status:      domain.TaskStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err = store.Create(ctx, "test-ws", task)
	require.NoError(t, err)

	// Pending -> Running
	task.Status = domain.TaskStatusRunning
	err = store.Update(ctx, "test-ws", task)
	require.NoError(t, err)

	retrieved, _ := store.Get(ctx, "test-ws", task.ID)
	assert.Equal(t, domain.TaskStatusRunning, retrieved.Status)

	// Running -> Completed
	task.Status = domain.TaskStatusCompleted
	err = store.Update(ctx, "test-ws", task)
	require.NoError(t, err)

	retrieved, _ = store.Get(ctx, "test-ws", task.ID)
	assert.Equal(t, domain.TaskStatusCompleted, retrieved.Status)
}

// TestIntegration_TaskFailureAndRetry tests the failure and retry flow.
func TestIntegration_TaskFailureAndRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create task
	task := &domain.Task{
		ID:          "task-11111111-1111-4111-8111-111111111111",
		WorkspaceID: "test-ws",
		Status:      domain.TaskStatusRunning,
		CurrentStep: 1,
		Steps: []domain.Step{
			{Name: "analyze", Status: "completed", Attempts: 1},
			{Name: "validate", Status: "pending", Attempts: 0},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(ctx, "test-ws", task)
	require.NoError(t, err)

	// Simulate validation failure
	task.Status = domain.TaskStatusValidationFailed
	task.Steps[1].Status = "failed"
	task.Steps[1].Attempts = 1
	task.Steps[1].Error = "lint check failed: unused variable"
	err = store.Update(ctx, "test-ws", task)
	require.NoError(t, err)

	// Verify failure state
	failed, _ := store.Get(ctx, "test-ws", task.ID)
	assert.Equal(t, domain.TaskStatusValidationFailed, failed.Status)
	assert.Equal(t, "failed", failed.Steps[1].Status)

	// Simulate retry
	task.Status = domain.TaskStatusRunning
	task.Steps[1].Status = "running"
	task.Steps[1].Attempts = 2
	task.Steps[1].Error = ""
	err = store.Update(ctx, "test-ws", task)
	require.NoError(t, err)

	// Simulate success after retry
	task.Status = domain.TaskStatusCompleted
	task.Steps[1].Status = "completed"
	err = store.Update(ctx, "test-ws", task)
	require.NoError(t, err)

	// Verify final state
	completed, _ := store.Get(ctx, "test-ws", task.ID)
	assert.Equal(t, domain.TaskStatusCompleted, completed.Status)
	assert.Equal(t, 2, completed.Steps[1].Attempts, "should record retry attempt")
}
