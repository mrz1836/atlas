package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzActivityParser_ParseLine fuzzes the stderr line parser with arbitrary input.
// It asserts the parser never panics and that any returned event carries a
// populated timestamp (a basic internal-consistency invariant).
func FuzzActivityParser_ParseLine(f *testing.F) {
	seeds := []string{
		"",
		"   ",
		"Reading file: internal/ai/runner.go",
		"edit file: internal/ai/base.go",
		"executing: go test ./...",
		"found bug in main.go",
		"the word exit appears here",
		"###$$$ not a line",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	parser := NewActivityParser()

	f.Fuzz(func(t *testing.T, line string) {
		event := parser.ParseLine(line)
		if event == nil {
			return
		}

		assert.False(t, event.Timestamp.IsZero())
	})
}
