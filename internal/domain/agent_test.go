package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgent_String(t *testing.T) {
	t.Run("returns string representation for claude", func(t *testing.T) {
		assert.Equal(t, "claude", AgentClaude.String())
	})

	t.Run("returns string representation for gemini", func(t *testing.T) {
		assert.Equal(t, "gemini", AgentGemini.String())
	})

	t.Run("returns string representation for codex", func(t *testing.T) {
		assert.Equal(t, "codex", AgentCodex.String())
	})

	t.Run("returns string representation for antigravity", func(t *testing.T) {
		assert.Equal(t, "antigravity", AgentAntigravity.String())
	})

	t.Run("returns empty string for empty agent", func(t *testing.T) {
		var a Agent
		assert.Empty(t, a.String())
	})
}

func TestAgent_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{"claude is valid", AgentClaude, true},
		{"gemini is valid", AgentGemini, true},
		{"codex is valid", AgentCodex, true},
		{"antigravity is valid", AgentAntigravity, true},
		{"empty is invalid", Agent(""), false},
		{"unknown is invalid", Agent("unknown"), false},
		{"gpt is invalid", Agent("gpt"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgent_DefaultModel(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{"claude default is sonnet", AgentClaude, "sonnet"},
		{"gemini default is flash", AgentGemini, "flash"},
		{"codex default is codex", AgentCodex, "codex"},
		{"antigravity default is pro", AgentAntigravity, "pro"},
		{"empty agent has no default", Agent(""), ""},
		{"unknown agent has no default", Agent("unknown"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.DefaultModel()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgent_ModelAliases(t *testing.T) {
	t.Run("claude model aliases", func(t *testing.T) {
		aliases := AgentClaude.ModelAliases()
		assert.ElementsMatch(t, []string{"sonnet", "opus", "haiku"}, aliases)
	})

	t.Run("gemini model aliases", func(t *testing.T) {
		aliases := AgentGemini.ModelAliases()
		assert.ElementsMatch(t, []string{"flash", "pro"}, aliases)
	})

	t.Run("codex model aliases", func(t *testing.T) {
		aliases := AgentCodex.ModelAliases()
		assert.ElementsMatch(t, []string{"codex", "max", "mini"}, aliases)
	})

	t.Run("antigravity model aliases", func(t *testing.T) {
		aliases := AgentAntigravity.ModelAliases()
		assert.ElementsMatch(t, []string{"pro", "flash"}, aliases)
	})

	t.Run("empty agent has no aliases", func(t *testing.T) {
		var a Agent
		assert.Nil(t, a.ModelAliases())
	})

	t.Run("unknown agent has no aliases", func(t *testing.T) {
		a := Agent("unknown")
		assert.Nil(t, a.ModelAliases())
	})
}

func TestAgent_ResolveModelAlias(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		alias string
		want  string
	}{
		// Claude aliases
		{"claude sonnet resolves", AgentClaude, "sonnet", "claude-sonnet-5"},
		{"claude opus resolves", AgentClaude, "opus", "claude-opus-5"},
		{"claude haiku resolves", AgentClaude, "haiku", "claude-haiku-4-5-20251001"},
		{"claude full name passes through", AgentClaude, "claude-sonnet-5", "claude-sonnet-5"},
		{"claude unknown passes through", AgentClaude, "custom-model", "custom-model"},

		// Gemini aliases
		{"gemini flash resolves", AgentGemini, "flash", "gemini-3-flash-preview"},
		{"gemini pro resolves", AgentGemini, "pro", "gemini-3-pro-preview"},
		{"gemini full name passes through", AgentGemini, "gemini-3-flash-preview", "gemini-3-flash-preview"},
		{"gemini unknown passes through", AgentGemini, "custom-model", "custom-model"},

		// Codex aliases
		{"codex codex resolves", AgentCodex, "codex", "gpt-5.4"},
		{"codex max resolves", AgentCodex, "max", "gpt-5.5"},
		{"codex mini resolves", AgentCodex, "mini", "gpt-5.4-mini"},
		{"codex full name passes through", AgentCodex, "gpt-5.4", "gpt-5.4"},
		{"codex unknown passes through", AgentCodex, "custom-model", "custom-model"},

		// Antigravity aliases
		{"antigravity pro resolves", AgentAntigravity, "pro", "gemini-3.1-pro-high"},
		{"antigravity flash resolves", AgentAntigravity, "flash", "gemini-3.7-flash-medium"},
		{"antigravity full name passes through", AgentAntigravity, "claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"antigravity unknown passes through", AgentAntigravity, "custom-model", "custom-model"},

		// Unknown agent
		{"unknown agent passes through", Agent("unknown"), "sonnet", "sonnet"},
		{"empty agent passes through", Agent(""), "flash", "flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.ResolveModelAlias(tt.alias)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgent_APIKeyEnvVar(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{"claude api key env var", AgentClaude, "ANTHROPIC_API_KEY"},
		{"gemini api key env var", AgentGemini, "GEMINI_API_KEY"},
		{"codex api key env var", AgentCodex, "OPENAI_API_KEY"},
		{"antigravity has no env var (OAuth)", AgentAntigravity, ""},
		{"empty agent has no env var", Agent(""), ""},
		{"unknown agent has no env var", Agent("unknown"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.APIKeyEnvVar()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgent_InstallHint(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{"claude install hint", AgentClaude, "Install Claude CLI: npm install -g @anthropic-ai/claude-code"},
		{"gemini install hint", AgentGemini, "Install Gemini CLI: npm install -g @google/gemini-cli"},
		{"codex install hint", AgentCodex, "Install Codex CLI: npm install -g @openai/codex"},
		{"antigravity install hint", AgentAntigravity, "Install Antigravity CLI: curl -fsSL https://antigravity.google/cli/install.sh | bash"},
		{"unknown agent install hint", Agent("unknown"), "Unknown agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.InstallHint()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAgent_ToolName(t *testing.T) {
	tests := []struct {
		name  string
		agent Agent
		want  string
	}{
		{"claude tool name", AgentClaude, "claude"},
		{"gemini tool name", AgentGemini, "gemini"},
		{"codex tool name", AgentCodex, "codex"},
		{"antigravity tool name is agy", AgentAntigravity, "agy"},
		{"empty agent has no tool name", Agent(""), ""},
		{"unknown agent has no tool name", Agent("unknown"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agent.ToolName()
			assert.Equal(t, tt.want, got)
		})
	}
}
