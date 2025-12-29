package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestCmd(t *testing.T) {
	cmd := newTestCmd()

	assert.Equal(t, "test", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Long, "magex test")
}

func TestAddTestCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddTestCommand(root)

	// Verify test command was added
	testCmd, _, err := root.Find([]string{"test"})
	require.NoError(t, err)
	assert.NotNil(t, testCmd)
	assert.Equal(t, "test", testCmd.Name())
}

func TestRunTest_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newTestCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runTest(ctx, cmd, &buf)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunTest_WithJSONOutput(t *testing.T) {
	// Skip if magex is not available
	t.Skip("Requires magex to be installed")

	cmd := newTestCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json"})
	root.AddCommand(cmd)

	// Set the output flag value
	_ = root.PersistentFlags().Set("output", "json")

	var buf bytes.Buffer
	err := runTest(context.Background(), cmd, &buf)

	// The command may succeed or fail depending on whether magex is installed
	if err == nil {
		var resp ValidationResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
		assert.True(t, resp.Success)
	}
}

func TestTestCommand_HasNoArgs(t *testing.T) {
	cmd := newTestCmd()
	// Test command should accept no arguments
	assert.Nil(t, cmd.Args)
}

func TestTestCommand_Examples(t *testing.T) {
	cmd := newTestCmd()
	// Verify examples are present
	assert.Contains(t, cmd.Long, "atlas test")
	assert.Contains(t, cmd.Long, "atlas test --output json")
}
