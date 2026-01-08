package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineResult_BuildChecks(t *testing.T) {
	t.Parallel()

	t.Run("all passing", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults:    []Result{{Success: true}},
			LintResults:      []Result{{Success: true}},
			TestResults:      []Result{{Success: true}},
			PreCommitResults: []Result{{Success: true}},
		}

		checks := result.BuildChecks()

		require.Len(t, checks, 4)
		assert.Equal(t, "Format", checks[0].Name)
		assert.True(t, checks[0].Passed)
		assert.Equal(t, "Lint", checks[1].Name)
		assert.True(t, checks[1].Passed)
		assert.Equal(t, "Test", checks[2].Name)
		assert.True(t, checks[2].Passed)
		assert.Equal(t, "Pre-commit", checks[3].Name)
		assert.True(t, checks[3].Passed)
		assert.False(t, checks[3].Skipped)
	})

	t.Run("some failing", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults:    []Result{{Success: true}},
			LintResults:      []Result{{Success: false}},
			TestResults:      []Result{{Success: true}},
			PreCommitResults: []Result{{Success: false}},
		}

		checks := result.BuildChecks()

		require.Len(t, checks, 4)
		assert.True(t, checks[0].Passed)
		assert.False(t, checks[1].Passed)
		assert.True(t, checks[2].Passed)
		assert.False(t, checks[3].Passed)
	})

	t.Run("empty results pass", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults:    []Result{},
			LintResults:      []Result{},
			TestResults:      []Result{},
			PreCommitResults: []Result{},
		}

		checks := result.BuildChecks()

		require.Len(t, checks, 4)
		assert.True(t, checks[0].Passed)
		assert.True(t, checks[1].Passed)
		assert.True(t, checks[2].Passed)
		assert.True(t, checks[3].Passed)
	})

	t.Run("pre-commit skipped", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults: []Result{{Success: true}},
			LintResults:   []Result{{Success: true}},
			TestResults:   []Result{{Success: true}},
			SkippedSteps:  []string{"pre-commit"},
		}

		checks := result.BuildChecks()

		require.Len(t, checks, 4)
		assert.True(t, checks[3].Passed)
		assert.True(t, checks[3].Skipped)
	})

	t.Run("nil result returns nil", func(t *testing.T) {
		t.Parallel()
		var result *PipelineResult
		checks := result.BuildChecks()
		assert.Nil(t, checks)
	})

	t.Run("multiple results with mixed success", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults:    []Result{{Success: true}, {Success: true}},
			LintResults:      []Result{{Success: true}, {Success: false}}, // One failure
			TestResults:      []Result{{Success: true}},
			PreCommitResults: []Result{{Success: true}},
		}

		checks := result.BuildChecks()

		assert.True(t, checks[0].Passed)
		assert.False(t, checks[1].Passed) // Lint should fail (one failure)
		assert.True(t, checks[2].Passed)
		assert.True(t, checks[3].Passed)
	})
}

func TestPipelineResult_BuildChecksAsMap(t *testing.T) {
	t.Parallel()

	t.Run("converts to map format", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults:    []Result{{Success: true}},
			LintResults:      []Result{{Success: false}},
			TestResults:      []Result{{Success: true}},
			PreCommitResults: []Result{{Success: true}},
		}

		checks := result.BuildChecksAsMap()

		require.Len(t, checks, 4)
		assert.Equal(t, "Format", checks[0]["name"])
		assert.True(t, checks[0]["passed"].(bool))
		assert.Equal(t, "Lint", checks[1]["name"])
		assert.False(t, checks[1]["passed"].(bool))
	})

	t.Run("includes skipped flag when true", func(t *testing.T) {
		t.Parallel()
		result := &PipelineResult{
			FormatResults: []Result{{Success: true}},
			LintResults:   []Result{{Success: true}},
			TestResults:   []Result{{Success: true}},
			SkippedSteps:  []string{"pre-commit"},
		}

		checks := result.BuildChecksAsMap()

		// Pre-commit should have skipped=true
		assert.True(t, checks[3]["skipped"].(bool))

		// Other checks should not have skipped key
		_, hasSkipped := checks[0]["skipped"]
		assert.False(t, hasSkipped)
	})

	t.Run("nil result returns nil", func(t *testing.T) {
		t.Parallel()
		var result *PipelineResult
		checks := result.BuildChecksAsMap()
		assert.Nil(t, checks)
	})
}

func TestHasFailedResult(t *testing.T) {
	t.Parallel()

	t.Run("returns false for empty slice", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasFailedResult([]Result{}))
	})

	t.Run("returns false for all success", func(t *testing.T) {
		t.Parallel()
		results := []Result{{Success: true}, {Success: true}}
		assert.False(t, hasFailedResult(results))
	})

	t.Run("returns true for any failure", func(t *testing.T) {
		t.Parallel()
		results := []Result{{Success: true}, {Success: false}}
		assert.True(t, hasFailedResult(results))
	})
}
