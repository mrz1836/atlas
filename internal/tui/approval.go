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

	// InterruptionCount is the number of times the task was interrupted (Ctrl+C).
	InterruptionCount int

	// WasPaused indicates if the task was paused at any point during execution.
	WasPaused bool
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

// ValidationCheck represents a single validation check result.
type ValidationCheck struct {
	// Name is the category name (e.g., "Format", "Lint", "Test", "Pre-commit", "CI").
	Name string

	// Passed indicates if this check passed.
	Passed bool

	// Skipped indicates if this check was skipped (not run).
	Skipped bool
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

	// Checks holds individual validation check results for verbose display.
	Checks []ValidationCheck

	// AIRetryCount is the number of AI retry attempts used (0 if no retries).
	AIRetryCount int
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

	// Extract CI status and add to validation checks
	summary.extractCIStatus(task.StepResults)

	// Count interruptions from task transitions
	summary.InterruptionCount = countInterruptions(task.Transitions)
	summary.WasPaused = summary.InterruptionCount > 0

	return summary
}

// countInterruptions counts how many times a task was interrupted (Ctrl+C).
// It counts transitions where the task moved to "interrupted" status.
func countInterruptions(transitions []domain.Transition) int {
	count := 0
	for _, t := range transitions {
		if t.ToStatus == constants.TaskStatusInterrupted {
			count++
		}
	}
	return count
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
// Priority: successful validation > latest failed validation > step name fallback.
// Iterates in reverse order to prefer latest results when a step runs multiple times
// (e.g., after interruptions and resumes).
func (s *ApprovalSummary) extractValidationStatus(results []domain.StepResult) {
	// Iterate in reverse to prefer latest results
	// Track latest failed result as fallback if no success found
	var latestFailed *domain.StepResult

	for i := len(results) - 1; i >= 0; i-- {
		result := results[i]
		if !hasValidationMetadata(result.Metadata) {
			continue
		}
		// Prefer successful result - return immediately
		if result.Status == "success" {
			s.Validation = buildValidationSummary(result)
			return
		}
		// Track latest failed as fallback (first one found in reverse = latest)
		if latestFailed == nil {
			latestFailed = &results[i]
		}
	}

	// Use latest failed if no success found
	if latestFailed != nil {
		s.Validation = buildValidationSummary(*latestFailed)
		return
	}

	// Fallback: look by step name (for backwards compatibility), also in reverse
	for i := len(results) - 1; i >= 0; i-- {
		if isValidationStep(results[i].StepName) {
			s.Validation = buildValidationSummary(results[i])
			return
		}
	}
}

// isValidationStep checks if a step name indicates a validation step.
func isValidationStep(stepName string) bool {
	return strings.Contains(strings.ToLower(stepName), "validate")
}

// hasValidationMetadata checks if a step has actual validation results in metadata.
// This is a more reliable indicator than step name, since validation can run
// in steps with different names (e.g., "detect" vs "validate").
func hasValidationMetadata(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	_, hasChecks := metadata["validation_checks"]
	return hasChecks
}

// buildValidationSummary creates a ValidationSummary from a step result.
func buildValidationSummary(result domain.StepResult) *ValidationSummary {
	completedAt := result.CompletedAt
	vs := &ValidationSummary{
		Status:    normalizeValidationStatus(result.Status),
		LastRunAt: &completedAt,
	}

	// Try to extract individual validation checks from metadata
	vs.Checks = extractChecksFromMetadata(result.Metadata)

	// Set pass/fail counts
	if len(vs.Checks) > 0 {
		vs.PassCount, vs.FailCount, _ = countValidationChecks(vs.Checks)
	} else {
		vs.PassCount, vs.FailCount = legacyPassFailCounts(result.Status)
	}

	// Extract AI retry attempt count from metadata
	vs.AIRetryCount = extractRetryAttempt(result.Metadata)

	return vs
}

// extractRetryAttempt extracts the AI retry attempt count from step metadata.
// Returns 0 if no retry was performed.
func extractRetryAttempt(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	// Try int first (direct assignment)
	if attempt, ok := metadata["retry_attempt"].(int); ok {
		return attempt
	}
	// Try float64 (from JSON deserialization)
	if attempt, ok := metadata["retry_attempt"].(float64); ok {
		return int(attempt)
	}
	return 0
}

// extractChecksFromMetadata extracts validation checks from step metadata.
func extractChecksFromMetadata(metadata map[string]any) []ValidationCheck {
	if metadata == nil {
		return nil
	}
	checksData, ok := metadata["validation_checks"]
	if !ok {
		return nil
	}
	return parseValidationChecks(checksData)
}

// countValidationChecks counts passed, failed, and skipped checks in a single iteration.
func countValidationChecks(checks []ValidationCheck) (passCount, failCount, skipCount int) {
	for _, check := range checks {
		switch {
		case check.Skipped:
			skipCount++
		case check.Passed:
			passCount++
		default:
			failCount++
		}
	}
	return passCount, failCount, skipCount
}

// legacyPassFailCounts returns pass/fail counts based on overall status.
func legacyPassFailCounts(status string) (passCount, failCount int) {
	if status == "success" {
		return 1, 0
	}
	if status == "failed" {
		return 0, 1
	}
	return 0, 0
}

// extractCIStatus finds CI step results and adds CI check to validation checks.
func (s *ApprovalSummary) extractCIStatus(results []domain.StepResult) {
	for _, result := range results {
		if !isCIStep(result.StepName) {
			continue
		}

		s.ensureValidationSummary(result)
		s.addCICheck(result)
		return // Use first CI result found
	}
}

// isCIStep checks if a step name indicates a CI step.
func isCIStep(stepName string) bool {
	stepNameLower := strings.ToLower(stepName)
	return strings.Contains(stepNameLower, "ci") || strings.Contains(stepNameLower, "checks")
}

// ensureValidationSummary creates a validation summary if it doesn't exist.
func (s *ApprovalSummary) ensureValidationSummary(result domain.StepResult) {
	if s.Validation != nil {
		return
	}
	completedAt := result.CompletedAt
	s.Validation = &ValidationSummary{
		Status:    normalizeValidationStatus(result.Status),
		LastRunAt: &completedAt,
	}
}

// addCICheck adds the CI check to the validation summary and updates counts.
func (s *ApprovalSummary) addCICheck(result domain.StepResult) {
	ciCheck := ValidationCheck{Name: "CI"}

	// Detect status
	switch result.Status {
	case "skipped":
		ciCheck.Skipped = true
		// Don't increment pass/fail counts for skipped checks
		// Don't change overall validation status
	case "success":
		ciCheck.Passed = true
		s.Validation.PassCount++
	default:
		ciCheck.Passed = false
		s.Validation.FailCount++
		// Update overall status if CI failed
		if s.Validation.Status == "passed" {
			s.Validation.Status = "failed"
		}
	}

	s.Validation.Checks = append(s.Validation.Checks, ciCheck)
}

// parseValidationChecks converts metadata validation checks to ValidationCheck slice.
func parseValidationChecks(data any) []ValidationCheck {
	checksSlice, ok := data.([]any)
	if !ok {
		// Try typed slice (from direct struct assignment)
		if typedSlice, ok := data.([]map[string]any); ok {
			checks := make([]ValidationCheck, 0, len(typedSlice))
			for _, checkMap := range typedSlice {
				check := parseCheckMap(checkMap)
				if check.Name != "" {
					checks = append(checks, check)
				}
			}
			return checks
		}
		return nil
	}

	checks := make([]ValidationCheck, 0, len(checksSlice))
	for _, item := range checksSlice {
		checkMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		check := parseCheckMap(checkMap)
		if check.Name != "" {
			checks = append(checks, check)
		}
	}
	return checks
}

// parseCheckMap extracts a ValidationCheck from a map.
func parseCheckMap(checkMap map[string]any) ValidationCheck {
	check := ValidationCheck{}
	if name, ok := checkMap["name"].(string); ok {
		check.Name = name
	}
	if passed, ok := checkMap["passed"].(bool); ok {
		check.Passed = passed
	}
	if skipped, ok := checkMap["skipped"].(bool); ok {
		check.Skipped = skipped
	}
	return check
}

// normalizeValidationStatus converts step status to validation status string.
func normalizeValidationStatus(status string) string {
	switch status {
	case "success":
		return "passed"
	case "failed":
		return "failed"
	case "skipped":
		return "skipped"
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
	return RenderApprovalSummaryWithWidth(summary, 0, false)
}

// displayMode represents the terminal width display mode.
type displayMode int

const (
	displayModeCompact  displayMode = iota // < TerminalWidthNarrow columns
	displayModeStandard                    // TerminalWidthNarrow to TerminalWidthWide-1 columns
	displayModeExpanded                    // >= TerminalWidthWide columns
)

// getDisplayMode determines the display mode based on terminal width.
func getDisplayMode(width int) displayMode {
	if width < TerminalWidthNarrow {
		return displayModeCompact
	}
	if width >= TerminalWidthWide {
		return displayModeExpanded
	}
	return displayModeStandard
}

// RenderApprovalSummaryWithWidth renders the approval summary at a specific width (AC: #5).
// Width of 0 means auto-detect from terminal.
// Verbose mode forces showing validation checks regardless of terminal width.
// Display modes:
//   - Compact (<80 cols): Abbreviated labels, truncated paths, max 3 files
//   - Standard (80-119 cols): Normal display, max 5 files
//   - Expanded (>=120 cols): Full paths, per-file stats, max 10 files
//   - Verbose mode: Always show validation checks even in compact mode
func RenderApprovalSummaryWithWidth(summary *ApprovalSummary, width int, verbose bool) string {
	if summary == nil {
		return ""
	}

	// Respect NO_COLOR
	CheckNoColor()

	// Determine width and display mode
	width, mode, effectiveMode := determineApprovalDisplayMode(width, verbose)

	// Build content sections
	var content strings.Builder

	// Add empty line for padding
	content.WriteString("\n")

	// Task info section (AC: #1)
	content.WriteString(renderInfoLineWithMode("Workspace", summary.WorkspaceName, width, mode))
	content.WriteString(renderInfoLineWithMode("Branch", summary.BranchName, width, mode))
	content.WriteString(renderStatusLine(summary.Status, width))
	// Show session info if task was paused/interrupted
	if summary.WasPaused {
		content.WriteString(renderSessionLine(summary.InterruptionCount, width))
	}
	content.WriteString(renderProgressLine(summary.CurrentStep, summary.TotalSteps, width))

	// Show description in expanded mode
	if mode == displayModeExpanded && summary.Description != "" {
		content.WriteString(renderInfoLineWithMode("Task", summary.Description, width, mode))
	}
	content.WriteString("\n")

	// PR section if available (AC: #2)
	// Uses dedicated renderPRLine to avoid truncating ANSI escape sequences
	if summary.PRURL != "" {
		content.WriteString(renderPRLine(summary.PRURL, mode))
		content.WriteString("\n")
	}

	// File changes section (AC: #3)
	if len(summary.FileChanges) > 0 {
		content.WriteString(renderFileChangesSectionWithMode(summary, width, mode))
		content.WriteString("\n")
	}

	// Validation section (AC: #4)
	if summary.Validation != nil {
		content.WriteString(renderValidationSectionWithMode(summary.Validation, width, effectiveMode))
		content.WriteString("\n")
	}

	// Render using BoxStyle
	box := NewBoxStyle().WithWidth(width)
	return box.Render("Approval Summary", content.String())
}

// determineApprovalDisplayMode determines the width and display modes for approval summary.
// Returns the effective width, display mode, and effective mode (for validation section).
func determineApprovalDisplayMode(width int, verbose bool) (int, displayMode, displayMode) {
	if width <= 0 {
		width = adaptWidth(DefaultBoxWidth)
	}

	mode := getDisplayMode(width)

	// In verbose mode, override compact mode to show validation checks
	effectiveMode := mode
	if verbose && mode == displayModeCompact {
		effectiveMode = displayModeStandard
	}

	return width, mode, effectiveMode
}

// renderInfoLineWithMode renders a labeled info line based on display mode.
func renderInfoLineWithMode(label, value string, width int, mode displayMode) string {
	switch mode {
	case displayModeCompact:
		// Abbreviated format for narrow terminals
		return "  " + abbreviateLabel(label) + ": " + truncateString(value, width-TruncateMargin) + "\n"
	case displayModeExpanded:
		// Expanded format with more space for labels
		return "  " + padRight(label+":", LabelWidthExpanded) + value + "\n"
	case displayModeStandard:
		// Standard format with fixed label width for alignment
		return "  " + padRight(label+":", LabelWidthStandard) + value + "\n"
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

	return "  " + padRight("Status:", LabelWidthStandard) + icon + " " + styledStatus + "\n"
}

// renderProgressLine renders step progress.
func renderProgressLine(current, total, _ int) string {
	progressText := strings.Builder{}
	progressText.WriteString("Step ")
	progressText.WriteString(strings.TrimSpace(padRight(intToString(current), 2)))
	progressText.WriteString("/")
	progressText.WriteString(intToString(total))

	return "  " + padRight("Progress:", LabelWidthStandard) + progressText.String() + "\n"
}

// renderSessionLine renders session info showing interruption count.
// Only shown when task was paused/interrupted at least once.
func renderSessionLine(interruptionCount, _ int) string {
	// Format interruption text with proper pluralization
	interruptText := strconv.Itoa(interruptionCount) + " interruption"
	if interruptionCount != 1 {
		interruptText += "s"
	}
	interruptText += ", resumed"

	// Apply dimmed style if colors are supported
	styledText := interruptText
	if HasColorSupport() {
		styledText = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(interruptText)
	}

	return "  " + padRight("Session:", LabelWidthStandard) + styledText + "\n"
}

// renderPRLine renders the PR link without truncation (PR numbers are inherently short).
// This avoids truncating ANSI escape sequences used for hyperlinks/underlines.
func renderPRLine(prURL string, mode displayMode) string {
	prDisplay := extractPRDisplay(prURL)
	if mode == displayModeExpanded {
		prDisplay = prDisplay + " (" + prURL + ")"
	}

	prText := FormatHyperlink(prURL, prDisplay)
	if !SupportsHyperlinks() {
		prText = StyleUnderline.Render(prDisplay)
	}

	// Format label based on mode (no truncation needed for PR)
	switch mode {
	case displayModeCompact:
		return "  " + abbreviateLabel("PR") + ": " + prText + "\n"
	case displayModeExpanded:
		return "  " + padRight("PR:", LabelWidthExpanded) + prText + "\n"
	case displayModeStandard:
		return "  " + padRight("PR:", LabelWidthStandard) + prText + "\n"
	default:
		return "  " + padRight("PR:", LabelWidthStandard) + prText + "\n"
	}
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
			return "    " + padRight(stats, LabelWidthStandard) + path + "\n"
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

// renderValidationSectionWithMode renders the validation status with individual checks (AC: #4).
func renderValidationSectionWithMode(validation *ValidationSummary, _ int, mode displayMode) string {
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

	// Add skipped count if any checks were skipped
	_, _, skipCount := countValidationChecks(validation.Checks)
	if skipCount > 0 {
		skippedText := " (" + intToString(skipCount) + " skipped)"
		passFailText += skippedText
	}

	result.WriteString("  " + padRight("Validation:", LabelWidthStandard) + icon + " " + statusText + passFailText + "\n")

	// Show AI retry indicator if retries were used
	if validation.AIRetryCount > 0 {
		result.WriteString(renderAIRetryLine(validation.AIRetryCount))
	}

	// Show individual checks in standard and expanded mode
	if len(validation.Checks) > 0 && mode != displayModeCompact {
		result.WriteString(renderChecksLine(validation.Checks))
	}

	return result.String()
}

// renderAIRetryLine renders the AI retry indicator with a distinguished style.
// Format: "    🤖 AI fixed (1 retry)" or "    🤖 AI fixed (2 retries)"
func renderAIRetryLine(retryCount int) string {
	retryWord := "retry"
	if retryCount > 1 {
		retryWord = "retries"
	}

	text := "AI fixed (" + intToString(retryCount) + " " + retryWord + ")"

	if HasColorSupport() {
		// Use a distinct color (cyan/blue) to make it stand out
		styledText := lipgloss.NewStyle().Foreground(ColorPrimary).Render(text)
		return "    🤖 " + styledText + "\n"
	}

	return "    [AI] " + text + "\n"
}

// renderChecksLine renders the individual validation checks in compact format.
// Format: "  Format ✓ | Lint ✓ | Test ✓ | Pre-commit ✓ | CI ✓"
func renderChecksLine(checks []ValidationCheck) string {
	parts := make([]string, 0, len(checks))
	for _, check := range checks {
		parts = append(parts, formatCheckItem(check))
	}
	return "    " + strings.Join(parts, " | ") + "\n"
}

// formatCheckItem formats a single validation check item with icon.
func formatCheckItem(check ValidationCheck) string {
	icon := "✓"
	color := ColorSuccess

	if check.Skipped {
		icon = "-"
		color = ColorMuted
	} else if !check.Passed {
		icon = "✗"
		color = ColorError
	}

	if !HasColorSupport() {
		return check.Name + " " + icon
	}

	return check.Name + " " + lipgloss.NewStyle().Foreground(color).Render(icon)
}

// renderValidationSection is kept for backward compatibility.
// Deprecated: Use renderValidationSectionWithMode instead.
func renderValidationSection(validation *ValidationSummary, width int) string {
	return renderValidationSectionWithMode(validation, width, displayModeStandard)
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

// extractPRDisplay extracts the PR number from a GitHub PR URL.
func extractPRDisplay(url string) string {
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
