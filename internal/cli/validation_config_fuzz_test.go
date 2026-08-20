package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzParseMultilineInput fuzzes the multiline splitter with arbitrary input.
// It asserts the parser never panics and that every returned entry is non-empty
// and fully trimmed of surrounding whitespace.
func FuzzParseMultilineInput(f *testing.F) {
	seeds := []string{
		"",
		"cmd1\ncmd2\ncmd3",
		"cmd1\n\ncmd2",
		"  cmd1  \n  cmd2  ",
		"   \n   \n   ",
		"magex lint",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		for _, line := range ParseMultilineInput(input) {
			assert.NotEmpty(t, line)
			assert.Equal(t, strings.TrimSpace(line), line)
		}
	})
}
