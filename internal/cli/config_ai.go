// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/constants"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ConfigAIFlags holds flags specific to the config ai command.
type ConfigAIFlags struct {
	// NoInteractive skips all prompts and shows current values.
	NoInteractive bool
}

// newConfigAICmd creates the 'config ai' subcommand for AI configuration.
func newConfigAICmd(flags *ConfigAIFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ai",
		Short: "Configure AI provider settings",
		Long: `Configure AI provider settings for ATLAS.

This command allows you to update your AI configuration settings without
running the full init wizard. It supports:
  - Default AI model selection (sonnet, opus, haiku)
  - API key environment variable configuration
  - Timeout settings
  - Max turns per step

Use --no-interactive to show current values without prompting.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigAI(cmd.Context(), cmd.OutOrStdout(), flags)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&flags.NoInteractive, "no-interactive", false, "show current values without prompting")

	return cmd
}

// newConfigCmd creates the 'config' parent command.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage ATLAS configuration",
		Long: `Manage ATLAS configuration settings.

Subcommands:
  show          Display effective configuration with sources
  ai            Configure AI provider settings
  validation    Configure validation command settings
  notifications Configure notification settings

Example:
  atlas config show          # Show current config with source annotations
  atlas config ai            # Configure AI settings interactively
  atlas config validation    # Configure validation commands interactively
  atlas config notifications # Configure notification settings interactively`,
	}

	// Add subcommands
	aiFlags := &ConfigAIFlags{}
	cmd.AddCommand(newConfigAICmd(aiFlags))

	// Add validation subcommand
	AddConfigValidationCommand(cmd)

	// Add notifications subcommand
	AddConfigNotificationCommand(cmd)

	// Add show subcommand
	AddConfigShowCommand(cmd)

	return cmd
}

// AddConfigCommand adds the config command to the root command.
func AddConfigCommand(rootCmd *cobra.Command) {
	rootCmd.AddCommand(newConfigCmd())
}

// runConfigAI executes the config ai command.
func runConfigAI(ctx context.Context, w io.Writer, flags *ConfigAIFlags) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	styles := newConfigAIStyles()

	// Load existing configuration
	existingCfg, configPath, err := loadExistingConfig()
	if err != nil {
		_, _ = fmt.Fprintln(w, styles.warning.Render("⚠ No existing configuration found. Run 'atlas init' first or create a new configuration."))
	}

	// Display current configuration if exists
	if existingCfg != nil {
		displayCurrentAIConfig(w, existingCfg, styles)
	}

	// If non-interactive, just show current config
	if flags.NoInteractive {
		if existingCfg == nil {
			_, _ = fmt.Fprintln(w, styles.dim.Render("No AI configuration found. Run 'atlas config ai' interactively to configure."))
		}
		return nil
	}

	// Collect new configuration
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Update AI Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 30)))

	// Start with existing values or defaults
	aiCfg := &AIProviderConfig{}
	if existingCfg != nil {
		aiCfg.Model = existingCfg.AI.Model
		aiCfg.APIKeyEnvVar = existingCfg.AI.APIKeyEnvVar
		aiCfg.Timeout = existingCfg.AI.Timeout
		aiCfg.MaxTurns = existingCfg.AI.MaxTurns
	}

	if err = CollectAIConfigInteractive(ctx, aiCfg); err != nil {
		return err
	}

	// Check if API key environment variable is set
	if exists, warning := CheckAPIKeyExists(aiCfg.APIKeyEnvVar); !exists {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, styles.warning.Render("⚠ "+warning))
	}

	// Merge with existing config (or create new one)
	var finalCfg AtlasConfig
	if existingCfg != nil {
		finalCfg = *existingCfg
	}
	finalCfg.AI = AIConfig{
		Model:        aiCfg.Model,
		APIKeyEnvVar: aiCfg.APIKeyEnvVar,
		Timeout:      aiCfg.Timeout,
		MaxTurns:     aiCfg.MaxTurns,
	}

	// Save configuration using shared function
	if err = saveAtlasConfig(finalCfg, "Updated by atlas config ai"); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Display success
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.success.Render("✓ AI configuration updated successfully!"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("Configuration saved to: "+configPath))

	return nil
}

// configAIStyles contains styling for the config ai command output.
type configAIStyles struct {
	header  lipgloss.Style
	success lipgloss.Style
	warning lipgloss.Style
	dim     lipgloss.Style
	key     lipgloss.Style
	value   lipgloss.Style
}

// newConfigAIStyles creates styles for config ai command output.
func newConfigAIStyles() *configAIStyles {
	return &configAIStyles{
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

// displayCurrentAIConfig shows the current AI configuration.
func displayCurrentAIConfig(w io.Writer, cfg *AtlasConfig, styles *configAIStyles) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.header.Render("Current AI Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 30)))

	_, _ = fmt.Fprintf(w, "%s: %s\n",
		styles.key.Render("Model"),
		styles.value.Render(cfg.AI.Model))
	_, _ = fmt.Fprintf(w, "%s: %s\n",
		styles.key.Render("API Key Env Var"),
		styles.value.Render(cfg.AI.APIKeyEnvVar))
	_, _ = fmt.Fprintf(w, "%s: %s\n",
		styles.key.Render("Timeout"),
		styles.value.Render(cfg.AI.Timeout))
	_, _ = fmt.Fprintf(w, "%s: %d\n",
		styles.key.Render("Max Turns"),
		cfg.AI.MaxTurns)

	// Check if API key is set
	if exists, _ := CheckAPIKeyExists(cfg.AI.APIKeyEnvVar); exists {
		_, _ = fmt.Fprintf(w, "%s: %s\n",
			styles.key.Render("API Key Status"),
			styles.success.Render("✓ Set"))
	} else {
		_, _ = fmt.Fprintf(w, "%s: %s\n",
			styles.key.Render("API Key Status"),
			styles.warning.Render("⚠ Not set"))
	}
}

// loadExistingConfig loads the existing ATLAS configuration if present.
// Returns the config, the config file path, and any error.
func loadExistingConfig() (*AtlasConfig, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(home, constants.AtlasHome, constants.GlobalConfigName)

	data, err := os.ReadFile(configPath) //nolint:gosec // Config file path from home dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, configPath, atlaserrors.ErrConfigNotFound
		}
		return nil, configPath, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg AtlasConfig
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, configPath, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, configPath, nil
}
