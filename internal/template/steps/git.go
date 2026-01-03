// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// GitOperation defines the supported git operations.
type GitOperation string

// Git operation constants.
const (
	GitOpCommit   GitOperation = "commit"
	GitOpPush     GitOperation = "push"
	GitOpCreatePR GitOperation = "create_pr"
)

// GarbageHandlingAction defines how to handle detected garbage files.
type GarbageHandlingAction int

// Garbage handling action constants.
const (
	// GarbageRemoveAndContinue removes garbage files and continues with commit.
	GarbageRemoveAndContinue GarbageHandlingAction = iota
	// GarbageIncludeAnyway includes garbage files in the commit (requires confirmation).
	GarbageIncludeAnyway
	// GarbageAbortManual aborts the commit for manual intervention.
	GarbageAbortManual
)

// GitExecutor handles git operations: commit, push, PR creation.
type GitExecutor struct {
	smartCommitter git.SmartCommitService
	pusher         git.PushService
	hubRunner      git.HubRunner
	prDescGen      git.PRDescriptionGenerator
	gitRunner      git.Runner
	workDir        string
	artifactsDir   string
	baseBranch     string
	logger         zerolog.Logger
}

// GitExecutorOption configures GitExecutor.
type GitExecutorOption func(*GitExecutor)

