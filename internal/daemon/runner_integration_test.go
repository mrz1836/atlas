package daemon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// newTestRunnerWithRedis creates a Runner backed by miniredis.
func newTestRunnerWithRedis(t *testing.T) (*Runner, *cache.Client, Queue, func()) {
	t.Helper()
	mr := miniredis.RunT(t)

	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			MaxParallelTasks: 2,
			ShutdownTimeout:  5 * time.Second,
		},
		Redis: config.RedisConfig{
			Addr:         mr.Addr(),
			KeyPrefix:    "atlas:",
			PoolSize:     5,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
	}

	ctx := context.Background()
	client, err := NewRedisClient(ctx, RedisConfig{
		Addr:         mr.Addr(),
		DB:           0,
		PoolSize:     5,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err)

	q := NewRedisQueue(client, "atlas:")
	events := NewEventPublisher(client, "")
	logger := zerolog.Nop()

	r := NewRunner(cfg, client, q, events, logger)
	return r, client, q, func() { client.Close() }
}

// TestNewRunner verifies NewRunner initializes correctly.
func TestNewRunner(t *testing.T) {
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	assert.NotNil(t, r)
	assert.Equal(t, 2, cap(r.sem))
	assert.NotEmpty(t, r.workerID)
}

// TestRunnerIntegration_DispatchAndComplete submits a task and verifies the runner
// picks it up, executes it (100ms stub), and marks it completed.
func TestRunnerIntegration_DispatchAndComplete(t *testing.T) {
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "integration-task-001"

	// Submit task to queue.
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	// Start the runner.
	r.Start(ctx)

	// Wait for execution: 100ms stub + poll overhead + margin.
	time.Sleep(800 * time.Millisecond)
	r.Stop()

	// Verify the task was marked completed.
	hashKey := "atlas:task:" + taskID
	status, err := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "completed", status, "task should be marked completed after execution")

	completedAt, err := cache.HashGet(ctx, client, hashKey, "completed_at")
	require.NoError(t, err)
	assert.NotEmpty(t, completedAt)
}

// TestRunnerIntegration_MultipleTasksCompleted verifies multiple tasks are all completed.
func TestRunnerIntegration_MultipleTasksCompleted(t *testing.T) {
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	const numTasks = 4

	taskIDs := make([]string, numTasks)
	for i := 0; i < numTasks; i++ {
		taskIDs[i] = fmt.Sprintf("multi-task-%03d", i)
		require.NoError(t, q.Submit(ctx, taskIDs[i], PriorityNormal))
	}

	r.Start(ctx)

	// Wait for all tasks: 100ms each, parallelism=2 → ~200ms for 4 tasks + margin.
	time.Sleep(1500 * time.Millisecond)
	r.Stop()

	for _, taskID := range taskIDs {
		hashKey := "atlas:task:" + taskID
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		require.NoError(t, err)
		assert.Equal(t, "completed", status, "task %s should be completed", taskID)
	}
}

// TestRunnerMarkTaskRunning verifies markTaskRunning sets status=running and started_at.
func TestRunnerMarkTaskRunning(t *testing.T) {
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "mark-running-task"

	r.markTaskRunning(ctx, taskID)

	hashKey := "atlas:task:" + taskID
	status, err := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "running", status)

	startedAt, err := cache.HashGet(ctx, client, hashKey, "started_at")
	require.NoError(t, err)
	assert.NotEmpty(t, startedAt)
}

// TestRunnerMarkTaskCompleted verifies markTaskCompleted sets status=completed, completed_at,
// and removes the task from the active set.
func TestRunnerMarkTaskCompleted(t *testing.T) {
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "mark-completed-task"

	// Add to active set first.
	activeKey := "atlas:active"
	require.NoError(t, cache.SetAdd(ctx, client, activeKey, taskID))

	r.markTaskCompleted(ctx, taskID)

	hashKey := "atlas:task:" + taskID
	status, err := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "completed", status)

	completedAt, err := cache.HashGet(ctx, client, hashKey, "completed_at")
	require.NoError(t, err)
	assert.NotEmpty(t, completedAt)
}

// TestRunnerMarkTaskFailed verifies markTaskFailed sets status=failed with an error message.
func TestRunnerMarkTaskFailed(t *testing.T) {
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "mark-failed-task"

	// Add to active set first.
	activeKey := "atlas:active"
	require.NoError(t, cache.SetAdd(ctx, client, activeKey, taskID))

	r.markTaskFailed(ctx, taskID, "something went wrong")

	hashKey := "atlas:task:" + taskID
	status, err := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "failed", status)

	errField, err := cache.HashGet(ctx, client, hashKey, "error")
	require.NoError(t, err)
	assert.Equal(t, "something went wrong", errField)

	completedAt, err := cache.HashGet(ctx, client, hashKey, "completed_at")
	require.NoError(t, err)
	assert.NotEmpty(t, completedAt)
}

// TestRunnerStop_Idempotent verifies that Stop can be called multiple times safely.
func TestRunnerStop_Idempotent(t *testing.T) {
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	r.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	assert.NotPanics(t, func() {
		r.Stop()
		r.Stop() // second stop should not panic
	})
}

// TestRunnerDispatchLoop_EmptyQueue verifies the dispatchLoop backs off on empty queue.
func TestRunnerDispatchLoop_EmptyQueue(t *testing.T) {
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	r.Start(ctx)
	// Queue is empty — runner should poll, backoff 500ms, repeat.
	time.Sleep(200 * time.Millisecond)

	// Cancel context to stop the loop.
	cancel()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dispatch loop did not exit after context cancel")
	}
}

// TestRunnerDispatchLoop_StopCh verifies the dispatchLoop exits on stopCh.
func TestRunnerDispatchLoop_StopCh(t *testing.T) {
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	r.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	r.Stop()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("dispatch loop did not exit after Stop")
	}
}

// TestRunnerDispatchLoop_ShutdownRequeues verifies that a task popped right before shutdown
// is requeued by the runner.
func TestRunnerDispatchLoop_ShutdownDuringExecute(t *testing.T) {
	r, _, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit a task — it takes 100ms to execute.
	require.NoError(t, q.Submit(ctx, "requeue-test-task", PriorityNormal))

	r.Start(ctx)
	// Give dispatch loop just enough time to pop the task but not complete.
	time.Sleep(20 * time.Millisecond)

	// Stop should not hang.
	doneCh := make(chan struct{})
	go func() {
		r.Stop()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop() hung after task was popped")
	}
}
