package ai

// This suite exercises the pure aggregation and mapping helpers in
// codex_response.go directly (no subprocess execution and no API calls).
// It complements codex_test.go, which drives the same code through the
// CodexRunner. All inputs are hand-built JSONL payloads that mirror the
// real `codex exec --json` event stream.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestParseCodexResponse_FullPayloadMapping verifies a complete, realistic
// event stream parses and that every resulting AIResult field is mapped.
func TestParseCodexResponse_FullPayloadMapping(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"thread.started","thread_id":"01a0-full"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"internal chain of thought"}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Final answer: café ☕ Ωμέγα наука"}}
{"type":"turn.completed","usage":{"input_tokens":14953,"cached_input_tokens":100,"output_tokens":16,"reasoning_output_tokens":8}}`)

	resp, err := parseCodexResponse(data)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Response-level aggregation.
	assert.True(t, resp.Success)
	assert.Equal(t, "Final answer: café ☕ Ωμέγα наука", resp.Content)
	assert.Equal(t, "01a0-full", resp.SessionID)
	assert.Equal(t, 1, resp.NumTurns)
	assert.Empty(t, resp.Error)
	assert.Equal(t, 14953, resp.Usage.InputTokens)
	assert.Equal(t, 100, resp.Usage.CachedInputTokens)
	assert.Equal(t, 16, resp.Usage.OutputTokens)
	assert.Equal(t, 8, resp.Usage.ReasoningOutputTokens)

	// Full AIResult field mapping. Codex reports tokens, not a dollar cost,
	// and never reports duration or a file list, so those stay zero-valued.
	result := resp.toAIResult("ignored stderr on success")
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "Final answer: café ☕ Ωμέγα наука", result.Output)
	assert.Equal(t, "01a0-full", result.SessionID)
	assert.Equal(t, 1, result.NumTurns)
	assert.Empty(t, result.Error)
	assert.Zero(t, result.DurationMs)
	assert.Zero(t, result.TotalCostUSD)
	assert.Nil(t, result.FilesChanged)
}

// TestCodexAggregate_StreamingSequence mirrors the production Run path: it
// folds a multi-turn event stream through apply/applyItem and asserts that
// finalize aggregates the final answer, the turn count, and the token usage
// from the last completed turn.
func TestCodexAggregate_StreamingSequence(t *testing.T) {
	t.Parallel()

	agg := &codexAggregate{resp: &CodexResponse{}}

	// Turn one: thread id, a turn, an intermediate answer, and usage.
	agg.apply(codexEvent{Type: "thread.started", ThreadID: "sess-agg"})
	agg.apply(codexEvent{Type: "turn.started"})
	agg.apply(codexEvent{Type: "item.completed", Item: &codexItem{Type: "agent_message", Text: "first draft"}})
	agg.apply(codexEvent{Type: "turn.completed", Usage: &CodexUsage{InputTokens: 10, OutputTokens: 2}})

	// Turn two: a second turn whose final answer supersedes the first and
	// whose usage overwrites the earlier turn's accounting.
	agg.apply(codexEvent{Type: "turn.started"})
	agg.apply(codexEvent{Type: "item.completed", Item: &codexItem{Type: "reasoning", Text: "more thinking"}})
	agg.apply(codexEvent{Type: "item.completed", Item: &codexItem{Type: "agent_message", Text: "final answer"}})
	agg.apply(codexEvent{Type: "turn.completed", Usage: &CodexUsage{InputTokens: 20, OutputTokens: 5}})

	resp := agg.finalize()
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "final answer", resp.Content)
	assert.Equal(t, "sess-agg", resp.SessionID)
	assert.Equal(t, 2, resp.NumTurns)
	// Usage reflects only the last turn.completed event.
	assert.Equal(t, 20, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	assert.Empty(t, resp.Error)
}

// TestCodexAggregate_Apply exercises each event branch of apply directly.
func TestCodexAggregate_Apply(t *testing.T) {
	t.Parallel()

	t.Run("thread.started sets session id", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "thread.started", ThreadID: "abc"})
		assert.Equal(t, "abc", agg.resp.SessionID)
	})

	t.Run("thread.started with empty id does not overwrite", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{SessionID: "keep"}}
		agg.apply(codexEvent{Type: "thread.started", ThreadID: ""})
		assert.Equal(t, "keep", agg.resp.SessionID)
	})

	t.Run("turn.started increments turn count", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "turn.started"})
		agg.apply(codexEvent{Type: "turn.started"})
		assert.Equal(t, 2, agg.resp.NumTurns)
	})

	t.Run("turn.completed records usage and completion", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "turn.completed", Usage: &CodexUsage{InputTokens: 7, OutputTokens: 3}})
		assert.True(t, agg.sawTurnCompleted)
		assert.Equal(t, 7, agg.resp.Usage.InputTokens)
		assert.Equal(t, 3, agg.resp.Usage.OutputTokens)
	})

	t.Run("turn.completed with nil usage still marks completion", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{Usage: CodexUsage{InputTokens: 99}}}
		agg.apply(codexEvent{Type: "turn.completed", Usage: nil})
		assert.True(t, agg.sawTurnCompleted)
		assert.Equal(t, 99, agg.resp.Usage.InputTokens) // unchanged
	})

	t.Run("turn.failed uses top-level message", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "turn.failed", Message: "rate limited"})
		assert.Equal(t, "rate limited", agg.fatalErr)
	})

	t.Run("turn.failed falls back to item message", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "turn.failed", Item: &codexItem{Message: "item-level failure"}})
		assert.Equal(t, "item-level failure", agg.fatalErr)
	})

	t.Run("error event records fatal error", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "error", Message: "stream error"})
		assert.Equal(t, "stream error", agg.fatalErr)
	})

	t.Run("unknown event type is ignored", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.apply(codexEvent{Type: "item.updated"})
		assert.Equal(t, &codexAggregate{resp: &CodexResponse{}}, agg)
	})
}

// TestCodexAggregate_ApplyItem exercises each item branch of applyItem.
func TestCodexAggregate_ApplyItem(t *testing.T) {
	t.Parallel()

	t.Run("nil item is a no-op", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{Content: "existing"}}
		agg.applyItem(nil)
		assert.Equal(t, "existing", agg.resp.Content)
		assert.Empty(t, agg.itemErr)
	})

	t.Run("agent_message sets content", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.applyItem(&codexItem{Type: "agent_message", Text: "hello"})
		assert.Equal(t, "hello", agg.resp.Content)
	})

	t.Run("whitespace-only agent_message does not overwrite content", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{Content: "keep"}}
		agg.applyItem(&codexItem{Type: "agent_message", Text: "   \n\t"})
		assert.Equal(t, "keep", agg.resp.Content)
	})

	t.Run("error item records item error", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.applyItem(&codexItem{Type: "error", Message: "disconnected"})
		assert.Equal(t, "disconnected", agg.itemErr)
	})

	t.Run("unknown item type is ignored", func(t *testing.T) {
		t.Parallel()
		agg := &codexAggregate{resp: &CodexResponse{}}
		agg.applyItem(&codexItem{Type: "reasoning", Text: "thoughts"})
		assert.Empty(t, agg.resp.Content)
		assert.Empty(t, agg.itemErr)
	})
}

// TestCodexAggregate_Finalize covers every resolution branch of finalize.
func TestCodexAggregate_Finalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		agg         *codexAggregate
		wantSuccess bool
		wantError   string
	}{
		{
			name:        "content wins even when a warning item error is present",
			agg:         &codexAggregate{resp: &CodexResponse{Content: "answer"}, itemErr: "non-fatal warning"},
			wantSuccess: true,
			wantError:   "",
		},
		{
			name:        "fatal error surfaces when no content",
			agg:         &codexAggregate{resp: &CodexResponse{}, fatalErr: "boom"},
			wantSuccess: false,
			wantError:   "boom",
		},
		{
			name:        "item error surfaces when no content or fatal error",
			agg:         &codexAggregate{resp: &CodexResponse{}, itemErr: "stream disconnected"},
			wantSuccess: false,
			wantError:   "stream disconnected",
		},
		{
			name:        "completed turn without content is a success",
			agg:         &codexAggregate{resp: &CodexResponse{}, sawTurnCompleted: true},
			wantSuccess: true,
			wantError:   "",
		},
		{
			name:        "empty stream yields a synthetic error",
			agg:         &codexAggregate{resp: &CodexResponse{}},
			wantSuccess: false,
			wantError:   "codex exec produced no agent message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := tt.agg.finalize()
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantSuccess, resp.Success)
			assert.Equal(t, tt.wantError, resp.Error)
		})
	}
}

// TestParseCodexResponse_ErrorsAndEdges covers Category F edge cases: empty
// input, malformed/skippable JSON, and an explicitly failed run flowing
// through toAIResult.
func TestParseCodexResponse_ErrorsAndEdges(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns wrapped error", func(t *testing.T) {
		t.Parallel()
		resp, err := parseCodexResponse([]byte(""))
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
		assert.ErrorContains(t, err, "empty response")
	})

	t.Run("non-json input returns wrapped error", func(t *testing.T) {
		t.Parallel()
		resp, err := parseCodexResponse([]byte("not json at all\njust text"))
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
		assert.ErrorContains(t, err, "no json events")
	})

	t.Run("lines that look like json but fail to parse are skipped", func(t *testing.T) {
		t.Parallel()
		// Every '{'-prefixed line is invalid JSON, so none parse and the
		// stream is treated as containing no events.
		resp, err := parseCodexResponse([]byte("{not valid\n{also: broken,}"))
		assert.Nil(t, resp)
		require.ErrorIs(t, err, atlaserrors.ErrCodexInvocation)
		assert.ErrorContains(t, err, "no json events")
	})

	t.Run("malformed lines are ignored around a valid answer", func(t *testing.T) {
		t.Parallel()
		data := []byte(`garbage prefix that is not json
{"type":"turn.started"}
{this line is broken json
{"type":"item.completed","item":{"type":"agent_message","text":"survived"}}`)
		resp, err := parseCodexResponse(data)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "survived", resp.Content)
		assert.Equal(t, 1, resp.NumTurns)
	})

	t.Run("explicitly failed run maps to a failed AIResult", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"type":"thread.started","thread_id":"sess-fail"}
{"type":"turn.started"}
{"type":"turn.failed","message":"API rate limit exceeded"}`)
		resp, err := parseCodexResponse(data)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "API rate limit exceeded", resp.Error)

		result := resp.toAIResult("")
		assert.False(t, result.Success)
		assert.Equal(t, "API rate limit exceeded", result.Error)
		assert.Equal(t, "sess-fail", result.SessionID)
		assert.Equal(t, 1, result.NumTurns)
	})

	t.Run("unicode content round-trips through parse", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"Ünïcödé — 🚀 Ωμέγα наука"}}
{"type":"turn.completed"}`)
		resp, err := parseCodexResponse(data)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, "Ünïcödé — 🚀 Ωμέγα наука", resp.Content)
	})
}

// TestCodexResponse_ToAIResult_FieldMapping asserts each toAIResult branch and
// that codex-specific zero-valued fields stay zero.
func TestCodexResponse_ToAIResult_FieldMapping(t *testing.T) {
	t.Parallel()

	t.Run("success ignores stderr and leaves error empty", func(t *testing.T) {
		t.Parallel()
		resp := &CodexResponse{
			Success:   true,
			Content:   "done",
			SessionID: "sess-ok",
			NumTurns:  3,
		}
		result := resp.toAIResult("noisy stderr")
		assert.True(t, result.Success)
		assert.Equal(t, "done", result.Output)
		assert.Equal(t, "sess-ok", result.SessionID)
		assert.Equal(t, 3, result.NumTurns)
		assert.Empty(t, result.Error)
		assert.Zero(t, result.DurationMs)
		assert.Zero(t, result.TotalCostUSD)
		assert.Nil(t, result.FilesChanged)
	})

	t.Run("failure prefers response error over stderr", func(t *testing.T) {
		t.Parallel()
		resp := &CodexResponse{Success: false, Error: "primary error"}
		result := resp.toAIResult("secondary stderr")
		assert.False(t, result.Success)
		assert.Equal(t, "primary error", result.Error)
	})

	t.Run("failure falls back to stderr when response error empty", func(t *testing.T) {
		t.Parallel()
		resp := &CodexResponse{Success: false}
		result := resp.toAIResult("stderr detail")
		assert.False(t, result.Success)
		assert.Equal(t, "stderr detail", result.Error)
	})

	t.Run("failure with no error and no stderr leaves error empty", func(t *testing.T) {
		t.Parallel()
		resp := &CodexResponse{Success: false}
		result := resp.toAIResult("")
		assert.False(t, result.Success)
		assert.Empty(t, result.Error)
	})
}
