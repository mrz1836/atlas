// Package daemon provides background process management and Redis connectivity
// for the Atlas task queue system.
package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	cache "github.com/mrz1836/go-cache"
)

// RedisConfig holds configuration for the Redis connection pool.
// These values are sourced from the daemon config section at runtime.
type RedisConfig struct {
	// Addr is the Redis server address (host:port).
	Addr string

	// DB is the Redis database number.
	DB int

	// Password is the Redis server password (empty for no auth).
	Password string

	// KeyPrefix is the namespace prefix for all Atlas keys (e.g., "atlas:").
	KeyPrefix string

	// PoolSize is the maximum number of active connections.
	PoolSize int

	// MaxRetries is the number of dial retry attempts.
	MaxRetries int

	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration

	// ReadTimeout is the timeout for read operations.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for write operations.
	WriteTimeout time.Duration
}

// NewRedisClient creates a new Redis client using the provided configuration.
// It establishes a connection pool via go-cache Connect and returns the client.
func NewRedisClient(ctx context.Context, cfg RedisConfig) (*cache.Client, error) {
	// Build the Redis URL in the format expected by go-cache:
	// redis://[:password@]host:port[/db]
	//
	// C3: redisURL may contain a plaintext password. It is zeroed after the
	// Connect call below to reduce the exposure window in process memory.
	// Ensure the library (go-cache) does not log the URL on connection errors.
	var redisURL string
	if cfg.Password != "" {
		redisURL = fmt.Sprintf("redis://:%s@%s/%d", cfg.Password, cfg.Addr, cfg.DB)
	} else {
		redisURL = fmt.Sprintf("redis://%s/%d", cfg.Addr, cfg.DB)
	}
	defer func() { redisURL = "" }() // zero the password-bearing string after use
	// safeURL omits the password so it is safe to use in error messages and logs.
	safeURL := fmt.Sprintf("redis://%s/%d", cfg.Addr, cfg.DB)

	// Derive pool params from config.
	// go-cache Connect: (ctx, url, maxActive, idleConnections, maxConnLifetime, idleTimeout, dependencyMode, newRelicEnabled)
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = 10
	}
	idleConnections := poolSize / 2
	if idleConnections < 1 {
		idleConnections = 1
	}

	dialTimeout := cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}

	// Use DialTimeout as idle timeout; max lifetime = 5× dial timeout.
	maxConnLifetime := dialTimeout * 5
	idleTimeout := dialTimeout * 3

	client, err := cache.Connect(ctx, redisURL, poolSize, idleConnections, maxConnLifetime, idleTimeout, false, false)
	if err != nil {
		return nil, fmt.Errorf("connecting to Redis at %s: %w", safeURL, err)
	}

	return client, nil
}

// PingRedis verifies the Redis connection is alive.
// Returns nil if the server responds, or an error describing the failure.
func PingRedis(ctx context.Context, client *cache.Client) error {
	if err := cache.Ping(ctx, client); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// Key builds a namespaced Redis key from a prefix and one or more parts.
// Parts are joined with ":" separators.
//
// Example:
//
//	Key("atlas:", "queue", "urgent")  →  "atlas:queue:urgent"
//	Key("atlas:", "task", id)         →  "atlas:task:<id>"
func Key(prefix string, parts ...string) string {
	if len(parts) == 0 {
		return prefix
	}
	// Trim any trailing colon from the prefix before joining.
	base := strings.TrimRight(prefix, ":")
	return base + ":" + strings.Join(parts, ":")
}
