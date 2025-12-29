package validation_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/validation"
)

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
