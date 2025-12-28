// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
)

// InitFlags holds flags specific to the init command.
type InitFlags struct {
	// NoInteractive skips all prompts and uses default values.
	NoInteractive bool
}

// AtlasConfig represents the user's ATLAS configuration.
// This is the structure that gets written to ~/.atlas/config.yaml.
type AtlasConfig struct {
	AI            AIConfig           `yaml:"ai"`
	Validation    ValidationConfig   `yaml:"validation"`
	Notifications NotificationConfig `yaml:"notifications"`
}

// AIConfig holds AI provider configuration.
// YAML field names match internal/config/config.go AIConfig struct.
type AIConfig struct {
	// Model is the default Claude model to use (sonnet|opus|haiku).
	Model string `yaml:"model"`
	// APIKeyEnvVar is the name of the environment variable containing the API key.
	APIKeyEnvVar string `yaml:"api_key_env_var"`
	// Timeout is the default timeout for AI operations.
	Timeout string `yaml:"timeout"`
	// MaxTurns is the maximum number of turns per AI step.
	MaxTurns int `yaml:"max_turns"`
}

// ValidationConfig holds validation command configuration.
type ValidationConfig struct {
	Commands          ValidationCommands                `yaml:"commands"`
	TemplateOverrides map[string]TemplateOverrideConfig `yaml:"template_overrides,omitempty"`
}

// ValidationCommands holds the validation commands by category.
type ValidationCommands struct {
	Format      []string `yaml:"format"`
	Lint        []string `yaml:"lint"`
	Test        []string `yaml:"test"`
	PreCommit   []string `yaml:"pre_commit"`
	CustomPrePR []string `yaml:"custom_pre_pr,omitempty"`
}

// TemplateOverrideConfig holds per-template validation overrides.
type TemplateOverrideConfig struct {
	// SkipTest indicates whether to skip tests for this template type.
	SkipTest bool `yaml:"skip_test"`
	// SkipLint indicates whether to skip linting for this template type.
	SkipLint bool `yaml:"skip_lint,omitempty"`
}

// NotificationConfig holds notification preferences.
type NotificationConfig struct {
	// BellEnabled enables terminal bell notifications.
	BellEnabled bool `yaml:"bell_enabled"`
	// Events is the list of events to notify on.
	Events []string `yaml:"events"`
}

// initStyles contains styling for the init command output.
// Using a struct avoids global variables while keeping styles reusable.
type initStyles struct {
	header    lipgloss.Style
	installed lipgloss.Style
	missing   lipgloss.Style
	outdated  lipgloss.Style
	success   lipgloss.Style
	err       lipgloss.Style
	info      lipgloss.Style
	dim       lipgloss.Style
}

// newInitStyles creates the styles for init command output.
func newInitStyles() *initStyles {
	return &initStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D7FF")).
			MarginBottom(1),
		installed: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")),
		missing:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F")),
		outdated:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")),
		success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		err: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")).
			Bold(true),
		info: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
	}
}

// Default configuration values for non-interactive mode.
const (
	defaultModel     = "sonnet"
	defaultTimeout   = "30m"
	defaultMaxTurns  = 10
	defaultBell      = true
	defaultAPIKeyEnv = "ANTHROPIC_API_KEY" //nolint:gosec // Not a credential, just env var name
)

// ErrMissingRequiredTools is returned when required tools are missing or outdated.
var ErrMissingRequiredTools = fmt.Errorf("required tools are missing or outdated")

// ToolDetector is an interface for detecting tools.
// This allows for mocking in tests.
type ToolDetector interface {
	Detect(ctx context.Context) (*config.ToolDetectionResult, error)
}

// defaultToolDetector wraps config.NewToolDetector for production use.
type defaultToolDetector struct{}

// Detect delegates to config.NewToolDetector for tool detection.
func (d *defaultToolDetector) Detect(ctx context.Context) (*config.ToolDetectionResult, error) {
	return config.NewToolDetector().Detect(ctx)
}

// Notification event types.
const (
	eventAwaitingApproval = "awaiting_approval"
	eventValidationFailed = "validation_failed"
	eventCIFailed         = "ci_failed"
	eventGitHubFailed     = "github_failed"
)

