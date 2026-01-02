package config

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
)

// Load reads configuration from all available sources with proper precedence.
// Configuration is loaded in the following order (highest precedence first):
//  1. Environment variables (ATLAS_* prefix)
//  2. Project config (.atlas/config.yaml)
//  3. Global config (~/.atlas/config.yaml)
//  4. Built-in defaults
//
// For CLI flag overrides, use LoadWithOverrides instead.
//
// The function returns an error only for actual configuration problems,
// not for missing config files (which are expected in many scenarios).
//
// The context parameter is accepted for API consistency and future use,
// but is not currently used for cancellation since config file reads are
// typically fast local I/O operations.
func Load(_ context.Context) (*Config, error) {
	v := viper.New()

	// Set defaults first (lowest precedence)
	setDefaults(v)

	// Configure environment variables (highest precedence after CLI flags)
	v.SetEnvPrefix("ATLAS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Load global config first (lower precedence)
	// Global config provides user-wide defaults that can be overridden per-project
	if err := loadGlobalConfig(v); err != nil {
		return nil, err
	}

	// Load project config (higher precedence, merges over global)
	// Project config allows per-project customization
	if err := loadProjectConfig(v); err != nil {
		return nil, err
	}

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg, viperDecoderOption()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Validate the configuration
	if err := Validate(&cfg); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	return &cfg, nil
}

// loadGlobalConfig attempts to load the global config file (~/.atlas/config.yaml).
// Returns nil if the file doesn't exist or home directory cannot be determined.
func loadGlobalConfig(v *viper.Viper) error {
	globalConfigPath, ok := getGlobalConfigPathIfExists()
	if !ok {
		// Global config doesn't exist or home dir unavailable, skip silently
		return nil
	}

	v.SetConfigFile(globalConfigPath)
	if err := v.ReadInConfig(); err != nil {
		var configNotFoundErr viper.ConfigFileNotFoundError
		if !stderrors.As(err, &configNotFoundErr) {
			return errors.Wrap(err, "failed to read global config file")
		}
	}
	return nil
}

// getGlobalConfigPathIfExists returns the global config path if it exists.
// Returns empty string and false if the home directory cannot be determined
// or the config file does not exist.
func getGlobalConfigPathIfExists() (string, bool) {
	globalDir, err := GlobalConfigDir()
	if err != nil {
		return "", false
	}

	globalConfigPath := filepath.Join(globalDir, "config.yaml")
	if _, err := os.Stat(globalConfigPath); err != nil {
		return "", false
	}

	return globalConfigPath, true
}

// loadProjectConfig attempts to load the project config file (.atlas/config.yaml).
// Returns nil if the file doesn't exist.
func loadProjectConfig(v *viper.Viper) error {
	projectConfigPath := ProjectConfigPath()
	if !fileExists(projectConfigPath) {
		// Project config doesn't exist, skip silently
		return nil
	}

	v.SetConfigFile(projectConfigPath)
	if err := v.MergeInConfig(); err != nil {
		var configNotFoundErr viper.ConfigFileNotFoundError
		if !stderrors.As(err, &configNotFoundErr) {
			return errors.Wrap(err, "failed to read project config file")
		}
	}
	return nil
}

// fileExists returns true if the file at path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// LoadWithOverrides loads configuration and applies CLI flag overrides.
// The overrides parameter contains values from CLI flags which have the
// highest precedence in the configuration hierarchy.
//
// Only non-zero values in overrides are applied. Zero values are ignored
// to allow partial overrides.
func LoadWithOverrides(ctx context.Context, overrides *Config) (*Config, error) {
	// Load base configuration first
	cfg, err := Load(ctx)
	if err != nil {
		return nil, err
	}

	// Apply overrides if provided
	if overrides != nil {
		applyOverrides(cfg, overrides)
	}

	// Re-validate after applying overrides
	if err := Validate(cfg); err != nil {
		return nil, errors.Wrap(err, "invalid configuration after overrides")
	}

	return cfg, nil
}

// LoadFromPaths loads configuration from specific file paths for testing.
// This function allows precise control over which config files are loaded.
//
// projectConfigPath is the path to project-level config (higher priority).
// globalConfigPath is the path to global config (lower priority).
// Either path can be empty to skip that level.
func LoadFromPaths(_ context.Context, projectConfigPath, globalConfigPath string) (*Config, error) {
	v := viper.New()

	// Set defaults first
	setDefaults(v)

	// Configure environment variables
	v.SetEnvPrefix("ATLAS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Load global config first (lower precedence)
	if globalConfigPath != "" {
		v.SetConfigFile(globalConfigPath)
		if err := v.ReadInConfig(); err != nil {
			var configNotFoundErr viper.ConfigFileNotFoundError
			if !stderrors.As(err, &configNotFoundErr) && !os.IsNotExist(err) {
				return nil, errors.Wrapf(err, "failed to read global config: %s", globalConfigPath)
			}
		}
	}

	// Load project config (higher precedence, merges over global)
	if projectConfigPath != "" {
		v.SetConfigFile(projectConfigPath)
		if err := v.MergeInConfig(); err != nil {
			var configNotFoundErr viper.ConfigFileNotFoundError
			if !stderrors.As(err, &configNotFoundErr) && !os.IsNotExist(err) {
				return nil, errors.Wrapf(err, "failed to read project config: %s", projectConfigPath)
			}
		}
	}

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg, viperDecoderOption()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Validate the configuration
	if err := Validate(&cfg); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	return &cfg, nil
}

// setDefaults configures all default values on the Viper instance.
// These defaults match the values from DefaultConfig().
// IMPORTANT: Keys must match the YAML tag names exactly for proper mapping.
func setDefaults(v *viper.Viper) {
	// AI defaults
	v.SetDefault("ai.model", "sonnet")
	v.SetDefault("ai.api_key_env_var", "ANTHROPIC_API_KEY")
	v.SetDefault("ai.timeout", constants.DefaultAITimeout)
	v.SetDefault("ai.max_turns", 10)

	// Git defaults
	v.SetDefault("git.base_branch", "main")
	v.SetDefault("git.auto_proceed_git", true)
	v.SetDefault("git.remote", "origin")

	// Worktree defaults
	v.SetDefault("worktree.base_dir", "")
	v.SetDefault("worktree.naming_suffix", "")

	// CI defaults
	v.SetDefault("ci.timeout", constants.DefaultCITimeout)
	v.SetDefault("ci.poll_interval", constants.CIPollInterval)
	v.SetDefault("ci.required_workflows", []string{})

	// Templates defaults
	v.SetDefault("templates.default_template", "")
	v.SetDefault("templates.custom_templates", map[string]string{})

	// Validation defaults
	v.SetDefault("validation.commands.format", []string{})
	v.SetDefault("validation.commands.lint", []string{})
	v.SetDefault("validation.commands.test", []string{})
	v.SetDefault("validation.commands.pre_commit", []string{})
	v.SetDefault("validation.commands.custom_pre_pr", []string{})
	v.SetDefault("validation.timeout", 5*time.Minute)
	v.SetDefault("validation.parallel_execution", true)
	v.SetDefault("validation.template_overrides", map[string]interface{}{})

	// Notifications defaults
	v.SetDefault("notifications.bell", true)
	v.SetDefault("notifications.events", []string{"awaiting_approval", "validation_failed"})
}

// applyOverrides merges non-zero override values into the config.
// Only non-zero values are applied to allow partial overrides.
//
// IMPORTANT: Boolean fields (AutoProceedGit, ParallelExecution, Bell) cannot
// be overridden to false using this function because Go's zero value for bool
// is false, making it impossible to distinguish "explicitly set to false" from
// "not set". CLI implementations should handle boolean flags separately:
//
//	// Example CLI handling for bool flags:
//	if cmd.Flags().Changed("auto-proceed-git") {
//	    cfg.Git.AutoProceedGit = autoGitFlag  // Use flag value directly
//	}
func applyOverrides(cfg, overrides *Config) {
	// AI overrides
	if overrides.AI.Model != "" {
		cfg.AI.Model = overrides.AI.Model
	}
	if overrides.AI.APIKeyEnvVar != "" {
		cfg.AI.APIKeyEnvVar = overrides.AI.APIKeyEnvVar
	}
	if overrides.AI.Timeout != 0 {
		cfg.AI.Timeout = overrides.AI.Timeout
	}
	if overrides.AI.MaxTurns != 0 {
		cfg.AI.MaxTurns = overrides.AI.MaxTurns
	}

	// Git overrides
	if overrides.Git.BaseBranch != "" {
		cfg.Git.BaseBranch = overrides.Git.BaseBranch
	}
	// AutoProceedGit is a bool - we can't distinguish false from unset,
	// so we don't override it here. Use explicit flag handling in CLI.
	if overrides.Git.Remote != "" {
		cfg.Git.Remote = overrides.Git.Remote
	}

	// Worktree overrides
	if overrides.Worktree.BaseDir != "" {
		cfg.Worktree.BaseDir = overrides.Worktree.BaseDir
	}
	if overrides.Worktree.NamingSuffix != "" {
		cfg.Worktree.NamingSuffix = overrides.Worktree.NamingSuffix
	}

	// CI overrides
	if overrides.CI.Timeout != 0 {
		cfg.CI.Timeout = overrides.CI.Timeout
	}
	if overrides.CI.PollInterval != 0 {
		cfg.CI.PollInterval = overrides.CI.PollInterval
	}
	if len(overrides.CI.RequiredWorkflows) > 0 {
		cfg.CI.RequiredWorkflows = overrides.CI.RequiredWorkflows
	}

	// Templates overrides
	if overrides.Templates.DefaultTemplate != "" {
		cfg.Templates.DefaultTemplate = overrides.Templates.DefaultTemplate
	}
	if len(overrides.Templates.CustomTemplates) > 0 {
		if cfg.Templates.CustomTemplates == nil {
			cfg.Templates.CustomTemplates = make(map[string]string)
		}
		for k, v := range overrides.Templates.CustomTemplates {
			cfg.Templates.CustomTemplates[k] = v
		}
	}

	// Validation overrides (extracted to reduce complexity)
	applyValidationOverrides(cfg, overrides)
	// ParallelExecution is a bool - same caveat as AutoProceedGit

	// Notifications overrides (Bell is a bool - same caveat)
	if len(overrides.Notifications.Events) > 0 {
		cfg.Notifications.Events = overrides.Notifications.Events
	}
}

// applyValidationOverrides applies validation-related overrides to the config.
// This is extracted from applyOverrides to reduce cognitive complexity.
func applyValidationOverrides(cfg, overrides *Config) {
	if len(overrides.Validation.Commands.Format) > 0 {
		cfg.Validation.Commands.Format = overrides.Validation.Commands.Format
	}
	if len(overrides.Validation.Commands.Lint) > 0 {
		cfg.Validation.Commands.Lint = overrides.Validation.Commands.Lint
	}
	if len(overrides.Validation.Commands.Test) > 0 {
		cfg.Validation.Commands.Test = overrides.Validation.Commands.Test
	}
	if len(overrides.Validation.Commands.PreCommit) > 0 {
		cfg.Validation.Commands.PreCommit = overrides.Validation.Commands.PreCommit
	}
	if len(overrides.Validation.Commands.CustomPrePR) > 0 {
		cfg.Validation.Commands.CustomPrePR = overrides.Validation.Commands.CustomPrePR
	}
	if overrides.Validation.Timeout != 0 {
		cfg.Validation.Timeout = overrides.Validation.Timeout
	}
	if len(overrides.Validation.TemplateOverrides) > 0 {
		if cfg.Validation.TemplateOverrides == nil {
			cfg.Validation.TemplateOverrides = make(map[string]TemplateOverrideConfig)
		}
		for k, v := range overrides.Validation.TemplateOverrides {
			cfg.Validation.TemplateOverrides[k] = v
		}
	}
}

// LoadWithWorktree loads configuration with worktree inheritance.
// Config is loaded in order (highest precedence first):
//  1. Environment variables (ATLAS_* prefix)
//  2. Worktree config (worktreePath/.atlas/config.yaml) - if different from main repo
//  3. Main repo config (mainRepoPath/.atlas/config.yaml)
//  4. Global config (~/.atlas/config.yaml)
//  5. Built-in defaults
//
// This enables worktree-specific overrides while inheriting base settings.
// The worktree config is only loaded if worktreePath differs from mainRepoPath.
func LoadWithWorktree(_ context.Context, mainRepoPath, worktreePath string) (*Config, error) {
	v := viper.New()

	// Set defaults first (lowest precedence)
	setDefaults(v)

	// Configure environment variables (highest precedence after CLI flags)
	v.SetEnvPrefix("ATLAS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Load global config first (lowest file precedence)
	if err := loadGlobalConfig(v); err != nil {
		return nil, err
	}

	// Load main repo config (middle precedence)
	mainConfigPath := filepath.Join(mainRepoPath, ".atlas", "config.yaml")
	if fileExists(mainConfigPath) {
		v.SetConfigFile(mainConfigPath)
		if err := v.MergeInConfig(); err != nil {
			var configNotFoundErr viper.ConfigFileNotFoundError
			if !stderrors.As(err, &configNotFoundErr) {
				return nil, errors.Wrap(err, "failed to read main repo config")
			}
		}
	}

	// Load worktree config if different from main repo (highest file precedence)
	if err := mergeWorktreeConfig(v, worktreePath, mainRepoPath); err != nil {
		return nil, err
	}

	// Unmarshal into Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg, viperDecoderOption()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Validate the configuration
	if err := Validate(&cfg); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	return &cfg, nil
}

// viperDecoderOption returns the decoder options for Viper unmarshal.
// This configures mapstructure to handle time.Duration conversion from strings.
func viperDecoderOption() viper.DecoderConfigOption {
	return viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	)
}

// mergeWorktreeConfig loads and merges the worktree-specific config if it exists.
// Returns nil if worktree is same as main repo or config doesn't exist.
func mergeWorktreeConfig(v *viper.Viper, worktreePath, mainRepoPath string) error {
	if worktreePath == mainRepoPath {
		return nil
	}

	worktreeConfigPath := filepath.Join(worktreePath, ".atlas", "config.yaml")
	if !fileExists(worktreeConfigPath) {
		return nil
	}

	v.SetConfigFile(worktreeConfigPath)
	if err := v.MergeInConfig(); err != nil {
		var configNotFoundErr viper.ConfigFileNotFoundError
		if !stderrors.As(err, &configNotFoundErr) {
			return errors.Wrap(err, "failed to read worktree config")
		}
	}
	return nil
}
