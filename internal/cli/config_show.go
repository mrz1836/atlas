// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
)

// ConfigShowFlags holds flags specific to the config show command.
type ConfigShowFlags struct {
	// OutputFormat specifies the output format (yaml or json).
	OutputFormat string
}

// newConfigShowCmd creates the 'config show' subcommand for displaying configuration.
func newConfigShowCmd(flags *ConfigShowFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display effective configuration",
		Long: `Display the effective ATLAS configuration with source annotations.

Shows the current configuration values and indicates where each value comes from:
  - default: Built-in default value
  - global: From ~/.atlas/config.yaml
  - project: From .atlas/config.yaml
  - env: From ATLAS_* environment variable

Sensitive values (API keys, tokens) are masked in the output.

Examples:
  atlas config show           # Display config in YAML format with sources
  atlas config show --output json   # Display config in JSON format`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigShow(cmd.Context(), cmd.OutOrStdout(), flags)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVarP(&flags.OutputFormat, "output", "o", "yaml", "output format (yaml or json)")

	return cmd
}

// AddConfigShowCommand adds the show subcommand to the config command.
func AddConfigShowCommand(configCmd *cobra.Command) {
	flags := &ConfigShowFlags{}
	configCmd.AddCommand(newConfigShowCmd(flags))
}

// ConfigSource represents where a configuration value came from.
type ConfigSource string

const (
	// SourceDefault indicates the value is a built-in default.
	SourceDefault ConfigSource = "default"
	// SourceGlobal indicates the value came from global config.
	SourceGlobal ConfigSource = "global"
	// SourceProject indicates the value came from project config.
	SourceProject ConfigSource = "project"
	// SourceEnv indicates the value came from an environment variable.
	SourceEnv ConfigSource = "env"
)

// ConfigValueWithSource represents a configuration value with its source.
type ConfigValueWithSource struct {
	Value  any          `json:"value" yaml:"value"`
	Source ConfigSource `json:"source" yaml:"source"`
}

// AnnotatedConfig represents configuration with source annotations.
type AnnotatedConfig struct {
	AI            map[string]ConfigValueWithSource `json:"ai" yaml:"ai"`
	Git           map[string]ConfigValueWithSource `json:"git" yaml:"git"`
	Worktree      map[string]ConfigValueWithSource `json:"worktree" yaml:"worktree"`
	CI            map[string]ConfigValueWithSource `json:"ci" yaml:"ci"`
	Templates     map[string]ConfigValueWithSource `json:"templates" yaml:"templates"`
	Validation    map[string]ConfigValueWithSource `json:"validation" yaml:"validation"`
	Notifications map[string]ConfigValueWithSource `json:"notifications" yaml:"notifications"`
}

// configShowStyles contains styling for the config show command output.
type configShowStyles struct {
	header    lipgloss.Style
	section   lipgloss.Style
	key       lipgloss.Style
	value     lipgloss.Style
	sourceEnv lipgloss.Style
	sourcePrj lipgloss.Style
	sourceGbl lipgloss.Style
	sourceDef lipgloss.Style
	masked    lipgloss.Style
	dim       lipgloss.Style
}

// newConfigShowStyles creates styles for config show command output.
func newConfigShowStyles() *configShowStyles {
	return &configShowStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D7FF")).
			MarginBottom(1),
		section: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")),
		key: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		value: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")),
		sourceEnv: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")), // Red for env (highest precedence)
		sourcePrj: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")), // Yellow for project
		sourceGbl: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")), // Green for global
		sourceDef: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")), // Gray for default
		masked: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
	}
}

// runConfigShow executes the config show command.
func runConfigShow(ctx context.Context, w io.Writer, flags *ConfigShowFlags) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Load the effective configuration
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Build annotated configuration with sources
	annotated := buildAnnotatedConfig(cfg)

	// Output based on format
	switch strings.ToLower(flags.OutputFormat) {
	case "json":
		return outputJSON(w, annotated)
	case "yaml":
		return outputYAML(w, cfg, annotated)
	default:
		return fmt.Errorf("%w: %s (use yaml or json)", errors.ErrUnsupportedOutputFormat, flags.OutputFormat)
	}
}

