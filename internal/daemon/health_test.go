package daemon

import (
	"context"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRefreshHeartbeat verifies that refreshHeartbeat writes the heartbeat key and state hash.
func TestRefreshHeartbeat(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.startedAt = time.Now().Add(-5 * time.Second)

	ctx := context.Background()
	d.refreshHeartbeat(ctx)

	// Heartbeat key should exist with a non-empty timestamp.
	val, err := cache.Get(ctx, d.redis, heartbeatKey)
	require.NoError(t, err)
	assert.NotEmpty(t, val)

	// State hash should have pid, uptime, version, status.
	pid, err := cache.HashGet(ctx, d.redis, daemonStateKey, "pid")
	require.NoError(t, err)
	assert.NotEmpty(t, pid)

	status, err := cache.HashGet(ctx, d.redis, daemonStateKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "running", status)

	version, err := cache.HashGet(ctx, d.redis, daemonStateKey, "version")
	require.NoError(t, err)
	assert.Equal(t, daemonVersion, version)

	uptime, err := cache.HashGet(ctx, d.redis, daemonStateKey, "uptime")
	require.NoError(t, err)
	assert.NotEmpty(t, uptime)
}

// TestRefreshHeartbeat_NilRedis verifies that refreshHeartbeat is a no-op when redis is nil.
func TestRefreshHeartbeat_NilRedis(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.redis = nil

	// Should not panic or error.
	assert.NotPanics(t, func() {
		d.refreshHeartbeat(context.Background())
	})
}

// TestDaemonHealth verifies that Health returns a populated response.
func TestDaemonHealth(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.startedAt = time.Now().Add(-10 * time.Second)

	ctx := context.Background()
	resp, err := d.Health(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Positive(t, resp.PID)
	assert.True(t, resp.RedisAlive, "redis should be alive (using miniredis)")
	assert.NotEmpty(t, resp.StartedAt)
	assert.NotEmpty(t, resp.Uptime)
	assert.Equal(t, 3, resp.Workers) // from newTestDaemonWithRedis MaxParallelTasks=3
}

// TestDaemonHealth_WithQueueDepth verifies that QueueDepth is populated from the queue.
func TestDaemonHealth_WithQueueDepth(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit some tasks.
	require.NoError(t, d.queue.Submit(ctx, "task-a", PriorityNormal))
	require.NoError(t, d.queue.Submit(ctx, "task-b", PriorityUrgent))

	resp, err := d.Health(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.QueueDepth)
}

// TestDaemonHealth_NilQueue verifies that Health works even when queue is nil.
func TestDaemonHealth_NilQueue(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.queue = nil
	d.startedAt = time.Now()

	resp, err := d.Health(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, resp.QueueDepth)
}

// TestDaemonHealth_NilRedis verifies that Health reports RedisAlive=false when redis is nil.
func TestDaemonHealth_NilRedis(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.redis = nil
	d.queue = nil
	d.startedAt = time.Now()

	resp, err := d.Health(context.Background())
	require.NoError(t, err)
	assert.False(t, resp.RedisAlive)
}

// TestStartHeartbeat verifies the heartbeat goroutine runs and refreshes the key.
func TestStartHeartbeat(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.startedAt = time.Now()

	// Use a very short interval.
	d.cfg.Daemon.HeartbeatInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.startHeartbeat(ctx)

	// Give the goroutine time for at least two heartbeat ticks.
	time.Sleep(200 * time.Millisecond)

	val, err := cache.Get(ctx, d.redis, heartbeatKey)
	require.NoError(t, err)
	assert.NotEmpty(t, val, "heartbeat key should be set by the goroutine")

	// Stop by canceling context.
	cancel()

	// Let the goroutine exit.
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat goroutine did not exit after context cancel")
	}
}

// TestStartHeartbeat_StopChannel verifies the heartbeat goroutine exits on stopCh.
func TestStartHeartbeat_StopChannel(t *testing.T) {
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.startedAt = time.Now()
	d.cfg.Daemon.HeartbeatInterval = 60 * time.Second // long interval, won't tick

	ctx := context.Background()
	d.startHeartbeat(ctx)

	// Signal stop.
	close(d.stopCh)

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat goroutine did not exit after stopCh closed")
	}
}
