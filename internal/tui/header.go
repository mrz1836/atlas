// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// ASCII art constants for the ATLAS header.
// Large 3D block-style logo with gradient coloring support.
const (
	// asciiArtLogo is the wide mode ASCII art.
	// Uses Unicode block characters for a bold, 3D appearance.
	// 6 lines tall, ~41 chars wide - fits comfortably in 80-col terminals.
	asciiArtLogo = ` █████╗ ████████╗██╗      █████╗ ███████╗
██╔══██╗╚══██╔══╝██║     ██╔══██╗██╔════╝
███████║   ██║   ██║     ███████║███████╗
██╔══██║   ██║   ██║     ██╔══██║╚════██║
██║  ██║   ██║   ███████╗██║  ██║███████║
╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═╝╚══════╝`

	// narrowHeader is the simple text header for terminals < 80 columns.
	narrowHeader = "═══ ATLAS ═══"

	// wideThreshold is the minimum terminal width for displaying ASCII art.
	wideThreshold = 80
)

// Header renders the ATLAS header component.
// Supports wide mode (ASCII art) and narrow mode (simple text).
type Header struct {
	width int
}

// NewHeader creates a new Header with the specified terminal width.
// Width of 0 or less triggers narrow mode (safe default).
func NewHeader(width int) *Header {
	return &Header{width: width}
}

// WithWidth returns a new Header with the specified width.
// Builder pattern for fluent configuration.
func (h *Header) WithWidth(w int) *Header {
	return &Header{width: w}
}

// Render returns the header string, centered for the current width.
// Wide mode (>= 80 cols) shows ASCII art; narrow mode shows simple text.
func (h *Header) Render() string {
	if h.width >= wideThreshold {
		return h.renderWide()
	}
	return h.renderNarrow()
}

// renderWide returns the ASCII art header, styled with gradient colors and centered.
func (h *Header) renderWide() string {
	lines := strings.Split(asciiArtLogo, "\n")
	styledLines := make([]string, 0, len(lines))

	for i, line := range lines {
		// Apply gradient color for this line (bright cyan at top → deep blue at bottom)
		colorIdx := i
		if colorIdx >= len(LogoGradientColors) {
			colorIdx = len(LogoGradientColors) - 1
		}
		style := lipgloss.NewStyle().Foreground(LogoGradientColors[colorIdx])

		styledLine := style.Render(line)
		centered := centerText(styledLine, line, h.width)
		styledLines = append(styledLines, centered)
	}

	return strings.Join(styledLines, "\n")
}

// renderNarrow returns the simple text header, centered.
func (h *Header) renderNarrow() string {
	// Apply primary color to the narrow header
	style := lipgloss.NewStyle().Foreground(ColorPrimary)
	styledHeader := style.Render(narrowHeader)
	return centerText(styledHeader, narrowHeader, h.width)
}

// centerText centers styled text based on the original (unstyled) text visual width.
// The styled parameter contains ANSI codes, while original is plain text for width calculation.
// Uses rune count for proper Unicode handling (multi-byte chars like ▄▀█ are 1 visual column each).
func centerText(styled, original string, totalWidth int) string {
	textWidth := runeWidth(original)
	if totalWidth <= 0 || textWidth >= totalWidth {
		return styled
	}
	padding := (totalWidth - textWidth) / 2
	if padding <= 0 {
		return styled
	}
	return strings.Repeat(" ", padding) + styled
}

// runeWidth returns the visual width of a string (rune count).
// For the ASCII art characters used (▄▀█░), each rune is 1 terminal column.
func runeWidth(s string) int {
	return len([]rune(s))
}

// GetTerminalWidth returns the current terminal width.
// Returns 0 if width cannot be determined (triggers narrow mode fallback).
func GetTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0 // Fallback to narrow mode
	}
	return width
}

// RenderHeader renders the ATLAS header at the specified width.
// Convenience function for one-off rendering without creating a Header struct.
func RenderHeader(width int) string {
	return NewHeader(width).Render()
}

// RenderHeaderAuto renders the ATLAS header, auto-detecting terminal width.
// Uses narrow mode if width detection fails.
func RenderHeaderAuto() string {
	return NewHeader(GetTerminalWidth()).Render()
}
