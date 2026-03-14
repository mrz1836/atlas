package dashboard_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

// ── HelpOverlay: visibility toggle ────────────────────────────────────────────

func TestHelpOverlay_StartsHidden(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	if h.IsVisible() {
		t.Error("NewHelpOverlay should start hidden")
	}
}

func TestHelpOverlay_Toggle_ShowsAndHides(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)

	h.Toggle()
	if !h.IsVisible() {
		t.Error("Toggle() should show the overlay")
	}

	h.Toggle()
	if h.IsVisible() {
		t.Error("Toggle() again should hide the overlay")
	}
}

func TestHelpOverlay_ShowHide(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)

	h.Show()
	if !h.IsVisible() {
		t.Error("Show() should make overlay visible")
	}

	h.Hide()
	if h.IsVisible() {
		t.Error("Hide() should make overlay invisible")
	}
}

// ── HelpOverlay: View content ─────────────────────────────────────────────────

func TestHelpOverlay_View_EmptyWhenHidden(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	// hidden by default
	view := h.View(120, 40)
	if view != "" {
		t.Errorf("View() should return empty string when hidden, got %q", view)
	}
}

func TestHelpOverlay_View_ContainsKeyGroups(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	h.Show()

	// Strip ANSI codes: lipgloss renders Bold+Underline char-by-char, breaking plain Contains.
	view := stripConfirmANSI(h.View(120, 40))

	for _, group := range []string{"Navigation", "Actions", "Log", "General"} {
		if !strings.Contains(view, group) {
			t.Errorf("help overlay should contain group %q", group)
		}
	}
}

func TestHelpOverlay_View_ContainsKeyBindings(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	h.Show()

	// Strip ANSI codes before checking text content.
	view := stripConfirmANSI(h.View(120, 40))

	// Navigation keys
	for _, k := range []string{"↑/k", "↓/j", "esc", "?", "q"} {
		if !strings.Contains(view, k) {
			t.Errorf("help overlay should contain key binding %q", k)
		}
	}

	// Action key descriptions
	for _, desc := range []string{"approve", "reject", "pause", "resume", "abandon", "destroy"} {
		if !strings.Contains(view, desc) {
			t.Errorf("help overlay should contain action %q", desc)
		}
	}
}

func TestHelpOverlay_View_ContainsDismissHint(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	h.Show()

	view := stripConfirmANSI(h.View(120, 40))
	if !strings.Contains(view, "any key") && !strings.Contains(view, "close") {
		t.Errorf("help overlay should mention how to dismiss it, view:\n%s", view)
	}
}

func TestHelpOverlay_View_NarrowTerminal(t *testing.T) {
	t.Parallel()
	km := dashboard.DefaultKeyMap()
	h := dashboard.NewHelpOverlay(km)
	h.Show()

	// Should not panic at narrow widths.
	view := h.View(40, 20)
	if view == "" {
		t.Error("View() should return content even at narrow widths")
	}
}

// ── HelpOverlay: dismissal via model '?' key ──────────────────────────────────

func TestModel_HelpKey_TogglesHelpOverlay(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Press '?' to open help.
	pressKey(t, m, "?")
	// viewOf calls stripConfirmANSI for us.
	view := viewOf(t, m)
	if !strings.Contains(view, "Navigation") && !strings.Contains(view, "Actions") &&
		!strings.Contains(view, "approve") && !strings.Contains(view, "Keyboard") {
		t.Errorf("help overlay should appear after '?' key, view:\n%s", view)
	}
}

func TestModel_HelpKey_AnyKeyDismisses(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Open help.
	pressKey(t, m, "?")

	// Any key should dismiss.
	pressKey(t, m, "a") // would normally be "approve" but overlay intercepts first

	view := viewOf(t, m)
	// Help overlay should be gone (no group headers).
	if strings.Contains(view, "Navigation") && strings.Contains(view, "Actions") &&
		strings.Contains(view, "General") {
		t.Error("help overlay should be dismissed after pressing any key")
	}
}

