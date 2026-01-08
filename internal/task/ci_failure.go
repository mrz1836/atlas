// Package task provides task lifecycle management for ATLAS.
//
// This file implements CI failure handling, providing options for users to
// view logs, retry from implement, fix manually, or abandon when CI fails.
//
// Import rules:
//   - CAN import: internal/constants, internal/domain, internal/errors, internal/git, std lib
//   - MUST NOT import: internal/workspace, internal/cli, internal/tui
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/ctxutil"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
)

// CIFailureAction represents user's choice for handling CI failure.
type CIFailureAction int

const (
	// CIFailureViewLogs opens GitHub Actions in browser.
	CIFailureViewLogs CIFailureAction = iota
	// CIFailureRetryImplement retries from implement step with error context.
	CIFailureRetryImplement
	// CIFailureFixManually user fixes in worktree, then resumes.
	CIFailureFixManually
	// CIFailureAbandon ends task, keeps PR as draft.
	CIFailureAbandon
)

// String returns a string representation of the CI failure action.
func (a CIFailureAction) String() string {
	switch a {
	case CIFailureViewLogs:
		return "view_logs"
	case CIFailureRetryImplement:
		return "retry_implement"
	case CIFailureFixManually:
		return "fix_manually"
	case CIFailureAbandon:
		return "abandon"
	default:
		return "unknown"
	}
}

// GHFailureAction represents user's choice for handling GitHub operation failure.
type GHFailureAction int

const (
	// GHFailureRetry retries the failed GitHub operation.
	GHFailureRetry GHFailureAction = iota
	// GHFailureFixAndRetry lets user fix and then retry.
	GHFailureFixAndRetry
	// GHFailureAbandon ends task.
	GHFailureAbandon
)

// String returns a string representation of the GH failure action.
func (a GHFailureAction) String() string {
	switch a {
	case GHFailureRetry:
		return "retry"
	case GHFailureFixAndRetry:
		return "fix_and_retry"
	case GHFailureAbandon:
		return "abandon"
	default:
		return "unknown"
	}
}

// CITimeoutAction represents user's choice for handling CI timeout.
type CITimeoutAction int

const (
	// CITimeoutContinueWaiting continues monitoring with extended timeout.
	CITimeoutContinueWaiting CITimeoutAction = iota
	// CITimeoutRetry retries from implement step.
	CITimeoutRetry
	// CITimeoutFixManually lets user fix manually.
	CITimeoutFixManually
	// CITimeoutAbandon ends task.
	CITimeoutAbandon
)

// String returns a string representation of the CI timeout action.
func (a CITimeoutAction) String() string {
	switch a {
	case CITimeoutContinueWaiting:
		return "continue_waiting"
	case CITimeoutRetry:
		return "retry"
	case CITimeoutFixManually:
		return "fix_manually"
	case CITimeoutAbandon:
		return "abandon"
	default:
		return "unknown"
	}
}

// CIFailureOptions configures CI failure handling.
type CIFailureOptions struct {
	// Action is the user's chosen action.
	Action CIFailureAction
	// PRNumber is the PR with failing CI.
	PRNumber int
	// CIResult is the result from WatchPRChecks.
	CIResult *git.CIWatchResult
	// WorktreePath is the path to the git worktree.
	WorktreePath string
	// WorkspaceName is the workspace identifier.
	WorkspaceName string
	// ArtifactDir is where to save ci-result.json.
	ArtifactDir string
}

// CIFailureResult contains the outcome of CI failure handling.
type CIFailureResult struct {
	// Action that was taken.
	Action CIFailureAction
	// ErrorContext is AI-friendly error description (for retry).
	ErrorContext string
	// NextStep is the step to resume from (for retry/resume).
	NextStep string
	// ArtifactPath is where ci-result.json was saved.
	ArtifactPath string
	// Message is user-facing result message.
	Message string
}

// BrowserOpener is a function that opens a URL in a browser.
// Used for testing.
type BrowserOpener func(url string) error

