// Package cli provides the command-line interface for atlas.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
)

// logsOptions holds the options for the logs command.
type logsOptions struct {
	follow   bool
	stepName string
	taskID   string
	tail     int
}

// logStyles holds lipgloss styles for log output formatting.
type logStyles struct {
	dim      lipgloss.Style
	stepName lipgloss.Style
	info     lipgloss.Style
	warn     lipgloss.Style
	errorSty lipgloss.Style
	debug    lipgloss.Style
}

// newLogStyles creates styles for log output.
func newLogStyles() *logStyles {
	return &logStyles{
		dim:      lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}),
		stepName: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0087AF", Dark: "#00D7FF"}).Bold(true),
		info:     lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#0087AF", Dark: "#00D7FF"}),
		warn:     lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#AF8700", Dark: "#FFD700"}),
		errorSty: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#AF0000", Dark: "#FF5F5F"}),
		debug:    lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#585858", Dark: "#6C6C6C"}),
	}
}

// levelColor returns the appropriate style for a log level.
func (s *logStyles) levelColor(level string) lipgloss.Style {
	switch strings.ToLower(level) {
	case "info":
		return s.info
	case "warn", "warning":
		return s.warn
	case "error", "fatal", "panic":
		return s.errorSty
	case "debug", "trace":
		return s.debug
	default:
		return s.info
	}
}

// logEntry represents a parsed JSON-lines log entry.
type logEntry struct {
	Timestamp     time.Time `json:"ts"`
	Level         string    `json:"level"`
	Event         string    `json:"event"`
	WorkspaceName string    `json:"workspace_name"`
	TaskID        string    `json:"task_id"`
	StepName      string    `json:"step_name"`
	DurationMs    int64     `json:"duration_ms,omitempty"`
	Error         string    `json:"error,omitempty"`
}

// logsResult represents the JSON output for logs operations.
type logsResult struct {
	Status    string `json:"status"`
	Workspace string `json:"workspace"`
	TaskID    string `json:"task_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// addWorkspaceLogsCmd adds the logs subcommand to the workspace command.
func addWorkspaceLogsCmd(parent *cobra.Command) {
	var (
		follow   bool
		stepName string
		taskID   string
		tail     int
	)

	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "View workspace task logs",
		Long: `Display task execution logs for a workspace.

Shows the most recent task's logs by default. Use flags to filter
by specific task or step, or follow logs in real-time.

Examples:
  atlas workspace logs auth           # View most recent task logs
  atlas workspace logs auth -f        # Follow logs in real-time
  atlas workspace logs auth --step validate  # Filter by step
  atlas workspace logs auth --task task-550e8400-e29b-41d4-a716-446655440000  # Specific task
  atlas workspace logs auth --tail 50  # Last 50 lines only`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runWorkspaceLogs(cmd.Context(), cmd, os.Stdout, args[0], logsOptions{
				follow:   follow,
				stepName: stepName,
				taskID:   taskID,
				tail:     tail,
			}, "")
			// If JSON error was already output, silence cobra's error printing
			if stderrors.Is(err, errors.ErrJSONErrorOutput) {
				cmd.SilenceErrors = true
			}
			return err
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().StringVar(&stepName, "step", "", "Filter logs by step name")
	cmd.Flags().StringVar(&taskID, "task", "", "Show logs for specific task ID")
	cmd.Flags().IntVarP(&tail, "tail", "n", 0, "Show last n lines (0 = all)")

	parent.AddCommand(cmd)
}

// runWorkspaceLogs executes the workspace logs command.
func runWorkspaceLogs(ctx context.Context, cmd *cobra.Command, w io.Writer, name string, opts logsOptions, storeBaseDir string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Get output format from global flags
	output := cmd.Flag("output").Value.String()

	return runWorkspaceLogsWithOutput(ctx, w, name, opts, storeBaseDir, output)
}

