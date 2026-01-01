package tui

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
)

// ========================================
// StatusFooter Tests (Story 7.9)
// ========================================

func TestStatusFooter_NewStatusFooter(t *testing.T) {
	t.Run("creates footer with attention items only", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval, CurrentStep: 6, TotalSteps: 7},
			{Workspace: "api", Branch: "feat/api", Status: constants.TaskStatusCIFailed, CurrentStep: 4, TotalSteps: 7},
			{Workspace: "done", Branch: "fix/done", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7},
		}

		footer := NewStatusFooter(rows)
		require.NotNil(t, footer)
		assert.True(t, footer.HasItems())
		items := footer.Items()
		assert.Len(t, items, 2) // Only awaiting_approval and ci_failed

		// Verify the attention items are correct
		assert.Equal(t, "payment", items[0].Workspace)
		assert.Equal(t, "atlas approve payment", items[0].Action)

		assert.Equal(t, "api", items[1].Workspace)
		assert.Equal(t, "atlas recover api", items[1].Action)
	})

	t.Run("creates empty footer when no attention items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning, CurrentStep: 3, TotalSteps: 7},
			{Workspace: "done", Branch: "fix/done", Status: constants.TaskStatusCompleted, CurrentStep: 7, TotalSteps: 7},
		}

		footer := NewStatusFooter(rows)
		require.NotNil(t, footer)
		assert.False(t, footer.HasItems())
		assert.Empty(t, footer.Items())
	})

	t.Run("handles nil rows", func(t *testing.T) {
		footer := NewStatusFooter(nil)
		require.NotNil(t, footer)
		assert.False(t, footer.HasItems())
	})

	t.Run("handles empty rows", func(t *testing.T) {
		footer := NewStatusFooter([]StatusRow{})
		require.NotNil(t, footer)
		assert.False(t, footer.HasItems())
	})
}

func TestStatusFooter_HasItems(t *testing.T) {
	t.Run("returns true when has attention items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		footer := NewStatusFooter(rows)
		assert.True(t, footer.HasItems())
	})

	t.Run("returns false when no attention items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		footer := NewStatusFooter(rows)
		assert.False(t, footer.HasItems())
	})
}

func TestStatusFooter_Items(t *testing.T) {
	t.Run("returns copy of items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		footer := NewStatusFooter(rows)

		// Get items and modify
		items := footer.Items()
		items[0].Workspace = "modified"

		// Original should be unchanged
		originalItems := footer.Items()
		assert.Equal(t, "payment", originalItems[0].Workspace)
	})

	t.Run("returns nil when no items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		footer := NewStatusFooter(rows)
		assert.Nil(t, footer.Items())
	})
}

func TestStatusFooter_Render(t *testing.T) {
	t.Run("renders nothing when no attention items (AC: no footer displayed)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.Render(&buf)
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})

	t.Run("renders single attention item (AC: shows Run: atlas approve workspace)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Run:")
		assert.Contains(t, output, "atlas approve payment")
	})

	t.Run("renders multiple attention items (AC: lists all commands)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusAwaitingApproval},
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusCIFailed},
			{Workspace: "api", Branch: "feat/api", Status: constants.TaskStatusValidationFailed},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Run: ")
		assert.Contains(t, output, "atlas approve auth")
		assert.Contains(t, output, "atlas recover payment")
		assert.Contains(t, output, "atlas recover api")
	})

	t.Run("includes blank line separator before footer", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.Render(&buf)
		require.NoError(t, err)

		output := buf.String()
		// Should start with a blank line
		assert.True(t, len(output) > 0 && output[0] == '\n')
	})
}

func TestStatusFooter_RenderPlain(t *testing.T) {
	t.Run("renders without styling", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.RenderPlain(&buf)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Run: atlas approve payment")
		// Should not contain ANSI codes
		assert.NotContains(t, output, "\x1b[")
	})

	t.Run("renders nothing when no items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		footer := NewStatusFooter(rows)

		var buf bytes.Buffer
		err := footer.RenderPlain(&buf)
		require.NoError(t, err)
		assert.Empty(t, buf.String())
	})
}

func TestStatusFooter_ToJSON(t *testing.T) {
	t.Run("returns attention items in JSON format (AC: --output json includes action commands)", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
			{Workspace: "api", Branch: "feat/api", Status: constants.TaskStatusCIFailed},
		}
		footer := NewStatusFooter(rows)

		jsonItems := footer.ToJSON()
		require.Len(t, jsonItems, 2)

		assert.Equal(t, "payment", jsonItems[0]["workspace"])
		assert.Equal(t, "atlas approve payment", jsonItems[0]["action"])

		assert.Equal(t, "api", jsonItems[1]["workspace"])
		assert.Equal(t, "atlas recover api", jsonItems[1]["action"])
	})

	t.Run("returns nil when no items", func(t *testing.T) {
		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		footer := NewStatusFooter(rows)
		assert.Nil(t, footer.ToJSON())
	})
}

func TestFormatSingleAction(t *testing.T) {
	t.Run("formats action with workspace", func(t *testing.T) {
		result := FormatSingleAction("payment", "atlas approve")
		assert.Equal(t, "Run: atlas approve payment", result)
	})
}

func TestFormatMultipleActions(t *testing.T) {
	t.Run("formats multiple actions on separate lines", func(t *testing.T) {
		items := []ActionItem{
			{Workspace: "auth", Action: "atlas approve auth"},
			{Workspace: "payment", Action: "atlas recover payment"},
		}
		result := FormatMultipleActions(items)
		assert.Contains(t, result, "Run: atlas approve auth")
		assert.Contains(t, result, "Run: atlas recover payment")
		assert.Contains(t, result, "\n") // Should have newline separator
	})

	t.Run("handles empty items", func(t *testing.T) {
		result := FormatMultipleActions([]ActionItem{})
		assert.Empty(t, result)
	})
}

