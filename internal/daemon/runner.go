package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
)

// Runner manages a pool of workers that execute tasks popped from the queue.
// Task execution is stubbed for Phase 4; real engine wiring is deferred to Phase 5.
type Runner struct {
	cfg      *config.Config
	redis    *cache.Client
	queue    Queue
	events   *EventPublisher
	logger   zerolog.Logger
	sem      chan struct{} // semaphore limiting concurrent tasks
	stopCh   chan struct{}
	wg       sync.WaitGroup
	workerID string // unique ID for lock namespacing

	// taskCtxs tracks per-task cancel funcs so individual tasks can be canceled
	// on demand (e.g., for Phase 5 task.cancel support).
	taskCtxMu sync.Mutex
	taskCtxs  map[string]context.CancelFunc
}

// NewRunner creates a Runner with a semaphore sized to cfg.Daemon.MaxParallelTasks.
func NewRunner(cfg *config.Config, redis *cache.Client, queue Queue, events *EventPublisher, logger zerolog.Logger) *Runner {
	maxP := cfg.Daemon.MaxParallelTasks
	if maxP <= 0 {
		maxP = 1
	}
	return &Runner{
		cfg:      cfg,
		redis:    redis,
		queue:    queue,
		events:   events,
		logger:   logger,
		sem:      make(chan struct{}, maxP),
		stopCh:   make(chan struct{}),
		workerID: fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		taskCtxs: make(map[string]context.CancelFunc),
	}
}

// Start begins the worker pool dispatch loop in a background goroutine.
func (r *Runner) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.dispatchLoop(ctx)
}

// Stop signals workers to stop and waits for in-flight tasks up to ShutdownTimeout.
func (r *Runner) Stop() {
	// Close stopCh once.
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}

	timeout := r.cfg.Daemon.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info().Msg("runner: all workers drained")
	case <-time.After(timeout):
		r.logger.Warn().Dur("timeout", timeout).Msg("runner: shutdown timeout exceeded, forcing stop")
	}
}

// dispatchLoop polls the queue and dispatches tasks to worker goroutines.
func (r *Runner) dispatchLoop(ctx context.Context) {
	defer r.wg.Done()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		taskID, err := r.queue.Pop(ctx)
		if err != nil {
			r.logger.Error().Err(err).Msg("runner: queue pop failed")
			select {
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		if taskID == "" {
			// Queue empty — back off before polling again.
			select {
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		// Acquire semaphore slot or return task to queue on shutdown.
		select {
		case r.sem <- struct{}{}:
			r.wg.Add(1)
			go r.executeTask(ctx, taskID) //nolint:gosec // G118: executeTask creates its own independent context; ctx is ignored inside.
		case <-r.stopCh:
			// Return the task so it can be picked up after restart.
			if submitErr := r.queue.Submit(ctx, taskID, PriorityNormal); submitErr != nil {
				r.logger.Warn().Err(submitErr).Str("task_id", taskID).Msg("runner: failed to requeue task on shutdown")
			}
			return
		}
	}
}

// executeTask runs a single task with panic recovery.
// Each task gets its own context derived from context.Background() so that
// daemon shutdown (which cancels the dispatch loop context) does not abruptly
// cancel in-flight Redis operations. Per-task cancel funcs are tracked in
// r.taskCtxs for future on-demand cancellation (Phase 5).
//
// Phase 4 stubs the actual execution with a simulated delay.
// Real task.Engine wiring is deferred to Phase 5.
//
//nolint:contextcheck // Intentional: each task gets an independent context so daemon shutdown does not cancel in-flight work.
func (r *Runner) executeTask(_ context.Context, taskID string) {
	// Create an independent context for this task's lifetime.
	//nolint:gosec // G118: context.Background() is intentional — task context must be independent of dispatch loop lifetime.
	taskCtx, cancel := context.WithCancel(context.Background()) //nolint:contextcheck // Intentional: independent task context; cancel is called in the deferred cleanup below.
	r.taskCtxMu.Lock()
	r.taskCtxs[taskID] = cancel
	r.taskCtxMu.Unlock()

	defer r.wg.Done()
	defer func() { <-r.sem }()
	defer func() {
		r.taskCtxMu.Lock()
		delete(r.taskCtxs, taskID)
		r.taskCtxMu.Unlock()
		cancel()
	}()

	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error().
				Interface("panic", rec).
				Str("task_id", taskID).
				Msg("runner: task panicked")
			r.markTaskFailed(taskCtx, taskID, fmt.Sprintf("panic: %v", rec))
			if r.events != nil {
				if pubErr := r.events.Publish(taskCtx, TaskEvent{
					Type:    EventTaskFailed,
					TaskID:  taskID,
					Status:  "failed",
					Message: fmt.Sprintf("panic: %v", rec),
				}); pubErr != nil {
					r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish task.failed event")
				}
			}
		}
	}()

	// Acquire a distributed lock to prevent double-execution across daemon instances.
	// Use a bounded timeout so a stalled Redis connection does not hold the worker slot.
	lockKey := r.cfg.Redis.KeyPrefix + "lock:task:" + taskID
	lockCtx, lockCancel := context.WithTimeout(taskCtx, 5*time.Second)
	defer lockCancel()
	locked, lockErr := cache.WriteLock(lockCtx, r.redis, lockKey, r.workerID, 60)
	if lockErr != nil {
		r.logger.Warn().Err(lockErr).Str("task_id", taskID).Msg("runner: lock check failed, skipping task")
		return
	}
	if !locked {
		r.logger.Debug().Str("task_id", taskID).Msg("runner: task already locked by another worker, skipping")
		return
	}
	defer func() {
		if _, relErr := cache.ReleaseLock(taskCtx, r.redis, lockKey, r.workerID); relErr != nil {
			r.logger.Warn().Err(relErr).Str("task_id", taskID).Msg("runner: failed to release lock")
		}
	}()

	r.markTaskRunning(taskCtx, taskID)
	if pubErr := r.events.Publish(taskCtx, TaskEvent{
		Type:   EventTaskStarted,
		TaskID: taskID,
		Status: "running",
	}); pubErr != nil {
		r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish task.started event")
	}

	r.logger.Info().Str("task_id", taskID).Msg("runner: executing task (stub)")

	// TODO(Phase 5): Wire to task.Engine for real execution.
	// Requires workspace, git worktree, AI config, and template resolution.
	time.Sleep(100 * time.Millisecond)

	r.markTaskCompleted(taskCtx, taskID)
	if pubErr := r.events.Publish(taskCtx, TaskEvent{
		Type:   EventTaskCompleted,
		TaskID: taskID,
		Status: "completed",
	}); pubErr != nil {
		r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish task.completed event")
	}
}

// markTaskRunning updates the task hash to status=running with a started_at timestamp.
func (r *Runner) markTaskRunning(ctx context.Context, taskID string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "running"},
		{"started_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to mark task running")
	}
}

// markTaskCompleted updates the task hash to status=completed with a completed_at timestamp.
func (r *Runner) markTaskCompleted(ctx context.Context, taskID string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "completed"},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to mark task completed")
	}

	// Remove from active set.
	activeKey := r.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetRemoveMember(ctx, r.redis, activeKey, taskID); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to remove task from active set")
	}
}

// markTaskFailed updates the task hash to status=failed with an error message.
func (r *Runner) markTaskFailed(ctx context.Context, taskID, errMsg string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "failed"},
		{"error", errMsg},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to mark task failed")
	}

	// Remove from active set.
	activeKey := r.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetRemoveMember(ctx, r.redis, activeKey, taskID); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to remove failed task from active set")
	}
}