// runWorkspaceLogsWithOutput executes the workspace logs command with explicit output format.
func runWorkspaceLogsWithOutput(ctx context.Context, w io.Writer, name string, opts logsOptions, storeBaseDir, output string) error {
	logger := GetLogger()

	// Respect NO_COLOR environment variable (UX-7)
	tui.CheckNoColor()

	// Create store
	store, err := workspace.NewFileStore(storeBaseDir)
	if err != nil {
		logger.Debug().Err(err).Msg("failed to create workspace store")
		if output == OutputJSON {
			_ = outputLogsErrorJSON(w, name, "", fmt.Sprintf("failed to create workspace store: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Check if workspace exists
	exists, err := store.Exists(ctx, name)
	if err != nil {
		logger.Debug().Err(err).Str("workspace", name).Msg("failed to check workspace existence")
		if output == OutputJSON {
			_ = outputLogsErrorJSON(w, name, "", fmt.Sprintf("failed to check workspace: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to check workspace '%s': %w", name, err)
	}

	if !exists {
		return handleLogsWorkspaceNotFound(name, output, w)
	}

	// Get workspace to access tasks
	ws, err := store.Get(ctx, name)
	if err != nil {
		if output == OutputJSON {
			_ = outputLogsErrorJSON(w, name, "", fmt.Sprintf("failed to get workspace: %v", err))
			return errors.ErrJSONErrorOutput
		}
		return fmt.Errorf("failed to get workspace '%s': %w", name, err)
	}

	// Determine which task to show logs for
	taskRef, err := selectTask(ws, opts.taskID)
	if err != nil {
		if stderrors.Is(err, errors.ErrNoTasksFound) {
			return handleNoLogsFound(w, name, output)
		}
		if output == OutputJSON {
			_ = outputLogsErrorJSON(w, name, opts.taskID, err.Error())
			return errors.ErrJSONErrorOutput
		}
		return err
	}

	// Construct log file path
	logPath, err := getTaskLogPath(storeBaseDir, name, taskRef.ID)
	if err != nil {
		if output == OutputJSON {
			_ = outputLogsErrorJSON(w, name, taskRef.ID, err.Error())
			return errors.ErrJSONErrorOutput
		}
		return err
	}

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return handleNoLogsFound(w, name, output)
	}

	// Handle follow mode
	if opts.follow {
		return followLogs(ctx, logPath, w, newLogStyles(), output, opts.stepName)
	}

	// Read and display logs
	return displayLogs(ctx, logPath, w, opts, output)
}

// handleLogsWorkspaceNotFound handles the case when a workspace is not found.
func handleLogsWorkspaceNotFound(name, output string, w io.Writer) error {
	if output == OutputJSON {
		_ = outputLogsErrorJSON(w, name, "", "workspace not found")
		return errors.ErrJSONErrorOutput
	}
	// Match AC10 format: "Workspace 'nonexistent' not found"
	//nolint:staticcheck // ST1005: AC requires capitalized error for user-facing message
	return fmt.Errorf("Workspace '%s' not found: %w", name, errors.ErrWorkspaceNotFound)
}

// handleNoLogsFound handles the case when no logs exist.
func handleNoLogsFound(w io.Writer, name, output string) error {
	if output == OutputJSON {
		_, _ = fmt.Fprintln(w, "[]")
		return nil
	}
	// Match AC6 format
	_, _ = fmt.Fprintf(w, "No logs found for workspace '%s'\n", name)
	return nil
}

// selectTask determines which task to show logs for.
func selectTask(ws *domain.Workspace, requestedTaskID string) (*domain.TaskRef, error) {
	if len(ws.Tasks) == 0 {
		return nil, errors.ErrNoTasksFound
	}

	// If specific task requested, find it
	if requestedTaskID != "" {
		for i := range ws.Tasks {
			if ws.Tasks[i].ID == requestedTaskID {
				return &ws.Tasks[i], nil
			}
		}
		//nolint:staticcheck // ST1005: AC requires capitalized error for user-facing message
		return nil, fmt.Errorf("Task '%s' not found: %w", requestedTaskID, errors.ErrTaskNotFound)
	}

	// Find most recent task
	return findMostRecentTask(ws.Tasks), nil
}

// findMostRecentTask returns the most recently started task.
func findMostRecentTask(tasks []domain.TaskRef) *domain.TaskRef {
	if len(tasks) == 0 {
		return nil
	}

	// Sort by StartedAt descending (most recent first), with ID as tiebreaker for determinism
	sorted := make([]domain.TaskRef, len(tasks))
	copy(sorted, tasks)
	sort.Slice(sorted, func(i, j int) bool {
		// Both nil - use ID as tiebreaker (lexicographically larger ID = more recent convention)
		if sorted[i].StartedAt == nil && sorted[j].StartedAt == nil {
			return sorted[i].ID > sorted[j].ID
		}
		if sorted[i].StartedAt == nil {
			return false
		}
		if sorted[j].StartedAt == nil {
			return true
		}
		// Equal times - use ID as tiebreaker
		if sorted[i].StartedAt.Equal(*sorted[j].StartedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].StartedAt.After(*sorted[j].StartedAt)
	})

	return &sorted[0]
}

// getTaskLogPath constructs the path to a task's log file.
func getTaskLogPath(storeBaseDir, wsName, taskID string) (string, error) {
	if storeBaseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		storeBaseDir = filepath.Join(home, constants.AtlasHome)
	}
	return filepath.Join(storeBaseDir, constants.WorkspacesDir, wsName, constants.TasksDir, taskID, constants.TaskLogFileName), nil
}

// displayLogs reads and displays log content.
func displayLogs(ctx context.Context, logPath string, w io.Writer, opts logsOptions, output string) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Open and read log file
	f, err := os.Open(logPath) //#nosec G304 -- path is constructed from validated workspace name
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Read all lines
	var lines [][]byte
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, append([]byte{}, scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	// Filter by step if requested
	if opts.stepName != "" {
		lines = filterByStep(lines, opts.stepName)
		if len(lines) == 0 {
			if output == OutputJSON {
				_, _ = fmt.Fprintln(w, "[]")
				return nil
			}
			_, _ = fmt.Fprintf(w, "No logs found for step '%s'\n", opts.stepName)
			return nil
		}
	}

	// Apply tail if requested
	if opts.tail > 0 && len(lines) > opts.tail {
		lines = lines[len(lines)-opts.tail:]
	}

	// Output based on format
	if output == OutputJSON {
		return outputLogsJSON(w, lines)
	}

	return outputLogsFormatted(w, lines)
}

// filterByStep filters log lines to only those matching the given step name.
func filterByStep(lines [][]byte, stepName string) [][]byte {
	var filtered [][]byte
	stepLower := strings.ToLower(stepName)

	for _, line := range lines {
		var entry logEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // Skip invalid JSON lines
		}

		// Case-insensitive prefix match
		if strings.HasPrefix(strings.ToLower(entry.StepName), stepLower) {
			filtered = append(filtered, line)
		}
	}

	return filtered
}

// outputLogsJSON outputs log lines as a JSON array.
func outputLogsJSON(w io.Writer, lines [][]byte) error {
	// Parse each line and collect into array
	entries := make([]json.RawMessage, 0, len(lines))
	for _, line := range lines {
		// Validate it's valid JSON before adding
		var js json.RawMessage
		if err := json.Unmarshal(line, &js); err != nil {
			continue // Skip invalid JSON
		}
		entries = append(entries, js)
	}

	// Handle empty case
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(w, "[]")
		return nil
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}

// outputLogsFormatted outputs log lines with formatting and colors.
//
//nolint:unparam // error return is for interface consistency
func outputLogsFormatted(w io.Writer, lines [][]byte) error {
	styles := newLogStyles()

	for _, line := range lines {
		formatted := formatLogLine(line, styles)
		_, _ = fmt.Fprintln(w, formatted)
	}

	return nil
}

// formatLogLine formats a single log line with styling.
func formatLogLine(line []byte, styles *logStyles) string {
	var entry logEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		// Fallback: return raw line if not valid JSON
		return string(line)
	}

	// Format: "2 min ago [INFO] implement: step execution started"
	timeStr := tui.RelativeTime(entry.Timestamp)
	levelStyle := styles.levelColor(entry.Level)

	// Build the formatted line
	var parts []string
	parts = append(parts, styles.dim.Render(timeStr))
	parts = append(parts, fmt.Sprintf("[%s]", levelStyle.Render(strings.ToUpper(entry.Level))))

	if entry.StepName != "" {
		parts = append(parts, styles.stepName.Render(entry.StepName)+":")
	}

	parts = append(parts, entry.Event)

	// Add error details if present
	if entry.Error != "" {
		parts = append(parts, styles.errorSty.Render(fmt.Sprintf("(error: %s)", entry.Error)))
	}

	// Add duration if present
	if entry.DurationMs > 0 {
		parts = append(parts, styles.dim.Render(fmt.Sprintf("[%dms]", entry.DurationMs)))
	}

	return strings.Join(parts, " ")
}

// followLogs streams new log entries in real-time.
func followLogs(ctx context.Context, path string, w io.Writer, styles *logStyles, output, stepFilter string) error {
	f, err := os.Open(path) //#nosec G304 -- path is constructed from validated workspace name
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Seek to end of file
	if _, err = f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	if output != OutputJSON {
		msg := "Watching for new log entries... (Ctrl+C to stop)"
		if stepFilter != "" {
			msg = fmt.Sprintf("Watching for new log entries (step: %s)... (Ctrl+C to stop)", stepFilter)
		}
		_, _ = fmt.Fprintln(w, styles.dim.Render(msg))
	}

	return pollLogFile(ctx, f, w, styles, output, stepFilter)
}

// pollLogFile continuously polls a log file for new content.
func pollLogFile(ctx context.Context, f *os.File, w io.Writer, styles *logStyles, output, stepFilter string) error {
	reader := bufio.NewReader(f)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := readNewLines(reader, w, styles, output, stepFilter); err != nil {
				return err
			}
		}
	}
}

