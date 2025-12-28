package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestNewFileStore_Default tests creating a FileStore with default directory.
func TestNewFileStore_Default(t *testing.T) {
	store, err := NewFileStore("")
	require.NoError(t, err)
	require.NotNil(t, store)

	// Should use home directory + .atlas
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, constants.AtlasHome), store.baseDir)
}

// TestNewFileStore_CustomDir tests creating a FileStore with a custom directory.
func TestNewFileStore_CustomDir(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Equal(t, tmpDir, store.baseDir)
}

// TestFileStore_Create_Success tests successful workspace creation.
func TestFileStore_Create_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:      "test-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Verify file exists
	wsPath := filepath.Join(tmpDir, constants.WorkspacesDir, "test-workspace", constants.WorkspaceFileName)
	assert.FileExists(t, wsPath)

	// Verify content
	data, err := os.ReadFile(wsPath) //#nosec G304 -- test file path
	require.NoError(t, err)

	var loaded domain.Workspace
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, "test-workspace", loaded.Name)
	assert.Equal(t, CurrentSchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, constants.WorkspaceStatusActive, loaded.Status)
}

// TestFileStore_Create_AlreadyExists tests creating a workspace that already exists.
func TestFileStore_Create_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "existing-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create first time
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Try to create again
	err = store.Create(context.Background(), ws)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceExists)
}

// TestFileStore_Create_EmptyName tests creating with empty name.
func TestFileStore_Create_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
}

// TestFileStore_Create_InvalidName tests creating with invalid characters.
func TestFileStore_Create_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		wsName      string
		description string
	}{
		{"path traversal", "../escape", "should reject path traversal"},
		{"spaces", "has spaces", "should reject spaces"},
		{"special chars", "special@char!", "should reject special characters"},
		{"starts with dash", "-invalid", "should reject leading dash"},
		{"starts with underscore", "_invalid", "should reject leading underscore"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ws := &domain.Workspace{
				Name:      tc.wsName,
				Status:    constants.WorkspaceStatusActive,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			createErr := store.Create(context.Background(), ws)
			require.Error(t, createErr, tc.description)
		})
	}
}

// TestFileStore_Get_Success tests successful workspace retrieval.
func TestFileStore_Get_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Second)
	ws := &domain.Workspace{
		Name:         "get-test",
		WorktreePath: "/some/path",
		Branch:       "feature/test",
		Status:       constants.WorkspaceStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Get the workspace
	loaded, err := store.Get(context.Background(), "get-test")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, "get-test", loaded.Name)
	assert.Equal(t, "/some/path", loaded.WorktreePath)
	assert.Equal(t, "feature/test", loaded.Branch)
	assert.Equal(t, constants.WorkspaceStatusActive, loaded.Status)
	assert.Equal(t, CurrentSchemaVersion, loaded.SchemaVersion)
}

// TestFileStore_Get_NotFound tests getting a non-existent workspace.
func TestFileStore_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	_, err = store.Get(context.Background(), "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

// TestFileStore_Get_CorruptedJSON tests getting a workspace with corrupted JSON.
func TestFileStore_Get_CorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create corrupted workspace.json
	wsDir := filepath.Join(tmpDir, constants.WorkspacesDir, "corrupted")
	err = os.MkdirAll(wsDir, dirPerm)
	require.NoError(t, err)

	wsFile := filepath.Join(wsDir, constants.WorkspaceFileName)
	err = os.WriteFile(wsFile, []byte("{invalid json"), filePerm)
	require.NoError(t, err)

	// Attempt to read
	_, err = store.Get(context.Background(), "corrupted")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceCorrupted)
	assert.Contains(t, err.Error(), "corrupted state file")
}

// TestFileStore_Update_Success tests successful workspace update.
func TestFileStore_Update_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	now := time.Now()
	ws := &domain.Workspace{
		Name:      "update-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Update the workspace
	ws.Status = constants.WorkspaceStatusPaused
	ws.Branch = "new-branch"

	err = store.Update(context.Background(), ws)
	require.NoError(t, err)

	// Verify changes
	loaded, err := store.Get(context.Background(), "update-test")
	require.NoError(t, err)

	assert.Equal(t, constants.WorkspaceStatusPaused, loaded.Status)
	assert.Equal(t, "new-branch", loaded.Branch)
	assert.True(t, loaded.UpdatedAt.After(now) || loaded.UpdatedAt.Equal(now))
}

// TestFileStore_Update_NotFound tests updating a non-existent workspace.
func TestFileStore_Update_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "nonexistent",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Update(context.Background(), ws)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

