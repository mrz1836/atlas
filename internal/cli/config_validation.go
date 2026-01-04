// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
)

// ConfigValidationFlags holds flags specific to the config validation command.
type ConfigValidationFlags struct {
	// NoInteractive skips all prompts and shows current values.
	NoInteractive bool
}

// newConfigValidationCmd creates the 'config validation' subcommand for validation configuration.
func newConfigValidationCmd(flags *ConfigValidationFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validation",
		Short: "Configure validation command settings",
		Long: `Configure validation command settings for ATLAS.

This command allows you to update your validation configuration settings without
running the full init wizard. It supports:
  - Format commands (code formatting)
  - Lint commands (code linting)
  - Test commands (running tests)
  - Pre-commit commands (git hooks)
  - Custom pre-PR hooks

Commands are validated against your PATH and warnings are shown for missing executables.

Use --no-interactive to show current values without prompting.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigValidation(cmd.Context(), cmd.OutOrStdout(), flags)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&flags.NoInteractive, "no-interactive", false, "show current values without prompting")

	return cmd
}

// AddConfigValidationCommand adds the 'validation' subcommand to the config command.
// This is called from config_ai.go where the config command is defined.
func AddConfigValidationCommand(configCmd *cobra.Command) {
	flags := &ConfigValidationFlags{}
	configCmd.AddCommand(newConfigValidationCmd(flags))
}

// runConfigValidation executes the config validation command.
func runConfigValidation(ctx context.Context, w io.Writer, flags *ConfigValidationFlags) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	styles := newConfigValidationStyles()

	// Load and display existing configuration
	existingCfg, configPath, err := loadExistingConfig()
	if err != nil {
		_, _ = fmt.Fprintln(w, styles.warning.Render("⚠ No existing configuration found. Run 'atlas init' first or create a new configuration."))
	}
	if existingCfg != nil {
		displayCurrentValidationConfig(w, existingCfg, styles)
	}

	// Handle non-interactive mode
	if flags.NoInteractive {
		return handleNonInteractiveMode(w, existingCfg, styles)
	}

	// Collect and save new configuration
	valCfg, err := collectValidationConfigStandalone(ctx, w, existingCfg, styles)
	if err != nil {
		return err
	}

	// Display validation warnings
	displayCommandWarnings(w, valCfg, styles)

	// Merge with existing config (or create new one)
	var finalCfg AtlasConfig
	if existingCfg != nil {
		finalCfg = *existingCfg
	}
	finalCfg.Validation = valCfg.ToValidationConfig()

	// Save configuration using shared function
	if err = saveAtlasConfig(finalCfg, "Updated by atlas config validation"); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Display success
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.success.Render("✓ Validation configuration updated successfully!"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("Configuration saved to: "+configPath))

	return nil
}

// handleNonInteractiveMode handles the --no-interactive flag case.
func handleNonInteractiveMode(w io.Writer, existingCfg *AtlasConfig, styles *configValidationStyles) error {
	if existingCfg == nil {
		_, _ = fmt.Fprintln(w, styles.dim.Render("No validation configuration found. Run 'atlas config validation' interactively to configure."))
	}
	return nil
}

// collectValidationConfigStandalone collects validation configuration in standalone mode.
func collectValidationConfigStandalone(ctx context.Context, w io.Writer, existingCfg *AtlasConfig, styles *configValidationStyles) (*ValidationProviderConfig, error) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Update Validation Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 35)))

	// Run tool detection to get default suggestions
	detector := &defaultToolDetector{}
	toolResult, _ := detector.Detect(ctx)

	// Prepare config from existing or defaults
	valCfg := prepareValidationProviderConfig(existingCfg, toolResult)

	// Run the main form
	form := NewValidationConfigForm(valCfg)
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("validation configuration failed: %w", err)
	}

	return valCfg, nil
}

// prepareValidationProviderConfig creates a ValidationProviderConfig from existing config or defaults.
func prepareValidationProviderConfig(existingCfg *AtlasConfig, toolResult *config.ToolDetectionResult) *ValidationProviderConfig {
	valCfg := &ValidationProviderConfig{}
	if existingCfg != nil {
		valCfg.FormatCmds = strings.Join(existingCfg.Validation.Commands.Format, "\n")
		valCfg.LintCmds = strings.Join(existingCfg.Validation.Commands.Lint, "\n")
		valCfg.TestCmds = strings.Join(existingCfg.Validation.Commands.Test, "\n")
		valCfg.PreCommitCmds = strings.Join(existingCfg.Validation.Commands.PreCommit, "\n")
		valCfg.CustomPrePR = strings.Join(existingCfg.Validation.Commands.CustomPrePR, "\n")
	} else {
		PopulateValidationConfigDefaults(valCfg, toolResult)
	}
	return valCfg
}

// displayCommandWarnings shows validation warnings for configured commands.
func displayCommandWarnings(w io.Writer, valCfg *ValidationProviderConfig, styles *configValidationStyles) {
	warnings := ValidateAllConfigCommands(valCfg)
	if len(warnings) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.warning.Render("⚠ Command validation warnings:"))
	for category, categoryWarnings := range warnings {
		for _, warning := range categoryWarnings {
			_, _ = fmt.Fprintf(w, "  %s: %s\n", styles.key.Render(category), styles.dim.Render(warning))
		}
	}
	_, _ = fmt.Fprintln(w, styles.dim.Render("  (These commands may fail when run. You can continue anyway.)"))
}

// configValidationStyles contains styling for the config validation command output.
type configValidationStyles struct {
	header  lipgloss.Style
	success lipgloss.Style
	warning lipgloss.Style
	dim     lipgloss.Style
	key     lipgloss.Style
	value   lipgloss.Style
}

// newConfigValidationStyles creates styles for config validation command output.
func newConfigValidationStyles() *configValidationStyles {
	return &configValidationStyles{
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

// displayCurrentValidationConfig shows the current validation configuration.
func displayCurrentValidationConfig(w io.Writer, cfg *AtlasConfig, styles *configValidationStyles) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Current Validation Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 35)))

	displayCommandCategory(w, "Format Commands", cfg.Validation.Commands.Format, styles)
	displayCommandCategory(w, "Lint Commands", cfg.Validation.Commands.Lint, styles)
	displayCommandCategory(w, "Test Commands", cfg.Validation.Commands.Test, styles)
	displayCommandCategory(w, "Pre-commit Commands", cfg.Validation.Commands.PreCommit, styles)
	displayCommandCategory(w, "Custom Pre-PR Hooks", cfg.Validation.Commands.CustomPrePR, styles)
}

// displayCommandCategory displays a category of commands.
func displayCommandCategory(w io.Writer, category string, cmds []string, styles *configValidationStyles) {
	_, _ = fmt.Fprintf(w, "%s:\n", styles.key.Render(category))
	if len(cmds) == 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", styles.dim.Render("(none configured)"))
	} else {
		for _, cmd := range cmds {
			// Check if command exists
			exists, _ := ValidateCommand(cmd)
			statusIcon := "✓"
			if !exists {
				statusIcon = "⚠"
			}
			_, _ = fmt.Fprintf(w, "  %s %s\n", statusIcon, styles.value.Render(cmd))
		}
	}
}

// runToolDetection runs tool detection and returns the result.
// Used by both init and config commands.
func runToolDetection(ctx context.Context) (*config.ToolDetectionResult, error) {
	return config.NewToolDetector().Detect(ctx)
}
