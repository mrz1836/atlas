package ai

import (
	"context"
	"fmt"
	"sync"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// RunnerRegistry maps agent types to their AI runners.
// It provides thread-safe registration and lookup of runners.
type RunnerRegistry struct {
	mu      sync.RWMutex
	runners map[domain.Agent]Runner
}

// NewRunnerRegistry creates a new empty runner registry.
func NewRunnerRegistry() *RunnerRegistry {
	return &RunnerRegistry{
		runners: make(map[domain.Agent]Runner),
	}
}

// Register adds a runner for an agent type.
// If a runner already exists for the agent, it is replaced.
func (r *RunnerRegistry) Register(agent domain.Agent, runner Runner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runners[agent] = runner
}

// Get retrieves the runner for an agent type.
// Returns ErrAgentNotFound if no runner is registered for the agent.
func (r *RunnerRegistry) Get(agent domain.Agent) (Runner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runner, ok := r.runners[agent]
	if !ok {
		return nil, fmt.Errorf("%w: %s", atlaserrors.ErrAgentNotFound, agent)
	}
	return runner, nil
}

// Has checks if a runner is registered for the agent.
func (r *RunnerRegistry) Has(agent domain.Agent) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.runners[agent]
	return ok
}

// Agents returns all registered agent types.
func (r *RunnerRegistry) Agents() []domain.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]domain.Agent, 0, len(r.runners))
	for a := range r.runners {
		agents = append(agents, a)
	}
	return agents
}

// MultiRunner dispatches AI requests to the appropriate runner based on the agent field.
// It implements the Runner interface to provide transparent agent routing.
type MultiRunner struct {
	registry *RunnerRegistry
}

// NewMultiRunner creates a multi-runner with the given registry.
func NewMultiRunner(registry *RunnerRegistry) *MultiRunner {
	return &MultiRunner{registry: registry}
}

// Run dispatches to the appropriate runner based on req.Agent.
// Returns ErrEmptyAgent if req.Agent is not specified.
func (m *MultiRunner) Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error) {
	if req.Agent == "" {
		return nil, fmt.Errorf("%w: agent must be specified in request", atlaserrors.ErrEmptyValue)
	}

	runner, err := m.registry.Get(req.Agent)
	if err != nil {
		return nil, err
	}

	return runner.Run(ctx, req)
}

// Compile-time check that MultiRunner implements Runner.
var _ Runner = (*MultiRunner)(nil)
