//nolint:contextcheck // cobra commands use SetContext; the linter doesn't understand this pattern
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/backlog"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
)

// TestGetOutputFormat tests the getOutputFormat helper function.
func TestGetOutputFormat(t *testing.T) {
	t.Parallel()

	t.Run("returns JSON when jsonFlag is true", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{}
		result := getOutputFormat(cmd, true)
		assert.Equal(t, "json", result)
	})

	t.Run("returns empty when no output flag and jsonFlag is false", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{}
		result := getOutputFormat(cmd, false)
		assert.Empty(t, result)
	})

	t.Run("returns flag value when output flag is defined", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{}
		cmd.Flags().String("output", "json", "output format")
		result := getOutputFormat(cmd, false)
		assert.Equal(t, "json", result)
	})

	t.Run("jsonFlag takes precedence over output flag", func(t *testing.T) {
		t.Parallel()
		cmd := &cobra.Command{}
		cmd.Flags().String("output", "text", "output format")
		result := getOutputFormat(cmd, true)
		assert.Equal(t, "json", result)
	})
}

// setupTestBacklogDir creates a temporary directory for backlog tests.
func setupTestBacklogDir(t *testing.T) (string, *backlog.Manager) {
	t.Helper()
	tmpDir := t.TempDir()
	mgr, err := backlog.NewManager(tmpDir)
	require.NoError(t, err)
	return tmpDir, mgr
}

// createTestDiscovery creates a test discovery with the given title.
func createTestDiscovery(ctx context.Context, t *testing.T, mgr *backlog.Manager, title string) *backlog.Discovery {
	t.Helper()
	d := &backlog.Discovery{
		Title:  title,
		Status: backlog.StatusPending,
		Content: backlog.Content{
			Category:    backlog.CategoryBug,
			Severity:    backlog.SeverityMedium,
			Description: "Test description",
		},
		Context: backlog.Context{
			DiscoveredAt: time.Now().UTC(),
			DiscoveredBy: "human:tester",
		},
	}
	err := mgr.Add(ctx, d)
	require.NoError(t, err)
	return d
}

// TestBacklogListCommand tests the backlog list command.
// These tests cannot run in parallel because they use os.Chdir.
func TestBacklogListCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("lists discoveries in text format", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create test discoveries
		_ = createTestDiscovery(ctx, t, mgr, "First bug")
		_ = createTestDiscovery(ctx, t, mgr, "Second bug")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogListCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "First bug")
		assert.Contains(t, output, "Second bug")
		assert.Contains(t, output, "bug")
		assert.Contains(t, output, "medium")
	})

	t.Run("lists discoveries in JSON format", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create test discovery
		d := createTestDiscovery(ctx, t, mgr, "JSON test bug")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogListCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"--json"})

		err := cmd.Execute()
		require.NoError(t, err)

		var result []*backlog.Discovery
		err = json.Unmarshal(buf.Bytes(), &result) //nolint:musttag // Discovery has json tags
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, d.ID, result[0].ID)
		assert.Equal(t, "JSON test bug", result[0].Title)
	})

	t.Run("shows warning for malformed files", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create valid discovery
		_ = createTestDiscovery(ctx, t, mgr, "Valid discovery")

		// Create malformed file
		err := mgr.EnsureDir()
		require.NoError(t, err)
		malformedPath := filepath.Join(mgr.Dir(), "disc-broken.yaml")
		err = os.WriteFile(malformedPath, []byte("invalid: yaml: ["), 0o600)
		require.NoError(t, err)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogListCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Valid discovery")
		assert.Contains(t, output, "Skipping malformed file")
		assert.Contains(t, output, "disc-broken.yaml")
	})

	t.Run("empty backlog shows no discoveries message", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogListCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "No discoveries found")
	})
}

// TestBacklogViewCommand tests the backlog view command.
// These tests cannot run in parallel because they use os.Chdir.
func TestBacklogViewCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("views discovery in text format", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create test discovery with description
		d := &backlog.Discovery{
			Title:  "View test bug",
			Status: backlog.StatusPending,
			Content: backlog.Content{
				Category:    backlog.CategorySecurity,
				Severity:    backlog.SeverityHigh,
				Description: "This is a detailed description",
				Tags:        []string{"security", "auth"},
			},
			Context: backlog.Context{
				DiscoveredAt: time.Now().UTC(),
				DiscoveredBy: "human:tester",
			},
			Location: &backlog.Location{
				File: "main.go",
				Line: 42,
			},
		}
		err := mgr.Add(ctx, d)
		require.NoError(t, err)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogViewCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID})

		err = cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, d.ID)
		assert.Contains(t, output, "View test bug")
		assert.Contains(t, output, "security")
		assert.Contains(t, output, "high")
		assert.Contains(t, output, "main.go:42")
		assert.Contains(t, output, "security, auth")
	})

	t.Run("views discovery in JSON format", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		d := createTestDiscovery(ctx, t, mgr, "JSON view test")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogViewCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID, "--json"})

		err := cmd.Execute()
		require.NoError(t, err)

		var result backlog.Discovery
		err = json.Unmarshal(buf.Bytes(), &result) //nolint:musttag // Discovery has json tags
		require.NoError(t, err)
		assert.Equal(t, d.ID, result.ID)
		assert.Equal(t, "JSON view test", result.Title)
	})

	t.Run("returns error for non-existent discovery", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogViewCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"disc-notfnd"})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestBacklogPromoteCommand tests the backlog promote command.
