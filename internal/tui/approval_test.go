// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

// TestApprovalSummary_NewApprovalSummary tests construction from domain.Task and domain.Workspace.
func TestApprovalSummary_NewApprovalSummary(t *testing.T) {
	// Create test task
	now := time.Now()
	task := &domain.Task{
		ID:          "task-test-abc",
		WorkspaceID: "ws-test",
		Description: "Fix null pointer in parseConfig",
		Status:      constants.TaskStatusAwaitingApproval,
		CurrentStep: 5,
		Steps: []domain.Step{
			{Name: "analyze", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "implement", Type: domain.StepTypeAI, Status: "completed"},
			{Name: "validate", Type: domain.StepTypeValidation, Status: "completed"},
			{Name: "commit", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "push", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "pr", Type: domain.StepTypeGit, Status: "completed"},
			{Name: "approval", Type: domain.StepTypeHuman, Status: "pending"},
		},
		StepResults: []domain.StepResult{
			{
				StepIndex:    1,
				StepName:     "implement",
				Status:       "success",
				FilesChanged: []string{"internal/config/parser.go", "internal/config/parser_test.go"},
			},
			{
				StepIndex: 2,
				StepName:  "validate",
				Status:    "success",
			},
		},
		Metadata: map[string]any{
			"pr_url": "https://github.com/org/repo/pull/47",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	workspace := &domain.Workspace{
		Name:         "payment",
		Branch:       "fix/payment-null-ptr",
		Status:       constants.WorkspaceStatusActive,
		WorktreePath: "/path/to/worktree",
	}

	// Create approval summary
	summary := NewApprovalSummary(task, workspace)

	// Verify basic fields
	require.NotNil(t, summary)
	assert.Equal(t, "task-test-abc", summary.TaskID)
	assert.Equal(t, "payment", summary.WorkspaceName)
	assert.Equal(t, constants.TaskStatusAwaitingApproval, summary.Status)
	assert.Equal(t, 6, summary.CurrentStep) // CurrentStep is 0-based, display is 1-based
	assert.Equal(t, 7, summary.TotalSteps)
	assert.Equal(t, "Fix null pointer in parseConfig", summary.Description)
	assert.Equal(t, "fix/payment-null-ptr", summary.BranchName)
	assert.Equal(t, "https://github.com/org/repo/pull/47", summary.PRURL)
}

// TestApprovalSummary_FileChanges tests file change tracking and stats.
func TestApprovalSummary_FileChanges(t *testing.T) {
	task := &domain.Task{
		ID:          "task-file-test",
		WorkspaceID: "ws-test",
		Status:      constants.TaskStatusAwaitingApproval,
		CurrentStep: 3,
		Steps: []domain.Step{
			{Name: "implement", Status: "completed"},
			{Name: "validate", Status: "completed"},
			{Name: "commit", Status: "completed"},
			{Name: "approval", Status: "pending"},
		},
		StepResults: []domain.StepResult{
			{
				StepName:     "implement",
				Status:       "success",
				FilesChanged: []string{"file1.go", "file2.go", "file3.go"},
			},
		},
	}

	workspace := &domain.Workspace{
		Name:   "test-ws",
		Branch: "feat/test",
	}

	summary := NewApprovalSummary(task, workspace)

	// Verify files are collected
	require.NotNil(t, summary.FileChanges)
	assert.Len(t, summary.FileChanges, 3)
}

// TestApprovalSummary_ValidationStatus tests validation status extraction.
func TestApprovalSummary_ValidationStatus(t *testing.T) {
	t.Run("validation passed", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-val-pass",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusAwaitingApproval,
			CurrentStep: 2,
			Steps: []domain.Step{
				{Name: "implement", Status: "completed"},
				{Name: "validate", Status: "completed"},
				{Name: "approval", Status: "pending"},
			},
			StepResults: []domain.StepResult{
				{StepName: "validate", Status: "success"},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Equal(t, "passed", summary.Validation.Status)
	})

	t.Run("validation failed", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-val-fail",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusValidationFailed,
			CurrentStep: 1,
			Steps: []domain.Step{
				{Name: "implement", Status: "completed"},
				{Name: "validate", Status: "failed"},
				{Name: "approval", Status: "pending"},
			},
			StepResults: []domain.StepResult{
				{StepName: "validate", Status: "failed", Error: "lint errors"},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Equal(t, "failed", summary.Validation.Status)
	})
}

// TestApprovalSummary_NilInputs tests behavior with nil inputs.
func TestApprovalSummary_NilInputs(t *testing.T) {
	t.Run("nil task", func(t *testing.T) {
		workspace := &domain.Workspace{Name: "test", Branch: "main"}
		summary := NewApprovalSummary(nil, workspace)
		assert.Nil(t, summary)
	})

	t.Run("nil workspace", func(t *testing.T) {
		task := &domain.Task{ID: "test", Status: constants.TaskStatusPending}
		summary := NewApprovalSummary(task, nil)
		// Should still work, just with empty workspace info
		require.NotNil(t, summary)
		assert.Empty(t, summary.WorkspaceName)
		assert.Empty(t, summary.BranchName)
	})
}

// TestApprovalSummary_NoPRURL tests behavior when PR URL is not present.
func TestApprovalSummary_NoPRURL(t *testing.T) {
	task := &domain.Task{
		ID:          "task-no-pr",
		WorkspaceID: "ws-test",
		Status:      constants.TaskStatusAwaitingApproval,
		CurrentStep: 2,
		Steps: []domain.Step{
			{Name: "implement", Status: "completed"},
			{Name: "validate", Status: "completed"},
			{Name: "approval", Status: "pending"},
		},
		// No metadata or no pr_url key
	}

	workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
	summary := NewApprovalSummary(task, workspace)

	require.NotNil(t, summary)
	assert.Empty(t, summary.PRURL)
}

// TestFileChange_Struct tests the FileChange struct.
func TestFileChange_Struct(t *testing.T) {
	fc := FileChange{
		Path:       "internal/config/parser.go",
		Insertions: 45,
		Deletions:  12,
	}

	assert.Equal(t, "internal/config/parser.go", fc.Path)
	assert.Equal(t, 45, fc.Insertions)
	assert.Equal(t, 12, fc.Deletions)
}

// TestValidationSummary_Struct tests the ValidationSummary struct.
func TestValidationSummary_Struct(t *testing.T) {
	now := time.Now()
	vs := ValidationSummary{
		PassCount: 3,
		FailCount: 0,
		Status:    "passed",
		LastRunAt: &now,
	}

	assert.Equal(t, 3, vs.PassCount)
	assert.Equal(t, 0, vs.FailCount)
	assert.Equal(t, "passed", vs.Status)
	assert.NotNil(t, vs.LastRunAt)
}

// TestRenderApprovalSummary tests the approval summary renderer (AC: #1, #2, #3, #4).
func TestRenderApprovalSummary(t *testing.T) {
	t.Run("full summary with all fields", func(t *testing.T) {
		now := time.Now()
		summary := &ApprovalSummary{
			TaskID:          "task-test-abc",
			WorkspaceName:   "payment",
			Status:          constants.TaskStatusAwaitingApproval,
			CurrentStep:     6,
			TotalSteps:      7,
			Description:     "Fix null pointer in parseConfig",
			BranchName:      "fix/payment-null-ptr",
			PRURL:           "https://github.com/org/repo/pull/47",
			TotalInsertions: 45,
			TotalDeletions:  12,
			FileChanges: []FileChange{
				{Path: "internal/payment/handler.go", Insertions: 30, Deletions: 10},
				{Path: "internal/payment/handler_test.go", Insertions: 15, Deletions: 2},
			},
			Validation: &ValidationSummary{
				PassCount: 3,
				FailCount: 0,
				Status:    "passed",
				LastRunAt: &now,
			},
		}

		result := RenderApprovalSummary(summary)

		// Verify key content is present
		assert.Contains(t, result, "Approval Summary")
		assert.Contains(t, result, "payment")
		assert.Contains(t, result, "fix/payment-null-ptr")
		assert.Contains(t, result, "awaiting_approval")
		assert.Contains(t, result, "6/7")
		assert.Contains(t, result, "internal/payment/handler.go")
		assert.Contains(t, result, "passed")
	})

	t.Run("summary without PR URL", func(t *testing.T) {
		summary := &ApprovalSummary{
			TaskID:        "task-no-pr",
			WorkspaceName: "test",
			Status:        constants.TaskStatusAwaitingApproval,
			CurrentStep:   3,
			TotalSteps:    5,
			Description:   "Test task",
			BranchName:    "feat/test",
		}

		result := RenderApprovalSummary(summary)

		// Should not contain PR section
		assert.Contains(t, result, "Approval Summary")
		assert.Contains(t, result, "test")
		assert.NotContains(t, result, "PR:")
	})

	t.Run("nil summary returns empty", func(t *testing.T) {
		result := RenderApprovalSummary(nil)
		assert.Empty(t, result)
	})
}

// TestRenderApprovalSummary_NOCOLORMode tests rendering without colors (AC: #4.8).
func TestRenderApprovalSummary_NOCOLORMode(t *testing.T) {
	// Set NO_COLOR environment variable
	t.Setenv("NO_COLOR", "1")

	now := time.Now()
	summary := &ApprovalSummary{
		TaskID:          "task-nocolor",
		WorkspaceName:   "test",
		Status:          constants.TaskStatusAwaitingApproval,
		CurrentStep:     2,
		TotalSteps:      4,
		Description:     "Test NO_COLOR",
		BranchName:      "feat/test",
		TotalInsertions: 10,
		TotalDeletions:  5,
		FileChanges: []FileChange{
			{Path: "file.go", Insertions: 10, Deletions: 5},
		},
		Validation: &ValidationSummary{
			PassCount: 1,
			Status:    "passed",
			LastRunAt: &now,
		},
	}

	result := RenderApprovalSummary(summary)

	// Should still render content (just without ANSI codes)
	assert.Contains(t, result, "Approval Summary")
	assert.Contains(t, result, "test")

	// Verify no ANSI escape codes in output
	// ANSI codes start with \x1b[ or \x1b]
	assert.NotContains(t, result, "\x1b[", "Output should not contain ANSI color codes when NO_COLOR is set")
}

// TestFormatHyperlink tests OSC 8 hyperlink formatting (AC: #2).
func TestFormatHyperlink(t *testing.T) {
	t.Run("hyperlink format", func(t *testing.T) {
		url := "https://github.com/org/repo/pull/47"
		display := "#47"

		// We can't easily test the actual hyperlink output without mocking SupportsHyperlinks
		// but we can verify the function doesn't panic and returns something
		result := FormatHyperlink(url, display)
		assert.NotEmpty(t, result)
		assert.Contains(t, result, display)
	})
}

// TestSupportsHyperlinks tests hyperlink detection (AC: #2).
func TestSupportsHyperlinks(t *testing.T) {
	t.Run("iTerm detection", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "iTerm.app")
		assert.True(t, SupportsHyperlinks())
	})

	t.Run("vscode detection", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "vscode")
		assert.True(t, SupportsHyperlinks())
	})

	t.Run("LC_TERMINAL iTerm2", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "")
		t.Setenv("LC_TERMINAL", "iTerm2")
		assert.True(t, SupportsHyperlinks())
	})

	t.Run("unknown terminal", func(t *testing.T) {
		t.Setenv("TERM_PROGRAM", "")
		t.Setenv("LC_TERMINAL", "")
		assert.False(t, SupportsHyperlinks())
	})
}

