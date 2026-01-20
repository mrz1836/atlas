// Package git provides Git operations for ATLAS.
// This file implements PR description generation services.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/prompts"
)

// Pre-compiled regex patterns for parsing PR descriptions.
// These are compiled once at package initialization for performance.
var (
	// conventionalTitlePattern validates conventional commits format.
	conventionalTitlePattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert)(\([a-zA-Z0-9_-]+\))?:\s+.+$`)

	// conventionalTypePattern extracts the commit type from title.
	conventionalTypePattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert)`)

	// titleScopePattern extracts the scope from title.
	titleScopePattern = regexp.MustCompile(`^\w+\(([^)]+)\):`)

	// codeBlockPattern strips markdown code blocks from AI output.
	codeBlockPattern = regexp.MustCompile("(?s)```(?:\\w*\\n)?(.+?)```")
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
	return conventionalTitlePattern.MatchString(title)
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
	agent    string // AI agent for PR description generation
	model    string // AI model for PR description generation
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

// WithAIDescModel sets the AI model for PR description generation.
// If not set, the AIRunner's default model is used.
func WithAIDescModel(model string) AIDescGenOption {
	return func(g *AIDescriptionGenerator) {
		g.model = model
	}
}

// WithAIDescAgent sets the AI agent for PR description generation.
// If not set, the AIRunner's default agent is used.
func WithAIDescAgent(agent string) AIDescGenOption {
	return func(g *AIDescriptionGenerator) {
		g.agent = agent
	}
}

