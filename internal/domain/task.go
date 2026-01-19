// Package domain provides shared domain types for the ATLAS task orchestration system.
// These types are used across all internal packages to ensure consistent data structures.
//
// This package follows strict import rules:
//   - CAN import: internal/constants, internal/errors, standard library
//   - MUST NOT import: any other internal packages
//
// All JSON field names use snake_case per architecture requirements.
package domain

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
)

// Task represents a single unit of work in the ATLAS system.
// Tasks are created from templates and track the execution progress
// through a series of steps (AI, validation, git, etc.).
//
// Example JSON representation:
//
//	{
//	    "id": "task-550e8400-e29b-41d4-a716-446655440000",
//	    "workspace_id": "auth-workspace",
//	    "template_id": "bugfix",
//	    "description": "Fix null pointer in parseConfig",
//	    "status": "running",
//	    "current_step": 2,
//	    "steps": [...],
//	    "created_at": "2025-12-27T10:00:00Z",
//	    "updated_at": "2025-12-27T10:05:00Z",
//	    "config": {...},
//	    "schema_version": 1
//	}
type Task struct {
	// ID is the unique identifier for the task.
	// Format: task-YYYYMMDD-HHMMSS
	ID string `json:"id"`

	// WorkspaceID links this task to its parent workspace.
	WorkspaceID string `json:"workspace_id"`

	// TemplateID identifies which template was used to create this task.
	TemplateID string `json:"template_id"`

	// Description is a human-readable summary of what the task does.
	Description string `json:"description"`

	// Status represents the current state in the task lifecycle.
	// Uses constants.TaskStatus values (pending, running, completed, etc.).
	Status constants.TaskStatus `json:"status"`

	// CurrentStep is the zero-based index of the currently executing step.
	CurrentStep int `json:"current_step"`

	// Steps is the ordered list of execution steps for this task.
	Steps []Step `json:"steps"`

	// StepResults stores the outcome of each completed step.
	StepResults []StepResult `json:"step_results,omitempty"`

	// Transitions records the history of status changes for audit trail.
	Transitions []Transition `json:"transitions,omitempty"`

	// CreatedAt is when the task was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the task was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// CompletedAt is when the task finished (nil if not yet complete).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Config contains task-specific configuration options.
	Config TaskConfig `json:"config"`

	// Metadata stores arbitrary key-value data associated with the task.
	Metadata map[string]any `json:"metadata,omitempty"`

	// RunningProcesses tracks PIDs of currently running validation commands.
	// This enables force-abandoning tasks by killing the underlying processes.
	// PIDs are added when commands start and removed when they complete/fail.
	RunningProcesses []int `json:"running_processes,omitempty"`

	// SchemaVersion indicates the version of the Task struct schema.
	// This enables forward-compatible schema migrations.
	// Uses string format (e.g., "1.0") for semantic versioning compatibility.
	SchemaVersion string `json:"schema_version"`
}

// TaskConfig holds configuration options for task execution.
type TaskConfig struct {
	// Agent specifies which AI CLI to use (e.g., "claude", "gemini").
	// If empty, defaults to "claude" for backwards compatibility.
	Agent Agent `json:"agent,omitempty"`

	// Model specifies the AI model to use (e.g., "sonnet", "flash").
	Model string `json:"model,omitempty"`

	// MaxTurns limits the number of AI conversation turns per step.
	MaxTurns int `json:"max_turns,omitempty"`

	// Timeout is the maximum duration for task execution.
	Timeout time.Duration `json:"timeout,omitempty"`

	// PermissionMode controls AI permissions ("", "plan").
	PermissionMode string `json:"permission_mode,omitempty"`

	// Variables are template-specific variables for this task.
	Variables map[string]string `json:"variables,omitempty"`
}

// Step represents a single execution step within a task.
// Steps are executed in order and track their own status independently.
//
// Example JSON representation:
//
//	{
//	    "name": "analyze",
//	    "type": "ai",
//	    "status": "completed",
//	    "started_at": "2025-12-27T10:00:00Z",
//	    "completed_at": "2025-12-27T10:05:00Z",
//	    "attempts": 1
//	}
type Step struct {
	// Name identifies the step (e.g., "analyze", "implement", "validate").
	Name string `json:"name"`

	// Type specifies the step execution type (ai, validation, git, etc.).
	Type StepType `json:"type"`

	// Status is the current state of this step.
	// Values: pending, running, completed, failed, skipped
	Status string `json:"status"`

	// StartedAt is when step execution began (nil if not yet started).
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when step execution finished (nil if not yet complete).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error contains the error message if the step failed.
	Error string `json:"error,omitempty"`

	// Attempts counts how many times this step has been executed.
	Attempts int `json:"attempts"`
}

