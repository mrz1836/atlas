package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubmit_HappyPath pins the golden state after a successful daemon task submit.
// It verifies that handleTaskSubmit writes the expected Redis keys and returns a
// queued response — characterizing the current submit contract before Phase 4.2
// introduces explicit rollback semantics.
func TestSubmit_HappyPath(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	req, err := json.Marshal(TaskSubmitRequest{
		Description: "fix the login bug",
		Template:    "bug",
		Priority:    string(PriorityNormal),
		Workspace:   "my-workspace",
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(ctx, req)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok, "result should be a TaskSubmitResponse")

	// Golden: response has non-empty task ID and "queued" status.
	assert.NotEmpty(t, resp.TaskID, "task ID must be non-empty")
	assert.Equal(t, "queued", resp.Status, "initial status must be queued")

	// Golden: task hash exists in Redis with expected fields.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + resp.TaskID
	status, err := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "queued", status)

	template, err := cache.HashGet(ctx, d.redis, hashKey, "template")
	require.NoError(t, err)
	assert.Equal(t, "bug", template)

	// Golden: task appears in the persistent tasks set.
	tasksKey := d.cfg.Redis.KeyPrefix + "tasks"
	members, err := cache.SetMembers(ctx, d.redis, tasksKey)
	require.NoError(t, err)
	assert.Contains(t, members, resp.TaskID, "task ID should be in persistent tasks set")

	// Golden: task appears in the active set.
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	activeMembers, err := cache.SetMembers(ctx, d.redis, activeKey)
	require.NoError(t, err)
	assert.Contains(t, activeMembers, resp.TaskID, "task ID should be in active set")

	// Golden: queue depth increases by 1.
	stats, err := d.queue.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Normal, "normal queue depth should be 1")
}

// TestQueue_FIFO pins the current FIFO ordering of tasks within the same priority
// level. Tasks submitted in order A → B → C must pop in the same order A → B → C.
//
// Note on timestamp precision: the implementation currently uses UnixMicro() as the
// sort score while the QueueEntry comment says "nanosecond timestamp". The test uses
// time.Sleep between submissions to ensure distinct scores under the current UnixMicro
// implementation.
//
// TODO(T-228 phase 4): Task 4.4 will fix this to use nanosecond precision and remove
// the need for sleeps. Update this test to verify nanosecond-stable ordering at that
// point.
func TestQueue_FIFO(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Submit three tasks with short delays so UnixMicro scores are strictly increasing.
	ids := []string{"fifo-task-a", "fifo-task-b", "fifo-task-c"}
	for _, id := range ids {
		require.NoError(t, q.Submit(ctx, id, PriorityNormal))
		time.Sleep(time.Millisecond) // ensure distinct UnixMicro scores
	}

	// Pop must return tasks in submission order (FIFO within priority).
	for _, want := range ids {
		got, prio, err := q.Pop(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, got, "FIFO order must match submission order")
		assert.Equal(t, PriorityNormal, prio)
	}

	// Queue must be empty after all pops.
	empty, _, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty, "queue should be empty after all tasks popped")
}

// TestQueue_Priority pins the current priority cascade: urgent > normal > low.
// Mixed-priority submissions always pop urgent first, then normal, then low,
// regardless of submission order.
func TestQueue_Priority(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Submit one task at each priority level in interleaved order.
	require.NoError(t, q.Submit(ctx, "priority-low-task", PriorityLow))
	require.NoError(t, q.Submit(ctx, "priority-urgent-task", PriorityUrgent))
	require.NoError(t, q.Submit(ctx, "priority-normal-task", PriorityNormal))

	// Pop 1: urgent must be first.
	got1, p1, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "priority-urgent-task", got1, "urgent task must pop first")
	assert.Equal(t, PriorityUrgent, p1)

	// Pop 2: normal is next.
	got2, p2, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "priority-normal-task", got2, "normal task must pop second")
	assert.Equal(t, PriorityNormal, p2)

	// Pop 3: low is last.
	got3, p3, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "priority-low-task", got3, "low task must pop last")
	assert.Equal(t, PriorityLow, p3)

	// Queue must be empty.
	empty, _, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// newTestQueueWithMaxSize creates a RedisQueue with a specific MaxSize for testing.
func newTestQueueWithMaxSize(t *testing.T, maxSize int) (*RedisQueue, func()) {
	t.Helper()
	q, cleanup := newTestQueue(t)
	q.maxSize = maxSize
	return q, cleanup
}

