package backlog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/contracts"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// defaultAnalysisBudgetUSD is the default maximum cost for AI analysis.
const defaultAnalysisBudgetUSD = 0.10

// AIAnalysis contains the AI's analysis of a discovery for optimal task configuration.
type AIAnalysis struct {
	// Template is the recommended template name.
	Template string `json:"template"`

	// Description is the optimized task description.
	Description string `json:"description"`

	// Reasoning explains why these choices were made.
	Reasoning string `json:"reasoning"`

	// WorkspaceName is the suggested workspace name.
	WorkspaceName string `json:"workspace_name,omitempty"`

	// Priority suggests task priority (1-5, where 1 is highest).
	Priority int `json:"priority,omitempty"`
}

// AIPromoter provides AI-assisted analysis for discovery promotion.
type AIPromoter struct {
	aiRunner contracts.AIRunner
	cfg      *config.AIConfig
}

// NewAIPromoter creates a new AIPromoter with the given AI runner and config.
func NewAIPromoter(runner contracts.AIRunner, cfg *config.AIConfig) *AIPromoter {
	return &AIPromoter{
		aiRunner: runner,
		cfg:      cfg,
	}
}

// AIPromoterConfig allows customization of AI promotion behavior.
type AIPromoterConfig struct {
	// Agent overrides the AI agent (claude, gemini, codex).
	Agent string

	// Model overrides the AI model.
	Model string

	// Timeout overrides the default timeout.
	Timeout time.Duration

	// MaxBudgetUSD limits the AI cost for analysis.
	MaxBudgetUSD float64
}

// Analyze uses AI to determine the optimal task configuration for a discovery.
// Returns AIAnalysis with recommended template, description, and reasoning.
// Falls back to deterministic mapping if AI fails.
func (p *AIPromoter) Analyze(ctx context.Context, d *Discovery, cfg *AIPromoterConfig) (*AIAnalysis, error) {
	if p.aiRunner == nil {
		return p.fallbackAnalysis(d), nil
	}

	// Build the prompt and request
	prompt := p.buildAnalysisPrompt(d)
	req := p.buildAIRequest(prompt, cfg)

	// Run AI analysis
	result, err := p.aiRunner.Run(ctx, req)
	if err != nil {
		// Fall back to deterministic analysis on AI error
		return p.fallbackAnalysis(d), nil //nolint:nilerr // intentional fallback on error
	}

	if !result.Success || result.Output == "" {
		return p.fallbackAnalysis(d), nil
	}

	// Parse the AI response
	analysis, err := p.parseAnalysisResponse(result.Output)
	if err != nil {
		// Fall back to deterministic analysis on parse error
		return p.fallbackAnalysis(d), nil //nolint:nilerr // intentional fallback on error
	}

	return analysis, nil
}

// AnalyzeWithFallback is a convenience function that analyzes a discovery and
// always returns a valid analysis, falling back to deterministic mapping on error.
func (p *AIPromoter) AnalyzeWithFallback(ctx context.Context, d *Discovery, cfg *AIPromoterConfig) *AIAnalysis {
	analysis, err := p.Analyze(ctx, d, cfg)
	if err != nil || analysis == nil {
		return p.fallbackAnalysis(d)
	}
	return analysis
}

// buildAIRequest creates the AI request with resolved agent, model, and timeout.
func (p *AIPromoter) buildAIRequest(prompt string, cfg *AIPromoterConfig) *domain.AIRequest {
	// Default values
	agent := domain.AgentClaude
	model := "sonnet"
	timeout := 30 * time.Second
	maxBudget := defaultAnalysisBudgetUSD

	// Apply global config
	p.applyGlobalConfig(&agent, &model, &maxBudget)

	// Apply per-call overrides
	applyConfigOverrides(cfg, &agent, &model, &timeout, &maxBudget)

	return &domain.AIRequest{
		Agent:        agent,
		Model:        model,
		Prompt:       prompt,
		Timeout:      timeout,
		MaxBudgetUSD: maxBudget,
	}
}

