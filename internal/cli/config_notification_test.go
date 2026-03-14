package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigNotificationCmd_NoInteractive_NoConfig(t *testing.T) {
	// Use temp HOME to isolate from real config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	flags := &ConfigNotificationFlags{NoInteractive: true}

	err := runConfigNotification(context.Background(), &buf, flags)

	require.NoError(t, err)
	// Should show warning about no config and instruction to configure
	assert.Contains(t, buf.String(), "No existing configuration found")
}

func TestConfigNotificationStyles(t *testing.T) {
	styles := newConfigNotificationStyles()

	assert.NotNil(t, styles.header)
	assert.NotNil(t, styles.success)
	assert.NotNil(t, styles.warning)
	assert.NotNil(t, styles.dim)
	assert.NotNil(t, styles.key)
	assert.NotNil(t, styles.value)
}

func TestFormatEventName(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{NotifyEventAwaitingApproval, "Task awaiting approval"},
		{NotifyEventValidationFailed, "Validation failed"},
		{NotifyEventCIFailed, "CI failed"},
		{NotifyEventGitHubFailed, "GitHub operation failed"},
		{"unknown_event", "unknown_event"},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := formatEventName(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDisplayCurrentNotificationConfig(t *testing.T) {
	var buf bytes.Buffer
	styles := newConfigNotificationStyles()
	cfg := &AtlasConfig{
		Notifications: NotificationConfig{
			BellEnabled: true,
			Events:      []string{NotifyEventAwaitingApproval, NotifyEventCIFailed},
		},
	}

	displayCurrentNotificationConfig(&buf, cfg, styles)

	output := buf.String()
	assert.Contains(t, output, "Current Notification Configuration")
	assert.Contains(t, output, "Terminal Bell")
	assert.Contains(t, output, "Enabled")
	assert.Contains(t, output, "Notification Events")
	assert.Contains(t, output, "Task awaiting approval")
	assert.Contains(t, output, "CI failed")
}

func TestDisplayCurrentNotificationConfig_BellDisabled(t *testing.T) {
	var buf bytes.Buffer
	styles := newConfigNotificationStyles()
	cfg := &AtlasConfig{
		Notifications: NotificationConfig{
			BellEnabled: false,
			Events:      AllNotificationEvents(),
		},
	}

	displayCurrentNotificationConfig(&buf, cfg, styles)

	output := buf.String()
	assert.Contains(t, output, "Disabled")
}

func TestDisplayCurrentNotificationConfig_NoEvents(t *testing.T) {
	var buf bytes.Buffer
	styles := newConfigNotificationStyles()
	cfg := &AtlasConfig{
		Notifications: NotificationConfig{
			BellEnabled: true,
			Events:      []string{},
		},
	}

	displayCurrentNotificationConfig(&buf, cfg, styles)

	output := buf.String()
	assert.Contains(t, output, "(none configured)")
}

func TestRunConfigNotification_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	flags := &ConfigNotificationFlags{}

	err := runConfigNotification(ctx, &buf, flags)

	assert.ErrorIs(t, err, context.Canceled)
}
