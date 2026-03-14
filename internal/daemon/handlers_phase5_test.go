package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTaskInRedis creates a task hash and adds it to the active set.
func seedTaskInRedis(t *testing.T, d *Daemon, taskID, status string) {
	t.Helper()
	ctx := context.Background()
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	pairs := [][2]interface{}{
		{"id", taskID},
		{"description", "test task"},
		{"status", status},
		{"priority", "normal"},
		{"submitted_at", time.Now().UTC().Format(time.RFC3339)},
	}
	require.NoError(t, cache.HashMapSet(ctx, d.redis, hashKey, pairs))
	activeKey := d.cfg.Redis.KeyPrefix + "active"
	require.NoError(t, cache.SetAdd(ctx, d.redis, activeKey, taskID))
}

// -- task.cancel --

func TestHandlerTaskCancel_NotRunning(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "cancel-test-001"
	seedTaskInRedis(t, d, taskID, "queued")

	params, err := json.Marshal(TaskCancelRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskCancel(ctx, params)
	require.NoError(t, err)

	m, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, m["ok"])

	// Status should be "canceled".
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	status, getErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, getErr)
	assert.Equal(t, "canceled", status)
}

func TestHandlerTaskCancel_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskCancelRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskCancel(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskCancel_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskCancel(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestHandlerTaskCancel_WithRunningTask(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "cancel-running-001"
	seedTaskInRedis(t, d, taskID, "running")

	// Wire a runner with the task "running" (context tracked).
	d.runner = &Runner{
		cfg:      d.cfg,
		redis:    d.redis,
		queue:    d.queue,
		events:   d.events,
		logger:   d.logger,
		sem:      make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		workerID: "test-worker",
		taskCtxs: make(map[string]context.CancelFunc),
	}
	// Register a cancel function for the task.
	cancelCalled := make(chan struct{}, 1)
	d.runner.taskCtxMu.Lock()
	d.runner.taskCtxs[taskID] = func() { close(cancelCalled) }
	d.runner.taskCtxMu.Unlock()

	params, err := json.Marshal(TaskCancelRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskCancel(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	// The cancel func should have been called.
	select {
	case <-cancelCalled:
		// expected
	case <-time.After(time.Second):
		t.Fatal("cancel func not called")
	}
}

// -- task.abandon --

func TestHandlerTaskAbandon_NotRunning(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "abandon-test-001"
	seedTaskInRedis(t, d, taskID, "awaiting_approval")

	params, err := json.Marshal(TaskAbandonRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskAbandon(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	status, getErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, getErr)
	assert.Equal(t, "abandoned", status)
}

func TestHandlerTaskAbandon_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskAbandonRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskAbandon(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskAbandon_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskAbandon(context.Background(), json.RawMessage(`notjson`))
	require.Error(t, err)
}

func TestHandlerTaskAbandon_WithExecutor(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "abandon-exec-test"
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID

	// Seed with engine_task_id and workspace so the executor can be called.
	pairs := [][2]interface{}{
		{"id", taskID},
		{"status", "failed"},
		{"engine_task_id", "eng-abc"},
		{"workspace", "my-ws"},
		{"repo_path", "/repo"},
	}
	require.NoError(t, cache.HashMapSet(ctx, d.redis, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, d.redis, d.cfg.Redis.KeyPrefix+"active", taskID))

	abandonCalled := false
	d.executor = &mockHandlerExecutor{abandonFn: func(_ context.Context, job TaskJob, reason string) error {
		abandonCalled = true
		assert.Equal(t, "eng-abc", job.EngineTaskID)
		assert.Equal(t, "abandoned by user", reason)
		return nil
	}}
	d.runner = &Runner{
		cfg:      d.cfg,
		redis:    d.redis,
		queue:    d.queue,
		events:   d.events,
		logger:   d.logger,
		sem:      make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		workerID: "test-worker",
		taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskAbandonRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskAbandon(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])
	assert.True(t, abandonCalled, "executor.Abandon should have been called")
}

// -- task.resume --

func TestHandlerTaskResume_Valid(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-test-001"
	seedTaskInRedis(t, d, taskID, "failed")

	// Wire a runner (no executor needed for requeue path).
	d.runner = &Runner{
		cfg:      d.cfg,
		redis:    d.redis,
		queue:    d.queue,
		events:   d.events,
		logger:   d.logger,
		sem:      make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		workerID: "test-worker",
		taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskResumeRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskResume(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	// Task should now be queued.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	status, getErr := cache.HashGet(ctx, d.redis, hashKey, "status")
	require.NoError(t, getErr)
	assert.Equal(t, "queued", status)
}

func TestHandlerTaskResume_AwaitingApproval(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-approval-001"
	seedTaskInRedis(t, d, taskID, "awaiting_approval")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskResumeRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskResume(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])
}

func TestHandlerTaskResume_NotResumable(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-invalid-001"
	seedTaskInRedis(t, d, taskID, "completed")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskResumeRequest{TaskID: taskID})
	require.NoError(t, err)

	_, err = d.handleTaskResume(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotResumable)
}

func TestHandlerTaskResume_NotFound(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskResumeRequest{TaskID: "nonexistent-task"})
	require.NoError(t, err)

	_, err = d.handleTaskResume(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotFound)
}

func TestHandlerTaskResume_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskResumeRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskResume(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskResume_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskResume(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

// -- task.approve --

func TestHandlerTaskApprove_Valid(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "approve-test-001"
	seedTaskInRedis(t, d, taskID, "awaiting_approval")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskApproveRequest{TaskID: taskID})
	require.NoError(t, err)

	result, err := d.handleTaskApprove(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	// Approval choice should be stored.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	choice, getErr := cache.HashGet(ctx, d.redis, hashKey, "approval_choice")
	require.NoError(t, getErr)
	assert.Equal(t, "approve", choice)
}

func TestHandlerTaskApprove_NotAwaitingApproval(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "approve-wrong-state"
	seedTaskInRedis(t, d, taskID, "running")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskApproveRequest{TaskID: taskID})
	require.NoError(t, err)

	_, err = d.handleTaskApprove(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotAwaitApproval)
}

func TestHandlerTaskApprove_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskApproveRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskApprove(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskApprove_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskApprove(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
}

// -- task.reject --

func TestHandlerTaskReject_Valid(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "reject-test-001"
	seedTaskInRedis(t, d, taskID, "awaiting_approval")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskRejectRequest{TaskID: taskID, Feedback: "looks wrong"})
	require.NoError(t, err)

	result, err := d.handleTaskReject(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["ok"])

	// Reject feedback should be stored.
	hashKey := d.cfg.Redis.KeyPrefix + "task:" + taskID
	feedback, getErr := cache.HashGet(ctx, d.redis, hashKey, "reject_feedback")
	require.NoError(t, getErr)
	assert.Equal(t, "looks wrong", feedback)

	choice, getErr := cache.HashGet(ctx, d.redis, hashKey, "approval_choice")
	require.NoError(t, getErr)
	assert.Equal(t, "reject", choice)
}

func TestHandlerTaskReject_NotAwaitingApproval(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "reject-wrong-state"
	seedTaskInRedis(t, d, taskID, "completed")

	d.runner = &Runner{
		cfg: d.cfg, redis: d.redis, queue: d.queue,
		events: d.events, logger: d.logger,
		sem: make(chan struct{}, 1), stopCh: make(chan struct{}),
		workerID: "test-worker", taskCtxs: make(map[string]context.CancelFunc),
	}

	params, err := json.Marshal(TaskRejectRequest{TaskID: taskID})
	require.NoError(t, err)

	_, err = d.handleTaskReject(ctx, params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotAwaitApproval)
}

func TestHandlerTaskReject_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskRejectRequest{})
	require.NoError(t, err)

	_, err = d.handleTaskReject(context.Background(), params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskReject_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskReject(context.Background(), json.RawMessage(`bad json`))
	require.Error(t, err)
}

// -- isResumableStatus --

func TestIsResumableStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   string
		expected bool
	}{
		{"awaiting_approval", true},
		{"failed", true},
		{"interrupted", true},
		{"completed", false},
		{"canceled", false},
		{"abandoned", false},
		{"running", false},
		{"queued", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.expected, isResumableStatus(tt.status))
		})
	}
}

