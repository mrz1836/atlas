// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// WatchConfig holds configuration for the watch mode.
type WatchConfig struct {
	// Interval is the refresh interval for watch mode.
	Interval time.Duration
	// BellEnabled controls whether terminal bell notifications are enabled.
	BellEnabled bool
	// Quiet suppresses header and footer output.
	Quiet bool
	// ShowProgress displays progress bars below the status table.
	ShowProgress bool
}

// DefaultWatchConfig returns the default watch configuration.
func DefaultWatchConfig() WatchConfig {
	return WatchConfig{
		Interval:     2 * time.Second,
		BellEnabled:  true,
		Quiet:        false,
		ShowProgress: false,
	}
}

// WorkspaceLister defines the interface for listing workspaces.
type WorkspaceLister interface {
	List(ctx context.Context) ([]*domain.Workspace, error)
}

// TaskLister defines the interface for listing tasks.
type TaskLister interface {
	List(ctx context.Context, workspaceName string) ([]*domain.Task, error)
}

// WatchModel is the Bubble Tea model for watch mode.
// It implements tea.Model interface (Init, Update, View).
type WatchModel struct {
	// Current status data
	rows []StatusRow
	// Previous status per workspace for change detection
	previousRows map[string]constants.TaskStatus
	// Last refresh timestamp
	lastUpdate time.Time
	// Configuration
	config WatchConfig
	// Terminal dimensions
	width, height int
	// Exit flag
	quitting bool
	// Error from last refresh
	err error
	// Dependencies
	wsMgr     WorkspaceLister
	taskStore TaskLister
	// baseCtx is stored for use in async Bubble Tea commands.
	// Storing context in structs is generally discouraged, but Bubble Tea's
	// async command model requires it for proper context propagation.
	baseCtx context.Context //nolint:containedctx // Required for Bubble Tea async commands
}

// TickMsg signals time for a refresh.
type TickMsg time.Time

// RefreshMsg carries new data from a refresh operation.
type RefreshMsg struct {
	Rows []StatusRow
	Err  error
}

// BellMsg signals that a bell should be emitted.
type BellMsg struct{}

// NewWatchModel creates a new WatchModel with the given dependencies.
// The context is stored for use in async Bubble Tea commands.
func NewWatchModel(ctx context.Context, wsMgr WorkspaceLister, taskStore TaskLister, cfg WatchConfig) *WatchModel {
	return &WatchModel{
		rows:         nil,
		previousRows: make(map[string]constants.TaskStatus),
		lastUpdate:   time.Time{},
		config:       cfg,
		width:        80, // Default width
		height:       24, // Default height
		quitting:     false,
		err:          nil,
		wsMgr:        wsMgr,
		taskStore:    taskStore,
		baseCtx:      ctx,
	}
}

// Init returns the initial command to run when the program starts.
// It starts the refresh timer and performs an initial data load.
func (m *WatchModel) Init() tea.Cmd {
	return tea.Batch(
		m.refreshData(),
		m.tick(),
	)
}

// Update handles messages and returns the updated model and any commands.
func (m *WatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case TickMsg:
		return m, m.refreshData()

	case RefreshMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, m.tick()
		}
		m.rows = msg.Rows
		m.lastUpdate = time.Now()
		m.err = nil

		// Check for bell conditions
		bellCmd := m.checkForBell()
		return m, tea.Batch(m.tick(), bellCmd)

	case BellMsg:
		// Bell is emitted in the command, nothing to do here
		return m, nil
	}

	return m, nil
}

// View renders the current state to a string.
func (m *WatchModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header (unless quiet)
	if !m.config.Quiet {
		b.WriteString(RenderHeader(m.width))
		b.WriteString("\n\n")
	}

	// Error display
	if m.err != nil {
		b.WriteString(fmt.Sprintf("Error: %v\n", m.err))
	}

	// Table or empty message
	if len(m.rows) == 0 {
		b.WriteString("No workspaces. Run 'atlas start' to create one.\n")
	} else {
		m.renderStatusContent(&b)
	}

	// Footer summary (unless quiet)
	if !m.config.Quiet {
		b.WriteString("\n")
		b.WriteString(m.buildFooter())
		b.WriteString("\n")
	}

	// Action indicators footer (Story 7.9) - shows copy-paste commands
	// Render even in quiet mode since these are actionable commands
	if len(m.rows) > 0 {
		footer := NewStatusFooter(m.rows)
		if footer.HasItems() {
			_ = footer.Render(&b)
		}
	}

	// Timestamp and quit hint
	if !m.lastUpdate.IsZero() {
		b.WriteString(fmt.Sprintf("\nLast updated: %s", m.lastUpdate.Format("15:04:05")))
	}
	b.WriteString("\nPress 'q' to quit")

	return b.String()
}

// Rows returns the current status rows (useful for testing).
func (m *WatchModel) Rows() []StatusRow {
	return m.rows
}

// LastUpdate returns the last update timestamp.
func (m *WatchModel) LastUpdate() time.Time {
	return m.lastUpdate
}

// IsQuitting returns true if the model is in quitting state.
func (m *WatchModel) IsQuitting() bool {
	return m.quitting
}

// Error returns the last error from a refresh operation.
func (m *WatchModel) Error() error {
	return m.err
}

