package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/contracts"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewBacklogPromoteCmd(t *testing.T) {
	t.Parallel()

	cmd := newBacklogPromoteCmd()

	t.Run("has correct use and short", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "promote [id]", cmd.Use)
		assert.NotEmpty(t, cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("has all flags", func(t *testing.T) {
		t.Parallel()
		flags := []string{"template", "ai", "agent", "model", "dry-run", "json"}
		for _, flag := range flags {
			f := cmd.Flags().Lookup(flag)
			assert.NotNil(t, f, "flag %s should exist", flag)
		}
	})

	t.Run("template flag has shorthand", func(t *testing.T) {
		t.Parallel()
		f := cmd.Flags().Lookup("template")
		assert.Equal(t, "t", f.Shorthand)
	})
}

func TestRunBacklogPromote_DryRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Dry Run Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityHigh,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Dry-run")
	assert.Contains(t, output, "bugfix") // Bug category maps to bugfix

	// Verify discovery was not modified
	got, err := mgr.Get(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, backlog.StatusPending, got.Status)
}

func TestRunBacklogPromote_JSONOutput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "JSON Output Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategorySecurity,
			Severity: backlog.SeverityCritical,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()
	// Add global output flag
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer

	opts := promoteOptions{
		jsonOutput:  true,
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.True(t, result["success"].(bool))
	assert.Equal(t, d.ID, result["id"])
	assert.Equal(t, "hotfix", result["template"]) // Critical security -> hotfix
	assert.True(t, result["dry_run"].(bool))
}

func TestRunBacklogPromote_TemplateOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Template Override Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityLow,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		template:    "feature", // Override to feature instead of bugfix
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "feature")
	assert.Contains(t, output, "feat/") // Feature prefix
}

func TestRunBacklogPromote_InvalidTemplate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		template: "invalid-template",
	}

	err := runBacklogPromote(ctx, cmd, &buf, "disc-abc123", opts)
	require.Error(t, err)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
	assert.Contains(t, err.Error(), "invalid template")
}

func TestRunBacklogPromote_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		projectRoot: tmpDir,
	}

	err := runBacklogPromote(ctx, cmd, &buf, "disc-notfnd", opts)
	require.Error(t, err)
	// Not found errors go through outputBacklogError
}

func TestRunBacklogPromote_AlreadyPromoted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Already Promoted",
		Status: backlog.StatusPromoted,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityLow,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
		Lifecycle: backlog.Lifecycle{
			PromotedToTask: "task-old",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.Error(t, err)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
	assert.Contains(t, err.Error(), "invalid status transition")
}

func TestTruncateDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		desc     string
		maxLen   int
		expected string
	}{
		{
			name:     "short description unchanged",
			desc:     "Fix bug",
			maxLen:   50,
			expected: "Fix bug",
		},
		{
			name:     "long description truncated",
			desc:     "This is a very long description that needs to be truncated",
			maxLen:   20,
			expected: "This is a very lo...",
		},
		{
			name:     "multiline takes first line",
			desc:     "First line\nSecond line\nThird line",
			maxLen:   50,
			expected: "First line",
		},
		{
			name:     "multiline truncated",
			desc:     "This is a very long first line\nSecond line",
			maxLen:   20,
			expected: "This is a very lo...",
		},
		{
			name:     "empty description",
			desc:     "",
			maxLen:   50,
			expected: "",
		},
		{
			name:     "exact length",
			desc:     "12345",
			maxLen:   5,
			expected: "12345",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := truncateDescription(tc.desc, tc.maxLen)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestPromoteOptions_AllCategories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Test that all category/severity combinations produce valid results
	categories := backlog.ValidCategories()
	severities := backlog.ValidSeverities()

	for _, cat := range categories {
		for _, sev := range severities {
			t.Run(string(cat)+"_"+string(sev), func(t *testing.T) {
				t.Parallel()

				tmpDir := t.TempDir()
				mgr, err := backlog.NewManager(tmpDir)
				require.NoError(t, err)

				d := &backlog.Discovery{
					Title:  "Test " + string(cat),
					Status: backlog.StatusPending,
					Content: backlog.Content{
						Category: cat,
						Severity: sev,
					},
					Context: backlog.Context{
						DiscoveredAt: time.Now().UTC(),
						DiscoveredBy: "human:tester",
					},
				}
				err = mgr.Add(ctx, d)
				require.NoError(t, err)

				opts := backlog.PromoteOptions{
					DryRun: true,
				}

				result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
				require.NoError(t, err)

				// All results should have valid template
				assert.NotEmpty(t, result.TemplateName)
				assert.True(t, backlog.IsValidTemplateName(result.TemplateName))
				assert.NotEmpty(t, result.WorkspaceName)
				assert.NotEmpty(t, result.BranchName)
				assert.NotEmpty(t, result.Description)
			})
		}
	}
}

// mockCLIAIRunner implements backlog.AIRunner for CLI tests.
type mockCLIAIRunner struct {
	result *domain.AIResult
	err    error
}

func (m *mockCLIAIRunner) Run(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
	return m.result, m.err
}

func TestRunBacklogPromote_WithLocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Bug in payment processor",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category:    backlog.CategoryBug,
			Severity:    backlog.SeverityHigh,
			Description: "Null pointer exception in payment flow",
		},
		Location: &backlog.Location{
			File: "internal/payment/processor.go",
			Line: 142,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "ai:claude:sonnet",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Bug in payment processor")
	assert.Contains(t, output, "bugfix")
}

