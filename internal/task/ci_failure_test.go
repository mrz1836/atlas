package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// MockHubRunner implements git.HubRunner for testing.
type MockHubRunner struct {
	CreatePRFunc       func(ctx context.Context, opts git.PRCreateOptions) (*git.PRResult, error)
	GetPRStatusFunc    func(ctx context.Context, prNumber int) (*git.PRStatus, error)
	WatchPRChecksFunc  func(ctx context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error)
	ConvertToDraftFunc func(ctx context.Context, prNumber int) error
	MergePRFunc        func(ctx context.Context, prNumber int, mergeMethod string, adminBypass, deleteBranch bool) error
	AddPRReviewFunc    func(ctx context.Context, prNumber int, body, event string) error
	AddPRCommentFunc   func(ctx context.Context, prNumber int, body string) error
}

func (m *MockHubRunner) CreatePR(ctx context.Context, opts git.PRCreateOptions) (*git.PRResult, error) {
	if m.CreatePRFunc != nil {
		return m.CreatePRFunc(ctx, opts)
	}
	return nil, fmt.Errorf("CreatePR not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) GetPRStatus(ctx context.Context, prNumber int) (*git.PRStatus, error) {
	if m.GetPRStatusFunc != nil {
		return m.GetPRStatusFunc(ctx, prNumber)
	}
	return nil, fmt.Errorf("GetPRStatus not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) WatchPRChecks(ctx context.Context, opts git.CIWatchOptions) (*git.CIWatchResult, error) {
	if m.WatchPRChecksFunc != nil {
		return m.WatchPRChecksFunc(ctx, opts)
	}
	return nil, fmt.Errorf("WatchPRChecks not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) ConvertToDraft(ctx context.Context, prNumber int) error {
	if m.ConvertToDraftFunc != nil {
		return m.ConvertToDraftFunc(ctx, prNumber)
	}
	return fmt.Errorf("ConvertToDraft not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) MergePR(ctx context.Context, prNumber int, mergeMethod string, adminBypass, deleteBranch bool) error {
	if m.MergePRFunc != nil {
		return m.MergePRFunc(ctx, prNumber, mergeMethod, adminBypass, deleteBranch)
	}
	return fmt.Errorf("MergePR not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) AddPRReview(ctx context.Context, prNumber int, body, event string) error {
	if m.AddPRReviewFunc != nil {
		return m.AddPRReviewFunc(ctx, prNumber, body, event)
	}
	return fmt.Errorf("AddPRReview not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func (m *MockHubRunner) AddPRComment(ctx context.Context, prNumber int, body string) error {
	if m.AddPRCommentFunc != nil {
		return m.AddPRCommentFunc(ctx, prNumber, body)
	}
	return fmt.Errorf("AddPRComment not implemented: %w", atlaserrors.ErrCommandNotConfigured)
}

func TestCIFailureAction_String(t *testing.T) {
	tests := []struct {
		action   CIFailureAction
		expected string
	}{
		{CIFailureViewLogs, "view_logs"},
		{CIFailureRetryImplement, "retry_implement"},
		{CIFailureFixManually, "fix_manually"},
		{CIFailureAbandon, "abandon"},
		{CIFailureAction(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.action.String())
		})
	}
}

func TestNewCIFailureHandler(t *testing.T) {
	t.Run("creates handler with defaults", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)
		require.NotNil(t, handler)
		assert.Nil(t, handler.hubRunner)
		assert.NotNil(t, handler.browserOpener)
	})

	t.Run("creates handler with HubRunner", func(t *testing.T) {
		mockHub := &MockHubRunner{}
		handler := NewCIFailureHandler(mockHub)
		require.NotNil(t, handler)
		assert.Equal(t, mockHub, handler.hubRunner)
	})

	t.Run("applies options", func(t *testing.T) {
		logger := zerolog.New(os.Stderr)
		customOpener := func(_ string) error { return nil }

		handler := NewCIFailureHandler(
			nil,
			WithCIFailureLogger(logger),
			WithBrowserOpener(customOpener),
		)

		require.NotNil(t, handler)
		assert.NotNil(t, handler.browserOpener)
	})
}

func TestCIFailureHandler_HandleCIFailure_ViewLogs(t *testing.T) {
	t.Run("opens browser with check URL", func(t *testing.T) {
		openCalled := false
		openURL := ""
		mockOpener := func(url string) error {
			openCalled = true
			openURL = url
			return nil
		}

		handler := NewCIFailureHandler(nil, WithBrowserOpener(mockOpener))

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action: CIFailureViewLogs,
			CIResult: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail", URL: "https://github.com/owner/repo/actions/runs/123"},
				},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, openCalled)
		assert.Equal(t, "https://github.com/owner/repo/actions/runs/123", openURL)
		assert.Equal(t, CIFailureViewLogs, result.Action)
		assert.Contains(t, result.Message, "https://github.com/owner/repo/actions/runs/123")
	})

	t.Run("returns error when no URL available", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action: CIFailureViewLogs,
			CIResult: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail", URL: ""},
				},
			},
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("returns error when browser open fails", func(t *testing.T) {
		mockOpener := func(_ string) error {
			return fmt.Errorf("browser not found: %w", atlaserrors.ErrGitOperation)
		}

		handler := NewCIFailureHandler(nil, WithBrowserOpener(mockOpener))

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action: CIFailureViewLogs,
			CIResult: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail", URL: "https://example.com"},
				},
			},
		})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to open browser")
	})
}

