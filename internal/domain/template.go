// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

import "time"

// StepType categorizes the kind of execution a step performs.
// This determines which executor handles the step.
type StepType string

// Step type constants define the valid execution types for steps.
const (
	// StepTypeAI indicates the step uses AI to generate or modify code.
	StepTypeAI StepType = "ai"

	// StepTypeValidation indicates the step runs validation commands.
	StepTypeValidation StepType = "validation"

	// StepTypeGit indicates the step performs git operations.
	StepTypeGit StepType = "git"

	// StepTypeHuman indicates the step requires human intervention.
	StepTypeHuman StepType = "human"

	// StepTypeSDD indicates the step uses Speckit SDD integration.
	StepTypeSDD StepType = "sdd"

	// StepTypeCI indicates the step monitors CI pipeline status.
	StepTypeCI StepType = "ci"
)

// String returns the string representation of the StepType.
// This implements fmt.Stringer for convenient logging and debugging.
func (s StepType) String() string {
	return string(s)
}

// Template defines a reusable task template that specifies the
// sequence of steps to execute. Templates are loaded from YAML
// configuration files.
//
// Example JSON representation:
//
//	{
//	    "name": "bugfix",
//	    "description": "Fix a reported bug",
//	    "branch_prefix": "fix/",
//	    "default_model": "claude-sonnet-4-20250514",
//	    "steps": [...],
//	    "validation_commands": ["magex lint", "magex test"]
//	}
type Template struct {
	// Name is the unique identifier for this template (e.g., "bugfix", "feature").
	Name string `json:"name"`

	// Description explains what this template is used for.
	Description string `json:"description"`

	// BranchPrefix is prepended to branch names created from this template.
	BranchPrefix string `json:"branch_prefix"`

	// DefaultModel is the AI model to use if not overridden.
	DefaultModel string `json:"default_model,omitempty"`

	// Steps defines the ordered sequence of step definitions.
	Steps []StepDefinition `json:"steps"`

	// ValidationCommands are run during the validation step.
	ValidationCommands []string `json:"validation_commands,omitempty"`

	// Variables defines template variables with optional defaults.
	Variables map[string]TemplateVariable `json:"variables,omitempty"`
}

// StepDefinition describes a step within a template.
// This is the blueprint from which Step instances are created.
//
// Example JSON representation:
//
//	{
//	    "name": "implement",
//	    "type": "ai",
//	    "description": "Implement the requested changes",
//	    "required": true,
//	    "timeout": "30m",
//	    "retry_count": 2
//	}
type StepDefinition struct {
	// Name identifies this step definition.
	Name string `json:"name"`

	// Type specifies the execution type (ai, validation, git, etc.).
	Type StepType `json:"type"`

	// Description explains what this step does.
	Description string `json:"description,omitempty"`

	// Required indicates whether this step can be skipped.
	Required bool `json:"required"`

	// Timeout is the maximum duration for this step.
	Timeout time.Duration `json:"timeout,omitempty"`

	// RetryCount is how many times to retry on failure.
	RetryCount int `json:"retry_count,omitempty"`

	// Config contains step-specific configuration.
	Config map[string]any `json:"config,omitempty"`
}

// TemplateVariable defines a variable that can be used in templates.
type TemplateVariable struct {
	// Description explains what this variable is used for.
	Description string `json:"description,omitempty"`

	// Default is the default value if not provided.
	Default string `json:"default,omitempty"`

	// Required indicates whether this variable must be provided.
	Required bool `json:"required"`
}
