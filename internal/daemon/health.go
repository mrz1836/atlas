package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	cache "github.com/mrz1836/go-cache"
)

const (
	// heartbeatTTL is the Redis TTL for the daemon heartbeat key.
	// The heartbeat is refreshed every HeartbeatInterval (default 10s),
	// so a 30s TTL gives three missed beats before expiry.
	heartbeatTTL = 30 * time.Second

	// heartbeatKey is the Redis key used for the daemon liveness heartbeat.
	heartbeatKey = "atlas:daemon:heartbeat"

	// daemonStateKey is the Redis hash key storing daemon state metadata.
	daemonStateKey = "atlas:daemon:state"

	// daemonVersion is the current Atlas daemon version string.
	daemonVersion = "dev"
)

// startHeartbeat starts a goroutine that periodically refreshes the daemon heartbeat
// in Redis. The heartbeat key has a heartbeatTTL TTL and is refreshed every
// HeartbeatInterval (default 10s).
func (d *Daemon) startHeartbeat(ctx context.Context) {
	interval := d.cfg.Daemon.HeartbeatInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		// Immediately write an initial heartbeat on start.
		d.refreshHeartbeat(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				d.refreshHeartbeat(ctx)
			case <-d.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// refreshHeartbeat updates the daemon heartbeat key and state hash in Redis.
// Errors are logged but not propagated — a missed heartbeat is non-fatal.
func (d *Daemon) refreshHeartbeat(ctx context.Context) {
	if d.redis == nil {
		return
	}

	pid := os.Getpid()
	uptime := time.Since(d.startedAt).Round(time.Second).String()
	now := time.Now().UTC().Format(time.RFC3339)

	// Set heartbeat key with 30s TTL.
	if err := cache.SetExp(ctx, d.redis, heartbeatKey, now, heartbeatTTL); err != nil {
		d.logger.Warn().Err(err).Msg("daemon: failed to refresh heartbeat")
	}

	// Update state hash with current metadata.
	pairs := [][2]interface{}{
		{"pid", fmt.Sprintf("%d", pid)},
		{"uptime", uptime},
		{"version", daemonVersion},
		{"status", "running"},
		{"updated_at", now},
	}
	if err := cache.HashMapSet(ctx, d.redis, daemonStateKey, pairs); err != nil {
		d.logger.Warn().Err(err).Msg("daemon: failed to update state hash")
	}
}

// Health returns the current health status of the daemon including PID, uptime,
// Redis liveness, worker count, active task count, and queue depth.
func (d *Daemon) Health(ctx context.Context) (*DaemonStatusResponse, error) {
	resp := &DaemonStatusResponse{
		Version:   daemonVersion,
		PID:       os.Getpid(),
		StartedAt: d.startedAt.UTC().Format(time.RFC3339),
		Uptime:    time.Since(d.startedAt).Round(time.Second).String(),
		Workers:   d.cfg.Daemon.MaxParallelTasks,
	}

	// Check Redis liveness.
	if d.redis != nil {
		if err := PingRedis(ctx, d.redis); err == nil {
			resp.RedisAlive = true
		}
	}

	// Get active task count from Redis set.
	if d.redis != nil {
		activeKey := d.cfg.Redis.KeyPrefix + "active"
		members, membersErr := cache.SetMembers(ctx, d.redis, activeKey)
		if membersErr == nil {
			resp.ActiveTasks = len(members)
		}
	}

	// Get queue depth from all priorities.
	if d.queue != nil {
		stats, err := d.queue.Stats(ctx)
		if err == nil {
			resp.QueueDepth = int(stats.Total)
		}
	}

	return resp, nil
}
