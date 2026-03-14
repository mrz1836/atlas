// Package ai provides AI execution capabilities for ATLAS.
package ai

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// FallbackConfig configures the fallback behavior for AI operations.
type FallbackConfig struct {
	// Enabled controls whether fallback is active.
	// When false, no fallback attempts are made.
	Enabled bool

	// ModelChains defines the fallback order of models per agent.
	// Example: {"claude": ["haiku", "sonnet", "opus"]}
	// The first model in the chain matching the request's model is used as the starting point.
	ModelChains map[string][]string

	// AgentFallbackOrder defines the order to try agents if all models fail.
	// Example: ["claude", "gemini", "codex"]
	// If empty, no cross-agent fallback occurs.
	AgentFallbackOrder []string

	// MaxRetriesPerModel is how many times to retry each model before fallback.
	// Default: 1 (try once, then move to next model)
	MaxRetriesPerModel int
}

// DefaultFallbackConfig returns the default fallback configuration.
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		Enabled: true,
		ModelChains: map[string][]string{
			"claude": {"haiku", "sonnet", "opus"},
			"gemini": {"flash", "pro"},
			"codex":  {"mini", "codex", "max"},
		},
		AgentFallbackOrder: nil, // No cross-agent fallback by default
		MaxRetriesPerModel: 1,
	}
}

// fallbackAttempt represents a single attempt configuration (agent + model).
type fallbackAttempt struct {
	Agent domain.Agent
	Model string
}

// FallbackRunner wraps an AI runner registry with automatic fallback capability.
// When a model fails with format/content errors, it automatically tries the next
// model in the configured fallback chain.
type FallbackRunner struct {
	registry *RunnerRegistry
	config   *FallbackConfig
	logger   zerolog.Logger
}

// NewFallbackRunner creates a new FallbackRunner with the given registry and config.
// If config is nil, DefaultFallbackConfig() is used.
func NewFallbackRunner(registry *RunnerRegistry, config *FallbackConfig, logger zerolog.Logger) *FallbackRunner {
	if config == nil {
		config = DefaultFallbackConfig()
	}
	if config.MaxRetriesPerModel < 1 {
		config.MaxRetriesPerModel = 1
	}
	return &FallbackRunner{
		registry: registry,
		config:   config,
		logger:   logger,
	}
}

// Run executes an AI request with automatic fallback on format/content errors.
// It builds an execution chain based on the request's agent and model, then
// tries each combination until one succeeds or all fail.
//
//nolint:gocognit // Complexity is inherent to the fallback logic with nested retry loops
func (r *FallbackRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if !r.config.Enabled {
		// Fallback disabled - just run directly
		return r.runDirect(ctx, req)
	}

	// Build the execution chain
	chain := r.buildExecutionChain(req.Agent, req.Model)
	if len(chain) == 0 {
		return nil, fmt.Errorf("%w: agent=%s model=%s", atlaserrors.ErrNoFallbackModels, req.Agent, req.Model)
	}

	var lastErr error
	for i, attempt := range chain {
		// Try this attempt (with retries)
		for retry := 0; retry < r.config.MaxRetriesPerModel; retry++ {
			result, err := r.executeAttempt(ctx, req, attempt)
			if err == nil {
				// Success!
				if i > 0 || retry > 0 {
					r.logger.Info().
						Str("agent", string(attempt.Agent)).
						Str("model", attempt.Model).
						Int("fallback_index", i).
						Int("retry", retry).
						Msg("AI request succeeded after fallback")
				}
				return result, nil
			}

			// Check if error is non-recoverable
			if isNonRecoverableError(err) {
				r.logger.Warn().
					Err(err).
					Str("agent", string(attempt.Agent)).
					Str("model", attempt.Model).
					Msg("non-recoverable error, stopping all attempts")
				return nil, err
			}

			lastErr = err

			// Check if we should fallback or retry
			if isFallbackTrigger(err) {
				// Format/content error - move to next model immediately
				r.logger.Info().
					Err(err).
					Str("agent", string(attempt.Agent)).
					Str("model", attempt.Model).
					Int("fallback_index", i).
					Msg("fallback triggered, trying next model")
				break // Exit retry loop, move to next model
			}

			// Transient error - retry same model
			if retry < r.config.MaxRetriesPerModel-1 {
				r.logger.Debug().
					Err(err).
					Str("agent", string(attempt.Agent)).
					Str("model", attempt.Model).
					Int("retry", retry+1).
					Int("max_retries", r.config.MaxRetriesPerModel).
					Msg("transient error, retrying same model")
			}
		}
	}

	// All attempts exhausted
	return nil, fmt.Errorf("%w: %w", atlaserrors.ErrAllFallbacksExhausted, lastErr)
}

// buildExecutionChain creates the ordered list of (agent, model) pairs to try.
// The chain starts from the requested agent/model and includes all fallbacks.
func (r *FallbackRunner) buildExecutionChain(agent domain.Agent, model string) []fallbackAttempt {
	var chain []fallbackAttempt

	// Helper to add models for an agent starting from a specific model
	addAgentModels := func(a domain.Agent, startModel string) {
		agentStr := string(a)
		models, ok := r.config.ModelChains[agentStr]
		if !ok {
			// No chain configured - just use the requested model
			chain = append(chain, fallbackAttempt{Agent: a, Model: startModel})
			return
		}

		// Find the starting position in the chain
		startIdx := 0
		for i, m := range models {
			if m == startModel {
				startIdx = i
				break
			}
		}

		// Add all models from start position onward
		for i := startIdx; i < len(models); i++ {
			chain = append(chain, fallbackAttempt{Agent: a, Model: models[i]})
		}
	}

	// Add the primary agent's models
	addAgentModels(agent, model)

	// Add fallback agents if configured
	for _, fallbackAgent := range r.config.AgentFallbackOrder {
		fa := domain.Agent(fallbackAgent)
		if fa == agent {
			continue // Skip the primary agent, already added
		}

		// For fallback agents, start from the beginning of their chain
		agentStr := string(fa)
		if models, ok := r.config.ModelChains[agentStr]; ok && len(models) > 0 {
			addAgentModels(fa, models[0])
		} else {
			// Use the agent's default model
			chain = append(chain, fallbackAttempt{Agent: fa, Model: fa.DefaultModel()})
		}
	}

	return chain
}

// executeAttempt runs a single AI request attempt with the specified agent/model.
func (r *FallbackRunner) executeAttempt(ctx context.Context, req *domain.AIRequest, attempt fallbackAttempt) (*domain.AIResult, error) {
	// Get the runner for this agent
	runner, err := r.registry.Get(attempt.Agent)
	if err != nil {
		return nil, fmt.Errorf("failed to get runner for agent %s: %w", attempt.Agent, err)
	}

	// Clone the request with the attempt's agent/model
	attemptReq := *req
	attemptReq.Agent = attempt.Agent
	attemptReq.Model = attempt.Model

	r.logger.Debug().
		Str("agent", string(attempt.Agent)).
		Str("model", attempt.Model).
		Msg("executing AI attempt")

	return runner.Run(ctx, &attemptReq)
}

// runDirect runs the request without any fallback logic.
func (r *FallbackRunner) runDirect(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	runner, err := r.registry.Get(req.Agent)
	if err != nil {
		return nil, err
	}
	return runner.Run(ctx, req)
}

// Compile-time check that FallbackRunner implements Runner.
var _ Runner = (*FallbackRunner)(nil)
