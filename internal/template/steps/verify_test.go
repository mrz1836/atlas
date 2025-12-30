// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// mockAIRunner implements ai.Runner for testing verification.
type mockVerifyRunner struct {
	runFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

func (m *mockVerifyRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, req)
	}
	return &domain.AIResult{
		Output:    `{"passed": true, "issues": [], "summary": "All checks passed"}`,
		SessionID: "test-session",
		NumTurns:  1,
	}, nil
}

func TestVerifyExecutor_Type(t *testing.T) {
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	assert.Equal(t, domain.StepTypeVerify, executor.Type())
}

func TestVerifyExecutor_Execute_Success(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			// Verify the prompt contains verification instructions
			assert.Contains(t, req.Prompt, "code reviewer")
			return &domain.AIResult{
				Output:    `{"passed": true, "issues": [], "summary": "All checks passed"}`,
				SessionID: "verify-session",
				NumTurns:  2,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Fix bug in user authentication",
		CurrentStep: 3,
	}

	step := &domain.StepDefinition{
		Name:        "verify",
		Type:        domain.StepTypeVerify,
		Description: "AI verification of implementation",
		Required:    false,
		Config: map[string]any{
			"model":  "gemini-3-pro",
			"checks": []string{"code_correctness", "garbage_files"},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "verify", result.StepName)
	assert.Equal(t, 3, result.StepIndex)
	assert.NotEmpty(t, result.Output)
}

func TestVerifyExecutor_Execute_WithIssues(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			// Return issues found
			return &domain.AIResult{
				Output: `{
					"passed": false,
					"issues": [
						{
							"severity": "warning",
							"category": "code_correctness",
							"file": "auth.go",
							"line": 42,
							"message": "Missing error handling",
							"suggestion": "Add error check after database call"
						}
					],
					"summary": "Found 1 issue to review"
				}`,
				SessionID: "verify-session",
				NumTurns:  3,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Add new feature",
		CurrentStep: 4,
	}

	step := &domain.StepDefinition{
		Name:     "verify",
		Type:     domain.StepTypeVerify,
		Required: true,
		Config: map[string]any{
			"checks": []string{"code_correctness", "test_coverage", "garbage_files", "security"},
		},
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	// Result should still be success since we return issues but let the handler decide
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Output, "issue")
}

func TestVerifyExecutor_Execute_AIError(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return nil, errors.ErrAIError
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Test task",
		CurrentStep: 1,
	}

	step := &domain.StepDefinition{
		Name: "verify",
		Type: domain.StepTypeVerify,
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "AI returned error")
}

func TestVerifyExecutor_Execute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Test task",
		CurrentStep: 0,
	}

	step := &domain.StepDefinition{
		Name: "verify",
		Type: domain.StepTypeVerify,
	}

	result, err := executor.Execute(ctx, task, step)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Nil(t, result)
}

