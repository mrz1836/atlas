// Package testfakes provides reusable test helpers for Atlas daemon tests.
// These helpers create and manage fake/temporary dependencies so tests can
// exercise daemon behavior without real Redis, Git, or AI infrastructure.
package testfakes

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/daemon"
)

// RedisFixture wraps a miniredis server and a connected go-cache client.
// Obtain fixtures via NewRedisFixture; the server and primary client are
// cleaned up automatically when the test ends.
type RedisFixture struct {
	// MR is the underlying miniredis server. Use for direct key inspection
	// (e.g. mr.Get, mr.HGet) without going through the cache layer.
	MR *miniredis.Miniredis

	// Client is the primary go-cache connection. Suitable for hash, set, and
	// sorted-set operations. For pub/sub tests that need a dedicated subscriber
	// connection, call NewClient to obtain an additional connection.
	Client *cache.Client

	// Prefix is the Redis key namespace (e.g. "atlas:").
	Prefix string
}

// NewRedisFixture starts a miniredis server, connects a go-cache client, and
// registers cleanup with t. keyPrefix defaults to "atlas:" when empty.
func NewRedisFixture(t *testing.T, keyPrefix string) *RedisFixture {
	t.Helper()
	if keyPrefix == "" {
		keyPrefix = "atlas:"
	}
	mr := miniredis.RunT(t)
	client := newClient(t, mr.Addr(), keyPrefix)
	return &RedisFixture{MR: mr, Client: client, Prefix: keyPrefix}
}

// NewClient creates an additional go-cache client connected to the same
// miniredis server as f. Useful for pub/sub tests that require separate
// subscriber and publisher connections. The client is closed at test cleanup.
func (f *RedisFixture) NewClient(t *testing.T) *cache.Client {
	t.Helper()
	return newClient(t, f.MR.Addr(), f.Prefix)
}

// newClient creates and registers a go-cache client pointed at addr.
func newClient(t *testing.T, addr, keyPrefix string) *cache.Client {
	t.Helper()
	ctx := context.Background()
	client, err := daemon.NewRedisClient(ctx, daemon.RedisConfig{
		Addr:         addr,
		DB:           0,
		KeyPrefix:    keyPrefix,
		PoolSize:     5,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err, "testfakes.RedisFixture: connect to miniredis")
	t.Cleanup(func() { client.Close() })
	return client
}
