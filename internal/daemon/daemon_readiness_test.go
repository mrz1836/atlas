package daemon

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// newReadinessTestConfig creates a config for readiness tests.
// It is identical to newLifecycleTestConfig but lives here so this test file
// can be read independently.
func newReadinessTestConfig(t *testing.T, mr *miniredis.Miniredis) *config.Config {
	t.Helper()
	dir, err := os.MkdirTemp("", "atlsrdns")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return &config.Config{
		Daemon: config.DaemonConfig{
			Enabled:           true,
			SocketPath:        filepath.Join(dir, "d.sock"),
			PIDFile:           filepath.Join(dir, "d.pid"),
			LogFile:           filepath.Join(dir, "logs", "d.log"),
			MaxParallelTasks:  1,
			ShutdownTimeout:   3 * time.Second,
			HeartbeatInterval: 60 * time.Second,
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

// ---------------------------------------------------------------------------
// Task 3.1: Readiness ordering
// ---------------------------------------------------------------------------

// TestReadiness_SocketBoundAfterPIDAndHeartbeat verifies that, after a successful Start,
// the PID file and heartbeat key are both present before (or alongside) the socket.
// Because the socket starts last, if PID or heartbeat were missing after Start returned
// successfully, the readiness contract would be violated.
func TestReadiness_SocketBoundAfterPIDAndHeartbeat(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// PID must be written (gate that precedes socket).
	_, pidErr := os.Stat(cfg.Daemon.PIDFile)
	require.NoError(t, pidErr, "PID file must exist when Start returns")

	// Heartbeat must be written (gate that precedes socket).
	hbVal, hbErr := mr.Get(heartbeatKey)
	require.NoError(t, hbErr, "heartbeat key must be written when Start returns")
	assert.NotEmpty(t, hbVal, "heartbeat value must not be empty")

	// Socket must be bound.
	_, sockErr := os.Stat(cfg.Daemon.SocketPath)
	require.NoError(t, sockErr, "socket must be bound when Start returns")

	// Runner and queue must be wired (worker pool gate precedes socket).
	assert.NotNil(t, d.runner, "worker pool must be started when Start returns")
	assert.NotNil(t, d.queue, "queue must be wired when Start returns")
}

// TestReadiness_RedisFailure_NoSocketBound verifies that if Redis is unreachable,
// Start fails before binding the socket. This is the inverse of the readiness contract:
// no socket should ever be bound if an earlier gate fails.
func TestReadiness_RedisFailure_NoSocketBound(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "atlsrfail")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			SocketPath:      filepath.Join(dir, "d.sock"),
			PIDFile:         filepath.Join(dir, "d.pid"),
			ShutdownTimeout: time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         "127.0.0.1:1", // nothing listening
			KeyPrefix:    "atlas:",
			PoolSize:     1,
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		},
	}
	logger := zerolog.Nop()
	d := New(cfg, logger)

	err = d.Start(context.Background())
	// Start must fail — Redis is unreachable.
	require.Error(t, err, "Start must fail when Redis is unreachable")
	assert.Contains(t, err.Error(), "start:")

	// The socket must NOT have been created (it starts last).
	_, sockErr := os.Stat(cfg.Daemon.SocketPath)
	assert.True(t, os.IsNotExist(sockErr),
		"socket must NOT be created when Redis is unreachable (readiness order violated)")

	// PID file must NOT have been written either.
	_, pidErr := os.Stat(cfg.Daemon.PIDFile)
	assert.True(t, os.IsNotExist(pidErr),
		"PID file must NOT be created when Redis is unreachable")
}

// TestReadiness_LogDirCreated verifies that the log file's parent directory is
// created automatically at startup.
func TestReadiness_LogDirCreated(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	dir, err := os.MkdirTemp("", "atlslogdir")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			SocketPath:        filepath.Join(dir, "d.sock"),
			PIDFile:           filepath.Join(dir, "d.pid"),
			LogFile:           filepath.Join(dir, "nested", "subdir", "d.log"),
			MaxParallelTasks:  1,
			ShutdownTimeout:   3 * time.Second,
			HeartbeatInterval: 60 * time.Second,
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
	logger := zerolog.Nop()
	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// The log directory must have been created.
	logDir := filepath.Join(dir, "nested", "subdir")
	info, statErr := os.Stat(logDir)
	require.NoError(t, statErr, "log directory should be created by Start")
	assert.True(t, info.IsDir(), "log directory path should be a directory")
}

