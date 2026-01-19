// Package git provides Git operations for ATLAS.
// This file implements CI monitoring and check watching operations.
package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/ctxutil"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

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

// WatchPRChecks monitors CI checks until completion or timeout.
// It implements a grace period for newly created PRs where CI checks may not have started yet.
func (r *CLIGitHubRunner) WatchPRChecks(ctx context.Context, opts CIWatchOptions) (*CIWatchResult, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
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

// validateCIWatchOptions validates CI watch options.
func validateCIWatchOptions(opts *CIWatchOptions) error {
	if opts.PRNumber <= 0 {
		return fmt.Errorf("invalid PR number %d: %w", opts.PRNumber, atlaserrors.ErrEmptyValue)
	}
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
		// return with context
		return fmt.Errorf("CI fetch failed: %w", err)
	default:
		// Fallback for any future error types
		return fmt.Errorf("CI fetch failed (unknown error type): %w", err)
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
