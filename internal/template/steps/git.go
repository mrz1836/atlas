// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
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
	GitOpCommit     GitOperation = "commit"
	GitOpPush       GitOperation = "push"
	GitOpCreatePR   GitOperation = "create_pr"
	GitOpMergePR    GitOperation = "merge_pr"
	GitOpAddReview  GitOperation = "add_pr_review"
	GitOpAddComment GitOperation = "add_pr_comment"
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

// GitExecutor handles git operations: commit, push, PR creation, merge, review, comment.
type GitExecutor struct {
	smartCommitter git.SmartCommitService
	pusher         git.PushService
	hubRunner      git.HubRunner
	prDescGen      git.PRDescriptionGenerator
	gitRunner      git.Runner
	workDir        string
	artifactSaver  ArtifactSaver
	artifactHelper *ArtifactHelper
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

// WithGitArtifactSaver sets the artifact saver for git results.
func WithGitArtifactSaver(saver ArtifactSaver) GitExecutorOption {
	return func(e *GitExecutor) {
		e.artifactSaver = saver
	}
}

// WithGitArtifactHelper sets the artifact helper for git results.
// This is the preferred option over WithGitArtifactSaver for new code.
func WithGitArtifactHelper(helper *ArtifactHelper) GitExecutorOption {
	return func(e *GitExecutor) {
		e.artifactHelper = helper
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
	case GitOpMergePR:
		result, err = e.executeMergePR(ctx, step, task)
	case GitOpAddReview:
		result, err = e.executeAddReview(ctx, step, task)
	case GitOpAddComment:
		result, err = e.executeAddComment(ctx, step, task)
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
			Status: constants.StepStatusAwaitingApproval,
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
	// Trailers are deprecated - commit messages now include an AI-generated synopsis body
	commitOpts := git.CommitOptions{}

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
	if jsonPath := e.saveCommitResultJSON(ctx, task, step.Name, result); jsonPath != "" {
		artifactPaths = append(artifactPaths, jsonPath)
	}

	// Collect all changed files from all commits
	var filesChanged []string
	for _, commit := range result.Commits {
		filesChanged = append(filesChanged, commit.FilesChanged...)
	}

	// Store commit messages in metadata for PR description generation
	commitMessages := make([]string, len(result.Commits))
	for i, commit := range result.Commits {
		commitMessages[i] = commit.Message
	}

	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		Output:       fmt.Sprintf("Created %d commit(s), %d files changed", len(result.Commits), result.TotalFiles),
		FilesChanged: filesChanged,
		ArtifactPath: joinArtifactPaths(artifactPaths),
		Metadata: map[string]any{
			"commit_messages": commitMessages,
		},
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
				Status: constants.StepStatusFailed,
				Output: fmt.Sprintf("Push failed (auth): %v", err),
				Error:  fmt.Sprintf("gh_failed: %v", err),
			}, nil
		}
		// Check for non-fast-forward rejection (remote has commits local doesn't)
		if result != nil && result.ErrorType == git.PushErrorNonFastForward {
			return &domain.StepResult{
				Status: constants.StepStatusFailed,
				Output: "Push rejected: remote branch has newer commits. Your local commits are preserved.",
				Error:  "gh_failed: non_fast_forward",
			}, nil
		}
		return nil, fmt.Errorf("failed to push: %w", err)
	}

	// Save push result artifact
	artifactPath := e.savePushResultJSON(ctx, task, step.Name, result)

	output := fmt.Sprintf("Pushed to %s/%s", pushOpts.Remote, branch)
	if result.Upstream != "" {
		output = fmt.Sprintf("Pushed to %s (tracking: %s)", pushOpts.Remote, result.Upstream)
	}

	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
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
	artifactPaths := e.savePRDescriptionArtifact(ctx, task, step.Name, description)

	prResult, err := e.createPR(ctx, description, baseBranch, headBranch)
	if err != nil {
		return e.handlePRCreationError(prResult, err)
	}

	// Save PR result artifact
	artifactPaths = append(artifactPaths, e.savePRResultArtifact(ctx, task, step.Name, prResult)...)

	e.storePRMetadata(task, prResult)

	// Format output with PR number and URL on separate lines for better display
	// The UI layer can detect the URL and make it clickable
	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		Output:       fmt.Sprintf("Created PR #%d\n%s", prResult.Number, prResult.URL),
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

