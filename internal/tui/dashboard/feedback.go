package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FeedbackInput is a modal text-input overlay for collecting rejection feedback.
// Press Enter to submit, Esc to cancel.
//
// Usage:
//
//	f := NewFeedbackInput(taskID)
//	_, cmd := f.Update(msg)
//	rendered := f.View(termWidth, termHeight)
type FeedbackInput struct {
	taskID string
	input  textinput.Model
}

// NewFeedbackInput creates a new feedback input overlay for the given task.
func NewFeedbackInput(taskID string) FeedbackInput {
	ti := textinput.New()
	ti.Placeholder = "Reason for rejection…"
	ti.Focus()
	ti.SetWidth(50)
	ti.CharLimit = 500

	return FeedbackInput{taskID: taskID, input: ti}
}

// Update handles key events and text input for the feedback overlay.
// Returns a FeedbackSubmittedMsg on Enter or ActionCanceledMsg on Esc.
func (f FeedbackInput) Update(msg tea.Msg) (FeedbackInput, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return f, func() tea.Msg {
				return FeedbackSubmittedMsg{TaskID: f.taskID, Feedback: f.input.Value()}
			}
		case "esc":
			return f, func() tea.Msg { return ActionCanceledMsg{} }
		}
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return f, cmd
}

// View renders the feedback input overlay centered within termWidth × termHeight.
func (f FeedbackInput) View(termWidth, termHeight int) string {
	s := GetStyles()

	boxW := 60
	if boxW > termWidth-4 {
		boxW = termWidth - 4
	}
	if boxW < 40 {
		boxW = 40
	}

	title := s.KeyHint.Render("Reject task — enter feedback:")
	hint := s.Dimmed.Render("[enter] submit  [esc] cancel")

	inner := fmt.Sprintf("\n  %s\n\n  %s\n\n  %s\n",
		title,
		f.input.View(),
		hint,
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF5F00")).
		Width(boxW).
		Padding(0, 1).
		Render(inner)

	return centerOverlay(box, termWidth, termHeight)
}

// Value returns the current text in the feedback input (for testing).
func (f FeedbackInput) Value() string { return f.input.Value() }

// FeedbackSubmittedMsg is fired when the user submits feedback via the overlay.
type FeedbackSubmittedMsg struct {
	// TaskID is the task being rejected.
	TaskID string
	// Feedback is the user-supplied rejection reason.
	Feedback string
}

// OverlayKind identifies which overlay is currently active.
type OverlayKind int

const (
	// OverlayNone means no overlay is shown.
	OverlayNone OverlayKind = iota
	// OverlayConfirm means the confirmation dialog is active.
	OverlayConfirm
	// OverlayFeedback means the feedback input is active.
	OverlayFeedback
)

// overlayState bundles the mutually-exclusive overlay state carried by the model.
// Exactly one overlay is active at a time; kind==OverlayNone when idle.
type overlayState struct {
	kind     OverlayKind
	confirm  ConfirmDialog
	feedback FeedbackInput
}

// isActive returns true when any overlay is shown.
func (o *overlayState) isActive() bool { return o.kind != OverlayNone }

// showConfirm activates the confirmation dialog for the given action.
func (o *overlayState) showConfirm(action ConfirmAction, taskID, subject string) {
	o.kind = OverlayConfirm
	o.confirm = NewConfirmDialog(action, taskID, subject)
	o.confirm.Show(action, taskID, subject)
}

// showFeedback activates the feedback input overlay for the given task.
func (o *overlayState) showFeedback(taskID string) {
	o.kind = OverlayFeedback
	o.feedback = NewFeedbackInput(taskID)
}

// dismiss clears any active overlay.
func (o *overlayState) dismiss() { o.kind = OverlayNone }

// update routes a message to the active overlay and returns a command.
// For OverlayConfirm, the returned cmd fires ActionConfirmedMsg or ActionCanceledMsg;
// the model.Update handler calls dismiss() on receipt of those messages.
// Returns nil cmd when no overlay is active.
func (o *overlayState) update(msg tea.Msg) tea.Cmd {
	switch o.kind {
	case OverlayConfirm:
		var cmd tea.Cmd
		o.confirm, cmd = o.confirm.Update(msg)
		return cmd

	case OverlayFeedback:
		var cmd tea.Cmd
		o.feedback, cmd = o.feedback.Update(msg)
		return cmd

	case OverlayNone:
		// No overlay active — nothing to do.
	}
	return nil
}

// view renders the active overlay centered in termWidth × termHeight.
// Returns an empty string when no overlay is active.
func (o *overlayState) view(termWidth, termHeight int) string {
	switch o.kind {
	case OverlayConfirm:
		return o.confirm.View(termWidth, termHeight)
	case OverlayFeedback:
		return o.feedback.View(termWidth, termHeight)
	case OverlayNone:
		// Nothing to render.
	}
	return ""
}

// overlayHintText returns a short status-bar hint shown while an overlay is active.
func overlayHintText(kind OverlayKind) string {
	switch kind {
	case OverlayConfirm:
		return "Confirm action — y/enter to confirm · n/esc to cancel"
	case OverlayFeedback:
		return "Enter rejection feedback — enter to submit · esc to cancel"
	case OverlayNone:
		// No active overlay.
	}
	return ""
}

// renderWithOverlay composites overlay on top of base using newline separation.
// When overlay is empty, base is returned unchanged.
func renderWithOverlay(base, overlay string) string {
	if overlay == "" {
		return base
	}
	// Overlay the dialog by printing it at the end — the TUI alt-screen will
	// position it via the terminal cursor. For a simple approach, just replace
	// the lower portion of the base with the overlay content.
	_ = base
	return overlay
}

// notificationStyle holds visual feedback for completed actions.
type notificationStyle struct {
	text    string
	isError bool
}

// Render returns the notification as a styled string.
func (n notificationStyle) Render() string {
	if n.text == "" {
		return ""
	}
	if n.isError {
		s := GetStyles()
		return s.LogError.Render("✗ " + n.text)
	}
	s := GetStyles()
	return s.DaemonConnected.Render("✓ " + n.text)
}

// actionNotifications maps action names to their success messages.
//
//nolint:gochecknoglobals // Package-level display constants.
var actionNotifications = map[string]string{
	"approve": "Task approved",
	"reject":  "Rejected with feedback",
	"pause":   "Task paused",
	"resume":  "Task resumed",
	"abandon": "Task abandoned",
	"destroy": "Workspace destroyed",
}

// notificationForAction returns the success notification text for an action.
func notificationForAction(action string) string {
	if msg, ok := actionNotifications[action]; ok {
		return msg
	}
	return strings.ToUpper(action[:1]) + action[1:] + " completed"
}
