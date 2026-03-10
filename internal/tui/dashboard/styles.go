package dashboard

import (
	"os"
	"sync"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/colorprofile"

	"github.com/mrz1836/atlas/internal/tui"
)

// dashboardColorProfile is the detected terminal color capability.
// Set once at initialisation and never changed.
//
//nolint:gochecknoglobals // Intentional singleton for colorprofile detection
var (
	dashboardColorProfile     colorprofile.Profile
	dashboardColorProfileOnce sync.Once
)

// getColorProfile returns the lazily-detected color profile for the dashboard.
func getColorProfile() colorprofile.Profile {
	dashboardColorProfileOnce.Do(func() {
		// Detect from stdout; fallback to Ascii for dumb/no-color terminals.
		dashboardColorProfile = colorprofile.Detect(os.Stdout, os.Environ())
	})
	return dashboardColorProfile
}

// hasColor returns true when the detected color profile supports at least ANSI colors.
func hasColor() bool {
	return getColorProfile() > colorprofile.Ascii
}

// --- Status colors for dashboard task indicators ---

// statusColors maps TaskStatus values to AdaptiveColors.
// running=blue, queued=gray, approval=yellow, done=green, failed=red.
//
//nolint:gochecknoglobals // Intentional package-level styling constants
var statusColors = map[TaskStatus]compat.AdaptiveColor{
	TaskStatusRunning:          tui.ColorPrimary, // blue — active
	TaskStatusQueued:           tui.ColorMuted,   // gray — waiting
	TaskStatusPaused:           tui.ColorMuted,   // gray — paused
	TaskStatusAwaitingApproval: tui.ColorWarning, // yellow — needs user action
	TaskStatusCompleted:        tui.ColorSuccess, // green — done
	TaskStatusFailed:           tui.ColorError,   // red — error
	TaskStatusAbandoned:        tui.ColorMuted,   // gray — terminal
}

// StatusColor returns the AdaptiveColor for the given task status.
// Falls back to ColorMuted for unknown statuses.
func StatusColor(status TaskStatus) compat.AdaptiveColor {
	if c, ok := statusColors[status]; ok {
		return c
	}
	return tui.ColorMuted
}

// StatusIcon returns the display icon for the given task status.
// Icons provide visual redundancy independent of color.
func StatusIcon(status TaskStatus) string {
	switch status {
	case TaskStatusRunning:
		return "●"
	case TaskStatusQueued:
		return "○"
	case TaskStatusPaused:
		return "⏸"
	case TaskStatusAwaitingApproval:
		return "⚠"
	case TaskStatusCompleted:
		return "✓"
	case TaskStatusFailed:
		return "✗"
	case TaskStatusAbandoned:
		return "✗"
	default:
		return "?"
	}
}

// RenderStatusIndicator returns a styled "icon status" string for a task.
// In no-color mode, returns plain text with the icon prefix.
func RenderStatusIndicator(status TaskStatus) string {
	icon := StatusIcon(status)
	text := icon + " " + string(status)
	if !hasColor() {
		return text
	}
	style := lipgloss.NewStyle().Foreground(StatusColor(status))
	return style.Render(text)
}

// --- Dashboard-specific styles ---

// Styles holds all the lipgloss styles used by the dashboard.
// Instantiate with NewStyles() for a ready-to-use set.
type Styles struct {
	// PanelBorder is applied to the border of each panel.
	PanelBorder lipgloss.Style
	// PanelBorderActive is the border style for the currently focused panel.
	PanelBorderActive lipgloss.Style
	// PanelTitle is the style for the panel title bar.
	PanelTitle lipgloss.Style
	// PanelTitleActive is the style for the title of the focused panel.
	PanelTitleActive lipgloss.Style

	// Selected is the style for the currently selected row in the task list.
	Selected lipgloss.Style
	// Dimmed is the style for inactive / secondary content.
	Dimmed lipgloss.Style

	// Header is the style for the dashboard title.
	Header lipgloss.Style
	// Clock is the style for the clock displayed in the header.
	Clock lipgloss.Style
	// DaemonConnected is the style for the connected status indicator.
	DaemonConnected lipgloss.Style
	// DaemonReconnecting is the style for the reconnecting status indicator.
	DaemonReconnecting lipgloss.Style
	// DaemonDisconnected is the style for the disconnected status indicator.
	DaemonDisconnected lipgloss.Style

	// StatusBar is the style for the bottom status bar.
	StatusBar lipgloss.Style
	// KeyHint is the style for keybinding hints in the status bar.
	KeyHint lipgloss.Style
	// KeyAction is the style for action labels in the status bar.
	KeyAction lipgloss.Style

	// LogDebug is the style for debug-level log lines.
	LogDebug lipgloss.Style
	// LogInfo is the style for info-level log lines.
	LogInfo lipgloss.Style
	// LogWarn is the style for warn-level log lines.
	LogWarn lipgloss.Style
	// LogError is the style for error-level log lines.
	LogError lipgloss.Style

	// Highlight is the style for search match highlighting.
	Highlight lipgloss.Style

	// HelpKey is the style for key labels in the help overlay.
	HelpKey lipgloss.Style
	// HelpDesc is the style for descriptions in the help overlay.
	HelpDesc lipgloss.Style
	// HelpGroup is the style for group headings in the help overlay.
	HelpGroup lipgloss.Style
}

