// Package git provides Git operations for ATLAS.
// This file implements PR creation and status operations.
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ctxutil"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// PRCreateOptions configures the PR creation operation.
type PRCreateOptions struct {
	// Title is the PR title (required).
	Title string
	// Body is the PR description/body (required).
	Body string
	// BaseBranch is the target branch (default: "main").
	BaseBranch string
	// HeadBranch is the source branch with changes (required).
	HeadBranch string
	// Draft creates the PR as a draft if true.
	Draft bool
}

// PRResult contains the outcome of a PR creation.
type PRResult struct {
	// Number is the PR number.
	Number int
	// URL is the full URL to the PR.
	URL string
	// State is the PR state ("open" or "draft").
	State string
	// ErrorType classifies the error if creation failed.
	ErrorType PRErrorType
	// Attempts is the number of creation attempts made.
	Attempts int
	// FinalErr is the final error if creation failed.
	FinalErr error
}

// PRStatus contains PR and CI check status (for future CI monitoring in Story 6.6).
type PRStatus struct {
	// Number is the PR number.
	Number int
	// State is the PR state (open, closed, merged).
	State string
	// Mergeable indicates if the PR can be merged.
	Mergeable bool
	// ChecksPass indicates if all CI checks have passed.
	ChecksPass bool
	// CIStatus is the overall CI status (pending, success, failure).
	CIStatus string
}

// prAttemptResult holds the result of a single PR creation attempt.
type prAttemptResult struct {
	success bool
	number  int
	url     string
	errType PRErrorType
	err     error
}

// ghPRViewResponse represents the JSON response from gh pr view.
type ghPRViewResponse struct {
	Number            int                  `json:"number"`
	State             string               `json:"state"`
	Mergeable         string               `json:"mergeable"`
	StatusCheckRollup []ghStatusCheckEntry `json:"statusCheckRollup"`
}

// ghStatusCheckEntry represents a single status check in the rollup.
type ghStatusCheckEntry struct {
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// CreatePR creates a pull request via gh CLI with retry logic.
func (r *CLIGitHubRunner) CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	// Validate and normalize options
	if err := validatePROptions(&opts, r.logger); err != nil {
		return nil, err
	}

	// Execute PR creation with retry
	return r.executePRCreateWithRetry(ctx, opts)
}

// GetPRStatus gets the status of an existing PR.
// This is a stub for future CI monitoring (Story 6.6).
func (r *CLIGitHubRunner) GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	if prNumber <= 0 {
		return nil, fmt.Errorf("invalid PR number %d: %w", prNumber, atlaserrors.ErrEmptyValue)
	}

	// Execute gh pr view command
	args := []string{"pr", "view", strconv.Itoa(prNumber), "--json", "number,state,mergeable,statusCheckRollup"}
	output, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		errType := classifyGHError(err)
		if errType == PRErrorNotFound {
			return nil, fmt.Errorf("PR #%d not found: %w", prNumber, atlaserrors.ErrPRNotFound)
		}
		return nil, fmt.Errorf("failed to get PR status: %w", err)
	}

	// Parse JSON output
	return parsePRStatusOutput(output)
}

