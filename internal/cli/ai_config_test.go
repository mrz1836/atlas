package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidEnvVarName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Valid cases
		{"simple uppercase", "API_KEY", true},
		{"simple lowercase", "api_key", true},
		{"mixed case", "Api_Key", true},
		{"starts with underscore", "_SECRET", true},
		{"single letter", "A", true},
		{"single underscore", "_", true},
		{"with numbers", "API_KEY_123", true},
		{"default anthropic key", "ANTHROPIC_API_KEY", true},

		// Invalid cases
		{"empty string", "", false},
		{"starts with number", "123_KEY", false},
		{"contains hyphen", "API-KEY", false},
		{"contains space", "API KEY", false},
		{"contains special char", "API@KEY", false},
		{"contains dot", "API.KEY", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidEnvVarName(tt.input)
			assert.Equal(t, tt.want, got, "IsValidEnvVarName(%q)", tt.input)
		})
	}
}

func TestCheckAPIKeyExists(t *testing.T) {
	tests := []struct {
		name       string
		envVar     string
		envValue   string
		wantExists bool
		wantWarn   bool
	}{
		{"key exists", "TEST_API_KEY", "sk-test-value-12345", true, false},
		{"key missing", "MISSING_KEY_XYZ", "", false, true},
		{"key empty string", "EMPTY_KEY_TEST", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envVar, tt.envValue)
			}
			exists, warning := CheckAPIKeyExists(tt.envVar)
			if tt.wantWarn {
				assert.False(t, exists)
				assert.NotEmpty(t, warning)
				assert.Contains(t, warning, tt.envVar)
			} else {
				assert.True(t, exists)
				assert.Empty(t, warning)
			}
		})
	}
}

func TestCollectAIConfigNonInteractive(t *testing.T) {
	cfg := CollectAIConfigNonInteractive()
	defaults := AIConfigDefaults()

	assert.Equal(t, defaults.Model, cfg.Model)
	assert.Equal(t, defaults.APIKeyEnvVar, cfg.APIKeyEnvVar)
	assert.Equal(t, defaults.Timeout, cfg.Timeout)
	assert.Equal(t, defaults.MaxTurns, cfg.MaxTurns)

	// Verify specific defaults
	assert.Equal(t, "sonnet", cfg.Model)
	assert.Equal(t, "ANTHROPIC_API_KEY", cfg.APIKeyEnvVar)
	assert.Equal(t, "30m", cfg.Timeout)
	assert.Equal(t, 10, cfg.MaxTurns)
}

func TestValidateAIConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AIProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with sonnet",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "30m",
				MaxTurns:     10,
			},
			wantErr: false,
		},
		{
			name: "valid config with opus",
			cfg: AIProviderConfig{
				Model:        "opus",
				APIKeyEnvVar: "MY_API_KEY",
				Timeout:      "1h",
				MaxTurns:     50,
			},
			wantErr: false,
		},
		{
			name: "valid config with haiku",
			cfg: AIProviderConfig{
				Model:        "haiku",
				APIKeyEnvVar: "CLAUDE_KEY",
				Timeout:      "1h30m",
				MaxTurns:     100,
			},
			wantErr: false,
		},
		{
			name: "empty model",
			cfg: AIProviderConfig{
				Model:        "",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "30m",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "value cannot be empty",
		},
		{
			name: "invalid model",
			cfg: AIProviderConfig{
				Model:        "gpt4",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "30m",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "invalid model",
		},
		{
			name: "empty env var",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "",
				Timeout:      "30m",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "value cannot be empty",
		},
		{
			name: "invalid env var name",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "123-invalid",
				Timeout:      "30m",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "invalid environment variable name",
		},
		{
			name: "invalid timeout format",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "invalid",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "invalid duration format",
		},
		{
			name: "empty timeout",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "",
				MaxTurns:     10,
			},
			wantErr: true,
			errMsg:  "value cannot be empty",
		},
		{
			name: "max turns too low",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "30m",
				MaxTurns:     0,
			},
			wantErr: true,
			errMsg:  "value out of range",
		},
		{
			name: "max turns too high",
			cfg: AIProviderConfig{
				Model:        "sonnet",
				APIKeyEnvVar: "ANTHROPIC_API_KEY",
				Timeout:      "30m",
				MaxTurns:     101,
			},
			wantErr: true,
			errMsg:  "value out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAIConfig(&tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseMaxTurnsWithDefault(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal int
		want       int
	}{
		{"valid number", "25", 10, 25},
		{"valid min", "1", 10, 1},
		{"valid max", "100", 10, 100},
		{"empty string", "", 10, 10},
		{"whitespace only", "   ", 10, 10},
		{"invalid string", "abc", 10, 10},
		{"negative number", "-5", 10, 10},
		{"too high", "101", 10, 10},
		{"zero", "0", 10, 10},
		{"float", "10.5", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMaxTurnsWithDefault(tt.input, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateTimeoutWithDefault(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultVal string
		want       string
	}{
		{"valid 30m", "30m", "30m", "30m"},
		{"valid 1h", "1h", "30m", "1h"},
		{"valid 1h30m", "1h30m", "30m", "1h30m"},
		{"valid 90m", "90m", "30m", "90m"},
		{"valid 2h", "2h", "30m", "2h"},
		{"empty string", "", "30m", "30m"},
		{"invalid format", "30", "30m", "30m"},
		{"invalid string", "abc", "30m", "30m"},
		{"negative duration", "-30m", "30m", "30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTimeoutWithDefault(tt.input, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewAIConfigForm(t *testing.T) {
	t.Run("creates form with config values", func(t *testing.T) {
		cfg := &AIProviderConfig{
			Model:        "opus",
			APIKeyEnvVar: "CUSTOM_KEY",
			Timeout:      "1h",
			MaxTurns:     50,
		}
		maxTurnsStr := "50"
		form := NewAIConfigForm(cfg, &maxTurnsStr)

		// Form should be created successfully
		assert.NotNil(t, form)

		// Config values should be preserved (form doesn't modify them until Run)
		assert.Equal(t, "opus", cfg.Model)
		assert.Equal(t, "CUSTOM_KEY", cfg.APIKeyEnvVar)
		assert.Equal(t, "1h", cfg.Timeout)
		assert.Equal(t, 50, cfg.MaxTurns)
	})

	t.Run("maxTurnsStr pointer is used for form binding", func(t *testing.T) {
		cfg := &AIProviderConfig{
			Model:        "sonnet",
			APIKeyEnvVar: "ANTHROPIC_API_KEY",
			Timeout:      "30m",
			MaxTurns:     10,
		}
		maxTurnsStr := "10"
		form := NewAIConfigForm(cfg, &maxTurnsStr)

		// Form should be created
		assert.NotNil(t, form)

		// maxTurnsStr should be modifiable by the form
		// (we can't easily test form.Run() without terminal, but we verify the binding)
		assert.Equal(t, "10", maxTurnsStr)
	})
}

func TestCollectAIConfigInteractive_MaxTurnsCapture(t *testing.T) {
	// This test verifies that the MaxTurns value would be properly captured
	// after form completion. Since we can't run the interactive form in tests,
	// we test the parsing logic that CollectAIConfigInteractive uses.
	t.Run("ParseMaxTurnsWithDefault correctly parses user input", func(t *testing.T) {
		defaults := AIConfigDefaults()

		// Simulate user entering "25" in the form
		maxTurnsStr := "25"
		result := ParseMaxTurnsWithDefault(maxTurnsStr, defaults.MaxTurns)
		assert.Equal(t, 25, result, "Should parse user input correctly")

		// Simulate user entering invalid value - should fall back to default
		maxTurnsStr = "invalid"
		result = ParseMaxTurnsWithDefault(maxTurnsStr, defaults.MaxTurns)
		assert.Equal(t, defaults.MaxTurns, result, "Should use default for invalid input")

		// Simulate user entering out-of-range value
		maxTurnsStr = "150"
		result = ParseMaxTurnsWithDefault(maxTurnsStr, defaults.MaxTurns)
		assert.Equal(t, defaults.MaxTurns, result, "Should use default for out-of-range input")
	})
}

func TestAIConfigDefaults(t *testing.T) {
	defaults := AIConfigDefaults()

	// Verify the default values are as expected by the story requirements
	assert.Equal(t, "sonnet", defaults.Model, "default model should be sonnet")
	assert.Equal(t, "ANTHROPIC_API_KEY", defaults.APIKeyEnvVar, "default API key env var")
	assert.Equal(t, "30m", defaults.Timeout, "default timeout should be 30m")
	assert.Equal(t, 10, defaults.MaxTurns, "default max turns should be 10")
}

func TestModelConstants(t *testing.T) {
	// Verify model constants exist
	assert.Equal(t, "sonnet", ModelSonnet)
	assert.Equal(t, "opus", ModelOpus)
	assert.Equal(t, "haiku", ModelHaiku)
}

func TestValidateMaxTurns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid value 10", "10", false, ""},
		{"valid value 1", "1", false, ""},
		{"valid value 100", "100", false, ""},
		{"valid value 50", "50", false, ""},
		{"empty string", "", true, "value cannot be empty"},
		{"whitespace only", "   ", true, "value cannot be empty"},
		{"non-numeric", "abc", true, "value out of range"},
		{"too low zero", "0", true, "value out of range"},
		{"too low negative", "-5", true, "value out of range"},
		{"too high 101", "101", true, "value out of range"},
		{"too high 500", "500", true, "value out of range"},
		{"float value", "10.5", true, "value out of range"},
		{"mixed chars", "10abc", true, "value out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMaxTurns(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateEnvVarName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid uppercase", "ANTHROPIC_API_KEY", false, ""},
		{"valid with underscore prefix", "_MY_KEY", false, ""},
		{"valid with numbers", "API_KEY_123", false, ""},
		{"valid simple", "KEY", false, ""},
		{"empty string", "", true, "value cannot be empty"},
		{"starts with number", "123_KEY", true, "invalid environment variable name"},
		{"contains hyphen", "MY-KEY", true, "invalid environment variable name"},
		{"contains space", "MY KEY", true, "invalid environment variable name"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEnvVarName(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTimeoutFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid 30m", "30m", false, ""},
		{"valid 1h", "1h", false, ""},
		{"valid 1h30m", "1h30m", false, ""},
		{"valid 90s", "90s", false, ""},
		{"valid 2h", "2h", false, ""},
		{"empty string", "", true, "value cannot be empty"},
		{"invalid format", "30", true, "invalid duration format"},
		{"invalid string", "abc", true, "invalid duration format"},
		{"negative duration", "-30m", true, "must be positive"},
		{"zero duration", "0s", true, "must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTimeoutFormat(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetModelOptions(t *testing.T) {
	options := getModelOptions()

	assert.Len(t, options, 3, "should have 3 model options")

	// Verify all model values are present
	values := make([]string, len(options))
	for i, opt := range options {
		values[i] = opt.Value
	}

	assert.Contains(t, values, ModelSonnet)
	assert.Contains(t, values, ModelOpus)
	assert.Contains(t, values, ModelHaiku)
}

// Phase 3: Form Interactions - Test AI config collection

func TestCollectAIConfigInteractive_ValidInput(t *testing.T) {
	// Save and restore original form factory
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	// Mock the form to simulate user input
	createAIConfigForm = func(cfg *AIProviderConfig, maxTurnsStr *string) formRunner {
		// Simulate user entering values
		cfg.Model = "opus"
		cfg.APIKeyEnvVar = "CUSTOM_API_KEY"
		cfg.Timeout = "1h"
		*maxTurnsStr = "20"
		return &mockFormRunner{}
	}

	cfg := &AIProviderConfig{}
	err := CollectAIConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, "opus", cfg.Model)
	assert.Equal(t, "CUSTOM_API_KEY", cfg.APIKeyEnvVar)
	assert.Equal(t, "1h", cfg.Timeout)
	assert.Equal(t, 20, cfg.MaxTurns)
}

func TestCollectAIConfigInteractive_CustomTimeout(t *testing.T) {
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	createAIConfigForm = func(cfg *AIProviderConfig, maxTurnsStr *string) formRunner {
		cfg.Timeout = "30m"
		*maxTurnsStr = "10"
		return &mockFormRunner{}
	}

	cfg := &AIProviderConfig{}
	err := CollectAIConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, "30m", cfg.Timeout)
	assert.Equal(t, 10, cfg.MaxTurns)
}

func TestCollectAIConfigInteractive_MaxTurnsParsing(t *testing.T) {
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	tests := []struct {
		name          string
		maxTurnsStr   string
		expectedTurns int
	}{
		{"valid number", "25", 25},
		{"default on empty", "", 10}, // Default from AIConfigDefaults
		{"default on invalid", "invalid", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createAIConfigForm = func(_ *AIProviderConfig, maxTurnsStr *string) formRunner {
				*maxTurnsStr = tt.maxTurnsStr
				return &mockFormRunner{}
			}

			cfg := &AIProviderConfig{}
			err := CollectAIConfigInteractive(context.Background(), cfg)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedTurns, cfg.MaxTurns)
		})
	}
}

func TestCollectAIConfigInteractive_FormError(t *testing.T) {
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	//nolint:err113 // Test-only error - static error not needed
	expectedErr := errors.New("form display error")
	createAIConfigForm = func(_ *AIProviderConfig, _ *string) formRunner {
		return &mockFormRunner{runErr: expectedErr}
	}

	cfg := &AIProviderConfig{}
	err := CollectAIConfigInteractive(context.Background(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "AI configuration failed")
}

func TestCollectAIConfigInteractive_PreservesExistingValues(t *testing.T) {
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	createAIConfigForm = func(_ *AIProviderConfig, _ *string) formRunner {
		// Don't modify config - form just runs
		return &mockFormRunner{}
	}

	// Start with pre-populated config
	cfg := &AIProviderConfig{
		Model:        "sonnet",
		APIKeyEnvVar: "ANTHROPIC_API_KEY",
		Timeout:      "5m",
		MaxTurns:     15,
	}

	err := CollectAIConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	// Values should be preserved (or defaults applied)
	assert.NotEmpty(t, cfg.Model)
	assert.NotEmpty(t, cfg.APIKeyEnvVar)
}

func TestCollectAIConfigInteractive_ContextCancellation(t *testing.T) {
	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := &AIProviderConfig{}
	err := CollectAIConfigInteractive(ctx, cfg)

	// Should return context.Canceled error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCollectAIConfigInteractive_InitializesDefaults(t *testing.T) {
	originalFactory := createAIConfigForm
	defer func() { createAIConfigForm = originalFactory }()

	var capturedConfig *AIProviderConfig
	createAIConfigForm = func(cfg *AIProviderConfig, _ *string) formRunner {
		capturedConfig = cfg
		return &mockFormRunner{}
	}

	// Start with empty config
	cfg := &AIProviderConfig{}
	err := CollectAIConfigInteractive(context.Background(), cfg)

	require.NoError(t, err)
	// Defaults should have been applied before form ran
	assert.Equal(t, "sonnet", capturedConfig.Model)
	assert.Equal(t, "ANTHROPIC_API_KEY", capturedConfig.APIKeyEnvVar)
	assert.Equal(t, "30m", capturedConfig.Timeout)
	assert.Equal(t, 10, capturedConfig.MaxTurns)
}
