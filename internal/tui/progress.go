// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"

	"github.com/mrz1836/atlas/internal/constants"
)

// ProgressBar wraps the charmbracelet/bubbles progress bar with ATLAS styling.
// Supports adaptive width and NO_COLOR compatibility.
type ProgressBar struct {
	bar   progress.Model
	width int
}

// ProgressOption is a functional option for configuring a ProgressBar.
type ProgressOption func(*ProgressBar)

// WithWidth sets the progress bar width.
func WithWidth(w int) ProgressOption {
	return func(pb *ProgressBar) {
		pb.width = w
		pb.bar.Width = w
	}
}

// NewProgressBar creates a new progress bar with ATLAS branding.
// Uses ColorPrimary gradient for styled rendering, solid fill for NO_COLOR mode.
func NewProgressBar(width int, opts ...ProgressOption) *ProgressBar {
	var bar progress.Model

	if HasColorSupport() {
		// Use ATLAS branding gradient (ColorPrimary light → dark)
		bar = progress.New(
			progress.WithWidth(width),
			progress.WithScaledGradient("#0087AF", "#00D7FF"), // Match ColorPrimary
		)
	} else {
		// NO_COLOR mode: use solid fill
		bar = progress.New(
			progress.WithWidth(width),
			progress.WithSolidFill("#808080"),
		)
	}

	pb := &ProgressBar{
		bar:   bar,
		width: width,
	}

	// Apply options
	for _, opt := range opts {
		opt(pb)
	}

	return pb
}

// Render returns the progress bar as a string for the given percentage (0.0-1.0).
// Uses ViewAs for static rendering (no animation).
func (pb *ProgressBar) Render(percent float64) string {
	// Clamp percent to valid range
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	return pb.bar.ViewAs(percent)
}

// Width returns the current width of the progress bar.
func (pb *ProgressBar) Width() int {
	return pb.width
}

// SetWidth updates the progress bar width.
func (pb *ProgressBar) SetWidth(w int) {
	pb.width = w
	pb.bar.Width = w
}

// StepProgress holds step progress information for display.
type StepProgress struct {
	Current  int
	Total    int
	StepName string
}

// FormatStepCounter formats step progress as "current/total" (e.g., "3/7").
func FormatStepCounter(current, total int) string {
	return fmt.Sprintf("%d/%d", current, total)
}

// FormatStepWithName formats step progress with name as "current/total name" (e.g., "3/7 Validating").
func FormatStepWithName(current, total int, name string) string {
	if name == "" {
		return FormatStepCounter(current, total)
	}
	return fmt.Sprintf("%d/%d %s", current, total, name)
}

// StepNameLookup maps step types to human-readable names.
// This is a package-level function to allow customization.
//
//nolint:gochecknoglobals // Intentional package-level function for step name mapping
var StepNameLookup = defaultStepNameLookup

// truncateToRuneWidth truncates a string to at most maxWidth runes.
// Returns the truncated string without any suffix (caller adds ellipsis if needed).
func truncateToRuneWidth(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth])
}

// defaultStepNameLookup provides default step type to name mappings.
// Handles both step types ("ai", "validation") and status values ("running", "validating").
func defaultStepNameLookup(stepType string) string {
	names := map[string]string{
		// Step types
		"ai":         "AI Processing",
		"validation": "Validating",
		"git":        "Git Operations",
		"github":     "GitHub",
		"ci":         "CI/CD",
		"approval":   "Awaiting Approval",
		"complete":   "Complete",
		// Status values (for when called with task status)
		"running":           "Running",
		"validating":        "Validating",
		"validation_failed": "Validation Failed",
		"awaiting_approval": "Awaiting Approval",
		"completed":         "Complete",
		"pending":           "Pending",
		"rejected":          "Rejected",
		"abandoned":         "Abandoned",
		"gh_failed":         "GitHub Failed",
		"ci_failed":         "CI Failed",
		"ci_timeout":        "CI Timeout",
	}
	if name, ok := names[stepType]; ok {
		return name
	}
	return stepType // Return raw type if no mapping
}

