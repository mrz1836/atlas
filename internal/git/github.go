// Package git provides Git operations for ATLAS.
// This file implements the HubRunner for GitHub operations via gh CLI.
package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// errContinuePolling is a sentinel error used internally to signal that polling should continue.
var errContinuePolling = errors.New("continue polling")

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

// CheckResult contains the outcome of a single CI check.
type CheckResult struct {
	// Name is the check name (e.g., "CI / lint").
	Name string
	// State is the raw GitHub state (SUCCESS, FAILURE, PENDING).
	State string
	// Bucket is the categorized state (pass, fail, pending, skipping, cancel).
	Bucket string
	// Conclusion is the check conclusion if completed.
	Conclusion string
	// URL is the link to the check details.
	URL string
	// Duration is how long the check ran.
	Duration time.Duration
	// Workflow is the parent workflow name.
	Workflow string
}

// CIProgressCallback receives progress updates during CI watch.
type CIProgressCallback func(elapsed time.Duration, checks []CheckResult)

// CIWatchOptions configures CI monitoring.
type CIWatchOptions struct {
	// PRNumber is the PR to monitor (required).
	PRNumber int
	// Interval is the polling interval (default: 2 minutes).
	Interval time.Duration
	// Timeout is the maximum time to wait (default: 30 minutes).
	Timeout time.Duration
	// RequiredChecks filters to specific check names.
	// Empty means monitor all checks.
	// Supports wildcards: "CI*" matches "CI / lint", "CI / test"
	RequiredChecks []string
	// ProgressCallback is called after each poll with current status.
	ProgressCallback CIProgressCallback
	// BellEnabled emits terminal bell on status change.
	BellEnabled bool
	// InitialGracePeriod is the time to wait for CI checks to appear after PR creation.
	// During this period, "no checks reported" is treated as expected (keep polling).
	// After this period, if no checks appear, it's treated as "no CI configured" (success).
	// Default: 2 minutes.
	InitialGracePeriod time.Duration
	// GracePollInterval is the polling interval during the initial grace period.
	// This is typically faster than the normal Interval since we're waiting for checks to appear.
	// Default: 10 seconds.
	GracePollInterval time.Duration
}

// CIWatchResult contains the outcome of CI monitoring.
type CIWatchResult struct {
	// Status is the final CI status (Success, Failure, Timeout).
	Status CIStatus
	// CheckResults contains individual check outcomes.
	CheckResults []CheckResult
	// ElapsedTime is total time spent monitoring.
	ElapsedTime time.Duration
	// Error contains details if Status is Failure or Timeout.
	Error error
}

// HubRunner defines operations for GitHub via gh CLI.
// Named HubRunner (not GitHubRunner) to avoid stutter with package name (git.GitHubRunner).
type HubRunner interface {
	// CreatePR creates a pull request and returns the result.
	CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)

	// GetPRStatus gets the current status of a PR.
	GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)

	// WatchPRChecks monitors CI checks until completion or timeout.
	WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error)

	// ConvertToDraft converts an open PR to draft status.
	ConvertToDraft(ctx context.Context, prNumber int) error

	// MergePR merges a pull request using the specified merge method.
	// mergeMethod: "squash", "merge", or "rebase"
	// adminBypass: if true, attempts merge with admin privileges (bypasses branch protection)
	// deleteBranch: if true, deletes the source branch after successful merge
	MergePR(ctx context.Context, prNumber int, mergeMethod string, adminBypass, deleteBranch bool) error

	// AddPRReview adds a review to a pull request.
	// event: "APPROVE", "REQUEST_CHANGES", or "COMMENT"
	AddPRReview(ctx context.Context, prNumber int, body, event string) error

	// AddPRComment adds a comment to a pull request.
	AddPRComment(ctx context.Context, prNumber int, body string) error
}

// Compile-time interface check.
var _ HubRunner = (*CLIGitHubRunner)(nil)

// CLIGitHubRunner implements HubRunner using the gh CLI.
type CLIGitHubRunner struct {
	workDir string
	logger  zerolog.Logger
	config  RetryConfig
	cmdExec CommandExecutor
}

