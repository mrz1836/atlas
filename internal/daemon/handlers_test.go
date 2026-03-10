package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- daemon.ping --

func TestHandlerDaemonPing(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	result, err := d.handleDaemonPing(context.Background(), nil)
	require.NoError(t, err)

	resp, ok := result.(DaemonPingResponse)
	require.True(t, ok)
	assert.True(t, resp.Alive)
	assert.Equal(t, daemonVersion, resp.Version)
}

// -- daemon.status --

func TestHandlerDaemonStatus(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.startedAt = time.Now().Add(-30 * time.Second)

	result, err := d.handleDaemonStatus(context.Background(), nil)
	require.NoError(t, err)

	resp, ok := result.(*DaemonStatusResponse)
	require.True(t, ok)
	assert.Positive(t, resp.PID)
	assert.NotEmpty(t, resp.StartedAt)
	assert.True(t, resp.RedisAlive)
}

// -- daemon.shutdown --

func TestHandlerDaemonShutdown(t *testing.T) {
	t.Parallel()
	d, _, _ := newTestDaemonWithRedis(t)
	// Don't defer cleanup — Stop() will close redis.

	result, err := d.handleDaemonShutdown(context.Background(), nil)
	require.NoError(t, err)

	m, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, m["ok"])

	// Wait for the async goroutine to call Stop (100ms delay + margin).
	select {
	case <-d.stopCh:
		// Daemon stopped as expected.
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop after shutdown handler")
	}
}

// -- task.submit --

func TestHandlerTaskSubmit_Valid(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskSubmitRequest{
		Description: "fix the thing",
		Template:    "bug",
		Priority:    "urgent",
		Workspace:   "ws1",
		Branch:      "feat/x",
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(context.Background(), params)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.TaskID)
	assert.Equal(t, "queued", resp.Status)
}

func TestHandlerTaskSubmit_DefaultPriority(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskSubmitRequest{
		Description: "task with default priority",
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(context.Background(), params)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.TaskID)
}

func TestHandlerTaskSubmit_MissingDescription(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskSubmitRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskSubmit(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errDescriptionRequired)
}

func TestHandlerTaskSubmit_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskSubmit(context.Background(), json.RawMessage(`{invalid`))
	require.Error(t, err)
}

func TestHandlerTaskSubmit_LowPriority(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskSubmitRequest{
		Description: "low prio task",
		Priority:    string(PriorityLow),
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(context.Background(), params)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok)
	assert.Equal(t, "queued", resp.Status)
}

func TestHandlerTaskSubmit_InvalidPriority(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskSubmitRequest{
		Description: "task with bad priority",
		Priority:    "critical",
	})
	require.NoError(t, err)

	_, err = d.handleTaskSubmit(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidPriority)
}

// -- task.status --

func TestHandlerTaskStatus_Valid(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit a task first.
	submitParams, err := json.Marshal(TaskSubmitRequest{Description: "status test task"})
	require.NoError(t, err)
	submitResult, err := d.handleTaskSubmit(ctx, submitParams)
	require.NoError(t, err)
	taskID := submitResult.(TaskSubmitResponse).TaskID

	// Now query its status.
	statusParams, err := json.Marshal(TaskStatusRequest{TaskID: taskID})
	require.NoError(t, err)
	result, err := d.handleTaskStatus(ctx, statusParams)
	require.NoError(t, err)

	resp, ok := result.(TaskStatusResponse)
	require.True(t, ok)
	assert.Equal(t, taskID, resp.TaskID)
	assert.Equal(t, "queued", resp.Status)
	assert.NotEmpty(t, resp.SubmittedAt)
}

func TestHandlerTaskStatus_MissingID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskStatusRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskStatus(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskStatus_NotFound(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskStatusRequest{TaskID: "nonexistent-task"})
	require.NoError(t, err)

	_, err = d.handleTaskStatus(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotFound)
}

func TestHandlerTaskStatus_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskStatus(context.Background(), json.RawMessage(`bad json`))
	require.Error(t, err)
}

// -- task.list --

func TestHandlerTaskList_Empty(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	result, err := d.handleTaskList(context.Background(), nil)
	require.NoError(t, err)

	resp, ok := result.(TaskListResponse)
	require.True(t, ok)
	assert.Equal(t, 0, resp.Total)
}

func TestHandlerTaskList_WithTasks(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit three tasks.
	for i := 0; i < 3; i++ {
		params, err := json.Marshal(TaskSubmitRequest{Description: "task"})
		require.NoError(t, err)
		_, err = d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
	}

	result, err := d.handleTaskList(ctx, nil)
	require.NoError(t, err)

	resp, ok := result.(TaskListResponse)
	require.True(t, ok)
	assert.Equal(t, 3, resp.Total)
}

