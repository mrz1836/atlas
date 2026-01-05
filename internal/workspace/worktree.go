// Package workspace provides workspace persistence and management for ATLAS.
// This file implements Git worktree operations for isolated working directories.
package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// WorktreeRunner defines operations for Git worktree management.
type WorktreeRunner interface {
	// Create creates a new worktree with the given options.
	// The worktree is created as a sibling to the repository.
	Create(ctx context.Context, opts WorktreeCreateOptions) (*WorktreeInfo, error)

	// List returns all worktrees in the repository.
	List(ctx context.Context) ([]*WorktreeInfo, error)

	// Remove removes a worktree. If force is true, removes even if dirty.
	Remove(ctx context.Context, path string, force bool) error

	// Prune removes stale worktree entries.
	Prune(ctx context.Context) error

	// BranchExists checks if a branch exists in the repository.
	BranchExists(ctx context.Context, name string) (bool, error)

	// DeleteBranch deletes a branch. If force is true, deletes even if not merged.
	DeleteBranch(ctx context.Context, name string, force bool) error

	// Fetch fetches from the specified remote.
	// If remote is empty, defaults to "origin".
	Fetch(ctx context.Context, remote string) error

	// RemoteBranchExists checks if a branch exists on the specified remote.
	// Returns true if refs/remotes/{remote}/{name} exists.
	RemoteBranchExists(ctx context.Context, remote, name string) (bool, error)
}

// WorktreeCreateOptions contains options for creating a worktree.
type WorktreeCreateOptions struct {
	RepoPath      string // Path to the main repository
	WorkspaceName string // Name of the workspace (used for path and branch)
	BranchType    string // Branch type prefix (feat, fix, chore)
	BaseBranch    string // Branch to create from (default: current branch)
}

// WorktreeInfo contains information about a worktree.
type WorktreeInfo struct {
	Path       string    // Absolute path to the worktree
	Branch     string    // Branch name (e.g., "feat/auth")
	HeadCommit string    // HEAD commit SHA
	IsPrunable bool      // True if worktree directory is missing
	IsLocked   bool      // True if worktree has a lock file
	CreatedAt  time.Time // When the worktree was created (if known)
}

// GitWorktreeRunner implements WorktreeRunner using git CLI.
type GitWorktreeRunner struct {
	repoPath string // Path to the main repository
}

// NewGitWorktreeRunner creates a new GitWorktreeRunner.
func NewGitWorktreeRunner(ctx context.Context, repoPath string) (*GitWorktreeRunner, error) {
	// Detect repo root to ensure we're in a git repo
	root, err := detectRepoRoot(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect git repository: %w", err)
	}
	return &GitWorktreeRunner{repoPath: root}, nil
}

// maxWorkspaceNameLength is the maximum allowed length for workspace names.
const maxWorkspaceNameLength = 255