// CommandExecutor executes shell commands. Used for testing.
type CommandExecutor interface {
	// Execute runs a command and returns its combined output.
	Execute(ctx context.Context, workDir, name string, args ...string) ([]byte, error)
}

// CLIGitHubRunnerOption configures a CLIGitHubRunner.
type CLIGitHubRunnerOption func(*CLIGitHubRunner)

// NewCLIGitHubRunner creates a CLIGitHubRunner with the given options.
func NewCLIGitHubRunner(workDir string, opts ...CLIGitHubRunnerOption) *CLIGitHubRunner {
	r := &CLIGitHubRunner{
		workDir: workDir,
		logger:  zerolog.Nop(),
		config:  DefaultRetryConfig(),
		cmdExec: &defaultCommandExecutor{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WithGHLogger sets the logger for GitHub operations.
func WithGHLogger(logger zerolog.Logger) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.logger = logger
	}
}

// WithGHRetryConfig sets custom retry configuration.
func WithGHRetryConfig(config RetryConfig) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.config = config
	}
}

// WithGHCommandExecutor sets a custom command executor (for testing).
func WithGHCommandExecutor(exec CommandExecutor) CLIGitHubRunnerOption {
	return func(r *CLIGitHubRunner) {
		r.cmdExec = exec
	}
}

// CreatePR creates a pull request via gh CLI with retry logic.
func (r *CLIGitHubRunner) CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
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

// defaultCommandExecutor is the default implementation using exec.Command.
// This struct and runGHCommand have 0% unit test coverage by design.
// Unit tests mock the CommandExecutor interface to avoid external dependencies.
// Integration tests (with //go:build integration tag) should cover these paths.
type defaultCommandExecutor struct{}

// Execute runs a command using the standard exec package.
func (e *defaultCommandExecutor) Execute(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
	return runGHCommand(ctx, workDir, name, args...)
}

// runGHCommand executes a gh CLI command and returns its output as bytes.
func runGHCommand(ctx context.Context, workDir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- args are validated
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Include stderr in error for debugging
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%s failed [%s]: %w", name, strings.TrimSpace(stderr.String()), atlaserrors.ErrGitHubOperation)
		}
		return nil, fmt.Errorf("%s failed: %w", name, atlaserrors.ErrGitHubOperation)
	}

	return stdout.Bytes(), nil
}

// Default CI watch configuration values.
const (
	// DefaultCIWatchInterval is the default polling interval (2 minutes).
	DefaultCIWatchInterval = 2 * time.Minute
	// DefaultCIWatchTimeout is the default timeout (30 minutes).
	DefaultCIWatchTimeout = 30 * time.Minute
	// DefaultCIGracePeriod is the default grace period for checks to appear (2 minutes).
	DefaultCIGracePeriod = 2 * time.Minute
	// DefaultCIGracePollInterval is the default polling interval during grace period (10 seconds).
	DefaultCIGracePollInterval = 10 * time.Second
)

// WatchPRChecks monitors CI checks until completion or timeout.
// It implements a grace period for newly created PRs where CI checks may not have started yet.
func (r *CLIGitHubRunner) WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate and apply defaults
	if err := r.initializeCIWatchOptions(&opts); err != nil {
		return nil, err
	}

	result := &CIWatchResult{}
	startTime := time.Now()
	bellEmitted := false

	r.logger.Info().
		Int("pr_number", opts.PRNumber).
		Dur("interval", opts.Interval).
		Dur("timeout", opts.Timeout).
		Dur("grace_period", opts.InitialGracePeriod).
		Strs("required_checks", opts.RequiredChecks).
		Msg("starting CI watch")

	for {
		pollResult, err := r.pollCIStatus(ctx, time.Since(startTime), opts, result, &bellEmitted, startTime)
		if errors.Is(err, errContinuePolling) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return pollResult, nil
	}
}

