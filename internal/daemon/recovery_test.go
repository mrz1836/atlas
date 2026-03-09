package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// newTestDaemonWithRedis creates a Daemon backed by a miniredis instance.
func newTestDaemonWithRedis(t *testing.T) (*Daemon, *miniredis.Miniredis, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	tmp := t.TempDir()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:           true,
			SocketPath:        filepath.Join(tmp, "daemon.sock"),
			PIDFile:           filepath.Join(tmp, "daemon.pid"),
			MaxParallelTasks:  3,
			ShutdownTimeout:   2 * time.Second,
			HeartbeatInterval: 10 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         mr.Addr(),
			KeyPrefix:    "atlas:",
			PoolSize:     5,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
	}

	ctx := context.Background()
	redisCfg := RedisConfig{
		Addr:         mr.Addr(),
		DB:           0,
		PoolSize:     5,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	client, err := NewRedisClient(ctx, redisCfg)
	require.NoError(t, err)

	logger := zerolog.Nop()
	d := New(cfg, logger)
	d.redis = client
	d.startedAt = time.Now()
	d.queue = NewRedisQueue(client, "atlas:")
	d.events = NewEventPublisher(client, "")

	return d, mr, func() { client.Close() }
}

// seedOrphanedTask creates a fake running task in miniredis without a lock key.
func seedOrphanedTask(t *testing.T, mr *miniredis.Miniredis, taskID string, retryCount int) {
	t.Helper()
	// Add to active set.
	_, sAddErr := mr.SAdd(activeSetKey, taskID)
	require.NoError(t, sAddErr)
	// Set task hash fields.
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	mr.HSet(hashKey, "status", "running")
	mr.HSet(hashKey, "retry_count", strconv.Itoa(retryCount))
	mr.HSet(hashKey, "priority", "normal")
}

// TestRecoverOrphanedTasks verifies that an orphaned task (running, no lock) is re-queued.
func TestRecoverOrphanedTasks(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "task-abc123"
	seedOrphanedTask(t, mr, taskID, 0)

	ctx := context.Background()
	err := d.RecoverOrphanedTasks(ctx)
	require.NoError(t, err)

	// Status should have been reset to "queued".
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, hErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, hErr)
	assert.Equal(t, "queued", status)

	// retry_count should be 1.
	retryCnt, hErr := cache.HashGet(ctx, d.redis, hashKey, "retry_count")
	require.NoError(t, hErr)
	assert.Equal(t, "1", retryCnt)

	// Task should be in the queue.
	stats, sErr := d.queue.Stats(ctx)
	require.NoError(t, sErr)
	assert.Positive(t, int(stats.Total), "task should have been re-submitted to the queue")
}

// TestRecoverMaxRetries verifies that a task at max retries is marked failed, not re-queued.
func TestRecoverMaxRetries(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "task-maxretry"
	seedOrphanedTask(t, mr, taskID, maxRetryCount) // already at limit

	ctx := context.Background()
	err := d.RecoverOrphanedTasks(ctx)
	require.NoError(t, err)

	// Status should be "failed".
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, hErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, hErr)
	assert.Equal(t, "failed", status)

	// Error field should be set.
	errField, hErr := cache.HashGet(ctx, d.redis, hashKey, "error")
	require.NoError(t, hErr)
	assert.Equal(t, "max retries exceeded", errField)

	// Queue should be empty (task was NOT re-submitted).
	stats, sErr := d.queue.Stats(ctx)
	require.NoError(t, sErr)
	assert.Zero(t, stats.Total, "exhausted task must not be re-queued")
}

// TestRecoverOrphanedTasks_WithLock verifies that a task with an active lock is not touched.
func TestRecoverOrphanedTasks_WithLock(t *testing.T) {
	t.Parallel()
	d, mr, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	taskID := "task-with-lock"
	seedOrphanedTask(t, mr, taskID, 0)

	// Add an active lock — this task is still being worked on.
	lockKey := fmt.Sprintf("atlas:lock:task:%s", taskID)
	require.NoError(t, mr.Set(lockKey, "1"))

	ctx := context.Background()
	err := d.RecoverOrphanedTasks(ctx)
	require.NoError(t, err)

	// Status should still be "running" (untouched).
	hashKey := fmt.Sprintf("atlas:task:%s", taskID)
	status, hErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, hErr)
	assert.Equal(t, "running", status)
}

// TestRecoverOrphanedTasks_Empty verifies no error is returned for an empty active set.
func TestRecoverOrphanedTasks_Empty(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	err := d.RecoverOrphanedTasks(ctx)
	require.NoError(t, err)
}
