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
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// InitFlags holds flags specific to the init command.
type InitFlags struct {
	// NoInteractive skips all prompts and uses default values.
	NoInteractive bool
	// Global forces configuration to be saved to global config only.
	Global bool
	// Project forces configuration to be saved to project config only.
	Project bool
}

// AtlasConfig represents the user's ATLAS configuration.
// This is the structure that gets written to ~/.atlas/config.yaml.
type AtlasConfig struct {
	AI            AIConfig           `yaml:"ai"`
	Validation    ValidationConfig   `yaml:"validation"`
	Notifications NotificationConfig `yaml:"notifications"`
	Hooks         HookConfig         `yaml:"hooks"`
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
// YAML field names match internal/config/config.go NotificationsConfig struct.
type NotificationConfig struct {
	// BellEnabled enables terminal bell notifications.
	// Uses "bell" YAML tag to match internal/config/config.go for config.Load() compatibility.
	BellEnabled bool `yaml:"bell"`
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

// Separator widths for consistent UI formatting.
const (
	separatorWidthTable  = 55 // Tool table header
	separatorWidthWide   = 35 // Section headers (validation)
	separatorWidthMedium = 30 // Section headers (AI)
	separatorWidthNarrow = 25 // Section headers (notifications)
)

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

// Notification event types are defined in notification_config.go as exported constants:
// NotifyEventAwaitingApproval, NotifyEventValidationFailed, NotifyEventCIFailed, NotifyEventGitHubFailed

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

Configuration can be saved to:
  - Global: ~/.atlas/config.yaml (applies to all projects)
  - Project: .atlas/config.yaml (project-specific overrides)

In a git repository, you'll be asked whether to create project-specific config.
Project config is recommended for shared projects with team settings.

Use --no-interactive for automated setups with sensible defaults.
Use --global to save only to global config (skip project config prompt).
Use --project to save only to project config (requires being in a project directory).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := runInit(cmd.Context(), cmd.OutOrStdout(), flags)
			if errors.Is(err, atlaserrors.ErrMissingRequiredTools) {
				// Exit with error code but don't print error again (already displayed)
				os.Exit(ExitError)
			}
			return err
		},
		SilenceUsage: true,
	}

	// Add init-specific flags
	cmd.Flags().BoolVar(&flags.NoInteractive, "no-interactive", false, "skip all prompts and use default values")
	cmd.Flags().BoolVar(&flags.Global, "global", false, "save to global config only (~/.atlas/config.yaml)")
	cmd.Flags().BoolVar(&flags.Project, "project", false, "save to project config only (.atlas/config.yaml)")
	cmd.MarkFlagsMutuallyExclusive("global", "project")

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

	// Validate --project flag requires being in a git repo
	if flags.Project && !isInGitRepo(ctx) {
		_, _ = fmt.Fprintln(w, styles.err.Render("Error: --project flag requires being in a git repository."))
		_, _ = fmt.Fprintln(w, styles.dim.Render("  Project config is stored at .atlas/config.yaml relative to the git root."))
		_, _ = fmt.Fprintln(w, styles.dim.Render("  Use --global to save to ~/.atlas/config.yaml instead."))
		return atlaserrors.ErrNotInProjectDir
	}

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
		return atlaserrors.ErrMissingRequiredTools
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

	// Determine and execute config save strategy
	saveResult, err := determineAndSaveConfig(ctx, w, flags, cfg, styles)
	if err != nil {
		return err
	}

	// Display success message with config paths
	displaySuccessMessageWithPaths(w, flags.NoInteractive, saveResult.projectConfigCreated, saveResult.configPaths, styles)

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
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", separatorWidthTable)))

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
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", separatorWidthMedium)))

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
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", separatorWidthWide)))

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

	// Notification Preferences using reusable functions from notification_config.go
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.info.Render("Notification Preferences"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", separatorWidthNarrow)))

	notifyCfg := &NotificationProviderConfig{
		BellEnabled: defaultBell,
		Events:      AllNotificationEvents(),
	}
	if err := CollectNotificationConfigInteractive(ctx, notifyCfg); err != nil {
		return AtlasConfig{}, fmt.Errorf("notification configuration failed: %w", err)
	}

	cfg.Notifications = notifyCfg.ToNotificationConfig()

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
		// Use reusable DefaultNotificationConfig from notification_config.go
		Notifications: DefaultNotificationConfig(),
		Hooks: HookConfig{
			Crypto: CryptoConfig{
				Provider: "native",
			},
		},
	}
}

// HookConfig holds hook system configuration.
type HookConfig struct {
	Crypto CryptoConfig `yaml:"crypto"`
}

// CryptoConfig holds cryptographic configuration.
type CryptoConfig struct {
	Provider string `yaml:"provider"`
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
			logger := Logger()
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
		headerSource, time.Now().Format(constants.TimeFormatISO))
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

// isInGitRepo checks if the current directory is inside a git repository.
// Uses git rev-parse for accurate detection even in worktrees.
func isInGitRepo(ctx context.Context) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	_, err = git.DetectRepo(ctx, cwd)
	return err == nil
}

// findGitRoot returns the root directory of the current working tree.
// For worktrees, this returns the worktree root (not main repo root).
// Returns empty string if not in a git repository.
func findGitRoot(ctx context.Context) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	info, err := git.DetectRepo(ctx, cwd)
	if err != nil {
		return ""
	}

	return info.WorktreePath
}

// configSaveResult contains the result of saving configuration.
type configSaveResult struct {
	configPaths          []string
	projectConfigCreated bool
}

