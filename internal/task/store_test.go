package task

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// createTestTask creates a test task with the given ID.
func createTestTask(id string) *domain.Task {
	now := time.Now().UTC()
	return &domain.Task{
		ID:          id,
		WorkspaceID: "test-ws",
		TemplateID:  "bugfix",
		Description: "Test task",
		Status:      domain.TaskStatusPending,
		CurrentStep: 0,
		Steps: []domain.Step{
			{
				Name:     "analyze",
				Type:     domain.StepTypeAI,
				Status:   "pending",
				Attempts: 0,
			},
		},
		CreatedAt:     now,
		UpdatedAt:     now,
		Config:        domain.TaskConfig{Model: "test-model"},
		SchemaVersion: constants.TaskSchemaVersion,
	}
}

// setupTestStore creates a test store with a temp directory.
func setupTestStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	tmpDir := t.TempDir()

	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create workspace tasks directory
	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	return store, tmpDir
}

func TestNewFileStore(t *testing.T) {
	t.Run("with custom path", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewFileStore(tmpDir)
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, tmpDir, store.atlasHome)
	})

	t.Run("with empty path uses default", func(t *testing.T) {
		store, err := NewFileStore("")
		require.NoError(t, err)
		assert.NotNil(t, store)
		// Should contain .atlas
		assert.Contains(t, store.atlasHome, constants.AtlasHome)
	})
}

