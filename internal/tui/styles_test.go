package tui

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
)

// TestSemanticColors_AllColorsExported verifies that all 5 semantic colors
// are exported as constants per UX-4 specification (AC: #1).
func TestSemanticColors_AllColorsExported(t *testing.T) {
	// Verify Primary (Blue) is exported
	assert.NotEmpty(t, ColorPrimary.Light, "ColorPrimary.Light should be defined")
	assert.NotEmpty(t, ColorPrimary.Dark, "ColorPrimary.Dark should be defined")
	assert.Equal(t, "#0087AF", ColorPrimary.Light)
	assert.Equal(t, "#00D7FF", ColorPrimary.Dark)

	// Verify Success (Green) is exported
	assert.NotEmpty(t, ColorSuccess.Light, "ColorSuccess.Light should be defined")
	assert.NotEmpty(t, ColorSuccess.Dark, "ColorSuccess.Dark should be defined")
	assert.Equal(t, "#008700", ColorSuccess.Light)
	assert.Equal(t, "#00FF87", ColorSuccess.Dark)

	// Verify Warning (Yellow) is exported
	assert.NotEmpty(t, ColorWarning.Light, "ColorWarning.Light should be defined")
	assert.NotEmpty(t, ColorWarning.Dark, "ColorWarning.Dark should be defined")
	assert.Equal(t, "#AF8700", ColorWarning.Light)
	assert.Equal(t, "#FFD700", ColorWarning.Dark)

	// Verify Error (Red) is exported
	assert.NotEmpty(t, ColorError.Light, "ColorError.Light should be defined")
	assert.NotEmpty(t, ColorError.Dark, "ColorError.Dark should be defined")
	assert.Equal(t, "#AF0000", ColorError.Light)
	assert.Equal(t, "#FF5F5F", ColorError.Dark)

	// Verify Muted (Gray) is exported
	assert.NotEmpty(t, ColorMuted.Light, "ColorMuted.Light should be defined")
	assert.NotEmpty(t, ColorMuted.Dark, "ColorMuted.Dark should be defined")
	assert.Equal(t, "#585858", ColorMuted.Light)
	assert.Equal(t, "#6C6C6C", ColorMuted.Dark)
}

func TestStatusColors(t *testing.T) {
	colors := StatusColors()

	// Verify all workspace statuses have colors defined
	statuses := []constants.WorkspaceStatus{
		constants.WorkspaceStatusActive,
		constants.WorkspaceStatusPaused,
		constants.WorkspaceStatusRetired,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}

func TestNewTableStyles(t *testing.T) {
	styles := NewTableStyles()
	assert.NotNil(t, styles)
	assert.NotNil(t, styles.StatusColors)
}

func TestNewOutputStyles(t *testing.T) {
	styles := NewOutputStyles()
	assert.NotNil(t, styles)
}

func TestTaskStatusColors(t *testing.T) {
	colors := TaskStatusColors()

	// Verify all task statuses have colors defined
	statuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}

func TestTaskStatusIcon(t *testing.T) {
	// Icons per epic-7-tui-components-from-scenarios.md Icon Reference
	tests := []struct {
		status       constants.TaskStatus
		expectedIcon string
	}{
		{constants.TaskStatusPending, "○"},          // Empty circle - waiting
		{constants.TaskStatusRunning, "●"},          // Filled circle - active (spec: ● or ⟳)
		{constants.TaskStatusValidating, "⟳"},       // Rotating - in progress
		{constants.TaskStatusValidationFailed, "⚠"}, // Warning - needs attention
		{constants.TaskStatusAwaitingApproval, "✓"}, // Checkmark - ready for user (spec: ✓ or ⚠)
		{constants.TaskStatusCompleted, "✓"},        // Checkmark - success
		{constants.TaskStatusRejected, "✗"},         // X mark - failed
		{constants.TaskStatusAbandoned, "✗"},        // X mark - failed/abandoned
		{constants.TaskStatusGHFailed, "✗"},         // X mark - failed
		{constants.TaskStatusCIFailed, "✗"},         // X mark - failed
		{constants.TaskStatusCITimeout, "⚠"},        // Warning - needs attention
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			icon := TaskStatusIcon(tc.status)
			assert.Equal(t, tc.expectedIcon, icon)
		})
	}
}

