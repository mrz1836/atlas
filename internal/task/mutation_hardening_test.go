package task

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template/steps"
)

// The tests in this file close specific gaps surfaced by mutation testing
// (gremlins). Each targets a boundary or guard whose behavior was executed by
// existing tests but never *asserted*, so a silent regression (an off-by-one or
// a dropped guard) would have gone unnoticed.

// TestHasSubstantiveDescription_Boundary pins the exact threshold: a description
// is substantive only when strictly longer than 20 characters. Prior tests used
// an 8-char and a 60-char description, so the ">20" vs ">=20" boundary was
// unguarded — a 20-char description is the discriminating case.
func TestHasSubstantiveDescription_Boundary(t *testing.T) {
	t.Parallel()
	e := &Engine{}

	tests := []struct {
		name string
		desc string
		want bool
	}{
		{name: "empty", desc: "", want: false},
		{name: "nineteen_chars", desc: "1234567890123456789", want: false},
		{name: "exactly_twenty_chars", desc: "12345678901234567890", want: false}, // 20 is NOT > 20
		{name: "twenty_one_chars", desc: "123456789012345678901", want: true},
		{name: "twenty_with_surrounding_whitespace_trimmed", desc: "   12345678901234567890   ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, e.hasSubstantiveDescription(&domain.Task{Description: tt.desc}),
				"len=%d", len(tt.desc))
		})
	}
}

// TestCreateValidationProgressCallback_NilGuard verifies the nil-guard: with no
// engine-level progress callback configured, no wrapper callback is created
// (returning nil avoids invoking a nil callback later). With one configured, a
// non-nil wrapper is returned.
func TestCreateValidationProgressCallback_NilGuard(t *testing.T) {
	t.Parallel()

	t.Run("nil_config_returns_nil", func(t *testing.T) {
		t.Parallel()
		e := &Engine{}
		assert.Nil(t, e.createValidationProgressCallback(&domain.Task{ID: "t1"}),
			"no wrapper should be created when ProgressCallback is unset")
	})

	t.Run("configured_returns_non_nil", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultEngineConfig()
		cfg.ProgressCallback = func(StepProgressEvent) {}
		e := NewEngine(newMockStore(), steps.NewExecutorRegistry(), cfg, testLogger())
		assert.NotNil(t, e.createValidationProgressCallback(&domain.Task{ID: "t1"}),
			"a wrapper must be created when ProgressCallback is set")
	})
}

// TestApplyStepApprovalChoice_OutOfBoundsGuard verifies the bounds guard returns
// an error (rather than panicking on template.Steps[CurrentStep]) when the
// current step index is at or beyond the number of template steps. This pins the
// ">= len" boundary that prevents an index-out-of-range panic.
func TestApplyStepApprovalChoice_OutOfBoundsGuard(t *testing.T) {
	t.Parallel()
	e := &Engine{}
	template := &domain.Template{Steps: []domain.StepDefinition{{Name: "a"}, {Name: "b"}}}

	for _, currentStep := range []int{2, 3, 99} {
		t.Run("index_at_or_past_end", func(t *testing.T) {
			t.Parallel()
			task := &domain.Task{CurrentStep: currentStep}
			err := e.applyStepApprovalChoice(context.Background(), task, template, "r")
			require.Error(t, err, "out-of-bounds current step must error, not panic")
			assert.ErrorIs(t, err, atlaserrors.ErrInvalidArgument)
		})
	}
}

// TestHandleSkippedStep_CurrentStepAtBoundary verifies handleSkippedStep does not
// panic when CurrentStep equals len(Steps) (the "CurrentStep < len(Steps)" guard
// around the Steps[CurrentStep] write), still records a skipped result, and
// advances. Prior tests only exercised in-bounds indices.
func TestHandleSkippedStep_CurrentStepAtBoundary(t *testing.T) {
	t.Parallel()

	store := newMockStore()
	engine := NewEngine(store, steps.NewExecutorRegistry(), DefaultEngineConfig(), testLogger())

	task := &domain.Task{
		ID:          "task-boundary",
		WorkspaceID: "ws-boundary",
		Description: "boundary skip",
		Steps:       []domain.Step{{Name: "only-step"}},
		CurrentStep: 1, // == len(Steps): the boundary that must not index Steps[1]
	}
	step := &domain.StepDefinition{Name: "phantom", Type: domain.StepTypeAI}

	require.NotPanics(t, func() {
		err := engine.handleSkippedStep(context.Background(), task, step)
		require.NoError(t, err)
	}, "handleSkippedStep must not panic when CurrentStep == len(Steps)")

	require.Len(t, task.StepResults, 1, "a skipped result must still be recorded at the boundary")
	assert.Equal(t, "skipped", task.StepResults[0].Status)
}
