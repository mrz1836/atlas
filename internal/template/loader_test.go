package template

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

const validYAMLTemplate = `
name: test-template
description: A test template for loading
branch_prefix: test
default_model: sonnet
verify: true
verify_model: opus

steps:
  - name: implement
    type: ai
    description: Implement the changes
    required: true
    timeout: 30m
    retry_count: 3
    config:
      permission_mode: default

  - name: validate
    type: validation
    required: true
    timeout: 10m

validation_commands:
  - make lint
  - make test

variables:
  ticket_id:
    description: JIRA ticket ID
    required: true
  component:
    description: Component name
    default: core
    required: false
`

const validJSONTemplate = `{
  "name": "json-template",
  "description": "A JSON test template",
  "branch_prefix": "json",
  "default_model": "haiku",
  "steps": [
    {
      "name": "implement",
      "type": "ai",
      "required": true,
      "timeout": "15m"
    }
  ]
}`

const minimalYAMLTemplate = `
name: minimal
steps:
  - name: step1
    type: ai
    required: true
`

func TestLoader_LoadFromFile_YAML_Success(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(validYAMLTemplate), 0o600))

	// Load template
	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.yaml")

	require.NoError(t, err)
	require.NotNil(t, tmpl)

	// Verify fields
	assert.Equal(t, "test-template", tmpl.Name)
	assert.Equal(t, "A test template for loading", tmpl.Description)
	assert.Equal(t, "test", tmpl.BranchPrefix)
	assert.Equal(t, "sonnet", tmpl.DefaultModel)
	assert.True(t, tmpl.Verify)
	assert.Equal(t, "opus", tmpl.VerifyModel)

	// Verify steps
	require.Len(t, tmpl.Steps, 2)
	assert.Equal(t, "implement", tmpl.Steps[0].Name)
	assert.Equal(t, domain.StepTypeAI, tmpl.Steps[0].Type)
	assert.True(t, tmpl.Steps[0].Required)
	assert.Equal(t, 30*time.Minute, tmpl.Steps[0].Timeout)
	assert.Equal(t, 3, tmpl.Steps[0].RetryCount)
	assert.Equal(t, "default", tmpl.Steps[0].Config["permission_mode"])

	assert.Equal(t, "validate", tmpl.Steps[1].Name)
	assert.Equal(t, domain.StepTypeValidation, tmpl.Steps[1].Type)

	// Verify validation commands
	assert.Equal(t, []string{"make lint", "make test"}, tmpl.ValidationCommands)

	// Verify variables
	require.Len(t, tmpl.Variables, 2)
	assert.True(t, tmpl.Variables["ticket_id"].Required)
	assert.Equal(t, "core", tmpl.Variables["component"].Default)
}

func TestLoader_LoadFromFile_JSON_Success(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte(validJSONTemplate), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.json")

	require.NoError(t, err)
	require.NotNil(t, tmpl)

	assert.Equal(t, "json-template", tmpl.Name)
	assert.Equal(t, "A JSON test template", tmpl.Description)
	assert.Equal(t, "json", tmpl.BranchPrefix)
	assert.Equal(t, "haiku", tmpl.DefaultModel)

	require.Len(t, tmpl.Steps, 1)
	assert.Equal(t, "implement", tmpl.Steps[0].Name)
	assert.Equal(t, domain.StepTypeAI, tmpl.Steps[0].Type)
	assert.Equal(t, 15*time.Minute, tmpl.Steps[0].Timeout)
}

func TestLoader_LoadFromFile_FileNotFound(t *testing.T) {
	loader := NewLoader(t.TempDir())
	_, err := loader.LoadFromFile("nonexistent.yaml")

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateFileMissing)
}

func TestLoader_LoadFromFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte("invalid: yaml: content: ["), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("invalid.yaml")

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateParseError)
}

func TestLoader_LoadFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte("{invalid json}"), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("invalid.json")

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateParseError)
}

func TestLoader_LoadFromFile_MissingName(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "noname.yaml")
	content := `
steps:
  - name: step1
    type: ai
    required: true
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("noname.yaml")

	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestLoader_LoadFromFile_NoSteps(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nosteps.yaml")
	content := `
name: no-steps-template
description: Template without steps
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("nosteps.yaml")

	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
}

