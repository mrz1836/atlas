// Package domain provides shared domain types for the ATLAS task orchestration system.
package domain

// Agent represents an AI CLI agent type (e.g., "claude", "gemini").
// This determines which CLI tool is used to execute AI requests.
type Agent string

// Agent constants define the supported AI CLI agents.
const (
	// AgentClaude uses the Claude Code CLI from Anthropic.
	AgentClaude Agent = "claude"

	// AgentGemini uses the Gemini CLI from Google.
	AgentGemini Agent = "gemini"

	// AgentCodex uses the Codex CLI from OpenAI.
	AgentCodex Agent = "codex"
)

// String returns the string representation of the Agent.
// This implements fmt.Stringer for convenient logging and debugging.
func (a Agent) String() string {
	return string(a)
}

// IsValid checks if the agent is a recognized type.
func (a Agent) IsValid() bool {
	switch a {
	case AgentClaude, AgentGemini, AgentCodex:
		return true
	}
	return false
}

// DefaultModel returns the default model alias for this agent.
func (a Agent) DefaultModel() string {
	switch a {
	case AgentClaude:
		return "sonnet"
	case AgentGemini:
		return "flash"
	case AgentCodex:
		return "codex"
	default:
		return ""
	}
}

// ModelAliases returns the valid short model aliases for this agent.
func (a Agent) ModelAliases() []string {
	switch a {
	case AgentClaude:
		return []string{"sonnet", "opus", "haiku"}
	case AgentGemini:
		return []string{"flash", "pro"}
	case AgentCodex:
		return []string{"codex", "max", "mini"}
	default:
		return nil
	}
}

// ResolveModelAlias converts a short model alias to the full model name.
// If the alias is not recognized, it returns the input unchanged (allowing full model names).
//
// Model names change frequently. Check current models at:
// - Claude: https://platform.claude.com/docs/en/about-claude/models/overview
// - Gemini: https://ai.google.dev/gemini-api/docs/models
// - Codex: https://developers.openai.com/codex/models/
func (a Agent) ResolveModelAlias(alias string) string {
	switch a {
	case AgentClaude:
		// Claude models - check docs for latest versions
		// https://platform.claude.com/docs/en/about-claude/models/overview
		switch alias {
		case "sonnet":
			return "claude-sonnet-4-20250514"
		case "opus":
			return "claude-opus-4-20250514"
		case "haiku":
			return "claude-haiku-3-20250514"
		}
	case AgentGemini:
		// Gemini models - check docs for latest versions
		// https://ai.google.dev/gemini-api/docs/models
		switch alias {
		case "flash":
			return "gemini-3-flash-preview"
		case "pro":
			return "gemini-3-pro-preview"
		}
	case AgentCodex:
		// Codex models - check docs for latest versions
		// https://developers.openai.com/codex/models/
		switch alias {
		case "codex":
			return "gpt-5.2-codex"
		case "max":
			return "gpt-5.1-codex-max"
		case "mini":
			return "gpt-5.1-codex-mini"
		}
	}
	// Return as-is if not an alias (might be a full model name)
	return alias
}

// APIKeyEnvVar returns the default environment variable name for the API key.
func (a Agent) APIKeyEnvVar() string {
	switch a {
	case AgentClaude:
		return "ANTHROPIC_API_KEY"
	case AgentGemini:
		return "GEMINI_API_KEY"
	case AgentCodex:
		return "OPENAI_API_KEY"
	default:
		return ""
	}
}

// InstallHint returns the installation instructions for this agent's CLI.
func (a Agent) InstallHint() string {
	switch a {
	case AgentClaude:
		return "Install Claude CLI: npm install -g @anthropic-ai/claude-code"
	case AgentGemini:
		return "Install Gemini CLI: npm install -g @google/gemini-cli"
	case AgentCodex:
		return "Install Codex CLI: npm install -g @openai/codex"
	default:
		return "Unknown agent"
	}
}

// ToolName returns the CLI command name for this agent.
func (a Agent) ToolName() string {
	switch a {
	case AgentClaude:
		return "claude"
	case AgentGemini:
		return "gemini"
	case AgentCodex:
		return "codex"
	default:
		return ""
	}
}
