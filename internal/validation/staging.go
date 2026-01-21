// Package validation provides command execution and result handling for validation pipelines.
package validation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/git"
)

// classifyGitAddError categorizes git add errors for diagnostics.
// Returns a string classification of the error type:
// - "file_not_found": Files don't exist or pathspec didn't match
// - "permission_denied": Access/permission issues
// - "invalid_path": Path is outside repo or invalid
// - "disk_full": No space left on device
// - "unknown": Unrecognized error pattern
func classifyGitAddError(errMsg string) string {
	// Error patterns for common git add failures
	fileNotFoundPatterns := []string{"did not match any files", "pathspec", "no such file or directory"}
	permissionDeniedPatterns := []string{"permission denied", "access denied"}
	invalidPathPatterns := []string{"outside repository", "not a valid", "is beyond a symbolic link"}
	diskFullPatterns := []string{"no space left", "disk full"}

	lower := strings.ToLower(errMsg)

	for _, pattern := range fileNotFoundPatterns {
		if strings.Contains(lower, pattern) {
			return "file_not_found"
		}
	}

	for _, pattern := range permissionDeniedPatterns {
		if strings.Contains(lower, pattern) {
			return "permission_denied"
		}
	}

	for _, pattern := range invalidPathPatterns {
		if strings.Contains(lower, pattern) {
			return "invalid_path"
		}
	}

	for _, pattern := range diskFullPatterns {
		if strings.Contains(lower, pattern) {
			return "disk_full"
		}
	}

	return "unknown"
}

