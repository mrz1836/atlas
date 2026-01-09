// Package tui provides terminal user interface components for ATLAS.
//
// This file provides the verification menu component, which presents options
// to users when AI verification finds issues.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// VerificationMenuChoice represents a user's choice for handling verification issues.
type VerificationMenuChoice int

const (
	// VerifyMenuAutoFix attempts to auto-fix issues using AI.
	VerifyMenuAutoFix VerificationMenuChoice = iota
	// VerifyMenuManualFix user fixes manually and resumes.
	VerifyMenuManualFix
	// VerifyMenuIgnoreContinue proceeds despite warnings.
	VerifyMenuIgnoreContinue
	// VerifyMenuViewReport displays the full verification report.
	VerifyMenuViewReport
)

// String returns a string representation of the menu choice.
func (c VerificationMenuChoice) String() string {
	switch c {
	case VerifyMenuAutoFix:
		return "auto_fix"
	case VerifyMenuManualFix:
		return "manual_fix"
	case VerifyMenuIgnoreContinue:
		return "ignore_continue"
	case VerifyMenuViewReport:
		return "view_report"
	default:
		return "unknown"
	}
}

// VerificationMenuOptions configures the verification menu display.
type VerificationMenuOptions struct {
	// TaskID is the task being verified.
	TaskID string
	// TaskDesc is the task description.
	TaskDesc string
	// TotalIssues is the number of issues found.
	TotalIssues int
	// ErrorCount is the number of error-level issues.
	ErrorCount int
	// WarningCount is the number of warning-level issues.
	WarningCount int
	// InfoCount is the number of info-level issues.
	InfoCount int
	// ReportPath is the path to the saved verification report.
	ReportPath string
	// Summary is the brief verification summary.
	Summary string
	// HasErrors indicates if there are any error-level issues.
	HasErrors bool
}

// RenderVerificationMenu generates a non-interactive menu display for verification issues.
// This is used in non-interactive mode or as a reference for the user.
func RenderVerificationMenu(opts VerificationMenuOptions) string {
	styles := GetOutputStyles()
	CheckNoColor()

	var sb strings.Builder

	// Header
	sb.WriteString(RenderStyledHeader("🔍", "Verification Issues Found", ColorWarning))
	sb.WriteString("\n\n")

	// Task info
	sb.WriteString(styles.Info.Render(fmt.Sprintf("Task: %s", opts.TaskID)))
	sb.WriteString("\n")
	if opts.TaskDesc != "" {
		sb.WriteString(styles.Dim.Render(opts.TaskDesc))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Issue summary
	sb.WriteString(styles.Warning.Render("Issues Found:"))
	sb.WriteString("\n")
	if opts.ErrorCount > 0 {
		errorStyle := lipgloss.NewStyle().Foreground(ColorError)
		sb.WriteString(fmt.Sprintf("  • %s\n", errorStyle.Render(fmt.Sprintf("%d error(s)", opts.ErrorCount))))
	}
	if opts.WarningCount > 0 {
		warningStyle := lipgloss.NewStyle().Foreground(ColorWarning)
		sb.WriteString(fmt.Sprintf("  • %s\n", warningStyle.Render(fmt.Sprintf("%d warning(s)", opts.WarningCount))))
	}
	if opts.InfoCount > 0 {
		infoStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
		sb.WriteString(fmt.Sprintf("  • %s\n", infoStyle.Render(fmt.Sprintf("%d info", opts.InfoCount))))
	}
	sb.WriteString("\n")

	// Summary
	if opts.Summary != "" {
		sb.WriteString(styles.Dim.Render(opts.Summary))
		sb.WriteString("\n\n")
	}

	// Menu options
	optionStyle := lipgloss.NewStyle().Bold(true)
	descStyle := styles.Dim

	sb.WriteString("Choose an action:\n\n")

	menuItems := []struct {
		key   string
		label string
		desc  string
		note  string
	}{
		{
			key:   "a",
			label: "Auto-fix issues",
			desc:  "AI attempts to fix based on verification feedback",
		},
		{
			key:   "m",
			label: "Manual fix",
			desc:  "You fix issues, then resume",
		},
		{
			key:   "i",
			label: "Ignore and continue",
			desc:  "Proceed despite warnings",
			note:  getIgnoreNote(opts.HasErrors),
		},
		{
			key:   "v",
			label: "View full report",
			desc:  "Display complete verification-report.md",
		},
	}

	for _, item := range menuItems {
		prefix := fmt.Sprintf("  [%s] ", item.key)
		sb.WriteString(prefix)
		sb.WriteString(optionStyle.Render(item.label))
		sb.WriteString(" — ")
		sb.WriteString(descStyle.Render(item.desc))
		if item.note != "" {
			sb.WriteString(" ")
			sb.WriteString(item.note)
		}
		sb.WriteString("\n")
	}

	// Report path
	if opts.ReportPath != "" {
		sb.WriteString("\n")
		sb.WriteString(styles.Dim.Render(fmt.Sprintf("Report saved: %s", opts.ReportPath)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// getIgnoreNote returns a note for the ignore option based on whether there are errors.
func getIgnoreNote(hasErrors bool) string {
	if hasErrors {
		return "(⚠️ has errors)"
	}
	return ""
}

// FormatVerificationSummary formats a brief verification summary for display.
func FormatVerificationSummary(totalIssues, errorCount, warningCount int, passed bool) string {
	styles := GetOutputStyles()
	CheckNoColor()

	if passed {
		return styles.Success.Render("✅ All verification checks passed")
	}

	var parts []string
	if errorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errorCount))
	}
	if warningCount > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warningCount))
	}
	infoCount := totalIssues - errorCount - warningCount
	if infoCount > 0 {
		parts = append(parts, fmt.Sprintf("%d info", infoCount))
	}

	icon := "⚠️"
	if errorCount > 0 {
		icon = "❌"
	}

	return styles.Warning.Render(fmt.Sprintf("%s Verification found: %s", icon, strings.Join(parts, ", ")))
}

// RenderVerificationReport renders the verification report for display.
func RenderVerificationReport(report string) string {
	styles := GetOutputStyles()
	CheckNoColor()

	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#5FAFFF")).
		MarginBottom(1)

	sb.WriteString(headerStyle.Render("📋 Verification Report"))
	sb.WriteString("\n")
	sb.WriteString(styles.Dim.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n\n")
	sb.WriteString(report)
	sb.WriteString("\n")
	sb.WriteString(styles.Dim.Render(strings.Repeat("─", 60)))
	sb.WriteString("\n")

	return sb.String()
}
