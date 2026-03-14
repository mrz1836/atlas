package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// Test errors for fallback testing.
var (
	errAuthFallbackTest    = errors.New("authentication failed: invalid API key")
	errNetworkFallbackTest = errors.New("network timeout")
)

func TestDefaultFallbackConfig(t *testing.T) {
	t.Run("returns sensible defaults", func(t *testing.T) {
		cfg := DefaultFallbackConfig()

		require.NotNil(t, cfg)
		assert.True(t, cfg.Enabled)
		assert.Equal(t, 1, cfg.MaxRetriesPerModel)
		assert.Empty(t, cfg.AgentFallbackOrder)

		// Check model chains
		assert.Equal(t, []string{"haiku", "sonnet", "opus"}, cfg.ModelChains["claude"])
		assert.Equal(t, []string{"flash", "pro"}, cfg.ModelChains["gemini"])
		assert.Equal(t, []string{"mini", "codex", "max"}, cfg.ModelChains["codex"])
	})
}

func TestNewFallbackRunner(t *testing.T) {
	t.Run("creates runner with provided config", func(t *testing.T) {
		reg := NewRunnerRegistry()
		cfg := &FallbackConfig{
			Enabled:            true,
			MaxRetriesPerModel: 3,
		}

		runner := NewFallbackRunner(reg, cfg, zerolog.Nop())

		require.NotNil(t, runner)
		assert.Equal(t, reg, runner.registry)
		assert.Equal(t, cfg, runner.config)
	})

	t.Run("uses default config when nil", func(t *testing.T) {
		reg := NewRunnerRegistry()

		runner := NewFallbackRunner(reg, nil, zerolog.Nop())

		require.NotNil(t, runner)
		assert.True(t, runner.config.Enabled)
		assert.Equal(t, 1, runner.config.MaxRetriesPerModel)
	})

	t.Run("sets minimum retry count", func(t *testing.T) {
		reg := NewRunnerRegistry()
		cfg := &FallbackConfig{
			MaxRetriesPerModel: 0, // Invalid value
		}

		runner := NewFallbackRunner(reg, cfg, zerolog.Nop())

		assert.Equal(t, 1, runner.config.MaxRetriesPerModel)
	})
}