// Git status --porcelain format constants.
// Format: "XY filename" where X=index status, Y=worktree status.
const (
	// gitStatusMinLen is the minimum line length for a valid status entry ("XY " + filename).
	gitStatusMinLen = 3
	// gitStatusPrefixLen is the length of the status prefix ("XY ").
	gitStatusPrefixLen = 3
	// gitUntrackedPrefix indicates an untracked file ("??").
	gitUntrackedPrefix = "??"
	// gitModifiedFlag indicates a modified file in the worktree position.
	gitModifiedFlag = 'M'
	// gitWorktreePos is the position of the worktree status character (0-indexed).
	gitWorktreePos = 1
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
// Uses a two-tier staging strategy for resilience:
//  1. Batch staging: Attempts to stage all files at once (fast path)
//  2. Individual staging: On batch failure, retries files one-by-one (resilient path)
//
// Partial success is treated as success - if some files are staged, returns nil.
//
// Returns error only if:
//   - Git status check fails
//   - Context is canceled
//   - Lock retry is exhausted
//   - No files could be staged despite fallback attempts
func StageModifiedFiles(ctx context.Context, workDir string) error {
	return StageModifiedFilesWithRunner(ctx, workDir, &DefaultGitRunner{})
}

// StageModifiedFilesWithRunner is the testable version of StageModifiedFiles.
// See StageModifiedFiles for behavior documentation.
func StageModifiedFilesWithRunner(ctx context.Context, workDir string, runner GitRunner) error {
	log := zerolog.Ctx(ctx)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check for modified files using git status --porcelain with lock retry
	output, err := git.RunWithLockRetry(ctx, git.DefaultLockRetryConfig(), *log, func(ctx context.Context) (string, error) {
		return runner.Run(ctx, workDir, "status", "--porcelain")
	})
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

	// Stage modified files using git add with lock file retry (batch staging - fast path)
	args := append([]string{"add"}, modified...)
	err = git.RunWithLockRetryVoid(ctx, git.DefaultLockRetryConfig(), *log, func(ctx context.Context) error {
		_, runErr := runner.Run(ctx, workDir, args...)
		return runErr
	})
	if err != nil {
		return handleStagingError(ctx, workDir, runner, modified, err, *log)
	}

	return nil
}

// handleStagingError processes batch staging failures and attempts individual file staging fallback.
func handleStagingError(ctx context.Context, workDir string, runner GitRunner, modified []string, err error, log zerolog.Logger) error {
	// Check if this is a lock file error (already retried, should fail)
	if git.MatchesLockFileError(err.Error()) {
		return fmt.Errorf("failed to stage modified files (lock retry exhausted): %w", err)
	}

	// Non-lock error: classify, log, and try individual staging fallback
	errorType := classifyGitAddError(err.Error())
	log.Warn().
		Str("error_type", errorType).
		Err(err).
		Msg("batch staging failed, attempting individual file staging")

	// Fallback to individual file staging (resilient path)
	succeeded, failed, individualErr := stageFilesIndividually(ctx, workDir, runner, modified, log)

	if individualErr != nil {
		return fmt.Errorf("staging failed (batch + individual): %w", individualErr)
	}

	// Log summary of individual staging results
	log.Info().
		Int("succeeded", len(succeeded)).
		Int("failed", len(failed)).
		Msg("individual file staging complete")

	// Partial success is acceptable - some files staged is better than none
	if len(succeeded) > 0 {
		if len(failed) > 0 {
			log.Warn().
				Strs("succeeded", succeeded).
				Int("failed_count", len(failed)).
				Msg("partial staging success - some files could not be staged")
		}
		return nil
	}

	// Total failure - no files could be staged
	return fmt.Errorf("failed to stage any files (batch failed, all individual attempts failed): %w", err)
}

// stageFilesIndividually attempts to stage files one by one when batch staging fails.
// This fallback strategy provides resilience when batch operations encounter issues like:
// - Files deleted between detection and staging
// - Special characters in filenames
// - Command line length limits
//
// Returns:
//   - succeeded: files successfully staged
//   - failed: map of file -> error message for files that couldn't be staged
//   - error: only if context is canceled or a catastrophic failure occurs
//
// Partial success is acceptable - some files staged is better than none.
func stageFilesIndividually(
	ctx context.Context,
	workDir string,
	runner GitRunner,
	files []string,
	log zerolog.Logger,
) (succeeded []string, failed map[string]string, err error) {
	succeeded = make([]string, 0, len(files))
	failed = make(map[string]string)

	for _, file := range files {
		// Check for context cancellation before each file
		select {
		case <-ctx.Done():
			return succeeded, failed, ctx.Err()
		default:
		}

		// Try to stage this individual file with lock retry
		stageErr := git.RunWithLockRetryVoid(
			ctx,
			git.DefaultLockRetryConfig(),
			log,
			func(ctx context.Context) error {
				_, runErr := runner.Run(ctx, workDir, "add", "--", file)
				return runErr
			},
		)

		if stageErr != nil {
			// Classify and record the failure
			errorType := classifyGitAddError(stageErr.Error())
			log.Warn().
				Str("file", file).
				Str("error_type", errorType).
				Err(stageErr).
				Msg("failed to stage individual file")
			failed[file] = stageErr.Error()
		} else {
			log.Debug().Str("file", file).Msg("successfully staged individual file")
			succeeded = append(succeeded, file)
		}
	}

	return succeeded, failed, nil
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
		if len(line) < gitStatusMinLen {
			continue
		}

		// Handle untracked files ("?? filename")
		// Pre-commit hooks may create new files that need to be staged
		if strings.HasPrefix(line, gitUntrackedPrefix) {
			filename := strings.TrimSpace(line[gitStatusPrefixLen:])
			if filename != "" {
				files = append(files, filename)
			}
			continue
		}

		// " M file.go" = modified but not staged
		// "MM file.go" = modified in both (stage the worktree changes)
		// We want files where Y (worktree) shows 'M' (modified)
		if line[gitWorktreePos] == gitModifiedFlag {
			// Extract filename (everything after "XY ")
			filename := strings.TrimSpace(line[gitStatusPrefixLen:])
			if filename != "" {
				files = append(files, filename)
			}
		}
	}
	return files
}
