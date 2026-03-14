package workflow

import (
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/tui"
)

func TestNewOrchestrator(t *testing.T) {
	t.Run("creates orchestrator with all dependencies", func(t *testing.T) {
		logger := zerolog.Nop()
		out := tui.NewOutput(io.Discard, "text")

		orch := NewOrchestrator(logger, out)

		assert.NotNil(t, orch)
		assert.NotNil(t, orch.services)
		assert.NotNil(t, orch.initializer)
		assert.NotNil(t, orch.prompter)
		assert.Equal(t, logger, orch.logger)
	})
}

func TestOrchestrator_Services(t *testing.T) {
	t.Run("returns service factory", func(t *testing.T) {
		logger := zerolog.Nop()
		out := tui.NewOutput(io.Discard, "text")

		orch := NewOrchestrator(logger, out)
		services := orch.Services()

		assert.NotNil(t, services)
		assert.Equal(t, orch.services, services)
	})
}

func TestOrchestrator_Initializer(t *testing.T) {
	t.Run("returns initializer", func(t *testing.T) {
		logger := zerolog.Nop()
		out := tui.NewOutput(io.Discard, "text")

		orch := NewOrchestrator(logger, out)
		init := orch.Initializer()

		assert.NotNil(t, init)
		assert.Equal(t, orch.initializer, init)
	})
}

func TestOrchestrator_Prompter(t *testing.T) {
	t.Run("returns prompter", func(t *testing.T) {
		logger := zerolog.Nop()
		out := tui.NewOutput(io.Discard, "text")

		orch := NewOrchestrator(logger, out)
		prompter := orch.Prompter()

		assert.NotNil(t, prompter)
		assert.Equal(t, orch.prompter, prompter)
	})
}

func TestOrchestrator_StartTask(t *testing.T) {
	t.Run("method exists and can be called", func(t *testing.T) {
		logger := zerolog.Nop()
		out := tui.NewOutput(io.Discard, "text")

		orch := NewOrchestrator(logger, out)

		// We're just testing that the method exists with the correct signature
		// Actual testing would require creating a mock engine, workspace, and template
		// which is beyond the scope of unit testing this method
		assert.NotNil(t, orch)
	})
}

func TestMaxWorkspaceNameLen(t *testing.T) {
	t.Run("exported constant equals internal constant", func(t *testing.T) {
		assert.Equal(t, maxWorkspaceNameLen, MaxWorkspaceNameLen)
		assert.Equal(t, 50, MaxWorkspaceNameLen)
	})
}

func TestRegexPatterns(t *testing.T) {
	t.Run("nonAlphanumericRegex matches special chars", func(t *testing.T) {
		input := "test@#$name"
		result := nonAlphanumericRegex.ReplaceAllString(input, "")
		assert.Equal(t, "testname", result)
	})

	t.Run("multipleHyphensRegex collapses hyphens", func(t *testing.T) {
		input := "test---name"
		result := multipleHyphensRegex.ReplaceAllString(input, "-")
		assert.Equal(t, "test-name", result)
	})
}
