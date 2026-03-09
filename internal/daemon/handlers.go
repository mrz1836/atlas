package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	cache "github.com/mrz1836/go-cache"
)

// Sentinel errors for handlers.
var (
	errDescriptionRequired = errors.New("description is required")
	errTaskIDRequired      = errors.New("task_id is required")
	errTaskNotFound        = errors.New("task not found")
	errInvalidPriority     = errors.New("invalid priority: must be urgent, normal, or low")
)

// setupRouter registers all JSON-RPC method handlers on the given Router.
func (d *Daemon) setupRouter(r *Router) {
	r.Register(MethodDaemonPing, d.handleDaemonPing)
	r.Register(MethodDaemonStatus, d.handleDaemonStatus)
	r.Register(MethodDaemonShutdown, d.handleDaemonShutdown)

	r.Register(MethodTaskSubmit, d.handleTaskSubmit)
	r.Register(MethodTaskStatus, d.handleTaskStatus)
	r.Register(MethodTaskList, d.handleTaskList)
	r.Register(MethodTaskApprove, stubHandler("task.approve"))
	r.Register(MethodTaskReject, stubHandler("task.reject"))
	r.Register(MethodTaskResume, stubHandler("task.resume"))
	r.Register(MethodTaskAbandon, stubHandler("task.abandon"))
	r.Register(MethodTaskCancel, stubHandler("task.cancel"))

	r.Register(MethodQueueStats, d.handleQueueStats)
	r.Register(MethodQueueList, d.handleQueueList)
	r.Register(MethodQueueClear, d.handleQueueClear)

	r.Register(MethodEventsSubscribe, d.handleEventsSubscribe)
}

// errNotImplemented is returned by Phase-5 stub handlers.
var errNotImplemented = errors.New("not implemented: deferred to Phase 5")

// stubHandler returns a HandlerFunc that always returns errNotImplemented.
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
	if err := cache.HashMapSet(ctx, d.redis, hashKey, pairs); err != nil {
		return nil, fmt.Errorf("store task hash: %w", err)
	}

	// Add to the priority queue.
	if err := d.queue.Submit(ctx, taskID, priority); err != nil {
		return nil, fmt.Errorf("queue submit: %w", err)
	}

	// Track in active set.
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	if err := cache.SetAdd(ctx, d.redis, activeKey, taskID); err != nil {
		d.logger.Warn().Err(err).Str("task_id", taskID).Msg("handlers: failed to add to active set")
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

	fields := []interface{}{"id", "status", "priority", "submitted_at", "started_at", "completed_at", "error"}
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

	// Retrieve active task IDs from the set.
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	members, err := cache.SetMembers(ctx, d.redis, activeKey)
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
		fields := []interface{}{"id", "status", "priority", "submitted_at", "started_at", "completed_at", "error"}
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
		}
		if req.Status == "" || t.Status == req.Status {
			tasks = append(tasks, t)
		}
	}

	return TaskListResponse{Tasks: tasks, Total: len(tasks)}, nil
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

// handleEventsSubscribe stubs the streaming subscription for Phase 5.
// Returns an "accepted" response; persistent streaming is deferred.
func (d *Daemon) handleEventsSubscribe(_ context.Context, _ json.RawMessage) (interface{}, error) {
	return map[string]interface{}{"accepted": true, "note": "streaming deferred to Phase 5"}, nil
}

// -- helpers --

// safeIndex returns vals[i] or "" if i is out of range.
func safeIndex(vals []string, i int) string {
	if i < len(vals) {
		return vals[i]
	}
	return ""
}
