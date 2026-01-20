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
		// Truncate if needed (require Width > 1 to avoid slice bounds panic)
		if col.Width > 1 && len(value) > col.Width {
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
			// Truncate if needed (require Width > 1 to avoid slice bounds panic)
			if col.Width > 1 && len(value) > col.Width {
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
	// StepName is the name of the currently executing step (e.g., "ci_wait", "implement").
	StepName string
	// Action is the suggested action, if any. If empty, uses SuggestedAction().
	Action string
}

// StatusTableConfig holds configuration for the status table.
type StatusTableConfig struct {
	// TerminalWidth is the detected terminal width (or forced width for testing).
	TerminalWidth int
	// Narrow indicates whether to use abbreviated headers (< TerminalWidthNarrow cols).
	Narrow bool
}

// StatusTableOption is a functional option for StatusTable configuration.
type StatusTableOption func(*StatusTable)

// WithTerminalWidth sets a specific terminal width (useful for testing).
func WithTerminalWidth(width int) StatusTableOption {
	return func(t *StatusTable) {
		t.config.TerminalWidth = width
		t.config.Narrow = width > 0 && width < TerminalWidthNarrow
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
	t.config.Narrow = t.config.TerminalWidth > 0 && t.config.TerminalWidth < TerminalWidthNarrow

	// Apply any options (may override width/narrow settings)
	for _, opt := range opts {
		opt(t)
	}

	return t
}

// detectTerminalWidth returns the current terminal width.
// Returns 80 if detection fails (assume standard terminal).
func detectTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Assume standard terminal if detection fails
	}
	return width
}

// IsNarrow returns true if the terminal is in narrow mode (< TerminalWidthNarrow cols) (AC: #5).
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
			padRight(t.formatStep(row.CurrentStep, row.TotalSteps, row.StepName, row.Status), widths.Step),
			t.renderActionCellPadded(row.Status, row.Action, widths.Action),
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
			t.formatStep(row.CurrentStep, row.TotalSteps, row.StepName, row.Status),
			t.renderActionCellPlain(row.Status, row.Action), // Plain for data transfer
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
			t.formatStep(row.CurrentStep, row.TotalSteps, row.StepName, row.Status),
			t.renderActionCellPlain(row.Status, row.Action), // Plain for JSON
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
	widthsSlice := t.initializeMinWidths()
	t.updateWidthsFromContent(widthsSlice)
	widthsSlice = t.applyWidthConstraints(widthsSlice)

	return StatusColumnWidths{
		Workspace: widthsSlice[0],
		Branch:    widthsSlice[1],
		Status:    widthsSlice[2],
		Step:      widthsSlice[3],
		Action:    widthsSlice[4],
	}
}

// initializeMinWidths creates the initial width slice using minimum widths and headers.
func (t *StatusTable) initializeMinWidths() []int {
	headers := t.Headers()
	return []int{
		max(MinColumnWidths.Workspace, utf8.RuneCountInString(headers[0])),
		max(MinColumnWidths.Branch, utf8.RuneCountInString(headers[1])),
		max(MinColumnWidths.Status, utf8.RuneCountInString(headers[2])),
		max(MinColumnWidths.Step, utf8.RuneCountInString(headers[3])),
		max(MinColumnWidths.Action, utf8.RuneCountInString(headers[4])),
	}
}

// updateWidthsFromContent expands widths based on actual row content.
func (t *StatusTable) updateWidthsFromContent(widths []int) {
	for _, row := range t.rows {
		// Workspace
		if w := utf8.RuneCountInString(row.Workspace); w > widths[0] {
			widths[0] = w
		}

		// Branch
		if w := utf8.RuneCountInString(row.Branch); w > widths[1] {
			widths[1] = w
		}

		// Status (icon + space + status text)
		statusCell := t.renderStatusCellPlain(row.Status)
		if w := utf8.RuneCountInString(statusCell); w > widths[2] {
			widths[2] = w
		}

		// Step
		stepCell := t.formatStep(row.CurrentStep, row.TotalSteps, row.StepName, row.Status)
		if w := utf8.RuneCountInString(stepCell); w > widths[3] {
			widths[3] = w
		}

		// Action (use plain version for width calculation to avoid ANSI codes)
		actionCell := t.renderActionCellPlain(row.Status, row.Action)
		if w := utf8.RuneCountInString(actionCell); w > widths[4] {
			widths[4] = w
		}
	}
}

