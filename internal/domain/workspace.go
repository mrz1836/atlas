// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
)

// Workspace represents a development workspace in ATLAS.
// Each workspace corresponds to a git worktree and can contain
// multiple tasks executed in sequence.
//
// Example JSON representation:
//
//	{
//	    "name": "auth-feature",
//	    "path": "~/.atlas/workspaces/auth-feature/",
//	    "worktree_path": "../repo-auth-feature/",
//	    "branch": "feat/user-auth",
//	    "status": "active",
//	    "tasks": [...],
//	    "created_at": "2025-12-27T10:00:00Z",
//	    "updated_at": "2025-12-27T10:05:00Z",
//	    "schema_version": 1
//	}
type Workspace struct {
	// Name is the unique identifier for this workspace.
	Name string `json:"name"`

	// Path is the directory where workspace data is stored.
	// Typically: ~/.atlas/workspaces/<name>/
	Path string `json:"path"`

	// WorktreePath is the path to the git worktree for this workspace.
	// Typically: ../repo-<name>/
	WorktreePath string `json:"worktree_path"`

	// Branch is the git branch this workspace operates on.
	Branch string `json:"branch"`

	// Status is the current state of the workspace.
	// Uses constants.WorkspaceStatus values (active, paused, closed).
	Status constants.WorkspaceStatus `json:"status"`

	// Tasks is the list of tasks associated with this workspace.
	Tasks []TaskRef `json:"tasks"`

	// CreatedAt is when the workspace was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the workspace was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// Metadata stores arbitrary key-value data associated with the workspace.
	Metadata map[string]any `json:"metadata,omitempty"`

	// SchemaVersion indicates the version of the Workspace struct schema.
	// This enables forward-compatible schema migrations.
	SchemaVersion int `json:"schema_version"`
}

// TaskRef is a lightweight reference to a task within a workspace.
// This allows workspaces to track task history without embedding
// full task objects.
//
// Example JSON representation:
//
//	{
//	    "id": "task-550e8400-e29b-41d4-a716-446655440000",
//	    "status": "completed",
//	    "started_at": "2025-12-27T10:00:00Z",
//	    "completed_at": "2025-12-27T10:30:00Z"
//	}
type TaskRef struct {
	// ID is the unique identifier of the referenced task.
	ID string `json:"id"`

	// Status is the current state of the referenced task.
	Status constants.TaskStatus `json:"status"`

	// StartedAt is when the task began execution.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the task finished execution.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
