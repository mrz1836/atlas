package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzToolStatus_UnmarshalJSON fuzzes the custom ToolStatus JSON decoder with
// arbitrary bytes. It asserts decoding never errors or panics and always resolves
// to one of the three well-defined statuses (defaulting to "missing").
func FuzzToolStatus_UnmarshalJSON(f *testing.F) {
	seeds := []string{
		"",
		`"installed"`,
		`"missing"`,
		`"outdated"`,
		`"unknown"`,
		`123`,
		`garbage`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	validStatuses := []string{"installed", "missing", "outdated"}

	f.Fuzz(func(t *testing.T, data string) {
		var status ToolStatus
		require.NoError(t, status.UnmarshalJSON([]byte(data)))

		// The decoder never yields an invalid enum value.
		assert.Contains(t, validStatuses, status.String())
	})
}