// These tests cannot run in parallel because they use os.Chdir.
func TestBacklogPromoteCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("generates task config for pending discovery", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		d := createTestDiscovery(ctx, t, mgr, "Promote test")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogPromoteCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID, "--dry-run"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Dry-run")
		assert.Contains(t, output, d.ID)
		assert.Contains(t, output, "bugfix") // Bug category maps to bugfix
	})

	t.Run("returns ExitCode2Error for already promoted discovery", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create and promote a discovery
		d := createTestDiscovery(ctx, t, mgr, "Already promoted")
		_, err := mgr.Promote(ctx, d.ID, "task-old")
		require.NoError(t, err)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogPromoteCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID})

		err = cmd.Execute()
		require.Error(t, err)
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error")
	})

	t.Run("generates auto-config with dry-run", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		d := createTestDiscovery(ctx, t, mgr, "Auto config test")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogPromoteCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		// Use dry-run to test auto-config without actually creating task
		cmd.SetArgs([]string{d.ID, "--dry-run"})

		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		// Should show dry-run output with auto-generated config
		assert.Contains(t, output, "Dry-run")
		assert.Contains(t, output, "bugfix") // Bug category maps to bugfix template
	})
}

// TestBacklogDismissCommand tests the backlog dismiss command.
// These tests cannot run in parallel because they use os.Chdir.
func TestBacklogDismissCommand(t *testing.T) {
	ctx := context.Background()

	t.Run("dismisses pending discovery", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		d := createTestDiscovery(ctx, t, mgr, "Dismiss test")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogDismissCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID, "--reason", "duplicate issue"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Dismissed")
		assert.Contains(t, output, d.ID)
		assert.Contains(t, output, "duplicate issue")

		// Verify the discovery was actually dismissed
		updated, err := mgr.Get(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, backlog.StatusDismissed, updated.Status)
		assert.Equal(t, "duplicate issue", updated.Lifecycle.DismissedReason)
	})

	t.Run("returns ExitCode2Error for already dismissed discovery", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Create and dismiss a discovery
		d := createTestDiscovery(ctx, t, mgr, "Already dismissed")
		_, err := mgr.Dismiss(ctx, d.ID, "old reason")
		require.NoError(t, err)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogDismissCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID, "--reason", "new reason"})

		err = cmd.Execute()
		require.Error(t, err)
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error")
	})

	t.Run("requires reason flag", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		d := createTestDiscovery(ctx, t, mgr, "Missing reason")

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogDismissCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{d.ID})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reason")
	})
}

