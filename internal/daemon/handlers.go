package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	cache "github.com/mrz1836/go-cache"

	"github.com/mrz1836/atlas/internal/workspace"
)

// Sentinel errors for handlers.
var (
	errDescriptionRequired  = errors.New("description is required")
	errTaskIDRequired       = errors.New("task_id is required")
	errTaskNotFound         = errors.New("task not found")
	errInvalidPriority      = errors.New("invalid priority: must be urgent, normal, or low")
	errTaskNotResumable     = errors.New("task is not in a resumable state")
	errTaskNotAwaitApproval = errors.New("task is not awaiting approval")
	errRunnerNotInitialized = errors.New("runner not initialized")
	errWorkspaceRequired    = errors.New("workspace is required")
	errRepoPathRequired     = errors.New("repo_path is required")
)

// setupRouter registers all JSON-RPC method handlers on the given Router.
func (d *Daemon) setupRouter(r *Router) {
	r.Register(MethodDaemonPing, d.handleDaemonPing)
	r.Register(MethodDaemonStatus, d.handleDaemonStatus)
	r.Register(MethodDaemonShutdown, d.handleDaemonShutdown)

	r.Register(MethodTaskSubmit, d.handleTaskSubmit)
	r.Register(MethodTaskStatus, d.handleTaskStatus)
	r.Register(MethodTaskList, d.handleTaskList)
	r.Register(MethodTaskApprove, d.handleTaskApprove)
	r.Register(MethodTaskReject, d.handleTaskReject)
	r.Register(MethodTaskResume, d.handleTaskResume)
	r.Register(MethodTaskAbandon, d.handleTaskAbandon)
	r.Register(MethodTaskCancel, d.handleTaskCancel)

	r.Register(MethodQueueStats, d.handleQueueStats)
	r.Register(MethodQueueList, d.handleQueueList)
	r.Register(MethodQueueClear, d.handleQueueClear)

	r.Register(MethodEventsSubscribe, d.handleEventsSubscribe)

	r.Register(MethodWorkspaceDestroy, d.handleWorkspaceDestroy)
	r.Register(MethodTaskPause, d.handleTaskPause)
}

// errNotImplemented is kept for test compatibility (stubHandler is used in tests).
var errNotImplemented = errors.New("not implemented: deferred to Phase 5")

// stubHandler returns a HandlerFunc that always returns errNotImplemented.
// Retained for use in tests.
func stubHandler(method string) HandlerFunc {
	return func(_ context.Context, _ json.RawMessage) (interface{}, error) {
		return nil, fmt.Errorf("%w: %s", errNotImplemented, method)
	}
}

// -- daemon.* --

func (d *Daemon) handleDaemonPing(_ context.Context, _ json.RawMessage) (interface{}, error) {
	return DaemonPingResponse{Alive: true, Version: daemonVersion}, nil
}

func (d *Daemon) handleDaemonStatus(ctx context.Context, _ json.RawMessage) (interface{}, error) {
	return d.Health(ctx)
}

func (d *Daemon) handleDaemonShutdown(_ context.Context, _ json.RawMessage) (interface{}, error) {
	go func() { //nolint:gosec,contextcheck // G118: background ctx is intentional — request ctx will be canceled before shutdown completes
		// Brief delay so the response can be flushed to the client before shutdown.
		time.Sleep(100 * time.Millisecond)
		if err := d.Stop(context.Background()); err != nil {
			d.logger.Error().Err(err).Msg("daemon: shutdown via RPC failed")
		}
	}()
	return map[string]interface{}{"ok": true}, nil
}

// -- task.* --

