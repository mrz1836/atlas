// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigValidationCmd(t *testing.T) {
	flags := &ConfigValidationFlags{}
	cmd := newConfigValidationCmd(flags)

	assert.Equal(t, "validation", cmd.Use)
	assert.Contains(t, cmd.Short, "validation")
	assert.NotNil(t, cmd.RunE)
}

func TestConfigValidationFlags(t *testing.T) {
	flags := &ConfigValidationFlags{}
	cmd := newConfigValidationCmd(flags)

	// Test that flag exists
	noInteractiveFlag := cmd.Flags().Lookup("no-interactive")
	require.NotNil(t, noInteractiveFlag)
	assert.Equal(t, "false", noInteractiveFlag.DefValue)
}

func TestRunConfigValidation_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	flags := &ConfigValidationFlags{}

	err := runConfigValidation(ctx, &buf, flags)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunConfigValidation_NonInteractive_NoConfig(t *testing.T) {
	// Use temp HOME to isolate from real config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	ctx := context.Background()
	var buf bytes.Buffer
	flags := &ConfigValidationFlags{NoInteractive: true}

	// This will not error even without config - it just displays a message
	err := runConfigValidation(ctx, &buf, flags)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No existing configuration found")
}

func TestDisplayCommandCategory(t *testing.T) {
	var buf bytes.Buffer
	styles := newConfigValidationStyles()

	// Test with commands
	displayCommandCategory(&buf, "Test Commands", []string{"go test ./..."}, styles)
	output := buf.String()
	assert.Contains(t, output, "Test Commands")
	assert.Contains(t, output, "go test ./...")

	// Test with empty commands
	buf.Reset()
	displayCommandCategory(&buf, "Pre-commit", nil, styles)
	output = buf.String()
	assert.Contains(t, output, "Pre-commit")
	assert.Contains(t, output, "(none configured)")
}

func TestAddConfigValidationCommand(t *testing.T) {
	// Create a parent command
	parentCmd := newConfigCmd()

	// Check that validation subcommand was added
	validationCmd, _, err := parentCmd.Find([]string{"validation"})
	require.NoError(t, err)
	assert.Equal(t, "validation", validationCmd.Use)
}

func TestDisplayCurrentValidationConfig(t *testing.T) {
	var buf bytes.Buffer
	styles := newConfigValidationStyles()

	cfg := &AtlasConfig{
		Validation: ValidationConfig{
			Commands: ValidationCommands{
				Format:      []string{"magex format:fix"},
				Lint:        []string{"magex lint"},
				Test:        []string{"magex test"},
				PreCommit:   []string{"go-pre-commit run --all-files"},
				CustomPrePR: []string{"custom-hook"},
			},
			TemplateOverrides: map[string]TemplateOverrideConfig{
				"bugfix": {SkipTest: true, SkipLint: false},
			},
		},
	}

	displayCurrentValidationConfig(&buf, cfg, styles)
	output := buf.String()

	assert.Contains(t, output, "Current Validation Configuration")
	assert.Contains(t, output, "Format Commands")
	assert.Contains(t, output, "Lint Commands")
	assert.Contains(t, output, "Test Commands")
	assert.Contains(t, output, "Pre-commit Commands")
	assert.Contains(t, output, "Custom Pre-PR Hooks")
	assert.Contains(t, output, "Template Overrides")
	assert.Contains(t, output, "bugfix")
}

func TestNewConfigValidationStyles(t *testing.T) {
	styles := newConfigValidationStyles()

	assert.NotNil(t, styles.header)
	assert.NotNil(t, styles.success)
	assert.NotNil(t, styles.warning)
	assert.NotNil(t, styles.dim)
	assert.NotNil(t, styles.key)
	assert.NotNil(t, styles.value)
}

func TestRunToolDetection(t *testing.T) {
	ctx := context.Background()

	// Run tool detection - should not error
	result, err := runToolDetection(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Tools)
}
