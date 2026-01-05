// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
)

// ValidationProviderConfig holds validation configuration values collected from user input.
// This struct is used for collecting configuration before saving.
type ValidationProviderConfig struct {
	// FormatCmds is the multiline string of format commands (one per line).
	FormatCmds string
	// LintCmds is the multiline string of lint commands (one per line).
	LintCmds string
	// TestCmds is the multiline string of test commands (one per line).
	TestCmds string
	// PreCommitCmds is the multiline string of pre-commit commands (one per line).
	PreCommitCmds string
	// CustomPrePR is the multiline string of custom pre-PR hook commands (one per line).
	CustomPrePR string
}

// ValidationConfigDefaults returns the default values for validation configuration.
// Defaults are empty as commands should be suggested based on detected tools.
func ValidationConfigDefaults() ValidationProviderConfig {
	return ValidationProviderConfig{}
}

// SuggestValidationDefaults suggests validation commands based on detected tools.
// This is the exported version of suggestValidationCommands for reuse.
func SuggestValidationDefaults(result *config.ToolDetectionResult) ValidationCommands {
	cmds := ValidationCommands{}

	hasMageX := false
	hasGoPreCommit := false

	if result != nil {
		for _, tool := range result.Tools {
			if tool.Status == config.ToolStatusInstalled {
				switch tool.Name {
				case constants.ToolMageX:
					hasMageX = true
				case constants.ToolGoPreCommit:
					hasGoPreCommit = true
				}
			}
		}
	}

	// Set commands based on available tools
	if hasMageX {
		cmds.Format = []string{"magex format:fix"}
		cmds.Lint = []string{"magex lint"}
		cmds.Test = []string{"magex test"}
	} else {
		// Fallback to basic go commands
		cmds.Format = []string{"gofmt -w ."}
		cmds.Lint = []string{"go vet ./..."}
		cmds.Test = []string{"go test ./..."}
	}

	if hasGoPreCommit {
		cmds.PreCommit = []string{"go-pre-commit run --all-files"}
	}

	return cmds
}

// ParseMultilineInput splits multiline input into a slice of strings.
// Each line is trimmed of whitespace and empty lines are filtered out.
func ParseMultilineInput(input string) []string {
	var result []string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// NewValidationConfigForm creates a Charm Huh form for validation configuration.
// The form collects format, lint, test, pre-commit, and custom pre-PR commands.
func NewValidationConfigForm(cfg *ValidationProviderConfig) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Format Commands").
				Description("Commands to run for code formatting (one per line)").
				Value(&cfg.FormatCmds).
				Placeholder("magex format:fix"),
			huh.NewText().
				Title("Lint Commands").
				Description("Commands to run for linting (one per line)").
				Value(&cfg.LintCmds).
				Placeholder("magex lint"),
			huh.NewText().
				Title("Test Commands").
				Description("Commands to run for testing (one per line)").
				Value(&cfg.TestCmds).
				Placeholder("magex test"),
			huh.NewText().
				Title("Pre-commit Commands").
				Description("Commands to run before commits (one per line)").
				Value(&cfg.PreCommitCmds).
				Placeholder("go-pre-commit run --all-files"),
			huh.NewText().
				Title("Custom Pre-PR Hooks").
				Description("Additional commands to run before PR creation (one per line, optional)").
				Value(&cfg.CustomPrePR).
				Placeholder("custom-validation-script"),
		),
	).WithTheme(huh.ThemeCharm())
}

// CollectValidationConfigInteractive runs the validation configuration form.
// It populates the config with defaults based on detected tools and runs the form.
func CollectValidationConfigInteractive(ctx context.Context, cfg *ValidationProviderConfig, toolResult *config.ToolDetectionResult) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Populate defaults based on detected tools
	PopulateValidationConfigDefaults(cfg, toolResult)

	// Create and run the main form
	form := NewValidationConfigForm(cfg)
	if err := form.Run(); err != nil {
		return err
	}

	return nil
}

