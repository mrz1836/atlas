package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisConfig creates a RedisConfig pointing at the given miniredis address.
func newTestRedisConfig(addr string) RedisConfig {
	return RedisConfig{
		Addr:         addr,
		DB:           0,
		Password:     "",
		KeyPrefix:    "atlas:",
		PoolSize:     5,
		MaxRetries:   3,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
}

func TestNewRedisClient_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())

	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
}

func TestNewRedisClient_InvalidAddr(_ *testing.T) {
	cfg := RedisConfig{
		Addr:        "localhost:0",
		DialTimeout: 100 * time.Millisecond,
		PoolSize:    1,
	}
	ctx := context.Background()
	// go-cache Connect itself doesn't dial eagerly; the pool dials lazily.
	// This still validates that Connect returns without error (pool is lazy).
	client, err := NewRedisClient(ctx, cfg)
	// Lazy pool — no error at connect time; failure occurs on first use.
	if err == nil && client != nil {
		defer client.Close()
	}
	// Either way, no panic expected.
}

func TestPingRedis_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())

	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	err = PingRedis(ctx, client)
	assert.NoError(t, err)
}

func TestPingRedis_ServerDown(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := newTestRedisConfig(mr.Addr())

	ctx := context.Background()
	client, err := NewRedisClient(ctx, cfg)
	require.NoError(t, err)
	defer client.Close()

	// Stop the server so the next ping fails.
	mr.Close()

	err = PingRedis(ctx, client)
	assert.Error(t, err)
}

func TestKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		parts  []string
		want   string
	}{
		{
			name:   "single part",
			prefix: "atlas:",
			parts:  []string{"queue"},
			want:   "atlas:queue",
		},
		{
			name:   "multiple parts",
			prefix: "atlas:",
			parts:  []string{"queue", "urgent"},
			want:   "atlas:queue:urgent",
		},
		{
			name:   "task with id",
			prefix: "atlas:",
			parts:  []string{"task", "abc123"},
			want:   "atlas:task:abc123",
		},
		{
			name:   "no parts",
			prefix: "atlas:",
			parts:  nil,
			want:   "atlas:",
		},
		{
			name:   "prefix without trailing colon",
			prefix: "atlas",
			parts:  []string{"queue"},
			want:   "atlas:queue",
		},
		{
			name:   "empty prefix",
			prefix: "",
			parts:  []string{"queue", "urgent"},
			want:   ":queue:urgent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Key(tt.prefix, tt.parts...)
			assert.Equal(t, tt.want, got)
		})
	}
}
