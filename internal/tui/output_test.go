package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

func TestTTYOutput(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Success("test message")
		assert.Contains(t, buf.String(), "test message")
	})

	t.Run("Error", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Error(atlaserrors.ErrWorkspaceNotFound)
		assert.Contains(t, buf.String(), "not found")
	})

	t.Run("Warning", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Warning("test warning")
		assert.Contains(t, buf.String(), "test warning")
	})

	t.Run("Info", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Info("test info")
		assert.Contains(t, buf.String(), "test info")
	})

	t.Run("JSON", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		err := out.JSON(map[string]string{"key": "value"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "key")
		assert.Contains(t, buf.String(), "value")
	})
}

func TestJSONOutput(t *testing.T) {
	t.Run("Success is no-op", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Success("test message")
		assert.Empty(t, buf.String())
	})

	t.Run("Error", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Error(atlaserrors.ErrWorkspaceNotFound)
		assert.Contains(t, buf.String(), "error")
		assert.Contains(t, buf.String(), "not found")
	})

	t.Run("Warning is no-op", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Warning("test warning")
		assert.Empty(t, buf.String())
	})

	t.Run("Info is no-op", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Info("test info")
		assert.Empty(t, buf.String())
	})

	t.Run("JSON", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		err := out.JSON(map[string]string{"key": "value"})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "key")
		assert.Contains(t, buf.String(), "value")
	})
}

func TestNewOutput(t *testing.T) {
	t.Run("json format returns JSONOutput", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutput(&buf, "json")
		_, ok := out.(*JSONOutput)
		assert.True(t, ok)
	})

	t.Run("text format returns TTYOutput", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutput(&buf, "text")
		_, ok := out.(*TTYOutput)
		assert.True(t, ok)
	})

	t.Run("empty format returns TTYOutput", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutput(&buf, "")
		_, ok := out.(*TTYOutput)
		assert.True(t, ok)
	})
}