// applyWidthConstraints constrains widths to terminal and applies proportional expansion.
func (t *StatusTable) applyWidthConstraints(widths []int) []int {
	// Constrain to terminal width first to ensure all columns are visible
	widths = t.constrainToTerminalWidth(widths)

	// Apply proportional width expansion for wide terminals (TerminalWidthWide+ cols) (Task 2.5)
	if t.config.TerminalWidth >= WideTerminalThreshold {
		widths = t.applyProportionalExpansion(widths)
	}

	return widths
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

// constrainToTerminalWidth reduces column widths to fit within terminal width.
// Prioritizes reducing variable-width columns (Branch, Workspace) while preserving
// fixed-width columns (Status, Step, Action) to ensure all columns are visible.
func (t *StatusTable) constrainToTerminalWidth(widths []int) []int {
	// Calculate total width (columns + separators)
	// 5 columns with 2-space separators = 4 separators * 2 chars = 8 chars
	const separatorWidth = 8
	totalContentWidth := 0
	for _, w := range widths {
		totalContentWidth += w
	}
	totalWidth := totalContentWidth + separatorWidth

	// If fits within terminal, no changes needed
	if t.config.TerminalWidth <= 0 || totalWidth <= t.config.TerminalWidth {
		return widths
	}

	// Calculate overflow amount
	overflow := totalWidth - t.config.TerminalWidth

	result := make([]int, len(widths))
	copy(result, widths)

	// Reduce Branch column first (index 1), then Workspace (index 0) if needed
	// These are variable-width columns that can be truncated
	reduceableIndices := []int{1, 0} // Branch first, then Workspace

	for _, idx := range reduceableIndices {
		if overflow <= 0 {
			break
		}

		// Calculate maximum reduction (current width - minimum width)
		minWidth := MinColumnWidths.Branch
		if idx == 0 {
			minWidth = MinColumnWidths.Workspace
		}

		maxReduction := result[idx] - minWidth
		if maxReduction <= 0 {
			continue // Already at minimum
		}

		// Apply reduction (up to max allowed)
		reduction := overflow
		if reduction > maxReduction {
			reduction = maxReduction
		}

		result[idx] -= reduction
		overflow -= reduction
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
// For attention states, applies warning color styling (Story 7.9).
// Maintains triple redundancy: icon + color + text for attention states.
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

	// Apply warning styling for attention states (Story 7.9, AC: #4)
	// NO_COLOR mode uses "(!) " prefix for accessibility (triple redundancy)
	if IsAttentionStatus(status) {
		if !HasColorSupport() {
			return "(!) " + action
		}
		return ActionStyle().Render(action)
	}
	return action
}

// renderActionCellPlain creates the action cell content without ANSI codes.
// Used for JSON output and width calculations.
func (t *StatusTable) renderActionCellPlain(status constants.TaskStatus, customAction string) string {
	// Use custom action if provided
	if customAction != "" {
		return customAction
	}

	// Otherwise use SuggestedAction
	action := SuggestedAction(status)
	if action == "" {
		return "—" // Em-dash for no action
	}

	// For attention states in NO_COLOR mode, include the prefix
	if IsAttentionStatus(status) && !HasColorSupport() {
		return "(!) " + action
	}
	return action
}

// humanizeStepName converts internal step names to user-friendly labels.
func humanizeStepName(name string) string {
	mapping := map[string]string{
		"analyze":     "Analyzing",
		"implement":   "Implementing",
		"verify":      "Verifying",
		"validate":    "Validating",
		"checklist":   "Checklist",
		"git_commit":  "Committing",
		"git_push":    "Pushing",
		"git_pr":      "Creating PR",
		"ci_wait":     "Waiting for CI",
		"review":      "Review",
		"specify":     "Specifying",
		"review_spec": "Reviewing Spec",
		"plan":        "Planning",
		"tasks":       "Creating Tasks",
	}
	if label, ok := mapping[name]; ok {
		return label
	}
	return name // fallback to raw name
}

// formatStep formats the step counter as "current/total" with optional step name for running tasks.
func (t *StatusTable) formatStep(current, total int, stepName string, status constants.TaskStatus) string {
	base := fmt.Sprintf("%d/%d", current, total)
	// Only show step name for running tasks
	if stepName != "" && status == constants.TaskStatusRunning {
		return fmt.Sprintf("%s %s", base, humanizeStepName(stepName))
	}
	return base
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

// renderActionCellPadded renders the action cell with proper padding.
// Padding is calculated based on visible character width (excluding ANSI codes).
func (t *StatusTable) renderActionCellPadded(status constants.TaskStatus, customAction string, width int) string {
	// Get the plain text version for width calculation
	plainText := t.renderActionCellPlain(status, customAction)
	plainWidth := utf8.RuneCountInString(plainText)

	// Get the styled version
	styledText := t.renderActionCell(status, customAction)

	// Calculate padding needed
	if plainWidth >= width {
		return styledText
	}
	return styledText + strings.Repeat(" ", width-plainWidth)
}

// ========================================
// HierarchicalStatusTable - Nested Task Display
// ========================================

// MaxTasksPerWorkspace is the maximum number of tasks shown per workspace.
// Additional tasks are indicated with "+N more".
const MaxTasksPerWorkspace = 3

// RowType identifies whether a row represents a workspace or a task.
type RowType string

const (
	// RowTypeWorkspace indicates a workspace row.
	RowTypeWorkspace RowType = "workspace"
	// RowTypeTask indicates a task row nested under a workspace.
	RowTypeTask RowType = "task"
	// RowTypeMore indicates a "+N more" indicator row.
	RowTypeMore RowType = "more"
)

// TaskInfo contains information about a single task for hierarchical display.
type TaskInfo struct {
	ID          string
	Path        string // Full path for hyperlink
	Template    string
	Status      constants.TaskStatus
	CurrentStep int
	TotalSteps  int
}

// HierarchicalRow represents a row in the hierarchical status table.
// Can be either a workspace row or a task row.
type HierarchicalRow struct {
	Type RowType

	// Workspace fields (for RowTypeWorkspace)
	Workspace  string
	Branch     string
	TaskCount  int // Total tasks in workspace
	Tasks      []TaskInfo
	TasksShown int // Number of tasks actually shown (may be limited)

	// Task fields (for RowTypeTask)
	Task   *TaskInfo
	IsLast bool // True if this is the last task in the workspace

	// More indicator (for RowTypeMore)
	MoreCount int
}

// WorkspaceGroup holds a workspace and all its tasks for hierarchical display.
type WorkspaceGroup struct {
	Name       string
	Branch     string
	Status     constants.TaskStatus // Aggregate status from most recent task
	Tasks      []TaskInfo
	TotalTasks int
}

// HierarchicalStatusTable renders workspace status with nested tasks.
type HierarchicalStatusTable struct {
	groups []WorkspaceGroup
	styles *TableStyles
	config StatusTableConfig
}

// NewHierarchicalStatusTable creates a new hierarchical status table.
func NewHierarchicalStatusTable(groups []WorkspaceGroup, opts ...StatusTableOption) *HierarchicalStatusTable {
	t := &HierarchicalStatusTable{
		groups: groups,
		styles: NewTableStyles(),
		config: StatusTableConfig{
			TerminalWidth: detectTerminalWidth(),
		},
	}

	t.config.Narrow = t.config.TerminalWidth > 0 && t.config.TerminalWidth < TerminalWidthNarrow

	// Apply options using adapter (StatusTableOption works on StatusTable)
	adapter := &StatusTable{config: t.config}
	for _, opt := range opts {
		opt(adapter)
	}
	t.config = adapter.config

	return t
}

// Groups returns the workspace groups.
func (t *HierarchicalStatusTable) Groups() []WorkspaceGroup {
	return t.groups
}

// Headers returns the column headers for the hierarchical table.
func (t *HierarchicalStatusTable) Headers() []string {
	if t.config.Narrow {
		return []string{"WS", "BRANCH", "STAT", "TASKS"}
	}
	return []string{"WORKSPACE", "BRANCH", "STATUS", "TASKS"}
}

// FullHeaders returns the full (non-abbreviated) column headers.
func (t *HierarchicalStatusTable) FullHeaders() []string {
	return []string{"WORKSPACE", "BRANCH", "STATUS", "TASKS"}
}

// HierarchicalColumnWidths holds the widths for hierarchical table columns.
type HierarchicalColumnWidths struct {
	Workspace int
	Branch    int
	Status    int
	Tasks     int
}

// MinHierarchicalColumnWidths defines minimum widths for hierarchical columns.
//
//nolint:gochecknoglobals // Intentional package-level constant
var MinHierarchicalColumnWidths = HierarchicalColumnWidths{
	Workspace: 10,
	Branch:    12,
	Status:    18,
	Tasks:     8,
}

// Render writes the hierarchical status table to the writer.
func (t *HierarchicalStatusTable) Render(w io.Writer) error {
	if len(t.groups) == 0 {
		return nil
	}

	headers := t.Headers()
	widths := t.calculateColumnWidths()

	// Render header
	headerParts := []string{
		t.styles.Header.Render(padRight(headers[0], widths.Workspace)),
		t.styles.Header.Render(padRight(headers[1], widths.Branch)),
		t.styles.Header.Render(padRight(headers[2], widths.Status)),
		t.styles.Header.Render(padRight(headers[3], widths.Tasks)),
	}
	if _, err := fmt.Fprintln(w, strings.Join(headerParts, "  ")); err != nil {
		return err
	}

	// Render each workspace group
	for _, group := range t.groups {
		if err := t.renderWorkspaceGroup(w, group, widths); err != nil {
			return err
		}
	}

	return nil
}

// ToJSONData converts the hierarchical table to JSON-compatible format.
func (t *HierarchicalStatusTable) ToJSONData() []HierarchicalJSONWorkspace {
	result := make([]HierarchicalJSONWorkspace, len(t.groups))

	for i, group := range t.groups {
		tasks := make([]HierarchicalJSONTask, len(group.Tasks))
		for j, task := range group.Tasks {
			tasks[j] = HierarchicalJSONTask{
				ID:       task.ID,
				Status:   string(task.Status),
				Step:     fmt.Sprintf("%d/%d", task.CurrentStep, task.TotalSteps),
				Template: task.Template,
			}
		}

		result[i] = HierarchicalJSONWorkspace{
			Name:       group.Name,
			Branch:     group.Branch,
			Status:     string(group.Status),
			Tasks:      tasks,
			TotalTasks: group.TotalTasks,
		}
	}

	return result
}

// calculateColumnWidths calculates column widths based on content.
func (t *HierarchicalStatusTable) calculateColumnWidths() HierarchicalColumnWidths {
	headers := t.Headers()
	widths := HierarchicalColumnWidths{
		Workspace: max(MinHierarchicalColumnWidths.Workspace, utf8.RuneCountInString(headers[0])),
		Branch:    max(MinHierarchicalColumnWidths.Branch, utf8.RuneCountInString(headers[1])),
		Status:    max(MinHierarchicalColumnWidths.Status, utf8.RuneCountInString(headers[2])),
		Tasks:     max(MinHierarchicalColumnWidths.Tasks, utf8.RuneCountInString(headers[3])),
	}

	for _, group := range t.groups {
		// Workspace name with count suffix (e.g., "workspace-name (2)")
		countSuffix := ""
		if group.TotalTasks > 0 {
			countSuffix = fmt.Sprintf(" (%d)", group.TotalTasks)
		}
		if w := utf8.RuneCountInString(group.Name + countSuffix); w > widths.Workspace {
			widths.Workspace = w
		}
		// Branch
		if w := utf8.RuneCountInString(group.Branch); w > widths.Branch {
			widths.Branch = w
		}
		// Status (icon + space + text)
		statusText := TaskStatusIcon(group.Status) + " " + string(group.Status)
		if w := utf8.RuneCountInString(statusText); w > widths.Status {
			widths.Status = w
		}
		// Tasks column: progress bar (8 chars) or percentage (4 chars "100%")
		// Use 8 as the standard width for progress bar display
		const progressBarWidth = 8
		if progressBarWidth > widths.Tasks {
			widths.Tasks = progressBarWidth
		}
	}

	// Constrain to terminal width
	widths = t.constrainWidths(widths)

	return widths
}

// constrainWidths reduces column widths to fit terminal.
func (t *HierarchicalStatusTable) constrainWidths(widths HierarchicalColumnWidths) HierarchicalColumnWidths {
	const separatorWidth = 6 // 3 columns with 2-space separators
	totalWidth := widths.Workspace + widths.Branch + widths.Status + widths.Tasks + separatorWidth

	if t.config.TerminalWidth <= 0 || totalWidth <= t.config.TerminalWidth {
		return widths
	}

	overflow := totalWidth - t.config.TerminalWidth

	// Reduce Branch first, then Workspace
	if widths.Branch > MinHierarchicalColumnWidths.Branch {
		reduction := min(overflow, widths.Branch-MinHierarchicalColumnWidths.Branch)
		widths.Branch -= reduction
		overflow -= reduction
	}

	if overflow > 0 && widths.Workspace > MinHierarchicalColumnWidths.Workspace {
		reduction := min(overflow, widths.Workspace-MinHierarchicalColumnWidths.Workspace)
		widths.Workspace -= reduction
	}

	return widths
}

// renderWorkspaceGroup renders a workspace and its nested tasks.
func (t *HierarchicalStatusTable) renderWorkspaceGroup(w io.Writer, group WorkspaceGroup, widths HierarchicalColumnWidths) error {
	// Workspace row
	statusCell := t.renderStatusCell(group.Status, widths.Status)

	// Build workspace name with subtle task count suffix
	dimStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	countSuffix := ""
	countSuffixLen := 0
	if group.TotalTasks > 0 {
		countSuffix = fmt.Sprintf(" (%d)", group.TotalTasks)
		countSuffixLen = len(countSuffix)
	}

	// Truncate workspace name to leave room for count suffix
	maxNameLen := widths.Workspace - countSuffixLen
	if maxNameLen < 4 {
		maxNameLen = 4 // Minimum readable name
	}
	truncatedName := truncateString(group.Name, maxNameLen)

	// Build the full workspace cell with styled suffix
	workspaceCell := truncatedName + dimStyle.Render(countSuffix)
	visibleLen := utf8.RuneCountInString(truncatedName + countSuffix)
	if visibleLen < widths.Workspace {
		workspaceCell += strings.Repeat(" ", widths.Workspace-visibleLen)
	}

	row := []string{
		workspaceCell,
		padRight(truncateString(group.Branch, widths.Branch), widths.Branch),
		statusCell,
		padRight("—", widths.Tasks), // TASKS column empty for workspace rows
	}
	if _, err := fmt.Fprintln(w, strings.Join(row, "  ")); err != nil {
		return err
	}

	// Task rows (limited to MaxTasksPerWorkspace)
	tasksToShow := group.Tasks
	if len(tasksToShow) > MaxTasksPerWorkspace {
		tasksToShow = tasksToShow[:MaxTasksPerWorkspace]
	}

	for i, task := range tasksToShow {
		isLast := i == len(tasksToShow)-1 && len(group.Tasks) <= MaxTasksPerWorkspace
		if err := t.renderTaskRow(w, task, isLast, widths); err != nil {
			return err
		}
	}

	// "+N more" indicator if tasks were truncated
	if len(group.Tasks) > MaxTasksPerWorkspace {
		moreCount := len(group.Tasks) - MaxTasksPerWorkspace
		if err := t.renderMoreRow(w, moreCount, widths); err != nil {
			return err
		}
	}

	return nil
}

// renderMiniProgressBar renders an 8-character wide progress bar.
// Uses block characters for styled mode, ASCII for NO_COLOR mode.
func renderMiniProgressBar(current, total int) string {
	const barWidth = 8

	if total == 0 {
		if HasColorSupport() {
			return strings.Repeat("█", barWidth)
		}
		return "[" + strings.Repeat("=", barWidth-2) + "]"
	}

	filled := (current * barWidth) / total
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	if HasColorSupport() {
		return strings.Repeat("█", filled) + strings.Repeat("░", empty)
	}
	// NO_COLOR mode: ASCII fallback
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", empty) + "]"
}

// renderTaskRow renders a single task row with tree prefix.
func (t *HierarchicalStatusTable) renderTaskRow(w io.Writer, task TaskInfo, isLast bool, widths HierarchicalColumnWidths) error {
	// Tree prefix
	prefix := TreeChars.Branch
	if isLast {
		prefix = TreeChars.LastBranch
	}

	// Task ID (dimmed, with hyperlink if path available)
	taskID := task.ID
	if task.Path != "" {
		taskID = RenderFileHyperlink(taskID, task.Path)
	}
	dimStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	taskIDStyled := dimStyle.Render(taskID)

	// Calculate visible width (prefix + task ID text only, no escape sequences)
	visibleFirstCol := prefix + task.ID
	visibleWidth := utf8.RuneCountInString(visibleFirstCol)

	// Pad to fill WORKSPACE + separator + BRANCH columns so status aligns correctly
	targetWidth := widths.Workspace + 2 + widths.Branch
	padding := ""
	if visibleWidth < targetWidth {
		padding = strings.Repeat(" ", targetWidth-visibleWidth)
	}

	// Status cell
	statusCell := t.renderStatusCell(task.Status, widths.Status)

	// Step progress: progress bar for active, percentage for completed/failed
	var stepInfo string
	percent := 0
	if task.TotalSteps > 0 {
		percent = (task.CurrentStep * 100) / task.TotalSteps
	}

	//exhaustive:ignore // All non-active/non-completed states show percentage
	switch task.Status {
	case constants.TaskStatusCompleted:
		// Clean text for completed - no visual noise
		stepInfo = "100%"
	case constants.TaskStatusRunning, constants.TaskStatusValidating:
		// Progress bar for active tasks
		stepInfo = renderMiniProgressBar(task.CurrentStep, task.TotalSteps)
	default:
		// Percentage for pending/failed/terminal states
		stepInfo = fmt.Sprintf("%d%%", percent)
	}

	// Build row manually: prefix + taskID + padding + separator + status + separator + tasks
	// This ensures proper alignment despite invisible escape sequences in taskIDStyled
	line := prefix + taskIDStyled + padding + "  " + statusCell + "  " + stepInfo

	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}

	return nil
}

// renderMoreRow renders the "+N more" indicator.
func (t *HierarchicalStatusTable) renderMoreRow(w io.Writer, count int, widths HierarchicalColumnWidths) error {
	prefix := TreeChars.LastBranch
	dimStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	moreTextPlain := fmt.Sprintf("+%d more", count)
	moreTextStyled := dimStyle.Render(moreTextPlain)

	// Calculate visible width (prefix + plain text, no escape sequences)
	visibleWidth := utf8.RuneCountInString(prefix + moreTextPlain)

	// Pad to fill WORKSPACE + separator + BRANCH columns for alignment
	targetWidth := widths.Workspace + 2 + widths.Branch
	padding := ""
	if visibleWidth < targetWidth {
		padding = strings.Repeat(" ", targetWidth-visibleWidth)
	}

	// Build row manually with proper alignment
	line := prefix + moreTextStyled + padding

	if _, err := fmt.Fprintln(w, line); err != nil {
		return err
	}

	return nil
}

// renderStatusCell renders the status with icon and color.
func (t *HierarchicalStatusTable) renderStatusCell(status constants.TaskStatus, width int) string {
	icon := TaskStatusIcon(status)
	color := TaskStatusColors()[status]
	style := lipgloss.NewStyle().Foreground(color)

	plainText := icon + " " + string(status)
	styledText := icon + " " + style.Render(string(status))

	plainWidth := utf8.RuneCountInString(plainText)
	if plainWidth >= width {
		return styledText
	}
	return styledText + strings.Repeat(" ", width-plainWidth)
}

// HierarchicalJSONWorkspace is the JSON representation of a workspace with tasks.
type HierarchicalJSONWorkspace struct {
	Name       string                 `json:"name"`
	Branch     string                 `json:"branch"`
	Status     string                 `json:"status"`
	Tasks      []HierarchicalJSONTask `json:"tasks"`
	TotalTasks int                    `json:"total_tasks"`
}

// HierarchicalJSONTask is the JSON representation of a task.
type HierarchicalJSONTask struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Step     string `json:"step"`
	Template string `json:"template"`
}
