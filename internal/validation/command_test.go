package validation_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/validation"
)

// safeBuffer is a thread-safe bytes.Buffer for testing concurrent writes.
type safeBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

var _ io.Writer = (*safeBuffer)(nil)

func TestDefaultCommandRunner_Run_SuccessfulCommand(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	stdout, stderr, exitCode, err := runner.Run(ctx, tmpDir, "echo hello")

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
	assert.Empty(t, stderr)
}

func TestDefaultCommandRunner_Run_FailedCommand(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	stdout, stderr, exitCode, err := runner.Run(ctx, tmpDir, "exit 42")

	require.Error(t, err)
	assert.Equal(t, 42, exitCode)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestDefaultCommandRunner_Run_StderrCapture(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	stdout, stderr, exitCode, err := runner.Run(ctx, tmpDir, "echo error >&2")

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "error\n", stderr)
}

func TestDefaultCommandRunner_Run_WorkingDirectory(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create a unique file in the temp directory
	testFile := "test_workdir_file.txt"
	err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("test"), 0o600)
	require.NoError(t, err)

	// Check that the file exists from within the working directory
	stdout, _, exitCode, err := runner.Run(ctx, tmpDir, "ls "+testFile)

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, testFile)
}

func TestDefaultCommandRunner_Run_ContextCancellation(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tmpDir := t.TempDir()

	_, _, exitCode, err := runner.Run(ctx, tmpDir, "sleep 10")

	require.Error(t, err)
	assert.NotEqual(t, 0, exitCode)
}

func TestDefaultCommandRunner_Run_NonexistentCommand(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	_, _, exitCode, err := runner.Run(ctx, tmpDir, "nonexistent_command_xyz")

	require.Error(t, err)
	assert.NotEqual(t, 0, exitCode)
}

func TestDefaultCommandRunner_Run_MultipleCommands(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	stdout, _, exitCode, err := runner.Run(ctx, tmpDir, "echo first && echo second")

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "first")
	assert.Contains(t, stdout, "second")
}

func TestDefaultCommandRunner_Run_BothStdoutAndStderr(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	stdout, stderr, exitCode, err := runner.Run(ctx, tmpDir, "echo out && echo err >&2")

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "out\n", stdout)
	assert.Equal(t, "err\n", stderr)
}

func TestDefaultCommandRunner_RunWithLiveOutput(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()
	tmpDir := t.TempDir()

	liveOutput := &safeBuffer{}

	stdout, stderr, exitCode, err := runner.RunWithLiveOutput(ctx, tmpDir, "echo live_stdout && echo live_stderr >&2", liveOutput)

	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Both captured and live output should contain the output
	assert.Equal(t, "live_stdout\n", stdout)
	assert.Equal(t, "live_stderr\n", stderr)

	// Live output should contain both stdout and stderr
	liveStr := liveOutput.String()
	assert.Contains(t, liveStr, "live_stdout")
	assert.Contains(t, liveStr, "live_stderr")
}

func TestDefaultCommandRunner_RunWithLiveOutput_FailedCommand(t *testing.T) {
	runner := &validation.DefaultCommandRunner{}
	ctx := context.Background()
	tmpDir := t.TempDir()

	liveOutput := &safeBuffer{}

	_, _, exitCode, err := runner.RunWithLiveOutput(ctx, tmpDir, "exit 42", liveOutput)

	require.Error(t, err)
	assert.Equal(t, 42, exitCode)
}