// ConvertToDraft converts an open PR to draft status.
func (r *CLIGitHubRunner) ConvertToDraft(ctx context.Context, prNumber int) error {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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

// pollCIStatus performs a single CI status poll iteration.
// Returns (result, nil) when done, (nil, errContinuePolling) to continue polling, or (nil, error) on error.
func (r *CLIGitHubRunner) pollCIStatus(
	ctx context.Context,
	elapsed time.Duration,
	opts CIWatchOptions,
	result *CIWatchResult,
	bellEmitted *bool,
	startTime time.Time,
) (*CIWatchResult, error) {
	inGracePeriod := elapsed <= opts.InitialGracePeriod

	// Check timeout
	if timeoutResult := r.checkCITimeout(elapsed, opts.Timeout, result, bellEmitted); timeoutResult != nil {
		return timeoutResult, nil
	}

	// Fetch and process checks
	checks, err := r.fetchPRChecksWithRetry(ctx, opts.PRNumber)
	if err != nil {
		fetchResult, fetchErr := r.handleCIFetchError(ctx, err, inGracePeriod, elapsed, opts, result, bellEmitted)
		if errors.Is(fetchErr, errContinuePolling) {
			return nil, errContinuePolling
		}
		if fetchErr != nil {
			return nil, fetchErr
		}
		return fetchResult, nil
	}

	// Handle no CI configured
	if len(checks) == 0 {
		return r.handleNoCIConfigured(elapsed, result, bellEmitted), nil
	}

	// Process check results and determine status
	if err := r.processCheckResults(checks, opts, result, startTime); err != nil {
		return nil, err
	}

	// Handle terminal states
	terminalResult := r.handleTerminalState(ctx, result, opts, bellEmitted)
	if terminalResult != nil {
		if terminalResult.Error != nil && errors.Is(terminalResult.Error, context.Canceled) {
			return nil, terminalResult.Error
		}
		return terminalResult, nil
	}

	return nil, errContinuePolling
}

// initializeCIWatchOptions validates options and applies defaults.
func (r *CLIGitHubRunner) initializeCIWatchOptions(opts *CIWatchOptions) error {
	if err := validateCIWatchOptions(opts); err != nil {
		return err
	}

	if opts.Interval == 0 {
		opts.Interval = DefaultCIWatchInterval
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultCIWatchTimeout
	}
	if opts.InitialGracePeriod == 0 {
		opts.InitialGracePeriod = DefaultCIGracePeriod
	}
	if opts.GracePollInterval == 0 {
		opts.GracePollInterval = DefaultCIGracePollInterval
	}
	return nil
}

// checkCITimeout checks if the timeout has been exceeded and returns a timeout result if so.
func (r *CLIGitHubRunner) checkCITimeout(
	elapsed, timeout time.Duration,
	result *CIWatchResult,
	bellEmitted *bool,
) *CIWatchResult {
	if elapsed <= timeout {
		return nil
	}

	result.Status = CIStatusTimeout
	result.ElapsedTime = elapsed
	result.Error = atlaserrors.ErrCITimeout
	r.emitBellIfEnabled(true, bellEmitted)
	r.logger.Warn().
		Dur("elapsed", elapsed).
		Dur("timeout", timeout).
		Msg("CI watch timed out")
	return result
}

// handleCIFetchError handles errors from fetching CI checks.
func (r *CLIGitHubRunner) handleCIFetchError(
	ctx context.Context,
	err error,
	inGracePeriod bool,
	elapsed time.Duration,
	opts CIWatchOptions,
	result *CIWatchResult,
	bellEmitted *bool,
) (*CIWatchResult, error) {
	errType := classifyGHError(err)

	if errType == PRErrorNoChecksYet {
		handledResult, shouldContinue, handleErr := r.handleNoChecksError(
			ctx, inGracePeriod, elapsed, opts, result, bellEmitted)
		if handleErr != nil {
			return nil, handleErr
		}
		if shouldContinue {
			return nil, errContinuePolling // Signal to continue loop
		}
		return handledResult, nil
	}

	// For transient errors (network, rate limit, unknown), try fallback verification
	if isTransientCIError(errType) {
		return r.handleTransientCIError(ctx, err, errType, elapsed, opts, result, bellEmitted)
	}

	// For permanent errors, return immediately with proper sentinel error
	return nil, r.mapPermanentCIError(errType, err)
}

// isTransientCIError returns true for error types that should attempt fallback verification.
// PRErrorOther is treated as transient since unknown gh errors may be recoverable.
func isTransientCIError(errType PRErrorType) bool {
	return errType == PRErrorNetwork || errType == PRErrorRateLimit || errType == PRErrorOther
}

// handleTransientCIError attempts fallback verification for transient CI fetch errors.
func (r *CLIGitHubRunner) handleTransientCIError(
	ctx context.Context,
	err error,
	errType PRErrorType,
	elapsed time.Duration,
	opts CIWatchOptions,
	result *CIWatchResult,
	bellEmitted *bool,
) (*CIWatchResult, error) {
	r.logger.Info().
		Err(err).
		Str("error_type", errType.String()).
		Int("pr_number", opts.PRNumber).
		Msg("CI fetch failed, attempting fallback verification via gh pr view")

	fallbackStatus, fallbackErr := r.verifyPRCIStatusFallback(ctx, opts.PRNumber)
	if fallbackErr == nil && fallbackStatus != nil {
		r.logger.Info().
			Str("fallback_status", fallbackStatus.String()).
			Dur("elapsed", elapsed).
			Msg("determined CI status via fallback verification")

		r.finalizeResult(result, *fallbackStatus, elapsed, opts.BellEnabled, bellEmitted)
		if *fallbackStatus == CIStatusFailure {
			result.Error = atlaserrors.ErrCIFailed
		}
		return result, nil
	}

	// Fallback also failed - return fetch error status instead of propagating error
	r.logger.Warn().
		Err(err).
		AnErr("fallback_error", fallbackErr).
		Dur("elapsed", elapsed).
		Msg("CI fetch failed after retries and fallback - returning fetch error status")

	r.finalizeResult(result, CIStatusFetchError, elapsed, opts.BellEnabled, bellEmitted)
	result.Error = fmt.Errorf("CI status fetch failed: %w", err)
	return result, nil
}

// mapPermanentCIError maps permanent error types to appropriate sentinel errors.
func (r *CLIGitHubRunner) mapPermanentCIError(errType PRErrorType, err error) error {
	switch errType {
	case PRErrorAuth:
		return fmt.Errorf("CI fetch failed - authentication error: %w", atlaserrors.ErrGHAuthFailed)
	case PRErrorNotFound:
		return fmt.Errorf("CI fetch failed - PR not found: %w", atlaserrors.ErrPRNotFound)
	case PRErrorNone, PRErrorRateLimit, PRErrorNetwork, PRErrorNoChecksYet, PRErrorOther:
		// For any other error type (including those that should have been handled earlier),
		// return the original error
		return err
	default:
		// Fallback for any future error types
		return err
	}
}

// finalizeResult sets the common result fields and emits bell if enabled.
func (r *CLIGitHubRunner) finalizeResult(
	result *CIWatchResult,
	status CIStatus,
	elapsed time.Duration,
	bellEnabled bool,
	bellEmitted *bool,
) {
	result.Status = status
	result.ElapsedTime = elapsed
	r.emitBellIfEnabled(bellEnabled, bellEmitted)
}

// verifyPRCIStatusFallback attempts to determine CI status via gh pr view
// when gh pr checks fails. This is a fallback mechanism for transient errors.
// Returns an error if status cannot be determined.
func (r *CLIGitHubRunner) verifyPRCIStatusFallback(ctx context.Context, prNumber int) (*CIStatus, error) {
	prStatus, err := r.GetPRStatus(ctx, prNumber)
	if err != nil {
		return nil, err
	}

	// Convert PRStatus.CIStatus string to our CIStatus enum
	switch strings.ToLower(prStatus.CIStatus) {
	case "success":
		status := CIStatusSuccess
		return &status, nil
	case "failure":
		status := CIStatusFailure
		return &status, nil
	case "pending":
		status := CIStatusPending
		return &status, nil
	}

	// Unable to determine status from the response
	return nil, fmt.Errorf("unable to determine CI status from PR: %w", atlaserrors.ErrCIFetchFailed)
}

// handleNoCIConfigured handles the case when no CI checks are configured.
func (r *CLIGitHubRunner) handleNoCIConfigured(
	elapsed time.Duration,
	result *CIWatchResult,
	bellEmitted *bool,
) *CIWatchResult {
	r.logger.Info().
		Dur("elapsed", elapsed).
		Msg("no CI checks configured - treating as success")
	result.Status = CIStatusSuccess
	result.ElapsedTime = elapsed
	r.emitBellIfEnabled(true, bellEmitted)
	return result
}

// handleTerminalState handles terminal CI states (success/failure/timeout).
func (r *CLIGitHubRunner) handleTerminalState(
	ctx context.Context,
	result *CIWatchResult,
	opts CIWatchOptions,
	bellEmitted *bool,
) *CIWatchResult {
	switch result.Status {
	case CIStatusSuccess:
		r.emitBellIfEnabled(opts.BellEnabled, bellEmitted)
		r.logger.Info().
			Dur("elapsed", result.ElapsedTime).
			Int("checks_passed", len(result.CheckResults)).
			Msg("CI checks passed")
		return result
	case CIStatusFailure:
		result.Error = atlaserrors.ErrCIFailed
		r.emitBellIfEnabled(opts.BellEnabled, bellEmitted)
		r.logger.Warn().
			Dur("elapsed", result.ElapsedTime).
			Msg("CI checks failed")
		return result
	case CIStatusPending:
		// Wait for next poll
		select {
		case <-ctx.Done():
			// Return error result
			return &CIWatchResult{Error: ctx.Err()}
		case <-time.After(opts.Interval):
			// Continue polling
			return nil
		}
	case CIStatusTimeout:
		// This case is handled at the top of the loop, but included for exhaustive switch
		return result
	case CIStatusFetchError:
		// Fetch error - return the result with error set
		return result
	}
	return nil
}

// handleNoChecksError handles the case when no checks are reported yet.
// Returns true if the error was handled and polling should continue.
func (r *CLIGitHubRunner) handleNoChecksError(
	ctx context.Context,
	inGracePeriod bool,
	elapsed time.Duration,
	opts CIWatchOptions,
	result *CIWatchResult,
	bellEmitted *bool,
) (*CIWatchResult, bool, error) {
	if inGracePeriod {
		// During grace period, this is expected - keep polling
		r.logger.Debug().
			Dur("elapsed", elapsed).
			Dur("grace_remaining", opts.InitialGracePeriod-elapsed).
			Msg("CI checks not yet registered, waiting during grace period")

		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(opts.GracePollInterval):
			return nil, true, nil // Continue polling
		}
	}

	// After grace period, no checks means no CI is configured
	r.logger.Info().
		Dur("elapsed", elapsed).
		Msg("grace period ended with no CI checks - treating as no CI configured")
	result.Status = CIStatusSuccess
	result.ElapsedTime = elapsed
	r.emitBellIfEnabled(opts.BellEnabled, bellEmitted)
	return result, false, nil
}

