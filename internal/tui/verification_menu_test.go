// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerificationMenuChoice_String(t *testing.T) {
	tests := []struct {
		choice   VerificationMenuChoice
		expected string
	}{
		{VerifyMenuAutoFix, "auto_fix"},
		{VerifyMenuManualFix, "manual_fix"},
		{VerifyMenuIgnoreContinue, "ignore_continue"},
		{VerifyMenuViewReport, "view_report"},
		{VerificationMenuChoice(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.choice.String())
		})
	}
}

func TestRenderVerificationMenu(t *testing.T) {
	t.Run("with errors and warnings", func(t *testing.T) {
		opts := VerificationMenuOptions{
			TaskID:       "task-123",
			TaskDesc:     "Fix authentication bug",
			TotalIssues:  5,
			ErrorCount:   2,
			WarningCount: 3,
			InfoCount:    0,
			ReportPath:   "/tmp/verification-report.md",
			Summary:      "Found 5 issues requiring attention",
			HasErrors:    true,
		}

		menu := RenderVerificationMenu(opts)

		assert.Contains(t, menu, "Verification Issues Found")
		assert.Contains(t, menu, "task-123")
		assert.Contains(t, menu, "Fix authentication bug")
		assert.Contains(t, menu, "2 error(s)")
		assert.Contains(t, menu, "3 warning(s)")
		assert.Contains(t, menu, "Auto-fix issues")
		assert.Contains(t, menu, "Manual fix")
		assert.Contains(t, menu, "Ignore and continue")
		assert.Contains(t, menu, "View full report")
		assert.Contains(t, menu, "has errors")
		assert.Contains(t, menu, "/tmp/verification-report.md")
	})

	t.Run("warnings only", func(t *testing.T) {
		opts := VerificationMenuOptions{
			TaskID:       "task-456",
			TotalIssues:  2,
			ErrorCount:   0,
			WarningCount: 2,
			InfoCount:    0,
			HasErrors:    false,
		}

		menu := RenderVerificationMenu(opts)

		assert.Contains(t, menu, "2 warning(s)")
		assert.NotContains(t, menu, "error(s)")
		assert.NotContains(t, menu, "has errors")
	})

	t.Run("info only", func(t *testing.T) {
		opts := VerificationMenuOptions{
			TaskID:      "task-789",
			TotalIssues: 1,
			ErrorCount:  0,
			InfoCount:   1,
			HasErrors:   false,
		}

		menu := RenderVerificationMenu(opts)

		assert.Contains(t, menu, "1 info")
		assert.NotContains(t, menu, "error(s)")
		assert.NotContains(t, menu, "warning(s)")
	})
}

func TestFormatVerificationSummary(t *testing.T) {
	t.Run("passed", func(t *testing.T) {
		summary := FormatVerificationSummary(0, 0, 0, true)
		assert.Contains(t, summary, "passed")
	})

	t.Run("errors", func(t *testing.T) {
		summary := FormatVerificationSummary(3, 2, 1, false)
		assert.Contains(t, summary, "2 error(s)")
		assert.Contains(t, summary, "1 warning(s)")
	})

	t.Run("warnings only", func(t *testing.T) {
		summary := FormatVerificationSummary(2, 0, 2, false)
		assert.Contains(t, summary, "2 warning(s)")
		assert.NotContains(t, summary, "error(s)")
	})

	t.Run("info only", func(t *testing.T) {
		summary := FormatVerificationSummary(1, 0, 0, false)
		assert.Contains(t, summary, "1 info")
	})
}

func TestRenderVerificationReport(t *testing.T) {
	report := "# Test Report\n\n## Summary\nAll good"

	rendered := RenderVerificationReport(report)

	assert.Contains(t, rendered, "Verification Report")
	assert.Contains(t, rendered, "# Test Report")
	assert.Contains(t, rendered, "## Summary")
	assert.Contains(t, rendered, "All good")
}

func TestGetIgnoreNote(t *testing.T) {
	assert.Contains(t, getIgnoreNote(true), "has errors")
	assert.Empty(t, getIgnoreNote(false))
}
