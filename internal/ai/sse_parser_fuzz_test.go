package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzStreamEventParser_ParseLine fuzzes ParseLine with arbitrary NDJSON input.
// It asserts the parser never panics and that any downstream conversions remain
// internally consistent (a result event always yields a non-nil ClaudeResponse).
func FuzzStreamEventParser_ParseLine(f *testing.F) {
	seeds := []string{
		"",
		"   \t\n",
		"not valid json",
		`{"type":"result","is_error":false,"result":"Task completed","session_id":"abc123","duration_ms":5000,"num_turns":3,"total_cost_usd":0.05}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"main.go"}}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"test.go"}}]}}`,
		`{invalid json`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, line string) {
		parser := NewStreamEventParser()
		event := parser.ParseLine(line)
		if event == nil {
			return
		}

		// Exercise downstream conversions; these must never panic.
		parser.ToActivityEvent(event)

		if parser.IsResultEvent(event) {
			assert.NotNil(t, parser.ToClaudeResponse(event))
		}
	})
}
