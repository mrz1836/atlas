// Package tui provides terminal user interface components for ATLAS.
//
// This file provides the approval summary component for displaying task context
// before user approval decisions. It shows task details, file changes, validation
// status, and PR information.
//
// # Approval Summary (AC: #1-#5)
//
// The ApprovalSummary struct aggregates all relevant information for an approval decision:
//   - Task status, step progress, and description
//   - Workspace and branch information
//   - PR URL with optional OSC 8 hyperlink support
//   - File changes with insertion/deletion counts
//   - Validation pass/fail summary
//
// # OSC 8 Hyperlinks (AC: #2, UX-3)
//
// PR URLs can be rendered as clickable hyperlinks in supported terminals using
// OSC 8 escape sequences. Use SupportsHyperlinks() to check terminal support
// and FormatHyperlink() to format URLs.
//
// # Terminal Width Adaptation (AC: #1)
//
// The component adapts to terminal width using adaptWidth() from menus.go:
//   - Compact mode (<80 cols): Abbreviated labels, truncated paths
//   - Standard mode (80-119 cols): Normal display
//   - Expanded mode (>=120 cols): Full paths, additional details
package tui

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// ApprovalSummary holds all information needed for an approval decision (AC: #1, #5).
// It aggregates task, workspace, and validation data into a single display-ready structure.
type ApprovalSummary struct {
	// TaskID is the unique identifier for the task.
	TaskID string

	// WorkspaceName is the name of the workspace containing this task.
	WorkspaceName string

	// Status is the current task status.
	Status constants.TaskStatus

	// CurrentStep is the current step number (1-based for display).
	CurrentStep int

	// TotalSteps is the total number of steps in the task.
	TotalSteps int

	// Description is the human-readable task description.
	Description string

	// BranchName is the git branch for this task's workspace.
	BranchName string

	// PRURL is the pull request URL if available (AC: #2).
	PRURL string

	// FileChanges lists all files modified by this task (AC: #3).
	FileChanges []FileChange

	// TotalInsertions is the sum of all insertions across files.
	TotalInsertions int

	// TotalDeletions is the sum of all deletions across files.
	TotalDeletions int

	// Validation holds the validation summary if validation has run (AC: #4).
	Validation *ValidationSummary
}

// FileChange represents a single file modification (AC: #3).
type FileChange struct {
	// Path is the file path relative to the repository root.
	Path string

	// Insertions is the number of lines added.
	Insertions int

	// Deletions is the number of lines removed.
	Deletions int
}

// ValidationSummary holds validation results (AC: #4).
type ValidationSummary struct {
	// PassCount is the number of passing validations.
	PassCount int

	// FailCount is the number of failing validations.
	FailCount int

	// Status is the overall validation status ("passed", "failed", "pending").
	Status string

	// LastRunAt is when validation was last executed.
	LastRunAt *time.Time
}

// NewApprovalSummary creates an ApprovalSummary from a task and workspace (AC: #1, #5).
// Returns nil if task is nil. Workspace can be nil, resulting in empty workspace fields.
func NewApprovalSummary(task *domain.Task, workspace *domain.Workspace) *ApprovalSummary {
	if task == nil {
		return nil
	}

	summary := &ApprovalSummary{
		TaskID:      task.ID,
		Status:      task.Status,
		CurrentStep: task.CurrentStep + 1, // Convert 0-based to 1-based for display
		TotalSteps:  len(task.Steps),
		Description: task.Description,
	}

	// Extract workspace info if available
	if workspace != nil {
		summary.WorkspaceName = workspace.Name
		summary.BranchName = workspace.Branch
	}

	// Extract PR URL from task metadata (AC: #2)
	if task.Metadata != nil {
		if prURL, ok := task.Metadata["pr_url"].(string); ok {
			summary.PRURL = prURL
		}
	}

	// Collect file changes from step results (AC: #3)
	summary.collectFileChanges(task.StepResults)

	// Extract validation status (AC: #4)
	summary.extractValidationStatus(task.StepResults)

	return summary
}

// SetFileStats updates file change statistics from git diff data.
// Call this after NewApprovalSummary() if diff stats are available.
// The stats map keys should match the paths in FileChanges.
func (s *ApprovalSummary) SetFileStats(stats map[string]FileChange) {
	for i, fc := range s.FileChanges {
		if stat, ok := stats[fc.Path]; ok {
			s.FileChanges[i].Insertions = stat.Insertions
			s.FileChanges[i].Deletions = stat.Deletions
			s.TotalInsertions += stat.Insertions
			s.TotalDeletions += stat.Deletions
		}
	}
}

