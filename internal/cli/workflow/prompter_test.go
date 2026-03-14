package workflow

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/template"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
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

// TestPrompter_SelectTemplate_WithNamedTemplate verifies the named-template fast-path
// that calls registry.Get directly without interactive selection.
func TestPrompter_SelectTemplate_WithNamedTemplate(t *testing.T) {
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	ctx := context.Background()

	// Build a registry with built-in templates (no custom templates).
	reg, err := template.NewRegistryWithConfig("", nil)
	require.NoError(t, err)

	tmpl, err := p.SelectTemplate(ctx, reg, "task", false, "text")
	require.NoError(t, err)
	assert.NotNil(t, tmpl)
	assert.Equal(t, "task", tmpl.Name)
}

// TestPrompter_SelectTemplate_WithBadTemplateName verifies that an unknown template
// name returns an error from registry.Get.
func TestPrompter_SelectTemplate_WithBadTemplateName(t *testing.T) {
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	ctx := context.Background()

	reg, err := template.NewRegistryWithConfig("", nil)
	require.NoError(t, err)

	_, err = p.SelectTemplate(ctx, reg, "no-such-template-xyz", false, "text")
	require.Error(t, err)
}

// TestSelectTemplate_Standalone_WithNamedTemplate exercises the standalone wrapper
// function (which previously had 0% coverage).
func TestSelectTemplate_Standalone_WithNamedTemplate(t *testing.T) {
	ctx := context.Background()

	reg, err := template.NewRegistryWithConfig("", nil)
	require.NoError(t, err)

	tmpl, err := SelectTemplate(ctx, reg, "task", true, "text")
	require.NoError(t, err)
	assert.NotNil(t, tmpl)
}

// TestPrompter_ResolveWorkspaceConflict_NotFound verifies that ResolveWorkspaceConflict
// returns the workspace name unchanged when the workspace does not exist.
func TestPrompter_ResolveWorkspaceConflict_NotFound(t *testing.T) {
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	ctx := context.Background()

	// Create a workspace store backed by a temp dir (no workspaces pre-created).
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)
	mgr := workspace.NewManager(wsStore, nil, zerolog.Nop())

	var buf bytes.Buffer
	name, err := p.ResolveWorkspaceConflict(ctx, mgr, "new-workspace", false, "text", &buf)
	require.NoError(t, err)
	assert.Equal(t, "new-workspace", name)
}

// TestPrompter_ResolveWorkspaceConflict_NonInteractiveJSON verifies the JSON non-interactive
// path that calls outputStartErrorJSON when the workspace exists.
func TestPrompter_ResolveWorkspaceConflict_NonInteractiveJSON(t *testing.T) {
	out := tui.NewOutput(io.Discard, "json")
	p := NewPrompter(out)
	ctx := context.Background()

	// Seed a workspace so Exists returns true.
	tmpDir := t.TempDir()
	wsStore, err := workspace.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)
	testWS := &domain.Workspace{
		Name:     "existing-ws",
		Status:   constants.WorkspaceStatusActive,
		RepoPath: tmpDir,
	}
	require.NoError(t, wsStore.Create(ctx, testWS))

	mgr := workspace.NewManager(wsStore, nil, zerolog.Nop())

	var buf bytes.Buffer
	_, err = p.ResolveWorkspaceConflict(ctx, mgr, "existing-ws", false, "json", &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, errJSONOutput)
}

// TestPrompter_ResolveWorkspaceConflict_NonTTY verifies the path taken when running in a
// non-TTY environment (e.g., CI / test) with an existing workspace and interactive mode not
// explicitly disabled. term.IsTerminal returns false, so we get a non-interactive error.
func TestPrompter_ResolveWorkspaceConflict_NonTTY(t *testing.T) {
	t.Parallel()
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	ctx := context.Background()

	tmpDir := t.TempDir()
	wsStore, err := workspace.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)
	testWS := &domain.Workspace{
		Name:     "nontty-ws",
		Status:   constants.WorkspaceStatusActive,
		RepoPath: tmpDir,
	}
	require.NoError(t, wsStore.Create(ctx, testWS))

	mgr := workspace.NewManager(wsStore, nil, zerolog.Nop())

	var buf bytes.Buffer
	// noInteractive=false and outputFormat="text": falls through to term.IsTerminal check.
	// In a test (non-TTY) environment, term.IsTerminal returns false, so this hits line 88.
	_, err = p.ResolveWorkspaceConflict(ctx, mgr, "nontty-ws", false, "text", &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use --workspace")
}

// TestPrompter_ResolveWorkspaceConflict_NonInteractiveText verifies the text non-interactive
// path (noInteractive=true) when the workspace exists.
func TestPrompter_ResolveWorkspaceConflict_NonInteractiveText(t *testing.T) {
	out := tui.NewOutput(io.Discard, "text")
	p := NewPrompter(out)
	ctx := context.Background()

	tmpDir := t.TempDir()
	wsStore, err := workspace.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)
	testWS := &domain.Workspace{
		Name:     "existing-ws-2",
		Status:   constants.WorkspaceStatusActive,
		RepoPath: tmpDir,
	}
	require.NoError(t, wsStore.Create(ctx, testWS))

	mgr := workspace.NewManager(wsStore, nil, zerolog.Nop())

	var buf bytes.Buffer
	_, err = p.ResolveWorkspaceConflict(ctx, mgr, "existing-ws-2", true, "text", &buf)
	require.Error(t, err)
	assert.ErrorIs(t, err, atlaserrors.ErrWorkspaceExists)
}
