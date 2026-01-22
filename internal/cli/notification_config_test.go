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

func TestCollectNotificationConfigInteractive_InitializesDefaults(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createNotificationConfigForm
	defer func() { createNotificationConfigForm = originalFactory }()

	var capturedConfig *NotificationProviderConfig
	createNotificationConfigForm = func(cfg *NotificationProviderConfig) formRunner {
		capturedConfig = cfg
		return &mockFormRunner{}
	}

	cfg := &NotificationProviderConfig{}
	err := CollectNotificationConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	// Defaults should have been applied before form ran
	assert.Len(t, capturedConfig.Events, 4, "Events should be populated with defaults")
	assert.Contains(t, capturedConfig.Events, NotifyEventAwaitingApproval)
	assert.Contains(t, capturedConfig.Events, NotifyEventValidationFailed)
	assert.Contains(t, capturedConfig.Events, NotifyEventCIFailed)
	assert.Contains(t, capturedConfig.Events, NotifyEventGitHubFailed)
}

func TestCollectNotificationConfigInteractive_PreservesExistingEvents(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createNotificationConfigForm
	defer func() { createNotificationConfigForm = originalFactory }()

	var capturedConfig *NotificationProviderConfig
	createNotificationConfigForm = func(cfg *NotificationProviderConfig) formRunner {
		capturedConfig = cfg
		return &mockFormRunner{}
	}

	cfg := &NotificationProviderConfig{
		BellEnabled: false,
		Events:      []string{NotifyEventCIFailed},
	}

	err := CollectNotificationConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	// Events should be preserved when non-empty
	assert.Equal(t, []string{NotifyEventCIFailed}, capturedConfig.Events)
	assert.Len(t, capturedConfig.Events, 1, "Should still have 1 event")
}

func TestCollectNotificationConfigInteractive_ValidInput(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createNotificationConfigForm
	defer func() { createNotificationConfigForm = originalFactory }()

	// Mock the form to simulate user input
	createNotificationConfigForm = func(cfg *NotificationProviderConfig) formRunner {
		// Simulate user modifying values in the form
		cfg.BellEnabled = false
		cfg.Events = []string{NotifyEventAwaitingApproval, NotifyEventCIFailed}
		return &mockFormRunner{}
	}

	cfg := &NotificationProviderConfig{}
	err := CollectNotificationConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, cfg.BellEnabled, "Bell should be disabled")
	assert.Len(t, cfg.Events, 2, "Should have 2 events")
	assert.Contains(t, cfg.Events, NotifyEventAwaitingApproval)
	assert.Contains(t, cfg.Events, NotifyEventCIFailed)
}

func TestCollectNotificationConfigInteractive_FormError(t *testing.T) {
	originalFactory := createNotificationConfigForm
	defer func() { createNotificationConfigForm = originalFactory }()

	expectedErr := assert.AnError
	createNotificationConfigForm = func(_ *NotificationProviderConfig) formRunner {
		return &mockFormRunner{runErr: expectedErr}
	}

	cfg := &NotificationProviderConfig{}
	err := CollectNotificationConfigInteractive(context.Background(), cfg)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestNewNotificationConfigForm(t *testing.T) {
	cfg := &NotificationProviderConfig{
		BellEnabled: true,
		Events:      []string{NotifyEventAwaitingApproval},
	}

	form := NewNotificationConfigForm(cfg)

	assert.NotNil(t, form, "Form should not be nil")
	// The form should be created successfully with the provided config
	// We can't test form interactions without a TTY, but we can verify creation
}

func TestNewNotificationConfigForm_EmptyConfig(t *testing.T) {
	cfg := &NotificationProviderConfig{}

	form := NewNotificationConfigForm(cfg)

	assert.NotNil(t, form, "Form should not be nil even with empty config")
	// Form should handle empty config gracefully
}

func TestNewNotificationConfigForm_AllEventsSelected(t *testing.T) {
	cfg := &NotificationProviderConfig{
		BellEnabled: true,
		Events:      AllNotificationEvents(),
	}

	form := NewNotificationConfigForm(cfg)

	assert.NotNil(t, form, "Form should not be nil")
	assert.True(t, cfg.BellEnabled, "Bell should be enabled")
	assert.Len(t, cfg.Events, 4, "Should have all 4 events")
}

func TestNewNotificationConfigForm_BellDisabled(t *testing.T) {
	cfg := &NotificationProviderConfig{
		BellEnabled: false,
		Events:      []string{},
	}

	form := NewNotificationConfigForm(cfg)

	assert.NotNil(t, form, "Form should not be nil")
	assert.False(t, cfg.BellEnabled, "Bell should be disabled")
	assert.Empty(t, cfg.Events, "Events should be empty")
}

func TestPopulateNotificationConfigDefaults_OverwritesExisting(t *testing.T) {
	cfg := &NotificationProviderConfig{
		BellEnabled: false,
		Events:      []string{NotifyEventCIFailed},
	}

	PopulateNotificationConfigDefaults(cfg)

	// Verify defaults overwrite existing values
	assert.True(t, cfg.BellEnabled, "Bell should be enabled after populating defaults")
	assert.Len(t, cfg.Events, 4, "Should have 4 default events")
	assert.Contains(t, cfg.Events, NotifyEventAwaitingApproval)
	assert.Contains(t, cfg.Events, NotifyEventValidationFailed)
	assert.Contains(t, cfg.Events, NotifyEventCIFailed)
	assert.Contains(t, cfg.Events, NotifyEventGitHubFailed)
}

func TestNotificationConfigDefaults_ReturnsNewInstance(t *testing.T) {
	cfg1 := NotificationConfigDefaults()
	cfg2 := NotificationConfigDefaults()

	// Modify first config
	cfg1.BellEnabled = false
	cfg1.Events = []string{}

	// Second config should still have defaults
	assert.True(t, cfg2.BellEnabled, "Second config should have default bell enabled")
	assert.Len(t, cfg2.Events, 4, "Second config should have all default events")
}

func TestDefaultNotificationConfig_ReturnsNotificationConfig(t *testing.T) {
	cfg := DefaultNotificationConfig()

	// Verify it returns NotificationConfig type (not NotificationProviderConfig)
	assert.True(t, cfg.BellEnabled)
	assert.Len(t, cfg.Events, 4)

	// Verify it contains the same values as NotificationConfigDefaults
	defaults := NotificationConfigDefaults()
	assert.Equal(t, defaults.BellEnabled, cfg.BellEnabled)
	assert.Equal(t, defaults.Events, cfg.Events)
}

func TestDefaultCreateNotificationConfigForm(t *testing.T) {
	cfg := &NotificationProviderConfig{
		BellEnabled: true,
		Events:      []string{NotifyEventAwaitingApproval},
	}

	form := defaultCreateNotificationConfigForm(cfg)

	assert.NotNil(t, form, "Form should not be nil")
	// This tests the production form factory function
}