func TestHandlerTaskList_TerminalTasksVisible(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit two tasks.
	var taskIDs [2]string
	for i := range taskIDs {
		params, err := json.Marshal(TaskSubmitRequest{Description: "terminal-test"})
		require.NoError(t, err)
		result, err := d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
		taskIDs[i] = result.(TaskSubmitResponse).TaskID
	}

	// Simulate what markTaskCompleted/markTaskFailed does:
	// update the hash status and remove from the active set.
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	for i, taskID := range taskIDs {
		hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
		status := "completed"
		if i == 1 {
			status = "failed"
		}
		pairs := [][2]interface{}{{"status", status}}
		require.NoError(t, cache.HashMapSet(ctx, d.redis, hashKey, pairs))
		require.NoError(t, cache.SetRemoveMember(ctx, d.redis, activeKey, taskID))
	}

	// Both tasks must still appear in the task list despite being removed from active set.
	result, err := d.handleTaskList(ctx, nil)
	require.NoError(t, err)
	resp := result.(TaskListResponse)
	assert.Equal(t, 2, resp.Total)

	// Filter by "failed" should return exactly one.
	listParams, err := json.Marshal(TaskListRequest{Status: "failed"})
	require.NoError(t, err)
	result2, err := d.handleTaskList(ctx, listParams)
	require.NoError(t, err)
	resp2 := result2.(TaskListResponse)
	assert.Equal(t, 1, resp2.Total)
	assert.Equal(t, "failed", resp2.Tasks[0].Status)
}

func TestHandlerTaskList_StatusFilter(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit a task (status=queued).
	params, err := json.Marshal(TaskSubmitRequest{Description: "filterable"})
	require.NoError(t, err)
	_, err = d.handleTaskSubmit(ctx, params)
	require.NoError(t, err)

	// Filter by "queued" — should find it.
	listParams, err := json.Marshal(TaskListRequest{Status: "queued", Limit: 50})
	require.NoError(t, err)
	result, err := d.handleTaskList(ctx, listParams)
	require.NoError(t, err)

	resp := result.(TaskListResponse)
	assert.GreaterOrEqual(t, resp.Total, 1)

	// Filter by "running" — should find nothing.
	listParams2, err := json.Marshal(TaskListRequest{Status: "running"})
	require.NoError(t, err)
	result2, err := d.handleTaskList(ctx, listParams2)
	require.NoError(t, err)

	resp2 := result2.(TaskListResponse)
	assert.Equal(t, 0, resp2.Total)
}

func TestHandlerTaskList_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskList(context.Background(), json.RawMessage(`notjson`))
	require.Error(t, err)
}

func TestHandlerTaskList_DefaultLimit(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit with limit=0 (should default to 100).
	listParams, err := json.Marshal(TaskListRequest{Limit: 0})
	require.NoError(t, err)
	result, err := d.handleTaskList(ctx, listParams)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// -- queue.stats --

func TestHandlerQueueStats_Empty(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	result, err := d.handleQueueStats(context.Background(), nil)
	require.NoError(t, err)

	resp, ok := result.(QueueStatsResponse)
	require.True(t, ok)
	assert.Equal(t, 0, resp.Total)
}

func TestHandlerQueueStats_WithItems(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	// Submit tasks at different priorities via the handler.
	for i := 0; i < 2; i++ {
		params, err := json.Marshal(TaskSubmitRequest{Description: "u", Priority: "urgent"})
		require.NoError(t, err)
		_, err = d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
	}

	params, err := json.Marshal(TaskSubmitRequest{Description: "n", Priority: "normal"})
	require.NoError(t, err)
	_, err = d.handleTaskSubmit(ctx, params)
	require.NoError(t, err)

	result, err := d.handleQueueStats(ctx, nil)
	require.NoError(t, err)

	resp := result.(QueueStatsResponse)
	assert.Equal(t, 2, resp.Urgent)
	assert.Equal(t, 1, resp.Normal)
	assert.Equal(t, 0, resp.Low)
	assert.Equal(t, 3, resp.Total)
}

// -- queue.list --

func TestHandlerQueueList_All(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	for _, prio := range []string{"urgent", "normal", "low"} {
		params, err := json.Marshal(TaskSubmitRequest{Description: "t", Priority: prio})
		require.NoError(t, err)
		_, err = d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
	}

	result, err := d.handleQueueList(ctx, nil)
	require.NoError(t, err)

	resp := result.(QueueListResponse)
	assert.Equal(t, 3, resp.Total)
}

func TestHandlerQueueList_ByPriority(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	params, err := json.Marshal(TaskSubmitRequest{Description: "urgent task", Priority: "urgent"})
	require.NoError(t, err)
	_, err = d.handleTaskSubmit(ctx, params)
	require.NoError(t, err)

	params, err = json.Marshal(TaskSubmitRequest{Description: "normal task", Priority: "normal"})
	require.NoError(t, err)
	_, err = d.handleTaskSubmit(ctx, params)
	require.NoError(t, err)

	// List only urgent.
	listParams, err := json.Marshal(QueueListRequest{Priority: "urgent"})
	require.NoError(t, err)
	result, err := d.handleQueueList(ctx, listParams)
	require.NoError(t, err)

	resp := result.(QueueListResponse)
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Entries, 1)
	assert.Equal(t, "urgent", resp.Entries[0].Priority)
}