// buildAnnotatedConfig creates an annotated configuration with source information.
func buildAnnotatedConfig(cfg *config.Config) *AnnotatedConfig {
	// Load individual config sources to determine where each value came from
	globalCfg := loadGlobalConfigOnly()
	projectCfg := loadProjectConfigOnly()

	annotated := &AnnotatedConfig{
		AI:            make(map[string]ConfigValueWithSource),
		Git:           make(map[string]ConfigValueWithSource),
		Worktree:      make(map[string]ConfigValueWithSource),
		CI:            make(map[string]ConfigValueWithSource),
		Templates:     make(map[string]ConfigValueWithSource),
		Validation:    make(map[string]ConfigValueWithSource),
		Notifications: make(map[string]ConfigValueWithSource),
	}

	// AI section
	annotated.AI["agent"] = determineSource("ai.agent", cfg.AI.Agent, globalCfg, projectCfg, "claude")
	annotated.AI["model"] = determineSource("ai.model", cfg.AI.Model, globalCfg, projectCfg, "sonnet")
	// For api_key_env_vars, show the value for the current agent
	currentAgentEnvVar := cfg.AI.GetAPIKeyEnvVar(cfg.AI.Agent)
	annotated.AI["api_key_env_var"] = determineSource("ai.api_key_env_vars", currentAgentEnvVar, globalCfg, projectCfg, "ANTHROPIC_API_KEY")
	annotated.AI["timeout"] = determineSource("ai.timeout", cfg.AI.Timeout.String(), globalCfg, projectCfg, constants.DefaultAITimeout.String())
	annotated.AI["max_turns"] = determineSource("ai.max_turns", cfg.AI.MaxTurns, globalCfg, projectCfg, 10)

	// Git section
	annotated.Git["base_branch"] = determineSource("git.base_branch", cfg.Git.BaseBranch, globalCfg, projectCfg, "main")
	annotated.Git["auto_proceed_git"] = determineSource("git.auto_proceed_git", cfg.Git.AutoProceedGit, globalCfg, projectCfg, true)
	annotated.Git["remote"] = determineSource("git.remote", cfg.Git.Remote, globalCfg, projectCfg, "origin")

	// Worktree section
	annotated.Worktree["base_dir"] = determineSource("worktree.base_dir", cfg.Worktree.BaseDir, globalCfg, projectCfg, "")
	annotated.Worktree["naming_suffix"] = determineSource("worktree.naming_suffix", cfg.Worktree.NamingSuffix, globalCfg, projectCfg, "")

	// CI section
	annotated.CI["timeout"] = determineSource("ci.timeout", cfg.CI.Timeout.String(), globalCfg, projectCfg, constants.DefaultCITimeout.String())
	annotated.CI["poll_interval"] = determineSource("ci.poll_interval", cfg.CI.PollInterval.String(), globalCfg, projectCfg, constants.CIPollInterval.String())

	// Notifications section
	annotated.Notifications["bell"] = determineSource("notifications.bell", cfg.Notifications.Bell, globalCfg, projectCfg, true)
	annotated.Notifications["events"] = determineSource("notifications.events", cfg.Notifications.Events, globalCfg, projectCfg, []string{"awaiting_approval", "validation_failed"})

	return annotated
}

// configValues represents parsed config values for source determination.
type configValues map[string]any

// loadGlobalConfigOnly loads only the global config for source comparison.
func loadGlobalConfigOnly() configValues {
	globalDir, err := config.GlobalConfigDir()
	if err != nil {
		return nil
	}

	globalConfigPath := filepath.Join(globalDir, "config.yaml")
	return loadConfigFile(globalConfigPath)
}

// loadProjectConfigOnly loads only the project config for source comparison.
func loadProjectConfigOnly() configValues {
	projectConfigPath := config.ProjectConfigPath()
	return loadConfigFile(projectConfigPath)
}

// loadConfigFile loads a config file into a map for source determination.
func loadConfigFile(path string) configValues {
	data, err := os.ReadFile(path) //nolint:gosec // Config file path
	if err != nil {
		return nil
	}

	// Parse YAML into a map
	result := make(configValues)
	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if this is a section header (no colon after first word or value is empty)
		if strings.HasSuffix(line, ":") && !strings.Contains(line[:len(line)-1], " ") {
			currentSection = strings.TrimSuffix(line, ":")
			continue
		}

		// Parse key: value
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if currentSection != "" {
				result[currentSection+"."+key] = value
			} else {
				result[key] = value
			}
		}
	}

	return result
}

