package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestValidate_NilConfig tests that nil config returns error
func TestValidate_NilConfig(t *testing.T) {
	t.Parallel()

	err := Validate(nil)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrConfigNil)
}

// TestValidate_DefaultConfig tests that default config is valid
func TestValidate_DefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	err := Validate(cfg)

	require.NoError(t, err)
}

// TestValidate_MinimumBoundaryValues tests minimum valid values
func TestValidate_MinimumBoundaryValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  1 * time.Second,
			MaxTurns: 1,
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		CI: CIConfig{
			Timeout:      1 * time.Second,
			PollInterval: 1 * time.Second,
		},
		Validation: ValidationConfig{
			Timeout:            1 * time.Second,
			MaxAIRetryAttempts: 0,
			AIRetryEnabled:     false,
		},
	}

	err := Validate(cfg)

	require.NoError(t, err)
}

// TestValidate_MaximumBoundaryValues tests maximum valid values
func TestValidate_MaximumBoundaryValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  24 * time.Hour,
			MaxTurns: 100,
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		CI: CIConfig{
			Timeout:      24 * time.Hour,
			PollInterval: 10 * time.Minute,
		},
		Validation: ValidationConfig{
			Timeout:            24 * time.Hour,
			MaxAIRetryAttempts: 10,
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.NoError(t, err)
}

// TestValidateAIConfig_ZeroTimeout tests zero timeout is invalid
func TestValidateAIConfig_ZeroTimeout(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  0,
			MaxTurns: 10,
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		CI: CIConfig{
			Timeout:      1 * time.Minute,
			PollInterval: 30 * time.Second,
		},
		Validation: ValidationConfig{
			Timeout:            1 * time.Minute,
			MaxAIRetryAttempts: 3,
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidAI)
	assert.Contains(t, err.Error(), "ai.timeout must be positive")
}

// TestValidateAIConfig_NegativeTimeout tests negative timeout is invalid
func TestValidateAIConfig_NegativeTimeout(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  -1 * time.Minute,
			MaxTurns: 10,
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		CI: CIConfig{
			Timeout:      1 * time.Minute,
			PollInterval: 30 * time.Second,
		},
		Validation: ValidationConfig{
			Timeout:            1 * time.Minute,
			MaxAIRetryAttempts: 3,
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidAI)
	assert.Contains(t, err.Error(), "ai.timeout must be positive")
}

// TestValidateAIConfig_MaxTurns tests max turns validation
func TestValidateAIConfig_MaxTurns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxTurns int
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "zero_turns",
			maxTurns: 0,
			wantErr:  true,
			errMsg:   "ai.max_turns must be between 1 and 100",
		},
		{
			name:     "negative_turns",
			maxTurns: -1,
			wantErr:  true,
			errMsg:   "ai.max_turns must be between 1 and 100",
		},
		{
			name:     "too_many_turns",
			maxTurns: 101,
			wantErr:  true,
			errMsg:   "ai.max_turns must be between 1 and 100",
		},
		{
			name:     "boundary_min_valid",
			maxTurns: 1,
			wantErr:  false,
		},
		{
			name:     "boundary_max_valid",
			maxTurns: 100,
			wantErr:  false,
		},
		{
			name:     "middle_valid",
			maxTurns: 10,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: tt.maxTurns,
				},
				Git: GitConfig{
					BaseBranch: "main",
				},
				CI: CIConfig{
					Timeout:      1 * time.Minute,
					PollInterval: 30 * time.Second,
				},
				Validation: ValidationConfig{
					Timeout:            1 * time.Minute,
					MaxAIRetryAttempts: 3,
					AIRetryEnabled:     true,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidAI)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateGitConfig_EmptyBaseBranch tests empty base branch is invalid
func TestValidateGitConfig_EmptyBaseBranch(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  30 * time.Minute,
			MaxTurns: 10,
		},
		Git: GitConfig{
			BaseBranch: "",
		},
		CI: CIConfig{
			Timeout:      1 * time.Minute,
			PollInterval: 30 * time.Second,
		},
		Validation: ValidationConfig{
			Timeout:            1 * time.Minute,
			MaxAIRetryAttempts: 3,
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidGit)
	assert.Contains(t, err.Error(), "git.base_branch must not be empty")
}

// TestValidateGitConfig_ValidBaseBranch tests valid base branch names
func TestValidateGitConfig_ValidBaseBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		baseBranch string
	}{
		{"main", "main"},
		{"master", "master"},
		{"develop", "develop"},
		{"feature/test", "feature/test"},
		{"single_char", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: 10,
				},
				Git: GitConfig{
					BaseBranch: tt.baseBranch,
				},
				CI: CIConfig{
					Timeout:      1 * time.Minute,
					PollInterval: 30 * time.Second,
				},
				Validation: ValidationConfig{
					Timeout:            1 * time.Minute,
					MaxAIRetryAttempts: 3,
					AIRetryEnabled:     true,
				},
			}

			err := Validate(cfg)

			require.NoError(t, err)
		})
	}
}

// TestValidateCIConfig_Timeout tests CI timeout validation
func TestValidateCIConfig_Timeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "zero_timeout",
			timeout: 0,
			wantErr: true,
			errMsg:  "ci.timeout must be positive",
		},
		{
			name:    "negative_timeout",
			timeout: -5 * time.Minute,
			wantErr: true,
			errMsg:  "ci.timeout must be positive",
		},
		{
			name:    "positive_timeout",
			timeout: 10 * time.Minute,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: 10,
				},
				Git: GitConfig{
					BaseBranch: "main",
				},
				CI: CIConfig{
					Timeout:      tt.timeout,
					PollInterval: 30 * time.Second,
				},
				Validation: ValidationConfig{
					Timeout:            1 * time.Minute,
					MaxAIRetryAttempts: 3,
					AIRetryEnabled:     true,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidCI)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateCIConfig_PollInterval tests poll interval validation
