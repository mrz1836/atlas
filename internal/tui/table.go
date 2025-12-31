// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/mrz1836/atlas/internal/constants"
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

// ========================================
// StatusTable - Workspace Status Display
// ========================================

// MinColumnWidths defines the minimum width for each status table column.
// Used to ensure readability even with short content.
//
//nolint:gochecknoglobals // Intentional package-level constant for status table minimum widths
var MinColumnWidths = StatusColumnWidths{
	Workspace: 10,
	Branch:    12,
	Status:    18,
	Step:      6,
	Action:    10,
}

// StatusColumnWidths holds the widths for each status table column.
type StatusColumnWidths struct {
	Workspace int
	Branch    int
	Status    int
	Step      int
	Action    int
}

// StatusRow represents one row in the status table (AC: #2).
// Contains all fields needed for workspace status display.
type StatusRow struct {
	Workspace   string
	Branch      string
	Status      constants.TaskStatus
	CurrentStep int
	TotalSteps  int
	// Action is the suggested action, if any. If empty, uses SuggestedAction().
	Action string
}

// StatusTableConfig holds configuration for the status table.
type StatusTableConfig struct {
	// TerminalWidth is the detected terminal width (or forced width for testing).
	TerminalWidth int
	// Narrow indicates whether to use abbreviated headers (< 80 cols).
	Narrow bool
}

// StatusTableOption is a functional option for StatusTable configuration.
type StatusTableOption func(*StatusTable)

// WithTerminalWidth sets a specific terminal width (useful for testing).
func WithTerminalWidth(width int) StatusTableOption {
	return func(t *StatusTable) {
		t.config.TerminalWidth = width
		t.config.Narrow = width > 0 && width < 80
	}
}

// StatusTable renders workspace status in a formatted table (AC: #1, #2).
// Supports both TTY and JSON output via the ToTableData method.
type StatusTable struct {
	rows   []StatusRow
	styles *TableStyles
	config StatusTableConfig
}

// NewStatusTable creates a new status table with the given rows (AC: #1).
// Automatically detects terminal width and narrow mode.
func NewStatusTable(rows []StatusRow, opts ...StatusTableOption) *StatusTable {
	t := &StatusTable{
		rows:   rows,
		styles: NewTableStyles(),
		config: StatusTableConfig{
			TerminalWidth: detectTerminalWidth(),
		},
	}

	// Apply terminal width detection first
	t.config.Narrow = t.config.TerminalWidth > 0 && t.config.TerminalWidth < 80

	// Apply any options (may override width/narrow settings)
	for _, opt := range opts {
		opt(t)
	}

	return t
}

// detectTerminalWidth returns the current terminal width.
// Returns 0 if detection fails (assume wide terminal).
func detectTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0 // Assume wide if detection fails
	}
	return width
}

// IsNarrow returns true if the terminal is in narrow mode (< 80 cols) (AC: #5).
func (t *StatusTable) IsNarrow() bool {
	return t.config.Narrow
}

// Headers returns the column headers, abbreviated if in narrow mode (AC: #5).
func (t *StatusTable) Headers() []string {
	if t.config.Narrow {
		return []string{"WS", "BRANCH", "STAT", "STEP", "ACT"}
	}
	return []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}
}

// FullHeaders returns the full (non-abbreviated) column headers.
// Used for JSON output which should always use full names (AC: #6).
func (t *StatusTable) FullHeaders() []string {
	return []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}
}

// Render writes the formatted table to the writer (AC: #1, #2, #5).
// Uses bold header styling and proper column alignment.
func (t *StatusTable) Render(w io.Writer) error {
	headers := t.Headers()
	widths := t.calculateColumnWidths()
	widthsSlice := []int{widths.Workspace, widths.Branch, widths.Status, widths.Step, widths.Action}

	// Render header row with bold styling
	headerParts := make([]string, len(headers))
	for i, h := range headers {
		headerParts[i] = t.styles.Header.Render(padRight(h, widthsSlice[i]))
	}
	_, err := fmt.Fprintln(w, strings.Join(headerParts, "  "))
	if err != nil {
		return err
	}

	// Render data rows
	for _, row := range t.rows {
		rowCells := []string{
			padRight(row.Workspace, widths.Workspace),
			padRight(row.Branch, widths.Branch),
			t.renderStatusCellPadded(row.Status, widths.Status),
			padRight(t.formatStep(row.CurrentStep, row.TotalSteps), widths.Step),
			padRight(t.renderActionCell(row.Status, row.Action), widths.Action),
		}
		_, err = fmt.Fprintln(w, strings.Join(rowCells, "  "))
		if err != nil {
			return err
		}
	}

	return nil
}

