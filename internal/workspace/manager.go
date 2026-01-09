// Package workspace provides workspace persistence and management for ATLAS.
// This file implements the Manager which orchestrates workspace lifecycle operations.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TaskLister defines the interface for listing tasks by workspace.
// Used to check for running tasks before closing a workspace.
type TaskLister interface {
	List(ctx context.Context, workspaceName string) ([]*domain.Task, error)
}

// CloseResult contains the result of a workspace close operation.
type CloseResult struct {
	// WorktreeWarning is non-empty if worktree removal failed.
	// The workspace is still marked as closed, but the worktree files remain on disk.
	WorktreeWarning string

	// BranchWarning is non-empty if branch deletion failed.
	// The workspace is still closed, but the branch remains in git.
	BranchWarning string
}

// WarningCollector accumulates non-fatal warnings during workspace operations.
// This allows operations to continue and report all issues at the end.
type WarningCollector struct {
	logger   zerolog.Logger
	context  string // e.g., workspace name for logging context
	warnings []error
}

// newWarningCollector creates a new warning collector with the given context.
func newWarningCollector(logger zerolog.Logger, context string) *WarningCollector {
	return &WarningCollector{
		logger:  logger,
		context: context,
	}
}

// Add appends a warning to the collector.
func (w *WarningCollector) Add(err error) {
	w.warnings = append(w.warnings, err)
}

// Addf appends a formatted warning to the collector.
//
//nolint:err113 // Dynamic errors acceptable for warning messages
func (w *WarningCollector) Addf(format string, args ...any) {
	w.warnings = append(w.warnings, fmt.Errorf(format, args...))
}

// Log writes all collected warnings to the logger.
func (w *WarningCollector) Log() {
	for _, warn := range w.warnings {
		w.logger.Warn().Err(warn).Str("workspace", w.context).Msg("operation warning")
	}
}

// Warnings returns all collected warnings.
func (w *WarningCollector) Warnings() []error {
	return w.warnings
}

// CreateOptions contains options for creating a new workspace.
// Using an options struct instead of positional parameters makes the API
// clearer and easier to extend without breaking changes.
type CreateOptions struct {
	// Name is the workspace name (required).
	Name string

	// RepoPath is the path to the git repository (required).
	RepoPath string

	// BranchType is the branch prefix type, e.g., "feature", "bugfix" (required).
	BranchType string

	// BaseBranch is the branch to create from. If empty, uses the default branch.
	// Supports both local and remote branches.
	BaseBranch string

	// UseLocal prefers local branches over remote when both exist.
	// Default (false) prefers remote branches for safety.
	UseLocal bool
}

// Reader provides read-only access to workspaces.
// Use this interface when you only need to query workspace information.
type Reader interface {
	// Get retrieves a workspace by name.
	// Returns ErrWorkspaceNotFound if not found.
	Get(ctx context.Context, name string) (*domain.Workspace, error)

	// List returns all workspaces.
	// Returns empty slice if none exist.
	List(ctx context.Context) ([]*domain.Workspace, error)

	// Exists returns true if a workspace exists.
	Exists(ctx context.Context, name string) (bool, error)
}

// Creator handles workspace creation.
// Use this interface when you only need to create workspaces.
type Creator interface {
	// Create creates a new workspace with a git worktree.
	// If BaseBranch is specified, validates it exists (locally or remotely) and creates from it.
	// By default, prefers remote branches (origin/branch) over local branches for safety.
	// Set UseLocal=true to explicitly prefer local branches when both exist.
	// Returns ErrWorkspaceExists if an active or paused workspace already exists.
	// Closed workspaces are automatically cleaned up, allowing the name to be reused.
	// Returns ErrBranchNotFound if BaseBranch is specified but doesn't exist.
	Create(ctx context.Context, opts CreateOptions) (*domain.Workspace, error)
}

// Lifecycle manages workspace lifecycle operations.
// Use this interface when you need to modify workspace state.
type Lifecycle interface {
	// Destroy removes a workspace and its worktree.
	// ALWAYS succeeds even if state is corrupted (NFR18).
	Destroy(ctx context.Context, name string) error

	// Close archives a workspace, removing worktree but keeping state.
	// If taskLister is provided, checks for running/validating tasks before closing.
	// Returns error if tasks are running or validating.
	// Returns CloseResult with WorktreeWarning if worktree removal fails.
	Close(ctx context.Context, name string, taskLister TaskLister) (*CloseResult, error)

	// UpdateStatus updates the status of a workspace.
	UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error
}

