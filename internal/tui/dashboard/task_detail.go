package dashboard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/mrz1836/atlas/internal/tui"
)

// TaskDetail is the right-panel task detail component.
// It is a pure renderer — it holds a pointer to the selected TaskInfo and renders
// all metadata, step progress, and a mini log tail from that data alone.
// No external state, no Bubble Tea model methods required.
type TaskDetail struct {
	task    *TaskInfo
	logTail []string // last ~10 log lines for the mini log tail section
}

// NewTaskDetail creates an empty TaskDetail.
// Call SetTask before rendering to populate the view.
func NewTaskDetail() TaskDetail {
	return TaskDetail{}
}

// SetTask updates the selected task. Pass nil to clear the panel.
func (td *TaskDetail) SetTask(task *TaskInfo) {
	td.task = task
}

// SetLogTail replaces the mini log tail lines. Callers should pass the last ~10 lines.
func (td *TaskDetail) SetLogTail(lines []string) {
	td.logTail = lines
}

// Task returns the currently set task, or nil.
func (td *TaskDetail) Task() *TaskInfo { return td.task }

// View renders the task detail panel into a string of height rows × width columns.
// Sections: header → metadata grid → step progress → mini log tail.
func (td *TaskDetail) View(width, height int) string {
	if td.task == nil {
		return detailEmptyState(width, height)
	}

	s := GetStyles()
	var lines []string

	// ── Header: task name + status badge ─────────────────────────────────────
	lines = append(lines, detailHeader(td.task, width, s)...)
	lines = append(lines, "") // blank separator

	// ── Metadata grid ─────────────────────────────────────────────────────────
	lines = append(lines, detailMetadata(td.task, width)...)
	lines = append(lines, "") // blank separator

	// ── Step progress ─────────────────────────────────────────────────────────
	if td.task.StepTotal > 0 || td.task.CurrentStep != "" {
		lines = append(lines, detailStepProgress(td.task)...)
		lines = append(lines, "") // blank separator
	}

	// ── Mini log tail ─────────────────────────────────────────────────────────
	if len(td.logTail) > 0 {
		lines = append(lines, detailLogTail(td.logTail, width)...)
	}

	return normaliseLines(lines, width, height)
}

// ── Section renderers ─────────────────────────────────────────────────────────

// detailHeader renders the task name and a status badge on one line.
func detailHeader(task *TaskInfo, width int, s *Styles) []string {
	badge := detailStatusBadge(task.Status)
	badgeLen := len([]rune(stripANSI(badge)))

	title := truncateRunes(task.Description, width-badgeLen-2)
	titleStyled := s.Header.Render(title)

	// Lay out: [title…] [badge] right-aligned.
	titleW := len([]rune(stripANSI(titleStyled)))
	gap := width - titleW - badgeLen
	if gap < 1 {
		gap = 1
	}
	line := titleStyled + strings.Repeat(" ", gap) + badge
	return []string{line}
}

// detailStatusBadge returns a colored status label with brackets.
func detailStatusBadge(status TaskStatus) string {
	icon := taskListIcon(status)
	label := icon + " " + string(status)
	style := lipgloss.NewStyle().Foreground(StatusColor(status)).Bold(true)
	return style.Render("[" + label + "]")
}

// detailMetadata renders a two-column key: value grid of task metadata.
func detailMetadata(task *TaskInfo, width int) []string {
	pairs := buildMetadataPairs(task, width)

	keyStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	valStyle := lipgloss.NewStyle()

	lines := make([]string, 0, len(pairs))
	for _, p := range pairs {
		k := keyStyle.Render(padRight(p.key+":", 10))
		v := valStyle.Render(p.val)
		line := k + " " + v
		lines = append(lines, truncateLine(line, width))
	}
	return lines
}

