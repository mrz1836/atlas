package dashboard

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ConfirmAction identifies which destructive action to perform on confirmation.
type ConfirmAction int

const (
	// ConfirmActionAbandon is used when the user wants to abandon a running/queued/paused task.
	ConfirmActionAbandon ConfirmAction = iota
	// ConfirmActionDestroy is used when the user wants to destroy the workspace for a task.
	ConfirmActionDestroy
)

// String returns the lowercase display name for the action (e.g. "abandon", "destroy").
func (a ConfirmAction) String() string {
	switch a {
	case ConfirmActionAbandon:
		return "abandon"
	case ConfirmActionDestroy:
		return "destroy"
	default:
		return "confirm"
	}
}

// ConfirmDialog is a modal overlay that asks "Are you sure you want to [subject]?"
// It is used for destructive actions: abandon task and destroy workspace.
//
// A newly created dialog is hidden; call Show to make it visible.
// The dialog does not self-hide on confirmation or cancellation — that responsibility
// belongs to the parent overlayState (which listens for ActionConfirmedMsg / ActionCanceledMsg).
//
// Usage:
//
//	d := dashboard.NewConfirmDialog(dashboard.ConfirmActionAbandon, taskID, subject)
//	d.Show(dashboard.ConfirmActionAbandon, taskID, subject)
//	// in Update: d, cmd = d.Update(keyMsg)  (cmd fires ActionConfirmedMsg or ActionCanceledMsg)
//	// in View:   rendered := d.View(width, height)
type ConfirmDialog struct {
	action  ConfirmAction
	taskID  string
	subject string
	visible bool
}

// NewConfirmDialog creates a new confirmation dialog for the given action and task.
// The dialog starts hidden; call Show to make it visible.
//
// subject is the human-readable identifier shown in the prompt (task description or
// workspace name, truncated for display).
func NewConfirmDialog(action ConfirmAction, taskID, subject string) ConfirmDialog {
	return ConfirmDialog{action: action, taskID: taskID, subject: subject}
}

// Show makes the dialog visible with the given action, taskID, and subject.
// Calling Show on an already-visible dialog replaces its content.
func (d *ConfirmDialog) Show(action ConfirmAction, taskID, subject string) {
	d.action = action
	d.taskID = taskID
	d.subject = subject
	d.visible = true
}

// Hide makes the dialog invisible (no-op if already hidden).
func (d *ConfirmDialog) Hide() { d.visible = false }

// IsVisible reports whether the dialog is currently shown.
func (d *ConfirmDialog) IsVisible() bool { return d.visible }

// TaskID returns the task ID associated with this dialog.
func (d *ConfirmDialog) TaskID() string { return d.taskID }

// Action returns the string name of the action (e.g. "abandon", "destroy").
func (d *ConfirmDialog) Action() string { return d.action.String() }

// Update handles key events for the confirmation dialog.
// Returns (updated dialog, cmd):
//   - When hidden: returns (d, nil) — no-op.
//   - y/Y/enter: cmd fires ActionConfirmedMsg with the action and taskID.
//   - n/N/esc: cmd fires ActionCanceledMsg.
//   - Other keys: (d, nil) — dialog remains visible and unchanged.
//
// The dialog does NOT self-hide; the parent overlayState calls dismiss() when
// it receives ActionConfirmedMsg or ActionCanceledMsg.
func (d *ConfirmDialog) Update(msg tea.Msg) (ConfirmDialog, tea.Cmd) {
	if !d.visible {
		return *d, nil
	}

	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *d, nil
	}

	action := d.action.String()
	taskID := d.taskID

	switch kp.String() {
	case "y", "Y", "enter":
		return *d, func() tea.Msg {
			return ActionConfirmedMsg{Action: action, TaskID: taskID}
		}
	case "n", "N", "esc":
		return *d, func() tea.Msg { return ActionCanceledMsg{} }
	}
	return *d, nil
}

// View renders the confirmation dialog centered within termWidth × termHeight.
// Returns an empty string when the dialog is hidden.
func (d *ConfirmDialog) View(termWidth, termHeight int) string {
	if !d.visible {
		return ""
	}

	s := GetStyles()
	prompt := d.buildPrompt()

	boxW := len([]rune(prompt)) + 8
	if boxW > termWidth-4 {
		boxW = termWidth - 4
	}
	if boxW < 34 {
		boxW = 34
	}

	hint := s.Dimmed.Render("  [y] confirm  [n/Esc] cancel  ")
	inner := fmt.Sprintf("\n  %s\n\n%s\n", prompt, hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF5F00")).
		Width(boxW).
		Padding(0, 1).
		Render(inner)

	return centerOverlay(box, termWidth, termHeight)
}

// buildPrompt returns the question string shown in the dialog.
func (d *ConfirmDialog) buildPrompt() string {
	subj := d.subject
	if len([]rune(subj)) > 40 {
		subj = string([]rune(subj)[:37]) + "..."
	}
	switch d.action {
	case ConfirmActionAbandon:
		return fmt.Sprintf("Abandon task: %s?", subj)
	case ConfirmActionDestroy:
		return fmt.Sprintf("Destroy workspace: %s?", subj)
	default:
		return fmt.Sprintf("Are you sure? (%s: %s)", d.action, subj)
	}
}

// centerOverlay pads content with newlines and spaces to center it within
// a terminal of termWidth × termHeight.
func centerOverlay(content string, termWidth, termHeight int) string {
	lines := strings.Split(content, "\n")
	contentH := len(lines)
	topPad := (termHeight - contentH) / 2
	if topPad < 0 {
		topPad = 0
	}

	// Find the widest visible line.
	maxW := 0
	for _, l := range lines {
		w := len([]rune(stripANSI(l)))
		if w > maxW {
			maxW = w
		}
	}
	leftPad := (termWidth - maxW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	pad := strings.Repeat(" ", leftPad)

	var sb strings.Builder
	sb.WriteString(strings.Repeat("\n", topPad))
	for _, l := range lines {
		sb.WriteString(pad + l + "\n")
	}
	return sb.String()
}
