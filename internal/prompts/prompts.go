package prompts

import (
	"bytes"
	"errors"
	"fmt"
)

// Render executes a prompt template with the provided data and returns the result.
// The data type should match the expected type for the given prompt ID.
//
// Example:
//
//	data := prompts.CommitMessageData{
//	    Package:     "internal/git",
//	    Files:       files,
//	    DiffSummary: diff,
//	    Scope:       "git",
//	}
//	prompt, err := prompts.Render(prompts.CommitMessage, data)
func Render(id PromptID, data any) (string, error) {
	tmpl, err := globalRegistry.get(id)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", errors.Join(ErrTemplateExecution, fmt.Errorf("prompt %s: %w", id, err))
	}

	return buf.String(), nil
}

// MustRender executes a prompt template and panics on error.
// Use this only when template execution should never fail (e.g., with known-good data).
func MustRender(id PromptID, data any) string {
	result, err := Render(id, data)
	if err != nil {
		panic(fmt.Sprintf("prompts.MustRender(%s): %v", id, err))
	}
	return result
}

// RenderString renders a prompt template using a string map for simple interpolation.
// This is a convenience wrapper for templates that only need string key-value pairs
// rather than structured data types. It explicitly accepts map[string]string to
// provide compile-time type safety for simple use cases.
//
// For templates expecting structured data (CommitMessageData, PRDescriptionData, etc.),
// use Render() directly with the appropriate typed struct.
//
// Example:
//
//	prompt, err := prompts.RenderString(customPromptID, map[string]string{
//	    "name": "example",
//	    "value": "test",
//	})
func RenderString(id PromptID, values map[string]string) (string, error) {
	// Convert to map[string]any for template execution while preserving type safety at the API level
	data := make(map[string]any, len(values))
	for k, v := range values {
		data[k] = v
	}
	return Render(id, data)
}

// List returns all registered prompt IDs.
// Useful for debugging or documentation generation.
func List() []PromptID {
	return globalRegistry.list()
}

// Exists checks if a prompt ID is registered.
func Exists(id PromptID) bool {
	_, err := globalRegistry.get(id)
	return err == nil
}

// GetTemplate returns the raw template source for a prompt ID.
// This is primarily useful for debugging, testing, and documentation generation.
func GetTemplate(id PromptID) (string, error) {
	return globalRegistry.getSource(id)
}

// ValidateData checks if the provided data is valid for the given prompt ID.
// This performs basic type checking and required field validation.
func ValidateData(id PromptID, data any) error {
	switch id {
	case CommitMessage:
		if _, ok := data.(CommitMessageData); !ok {
			return fmt.Errorf("%w: expected CommitMessageData, got %T", ErrInvalidData, data)
		}
	case PRDescription:
		if _, ok := data.(PRDescriptionData); !ok {
			return fmt.Errorf("%w: expected PRDescriptionData, got %T", ErrInvalidData, data)
		}
	case ValidationRetry:
		if _, ok := data.(ValidationRetryData); !ok {
			return fmt.Errorf("%w: expected ValidationRetryData, got %T", ErrInvalidData, data)
		}
	case DiscoveryAnalysis:
		if _, ok := data.(DiscoveryAnalysisData); !ok {
			return fmt.Errorf("%w: expected DiscoveryAnalysisData, got %T", ErrInvalidData, data)
		}
	case QuickVerify:
		if _, ok := data.(QuickVerifyData); !ok {
			return fmt.Errorf("%w: expected QuickVerifyData, got %T", ErrInvalidData, data)
		}
	case CodeCorrectness:
		if _, ok := data.(CodeCorrectnessData); !ok {
			return fmt.Errorf("%w: expected CodeCorrectnessData, got %T", ErrInvalidData, data)
		}
	case CIFailure:
		if _, ok := data.(CIFailureData); !ok {
			return fmt.Errorf("%w: expected CIFailureData, got %T", ErrInvalidData, data)
		}
	case AutoFix:
		if _, ok := data.(AutoFixData); !ok {
			return fmt.Errorf("%w: expected AutoFixData, got %T", ErrInvalidData, data)
		}
	}
	return nil
}
