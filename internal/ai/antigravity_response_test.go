package ai

// This suite exercises the pure parsing and mapping helpers in
// antigravity_response.go directly (no subprocess execution and no API calls).
// It complements antigravity_test.go, which drives the same code through the
// AntigravityRunner. All inputs are hand-built JSON payloads that mirror the
// real `agy --output-format json` schema.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestAntigravityResponse_IsSuccess covers both branches of isSuccess,
// including the trim/case-insensitive handling.
func TestAntigravityResponse_IsSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "exact success", status: "SUCCESS", want: true},
		{name: "lowercase success is case-insensitive", status: "success", want: true},
		{name: "mixed case success", status: "Success", want: true},
		{name: "surrounding whitespace is trimmed", status: "  SUCCESS\n", want: true},
		{name: "error status is not success", status: "ERROR", want: false},
		{name: "empty status is not success", status: "", want: false},
		{name: "failed status is not success", status: "FAILED", want: false},
		{name: "prefix match is not enough", status: "SUCCESSFUL", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &AntigravityResponse{Status: tt.status}
			assert.Equal(t, tt.want, resp.isSuccess())
		})
	}
}

// TestAntigravityResponse_ToAIResult_Success asserts every mapped field on a
// successful conversion, including the seconds-to-milliseconds derivation.
func TestAntigravityResponse_ToAIResult_Success(t *testing.T) {
	t.Parallel()

	resp := &AntigravityResponse{
		ConversationID:  "conv-ok",
		Status:          "SUCCESS",
		Response:        "All set. ✅",
		DurationSeconds: 1.5,
		NumTurns:        4,
		Usage:           AntigravityUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}

	result := resp.toAIResult("stderr should be ignored on success")
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "All set. ✅", result.Output)
	assert.Equal(t, "conv-ok", result.SessionID)
	assert.Equal(t, 4, result.NumTurns)
	assert.Empty(t, result.Error)
	// 1.5s -> ~1500ms; assert with bounds rather than exact float-derived value.
	assert.GreaterOrEqual(t, result.DurationMs, 1490)
	assert.LessOrEqual(t, result.DurationMs, 1500)
	// agy reports tokens, not a dollar cost, and never a file list.
	assert.Zero(t, result.TotalCostUSD)
	assert.Nil(t, result.FilesChanged)
}

// TestAntigravityResponse_ToAIResult_Failure covers the error-message
// precedence chain used on non-success conversions.
func TestAntigravityResponse_ToAIResult_Failure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resp      *AntigravityResponse
		stderr    string
		wantError string
	}{
		{
			name:      "explicit error field wins",
			resp:      &AntigravityResponse{Status: "ERROR", Error: "explicit error", Message: "msg", Response: "body"},
			stderr:    "stderr",
			wantError: "explicit error",
		},
		{
			name:      "message used when error empty",
			resp:      &AntigravityResponse{Status: "ERROR", Message: "message field", Response: "body"},
			stderr:    "stderr",
			wantError: "message field",
		},
		{
			name:      "response body used when error and message empty",
			resp:      &AntigravityResponse{Status: "ERROR", Response: "response body"},
			stderr:    "stderr",
			wantError: "response body",
		},
		{
			name:      "stderr used when all payload fields empty",
			resp:      &AntigravityResponse{Status: "ERROR"},
			stderr:    "stderr fallback",
			wantError: "stderr fallback",
		},
		{
			name:      "whitespace-only candidates are skipped",
			resp:      &AntigravityResponse{Status: "ERROR", Error: "   ", Message: "\n\t", Response: "real cause"},
			stderr:    "stderr",
			wantError: "real cause",
		},
		{
			name:      "no context leaves error empty",
			resp:      &AntigravityResponse{Status: "ERROR"},
			stderr:    "",
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.resp.toAIResult(tt.stderr)
			require.NotNil(t, result)
			assert.False(t, result.Success)
			assert.Equal(t, tt.wantError, result.Error)
			// Output always mirrors the response body regardless of success.
			assert.Equal(t, tt.resp.Response, result.Output)
		})
	}
}