// newInitCmd creates the init command for setting up ATLAS.
func newInitCmd(flags *InitFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize ATLAS configuration",
		Long: `Initialize ATLAS with a guided setup wizard.

The init command walks you through configuring ATLAS for your environment,
including:
  - Tool detection and verification
  - AI provider settings (model, API key, timeouts)
  - Validation commands (format, lint, test)
  - Notification preferences

Use --no-interactive for automated setups with sensible defaults.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := runInit(cmd.Context(), cmd.OutOrStdout(), flags)
			if errors.Is(err, ErrMissingRequiredTools) {
				// Exit with error code but don't print error again (already displayed)
				os.Exit(ExitError)
			}
			return err
		},
		SilenceUsage: true,
	}

	// Add init-specific flags
	cmd.Flags().BoolVar(&flags.NoInteractive, "no-interactive", false, "skip all prompts and use default values")

	return cmd
}

// AddInitCommand adds the init command to the root command.
func AddInitCommand(rootCmd *cobra.Command) {
	flags := &InitFlags{}
	rootCmd.AddCommand(newInitCmd(flags))
}

// runInit executes the init wizard using the default tool detector.
func runInit(ctx context.Context, w io.Writer, flags *InitFlags) error {
	return runInitWithDetector(ctx, w, flags, &defaultToolDetector{})
}

// runInitWithDetector executes the init wizard with a custom tool detector.
// This allows for mocking in tests.
func runInitWithDetector(ctx context.Context, w io.Writer, flags *InitFlags, detector ToolDetector) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	styles := newInitStyles()

	// Display ATLAS header
	displayHeader(w, styles)

	// Step 1: Run tool detection
	_, _ = fmt.Fprintln(w, styles.info.Render("Detecting tools..."))
	_, _ = fmt.Fprintln(w)

	result, err := detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect tools: %w", err)
	}

	// Display tool status table
	displayToolTable(w, result, styles)

	// Check for missing required tools
	if result.HasMissingRequired {
		missing := result.MissingRequiredTools()
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, styles.err.Render("Required tools are missing or outdated:"))
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprint(w, config.FormatMissingToolsError(missing))
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, styles.err.Render("Please install the required tools and run 'atlas init' again."))
		return ErrMissingRequiredTools
	}

	// Step 2: Handle managed tools
	managedNeedAction := getManagedToolsNeedingAction(result)
	installManaged := false

	if len(managedNeedAction) > 0 && !flags.NoInteractive {
		var promptErr error
		installManaged, promptErr = promptInstallManagedTools(w, managedNeedAction, styles)
		if promptErr != nil {
			return fmt.Errorf("failed to prompt for managed tools: %w", promptErr)
		}
	}

	if installManaged {
		installManagedTools(ctx, w, managedNeedAction, styles)
	}

	// Build configuration
	var cfg AtlasConfig

	if flags.NoInteractive {
		// Use defaults for non-interactive mode
		cfg = buildDefaultConfig(result)
	} else {
		// Interactive wizard
		var wizardErr error
		cfg, wizardErr = runInteractiveWizard(ctx, w, result, styles)
		if wizardErr != nil {
			return wizardErr
		}
	}

	// Save configuration
	if err = saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Display success message
	displaySuccessMessage(w, flags.NoInteractive, styles)

	return nil
}

// displayHeader shows the ATLAS banner.
func displayHeader(w io.Writer, styles *initStyles) {
	header := `
    ╔═══════════════════════════════════════╗
    ║             A T L A S                 ║
    ║     Autonomous Task & Lifecycle       ║
    ║      Automation System                ║
    ╚═══════════════════════════════════════╝`

	_, _ = fmt.Fprintln(w, styles.header.Render(header))
	_, _ = fmt.Fprintln(w)
}

// displayToolTable displays a formatted table of tool status.
func displayToolTable(w io.Writer, result *config.ToolDetectionResult, styles *initStyles) {
	// Table header
	_, _ = fmt.Fprintln(w, styles.dim.Render("TOOL            REQUIRED   VERSION        STATUS"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 55)))

	// Sort tools: required first, then managed
	tools := make([]config.Tool, len(result.Tools))
	copy(tools, result.Tools)

	// Simple sort: required tools first
	sortedTools := make([]config.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Required {
			sortedTools = append(sortedTools, t)
		}
	}
	for _, t := range tools {
		if !t.Required {
			sortedTools = append(sortedTools, t)
		}
	}

	for _, tool := range sortedTools {
		requiredStr := "managed"
		if tool.Required {
			requiredStr = "yes"
		}

		version := tool.CurrentVersion
		if version == "" {
			version = "-"
		}
		if len(version) > 12 {
			version = version[:12]
		}

		statusStr := formatToolStatus(tool, styles)

		// Pad fields for alignment
		name := fmt.Sprintf("%-15s", tool.Name)
		req := fmt.Sprintf("%-10s", requiredStr)
		ver := fmt.Sprintf("%-14s", version)

		_, _ = fmt.Fprintf(w, "%s %s %s %s\n", name, req, ver, statusStr)
	}
}

// formatToolStatus returns a styled status string for a tool.
func formatToolStatus(tool config.Tool, styles *initStyles) string {
	switch tool.Status {
	case config.ToolStatusInstalled:
		return styles.installed.Render("✓ installed")
	case config.ToolStatusMissing:
		return styles.missing.Render("✗ missing")
	case config.ToolStatusOutdated:
		return styles.outdated.Render("⚠ outdated")
	default:
		return styles.dim.Render("? unknown")
	}
}

// getManagedToolsNeedingAction returns managed tools that are missing or outdated.
func getManagedToolsNeedingAction(result *config.ToolDetectionResult) []config.Tool {
	var needAction []config.Tool
	for _, tool := range result.Tools {
		if tool.Managed && (tool.Status == config.ToolStatusMissing || tool.Status == config.ToolStatusOutdated) {
			needAction = append(needAction, tool)
		}
	}
	return needAction
}

// promptInstallManagedTools prompts the user to install managed tools.
func promptInstallManagedTools(w io.Writer, tools []config.Tool, styles *initStyles) (bool, error) {
	_, _ = fmt.Fprintln(w)

	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}

	_, _ = fmt.Fprintln(w, styles.info.Render("Optional tools available for installation: "+strings.Join(names, ", ")))

	var install bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Install/upgrade ATLAS-managed tools?").
				Affirmative("Yes").
				Negative("No").
				Value(&install),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return false, err
	}

	return install, nil
}

// installManagedTools installs the specified managed tools.
func installManagedTools(ctx context.Context, w io.Writer, tools []config.Tool, styles *initStyles) {
	for _, tool := range tools {
		_, _ = fmt.Fprintln(w, styles.info.Render("Installing "+tool.Name+"..."))

		var cmd *exec.Cmd
		switch tool.Name {
		case constants.ToolMageX:
			cmd = exec.CommandContext(ctx, "go", "install", constants.InstallPathMageX) //nolint:gosec // Install path is from constants, not user input
		case constants.ToolGoPreCommit:
			cmd = exec.CommandContext(ctx, "go", "install", constants.InstallPathGoPreCommit) //nolint:gosec // Install path is from constants, not user input
		case constants.ToolSpeckit:
			// Speckit may have a different install method
			_, _ = fmt.Fprintln(w, styles.dim.Render("  Skipping speckit (manual installation required)"))
			continue
		default:
			continue
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			_, _ = fmt.Fprintln(w, styles.err.Render("  Failed to install "+tool.Name+": "+err.Error()))
			if len(output) > 0 {
				_, _ = fmt.Fprintln(w, styles.dim.Render(string(output)))
			}
		} else {
			_, _ = fmt.Fprintln(w, styles.success.Render("  ✓ Installed "+tool.Name))
		}
	}
}

// runInteractiveWizard runs the interactive configuration wizard.
func runInteractiveWizard(ctx context.Context, w io.Writer, toolResult *config.ToolDetectionResult, styles *initStyles) (AtlasConfig, error) {
	// Check cancellation
	select {
	case <-ctx.Done():
		return AtlasConfig{}, ctx.Err()
	default:
	}

	var cfg AtlasConfig

	// AI Provider Configuration using reusable functions from ai_config.go
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.info.Render("AI Provider Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 30)))

	aiCfg := &AIProviderConfig{}
	if err := CollectAIConfigInteractive(ctx, aiCfg); err != nil {
		return AtlasConfig{}, err
	}

	// Check if API key environment variable is set and warn if not
	if exists, warning := CheckAPIKeyExists(aiCfg.APIKeyEnvVar); !exists {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, styles.outdated.Render("⚠ "+warning))
		_, _ = fmt.Fprintln(w, styles.dim.Render("  Make sure to set it before running ATLAS tasks"))
	}

	cfg.AI = AIConfig{
		Model:        aiCfg.Model,
		APIKeyEnvVar: aiCfg.APIKeyEnvVar,
		Timeout:      aiCfg.Timeout,
		MaxTurns:     aiCfg.MaxTurns,
	}

	// Validation Commands Configuration using reusable functions from validation_config.go
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.info.Render("Validation Commands Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 35)))

	valCfg := &ValidationProviderConfig{}
	if err := CollectValidationConfigInteractive(ctx, valCfg, toolResult); err != nil {
		return AtlasConfig{}, fmt.Errorf("validation configuration failed: %w", err)
	}

	// Validate commands and show warnings (AC5: command validation)
	warnings := ValidateAllConfigCommands(valCfg)
	if len(warnings) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, styles.outdated.Render("⚠ Command validation warnings:"))
		for category, categoryWarnings := range warnings {
			for _, warning := range categoryWarnings {
				_, _ = fmt.Fprintf(w, "  %s: %s\n", styles.info.Render(category), styles.dim.Render(warning))
			}
		}
		_, _ = fmt.Fprintln(w, styles.dim.Render("  (These commands may fail when run. You can continue anyway.)"))
	}

	cfg.Validation = valCfg.ToValidationConfig()

	// Notification Preferences
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.info.Render("Notification Preferences"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 25)))

	bellEnabled := defaultBell
	events := []string{eventAwaitingApproval, eventValidationFailed, eventCIFailed, eventGitHubFailed}

	notifyForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Terminal Bell").
				Description("Play a sound when ATLAS needs your attention").
				Affirmative("Yes").
				Negative("No").
				Value(&bellEnabled),
			huh.NewMultiSelect[string]().
				Title("Notification Events").
				Description("Select events that should trigger notifications").
				Options(
					huh.NewOption("Task awaiting approval", eventAwaitingApproval).Selected(true),
					huh.NewOption("Validation failed", eventValidationFailed).Selected(true),
					huh.NewOption("CI failed", eventCIFailed).Selected(true),
					huh.NewOption("GitHub operation failed", eventGitHubFailed).Selected(true),
				).
				Value(&events),
		),
	).WithTheme(huh.ThemeCharm())

	if err := notifyForm.Run(); err != nil {
		return AtlasConfig{}, fmt.Errorf("notification configuration failed: %w", err)
	}

	cfg.Notifications = NotificationConfig{
		BellEnabled: bellEnabled,
		Events:      events,
	}

	return cfg, nil
}

// buildDefaultConfig creates a configuration with sensible defaults.
func buildDefaultConfig(toolResult *config.ToolDetectionResult) AtlasConfig {
	// Use reusable SuggestValidationDefaults from validation_config.go
	defaultCommands := SuggestValidationDefaults(toolResult)

	return AtlasConfig{
		AI: AIConfig{
			Model:        defaultModel,
			APIKeyEnvVar: defaultAPIKeyEnv,
			Timeout:      defaultTimeout,
			MaxTurns:     defaultMaxTurns,
		},
		Validation: ValidationConfig{
			Commands: defaultCommands,
		},
		Notifications: NotificationConfig{
			BellEnabled: defaultBell,
			Events: []string{
				eventAwaitingApproval,
				eventValidationFailed,
				eventCIFailed,
				eventGitHubFailed,
			},
		},
	}
}

// suggestValidationCommands suggests validation commands based on detected tools.
// Deprecated: Use SuggestValidationDefaults instead for new code.
func suggestValidationCommands(result *config.ToolDetectionResult) ValidationCommands {
	return SuggestValidationDefaults(result)
}

// parseMultilineInput splits multiline input into a slice of strings.
// Deprecated: Use ParseMultilineInput instead for new code.
func parseMultilineInput(input string) []string {
	return ParseMultilineInput(input)
}

// saveConfig writes the configuration to ~/.atlas/config.yaml.
// If a config file already exists, it creates a backup before overwriting.
func saveConfig(cfg AtlasConfig) error {
	return saveAtlasConfig(cfg, "Generated by atlas init")
}

// saveAtlasConfig writes the configuration to ~/.atlas/config.yaml with a custom header source.
// This is the shared implementation used by saveConfig and saveConfigAI.
// If a config file already exists, it creates a backup before overwriting.
func saveAtlasConfig(cfg AtlasConfig, headerSource string) error {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create ~/.atlas directory with restrictive permissions
	atlasDir := filepath.Join(home, constants.AtlasHome)
	if err = os.MkdirAll(atlasDir, 0o700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)

	// Check if config file already exists and create backup
	if _, statErr := os.Stat(configPath); statErr == nil {
		backupPath := configPath + ".backup"
		if copyErr := copyFile(configPath, backupPath); copyErr != nil {
			// Log warning but continue - backup is best effort
			logger := GetLogger()
			logger.Warn().
				Err(copyErr).
				Str("backup_path", backupPath).
				Msg("failed to create config backup")
		}
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := fmt.Sprintf("# ATLAS Configuration\n# %s on %s\n\n",
		headerSource, time.Now().Format("2006-01-02 15:04:05"))
	content := header + string(data)

	// Write config file with restrictive permissions
	if err = os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) //nolint:gosec // Source is config file
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// displaySuccessMessage shows the success message after configuration.
func displaySuccessMessage(w io.Writer, nonInteractive bool, styles *initStyles) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.success.Render("✓ ATLAS configuration saved successfully!"))
	_, _ = fmt.Fprintln(w)

	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, constants.AtlasHome, constants.GlobalConfigName)
	_, _ = fmt.Fprintln(w, styles.dim.Render("Configuration saved to: "+configPath))
	_, _ = fmt.Fprintln(w)

	_, _ = fmt.Fprintln(w, styles.info.Render("Suggested next commands:"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  atlas status    - View current project status"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  atlas start     - Start a new task"))
	_, _ = fmt.Fprintln(w)

	if nonInteractive {
		_, _ = fmt.Fprintln(w, styles.dim.Render("Note: Non-interactive mode used default values."))
		_, _ = fmt.Fprintln(w, styles.dim.Render("Edit the config file or run 'atlas init' interactively to customize."))
	}
}
