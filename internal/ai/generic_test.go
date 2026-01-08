package ai

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTestParse = errors.New("test parse error")

// testResponse is a simple response type for testing the generic parser.
type testResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func TestParseResponse(t *testing.T) {
	t.Run("parses valid JSON successfully", func(t *testing.T) {
		data := []byte(`{"success":true,"message":"hello"}`)

		resp, err := parseResponse[testResponse](data, errTestParse)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "hello", resp.Message)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		resp, err := parseResponse[testResponse]([]byte{}, errTestParse)

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, errTestParse)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("returns error for nil data", func(t *testing.T) {
		resp, err := parseResponse[testResponse](nil, errTestParse)

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, errTestParse)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		data := []byte(`not valid json`)

		resp, err := parseResponse[testResponse](data, errTestParse)

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, errTestParse)
		assert.Contains(t, err.Error(), "parse json")
	})

	t.Run("includes byte count in error message", func(t *testing.T) {
		data := []byte(`{invalid}`)

		_, err := parseResponse[testResponse](data, errTestParse)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "9 bytes")
	})

	t.Run("handles partial JSON gracefully", func(t *testing.T) {
		data := []byte(`{"success":true`)

		resp, err := parseResponse[testResponse](data, errTestParse)

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, errTestParse)
	})

	t.Run("handles missing fields", func(t *testing.T) {
		data := []byte(`{}`)

		resp, err := parseResponse[testResponse](data, errTestParse)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success) // Zero value for bool
		assert.Empty(t, resp.Message) // Zero value for string
	})
}
