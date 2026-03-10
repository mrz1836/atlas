package dashboard

import (
	"strings"
)

// StatusBar is the single-row footer component of the ATLAS dashboard.
// It renders context-sensitive keybinding hints based on the currently selected task.
// StatusBar is a pure renderer — call SetTask before each View call.
type StatusBar struct {
	keys         KeyMap
	selectedTask *TaskInfo
}

// NewStatusBar creates a StatusBar wired to the given KeyMap.
func NewStatusBar(km KeyMap) StatusBar {
	return StatusBar{keys: km}
}

// SetTask updates the selected task context.
// Pass nil when no task is selected.
func (sb *StatusBar) SetTask(task *TaskInfo) { sb.selectedTask = task }

// SelectedTask returns the current task context (may be nil).
func (sb *StatusBar) SelectedTask() *TaskInfo { return sb.selectedTask }

// View renders the status bar into a single line of exactly width columns.
// The hints shown depend on the selected task's status:
//   - "a approve" — only when status=awaiting_approval
//   - "p pause"   — only when status=running or queued
//   - "R resume"  — only when status=paused or failed
//   - "x abandon" — only when status=running, queued, or paused
//   - "? help  q quit" — always shown
func (sb *StatusBar) View(width int) string {
	s := GetStyles()

	hints := sb.buildHints()
	line := renderHints(hints, s)

	// Pad or truncate to width.
	plain := stripANSI(line)
	plainRunes := []rune(plain)
	if len(plainRunes) < width {
		line += strings.Repeat(" ", width-len(plainRunes))
	} else if len(plainRunes) > width {
		// Keep as much as fits (ANSI-unaware truncation for safety).
		line = string(plainRunes[:width])
	}

	return s.StatusBar.Render(line)
}

// hint is a key + action label pair for display in the status bar.
type hint struct {
	key    string
	action string
}

// buildHints returns the ordered list of key hints based on current task context.
func (sb *StatusBar) buildHints() []hint {
	hints := make([]hint, 0, 7)

	if sb.selectedTask != nil {
		hints = append(hints, hintsForStatus(sb.selectedTask.Status)...)
	}

	// Always-present hints.
	hints = append(hints, hint{"?", "help"})
	hints = append(hints, hint{"q", "quit"})

	return hints
}

// hintsForStatus returns the context-sensitive hints for a given task status.
// Kept separate from buildHints to flatten nesting.
func hintsForStatus(status TaskStatus) []hint {
	hints := make([]hint, 0, 5)

	if status == TaskStatusAwaitingApproval {
		hints = append(hints, hint{"a", "approve"})
		hints = append(hints, hint{"r", "reject"})
	}

	if status == TaskStatusRunning || status == TaskStatusQueued {
		hints = append(hints, hint{"p", "pause"})
	}

	if status == TaskStatusPaused || status == TaskStatusFailed {
		hints = append(hints, hint{"R", "resume"})
	}

	if status == TaskStatusRunning || status == TaskStatusQueued || status == TaskStatusPaused {
		hints = append(hints, hint{"x", "abandon"})
	}

	if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusAbandoned {
		hints = append(hints, hint{"d", "destroy ws"})
	}

	return hints
}

// renderHints formats a slice of hints as "key action  key action  …" with styled keys.
func renderHints(hints []hint, s *Styles) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		key := s.KeyHint.Render(h.key)
		action := s.KeyAction.Render(h.action)
		parts = append(parts, key+" "+action)
	}
	return strings.Join(parts, "  ")
}