// TestTaskStatusIcon_UnknownStatus returns fallback for unknown status.
func TestTaskStatusIcon_UnknownStatus(t *testing.T) {
	icon := TaskStatusIcon(constants.TaskStatus("unknown"))
	assert.Equal(t, "?", icon)
}

func TestIsAttentionStatus(t *testing.T) {
	attentionStatuses := []constants.TaskStatus{
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	nonAttentionStatuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
	}

	for _, status := range attentionStatuses {
		t.Run(string(status)+"_needs_attention", func(t *testing.T) {
			assert.True(t, IsAttentionStatus(status))
		})
	}

	for _, status := range nonAttentionStatuses {
		t.Run(string(status)+"_no_attention", func(t *testing.T) {
			assert.False(t, IsAttentionStatus(status))
		})
	}
}

func TestSuggestedAction(t *testing.T) {
	tests := []struct {
		status         constants.TaskStatus
		expectedAction string
	}{
		{constants.TaskStatusValidationFailed, "atlas recover"},
		{constants.TaskStatusAwaitingApproval, "atlas approve"},
		{constants.TaskStatusGHFailed, "atlas recover"},
		{constants.TaskStatusCIFailed, "atlas recover"},
		{constants.TaskStatusCITimeout, "atlas recover"},
		{constants.TaskStatusRunning, ""},
		{constants.TaskStatusCompleted, ""},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			action := SuggestedAction(tc.status)
			assert.Equal(t, tc.expectedAction, action)
		})
	}
}

// TestWorkspaceStatusIcon verifies icons are defined for all workspace statuses (AC: #2).
func TestWorkspaceStatusIcon(t *testing.T) {
	tests := []struct {
		status       constants.WorkspaceStatus
		expectedIcon string
	}{
		{constants.WorkspaceStatusActive, "●"},  // Filled circle - active
		{constants.WorkspaceStatusPaused, "○"},  // Empty circle - paused
		{constants.WorkspaceStatusRetired, "◌"}, // Dashed circle - retired
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			icon := WorkspaceStatusIcon(tc.status)
			assert.Equal(t, tc.expectedIcon, icon)
		})
	}
}

// TestWorkspaceStatusIcon_UnknownStatus returns fallback for unknown status.
func TestWorkspaceStatusIcon_UnknownStatus(t *testing.T) {
	icon := WorkspaceStatusIcon(constants.WorkspaceStatus("unknown"))
	assert.Equal(t, "?", icon)
}

// TestFormatStatusWithIcon verifies the triple redundancy pattern (AC: #5).
func TestFormatStatusWithIcon(t *testing.T) {
	// Test with task status (Running now uses ● per spec)
	result := FormatStatusWithIcon(constants.TaskStatusRunning, "Running")
	assert.Contains(t, result, "●")       // Icon (filled circle per spec)
	assert.Contains(t, result, "Running") // Text

	// Test with workspace status
	result = FormatStatusWithIcon(constants.WorkspaceStatusActive, "Active")
	assert.Contains(t, result, "●")      // Icon
	assert.Contains(t, result, "Active") // Text
}

// TestFormatStatusWithIcon_AllTaskStatuses verifies all task statuses format correctly.
func TestFormatStatusWithIcon_AllTaskStatuses(t *testing.T) {
	statuses := []constants.TaskStatus{
		constants.TaskStatusPending,
		constants.TaskStatusRunning,
		constants.TaskStatusValidating,
		constants.TaskStatusValidationFailed,
		constants.TaskStatusAwaitingApproval,
		constants.TaskStatusCompleted,
		constants.TaskStatusRejected,
		constants.TaskStatusAbandoned,
		constants.TaskStatusGHFailed,
		constants.TaskStatusCIFailed,
		constants.TaskStatusCITimeout,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			result := FormatStatusWithIcon(status, string(status))
			assert.NotEmpty(t, result)
			// Should contain both icon and text
			assert.Contains(t, result, string(status))
		})
	}
}

// TestTypographyStyles_AllExported verifies all typography styles are exported (AC: #3).
func TestTypographyStyles_AllExported(t *testing.T) {
	// Verify Bold style exists and works
	boldText := StyleBold.Render("test")
	assert.NotEmpty(t, boldText)

	// Verify Dim style exists and works
	dimText := StyleDim.Render("test")
	assert.NotEmpty(t, dimText)

	// Verify Underline style exists and works
	underlineText := StyleUnderline.Render("test")
	assert.NotEmpty(t, underlineText)

	// Verify Reverse style exists and works
	reverseText := StyleReverse.Render("test")
	assert.NotEmpty(t, reverseText)
}

