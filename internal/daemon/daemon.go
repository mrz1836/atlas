package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
)

// errInvalidPID is returned when the PID file contains a non-numeric or non-positive value.
var errInvalidPID = errors.New("invalid pid in pid file")

// Daemon manages the Atlas background process lifecycle.
type Daemon struct {
	cfg       *config.Config
	redis     *cache.Client
	queue     Queue
	events    *EventPublisher
	logger    zerolog.Logger
	stopCh    chan struct{}
	wg        sync.WaitGroup
	startedAt time.Time

	// server is the Unix socket JSON-RPC server (wired in Start).
	server *Server
	// runner is the worker pool that executes queued tasks (wired in Start).
	runner *Runner
}

// New creates a new Daemon instance.
func New(cfg *config.Config, logger zerolog.Logger) *Daemon {
	return &Daemon{
		cfg:    cfg,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start connects Redis, starts the heartbeat, writes the PID file, and
// publishes the daemon.started event.
//
// Unix-socket binding and worker-pool startup are deferred to Phase 4
// (server.go and runner.go). Those call-sites are marked with TODO comments below.
func (d *Daemon) Start(ctx context.Context) error {
	d.startedAt = time.Now().UTC()

	// 1. Connect to Redis.
	redisCfg := RedisConfig{
		Addr:         d.cfg.Redis.Addr,
		DB:           d.cfg.Redis.DB,
		Password:     d.cfg.Redis.Password,
		KeyPrefix:    d.cfg.Redis.KeyPrefix,
		PoolSize:     d.cfg.Redis.PoolSize,
		MaxRetries:   d.cfg.Redis.MaxRetries,
		DialTimeout:  d.cfg.Redis.DialTimeout,
		ReadTimeout:  d.cfg.Redis.ReadTimeout,
		WriteTimeout: d.cfg.Redis.WriteTimeout,
	}

	client, err := NewRedisClient(ctx, redisCfg)
	if err != nil {
		return fmt.Errorf("start: connect redis: %w", err)
	}
	d.redis = client

	// 2. Create queue and event publisher.
	keyPrefix := d.cfg.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "atlas:"
	}
	d.queue = NewRedisQueue(d.redis, keyPrefix)
	d.events = NewEventPublisher(d.redis, "")

	// 3. Create socket directory if needed and start the IPC server.
	if d.cfg.Daemon.SocketPath != "" {
		if err := d.startServer(ctx); err != nil {
			return err
		}
	}

	// 4. Write PID file.
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("start: write pid file: %w", err)
	}

	// 5. Start heartbeat goroutine.
	d.startHeartbeat(ctx)

	// Start worker pool.
	d.runner = NewRunner(d.cfg, d.redis, d.queue, d.events, d.logger)
	d.runner.Start(ctx)

	// 6. Publish daemon.started event.
	evt := TaskEvent{
		Type:   "daemon.started",
		TaskID: "",
		Status: "running",
	}
	if pubErr := d.events.Publish(ctx, evt); pubErr != nil {
		// Non-fatal — log and continue.
		d.logger.Warn().Err(pubErr).Msg("daemon: failed to publish started event")
	}

	d.logger.Info().
		Int("pid", os.Getpid()).
		Str("socket", d.cfg.Daemon.SocketPath).
		Msg("daemon: started")

	return nil
}

// Stop gracefully shuts down the daemon: signals goroutines to stop, waits for
// in-flight work up to ShutdownTimeout, removes the PID file, and disconnects Redis.
func (d *Daemon) Stop(_ context.Context) error {
	d.logger.Info().Msg("daemon: stopping")

	// Signal all goroutines.
	select {
	case <-d.stopCh:
		// Already closed — nothing to do.
	default:
		close(d.stopCh)
	}

	// Wait for goroutines to exit, honoring ShutdownTimeout.
	timeout := d.cfg.Daemon.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		d.logger.Info().Msg("daemon: all workers stopped cleanly")
	case <-time.After(timeout):
		d.logger.Warn().Dur("timeout", timeout).Msg("daemon: shutdown timeout exceeded; some tasks may still be in-flight")
	}

	// Stop IPC server and worker pool.
	if d.server != nil {
		d.server.Stop()
	}
	if d.runner != nil {
		d.runner.Stop()
	}

	// Remove PID file.
	d.removePIDFile()

	// Disconnect Redis.
	if d.redis != nil {
		d.redis.Close()
	}

	d.logger.Info().Msg("daemon: stopped")
	return nil
}

