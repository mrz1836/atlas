package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mrz1836/atlas/internal/tui"
)

// TaskList is the left-panel task list component.
// It displays all known tasks with status icons, step progress, and timing.
// Supports j/k and ↑/↓ keyboard navigation with visible selection highlight and
// auto-scrolling to keep the selected row visible.
type TaskList struct {
	items  []TaskInfo
	cursor int // index of the selected task (absolute, into items)
	offset int // index of the first visible item (for scrolling)
}

// NewTaskList creates an empty TaskList ready to receive items.
func NewTaskList() TaskList {
	return TaskList{}
}

// SetItems replaces the current item list, clamping the cursor if needed.
// The offset is reset if the cursor would fall out of the visible window.
func (tl *TaskList) SetItems(items []TaskInfo) {
	tl.items = items
	if tl.cursor >= len(items) {
		if len(items) == 0 {
			tl.cursor = 0
		} else {
			tl.cursor = len(items) - 1
		}
	}
}

// Items returns a copy of the current item slice (useful for tests).
func (tl *TaskList) Items() []TaskInfo {
	out := make([]TaskInfo, len(tl.items))
	copy(out, tl.items)
	return out
}

// Cursor returns the current cursor position (0-based index into items).
func (tl *TaskList) Cursor() int { return tl.cursor }

// Offset returns the current scroll offset (index of the first visible row).
func (tl *TaskList) Offset() int { return tl.offset }

// SelectedID returns the ID of the currently selected task, or "" if the list is empty.
func (tl *TaskList) SelectedID() string {
	if len(tl.items) == 0 {
		return ""
	}
	return tl.items[tl.cursor].ID
}

// Selected returns a pointer to the currently selected TaskInfo, or nil if empty.
// The returned pointer is valid until the next call to SetItems or Update.
func (tl *TaskList) Selected() *TaskInfo {
	if len(tl.items) == 0 {
		return nil
	}
	return &tl.items[tl.cursor]
}

// Update handles Bubble Tea messages. Navigation keys (up/k, down/j) move the cursor
// and update the scroll offset. Other messages fall through unchanged.
// Update uses a pointer receiver for consistency with the rest of the type's methods.
func (tl *TaskList) Update(msg tea.Msg) (TaskList, tea.Cmd) {
	if len(tl.items) == 0 {
		return *tl, nil
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *tl, nil
	}

	var changed bool

	switch keyMsg.String() {
	case "up", "k":
		if tl.cursor > 0 {
			tl.cursor--
			changed = true
		}
	case "down", "j":
		if tl.cursor < len(tl.items)-1 {
			tl.cursor++
			changed = true
		}
	}

	if changed {
		selectedID := tl.items[tl.cursor].ID
		return *tl, func() tea.Msg {
			return TaskSelectedMsg{TaskID: selectedID}
		}
	}

	return *tl, nil
}

// View renders the task list into a string of exactly height rows and width columns.
// Each row shows: status icon + task description + step progress + elapsed time.
// The selected row is highlighted using the dashboard selection style.
// The list auto-scrolls to keep the cursor visible.
func (tl *TaskList) View(width, height int) string {
	if len(tl.items) == 0 {
		return taskListEmptyState(width, height)
	}

	// Auto-scroll: adjust offset so cursor stays visible.
	offset := autoScrollOffset(tl.cursor, tl.offset, height)

	s := GetStyles()
	rows := make([]string, 0, height)

	end := offset + height
	if end > len(tl.items) {
		end = len(tl.items)
	}

	for i := offset; i < end; i++ {
		row := renderTaskRow(tl.items[i], width, i == tl.cursor, s)
		rows = append(rows, row)
	}

	// Pad remaining rows with blank space.
	blank := strings.Repeat(" ", width)
	for len(rows) < height {
		rows = append(rows, blank)
	}

	return strings.Join(rows, "\n")
}

// autoScrollOffset computes the visible window offset so cursor stays in view.
// It adjusts offset only if the cursor is outside the current window.
func autoScrollOffset(cursor, offset, height int) int {
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+height {
		return cursor - height + 1
	}
	return offset
}