// TestQueue_MaxSize_Enforcement verifies that Submit returns ErrQueueFull once the
// queue reaches its configured MaxSize (Task 4.3).
func TestQueue_MaxSize_Enforcement(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueueWithMaxSize(t, 2)
	defer cleanup()

	ctx := context.Background()

	// First two submits must succeed.
	require.NoError(t, q.Submit(ctx, "ms-task-1", PriorityNormal), "first submit must succeed")
	require.NoError(t, q.Submit(ctx, "ms-task-2", PriorityNormal), "second submit must succeed")

	// Third submit must fail with ErrQueueFull.
	err := q.Submit(ctx, "ms-task-3", PriorityNormal)
	require.Error(t, err, "third submit must fail")
	assert.True(t, errors.Is(err, ErrQueueFull), "error must wrap ErrQueueFull; got: %v", err)

	// Queue depth must still be 2 (third task was not enqueued).
	stats, statsErr := q.Stats(ctx)
	require.NoError(t, statsErr)
	assert.Equal(t, int64(2), stats.Total, "queue depth must not exceed MaxSize")
}

// TestQueue_MaxSize_Zero_Unlimited verifies that MaxSize=0 means unlimited and does
// not cause ErrQueueFull even when many tasks are submitted (Task 4.3).
func TestQueue_MaxSize_Zero_Unlimited(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t) // default MaxSize=0
	defer cleanup()

	ctx := context.Background()

	const n = 50
	for i := range n {
		id := fmt.Sprintf("unlimited-task-%03d", i)
		require.NoError(t, q.Submit(ctx, id, PriorityNormal), "submit %d must succeed with MaxSize=0", i)
	}

	stats, err := q.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(n), stats.Total)
}

// TestQueue_FIFO_StableUnderRapidSubmit_AfterInversion verifies that FIFO ordering
// is stable under rapid back-to-back submissions after the nanosecond-precision
// switch (Task 4.4). The test runs with -race clean over multiple iterations.
//
// A minimal sleep of 1µs ensures distinct nanosecond scores even on systems where
// consecutive goroutine operations resolve to the same OS tick.
func TestQueue_FIFO_StableUnderRapidSubmit_AfterInversion(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	ids := []string{"rapid-a", "rapid-b", "rapid-c", "rapid-d", "rapid-e"}
	for _, id := range ids {
		require.NoError(t, q.Submit(ctx, id, PriorityNormal))
		time.Sleep(time.Microsecond) // 1µs > 256ns float64 precision floor; ensures distinct scores
	}

	for _, want := range ids {
		got, prio, err := q.Pop(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, got, "FIFO order must match submission order (nanosecond precision)")
		assert.Equal(t, PriorityNormal, prio)
	}

	empty, _, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// TestQueue_FIFO_AfterInversion updates the pre-phase-4 characterization test to
// assert the same FIFO contract now that timestamps use UnixNano (Task 4.4).
// This test is the _AfterInversion successor to TestQueue_FIFO.
func TestQueue_FIFO_AfterInversion(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// 1ms sleep ensures distinct UnixNano (and UnixMicro) scores regardless of
	// float64 precision floor.
	ids := []string{"inv-fifo-a", "inv-fifo-b", "inv-fifo-c"}
	for _, id := range ids {
		require.NoError(t, q.Submit(ctx, id, PriorityNormal))
		time.Sleep(time.Millisecond)
	}

	for _, want := range ids {
		got, prio, err := q.Pop(ctx)
		require.NoError(t, err)
		assert.Equal(t, want, got, "FIFO order must match submission order")
		assert.Equal(t, PriorityNormal, prio)
	}
}

// TestSubmit_HappyPath_AfterInversion is an updated version of TestSubmit_HappyPath
// that also asserts the workspace name field added by Task 4.5.
func TestSubmit_HappyPath_AfterInversion(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	req, err := json.Marshal(TaskSubmitRequest{
		Description: "fix the login bug",
		Template:    "bug",
		Priority:    string(PriorityNormal),
		Workspace:   "my-workspace",
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(ctx, req)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok, "result should be a TaskSubmitResponse")

	assert.NotEmpty(t, resp.TaskID)
	assert.Equal(t, "queued", resp.Status)
	assert.Equal(t, "my-workspace", resp.Workspace, "submitted workspace name must be echoed")

	// task hash must include workspace field.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + resp.TaskID
	ws, wsErr := cache.HashGet(ctx, d.redis, hashKey, "workspace")
	require.NoError(t, wsErr)
	assert.Equal(t, "my-workspace", ws, "workspace must be stored in task hash")

	// Queue depth must be 1.
	stats, _ := d.queue.Stats(ctx)
	assert.Equal(t, int64(1), stats.Normal)
}