func (d *Daemon) handleTaskSubmit(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskSubmitRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.Description == "" {
		return nil, errDescriptionRequired
	}

	taskID := uuid.New().String()
	priority := Priority(req.Priority)
	switch priority {
	case PriorityUrgent, PriorityNormal, PriorityLow:
		// valid
	case "":
		priority = PriorityNormal
	default:
		return nil, fmt.Errorf("%w: got %q", errInvalidPriority, req.Priority)
	}

	// Store task metadata in a Redis hash.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"id", taskID},
		{"description", req.Description},
		{"template", req.Template},
		{"status", "queued"},
		{"priority", string(priority)},
		{"submitted_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if req.Workspace != "" {
		pairs = append(pairs, [2]interface{}{"workspace", req.Workspace})
	}
	if req.Branch != "" {
		pairs = append(pairs, [2]interface{}{"branch", req.Branch})
	}
	if req.RepoPath != "" {
		pairs = append(pairs, [2]interface{}{"repo_path", req.RepoPath})
	}
	if req.Agent != "" {
		pairs = append(pairs, [2]interface{}{"agent", req.Agent})
	}
	if req.Model != "" {
		pairs = append(pairs, [2]interface{}{"model", req.Model})
	}
	if req.TargetBranch != "" {
		pairs = append(pairs, [2]interface{}{"target_branch", req.TargetBranch})
	}
	if req.UseLocal {
		pairs = append(pairs, [2]interface{}{"use_local", "true"})
	}
	if req.Verify {
		pairs = append(pairs, [2]interface{}{"verify", "true"})
	}
	if req.NoVerify {
		pairs = append(pairs, [2]interface{}{"no_verify", "true"})
	}
	if err := cache.HashMapSet(ctx, d.redis, hashKey, pairs); err != nil {
		return nil, fmt.Errorf("store task hash: %w", err)
	}

	// Track in persistent tasks set so the task remains visible in listings
	// even after it reaches a terminal state (completed/failed/canceled).
	tasksKey := d.cfg.Redis.KeyPrefix + "tasks"
	if err := cache.SetAdd(ctx, d.redis, tasksKey, taskID); err != nil {
		return nil, fmt.Errorf("track in tasks set: %w", err)
	}

	// Track in active set BEFORE queuing so the task is always visible once submitted.
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetAdd(ctx, d.redis, activeKey, taskID); err != nil {
		return nil, fmt.Errorf("track in active set: %w", err)
	}

	// Add to the priority queue; roll back the active-set entry on failure.
	if err := d.queue.Submit(ctx, taskID, priority); err != nil {
		_ = cache.SetRemoveMember(ctx, d.redis, activeKey, taskID)
		return nil, fmt.Errorf("queue submit: %w", err)
	}

	// Publish event.
	if err := d.events.Publish(ctx, TaskEvent{
		Type:   EventTaskSubmitted,
		TaskID: taskID,
		Status: "queued",
	}); err != nil {
		d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: failed to publish task.submitted event")
	}

	return TaskSubmitResponse{TaskID: taskID, Status: "queued"}, nil
}

func (d *Daemon) handleTaskStatus(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskStatusRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	hashKey := d.cfg.Redis.KeyPrefix + "task:" + req.TaskID

	fields := []interface{}{
		"id", "status", "priority", "submitted_at", "started_at", "completed_at", "error",
		"description", "workspace", "agent", "model", "branch", "template",
	}
	vals, err := cache.HashMapGet(ctx, d.redis, hashKey, fields...)
	if err != nil {
		return nil, fmt.Errorf("read task hash: %w", err)
	}

	// HashMapGet returns values in the same order as keys.
	resp := TaskStatusResponse{
		TaskID:      safeIndex(vals, 0),
		Status:      safeIndex(vals, 1),
		Priority:    safeIndex(vals, 2),
		SubmittedAt: safeIndex(vals, 3),
		StartedAt:   safeIndex(vals, 4),
		CompletedAt: safeIndex(vals, 5),
		Error:       safeIndex(vals, 6),
		Description: safeIndex(vals, 7),
		Workspace:   safeIndex(vals, 8),
		Agent:       safeIndex(vals, 9),
		Model:       safeIndex(vals, 10),
		Branch:      safeIndex(vals, 11),
		Template:    safeIndex(vals, 12),
	}
	if resp.TaskID == "" {
		return nil, fmt.Errorf("task %s: %w", req.TaskID, errTaskNotFound)
	}
	return resp, nil
}

