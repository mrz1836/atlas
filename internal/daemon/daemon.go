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

	"github.com/google/uuid"
	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
)

// errInvalidPID is returned when the PID file contains a non-numeric or non-positive value.
var errInvalidPID = errors.New("invalid pid in pid file")

// errShutdownTimeout is returned from Stop when goroutines do not drain within ShutdownTimeout.
var errShutdownTimeout = errors.New("shutdown timeout exceeded: goroutines still running")

// errDaemonAlreadyRunning is returned when a live daemon is detected at startup.
var errDaemonAlreadyRunning = errors.New("daemon already running")

// readyEnvVar is the env variable the parent passes the write-end fd number through.
// When set, the daemon writes "ready\n" (success) or "error:<msg>\n" (failure) to that fd.
const readyEnvVar = "ATLAS_READY_FD"

// Option configures a Daemon.
type Option func(*Daemon)

// WithExecutor sets the TaskExecutor used by the daemon's worker pool.
// When not provided, the runner operates in stub mode (simulates execution).
func WithExecutor(e TaskExecutor) Option {
	return func(d *Daemon) {
		d.executor = e
	}
}

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

	// lastHeartbeatAt tracks when the heartbeat was last refreshed (for doctor diagnostics).
	lastHeartbeatAt time.Time
	heartbeatMu     sync.RWMutex

	// recoveryEvents holds the most recent per-task recovery decisions from startup.
	// Protected by recoveryMu; capped at maxStoredRecoveryEvents.
	recoveryEvents []RecoveryEvent
	recoveryMu     sync.RWMutex

	// executor is the task engine bridge; injected via WithExecutor.
	executor TaskExecutor

	// server is the Unix socket JSON-RPC server (wired in Start).
	server *Server
	// runner is the worker pool that executes queued tasks (wired in Start).
	runner *Runner

	// workspaceLoader is an injectable function for listing filesystem workspaces
	// during reconcile. Defaults to the production implementation; tests override it.
	workspaceLoader func(atlasHome string) ([]*workspaceEntry, error)

	// newTaskID is an injectable task-ID generator. Defaults to a random UUID;
	// tests override it to make submit flows deterministic (e.g., to fault-inject
	// on a known task-hash key).
	newTaskID func() string
}

// New creates a new Daemon instance.
func New(cfg *config.Config, logger zerolog.Logger, opts ...Option) *Daemon {
	d := &Daemon{
		cfg:    cfg,
		logger: logger,
		stopCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Start runs the daemon readiness sequence and binds the IPC socket last.
//
// Readiness order:
//  1. Expand path config fields
//  2. Ensure log directory exists
//  3. Redis connect + ping
//  4. Create queue and event publisher
//  5. Check/remove stale PID file
//  6. Write PID file
//  7. Start heartbeat goroutine
//  8. Orphan recovery
//  9. Start worker pool
//  10. Start IPC server (last — no submit can be accepted before all gates above pass)
//  11. Signal ready to parent
//  12. Publish daemon.started event
func (d *Daemon) Start(ctx context.Context) error {
	err := d.doStart(ctx)
	if err != nil {
		d.signalReady(err) // notify parent of failure before returning
	}
	return err
}

// doStart performs the ordered startup sequence and signals ready on success.
func (d *Daemon) doStart(ctx context.Context) error { //nolint:funcorder // called by Start before Run is defined
	d.startedAt = time.Now().UTC()

	// 0. Expand ~ in all path config fields.
	if expanded, err := ExpandSocketPath(d.cfg.Daemon.SocketPath); err == nil {
		d.cfg.Daemon.SocketPath = expanded
	}
	if expanded, err := ExpandSocketPath(d.cfg.Daemon.PIDFile); err == nil {
		d.cfg.Daemon.PIDFile = expanded
	}
	if expanded, err := ExpandSocketPath(d.cfg.Daemon.LogFile); err == nil {
		d.cfg.Daemon.LogFile = expanded
	}

	// 0b. Ensure log directory exists before anything can fail.
	if d.cfg.Daemon.LogFile != "" {
		if err := d.ensureLogDir(); err != nil {
			return err
		}
	}

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
		return fmt.Errorf("start: %w", err)
	}
	d.redis = client

	// 1b. Verify Redis is responsive (catches lazy-connect pool successes).
	if pingErr := PingRedis(ctx, d.redis); pingErr != nil {
		d.redis.Close()
		d.redis = nil
		return fmt.Errorf("start: redis not responding at %s: %w\n  Diagnose: redis-cli ping %s\n  macOS:    brew services start redis",
			redisCfg.Addr, pingErr, redisCfg.Addr)
	}

	// 2. Create queue and event publisher.
	keyPrefix := d.cfg.Redis.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "atlas:"
	}
	d.queue = NewRedisQueue(d.redis, keyPrefix)
	d.events = NewEventPublisher(d.redis, "")

	// Inject log writer into executor so step progress is streamed to Redis.
	logWriter := NewLogWriter(d.redis, keyPrefix, d.cfg.Redis.LogStreamMaxLen)
	if setter, ok := d.executor.(interface{ SetLogWriter(lw *LogWriter) }); ok {
		setter.SetLogWriter(logWriter)
	}

	// 3. Check for stale PID file; return error if a live daemon is already running.
	if err := d.checkStalePID(); err != nil {
		return err
	}

	// 4. Write PID file.
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("start: write pid file: %w", err)
	}

	// 5. Start heartbeat goroutine.
	d.startHeartbeat(ctx)

	// 6. Recover any orphaned tasks from a previous daemon run before accepting new work.
	if recoverErr := d.RecoverOrphanedTasks(ctx); recoverErr != nil {
		d.logger.Warn().Err(recoverErr).Msg("daemon: orphan recovery encountered errors (non-fatal)")
	}

	// 7. Start worker pool.
	d.runner = NewRunner(d.cfg, d.redis, d.queue, d.events, d.logger, d.executor)
	d.runner.Start(ctx)

	// 8. Start IPC server last — a client cannot submit until all readiness gates above pass.
	if d.cfg.Daemon.SocketPath != "" {
		if err := d.startServer(ctx); err != nil {
			return err
		}
	}

	// Signal ready to the parent process (no-op if ATLAS_READY_FD is not set).
	d.signalReady(nil)

	// 9. Publish daemon.started event (non-fatal).
	evt := TaskEvent{
		Type:   "daemon.started",
		TaskID: "",
		Status: "running",
	}
	if pubErr := d.events.Publish(ctx, evt); pubErr != nil {
		d.logger.Warn().Err(pubErr).Msg("daemon: failed to publish started event")
	}

	d.logger.Info().
		Int("pid", os.Getpid()).
		Str("socket", d.cfg.Daemon.SocketPath).
		Msg("daemon: started")

	return nil
}