// collectFileChanges aggregates file changes from all step results (AC: #3).
// domain.StepResult.FilesChanged only contains paths, not diff stats.
// Insertions/Deletions can be set via SetFileStats() if git diff data is available.
func (s *ApprovalSummary) collectFileChanges(results []domain.StepResult) {
	// Use a map to deduplicate files
	seen := make(map[string]bool)

	for _, result := range results {
		for _, path := range result.FilesChanged {
			if !seen[path] {
				seen[path] = true
				s.FileChanges = append(s.FileChanges, FileChange{
					Path:       path,
					Insertions: 0,
					Deletions:  0,
				})
			}
		}
	}
}

// extractValidationStatus finds and parses validation results from step results (AC: #4).
func (s *ApprovalSummary) extractValidationStatus(results []domain.StepResult) {
	for _, result := range results {
		// Look for validation step by name
		if strings.Contains(strings.ToLower(result.StepName), "validate") {
			completedAt := result.CompletedAt
			s.Validation = &ValidationSummary{
				Status:    normalizeValidationStatus(result.Status),
				LastRunAt: &completedAt,
			}

			// Set pass/fail counts based on status
			switch result.Status {
			case "success":
				s.Validation.PassCount = 1
				s.Validation.FailCount = 0
			case "failed":
				s.Validation.PassCount = 0
				s.Validation.FailCount = 1
			}

			return // Use first validation result found
		}
	}
}

// normalizeValidationStatus converts step status to validation status string.
func normalizeValidationStatus(status string) string {
	switch status {
	case "success":
		return "passed"
	case "failed":
		return "failed"
	default:
		return "pending"
	}
}

// SupportsHyperlinks returns true if the terminal supports OSC 8 hyperlinks (AC: #2, UX-3).
// Detection is based on known terminal programs that support the feature.
func SupportsHyperlinks() bool {
	// Check for terminals known to support hyperlinks
	termProgram := os.Getenv("TERM_PROGRAM")
	lcTerminal := os.Getenv("LC_TERMINAL")

	// Known good terminals
	if termProgram == "iTerm.app" || termProgram == "vscode" {
		return true
	}
	if lcTerminal == "iTerm2" {
		return true
	}

	// macOS Terminal.app versions vary - safer to use underline fallback
	return false
}

// FormatHyperlink formats a URL as an OSC 8 hyperlink if supported (AC: #2).
// Falls back to underlined text if hyperlinks are not supported.
//
// OSC 8 format: \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\
func FormatHyperlink(url, displayText string) string {
	if !SupportsHyperlinks() {
		// Fallback: return underlined text (caller should apply underline style)
		return displayText
	}

	// OSC 8 escape sequence for hyperlinks
	// Format: ESC ] 8 ; ; URL ESC \ TEXT ESC ] 8 ; ; ESC \
	return "\x1b]8;;" + url + "\x1b\\" + displayText + "\x1b]8;;\x1b\\"
}

// RenderApprovalSummary renders the approval summary using the BoxStyle (AC: #1, #2, #3, #4).
// Uses semantic colors from styles.go and adapts to terminal width.
func RenderApprovalSummary(summary *ApprovalSummary) string {
	return RenderApprovalSummaryWithWidth(summary, 0)
}

// displayMode represents the terminal width display mode.
type displayMode int

const (
	displayModeCompact  displayMode = iota // < 80 columns
	displayModeStandard                    // 80-119 columns
	displayModeExpanded                    // >= 120 columns
)

// getDisplayMode determines the display mode based on terminal width.
func getDisplayMode(width int) displayMode {
	if width < 80 {
		return displayModeCompact
	}
	if width >= 120 {
		return displayModeExpanded
	}
	return displayModeStandard
}

