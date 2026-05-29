// Package lifecycle provides shared lifecycle utilities for daemon and direct Atlas modes.
package lifecycle

import "github.com/mrz1836/atlas/internal/constants"

// Canonical daemon state name constants (Redis-backed task states).
// These are the values stored in Redis and returned by daemon RPC methods.
const (
	StateDaemonQueued           = "queued"
	StateDaemonRunning          = "running"
	StateDaemonAwaitingApproval = "awaiting_approval"
	StateDaemonCompleted        = "completed"
	StateDaemonFailed           = "failed"
	StateDaemonCanceled         = "canceled"
	StateDaemonAbandoned        = "abandoned"
	StateDaemonPaused           = "paused"
	StateDaemonDegraded         = "degraded"

	// StateUnknown is used when a state string cannot be mapped.
	StateUnknown = "unknown"
)

// Canonical display labels — what users see in CLI output.
// These are hyphenated and human-readable, matching the plan's vocabulary:
// queued | running | awaiting-approval | completed | failed | canceled | abandoned | degraded
const (
	LabelQueued           = "queued"
	LabelRunning          = "running"
	LabelAwaitingApproval = "awaiting-approval"
	LabelCompleted        = "completed"
	LabelFailed           = "failed"
	LabelCanceled         = "canceled"
	LabelAbandoned        = "abandoned"
	LabelPaused           = "paused"
	LabelDegraded         = "degraded"
	LabelInterrupted      = "interrupted"
	LabelUnknown          = "unknown"
)

// DaemonStateLabel returns the canonical display label for a daemon (Redis) task state.
// All CLI commands should call this when rendering a state from a daemon RPC response.
func DaemonStateLabel(status string) string {
	switch status {
	case StateDaemonQueued:
		return LabelQueued
	case StateDaemonRunning:
		return LabelRunning
	case StateDaemonAwaitingApproval:
		return LabelAwaitingApproval
	case StateDaemonCompleted:
		return LabelCompleted
	case StateDaemonFailed:
		return LabelFailed
	case StateDaemonCanceled:
		return LabelCanceled
	case StateDaemonAbandoned:
		return LabelAbandoned
	case StateDaemonPaused:
		return LabelPaused
	case StateDaemonDegraded:
		return LabelDegraded
	default:
		if status == "" {
			return LabelUnknown
		}
		return status
	}
}

// DaemonStateIcon returns a compact icon for a daemon task state.
// Icons are single-character to fit narrow columns.
func DaemonStateIcon(status string) string {
	switch status {
	case StateDaemonQueued:
		return "⏳"
	case StateDaemonRunning:
		return "⚙"
	case StateDaemonAwaitingApproval:
		return "👁"
	case StateDaemonCompleted:
		return "✓"
	case StateDaemonFailed:
		return "✗"
	case StateDaemonCanceled:
		return "⊘"
	case StateDaemonAbandoned:
		return "🗑"
	case StateDaemonPaused:
		return "⏸"
	case StateDaemonDegraded:
		return "⚠"
	default:
		return "?"
	}
}

// FileTaskLabel returns the canonical display label for a filesystem (engine) task status.
// Maps constants.TaskStatus values to the same canonical labels used by daemon states,
// so that `atlas status` and `atlas daemon status` show consistent vocabulary.
func FileTaskLabel(status constants.TaskStatus) string {
	switch status {
	case constants.TaskStatusPending:
		// "pending" is the filesystem equivalent of daemon "queued"
		return LabelQueued
	case constants.TaskStatusRunning, constants.TaskStatusValidating:
		return LabelRunning
	case constants.TaskStatusAwaitingApproval:
		return LabelAwaitingApproval
	case constants.TaskStatusCompleted:
		return LabelCompleted
	case constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusRejected:
		// Multiple failure modes map to the canonical "failed" label.
		// The specific subtype is available via constants.TaskStatus.String() if needed.
		return LabelFailed
	case constants.TaskStatusInterrupted:
		return LabelInterrupted
	case constants.TaskStatusAbandoned:
		return LabelAbandoned
	default:
		if status == "" {
			return LabelUnknown
		}
		return string(status)
	}
}

// FileTaskIcon returns a compact icon for a filesystem task status.
func FileTaskIcon(status constants.TaskStatus) string {
	switch status {
	case constants.TaskStatusPending:
		return "⏳"
	case constants.TaskStatusRunning, constants.TaskStatusValidating:
		return "⚙"
	case constants.TaskStatusAwaitingApproval:
		return "👁"
	case constants.TaskStatusCompleted:
		return "✓"
	case constants.TaskStatusValidationFailed,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
		constants.TaskStatusRejected:
		return "✗"
	case constants.TaskStatusInterrupted:
		return "⏸"
	case constants.TaskStatusAbandoned:
		return "🗑"
	default:
		return "?"
	}
}

// IsTerminalDaemonState reports whether a daemon state string is terminal
// (completed, failed, canceled, or abandoned).
func IsTerminalDaemonState(status string) bool {
	switch status {
	case StateDaemonCompleted, StateDaemonFailed, StateDaemonCanceled, StateDaemonAbandoned:
		return true
	default:
		return false
	}
}

// IsActiveDaemonState reports whether a daemon state represents an in-progress task
// (queued, running, awaiting_approval, or paused).
func IsActiveDaemonState(status string) bool {
	switch status {
	case StateDaemonQueued, StateDaemonRunning, StateDaemonAwaitingApproval, StateDaemonPaused:
		return true
	default:
		return false
	}
}