// ToTableData converts the table to Output.Table() compatible format (AC: #6).
// Returns headers and rows as string slices.
// Uses abbreviated headers in narrow mode.
func (t *StatusTable) ToTableData() ([]string, [][]string) {
	headers := t.Headers()

	rows := make([][]string, len(t.rows))
	for i, row := range t.rows {
		rows[i] = []string{
			row.Workspace,
			row.Branch,
			t.renderStatusCellPlain(row.Status), // Plain for data transfer
			t.formatStep(row.CurrentStep, row.TotalSteps),
			t.renderActionCell(row.Status, row.Action),
		}
	}
	return headers, rows
}

// ToJSONData converts the table to JSON-compatible format (AC: #6).
// Returns headers and rows with full (non-abbreviated) header names.
func (t *StatusTable) ToJSONData() ([]string, [][]string) {
	headers := t.FullHeaders() // Always use full headers for JSON

	rows := make([][]string, len(t.rows))
	for i, row := range t.rows {
		rows[i] = []string{
			row.Workspace,
			row.Branch,
			t.renderStatusCellPlain(row.Status),
			t.formatStep(row.CurrentStep, row.TotalSteps),
			t.renderActionCell(row.Status, row.Action),
		}
	}
	return headers, rows
}

// Rows returns a copy of the status rows (useful for iteration).
// Returns a copy to prevent external mutation of internal state.
func (t *StatusTable) Rows() []StatusRow {
	if t.rows == nil {
		return nil
	}
	result := make([]StatusRow, len(t.rows))
	copy(result, t.rows)
	return result
}

// WideTerminalThreshold is the minimum terminal width for proportional column expansion.
// Terminals 120+ columns wide get proportionally expanded columns (UX-10).
const WideTerminalThreshold = 120

// calculateColumnWidths calculates widths for each column based on content (AC: #1, #2).
// Uses utf8.RuneCountInString for proper Unicode handling.
// For wide terminals (120+ cols), applies proportional width expansion (Task 2.5).
func (t *StatusTable) calculateColumnWidths() StatusColumnWidths {
	// Start with minimum widths
	widths := StatusColumnWidths{
		Workspace: MinColumnWidths.Workspace,
		Branch:    MinColumnWidths.Branch,
		Status:    MinColumnWidths.Status,
		Step:      MinColumnWidths.Step,
		Action:    MinColumnWidths.Action,
	}

	// Also consider header widths
	headers := t.Headers()
	widthsSlice := []int{
		max(widths.Workspace, utf8.RuneCountInString(headers[0])),
		max(widths.Branch, utf8.RuneCountInString(headers[1])),
		max(widths.Status, utf8.RuneCountInString(headers[2])),
		max(widths.Step, utf8.RuneCountInString(headers[3])),
		max(widths.Action, utf8.RuneCountInString(headers[4])),
	}

	// Calculate widths based on content
	for _, row := range t.rows {
		// Workspace
		w := utf8.RuneCountInString(row.Workspace)
		if w > widthsSlice[0] {
			widthsSlice[0] = w
		}

		// Branch
		w = utf8.RuneCountInString(row.Branch)
		if w > widthsSlice[1] {
			widthsSlice[1] = w
		}

		// Status (icon + space + status text)
		statusCell := t.renderStatusCellPlain(row.Status)
		w = utf8.RuneCountInString(statusCell)
		if w > widthsSlice[2] {
			widthsSlice[2] = w
		}

		// Step
		stepCell := t.formatStep(row.CurrentStep, row.TotalSteps)
		w = utf8.RuneCountInString(stepCell)
		if w > widthsSlice[3] {
			widthsSlice[3] = w
		}

		// Action
		actionCell := t.renderActionCell(row.Status, row.Action)
		w = utf8.RuneCountInString(actionCell)
		if w > widthsSlice[4] {
			widthsSlice[4] = w
		}
	}

	// Apply proportional width expansion for wide terminals (120+ cols) (Task 2.5)
	if t.config.TerminalWidth >= WideTerminalThreshold {
		widthsSlice = t.applyProportionalExpansion(widthsSlice)
	}

	return StatusColumnWidths{
		Workspace: widthsSlice[0],
		Branch:    widthsSlice[1],
		Status:    widthsSlice[2],
		Step:      widthsSlice[3],
		Action:    widthsSlice[4],
	}
}

