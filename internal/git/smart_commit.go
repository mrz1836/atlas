// Package git provides Git operations for ATLAS.
// This file implements the SmartCommitRunner for intelligent commit operations.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ai"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Compile-time interface check.
var _ SmartCommitService = (*SmartCommitRunner)(nil)

// SmartCommitRunner implements SmartCommitService using the git CLI.
type SmartCommitRunner struct {
	runner             Runner           // Git runner for CLI operations
	aiRunner           ai.Runner        // AI runner for commit message generation
	garbageDetector    *GarbageDetector // Garbage file detector
	logger             zerolog.Logger   // Logger for operations
	workDir            string           // Working directory
	taskID             string           // Current task ID (kept for artifact metadata)
	templateName       string           // Template name (kept for artifact metadata)
	artifactsDir       string           // Directory for saving artifacts
	agent              string           // AI agent for commit message generation
	model              string           // AI model for commit message generation
	timeout            time.Duration    // Timeout for AI commit message generation
	maxRetries         int              // Maximum number of retry attempts
	retryBackoffFactor float64          // Exponential backoff factor for retries
}

// SmartCommitRunnerOption configures a SmartCommitRunner.
type SmartCommitRunnerOption func(*SmartCommitRunner)

// WithTaskID sets the task ID for ATLAS trailers.
func WithTaskID(taskID string) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.taskID = taskID
	}
}

// WithTemplateName sets the template name for ATLAS trailers.
func WithTemplateName(name string) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.templateName = name
	}
}

// WithArtifactsDir sets the directory for saving commit artifacts.
func WithArtifactsDir(dir string) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.artifactsDir = dir
	}
}

// WithGarbageConfig sets a custom garbage detection config.
func WithGarbageConfig(config *GarbageConfig) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.garbageDetector = NewGarbageDetector(config)
	}
}

// WithModel sets the AI model for commit message generation.
// If not set, the AIRunner's default model is used.
func WithModel(model string) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.model = model
	}
}

// WithAgent sets the AI agent for commit message generation.
// If not set, the AIRunner's default agent is used.
func WithAgent(agent string) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.agent = agent
	}
}

// WithTimeout sets the timeout for AI commit message generation.
// If not set, a default of 30 seconds is used.
func WithTimeout(timeout time.Duration) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.timeout = timeout
	}
}

// WithMaxRetries sets the maximum number of retry attempts for AI generation.
// If not set, a default of 2 retries is used.
func WithMaxRetries(maxRetries int) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.maxRetries = maxRetries
	}
}

// WithRetryBackoffFactor sets the exponential backoff factor for retries.
// If not set, a default of 1.5 is used.
func WithRetryBackoffFactor(factor float64) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.retryBackoffFactor = factor
	}
}

// WithLogger sets the logger for the runner.
func WithLogger(logger zerolog.Logger) SmartCommitRunnerOption {
	return func(r *SmartCommitRunner) {
		r.logger = logger
	}
}

// NewSmartCommitRunner creates a new SmartCommitRunner.
// The aiRunner is optional - if nil, simple message generation will be used.
func NewSmartCommitRunner(gitRunner Runner, workDir string, aiRunner ai.Runner, opts ...SmartCommitRunnerOption) *SmartCommitRunner {
	runner := &SmartCommitRunner{
		runner:             gitRunner,
		aiRunner:           aiRunner,
		garbageDetector:    NewGarbageDetector(nil),
		logger:             zerolog.Nop(),
		workDir:            workDir,
		timeout:            30 * time.Second, // Default timeout
		maxRetries:         2,                // Default max retries
		retryBackoffFactor: 1.5,              // Default backoff factor
	}

	for _, opt := range opts {
		opt(runner)
	}

	return runner
}

