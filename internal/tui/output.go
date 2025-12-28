// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"encoding/json"
	"fmt"
	"io"
)

// Output provides methods for structured output to a terminal.
type Output interface {
	// Success prints a success message.
	Success(msg string)
	// Error prints an error message.
	Error(err error)
	// Warning prints a warning message.
	Warning(msg string)
	// Info prints an informational message.
	Info(msg string)
	// JSON outputs a value as formatted JSON.
	JSON(v any) error
}

// TTYOutput provides styled output for terminal displays.
type TTYOutput struct {
	w      io.Writer
	styles *OutputStyles
}

// NewTTYOutput creates a new TTYOutput.
func NewTTYOutput(w io.Writer) *TTYOutput {
	return &TTYOutput{
		w:      w,
		styles: NewOutputStyles(),
	}
}

// Success prints a success message.
func (o *TTYOutput) Success(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Success.Render("✓ "+msg))
}

// Error prints an error message.
func (o *TTYOutput) Error(err error) {
	_, _ = fmt.Fprintln(o.w, o.styles.Error.Render("✗ "+err.Error()))
}

// Warning prints a warning message.
func (o *TTYOutput) Warning(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Warning.Render("⚠ "+msg))
}

// Info prints an informational message.
func (o *TTYOutput) Info(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Info.Render(msg))
}

// JSON outputs a value as formatted JSON.
func (o *TTYOutput) JSON(v any) error {
	encoder := json.NewEncoder(o.w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// JSONOutput provides plain JSON output without styling.
type JSONOutput struct {
	w io.Writer
}

// NewJSONOutput creates a new JSONOutput.
func NewJSONOutput(w io.Writer) *JSONOutput {
	return &JSONOutput{w: w}
}

// Success is a no-op for JSON output.
func (o *JSONOutput) Success(_ string) {}

// Error outputs the error as JSON.
func (o *JSONOutput) Error(err error) {
	_, _ = fmt.Fprintf(o.w, "{\"error\": %q}\n", err.Error())
}

// Warning is a no-op for JSON output.
func (o *JSONOutput) Warning(_ string) {}

// Info is a no-op for JSON output.
func (o *JSONOutput) Info(_ string) {}

// JSON outputs a value as formatted JSON.
func (o *JSONOutput) JSON(v any) error {
	encoder := json.NewEncoder(o.w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

// NewOutput creates the appropriate output based on format.
func NewOutput(w io.Writer, format string) Output {
	if format == "json" {
		return NewJSONOutput(w)
	}
	return NewTTYOutput(w)
}
