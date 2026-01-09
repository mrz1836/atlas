// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// CIFailureHandler abstracts CI failure handling to avoid import cycle.
// The concrete implementation is task.CIFailureHandler.
type CIFailureHandler interface {
	// HasHandler returns true if a failure handler is available.
	HasHandler() bool
}

// CIExecutor handles CI waiting steps by monitoring GitHub Actions.
type CIExecutor struct {
	hubRunner        git.HubRunner
	ciFailureHandler CIFailureHandler
	ciConfig         *config.CIConfig
	logger           zerolog.Logger
	artifactSaver    ArtifactSaver
}

// CIExecutorOption configures CIExecutor.
type CIExecutorOption func(*CIExecutor)

// NewCIExecutor creates a CIExecutor with dependencies.
func NewCIExecutor(opts ...CIExecutorOption) *CIExecutor {
	e := &CIExecutor{
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithCIHubRunner sets the GitHub runner for CI monitoring.
func WithCIHubRunner(runner git.HubRunner) CIExecutorOption {
	return func(e *CIExecutor) {
		e.hubRunner = runner
	}
}

// WithCIFailureHandler sets the CI failure handler.
func WithCIFailureHandler(handler CIFailureHandler) CIExecutorOption {
	return func(e *CIExecutor) {
		e.ciFailureHandler = handler
	}
}

// WithCILogger sets the logger for CI operations.
func WithCILogger(logger zerolog.Logger) CIExecutorOption {
	return func(e *CIExecutor) {
		e.logger = logger
	}
}

// WithCIConfig sets the CI configuration from project config.
func WithCIConfig(cfg *config.CIConfig) CIExecutorOption {
	return func(e *CIExecutor) {
		e.ciConfig = cfg
	}
}

// WithCIArtifactSaver sets the artifact saver for CI results.
func WithCIArtifactSaver(saver ArtifactSaver) CIExecutorOption {
	return func(e *CIExecutor) {
		e.artifactSaver = saver
	}
}

// CheckStateCounts holds the count of checks in each state category.
type CheckStateCounts struct {
	Pending   int // Checks still running
	Completed int // Checks passed or skipped
	Failed    int // Checks failed or canceled
}

// countChecksByState counts checks in each state category.
func countChecksByState(checks []git.CheckResult) CheckStateCounts {
	counts := CheckStateCounts{}
	for _, check := range checks {
		bucket := strings.ToLower(check.Bucket)
		switch bucket {
		case "pending":
			counts.Pending++
		case "pass", "skipping":
			counts.Completed++
		case "fail", "cancel":
			counts.Failed++
		default:
			// Unknown state - treat as pending for safety
			counts.Pending++
		}
	}
	return counts
}

// Execute polls CI status until completion or timeout.
// Configuration from step.Config:
//   - poll_interval: time.Duration (default: 2 minutes)
//   - timeout: time.Duration (default: 30 minutes)
//   - workflows: []string (default: all - filter to specific workflows)
//
// Requires task.Metadata["pr_number"] to be set with the PR number to monitor.
func (e *CIExecutor) Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	startTime := time.Now()
	e.logger.Info().
		Str("task_id", task.ID).
		Str("step_name", step.Name).
		Str("step_type", string(step.Type)).
		Msg("executing ci step")

	// Check if git steps were skipped (no changes to commit)
	// If so, skip CI wait since there's no PR to monitor
	if task.Metadata != nil {
		if skipGit, ok := task.Metadata["skip_git_steps"].(bool); ok && skipGit {
			e.logger.Info().
				Str("task_id", task.ID).
				Str("step_name", step.Name).
				Msg("skipping CI wait - no PR was created (no changes to commit)")
			return &domain.StepResult{
				StepIndex:   task.CurrentStep,
				StepName:    step.Name,
				Status:      constants.StepStatusSkipped,
				StartedAt:   startTime,
				CompletedAt: time.Now(),
				Output:      "Skipped - no PR was created (no changes to commit)",
			}, nil
		}
	}

	// Validate HubRunner dependency
	if e.hubRunner == nil {
		return e.buildErrorResult(task, step, startTime, "CI executor missing HubRunner dependency"),
			fmt.Errorf("CI executor missing HubRunner: %w", atlaserrors.ErrExecutorNotFound)
	}

	// Extract PR number from task metadata (required)
	prNumber, err := e.extractPRNumber(task)
	if err != nil {
		return e.buildErrorResult(task, step, startTime, err.Error()), err
	}

	// Extract configuration with precedence: step.Config > runtime config > constants
	var runtimePollInterval, runtimeGracePeriod time.Duration
	var runtimeTimeout time.Duration
	if e.ciConfig != nil {
		runtimePollInterval = e.ciConfig.PollInterval
		runtimeGracePeriod = e.ciConfig.GracePeriod
		runtimeTimeout = e.ciConfig.Timeout
	}

	e.logger.Debug().
		Dur("runtime_poll_interval", runtimePollInterval).
		Dur("runtime_timeout", runtimeTimeout).
		Dur("runtime_grace_period", runtimeGracePeriod).
		Bool("ciconfig_nil", e.ciConfig == nil).
		Msg("extracted CI runtime configuration from ciConfig")

	pollInterval := e.getConfigDuration("poll_interval", step.Config, runtimePollInterval, constants.CIPollInterval)
	gracePeriod := e.getConfigDuration("grace_period", step.Config, runtimeGracePeriod, constants.CIInitialGracePeriod)
	gracePollInterval := extractDuration(step.Config, "grace_poll_interval", constants.CIGracePollInterval)

	timeout := step.Timeout
	if timeout == 0 {
		timeout = e.getConfigDuration("timeout", step.Config, runtimeTimeout, constants.DefaultCITimeout)
	}
	workflows := extractStringSlice(step.Config, "workflows")

	e.logger.Debug().
		Dur("resolved_poll_interval", pollInterval).
		Dur("resolved_grace_period", gracePeriod).
		Dur("resolved_timeout", timeout).
		Msg("resolved final CI configuration values")

	// Build watch options
	watchOpts := git.CIWatchOptions{
		PRNumber:           prNumber,
		Interval:           pollInterval,
		Timeout:            timeout,
		RequiredChecks:     workflows,
		BellEnabled:        true, // Always enable bell for CI completion
		InitialGracePeriod: gracePeriod,
		GracePollInterval:  gracePollInterval,
		ProgressCallback: func(elapsed time.Duration, checks []git.CheckResult) {
			// Calculate start time for display
			startTime := time.Now().Add(-elapsed).Format("3:04PM")
			elapsedStr := formatDuration(elapsed)

			// Count checks by state
			stateCounts := countChecksByState(checks)

			// Build state summary for message (show non-zero states)
			var stateDetails []string
			if stateCounts.Pending > 0 {
				stateDetails = append(stateDetails, fmt.Sprintf("%d running", stateCounts.Pending))
			}
			if stateCounts.Completed > 0 {
				stateDetails = append(stateDetails, fmt.Sprintf("%d passed", stateCounts.Completed))
			}
			if stateCounts.Failed > 0 {
				stateDetails = append(stateDetails, fmt.Sprintf("%d failed", stateCounts.Failed))
			}

			stateMsg := ""
			if len(stateDetails) > 0 {
				stateMsg = strings.Join(stateDetails, ", ")
			}

			// Simple progress for Info level (always visible)
			e.logger.Info().Msgf("CI: ⏳ %s (%s)", stateMsg, elapsedStr)

			// Detailed progress for Debug level (verbose mode)
			e.logger.Debug().
				Str("elapsed", elapsedStr).
				Str("started", startTime).
				Int("check_count_total", len(checks)).
				Int("check_count_pending", stateCounts.Pending).
				Int("check_count_completed", stateCounts.Completed).
				Int("check_count_failed", stateCounts.Failed).
				Msgf("CI progress: %s elapsed (started %s) - %s",
					elapsedStr, startTime, stateMsg)
		},
	}

	e.logger.Info().
		Int("pr_number", prNumber).
		Dur("poll_interval", pollInterval).
		Dur("timeout", timeout).
		Dur("grace_period", gracePeriod).
		Strs("workflows", workflows).
		Msg("starting CI monitoring")

	// Execute CI monitoring
	result, err := e.hubRunner.WatchPRChecks(ctx, watchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to watch PR checks: %w", err)
	}

	// Save CI result artifact
	artifactPath := e.saveCIArtifact(ctx, result, task, step.Name)

	// Handle result based on status
	switch result.Status {
	case git.CIStatusSuccess:
		return e.handleSuccess(result, task, step, startTime, artifactPath)
	case git.CIStatusFailure:
		return e.handleFailure(result, task, step, startTime, artifactPath)
	case git.CIStatusTimeout:
		return e.handleTimeout(result, task, step, startTime, artifactPath)
	case git.CIStatusFetchError:
		return e.handleFetchError(result, task, step, startTime, artifactPath)
	case git.CIStatusPending:
		// Pending should not be returned as a final status; treat as unexpected
		return e.buildErrorResult(task, step, startTime, fmt.Sprintf("unexpected CI status: %v", result.Status)),
			fmt.Errorf("unexpected CI status %v: %w", result.Status, atlaserrors.ErrCIFailed)
	}

	// Unreachable but needed for exhaustive switch
	return e.buildErrorResult(task, step, startTime, fmt.Sprintf("unexpected CI status: %v", result.Status)),
		fmt.Errorf("unexpected CI status %v: %w", result.Status, atlaserrors.ErrCIFailed)
}

// Type returns the step type this executor handles.
func (e *CIExecutor) Type() domain.StepType {
	return domain.StepTypeCI
}

// extractPRNumber extracts the PR number from task metadata.
func (e *CIExecutor) extractPRNumber(t *domain.Task) (int, error) {
	if t.Metadata == nil {
		return 0, fmt.Errorf("CI wait step requires pr_number in task metadata: %w", atlaserrors.ErrEmptyValue)
	}

	prNumber, ok := t.Metadata["pr_number"]
	if !ok {
		return 0, fmt.Errorf("CI wait step requires pr_number in task metadata: %w", atlaserrors.ErrEmptyValue)
	}

	num, valid := getIntFromAny(prNumber)
	if !valid {
		return 0, fmt.Errorf("pr_number must be a positive number, got %T: %w", prNumber, atlaserrors.ErrEmptyValue)
	}
	return num, nil
}

// handleSuccess returns a completed StepResult for successful CI.
func (e *CIExecutor) handleSuccess(result *git.CIWatchResult, t *domain.Task, step *domain.StepDefinition, startTime time.Time, artifactPath string) (*domain.StepResult, error) {
	completedAt := time.Now()
	e.logger.Info().Msgf("CI: ✓ All %d checks passed (%s)",
		len(result.CheckResults), formatDuration(result.ElapsedTime))

	return &domain.StepResult{
		StepIndex:    t.CurrentStep,
		StepName:     step.Name,
		Status:       "success",
		StartedAt:    startTime,
		CompletedAt:  completedAt,
		DurationMs:   completedAt.Sub(startTime).Milliseconds(),
		Output:       fmt.Sprintf("CI passed in %s (%d checks)", result.ElapsedTime.Round(time.Second), len(result.CheckResults)),
		ArtifactPath: artifactPath,
	}, nil
}

// handleFailure returns appropriate StepResult for CI failure.
func (e *CIExecutor) handleFailure(result *git.CIWatchResult, t *domain.Task, step *domain.StepDefinition, startTime time.Time, artifactPath string) (*domain.StepResult, error) {
	completedAt := time.Now()
	stateCounts := countChecksByState(result.CheckResults)
	e.logger.Warn().Msgf("CI: ✗ %d failed, %d passed (%s)",
		stateCounts.Failed, stateCounts.Completed, formatDuration(result.ElapsedTime))

	// Format failure message
	failureMsg := e.formatCIFailureMessage(result)

	// If no failure handler or handler not available, return simple failure
	if e.ciFailureHandler == nil || !e.ciFailureHandler.HasHandler() {
		return &domain.StepResult{
			StepIndex:    t.CurrentStep,
			StepName:     step.Name,
			Status:       "failed",
			StartedAt:    startTime,
			CompletedAt:  completedAt,
			DurationMs:   completedAt.Sub(startTime).Milliseconds(),
			Output:       failureMsg,
			Error:        "ci checks failed",
			ArtifactPath: artifactPath,
		}, atlaserrors.ErrCIFailed
	}

	// Return awaiting_approval to trigger failure handling menu
	return &domain.StepResult{
		StepIndex:    t.CurrentStep,
		StepName:     step.Name,
		Status:       "awaiting_approval",
		StartedAt:    startTime,
		CompletedAt:  completedAt,
		DurationMs:   completedAt.Sub(startTime).Milliseconds(),
		Output:       failureMsg,
		ArtifactPath: artifactPath,
	}, nil
}

// handleTimeout returns appropriate StepResult for CI timeout.
func (e *CIExecutor) handleTimeout(result *git.CIWatchResult, t *domain.Task, step *domain.StepDefinition, startTime time.Time, artifactPath string) (*domain.StepResult, error) {
	completedAt := time.Now()
	e.logger.Warn().
		Dur("elapsed", result.ElapsedTime).
		Msg("CI monitoring timed out")

	return &domain.StepResult{
		StepIndex:    t.CurrentStep,
		StepName:     step.Name,
		Status:       "awaiting_approval",
		StartedAt:    startTime,
		CompletedAt:  completedAt,
		DurationMs:   completedAt.Sub(startTime).Milliseconds(),
		Output:       fmt.Sprintf("CI monitoring timed out after %s", result.ElapsedTime.Round(time.Second)),
		ArtifactPath: artifactPath,
	}, nil
}

// handleFetchError returns appropriate StepResult when CI status fetch fails.
// This is distinct from CI failure - the CI may have passed, but we couldn't verify.
// Returns awaiting_approval status to allow user to decide how to proceed.
func (e *CIExecutor) handleFetchError(result *git.CIWatchResult, t *domain.Task, step *domain.StepDefinition, startTime time.Time, artifactPath string) (*domain.StepResult, error) {
	completedAt := time.Now()

	errMsg := "Unable to determine CI status"
	if result.Error != nil {
		errMsg = result.Error.Error()
	}

	e.logger.Warn().
		Dur("elapsed", result.ElapsedTime).
		Str("error", errMsg).
		Msg("CI status fetch failed - awaiting user decision")

	return &domain.StepResult{
		StepIndex:    t.CurrentStep,
		StepName:     step.Name,
		Status:       "awaiting_approval",
		StartedAt:    startTime,
		CompletedAt:  completedAt,
		DurationMs:   completedAt.Sub(startTime).Milliseconds(),
		Output:       fmt.Sprintf("Unable to fetch CI status after %s: %s", result.ElapsedTime.Round(time.Second), errMsg),
		ArtifactPath: artifactPath,
		Metadata: map[string]any{
			"failure_type":   "ci_fetch_error",
			"original_error": errMsg,
		},
	}, nil
}

// buildErrorResult builds a failed StepResult for errors.
func (e *CIExecutor) buildErrorResult(t *domain.Task, step *domain.StepDefinition, startTime time.Time, errMsg string) *domain.StepResult {
	completedAt := time.Now()
	return &domain.StepResult{
		StepIndex:   t.CurrentStep,
		StepName:    step.Name,
		Status:      "failed",
		StartedAt:   startTime,
		CompletedAt: completedAt,
		DurationMs:  completedAt.Sub(startTime).Milliseconds(),
		Error:       errMsg,
	}
}

// formatCIFailureMessage formats a human-readable CI failure message.
func (e *CIExecutor) formatCIFailureMessage(result *git.CIWatchResult) string {
	var sb strings.Builder
	sb.WriteString("CI checks failed:\n\n")

	for _, check := range result.CheckResults {
		bucket := strings.ToLower(check.Bucket)
		if bucket == "fail" || bucket == "cancel" {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", check.Name, check.Bucket))
			if check.URL != "" {
				sb.WriteString(fmt.Sprintf("    Logs: %s\n", check.URL))
			}
		}
	}

	return sb.String()
}

// saveCIArtifact saves the CI result to ci-result.json using the artifact saver.
// Returns the artifact filename if saved successfully, empty string otherwise.
func (e *CIExecutor) saveCIArtifact(ctx context.Context, result *git.CIWatchResult, t *domain.Task, stepName string) string {
	// Skip if no artifact saver configured
	if e.artifactSaver == nil {
		e.logger.Debug().Msg("skipping CI artifact save - no artifact saver configured")
		return ""
	}

	artifact := domain.CIResultArtifact{
		Status:      result.Status.String(),
		ElapsedTime: result.ElapsedTime.String(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	if result.Error != nil {
		artifact.ErrorMessage = result.Error.Error()
	}

	// Convert check results
	allChecks := make([]domain.CICheckArtifact, 0, len(result.CheckResults))
	failedChecks := make([]domain.CICheckArtifact, 0)

	for _, check := range result.CheckResults {
		checkArtifact := domain.CICheckArtifact{
			Name:     check.Name,
			State:    check.State,
			Bucket:   check.Bucket,
			URL:      check.URL,
			Workflow: check.Workflow,
		}
		if check.Duration > 0 {
			checkArtifact.Duration = check.Duration.String()
		}

		allChecks = append(allChecks, checkArtifact)

		bucket := strings.ToLower(check.Bucket)
		if bucket == "fail" || bucket == "cancel" {
			failedChecks = append(failedChecks, checkArtifact)
		}
	}

	artifact.AllChecks = allChecks
	artifact.FailedChecks = failedChecks

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to marshal CI artifact")
		return ""
	}

	// Save using artifact saver with step-based subdirectory
	filename := filepath.Join(stepName, "ci-result.json")
	if err := e.artifactSaver.SaveArtifact(ctx, t.WorkspaceID, t.ID, filename, data); err != nil {
		e.logger.Warn().Err(err).Msg("failed to save CI artifact")
		return ""
	}

	e.logger.Info().
		Str("filename", filename).
		Str("status", artifact.Status).
		Int("failed_checks", len(failedChecks)).
		Msg("saved CI result artifact")

	return filename
}

// extractDuration extracts a duration from step config with fallback.
func extractDuration(config map[string]any, key string, defaultVal time.Duration) time.Duration {
	if config == nil {
		return defaultVal
	}

	val, ok := config[key]
	if !ok {
		return defaultVal
	}

	switch v := val.(type) {
	case time.Duration:
		return v
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return defaultVal
		}
		return d
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	default:
		return defaultVal
	}
}

// getConfigDuration extracts duration with precedence: step.Config > runtime config > constants.
// This enables proper configuration override hierarchy for CI timing values.
func (e *CIExecutor) getConfigDuration(key string, stepConfig map[string]any, runtimeConfig, defaultValue time.Duration) time.Duration {
	// 1. Check step.Config (highest priority - template override)
	if val := extractDuration(stepConfig, key, 0); val > 0 {
		return val
	}

	// 2. Check runtime config (from .atlas/config.yaml)
	if runtimeConfig > 0 {
		return runtimeConfig
	}

	// 3. Fall back to constant default
	return defaultValue
}

// extractStringSlice extracts a string slice from step config.
func extractStringSlice(config map[string]any, key string) []string {
	if config == nil {
		return nil
	}

	val, ok := config[key]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// formatDuration formats a time.Duration into a human-readable string.
// Examples: "45s", "2m 18s", "15m 30s"
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		// Under 1 minute: show seconds only
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	// 1 minute or more: show minutes and remaining seconds
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