func (d *Daemon) handleTaskList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskListRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	// Retrieve all task IDs from the persistent tasks set (includes terminal-state tasks).
	tasksKey := d.cfg.Redis.KeyPrefix + "tasks"
	members, err := cache.SetMembers(ctx, d.redis, tasksKey)
	if err != nil {
		return nil, fmt.Errorf("list active tasks: %w", err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	var tasks []TaskStatusResponse
	for _, taskID := range members {
		if len(tasks) >= limit {
			break
		}
		hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
		fields := []interface{}{
			"id", "status", "priority", "submitted_at", "started_at", "completed_at", "error",
			"description", "workspace", "agent", "model", "branch", "template",
		}
		vals, err := cache.HashMapGet(ctx, d.redis, hashKey, fields...)
		if err != nil {
			d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: failed to read task hash during list")
			continue
		}
		t := TaskStatusResponse{
			TaskID:      safeIndex(vals, 0),
			Status:      safeIndex(vals, 1),
			Priority:    safeIndex(vals, 2),
			SubmittedAt: safeIndex(vals, 3),
			StartedAt:   safeIndex(vals, 4),
			CompletedAt: safeIndex(vals, 5),
			Error:       safeIndex(vals, 6),
			Description: safeIndex(vals, 7),
			Workspace:   safeIndex(vals, 8),
			Agent:       safeIndex(vals, 9),
			Model:       safeIndex(vals, 10),
			Branch:      safeIndex(vals, 11),
			Template:    safeIndex(vals, 12),
		}
		if req.Status == "" || t.Status == req.Status {
			tasks = append(tasks, t)
		}
	}

	return TaskListResponse{Tasks: tasks, Total: len(tasks)}, nil
}

func (d *Daemon) handleTaskCancel(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskCancelRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	// If the task is actively running, cancel its context.
	// executeTask will set the final "canceled" status when it detects context cancellation.
	wasRunning := false
	if d.runner != nil {
		wasRunning = d.runner.CancelTask(req.TaskID)
	}

	if !wasRunning {
		// Task is queued or not yet started — write the terminal state to Redis directly.
		if err := d.cancelQueuedTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{"ok": true}, nil
}

// cancelQueuedTask marks a non-running task as canceled in Redis and removes it from
// the active set. This path is taken when the task has not yet been picked up by a worker.
func (d *Daemon) cancelQueuedTask(ctx context.Context, taskID string) error {
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "canceled"},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, d.redis, hashKey, pairs); err != nil {
		return fmt.Errorf("mark task canceled: %w", err)
	}
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetRemoveMember(ctx, d.redis, activeKey, taskID); err != nil {
		d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: failed to remove canceled task from active set")
	}
	return nil
}

func (d *Daemon) handleTaskAbandon(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskAbandonRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	// If the task is running, signal it to stop; executeTask handles Redis cleanup.
	wasRunning := false
	if d.runner != nil {
		wasRunning = d.runner.AbandonRunningTask(req.TaskID)
	}

	if !wasRunning {
		if err := d.abandonQueuedTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{"ok": true}, nil
}

// abandonQueuedTask notifies the executor (if available) and marks the task as abandoned in Redis.
// This path is taken when the task is not currently running.
func (d *Daemon) abandonQueuedTask(ctx context.Context, taskID string) error {
	// Best-effort: notify the engine task store if the task has an associated engine task.
	if d.executor != nil && d.runner != nil {
		d.tryAbandonEngineTask(ctx, taskID)
	}
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "abandoned"},
		{"completed_at", time.Now().UTC().Format(time.RFC3339)},
	}
	if err := cache.HashMapSet(ctx, d.redis, hashKey, pairs); err != nil {
		return fmt.Errorf("mark task abandoned: %w", err)
	}
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetRemoveMember(ctx, d.redis, activeKey, taskID); err != nil {
		d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: failed to remove abandoned task from active set")
	}
	return nil
}

