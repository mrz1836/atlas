package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLintCmd(t *testing.T) {
	cmd := newLintCmd()

	assert.Equal(t, "lint", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Long, "magex lint")
}

func TestAddLintCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddLintCommand(root)

	// Verify lint command was added
	lintCmd, _, err := root.Find([]string{"lint"})
	require.NoError(t, err)
	assert.NotNil(t, lintCmd)
	assert.Equal(t, "lint", lintCmd.Name())
}

func TestRunLint_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newLintCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runLint(ctx, cmd, &buf)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunLint_WithJSONOutput(t *testing.T) {
	// Skip if magex is not available
	t.Skip("Requires magex to be installed")

	cmd := newLintCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json"})
	root.AddCommand(cmd)

	// Set the output flag value
	_ = root.PersistentFlags().Set("output", "json")

	var buf bytes.Buffer
	err := runLint(context.Background(), cmd, &buf)

	// The command may succeed or fail depending on whether magex is installed
	if err == nil {
		var resp ValidationResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
		assert.True(t, resp.Success)
	}
}

func TestLintCommand_HasNoArgs(t *testing.T) {
	cmd := newLintCmd()
	// Lint command should accept no arguments
	assert.Nil(t, cmd.Args)
}

func TestLintCommand_Examples(t *testing.T) {
	cmd := newLintCmd()
	// Verify examples are present
	assert.Contains(t, cmd.Long, "atlas lint")
	assert.Contains(t, cmd.Long, "atlas lint --output json")
}

func TestRunLint_DefaultsWhenConfigLoadFails(t *testing.T) {
	cmd := newLintCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	require.NoError(t, os.Chdir(tmpDir))

	var buf bytes.Buffer
	_ = runLint(context.Background(), cmd, &buf)
}

func TestRunLint_VerboseMode(t *testing.T) {
	cmd := newLintCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Verbose: true})
	root.AddCommand(cmd)

	_ = root.PersistentFlags().Set("verbose", "true")

	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origWd) }()

	require.NoError(t, os.Chdir(tmpDir))

	var buf bytes.Buffer
	_ = runLint(context.Background(), cmd, &buf)
}

func TestNewLintCmd_RunEFunctionExists(t *testing.T) {
	cmd := newLintCmd()
	assert.NotNil(t, cmd.RunE, "lint command should have RunE function")
}

func TestAddLintCommand_AddsToRoot(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	initialCmdCount := len(root.Commands())

	AddLintCommand(root)

	assert.Len(t, root.Commands(), initialCmdCount+1, "should add one command")
}
