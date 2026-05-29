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
// TODO(T-228 phase 4): After Phase 4.1 inverts the default to direct-first, this
// test should be updated to assert the new "mode: direct (daemon available — pass
// --daemon to opt in)" banner when a daemon IS running.
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

// TestTryDaemonSubmit_NoDaemonSocket pins the current "auto-prefer daemon"
// behavior: tryDaemonSubmit returns nil (fall through to direct mode) when no
// daemon socket is reachable. This function is the entry point that Phase 4.1
// will change — after the inversion, tryDaemonSubmit will only be invoked when
// --daemon is explicitly passed or config.Daemon.Default = true.
//
// TODO(T-228 phase 4): After Phase 4.1 inversion, update or remove this test
// and add a replacement that verifies direct mode is taken by default.
func TestTryDaemonSubmit_NoDaemonSocket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cmd := newStartCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	// Pass a non-existent repo path so the submit uses temp dir and the daemon
	// config will not find a socket (standard test environment has no atlas daemon).
	result := tryDaemonSubmit(ctx, cmd, &buf, "test task",
		startOptions{templateName: "bug"}, t.TempDir())

	// When no daemon is running, tryDaemonSubmit must return nil (fall-through).
	if result != nil {
		// A live atlas daemon is running in this environment; skip the characterization
		// so the test does not fail non-deterministically for developers with a daemon up.
		t.Skip("live atlas daemon detected; skipping fall-through characterization")
	}
	assert.Nil(t, result, "tryDaemonSubmit must return nil when daemon is unavailable")
	assert.Empty(t, buf.String(), "no output should be written when falling through")
}