func TestFallbackRunner_Run(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		reg := NewRunnerRegistry()
		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{Success: true, Output: "success"}, nil
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Output)
	})

	t.Run("falls back on format error", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				if req.Model == "haiku" {
					return nil, atlaserrors.ErrAIInvalidFormat
				}
				return &domain.AIResult{Success: true, Output: "sonnet worked"}, nil
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "sonnet worked", result.Output)
		assert.Equal(t, 2, callCount) // haiku failed, sonnet succeeded
	})

	t.Run("falls back on empty response error", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				if req.Model == "haiku" {
					return nil, atlaserrors.ErrAIEmptyResponse
				}
				return &domain.AIResult{Success: true, Output: "sonnet worked"}, nil
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "sonnet worked", result.Output)
		assert.Equal(t, 2, callCount)
	})

	t.Run("exhausts all fallbacks", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				return nil, atlaserrors.ErrAIInvalidFormat // Always fail with format error
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrAllFallbacksExhausted)
		assert.Nil(t, result)
		assert.Equal(t, 3, callCount) // haiku, sonnet, opus all tried
	})

	t.Run("stops on non-recoverable error", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				return nil, context.Canceled // Non-recoverable
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
		assert.Equal(t, 1, callCount) // Only one attempt
	})

	t.Run("stops on auth error", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				return nil, errAuthFallbackTest
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 1, callCount) // Only one attempt
	})

	t.Run("retries transient errors on same model", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				if callCount == 1 && req.Model == "haiku" {
					return nil, errNetworkFallbackTest // Transient error
				}
				return &domain.AIResult{Success: true, Output: "success"}, nil
			},
		})

		cfg := DefaultFallbackConfig()
		cfg.MaxRetriesPerModel = 2 // Allow retries

		runner := NewFallbackRunner(reg, cfg, zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "success", result.Output)
		assert.Equal(t, 2, callCount) // First failed, retry succeeded
	})

	t.Run("skips fallback when disabled", func(t *testing.T) {
		reg := NewRunnerRegistry()
		callCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				callCount++
				return nil, atlaserrors.ErrAIInvalidFormat
			},
		})

		cfg := DefaultFallbackConfig()
		cfg.Enabled = false

		runner := NewFallbackRunner(reg, cfg, zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrAIInvalidFormat)
		assert.Equal(t, 1, callCount) // No fallback attempted
	})

	t.Run("returns error for missing agent", func(t *testing.T) {
		reg := NewRunnerRegistry()
		// No runners registered

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		_, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent not found")
	})

	t.Run("starts from requested model in chain", func(t *testing.T) {
		reg := NewRunnerRegistry()
		modelsAttempted := []string{}

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				modelsAttempted = append(modelsAttempted, req.Model)
				return nil, atlaserrors.ErrAIInvalidFormat
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		// Start from sonnet (middle of chain)
		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "sonnet",
		}
		_, _ = runner.Run(context.Background(), req)

		// Should only try sonnet and opus (not haiku)
		assert.Equal(t, []string{"sonnet", "opus"}, modelsAttempted)
	})

	t.Run("handles cross-agent fallback", func(t *testing.T) {
		reg := NewRunnerRegistry()
		agentsAttempted := []string{}

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				agentsAttempted = append(agentsAttempted, "claude")
				return nil, atlaserrors.ErrAIInvalidFormat
			},
		})
		reg.Register(domain.AgentGemini, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				agentsAttempted = append(agentsAttempted, "gemini")
				return &domain.AIResult{Success: true, Output: "gemini success"}, nil
			},
		})

		cfg := DefaultFallbackConfig()
		cfg.AgentFallbackOrder = []string{"claude", "gemini"}

		runner := NewFallbackRunner(reg, cfg, zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, "gemini success", result.Output)
		// claude: haiku, sonnet, opus all failed, then gemini: flash succeeded
		assert.Equal(t, []string{"claude", "claude", "claude", "gemini"}, agentsAttempted)
	})

	t.Run("preserves request fields in fallback", func(t *testing.T) {
		reg := NewRunnerRegistry()
		var capturedReq *domain.AIRequest

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				if req.Model == "sonnet" {
					capturedReq = req
					return &domain.AIResult{Success: true}, nil
				}
				return nil, atlaserrors.ErrAIInvalidFormat
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent:      domain.AgentClaude,
			Model:      "haiku",
			Prompt:     "test prompt",
			Context:    "test context",
			WorkingDir: "/test/dir",
		}
		_, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, capturedReq)
		assert.Equal(t, "test prompt", capturedReq.Prompt)
		assert.Equal(t, "test context", capturedReq.Context)
		assert.Equal(t, "/test/dir", capturedReq.WorkingDir)
		assert.Equal(t, "sonnet", capturedReq.Model) // Model was updated
	})
}