// NewGitExecutor creates a GitExecutor with dependencies.
func NewGitExecutor(workDir string, opts ...GitExecutorOption) *GitExecutor {
	e := &GitExecutor{
		workDir: workDir,
		logger:  zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithSmartCommitter sets the smart committer.
func WithSmartCommitter(committer git.SmartCommitService) GitExecutorOption {
	return func(e *GitExecutor) {
		e.smartCommitter = committer
	}
}

// WithPusher sets the pusher.
func WithPusher(pusher git.PushService) GitExecutorOption {
	return func(e *GitExecutor) {
		e.pusher = pusher
	}
}

// WithHubRunner sets the GitHub runner.
func WithHubRunner(runner git.HubRunner) GitExecutorOption {
	return func(e *GitExecutor) {
		e.hubRunner = runner
	}
}

// WithPRDescriptionGenerator sets the PR description generator.
func WithPRDescriptionGenerator(gen git.PRDescriptionGenerator) GitExecutorOption {
	return func(e *GitExecutor) {
		e.prDescGen = gen
	}
}

// WithGitRunner sets the git runner.
func WithGitRunner(runner git.Runner) GitExecutorOption {
	return func(e *GitExecutor) {
		e.gitRunner = runner
	}
}

// WithGitLogger sets the logger for git operations.
func WithGitLogger(logger zerolog.Logger) GitExecutorOption {
	return func(e *GitExecutor) {
		e.logger = logger
	}
}

// WithArtifactsDir sets the directory for saving artifacts.
func WithArtifactsDir(dir string) GitExecutorOption {
	return func(e *GitExecutor) {
		e.artifactsDir = dir
	}
}

// WithBaseBranch sets the default base branch for PR creation.
func WithBaseBranch(branch string) GitExecutorOption {
	return func(e *GitExecutor) {
		e.baseBranch = branch
	}
}

// Execute runs a git operation.
// The operation type is read from step.Config["operation"].
// Supported operations: commit, push, create_pr
func (e *GitExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	startTime := time.Now()

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing git step")

	// Get operation from step config
	operation, ok := step.Config["operation"].(string)
	if !ok {
		operation = string(GitOpCommit) // Default to commit
	}

	e.logger.Debug().
		Str("operation", operation).
		Str("work_dir", e.workDir).
		Msg("git operation")

	var result *domain.StepResult
	var err error

	switch GitOperation(operation) {
	case GitOpCommit:
		result, err = e.executeCommit(ctx, step, task)
	case GitOpPush:
		result, err = e.executePush(ctx, step, task)
	case GitOpCreatePR:
		result, err = e.executeCreatePR(ctx, step, task)
	default:
		return nil, fmt.Errorf("unknown git operation %q: %w", operation, atlaserrors.ErrGitOperation)
	}

	if err != nil {
		return nil, err
	}

	// Fill in common result fields
	elapsed := time.Since(startTime)
	result.StepIndex = task.CurrentStep
	result.StepName = step.Name
	result.StartedAt = startTime
	result.CompletedAt = time.Now()
	result.DurationMs = elapsed.Milliseconds()

	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("operation", operation).
		Dur("duration_ms", elapsed).
		Str("status", result.Status).
		Msg("git step completed")

	return result, nil
}

// Type returns the step type this executor handles.
func (e *GitExecutor) Type() domain.StepType {
	return domain.StepTypeGit
}

// HandleGarbageDetected processes garbage files according to the specified action.
func (e *GitExecutor) HandleGarbageDetected(_ context.Context, garbageFiles []git.GarbageFile, action GarbageHandlingAction) error {
	if e.gitRunner == nil {
		return fmt.Errorf("git runner not configured: %w", atlaserrors.ErrGitOperation)
	}

	switch action {
	case GarbageRemoveAndContinue:
		// Unstage garbage files using git rm --cached
		for _, gf := range garbageFiles {
			// We need to use the runner to execute git rm --cached
			// For now, log the action - full implementation requires git.Runner.Remove method
			e.logger.Info().Str("file", gf.Path).Msg("would remove garbage file from staging")
		}
		return nil

	case GarbageIncludeAnyway:
		// Log warning and proceed - caller must have set confirmation flag
		e.logger.Warn().
			Int("garbage_count", len(garbageFiles)).
			Msg("including garbage files in commit as requested")
		return nil

	case GarbageAbortManual:
		return fmt.Errorf("commit aborted for manual intervention: %w", atlaserrors.ErrOperationCanceled)

	default:
		return fmt.Errorf("unknown garbage handling action %d: %w", action, atlaserrors.ErrGitOperation)
	}
}

// executeCommit handles the commit operation.
func (e *GitExecutor) executeCommit(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	if e.smartCommitter == nil {
		return nil, fmt.Errorf("smart committer not configured: %w", atlaserrors.ErrGitOperation)
	}

	// Step 1: Analyze worktree for garbage detection
	analysis, err := e.smartCommitter.Analyze(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze worktree: %w", err)
	}

	// If garbage files are detected, return awaiting_approval status.
	// The garbage file info is stored in the step output.
	if analysis.HasGarbage {
		return &domain.StepResult{
			Status: "awaiting_approval",
			Output: formatGarbageWarning(analysis.GarbageFiles),
		}, nil
	}

	// No changes to commit - return no_changes status so engine can skip push/PR steps
	if len(analysis.FileGroups) == 0 {
		return &domain.StepResult{
			Status: constants.StepStatusNoChanges,
			Output: "No changes to commit - AI made no modifications",
		}, nil
	}

	// Step 2: Execute smart commit with file grouping
	commitOpts := git.CommitOptions{
		Trailers: map[string]string{
			"ATLAS-Task":     task.ID,
			"ATLAS-Template": task.TemplateID,
		},
	}

	result, err := e.smartCommitter.Commit(ctx, commitOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	// Step 3: Save commit artifacts
	artifactPaths := []string{}
	if result.ArtifactPath != "" {
		artifactPaths = append(artifactPaths, result.ArtifactPath)
	}

	// Also save detailed commit result as JSON
	artifactDir := e.getArtifactDir(step.Name, task)
	if artifactDir != "" {
		jsonPath, jsonErr := e.saveCommitResultJSON(artifactDir, result)
		if jsonErr == nil {
			artifactPaths = append(artifactPaths, jsonPath)
		} else {
			e.logger.Warn().Err(jsonErr).Msg("failed to save commit result JSON")
		}
	}

	// Collect all changed files from all commits
	var filesChanged []string
	for _, commit := range result.Commits {
		filesChanged = append(filesChanged, commit.FilesChanged...)
	}

	return &domain.StepResult{
		Status:       "success",
		Output:       fmt.Sprintf("Created %d commit(s), %d files changed", len(result.Commits), result.TotalFiles),
		FilesChanged: filesChanged,
		ArtifactPath: joinArtifactPaths(artifactPaths),
	}, nil
}

// executePush handles the push operation.
func (e *GitExecutor) executePush(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	if e.pusher == nil {
		return nil, fmt.Errorf("pusher not configured: %w", atlaserrors.ErrGitOperation)
	}

	// Get branch from step config or task metadata
	branch := getBranchFromConfig(step.Config, task)
	if branch == "" {
		return nil, fmt.Errorf("branch name not configured: %w", atlaserrors.ErrEmptyValue)
	}

	// Get remote from step config, default to "origin"
	remote := "origin"
	if r, ok := step.Config["remote"].(string); ok && r != "" {
		remote = r
	}

	pushOpts := git.PushOptions{
		Remote:      remote,
		Branch:      branch,
		SetUpstream: true,
	}

	result, err := e.pusher.Push(ctx, pushOpts)
	if err != nil {
		// Check for permanent auth failure
		if result != nil && result.ErrorType == git.PushErrorAuth {
			return &domain.StepResult{
				Status: "failed",
				Output: fmt.Sprintf("Push failed (auth): %v", err),
				Error:  fmt.Sprintf("gh_failed: %v", err),
			}, nil
		}
		// Check for non-fast-forward rejection (remote has commits local doesn't)
		if result != nil && result.ErrorType == git.PushErrorNonFastForward {
			return &domain.StepResult{
				Status: "failed",
				Output: "Push rejected: remote branch has newer commits. Your local commits are preserved.",
				Error:  "gh_failed: non_fast_forward",
			}, nil
		}
		return nil, fmt.Errorf("failed to push: %w", err)
	}

	// Save push result artifact
	artifactDir := e.getArtifactDir(step.Name, task)
	artifactPath := ""
	if artifactDir != "" {
		path, saveErr := e.savePushResultJSON(artifactDir, result)
		if saveErr != nil {
			e.logger.Warn().Err(saveErr).Msg("failed to save push result artifact")
		} else {
			artifactPath = path
		}
	}

	output := fmt.Sprintf("Pushed to %s/%s", pushOpts.Remote, branch)
	if result.Upstream != "" {
		output = fmt.Sprintf("Pushed to %s (tracking: %s)", pushOpts.Remote, result.Upstream)
	}

	return &domain.StepResult{
		Status:       "success",
		Output:       output,
		ArtifactPath: artifactPath,
	}, nil
}

// executeCreatePR handles the PR creation operation.
func (e *GitExecutor) executeCreatePR(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	if err := e.validatePRDependencies(); err != nil {
		return nil, err
	}

	headBranch, baseBranch, err := e.getBranchesForPR(step, task)
	if err != nil {
		return nil, err
	}

	// Pre-check: verify there are commits to create a PR for
	if result := e.checkForCommits(ctx, baseBranch); result != nil {
		return result, nil
	}

	// Generate PR description
	description, err := e.generatePRDescription(ctx, task, baseBranch, headBranch)
	if err != nil {
		return nil, err
	}

	// Save PR description and create PR
	artifactDir := e.getArtifactDir(step.Name, task)
	artifactPaths := e.savePRDescriptionArtifact(artifactDir, description)

	prResult, err := e.createPR(ctx, description, baseBranch, headBranch)
	if err != nil {
		return e.handlePRCreationError(prResult, err)
	}

	// Save PR result artifact
	artifactPaths = append(artifactPaths, e.savePRResultArtifact(artifactDir, prResult)...)

	e.storePRMetadata(task, prResult)

	return &domain.StepResult{
		Status:       "success",
		Output:       fmt.Sprintf("Created PR #%d: %s", prResult.Number, prResult.URL),
		ArtifactPath: joinArtifactPaths(artifactPaths),
	}, nil
}

// validatePRDependencies checks that required dependencies are configured.
func (e *GitExecutor) validatePRDependencies() error {
	if e.hubRunner == nil {
		return fmt.Errorf("hub runner not configured: %w", atlaserrors.ErrGitHubOperation)
	}
	if e.prDescGen == nil {
		return fmt.Errorf("PR description generator not configured: %w", atlaserrors.ErrGitHubOperation)
	}
	return nil
}

// getBranchesForPR extracts the head and base branch names from configuration.
func (e *GitExecutor) getBranchesForPR(step *domain.StepDefinition, task *domain.Task) (string, string, error) {
	headBranch := getBranchFromConfig(step.Config, task)
	if headBranch == "" {
		return "", "", fmt.Errorf("head branch not configured: %w", atlaserrors.ErrEmptyValue)
	}

	baseBranch := e.baseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}
	if b, ok := step.Config["base_branch"].(string); ok && b != "" {
		baseBranch = b
	}

	return headBranch, baseBranch, nil
}

// checkForCommits verifies there are commits between branches.
// Returns a StepResult if there are no commits (indicating no PR needed), nil otherwise.
func (e *GitExecutor) checkForCommits(ctx context.Context, baseBranch string) *domain.StepResult {
	hasCommits, err := e.hasCommitsBetweenBranches(ctx, baseBranch)
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to check for commits between branches, proceeding anyway")
		return nil
	}
	if !hasCommits {
		return &domain.StepResult{
			Status: constants.StepStatusNoChanges,
			Output: fmt.Sprintf("No commits between %s and HEAD - nothing to create PR for", baseBranch),
		}
	}
	return nil
}

// generatePRDescription creates the PR description using the configured generator.
func (e *GitExecutor) generatePRDescription(ctx context.Context, task *domain.Task, baseBranch, headBranch string) (*git.PRDescription, error) {
	descOpts := git.PRDescOptions{
		TaskDescription: task.Description,
		TemplateName:    task.TemplateID,
		TaskID:          task.ID,
		BaseBranch:      baseBranch,
		HeadBranch:      headBranch,
		CommitMessages:  extractCommitMessages(task.StepResults),
		FilesChanged:    convertToFileChanges(extractFilesChanged(task.StepResults)),
	}

	description, err := e.prDescGen.Generate(ctx, descOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PR description: %w", err)
	}
	return description, nil
}

// savePRDescriptionArtifact saves the PR description to disk and returns artifact paths.
func (e *GitExecutor) savePRDescriptionArtifact(artifactDir string, description *git.PRDescription) []string {
	if artifactDir == "" {
		return nil
	}

	descPath, err := e.savePRDescriptionMD(artifactDir, description)
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to save PR description")
		return nil
	}
	return []string{descPath}
}