func TestLoader_LoadFromFile_InvalidStepType(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "badtype.yaml")
	content := `
name: bad-type-template
steps:
  - name: step1
    type: invalid_type
    required: true
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("badtype.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not valid")
}

func TestLoader_LoadFromFile_InvalidTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "badtimeout.yaml")
	content := `
name: bad-timeout-template
steps:
  - name: step1
    type: ai
    required: true
    timeout: thirty minutes
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("badtimeout.yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

func TestLoader_LoadFromFile_NegativeRetryCount(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "badretry.yaml")
	content := `
name: bad-retry-template
steps:
  - name: step1
    type: ai
    required: true
    retry_count: -5
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("badretry.yaml")

	require.ErrorIs(t, err, atlaserrors.ErrTemplateInvalid)
	assert.Contains(t, err.Error(), "retry_count cannot be negative")
}

func TestLoader_LoadFromFile_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "templates")
	require.NoError(t, os.MkdirAll(subDir, 0o750))
	tmpFile := filepath.Join(subDir, "template.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(minimalYAMLTemplate), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("templates/template.yaml")

	require.NoError(t, err)
	assert.Equal(t, "minimal", tmpl.Name)
}

func TestLoader_LoadFromFile_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(minimalYAMLTemplate), 0o600))

	// Use a different basePath to prove absolute path takes precedence
	loader := NewLoader("/some/other/path")
	tmpl, err := loader.LoadFromFile(tmpFile) // Absolute path

	require.NoError(t, err)
	assert.Equal(t, "minimal", tmpl.Name)
}

func TestLoader_LoadFromFile_UnknownExtension(t *testing.T) {
	tmpDir := t.TempDir()
	// File with unknown extension should be parsed as YAML
	tmpFile := filepath.Join(tmpDir, "template.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte(minimalYAMLTemplate), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.txt")

	require.NoError(t, err)
	assert.Equal(t, "minimal", tmpl.Name)
}

func TestLoader_LoadFromFile_YMLExtension(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(minimalYAMLTemplate), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.yml")

	require.NoError(t, err)
	assert.Equal(t, "minimal", tmpl.Name)
}

func TestLoader_LoadFromFile_CaseInsensitiveStepType(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yaml")
	content := `
name: case-test
steps:
  - name: step1
    type: AI
    required: true
  - name: step2
    type: VALIDATION
    required: true
  - name: step3
    type: Git
    required: true
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.yaml")

	require.NoError(t, err)
	assert.Equal(t, domain.StepTypeAI, tmpl.Steps[0].Type)
	assert.Equal(t, domain.StepTypeValidation, tmpl.Steps[1].Type)
	assert.Equal(t, domain.StepTypeGit, tmpl.Steps[2].Type)
}

func TestLoader_LoadFromFile_AllStepTypes(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yaml")
	content := `
name: all-types
steps:
  - name: step1
    type: ai
    required: true
  - name: step2
    type: validation
    required: true
  - name: step3
    type: git
    required: true
  - name: step4
    type: human
    required: true
  - name: step5
    type: sdd
    required: true
  - name: step6
    type: ci
    required: true
  - name: step7
    type: verify
    required: true
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	tmpl, err := loader.LoadFromFile("template.yaml")

	require.NoError(t, err)
	require.Len(t, tmpl.Steps, 7)

	expectedTypes := []domain.StepType{
		domain.StepTypeAI,
		domain.StepTypeValidation,
		domain.StepTypeGit,
		domain.StepTypeHuman,
		domain.StepTypeSDD,
		domain.StepTypeCI,
		domain.StepTypeVerify,
	}

	for i, expectedType := range expectedTypes {
		assert.Equal(t, expectedType, tmpl.Steps[i].Type, "step %d should have type %s", i, expectedType)
	}
}

func TestLoader_LoadAll_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple template files
	template1 := `
name: template1
steps:
  - name: step1
    type: ai
    required: true
`
	template2 := `
name: template2
steps:
  - name: step1
    type: validation
    required: true
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "t1.yaml"), []byte(template1), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "t2.yaml"), []byte(template2), 0o600))

	loader := NewLoader(tmpDir)
	templates, err := loader.LoadAll(map[string]string{
		"first":  "t1.yaml",
		"second": "t2.yaml",
	})

	require.NoError(t, err)
	require.Len(t, templates, 2)

	// Templates should have names from config keys, not file names
	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
	}
	assert.True(t, names["first"])
	assert.True(t, names["second"])
}

