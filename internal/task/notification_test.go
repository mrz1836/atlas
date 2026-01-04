package task

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestIsAttentionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   constants.TaskStatus
		expected bool
	}{
		{"ValidationFailed is attention", constants.TaskStatusValidationFailed, true},
		{"AwaitingApproval is attention", constants.TaskStatusAwaitingApproval, true},
		{"GHFailed is attention", constants.TaskStatusGHFailed, true},
		{"CIFailed is attention", constants.TaskStatusCIFailed, true},
		{"CITimeout is attention", constants.TaskStatusCITimeout, true},
		{"Pending is not attention", constants.TaskStatusPending, false},
		{"Running is not attention", constants.TaskStatusRunning, false},
		{"Validating is not attention", constants.TaskStatusValidating, false},
		{"Completed is not attention", constants.TaskStatusCompleted, false},
		{"Rejected is not attention", constants.TaskStatusRejected, false},
		{"Abandoned is not attention", constants.TaskStatusAbandoned, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, isAttentionStatus(tt.status))
		})
	}
}

func TestStatusToEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   constants.TaskStatus
		expected string
	}{
		{"AwaitingApproval maps to awaiting_approval", constants.TaskStatusAwaitingApproval, "awaiting_approval"},
		{"ValidationFailed maps to validation_failed", constants.TaskStatusValidationFailed, "validation_failed"},
		{"GHFailed maps to github_failed", constants.TaskStatusGHFailed, "github_failed"},
		{"CIFailed maps to ci_failed", constants.TaskStatusCIFailed, "ci_failed"},
		{"CITimeout maps to ci_failed", constants.TaskStatusCITimeout, "ci_failed"},
		{"Running maps to empty", constants.TaskStatusRunning, ""},
		{"Completed maps to empty", constants.TaskStatusCompleted, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, statusToEventType(tt.status))
		})
	}
}

func TestNewStateChangeNotifier(t *testing.T) {
	t.Parallel()

	cfg := DefaultNotificationConfig()
	n := NewStateChangeNotifier(cfg)

	assert.NotNil(t, n)
	assert.True(t, n.config.BellEnabled)
	assert.False(t, n.config.Quiet)
}

func TestNewStateChangeNotifierWithWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := DefaultNotificationConfig()
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	assert.NotNil(t, n)
	assert.Equal(t, &buf, n.writer)
}

func TestStateChangeNotifier_Bell_EmitsBellCharacter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	n.Bell()

	assert.Equal(t, "\a", buf.String())
}

