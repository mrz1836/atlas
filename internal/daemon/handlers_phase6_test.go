package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- task.pause --

func TestHandlerTaskPause_MissingTaskID(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(TaskPauseRequest{})
	require.NoError(t, err)
	_, err = d.handleTaskPause(context.Background(), params)
	require.ErrorIs(t, err, errTaskIDRequired)
}

func TestHandlerTaskPause_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleTaskPause(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestHandlerTaskPause_QueuedTask(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// Submit a task first so it exists in Redis.
	ctx := context.Background()
	submitParams, err := json.Marshal(TaskSubmitRequest{Description: "pause me"})
	require.NoError(t, err)
	result, err := d.handleTaskSubmit(ctx, submitParams)
	require.NoError(t, err)
	taskID := result.(TaskSubmitResponse).TaskID

	// Pause the queued task.
	pauseParams, err := json.Marshal(TaskPauseRequest{TaskID: taskID})
	require.NoError(t, err)
	resp, err := d.handleTaskPause(ctx, pauseParams)
	require.NoError(t, err)

	m, ok := resp.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, m["ok"])

	// Status should be paused.
	statusParams, err := json.Marshal(TaskStatusRequest{TaskID: taskID})
	require.NoError(t, err)
	statusResult, err := d.handleTaskStatus(ctx, statusParams)
	require.NoError(t, err)
	statusResp := statusResult.(TaskStatusResponse)
	assert.Equal(t, "paused", statusResp.Status)
}

func TestHandlerTaskPause_RunnerNotInitialized(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// Remove runner — pause of non-running task falls through to pauseQueuedTask.
	d.runner = nil

	ctx := context.Background()
	submitParams, err := json.Marshal(TaskSubmitRequest{Description: "test pause"})
	require.NoError(t, err)
	result, err := d.handleTaskSubmit(ctx, submitParams)
	require.NoError(t, err)
	taskID := result.(TaskSubmitResponse).TaskID

	pauseParams, err := json.Marshal(TaskPauseRequest{TaskID: taskID})
	require.NoError(t, err)
	resp, err := d.handleTaskPause(ctx, pauseParams)
	require.NoError(t, err)
	m := resp.(map[string]interface{})
	assert.Equal(t, true, m["ok"])
}

// -- workspace.destroy --

func TestHandlerWorkspaceDestroy_MissingWorkspace(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(WorkspaceDestroyRequest{RepoPath: "/tmp/repo"})
	require.NoError(t, err)
	_, err = d.handleWorkspaceDestroy(context.Background(), params)
	require.ErrorIs(t, err, errWorkspaceRequired)
}

func TestHandlerWorkspaceDestroy_MissingRepoPath(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	params, err := json.Marshal(WorkspaceDestroyRequest{Workspace: "my-ws"})
	require.NoError(t, err)
	_, err = d.handleWorkspaceDestroy(context.Background(), params)
	require.ErrorIs(t, err, errRepoPathRequired)
}

func TestHandlerWorkspaceDestroy_InvalidJSON(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	_, err := d.handleWorkspaceDestroy(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestHandlerWorkspaceDestroy_BadRepoPath(t *testing.T) {
	t.Parallel()
	d, _, cleanup := newTestDaemonWithRedis(t)
	defer cleanup()

	// A non-git directory should cause an error setting up the worktree runner.
	params, err := json.Marshal(WorkspaceDestroyRequest{
		Workspace: "my-ws",
		RepoPath:  "/tmp/definitely-not-a-git-repo-path-xyz123",
	})
	require.NoError(t, err)
	_, err = d.handleWorkspaceDestroy(context.Background(), params)
	require.Error(t, err, "non-git repo path should fail")
}

// -- task.pause isResumableStatus --

func TestIsResumableStatus_Paused(t *testing.T) {
	t.Parallel()
	assert.True(t, isResumableStatus("paused"), "paused should be resumable")
	assert.True(t, isResumableStatus("failed"))
	assert.True(t, isResumableStatus("awaiting_approval"))
	assert.True(t, isResumableStatus("interrupted"))
	assert.False(t, isResumableStatus("running"))
	assert.False(t, isResumableStatus("completed"))
	assert.False(t, isResumableStatus(""))
}