func TestCIFailureHandler_HandleCIFailure_RetryImplement(t *testing.T) {
	t.Run("extracts error context for AI", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:   CIFailureRetryImplement,
			PRNumber: 42,
			CIResult: &git.CIWatchResult{
				Status: git.CIStatusFailure,
				CheckResults: []git.CheckResult{
					{Name: "CI / lint", Bucket: "fail", URL: "https://github.com/actions/lint"},
					{Name: "CI / test", Bucket: "pass"},
				},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CIFailureRetryImplement, result.Action)
		assert.Equal(t, "implement", result.NextStep)
		assert.Contains(t, result.ErrorContext, "CI / lint")
		assert.Contains(t, result.ErrorContext, "fail")
		assert.NotContains(t, result.ErrorContext, "CI / test")
	})
}

func TestCIFailureHandler_HandleCIFailure_FixManually(t *testing.T) {
	t.Run("formats manual fix instructions", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:        CIFailureFixManually,
			WorktreePath:  "/path/to/worktree",
			WorkspaceName: "fix-bug",
			CIResult: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail"},
				},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CIFailureFixManually, result.Action)
		assert.Contains(t, result.Message, "/path/to/worktree")
		assert.Contains(t, result.Message, "atlas resume fix-bug")
	})
}

func TestCIFailureHandler_HandleCIFailure_Abandon(t *testing.T) {
	t.Run("converts PR to draft and abandons", func(t *testing.T) {
		draftCalled := false
		mockHub := &MockHubRunner{
			ConvertToDraftFunc: func(_ context.Context, prNumber int) error {
				draftCalled = true
				assert.Equal(t, 42, prNumber)
				return nil
			},
		}

		handler := NewCIFailureHandler(mockHub)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:   CIFailureAbandon,
			PRNumber: 42,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, draftCalled)
		assert.Equal(t, CIFailureAbandon, result.Action)
		assert.Contains(t, result.Message, "PR #42")
		assert.Contains(t, result.Message, "draft")
	})

	t.Run("continues even if draft conversion fails", func(t *testing.T) {
		mockHub := &MockHubRunner{
			ConvertToDraftFunc: func(_ context.Context, _ int) error {
				return fmt.Errorf("conversion failed: %w", atlaserrors.ErrGitHubOperation)
			},
		}

		handler := NewCIFailureHandler(mockHub)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:   CIFailureAbandon,
			PRNumber: 42,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CIFailureAbandon, result.Action)
	})

	t.Run("works without HubRunner", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:   CIFailureAbandon,
			PRNumber: 42,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, CIFailureAbandon, result.Action)
	})
}

func TestCIFailureHandler_HandleCIFailure_ContextCancellation(t *testing.T) {
	handler := NewCIFailureHandler(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := handler.HandleCIFailure(ctx, CIFailureOptions{
		Action: CIFailureViewLogs,
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCIFailureHandler_HandleCIFailure_UnknownAction(t *testing.T) {
	handler := NewCIFailureHandler(nil)

	result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
		Action: CIFailureAction(99), // Unknown action
	})

	require.Error(t, err)
	assert.Nil(t, result)
	require.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	assert.Contains(t, err.Error(), "unknown CI failure action")
}

func TestCIFailureHandler_HandleCIFailure_AutoSavesArtifact(t *testing.T) {
	t.Run("saves artifact when ArtifactDir provided", func(t *testing.T) {
		dir := t.TempDir()
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:        CIFailureRetryImplement,
			ArtifactDir:   dir,
			WorkspaceName: "test-workspace",
			CIResult: &git.CIWatchResult{
				Status: git.CIStatusFailure,
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail"},
				},
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.ArtifactPath)
		assert.FileExists(t, result.ArtifactPath)
	})

	t.Run("skips artifact when ArtifactDir empty", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action: CIFailureRetryImplement,
			// ArtifactDir not set
			CIResult: &git.CIWatchResult{
				Status: git.CIStatusFailure,
			},
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.ArtifactPath)
	})

	t.Run("skips artifact when CIResult nil", func(t *testing.T) {
		dir := t.TempDir()
		handler := NewCIFailureHandler(nil)

		result, err := handler.HandleCIFailure(context.Background(), CIFailureOptions{
			Action:      CIFailureRetryImplement,
			ArtifactDir: dir,
			// CIResult not set
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.ArtifactPath)
	})
}