// TestBacklogAddFlagMode tests the backlog add command in flag mode.
// These tests cannot run in parallel because they use os.Chdir.
func TestBacklogAddFlagMode(t *testing.T) {
	ctx := context.Background()

	t.Run("adds discovery with required flags", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{
			"New test bug",
			"--category", "bug",
			"--severity", "high",
			"--description", "Test description",
			"--file", "main.go",
			"--line", "100",
		})

		err := cmd.Execute()
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "Created discovery")
		assert.Contains(t, output, "New test bug")
		assert.Contains(t, output, "bug")
		assert.Contains(t, output, "high")
		assert.Contains(t, output, "main.go:100")

		// Verify the discovery was created
		discoveries, _, err := mgr.List(ctx, backlog.Filter{})
		require.NoError(t, err)
		require.Len(t, discoveries, 1)
		assert.Equal(t, "New test bug", discoveries[0].Title)
		assert.Equal(t, backlog.CategoryBug, discoveries[0].Content.Category)
		assert.Equal(t, backlog.SeverityHigh, discoveries[0].Content.Severity)
		assert.Equal(t, "Test description", discoveries[0].Content.Description)
		require.NotNil(t, discoveries[0].Location)
		assert.Equal(t, "main.go", discoveries[0].Location.File)
		assert.Equal(t, 100, discoveries[0].Location.Line)
	})

	t.Run("adds discovery with tags", func(t *testing.T) {
		tmpDir, mgr := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{
			"Tagged bug",
			"--category", "security",
			"--severity", "critical",
			"--tags", "auth,login,urgent",
		})

		err := cmd.Execute()
		require.NoError(t, err)

		// Verify tags were set
		discoveries, _, err := mgr.List(ctx, backlog.Filter{})
		require.NoError(t, err)
		require.Len(t, discoveries, 1)
		assert.Equal(t, []string{"auth", "login", "urgent"}, discoveries[0].Content.Tags)
	})

	t.Run("returns ExitCode2Error for missing category", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"Missing category", "--severity", "high"})

		err := cmd.Execute()
		require.Error(t, err)
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error")
		assert.Contains(t, err.Error(), "category")
	})

	t.Run("returns ExitCode2Error for missing severity", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"Missing severity", "--category", "bug"})

		err := cmd.Execute()
		require.Error(t, err)
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error")
		assert.Contains(t, err.Error(), "severity")
	})

	t.Run("returns ExitCode2Error for invalid category", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"Invalid category", "--category", "invalid", "--severity", "high"})

		err := cmd.Execute()
		require.Error(t, err)
		assert.True(t, atlaserrors.IsExitCode2Error(err), "expected ExitCode2Error")
		assert.Contains(t, err.Error(), "invalid category")
	})

	t.Run("outputs JSON when --json flag is set", func(t *testing.T) {
		tmpDir, _ := setupTestBacklogDir(t)

		// Change to temp dir for the command
		origDir, _ := os.Getwd()
		defer func() { _ = os.Chdir(origDir) }()
		_ = os.Chdir(tmpDir)

		var buf bytes.Buffer
		cmd := newBacklogAddCmd()
		cmd.SetContext(ctx)
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{
			"JSON output test",
			"--category", "bug",
			"--severity", "low",
			"--json",
		})

		err := cmd.Execute()
		require.NoError(t, err)

		var result backlog.Discovery
		err = json.Unmarshal(buf.Bytes(), &result) //nolint:musttag // Discovery has json tags
		require.NoError(t, err)
		assert.Equal(t, "JSON output test", result.Title)
		assert.Equal(t, backlog.CategoryBug, result.Content.Category)
		assert.Equal(t, backlog.SeverityLow, result.Content.Severity)
	})
}

// TestGetGitHubUsername tests the getGitHubUsername helper function.
func TestGetGitHubUsername(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Save and clear relevant env vars
	origGitHubUser := os.Getenv("GITHUB_USER")
	origGitHubActor := os.Getenv("GITHUB_ACTOR")
	defer func() {
		_ = os.Setenv("GITHUB_USER", origGitHubUser)
		_ = os.Setenv("GITHUB_ACTOR", origGitHubActor)
	}()

	t.Run("GITHUB_USER env var takes priority", func(t *testing.T) {
		_ = os.Setenv("GITHUB_USER", "envuser")
		_ = os.Setenv("GITHUB_ACTOR", "actoruser")
		defer func() {
			_ = os.Unsetenv("GITHUB_USER")
			_ = os.Unsetenv("GITHUB_ACTOR")
		}()

		result := getGitHubUsername(ctx, tmpDir)
		assert.Equal(t, "envuser", result)
	})

	t.Run("GITHUB_ACTOR env var is second priority", func(t *testing.T) {
		_ = os.Unsetenv("GITHUB_USER")
		_ = os.Setenv("GITHUB_ACTOR", "actoruser")
		defer func() {
			_ = os.Unsetenv("GITHUB_ACTOR")
		}()

		result := getGitHubUsername(ctx, tmpDir)
		assert.Equal(t, "actoruser", result)
	})

	t.Run("falls back to OS username when env vars not set", func(t *testing.T) {
		_ = os.Unsetenv("GITHUB_USER")
		_ = os.Unsetenv("GITHUB_ACTOR")

		result := getGitHubUsername(ctx, tmpDir)
		// Should return OS username (not empty) since gh CLI likely not authenticated in test
		assert.NotEmpty(t, result, "should fall back to OS username")
	})
}

// TestGetGitHubUsernameViaCLI tests the getGitHubUsernameViaCLI helper function.
func TestGetGitHubUsernameViaCLI(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("returns empty string when gh is not installed or not authenticated", func(t *testing.T) {
		// In most test environments, gh won't be authenticated
		// This test verifies the function doesn't panic and handles errors gracefully
		result := getGitHubUsernameViaCLI(ctx, tmpDir)
		// Result could be empty (not authenticated) or a username (if authenticated)
		// We just verify it doesn't panic and returns a string
		assert.IsType(t, "", result)
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		result := getGitHubUsernameViaCLI(cancelCtx, tmpDir)
		assert.Empty(t, result, "should return empty on canceled context")
	})
}

