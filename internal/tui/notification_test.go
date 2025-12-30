package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNotifier(t *testing.T) {
	t.Parallel()

	t.Run("with defaults", func(t *testing.T) {
		t.Parallel()
		n := NewNotifier(true, false)
		assert.NotNil(t, n)
		assert.True(t, n.bellEnabled)
		assert.False(t, n.quiet)
	})

	t.Run("bell disabled", func(t *testing.T) {
		t.Parallel()
		n := NewNotifier(false, false)
		assert.False(t, n.bellEnabled)
	})

	t.Run("quiet mode", func(t *testing.T) {
		t.Parallel()
		n := NewNotifier(true, true)
		assert.True(t, n.quiet)
	})
}

func TestNotifier_Bell_EmitsBellCharacter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n := NewNotifierWithWriter(true, false, &buf)

	n.Bell()

	assert.Equal(t, "\a", buf.String())
}

func TestNotifier_Bell_DisabledDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n := NewNotifierWithWriter(false, false, &buf)

	n.Bell()

	assert.Empty(t, buf.String())
}

func TestNotifier_Bell_QuietModeDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n := NewNotifierWithWriter(true, true, &buf)

	n.Bell()

	assert.Empty(t, buf.String())
}

func TestNotifier_Bell_DisabledAndQuietDoesNotEmit(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n := NewNotifierWithWriter(false, true, &buf)

	n.Bell()

	assert.Empty(t, buf.String())
}

func TestNotifier_MultipleBells(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n := NewNotifierWithWriter(true, false, &buf)

	n.Bell()
	n.Bell()
	n.Bell()

	assert.Equal(t, "\a\a\a", buf.String())
}