func TestRunBacklogPromote_WithTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Performance issue in API",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category:    backlog.CategoryPerformance,
			Severity:    backlog.SeverityMedium,
			Description: "Slow response times under load",
			Tags:        []string{"api", "latency", "optimization"},
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:developer",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Performance issue in API")
	assert.Contains(t, output, "task") // Performance category maps to task
}

func TestRunBacklogPromote_LongTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)

	longTitle := "This is a very long title that exceeds the normal display length and should be truncated when displayed in the output"
	d := &backlog.Discovery{
		Title:  longTitle,
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityLow,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	// Title should be in output (the title display itself isn't truncated)
	assert.Contains(t, output, longTitle)
}

func TestRunBacklogPromote_SpecialCharsInTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)

	// Title with special characters that need sanitization for workspace name
	d := &backlog.Discovery{
		Title:  "Fix: NULL pointer @ line 42 [urgent!]",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityHigh,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	opts := backlog.PromoteOptions{
		DryRun: true,
	}

	result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
	require.NoError(t, err)

	// Workspace name should be sanitized (no special chars)
	assert.NotContains(t, result.WorkspaceName, ":")
	assert.NotContains(t, result.WorkspaceName, "@")
	assert.NotContains(t, result.WorkspaceName, "[")
	assert.NotContains(t, result.WorkspaceName, "]")
	assert.NotContains(t, result.WorkspaceName, "!")
	// Should still contain meaningful parts
	assert.NotEmpty(t, result.WorkspaceName)
}

func TestRunBacklogPromote_AIWithTemplateOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "AI Override Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityHigh,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Create mock AI runner that returns "bugfix" template
	aiRunner := &mockCLIAIRunner{
		result: &domain.AIResult{
			Success: true,
			Output: `{
				"template": "bugfix",
				"description": "AI generated description",
				"reasoning": "AI reasoning",
				"workspace_name": "ai-workspace",
				"priority": 2
			}`,
		},
	}

	aiPromoter := backlog.NewAIPromoter(aiRunner, nil)

	// Template override should win over AI suggestion
	opts := backlog.PromoteOptions{
		Template: "feature", // Override to feature instead of bugfix
		UseAI:    true,
		DryRun:   true,
	}

	result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, aiPromoter)
	require.NoError(t, err)

	// Template override should win
	assert.Equal(t, "feature", result.TemplateName)
	// But AI analysis should still be captured
	assert.NotNil(t, result.AIAnalysis)
	assert.Equal(t, "bugfix", result.AIAnalysis.Template) // AI suggested bugfix
}

func TestRunBacklogPromote_DismissedDiscovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Dismissed Discovery",
		Status: backlog.StatusDismissed,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityLow,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
		Lifecycle: backlog.Lifecycle{
			DismissedReason: "Not a real bug",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.Error(t, err)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
	assert.Contains(t, err.Error(), "invalid status transition")
}