// tryAbandonEngineTask loads the task job and signals the engine to mark it abandoned.
// Errors are logged but not propagated — this is a best-effort cleanup step.
func (d *Daemon) tryAbandonEngineTask(ctx context.Context, taskID string) {
	job, loadErr := d.runner.loadTaskJob(ctx, taskID)
	if loadErr != nil || job.EngineTaskID == "" {
		return
	}
	if err := d.executor.Abandon(ctx, job, "abandoned by user"); err != nil {
		d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: executor abandon failed")
	}
}

func (d *Daemon) handleTaskResume(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskResumeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	// Validate that the task is in a resumable state.
	status, statusErr := d.fetchTaskStatus(ctx, req.TaskID)
	if statusErr != nil {
		return nil, statusErr
	}
	if !isResumableStatus(status) {
		return nil, fmt.Errorf("task %s: %w (status=%s)", req.TaskID, errTaskNotResumable, status)
	}

	if d.runner == nil {
		return nil, errRunnerNotInitialized
	}
	if err := d.runner.RequeueForResume(ctx, req.TaskID, "", ""); err != nil {
		return nil, fmt.Errorf("requeue task: %w", err)
	}
	return map[string]interface{}{"ok": true}, nil
}

func (d *Daemon) handleTaskApprove(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskApproveRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	status, statusErr := d.fetchTaskStatus(ctx, req.TaskID)
	if statusErr != nil {
		return nil, statusErr
	}
	if status != "awaiting_approval" {
		return nil, fmt.Errorf("task %s: %w (status=%s)", req.TaskID, errTaskNotAwaitApproval, status)
	}

	if err := d.runner.RequeueForResume(ctx, req.TaskID, "approve", ""); err != nil {
		return nil, fmt.Errorf("requeue task for approval: %w", err)
	}
	return map[string]interface{}{"ok": true}, nil
}

func (d *Daemon) handleTaskReject(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskRejectRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	status, statusErr := d.fetchTaskStatus(ctx, req.TaskID)
	if statusErr != nil {
		return nil, statusErr
	}
	if status != "awaiting_approval" {
		return nil, fmt.Errorf("task %s: %w (status=%s)", req.TaskID, errTaskNotAwaitApproval, status)
	}

	if err := d.runner.RequeueForResume(ctx, req.TaskID, "reject", req.Feedback); err != nil {
		return nil, fmt.Errorf("requeue task for rejection: %w", err)
	}
	return map[string]interface{}{"ok": true}, nil
}

func (d *Daemon) handleTaskPause(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req TaskPauseRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.TaskID == "" {
		return nil, errTaskIDRequired
	}

	// If the task is actively running, pause it via context cancellation.
	// The runner stores "paused" so executeTask sets the correct final status.
	wasRunning := false
	if d.runner != nil {
		wasRunning = d.runner.PauseRunningTask(req.TaskID)
	}

	if !wasRunning {
		// Task is queued — write paused state directly to Redis.
		if err := d.pauseQueuedTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"ok": true}, nil
}

// pauseQueuedTask marks a non-running task as paused in Redis and removes it from
// the active set. Paused tasks are resumable via task.resume.
func (d *Daemon) pauseQueuedTask(ctx context.Context, taskID string) error {
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"status", "paused"},
	}
	if err := cache.HashMapSet(ctx, d.redis, hashKey, pairs); err != nil {
		return fmt.Errorf("mark task paused: %w", err)
	}
	return nil
}

// -- queue.* --