// processCheckResults processes the fetched check results and determines status.
func (r *CLIGitHubRunner) processCheckResults(
	checks []CheckResult,
	opts CIWatchOptions,
	result *CIWatchResult,
	startTime time.Time,
) error {
	// Filter to required checks if specified
	filteredChecks := filterChecks(checks, opts.RequiredChecks)
	result.CheckResults = filteredChecks

	// Validate that required checks were found (if specified)
	if len(opts.RequiredChecks) > 0 && len(filteredChecks) == 0 && len(checks) > 0 {
		// Required checks were specified but none matched - this is an error
		return fmt.Errorf("no checks matched required patterns %v: %w",
			opts.RequiredChecks, atlaserrors.ErrCICheckNotFound)
	}

	// Determine overall status
	status := determineOverallCIStatus(filteredChecks)
	result.Status = status
	result.ElapsedTime = time.Since(startTime)

	// Call progress callback
	if opts.ProgressCallback != nil {
		opts.ProgressCallback(result.ElapsedTime, filteredChecks)
	}

	// Log poll completion with formatted elapsed time
	elapsedStr := formatDuration(result.ElapsedTime)
	startTimeStr := startTime.Format("3:04PM")
	r.logger.Debug().
		Str("status", status.String()).
		Str("elapsed", elapsedStr).
		Str("started", startTimeStr).
		Int("check_count", len(filteredChecks)).
		Msgf("CI poll completed: %s status (started %s, %s elapsed)",
			status.String(), startTimeStr, elapsedStr)

	return nil
}

