// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"io"
)

// TableColumn defines a column in a table.
type TableColumn struct {
	Name  string
	Width int
	Align Alignment
}

// Alignment defines text alignment in a column.
type Alignment int

// Alignment constants.
const (
	AlignLeft Alignment = iota
	AlignRight
	AlignCenter
)

// Table provides styled table rendering.
type Table struct {
	w       io.Writer
	styles  *TableStyles
	columns []TableColumn
}

// NewTable creates a new table with the given columns.
func NewTable(w io.Writer, columns []TableColumn) *Table {
	return &Table{
		w:       w,
		styles:  NewTableStyles(),
		columns: columns,
	}
}

// WriteHeader writes the table header row.
func (t *Table) WriteHeader() {
	header := ""
	for i, col := range t.columns {
		if i > 0 {
			header += " "
		}
		format := t.formatSpec(col)
		header += fmt.Sprintf(format, col.Name)
	}
	_, _ = fmt.Fprintln(t.w, t.styles.Header.Render(header))
}

// WriteRow writes a data row to the table.
func (t *Table) WriteRow(values ...string) {
	row := ""
	for i, col := range t.columns {
		if i > 0 {
			row += " "
		}
		format := t.formatSpec(col)
		value := ""
		if i < len(values) {
			value = values[i]
		}
		// Truncate if needed
		if len(value) > col.Width {
			value = value[:col.Width-1] + "…"
		}
		row += fmt.Sprintf(format, value)
	}
	_, _ = fmt.Fprintln(t.w, row)
}

// WriteStyledRow writes a data row with one styled cell.
func (t *Table) WriteStyledRow(values []string, styledIndex int, styledValue, plainValue string) {
	row := ""
	for i, col := range t.columns {
		if i > 0 {
			row += " "
		}
		format := t.formatSpec(col)

		if i == styledIndex {
			// Account for ANSI escape codes in width calculation
			offset := len(styledValue) - len(plainValue)
			adjustedFormat := t.formatSpecWithOffset(col, offset)
			row += fmt.Sprintf(adjustedFormat, styledValue)
		} else {
			value := ""
			if i < len(values) {
				value = values[i]
			}
			// Truncate if needed
			if len(value) > col.Width {
				value = value[:col.Width-1] + "…"
			}
			row += fmt.Sprintf(format, value)
		}
	}
	_, _ = fmt.Fprintln(t.w, row)
}

// formatSpec returns the format specifier for a column.
func (t *Table) formatSpec(col TableColumn) string {
	switch col.Align {
	case AlignRight:
		return fmt.Sprintf("%%%ds", col.Width)
	case AlignLeft, AlignCenter:
		return fmt.Sprintf("%%-%ds", col.Width)
	default:
		return fmt.Sprintf("%%-%ds", col.Width)
	}
}

// formatSpecWithOffset returns the format specifier with width adjusted for ANSI codes.
func (t *Table) formatSpecWithOffset(col TableColumn, offset int) string {
	width := col.Width + offset
	switch col.Align {
	case AlignRight:
		return fmt.Sprintf("%%%ds", width)
	case AlignLeft, AlignCenter:
		return fmt.Sprintf("%%-%ds", width)
	default:
		return fmt.Sprintf("%%-%ds", width)
	}
}

// ColorOffset calculates the difference in visible vs actual length due to ANSI codes.
func ColorOffset(rendered, plain string) int {
	return len(rendered) - len(plain)
}