// determineSource determines where a configuration value came from.
func determineSource(key string, value any, globalCfg, projectCfg configValues, _ any) ConfigValueWithSource {
	// Check env var first (highest precedence after CLI)
	envKey := "ATLAS_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if envVal := os.Getenv(envKey); envVal != "" {
		return ConfigValueWithSource{Value: value, Source: SourceEnv}
	}

	// Check project config
	if projectCfg != nil {
		if _, exists := projectCfg[key]; exists {
			return ConfigValueWithSource{Value: value, Source: SourceProject}
		}
	}

	// Check global config
	if globalCfg != nil {
		if _, exists := globalCfg[key]; exists {
			return ConfigValueWithSource{Value: value, Source: SourceGlobal}
		}
	}

	// Must be default
	return ConfigValueWithSource{Value: value, Source: SourceDefault}
}

// outputJSON outputs the configuration in JSON format.
func outputJSON(w io.Writer, annotated *AnnotatedConfig) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(annotated)
}

// outputYAML outputs the configuration in YAML format with source comments.
func outputYAML(w io.Writer, cfg *config.Config, annotated *AnnotatedConfig) error {
	styles := newConfigShowStyles()

	_, _ = fmt.Fprintln(w, styles.header.Render("Effective ATLAS Configuration"))
	_, _ = fmt.Fprintln(w, styles.dim.Render(strings.Repeat("─", 50)))
	_, _ = fmt.Fprintln(w)

	// Display legend
	_, _ = fmt.Fprintln(w, styles.dim.Render("Sources: ")+
		styles.sourceEnv.Render("env")+" > "+
		styles.sourcePrj.Render("project")+" > "+
		styles.sourceGbl.Render("global")+" > "+
		styles.sourceDef.Render("default"))
	_, _ = fmt.Fprintln(w)

	// AI section
	_, _ = fmt.Fprintln(w, styles.section.Render("ai:"))
	printConfigValue(w, styles, "  agent", annotated.AI["agent"])
	printConfigValue(w, styles, "  model", annotated.AI["model"])
	printConfigValue(w, styles, "  api_key_env_var", annotated.AI["api_key_env_var"])
	printConfigValue(w, styles, "  timeout", annotated.AI["timeout"])
	printConfigValue(w, styles, "  max_turns", annotated.AI["max_turns"])
	_, _ = fmt.Fprintln(w)

	// Git section
	_, _ = fmt.Fprintln(w, styles.section.Render("git:"))
	printConfigValue(w, styles, "  base_branch", annotated.Git["base_branch"])
	printConfigValue(w, styles, "  auto_proceed_git", annotated.Git["auto_proceed_git"])
	printConfigValue(w, styles, "  remote", annotated.Git["remote"])
	_, _ = fmt.Fprintln(w)

	// Worktree section
	_, _ = fmt.Fprintln(w, styles.section.Render("worktree:"))
	printConfigValue(w, styles, "  base_dir", annotated.Worktree["base_dir"])
	printConfigValue(w, styles, "  naming_suffix", annotated.Worktree["naming_suffix"])
	_, _ = fmt.Fprintln(w)

	// CI section
	_, _ = fmt.Fprintln(w, styles.section.Render("ci:"))
	printConfigValue(w, styles, "  timeout", annotated.CI["timeout"])
	printConfigValue(w, styles, "  poll_interval", annotated.CI["poll_interval"])
	_, _ = fmt.Fprintln(w)

	// Validation section
	_, _ = fmt.Fprintln(w, styles.section.Render("validation:"))
	printConfigValue(w, styles, "  timeout", ConfigValueWithSource{Value: cfg.Validation.Timeout.String(), Source: SourceDefault})
	printConfigValue(w, styles, "  parallel_execution", ConfigValueWithSource{Value: cfg.Validation.ParallelExecution, Source: SourceDefault})
	_, _ = fmt.Fprintln(w, styles.key.Render("  commands:"))
	if len(cfg.Validation.Commands.Format) > 0 {
		_, _ = fmt.Fprintf(w, "    %s: %v\n", styles.key.Render("format"), cfg.Validation.Commands.Format)
	}
	if len(cfg.Validation.Commands.Lint) > 0 {
		_, _ = fmt.Fprintf(w, "    %s: %v\n", styles.key.Render("lint"), cfg.Validation.Commands.Lint)
	}
	if len(cfg.Validation.Commands.Test) > 0 {
		_, _ = fmt.Fprintf(w, "    %s: %v\n", styles.key.Render("test"), cfg.Validation.Commands.Test)
	}
	if len(cfg.Validation.Commands.PreCommit) > 0 {
		_, _ = fmt.Fprintf(w, "    %s: %v\n", styles.key.Render("pre_commit"), cfg.Validation.Commands.PreCommit)
	}
	_, _ = fmt.Fprintln(w)

	// Notifications section
	_, _ = fmt.Fprintln(w, styles.section.Render("notifications:"))
	printConfigValue(w, styles, "  bell", annotated.Notifications["bell"])
	printConfigValue(w, styles, "  events", annotated.Notifications["events"])
	_, _ = fmt.Fprintln(w)

	// Config file locations
	_, _ = fmt.Fprintln(w, styles.dim.Render("Configuration files:"))
	if globalPath, err := config.GlobalConfigPath(); err == nil {
		if _, err := os.Stat(globalPath); err == nil {
			_, _ = fmt.Fprintln(w, styles.dim.Render("  Global: ")+styles.sourceGbl.Render(globalPath))
		} else {
			_, _ = fmt.Fprintln(w, styles.dim.Render("  Global: ")+styles.dim.Render(globalPath+" (not found)"))
		}
	}

	projectPath := config.ProjectConfigPath()
	if _, err := os.Stat(projectPath); err == nil {
		absPath, _ := filepath.Abs(projectPath)
		_, _ = fmt.Fprintln(w, styles.dim.Render("  Project: ")+styles.sourcePrj.Render(absPath))
	} else {
		_, _ = fmt.Fprintln(w, styles.dim.Render("  Project: ")+styles.dim.Render(projectPath+" (not found)"))
	}

	return nil
}

