package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// codexEvent is a single JSONL event emitted by `codex exec --json`.
// Only the fields ATLAS consumes are modeled; unknown events/fields are ignored.
//
// Example stream:
//
//	{"type":"thread.started","thread_id":"01a0..."}
//	{"type":"turn.started"}
//	{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"OK"}}
//	{"type":"turn.completed","usage":{"input_tokens":14953,"output_tokens":16}}
type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"`
	Item     *codexItem  `json:"item"`
	Usage    *CodexUsage `json:"usage"`
	// Message carries top-level error events, e.g. {"type":"error","message":"..."}.
	Message string `json:"message"`
}

// codexItem is the payload of an item.completed event.
type codexItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`    // e.g. "agent_message", "error", "reasoning"
	Text    string `json:"text"`    // agent_message text
	Message string `json:"message"` // error item message
}

// CodexUsage reports token accounting from a turn.completed event.
type CodexUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

// CodexResponse is the aggregated result of a `codex exec --json` event stream.
type CodexResponse struct {
	// Success indicates the run produced a final agent message (or a completed
	// turn) with no fatal error.
	Success bool

	// Content is the final agent message text.
	Content string

	// SessionID is the codex thread id, useful for debugging.
	SessionID string

	// NumTurns counts turn.started events.
	NumTurns int

	// Usage reports token accounting from the final turn.completed event.
	Usage CodexUsage

	// Error holds a fatal error message when Success is false.
	Error string
}

// parseCodexResponse parses the JSONL event stream from `codex exec --json`.
// It aggregates the stream into a CodexResponse: the final agent_message becomes
// Content, thread.started supplies SessionID, turn.completed supplies Usage, and
// fatal errors (turn.failed, top-level error events, or a terminal error item)
// become Error. Non-fatal warning items (e.g. model-metadata fallbacks) that are
// followed by a real agent message do not fail the run.
func parseCodexResponse(data []byte) (*CodexResponse, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty response", atlaserrors.ErrCodexInvocation)
	}

	resp := &CodexResponse{}
	var (
		parsedAny        bool
		sawTurnCompleted bool
		fatalErr         string // turn.failed / top-level error event
		itemErr          string // last terminal error item message
	)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// codex can emit large events (reasoning, config); allow generous line sizes.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev codexEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // ignore malformed / partial lines
		}
		parsedAny = true

		switch ev.Type {
		case "thread.started":
			if ev.ThreadID != "" {
				resp.SessionID = ev.ThreadID
			}
		case "turn.started":
			resp.NumTurns++
		case "turn.completed":
			sawTurnCompleted = true
			if ev.Usage != nil {
				resp.Usage = *ev.Usage
			}
		case "turn.failed":
			if ev.Message != "" {
				fatalErr = ev.Message
			} else if ev.Item != nil && ev.Item.Message != "" {
				fatalErr = ev.Item.Message
			}
		case "error":
			if ev.Message != "" {
				fatalErr = ev.Message
			}
		case "item.completed":
			if ev.Item == nil {
				continue
			}
			switch ev.Item.Type {
			case "agent_message":
				if strings.TrimSpace(ev.Item.Text) != "" {
					resp.Content = ev.Item.Text // keep the final agent message
				}
			case "error":
				if ev.Item.Message != "" {
					itemErr = ev.Item.Message
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: failed to read codex json stream: %w", atlaserrors.ErrCodexInvocation, err)
	}
	if !parsedAny {
		return nil, fmt.Errorf("%w: no json events in response (%d bytes)", atlaserrors.ErrCodexInvocation, len(data))
	}

	switch {
	case resp.Content != "":
		// A final answer was produced; warning-level item errors are ignored.
		resp.Success = true
	case fatalErr != "":
		resp.Error = fatalErr
	case itemErr != "":
		resp.Error = itemErr
	case sawTurnCompleted:
		resp.Success = true
	default:
		resp.Error = "codex exec produced no agent message"
	}

	return resp, nil
}

// toAIResult converts a CodexResponse to a domain.AIResult.
// Codex reports token usage rather than a dollar cost, so TotalCostUSD is left at 0.
func (r *CodexResponse) toAIResult(stderr string) *domain.AIResult {
	result := &domain.AIResult{
		Success:   r.Success,
		Output:    r.Content,
		SessionID: r.SessionID,
		NumTurns:  r.NumTurns,
	}
	if !r.Success {
		switch {
		case r.Error != "":
			result.Error = r.Error
		case stderr != "":
			result.Error = stderr
		}
	}
	return result
}