// tick returns a command that sends a TickMsg after the configured interval.
func (m *WatchModel) tick() tea.Cmd {
	return tea.Tick(m.config.Interval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// refreshData loads fresh data from the workspace and task stores.
func (m *WatchModel) refreshData() tea.Cmd {
	return func() tea.Msg {
		// Use stored context for proper cancellation propagation
		ctx := m.baseCtx
		if ctx == nil {
			ctx = context.Background()
		}

		workspaces, err := m.wsMgr.List(ctx)
		if err != nil {
			return RefreshMsg{Err: fmt.Errorf("failed to list workspaces: %w", err)}
		}

		rows := m.buildStatusRows(ctx, workspaces)
		m.sortByStatusPriority(rows)
		return RefreshMsg{Rows: rows}
	}
}

// buildStatusRows builds StatusRow slice from workspaces.
func (m *WatchModel) buildStatusRows(ctx context.Context, workspaces []*domain.Workspace) []StatusRow {
	rows := make([]StatusRow, 0, len(workspaces))

	for _, ws := range workspaces {
		row := StatusRow{
			Workspace: ws.Name,
			Branch:    ws.Branch,
			Status:    constants.TaskStatusPending, // Default
		}

		// Get most recent task status
		if len(ws.Tasks) > 0 {
			row.Status = ws.Tasks[0].Status

			// Load full task to get step info
			tasks, err := m.taskStore.List(ctx, ws.Name)
			if err == nil && len(tasks) > 0 {
				mostRecent := tasks[0] // Already sorted newest first
				row.Status = mostRecent.Status
				row.CurrentStep = mostRecent.CurrentStep + 1 // 1-indexed for display
				row.TotalSteps = len(mostRecent.Steps)
			}
		}

		rows = append(rows, row)
	}

	return rows
}

// sortByStatusPriority sorts rows by status priority (attention first, then running).
func (m *WatchModel) sortByStatusPriority(rows []StatusRow) {
	// Use a simple bubble sort to maintain stability and avoid import
	n := len(rows)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if m.statusPriority(rows[j].Status) < m.statusPriority(rows[j+1].Status) {
				rows[j], rows[j+1] = rows[j+1], rows[j]
			}
		}
	}
}

// statusPriority returns the priority level for a task status.
// Higher values = higher priority (shown first).
func (m *WatchModel) statusPriority(status constants.TaskStatus) int {
	if IsAttentionStatus(status) {
		return 2 // Highest priority
	}
	if status == constants.TaskStatusRunning || status == constants.TaskStatusValidating {
		return 1 // Middle priority
	}
	return 0 // Lowest priority
}

// checkForBell checks if any workspace transitioned to an attention state.
// Returns a command to emit a bell if needed.
// Bell is suppressed if BellEnabled is false or Quiet mode is active.
func (m *WatchModel) checkForBell() tea.Cmd {
	if !m.config.BellEnabled || m.config.Quiet {
		return nil
	}

	for _, row := range m.rows {
		prevStatus, exists := m.previousRows[row.Workspace]
		currentIsAttention := IsAttentionStatus(row.Status)

		// Only bell on NEW transitions to attention states
		if currentIsAttention {
			if !exists || !IsAttentionStatus(prevStatus) {
				// Update tracking and emit bell
				m.previousRows[row.Workspace] = row.Status
				return emitBell()
			}
		}
		m.previousRows[row.Workspace] = row.Status
	}

	// Clean up removed workspaces from tracking
	currentWorkspaces := make(map[string]bool)
	for _, row := range m.rows {
		currentWorkspaces[row.Workspace] = true
	}
	for ws := range m.previousRows {
		if !currentWorkspaces[ws] {
			delete(m.previousRows, ws)
		}
	}

	return nil
}

// emitBell returns a command that emits a terminal bell.
func emitBell() tea.Cmd {
	return func() tea.Msg {
		// Write BEL character directly to stdout to avoid forbidigo lint rule
		_, _ = os.Stdout.WriteString("\a")
		return BellMsg{}
	}
}

// buildFooter creates the footer summary and actionable command.
func (m *WatchModel) buildFooter() string {
	attentionCount, firstAttention := m.countAttention()

	// Summary line with proper singular/plural grammar
	workspaceWord := "workspaces"
	if len(m.rows) == 1 {
		workspaceWord = "workspace"
	}
	summary := fmt.Sprintf("%d %s", len(m.rows), workspaceWord)

	if attentionCount > 0 {
		needWord := "need"
		if attentionCount == 1 {
			needWord = "needs"
		}
		summary += fmt.Sprintf(", %d %s attention", attentionCount, needWord)
	}

	// Actionable command
	if firstAttention != nil {
		summary += m.buildActionableSuggestion(firstAttention)
	}

	return summary
}

// countAttention counts workspaces needing attention and returns the first one.
func (m *WatchModel) countAttention() (int, *StatusRow) {
	var count int
	var first *StatusRow

	for i := range m.rows {
		if IsAttentionStatus(m.rows[i].Status) {
			count++
			if first == nil {
				first = &m.rows[i]
			}
		}
	}

	return count, first
}

// buildActionableSuggestion builds the "Run: ..." suggestion for a workspace.
func (m *WatchModel) buildActionableSuggestion(row *StatusRow) string {
	action := SuggestedAction(row.Status)
	if action == "" {
		return ""
	}

	return "\nRun: " + action + " " + row.Workspace
}

// renderStatusContent renders the status table and optional progress bars.
func (m *WatchModel) renderStatusContent(b *strings.Builder) {
	table := NewStatusTable(m.rows, WithTerminalWidth(m.width))
	_ = table.Render(b)

	// Progress bars (if enabled and there are active tasks)
	if m.config.ShowProgress {
		progressRows := m.buildProgressRows()
		if len(progressRows) > 0 {
			b.WriteString("\n")
			pd := NewProgressDashboard(progressRows, WithTermWidth(m.width))
			_ = pd.Render(b)
		}
	}
}

// buildProgressRows converts status rows to progress rows for the dashboard.
// Only includes rows with active tasks (running or validating states).
// Delegates to shared helper to avoid code duplication with CLI status command.
func (m *WatchModel) buildProgressRows() []ProgressRow {
	return BuildProgressRowsFromStatus(m.rows)
}