func TestVerifyExecutor_Execute_WithTimeout(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{
		runFunc: func(execCtx context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			// Verify context has timeout
			deadline, ok := execCtx.Deadline()
			assert.True(t, ok, "context should have deadline")
			assert.True(t, deadline.After(time.Now()), "deadline should be in future")
			return &domain.AIResult{
				Output:    `{"passed": true, "issues": [], "summary": "OK"}`,
				SessionID: "test",
				NumTurns:  1,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Test task",
		CurrentStep: 0,
	}

	step := &domain.StepDefinition{
		Name:    "verify",
		Type:    domain.StepTypeVerify,
		Timeout: 5 * time.Minute,
	}

	result, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
}

func TestVerifyExecutor_Execute_ModelOverride(t *testing.T) {
	ctx := context.Background()
	var capturedReq *domain.AIRequest
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			capturedReq = req
			return &domain.AIResult{
				Output:    `{"passed": true, "issues": [], "summary": "OK"}`,
				SessionID: "test",
				NumTurns:  1,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	task := &domain.Task{
		ID:          "test-task",
		Description: "Test task",
		CurrentStep: 0,
		Config: domain.TaskConfig{
			Model: "claude-sonnet-4", // Default model
		},
	}

	step := &domain.StepDefinition{
		Name: "verify",
		Type: domain.StepTypeVerify,
		Config: map[string]any{
			"model": "gemini-3-pro", // Override for verification
		},
	}

	_, err := executor.Execute(ctx, task, step)

	require.NoError(t, err)
	require.NotNil(t, capturedReq)
	assert.Equal(t, "gemini-3-pro", capturedReq.Model)
}

func TestNewVerifyExecutor(t *testing.T) {
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	logger := zerolog.Nop()

	executor := NewVerifyExecutor(runner, detector, logger)

	require.NotNil(t, executor)
	assert.Equal(t, domain.StepTypeVerify, executor.Type())
}

func TestVerifyConfig_Defaults(t *testing.T) {
	config := DefaultVerifyConfig()

	assert.Contains(t, config.Checks, "code_correctness")
	assert.Contains(t, config.Checks, "garbage_files")
	assert.False(t, config.FailOnWarnings)
}

func TestVerifyExecutor_CheckCodeCorrectness(t *testing.T) {
	ctx := context.Background()

	t.Run("returns issues from AI", func(t *testing.T) {
		runner := &mockVerifyRunner{
			runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				assert.Contains(t, req.Prompt, "Review the following code changes")
				return &domain.AIResult{
					Output: `[{"severity": "warning", "file": "auth.go", "line": 10, "message": "Missing nil check", "suggestion": "Add nil check"}]`,
				}, nil
			},
		}
		detector := git.NewGarbageDetector(nil)
		executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

		files := []ChangedFile{
			{Path: "auth.go", Language: "go", Content: "package auth\n\nfunc Auth() {}"},
		}

		issues, err := executor.CheckCodeCorrectness(ctx, "Add authentication", files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "warning", issues[0].Severity)
		assert.Equal(t, "code_correctness", issues[0].Category)
		assert.Equal(t, "auth.go", issues[0].File)
	})

	t.Run("handles empty files", func(t *testing.T) {
		runner := &mockVerifyRunner{}
		detector := git.NewGarbageDetector(nil)
		executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

		issues, err := executor.CheckCodeCorrectness(ctx, "Task", nil)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})
}

func TestVerifyExecutor_CheckTestCoverage(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	t.Run("detects missing test files", func(t *testing.T) {
		files := []ChangedFile{
			{Path: "internal/auth/login.go", Content: "package auth"},
		}

		issues, err := executor.CheckTestCoverage(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "warning", issues[0].Severity)
		assert.Equal(t, "test_coverage", issues[0].Category)
		assert.Contains(t, issues[0].Suggestion, "login_test.go")
	})

	t.Run("no issues when test file exists", func(t *testing.T) {
		files := []ChangedFile{
			{Path: "internal/auth/login.go", Content: "package auth"},
			{Path: "internal/auth/login_test.go", Content: "package auth"},
		}

		issues, err := executor.CheckTestCoverage(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})

	t.Run("skips test files themselves", func(t *testing.T) {
		files := []ChangedFile{
			{Path: "internal/auth/login_test.go", Content: "package auth"},
		}

		issues, err := executor.CheckTestCoverage(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})

	t.Run("skips non-Go files", func(t *testing.T) {
		files := []ChangedFile{
			{Path: "README.md", Content: "# Readme"},
		}

		issues, err := executor.CheckTestCoverage(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})
}

func TestVerifyExecutor_CheckGarbageFiles(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	t.Run("detects debug files", func(t *testing.T) {
		files := []string{"__debug_bin123", "main.go"}

		issues, err := executor.CheckGarbageFiles(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "warning", issues[0].Severity)
		assert.Equal(t, "garbage", issues[0].Category)
		assert.Equal(t, "__debug_bin123", issues[0].File)
	})

	t.Run("detects secret files as error", func(t *testing.T) {
		files := []string{".env", "main.go"}

		issues, err := executor.CheckGarbageFiles(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "error", issues[0].Severity)
		assert.Equal(t, "garbage", issues[0].Category)
	})

	t.Run("detects temp files", func(t *testing.T) {
		files := []string{"file.tmp", "backup.bak"}

		issues, err := executor.CheckGarbageFiles(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 2)
	})

	t.Run("no issues for clean files", func(t *testing.T) {
		files := []string{"main.go", "handler.go", "main_test.go"}

		issues, err := executor.CheckGarbageFiles(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})
}

func TestVerifyExecutor_CheckSecurityIssues(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	t.Run("detects hardcoded password", func(t *testing.T) {
		files := []ChangedFile{
			{
				Path:    "config.go",
				Content: `password := "secret123"`,
			},
		}

		issues, err := executor.CheckSecurityIssues(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "error", issues[0].Severity)
		assert.Equal(t, "security", issues[0].Category)
		assert.Contains(t, issues[0].Message, "password")
	})

	t.Run("detects hardcoded API key", func(t *testing.T) {
		files := []ChangedFile{
			{
				Path:    "client.go",
				Content: `api_key = "EXAMPLE_KEY_VALUE"`,
			},
		}

		issues, err := executor.CheckSecurityIssues(ctx, files)

		require.NoError(t, err)
		require.Len(t, issues, 1)
		assert.Equal(t, "error", issues[0].Severity)
		assert.Contains(t, issues[0].Message, "secret")
	})

	t.Run("skips test files", func(t *testing.T) {
		files := []ChangedFile{
			{
				Path:    "config_test.go",
				Content: `password := "test_password"`, // OK in test files
			},
		}

		issues, err := executor.CheckSecurityIssues(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})

	t.Run("no issues for clean code", func(t *testing.T) {
		files := []ChangedFile{
			{
				Path:    "handler.go",
				Content: `password := os.Getenv("PASSWORD")`, // Safe - from env
			},
		}

		issues, err := executor.CheckSecurityIssues(ctx, files)

		require.NoError(t, err)
		assert.Empty(t, issues)
	})
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", false},
		{"main_test.go", true},
		{"internal/auth/login.go", false},
		{"internal/auth/login_test.go", true},
		{"testdata/fixtures.json", false},     // testdata without /testdata/ path
		{"internal/testdata/sample.go", true}, // contains /testdata/
		{"pkg/testdata/fixtures.json", true},  // contains /testdata/
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.expected, isTestFile(tc.path))
		})
	}
}

func TestIsGoFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"main_test.go", true},
		{"README.md", false},
		{"config.yaml", false},
		{".env", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.expected, isGoFile(tc.path))
		})
	}
}

func TestToTestFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main.go", "main_test.go"},
		{"internal/auth/login.go", "internal/auth/login_test.go"},
		{"handler.go", "handler_test.go"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, toTestFileName(tc.input))
		})
	}
}

func TestGenerateVerificationReport(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Second)

	t.Run("with no issues", func(t *testing.T) {
		report := GenerateVerificationReport(
			"task-123",
			"Fix login bug",
			nil,
			[]string{"code_correctness", "garbage_files"},
			nil,
			startTime,
		)

		assert.Equal(t, "task-123", report.TaskID)
		assert.Equal(t, "Fix login bug", report.TaskDesc)
		assert.Equal(t, 0, report.TotalIssues)
		assert.Equal(t, 0, report.ErrorCount)
		assert.Equal(t, 0, report.WarningCount)
		assert.Contains(t, report.Summary, "passed successfully")
		assert.Len(t, report.PassedChecks, 2)
	})

	t.Run("with issues", func(t *testing.T) {
		issues := []VerificationIssue{
			{Severity: "error", Category: "security", Message: "Hardcoded secret"},
			{Severity: "warning", Category: "test_coverage", Message: "Missing tests"},
			{Severity: "warning", Category: "code_correctness", Message: "Logic issue"},
			{Severity: "info", Category: "style", Message: "Naming convention"},
		}

		report := GenerateVerificationReport(
			"task-456",
			"Add new feature",
			issues,
			[]string{"garbage_files"},
			[]string{"security", "test_coverage"},
			startTime,
		)

		assert.Equal(t, 4, report.TotalIssues)
		assert.Equal(t, 1, report.ErrorCount)
		assert.Equal(t, 2, report.WarningCount)
		assert.Equal(t, 1, report.InfoCount)
		assert.Contains(t, report.Summary, "4 issue(s)")
		assert.Len(t, report.FailedChecks, 2)
	})
}

func TestSaveVerificationReport(t *testing.T) {
	tmpDir := t.TempDir()

	report := &VerificationReport{
		TaskID:       "test-task",
		TaskDesc:     "Test task description",
		Summary:      "Test summary",
		TotalIssues:  1,
		ErrorCount:   1,
		WarningCount: 0,
		InfoCount:    0,
		Issues: []VerificationIssue{
			{
				Severity:   "error",
				Category:   "security",
				File:       "config.go",
				Line:       42,
				Message:    "Hardcoded password",
				Suggestion: "Use environment variable",
			},
		},
		PassedChecks: []string{"garbage_files"},
		FailedChecks: []string{"security"},
		Timestamp:    time.Now(),
		Duration:     5 * time.Second,
	}

	path, err := SaveVerificationReport(report, tmpDir)

	require.NoError(t, err)
	assert.Contains(t, path, "verification-report.md")

	// Verify file contents
	content, err := os.ReadFile(path) //nolint:gosec // Reading test artifact file
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "# Verification Report")
	assert.Contains(t, contentStr, "test-task")
	assert.Contains(t, contentStr, "Test task description")
	assert.Contains(t, contentStr, "security")
	assert.Contains(t, contentStr, "Hardcoded password")
	assert.Contains(t, contentStr, "config.go:42")
}

