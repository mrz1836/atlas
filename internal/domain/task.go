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
//	    "id": "task-20251227-100000",
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

	// SchemaVersion indicates the version of the Task struct schema.
	// This enables forward-compatible schema migrations.
	SchemaVersion int `json:"schema_version"`
}

// TaskConfig holds configuration options for task execution.
type TaskConfig struct {
	// Model specifies the AI model to use (e.g., "claude-sonnet-4-20250514").
	Model string `json:"model,omitempty"`

	// MaxTurns limits the number of AI conversation turns per step.
	MaxTurns int `json:"max_turns,omitempty"`

	// Timeout is the maximum duration for task execution.
	Timeout time.Duration `json:"timeout,omitempty"`

	// PermissionMode controls AI permissions ("", "plan").
	PermissionMode string `json:"permission_mode,omitempty"`

	// ValidationCommands are the commands to run during validation.
	ValidationCommands []string `json:"validation_commands,omitempty"`

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
//	    "step_name": "implement",
//	    "success": true,
//	    "output": "Created 3 files...",
//	    "duration_ms": 45000,
//	    "files_changed": ["cmd/main.go", "internal/service.go"]
//	}
type StepResult struct {
	// StepName identifies which step produced this result.
	StepName string `json:"step_name"`

	// Success indicates whether the step completed without errors.
	Success bool `json:"success"`

	// Output contains any text output from the step execution.
	Output string `json:"output,omitempty"`

	// Error contains the error message if Success is false.
	Error string `json:"error,omitempty"`

	// Duration is how long the step took to execute.
	Duration time.Duration `json:"duration"`

	// FilesChanged lists paths of files that were created or modified.
	FilesChanged []string `json:"files_changed,omitempty"`

	// ArtifactPath points to any output artifacts (logs, reports, etc.).
	ArtifactPath string `json:"artifact_path,omitempty"`
}
