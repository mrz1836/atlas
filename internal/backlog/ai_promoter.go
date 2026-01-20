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

	// BaseBranch suggests which branch to base the work on (e.g., "develop", "main").
	BaseBranch string `json:"base_branch,omitempty"`

	// UseVerify suggests whether to enable AI verification (true=--verify, false=--no-verify, nil=default).
	UseVerify *bool `json:"use_verify,omitempty"`

	// Agent is the recommended AI agent (claude, gemini, codex).
	Agent string `json:"agent,omitempty"`

	// Model is the recommended AI model for the task.
	Model string `json:"model,omitempty"`

	// File is the relevant file for this task (echoed from discovery or refined).
	File string `json:"file,omitempty"`

	// Line is the relevant line number (if applicable).
	Line int `json:"line,omitempty"`
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

// ResolvedConfig returns the resolved agent and model that will be used for analysis.
// This applies global config and optional per-call overrides, matching the resolution
// logic used in buildAIRequest.
func (p *AIPromoter) ResolvedConfig(cfg *AIPromoterConfig) (agent, model string) {
	// Default values (same as buildAIRequest)
	a := domain.AgentClaude
	m := "sonnet"
	var maxBudget float64

	// Apply global config
	p.applyGlobalConfig(&a, &m, &maxBudget)

	// Apply per-call overrides
	if cfg != nil {
		if cfg.Agent != "" {
			a = domain.Agent(cfg.Agent)
		}
		if cfg.Model != "" {
			m = cfg.Model
		}
	}

	return string(a), m
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

	// AvailableAgents lists the agents that are detected/installed.
	// If empty, all agents are shown in the prompt guidance.
	AvailableAgents []string
}

