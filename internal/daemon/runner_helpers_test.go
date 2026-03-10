package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	cache "github.com/mrz1836/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errEngineError is a package-level sentinel used by tests that simulate an executor failure.
var errEngineError = errors.New("engine error")

// TestLoadTaskJob verifies that loadTaskJob correctly reads all fields from Redis.
func TestLoadTaskJob(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "load-job-test"
	hashKey := "atlas:task:" + taskID

	// Seed all fields in Redis.
	pairs := [][2]interface{}{
		{"description", "do something"},
		{"template", "bug"},
		{"workspace", "my-ws"},
		{"branch", "feat/x"},
		{"repo_path", "/home/repo"},
		{"agent", "claude"},
		{"model", "sonnet"},
		{"engine_task_id", "eng-123"},
		{"approval_choice", "approve"},
		{"reject_feedback", "fix it"},
	}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))

	job, err := r.loadTaskJob(ctx, taskID)
	require.NoError(t, err)

	assert.Equal(t, taskID, job.TaskID)
	assert.Equal(t, "do something", job.Description)
	assert.Equal(t, "bug", job.Template)
	assert.Equal(t, "my-ws", job.Workspace)
	assert.Equal(t, "feat/x", job.Branch)
	assert.Equal(t, "/home/repo", job.RepoPath)
	assert.Equal(t, "claude", job.Agent)
	assert.Equal(t, "sonnet", job.Model)
	assert.Equal(t, "eng-123", job.EngineTaskID)
	assert.Equal(t, "approve", job.ApprovalChoice)
	assert.Equal(t, "fix it", job.RejectFeedback)
}

// TestLoadTaskJob_PartialFields verifies that missing fields are returned as empty strings.
func TestLoadTaskJob_PartialFields(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "partial-job-test"
	hashKey := "atlas:task:" + taskID

	// Seed only description.
	pairs := [][2]interface{}{{"description", "minimal"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))

	job, err := r.loadTaskJob(ctx, taskID)
	require.NoError(t, err)

	assert.Equal(t, "minimal", job.Description)
	assert.Empty(t, job.Template)
	assert.Empty(t, job.EngineTaskID)
}

// TestStoreEngineTaskID verifies that storeEngineTaskID writes to the Redis hash.
func TestStoreEngineTaskID(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "store-eid-test"

	r.storeEngineTaskID(ctx, taskID, "engine-abc")

	hashKey := "atlas:task:" + taskID
	val, err := cache.HashGet(ctx, client, hashKey, "engine_task_id")
	require.NoError(t, err)
	assert.Equal(t, "engine-abc", val)
}

// TestMarkTaskStatus verifies that markTaskStatus updates the status field.
func TestMarkTaskStatus(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "mark-status-test"

	r.markTaskStatus(ctx, taskID, "awaiting_approval")

	hashKey := "atlas:task:" + taskID
	val, err := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, err)
	assert.Equal(t, "awaiting_approval", val)
}

// TestCancelTask_Running verifies CancelTask returns true and cancels the context.
func TestCancelTask_Running(t *testing.T) {
	t.Parallel()
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "cancel-running-task"
	r.taskCtxMu.Lock()
	r.taskCtxs[taskID] = cancel
	r.taskCtxMu.Unlock()

	wasRunning := r.CancelTask(taskID)
	assert.True(t, wasRunning)

	// The context should now be done.
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("context should have been canceled")
	}

	// canceledTasks should have the final status.
	val, ok := r.canceledTasks.Load(taskID)
	assert.True(t, ok)
	assert.Equal(t, "canceled", val)
}

// TestCancelTask_NotRunning verifies CancelTask returns false for non-running tasks
// and cleans up the canceledTasks marker.
func TestCancelTask_NotRunning(t *testing.T) {
	t.Parallel()
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	wasRunning := r.CancelTask("no-such-task")
	assert.False(t, wasRunning)

	// canceledTasks entry must be cleaned up.
	_, ok := r.canceledTasks.Load("no-such-task")
	assert.False(t, ok)
}

