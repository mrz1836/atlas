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
	// Branch is the base git branch to create the workspace from (maps to --branch CLI flag).
	Branch string
	// TargetBranch is an existing branch to check out directly, skipping new branch creation
	// (maps to --target CLI flag; mutually exclusive with Branch).
	TargetBranch string
	// UseLocal prefers the local copy of the branch over the remote when both exist
	// (maps to --use-local CLI flag).
	UseLocal bool
	// RepoPath is the absolute path to the git repository.
	RepoPath string
	// Agent overrides the template's default AI agent (optional).
	Agent string
	// Model overrides the template's default AI model (optional).
	Model string
	// Verify enables the AI verification step (maps to --verify CLI flag).
	Verify bool
	// NoVerify disables the AI verification step (maps to --no-verify CLI flag).
	NoVerify bool
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
