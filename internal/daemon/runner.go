package daemon

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/lifecycle"
)

// Runner manages a pool of workers that execute tasks popped from the queue.
type Runner struct {
	cfg       *config.Config
	redis     *cache.Client
	queue     Queue
	events    *EventPublisher
	logWriter *LogWriter
	logger    zerolog.Logger
	sem       chan struct{} // semaphore limiting concurrent tasks
	stopCh    chan struct{}
	wg        sync.WaitGroup
	workerID  string // unique ID for lock namespacing

	// executor runs the actual task engine. Nil means stub/dev mode.
	executor TaskExecutor

	// taskCtxs tracks per-task cancel funcs for on-demand cancellation.
	taskCtxMu sync.Mutex
	taskCtxs  map[string]context.CancelFunc

	// canceledTasks signals that a task was explicitly canceled or abandoned.
	// Value is the final status string: "canceled" | "abandoned" | "paused".
	canceledTasks sync.Map
}

// NewRunner creates a Runner with a semaphore sized to cfg.Daemon.MaxParallelTasks.
func NewRunner(cfg *config.Config, redis *cache.Client, queue Queue, events *EventPublisher, logger zerolog.Logger, executor TaskExecutor) *Runner {
	maxP := cfg.Daemon.MaxParallelTasks
	if maxP <= 0 {
		maxP = 1
	}
	return &Runner{
		cfg:       cfg,
		redis:     redis,
		queue:     queue,
		events:    events,
		logWriter: NewLogWriter(redis, cfg.Redis.KeyPrefix, cfg.Redis.LogStreamMaxLen),
		logger:    logger,
		executor:  executor,
		sem:       make(chan struct{}, maxP),
		stopCh:    make(chan struct{}),
		workerID:  fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		taskCtxs:  make(map[string]context.CancelFunc),
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

// CancelTask cancels the context of a running task, signaling it to stop.
// Stores "canceled" in canceledTasks so executeTask can set the correct final status.
// Returns true if the task was actively running, false otherwise.
func (r *Runner) CancelTask(ctx context.Context, taskID string) bool {
	r.canceledTasks.Store(taskID, "canceled")
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	if err := cache.HashMapSet(ctx, r.redis, hashKey, [][2]any{{"status", "canceled"}}); err != nil {
		r.logger.Error().Err(err).Str("task_id", taskID).Msg("runner: failed to persist canceled status")
	}

	r.taskCtxMu.Lock()
	cancel, ok := r.taskCtxs[taskID]
	r.taskCtxMu.Unlock()
	if ok {
		cancel()
		return true
	}
	return false
}

// PauseRunningTask cancels a running task and marks it for "paused" final status.
// The task can be resumed via task.resume. Returns true if the task was running.
func (r *Runner) PauseRunningTask(ctx context.Context, taskID string) bool {
	r.canceledTasks.Store(taskID, "paused")
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	if err := cache.HashMapSet(ctx, r.redis, hashKey, [][2]any{{"status", "paused"}}); err != nil {
		r.logger.Error().Err(err).Str("task_id", taskID).Msg("runner: failed to persist paused status")
	}

	r.taskCtxMu.Lock()
	cancel, ok := r.taskCtxs[taskID]
	r.taskCtxMu.Unlock()
	if ok {
		cancel()
		return true
	}
	return false
}

// AbandonRunningTask cancels a running task and marks it for "abandoned" final status.
// Returns true if the task was actively running, false otherwise.
func (r *Runner) AbandonRunningTask(ctx context.Context, taskID string) bool {
	r.canceledTasks.Store(taskID, "abandoned")
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	if err := cache.HashMapSet(ctx, r.redis, hashKey, [][2]any{{"status", "abandoned"}}); err != nil {
		r.logger.Error().Err(err).Str("task_id", taskID).Msg("runner: failed to persist abandoned status")
	}

	r.taskCtxMu.Lock()
	cancel, ok := r.taskCtxs[taskID]
	r.taskCtxMu.Unlock()
	if ok {
		cancel()
		return true
	}
	return false
}

// RequeueForResume stores optional approval fields in the Redis hash and
// re-submits the task to the queue at normal priority.
func (r *Runner) RequeueForResume(ctx context.Context, taskID, approvalChoice, rejectFeedback string) error {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{{"status", "queued"}}
	if approvalChoice != "" {
		pairs = append(pairs, [2]any{"approval_choice", approvalChoice})
	}
	if rejectFeedback != "" {
		pairs = append(pairs, [2]any{"reject_feedback", rejectFeedback})
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		return fmt.Errorf("requeue: update task hash: %w", err)
	}
	// Fetch previous priority to prevent inversion
	prio := PriorityNormal
	if vals, err := cache.HashMapGet(ctx, r.redis, hashKey, "priority"); err == nil {
		if p := safeIndex(vals, 0); p != "" {
			prio = Priority(p)
		}
	}
	// Ensure task is present in the active set (may have been removed on failure).
	activeKey := r.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetAdd(ctx, r.redis, activeKey, taskID); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to re-add task to active set on resume")
	}
	return r.queue.Submit(ctx, taskID, prio)
}

// loadTaskJob reads all per-task metadata fields from the Redis hash and
// returns a populated TaskJob.
func (r *Runner) loadTaskJob(ctx context.Context, taskID string) (TaskJob, error) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	fields := []any{
		"description", "template", "workspace", "branch",
		"repo_path", "agent", "model",
		"engine_task_id", "approval_choice", "reject_feedback",
		"target_branch", "use_local", "verify", "no_verify",
	}
	vals, err := cache.HashMapGet(ctx, r.redis, hashKey, fields...)
	if err != nil {
		return TaskJob{}, fmt.Errorf("load task job %s: %w", taskID, err)
	}
	return TaskJob{
		TaskID:         taskID,
		Description:    safeIndex(vals, 0),
		Template:       safeIndex(vals, 1),
		Workspace:      safeIndex(vals, 2),
		Branch:         safeIndex(vals, 3),
		RepoPath:       safeIndex(vals, 4),
		Agent:          safeIndex(vals, 5),
		Model:          safeIndex(vals, 6),
		EngineTaskID:   safeIndex(vals, 7),
		ApprovalChoice: safeIndex(vals, 8),
		RejectFeedback: safeIndex(vals, 9),
		TargetBranch:   safeIndex(vals, 10),
		UseLocal:       safeIndex(vals, 11) == "true",
		Verify:         safeIndex(vals, 12) == "true",
		NoVerify:       safeIndex(vals, 13) == "true",
	}, nil
}

