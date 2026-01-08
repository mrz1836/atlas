// Package tui provides terminal user interface components for ATLAS.
//
// This package provides a centralized style system using Lip Gloss for consistent
// TUI component styling. All colors use AdaptiveColor for light/dark terminal support.
//
// # Semantic Colors (UX-4)
//
// Five semantic colors are exported for use across TUI components:
//   - ColorPrimary (Blue): Active states, links, primary actions
//   - ColorSuccess (Green): Success states, completed items
//   - ColorWarning (Yellow): Warning states, attention required
//   - ColorError (Red): Error states, failed items
//   - ColorMuted (Gray): Dim/inactive states, secondary text
//
// # Status Icons (UX-8)
//
// Triple redundancy is maintained for all status displays: icon + color + text.
// See TaskStatusIcon and WorkspaceStatusIcon for icon mappings.
//
// # NO_COLOR Support (UX-7)
//
// Call CheckNoColor() at the start of commands to respect the NO_COLOR environment
// variable. Colors are also disabled when TERM=dumb.
package tui

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/mrz1836/atlas/internal/constants"
)

//nolint:gochecknoglobals // Intentional package-level constants for TUI styling API
var (
	// ColorPrimary is blue, used for active states, links, and primary actions (UX-4).
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#0087AF", Dark: "#00D7FF"}

	// ColorSuccess is green, used for success states and completed items (UX-4).
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#008700", Dark: "#00FF87"}

	// ColorWarning is yellow, used for warning states and attention-required items (UX-4).
	ColorWarning = lipgloss.AdaptiveColor{Light: "#AF8700", Dark: "#FFD700"}

	// ColorError is red, used for error states and failed items (UX-4).
	ColorError = lipgloss.AdaptiveColor{Light: "#AF0000", Dark: "#FF5F5F"}

	// ColorMuted is gray, used for dim/inactive states and secondary text (UX-4).
	ColorMuted = lipgloss.AdaptiveColor{Light: "#585858", Dark: "#6C6C6C"}

	// LogoGradientColors defines the gradient colors for the ASCII logo (top to bottom).
	// Creates a 3D depth effect: bright cyan at top fading to deep blue at bottom.
	LogoGradientColors = []lipgloss.AdaptiveColor{
		{Light: "#00D7FF", Dark: "#00FFFF"}, // Brightest cyan (top)
		{Light: "#00AFFF", Dark: "#00D7FF"},
		{Light: "#0087FF", Dark: "#00AFFF"},
		{Light: "#005FD7", Dark: "#0087FF"},
		{Light: "#005FAF", Dark: "#005FD7"},
		{Light: "#00438B", Dark: "#005FAF"}, // Deepest blue (bottom)
	}

	// StyleBold applies bold formatting to text (AC: #3).
	StyleBold = lipgloss.NewStyle().Bold(true)

	// StyleDim applies dim/faint formatting to text (AC: #3).
	StyleDim = lipgloss.NewStyle().Faint(true)

	// StyleUnderline applies underline formatting to text (AC: #3).
	StyleUnderline = lipgloss.NewStyle().Underline(true)

	// StyleReverse applies reverse video (inverted colors) formatting to text (AC: #3).
	StyleReverse = lipgloss.NewStyle().Reverse(true)
)

// StatusColors returns the semantic color definitions for workspace statuses.
// Uses AdaptiveColor for light/dark terminal support (UX-6).
func StatusColors() map[constants.WorkspaceStatus]lipgloss.AdaptiveColor {
	return map[constants.WorkspaceStatus]lipgloss.AdaptiveColor{
		constants.WorkspaceStatusActive: {Light: "#0087AF", Dark: "#00D7FF"}, // Blue
		constants.WorkspaceStatusPaused: {Light: "#585858", Dark: "#6C6C6C"}, // Gray
		constants.WorkspaceStatusClosed: {Light: "#585858", Dark: "#6C6C6C"}, // Dim
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

// NewOutputStyles creates common output styles using AdaptiveColor for light/dark terminal support.
func NewOutputStyles() *OutputStyles {
	return &OutputStyles{
		Success: lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true),
		Error: lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true),
		Warning: lipgloss.NewStyle().
			Foreground(ColorWarning),
		Info: lipgloss.NewStyle().
			Foreground(ColorPrimary),
		Dim: lipgloss.NewStyle().
			Foreground(ColorMuted),
	}
}