func (d *Daemon) handleQueueStats(ctx context.Context, _ json.RawMessage) (interface{}, error) {
	stats, err := d.queue.Stats(ctx)
	if err != nil {
		return nil, err
	}
	return QueueStatsResponse{
		Urgent: int(stats.Urgent),
		Normal: int(stats.Normal),
		Low:    int(stats.Low),
		Total:  int(stats.Total),
	}, nil
}

func (d *Daemon) handleQueueList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req QueueListRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	var prio *Priority
	if req.Priority != "" {
		p := Priority(req.Priority)
		prio = &p
	}

	entries, err := d.queue.List(ctx, prio)
	if err != nil {
		return nil, err
	}

	// Apply offset/limit with a safety cap so a huge queue cannot OOM the daemon.
	const maxQueueListLimit = 500
	limit := req.Limit
	if limit <= 0 || limit > maxQueueListLimit {
		limit = maxQueueListLimit
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(entries) {
		offset = len(entries)
	}
	entries = entries[offset:]
	if len(entries) > limit {
		entries = entries[:limit]
	}

	resp := QueueListResponse{Total: len(entries)}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, QueueEntryResponse{
			TaskID:   e.TaskID,
			Priority: string(e.Priority),
			Score:    e.Score,
		})
	}
	return resp, nil
}

func (d *Daemon) handleQueueClear(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req QueueClearRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
	}

	var prio *Priority
	if req.Priority != "" {
		p := Priority(req.Priority)
		prio = &p
	}

	if err := d.queue.Clear(ctx, prio); err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true}, nil
}

// -- events.* --

// handleEventsSubscribe returns the Redis channel name and log key prefix so clients
// can subscribe directly to Redis pub/sub and tail log streams without routing
// through the JSON-RPC socket.
func (d *Daemon) handleEventsSubscribe(_ context.Context, _ json.RawMessage) (interface{}, error) {
	return EventSubscribeResponse{
		Channel:   defaultEventsChannel,
		LogPrefix: d.cfg.Redis.KeyPrefix + logKeyPrefix,
	}, nil
}

// -- workspace.* --

// handleWorkspaceDestroy destroys a workspace (removes worktree + branch).
// It uses workspace.DefaultManager so all the NFR18 "always succeed" semantics are preserved.
func (d *Daemon) handleWorkspaceDestroy(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req WorkspaceDestroyRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if req.Workspace == "" {
		return nil, errWorkspaceRequired
	}
	if req.RepoPath == "" {
		return nil, errRepoPathRequired
	}

	wsStore, err := workspace.NewRepoScopedFileStore(req.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, req.RepoPath, d.logger)
	if err != nil {
		return nil, fmt.Errorf("worktree runner: %w", err)
	}
	mgr := workspace.NewManager(wsStore, wtRunner, d.logger)
	if err = mgr.Destroy(ctx, req.Workspace); err != nil {
		return nil, fmt.Errorf("destroy workspace: %w", err)
	}
	return map[string]interface{}{"ok": true}, nil
}

// -- helpers --

// safeIndex returns vals[i] or "" if i is out of range.
func safeIndex(vals []string, i int) string {
	if i < len(vals) {
		return vals[i]
	}
	return ""
}

// fetchTaskStatus reads the status field for a task from Redis.
// Returns errTaskNotFound if the task hash does not exist.
func (d *Daemon) fetchTaskStatus(ctx context.Context, taskID string) (string, error) {
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	vals, err := cache.HashMapGet(ctx, d.redis, hashKey, "status")
	if err != nil {
		return "", fmt.Errorf("read task status: %w", err)
	}
	status := safeIndex(vals, 0)
	if status == "" {
		return "", fmt.Errorf("task %s: %w", taskID, errTaskNotFound)
	}
	return status, nil
}

// isResumableStatus returns true for statuses from which a task can be resumed.
func isResumableStatus(status string) bool {
	switch status {
	case "awaiting_approval", "failed", "interrupted", "paused":
		return true
	}
	return false
}
