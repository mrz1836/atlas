package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiStreamEventParser_ParseLine(t *testing.T) {
	t.Parallel()

	parser := NewGeminiStreamEventParser()

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

	t.Run("parses init event", func(t *testing.T) {
		line := `{"type":"init","timestamp":"2026-01-21T14:44:36.452Z","session_id":"b5c2d098-0c15-476f-a9ba-c006c099c266","model":"auto-gemini-3"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "init", event.Type)
		assert.Equal(t, "b5c2d098-0c15-476f-a9ba-c006c099c266", event.SessionID)
		assert.Equal(t, "auto-gemini-3", event.Model)
		assert.Equal(t, "2026-01-21T14:44:36.452Z", event.Timestamp)
	})

	t.Run("parses tool_use event", func(t *testing.T) {
		line := `{"type":"tool_use","timestamp":"2026-01-21T14:44:55.963Z","tool_name":"read_file","tool_id":"read_file-1769006695963-68746c54586a2","parameters":{"offset":0,"limit":1,"file_path":"internal/ai/activity.go"}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "tool_use", event.Type)
		assert.Equal(t, "read_file", event.ToolName)
		assert.Equal(t, "read_file-1769006695963-68746c54586a2", event.ToolID)
		assert.NotEmpty(t, event.Parameters)
	})

	t.Run("parses result event", func(t *testing.T) {
		line := `{"type":"result","timestamp":"2026-01-21T14:44:57.289Z","status":"success","stats":{"total_tokens":16166,"input_tokens":15783,"output_tokens":124,"cached":3378,"input":12405,"duration_ms":5419,"tool_calls":1}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "result", event.Type)
		assert.Equal(t, "success", event.Status)
		require.NotNil(t, event.Stats)
		assert.Equal(t, 16166, event.Stats.TotalTokens)
		assert.Equal(t, 15783, event.Stats.InputTokens)
		assert.Equal(t, 124, event.Stats.OutputTokens)
		assert.Equal(t, 5419, event.Stats.DurationMs)
		assert.Equal(t, 1, event.Stats.ToolCalls)
	})

	t.Run("parses message event", func(t *testing.T) {
		line := `{"type":"message","timestamp":"2026-01-21T14:44:50.000Z","role":"assistant","content":"Let me read that file.","delta":true}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "message", event.Type)
		assert.Equal(t, "assistant", event.Role)
		assert.Equal(t, "Let me read that file.", event.Content)
		assert.True(t, event.Delta)
	})

	t.Run("captures session ID from init event", func(t *testing.T) {
		line := `{"type":"init","session_id":"test-session-123","model":"gemini-3"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "test-session-123", parser.sessionID)
	})
}

func TestGeminiStreamEventParser_ToActivityEvent(t *testing.T) {
	t.Parallel()

	parser := NewGeminiStreamEventParser()

	t.Run("returns nil for nil event", func(t *testing.T) {
		result := parser.ToActivityEvent(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil for non-tool events", func(t *testing.T) {
		event := &GeminiStreamEvent{Type: "message"}
		result := parser.ToActivityEvent(event)
		assert.Nil(t, result)
	})

	t.Run("returns nil for init events", func(t *testing.T) {
		event := &GeminiStreamEvent{Type: "init"}
		result := parser.ToActivityEvent(event)
		assert.Nil(t, result)
	})

	t.Run("returns nil for result events", func(t *testing.T) {
		event := &GeminiStreamEvent{Type: "result"}
		result := parser.ToActivityEvent(event)
		assert.Nil(t, result)
	})

	t.Run("maps read_file tool to reading activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "read_file",
			Parameters: []byte(`{"file_path":"internal/ai/parser.go"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Equal(t, "internal/ai/parser.go", activity.File)
	})

	t.Run("maps edit_file tool to writing activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "edit_file",
			Parameters: []byte(`{"file_path":"main.go"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityWriting, activity.Type)
		assert.Equal(t, "Editing", activity.Message)
		assert.Equal(t, "main.go", activity.File)
	})

	t.Run("maps write_file tool to writing activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "write_file",
			Parameters: []byte(`{"file_path":"new_file.go"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityWriting, activity.Type)
		assert.Equal(t, "Writing", activity.Message)
		assert.Equal(t, "new_file.go", activity.File)
	})

	t.Run("maps shell tool to executing activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "shell",
			Parameters: []byte(`{"command":"go test ./..."}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Equal(t, "Executing", activity.Message)
		assert.Equal(t, "go test ./...", activity.File)
	})

	t.Run("maps run_shell_command tool to executing activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "run_shell_command",
			Parameters: []byte(`{"command":"npm install"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Equal(t, "Executing", activity.Message)
		assert.Equal(t, "npm install", activity.File)
	})

	t.Run("truncates long commands", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "shell",
			Parameters: []byte(`{"command":"very-long-command-that-exceeds-fifty-characters-and-should-be-truncated"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Len(t, activity.File, 50)
		assert.Contains(t, activity.File, "...")
	})

	t.Run("maps search_files tool to searching activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "search_files",
			Parameters: []byte(`{"pattern":"func Test"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Searching", activity.Message)
		assert.Equal(t, "func Test", activity.File)
	})

	t.Run("maps find_files tool to searching activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "find_files",
			Parameters: []byte(`{"pattern":"**/*.go"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Finding files", activity.Message)
		assert.Equal(t, "**/*.go", activity.File)
	})

	t.Run("maps grep_search tool to searching activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "grep_search",
			Parameters: []byte(`{"query":"TODO"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Searching", activity.Message)
		assert.Equal(t, "TODO", activity.File)
	})

	t.Run("maps web_search tool to searching activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "web_search",
			Parameters: []byte(`{"query":"golang error handling"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Searching web", activity.Message)
		assert.Equal(t, "golang error handling", activity.File)
	})

	t.Run("maps google_web_search tool to searching activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "google_web_search",
			Parameters: []byte(`{"query":"best practices"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivitySearching, activity.Type)
		assert.Equal(t, "Searching web", activity.Message)
		assert.Equal(t, "best practices", activity.File)
	})

	t.Run("maps fetch_url tool to reading activity", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "fetch_url",
			Parameters: []byte(`{"url":"https://example.com"}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Fetching", activity.Message)
	})

	t.Run("handles unknown tool as analyzing", func(t *testing.T) {
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "UnknownTool",
			Parameters: []byte(`{}`),
		}

		activity := parser.ToActivityEvent(event)

		require.NotNil(t, activity)
		assert.Equal(t, ActivityAnalyzing, activity.Type)
		assert.Equal(t, "Using UnknownTool", activity.Message)
	})
}

func TestGeminiStreamEventParser_IsResultEvent(t *testing.T) {
	t.Parallel()

	parser := NewGeminiStreamEventParser()

	t.Run("returns false for nil", func(t *testing.T) {
		assert.False(t, parser.IsResultEvent(nil))
	})

	t.Run("returns true for result event", func(t *testing.T) {
		event := &GeminiStreamEvent{Type: "result"}
		assert.True(t, parser.IsResultEvent(event))
	})

	t.Run("returns false for non-result event", func(t *testing.T) {
		event := &GeminiStreamEvent{Type: "tool_use"}
		assert.False(t, parser.IsResultEvent(event))
	})
}

func TestGeminiStreamEventParser_ToGeminiResult(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for nil event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		result := parser.ToGeminiResult(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil for non-result event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{Type: "tool_use"}
		result := parser.ToGeminiResult(event)
		assert.Nil(t, result)
	})

	t.Run("converts success result event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		// First capture session ID via init
		parser.ParseLine(`{"type":"init","session_id":"test-session"}`)

		event := &GeminiStreamEvent{
			Type:   "result",
			Status: "success",
			Stats: &GeminiStats{
				TotalTokens:  16166,
				InputTokens:  15783,
				OutputTokens: 124,
				DurationMs:   5419,
				ToolCalls:    3,
			},
		}

		result := parser.ToGeminiResult(event)

		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "test-session", result.SessionID)
		assert.Equal(t, 5419, result.DurationMs)
		assert.Equal(t, 16166, result.TotalTokens)
		assert.Equal(t, 15783, result.InputTokens)
		assert.Equal(t, 124, result.OutputTokens)
		assert.Equal(t, 3, result.ToolCalls)
	})

	t.Run("converts error result event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:   "result",
			Status: "error",
		}

		result := parser.ToGeminiResult(event)

		require.NotNil(t, result)
		assert.False(t, result.Success)
	})

	t.Run("handles result with no stats", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:   "result",
			Status: "success",
			Stats:  nil,
		}

		result := parser.ToGeminiResult(event)

		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.DurationMs)
		assert.Equal(t, 0, result.TotalTokens)
	})
}

func TestNewGeminiStreamEventParser(t *testing.T) {
	t.Parallel()

	parser := NewGeminiStreamEventParser()

	require.NotNil(t, parser)
	require.NotNil(t, parser.toolToActivity)

	// Verify key mappings exist
	assert.Equal(t, ActivityReading, parser.toolToActivity["read_file"])
	assert.Equal(t, ActivityWriting, parser.toolToActivity["edit_file"])
	assert.Equal(t, ActivityWriting, parser.toolToActivity["write_file"])
	assert.Equal(t, ActivityExecuting, parser.toolToActivity["shell"])
	assert.Equal(t, ActivityExecuting, parser.toolToActivity["run_shell_command"])
	assert.Equal(t, ActivitySearching, parser.toolToActivity["search_files"])
	assert.Equal(t, ActivitySearching, parser.toolToActivity["find_files"])
}

// TestGeminiStreamEventParser_RealisticOutput tests with actual Gemini CLI output format.
func TestGeminiStreamEventParser_RealisticOutput(t *testing.T) {
	t.Parallel()

	t.Run("parses realistic init event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		line := `{"type":"init","timestamp":"2026-01-21T14:44:36.452Z","session_id":"b5c2d098-0c15-476f-a9ba-c006c099c266","model":"auto-gemini-3"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "init", event.Type)
		assert.Equal(t, "b5c2d098-0c15-476f-a9ba-c006c099c266", event.SessionID)
		assert.Equal(t, "auto-gemini-3", event.Model)
		// Should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic user message event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		line := `{"type":"message","timestamp":"2026-01-21T14:44:36.517Z","role":"user","content":"What version of Go is this project using?"}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "message", event.Type)
		assert.Equal(t, "user", event.Role)
		assert.Contains(t, event.Content, "Go")
		// Should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic assistant message with delta", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		line := `{"type":"message","timestamp":"2026-01-21T14:44:38.123Z","role":"assistant","content":"Let me check the go.mod file.","delta":true}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "message", event.Type)
		assert.Equal(t, "assistant", event.Role)
		assert.True(t, event.Delta)
		// Should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic tool_use event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		line := `{"type":"tool_use","timestamp":"2026-01-21T14:44:55.963Z","tool_name":"read_file","tool_id":"read_file-1769006695963-68746c54586a2","parameters":{"offset":0,"limit":1,"file_path":"internal/ai/activity.go"}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "tool_use", event.Type)
		assert.Equal(t, "read_file", event.ToolName)
		assert.Equal(t, "read_file-1769006695963-68746c54586a2", event.ToolID)

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Equal(t, "internal/ai/activity.go", activity.File)
	})

	t.Run("parses realistic tool_result event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		line := `{"type":"tool_result","timestamp":"2026-01-21T14:44:56.123Z","tool_id":"read_file-1769006695963-68746c54586a2","status":"success","output":"package ai\n\nimport..."}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "tool_result", event.Type)
		assert.Equal(t, "read_file-1769006695963-68746c54586a2", event.ToolID)
		assert.Equal(t, "success", event.Status)
		assert.Contains(t, event.Output, "package ai")
		// Tool results should not produce activity
		activity := parser.ToActivityEvent(event)
		assert.Nil(t, activity)
	})

	t.Run("parses realistic result event", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		// First set session ID
		parser.ParseLine(`{"type":"init","session_id":"b5c2d098-0c15-476f-a9ba-c006c099c266"}`)

		line := `{"type":"result","timestamp":"2026-01-21T14:44:57.289Z","status":"success","stats":{"total_tokens":16166,"input_tokens":15783,"output_tokens":124,"cached":3378,"input":12405,"duration_ms":5419,"tool_calls":1}}`

		event := parser.ParseLine(line)

		require.NotNil(t, event)
		assert.Equal(t, "result", event.Type)
		assert.Equal(t, "success", event.Status)
		require.NotNil(t, event.Stats)
		assert.Equal(t, 16166, event.Stats.TotalTokens)
		assert.Equal(t, 5419, event.Stats.DurationMs)
		assert.Equal(t, 1, event.Stats.ToolCalls)

		// Convert to result
		result := parser.ToGeminiResult(event)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "b5c2d098-0c15-476f-a9ba-c006c099c266", result.SessionID)
		assert.Equal(t, 5419, result.DurationMs)
	})

	t.Run("parses full realistic session flow", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()

		// Simulate a full session
		lines := []string{
			`{"type":"init","timestamp":"2026-01-21T14:44:36.452Z","session_id":"b5c2d098-0c15-476f-a9ba-c006c099c266","model":"auto-gemini-3"}`,
			`{"type":"message","timestamp":"2026-01-21T14:44:36.517Z","role":"user","content":"What version of Go?"}`,
			`{"type":"message","timestamp":"2026-01-21T14:44:38.123Z","role":"assistant","content":"Let me check.","delta":true}`,
			`{"type":"tool_use","timestamp":"2026-01-21T14:44:55.963Z","tool_name":"read_file","tool_id":"read-1","parameters":{"file_path":"go.mod"}}`,
			`{"type":"tool_result","timestamp":"2026-01-21T14:44:56.123Z","tool_id":"read-1","status":"success","output":"module example\n\ngo 1.24"}`,
			`{"type":"message","timestamp":"2026-01-21T14:44:56.500Z","role":"assistant","content":"Go 1.24","delta":true}`,
			`{"type":"result","timestamp":"2026-01-21T14:44:57.289Z","status":"success","stats":{"total_tokens":1000,"input_tokens":800,"output_tokens":200,"duration_ms":5000,"tool_calls":1}}`,
		}

		var activities []*ActivityEvent
		var result *GeminiStreamResult

		for _, line := range lines {
			event := parser.ParseLine(line)
			if event == nil {
				continue
			}

			if activity := parser.ToActivityEvent(event); activity != nil {
				activities = append(activities, activity)
			}

			if parser.IsResultEvent(event) {
				result = parser.ToGeminiResult(event)
			}
		}

		// Should have 1 activity (read_file)
		require.Len(t, activities, 1)
		assert.Equal(t, ActivityReading, activities[0].Type)
		assert.Equal(t, "go.mod", activities[0].File)

		// Should have result
		require.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "b5c2d098-0c15-476f-a9ba-c006c099c266", result.SessionID)
		assert.Equal(t, 5000, result.DurationMs)
		assert.Equal(t, 1, result.ToolCalls)
	})
}

