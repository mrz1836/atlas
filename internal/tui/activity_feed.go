// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/mrz1836/atlas/internal/ai"
)

// ActivityFeedConfig configures the activity feed display.
type ActivityFeedConfig struct {
	// MaxLines is the maximum number of lines to display.
	MaxLines int

	// Width is the box width. 0 means auto-detect.
	Width int

	// Title is the box title.
	Title string

	// ShowTimestamps shows timestamps for each activity.
	ShowTimestamps bool
}

// DefaultActivityFeedConfig returns the default configuration.
func DefaultActivityFeedConfig() ActivityFeedConfig {
	return ActivityFeedConfig{
		MaxLines:       5,
		Width:          60,
		Title:          "AI Activity",
		ShowTimestamps: false,
	}
}

// ActivityFeed displays a scrolling list of AI activity events.
// It is thread-safe and can receive updates from concurrent goroutines.
type ActivityFeed struct {
	config     ActivityFeedConfig
	activities []ai.ActivityEvent
	mu         sync.Mutex
	w          io.Writer
	styles     *OutputStyles
	lastRender time.Time
}

// NewActivityFeed creates a new ActivityFeed with the given writer and config.
func NewActivityFeed(w io.Writer, config ActivityFeedConfig) *ActivityFeed {
	if config.MaxLines <= 0 {
		config.MaxLines = 5
	}
	if config.Width <= 0 {
		config.Width = 60
	}
	if config.Title == "" {
		config.Title = "AI Activity"
	}

	return &ActivityFeed{
		config:     config,
		activities: make([]ai.ActivityEvent, 0, config.MaxLines),
		w:          w,
		styles:     GetOutputStyles(),
	}
}

// Add adds a new activity event to the feed.
// Thread-safe for concurrent updates.
func (f *ActivityFeed) Add(event ai.ActivityEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Add the new event
	f.activities = append(f.activities, event)

	// Keep only the last MaxLines events
	if len(f.activities) > f.config.MaxLines {
		f.activities = f.activities[len(f.activities)-f.config.MaxLines:]
	}
}

// Render returns the formatted activity feed box as a string.
// Thread-safe for concurrent reads.
func (f *ActivityFeed) Render() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.activities) == 0 {
		return ""
	}

	return f.renderBox()
}

// RenderAndWrite renders the activity feed and writes it to the writer.
// It handles line clearing for animated updates.
func (f *ActivityFeed) RenderAndWrite() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.activities) == 0 {
		return
	}

	// Clear previous lines if this isn't the first render
	if !f.lastRender.IsZero() {
		f.clearPreviousRender()
	}

	// Render and write
	rendered := f.renderBox()
	_, _ = fmt.Fprint(f.w, rendered)

	f.lastRender = time.Now()
}

// Clear clears all activities from the feed and the display.
func (f *ActivityFeed) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.lastRender.IsZero() && len(f.activities) > 0 {
		f.clearPreviousRender()
	}

	f.activities = f.activities[:0]
	f.lastRender = time.Time{}
}

// ActivityCount returns the current number of activities in the feed.
func (f *ActivityFeed) ActivityCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.activities)
}

// clearPreviousRender clears the previously rendered lines.
func (f *ActivityFeed) clearPreviousRender() {
	// Calculate lines to clear: box lines = activities + 2 (top/bottom border) + 1 (title divider)
	linesToClear := len(f.activities) + 3

	// Move cursor up and clear each line
	for i := 0; i < linesToClear; i++ {
		_, _ = fmt.Fprint(f.w, "\033[A\033[K") // Move up, clear line
	}
}

// renderBox renders the activity feed as a bordered box.
func (f *ActivityFeed) renderBox() string {
	innerWidth := f.config.Width - 4 // Account for borders and padding

	var sb strings.Builder

	// Top border with title
	titleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	title := titleStyle.Render(f.config.Title)
	topBorder := fmt.Sprintf("┌─ %s %s┐\n",
		title,
		strings.Repeat("─", max(0, innerWidth-len(f.config.Title)-3)))
	sb.WriteString(topBorder)

	// Activity lines
	for i, event := range f.activities {
		line := f.formatActivity(event, innerWidth-2) // -2 for icon and space

		// Last activity gets a different icon
		icon := "●"
		if i == len(f.activities)-1 {
			icon = "⋮"
		}

		// Color the icon based on activity type
		iconStyle := f.getIconStyle(event.Type)
		coloredIcon := iconStyle.Render(icon)

		fmt.Fprintf(&sb, "│ %s %s │\n", coloredIcon, padRight(line, innerWidth-2))
	}

	// Fill empty lines if we have fewer than MaxLines
	for i := len(f.activities); i < f.config.MaxLines; i++ {
		fmt.Fprintf(&sb, "│ %s │\n", strings.Repeat(" ", innerWidth))
	}

	// Bottom border
	fmt.Fprintf(&sb, "└%s┘\n", strings.Repeat("─", f.config.Width-2))

	return sb.String()
}

// formatActivity formats a single activity event for display.
func (f *ActivityFeed) formatActivity(event ai.ActivityEvent, maxWidth int) string {
	var parts []string

	// Add timestamp if configured
	if f.config.ShowTimestamps {
		ts := event.Timestamp.Format("15:04:05")
		parts = append(parts, f.styles.Dim.Render(ts))
	}

	// Add the message
	msg := event.FormatMessage()
	parts = append(parts, msg)

	result := strings.Join(parts, " ")

	// Truncate if too long
	if len(result) > maxWidth {
		result = result[:maxWidth-3] + "..."
	}

	return result
}

// getIconStyle returns the lipgloss style for the activity type icon.
func (f *ActivityFeed) getIconStyle(actType ai.ActivityType) lipgloss.Style {
	switch actType {
	case ai.ActivityReading:
		return lipgloss.NewStyle().Foreground(ColorPrimary)
	case ai.ActivityWriting:
		return lipgloss.NewStyle().Foreground(ColorSuccess)
	case ai.ActivityThinking:
		return lipgloss.NewStyle().Foreground(ColorMuted)
	case ai.ActivityPlanning:
		return lipgloss.NewStyle().Foreground(ColorWarning)
	case ai.ActivityImplementing:
		return lipgloss.NewStyle().Foreground(ColorSuccess)
	case ai.ActivityVerifying:
		return lipgloss.NewStyle().Foreground(ColorPrimary)
	case ai.ActivityAnalyzing:
		return lipgloss.NewStyle().Foreground(ColorPrimary)
	case ai.ActivitySearching:
		return lipgloss.NewStyle().Foreground(ColorPrimary)
	case ai.ActivityExecuting:
		return lipgloss.NewStyle().Foreground(ColorWarning)
	default:
		return lipgloss.NewStyle().Foreground(ColorMuted)
	}
}
