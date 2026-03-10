package dashboard

import "charm.land/bubbles/v2/key"

// KeyMap defines all keyboard bindings for the dashboard.
// Grouped by functional area for help rendering.
type KeyMap struct {
	// --- Navigation group ---

	// Up moves the cursor up in the task list.
	Up key.Binding
	// Down moves the cursor down in the task list.
	Down key.Binding
	// Enter selects the current item or confirms an action.
	Enter key.Binding
	// Log switches focus to the log panel (or enters full-screen log view).
	Log key.Binding
	// Tab cycles focus between task list and log panels.
	Tab key.Binding
	// Esc goes back, dismisses overlays, or exits full-screen modes.
	Esc key.Binding

	// --- Action group ---

	// Approve triggers the approve action for an awaiting_approval task.
	Approve key.Binding
	// Reject triggers the reject action for an awaiting_approval task.
	Reject key.Binding
	// Pause triggers the pause action for a running or queued task.
	Pause key.Binding
	// Resume triggers the resume action for a paused or failed task.
	Resume key.Binding
	// Abandon triggers the abandon action (with confirmation) for an active task.
	Abandon key.Binding
	// Destroy triggers the workspace destroy action (with confirmation).
	Destroy key.Binding

	// --- Log filter group ---

	// LogLevel1 shows all log levels (debug+).
	LogLevel1 key.Binding
	// LogLevel2 shows info and above.
	LogLevel2 key.Binding
	// LogLevel3 shows warn and above.
	LogLevel3 key.Binding
	// LogLevel4 shows error only.
	LogLevel4 key.Binding
	// LogSearch enters search mode in the log panel.
	LogSearch key.Binding
	// LogNextMatch jumps to the next search match.
	LogNextMatch key.Binding
	// LogPrevMatch jumps to the previous search match.
	LogPrevMatch key.Binding
	// LogBottom jumps to the bottom of the log (follows tail).
	LogBottom key.Binding
	// LogTop jumps to the top of the log.
	LogTop key.Binding

	// --- General group ---

	// Help toggles the help overlay.
	Help key.Binding
	// Quit exits the dashboard (does not stop the daemon).
	Quit key.Binding
}

// DefaultKeyMap returns the default key bindings for the dashboard.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// Navigation
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Log: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "log view"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),

		// Actions
		Approve: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "approve"),
		),
		Reject: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reject"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause"),
		),
		Resume: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "resume"),
		),
		Abandon: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "abandon"),
		),
		Destroy: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "destroy ws"),
		),

		// Log filters
		LogLevel1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "all levels"),
		),
		LogLevel2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "info+"),
		),
		LogLevel3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "warn+"),
		),
		LogLevel4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "error only"),
		),
		LogSearch: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		LogNextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		LogPrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		LogBottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "jump to bottom"),
		),
		LogTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "jump to top"),
		),

		// General
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// NavigationKeys returns the navigation group bindings for help rendering.
func (k KeyMap) NavigationKeys() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Log, k.Tab, k.Esc}
}

// ActionKeys returns the action group bindings for help rendering.
func (k KeyMap) ActionKeys() []key.Binding {
	return []key.Binding{k.Approve, k.Reject, k.Pause, k.Resume, k.Abandon, k.Destroy}
}

// LogKeys returns the log control bindings for help rendering.
func (k KeyMap) LogKeys() []key.Binding {
	return []key.Binding{
		k.LogLevel1, k.LogLevel2, k.LogLevel3, k.LogLevel4,
		k.LogSearch, k.LogNextMatch, k.LogPrevMatch, k.LogBottom, k.LogTop,
	}
}

// GeneralKeys returns the general group bindings for help rendering.
func (k KeyMap) GeneralKeys() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}