// signalReady writes the ready status to the parent process via the ready pipe.
// It is a no-op when ATLAS_READY_FD is not set.
func (d *Daemon) signalReady(err error) { //nolint:funcorder // called by Start/doStart before Run is defined
	fdStr := os.Getenv(readyEnvVar)
	if fdStr == "" {
		return
	}
	fd, parseErr := strconv.Atoi(fdStr)
	if parseErr != nil || fd <= 2 { // guard stdin/stdout/stderr
		return
	}
	f := os.NewFile(uintptr(fd), "atlas-ready-pipe")
	if f == nil {
		return
	}
	defer func() { _ = f.Close() }()
	if err != nil {
		_, _ = fmt.Fprintf(f, "error:%s\n", err.Error())
	} else {
		_, _ = fmt.Fprint(f, "ready\n")
	}
}

// checkStalePID inspects the PID file and handles stale state.
// Returns errDaemonAlreadyRunning if a live daemon is detected.
// Removes and logs stale PID files (dead process or invalid content).
func (d *Daemon) checkStalePID() error { //nolint:funcorder // called by doStart before Run is defined
	pidFile := d.cfg.Daemon.PIDFile
	if pidFile == "" {
		return nil
	}

	// No PID file — nothing stale.
	if _, statErr := os.Stat(pidFile); os.IsNotExist(statErr) {
		return nil
	}

	running, existingPID, checkErr := IsRunning(pidFile)
	if checkErr != nil {
		// Invalid PID content — log and remove.
		d.logger.Warn().Err(checkErr).Str("path", pidFile).Msg("daemon: removing stale PID file with invalid content")
		_ = os.Remove(pidFile)
		return nil
	}
	if running {
		return fmt.Errorf("%w with PID %d; run: atlas daemon stop", errDaemonAlreadyRunning, existingPID)
	}

	// PID file exists but process is dead — remove it and log the decision.
	d.logger.Info().Str("path", pidFile).Msg("daemon: removing stale PID file (process no longer running)")
	if rmErr := os.Remove(pidFile); rmErr != nil && !os.IsNotExist(rmErr) {
		d.logger.Warn().Err(rmErr).Str("path", pidFile).Msg("daemon: failed to remove stale PID file")
	}
	return nil
}

// ensureLogDir creates the log file's parent directory if it does not exist.
func (d *Daemon) ensureLogDir() error { //nolint:funcorder // called by doStart before Run is defined
	logDir := socketDir(d.cfg.Daemon.LogFile)
	if logDir == "" {
		return nil
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("start: create log dir %q: %w", logDir, err)
	}
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
	// The goroutine below outlives Stop() if the timeout fires, but will
	// complete once in-flight goroutines finish. This is intentional — it
	// prevents a goroutine leak in the WaitGroup sense.
	go func() {
		d.wg.Wait()
		close(done)
	}()

	var stopErr error
	select {
	case <-done:
		d.logger.Info().Msg("daemon: all workers stopped cleanly")
	case <-time.After(timeout):
		stopErr = fmt.Errorf("%w after %s", errShutdownTimeout, timeout)
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
	return stopErr
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

// maxStoredRecoveryEvents is the maximum number of recovery events retained in memory
// for doctor/status output. Older events are dropped.
const maxStoredRecoveryEvents = 50

// storeRecoveryEvents saves recovery events for later access via Doctor/Health.
func (d *Daemon) storeRecoveryEvents(events []RecoveryEvent) {
	if len(events) == 0 {
		return
	}
	d.recoveryMu.Lock()
	defer d.recoveryMu.Unlock()
	d.recoveryEvents = append(d.recoveryEvents, events...)
	if len(d.recoveryEvents) > maxStoredRecoveryEvents {
		d.recoveryEvents = d.recoveryEvents[len(d.recoveryEvents)-maxStoredRecoveryEvents:]
	}
}

// getRecoveryEvents returns a snapshot of the stored recovery events.
func (d *Daemon) getRecoveryEvents() []RecoveryEvent {
	d.recoveryMu.RLock()
	defer d.recoveryMu.RUnlock()
	if len(d.recoveryEvents) == 0 {
		return nil
	}
	out := make([]RecoveryEvent, len(d.recoveryEvents))
	copy(out, d.recoveryEvents)
	return out
}

// socketDir returns the directory part of a socket/PID path.
// Returns "" for bare filenames with no directory component.
// generateTaskID returns a unique task ID, using the injected generator when set
// (tests) and a random UUID otherwise (production).
func (d *Daemon) generateTaskID() string {
	if d.newTaskID != nil {
		return d.newTaskID()
	}
	return uuid.New().String()
}

func socketDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}