// CollectValidationConfigNonInteractive returns a configuration with defaults based on detected tools.
// Used when running in non-interactive mode.
func CollectValidationConfigNonInteractive(toolResult *config.ToolDetectionResult) ValidationProviderConfig {
	cfg := ValidationConfigDefaults()
	PopulateValidationConfigDefaults(&cfg, toolResult)
	return cfg
}

// PopulateValidationConfigDefaults populates the config with suggested defaults based on detected tools.
// The defaults are set as multiline strings for display in the form.
func PopulateValidationConfigDefaults(cfg *ValidationProviderConfig, toolResult *config.ToolDetectionResult) {
	if cfg == nil {
		return
	}

	defaults := SuggestValidationDefaults(toolResult)

	cfg.FormatCmds = strings.Join(defaults.Format, "\n")
	cfg.LintCmds = strings.Join(defaults.Lint, "\n")
	cfg.TestCmds = strings.Join(defaults.Test, "\n")
	cfg.PreCommitCmds = strings.Join(defaults.PreCommit, "\n")
}

// ToValidationCommands converts the provider config to ValidationCommands struct.
func (cfg *ValidationProviderConfig) ToValidationCommands() ValidationCommands {
	return ValidationCommands{
		Format:      ParseMultilineInput(cfg.FormatCmds),
		Lint:        ParseMultilineInput(cfg.LintCmds),
		Test:        ParseMultilineInput(cfg.TestCmds),
		PreCommit:   ParseMultilineInput(cfg.PreCommitCmds),
		CustomPrePR: ParseMultilineInput(cfg.CustomPrePR),
	}
}

// ToValidationConfig converts the provider config to ValidationConfig struct.
func (cfg *ValidationProviderConfig) ToValidationConfig() ValidationConfig {
	return ValidationConfig{
		Commands: cfg.ToValidationCommands(),
	}
}

// extractBaseCommand extracts the base command from a full command string.
// For example, "magex format:fix" returns "magex".
func extractBaseCommand(fullCmd string) string {
	parts := strings.Fields(fullCmd)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// ValidateCommand checks if a command is executable.
// It returns whether the command exists in PATH and a warning message if not.
// The function only checks the base command (first word), ignoring arguments.
func ValidateCommand(cmd string) (exists bool, warning string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false, "empty command"
	}

	baseCmd := extractBaseCommand(cmd)
	if baseCmd == "" {
		return false, "empty command"
	}

	_, err := exec.LookPath(baseCmd)
	if err != nil {
		return false, fmt.Sprintf("command '%s' not found in PATH", baseCmd)
	}
	return true, ""
}

// ValidateCommands checks all commands in a multiline string.
// Returns a slice of warnings for commands that are not found.
// Commands are not rejected - only warnings are returned.
func ValidateCommands(multilineInput string) []string {
	var warnings []string
	cmds := ParseMultilineInput(multilineInput)
	for _, cmd := range cmds {
		if exists, warning := ValidateCommand(cmd); !exists {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

// ValidateAllConfigCommands validates all commands in the configuration.
// Returns a map of category to warnings for that category.
func ValidateAllConfigCommands(cfg *ValidationProviderConfig) map[string][]string {
	result := make(map[string][]string)

	if warnings := ValidateCommands(cfg.FormatCmds); len(warnings) > 0 {
		result["Format"] = warnings
	}
	if warnings := ValidateCommands(cfg.LintCmds); len(warnings) > 0 {
		result["Lint"] = warnings
	}
	if warnings := ValidateCommands(cfg.TestCmds); len(warnings) > 0 {
		result["Test"] = warnings
	}
	if warnings := ValidateCommands(cfg.PreCommitCmds); len(warnings) > 0 {
		result["Pre-commit"] = warnings
	}
	if warnings := ValidateCommands(cfg.CustomPrePR); len(warnings) > 0 {
		result["Custom Pre-PR"] = warnings
	}

	return result
}