func TestPromoteResult_BranchNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name           string
		template       string
		title          string
		expectedPrefix string
	}{
		{
			name:           "bugfix template",
			template:       "bugfix",
			title:          "Fix Login Bug",
			expectedPrefix: "fix/",
		},
		{
			name:           "feature template",
			template:       "feature",
			title:          "Add Dark Mode",
			expectedPrefix: "feat/",
		},
		{
			name:           "hotfix template",
			template:       "hotfix",
			title:          "Critical Security Patch",
			expectedPrefix: "hotfix/",
		},
		{
			name:           "task template",
			template:       "task",
			title:          "Update Documentation",
			expectedPrefix: "task/",
		},
		{
			name:           "commit template",
			template:       "commit",
			title:          "Cleanup Unused Code",
			expectedPrefix: "chore/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			mgr, err := backlog.NewManager(tmpDir)
			require.NoError(t, err)

			d := &backlog.Discovery{
				Title:  tc.title,
				Status: backlog.StatusPending,
				Content: backlog.Content{
					Category: backlog.CategoryBug,
					Severity: backlog.SeverityMedium,
				},
				Context: backlog.Context{
					DiscoveredAt: time.Now().UTC(),
					DiscoveredBy: "human:tester",
				},
			}
			err = mgr.Add(ctx, d)
			require.NoError(t, err)

			opts := backlog.PromoteOptions{
				Template: tc.template,
				DryRun:   true,
			}

			result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, nil)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(result.BranchName, tc.expectedPrefix),
				"expected branch %q to have prefix %q", result.BranchName, tc.expectedPrefix)
		})
	}
}

func TestRunBacklogPromote_JSONOutput_WithAIAnalysis(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "JSON AI Output Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category:    backlog.CategoryBug,
			Severity:    backlog.SeverityHigh,
			Description: "Test discovery for JSON output",
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Create mock AI runner
	aiRunner := &mockCLIAIRunner{
		result: &domain.AIResult{
			Success: true,
			Output: `{
				"template": "bugfix",
				"description": "AI optimized description for the bug fix",
				"reasoning": "High severity bug requires immediate attention",
				"workspace_name": "json-ai-test",
				"priority": 2
			}`,
		},
	}

	aiPromoter := backlog.NewAIPromoter(aiRunner, nil)

	opts := backlog.PromoteOptions{
		UseAI:  true,
		DryRun: true,
	}

	result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, aiPromoter)
	require.NoError(t, err)

	// Verify AI analysis is captured
	require.NotNil(t, result.AIAnalysis)
	assert.Equal(t, "bugfix", result.AIAnalysis.Template)
	assert.Equal(t, "AI optimized description for the bug fix", result.AIAnalysis.Description)
	assert.Equal(t, "High severity bug requires immediate attention", result.AIAnalysis.Reasoning)
	assert.Equal(t, "json-ai-test", result.AIAnalysis.WorkspaceName)
	assert.Equal(t, 2, result.AIAnalysis.Priority)
}

func TestRunBacklogPromote_AIMode_Fallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "AI Fallback Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategorySecurity,
			Severity: backlog.SeverityCritical,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Create mock AI runner that returns an error
	aiRunner := &mockCLIAIRunner{
		err: atlaserrors.ErrClaudeInvocation,
	}

	aiPromoter := backlog.NewAIPromoter(aiRunner, nil)

	opts := backlog.PromoteOptions{
		UseAI:  true,
		DryRun: true,
	}

	result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, aiPromoter)
	require.NoError(t, err)

	// Should fall back to deterministic mapping
	// Critical security -> hotfix
	assert.Equal(t, "hotfix", result.TemplateName)
	// AI analysis should still be present with fallback values
	require.NotNil(t, result.AIAnalysis)
	assert.Contains(t, result.AIAnalysis.Reasoning, "Deterministic")
}

func TestRunBacklogPromote_AIMode_ConfigOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Config Override Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Create mock AI runner that records requests
	requests := make([]*domain.AIRequest, 0)
	aiRunner := &mockCLIAIRunnerRecorder{
		result: &domain.AIResult{
			Success: true,
			Output: `{
				"template": "bugfix",
				"description": "Test",
				"reasoning": "Test"
			}`,
		},
		requests: &requests,
	}

	aiPromoter := backlog.NewAIPromoter(aiRunner, nil)

	opts := backlog.PromoteOptions{
		UseAI:  true,
		Agent:  "gemini",
		Model:  "flash",
		DryRun: true,
	}

	_, err = mgr.PromoteWithOptions(ctx, d.ID, opts, aiPromoter)
	require.NoError(t, err)

	// Verify agent/model overrides were applied
	require.Len(t, requests, 1)
	assert.Equal(t, domain.Agent("gemini"), requests[0].Agent)
	assert.Equal(t, "flash", requests[0].Model)
}