// TestRenderApprovalSummary_TerminalWidth tests width adaptation (AC: #5).
func TestRenderApprovalSummary_TerminalWidth(t *testing.T) {
	summary := &ApprovalSummary{
		TaskID:        "task-width",
		WorkspaceName: "test",
		Status:        constants.TaskStatusAwaitingApproval,
		CurrentStep:   2,
		TotalSteps:    4,
		Description:   "Test width adaptation",
		BranchName:    "feat/test",
		FileChanges: []FileChange{
			{Path: "very/long/path/to/some/deeply/nested/file.go"},
		},
	}

	// Test at different widths
	t.Run("narrow width 60", func(t *testing.T) {
		result := RenderApprovalSummaryWithWidth(summary, 60)
		assert.NotEmpty(t, result)
		// In narrow mode, paths might be truncated
	})

	t.Run("standard width 80", func(t *testing.T) {
		result := RenderApprovalSummaryWithWidth(summary, 80)
		assert.NotEmpty(t, result)
	})

	t.Run("expanded width 120", func(t *testing.T) {
		result := RenderApprovalSummaryWithWidth(summary, 120)
		assert.NotEmpty(t, result)
		// Expanded mode shows description
		assert.Contains(t, result, "Test width adaptation")
	})

	t.Run("expanded mode shows more files", func(t *testing.T) {
		// Create summary with many files
		manyFiles := &ApprovalSummary{
			TaskID:        "task-many",
			WorkspaceName: "test",
			Status:        constants.TaskStatusAwaitingApproval,
			CurrentStep:   1,
			TotalSteps:    2,
			BranchName:    "feat/test",
			FileChanges: []FileChange{
				{Path: "file1.go"},
				{Path: "file2.go"},
				{Path: "file3.go"},
				{Path: "file4.go"},
				{Path: "file5.go"},
				{Path: "file6.go"},
				{Path: "file7.go"},
				{Path: "file8.go"},
			},
		}

		// Compact mode shows max 3 files
		compactResult := RenderApprovalSummaryWithWidth(manyFiles, 60)
		assert.Contains(t, compactResult, "... and 5 more files")

		// Standard mode shows max 5 files
		standardResult := RenderApprovalSummaryWithWidth(manyFiles, 80)
		assert.Contains(t, standardResult, "... and 3 more files")

		// Expanded mode shows max 10 files (8 files = all shown)
		expandedResult := RenderApprovalSummaryWithWidth(manyFiles, 120)
		assert.NotContains(t, expandedResult, "... and")
		assert.Contains(t, expandedResult, "file8.go")
	})
}

