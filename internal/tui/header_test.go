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

			// Wide mode should render ASCII art (contains block characters)
			assert.Contains(t, result, "▄▀█", "should contain ASCII art block characters")
			assert.Contains(t, result, "█▀█", "should contain ASCII art block characters")

			// Should NOT contain narrow header markers
			assert.NotContains(t, result, "═══", "should not contain narrow header markers")
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
			assert.NotContains(t, result, "▄▀█", "should not contain ASCII art")
		})
	}
}

func TestHeader_Render_Centering(t *testing.T) {
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

			// Each line should have leading spaces for centering
			lines := strings.Split(result, "\n")
			for _, line := range lines {
				if len(line) > 0 {
					// Lines should start with spaces (centered content)
					assert.True(t, strings.HasPrefix(line, " ") || len(strings.TrimSpace(stripANSI(line))) == 0,
						"line should be centered with leading spaces: %q", line)
				}
			}
		})
	}
}

func TestHeader_Render_CenteringAccuracy(t *testing.T) {
	// Test that centering calculation is accurate using rune width, not byte length.
	// The ASCII art constant has 22 runes total (4 leading spaces + 18 art chars).
	// Centering adds padding = (width - 22) / 2.
	// Total leading spaces = padding + 4 (the existing spaces in the constant).
	tests := []struct {
		name            string
		width           int
		expectedPadding int // Expected total leading spaces (padding + 4 embedded spaces)
	}{
		{"80 columns", 80, 33},   // (80-22)/2 = 29, + 4 embedded = 33
		{"100 columns", 100, 43}, // (100-22)/2 = 39, + 4 embedded = 43
		{"120 columns", 120, 53}, // (120-22)/2 = 49, + 4 embedded = 53
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHeader(tc.width)
			result := h.Render()

			lines := strings.Split(result, "\n")
			require.GreaterOrEqual(t, len(lines), 1, "should have at least one line")

			// Count leading spaces on first line
			firstLine := stripANSI(lines[0])
			leadingSpaces := 0
			for _, r := range firstLine {
				if r == ' ' {
					leadingSpaces++
				} else {
					break
				}
			}

			assert.Equal(t, tc.expectedPadding, leadingSpaces,
				"centering should use rune width (22 chars), not byte length (50 bytes)")
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
		{"wide mode with NO_COLOR", 80, "▄▀█"},     // ASCII art uses block characters
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
	assert.Contains(t, result, "▄▀█", "wide mode should show ASCII art")
}

func TestRenderHeaderAuto(t *testing.T) {
	// Test auto width detection (may be narrow in test environment)
	result := RenderHeaderAuto()
	assert.NotEmpty(t, result)
	// Should contain ATLAS in some form
	assert.Contains(t, result, "A")
}

func TestGetTerminalWidth(t *testing.T) {
	// This test just verifies the function doesn't panic
	width := GetTerminalWidth()
	// Width should be 0 or a reasonable terminal width
	assert.GreaterOrEqual(t, width, 0)
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		name       string
		styled     string
		original   string
		totalWidth int
		wantPrefix string
	}{
		{
			name:       "center 5-char text in 10 cols",
			styled:     "hello",
			original:   "hello",
			totalWidth: 10,
			wantPrefix: "  ", // (10-5)/2 = 2 spaces
		},
		{
			name:       "text wider than width",
			styled:     "hello world",
			original:   "hello world",
			totalWidth: 5,
			wantPrefix: "", // No padding when text wider
		},
		{
			name:       "zero width",
			styled:     "test",
			original:   "test",
			totalWidth: 0,
			wantPrefix: "", // No padding for zero width
		},
		{
			name:       "negative width",
			styled:     "test",
			original:   "test",
			totalWidth: -10,
			wantPrefix: "", // No padding for negative width
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := centerText(tc.styled, tc.original, tc.totalWidth)
			if tc.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(result, tc.wantPrefix),
					"expected prefix %q, got %q", tc.wantPrefix, result)
			}
			assert.Contains(t, result, tc.styled)
		})
	}
}

// stripANSI removes ANSI escape codes from a string for testing.
func stripANSI(s string) string {
	// Simple ANSI stripping for test purposes
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
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
	assert.Contains(t, narrowResult, "═══")
	assert.NotContains(t, narrowResult, "▄▀█")

	// 80 should be wide (ASCII art)
	assert.Contains(t, wideResult, "▄▀█")
	assert.NotContains(t, wideResult, "═══")
}
