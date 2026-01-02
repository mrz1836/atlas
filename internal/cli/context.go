// Package cli provides the command-line interface for atlas.
// This file provides execution context resolution for worktree support.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/git"
)

// ExecutionContext holds resolved paths and config for command execution.
// It enables worktree-aware operation when the --worktree flag is used.
type ExecutionContext struct {
	// WorkDir is the directory to operate on (worktree path or current repo root).
	WorkDir string

	// MainRepoPath is the path to the main repository (for config inheritance).
	MainRepoPath string

	// IsWorktree indicates if WorkDir is a linked worktree.
	IsWorktree bool

	// Config is the merged configuration.
	Config *config.Config
}

// executionContextKey is the context key for ExecutionContext.
type executionContextKey struct{}

// ResolveExecutionContext resolves the execution context from the worktree flag.
// If worktreeName is specified, finds and validates the worktree.
// Otherwise, uses current directory.
//
// Config is loaded with worktree inheritance:
//   - global config (~/.atlas/config.yaml) - lowest precedence
//   - main repo config (<main-repo>/.atlas/config.yaml) - middle precedence
//   - worktree config (<worktree>/.atlas/config.yaml) - highest precedence
func ResolveExecutionContext(ctx context.Context, worktreeName string) (*ExecutionContext, error) {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Detect repository info
	repoInfo, err := git.DetectRepo(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}

	ec := &ExecutionContext{
		MainRepoPath: repoInfo.Root,
	}

	if worktreeName == "" {
		// No worktree specified - operate on current location
		ec.WorkDir = repoInfo.WorktreePath
		ec.IsWorktree = repoInfo.IsWorktree
	} else {
		// Find specified worktree
		var wt *git.WorktreeEntry
		wt, err = git.FindWorktreeByName(ctx, repoInfo.Root, worktreeName)
		if err != nil {
			return nil, fmt.Errorf("worktree '%s' not found: %w", worktreeName, err)
		}
		ec.WorkDir = wt.Path
		ec.IsWorktree = true
	}

	// Load merged config with worktree inheritance
	ec.Config, err = config.LoadWithWorktree(ctx, ec.MainRepoPath, ec.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return ec, nil
}

// WithExecutionContext returns a new context with the ExecutionContext attached.
func WithExecutionContext(ctx context.Context, ec *ExecutionContext) context.Context {
	return context.WithValue(ctx, executionContextKey{}, ec)
}

// GetExecutionContext retrieves the ExecutionContext from the context.
// Returns nil if no execution context was set.
func GetExecutionContext(ctx context.Context) *ExecutionContext {
	ec, _ := ctx.Value(executionContextKey{}).(*ExecutionContext)
	return ec
}

// ProjectConfigPath returns the path to the project config file for the execution context.
// Returns the worktree config path if in a worktree, otherwise the main repo config path.
func (ec *ExecutionContext) ProjectConfigPath() string {
	return filepath.Join(ec.WorkDir, ".atlas", "config.yaml")
}

// MainRepoConfigPath returns the path to the main repository's config file.
func (ec *ExecutionContext) MainRepoConfigPath() string {
	return filepath.Join(ec.MainRepoPath, ".atlas", "config.yaml")
}