// TestGeminiStreamEventParser_EdgeCases tests edge cases and error handling.
func TestGeminiStreamEventParser_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("handles malformed parameters JSON in tool", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "read_file",
			Parameters: []byte(`{invalid json`),
		}

		// Should still create activity, just without file info
		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Equal(t, "Reading", activity.Message)
		assert.Empty(t, activity.File)
	})

	t.Run("handles empty parameters in tool", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "shell",
			Parameters: []byte(`{}`),
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityExecuting, activity.Type)
		assert.Equal(t, "Executing", activity.Message)
		assert.Empty(t, activity.File)
	})

	t.Run("handles nil parameters in tool", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "read_file",
			Parameters: nil,
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, ActivityReading, activity.Type)
		assert.Empty(t, activity.File)
	})

	t.Run("handles search with query parameter", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "search_files",
			Parameters: []byte(`{"query":"TODO fixme"}`),
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, "TODO fixme", activity.File)
	})

	t.Run("prefers pattern over query for search", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		event := &GeminiStreamEvent{
			Type:       "tool_use",
			ToolName:   "search_files",
			Parameters: []byte(`{"pattern":"func.*Test","query":"tests"}`),
		}

		activity := parser.ToActivityEvent(event)
		require.NotNil(t, activity)
		assert.Equal(t, "func.*Test", activity.File)
	})

	t.Run("handles session ID not set", func(t *testing.T) {
		parser := NewGeminiStreamEventParser()
		// Don't send init event, so session ID is empty

		event := &GeminiStreamEvent{
			Type:   "result",
			Status: "success",
			Stats: &GeminiStats{
				DurationMs: 1000,
			},
		}

		result := parser.ToGeminiResult(event)
		require.NotNil(t, result)
		assert.Empty(t, result.SessionID)
	})
}
