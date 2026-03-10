package dashboard

import (
	tea "charm.land/bubbletea/v2"
)

// Model is the top-level Bubble Tea model for the ATLAS dashboard.
// It composes the layout engine, task list, detail panel, log panel,
// and interactive controls into a single full-screen TUI.
//
// Lifecycle:
//   - Init() connects to the daemon, starts the event subscriber, and loads the
//     initial task list.
//   - Update() routes incoming messages to the appropriate sub-components.
//   - View() renders the full-screen layout via the Layout engine.
type Model struct {
	// layout controls split-pane geometry.
	layout Layout
	// keys holds all keyboard bindings.
	keys KeyMap
	// tasks is the ordered list of tasks currently visible in the task list.
	tasks []TaskInfo
	// connState is the current daemon/Redis connection state.
	connState ConnectionState
	// width and height track the current terminal dimensions.
	width, height int
	// quitting is set to true when the user requests exit.
	quitting bool
}

// New creates a new dashboard Model with sensible defaults.
// Call tea.NewProgram(New(...)).Run() to launch the dashboard.
func New() *Model {
	return &Model{
		layout:    NewLayout(80, 24), // overridden by tea.WindowSizeMsg on launch
		keys:      DefaultKeyMap(),
		connState: ConnectionStateReconnecting,
	}
}

// Init is called once when the Bubble Tea program starts.
// It returns the initial set of commands: connect to daemon, start event subscriber,
// fetch the initial task list, and start the clock tick.
func (m *Model) Init() tea.Cmd {
	// Phase 3 wires the real connection commands.
	// For now, return nil so the package compiles.
	return nil
}

// Update handles incoming messages and updates the model accordingly.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout.Width = msg.Width
		m.layout.Height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case msg.String() == "q" || msg.String() == "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case msg.String() == "?":
			m.layout.Mode = ViewModeHelp
			return m, nil
		case msg.String() == "esc":
			m.layout.Mode = ViewModeList
			return m, nil
		}

	case TickMsg:
		// Clock update — re-render header.
		return m, nil

	case TaskEventMsg:
		m.applyTaskEvent(msg.Event)
		return m, nil

	case ResizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout.Width = msg.Width
		m.layout.Height = msg.Height
		return m, nil

	case ViewChangeMsg:
		m.layout.Mode = msg.Mode
		return m, nil

	case ReconnectedMsg:
		m.connState = ConnectionStateConnected
		return m, nil

	case DisconnectedMsg:
		m.connState = ConnectionStateDisconnected
		return m, nil

	case ErrorMsg:
		// Error display handled in View (status bar).
		return m, nil
	}

	return m, nil
}

// View renders the full-screen dashboard layout.
func (m *Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// Phase 3 fills in real panel rendering.
	// Placeholder renders a minimal skeleton so the package compiles and tests pass.
	left := "Tasks\n"
	right := "Detail\n"

	content := m.layout.Render(left, right)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Tasks returns a copy of the current task list (useful for tests).
func (m *Model) Tasks() []TaskInfo {
	out := make([]TaskInfo, len(m.tasks))
	copy(out, m.tasks)
	return out
}

// ConnState returns the current connection state.
func (m *Model) ConnState() ConnectionState {
	return m.connState
}

// applyTaskEvent updates the internal task list based on a real-time TaskEvent.
// Unexported helper; must appear after all exported methods (funcorder).
func (m *Model) applyTaskEvent(_ interface{}) {
	// Phase 3 implements full event handling.
}