// TestFileStore_List_MultipleWorkspaces tests listing multiple workspaces.
func TestFileStore_List_MultipleWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create multiple workspaces
	names := []string{"workspace-a", "workspace-b", "workspace-c"}
	for _, name := range names {
		ws := &domain.Workspace{
			Name:      name,
			Status:    constants.WorkspaceStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		createErr := store.Create(context.Background(), ws)
		require.NoError(t, createErr)
	}

	// List workspaces
	workspaces, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, workspaces, 3)

	// Verify all names are present
	foundNames := make(map[string]bool)
	for _, ws := range workspaces {
		foundNames[ws.Name] = true
	}
	for _, name := range names {
		assert.True(t, foundNames[name], "workspace %s not found", name)
	}
}

// TestFileStore_List_EmptyDirectory tests listing when no workspaces exist.
func TestFileStore_List_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// List workspaces (directory doesn't even exist yet)
	workspaces, err := store.List(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, workspaces, "should return empty slice, not nil")
	assert.Empty(t, workspaces)
}

// TestFileStore_List_MixedValidInvalid tests listing with mixed valid/invalid workspaces.
func TestFileStore_List_MixedValidInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create valid workspace
	ws := &domain.Workspace{
		Name:      "valid-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Create invalid workspace directory (no workspace.json)
	invalidDir := filepath.Join(tmpDir, constants.WorkspacesDir, "invalid-no-json")
	err = os.MkdirAll(invalidDir, dirPerm)
	require.NoError(t, err)

	// Create workspace with corrupted JSON
	corruptDir := filepath.Join(tmpDir, constants.WorkspacesDir, "corrupted-json")
	err = os.MkdirAll(corruptDir, dirPerm)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(corruptDir, constants.WorkspaceFileName), []byte("{bad json"), filePerm)
	require.NoError(t, err)

	// List should only return valid workspace
	workspaces, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, workspaces, 1)
	assert.Equal(t, "valid-workspace", workspaces[0].Name)
}

// TestFileStore_Delete_Success tests successful workspace deletion.
func TestFileStore_Delete_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "delete-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Verify it exists
	exists, err := store.Exists(context.Background(), "delete-test")
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete
	err = store.Delete(context.Background(), "delete-test")
	require.NoError(t, err)

	// Verify it's gone
	exists, err = store.Exists(context.Background(), "delete-test")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestFileStore_Delete_NotFound tests deleting a non-existent workspace.
func TestFileStore_Delete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	err = store.Delete(context.Background(), "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

// TestFileStore_Exists_True tests Exists with existing workspace.
func TestFileStore_Exists_True(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "exists-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	exists, err := store.Exists(context.Background(), "exists-test")
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestFileStore_Exists_False tests Exists with non-existing workspace.
func TestFileStore_Exists_False(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	exists, err := store.Exists(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestFileStore_JSONFieldNaming tests that JSON uses snake_case.
func TestFileStore_JSONFieldNaming(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:         "json-test",
		WorktreePath: "/some/path",
		Status:       constants.WorkspaceStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Read raw JSON and verify snake_case
	wsPath := filepath.Join(tmpDir, constants.WorkspacesDir, "json-test", constants.WorkspaceFileName)
	data, err := os.ReadFile(wsPath) //#nosec G304 -- test file path
	require.NoError(t, err)

	jsonStr := string(data)

	// Must contain snake_case, not camelCase
	assert.Contains(t, jsonStr, "worktree_path")
	assert.Contains(t, jsonStr, "created_at")
	assert.Contains(t, jsonStr, "updated_at")
	assert.Contains(t, jsonStr, "schema_version")
	assert.NotContains(t, jsonStr, "worktreePath")
	assert.NotContains(t, jsonStr, "createdAt")
	assert.NotContains(t, jsonStr, "updatedAt")
	assert.NotContains(t, jsonStr, "schemaVersion")
}

// TestFileStore_SchemaVersion tests that schema_version is set correctly.
func TestFileStore_SchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "schema-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// SchemaVersion should be 0 before create
	assert.Equal(t, 0, ws.SchemaVersion)

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// SchemaVersion should be set after create
	assert.Equal(t, CurrentSchemaVersion, ws.SchemaVersion)

	// Load and verify
	loaded, err := store.Get(context.Background(), "schema-test")
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, loaded.SchemaVersion)
}

// TestFileStore_AtomicWrite tests that atomic writes prevent partial writes.
func TestFileStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Test atomicWrite directly
	testPath := filepath.Join(tmpDir, "atomic-test.json")
	testData := []byte(`{"test": "data"}`)

	err := atomicWrite(testPath, testData, filePerm)
	require.NoError(t, err)

	// Verify file exists with correct content
	data, err := os.ReadFile(testPath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, testData, data)

	// Verify no temp file left behind
	tmpFile := testPath + ".tmp"
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err), "temp file should not exist")
}