// Run blocks until a SIGTERM/SIGINT is received or Stop() is called.
// It starts the daemon first, then waits for a shutdown signal.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.Start(ctx); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		d.logger.Info().Str("signal", sig.String()).Msg("daemon: received signal")
		return d.Stop(context.Background()) //nolint:contextcheck // ctx may be canceled; use background for graceful shutdown
	case <-d.stopCh:
		return nil
	case <-ctx.Done():
		return d.Stop(context.Background()) //nolint:contextcheck // ctx is done; use background for graceful shutdown
	}
}

// IsRunning checks whether a daemon is already running by reading the PID file
// and probing the process with signal 0.
//
// Returns (true, pid, nil) if the process is alive, (false, 0, nil) if not,
// or (false, 0, err) on unexpected read errors.
func IsRunning(pidFile string) (bool, int, error) {
	data, err := os.ReadFile(pidFile) //nolint:gosec // pidFile is a controlled config value
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("read pid file %q: %w", pidFile, err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return false, 0, fmt.Errorf("%w: %q in %s", errInvalidPID, pidStr, pidFile)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		// On Unix, FindProcess never fails; on Windows it might.
		// Treat as not running rather than propagating the error.
		return false, 0, nil //nolint:nilerr // treat as not-running; FindProcess on Unix never fails, Windows may
	}

	// Signal(0) tests process existence without sending a real signal.
	if sigErr := proc.Signal(syscall.Signal(0)); sigErr != nil {
		// Process does not exist or is not accessible — treat as not running.
		return false, 0, nil //nolint:nilerr // intentional: signal(0) failure means no process
	}

	return true, pid, nil
}

// writePIDFile writes the current process PID to the configured PID file.
func (d *Daemon) writePIDFile() error {
	pidFile := d.cfg.Daemon.PIDFile
	if pidFile == "" {
		return nil
	}

	// Ensure the directory exists.
	dir := socketDir(pidFile)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create pid file dir %q: %w", dir, err)
		}
	}

	content := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(pidFile, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write pid file %q: %w", pidFile, err)
	}
	return nil
}

// removePIDFile removes the PID file on shutdown, logging errors but not propagating them.
func (d *Daemon) removePIDFile() {
	pidFile := d.cfg.Daemon.PIDFile
	if pidFile == "" {
		return
	}
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		d.logger.Warn().Err(err).Str("path", pidFile).Msg("daemon: failed to remove pid file")
	}
}

// startServer ensures the socket directory exists, then binds the Unix socket
// and starts the JSON-RPC server. Called from Start when SocketPath is non-empty.
func (d *Daemon) startServer(ctx context.Context) error {
	sockDir := socketDir(d.cfg.Daemon.SocketPath)
	if sockDir != "" {
		if mkErr := os.MkdirAll(sockDir, 0o700); mkErr != nil {
			return fmt.Errorf("start: create socket dir %q: %w", sockDir, mkErr)
		}
	}

	router := NewRouter(d.logger)
	d.setupRouter(router)
	d.server = NewServer(d.cfg.Daemon.SocketPath, router, d.logger)
	if srvErr := d.server.Start(ctx); srvErr != nil {
		return fmt.Errorf("start: bind unix socket: %w", srvErr)
	}
	return nil
}

// socketDir returns the directory part of a socket/PID path.
// Returns "" for bare filenames with no directory component.
func socketDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}
