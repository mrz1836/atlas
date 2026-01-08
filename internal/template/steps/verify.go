// Package steps provides step execution implementations for the ATLAS task engine.
//
//nolint:funcorder // Methods are organized by functionality (related methods grouped together) rather than export status
package steps

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// Verification step configuration constants.
const (
	// DefaultVerifyMaxTurns is the maximum number of AI conversation turns for verification.
	DefaultVerifyMaxTurns = 3

	// DefaultVerifyTimeout is the timeout for verification AI requests.
	DefaultVerifyTimeout = 3 * time.Minute
)

// Verification step errors.
var (
	// ErrNoJSONObjectFound is returned when JSON parsing fails to find a valid JSON object.
	ErrNoJSONObjectFound = stderrors.New("no valid JSON object found")

	// ErrNoJSONArrayFound is returned when JSON parsing fails to find a valid JSON array.
	ErrNoJSONArrayFound = stderrors.New("no valid JSON array found")
)

// VerifyExecutor handles AI verification steps.
// It uses a secondary AI model to review implementation changes and detect potential issues.
type VerifyExecutor struct {
	runner          ai.Runner
	garbageDetector GarbageChecker
	logger          zerolog.Logger
	workingDir      string
	artifactHelper  *ArtifactHelper
}

// GarbageChecker defines the interface for garbage file detection.
// This allows for easier testing by enabling mock implementations.
type GarbageChecker interface {
	DetectGarbage(files []string) []git.GarbageFile
}

// VerifyExecutorOption is a functional option for configuring VerifyExecutor.
type VerifyExecutorOption func(*VerifyExecutor)

// WithVerifyWorkingDir sets the working directory for the verify executor.
// The working directory is used to set the Claude CLI's working directory,
// ensuring file operations happen in the correct location (e.g., worktree).
func WithVerifyWorkingDir(dir string) VerifyExecutorOption {
	return func(e *VerifyExecutor) {
		e.workingDir = dir
	}
}

// WithGarbageChecker sets a custom garbage checker for testing.
func WithGarbageChecker(checker GarbageChecker) VerifyExecutorOption {
	return func(e *VerifyExecutor) {
		e.garbageDetector = checker
	}
}