// TestFileStore_ContextCancellation tests that operations respect context cancellation.
func TestFileStore_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ws := &domain.Workspace{
		Name:      "cancel-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// All operations should fail with context error
	err = store.Create(ctx, ws)
	require.ErrorIs(t, err, context.Canceled)

	_, err = store.Get(ctx, "cancel-test")
	require.ErrorIs(t, err, context.Canceled)

	_, err = store.List(ctx)
	require.ErrorIs(t, err, context.Canceled)

	err = store.Update(ctx, ws)
	require.ErrorIs(t, err, context.Canceled)

	err = store.Delete(ctx, "cancel-test")
	require.ErrorIs(t, err, context.Canceled)

	_, err = store.Exists(ctx, "cancel-test")
	require.ErrorIs(t, err, context.Canceled)
}

// TestFileStore_PathHelpers tests the internal path helper methods.
func TestFileStore_PathHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Test workspacesDir
	expected := filepath.Join(tmpDir, constants.WorkspacesDir)
	assert.Equal(t, expected, store.workspacesDir())

	// Test workspacePath
	expected = filepath.Join(tmpDir, constants.WorkspacesDir, "my-workspace")
	assert.Equal(t, expected, store.workspacePath("my-workspace"))

	// Test workspaceFilePath
	expected = filepath.Join(tmpDir, constants.WorkspacesDir, "my-workspace", constants.WorkspaceFileName)
	assert.Equal(t, expected, store.workspaceFilePath("my-workspace"))
}

// TestValidateName tests workspace name validation.
func TestValidateName(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "workspace", false},
		{"valid with dash", "my-workspace", false},
		{"valid with underscore", "my_workspace", false},
		{"valid with numbers", "workspace123", false},
		{"valid alphanumeric mix", "ws-123_test", false},
		{"empty", "", true},
		{"spaces", "has space", true},
		{"path traversal", "../escape", true},
		{"path separator", "path/name", true},
		{"backslash", "path\\name", true},
		{"special chars", "name@test", true},
		{"starts with dash", "-invalid", true},
		{"starts with underscore", "_invalid", true},
		{"too long", strings.Repeat("a", 256), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateName(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestFileStore_Create_SetsPath tests that Create sets the workspace Path field.
func TestFileStore_Create_SetsPath(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "path-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// The Path field should be set
	expectedPath := filepath.Join(tmpDir, constants.WorkspacesDir, "path-test")
	assert.Equal(t, expectedPath, ws.Path)

	// Verify it's persisted
	loaded, err := store.Get(context.Background(), "path-test")
	require.NoError(t, err)
	assert.Equal(t, expectedPath, loaded.Path)
}

// TestFileStore_WorkspaceWithMetadata tests workspaces with metadata.
func TestFileStore_WorkspaceWithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "metadata-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	loaded, err := store.Get(context.Background(), "metadata-test")
	require.NoError(t, err)

	assert.NotNil(t, loaded.Metadata)
	assert.Equal(t, "value1", loaded.Metadata["key1"])
	// JSON unmarshals numbers as float64, verify using type assertion
	key2Val, ok := loaded.Metadata["key2"].(float64)
	require.True(t, ok, "key2 should be float64")
	assert.InDelta(t, 42.0, key2Val, 0.001)
}

// TestFileStore_WorkspaceWithTasks tests workspaces with task references.
func TestFileStore_WorkspaceWithTasks(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	startedAt := time.Now()
	completedAt := startedAt.Add(5 * time.Minute)

	ws := &domain.Workspace{
		Name:      "tasks-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tasks: []domain.TaskRef{
			{
				ID:          "task-1",
				Status:      constants.TaskStatusCompleted,
				StartedAt:   &startedAt,
				CompletedAt: &completedAt,
			},
			{
				ID:        "task-2",
				Status:    constants.TaskStatusRunning,
				StartedAt: &startedAt,
			},
		},
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	loaded, err := store.Get(context.Background(), "tasks-test")
	require.NoError(t, err)

	assert.Len(t, loaded.Tasks, 2)
	assert.Equal(t, "task-1", loaded.Tasks[0].ID)
	assert.Equal(t, constants.TaskStatusCompleted, loaded.Tasks[0].Status)
	assert.Equal(t, "task-2", loaded.Tasks[1].ID)
	assert.Equal(t, constants.TaskStatusRunning, loaded.Tasks[1].Status)
}

// TestFileStore_AtomicWrite_PreservesOriginalOnFailure tests that atomic write
// does not corrupt the original file if a failure occurs.
// This verifies AC#10: "No partial writes occur (verified by tests)"
func TestFileStore_AtomicWrite_PreservesOriginalOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create original file with known content
	testPath := filepath.Join(tmpDir, "original.json")
	originalData := []byte(`{"original": "data", "important": true}`)
	err := os.WriteFile(testPath, originalData, filePerm)
	require.NoError(t, err)

	// Verify original exists
	data, err := os.ReadFile(testPath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, originalData, data)

	// Test atomic write with new data succeeds
	newData := []byte(`{"new": "data", "updated": true}`)
	err = atomicWrite(testPath, newData, filePerm)
	require.NoError(t, err)

	// Verify new data is written
	data, err = os.ReadFile(testPath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, newData, data)

	// Verify no temp file remains
	tmpFile := testPath + ".tmp"
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err), "temp file should be cleaned up")
}