// Generate creates a PR description using AI.
func (g *AIDescriptionGenerator) Generate(ctx context.Context, opts PRDescOptions) (*PRDescription, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if opts.TaskDescription == "" && len(opts.CommitMessages) == 0 {
		return nil, fmt.Errorf("either task description or commit messages required: %w", atlaserrors.ErrEmptyValue)
	}

	g.logger.Info().
		Str("task_id", opts.TaskID).
		Str("template", opts.TemplateName).
		Str("agent", g.agent).
		Str("model", g.model).
		Int("commit_count", len(opts.CommitMessages)).
		Int("file_count", len(opts.FilesChanged)).
		Msg("generating PR description with AI")

	// Build prompt
	prompt := g.buildPrompt(opts)

	// Create AI request
	aiCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	req := &domain.AIRequest{
		Agent:          domain.Agent(g.agent), // Use configured agent (empty uses AIRunner's default)
		Prompt:         prompt,
		Model:          g.model, // Use configured model (empty string uses AIRunner's default)
		PermissionMode: "plan",  // Read-only for description generation
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

	// Append hidden metadata to the body
	var metaSb strings.Builder
	writeMetadataSection(&metaSb, opts)
	if metaSb.Len() > 0 {
		desc.Body = desc.Body + "\n" + metaSb.String()
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
	// Convert local PRFileChange to prompts.PRFileChange
	files := make([]prompts.PRFileChange, len(opts.FilesChanged))
	for i, f := range opts.FilesChanged {
		files[i] = prompts.PRFileChange{
			Path:       f.Path,
			Insertions: f.Insertions,
			Deletions:  f.Deletions,
		}
	}

	data := prompts.PRDescriptionData{
		TaskDescription:   opts.TaskDescription,
		CommitMessages:    opts.CommitMessages,
		FilesChanged:      files,
		DiffSummary:       opts.DiffSummary,
		ValidationResults: opts.ValidationResults,
		TemplateName:      opts.TemplateName,
		TaskID:            opts.TaskID,
		WorkspaceName:     opts.WorkspaceName,
	}

	return prompts.MustRender(prompts.PRDescription, data)
}

// parseResponse extracts the PR description from AI output.
// It expects the output to contain "TITLE:" and "BODY:" markers (case-insensitive).
// Uses positional parsing to correctly handle edge cases like BODY appearing before TITLE.
// Returns an error if the expected format is not found (caller should use template fallback).
func (g *AIDescriptionGenerator) parseResponse(output, templateName string) (*PRDescription, error) {
	desc := &PRDescription{}

	// Preprocess: strip markdown code blocks (AI sometimes wraps output in ```)
	output = stripMarkdownCodeBlocks(output)

	// Find positions of markers (case-insensitive)
	outputLower := strings.ToLower(output)
	titleIdx := strings.Index(outputLower, "title:")
	bodyIdx := strings.Index(outputLower, "body:")

	// Both markers must exist and TITLE must come first
	if titleIdx == -1 {
		return nil, fmt.Errorf("AI response missing TITLE: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}
	if bodyIdx == -1 {
		return nil, fmt.Errorf("AI response missing BODY: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}
	if titleIdx > bodyIdx {
		return nil, fmt.Errorf("AI response has BODY: before TITLE: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Extract title: from TITLE: to BODY: (accounting for newlines)
	titleStart := titleIdx + len("title:")
	titleEnd := bodyIdx
	if newlineIdx := strings.Index(output[titleStart:], "\n"); newlineIdx != -1 && titleStart+newlineIdx < bodyIdx {
		titleEnd = titleStart + newlineIdx
	}
	desc.Title = strings.TrimSpace(output[titleStart:titleEnd])

	// Extract body: from BODY: to end
	bodyStart := bodyIdx + len("body:")
	desc.Body = strings.TrimSpace(output[bodyStart:])

	// Validation: title must not be empty
	if desc.Title == "" {
		return nil, fmt.Errorf("AI response has empty TITLE: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Validation: body must not be empty
	if desc.Body == "" {
		return nil, fmt.Errorf("AI response has empty BODY: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Validation: body should NOT contain TITLE: marker (indicates parsing went wrong)
	if strings.Contains(strings.ToLower(desc.Body), "title:") {
		return nil, fmt.Errorf("parsed body contains TITLE: marker: %w", atlaserrors.ErrAIInvalidFormat)
	}

	// Extract conventional type from title
	if match := conventionalTypePattern.FindStringSubmatch(desc.Title); len(match) > 1 {
		desc.ConventionalType = match[1]
	} else {
		// Default based on template
		desc.ConventionalType = typeFromTemplate(templateName)
	}

	// Extract scope from title
	if match := titleScopePattern.FindStringSubmatch(desc.Title); len(match) > 1 {
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
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

// writeMetadataSection writes the ATLAS metadata as a hidden HTML comment with JSON.
// Format: <!-- ATLAS_METADATA: {"task_id":"...","template":"...","workspace":"..."} -->
func writeMetadataSection(sb *strings.Builder, opts PRDescOptions) {
	if opts.TaskID == "" && opts.TemplateName == "" && opts.WorkspaceName == "" {
		return
	}

	meta := make(map[string]string)
	if opts.TaskID != "" {
		meta["task_id"] = opts.TaskID
	}
	if opts.TemplateName != "" {
		meta["template"] = opts.TemplateName
	}
	if opts.WorkspaceName != "" {
		meta["workspace"] = opts.WorkspaceName
	}

	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		// If metadata marshaling fails, skip the metadata comment
		// This is non-critical metadata, so we don't want to fail the entire operation
		return
	}
	_, _ = fmt.Fprintf(sb, "<!-- ATLAS_METADATA: %s -->\n", string(jsonBytes))
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
		// Take first sentence or first MaxPRSummaryLength chars
		summary := taskDesc
		if idx := strings.Index(summary, "."); idx > 0 && idx < 60 {
			summary = summary[:idx]
		}
		if len(summary) > constants.MaxPRSummaryLength {
			truncLen := constants.MaxPRSummaryLength - len(constants.PRSummaryTruncationSuffix)
			summary = summary[:truncLen] + constants.PRSummaryTruncationSuffix
		}
		return lowercaseFirst(strings.TrimSpace(summary))
	}

	// Fall back to first commit message
	if len(commits) > 0 {
		summary := commits[0]
		// Strip conventional commit prefix if present
		if idx := strings.Index(summary, ": "); idx > 0 && idx < constants.ConventionalCommitPrefixMaxLength {
			summary = summary[idx+2:]
		}
		if len(summary) > constants.MaxPRSummaryLength {
			truncLen := constants.MaxPRSummaryLength - len(constants.PRSummaryTruncationSuffix)
			summary = summary[:truncLen] + constants.PRSummaryTruncationSuffix
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

// stripMarkdownCodeBlocks removes markdown code block delimiters from AI output.
// AI models sometimes wrap their output in ```...``` blocks.
func stripMarkdownCodeBlocks(s string) string {
	// Remove fenced code blocks (```...``` or ```lang\n...\n```)
	if matches := codeBlockPattern.FindStringSubmatch(s); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return s
}

// Compile-time interface checks.
var (
	_ PRDescriptionGenerator = (*AIDescriptionGenerator)(nil)
	_ PRDescriptionGenerator = (*TemplateDescriptionGenerator)(nil)
)
