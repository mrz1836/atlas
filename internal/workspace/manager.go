// Package workspace provides workspace persistence and management for ATLAS.
// This file implements the Manager which orchestrates workspace lifecycle operations.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// CloseResult contains the result of a workspace close operation.
type CloseResult struct {
	// WorktreeWarning is non-empty if worktree removal failed.
	// The workspace is still marked as closed, but the worktree files remain on disk.
	WorktreeWarning string
}

// Manager orchestrates workspace lifecycle operations.
// It coordinates between the Store (state persistence) and WorktreeRunner (git worktrees).
type Manager interface {
	// Create creates a new workspace with a git worktree.
	// If baseBranch is specified, validates it exists (locally or remotely) and creates from it.
	// Returns ErrWorkspaceExists if an active or paused workspace already exists.
	// Closed workspaces are automatically cleaned up, allowing the name to be reused.
	// Returns ErrBranchNotFound if baseBranch is specified but doesn't exist.
	Create(ctx context.Context, name, repoPath, branchType, baseBranch string) (*domain.Workspace, error)

	// Get retrieves a workspace by name.
	// Returns ErrWorkspaceNotFound if not found.
	Get(ctx context.Context, name string) (*domain.Workspace, error)

	// List returns all workspaces.
	// Returns empty slice if none exist.
	List(ctx context.Context) ([]*domain.Workspace, error)

	// Destroy removes a workspace and its worktree.
	// ALWAYS succeeds even if state is corrupted (NFR18).
	Destroy(ctx context.Context, name string) error

	// Close archives a workspace, removing worktree but keeping state.
	// Returns error if tasks are running.
	// Returns CloseResult with WorktreeWarning if worktree removal fails.
	Close(ctx context.Context, name string) (*CloseResult, error)

	// UpdateStatus updates the status of a workspace.
	UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error

	// Exists returns true if a workspace exists.
	Exists(ctx context.Context, name string) (bool, error)
}

// DefaultManager implements Manager using Store and WorktreeRunner.
type DefaultManager struct {
	store          Store
	worktreeRunner WorktreeRunner
}

// NewManager creates a new DefaultManager.
func NewManager(store Store, worktreeRunner WorktreeRunner) *DefaultManager {
	return &DefaultManager{
		store:          store,
		worktreeRunner: worktreeRunner,
	}
}

// Create creates a new workspace with a git worktree.
func (m *DefaultManager) Create(ctx context.Context, name, repoPath, branchType, baseBranch string) (*domain.Workspace, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate inputs
	if name == "" {
		return nil, fmt.Errorf("failed to create workspace: name %w", atlaserrors.ErrEmptyValue)
	}
	if repoPath == "" {
		return nil, fmt.Errorf("failed to create workspace: repoPath %w", atlaserrors.ErrEmptyValue)
	}
	if branchType == "" {
		return nil, fmt.Errorf("failed to create workspace: branchType %w", atlaserrors.ErrEmptyValue)
	}
	if m.worktreeRunner == nil {
		return nil, fmt.Errorf("failed to create workspace: %w", atlaserrors.ErrWorktreeRunnerNotAvailable)
	}

	// Check if workspace already exists
	existingWs, err := m.store.Get(ctx, name)
	if err != nil && !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
		return nil, fmt.Errorf("failed to check workspace existence: %w", err)
	}

	// Handle existing workspace
	if existingWs != nil {
		// Active or paused workspace - cannot overwrite
		if existingWs.Status != constants.WorkspaceStatusClosed {
			return nil, fmt.Errorf("failed to create workspace '%s': %w", name, atlaserrors.ErrWorkspaceExists)
		}
		// Delete the closed workspace entry to make room for the new one
		if deleteErr := m.store.Delete(ctx, name); deleteErr != nil {
			return nil, fmt.Errorf("failed to cleanup closed workspace '%s': %w", name, deleteErr)
		}
	}

	// Validate and resolve base branch if specified
	resolvedBaseBranch := baseBranch
	if baseBranch != "" {
		var resolveErr error
		resolved, resolveErr := m.ensureBaseBranch(ctx, baseBranch)
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to validate base branch '%s': %w", baseBranch, resolveErr)
		}
		resolvedBaseBranch = resolved
	}

	// Create worktree
	wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
		RepoPath:      repoPath,
		WorkspaceName: name,
		BranchType:    branchType,
		BaseBranch:    resolvedBaseBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Build workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         name,
		WorktreePath: wtInfo.Path,
		Branch:       wtInfo.Branch,
		Status:       constants.WorkspaceStatusActive,
		Tasks:        []domain.TaskRef{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Persist to store
	if err := m.store.Create(ctx, ws); err != nil {
		// CRITICAL: Rollback worktree on store failure
		_ = m.worktreeRunner.Remove(ctx, wtInfo.Path, true)
		return nil, fmt.Errorf("failed to persist workspace: %w", err)
	}

	return ws, nil
}

