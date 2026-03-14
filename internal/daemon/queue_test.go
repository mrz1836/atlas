package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestQueue spins up a miniredis instance and returns a RedisQueue backed by it.
func newTestQueue(t *testing.T) (*RedisQueue, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())
	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	q := NewRedisQueue(client, "atlas:")
	return q, func() { client.Close() }
}

func TestQueueSubmitAndPop(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Submit three tasks in order; verify they pop FIFO.
	require.NoError(t, q.Submit(ctx, "task-1", PriorityNormal))
	time.Sleep(time.Millisecond) // ensure distinct nanosecond scores
	require.NoError(t, q.Submit(ctx, "task-2", PriorityNormal))
	time.Sleep(time.Millisecond)
	require.NoError(t, q.Submit(ctx, "task-3", PriorityNormal))

	got1, p1, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-1", got1)
	assert.Equal(t, PriorityNormal, p1)

	got2, p2, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-2", got2)
	assert.Equal(t, PriorityNormal, p2)

	got3, p3, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-3", got3)
	assert.Equal(t, PriorityNormal, p3)
}

func TestQueuePriorityOrder(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	// Submit one task at each priority; urgent must pop first.
	require.NoError(t, q.Submit(ctx, "low-task", PriorityLow))
	require.NoError(t, q.Submit(ctx, "normal-task", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "urgent-task", PriorityUrgent))

	got1, p1, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "urgent-task", got1)
	assert.Equal(t, PriorityUrgent, p1)

	got2, p2, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "normal-task", got2)
	assert.Equal(t, PriorityNormal, p2)

	got3, p3, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "low-task", got3)
	assert.Equal(t, PriorityLow, p3)
}

func TestQueueRemove(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, q.Submit(ctx, "task-A", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "task-B", PriorityNormal))

	// Remove task-A
	require.NoError(t, q.Remove(ctx, "task-A"))

	// Only task-B should remain.
	got, p, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-B", got)
	assert.Equal(t, PriorityNormal, p)

	// Queue should now be empty.
	empty, pEmpty, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.Empty(t, pEmpty)
}

func TestQueueStats(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, q.Submit(ctx, "u1", PriorityUrgent))
	require.NoError(t, q.Submit(ctx, "n1", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "n2", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "l1", PriorityLow))
	require.NoError(t, q.Submit(ctx, "l2", PriorityLow))
	require.NoError(t, q.Submit(ctx, "l3", PriorityLow))

	stats, err := q.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Urgent)
	assert.Equal(t, int64(2), stats.Normal)
	assert.Equal(t, int64(3), stats.Low)
	assert.Equal(t, int64(6), stats.Total)
}

func TestQueueClear(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, q.Submit(ctx, "u1", PriorityUrgent))
	require.NoError(t, q.Submit(ctx, "n1", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "l1", PriorityLow))

	// Clear only the normal queue.
	normal := PriorityNormal
	require.NoError(t, q.Clear(ctx, &normal))

	stats, err := q.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Urgent)
	assert.Equal(t, int64(0), stats.Normal)
	assert.Equal(t, int64(1), stats.Low)

	// Clear all queues.
	require.NoError(t, q.Clear(ctx, nil))

	stats, err = q.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Total)
}

func TestQueueEmpty(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	got, p, err := q.Pop(ctx)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, p)
}

func TestQueueList(t *testing.T) {
	t.Parallel()
	q, cleanup := newTestQueue(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, q.Submit(ctx, "u1", PriorityUrgent))
	require.NoError(t, q.Submit(ctx, "n1", PriorityNormal))
	require.NoError(t, q.Submit(ctx, "l1", PriorityLow))

	// List all
	all, err := q.List(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// List filtered by priority
	urgent := PriorityUrgent
	entries, err := q.List(ctx, &urgent)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "u1", entries[0].TaskID)
	assert.Equal(t, PriorityUrgent, entries[0].Priority)
}