// ---------------------------------------------------------------------------
// Task 3.2: Stale socket / PID / log path handling
// ---------------------------------------------------------------------------

// TestStaleSocket_RemovedSafely verifies that a stale socket file (left by a crashed
// daemon) is silently removed so the new daemon can start cleanly.
func TestStaleSocket_RemovedSafely(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	// Write a stale socket file.
	require.NoError(t, os.WriteFile(cfg.Daemon.SocketPath, []byte("stale"), 0o600))

	d := New(cfg, logger)
	ctx := context.Background()

	// Start must succeed despite the stale socket.
	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// A real, live socket should now exist.
	_, err := os.Stat(cfg.Daemon.SocketPath)
	require.NoError(t, err, "socket should exist after removing stale one")
}

// TestStalePID_DetectedAndRemediated verifies that a PID file pointing to a dead
// process is removed automatically and the daemon starts cleanly.
func TestStalePID_DetectedAndRemediated(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)

	var logBuf bytes.Buffer
	logger := zerolog.New(&logBuf)

	// Write a stale PID file pointing to a very-high PID that should not exist.
	require.NoError(t, os.WriteFile(cfg.Daemon.PIDFile, []byte("999998\n"), 0o600))

	d := New(cfg, logger)
	ctx := context.Background()

	// Start must succeed — stale PID should be cleaned up.
	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// Log should mention stale PID removal.
	logOut := logBuf.String()
	assert.True(t,
		strings.Contains(logOut, "stale PID") || strings.Contains(logOut, "stale pid"),
		"log should mention stale PID removal; got: %s", logOut)

	// The new PID file must point to the current process.
	pidData, readErr := os.ReadFile(cfg.Daemon.PIDFile)
	require.NoError(t, readErr)
	assert.Equal(t, strconv.Itoa(os.Getpid()), strings.TrimSpace(string(pidData)))
}

// TestStalePID_InvalidContent_RemovedAndReplaced verifies that a PID file with invalid
// content (non-numeric) is removed and replaced by a fresh one on startup.
func TestStalePID_InvalidContent_RemovedAndReplaced(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	require.NoError(t, os.WriteFile(cfg.Daemon.PIDFile, []byte("not-a-pid"), 0o600))

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// The PID file should now contain the actual process PID.
	pidData, readErr := os.ReadFile(cfg.Daemon.PIDFile)
	require.NoError(t, readErr)
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
	require.NoError(t, parseErr, "new PID file must contain a numeric PID")
	assert.Equal(t, os.Getpid(), pid)
}

// TestStalePID_LiveDaemon_ReturnsError verifies that if a live daemon PID already exists,
// Start returns errDaemonAlreadyRunning.
func TestStalePID_LiveDaemon_ReturnsError(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	// Write the current process's PID — it is guaranteed to be alive.
	require.NoError(t, os.WriteFile(cfg.Daemon.PIDFile,
		[]byte(strconv.Itoa(os.Getpid())+"\n"), 0o600))

	d := New(cfg, logger)
	err := d.Start(context.Background())
	require.Error(t, err, "Start must fail when a live daemon is already running")
	assert.ErrorIs(t, err, errDaemonAlreadyRunning)
}

// TestCheckStalePID_NoPIDFile verifies checkStalePID is a no-op when PIDFile is empty.
func TestCheckStalePID_NoPIDFile(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	d := New(cfg, zerolog.Nop())
	require.NoError(t, d.checkStalePID())
}

// TestCheckStalePID_AbsentFile verifies checkStalePID is a no-op when the file does not exist.
func TestCheckStalePID_AbsentFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			PIDFile: filepath.Join(tmp, "nonexistent.pid"),
		},
	}
	d := New(cfg, zerolog.Nop())
	require.NoError(t, d.checkStalePID())
}

// ---------------------------------------------------------------------------
// Task 3.3: Ready signaling via pipe
// ---------------------------------------------------------------------------

