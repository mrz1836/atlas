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

// startHeartbeat writes the first heartbeat synchronously (so callers can rely on it
// being present once startHeartbeat returns), then starts a goroutine for subsequent
// periodic refreshes.
func (d *Daemon) startHeartbeat(ctx context.Context) {
	// Write the first heartbeat synchronously so Start() doesn't return until
	// the heartbeat key is in Redis.
	d.refreshHeartbeat(ctx)

	interval := d.cfg.Daemon.HeartbeatInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

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
	} else {
		// Track last successful heartbeat time for doctor diagnostics.
		d.heartbeatMu.Lock()
		d.lastHeartbeatAt = time.Now().UTC()
		d.heartbeatMu.Unlock()
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

// Doctor returns a full diagnostic report: all fields from Health plus socket/PID/log
// file existence, Redis address, heartbeat age, queue depth by priority, and degraded reasons.
func (d *Daemon) Doctor(ctx context.Context) (*DaemonDoctorResponse, error) {
	resp := &DaemonDoctorResponse{
		Version:   daemonVersion,
		PID:       os.Getpid(),
		StartedAt: d.startedAt.UTC().Format(time.RFC3339),
		Uptime:    time.Since(d.startedAt).Round(time.Second).String(),
		Workers:   d.cfg.Daemon.MaxParallelTasks,
		RedisAddr: d.cfg.Redis.Addr,
		SocketPath: d.cfg.Daemon.SocketPath,
		PIDFile:   d.cfg.Daemon.PIDFile,
		LogFile:   d.cfg.Daemon.LogFile,
	}

	// Check file existence.
	if d.cfg.Daemon.SocketPath != "" {
		if _, err := os.Stat(d.cfg.Daemon.SocketPath); err == nil {
			resp.SocketExists = true
		}
	}
	if d.cfg.Daemon.PIDFile != "" {
		if _, err := os.Stat(d.cfg.Daemon.PIDFile); err == nil {
			resp.PIDFileExists = true
		}
	}

	// Redis liveness.
	if d.redis != nil {
		if err := PingRedis(ctx, d.redis); err == nil {
			resp.RedisAlive = true
		}
	}

	// Heartbeat age (from in-memory last-refreshed time).
	d.heartbeatMu.RLock()
	lastHB := d.lastHeartbeatAt
	d.heartbeatMu.RUnlock()
	if !lastHB.IsZero() {
		resp.HeartbeatAge = time.Since(lastHB).Round(time.Second).String()
	}

	// Active task count.
	if d.redis != nil {
		activeKey := d.cfg.Redis.KeyPrefix + "active"
		members, membersErr := cache.SetMembers(ctx, d.redis, activeKey)
		if membersErr == nil {
			resp.ActiveTasks = len(members)
		}
	}

	// Queue depth by priority.
	if d.queue != nil {
		if stats, err := d.queue.Stats(ctx); err == nil {
			resp.QueueByPriority = QueuePriorityStats{
				Urgent: int(stats.Urgent),
				Normal: int(stats.Normal),
				Low:    int(stats.Low),
				Total:  int(stats.Total),
			}
		}
	}

	// Collect degraded reasons (non-nil slice for clean JSON: "[]" not "null").
	degraded := make([]string, 0)
	if !resp.RedisAlive {
		degraded = append(degraded, fmt.Sprintf("redis unreachable at %s", resp.RedisAddr))
	}
	if d.cfg.Daemon.SocketPath != "" && !resp.SocketExists {
		degraded = append(degraded, fmt.Sprintf("socket file missing: %s", resp.SocketPath))
	}
	resp.DegradedReasons = degraded

	return resp, nil
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