// renderTaskRow produces a single row string for a TaskInfo.
// Layout (rune-accurate): [icon] [description…] [step progress] [elapsed]
// The selected row is rendered with reverse highlighting; others get status color on the icon.
func renderTaskRow(task TaskInfo, width int, selected bool, s *Styles) string {
	icon := taskListIcon(task.Status)
	elapsed := taskListElapsed(task)
	stepProgress := taskListStepProgress(task)

	// Build the right-hand annotation (step progress + elapsed).
	rightPart := buildRightPart(stepProgress, elapsed)

	iconRunes := []rune(icon)
	rightRunes := []rune(rightPart)
	rightWidth := len(rightRunes)
	if rightWidth > 0 {
		rightWidth++ // leading space separator
	}

	// Description occupies the remaining columns.
	// icon(len) + space(1) + desc(descWidth) + optional(space+rightPart)
	descWidth := width - len(iconRunes) - 1 - rightWidth
	if descWidth < 1 {
		descWidth = 1
	}

	desc := truncateRunes(task.Description, descWidth)

	// Build the row as a rune slice for accurate width control.
	parts := make([]rune, 0, width)
	parts = append(parts, iconRunes...)
	parts = append(parts, ' ')
	parts = append(parts, []rune(padRight(desc, descWidth))...)
	if len(rightRunes) > 0 {
		parts = append(parts, ' ')
		parts = append(parts, rightRunes...)
	}
	// Ensure exactly width runes.
	for len(parts) < width {
		parts = append(parts, ' ')
	}
	if len(parts) > width {
		parts = parts[:width]
	}

	row := string(parts)

	if selected {
		return s.Selected.Render(row)
	}

	// For non-selected rows, color just the icon rune(s).
	iconStyled := lipgloss.NewStyle().Foreground(StatusColor(task.Status)).Render(icon)
	rest := string(parts[len(iconRunes):])
	return iconStyled + rest
}

// buildRightPart concatenates stepProgress and elapsed into the right-column annotation.
func buildRightPart(stepProgress, elapsed string) string {
	if stepProgress == "" {
		return elapsed
	}
	if elapsed == "" {
		return stepProgress
	}
	return stepProgress + " " + elapsed
}

// taskListIcon returns the display icon for a task status, using the
// icon set optimized for list legibility.
func taskListIcon(status TaskStatus) string {
	switch status {
	case TaskStatusRunning:
		return "●"
	case TaskStatusQueued:
		return "○"
	case TaskStatusAwaitingApproval:
		return "◉"
	case TaskStatusCompleted:
		return "✓"
	case TaskStatusFailed:
		return "✗"
	case TaskStatusPaused:
		return "⏸"
	case TaskStatusAbandoned:
		return "⊘"
	default:
		return "?"
	}
}

// taskListStepProgress returns a compact "N/T" step indicator, or "" if unavailable.
func taskListStepProgress(task TaskInfo) string {
	if task.StepTotal > 0 {
		return fmt.Sprintf("%d/%d", task.StepIndex, task.StepTotal)
	}
	if task.CurrentStep != "" {
		return task.CurrentStep
	}
	return ""
}

// taskListElapsed returns a human-readable elapsed time for the task,
// appropriate to its status.
func taskListElapsed(task TaskInfo) string {
	now := time.Now()
	switch task.Status {
	case TaskStatusRunning, TaskStatusAwaitingApproval:
		if !task.StartedAt.IsZero() {
			return tui.FormatDuration(now.Sub(task.StartedAt).Milliseconds())
		}
		return ""
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusAbandoned:
		if !task.StartedAt.IsZero() && !task.CompletedAt.IsZero() {
			return tui.FormatDuration(task.CompletedAt.Sub(task.StartedAt).Milliseconds())
		}
		return ""
	case TaskStatusQueued:
		if !task.SubmittedAt.IsZero() {
			return tui.FormatDuration(now.Sub(task.SubmittedAt).Milliseconds())
		}
		return ""
	case TaskStatusPaused:
		if !task.UpdatedAt.IsZero() {
			return tui.FormatDuration(now.Sub(task.UpdatedAt).Milliseconds())
		}
		return ""
	default:
		return ""
	}
}

// taskListEmptyState renders a placeholder when there are no tasks.
func taskListEmptyState(width, height int) string {
	msg := "No tasks"
	line := padOrTruncateLine(msg, width)
	blank := strings.Repeat(" ", width)
	rows := make([]string, height)
	rows[0] = line
	for i := 1; i < height; i++ {
		rows[i] = blank
	}
	return strings.Join(rows, "\n")
}

// truncateRunes truncates a string to at most maxRunes runes (not bytes).
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "…"
}

// padRight pads s with spaces on the right to reach width runes.
// If s is already at or above width, it is returned unchanged.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
