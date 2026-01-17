// Package git provides Git operations for ATLAS.
// This file implements git hook management for checkpoint creation.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HookType represents the type of git hook.
type HookType string

const (
	// HookPostCommit is the post-commit hook.
	HookPostCommit HookType = "post-commit"
	// HookPostPush is the post-push hook.
	HookPostPush HookType = "post-push"
)

// hookMarker is the identifier we embed in our wrapper scripts.
const hookMarker = "# ATLAS_HOOK_WRAPPER"

// originalSuffix is the suffix used for renamed existing hooks.
const originalSuffix = ".original"

// HookInstaller manages git hook installation for ATLAS checkpoints.
// It uses a wrapper approach that chains existing hooks rather than replacing them.
// For worktrees, it correctly identifies and uses the main repository's hooks directory.
type HookInstaller struct {
	hooksDir string // The actual hooks directory (handles worktrees)
}

// NewGitHookInstaller creates a new hook installer for the given repository or worktree path.
// It automatically detects and uses the correct hooks directory, even for worktrees.
func NewGitHookInstaller(ctx context.Context, repoPath string) (*HookInstaller, error) {
	// Use git rev-parse to get the hooks directory (works for both regular repos and worktrees)
	hooksDir, err := resolveHooksDir(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve hooks directory: %w", err)
	}

	return &HookInstaller{
		hooksDir: hooksDir,
	}, nil
}

// resolveHooksDir finds the hooks directory for a repository, handling worktrees correctly.
func resolveHooksDir(ctx context.Context, repoPath string) (string, error) {
	// Get the common git directory (for worktrees, this is the main repo's .git)
	commonDir, err := RunCommand(ctx, repoPath, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}

	commonDir = strings.TrimSpace(commonDir)

	// If it's a relative path, make it absolute relative to repoPath
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(repoPath, commonDir)
	}

	// Clean the path
	commonDir = filepath.Clean(commonDir)

	return filepath.Join(commonDir, "hooks"), nil
}

// Install installs the ATLAS hook wrapper for the specified hook type.
// If an existing hook exists, it is renamed to {hook}.original and the wrapper
// will chain to it after executing the ATLAS checkpoint logic.
//
// Returns an error if:
// - The hooks directory cannot be created
// - The existing hook cannot be renamed
// - The wrapper cannot be written
func (g *HookInstaller) Install(_ context.Context, hookType HookType, taskID, workspaceID string) error {
	hookPath := g.hookPath(hookType)
	originalPath := hookPath + originalSuffix

	// Create hooks directory if it doesn't exist
	if err := os.MkdirAll(g.hooksDir, 0o750); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Check if we're already installed
	if g.isInstalled(hookType) {
		// Already installed, just update the task ID
		return g.updateWrapper(hookPath, taskID, workspaceID)
	}

	// If existing hook exists (and is not our wrapper), rename it
	if info, err := os.Stat(hookPath); err == nil && info.Mode().IsRegular() {
		if err := os.Rename(hookPath, originalPath); err != nil {
			return fmt.Errorf("failed to preserve existing hook: %w", err)
		}
	}

	// Write our wrapper script
	wrapper := g.generateWrapper(hookType, taskID, workspaceID)
	if err := os.WriteFile(hookPath, []byte(wrapper), 0o600); err != nil {
		return fmt.Errorf("failed to write hook wrapper: %w", err)
	}

	return nil
}

// Uninstall removes the ATLAS hook wrapper and restores the original hook if it exists.
//
// Returns an error if:
// - The hook cannot be removed
// - The original hook cannot be restored
func (g *HookInstaller) Uninstall(_ context.Context, hookType HookType) error {
	hookPath := g.hookPath(hookType)
	originalPath := hookPath + originalSuffix

	// Remove our wrapper
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove hook wrapper: %w", err)
	}

	// Restore original if it exists
	if _, err := os.Stat(originalPath); err == nil {
		if err := os.Rename(originalPath, hookPath); err != nil {
			return fmt.Errorf("failed to restore original hook: %w", err)
		}
	}

	return nil
}

// IsInstalled checks if the ATLAS hook wrapper is currently installed.
func (g *HookInstaller) IsInstalled(_ context.Context, hookType HookType) bool {
	return g.isInstalled(hookType)
}

// isInstalled is the internal implementation of IsInstalled that doesn't require context.
func (g *HookInstaller) isInstalled(hookType HookType) bool {
	hookPath := g.hookPath(hookType)

	// #nosec G304 - hookPath is constructed from validated hookType and controlled hooksDir
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}

	return strings.Contains(string(data), hookMarker)
}

// hookPath returns the full path to the hook file.
func (g *HookInstaller) hookPath(hookType HookType) string {
	return filepath.Join(g.hooksDir, string(hookType))
}

// updateWrapper updates an existing wrapper with new task information.
func (g *HookInstaller) updateWrapper(hookPath, taskID, workspaceID string) error {
	// #nosec G304 - hookPath is constructed from validated hookType and controlled hooksDir
	data, err := os.ReadFile(hookPath)
	if err != nil {
		return fmt.Errorf("failed to read existing wrapper: %w", err)
	}

	// Parse hook type from path
	hookType := HookType(filepath.Base(hookPath))

	// Generate new wrapper with updated IDs
	wrapper := g.generateWrapper(hookType, taskID, workspaceID)

	// Preserve original hook reference if present
	_ = data // Preserve original hook reference - our template handles this

	if err := os.WriteFile(hookPath, []byte(wrapper), 0o600); err != nil {
		return fmt.Errorf("failed to update hook wrapper: %w", err)
	}

	return nil
}

// generateWrapper generates the hook wrapper script content.
func (g *HookInstaller) generateWrapper(hookType HookType, taskID, workspaceID string) string {
	originalPath := g.hookPath(hookType) + originalSuffix

	// Determine checkpoint trigger based on hook type
	trigger := "git_commit"
	if hookType == HookPostPush {
		trigger = "git_push"
	}

	return fmt.Sprintf(`#!/bin/sh
%s
# Installed by ATLAS for checkpoint creation
# Task: %s | Workspace: %s
# DO NOT EDIT - managed by atlas

set -e

# ATLAS checkpoint creation (failure should not block git operations)
if command -v atlas >/dev/null 2>&1; then
    atlas checkpoint --trigger %s 2>/dev/null || true
fi

# Chain to original hook if it exists
if [ -x "%s" ]; then
    exec "%s" "$@"
fi
`, hookMarker, taskID, workspaceID, trigger, originalPath, originalPath)
}
