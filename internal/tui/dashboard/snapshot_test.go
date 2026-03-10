package dashboard_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/mrz1836/atlas/internal/daemon"
	"github.com/mrz1836/atlas/internal/tui/dashboard"
)

// update controls whether golden files are regenerated on this run.
// Run with: go test ./internal/tui/dashboard/... -run TestSnapshot -update
var update = flag.Bool("update", false, "update golden files") //nolint:gochecknoglobals // test flag

// testdataDir is the directory containing golden test files.
// Using "snapshots" instead of "testdata" to avoid project-wide .gitignore rule.
const testdataDir = "snapshots"

// fixedTime is a deterministic timestamp used in snapshot tests to ensure stable output.
var fixedTime = time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // test helper

// viewSnapshot renders the model at the given terminal dimensions, strips ANSI,
// and normalizes time-varying values for stable golden-file comparison.
func viewSnapshot(m *dashboard.Model, width, height int) string {
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	v := m.View()
	return normalizeSnapshot(stripANSIForSnapshot(v.Content))
}

// elapsedPattern matches elapsed time strings produced by FormatDuration:
// e.g., "3617644.0s", "150ms", "3.5s" — and also bare integers that appear
// before the pane divider when the unit suffix is clipped by column trimming.
var elapsedPattern = regexp.MustCompile(`\d+(?:\.\d+)?(?:ms|s)\b|\b\d{4,}\b`)

// normalizeSnapshot replaces time-varying elapsed values with a stable placeholder
// so that snapshot comparisons are deterministic regardless of when the test runs.
func normalizeSnapshot(s string) string {
	return elapsedPattern.ReplaceAllString(s, "Xs")
}

// stripANSIForSnapshot removes ANSI escape sequences from s.
func stripANSIForSnapshot(s string) string {
	var out strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			i += 2
			for i < len(runes) && (runes[i] < 0x40 || runes[i] > 0x7E) {
				i++
			}
			if i < len(runes) {
				i++
			}
			continue
		}
		if utf8.ValidRune(runes[i]) {
			out.WriteRune(runes[i])
		}
		i++
	}
	return out.String()
}

// goldenPath returns the path to the golden file for a given test name.
func goldenPath(name string) string {
	return filepath.Join(testdataDir, name+".golden")
}

// checkOrUpdateGolden compares got against the golden file content, or writes the
// golden file if -update is set (or the file does not yet exist).
func checkOrUpdateGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		t.Logf("updated golden file: %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // path is under testdata/
	if err != nil {
		// Golden file missing → write it now and skip the comparison.
		if os.IsNotExist(err) {
			if writeErr := os.WriteFile(path, []byte(got), 0o600); writeErr != nil {
				t.Fatalf("write initial golden %s: %v", path, writeErr)
			}
			t.Logf("created golden file (first run): %s", path)
			return
		}
		t.Fatalf("read golden %s: %v", path, err)
	}

	// Normalize the golden content the same way to ensure stability.
	wantNorm := normalizeSnapshot(string(want))
	if got != wantNorm {
		t.Errorf("snapshot mismatch for %q\n\nGot:\n%s\n\nWant (golden):\n%s\n\nRun with -update to regenerate.", name, got, wantNorm)
	}
}

// ── Snapshot tests ─────────────────────────────────────────────────────────────

// TestSnapshot_EmptyState captures the View() at 80×24 with no tasks.
func TestSnapshot_EmptyState(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	got := viewSnapshot(m, 80, 24)
	checkOrUpdateGolden(t, "empty_80x24", got)
}

// TestSnapshot_EmptyState_Wide captures the empty state at a wider terminal.
func TestSnapshot_EmptyState_Wide(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	got := viewSnapshot(m, 120, 40)
	checkOrUpdateGolden(t, "empty_120x40", got)
}

