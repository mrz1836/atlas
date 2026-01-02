// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

import "github.com/mrz1836/atlas/internal/constants"

// Re-export TaskStatus and WorkspaceStatus from constants package.
// This allows consumers to import domain types and status types together,
// providing a unified API for working with ATLAS domain objects.
//
// Example usage:
//
//	import "github.com/mrz1836/atlas/internal/domain"
//
//	task := domain.Task{
//	    Status: domain.TaskStatusPending,
//	}
type (
	// TaskStatus represents the state of a task in the ATLAS state machine.
	TaskStatus = constants.TaskStatus

	// WorkspaceStatus represents the state of a workspace in ATLAS.
	WorkspaceStatus = constants.WorkspaceStatus
)

// Re-export TaskStatus constants for convenience.
// These mirror the values in internal/constants/status.go.
const (
	// TaskStatusPending indicates a task is queued but not yet started.
	TaskStatusPending = constants.TaskStatusPending

	// TaskStatusRunning indicates the AI agent is actively executing the task.
	TaskStatusRunning = constants.TaskStatusRunning

	// TaskStatusValidating indicates the task is undergoing validation checks.
	TaskStatusValidating = constants.TaskStatusValidating

	// TaskStatusValidationFailed indicates one or more validation checks failed.
	TaskStatusValidationFailed = constants.TaskStatusValidationFailed

	// TaskStatusAwaitingApproval indicates validation passed and the task
	// is waiting for user approval before completion.
	TaskStatusAwaitingApproval = constants.TaskStatusAwaitingApproval

	// TaskStatusCompleted indicates the task finished successfully and was approved.
	TaskStatusCompleted = constants.TaskStatusCompleted

	// TaskStatusRejected indicates the user rejected the task during approval.
	TaskStatusRejected = constants.TaskStatusRejected

	// TaskStatusAbandoned indicates the task was manually abandoned by the user.
	TaskStatusAbandoned = constants.TaskStatusAbandoned

	// TaskStatusGHFailed indicates GitHub operations (PR creation, etc.) failed.
	TaskStatusGHFailed = constants.TaskStatusGHFailed

	// TaskStatusCIFailed indicates CI pipeline checks failed.
	TaskStatusCIFailed = constants.TaskStatusCIFailed

	// TaskStatusCITimeout indicates CI pipeline exceeded the configured timeout.
	TaskStatusCITimeout = constants.TaskStatusCITimeout
)

// Re-export WorkspaceStatus constants for convenience.
// These mirror the values in internal/constants/status.go.
const (
	// WorkspaceStatusActive indicates the workspace is currently in use.
	WorkspaceStatusActive = constants.WorkspaceStatusActive

	// WorkspaceStatusPaused indicates the workspace is temporarily inactive.
	WorkspaceStatusPaused = constants.WorkspaceStatusPaused

	// WorkspaceStatusClosed indicates the workspace has been closed.
	WorkspaceStatusClosed = constants.WorkspaceStatusClosed
)
