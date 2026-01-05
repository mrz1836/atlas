package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// FileTemplate represents the YAML/JSON structure for custom templates.
// Field names use both yaml and json tags for dual format support.
type FileTemplate struct {
	Name               string                          `yaml:"name" json:"name"`
	Description        string                          `yaml:"description" json:"description"`
	BranchPrefix       string                          `yaml:"branch_prefix" json:"branch_prefix"`
	DefaultAgent       string                          `yaml:"default_agent,omitempty" json:"default_agent,omitempty"`
	DefaultModel       string                          `yaml:"default_model,omitempty" json:"default_model,omitempty"`
	Steps              []FileStepDefinition            `yaml:"steps" json:"steps"`
	ValidationCommands []string                        `yaml:"validation_commands,omitempty" json:"validation_commands,omitempty"`
	Variables          map[string]FileTemplateVariable `yaml:"variables,omitempty" json:"variables,omitempty"`
	Verify             bool                            `yaml:"verify,omitempty" json:"verify,omitempty"`
	VerifyModel        string                          `yaml:"verify_model,omitempty" json:"verify_model,omitempty"`
}

// FileStepDefinition represents a step in the YAML/JSON file.
type FileStepDefinition struct {
	Name        string         `yaml:"name" json:"name"`
	Type        string         `yaml:"type" json:"type"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool           `yaml:"required" json:"required"`
	Timeout     string         `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	RetryCount  int            `yaml:"retry_count,omitempty" json:"retry_count,omitempty"`
	Config      map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// FileTemplateVariable represents a variable in the YAML/JSON file.
type FileTemplateVariable struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
	Required    bool   `yaml:"required" json:"required"`
}

// Loader loads templates from files.
type Loader struct {
	basePath string
}

// NewLoader creates a new template loader.
// basePath is used to resolve relative template paths (typically project root).
func NewLoader(basePath string) *Loader {
	return &Loader{basePath: basePath}
}

// LoadFromFile loads a template from a YAML or JSON file.
// The format is auto-detected based on file extension (.json for JSON, otherwise YAML).
// Returns an error if the file cannot be read, parsed, or validated.
func (l *Loader) LoadFromFile(path string) (*domain.Template, error) {
	// Resolve path (absolute or relative to basePath)
	resolvedPath := l.resolvePath(path)

	// Read file
	data, err := os.ReadFile(resolvedPath) //nolint:gosec // Path is resolved from user config
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", atlaserrors.ErrTemplateFileMissing, resolvedPath)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: permission denied: %s", atlaserrors.ErrTemplateLoadFailed, resolvedPath)
		}
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrTemplateLoadFailed, err)
	}

	// Parse file based on format
	var fileTemplate FileTemplate
	format := l.detectFormat(path)

	if format == "json" {
		if parseErr := json.Unmarshal(data, &fileTemplate); parseErr != nil {
			return nil, fmt.Errorf("%w: %w", atlaserrors.ErrTemplateParseError, parseErr)
		}
	} else {
		if parseErr := yaml.Unmarshal(data, &fileTemplate); parseErr != nil {
			return nil, fmt.Errorf("%w: %w", atlaserrors.ErrTemplateParseError, parseErr)
		}
	}

	// Convert to domain.Template
	tmpl, convertErr := toTemplate(&fileTemplate)
	if convertErr != nil {
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrTemplateLoadFailed, convertErr)
	}

	// Validate the template
	if err := ValidateTemplate(tmpl); err != nil {
		return nil, err
	}

	return tmpl, nil
}

// LoadAll loads multiple templates from name->path mappings.
// The configName (map key) is used as the template name if different from the file's name field.
// Returns an error on the first failure (fail-fast behavior).
func (l *Loader) LoadAll(templates map[string]string) ([]*domain.Template, error) {
	if len(templates) == 0 {
		return nil, nil
	}

	loaded := make([]*domain.Template, 0, len(templates))

	for configName, path := range templates {
		tmpl, err := l.LoadFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("template %q from %q: %w", configName, path, err)
		}

		// Override name with config key (config takes precedence)
		// This allows users to rename templates via config
		tmpl.Name = configName

		loaded = append(loaded, tmpl)
	}

	return loaded, nil
}

// resolvePath resolves a template path, supporting both absolute and relative paths.
// Relative paths are resolved relative to the loader's basePath.
func (l *Loader) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.basePath, path)
}

// detectFormat returns the file format based on extension.
// Returns "json" for .json files, "yaml" for everything else.
func (l *Loader) detectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		return "json"
	}
	return "yaml"
}

// toTemplate converts a FileTemplate to a domain.Template.
func toTemplate(f *FileTemplate) (*domain.Template, error) {
	t := &domain.Template{
		Name:               f.Name,
		Description:        f.Description,
		BranchPrefix:       f.BranchPrefix,
		DefaultAgent:       domain.Agent(f.DefaultAgent),
		DefaultModel:       f.DefaultModel,
		ValidationCommands: f.ValidationCommands,
		Verify:             f.Verify,
		VerifyModel:        f.VerifyModel,
	}

	// Convert steps
	t.Steps = make([]domain.StepDefinition, len(f.Steps))
	for i, fs := range f.Steps {
		step, err := toStepDefinition(&fs)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s): %w", i, fs.Name, err)
		}
		t.Steps[i] = step
	}

	// Convert variables
	if f.Variables != nil {
		t.Variables = make(map[string]domain.TemplateVariable, len(f.Variables))
		for k, v := range f.Variables {
			t.Variables[k] = domain.TemplateVariable{
				Description: v.Description,
				Default:     v.Default,
				Required:    v.Required,
			}
		}
	}

	return t, nil
}

// toStepDefinition converts a FileStepDefinition to a domain.StepDefinition.
func toStepDefinition(f *FileStepDefinition) (domain.StepDefinition, error) {
	step := domain.StepDefinition{
		Name:        f.Name,
		Description: f.Description,
		Required:    f.Required,
		RetryCount:  f.RetryCount,
		Config:      f.Config,
	}

	// Parse step type (case-insensitive)
	stepType, err := ParseStepType(f.Type)
	if err != nil {
		return step, err
	}
	step.Type = stepType

	// Parse timeout if provided
	if f.Timeout != "" {
		timeout, err := time.ParseDuration(f.Timeout)
		if err != nil {
			return step, fmt.Errorf("invalid timeout %q: %w", f.Timeout, err)
		}
		step.Timeout = timeout
	}

	return step, nil
}