// TestAbandonRunningTask_Running verifies AbandonRunningTask cancels the context
// and stores "abandoned" as the final status.
func TestAbandonRunningTask_Running(t *testing.T) {
	t.Parallel()
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "abandon-running-task"
	r.taskCtxMu.Lock()
	r.taskCtxs[taskID] = cancel
	r.taskCtxMu.Unlock()

	wasRunning := r.AbandonRunningTask(taskID)
	assert.True(t, wasRunning)

	// Context must be canceled.
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context should have been canceled")
	}

	// canceledTasks must record "abandoned".
	val, ok := r.canceledTasks.Load(taskID)
	assert.True(t, ok)
	assert.Equal(t, "abandoned", val)
}

// TestAbandonRunningTask_NotRunning verifies AbandonRunningTask returns false
// and cleans up when the task is not running.
func TestAbandonRunningTask_NotRunning(t *testing.T) {
	t.Parallel()
	r, _, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	wasRunning := r.AbandonRunningTask("ghost-task")
	assert.False(t, wasRunning)

	_, ok := r.canceledTasks.Load("ghost-task")
	assert.False(t, ok)
}

// TestRequeueForResume_Basic verifies RequeueForResume updates status and resubmits.
func TestRequeueForResume_Basic(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-basic-task"
	hashKey := "atlas:task:" + taskID

	// Seed task as failed.
	pairs := [][2]interface{}{{"status", "failed"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))

	err := r.RequeueForResume(ctx, taskID, "", "")
	require.NoError(t, err)

	// Status should be updated to "queued".
	status, getErr := cache.HashGet(ctx, client, hashKey, "status")
	require.NoError(t, getErr)
	assert.Equal(t, "queued", status)

	// Task should be re-submitted to queue.
	poppedID, popErr := q.Pop(ctx)
	require.NoError(t, popErr)
	assert.Equal(t, taskID, poppedID)
}

// TestRequeueForResume_WithApprovalFields verifies approval fields are stored.
func TestRequeueForResume_WithApprovalFields(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-approval-task"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"status", "awaiting_approval"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))

	err := r.RequeueForResume(ctx, taskID, "approve", "")
	require.NoError(t, err)

	approvalChoice, err := cache.HashGet(ctx, client, hashKey, "approval_choice")
	require.NoError(t, err)
	assert.Equal(t, "approve", approvalChoice)
}

// TestRequeueForResume_WithRejectFeedback verifies reject fields are stored.
func TestRequeueForResume_WithRejectFeedback(t *testing.T) {
	t.Parallel()
	r, client, _, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	ctx := context.Background()
	taskID := "resume-reject-task"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"status", "awaiting_approval"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))

	err := r.RequeueForResume(ctx, taskID, "reject", "the code is wrong")
	require.NoError(t, err)

	rejectFeedback, err := cache.HashGet(ctx, client, hashKey, "reject_feedback")
	require.NoError(t, err)
	assert.Equal(t, "the code is wrong", rejectFeedback)
}

// TestExecuteTask_WithExecutor_Completed verifies that a real executor that returns
// "completed" causes the task to be marked completed in Redis.
func TestExecuteTask_WithExecutor_Completed(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	// Wire a stub executor that returns "completed" immediately.
	r.executor = &mockExecutor{finalStatus: "completed", engineTaskID: "eng-1"}

	ctx := context.Background()
	taskID := "exec-completed"
	hashKey := "atlas:task:" + taskID

	// Seed task metadata.
	pairs := [][2]interface{}{
		{"description", "test task"},
		{"template", "bug"},
		{"status", "queued"},
	}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, client, "atlas:active", taskID))
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)
	defer r.Stop()

	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "completed"
	}, 5*time.Second, 50*time.Millisecond, "task should be completed")

	// Engine task ID should be stored.
	eid, err := cache.HashGet(ctx, client, hashKey, "engine_task_id")
	require.NoError(t, err)
	assert.Equal(t, "eng-1", eid)
}