// readNewLines reads and outputs any new lines from the reader.
func readNewLines(reader *bufio.Reader, w io.Writer, styles *logStyles, output, stepFilter string) error {
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil // No more data
		}
		if err != nil {
			return fmt.Errorf("failed to read log: %w", err)
		}

		// Apply step filter if specified
		if stepFilter != "" {
			var entry logEntry
			if err := json.Unmarshal(line, &entry); err == nil {
				if !strings.HasPrefix(strings.ToLower(entry.StepName), strings.ToLower(stepFilter)) {
					continue // Skip lines that don't match the step filter
				}
			}
		}

		if output == OutputJSON {
			_, _ = w.Write(line)
		} else {
			_, _ = fmt.Fprintln(w, formatLogLine(line, styles))
		}
	}
}

// outputLogsErrorJSON outputs an error result as JSON.
// Returns the encoding error if JSON output fails, which callers typically
// ignore with `_ =` since ErrJSONErrorOutput is already being returned.
// This is intentional: if we can't write JSON, there's no useful fallback,
// and the caller's return of ErrJSONErrorOutput signals to cobra to suppress
// its own error printing regardless of whether our JSON succeeded.
func outputLogsErrorJSON(w io.Writer, name, taskID, errMsg string) error {
	result := logsResult{
		Status:    "error",
		Workspace: name,
		TaskID:    taskID,
		Error:     errMsg,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
