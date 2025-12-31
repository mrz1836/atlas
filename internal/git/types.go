// Package git provides Git operations for ATLAS.
// This file defines types used by the GitRunner.
package git

// Status represents the current state of a Git working tree.
type Status struct {
	Staged    []FileChange // Files staged for commit
	Unstaged  []FileChange // Modified but not staged
	Untracked []string     // Untracked files
	Branch    string       // Current branch name
	Ahead     int          // Commits ahead of upstream
	Behind    int          // Commits behind upstream
}

// FileChange represents a changed file in the working tree.
type FileChange struct {
	Path    string     // File path relative to repo root
	Status  ChangeType // Type of change (Added, Modified, Deleted, etc.)
	OldPath string     // For renamed files, the original path
}

// ChangeType represents the type of change for a file.
type ChangeType string

// Change type constants for git status.
const (
	ChangeAdded    ChangeType = "A"
	ChangeModified ChangeType = "M"
	ChangeDeleted  ChangeType = "D"
	ChangeRenamed  ChangeType = "R"
	ChangeCopied   ChangeType = "C"
	ChangeUnmerged ChangeType = "U"
)

// IsClean returns true if the working tree has no changes.
func (s *Status) IsClean() bool {
	return len(s.Staged) == 0 && len(s.Unstaged) == 0 && len(s.Untracked) == 0
}

// HasStagedChanges returns true if there are staged changes ready to commit.
func (s *Status) HasStagedChanges() bool {
	return len(s.Staged) > 0
}

// HasUnstagedChanges returns true if there are unstaged changes.
func (s *Status) HasUnstagedChanges() bool {
	return len(s.Unstaged) > 0
}

// HasUntrackedFiles returns true if there are untracked files.
func (s *Status) HasUntrackedFiles() bool {
	return len(s.Untracked) > 0
}
