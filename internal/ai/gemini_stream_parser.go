package ai

import (
	"encoding/json"
	"strings"
	"time"
)

// GeminiStreamEvent represents a single event from Gemini CLI's stream-json output.
// Gemini CLI outputs newline-delimited JSON (NDJSON) with various event types.
type GeminiStreamEvent struct {
	// Type indicates the event type (e.g., "init", "tool_use", "result", "message")
	Type string `json:"type"`

	// Timestamp of the event
	Timestamp string `json:"timestamp,omitempty"`

	// SessionID is set on init events
	SessionID string `json:"session_id,omitempty"`

	// Model is the model being used (set on init events)
	Model string `json:"model,omitempty"`

	// ToolName is the name of the tool being used (for tool_use events)
	ToolName string `json:"tool_name,omitempty"`

	// ToolID is a unique identifier for this tool invocation
	ToolID string `json:"tool_id,omitempty"`

	// Parameters contains the tool input parameters (for tool_use events)
	Parameters json.RawMessage `json:"parameters,omitempty"`

	// Status indicates result status: "success" or "error" (for result events)
	Status string `json:"status,omitempty"`

	// Stats contains execution statistics (for result events)
	Stats *GeminiStats `json:"stats,omitempty"`

	// Output contains tool result output (for tool_result events)
	Output string `json:"output,omitempty"`

	// Role is "user" or "assistant" (for message events)
	Role string `json:"role,omitempty"`

	// Content is the message content (for message events)
	Content string `json:"content,omitempty"`

	// Delta indicates if this is a streaming delta (for message events)
	Delta bool `json:"delta,omitempty"`
}

// GeminiStats contains execution statistics from a Gemini result event.
type GeminiStats struct {
	TotalTokens  int `json:"total_tokens"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	Cached       int `json:"cached,omitempty"`
	Input        int `json:"input,omitempty"`
	DurationMs   int `json:"duration_ms"`
	ToolCalls    int `json:"tool_calls"`
}

// GeminiStreamResult represents the final result extracted from streaming.
type GeminiStreamResult struct {
	Success      bool
	SessionID    string
	DurationMs   int
	TotalTokens  int
	InputTokens  int
	OutputTokens int
	ToolCalls    int
	ResponseText string // Accumulated assistant response text from message events
}

// GeminiToolParameters represents common tool parameter fields.
type GeminiToolParameters struct {
	// For read_file, edit_file, write_file tools
	FilePath string `json:"file_path,omitempty"`

	// For read_file tool
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`

	// For search_files, find_files tools
	Pattern string `json:"pattern,omitempty"`
	Query   string `json:"query,omitempty"`

	// For shell, run_shell_command tools
	Command string `json:"command,omitempty"`

	// For file content in edit/write
	Content string `json:"content,omitempty"`
}

// GeminiStreamEventParser parses streaming JSON events from Gemini CLI.
type GeminiStreamEventParser struct {
	// toolToActivity maps Gemini tool names to activity types.
	toolToActivity map[string]ActivityType

	// sessionID captured from init event
	sessionID string

	// responseBuilder accumulates assistant message content for response text
	responseBuilder strings.Builder
}

// NewGeminiStreamEventParser creates a new GeminiStreamEventParser.
func NewGeminiStreamEventParser() *GeminiStreamEventParser {
	return &GeminiStreamEventParser{
		toolToActivity: map[string]ActivityType{
			// Reading tools
			"read_file": ActivityReading,

			// Writing tools
			"edit_file":  ActivityWriting,
			"write_file": ActivityWriting,

			// Searching tools
			"search_files": ActivitySearching,
			"find_files":   ActivitySearching,
			"grep_search":  ActivitySearching,
			"file_search":  ActivitySearching,

			// Executing tools
			"shell":             ActivityExecuting,
			"run_shell_command": ActivityExecuting,
			"execute_command":   ActivityExecuting,

			// Web tools
			"web_search":        ActivitySearching,
			"fetch_url":         ActivityReading,
			"google_web_search": ActivitySearching,
		},
	}
}

