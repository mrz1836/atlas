package testfakes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TempGitRepo creates a fully initialized git repository in a new temporary
// directory. The repository has one initial commit on branch "master" and is
// cleaned up automatically when the test ends. Returns the absolute repo path.
func TempGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...) // #nosec G204 -- test helper
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "TempGitRepo: git %v: %s", args, out)
	}

	run("init")
	run("config", "user.email", "atlas-test@example.com")
	run("config", "user.name", "Atlas Test")

	readmePath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# atlas test repo\n"), 0o600))

	run("add", ".")
	run("commit", "-m", "init: test repo")

	return dir
}
