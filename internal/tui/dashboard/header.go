package dashboard

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Header is the top-bar component of the ATLAS dashboard.
// It shows the dashboard title, a right-aligned clock, and a daemon connection indicator.
// Header is a pure renderer — call SetTime and SetConnection before each View call.
type Header struct {
	title           string
	clock           time.Time
	connectionState ConnectionState
}

// NewHeader creates a Header with the given title and a disconnected initial state.
func NewHeader(title string) Header {
	return Header{
		title:           title,
		connectionState: ConnectionStateReconnecting,
	}
}

// SetTime updates the clock timestamp shown in the header.
func (h *Header) SetTime(t time.Time) { h.clock = t }

// SetConnection updates the daemon connection status indicator.
func (h *Header) SetConnection(state ConnectionState) { h.connectionState = state }

// Title returns the header title string.
func (h *Header) Title() string { return h.title }

// Clock returns the current clock time.
func (h *Header) Clock() time.Time { return h.clock }

// ConnectionState returns the current connection state.
func (h *Header) ConnectionState() ConnectionState { return h.connectionState }

// View renders the header into a single line of exactly width columns.
// Layout: [title]    [daemon status]  [clock]
func (h *Header) View(width int) string {
	s := GetStyles()

	// Left: dashboard title.
	titleStr := s.Header.Render(h.title)

	// Middle: daemon connection indicator.
	daemonStr := h.renderDaemonStatus(s)

	// Right: clock (HH:MM:SS).
	clockStr := ""
	if !h.clock.IsZero() {
		clockStr = s.Clock.Render(h.clock.Format("15:04:05"))
	}

	// Measure plain-text widths for positioning.
	titleW := len([]rune(stripANSI(titleStr)))
	daemonW := len([]rune(stripANSI(daemonStr)))
	clockW := len([]rune(stripANSI(clockStr)))

	// Calculate gap between title and daemon indicator (centered).
	totalFixed := titleW + daemonW + clockW
	if totalFixed >= width {
		// Not enough space — fall back to title only.
		return padRight(stripANSI(titleStr), width)
	}

	// Place daemon in the middle region, clock at the far right.
	// gap1 = between title and daemon, gap2 = between daemon and clock.
	remaining := width - totalFixed
	gap1 := remaining / 2
	gap2 := remaining - gap1

	line := titleStr + strings.Repeat(" ", gap1) + daemonStr + strings.Repeat(" ", gap2) + clockStr

	// Final safety: pad or truncate to exactly width (ANSI-unaware but good enough
	// for the layout skeleton; rendering will adjust visually).
	plain := stripANSI(line)
	if len([]rune(plain)) < width {
		line += strings.Repeat(" ", width-len([]rune(plain)))
	}

	return line
}

// renderDaemonStatus returns the colored daemon connection string.
// "● daemon" (green=connected), "⚠ Reconnecting…" (yellow), "✗ daemon" (red=disconnected).
func (h *Header) renderDaemonStatus(s *Styles) string {
	switch h.connectionState {
	case ConnectionStateConnected:
		return lipgloss.NewStyle().Inherit(s.DaemonConnected).Render("● daemon")
	case ConnectionStateReconnecting:
		return lipgloss.NewStyle().Inherit(s.DaemonReconnecting).Render("⚠ Reconnecting…")
	case ConnectionStateDisconnected:
		return lipgloss.NewStyle().Inherit(s.DaemonDisconnected).Render("✗ daemon")
	default:
		return ""
	}
}
