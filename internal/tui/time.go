// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/clock"
)

// DefaultClock is the default clock used for time operations.
// It can be replaced in tests with a mock clock.
//
//nolint:gochecknoglobals // Package-level default for dependency injection
var DefaultClock clock.Clock = clock.RealClock{}

// RelativeTime formats a time as a human-readable relative string.
// Examples: "just now", "2 minutes ago", "1 hour ago", "3 days ago", "2 weeks ago"
func RelativeTime(t time.Time) string {
	return RelativeTimeWith(t, DefaultClock)
}

// RelativeTimeWith formats a time as a human-readable relative string using the provided clock.
// This function allows for testable time-based formatting.
func RelativeTimeWith(t time.Time, c clock.Clock) string {
	now := c.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}
}
