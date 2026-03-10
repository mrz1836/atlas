package daemon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTrip marshals v to JSON then unmarshals into a new value of the same type.
func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestRequestRoundTrip(t *testing.T) {
	t.Parallel()
	req, err := NewRequest(MethodTaskSubmit, TaskSubmitRequest{Description: "test", Template: "default"}, 1)
	require.NoError(t, err)
	got := roundTrip(t, *req)
	assert.Equal(t, "2.0", got.JSONRPC)
	assert.Equal(t, MethodTaskSubmit, got.Method)
	assert.NotEmpty(t, got.Params)
}

func TestResponseRoundTrip(t *testing.T) {
	t.Parallel()
	resp := NewResponse(TaskSubmitResponse{TaskID: "abc", Status: "queued"}, 42)
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var got Response
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "2.0", got.JSONRPC)
	assert.Nil(t, got.Error)
}

func TestErrorResponseRoundTrip(t *testing.T) {
	t.Parallel()
	resp := NewErrorResponse(ErrCodeMethodNotFound, "method not found", 99)
	got := roundTrip(t, *resp)
	assert.Equal(t, "2.0", got.JSONRPC)
	require.NotNil(t, got.Error)
	assert.Equal(t, ErrCodeMethodNotFound, got.Error.Code)
	assert.Equal(t, "method not found", got.Error.Message)
}

func TestNotificationRoundTrip(t *testing.T) {
	t.Parallel()
	n := NewNotification(EventTaskStarted, TaskEvent{Type: EventTaskStarted, TaskID: "xyz"})
	got := roundTrip(t, *n)
	assert.Equal(t, "2.0", got.JSONRPC)
	assert.Equal(t, EventTaskStarted, got.Method)
}

func TestTaskSubmitRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskSubmitRequest{
		Description: "fix bug",
		Template:    "bugfix",
		Priority:    "urgent",
		Workspace:   "/tmp/ws",
		Agent:       "claude",
		Model:       "sonnet",
		Branch:      "fix/bug-123",
	}
	got := roundTrip(t, v)
	assert.Equal(t, v, got)
}

func TestTaskSubmitResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskSubmitResponse{TaskID: "t1", Status: "queued"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskStatusRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskStatusRequest{TaskID: "t1"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskStatusResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskStatusResponse{
		TaskID:      "t1",
		Status:      "running",
		Priority:    "normal",
		CurrentStep: 2,
		TotalSteps:  5,
		SubmittedAt: "2026-03-09T14:00:00Z",
		StartedAt:   "2026-03-09T14:01:00Z",
	}
	got := roundTrip(t, v)
	assert.Equal(t, v, got)
}

func TestTaskListRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskListRequest{Status: "running", Priority: "urgent", Limit: 10}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskListResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskListResponse{
		Tasks: []TaskStatusResponse{{TaskID: "t1", Status: "queued", Priority: "low", SubmittedAt: "2026-03-09T14:00:00Z"}},
		Total: 1,
	}
	got := roundTrip(t, v)
	assert.Equal(t, v, got)
}

func TestTaskApproveRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskApproveRequest{TaskID: "t1", Close: true, Message: "lgtm"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskRejectRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskRejectRequest{TaskID: "t1", Retry: true, Feedback: "needs tests", Step: "test"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskResumeRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskResumeRequest{TaskID: "t1", AIFix: true}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskAbandonRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskAbandonRequest{TaskID: "t1"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskCancelRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskCancelRequest{TaskID: "t1"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestQueueListRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := QueueListRequest{Priority: "urgent"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestQueueListResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := QueueListResponse{
		Entries: []QueueEntryResponse{{TaskID: "t1", Priority: "urgent", Score: 1234567890}},
		Total:   1,
	}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestQueueClearRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := QueueClearRequest{Priority: "low"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestQueueStatsResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := QueueStatsResponse{Urgent: 1, Normal: 5, Low: 2, Total: 8}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestDaemonPingResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := DaemonPingResponse{Alive: true, Version: "1.0.0"}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestDaemonStatusResponseRoundTrip(t *testing.T) {
	t.Parallel()
	v := DaemonStatusResponse{
		Version:     "dev",
		PID:         12345,
		Uptime:      "1h2m3s",
		StartedAt:   "2026-03-09T13:00:00Z",
		RedisAlive:  true,
		Workers:     4,
		ActiveTasks: 2,
		QueueDepth:  7,
	}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestDaemonShutdownRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := DaemonShutdownRequest{Graceful: true}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestEventSubscribeRequestRoundTrip(t *testing.T) {
	t.Parallel()
	v := EventSubscribeRequest{Events: []string{"task.*", "queue.*"}}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestTaskEventRoundTrip(t *testing.T) {
	t.Parallel()
	v := TaskEvent{
		Type:    EventTaskSubmitted,
		TaskID:  "t1",
		Status:  "queued",
		Message: "task submitted",
		Time:    "2026-03-09T14:00:00Z",
	}
	assert.Equal(t, v, roundTrip(t, v))
}

func TestRPCErrorCodeConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, -32700, ErrCodeParseError)
	assert.Equal(t, -32600, ErrCodeInvalidRequest)
	assert.Equal(t, -32601, ErrCodeMethodNotFound)
	assert.Equal(t, -32602, ErrCodeInvalidParams)
	assert.Equal(t, -32603, ErrCodeInternal)
}

func TestMethodConstants(t *testing.T) {
	t.Parallel()
	methods := []string{
		MethodTaskSubmit, MethodTaskStatus, MethodTaskList,
		MethodTaskApprove, MethodTaskReject, MethodTaskResume,
		MethodTaskAbandon, MethodTaskCancel,
		MethodQueueList, MethodQueueClear, MethodQueueStats,
		MethodDaemonPing, MethodDaemonStatus, MethodDaemonShutdown,
		MethodEventsSubscribe, MethodEventsUnsubscribe,
	}
	for _, m := range methods {
		assert.NotEmpty(t, m)
	}
}

func TestEventConstants(t *testing.T) {
	t.Parallel()
	events := []string{
		EventTaskSubmitted, EventTaskStarted, EventTaskCompleted,
		EventTaskFailed, EventTaskApproved, EventQueueChanged,
		EventDaemonStarted, EventDaemonStopping,
	}
	for _, e := range events {
		assert.NotEmpty(t, e)
	}
}

func TestNewRequestNilParams(t *testing.T) {
	t.Parallel()
	req, err := NewRequest(MethodDaemonPing, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "2.0", req.JSONRPC)
	assert.Nil(t, req.Params)
}