// Analyze inspects the current worktree and returns a commit analysis.
func (r *SmartCommitRunner) Analyze(ctx context.Context) (*CommitAnalysis, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	r.logger.Debug().Str("work_dir", r.workDir).Msg("analyzing worktree for smart commit")

	// Get current status
	status, err := r.runner.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	// Collect all changed files
	allFiles := make([]FileChange, 0, len(status.Staged)+len(status.Unstaged)+len(status.Untracked))
	allFiles = append(allFiles, status.Staged...)
	allFiles = append(allFiles, status.Unstaged...)

	// Add untracked files as additions
	for _, path := range status.Untracked {
		allFiles = append(allFiles, FileChange{
			Path:   path,
			Status: ChangeAdded,
		})
	}

	if len(allFiles) == 0 {
		return &CommitAnalysis{
			FileGroups:   []FileGroup{},
			GarbageFiles: []GarbageFile{},
			TotalChanges: 0,
			HasGarbage:   false,
		}, nil
	}

	// Extract paths for garbage detection
	paths := GetFilePaths(allFiles)

	// Detect garbage files
	garbageFiles := r.garbageDetector.DetectGarbage(paths)

	// Filter out garbage from files to commit (for grouping)
	garbageSet := make(map[string]bool)
	for _, g := range garbageFiles {
		garbageSet[g.Path] = true
	}

	var cleanFiles []FileChange
	for _, f := range allFiles {
		if !garbageSet[f.Path] {
			cleanFiles = append(cleanFiles, f)
		}
	}

	// Group files by package
	fileGroups := GroupFilesByPackage(cleanFiles)

	r.logger.Debug().
		Int("total_files", len(allFiles)).
		Int("garbage_files", len(garbageFiles)).
		Int("file_groups", len(fileGroups)).
		Msg("analysis complete")

	return &CommitAnalysis{
		FileGroups:   fileGroups,
		GarbageFiles: garbageFiles,
		TotalChanges: len(allFiles),
		HasGarbage:   len(garbageFiles) > 0,
	}, nil
}

// Commit creates one or more commits based on the analysis.
func (r *SmartCommitRunner) Commit(ctx context.Context, opts CommitOptions) (*CommitResult, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Analyze and handle garbage files
	analysis, err := r.analyzeAndHandleGarbage(ctx, opts)
	if err != nil {
		return nil, err
	}

	if len(analysis.FileGroups) == 0 {
		return nil, fmt.Errorf("no files to commit: %w", atlaserrors.ErrGitOperation)
	}

	// Dry run just returns what would be committed
	if opts.DryRun {
		return r.buildDryRunResult(analysis, opts)
	}

	// Determine groups and perform commits
	groups := r.determineCommitGroups(analysis, opts)
	commits, err := r.performCommits(ctx, groups)
	if err != nil {
		return nil, err
	}

	return r.buildCommitResult(commits)
}

// analyzeAndHandleGarbage runs analysis and processes garbage files based on options.
func (r *SmartCommitRunner) analyzeAndHandleGarbage(ctx context.Context, opts CommitOptions) (*CommitAnalysis, error) {
	analysis, err := r.Analyze(ctx)
	if err != nil {
		return nil, err
	}

	// Check for garbage if not skipping
	if !opts.SkipGarbageCheck && analysis.HasGarbage && !opts.IncludeGarbage {
		return nil, fmt.Errorf("garbage files detected (use SkipGarbageCheck or IncludeGarbage): %w", atlaserrors.ErrGitOperation)
	}

	// If IncludeGarbage is true, add garbage files back to the groups
	if opts.IncludeGarbage && len(analysis.GarbageFiles) > 0 {
		analysis = r.addGarbageToGroups(analysis)
	}

	return analysis, nil
}

// determineCommitGroups decides whether to use multiple groups or combine into one.
func (r *SmartCommitRunner) determineCommitGroups(analysis *CommitAnalysis, opts CommitOptions) []FileGroup {
	if !opts.SingleCommit {
		return analysis.FileGroups
	}

	// Collect all files into single group
	var allFiles []FileChange
	for _, g := range analysis.FileGroups {
		allFiles = append(allFiles, g.Files...)
	}
	return GroupFilesForSingleCommit(allFiles)
}