func TestFormatReportAsMarkdown(t *testing.T) {
	report := &VerificationReport{
		TaskID:       "task-789",
		TaskDesc:     "Implement feature X",
		Summary:      "Found 2 issues",
		TotalIssues:  2,
		ErrorCount:   1,
		WarningCount: 1,
		InfoCount:    0,
		Issues: []VerificationIssue{
			{Severity: "error", Category: "security", File: "auth.go", Line: 10, Message: "Bad", Suggestion: "Fix it"},
			{Severity: "warning", Category: "test_coverage", File: "handler.go", Message: "Missing test"},
		},
		PassedChecks: []string{"garbage_files"},
		FailedChecks: []string{"security", "test_coverage"},
		Timestamp:    time.Date(2025, 12, 30, 10, 0, 0, 0, time.UTC),
		Duration:     3 * time.Second,
	}

	md := formatReportAsMarkdown(report)

	assert.Contains(t, md, "# Verification Report")
	assert.Contains(t, md, "task-789")
	assert.Contains(t, md, "## Summary")
	assert.Contains(t, md, "### Statistics")
	assert.Contains(t, md, "### Passed Checks")
	assert.Contains(t, md, "✅ garbage_files")
	assert.Contains(t, md, "### Failed Checks")
	assert.Contains(t, md, "❌ security")
	assert.Contains(t, md, "## Issues")
	assert.Contains(t, md, "### Errors")
	assert.Contains(t, md, "### Warnings")
	assert.Contains(t, md, "auth.go:10")
	assert.Contains(t, md, "handler.go")
}

func TestFilterIssuesBySeverity(t *testing.T) {
	issues := []VerificationIssue{
		{Severity: "error", Message: "Error 1"},
		{Severity: "warning", Message: "Warning 1"},
		{Severity: "error", Message: "Error 2"},
		{Severity: "info", Message: "Info 1"},
	}

	errors := filterIssuesBySeverity(issues, "error")
	warnings := filterIssuesBySeverity(issues, "warning")
	infos := filterIssuesBySeverity(issues, "info")

	assert.Len(t, errors, 2)
	assert.Len(t, warnings, 1)
	assert.Len(t, infos, 1)
}

func TestVerificationAction_String(t *testing.T) {
	tests := []struct {
		action   VerificationAction
		expected string
	}{
		{VerifyActionAutoFix, "auto_fix"},
		{VerifyActionManualFix, "manual_fix"},
		{VerifyActionIgnoreContinue, "ignore_continue"},
		{VerifyActionViewReport, "view_report"},
		{VerificationAction(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.action.String())
		})
	}
}

func TestVerifyExecutor_RunAllChecks(t *testing.T) {
	// Integration test that verifies all check types work together
	ctx := context.Background()
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			// AI check returns issues
			return &domain.AIResult{
				Output: `[{"severity": "warning", "file": "handler.go", "line": 10, "message": "Missing error handling", "suggestion": "Add err check"}]`,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	// Test code correctness check
	changedFiles := []ChangedFile{
		{Path: "handler.go", Language: "go", Content: "package main\n\nfunc handle() {}"},
	}
	issues, err := executor.CheckCodeCorrectness(ctx, "Handle requests", changedFiles)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Equal(t, "code_correctness", issues[0].Category)

	// Test test coverage check
	coverageIssues, err := executor.CheckTestCoverage(ctx, changedFiles)
	require.NoError(t, err)
	assert.Len(t, coverageIssues, 1)
	assert.Equal(t, "test_coverage", coverageIssues[0].Category)

	// Test garbage files check
	stagedFiles := []string{"handler.go", "__debug_bin123", ".env.local"}
	garbageIssues, err := executor.CheckGarbageFiles(ctx, stagedFiles)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(garbageIssues), 1)

	// Test security check
	securityFiles := []ChangedFile{
		{Path: "config.go", Content: `apiKey := "sk-secret123"`},
	}
	securityIssues, err := executor.CheckSecurityIssues(ctx, securityFiles)
	require.NoError(t, err)
	assert.Len(t, securityIssues, 1)
	assert.Equal(t, "security", securityIssues[0].Category)
}

