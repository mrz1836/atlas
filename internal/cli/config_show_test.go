package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigShowCmd(t *testing.T) {
	t.Parallel()

	flags := &ConfigShowFlags{}
	cmd := newConfigShowCmd(flags)

	assert.Equal(t, "show", cmd.Use)
	assert.Contains(t, cmd.Short, "Display effective configuration")
	assert.Contains(t, cmd.Long, "source annotations")

	// Verify --output flag exists
	outputFlag := cmd.Flags().Lookup("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "yaml", outputFlag.DefValue)
}

func TestRunConfigShow_DefaultFormat(t *testing.T) {
	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	var buf bytes.Buffer
	flags := &ConfigShowFlags{OutputFormat: "yaml"}

	err = runConfigShow(context.Background(), &buf, flags)
	require.NoError(t, err)

	output := buf.String()

	// Verify YAML output contains expected sections
	assert.Contains(t, output, "Effective ATLAS Configuration")
	assert.Contains(t, output, "ai:")
	assert.Contains(t, output, "git:")
	assert.Contains(t, output, "model")
	assert.Contains(t, output, "# default")
}

func TestRunConfigShow_JSONFormat(t *testing.T) {
	// Change to a temp directory with no config files
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	var buf bytes.Buffer
	flags := &ConfigShowFlags{OutputFormat: "json"}

	err = runConfigShow(context.Background(), &buf, flags)
	require.NoError(t, err)

	output := buf.String()

	// Verify JSON output
	assert.Contains(t, output, `"ai"`)
	assert.Contains(t, output, `"source"`)
	assert.Contains(t, output, `"value"`)
}

func TestRunConfigShow_UnsupportedFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	flags := &ConfigShowFlags{OutputFormat: "xml"}

	err := runConfigShow(context.Background(), &buf, flags)
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrUnsupportedOutputFormat)
}

func TestRunConfigShow_ContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var buf bytes.Buffer
	flags := &ConfigShowFlags{OutputFormat: "yaml"}

	err := runConfigShow(ctx, &buf, flags)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestRunConfigShow_WithProjectConfig(t *testing.T) {
	// Create a fake git repo with project config
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	require.NoError(t, err)

	// Create project config
	atlasDir := filepath.Join(tmpDir, constants.AtlasHome)
	err = os.MkdirAll(atlasDir, 0o700)
	require.NoError(t, err)

	configPath := filepath.Join(atlasDir, constants.GlobalConfigName)
	err = os.WriteFile(configPath, []byte(`
ai:
  model: opus
git:
  base_branch: develop
`), 0o600)
	require.NoError(t, err)

	// Change to temp directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	var buf bytes.Buffer
	flags := &ConfigShowFlags{OutputFormat: "yaml"}

	err = runConfigShow(context.Background(), &buf, flags)
	require.NoError(t, err)

	output := buf.String()

	// Verify project config values are shown
	assert.Contains(t, output, "opus")
	assert.Contains(t, output, "develop")
	assert.Contains(t, output, "# project")
}

func TestFormatConfigValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"empty string", "", "(not set)"},
		{"non-empty string", "hello", "hello"},
		{"empty slice", []string{}, "[]"},
		{"string slice", []string{"a", "b", "c"}, "[a, b, c]"},
		{"empty interface slice", []interface{}{}, "[]"},
		{"interface slice", []interface{}{"x", 1, true}, "[x, 1, true]"},
		{"integer", 42, "42"},
		{"boolean true", true, "true"},
		{"boolean false", false, "false"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := formatConfigValue(tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMaskSensitiveValue(t *testing.T) {
	t.Parallel()

	styles := newConfigShowStyles()

	tests := []struct {
		name       string
		key        string
		value      string
		source     ConfigSource
		shouldMask bool
	}{
		{"non-sensitive key", "model", "opus", SourceEnv, false},
		{"api_key from env", "api_key", "secret123", SourceEnv, true},
		{"api_key_env_var from env", "api_key_env_var", "ANTHROPIC_API_KEY", SourceEnv, false},
		{"token from env", "token", "tok123", SourceEnv, true},
		{"password from project", "password", "pass123", SourceProject, false},
		{"not set", "api_key", "(not set)", SourceEnv, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := maskSensitiveValue(tc.key, tc.value, tc.source, styles)
			if tc.shouldMask {
				assert.Contains(t, result, "****")
			} else {
				assert.Equal(t, tc.value, result)
			}
		})
	}
}

func TestGetSourceStyle(t *testing.T) {
	t.Parallel()

	styles := newConfigShowStyles()

	tests := []struct {
		source   ConfigSource
		expected string
	}{
		{SourceEnv, "env"},
		{SourceProject, "project"},
		{SourceGlobal, "global"},
		{SourceDefault, "default"},
	}

	for _, tc := range tests {
		t.Run(string(tc.source), func(t *testing.T) {
			t.Parallel()
			style := getSourceStyle(tc.source, styles)
			// Verify style is not empty/nil by rendering something
			rendered := style.Render("test")
			assert.NotEmpty(t, rendered)
		})
	}
}

func TestDetermineSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        string
		value      interface{}
		globalCfg  configValues
		projectCfg configValues
		envSet     bool
		expected   ConfigSource
	}{
		{
			name:       "default when nothing set",
			key:        "ai.model",
			value:      "sonnet",
			globalCfg:  nil,
			projectCfg: nil,
			expected:   SourceDefault,
		},
		{
			name:       "global when in global config",
			key:        "ai.model",
			value:      "opus",
			globalCfg:  configValues{"ai.model": "opus"},
			projectCfg: nil,
			expected:   SourceGlobal,
		},
		{
			name:       "project overrides global",
			key:        "ai.model",
			value:      "haiku",
			globalCfg:  configValues{"ai.model": "opus"},
			projectCfg: configValues{"ai.model": "haiku"},
			expected:   SourceProject,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := determineSource(tc.key, tc.value, tc.globalCfg, tc.projectCfg, nil)
			assert.Equal(t, tc.expected, result.Source)
			assert.Equal(t, tc.value, result.Value)
		})
	}
}

func TestDetermineSource_EnvOverridesAll(t *testing.T) {
	// Set env var
	t.Setenv("ATLAS_AI_MODEL", "env-model")

	globalCfg := configValues{"ai.model": "opus"}
	projectCfg := configValues{"ai.model": "haiku"}

	result := determineSource("ai.model", "env-model", globalCfg, projectCfg, nil)

	assert.Equal(t, SourceEnv, result.Source)
	assert.Equal(t, "env-model", result.Value)
}

func TestConfigShowStyles(t *testing.T) {
	t.Parallel()

	styles := newConfigShowStyles()

	// Verify all styles are initialized (non-empty render)
	assert.NotEmpty(t, styles.header.Render("test"))
	assert.NotEmpty(t, styles.section.Render("test"))
	assert.NotEmpty(t, styles.key.Render("test"))
	assert.NotEmpty(t, styles.value.Render("test"))
	assert.NotEmpty(t, styles.sourceEnv.Render("test"))
	assert.NotEmpty(t, styles.sourcePrj.Render("test"))
	assert.NotEmpty(t, styles.sourceGbl.Render("test"))
	assert.NotEmpty(t, styles.sourceDef.Render("test"))
	assert.NotEmpty(t, styles.masked.Render("test"))
	assert.NotEmpty(t, styles.dim.Render("test"))
}

func TestPrintConfigValue(t *testing.T) {
	t.Parallel()

	styles := newConfigShowStyles()
	var buf bytes.Buffer

	vs := ConfigValueWithSource{
		Value:  "sonnet",
		Source: SourceDefault,
	}

	printConfigValue(&buf, styles, "model", vs)

	output := buf.String()
	assert.Contains(t, output, "model")
	assert.Contains(t, output, "sonnet")
	assert.Contains(t, output, "# default")
}
