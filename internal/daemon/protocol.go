// Package daemon provides background process management and Redis connectivity
// for the Atlas task queue system.
package daemon

import "encoding/json"

// Request is a JSON-RPC 2.0 request object.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// Response is a JSON-RPC 2.0 response object.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

// Notification is a JSON-RPC 2.0 notification (request with no ID).
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// RPCError is the JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// Method name constants for the Atlas daemon protocol.
const (
	MethodTaskSubmit        = "task.submit"
	MethodTaskStatus        = "task.status"
	MethodTaskList          = "task.list"
	MethodTaskApprove       = "task.approve"
	MethodTaskReject        = "task.reject"
	MethodTaskResume        = "task.resume"
	MethodTaskAbandon       = "task.abandon"
	MethodTaskCancel        = "task.cancel"
	MethodQueueList         = "queue.list"
	MethodQueueClear        = "queue.clear"
	MethodQueueStats        = "queue.stats"
	MethodDaemonPing        = "daemon.ping"
	MethodDaemonStatus      = "daemon.status"
	MethodDaemonShutdown    = "daemon.shutdown"
	MethodEventsSubscribe   = "events.subscribe"
	MethodEventsUnsubscribe = "events.unsubscribe"
	MethodWorkspaceDestroy  = "workspace.destroy"
	MethodTaskPause         = "task.pause"
)

// TaskSubmitRequest is the params for task.submit.
type TaskSubmitRequest struct {
	Description  string `json:"description"`
	Template     string `json:"template"`
	Priority     string `json:"priority,omitempty"` // urgent|normal|low
	Workspace    string `json:"workspace,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Model        string `json:"model,omitempty"`
	Branch       string `json:"branch,omitempty"` // base branch to create workspace from
	RepoPath     string `json:"repo_path,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"` // existing branch to checkout (mutually exclusive with Branch)
	UseLocal     bool   `json:"use_local,omitempty"`     // prefer local branch over remote
	Verify       bool   `json:"verify,omitempty"`        // enable AI verification step
	NoVerify     bool   `json:"no_verify,omitempty"`     // disable AI verification step
}

// TaskSubmitResponse is the result for task.submit.
type TaskSubmitResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// TaskStatusRequest is the params for task.status.
type TaskStatusRequest struct {
	TaskID string `json:"task_id"`
}

// TaskStatusResponse is the result for task.status.
type TaskStatusResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	SubmittedAt string `json:"submitted_at"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Template    string `json:"template,omitempty"`
}

