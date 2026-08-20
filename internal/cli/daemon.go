package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/cli/workflow"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/daemon"
)

// Compile-time check: DaemonTaskExecutor implements daemon.TaskExecutor.
var _ daemon.TaskExecutor = (*workflow.DaemonTaskExecutor)(nil)

// Sentinel errors for daemon startup failures.
var (
	errDaemonFailedToStart     = errors.New("daemon failed to start")
	errDaemonExitedBeforeReady = errors.New("daemon process exited before signaling readiness")
	errDaemonStartTimedOut     = errors.New("daemon start timed out after 30s")
	errDaemonModeUnavailable   = errors.New("daemon mode requested (--daemon) but daemon is not running or unreachable; start the daemon with 'atlas daemon start' or omit --daemon to run direct mode")
)

// AddDaemonCommand adds the daemon command group to the root command.
func AddDaemonCommand(root *cobra.Command) {
	root.AddCommand(newDaemonCmd())
}

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the Atlas background daemon",
		Long:  "Start, stop, restart, and monitor the Atlas background daemon process.",
	}
	cmd.AddCommand(
		newDaemonStartCmd(),
		newDaemonStopCmd(),
		newDaemonRestartCmd(),
		newDaemonStatusCmd(),
		newDaemonPingCmd(),
		newDaemonDoctorCmd(),
	)
	return cmd
}

// newDaemonStartCmd creates `atlas daemon start`.
func newDaemonStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the Atlas daemon",
		RunE:  runDaemonStart,
	}
}

func runDaemonStart(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	socketPath, expandErr := daemon.ExpandSocketPath(cfg.Daemon.SocketPath)
	if expandErr != nil {
		return fmt.Errorf("expand socket path: %w", expandErr)
	}

	// Check if already running.
	if daemon.PingSocket(socketPath) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon is already running")
		return nil
	}

	// Re-exec self with --daemon flag as a detached background process.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	// Create a pipe so the child daemon can signal readiness (or failure) to this parent.
	readyR, readyW, pipeErr := os.Pipe()
	if pipeErr != nil {
		return fmt.Errorf("create ready pipe: %w", pipeErr)
	}
	defer func() { _ = readyR.Close() }()

	//nolint:gosec // G204: exe is from os.Executable, which is the current binary
	daemonCmd := exec.CommandContext(context.Background(), exe, "--daemon")
	setDaemonSysProcAttr(daemonCmd)
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil
	daemonCmd.Stdin = nil
	// Pass the write end of the ready pipe as an extra file descriptor (fd 3).
	daemonCmd.ExtraFiles = []*os.File{readyW}
	daemonCmd.Env = append(os.Environ(), "ATLAS_READY_FD=3")

	if startErr := daemonCmd.Start(); startErr != nil {
		_ = readyW.Close()
		return fmt.Errorf("start daemon: %w", startErr)
	}
	// Parent closes its copy of the write end — the child owns it.
	_ = readyW.Close()

	// Log file path for error messages.
	logPath := cfg.Daemon.LogFile
	if logPath == "" {
		logPath = "~/.atlas/logs/daemon.log"
	}

	// Read the ready signal from the child.
	readyCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(readyR)
		if scanner.Scan() {
			readyCh <- scanner.Text()
		} else {
			readyCh <- "" // child exited without writing
		}
	}()

	select {
	case msg := <-readyCh:
		switch {
		case msg == "ready":
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Daemon started (PID %d)\n  socket: %s\n  log:    %s\n",
				daemonCmd.Process.Pid, socketPath, logPath)
			return nil
		case strings.HasPrefix(msg, "error:"):
			reason := strings.TrimPrefix(msg, "error:")
			return fmt.Errorf("%w: %s\n  Logs: %s", errDaemonFailedToStart, reason, logPath)
		default:
			// Child exited without signaling — likely a very early failure.
			return fmt.Errorf("%w\n  Logs: %s", errDaemonExitedBeforeReady, logPath)
		}
	case <-time.After(30 * time.Second):
		return fmt.Errorf("%w\n  Logs: %s", errDaemonStartTimedOut, logPath)
	}
}