// DensityMode determines how progress rows are displayed.
type DensityMode int

// Density mode constants.
const (
	// DensityExpanded uses 2-line mode with more details (for ≤5 tasks).
	DensityExpanded DensityMode = iota
	// DensityCompact uses 1-line mode for many tasks (for >5 tasks).
	DensityCompact
)

// DensityThreshold is the task count that triggers compact mode.
const DensityThreshold = 5

// DetermineMode returns the appropriate density mode based on task count.
// Returns DensityExpanded for ≤5 tasks, DensityCompact for >5 tasks.
func DetermineMode(taskCount int) DensityMode {
	if taskCount <= DensityThreshold {
		return DensityExpanded
	}
	return DensityCompact
}

// ProgressRow represents a single row in the progress dashboard.
type ProgressRow struct {
	Name        string  // Workspace/task name
	Percent     float64 // Progress percentage (0.0-1.0)
	CurrentStep int     // Current step number
	TotalSteps  int     // Total steps
	StepName    string  // Optional: current step name
	Duration    string  // Optional: elapsed time
}

// BuildProgressRowsFromStatus converts status rows to progress rows for the dashboard.
// Only includes rows with active tasks (running or validating states).
// This is a shared helper used by both CLI status command and watch mode.
func BuildProgressRowsFromStatus(rows []StatusRow) []ProgressRow {
	progressRows := make([]ProgressRow, 0, len(rows))

	for _, row := range rows {
		// Only show progress for active tasks
		if row.Status != constants.TaskStatusRunning &&
			row.Status != constants.TaskStatusValidating {
			continue
		}

		// Calculate progress percentage
		var percent float64
		if row.TotalSteps > 0 {
			percent = float64(row.CurrentStep) / float64(row.TotalSteps)
		}

		progressRows = append(progressRows, ProgressRow{
			Name:        row.Workspace,
			Percent:     percent,
			CurrentStep: row.CurrentStep,
			TotalSteps:  row.TotalSteps,
			StepName:    StepNameLookup(string(row.Status)),
		})
	}

	return progressRows
}

// ProgressRowCompact renders a progress row in 1-line compact mode.
// Format: "[████████░░░░] 40% 3/7 workspace-name"
func ProgressRowCompact(row ProgressRow, barWidth int) string {
	bar := NewProgressBar(barWidth)
	barStr := bar.Render(row.Percent)

	percentStr := fmt.Sprintf("%3d%%", int(row.Percent*100))
	stepStr := FormatStepCounter(row.CurrentStep, row.TotalSteps)

	// Truncate step name if too long (using rune width for Unicode support)
	stepName := row.StepName
	if runeWidth(stepName) > 10 {
		stepName = truncateToRuneWidth(stepName, 9) + "…"
	}

	if stepName != "" {
		return fmt.Sprintf("%s %s %s %s %s", barStr, percentStr, stepStr, stepName, row.Name)
	}
	return fmt.Sprintf("%s %s %s %s", barStr, percentStr, stepStr, row.Name)
}

// ProgressRowExpanded renders a progress row in 2-line expanded mode.
// Line 1: "name         [████████████░░░░░░░░░░░░░░░░░░] 40%"
// Line 2: "             Step 3/7: Validating • 2m 15s"
func ProgressRowExpanded(row ProgressRow, barWidth, nameWidth int) string {
	bar := NewProgressBar(barWidth)
	barStr := bar.Render(row.Percent)

	percentStr := fmt.Sprintf("%3d%%", int(row.Percent*100))

	// Pad or truncate name (using rune width for Unicode support)
	name := row.Name
	nameVisualWidth := runeWidth(name)
	if nameVisualWidth > nameWidth {
		name = truncateToRuneWidth(name, nameWidth-1) + "…"
		nameVisualWidth = nameWidth
	}
	// Pad to nameWidth using visual width
	padding := nameWidth - nameVisualWidth
	namePadded := name + strings.Repeat(" ", padding)

	// Line 1: name + bar + percent
	line1 := fmt.Sprintf("%s %s %s", namePadded, barStr, percentStr)

	// Line 2: step info (indented to match name width)
	indent := strings.Repeat(" ", nameWidth+1)
	stepInfo := fmt.Sprintf("Step %s", FormatStepWithName(row.CurrentStep, row.TotalSteps, row.StepName))
	if row.Duration != "" {
		stepInfo += " • " + row.Duration
	}
	line2 := indent + lipgloss.NewStyle().Foreground(ColorMuted).Render(stepInfo)

	return line1 + "\n" + line2
}