func TestCIFailureHandler_SaveCIResultArtifact(t *testing.T) {
	t.Run("saves artifact file with correct content", func(t *testing.T) {
		dir := t.TempDir()
		handler := NewCIFailureHandler(nil)

		ciResult := &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: 5 * time.Minute,
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "fail", State: "FAILURE", URL: "https://example.com/lint"},
				{Name: "CI / test", Bucket: "pass", State: "SUCCESS"},
			},
			Error: atlaserrors.ErrCIFailed,
		}

		path, err := handler.SaveCIResultArtifact(context.Background(), ciResult, dir)

		require.NoError(t, err)
		assert.FileExists(t, path)
		assert.Equal(t, filepath.Join(dir, "ci-result.json"), path)

		// Verify JSON content
		data, readErr := os.ReadFile(path) //#nosec G304 -- test file only
		require.NoError(t, readErr)

		var artifact domain.CIResultArtifact
		require.NoError(t, json.Unmarshal(data, &artifact))

		assert.Equal(t, "failure", artifact.Status)
		assert.Equal(t, "5m0s", artifact.ElapsedTime)
		assert.Len(t, artifact.AllChecks, 2)
		assert.Len(t, artifact.FailedChecks, 1)
		assert.Equal(t, "CI / lint", artifact.FailedChecks[0].Name)
		assert.Contains(t, artifact.ErrorMessage, "ci workflow failed")
		assert.NotEmpty(t, artifact.Timestamp)
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "dir")
		handler := NewCIFailureHandler(nil)

		ciResult := &git.CIWatchResult{
			Status: git.CIStatusTimeout,
		}

		path, err := handler.SaveCIResultArtifact(context.Background(), ciResult, dir)

		require.NoError(t, err)
		assert.FileExists(t, path)
	})

	t.Run("returns error for nil result", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		path, err := handler.SaveCIResultArtifact(context.Background(), nil, t.TempDir())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("returns error for empty directory", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		path, err := handler.SaveCIResultArtifact(context.Background(), &git.CIWatchResult{}, "")

		require.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		handler := NewCIFailureHandler(nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		path, err := handler.SaveCIResultArtifact(ctx, &git.CIWatchResult{}, t.TempDir())

		require.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestExtractCIErrorContext(t *testing.T) {
	tests := []struct {
		name     string
		result   *git.CIWatchResult
		contains []string
	}{
		{
			name:     "nil result",
			result:   nil,
			contains: []string{"no details available"},
		},
		{
			name: "empty check results",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{},
			},
			contains: []string{"no details available"},
		},
		{
			name: "single failure",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI / lint", Bucket: "fail", URL: "https://example.com"},
				},
			},
			contains: []string{"CI / lint", "fail", "https://example.com"},
		},
		{
			name: "failure with workflow",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "lint", Bucket: "fail", Workflow: "CI"},
				},
			},
			contains: []string{"lint", "Workflow: CI"},
		},
		{
			name: "multiple with pass and fail",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI / lint", Bucket: "pass"},
					{Name: "CI / test", Bucket: "fail"},
				},
			},
			contains: []string{"CI / test"},
		},
		{
			name: "cancel treated as failure",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "cancel"},
				},
			},
			contains: []string{"CI", "cancel"},
		},
		{
			name: "all passing - no specific failures",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "pass"},
				},
			},
			contains: []string{"No specific failures identified"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := ExtractCIErrorContext(tt.result)
			for _, expected := range tt.contains {
				assert.Contains(t, ctx, expected)
			}
		})
	}
}

func TestFormatManualFixInstructions(t *testing.T) {
	t.Run("formats instructions with failed checks", func(t *testing.T) {
		result := &git.CIWatchResult{
			CheckResults: []git.CheckResult{
				{Name: "CI / lint", Bucket: "fail", URL: "https://example.com/lint"},
				{Name: "CI / test", Bucket: "pass"},
			},
		}

		instructions := FormatManualFixInstructions("/work/tree", "my-workspace", result)

		assert.Contains(t, instructions, "cd /work/tree")
		assert.Contains(t, instructions, "CI / lint")
		assert.Contains(t, instructions, "https://example.com/lint")
		assert.Contains(t, instructions, "atlas resume my-workspace")
		assert.Contains(t, instructions, "git add -A")
		assert.Contains(t, instructions, "git commit")
		assert.Contains(t, instructions, "git push")
	})

	t.Run("handles nil result", func(t *testing.T) {
		instructions := FormatManualFixInstructions("/work/tree", "workspace", nil)

		assert.Contains(t, instructions, "cd /work/tree")
		assert.Contains(t, instructions, "atlas resume workspace")
		assert.NotContains(t, instructions, "Failed Checks")
	})

	t.Run("handles empty check results", func(t *testing.T) {
		result := &git.CIWatchResult{
			CheckResults: []git.CheckResult{},
		}

		instructions := FormatManualFixInstructions("/work", "ws", result)

		assert.Contains(t, instructions, "cd /work")
	})
}

