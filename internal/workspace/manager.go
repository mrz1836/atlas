// Package workspace provides workspace persistence and management for ATLAS.
// This file implements the Manager which orchestrates workspace lifecycle operations.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Manager orchestrates workspace lifecycle operations.
// It coordinates between the Store (state persistence) and WorktreeRunner (git worktrees).
type Manager interface {
	// Create creates a new workspace with a git worktree.
	// Returns ErrWorkspaceExists if workspace already exists.
	Create(ctx context.Context, name, repoPath, branchType string) (*domain.Workspace, error)

	// Get retrieves a workspace by name.
	// Returns ErrWorkspaceNotFound if not found.
	Get(ctx context.Context, name string) (*domain.Workspace, error)

	// List returns all workspaces.
	// Returns empty slice if none exist.
	List(ctx context.Context) ([]*domain.Workspace, error)

	// Destroy removes a workspace and its worktree.
	// ALWAYS succeeds even if state is corrupted (NFR18).
	Destroy(ctx context.Context, name string) error

	// Retire archives a workspace, removing worktree but keeping state.
	// Returns error if tasks are running.
	Retire(ctx context.Context, name string) error

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
func (m *DefaultManager) Create(ctx context.Context, name, repoPath, branchType string) (*domain.Workspace, error) {
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

	// Check if workspace already exists
	exists, err := m.store.Exists(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check workspace existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("failed to create workspace '%s': %w", name, atlaserrors.ErrWorkspaceExists)
	}

	// Create worktree
	wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
		RepoPath:      repoPath,
		WorkspaceName: name,
		BranchType:    branchType,
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
	ws, err := m.store.Get(ctx, name)
	if err != nil && !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
		// Log warning but continue - state might be corrupted
		warnings = append(warnings, fmt.Errorf("warning: failed to load workspace: %w", err))
	}

	// Try to remove worktree if we know the path
	if ws != nil && ws.WorktreePath != "" {
		if err := m.worktreeRunner.Remove(ctx, ws.WorktreePath, true); err != nil {
			// Log warning but continue
			warnings = append(warnings, fmt.Errorf("warning: failed to remove worktree: %w", err))
		}
	}

	// Try to delete branch if we know it
	if ws != nil && ws.Branch != "" {
		if err := m.worktreeRunner.DeleteBranch(ctx, ws.Branch, true); err != nil {
			// Log warning but continue - branch might already be deleted
			warnings = append(warnings, fmt.Errorf("warning: failed to delete branch: %w", err))
		}
	}

	// Prune stale worktrees
	if err := m.worktreeRunner.Prune(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("warning: failed to prune worktrees: %w", err))
	}

	// Delete workspace state
	if err := m.store.Delete(ctx, name); err != nil {
		if !errors.Is(err, atlaserrors.ErrWorkspaceNotFound) {
			warnings = append(warnings, fmt.Errorf("warning: failed to delete workspace state: %w", err))
		}
	}

	// NFR18: ALWAYS succeed - warnings are collected but not returned as errors
	// Future: integrate with zerolog for warning output
	_ = warnings

	return nil
}

// Retire archives a workspace, removing worktree but keeping state.
func (m *DefaultManager) Retire(ctx context.Context, name string) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Load workspace
	ws, err := m.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to retire workspace '%s': %w", name, err)
	}

	// Check for running tasks
	for _, task := range ws.Tasks {
		if task.Status == constants.TaskStatusRunning ||
			task.Status == constants.TaskStatusValidating {
			return fmt.Errorf("cannot retire workspace '%s': task '%s' is still running: %w",
				name, task.ID, atlaserrors.ErrWorkspaceHasRunningTasks)
		}
	}

	// Store the worktree path before clearing it
	worktreePath := ws.WorktreePath

	// Update state FIRST (before removing worktree) for consistency
	// If store update fails, worktree is still intact
	ws.Status = constants.WorkspaceStatusRetired
	ws.WorktreePath = "" // Will be removed
	ws.UpdatedAt = time.Now()

	if err := m.store.Update(ctx, ws); err != nil {
		return fmt.Errorf("failed to update workspace status: %w", err)
	}

	// Now remove worktree (state is already consistent)
	// If this fails, state says retired with no worktree, which is acceptable
	if worktreePath != "" {
		if err := m.worktreeRunner.Remove(ctx, worktreePath, false); err != nil {
			// If worktree is dirty or has other issues, force remove
			// Log both errors for debugging context
			firstErr := err
			if forceErr := m.worktreeRunner.Remove(ctx, worktreePath, true); forceErr != nil {
				// Both attempts failed - state is already updated, log warning
				// Future: integrate with zerolog for warning output
				_ = fmt.Errorf("warning: failed to remove worktree (non-force: %w, force: %w)", firstErr, forceErr)
			}
		}
	}

	return nil
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
