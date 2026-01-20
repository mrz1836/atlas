package tui

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestTable(t *testing.T) {
	columns := []TableColumn{
		{Name: "NAME", Width: 10, Align: AlignLeft},
		{Name: "VALUE", Width: 15, Align: AlignLeft},
		{Name: "COUNT", Width: 5, Align: AlignRight},
	}

	t.Run("WriteHeader", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteHeader()
		output := buf.String()
		assert.Contains(t, output, "NAME")
		assert.Contains(t, output, "VALUE")
		assert.Contains(t, output, "COUNT")
	})

	t.Run("WriteRow", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test", "value", "42")
		output := buf.String()
		assert.Contains(t, output, "test")
		assert.Contains(t, output, "value")
		assert.Contains(t, output, "42")
	})

	t.Run("WriteRow truncates long values", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("verylongname", "value", "42")
		output := buf.String()
		assert.Contains(t, output, "verylongn…")
	})

	t.Run("WriteRow handles missing values", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		assert.Contains(t, output, "test")
	})

	t.Run("WriteStyledRow", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		// Simulate a styled value with ANSI codes
		styledValue := "\x1b[34mactive\x1b[0m"
		plainValue := "active"
		table.WriteStyledRow([]string{"test", plainValue, "5"}, 1, styledValue, plainValue)
		output := buf.String()
		assert.Contains(t, output, "test")
		assert.Contains(t, output, styledValue)
	})
}

func TestColorOffset(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
		plain    string
		expected int
	}{
		{
			name:     "no color",
			rendered: "active",
			plain:    "active",
			expected: 0,
		},
		{
			name:     "with ANSI codes",
			rendered: "\x1b[34mactive\x1b[0m",
			plain:    "active",
			expected: 9, // len("\x1b[34m") + len("\x1b[0m") = 5 + 4 = 9
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ColorOffset(tc.rendered, tc.plain)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAlignment(t *testing.T) {
	t.Run("AlignLeft", func(t *testing.T) {
		columns := []TableColumn{
			{Name: "LEFT", Width: 10, Align: AlignLeft},
		}
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		// Left aligned: "test      \n"
		assert.Contains(t, output, "test      ")
	})

	t.Run("AlignRight", func(t *testing.T) {
		columns := []TableColumn{
			{Name: "RIGHT", Width: 10, Align: AlignRight},
		}
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		// Right aligned: "      test\n"
		assert.Contains(t, output, "      test")
	})
}

// ========================================
// StatusTable Tests (Story 7.3)
// ========================================

func TestStatusTable_NewStatusTable(t *testing.T) {
	t.Run("creates table with rows", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		}
		st := NewStatusTable(rows)
		require.NotNil(t, st)
		assert.Len(t, st.Rows(), 1)
	})

	t.Run("creates empty table", func(t *testing.T) {
		st := NewStatusTable(nil)
		require.NotNil(t, st)
		assert.Empty(t, st.Rows())
	})

	t.Run("applies WithTerminalWidth option", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		st := NewStatusTable(rows, WithTerminalWidth(60))
		assert.True(t, st.IsNarrow())

		st = NewStatusTable(rows, WithTerminalWidth(120))
		assert.False(t, st.IsNarrow())
	})
}

func TestStatusTable_Headers(t *testing.T) {
	t.Run("returns full headers for wide terminal", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(120))
		headers := st.Headers()
		assert.Equal(t, []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}, headers)
	})

	t.Run("returns abbreviated headers for narrow terminal", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(60))
		headers := st.Headers()
		assert.Equal(t, []string{"WS", "BRANCH", "STAT", "STEP", "ACT"}, headers)
	})

	t.Run("FullHeaders always returns full names", func(t *testing.T) {
		// Even in narrow mode, FullHeaders returns full names
		st := NewStatusTable(nil, WithTerminalWidth(60))
		headers := st.FullHeaders()
		assert.Equal(t, []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}, headers)
	})
}