// Manager orchestrates workspace lifecycle operations.
// It coordinates between the Store (state persistence) and WorktreeRunner (git worktrees).
// Manager composes Reader, Creator, and Lifecycle interfaces,
// allowing consumers to depend on the minimal interface they need.
type Manager interface {
	Reader
	Creator
	Lifecycle
}

// DefaultManager implements Manager using Store and WorktreeRunner.
type DefaultManager struct {
	store          Store
	worktreeRunner WorktreeRunner
	logger         zerolog.Logger
}

// NewManager creates a new DefaultManager.
func NewManager(store Store, worktreeRunner WorktreeRunner, logger zerolog.Logger) *DefaultManager {
	return &DefaultManager{
		store:          store,
		worktreeRunner: worktreeRunner,
		logger:         logger,
	}
}

// Create creates a new workspace with a git worktree.
func (m *DefaultManager) Create(ctx context.Context, opts CreateOptions) (*domain.Workspace, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate inputs
	if opts.Name == "" {
		return nil, fmt.Errorf("failed to create workspace: name %w", atlaserrors.ErrEmptyValue)
	}
	if opts.RepoPath == "" {
		return nil, fmt.Errorf("failed to create workspace: repoPath %w", atlaserrors.ErrEmptyValue)
	}
	if opts.BranchType == "" {
		return nil, fmt.Errorf("failed to create workspace: branchType %w", atlaserrors.ErrEmptyValue)
	}
	if m.worktreeRunner == nil {
		return nil, fmt.Errorf("failed to create workspace: %w", atlaserrors.ErrWorktreeRunnerNotAvailable)
	}

	// Check if workspace already exists
	existingWs, err := m.store.Get(ctx, opts.Name)
	if err != nil && !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
		return nil, fmt.Errorf("failed to check workspace existence: %w", err)
	}

	// Handle existing workspace
	if existingWs != nil {
		// Active or paused workspace - cannot overwrite
		if existingWs.Status != constants.WorkspaceStatusClosed {
			return nil, fmt.Errorf("failed to create workspace '%s': %w", opts.Name, atlaserrors.ErrWorkspaceExists)
		}
		// Reset metadata (preserve tasks) to make room for the new workspace
		if deleteErr := m.store.ResetMetadata(ctx, opts.Name); deleteErr != nil {
			return nil, fmt.Errorf("failed to cleanup closed workspace '%s': %w", opts.Name, deleteErr)
		}
	}

	// Validate and resolve base branch if specified
	resolvedBaseBranch := opts.BaseBranch
	if opts.BaseBranch != "" {
		var resolveErr error
		resolved, resolveErr := m.ensureBaseBranch(ctx, opts.BaseBranch, opts.UseLocal)
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to validate base branch '%s': %w", opts.BaseBranch, resolveErr)
		}
		resolvedBaseBranch = resolved
	}

	// Create worktree
	wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
		RepoPath:      opts.RepoPath,
		WorkspaceName: opts.Name,
		BranchType:    opts.BranchType,
		BaseBranch:    resolvedBaseBranch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Build workspace
	now := time.Now()
	ws := &domain.Workspace{
		Name:         opts.Name,
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}
	return m.store.Get(ctx, name)
}

// List returns all workspaces.
func (m *DefaultManager) List(ctx context.Context) ([]*domain.Workspace, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}
	return m.store.List(ctx)
}

// Destroy removes a workspace and its worktree.
// ALWAYS succeeds even if state is corrupted (NFR18).
func (m *DefaultManager) Destroy(ctx context.Context, name string) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	// Collect warnings (for logging in production)
	wc := newWarningCollector(m.logger, name)

	// Try to load workspace (may be corrupted)
	ws := m.loadWorkspaceForDestroy(ctx, name, &wc.warnings)

	// Try to remove worktree
	m.removeWorktree(ctx, ws, &wc.warnings)

	// Prune stale worktrees (must happen BEFORE deleteBranch so git doesn't
	// think a stale worktree is still using the branch)
	m.pruneWorktrees(ctx, &wc.warnings)

	// Try to delete branch
	m.deleteBranch(ctx, ws, &wc.warnings)

	// Delete workspace state
	m.deleteWorkspaceState(ctx, name, &wc.warnings)

	// NFR18: ALWAYS succeed - warnings are collected but not returned as errors
	// Log warnings for debugging and observability
	wc.Log()

	return nil
}