// ProgressDashboard renders multiple progress rows with auto-density mode.
type ProgressDashboard struct {
	rows           []ProgressRow
	width          int
	mode           DensityMode
	autoAdjustMode bool
}

// DashboardOption is a functional option for configuring ProgressDashboard.
type DashboardOption func(*ProgressDashboard)

// WithTermWidth sets the terminal width for the dashboard.
func WithTermWidth(width int) DashboardOption {
	return func(pd *ProgressDashboard) {
		pd.width = width
	}
}

// WithDensityMode sets a specific density mode (overrides auto-detection).
func WithDensityMode(mode DensityMode) DashboardOption {
	return func(pd *ProgressDashboard) {
		pd.mode = mode
		pd.autoAdjustMode = false
	}
}

// NewProgressDashboard creates a new progress dashboard with the given rows.
// Auto-detects density mode based on row count unless overridden.
func NewProgressDashboard(rows []ProgressRow, opts ...DashboardOption) *ProgressDashboard {
	pd := &ProgressDashboard{
		rows:           rows,
		width:          80, // Default width
		mode:           DetermineMode(len(rows)),
		autoAdjustMode: true,
	}

	// Apply options
	for _, opt := range opts {
		opt(pd)
	}

	// Auto-adjust mode based on row count if not manually set
	if pd.autoAdjustMode {
		pd.mode = DetermineMode(len(pd.rows))
	}

	return pd
}

// Render writes the progress dashboard to the writer.
func (pd *ProgressDashboard) Render(w io.Writer) error {
	if len(pd.rows) == 0 {
		return nil
	}

	// Calculate bar width based on terminal width and mode
	barWidth := pd.calculateBarWidth()
	nameWidth := pd.calculateNameWidth()

	for i, row := range pd.rows {
		var line string
		if pd.mode == DensityCompact {
			line = ProgressRowCompact(row, barWidth)
		} else {
			line = ProgressRowExpanded(row, barWidth, nameWidth)
		}

		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}

		// Add blank line between expanded rows (except last)
		if pd.mode == DensityExpanded && i < len(pd.rows)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}

	return nil
}

// Mode returns the current density mode.
func (pd *ProgressDashboard) Mode() DensityMode {
	return pd.mode
}

// Rows returns a copy of the progress rows.
func (pd *ProgressDashboard) Rows() []ProgressRow {
	if pd.rows == nil {
		return nil
	}
	result := make([]ProgressRow, len(pd.rows))
	copy(result, pd.rows)
	return result
}

// calculateBarWidth determines progress bar width based on terminal width.
func (pd *ProgressDashboard) calculateBarWidth() int {
	// Reserve space for: percent (4), step (5), name (12+), spacing (3)
	// Minimum bar width: 20
	// Standard (80-120): 40
	// Wide (>120): 60+

	switch {
	case pd.width < 80:
		return 20
	case pd.width >= 80 && pd.width < 120:
		return 40
	default:
		return 60
	}
}

// calculateNameWidth determines name column width for expanded mode.
func (pd *ProgressDashboard) calculateNameWidth() int {
	// Find max name length, cap at reasonable value
	maxLen := 12 // Minimum
	for _, row := range pd.rows {
		if len(row.Name) > maxLen {
			maxLen = len(row.Name)
		}
	}

	// Cap at terminal width constraints
	maxAllowed := (pd.width - 50) / 3 // Reserve 2/3 for bar+details
	if maxAllowed < 12 {
		maxAllowed = 12
	}
	if maxLen > maxAllowed {
		return maxAllowed
	}
	return maxLen
}