// newReadyPipe creates an OS pipe and returns the read end plus a dup'd write fd.
// The caller must close the read end (defer r.Close()) and the returned fd is owned
// by the caller; signalReady will close it after writing.
//
// Using syscall.Dup avoids a double-close bug: calling w.Fd() on an *os.File puts
// the fd into blocking mode but does not disable the GC finalizer.  If signalReady
// then closes the raw fd, the finalizer later closes the same (possibly reused) fd
// again.  Dup creates a new fd that signalReady owns; we close the original *os.File
// normally so the GC never sees a stale reference.
func newReadyPipe(t *testing.T) (r *os.File, writeFD int) {
	t.Helper()
	rFile, w, err := os.Pipe()
	require.NoError(t, err)

	// Duplicate the write end so we have an independent fd to hand to signalReady.
	wfd, dupErr := syscall.Dup(int(w.Fd()))
	require.NoError(t, dupErr)

	// Close the original wrapper — we now own wfd exclusively.
	require.NoError(t, w.Close())

	// If the test fails before signalReady runs, clean up the dup'd fd.
	t.Cleanup(func() {
		_ = syscall.Close(wfd) // EBADF if already closed by signalReady; that's fine
	})

	return rFile, wfd
}

// TestSignalReady_Success verifies that signalReady(nil) writes "ready\n" to the pipe.
func TestSignalReady_Success(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — run serially.
	r, wfd := newReadyPipe(t)
	defer func() { _ = r.Close() }()

	t.Setenv(readyEnvVar, strconv.Itoa(wfd))

	cfg := &config.Config{}
	d := New(cfg, zerolog.Nop())
	d.signalReady(nil)

	// signalReady closes the write end; read what was written.
	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan(), "should be able to read from pipe after signalReady")
	assert.Equal(t, "ready", scanner.Text())
}

// TestSignalReady_Error verifies that signalReady(err) writes "error:<msg>\n" to the pipe.
func TestSignalReady_Error(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — run serially.
	r, wfd := newReadyPipe(t)
	defer func() { _ = r.Close() }()

	t.Setenv(readyEnvVar, strconv.Itoa(wfd))

	cfg := &config.Config{}
	d := New(cfg, zerolog.Nop())

	testErr := assert.AnError
	d.signalReady(testErr)

	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan())
	line := scanner.Text()
	assert.True(t, strings.HasPrefix(line, "error:"), "expected error: prefix, got: %s", line)
	assert.Contains(t, line, testErr.Error())
}

// TestSignalReady_NoEnvVar verifies signalReady is a no-op when ATLAS_READY_FD is not set.
func TestSignalReady_NoEnvVar(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — run serially.
	// Ensure the env var is absent.
	t.Setenv(readyEnvVar, "")

	cfg := &config.Config{}
	d := New(cfg, zerolog.Nop())
	// Must not panic.
	assert.NotPanics(t, func() { d.signalReady(nil) })
}

// TestStart_SignalsReady verifies that a full Start emits "ready" via the pipe.
func TestStart_SignalsReady(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — run serially.
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)

	r, wfd := newReadyPipe(t)
	defer func() { _ = r.Close() }()

	t.Setenv(readyEnvVar, strconv.Itoa(wfd))

	logger := zerolog.Nop()
	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// Start() calls signalReady which closes the write end.
	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan(), "Start must write a ready signal to the pipe")
	assert.Equal(t, "ready", scanner.Text())
}

// TestStart_SignalsError_OnRedisFailure verifies that when Start fails due to Redis
// being unreachable, the error is written to the ready pipe.
func TestStart_SignalsError_OnRedisFailure(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — run serially.
	dir, err := os.MkdirTemp("", "atlspiperr")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			SocketPath:      filepath.Join(dir, "d.sock"),
			PIDFile:         filepath.Join(dir, "d.pid"),
			ShutdownTimeout: time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         "127.0.0.1:1",
			KeyPrefix:    "atlas:",
			PoolSize:     1,
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		},
	}

	r, wfd := newReadyPipe(t)
	defer func() { _ = r.Close() }()

	t.Setenv(readyEnvVar, strconv.Itoa(wfd))

	d := New(cfg, zerolog.Nop())
	err = d.Start(context.Background())
	require.Error(t, err, "Start should fail with unreachable Redis")

	// The pipe must have received an error signal.
	scanner := bufio.NewScanner(r)
	require.True(t, scanner.Scan(), "Start failure must write an error signal to the pipe")
	line := scanner.Text()
	assert.True(t, strings.HasPrefix(line, "error:"),
		"expected error: prefix in pipe signal, got: %s", line)
}

// ---------------------------------------------------------------------------
// Task 3.4: Redis-down diagnostics
// ---------------------------------------------------------------------------