// performCommits creates commits for each group and returns commit info.
func (r *SmartCommitRunner) performCommits(ctx context.Context, groups []FileGroup) ([]CommitInfo, error) {
	commits := make([]CommitInfo, 0, len(groups))

	for _, group := range groups {
		commit, err := r.commitGroup(ctx, group)
		if err != nil {
			return nil, fmt.Errorf("failed to commit group '%s': %w", group.Package, err)
		}
		commits = append(commits, *commit)
	}

	return commits, nil
}

// buildCommitResult saves artifacts and constructs the final result.
func (r *SmartCommitRunner) buildCommitResult(commits []CommitInfo) (*CommitResult, error) {
	totalFiles := 0
	for _, c := range commits {
		totalFiles += c.FileCount
	}

	// Save artifact
	artifactPath := ""
	if r.artifactsDir != "" {
		var err error
		artifactPath, err = r.saveArtifact(commits)
		if err != nil {
			r.logger.Warn().Err(err).Msg("failed to save commit artifact")
			// Don't fail the operation, just log the warning
		}
	}

	r.logger.Info().
		Int("commits", len(commits)).
		Int("total_files", totalFiles).
		Str("artifact_path", artifactPath).
		Msg("smart commit completed")

	return &CommitResult{
		Commits:      commits,
		ArtifactPath: artifactPath,
		TotalFiles:   totalFiles,
	}, nil
}

