// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/ctxutil"
)

// ConfigNotificationFlags holds flags specific to the config notifications command.
type ConfigNotificationFlags struct {
	// NoInteractive skips all prompts and shows current values.
	NoInteractive bool
}

// newConfigNotificationCmd creates the 'config notifications' subcommand for notification configuration.
func newConfigNotificationCmd(flags *ConfigNotificationFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Configure notification settings",
		Long: `Configure notification settings for ATLAS.

This command allows you to update your notification configuration settings without
running the full init wizard. It supports:
  - Terminal bell enable/disable
  - Notification event selection (task awaiting approval, validation failed, etc.)

Use --no-interactive to show current values without prompting.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigNotification(cmd.Context(), cmd.OutOrStdout(), flags)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&flags.NoInteractive, "no-interactive", false, "show current values without prompting")

	return cmd
}

// AddConfigNotificationCommand adds the 'notifications' subcommand to the config command.
// This is called from config_ai.go where the config command is defined.
func AddConfigNotificationCommand(configCmd *cobra.Command) {
	flags := &ConfigNotificationFlags{}
	configCmd.AddCommand(newConfigNotificationCmd(flags))
}

// runConfigNotification executes the config notifications command.
func runConfigNotification(ctx context.Context, w io.Writer, flags *ConfigNotificationFlags) error {
	// Check cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	styles := newConfigNotificationStyles()

	// Load and display existing configuration
	existingCfg, configPath, err := loadExistingConfig()
	if err != nil {
		_, _ = fmt.Fprintln(w, styles.warning.Render("⚠ No existing configuration found. Run 'atlas init' first or create a new configuration."))
	}
	if existingCfg != nil {
		displayCurrentNotificationConfig(w, existingCfg, styles)
	}

	// Handle non-interactive mode
	if flags.NoInteractive {
		if existingCfg == nil {
			_, _ = fmt.Fprintln(w, styles.dim.Render("No notification configuration found. Run 'atlas config notifications' interactively to configure."))
		}
		return nil
	}

	// Collect new configuration
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Update Notification Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 35)))

	// Start with existing values or defaults
	notifyCfg := &NotificationProviderConfig{}
	if existingCfg != nil {
		notifyCfg.BellEnabled = existingCfg.Notifications.BellEnabled
		notifyCfg.Events = existingCfg.Notifications.Events
	} else {
		defaults := NotificationConfigDefaults()
		notifyCfg.BellEnabled = defaults.BellEnabled
		notifyCfg.Events = defaults.Events
	}

	if err = CollectNotificationConfigInteractive(ctx, notifyCfg); err != nil {
		return fmt.Errorf("notification configuration failed: %w", err)
	}

	// Merge with existing config (or create new one)
	var finalCfg AtlasConfig
	if existingCfg != nil {
		finalCfg = *existingCfg
	}
	finalCfg.Notifications = notifyCfg.ToNotificationConfig()

	// Save configuration using shared function
	if err = saveAtlasConfig(finalCfg, "Updated by atlas config notifications"); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Display success
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.success.Render("✓ Notification configuration updated successfully!"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("Configuration saved to: "+configPath))

	return nil
}

// configNotificationStyles contains styling for the config notifications command output.
type configNotificationStyles struct {
	header  lipgloss.Style
	success lipgloss.Style
	warning lipgloss.Style
	dim     lipgloss.Style
	key     lipgloss.Style
	value   lipgloss.Style
}

// newConfigNotificationStyles creates styles for config notifications command output.
func newConfigNotificationStyles() *configNotificationStyles {
	return &configNotificationStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D7FF")).
			MarginBottom(1),
		success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
		key: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		value: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")),
	}
}

// displayCurrentNotificationConfig shows the current notification configuration.
func displayCurrentNotificationConfig(w io.Writer, cfg *AtlasConfig, styles *configNotificationStyles) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Current Notification Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 35)))

	// Display bell status
	bellStatus := "Disabled"
	if cfg.Notifications.BellEnabled {
		bellStatus = "Enabled"
	}
	_, _ = fmt.Fprintf(w, "%s: %s\n",
		styles.key.Render("Terminal Bell"),
		styles.value.Render(bellStatus))

	// Display events
	_, _ = fmt.Fprintf(w, "%s:\n", styles.key.Render("Notification Events"))
	if len(cfg.Notifications.Events) == 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", styles.dim.Render("(none configured)"))
	} else {
		for _, event := range cfg.Notifications.Events {
			_, _ = fmt.Fprintf(w, "  ✓ %s\n", styles.value.Render(formatEventName(event)))
		}
	}
}

// formatEventName converts an event constant to a human-readable name.
func formatEventName(event string) string {
	switch event {
	case NotifyEventAwaitingApproval:
		return "Task awaiting approval"
	case NotifyEventValidationFailed:
		return "Validation failed"
	case NotifyEventCIFailed:
		return "CI failed"
	case NotifyEventGitHubFailed:
		return "GitHub operation failed"
	default:
		return event
	}
}