func TestVerifyExecutor_CompleteWorkflow(t *testing.T) {
	// End-to-end workflow test
	ctx := context.Background()

	var capturedPrompt string
	runner := &mockVerifyRunner{
		runFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
			capturedPrompt = req.Prompt
			return &domain.AIResult{
				Output:    `{"passed": false, "issues": [{"severity": "warning", "category": "code_correctness", "file": "main.go", "line": 42, "message": "Consider using defer for cleanup", "suggestion": "Add defer statement"}], "summary": "Found 1 issue"}`,
				SessionID: "verify-session-123",
				NumTurns:  2,
			}, nil
		},
	}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	// Create a task
	task := &domain.Task{
		ID:          "task-workflow-test",
		Description: "Implement user login feature",
		CurrentStep: 2,
		Config: domain.TaskConfig{
			Model: "claude-sonnet-4",
		},
	}

	// Create step definition
	step := &domain.StepDefinition{
		Name:        "verify",
		Type:        domain.StepTypeVerify,
		Description: "Verify implementation",
		Required:    true,
		Timeout:     5 * time.Minute,
		Config: map[string]any{
			"model":  "gemini-3-pro", // Override model for verification
			"checks": []string{"code_correctness", "test_coverage", "garbage_files", "security"},
		},
	}

	// Execute verification
	result, err := executor.Execute(ctx, task, step)
	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, 2, result.StepIndex)
	assert.Contains(t, result.Output, "issue")

	// Verify prompt was built correctly
	assert.Contains(t, capturedPrompt, "code reviewer")
	assert.Contains(t, capturedPrompt, "Implement user login feature")
}

func TestVerifyExecutor_HandleVerificationIssues(t *testing.T) {
	ctx := context.Background()
	runner := &mockVerifyRunner{}
	detector := git.NewGarbageDetector(nil)
	executor := NewVerifyExecutor(runner, detector, zerolog.Nop())

	report := &VerificationReport{
		TaskID:       "test-task",
		TaskDesc:     "Test task",
		TotalIssues:  2,
		ErrorCount:   1,
		WarningCount: 1,
		Issues: []VerificationIssue{
			{Severity: "error", Category: "security", File: "auth.go", Message: "Bad"},
			{Severity: "warning", Category: "test_coverage", File: "handler.go", Message: "Missing test"},
		},
	}

	t.Run("auto fix success", func(t *testing.T) {
		runner.runFunc = func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return &domain.AIResult{
				SessionID:    "fix-session",
				FilesChanged: []string{"auth.go", "handler.go"},
			}, nil
		}

		result, err := executor.HandleVerificationIssues(ctx, report, VerifyActionAutoFix)

		require.NoError(t, err)
		assert.True(t, result.ShouldContinue)
		assert.True(t, result.AutoFixAttempted)
		assert.True(t, result.AutoFixSuccess)
		assert.Contains(t, result.Message, "2 files modified")
	})

	t.Run("auto fix failure", func(t *testing.T) {
		runner.runFunc = func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
			return nil, errors.ErrAIError
		}

		result, err := executor.HandleVerificationIssues(ctx, report, VerifyActionAutoFix)

		require.NoError(t, err)
		assert.False(t, result.ShouldContinue)
		assert.True(t, result.AutoFixAttempted)
		assert.False(t, result.AutoFixSuccess)
		assert.Contains(t, result.Message, "failed")
	})

	t.Run("manual fix", func(t *testing.T) {
		result, err := executor.HandleVerificationIssues(ctx, report, VerifyActionManualFix)

		require.NoError(t, err)
		assert.False(t, result.ShouldContinue)
		assert.True(t, result.AwaitingManualFix)
		assert.Equal(t, VerifyActionManualFix, result.Action)
	})

	t.Run("ignore and continue", func(t *testing.T) {
		result, err := executor.HandleVerificationIssues(ctx, report, VerifyActionIgnoreContinue)

		require.NoError(t, err)
		assert.True(t, result.ShouldContinue)
		assert.Contains(t, result.Message, "Ignoring")
		assert.Contains(t, result.Message, "error(s) ignored")
	})

	t.Run("view report", func(t *testing.T) {
		result, err := executor.HandleVerificationIssues(ctx, report, VerifyActionViewReport)

		require.NoError(t, err)
		assert.False(t, result.ShouldContinue)
		assert.Contains(t, result.Message, "# Verification Report")
	})

	t.Run("unknown action", func(t *testing.T) {
		_, err := executor.HandleVerificationIssues(ctx, report, VerificationAction(99))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification action")
	})
}