// createPR creates the pull request via the hub runner.
func (e *GitExecutor) createPR(ctx context.Context, description *git.PRDescription, baseBranch, headBranch string) (*git.PRResult, error) {
	prOpts := git.PRCreateOptions{
		Title:      description.Title,
		Body:       description.Body,
		BaseBranch: baseBranch,
		HeadBranch: headBranch,
	}
	return e.hubRunner.CreatePR(ctx, prOpts)
}

// handlePRCreationError handles errors from PR creation, converting known errors to step results.
func (e *GitExecutor) handlePRCreationError(prResult *git.PRResult, err error) (*domain.StepResult, error) {
	// Check for rate limit or auth errors
	if prResult != nil && (prResult.ErrorType == git.PRErrorRateLimit || prResult.ErrorType == git.PRErrorAuth) {
		return &domain.StepResult{
			Status: "failed",
			Output: fmt.Sprintf("PR creation failed: %v", err),
			Error:  fmt.Sprintf("gh_failed: %v", err),
		}, nil
	}
	return nil, fmt.Errorf("failed to create PR: %w", err)
}

// savePRResultArtifact saves the PR result to disk and returns artifact paths.
func (e *GitExecutor) savePRResultArtifact(artifactDir string, prResult *git.PRResult) []string {
	if artifactDir == "" {
		return nil
	}

	resultPath, err := e.savePRResultJSON(artifactDir, prResult)
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to save PR result")
		return nil
	}
	return []string{resultPath}
}