// commitGroup stages files and creates a commit for a single group.
func (r *SmartCommitRunner) commitGroup(ctx context.Context, group FileGroup) (*CommitInfo, error) {
	// Reset staging to ensure clean state for this group
	// This prevents files from other groups (or pre-staged files) from being included
	// Use lock retry to handle concurrent git operations
	err := RunWithLockRetryVoid(ctx, DefaultLockRetryConfig(), r.logger, func(ctx context.Context) error {
		return r.runner.Reset(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to reset staging: %w", err)
	}

	// Stage only this group's files with lock retry
	paths := GetFilePaths(group.Files)
	err = RunWithLockRetryVoid(ctx, DefaultLockRetryConfig(), r.logger, func(ctx context.Context) error {
		return r.runner.Add(ctx, paths)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to stage files: %w", err)
	}

	// Generate commit message
	message := r.generateCommitMessage(ctx, group)

	// Create the commit with lock retry to handle concurrent git operations
	err = RunWithLockRetryVoid(ctx, DefaultLockRetryConfig(), r.logger, func(ctx context.Context) error {
		return r.runner.Commit(ctx, message)
	})
	if err != nil {
		return nil, err
	}

	// Get the commit hash (we just created it, so HEAD is our commit)
	hash, err := r.getHeadShortHash(ctx)
	if err != nil {
		hash = "unknown"
	}

	r.logger.Debug().
		Str("hash", hash).
		Str("package", group.Package).
		Int("files", len(paths)).
		Msg("created commit")

	return &CommitInfo{
		Hash:         hash,
		Message:      message,
		FileCount:    len(paths),
		Package:      group.Package,
		CommitType:   group.CommitType,
		FilesChanged: paths,
	}, nil
}

// generateCommitMessage generates a commit message for a file group.
func (r *SmartCommitRunner) generateCommitMessage(ctx context.Context, group FileGroup) string {
	// If message is already set (from analysis or user), use it
	if group.SuggestedMessage != "" {
		return group.SuggestedMessage
	}

	// Try AI-powered generation first
	if r.aiRunner != nil {
		message, err := r.generateAIMessageWithRetry(ctx, group)
		if err == nil {
			return message
		}
		// Log warning with fallback message preview
		fallbackMsg := r.generateSimpleMessage(group)
		firstLine := strings.Split(fallbackMsg, "\n")[0]
		r.logger.Warn().
			Err(err).
			Str("fallback_message", firstLine).
			Msg("AI message generation failed, using fallback")
		return fallbackMsg
	}

	// Fallback to simple message generation
	return r.generateSimpleMessage(group)
}

// generateAIMessageWithRetry attempts AI message generation with exponential backoff retry logic.
func (r *SmartCommitRunner) generateAIMessageWithRetry(ctx context.Context, group FileGroup) (string, error) {
	var lastErr error
	currentTimeout := r.timeout

	// Calculate diff size for logging
	diffSummary := r.getDiffSummary(ctx, group)
	diffLines := len(strings.Split(diffSummary, "\n"))

	// Try initial attempt + retries
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		// Log attempt details
		if attempt == 0 {
			r.logger.Info().
				Str("agent", r.agent).
				Str("model", r.model).
				Str("package", group.Package).
				Int("files", len(group.Files)).
				Int("diff_lines", diffLines).
				Dur("timeout", currentTimeout).
				Msg("generating commit message with AI")
		} else {
			r.logger.Info().
				Str("agent", r.agent).
				Str("model", r.model).
				Int("attempt", attempt+1).
				Int("max_attempts", r.maxRetries+1).
				Dur("timeout", currentTimeout).
				Msg("retrying AI commit message generation")
		}

		// Track start time for latency measurement
		startTime := time.Now()

		// Attempt generation
		message, err := r.generateAIMessage(ctx, group, currentTimeout, diffSummary)

		// Calculate latency
		latency := time.Since(startTime)

		if err == nil {
			// Success! Log and return
			r.logger.Info().
				Dur("latency", latency).
				Int("attempt", attempt+1).
				Msg("AI commit message generated successfully")
			return message, nil
		}

		// Log failure with latency
		r.logger.Warn().
			Err(err).
			Dur("latency", latency).
			Dur("timeout", currentTimeout).
			Int("attempt", attempt+1).
			Msg("AI generation attempt failed")

		lastErr = err

		// If this was the last attempt, don't increase timeout
		if attempt < r.maxRetries {
			// Apply exponential backoff for next attempt
			currentTimeout = time.Duration(float64(currentTimeout) * r.retryBackoffFactor)
		}
	}

	// All attempts failed
	return "", fmt.Errorf("AI generation failed after %d attempts: %w", r.maxRetries+1, lastErr)
}

// generateAIMessage uses the AI runner to generate a commit message with subject and synopsis body.
// This is the core generation logic called by generateAIMessageWithRetry.
func (r *SmartCommitRunner) generateAIMessage(ctx context.Context, group FileGroup, timeout time.Duration, diffSummary string) (string, error) {
	// Build the prompt with diff context
	prompt := r.buildAIPromptWithDiff(group, diffSummary)

	// Create AI request with optional agent/model override
	req := &domain.AIRequest{
		Agent:      domain.Agent(r.agent), // Use configured agent (empty uses AIRunner's default)
		Prompt:     prompt,
		Model:      r.model, // Use configured model (empty string uses AIRunner's default)
		MaxTurns:   1,
		Timeout:    timeout, // Use provided timeout
		WorkingDir: r.workDir,
	}

	// Run AI
	result, err := r.aiRunner.Run(ctx, req)
	if err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("%w: %s", atlaserrors.ErrAIError, result.Error)
	}

	// Parse and validate the message
	message := strings.TrimSpace(result.Output)
	if message == "" {
		return "", atlaserrors.ErrAIEmptyResponse
	}

	// Extract first line for validation
	lines := strings.Split(message, "\n")
	firstLine := strings.TrimSpace(lines[0])

	// Validate conventional commits format for subject line
	if !isValidConventionalCommit(firstLine) {
		return "", atlaserrors.ErrAIInvalidFormat
	}

	// Return full message including body (subject + synopsis)
	return message, nil
}

// getDiffSummary retrieves a summary of changes for the given file group.
// Returns empty string if diff cannot be retrieved.
func (r *SmartCommitRunner) getDiffSummary(ctx context.Context, group FileGroup) string {
	diff := r.fetchDiff(ctx)
	if diff == "" {
		return ""
	}

	// Build set of paths in this group for filtering
	pathSet := make(map[string]bool)
	for _, p := range GetFilePaths(group.Files) {
		pathSet[p] = true
	}

	return parseDiffStats(diff, pathSet)
}

// fetchDiff gets the diff output, trying unstaged first then staged.
func (r *SmartCommitRunner) fetchDiff(ctx context.Context) string {
	diff, err := r.runner.DiffUnstaged(ctx)
	if err != nil {
		r.logger.Debug().Err(err).Msg("failed to get diff for AI prompt")
		return ""
	}

	if diff != "" {
		return diff
	}

	diff, err = r.runner.DiffStaged(ctx)
	if err != nil {
		r.logger.Debug().Err(err).Msg("failed to get staged diff for AI prompt")
		return ""
	}
	return diff
}

// maxDiffLinesForContent is the threshold for including actual diff content.
// If total changed lines are below this, we include the actual changes.
const maxDiffLinesForContent = 50

// parseDiffStats extracts line statistics and actual content from a git diff output.
// For small diffs (under maxDiffLinesForContent lines), includes actual changed lines.
func parseDiffStats(diff string, pathSet map[string]bool) string {
	var summary strings.Builder
	var currentFile string
	var additions, deletions int

	// First pass: collect stats
	for _, line := range strings.Split(diff, "\n") {
		currentFile, additions, deletions = processDiffLine(
			line, currentFile, additions, deletions, pathSet, &summary,
		)
	}

	// Flush last file
	writeDiffStats(&summary, currentFile, additions, deletions, pathSet)

	// Second pass: if diff is small, include actual content
	diffContent := extractDiffContent(diff, pathSet)
	if diffContent != "" {
		summary.WriteString("\nActual changes:\n")
		summary.WriteString(diffContent)
	}

	return summary.String()
}

// extractDiffContent extracts the actual changed lines from a diff.
// Returns empty string if diff is too large or no relevant changes.
func extractDiffContent(diff string, pathSet map[string]bool) string {
	var content strings.Builder
	var currentFile string
	var inRelevantFile bool
	var lineCount int

	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		// Track which file we're in
		if strings.HasPrefix(line, "diff --git") {
			if parts := strings.Split(line, " "); len(parts) >= 4 {
				currentFile = strings.TrimPrefix(parts[2], "a/")
				inRelevantFile = pathSet[currentFile]
			}
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "@@ ") {
			continue
		}

		// Collect actual changes for relevant files
		if inRelevantFile {
			if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				lineCount++
				if lineCount > maxDiffLinesForContent {
					// Diff too large, return empty to skip content
					return ""
				}
				content.WriteString(line)
				content.WriteString("\n")
			}
		}
	}

	return content.String()
}

