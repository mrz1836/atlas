package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamEventParser_ParseLine(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("parses empty line as nil", func(t *testing.T) {
		result := parser.ParseLine("")
		assert.Nil(t, result)
	})

	t.Run("parses whitespace-only line as nil", func(t *testing.T) {
		result := parser.ParseLine("   \t\n")
		assert.Nil(t, result)
	})

	t.Run("parses invalid JSON as nil", func(t *testing.T) {
		result := parser.ParseLine("not valid json")
		assert.Nil(t, result)
	})

	t.Run("parses result event", func(t *testing.T) {
		line := `{"type":"result","is_error":false,"result":"Task completed","session_id":"abc123","duration_ms":5000,"num_turns":3,"total_cost_usd":0.05}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "result", event.Type)
		assert.False(t, event.IsError)
		assert.Equal(t, "Task completed", event.Result)
		assert.Equal(t, "abc123", event.SessionID)
		assert.Equal(t, 5000, event.DurationMs)
		assert.Equal(t, 3, event.NumTurns)
		assert.InEpsilon(t, 0.05, event.TotalCostUSD, 0.0001)
	})

	t.Run("parses content_block_start with tool_use", func(t *testing.T) {
		line := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"main.go"}}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "content_block_start", event.Type)
		require.NotNil(t, event.ContentBlock)
		assert.Equal(t, "tool_use", event.ContentBlock.Type)
		assert.Equal(t, "Read", event.ContentBlock.Name)
	})

	t.Run("parses assistant message with content", func(t *testing.T) {
		line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"test.go"}}]}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "assistant", event.Type)
		require.NotNil(t, event.Message)
		require.Len(t, event.Message.Content, 1)
		assert.Equal(t, "tool_use", event.Message.Content[0].Type)
		assert.Equal(t, "Edit", event.Message.Content[0].Name)
	})
}

func TestStreamEventParser_ToActivityEvent(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("returns nil for nil event", func(t *testing.T) {
		result := parser.ToActivityEvent(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil for non-tool events", func(t *testing.T) {
		event := &StreamEvent{Type: "text"}
		result := parser.ToActivityEvent(event)
		assert.Nil(t, result)
	})

	t.Run("maps Read tool to reading activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Read",
				Input: []byte(`{"file_path":"internal/ai/parser.go"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Equal(t, "internal/ai/parser.go", activity.File)
	})

	t.Run("maps Edit tool to writing activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Edit",
				Input: []byte(`{"file_path":"main.go"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityWriting, activity.Type)
		assert.Equal(t, "Editing", activity.Message)
		assert.Equal(t, "main.go", activity.File)
	})

	t.Run("maps Write tool to writing activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Write",
				Input: []byte(`{"file_path":"new_file.go"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityWriting, activity.Type)
		assert.Equal(t, "Writing", activity.Message)
		assert.Equal(t, "new_file.go", activity.File)
	})

	t.Run("maps Bash tool to executing activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Bash",
				Input: []byte(`{"command":"go test ./...","description":"Run all tests"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Equal(t, "Executing", activity.Message)
		assert.Equal(t, "Run all tests", activity.File)
	})

	t.Run("maps Bash tool with long command truncates", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Bash",
				Input: []byte(`{"command":"very-long-command-that-exceeds-fifty-characters-and-should-be-truncated"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Len(t, activity.File, 50)
		assert.Contains(t, activity.File, "...")
	})

	t.Run("maps Grep tool to searching activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Grep",
				Input: []byte(`{"pattern":"func Test"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Searching", activity.Message)
		assert.Equal(t, "func Test", activity.File)
	})

	t.Run("maps Glob tool to searching activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Glob",
				Input: []byte(`{"pattern":"**/*.go"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Finding files", activity.Message)
		assert.Equal(t, "**/*.go", activity.File)
	})

	t.Run("maps Task tool to analyzing activity", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "Task",
				Input: []byte(`{"prompt":"Research the codebase","subagent_type":"Explore"}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityAnalyzing, activity.Type)
		assert.Equal(t, "Running sub-agent", activity.Message)
		assert.Equal(t, "Explore", activity.File)
	})

	t.Run("handles unknown tool as analyzing", func(t *testing.T) {
		event := &StreamEvent{
			Type: "content_block_start",
			ContentBlock: &ContentBlock{
				Type:  "tool_use",
				Name:  "UnknownTool",
				Input: []byte(`{}`),
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityAnalyzing, activity.Type)
		assert.Equal(t, "Using UnknownTool", activity.Message)
	})

	t.Run("handles assistant message with tool_use", func(t *testing.T) {
		event := &StreamEvent{
			Type: "assistant",
			Message: &StreamMessage{
				Content: []ContentBlock{
					{
						Type:  "tool_use",
						Name:  "Read",
						Input: []byte(`{"file_path":"config.yaml"}`),
					},
				},
			},
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Equal(t, "config.yaml", activity.File)
	})
}

func TestStreamEventParser_IsResultEvent(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("returns false for nil", func(t *testing.T) {
		assert.False(t, parser.IsResultEvent(nil))
	})

	t.Run("returns true for result event", func(t *testing.T) {
		event := &StreamEvent{Type: "result"}
		assert.True(t, parser.IsResultEvent(event))
	})

	t.Run("returns false for non-result event", func(t *testing.T) {
		event := &StreamEvent{Type: "assistant"}
		assert.False(t, parser.IsResultEvent(event))
	})
}

func TestStreamEventParser_ToClaudeResponse(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("returns nil for nil event", func(t *testing.T) {
		result := parser.ToClaudeResponse(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil for non-result event", func(t *testing.T) {
		event := &StreamEvent{Type: "assistant"}
		result := parser.ToClaudeResponse(event)
		assert.Nil(t, result)
	})

	t.Run("converts result event to ClaudeResponse", func(t *testing.T) {
		event := &StreamEvent{
			Type:         "result",
			Subtype:      "success",
			IsError:      false,
			Result:       "Task completed successfully",
			SessionID:    "session-abc",
			DurationMs:   10000,
			NumTurns:     5,
			TotalCostUSD: 0.15,
		}

		resp := parser.ToClaudeResponse(event)

		require.NotNil(t, resp)
		assert.Equal(t, "result", resp.Type)
		assert.Equal(t, "success", resp.Subtype)
		assert.False(t, resp.IsError)
		assert.Equal(t, "Task completed successfully", resp.Result)
		assert.Equal(t, "session-abc", resp.SessionID)
		assert.Equal(t, 10000, resp.Duration)
		assert.Equal(t, 5, resp.NumTurns)
		assert.InEpsilon(t, 0.15, resp.TotalCost, 0.0001)
	})

	t.Run("converts error result event", func(t *testing.T) {
		event := &StreamEvent{
			Type:    "result",
			IsError: true,
			Result:  "An error occurred",
		}

		resp := parser.ToClaudeResponse(event)

		require.NotNil(t, resp)
		assert.True(t, resp.IsError)
		assert.Equal(t, "An error occurred", resp.Result)
	})
}

func TestNewStreamEventParser(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	require.NotNil(t, parser)
	require.NotNil(t, parser.toolToActivity)

	// Verify key mappings exist
	assert.Equal(t, ActivityReading, parser.toolToActivity["Read"])
	assert.Equal(t, ActivityWriting, parser.toolToActivity["Edit"])
	assert.Equal(t, ActivityWriting, parser.toolToActivity["Write"])
	assert.Equal(t, ActivityExecuting, parser.toolToActivity["Bash"])
	assert.Equal(t, ActivitySearching, parser.toolToActivity["Grep"])
	assert.Equal(t, ActivitySearching, parser.toolToActivity["Glob"])
	assert.Equal(t, ActivityAnalyzing, parser.toolToActivity["Task"])
}

// TestStreamEventParser_RealisticClaudeOutput tests with actual Claude Code output format.
func TestStreamEventParser_RealisticClaudeOutput(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("parses realistic system init event", func(t *testing.T) {
		// Actual format from Claude Code
		line := `{"type":"system","subtype":"init","cwd":"/Users/test/project","session_id":"9cc32b77-5828-401f-aedc-f1f9dc248a3d","tools":["Task","Bash","Read","Edit","Write"],"model":"claude-opus-4-5-20251101"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "system", event.Type)
		assert.Equal(t, "init", event.Subtype)
		// System events should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic assistant message with Read tool", func(t *testing.T) {
		// Actual format from Claude Code
		line := `{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","id":"msg_01PWx4KDAKqK7ZUK1SB3HNAc","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01VaCfcDR4TS7dP471UNkCWd","name":"Read","input":{"file_path":"/Users/test/project/go.mod"}}],"stop_reason":null},"session_id":"9cc32b77-5828-401f-aedc-f1f9dc248a3d"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "assistant", event.Type)
		require.NotNil(t, event.Message)
		require.Len(t, event.Message.Content, 1)
		assert.Equal(t, "tool_use", event.Message.Content[0].Type)
		assert.Equal(t, "Read", event.Message.Content[0].Name)
		assert.Equal(t, "toolu_01VaCfcDR4TS7dP471UNkCWd", event.Message.Content[0].ID)

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Equal(t, "/Users/test/project/go.mod", activity.File)
	})

	t.Run("parses realistic user tool result event", func(t *testing.T) {
		// User events contain tool results - should not produce activity
		line := `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_01VaCfcDR4TS7dP471UNkCWd","type":"tool_result","content":"file contents here"}]},"session_id":"9cc32b77-5828-401f-aedc-f1f9dc248a3d"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "user", event.Type)
		// User events should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic result event", func(t *testing.T) {
		// Actual format from Claude Code
		line := `{"type":"result","subtype":"success","is_error":false,"duration_ms":5547,"duration_api_ms":5325,"num_turns":2,"result":"The project is using **Go 1.24.3** (line 3 of go.mod).","session_id":"9cc32b77-5828-401f-aedc-f1f9dc248a3d","total_cost_usd":0.06705775}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "result", event.Type)
		assert.Equal(t, "success", event.Subtype)
		assert.False(t, event.IsError)
		assert.Equal(t, 5547, event.DurationMs)
		assert.Equal(t, 2, event.NumTurns)
		assert.Contains(t, event.Result, "Go 1.24.3")
		assert.InEpsilon(t, 0.06705775, event.TotalCostUSD, 0.0001)

		// Convert to ClaudeResponse
		resp := parser.ToClaudeResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, "result", resp.Type)
		assert.False(t, resp.IsError)
		assert.Equal(t, 5547, resp.Duration)
	})

	t.Run("parses assistant text response without tools", func(t *testing.T) {
		// Sometimes assistant responds with just text, no tools
		line := `{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","content":[{"type":"text","text":"The project is using Go 1.24.3."}]},"session_id":"test123"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "assistant", event.Type)
		// Text-only responses should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("ignores system hook events", func(t *testing.T) {
		line := `{"type":"system","subtype":"hook_response","session_id":"test123","hook_name":"SessionStart:startup","exit_code":0}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "system", event.Type)
		assert.Equal(t, "hook_response", event.Subtype)
		// Hook events should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})
}

// TestStreamEventParser_EdgeCases tests edge cases and error handling.
func TestStreamEventParser_EdgeCases(t *testing.T) {
	t.Parallel()

	parser := NewStreamEventParser()

	t.Run("handles malformed input JSON in tool", func(t *testing.T) {
		event := &StreamEvent{
			Type: "assistant",
			Message: &StreamMessage{
				Content: []ContentBlock{
					{
						Type:  "tool_use",
						Name:  "Read",
						Input: []byte(`{invalid json`), // Malformed JSON
					},
				},
			},
		}

		// Should still create activity, just without file info
		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Empty(t, activity.File) // No file due to parse error
	})

	t.Run("handles empty input in tool", func(t *testing.T) {
		event := &StreamEvent{
			Type: "assistant",
			Message: &StreamMessage{
				Content: []ContentBlock{
					{
						Type:  "tool_use",
						Name:  "Bash",
						Input: []byte(`{}`), // Empty input
					},
				},
			},
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Equal(t, "Executing", activity.Message)
		assert.Empty(t, activity.File)
	})

	t.Run("handles multiple content blocks", func(t *testing.T) {
		event := &StreamEvent{
			Type: "assistant",
			Message: &StreamMessage{
				Content: []ContentBlock{
					{
						Type: "text",
						Text: "Let me read the file first.",
					},
					{
						Type:  "tool_use",
						Name:  "Read",
						Input: []byte(`{"file_path":"main.go"}`),
					},
				},
			},
		}

		// Should return activity for the first tool_use found
		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "main.go", activity.File)
	})

	t.Run("handles nil input in content block", func(t *testing.T) {
		event := &StreamEvent{
			Type: "assistant",
			Message: &StreamMessage{
				Content: []ContentBlock{
					{
						Type:  "tool_use",
						Name:  "Read",
						Input: nil, // Nil input
					},
				},
			},
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Empty(t, activity.File)
	})
}
