//go:build integration

package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// TestIntegration_WorkspaceLifecycle tests the full workspace lifecycle:
// create -> update -> list -> close -> destroy
func TestIntegration_WorkspaceLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create store
	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// 1. Create workspace
	ws := &domain.Workspace{
		Name:         "integration-test-ws",
		Status:       constants.WorkspaceStatusActive,
		Branch:       "feature/test-branch",
		WorktreePath: tmpDir + "/worktree",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     map[string]any{"test": "value"},
	}

	err = store.Create(ctx, ws)
	require.NoError(t, err, "should create workspace")

	// 2. Verify workspace exists
	exists, err := store.Exists(ctx, "integration-test-ws")
	require.NoError(t, err)
	assert.True(t, exists, "workspace should exist after creation")

	// 3. Get and verify workspace
	retrieved, err := store.Get(ctx, "integration-test-ws")
	require.NoError(t, err)
	assert.Equal(t, ws.Name, retrieved.Name)
	assert.Equal(t, ws.Branch, retrieved.Branch)
	assert.Equal(t, "value", retrieved.Metadata["test"])

	// 4. Update workspace
	retrieved.Status = constants.WorkspaceStatusClosed
	retrieved.Branch = "feature/updated-branch"
	err = store.Update(ctx, retrieved)
	require.NoError(t, err)

	// 5. Verify update
	updated, err := store.Get(ctx, "integration-test-ws")
	require.NoError(t, err)
	assert.Equal(t, constants.WorkspaceStatusClosed, updated.Status)
	assert.Equal(t, "feature/updated-branch", updated.Branch)

	// 6. List workspaces
	workspaces, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 1)
	assert.Equal(t, "integration-test-ws", workspaces[0].Name)

	// 7. Delete workspace
	err = store.Delete(ctx, "integration-test-ws")
	require.NoError(t, err)

	// 8. Verify deletion
	exists, err = store.Exists(ctx, "integration-test-ws")
	require.NoError(t, err)
	assert.False(t, exists, "workspace should not exist after deletion")
}

// TestIntegration_MultipleWorkspaces tests managing multiple workspaces concurrently.
func TestIntegration_MultipleWorkspaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	store, err := NewFileStore(tmpDir)
	require.NoError(t, err)

	// Create multiple workspaces
	workspaceNames := []string{"ws-alpha", "ws-beta", "ws-gamma", "ws-delta"}
	for _, name := range workspaceNames {
		ws := &domain.Workspace{
			Name:      name,
			Status:    constants.WorkspaceStatusActive,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := store.Create(ctx, ws)
		require.NoError(t, err, "should create workspace %s", name)
	}

	// List and verify all exist
	workspaces, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 4)

	// Delete some workspaces
	err = store.Delete(ctx, "ws-beta")
	require.NoError(t, err)
	err = store.Delete(ctx, "ws-delta")
	require.NoError(t, err)

	// Verify remaining
	workspaces, err = store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 2)

	// Verify correct ones remain
	names := make([]string, len(workspaces))
	for i, ws := range workspaces {
		names[i] = ws.Name
	}
	assert.Contains(t, names, "ws-alpha")
	assert.Contains(t, names, "ws-gamma")
}
