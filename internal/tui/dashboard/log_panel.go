package dashboard

import (
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/daemon"
)

// levelBadge maps a log level to a fixed-width display badge.
// All badges are 7 chars wide (including brackets) for alignment.
//
//nolint:gochecknoglobals // Intentional package-level constant for log level display
var levelBadge = map[string]string{
	"debug": "[DEBUG]",
	"info":  "[INFO ]",
	"warn":  "[WARN ]",
	"error": "[ERROR]",
}

// LogPanel is a scrollable log display with auto-scroll, level filtering, and
// optional search highlighting. It is a pure value-type component (no bubbletea
// state machine); the parent model drives mutations via method calls.
type LogPanel struct {
	// buffer holds all received log entries for the current task.
	buffer *LogBuffer
	// search is the active search state (may be inactive).
	search *LogSearch

	// scrollOffset is the number of lines scrolled up from the bottom.
	// 0 = at the tail (auto-scroll position).
	scrollOffset int
	// autoScroll is true when the view should follow new entries.
	autoScroll bool
	// level is the active level filter (LogLevelAll/Info/Warn/Error).
	level string

	// width and height are the panel dimensions in terminal columns/rows.
	width, height int
}

// NewLogPanel creates a LogPanel with auto-scroll enabled and "all" level filter.
func NewLogPanel() *LogPanel {
	return &LogPanel{
		buffer:     NewLogBuffer(),
		search:     NewLogSearch(),
		autoScroll: true,
		level:      LogLevelAll,
	}
}

// AddEntry appends a new log entry to the buffer.
// If auto-scroll is active the panel will show the new entry immediately.
func (p *LogPanel) AddEntry(entry daemon.LogEntry) {
	p.buffer.Add(entry)
}

// SetLevel updates the active level filter and resets the scroll position.
func (p *LogPanel) SetLevel(level string) {
	p.level = level
	if p.autoScroll {
		p.scrollOffset = 0
	}
}

// SetSize updates the panel dimensions.
func (p *LogPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// ScrollUp moves the viewport up by n lines, disabling auto-scroll.
func (p *LogPanel) ScrollUp(n int) {
	p.autoScroll = false
	p.scrollOffset += n
}

// ScrollDown moves the viewport down by n lines.
// Re-enables auto-scroll when at the bottom.
func (p *LogPanel) ScrollDown(n int) {
	p.scrollOffset -= n
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
		p.autoScroll = true
	}
}

// JumpToBottom scrolls to the tail and re-enables auto-scroll.
func (p *LogPanel) JumpToBottom() {
	p.scrollOffset = 0
	p.autoScroll = true
}

// JumpToTop scrolls to the oldest visible entry.
func (p *LogPanel) JumpToTop() {
	p.autoScroll = false
	entries := p.buffer.Filter(p.level)
	visible := p.visibleLines()
	if len(entries) > visible {
		p.scrollOffset = len(entries) - visible
	} else {
		p.scrollOffset = 0
	}
}

// AutoScroll reports whether the panel is currently in auto-scroll mode.
func (p *LogPanel) AutoScroll() bool { return p.autoScroll }

// ScrollOffset returns the current scroll offset (lines from bottom).
func (p *LogPanel) ScrollOffset() int { return p.scrollOffset }

// Level returns the current level filter string.
func (p *LogPanel) Level() string { return p.level }

// Buffer returns the underlying LogBuffer (used for testing and search).
func (p *LogPanel) Buffer() *LogBuffer { return p.buffer }

// Search returns the active LogSearch state.
func (p *LogPanel) Search() *LogSearch { return p.search }

// ResetForTask clears the buffer and resets scroll/search state for a new task.
func (p *LogPanel) ResetForTask() {
	p.buffer.Clear()
	p.search.Reset()
	p.scrollOffset = 0
	p.autoScroll = true
}

// View renders the log panel as a string of exactly p.height lines,
// each padded to p.width columns.
func (p *LogPanel) View() string {
	entries := p.buffer.Filter(p.level)
	visible := p.visibleLines()
	if visible <= 0 {
		return ""
	}

	// Clamp scroll offset.
	maxOffset := len(entries) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := p.scrollOffset
	if offset > maxOffset {
		offset = maxOffset
	}

	// Determine window of entries to display.
	start := len(entries) - visible - offset
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(entries) {
		end = len(entries)
	}

	window := entries[start:end]
	lines := make([]string, 0, visible)

	styles := GetStyles()

	// Pre-compute match set for quick lookup.
	var matchSet map[int]bool
	if p.search.IsActive() && p.search.HasMatches() {
		matchSet = make(map[int]bool)
		for _, idx := range p.search.Matches() {
			absIdx := idx - start
			if absIdx >= 0 && absIdx < len(window) {
				matchSet[absIdx] = true
			}
		}
	}

	for i, entry := range window {
		line := p.formatEntry(entry, styles)
		// Highlight if this entry is a search match.
		if matchSet != nil && matchSet[i] {
			line = p.search.Highlight(line)
		}
		lines = append(lines, line)
	}

	// Pad to fill the panel height.
	for len(lines) < visible {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// formatEntry formats a single log entry as a display line.
// Format: "14:22:45 [INFO ] Running magex lint"
func (p *LogPanel) formatEntry(entry daemon.LogEntry, styles *Styles) string {
	ts := entry.Timestamp.Format("15:04:05")
	badge, ok := levelBadge[entry.Level]
	if !ok {
		badge = fmt.Sprintf("[%-5s]", strings.ToUpper(entry.Level))
	}

	// Truncate message to fit panel width (ts=8, space=1, badge=7, space=1 = 17 prefix chars).
	prefix := ts + " " + badge + " "
	msg := entry.Message
	maxMsgLen := p.width - len(prefix)
	if maxMsgLen > 0 && len(msg) > maxMsgLen {
		msg = msg[:maxMsgLen]
	}

	line := prefix + msg

	// Apply level coloring.
	switch entry.Level {
	case "debug":
		return styles.LogDebug.Render(line)
	case "warn":
		return styles.LogWarn.Render(line)
	case "error":
		return styles.LogError.Render(line)
	default: // info or unknown
		return styles.LogInfo.Render(line)
	}
}

// visibleLines returns the number of log lines that fit in the panel.
func (p *LogPanel) visibleLines() int {
	if p.height <= 0 {
		return 0
	}
	return p.height
}
