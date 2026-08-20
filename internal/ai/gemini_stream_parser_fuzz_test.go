package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzGeminiStreamEventParser_ParseLine fuzzes ParseLine with arbitrary Gemini
// NDJSON input. It asserts the parser never panics and that a result event always
// yields a non-nil GeminiStreamResult.
func FuzzGeminiStreamEventParser_ParseLine(f *testing.F) {
	seeds := []string{
		"",
		"   \t\n",
		"not valid json",
		`{"type":"init","session_id":"test-session-123","model":"gemini-3"}`,
		`{"type":"tool_use","tool_name":"read_file","parameters":{"file_path":"internal/ai/activity.go"}}`,
		`{"type":"message","role":"assistant","content":"Let me read that file.","delta":true}`,
		`{"type":"result","status":"success","stats":{"total_tokens":16166,"duration_ms":5419,"tool_calls":1}}`,
		`{invalid`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, line string) {
		parser := NewGeminiStreamEventParser()
		event := parser.ParseLine(line)
		if event == nil {
			return
		}

		// Exercise downstream conversions; these must never panic.
		parser.ToActivityEvent(event)

		if parser.IsResultEvent(event) {
			assert.NotNil(t, parser.ToGeminiResult(event))
		}
	})
}
