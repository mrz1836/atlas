// Package prompts provides centralized AI prompt management for ATLAS.
// All AI prompts are stored as text/template files and embedded at compile time.
package prompts

import "errors"

// Package errors for prompt management.
var (
	// ErrTemplateNotFound indicates the requested template doesn't exist.
	ErrTemplateNotFound = errors.New("template not found")

	// ErrTemplateExecution indicates a failure during template execution.
	ErrTemplateExecution = errors.New("template execution failed")

	// ErrInvalidData indicates the provided data doesn't match expected type.
	ErrInvalidData = errors.New("invalid data type for template")
)
