package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/daemon"
)

// helpers ─────────────────────────────────────────────────────────────────────

func newPanel(w, h int) *LogPanel {
	p := NewLogPanel()
	p.SetSize(w, h)
	return p
}

func addEntries(p *LogPanel, levels []string) {
	for i, lvl := range levels {
		p.AddEntry(daemon.LogEntry{
			Timestamp: time.Now(),
			Level:     lvl,
			Message:   lvl + "-message",
			Source:    strings.Repeat("x", i), // unique per entry
		})
	}
}

// ── NewLogPanel ───────────────────────────────────────────────────────────────

func TestNewLogPanel(t *testing.T) {
	p := NewLogPanel()
	if p == nil {
		t.Fatal("NewLogPanel returned nil")
	}
	if !p.AutoScroll() {
		t.Error("expected auto-scroll enabled by default")
	}
	if p.Level() != LogLevelAll {
		t.Errorf("expected default level %q, got %q", LogLevelAll, p.Level())
	}
	if p.ScrollOffset() != 0 {
		t.Errorf("expected scroll offset 0, got %d", p.ScrollOffset())
	}
}

// ── Auto-scroll behavior ──────────────────────────────────────────────────────

func TestLogPanel_AutoScroll_EnabledByDefault(t *testing.T) {
	p := newPanel(80, 10)
	if !p.AutoScroll() {
		t.Error("auto-scroll should be enabled on creation")
	}
}

func TestLogPanel_ScrollUp_DisablesAutoScroll(t *testing.T) {
	p := newPanel(80, 10)
	p.ScrollUp(1)
	if p.AutoScroll() {
		t.Error("auto-scroll should be disabled after ScrollUp")
	}
}

func TestLogPanel_ScrollDown_ReEnablesAutoScrollAtBottom(t *testing.T) {
	p := newPanel(80, 10)
	p.ScrollUp(3)
	if p.AutoScroll() {
		t.Fatal("auto-scroll should be off after scrolling up")
	}
	// Scroll back down past the bottom.
	p.ScrollDown(10)
	if !p.AutoScroll() {
		t.Error("auto-scroll should re-enable when scrolling to bottom")
	}
	if p.ScrollOffset() != 0 {
		t.Errorf("scroll offset should be 0 at bottom, got %d", p.ScrollOffset())
	}
}

func TestLogPanel_JumpToBottom_EnablesAutoScroll(t *testing.T) {
	p := newPanel(80, 10)
	p.ScrollUp(5)
	p.JumpToBottom()
	if !p.AutoScroll() {
		t.Error("JumpToBottom should re-enable auto-scroll")
	}
	if p.ScrollOffset() != 0 {
		t.Errorf("JumpToBottom should set offset to 0, got %d", p.ScrollOffset())
	}
}

func TestLogPanel_JumpToTop_DisablesAutoScroll(t *testing.T) {
	p := newPanel(80, 5)
	// Add more entries than visible.
	for i := 0; i < 20; i++ {
		p.AddEntry(makeEntry("info", "msg"))
	}
	p.JumpToTop()
	if p.AutoScroll() {
		t.Error("JumpToTop should disable auto-scroll")
	}
}

// ── Scroll position tracking ───────────────────────────────────────────────────

func TestLogPanel_ScrollUp_IncreasesOffset(t *testing.T) {
	p := newPanel(80, 10)
	p.ScrollUp(3)
	if p.ScrollOffset() != 3 {
		t.Errorf("expected offset 3, got %d", p.ScrollOffset())
	}
}

func TestLogPanel_ScrollDown_DecreasesOffset(t *testing.T) {
	p := newPanel(80, 10)
	p.ScrollUp(5)
	p.ScrollDown(2)
	if p.ScrollOffset() != 3 {
		t.Errorf("expected offset 3, got %d", p.ScrollOffset())
	}
}

// ── Level display ──────────────────────────────────────────────────────────────

func TestLogPanel_View_RespectsLevelFilter(t *testing.T) {
	p := newPanel(120, 20)
	addEntries(p, []string{"debug", "info", "warn", "error"})

	p.SetLevel(LogLevelError)
	view := p.View()
	if strings.Contains(view, "debug-message") {
		t.Error("error filter should not show debug messages")
	}
	if !strings.Contains(view, "error-message") {
		t.Error("error filter should show error messages")
	}
}

func TestLogPanel_View_ShowsAllLevels(t *testing.T) {
	p := newPanel(120, 20)
	addEntries(p, []string{"debug", "info", "warn", "error"})

	view := p.View()
	for _, level := range []string{"debug", "info", "warn", "error"} {
		if !strings.Contains(view, level+"-message") {
			t.Errorf("all-filter should show %s messages", level)
		}
	}
}

func TestLogPanel_View_EmptyBuffer(_ *testing.T) {
	p := newPanel(80, 10)
	// Should return empty lines without panicking.
	_ = p.View()
}

func TestLogPanel_SetLevel_ResetsOffsetWhenAutoScroll(t *testing.T) {
	p := newPanel(80, 10)
	// With auto-scroll on, SetLevel should not change the offset (already 0).
	p.SetLevel(LogLevelWarn)
	if p.ScrollOffset() != 0 {
		t.Errorf("offset should remain 0, got %d", p.ScrollOffset())
	}
}

// ── ResetForTask ───────────────────────────────────────────────────────────────

func TestLogPanel_ResetForTask(t *testing.T) {
	p := newPanel(80, 10)
	addEntries(p, []string{"info", "error"})
	p.ScrollUp(3)
	p.ResetForTask()

	if p.Buffer().Len() != 0 {
		t.Errorf("buffer should be cleared after reset, got %d entries", p.Buffer().Len())
	}
	if !p.AutoScroll() {
		t.Error("auto-scroll should be re-enabled after reset")
	}
	if p.ScrollOffset() != 0 {
		t.Errorf("scroll offset should be 0 after reset, got %d", p.ScrollOffset())
	}
}

// ── View height ────────────────────────────────────────────────────────────────

func TestLogPanel_View_ZeroHeight(t *testing.T) {
	p := newPanel(80, 0)
	view := p.View()
	if view != "" {
		t.Errorf("zero-height panel should return empty string, got %q", view)
	}
}

func TestLogPanel_View_LinesMatchHeight(t *testing.T) {
	height := 5
	p := newPanel(80, height)
	// Add fewer entries than height — should pad.
	addEntries(p, []string{"info", "warn"})
	view := p.View()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Errorf("expected %d lines in view, got %d", height, len(lines))
	}
}