// Get retrieves a workspace by name.
func (m *DefaultManager) Get(ctx context.Context, name string) (*domain.Workspace, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return m.store.Get(ctx, name)
}

// List returns all workspaces.
func (m *DefaultManager) List(ctx context.Context) ([]*domain.Workspace, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return m.store.List(ctx)
}

// Destroy removes a workspace and its worktree.
// ALWAYS succeeds even if state is corrupted (NFR18).
func (m *DefaultManager) Destroy(ctx context.Context, name string) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Collect warnings (for logging in production)
	var warnings []error

	// Try to load workspace (may be corrupted)
	ws := m.loadWorkspaceForDestroy(ctx, name, &warnings)

	// Try to remove worktree
	m.removeWorktree(ctx, ws, &warnings)

	// Prune stale worktrees (must happen BEFORE deleteBranch so git doesn't
	// think a stale worktree is still using the branch)
	m.pruneWorktrees(ctx, &warnings)

	// Try to delete branch
	m.deleteBranch(ctx, ws, &warnings)

	// Delete workspace state
	m.deleteWorkspaceState(ctx, name, &warnings)

	// NFR18: ALWAYS succeed - warnings are collected but not returned as errors
	// Log warnings for debugging and observability
	for _, warn := range warnings {
		log.Warn().Err(warn).Str("workspace", name).Msg("destroy warning")
	}

	return nil
}

// removeOrphanedDirectory removes a directory that is no longer a registered git worktree.
// This is used as a fallback when git worktree remove fails.
func removeOrphanedDirectory(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil // Already gone
	}
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	// Only remove directories, not files
	if !info.IsDir() {
		return fmt.Errorf("%w: %s", atlaserrors.ErrNotADirectory, path)
	}

	log.Info().Str("path", path).Msg("removing orphaned worktree directory")
	return os.RemoveAll(path)
}

