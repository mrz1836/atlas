package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHeader(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"zero width", 0},
		{"negative width", -10},
		{"narrow width", 40},
		{"threshold width", 80},
		{"wide width", 120},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			require.NotNil(t, h)
			assert.Equal(t, tc.width, h.width)
		})
	}
}

func TestHeader_WithWidth(t *testing.T) {
	h := NewHeader(80)
	h2 := h.WithWidth(120)

	// Original unchanged
	assert.Equal(t, 80, h.width)
	// New has updated width
	assert.Equal(t, 120, h2.width)
}

func TestHeader_Render_WideMode(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"exactly 80 columns", 80},
		{"100 columns", 100},
		{"120 columns", 120},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()

			// Wide mode should render ASCII art (contains block characters from new logo)
			assert.Contains(t, result, "█████╗", "should contain ASCII art block characters")
			assert.Contains(t, result, "╚═╝", "should contain ASCII art block characters")

			// Should NOT contain narrow header markers
			assert.NotContains(t, result, "═══ ATLAS ═══", "should not contain narrow header")
		})
	}
}

func TestHeader_Render_NarrowMode(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"79 columns (just under threshold)", 79},
		{"40 columns", 40},
		{"20 columns", 20},
		{"zero width (fallback)", 0},
		{"negative width (fallback)", -10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()

			// Narrow mode should use simple text
			assert.Contains(t, result, "ATLAS", "should contain ATLAS text")
			assert.Contains(t, result, "═", "should contain box-drawing characters")

			// Should NOT contain ASCII art block characters
			assert.NotContains(t, result, "█████╗", "should not contain ASCII art")
		})
	}
}

func TestHeader_Render_LeftAligned(t *testing.T) {
	// Header should be left-aligned (no centering padding)
	tests := []struct {
		name  string
		width int
	}{
		{"80 columns", 80},
		{"100 columns", 100},
		{"120 columns", 120},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()

			lines := strings.Split(result, "\n")
			require.GreaterOrEqual(t, len(lines), 1, "should have at least one line")

			// First line should start with the ASCII art (single leading space from the constant)
			firstLine := stripANSI(lines[0])
			leadingSpaces := 0
			for _, r := range firstLine {
				if r == ' ' {
					leadingSpaces++
				} else {
					break
				}
			}

			// Should only have 1 leading space (embedded in the ASCII art constant)
			assert.Equal(t, 1, leadingSpaces, "header should be left-aligned with minimal leading space")
		})
	}
}

func TestHeader_Render_NOCOLORSupport(t *testing.T) {
	// Set NO_COLOR and check it doesn't break rendering
	t.Setenv("NO_COLOR", "1")
	CheckNoColor() // Apply the NO_COLOR setting

	tests := []struct {
		name     string
		width    int
		contains string // What the output should contain
	}{
		{"wide mode with NO_COLOR", 80, "█████╗"},  // ASCII art uses block characters
		{"narrow mode with NO_COLOR", 40, "ATLAS"}, // Narrow mode uses text
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()

			// Should still render valid content
			assert.NotEmpty(t, result)
			assert.Contains(t, result, tc.contains)
		})
	}
}

func TestHeader_Render_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		expected string
	}{
		{"zero width returns narrow header", 0, "ATLAS"},
		{"negative width returns narrow header", -100, "ATLAS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()
			assert.Contains(t, result, tc.expected)
		})
	}
}

func TestRenderHeader(t *testing.T) {
	// Test the convenience function
	result := RenderHeader(100)
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "█████╗", "wide mode should show ASCII art")
}

func TestRenderHeaderAuto(t *testing.T) {
	// Test auto width detection (may be narrow in test environment)
	result := RenderHeaderAuto()
	assert.NotEmpty(t, result)
	// Should contain ATLAS in some form
	assert.Contains(t, result, "A")
}

func TestTerminalWidth(t *testing.T) {
	// This test just verifies the function doesn't panic
	width := TerminalWidth()
	// Width should be 0 or a reasonable terminal width
	assert.GreaterOrEqual(t, width, 0)
}

func TestHeader_ConsistentOutput(t *testing.T) {
	// Verify same width produces same output
	h1 := NewHeader(100)
	h2 := NewHeader(100)

	result1 := h1.Render()
	result2 := h2.Render()

	assert.Equal(t, result1, result2, "same width should produce identical output")
}

func TestWideThreshold(t *testing.T) {
	// Test exactly at the threshold boundary
	narrowH := NewHeader(79)
	wideH := NewHeader(80)

	narrowResult := narrowH.Render()
	wideResult := wideH.Render()

	// 79 should be narrow (simple text)
	assert.Contains(t, narrowResult, "═══ ATLAS ═══")
	assert.NotContains(t, narrowResult, "█████╗")

	// 80 should be wide (ASCII art)
	assert.Contains(t, wideResult, "█████╗")
	assert.NotContains(t, wideResult, "═══ ATLAS ═══")
}
