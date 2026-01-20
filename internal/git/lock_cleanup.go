// Package git provides Git operations for ATLAS.
// This file implements lock file cleanup utilities for handling stale git locks.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	// DefaultLockStalenessThreshold is the default time after which a lock file
	// is considered stale and safe to remove. Normal git operations complete in <5s.
	DefaultLockStalenessThreshold = 60 * time.Second
)

// ErrLockNotStale indicates a lock file is not old enough to be considered stale.
var ErrLockNotStale = errors.New("lock file is not stale")

// ErrInvalidGitdirFormat indicates a .git file has an invalid format.
var ErrInvalidGitdirFormat = errors.New("invalid gitdir file format")

// resolveGitDir resolves the actual git directory path.
// In a normal repo, path points to a .git directory and is returned as-is.
// In a worktree, path points to a .git file containing "gitdir: /path/to/actual/gitdir"
// and this function returns the resolved path.
func resolveGitDir(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	// If it's a directory, return as-is
	if info.IsDir() {
		return path, nil
	}

	// It's a file - read the gitdir reference (worktree case)
	// #nosec G304 -- path is from .git directory which is trusted
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Parse "gitdir: /path/to/.git/worktrees/name"
	line := strings.TrimSpace(string(content))
	if strings.HasPrefix(line, "gitdir: ") {
		return strings.TrimPrefix(line, "gitdir: "), nil
	}

	return "", fmt.Errorf("%w: %s", ErrInvalidGitdirFormat, path)
}

// DetectStaleLockFile checks if a lock file is stale (safe to remove).
// A lock file is considered stale if it exists and its modification time is
// older than the specified threshold.
//
// Returns:
//   - true, nil if the file exists and is stale
//   - false, nil if the file doesn't exist or is not stale
//   - false, error if there was an error checking the file
func DetectStaleLockFile(lockPath string, threshold time.Duration) (bool, error) {
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, not stale
			return false, nil
		}
		return false, fmt.Errorf("failed to stat lock file %s: %w", lockPath, err)
	}

	// Check if the file is older than the threshold
	age := time.Since(info.ModTime())
	return age > threshold, nil
}

// RemoveStaleLockFile safely removes a stale lock file with logging.
// It first checks if the file is stale before removing it.
// Returns an error if the file is not stale or if removal fails.
func RemoveStaleLockFile(ctx context.Context, lockPath string, threshold time.Duration, logger zerolog.Logger) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if the file exists first
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, nothing to remove (success)
			return nil
		}
		return fmt.Errorf("failed to stat lock file %s: %w", lockPath, err)
	}

	// Check if the lock file is stale
	age := time.Since(info.ModTime())
	if age <= threshold {
		return fmt.Errorf("%w: %s (age: %s, threshold: %s)", ErrLockNotStale, lockPath, age, threshold)
	}

	// Remove the stale lock file
	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) {
			// File was removed between check and removal
			return nil
		}
		return fmt.Errorf("failed to remove stale lock file %s: %w", lockPath, err)
	}

	logger.Warn().
		Str("path", lockPath).
		Dur("age", age).
		Msg("removed stale git lock file")

	return nil
}

// CleanupStaleLockFiles removes all stale lock files in a git directory.
// It checks common lock file locations:
//   - .git/index.lock
//   - .git/worktrees/<name>/index.lock (for worktrees)
//   - .git/refs/heads/*.lock (branch locks)
//
// This function is safe to call repeatedly and will skip files that don't exist
// or are not stale. Errors for individual files are logged but don't stop the
// cleanup process.
func CleanupStaleLockFiles(ctx context.Context, gitDir string, threshold time.Duration, logger zerolog.Logger) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Resolve the actual git directory (handles worktrees where .git is a file)
	resolvedDir, err := resolveGitDir(gitDir)
	if err != nil {
		// If we can't resolve the git dir (e.g., doesn't exist), just return nil
		// since there's nothing to clean up
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to resolve git directory %s: %w", gitDir, err)
	}
	gitDir = resolvedDir

	// List of lock file paths to check
	lockPaths := []string{
		filepath.Join(gitDir, "index.lock"),
	}

	// Check for worktree index locks
	worktreesDir := filepath.Join(gitDir, "worktrees")
	if entries, err := os.ReadDir(worktreesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				lockPath := filepath.Join(worktreesDir, entry.Name(), "index.lock")
				lockPaths = append(lockPaths, lockPath)
			}
		}
	}

	// Check for ref locks in refs/heads/
	refsHeadsDir := filepath.Join(gitDir, "refs", "heads")
	if entries, err := os.ReadDir(refsHeadsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".lock" {
				lockPath := filepath.Join(refsHeadsDir, entry.Name())
				lockPaths = append(lockPaths, lockPath)
			}
		}
	}

	// Attempt to remove each stale lock file
	var lastErr error
	for _, lockPath := range lockPaths {
		if err := RemoveStaleLockFile(ctx, lockPath, threshold, logger); err != nil {
			// Log the error but continue with other files
			logger.Debug().
				Err(err).
				Str("path", lockPath).
				Msg("failed to remove lock file (continuing)")
			lastErr = err
		}
	}

	return lastErr
}