// cachedStyles is the singleton Styles instance.
//
//nolint:gochecknoglobals // Intentional singleton for performance
var (
	cachedStyles     *Styles
	cachedStylesOnce sync.Once
)

// GetStyles returns a cached Styles instance.
// The styles use AdaptiveColor which adapts at render time, so caching is safe.
func GetStyles() *Styles {
	cachedStylesOnce.Do(func() {
		cachedStyles = NewStyles()
	})
	return cachedStyles
}

// NewStyles builds a new Styles set adapted to the current color profile.
func NewStyles() *Styles {
	// Border color: blue when color available, plain otherwise.
	borderColor := compat.AdaptiveColor{
		Light: lipgloss.Color("#0087AF"),
		Dark:  lipgloss.Color("#00AFFF"),
	}
	activeBorderColor := tui.ColorPrimary

	s := &Styles{
		// Panel borders
		PanelBorder: lipgloss.NewStyle().
			Foreground(borderColor),
		PanelBorderActive: lipgloss.NewStyle().
			Foreground(activeBorderColor).
			Bold(true),

		// Panel titles
		PanelTitle: lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Bold(false),
		PanelTitleActive: lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true),

		// List selection and dim
		Selected: lipgloss.NewStyle().
			Reverse(true).
			Bold(true),
		Dimmed: lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Faint(true),

		// Header
		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorPrimary),
		Clock: lipgloss.NewStyle().
			Foreground(tui.ColorMuted),
		DaemonConnected: lipgloss.NewStyle().
			Foreground(tui.ColorSuccess),
		DaemonReconnecting: lipgloss.NewStyle().
			Foreground(tui.ColorWarning),
		DaemonDisconnected: lipgloss.NewStyle().
			Foreground(tui.ColorError),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(tui.ColorMuted),
		KeyHint: lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true),
		KeyAction: lipgloss.NewStyle().
			Foreground(tui.ColorMuted),

		// Log levels
		LogDebug: lipgloss.NewStyle().
			Foreground(tui.ColorMuted).
			Faint(true),
		LogInfo: lipgloss.NewStyle(), // default terminal color
		LogWarn: lipgloss.NewStyle().
			Foreground(tui.ColorWarning),
		LogError: lipgloss.NewStyle().
			Foreground(tui.ColorError).
			Bold(true),

		// Search highlight
		Highlight: lipgloss.NewStyle().
			Background(tui.ColorWarning).
			Foreground(compat.AdaptiveColor{
				Light: lipgloss.Color("#000000"),
				Dark:  lipgloss.Color("#000000"),
			}),

		// Help overlay
		HelpKey: lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true),
		HelpDesc: lipgloss.NewStyle().
			Foreground(tui.ColorMuted),
		HelpGroup: lipgloss.NewStyle().
			Foreground(tui.ColorPrimary).
			Bold(true).
			Underline(true),
	}

	// Degrade gracefully when color is not available.
	if !hasColor() {
		s.applyNoColor()
	}

	return s
}

// StylesForProfile returns a Styles set tuned for the given color profile.
// This is the explicit-profile variant of GetStyles (which auto-detects).
//
//   - TrueColor / ANSI256 / ANSI: full adaptive color palette (same as NewStyles).
//   - Ascii (no color): structural formatting only, no color values.
func StylesForProfile(profile colorprofile.Profile) *Styles {
	s := NewStyles()
	if profile == colorprofile.Ascii {
		s.applyNoColor()
	}
	return s
}

// applyNoColor strips all color from styles, leaving only structural formatting.
// Called automatically by NewStyles when the color profile is Ascii.
func (s *Styles) applyNoColor() {
	plain := lipgloss.NewStyle()
	s.PanelBorder = plain
	s.PanelBorderActive = lipgloss.NewStyle().Bold(true)
	s.PanelTitle = plain
	s.PanelTitleActive = lipgloss.NewStyle().Bold(true)
	s.Selected = lipgloss.NewStyle().Reverse(true)
	s.Dimmed = plain
	s.Header = lipgloss.NewStyle().Bold(true)
	s.Clock = plain
	s.DaemonConnected = plain
	s.DaemonReconnecting = plain
	s.DaemonDisconnected = plain
	s.StatusBar = plain
	s.KeyHint = lipgloss.NewStyle().Bold(true)
	s.KeyAction = plain
	s.LogDebug = plain
	s.LogInfo = plain
	s.LogWarn = plain
	s.LogError = lipgloss.NewStyle().Bold(true)
	s.Highlight = lipgloss.NewStyle().Reverse(true)
	s.HelpKey = lipgloss.NewStyle().Bold(true)
	s.HelpDesc = plain
	s.HelpGroup = lipgloss.NewStyle().Bold(true).Underline(true)
}
