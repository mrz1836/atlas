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

// agentConfig holds all configuration for an agent.
// Adding a new agent only requires adding a single entry to agentConfigs.
type agentConfig struct {
	model     string   // default model alias (e.g., "sonnet", "flash")
	apiKeyEnv string   // environment variable for API key
	hint      string   // CLI installation hint
	tool      string   // CLI command name
	aliases   []string // valid short model aliases
	// resolution maps short aliases to full model names.
	// Model names change frequently. Check current models at:
	// - Claude: https://platform.claude.com/docs/en/about-claude/models/overview
	// - Gemini: https://ai.google.dev/gemini-api/docs/models
	// - Codex: https://developers.openai.com/codex/models/
	resolution map[string]string
}

// agentConfigs is the central configuration for all supported agents.
// Adding a new agent only requires adding an entry here - all methods use this lookup.
var agentConfigs = map[Agent]agentConfig{ //nolint:gochecknoglobals // Central config lookup
	AgentClaude: { //nolint:gosec // G101: apiKeyEnv stores env var names, not hardcoded credentials
		model:     "sonnet",
		apiKeyEnv: "ANTHROPIC_API_KEY",
		hint:      "Install Claude CLI: npm install -g @anthropic-ai/claude-code",
		tool:      "claude",
		aliases:   []string{"sonnet", "opus", "haiku"},
		resolution: map[string]string{
			"sonnet": "claude-sonnet-4-20250514",
			"opus":   "claude-opus-4-20250514",
			"haiku":  "claude-haiku-3-20250514",
		},
	},
	AgentGemini: { //nolint:gosec // G101: apiKeyEnv stores env var names, not hardcoded credentials
		model:     "flash",
		apiKeyEnv: "GEMINI_API_KEY",
		hint:      "Install Gemini CLI: npm install -g @google/gemini-cli",
		tool:      "gemini",
		aliases:   []string{"flash", "pro"},
		resolution: map[string]string{
			"flash": "gemini-3-flash-preview",
			"pro":   "gemini-3-pro-preview",
		},
	},
	AgentCodex: { //nolint:gosec // G101: apiKeyEnv stores env var names, not hardcoded credentials
		model:     "codex",
		apiKeyEnv: "OPENAI_API_KEY",
		hint:      "Install Codex CLI: npm install -g @openai/codex",
		tool:      "codex",
		aliases:   []string{"codex", "max", "mini"},
		resolution: map[string]string{
			"codex": "gpt-5.2-codex",
			"max":   "gpt-5.1-codex-max",
			"mini":  "gpt-5.1-codex-mini",
		},
	},
}

// String returns the string representation of the Agent.
// This implements fmt.Stringer for convenient logging and debugging.
func (a Agent) String() string {
	return string(a)
}

// IsValid checks if the agent is a recognized type.
func (a Agent) IsValid() bool {
	_, ok := a.config()
	return ok
}

// DefaultModel returns the default model alias for this agent.
func (a Agent) DefaultModel() string {
	if cfg, ok := a.config(); ok {
		return cfg.model
	}
	return ""
}

// ModelAliases returns the valid short model aliases for this agent.
func (a Agent) ModelAliases() []string {
	if cfg, ok := a.config(); ok {
		return cfg.aliases
	}
	return nil
}

// ResolveModelAlias converts a short model alias to the full model name.
// If the alias is not recognized, it returns the input unchanged (allowing full model names).
func (a Agent) ResolveModelAlias(alias string) string {
	if cfg, ok := a.config(); ok {
		if fullName, found := cfg.resolution[alias]; found {
			return fullName
		}
	}
	// Return as-is if not an alias (might be a full model name)
	return alias
}

// APIKeyEnvVar returns the default environment variable name for the API key.
func (a Agent) APIKeyEnvVar() string {
	if cfg, ok := a.config(); ok {
		return cfg.apiKeyEnv
	}
	return ""
}

// InstallHint returns the installation instructions for this agent's CLI.
func (a Agent) InstallHint() string {
	if cfg, ok := a.config(); ok {
		return cfg.hint
	}
	return "Unknown agent"
}

// ToolName returns the CLI command name for this agent.
func (a Agent) ToolName() string {
	if cfg, ok := a.config(); ok {
		return cfg.tool
	}
	return ""
}

// config returns the configuration for this agent.
// Returns the config and true if found, or zero value and false if not.
func (a Agent) config() (agentConfig, bool) {
	cfg, ok := agentConfigs[a]
	return cfg, ok
}
