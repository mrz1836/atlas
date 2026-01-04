// Package template provides template loading, validation, and registry functionality.
package template

import (
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ValidStepTypes returns all valid step type values.
func ValidStepTypes() []domain.StepType {
	return []domain.StepType{
		domain.StepTypeAI,
		domain.StepTypeValidation,
		domain.StepTypeGit,
		domain.StepTypeHuman,
		domain.StepTypeSDD,
		domain.StepTypeCI,
		domain.StepTypeVerify,
	}
}

// ValidateTemplate validates a template has all required fields and valid values.
// Returns nil if the template is valid, otherwise returns a descriptive error.
func ValidateTemplate(t *domain.Template) error {
	if t == nil {
		return atlaserrors.ErrTemplateNil
	}

	if strings.TrimSpace(t.Name) == "" {
		return atlaserrors.ErrTemplateNameEmpty
	}

	if len(t.Steps) == 0 {
		return fmt.Errorf("%w: template must have at least one step", atlaserrors.ErrTemplateInvalid)
	}

	// Validate each step
	for i, step := range t.Steps {
		if err := validateStep(&step, i); err != nil {
			return err
		}
	}

	// Validate variables (if any)
	for name := range t.Variables {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: variable name cannot be empty", atlaserrors.ErrTemplateInvalid)
		}
	}

	return nil
}

// validateStep validates a step definition at the given index.
func validateStep(step *domain.StepDefinition, index int) error {
	if strings.TrimSpace(step.Name) == "" {
		return fmt.Errorf("%w: step %d: name is required", atlaserrors.ErrTemplateInvalid, index)
	}

	if !IsValidStepType(step.Type) {
		return fmt.Errorf("%w: step %d (%s): invalid type %q: must be one of: %s",
			atlaserrors.ErrTemplateInvalid, index, step.Name, step.Type, validStepTypesString())
	}

	if step.Timeout < 0 {
		return fmt.Errorf("%w: step %d (%s): timeout cannot be negative", atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	if step.RetryCount < 0 {
		return fmt.Errorf("%w: step %d (%s): retry_count cannot be negative", atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	return nil
}

// IsValidStepType checks if the step type is a known valid type.
func IsValidStepType(t domain.StepType) bool {
	for _, valid := range ValidStepTypes() {
		if t == valid {
			return true
		}
	}
	return false
}

// ParseStepType converts a string to a StepType with validation.
// The conversion is case-insensitive.
func ParseStepType(s string) (domain.StepType, error) {
	t := domain.StepType(strings.ToLower(strings.TrimSpace(s)))
	if !IsValidStepType(t) {
		return "", fmt.Errorf("%w: %q is not valid, must be one of: %s",
			atlaserrors.ErrTemplateInvalid, s, validStepTypesString())
	}
	return t, nil
}

// validStepTypesString returns a comma-separated list of valid step types.
func validStepTypesString() string {
	stepTypes := ValidStepTypes()
	types := make([]string, len(stepTypes))
	for i, t := range stepTypes {
		types[i] = string(t)
	}
	return strings.Join(types, ", ")
}