// processDiffLine processes a single line from the diff output.
func processDiffLine(
	line, currentFile string,
	additions, deletions int,
	pathSet map[string]bool,
	summary *strings.Builder,
) (string, int, int) {
	if strings.HasPrefix(line, "diff --git") {
		// Flush previous file stats
		writeDiffStats(summary, currentFile, additions, deletions, pathSet)
		// Parse new file path
		if parts := strings.Split(line, " "); len(parts) >= 4 {
			currentFile = strings.TrimPrefix(parts[2], "a/")
		}
		return currentFile, 0, 0
	}

	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		additions++
	} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		deletions++
	}
	return currentFile, additions, deletions
}

// writeDiffStats writes file statistics to the summary if the file is in pathSet.
func writeDiffStats(summary *strings.Builder, file string, additions, deletions int, pathSet map[string]bool) {
	if file != "" && pathSet[file] {
		fmt.Fprintf(summary, "  %s: +%d/-%d lines\n", file, additions, deletions)
	}
}

// buildAIPromptWithDiff creates the prompt for AI commit message generation with diff context.
func (r *SmartCommitRunner) buildAIPromptWithDiff(group FileGroup, diffSummary string) string {
	var fileList strings.Builder
	for _, f := range group.Files {
		fileList.WriteString(fmt.Sprintf("- %s (%s)\n", f.Path, f.Status))
	}

	scope := GetScopeFromPackage(group.Package)

	diffSection := ""
	if diffSummary != "" {
		diffSection = fmt.Sprintf("\nChange summary:\n%s", diffSummary)
	}

	return fmt.Sprintf(`Generate a conventional commit message for these changes:

Package: %s
Files changed:
%s%s

Requirements:
1. Use conventional commits format: <type>(<scope>): <description>
2. Type must be one of: feat, fix, docs, style, refactor, test, chore, build, ci
3. Scope should be: %s
4. Description should be lowercase, no period at end
5. Keep subject line under 72 characters
6. Include a blank line followed by a 1-2 sentence synopsis (under 150 chars)
7. Synopsis should explain WHAT changed and WHY
8. Do NOT include any AI attribution or mention of Claude/Anthropic
9. Base your message on the actual diff content shown above, not assumptions from filenames
10. Be accurate and specific about what actually changed

Return format (no extra formatting or explanations):
<type>(<scope>): <description>

<synopsis>`,
		group.Package,
		fileList.String(),
		diffSection,
		scope,
	)
}

