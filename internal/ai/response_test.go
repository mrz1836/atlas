package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestParseClaudeResponse(t *testing.T) {
	t.Run("parses valid JSON response", func(t *testing.T) {
		data := []byte(`{
			"type": "result",
			"subtype": "success",
			"is_error": false,
			"result": "Task completed successfully",
			"session_id": "sess-abc123",
			"duration_ms": 5000,
			"num_turns": 3,
			"total_cost_usd": 0.05
		}`)

		resp, err := parseClaudeResponse(data)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "result", resp.Type)
		assert.Equal(t, "success", resp.Subtype)
		assert.False(t, resp.IsError)
		assert.Equal(t, "Task completed successfully", resp.Result)
		assert.Equal(t, "sess-abc123", resp.SessionID)
		assert.Equal(t, 5000, resp.Duration)
		assert.Equal(t, 3, resp.NumTurns)
		assert.InEpsilon(t, 0.05, resp.TotalCost, 0.0001)
	})

	t.Run("parses error response", func(t *testing.T) {
		data := []byte(`{
			"type": "result",
			"is_error": true,
			"result": "An error occurred",
			"session_id": "sess-err123",
			"duration_ms": 1000,
			"num_turns": 1,
			"total_cost_usd": 0.01
		}`)

		resp, err := parseClaudeResponse(data)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.IsError)
		assert.Equal(t, "An error occurred", resp.Result)
	})

	t.Run("returns error for empty data", func(t *testing.T) {
		resp, err := parseClaudeResponse([]byte(""))

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "empty response")
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		data := []byte("not valid json")

		resp, err := parseClaudeResponse(data)

		require.Error(t, err)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrClaudeInvocation)
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("handles minimal valid response", func(t *testing.T) {
		data := []byte(`{"result":"minimal"}`)

		resp, err := parseClaudeResponse(data)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "minimal", resp.Result)
		// Zero values for unset fields
		assert.Equal(t, 0, resp.Duration)
		assert.Equal(t, 0, resp.NumTurns)
	})
}

func TestClaudeResponse_ToAIResult(t *testing.T) {
	t.Run("converts successful response", func(t *testing.T) {
		resp := &ClaudeResponse{
			Type:      "result",
			IsError:   false,
			Result:    "Task completed",
			SessionID: "sess-123",
			Duration:  5000,
			NumTurns:  3,
			TotalCost: 0.05,
		}

		result := resp.toAIResult("")

		assert.True(t, result.Success)
		assert.Equal(t, "Task completed", result.Output)
		assert.Equal(t, "sess-123", result.SessionID)
		assert.Equal(t, 5000, result.DurationMs)
		assert.Equal(t, 3, result.NumTurns)
		assert.InEpsilon(t, 0.05, result.TotalCostUSD, 0.0001)
		assert.Empty(t, result.Error)
	})

	t.Run("converts error response with stderr", func(t *testing.T) {
		resp := &ClaudeResponse{
			Type:      "result",
			IsError:   true,
			Result:    "Failed to complete",
			SessionID: "sess-err",
			Duration:  1000,
			NumTurns:  1,
			TotalCost: 0.01,
		}

		result := resp.toAIResult("Error details from stderr")

		assert.False(t, result.Success)
		assert.Equal(t, "Failed to complete", result.Output)
		assert.Equal(t, "Error details from stderr", result.Error)
	})

	t.Run("does not include stderr for non-error response", func(t *testing.T) {
		resp := &ClaudeResponse{
			Type:    "result",
			IsError: false,
			Result:  "Success",
		}

		result := resp.toAIResult("Some stderr output")

		assert.True(t, result.Success)
		assert.Empty(t, result.Error) // No error field for success
	})
}
