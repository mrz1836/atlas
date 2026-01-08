// Package git provides Git operations for ATLAS.
// This file contains GitHub error types and classification functions.
package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	// PRErrorNoChecksYet indicates CI checks haven't been registered yet - transient, retry.
	// This occurs immediately after PR creation before GitHub Actions workflows start.
	PRErrorNoChecksYet
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
	case PRErrorNoChecksYet:
		return "no_checks_yet"
	case PRErrorOther:
		return "other"
	}
	return "other"
}

// CIStatus represents the overall CI status.
type CIStatus int

const (
	// CIStatusPending indicates CI checks are still running.
	CIStatusPending CIStatus = iota
	// CIStatusSuccess indicates all required CI checks passed.
	CIStatusSuccess
	// CIStatusFailure indicates one or more CI checks failed.
	CIStatusFailure
	// CIStatusTimeout indicates CI polling exceeded the timeout.
	CIStatusTimeout
	// CIStatusFetchError indicates CI status could not be determined due to fetch failures.
	// This is distinct from CIStatusFailure - the CI may have passed, but we couldn't verify.
	CIStatusFetchError
)

// String returns a string representation of the CI status.
func (s CIStatus) String() string {
	switch s {
	case CIStatusPending:
		return "pending"
	case CIStatusSuccess:
		return "success"
	case CIStatusFailure:
		return "failure"
	case CIStatusTimeout:
		return "timeout"
	case CIStatusFetchError:
		return "fetch_error"
	default:
		return "unknown"
	}
}

// shouldRetryPR determines if the error type is retryable.
// PRErrorOther is now retryable since unknown gh errors may be transient.
func shouldRetryPR(errType PRErrorType) bool {
	return errType == PRErrorNetwork || errType == PRErrorRateLimit || errType == PRErrorOther
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
	case PRErrorNoChecksYet:
		return fmt.Errorf("no checks reported yet: %w", atlaserrors.ErrPRCreationFailed)
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

	// Check for sentinel errors first (more reliable than string matching)
	if errors.Is(err, atlaserrors.ErrGHAuthFailed) {
		return PRErrorAuth
	}
	if errors.Is(err, atlaserrors.ErrGHRateLimited) {
		return PRErrorRateLimit
	}
	if errors.Is(err, atlaserrors.ErrPRNotFound) {
		return PRErrorNotFound
	}

	errStr := strings.ToLower(err.Error())

	if MatchesRateLimitError(errStr) {
		return PRErrorRateLimit
	}

	if MatchesAuthError(errStr) {
		return PRErrorAuth
	}

	if MatchesNetworkError(errStr) {
		return PRErrorNetwork
	}

	if MatchesNotFoundError(errStr) {
		return PRErrorNotFound
	}

	// Check for "no checks reported" before falling through to PRErrorOther
	// This is a transient condition when CI checks haven't started yet
	if isGHNoChecksReportedError(errStr) {
		return PRErrorNoChecksYet
	}

	return PRErrorOther
}

// isGHNoChecksReportedError checks if the error indicates CI checks haven't been registered yet.
// This is a transient condition that occurs immediately after PR creation before workflows start.
func isGHNoChecksReportedError(errStr string) bool {
	return strings.Contains(errStr, "no checks reported")
}
