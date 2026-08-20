package ai

import (
	"strings"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// AntigravityResponse represents the JSON response from the Antigravity CLI (agy)
// when invoked with --output-format json.
//
// Example:
//
//	{"conversation_id":"a51...","status":"SUCCESS","response":"Hi there!\n",
//	 "duration_seconds":11.97,"num_turns":1,
//	 "usage":{"input_tokens":14187,"output_tokens":90,"total_tokens":14277}}
//
// agy's schema differs from Claude Code's (response/status/conversation_id
// vs result/is_error/session_id), so it uses its own struct.
type AntigravityResponse struct {
	// ConversationID identifies the agy session for debugging.
	ConversationID string `json:"conversation_id"`

	// Status is the terminal status of the run (e.g. "SUCCESS").
	Status string `json:"status"`

	// Response contains the AI's text response.
	Response string `json:"response"`

	// DurationSeconds is how long the run took, in seconds.
	DurationSeconds float64 `json:"duration_seconds"`

	// NumTurns is how many conversation turns occurred.
	NumTurns int `json:"num_turns"`

	// Usage reports token accounting for the run.
	Usage AntigravityUsage `json:"usage"`

	// Error and Message are best-effort captures of error payload fields; agy
	// does not document its failure schema, so both are optional.
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// AntigravityUsage reports token usage for an agy run.
type AntigravityUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ThinkingTokens  int `json:"thinking_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

// isSuccess reports whether the run completed successfully.
func (r *AntigravityResponse) isSuccess() bool {
	return strings.EqualFold(strings.TrimSpace(r.Status), "SUCCESS")
}

// toAIResult converts an AntigravityResponse to a domain.AIResult.
// agy reports token usage rather than a dollar cost, so TotalCostUSD is left at 0.
func (r *AntigravityResponse) toAIResult(stderr string) *domain.AIResult {
	success := r.isSuccess()
	result := &domain.AIResult{
		Success:    success,
		Output:     r.Response,
		SessionID:  r.ConversationID,
		DurationMs: int(r.DurationSeconds * 1000),
		NumTurns:   r.NumTurns,
	}

	if !success {
		// Prefer an explicit error field, then any message, then the response
		// body, then stderr, so failures always carry some context.
		for _, candidate := range []string{r.Error, r.Message, r.Response, stderr} {
			if msg := strings.TrimSpace(candidate); msg != "" {
				result.Error = msg
				break
			}
		}
	}

	return result
}

// parseAntigravityResponse parses the JSON output from the Antigravity CLI (agy).
// Returns an error wrapped with ErrAntigravityInvocation on parse failure.
func parseAntigravityResponse(data []byte) (*AntigravityResponse, error) {
	return parseResponse[AntigravityResponse](data, atlaserrors.ErrAntigravityInvocation)
}