func TestFileStore_Create(t *testing.T) {
	t.Run("creates task successfully", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100000")

		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Verify file exists
		taskPath := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID, constants.TaskFileName)
		_, err = os.Stat(taskPath)
		require.NoError(t, err)

		// Verify content
		data, err := os.ReadFile(taskPath) //#nosec G304 -- test file path
		require.NoError(t, err)

		var loaded domain.Task
		err = json.Unmarshal(data, &loaded)
		require.NoError(t, err)
		assert.Equal(t, task.ID, loaded.ID)
		assert.Equal(t, task.Description, loaded.Description)
		assert.Equal(t, constants.TaskSchemaVersion, loaded.SchemaVersion)
	})

	t.Run("errors on duplicate task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100001")

		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Try to create again
		err = store.Create(context.Background(), "test-ws", task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("errors on empty workspace name", func(t *testing.T) {
		store, _ := setupTestStore(t)
		task := createTestTask("task-20251228-100002")

		err := store.Create(context.Background(), "", task)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("errors on nil task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.Create(context.Background(), "test-ws", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("errors on empty task ID", func(t *testing.T) {
		store, _ := setupTestStore(t)
		task := createTestTask("")

		err := store.Create(context.Background(), "test-ws", task)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		store, _ := setupTestStore(t)
		task := createTestTask("task-20251228-100003")

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := store.Create(ctx, "test-ws", task)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestFileStore_Get(t *testing.T) {
	t.Run("retrieves existing task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100010")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		retrieved, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, task.ID, retrieved.ID)
		assert.Equal(t, task.Description, retrieved.Description)
		assert.Equal(t, task.Status, retrieved.Status)
	})

	t.Run("errors on non-existent task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		_, err := store.Get(context.Background(), "test-ws", "task-nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
	})

	t.Run("errors on empty workspace name", func(t *testing.T) {
		store, _ := setupTestStore(t)

		_, err := store.Get(context.Background(), "", "task-20251228-100011")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("errors on empty task ID", func(t *testing.T) {
		store, _ := setupTestStore(t)

		_, err := store.Get(context.Background(), "test-ws", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		store, _ := setupTestStore(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := store.Get(ctx, "test-ws", "task-20251228-100012")
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestFileStore_Update(t *testing.T) {
	t.Run("updates existing task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100020")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Modify and update
		task.Status = domain.TaskStatusRunning
		task.CurrentStep = 1
		task.Description = "Updated description"

		err = store.Update(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Verify
		retrieved, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStatusRunning, retrieved.Status)
		assert.Equal(t, 1, retrieved.CurrentStep)
		assert.Equal(t, "Updated description", retrieved.Description)
		assert.True(t, retrieved.UpdatedAt.After(task.CreatedAt))
	})

	t.Run("errors on non-existent task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-nonexistent")
		err := store.Update(context.Background(), "test-ws", task)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
	})

	t.Run("errors on empty workspace name", func(t *testing.T) {
		store, _ := setupTestStore(t)
		task := createTestTask("task-20251228-100021")

		err := store.Update(context.Background(), "", task)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("errors on nil task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.Update(context.Background(), "test-ws", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})
}

func TestFileStore_List(t *testing.T) {
	t.Run("lists multiple tasks sorted by creation time", func(t *testing.T) {
		store, _ := setupTestStore(t)

		// Create tasks with different creation times
		task1 := createTestTask("task-20251228-100030")
		task1.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)

		task2 := createTestTask("task-20251228-100031")
		task2.CreatedAt = time.Now().UTC().Add(-1 * time.Hour)

		task3 := createTestTask("task-20251228-100032")
		task3.CreatedAt = time.Now().UTC()

		require.NoError(t, store.Create(context.Background(), "test-ws", task1))
		require.NoError(t, store.Create(context.Background(), "test-ws", task2))
		require.NoError(t, store.Create(context.Background(), "test-ws", task3))

		tasks, err := store.List(context.Background(), "test-ws")
		require.NoError(t, err)
		require.Len(t, tasks, 3)

		// Should be sorted newest first
		assert.Equal(t, task3.ID, tasks[0].ID)
		assert.Equal(t, task2.ID, tasks[1].ID)
		assert.Equal(t, task1.ID, tasks[2].ID)
	})

	t.Run("returns empty list for empty workspace", func(t *testing.T) {
		store, _ := setupTestStore(t)

		tasks, err := store.List(context.Background(), "test-ws")
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("errors on empty workspace name", func(t *testing.T) {
		store, _ := setupTestStore(t)

		_, err := store.List(context.Background(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})
}

func TestFileStore_Delete(t *testing.T) {
	t.Run("deletes existing task", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100040")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		err = store.Delete(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)

		// Verify task directory is removed
		taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID)
		_, err = os.Stat(taskDir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("errors on non-existent task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.Delete(context.Background(), "test-ws", "task-nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
	})

	t.Run("errors on empty workspace name", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.Delete(context.Background(), "", "task-20251228-100041")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("errors on empty task ID", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.Delete(context.Background(), "test-ws", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})
}

func TestFileStore_AppendLog(t *testing.T) {
	t.Run("appends log entries", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100050")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Append multiple log entries
		entry1 := []byte(`{"level":"info","msg":"Starting task"}`)
		entry2 := []byte(`{"level":"info","msg":"Task running"}`)

		err = store.AppendLog(context.Background(), "test-ws", task.ID, entry1)
		require.NoError(t, err)

		err = store.AppendLog(context.Background(), "test-ws", task.ID, entry2)
		require.NoError(t, err)

		// Verify log file
		logPath := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID, constants.TaskLogFileName)
		data, err := os.ReadFile(logPath) //#nosec G304 -- test file path
		require.NoError(t, err)

		// Each entry should be on its own line
		assert.Contains(t, string(data), `{"level":"info","msg":"Starting task"}`)
		assert.Contains(t, string(data), `{"level":"info","msg":"Task running"}`)
	})

	t.Run("adds newline if missing", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100051")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		entry := []byte(`{"level":"info","msg":"No newline"}`)
		err = store.AppendLog(context.Background(), "test-ws", task.ID, entry)
		require.NoError(t, err)

		logPath := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID, constants.TaskLogFileName)
		data, err := os.ReadFile(logPath) //#nosec G304 -- test file path
		require.NoError(t, err)

		assert.Equal(t, byte('\n'), data[len(data)-1])
	})

	t.Run("errors on non-existent task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.AppendLog(context.Background(), "test-ws", "task-nonexistent", []byte("test"))
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
	})
}

func TestFileStore_Artifacts(t *testing.T) {
	t.Run("saves and retrieves artifact", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100060")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		artifactData := []byte(`{"result":"success"}`)
		err = store.SaveArtifact(context.Background(), "test-ws", task.ID, "result.json", artifactData)
		require.NoError(t, err)

		retrieved, err := store.GetArtifact(context.Background(), "test-ws", task.ID, "result.json")
		require.NoError(t, err)
		assert.Equal(t, artifactData, retrieved)
	})

	t.Run("lists artifacts", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100061")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		require.NoError(t, store.SaveArtifact(context.Background(), "test-ws", task.ID, "a.json", []byte("{}")))
		require.NoError(t, store.SaveArtifact(context.Background(), "test-ws", task.ID, "b.json", []byte("{}")))
		require.NoError(t, store.SaveArtifact(context.Background(), "test-ws", task.ID, "c.json", []byte("{}")))

		files, err := store.ListArtifacts(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		require.Len(t, files, 3)

		// Should be sorted
		assert.Equal(t, "a.json", files[0])
		assert.Equal(t, "b.json", files[1])
		assert.Equal(t, "c.json", files[2])
	})

	t.Run("returns empty list for no artifacts", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100062")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		files, err := store.ListArtifacts(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("prevents path traversal in filename", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100063")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		err = store.SaveArtifact(context.Background(), "test-ws", task.ID, "../evil.json", []byte("{}"))
		require.ErrorIs(t, err, atlaserrors.ErrPathTraversal)

		err = store.SaveArtifact(context.Background(), "test-ws", task.ID, "sub/dir.json", []byte("{}"))
		require.ErrorIs(t, err, atlaserrors.ErrPathTraversal)
	})

	t.Run("errors on non-existent task", func(t *testing.T) {
		store, _ := setupTestStore(t)

		err := store.SaveArtifact(context.Background(), "test-ws", "task-nonexistent", "file.json", []byte("{}"))
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
	})

	t.Run("errors on non-existent artifact", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100064")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		_, err = store.GetArtifact(context.Background(), "test-ws", task.ID, "nonexistent.json")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrArtifactNotFound)
	})
}

func TestFileStore_SaveVersionedArtifact(t *testing.T) {
	t.Run("creates versioned artifacts", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100070")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Save multiple versions
		name1, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "validation.json", []byte(`{"v":1}`))
		require.NoError(t, err)
		assert.Equal(t, "validation.1.json", name1)

		name2, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "validation.json", []byte(`{"v":2}`))
		require.NoError(t, err)
		assert.Equal(t, "validation.2.json", name2)

		name3, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "validation.json", []byte(`{"v":3}`))
		require.NoError(t, err)
		assert.Equal(t, "validation.3.json", name3)

		// Verify all exist
		files, err := store.ListArtifacts(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		require.Len(t, files, 3)
	})

	t.Run("handles different extensions", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100071")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		name1, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "report.md", []byte("# Report"))
		require.NoError(t, err)
		assert.Equal(t, "report.1.md", name1)

		name2, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "data.txt", []byte("data"))
		require.NoError(t, err)
		assert.Equal(t, "data.1.txt", name2)
	})

	t.Run("prevents path traversal", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100072")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		_, err = store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "../evil.json", []byte("{}"))
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrPathTraversal)
	})
}

