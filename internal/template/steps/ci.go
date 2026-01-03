// Package steps provides step execution implementations for the ATLAS task engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// CIFailureHandlerInterface abstracts CI failure handling to avoid import cycle.
// The concrete implementation is task.CIFailureHandler.
type CIFailureHandlerInterface interface {
	// HasHandler returns true if a failure handler is available.
	HasHandler() bool
}

// CIExecutor handles CI waiting steps by monitoring GitHub Actions.
type CIExecutor struct {
	hubRunner        git.HubRunner
	ciFailureHandler CIFailureHandlerInterface
	logger           zerolog.Logger
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

// WithCIFailureHandlerInterface sets the CI failure handler.
func WithCIFailureHandlerInterface(handler CIFailureHandlerInterface) CIExecutorOption {
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

	// Extract configuration from step
	pollInterval := extractDuration(step.Config, "poll_interval", constants.CIPollInterval)
	timeout := step.Timeout
	if timeout == 0 {
		timeout = constants.DefaultCITimeout
	}
	workflows := extractStringSlice(step.Config, "workflows")
	gracePeriod := extractDuration(step.Config, "grace_period", constants.CIInitialGracePeriod)
	gracePollInterval := extractDuration(step.Config, "grace_poll_interval", constants.CIGracePollInterval)

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
			e.logger.Debug().
				Dur("elapsed", elapsed).
				Int("check_count", len(checks)).
				Msg("CI progress update")
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
	artifactPath := e.saveCIArtifact(result, task, step.Name)

	// Handle result based on status
	switch result.Status {
	case git.CIStatusSuccess:
		return e.handleSuccess(result, task, step, startTime, artifactPath)
	case git.CIStatusFailure:
		return e.handleFailure(result, task, step, startTime, artifactPath)
	case git.CIStatusTimeout:
		return e.handleTimeout(result, task, step, startTime, artifactPath)
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

	// Handle different numeric types
	switch v := prNumber.(type) {
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("invalid PR number %d: %w", v, atlaserrors.ErrEmptyValue)
		}
		return v, nil
	case int64:
		if v <= 0 {
			return 0, fmt.Errorf("invalid PR number %d: %w", v, atlaserrors.ErrEmptyValue)
		}
		return int(v), nil
	case float64:
		if v <= 0 {
			return 0, fmt.Errorf("invalid PR number %v: %w", v, atlaserrors.ErrEmptyValue)
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("pr_number must be a number, got %T: %w", prNumber, atlaserrors.ErrEmptyValue)
	}
}

// handleSuccess returns a completed StepResult for successful CI.
func (e *CIExecutor) handleSuccess(result *git.CIWatchResult, t *domain.Task, step *domain.StepDefinition, startTime time.Time, artifactPath string) (*domain.StepResult, error) {
	completedAt := time.Now()
	e.logger.Info().
		Dur("elapsed", result.ElapsedTime).
		Int("checks_passed", len(result.CheckResults)).
		Msg("CI checks passed")

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
	e.logger.Warn().
		Dur("elapsed", result.ElapsedTime).
		Msg("CI checks failed")

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

// saveCIArtifact saves the CI result to ci-result.json.
func (e *CIExecutor) saveCIArtifact(result *git.CIWatchResult, t *domain.Task, stepName string) string {
	// Determine artifact directory
	artifactDir := filepath.Join(constants.ArtifactsDir, t.ID, stepName)
	if t.Metadata != nil {
		if dir, ok := t.Metadata["artifact_dir"].(string); ok && dir != "" {
			artifactDir = filepath.Join(dir, stepName)
		}
	}

	if err := os.MkdirAll(artifactDir, 0o750); err != nil {
		e.logger.Warn().Err(err).Msg("failed to create artifact directory")
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

	artifactPath := filepath.Join(artifactDir, "ci-result.json")
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		e.logger.Warn().Err(err).Msg("failed to marshal CI artifact")
		return ""
	}

	if err := os.WriteFile(artifactPath, data, 0o600); err != nil {
		e.logger.Warn().Err(err).Msg("failed to write CI artifact")
		return ""
	}

	e.logger.Info().
		Str("path", artifactPath).
		Str("status", artifact.Status).
		Int("failed_checks", len(failedChecks)).
		Msg("saved CI result artifact")

	return artifactPath
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