// mockCLIAIRunnerRecorder implements backlog.AIRunner and records requests.
type mockCLIAIRunnerRecorder struct {
	result   *domain.AIResult
	err      error
	requests *[]*domain.AIRequest
}

func (m *mockCLIAIRunnerRecorder) Run(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	*m.requests = append(*m.requests, req)
	return m.result, m.err
}

func TestRunBacklogPromote_AIProgress_TTYOutput(t *testing.T) {
	// Cannot use t.Parallel() - test modifies global aiRunnerFactory
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "AI Progress Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Set up mock AI runner to avoid real CLI calls
	aiRunnerFactory = func(_ *config.Config) contracts.AIRunner {
		return &mockCLIAIRunner{
			result: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bugfix",
					"description": "AI generated description",
					"reasoning": "Test reasoning"
				}`,
			},
		}
	}
	t.Cleanup(func() { aiRunnerFactory = nil })

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		ai:          true,
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	// Should show AI analysis progress message with agent/model
	assert.Contains(t, output, "AI Analysis")
	assert.Contains(t, output, "claude")
	assert.Contains(t, output, "sonnet")
	// Should show completion message
	assert.Contains(t, output, "AI Analysis complete")
}

func TestRunBacklogPromote_AIProgress_WithAgentOverride(t *testing.T) {
	// Cannot use t.Parallel() - test modifies global aiRunnerFactory
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "AI Progress Agent Override Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Set up mock AI runner to avoid real CLI calls
	aiRunnerFactory = func(_ *config.Config) contracts.AIRunner {
		return &mockCLIAIRunner{
			result: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bugfix",
					"description": "AI generated description",
					"reasoning": "Test reasoning"
				}`,
			},
		}
	}
	t.Cleanup(func() { aiRunnerFactory = nil })

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		ai:          true,
		agent:       "gemini",
		model:       "flash",
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	// Should show AI analysis progress with overridden agent/model
	assert.Contains(t, output, "AI Analysis")
	assert.Contains(t, output, "gemini")
	assert.Contains(t, output, "flash")
}

