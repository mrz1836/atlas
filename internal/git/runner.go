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

	// Commit creates a commit with the given message and optional trailers.
	// Trailers are appended to the commit message footer (e.g., ATLAS-Task: taskID).
	Commit(ctx context.Context, message string, trailers map[string]string) error

	// Push pushes commits to the remote repository.
	// If setUpstream is true, sets the upstream tracking reference.
	Push(ctx context.Context, remote, branch string, setUpstream bool) error

	// CurrentBranch returns the name of the currently checked out branch.
	// Returns an error if in detached HEAD state.
	CurrentBranch(ctx context.Context) (string, error)

	// CreateBranch creates a new branch and checks it out.
	// Returns an error if the branch already exists.
	CreateBranch(ctx context.Context, name string) error

	// Diff returns the diff output.
	// If cached is true, shows staged changes; otherwise shows unstaged changes.
	Diff(ctx context.Context, cached bool) (string, error)
}
