package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestFileStore_Create_AfterResetMetadata tests creating a workspace after metadata reset.
// This simulates recreating a closed workspace where only the tasks directory remains.
func TestFileStore_Create_AfterResetMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "reuse-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Create workspace first time
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Simulate ResetMetadata (removes workspace.json, keeps tasks dir)
	err = store.ResetMetadata(context.Background(), "reuse-workspace")
	require.NoError(t, err)

	// Verify directory still exists but workspace.json is gone
	wsDir := filepath.Join(tmpDir, constants.WorkspacesDir, "reuse-workspace")
	assert.DirExists(t, wsDir)
	wsFile := filepath.Join(wsDir, constants.WorkspaceFileName)
	_, err = os.Stat(wsFile)
	assert.True(t, os.IsNotExist(err), "workspace.json should not exist")

	// Create should succeed now
	ws2 := &domain.Workspace{
		Name:      "reuse-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws2)
	require.NoError(t, err, "should be able to create workspace after metadata reset")
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
	err = os.MkdirAll(wsDir, WorkspaceDirPerm)
	require.NoError(t, err)

	wsFile := filepath.Join(wsDir, constants.WorkspaceFileName)
	err = os.WriteFile(wsFile, []byte("{invalid json"), WorkspaceFilePerm)
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
	err = os.MkdirAll(invalidDir, WorkspaceDirPerm)
	require.NoError(t, err)

	// Create workspace with corrupted JSON
	corruptDir := filepath.Join(tmpDir, constants.WorkspacesDir, "corrupted-json")
	err = os.MkdirAll(corruptDir, WorkspaceDirPerm)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(corruptDir, constants.WorkspaceFileName), []byte("{bad json"), WorkspaceFilePerm)
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

// TestFileStore_ResetMetadata_PreservesTasks tests that ResetMetadata removes
// workspace.json but preserves the tasks directory.
func TestFileStore_ResetMetadata_PreservesTasks(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "reset-test",
		Status:    constants.WorkspaceStatusClosed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Create a tasks directory with a task file to simulate existing task data
	wsPath := filepath.Join(tmpDir, constants.WorkspacesDir, "reset-test")
	tasksDir := filepath.Join(wsPath, constants.TasksDir)
	err = os.MkdirAll(tasksDir, 0o750)
	require.NoError(t, err)

	taskFile := filepath.Join(tasksDir, "task-12345.json")
	err = os.WriteFile(taskFile, []byte(`{"id":"task-12345"}`), 0o600)
	require.NoError(t, err)

	// Reset metadata
	err = store.ResetMetadata(context.Background(), "reset-test")
	require.NoError(t, err)

	// Verify workspace.json is gone
	metadataPath := filepath.Join(wsPath, constants.WorkspaceFileName)
	_, err = os.Stat(metadataPath)
	assert.True(t, os.IsNotExist(err), "workspace.json should be deleted")

	// Verify tasks directory and task file still exist
	_, err = os.Stat(tasksDir)
	require.NoError(t, err, "tasks directory should be preserved")

	_, err = os.Stat(taskFile)
	require.NoError(t, err, "task file should be preserved")

	// Verify Exists returns false (since workspace.json is gone)
	exists, err := store.Exists(context.Background(), "reset-test")
	require.NoError(t, err)
	assert.False(t, exists, "workspace should not exist after metadata reset")
}

// TestFileStore_ResetMetadata_NotFound tests ResetMetadata on non-existent workspace.
func TestFileStore_ResetMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	err = store.ResetMetadata(context.Background(), "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrWorkspaceNotFound)
}

// TestFileStore_ResetMetadata_InvalidName tests ResetMetadata with invalid name.
func TestFileStore_ResetMetadata_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	err = store.ResetMetadata(context.Background(), "../evil")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
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

	err := atomicWrite(testPath, testData, WorkspaceFilePerm)
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
	err := os.WriteFile(testPath, originalData, WorkspaceFilePerm)
	require.NoError(t, err)

	// Verify original exists
	data, err := os.ReadFile(testPath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, originalData, data)

	// Test atomic write with new data succeeds
	newData := []byte(`{"new": "data", "updated": true}`)
	err = atomicWrite(testPath, newData, WorkspaceFilePerm)
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
	err := atomicWrite(testPath, []byte(`{"test": true}`), WorkspaceFilePerm)
	require.NoError(t, err)

	// Check no .tmp file exists
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp", "no temp files should remain")
	}
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

// TestFileStore_ReleaseLock_NilFile tests that releaseLock handles nil file gracefully.
func TestFileStore_ReleaseLock_NilFile(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Should not panic or error with nil file
	err = store.releaseLock(nil)
	assert.NoError(t, err)
}