func TestCIFailureHandler_extractBestCheckURL(t *testing.T) {
	handler := NewCIFailureHandler(nil)

	tests := []struct {
		name     string
		result   *git.CIWatchResult
		expected string
	}{
		{
			name:     "nil result",
			result:   nil,
			expected: "",
		},
		{
			name: "empty checks",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{},
			},
			expected: "",
		},
		{
			name: "failed check with URL",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail", URL: "https://failed.example.com"},
				},
			},
			expected: "https://failed.example.com",
		},
		{
			name: "prefers failed check URL over passing",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "pass", Bucket: "pass", URL: "https://pass.example.com"},
					{Name: "fail", Bucket: "fail", URL: "https://fail.example.com"},
				},
			},
			expected: "https://fail.example.com",
		},
		{
			name: "falls back to any URL if no failed",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "pending", Bucket: "pending", URL: "https://pending.example.com"},
				},
			},
			expected: "https://pending.example.com",
		},
		{
			name: "no URLs available",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "fail", URL: ""},
				},
			},
			expected: "",
		},
		{
			name: "cancel bucket treated as failure",
			result: &git.CIWatchResult{
				CheckResults: []git.CheckResult{
					{Name: "CI", Bucket: "cancel", URL: "https://cancel.example.com"},
				},
			},
			expected: "https://cancel.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := handler.extractBestCheckURL(tt.result)
			assert.Equal(t, tt.expected, url)
		})
	}
}

func TestCIFailureHandler_buildCIResultArtifact(t *testing.T) {
	handler := NewCIFailureHandler(nil)

	t.Run("builds artifact with all fields", func(t *testing.T) {
		result := &git.CIWatchResult{
			Status:      git.CIStatusFailure,
			ElapsedTime: 10 * time.Minute,
			CheckResults: []git.CheckResult{
				{
					Name:     "CI / lint",
					State:    "FAILURE",
					Bucket:   "fail",
					URL:      "https://example.com",
					Duration: 2 * time.Minute,
					Workflow: "CI",
				},
				{
					Name:   "CI / test",
					State:  "SUCCESS",
					Bucket: "pass",
				},
			},
			Error: atlaserrors.ErrCIFailed,
		}

		artifact := handler.buildCIResultArtifact(result)

		assert.Equal(t, "failure", artifact.Status)
		assert.Equal(t, "10m0s", artifact.ElapsedTime)
		assert.Equal(t, "ci workflow failed", artifact.ErrorMessage)
		assert.NotEmpty(t, artifact.Timestamp)
		assert.Len(t, artifact.AllChecks, 2)
		assert.Len(t, artifact.FailedChecks, 1)

		// Verify failed check
		assert.Equal(t, "CI / lint", artifact.FailedChecks[0].Name)
		assert.Equal(t, "fail", artifact.FailedChecks[0].Bucket)
		assert.Equal(t, "2m0s", artifact.FailedChecks[0].Duration)
	})

	t.Run("handles nil error", func(t *testing.T) {
		result := &git.CIWatchResult{
			Status: git.CIStatusSuccess,
			Error:  nil,
		}

		artifact := handler.buildCIResultArtifact(result)

		assert.Empty(t, artifact.ErrorMessage)
	})

	t.Run("handles zero duration", func(t *testing.T) {
		result := &git.CIWatchResult{
			Status: git.CIStatusFailure,
			CheckResults: []git.CheckResult{
				{Name: "CI", Bucket: "fail", Duration: 0},
			},
		}

		artifact := handler.buildCIResultArtifact(result)

		assert.Empty(t, artifact.FailedChecks[0].Duration)
	})
}

func TestOpenInBrowser_ValidOS(_ *testing.T) {
	// Test that openInBrowser doesn't panic on current OS
	// We can't actually verify the browser opens, but we can verify
	// the function executes without error for supported OSes

	err := openInBrowser("https://example.com")

	// The error might occur if the browser command doesn't exist,
	// but the function itself should work correctly for the OS check
	// On CI systems, the browser command might not exist, so we just
	// verify no panic occurs
	_ = err
}