// -- fetchTaskStatus --

func TestFetchTaskStatus_Found(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "fetch-status-task"
	seedTaskInRedis(t, d, taskID, "running")

	status, err := d.fetchTaskStatus(ctx, taskID)
	require.NoError(t, err)
	assert.Equal(t, "running", status)
}

func TestFetchTaskStatus_NotFound(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.fetchTaskStatus(context.Background(), "ghost-task")
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskNotFound)
}

// -- task.submit with new fields --

func TestHandlerTaskSubmit_WithRepoPath(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	params, err := json.Marshal(TaskSubmitRequest{
		Description: "test with repo",
		Template:    "bug",
		RepoPath:    "/home/user/myrepo",
		Agent:       "claude",
		Model:       "sonnet",
	})
	require.NoError(t, err)

	result, err := d.handleTaskSubmit(ctx, params)
	require.NoError(t, err)

	resp, ok := result.(TaskSubmitResponse)
	require.True(t, ok)

	hashKey := d.cfg.Redis.KeyPrefix + "task:" + resp.TaskID
	repoPath, err := cache.HashGet(ctx, d.redis, hashKey, "repo_path")
	require.NoError(t, err)
	assert.Equal(t, "/home/user/myrepo", repoPath)

	agent, err := cache.HashGet(ctx, d.redis, hashKey, "agent")
	require.NoError(t, err)
	assert.Equal(t, "claude", agent)

	model, err := cache.HashGet(ctx, d.redis, hashKey, "model")
	require.NoError(t, err)
	assert.Equal(t, "sonnet", model)
}

// -- daemon.New with options --

func TestDaemonNewWithExecutor(t *testing.T) {
	t.Parallel()
	cfg := newTestConfig(t)
	exec := &mockHandlerExecutor{}
	d := New(cfg, zerolog.Nop(), WithExecutor(exec))
	assert.Equal(t, exec, d.executor)
}

// -- helpers for handler tests --

// mockHandlerExecutor is a TaskExecutor for handler-level tests.
type mockHandlerExecutor struct {
	abandonFn func(ctx context.Context, job TaskJob, reason string) error
}

func (m *mockHandlerExecutor) Execute(_ context.Context, _ TaskJob) (string, string, error) {
	return "", "completed", nil
}

func (m *mockHandlerExecutor) Abandon(ctx context.Context, job TaskJob, reason string) error {
	if m.abandonFn != nil {
		return m.abandonFn(ctx, job, reason)
	}
	return nil
}
