package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzParseStepType fuzzes the step-type parser with arbitrary input.
// It asserts the parser never panics and that its (StepType, error) result is
// internally consistent: a nil error implies a valid, non-empty type, while an
// error implies an empty type.
func FuzzParseStepType(f *testing.F) {
	seeds := []string{
		"",
		"ai",
		"VALIDATION",
		"  git  ",
		"loop",
		"invalid",
		"magic",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		stepType, err := ParseStepType(input)
		if err != nil {
			assert.Empty(t, string(stepType))
			return
		}

		assert.True(t, IsValidStepType(stepType))
		assert.NotEmpty(t, string(stepType))
	})
}
