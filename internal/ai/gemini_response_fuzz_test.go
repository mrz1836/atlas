package ai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzGeminiError_UnmarshalJSON fuzzes the custom UnmarshalJSON that accepts both
// string and object error formats. It asserts unmarshaling never panics and that a
// successfully-decoded error always renders a non-empty String().
func FuzzGeminiError_UnmarshalJSON(f *testing.F) {
	seeds := []string{
		"",
		`"simple error message"`,
		`{"type":"ApiError","message":"Rate limit exceeded"}`,
		`{"type":"AuthError","message":"Invalid API key","code":401}`,
		`{invalid json}`,
		`123`,
		`null`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data string) {
		var ge GeminiError
		if err := json.Unmarshal([]byte(data), &ge); err != nil {
			return
		}

		// A decoded error must always render a usable, non-empty message.
		assert.NotEmpty(t, ge.String())
	})
}
