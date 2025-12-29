// Package steps provides step execution implementations for the ATLAS task engine.
//
// This package contains the StepExecutor interface and implementations for
// each step type (AI, validation, git, human, SDD, CI). The ExecutorRegistry
// maps step types to their appropriate executors.
//
// Import rules:
//   - CAN import: internal/constants, internal/domain, internal/errors, internal/ai
//   - MUST NOT import: internal/task, internal/workspace, internal/cli, internal/template (parent)
package steps

import (
	"context"
	"fmt"
	"sync"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// StepExecutor defines the interface for executing a single step type.
// Implementations handle specific step types (AI, validation, git, etc.)
// and return structured results.
//
// All Execute implementations must:
//   - Check ctx.Done() at the start and during long operations
//   - Log execution start/end with step context
//   - Return StepResult with appropriate status, output, and timing
//   - Handle context cancellation gracefully
type StepExecutor interface {
	// Execute runs the step and returns its result.
	// The context controls timeout and cancellation.
	// task provides the full task context for step execution.
	// step is the specific step being executed.
	Execute(ctx context.Context, task *domain.Task, step *domain.StepDefinition) (*domain.StepResult, error)

	// Type returns the StepType this executor handles.
	Type() domain.StepType
}

// ExecutorRegistry maps step types to their executors.
// It is safe for concurrent read access after initialization.
// Use NewExecutorRegistry() to create and Register() to add executors.
type ExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[domain.StepType]StepExecutor
}

// NewExecutorRegistry creates a new empty executor registry.
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[domain.StepType]StepExecutor),
	}
}

// Register adds an executor to the registry.
// If an executor for the same type already exists, it will be replaced.
func (r *ExecutorRegistry) Register(e StepExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[e.Type()] = e
}

// Get retrieves the executor for a step type.
// Returns ErrExecutorNotFound if no executor is registered for the type.
func (r *ExecutorRegistry) Get(stepType domain.StepType) (StepExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.executors[stepType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", atlaserrors.ErrExecutorNotFound, stepType)
	}
	return e, nil
}

// Has checks if an executor is registered for the given step type.
func (r *ExecutorRegistry) Has(stepType domain.StepType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.executors[stepType]
	return ok
}

// Types returns all registered step types.
func (r *ExecutorRegistry) Types() []domain.StepType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]domain.StepType, 0, len(r.executors))
	for t := range r.executors {
		types = append(types, t)
	}
	return types
}
