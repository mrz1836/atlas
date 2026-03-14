package ai

import (
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// RequestOption is a functional option for configuring an AIRequest.
type RequestOption func(*domain.AIRequest)

// NewAIRequest creates a new AIRequest with the given prompt and optional configuration.
// Default values are applied for unspecified options.
//
// Example:
//
//	req := NewAIRequest("Fix the bug",
//	    WithModel("sonnet"),
//	    WithTimeout(10*time.Minute),
//	    WithPermissionMode("plan"),
//	)
func NewAIRequest(prompt string, opts ...RequestOption) *domain.AIRequest {
	req := &domain.AIRequest{
		Prompt:  prompt,
		Timeout: constants.DefaultAITimeout,
	}

	for _, opt := range opts {
		opt(req)
	}

	return req
}

// WithModel sets the AI model to use.
// Examples: "sonnet", "opus", "haiku"
func WithModel(model string) RequestOption {
	return func(req *domain.AIRequest) {
		req.Model = model
	}
}

// WithTimeout sets the maximum duration for the AI session.
// If not specified, defaults to constants.DefaultAITimeout (30 minutes).
func WithTimeout(timeout time.Duration) RequestOption {
	return func(req *domain.AIRequest) {
		req.Timeout = timeout
	}
}

// WithPermissionMode sets the AI permission mode.
// Valid values: "acceptEdits", "bypassPermissions", "default", "delegate", "dontAsk", "plan"
// Use "plan" for read-only analysis mode.
func WithPermissionMode(mode string) RequestOption {
	return func(req *domain.AIRequest) {
		req.PermissionMode = mode
	}
}

// WithSystemPrompt sets an additional system prompt to append to the default.
// This is useful for providing project-specific context to the AI.
func WithSystemPrompt(prompt string) RequestOption {
	return func(req *domain.AIRequest) {
		req.SystemPrompt = prompt
	}
}

// WithWorkingDir sets the working directory for the AI session.
// The AI will operate within this directory for file operations.
func WithWorkingDir(dir string) RequestOption {
	return func(req *domain.AIRequest) {
		req.WorkingDir = dir
	}
}

// WithContext sets additional context for the AI session.
// This is useful for providing background information about the task.
func WithContext(ctx string) RequestOption {
	return func(req *domain.AIRequest) {
		req.Context = ctx
	}
}

// WithMaxTurns sets the maximum number of conversation turns.
// The Claude CLI may not directly support this flag; included for future compatibility.
func WithMaxTurns(turns int) RequestOption {
	return func(req *domain.AIRequest) {
		req.MaxTurns = turns
	}
}
