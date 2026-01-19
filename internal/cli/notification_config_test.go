package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationConfigDefaults(t *testing.T) {
	cfg := NotificationConfigDefaults()

	assert.True(t, cfg.BellEnabled, "Bell should be enabled by default")
	assert.Contains(t, cfg.Events, NotifyEventAwaitingApproval)
	assert.Contains(t, cfg.Events, NotifyEventValidationFailed)
	assert.Contains(t, cfg.Events, NotifyEventCIFailed)
	assert.Contains(t, cfg.Events, NotifyEventGitHubFailed)
	assert.Len(t, cfg.Events, 4, "Should have 4 default events")
}

func TestDefaultNotificationConfig(t *testing.T) {
	cfg := DefaultNotificationConfig()

	assert.True(t, cfg.BellEnabled, "Bell should be enabled by default")
	assert.Contains(t, cfg.Events, NotifyEventAwaitingApproval)
	assert.Contains(t, cfg.Events, NotifyEventValidationFailed)
	assert.Contains(t, cfg.Events, NotifyEventCIFailed)
	assert.Contains(t, cfg.Events, NotifyEventGitHubFailed)
	assert.Len(t, cfg.Events, 4, "Should have 4 default events")
}

func TestAllNotificationEvents(t *testing.T) {
	events := AllNotificationEvents()

	assert.Len(t, events, 4, "Should return 4 events")
	assert.Contains(t, events, NotifyEventAwaitingApproval)
	assert.Contains(t, events, NotifyEventValidationFailed)
	assert.Contains(t, events, NotifyEventCIFailed)
	assert.Contains(t, events, NotifyEventGitHubFailed)
}

func TestNotificationEventConstants(t *testing.T) {
	// Verify event constants have expected string values
	assert.Equal(t, "awaiting_approval", NotifyEventAwaitingApproval)
	assert.Equal(t, "validation_failed", NotifyEventValidationFailed)
	assert.Equal(t, "ci_failed", NotifyEventCIFailed)
	assert.Equal(t, "github_failed", NotifyEventGitHubFailed)
}

func TestNotificationProviderConfig_ToNotificationConfig(t *testing.T) {
	provider := &NotificationProviderConfig{
		BellEnabled: true,
		Events:      []string{NotifyEventAwaitingApproval, NotifyEventCIFailed},
	}

	cfg := provider.ToNotificationConfig()

	assert.True(t, cfg.BellEnabled)
	assert.Equal(t, provider.Events, cfg.Events)
	assert.Len(t, cfg.Events, 2)
}

func TestNotificationProviderConfig_ToNotificationConfig_Empty(t *testing.T) {
	provider := &NotificationProviderConfig{
		BellEnabled: false,
		Events:      []string{},
	}

	cfg := provider.ToNotificationConfig()

	assert.False(t, cfg.BellEnabled)
	assert.Empty(t, cfg.Events)
}

func TestValidateNotificationConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *NotificationProviderConfig
		wantErr bool
	}{
		{
			name: "valid config with all events",
			cfg: &NotificationProviderConfig{
				BellEnabled: true,
				Events:      AllNotificationEvents(),
			},
			wantErr: false,
		},
		{
			name: "valid config with no events",
			cfg: &NotificationProviderConfig{
				BellEnabled: true,
				Events:      []string{},
			},
			wantErr: false,
		},
		{
			name: "valid config with bell disabled",
			cfg: &NotificationProviderConfig{
				BellEnabled: false,
				Events:      AllNotificationEvents(),
			},
			wantErr: false,
		},
		{
			name: "valid config with single event",
			cfg: &NotificationProviderConfig{
				BellEnabled: true,
				Events:      []string{NotifyEventCIFailed},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotificationConfig(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPopulateNotificationConfigDefaults(t *testing.T) {
	cfg := &NotificationProviderConfig{}

	PopulateNotificationConfigDefaults(cfg)

	assert.True(t, cfg.BellEnabled)
	assert.Len(t, cfg.Events, 4, "Should have 4 default events")
	assert.Contains(t, cfg.Events, NotifyEventAwaitingApproval)
}

func TestPopulateNotificationConfigDefaults_NilConfig(_ *testing.T) {
	// Should not panic on nil config
	PopulateNotificationConfigDefaults(nil)
}

func TestCollectNotificationConfigNonInteractive(t *testing.T) {
	cfg := CollectNotificationConfigNonInteractive()

	assert.True(t, cfg.BellEnabled)
	assert.Len(t, cfg.Events, 4)
}

func TestCollectNotificationConfigInteractive_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := &NotificationProviderConfig{}
	err := CollectNotificationConfigInteractive(ctx, cfg)

	assert.ErrorIs(t, err, context.Canceled)
}