// CIFailureHandler handles CI failure scenarios.
type CIFailureHandler struct {
	hubRunner     git.HubRunner
	logger        zerolog.Logger
	browserOpener BrowserOpener
}

// CIFailureHandlerOption configures a CIFailureHandler.
type CIFailureHandlerOption func(*CIFailureHandler)

// NewCIFailureHandler creates a CI failure handler.
func NewCIFailureHandler(hubRunner git.HubRunner, opts ...CIFailureHandlerOption) *CIFailureHandler {
	h := &CIFailureHandler{
		hubRunner:     hubRunner,
		logger:        zerolog.Nop(),
		browserOpener: openInBrowser,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// WithCIFailureLogger sets the logger for CI failure handling.
func WithCIFailureLogger(logger zerolog.Logger) CIFailureHandlerOption {
	return func(h *CIFailureHandler) {
		h.logger = logger
	}
}

// WithBrowserOpener sets a custom browser opener (for testing).
func WithBrowserOpener(opener BrowserOpener) CIFailureHandlerOption {
	return func(h *CIFailureHandler) {
		h.browserOpener = opener
	}
}

// HasHandler returns true if the handler is properly configured.
// This is used by steps.CIFailureHandlerInterface to check if
// interactive failure handling is available.
func (h *CIFailureHandler) HasHandler() bool {
	return h != nil && h.hubRunner != nil
}

// HandleCIFailure processes the user's chosen action for CI failure.
// If opts.ArtifactDir is provided and opts.CIResult is available, the CI result
// is automatically saved to ci-result.json before processing the action.
func (h *CIFailureHandler) HandleCIFailure(ctx context.Context, opts CIFailureOptions) (*CIFailureResult, error) {
	// Check for cancellation at entry
	if err := ctxutil.Canceled(ctx); err != nil {
		return nil, err
	}

	h.logger.Info().
		Str("action", opts.Action.String()).
		Int("pr_number", opts.PRNumber).
		Str("workspace", opts.WorkspaceName).
		Msg("handling CI failure")

	// Auto-save CI result artifact if artifact directory is provided (AC6)
	var artifactPath string
	if opts.ArtifactDir != "" && opts.CIResult != nil {
		var err error
		artifactPath, err = h.SaveCIResultArtifact(ctx, opts.CIResult, opts.ArtifactDir)
		if err != nil {
			// Log warning but don't fail - artifact saving is secondary to action handling
			h.logger.Warn().Err(err).Msg("failed to save CI result artifact")
		}
	}

	var result *CIFailureResult
	var err error

	switch opts.Action {
	case CIFailureViewLogs:
		result, err = h.handleViewLogs(ctx, opts)
	case CIFailureRetryImplement:
		result, err = h.handleRetryImplement(ctx, opts)
	case CIFailureFixManually:
		result, err = h.handleFixManually(ctx, opts)
	case CIFailureAbandon:
		result, err = h.handleAbandon(ctx, opts)
	default:
		// This should never happen with properly typed actions
		return nil, fmt.Errorf("unknown CI failure action %d: %w", opts.Action, atlaserrors.ErrEmptyValue)
	}

	// Set artifact path on result if it was saved
	if err == nil && result != nil && artifactPath != "" {
		result.ArtifactPath = artifactPath
	}

	return result, err
}

// SaveCIResultArtifact saves the CI result to ci-result.json.
func (h *CIFailureHandler) SaveCIResultArtifact(ctx context.Context, result *git.CIWatchResult, artifactDir string) (string, error) {
	if err := ctxutil.Canceled(ctx); err != nil {
		return "", err
	}

	if result == nil {
		return "", fmt.Errorf("CI result is nil: %w", atlaserrors.ErrEmptyValue)
	}

	if artifactDir == "" {
		return "", fmt.Errorf("artifact directory is empty: %w", atlaserrors.ErrEmptyValue)
	}

	// Ensure directory exists
	if err := os.MkdirAll(artifactDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create artifact directory: %w", err)
	}

	artifact := h.buildCIResultArtifact(result)

	artifactPath := filepath.Join(artifactDir, "ci-result.json")
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal CI result artifact: %w", err)
	}

	if err := os.WriteFile(artifactPath, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write CI result artifact: %w", err)
	}

	h.logger.Info().
		Str("path", artifactPath).
		Str("status", artifact.Status).
		Int("failed_checks", len(artifact.FailedChecks)).
		Msg("saved CI result artifact")

	return artifactPath, nil
}

// handleViewLogs opens the GitHub Actions URL in the default browser.
func (h *CIFailureHandler) handleViewLogs(_ context.Context, opts CIFailureOptions) (*CIFailureResult, error) {
	url := h.extractBestCheckURL(opts.CIResult)
	if url == "" {
		return nil, fmt.Errorf("no workflow URL available: %w", atlaserrors.ErrEmptyValue)
	}

	if err := h.browserOpener(url); err != nil {
		return nil, fmt.Errorf("failed to open browser: %w", err)
	}

	h.logger.Info().Str("url", url).Msg("opened CI logs in browser")

	return &CIFailureResult{
		Action:  CIFailureViewLogs,
		Message: fmt.Sprintf("Opened CI logs in browser: %s", url),
	}, nil
}

// handleRetryImplement extracts CI error context for AI retry.
//
//nolint:unparam // error return kept for interface consistency with other handlers
func (h *CIFailureHandler) handleRetryImplement(_ context.Context, opts CIFailureOptions) (*CIFailureResult, error) {
	errorContext := ExtractCIErrorContext(opts.CIResult)

	h.logger.Info().
		Int("pr_number", opts.PRNumber).
		Msg("prepared retry context for AI")

	return &CIFailureResult{
		Action:       CIFailureRetryImplement,
		ErrorContext: errorContext,
		NextStep:     "implement",
		Message:      "Retry from implement with CI error context",
	}, nil
}

// handleFixManually provides instructions for manual fixing.
//
//nolint:unparam // error return kept for interface consistency with other handlers
func (h *CIFailureHandler) handleFixManually(_ context.Context, opts CIFailureOptions) (*CIFailureResult, error) {
	instructions := FormatManualFixInstructions(opts.WorktreePath, opts.WorkspaceName, opts.CIResult)

	h.logger.Info().
		Str("worktree", opts.WorktreePath).
		Str("workspace", opts.WorkspaceName).
		Msg("provided manual fix instructions")

	return &CIFailureResult{
		Action:  CIFailureFixManually,
		Message: instructions,
	}, nil
}

// handleAbandon converts PR to draft and marks task abandoned.
//
//nolint:unparam // error return kept for interface consistency with other handlers
func (h *CIFailureHandler) handleAbandon(ctx context.Context, opts CIFailureOptions) (*CIFailureResult, error) {
	// Attempt to convert PR to draft if we have a PR number and HubRunner
	if opts.PRNumber > 0 && h.hubRunner != nil {
		if err := h.hubRunner.ConvertToDraft(ctx, opts.PRNumber); err != nil {
			// Log warning but don't fail - abandonment should still proceed
			h.logger.Warn().
				Err(err).
				Int("pr_number", opts.PRNumber).
				Msg("could not convert PR to draft, continuing with abandon")
		} else {
			h.logger.Info().
				Int("pr_number", opts.PRNumber).
				Msg("converted PR to draft")
		}
	}

	h.logger.Info().
		Str("workspace", opts.WorkspaceName).
		Int("pr_number", opts.PRNumber).
		Msg("task abandoned, PR kept as draft")

	return &CIFailureResult{
		Action:  CIFailureAbandon,
		Message: fmt.Sprintf("Task abandoned. PR #%d kept as draft, branch preserved.", opts.PRNumber),
	}, nil
}

// buildCIResultArtifact constructs the artifact structure from CI watch result.
func (h *CIFailureHandler) buildCIResultArtifact(result *git.CIWatchResult) domain.CIResultArtifact {
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

		// Identify failed checks
		bucket := strings.ToLower(check.Bucket)
		if bucket == "fail" || bucket == "cancel" {
			failedChecks = append(failedChecks, checkArtifact)
		}
	}

	artifact.AllChecks = allChecks
	artifact.FailedChecks = failedChecks

	return artifact
}