// prAttemptResult holds the result of a single PR creation attempt.
type prAttemptResult struct {
	success bool
	number  int
	url     string
	errType PRErrorType
	err     error
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
	result := &PRResult{}
	delay := r.config.InitialDelay

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		attemptResult := r.attemptPRCreate(ctx, opts, attempt)
		if attemptResult.success {
			return buildPRSuccessResult(result, attemptResult, opts), nil
		}

		result.ErrorType = attemptResult.errType
		result.FinalErr = attemptResult.err

		// Check if we should stop retrying
		if !shouldRetryPR(attemptResult.errType) {
			break
		}

		// Wait before retrying (unless this is the last attempt)
		if attempt < r.config.MaxAttempts {
			if err := r.waitForPRRetry(ctx, &delay, attempt); err != nil {
				return nil, err
			}
		}
	}

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

// waitForPRRetry waits before the next retry attempt.
func (r *CLIGitHubRunner) waitForPRRetry(ctx context.Context, delay *time.Duration, attempt int) error {
	r.logger.Info().
		Int("next_attempt", attempt+1).
		Dur("delay", *delay).
		Msg("retrying PR creation")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(*delay):
	}

	// Increase delay for next attempt
	*delay = time.Duration(float64(*delay) * r.config.Multiplier)
	if *delay > r.config.MaxDelay {
		*delay = r.config.MaxDelay
	}

	return nil
}

