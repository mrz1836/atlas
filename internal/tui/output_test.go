package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestOutputInterface_TTYOutput tests TTYOutput implements the Output interface.
func TestOutputInterface_TTYOutput(t *testing.T) {
	var buf bytes.Buffer
	var out Output = NewTTYOutput(&buf)
	assert.NotNil(t, out)
}

// TestOutputInterface_JSONOutput tests JSONOutput implements the Output interface.
func TestOutputInterface_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	var out Output = NewJSONOutput(&buf)
	assert.NotNil(t, out)
}

func TestTTYOutput_Success(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	out.Success("test message")
	output := buf.String()
	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "test message")
}

func TestTTYOutput_Error(t *testing.T) {
	t.Run("standard error", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Error(atlaserrors.ErrWorkspaceNotFound)
		output := buf.String()
		assert.Contains(t, output, "✗")
		assert.Contains(t, output, "not found")
	})

	t.Run("actionable error with suggestion", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		err := NewActionableError("config not found", "Run: atlas init")
		out.Error(err)
		output := buf.String()
		assert.Contains(t, output, "✗")
		assert.Contains(t, output, "config not found")
		assert.Contains(t, output, "▸ Try:")
		assert.Contains(t, output, "atlas init")
	})

	t.Run("actionable error with context", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		err := NewActionableError("file not found", "Check the path").
			WithContext("/path/to/file")
		out.Error(err)
		output := buf.String()
		assert.Contains(t, output, "✗")
		assert.Contains(t, output, "file not found")
		assert.Contains(t, output, "/path/to/file")
		assert.Contains(t, output, "▸ Try:")
		assert.Contains(t, output, "Check the path")
	})

	t.Run("actionable error with empty suggestion", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		err := NewActionableError("something went wrong", "")
		out.Error(err)
		output := buf.String()
		assert.Contains(t, output, "✗")
		assert.Contains(t, output, "something went wrong")
		assert.NotContains(t, output, "▸ Try:")
	})
}

func TestTTYOutput_Warning(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	out.Warning("test warning")
	output := buf.String()
	assert.Contains(t, output, "⚠")
	assert.Contains(t, output, "test warning")
}

func TestTTYOutput_Info(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	out.Info("test info")
	output := buf.String()
	assert.Contains(t, output, "ℹ")
	assert.Contains(t, output, "test info")
}

func TestTTYOutput_Text(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	out.Text("plain text message")
	output := buf.String()
	assert.Contains(t, output, "plain text message")
	assert.NotContains(t, output, "ℹ")
	assert.NotContains(t, output, "✓")
	assert.NotContains(t, output, "⚠")
	assert.NotContains(t, output, "✗")
}

func TestTTYOutput_Table(t *testing.T) {
	t.Run("basic table", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Table([]string{"Name", "Status"}, [][]string{
			{"ws1", "active"},
			{"ws2", "paused"},
		})
		output := buf.String()
		assert.Contains(t, output, "Name")
		assert.Contains(t, output, "Status")
		assert.Contains(t, output, "ws1")
		assert.Contains(t, output, "active")
		assert.Contains(t, output, "ws2")
		assert.Contains(t, output, "paused")
	})

	t.Run("empty table", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Table([]string{}, [][]string{})
		assert.Empty(t, buf.String())
	})

	t.Run("table with short row", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Table([]string{"A", "B", "C"}, [][]string{
			{"1"}, // Short row - should handle gracefully
		})
		output := buf.String()
		assert.Contains(t, output, "A")
		assert.Contains(t, output, "B")
		assert.Contains(t, output, "C")
		assert.Contains(t, output, "1")
	})

	t.Run("table with unicode", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.Table([]string{"Icon", "Text"}, [][]string{
			{"✓", "Success"},
			{"⚠", "Warning"},
		})
		output := buf.String()
		assert.Contains(t, output, "✓")
		assert.Contains(t, output, "⚠")
	})
}

func TestTTYOutput_JSON(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	err := out.JSON(map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "key")
	assert.Contains(t, buf.String(), "value")
}

func TestTTYOutput_Spinner(t *testing.T) {
	var buf bytes.Buffer
	out := NewTTYOutput(&buf)
	ctx := context.Background()
	spinner := out.Spinner(ctx, "Loading...")
	assert.NotNil(t, spinner)
	spinner.Update("Still loading...")
	spinner.Stop()
}

func TestJSONOutput_Success(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)
	out.Success("test message")

	var result jsonMessage
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "success", result.Type)
	assert.Equal(t, "test message", result.Message)
}