// determineAndSaveConfig determines where to save config and performs the save.
// This function handles the logic for deciding between project and global config.
func determineAndSaveConfig(ctx context.Context, w io.Writer, flags *InitFlags, cfg AtlasConfig, styles *initStyles) (*configSaveResult, error) {
	result := &configSaveResult{
		configPaths: []string{},
	}

	saveToProject := flags.Project
	saveToGlobal := flags.Global

	// If neither flag is set, determine based on context
	if !saveToProject && !saveToGlobal {
		saveToProject, saveToGlobal = determineConfigLocations(ctx, w, flags, styles)
	}

	// Save to project config if requested
	if saveToProject {
		if err := saveProjectConfig(ctx, cfg); err != nil {
			return nil, fmt.Errorf("failed to save project configuration: %w", err)
		}
		gitRoot := findGitRoot(ctx)
		projectPath := filepath.Join(gitRoot, constants.AtlasHome, constants.GlobalConfigName)
		result.configPaths = append(result.configPaths, projectPath)
		result.projectConfigCreated = true
	}

	// Save to global config if requested
	if saveToGlobal {
		if err := saveGlobalConfig(cfg); err != nil {
			return nil, fmt.Errorf("failed to save global configuration: %w", err)
		}
		home, _ := os.UserHomeDir()
		globalPath := filepath.Join(home, constants.AtlasHome, constants.GlobalConfigName)
		result.configPaths = append(result.configPaths, globalPath)
	}

	return result, nil
}

// determineConfigLocations decides whether to save to project and/or global config.
// Returns (saveToProject, saveToGlobal).
func determineConfigLocations(ctx context.Context, w io.Writer, flags *InitFlags, styles *initStyles) (bool, bool) {
	inGitRepo := isInGitRepo(ctx)

	// Non-interactive or not in git repo: save to global only
	if flags.NoInteractive || !inGitRepo {
		return false, true
	}

	// Interactive and in git repo: ask user
	saveToProject, promptErr := promptProjectConfigCreation(w, styles)
	if promptErr != nil {
		// On error, fall back to global only
		return false, true
	}

	// If not project, save to global
	if !saveToProject {
		return false, true
	}

	return true, false
}

// promptProjectConfigCreation prompts the user to create project-specific config.
func promptProjectConfigCreation(w io.Writer, styles *initStyles) (bool, error) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.info.Render("Git repository detected"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  You can create a project-specific configuration that overrides global settings."))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  This is recommended for shared projects with team-specific settings."))
	_, _ = fmt.Fprintln(w)

	var createProjectConfig bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Create project-specific configuration?").
				Description("Creates .atlas/config.yaml in the project root").
				Affirmative("Yes (recommended for teams)").
				Negative("No (use global config only)").
				Value(&createProjectConfig),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return false, err
	}

	return createProjectConfig, nil
}

// saveProjectConfig writes the configuration to .atlas/config.yaml in the git root.
// If a config file already exists, it creates a backup before overwriting.
func saveProjectConfig(ctx context.Context, cfg AtlasConfig) error {
	gitRoot := findGitRoot(ctx)
	if gitRoot == "" {
		return atlaserrors.ErrNotInGitRepo
	}

	// Create .atlas directory with restrictive permissions (0700)
	// This is a config directory that may contain sensitive data
	atlasDir := filepath.Join(gitRoot, constants.AtlasHome)
	if err := os.MkdirAll(atlasDir, 0o700); err != nil {
		return fmt.Errorf("failed to create project config directory: %w", err)
	}

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)

	// Check if config file already exists and create backup
	if _, statErr := os.Stat(configPath); statErr == nil {
		backupPath := configPath + ".backup"
		if copyErr := copyFile(configPath, backupPath); copyErr != nil {
			// Log warning but continue - backup is best effort
			logger := Logger()
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
	header := fmt.Sprintf("# ATLAS Project Configuration\n# Generated by atlas init on %s\n# This file overrides ~/.atlas/config.yaml for this project.\n# Consider adding .atlas/config.yaml to .gitignore if it contains sensitive data.\n\n",
		time.Now().Format(constants.TimeFormatISO))
	content := header + string(data)

	// Write config file with restrictive permissions (0600)
	if err = os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write project config file: %w", err)
	}

	return nil
}

// saveGlobalConfig writes the configuration to ~/.atlas/config.yaml.
// If a config file already exists, it creates a backup before overwriting.
func saveGlobalConfig(cfg AtlasConfig) error {
	return saveAtlasConfig(cfg, "Generated by atlas init")
}

// displaySuccessMessageWithPaths shows the success message after configuration with specific paths.
func displaySuccessMessageWithPaths(w io.Writer, nonInteractive, projectConfigCreated bool, configPaths []string, styles *initStyles) {
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.success.Render("✓ ATLAS configuration saved successfully!"))
	_, _ = fmt.Fprintln(w)

	_, _ = fmt.Fprintln(w, styles.info.Render("Configuration saved to:"))
	for _, path := range configPaths {
		_, _ = fmt.Fprintln(w, styles.dim.Render("  "+path))
	}
	_, _ = fmt.Fprintln(w)

	// Show gitignore suggestion if project config was created
	if projectConfigCreated {
		_, _ = fmt.Fprintln(w, styles.outdated.Render("Tip: Consider adding to .gitignore if config contains sensitive data:"))
		_, _ = fmt.Fprintln(w, styles.dim.Render("  .atlas/config.yaml"))
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w, styles.info.Render("Suggested next commands:"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  atlas status       - View current project status"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  atlas start        - Start a new task"))
	_, _ = fmt.Fprintln(w, styles.dim.Render("  atlas config show  - View effective configuration"))
	_, _ = fmt.Fprintln(w)

	if nonInteractive {
		_, _ = fmt.Fprintln(w, styles.dim.Render("Note: Non-interactive mode used default values."))
		_, _ = fmt.Fprintln(w, styles.dim.Render("Edit the config file or run 'atlas init' interactively to customize."))
	}
}