// removeOrphanedDirectory removes a directory that is no longer a registered git worktree.
// This is used as a fallback when git worktree remove fails.
func removeOrphanedDirectory(logger zerolog.Logger, path string) error {
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

	logger.Info().Str("path", path).Msg("removing orphaned worktree directory")
	return os.RemoveAll(path)
}

// Close archives a workspace, removing worktree but keeping state.
func (m *DefaultManager) Close(ctx context.Context, name string, taskLister TaskLister) (*CloseResult, error) {
	result := &CloseResult{}

	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Load workspace
	ws, err := m.store.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to close workspace '%s': %w", name, err)
	}

	// Check for running tasks using the authoritative task store (ws.Tasks is not populated at runtime)
	if err := m.checkForRunningTasks(ctx, name, taskLister); err != nil {
		return nil, err
	}

	// Store the worktree path before clearing it
	worktreePath := ws.WorktreePath

	// Discovery: if path is empty, try to find worktree by branch name
	// This handles recovery from previous failed close operations where
	// state was persisted but worktree removal failed
	if worktreePath == "" && m.worktreeRunner != nil && ws.Branch != "" {
		worktreePath = m.worktreeRunner.FindByBranch(ctx, ws.Branch)
		if worktreePath != "" {
			m.logger.Info().
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

	// Delete branch if worktree was successfully removed
	// This prevents orphaned local branches after PR merge
	if worktreeRemoved && m.worktreeRunner != nil && ws.Branch != "" {
		result.BranchWarning = m.tryDeleteBranch(ctx, ws.Branch)
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

	logEvent := m.logger.Info().Str("workspace", name)
	if result.WorktreeWarning != "" {
		logEvent = logEvent.Str("warning", result.WorktreeWarning)
	}
	logEvent.Msg("workspace closed")

	return result, nil
}

// UpdateStatus updates the status of a workspace.
func (m *DefaultManager) UpdateStatus(ctx context.Context, name string, status constants.WorkspaceStatus) error {
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
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
	if err := ctxutil.Canceled(ctx); err != nil {
		return false, err
	}
	return m.store.Exists(ctx, name)
}

// checkForRunningTasks verifies no tasks are in running or validating state.
// Returns an error if any active tasks are found.
// If taskLister is nil or returns an error, returns nil to allow fail-open behavior.
func (m *DefaultManager) checkForRunningTasks(ctx context.Context, name string, taskLister TaskLister) error {
	if taskLister == nil {
		return nil
	}

	tasks, err := taskLister.List(ctx, name)
	if err != nil {
		// Fail-open: don't block close when task store is unavailable
		m.logger.Warn().Err(err).Str("workspace", name).
			Msg("could not check for running tasks, proceeding with close")
		return nil
	}

	for _, task := range tasks {
		if task.Status == constants.TaskStatusRunning ||
			task.Status == constants.TaskStatusValidating {
			return fmt.Errorf("cannot close workspace '%s': task '%s' is still %s: %w",
				name, task.ID, task.Status, atlaserrors.ErrWorkspaceHasRunningTasks)
		}
	}
	return nil
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
		m.logger.Warn().Err(verifyErr).Msg("could not verify worktree removal")
		return false
	}

	if !dirExists && !inGit {
		m.logger.Debug().Str("path", path).Msg("worktree removed successfully")
		return true
	}

	m.logger.Warn().
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

// tryDeleteBranch safely attempts to delete a branch after worktree removal.
// Returns warning message if deletion fails (non-blocking).
func (m *DefaultManager) tryDeleteBranch(ctx context.Context, branch string) string {
	// Step 1: Prune BEFORE attempting branch deletion (critical for safety)
	// This cleans up stale worktree metadata to prevent "branch is checked out" errors
	if err := m.worktreeRunner.Prune(ctx); err != nil {
		m.logger.Warn().Err(err).Msg("prune failed before branch deletion")
	}

	// Step 1.5: Check if branch exists before attempting deletion
	// This handles cases where branch was already deleted or workspace data is stale
	exists, err := m.worktreeRunner.BranchExists(ctx, branch)
	if err != nil {
		m.logger.Warn().Err(err).Str("branch", branch).Msg("failed to check branch existence")
		// Continue with deletion attempt despite error
	} else if !exists {
		// Branch doesn't exist - skip deletion, don't warn
		m.logger.Debug().Str("branch", branch).Msg("branch already deleted or never existed")
		return ""
	}

	// Step 2: Safety check - ensure no other worktrees use this branch
	otherWorktreePath := m.worktreeRunner.FindByBranch(ctx, branch)
	if otherWorktreePath != "" {
		return fmt.Sprintf("branch '%s' not deleted: in use by worktree at '%s'",
			branch, otherWorktreePath)
	}

	// Step 3: Delete branch with force flag
	if err := m.worktreeRunner.DeleteBranch(ctx, branch, true); err != nil {
		return fmt.Sprintf("failed to delete branch '%s': %v", branch, err)
	}

	m.logger.Info().Str("branch", branch).Msg("branch deleted after workspace close")
	return ""
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
			m.logger.Info().
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
	m.logger.Info().Str("path", worktreePath).Msg("attempting fallback: manual directory removal")
	if removeErr := removeOrphanedDirectory(m.logger, worktreePath); removeErr != nil {
		*warnings = append(*warnings, fmt.Errorf("warning: failed to remove directory '%s': %w", worktreePath, removeErr))
		return // Can't proceed with cleanup
	}

	// After removing directory, immediately try to prune to clean up git metadata
	m.logger.Debug().Msg("directory removed, attempting immediate prune")
	if pruneErr := m.worktreeRunner.Prune(ctx); pruneErr != nil {
		m.logger.Warn().Err(pruneErr).Msg("prune after directory removal failed")
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
		m.logger.Info().Str("path", worktreePath).Msg("worktree removed via fallback + prune")
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
// Returns the resolved branch reference to use (e.g., "origin/develop" for remote, "develop" for local).
// By default (useLocal=false), prefers remote branches for safety.
// When useLocal=true, prefers local branches and errors if local doesn't exist but remote does.
func (m *DefaultManager) ensureBaseBranch(ctx context.Context, branch string, useLocal bool) (string, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return "", err
	}

	// Check if local branch exists
	localExists, err := m.worktreeRunner.BranchExists(ctx, branch)
	if err != nil {
		return "", fmt.Errorf("failed to check local branch: %w", err)
	}

	m.logger.Debug().
		Str("branch", branch).
		Bool("local_exists", localExists).
		Bool("use_local", useLocal).
		Msg("checking branch availability")

	// If useLocal is true, use the current logic (local first)
	if useLocal {
		if localExists {
			m.logger.Info().
				Str("branch", branch).
				Str("source", "local").
				Bool("use_local_flag", true).
				Msg("using local branch")
			return branch, nil
		}
		// --use-local was specified but branch doesn't exist locally
		return "", fmt.Errorf("%w: --use-local was specified but branch '%s' does not exist locally. "+
			"Create a local branch with 'git checkout -b %s origin/%s' or omit --use-local",
			atlaserrors.ErrBranchNotFound, branch, branch, branch)
	}

	// NEW DEFAULT: Remote first (useLocal=false)
	// Try fetching from remote
	// Fetch may fail if branch doesn't exist or network error
	// We'll still check if remote branch exists after fetch attempt
	// (fetch may have partially succeeded or refs may already be present)
	_ = m.worktreeRunner.Fetch(ctx, constants.DefaultRemote)

	// Check if branch exists on remote
	remoteExists, err := m.worktreeRunner.RemoteBranchExists(ctx, constants.DefaultRemote, branch)
	if err != nil {
		return "", fmt.Errorf("failed to check remote branch: %w", err)
	}

	m.logger.Debug().
		Str("branch", branch).
		Bool("remote_exists", remoteExists).
		Bool("local_exists", localExists).
		Str("remote", constants.DefaultRemote).
		Msg("checked remote branch availability")

	if remoteExists {
		m.logger.Info().
			Str("branch", branch).
			Str("source", "remote").
			Str("remote", constants.DefaultRemote).
			Str("resolved_ref", fmt.Sprintf("%s/%s", constants.DefaultRemote, branch)).
			Msg("resolved to remote branch")
		// Return the remote tracking reference
		return fmt.Sprintf("%s/%s", constants.DefaultRemote, branch), nil
	}

	// Fallback to local if remote doesn't exist but local does
	// This allows working with local-only branches
	if localExists {
		m.logger.Info().
			Str("branch", branch).
			Str("source", "local").
			Bool("remote_exists", false).
			Msg("resolved to local branch (remote not available)")
		return branch, nil
	}

	// Branch doesn't exist locally or remotely
	return "", fmt.Errorf("%w: branch '%s' does not exist locally or on remote '%s'. "+
		"Use 'git branch -a' to see available branches",
		atlaserrors.ErrBranchNotFound, branch, constants.DefaultRemote)
}
