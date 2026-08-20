package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/lifecycle"
)

// worktreeLockHeld reports whether the Redis worktree lock for repoPath exists.
func worktreeLockHeld(t *testing.T, client *cache.Client, repoPath string) bool {
	t.Helper()
	key := lifecycle.WorktreeLockRedisKey("atlas:", repoPath)
	exists, err := cache.Exists(context.Background(), client, key)
	require.NoError(t, err)
	return exists
}

// submitReq marshals a task.submit request with the given repo path.
func submitReq(t *testing.T, repoPath string) json.RawMessage {
	t.Helper()
	params, err := json.Marshal(TaskSubmitRequest{
		Description: "rollback test task",
		Template:    "task",
		RepoPath:    repoPath,
	})
	require.NoError(t, err)
	return params
}

// TestSubmit_HashWriteFailure_ReleasesWorktreeLock is the regression guard for the
// worktree-lock leak: when the task-hash write (Step 1) fails AFTER the worktree
// lock (Step 0) was acquired, the deferred transaction rollback must release the
// lock. Before the submitTxn fix, the hash-failure branch returned without
// rolling back, leaking the lock until its TTL expired (~45m).
func TestSubmit_HashWriteFailure_ReleasesWorktreeLock(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	// Make the generated task ID deterministic so we can poison its hash key.
	const fixedID = "deadbeef-dead-beef-dead-beefdeadbeef"
	d.newTaskID = func() string { return fixedID }

	// Poison the task-hash key with a string so the HMSET write fails (WRONGTYPE),
	// while the worktree-lock SET NX (a different key) still succeeds.
	mr.Set("atlas:task:"+fixedID, "not-a-hash") //nolint:errcheck,gosec // deliberate fault injection

	repoPath := t.TempDir()

	_, err := d.handleTaskSubmit(context.Background(), submitReq(t, repoPath))
	require.Error(t, err, "submit must fail when the task-hash write fails")
	assert.Contains(t, err.Error(), "store task hash", "failure must be the hash write step")

	assert.False(t, worktreeLockHeld(t, client, repoPath),
		"worktree lock must be released after a failed hash write (no leak)")
}

// TestSubmit_HashWriteFailure_LockReacquirable proves the released lock is truly
// free: a subsequent, clean submit for the same worktree succeeds.
func TestSubmit_HashWriteFailure_LockReacquirable(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	const failID = "11111111-1111-1111-1111-111111111111"
	d.newTaskID = func() string { return failID }
	mr.Set("atlas:task:"+failID, "not-a-hash") //nolint:errcheck,gosec // deliberate fault injection

	repoPath := t.TempDir()

	// First submit fails at the hash write and must release the lock.
	_, err := d.handleTaskSubmit(context.Background(), submitReq(t, repoPath))
	require.Error(t, err)

	// Second submit (clean ID, no poison) for the SAME worktree must succeed,
	// which is only possible if the lock was actually released.
	d.newTaskID = func() string { return "22222222-2222-2222-2222-222222222222" }
	resp, err := d.handleTaskSubmit(context.Background(), submitReq(t, repoPath))
	require.NoError(t, err, "a clean submit for the same worktree must succeed after the lock was released")
	_, ok := resp.(TaskSubmitResponse)
	assert.True(t, ok)
	assert.True(t, worktreeLockHeld(t, client, repoPath), "the clean submit should now hold the lock")
}

// TestSubmit_MidStepFailure_ReleasesWorktreeLock verifies the lock is released
// regardless of WHICH later step fails (tasks-set here), exercising the uniform
// deferred rollback end-to-end with a repo path.
func TestSubmit_MidStepFailure_ReleasesWorktreeLock(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	d, client := newTestDaemonForLock(t, mr)
	defer client.Close()

	// Poison the tasks set so Step 2 (SADD atlas:tasks) fails with WRONGTYPE.
	mr.Set("atlas:tasks", "not-a-set") //nolint:errcheck,gosec // deliberate fault injection

	repoPath := t.TempDir()

	_, err := d.handleTaskSubmit(context.Background(), submitReq(t, repoPath))
	require.Error(t, err, "submit must fail when the tasks-set write fails")
	assert.Contains(t, err.Error(), "track in tasks set")

	assert.False(t, worktreeLockHeld(t, client, repoPath),
		"worktree lock must be released after a failed tasks-set write")
}
