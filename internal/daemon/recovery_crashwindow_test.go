package daemon

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTask creates a task entry in miniredis with the given status and priority.
// It also adds the task to the active set.
func seedTask(t *testing.T, mr *miniredis.Miniredis, taskID, status, priority string, retryCount int) {
	t.Helper()
	_, err := mr.SAdd("atlas:active", taskID)
	require.NoError(t, err)
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	mr.HSet(hashKey, "status", status)
	mr.HSet(hashKey, "priority", priority)
	if retryCount > 0 {
		mr.HSet(hashKey, "retry_count", strconv.Itoa(retryCount))
	}
}

// activeSetContains returns whether taskID is a member of atlas:active in miniredis.
func activeSetContains(t *testing.T, mr *miniredis.Miniredis, taskID string) bool {
	t.Helper()
	members, err := mr.SMembers("atlas:active")
	if err != nil {
		return false // key doesn't exist → not in set
	}
	for _, m := range members {
		if m == taskID {
			return true
		}
	}
	return false
}

// TestCrashWindow_TaskPoppedBeforeWorkerSlot verifies that a running task with no lock
// (popped before worker slot on shutdown) is re-queued on recovery (scenario 1).
func TestCrashWindow_TaskPoppedBeforeWorkerSlot(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0001-0000-0000-0000-000000000001"
	seedTask(t, mr, taskID, "running", "normal", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	// Task should be re-queued.
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, _ := cache.HashGet(ctx, d.redis, hashKey, "status")
	assert.Equal(t, "queued", status, "task should be re-queued")

	stats, _ := d.queue.Stats(ctx)
	assert.Positive(t, int(stats.Total), "task should be in the queue")
}

// TestCrashWindow_RunningWithoutLock is equivalent to the poppedBeforeWorkerSlot case
// but confirms the basic orphan path (scenario 2 — running without lock).
func TestCrashWindow_RunningWithoutLock(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0002-0000-0000-0000-000000000002"
	seedTask(t, mr, taskID, "running", "urgent", 1) // already retried once

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, _ := cache.HashGet(ctx, d.redis, hashKey, "status")
	retryStr, _ := cache.HashGet(ctx, d.redis, hashKey, "retry_count")

	assert.Equal(t, "queued", status)
	assert.Equal(t, "2", retryStr)
}

// TestCrashWindow_MaxRetriesExceeded verifies that a task exceeding maxRetryCount
// is marked failed with "max retries exceeded" (scenario 3).
func TestCrashWindow_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0003-0000-0000-0000-000000000003"
	seedTask(t, mr, taskID, "running", "normal", maxRetryCount)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, _ := cache.HashGet(ctx, d.redis, hashKey, "status")
	errMsg, _ := cache.HashGet(ctx, d.redis, hashKey, "error")

	assert.Equal(t, "failed", status)
	assert.Equal(t, "max retries exceeded", errMsg)

	// Should be removed from active set.
	assert.False(t, activeSetContains(t, mr, taskID), "exhausted task should be removed from active set")
}

