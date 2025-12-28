package ai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestNewAIRequest(t *testing.T) {
	t.Run("creates request with prompt and defaults", func(t *testing.T) {
		req := NewAIRequest("Fix the bug")

		require.NotNil(t, req)
		assert.Equal(t, "Fix the bug", req.Prompt)
		assert.Equal(t, constants.DefaultAITimeout, req.Timeout)
		assert.Empty(t, req.Model)
		assert.Empty(t, req.PermissionMode)
		assert.Empty(t, req.SystemPrompt)
		assert.Empty(t, req.WorkingDir)
	})

	t.Run("applies WithModel option", func(t *testing.T) {
		req := NewAIRequest("test", WithModel("opus"))

		assert.Equal(t, "opus", req.Model)
	})

	t.Run("applies WithTimeout option", func(t *testing.T) {
		timeout := 10 * time.Minute
		req := NewAIRequest("test", WithTimeout(timeout))

		assert.Equal(t, timeout, req.Timeout)
	})

	t.Run("applies WithPermissionMode option", func(t *testing.T) {
		req := NewAIRequest("test", WithPermissionMode("plan"))

		assert.Equal(t, "plan", req.PermissionMode)
	})

	t.Run("applies WithSystemPrompt option", func(t *testing.T) {
		req := NewAIRequest("test", WithSystemPrompt("You are a helpful assistant"))

		assert.Equal(t, "You are a helpful assistant", req.SystemPrompt)
	})

	t.Run("applies WithWorkingDir option", func(t *testing.T) {
		req := NewAIRequest("test", WithWorkingDir("/tmp/workdir"))

		assert.Equal(t, "/tmp/workdir", req.WorkingDir)
	})

	t.Run("applies WithContext option", func(t *testing.T) {
		req := NewAIRequest("test", WithContext("This is a Go project"))

		assert.Equal(t, "This is a Go project", req.Context)
	})

	t.Run("applies WithMaxTurns option", func(t *testing.T) {
		req := NewAIRequest("test", WithMaxTurns(10))

		assert.Equal(t, 10, req.MaxTurns)
	})

	t.Run("applies multiple options", func(t *testing.T) {
		req := NewAIRequest("Fix the bug",
			WithModel("sonnet"),
			WithTimeout(15*time.Minute),
			WithPermissionMode("plan"),
			WithSystemPrompt("You are a code reviewer"),
			WithWorkingDir("/projects/myapp"),
			WithContext("This is a Go microservice"),
			WithMaxTurns(5),
		)

		assert.Equal(t, "Fix the bug", req.Prompt)
		assert.Equal(t, "sonnet", req.Model)
		assert.Equal(t, 15*time.Minute, req.Timeout)
		assert.Equal(t, "plan", req.PermissionMode)
		assert.Equal(t, "You are a code reviewer", req.SystemPrompt)
		assert.Equal(t, "/projects/myapp", req.WorkingDir)
		assert.Equal(t, "This is a Go microservice", req.Context)
		assert.Equal(t, 5, req.MaxTurns)
	})

	t.Run("later options override earlier ones", func(t *testing.T) {
		req := NewAIRequest("test",
			WithModel("haiku"),
			WithModel("opus"), // Override
		)

		assert.Equal(t, "opus", req.Model)
	})
}