func TestFallbackRunner_BuildExecutionChain(t *testing.T) {
	t.Run("builds chain from model to end", func(t *testing.T) {
		runner := NewFallbackRunner(NewRunnerRegistry(), DefaultFallbackConfig(), zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "haiku")

		require.Len(t, chain, 3)
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "haiku"}, chain[0])
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "sonnet"}, chain[1])
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "opus"}, chain[2])
	})

	t.Run("starts from middle of chain", func(t *testing.T) {
		runner := NewFallbackRunner(NewRunnerRegistry(), DefaultFallbackConfig(), zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "sonnet")

		require.Len(t, chain, 2)
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "sonnet"}, chain[0])
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "opus"}, chain[1])
	})

	t.Run("starts from end of chain", func(t *testing.T) {
		runner := NewFallbackRunner(NewRunnerRegistry(), DefaultFallbackConfig(), zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "opus")

		require.Len(t, chain, 1)
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "opus"}, chain[0])
	})

	t.Run("includes agent fallbacks", func(t *testing.T) {
		cfg := DefaultFallbackConfig()
		cfg.AgentFallbackOrder = []string{"claude", "gemini"}

		runner := NewFallbackRunner(NewRunnerRegistry(), cfg, zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "sonnet")

		// claude: sonnet, opus + gemini: flash, pro
		require.Len(t, chain, 4)
		assert.Equal(t, domain.AgentClaude, chain[0].Agent)
		assert.Equal(t, domain.AgentClaude, chain[1].Agent)
		assert.Equal(t, domain.AgentGemini, chain[2].Agent)
		assert.Equal(t, domain.AgentGemini, chain[3].Agent)
	})

	t.Run("handles unknown model in chain", func(t *testing.T) {
		runner := NewFallbackRunner(NewRunnerRegistry(), DefaultFallbackConfig(), zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "unknown-model")

		// Should start from beginning since unknown model not found
		require.Len(t, chain, 3)
		assert.Equal(t, "haiku", chain[0].Model)
	})

	t.Run("handles agent with no configured chain", func(t *testing.T) {
		cfg := &FallbackConfig{
			Enabled:     true,
			ModelChains: map[string][]string{}, // Empty chains
		}

		runner := NewFallbackRunner(NewRunnerRegistry(), cfg, zerolog.Nop())

		chain := runner.buildExecutionChain(domain.AgentClaude, "sonnet")

		// Should just use the requested model
		require.Len(t, chain, 1)
		assert.Equal(t, fallbackAttempt{Agent: domain.AgentClaude, Model: "sonnet"}, chain[0])
	})
}

func TestFallbackRunner_IntegrationScenarios(t *testing.T) {
	t.Run("smart commit scenario: haiku fails, sonnet succeeds", func(t *testing.T) {
		reg := NewRunnerRegistry()
		attemptLog := []string{}

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				attemptLog = append(attemptLog, fmt.Sprintf("%s:%s", req.Agent, req.Model))
				if req.Model == "haiku" {
					// Simulate the format error from logs
					return nil, fmt.Errorf("AI response not in expected format: %w", atlaserrors.ErrAIInvalidFormat)
				}
				// Sonnet produces valid conventional commit
				return &domain.AIResult{
					Success: true,
					Output:  "feat(cli): add new feature\n\nThis implements the requested functionality.",
				}, nil
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent:      domain.AgentClaude,
			Model:      "haiku",
			Prompt:     "Generate a commit message for the staged changes",
			WorkingDir: "/test/repo",
		}
		result, err := runner.Run(context.Background(), req)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "feat(cli)")
		assert.Equal(t, []string{"claude:haiku", "claude:sonnet"}, attemptLog)
	})

	t.Run("all models fail, falls back to simple message", func(t *testing.T) {
		reg := NewRunnerRegistry()
		attemptCount := 0

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				attemptCount++
				return nil, atlaserrors.ErrAIInvalidFormat
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		req := &domain.AIRequest{
			Agent: domain.AgentClaude,
			Model: "haiku",
		}
		result, err := runner.Run(context.Background(), req)

		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrAllFallbacksExhausted)
		assert.Nil(t, result)
		assert.Equal(t, 3, attemptCount) // All three models tried
	})

	t.Run("concurrent requests with fallback", func(t *testing.T) {
		reg := NewRunnerRegistry()

		reg.Register(domain.AgentClaude, &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				if req.Model == "haiku" {
					return nil, atlaserrors.ErrAIInvalidFormat
				}
				return &domain.AIResult{Success: true, Output: "success"}, nil
			},
		})

		runner := NewFallbackRunner(reg, DefaultFallbackConfig(), zerolog.Nop())

		// Run multiple concurrent requests
		done := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func() {
				req := &domain.AIRequest{
					Agent: domain.AgentClaude,
					Model: "haiku",
				}
				_, err := runner.Run(context.Background(), req)
				done <- err
			}()
		}

		// All should succeed
		for i := 0; i < 10; i++ {
			err := <-done
			assert.NoError(t, err)
		}
	})
}

// Compile-time check that FallbackRunner implements Runner.
var _ Runner = (*FallbackRunner)(nil)
