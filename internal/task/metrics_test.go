package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNoopMetrics_ImplementsInterface(t *testing.T) {
	// Verify NoopMetrics implements Metrics interface
	var m Metrics = NoopMetrics{}
	assert.NotNil(t, m)
}

func TestNoopMetrics_MethodsDoNotPanic(t *testing.T) {
	m := NoopMetrics{}

	// All methods should complete without panicking
	assert.NotPanics(t, func() {
		m.TaskStarted("task-1", "template-1")
	})

	assert.NotPanics(t, func() {
		m.TaskCompleted("task-1", time.Second, "completed")
	})

	assert.NotPanics(t, func() {
		m.StepExecuted("task-1", "step-1", domain.StepTypeAI, time.Second, true)
	})

	assert.NotPanics(t, func() {
		m.LoopIteration("task-1", "loop-step", 1, time.Second)
	})
}

// mockMetrics is a test implementation that records calls for verification.
type mockMetrics struct {
	taskStartedCalls   []taskStartedCall
	taskCompletedCalls []taskCompletedCall
	stepExecutedCalls  []stepExecutedCall
	loopIterationCalls []loopIterationCall
}

type taskStartedCall struct {
	taskID       string
	templateName string
}

type taskCompletedCall struct {
	taskID   string
	duration time.Duration
	status   string
}

type stepExecutedCall struct {
	taskID   string
	stepName string
	stepType domain.StepType
	duration time.Duration
	success  bool
}

type loopIterationCall struct {
	taskID    string
	stepName  string
	iteration int
	duration  time.Duration
}

func (m *mockMetrics) TaskStarted(taskID, templateName string) {
	m.taskStartedCalls = append(m.taskStartedCalls, taskStartedCall{taskID, templateName})
}

func (m *mockMetrics) TaskCompleted(taskID string, duration time.Duration, status string) {
	m.taskCompletedCalls = append(m.taskCompletedCalls, taskCompletedCall{taskID, duration, status})
}

func (m *mockMetrics) StepExecuted(taskID, stepName string, stepType domain.StepType, duration time.Duration, success bool) {
	m.stepExecutedCalls = append(m.stepExecutedCalls, stepExecutedCall{taskID, stepName, stepType, duration, success})
}

func (m *mockMetrics) LoopIteration(taskID, stepName string, iteration int, duration time.Duration) {
	m.loopIterationCalls = append(m.loopIterationCalls, loopIterationCall{taskID, stepName, iteration, duration})
}

func TestMockMetrics_ImplementsInterface(t *testing.T) {
	// Verify mockMetrics implements Metrics interface
	var m Metrics = &mockMetrics{}
	assert.NotNil(t, m)
}

func TestMockMetrics_RecordsCalls(t *testing.T) {
	m := &mockMetrics{}

	m.TaskStarted("task-123", "bugfix")
	m.TaskCompleted("task-123", 5*time.Second, "completed")
	m.StepExecuted("task-123", "implement", domain.StepTypeAI, 2*time.Second, true)
	m.LoopIteration("task-123", "fix-loop", 3, time.Second)

	assert.Len(t, m.taskStartedCalls, 1)
	assert.Equal(t, "task-123", m.taskStartedCalls[0].taskID)
	assert.Equal(t, "bugfix", m.taskStartedCalls[0].templateName)

	assert.Len(t, m.taskCompletedCalls, 1)
	assert.Equal(t, "task-123", m.taskCompletedCalls[0].taskID)
	assert.Equal(t, 5*time.Second, m.taskCompletedCalls[0].duration)
	assert.Equal(t, "completed", m.taskCompletedCalls[0].status)

	assert.Len(t, m.stepExecutedCalls, 1)
	assert.Equal(t, "task-123", m.stepExecutedCalls[0].taskID)
	assert.Equal(t, "implement", m.stepExecutedCalls[0].stepName)
	assert.Equal(t, domain.StepTypeAI, m.stepExecutedCalls[0].stepType)
	assert.True(t, m.stepExecutedCalls[0].success)

	assert.Len(t, m.loopIterationCalls, 1)
	assert.Equal(t, "task-123", m.loopIterationCalls[0].taskID)
	assert.Equal(t, "fix-loop", m.loopIterationCalls[0].stepName)
	assert.Equal(t, 3, m.loopIterationCalls[0].iteration)
}