// TestExecuteTask_WithExecutor_AwaitingApproval verifies awaiting_approval status.
func TestExecuteTask_WithExecutor_AwaitingApproval(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	r.executor = &mockExecutor{finalStatus: "awaiting_approval", engineTaskID: "eng-2"}

	ctx := context.Background()
	taskID := "exec-approval"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"description", "approval task"}, {"status", "queued"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, client, "atlas:active", taskID))
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)
	defer r.Stop()

	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "awaiting_approval"
	}, 5*time.Second, 50*time.Millisecond, "task should be awaiting_approval")
}

// TestExecuteTask_WithExecutor_Failed verifies executor error causes task to be failed.
func TestExecuteTask_WithExecutor_Failed(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	r.executor = &mockExecutor{execErr: errEngineError}

	ctx := context.Background()
	taskID := "exec-failed"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"description", "failing task"}, {"status", "queued"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, client, "atlas:active", taskID))
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)
	defer r.Stop()

	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "failed"
	}, 5*time.Second, 50*time.Millisecond, "task should be failed")
}

// TestExecuteTask_WithExecutor_CancelDuringExecution verifies that canceling a running
// task marks it as "canceled" in Redis.
func TestExecuteTask_WithExecutor_CancelDuringExecution(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	// Executor that blocks until context is canceled.
	r.executor = &mockExecutor{blockUntilCtxDone: true}

	ctx := context.Background()
	taskID := "exec-cancel"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"description", "long task"}, {"status", "queued"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, client, "atlas:active", taskID))
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)
	defer r.Stop()

	// Wait until the task is running.
	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "running"
	}, 5*time.Second, 50*time.Millisecond, "task should start running")

	// Now cancel it.
	wasRunning := r.CancelTask(taskID)
	assert.True(t, wasRunning)

	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "canceled"
	}, 5*time.Second, 50*time.Millisecond, "task should be marked canceled")
}

// TestExecuteTask_WithExecutor_AbandonDuringExecution verifies that abandoning a running
// task marks it as "abandoned" in Redis.
func TestExecuteTask_WithExecutor_AbandonDuringExecution(t *testing.T) {
	t.Parallel()
	r, client, q, cleanup := newTestRunnerWithRedis(t)
	defer cleanup()

	r.executor = &mockExecutor{blockUntilCtxDone: true}

	ctx := context.Background()
	taskID := "exec-abandon"
	hashKey := "atlas:task:" + taskID

	pairs := [][2]interface{}{{"description", "long task 2"}, {"status", "queued"}}
	require.NoError(t, cache.HashMapSet(ctx, client, hashKey, pairs))
	require.NoError(t, cache.SetAdd(ctx, client, "atlas:active", taskID))
	require.NoError(t, q.Submit(ctx, taskID, PriorityNormal))

	r.Start(ctx)
	defer r.Stop()

	// Wait until running.
	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "running"
	}, 5*time.Second, 50*time.Millisecond, "task should start running")

	wasRunning := r.AbandonRunningTask(taskID)
	assert.True(t, wasRunning)

	require.Eventually(t, func() bool {
		status, err := cache.HashGet(ctx, client, hashKey, "status")
		return err == nil && status == "abandoned"
	}, 5*time.Second, 50*time.Millisecond, "task should be marked abandoned")
}

// mockExecutor is a test double for TaskExecutor.
type mockExecutor struct {
	engineTaskID      string
	finalStatus       string
	execErr           error
	blockUntilCtxDone bool
	abandonErr        error
}

func (m *mockExecutor) Execute(ctx context.Context, _ TaskJob) (string, string, error) {
	if m.blockUntilCtxDone {
		<-ctx.Done()
		return "", "", ctx.Err()
	}
	return m.engineTaskID, m.finalStatus, m.execErr
}

func (m *mockExecutor) Abandon(_ context.Context, _ TaskJob, _ string) error {
	return m.abandonErr
}