func TestFileStore_AtomicWrite(t *testing.T) {
	t.Run("atomic write prevents partial data on failure", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100080")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Get the original data
		original, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)

		// Modify task
		task.Description = "Modified"
		err = store.Update(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Verify no temp file left behind
		taskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID)
		entries, err := os.ReadDir(taskDir)
		require.NoError(t, err)

		for _, entry := range entries {
			assert.NotContains(t, entry.Name(), ".tmp")
		}

		// Verify update worked
		updated, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, "Modified", updated.Description)
		assert.NotEqual(t, original.Description, updated.Description)
	})
}

func TestFileStore_ConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent updates with locking", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100090")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Run concurrent updates
		var wg sync.WaitGroup
		numGoroutines := 10
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				// Get current state
				current, getErr := store.Get(context.Background(), "test-ws", task.ID)
				if getErr != nil {
					errors <- getErr
					return
				}

				// Modify
				current.CurrentStep = i
				if updateErr := store.Update(context.Background(), "test-ws", current); updateErr != nil {
					errors <- updateErr
					return
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// All operations should succeed (with locking)
		for rangeErr := range errors {
			require.NoError(t, rangeErr)
		}

		// Verify task is still valid
		final, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.NotNil(t, final)
	})
}

func TestFileStore_CorruptedJSON(t *testing.T) {
	t.Run("returns error for corrupted task.json", func(t *testing.T) {
		store, tmpDir := setupTestStore(t)

		task := createTestTask("task-20251228-100100")
		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		// Corrupt the file
		taskFile := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID, constants.TaskFileName)
		err = os.WriteFile(taskFile, []byte("not valid json"), 0o600)
		require.NoError(t, err)

		// Try to read
		_, err = store.Get(context.Background(), "test-ws", task.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "corrupted")
	})
}

