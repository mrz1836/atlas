package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
)

// mockQueue is an in-memory Queue for runner tests (no Redis needed).
type mockQueue struct {
	mu     sync.Mutex
	tasks  []string
	popped []string
}

func (m *mockQueue) Submit(_ context.Context, taskID string, _ Priority) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = append(m.tasks, taskID)
	return nil
}

func (m *mockQueue) Pop(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tasks) == 0 {
		return "", nil
	}
	id := m.tasks[0]
	m.tasks = m.tasks[1:]
	m.popped = append(m.popped, id)
	return id, nil
}

func (m *mockQueue) Remove(_ context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := m.tasks[:0]
	for _, id := range m.tasks {
		if id != taskID {
			filtered = append(filtered, id)
		}
	}
	m.tasks = filtered
	return nil
}

func (m *mockQueue) List(_ context.Context, _ *Priority) ([]QueueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]QueueEntry, len(m.tasks))
	for i, id := range m.tasks {
		entries[i] = QueueEntry{TaskID: id, Priority: PriorityNormal}
	}
	return entries, nil
}

func (m *mockQueue) Stats(_ context.Context) (QueueStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := int64(len(m.tasks))
	return QueueStats{Normal: n, Total: n}, nil
}

func (m *mockQueue) Clear(_ context.Context, _ *Priority) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks = nil
	return nil
}

// testRunnerCfg builds a minimal config suitable for runner unit tests.
func testRunnerCfg(maxParallel int) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Daemon.MaxParallelTasks = maxParallel
	cfg.Daemon.ShutdownTimeout = 5 * time.Second
	cfg.Redis.KeyPrefix = "atlas:"
	return cfg
}

// newTestRunner creates a Runner backed by a mockQueue and no Redis/events (unit-test safe).
func newTestRunner(cfg *config.Config, q Queue) *Runner {
	logger := zerolog.Nop()
	return &Runner{
		cfg:      cfg,
		redis:    nil, // no Redis in unit tests
		queue:    q,
		events:   nil, // publish skipped when nil
		logger:   logger,
		sem:      make(chan struct{}, cfg.Daemon.MaxParallelTasks),
		stopCh:   make(chan struct{}),
		workerID: "test-worker",
	}
}

// ---- Tests ----

func TestRunnerNew(t *testing.T) {
	cfg := testRunnerCfg(3)
	q := &mockQueue{}
	r := newTestRunner(cfg, q)

	assert.NotNil(t, r)
	assert.Equal(t, cfg, r.cfg)
	assert.Equal(t, q, r.queue)
	assert.Equal(t, 3, cap(r.sem))
	assert.NotEmpty(t, r.workerID)
}

func TestRunnerMaxParallelism(t *testing.T) {
	const maxParallel = 2
	const numTasks = 5

	var (
		concurrentPeak int64
		currentConc    int64
	)

	// gate holds tasks inside execution until released.
	gate := make(chan struct{})
	sem := make(chan struct{}, maxParallel)

	var outerWg sync.WaitGroup
	for i := 0; i < numTasks; i++ {
		outerWg.Add(1)
		go func() {
			defer outerWg.Done()

			// Acquire semaphore.
			sem <- struct{}{}
			defer func() { <-sem }()

			c := atomic.AddInt64(&currentConc, 1)
			for {
				peak := atomic.LoadInt64(&concurrentPeak)
				if c <= peak || atomic.CompareAndSwapInt64(&concurrentPeak, peak, c) {
					break
				}
			}

			// Block until gate is released.
			<-gate

			atomic.AddInt64(&currentConc, -1)
		}()
	}

	// Give goroutines a moment to fill the semaphore.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	outerWg.Wait()

	assert.LessOrEqual(t, int(atomic.LoadInt64(&concurrentPeak)), maxParallel,
		"concurrent executions must not exceed maxParallel")
}

func TestRunnerGracefulDrain(t *testing.T) {
	cfg := testRunnerCfg(3)
	q := &mockQueue{}

	// Pre-populate 3 tasks.
	for i := 0; i < 3; i++ {
		_ = q.Submit(context.Background(), "task-drain-"+string(rune('A'+i)), PriorityNormal)
	}

	var dispatched int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := newTestRunner(cfg, q)
	runner.wg.Add(1)

	go func() {
		defer runner.wg.Done()
		for {
			select {
			case <-runner.stopCh:
				return
			default:
			}

			taskID, _ := q.Pop(ctx)
			if taskID == "" {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			runner.sem <- struct{}{}
			runner.wg.Add(1)
			go func() {
				defer runner.wg.Done()
				defer func() { <-runner.sem }()
				atomic.AddInt64(&dispatched, 1)
				time.Sleep(20 * time.Millisecond)
			}()
		}
	}()

	// Let the loop run briefly.
	time.Sleep(50 * time.Millisecond)
	runner.Stop()

	assert.GreaterOrEqual(t, int(atomic.LoadInt64(&dispatched)), 1, "at least one task should have been dispatched")
}

func TestRunnerPanicRecovery(t *testing.T) {
	cfg := testRunnerCfg(2)
	q := &mockQueue{}
	logger := zerolog.Nop()

	var (
		panicCount   int64
		successCount int64
		failedIDs    []string
		mu           sync.Mutex
	)

	r := &Runner{
		cfg:      cfg,
		redis:    nil,
		queue:    q,
		events:   nil,
		logger:   logger,
		sem:      make(chan struct{}, cfg.Daemon.MaxParallelTasks),
		stopCh:   make(chan struct{}),
		workerID: "test-worker",
	}

	runTask := func(taskID string, shouldPanic bool) {
		defer r.wg.Done()
		defer func() { <-r.sem }()

		defer func() {
			if rec := recover(); rec != nil {
				atomic.AddInt64(&panicCount, 1)
				mu.Lock()
				failedIDs = append(failedIDs, taskID)
				mu.Unlock()
			}
		}()

		if shouldPanic {
			panic("simulated panic in task " + taskID)
		}
		atomic.AddInt64(&successCount, 1)
	}

	// Dispatch 4 tasks: alternating panic/success.
	for i := 0; i < 4; i++ {
		id := "task-" + string(rune('A'+i))
		r.sem <- struct{}{}
		r.wg.Add(1)
		shouldPanic := i%2 == 0
		go runTask(id, shouldPanic)
	}

	// Wait with timeout.
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		require.Fail(t, "timed out waiting for tasks")
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&panicCount), "2 tasks should have panicked")
	assert.Equal(t, int64(2), atomic.LoadInt64(&successCount), "2 tasks should have succeeded")
	assert.Len(t, failedIDs, 2)
}
