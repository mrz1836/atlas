package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// newTestDaemonWithPrefix creates a Daemon backed by a given miniredis instance
// using a custom Redis key prefix. Useful for multi-daemon isolation tests.
func newTestDaemonWithPrefix(t *testing.T, mr *miniredis.Miniredis, prefix string) (*Daemon, func()) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "atls-prefix")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:           true,
			SocketPath:        filepath.Join(tmp, "d.sock"),
			PIDFile:           filepath.Join(tmp, "d.pid"),
			MaxParallelTasks:  1,
			ShutdownTimeout:   2 * time.Second,
			HeartbeatInterval: 10 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         mr.Addr(),
			KeyPrefix:    prefix,
			PoolSize:     5,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
	}

	redisCfg := RedisConfig{
		Addr:         mr.Addr(),
		DB:           0,
		PoolSize:     5,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	client, err := NewRedisClient(context.Background(), redisCfg)
	require.NoError(t, err)

	d := New(cfg, zerolog.Nop())
	d.redis = client
	d.startedAt = time.Now()
	d.queue = NewRedisQueue(client, prefix)
	d.events = NewEventPublisher(client, "")

	return d, func() { client.Close() }
}

// TestPrefixIsolation_HeartbeatKeys verifies that two daemons running with different
// Redis key prefixes do not see each other's heartbeat (Task 4.6: AC-AI-6).
//
// This test proves the contract: heartbeatKey and daemonStateKey are both namespaced
// by cfg.Redis.KeyPrefix, so prefix-A and prefix-B daemons are fully isolated.
func TestPrefixIsolation_HeartbeatKeys(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)

	dA, cleanupA := newTestDaemonWithPrefix(t, mr, "alpha:")
	defer cleanupA()

	dB, cleanupB := newTestDaemonWithPrefix(t, mr, "beta:")
	defer cleanupB()

	ctx := context.Background()

	// Write daemon-A heartbeat.
	dA.refreshHeartbeat(ctx)

	// Daemon-B should NOT see daemon-A's heartbeat.
	hbKeyA := heartbeatKey("alpha:")
	hbKeyB := heartbeatKey("beta:")

	assert.NotEqual(t, hbKeyA, hbKeyB, "heartbeat keys for different prefixes must differ")

	valA, err := mr.Get(hbKeyA)
	require.NoError(t, err, "daemon-A heartbeat key must exist")
	assert.NotEmpty(t, valA, "daemon-A heartbeat must be non-empty")

	// beta: heartbeat key must NOT exist yet (daemon-B has not written its heartbeat).
	_, errB := mr.Get(hbKeyB)
	assert.Error(t, errB, "daemon-B heartbeat key must not exist before daemon-B writes it")

	// Write daemon-B heartbeat.
	dB.refreshHeartbeat(ctx)

	valB, err := mr.Get(hbKeyB)
	require.NoError(t, err, "daemon-B heartbeat key must exist after write")
	assert.NotEmpty(t, valB, "daemon-B heartbeat must be non-empty")

	// Verify state keys are also isolated.
	stateKeyA := daemonStateKey("alpha:")
	stateKeyB := daemonStateKey("beta:")
	assert.NotEqual(t, stateKeyA, stateKeyB, "state keys for different prefixes must differ")

	pidA := mr.HGet(stateKeyA, "pid")
	assert.NotEmpty(t, pidA, "daemon-A state hash pid must exist")

	pidB := mr.HGet(stateKeyB, "pid")
	assert.NotEmpty(t, pidB, "daemon-B state hash pid must exist")

	// Both PIDs point to the same process (test process) — that's fine; what matters
	// is that the keys are different and both daemons wrote to their own namespace.
	assert.Equal(t, stateKeyA[:len("alpha")], "alpha", "state key A must start with alpha prefix")
	assert.Equal(t, stateKeyB[:len("beta")], "beta", "state key B must start with beta prefix")
}

// TestPrefixIsolation_QueueKeys verifies that two daemons with different prefixes
// do not see each other's queued tasks (Task 4.6: AC-AI-6).
func TestPrefixIsolation_QueueKeys(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)

	dA, cleanupA := newTestDaemonWithPrefix(t, mr, "alpha:")
	defer cleanupA()

	dB, cleanupB := newTestDaemonWithPrefix(t, mr, "beta:")
	defer cleanupB()

	ctx := context.Background()

	// Submit one task to each daemon's queue.
	require.NoError(t, dA.queue.Submit(ctx, "task-in-alpha", PriorityNormal))
	require.NoError(t, dB.queue.Submit(ctx, "task-in-beta", PriorityNormal))

	// Each daemon's queue should only contain its own task.
	statsA, err := dA.queue.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), statsA.Total, "alpha queue must contain only alpha tasks")

	statsB, err := dB.queue.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), statsB.Total, "beta queue must contain only beta tasks")

	// Pop from alpha — must get only alpha's task.
	gotA, _, err := dA.queue.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-in-alpha", gotA, "alpha pop must return alpha's task")

	// Pop from beta — must get only beta's task.
	gotB, _, err := dB.queue.Pop(ctx)
	require.NoError(t, err)
	assert.Equal(t, "task-in-beta", gotB, "beta pop must return beta's task")
}