// applyGlobalConfig applies the global AI config settings.
func (p *AIPromoter) applyGlobalConfig(agent *domain.Agent, model *string, maxBudget *float64) {
	if p.cfg == nil {
		return
	}
	if p.cfg.Agent != "" {
		*agent = domain.Agent(p.cfg.Agent)
	}
	if p.cfg.Model != "" {
		*model = p.cfg.Model
	}
	if p.cfg.MaxBudgetUSD > 0 {
		*maxBudget = p.cfg.MaxBudgetUSD
	}
}

// applyConfigOverrides applies per-call config overrides.
func applyConfigOverrides(cfg *AIPromoterConfig, agent *domain.Agent, model *string, timeout *time.Duration, maxBudget *float64) {
	if cfg == nil {
		return
	}
	if cfg.Agent != "" {
		*agent = domain.Agent(cfg.Agent)
	}
	if cfg.Model != "" {
		*model = cfg.Model
	}
	if cfg.Timeout > 0 {
		*timeout = cfg.Timeout
	}
	if cfg.MaxBudgetUSD > 0 {
		*maxBudget = cfg.MaxBudgetUSD
	}
}

// buildAnalysisPrompt creates the prompt for AI analysis.
func (p *AIPromoter) buildAnalysisPrompt(d *Discovery) string {
	var sb strings.Builder

	sb.WriteString("Analyze this discovery and determine the best ATLAS task configuration:\n\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", d.Title))
	sb.WriteString(fmt.Sprintf("Category: %s\n", d.Content.Category))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", d.Content.Severity))

	if d.Content.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", d.Content.Description))
	}

	if d.Location != nil && d.Location.File != "" {
		sb.WriteString(fmt.Sprintf("Location: %s", d.Location.File))
		if d.Location.Line > 0 {
			sb.WriteString(fmt.Sprintf(":%d", d.Location.Line))
		}
		sb.WriteString("\n")
	}

	if len(d.Content.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(d.Content.Tags, ", ")))
	}

	sb.WriteString("\nAvailable templates: bugfix, feature, task, fix, hotfix, commit\n\n")
	sb.WriteString("Return JSON only, no markdown:\n")
	sb.WriteString(`{
  "template": "best template name",
  "description": "optimized task description",
  "reasoning": "brief explanation",
  "workspace_name": "suggested-workspace-name",
  "priority": 1-5 where 1 is highest
}`)

	return sb.String()
}

// parseAnalysisResponse parses the AI's JSON response into an AIAnalysis struct.
func (p *AIPromoter) parseAnalysisResponse(output string) (*AIAnalysis, error) {
	// Clean the output - remove any markdown code blocks
	output = strings.TrimSpace(output)
	output = strings.TrimPrefix(output, "```json")
	output = strings.TrimPrefix(output, "```")
	output = strings.TrimSuffix(output, "```")
	output = strings.TrimSpace(output)

	var analysis AIAnalysis
	if err := json.Unmarshal([]byte(output), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Validate template
	if !IsValidTemplateName(analysis.Template) {
		return nil, fmt.Errorf("%w: AI returned invalid template %q", atlaserrors.ErrInvalidArgument, analysis.Template)
	}

	return &analysis, nil
}

// fallbackAnalysis returns a deterministic analysis when AI is unavailable or fails.
func (p *AIPromoter) fallbackAnalysis(d *Discovery) *AIAnalysis {
	template := MapCategoryToTemplate(d.Content.Category, d.Content.Severity)
	description := GenerateTaskDescription(d)
	workspaceName := SanitizeWorkspaceName(d.Title)

	// Determine priority based on severity
	priority := severityToPriority(d.Content.Severity)

	return &AIAnalysis{
		Template:      template,
		Description:   description,
		WorkspaceName: workspaceName,
		Priority:      priority,
		Reasoning:     "Deterministic mapping based on category and severity",
	}
}

// severityToPriority converts severity to a priority value (1-5, 1 is highest).
func severityToPriority(s Severity) int {
	switch s {
	case SeverityCritical:
		return 1
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 4
	default:
		return 3
	}
}