// extractBestCheckURL finds the best URL to show for failed checks.
func (h *CIFailureHandler) extractBestCheckURL(result *git.CIWatchResult) string {
	if result == nil || len(result.CheckResults) == 0 {
		return ""
	}

	// First, try to find a URL from a failed check
	for _, check := range result.CheckResults {
		bucket := strings.ToLower(check.Bucket)
		if (bucket == "fail" || bucket == "cancel") && check.URL != "" {
			return check.URL
		}
	}

	// Fall back to any check with a URL
	for _, check := range result.CheckResults {
		if check.URL != "" {
			return check.URL
		}
	}

	return ""
}

// ExtractCIErrorContext creates AI-friendly context from CI failure.
func ExtractCIErrorContext(result *git.CIWatchResult) string {
	if result == nil || len(result.CheckResults) == 0 {
		return "CI checks failed but no details available."
	}

	var sb strings.Builder
	sb.WriteString("## CI Failure Context\n\n")
	sb.WriteString("The following CI checks failed:\n\n")

	hasFailures := false
	for _, check := range result.CheckResults {
		bucket := strings.ToLower(check.Bucket)
		if bucket == "fail" || bucket == "cancel" {
			hasFailures = true
			sb.WriteString(fmt.Sprintf("### %s\n", check.Name))
			sb.WriteString(fmt.Sprintf("- Status: %s\n", check.Bucket))
			if check.Workflow != "" {
				sb.WriteString(fmt.Sprintf("- Workflow: %s\n", check.Workflow))
			}
			if check.URL != "" {
				sb.WriteString(fmt.Sprintf("- Logs: %s\n", check.URL))
			}
			sb.WriteString("\n")
		}
	}

	if !hasFailures {
		sb.WriteString("No specific failures identified, but overall CI status indicates failure.\n\n")
	}

	sb.WriteString("Please analyze the failures and fix the issues in the code.\n")
	return sb.String()
}

