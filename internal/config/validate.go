package config

import (
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/errors"
)

// CI poll interval boundaries for validation.
const (
	// MinCIPollInterval is the minimum allowed CI poll interval.
	MinCIPollInterval = 1 * time.Second
	// MaxCIPollInterval is the maximum allowed CI poll interval.
	MaxCIPollInterval = 10 * time.Minute
)

// Validate checks the configuration for invalid or inconsistent values.
// It returns an error describing the first validation failure found.
//
// Validation rules:
//   - AI timeout must be positive
//   - AI max turns must be between 1 and 100
//   - CI timeout must be positive
//   - CI poll interval must be between 1 second and 10 minutes
//   - Git base branch must not be empty
//   - Validation timeout must be positive
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.ErrConfigNil
	}

	// Validate AI config
	if err := validateAIConfig(&cfg.AI); err != nil {
		return fmt.Errorf("validate AI config: %w", err)
	}

	// Validate Git config
	if err := validateGitConfig(&cfg.Git); err != nil {
		return fmt.Errorf("validate git config: %w", err)
	}

	// Validate CI config
	if err := validateCIConfig(&cfg.CI); err != nil {
		return fmt.Errorf("validate CI config: %w", err)
	}

	// Validate Validation config
	if err := validateValidationConfig(&cfg.Validation); err != nil {
		return fmt.Errorf("validate validation config: %w", err)
	}

	return nil
}

// validateAIConfig checks AI-specific configuration values.
func validateAIConfig(cfg *AIConfig) error {
	if cfg.Timeout <= 0 {
		return errors.Wrapf(errors.ErrConfigInvalidAI,
			"ai.timeout must be positive, got %s", cfg.Timeout)
	}

	if cfg.MaxTurns < 1 || cfg.MaxTurns > 100 {
		return errors.Wrapf(errors.ErrConfigInvalidAI,
			"ai.max_turns must be between 1 and 100, got %d", cfg.MaxTurns)
	}

	return nil
}

// validateGitConfig checks Git-specific configuration values.
func validateGitConfig(cfg *GitConfig) error {
	if cfg.BaseBranch == "" {
		return errors.Wrap(errors.ErrConfigInvalidGit,
			"git.base_branch must not be empty")
	}

	return nil
}

// validateCIConfig checks CI-specific configuration values.
func validateCIConfig(cfg *CIConfig) error {
	if cfg.Timeout <= 0 {
		return errors.Wrapf(errors.ErrConfigInvalidCI,
			"ci.timeout must be positive, got %s", cfg.Timeout)
	}

	if cfg.PollInterval < MinCIPollInterval || cfg.PollInterval > MaxCIPollInterval {
		return errors.Wrapf(errors.ErrConfigInvalidCI,
			"ci.poll_interval must be between %s and %s, got %s",
			MinCIPollInterval, MaxCIPollInterval, cfg.PollInterval)
	}

	return nil
}

// validateValidationConfig checks Validation-specific configuration values.
func validateValidationConfig(cfg *ValidationConfig) error {
	if cfg.Timeout <= 0 {
		return errors.Wrapf(errors.ErrConfigInvalidValidation,
			"validation.timeout must be positive, got %s", cfg.Timeout)
	}

	// Validate AI retry settings
	if cfg.MaxAIRetryAttempts < 0 {
		return errors.Wrapf(errors.ErrConfigInvalidValidation,
			"validation.max_ai_retry_attempts cannot be negative, got %d", cfg.MaxAIRetryAttempts)
	}

	// If AI retry is enabled, ensure at least 1 attempt is allowed
	if cfg.AIRetryEnabled && cfg.MaxAIRetryAttempts == 0 {
		return errors.Wrapf(errors.ErrConfigInvalidValidation,
			"validation.max_ai_retry_attempts must be at least 1 when ai_retry_enabled is true")
	}

	return nil
}