// savePRDescriptionArtifact saves the PR description and returns artifact paths.
func (e *GitExecutor) savePRDescriptionArtifact(ctx context.Context, task *domain.Task, stepName string, description *git.PRDescription) []string {
	if descPath := e.savePRDescriptionMD(ctx, task, stepName, description); descPath != "" {
		return []string{descPath}
	}
	return nil
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
			Status: constants.StepStatusFailed,
			Output: fmt.Sprintf("PR creation failed: %v", err),
			Error:  fmt.Sprintf("gh_failed: %v", err),
		}, nil
	}
	return nil, fmt.Errorf("failed to create PR: %w", err)
}

// savePRResultArtifact saves the PR result and returns artifact paths.
func (e *GitExecutor) savePRResultArtifact(ctx context.Context, task *domain.Task, stepName string, prResult *git.PRResult) []string {
	if resultPath := e.savePRResultJSON(ctx, task, stepName, prResult); resultPath != "" {
		return []string{resultPath}
	}
	return nil
}

// storePRMetadata stores PR information in task metadata for downstream steps.
func (e *GitExecutor) storePRMetadata(task *domain.Task, prResult *git.PRResult) {
	if task.Metadata == nil {
		task.Metadata = make(map[string]any)
	}
	task.Metadata["pr_number"] = prResult.Number
	task.Metadata["pr_url"] = prResult.URL
}

// executeMergePR merges a pull request.
func (e *GitExecutor) executeMergePR(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	log := e.logger.With().Str("operation", "merge_pr").Logger()
	log.Debug().Msg("executing merge PR")

	if e.hubRunner == nil {
		return nil, fmt.Errorf("hub runner not configured: %w", atlaserrors.ErrGitHubOperation)
	}

	// Extract PR number (from config or task metadata)
	prNumber := e.getPRNumber(step.Config, task)
	if prNumber <= 0 {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: "PR number not found in config or task metadata",
			Error:  "missing pr_number",
		}, nil
	}

	// Extract merge method (default: squash)
	mergeMethod := MergeMethodSquash
	if m, ok := step.Config["merge_method"].(string); ok && m != "" {
		if ValidMergeMethod(m) {
			mergeMethod = m
		}
	}

	// Extract admin bypass flag
	adminBypass := false
	if ab, ok := step.Config["admin_bypass"].(bool); ok {
		adminBypass = ab
	}

	// Extract delete branch flag (default: false - keep branch)
	deleteBranch := false
	if db, ok := step.Config["delete_branch"].(bool); ok {
		deleteBranch = db
	}

	log.Debug().
		Int("pr_number", prNumber).
		Str("merge_method", mergeMethod).
		Bool("admin_bypass", adminBypass).
		Bool("delete_branch", deleteBranch).
		Msg("merging PR")

	// Execute merge
	if err := e.hubRunner.MergePR(ctx, prNumber, mergeMethod, adminBypass, deleteBranch); err != nil {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: fmt.Sprintf("Failed to merge PR #%d: %v", prNumber, err),
			Error:  err.Error(),
		}, nil
	}

	// Save artifact using helper
	result := &MergeResult{
		PRNumber:     prNumber,
		MergeMethod:  mergeMethod,
		AdminBypass:  adminBypass,
		DeleteBranch: deleteBranch,
		MergedAt:     time.Now(),
	}
	artifactPath := e.artifactHelper.SaveJSON(ctx, task, step.Name, constants.ArtifactMergeResult, result)

	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		Output:       fmt.Sprintf("Merged PR #%d using %s", prNumber, mergeMethod),
		ArtifactPath: artifactPath,
	}, nil
}