func TestRunBacklogPromote_AIProgress_JSONOutput_NoProgress(t *testing.T) {
	// Cannot use t.Parallel() - test modifies global aiRunnerFactory
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "AI JSON No Progress Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Set up mock AI runner to avoid real CLI calls
	aiRunnerFactory = func(_ *config.Config) contracts.AIRunner {
		return &mockCLIAIRunner{
			result: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bugfix",
					"description": "AI generated description",
					"reasoning": "Test reasoning"
				}`,
			},
		}
	}
	t.Cleanup(func() { aiRunnerFactory = nil })

	cmd := newBacklogPromoteCmd()
	// Add global output flag
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer

	opts := promoteOptions{
		ai:          true,
		jsonOutput:  true,
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	// JSON output should NOT contain progress messages
	assert.NotContains(t, output, "AI Analysis (")
	assert.NotContains(t, output, "AI Analysis complete")

	// Should be valid JSON
	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.True(t, result["success"].(bool))
}

func TestBuildStartCommand(t *testing.T) {
	t.Parallel()

	t.Run("includes branch flag", func(t *testing.T) {
		t.Parallel()
		result := &backlog.PromoteResult{
			TemplateName:  "bugfix",
			WorkspaceName: "fix-bug",
			BranchName:    "fix/fix-bug",
			Discovery: &backlog.Discovery{
				ID: "disc-abc123",
				Context: backlog.Context{
					Git: &backlog.GitContext{
						Branch: "develop",
						Commit: "abc1234",
					},
				},
			},
		}

		cmd := buildStartCommand(result)

		assert.Contains(t, cmd, "-b develop")
		assert.Contains(t, cmd, "-t bugfix")
		assert.Contains(t, cmd, "-w fix-bug")
		assert.Contains(t, cmd, "--from-backlog disc-abc123")
	})

	t.Run("includes verify flag when UseVerify is true", func(t *testing.T) {
		t.Parallel()
		useVerify := true
		result := &backlog.PromoteResult{
			TemplateName:  "hotfix",
			WorkspaceName: "security-fix",
			BranchName:    "hotfix/security-fix",
			Discovery: &backlog.Discovery{
				ID: "disc-sec123",
			},
			AIAnalysis: &backlog.AIAnalysis{
				UseVerify: &useVerify,
			},
		}

		cmd := buildStartCommand(result)

		assert.Contains(t, cmd, "--verify")
		assert.NotContains(t, cmd, "--no-verify")
	})

	t.Run("includes no-verify flag when UseVerify is false", func(t *testing.T) {
		t.Parallel()
		useVerify := false
		result := &backlog.PromoteResult{
			TemplateName:  "task",
			WorkspaceName: "simple-task",
			BranchName:    "task/simple-task",
			Discovery: &backlog.Discovery{
				ID: "disc-tsk123",
			},
			AIAnalysis: &backlog.AIAnalysis{
				UseVerify: &useVerify,
			},
		}

		cmd := buildStartCommand(result)

		assert.Contains(t, cmd, "--no-verify")
		assert.NotContains(t, cmd, "--verify ")
	})

	t.Run("omits verify flags when UseVerify is nil", func(t *testing.T) {
		t.Parallel()
		result := &backlog.PromoteResult{
			TemplateName:  "bugfix",
			WorkspaceName: "some-bug",
			BranchName:    "fix/some-bug",
			Discovery: &backlog.Discovery{
				ID: "disc-def456",
			},
			AIAnalysis: &backlog.AIAnalysis{
				UseVerify: nil,
			},
		}

		cmd := buildStartCommand(result)

		assert.NotContains(t, cmd, "--verify")
		assert.NotContains(t, cmd, "--no-verify")
	})

	t.Run("works without AIAnalysis", func(t *testing.T) {
		t.Parallel()
		result := &backlog.PromoteResult{
			TemplateName:  "bugfix",
			WorkspaceName: "another-bug",
			BranchName:    "fix/another-bug",
			Discovery: &backlog.Discovery{
				ID: "disc-xyz789",
				Context: backlog.Context{
					Git: &backlog.GitContext{
						Branch: "main",
						Commit: "xyz7890",
					},
				},
			},
			AIAnalysis: nil,
		}

		cmd := buildStartCommand(result)

		assert.Contains(t, cmd, "atlas start")
		assert.Contains(t, cmd, "-b main")
		assert.NotContains(t, cmd, "--verify")
		assert.NotContains(t, cmd, "--no-verify")
	})
}

func TestRunBacklogPromote_OutputIncludesBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Branch Output Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
			Git: &backlog.GitContext{
				Branch: "develop",
				Commit: "abc1234",
			},
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	output := buf.String()
	// Should include -b flag with the discovery's source branch (not the generated branch name)
	assert.Contains(t, output, "-b develop")
}

func TestRunBacklogPromote_JSONOutput_IncludesStartCommand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "JSON Start Command Test",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategoryBug,
			Severity: backlog.SeverityMedium,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	cmd := newBacklogPromoteCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer

	opts := promoteOptions{
		jsonOutput:  true,
		dryRun:      true,
		projectRoot: tmpDir,
	}

	err = runBacklogPromote(ctx, cmd, &buf, d.ID, opts)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Should include start_command in JSON output
	startCmd, ok := result["start_command"].(string)
	require.True(t, ok, "start_command should be a string")
	assert.Contains(t, startCmd, "atlas start")
	assert.Contains(t, startCmd, "-b")
	assert.Contains(t, startCmd, "--from-backlog")
}

func TestRunBacklogPromote_CriticalSecurity_IncludesVerify(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	d := &backlog.Discovery{
		Title:  "Critical Security Issue",
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category: backlog.CategorySecurity,
			Severity: backlog.SeverityCritical,
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err = mgr.Add(ctx, d)
	require.NoError(t, err)

	// Use AI promoter with mock runner that returns fallback (error triggers fallback)
	aiRunner := &mockCLIAIRunner{
		err: atlaserrors.ErrClaudeInvocation, // Triggers fallback analysis
	}
	aiPromoter := backlog.NewAIPromoter(aiRunner, nil)

	opts := backlog.PromoteOptions{
		UseAI:  true,
		DryRun: true,
	}

	result, err := mgr.PromoteWithOptions(ctx, d.ID, opts, aiPromoter)
	require.NoError(t, err)

	// Critical security issues should have UseVerify set to true in fallback analysis
	require.NotNil(t, result.AIAnalysis)
	require.NotNil(t, result.AIAnalysis.UseVerify)
	assert.True(t, *result.AIAnalysis.UseVerify)

	// Verify the start command includes --verify
	cmd := buildStartCommand(result)
	assert.Contains(t, cmd, "--verify")
}

func TestIsPromoteInteractiveMode(t *testing.T) {
	t.Parallel()

	t.Run("returns false when ID provided", func(t *testing.T) {
		t.Parallel()
		opts := promoteOptions{}
		result := isPromoteInteractiveMode("item-ABC123", opts)
		assert.False(t, result, "should not be interactive when ID is provided")
	})

	t.Run("returns false when JSON output requested", func(t *testing.T) {
		t.Parallel()
		opts := promoteOptions{jsonOutput: true}
		result := isPromoteInteractiveMode("", opts)
		assert.False(t, result, "should not be interactive with JSON output")
	})

	t.Run("returns false when both ID and JSON provided", func(t *testing.T) {
		t.Parallel()
		opts := promoteOptions{jsonOutput: true}
		result := isPromoteInteractiveMode("item-ABC123", opts)
		assert.False(t, result, "should not be interactive when ID is provided")
	})
}

func TestBuildDiscoveryOptions(t *testing.T) {
	t.Parallel()

	t.Run("builds options from discoveries", func(t *testing.T) {
		t.Parallel()

		discoveries := []*backlog.Discovery{
			{
				ID:     "item-ABC123",
				Title:  "First discovery",
				Status: backlog.StatusPending,
				Content: backlog.Content{
					Category: backlog.CategoryBug,
					Severity: backlog.SeverityHigh,
				},
				Context: backlog.Context{
					DiscoveredAt: time.Now().UTC(),
					DiscoveredBy: "human:tester",
				},
			},
			{
				ID:     "item-DEF456",
				Title:  "Second discovery",
				Status: backlog.StatusPending,
				Content: backlog.Content{
					Category: backlog.CategorySecurity,
					Severity: backlog.SeverityCritical,
				},
				Context: backlog.Context{
					DiscoveredAt: time.Now().UTC(),
					DiscoveredBy: "human:tester",
				},
			},
		}

		options := buildDiscoveryOptions(discoveries)

		require.Len(t, options, 2)

		// First option
		assert.Contains(t, options[0].Label, "[item-ABC123]")
		assert.Contains(t, options[0].Label, "First discovery")
		assert.Contains(t, options[0].Description, "bug/high")
		assert.Equal(t, "item-ABC123", options[0].Value)

		// Second option
		assert.Contains(t, options[1].Label, "[item-DEF456]")
		assert.Contains(t, options[1].Label, "Second discovery")
		assert.Contains(t, options[1].Description, "security/critical")
		assert.Equal(t, "item-DEF456", options[1].Value)
	})

	t.Run("truncates long titles", func(t *testing.T) {
		t.Parallel()

		longTitle := "This is a very long title that exceeds fifty characters and should be truncated"
		discoveries := []*backlog.Discovery{
			{
				ID:     "item-ABC123",
				Title:  longTitle,
				Status: backlog.StatusPending,
				Content: backlog.Content{
					Category: backlog.CategoryBug,
					Severity: backlog.SeverityMedium,
				},
				Context: backlog.Context{
					DiscoveredAt: time.Now().UTC(),
					DiscoveredBy: "human:tester",
				},
			},
		}

		options := buildDiscoveryOptions(discoveries)

		require.Len(t, options, 1)
		// Label should contain truncated title with "..."
		assert.Contains(t, options[0].Label, "...")
		assert.Less(t, len(options[0].Label), len("[item-ABC123] ")+len(longTitle))
	})

	t.Run("handles empty list", func(t *testing.T) {
		t.Parallel()

		discoveries := []*backlog.Discovery{}
		options := buildDiscoveryOptions(discoveries)

		assert.Empty(t, options)
	})
}

func TestRunBacklogPromote_NoIDWithJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cmd := newBacklogPromoteCmd()

	var buf bytes.Buffer

	opts := promoteOptions{
		jsonOutput: true,
	}

	// Call with empty ID and JSON flag should error
	err := runBacklogPromote(ctx, cmd, &buf, "", opts)
	require.Error(t, err)
	assert.True(t, atlaserrors.IsExitCode2Error(err))
	assert.Contains(t, err.Error(), "ID required")
}