// TestExtractPRNumber tests PR number extraction from URLs.
func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard github PR URL",
			url:      "https://github.com/org/repo/pull/47",
			expected: "#47",
		},
		{
			name:     "PR URL with trailing slash",
			url:      "https://github.com/org/repo/pull/123/",
			expected: "#123",
		},
		{
			name:     "PR URL with files suffix",
			url:      "https://github.com/org/repo/pull/999/files",
			expected: "#999",
		},
		{
			name:     "non-github URL",
			url:      "https://example.com/pr/42",
			expected: "https://example.com/pr/42",
		},
		{
			name:     "empty URL",
			url:      "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPRNumber(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncatePath tests path truncation with filename preservation.
func TestTruncatePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		maxLen   int
		expected string
	}{
		{
			name:     "short path unchanged",
			path:     "file.go",
			maxLen:   20,
			expected: "file.go",
		},
		{
			name:     "long path truncated preserving filename",
			path:     "very/long/path/to/file.go",
			maxLen:   24, // Path is 25 chars, so this triggers truncation
			expected: ".../long/path/to/file.go",
		},
		{
			name:     "filename only when path too long",
			path:     "a/b/c/d/important_file.go",
			maxLen:   20,
			expected: "important_file.go", // Filename fits, dir truncated to nothing
		},
		{
			name:     "no path separator truncates string",
			path:     "verylongfilename.go",
			maxLen:   13,
			expected: "verylongfi...",
		},
		{
			name:     "exact length unchanged",
			path:     "exactly/twenty/chars",
			maxLen:   20,
			expected: "exactly/twenty/chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePath(tt.path, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAbbreviateLabel tests label abbreviation for compact mode.
func TestAbbreviateLabel(t *testing.T) {
	tests := []struct {
		label    string
		expected string
	}{
		{"Workspace", "WS"},
		{"Branch", "Br"},
		{"Status", "St"},
		{"Progress", "Pr"},
		{"Validation", "Val"},
		{"Unknown", "Unknown"}, // Unknown labels unchanged
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			result := abbreviateLabel(tt.label)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSetFileStats tests setting file statistics after construction.
func TestSetFileStats(t *testing.T) {
	task := &domain.Task{
		ID:          "task-stats",
		WorkspaceID: "ws-test",
		Status:      constants.TaskStatusAwaitingApproval,
		CurrentStep: 1,
		Steps:       []domain.Step{{Name: "implement", Status: "completed"}},
		StepResults: []domain.StepResult{
			{
				StepName:     "implement",
				Status:       "success",
				FilesChanged: []string{"file1.go", "file2.go", "file3.go"},
			},
		},
	}

	workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
	summary := NewApprovalSummary(task, workspace)

	// Initially, stats are zero
	assert.Equal(t, 0, summary.TotalInsertions)
	assert.Equal(t, 0, summary.TotalDeletions)

	// Set stats from git diff data
	stats := map[string]FileChange{
		"file1.go": {Path: "file1.go", Insertions: 30, Deletions: 10},
		"file2.go": {Path: "file2.go", Insertions: 15, Deletions: 5},
		// file3.go not in stats - should remain zero
	}
	summary.SetFileStats(stats)

	// Verify totals updated
	assert.Equal(t, 45, summary.TotalInsertions)
	assert.Equal(t, 15, summary.TotalDeletions)

	// Verify individual files updated
	assert.Equal(t, 30, summary.FileChanges[0].Insertions)
	assert.Equal(t, 10, summary.FileChanges[0].Deletions)
	assert.Equal(t, 15, summary.FileChanges[1].Insertions)
	assert.Equal(t, 5, summary.FileChanges[1].Deletions)
	assert.Equal(t, 0, summary.FileChanges[2].Insertions) // Not in stats
	assert.Equal(t, 0, summary.FileChanges[2].Deletions)
}

// TestGetDisplayMode tests display mode selection.
func TestGetDisplayMode(t *testing.T) {
	tests := []struct {
		width    int
		expected displayMode
	}{
		{60, displayModeCompact},
		{79, displayModeCompact},
		{80, displayModeStandard},
		{100, displayModeStandard},
		{119, displayModeStandard},
		{120, displayModeExpanded},
		{150, displayModeExpanded},
	}

	for _, tt := range tests {
		t.Run("width_"+string(rune('0'+tt.width/10))+string(rune('0'+tt.width%10)), func(t *testing.T) {
			result := getDisplayMode(tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRenderPRLine tests PR line rendering across display modes.
// This specifically tests that PR text is not truncated, avoiding ANSI escape sequence corruption.
func TestRenderPRLine(t *testing.T) {
	tests := []struct {
		name        string
		prURL       string
		mode        displayMode
		expectLabel string
		expectPR    string
	}{
		{
			name:        "compact mode shows abbreviated label",
			prURL:       "https://github.com/org/repo/pull/47",
			mode:        displayModeCompact,
			expectLabel: "PR:",
			expectPR:    "#47",
		},
		{
			name:        "standard mode shows full label",
			prURL:       "https://github.com/org/repo/pull/123",
			mode:        displayModeStandard,
			expectLabel: "PR:",
			expectPR:    "#123",
		},
		{
			name:        "expanded mode shows full URL in parentheses",
			prURL:       "https://github.com/org/repo/pull/999",
			mode:        displayModeExpanded,
			expectLabel: "PR:",
			expectPR:    "#999",
		},
		{
			name:        "non-github URL shows full URL as display text",
			prURL:       "https://gitlab.com/org/repo/-/merge_requests/42",
			mode:        displayModeStandard,
			expectLabel: "PR:",
			expectPR:    "https://gitlab.com/org/repo/-/merge_requests/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderPRLine(tt.prURL, tt.mode)

			// Verify label is present
			assert.Contains(t, result, tt.expectLabel)

			// Verify PR number/URL is present (may be wrapped in ANSI codes)
			assert.Contains(t, result, tt.expectPR)

			// Verify not truncated to "..."
			assert.NotContains(t, result, "...")

			// Verify line ends with newline
			assert.True(t, len(result) > 0 && result[len(result)-1] == '\n')
		})
	}
}

// TestRenderPRLine_NoTruncation verifies that long PR URLs are not truncated.
// This is the key fix - the PR line should never be truncated because truncation
// corrupts ANSI escape sequences used for hyperlinks and underlines.
func TestRenderPRLine_NoTruncation(t *testing.T) {
	// Very long URL that would trigger truncation in compact mode
	longURL := "https://github.com/very-long-organization-name/extremely-long-repository-name/pull/12345"

	modes := []struct {
		name string
		mode displayMode
	}{
		{"compact", displayModeCompact},
		{"standard", displayModeStandard},
		{"expanded", displayModeExpanded},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			result := renderPRLine(longURL, m.mode)

			// Should contain the PR number
			assert.Contains(t, result, "#12345")

			// Should never be truncated
			assert.NotContains(t, result, "...")
		})
	}
}

// TestValidationCheck_Struct tests the ValidationCheck struct.
func TestValidationCheck_Struct(t *testing.T) {
	vc := ValidationCheck{
		Name:   "Format",
		Passed: true,
	}

	assert.Equal(t, "Format", vc.Name)
	assert.True(t, vc.Passed)
}

// TestValidationSummary_WithChecks tests ValidationSummary with individual checks.
func TestValidationSummary_WithChecks(t *testing.T) {
	now := time.Now()
	vs := ValidationSummary{
		PassCount: 4,
		FailCount: 1,
		Status:    "failed",
		LastRunAt: &now,
		Checks: []ValidationCheck{
			{Name: "Format", Passed: true},
			{Name: "Lint", Passed: false},
			{Name: "Test", Passed: true},
			{Name: "Pre-commit", Passed: true},
			{Name: "CI", Passed: true},
		},
	}

	assert.Equal(t, 4, vs.PassCount)
	assert.Equal(t, 1, vs.FailCount)
	assert.Equal(t, "failed", vs.Status)
	assert.Len(t, vs.Checks, 5)
	assert.Equal(t, "Lint", vs.Checks[1].Name)
	assert.False(t, vs.Checks[1].Passed)
}

// TestExtractValidationStatus_WithMetadata tests extraction with validation_checks metadata.
func TestExtractValidationStatus_WithMetadata(t *testing.T) {
	t.Run("extracts checks from metadata", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-val-checks",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusAwaitingApproval,
			CurrentStep: 2,
			Steps: []domain.Step{
				{Name: "implement", Status: "completed"},
				{Name: "validate", Status: "completed"},
				{Name: "approval", Status: "pending"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "validate",
					Status:   "success",
					Metadata: map[string]any{
						"validation_checks": []map[string]any{
							{"name": "Format", "passed": true},
							{"name": "Lint", "passed": true},
							{"name": "Test", "passed": true},
							{"name": "Pre-commit", "passed": true},
						},
					},
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Equal(t, "passed", summary.Validation.Status)
		assert.Len(t, summary.Validation.Checks, 4)
		assert.Equal(t, 4, summary.Validation.PassCount)
		assert.Equal(t, 0, summary.Validation.FailCount)
	})

	t.Run("extracts checks with failures", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-val-fail-checks",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusValidationFailed,
			CurrentStep: 1,
			Steps: []domain.Step{
				{Name: "implement", Status: "completed"},
				{Name: "validate", Status: "failed"},
				{Name: "approval", Status: "pending"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "validate",
					Status:   "failed",
					Metadata: map[string]any{
						"validation_checks": []map[string]any{
							{"name": "Format", "passed": true},
							{"name": "Lint", "passed": false},
							{"name": "Test", "passed": true},
							{"name": "Pre-commit", "passed": false},
						},
					},
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Equal(t, "failed", summary.Validation.Status)
		assert.Len(t, summary.Validation.Checks, 4)
		assert.Equal(t, 2, summary.Validation.PassCount)
		assert.Equal(t, 2, summary.Validation.FailCount)
	})

	t.Run("fallback without metadata", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-val-no-meta",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusAwaitingApproval,
			CurrentStep: 1,
			Steps: []domain.Step{
				{Name: "validate", Status: "completed"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "validate",
					Status:   "success",
					// No Metadata
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Equal(t, "passed", summary.Validation.Status)
		assert.Empty(t, summary.Validation.Checks)
		// Fallback to legacy count
		assert.Equal(t, 1, summary.Validation.PassCount)
		assert.Equal(t, 0, summary.Validation.FailCount)
	})
}

// TestParseValidationChecks tests parsing validation checks from metadata.
func TestParseValidationChecks(t *testing.T) {
	t.Run("parses []any slice", func(t *testing.T) {
		data := []any{
			map[string]any{"name": "Format", "passed": true},
			map[string]any{"name": "Lint", "passed": false},
		}

		checks := parseValidationChecks(data)

		require.Len(t, checks, 2)
		assert.Equal(t, "Format", checks[0].Name)
		assert.True(t, checks[0].Passed)
		assert.Equal(t, "Lint", checks[1].Name)
		assert.False(t, checks[1].Passed)
	})

	t.Run("parses []map[string]any slice", func(t *testing.T) {
		data := []map[string]any{
			{"name": "Test", "passed": true},
			{"name": "Pre-commit", "passed": true},
		}

		checks := parseValidationChecks(data)

		require.Len(t, checks, 2)
		assert.Equal(t, "Test", checks[0].Name)
		assert.True(t, checks[0].Passed)
	})

	t.Run("returns nil for invalid data", func(t *testing.T) {
		checks := parseValidationChecks("invalid")
		assert.Nil(t, checks)
	})

	t.Run("skips invalid items", func(t *testing.T) {
		data := []any{
			map[string]any{"name": "Format", "passed": true},
			"invalid",
			map[string]any{"name": "Lint", "passed": false},
		}

		checks := parseValidationChecks(data)

		require.Len(t, checks, 2)
	})

	t.Run("skips items without name", func(t *testing.T) {
		data := []any{
			map[string]any{"name": "Format", "passed": true},
			map[string]any{"passed": true}, // No name
		}

		checks := parseValidationChecks(data)

		require.Len(t, checks, 1)
		assert.Equal(t, "Format", checks[0].Name)
	})
}

// TestParseCheckMap tests parsing a single check map.
func TestParseCheckMap(t *testing.T) {
	t.Run("parses complete map", func(t *testing.T) {
		checkMap := map[string]any{
			"name":   "Format",
			"passed": true,
		}

		check := parseCheckMap(checkMap)

		assert.Equal(t, "Format", check.Name)
		assert.True(t, check.Passed)
	})

	t.Run("handles missing name", func(t *testing.T) {
		checkMap := map[string]any{
			"passed": true,
		}

		check := parseCheckMap(checkMap)

		assert.Empty(t, check.Name)
	})

	t.Run("handles missing passed", func(t *testing.T) {
		checkMap := map[string]any{
			"name": "Lint",
		}

		check := parseCheckMap(checkMap)

		assert.Equal(t, "Lint", check.Name)
		assert.False(t, check.Passed) // Default to false
	})

	t.Run("handles wrong type for name", func(t *testing.T) {
		checkMap := map[string]any{
			"name":   123, // Wrong type
			"passed": true,
		}

		check := parseCheckMap(checkMap)

		assert.Empty(t, check.Name)
	})

	t.Run("handles wrong type for passed", func(t *testing.T) {
		checkMap := map[string]any{
			"name":   "Test",
			"passed": "yes", // Wrong type
		}

		check := parseCheckMap(checkMap)

		assert.Equal(t, "Test", check.Name)
		assert.False(t, check.Passed) // Default to false
	})
}

// TestExtractCIStatus tests CI status extraction.
func TestExtractCIStatus(t *testing.T) {
	t.Run("extracts CI passed status", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-ci-pass",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusAwaitingApproval,
			CurrentStep: 3,
			Steps: []domain.Step{
				{Name: "validate", Status: "completed"},
				{Name: "git", Status: "completed"},
				{Name: "ci", Status: "completed"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "validate",
					Status:   "success",
					Metadata: map[string]any{
						"validation_checks": []map[string]any{
							{"name": "Format", "passed": true},
							{"name": "Lint", "passed": true},
						},
					},
				},
				{
					StepName: "ci",
					Status:   "success",
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Len(t, summary.Validation.Checks, 3) // Format, Lint, CI
		assert.Equal(t, "CI", summary.Validation.Checks[2].Name)
		assert.True(t, summary.Validation.Checks[2].Passed)
		assert.Equal(t, 3, summary.Validation.PassCount)
	})

	t.Run("extracts CI failed status", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-ci-fail",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusCIFailed,
			CurrentStep: 3,
			Steps: []domain.Step{
				{Name: "validate", Status: "completed"},
				{Name: "git", Status: "completed"},
				{Name: "ci-checks", Status: "failed"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "validate",
					Status:   "success",
					Metadata: map[string]any{
						"validation_checks": []map[string]any{
							{"name": "Format", "passed": true},
						},
					},
				},
				{
					StepName: "ci-checks",
					Status:   "failed",
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Len(t, summary.Validation.Checks, 2) // Format, CI
		assert.Equal(t, "CI", summary.Validation.Checks[1].Name)
		assert.False(t, summary.Validation.Checks[1].Passed)
		assert.Equal(t, "failed", summary.Validation.Status)
	})

	t.Run("creates validation summary if none exists", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task-ci-only",
			WorkspaceID: "ws-test",
			Status:      constants.TaskStatusAwaitingApproval,
			CurrentStep: 1,
			Steps: []domain.Step{
				{Name: "ci", Status: "completed"},
			},
			StepResults: []domain.StepResult{
				{
					StepName: "ci",
					Status:   "success",
				},
			},
		}

		workspace := &domain.Workspace{Name: "test", Branch: "feat/x"}
		summary := NewApprovalSummary(task, workspace)

		require.NotNil(t, summary.Validation)
		assert.Len(t, summary.Validation.Checks, 1)
		assert.Equal(t, "CI", summary.Validation.Checks[0].Name)
	})
}

// TestRenderChecksLine tests rendering individual checks.
func TestRenderChecksLine(t *testing.T) {
	// Disable colors for consistent test output
	t.Setenv("NO_COLOR", "1")

	t.Run("renders all passing checks", func(t *testing.T) {
		checks := []ValidationCheck{
			{Name: "Format", Passed: true},
			{Name: "Lint", Passed: true},
			{Name: "Test", Passed: true},
			{Name: "Pre-commit", Passed: true},
			{Name: "CI", Passed: true},
		}

		result := renderChecksLine(checks)

		assert.Contains(t, result, "Format ✓")
		assert.Contains(t, result, "Lint ✓")
		assert.Contains(t, result, "Test ✓")
		assert.Contains(t, result, "Pre-commit ✓")
		assert.Contains(t, result, "CI ✓")
		assert.Contains(t, result, " | ")
	})

	t.Run("renders mixed pass/fail checks", func(t *testing.T) {
		checks := []ValidationCheck{
			{Name: "Format", Passed: true},
			{Name: "Lint", Passed: false},
			{Name: "Test", Passed: true},
			{Name: "Pre-commit", Passed: false},
		}

		result := renderChecksLine(checks)

		assert.Contains(t, result, "Format ✓")
		assert.Contains(t, result, "Lint ✗")
		assert.Contains(t, result, "Test ✓")
		assert.Contains(t, result, "Pre-commit ✗")
	})

	t.Run("handles empty checks", func(t *testing.T) {
		checks := []ValidationCheck{}

		result := renderChecksLine(checks)

		assert.Equal(t, "    \n", result)
	})
}

// TestRenderValidationSectionWithMode tests validation section rendering with modes.
func TestRenderValidationSectionWithMode(t *testing.T) {
	// Disable colors for consistent test output
	t.Setenv("NO_COLOR", "1")

	now := time.Now()

	t.Run("standard mode shows checks", func(t *testing.T) {
		validation := &ValidationSummary{
			PassCount: 5,
			FailCount: 0,
			Status:    "passed",
			LastRunAt: &now,
			Checks: []ValidationCheck{
				{Name: "Format", Passed: true},
				{Name: "Lint", Passed: true},
				{Name: "Test", Passed: true},
				{Name: "Pre-commit", Passed: true},
				{Name: "CI", Passed: true},
			},
		}

		result := renderValidationSectionWithMode(validation, 100, displayModeStandard)

		assert.Contains(t, result, "Validation:")
		assert.Contains(t, result, "passed")
		assert.Contains(t, result, "5/5")
		assert.Contains(t, result, "Format ✓")
		assert.Contains(t, result, "CI ✓")
	})

	t.Run("compact mode hides checks", func(t *testing.T) {
		validation := &ValidationSummary{
			PassCount: 4,
			FailCount: 0,
			Status:    "passed",
			LastRunAt: &now,
			Checks: []ValidationCheck{
				{Name: "Format", Passed: true},
				{Name: "Lint", Passed: true},
				{Name: "Test", Passed: true},
				{Name: "Pre-commit", Passed: true},
			},
		}

		result := renderValidationSectionWithMode(validation, 60, displayModeCompact)

		assert.Contains(t, result, "Validation:")
		assert.Contains(t, result, "passed")
		assert.Contains(t, result, "4/4")
		// Individual checks should NOT be shown in compact mode
		assert.NotContains(t, result, "Format ✓")
	})

	t.Run("expanded mode shows checks", func(t *testing.T) {
		validation := &ValidationSummary{
			PassCount: 3,
			FailCount: 1,
			Status:    "failed",
			LastRunAt: &now,
			Checks: []ValidationCheck{
				{Name: "Format", Passed: true},
				{Name: "Lint", Passed: false},
				{Name: "Test", Passed: true},
				{Name: "Pre-commit", Passed: true},
			},
		}

		result := renderValidationSectionWithMode(validation, 120, displayModeExpanded)

		assert.Contains(t, result, "Validation:")
		assert.Contains(t, result, "failed")
		assert.Contains(t, result, "3/4")
		assert.Contains(t, result, "Format ✓")
		assert.Contains(t, result, "Lint ✗")
	})

	t.Run("no checks falls back to legacy display", func(t *testing.T) {
		validation := &ValidationSummary{
			PassCount: 1,
			FailCount: 0,
			Status:    "passed",
			LastRunAt: &now,
			Checks:    nil, // No checks
		}

		result := renderValidationSectionWithMode(validation, 100, displayModeStandard)

		assert.Contains(t, result, "Validation:")
		assert.Contains(t, result, "passed")
		assert.Contains(t, result, "1/1")
		// No individual check line
		assert.NotContains(t, result, " | ")
	})
}

// TestRenderApprovalSummary_WithChecks tests full approval summary with validation checks.
func TestRenderApprovalSummary_WithChecks(t *testing.T) {
	// Disable colors for consistent test output
	t.Setenv("NO_COLOR", "1")

	now := time.Now()
	summary := &ApprovalSummary{
		TaskID:        "task-test-checks",
		WorkspaceName: "test-ws",
		Status:        constants.TaskStatusAwaitingApproval,
		CurrentStep:   5,
		TotalSteps:    6,
		Description:   "Test with validation checks",
		BranchName:    "feat/checks",
		PRURL:         "https://github.com/org/repo/pull/100",
		FileChanges: []FileChange{
			{Path: "file.go"},
		},
		Validation: &ValidationSummary{
			PassCount: 5,
			FailCount: 0,
			Status:    "passed",
			LastRunAt: &now,
			Checks: []ValidationCheck{
				{Name: "Format", Passed: true},
				{Name: "Lint", Passed: true},
				{Name: "Test", Passed: true},
				{Name: "Pre-commit", Passed: true},
				{Name: "CI", Passed: true},
			},
		},
	}

	result := RenderApprovalSummaryWithWidth(summary, 100)

	assert.Contains(t, result, "Approval Summary")
	assert.Contains(t, result, "test-ws")
	assert.Contains(t, result, "Validation:")
	assert.Contains(t, result, "5/5")
	assert.Contains(t, result, "Format ✓")
	assert.Contains(t, result, "Lint ✓")
	assert.Contains(t, result, "Test ✓")
	assert.Contains(t, result, "Pre-commit ✓")
	assert.Contains(t, result, "CI ✓")
}

// TestRenderValidationSection_BackwardCompatibility tests the deprecated function.
func TestRenderValidationSection_BackwardCompatibility(t *testing.T) {
	// Disable colors for consistent test output
	t.Setenv("NO_COLOR", "1")

	now := time.Now()
	validation := &ValidationSummary{
		PassCount: 3,
		FailCount: 0,
		Status:    "passed",
		LastRunAt: &now,
		Checks: []ValidationCheck{
			{Name: "Format", Passed: true},
			{Name: "Lint", Passed: true},
			{Name: "Test", Passed: true},
		},
	}

	result := renderValidationSection(validation, 100)

	// Should behave like standard mode
	assert.Contains(t, result, "Validation:")
	assert.Contains(t, result, "passed")
	assert.Contains(t, result, "Format ✓")
}
