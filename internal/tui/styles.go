// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/mrz1836/atlas/internal/constants"
)

// StatusColors returns the semantic color definitions for workspace statuses.
// Uses AdaptiveColor for light/dark terminal support (UX-6).
func StatusColors() map[constants.WorkspaceStatus]lipgloss.AdaptiveColor {
	return map[constants.WorkspaceStatus]lipgloss.AdaptiveColor{
		constants.WorkspaceStatusActive:  {Light: "#0087AF", Dark: "#00D7FF"}, // Blue
		constants.WorkspaceStatusPaused:  {Light: "#585858", Dark: "#6C6C6C"}, // Gray
		constants.WorkspaceStatusRetired: {Light: "#585858", Dark: "#6C6C6C"}, // Dim
	}
}

// TableStyles holds lipgloss styles for table rendering.
type TableStyles struct {
	Header       lipgloss.Style
	Cell         lipgloss.Style
	Dim          lipgloss.Style
	StatusColors map[constants.WorkspaceStatus]lipgloss.AdaptiveColor
}

// NewTableStyles creates styles for table rendering.
func NewTableStyles() *TableStyles {
	return &TableStyles{
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"}),
		Cell: lipgloss.NewStyle(),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}),
		StatusColors: StatusColors(),
	}
}

// OutputStyles holds common output styles.
type OutputStyles struct {
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style
	Dim     lipgloss.Style
}

// NewOutputStyles creates common output styles.
func NewOutputStyles() *OutputStyles {
	return &OutputStyles{
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")).
			Bold(true),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")),
		Info: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
	}
}

// CheckNoColor respects the NO_COLOR environment variable (UX-7).
// Call this at the start of commands that output styled text.
func CheckNoColor() {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// TaskStatusColors returns the semantic color definitions for task statuses.
// Uses AdaptiveColor for light/dark terminal support (UX-6).
func TaskStatusColors() map[constants.TaskStatus]lipgloss.AdaptiveColor {
	return map[constants.TaskStatus]lipgloss.AdaptiveColor{
		// Active states - Blue
		constants.TaskStatusPending:    {Light: "#0087AF", Dark: "#00D7FF"},
		constants.TaskStatusRunning:    {Light: "#0087AF", Dark: "#00D7FF"},
		constants.TaskStatusValidating: {Light: "#0087AF", Dark: "#00D7FF"},

		// Warning states - Yellow/Orange
		constants.TaskStatusValidationFailed: {Light: "#D7AF00", Dark: "#FFD700"},
		constants.TaskStatusAwaitingApproval: {Light: "#D7AF00", Dark: "#FFD700"},
		constants.TaskStatusGHFailed:         {Light: "#D7AF00", Dark: "#FFD700"},
		constants.TaskStatusCIFailed:         {Light: "#D7AF00", Dark: "#FFD700"},
		constants.TaskStatusCITimeout:        {Light: "#D7AF00", Dark: "#FFD700"},

		// Success state - Green
		constants.TaskStatusCompleted: {Light: "#00875F", Dark: "#00FF87"},

		// Terminal states - Gray/Dim
		constants.TaskStatusRejected:  {Light: "#585858", Dark: "#6C6C6C"},
		constants.TaskStatusAbandoned: {Light: "#585858", Dark: "#6C6C6C"},
	}
}

// TaskStatusIcon returns the icon/symbol for a given task status.
// Used for visual status indicators in status displays.
func TaskStatusIcon(status constants.TaskStatus) string {
	icons := map[constants.TaskStatus]string{
		constants.TaskStatusPending:          "○", // Empty circle - waiting
		constants.TaskStatusRunning:          "▶", // Play - active
		constants.TaskStatusValidating:       "◐", // Half circle - in progress
		constants.TaskStatusValidationFailed: "⚠", // Warning - needs attention
		constants.TaskStatusAwaitingApproval: "◉", // Filled circle - ready for user
		constants.TaskStatusCompleted:        "✓", // Checkmark - success
		constants.TaskStatusRejected:         "✗", // X mark - rejected
		constants.TaskStatusAbandoned:        "⊘", // Null - abandoned
		constants.TaskStatusGHFailed:         "⚠", // Warning - needs attention
		constants.TaskStatusCIFailed:         "⚠", // Warning - needs attention
		constants.TaskStatusCITimeout:        "⏱", // Timer - timeout
	}
	if icon, ok := icons[status]; ok {
		return icon
	}
	return "?"
}

// IsAttentionStatus returns true if the task status requires user attention.
// These statuses should be highlighted and sorted to the top of status lists.
func IsAttentionStatus(status constants.TaskStatus) bool {
	attentionStatuses := map[constants.TaskStatus]bool{
		constants.TaskStatusValidationFailed: true,
		constants.TaskStatusAwaitingApproval: true,
		constants.TaskStatusGHFailed:         true,
		constants.TaskStatusCIFailed:         true,
		constants.TaskStatusCITimeout:        true,
	}
	return attentionStatuses[status]
}

// SuggestedAction returns the suggested CLI command for a given task status.
// Returns empty string if no action is needed or available.
func SuggestedAction(status constants.TaskStatus) string {
	actions := map[constants.TaskStatus]string{
		constants.TaskStatusValidationFailed: "atlas resume",
		constants.TaskStatusAwaitingApproval: "atlas approve",
		constants.TaskStatusGHFailed:         "atlas retry",
		constants.TaskStatusCIFailed:         "atlas retry",
		constants.TaskStatusCITimeout:        "atlas retry",
	}
	if action, ok := actions[status]; ok {
		return action
	}
	return ""
}
