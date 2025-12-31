// Package git provides Git operations for ATLAS.
// This file implements PR description generation services.
package git

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// PRDescriptionGenerator generates PR descriptions.
type PRDescriptionGenerator interface {
	// Generate creates a PR description from the given options.
	Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error)
}

// PRDescOptions contains inputs for PR description generation.
type PRDescOptions struct {
	// TaskDescription describes what the task is trying to accomplish.
	TaskDescription string
	// CommitMessages are the commit messages for the changes.
	CommitMessages []string
	// FilesChanged lists the files that were modified.
	FilesChanged []PRFileChange
	// DiffSummary is a summary of the changes (optional).
	DiffSummary string
	// ValidationResults are the results of validation commands.
	ValidationResults string
	// TemplateName is the task template name (bugfix, feature, etc.).
	TemplateName string
	// TaskID is the task identifier.
	TaskID string
	// WorkspaceName is the workspace name.
	WorkspaceName string
	// BaseBranch is the target branch for the PR (used in artifact metadata).
	BaseBranch string
	// HeadBranch is the source branch with changes (used in artifact metadata).
	HeadBranch string
}

// PRFileChange represents a changed file for PR descriptions.
// This extends the git.FileChange type with insertion/deletion counts.
type PRFileChange struct {
	// Path is the file path relative to repository root.
	Path string
	// Insertions is the number of lines added.
	Insertions int
	// Deletions is the number of lines removed.
	Deletions int
}

// PRDescription contains the generated PR content.
type PRDescription struct {
	// Title is the PR title in conventional commits format.
	Title string
	// Body is the PR description markdown.
	Body string
	// ConventionalType is the commit type (feat, fix, etc.).
	ConventionalType string
	// Scope is the scope derived from changed files.
	Scope string
}

