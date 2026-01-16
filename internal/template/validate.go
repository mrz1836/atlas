// Package template provides template loading, validation, and registry functionality.
package template

import (
	"fmt"
	"slices"
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
		domain.StepTypeLoop,
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

	// Validate loop-specific configuration
	if step.Type == domain.StepTypeLoop {
		if err := validateLoopStep(step, index); err != nil {
			return err
		}
	}

	return nil
}

// validateLoopStep validates loop-specific configuration.
func validateLoopStep(step *domain.StepDefinition, index int) error {
	if step.Config == nil {
		return fmt.Errorf("%w: step %d (%s): loop step requires config",
			atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	stepsSlice, err := validateLoopInnerSteps(step, index)
	if err != nil {
		return err
	}

	if !hasLoopTerminationCondition(step.Config) {
		return fmt.Errorf("%w: step %d (%s): loop must have max_iterations, until, or until_signal",
			atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	return validateInnerStepsRecursively(stepsSlice, step, index)
}

// validateLoopInnerSteps checks that inner steps exist and are valid.
func validateLoopInnerSteps(step *domain.StepDefinition, index int) ([]any, error) {
	steps, hasSteps := step.Config["steps"]
	if !hasSteps {
		return nil, fmt.Errorf("%w: step %d (%s): loop step must have inner steps",
			atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	stepsSlice, isSlice := steps.([]any)
	if !isSlice || len(stepsSlice) == 0 {
		return nil, fmt.Errorf("%w: step %d (%s): loop step must have at least one inner step",
			atlaserrors.ErrTemplateInvalid, index, step.Name)
	}

	return stepsSlice, nil
}

// hasLoopTerminationCondition checks if any termination condition is set.
func hasLoopTerminationCondition(config map[string]any) bool {
	if hasPositiveInt(config, "max_iterations") {
		return true
	}
	if hasNonEmptyString(config, "until") {
		return true
	}
	if hasTrueBool(config, "until_signal") {
		return true
	}
	return false
}

// hasPositiveInt checks if a config key has a positive integer value.
func hasPositiveInt(config map[string]any, key string) bool {
	v, ok := config[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case int:
		return val > 0
	case float64:
		return val > 0
	}
	return false
}

// hasNonEmptyString checks if a config key has a non-empty string value.
func hasNonEmptyString(config map[string]any, key string) bool {
	v, ok := config[key]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s != ""
}

// hasTrueBool checks if a config key has a true boolean value.
func hasTrueBool(config map[string]any, key string) bool {
	v, ok := config[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// validateInnerStepsRecursively validates each inner step.
func validateInnerStepsRecursively(stepsSlice []any, step *domain.StepDefinition, index int) error {
	for i, innerStep := range stepsSlice {
		innerMap, ok := innerStep.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: step %d (%s): inner step %d has invalid format",
				atlaserrors.ErrTemplateInvalid, index, step.Name, i)
		}

		innerDef := parseInnerStepDefinition(innerMap)
		if err := validateStep(&innerDef, i); err != nil {
			return fmt.Errorf("%w: step %d (%s) inner step %d: %w",
				atlaserrors.ErrTemplateInvalid, index, step.Name, i, err)
		}
	}
	return nil
}

// parseInnerStepDefinition converts a map to a StepDefinition for validation.
func parseInnerStepDefinition(m map[string]any) domain.StepDefinition {
	step := domain.StepDefinition{}

	if v, ok := m["name"].(string); ok {
		step.Name = v
	}
	if v, ok := m["type"].(string); ok {
		step.Type = domain.StepType(v)
	}
	if v, ok := m["description"].(string); ok {
		step.Description = v
	}
	if v, ok := m["required"].(bool); ok {
		step.Required = v
	}
	if v, ok := m["config"].(map[string]any); ok {
		step.Config = v
	}

	return step
}

// IsValidStepType checks if the step type is a known valid type.
func IsValidStepType(t domain.StepType) bool {
	return slices.Contains(ValidStepTypes(), t)
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
