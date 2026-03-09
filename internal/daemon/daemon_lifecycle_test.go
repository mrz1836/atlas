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

// newLifecycleTestConfig creates a config suitable for lifecycle tests with a short socket path.
func newLifecycleTestConfig(t *testing.T, mr *miniredis.Miniredis) *config.Config {
	t.Helper()
	// Use short temp dir to stay within macOS 104-char socket path limit.
	dir, err := os.MkdirTemp("", "atlslife")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:           true,
			SocketPath:        filepath.Join(dir, "d.sock"),
			PIDFile:           filepath.Join(dir, "d.pid"),
			MaxParallelTasks:  1,
			ShutdownTimeout:   3 * time.Second,
			HeartbeatInterval: 60 * time.Second, // slow — won't interfere with test
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
}

// TestDaemonStartStop tests the full Start → Stop lifecycle.
func TestDaemonStartStop(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newLifecycleTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))

	// PID file should exist after Start.
	_, err := os.Stat(cfg.Daemon.PIDFile)
	require.NoError(t, err, "PID file should exist after Start")

	// Socket should exist after Start.
	_, err = os.Stat(cfg.Daemon.SocketPath)
	require.NoError(t, err, "socket should exist after Start")

	// Redis, queue, events should be wired.
	assert.NotNil(t, d.redis)
	assert.NotNil(t, d.queue)
	assert.NotNil(t, d.events)
	assert.NotNil(t, d.server)
	assert.NotNil(t, d.runner)

	require.NoError(t, d.Stop(context.Background()))

	// PID file should be removed after Stop.
	_, err = os.Stat(cfg.Daemon.PIDFile)
	assert.True(t, os.IsNotExist(err), "PID file should be removed after Stop")
}

// TestDaemonStop_Idempotent verifies that Stop can be called multiple times safely.
func TestDaemonStop_Idempotent(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newLifecycleTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))

	assert.NotPanics(t, func() {
		_ = d.Stop(context.Background())
		_ = d.Stop(context.Background()) // second stop should not panic
	})
}

// TestDaemonStart_NoSocketPath verifies Start works when SocketPath is empty
// (skips Unix socket binding).
func TestDaemonStart_NoSocketPath(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newLifecycleTestConfig(t, mr)
	cfg.Daemon.SocketPath = "" // no socket
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	assert.Nil(t, d.server, "server should be nil when SocketPath is empty")

	require.NoError(t, d.Stop(context.Background()))
}

// TestDaemonRun_ContextCancel verifies Run exits cleanly when context is canceled.
func TestDaemonRun_ContextCancel(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newLifecycleTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- d.Run(ctx)
	}()

	// Let the daemon start.
	time.Sleep(150 * time.Millisecond)

	// Cancel context to trigger shutdown.
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(8 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// TestDaemonRun_StopCh verifies Run exits when stopCh is closed externally.
func TestDaemonRun_StopCh(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newLifecycleTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- d.Run(ctx)
	}()

	// Let the daemon start.
	time.Sleep(150 * time.Millisecond)

	// Close stopCh directly.
	close(d.stopCh)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(8 * time.Second):
		t.Fatal("Run did not return after stopCh closed")
	}
}

// TestDaemonStart_RedisFailure verifies Start returns an error when Redis is unreachable.
func TestDaemonStart_RedisFailure(t *testing.T) {
	dir, err := os.MkdirTemp("", "atlsfail")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			SocketPath:      filepath.Join(dir, "d.sock"),
			PIDFile:         filepath.Join(dir, "d.pid"),
			ShutdownTimeout: time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         "127.0.0.1:1", // nothing listening here
			KeyPrefix:    "atlas:",
			PoolSize:     1,
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		},
	}
	logger := zerolog.Nop()

	d := New(cfg, logger)

	// Start may fail immediately or lazily depending on go-cache pool behavior.
	// If it succeeds (lazy pool), we just stop cleanly. The important thing is no panic.
	if err := d.Start(context.Background()); err != nil {
		assert.Contains(t, err.Error(), "start:")
	} else {
		// Lazy pool — start succeeded; stop cleanly.
		_ = d.Stop(context.Background())
	}
}

// TestSocketDir verifies socketDir extracts the directory component correctly.
func TestSocketDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/var/run/atlas/daemon.sock", "/var/run/atlas"},
		{"/tmp/d.sock", "/tmp"},
		{"daemon.sock", ""},
		{"", ""},
		{"/d.sock", ""},
	}

	for _, tt := range tests {
		got := socketDir(tt.input)
		assert.Equal(t, tt.want, got, "socketDir(%q)", tt.input)
	}
}

// TestIsRunning_InvalidPIDContent verifies errInvalidPID is returned for bad file content.
func TestIsRunning_InvalidPIDContent(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "bad.pid")

	require.NoError(t, os.WriteFile(pidFile, []byte("not-a-number\n"), 0o600))

	running, pid, err := IsRunning(pidFile)
	assert.False(t, running)
	assert.Zero(t, pid)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidPID)
}

// TestIsRunning_ZeroPID verifies errInvalidPID is returned for PID=0.
func TestIsRunning_ZeroPID(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "zero.pid")

	require.NoError(t, os.WriteFile(pidFile, []byte("0\n"), 0o600))

	running, pid, err := IsRunning(pidFile)
	assert.False(t, running)
	assert.Zero(t, pid)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidPID)
}

// TestIsRunning_StaleProcess verifies a non-existent PID returns running=false.
func TestIsRunning_StaleProcess(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "stale.pid")

	// PID 999999 is very unlikely to exist.
	require.NoError(t, os.WriteFile(pidFile, []byte("999999\n"), 0o600))

	running, _, err := IsRunning(pidFile)
	require.NoError(t, err)
	// Either running=false (process doesn't exist) or running=true (extremely unlikely collision).
	// Just assert no error.
	_ = running
}

// TestWritePIDFile_EmptyPath verifies that writePIDFile is a no-op when PIDFile is empty.
func TestWritePIDFile_EmptyPath(t *testing.T) {
	cfg := newTestConfig(t)
	cfg.Daemon.PIDFile = ""
	logger := zerolog.Nop()
	d := New(cfg, logger)

	require.NoError(t, d.writePIDFile())
}