func TestHandlerQueueList_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleQueueList(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
}

// -- queue.clear --

func TestHandlerQueueClear_All(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	for _, prio := range []string{"urgent", "normal", "low"} {
		params, err := json.Marshal(TaskSubmitRequest{Description: "t", Priority: prio})
		require.NoError(t, err)
		_, err = d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
	}

	// Clear all.
	result, err := d.handleQueueClear(ctx, nil)
	require.NoError(t, err)
	m := result.(map[string]interface{})
	assert.Equal(t, true, m["ok"])

	// Verify empty.
	statsResult, err := d.handleQueueStats(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, statsResult.(QueueStatsResponse).Total)
}

func TestHandlerQueueClear_ByPriority(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()

	for _, prio := range []string{"urgent", "normal", "low"} {
		params, err := json.Marshal(TaskSubmitRequest{Description: "t", Priority: prio})
		require.NoError(t, err)
		_, err = d.handleTaskSubmit(ctx, params)
		require.NoError(t, err)
	}

	// Clear only normal.
	clearParams, err := json.Marshal(QueueClearRequest{Priority: "normal"})
	require.NoError(t, err)
	result, err := d.handleQueueClear(ctx, clearParams)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	statsResult, err := d.handleQueueStats(ctx, nil)
	require.NoError(t, err)
	stats := statsResult.(QueueStatsResponse)
	assert.Equal(t, 0, stats.Normal)
	assert.Equal(t, 2, stats.Urgent+stats.Low)
}

func TestHandlerQueueClear_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleQueueClear(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

// -- events.subscribe --

func TestHandlerEventsSubscribe(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	result, err := d.handleEventsSubscribe(context.Background(), nil)
	require.NoError(t, err)

	resp, ok := result.(EventSubscribeResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.Channel)
	assert.NotEmpty(t, resp.LogPrefix)
}

// -- stubHandler --

func TestStubHandler(t *testing.T) {
	t.Parallel()
	h := stubHandler("task.approve")
	result, err := h(context.Background(), nil)
	require.ErrorIs(t, err, errNotImplemented)
	assert.Nil(t, result)
}

// -- setupRouter --

func TestSetupRouter(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	r := NewRouter(d.logger)
	d.setupRouter(r)

	// All registered methods should dispatch without "method not found".
	methods := []string{
		MethodDaemonPing, MethodDaemonStatus,
		MethodTaskList, MethodTaskApprove, MethodTaskReject,
		MethodTaskResume, MethodTaskAbandon, MethodTaskCancel,
		MethodQueueStats, MethodEventsSubscribe,
		MethodWorkspaceDestroy, MethodTaskPause,
	}
	ctx := context.Background()
	for _, m := range methods {
		req := &Request{JSONRPC: "2.0", Method: m, ID: 1}
		resp := r.Dispatch(ctx, req)
		// Should not return a "method not found" error.
		if resp != nil && resp.Error != nil {
			assert.NotEqual(t, ErrCodeMethodNotFound, resp.Error.Code,
				"method %s should be registered", m)
		}
	}
}

// -- safeIndex --

func TestSafeIndex(t *testing.T) {
	t.Parallel()
	vals := []string{"a", "b", "c"}
	assert.Equal(t, "a", safeIndex(vals, 0))
	assert.Equal(t, "b", safeIndex(vals, 1))
	assert.Equal(t, "c", safeIndex(vals, 2))
	assert.Empty(t, safeIndex(vals, 3))
	assert.Empty(t, safeIndex(vals, 100))
	assert.Empty(t, safeIndex(nil, 0))
}
