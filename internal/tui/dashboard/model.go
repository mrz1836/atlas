package dashboard

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mrz1836/atlas/internal/daemon"
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

	// ── Sub-components ────────────────────────────────────────────────────────
	taskList   TaskList
	taskDetail TaskDetail
	header     Header
	statusBar  StatusBar

	// ── Task state ────────────────────────────────────────────────────────────
	// tasks is the canonical map of task ID → TaskInfo, updated by events and RPC.
	tasks map[string]TaskInfo
	// taskOrder preserves insertion order for stable list rendering.
	taskOrder []string
	// selectedTaskID is the ID of the currently focused task.
	selectedTaskID string

	// ── Daemon client ─────────────────────────────────────────────────────────
	// client is the JSON-RPC client for daemon calls.
	// May be nil if not yet connected (Phase 6 wires reconnect logic).
	client *daemon.Client

	// ── Display state ─────────────────────────────────────────────────────────
	// connState is the current daemon/Redis connection state.
	connState ConnectionState
	// width and height track the current terminal dimensions.
	width, height int
	// quitting is set to true when the user requests exit.
	quitting bool
	// showHelp toggles the help overlay.
	showHelp bool
}

// New creates a new dashboard Model with sensible defaults.
// Call tea.NewProgram(New()).Run() to launch the dashboard.
func New() *Model {
	km := DefaultKeyMap()
	return &Model{
		layout:     NewLayout(80, 24), // overridden by tea.WindowSizeMsg on launch
		keys:       km,
		taskList:   NewTaskList(),
		taskDetail: NewTaskDetail(),
		header:     NewHeader("ATLAS Dashboard"),
		statusBar:  NewStatusBar(km),
		tasks:      make(map[string]TaskInfo),
		connState:  ConnectionStateReconnecting,
	}
}

// NewWithClient creates a Model pre-wired to an existing daemon client.
// This is the production entry point; New() is for testing.
func NewWithClient(client *daemon.Client) *Model {
	m := New()
	m.client = client
	m.connState = ConnectionStateConnected
	return m
}

// Init is called once when the Bubble Tea program starts.
// It returns the initial set of commands: start the clock tick and set up a
// placeholder event subscription command (Phase 6 replaces this with real connection).
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.initialTaskListCmd(),
	)
}

// Update handles incoming messages and updates the model accordingly.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg.Width, msg.Height)

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case TickMsg:
		return m.handleTick(msg.Time)

	case TaskEventMsg:
		m.applyTaskEvent(msg.Event)
		return m, nil

	case TaskSelectedMsg:
		m.selectTask(msg.TaskID)
		return m, nil

	case ResizeMsg:
		return m.handleResize(msg.Width, msg.Height)

	case ViewChangeMsg:
		m.layout.Mode = msg.Mode
		return m, nil

	case ReconnectedMsg:
		m.connState = ConnectionStateConnected
		m.header.SetConnection(ConnectionStateConnected)
		return m, m.initialTaskListCmd()

	case DisconnectedMsg:
		m.connState = ConnectionStateDisconnected
		m.header.SetConnection(ConnectionStateDisconnected)
		return m, nil

	case ErrorMsg:
		// Error display handled in View (status bar) — Phase 5 wires this up.
		return m, nil

	case taskListLoadedMsg:
		m.processTaskListLoaded(msg.tasks)
		return m, nil
	}

	return m, nil
}

// View renders the full-screen dashboard layout.
func (m *Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// ── Header ────────────────────────────────────────────────────────────────
	headerStr := m.header.View(m.width)

	// ── Content panes ─────────────────────────────────────────────────────────
	contentH := m.layout.ContentHeight()
	leftW := m.layout.LeftWidth()
	rightW := m.layout.RightWidth()

	leftStr := m.taskList.View(leftW, contentH)
	rightStr := m.taskDetail.View(rightW, contentH)

	// ── Layout: combines left + divider + right ────────────────────────────────
	contentStr := m.layout.Render(leftStr, rightStr)

	// ── Footer (status bar) ────────────────────────────────────────────────────
	footerStr := m.statusBar.View(m.width)

	// ── Assemble full screen ───────────────────────────────────────────────────
	full := headerStr + "\n" + contentStr + "\n" + footerStr

	v := tea.NewView(full)
	v.AltScreen = true
	return v
}