// StepResult captures the outcome of executing a step.
// This is used to record results after step completion.
//
// Example JSON representation:
//
//	{
//	    "step_index": 1,
//	    "step_name": "implement",
//	    "status": "success",
//	    "started_at": "2025-12-27T10:00:00Z",
//	    "completed_at": "2025-12-27T10:05:00Z",
//	    "duration_ms": 45000,
//	    "output": "Created 3 files...",
//	    "files_changed": ["cmd/main.go", "internal/service.go"]
//	}
type StepResult struct {
	// StepIndex is the zero-based index of the step in the task's steps array.
	StepIndex int `json:"step_index"`

	// StepName identifies which step produced this result.
	StepName string `json:"step_name"`

	// Status is the outcome of the step execution (success, failed, skipped).
	Status string `json:"status"`

	// StartedAt is when step execution began.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when step execution finished.
	CompletedAt time.Time `json:"completed_at"`

	// DurationMs is how long the step took to execute in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// Output contains any text output from the step execution.
	Output string `json:"output,omitempty"`

	// Error contains the error message if status is "failed".
	Error string `json:"error,omitempty"`

	// FilesChanged lists paths of files that were created or modified.
	FilesChanged []string `json:"files_changed,omitempty"`

	// ArtifactPath points to any output artifacts (logs, reports, etc.).
	ArtifactPath string `json:"artifact_path,omitempty"`

	// SessionID identifies the AI session for debugging (AI steps only).
	SessionID string `json:"session_id,omitempty"`

	// NumTurns is how many conversation turns occurred (AI steps only).
	NumTurns int `json:"num_turns,omitempty"`

	// Metadata contains additional step-specific data.
	// Used for passing failure_type and ci_result for specialized failure handling.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Transition records a state change for audit trail.
// This enables tracking of task lifecycle and debugging issues.
//
// Example JSON representation:
//
//	{
//	    "from_status": "running",
//	    "to_status": "validating",
//	    "timestamp": "2025-12-27T10:05:00Z",
//	    "reason": "step completed successfully"
//	}
type Transition struct {
	// FromStatus is the status before the transition.
	FromStatus constants.TaskStatus `json:"from_status"`

	// ToStatus is the status after the transition.
	ToStatus constants.TaskStatus `json:"to_status"`

	// Timestamp is when the transition occurred.
	Timestamp time.Time `json:"timestamp"`

	// Reason optionally describes why the transition happened.
	Reason string `json:"reason,omitempty"`
}

// LoopState tracks the current state of a loop step execution.
// This enables checkpointing and resumption of iterative workflows.
type LoopState struct {
	// StepName identifies the loop step.
	StepName string `json:"step_name"`

	// CurrentIteration is the iteration currently executing (1-indexed).
	CurrentIteration int `json:"current_iteration"`

	// MaxIterations is the configured maximum (0 = unlimited).
	MaxIterations int `json:"max_iterations"`

	// CurrentInnerStep is the index within the current iteration.
	CurrentInnerStep int `json:"current_inner_step"`

	// CompletedIterations holds results from finished iterations.
	CompletedIterations []IterationResult `json:"completed_iterations"`

	// ExitReason explains why the loop terminated.
	// Values: "max_iterations_reached", "exit_signal", "condition_met",
	// "circuit_breaker_stagnation", "circuit_breaker_errors", "context_canceled".
	ExitReason string `json:"exit_reason,omitempty"`

	// ScratchpadPath is the full path to the scratchpad file.
	ScratchpadPath string `json:"scratchpad_path,omitempty"`

	// StagnationCount tracks consecutive iterations with no file changes.
	StagnationCount int `json:"stagnation_count"`

	// ConsecutiveErrors tracks consecutive iteration failures.
	ConsecutiveErrors int `json:"consecutive_errors"`

	// ConsecutiveCheckpointErrors tracks consecutive checkpoint save failures.
	// If this exceeds a threshold, the loop should fail to prevent data loss.
	ConsecutiveCheckpointErrors int `json:"consecutive_checkpoint_errors"`

	// StartedAt is when the loop step started.
	StartedAt time.Time `json:"started_at"`

	// LastCheckpoint is when state was last saved.
	LastCheckpoint time.Time `json:"last_checkpoint"`
}

// IterationResult captures the outcome of a single loop iteration.
type IterationResult struct {
	// Iteration is the 1-indexed iteration number.
	Iteration int `json:"iteration"`

	// StepResults contains results from each inner step.
	StepResults []StepResult `json:"step_results"`

	// FilesChanged lists files modified during this iteration.
	FilesChanged []string `json:"files_changed"`

	// ExitSignal indicates if AI signaled completion.
	ExitSignal bool `json:"exit_signal"`

	// Duration is how long the iteration took.
	Duration time.Duration `json:"duration"`

	// StartedAt is when the iteration started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the iteration finished.
	CompletedAt time.Time `json:"completed_at"`

	// Error contains any error message from the iteration.
	Error string `json:"error,omitempty"`
}