// TestCrashWindow_CanceledQueuedTask verifies that canceled tasks are removed from
// the active set and not re-queued (scenario 4 — canceled queued task).
func TestCrashWindow_CanceledQueuedTask(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0004-0000-0000-0000-000000000004"
	seedTask(t, mr, taskID, "canceled", "normal", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	// Should be removed from active set.
	assert.False(t, activeSetContains(t, mr, taskID), "canceled task should be removed from active set")

	// Queue should be empty.
	stats, _ := d.queue.Stats(ctx)
	assert.Zero(t, int(stats.Total), "canceled task must not be re-queued")
}

// TestCrashWindow_AwaitingApprovalPreserved verifies awaiting_approval tasks are
// not re-queued (scenario 5 — awaiting approval preserved).
func TestCrashWindow_AwaitingApprovalPreserved(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0005-0000-0000-0000-000000000005"
	seedTask(t, mr, taskID, "awaiting_approval", "normal", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, _ := cache.HashGet(ctx, d.redis, hashKey, "status")
	assert.Equal(t, "awaiting_approval", status, "awaiting_approval status should be preserved")

	// Task should still be in the active set (removal happens on resume/approve).
	assert.True(t, activeSetContains(t, mr, taskID), "awaiting_approval task should remain in active set")

	// Queue should be empty.
	stats, _ := d.queue.Stats(ctx)
	assert.Zero(t, int(stats.Total), "awaiting_approval task must not be re-queued")
}

// TestCrashWindow_CompletedTaskCleanup verifies that completed tasks stale in the
// active set are removed during recovery.
func TestCrashWindow_CompletedTaskCleanup(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0006-0000-0000-0000-000000000006"
	seedTask(t, mr, taskID, "completed", "normal", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	assert.False(t, activeSetContains(t, mr, taskID), "completed task should be removed from active set")

	stats, _ := d.queue.Stats(ctx)
	assert.Zero(t, int(stats.Total), "completed task must not be re-queued")
}

// TestCrashWindow_AbandonedTaskCleanup verifies abandoned tasks are removed from active set.
func TestCrashWindow_AbandonedTaskCleanup(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0007-0000-0000-0000-000000000007"
	seedTask(t, mr, taskID, "abandoned", "low", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	assert.False(t, activeSetContains(t, mr, taskID), "abandoned task should be removed from active set")
}

// TestCrashWindow_FailedTaskCleanup verifies failed tasks are removed from active set.
func TestCrashWindow_FailedTaskCleanup(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0008-0000-0000-0000-000000000008"
	seedTask(t, mr, taskID, "failed", "normal", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	assert.False(t, activeSetContains(t, mr, taskID), "failed task should be removed from active set")
}

// TestCrashWindow_RunningWithActiveLockNotTouched verifies that a running task with
// a live lock is not re-queued (daemon kill scenario where lock still held).
func TestCrashWindow_RunningWithActiveLockNotTouched(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "aaaa0009-0000-0000-0000-000000000009"
	seedTask(t, mr, taskID, "running", "urgent", 0)

	// Simulate live lock.
	lockKey := fmt.Sprintf("atlas:lock:task:%s", taskID)
	require.NoError(t, mr.Set(lockKey, "1"))

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, _ := cache.HashGet(ctx, d.redis, hashKey, "status")
	assert.Equal(t, "running", status, "task with live lock should not be touched")

	stats, _ := d.queue.Stats(ctx)
	assert.Zero(t, int(stats.Total), "task with live lock must not be re-queued")
}

// TestCrashWindow_VerboseRecoveryEvents verifies that recovery decisions are stored
// in daemon memory for doctor/status output (Q4 verbose).
func TestCrashWindow_VerboseRecoveryEvents(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// Seed two tasks with different recovery outcomes.
	orphanID := "aaaa000a-0000-0000-0000-00000000000a"
	canceledID := "aaaa000b-0000-0000-0000-00000000000b"
	seedTask(t, mr, orphanID, "running", "normal", 0)
	seedTask(t, mr, canceledID, "canceled", "low", 0)

	ctx := context.Background()
	require.NoError(t, d.RecoverOrphanedTasks(ctx))

	events := d.getRecoveryEvents()
	require.NotEmpty(t, events, "recovery events should be stored")

	decisions := make(map[string]string)
	for _, ev := range events {
		decisions[ev.TaskID] = ev.Decision
	}

	assert.Equal(t, "requeue", decisions[orphanID], "orphaned running task should be re-queued")
	assert.Equal(t, "remove_terminal", decisions[canceledID], "canceled task should be removed")
}

// TestCrashWindow_MalformedTaskIDSkipped verifies non-UUID active-set entries are skipped.
func TestCrashWindow_MalformedTaskIDSkipped(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// Inject a non-UUID entry directly into the active set.
	_, err := mr.SAdd("atlas:active", "not-a-uuid")
	require.NoError(t, err)

	ctx := context.Background()
	// Should not return an error even though the entry is malformed.
	err = d.RecoverOrphanedTasks(ctx)
	assert.NoError(t, err)
}