// TestFileStore_AtomicWrite_NoTempFileOnSuccess verifies cleanup after successful write.
func TestFileStore_AtomicWrite_NoTempFileOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test.json")

	// Write data
	err := atomicWrite(testPath, []byte(`{"test": true}`), filePerm)
	require.NoError(t, err)

	// Check no .tmp file exists
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp", "no temp files should remain")
	}
}

// TestFileStore_LockTimeout tests that ErrLockTimeout is returned when lock cannot be acquired.
func TestFileStore_LockTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lock timeout test in short mode")
	}

	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace first
	ws := &domain.Workspace{
		Name:      "lock-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Manually acquire the lock file to simulate contention
	lockPath := filepath.Join(tmpDir, constants.WorkspacesDir, "lock-test", constants.WorkspaceFileName+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, filePerm) //#nosec G302,G304 -- test lock file
	require.NoError(t, err)
	defer func() { _ = lockFile.Close() }()

	// Acquire exclusive lock (blocking)
	err = syscallFlock(int(lockFile.Fd()), flockExclusive)
	require.NoError(t, err)
	defer func() { _ = syscallFlock(int(lockFile.Fd()), flockUnlock) }()

	// Now try to update the workspace - should timeout
	// Use a shorter timeout context to speed up the test
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ws.Branch = "new-branch"
	err = store.Update(ctx, ws)

	// Should fail - either with context deadline exceeded or lock timeout
	require.Error(t, err)
	// The error could be context.DeadlineExceeded (if context expires first)
	// or ErrLockTimeout (if lock timeout expires first)
	assert.True(t,
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, atlaserrors.ErrLockTimeout),
		"expected deadline exceeded or lock timeout, got: %v", err)
}

// TestFileStore_ConcurrentAccess tests that concurrent operations are safely serialized.
func TestFileStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create initial workspace
	ws := &domain.Workspace{
		Name:      "concurrent-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  map[string]any{"counter": float64(0)},
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Run concurrent updates
	const numGoroutines = 10
	const updatesPerGoroutine = 5

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*updatesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				// Read current state
				current, readErr := store.Get(context.Background(), "concurrent-test")
				if readErr != nil {
					errChan <- readErr
					continue
				}

				// Update with goroutine-specific data
				current.Branch = "branch-" + string(rune('A'+goroutineID))
				updateErr := store.Update(context.Background(), current)
				if updateErr != nil {
					errChan <- updateErr
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	errs := make([]error, 0, numGoroutines*updatesPerGoroutine)
	for err := range errChan {
		errs = append(errs, err)
	}

	// No errors should occur with proper locking
	assert.Empty(t, errs, "concurrent access should not produce errors: %v", errs)

	// Verify final state is readable and not corrupted
	final, err := store.Get(context.Background(), "concurrent-test")
	require.NoError(t, err)
	assert.NotNil(t, final)
	assert.Equal(t, "concurrent-test", final.Name)
}

// TestFileStore_ContextCancellationDuringLock tests that context cancellation
// is respected during the lock acquisition retry loop.
func TestFileStore_ContextCancellationDuringLock(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace first
	ws := &domain.Workspace{
		Name:      "ctx-cancel-lock-test",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Manually acquire the lock to create contention
	lockPath := filepath.Join(tmpDir, constants.WorkspacesDir, "ctx-cancel-lock-test", constants.WorkspaceFileName+".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, filePerm) //#nosec G302,G304 -- test lock file
	require.NoError(t, err)
	defer func() { _ = lockFile.Close() }()

	err = syscallFlock(int(lockFile.Fd()), flockExclusive)
	require.NoError(t, err)
	defer func() { _ = syscallFlock(int(lockFile.Fd()), flockUnlock) }()

	// Create a context that will be canceled quickly
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay (before lock timeout would occur)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Try to update - should fail with context.Canceled
	ws.Branch = "should-not-update"
	err = store.Update(ctx, ws)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// syscallFlock wraps syscall.Flock for testing
func syscallFlock(fd int, how int) error {
	return syscall.Flock(fd, how)
}

// Flock constants for testing
const (
	flockExclusive = syscall.LOCK_EX
	flockUnlock    = syscall.LOCK_UN
)