// printConfigValue prints a configuration value with its source annotation.
func printConfigValue(w io.Writer, styles *configShowStyles, key string, vs ConfigValueWithSource) {
	valueStr := formatConfigValue(vs.Value)
	valueStr = maskSensitiveValue(key, valueStr, vs.Source, styles)
	sourceStyle := getSourceStyle(vs.Source, styles)

	_, _ = fmt.Fprintf(w, "%s: %s  %s\n",
		styles.key.Render(key),
		styles.value.Render(valueStr),
		sourceStyle.Render("# "+string(vs.Source)))
}

// formatConfigValue converts a configuration value to a displayable string.
func formatConfigValue(value any) string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "(not set)"
		}
		return v
	case []string:
		if len(v) == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%s]", strings.Join(v, ", "))
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		strs := make([]string, len(v))
		for i, item := range v {
			strs[i] = fmt.Sprintf("%v", item)
		}
		return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// maskSensitiveValue masks sensitive values like API keys.
func maskSensitiveValue(key, valueStr string, source ConfigSource, styles *configShowStyles) string {
	lowerKey := strings.ToLower(key)
	isSensitive := strings.Contains(lowerKey, "key") ||
		strings.Contains(lowerKey, "secret") ||
		strings.Contains(lowerKey, "token") ||
		strings.Contains(lowerKey, "password")

	if !isSensitive {
		return valueStr
	}

	// Only mask actual secret values, not env var names
	if source == SourceEnv && valueStr != "(not set)" && valueStr != "" {
		if !strings.HasPrefix(valueStr, "ANTHROPIC") && !strings.HasSuffix(valueStr, "_KEY") {
			return styles.masked.Render("****")
		}
	}

	return valueStr
}

// getSourceStyle returns the appropriate style for a config source.
func getSourceStyle(source ConfigSource, styles *configShowStyles) lipgloss.Style {
	switch source {
	case SourceEnv:
		return styles.sourceEnv
	case SourceProject:
		return styles.sourcePrj
	case SourceGlobal:
		return styles.sourceGbl
	case SourceDefault:
		return styles.sourceDef
	default:
		return styles.sourceDef
	}
}
