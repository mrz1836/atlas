// Package git provides Git operations for ATLAS.
// This file implements the HubRunner for GitHub operations via gh CLI.
package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// PRErrorType classifies GitHub PR operation failures for appropriate handling.
type PRErrorType int

const (
	// PRErrorNone indicates no error occurred.
	PRErrorNone PRErrorType = iota
	// PRErrorAuth indicates authentication failed - don't retry.
	PRErrorAuth
	// PRErrorRateLimit indicates rate limited - retry with backoff.
	PRErrorRateLimit
	// PRErrorNetwork indicates a network issue - retry with backoff.
	PRErrorNetwork
	// PRErrorNotFound indicates resource not found - don't retry.
	PRErrorNotFound
	// PRErrorOther indicates an unknown error - don't retry.
	PRErrorOther
)

// String returns a string representation of the error type.
func (t PRErrorType) String() string {
	switch t {
	case PRErrorNone:
		return "none"
	case PRErrorAuth:
		return "auth"
	case PRErrorRateLimit:
		return "rate_limit"
	case PRErrorNetwork:
		return "network"
	case PRErrorNotFound:
		return "not_found"
	case PRErrorOther:
		return "other"
	}
	return "other"
}

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

// HubRunner defines operations for GitHub via gh CLI.
// Named HubRunner (not GitHubRunner) to avoid stutter with package name (git.GitHubRunner).
type HubRunner interface {
	// CreatePR creates a pull request and returns the result.
	CreatePR(ctx context.Context, opts PRCreateOptions) (*PRResult, error)

	// GetPRStatus gets the current status of a PR (for CI monitoring in Story 6.6).
	GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error)
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

// shouldRetryPR determines if the error type is retryable.
func shouldRetryPR(errType PRErrorType) bool {
	return errType == PRErrorNetwork || errType == PRErrorRateLimit
}

// buildPRFinalError builds the appropriate error based on the error type.
func buildPRFinalError(result *PRResult) error {
	switch result.ErrorType {
	case PRErrorNone:
		return nil
	case PRErrorAuth:
		return fmt.Errorf("authentication failed: %w", atlaserrors.ErrGHAuthFailed)
	case PRErrorRateLimit:
		return fmt.Errorf("rate limited after %d attempts: %w", result.Attempts, atlaserrors.ErrGHRateLimited)
	case PRErrorNetwork:
		return fmt.Errorf("network error after %d attempts: %w", result.Attempts, atlaserrors.ErrPRCreationFailed)
	case PRErrorNotFound:
		return fmt.Errorf("resource not found: %w", atlaserrors.ErrPRCreationFailed)
	case PRErrorOther:
		return fmt.Errorf("failed to create PR: %w", result.FinalErr)
	}
	return fmt.Errorf("failed to create PR: %w", result.FinalErr)
}

// classifyGHError classifies a gh CLI error for retry handling.
func classifyGHError(err error) PRErrorType {
	if err == nil {
		return PRErrorNone
	}

	// Check for context timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return PRErrorNetwork
	}

	errStr := strings.ToLower(err.Error())

	if isGHRateLimitError(errStr) {
		return PRErrorRateLimit
	}

	if isGHAuthError(errStr) {
		return PRErrorAuth
	}

	if isGHNetworkError(errStr) {
		return PRErrorNetwork
	}

	if isGHNotFoundError(errStr) {
		return PRErrorNotFound
	}

	return PRErrorOther
}

// isGHRateLimitError checks if the error indicates a rate limit.
func isGHRateLimitError(errStr string) bool {
	patterns := []string{
		"rate limit exceeded",
		"api rate limit",
		"secondary rate limit",
		"abuse detection",
		"too many requests",
	}
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// isGHAuthError checks if the error indicates an authentication failure.
func isGHAuthError(errStr string) bool {
	patterns := []string{
		"authentication required",
		"bad credentials",
		"not logged into",
		"must be authenticated",
		"gh auth login",
		"invalid token",
		"token expired",
	}
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// isGHNetworkError checks if the error indicates a network issue.
func isGHNetworkError(errStr string) bool {
	patterns := []string{
		"could not resolve host",
		"connection refused",
		"network is unreachable",
		"connection timed out",
		"no route to host",
		"failed to connect",
		"timeout",
	}
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// isGHNotFoundError checks if the error indicates a not found condition.
func isGHNotFoundError(errStr string) bool {
	patterns := []string{
		"not found",
		"no such",
		"repository not found",
		"does not exist",
	}
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
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
				number, _ = strconv.Atoi(match[1])
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
