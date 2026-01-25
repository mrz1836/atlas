package backlog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// mockAIRunner implements AIRunner for testing.
type mockAIRunner struct {
	response *domain.AIResult
	err      error
	requests []*domain.AIRequest
}

func (m *mockAIRunner) Run(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	m.requests = append(m.requests, req)
	return m.response, m.err
}

func validDiscoveryForAI() *Discovery {
	return &Discovery{
		ID:     "disc-abc123",
		Title:  "Missing error handling in payment processor",
		Status: StatusPending,
		Content: Content{
			Description: "The API endpoint doesn't handle network failures",
			Category:    CategoryBug,
			Severity:    SeverityHigh,
			Tags:        []string{"error-handling", "network"},
		},
		Location: &Location{
			File: "cmd/api.go",
			Line: 47,
		},
		Context: Context{
			DiscoveredAt: time.Now(),
			DiscoveredBy: "ai:claude:sonnet",
		},
	}
}

func TestNewAIPromoter(t *testing.T) {
	t.Parallel()

	t.Run("creates promoter with runner and config", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{}
		cfg := &config.AIConfig{
			Agent: "claude",
			Model: "sonnet",
		}

		promoter := NewAIPromoter(runner, cfg)

		assert.NotNil(t, promoter)
		assert.Equal(t, runner, promoter.aiRunner)
		assert.Equal(t, cfg, promoter.cfg)
	})

	t.Run("creates promoter with nil config", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{}

		promoter := NewAIPromoter(runner, nil)

		assert.NotNil(t, promoter)
	})

	t.Run("creates promoter with nil runner", func(t *testing.T) {
		t.Parallel()
		promoter := NewAIPromoter(nil, nil)

		assert.NotNil(t, promoter)
	})
}

func TestAIPromoter_Analyze(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("successful AI analysis", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Fix missing error handling in payment processor API endpoint",
					"reasoning": "Bug category with high severity warrants bugfix template",
					"workspace_name": "fix-payment-error-handling",
					"priority": 2
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
		assert.Contains(t, analysis.Description, "error handling")
		assert.NotEmpty(t, analysis.Reasoning)
		assert.Equal(t, "fix-payment-error-handling", analysis.WorkspaceName)
		assert.Equal(t, 2, analysis.Priority)
	})

	t.Run("AI returns JSON with markdown code block", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: "```json\n" + `{
					"template": "bug",
					"description": "Test description",
					"reasoning": "Test reasoning"
				}` + "\n```",
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
	})

	t.Run("falls back on AI error", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			err: atlaserrors.ErrClaudeInvocation,
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		// Should fall back to deterministic mapping
		assert.Equal(t, "bug", analysis.Template) // CategoryBug -> bugfix
		assert.Contains(t, analysis.Reasoning, "Deterministic")
	})

	t.Run("falls back on AI failure result", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: false,
				Error:   "Rate limited",
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
		assert.Contains(t, analysis.Reasoning, "Deterministic")
	})

	t.Run("falls back on empty AI output", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output:  "",
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
	})

	t.Run("falls back on invalid JSON response", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output:  "not valid json",
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
		assert.Contains(t, analysis.Reasoning, "Deterministic")
	})

	t.Run("falls back on invalid template in response", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "invalid-template",
					"description": "Test",
					"reasoning": "Test"
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		// Should fall back to deterministic
		assert.Equal(t, "bug", analysis.Template)
	})

	t.Run("uses nil runner falls back immediately", func(t *testing.T) {
		t.Parallel()
		promoter := NewAIPromoter(nil, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "bug", analysis.Template)
		assert.Contains(t, analysis.Reasoning, "Deterministic")
	})
}