// newDaemonStopCmd creates `atlas daemon stop`.
func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the Atlas daemon",
		RunE:  runDaemonStop,
	}
}

func runDaemonStop(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	c, dialErr := daemon.DialFromConfigContext(cmd.Context(), cfg.Daemon.SocketPath)
	if dialErr != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon is not running")
		return nil
	}
	defer func() { _ = c.Close() }()

	var result map[string]any
	callCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	if callErr := c.Call(callCtx, daemon.MethodDaemonShutdown, nil, &result); callErr != nil {
		return fmt.Errorf("shutdown daemon: %w", callErr)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon stopped")
	return nil
}

// newDaemonRestartCmd creates `atlas daemon restart`.
func newDaemonRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the Atlas daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stop first (ignore errors — daemon may not be running)
			_ = runDaemonStop(cmd, args)
			// Wait a moment for the daemon to shut down
			time.Sleep(500 * time.Millisecond)
			// Start fresh
			return runDaemonStart(cmd, args)
		},
	}
}

// newDaemonStatusCmd creates `atlas daemon status`.
func newDaemonStatusCmd() *cobra.Command {
	var jsonOutput bool
	var reconcile bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the Atlas daemon status",
		Long: `Show the current Atlas daemon status.

Use --json for machine-readable output suitable for scripts and tooling.
Use --reconcile to walk Redis, task-store files, and workspace files and
report any drift between them.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDaemonStatus(cmd, jsonOutput, reconcile)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&reconcile, "reconcile", false, "Report drift between Redis and filesystem state")
	return cmd
}

func runDaemonStatus(cmd *cobra.Command, jsonOutput, reconcile bool) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	c, dialErr := daemon.DialFromConfigContext(cmd.Context(), cfg.Daemon.SocketPath)
	if dialErr != nil { //nolint:nestif // nested ifs are needed for JSON vs text output with Redis diagnostics
		out := cmd.OutOrStdout()

		if jsonOutput {
			b, err := json.MarshalIndent(map[string]any{
				"running": false,
				"error":   "daemon not running",
			}, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal status: %w", err)
			}
			_, _ = fmt.Fprintf(out, "%s\n", b)
			return nil
		}

		_, _ = fmt.Fprintln(out, "Daemon is not running")

		// Show Redis diagnostic when daemon is down.
		redisAddr := cfg.Redis.Addr
		if redisAddr == "" {
			redisAddr = "localhost:6379"
		}
		pingCtx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
		defer cancel()
		redisCfg := daemon.RedisConfig{
			Addr:        redisAddr,
			PoolSize:    1,
			DialTimeout: 2 * time.Second,
		}
		if rc, rcErr := daemon.NewRedisClient(pingCtx, redisCfg); rcErr != nil {
			_, _ = fmt.Fprintf(out, "\n%s\n", daemon.RedisDownMessage(redisAddr, rcErr))
		} else {
			if pingErr := daemon.PingRedis(pingCtx, rc); pingErr != nil {
				_, _ = fmt.Fprintf(out, "\n%s\n", daemon.RedisDownMessage(redisAddr, pingErr))
			}
			rc.Close()
		}
		return nil
	}
	defer func() { _ = c.Close() }()

	// Reconcile mode: walk Redis + filesystem and report drift.
	if reconcile {
		return runDaemonReconcile(cmd, c, jsonOutput)
	}

	var status daemon.DaemonStatusResponse
	callCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	if callErr := c.Call(callCtx, daemon.MethodDaemonStatus, nil, &status); callErr != nil {
		return fmt.Errorf("get daemon status: %w", callErr)
	}

	out := cmd.OutOrStdout()

	if jsonOutput {
		b, marshalErr := json.MarshalIndent(status, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal daemon status: %w", marshalErr)
		}
		_, _ = fmt.Fprintf(out, "%s\n", b)
		return nil
	}

	_, _ = fmt.Fprintln(out, "Daemon Status")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	_, _ = fmt.Fprintf(out, "  Version:      %s\n", status.Version)
	_, _ = fmt.Fprintf(out, "  PID:          %d\n", status.PID)
	_, _ = fmt.Fprintf(out, "  Uptime:       %s\n", status.Uptime)
	_, _ = fmt.Fprintf(out, "  Started at:   %s\n", status.StartedAt)
	_, _ = fmt.Fprintf(out, "  Redis alive:  %v\n", status.RedisAlive)
	_, _ = fmt.Fprintf(out, "  Workers:      %d\n", status.Workers)
	_, _ = fmt.Fprintf(out, "  Active tasks: %d\n", status.ActiveTasks)
	_, _ = fmt.Fprintf(out, "  Queue depth:  %d\n", status.QueueDepth)
	return nil
}

// runDaemonReconcile calls daemon.reconcile and renders the drift report.
func runDaemonReconcile(cmd *cobra.Command, c *daemon.Client, jsonOutput bool) error {
	var resp daemon.ReconcileResponse
	callCtx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()
	if callErr := c.Call(callCtx, daemon.MethodDaemonReconcile, nil, &resp); callErr != nil {
		return fmt.Errorf("reconcile: %w", callErr)
	}

	out := cmd.OutOrStdout()

	if jsonOutput {
		b, marshalErr := json.MarshalIndent(resp, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal reconcile response: %w", marshalErr)
		}
		_, _ = fmt.Fprintf(out, "%s\n", b)
		return nil
	}

	_, _ = fmt.Fprintln(out, "Reconciliation Report")
	_, _ = fmt.Fprintln(out, "═══════════════════════════════════════")
	_, _ = fmt.Fprintf(out, "  Atlas home:  %s\n", resp.AtlasHome)
	_, _ = fmt.Fprintf(out, "  Drift items: %d\n", resp.Total)
	_, _ = fmt.Fprintln(out)

	if resp.Total == 0 {
		_, _ = fmt.Fprintln(out, "✓ "+resp.Summary)
		return nil
	}

	for i, item := range resp.DriftItems {
		_, _ = fmt.Fprintf(out, "  [%d] type: %s\n", i+1, item.Type)
		if item.TaskID != "" {
			_, _ = fmt.Fprintf(out, "      task:       %s\n", item.TaskID)
		}
		if item.Workspace != "" {
			_, _ = fmt.Fprintf(out, "      workspace:  %s\n", item.Workspace)
		}
		if item.RedisStatus != "" {
			_, _ = fmt.Fprintf(out, "      redis:      %s\n", item.RedisStatus)
		}
		if item.FileStatus != "" {
			_, _ = fmt.Fprintf(out, "      file:       %s\n", item.FileStatus)
		}
		_, _ = fmt.Fprintf(out, "      action:     %s\n", item.SuggestedAction)
		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintln(out, "⚠ "+resp.Summary)
	return nil
}

// newDaemonPingCmd creates `atlas daemon ping`.
func newDaemonPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Ping the Atlas daemon",
		RunE:  runDaemonPing,
	}
}

func runDaemonPing(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	c, dialErr := daemon.DialFromConfigContext(cmd.Context(), cfg.Daemon.SocketPath)
	if dialErr != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "pong: daemon not running")
		return nil
	}
	defer func() { _ = c.Close() }()

	var resp daemon.DaemonPingResponse
	callCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	if callErr := c.Call(callCtx, daemon.MethodDaemonPing, nil, &resp); callErr != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "pong: daemon not responding")
		return nil
	}

	if resp.Version != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pong: daemon alive (version %s)\n", resp.Version)
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "pong: daemon alive")
	}
	return nil
}

// newDaemonDoctorCmd creates `atlas daemon doctor`.
func newDaemonDoctorCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show full daemon diagnostics",
		Long: `Show a detailed diagnostic report for the Atlas daemon.

Includes: Redis connection status, queue depth by priority, worker count,
active task count, socket/PID/log file paths, heartbeat age, and degraded reasons.

Use --json for machine-readable output.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDaemonDoctor(cmd, jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	return cmd
}

func runDaemonDoctor(cmd *cobra.Command, jsonOutput bool) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	c, dialErr := daemon.DialFromConfigContext(cmd.Context(), cfg.Daemon.SocketPath)
	if dialErr != nil {
		if jsonOutput {
			out := map[string]any{
				"error":  "daemon not running",
				"detail": dialErr.Error(),
			}
			b, marshalErr := json.MarshalIndent(out, "", "  ")
			if marshalErr != nil {
				return fmt.Errorf("marshal doctor: %w", marshalErr)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b)
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon is not running")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Start with: atlas daemon start\n")
		return nil
	}
	defer func() { _ = c.Close() }()

	var doc daemon.DaemonDoctorResponse
	callCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	if callErr := c.Call(callCtx, daemon.MethodDaemonDoctor, nil, &doc); callErr != nil {
		return fmt.Errorf("get doctor report: %w", callErr)
	}

	if jsonOutput {
		b, marshalErr := json.MarshalIndent(doc, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal doctor response: %w", marshalErr)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b)
		return nil
	}

	// Human-readable output.
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "Daemon Doctor Report")
	_, _ = fmt.Fprintln(out, "════════════════════════════════")
	_, _ = fmt.Fprintf(out, "  Version:        %s\n", doc.Version)
	_, _ = fmt.Fprintf(out, "  PID:            %d\n", doc.PID)
	_, _ = fmt.Fprintf(out, "  Uptime:         %s\n", doc.Uptime)
	_, _ = fmt.Fprintf(out, "  Started at:     %s\n", doc.StartedAt)
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Process State")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	socketStatus := "✗ missing"
	if doc.SocketExists {
		socketStatus = "✓ exists"
	}
	pidStatus := "✗ missing"
	if doc.PIDFileExists {
		pidStatus = "✓ exists"
	}
	_, _ = fmt.Fprintf(out, "  Socket path:    %s  %s\n", doc.SocketPath, socketStatus)
	_, _ = fmt.Fprintf(out, "  PID file:       %s  %s\n", doc.PIDFile, pidStatus)
	_, _ = fmt.Fprintf(out, "  Log file:       %s\n", doc.LogFile)
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Redis")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	redisStatus := "✗ unreachable"
	if doc.RedisAlive {
		redisStatus = "✓ connected"
	}
	_, _ = fmt.Fprintf(out, "  Address:        %s\n", doc.RedisAddr)
	_, _ = fmt.Fprintf(out, "  Status:         %s\n", redisStatus)
	if doc.HeartbeatAge != "" {
		_, _ = fmt.Fprintf(out, "  Heartbeat age:  %s\n", doc.HeartbeatAge)
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Queue")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	_, _ = fmt.Fprintf(out, "  Urgent:         %d\n", doc.QueueByPriority.Urgent)
	_, _ = fmt.Fprintf(out, "  Normal:         %d\n", doc.QueueByPriority.Normal)
	_, _ = fmt.Fprintf(out, "  Low:            %d\n", doc.QueueByPriority.Low)
	_, _ = fmt.Fprintf(out, "  Total:          %d\n", doc.QueueByPriority.Total)
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Workers")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	_, _ = fmt.Fprintf(out, "  Configured:     %d\n", doc.Workers)
	_, _ = fmt.Fprintf(out, "  Active tasks:   %d\n", doc.ActiveTasks)
	_, _ = fmt.Fprintln(out)

	if len(doc.DegradedReasons) > 0 {
		_, _ = fmt.Fprintln(out, "⚠ Degraded Reasons")
		_, _ = fmt.Fprintln(out, "─────────────────────────────")
		for _, reason := range doc.DegradedReasons {
			_, _ = fmt.Fprintf(out, "  • %s\n", reason)
		}
	} else {
		_, _ = fmt.Fprintln(out, "✓ No degraded reasons")
	}
	return nil
}

// RunDaemonProcess starts the daemon process in-process (blocking).
// Called when the binary is invoked with the --daemon flag.
func RunDaemonProcess(ctx context.Context) error {
	cfg, err := config.Load(ctx)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	logger := InitLogger(false, false)
	executor := workflow.NewDaemonTaskExecutor(cfg, logger)
	d := daemon.New(cfg, logger, daemon.WithExecutor(executor))
	return d.Run(ctx)
}
