// Package git provides Git operations for ATLAS.
// This file defines the GitRunner interface for git CLI operations.
package git

import "context"

// Runner defines operations for Git repository management.
// All operations run in the specified working directory and use context for cancellation.
type Runner interface {
	// Status returns the current working tree status including staged, unstaged, and untracked files.
	Status(ctx context.Context) (*Status, error)

	// Add stages files for commit. If paths is empty, stages all changes.
	Add(ctx context.Context, paths []string) error

	// Commit creates a commit with the given message.
	Commit(ctx context.Context, message string) error

	// Push pushes commits to the remote repository.
	// If setUpstream is true, sets the upstream tracking reference.
	Push(ctx context.Context, remote, branch string, setUpstream bool) error

	// CurrentBranch returns the name of the currently checked out branch.
	// Returns an error if in detached HEAD state.
	CurrentBranch(ctx context.Context) (string, error)

	// CreateBranch creates a new branch from the specified base and checks it out.
	// If baseBranch is empty, creates from current HEAD.
	// Returns an error if the branch already exists.
	CreateBranch(ctx context.Context, name, baseBranch string) error

	// Diff returns the diff output.
	// If cached is true, shows staged changes; otherwise shows unstaged changes.
	// Deprecated: Use DiffStaged() or DiffUnstaged() for clearer intent.
	Diff(ctx context.Context, cached bool) (string, error)

	// DiffStaged returns the diff of staged (cached) changes.
	DiffStaged(ctx context.Context) (string, error)

	// DiffUnstaged returns the diff of unstaged changes in the working tree.
	DiffUnstaged(ctx context.Context) (string, error)

	// BranchExists checks if a branch exists in the repository.
	BranchExists(ctx context.Context, name string) (bool, error)

	// Fetch downloads objects and refs from a remote repository without merging.
	Fetch(ctx context.Context, remote string) error

	// Rebase replays commits on top of another branch.
	// Returns an error if conflicts occur during rebase.
	Rebase(ctx context.Context, onto string) error

	// RebaseAbort cancels an in-progress rebase operation.
	RebaseAbort(ctx context.Context) error

	// Reset unstages all staged changes (git reset HEAD).
	Reset(ctx context.Context) error
}