func TestAIPromoter_Analyze_ConfigOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("uses config agent and model", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Test",
					"reasoning": "Test"
				}`,
			},
		}

		cfg := &config.AIConfig{
			Agent: "gemini",
			Model: "flash",
		}

		promoter := NewAIPromoter(runner, cfg)
		d := validDiscoveryForAI()

		_, err := promoter.Analyze(ctx, d, nil)
		require.NoError(t, err)

		require.Len(t, runner.requests, 1)
		assert.Equal(t, domain.Agent("gemini"), runner.requests[0].Agent)
		assert.Equal(t, "flash", runner.requests[0].Model)
	})

	t.Run("per-call config overrides global config", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Test",
					"reasoning": "Test"
				}`,
			},
		}

		globalCfg := &config.AIConfig{
			Agent: "claude",
			Model: "sonnet",
		}

		callCfg := &AIPromoterConfig{
			Agent:   "gemini",
			Model:   "pro",
			Timeout: 60 * time.Second,
		}

		promoter := NewAIPromoter(runner, globalCfg)
		d := validDiscoveryForAI()

		_, err := promoter.Analyze(ctx, d, callCfg)
		require.NoError(t, err)

		require.Len(t, runner.requests, 1)
		assert.Equal(t, domain.Agent("gemini"), runner.requests[0].Agent)
		assert.Equal(t, "pro", runner.requests[0].Model)
		assert.Equal(t, 60*time.Second, runner.requests[0].Timeout)
	})

	t.Run("respects max budget config", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Test",
					"reasoning": "Test"
				}`,
			},
		}

		callCfg := &AIPromoterConfig{
			MaxBudgetUSD: 0.05,
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		_, err := promoter.Analyze(ctx, d, callCfg)
		require.NoError(t, err)

		require.Len(t, runner.requests, 1)
		assert.InDelta(t, 0.05, runner.requests[0].MaxBudgetUSD, 0.001)
	})
}

func TestAIPromoter_ResolvedConfig(t *testing.T) {
	t.Parallel()

	t.Run("returns defaults with nil global and call config", func(t *testing.T) {
		t.Parallel()
		promoter := NewAIPromoter(nil, nil)

		agent, model := promoter.ResolvedConfig(nil)

		assert.Equal(t, "claude", agent)
		assert.Equal(t, "sonnet", model)
	})

	t.Run("applies global config", func(t *testing.T) {
		t.Parallel()
		globalCfg := &config.AIConfig{
			Agent: "gemini",
			Model: "flash",
		}
		promoter := NewAIPromoter(nil, globalCfg)

		agent, model := promoter.ResolvedConfig(nil)

		assert.Equal(t, "gemini", agent)
		assert.Equal(t, "flash", model)
	})

	t.Run("applies per-call config overrides", func(t *testing.T) {
		t.Parallel()
		promoter := NewAIPromoter(nil, nil)
		callCfg := &AIPromoterConfig{
			Agent: "codex",
			Model: "o1",
		}

		agent, model := promoter.ResolvedConfig(callCfg)

		assert.Equal(t, "codex", agent)
		assert.Equal(t, "o1", model)
	})

	t.Run("per-call config overrides global config", func(t *testing.T) {
		t.Parallel()
		globalCfg := &config.AIConfig{
			Agent: "claude",
			Model: "sonnet",
		}
		promoter := NewAIPromoter(nil, globalCfg)
		callCfg := &AIPromoterConfig{
			Agent: "gemini",
			Model: "pro",
		}

		agent, model := promoter.ResolvedConfig(callCfg)

		assert.Equal(t, "gemini", agent)
		assert.Equal(t, "pro", model)
	})

	t.Run("partial per-call override only overrides specified fields", func(t *testing.T) {
		t.Parallel()
		globalCfg := &config.AIConfig{
			Agent: "claude",
			Model: "opus",
		}
		promoter := NewAIPromoter(nil, globalCfg)
		callCfg := &AIPromoterConfig{
			Model: "haiku", // Only override model
		}

		agent, model := promoter.ResolvedConfig(callCfg)

		assert.Equal(t, "claude", agent) // From global
		assert.Equal(t, "haiku", model)  // From per-call
	})

	t.Run("partial global config uses defaults for unspecified fields", func(t *testing.T) {
		t.Parallel()
		globalCfg := &config.AIConfig{
			Agent: "gemini", // Only agent specified
		}
		promoter := NewAIPromoter(nil, globalCfg)

		agent, model := promoter.ResolvedConfig(nil)

		assert.Equal(t, "gemini", agent)
		assert.Equal(t, "sonnet", model) // Default
	})
}

func TestAIPromoter_buildAnalysisPrompt(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)

	t.Run("includes all discovery fields", func(t *testing.T) {
		t.Parallel()
		d := validDiscoveryForAI()

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.Contains(t, prompt, d.Title)
		assert.Contains(t, prompt, string(d.Content.Category))
		assert.Contains(t, prompt, string(d.Content.Severity))
		assert.Contains(t, prompt, d.Content.Description)
		assert.Contains(t, prompt, d.Location.File)
		assert.Contains(t, prompt, "47") // Line number
		assert.Contains(t, prompt, "error-handling")
		assert.Contains(t, prompt, "network")
	})

	t.Run("includes available templates", func(t *testing.T) {
		t.Parallel()
		d := validDiscoveryForAI()

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.Contains(t, prompt, "bug")
		assert.Contains(t, prompt, "feature")
		assert.Contains(t, prompt, "task")
		assert.Contains(t, prompt, "patch")
	})

	t.Run("handles missing optional fields", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Simple issue",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityLow,
				// No description, no tags
			},
			// No location
		}

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.Contains(t, prompt, "Simple issue")
		assert.NotContains(t, prompt, "Description:")
		assert.NotContains(t, prompt, "Location:")
		assert.NotContains(t, prompt, "Tags:")
	})

	t.Run("requests JSON output format", func(t *testing.T) {
		t.Parallel()
		d := validDiscoveryForAI()

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.Contains(t, prompt, "JSON only")
		assert.Contains(t, prompt, "template")
		assert.Contains(t, prompt, "description")
		assert.Contains(t, prompt, "reasoning")
	})
}

func TestAIPromoter_fallbackAnalysis(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)

	t.Run("maps bug category to bugfix template", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Bug issue",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityHigh,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		assert.Equal(t, "bug", analysis.Template)
	})

	t.Run("maps critical security to hotfix template", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Security issue",
			Content: Content{
				Category: CategorySecurity,
				Severity: SeverityCritical,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		assert.Equal(t, "patch", analysis.Template)
	})

	t.Run("generates workspace name from title", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Fix Null Pointer Bug",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		assert.Equal(t, "fix-null-pointer-bug", analysis.WorkspaceName)
	})

	t.Run("includes deterministic reasoning", func(t *testing.T) {
		t.Parallel()
		d := validDiscoveryForAI()

		analysis := promoter.fallbackAnalysis(d)

		assert.Contains(t, analysis.Reasoning, "Deterministic")
	})
}

func TestSeverityToPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityCritical, 1},
		{SeverityHigh, 2},
		{SeverityMedium, 3},
		{SeverityLow, 4},
		{Severity("unknown"), 3}, // Default to medium
	}

	for _, tc := range tests {
		t.Run(string(tc.severity), func(t *testing.T) {
			t.Parallel()
			result := severityToPriority(tc.severity)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAIPromoter_AnalyzeWithFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("returns AI analysis when successful", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "feature",
					"description": "AI description",
					"reasoning": "AI reasoning"
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis := promoter.AnalyzeWithFallback(ctx, d, nil)

		assert.Equal(t, "feature", analysis.Template)
		assert.Equal(t, "AI description", analysis.Description)
	})

	t.Run("returns fallback on AI error", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			err: atlaserrors.ErrClaudeInvocation,
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis := promoter.AnalyzeWithFallback(ctx, d, nil)

		// Should still return valid analysis
		assert.Equal(t, "bug", analysis.Template)
		assert.NotEmpty(t, analysis.Description)
	})

	t.Run("never returns nil", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: false,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis := promoter.AnalyzeWithFallback(ctx, d, nil)

		assert.NotNil(t, analysis)
	})
}

func TestAIAnalysis_AllCategories(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)

	// Test fallback analysis for all categories
	for _, cat := range ValidCategories() {
		for _, sev := range ValidSeverities() {
			t.Run(string(cat)+"_"+string(sev), func(t *testing.T) {
				t.Parallel()
				d := &Discovery{
					Title: "Test issue",
					Content: Content{
						Category: cat,
						Severity: sev,
					},
				}

				analysis := promoter.fallbackAnalysis(d)

				// All fields should be populated
				assert.NotEmpty(t, analysis.Template)
				assert.NotEmpty(t, analysis.Description)
				assert.NotEmpty(t, analysis.Reasoning)
				assert.True(t, IsValidTemplateName(analysis.Template))
				assert.Positive(t, analysis.Priority)
				assert.LessOrEqual(t, analysis.Priority, 5)
			})
		}
	}
}

func TestAIPromoter_Analyze_PartialJSONResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// AI returns JSON with only required fields (no optional fields)
	runner := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output: `{
				"template": "task",
				"description": "Minimal response",
				"reasoning": "Basic reasoning"
			}`,
		},
	}

	promoter := NewAIPromoter(runner, nil)
	d := validDiscoveryForAI()

	analysis, err := promoter.Analyze(ctx, d, nil)

	require.NoError(t, err)
	assert.Equal(t, "task", analysis.Template)
	assert.Equal(t, "Minimal response", analysis.Description)
	assert.Equal(t, "Basic reasoning", analysis.Reasoning)
	// Optional fields should be empty/zero
	assert.Empty(t, analysis.WorkspaceName)
	assert.Zero(t, analysis.Priority)
}

func TestAIPromoter_Analyze_ExtraFieldsInResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// AI returns JSON with extra fields not in our struct
	runner := &mockAIRunner{
		response: &domain.AIResult{
			Success: true,
			Output: `{
				"template": "bug",
				"description": "With extra fields",
				"reasoning": "Test reasoning",
				"workspace_name": "test-workspace",
				"priority": 3,
				"extra_field": "should be ignored",
				"another_extra": 12345,
				"nested_extra": {"key": "value"}
			}`,
		},
	}

	promoter := NewAIPromoter(runner, nil)
	d := validDiscoveryForAI()

	analysis, err := promoter.Analyze(ctx, d, nil)

	require.NoError(t, err)
	// Should successfully parse known fields
	assert.Equal(t, "bug", analysis.Template)
	assert.Equal(t, "With extra fields", analysis.Description)
	assert.Equal(t, "Test reasoning", analysis.Reasoning)
	assert.Equal(t, "test-workspace", analysis.WorkspaceName)
	assert.Equal(t, 3, analysis.Priority)
}

func TestAIPromoter_Analyze_WhitespaceHandling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name   string
		output string
	}{
		{
			name: "leading and trailing whitespace",
			output: `
			{
				"template": "bug",
				"description": "Whitespace test",
				"reasoning": "Test"
			}
			`,
		},
		{
			name: "newlines in JSON",
			output: `