// FormatManualFixInstructions generates user-facing instructions for manual fixing.
func FormatManualFixInstructions(worktreePath, workspaceName string, result *git.CIWatchResult) string {
	var sb strings.Builder

	sb.WriteString("## Manual Fix Instructions\n\n")
	sb.WriteString("CI has failed. Please fix the issues manually.\n\n")

	sb.WriteString("### Worktree Location\n")
	sb.WriteString(fmt.Sprintf("```\ncd %s\n```\n\n", worktreePath))

	// List failed checks
	if result != nil && len(result.CheckResults) > 0 {
		sb.WriteString("### Failed Checks\n")
		for _, check := range result.CheckResults {
			bucket := strings.ToLower(check.Bucket)
			if bucket == "fail" || bucket == "cancel" {
				sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", check.Name, check.Bucket))
				if check.URL != "" {
					sb.WriteString(fmt.Sprintf("  - Logs: %s\n", check.URL))
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Steps to Fix\n")
	sb.WriteString("1. Navigate to the worktree directory above\n")
	sb.WriteString("2. Analyze the CI failures from the logs\n")
	sb.WriteString("3. Make the necessary code changes\n")
	sb.WriteString("4. Commit and push your changes:\n")
	sb.WriteString("   ```bash\n")
	sb.WriteString("   git add -A\n")
	sb.WriteString("   git commit -m \"fix: address CI failures\"\n")
	sb.WriteString("   git push\n")
	sb.WriteString("   ```\n")
	sb.WriteString("5. Wait for CI to re-run in GitHub\n")
	sb.WriteString("6. Once CI passes, resume with:\n")
	sb.WriteString(fmt.Sprintf("   ```bash\n   atlas resume %s\n   ```\n\n", workspaceName))

	return sb.String()
}

// openInBrowser opens a URL in the default browser.
func openInBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("OS %s: %w", runtime.GOOS, atlaserrors.ErrUnsupportedOS)
	}

	// Use background context with short timeout for browser open
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, cmd, args...).Start() //#nosec G204 -- URL is user-provided, browser handles validation
}