// ParseLine parses a single line of NDJSON into a GeminiStreamEvent.
// Returns nil if the line is empty or cannot be parsed.
func (p *GeminiStreamEventParser) ParseLine(line string) *GeminiStreamEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var event GeminiStreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}

	// Capture session ID from init events
	if event.Type == "init" && event.SessionID != "" {
		p.sessionID = event.SessionID
	}

	// Accumulate assistant message content for response text
	if event.Type == "message" && event.Role == "assistant" && event.Content != "" {
		p.responseBuilder.WriteString(event.Content)
	}

	return &event
}

// ToActivityEvent converts a GeminiStreamEvent to an ActivityEvent if applicable.
// Returns nil if the event doesn't map to an activity (e.g., message, init).
func (p *GeminiStreamEventParser) ToActivityEvent(event *GeminiStreamEvent) *ActivityEvent {
	if event == nil {
		return nil
	}

	// Only handle tool_use events
	if event.Type != "tool_use" {
		return nil
	}

	return p.createToolActivity(event.ToolName, event.Parameters)
}

// IsResultEvent returns true if the event is the final result.
func (p *GeminiStreamEventParser) IsResultEvent(event *GeminiStreamEvent) bool {
	return event != nil && event.Type == "result"
}

// ToGeminiResult converts a result GeminiStreamEvent to a GeminiStreamResult.
func (p *GeminiStreamEventParser) ToGeminiResult(event *GeminiStreamEvent) *GeminiStreamResult {
	if event == nil || event.Type != "result" {
		return nil
	}

	result := &GeminiStreamResult{
		Success:      event.Status == "success",
		SessionID:    p.sessionID,
		ResponseText: p.responseBuilder.String(),
	}
	p.responseBuilder.Reset()

	if event.Stats != nil {
		result.DurationMs = event.Stats.DurationMs
		result.TotalTokens = event.Stats.TotalTokens
		result.InputTokens = event.Stats.InputTokens
		result.OutputTokens = event.Stats.OutputTokens
		result.ToolCalls = event.Stats.ToolCalls
	}

	return result
}

// createToolActivity creates an ActivityEvent for a tool use.
func (p *GeminiStreamEventParser) createToolActivity(toolName string, paramsJSON json.RawMessage) *ActivityEvent {
	actType, ok := p.toolToActivity[toolName]
	if !ok {
		// Unknown tool - treat as analyzing
		actType = ActivityAnalyzing
	}

	// Parse the parameters to extract relevant details
	var params GeminiToolParameters
	if len(paramsJSON) > 0 {
		_ = json.Unmarshal(paramsJSON, &params)
	}

	// Build the activity event
	activity := &ActivityEvent{
		Timestamp: time.Now(),
		Type:      actType,
	}

	// Set message and file based on tool type
	switch toolName {
	case "read_file":
		activity.Message = "Reading"
		activity.File = params.FilePath
	case "edit_file":
		activity.Message = "Editing"
		activity.File = params.FilePath
	case "write_file":
		activity.Message = "Writing"
		activity.File = params.FilePath
	case "search_files", "grep_search", "file_search":
		activity.Message = "Searching"
		if params.Pattern != "" {
			activity.File = params.Pattern
		} else if params.Query != "" {
			activity.File = params.Query
		}
	case "find_files":
		activity.Message = "Finding files"
		if params.Pattern != "" {
			activity.File = params.Pattern
		}
	case "shell", "run_shell_command", "execute_command":
		activity.Message = "Executing"
		if params.Command != "" {
			// Truncate long commands
			cmd := params.Command
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			activity.File = cmd
		}
	case "web_search", "google_web_search":
		activity.Message = "Searching web"
		if params.Query != "" {
			activity.File = params.Query
		}
	case "fetch_url":
		activity.Message = "Fetching"
		// URL would be in a different field, leave File empty for now
	default:
		activity.Message = "Using " + toolName
	}

	return activity
}
