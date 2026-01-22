package workflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPrompter(t *testing.T) {
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	assert.NotNil(t, p)
	assert.Equal(t, out, p.out)
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid name",
			input:   "my-workspace",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "valid name with spaces",
			input:   "my workspace",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspaceName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				// Should wrap ErrEmptyValue
				assert.ErrorIs(t, err, atlaserrors.ErrEmptyValue)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeWorkspaceName_ExportedFunction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic sanitization",
			input: "Test Workspace",
			want:  "test-workspace",
		},
		{
			name:  "special characters",
			input: "test@#$workspace",
			want:  "testworkspace",
		},
		{
			name:  "multiple hyphens",
			input: "test---workspace",
			want:  "test-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeWorkspaceName(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOutputStartErrorJSON(t *testing.T) {
	t.Run("returns error with JSON sentinel", func(t *testing.T) {
		var buf bytes.Buffer
		err := outputStartErrorJSON(&buf, "test-workspace", "", "test error message")

		require.ErrorIs(t, err, errJSONOutput)
		assert.Contains(t, err.Error(), "test error message")
	})
}

func TestSelectTemplate_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(_ *testing.T) {
		// We can't easily test SelectTemplate without a real registry
		// since it will try to call registry.Get() which panics on nil
		// Instead, we verify the function signature compiles
		ctx := context.Background()
		_ = ctx
		// Just verify the function exists by referencing it
		_ = SelectTemplate
	})
}

func TestPrompter_SelectTemplate_NonInteractive(t *testing.T) {
	t.Run("returns error when no template specified in non-interactive mode", func(t *testing.T) {
		out := tui.NewOutput(io.Discard, "text")
		p := NewPrompter(out)
		ctx := context.Background()

		_, err := p.SelectTemplate(ctx, nil, "", true, "text")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTemplateRequired)
	})

	t.Run("returns error when no template specified with JSON output", func(t *testing.T) {
		out := tui.NewOutput(io.Discard, "json")
		p := NewPrompter(out)
		ctx := context.Background()

		_, err := p.SelectTemplate(ctx, nil, "", false, "json")
		require.Error(t, err)
		assert.ErrorIs(t, err, atlaserrors.ErrTemplateRequired)
	})
}

func TestPrompter_SelectTemplate_ContextCancellation(t *testing.T) {
	t.Run("returns context error when context is canceled", func(t *testing.T) {
		out := tui.NewOutput(io.Discard, "text")
		p := NewPrompter(out)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := p.SelectTemplate(ctx, nil, "", false, "text")
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestPrompter_ResolveWorkspaceConflict_ContextCancellation(t *testing.T) {
	t.Run("returns context error when context is canceled", func(t *testing.T) {
		out := tui.NewOutput(io.Discard, "text")
		p := NewPrompter(out)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		var buf bytes.Buffer
		_, err := p.ResolveWorkspaceConflict(ctx, nil, "test-ws", false, "text", &buf)
		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestPrompter_ResolveWorkspaceConflict_MethodExists(t *testing.T) {
	t.Run("method exists with correct signature", func(t *testing.T) {
		// Testing ResolveWorkspaceConflict requires a real workspace manager
		// which is beyond the scope of unit testing this specific method.
		// Instead, we verify the method exists and has the correct signature
		out := tui.NewOutput(io.Discard, "text")
		p := NewPrompter(out)
		assert.NotNil(t, p)

		// Verify the method exists by checking it compiles
		_ = p.ResolveWorkspaceConflict
	})
}

func TestErrJSONOutput(t *testing.T) {
	t.Run("sentinel error exists and is matchable", func(t *testing.T) {
		require.Error(t, errJSONOutput)
		assert.Equal(t, "JSON output error", errJSONOutput.Error())

		// Test that errors wrapping it can be matched
		wrappedErr := fmt.Errorf("wrapped: %w", errJSONOutput)
		require.Error(t, wrappedErr)
		require.ErrorIs(t, wrappedErr, errJSONOutput)
		assert.Contains(t, wrappedErr.Error(), errJSONOutput.Error())
	})
}