// applyProportionalExpansion distributes extra terminal width among columns (Task 2.5).
// Only expands variable-width columns (Workspace, Branch, Action).
// Fixed-width columns (Status, Step) remain unchanged for consistency.
func (t *StatusTable) applyProportionalExpansion(widths []int) []int {
	// Calculate current total width (columns + separators)
	// 5 columns with 2-space separators = 4 separators * 2 chars = 8 chars
	const separatorWidth = 8
	totalContentWidth := 0
	for _, w := range widths {
		totalContentWidth += w
	}
	totalWidth := totalContentWidth + separatorWidth

	// Calculate available extra space
	extraSpace := t.config.TerminalWidth - totalWidth
	if extraSpace <= 0 {
		return widths // No extra space to distribute
	}

	// Only expand variable-width columns: Workspace (0), Branch (1), Action (4)
	// Status (2) and Step (3) are fixed-width for visual consistency
	expandableIndices := []int{0, 1, 4}
	expandableTotal := widths[0] + widths[1] + widths[4]

	if expandableTotal == 0 {
		return widths // Avoid division by zero
	}

	// Distribute extra space proportionally among expandable columns
	// Cap expansion at 50% of original width to avoid overly wide columns
	result := make([]int, len(widths))
	copy(result, widths)

	for _, idx := range expandableIndices {
		proportion := float64(widths[idx]) / float64(expandableTotal)
		expansion := int(float64(extraSpace) * proportion)

		// Cap expansion at 50% of original width
		maxExpansion := widths[idx] / 2
		if expansion > maxExpansion {
			expansion = maxExpansion
		}

		result[idx] = widths[idx] + expansion
	}

	return result
}

// renderStatusCell creates the status cell content with icon and colored text (AC: #3).
// Uses triple redundancy: icon + color + text per UX-8.
func (t *StatusTable) renderStatusCell(status constants.TaskStatus) string {
	icon := TaskStatusIcon(status)
	color := TaskStatusColors()[status]
	style := lipgloss.NewStyle().Foreground(color)
	return icon + " " + style.Render(string(status))
}

// renderStatusCellPlain creates the status cell content without ANSI color codes.
// Used for JSON output and width calculations.
func (t *StatusTable) renderStatusCellPlain(status constants.TaskStatus) string {
	icon := TaskStatusIcon(status)
	return icon + " " + string(status)
}

// renderActionCell creates the action cell content (AC: #4).
// Returns the suggested action or em-dash if no action is needed.
func (t *StatusTable) renderActionCell(status constants.TaskStatus, customAction string) string {
	// Use custom action if provided
	if customAction != "" {
		return customAction
	}

	// Otherwise use SuggestedAction
	action := SuggestedAction(status)
	if action == "" {
		return "—" // Em-dash for no action
	}
	return action
}

// formatStep formats the step counter as "current/total".
func (t *StatusTable) formatStep(current, total int) string {
	return fmt.Sprintf("%d/%d", current, total)
}

// renderStatusCellPadded renders the status cell with proper padding.
// Padding is calculated based on visible character width (excluding ANSI codes).
func (t *StatusTable) renderStatusCellPadded(status constants.TaskStatus, width int) string {
	// Get the plain text version for width calculation
	plainText := t.renderStatusCellPlain(status)
	plainWidth := utf8.RuneCountInString(plainText)

	// Get the styled version
	styledText := t.renderStatusCell(status)

	// Calculate padding needed
	if plainWidth >= width {
		return styledText
	}
	return styledText + strings.Repeat(" ", width-plainWidth)
}