// TestSnapshot_MultipleTasks captures a model with several tasks in mixed states.
func TestSnapshot_MultipleTasks(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Inject a mix of tasks across all statuses.
	tasks := []struct {
		id     string
		desc   string
		status string
		step   string
	}{
		{"task-q1", "fix-null-pointer", "queued", ""},
		{"task-r1", "add-unit-tests", "running", "validate"},
		{"task-a1", "refactor-auth", "awaiting_approval", ""},
		{"task-c1", "update-readme", "completed", ""},
		{"task-f1", "deploy-to-prod", "failed", ""},
		{"task-p1", "migrate-db", "paused", ""},
	}
	for _, tc := range tasks {
		m.Update(dashboard.TaskEventMsg{
			Event: daemon.TaskEvent{
				Type:        daemon.EventTaskSubmitted,
				TaskID:      tc.id,
				Status:      tc.status,
				Message:     tc.desc,
				Description: tc.desc,
				Step:        tc.step,
				Time:        fixedTime.Format(time.RFC3339),
			},
		})
	}

	// Select the running task.
	m.Update(dashboard.TaskSelectedMsg{TaskID: "task-r1"})

	got := viewSnapshot(m, 120, 40)
	checkOrUpdateGolden(t, "multiple_tasks_120x40", got)
}

// TestSnapshot_LogViewMode captures the full-screen log view.
func TestSnapshot_LogViewMode(t *testing.T) {
	t.Parallel()
	m := dashboard.New()

	// Add a task.
	m.Update(dashboard.TaskEventMsg{
		Event: daemon.TaskEvent{
			Type:        daemon.EventTaskSubmitted,
			TaskID:      "log-task-1",
			Status:      "running",
			Description: "streaming-logs-task",
			Time:        fixedTime.Format(time.RFC3339),
		},
	})
	m.Update(dashboard.TaskSelectedMsg{TaskID: "log-task-1"})

	// Add some log entries with fixed timestamps for deterministic output.
	for i, entry := range []struct {
		level string
		msg   string
	}{
		{"info", "Starting task execution"},
		{"debug", "Loading configuration"},
		{"info", "Running validation step"},
		{"warn", "Slow network response"},
		{"error", "Timeout on external call"},
	} {
		m.Update(dashboard.LogEntryMsg{
			Entry: daemon.LogEntry{
				ID:        formatStreamID(i),
				Timestamp: fixedTime.Add(time.Duration(i) * time.Second),
				Level:     entry.level,
				Message:   entry.msg,
			},
		})
	}

	// Switch to full log view.
	m.Update(dashboard.ViewChangeMsg{Mode: dashboard.ViewModeLog})

	got := viewSnapshot(m, 80, 24)
	checkOrUpdateGolden(t, "log_view_80x24", got)
}

// TestSnapshot_HelpOverlay captures the help overlay.
func TestSnapshot_HelpOverlay(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press '?' to open help overlay.
	m.Update(confirmKeyPress("?"))

	got := viewSnapshot(m, 80, 24)
	checkOrUpdateGolden(t, "help_overlay_80x24", got)
}

// TestSnapshot_StartupError captures the startup error view (daemon not running).
func TestSnapshot_StartupError(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	m.SetStartupError("Cannot connect to Atlas daemon: connection refused\n\nRun: atlas daemon start\n\nPress q to quit.")
	got := viewSnapshot(m, 80, 24)
	checkOrUpdateGolden(t, "startup_error_80x24", got)
}

// TestSnapshot_Narrow captures the layout at a narrow (sub-80) terminal width.
func TestSnapshot_Narrow(t *testing.T) {
	t.Parallel()
	m := dashboard.New()
	got := viewSnapshot(m, 60, 20)
	checkOrUpdateGolden(t, "narrow_60x20", got)
}

// formatStreamID returns a fake Redis stream ID for test log entries.
func formatStreamID(i int) string {
	digit := '0' + rune(i) //nolint:gosec // safe: i is bounded by test input (0-4)
	return strings.Repeat("0", 13-len(string(digit))) + string(digit) + "-0"
}