// validateCIWatchOptions validates CI watch options.
func validateCIWatchOptions(opts *CIWatchOptions) error {
	if opts.PRNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", opts.PRNumber, atlaserrors.ErrEmptyValue)
	}
	return nil
}

// addJitter adds random jitter to a duration to prevent synchronized retry storms.
// The factor determines the jitter range: 0.2 means +/- 20% of the base duration.
func addJitter(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return d
	}
	// Generate random value between -factor and +factor
	jitterRatio := (rand.Float64()*2 - 1) * factor //nolint:gosec // Non-cryptographic use for jitter
	jitter := time.Duration(float64(d) * jitterRatio)
	return d + jitter
}

// fetchPRChecksWithRetry fetches PR checks with retry logic for transient failures.
func (r *CLIGitHubRunner) fetchPRChecksWithRetry(ctx context.Context, prNumber int) ([]CheckResult, error) {
	var checks []CheckResult
	var lastErr error
	delay := r.config.InitialDelay

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		checks, lastErr = r.fetchPRChecks(ctx, prNumber)
		if lastErr == nil {
			return checks, nil
		}

		errType := classifyGHError(lastErr)
		if !shouldRetryPR(errType) {
			return nil, lastErr
		}

		// Add jitter to delay to prevent synchronized retries
		jitteredDelay := addJitter(delay, 0.2) // +/- 20% jitter

		r.logger.Warn().
			Err(lastErr).
			Int("attempt", attempt).
			Int("max_attempts", r.config.MaxAttempts).
			Dur("next_delay", jitteredDelay).
			Msg("PR checks fetch failed, retrying")

		if attempt < r.config.MaxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(jitteredDelay):
			}
			delay = time.Duration(float64(delay) * r.config.Multiplier)
			if delay > r.config.MaxDelay {
				delay = r.config.MaxDelay
			}
		}
	}

	return nil, fmt.Errorf("failed to fetch PR checks after %d attempts: %w", r.config.MaxAttempts, lastErr)
}

// ghPRChecksEntry represents a single check from gh pr checks JSON output.
type ghPRChecksEntry struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Bucket      string `json:"bucket"`
	CompletedAt string `json:"completedAt"`
	StartedAt   string `json:"startedAt"`
	Description string `json:"description"`
	Workflow    string `json:"workflow"`
	Link        string `json:"link"`
}

