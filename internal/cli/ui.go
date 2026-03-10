package cli

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/daemon"
	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

// Sentinel errors for the ui command.
var (
	errDaemonSocketNotConfigured = errors.New("daemon socket path not configured")
	errDaemonNotResponding       = errors.New("daemon is not responding — start it with: atlas daemon start")
)

// AddUICommand adds the ui command to the root command.
func AddUICommand(root *cobra.Command) {
	root.AddCommand(newUICmd())
}

// newUICmd creates the `atlas ui` command.
func newUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ui",
		Short: "Launch the Atlas interactive dashboard",
		Long: `Launch the full-screen Atlas TUI dashboard.

The dashboard provides real-time monitoring and interactive control of all Atlas
daemon tasks, including task list, detail panel, log streaming, and actions
(approve, reject, pause, resume, abandon, destroy).

Keyboard shortcuts:
  ↑/↓ or j/k  Navigate task list
  a            Approve task awaiting approval
  r            Reject task (with feedback)
  p            Pause running/queued task
  R            Resume paused/failed task
  x            Abandon task (with confirmation)
  d            Destroy workspace (with confirmation)
  l            Toggle full-screen log view
  1-4          Filter logs by level (all/info/warn/error)
  /            Search logs
  n/N          Next/prev search match
  g/G          Jump to top/bottom of logs
  ?            Show help overlay
  q/Ctrl+C     Quit

Requires the Atlas daemon to be running: atlas daemon start`,
		RunE: runUI,
	}
}

// runUI connects to the daemon and Redis, then launches the interactive dashboard.
func runUI(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Load configuration.
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Attempt to connect to the Atlas daemon.
	daemonClient, daemonErr := connectDaemonClient(ctx, cfg)

	// Attempt to connect to Redis for log streaming.
	redisClient, redisErr := connectRedisClient(ctx, cfg)

	// Create the dashboard model based on available connections.
	var model *dashboard.Model
	switch {
	case daemonClient != nil && redisClient != nil:
		keyPrefix := cfg.Redis.KeyPrefix
		if keyPrefix == "" {
			keyPrefix = "atlas:"
		}
		model = dashboard.NewWithCacheClient(daemonClient, redisClient)
		model.SetCacheClientWithPrefix(redisClient, keyPrefix)
	case daemonClient != nil:
		model = dashboard.NewWithClient(daemonClient)
	default:
		model = dashboard.New()
	}

	// Defer cleanup of connections.
	if daemonClient != nil {
		defer func() { _ = daemonClient.Close() }()
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	// Show startup error if daemon is unreachable.
	if daemonErr != nil {
		model.SetStartupError(buildStartupError(daemonErr, redisErr))
	} else if redisErr != nil {
		// Daemon is up but Redis unavailable — log streaming disabled, task monitoring still works.
		model.SetStartupError(fmt.Sprintf(
			"Redis unavailable — log streaming disabled: %v\n\nPress q to quit.",
			redisErr,
		))
	}

	// Launch the Bubble Tea program.
	p := tea.NewProgram(model, tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("dashboard: %w", err)
	}
	return nil
}

// connectDaemonClient attempts to connect to the Atlas daemon.
// Returns (client, nil) on success or (nil, error) if unreachable.
func connectDaemonClient(ctx context.Context, cfg *config.Config) (*daemon.Client, error) {
	socketPath := cfg.Daemon.SocketPath
	if socketPath == "" {
		return nil, errDaemonSocketNotConfigured
	}

	expanded, err := daemon.ExpandSocketPath(socketPath)
	if err != nil {
		return nil, fmt.Errorf("expand socket path: %w", err)
	}

	c, err := daemon.DialFromConfigContext(ctx, expanded)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", expanded, err)
	}

	if !c.PingContext(ctx) {
		_ = c.Close()
		return nil, errDaemonNotResponding
	}

	return c, nil
}

// connectRedisClient attempts to connect to Redis for log streaming.
// Returns (client, nil) on success or (nil, error) if unavailable.
func connectRedisClient(ctx context.Context, cfg *config.Config) (*cache.Client, error) {
	redisCfg := daemon.RedisConfig{
		Addr:         cfg.Redis.Addr,
		DB:           cfg.Redis.DB,
		Password:     cfg.Redis.Password,
		KeyPrefix:    cfg.Redis.KeyPrefix,
		PoolSize:     cfg.Redis.PoolSize,
		MaxRetries:   cfg.Redis.MaxRetries,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	}

	client, err := daemon.NewRedisClient(ctx, redisCfg)
	if err != nil {
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}

	if err := daemon.PingRedis(ctx, client); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}

// buildStartupError constructs a user-friendly startup error message.
func buildStartupError(daemonErr, redisErr error) string {
	msg := fmt.Sprintf(
		"Cannot connect to Atlas daemon: %v\n\nRun: atlas daemon start\n\nPress q to quit.",
		daemonErr,
	)
	if redisErr != nil {
		msg += fmt.Sprintf("\n\n(Redis also unavailable: %v)", redisErr)
	}
	return msg
}
