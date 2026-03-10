package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/cli/workflow"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/daemon"
)

// Compile-time check: DaemonTaskExecutor implements daemon.TaskExecutor.
var _ daemon.TaskExecutor = (*workflow.DaemonTaskExecutor)(nil)

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

	// Check if already running
	if daemon.PingSocket(socketPath) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon is already running")
		return nil
	}

	// Re-exec self with --daemon flag as a detached background process
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	//nolint:gosec // G204: exe is from os.Executable, which is the current binary
	daemonCmd := exec.CommandContext(context.Background(), exe, "--daemon")
	daemonCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil
	daemonCmd.Stdin = nil
	if startErr := daemonCmd.Start(); startErr != nil {
		return fmt.Errorf("start daemon: %w", startErr)
	}

	// Wait briefly for daemon to bind socket (up to 2 seconds)
	for range 10 {
		time.Sleep(200 * time.Millisecond)
		if daemon.PingSocket(socketPath) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Daemon started (PID %d)\n", daemonCmd.Process.Pid)
			return nil
		}
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Daemon started but not yet responding")
	return nil
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

	var result map[string]interface{}
	if callErr := c.Call(cmd.Context(), daemon.MethodDaemonShutdown, nil, &result); callErr != nil {
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
	return &cobra.Command{
		Use:   "status",
		Short: "Show the Atlas daemon status",
		RunE:  runDaemonStatus,
	}
}

func runDaemonStatus(cmd *cobra.Command, _ []string) error {
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

	var status daemon.DaemonStatusResponse
	if callErr := c.Call(cmd.Context(), daemon.MethodDaemonStatus, nil, &status); callErr != nil {
		return fmt.Errorf("get daemon status: %w", callErr)
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "Daemon Status")
	_, _ = fmt.Fprintln(out, "─────────────────────────────")
	_, _ = fmt.Fprintf(out, "  PID:          %d\n", status.PID)
	_, _ = fmt.Fprintf(out, "  Uptime:       %s\n", status.Uptime)
	_, _ = fmt.Fprintf(out, "  Started at:   %s\n", status.StartedAt)
	_, _ = fmt.Fprintf(out, "  Redis alive:  %v\n", status.RedisAlive)
	_, _ = fmt.Fprintf(out, "  Workers:      %d\n", status.Workers)
	_, _ = fmt.Fprintf(out, "  Active tasks: %d\n", status.ActiveTasks)
	_, _ = fmt.Fprintf(out, "  Queue depth:  %d\n", status.QueueDepth)
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
	if callErr := c.Call(cmd.Context(), daemon.MethodDaemonPing, nil, &resp); callErr != nil {
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