// TaskListRequest is the params for task.list.
type TaskListRequest struct {
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// TaskListResponse is the result for task.list.
type TaskListResponse struct {
	Tasks []TaskStatusResponse `json:"tasks"`
	Total int                  `json:"total"`
}

// TaskApproveRequest is the params for task.approve.
type TaskApproveRequest struct {
	TaskID  string `json:"task_id"`
	Close   bool   `json:"close,omitempty"`
	Message string `json:"message,omitempty"`
}

// TaskRejectRequest is the params for task.reject.
type TaskRejectRequest struct {
	TaskID   string `json:"task_id"`
	Retry    bool   `json:"retry,omitempty"`
	Feedback string `json:"feedback,omitempty"`
	Step     string `json:"step,omitempty"`
}

// TaskResumeRequest is the params for task.resume.
type TaskResumeRequest struct {
	TaskID string `json:"task_id"`
	AIFix  bool   `json:"ai_fix,omitempty"`
}

// TaskAbandonRequest is the params for task.abandon.
type TaskAbandonRequest struct {
	TaskID string `json:"task_id"`
}

// TaskCancelRequest is the params for task.cancel.
type TaskCancelRequest struct {
	TaskID string `json:"task_id"`
}

// QueueListRequest is the params for queue.list.
type QueueListRequest struct {
	Priority string `json:"priority,omitempty"`
	Limit    int    `json:"limit,omitempty"`  // max entries to return; default/max 500
	Offset   int    `json:"offset,omitempty"` // skip first N entries
}

// QueueListResponse is the result for queue.list.
type QueueListResponse struct {
	Entries []QueueEntryResponse `json:"entries"`
	Total   int                  `json:"total"`
}

// QueueEntryResponse describes a single queued task.
type QueueEntryResponse struct {
	TaskID   string  `json:"task_id"`
	Priority string  `json:"priority"`
	Score    float64 `json:"score"`
}

// QueueClearRequest is the params for queue.clear.
type QueueClearRequest struct {
	Priority string `json:"priority,omitempty"`
}

// QueueStatsResponse is the result for queue.stats.
type QueueStatsResponse struct {
	Urgent int `json:"urgent"`
	Normal int `json:"normal"`
	Low    int `json:"low"`
	Total  int `json:"total"`
}

// DaemonPingResponse is the result for daemon.ping.
//
//nolint:revive // DaemonPingResponse is intentionally prefixed; it disambiguates across packages.
type DaemonPingResponse struct {
	Alive   bool   `json:"alive"`
	Version string `json:"version,omitempty"`
}

// DaemonStatusResponse is the result for daemon.status.
//
//nolint:revive // DaemonStatusResponse is intentionally prefixed; it disambiguates across packages.
type DaemonStatusResponse struct {
	Version     string `json:"version,omitempty"`
	PID         int    `json:"pid"`
	Uptime      string `json:"uptime"`
	StartedAt   string `json:"started_at"`
	RedisAlive  bool   `json:"redis_alive"`
	Workers     int    `json:"workers"`
	ActiveTasks int    `json:"active_tasks"`
	QueueDepth  int    `json:"queue_depth"`
}

// DaemonShutdownRequest is the params for daemon.shutdown.
//
//nolint:revive // DaemonShutdownRequest is intentionally prefixed; it disambiguates across packages.
type DaemonShutdownRequest struct {
	Graceful bool `json:"graceful,omitempty"`
}

// EventSubscribeRequest is the params for events.subscribe.
type EventSubscribeRequest struct {
	Events []string `json:"events,omitempty"` // e.g., ["task.*", "queue.*"]
}

// EventSubscribeResponse is the result for events.subscribe.
// It returns the channel name and key prefix so clients know where to connect.
type EventSubscribeResponse struct {
	Channel   string `json:"channel"`    // e.g., "atlas:events"
	LogPrefix string `json:"log_prefix"` // e.g., "atlas:log:" for stream keys
}

// WorkspaceDestroyRequest is the params for workspace.destroy.
type WorkspaceDestroyRequest struct {
	Workspace string `json:"workspace"`
	RepoPath  string `json:"repo_path,omitempty"`
}

// TaskPauseRequest is the params for task.pause.
type TaskPauseRequest struct {
	TaskID string `json:"task_id"`
}

// TaskEvent is the event payload published to the atlas:events channel.
type TaskEvent struct {
	Type        string `json:"type"` // task.submitted, task.started, etc.
	TaskID      string `json:"task_id"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	Time        string `json:"time"`
	Workspace   string `json:"workspace,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Template    string `json:"template,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Step        string `json:"step,omitempty"`
	StepIndex   int    `json:"step_index,omitempty"`
	StepTotal   int    `json:"step_total,omitempty"`
	PRURL       string `json:"pr_url,omitempty"`
	Error       string `json:"error,omitempty"`
	Description string `json:"description,omitempty"`
}

// Event type constants.
const (
	EventTaskSubmitted        = "task.submitted"
	EventTaskStarted          = "task.started"
	EventTaskCompleted        = "task.completed"
	EventTaskFailed           = "task.failed"
	EventTaskApproved         = "task.approved"
	EventTaskRejected         = "task.rejected"
	EventTaskPaused           = "task.paused"
	EventTaskResumed          = "task.resumed"
	EventTaskAbandoned        = "task.abandoned"
	EventTaskCancelled        = "task.cancelled"
	EventTaskStepStarted      = "task.step_started"
	EventTaskStepCompleted    = "task.step_completed"
	EventTaskApprovalRequired = "task.approval_required"
	EventTaskLogEntry         = "task.log_entry"
	EventQueueChanged         = "queue.changed"
	EventDaemonStarted        = "daemon.started"
	EventDaemonStopping       = "daemon.stopping"
	EventDaemonHeartbeat      = "daemon.heartbeat"
)

// NewRequest constructs a JSON-RPC 2.0 request, marshaling params to JSON.
func NewRequest(method string, params, id interface{}) (*Request, error) {
	var raw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = data
	}
	return &Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  raw,
		ID:      id,
	}, nil
}

// NewResponse constructs a successful JSON-RPC 2.0 response.
func NewResponse(result, id interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// NewErrorResponse constructs a JSON-RPC 2.0 error response.
func NewErrorResponse(code int, message string, id interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

// NewNotification constructs a JSON-RPC 2.0 notification (no ID).
func NewNotification(method string, params interface{}) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}