func TestActionItem(t *testing.T) {
	t.Run("struct fields are accessible", func(t *testing.T) {
		item := ActionItem{
			Workspace: "payment",
			Action:    "atlas approve payment",
			Status:    constants.TaskStatusAwaitingApproval,
		}
		assert.Equal(t, "payment", item.Workspace)
		assert.Equal(t, "atlas approve payment", item.Action)
		assert.Equal(t, constants.TaskStatusAwaitingApproval, item.Status)
	})
}

// ========================================
// ActionStyle Tests (Story 7.9, AC: #4)
// ========================================

func TestActionStyle(t *testing.T) {
	t.Run("returns warning style when colors enabled", func(t *testing.T) {
		// Ensure NO_COLOR is not set
		originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
		_ = os.Unsetenv("NO_COLOR")
		defer func() {
			if hadNoColor {
				_ = os.Setenv("NO_COLOR", originalNoColor)
			}
		}()

		style := ActionStyle()
		// The style should be able to render text (not nil)
		rendered := style.Render("test")
		assert.NotEmpty(t, rendered)
	})

	t.Run("returns plain style when NO_COLOR set", func(t *testing.T) {
		// Set NO_COLOR
		originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
		_ = os.Setenv("NO_COLOR", "1")
		defer func() {
			if hadNoColor {
				_ = os.Setenv("NO_COLOR", originalNoColor)
			} else {
				_ = os.Unsetenv("NO_COLOR")
			}
		}()

		style := ActionStyle()
		rendered := style.Render("test")
		// In NO_COLOR mode, should render without ANSI codes
		assert.Equal(t, "test", rendered)
	})
}

// ========================================
// Action Column NO_COLOR Tests (Story 7.9)
// ========================================

func TestActionCellNoColor(t *testing.T) {
	t.Run("includes (!) prefix for attention states in NO_COLOR mode", func(t *testing.T) {
		// Set NO_COLOR
		originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
		_ = os.Setenv("NO_COLOR", "1")
		defer func() {
			if hadNoColor {
				_ = os.Setenv("NO_COLOR", originalNoColor)
			} else {
				_ = os.Unsetenv("NO_COLOR")
			}
		}()

		rows := []StatusRow{
			{Workspace: "payment", Branch: "fix/payment", Status: constants.TaskStatusAwaitingApproval},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		_, dataRows := st.ToTableData()

		require.Len(t, dataRows, 1)
		actionCell := dataRows[0][4]
		assert.Contains(t, actionCell, "(!) ")
		assert.Contains(t, actionCell, "atlas approve")
	})

	t.Run("no prefix for non-attention states in NO_COLOR mode", func(t *testing.T) {
		// Set NO_COLOR
		originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
		_ = os.Setenv("NO_COLOR", "1")
		defer func() {
			if hadNoColor {
				_ = os.Setenv("NO_COLOR", originalNoColor)
			} else {
				_ = os.Unsetenv("NO_COLOR")
			}
		}()

		rows := []StatusRow{
			{Workspace: "auth", Branch: "feat/auth", Status: constants.TaskStatusRunning},
		}
		st := NewStatusTable(rows, WithTerminalWidth(120))
		_, dataRows := st.ToTableData()

		require.Len(t, dataRows, 1)
		actionCell := dataRows[0][4]
		// Running has no action, should show em-dash without prefix
		assert.Equal(t, "—", actionCell)
	})
}

// ========================================
// All Attention States Coverage (Story 7.9)
// ========================================

func TestStatusFooter_AllAttentionStates(t *testing.T) {
	attentionStatuses := []struct {
		status         constants.TaskStatus
		expectedAction string
	}{
		{constants.TaskStatusValidationFailed, "atlas recover"},
		{constants.TaskStatusAwaitingApproval, "atlas approve"},
		{constants.TaskStatusGHFailed, "atlas recover"},
		{constants.TaskStatusCIFailed, "atlas recover"},
		{constants.TaskStatusCITimeout, "atlas recover"},
	}

	for _, tc := range attentionStatuses {
		t.Run(string(tc.status), func(t *testing.T) {
			rows := []StatusRow{
				{Workspace: "test-workspace", Branch: "feat/test", Status: tc.status},
			}
			footer := NewStatusFooter(rows)

			assert.True(t, footer.HasItems(), "Status %s should be attention status", tc.status)
			items := footer.Items()
			require.Len(t, items, 1)
			assert.Equal(t, "test-workspace", items[0].Workspace)
			assert.Equal(t, tc.expectedAction+" test-workspace", items[0].Action)
		})
	}
}

func TestStatusFooter_NonAttentionStates(t *testing.T) {
	nonAttentionStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range nonAttentionStatuses {
		t.Run(string(status), func(t *testing.T) {
			rows := []StatusRow{
				{Workspace: "test-workspace", Branch: "feat/test", Status: status},
			}
			footer := NewStatusFooter(rows)

			assert.False(t, footer.HasItems(), "Status %s should not be attention status", status)
		})
	}
}

// ========================================
// Em-Dash Display Tests (Story 7.9)
// ========================================

func TestEmDashDisplay(t *testing.T) {
	t.Run("shows em-dash for non-actionable states (AC: em-dash for running/pending/completed)", func(t *testing.T) {
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
					{Workspace: "test", Branch: "feat/test", Status: status},
				}
				st := NewStatusTable(rows, WithTerminalWidth(120))
				_, dataRows := st.ToTableData()

				require.Len(t, dataRows, 1)
				actionCell := dataRows[0][4]
				assert.Equal(t, "—", actionCell, "Status %s should show em-dash", status)
			})
		}
	})
}
