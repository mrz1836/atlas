// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
)

// AIProviderConfig holds AI provider configuration values collected from user input.
// This struct is used for collecting configuration before saving.
type AIProviderConfig struct {
	// Model is the default Claude model to use (sonnet|opus|haiku).
	Model string
	// APIKeyEnvVar is the name of the environment variable containing the API key.
	APIKeyEnvVar string
	// Timeout is the default timeout for AI operations as a duration string.
	Timeout string
	// MaxTurns is the maximum number of turns per AI step.
	MaxTurns int
}

// AIConfigDefaults returns the default values for AI configuration.
func AIConfigDefaults() AIProviderConfig {
	return AIProviderConfig{
		Model:        "sonnet",
		APIKeyEnvVar: "ANTHROPIC_API_KEY",
		Timeout:      "30m",
		MaxTurns:     10,
	}
}

// Model constants for descriptive labels.
const (
	ModelSonnet = "sonnet"
	ModelOpus   = "opus"
	ModelHaiku  = "haiku"
)

// getModelOptions returns the available model options for selection.
func getModelOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("Claude Sonnet (faster, balanced)", ModelSonnet),
		huh.NewOption("Claude Opus (most capable)", ModelOpus),
		huh.NewOption("Claude Haiku (fast, cost-efficient)", ModelHaiku),
	}
}

// envVarNameRegex validates environment variable names.
// Must start with letter or underscore, followed by letters, digits, or underscores.
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// NewAIConfigForm creates a Charm Huh form for AI configuration.
// The form collects model, API key env var, timeout, and max turns settings.
// IMPORTANT: maxTurnsStr must be passed from the caller so the value can be
// captured after form.Run() completes. The caller is responsible for parsing
// maxTurnsStr back to cfg.MaxTurns.
func NewAIConfigForm(cfg *AIProviderConfig, maxTurnsStr *string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default AI Model").
				Description("Choose the default Claude model for ATLAS tasks").
				Options(getModelOptions()...).
				Value(&cfg.Model),
			huh.NewInput().
				Title("API Key Environment Variable").
				Description("Name of the environment variable containing your Anthropic API key").
				Value(&cfg.APIKeyEnvVar).
				Placeholder("ANTHROPIC_API_KEY").
				Validate(validateEnvVarName),
			huh.NewInput().
				Title("Default AI Timeout").
				Description("Maximum time for AI operations (e.g., 30m, 1h, 1h30m)").
				Value(&cfg.Timeout).
				Placeholder("30m").
				Validate(validateTimeoutFormat),
			huh.NewInput().
				Title("Max Turns per Step").
				Description("Maximum number of AI turns per step (1-100)").
				Value(maxTurnsStr).
				Placeholder("10").
				Validate(validateMaxTurns),
		),
	).WithTheme(tui.AtlasTheme())
}

// createAIConfigForm is the default factory for creating AI config forms.
// This variable can be overridden in tests to inject mock forms.
//
//nolint:gochecknoglobals // Test injection point - standard Go testing pattern
var createAIConfigForm = defaultCreateAIConfigForm

// defaultCreateAIConfigForm creates the actual Charm Huh form for AI configuration.
func defaultCreateAIConfigForm(cfg *AIProviderConfig, maxTurnsStr *string) formRunner {
	return NewAIConfigForm(cfg, maxTurnsStr)
}

// CollectAIConfigInteractive runs the AI configuration form and returns the collected config.
// It validates all inputs and handles form errors.
func CollectAIConfigInteractive(ctx context.Context, cfg *AIProviderConfig) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	defaults := AIConfigDefaults()

	// Initialize with defaults if empty
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.APIKeyEnvVar == "" {
		cfg.APIKeyEnvVar = defaults.APIKeyEnvVar
	}
	if cfg.Timeout == "" {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = defaults.MaxTurns
	}

	// Create string for max turns input (huh.Input works with strings)
	// This must be defined here so we can capture the value after form.Run()
	maxTurnsStr := strconv.Itoa(cfg.MaxTurns)

	form := createAIConfigForm(cfg, &maxTurnsStr)
	if err := form.Run(); err != nil {
		return fmt.Errorf("AI configuration failed: %w", err)
	}

	// Parse maxTurnsStr back to cfg.MaxTurns after form completes
	cfg.MaxTurns = ParseMaxTurnsWithDefault(maxTurnsStr, defaults.MaxTurns)

	return nil
}