// TestParseAntigravityResponse_FullPayloadMapping verifies a complete payload
// parses and maps every AIResult field.
func TestParseAntigravityResponse_FullPayloadMapping(t *testing.T) {
	t.Parallel()

	data := []byte(`{"conversation_id":"conv-77","status":"SUCCESS",` +
		`"response":"Hi there! ✅ Ωμέγα наука","duration_seconds":11.97,"num_turns":3,` +
		`"usage":{"input_tokens":14187,"output_tokens":90,"thinking_tokens":12,` +
		`"cache_read_tokens":5,"total_tokens":14294}}`)

	resp, err := parseAntigravityResponse(data)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Parsed struct fields.
	assert.Equal(t, "conv-77", resp.ConversationID)
	assert.Equal(t, "SUCCESS", resp.Status)
	assert.Equal(t, "Hi there! ✅ Ωμέγα наука", resp.Response)
	assert.InDelta(t, 11.97, resp.DurationSeconds, 0.0001)
	assert.Equal(t, 3, resp.NumTurns)
	assert.Equal(t, 14187, resp.Usage.InputTokens)
	assert.Equal(t, 90, resp.Usage.OutputTokens)
	assert.Equal(t, 12, resp.Usage.ThinkingTokens)
	assert.Equal(t, 5, resp.Usage.CacheReadTokens)
	assert.Equal(t, 14294, resp.Usage.TotalTokens)
	assert.True(t, resp.isSuccess())

	// Full AIResult mapping.
	result := resp.toAIResult("")
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "Hi there! ✅ Ωμέγα наука", result.Output)
	assert.Equal(t, "conv-77", result.SessionID)
	assert.Equal(t, 3, result.NumTurns)
	assert.Empty(t, result.Error)
	// 11.97s -> ~11970ms; bounds guard against float truncation.
	assert.GreaterOrEqual(t, result.DurationMs, 11960)
	assert.LessOrEqual(t, result.DurationMs, 11970)
	assert.Zero(t, result.TotalCostUSD)
	assert.Nil(t, result.FilesChanged)
}

// TestParseAntigravityResponse_EdgeCases covers Category F edge cases and an
// explicitly failed response flowing through toAIResult.
func TestParseAntigravityResponse_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns wrapped error", func(t *testing.T) {
		t.Parallel()
		resp, err := parseAntigravityResponse([]byte(""))
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
		assert.ErrorContains(t, err, "empty response")
	})

	t.Run("nil input returns wrapped error", func(t *testing.T) {
		t.Parallel()
		resp, err := parseAntigravityResponse(nil)
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
	})

	t.Run("invalid json returns wrapped error", func(t *testing.T) {
		t.Parallel()
		resp, err := parseAntigravityResponse([]byte(`{"status": not-valid-json`))
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrAntigravityInvocation)
		assert.ErrorContains(t, err, "failed to parse json response")
	})

	t.Run("zero-valued fields parse cleanly", func(t *testing.T) {
		t.Parallel()
		resp, err := parseAntigravityResponse([]byte(`{}`))
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.isSuccess())
		assert.Empty(t, resp.Response)
		assert.Zero(t, resp.NumTurns)

		result := resp.toAIResult("")
		assert.False(t, result.Success)
		assert.Empty(t, result.Output)
		assert.Empty(t, result.Error)
	})

	t.Run("explicitly failed response maps to a failed AIResult", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"conversation_id":"conv-err","status":"ERROR",` +
			`"response":"","message":"quota exceeded","duration_seconds":0.5,"num_turns":1}`)
		resp, err := parseAntigravityResponse(data)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.isSuccess())

		result := resp.toAIResult("stderr detail")
		assert.False(t, result.Success)
		assert.Equal(t, "quota exceeded", result.Error)
		assert.Equal(t, "conv-err", result.SessionID)
		assert.Equal(t, 1, result.NumTurns)
	})
}