func TestLoader_LoadAll_FailFast(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one valid and one invalid template
	validTemplate := `
name: valid
steps:
  - name: step1
    type: ai
    required: true
`
	invalidTemplate := `
name: invalid
steps:
  - name: step1
    type: unknown_type
    required: true
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "valid.yaml"), []byte(validTemplate), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalidTemplate), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadAll(map[string]string{
		"valid":   "valid.yaml",
		"invalid": "invalid.yaml",
	})

	require.Error(t, err)
	// Should mention the template name and contain the underlying error
	assert.Contains(t, err.Error(), "is not valid")
}

func TestLoader_LoadAll_Empty(t *testing.T) {
	loader := NewLoader(t.TempDir())
	templates, err := loader.LoadAll(map[string]string{})

	require.NoError(t, err)
	assert.Nil(t, templates)
}

func TestLoader_LoadAll_Nil(t *testing.T) {
	loader := NewLoader(t.TempDir())
	templates, err := loader.LoadAll(nil)

	require.NoError(t, err)
	assert.Nil(t, templates)
}

func TestLoader_LoadFromFile_ConfigNameOverride(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "template.yaml")
	content := `
name: original-name
steps:
  - name: step1
    type: ai
    required: true
`
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	loader := NewLoader(tmpDir)
	templates, err := loader.LoadAll(map[string]string{
		"override-name": "template.yaml",
	})

	require.NoError(t, err)
	require.Len(t, templates, 1)
	// Config key should override the name from file
	assert.Equal(t, "override-name", templates[0].Name)
}

func TestLoader_LoadFromFile_PermissionDenied(t *testing.T) {
	// Skip on Windows where file permissions work differently
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "noperm.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(minimalYAMLTemplate), 0o600))

	// Remove read permission
	require.NoError(t, os.Chmod(tmpFile, 0o000))
	defer func() { _ = os.Chmod(tmpFile, 0o600) }() // Restore for cleanup

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("noperm.yaml")

	require.ErrorIs(t, err, atlaserrors.ErrTemplateLoadFailed)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestLoader_LoadFromFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(""), 0o600))

	loader := NewLoader(tmpDir)
	_, err := loader.LoadFromFile("empty.yaml")

	require.Error(t, err)
	// Empty file results in empty name
	assert.ErrorIs(t, err, atlaserrors.ErrTemplateNameEmpty)
}

func TestLoader_detectFormat(t *testing.T) {
	loader := NewLoader("")

	tests := []struct {
		path   string
		format string
	}{
		{"template.json", "json"},
		{"template.JSON", "json"},
		{"template.yaml", "yaml"},
		{"template.yml", "yaml"},
		{"template.YAML", "yaml"},
		{"template.txt", "yaml"},
		{"template", "yaml"},
		{"/path/to/template.json", "json"},
		{"/path/to/template.yaml", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := loader.detectFormat(tt.path)
			assert.Equal(t, tt.format, got)
		})
	}
}

func TestLoader_resolvePath(t *testing.T) {
	loader := NewLoader("/base/path")

	tests := []struct {
		input    string
		expected string
	}{
		{"relative/path.yaml", "/base/path/relative/path.yaml"},
		{"/absolute/path.yaml", "/absolute/path.yaml"},
		{"file.yaml", "/base/path/file.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := loader.resolvePath(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/test/path")
	assert.NotNil(t, loader)
	assert.Equal(t, "/test/path", loader.basePath)
}

func TestLoader_LoadFromFile_VariousTimeoutFormats(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		timeout  string
		expected time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"1h", time.Hour},
		{"1h30m", 90 * time.Minute},
		{"90s", 90 * time.Second},
		{"2h30m45s", 2*time.Hour + 30*time.Minute + 45*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.timeout, func(t *testing.T) {
			content := `
name: timeout-test
steps:
  - name: step1
    type: ai
    required: true
    timeout: ` + tt.timeout
			tmpFile := filepath.Join(tmpDir, "timeout.yaml")
			require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

			loader := NewLoader(tmpDir)
			tmpl, err := loader.LoadFromFile("timeout.yaml")

			require.NoError(t, err)
			assert.Equal(t, tt.expected, tmpl.Steps[0].Timeout)
		})
	}
}
