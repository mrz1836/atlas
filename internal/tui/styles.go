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
