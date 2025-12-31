// Package tui provides terminal user interface components for ATLAS.
//
// This file provides the CI failure menu component, which presents options
// to users when CI fails or times out.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CIFailureMenuChoice represents a user's choice for handling CI failure.
//
// This enum mirrors task.CIFailureAction but is defined separately
// to maintain package boundaries (tui must not import task).
// When converting between them, use this mapping:
//
//	CIMenuViewLogs         <-> task.CIFailureViewLogs
//	CIMenuRetryFromImplement <-> task.CIFailureRetryImplement
//	CIMenuFixManually      <-> task.CIFailureFixManually
//	CIMenuAbandonTask      <-> task.CIFailureAbandon
type CIFailureMenuChoice int

const (
	// CIMenuViewLogs opens GitHub Actions in browser.
	// Maps to: task.CIFailureViewLogs
	CIMenuViewLogs CIFailureMenuChoice = iota
	// CIMenuRetryFromImplement retries from implement step with error context.
	// Maps to: task.CIFailureRetryImplement
	CIMenuRetryFromImplement
	// CIMenuFixManually user fixes in worktree, then resumes.
	// Maps to: task.CIFailureFixManually
	CIMenuFixManually
	// CIMenuAbandonTask ends task, keeps PR as draft.
	// Maps to: task.CIFailureAbandon
	CIMenuAbandonTask
)

// String returns a string representation of the menu choice.
func (c CIFailureMenuChoice) String() string {
	switch c {
	case CIMenuViewLogs:
		return "view_logs"
	case CIMenuRetryFromImplement:
		return "retry_implement"
	case CIMenuFixManually:
		return "fix_manually"
	case CIMenuAbandonTask:
		return "abandon"
	default:
		return "unknown"
	}
}

// CIFailureMenuOptions configures the CI failure menu display.
type CIFailureMenuOptions struct {
	// PRNumber is the PR with failing CI.
	PRNumber int
	// WorkspaceName is the workspace identifier.
	WorkspaceName string
	// FailedChecks lists the names of failed CI checks.
	FailedChecks []string
	// HasCheckURL indicates if a check URL is available.
	HasCheckURL bool
	// Status is the CI status (failure, timeout).
	Status string
}

// RenderCIFailureMenu generates a non-interactive menu display for CI failure.
// This is used in non-interactive mode or as a reference for the user.
func RenderCIFailureMenu(opts CIFailureMenuOptions) string {
	styles := NewOutputStyles()
	CheckNoColor()

	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF5F5F")).
		MarginBottom(1)

	statusText := "CI Failed"
	if opts.Status == "timeout" {
		statusText = "CI Timed Out"
	}
	sb.WriteString(headerStyle.Render(fmt.Sprintf("⚠️  %s", statusText)))
	sb.WriteString("\n\n")

	// PR info
	sb.WriteString(styles.Info.Render(fmt.Sprintf("PR #%d", opts.PRNumber)))
	sb.WriteString(" in workspace ")
	sb.WriteString(styles.Info.Render(opts.WorkspaceName))
	sb.WriteString("\n\n")

	// Failed checks
	if len(opts.FailedChecks) > 0 {
		sb.WriteString(styles.Warning.Render("Failed Checks:"))
		sb.WriteString("\n")
		for _, check := range opts.FailedChecks {
			sb.WriteString(fmt.Sprintf("  • %s\n", check))
		}
		sb.WriteString("\n")
	}

	// Menu options
	optionStyle := lipgloss.NewStyle().Bold(true)
	descStyle := styles.Dim

	sb.WriteString("Choose an action:\n\n")

	menuItems := []struct {
		key      string
		label    string
		desc     string
		disabled bool
	}{
		{
			key:      "1",
			label:    "View workflow logs",
			desc:     "Open GitHub Actions in browser",
			disabled: !opts.HasCheckURL,
		},
		{
			key:   "2",
			label: "Retry from implement",
			desc:  "AI tries to fix based on CI output",
		},
		{
			key:   "3",
			label: "Fix manually",
			desc:  "You fix in worktree, then resume",
		},
		{
			key:   "4",
			label: "Abandon task",
			desc:  "End task, keep PR as draft",
		},
	}

	for _, item := range menuItems {
		prefix := fmt.Sprintf("  [%s] ", item.key)
		if item.disabled {
			sb.WriteString(descStyle.Render(prefix + item.label + " (unavailable)"))
		} else {
			sb.WriteString(prefix)
			sb.WriteString(optionStyle.Render(item.label))
			sb.WriteString(" — ")
			sb.WriteString(descStyle.Render(item.desc))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatCIFailureStatus formats the CI failure status for display.
func FormatCIFailureStatus(prNumber int, failedChecks []string, status string) string {
	styles := NewOutputStyles()
	CheckNoColor()

	var sb strings.Builder

	icon := "⚠️"
	if status == "timeout" {
		icon = "⏱️"
	}

	sb.WriteString(styles.Error.Render(fmt.Sprintf("%s CI %s for PR #%d", icon, status, prNumber)))
	sb.WriteString("\n")

	if len(failedChecks) > 0 {
		sb.WriteString(styles.Dim.Render("Failed: "))
		sb.WriteString(strings.Join(failedChecks, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}