// Close archives a workspace, removing worktree but keeping state.
func (m *DefaultManager) Close(ctx context.Context, name string) (*CloseResult, error) {
	result := &CloseResult{}

	// Capture caller information for debugging workspace closure issues
	// This helps trace unexpected Close() calls that might be corrupting workspace state
	pc := make([]uintptr, 10)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	var callers []string
	for {
		frame, more := frames.Next()
		callers = append(callers, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more || len(callers) >= 5 {
			break
		}
	}

	log.Info().
		Str("workspace", name).
		Strs("call_stack", callers).
		Msg("workspace Close() called - tracing for debugging")

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Load workspace
	ws, err := m.store.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to close workspace '%s': %w", name, err)
	}

	// Log workspace state before close
	log.Info().
		Str("workspace", name).
		Str("current_status", string(ws.Status)).
		Str("worktree_path", ws.WorktreePath).
		Str("branch", ws.Branch).
		Int("task_count", len(ws.Tasks)).
		Msg("workspace state before close")

	// Check for running tasks
	for _, task := range ws.Tasks {
		if task.Status == constants.TaskStatusRunning ||
			task.Status == constants.TaskStatusValidating {
			return nil, fmt.Errorf("cannot close workspace '%s': task '%s' is still running: %w",
				name, task.ID, atlaserrors.ErrWorkspaceHasRunningTasks)
		}
	}

	// Store the worktree path before clearing it
	worktreePath := ws.WorktreePath

	// Discovery: if path is empty, try to find worktree by branch name
	// This handles recovery from previous failed close operations where
	// state was persisted but worktree removal failed
	if worktreePath == "" && m.worktreeRunner != nil && ws.Branch != "" {
		worktreePath = m.worktreeRunner.FindByBranch(ctx, ws.Branch)
		if worktreePath != "" {
			log.Info().
				Str("branch", ws.Branch).
				Str("discovered_path", worktreePath).
				Msg("discovered orphaned worktree by branch")
		}
	}

	// Remove worktree FIRST (before updating state)
	// This prevents state corruption where WorktreePath is cleared but files remain
	worktreeRemoved := false
	if worktreePath != "" {
		result.WorktreeWarning = m.tryRemoveWorktree(ctx, worktreePath)
		worktreeRemoved = (result.WorktreeWarning == "")
	}

	// Update state - only clear path if removal succeeded or was already empty
	ws.Status = constants.WorkspaceStatusClosed
	if worktreeRemoved || ws.WorktreePath == "" {
		ws.WorktreePath = ""
	}
	ws.UpdatedAt = time.Now()

	if err := m.store.Update(ctx, ws); err != nil {
		return nil, fmt.Errorf("failed to update workspace status: %w", err)
	}

	log.Info().
		Str("workspace", name).
		Str("worktree_removed", worktreePath).
		Str("warning", result.WorktreeWarning).
		Bool("path_cleared", worktreeRemoved || ws.WorktreePath == "").
		Msg("workspace Close() completed successfully")

	return result, nil
}

// UpdateStatus updates the status of a workspace.
func (m *DefaultManager) UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Load workspace
	ws, err := m.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to update workspace '%s' status: %w", name, err)
	}

	// Update Status field and timestamp (Manager owns timestamp for consistency)
	ws.Status = status
	ws.UpdatedAt = time.Now()

	// Persist via store.Update()
	if err := m.store.Update(ctx, ws); err != nil {
		return fmt.Errorf("failed to update workspace '%s' status: %w", name, err)
	}

	return nil
}

// Exists returns true if a workspace exists.
func (m *DefaultManager) Exists(ctx context.Context, name string) (bool, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return m.store.Exists(ctx, name)
}

// verifyWorktreeRemoved checks if a worktree was actually removed from both
// filesystem and git metadata.
func (m *DefaultManager) verifyWorktreeRemoved(ctx context.Context, path string) (dirExists, inGitMetadata bool, err error) {
	// Check filesystem
	_, statErr := os.Stat(path)
	dirExists = !os.IsNotExist(statErr)

	// Check git metadata
	if m.worktreeRunner == nil {
		return dirExists, false, nil
	}

	worktrees, listErr := m.worktreeRunner.List(ctx)
	if listErr != nil {
		return dirExists, false, fmt.Errorf("failed to list worktrees: %w", listErr)
	}

	absPath, _ := filepath.Abs(path)
	for _, wt := range worktrees {
		if wt.Path == absPath || wt.Path == path {
			inGitMetadata = true
			break
		}
	}

	return dirExists, inGitMetadata, nil
}

