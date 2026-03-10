package dashboard

import (
	"time"

	"github.com/mrz1836/atlas/internal/daemon"
)

// TaskEventMsg carries a real-time task event from the daemon event subscriber.
// Received from the Redis pub/sub channel.
type TaskEventMsg struct {
	Event daemon.TaskEvent
}

// LogEntryMsg carries a single log entry for the currently selected task.
// Received from the Redis log stream tail.
type LogEntryMsg struct {
	Entry daemon.LogEntry
}

// DaemonStatusMsg carries the current daemon status from a daemon.status RPC call.
// Used to populate the header status indicator and task list on startup/reconnect.
type DaemonStatusMsg struct {
	Status daemon.DaemonStatusResponse
}

// TickMsg signals that the clock should update (sent every second).
type TickMsg struct {
	Time time.Time
}

// ResizeMsg signals that the terminal has been resized.
// Sent when a tea.WindowSizeMsg is received by the top-level model.
type ResizeMsg struct {
	Width  int
	Height int
}

// ViewChangeMsg signals that the dashboard should switch display modes.
type ViewChangeMsg struct {
	Mode ViewMode
}

// TaskSelectedMsg signals that the user has selected a specific task.
// Triggers loading of full task detail and switching log stream.
type TaskSelectedMsg struct {
	TaskID string
}

// ActionConfirmedMsg signals that the user confirmed an action in a dialog.
// The parent model uses this to issue the appropriate daemon RPC call.
type ActionConfirmedMsg struct {
	// Action is the action name (approve, reject, pause, resume, abandon, destroy).
	Action string
	// TaskID is the task the action applies to.
	TaskID string
}

// ActionCanceledMsg signals that the user canceled an in-progress action dialog.
type ActionCanceledMsg struct{}

// ErrorMsg carries an error to be displayed in the status bar.
type ErrorMsg struct {
	Err error
}

// ReconnectedMsg signals that the connection to the daemon has been restored.
// The model should refresh the full task list via RPC.
type ReconnectedMsg struct{}

// DisconnectedMsg signals that the connection to the daemon has been lost.
// The Err field provides context for the disconnection cause.
type DisconnectedMsg struct {
	Err error
}
