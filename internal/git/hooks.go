// Package git provides Git operations for ATLAS.
// This file implements git hook management for checkpoint creation.
package git

import (
	"context"
	"fmt"
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

// ResolveHooksDir finds the hooks directory for a repository, handling worktrees correctly.
func ResolveHooksDir(ctx context.Context, repoPath string) (string, error) {
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

// GenerateHookScript generates the hook wrapper script content.
// This is used by the 'atlas hook install' command to print instructions.
func GenerateHookScript(hookType HookType, taskID, workspaceID string) string {
	// Determine checkpoint trigger based on hook type
	trigger := "git_commit"
	if hookType == HookPostPush {
		trigger = "git_push"
	}

	// For manual install scenarios, we provide a standalone script that doesn't interfere with existing hooks.
	// Users can chain this with existing hooks if needed.

	return fmt.Sprintf(`#!/bin/sh
%s
# Manual install for ATLAS checkpoint creation
# Task: %s | Workspace: %s

# ATLAS checkpoint creation (failure should not block git operations)
if command -v atlas >/dev/null 2>&1; then
    atlas checkpoint --trigger %s 2>/dev/null || true
fi
`, hookMarker, taskID, workspaceID, trigger)
}