// verifyAndLogRemoval verifies worktree removal and logs the result.
// Returns true if removal was successful (no dir, no git metadata).
func (m *DefaultManager) verifyAndLogRemoval(ctx context.Context, path string) bool {
	dirExists, inGit, verifyErr := m.verifyWorktreeRemoved(ctx, path)
	if verifyErr != nil {
		log.Warn().Err(verifyErr).Msg("could not verify worktree removal")
		return false
	}

	if !dirExists && !inGit {
		log.Debug().Str("path", path).Msg("worktree removed successfully")
		return true
	}

	log.Warn().
		Bool("dir_exists", dirExists).
		Bool("in_git", inGit).
		Str("path", path).
		Msg("git worktree remove succeeded but worktree still detected")
	return false
}

// tryRemoveWorktree attempts to remove a worktree, returning a warning message if it fails.
// It tries normal removal first, then force removal if that fails.
func (m *DefaultManager) tryRemoveWorktree(ctx context.Context, worktreePath string) string {
	if m.worktreeRunner == nil {
		return fmt.Sprintf("worktree at '%s' was not removed: no worktree runner available", worktreePath)
	}

	err := m.worktreeRunner.Remove(ctx, worktreePath, false)
	if err == nil {
		return "" // Success
	}

	// If worktree is dirty or has other issues, force remove
	forceErr := m.worktreeRunner.Remove(ctx, worktreePath, true)
	if forceErr != nil {
		// Both attempts failed - return warning
		return fmt.Sprintf("failed to remove worktree at '%s': %v", worktreePath, forceErr)
	}

	return "" // Force removal succeeded
}

// loadWorkspaceForDestroy attempts to load a workspace, collecting warnings on failure.
func (m *DefaultManager) loadWorkspaceForDestroy(ctx context.Context, name string, warnings *[]error) *domain.Workspace {
	ws, err := m.store.Get(ctx, name)
	if err != nil && !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
		// Log warning but continue - state might be corrupted
		*warnings = append(*warnings, fmt.Errorf("warning: failed to load workspace: %w", err))
	}
	return ws
}

// removeWorktree removes the worktree directory and falls back to direct removal if needed.
func (m *DefaultManager) removeWorktree(ctx context.Context, ws *domain.Workspace, warnings *[]error) {
	if ws == nil {
		return
	}

	if m.worktreeRunner == nil {
		if ws.WorktreePath != "" {
			//nolint:err113 // warning messages, not returned errors
			*warnings = append(*warnings, fmt.Errorf("warning: worktree runner not available, cannot remove worktree at '%s'", ws.WorktreePath))
		}
		return
	}

	// Get worktree path, using branch-based discovery if path is empty
	worktreePath := ws.WorktreePath
	if worktreePath == "" && ws.Branch != "" {
		// Discovery: try to find worktree by branch name
		// This handles recovery from previous failed close operations where
		// state was persisted but worktree removal failed
		worktreePath = m.worktreeRunner.FindByBranch(ctx, ws.Branch)
		if worktreePath != "" {
			log.Info().
				Str("branch", ws.Branch).
				Str("discovered_path", worktreePath).
				Msg("discovered orphaned worktree by branch for destroy")
		} else {
			return // No worktree to remove
		}
	}

	if worktreePath == "" {
		return // No worktree path and no branch to discover by
	}

	// Attempt 1: Normal git worktree remove with --force
	err := m.worktreeRunner.Remove(ctx, worktreePath, true)
	if err == nil {
		// Verify it actually worked
		if m.verifyAndLogRemoval(ctx, worktreePath) {
			return // Complete success!
		}
	} else {
		*warnings = append(*warnings, fmt.Errorf("warning: git worktree remove failed for '%s': %w", worktreePath, err))
	}

	// Attempt 2: Fallback to manual directory removal + immediate prune
	log.Info().Str("path", worktreePath).Msg("attempting fallback: manual directory removal")
	if removeErr := removeOrphanedDirectory(worktreePath); removeErr != nil {
		*warnings = append(*warnings, fmt.Errorf("warning: failed to remove directory '%s': %w", worktreePath, removeErr))
		return // Can't proceed with cleanup
	}

	// After removing directory, immediately try to prune to clean up git metadata
	log.Debug().Msg("directory removed, attempting immediate prune")
	if pruneErr := m.worktreeRunner.Prune(ctx); pruneErr != nil {
		log.Warn().Err(pruneErr).Msg("prune after directory removal failed")
	}

	// Final verification
	dirExists, inGit, verifyErr := m.verifyWorktreeRemoved(ctx, worktreePath)
	if verifyErr != nil {
		*warnings = append(*warnings, fmt.Errorf("warning: could not verify worktree removal: %w", verifyErr))
	} else if dirExists || inGit {
		//nolint:err113 // warning messages, not returned errors
		*warnings = append(*warnings, fmt.Errorf(
			"warning: worktree cleanup incomplete for '%s' (dir_exists=%v, in_git_metadata=%v)",
			worktreePath, dirExists, inGit))
	} else {
		log.Info().Str("path", worktreePath).Msg("worktree removed via fallback + prune")
	}
}