// fetchPRChecks fetches the current CI check status for a PR.
func (r *CLIGitHubRunner) fetchPRChecks(ctx context.Context, prNumber int) ([]CheckResult, error) {
	args := []string{
		"pr", "checks", strconv.Itoa(prNumber),
		"--json", "name,state,bucket,completedAt,startedAt,description,workflow,link",
	}

	output, err := r.cmdExec.Execute(ctx, r.workDir, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR checks: %w", err)
	}

	return parseCheckResults(output)
}

// parseCheckResults parses JSON output from gh pr checks command.
func parseCheckResults(output []byte) ([]CheckResult, error) {
	// Handle empty output (no checks configured)
	if len(bytes.TrimSpace(output)) == 0 {
		return []CheckResult{}, nil
	}

	var entries []ghPRChecksEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse PR checks JSON: %w", err)
	}

	results := make([]CheckResult, 0, len(entries))
	for _, entry := range entries {
		result := CheckResult{
			Name:       entry.Name,
			State:      entry.State,
			Bucket:     entry.Bucket,
			Conclusion: entry.State, // State serves as conclusion in gh CLI output
			URL:        entry.Link,
			Workflow:   entry.Workflow,
			Duration:   calculateCheckDuration(entry.StartedAt, entry.CompletedAt),
		}
		results = append(results, result)
	}

	return results, nil
}

// calculateCheckDuration calculates the duration of a check from timestamps.
func calculateCheckDuration(startedAt, completedAt string) time.Duration {
	if startedAt == "" {
		return 0
	}

	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0
	}

	if completedAt == "" {
		// Still running, calculate from now
		return time.Since(start)
	}

	end, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		return 0
	}

	return end.Sub(start)
}

// filterChecks filters checks by required check names with wildcard support.
func filterChecks(checks []CheckResult, required []string) []CheckResult {
	if len(required) == 0 {
		return checks // No filter, return all
	}

	var filtered []CheckResult
	for _, check := range checks {
		if matchesAnyPattern(check.Name, required) {
			filtered = append(filtered, check)
		}
	}
	return filtered
}

// matchesAnyPattern checks if name matches any of the patterns.
// Supports glob-style wildcards: "CI*" matches "CI / lint"
func matchesAnyPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		// Exact match
		if pattern == name {
			return true
		}
		// Prefix matching for patterns ending in *
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

// determineOverallCIStatus determines the overall CI status from check results.
func determineOverallCIStatus(checks []CheckResult) CIStatus {
	if len(checks) == 0 {
		// No checks configured = success
		return CIStatusSuccess
	}

	hasFailure := false
	hasPending := false

	for _, check := range checks {
		bucket := strings.ToLower(check.Bucket)
		switch bucket {
		case "fail", "cancel":
			hasFailure = true
		case "pass":
			// Success, continue checking others
		case "pending":
			hasPending = true
		case "skipping":
			// Treat skipping as pass (for optional checks)
		default:
			// Unknown bucket, treat as pending
			hasPending = true
		}
	}

	switch {
	case hasFailure:
		return CIStatusFailure
	case hasPending:
		return CIStatusPending
	default:
		return CIStatusSuccess
	}
}

// emitBellIfEnabled emits a terminal bell if enabled and not already emitted.
func (r *CLIGitHubRunner) emitBellIfEnabled(enabled bool, emitted *bool) {
	if enabled && !*emitted {
		_, _ = os.Stdout.Write([]byte("\a")) // BEL character
		*emitted = true
	}
}

// FormatCIProgressMessage generates a human-readable progress message for CI monitoring.
// Format: "Waiting for CI... (5m elapsed, checking: CI, Lint)"
func FormatCIProgressMessage(elapsed time.Duration, checks []CheckResult) string {
	if len(checks) == 0 {
		return fmt.Sprintf("Waiting for CI... (%s elapsed, no checks found)", formatDuration(elapsed))
	}

	// Collect unique check names (prefer workflow name, fallback to check name)
	names := make([]string, 0, len(checks))
	seen := make(map[string]bool)
	for _, check := range checks {
		name := check.Workflow
		if name == "" {
			name = check.Name
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return fmt.Sprintf("Waiting for CI... (%s elapsed, checking: %s)",
		formatDuration(elapsed), strings.Join(names, ", "))
}

// formatDuration formats a duration in a human-friendly way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		// Show minutes and seconds for better granularity
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