// TestDetectDiscoverer tests the detectDiscoverer function.
func TestDetectDiscoverer(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Save original env vars
	origGitHubUser := os.Getenv("GITHUB_USER")
	origAtlasAgent := os.Getenv("ATLAS_AGENT")
	origAtlasModel := os.Getenv("ATLAS_MODEL")
	origClaudeCode := os.Getenv("CLAUDE_CODE")
	origAnthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	origAnthropicModel := os.Getenv("ANTHROPIC_MODEL")

	defer func() {
		_ = os.Setenv("GITHUB_USER", origGitHubUser)
		_ = os.Setenv("ATLAS_AGENT", origAtlasAgent)
		_ = os.Setenv("ATLAS_MODEL", origAtlasModel)
		_ = os.Setenv("CLAUDE_CODE", origClaudeCode)
		_ = os.Setenv("ANTHROPIC_API_KEY", origAnthropicKey)
		_ = os.Setenv("ANTHROPIC_MODEL", origAnthropicModel)
	}()

	// Clear all env vars before tests
	clearDiscovererEnvVars := func() {
		_ = os.Unsetenv("GITHUB_USER")
		_ = os.Unsetenv("GITHUB_ACTOR")
		_ = os.Unsetenv("ATLAS_AGENT")
		_ = os.Unsetenv("ATLAS_MODEL")
		_ = os.Unsetenv("CLAUDE_CODE")
		_ = os.Unsetenv("ANTHROPIC_API_KEY")
		_ = os.Unsetenv("ANTHROPIC_MODEL")
	}

	t.Run("interactive mode returns human with GitHub username", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("GITHUB_USER", "testuser")
		defer func() { _ = os.Unsetenv("GITHUB_USER") }()

		result := detectDiscoverer(ctx, tmpDir, true)
		assert.Equal(t, "human:testuser", result)
	})

	t.Run("interactive mode lowercases username", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("GITHUB_USER", "TestUser")
		defer func() { _ = os.Unsetenv("GITHUB_USER") }()

		result := detectDiscoverer(ctx, tmpDir, true)
		assert.Equal(t, "human:testuser", result)
	})

	t.Run("flag mode with ATLAS_AGENT and ATLAS_MODEL returns AI format", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("ATLAS_AGENT", "custom-agent")
		_ = os.Setenv("ATLAS_MODEL", "custom-model")
		defer func() {
			_ = os.Unsetenv("ATLAS_AGENT")
			_ = os.Unsetenv("ATLAS_MODEL")
		}()

		result := detectDiscoverer(ctx, tmpDir, false)
		assert.Equal(t, "ai:custom-agent:custom-model", result)
	})

	t.Run("flag mode with CLAUDE_CODE returns Claude format", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("CLAUDE_CODE", "1")
		_ = os.Setenv("ANTHROPIC_MODEL", "claude-sonnet-4")
		defer func() {
			_ = os.Unsetenv("CLAUDE_CODE")
			_ = os.Unsetenv("ANTHROPIC_MODEL")
		}()

		result := detectDiscoverer(ctx, tmpDir, false)
		assert.Equal(t, "ai:claude-code:claude-sonnet-4", result)
	})

	t.Run("flag mode with ANTHROPIC_API_KEY returns Claude format", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
		defer func() { _ = os.Unsetenv("ANTHROPIC_API_KEY") }()

		result := detectDiscoverer(ctx, tmpDir, false)
		assert.Equal(t, "ai:claude-code:unknown", result)
	})

	t.Run("flag mode without AI env vars falls back to human", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("GITHUB_USER", "fallbackuser")
		defer func() { _ = os.Unsetenv("GITHUB_USER") }()

		result := detectDiscoverer(ctx, tmpDir, false)
		assert.Equal(t, "human:fallbackuser", result)
	})

	t.Run("ATLAS_AGENT requires ATLAS_MODEL to use AI format", func(t *testing.T) {
		clearDiscovererEnvVars()
		_ = os.Setenv("ATLAS_AGENT", "orphan-agent")
		_ = os.Setenv("GITHUB_USER", "humanuser")
		defer func() {
			_ = os.Unsetenv("ATLAS_AGENT")
			_ = os.Unsetenv("GITHUB_USER")
		}()

		result := detectDiscoverer(ctx, tmpDir, false)
		// Without ATLAS_MODEL, should fall through to human
		assert.Equal(t, "human:humanuser", result)
	})
}
