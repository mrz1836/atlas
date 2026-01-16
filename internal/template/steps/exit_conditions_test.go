package steps

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestExitEvaluator_ParseExitSignal(t *testing.T) {
	logger := zerolog.Nop()
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "standard format",
			output:   `Some output {"exit": true} more text`,
			expected: true,
		},
		{
			name:     "compact format",
			output:   `{"exit":true}`,
			expected: true,
		},
		{
			name:     "with extra whitespace",
			output:   `{  "exit"  :  true  }`,
			expected: true,
		},
		{
			name:     "exit false",
			output:   `{"exit": false}`,
			expected: false,
		},
		{
			name:     "no signal",
			output:   `no signal here`,
			expected: false,
		},
		{
			name:     "partial match - exit string only",
			output:   `the word exit appears here`,
			expected: false,
		},
		{
			name:     "empty output",
			output:   ``,
			expected: false,
		},
		{
			name:     "multiline with signal",
			output:   "line1\nline2\n{\"exit\": true}\nline3",
			expected: true,
		},
	}

	e := NewExitEvaluator(nil, logger)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := e.ParseExitSignal(tc.output)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result, "input: %s", tc.output)
		})
	}
}

func TestExitEvaluator_CheckConditions(t *testing.T) {
	logger := zerolog.Nop()

	tests := []struct {
		name       string
		conditions []string
		output     string
		expected   bool
	}{
		{
			name:       "no conditions",
			conditions: nil,
			output:     "any output",
			expected:   true,
		},
		{
			name:       "single condition met",
			conditions: []string{"all tests passing"},
			output:     "All tests passing successfully",
			expected:   true,
		},
		{
			name:       "single condition not met",
			conditions: []string{"all tests passing"},
			output:     "some tests failed",
			expected:   false,
		},
		{
			name:       "multiple conditions all met",
			conditions: []string{"tests passing", "no lint errors"},
			output:     "All tests passing and no lint errors found",
			expected:   true,
		},
		{
			name:       "multiple conditions partial met",
			conditions: []string{"tests passing", "no lint errors"},
			output:     "All tests passing but lint errors found",
			expected:   false,
		},
		{
			name:       "case insensitive",
			conditions: []string{"TESTS PASSING"},
			output:     "all tests passing",
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExitEvaluator(tc.conditions, logger)
			result := e.CheckConditions(tc.output)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestExitEvaluator_DualGate(t *testing.T) {
	logger := zerolog.Nop()

	tests := []struct {
		name       string
		conditions []string
		output     string
		shouldExit bool
		reason     string
	}{
		{
			name:       "signal only - no conditions",
			conditions: nil,
			output:     `{"exit": true}`,
			shouldExit: true,
			reason:     "exit signal received",
		},
		{
			name:       "signal without conditions met",
			conditions: []string{"all tests passing"},
			output:     `{"exit": true}`,
			shouldExit: false,
			reason:     "exit signal received but conditions not met",
		},
		{
			name:       "conditions without signal",
			conditions: []string{"all tests passing"},
			output:     "all tests passing",
			shouldExit: false,
			reason:     "no exit signal",
		},
		{
			name:       "both signal and conditions",
			conditions: []string{"all tests passing"},
			output:     `all tests passing {"exit": true}`,
			shouldExit: true,
			reason:     "all conditions met with exit signal",
		},
		{
			name:       "multiple conditions with signal",
			conditions: []string{"tests passing", "lint clean"},
			output:     `tests passing, lint clean {"exit": true}`,
			shouldExit: true,
			reason:     "all conditions met with exit signal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExitEvaluator(tc.conditions, logger)
			decision := e.Evaluate(nil, tc.output)
			assert.Equal(t, tc.shouldExit, decision.ShouldExit)
			assert.Contains(t, decision.Reason, tc.reason)
		})
	}
}

func TestBuiltinConditions_CheckTestsPassed(t *testing.T) {
	tests := []struct {
		name     string
		task     *domain.Task
		expected bool
	}{
		{
			name: "validation passed",
			task: &domain.Task{
				StepResults: []domain.StepResult{
					{StepName: "implement", Status: "success"},
					{StepName: "validate", Status: "success"},
				},
			},
			expected: true,
		},
		{
			name: "validation failed",
			task: &domain.Task{
				StepResults: []domain.StepResult{
					{StepName: "implement", Status: "success"},
					{StepName: "validate", Status: "failed"},
				},
			},
			expected: false,
		},
		{
			name: "no validation step",
			task: &domain.Task{
				StepResults: []domain.StepResult{
					{StepName: "implement", Status: "success"},
				},
			},
			expected: false,
		},
		{
			name:     "empty results",
			task:     &domain.Task{StepResults: []domain.StepResult{}},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := checkTestsPassed(tc.task)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuiltinConditions_CheckNoRecentChanges(t *testing.T) {
	tests := []struct {
		name     string
		task     *domain.Task
		expected bool
	}{
		{
			name: "no changes in last result",
			task: &domain.Task{
				StepResults: []domain.StepResult{
					{StepName: "fix", FilesChanged: []string{}},
				},
			},
			expected: true,
		},
		{
			name: "changes in last result",
			task: &domain.Task{
				StepResults: []domain.StepResult{
					{StepName: "fix", FilesChanged: []string{"file.go"}},
				},
			},
			expected: false,
		},
		{
			name:     "empty results",
			task:     &domain.Task{StepResults: []domain.StepResult{}},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := checkNoRecentChanges(tc.task)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestEvaluateBuiltinCondition(t *testing.T) {
	task := &domain.Task{
		StepResults: []domain.StepResult{
			{StepName: "validate", Status: "success"},
		},
	}

	// Known condition
	assert.True(t, EvaluateBuiltinCondition("all_tests_pass", task))
	assert.True(t, EvaluateBuiltinCondition("validation_passed", task))

	// Unknown condition
	assert.False(t, EvaluateBuiltinCondition("unknown_condition", task))
}
