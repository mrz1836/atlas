// Package workflow provides workflow orchestration for ATLAS task execution.
package workflow

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/workspace"
)

// Initializer handles workspace and task initialization.
type Initializer struct {
	logger zerolog.Logger
}

// NewInitializer creates a new Initializer.
func NewInitializer(logger zerolog.Logger) *Initializer {
	return &Initializer{logger: logger}
}

// WorkspaceOptions contains options for workspace creation.
type WorkspaceOptions struct {
	Name          string
	RepoPath      string
	BranchPrefix  string
	BaseBranch    string
	TargetBranch  string
	UseLocal      bool
	NoInteractive bool
	OutputFormat  string
	ErrorHandler  func(wsName string, err error) error
}

// CreateWorkspace creates a new workspace or uses an existing one (upsert behavior).
// If a workspace with the given name already exists and is active/paused, it will be reused.
// If a closed workspace with the same name exists, a new workspace is created.
// Supports two modes:
//   - New branch mode (BranchPrefix set): Creates a new branch from BaseBranch
//   - Existing branch mode (TargetBranch set): Checks out an existing branch
func (i *Initializer) CreateWorkspace(ctx context.Context, opts WorkspaceOptions) (*domain.Workspace, error) {
	// Create workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return nil, opts.ErrorHandler(opts.Name, fmt.Errorf("failed to create workspace store: %w", err))
	}

	// Create worktree runner
	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, opts.RepoPath, i.logger)
	if err != nil {
		return nil, opts.ErrorHandler(opts.Name, fmt.Errorf("failed to create worktree runner: %w", err))
	}

	// Create manager
	wsMgr := workspace.NewManager(wsStore, wtRunner, i.logger)

	// Check if workspace already exists (upsert behavior)
	existingWs, err := wsMgr.Get(ctx, opts.Name)

	// Handle existing active/paused workspace - reuse it
	if err == nil && existingWs != nil && existingWs.Status != constants.WorkspaceStatusClosed {
		i.logger.Info().
			Str("workspace_name", opts.Name).
			Str("worktree_path", existingWs.WorktreePath).
			Str("status", string(existingWs.Status)).
			Msg("using existing workspace")
		return existingWs, nil
	}

	// Handle existing closed workspace - log and create new
	if err == nil && existingWs != nil && existingWs.Status == constants.WorkspaceStatusClosed {
		i.logger.Info().
			Str("workspace_name", opts.Name).
			Msg("workspace is closed, creating new workspace with same name")
	}

	// Build create options based on mode
	createOpts := workspace.CreateOptions{
		Name:     opts.Name,
		RepoPath: opts.RepoPath,
		UseLocal: opts.UseLocal,
	}

	if opts.TargetBranch != "" {
		// Existing branch mode: checkout an existing branch
		createOpts.ExistingBranch = opts.TargetBranch
		i.logger.Info().
			Str("workspace_name", opts.Name).
			Str("target_branch", opts.TargetBranch).
			Msg("creating workspace with existing branch (hotfix mode)")
	} else {
		// New branch mode: create a new branch from base
		createOpts.BranchType = opts.BranchPrefix
		createOpts.BaseBranch = opts.BaseBranch
	}

	// Create new workspace
	ws, err := wsMgr.Create(ctx, createOpts)
	if err != nil {
		return nil, opts.ErrorHandler(opts.Name, fmt.Errorf("failed to create workspace: %w", err))
	}

	return ws, nil
}

// FindGitRepository finds the git repository root from the current directory.
// Uses git rev-parse for accurate detection even in worktrees.
func (i *Initializer) FindGitRepository(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	info, err := git.DetectRepo(ctx, cwd)
	if err != nil {
		return "", atlaserrors.ErrNotGitRepo
	}

	return info.WorktreePath, nil
}

// CleanupWorkspace removes a workspace after a failed task start.
// This calls Destroy() (complete removal), not Close() (archive).
func (i *Initializer) CleanupWorkspace(ctx context.Context, wsName, repoPath string) error {
	i.logger.Debug().
		Str("workspace_name", wsName).
		Str("repo_path", repoPath).
		Msg("cleanupWorkspace called - will call Destroy() (not Close())")

	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return fmt.Errorf("failed to create workspace store: %w", err)
	}

	wtRunner, err := workspace.NewGitWorktreeRunner(ctx, repoPath, i.logger)
	if err != nil {
		return fmt.Errorf("failed to create worktree runner: %w", err)
	}

	mgr := workspace.NewManager(wsStore, wtRunner, i.logger)
	return mgr.Destroy(ctx, wsName)
}

// FindGitRepository is a standalone function for finding the git repository.
// It creates a temporary initializer with a no-op logger.
// This is primarily for testing and backwards compatibility.
func FindGitRepository(ctx context.Context) (string, error) {
	i := NewInitializer(zerolog.Nop())
	return i.FindGitRepository(ctx)
}

// CleanupWorkspace is a standalone function for cleaning up a workspace.
// It creates a temporary initializer with a no-op logger.
// This is primarily for testing and backwards compatibility.
func CleanupWorkspace(ctx context.Context, wsName, repoPath string) error {
	i := NewInitializer(zerolog.Nop())
	return i.CleanupWorkspace(ctx, wsName, repoPath)
}

// CreateWorkspaceSimple is a standalone function for creating a workspace.
// It creates a temporary initializer with a no-op logger and uses a simple
// error passthrough. This is primarily for testing and backwards compatibility.
func CreateWorkspaceSimple(ctx context.Context, name, repoPath, branchPrefix, baseBranch, targetBranch string, useLocal bool) (*domain.Workspace, error) {
	i := NewInitializer(zerolog.Nop())
	opts := WorkspaceOptions{
		Name:         name,
		RepoPath:     repoPath,
		BranchPrefix: branchPrefix,
		BaseBranch:   baseBranch,
		TargetBranch: targetBranch,
		UseLocal:     useLocal,
		ErrorHandler: func(_ string, err error) error { return err },
	}
	return i.CreateWorkspace(ctx, opts)
}