func TestValidateCIConfig_PollInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pollInterval time.Duration
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "zero_interval",
			pollInterval: 0,
			wantErr:      true,
			errMsg:       "ci.poll_interval must be between",
		},
		{
			name:         "below_minimum",
			pollInterval: 999 * time.Millisecond,
			wantErr:      true,
			errMsg:       "ci.poll_interval must be between",
		},
		{
			name:         "above_maximum",
			pollInterval: 11 * time.Minute,
			wantErr:      true,
			errMsg:       "ci.poll_interval must be between",
		},
		{
			name:         "boundary_minimum_valid",
			pollInterval: 1 * time.Second,
			wantErr:      false,
		},
		{
			name:         "boundary_maximum_valid",
			pollInterval: 10 * time.Minute,
			wantErr:      false,
		},
		{
			name:         "middle_valid",
			pollInterval: 30 * time.Second,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: 10,
				},
				Git: GitConfig{
					BaseBranch: "main",
				},
				CI: CIConfig{
					Timeout:      10 * time.Minute,
					PollInterval: tt.pollInterval,
				},
				Validation: ValidationConfig{
					Timeout:            1 * time.Minute,
					MaxAIRetryAttempts: 3,
					AIRetryEnabled:     true,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidCI)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateValidationConfig_Timeout tests validation timeout
func TestValidateValidationConfig_Timeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
		errMsg  string
	}{
		{
			name:    "zero_timeout",
			timeout: 0,
			wantErr: true,
			errMsg:  "validation.timeout must be positive",
		},
		{
			name:    "negative_timeout",
			timeout: -10 * time.Minute,
			wantErr: true,
			errMsg:  "validation.timeout must be positive",
		},
		{
			name:    "positive_timeout",
			timeout: 5 * time.Minute,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: 10,
				},
				Git: GitConfig{
					BaseBranch: "main",
				},
				CI: CIConfig{
					Timeout:      10 * time.Minute,
					PollInterval: 30 * time.Second,
				},
				Validation: ValidationConfig{
					Timeout:            tt.timeout,
					MaxAIRetryAttempts: 3,
					AIRetryEnabled:     true,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidValidation)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateValidationConfig_MaxAIRetryAttempts tests retry attempts validation
func TestValidateValidationConfig_MaxAIRetryAttempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		retryEnabled     bool
		maxRetryAttempts int
		wantErr          bool
		errMsg           string
	}{
		{
			name:             "negative_attempts",
			retryEnabled:     false,
			maxRetryAttempts: -1,
			wantErr:          true,
			errMsg:           "max_ai_retry_attempts cannot be negative",
		},
		{
			name:             "zero_attempts_retry_enabled",
			retryEnabled:     true,
			maxRetryAttempts: 0,
			wantErr:          true,
			errMsg:           "max_ai_retry_attempts must be at least 1",
		},
		{
			name:             "zero_attempts_retry_disabled",
			retryEnabled:     false,
			maxRetryAttempts: 0,
			wantErr:          false,
		},
		{
			name:             "positive_attempts_retry_enabled",
			retryEnabled:     true,
			maxRetryAttempts: 3,
			wantErr:          false,
		},
		{
			name:             "positive_attempts_retry_disabled",
			retryEnabled:     false,
			maxRetryAttempts: 3,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				AI: AIConfig{
					Timeout:  30 * time.Minute,
					MaxTurns: 10,
				},
				Git: GitConfig{
					BaseBranch: "main",
				},
				CI: CIConfig{
					Timeout:      10 * time.Minute,
					PollInterval: 30 * time.Second,
				},
				Validation: ValidationConfig{
					Timeout:            5 * time.Minute,
					MaxAIRetryAttempts: tt.maxRetryAttempts,
					AIRetryEnabled:     tt.retryEnabled,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidValidation)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidate_ErrorWrapping tests that errors are properly wrapped
func TestValidate_ErrorWrapping(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  0, // Invalid
			MaxTurns: 10,
		},
		Git: GitConfig{
			BaseBranch: "main",
		},
		CI: CIConfig{
			Timeout:      10 * time.Minute,
			PollInterval: 30 * time.Second,
		},
		Validation: ValidationConfig{
			Timeout:            5 * time.Minute,
			MaxAIRetryAttempts: 3,
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	// Verify error unwrapping works
	require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidAI)
}

// TestValidate_MultipleErrors tests that validation stops at first error
func TestValidate_MultipleErrors(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		AI: AIConfig{
			Timeout:  0,  // Invalid (first error)
			MaxTurns: -1, // Also invalid
		},
		Git: GitConfig{
			BaseBranch: "", // Also invalid
		},
		CI: CIConfig{
			Timeout:      0, // Also invalid
			PollInterval: 0, // Also invalid
		},
		Validation: ValidationConfig{
			Timeout:            0,  // Also invalid
			MaxAIRetryAttempts: -1, // Also invalid
			AIRetryEnabled:     true,
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	// Should fail on first validation (AI timeout)
	require.ErrorIs(t, err, atlaserrors.ErrConfigInvalidAI)
	assert.Contains(t, err.Error(), "ai.timeout must be positive")
}