// storePRMetadata stores PR information in task metadata for downstream steps.
func (e *GitExecutor) storePRMetadata(task *domain.Task, prResult *git.PRResult) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}
	task.Metadata["pr_number"] = prResult.Number
	task.Metadata["pr_url"] = prResult.URL
}

// Helper functions

// formatGarbageWarning creates a human-readable warning about detected garbage files.
func formatGarbageWarning(files []git.GarbageFile) string {
	if len(files) == 0 {
		return ""
	}

	msg := fmt.Sprintf("⚠️ Detected %d garbage file(s) that shouldn't be committed:\n\n", len(files))
	for _, f := range files {
		msg += fmt.Sprintf("  • %s (%s): %s\n", f.Path, f.Category, f.Reason)
	}
	msg += "\nOptions:\n"
	msg += "  [r] Remove and continue (recommended)\n"
	msg += "  [i] Include anyway\n"
	msg += "  [a] Abort and fix manually\n"
	return msg
}

// getArtifactDir returns the artifact directory for a step, creating it if needed.
func (e *GitExecutor) getArtifactDir(stepName string, task *domain.Task) string {
	// Use the executor's artifacts dir if set
	if e.artifactsDir != "" {
		dir := filepath.Join(e.artifactsDir, stepName)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			e.logger.Warn().Err(err).Str("dir", dir).Msg("failed to create artifact directory")
			return ""
		}
		return dir
	}

	// Fall back to task metadata if available
	if task.Metadata != nil {
		if artifactDir, ok := task.Metadata["artifact_dir"].(string); ok && artifactDir != "" {
			dir := filepath.Join(artifactDir, stepName)
			if err := os.MkdirAll(dir, 0o750); err != nil {
				e.logger.Warn().Err(err).Str("dir", dir).Msg("failed to create artifact directory")
				return ""
			}
			return dir
		}
	}

	return ""
}

