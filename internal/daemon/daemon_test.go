package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// newTestConfig returns a minimal Config suitable for daemon tests.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmp := t.TempDir()
	return &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:           true,
			SocketPath:        filepath.Join(tmp, "daemon.sock"),
			PIDFile:           filepath.Join(tmp, "daemon.pid"),
			LogFile:           filepath.Join(tmp, "daemon.log"),
			MaxParallelTasks:  3,
			TaskTimeout:       45 * time.Minute,
			ShutdownTimeout:   5 * time.Second,
			HeartbeatInterval: 10 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         "localhost:6379",
			KeyPrefix:    "atlas:",
			PoolSize:     5,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
	}
}

// TestDaemonNew verifies that New returns a non-nil, correctly initialized Daemon.
func TestDaemonNew(t *testing.T) {
	cfg := newTestConfig(t)
	logger := zerolog.Nop()

	d := New(cfg, logger)

	require.NotNil(t, d)
	assert.Equal(t, cfg, d.cfg)
	assert.NotNil(t, d.stopCh)
	assert.Nil(t, d.redis)
	assert.Nil(t, d.queue)
	assert.Nil(t, d.events)
}

// TestIsRunning_NoFile verifies that a missing PID file is treated as not running.
func TestIsRunning(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "daemon.pid")

	running, pid, err := IsRunning(pidFile)

	require.NoError(t, err)
	assert.False(t, running)
	assert.Zero(t, pid)
}

// TestIsRunning_SelfPID verifies that writing the current PID returns running=true.
func TestIsRunning_SelfPID(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "daemon.pid")

	myPID := os.Getpid()
	err := os.WriteFile(pidFile, []byte(strconv.Itoa(myPID)+"\n"), 0o600)
	require.NoError(t, err)

	running, pid, err := IsRunning(pidFile)

	require.NoError(t, err)
	assert.True(t, running)
	assert.Equal(t, myPID, pid)
}

// TestWriteRemovePIDFile verifies the full PID-file lifecycle.
func TestWriteRemovePIDFile(t *testing.T) {
	cfg := newTestConfig(t)
	logger := zerolog.Nop()
	d := New(cfg, logger)
	d.startedAt = time.Now()

	// Write PID file and verify its content.
	err := d.writePIDFile()
	require.NoError(t, err)

	data, err := os.ReadFile(cfg.Daemon.PIDFile)
	require.NoError(t, err)
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), pid)

	// Remove PID file and verify it is gone.
	d.removePIDFile()
	_, err = os.Stat(cfg.Daemon.PIDFile)
	assert.True(t, os.IsNotExist(err), "PID file should have been removed")
}

// TestRemovePIDFile_NonExistent verifies that removePIDFile does not panic or error
// when the PID file does not exist.
func TestRemovePIDFile_NonExistent(t *testing.T) {
	cfg := newTestConfig(t)
	logger := zerolog.Nop()
	d := New(cfg, logger)

	// Should not panic.
	assert.NotPanics(t, func() { d.removePIDFile() })
}
