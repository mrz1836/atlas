package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/ai"
)

func TestDefaultActivityFeedConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultActivityFeedConfig()

	if cfg.MaxLines != 5 {
		t.Errorf("MaxLines = %d, want 5", cfg.MaxLines)
	}
	if cfg.Width != 60 {
		t.Errorf("Width = %d, want 60", cfg.Width)
	}
	if cfg.Title != "AI Activity" {
		t.Errorf("Title = %q, want %q", cfg.Title, "AI Activity")
	}
	if cfg.ShowTimestamps {
		t.Error("ShowTimestamps should be false by default")
	}
}

func TestNewActivityFeed(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, DefaultActivityFeedConfig())

	if feed == nil {
		t.Fatal("NewActivityFeed returned nil")
	}

	if feed.ActivityCount() != 0 {
		t.Errorf("Initial ActivityCount = %d, want 0", feed.ActivityCount())
	}
}

func TestNewActivityFeed_DefaultValues(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}

	// Test with zero/empty config
	feed := NewActivityFeed(buf, ActivityFeedConfig{})

	if feed == nil {
		t.Fatal("NewActivityFeed returned nil")
	}

	// Should use defaults
	if feed.config.MaxLines != 5 {
		t.Errorf("MaxLines = %d, want 5 (default)", feed.config.MaxLines)
	}
	if feed.config.Width != 60 {
		t.Errorf("Width = %d, want 60 (default)", feed.config.Width)
	}
	if feed.config.Title != "AI Activity" {
		t.Errorf("Title = %q, want %q (default)", feed.config.Title, "AI Activity")
	}
}

func TestActivityFeed_Add(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, DefaultActivityFeedConfig())

	// Add an event
	feed.Add(ai.ActivityEvent{
		Timestamp: time.Now(),
		Type:      ai.ActivityReading,
		Message:   "Reading file",
		File:      "test.go",
	})

	if feed.ActivityCount() != 1 {
		t.Errorf("ActivityCount = %d, want 1", feed.ActivityCount())
	}
}

func TestActivityFeed_Add_MaxLines(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines: 3,
	})

	// Add more events than MaxLines
	for i := 0; i < 5; i++ {
		feed.Add(ai.ActivityEvent{
			Timestamp: time.Now(),
			Type:      ai.ActivityReading,
			Message:   "Event " + string(rune('A'+i)),
		})
	}

	// Should only keep MaxLines events
	if feed.ActivityCount() != 3 {
		t.Errorf("ActivityCount = %d, want 3 (MaxLines)", feed.ActivityCount())
	}
}

func TestActivityFeed_Render_Empty(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, DefaultActivityFeedConfig())

	// Render with no events should return empty string
	result := feed.Render()
	if result != "" {
		t.Errorf("Render() with no events = %q, want empty", result)
	}
}

func TestActivityFeed_Render_WithEvents(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines: 3,
		Width:    40,
		Title:    "Test Activity",
	})

	// Add some events
	feed.Add(ai.ActivityEvent{
		Type:    ai.ActivityReading,
		Message: "Reading main.go",
	})
	feed.Add(ai.ActivityEvent{
		Type:    ai.ActivityWriting,
		Message: "Writing output",
	})

	result := feed.Render()

	// Should contain box borders
	if !strings.Contains(result, "┌") || !strings.Contains(result, "┐") {
		t.Error("Render should contain top border characters")
	}
	if !strings.Contains(result, "└") || !strings.Contains(result, "┘") {
		t.Error("Render should contain bottom border characters")
	}
	if !strings.Contains(result, "│") {
		t.Error("Render should contain side border characters")
	}

	// Should contain the title
	if !strings.Contains(result, "Test Activity") {
		t.Error("Render should contain the title")
	}

	// Should contain event messages
	if !strings.Contains(result, "Reading") || !strings.Contains(result, "main.go") {
		t.Error("Render should contain first event message")
	}
	if !strings.Contains(result, "Writing") || !strings.Contains(result, "output") {
		t.Error("Render should contain second event message")
	}
}

func TestActivityFeed_Render_WithTimestamps(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines:       3,
		Width:          60,
		ShowTimestamps: true,
	})

	// Add an event
	now := time.Now()
	feed.Add(ai.ActivityEvent{
		Timestamp: now,
		Type:      ai.ActivityReading,
		Message:   "Reading file",
	})

	result := feed.Render()

	// Should contain timestamp format
	expectedTime := now.Format("15:04:05")
	if !strings.Contains(result, expectedTime) {
		t.Errorf("Render should contain timestamp %s", expectedTime)
	}
}

func TestActivityFeed_Clear(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, DefaultActivityFeedConfig())

	// Add events
	feed.Add(ai.ActivityEvent{Type: ai.ActivityReading, Message: "Test"})
	feed.Add(ai.ActivityEvent{Type: ai.ActivityWriting, Message: "Test"})

	if feed.ActivityCount() != 2 {
		t.Errorf("ActivityCount before clear = %d, want 2", feed.ActivityCount())
	}

	// Clear
	feed.Clear()

	if feed.ActivityCount() != 0 {
		t.Errorf("ActivityCount after clear = %d, want 0", feed.ActivityCount())
	}

	// Render should return empty
	if result := feed.Render(); result != "" {
		t.Errorf("Render after clear = %q, want empty", result)
	}
}

func TestActivityFeed_Concurrent(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines: 10,
	})

	// Add events concurrently
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			feed.Add(ai.ActivityEvent{
				Type:    ai.ActivityReading,
				Message: "Concurrent event",
			})
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have exactly 10 events
	count := feed.ActivityCount()
	if count != 10 {
		t.Errorf("ActivityCount after concurrent adds = %d, want 10", count)
	}
}

func TestActivityFeed_IconColors(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines: 5,
		Width:    60,
	})

	// Add events of different types to test color styling
	eventTypes := []ai.ActivityType{
		ai.ActivityReading,
		ai.ActivityWriting,
		ai.ActivityThinking,
		ai.ActivityPlanning,
		ai.ActivityVerifying,
	}

	for _, t := range eventTypes {
		feed.Add(ai.ActivityEvent{
			Type:    t,
			Message: "Test " + string(t),
		})
	}

	result := feed.Render()

	// Should render without panic
	if result == "" {
		t.Error("Render with multiple event types should not be empty")
	}
}

func TestActivityFeed_TruncateLongMessages(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	feed := NewActivityFeed(buf, ActivityFeedConfig{
		MaxLines: 3,
		Width:    30, // Narrow width to trigger truncation
	})

	// Add event with very long message
	longMessage := "This is a very long message that should be truncated because it exceeds the available width"
	feed.Add(ai.ActivityEvent{
		Type:    ai.ActivityReading,
		Message: longMessage,
	})

	result := feed.Render()

	// Result should fit within the box width
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		// Each line should not exceed the width significantly
		// (allowing for ANSI codes which don't count toward visible width)
		visibleLen := len(stripANSI(line))
		if visibleLen > 100 { // generous threshold
			t.Errorf("Line too long: %d characters", visibleLen)
		}
	}

	// Should contain truncation indicator
	if !strings.Contains(result, "...") {
		t.Error("Long message should be truncated with ...")
	}
}
