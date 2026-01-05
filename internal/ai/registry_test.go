package ai

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// MockRunner is a test implementation of Runner.
type MockRunner struct {
	RunFunc func(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

func (m *MockRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if m.RunFunc != nil {
		return m.RunFunc(ctx, req)
	}
	return &domain.AIResult{Success: true}, nil
}

func TestNewRunnerRegistry(t *testing.T) {
	t.Run("creates empty registry", func(t *testing.T) {
		reg := NewRunnerRegistry()
		require.NotNil(t, reg)
		assert.NotNil(t, reg.runners)
		assert.Empty(t, reg.runners)
	})
}

func TestRunnerRegistry_Register(t *testing.T) {
	t.Run("registers runner for agent", func(t *testing.T) {
		reg := NewRunnerRegistry()
		runner := &MockRunner{}

		reg.Register(domain.AgentClaude, runner)

		got, err := reg.Get(domain.AgentClaude)
		require.NoError(t, err)
		assert.Equal(t, runner, got)
	})

	t.Run("replaces existing runner", func(t *testing.T) {
		reg := NewRunnerRegistry()
		runner1 := &MockRunner{}
		runner2 := &MockRunner{}

		reg.Register(domain.AgentClaude, runner1)
		reg.Register(domain.AgentClaude, runner2)

		got, err := reg.Get(domain.AgentClaude)
		require.NoError(t, err)
		assert.Equal(t, runner2, got)
	})

	t.Run("registers multiple agents", func(t *testing.T) {
		reg := NewRunnerRegistry()
		claudeRunner := &MockRunner{}
		geminiRunner := &MockRunner{}

		reg.Register(domain.AgentClaude, claudeRunner)
		reg.Register(domain.AgentGemini, geminiRunner)

		gotClaude, err := reg.Get(domain.AgentClaude)
		require.NoError(t, err)
		assert.Equal(t, claudeRunner, gotClaude)

		gotGemini, err := reg.Get(domain.AgentGemini)
		require.NoError(t, err)
		assert.Equal(t, geminiRunner, gotGemini)
	})
}

func TestRunnerRegistry_Get(t *testing.T) {
	t.Run("returns runner for registered agent", func(t *testing.T) {
		reg := NewRunnerRegistry()
		runner := &MockRunner{}
		reg.Register(domain.AgentClaude, runner)

		got, err := reg.Get(domain.AgentClaude)
		require.NoError(t, err)
		assert.Equal(t, runner, got)
	})

	t.Run("returns error for unregistered agent", func(t *testing.T) {
		reg := NewRunnerRegistry()

		got, err := reg.Get(domain.AgentClaude)
		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrAgentNotFound)
		assert.Nil(t, got)
	})
}

func TestRunnerRegistry_Has(t *testing.T) {
	t.Run("returns true for registered agent", func(t *testing.T) {
		reg := NewRunnerRegistry()
		reg.Register(domain.AgentClaude, &MockRunner{})

		assert.True(t, reg.Has(domain.AgentClaude))
	})

	t.Run("returns false for unregistered agent", func(t *testing.T) {
		reg := NewRunnerRegistry()

		assert.False(t, reg.Has(domain.AgentClaude))
	})
}

func TestRunnerRegistry_Agents(t *testing.T) {
	t.Run("returns empty slice for empty registry", func(t *testing.T) {
		reg := NewRunnerRegistry()

		agents := reg.Agents()
		assert.Empty(t, agents)
	})

	t.Run("returns all registered agents", func(t *testing.T) {
		reg := NewRunnerRegistry()
		reg.Register(domain.AgentClaude, &MockRunner{})
		reg.Register(domain.AgentGemini, &MockRunner{})

		agents := reg.Agents()
		assert.Len(t, agents, 2)
		assert.ElementsMatch(t, []domain.Agent{domain.AgentClaude, domain.AgentGemini}, agents)
	})
}

func TestRunnerRegistry_Concurrency(t *testing.T) {
	t.Run("handles concurrent access", func(t *testing.T) {
		reg := NewRunnerRegistry()
		var wg sync.WaitGroup

		// Concurrent writes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reg.Register(domain.AgentClaude, &MockRunner{})
			}()
		}

		// Concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reg.Has(domain.AgentClaude)
				_, _ = reg.Get(domain.AgentClaude)
				reg.Agents()
			}()
		}

		wg.Wait()
		assert.True(t, reg.Has(domain.AgentClaude))
	})
}