// buildMetadataPairs assembles the ordered key/value pairs for the metadata grid.
func buildMetadataPairs(task *TaskInfo, width int) []struct{ key, val string } {
	type pair = struct{ key, val string }
	pairs := make([]pair, 0, 11)

	if task.Template != "" {
		pairs = append(pairs, pair{"Template", task.Template})
	}
	if task.Agent != "" {
		pairs = append(pairs, pair{"Agent", task.Agent})
	}
	if task.Model != "" {
		pairs = append(pairs, pair{"Model", task.Model})
	}
	if task.Branch != "" {
		pairs = append(pairs, pair{"Branch", task.Branch})
	}
	if task.Workspace != "" {
		pairs = append(pairs, pair{"Workspace", task.Workspace})
	}
	if task.Priority != "" {
		pairs = append(pairs, pair{"Priority", task.Priority})
	}
	if !task.SubmittedAt.IsZero() {
		pairs = append(pairs, pair{"Submitted", task.SubmittedAt.Format("15:04:05")})
	}
	if !task.StartedAt.IsZero() {
		pairs = append(pairs, pair{"Started", task.StartedAt.Format("15:04:05")})
	}
	if !task.CompletedAt.IsZero() {
		pairs = append(pairs, pair{"Completed", task.CompletedAt.Format("15:04:05")})
	}
	if task.PRURL != "" {
		pairs = append(pairs, pair{"PR", truncateRunes(task.PRURL, width-12)})
	}
	if task.Error != "" {
		pairs = append(pairs, pair{"Error", truncateRunes(task.Error, width-10)})
	}
	return pairs
}

// detailStepProgress renders a list of step entries showing status.
// "✓ step N" / "▶ validate  running…" / "○ step N"
func detailStepProgress(task *TaskInfo) []string {
	lines := []string{lipgloss.NewStyle().Bold(true).Render("Steps:")}

	if task.StepTotal > 0 {
		for i := 1; i <= task.StepTotal; i++ {
			lines = append(lines, renderStepLine(task, i))
		}
	} else if task.CurrentStep != "" {
		// No total known — just show the current step.
		lines = append(lines, fmt.Sprintf("  ▶ %s  running…", task.CurrentStep))
	}

	return lines
}

// renderStepLine renders one step row for the step progress list.
func renderStepLine(task *TaskInfo, stepNum int) string {
	switch {
	case stepNum < task.StepIndex:
		return fmt.Sprintf("  ✓ step %d", stepNum)
	case stepNum == task.StepIndex:
		label := task.CurrentStep
		if label == "" {
			label = fmt.Sprintf("step %d", stepNum)
		}
		return fmt.Sprintf("  ▶ %s  running…", label)
	default:
		return fmt.Sprintf("  ○ step %d", stepNum)
	}
}

// detailLogTail renders the last ~10 log lines with a section header.
func detailLogTail(logLines []string, width int) []string {
	header := lipgloss.NewStyle().Bold(true).Render("Recent Logs:")
	lines := make([]string, 0, 1+len(logLines))
	lines = append(lines, header)

	dimStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted).Faint(true)
	for _, l := range logLines {
		lines = append(lines, dimStyle.Render(truncateRunes(l, width-2)))
	}
	return lines
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// detailEmptyState returns a placeholder view when no task is selected.
func detailEmptyState(width, height int) string {
	msg := "Select a task to view details"
	blank := strings.Repeat(" ", width)
	rows := make([]string, height)
	rows[0] = padRight(msg, width)
	for i := 1; i < height; i++ {
		rows[i] = blank
	}
	return strings.Join(rows, "\n")
}

// normaliseLines pads or truncates lines to exactly height rows of width columns.
func normaliseLines(lines []string, width, height int) string {
	blank := strings.Repeat(" ", width)
	out := make([]string, height)
	for i := range out {
		if i < len(lines) {
			out[i] = padRight(truncateRunes(stripANSI(lines[i]), width), width)
		} else {
			out[i] = blank
		}
	}
	return strings.Join(out, "\n")
}

// truncateLine truncates a line to width runes (plain string, no ANSI).
func truncateLine(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width])
}

// stripANSI removes ANSI escape sequences from s for accurate rune-width measurement.
// This is a lightweight implementation sufficient for width calculations.
func stripANSI(s string) string {
	var out strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			// Skip until 'm' (SGR terminator) or other final byte.
			i += 2
			for i < len(runes) && (runes[i] < 0x40 || runes[i] > 0x7E) {
				i++
			}
			if i < len(runes) {
				i++ // consume the final byte
			}
		} else {
			out.WriteRune(runes[i])
			i++
		}
	}
	return out.String()
}