// RenderApprovalSummaryWithWidth renders the approval summary at a specific width (AC: #5).
// Width of 0 means auto-detect from terminal.
// Display modes:
//   - Compact (<80 cols): Abbreviated labels, truncated paths, max 3 files
//   - Standard (80-119 cols): Normal display, max 5 files
//   - Expanded (>=120 cols): Full paths, per-file stats, max 10 files
func RenderApprovalSummaryWithWidth(summary *ApprovalSummary, width int) string {
	if summary == nil {
		return ""
	}

	// Respect NO_COLOR
	CheckNoColor()

	// Determine width
	if width <= 0 {
		width = adaptWidth(DefaultBoxWidth)
	}

	// Determine display mode based on width
	mode := getDisplayMode(width)

	// Build content sections
	var content strings.Builder

	// Add empty line for padding
	content.WriteString("\n")

	// Task info section (AC: #1)
	content.WriteString(renderInfoLineWithMode("Workspace", summary.WorkspaceName, width, mode))
	content.WriteString(renderInfoLineWithMode("Branch", summary.BranchName, width, mode))
	content.WriteString(renderStatusLine(summary.Status, width))
	content.WriteString(renderProgressLine(summary.CurrentStep, summary.TotalSteps, width))

	// Show description in expanded mode
	if mode == displayModeExpanded && summary.Description != "" {
		content.WriteString(renderInfoLineWithMode("Task", summary.Description, width, mode))
	}
	content.WriteString("\n")

	// PR section if available (AC: #2)
	if summary.PRURL != "" {
		prDisplay := extractPRNumber(summary.PRURL)
		// In expanded mode, show full URL
		if mode == displayModeExpanded {
			prDisplay = prDisplay + " (" + summary.PRURL + ")"
		}
		prText := FormatHyperlink(summary.PRURL, prDisplay)
		if !SupportsHyperlinks() {
			prText = StyleUnderline.Render(prDisplay)
		}
		content.WriteString(renderInfoLineWithMode("PR", prText, width, mode))
		content.WriteString("\n")
	}

	// File changes section (AC: #3)
	if len(summary.FileChanges) > 0 {
		content.WriteString(renderFileChangesSectionWithMode(summary, width, mode))
		content.WriteString("\n")
	}

	// Validation section (AC: #4)
	if summary.Validation != nil {
		content.WriteString(renderValidationSection(summary.Validation, width))
		content.WriteString("\n")
	}

	// Render using BoxStyle
	box := NewBoxStyle().WithWidth(width)
	return box.Render("Approval Summary", content.String())
}

// renderInfoLineWithMode renders a labeled info line based on display mode.
func renderInfoLineWithMode(label, value string, width int, mode displayMode) string {
	switch mode {
	case displayModeCompact:
		// Abbreviated format for narrow terminals
		return "  " + abbreviateLabel(label) + ": " + truncateString(value, width-15) + "\n"
	case displayModeExpanded:
		// Expanded format with more space for labels
		return "  " + padRight(label+":", 14) + value + "\n"
	case displayModeStandard:
		// Standard format with fixed label width for alignment
		return "  " + padRight(label+":", 12) + value + "\n"
	}
	return ""
}

// renderStatusLine renders the status with appropriate icon and color.
func renderStatusLine(status constants.TaskStatus, _ int) string {
	icon := TaskStatusIcon(status)
	statusText := string(status)

	// Apply color based on status
	styledStatus := statusText
	if HasColorSupport() {
		colors := TaskStatusColors()
		if color, ok := colors[status]; ok {
			styledStatus = lipgloss.NewStyle().Foreground(color).Render(statusText)
		}
	}

	return "  " + padRight("Status:", 12) + icon + " " + styledStatus + "\n"
}

// renderProgressLine renders step progress.
func renderProgressLine(current, total, _ int) string {
	progressText := strings.Builder{}
	progressText.WriteString("Step ")
	progressText.WriteString(strings.TrimSpace(padRight(intToString(current), 2)))
	progressText.WriteString("/")
	progressText.WriteString(intToString(total))

	return "  " + padRight("Progress:", 12) + progressText.String() + "\n"
}

// renderFileChangesSectionWithMode renders the file changes based on display mode (AC: #3).
func renderFileChangesSectionWithMode(summary *ApprovalSummary, width int, mode displayMode) string {
	var result strings.Builder

	result.WriteString("  Files Changed:\n")
	result.WriteString(renderTotalStats(summary.TotalInsertions, summary.TotalDeletions))

	maxFiles := maxFilesForMode(mode)
	for i, fc := range summary.FileChanges {
		if i >= maxFiles {
			remaining := len(summary.FileChanges) - maxFiles
			result.WriteString("    ... and " + intToString(remaining) + " more files\n")
			break
		}
		result.WriteString(renderFileChangeLine(fc, width, mode))
	}

	return result.String()
}