func TestGenerateTaskID(t *testing.T) {
	t.Run("generates valid format", func(t *testing.T) {
		id := GenerateTaskID()
		assert.True(t, validTaskIDRegex.MatchString(id), "ID should match pattern: %s", id)
		assert.GreaterOrEqual(t, len(id), 20, "ID should be at least 20 chars: %s", id)

		// Verify format structure: task-YYYYMMDD-HHMMSS
		assert.GreaterOrEqual(t, len(id), 20, "ID should be at least 20 chars")
		assert.Equal(t, "task-", id[:5], "ID should start with 'task-'")

		// Verify date portion is 8 digits
		datePart := id[5:13]
		for _, c := range datePart {
			assert.True(t, c >= '0' && c <= '9', "Date part should be all digits: %s", datePart)
		}

		// Verify time portion is 6 digits
		timePart := id[14:20]
		for _, c := range timePart {
			assert.True(t, c >= '0' && c <= '9', "Time part should be all digits: %s", timePart)
		}
	})

	t.Run("IDs within same second are identical", func(t *testing.T) {
		// In a tight loop, IDs within the same second will be identical.
		// This is expected behavior - use GenerateTaskIDUnique for uniqueness.
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateTaskID()
			assert.True(t, validTaskIDRegex.MatchString(id), "Each ID should match pattern")
			ids[id] = true
		}
		// Since this runs within a second, we expect 1-2 unique IDs max
		assert.LessOrEqual(t, len(ids), 2, "IDs within same second should be identical or span at most 2 seconds")
	})
}

func TestGenerateTaskIDUnique(t *testing.T) {
	t.Run("returns base ID if not exists", func(t *testing.T) {
		existing := make(map[string]bool)
		id := GenerateTaskIDUnique(existing)
		assert.True(t, validTaskIDRegex.MatchString(id))
	})

	t.Run("adds milliseconds if base exists", func(t *testing.T) {
		existing := make(map[string]bool)

		// Generate first ID
		id1 := GenerateTaskID()
		existing[id1] = true

		// Generate unique ID when first exists
		id2 := GenerateTaskIDUnique(existing)
		assert.True(t, validTaskIDRegex.MatchString(id2))
		assert.NotEqual(t, id1, id2)
		assert.Contains(t, id2, "-") // Should have millisecond suffix
	})
}

func TestFileStore_SchemaVersion(t *testing.T) {
	t.Run("sets schema version on create", func(t *testing.T) {
		store, _ := setupTestStore(t)

		task := createTestTask("task-20251228-100110")
		task.SchemaVersion = "" // Empty before create

		err := store.Create(context.Background(), "test-ws", task)
		require.NoError(t, err)

		retrieved, err := store.Get(context.Background(), "test-ws", task.ID)
		require.NoError(t, err)
		assert.Equal(t, constants.TaskSchemaVersion, retrieved.SchemaVersion)
	})
}

// TestFileStore_releaseLock_NilFile tests that releaseLock handles nil file gracefully.
func TestFileStore_releaseLock_NilFile(t *testing.T) {
	store, _ := setupTestStore(t)

	// Should not panic or error with nil file
	err := store.releaseLock(nil)
	assert.NoError(t, err)
}

