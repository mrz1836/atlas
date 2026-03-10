package daemon

import "context"

// TaskJob carries all per-task metadata the executor needs.
type TaskJob struct {
	// TaskID is the daemon-assigned UUID (Redis key).
	TaskID string
	// EngineTaskID is the engine-generated task ID; non-empty means this is a resume.
	EngineTaskID string
	// Description is the human-readable task description.
	Description string
	// Template is the template name to use for execution.
	Template string
	// Workspace is the workspace name to execute in.
	Workspace string
	// Branch is the git branch name for this task.
	Branch string
	// RepoPath is the absolute path to the git repository.
	RepoPath string
	// Agent overrides the template's default AI agent (optional).
	Agent string
	// Model overrides the template's default AI model (optional).
	Model string
	// ApprovalChoice is "approve" or "reject" when resuming after approval.
	ApprovalChoice string
	// RejectFeedback is AI feedback passed on rejection (optional).
	RejectFeedback string
}

// TaskExecutor bridges the daemon queue to the task engine layer.
type TaskExecutor interface {
	// Execute starts a new task (EngineTaskID == "") or resumes a paused one.
	// Returns the engine-assigned task ID, the engine's final status string, and any error.
	Execute(ctx context.Context, job TaskJob) (engineTaskID, finalStatus string, err error)

	// Abandon terminates a paused/error task (not actively running).
	// For running tasks, cancel the context via Runner.CancelTask first.
	Abandon(ctx context.Context, job TaskJob, reason string) error
}
