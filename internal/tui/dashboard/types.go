// Package dashboard provides the interactive full-screen TUI dashboard for ATLAS.
// It displays real-time task monitoring and interactive controls for all daemon tasks.
package dashboard

import "time"

// ViewMode represents the current display mode of the dashboard.
type ViewMode int

const (
	// ViewModeList is the default split-pane view with task list + detail.
	ViewModeList ViewMode = iota
	// ViewModeDetail is the full-screen task detail view.
	ViewModeDetail
	// ViewModeLog is the full-screen log view.
	ViewModeLog
	// ViewModeHelp is the help overlay.
	ViewModeHelp
)

// String returns the string representation of the ViewMode.
func (v ViewMode) String() string {
	switch v {
	case ViewModeList:
		return "list"
	case ViewModeDetail:
		return "detail"
	case ViewModeLog:
		return "log"
	case ViewModeHelp:
		return "help"
	default:
		return "unknown"
	}
}

// TaskStatus represents the status of a task as seen by the dashboard.
// These map to the daemon's internal statuses but are normalized for display.
type TaskStatus string

const (
	// TaskStatusQueued is a task waiting to be picked up by the runner.
	TaskStatusQueued TaskStatus = "queued"
	// TaskStatusRunning is a task actively being executed.
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusPaused is a task that has been paused (resumable).
	TaskStatusPaused TaskStatus = "paused"
	// TaskStatusAwaitingApproval is a task waiting for user approval.
	TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"
	// TaskStatusCompleted is a successfully completed task.
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusFailed is a task that failed.
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusAbandoned is a task that was abandoned by the user.
	TaskStatusAbandoned TaskStatus = "abandoned"
)

// TaskInfo holds all display information for a single task in the dashboard.
// It is populated from daemon RPC responses and updated by real-time events.
type TaskInfo struct {
	// ID is the unique task identifier.
	ID string
	// Description is the human-readable task description.
	Description string
	// Status is the current task status.
	Status TaskStatus
	// Priority is the task priority (high, normal, low).
	Priority string
	// Template is the task template name.
	Template string
	// Agent is the AI agent being used (e.g., "claude", "codex").
	Agent string
	// Model is the AI model being used (e.g., "claude-opus-4").
	Model string
	// Branch is the Git branch for this task.
	Branch string
	// Workspace is the workspace name for this task.
	Workspace string
	// CurrentStep is the name of the step currently executing.
	CurrentStep string
	// StepIndex is the current step number (1-based).
	StepIndex int
	// StepTotal is the total number of steps.
	StepTotal int
	// PRURL is the URL of the pull request created by this task.
	PRURL string
	// Error holds the error message if the task failed.
	Error string
	// SubmittedAt is when the task was first submitted.
	SubmittedAt time.Time
	// StartedAt is when the task started executing.
	StartedAt time.Time
	// CompletedAt is when the task finished (success, failure, or abandonment).
	CompletedAt time.Time
	// UpdatedAt is the timestamp of the last event for this task.
	UpdatedAt time.Time
}

// ConnectionState represents the dashboard's connection state to the daemon.
type ConnectionState int

const (
	// ConnectionStateConnected means both daemon RPC and Redis pub/sub are active.
	ConnectionStateConnected ConnectionState = iota
	// ConnectionStateReconnecting means a reconnection attempt is in progress.
	ConnectionStateReconnecting
	// ConnectionStateDisconnected means the connection is lost and reconnection failed.
	ConnectionStateDisconnected
)

// String returns the string representation of the ConnectionState.
func (c ConnectionState) String() string {
	switch c {
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateReconnecting:
		return "reconnecting"
	case ConnectionStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}