// CheckNoColor respects the NO_COLOR environment variable (UX-7).
// Call this at the start of commands that output styled text.
func CheckNoColor() {
	if !HasColorSupport() {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// HasColorSupport returns true if the terminal supports colors.
// Returns false if NO_COLOR is set (any value including empty string) or TERM=dumb.
// This follows the NO_COLOR standard: https://no-color.org/
func HasColorSupport() bool {
	// NO_COLOR spec: If NO_COLOR exists in the environment (with any value, including empty),
	// color should be disabled.
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return false
	}

	// Also disable colors for dumb terminals
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
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
// Icons follow the spec from epic-7-tui-components-from-scenarios.md Icon Reference.
func TaskStatusIcon(status constants.TaskStatus) string {
	icons := map[constants.TaskStatus]string{
		constants.TaskStatusPending:          "○", // Empty circle - waiting (spec: ○ or [ ])
		constants.TaskStatusRunning:          "●", // Filled circle - active (spec: ● or ⟳)
		constants.TaskStatusValidating:       "⟳", // Rotating - in progress (spec: ● or ⟳)
		constants.TaskStatusValidationFailed: "⚠", // Warning - needs attention
		constants.TaskStatusAwaitingApproval: "✓", // Checkmark - ready for user (spec: ✓ or ⚠)
		constants.TaskStatusCompleted:        "✓", // Checkmark - success
		constants.TaskStatusRejected:         "✗", // X mark - failed
		constants.TaskStatusAbandoned:        "✗", // X mark - failed/abandoned
		constants.TaskStatusGHFailed:         "✗", // X mark - failed
		constants.TaskStatusCIFailed:         "✗", // X mark - failed
		constants.TaskStatusCITimeout:        "⚠", // Warning - needs attention
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
		constants.TaskStatusValidationFailed: "atlas recover",
		constants.TaskStatusAwaitingApproval: "atlas approve",
		constants.TaskStatusGHFailed:         "atlas recover",
		constants.TaskStatusCIFailed:         "atlas recover",
		constants.TaskStatusCITimeout:        "atlas recover",
	}
	if action, ok := actions[status]; ok {
		return action
	}
	return ""
}

// ActionStyle returns a lipgloss.Style for action indicators in attention states.
// Uses ColorWarning for visibility. Supports NO_COLOR via HasColorSupport().
// For NO_COLOR mode, returns an unstyled style (plain text indicators handled by caller).
func ActionStyle() lipgloss.Style {
	if !HasColorSupport() {
		return lipgloss.NewStyle() // Plain style for NO_COLOR mode
	}
	return lipgloss.NewStyle().Foreground(ColorWarning)
}

// WorkspaceStatusIcon returns the icon/symbol for a given workspace status.
// Used for visual status indicators in status displays.
func WorkspaceStatusIcon(status constants.WorkspaceStatus) string {
	icons := map[constants.WorkspaceStatus]string{
		constants.WorkspaceStatusActive: "●", // Filled circle - active
		constants.WorkspaceStatusPaused: "○", // Empty circle - paused
		constants.WorkspaceStatusClosed: "◌", // Dashed circle - closed
	}
	if icon, ok := icons[status]; ok {
		return icon
	}
	return "?"
}

// Status is an interface that both TaskStatus and WorkspaceStatus satisfy.
// Used for generic status formatting functions.
type Status interface {
	String() string
}

// FormatStatusWithIcon formats a status with its icon and text for triple redundancy (UX-8).
// This implements the icon + color + text pattern for accessibility.
// Color is applied via Lip Gloss styles when rendering; this function provides icon + text.
func FormatStatusWithIcon[S Status](status S, text string) string {
	var icon string

	// Type switch to get the appropriate icon
	switch s := any(status).(type) {
	case constants.TaskStatus:
		icon = TaskStatusIcon(s)
	case constants.WorkspaceStatus:
		icon = WorkspaceStatusIcon(s)
	default:
		icon = "?"
	}

	return icon + " " + text
}

// StyleSystem consolidates all style configurations for TUI components.
// Use NewStyleSystem() to create an instance with default values.
type StyleSystem struct {
	Colors     ColorPalette
	Typography TypographyStyles
	Icons      IconFunctions
}

// ColorPalette holds all semantic colors.
type ColorPalette struct {
	Primary lipgloss.AdaptiveColor
	Success lipgloss.AdaptiveColor
	Warning lipgloss.AdaptiveColor
	Error   lipgloss.AdaptiveColor
	Muted   lipgloss.AdaptiveColor
}

// TypographyStyles holds all text formatting styles.
type TypographyStyles struct {
	Bold      lipgloss.Style
	Dim       lipgloss.Style
	Underline lipgloss.Style
	Reverse   lipgloss.Style
}

// IconFunctions holds icon-related helper functions.
type IconFunctions struct {
	TaskStatus      func(constants.TaskStatus) string
	WorkspaceStatus func(constants.WorkspaceStatus) string
	FormatWithIcon  func(status any, text string) string
}

// NewStyleSystem creates a new StyleSystem with default values.
// This provides a convenient way to access all style configurations.
func NewStyleSystem() *StyleSystem {
	return &StyleSystem{
		Colors: ColorPalette{
			Primary: ColorPrimary,
			Success: ColorSuccess,
			Warning: ColorWarning,
			Error:   ColorError,
			Muted:   ColorMuted,
		},
		Typography: TypographyStyles{
			Bold:      StyleBold,
			Dim:       StyleDim,
			Underline: StyleUnderline,
			Reverse:   StyleReverse,
		},
		Icons: IconFunctions{
			TaskStatus:      TaskStatusIcon,
			WorkspaceStatus: WorkspaceStatusIcon,
			// FormatWithIcon delegates to the generic FormatStatusWithIcon function
			FormatWithIcon: formatStatusWithIconAny,
		},
	}
}

// formatStatusWithIconAny is a helper that wraps FormatStatusWithIcon for use with any type.
// This avoids duplicating the switch logic in NewStyleSystem.
func formatStatusWithIconAny(status any, text string) string {
	switch s := status.(type) {
	case constants.TaskStatus:
		return FormatStatusWithIcon(s, text)
	case constants.WorkspaceStatus:
		return FormatStatusWithIcon(s, text)
	default:
		return "? " + text
	}
}

// DefaultBoxWidth is the default width for TUI boxes.
const DefaultBoxWidth = 100

// BoxBorder defines the characters used for box borders.
type BoxBorder struct {
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	Top         string
	Bottom      string
	Left        string
	Right       string
	MiddleLeft  string // For divider lines
	MiddleRight string
}

// DefaultBorder is the default border style with square corners per UX spec.
// From epic-7-tui-components-from-scenarios.md: "Single-line box drawing characters (┌┐└┘─│├┤)"
//
//nolint:gochecknoglobals // Intentional package-level constant for TUI border styling
var DefaultBorder = BoxBorder{
	TopLeft:     "┌",
	TopRight:    "┐",
	BottomLeft:  "└",
	BottomRight: "┘",
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	MiddleLeft:  "├",
	MiddleRight: "┤",
}

// RoundedBorder is an alternative border style with rounded corners.
// Use DefaultBorder for standard UX-compliant boxes.
//
//nolint:gochecknoglobals // Intentional package-level constant for TUI border styling
var RoundedBorder = BoxBorder{
	TopLeft:     "╭",
	TopRight:    "╮",
	BottomLeft:  "╰",
	BottomRight: "╯",
	Top:         "─",
	Bottom:      "─",
	Left:        "│",
	Right:       "│",
	MiddleLeft:  "├",
	MiddleRight: "┤",
}

// BoxStyle holds configuration for rendering bordered boxes.
type BoxStyle struct {
	Width  int
	Border *BoxBorder
}

// NewBoxStyle creates a new BoxStyle with defaults (square border per UX spec, 65 char width).
func NewBoxStyle() *BoxStyle {
	border := DefaultBorder // Make a copy - uses square corners per UX spec
	return &BoxStyle{
		Width:  DefaultBoxWidth,
		Border: &border,
	}
}

// WithWidth returns a new BoxStyle with the specified width.
func (b *BoxStyle) WithWidth(width int) *BoxStyle {
	return &BoxStyle{
		Width:  width,
		Border: b.Border,
	}
}

// Render renders a box with the given title and content.
// Supports multi-line content by splitting on newlines.
func (b *BoxStyle) Render(title, content string) string {
	innerWidth := b.Width - 2 // Account for left and right borders

	// Build top line
	topLine := b.Border.TopLeft + strings.Repeat(b.Border.Top, innerWidth) + b.Border.TopRight

	// Build title line
	titleLine := b.Border.Left + " " + padRight(title, innerWidth-1) + b.Border.Right

	// Build divider line
	dividerLine := b.Border.MiddleLeft + strings.Repeat(b.Border.Top, innerWidth) + b.Border.MiddleRight

	// Build content lines (support multi-line content)
	splitLines := strings.Split(content, "\n")
	contentLines := make([]string, 0, len(splitLines))
	for _, line := range splitLines {
		contentLines = append(contentLines, b.Border.Left+" "+padRight(line, innerWidth-1)+b.Border.Right)
	}

	// Build bottom line
	bottomLine := b.Border.BottomLeft + strings.Repeat(b.Border.Bottom, innerWidth) + b.Border.BottomRight

	// Combine all parts
	result := topLine + "\n" + titleLine + "\n" + dividerLine + "\n"
	result += strings.Join(contentLines, "\n")
	result += "\n" + bottomLine

	return result
}

// stripANSI removes ANSI escape codes from a string.
// Used to calculate visible character count (excluding color codes).
// Handles both CSI sequences (\x1b[...letter) and OSC sequences (\x1b]...ST).
func stripANSI(s string) string {
	var result strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if newI := trySkipANSI(runes, i); newI != i {
			i = newI
			continue
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}

// trySkipANSI attempts to skip an ANSI escape sequence starting at position i.
// Returns the new position after the sequence, or i if no sequence was found.
func trySkipANSI(runes []rune, i int) int {
	if i >= len(runes) || runes[i] != '\x1b' || i+1 >= len(runes) {
		return i
	}

	next := runes[i+1]
	if next == '[' {
		return skipCSISequence(runes, i)
	}
	if next == ']' {
		return skipOSCSequence(runes, i)
	}
	return i
}

// skipCSISequence skips a CSI sequence: \x1b[...letter
func skipCSISequence(runes []rune, i int) int {
	i += 2 // skip \x1b[
	for i < len(runes) {
		c := runes[i]
		i++
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			break // CSI sequence ends with a letter
		}
	}
	return i
}

// skipOSCSequence skips an OSC sequence: \x1b]...ST (where ST is \x1b\\ or \x07)
func skipOSCSequence(runes []rune, i int) int {
	i += 2 // skip \x1b]
	for i < len(runes) {
		c := runes[i]
		if c == '\x07' {
			i++ // skip BEL terminator
			break
		}
		if c == '\x1b' && i+1 < len(runes) && runes[i+1] == '\\' {
			i += 2 // skip ST (\x1b\\)
			break
		}
		i++
	}
	return i
}

// padRight pads a string to the right to reach the target width.
// Uses visible character count (excluding ANSI escape codes) for proper width calculation.
func padRight(s string, width int) string {
	// Strip ANSI codes to get visible character count
	visible := stripANSI(s)
	runeCount := utf8.RuneCountInString(visible)
	if runeCount >= width {
		// Truncate to width runes (not bytes)
		runes := []rune(s)
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-runeCount)
}

// HeaderStyle creates a styled header with the given color.
// Used for consistent menu headers across TUI components.
func HeaderStyle(color lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(color).
		MarginBottom(1)
}

// RenderStyledHeader renders a styled header with icon and text.
// This provides consistent header styling across menu components.
func RenderStyledHeader(icon, text string, color lipgloss.TerminalColor) string {
	style := HeaderStyle(color)
	return style.Render(icon + " " + text)
}

// NarrowTerminalWidth is the threshold for narrow terminal mode.
// Terminals narrower than this value may need adjusted formatting.
const NarrowTerminalWidth = 80

// DefaultTerminalWidth is used when terminal width cannot be determined.
const DefaultTerminalWidth = 80

// IsNarrowTerminal returns true if terminal width is below the narrow threshold.
// Use this to adapt output format for narrow terminals.
// Uses TerminalWidth() from header.go for actual terminal detection.
func IsNarrowTerminal() bool {
	width := TerminalWidth()
	if width == 0 {
		// Width 0 means detection failed - treat as narrow for safety
		return true
	}
	return width < NarrowTerminalWidth
}