// addGarbageToGroups adds garbage files back to the analysis groups.
// This is used when IncludeGarbage option is true.
func (r *SmartCommitRunner) addGarbageToGroups(analysis *CommitAnalysis) *CommitAnalysis {
	// Convert garbage files to FileChange and group them
	garbageChanges := make([]FileChange, 0, len(analysis.GarbageFiles))
	for _, g := range analysis.GarbageFiles {
		garbageChanges = append(garbageChanges, FileChange{
			Path:   g.Path,
			Status: ChangeAdded, // Garbage is typically untracked/new
		})
	}

	// Group garbage files
	garbageGroups := GroupFilesByPackage(garbageChanges)

	// Merge with existing groups
	existingGroups := make(map[string]*FileGroup)
	for i := range analysis.FileGroups {
		existingGroups[analysis.FileGroups[i].Package] = &analysis.FileGroups[i]
	}

	for _, gg := range garbageGroups {
		if existing, ok := existingGroups[gg.Package]; ok {
			existing.Files = append(existing.Files, gg.Files...)
		} else {
			analysis.FileGroups = append(analysis.FileGroups, gg)
		}
	}

	// Update total changes
	analysis.TotalChanges += len(garbageChanges)

	return analysis
}

// generateSimpleMessage creates a fallback commit message with subject and synopsis body.
func (r *SmartCommitRunner) generateSimpleMessage(group FileGroup) string {
	scope := GetScopeFromPackage(group.Package)
	commitType := group.CommitType

	var description string
	var synopsis string
	switch {
	case len(group.Files) == 1:
		description = getFileDescription(group.Files[0])
		synopsis = fmt.Sprintf("Updated %s in %s.", filepath.Base(group.Files[0].Path), group.Package)
	default:
		description = fmt.Sprintf("update %d files", len(group.Files))
		synopsis = fmt.Sprintf("Updated %d files in %s.", len(group.Files), group.Package)
	}

	var subject string
	if scope != "" {
		subject = fmt.Sprintf("%s(%s): %s", commitType, scope, description)
	} else {
		subject = fmt.Sprintf("%s: %s", commitType, description)
	}

	// Return subject + synopsis body
	return fmt.Sprintf("%s\n\n%s", subject, synopsis)
}

// getFileDescription creates a description based on file change.
func getFileDescription(f FileChange) string {
	filename := filepath.Base(f.Path)
	switch f.Status {
	case ChangeAdded:
		return fmt.Sprintf("add %s", filename)
	case ChangeDeleted:
		return fmt.Sprintf("remove %s", filename)
	case ChangeRenamed:
		return fmt.Sprintf("rename %s", filename)
	case ChangeModified:
		return fmt.Sprintf("update %s", filename)
	case ChangeCopied:
		return fmt.Sprintf("copy %s", filename)
	case ChangeUnmerged:
		return fmt.Sprintf("resolve %s", filename)
	}
	// Default case for any unknown status
	return fmt.Sprintf("update %s", filename)
}