// storeEngineTaskID persists the engine-assigned task ID into the Redis hash
// so future resume/approve/reject calls can find the engine task.
func (r *Runner) storeEngineTaskID(ctx context.Context, taskID, engineTaskID string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{{"engine_task_id", engineTaskID}}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to store engine task ID")
	}
}

// markTaskStatus sets the task's status field in the Redis hash.
func (r *Runner) markTaskStatus(ctx context.Context, taskID, status string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{{"status", status}}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to update task status")
	}
}

// setupQueueNotify subscribes to Redis queue notifications and returns a channel
// that receives a signal whenever a new task is enqueued. Returns nil if Redis
// is unavailable or the subscription fails.
func (r *Runner) setupQueueNotify(ctx context.Context) <-chan struct{} {
	if r.redis == nil {
		return nil
	}
	sub, err := cache.Subscribe(ctx, r.redis, []string{r.cfg.Redis.KeyPrefix + "queue:notify"})
	if err != nil {
		return nil
	}
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			case _, ok := <-sub.Messages:
				if !ok {
					return
				}
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ch
}

// backoffAfterError waits one second after a queue pop error.
// Returns false if the dispatch loop should exit.
func (r *Runner) backoffAfterError(ctx context.Context) bool {
	select {
	case <-r.stopCh:
		return false
	case <-ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}

// waitForEmptyQueue blocks until a new task may be available, or stop is signaled.
// notifyCh may be nil; a nil channel blocks forever so the timeout always fires.
// Returns false if the dispatch loop should exit.
func (r *Runner) waitForEmptyQueue(ctx context.Context, notifyCh <-chan struct{}) bool {
	select {
	case <-r.stopCh:
		return false
	case <-ctx.Done():
		return false
	case <-notifyCh: // wakes instantly on queue notification; nil channel never fires
	case <-time.After(500 * time.Millisecond):
	}
	return true
}

// dispatchLoop polls the queue and dispatches tasks to worker goroutines.
func (r *Runner) dispatchLoop(ctx context.Context) {
	defer r.wg.Done()

	notifyCh := r.setupQueueNotify(ctx)

	for {
		select {
		case <-r.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		taskID, prio, err := r.queue.Pop(ctx)
		if err != nil {
			r.logger.Error().Err(err).Msg("runner: queue pop failed")
			if !r.backoffAfterError(ctx) {
				return
			}
			continue
		}

		if taskID == "" {
			// Queue empty — back off before polling again.
			if !r.waitForEmptyQueue(ctx, notifyCh) {
				return
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
			if submitErr := r.queue.Submit(ctx, taskID, prio); submitErr != nil {
				r.logger.Warn().Err(submitErr).Str("task_id", taskID).Msg("runner: failed to requeue task on shutdown")
			}
			return
		}
	}
}

// executeTask runs a single task with panic recovery.
// Each task gets its own context derived from context.Background() so that
// daemon shutdown (which cancels the dispatch loop context) does not abruptly
// cancel in-flight Redis operations.
//
//nolint:contextcheck // Intentional independent task context; see doc comment above.
func (r *Runner) executeTask(_ context.Context, taskID string) {
	taskTimeout := r.cfg.Daemon.TaskTimeout
	if taskTimeout <= 0 {
		taskTimeout = 45 * time.Minute
	}
	taskCtx, cancel := context.WithTimeout(context.Background(), taskTimeout) //nolint:contextcheck // Intentional: independent task context; cancel is called in the deferred cleanup below.
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

	defer r.recoverTaskPanic(taskCtx, taskID)

	// Acquire a distributed lock to prevent double-execution across daemon instances.
	lockKey := r.cfg.Redis.KeyPrefix + "lock:task:" + taskID
	lockCtx, lockCancel := context.WithTimeout(taskCtx, 5*time.Second)
	defer lockCancel()
	lockTTL := int64(r.cfg.Daemon.TaskTimeout.Seconds())
	if lockTTL <= 0 {
		lockTTL = 2700 // 45 minutes fallback
	}
	locked, lockErr := cache.WriteLock(lockCtx, r.redis, lockKey, r.workerID, lockTTL)
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

	// H2: re-read the status from Redis after acquiring the lock. A concurrent
	// task.cancel handler may have written "canceled" between the queue Pop and
	// this point; if so, skip execution rather than overwriting the terminal state.
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	if statusVals, statusErr := cache.HashMapGet(taskCtx, r.redis, hashKey, "status"); statusErr == nil {
		if s := safeIndex(statusVals, 0); s == "canceled" || s == "abandoned" || s == "paused" {
			r.logger.Debug().Str("task_id", taskID).Str("status", s).
				Msg("runner: task already terminal; skipping execution")
			r.canceledTasks.Delete(taskID)
			r.finalizeCanceledTask(taskID, hashKey)
			return
		}
	}

	// H1: treat a failure to record running state as a hard error; proceeding
	// without updating Redis would leave the task stuck in "queued" forever.
	if err := r.markTaskRunning(taskCtx, taskID); err != nil {
		return
	}

	job, ok := r.loadJobAndAnnounce(taskCtx, taskID)
	if !ok {
		return
	}

	if r.executor == nil {
		r.runStubTask(taskCtx, taskID, job)
		return
	}

	r.logger.Info().Str("task_id", taskID).Msg("runner: executing task")
	engineTaskID, finalStatus, execErr := r.executor.Execute(taskCtx, job)

	// If the task context was canceled or timed out, handle accordingly.
	if taskCtx.Err() != nil {
		if r.handleTaskTimeout(taskCtx, taskID, taskTimeout) {
			return
		}
		r.finalizeCanceledTask(taskID, hashKey)
		return
	}

	// Store engine task ID for future resume/approve/reject (only on first start).
	if engineTaskID != "" && job.EngineTaskID == "" {
		r.storeEngineTaskID(taskCtx, taskID, engineTaskID)
	}

	r.handleExecutionOutcome(taskCtx, taskID, job, finalStatus, execErr)
}

// loadJobAndAnnounce loads the task's job metadata (needed for enriched events,
// hook initialization, and worktree lock release), runs optional hook
// initialization, and publishes the task.started event. It returns ok=false —
// after recording the failure — when the job cannot be loaded.
func (r *Runner) loadJobAndAnnounce(taskCtx context.Context, taskID string) (TaskJob, bool) {
	job, loadErr := r.loadTaskJob(taskCtx, taskID)
	if loadErr != nil {
		r.logger.Error().Err(loadErr).Str("task_id", taskID).Msg("runner: failed to load task job")
		r.markTaskFailed(taskCtx, taskID, loadErr.Error())
		r.writeLog(taskCtx, taskID, LogEntry{Level: "error", Message: "failed to load task job: " + loadErr.Error(), Source: "runner"})
		r.publishTaskEvent(taskCtx, EventTaskFailed, taskID, "failed", loadErr.Error(), loadErr.Error(), nil)
		return TaskJob{}, false
	}

	// Optional: run hook initialization if executor supports it (Task 5.3).
	// Failure marks crash_recovery=degraded but does NOT abort execution.
	if hi, ok := r.executor.(HookInitializer); ok {
		if degraded, reason := hi.InitHooks(taskCtx, job); degraded {
			r.markCrashRecoveryDegraded(taskCtx, taskID, reason)
		}
	}

	r.publishTaskEvent(taskCtx, EventTaskStarted, taskID, "running", "", "", &job)
	r.writeLog(taskCtx, taskID, LogEntry{Level: "info", Message: "task started", Source: "runner"})
	return job, true
}

// recoverTaskPanic recovers from a panic in executeTask, marks the task failed,
// and publishes a failure event. It is intended to be used with defer.
func (r *Runner) recoverTaskPanic(taskCtx context.Context, taskID string) {
	rec := recover()
	if rec == nil {
		return
	}
	r.logger.Error().
		Interface("panic", rec).
		Str("task_id", taskID).
		Msg("runner: task panicked")
	r.markTaskFailed(taskCtx, taskID, fmt.Sprintf("panic: %v", rec))
	r.publishTaskEvent(taskCtx, EventTaskFailed, taskID, "failed", fmt.Sprintf("panic: %v", rec), "", nil)
}

// runStubTask handles execution when no executor is wired (test/dev mode): it
// simulates a brief run and marks the task completed.
func (r *Runner) runStubTask(taskCtx context.Context, taskID string, job TaskJob) {
	r.logger.Info().Str("task_id", taskID).Msg("runner: executing task (stub)")
	r.writeLog(taskCtx, taskID, LogEntry{Level: "info", Message: "executing task (stub mode)", Source: "runner"})
	time.Sleep(100 * time.Millisecond)
	r.markTaskCompleted(taskCtx, taskID)
	r.writeLog(taskCtx, taskID, LogEntry{Level: "info", Message: "task completed", Source: "runner"})
	r.publishTaskEvent(taskCtx, EventTaskCompleted, taskID, "completed", "", "", &job)
}

// handleExecutionOutcome records the terminal task state and publishes the
// matching lifecycle event based on the status returned by the executor.
func (r *Runner) handleExecutionOutcome(taskCtx context.Context, taskID string, job TaskJob, finalStatus string, execErr error) {
	switch finalStatus {
	case "completed":
		r.markTaskCompleted(taskCtx, taskID)
		r.writeLog(taskCtx, taskID, LogEntry{Level: "info", Message: "task completed", Source: "runner"})
		r.publishTaskEvent(taskCtx, EventTaskCompleted, taskID, "completed", "", "", &job)
	case "awaiting_approval":
		r.markTaskStatus(taskCtx, taskID, "awaiting_approval")
		// Remove from active set — worker is returning. RequeueForResume() re-adds on resume.
		activeKey := r.cfg.Redis.KeyPrefix + "active"
		if err := cache.SetRemoveMember(taskCtx, r.redis, activeKey, taskID); err != nil {
			r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to remove awaiting_approval task from active set")
		}
		r.writeLog(taskCtx, taskID, LogEntry{Level: "info", Message: "task awaiting approval", Source: "runner"})
		r.publishTaskEvent(taskCtx, EventTaskApprovalRequired, taskID, "awaiting_approval", "", "", &job)
	default:
		msg := "task execution failed"
		if execErr != nil {
			msg = execErr.Error()
		} else if finalStatus != "" {
			msg = fmt.Sprintf("task ended with status: %s", finalStatus)
		}
		r.markTaskFailed(taskCtx, taskID, msg)
		r.writeLog(taskCtx, taskID, LogEntry{Level: "error", Message: "task failed: " + msg, Source: "runner"})
		r.publishTaskEvent(taskCtx, EventTaskFailed, taskID, "failed", msg, msg, &job)
	}
}

// publishTaskEvent publishes a task lifecycle event, enriching it with job
// metadata when a job is available, and logs a warning if publishing fails.
// It is a no-op when no event bus is configured.
func (r *Runner) publishTaskEvent(ctx context.Context, eventType, taskID, status, message, errMsg string, job *TaskJob) {
	if r.events == nil {
		return
	}
	ev := TaskEvent{
		Type:    eventType,
		TaskID:  taskID,
		Status:  status,
		Message: message,
		Error:   errMsg,
	}
	if job != nil {
		ev.Workspace = job.Workspace
		ev.Agent = job.Agent
		ev.Model = job.Model
		ev.Branch = job.Branch
		ev.Template = job.Template
		ev.Description = job.Description
	}
	if pubErr := r.events.Publish(ctx, ev); pubErr != nil {
		r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish " + eventType + " event")
	}
}

// markTaskRunning updates the task hash to status=running with a started_at timestamp.
// Returns an error if the Redis write fails; callers should abort execution on failure
// to avoid running a task whose state cannot be recorded.
func (r *Runner) markTaskRunning(ctx context.Context, taskID string) error {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{
		{"status", "running"},
		{"started_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Error().Err(err).Str("task_id", taskID).Msg("runner: failed to mark task running")
		return fmt.Errorf("mark task running: %w", err)
	}
	return nil
}

// markTaskCompleted updates the task hash to status=completed with a completed_at timestamp.
func (r *Runner) markTaskCompleted(ctx context.Context, taskID string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{
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

	// Release worktree lock if repo_path is available.
	r.releaseWorktreeLock(ctx, taskID)
}

// finalizeCanceledTask writes the terminal state for a task whose context was canceled.
// It uses a fresh background context because taskCtx is already done.
func (r *Runner) finalizeCanceledTask(taskID, hashKey string) {
	bgCtx := context.Background()
	finalStatVal, loaded := r.canceledTasks.LoadAndDelete(taskID)
	status := "canceled"
	if loaded {
		status, _ = finalStatVal.(string)
	}
	termPairs := [][2]any{
		{"status", status},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(bgCtx, r.redis, hashKey, termPairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to update status on cancel")
	}
	activeKey := r.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetRemoveMember(bgCtx, r.redis, activeKey, taskID); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to remove task from active set on cancel")
	}
	// Release worktree lock (best-effort).
	r.releaseWorktreeLock(bgCtx, taskID)
	// Choose the appropriate event type based on final status.
	if r.events != nil {
		evType := EventTaskCancelled
		switch status {
		case "abandoned":
			evType = EventTaskAbandoned
		case "paused":
			evType = EventTaskPaused
		}
		if pubErr := r.events.Publish(bgCtx, TaskEvent{
			Type:    evType,
			TaskID:  taskID,
			Status:  status,
			Message: "task " + status,
		}); pubErr != nil {
			r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish cancel event")
		}
	}
}

// handleTaskTimeout checks whether a context error is a deadline-exceeded timeout
// (not an explicit cancellation). If so, it marks the task failed and returns true.
//
//nolint:contextcheck // Intentional: taskCtx is expired, so we use background context for cleanup operations.
func (r *Runner) handleTaskTimeout(taskCtx context.Context, taskID string, taskTimeout time.Duration) bool {
	if !errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	if _, wasCanceled := r.canceledTasks.Load(taskID); wasCanceled {
		return false
	}
	bgCtx := context.Background()
	msg := fmt.Sprintf("task exceeded timeout of %s", taskTimeout)
	r.markTaskFailed(bgCtx, taskID, msg)
	r.writeLog(bgCtx, taskID, LogEntry{Level: "error", Message: msg, Source: "runner"})
	if r.events != nil {
		if pubErr := r.events.Publish(bgCtx, TaskEvent{
			Type:    EventTaskFailed,
			TaskID:  taskID,
			Status:  "failed",
			Message: msg,
		}); pubErr != nil {
			r.logger.Warn().Err(pubErr).Str("task_id", taskID).Msg("runner: failed to publish task.failed event")
		}
	}
	return true
}

// writeLog writes a single LogEntry to the task's log stream.
// Errors are logged but not propagated — log stream failures are non-fatal.
func (r *Runner) writeLog(ctx context.Context, taskID string, entry LogEntry) {
	if r.logWriter == nil {
		return
	}
	if err := r.logWriter.Write(ctx, taskID, entry); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to write log entry")
	}
}

// markTaskFailed updates the task hash to status=failed with an error message.
func (r *Runner) markTaskFailed(ctx context.Context, taskID, errMsg string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{
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

	// Release worktree lock if repo_path is available.
	r.releaseWorktreeLock(ctx, taskID)
}

// markCrashRecoveryDegraded sets crash_recovery=degraded in the task hash.
// The task continues to execute, but crash recovery is unavailable for this run.
func (r *Runner) markCrashRecoveryDegraded(ctx context.Context, taskID, reason string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]any{
		{"crash_recovery", "degraded"},
		{"crash_recovery_reason", reason},
	}
	if err := cache.HashMapSet(ctx, r.redis, hashKey, pairs); err != nil {
		r.logger.Warn().Err(err).Str("task_id", taskID).Msg("runner: failed to mark crash_recovery degraded")
	}
	r.logger.Warn().
		Str("task_id", taskID).
		Str("reason", reason).
		Msg("runner: task running with degraded crash recovery (hook init failed)")
}

// releaseWorktreeLock reads the repo_path from Redis and releases the worktree lock.
// Best-effort: logs but does not propagate errors.
func (r *Runner) releaseWorktreeLock(ctx context.Context, taskID string) {
	hashKey := r.cfg.Redis.KeyPrefix + "task:" + taskID
	vals, err := cache.HashMapGet(ctx, r.redis, hashKey, "repo_path")
	if err != nil || len(vals) == 0 || vals[0] == "" {
		return
	}
	repoPath := vals[0]
	if relErr := lifecycle.ReleaseWorktreeRedisLock(ctx, r.redis, r.cfg.Redis.KeyPrefix, repoPath, taskID); relErr != nil {
		r.logger.Warn().Err(relErr).Str("task_id", taskID).Str("repo_path", repoPath).Msg("runner: failed to release worktree lock")
	}
}
