// Package git provides Git operations for ATLAS.
// This file implements the CLIRunner which wraps git CLI commands.
package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// CLIRunner implements Runner using the git CLI.
type CLIRunner struct {
	workDir string // Working directory for git commands
}

// NewRunner creates a new CLIRunner for the given working directory.
// Returns an error if the directory is not a git repository.
func NewRunner(workDir string) (*CLIRunner, error) {
	if workDir == "" {
		return nil, fmt.Errorf("work directory cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	r := &CLIRunner{workDir: workDir}

	// Verify this is a git repository
	_, err := r.runGitCommand(context.Background(), "rev-parse", "--git-dir")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", atlaserrors.ErrNotGitRepo, err)
	}

	return r, nil
}

// Status returns the current working tree status.
func (r *CLIRunner) Status(ctx context.Context) (*Status, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get porcelain status for parsing
	output, err := r.runGitCommand(ctx, "status", "--porcelain", "-uall", "--branch")
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return parseGitStatus(output), nil
}

// Add stages files for commit.
func (r *CLIRunner) Add(ctx context.Context, paths []string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	args := []string{"add"}
	if len(paths) == 0 {
		// Stage all changes
		args = append(args, "-A")
	} else {
		args = append(args, "--")
		args = append(args, paths...)
	}

	_, err := r.runGitCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to add files: %w", err)
	}

	return nil
}

// Commit creates a commit with the given message and trailers.
func (r *CLIRunner) Commit(ctx context.Context, message string, trailers map[string]string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if message == "" {
		return fmt.Errorf("commit message cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Build commit message with trailers in footer
	fullMsg := message
	if len(trailers) > 0 {
		fullMsg += "\n\n"
		for k, v := range trailers {
			fullMsg += fmt.Sprintf("%s: %s\n", k, v)
		}
	}

	// Use --cleanup=strip to handle formatting (removes trailing whitespace, leading/trailing blank lines)
	_, err := r.runGitCommand(ctx, "commit", "-m", fullMsg, "--cleanup=strip")
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Push pushes commits to the remote repository.
func (r *CLIRunner) Push(ctx context.Context, remote, branch string, setUpstream bool) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	args = append(args, remote, branch)

	_, err := r.runGitCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// CurrentBranch returns the name of the currently checked out branch.
func (r *CLIRunner) CurrentBranch(ctx context.Context) (string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	output, err := r.runGitCommand(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	// Handle detached HEAD state
	if output == "HEAD" {
		return "", fmt.Errorf("repository is in detached HEAD state: %w", atlaserrors.ErrGitOperation)
	}

	return output, nil
}

// CreateBranch creates a new branch and checks it out.
func (r *CLIRunner) CreateBranch(ctx context.Context, name string) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if name == "" {
		return fmt.Errorf("branch name cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Check if branch already exists
	exists, err := r.branchExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("branch '%s' already exists: %w", name, atlaserrors.ErrBranchExists)
	}

	// Create and checkout the branch
	_, err = r.runGitCommand(ctx, "checkout", "-b", name)
	if err != nil {
		return fmt.Errorf("failed to create branch '%s': %w", name, err)
	}

	return nil
}

// Diff returns the diff output.
func (r *CLIRunner) Diff(ctx context.Context, cached bool) (string, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	args := []string{"diff"}
	if cached {
		args = append(args, "--cached")
	}

	output, err := r.runGitCommand(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	return output, nil
}

// branchExists checks if a branch exists in the repository.
func (r *CLIRunner) branchExists(ctx context.Context, name string) (bool, error) {
	_, err := r.runGitCommand(ctx, "show-ref", "--verify", "refs/heads/"+name)
	if err != nil {
		// Exit code 1 or "not a valid ref" means ref not found, which is expected
		errStr := err.Error()
		if strings.Contains(errStr, "exit status 1") || strings.Contains(errStr, "not a valid ref") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check branch existence: %w", err)
	}
	return true, nil
}

// runGitCommand executes a git command and returns its output.
// This is a convenience wrapper around RunCommand that uses the runner's workDir.
func (r *CLIRunner) runGitCommand(ctx context.Context, args ...string) (string, error) {
	return RunCommand(ctx, r.workDir, args...)
}

// parseGitStatus parses git status --porcelain --branch output.
func parseGitStatus(output string) *Status {
	status := &Status{
		Staged:    []FileChange{},
		Unstaged:  []FileChange{},
		Untracked: []string{},
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}

		// Parse branch line: ## branch...origin/branch [ahead N, behind M]
		if strings.HasPrefix(line, "## ") {
			parseBranchLine(line, status)
			continue
		}

		// Parse file status lines
		// XY PATH or XY ORIG -> PATH (for renames)
		indexStatus := line[0]
		workTreeStatus := line[1]
		path := strings.TrimSpace(line[3:])

		// Handle renames: XY ORIG -> DEST
		var oldPath string
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			oldPath = parts[0]
			path = parts[1]
		}

		// Untracked files
		if indexStatus == '?' && workTreeStatus == '?' {
			status.Untracked = append(status.Untracked, path)
			continue
		}

		// Staged changes (index status)
		if indexStatus != ' ' && indexStatus != '?' {
			status.Staged = append(status.Staged, FileChange{
				Path:    path,
				Status:  ChangeType(string(indexStatus)),
				OldPath: oldPath,
			})
		}

		// Unstaged changes (work tree status)
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			status.Unstaged = append(status.Unstaged, FileChange{
				Path:    path,
				Status:  ChangeType(string(workTreeStatus)),
				OldPath: oldPath,
			})
		}
	}

	return status
}

// parseBranchLine parses the branch line from git status --porcelain --branch.
// Format: ## branch...origin/branch [ahead N, behind M]
func parseBranchLine(line string, status *Status) {
	// Remove "## " prefix
	line = strings.TrimPrefix(line, "## ")

	// Split on "..." to separate local and remote
	parts := strings.SplitN(line, "...", 2)
	status.Branch = parts[0]

	if len(parts) < 2 {
		return
	}

	// Parse remote and ahead/behind info
	remotePart := parts[1]

	// Look for [ahead N, behind M] or [ahead N] or [behind M]
	bracketStart := strings.Index(remotePart, " [")
	if bracketStart == -1 {
		return
	}

	// Verify string ends with "]" and has enough length for slice
	if len(remotePart) < bracketStart+4 || remotePart[len(remotePart)-1] != ']' {
		return
	}

	info := remotePart[bracketStart+2 : len(remotePart)-1] // Remove " [" and "]"
	status.Ahead = parseAheadBehind(info, "ahead ")
	status.Behind = parseAheadBehind(info, "behind ")
}

// parseAheadBehind extracts the count from "ahead N" or "behind N" in the info string.
func parseAheadBehind(info, prefix string) int {
	idx := strings.Index(info, prefix)
	if idx == -1 {
		return 0
	}

	numStr := info[idx+len(prefix):]
	if commaIdx := strings.Index(numStr, ","); commaIdx != -1 {
		numStr = numStr[:commaIdx]
	}

	n, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil {
		return 0
	}
	return n
}
