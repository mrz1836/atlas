package constants

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVersioningConstants(t *testing.T) {
	t.Run("MaxVersionNumber prevents runaway versioning", func(t *testing.T) {
		assert.Equal(t, 10000, MaxVersionNumber)
		assert.Greater(t, MaxVersionNumber, 1000, "should allow many versions before limiting")
	})

	t.Run("LockRetryInterval is reasonable", func(t *testing.T) {
		assert.Equal(t, 50*time.Millisecond, LockRetryInterval)
		assert.Less(t, LockRetryInterval, time.Second, "should retry quickly")
	})
}

func TestProcessManagementConstants(t *testing.T) {
	t.Run("ProcessTerminationTimeout allows graceful shutdown", func(t *testing.T) {
		assert.Equal(t, 2*time.Second, ProcessTerminationTimeout)
		assert.GreaterOrEqual(t, ProcessTerminationTimeout, time.Second, "should give processes time to terminate")
	})
}

func TestPRDescriptionConstants(t *testing.T) {
	t.Run("MaxPRSummaryLength matches conventional commit guidelines", func(t *testing.T) {
		assert.Equal(t, 50, MaxPRSummaryLength)
	})

	t.Run("PRSummaryTruncationSuffix is ellipsis", func(t *testing.T) {
		assert.Equal(t, "...", PRSummaryTruncationSuffix)
	})

	t.Run("ConventionalCommitPrefixMaxLength handles common prefixes", func(t *testing.T) {
		assert.Equal(t, 20, ConventionalCommitPrefixMaxLength)
		// Should be long enough for "refactor(component): "
		assert.GreaterOrEqual(t, ConventionalCommitPrefixMaxLength, len("refactor(): "))
	})
}

func TestGitOperationConstants(t *testing.T) {
	t.Run("GitPushRetryCount allows recovery from transient failures", func(t *testing.T) {
		assert.Equal(t, 3, GitPushRetryCount)
		assert.Greater(t, GitPushRetryCount, 1, "should retry at least once")
	})

	t.Run("GitPRRetryCount is fewer than push retries", func(t *testing.T) {
		assert.Equal(t, 2, GitPRRetryCount)
		assert.LessOrEqual(t, GitPRRetryCount, GitPushRetryCount, "PR creation is more critical than push")
	})
}

func TestStepStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"StepStatusSuccess", StepStatusSuccess, "success"},
		{"StepStatusFailed", StepStatusFailed, "failed"},
		{"StepStatusPending", StepStatusPending, "pending"},
		{"StepStatusRunning", StepStatusRunning, "running"},
		{"StepStatusAwaitingApproval", StepStatusAwaitingApproval, "awaiting_approval"},
		{"StepStatusNoChanges", StepStatusNoChanges, "no_changes"},
		{"StepStatusSkipped", StepStatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}