// Validate checks that the PR description has required fields.
func (d *PRDescription) Validate() error {
	if d.Title == "" {
		return fmt.Errorf("PR title is empty: %w", atlaserrors.ErrEmptyValue)
	}
	if d.Body == "" {
		return fmt.Errorf("PR body is empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Validate title format (should match conventional commits)
	if !isValidConventionalTitle(d.Title) {
		return fmt.Errorf("PR title does not match conventional commits format: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Validate body has required sections
	if !hasRequiredSections(d.Body) {
		return fmt.Errorf("PR body missing required sections: %w", atlaserrors.ErrAIInvalidFormat)
	}

	return nil
}

// isValidConventionalTitle checks if the title matches conventional commits format.
// Format: <type>(<scope>): <description> or <type>: <description>
func isValidConventionalTitle(title string) bool {
	// Matches: type(scope): description or type: description
	pattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert)(\([a-zA-Z0-9_-]+\))?:\s+.+$`)
	return pattern.MatchString(title)
}

// hasRequiredSections checks if the body has required sections.
func hasRequiredSections(body string) bool {
	bodyLower := strings.ToLower(body)
	required := []string{"## summary", "## changes", "## test plan"}
	for _, section := range required {
		if !strings.Contains(bodyLower, section) {
			return false
		}
	}
	return true
}

// AIDescriptionGenerator uses AI to generate PR descriptions.
type AIDescriptionGenerator struct {
	aiRunner ai.Runner
	logger   zerolog.Logger
	timeout  time.Duration
}

// AIDescGenOption configures an AIDescriptionGenerator.
type AIDescGenOption func(*AIDescriptionGenerator)

// NewAIDescriptionGenerator creates a new AI-based PR description generator.
func NewAIDescriptionGenerator(runner ai.Runner, opts ...AIDescGenOption) *AIDescriptionGenerator {
	g := &AIDescriptionGenerator{
		aiRunner: runner,
		logger:   zerolog.Nop(),
		timeout:  2 * time.Minute,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// WithAIDescLogger sets the logger for the generator.
func WithAIDescLogger(logger zerolog.Logger) AIDescGenOption {
	return func(g *AIDescriptionGenerator) {
		g.logger = logger
	}
}

// WithAIDescTimeout sets the timeout for AI operations.
func WithAIDescTimeout(timeout time.Duration) AIDescGenOption {
	return func(g *AIDescriptionGenerator) {
		g.timeout = timeout
	}
}

// Generate creates a PR description using AI.
func (g *AIDescriptionGenerator) Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if opts.TaskDescription == "" && len(opts.CommitMessages) == 0 {
		return nil, fmt.Errorf("either task description or commit messages required: %w", atlaserrors.ErrEmptyValue)
	}

	g.logger.Info().
		Str("task_id", opts.TaskID).
		Str("template", opts.TemplateName).
		Int("commit_count", len(opts.CommitMessages)).
		Int("file_count", len(opts.FilesChanged)).
		Msg("generating PR description with AI")

	// Build prompt
	prompt := g.buildPrompt(opts)

	// Create AI request
	aiCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	req := &domain.AIRequest{
		Prompt:         prompt,
		PermissionMode: "plan", // Read-only for description generation
		MaxTurns:       1,
		Timeout:        g.timeout,
	}

	// Call AI
	result, err := g.aiRunner.Run(aiCtx, req)
	if err != nil {
		g.logger.Error().Err(err).Msg("AI failed to generate PR description")
		return nil, fmt.Errorf("failed to generate PR description: %w", err)
	}

	if !result.Success || result.Output == "" {
		g.logger.Error().
			Bool("success", result.Success).
			Str("error", result.Error).
			Msg("AI returned unsuccessful result")
		return nil, fmt.Errorf("AI returned empty or unsuccessful response: %w", atlaserrors.ErrAIEmptyResponse)
	}

	// Parse response
	desc, err := g.parseResponse(result.Output, opts.TemplateName)
	if err != nil {
		g.logger.Warn().Err(err).Msg("failed to parse AI response, using fallback")
		// Fall back to template-based generation
		return NewTemplateDescriptionGenerator().Generate(ctx, opts)
	}

	// Validate the generated description
	if err := desc.Validate(); err != nil {
		g.logger.Warn().Err(err).Msg("generated description failed validation, using fallback")
		// Fall back to template-based generation
		return NewTemplateDescriptionGenerator().Generate(ctx, opts)
	}

	g.logger.Info().
		Str("title", desc.Title).
		Str("type", desc.ConventionalType).
		Str("scope", desc.Scope).
		Msg("PR description generated successfully")

	return desc, nil
}

// buildPrompt constructs the AI prompt for PR description generation.
func (g *AIDescriptionGenerator) buildPrompt(opts PRDescOptions) string {
	var sb strings.Builder

	sb.WriteString(`Generate a pull request title and body for the following changes.

REQUIREMENTS:
1. Title MUST follow conventional commits format: <type>(<scope>): <description>
   - Types: feat, fix, docs, style, refactor, test, chore, build, ci
   - Scope is optional but recommended (derived from changed files/packages)
   - Description should be concise (50 chars max)

2. Body MUST contain these exact sections (with ## headers):
   ## Summary
   Brief description of what changes were made and why.

   ## Changes
   List of changed files with brief descriptions.

   ## Test Plan
   How the changes were tested/validated.

   ## ATLAS Metadata (optional)
   Task and workspace information.

OUTPUT FORMAT:
Return ONLY the title and body in this exact format:
TITLE: <conventional commits title here>
BODY:
<markdown body here>

`)

	sb.WriteString("## Task Description\n")
	if opts.TaskDescription != "" {
		sb.WriteString(opts.TaskDescription)
	} else {
		sb.WriteString("(Not provided)")
	}
	sb.WriteString("\n\n")

	sb.WriteString("## Commits\n")
	if len(opts.CommitMessages) > 0 {
		for _, msg := range opts.CommitMessages {
			sb.WriteString("- " + msg + "\n")
		}
	} else {
		sb.WriteString("(No commit messages provided)\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## Files Changed\n")
	if len(opts.FilesChanged) > 0 {
		for _, f := range opts.FilesChanged {
			sb.WriteString(fmt.Sprintf("- %s (+%d, -%d)\n", f.Path, f.Insertions, f.Deletions))
		}
	} else {
		sb.WriteString("(No file changes provided)\n")
	}
	sb.WriteString("\n")

	if opts.DiffSummary != "" {
		sb.WriteString("## Diff Summary\n")
		sb.WriteString(opts.DiffSummary)
		sb.WriteString("\n\n")
	}

	if opts.ValidationResults != "" {
		sb.WriteString("## Validation Results\n")
		sb.WriteString(opts.ValidationResults)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Template Type\n")
	sb.WriteString(opts.TemplateName)
	sb.WriteString("\n\n")

	if opts.TaskID != "" {
		sb.WriteString("## Task ID\n")
		sb.WriteString(opts.TaskID)
		sb.WriteString("\n\n")
	}

	if opts.WorkspaceName != "" {
		sb.WriteString("## Workspace\n")
		sb.WriteString(opts.WorkspaceName)
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseResponse extracts the PR description from AI output.
// It expects the output to contain "TITLE:" and "BODY:" markers.
// Returns an error if the expected format is not found (caller should use template fallback).
func (g *AIDescriptionGenerator) parseResponse(output, templateName string) (*PRDescription, error) {
	desc := &PRDescription{}

	// Extract title - require TITLE: marker
	titlePattern := regexp.MustCompile(`(?m)^TITLE:\s*(.+)$`)
	if match := titlePattern.FindStringSubmatch(output); len(match) > 1 {
		desc.Title = strings.TrimSpace(match[1])
	}

	// Extract body - require BODY: marker
	bodyPattern := regexp.MustCompile(`(?s)BODY:\s*(.+)$`)
	if match := bodyPattern.FindStringSubmatch(output); len(match) > 1 {
		desc.Body = strings.TrimSpace(match[1])
	}

	// Strict validation: both markers must be present
	if desc.Title == "" {
		return nil, fmt.Errorf("AI response missing TITLE: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}
	if desc.Body == "" {
		return nil, fmt.Errorf("AI response missing BODY: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Extract conventional type from title
	typePattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert)`)
	if match := typePattern.FindStringSubmatch(desc.Title); len(match) > 1 {
		desc.ConventionalType = match[1]
	} else {
		// Default based on template
		desc.ConventionalType = typeFromTemplate(templateName)
	}

	// Extract scope from title
	scopePattern := regexp.MustCompile(`^\w+\(([^)]+)\):`)
	if match := scopePattern.FindStringSubmatch(desc.Title); len(match) > 1 {
		desc.Scope = match[1]
	}

	return desc, nil
}

// TemplateDescriptionGenerator generates PR descriptions using templates (no AI).
type TemplateDescriptionGenerator struct {
	logger zerolog.Logger
}

// TemplateDescGenOption configures a TemplateDescriptionGenerator.
type TemplateDescGenOption func(*TemplateDescriptionGenerator)

// NewTemplateDescriptionGenerator creates a template-based generator.
func NewTemplateDescriptionGenerator(opts ...TemplateDescGenOption) *TemplateDescriptionGenerator {
	g := &TemplateDescriptionGenerator{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// WithTemplateDescLogger sets the logger.
func WithTemplateDescLogger(logger zerolog.Logger) TemplateDescGenOption {
	return func(g *TemplateDescriptionGenerator) {
		g.logger = logger
	}
}

// Generate creates a PR description using templates.
func (g *TemplateDescriptionGenerator) Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	g.logger.Info().
		Str("task_id", opts.TaskID).
		Str("template", opts.TemplateName).
		Msg("generating PR description from template")

	// Derive conventional type from template
	commitType := typeFromTemplate(opts.TemplateName)

	// Derive scope from files
	scope := scopeFromFiles(opts.FilesChanged)

	// Build title
	summary := summarizeDescription(opts.TaskDescription, opts.CommitMessages)
	title := formatPRTitle(commitType, scope, summary)

	// Build body
	body := g.buildBody(opts, commitType)

	desc := &PRDescription{
		Title:            title,
		Body:             body,
		ConventionalType: commitType,
		Scope:            scope,
	}

	g.logger.Info().
		Str("title", desc.Title).
		Str("type", desc.ConventionalType).
		Str("scope", desc.Scope).
		Msg("PR description generated from template")

	return desc, nil
}

// buildBody constructs the PR body from template.
func (g *TemplateDescriptionGenerator) buildBody(opts PRDescOptions, _ string) string {
	var sb strings.Builder

	writeSummarySection(&sb, opts)
	writeChangesSection(&sb, opts)
	writeTestPlanSection(&sb, opts)
	writeMetadataSection(&sb, opts)

	return sb.String()
}

// writeSummarySection writes the Summary section.
func writeSummarySection(sb *strings.Builder, opts PRDescOptions) {
	sb.WriteString("## Summary\n\n")
	switch {
	case opts.TaskDescription != "":
		sb.WriteString(opts.TaskDescription)
	case len(opts.CommitMessages) > 0:
		sb.WriteString(opts.CommitMessages[0])
	default:
		sb.WriteString("(No description provided)")
	}
	sb.WriteString("\n\n")
}

// writeChangesSection writes the Changes section.
func writeChangesSection(sb *strings.Builder, opts PRDescOptions) {
	sb.WriteString("## Changes\n\n")
	if len(opts.FilesChanged) == 0 {
		sb.WriteString("(No files listed)\n\n")
		return
	}
	for _, f := range opts.FilesChanged {
		_, _ = fmt.Fprintf(sb, "- `%s`", f.Path)
		if f.Insertions > 0 || f.Deletions > 0 {
			_, _ = fmt.Fprintf(sb, " (+%d, -%d)", f.Insertions, f.Deletions)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// writeTestPlanSection writes the Test Plan section.
func writeTestPlanSection(sb *strings.Builder, opts PRDescOptions) {
	sb.WriteString("## Test Plan\n\n")
	if opts.ValidationResults != "" {
		sb.WriteString(opts.ValidationResults)
	} else {
		sb.WriteString("- [ ] Tests pass\n")
		sb.WriteString("- [ ] Lint passes\n")
		sb.WriteString("- [ ] Pre-commit hooks pass\n")
	}
	sb.WriteString("\n\n")
}

// writeMetadataSection writes the ATLAS Metadata section if any fields are present.
func writeMetadataSection(sb *strings.Builder, opts PRDescOptions) {
	if opts.TaskID == "" && opts.TemplateName == "" && opts.WorkspaceName == "" {
		return
	}
	sb.WriteString("## ATLAS Metadata\n\n")
	if opts.TaskID != "" {
		_, _ = fmt.Fprintf(sb, "- Task: %s\n", opts.TaskID)
	}
	if opts.TemplateName != "" {
		_, _ = fmt.Fprintf(sb, "- Template: %s\n", opts.TemplateName)
	}
	if opts.WorkspaceName != "" {
		_, _ = fmt.Fprintf(sb, "- Workspace: %s\n", opts.WorkspaceName)
	}
}

// typeFromTemplate derives the conventional commit type from template name.
func typeFromTemplate(templateName string) string {
	switch strings.ToLower(templateName) {
	case "bugfix", "bug", "hotfix":
		return "fix"
	case "feature", "feat":
		return "feat"
	case "docs", "documentation":
		return "docs"
	case "refactor", "refactoring":
		return "refactor"
	case "test", "testing":
		return "test"
	case "chore", "maintenance":
		return "chore"
	case "style", "formatting":
		return "style"
	case "build":
		return "build"
	case "ci":
		return "ci"
	case "perf", "performance":
		return "perf"
	default:
		return "feat"
	}
}

// scopeFromFiles derives a scope from the changed files.
func scopeFromFiles(files []PRFileChange) string {
	if len(files) == 0 {
		return ""
	}

	// Common structural directories to skip when determining scope
	skipDirs := map[string]bool{
		"":             true,
		".":            true,
		"internal":     true,
		"pkg":          true,
		"cmd":          true,
		"src":          true,
		"lib":          true,
		"test":         true,
		"tests":        true,
		"spec":         true,
		"vendor":       true,
		"node_modules": true,
	}

	// Find common package/directory
	dirs := make(map[string]int)
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		// Get the most specific meaningful directory
		parts := strings.Split(dir, string(filepath.Separator))
		for _, part := range parts {
			if !skipDirs[part] {
				dirs[part]++
				break
			}
		}
	}

	// Return the most common directory
	var maxDir string
	maxCount := 0
	for dir, count := range dirs {
		if count > maxCount {
			maxCount = count
			maxDir = dir
		}
	}

	return maxDir
}

// formatPRTitle formats a PR title in conventional commits format.
func formatPRTitle(commitType, scope, description string) string {
	if scope != "" {
		return fmt.Sprintf("%s(%s): %s", commitType, scope, description)
	}
	return fmt.Sprintf("%s: %s", commitType, description)
}

// summarizeDescription creates a short summary from description or commits.
// Returns a lowercase-first summary suitable for conventional commits format.
func summarizeDescription(taskDesc string, commits []string) string {
	// Prefer task description
	if taskDesc != "" {
		// Take first sentence or first 50 chars
		summary := taskDesc
		if idx := strings.Index(summary, "."); idx > 0 && idx < 60 {
			summary = summary[:idx]
		}
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		return lowercaseFirst(strings.TrimSpace(summary))
	}

	// Fall back to first commit message
	if len(commits) > 0 {
		summary := commits[0]
		// Strip conventional commit prefix if present
		if idx := strings.Index(summary, ": "); idx > 0 && idx < 20 {
			summary = summary[idx+2:]
		}
		if len(summary) > 50 {
			summary = summary[:47] + "..."
		}
		return lowercaseFirst(strings.TrimSpace(summary))
	}

	return "update code"
}

// lowercaseFirst lowercases only the first character of a string,
// preserving the case of the rest (for acronyms, proper nouns, etc.).
func lowercaseFirst(s string) string {
	if s == "" {
		return s
	}
	// Handle multi-byte UTF-8 characters properly
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// Compile-time interface checks.
var (
	_ PRDescriptionGenerator = (*AIDescriptionGenerator)(nil)
	_ PRDescriptionGenerator = (*TemplateDescriptionGenerator)(nil)
)
