// Package git provides Git operations for ATLAS.
// This file implements PR modification operations (draft, merge, review, comment).
package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mrz1836/atlas/internal/ctxutil"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// ConvertToDraft converts an open PR to draft status.
func (r *CLIGitHubRunner) ConvertToDraft(ctx context.Context, prNumber int) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	if prNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", prNumber, atlaserrors.ErrEmptyValue)
	}

	args := []string{"pr", "ready", "--undo", strconv.Itoa(prNumber)}
	_, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		errType := classifyGHError(err)
		switch errType {
		case PRErrorNotFound:
			return fmt.Errorf("PR #%d not found: %w", prNumber, atlaserrors.ErrPRNotFound)
		case PRErrorNone:
			// Shouldn't happen, but handle for exhaustive switch
			return nil
		case PRErrorAuth:
			return fmt.Errorf("failed to convert PR to draft: %w", atlaserrors.ErrGHAuthFailed)
		case PRErrorNoChecksYet:
			// Not applicable for draft conversion, treat as other error
			return fmt.Errorf("failed to convert PR to draft: %w", err)
		case PRErrorRateLimit, PRErrorNetwork, PRErrorOther:
			// Check if already draft or merged (not an error for our use case)
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "already a draft") {
				r.logger.Debug().Int("pr_number", prNumber).Msg("PR already a draft")
				return nil // Already draft, success
			}
			if strings.Contains(errStr, "merged") || strings.Contains(errStr, "closed") {
				// Can't convert merged/closed PR, but this isn't a failure
				r.logger.Warn().Int("pr_number", prNumber).Msg("PR already merged/closed, cannot convert to draft")
				return nil
			}
			return fmt.Errorf("failed to convert PR to draft: %w", err)
		}
	}

	r.logger.Info().Int("pr_number", prNumber).Msg("converted PR to draft")
	return nil
}

// MergePR merges a pull request using the specified merge method.
func (r *CLIGitHubRunner) MergePR(ctx context.Context, prNumber int, mergeMethod string, adminBypass, deleteBranch bool) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	if prNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", prNumber, atlaserrors.ErrEmptyValue)
	}

	args := []string{"pr", "merge", strconv.Itoa(prNumber)}

	// Add merge method flag
	switch mergeMethod {
	case "squash":
		args = append(args, "--squash")
	case "merge":
		args = append(args, "--merge")
	case "rebase":
		args = append(args, "--rebase")
	default:
		args = append(args, "--squash") // Default to squash
	}

	// Add admin bypass if requested
	if adminBypass {
		args = append(args, "--admin")
	}

	// Handle branch deletion - default to keeping branch (workspace close handles deletion)
	if deleteBranch {
		args = append(args, "--delete-branch")
	} else {
		args = append(args, "--delete-branch=false")
	}

	_, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		errType := classifyGHError(err)
		//nolint:exhaustive // Other error types handled by default case
		switch errType {
		case PRErrorNotFound:
			return fmt.Errorf("PR #%d not found: %w", prNumber, atlaserrors.ErrPRNotFound)
		case PRErrorAuth:
			return fmt.Errorf("merge failed - permission denied: %w", atlaserrors.ErrGHAuthFailed)
		default:
			return fmt.Errorf("failed to merge PR: %w", atlaserrors.ErrPRMergeFailed)
		}
	}

	r.logger.Info().
		Int("pr_number", prNumber).
		Str("method", mergeMethod).
		Bool("admin", adminBypass).
		Bool("delete_branch", deleteBranch).
		Msg("PR merged")
	return nil
}

// AddPRReview adds a review to a pull request using gh CLI.
func (r *CLIGitHubRunner) AddPRReview(ctx context.Context, prNumber int, body, event string) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	if prNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", prNumber, atlaserrors.ErrEmptyValue)
	}

	args := []string{"pr", "review", strconv.Itoa(prNumber)}

	// Add event flag
	switch strings.ToUpper(event) {
	case "APPROVE":
		args = append(args, "--approve")
	case "REQUEST_CHANGES":
		args = append(args, "--request-changes")
	case "COMMENT":
		args = append(args, "--comment")
	default:
		args = append(args, "--approve") // Default to approve
	}

	// Add body if provided
	if body != "" {
		args = append(args, "--body", body)
	}

	_, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		// Check if user cannot approve (e.g., own PR)
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "cannot approve") ||
			strings.Contains(errStr, "cannot request changes") ||
			strings.Contains(errStr, "author") ||
			strings.Contains(errStr, "own pull request") {
			return fmt.Errorf("cannot add review: %w", atlaserrors.ErrPRReviewNotAllowed)
		}

		errType := classifyGHError(err)
		//nolint:exhaustive // Other error types handled by default case
		switch errType {
		case PRErrorNotFound:
			return fmt.Errorf("PR #%d not found: %w", prNumber, atlaserrors.ErrPRNotFound)
		case PRErrorAuth:
			return fmt.Errorf("review failed - permission denied: %w", atlaserrors.ErrGHAuthFailed)
		default:
			return fmt.Errorf("failed to add review: %w", err)
		}
	}

	r.logger.Info().Int("pr_number", prNumber).Str("event", event).Msg("PR review added")
	return nil
}

// AddPRComment adds a comment to a pull request using gh CLI.
func (r *CLIGitHubRunner) AddPRComment(ctx context.Context, prNumber int, body string) error {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return err
	}

	if prNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", prNumber, atlaserrors.ErrEmptyValue)
	}
	if body == "" {
		return fmt.Errorf("comment body cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}

	args := []string{"pr", "comment", strconv.Itoa(prNumber), "--body", body}

	_, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		errType := classifyGHError(err)
		//nolint:exhaustive // Other error types handled by default case
		switch errType {
		case PRErrorNotFound:
			return fmt.Errorf("PR #%d not found: %w", prNumber, atlaserrors.ErrPRNotFound)
		case PRErrorAuth:
			return fmt.Errorf("comment failed - permission denied: %w", atlaserrors.ErrGHAuthFailed)
		default:
			return fmt.Errorf("failed to add comment: %w", err)
		}
	}

	r.logger.Info().Int("pr_number", prNumber).Msg("PR comment added")
	return nil
}
