// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

import "time"

// AIRequest contains the parameters for an AI execution request.
// This is passed to AIRunner implementations like ClaudeCodeRunner.
//
// Example JSON representation:
//
//	{
//	    "prompt": "Fix the null pointer in parseConfig",
//	    "context": "This is a Go project using...",
//	    "model": "claude-sonnet-4-20250514",
//	    "max_turns": 10,
//	    "timeout": "30m",
//	    "permission_mode": "plan",
//	    "working_dir": "/path/to/repo"
//	}
type AIRequest struct {
	// Agent specifies which AI CLI to use (claude, gemini).
	// If empty, defaults to "claude" for backwards compatibility.
	Agent Agent `json:"agent,omitempty"`

	// Prompt is the main instruction for the AI agent.
	Prompt string `json:"prompt"`

	// Context provides additional background information.
	Context string `json:"context,omitempty"`

	// Model specifies which AI model to use.
	Model string `json:"model"`

	// MaxTurns limits the number of conversation turns.
	// DEPRECATED: Not supported by Claude CLI.
	MaxTurns int `json:"max_turns,omitempty"`

	// MaxBudgetUSD limits AI spending for this request.
	MaxBudgetUSD float64 `json:"max_budget_usd,omitempty"`

	// Timeout is the maximum duration for the AI session.
	Timeout time.Duration `json:"timeout"`

	// PermissionMode controls AI permissions.
	// Empty string for default, "plan" for plan mode.
	PermissionMode string `json:"permission_mode"`

	// SystemPrompt overrides the default system prompt.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// WorkingDir is the directory where the AI will operate.
	WorkingDir string `json:"working_dir"`
}

// AIResult captures the outcome of an AI execution.
// This is returned by AIRunner implementations after execution.
//
// Example JSON representation:
//
//	{
//	    "success": true,
//	    "output": "I've fixed the null pointer...",
//	    "session_id": "sess-abc123",
//	    "duration_ms": 45000,
//	    "num_turns": 5,
//	    "total_cost_usd": 0.15,
//	    "files_changed": ["internal/config/parser.go"]
//	}
type AIResult struct {
	// Success indicates whether the AI completed without errors.
	Success bool `json:"success"`

	// Output contains the AI's response or summary.
	Output string `json:"output"`

	// SessionID identifies the AI session for debugging.
	SessionID string `json:"session_id"`

	// DurationMs is how long the AI session took in milliseconds.
	DurationMs int `json:"duration_ms"`

	// NumTurns is how many conversation turns occurred.
	NumTurns int `json:"num_turns"`

	// TotalCostUSD is the estimated cost of the AI session.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Error contains the error message if Success is false.
	Error string `json:"error,omitempty"`

	// FilesChanged lists paths of files that were created or modified.
	FilesChanged []string `json:"files_changed,omitempty"`
}
