package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// TTYOutput provides styled terminal output using Lip Gloss (AC: #3).
// Uses the style system from Story 7.1 for consistent styling.
type TTYOutput struct {
	w      io.Writer
	styles *OutputStyles
	table  *TableStyles
}

// NewTTYOutput creates a new TTYOutput with styled output (AC: #3, #7).
// Respects NO_COLOR environment variable via CheckNoColor().
func NewTTYOutput(w io.Writer) *TTYOutput {
	// Respect NO_COLOR environment variable (AC: #7)
	CheckNoColor()

	return &TTYOutput{
		w:      w,
		styles: NewOutputStyles(),
		table:  NewTableStyles(),
	}
}

// Success outputs a success message with green color and ✓ icon (AC: #3).
func (o *TTYOutput) Success(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Success.Render("✓ "+msg))
}

// Error outputs an error with red color and ✗ icon (AC: #3).
func (o *TTYOutput) Error(err error) {
	_, _ = fmt.Fprintln(o.w, o.styles.Error.Render("✗ "+err.Error()))
}

// Warning outputs a warning message with yellow color and ⚠ icon (AC: #3).
func (o *TTYOutput) Warning(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Warning.Render("⚠ "+msg))
}

// Info outputs an informational message with blue color and ℹ icon (AC: #3).
func (o *TTYOutput) Info(msg string) {
	_, _ = fmt.Fprintln(o.w, o.styles.Info.Render("ℹ "+msg))
}

// Table outputs tabular data with aligned columns (AC: #5).
// Applies TableStyles from styles.go for headers and cells.
func (o *TTYOutput) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths based on content (AC: #5 subtask 4.3)
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				cellWidth := utf8.RuneCountInString(cell)
				if cellWidth > widths[i] {
					widths[i] = cellWidth
				}
			}
		}
	}

	// Render header row with bold styling
	headerParts := make([]string, 0, len(headers))
	for i, h := range headers {
		headerParts = append(headerParts, o.table.Header.Render(padRight(h, widths[i])))
	}
	_, _ = fmt.Fprintln(o.w, strings.Join(headerParts, "  "))

	// Render data rows
	for _, row := range rows {
		var rowParts []string
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			rowParts = append(rowParts, o.table.Cell.Render(padRight(cell, widths[i])))
		}
		_, _ = fmt.Fprintln(o.w, strings.Join(rowParts, "  "))
	}
}

// JSON outputs an arbitrary value as formatted JSON.
// For TTY output, this is used when commands need to output structured data.
// Returns an error if encoding fails.
func (o *TTYOutput) JSON(v interface{}) error {
	encoder := json.NewEncoder(o.w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// Spinner returns a SpinnerAdapter for animated progress indication (AC: #6).
func (o *TTYOutput) Spinner(msg string) Spinner {
	return NewSpinnerAdapter(o.w, msg)
}