// validatePROptions validates PR creation options and sets defaults.
func validatePROptions(opts *PRCreateOptions, logger zerolog.Logger) error {
	if opts.Title == "" {
		return fmt.Errorf("PR title cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if opts.Body == "" {
		return fmt.Errorf("PR body cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if opts.HeadBranch == "" {
		return fmt.Errorf("head branch cannot be empty: %w", atlaserrors.ErrEmptyValue)
	}
	if opts.BaseBranch == "" {
		opts.BaseBranch = "main"
		logger.Debug().Msg("using default base branch 'main'")
	}
	return nil
}

// executePRCreateWithRetry executes PR creation with retry logic.
func (r *CLIGitHubRunner) executePRCreateWithRetry(ctx context.Context, opts PRCreateOptions) (*PRResult, error) {
	op := &SimpleRetryOperation[prAttemptResult]{
		AttemptFunc: func(ctx context.Context, attempt int) (prAttemptResult, bool, error) {
			result := r.attemptPRCreate(ctx, opts, attempt)
			return result, result.success, result.err
		},
		ShouldRetryFunc: func(err error) bool {
			errType := classifyGHError(err)
			return shouldRetryPR(errType)
		},
		OnRetryWaitFunc: func(attempt int, delay time.Duration) {
			r.logger.Info().
				Int("next_attempt", attempt+1).
				Dur("delay", delay).
				Msg("retrying PR creation")
		},
	}

	attemptResult, attempts, err := ExecuteWithRetry(ctx, r.config, op, r.logger)

	result := &PRResult{Attempts: attempts}
	if err == nil && attemptResult.success {
		return buildPRSuccessResult(result, attemptResult, opts), nil
	}

	// Handle context cancellation directly without wrapping.
	// Check ctx.Err() to distinguish parent context cancellation from operation timeout:
	// - If ctx.Err() != nil, the parent context was canceled/timed out
	// - If ctx.Err() == nil but err is DeadlineExceeded, the operation itself timed out
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	result.ErrorType = attemptResult.errType
	result.FinalErr = attemptResult.err

	return result, buildPRFinalError(result)
}

// attemptPRCreate performs a single PR creation attempt.
func (r *CLIGitHubRunner) attemptPRCreate(ctx context.Context, opts PRCreateOptions, attempt int) prAttemptResult {
	r.logger.Info().
		Int("attempt", attempt).
		Str("title", opts.Title).
		Str("base", opts.BaseBranch).
		Str("head", opts.HeadBranch).
		Bool("draft", opts.Draft).
		Msg("creating pull request")

	// Build gh pr create command
	args := []string{
		"pr", "create",
		"--title", opts.Title,
		"--body", opts.Body,
		"--base", opts.BaseBranch,
		"--head", opts.HeadBranch,
	}
	if opts.Draft {
		args = append(args, "--draft")
	}

	// Execute gh CLI
	output, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		errType := classifyGHError(err)
		r.logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Str("error_type", errType.String()).
			Msg("PR creation failed")
		return prAttemptResult{success: false, errType: errType, err: err}
	}

	// Parse output to extract PR URL and number
	url, number := parsePRCreateOutput(string(output))
	if url == "" {
		parseErr := fmt.Errorf("failed to parse PR URL from gh output [%s]: %w", string(output), atlaserrors.ErrPRCreationFailed)
		return prAttemptResult{success: false, errType: PRErrorOther, err: parseErr}
	}

	r.logger.Info().
		Int("attempt", attempt).
		Int("pr_number", number).
		Str("pr_url", url).
		Msg("PR created successfully")

	return prAttemptResult{success: true, number: number, url: url}
}

// buildPRSuccessResult builds the success result.
func buildPRSuccessResult(result *PRResult, attemptResult prAttemptResult, opts PRCreateOptions) *PRResult {
	result.Number = attemptResult.number
	result.URL = attemptResult.url
	if opts.Draft {
		result.State = "draft"
	} else {
		result.State = "open"
	}
	return result
}

// parsePRCreateOutput extracts the PR URL and number from gh pr create output.
// gh pr create outputs the PR URL on success: https://github.com/owner/repo/pull/42
func parsePRCreateOutput(output string) (url string, number int) {
	output = strings.TrimSpace(output)
	lines := strings.Split(output, "\n")

	// Look for a URL pattern in the output
	prURLPattern := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)

	for _, line := range lines {
		if match := prURLPattern.FindStringSubmatch(line); match != nil {
			url = strings.TrimSpace(match[0])
			if len(match) > 1 {
				// Regex guarantees \d+ match, but handle error explicitly for safety
				if n, err := strconv.Atoi(match[1]); err == nil {
					number = n
				}
			}
			return url, number
		}
	}

	return "", 0
}

// parsePRStatusOutput parses the JSON output from gh pr view.
func parsePRStatusOutput(output []byte) (*PRStatus, error) {
	var resp ghPRViewResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse PR status JSON: %w", err)
	}

	status := &PRStatus{
		Number:    resp.Number,
		State:     strings.ToLower(resp.State),
		Mergeable: strings.EqualFold(resp.Mergeable, "MERGEABLE"),
	}

	// Determine CI status from statusCheckRollup
	status.CIStatus, status.ChecksPass = determineCIStatus(resp.StatusCheckRollup)

	return status, nil
}

// determineCIStatus analyzes status check entries to determine overall CI status.
func determineCIStatus(checks []ghStatusCheckEntry) (status string, pass bool) {
	if len(checks) == 0 {
		// No checks configured = pass
		return "success", true
	}

	hasFailure := false
	hasPending := false

	for _, check := range checks {
		conclusion := strings.ToUpper(check.Conclusion)
		state := strings.ToUpper(check.State)

		switch conclusion {
		case "FAILURE", "TIMED_OUT", "CANCELED":
			hasFailure = true
		case "SUCCESS":
			// Success, continue checking others
		default:
			// No conclusion yet, check state
			if state == "PENDING" || state == "QUEUED" || state == "IN_PROGRESS" {
				hasPending = true
			}
		}
	}

	switch {
	case hasFailure:
		return "failure", false
	case hasPending:
		return "pending", false
	default:
		return "success", true
	}
}
