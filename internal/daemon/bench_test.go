package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
)

// benchRedisClient creates a Redis client connected to a miniredis instance for benchmarks.
func benchRedisClient(b *testing.B) (*cache.Client, func()) {
	b.Helper()
	mr := miniredis.RunT(b)
	cfg := RedisConfig{
		Addr:         mr.Addr(),
		DB:           0,
		PoolSize:     10,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	client, err := NewRedisClient(context.Background(), cfg)
	if err != nil {
		b.Fatalf("redis connect: %v", err)
	}
	return client, func() { client.Close() }
}

// BenchmarkQueueSubmit measures the throughput of submitting tasks to the priority queue.
func BenchmarkQueueSubmit(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	q := NewRedisQueue(client, "bench:")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Submit(ctx, fmt.Sprintf("task-%d", i), PriorityNormal)
	}
}

// BenchmarkQueueSubmitUrgent measures Submit throughput for urgent priority.
func BenchmarkQueueSubmitUrgent(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	q := NewRedisQueue(client, "bench:")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Submit(ctx, fmt.Sprintf("urgent-task-%d", i), PriorityUrgent)
	}
}

// BenchmarkQueuePop measures the throughput of popping tasks from the priority queue.
func BenchmarkQueuePop(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	q := NewRedisQueue(client, "bench:")
	ctx := context.Background()

	// Pre-fill with tasks (enough for the benchmark run + buffer).
	fill := b.N + 1000
	for i := 0; i < fill; i++ {
		_ = q.Submit(ctx, fmt.Sprintf("task-%d", i), PriorityNormal)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = q.Pop(ctx)
	}
}

// BenchmarkQueueStats measures the throughput of reading queue statistics.
func BenchmarkQueueStats(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	q := NewRedisQueue(client, "bench:")
	ctx := context.Background()

	// Add some tasks so stats are non-trivial.
	for i := 0; i < 100; i++ {
		_ = q.Submit(ctx, fmt.Sprintf("task-%d", i), PriorityNormal)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = q.Stats(ctx)
	}
}

// BenchmarkConcurrentQueueSubmit measures concurrent submit throughput.
func BenchmarkConcurrentQueueSubmit(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	q := NewRedisQueue(client, "bench:")
	ctx := context.Background()
	var taskCounter int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int64
		for pb.Next() {
			i++
			_ = q.Submit(ctx, fmt.Sprintf("ptask-%d-%d", taskCounter, i), PriorityNormal)
		}
	})
}

// BenchmarkClientCall measures the round-trip latency of a JSON-RPC call over Unix socket.
func BenchmarkClientCall(b *testing.B) {
	dir, err := os.MkdirTemp("", "atlsbench")
	if err != nil {
		b.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	socketPath := dir + "/bench.sock"

	// Start a minimal test server inline.
	lc := &net.ListenConfig{}
	ln, listenErr := lc.Listen(context.Background(), "unix", socketPath)
	if listenErr != nil {
		b.Fatalf("listen: %v", listenErr)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go handleTestConn(conn)
		}
	}()

	c, dialErr := Dial(socketPath)
	if dialErr != nil {
		b.Fatalf("dial: %v", dialErr)
	}
	defer func() { _ = c.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var resp DaemonPingResponse
		_ = c.Call(MethodDaemonPing, nil, &resp)
	}
}

// BenchmarkEventPublish measures event publishing throughput.
func BenchmarkEventPublish(b *testing.B) {
	client, cleanup := benchRedisClient(b)
	defer cleanup()

	pub := NewEventPublisher(client, "bench:")
	ctx := context.Background()
	evt := TaskEvent{
		Type:   EventTaskSubmitted,
		TaskID: "bench-task-001",
		Status: "queued",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pub.Publish(ctx, evt)
	}
}

// BenchmarkHandlerDaemonPing measures the latency of the daemon.ping handler.
func BenchmarkHandlerDaemonPing(b *testing.B) {
	mr := miniredis.RunT(b)
	client, err := NewRedisClient(context.Background(), RedisConfig{
		Addr: mr.Addr(), PoolSize: 5,
		DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second,
	})
	if err != nil {
		b.Fatalf("redis: %v", err)
	}
	defer client.Close()

	cfg := &config.Config{
		Daemon: config.DaemonConfig{MaxParallelTasks: 1},
		Redis:  config.RedisConfig{Addr: mr.Addr(), KeyPrefix: "bench:"},
	}
	logger := zerolog.Nop()
	d := New(cfg, logger)
	d.redis = client
	d.queue = NewRedisQueue(client, "bench:")
	d.events = NewEventPublisher(client, "")

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.handleDaemonPing(ctx, nil)
	}
}
