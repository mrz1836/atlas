package steps

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzDefaultExitEvaluator_ParseExitSignal fuzzes the {"exit": true} signal parser
// with arbitrary AI output. It asserts the parser never errors or panics and that a
// positive match always implies the output actually contains the "exit" key.
func FuzzDefaultExitEvaluator_ParseExitSignal(f *testing.F) {
	seeds := []string{
		"",
		`Some output {"exit": true} more text`,
		`{"exit":true}`,
		`{  "exit"  :  true  }`,
		`{"exit": false}`,
		`the word exit appears here`,
		"line1\nline2\n{\"exit\": true}\nline3",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	evaluator := NewExitEvaluator(nil, zerolog.Nop())

	f.Fuzz(func(t *testing.T, output string) {
		hasSignal, err := evaluator.ParseExitSignal(output)
		require.NoError(t, err)

		if hasSignal {
			assert.Contains(t, output, `"exit"`)
		}
	})
}
