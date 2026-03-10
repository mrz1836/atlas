package dashboard

import (
	"strings"

	"github.com/mrz1836/atlas/internal/tui"
)

const (
	// splitVerticalChar is the Unicode vertical bar used between panes.
	splitVerticalChar = "│"

	// headerHeight is the number of lines occupied by the dashboard header.
	headerHeight = 3
	// footerHeight is the number of lines occupied by the status bar footer.
	footerHeight = 1

	// defaultSplitRatio is the default fraction of terminal width for the left pane.
	defaultSplitRatio = 0.30
)

// Layout manages the split-pane layout geometry for the dashboard.
// It computes pane dimensions and renders content side-by-side with a vertical divider.
//
// Responsive breakpoints (inherited from tui.TerminalWidthNarrow / tui.TerminalWidthWide):
//   - narrow  < 80: single-column mode (no split, full-width left pane)
//   - standard 80-119: standard split-pane (30/70 default)
//   - wide   >= 120: expanded split-pane (richer detail in right pane)
type Layout struct {
	// Width is the total terminal width in columns.
	Width int
	// Height is the total terminal height in rows.
	Height int
	// SplitRatio is the fraction of Width allocated to the left pane (0.0–1.0).
	// Default is 0.30 (30% left, 70% right).
	SplitRatio float64
	// Mode controls the overall rendering mode.
	Mode ViewMode
}

// NewLayout creates a Layout with sensible defaults.
func NewLayout(width, height int) Layout {
	return Layout{
		Width:      width,
		Height:     height,
		SplitRatio: defaultSplitRatio,
		Mode:       ViewModeList,
	}
}

// LeftWidth returns the number of columns available to the left pane.
// In narrow mode the full width is returned (single-column layout).
// Accounts for the single-character divider column.
func (l *Layout) LeftWidth() int {
	if l.isNarrow() {
		return l.Width
	}
	// Subtract 1 for the vertical divider character.
	left := int(float64(l.Width-1) * l.SplitRatio)
	if left < 1 {
		left = 1
	}
	return left
}

// RightWidth returns the number of columns available to the right pane.
// In narrow mode this returns 0 (no right pane in single-column layout).
func (l *Layout) RightWidth() int {
	if l.isNarrow() {
		return 0
	}
	return l.Width - l.LeftWidth() - 1 // 1 for divider
}

// ContentHeight returns the number of rows available for panel content.
// This subtracts the fixed header and footer rows from the total height.
func (l *Layout) ContentHeight() int {
	h := l.Height - headerHeight - footerHeight
	if h < 1 {
		h = 1
	}
	return h
}

// Render combines left and right pane content into a single display string.
// In narrow mode, only the left pane is rendered (full width).
// In standard/wide mode, a vertical divider separates the two panes.
//
// Both left and right are expected to be pre-rendered strings (lines joined by "\n").
// Each string should contain exactly ContentHeight() lines; this function will pad
// or truncate as needed to keep panes aligned.
func (l *Layout) Render(left, right string) string {
	leftLines := splitLines(left)
	h := l.ContentHeight()

	// Narrow mode: render left pane only.
	if l.isNarrow() {
		padded := padOrTruncate(leftLines, h)
		return strings.Join(padded, "\n")
	}

	rightLines := splitLines(right)
	leftW := l.LeftWidth()
	rightW := l.RightWidth()

	leftPadded := padOrTruncate(leftLines, h)
	rightPadded := padOrTruncate(rightLines, h)

	var sb strings.Builder
	sb.Grow((leftW + 1 + rightW + 1) * h) // +1 divider, +1 newline per row

	for i := 0; i < h; i++ {
		leftCell := padOrTruncateLine(leftPadded[i], leftW)
		rightCell := padOrTruncateLine(rightPadded[i], rightW)

		sb.WriteString(leftCell)
		sb.WriteString(splitVerticalChar)
		sb.WriteString(rightCell)
		if i < h-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// isNarrow returns true when the terminal is below the narrow threshold.
// Unexported helper; must appear after all exported methods (funcorder).
func (l *Layout) isNarrow() bool {
	return l.Width < tui.TerminalWidthNarrow
}

// splitLines splits a string by newlines into a slice of lines.
func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

// padOrTruncate ensures a slice of lines has exactly n entries.
// Short slices are padded with empty strings; long slices are truncated.
func padOrTruncate(lines []string, n int) []string {
	if len(lines) == n {
		return lines
	}
	out := make([]string, n)
	copy(out, lines)
	// remaining entries are already "" (zero value)
	return out
}

// padOrTruncateLine pads a single line to width with spaces, or truncates it.
// Does not account for ANSI escape codes — callers should strip styling before
// measuring if precise width control is needed. For layout skeleton rendering
// (no ANSI) this is accurate.
func padOrTruncateLine(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}
