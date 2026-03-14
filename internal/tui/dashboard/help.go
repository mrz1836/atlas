package dashboard

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// HelpOverlay renders a keyboard shortcut reference as a modal overlay.
// It is toggled by pressing '?' and dismissed by any key.
//
// Usage:
//
//	h := NewHelpOverlay(km)
//	h.Toggle()        // show/hide
//	if h.IsVisible() { view = h.View(w, height) }
type HelpOverlay struct {
	visible bool
	keymap  KeyMap
}

// NewHelpOverlay creates a HelpOverlay for the given key map.
// Returns a pointer so the model can store and mutate it.
func NewHelpOverlay(km KeyMap) *HelpOverlay {
	return &HelpOverlay{keymap: km}
}

// Toggle flips the visibility of the help overlay.
func (h *HelpOverlay) Toggle() { h.visible = !h.visible }

// Show makes the help overlay visible.
func (h *HelpOverlay) Show() { h.visible = true }

// Hide makes the help overlay invisible.
func (h *HelpOverlay) Hide() { h.visible = false }

// IsVisible returns true when the help overlay should be rendered.
func (h *HelpOverlay) IsVisible() bool { return h.visible }

// View renders the help overlay content within the given terminal dimensions.
// Returns an empty string when not visible.
//
// Group headers are rendered as plain-text section labels to avoid ANSI
// per-character re-encoding that occurs when pre-styled strings are passed
// through a width-constrained lipgloss Render call.
func (h *HelpOverlay) View(width, height int) string {
	if !h.visible {
		return ""
	}

	s := GetStyles()

	groups := []struct {
		name     string
		bindings []key.Binding
	}{
		{"Navigation", h.keymap.NavigationKeys()},
		{"Actions", h.keymap.ActionKeys()},
		{"Log", h.keymap.LogKeys()},
		{"General", h.keymap.GeneralKeys()},
	}

	// Build content lines without pre-applying group-level ANSI styling.
	// The whole block is rendered into a border box below, so we keep the
	// group names as plain strings (styling is applied per key-binding only).
	var lines []string
	for _, g := range groups {
		// Plain group header — avoids ANSI per-char re-encoding inside the box.
		lines = append(lines, "── "+g.name+" ──")
		for _, b := range g.bindings {
			line := renderHelpBinding(b, s)
			if line != "" {
				lines = append(lines, line)
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, s.Dimmed.Render("Press any key to close"))

	content := strings.Join(lines, "\n")

	// ── Box sizing ──────────────────────────────────────────────────────────
	// Build the overlay box using a border-only style (no Width constraint)
	// to avoid lipgloss re-encoding already-styled binding strings.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00AFFF")).
		Padding(1, 2).
		Render(content)

	// Center the box vertically and horizontally.
	boxLines := strings.Split(box, "\n")
	boxH := len(boxLines)

	// Estimate box visual width from the widest line.
	maxLineW := 0
	for _, line := range boxLines {
		if rw := len([]rune(line)); rw > maxLineW {
			maxLineW = rw
		}
	}

	topPad := (height - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (width - maxLineW) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	var out strings.Builder
	padding := strings.Repeat(" ", leftPad)

	for i := 0; i < topPad; i++ {
		out.WriteByte('\n')
	}
	for _, line := range boxLines {
		out.WriteString(padding)
		out.WriteString(line)
		out.WriteByte('\n')
	}

	return out.String()
}

// renderHelpBinding formats a single key binding as "  key   description".
func renderHelpBinding(b key.Binding, s *Styles) string {
	const keyWidth = 12
	keyStr := b.Help().Key
	if keyStr == "" {
		return ""
	}
	desc := b.Help().Desc

	keyPart := s.HelpKey.Render(padRight(keyStr, keyWidth))
	descPart := s.HelpDesc.Render(desc)
	return "  " + keyPart + " " + descPart
}