func TestJSONOutput_Error(t *testing.T) {
	t.Run("simple error", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Error(atlaserrors.ErrWorkspaceNotFound)

		var result jsonError
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "error", result.Type)
		assert.Contains(t, result.Message, "not found")
		assert.Empty(t, result.Details) // No wrapped error
	})

	t.Run("wrapped error includes details", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		wrappedErr := fmt.Errorf("operation failed: %w", atlaserrors.ErrWorkspaceNotFound)
		out.Error(wrappedErr)

		var result jsonError
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "error", result.Type)
		assert.Contains(t, result.Message, "operation failed")
		assert.Contains(t, result.Details, "not found") // Wrapped error as details
	})

	t.Run("actionable error with suggestion", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		actionErr := NewActionableError("config not found", "Run: atlas init")
		out.Error(actionErr)

		var result jsonError
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "error", result.Type)
		assert.Equal(t, "config not found", result.Message)
		assert.Equal(t, "Run: atlas init", result.Suggestion)
		assert.Empty(t, result.Context)
	})

	t.Run("actionable error with context", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		actionErr := NewActionableError("file not found", "Check the path").
			WithContext("/path/to/file")
		out.Error(actionErr)

		var result jsonError
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "error", result.Type)
		assert.Contains(t, result.Message, "file not found")
		assert.Contains(t, result.Message, "/path/to/file") // Context in message
		assert.Equal(t, "Check the path", result.Suggestion)
		assert.Equal(t, "/path/to/file", result.Context)
	})

	t.Run("actionable error with empty suggestion", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		actionErr := NewActionableError("something went wrong", "")
		out.Error(actionErr)

		var result jsonError
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "error", result.Type)
		assert.Equal(t, "something went wrong", result.Message)
		assert.Empty(t, result.Suggestion)
	})
}

func TestJSONOutput_Warning(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)
	out.Warning("test warning")

	var result jsonMessage
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "warning", result.Type)
	assert.Equal(t, "test warning", result.Message)
}

func TestJSONOutput_Info(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)
	out.Info("test info")

	var result jsonMessage
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "info", result.Type)
	assert.Equal(t, "test info", result.Message)
}

func TestJSONOutput_Text(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)
	out.Text("plain text message")

	var result jsonMessage
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "text", result.Type)
	assert.Equal(t, "plain text message", result.Message)
}

func TestJSONOutput_Table(t *testing.T) {
	t.Run("basic table", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Table([]string{"workspace", "branch", "status"}, [][]string{
			{"auth", "feat/auth", "running"},
			{"payment", "fix/payment", "awaiting_approval"},
		})

		var result []map[string]string
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		require.Len(t, result, 2)

		assert.Equal(t, "auth", result[0]["workspace"])
		assert.Equal(t, "feat/auth", result[0]["branch"])
		assert.Equal(t, "running", result[0]["status"])

		assert.Equal(t, "payment", result[1]["workspace"])
		assert.Equal(t, "fix/payment", result[1]["branch"])
		assert.Equal(t, "awaiting_approval", result[1]["status"])
	})

	t.Run("empty table", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Table([]string{}, [][]string{})

		var result []map[string]string
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("table with missing values", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.Table([]string{"A", "B", "C"}, [][]string{
			{"1", "2"}, // Missing C
		})

		var result []map[string]string
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "1", result[0]["A"])
		assert.Equal(t, "2", result[0]["B"])
		assert.Empty(t, result[0]["C"]) // Empty string for missing value
	})
}

func TestJSONOutput_JSON(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)

	data := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}
	err := out.JSON(data)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
	assert.InDelta(t, float64(42), result["count"], 0.001) // JSON numbers are float64
}

func TestJSONOutput_Spinner(t *testing.T) {
	var buf bytes.Buffer
	out := NewJSONOutput(&buf)
	ctx := context.Background()
	spinner := out.Spinner(ctx, "Loading...")

	// NoopSpinner should do nothing
	assert.NotNil(t, spinner)
	_, ok := spinner.(*NoopSpinner)
	assert.True(t, ok)

	// These should not panic or produce output
	spinner.Update("Updated")
	spinner.Stop()
	assert.Empty(t, buf.String())
}

func TestNewOutput_FormatSelection(t *testing.T) {
	t.Run("json format returns JSONOutput", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutput(&buf, FormatJSON)
		_, ok := out.(*JSONOutput)
		assert.True(t, ok)
	})

	t.Run("text format returns TTYOutput", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewOutput(&buf, FormatText)
		_, ok := out.(*TTYOutput)
		assert.True(t, ok)
	})

	t.Run("empty format auto-detects non-TTY as JSON", func(t *testing.T) {
		var buf bytes.Buffer
		// bytes.Buffer is not a TTY, so should return JSONOutput
		out := NewOutput(&buf, FormatAuto)
		_, ok := out.(*JSONOutput)
		assert.True(t, ok)
	})
}

func TestIsTTY(t *testing.T) {
	t.Run("bytes.Buffer is not TTY", func(t *testing.T) {
		var buf bytes.Buffer
		assert.False(t, isTTY(&buf))
	})

	t.Run("nil file is not TTY", func(t *testing.T) {
		assert.False(t, isTTY(nil))
	})

	t.Run("DevNull is not TTY", func(t *testing.T) {
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Skip("Cannot open /dev/null")
		}
		defer func() { _ = f.Close() }()
		assert.False(t, isTTY(f))
	})
}

func TestFormatConstants(t *testing.T) {
	assert.Empty(t, FormatAuto)
	assert.Equal(t, FormatText, "text")
	//nolint:testifylint // Linter incorrectly suggests JSONEq for non-JSON string comparison
	require.Equal(t, FormatJSON, "json")
}