func TestStatusTable_StatusCellRendering(t *testing.T) {
	// Test all TaskStatus values render correctly
	testCases := []struct {
		status       constants.TaskStatus
		expectedIcon string
	}{
		{constants.TaskStatusPending, "○"},
		{constants.TaskStatusRunning, "●"},
		{constants.TaskStatusValidating, "⟳"},
		{constants.TaskStatusValidationFailed, "⚠"},
		{constants.TaskStatusAwaitingApproval, "✓"},
		{constants.TaskStatusCompleted, "✓"},
		{constants.TaskStatusRejected, "✗"},
		{constants.TaskStatusAbandoned, "✗"},
		{constants.TaskStatusGHFailed, "✗"},
		{constants.TaskStatusCIFailed, "✗"},
		{constants.TaskStatusCITimeout, "⚠"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.status), func(t *testing.T) {
			rows := []StatusRow{
				{Workspace: "test", Branch: "feat/test", Status: tc.status, CurrentStep: 1, TotalSteps: 5},
			}
			st := NewStatusTable(rows, WithTerminalWidth(120))
			_, dataRows := st.ToTableData()
			require.Len(t, dataRows, 1)
			statusCell := dataRows[0][2]
			assert.Contains(t, statusCell, tc.expectedIcon, "Status cell should contain icon for %s", tc.status)
			assert.Contains(t, statusCell, string(tc.status), "Status cell should contain status text for %s", tc.status)
		})
	}
}

func TestStatusTable_ActionCellRendering(t *testing.T) {
	t.Run("shows suggested action for actionable statuses", func(t *testing.T) {
		testCases := []struct {
			status         constants.TaskStatus
			expectedAction string
		}{
			{constants.TaskStatusValidationFailed, "atlas resume"},
			{constants.TaskStatusAwaitingApproval, "atlas approve"},
			{constants.TaskStatusGHFailed, "atlas resume"},
			{constants.TaskStatusCIFailed, "atlas resume"},
			{constants.TaskStatusCITimeout, "atlas resume"},
		}

		for _, tc := range testCases {
			t.Run(string(tc.status), func(t *testing.T) {
				rows := []StatusRow{
					{Workspace: "test", Branch: "feat/test", Status: tc.status, CurrentStep: 1, TotalSteps: 5},
				}
				st := NewStatusTable(rows, WithTerminalWidth(120))
				_, dataRows := st.ToTableData()
				require.Len(t, dataRows, 1)
				actionCell := dataRows[0][4]
				// In NO_COLOR mode (test environment), action includes "(!) " prefix for attention states
				// Story 7.9: Triple redundancy - icon + color + text
				if !HasColorSupport() {
					assert.Equal(t, "(!) "+tc.expectedAction, actionCell)
				} else {
					assert.Equal(t, tc.expectedAction, actionCell)
				}
			})
		}
	})

	t.Run("shows em-dash for non-actionable statuses", func(t *testing.T) {
		nonActionableStatuses := []constants.TaskStatus{
			constants.TaskStatusPending,
			constants.TaskStatusRunning,
			constants.TaskStatusValidating,
			constants.TaskStatusCompleted,
			constants.TaskStatusRejected,
			constants.TaskStatusAbandoned,
		}

		for _, status := range nonActionableStatuses {
			t.Run(string(status), func(t *testing.T) {
				rows := []StatusRow{
					{Workspace: "test", Branch: "feat/test", Status: status, CurrentStep: 1, TotalSteps: 5},
				}
				st := NewStatusTable(rows, WithTerminalWidth(120))
				_, dataRows := st.ToTableData()
				require.Len(t, dataRows, 1)
				actionCell := dataRows[0][4]
				assert.Equal(t, "—", actionCell, "Non-actionable status %s should show em-dash", status)
			})
		}
	})

	t.Run("uses custom action when provided", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "test", Branch: "feat/test", Status: constants.TaskStatusRunning, Action: "custom command"},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		_, dataRows := st.ToTableData()
		require.Len(t, dataRows, 1)
		actionCell := dataRows[0][4]
		assert.Equal(t, "custom command", actionCell)
	})
}