// TestRedisDownMessage verifies the helper produces a message containing address,
// error, and fix commands.
func TestRedisDownMessage(t *testing.T) {
	t.Parallel()
	msg := RedisDownMessage("localhost:6379", assert.AnError)
	assert.Contains(t, msg, "localhost:6379", "message must include the Redis address")
	assert.Contains(t, msg, assert.AnError.Error(), "message must include the error")
	assert.Contains(t, msg, "redis-cli ping", "message must include redis-cli fix command")
	assert.Contains(t, msg, "brew services start redis", "message must include macOS fix command")
}

// TestDaemonStart_RedisDownErrorContainsAddress verifies that when Start fails due to
// Redis being unavailable, the error message contains the configured Redis address
// and a diagnostic fix command.
func TestDaemonStart_RedisDownErrorContainsAddress(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "atlsrdnserr")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	redisAddr := "127.0.0.1:1"
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			SocketPath:      filepath.Join(dir, "d.sock"),
			PIDFile:         filepath.Join(dir, "d.pid"),
			ShutdownTimeout: time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         redisAddr,
			KeyPrefix:    "atlas:",
			PoolSize:     1,
			DialTimeout:  100 * time.Millisecond,
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
		},
	}
	d := New(cfg, zerolog.Nop())

	err = d.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start:")
}

// ---------------------------------------------------------------------------
// Task 3.5: Daemon doctor handler
// ---------------------------------------------------------------------------

// TestDaemonDoctor_ReturnsAllFields verifies that the doctor handler returns a response
// with the required diagnostic fields: socket path, PID file, log file, Redis addr,
// heartbeat age, queue depth by priority, workers, and degraded reasons.
func TestDaemonDoctor_ReturnsAllFields(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// Allow heartbeat goroutine to write at least once.
	require.Eventually(t, func() bool {
		d.heartbeatMu.RLock()
		defer d.heartbeatMu.RUnlock()
		return !d.lastHeartbeatAt.IsZero()
	}, 3*time.Second, 10*time.Millisecond, "heartbeat must fire at least once")

	doc, err := d.Doctor(ctx)
	require.NoError(t, err)

	// Required fields.
	assert.Equal(t, cfg.Daemon.SocketPath, doc.SocketPath)
	assert.Equal(t, cfg.Daemon.PIDFile, doc.PIDFile)
	assert.Equal(t, cfg.Daemon.LogFile, doc.LogFile)
	assert.Equal(t, cfg.Redis.Addr, doc.RedisAddr)
	assert.True(t, doc.RedisAlive, "Redis should be alive")
	assert.True(t, doc.SocketExists, "socket should exist")
	assert.True(t, doc.PIDFileExists, "PID file should exist")
	assert.NotEmpty(t, doc.HeartbeatAge, "heartbeat age must be non-empty after heartbeat fired")
	assert.Empty(t, doc.DegradedReasons, "no degraded reasons on healthy daemon")
}

// TestDaemonDoctor_DegradedWhenRedisDown verifies degraded reasons are populated
// when Redis becomes unavailable after startup.
func TestDaemonDoctor_DegradedWhenRedisDown(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	// Simulate Redis going down mid-session.
	mr.Close()

	doc, err := d.Doctor(ctx)
	require.NoError(t, err)
	assert.False(t, doc.RedisAlive, "Redis should be unreachable after mr.Close()")
	require.NotEmpty(t, doc.DegradedReasons, "degraded reasons must not be empty when Redis is down")
	assert.Contains(t, doc.DegradedReasons[0], cfg.Redis.Addr)
}

// TestDaemonDoctor_JSON verifies the JSON output of the doctor response is parseable
// and contains all required keys.
func TestDaemonDoctor_JSON(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cfg := newReadinessTestConfig(t, mr)
	logger := zerolog.Nop()

	d := New(cfg, logger)
	ctx := context.Background()

	require.NoError(t, d.Start(ctx))
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	doc, err := d.Doctor(ctx)
	require.NoError(t, err)

	// Verify all JSON-exported fields are present.
	assert.NotEmpty(t, doc.Version)
	assert.Greater(t, doc.PID, 0)
	assert.NotEmpty(t, doc.Uptime)
	assert.NotEmpty(t, doc.StartedAt)
	assert.NotEmpty(t, doc.RedisAddr)
	assert.NotEmpty(t, doc.SocketPath)
	assert.NotEmpty(t, doc.PIDFile)
	assert.NotNil(t, doc.DegradedReasons, "DegradedReasons must not be nil (use empty slice)")
}