// Tasks returns a copy of the current task list (useful for tests).
func (m *Model) Tasks() []TaskInfo {
	out := make([]TaskInfo, 0, len(m.taskOrder))
	for _, id := range m.taskOrder {
		if t, ok := m.tasks[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

// ConnState returns the current connection state.
func (m *Model) ConnState() ConnectionState { return m.connState }

// SelectedTaskID returns the currently selected task ID.
func (m *Model) SelectedTaskID() string { return m.selectedTaskID }

// SetClient replaces the daemon client (useful for testing).
func (m *Model) SetClient(c *daemon.Client) { m.client = c }

// ── Internal message handlers (unexported, after exported methods per funcorder) ──

// handleResize updates layout dimensions when the terminal is resized.
func (m *Model) handleResize(w, h int) (tea.Model, tea.Cmd) {
	m.width = w
	m.height = h
	m.layout.Width = w
	m.layout.Height = h
	return m, nil
}

// handleKey dispatches keyboard events to the appropriate handler.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit keys.
	if key == "q" || key == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	// Help overlay toggle.
	if key == "?" {
		m.showHelp = !m.showHelp
		m.layout.Mode = map[bool]ViewMode{true: ViewModeHelp, false: ViewModeList}[m.showHelp]
		return m, nil
	}
	if key == "esc" {
		m.showHelp = false
		m.layout.Mode = ViewModeList
		return m, nil
	}

	// Navigation — forward to task list.
	if key == "up" || key == "k" || key == "down" || key == "j" {
		updated, cmd := m.taskList.Update(msg)
		m.taskList = updated
		return m, cmd
	}

	return m, nil
}

// handleTick processes a clock tick: updates the header clock.
func (m *Model) handleTick(t time.Time) (tea.Model, tea.Cmd) {
	m.header.SetTime(t)
	return m, tickCmd()
}

// selectTask updates the selected task and propagates it to sub-components.
func (m *Model) selectTask(id string) {
	m.selectedTaskID = id
	if task, ok := m.tasks[id]; ok {
		m.taskDetail.SetTask(&task)
		m.statusBar.SetTask(&task)
	} else {
		m.taskDetail.SetTask(nil)
		m.statusBar.SetTask(nil)
	}
}

// applyTaskEvent updates the internal task map from a real-time TaskEvent.
func (m *Model) applyTaskEvent(event daemon.TaskEvent) {
	if event.TaskID == "" {
		return
	}

	existing := m.getOrCreateTask(event.TaskID)
	mergeEventFields(event, &existing)
	m.tasks[event.TaskID] = existing

	// Rebuild ordered task list for the task list component.
	m.rebuildTaskList()

	// If the updated task is currently selected, refresh detail panel.
	if m.selectedTaskID == event.TaskID {
		updated := m.tasks[event.TaskID]
		m.taskDetail.SetTask(&updated)
		m.statusBar.SetTask(&updated)
	}
}

// getOrCreateTask returns the existing TaskInfo for taskID, or a new empty one.
// If new, appends the ID to the task order slice.
func (m *Model) getOrCreateTask(taskID string) TaskInfo {
	existing, exists := m.tasks[taskID]
	if !exists {
		m.taskOrder = append(m.taskOrder, taskID)
		existing = TaskInfo{ID: taskID}
	}
	return existing
}

// mergeEventFields applies non-zero fields from event into task.
func mergeEventFields(event daemon.TaskEvent, task *TaskInfo) {
	mergeStringFields(event, task)
	mergeStepFields(event, task)
	mergeTimestamps(event, task)
}

// mergeStringFields copies non-empty string fields from event to task.
func mergeStringFields(event daemon.TaskEvent, task *TaskInfo) {
	if event.Status != "" {
		task.Status = TaskStatus(event.Status)
	}
	if event.Description != "" {
		task.Description = event.Description
	}
	if event.Agent != "" {
		task.Agent = event.Agent
	}
	if event.Model != "" {
		task.Model = event.Model
	}
	if event.Branch != "" {
		task.Branch = event.Branch
	}
	if event.Template != "" {
		task.Template = event.Template
	}
	if event.Priority != "" {
		task.Priority = event.Priority
	}
	if event.Workspace != "" {
		task.Workspace = event.Workspace
	}
	if event.PRURL != "" {
		task.PRURL = event.PRURL
	}
	if event.Error != "" {
		task.Error = event.Error
	}
}

// mergeStepFields copies step-related fields from event to task.
func mergeStepFields(event daemon.TaskEvent, task *TaskInfo) {
	if event.Step != "" {
		task.CurrentStep = event.Step
	}
	if event.StepIndex > 0 {
		task.StepIndex = event.StepIndex
	}
	if event.StepTotal > 0 {
		task.StepTotal = event.StepTotal
	}
}

// mergeTimestamps parses event.Time and updates phase-specific timestamps.
func mergeTimestamps(event daemon.TaskEvent, task *TaskInfo) {
	if event.Time == "" {
		return
	}
	t, err := time.Parse(time.RFC3339, event.Time)
	if err != nil {
		return
	}
	task.UpdatedAt = t

	switch event.Type {
	case daemon.EventTaskSubmitted:
		if task.SubmittedAt.IsZero() {
			task.SubmittedAt = t
		}
	case daemon.EventTaskStarted:
		if task.StartedAt.IsZero() {
			task.StartedAt = t
		}
	case daemon.EventTaskCompleted, daemon.EventTaskFailed, daemon.EventTaskAbandoned:
		if task.CompletedAt.IsZero() {
			task.CompletedAt = t
		}
	}
}

// rebuildTaskList reconstructs the TaskList items from the current task map.
func (m *Model) rebuildTaskList() {
	items := make([]TaskInfo, 0, len(m.taskOrder))
	for _, id := range m.taskOrder {
		if t, ok := m.tasks[id]; ok {
			items = append(items, t)
		}
	}
	m.taskList.SetItems(items)
}

// initialTaskListCmd returns a command that loads the initial task list from the daemon.
// If no client is available, it returns nil (dashboard starts empty).
func (m *Model) initialTaskListCmd() tea.Cmd {
	if m.client == nil {
		return nil
	}
	c := m.client
	return func() tea.Msg {
		var resp daemon.TaskListResponse
		if err := c.Call(context.Background(), daemon.MethodTaskList, daemon.TaskListRequest{Limit: 100}, &resp); err != nil {
			return ErrorMsg{Err: err}
		}
		return taskListLoadedMsg{tasks: resp.Tasks}
	}
}

// taskListLoadedMsg is an internal message fired after the initial task list RPC.
type taskListLoadedMsg struct {
	tasks []daemon.TaskStatusResponse
}

// processTaskListLoaded converts RPC responses into TaskInfo and populates the model.
func (m *Model) processTaskListLoaded(tasks []daemon.TaskStatusResponse) {
	for _, t := range tasks {
		info := taskStatusToInfo(t)
		if _, exists := m.tasks[info.ID]; !exists {
			m.taskOrder = append(m.taskOrder, info.ID)
		}
		m.tasks[info.ID] = info
	}
	m.rebuildTaskList()

	// Auto-select the first task if nothing is selected.
	if m.selectedTaskID == "" && len(m.taskOrder) > 0 {
		m.selectTask(m.taskOrder[0])
	}
}

// taskStatusToInfo converts a daemon.TaskStatusResponse to a dashboard TaskInfo.
func taskStatusToInfo(t daemon.TaskStatusResponse) TaskInfo {
	info := TaskInfo{
		ID:          t.TaskID,
		Status:      TaskStatus(t.Status),
		Priority:    t.Priority,
		Description: t.Description,
		Workspace:   t.Workspace,
		Agent:       t.Agent,
		Model:       t.Model,
		Branch:      t.Branch,
		Template:    t.Template,
		StepIndex:   t.CurrentStep,
		StepTotal:   t.TotalSteps,
		Error:       t.Error,
	}
	if t.SubmittedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, t.SubmittedAt); err == nil {
			info.SubmittedAt = parsed
		}
	}
	if t.StartedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, t.StartedAt); err == nil {
			info.StartedAt = parsed
		}
	}
	if t.CompletedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
			info.CompletedAt = parsed
		}
	}
	return info
}

// tickCmd returns a command that sends a TickMsg after 1 second.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}