// maxFilesForMode returns the maximum number of files to display for a given mode.
func maxFilesForMode(mode displayMode) int {
	switch mode {
	case displayModeCompact:
		return 3
	case displayModeExpanded:
		return 10
	case displayModeStandard:
		return 5
	}
	return 5
}

// renderTotalStats renders the total insertions/deletions stats line.
func renderTotalStats(insertions, deletions int) string {
	if insertions == 0 && deletions == 0 {
		return ""
	}
	statsLine := "    "
	if HasColorSupport() {
		statsLine += lipgloss.NewStyle().Foreground(ColorSuccess).Render("+" + intToString(insertions))
		statsLine += "  "
		statsLine += lipgloss.NewStyle().Foreground(ColorError).Render("-" + intToString(deletions))
	} else {
		statsLine += "+" + intToString(insertions) + "  -" + intToString(deletions)
	}
	return statsLine + "  total\n"
}

// renderFileChangeLine renders a single file change line based on display mode.
func renderFileChangeLine(fc FileChange, width int, mode displayMode) string {
	path := fc.Path
	switch mode {
	case displayModeCompact:
		return "    " + truncatePath(path, width-10) + "\n"
	case displayModeExpanded:
		if fc.Insertions > 0 || fc.Deletions > 0 {
			stats := renderFileStats(fc.Insertions, fc.Deletions)
			return "    " + padRight(stats, 12) + path + "\n"
		}
		return "    " + path + "\n"
	case displayModeStandard:
		return "    " + path + "\n"
	}
	return "    " + path + "\n"
}

// renderFileStats renders file-level insertion/deletion stats.
func renderFileStats(insertions, deletions int) string {
	if HasColorSupport() {
		return lipgloss.NewStyle().Foreground(ColorSuccess).Render("+"+intToString(insertions)) +
			" " +
			lipgloss.NewStyle().Foreground(ColorError).Render("-"+intToString(deletions))
	}
	return "+" + intToString(insertions) + " -" + intToString(deletions)
}

// renderValidationSection renders the validation status (AC: #4).
func renderValidationSection(validation *ValidationSummary, _ int) string {
	var result strings.Builder

	icon := "○"
	statusText := validation.Status

	switch validation.Status {
	case "passed":
		icon = "✓"
		if HasColorSupport() {
			statusText = lipgloss.NewStyle().Foreground(ColorSuccess).Render(statusText)
		}
	case "failed":
		icon = "✗"
		if HasColorSupport() {
			statusText = lipgloss.NewStyle().Foreground(ColorError).Render(statusText)
		}
	}

	passFailText := ""
	if validation.PassCount > 0 || validation.FailCount > 0 {
		passFailText = " " + intToString(validation.PassCount) + "/" + intToString(validation.PassCount+validation.FailCount)
	}

	result.WriteString("  " + padRight("Validation:", 12) + icon + " " + statusText + passFailText + "\n")

	return result.String()
}

// Helper functions

// abbreviateLabel shortens labels for compact mode.
func abbreviateLabel(label string) string {
	abbreviations := map[string]string{
		"Workspace":  "WS",
		"Branch":     "Br",
		"Status":     "St",
		"Progress":   "Pr",
		"Validation": "Val",
	}
	if abbr, ok := abbreviations[label]; ok {
		return abbr
	}
	return label
}

// truncateString truncates a string to maxLen, adding ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// truncatePath truncates a file path, preserving the filename.
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Find the last path separator
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		return truncateString(path, maxLen)
	}

	filename := path[lastSlash+1:]
	if len(filename) >= maxLen-3 {
		return truncateString(filename, maxLen)
	}

	// Truncate directory part, keep filename
	availableForDir := maxLen - len(filename) - 4 // 4 for ".../"
	if availableForDir <= 0 {
		return truncateString(filename, maxLen)
	}

	return "..." + path[lastSlash-availableForDir:lastSlash] + "/" + filename
}

// intToString converts an integer to a string using strconv.
func intToString(n int) string {
	return strconv.Itoa(n)
}

// extractPRNumber extracts the PR number from a GitHub PR URL.
func extractPRNumber(url string) string {
	// Look for /pull/NUMBER pattern
	if idx := strings.LastIndex(url, "/pull/"); idx != -1 {
		prNum := url[idx+6:]
		// Remove any trailing content (like /files, etc.)
		if slashIdx := strings.Index(prNum, "/"); slashIdx != -1 {
			prNum = prNum[:slashIdx]
		}
		return "#" + prNum
	}
	// Fallback: return the full URL
	return url
}
