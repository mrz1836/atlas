package constants

// TaskStatus represents the state of a task in the ATLAS state machine.
// Status values use snake_case for JSON serialization compatibility.
type TaskStatus string

// Task status constants define the valid states a task can be in.
// These follow the state machine defined in the architecture:
//
//	Pending → Running
//	Running → Validating, GHFailed, CIFailed, CITimeout
//	Validating → AwaitingApproval, ValidationFailed
//	ValidationFailed → Running, Abandoned
//	AwaitingApproval → Completed, Running, Rejected
//	GHFailed → Running, Abandoned
//	CIFailed → Running, Abandoned
//	CITimeout → Running, Abandoned
const (
	// TaskStatusPending indicates a task is queued but not yet started.
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusRunning indicates the AI agent is actively executing the task.
	TaskStatusRunning TaskStatus = "running"

	// TaskStatusValidating indicates the task is undergoing validation checks.
	TaskStatusValidating TaskStatus = "validating"

	// TaskStatusValidationFailed indicates one or more validation checks failed.
	// The task can be retried (→ Running) or abandoned (→ Abandoned).
	TaskStatusValidationFailed TaskStatus = "validation_failed"

	// TaskStatusAwaitingApproval indicates validation passed and the task
	// is waiting for user approval before completion.
	TaskStatusAwaitingApproval TaskStatus = "awaiting_approval"

	// TaskStatusCompleted indicates the task finished successfully and was approved.
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusRejected indicates the user rejected the task during approval.
	TaskStatusRejected TaskStatus = "rejected"

	// TaskStatusAbandoned indicates the task was manually abandoned by the user.
	TaskStatusAbandoned TaskStatus = "abandoned"

	// TaskStatusGHFailed indicates GitHub operations (PR creation, etc.) failed.
	// The task can be retried (→ Running) or abandoned (→ Abandoned).
	TaskStatusGHFailed TaskStatus = "gh_failed"

	// TaskStatusCIFailed indicates CI pipeline checks failed.
	// The task can be retried (→ Running) or abandoned (→ Abandoned).
	TaskStatusCIFailed TaskStatus = "ci_failed"

	// TaskStatusCITimeout indicates CI pipeline exceeded the configured timeout.
	// The task can be retried (→ Running) or abandoned (→ Abandoned).
	TaskStatusCITimeout TaskStatus = "ci_timeout"
)

// String returns the string representation of the TaskStatus.
// This implements fmt.Stringer for convenient logging and debugging.
func (s TaskStatus) String() string {
	return string(s)
}

// WorkspaceStatus represents the state of a workspace in ATLAS.
// Status values use snake_case for JSON serialization compatibility.
type WorkspaceStatus string

// Workspace status constants define the valid states a workspace can be in.
const (
	// WorkspaceStatusActive indicates the workspace is currently in use.
	WorkspaceStatusActive WorkspaceStatus = "active"

	// WorkspaceStatusPaused indicates the workspace is temporarily inactive
	// but can be resumed.
	WorkspaceStatusPaused WorkspaceStatus = "paused"

	// WorkspaceStatusRetired indicates the workspace has been archived
	// and is no longer in active use.
	WorkspaceStatusRetired WorkspaceStatus = "retired"
)

// String returns the string representation of the WorkspaceStatus.
// This implements fmt.Stringer for convenient logging and debugging.
func (s WorkspaceStatus) String() string {
	return string(s)
}
