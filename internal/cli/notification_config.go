// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"

	"github.com/charmbracelet/huh"
)

// Notification event constants.
// These define the events that can trigger notifications.
const (
	NotifyEventAwaitingApproval = "awaiting_approval"
	NotifyEventValidationFailed = "validation_failed"
	NotifyEventCIFailed         = "ci_failed"
	NotifyEventGitHubFailed     = "github_failed"
)

// AllNotificationEvents returns all supported notification event types.
func AllNotificationEvents() []string {
	return []string{
		NotifyEventAwaitingApproval,
		NotifyEventValidationFailed,
		NotifyEventCIFailed,
		NotifyEventGitHubFailed,
	}
}

// NotificationProviderConfig holds notification configuration values collected from user input.
// This struct is used for collecting configuration before saving.
type NotificationProviderConfig struct {
	// BellEnabled enables terminal bell notifications.
	BellEnabled bool
	// Events is the list of events to notify on.
	Events []string
}

// NotificationConfigDefaults returns the default values for notification configuration.
func NotificationConfigDefaults() NotificationProviderConfig {
	return NotificationProviderConfig{
		BellEnabled: true,
		Events:      AllNotificationEvents(),
	}
}

// DefaultNotificationConfig returns a NotificationConfig with sensible defaults.
// Bell is enabled and all events are selected.
func DefaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		BellEnabled: true,
		Events:      AllNotificationEvents(),
	}
}

// NewNotificationConfigForm creates a Charm Huh form for notification configuration.
// The form collects bell enable/disable and event selection settings.
func NewNotificationConfigForm(cfg *NotificationProviderConfig) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Terminal Bell").
				Description("Play a sound when ATLAS needs your attention").
				Affirmative("Yes").
				Negative("No").
				Value(&cfg.BellEnabled),
			huh.NewMultiSelect[string]().
				Title("Notification Events").
				Description("Select events that should trigger notifications").
				Options(
					huh.NewOption("Task awaiting approval", NotifyEventAwaitingApproval).Selected(true),
					huh.NewOption("Validation failed", NotifyEventValidationFailed).Selected(true),
					huh.NewOption("CI failed", NotifyEventCIFailed).Selected(true),
					huh.NewOption("GitHub operation failed", NotifyEventGitHubFailed).Selected(true),
				).
				Value(&cfg.Events),
		),
	).WithTheme(huh.ThemeCharm())
}

// CollectNotificationConfigInteractive runs the notification configuration form and returns the collected config.
// It validates all inputs and handles form errors.
func CollectNotificationConfigInteractive(ctx context.Context, cfg *NotificationProviderConfig) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	defaults := NotificationConfigDefaults()

	// Initialize Events with defaults if empty (form will show current selections).
	// BellEnabled is handled by the form's default value directly.
	if len(cfg.Events) == 0 {
		cfg.Events = defaults.Events
	}

	form := NewNotificationConfigForm(cfg)
	if err := form.Run(); err != nil {
		return err
	}

	return nil
}

// CollectNotificationConfigNonInteractive returns a configuration with default values.
// Used when running in non-interactive mode.
func CollectNotificationConfigNonInteractive() NotificationProviderConfig {
	return NotificationConfigDefaults()
}

// ToNotificationConfig converts the provider config to NotificationConfig struct.
func (cfg *NotificationProviderConfig) ToNotificationConfig() NotificationConfig {
	return NotificationConfig{
		BellEnabled: cfg.BellEnabled,
		Events:      cfg.Events,
	}
}

// ValidateNotificationConfig validates a NotificationProviderConfig.
// Returns an error if the configuration is invalid.
func ValidateNotificationConfig(_ *NotificationProviderConfig) error {
	// Events can be empty (user may not want any notifications)
	// BellEnabled is always valid (true or false)
	// No validation errors for notification config
	return nil
}

// PopulateNotificationConfigDefaults populates the config with default values.
func PopulateNotificationConfigDefaults(cfg *NotificationProviderConfig) {
	if cfg == nil {
		return
	}

	defaults := NotificationConfigDefaults()
	cfg.BellEnabled = defaults.BellEnabled
	cfg.Events = defaults.Events
}