func TestNewMultiRunner(t *testing.T) {
	t.Run("creates multi-runner with registry", func(t *testing.T) {
		reg := NewRunnerRegistry()
		multi := NewMultiRunner(reg)

		require.NotNil(t, multi)
		assert.Equal(t, reg, multi.registry)
	})
}

func TestMultiRunner_Run(t *testing.T) {
	t.Run("dispatches to correct runner", func(t *testing.T) {
		reg := NewRunnerRegistry()

		claudeRunner := &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{Success: true, Output: "claude response"}, nil
			},
		}
		geminiRunner := &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{Success: true, Output: "gemini response"}, nil
			},
		}

		reg.Register(domain.AgentClaude, claudeRunner)
		reg.Register(domain.AgentGemini, geminiRunner)

		multi := NewMultiRunner(reg)

		// Test Claude dispatch
		req := &domain.AIRequest{
			Agent:  domain.AgentClaude,
			Prompt: "test",
		}
		result, err := multi.Run(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "claude response", result.Output)

		// Test Gemini dispatch
		req.Agent = domain.AgentGemini
		result, err = multi.Run(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "gemini response", result.Output)
	})

	t.Run("defaults to claude when agent is empty", func(t *testing.T) {
		reg := NewRunnerRegistry()

		claudeRunner := &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return &domain.AIResult{Success: true, Output: "claude default"}, nil
			},
		}

		reg.Register(domain.AgentClaude, claudeRunner)

		multi := NewMultiRunner(reg)

		// Request with empty agent should default to Claude
		req := &domain.AIRequest{
			Agent:  "", // Empty agent
			Prompt: "test",
		}
		result, err := multi.Run(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "claude default", result.Output)
	})

	t.Run("returns error for unregistered agent", func(t *testing.T) {
		reg := NewRunnerRegistry()
		multi := NewMultiRunner(reg)

		req := &domain.AIRequest{
			Agent:  domain.AgentGemini,
			Prompt: "test",
		}
		result, err := multi.Run(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, atlaserrors.ErrAgentNotFound)
		assert.Nil(t, result)
	})

	t.Run("propagates runner errors", func(t *testing.T) {
		reg := NewRunnerRegistry()

		errTest := fmt.Errorf("runner error: %w", atlaserrors.ErrCommandFailed)
		claudeRunner := &MockRunner{
			RunFunc: func(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				return nil, errTest
			},
		}

		reg.Register(domain.AgentClaude, claudeRunner)

		multi := NewMultiRunner(reg)

		req := &domain.AIRequest{
			Agent:  domain.AgentClaude,
			Prompt: "test",
		}
		result, err := multi.Run(context.Background(), req)
		require.Error(t, err)
		assert.Equal(t, errTest, err)
		assert.Nil(t, result)
	})

	t.Run("passes context to runner", func(t *testing.T) {
		reg := NewRunnerRegistry()

		var receivedCtx context.Context
		claudeRunner := &MockRunner{
			RunFunc: func(ctx context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
				receivedCtx = ctx
				return &domain.AIResult{Success: true}, nil
			},
		}

		reg.Register(domain.AgentClaude, claudeRunner)

		multi := NewMultiRunner(reg)

		ctx := context.WithValue(context.Background(), "key", "value") //nolint:staticcheck // test context
		req := &domain.AIRequest{
			Agent:  domain.AgentClaude,
			Prompt: "test",
		}
		_, err := multi.Run(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, ctx, receivedCtx)
	})

	t.Run("passes request to runner", func(t *testing.T) {
		reg := NewRunnerRegistry()

		var receivedReq *domain.AIRequest
		claudeRunner := &MockRunner{
			RunFunc: func(_ context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
				receivedReq = req
				return &domain.AIResult{Success: true}, nil
			},
		}

		reg.Register(domain.AgentClaude, claudeRunner)

		multi := NewMultiRunner(reg)

		req := &domain.AIRequest{
			Agent:  domain.AgentClaude,
			Prompt: "test prompt",
			Model:  "sonnet",
		}
		_, err := multi.Run(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, req, receivedReq)
	})
}

// Compile-time check that MultiRunner implements Runner.
var _ Runner = (*MultiRunner)(nil)
