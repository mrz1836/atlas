package cli

import (
	"context"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/daemon"
)

// tryDaemonClient attempts to connect to the Atlas daemon via the configured
// Unix socket. Returns nil if the daemon is not running, so callers can fall
// through to their direct (non-daemon) execution path.
//
// Usage pattern:
//
//	if c := tryDaemonClient(ctx, cfg); c != nil {
//	    defer c.Close()
//	    // submit / query via daemon
//	    return
//	}
//	// fall through: direct execution
func tryDaemonClient(ctx context.Context, cfg *config.Config) *daemon.Client {
	c, err := daemon.DialFromConfigContext(ctx, cfg.Daemon.SocketPath)
	if err != nil {
		return nil
	}
	if !c.Ping() {
		_ = c.Close()
		return nil
	}
	return c
}