// NewVerifyExecutor creates a new verify executor with the given dependencies and options.
func NewVerifyExecutor(runner ai.Runner, garbageDetector *git.GarbageDetector, artifactSaver ArtifactSaver, logger zerolog.Logger, opts ...VerifyExecutorOption) *VerifyExecutor {
	e := &VerifyExecutor{
		runner:          runner,
		garbageDetector: garbageDetector,
		logger:          logger,
		artifactHelper:  NewArtifactHelper(artifactSaver, logger),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// NewVerifyExecutorWithWorkingDir creates a verify executor with a working directory.
// Deprecated: Use NewVerifyExecutor with WithVerifyWorkingDir option instead.
func NewVerifyExecutorWithWorkingDir(runner ai.Runner, garbageDetector *git.GarbageDetector, artifactSaver ArtifactSaver, logger zerolog.Logger, workingDir string) *VerifyExecutor {
	return NewVerifyExecutor(runner, garbageDetector, artifactSaver, logger, WithVerifyWorkingDir(workingDir))
}

// VerifyConfig configures the verification step.
type VerifyConfig struct {
	// Model to use for verification (different from implementation model).
	Model string `json:"model"`
	// Checks to run during verification.
	Checks []string `json:"checks"` // code_correctness, test_coverage, garbage_files, security
	// FailOnWarnings treats warnings as errors.
	FailOnWarnings bool `json:"fail_on_warnings"`
}

// DefaultVerifyConfig returns the default verification configuration.
// By default, only code_correctness is checked for speed. Other checks
// (test_coverage, garbage_files, security) can be enabled via step config.
func DefaultVerifyConfig() *VerifyConfig {
	return &VerifyConfig{
		Model:          "", // Use task default or step config
		Checks:         []string{"code_correctness"},
		FailOnWarnings: false,
	}
}

// VerificationIssue represents a single issue found during verification.
type VerificationIssue struct {
	Severity   string `json:"severity"`   // "error", "warning", "info"
	Category   string `json:"category"`   // "code_correctness", "test_coverage", "garbage", "security"
	File       string `json:"file"`       // File path where issue was found
	Line       int    `json:"line"`       // Line number (0 if not applicable)
	Message    string `json:"message"`    // Description of the issue
	Suggestion string `json:"suggestion"` // How to fix the issue
}

// VerificationResult holds the parsed verification response.
type VerificationResult struct {
	Passed  bool                `json:"passed"`
	Issues  []VerificationIssue `json:"issues"`
	Summary string              `json:"summary"`
}

// VerificationReport is the complete verification output artifact.
type VerificationReport struct {
	TaskID       string              `json:"task_id"`
	TaskDesc     string              `json:"task_description"`
	Summary      string              `json:"summary"`
	TotalIssues  int                 `json:"total_issues"`
	ErrorCount   int                 `json:"error_count"`
	WarningCount int                 `json:"warning_count"`
	InfoCount    int                 `json:"info_count"`
	Issues       []VerificationIssue `json:"issues"`
	PassedChecks []string            `json:"passed_checks"`
	FailedChecks []string            `json:"failed_checks"`
	Timestamp    time.Time           `json:"timestamp"`
	Duration     time.Duration       `json:"duration"`
}

// Execute runs an AI verification step.
// The step config may contain:
//   - model: string specifying which model to use for verification
//   - checks: []string of check types to run
//   - fail_on_warnings: bool to treat warnings as failures
func (e *VerifyExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	startTime := time.Now()

	// Build verification request first so we can log resolved agent/model
	req := e.buildRequest(task, step)

	// Get checks for logging
	checks := e.getChecksFromConfig(step.Config)

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Strs("checks", checks).
		Msg("executing verify step")

	// Debug log for verbose mode - shows exact request configuration
	e.logger.Debug().
		Str("task_id", task.ID).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Str("permission_mode", req.PermissionMode).
		Str("working_dir", req.WorkingDir).
		Dur("timeout", req.Timeout).
		Int("max_turns", req.MaxTurns).
		Int("prompt_length", len(req.Prompt)).
		Msg("AI request details")

	// Execute with timeout from step definition if set
	execCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Run AI verification
	result, err := e.runner.Run(execCtx, req)
	elapsed := time.Since(startTime)

	// Save AI artifact for audit trail (versioned to handle multiple verification calls)
	e.saveVerificationArtifact(ctx, task, step, req, result, startTime, elapsed, err)

	if err != nil {
		e.logger.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("step_name", step.Name).
			Str("agent", string(req.Agent)).
			Str("model", req.Model).
			Dur("duration_ms", elapsed).
			Msg("verify step failed")

		return &domain.StepResult{
			StepIndex:   task.CurrentStep,
			StepName:    step.Name,
			Status:      "failed",
			StartedAt:   startTime,
			CompletedAt: time.Now(),
			DurationMs:  elapsed.Milliseconds(),
			Error:       err.Error(),
		}, err
	}

	// Parse verification result
	verifyResult, parseErr := e.parseVerificationResult(result.Output)
	if parseErr != nil {
		e.logger.Warn().
			Err(parseErr).
			Str("task_id", task.ID).
			Str("output", result.Output).
			Msg("failed to parse verification result, using raw output")
	}

	// Determine output and metadata
	output := result.Output
	metadata := make(map[string]any)
	if verifyResult != nil {
		metadata["passed"] = verifyResult.Passed
		metadata["issue_count"] = len(verifyResult.Issues)
		metadata["summary"] = verifyResult.Summary
	}

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("agent", string(req.Agent)).
		Str("model", req.Model).
		Str("session_id", result.SessionID).
		Int("num_turns", result.NumTurns).
		Dur("duration_ms", elapsed).
		Interface("metadata", metadata).
		Msg("verify step completed")

	return &domain.StepResult{
		StepIndex:   task.CurrentStep,
		StepName:    step.Name,
		Status:      "success",
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  elapsed.Milliseconds(),
		Output:      output,
		SessionID:   result.SessionID,
		NumTurns:    result.NumTurns,
	}, nil
}

// Type returns the step type this executor handles.
func (e *VerifyExecutor) Type() domain.StepType {
	return domain.StepTypeVerify
}

// buildRequest constructs an AIRequest for verification.
func (e *VerifyExecutor) buildRequest(task *domain.Task, step *domain.StepDefinition) *domain.AIRequest {
	req := &domain.AIRequest{
		Agent:          task.Config.Agent, // Default to task agent
		Prompt:         e.buildVerificationPrompt(task, step),
		Model:          task.Config.Model,
		MaxTurns:       5, // Verification should be quick
		Timeout:        5 * time.Minute,
		WorkingDir:     e.workingDir,
		PermissionMode: "plan", // Default to read-only for verification (safety)
	}

	// Apply step-specific config overrides
	if step.Config == nil {
		return req
	}

	// Agent override for this step
	agentChanged := false
	if agent, ok := step.Config["agent"].(string); ok && agent != "" {
		newAgent := domain.Agent(agent)
		// Only consider it a change if it's actually different
		if newAgent != req.Agent {
			req.Agent = newAgent
			agentChanged = true
		}
	}

	if model, ok := step.Config["model"].(string); ok && model != "" {
		req.Model = model
	} else if agentChanged {
		// Use new agent's default model when agent changed but model wasn't specified
		req.Model = req.Agent.DefaultModel()
	}

	if timeout, ok := step.Config["timeout"].(time.Duration); ok {
		req.Timeout = timeout
	}

	// Allow override of permission mode (opt-out of read-only)
	if mode, ok := step.Config["permission_mode"].(string); ok {
		req.PermissionMode = mode
	}

	return req
}

// buildVerificationPrompt creates the prompt for AI verification.
// The prompt is kept concise to minimize AI processing time.
func (e *VerifyExecutor) buildVerificationPrompt(task *domain.Task, step *domain.StepDefinition) string {
	checks := e.getChecksFromConfig(step.Config)

	// Build a concise prompt - the simpler the prompt, the faster the response
	prompt := fmt.Sprintf(`Review if the implementation matches the task.

## Task
%s

## Instructions
Quickly verify:
%s

Respond with JSON:
{"passed": true/false, "issues": [{"severity": "error|warning", "category": "code_correctness", "file": "path", "line": 0, "message": "issue", "suggestion": "fix"}], "summary": "Brief assessment"}
`, task.Description, e.formatChecksCompact(checks))

	return prompt
}

// getChecksFromConfig extracts the checks list from step config.
func (e *VerifyExecutor) getChecksFromConfig(config map[string]any) []string {
	if config == nil {
		return DefaultVerifyConfig().Checks
	}

	if checks, ok := config["checks"].([]string); ok {
		return checks
	}

	// Handle []any from JSON unmarshaling
	if checksAny, ok := config["checks"].([]any); ok {
		checks := make([]string, 0, len(checksAny))
		for _, c := range checksAny {
			if s, ok := c.(string); ok {
				checks = append(checks, s)
			}
		}
		if len(checks) > 0 {
			return checks
		}
	}

	return DefaultVerifyConfig().Checks
}

// formatChecksCompact formats the checks list for the prompt (compact version for speed).
func (e *VerifyExecutor) formatChecksCompact(checks []string) string {
	checkDescriptions := map[string]string{
		"code_correctness": "1. Does the code address the task? Any obvious bugs?",
		"test_coverage":    "2. Are there tests for the changes?",
		"garbage_files":    "3. Any temp/debug files to remove?",
		"security":         "4. Any hardcoded secrets or vulnerabilities?",
	}

	result := ""
	for _, check := range checks {
		if desc, ok := checkDescriptions[check]; ok {
			result += desc + "\n"
		}
	}
	return result
}

// parseVerificationResult parses the JSON verification result from AI output.
func (e *VerifyExecutor) parseVerificationResult(output string) (*VerificationResult, error) {
	var result VerificationResult

	// Try direct parse first
	if err := json.Unmarshal([]byte(output), &result); err == nil {
		return &result, nil
	}

	// Try extracting JSON object from output
	jsonOutput, ok := extractJSONObject(output)
	if !ok {
		return nil, fmt.Errorf("failed to parse verification result: %w", ErrNoJSONObjectFound)
	}

	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
		return nil, fmt.Errorf("failed to parse verification result: %w", err)
	}

	return &result, nil
}

// extractJSONObject extracts a JSON object from text that may contain non-JSON content.
func extractJSONObject(output string) (string, bool) {
	jsonStart := strings.Index(output, "{")
	jsonEnd := strings.LastIndex(output, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		return output[jsonStart : jsonEnd+1], true
	}
	return "", false
}

// CheckCodeCorrectness uses AI to review code changes against task description.
// Returns issues related to implementation correctness.
func (e *VerifyExecutor) CheckCodeCorrectness(ctx context.Context, taskDescription string, changedFiles []ChangedFile) ([]VerificationIssue, error) {
	if len(changedFiles) == 0 {
		return nil, nil
	}

	prompt := fmt.Sprintf(`Review the following code changes for correctness against the task description.

## Task Description
%s

## Changed Files
%s

## Instructions
Check if the implementation:
1. Correctly addresses the task description
2. Has any obvious bugs or logic errors
3. Follows Go best practices (if Go code)
4. Has proper error handling

Respond with JSON array of issues found:
[{"severity": "error|warning|info", "file": "path", "line": 0, "message": "description", "suggestion": "how to fix"}]

If no issues, respond with: []
`, taskDescription, e.formatChangedFiles(changedFiles))

	req := &domain.AIRequest{
		Prompt:     prompt,
		MaxTurns:   DefaultVerifyMaxTurns,
		Timeout:    DefaultVerifyTimeout,
		WorkingDir: e.workingDir,
	}

	result, err := e.runner.Run(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to check code correctness: %w", err)
	}

	return e.parseIssuesResponse(result.Output, "code_correctness")
}

// CheckTestCoverage verifies that tests exist for changed code.
// Returns issues related to missing or inadequate test coverage.
func (e *VerifyExecutor) CheckTestCoverage(_ context.Context, changedFiles []ChangedFile) ([]VerificationIssue, error) {
	var issues []VerificationIssue

	// Check for corresponding test files
	for _, f := range changedFiles {
		// Skip test files themselves
		if isTestFile(f.Path) {
			continue
		}

		// Skip non-Go files for now
		if !isGoFile(f.Path) {
			continue
		}

		// Check if there's a corresponding test file
		testFile := toTestFileName(f.Path)
		hasTest := false
		for _, tf := range changedFiles {
			if tf.Path == testFile {
				hasTest = true
				break
			}
		}

		if !hasTest {
			issues = append(issues, VerificationIssue{
				Severity:   "warning",
				Category:   "test_coverage",
				File:       f.Path,
				Line:       0,
				Message:    "No corresponding test file found for modified code",
				Suggestion: fmt.Sprintf("Consider adding tests in %s", testFile),
			})
		}
	}

	return issues, nil
}

// CheckGarbageFiles detects garbage files that shouldn't be committed.
// Reuses the GarbageDetector from internal/git package.
func (e *VerifyExecutor) CheckGarbageFiles(_ context.Context, stagedFiles []string) ([]VerificationIssue, error) {
	checker := e.garbageDetector
	if checker == nil {
		checker = git.NewGarbageDetector(nil)
	}

	garbageFiles := checker.DetectGarbage(stagedFiles)

	issues := make([]VerificationIssue, 0, len(garbageFiles))
	for _, gf := range garbageFiles {
		severity := "warning"
		if gf.Category == git.GarbageSecrets {
			severity = "error"
		}

		issues = append(issues, VerificationIssue{
			Severity:   severity,
			Category:   "garbage",
			File:       gf.Path,
			Line:       0,
			Message:    fmt.Sprintf("Garbage file detected (%s): %s", gf.Category, gf.Reason),
			Suggestion: "Remove this file before committing",
		})
	}

	return issues, nil
}

// CheckSecurityIssues scans for common security vulnerabilities.
// Uses pattern matching for common issues like hardcoded secrets.
func (e *VerifyExecutor) CheckSecurityIssues(_ context.Context, changedFiles []ChangedFile) ([]VerificationIssue, error) {
	issues := make([]VerificationIssue, 0, len(changedFiles))

	for _, f := range changedFiles {
		fileIssues := e.scanForSecurityPatterns(f)
		issues = append(issues, fileIssues...)
	}

	return issues, nil
}

// ChangedFile represents a file that was modified in the implementation.
type ChangedFile struct {
	Path     string // File path relative to repo root
	Language string // Programming language (inferred from extension)
	Content  string // File content (may be diff or full content)
}

// securityPattern defines a pattern to detect security issues.
type securityPattern struct {
	Name        string
	Regex       *regexp.Regexp
	Description string
	Severity    string
}

// Common security patterns to detect.
// Patterns are pre-compiled at package initialization for performance.
//
//nolint:gochecknoglobals // Constant-like data structure, initialized once
var securityPatterns = []securityPattern{
	{
		Name:        "hardcoded_password",
		Regex:       regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]+\s*["'][^"']+["']`),
		Description: "Hardcoded password detected",
		Severity:    "error",
	},
	{
		Name:        "hardcoded_secret",
		Regex:       regexp.MustCompile(`(?i)(secret|api[_-]?key|auth[_-]?token)\s*[:=]+\s*["'][^"']+["']`),
		Description: "Hardcoded secret or API key detected",
		Severity:    "error",
	},
	{
		Name:        "sql_concatenation",
		Regex:       regexp.MustCompile(`(?i)(exec|query).*\+.*\$|fmt\.Sprintf.*SELECT|fmt\.Sprintf.*INSERT|fmt\.Sprintf.*UPDATE|fmt\.Sprintf.*DELETE`),
		Description: "Potential SQL injection - string concatenation in query",
		Severity:    "warning",
	},
	{
		Name:        "exec_with_var",
		Regex:       regexp.MustCompile(`exec\.Command\([^"]+\)|exec\.CommandContext\([^,]+,[^"]+\)`),
		Description: "Potential command injection - variable in exec",
		Severity:    "warning",
	},
}

// scanForSecurityPatterns scans a file for security issues using regex patterns.
func (e *VerifyExecutor) scanForSecurityPatterns(file ChangedFile) []VerificationIssue {
	var issues []VerificationIssue

	// Skip test files for security scanning - they often have test credentials
	if isTestFile(file.Path) {
		return issues
	}

	for _, pattern := range securityPatterns {
		matches := findPatternMatches(file.Content, pattern.Regex)
		for _, match := range matches {
			issues = append(issues, VerificationIssue{
				Severity:   pattern.Severity,
				Category:   "security",
				File:       file.Path,
				Line:       match.Line,
				Message:    pattern.Description,
				Suggestion: "Review and fix potential security issue",
			})
		}
	}

	return issues
}

// patternMatch represents a regex match with line number.
type patternMatch struct {
	Line    int
	Content string
}

// findPatternMatches finds all matches of a pre-compiled regex in content with line numbers.
func findPatternMatches(content string, re *regexp.Regexp) []patternMatch {
	var matches []patternMatch

	lines := splitLines(content)
	for i, line := range lines {
		if re.MatchString(line) {
			matches = append(matches, patternMatch{
				Line:    i + 1, // 1-indexed
				Content: line,
			})
		}
	}

	return matches
}

// splitLines splits content into lines.
func splitLines(content string) []string {
	return strings.Split(content, "\n")
}

// isTestFile checks if a file path is a test file.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/testdata/")
}

// isGoFile checks if a file path is a Go source file.
func isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}

// toTestFileName converts a Go file path to its test file path.
func toTestFileName(path string) string {
	if strings.HasSuffix(path, ".go") {
		return strings.TrimSuffix(path, ".go") + "_test.go"
	}
	return path + "_test"
}

// formatChangedFiles formats changed files for the AI prompt.
func (e *VerifyExecutor) formatChangedFiles(files []ChangedFile) string {
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("### %s\n", f.Path))
		if f.Language != "" {
			sb.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", f.Language, f.Content))
		} else {
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", f.Content))
		}
	}
	return sb.String()
}

// parseIssuesResponse parses a JSON array of issues from AI output.
func (e *VerifyExecutor) parseIssuesResponse(output, category string) ([]VerificationIssue, error) {
	var issues []VerificationIssue

	// Try direct parse first
	if err := json.Unmarshal([]byte(output), &issues); err == nil {
		setCategoryForIssues(issues, category)
		return issues, nil
	}

	// Try extracting JSON array from output
	jsonOutput, ok := extractJSONArray(output)
	if !ok {
		return nil, fmt.Errorf("failed to parse issues response: %w", ErrNoJSONArrayFound)
	}

	if err := json.Unmarshal([]byte(jsonOutput), &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues response: %w", err)
	}

	setCategoryForIssues(issues, category)
	return issues, nil
}

// extractJSONArray extracts a JSON array from text that may contain non-JSON content.
func extractJSONArray(output string) (string, bool) {
	jsonStart := strings.Index(output, "[")
	jsonEnd := strings.LastIndex(output, "]")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		return output[jsonStart : jsonEnd+1], true
	}
	return "", false
}

// setCategoryForIssues sets the category field for all issues in the slice.
func setCategoryForIssues(issues []VerificationIssue, category string) {
	for i := range issues {
		issues[i].Category = category
	}
}

// GenerateVerificationReport creates a VerificationReport from check results.
func GenerateVerificationReport(taskID, taskDesc string, issues []VerificationIssue, passedChecks, failedChecks []string, startTime time.Time) *VerificationReport {
	report := &VerificationReport{
		TaskID:       taskID,
		TaskDesc:     taskDesc,
		Issues:       issues,
		PassedChecks: passedChecks,
		FailedChecks: failedChecks,
		Timestamp:    startTime,
		Duration:     time.Since(startTime),
		TotalIssues:  len(issues),
	}

	// Count issues by severity
	for _, issue := range issues {
		switch issue.Severity {
		case "error":
			report.ErrorCount++
		case "warning":
			report.WarningCount++
		case "info":
			report.InfoCount++
		}
	}

	// Generate summary
	if report.TotalIssues == 0 {
		report.Summary = "All verification checks passed successfully"
	} else {
		report.Summary = fmt.Sprintf("Found %d issue(s): %d error(s), %d warning(s), %d info",
			report.TotalIssues, report.ErrorCount, report.WarningCount, report.InfoCount)
	}

	return report
}

// SaveVerificationReport saves the report to a markdown file.
func SaveVerificationReport(report *VerificationReport, artifactDir string) (string, error) {
	// Ensure directory exists
	if err := os.MkdirAll(artifactDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create artifact directory: %w", err)
	}

	filename := filepath.Join(artifactDir, "verification-report.md")

	content := formatReportAsMarkdown(report)

	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write verification report: %w", err)
	}

	return filename, nil
}

// formatReportAsMarkdown formats the verification report as markdown.
//
//nolint:gocognit // Formatting function with multiple sections, kept together for clarity
func formatReportAsMarkdown(report *VerificationReport) string {
	var sb strings.Builder

	sb.WriteString("# Verification Report\n\n")
	sb.WriteString(fmt.Sprintf("**Task ID:** %s\n", report.TaskID))
	sb.WriteString(fmt.Sprintf("**Task Description:** %s\n", report.TaskDesc))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n", report.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Duration:** %s\n\n", report.Duration.Round(time.Millisecond)))

	// Summary section
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("%s\n\n", report.Summary))

	// Statistics
	sb.WriteString("### Statistics\n\n")
	sb.WriteString("| Severity | Count |\n")
	sb.WriteString("|----------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Errors   | %d    |\n", report.ErrorCount))
	sb.WriteString(fmt.Sprintf("| Warnings | %d    |\n", report.WarningCount))
	sb.WriteString(fmt.Sprintf("| Info     | %d    |\n\n", report.InfoCount))

	// Passed checks
	if len(report.PassedChecks) > 0 {
		sb.WriteString("### Passed Checks\n\n")
		for _, check := range report.PassedChecks {
			sb.WriteString(fmt.Sprintf("- ✅ %s\n", check))
		}
		sb.WriteString("\n")
	}

	// Failed checks
	if len(report.FailedChecks) > 0 {
		sb.WriteString("### Failed Checks\n\n")
		for _, check := range report.FailedChecks {
			sb.WriteString(fmt.Sprintf("- ❌ %s\n", check))
		}
		sb.WriteString("\n")
	}

	// Issues section
	if len(report.Issues) > 0 {
		sb.WriteString("## Issues\n\n")

		// Group issues by severity
		errors := filterIssuesBySeverity(report.Issues, "error")
		warnings := filterIssuesBySeverity(report.Issues, "warning")
		infos := filterIssuesBySeverity(report.Issues, "info")

		if len(errors) > 0 {
			sb.WriteString("### Errors\n\n")
			for _, issue := range errors {
				sb.WriteString(formatIssueMarkdown(issue))
			}
		}

		if len(warnings) > 0 {
			sb.WriteString("### Warnings\n\n")
			for _, issue := range warnings {
				sb.WriteString(formatIssueMarkdown(issue))
			}
		}

		if len(infos) > 0 {
			sb.WriteString("### Info\n\n")
			for _, issue := range infos {
				sb.WriteString(formatIssueMarkdown(issue))
			}
		}
	}

	return sb.String()
}

// formatIssueMarkdown formats a single issue as markdown.
func formatIssueMarkdown(issue VerificationIssue) string {
	var sb strings.Builder

	// File and line info
	location := issue.File
	if issue.Line > 0 {
		location = fmt.Sprintf("%s:%d", issue.File, issue.Line)
	}

	sb.WriteString(fmt.Sprintf("#### %s\n\n", location))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n\n", issue.Category))
	sb.WriteString(fmt.Sprintf("**Message:** %s\n\n", issue.Message))

	if issue.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("**Suggestion:** %s\n\n", issue.Suggestion))
	}

	sb.WriteString("---\n\n")

	return sb.String()
}

// filterIssuesBySeverity returns issues of a specific severity.
func filterIssuesBySeverity(issues []VerificationIssue, severity string) []VerificationIssue {
	var filtered []VerificationIssue
	for _, issue := range issues {
		if issue.Severity == severity {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// VerificationAction represents the action chosen to handle verification issues.
type VerificationAction int

const (
	// VerifyActionAutoFix attempts to auto-fix issues using AI.
	VerifyActionAutoFix VerificationAction = iota
	// VerifyActionManualFix user fixes manually and resumes.
	VerifyActionManualFix
	// VerifyActionIgnoreContinue proceeds despite warnings.
	VerifyActionIgnoreContinue
	// VerifyActionViewReport displays the full verification report.
	VerifyActionViewReport
)

// String returns a string representation of the action.
func (a VerificationAction) String() string {
	switch a {
	case VerifyActionAutoFix:
		return "auto_fix"
	case VerifyActionManualFix:
		return "manual_fix"
	case VerifyActionIgnoreContinue:
		return "ignore_continue"
	case VerifyActionViewReport:
		return "view_report"
	default:
		return "unknown"
	}
}

// VerificationHandleResult represents the result of handling verification issues.
type VerificationHandleResult struct {
	// Action is the action that was taken.
	Action VerificationAction
	// ShouldContinue indicates if the task should continue.
	ShouldContinue bool
	// AwaitingManualFix indicates if waiting for manual intervention.
	AwaitingManualFix bool
	// AutoFixAttempted indicates if auto-fix was attempted.
	AutoFixAttempted bool
	// AutoFixSuccess indicates if auto-fix succeeded.
	AutoFixSuccess bool
	// Message is a human-readable message about the result.
	Message string
}

// HandleVerificationIssues handles verification issues based on the chosen action.
func (e *VerifyExecutor) HandleVerificationIssues(ctx context.Context, report *VerificationReport, action VerificationAction) (*VerificationHandleResult, error) {
	switch action {
	case VerifyActionAutoFix:
		return e.handleAutoFix(ctx, report)
	case VerifyActionManualFix:
		return e.handleManualFix(report)
	case VerifyActionIgnoreContinue:
		return e.handleIgnoreContinue(report)
	case VerifyActionViewReport:
		return e.handleViewReport(report)
	default:
		return nil, fmt.Errorf("%w: %d", errors.ErrInvalidVerificationAction, action)
	}
}

// handleAutoFix attempts to fix issues using AI.
func (e *VerifyExecutor) handleAutoFix(ctx context.Context, report *VerificationReport) (*VerificationHandleResult, error) {
	e.logger.Info().
		Str("task_id", report.TaskID).
		Int("issue_count", report.TotalIssues).
		Msg("attempting auto-fix for verification issues")

	// Build prompt with issue context
	prompt := e.buildAutoFixPrompt(report)

	req := &domain.AIRequest{
		Prompt:     prompt,
		MaxTurns:   10,
		Timeout:    10 * time.Minute,
		WorkingDir: e.workingDir,
	}

	result, err := e.runner.Run(ctx, req)
	if err != nil {
		e.logger.Error().Err(err).Msg("auto-fix attempt failed")
		return &VerificationHandleResult{
			Action:           VerifyActionAutoFix,
			ShouldContinue:   false,
			AutoFixAttempted: true,
			AutoFixSuccess:   false,
			Message:          fmt.Sprintf("Auto-fix failed: %v", err),
		}, nil
	}

	e.logger.Info().
		Str("session_id", result.SessionID).
		Int("files_changed", len(result.FilesChanged)).
		Msg("auto-fix completed")

	return &VerificationHandleResult{
		Action:           VerifyActionAutoFix,
		ShouldContinue:   true,
		AutoFixAttempted: true,
		AutoFixSuccess:   true,
		Message:          fmt.Sprintf("Auto-fix completed: %d files modified", len(result.FilesChanged)),
	}, nil
}

// buildAutoFixPrompt creates the prompt for AI auto-fix.
func (e *VerifyExecutor) buildAutoFixPrompt(report *VerificationReport) string {
	var sb strings.Builder

	sb.WriteString("Please fix the following verification issues found in the code:\n\n")
	sb.WriteString(fmt.Sprintf("## Task: %s\n", report.TaskDesc))
	sb.WriteString(fmt.Sprintf("## Issues Found: %d\n\n", report.TotalIssues))

	// Group by severity
	titleCaser := cases.Title(language.English)
	for _, severity := range []string{"error", "warning", "info"} {
		issues := filterIssuesBySeverity(report.Issues, severity)
		if len(issues) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s Issues (%d)\n", titleCaser.String(severity), len(issues)))
		for _, issue := range issues {
			sb.WriteString(fmt.Sprintf("- **%s** (line %d): %s\n", issue.File, issue.Line, issue.Message))
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("  Suggestion: %s\n", issue.Suggestion))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Instructions\n")
	sb.WriteString("1. Fix each issue listed above\n")
	sb.WriteString("2. Make minimal changes necessary\n")
	sb.WriteString("3. Ensure all tests still pass after changes\n")
	sb.WriteString("4. Do not introduce new issues\n")

	return sb.String()
}

// handleManualFix returns instructions for manual fix.
func (e *VerifyExecutor) handleManualFix(report *VerificationReport) (*VerificationHandleResult, error) {
	e.logger.Info().
		Str("task_id", report.TaskID).
		Msg("awaiting manual fix for verification issues")

	message := fmt.Sprintf("Manual fix requested. Please address the %d issue(s) and resume the task.\n", report.TotalIssues)
	message += "Issues are documented in the verification report."

	return &VerificationHandleResult{
		Action:            VerifyActionManualFix,
		ShouldContinue:    false,
		AwaitingManualFix: true,
		Message:           message,
	}, nil
}

// handleIgnoreContinue logs warning and continues.
func (e *VerifyExecutor) handleIgnoreContinue(report *VerificationReport) (*VerificationHandleResult, error) {
	e.logger.Warn().
		Str("task_id", report.TaskID).
		Int("error_count", report.ErrorCount).
		Int("warning_count", report.WarningCount).
		Msg("continuing despite verification issues")

	message := fmt.Sprintf("Ignoring %d verification issue(s) and continuing.", report.TotalIssues)
	if report.ErrorCount > 0 {
		message += fmt.Sprintf(" WARNING: %d error(s) ignored.", report.ErrorCount)
	}

	return &VerificationHandleResult{
		Action:         VerifyActionIgnoreContinue,
		ShouldContinue: true,
		Message:        message,
	}, nil
}

// handleViewReport returns the report for viewing.
func (e *VerifyExecutor) handleViewReport(report *VerificationReport) (*VerificationHandleResult, error) {
	// Format report as markdown for viewing
	content := formatReportAsMarkdown(report)

	return &VerificationHandleResult{
		Action:         VerifyActionViewReport,
		ShouldContinue: false, // After viewing, need another action choice
		Message:        content,
	}, nil
}

// saveVerificationArtifact saves the verification AI request/response as an artifact for audit trail.
// Uses versioned saving to handle multiple verification attempts without overwriting.
// This is non-blocking - artifact save failures are logged but don't fail the task.
func (e *VerifyExecutor) saveVerificationArtifact(ctx context.Context, task *domain.Task, step *domain.StepDefinition,
	req *domain.AIRequest, result *domain.AIResult, startTime time.Time, elapsed time.Duration, runErr error,
) {
	if e.artifactHelper == nil {
		return
	}

	artifact := &ai.Artifact{
		Timestamp:       startTime,
		StepName:        step.Name,
		StepIndex:       task.CurrentStep,
		Agent:           string(req.Agent),
		Model:           req.Model,
		Request:         req,
		Response:        result,
		ExecutionTimeMs: elapsed.Milliseconds(),
		Success:         runErr == nil,
	}

	if runErr != nil {
		artifact.ErrorMessage = runErr.Error()
	}

	// Use versioned saving for multiple verification attempts
	path, err := e.artifactHelper.SaveAIInteractionVersioned(ctx, task, "verify_step", artifact)
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to save verification artifact (non-fatal)")
	} else if path != "" {
		e.logger.Debug().
			Str("artifact_path", path).
			Msg("saved verification interaction artifact")
	}
}