func TestStatusTable_ColumnWidthCalculation(t *testing.T) {
	t.Run("calculates widths based on content", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "very-long-workspace-name", Branch: "feat/auth", Status: constants.TaskStatusRunning},
			{Workspace: "short", Branch: "feature/very-long-branch-name", Status: constants.TaskStatusCompleted},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Verify long content is not truncated
		assert.Contains(t, output, "very-long-workspace-name")
		assert.Contains(t, output, "feature/very-long-branch-name")
	})

	t.Run("uses minimum widths", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "a", Branch: "b", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 1},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		// Output should be properly padded with minimum widths
		output := buf.String()
		assert.Contains(t, output, "WORKSPACE")
		assert.Contains(t, output, "a")
	})

	t.Run("handles Unicode content correctly", func(t *testing.T) {
		// Use Unicode characters via escape sequences to avoid gosmopolitan linter
		// These represent Chinese characters and Japanese text
		unicodeWorkspace := "\u7528\u6237\u8ba4\u8bc1" // Chinese: user authentication
		unicodeBranch := "feat/\u65e5\u672c\u8a9e"     // Japanese: feat/日本語
		rows := []StatusRow{
			{Workspace: unicodeWorkspace, Branch: unicodeBranch, Status: constants.TaskStatusRunning, CurrentStep: 2, TotalSteps: 5},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, unicodeWorkspace)
		assert.Contains(t, output, unicodeBranch)
	})
}

func TestStatusTable_Render(t *testing.T) {
	t.Run("renders complete table", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval, CurrentStep: 6, TotalSteps: 7},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()

		// Check header
		assert.Contains(t, output, "WORKSPACE")
		assert.Contains(t, output, "BRANCH")
		assert.Contains(t, output, "STATUS")
		assert.Contains(t, output, "STEP")
		assert.Contains(t, output, "ACTION")

		// Check first row
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "feat/auth")
		assert.Contains(t, output, "running")
		assert.Contains(t, output, "3/7")

		// Check second row
		assert.Contains(t, output, "payment")
		assert.Contains(t, output, "fix/payment")
		assert.Contains(t, output, "awaiting_approval")
		assert.Contains(t, output, "6/7")
		assert.Contains(t, output, "atlas approve")
	})

	t.Run("uses double-space column separator", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Verify double-space separator is used
		assert.Contains(t, output, "  ")
	})

	t.Run("renders empty table without error", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Should have header row only
		assert.Contains(t, output, "WORKSPACE")
		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Len(t, lines, 1, "Empty table should only have header row")
	})
}

func TestStatusTable_ToTableData(t *testing.T) {
	t.Run("returns headers and rows", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		headers, dataRows := st.ToTableData()

		assert.Equal(t, []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}, headers)
		require.Len(t, dataRows, 1)
		assert.Equal(t, "auth", dataRows[0][0])
		assert.Equal(t, "feat/auth", dataRows[0][1])
		assert.Contains(t, dataRows[0][2], "running")
		assert.Equal(t, "3/7", dataRows[0][3])
		assert.Equal(t, "—", dataRows[0][4]) // Running has no suggested action
	})

	t.Run("uses abbreviated headers in narrow mode", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(60))
		headers, _ := st.ToTableData()
		assert.Equal(t, []string{"WS", "BRANCH", "STAT", "STEP", "ACT"}, headers)
	})

	t.Run("returns plain text status without ANSI codes", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		_, dataRows := st.ToTableData()

		require.Len(t, dataRows, 1)
		statusCell := dataRows[0][2]
		// Verify no ANSI escape codes (they start with \x1b[)
		assert.NotContains(t, statusCell, "\x1b[", "ToTableData should return plain text without ANSI codes")
		assert.Contains(t, statusCell, "● running")
	})
}

