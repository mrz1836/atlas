package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	columns := []TableColumn{
		{Name: "NAME", Width: 10, Align: AlignLeft},
		{Name: "VALUE", Width: 15, Align: AlignLeft},
		{Name: "COUNT", Width: 5, Align: AlignRight},
	}

	t.Run("WriteHeader", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteHeader()
		output := buf.String()
		assert.Contains(t, output, "NAME")
		assert.Contains(t, output, "VALUE")
		assert.Contains(t, output, "COUNT")
	})

	t.Run("WriteRow", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test", "value", "42")
		output := buf.String()
		assert.Contains(t, output, "test")
		assert.Contains(t, output, "value")
		assert.Contains(t, output, "42")
	})

	t.Run("WriteRow truncates long values", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("verylongname", "value", "42")
		output := buf.String()
		assert.Contains(t, output, "verylongn…")
	})

	t.Run("WriteRow handles missing values", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		assert.Contains(t, output, "test")
	})

	t.Run("WriteStyledRow", func(t *testing.T) {
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		// Simulate a styled value with ANSI codes
		styledValue := "\x1b[34mactive\x1b[0m"
		plainValue := "active"
		table.WriteStyledRow([]string{"test", plainValue, "5"}, 1, styledValue, plainValue)
		output := buf.String()
		assert.Contains(t, output, "test")
		assert.Contains(t, output, styledValue)
	})
}

func TestColorOffset(t *testing.T) {
	tests := []struct {
		name     string
		rendered string
		plain    string
		expected int
	}{
		{
			name:     "no color",
			rendered: "active",
			plain:    "active",
			expected: 0,
		},
		{
			name:     "with ANSI codes",
			rendered: "\x1b[34mactive\x1b[0m",
			plain:    "active",
			expected: 9, // len("\x1b[34m") + len("\x1b[0m") = 5 + 4 = 9
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ColorOffset(tc.rendered, tc.plain)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAlignment(t *testing.T) {
	t.Run("AlignLeft", func(t *testing.T) {
		columns := []TableColumn{
			{Name: "LEFT", Width: 10, Align: AlignLeft},
		}
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		// Left aligned: "test      \n"
		assert.Contains(t, output, "test      ")
	})

	t.Run("AlignRight", func(t *testing.T) {
		columns := []TableColumn{
			{Name: "RIGHT", Width: 10, Align: AlignRight},
		}
		var buf bytes.Buffer
		table := NewTable(&buf, columns)
		table.WriteRow("test")
		output := buf.String()
		// Right aligned: "      test\n"
		assert.Contains(t, output, "      test")
	})
}