{
	"template": "bug",
	"description": "Newlines test",
	"reasoning": "Test"
}


`,
		},
		{
			name: "markdown code block with extra whitespace",
			output: `
` + "```json" + `
{
	"template": "bug",
	"description": "Code block whitespace",
	"reasoning": "Test"
}
` + "```" + `
`,
		},
		{
			name: "just backticks no json label",
			output: "```\n" + `{
				"template": "bug",
				"description": "Plain backticks",
				"reasoning": "Test"
			}` + "\n```",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runner := &mockAIRunner{
				response: &domain.AIResult{
					Success: true,
					Output:  tc.output,
				},
			}

			promoter := NewAIPromoter(runner, nil)
			d := validDiscoveryForAI()

			analysis, err := promoter.Analyze(ctx, d, nil)

			require.NoError(t, err)
			assert.Equal(t, "bug", analysis.Template)
			assert.NotEmpty(t, analysis.Description)
		})
	}
}

func TestAIPromoter_buildAnalysisPrompt_GitContext(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)

	t.Run("includes git context when present", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Test issue",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
			Context: Context{
				DiscoveredAt: time.Now(),
				DiscoveredBy: "human:tester",
				Git: &GitContext{
					Branch: "feature/test-branch",
					Commit: "abc123f",
				},
			},
		}

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.Contains(t, prompt, "Discovery git context:")
		assert.Contains(t, prompt, "Found on branch: feature/test-branch")
		assert.Contains(t, prompt, "Commit: abc123f")
	})

	t.Run("omits git context when not present", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Test issue",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
			Context: Context{
				DiscoveredAt: time.Now(),
				DiscoveredBy: "human:tester",
				// No Git context
			},
		}

		prompt := promoter.buildAnalysisPrompt(d, nil)

		assert.NotContains(t, prompt, "Discovery git context:")
		assert.NotContains(t, prompt, "Found on branch:")
	})
}