// isValidConventionalCommit checks if a message follows conventional commits.
func isValidConventionalCommit(message string) bool {
	// Simple validation: must start with a known type
	for _, t := range ValidCommitTypes {
		typeStr := string(t)
		if strings.HasPrefix(message, typeStr+"(") || strings.HasPrefix(message, typeStr+":") {
			return true
		}
	}
	return false
}

// getHeadShortHash returns the short hash of HEAD.
func (r *SmartCommitRunner) getHeadShortHash(ctx context.Context) (string, error) {
	output, err := RunCommand(ctx, r.workDir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// saveArtifact saves the commit information to a markdown file.
func (r *SmartCommitRunner) saveArtifact(commits []CommitInfo) (string, error) {
	if r.artifactsDir == "" {
		return "", nil
	}

	// Ensure directory exists (0750 for security)
	if err := os.MkdirAll(r.artifactsDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create artifacts directory: %w", err)
	}

	artifact := &CommitArtifact{
		TaskID:    r.taskID,
		Template:  r.templateName,
		Commits:   commits,
		Timestamp: time.Now().Format(time.RFC3339),
		Summary:   fmt.Sprintf("Created %d commit(s)", len(commits)),
	}

	content := formatArtifactMarkdown(artifact)
	path := filepath.Join(r.artifactsDir, "commit-message.md")

	// Use 0600 for security
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write artifact: %w", err)
	}

	return path, nil
}

// formatArtifactMarkdown formats the commit artifact as markdown.
func formatArtifactMarkdown(artifact *CommitArtifact) string {
	var sb strings.Builder

	sb.WriteString("# Commit Summary\n\n")
	sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n", artifact.Timestamp))
	if artifact.TaskID != "" {
		sb.WriteString(fmt.Sprintf("**Task:** %s\n", artifact.TaskID))
	}
	if artifact.Template != "" {
		sb.WriteString(fmt.Sprintf("**Template:** %s\n", artifact.Template))
	}
	sb.WriteString("\n## Commits\n\n")

	for i, commit := range artifact.Commits {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, commit.Hash))
		// Show full message (subject + body) in a code block
		sb.WriteString("**Message:**\n```\n")
		sb.WriteString(commit.Message)
		sb.WriteString("\n```\n\n")
		sb.WriteString(fmt.Sprintf("**Package:** %s\n\n", commit.Package))
		sb.WriteString(fmt.Sprintf("**Type:** %s\n\n", commit.CommitType))
		sb.WriteString("**Files:**\n")
		for _, f := range commit.FilesChanged {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildDryRunResult creates a result showing what would be committed.
func (r *SmartCommitRunner) buildDryRunResult(analysis *CommitAnalysis, opts CommitOptions) (*CommitResult, error) {
	var groups []FileGroup
	if opts.SingleCommit {
		totalFiles := 0
		for _, g := range analysis.FileGroups {
			totalFiles += len(g.Files)
		}
		allFiles := make([]FileChange, 0, totalFiles)
		for _, g := range analysis.FileGroups {
			allFiles = append(allFiles, g.Files...)
		}
		groups = GroupFilesForSingleCommit(allFiles)
	} else {
		groups = analysis.FileGroups
	}

	commits := make([]CommitInfo, 0, len(groups))
	totalFiles := 0

	for _, group := range groups {
		message := r.generateSimpleMessage(group)
		commits = append(commits, CommitInfo{
			Hash:         "(dry-run)",
			Message:      message,
			FileCount:    len(group.Files),
			Package:      group.Package,
			CommitType:   group.CommitType,
			FilesChanged: GetFilePaths(group.Files),
		})
		totalFiles += len(group.Files)
	}

	return &CommitResult{
		Commits:      commits,
		ArtifactPath: "",
		TotalFiles:   totalFiles,
	}, nil
}