// executeAddReview adds a review to a pull request.
func (e *GitExecutor) executeAddReview(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	log := e.logger.With().Str("operation", "add_pr_review").Logger()
	log.Debug().Msg("executing add PR review")

	if e.hubRunner == nil {
		return nil, fmt.Errorf("hub runner not configured: %w", atlaserrors.ErrGitHubOperation)
	}

	prNumber := e.getPRNumber(step.Config, task)
	if prNumber <= 0 {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: "PR number not found in config or task metadata",
			Error:  "missing pr_number",
		}, nil
	}

	// Event: APPROVE, REQUEST_CHANGES, or COMMENT (default: APPROVE)
	event := ReviewEventApprove
	if ev, ok := step.Config["event"].(string); ok && ev != "" {
		upperEvent := strings.ToUpper(ev)
		if ValidReviewEvent(upperEvent) {
			event = upperEvent
		}
	}

	body, ok := step.Config["body"].(string)
	if !ok && step.Config["body"] != nil {
		log.Warn().Msg("review body config is not a string, ignoring")
	}

	log.Debug().
		Int("pr_number", prNumber).
		Str("event", event).
		Msg("adding review to PR")

	if err := e.hubRunner.AddPRReview(ctx, prNumber, body, event); err != nil {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: fmt.Sprintf("Failed to add review to PR #%d: %v", prNumber, err),
			Error:  err.Error(),
		}, nil
	}

	// Save artifact using helper
	result := &ReviewResult{
		PRNumber: prNumber,
		Event:    event,
		Body:     body,
		AddedAt:  time.Now(),
	}
	artifactPath := e.artifactHelper.SaveJSON(ctx, task, step.Name, constants.ArtifactReviewResult, result)

	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		Output:       fmt.Sprintf("Added %s review to PR #%d", event, prNumber),
		ArtifactPath: artifactPath,
	}, nil
}

// executeAddComment adds a comment to a pull request.
func (e *GitExecutor) executeAddComment(ctx context.Context, step *domain.StepDefinition, task *domain.Task) (*domain.StepResult, error) {
	log := e.logger.With().Str("operation", "add_pr_comment").Logger()
	log.Debug().Msg("executing add PR comment")

	if e.hubRunner == nil {
		return nil, fmt.Errorf("hub runner not configured: %w", atlaserrors.ErrGitHubOperation)
	}

	prNumber := e.getPRNumber(step.Config, task)
	if prNumber <= 0 {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: "PR number not found in config or task metadata",
			Error:  "missing pr_number",
		}, nil
	}

	body, ok := step.Config["body"].(string)
	if !ok || body == "" {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: "Comment body is required",
			Error:  "missing body",
		}, nil
	}

	log.Debug().
		Int("pr_number", prNumber).
		Msg("adding comment to PR")

	if err := e.hubRunner.AddPRComment(ctx, prNumber, body); err != nil {
		return &domain.StepResult{
			Status: constants.StepStatusFailed,
			Output: fmt.Sprintf("Failed to add comment to PR #%d: %v", prNumber, err),
			Error:  err.Error(),
		}, nil
	}

	// Save artifact using helper
	result := &CommentResult{
		PRNumber: prNumber,
		Body:     body,
		AddedAt:  time.Now(),
	}
	artifactPath := e.artifactHelper.SaveJSON(ctx, task, step.Name, constants.ArtifactCommentResult, result)

	return &domain.StepResult{
		Status:       constants.StepStatusSuccess,
		Output:       fmt.Sprintf("Added comment to PR #%d", prNumber),
		ArtifactPath: artifactPath,
	}, nil
}

// getPRNumber extracts PR number from config or task metadata.
func (e *GitExecutor) getPRNumber(config map[string]any, task *domain.Task) int {
	// 1. Try step config
	if num, ok := getIntFromAny(config["pr_number"]); ok {
		return num
	}

	// 2. Try task metadata (set by CreatePR step)
	if task.Metadata != nil {
		if num, ok := getIntFromAny(task.Metadata["pr_number"]); ok {
			return num
		}
	}

	return 0
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

// saveArtifact saves data as an artifact and returns the filename.
// Returns empty string if artifactSaver is nil or on error.
func (e *GitExecutor) saveArtifact(ctx context.Context, task *domain.Task, stepName, filename string, data []byte) string {
	if e.artifactSaver == nil {
		return ""
	}
	fullPath := filepath.Join(stepName, filename)
	if err := e.artifactSaver.SaveArtifact(ctx, task.WorkspaceID, task.ID, fullPath, data); err != nil {
		e.logger.Warn().Err(err).Str("filename", filename).Msg("failed to save artifact")
		return ""
	}
	return fullPath
}

// saveJSONArtifact marshals data to JSON and saves it as an artifact.
func (e *GitExecutor) saveJSONArtifact(ctx context.Context, task *domain.Task, stepName, filename string, data any) string {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		e.logger.Warn().Err(err).Str("filename", filename).Msg("failed to marshal artifact data")
		return ""
	}
	return e.saveArtifact(ctx, task, stepName, filename, jsonData)
}

