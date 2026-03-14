// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import "time"

// MergeResult captures the outcome of a PR merge operation.
// Saved as JSON artifact at: <stepName>/merge-result.json
type MergeResult struct {
	// PRNumber is the pull request number that was merged.
	PRNumber int `json:"pr_number"`

	// MergeMethod is the merge strategy used: "squash", "merge", or "rebase".
	MergeMethod string `json:"merge_method"`

	// AdminBypass indicates if admin privileges were used to bypass branch protection.
	AdminBypass bool `json:"admin_bypass"`

	// DeleteBranch indicates if the source branch was deleted after merge.
	DeleteBranch bool `json:"delete_branch"`

	// MergedAt is the timestamp when the merge was completed.
	MergedAt time.Time `json:"merged_at"`
}

// ReviewResult captures the outcome of a PR review operation.
// Saved as JSON artifact at: <stepName>/review-result.json
type ReviewResult struct {
	// PRNumber is the pull request number that was reviewed.
	PRNumber int `json:"pr_number"`

	// Event is the review action: "APPROVE", "REQUEST_CHANGES", or "COMMENT".
	Event string `json:"event"`

	// Body is the review comment text, if any.
	Body string `json:"body,omitempty"`

	// AddedAt is the timestamp when the review was submitted.
	AddedAt time.Time `json:"added_at"`
}

// CommentResult captures the outcome of a PR comment operation.
// Saved as JSON artifact at: <stepName>/comment-result.json
type CommentResult struct {
	// PRNumber is the pull request number that was commented on.
	PRNumber int `json:"pr_number"`

	// Body is the comment text.
	Body string `json:"body"`

	// AddedAt is the timestamp when the comment was added.
	AddedAt time.Time `json:"added_at"`
}

// ReviewEvent constants for PR review actions.
const (
	// ReviewEventApprove approves the pull request.
	ReviewEventApprove = "APPROVE"

	// ReviewEventRequestChanges requests changes to the pull request.
	ReviewEventRequestChanges = "REQUEST_CHANGES"

	// ReviewEventComment adds a comment without approving or requesting changes.
	ReviewEventComment = "COMMENT"
)

// MergeMethod constants for PR merge strategies.
const (
	// MergeMethodSquash combines all commits into one.
	MergeMethodSquash = "squash"

	// MergeMethodMerge creates a merge commit.
	MergeMethodMerge = "merge"

	// MergeMethodRebase rebases commits onto the base branch.
	MergeMethodRebase = "rebase"
)

// ValidMergeMethod returns true if the given method is a valid merge method.
func ValidMergeMethod(method string) bool {
	switch method {
	case MergeMethodSquash, MergeMethodMerge, MergeMethodRebase:
		return true
	default:
		return false
	}
}

// ValidReviewEvent returns true if the given event is a valid review event.
func ValidReviewEvent(event string) bool {
	switch event {
	case ReviewEventApprove, ReviewEventRequestChanges, ReviewEventComment:
		return true
	default:
		return false
	}
}
