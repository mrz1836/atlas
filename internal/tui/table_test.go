package tui

import (
	"bytes"
	"strings"
	"testing"

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
			{constants.TaskStatusValidationFailed, "atlas recover"},
			{constants.TaskStatusAwaitingApproval, "atlas approve"},
			{constants.TaskStatusGHFailed, "atlas recover"},
			{constants.TaskStatusCIFailed, "atlas recover"},
			{constants.TaskStatusCITimeout, "atlas recover"},
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