// CollectAIConfigNonInteractive returns a configuration with default values.
// Used when running in non-interactive mode.
func CollectAIConfigNonInteractive() AIProviderConfig {
	return AIConfigDefaults()
}

// validateEnvVarName validates that the input is a valid environment variable name.
func validateEnvVarName(s string) error {
	if s == "" {
		return fmt.Errorf("%w: environment variable name", atlaserrors.ErrEmptyValue)
	}
	if !IsValidEnvVarName(s) {
		return fmt.Errorf("%w: must start with letter or underscore, contain only letters, digits, or underscores", atlaserrors.ErrInvalidEnvVarName)
	}
	return nil
}

// IsValidEnvVarName checks if a string is a valid environment variable name.
// A valid name starts with a letter or underscore and contains only
// letters, digits, or underscores.
func IsValidEnvVarName(name string) bool {
	if name == "" {
		return false
	}
	return envVarNameRegex.MatchString(name)
}

// validateTimeoutFormat validates that the input is a valid duration string.
func validateTimeoutFormat(s string) error {
	if s == "" {
		return fmt.Errorf("%w: timeout", atlaserrors.ErrEmptyValue)
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("%w: use formats like 30m, 1h, 1h30m", atlaserrors.ErrInvalidDuration)
	}
	if duration <= 0 {
		return fmt.Errorf("%w: timeout must be positive", atlaserrors.ErrInvalidDuration)
	}
	return nil
}

// validateMaxTurns validates that the input is a valid max turns value (1-100).
func validateMaxTurns(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("%w: max turns", atlaserrors.ErrEmptyValue)
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("%w: max turns must be a number", atlaserrors.ErrValueOutOfRange)
	}
	if val < 1 || val > 100 {
		return fmt.Errorf("%w: max turns must be between 1 and 100", atlaserrors.ErrValueOutOfRange)
	}
	return nil
}

// CheckAPIKeyExists checks if the specified environment variable is set.
// Returns whether the key exists and a warning message if it doesn't.
func CheckAPIKeyExists(envVarName string) (exists bool, warning string) {
	value := os.Getenv(envVarName)
	if value == "" {
		return false, fmt.Sprintf("Warning: %s is not set in your environment. ATLAS will fail to run AI operations until this is configured.", envVarName)
	}
	return true, ""
}

// ValidateAIConfig validates an AIProviderConfig and returns any validation errors.
func ValidateAIConfig(cfg *AIProviderConfig) error {
	if cfg.Model == "" {
		return fmt.Errorf("%w: model", atlaserrors.ErrEmptyValue)
	}
	if cfg.Model != ModelSonnet && cfg.Model != ModelOpus && cfg.Model != ModelHaiku {
		return fmt.Errorf("%w: must be 'sonnet', 'opus', or 'haiku'", atlaserrors.ErrInvalidModel)
	}
	if err := validateEnvVarName(cfg.APIKeyEnvVar); err != nil {
		return err
	}
	if err := validateTimeoutFormat(cfg.Timeout); err != nil {
		return err
	}
	if cfg.MaxTurns < 1 || cfg.MaxTurns > 100 {
		return fmt.Errorf("%w: max turns must be between 1 and 100", atlaserrors.ErrValueOutOfRange)
	}
	return nil
}

// ParseMaxTurnsWithDefault parses a string to int, returning a default on error.
func ParseMaxTurnsWithDefault(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil || val < 1 || val > 100 {
		return defaultVal
	}
	return val
}

// ValidateTimeoutWithDefault validates and returns a timeout string.
// If the input is invalid, returns the default value.
func ValidateTimeoutWithDefault(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	duration, err := time.ParseDuration(s)
	if err != nil || duration <= 0 {
		return defaultVal
	}
	return s
}