// Analyze uses AI to determine the optimal task configuration for a discovery.
// Returns AIAnalysis with recommended template, description, and reasoning.
// Falls back to deterministic mapping if AI fails.
func (p *AIPromoter) Analyze(ctx context.Context, d *Discovery, cfg *AIPromoterConfig) (*AIAnalysis, error) {
	if p.aiRunner == nil {
		return p.fallbackAnalysis(d), nil
	}

	// Build the prompt and request
	prompt := p.buildAnalysisPrompt(d, cfg)
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
func (p *AIPromoter) buildAnalysisPrompt(d *Discovery, cfg *AIPromoterConfig) string {
	var sb strings.Builder

	p.writeDiscoveryInfo(&sb, d)
	p.writeCommandOptions(&sb)

	availableAgents := p.getAvailableAgentsForPrompt(cfg)
	p.writeAgentGuidance(&sb, availableAgents)

	return sb.String()
}

// writeDiscoveryInfo writes discovery information to the string builder.
func (p *AIPromoter) writeDiscoveryInfo(sb *strings.Builder, d *Discovery) {
	sb.WriteString("Analyze this discovery and determine the best ATLAS task configuration:\n\n")
	fmt.Fprintf(sb, "Title: %s\n", d.Title)
	fmt.Fprintf(sb, "Category: %s\n", d.Content.Category)
	fmt.Fprintf(sb, "Severity: %s\n", d.Content.Severity)

	if d.Content.Description != "" {
		fmt.Fprintf(sb, "Description: %s\n", d.Content.Description)
	}

	if d.Location != nil && d.Location.File != "" {
		fmt.Fprintf(sb, "Location: %s", d.Location.File)
		if d.Location.Line > 0 {
			fmt.Fprintf(sb, ":%d", d.Location.Line)
		}
		sb.WriteString("\n")
	}

	if len(d.Content.Tags) > 0 {
		fmt.Fprintf(sb, "Tags: %s\n", strings.Join(d.Content.Tags, ", "))
	}

	p.writeGitContext(sb, d)
	sb.WriteString("\nAvailable templates: bugfix, feature, task, fix, hotfix, commit\n")
}

// writeGitContext writes git context information to the string builder.
func (p *AIPromoter) writeGitContext(sb *strings.Builder, d *Discovery) {
	if d.Context.Git == nil {
		return
	}

	sb.WriteString("\nDiscovery git context:\n")
	if d.Context.Git.Branch != "" {
		fmt.Fprintf(sb, "  Found on branch: %s\n", d.Context.Git.Branch)
	}
	if d.Context.Git.Commit != "" {
		fmt.Fprintf(sb, "  Commit: %s\n", d.Context.Git.Commit)
	}
}

// writeCommandOptions writes available command options to the string builder.
func (p *AIPromoter) writeCommandOptions(sb *strings.Builder) {
	sb.WriteString("\nAvailable 'atlas start' command options:\n")
	sb.WriteString("  --template/-t    Template to use (bugfix, feature, commit, hotfix, task, fix)\n")
	sb.WriteString("  --workspace/-w   Custom workspace name\n")
	sb.WriteString("  --branch/-b      Base branch to create workspace from (fetches from remote)\n")
	sb.WriteString("  --target         Existing branch to checkout (mutually exclusive with --branch)\n")
	sb.WriteString("  --use-local      Prefer local branch over remote when both exist\n")
	sb.WriteString("  --verify         Enable AI verification step (for critical changes)\n")
	sb.WriteString("  --no-verify      Disable AI verification step (for simple changes)\n")
	sb.WriteString("  --agent/-a       AI agent to use (claude, gemini, codex)\n")
	sb.WriteString("  --model/-m       AI model to use\n")
	sb.WriteString("  --from-backlog   Link to backlog discovery (auto-set)\n")
}

// writeAgentGuidance writes agent and model guidance to the string builder.
func (p *AIPromoter) writeAgentGuidance(sb *strings.Builder, availableAgents []string) {
	if len(availableAgents) == 0 {
		p.writeJSONTemplateWithoutAgents(sb)
		return
	}

	p.writeAvailableAgentsAndModels(sb, availableAgents)
	p.writeModelSelectionGuidance(sb, availableAgents)
	p.writeJSONTemplateWithAgents(sb, availableAgents)
}

// writeAvailableAgentsAndModels writes available agents and their models.
func (p *AIPromoter) writeAvailableAgentsAndModels(sb *strings.Builder, availableAgents []string) {
	sb.WriteString("\nAvailable agents and models:\n")
	for _, agent := range availableAgents {
		a := domain.Agent(agent)
		aliases := a.ModelAliases()
		defaultModel := a.DefaultModel()
		if len(aliases) > 0 {
			var models []string
			for _, alias := range aliases {
				if alias == defaultModel {
					models = append(models, fmt.Sprintf("%s (default)", alias))
				} else {
					models = append(models, alias)
				}
			}
			fmt.Fprintf(sb, "  %s: %s\n", capitalizeFirst(agent), strings.Join(models, ", "))
		}
	}
}

// writeModelSelectionGuidance writes model selection guidance for available agents.
func (p *AIPromoter) writeModelSelectionGuidance(sb *strings.Builder, availableAgents []string) {
	sb.WriteString("\nModel selection guidance:\n")

	if containsAgent(availableAgents, "claude") {
		sb.WriteString("  Complex architecture, critical decisions → opus (deep reasoning for advanced tasks)\n")
		sb.WriteString("  Standard development, most tasks → sonnet (best balance of capability and cost)\n")
		sb.WriteString("  Simple changes, typos, minor fixes → haiku (fast and cost-effective)\n")
	}

	if containsAgent(availableAgents, "gemini") {
		if containsAgent(availableAgents, "claude") {
			sb.WriteString("  Commit messages, routine tasks → haiku or flash (speed over depth)\n")
		} else {
			sb.WriteString("  Standard development → pro (capable)\n")
			sb.WriteString("  Simple changes, routine tasks → flash (fast and efficient)\n")
		}
	}

	if containsAgent(availableAgents, "codex") && !containsAgent(availableAgents, "claude") && !containsAgent(availableAgents, "gemini") {
		sb.WriteString("  Complex tasks → max (most capable)\n")
		sb.WriteString("  Standard development → codex (balanced)\n")
		sb.WriteString("  Simple changes → mini (fast)\n")
	}
}

// writeJSONTemplateWithAgents writes the JSON response template including agent fields.
func (p *AIPromoter) writeJSONTemplateWithAgents(sb *strings.Builder, availableAgents []string) {
	agentList := strings.Join(availableAgents, ", ")
	sb.WriteString("\nReturn JSON only, no markdown:\n")
	fmt.Fprintf(sb, `{
  "template": "best template name",
  "description": "optimized task description",
  "reasoning": "brief explanation of your choices",
  "workspace_name": "suggested-workspace-name",
  "priority": 1-5 where 1 is highest,
  "base_branch": "optional: branch to base work from if not default",
  "use_verify": "optional: true for critical/security, false for simple changes, omit for default",
  "agent": "optional: recommended agent (%s), omit to use default",
  "model": "optional: recommended model based on task complexity",
  "file": "optional: relevant file path from discovery location",
  "line": "optional: relevant line number if applicable"
}`, agentList)
}

// writeJSONTemplateWithoutAgents writes the JSON response template without agent fields.
func (p *AIPromoter) writeJSONTemplateWithoutAgents(sb *strings.Builder) {
	sb.WriteString("\nReturn JSON only, no markdown:\n")
	sb.WriteString(`{
  "template": "best template name",
  "description": "optimized task description",
  "reasoning": "brief explanation of your choices",
  "workspace_name": "suggested-workspace-name",
  "priority": 1-5 where 1 is highest,
  "base_branch": "optional: branch to base work from if not default",
  "use_verify": "optional: true for critical/security, false for simple changes, omit for default",
  "file": "optional: relevant file path from discovery location",
  "line": "optional: relevant line number if applicable"
}`)
}

// getAvailableAgentsForPrompt returns the list of agents to show in the prompt.
// If AvailableAgents is set in config, uses that. Otherwise returns empty (no agents shown).
func (p *AIPromoter) getAvailableAgentsForPrompt(cfg *AIPromoterConfig) []string {
	if cfg != nil && len(cfg.AvailableAgents) > 0 {
		return cfg.AvailableAgents
	}
	return nil
}

// containsAgent checks if an agent is in the list.
func containsAgent(agents []string, agent string) bool {
	for _, a := range agents {
		if a == agent {
			return true
		}
	}
	return false
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
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

	// For critical/security issues, recommend --verify
	var useVerify *bool
	if d.Content.Category == CategorySecurity || d.Content.Severity == SeverityCritical {
		v := true
		useVerify = &v
	}

	return &AIAnalysis{
		Template:      template,
		Description:   description,
		WorkspaceName: workspaceName,
		Priority:      priority,
		Reasoning:     "Deterministic mapping based on category and severity",
		UseVerify:     useVerify,
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