// deleteBranch deletes the workspace branch.
func (m *DefaultManager) deleteBranch(ctx context.Context, ws *domain.Workspace, warnings *[]error) {
	if ws == nil || ws.Branch == "" {
		return
	}

	if m.worktreeRunner == nil {
		//nolint:err113 // warning messages, not returned errors
		*warnings = append(*warnings, fmt.Errorf("warning: worktree runner not available, cannot delete branch '%s'", ws.Branch))
		return
	}

	if err := m.worktreeRunner.DeleteBranch(ctx, ws.Branch, true); err != nil {
		// Log warning but continue - branch might already be deleted
		*warnings = append(*warnings, fmt.Errorf("warning: failed to delete branch: %w", err))
	}
}

// pruneWorktrees prunes stale worktrees from git.
func (m *DefaultManager) pruneWorktrees(ctx context.Context, warnings *[]error) {
	if m.worktreeRunner == nil {
		//nolint:err113 // warning messages, not returned errors
		*warnings = append(*warnings, fmt.Errorf("warning: worktree runner not available, cannot prune stale worktrees"))
		return
	}

	if err := m.worktreeRunner.Prune(ctx); err != nil {
		*warnings = append(*warnings, fmt.Errorf("warning: failed to prune worktrees: %w", err))
	}
}

// deleteWorkspaceState deletes the workspace state from the store.
func (m *DefaultManager) deleteWorkspaceState(ctx context.Context, name string, warnings *[]error) {
	if err := m.store.Delete(ctx, name); err != nil {
		if !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
			*warnings = append(*warnings, fmt.Errorf("warning: failed to delete workspace state: %w", err))
		}
	}
}

// ensureBaseBranch ensures the base branch exists, fetching from remote if needed.
// Returns the resolved branch reference to use (e.g., "origin/develop" if only remote exists).
func (m *DefaultManager) ensureBaseBranch(ctx context.Context, branch string) (string, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Check local first
	localExists, err := m.worktreeRunner.BranchExists(ctx, branch)
	if err != nil {
		return "", fmt.Errorf("failed to check local branch: %w", err)
	}
	if localExists {
		return branch, nil
	}

	// Try fetching from remote
	// Fetch may fail if branch doesn't exist or network error
	// We'll still check if remote branch exists after fetch attempt
	// (fetch may have partially succeeded or refs may already be present)
	_ = m.worktreeRunner.Fetch(ctx, "origin")

	// Check if branch exists on remote
	remoteExists, err := m.worktreeRunner.RemoteBranchExists(ctx, "origin", branch)
	if err != nil {
		return "", fmt.Errorf("failed to check remote branch: %w", err)
	}
	if remoteExists {
		// Return the remote tracking reference
		return fmt.Sprintf("origin/%s", branch), nil
	}

	// Branch doesn't exist locally or remotely
	return "", atlaserrors.ErrBranchNotFound
}