// TestHasColorSupport verifies color support detection (AC: #4).
func TestHasColorSupport(t *testing.T) {
	// Save original env vars
	origNoColor := os.Getenv("NO_COLOR")
	origTerm := os.Getenv("TERM")
	defer func() {
		_ = os.Setenv("NO_COLOR", origNoColor)
		_ = os.Setenv("TERM", origTerm)
	}()

	t.Run("has color when NO_COLOR is unset", func(t *testing.T) {
		_ = os.Unsetenv("NO_COLOR")
		_ = os.Setenv("TERM", "xterm-256color")
		assert.True(t, HasColorSupport())
	})

	t.Run("no color when NO_COLOR is set", func(t *testing.T) {
		_ = os.Setenv("NO_COLOR", "1")
		_ = os.Setenv("TERM", "xterm-256color")
		assert.False(t, HasColorSupport())
	})

	t.Run("no color when TERM is dumb", func(t *testing.T) {
		_ = os.Unsetenv("NO_COLOR")
		_ = os.Setenv("TERM", "dumb")
		assert.False(t, HasColorSupport())
	})

	t.Run("no color when NO_COLOR is empty string (should still be set)", func(t *testing.T) {
		// NO_COLOR spec requires that any value including empty string means no color
		_ = os.Setenv("NO_COLOR", "")
		_ = os.Setenv("TERM", "xterm-256color")
		// Empty string still counts as "set" for NO_COLOR
		assert.False(t, HasColorSupport())
	})
}

// TestCheckNoColor verifies CheckNoColor handles env vars correctly (AC: #4).
func TestCheckNoColor(t *testing.T) {
	// Save original env vars
	origNoColor := os.Getenv("NO_COLOR")
	origTerm := os.Getenv("TERM")
	defer func() {
		_ = os.Setenv("NO_COLOR", origNoColor)
		_ = os.Setenv("TERM", origTerm)
	}()

	t.Run("CheckNoColor is callable", func(_ *testing.T) {
		// Just verify the function doesn't panic
		_ = os.Unsetenv("NO_COLOR")
		_ = os.Setenv("TERM", "xterm")
		CheckNoColor() // Should not panic
	})
}

// TestStyleSystem_NewStyleSystem verifies StyleSystem creation (AC: #6).
func TestStyleSystem_NewStyleSystem(t *testing.T) {
	sys := NewStyleSystem()
	assert.NotNil(t, sys)

	// Verify colors are initialized
	assert.NotEmpty(t, sys.Colors.Primary.Light)
	assert.NotEmpty(t, sys.Colors.Primary.Dark)
	assert.NotEmpty(t, sys.Colors.Success.Light)
	assert.NotEmpty(t, sys.Colors.Warning.Light)
	assert.NotEmpty(t, sys.Colors.Error.Light)
	assert.NotEmpty(t, sys.Colors.Muted.Light)

	// Verify typography is initialized
	assert.NotNil(t, sys.Typography.Bold)
	assert.NotNil(t, sys.Typography.Dim)
	assert.NotNil(t, sys.Typography.Underline)
	assert.NotNil(t, sys.Typography.Reverse)
}

// TestStyleSystem_Icons verifies icon functions are accessible.
func TestStyleSystem_Icons(t *testing.T) {
	sys := NewStyleSystem()

	// Verify TaskStatusIcon works (Running now uses ● per spec)
	icon := sys.Icons.TaskStatus(constants.TaskStatusRunning)
	assert.Equal(t, "●", icon)

	// Verify WorkspaceStatusIcon works
	wsIcon := sys.Icons.WorkspaceStatus(constants.WorkspaceStatusActive)
	assert.Equal(t, "●", wsIcon)

	// Verify FormatWithIcon works
	formatted := sys.Icons.FormatWithIcon(constants.TaskStatusRunning, "Running")
	assert.Contains(t, formatted, "●")
	assert.Contains(t, formatted, "Running")
}

