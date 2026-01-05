package tui

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

// JSONOutput provides structured JSON output for non-TTY environments (AC: #4).
// All messages are output as structured JSON objects.
type JSONOutput struct {
	w       io.Writer
	encoder *json.Encoder
}

// NewJSONOutput creates a new JSONOutput.
func NewJSONOutput(w io.Writer) *JSONOutput {
	return &JSONOutput{
		w:       w,
		encoder: json.NewEncoder(w),
	}
}

// jsonMessage is the structured format for Success/Warning/Info messages (AC: #4).
type jsonMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// jsonError is the structured format for Error messages (AC: #4).
type jsonError struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Context    string `json:"context,omitempty"`
}

// Success outputs a success message as JSON (AC: #4).
// Format: {"type": "success", "message": "..."}
func (o *JSONOutput) Success(msg string) {
	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(jsonMessage{
		Type:    "success",
		Message: msg,
	})
}

// Error outputs an error as JSON with details (AC: #4).
// Format: {"type": "error", "message": "...", "details": "...", "suggestion": "...", "context": "..."}
// Details field is populated with the wrapped error's message if present.
// If the error is an ActionableError, suggestion and context fields are included.
func (o *JSONOutput) Error(err error) {
	jsonErr := jsonError{
		Type:    "error",
		Message: err.Error(),
	}

	// Check for ActionableError and extract suggestion/context
	var ae *ActionableError
	if errors.As(err, &ae) {
		if ae.Suggestion != "" {
			jsonErr.Suggestion = ae.Suggestion
		}
		if ae.Context != "" {
			jsonErr.Context = ae.Context
		}
	}

	// Extract details from wrapped error if present (AC: #4)
	var wrapped error
	if errors.Unwrap(err) != nil {
		wrapped = errors.Unwrap(err)
		jsonErr.Details = wrapped.Error()
	}

	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(jsonErr)
}

// Warning outputs a warning message as JSON (AC: #4).
// Format: {"type": "warning", "message": "..."}
func (o *JSONOutput) Warning(msg string) {
	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(jsonMessage{
		Type:    "warning",
		Message: msg,
	})
}

// Info outputs an informational message as JSON (AC: #4).
// Format: {"type": "info", "message": "..."}
func (o *JSONOutput) Info(msg string) {
	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(jsonMessage{
		Type:    "info",
		Message: msg,
	})
}

// Table outputs tabular data as an array of objects (AC: #5).
// Format: [{"col1": "val1", ...}, ...]
func (o *JSONOutput) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		//nolint:errchkjson // Method has no error return per interface contract
		_ = o.encoder.Encode([]map[string]string{})
		return
	}

	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(row) {
				obj[h] = row[i]
			} else {
				obj[h] = ""
			}
		}
		result = append(result, obj)
	}
	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(result)
}

// JSON outputs an arbitrary value as JSON.
// Returns an error if encoding fails.
func (o *JSONOutput) JSON(v interface{}) error {
	return o.encoder.Encode(v)
}

// Spinner returns a NoopSpinner for JSON output (AC: #6).
// JSON output doesn't support animated spinners.
// Context parameter accepted for interface compliance but not used.
func (o *JSONOutput) Spinner(_ context.Context, _ string) Spinner {
	return &NoopSpinner{}
}

// jsonURL is the structured format for URL output.
type jsonURL struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Display string `json:"display,omitempty"`
}

// URL outputs a URL as structured JSON.
// Format: {"type": "url", "url": "...", "display": "..."}
func (o *JSONOutput) URL(url, displayText string) {
	msg := jsonURL{
		Type: "url",
		URL:  url,
	}
	if displayText != "" && displayText != url {
		msg.Display = displayText
	}
	//nolint:errchkjson // Method has no error return per interface contract
	_ = o.encoder.Encode(msg)
}