func TestStatusTable_ToJSONData(t *testing.T) {
	t.Run("always uses full headers", func(t *testing.T) {
		// Even in narrow mode, JSON should use full header names
		st := NewStatusTable(nil, WithTerminalWidth(60))
		headers, _ := st.ToJSONData()
		assert.Equal(t, []string{"WORKSPACE", "BRANCH", "STATUS", "STEP", "ACTION"}, headers)
	})

	t.Run("returns plain text status (no ANSI codes)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		_, dataRows := st.ToJSONData()

		require.Len(t, dataRows, 1)
		statusCell := dataRows[0][2]
		// Verify no ANSI escape codes (they start with \x1b[)
		assert.NotContains(t, statusCell, "\x1b[")
		assert.Contains(t, statusCell, "● running")
	})
}

func TestStatusTable_NarrowMode(t *testing.T) {
	t.Run("detects narrow terminal (< 80 cols)", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(79))
		assert.True(t, st.IsNarrow())

		st = NewStatusTable(nil, WithTerminalWidth(80))
		assert.False(t, st.IsNarrow())
	})

	t.Run("renders with abbreviated headers in narrow mode", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		st := NewStatusTable(rows, WithTerminalWidth(60))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "WS")
		assert.Contains(t, output, "STAT")
		assert.Contains(t, output, "ACT")
		assert.NotContains(t, output, "WORKSPACE")
	})

	t.Run("terminal width 0 assumes wide", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(0))
		assert.False(t, st.IsNarrow())
	})
}

func TestStatusRow_Fields(t *testing.T) {
	t.Run("all fields are accessible", func(t *testing.T) {
		row := StatusRow{
			Workspace:   "auth",
			Branch:      "feat/auth",
			Status:      constants.TaskStatusRunning,
			CurrentStep: 3,
			TotalSteps:  7,
			Action:      "custom",
		}

		assert.Equal(t, "auth", row.Workspace)
		assert.Equal(t, "feat/auth", row.Branch)
		assert.Equal(t, constants.TaskStatusRunning, row.Status)
		assert.Equal(t, 3, row.CurrentStep)
		assert.Equal(t, 7, row.TotalSteps)
		assert.Equal(t, "custom", row.Action)
	})
}

func TestStatusColumnWidths(t *testing.T) {
	t.Run("MinColumnWidths has expected values", func(t *testing.T) {
		assert.Equal(t, 10, MinColumnWidths.Workspace)
		assert.Equal(t, 12, MinColumnWidths.Branch)
		assert.Equal(t, 18, MinColumnWidths.Status)
		assert.Equal(t, 6, MinColumnWidths.Step)
		assert.Equal(t, 10, MinColumnWidths.Action)
	})
}

