package ai

import (
	"encoding/json"
	"fmt"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// GeminiError represents the error format from Gemini CLI.
// The error can be either a simple string or a structured object with type, message, and optional code.
type GeminiError struct {
	// Type is the error category (e.g., "ApiError", "AuthError", "Error").
	Type string `json:"type"`

	// Message is the human-readable error description.
	Message string `json:"message"`

	// Code is an optional numeric error identifier.
	Code int `json:"code,omitempty"`

	// RawString stores the error if it was provided as a plain string.
	RawString string `json:"-"`
}

// String returns a formatted error message.
func (e *GeminiError) String() string {
	// If we got a plain string error, return it
	if e.RawString != "" {
		return e.RawString
	}

	if e.Type != "" && e.Message != "" {
		if e.Code != 0 {
			return fmt.Sprintf("%s (code %d): %s", e.Type, e.Code, e.Message)
		}
		return fmt.Sprintf("%s: %s", e.Type, e.Message)
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Type != "" {
		return e.Type
	}
	return "unknown error"
}

// UnmarshalJSON handles both string and object error formats.
func (e *GeminiError) UnmarshalJSON(data []byte) error {
	// First try to unmarshal as a string
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		e.RawString = str
		return nil
	}

	// If not a string, try to unmarshal as an object
	type geminiErrorObj struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    int    `json:"code,omitempty"`
	}
	var obj geminiErrorObj
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	e.Type = obj.Type
	e.Message = obj.Message
	e.Code = obj.Code
	return nil
}

// GeminiResponse represents the JSON response from Gemini CLI.
// This struct matches the JSON output format when using --output-format json.
type GeminiResponse struct {
	// Success indicates whether the request completed successfully.
	Success bool `json:"success"`

	// Response contains the AI's text response (primary output field).
	Response string `json:"response"`

	// Content contains the AI's text response or output (alternative field).
	Content string `json:"content"`

	// Result is an alternative field for the response content.
	// Some versions may use "result" instead of "content".
	Result string `json:"result"`

	// SessionID identifies the AI session for debugging.
	SessionID string `json:"session_id"`

	// DurationMs is how long the AI session took in milliseconds.
	DurationMs int `json:"duration_ms"`

	// NumTurns is how many conversation turns occurred.
	NumTurns int `json:"num_turns"`

	// TotalCostUSD is the estimated cost of the AI session in USD.
	// May be 0 for free tier usage.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Error contains structured error information if the request failed.
	// This is an object with type, message, and optional code fields.
	Error *GeminiError `json:"error,omitempty"`
}

// parseGeminiResponse parses the JSON output from Gemini CLI.
// Returns an error wrapped with ErrGeminiInvocation on parse failure.
func parseGeminiResponse(data []byte) (*GeminiResponse, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty response", atlaserrors.ErrGeminiInvocation)
	}

	var resp GeminiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse json response: %s", atlaserrors.ErrGeminiInvocation, err.Error())
	}

	return &resp, nil
}

// toAIResult converts a GeminiResponse to a domain.AIResult.
// Maps Gemini-specific fields to the domain-agnostic AIResult structure.
func (r *GeminiResponse) toAIResult(stderr string) *domain.AIResult {
	// Use Response, Content, or Result, whichever is populated (in priority order)
	output := r.Response
	if output == "" {
		output = r.Content
	}
	if output == "" {
		output = r.Result
	}

	result := &domain.AIResult{
		Success:      r.Success,
		Output:       output,
		SessionID:    r.SessionID,
		DurationMs:   r.DurationMs,
		NumTurns:     r.NumTurns,
		TotalCostUSD: r.TotalCostUSD,
	}

	// Include error information if this is an error response
	if !r.Success {
		if r.Error != nil {
			result.Error = r.Error.String()
		} else if stderr != "" {
			result.Error = stderr
		}
	}

	return result
}
