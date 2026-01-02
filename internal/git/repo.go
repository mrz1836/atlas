// Package git provides Git operations for ATLAS.
// This file provides repository detection utilities with proper worktree support.
package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// RepoInfo contains information about a git repository.
type RepoInfo struct {
	// Root is the absolute path to the main repository root.
	// For worktrees, this is the main repository, not the worktree.
	Root string

	// WorktreePath is the current working tree path.
	// Same as Root if not in a linked worktree.
	WorktreePath string

	// IsWorktree indicates if the current directory is inside a linked worktree.
	IsWorktree bool

	// CommonDir is the path to the shared .git directory.
	CommonDir string
}

// WorktreeEntry contains information about a git worktree.
type WorktreeEntry struct {
	// Path is the absolute path to the worktree directory.
	Path string
	// Branch is the branch name (without refs/heads/ prefix).
	Branch string
	// Head is the HEAD commit SHA.
	Head string
	// IsPrunable indicates if the worktree directory is missing.
	IsPrunable bool
	// IsLocked indicates if the worktree has a lock file.
	IsLocked bool
}

// DetectRepo returns information about the git repository at the given path.
// Uses git rev-parse for accurate detection even in worktrees.
func DetectRepo(ctx context.Context, path string) (*RepoInfo, error) {
	// Get the toplevel (worktree root)
	toplevel, err := RunCommand(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrNotGitRepo, err)
	}

	// Get the git-dir
	gitDir, err := RunCommand(ctx, path, "rev-parse", "--git-dir")
	if err != nil {
		return nil, err
	}

	// Check if this is a linked worktree (git-dir contains "worktrees/")
	isWorktree := strings.Contains(gitDir, "worktrees/") || strings.Contains(gitDir, "worktrees\\")

	info := &RepoInfo{
		WorktreePath: toplevel,
		IsWorktree:   isWorktree,
	}

	if isWorktree {
		// Get the common dir (main repo's .git)
		commonDir, err := RunCommand(ctx, path, "rev-parse", "--git-common-dir")
		if err != nil {
			return nil, err
		}
		// Convert to absolute path if relative
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(path, commonDir)
		}
		commonDir = filepath.Clean(commonDir)
		info.CommonDir = commonDir
		// Main repo root is one level up from common dir (which is the .git directory)
		info.Root = filepath.Dir(commonDir)
	} else {
		info.Root = toplevel
		info.CommonDir = filepath.Join(toplevel, ".git")
	}

	return info, nil
}

// ListWorktrees returns all worktrees associated with the repository at the given path.
func ListWorktrees(ctx context.Context, path string) ([]WorktreeEntry, error) {
	output, err := RunCommand(ctx, path, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	return parseWorktreeListOutput(output), nil
}

// FindWorktreeByName finds a worktree by its directory name or suffix.
// Matches if the worktree directory name equals the name or ends with "-<name>".
func FindWorktreeByName(ctx context.Context, path, name string) (*WorktreeEntry, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: worktree name", atlaserrors.ErrEmptyValue)
	}

	worktrees, err := ListWorktrees(ctx, path)
	if err != nil {
		return nil, err
	}

	for i := range worktrees {
		wt := &worktrees[i]
		baseName := filepath.Base(wt.Path)
		// Match exact name or suffix pattern (e.g., "atlas-auth" matches "auth")
		if baseName == name || strings.HasSuffix(baseName, "-"+name) {
			return wt, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", atlaserrors.ErrWorktreeNotFound, name)
}

// parseWorktreeListOutput parses git worktree list --porcelain output.
func parseWorktreeListOutput(output string) []WorktreeEntry {
	var worktrees []WorktreeEntry
	var current *WorktreeEntry

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			current = &WorktreeEntry{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case strings.HasPrefix(line, "HEAD ") && current != nil:
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch ") && current != nil:
			// refs/heads/feat/auth -> feat/auth
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			current.Branch = branch
		case line == "prunable" && current != nil:
			current.IsPrunable = true
		case strings.HasPrefix(line, "locked") && current != nil:
			current.IsLocked = true
		}
	}

	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees
}
