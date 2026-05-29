package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/lifecycle"
)

// seedActiveTaskForReconcile adds a task to the Redis active set and sets its hash fields.
func seedActiveTaskForReconcile(t *testing.T, mr *miniredis.Miniredis, prefix, taskID, status, wsName string) {
	t.Helper()
	_, err := mr.SAdd(prefix+"active", taskID)
	require.NoError(t, err)
	hashKey := fmt.Sprintf("%stask:%s", prefix, taskID)
	mr.HSet(hashKey, "status", status)
	mr.HSet(hashKey, "priority", "normal")
	if wsName != "" {
		mr.HSet(hashKey, "workspace", wsName)
	}
}

// withWorkspaces sets a static workspace loader on the daemon and resets it via cleanup.
func withWorkspaces(d *Daemon, entries []*workspaceEntry) {
	d.workspaceLoader = func(_ string) ([]*workspaceEntry, error) {
		return entries, nil
	}
}

// TestReconcile_NoActiveTasks ensures a clean system produces no drift.
func TestReconcile_NoActiveTasks(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	withWorkspaces(d, nil)

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total)
	assert.Contains(t, resp.Summary, "no drift detected")
}

// TestReconcile_RedisOnly detects a task in Redis with no workspace on disk.
func TestReconcile_RedisOnly(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	const taskID = "11111111-1111-1111-1111-111111111111"
	const wsName = "ws-ghost"

	seedActiveTaskForReconcile(t, mr, "atlas:", taskID, lifecycle.StateDaemonRunning, wsName)

	// No workspaces on disk.
	withWorkspaces(d, nil)

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)

	require.Equal(t, 1, resp.Total, "expected 1 redis-only drift item")
	item := resp.DriftItems[0]
	assert.Equal(t, DriftTypeRedisOnly, item.Type)
	assert.Equal(t, taskID, item.TaskID)
	assert.Equal(t, wsName, item.Workspace)
	assert.Equal(t, lifecycle.LabelRunning, item.RedisStatus)
	assert.NotEmpty(t, item.SuggestedAction)
}

// TestReconcile_FileOnly detects an active workspace on disk not in Redis.
func TestReconcile_FileOnly(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// No tasks in Redis.
	withWorkspaces(d, []*workspaceEntry{
		{Name: "ws-orphan", Status: constants.WorkspaceStatusActive},
	})

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)

	require.Equal(t, 1, resp.Total, "expected 1 file-only drift item")
	item := resp.DriftItems[0]
	assert.Equal(t, DriftTypeFileOnly, item.Type)
	assert.Equal(t, "ws-orphan", item.Workspace)
	assert.Equal(t, string(constants.WorkspaceStatusActive), item.FileStatus)
	assert.NotEmpty(t, item.SuggestedAction)
}

// TestReconcile_StatusMismatch detects a terminal Redis task with an active workspace on disk.
func TestReconcile_StatusMismatch(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	const taskID = "22222222-2222-2222-2222-222222222222"
	const wsName = "ws-stale"

	// Redis says the task is completed (terminal).
	seedActiveTaskForReconcile(t, mr, "atlas:", taskID, lifecycle.StateDaemonCompleted, wsName)

	// But the workspace is still "active" on disk.
	withWorkspaces(d, []*workspaceEntry{
		{Name: wsName, Status: constants.WorkspaceStatusActive},
	})

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)

	var mismatch *DriftItem
	for i := range resp.DriftItems {
		if resp.DriftItems[i].Type == DriftTypeStatusMismatch {
			mismatch = &resp.DriftItems[i]
			break
		}
	}
	require.NotNil(t, mismatch, "expected a status_mismatch drift item; got items: %v", resp.DriftItems)
	assert.Equal(t, taskID, mismatch.TaskID)
	assert.Equal(t, wsName, mismatch.Workspace)
	assert.Equal(t, lifecycle.LabelCompleted, mismatch.RedisStatus)
	assert.Contains(t, mismatch.SuggestedAction, "atlas cleanup")
}

// TestReconcile_MultipleTypes verifies that multiple drift types can co-exist.
func TestReconcile_MultipleTypes(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	const redisOnlyTask = "33333333-3333-3333-3333-333333333333"
	seedActiveTaskForReconcile(t, mr, "atlas:", redisOnlyTask, lifecycle.StateDaemonRunning, "ws-redis-only")

	withWorkspaces(d, []*workspaceEntry{
		{Name: "ws-file-only", Status: constants.WorkspaceStatusActive},
	})

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)

	assert.Equal(t, 2, resp.Total)
	assert.Contains(t, resp.Summary, "2 drift item(s) detected")

	types := make(map[string]bool)
	for _, item := range resp.DriftItems {
		types[item.Type] = true
	}
	assert.True(t, types[DriftTypeRedisOnly], "expected redis_only drift item")
	assert.True(t, types[DriftTypeFileOnly], "expected file_only drift item")
}

// TestReconcile_CleanSystem_InSyncWorkspace verifies no drift when Redis and filesystem agree.
func TestReconcile_CleanSystem_InSyncWorkspace(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	const taskID = "44444444-4444-4444-4444-444444444444"
	const wsName = "ws-in-sync"

	seedActiveTaskForReconcile(t, mr, "atlas:", taskID, lifecycle.StateDaemonRunning, wsName)
	withWorkspaces(d, []*workspaceEntry{
		{Name: wsName, Status: constants.WorkspaceStatusActive},
	})

	resp, err := d.Reconcile(context.Background(), ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Total, "no drift expected for in-sync workspace; items: %v", resp.DriftItems)
	assert.Contains(t, resp.Summary, "no drift detected")
}

// TestHandleDaemonReconcile_ViaRPC verifies the handler dispatches correctly.
func TestHandleDaemonReconcile_ViaRPC(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	withWorkspaces(d, nil)

	raw, err := json.Marshal(ReconcileRequest{AtlasHome: t.TempDir()})
	require.NoError(t, err)

	result, err := d.handleDaemonReconcile(context.Background(), raw)
	require.NoError(t, err)

	resp, ok := result.(ReconcileResponse)
	require.True(t, ok, "handler must return ReconcileResponse")
	assert.Equal(t, 0, resp.Total)
}

// TestHandleDaemonReconcile_NullParams ensures null/empty params are handled gracefully.
func TestHandleDaemonReconcile_NullParams(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	withWorkspaces(d, nil)

	result, err := d.handleDaemonReconcile(context.Background(), json.RawMessage(`null`))
	require.NoError(t, err)
	_, ok := result.(ReconcileResponse)
	assert.True(t, ok)
}
