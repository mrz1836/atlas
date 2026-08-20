package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzParseMaxTurnsWithDefault fuzzes the max-turns parser with arbitrary input.
// It asserts the parser never panics and always returns a value within the valid
// 1-100 range (the in-range default is returned for any malformed input).
func FuzzParseMaxTurnsWithDefault(f *testing.F) {
	const defaultVal = 10

	seeds := []string{
		"",
		"25",
		"100",
		"101",
		"abc",
		"-5",
		"10.5",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result := ParseMaxTurnsWithDefault(input, defaultVal)

		assert.GreaterOrEqual(t, result, 1)
		assert.LessOrEqual(t, result, 100)
	})
}
