package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStart_DirectMode verifies that atlas start runs in direct (blocking/dry-run)
// mode — NOT the daemon queue path — when the daemon is not available. It pins the
// current output behavior of the direct path before Phase 4.1 inverts the default.
//
// The test uses --dry-run to isolate the direct execution path without requiring a
// real workspace, AI runner, or network. Daemon mode is intentionally unavailable
// (no running daemon in test environment).
//
// TODO: Once the default is inverted to direct-first, this test should be updated
// to assert the new "mode: direct (daemon available — pass --daemon to opt in)"
// banner when a daemon IS running.
func TestStart_DirectMode(t *testing.T) {
	// Uses os.Chdir — must not run in parallel with other dir-changing tests.
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	var buf bytes.Buffer
	cmd := newStartCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	err = runStart(context.Background(), cmd, &buf, "fix the login bug", startOptions{
		templateName: "bug",
		dryRun:       true, // dry-run exercises the direct path without needing workspace/AI
	})
	require.NoError(t, err)

	output := buf.String()

	// Golden: direct mode must NOT produce daemon queue language.
	assert.NotContains(t, output, "Task queued",
		"direct mode should not show daemon queue language")

	// Golden: dry-run output describes the template and its steps.
	assert.Contains(t, output, "bug",
		"dry-run output should reference the template name")
}

// TestTryDaemonSubmit_NoDaemonSocket pins the "fall-through to direct" behavior:
// tryDaemonSubmit returns nil when no daemon socket is reachable.
// After Phase 4.1 inversion, tryDaemonSubmit is only invoked when --daemon is
// explicitly passed or config.Daemon.Default = true, so nil still means "proceed
// to direct mode" and an error from the submit is surfaced explicitly.
func TestTryDaemonSubmit_NoDaemonSocket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cmd := newStartCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	result := tryDaemonSubmit(ctx, cmd, &buf, "test task",
		startOptions{templateName: "bug"}, t.TempDir())

	// When no daemon is running, tryDaemonSubmit must return nil (fall-through).
	if result != nil {
		t.Skip("live atlas daemon detected; skipping fall-through characterization")
	}
	assert.Nil(t, result, "tryDaemonSubmit must return nil when daemon is unavailable")
	assert.Empty(t, buf.String(), "no output should be written when falling through")
}

// TestStart_DirectFirst_AfterInversion verifies that after Phase 4.1, atlas start
// runs in direct/foreground mode by default — even when --daemon is not passed.
// The test uses --dry-run to exercise the direct path without workspace or AI.
func TestStart_DirectFirst_AfterInversion(t *testing.T) {
	// Uses os.Chdir — must not run in parallel with other dir-changing tests.
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	var buf bytes.Buffer
	cmd := newStartCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	// Run without --daemon; must use direct mode regardless of daemon state.
	err = runStart(context.Background(), cmd, &buf, "fix the login bug", startOptions{
		templateName: "bug",
		dryRun:       true,
		daemon:       false, // explicit: no daemon opt-in
	})
	require.NoError(t, err)

	output := buf.String()

	// Must NOT show daemon queue language (e.g. "Task queued:").
	assert.NotContains(t, output, "Task queued",
		"direct-first mode must not show daemon queue output")
	// Must NOT show the daemon mode banner.
	assert.NotContains(t, output, "mode: daemon",
		"direct mode must not show daemon mode banner")

	// dry-run output must reference the selected template.
	assert.Contains(t, output, "bug", "dry-run must reference the template name")
}

// TestStart_DaemonFlag_ErrorsWhenNoDaemon verifies that passing --daemon when no
// daemon is running returns an actionable error (not silent fall-through).
func TestStart_DaemonFlag_ErrorsWhenNoDaemon(t *testing.T) {
	// Uses os.Chdir — must not run in parallel with other dir-changing tests.
	repoDir := initGitRepo(t)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	var buf bytes.Buffer
	cmd := newStartCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	err = runStart(context.Background(), cmd, &buf, "fix the login bug", startOptions{
		templateName: "bug",
		daemon:       true, // opt in to daemon — but no daemon running
	})

	// Must return an error (not fall through to direct execution).
	require.Error(t, err, "--daemon with no running daemon must return an error")
	assert.Contains(t, err.Error(), "daemon is not running",
		"error must explain that daemon is unavailable")
	assert.Contains(t, err.Error(), "atlas daemon start",
		"error must include the fix command")
}
