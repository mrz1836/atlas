package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/atlas/internal/constants"
)

func TestStatusColors(t *testing.T) {
	colors := StatusColors()

	// Verify all workspace statuses have colors defined
	statuses := []constants.WorkspaceStatus{
		constants.WorkspaceStatusActive,
		constants.WorkspaceStatusPaused,
		constants.WorkspaceStatusRetired,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			color, ok := colors[status]
			assert.True(t, ok, "color should be defined for status %s", status)
			assert.NotEmpty(t, color.Light, "light color should be defined")
			assert.NotEmpty(t, color.Dark, "dark color should be defined")
		})
	}
}

func TestNewTableStyles(t *testing.T) {
	styles := NewTableStyles()
	assert.NotNil(t, styles)
	assert.NotNil(t, styles.StatusColors)
}

func TestNewOutputStyles(t *testing.T) {
	styles := NewOutputStyles()
	assert.NotNil(t, styles)
}