func TestStateChangeNotifier_Bell_DisabledDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: false,
		Quiet:       false,
		Events:      []string{"awaiting_approval"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	n.Bell()

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_Bell_QuietModeDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       true,
		Events:      []string{"awaiting_approval"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	n.Bell()

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_Bell_NilNotifierIsSafe(t *testing.T) {
	t.Parallel()

	var n *StateChangeNotifier
	// Should not panic
	n.Bell()
}

func TestStateChangeNotifier_NotifyStateChange_EmitsBellOnNewAttentionTransition(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	// Transition from Running to AwaitingApproval should bell
	n.NotifyStateChange(constants.TaskStatusRunning, constants.TaskStatusAwaitingApproval)

	assert.Equal(t, "\a", buf.String())
}

func TestStateChangeNotifier_NotifyStateChange_DoesNotBellWithinAttentionStates(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	// Transition from one attention state to another should NOT bell
	n.NotifyStateChange(constants.TaskStatusValidationFailed, constants.TaskStatusAwaitingApproval)

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_NotifyStateChange_DoesNotBellOnNonAttentionTransition(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	// Transition to non-attention state should NOT bell
	n.NotifyStateChange(constants.TaskStatusPending, constants.TaskStatusRunning)

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_NotifyStateChange_RespectsEventsFilter(t *testing.T) {
	t.Parallel()

	t.Run("configured event triggers bell", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		cfg := NotificationConfig{
			BellEnabled: true,
			Quiet:       false,
			Events:      []string{"awaiting_approval"}, // Only awaiting_approval
		}
		n := NewStateChangeNotifierWithWriter(cfg, &buf)

		n.NotifyStateChange(constants.TaskStatusRunning, constants.TaskStatusAwaitingApproval)

		assert.Equal(t, "\a", buf.String())
	})

	t.Run("non-configured event does not trigger bell", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		cfg := NotificationConfig{
			BellEnabled: true,
			Quiet:       false,
			Events:      []string{"awaiting_approval"}, // Only awaiting_approval
		}
		n := NewStateChangeNotifierWithWriter(cfg, &buf)

		// validation_failed is not in Events
		n.NotifyStateChange(constants.TaskStatusValidating, constants.TaskStatusValidationFailed)

		assert.Empty(t, buf.String())
	})
}

func TestStateChangeNotifier_NotifyStateChange_DisabledDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: false,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	n.NotifyStateChange(constants.TaskStatusRunning, constants.TaskStatusAwaitingApproval)

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_NotifyStateChange_QuietModeDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       true,
		Events:      []string{"awaiting_approval", "validation_failed", "error"},
	}
	n := NewStateChangeNotifierWithWriter(cfg, &buf)

	n.NotifyStateChange(constants.TaskStatusRunning, constants.TaskStatusAwaitingApproval)

	assert.Empty(t, buf.String())
}

func TestStateChangeNotifier_NotifyStateChange_NilNotifierIsSafe(t *testing.T) {
	t.Parallel()

	var n *StateChangeNotifier
	// Should not panic
	n.NotifyStateChange(constants.TaskStatusRunning, constants.TaskStatusAwaitingApproval)
}

func TestStateChangeNotifier_NotifyStateChange_AllAttentionStatuses(t *testing.T) {
	t.Parallel()

	attentionStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range attentionStatuses {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			cfg := NotificationConfig{
				BellEnabled: true,
				Quiet:       false,
				Events:      []string{"awaiting_approval", "validation_failed", "error"},
			}
			n := NewStateChangeNotifierWithWriter(cfg, &buf)

			n.NotifyStateChange(constants.TaskStatusRunning, status)

			assert.Equal(t, "\a", buf.String(), "Expected bell for status %s", status)
		})
	}
}

func TestDefaultNotificationConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultNotificationConfig()

	assert.True(t, cfg.BellEnabled)
	assert.False(t, cfg.Quiet)
	assert.Contains(t, cfg.Events, "awaiting_approval")
	assert.Contains(t, cfg.Events, "validation_failed")
	assert.Contains(t, cfg.Events, "ci_failed")
	assert.Contains(t, cfg.Events, "github_failed")
	assert.NotContains(t, cfg.Events, "error", "Default should not include legacy error event")
	assert.Len(t, cfg.Events, 4, "Should have 4 default events (granular only)")
}

func TestStateChangeNotifier_ShouldNotifyForStatus(t *testing.T) {
	t.Parallel()

	cfg := NotificationConfig{
		BellEnabled: true,
		Quiet:       false,
		Events:      []string{"awaiting_approval", "error"},
	}
	n := NewStateChangeNotifier(cfg)

	// Test that configured events return true
	assert.True(t, n.shouldNotifyForStatus(constants.TaskStatusAwaitingApproval))
	assert.True(t, n.shouldNotifyForStatus(constants.TaskStatusGHFailed)) // Maps to "error"
	assert.True(t, n.shouldNotifyForStatus(constants.TaskStatusCIFailed)) // Maps to "error"

	// Test that non-configured events return false
	assert.False(t, n.shouldNotifyForStatus(constants.TaskStatusValidationFailed)) // Not in Events
	assert.False(t, n.shouldNotifyForStatus(constants.TaskStatusRunning))          // No mapping
}
