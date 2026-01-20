package ai

import (
	"testing"
	"time"
)

func TestParseVerbosity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected VerbosityLevel
	}{
		{
			name:     "low",
			input:    "low",
			expected: VerbosityLow,
		},
		{
			name:     "medium",
			input:    "medium",
			expected: VerbosityMedium,
		},
		{
			name:     "high",
			input:    "high",
			expected: VerbosityHigh,
		},
		{
			name:     "empty defaults to medium",
			input:    "",
			expected: VerbosityMedium,
		},
		{
			name:     "invalid defaults to medium",
			input:    "invalid",
			expected: VerbosityMedium,
		},
		{
			name:     "uppercase defaults to medium",
			input:    "HIGH",
			expected: VerbosityMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ParseVerbosity(tt.input)
			if result != tt.expected {
				t.Errorf("ParseVerbosity(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestVerbosityLevel_ShouldShow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		verbosity VerbosityLevel
		actType   ActivityType
		expected  bool
	}{
		// Low verbosity - only phase changes
		{
			name:      "low shows planning",
			verbosity: VerbosityLow,
			actType:   ActivityPlanning,
			expected:  true,
		},
		{
			name:      "low shows implementing",
			verbosity: VerbosityLow,
			actType:   ActivityImplementing,
			expected:  true,
		},
		{
			name:      "low shows verifying",
			verbosity: VerbosityLow,
			actType:   ActivityVerifying,
			expected:  true,
		},
		{
			name:      "low shows analyzing",
			verbosity: VerbosityLow,
			actType:   ActivityAnalyzing,
			expected:  true,
		},
		{
			name:      "low hides reading",
			verbosity: VerbosityLow,
			actType:   ActivityReading,
			expected:  false,
		},
		{
			name:      "low hides writing",
			verbosity: VerbosityLow,
			actType:   ActivityWriting,
			expected:  false,
		},
		{
			name:      "low hides thinking",
			verbosity: VerbosityLow,
			actType:   ActivityThinking,
			expected:  false,
		},

		// Medium verbosity - phases + file operations
		{
			name:      "medium shows planning",
			verbosity: VerbosityMedium,
			actType:   ActivityPlanning,
			expected:  true,
		},
		{
			name:      "medium shows reading",
			verbosity: VerbosityMedium,
			actType:   ActivityReading,
			expected:  true,
		},
		{
			name:      "medium shows writing",
			verbosity: VerbosityMedium,
			actType:   ActivityWriting,
			expected:  true,
		},
		{
			name:      "medium hides thinking",
			verbosity: VerbosityMedium,
			actType:   ActivityThinking,
			expected:  false,
		},

		// High verbosity - everything
		{
			name:      "high shows planning",
			verbosity: VerbosityHigh,
			actType:   ActivityPlanning,
			expected:  true,
		},
		{
			name:      "high shows reading",
			verbosity: VerbosityHigh,
			actType:   ActivityReading,
			expected:  true,
		},
		{
			name:      "high shows thinking",
			verbosity: VerbosityHigh,
			actType:   ActivityThinking,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.verbosity.ShouldShow(tt.actType)
			if result != tt.expected {
				t.Errorf("%s.ShouldShow(%s) = %v, want %v",
					tt.verbosity, tt.actType, result, tt.expected)
			}
		})
	}
}

func TestActivityType_Icon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		actType  ActivityType
		expected string
	}{
		{ActivityReading, "📖"},
		{ActivityWriting, "✏️"},
		{ActivityThinking, "⋮"},
		{ActivityPlanning, "📋"},
		{ActivityImplementing, "🔧"},
		{ActivityVerifying, "✓"},
		{ActivityAnalyzing, "🔍"},
		{ActivitySearching, "🔎"},
		{ActivityExecuting, "▶"},
		{ActivityType("unknown"), "●"},
	}

	for _, tt := range tests {
		t.Run(string(tt.actType), func(t *testing.T) {
			t.Parallel()
			result := tt.actType.Icon()
			if result != tt.expected {
				t.Errorf("%s.Icon() = %q, want %q", tt.actType, result, tt.expected)
			}
		})
	}
}

func TestActivityEvent_FormatMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    ActivityEvent
		expected string
	}{
		{
			name: "message only",
			event: ActivityEvent{
				Message: "Analyzing code",
			},
			expected: "Analyzing code",
		},
		{
			name: "message with file",
			event: ActivityEvent{
				Message: "Reading",
				File:    "internal/ai/runner.go",
			},
			expected: "Reading internal/ai/runner.go",
		},
		{
			name: "empty message with file",
			event: ActivityEvent{
				Message: "",
				File:    "test.go",
			},
			expected: " test.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.event.FormatMessage()
			if result != tt.expected {
				t.Errorf("FormatMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestActivityCallback_Invocation(t *testing.T) {
	t.Parallel()

	var receivedEvents []ActivityEvent

	callback := func(event ActivityEvent) {
		receivedEvents = append(receivedEvents, event)
	}

	// Send some events
	events := []ActivityEvent{
		{Timestamp: time.Now(), Type: ActivityReading, Message: "Reading file"},
		{Timestamp: time.Now(), Type: ActivityWriting, Message: "Writing file"},
	}

	for _, e := range events {
		callback(e)
	}

	if len(receivedEvents) != len(events) {
		t.Errorf("Received %d events, want %d", len(receivedEvents), len(events))
	}

	for i, e := range events {
		if receivedEvents[i].Type != e.Type {
			t.Errorf("Event %d type = %s, want %s", i, receivedEvents[i].Type, e.Type)
		}
		if receivedEvents[i].Message != e.Message {
			t.Errorf("Event %d message = %q, want %q", i, receivedEvents[i].Message, e.Message)
		}
	}
}