func TestAIPromoter_buildAnalysisPrompt_AtlasStartFlags(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)
	d := validDiscoveryForAI()

	prompt := promoter.buildAnalysisPrompt(d, nil)

	// Verify all atlas start flags are mentioned
	assert.Contains(t, prompt, "Available 'atlas start' command options:")
	assert.Contains(t, prompt, "--template/-t")
	assert.Contains(t, prompt, "--workspace/-w")
	assert.Contains(t, prompt, "--branch/-b")
	assert.Contains(t, prompt, "--target")
	assert.Contains(t, prompt, "--use-local")
	assert.Contains(t, prompt, "--verify")
	assert.Contains(t, prompt, "--no-verify")
	assert.Contains(t, prompt, "--agent/-a")
	assert.Contains(t, prompt, "--model/-m")
	assert.Contains(t, prompt, "--from-backlog")

	// Verify new fields in JSON schema
	assert.Contains(t, prompt, "base_branch")
	assert.Contains(t, prompt, "use_verify")
	assert.Contains(t, prompt, `"file"`)
	assert.Contains(t, prompt, `"line"`)
}

func TestAIPromoter_Analyze_NewFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("parses base_branch field", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Fix issue",
					"reasoning": "Test reasoning",
					"base_branch": "develop"
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "develop", analysis.BaseBranch)
	})

	t.Run("parses use_verify true", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "patch",
					"description": "Critical fix",
					"reasoning": "Security issue",
					"use_verify": true
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		require.NotNil(t, analysis.UseVerify)
		assert.True(t, *analysis.UseVerify)
	})

	t.Run("parses use_verify false", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "task",
					"description": "Simple task",
					"reasoning": "Documentation update",
					"use_verify": false
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		require.NotNil(t, analysis.UseVerify)
		assert.False(t, *analysis.UseVerify)
	})

	t.Run("handles missing optional new fields", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Fix issue",
					"reasoning": "Test reasoning"
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Empty(t, analysis.BaseBranch)
		assert.Nil(t, analysis.UseVerify)
	})

	t.Run("parses file field", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Fix issue",
					"reasoning": "Test reasoning",
					"file": "internal/api/handler.go"
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "internal/api/handler.go", analysis.File)
	})

	t.Run("parses file and line fields", func(t *testing.T) {
		t.Parallel()
		runner := &mockAIRunner{
			response: &domain.AIResult{
				Success: true,
				Output: `{
					"template": "bug",
					"description": "Fix issue",
					"reasoning": "Test reasoning",
					"file": "internal/api/handler.go",
					"line": 42
				}`,
			},
		}

		promoter := NewAIPromoter(runner, nil)
		d := validDiscoveryForAI()

		analysis, err := promoter.Analyze(ctx, d, nil)

		require.NoError(t, err)
		assert.Equal(t, "internal/api/handler.go", analysis.File)
		assert.Equal(t, 42, analysis.Line)
	})
}

func TestAIPromoter_fallbackAnalysis_UseVerify(t *testing.T) {
	t.Parallel()

	promoter := NewAIPromoter(nil, nil)

	t.Run("sets UseVerify true for security category", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Security vulnerability",
			Content: Content{
				Category: CategorySecurity,
				Severity: SeverityHigh,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		require.NotNil(t, analysis.UseVerify)
		assert.True(t, *analysis.UseVerify)
	})

	t.Run("sets UseVerify true for critical severity", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Critical bug",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityCritical,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		require.NotNil(t, analysis.UseVerify)
		assert.True(t, *analysis.UseVerify)
	})

	t.Run("does not set UseVerify for non-critical non-security", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Simple testing task",
			Content: Content{
				Category: CategoryTesting,
				Severity: SeverityLow,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		assert.Nil(t, analysis.UseVerify)
	})

	t.Run("does not set UseVerify for medium severity bug", func(t *testing.T) {
		t.Parallel()
		d := &Discovery{
			Title: "Medium bug",
			Content: Content{
				Category: CategoryBug,
				Severity: SeverityMedium,
			},
		}

		analysis := promoter.fallbackAnalysis(d)

		assert.Nil(t, analysis.UseVerify)
	})
}
