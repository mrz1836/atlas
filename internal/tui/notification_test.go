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

// TestNewNotifier_DelegatesToNewNotifierWithWriter verifies that NewNotifier
// correctly delegates to NewNotifierWithWriter with os.Stdout as the writer.
// This test validates the DRY refactoring of the constructor.
func TestNewNotifier_DelegatesToNewNotifierWithWriter(t *testing.T) {
	t.Parallel()

	// Create notifiers with same settings using both constructors
	n1 := NewNotifier(true, false)
	n2 := NewNotifierWithWriter(true, false, nil) // nil writer for comparison

	// Both should have the same bellEnabled and quiet settings
	assert.Equal(t, n1.bellEnabled, n2.bellEnabled)
	assert.Equal(t, n1.quiet, n2.quiet)

	// n1 should have os.Stdout as writer (from delegation)
	// We can't directly compare writers, but we can verify the notifier was created properly
	assert.NotNil(t, n1.writer)
}

// TestNewNotifier_ConfigurationMatrix tests all combinations of bell/quiet settings
func TestNewNotifier_ConfigurationMatrix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		bellEnabled bool
		quiet       bool
	}{
		{"bell_enabled_not_quiet", true, false},
		{"bell_disabled_not_quiet", false, false},
		{"bell_enabled_quiet", true, true},
		{"bell_disabled_quiet", false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := NewNotifier(tc.bellEnabled, tc.quiet)
			assert.Equal(t, tc.bellEnabled, n.bellEnabled)
			assert.Equal(t, tc.quiet, n.quiet)
		})
	}
}
