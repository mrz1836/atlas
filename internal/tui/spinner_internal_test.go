package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFormatElapsedTime tests the unexported formatElapsedTime function.
// This covers Task 6.4: "Add tests for elapsed time display threshold"
func TestFormatElapsedTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "30 seconds",
			duration: 30 * time.Second,
			expected: "(30s elapsed)",
		},
		{
			name:     "45 seconds",
			duration: 45 * time.Second,
			expected: "(45s elapsed)",
		},
		{
			name:     "59 seconds",
			duration: 59 * time.Second,
			expected: "(59s elapsed)",
		},
		{
			name:     "1 minute exactly",
			duration: 60 * time.Second,
			expected: "(1m 0s elapsed)",
		},
		{
			name:     "1 minute 15 seconds",
			duration: 75 * time.Second,
			expected: "(1m 15s elapsed)",
		},
		{
			name:     "2 minutes 30 seconds",
			duration: 150 * time.Second,
			expected: "(2m 30s elapsed)",
		},
		{
			name:     "5 minutes",
			duration: 5 * time.Minute,
			expected: "(5m 0s elapsed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatElapsedTime(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestElapsedTimeThreshold verifies the threshold constant is correctly set.
func TestElapsedTimeThreshold(t *testing.T) {
	// Verify the threshold is 30 seconds as per AC #8
	assert.Equal(t, 30*time.Second, ElapsedTimeThreshold,
		"Elapsed time should be displayed after 30 seconds per AC #8")
}

// TestNoopSpinnerInternal tests the NoopSpinner methods directly for coverage.
// The internal test package can access unexported methods if needed.
func TestNoopSpinnerInternal(t *testing.T) {
	t.Parallel()
	spinner := &NoopSpinner{}

	// Update should be a no-op and not panic
	spinner.Update("test message")
	spinner.Update("")
	spinner.Update("another update")

	// Stop should be a no-op and not panic
	spinner.Stop()
	spinner.Stop() // Multiple stops should be safe

	// Combined operations
	spinner.Update("before stop")
	spinner.Stop()
	spinner.Update("after stop") // Should still work
}
