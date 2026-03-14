package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCIFailureMenuChoice_String(t *testing.T) {
	tests := []struct {
		choice   CIFailureMenuChoice
		expected string
	}{
		{CIMenuViewLogs, "view_logs"},
		{CIMenuRetryFromImplement, "retry_implement"},
		{CIMenuFixManually, "fix_manually"},
		{CIMenuAbandonTask, "abandon"},
		{CIFailureMenuChoice(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.choice.String())
		})
	}
}

func TestRenderCIFailureMenu(t *testing.T) {
	t.Run("renders menu with failed checks", func(t *testing.T) {
		output := RenderCIFailureMenu(CIFailureMenuOptions{
			PRNumber:      42,
			WorkspaceName: "fix-bug",
			FailedChecks:  []string{"CI / lint", "CI / test"},
			HasCheckURL:   true,
			Status:        "failure",
		})

		assert.Contains(t, output, "CI Failed")
		assert.Contains(t, output, "PR #42")
		assert.Contains(t, output, "fix-bug")
		assert.Contains(t, output, "CI / lint")
		assert.Contains(t, output, "CI / test")
		assert.Contains(t, output, "View workflow logs")
		assert.Contains(t, output, "Retry from implement")
		assert.Contains(t, output, "Fix manually")
		assert.Contains(t, output, "Abandon task")
	})

	t.Run("renders timeout status", func(t *testing.T) {
		output := RenderCIFailureMenu(CIFailureMenuOptions{
			PRNumber:    123,
			HasCheckURL: false,
			Status:      "timeout",
		})

		assert.Contains(t, output, "CI Timed Out")
		assert.Contains(t, output, "PR #123")
		// View logs should be unavailable
		assert.Contains(t, output, "unavailable")
	})

	t.Run("handles empty failed checks", func(t *testing.T) {
		output := RenderCIFailureMenu(CIFailureMenuOptions{
			PRNumber:     42,
			FailedChecks: []string{},
			HasCheckURL:  true,
		})

		assert.Contains(t, output, "PR #42")
		assert.NotContains(t, output, "Failed Checks:")
	})
}

func TestFormatCIFailureStatus(t *testing.T) {
	t.Run("formats failure status", func(t *testing.T) {
		output := FormatCIFailureStatus(42, []string{"lint", "test"}, "failure")

		assert.Contains(t, output, "CI failure")
		assert.Contains(t, output, "PR #42")
		assert.Contains(t, output, "lint, test")
	})

	t.Run("formats timeout status", func(t *testing.T) {
		output := FormatCIFailureStatus(123, nil, "timeout")

		assert.Contains(t, output, "CI timeout")
		assert.Contains(t, output, "PR #123")
	})

	t.Run("handles no failed checks", func(t *testing.T) {
		output := FormatCIFailureStatus(42, []string{}, "failure")

		assert.Contains(t, output, "PR #42")
		assert.NotContains(t, output, "Failed:")
	})
}