func TestNoopSpinner(_ *testing.T) {
	spinner := &NoopSpinner{}

	// All methods should be no-ops
	spinner.Update("test")
	spinner.Stop()

	// Should be usable as Spinner interface
	var s Spinner = spinner
	s.Update("test")
	s.Stop()
}

func TestSpinnerAdapter(t *testing.T) {
	var buf bytes.Buffer
	ctx := context.Background()
	adapter := NewSpinnerAdapter(ctx, &buf, "Loading...")

	// Should be usable as Spinner interface
	var s Spinner = adapter
	assert.NotNil(t, s)

	// Update should work
	adapter.Update("Updated message")

	// Stop should not panic
	adapter.Stop()

	// Calling Stop multiple times should not panic
	adapter.Stop()
}

func TestJSONOutput_URL(t *testing.T) {
	t.Run("url with display text", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.URL("https://github.com/mrz1836/atlas", "Atlas Repository")

		var result jsonURL
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "url", result.Type)
		assert.Equal(t, "https://github.com/mrz1836/atlas", result.URL)
		assert.Equal(t, "Atlas Repository", result.Display)
	})

	t.Run("url without display text", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		out.URL("https://github.com/mrz1836/atlas", "")

		var result jsonURL
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "url", result.Type)
		assert.Equal(t, "https://github.com/mrz1836/atlas", result.URL)
		assert.Empty(t, result.Display)
	})

	t.Run("url with display text same as url", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewJSONOutput(&buf)
		url := "https://github.com/mrz1836/atlas"
		out.URL(url, url)

		var result jsonURL
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
		assert.Equal(t, "url", result.Type)
		assert.Equal(t, url, result.URL)
		// Display should be omitted when same as URL (per AC: #147 line 154)
		assert.Empty(t, result.Display)
	})
}

func TestTTYOutput_URL(t *testing.T) {
	t.Run("url with display text", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://github.com/mrz1836/atlas", "Atlas Repository")

		output := buf.String()
		// Should contain display text
		assert.Contains(t, output, "Atlas Repository")
		// If hyperlinks supported, may use OSC 8, otherwise shows URL separately
		assert.Contains(t, output, "https://github.com/mrz1836/atlas")
	})

	t.Run("url without display text", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://github.com/mrz1836/atlas", "")

		output := buf.String()
		// Should contain URL (as both URL and display text when empty)
		assert.Contains(t, output, "https://github.com/mrz1836/atlas")
	})

	t.Run("url with same display text as url", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		url := "https://example.com"
		out.URL(url, url)

		output := buf.String()
		// Should contain the URL
		assert.Contains(t, output, url)
		// Should not duplicate the URL in fallback format (no parentheses with same content)
		assert.NotContains(t, output, url+" ("+url+")")
	})

	t.Run("url with different display text", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://example.com/very/long/path", "Short Link")

		output := buf.String()
		// Should contain both display text and URL
		assert.Contains(t, output, "Short Link")
		assert.Contains(t, output, "https://example.com/very/long/path")
	})

	t.Run("url output is formatted", func(t *testing.T) {
		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://example.com", "Example")

		output := buf.String()
		// Output should be non-empty and formatted
		assert.NotEmpty(t, output)
		// Should have leading whitespace (indentation)
		assert.Contains(t, output, "  ")
		// Should have newline
		assert.Contains(t, output, "\n")
	})

	t.Run("url with hyperlink support enabled", func(t *testing.T) {
		// Set environment variable to enable hyperlink support
		t.Setenv("TERM_PROGRAM", "iTerm.app")

		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://example.com", "Click Here")

		output := buf.String()
		// Should contain the display text
		assert.Contains(t, output, "Click Here")
		// When hyperlinks are supported, OSC 8 escape sequence is used
		// OSC 8 format: \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\
		// We just verify the URL is in the output
		assert.Contains(t, output, "https://example.com")
	})

	t.Run("url without hyperlink support uses fallback", func(t *testing.T) {
		// Clear environment variables that would enable hyperlink support
		t.Setenv("TERM_PROGRAM", "")
		t.Setenv("LC_TERMINAL", "")

		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		out.URL("https://example.com/path", "Link Text")

		output := buf.String()
		// In fallback mode with different display text, should show: "Link Text (URL)"
		assert.Contains(t, output, "Link Text")
		assert.Contains(t, output, "https://example.com/path")
		// Should have parentheses in fallback format
		assert.Contains(t, output, "(")
		assert.Contains(t, output, ")")
	})

	t.Run("url without hyperlink support and same display text", func(t *testing.T) {
		// Clear environment variables
		t.Setenv("TERM_PROGRAM", "")
		t.Setenv("LC_TERMINAL", "")

		var buf bytes.Buffer
		out := NewTTYOutput(&buf)
		url := "https://short.url"
		out.URL(url, url)

		output := buf.String()
		// When display text equals URL in fallback mode, should not show "(URL)"
		assert.Contains(t, output, url)
		// Count occurrences - should appear only once in the visible text
		// (may have ANSI codes in between, but logically appears once)
		assert.NotContains(t, output, url+" ("+url+")")
	})
}