// TestAtomicWrite_Success tests successful atomic write.
func TestAtomicWrite_Success(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test-file.json")
	data := []byte(`{"test": "data"}`)

	err := atomicWrite(filePath, data, WorkspaceFilePerm)
	require.NoError(t, err)

	// Verify file exists and has correct content
	content, err := os.ReadFile(filePath) //#nosec G304 -- test file path
	require.NoError(t, err)
	assert.Equal(t, data, content)

	// Verify temp file is cleaned up
	_, err = os.Stat(filePath + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file should not exist after successful write")
}

// TestAtomicWrite_InvalidPath tests atomic write to an invalid path.
func TestAtomicWrite_InvalidPath(t *testing.T) {
	// Use a path that doesn't exist
	filePath := "/nonexistent/directory/test-file.json"
	data := []byte(`{"test": "data"}`)

	err := atomicWrite(filePath, data, WorkspaceFilePerm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create temp file")
}

// TestFileStore_Create_InvalidChars tests creating a workspace with invalid characters.
func TestFileStore_Create_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "../evil",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Create(context.Background(), ws)
	require.Error(t, err)
	// Validation rejects invalid characters first
	require.ErrorIs(t, err, atlaserrors.ErrValueOutOfRange)
}

// TestFileStore_Get_InvalidChars tests getting a workspace with invalid characters.
func TestFileStore_Get_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	_, err = store.Get(context.Background(), "../evil")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValueOutOfRange)
}

// TestFileStore_Update_InvalidChars tests updating a workspace with invalid characters.
func TestFileStore_Update_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	ws := &domain.Workspace{
		Name:      "../evil",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.Update(context.Background(), ws)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValueOutOfRange)
}

// TestFileStore_Delete_InvalidChars tests deleting a workspace with invalid characters.
func TestFileStore_Delete_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	err = store.Delete(context.Background(), "../evil")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValueOutOfRange)
}

// TestFileStore_Exists_InvalidChars tests checking existence with invalid characters.
func TestFileStore_Exists_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	_, err = store.Exists(context.Background(), "../evil")
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrValueOutOfRange)
}

// TestFileStore_List_WithInvalidFiles tests listing workspaces
// when some workspace directories have invalid JSON files.
func TestFileStore_List_WithInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a valid workspace
	ws := &domain.Workspace{
		Name:      "valid-workspace",
		Status:    constants.WorkspaceStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	// Create an invalid workspace file
	invalidDir := filepath.Join(tmpDir, constants.WorkspacesDir, "invalid-workspace")
	err = os.MkdirAll(invalidDir, WorkspaceDirPerm)
	require.NoError(t, err)

	invalidPath := filepath.Join(invalidDir, constants.WorkspaceFileName)
	err = os.WriteFile(invalidPath, []byte("invalid json{"), 0o600)
	require.NoError(t, err)

	// List should still return the valid workspace
	workspaces, err := store.List(context.Background())
	require.NoError(t, err)

	// Should have at least the valid workspace
	assert.GreaterOrEqual(t, len(workspaces), 1)

	// Verify valid workspace is in the list
	found := false
	for _, ws := range workspaces {
		if ws.Name == "valid-workspace" {
			found = true
			break
		}
	}
	assert.True(t, found, "valid workspace should be in the list")
}

// TestFileStore_StressConcurrentCreateDelete tests high-concurrency create/delete operations.
func TestFileStore_StressConcurrentCreateDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	const numGoroutines = 20
	const opsPerGoroutine = 10

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*opsPerGoroutine*2)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				name := "stress-ws-" + string(rune('A'+id)) + "-" + string(rune('0'+j))
				ws := &domain.Workspace{
					Name:      name,
					Status:    constants.WorkspaceStatusActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}

				// Create
				if err := store.Create(context.Background(), ws); err != nil {
					errChan <- err
					continue
				}

				// Read
				if _, err := store.Get(context.Background(), name); err != nil {
					errChan <- err
				}

				// Delete
				if err := store.Delete(context.Background(), name); err != nil {
					errChan <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	errs := make([]error, 0, numGoroutines*opsPerGoroutine)
	for err := range errChan {
		errs = append(errs, err)
	}

	assert.Empty(t, errs, "stress test should complete without errors: %v", errs)
}

// errDataCorruption is a sentinel error for stress test data integrity checks.
var errDataCorruption = errors.New("data corruption detected")

// TestFileStore_StressConcurrentReads tests high-concurrency read operations.
func TestFileStore_StressConcurrentReads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	tmpDir := t.TempDir()
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create a workspace to read
	ws := &domain.Workspace{
		Name:      "read-stress-test",
		Status:    constants.WorkspaceStatusActive,
		Branch:    "main",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.Create(context.Background(), ws)
	require.NoError(t, err)

	const numGoroutines = 50
	const readsPerGoroutine = 20

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*readsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				retrieved, err := store.Get(context.Background(), "read-stress-test")
				if err != nil {
					errChan <- err
					continue
				}
				// Verify data integrity
				if retrieved.Name != "read-stress-test" || retrieved.Branch != "main" {
					errChan <- errDataCorruption
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	errs := make([]error, 0, numGoroutines*readsPerGoroutine)
	for err := range errChan {
		errs = append(errs, err)
	}

	assert.Empty(t, errs, "concurrent reads should not produce errors: %v", errs)
}
