package ai

import (
	"encoding/json"
	"strings"
	"time"
)

// StreamEvent represents a single event from Claude Code's stream-json output.
// Claude Code outputs newline-delimited JSON (NDJSON) with various event types.
type StreamEvent struct {
	// Type indicates the event type (e.g., "assistant", "result", "content_block_start", etc.)
	Type string `json:"type"`

	// Subtype provides additional type information for certain events.
	Subtype string `json:"subtype,omitempty"`

	// Message contains assistant message content for "assistant" type events.
	Message *StreamMessage `json:"message,omitempty"`

	// ContentBlock contains content block data for content_block events.
	ContentBlock *ContentBlock `json:"content_block,omitempty"`

	// Index is the content block index for delta events.
	Index int `json:"index,omitempty"`

	// ToolUse contains tool usage data when type is "tool_use".
	ToolUse *ToolUseEvent `json:"tool_use,omitempty"`

	// Result fields for final result event
	IsError      bool    `json:"is_error,omitempty"`
	Result       string  `json:"result,omitempty"`
	SessionID    string  `json:"session_id,omitempty"`
	DurationMs   int     `json:"duration_ms,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
}

// StreamMessage represents a message in the stream.
type StreamMessage struct {
	Content []ContentBlock `json:"content,omitempty"`
}

// ContentBlock represents a content block in a message.
type ContentBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	Text  string          `json:"text,omitempty"`
}

// ToolUseEvent represents a tool_use event in the stream.
type ToolUseEvent struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// ToolInput represents common tool input fields.
type ToolInput struct {
	// For Read, Edit, Write tools
	FilePath string `json:"file_path,omitempty"`

	// For Grep and Glob tools - both use "pattern" field
	Pattern string `json:"pattern,omitempty"`

	// For Grep tool - search path
	Path string `json:"path,omitempty"`

	// For Bash tool
	Command     string `json:"command,omitempty"`
	Description string `json:"description,omitempty"`

	// For Task tool
	Prompt       string `json:"prompt,omitempty"`
	SubagentType string `json:"subagent_type,omitempty"`
}

// StreamEventParser parses streaming JSON events from Claude Code.
type StreamEventParser struct {
	// toolToActivity maps tool names to activity types.
	toolToActivity map[string]ActivityType
}

// NewStreamEventParser creates a new StreamEventParser.
func NewStreamEventParser() *StreamEventParser {
	return &StreamEventParser{
		toolToActivity: map[string]ActivityType{
			// Reading tools
			"Read": ActivityReading,

			// Writing tools
			"Edit":         ActivityWriting,
			"Write":        ActivityWriting,
			"MultiEdit":    ActivityWriting,
			"NotebookEdit": ActivityWriting,

			// Searching tools
			"Grep": ActivitySearching,
			"Glob": ActivitySearching,

			// Executing tools
			"Bash": ActivityExecuting,

			// Analyzing tools (sub-agents)
			"Task": ActivityAnalyzing,

			// Web tools
			"WebFetch":  ActivityReading,
			"WebSearch": ActivitySearching,
		},
	}
}

// ParseLine parses a single line of NDJSON into a StreamEvent.
// Returns nil if the line is empty or cannot be parsed.
func (p *StreamEventParser) ParseLine(line string) *StreamEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var event StreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}

	return &event
}

// ToActivityEvent converts a StreamEvent to an ActivityEvent if applicable.
// Returns nil if the event doesn't map to an activity (e.g., thinking, text output).
func (p *StreamEventParser) ToActivityEvent(event *StreamEvent) *ActivityEvent {
	if event == nil {
		return nil
	}

	// Handle content_block_start events with tool_use
	if event.Type == "content_block_start" && event.ContentBlock != nil {
		if event.ContentBlock.Type == "tool_use" {
			return p.createToolActivity(event.ContentBlock.Name, event.ContentBlock.Input)
		}
	}

	// Handle assistant messages with tool_use content
	if event.Type == "assistant" && event.Message != nil {
		for _, block := range event.Message.Content {
			if block.Type == "tool_use" {
				return p.createToolActivity(block.Name, block.Input)
			}
		}
	}

	return nil
}

// IsResultEvent returns true if the event is the final result.
func (p *StreamEventParser) IsResultEvent(event *StreamEvent) bool {
	return event != nil && event.Type == "result"
}

// ToClaudeResponse converts a result StreamEvent to a ClaudeResponse.
func (p *StreamEventParser) ToClaudeResponse(event *StreamEvent) *ClaudeResponse {
	if event == nil || event.Type != "result" {
		return nil
	}

	return &ClaudeResponse{
		Type:      event.Type,
		Subtype:   event.Subtype,
		IsError:   event.IsError,
		Result:    event.Result,
		SessionID: event.SessionID,
		Duration:  event.DurationMs,
		NumTurns:  event.NumTurns,
		TotalCost: event.TotalCostUSD,
	}
}

// createToolActivity creates an ActivityEvent for a tool use.
func (p *StreamEventParser) createToolActivity(toolName string, inputJSON json.RawMessage) *ActivityEvent {
	actType, ok := p.toolToActivity[toolName]
	if !ok {
		// Unknown tool - treat as analyzing
		actType = ActivityAnalyzing
	}

	// Parse the input to extract relevant details
	var input ToolInput
	if len(inputJSON) > 0 {
		_ = json.Unmarshal(inputJSON, &input)
	}

	// Build the activity event
	activity := &ActivityEvent{
		Timestamp: time.Now(),
		Type:      actType,
	}

	// Set message and file based on tool type
	switch toolName {
	case "Read":
		activity.Message = "Reading"
		activity.File = input.FilePath
	case "Edit", "MultiEdit":
		activity.Message = "Editing"
		activity.File = input.FilePath
	case "Write":
		activity.Message = "Writing"
		activity.File = input.FilePath
	case "NotebookEdit":
		activity.Message = "Editing notebook"
		activity.File = input.FilePath
	case "Grep":
		activity.Message = "Searching"
		if input.Pattern != "" {
			activity.File = input.Pattern
		}
	case "Glob":
		activity.Message = "Finding files"
		if input.Pattern != "" {
			activity.File = input.Pattern
		}
	case "Bash":
		activity.Message = "Executing"
		if input.Description != "" {
			activity.File = input.Description
		} else if input.Command != "" {
			// Truncate long commands
			cmd := input.Command
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			activity.File = cmd
		}
	case "Task":
		activity.Message = "Running sub-agent"
		if input.SubagentType != "" {
			activity.File = input.SubagentType
		}
	case "WebFetch":
		activity.Message = "Fetching"
		if input.Path != "" {
			activity.File = input.Path
		}
	case "WebSearch":
		activity.Message = "Searching web"
		if input.Pattern != "" {
			activity.File = input.Pattern
		}
	default:
		activity.Message = "Using " + toolName
	}

	return activity
}
