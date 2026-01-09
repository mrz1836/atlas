package steps

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewStepResult(t *testing.T) {
	t.Parallel()

	task := &domain.Task{CurrentStep: 2}
	step := &domain.StepDefinition{Name: "test_step"}
	startTime := time.Now().Add(-5 * time.Second)

	result := newStepResult(task, step, startTime, "custom_status")

	require.NotNil(t, result)
	assert.Equal(t, 2, result.StepIndex)
	assert.Equal(t, "test_step", result.StepName)
	assert.Equal(t, "custom_status", result.Status)
	assert.Equal(t, startTime, result.StartedAt)
	assert.True(t, result.CompletedAt.After(startTime))
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestNewSuccessResult(t *testing.T) {
	t.Parallel()

	task := &domain.Task{CurrentStep: 1}
	step := &domain.StepDefinition{Name: "implement"}
	startTime := time.Now()

	result := newSuccessResult(task, step, startTime)

	require.NotNil(t, result)
	assert.Equal(t, 1, result.StepIndex)
	assert.Equal(t, "implement", result.StepName)
	assert.Equal(t, constants.StepStatusSuccess, result.Status)
	assert.Empty(t, result.Error)
}

func TestNewFailedResult(t *testing.T) {
	t.Parallel()

	task := &domain.Task{CurrentStep: 3}
	step := &domain.StepDefinition{Name: "validate"}
	startTime := time.Now()
	errMsg := "validation failed: lint errors"

	result := newFailedResult(task, step, startTime, errMsg)

	require.NotNil(t, result)
	assert.Equal(t, 3, result.StepIndex)
	assert.Equal(t, "validate", result.StepName)
	assert.Equal(t, constants.StepStatusFailed, result.Status)
	assert.Equal(t, errMsg, result.Error)
}

func TestNewAwaitingApprovalResult(t *testing.T) {
	t.Parallel()

	task := &domain.Task{CurrentStep: 4}
	step := &domain.StepDefinition{Name: "review"}
	startTime := time.Now()

	result := newAwaitingApprovalResult(task, step, startTime)

	require.NotNil(t, result)
	assert.Equal(t, 4, result.StepIndex)
	assert.Equal(t, "review", result.StepName)
	assert.Equal(t, constants.StepStatusAwaitingApproval, result.Status)
}

func TestResultCanBeCustomized(t *testing.T) {
	t.Parallel()

	task := &domain.Task{CurrentStep: 0}
	step := &domain.StepDefinition{Name: "ci_wait"}
	startTime := time.Now()

	result := newSuccessResult(task, step, startTime)
	result.Output = "CI passed in 2m30s"
	result.ArtifactPath = "/path/to/artifact.json"
	result.Metadata = map[string]any{"checks": 5}

	assert.Equal(t, "CI passed in 2m30s", result.Output)
	assert.Equal(t, "/path/to/artifact.json", result.ArtifactPath)
	assert.Equal(t, 5, result.Metadata["checks"])
}