// TestBoxStyle_DefaultWidth verifies default box width (Task 6).
func TestBoxStyle_DefaultWidth(t *testing.T) {
	box := NewBoxStyle()
	assert.Equal(t, DefaultBoxWidth, box.Width)
	assert.Equal(t, 65, box.Width) // Per UX spec
}

// TestBoxStyle_DefaultBorder verifies square corners per UX spec.
func TestBoxStyle_DefaultBorder(t *testing.T) {
	box := NewBoxStyle()
	assert.NotNil(t, box.Border)

	// Verify border characters for square corners per UX spec
	// From epic-7-tui-components-from-scenarios.md: "Single-line box drawing characters (┌┐└┘─│├┤)"
	assert.Equal(t, "┌", box.Border.TopLeft)
	assert.Equal(t, "┐", box.Border.TopRight)
	assert.Equal(t, "└", box.Border.BottomLeft)
	assert.Equal(t, "┘", box.Border.BottomRight)
	assert.Equal(t, "─", box.Border.Top)
	assert.Equal(t, "─", box.Border.Bottom)
	assert.Equal(t, "│", box.Border.Left)
	assert.Equal(t, "│", box.Border.Right)
}

// TestBoxStyle_RoundedBorderAlternative verifies rounded border is still available.
func TestBoxStyle_RoundedBorderAlternative(t *testing.T) {
	// RoundedBorder is still available as an alternative
	assert.Equal(t, "╭", RoundedBorder.TopLeft)
	assert.Equal(t, "╮", RoundedBorder.TopRight)
	assert.Equal(t, "╰", RoundedBorder.BottomLeft)
	assert.Equal(t, "╯", RoundedBorder.BottomRight)
}

// TestBoxStyle_WithWidth verifies variable width support.
func TestBoxStyle_WithWidth(t *testing.T) {
	box := NewBoxStyle().WithWidth(80)
	assert.Equal(t, 80, box.Width)

	// Original should be unchanged
	original := NewBoxStyle()
	assert.Equal(t, DefaultBoxWidth, original.Width)
}

// TestBoxStyle_Render renders a simple box.
func TestBoxStyle_Render(t *testing.T) {
	box := NewBoxStyle().WithWidth(20)
	rendered := box.Render("Test", "Content")

	// Should contain title and content
	assert.Contains(t, rendered, "Test")
	assert.Contains(t, rendered, "Content")

	// Should contain square border characters per UX spec
	assert.Contains(t, rendered, "┌")
	assert.Contains(t, rendered, "┘")
}

// TestBoxStyle_Render_MultiLine verifies multi-line content support.
func TestBoxStyle_Render_MultiLine(t *testing.T) {
	box := NewBoxStyle().WithWidth(30)
	rendered := box.Render("Title", "Line 1\nLine 2\nLine 3")

	// Should contain all lines
	assert.Contains(t, rendered, "Line 1")
	assert.Contains(t, rendered, "Line 2")
	assert.Contains(t, rendered, "Line 3")

	// Should have proper structure (count newlines)
	lines := strings.Split(rendered, "\n")
	// Expected: top + title + divider + 3 content lines + bottom = 7 lines
	assert.Len(t, lines, 7)
}

// TestBoxStyle_Render_UnicodeContent verifies Unicode content is handled correctly.
func TestBoxStyle_Render_UnicodeContent(t *testing.T) {
	box := NewBoxStyle().WithWidth(20)
	rendered := box.Render("● Status", "✓ Done")

	// Should contain Unicode characters
	assert.Contains(t, rendered, "●")
	assert.Contains(t, rendered, "✓")
}

// TestPadRight_Unicode verifies padRight handles Unicode correctly.
func TestPadRight_Unicode(t *testing.T) {
	// "● Test" is 6 visual chars (● counts as 1, space as 1, T-e-s-t as 4)
	// but 8 bytes (● is 3 bytes in UTF-8)
	result := padRight("● Test", 10)

	// Should be exactly 10 runes, not 10 bytes
	assert.Equal(t, 10, utf8.RuneCountInString(result))
	assert.True(t, strings.HasPrefix(result, "● Test"))
}

// TestPadRight_Truncation verifies padRight truncates by rune count.
func TestPadRight_Truncation(t *testing.T) {
	result := padRight("●●●●●", 3)

	// Should be exactly 3 runes
	assert.Equal(t, 3, utf8.RuneCountInString(result))
	assert.Equal(t, "●●●", result)
}
