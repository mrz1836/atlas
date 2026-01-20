// Package ai provides AI execution capabilities for ATLAS.
package ai

import (
	"time"
)

// VerbosityLevel represents the activity feed verbosity setting.
type VerbosityLevel string

const (
	// VerbosityLow shows phase changes only.
	VerbosityLow VerbosityLevel = "low"

	// VerbosityMedium shows phases + file operations + key decisions.
	VerbosityMedium VerbosityLevel = "medium"

	// VerbosityHigh shows everything including thinking indicators.
	VerbosityHigh VerbosityLevel = "high"
)

// ParseVerbosity converts a string to a VerbosityLevel.
// Returns VerbosityMedium for invalid values.
func ParseVerbosity(s string) VerbosityLevel {
	switch s {
	case "low":
		return VerbosityLow
	case "medium":
		return VerbosityMedium
	case "high":
		return VerbosityHigh
	default:
		return VerbosityMedium
	}
}

// ShouldShow returns true if the activity type should be shown at the given verbosity level.
func (v VerbosityLevel) ShouldShow(actType ActivityType) bool {
	switch v {
	case VerbosityLow:
		// Only show phase changes
		return actType == ActivityPlanning || actType == ActivityImplementing ||
			actType == ActivityVerifying || actType == ActivityAnalyzing
	case VerbosityMedium:
		// Show phases + file operations + key decisions
		return actType != ActivityThinking
	case VerbosityHigh:
		// Show everything
		return true
	default:
		return true
	}
}

// ActivityType represents the type of AI activity.
type ActivityType string

const (
	// ActivityReading indicates the AI is reading a file.
	ActivityReading ActivityType = "reading"

	// ActivityWriting indicates the AI is writing to a file.
	ActivityWriting ActivityType = "writing"

	// ActivityThinking indicates the AI is thinking/processing.
	ActivityThinking ActivityType = "thinking"

	// ActivityPlanning indicates the AI is planning its approach.
	ActivityPlanning ActivityType = "planning"

	// ActivityImplementing indicates the AI is implementing changes.
	ActivityImplementing ActivityType = "implementing"

	// ActivityVerifying indicates the AI is verifying changes.
	ActivityVerifying ActivityType = "verifying"

	// ActivityAnalyzing indicates the AI is analyzing code.
	ActivityAnalyzing ActivityType = "analyzing"

	// ActivitySearching indicates the AI is searching for files or patterns.
	ActivitySearching ActivityType = "searching"

	// ActivityExecuting indicates the AI is executing a command.
	ActivityExecuting ActivityType = "executing"
)

// Icon returns the display icon for this activity type.
func (t ActivityType) Icon() string {
	switch t {
	case ActivityReading:
		return "📖"
	case ActivityWriting:
		return "✏️"
	case ActivityThinking:
		return "⋮"
	case ActivityPlanning:
		return "📋"
	case ActivityImplementing:
		return "🔧"
	case ActivityVerifying:
		return "✓"
	case ActivityAnalyzing:
		return "🔍"
	case ActivitySearching:
		return "🔎"
	case ActivityExecuting:
		return "▶"
	default:
		return "●"
	}
}

// ActivityEvent represents a single activity from the AI.
type ActivityEvent struct {
	// Timestamp is when this activity occurred.
	Timestamp time.Time

	// Type is the category of activity.
	Type ActivityType

	// Message is the human-readable description of the activity.
	Message string

	// File is the optional file path being operated on.
	File string

	// Phase is the optional current phase name.
	Phase string
}

// FormatMessage returns a formatted message for display.
// If a file is specified, it's included in the message.
func (e ActivityEvent) FormatMessage() string {
	if e.File != "" {
		return e.Message + " " + e.File
	}
	return e.Message
}

// ActivityCallback is a function that receives activity events.
type ActivityCallback func(event ActivityEvent)

// ActivityOptions configures activity streaming behavior.
type ActivityOptions struct {
	// Callback receives activity events during execution.
	Callback ActivityCallback

	// Verbosity controls which events are emitted.
	Verbosity VerbosityLevel

	// TaskID is the task ID for logging purposes.
	TaskID string

	// WorkspaceName is the workspace name for logging purposes.
	WorkspaceName string
}
