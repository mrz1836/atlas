// Package task provides task lifecycle management for ATLAS.
//
// This file implements state change notifications for the task engine.
// It emits terminal bell notifications when tasks transition to attention-required states.
//
// Import rules:
//   - CAN import: internal/constants, std lib
//   - MUST NOT import: internal/tui, internal/workspace, internal/ai, internal/cli
package task

import (
	"io"
	"os"

	"github.com/mrz1836/atlas/internal/constants"
)

// attentionStatuses defines task statuses that require user attention.
// These states should trigger a bell notification when transitioned to.
// This mirrors tui.IsAttentionStatus() but avoids an import cycle.
//
//nolint:gochecknoglobals // Read-only lookup table for attention status checks
var attentionStatuses = map[constants.TaskStatus]bool{
	constants.TaskStatusValidationFailed: true,
	constants.TaskStatusAwaitingApproval: true,
	constants.TaskStatusGHFailed:         true,
	constants.TaskStatusCIFailed:         true,
	constants.TaskStatusCITimeout:        true,
}

// isAttentionStatus returns true if the status requires user attention.
// This function is package-local to avoid duplicating tui.IsAttentionStatus.
func isAttentionStatus(status constants.TaskStatus) bool {
	return attentionStatuses[status]
}

// NotificationConfig holds configuration for bell notifications.
type NotificationConfig struct {
	// BellEnabled controls whether terminal bell notifications are enabled.
	BellEnabled bool

	// Quiet suppresses all notifications.
	Quiet bool

	// Events is the list of event types that trigger notifications.
	// Supported: "awaiting_approval", "validation_failed", "task_complete", "error"
	Events []string
}

// DefaultNotificationConfig returns sensible defaults.
// These defaults should match config.DefaultConfig().Notifications for consistency.
func DefaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
}

// StateChangeNotifier handles notifications for task state transitions.
// It emits a terminal bell when tasks transition to attention-required states.
type StateChangeNotifier struct {
	config NotificationConfig
	writer io.Writer
}

// NewStateChangeNotifier creates a notifier with the given configuration.
func NewStateChangeNotifier(cfg NotificationConfig) *StateChangeNotifier {
	return &StateChangeNotifier{
		config: cfg,
		writer: os.Stdout,
	}
}

// NewStateChangeNotifierWithWriter creates a notifier with a custom writer.
// This is useful for testing.
func NewStateChangeNotifierWithWriter(cfg NotificationConfig, w io.Writer) *StateChangeNotifier {
	return &StateChangeNotifier{
		config: cfg,
		writer: w,
	}
}

// NotifyStateChange emits a bell notification if the state change warrants it.
// It checks:
// 1. Bell is enabled and not in quiet mode
// 2. The new status is an attention-required status
// 3. The old status was NOT an attention-required status (only bell on NEW transitions)
// 4. The event type is in the configured events list
func (n *StateChangeNotifier) NotifyStateChange(oldStatus, newStatus constants.TaskStatus) {
	if n == nil {
		return
	}

	// Check if notifications are enabled
	if !n.config.BellEnabled || n.config.Quiet {
		return
	}

	// Only bell on transitions TO attention states (not within attention states)
	if !isAttentionStatus(newStatus) {
		return
	}

	// Don't bell if we were already in an attention state
	if isAttentionStatus(oldStatus) {
		return
	}

	// Check if this event type is configured for notifications
	if !n.shouldNotifyForStatus(newStatus) {
		return
	}

	// Emit bell
	n.emitBell()
}

// Bell emits a terminal bell if enabled and not in quiet mode.
// This method can be called directly for manual bell emission.
func (n *StateChangeNotifier) Bell() {
	if n == nil {
		return
	}

	if !n.config.BellEnabled || n.config.Quiet {
		return
	}

	n.emitBell()
}

// shouldNotifyForStatus checks if the status matches a configured event type.
func (n *StateChangeNotifier) shouldNotifyForStatus(status constants.TaskStatus) bool {
	eventType := statusToEventType(status)
	if eventType == "" {
		return false
	}

	for _, event := range n.config.Events {
		if event == eventType {
			return true
		}
	}

	return false
}

// emitBell writes the terminal bell character to the configured writer.
func (n *StateChangeNotifier) emitBell() {
	_, _ = n.writer.Write([]byte("\a"))
}

// statusToEventType maps task statuses to notification event types.
// Only attention-requiring statuses have mappings; others return empty string.
func statusToEventType(status constants.TaskStatus) string {
	//nolint:exhaustive // Only attention statuses need event type mappings
	switch status {
	case constants.TaskStatusAwaitingApproval:
		return "awaiting_approval"
	case constants.TaskStatusValidationFailed:
		return "validation_failed"
	case constants.TaskStatusGHFailed, constants.TaskStatusCIFailed, constants.TaskStatusCITimeout:
		return "error"
	default:
		return ""
	}
}