// Create creates a new worktree with the given options.
func (r *GitWorktreeRunner) Create(ctx context.Context, opts WorktreeCreateOptions) (*WorktreeInfo, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate workspace name
	if opts.WorkspaceName == "" {
		return nil, fmt.Errorf("workspace name cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if len(opts.WorkspaceName) > maxWorkspaceNameLength {
		return nil, fmt.Errorf("workspace name exceeds maximum length of %d characters: %w",
			maxWorkspaceNameLength, atlaserrors.ErrEmptyValue)
	}

	// Validate branch type
	if opts.BranchType == "" {
		return nil, fmt.Errorf("branch type cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Calculate sibling path
	wtPath := siblingPath(r.repoPath, opts.WorkspaceName)

	// Clean up orphaned directory if it exists but isn't a registered worktree
	if err := r.cleanupOrphanedPath(ctx, wtPath); err != nil {
		// Log but continue - worst case ensureUniquePath will add a suffix
		log.Debug().Err(err).Str("path", wtPath).Msg("failed to cleanup orphaned path")
	}

	wtPath, err := ensureUniquePath(wtPath)
	if err != nil {
		return nil, err
	}

	// Generate unique branch name
	baseBranch := generateBranchName(opts.BranchType, opts.WorkspaceName)
	branchName, err := r.generateUniqueBranchName(ctx, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to generate branch name: %w", err)
	}

	// Build worktree add command
	args := []string{"worktree", "add", wtPath, "-b", branchName}
	if opts.BaseBranch != "" {
		args = append(args, opts.BaseBranch)
	}

	_, err = git.RunCommand(ctx, r.repoPath, args...)
	if err != nil {
		// CRITICAL: Clean up on failure (atomic creation)
		_ = os.RemoveAll(wtPath)
		log.Error().
			Err(err).
			Str("branch_name", branchName).
			Str("workspace_name", opts.WorkspaceName).
			Str("base_branch", opts.BaseBranch).
			Msg("failed to create worktree")
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Log successful branch creation
	log.Info().
		Str("branch_name", branchName).
		Str("base_branch", opts.BaseBranch).
		Str("workspace_name", opts.WorkspaceName).
		Str("worktree_path", wtPath).
		Msg("branch created")

	return &WorktreeInfo{
		Path:      wtPath,
		Branch:    branchName,
		CreatedAt: time.Now(),
	}, nil
}

// List returns all worktrees in the repository.
func (r *GitWorktreeRunner) List(ctx context.Context) ([]*WorktreeInfo, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	output, err := git.RunCommand(ctx, r.repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(output), nil
}

// Remove removes a worktree.
func (r *GitWorktreeRunner) Remove(ctx context.Context, path string, force bool) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Validate path is a worktree (not main repo)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if this is the main repo
	if absPath == r.repoPath {
		return fmt.Errorf("'%s' is the main repository, not a worktree: %w. "+
			"Use 'git worktree list' to see valid worktrees",
			path, atlaserrors.ErrNotAWorktree)
	}

	// Build remove command
	args := []string{"worktree", "remove", absPath}
	if force {
		args = append(args, "--force")
	}

	_, err = git.RunCommand(ctx, r.repoPath, args...)
	if err != nil {
		// Check for dirty worktree error
		errStr := err.Error()
		if strings.Contains(errStr, "contains modified or untracked files") ||
			strings.Contains(errStr, "is dirty") {
			return fmt.Errorf("worktree at '%s' has uncommitted changes: %w. "+
				"Commit or stash changes, or use force=true to remove anyway",
				path, atlaserrors.ErrWorktreeDirty)
		}
		// Check for not a worktree error (includes main working tree)
		if strings.Contains(errStr, "is not a working tree") ||
			strings.Contains(errStr, "is a main working tree") {
			return fmt.Errorf("'%s' is not a git worktree: %w. "+
				"Use 'git worktree list' to see valid worktrees",
				path, atlaserrors.ErrNotAWorktree)
		}
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// Prune removes stale worktree entries.
func (r *GitWorktreeRunner) Prune(ctx context.Context) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	_, err := git.RunCommand(ctx, r.repoPath, "worktree", "prune")
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}

// BranchExists checks if a branch exists in the repository.
func (r *GitWorktreeRunner) BranchExists(ctx context.Context, name string) (bool, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	_, err := git.RunCommand(ctx, r.repoPath, "show-ref", "--verify", "refs/heads/"+name)
	if err != nil {
		// Exit code 1 or "not a valid ref" means ref not found, which is expected
		errStr := err.Error()
		if strings.Contains(errStr, "exit status 1") || strings.Contains(errStr, "not a valid ref") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check branch existence: %w", err)
	}

	return true, nil
}

// DeleteBranch deletes a branch.
func (r *GitWorktreeRunner) DeleteBranch(ctx context.Context, name string, force bool) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	flag := "-d"
	if force {
		flag = "-D"
	}

	_, err := git.RunCommand(ctx, r.repoPath, "branch", flag, name)
	if err != nil {
		return fmt.Errorf("failed to delete branch '%s': %w", name, err)
	}

	return nil
}

// Fetch fetches from the specified remote.
func (r *GitWorktreeRunner) Fetch(ctx context.Context, remote string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if remote == "" {
		remote = "origin"
	}

	_, err := git.RunCommand(ctx, r.repoPath, "fetch", remote)
	if err != nil {
		return fmt.Errorf("failed to fetch from %s: %w", remote, err)
	}

	return nil
}

// RemoteBranchExists checks if a branch exists on the specified remote.
func (r *GitWorktreeRunner) RemoteBranchExists(ctx context.Context, remote, name string) (bool, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if remote == "" {
		remote = "origin"
	}

	// Check for refs/remotes/{remote}/{name}
	ref := fmt.Sprintf("refs/remotes/%s/%s", remote, name)
	_, err := git.RunCommand(ctx, r.repoPath, "show-ref", "--verify", ref)
	if err != nil {
		// Exit code 1 or "not a valid ref" means ref not found, which is expected
		errStr := err.Error()
		if strings.Contains(errStr, "exit status 1") || strings.Contains(errStr, "not a valid ref") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check remote branch existence: %w", err)
	}

	return true, nil
}

// generateUniqueBranchName ensures branch name is unique.
// If the base name already exists, appends a timestamp suffix.
// Delegates to the shared git.GenerateUniqueBranchNameWithChecker function.
func (r *GitWorktreeRunner) generateUniqueBranchName(ctx context.Context, baseName string) (string, error) {
	return git.GenerateUniqueBranchNameWithChecker(ctx, r, baseName)
}

// DetectRepoRoot finds the root of the git repository.
// This is a public wrapper for detectRepoRoot.
func DetectRepoRoot(ctx context.Context, path string) (string, error) {
	return detectRepoRoot(ctx, path)
}

// SiblingPath computes the sibling worktree path.
// This is a public wrapper for siblingPath.
func SiblingPath(repoRoot, workspaceName string) string {
	return siblingPath(repoRoot, workspaceName)
}

// detectRepoRoot finds the root of the git repository.
func detectRepoRoot(ctx context.Context, path string) (string, error) {
	output, err := git.RunCommand(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%w: %w", atlaserrors.ErrNotGitRepo, err)
	}
	return output, nil
}

// siblingPath computes the sibling worktree path.
// Given repo at /path/to/myrepo and workspace "auth":
// Returns /path/to/myrepo-auth
func siblingPath(repoRoot, workspaceName string) string {
	repoDir := filepath.Dir(repoRoot)
	repoName := filepath.Base(repoRoot)
	return filepath.Join(repoDir, repoName+"-"+workspaceName)
}

// generateBranchName creates a branch name from type and workspace name.
// This delegates to git.GenerateBranchName for centralized branch naming logic.
func generateBranchName(branchType, workspaceName string) string {
	return git.GenerateBranchName(branchType, workspaceName)
}

// maxPathRetries is the maximum number of numeric suffixes to try before using timestamp.
const maxPathRetries = 100

// cleanupOrphanedPath removes a directory if it exists but is not a registered git worktree.
// This handles the case where a previous destroy failed to fully clean up, leaving an orphaned directory.
func (r *GitWorktreeRunner) cleanupOrphanedPath(ctx context.Context, path string) error {
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}

	// Check if it's a registered worktree
	worktrees, err := r.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	for _, wt := range worktrees {
		if wt.Path == path {
			return nil // It's an active worktree, don't touch it
		}
	}

	// Path exists but isn't a worktree - it's orphaned
	log.Info().Str("path", path).Msg("removing orphaned worktree directory")
	return os.RemoveAll(path)
}

// ensureUniquePath finds a unique worktree path, appending -2, -3, etc.
// Returns the path and an error if no unique path could be found.
// There is an inherent TOCTOU race between this check and actual worktree creation.
// This is acceptable because git worktree add will fail atomically if the path exists,
// and we clean up on failure.
func ensureUniquePath(basePath string) (string, error) {
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return basePath, nil
	}

	for i := 2; i < maxPathRetries; i++ {
		path := fmt.Sprintf("%s-%d", basePath, i)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, nil
		}
	}

	// Fallback to timestamp
	timestampPath := fmt.Sprintf("%s-%d", basePath, time.Now().Unix())
	if _, err := os.Stat(timestampPath); os.IsNotExist(err) {
		return timestampPath, nil
	}

	// Extremely unlikely: all paths including timestamp exist
	return "", fmt.Errorf("path '%s' and all variants already exist: %w", basePath, atlaserrors.ErrWorktreeExists)
}

// parseWorktreeList parses git worktree list --porcelain output.
//
//nolint:nestif // Parsing porcelain output requires nested conditionals
func parseWorktreeList(output string) []*WorktreeInfo {
	var worktrees []*WorktreeInfo
	var current *WorktreeInfo

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				worktrees = append(worktrees, current)
			}
			current = &WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "HEAD ") && current != nil {
			current.HeadCommit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			// refs/heads/feat/auth -> feat/auth
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			current.Branch = branch
		} else if line == "prunable" && current != nil {
			current.IsPrunable = true
		} else if strings.HasPrefix(line, "locked") && current != nil {
			current.IsLocked = true
		}
	}

	if current != nil {
		worktrees = append(worktrees, current)
	}

	return worktrees
}
