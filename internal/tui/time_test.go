package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/clock"
)

// mockClock is a Clock implementation for testing that returns a fixed time.
type mockClock struct {
	fixedTime time.Time
}

func (m mockClock) Now() time.Time {
	return m.fixedTime
}

func TestRelativeTime(t *testing.T) {
	t.Parallel()
	// Use a fixed reference time for deterministic testing
	fixedNow := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	mc := mockClock{fixedTime: fixedNow}

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "just now",
			input:    fixedNow.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "1 minute ago",
			input:    fixedNow.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			input:    fixedNow.Add(-5 * time.Minute),
			expected: "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			input:    fixedNow.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "2 hours ago",
			input:    fixedNow.Add(-2 * time.Hour),
			expected: "2 hours ago",
		},
		{
			name:     "1 day ago",
			input:    fixedNow.Add(-24 * time.Hour),
			expected: "1 day ago",
		},
		{
			name:     "3 days ago",
			input:    fixedNow.Add(-3 * 24 * time.Hour),
			expected: "3 days ago",
		},
		{
			name:     "1 week ago",
			input:    fixedNow.Add(-7 * 24 * time.Hour),
			expected: "1 week ago",
		},
		{
			name:     "2 weeks ago",
			input:    fixedNow.Add(-14 * 24 * time.Hour),
			expected: "2 weeks ago",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Use RelativeTimeWith for deterministic testing
			result := RelativeTimeWith(tc.input, mc)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRelativeTime_WithDefaultClock(t *testing.T) {
	t.Parallel()
	// Test that RelativeTime still works with the default clock
	// This test uses real time so it's less deterministic but verifies the default path
	now := time.Now()
	result := RelativeTime(now.Add(-30 * time.Second))
	assert.Equal(t, "just now", result)
}

func TestRelativeTimeWith_EnsuresClockInterfaceIsUsed(t *testing.T) {
	t.Parallel()
	// Verify that we can pass any Clock implementation
	var c clock.Clock = clock.RealClock{}

	// Just verify it compiles and doesn't panic
	result := RelativeTimeWith(time.Now().Add(-1*time.Minute), c)
	assert.NotEmpty(t, result)
}
