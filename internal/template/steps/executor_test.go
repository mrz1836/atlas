package steps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestNewExecutorRegistry(t *testing.T) {
	r := NewExecutorRegistry()

	require.NotNil(t, r)
	assert.NotNil(t, r.executors)
	assert.Empty(t, r.Types())
}

func TestExecutorRegistry_Register(t *testing.T) {
	r := NewExecutorRegistry()
	executor := NewHumanExecutor()

	r.Register(executor)

	assert.True(t, r.Has(domain.StepTypeHuman))
	assert.Len(t, r.Types(), 1)
}

func TestExecutorRegistry_Register_Replace(t *testing.T) {
	r := NewExecutorRegistry()
	executor1 := NewHumanExecutor()
	executor2 := NewHumanExecutor()

	r.Register(executor1)
	r.Register(executor2)

	// Should replace, not add
	assert.Len(t, r.Types(), 1)

	got, err := r.Get(domain.StepTypeHuman)
	require.NoError(t, err)
	assert.Equal(t, executor2, got)
}

func TestExecutorRegistry_Get_Found(t *testing.T) {
	r := NewExecutorRegistry()
	executor := NewHumanExecutor()
	r.Register(executor)

	got, err := r.Get(domain.StepTypeHuman)

	require.NoError(t, err)
	assert.Equal(t, executor, got)
}

func TestExecutorRegistry_Get_NotFound(t *testing.T) {
	r := NewExecutorRegistry()

	got, err := r.Get(domain.StepTypeAI)

	assert.Nil(t, got)
	require.Error(t, err)
	require.ErrorIs(t, err, atlaserrors.ErrExecutorNotFound)
	assert.Contains(t, err.Error(), string(domain.StepTypeAI))
}

func TestExecutorRegistry_Has(t *testing.T) {
	r := NewExecutorRegistry()
	r.Register(NewHumanExecutor())

	assert.True(t, r.Has(domain.StepTypeHuman))
	assert.False(t, r.Has(domain.StepTypeAI))
	assert.False(t, r.Has(domain.StepTypeValidation))
}

func TestExecutorRegistry_Types(t *testing.T) {
	r := NewExecutorRegistry()
	r.Register(NewHumanExecutor())
	r.Register(NewCIExecutor())

	types := r.Types()

	assert.Len(t, types, 2)
	assert.Contains(t, types, domain.StepTypeHuman)
	assert.Contains(t, types, domain.StepTypeCI)
}

func TestExecutorRegistry_Concurrent(t *testing.T) {
	t.Parallel()
	r := NewExecutorRegistry()
	r.Register(NewHumanExecutor())

	// Concurrent reads should not race
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = r.Get(domain.StepTypeHuman)
			_ = r.Has(domain.StepTypeHuman)
			_ = r.Types()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// mockExecutor for testing
type mockExecutor struct {
	stepType domain.StepType
	result   *domain.StepResult
	err      error
}

func (m *mockExecutor) Execute(_ context.Context, _ *domain.Task, _ *domain.StepDefinition) (*domain.StepResult, error) {
	return m.result, m.err
}

func (m *mockExecutor) Type() domain.StepType {
	return m.stepType
}

func TestExecutorRegistry_CustomExecutor(t *testing.T) {
	r := NewExecutorRegistry()
	custom := &mockExecutor{
		stepType: domain.StepTypeAI,
		result:   &domain.StepResult{Status: "success"},
	}

	r.Register(custom)

	got, err := r.Get(domain.StepTypeAI)
	require.NoError(t, err)
	assert.Equal(t, custom, got)
}
