package ai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/domain"
)

// mockRunner is a test implementation of AIRunner.
type mockRunner struct {
	result *domain.AIResult
	err    error
}

func (m *mockRunner) Run(_ context.Context, _ *domain.AIRequest) (*domain.AIResult, error) {
	return m.result, m.err
}

// Compile-time check that mockRunner implements Runner.
var _ Runner = (*mockRunner)(nil)

func TestAIRunner_Interface(t *testing.T) {
	t.Run("interface is satisfied by mock", func(t *testing.T) {
		runner := &mockRunner{
			result: &domain.AIResult{
				Success:   true,
				Output:    "test output",
				SessionID: "test-session",
			},
		}

		result, err := runner.Run(context.Background(), &domain.AIRequest{
			Prompt: "test prompt",
			Model:  "sonnet",
		})

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, "test output", result.Output)
	})

	t.Run("interface returns error", func(t *testing.T) {
		expectedErr := assert.AnError
		runner := &mockRunner{
			err: expectedErr,
		}

		result, err := runner.Run(context.Background(), &domain.AIRequest{})

		require.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result)
	})
}
