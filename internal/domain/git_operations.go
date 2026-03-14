// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

// Git operation constants define the valid operations for git steps.
// These are used in template step configurations to specify which git operation to perform.
const (
	// GitOpCommit creates a standard commit with the staged changes.
	GitOpCommit = "commit"

	// GitOpPush pushes the current branch to the remote repository.
	GitOpPush = "push"

	// GitOpCreatePR creates a pull request for the current branch.
	GitOpCreatePR = "create_pr"

	// GitOpSmartCommit uses AI to analyze changes and create logical commits.
	GitOpSmartCommit = "smart_commit"
)