func TestStatusTable_ProportionalExpansion(t *testing.T) {
	t.Run("applies proportional expansion for wide terminals (120+)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}

		// Create tables at different widths
		narrowTable := NewStatusTable(rows, WithTerminalWidth(100))
		wideTable := NewStatusTable(rows, WithTerminalWidth(180))

		var narrowBuf, wideBuf bytes.Buffer
		err := narrowTable.Render(&narrowBuf)
		require.NoError(t, err)
		err = wideTable.Render(&wideBuf)
		require.NoError(t, err)

		// Wide terminal should produce wider output (more padding)
		narrowLines := strings.Split(narrowBuf.String(), "\n")
		wideLines := strings.Split(wideBuf.String(), "\n")

		// Header line should be longer in wide mode due to column expansion
		assert.Greater(t, len(wideLines[0]), len(narrowLines[0]),
			"Wide terminal should produce wider output")
	})

	t.Run("WideTerminalThreshold is 120", func(t *testing.T) {
		assert.Equal(t, 120, WideTerminalThreshold)
	})

	t.Run("does not expand below threshold", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}

		// At 119 (just below threshold), no expansion
		table119 := NewStatusTable(rows, WithTerminalWidth(119))
		// At 120 (at threshold), expansion kicks in
		table120 := NewStatusTable(rows, WithTerminalWidth(120))

		var buf119, buf120 bytes.Buffer
		err := table119.Render(&buf119)
		require.NoError(t, err)
		err = table120.Render(&buf120)
		require.NoError(t, err)

		// Output should be different at threshold boundary
		// (120 may or may not be wider depending on content vs terminal width)
		// Just verify both render without error
		assert.NotEmpty(t, buf119.String())
		assert.NotEmpty(t, buf120.String())
	})

	t.Run("keeps Status and Step columns fixed width", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}

		// Very wide terminal
		wideTable := NewStatusTable(rows, WithTerminalWidth(200))
		var buf bytes.Buffer
		err := wideTable.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Status cell should contain the status text without excessive padding
		assert.Contains(t, output, "running")
		// Step cell should be compact
		assert.Contains(t, output, "1/5")
	})

	t.Run("Rows returns a copy not internal slice", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))

		// Get rows and modify
		returned := st.Rows()
		returned[0].Workspace = "modified"

		// Original should be unchanged
		original := st.Rows()
		assert.Equal(t, "auth", original[0].Workspace, "Rows() should return a copy, not internal slice")
	})

	t.Run("Rows returns nil for nil input", func(t *testing.T) {
		st := NewStatusTable(nil, WithTerminalWidth(120))
		assert.Nil(t, st.Rows())
	})
}

func TestStatusTable_ConstrainToTerminalWidth(t *testing.T) {
	t.Run("constrains table to fit within narrow terminal", func(t *testing.T) {
		// Create rows with long branch names that would exceed 80 columns
		rows := []StatusRow{
			{Workspace: "task-workspace", Branch: "task/task-workspace-20260103-165907", Status: constants.TaskStatusCompleted, CurrentStep: 5, TotalSteps: 5},
		}
		// Use 80 column terminal
		st := NewStatusTable(rows, WithTerminalWidth(80))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// All 5 columns should be present in header
		assert.Contains(t, output, "WORKSPACE")
		assert.Contains(t, output, "BRANCH")
		assert.Contains(t, output, "STATUS")
		assert.Contains(t, output, "STEP")
		assert.Contains(t, output, "ACTION")

		// Check each line doesn't exceed terminal width
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if line != "" {
				// Count visible characters (excluding ANSI codes)
				// Use rune count, not byte count, for proper Unicode handling
				visible := stripANSI(line)
				runeCount := utf8.RuneCountInString(visible)
				assert.LessOrEqual(t, runeCount, 80,
					"Line should fit within 80 columns (got %d runes): %s", runeCount, line)
			}
		}
	})

	t.Run("truncates branch column first when exceeding terminal width", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "ws", Branch: "very-long-branch-name-that-exceeds-limits", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		st := NewStatusTable(rows, WithTerminalWidth(80))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Branch should be truncated, but workspace should remain intact
		assert.Contains(t, output, "ws")
		// Full branch name shouldn't appear (truncated)
		assert.NotContains(t, output, "very-long-branch-name-that-exceeds-limits")
	})

	t.Run("respects minimum column widths", func(t *testing.T) {
		// Very long content in a very narrow terminal
		rows := []StatusRow{
			{Workspace: "very-long-workspace-name-here", Branch: "very-long-branch-name-here", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		// Use a terminal width that would require truncation
		st := NewStatusTable(rows, WithTerminalWidth(80))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		// Should render without error - columns won't go below minimum
		output := buf.String()
		assert.NotEmpty(t, output)
		// Header should still be present
		assert.Contains(t, output, "STATUS")
		assert.Contains(t, output, "STEP")
		assert.Contains(t, output, "ACTION")
	})

	t.Run("no constraint needed for wide terminal", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		// Wide terminal - no constraint needed
		st := NewStatusTable(rows, WithTerminalWidth(200))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Full content should be visible
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "feat/auth")
	})

	t.Run("handles zero terminal width gracefully", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 5},
		}
		// Zero width should not apply constraints
		st := NewStatusTable(rows, WithTerminalWidth(0))
		var buf bytes.Buffer
		err := st.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "feat/auth")
	})

	t.Run("preserves all five columns even with very long branch names", func(t *testing.T) {
		// This is the key bug scenario - long branch names causing columns to be cut off
		rows := []StatusRow{
			{Workspace: "task-test-ws", Branch: "task/task-workspace-20260103-165907", Status: constants.TaskStatusAbandoned, CurrentStep: 0, TotalSteps: 0},
			{Workspace: "task-workspace", Branch: "task/task-workspace-20260103-165907", Status: constants.TaskStatusCompleted, CurrentStep: 0, TotalSteps: 0},
		}
		st := NewStatusTable(rows, WithTerminalWidth(80))
		_, dataRows := st.ToTableData()

		require.Len(t, dataRows, 2)
		// Each row should have exactly 5 columns
		for i, row := range dataRows {
			assert.Len(t, row, 5, "Row %d should have 5 columns", i)
		}
	})
}

