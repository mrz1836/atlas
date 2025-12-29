// Package validation provides command execution and result handling for validation pipelines.
package validation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"
)

// GitRunner abstracts git command execution for testability.
type GitRunner interface {
	// Run executes a git command and returns its output.
	Run(ctx context.Context, workDir string, args ...string) (string, error)
}

// DefaultGitRunner implements GitRunner using os/exec.
type DefaultGitRunner struct{}

// Run executes a git command and returns its combined output.
func (r *DefaultGitRunner) Run(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// StageModifiedFiles stages any files modified during validation (e.g., by pre-commit auto-fixes).
// This ensures that auto-formatted files are ready for the subsequent commit step.
//
// Returns nil if no files need staging or staging succeeds.
// Returns error if git operations fail.
func StageModifiedFiles(ctx context.Context, workDir string) error {
	return StageModifiedFilesWithRunner(ctx, workDir, &DefaultGitRunner{})
}

// StageModifiedFilesWithRunner is the testable version of StageModifiedFiles.
func StageModifiedFilesWithRunner(ctx context.Context, workDir string, runner GitRunner) error {
	log := zerolog.Ctx(ctx)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check for modified files using git status --porcelain
	output, err := runner.Run(ctx, workDir, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	// Parse modified files (lines starting with " M" are modified but unstaged)
	modified := parseModifiedFiles(output)
	if len(modified) == 0 {
		log.Debug().Msg("no modified files to stage")
		return nil // Nothing to stage
	}

	log.Info().
		Int("file_count", len(modified)).
		Strs("files", modified).
		Msg("staging files modified by pre-commit hooks")

	// Stage modified files using git add
	args := append([]string{"add"}, modified...)
	if _, err := runner.Run(ctx, workDir, args...); err != nil {
		return fmt.Errorf("failed to stage modified files: %w", err)
	}

	return nil
}

// parseModifiedFiles extracts files that need staging from git status --porcelain output.
// It handles both modified files and untracked files created by pre-commit hooks.
//
// Git status --porcelain format:
//
//	XY filename
//	- X = index status (staged changes)
//	- Y = worktree status (unstaged changes)
//	- " M" = modified in worktree but not staged
//	- "M " = modified and staged (skip - already staged)
//	- "MM" = modified in both index and worktree
//	- "??" = untracked file (new file, needs staging)
func parseModifiedFiles(statusOutput string) []string {
	var files []string
	for _, line := range strings.Split(statusOutput, "\n") {
		if len(line) < 3 {
			continue
		}

		// Handle untracked files ("?? filename")
		// Pre-commit hooks may create new files that need to be staged
		if strings.HasPrefix(line, "??") {
			filename := strings.TrimSpace(line[3:])
			if filename != "" {
				files = append(files, filename)
			}
			continue
		}

		// " M file.go" = modified but not staged
		// "MM file.go" = modified in both (stage the worktree changes)
		// We want files where Y (worktree) shows 'M' (modified)
		if line[1] == 'M' {
			// Extract filename (everything after "XY ")
			filename := strings.TrimSpace(line[3:])
			if filename != "" {
				files = append(files, filename)
			}
		}
	}
	return files
}