// saveCommitResultJSON saves commit result as a JSON artifact.
func (e *GitExecutor) saveCommitResultJSON(ctx context.Context, task *domain.Task, stepName string, result *git.CommitResult) string {
	return e.saveJSONArtifact(ctx, task, stepName, constants.ArtifactCommitResult, result)
}

// savePushResultJSON saves push result as a JSON artifact.
func (e *GitExecutor) savePushResultJSON(ctx context.Context, task *domain.Task, stepName string, result *git.PushResult) string {
	return e.saveJSONArtifact(ctx, task, stepName, constants.ArtifactPushResult, result)
}

// savePRDescriptionMD saves PR description as a markdown artifact.
func (e *GitExecutor) savePRDescriptionMD(ctx context.Context, task *domain.Task, stepName string, desc *git.PRDescription) string {
	content := fmt.Sprintf("# %s\n\n%s", desc.Title, desc.Body)
	return e.saveArtifact(ctx, task, stepName, constants.ArtifactPRDescription, []byte(content))
}

// savePRResultJSON saves PR result as a JSON artifact.
func (e *GitExecutor) savePRResultJSON(ctx context.Context, task *domain.Task, stepName string, result *git.PRResult) string {
	return e.saveJSONArtifact(ctx, task, stepName, constants.ArtifactPRResult, result)
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
		if r.Status != "success" {
			continue
		}
		msgs := extractMessagesFromResult(r)
		messages = append(messages, msgs...)
	}
	return messages
}

// extractMessagesFromResult extracts commit messages from a single step result.
func extractMessagesFromResult(r domain.StepResult) []string {
	// Try to extract from metadata first (preferred method)
	if msgs := extractFromMetadata(r.Metadata); len(msgs) > 0 {
		return msgs
	}

	// Fallback: use Output for backward compatibility
	if r.Output != "" && looksLikeCommitMessage(r.Output) {
		return []string{r.Output}
	}

	return nil
}

// extractFromMetadata extracts commit messages from metadata.
func extractFromMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}

	// Try []string type first
	if commitMsgs, ok := metadata["commit_messages"].([]string); ok {
		return commitMsgs
	}

	// Handle []any type
	if commitMsgsAny, ok := metadata["commit_messages"].([]any); ok {
		var msgs []string
		for _, msg := range commitMsgsAny {
			if msgStr, ok := msg.(string); ok {
				msgs = append(msgs, msgStr)
			}
		}
		return msgs
	}

	return nil
}

// looksLikeCommitMessage checks if a string looks like a commit message.
// Returns true if the string starts with a conventional commit type and has a description.
func looksLikeCommitMessage(s string) bool {
	conventionalTypes := []string{"feat:", "fix:", "docs:", "style:", "refactor:", "test:", "chore:", "build:", "ci:", "perf:", "revert:"}
	for _, prefix := range conventionalTypes {
		// Check for simple format: type: description
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			// Make sure there's actual content after the colon (not just whitespace)
			remaining := strings.TrimSpace(s[len(prefix):])
			if len(remaining) > 0 {
				return true
			}
		}
		// Also check for scoped format: type(scope): description
		if len(s) > len(prefix)+2 && s[:len(prefix)-1] == prefix[:len(prefix)-1] && s[len(prefix)-1] == '(' {
			// Find the closing paren and colon
			closeParenIdx := strings.Index(s[len(prefix)-1:], "):")
			if closeParenIdx > 0 {
				afterColon := s[len(prefix)-1+closeParenIdx+2:]
				if len(strings.TrimSpace(afterColon)) > 0 {
					return true
				}
			}
		}
	}
	return false
}

// extractFilesChanged extracts files changed from previous step results.
func extractFilesChanged(results []domain.StepResult) []string {
	totalFiles := 0
	for _, r := range results {
		totalFiles += len(r.FilesChanged)
	}
	files := make([]string, 0, totalFiles)
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