func TestModel_EscKey_DismissesHelp(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Open help.
	pressKey(t, m, "?")
	// Esc to dismiss.
	pressKey(t, m, "esc")

	view := viewOf(t, m)
	// Overlay gone.
	if strings.Contains(view, "Navigation") && strings.Contains(view, "Log Controls") {
		t.Error("help overlay should be dismissed after esc")
	}
}

// ── Layout width adaptation ───────────────────────────────────────────────────

func TestLayout_Narrow_SingleColumn(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(60, 24)
	if !l.IsNarrow() {
		t.Error("layout at 60 cols should be narrow")
	}
	if l.RightWidth() != 0 {
		t.Errorf("narrow layout should have 0 right width, got %d", l.RightWidth())
	}
	if l.LeftWidth() != 60 {
		t.Errorf("narrow layout left width should equal terminal width (60), got %d", l.LeftWidth())
	}
}

func TestLayout_Standard_SplitPane(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(100, 24)
	if l.IsNarrow() {
		t.Error("layout at 100 cols should not be narrow")
	}
	if l.IsWide() {
		t.Error("layout at 100 cols should not be wide")
	}
	if l.LeftWidth() <= 0 {
		t.Error("standard layout should have positive left width")
	}
	if l.RightWidth() <= 0 {
		t.Error("standard layout should have positive right width")
	}
	// Verify total adds up (left + divider + right == total).
	total := l.LeftWidth() + 1 + l.RightWidth()
	if total != 100 {
		t.Errorf("standard layout widths should sum to terminal width: %d+1+%d=%d != 100", l.LeftWidth(), l.RightWidth(), total)
	}
}

func TestLayout_Wide_IsWide(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(200, 40)
	if !l.IsWide() {
		t.Error("layout at 200 cols should be wide")
	}
	// Wide layouts still use SplitRatio (default 30%) — more space goes to right pane.
	wideLeft := l.LeftWidth()
	if wideLeft <= 0 {
		t.Errorf("wide layout should have positive left width, got %d", wideLeft)
	}
	if wideLeft >= 200 {
		t.Errorf("wide layout left width (%d) should be less than terminal width (200)", wideLeft)
	}
}

func TestLayout_SetSize(t *testing.T) {
	t.Parallel()
	l := dashboard.NewLayout(80, 24)
	l.SetSize(120, 40)
	if l.Width != 120 || l.Height != 40 {
		t.Errorf("SetSize: got %dx%d, want 120x40", l.Width, l.Height)
	}
}

// ── StylesForProfile ─────────────────────────────────────────────────────────

func TestStylesForProfile_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	s := dashboard.StylesForProfile(colorprofile.Ascii)
	if s == nil {
		t.Error("StylesForProfile should never return nil for Ascii profile")
	}
	s2 := dashboard.StylesForProfile(colorprofile.TrueColor)
	if s2 == nil {
		t.Error("StylesForProfile should never return nil for TrueColor profile")
	}
}

// ── Reconnect state machine ───────────────────────────────────────────────────

func TestModel_DisconnectedMsg_SetsReconnecting(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	m.Update(dashboard.DisconnectedMsg{})
	if m.ConnState() != dashboard.ConnectionStateReconnecting {
		t.Errorf("DisconnectedMsg should set state to Reconnecting, got %v", m.ConnState())
	}
}

func TestModel_ReconnectedMsg_SetsConnected(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	// First disconnect.
	m.Update(dashboard.DisconnectedMsg{})
	// Then reconnect.
	m.Update(dashboard.ReconnectedMsg{})
	if m.ConnState() != dashboard.ConnectionStateConnected {
		t.Errorf("ReconnectedMsg should set state to Connected, got %v", m.ConnState())
	}
}

// ── WindowSizeMsg handling ────────────────────────────────────────────────────

func TestModel_WindowSizeMsg_UpdatesDimensions(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Test various widths including all breakpoints.
	for _, tc := range []struct {
		width, height int
	}{
		{60, 20},  // narrow
		{80, 24},  // standard lower bound
		{119, 30}, // standard upper bound
		{120, 40}, // wide lower bound
		{200, 60}, // very wide
	} {
		_, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
		// Verify no panic and model remains usable.
		_ = m.View()
	}
}
