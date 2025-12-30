package steps

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

func TestNewDefaultRegistry(t *testing.T) {
	runner := &mockAIRunner{}
	deps := ExecutorDeps{
		AIRunner:     runner,
		WorkDir:      "/tmp/work",
		ArtifactsDir: "/tmp/artifacts",
	}

	registry := NewDefaultRegistry(deps)

	require.NotNil(t, registry)

	// All 7 step types should be registered
	assert.True(t, registry.Has(domain.StepTypeAI))
	assert.True(t, registry.Has(domain.StepTypeValidation))
	assert.True(t, registry.Has(domain.StepTypeGit))
	assert.True(t, registry.Has(domain.StepTypeHuman))
	assert.True(t, registry.Has(domain.StepTypeSDD))
	assert.True(t, registry.Has(domain.StepTypeCI))
	assert.True(t, registry.Has(domain.StepTypeVerify))

	types := registry.Types()
	assert.Len(t, types, 7)
}

func TestNewDefaultRegistry_NilAIRunner(t *testing.T) {
	deps := ExecutorDeps{
		AIRunner: nil, // No AI runner
		WorkDir:  "/tmp/work",
	}

	registry := NewDefaultRegistry(deps)

	require.NotNil(t, registry)

	// AI, SDD, and Verify should NOT be registered without AIRunner
	assert.False(t, registry.Has(domain.StepTypeAI))
	assert.False(t, registry.Has(domain.StepTypeSDD))
	assert.False(t, registry.Has(domain.StepTypeVerify))

	// Others should still be registered
	assert.True(t, registry.Has(domain.StepTypeValidation))
	assert.True(t, registry.Has(domain.StepTypeGit))
	assert.True(t, registry.Has(domain.StepTypeHuman))
	assert.True(t, registry.Has(domain.StepTypeCI))

	types := registry.Types()
	assert.Len(t, types, 4)
}

func TestNewDefaultRegistry_ExecutorTypes(t *testing.T) {
	runner := &mockAIRunner{}
	deps := ExecutorDeps{
		AIRunner:     runner,
		WorkDir:      "/tmp/work",
		ArtifactsDir: "/tmp/artifacts",
	}

	registry := NewDefaultRegistry(deps)

	tests := []struct {
		stepType domain.StepType
		expected domain.StepType
	}{
		{domain.StepTypeAI, domain.StepTypeAI},
		{domain.StepTypeValidation, domain.StepTypeValidation},
		{domain.StepTypeGit, domain.StepTypeGit},
		{domain.StepTypeHuman, domain.StepTypeHuman},
		{domain.StepTypeSDD, domain.StepTypeSDD},
		{domain.StepTypeCI, domain.StepTypeCI},
		{domain.StepTypeVerify, domain.StepTypeVerify},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			executor, err := registry.Get(tt.stepType)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, executor.Type())
		})
	}
}

func TestNewMinimalRegistry(t *testing.T) {
	registry := NewMinimalRegistry("/tmp/work")

	require.NotNil(t, registry)

	// Should have 4 non-AI executors
	assert.True(t, registry.Has(domain.StepTypeValidation))
	assert.True(t, registry.Has(domain.StepTypeGit))
	assert.True(t, registry.Has(domain.StepTypeHuman))
	assert.True(t, registry.Has(domain.StepTypeCI))

	// Should NOT have AI-dependent executors
	assert.False(t, registry.Has(domain.StepTypeAI))
	assert.False(t, registry.Has(domain.StepTypeSDD))

	types := registry.Types()
	assert.Len(t, types, 4)
}

func TestNewMinimalRegistry_ExecutorTypes(t *testing.T) {
	registry := NewMinimalRegistry("/tmp/work")

	tests := []struct {
		stepType domain.StepType
		expected domain.StepType
	}{
		{domain.StepTypeValidation, domain.StepTypeValidation},
		{domain.StepTypeGit, domain.StepTypeGit},
		{domain.StepTypeHuman, domain.StepTypeHuman},
		{domain.StepTypeCI, domain.StepTypeCI},
	}

	for _, tt := range tests {
		t.Run(string(tt.stepType), func(t *testing.T) {
			executor, err := registry.Get(tt.stepType)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, executor.Type())
		})
	}
}

func TestExecutorDeps(t *testing.T) {
	runner := &mockAIRunner{}
	deps := ExecutorDeps{
		AIRunner:     runner,
		WorkDir:      "/path/to/work",
		ArtifactsDir: "/path/to/artifacts",
	}

	assert.Equal(t, runner, deps.AIRunner)
	assert.Equal(t, "/path/to/work", deps.WorkDir)
	assert.Equal(t, "/path/to/artifacts", deps.ArtifactsDir)
}