// TestFileStore_atomicWrite_Success tests successful atomic write.
func TestFileStore_atomicWrite_Success(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-file.json")
	data := []byte(`{"test": "data"}`)

	err := atomicWrite(filePath, data)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(filePath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, data, content)

	// Verify temp file is cleaned up
	_, err = os.Stat(filePath + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file should not exist after successful write")
}

// TestFileStore_atomicWrite_InvalidPath tests atomic write to an invalid path.
func TestFileStore_atomicWrite_InvalidPath(t *testing.T) {
	// Use a path that doesn't exist
	filePath := "/nonexistent/directory/test-file.json"
	data := []byte(`{"test": "data"}`)

	err := atomicWrite(filePath, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create temp file")
}

// TestFileStore_List_EmptyWorkspace tests listing tasks from an empty workspace.
func TestFileStore_List_EmptyWorkspace(t *testing.T) {
	store, tmpDir := setupTestStore(t)

	// Create workspace tasks directory but don't add any tasks
	wsTaskDir := filepath.Join(tmpDir, constants.WorkspacesDir, "empty-ws", constants.TasksDir)
	require.NoError(t, os.MkdirAll(wsTaskDir, 0o750))

	tasks, err := store.List(context.Background(), "empty-ws")
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

// TestFileStore_GetArtifact_NotFound tests getting a non-existent artifact.
func TestFileStore_GetArtifact_NotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	_, err := store.GetArtifact(context.Background(), "test-ws", "nonexistent-task", "artifact.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestFileStore_ListArtifacts_NoArtifacts tests listing artifacts when none exist.
func TestFileStore_ListArtifacts_NoArtifacts(t *testing.T) {
	store, _ := setupTestStore(t)

	task := createTestTask("task-20251228-110000")
	err := store.Create(context.Background(), "test-ws", task)
	require.NoError(t, err)

	artifacts, err := store.ListArtifacts(context.Background(), "test-ws", task.ID)
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

// TestFileStore_SaveArtifact_AndList tests saving and listing artifacts.
func TestFileStore_SaveArtifact_AndList(t *testing.T) {
	store, _ := setupTestStore(t)

	task := createTestTask("task-20251228-110001")
	err := store.Create(context.Background(), "test-ws", task)
	require.NoError(t, err)

	// Save an artifact
	err = store.SaveArtifact(context.Background(), "test-ws", task.ID, "output.txt", []byte("test content"))
	require.NoError(t, err)

	// List artifacts
	artifacts, err := store.ListArtifacts(context.Background(), "test-ws", task.ID)
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "output.txt", artifacts[0])

	// Get the artifact
	content, err := store.GetArtifact(context.Background(), "test-ws", task.ID, "output.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("test content"), content)
}

// TestFileStore_SaveVersionedArtifact_MultipleVersions tests saving multiple versioned artifacts.
func TestFileStore_SaveVersionedArtifact_MultipleVersions(t *testing.T) {
	store, _ := setupTestStore(t)

	task := createTestTask("task-20251228-110002")
	err := store.Create(context.Background(), "test-ws", task)
	require.NoError(t, err)

	// Save first version
	path1, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "result.json", []byte("v1"))
	require.NoError(t, err)
	assert.Contains(t, path1, "result") // Path contains result

	// Save second version - should create a new file with version suffix
	path2, err := store.SaveVersionedArtifact(context.Background(), "test-ws", task.ID, "result.json", []byte("v2"))
	require.NoError(t, err)
	assert.NotEqual(t, path1, path2) // Different paths
}

// TestFileStore_AppendLog_MultipleEntries tests appending multiple entries to task logs.
func TestFileStore_AppendLog_MultipleEntries(t *testing.T) {
	store, tmpDir := setupTestStore(t)

	task := createTestTask("task-20251228-110003")
	err := store.Create(context.Background(), "test-ws", task)
	require.NoError(t, err)

	// Append log entries
	err = store.AppendLog(context.Background(), "test-ws", task.ID, []byte("First entry\n"))
	require.NoError(t, err)

	err = store.AppendLog(context.Background(), "test-ws", task.ID, []byte("Second entry\n"))
	require.NoError(t, err)

	// Read the log file and verify content - log file is directly in task directory
	logPath := filepath.Join(tmpDir, constants.WorkspacesDir, "test-ws", constants.TasksDir, task.ID, constants.TaskLogFileName)
	content, err := os.ReadFile(logPath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Contains(t, string(content), "First entry")
	assert.Contains(t, string(content), "Second entry")
}

// TestFileStore_Delete_NotFound tests deleting a non-existent task.
func TestFileStore_Delete_NotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	err := store.Delete(context.Background(), "test-ws", "nonexistent-task")
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
}

// TestFileStore_Update_NotFound tests updating a non-existent task.
func TestFileStore_Update_NotFound(t *testing.T) {
	store, _ := setupTestStore(t)

	task := createTestTask("nonexistent-task")
	err := store.Update(context.Background(), "test-ws", task)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTaskNotFound)
}
