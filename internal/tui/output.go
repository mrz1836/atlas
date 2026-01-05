// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"context"
	"io"
	"os"

	"golang.org/x/term"
)

// Output format constants (AC: #2).
const (
	// FormatAuto auto-detects the output format based on TTY status.
	FormatAuto = ""

	// FormatText forces human-readable styled output.
	FormatText = "text"

	// FormatJSON forces machine-readable JSON output.
	FormatJSON = "json"
)

// Output is the interface for handling TTY vs JSON output (AC: #1).
// Commands use this interface to output human-friendly or machine-readable formats.
type Output interface {
	// Success outputs a success message with green styling (TTY) or structured JSON.
	Success(msg string)

	// Error outputs an error with red styling (TTY) or structured JSON with details.
	Error(err error)

	// Warning outputs a warning message with yellow styling (TTY) or structured JSON.
	Warning(msg string)

	// Info outputs an informational message with blue styling (TTY) or structured JSON.
	Info(msg string)

	// Table outputs tabular data with aligned columns (TTY) or array of objects (JSON).
	Table(headers []string, rows [][]string)

	// JSON outputs an arbitrary value as JSON (used for command-specific structured output).
	// Returns an error if encoding fails.
	JSON(v interface{}) error

	// Spinner returns a spinner for long-running operations.
	// TTY: Animated spinner using custom TerminalSpinner.
	// JSON: No-op spinner that does nothing.
	// Context is used for cancellation propagation.
	Spinner(ctx context.Context, msg string) Spinner

	// URL outputs a URL with optional display text.
	// TTY: Clickable OSC 8 hyperlink in supported terminals, underlined fallback otherwise.
	// JSON: Structured JSON with url and display fields.
	URL(url, displayText string)
}

// Spinner is the interface for progress indication during long-running operations (AC: #6).
type Spinner interface {
	// Update changes the spinner message.
	Update(msg string)

	// Stop terminates the spinner.
	Stop()
}

// NewOutput creates the appropriate Output implementation based on format (AC: #2).
//
// Format selection:
//   - FormatJSON ("json"): Always use JSONOutput
//   - FormatText ("text"): Always use TTYOutput
//   - FormatAuto (""): Auto-detect based on whether w is a TTY
//
// When auto-detecting, if w is an *os.File that is a terminal, TTYOutput is used.
// Otherwise, JSONOutput is used for non-TTY (piped) output.
func NewOutput(w io.Writer, format string) Output {
	switch format {
	case FormatJSON:
		return NewJSONOutput(w)
	case FormatText:
		return NewTTYOutput(w)
	default:
		// Auto-detect based on TTY status
		if isTTY(w) {
			return NewTTYOutput(w)
		}
		return NewJSONOutput(w)
	}
}

// isTTY checks if the writer is a terminal (AC: #2 auto-detection).
// Returns true if w is an *os.File and the file descriptor is a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