// saveCommitResultJSON saves commit result as a JSON file.
func (e *GitExecutor) saveCommitResultJSON(dir string, result *git.CommitResult) (string, error) {
	path := filepath.Join(dir, "commit-result.json")
	data, err := json.MarshalIndent(result, "", "  ") //nolint:musttag // external type from git package
	if err != nil {
		return "", fmt.Errorf("failed to marshal commit result: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write commit result: %w", err)
	}
	return path, nil
}

// savePushResultJSON saves push result as a JSON file.
func (e *GitExecutor) savePushResultJSON(dir string, result *git.PushResult) (string, error) {
	path := filepath.Join(dir, "push-result.json")
	data, err := json.MarshalIndent(result, "", "  ") //nolint:musttag // external type from git package
	if err != nil {
		return "", fmt.Errorf("failed to marshal push result: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write push result: %w", err)
	}
	return path, nil
}

// savePRDescriptionMD saves PR description as a markdown file.
func (e *GitExecutor) savePRDescriptionMD(dir string, desc *git.PRDescription) (string, error) {
	path := filepath.Join(dir, "pr-description.md")
	content := fmt.Sprintf("# %s\n\n%s", desc.Title, desc.Body)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write PR description: %w", err)
	}
	return path, nil
}

// savePRResultJSON saves PR result as a JSON file.
func (e *GitExecutor) savePRResultJSON(dir string, result *git.PRResult) (string, error) {
	path := filepath.Join(dir, "pr-result.json")
	data, err := json.MarshalIndent(result, "", "  ") //nolint:musttag // external type from git package
	if err != nil {
		return "", fmt.Errorf("failed to marshal PR result: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write PR result: %w", err)
	}
	return path, nil
}

// getBranchFromConfig extracts the branch name from step config or task.
func getBranchFromConfig(config map[string]any, task *domain.Task) string {
	// First check step config
	if branch, ok := config["branch"].(string); ok && branch != "" {
		return branch
	}

	// Then check task metadata
	if task.Metadata != nil {
		if branch, ok := task.Metadata["branch"].(string); ok && branch != "" {
			return branch
		}
	}

	// Try to get from task config variables
	if task.Config.Variables != nil {
		if branch, ok := task.Config.Variables["branch"]; ok && branch != "" {
			return branch
		}
	}

	return ""
}

// extractCommitMessages extracts commit messages from previous step results.
func extractCommitMessages(results []domain.StepResult) []string {
	var messages []string
	for _, r := range results {
		if r.Output != "" && r.Status == "success" {
			// Check if this was a commit step by looking for commit-related output
			// This is a simplified heuristic
			messages = append(messages, r.Output)
		}
	}
	return messages
}

// extractFilesChanged extracts files changed from previous step results.
func extractFilesChanged(results []domain.StepResult) []string {
	var files []string
	for _, r := range results {
		files = append(files, r.FilesChanged...)
	}
	return files
}

// convertToFileChanges converts file paths to PRFileChange.
func convertToFileChanges(paths []string) []git.PRFileChange {
	changes := make([]git.PRFileChange, len(paths))
	for i, path := range paths {
		changes[i] = git.PRFileChange{
			Path: path,
			// Insertions and Deletions are unknown at this point
		}
	}
	return changes
}

// joinArtifactPaths joins multiple artifact paths with semicolons.
func joinArtifactPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return paths[0]
	}
	result := paths[0]
	for i := 1; i < len(paths); i++ {
		result += ";" + paths[i]
	}
	return result
}

// hasCommitsBetweenBranches checks if there are any commits between the base branch and HEAD.
// Returns true if there are commits to create a PR for, false otherwise.
func (e *GitExecutor) hasCommitsBetweenBranches(ctx context.Context, baseBranch string) (bool, error) {
	// Use git rev-list to count commits between base branch and HEAD
	output, err := git.RunCommand(ctx, e.workDir, "rev-list", "--count", baseBranch+"..HEAD")
	if err != nil {
		return false, fmt.Errorf("failed to check commits: %w", err)
	}

	// Parse the count - if it's 0 or empty, there are no commits
	count := strings.TrimSpace(output)
	return count != "" && count != "0", nil
}