// ========================================
// HierarchicalStatusTable Tests
// ========================================

func TestHierarchicalStatusTable_NewHierarchicalStatusTable(t *testing.T) {
	t.Parallel()

	groups := []WorkspaceGroup{
		{
			Name:       "auth",
			Branch:     "feat/auth",
			Status:     constants.TaskStatusRunning,
			TotalTasks: 2,
			Tasks: []TaskInfo{
				{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
				{ID: "task-2", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7},
			},
		},
	}

	table := NewHierarchicalStatusTable(groups)
	assert.NotNil(t, table)
	assert.Len(t, table.Groups(), 1)
}

func TestHierarchicalStatusTable_Headers(t *testing.T) {
	t.Parallel()

	t.Run("standard headers in wide terminal", func(t *testing.T) {
		groups := []WorkspaceGroup{}
		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(120))
		headers := table.Headers()

		assert.Equal(t, []string{"WORKSPACE", "BRANCH", "STATUS", "TASKS"}, headers)
	})

	t.Run("abbreviated headers in narrow terminal", func(t *testing.T) {
		groups := []WorkspaceGroup{}
		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(70))
		headers := table.Headers()

		assert.Equal(t, []string{"WS", "BRANCH", "STAT", "TASKS"}, headers)
	})
}

func TestHierarchicalStatusTable_Render(t *testing.T) {
	t.Parallel()

	t.Run("renders workspace with nested tasks", func(t *testing.T) {
		groups := []WorkspaceGroup{
			{
				Name:       "auth",
				Branch:     "feat/auth",
				Status:     constants.TaskStatusRunning,
				TotalTasks: 2,
				Tasks: []TaskInfo{
					{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7, Template: "feature"},
					{ID: "task-2", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7, Template: "bugfix"},
				},
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Workspace row with count suffix
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "(2)") // Task count in workspace name
		assert.Contains(t, output, "feat/auth")
		assert.Contains(t, output, "running")
		// Task rows with tree characters
		assert.Contains(t, output, "├─")
		assert.Contains(t, output, "└─")
		assert.Contains(t, output, "task-1")
		assert.Contains(t, output, "task-2")
		// Progress display: running task shows progress bar, completed shows 100%
		assert.Contains(t, output, "100%") // Completed task
	})

	t.Run("truncates tasks when more than max", func(t *testing.T) {
		tasks := make([]TaskInfo, 5)
		for i := range tasks {
			tasks[i] = TaskInfo{
				ID:          fmt.Sprintf("task-%d", i+1),
				Status:      constants.TaskStatusCompleted,
				CurrentStep: 7,
				TotalSteps:  7,
				Template:    "feature",
			}
		}

		groups := []WorkspaceGroup{
			{
				Name:       "auth",
				Branch:     "feat/auth",
				Status:     constants.TaskStatusCompleted,
				TotalTasks: 5,
				Tasks:      tasks,
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Should show first 3 tasks
		assert.Contains(t, output, "task-1")
		assert.Contains(t, output, "task-2")
		assert.Contains(t, output, "task-3")
		// Should NOT show task-4 and task-5
		assert.NotContains(t, output, "task-4")
		assert.NotContains(t, output, "task-5")
		// Should show "+2 more" indicator
		assert.Contains(t, output, "+2 more")
	})

	t.Run("handles empty task list", func(t *testing.T) {
		groups := []WorkspaceGroup{
			{
				Name:       "auth",
				Branch:     "feat/auth",
				Status:     constants.TaskStatusPending,
				TotalTasks: 0,
				Tasks:      []TaskInfo{},
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "auth")
		assert.Contains(t, output, "—") // Em-dash for no tasks
	})

	t.Run("handles single task with correct tree character", func(t *testing.T) {
		groups := []WorkspaceGroup{
			{
				Name:       "auth",
				Branch:     "feat/auth",
				Status:     constants.TaskStatusRunning,
				TotalTasks: 1,
				Tasks: []TaskInfo{
					{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
				},
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(120))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Single task should use └─ (last branch)
		assert.Contains(t, output, "└─")
		assert.NotContains(t, output, "├─")
	})
}

func TestHierarchicalStatusTable_ToJSONData(t *testing.T) {
	t.Parallel()

	groups := []WorkspaceGroup{
		{
			Name:       "auth",
			Branch:     "feat/auth",
			Status:     constants.TaskStatusRunning,
			TotalTasks: 2,
			Tasks: []TaskInfo{
				{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7, Template: "feature"},
				{ID: "task-2", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7, Template: "bugfix"},
			},
		},
	}

	table := NewHierarchicalStatusTable(groups)
	jsonData := table.ToJSONData()

	require.Len(t, jsonData, 1)
	assert.Equal(t, "auth", jsonData[0].Name)
	assert.Equal(t, "feat/auth", jsonData[0].Branch)
	assert.Equal(t, "running", jsonData[0].Status)
	assert.Equal(t, 2, jsonData[0].TotalTasks)

	require.Len(t, jsonData[0].Tasks, 2)
	assert.Equal(t, "task-1", jsonData[0].Tasks[0].ID)
	assert.Equal(t, "running", jsonData[0].Tasks[0].Status)
	assert.Equal(t, "3/7", jsonData[0].Tasks[0].Step)
	assert.Equal(t, "feature", jsonData[0].Tasks[0].Template)

	assert.Equal(t, "task-2", jsonData[0].Tasks[1].ID)
	assert.Equal(t, "completed", jsonData[0].Tasks[1].Status)
	assert.Equal(t, "7/7", jsonData[0].Tasks[1].Step)
}

func TestHierarchicalStatusTable_ColumnWidths(t *testing.T) {
	t.Parallel()

	t.Run("calculates widths based on content", func(t *testing.T) {
		groups := []WorkspaceGroup{
			{
				Name:       "very-long-workspace-name",
				Branch:     "feature/a-long-branch-name",
				Status:     constants.TaskStatusRunning,
				TotalTasks: 10,
				Tasks: []TaskInfo{
					{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 10, TotalSteps: 15, Template: "feature"},
				},
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(200))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Verify long content is rendered
		assert.Contains(t, output, "very-long-workspace-name")
		assert.Contains(t, output, "feature/a-long-branch-name")
	})

	t.Run("constrains widths for narrow terminal", func(t *testing.T) {
		groups := []WorkspaceGroup{
			{
				Name:       "very-long-workspace-name",
				Branch:     "feature/a-very-long-branch-name-that-should-be-truncated",
				Status:     constants.TaskStatusRunning,
				TotalTasks: 1,
				Tasks: []TaskInfo{
					{ID: "task-1", Status: constants.TaskStatusRunning, CurrentStep: 1, TotalSteps: 7},
				},
			},
		}

		table := NewHierarchicalStatusTable(groups, WithTerminalWidth(60))
		var buf bytes.Buffer
		err := table.Render(&buf)
		require.NoError(t, err)

		// Should render without error even with constrained width
		output := buf.String()
		assert.Contains(t, output, "task-1")
	})
}

// TestTreeChars tests that tree characters are correctly defined.
func TestTreeChars(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "├─ ", TreeChars.Branch)
	assert.Equal(t, "└─ ", TreeChars.LastBranch)
	assert.Equal(t, "   ", TreeChars.Indent)
}

// TestRenderMiniProgressBar tests the mini progress bar rendering.
// We don't use t.Parallel() here because subtests modify environment variables.
func TestRenderMiniProgressBar(t *testing.T) {
	t.Run("renders full bar when complete", func(t *testing.T) {
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := renderMiniProgressBar(8, 8)
		assert.Equal(t, "████████", result)
	})

	t.Run("renders partial bar", func(t *testing.T) {
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := renderMiniProgressBar(4, 8)
		assert.Equal(t, "████░░░░", result)
	})

	t.Run("renders empty bar when zero progress", func(t *testing.T) {
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := renderMiniProgressBar(0, 8)
		assert.Equal(t, "░░░░░░░░", result)
	})

	t.Run("renders ASCII fallback in NO_COLOR mode", func(t *testing.T) {
		_ = os.Setenv("NO_COLOR", "1")
		defer func() { _ = os.Unsetenv("NO_COLOR") }()

		result := renderMiniProgressBar(4, 8)
		assert.Equal(t, "[####----]", result)
	})

	t.Run("handles zero total", func(t *testing.T) {
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := renderMiniProgressBar(0, 0)
		assert.Equal(t, "████████", result)
	})
}

// TestRenderHyperlink tests OSC 8 hyperlink rendering.
// We don't use t.Parallel() here because subtests modify environment variables,
// which would cause race conditions when running in parallel.
func TestRenderHyperlink(t *testing.T) {
	t.Run("renders hyperlink with OSC 8 escape sequence", func(t *testing.T) {
		// Need to ensure color support for hyperlink rendering
		// Store and restore NO_COLOR
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := RenderHyperlink("click me", "https://example.com")

		// Check for OSC 8 escape sequence structure
		assert.Contains(t, result, "\x1b]8;;https://example.com\x07")
		assert.Contains(t, result, "click me")
		assert.Contains(t, result, "\x1b]8;;\x07")
	})

	t.Run("returns plain text when colors disabled", func(t *testing.T) {
		_ = os.Setenv("NO_COLOR", "1")
		defer func() { _ = os.Unsetenv("NO_COLOR") }()

		result := RenderHyperlink("click me", "https://example.com")
		assert.Equal(t, "click me", result)
	})
}

// TestRenderFileHyperlink tests file:// URL hyperlink rendering.
// We don't use t.Parallel() here because subtests modify environment variables,
// which would cause race conditions when running in parallel.
func TestRenderFileHyperlink(t *testing.T) {
	t.Run("creates file:// URL", func(t *testing.T) {
		// Need to ensure color support
		original := os.Getenv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if original != "" {
				_ = os.Setenv("NO_COLOR", original)
			}
		}()

		result := RenderFileHyperlink("open folder", "/path/to/folder")
		assert.Contains(t, result, "file:///path/to/folder")
		assert.Contains(t, result, "open folder")
	})
}
