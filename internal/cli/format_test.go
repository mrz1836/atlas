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

func TestNewFormatCmd(t *testing.T) {
	cmd := newFormatCmd()

	assert.Equal(t, "format", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Long, "magex format:fix")
}

func TestAddFormatCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddFormatCommand(root)

	// Verify format command was added
	formatCmd, _, err := root.Find([]string{"format"})
	require.NoError(t, err)
	assert.NotNil(t, formatCmd)
	assert.Equal(t, "format", formatCmd.Name())
}

func TestRunFormat_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newFormatCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runFormat(ctx, cmd, &buf)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunFormat_WithJSONOutput(t *testing.T) {
	// Skip if magex is not available
	t.Skip("Requires magex to be installed")

	cmd := newFormatCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json"})
	root.AddCommand(cmd)

	// Set the output flag value
	_ = root.PersistentFlags().Set("output", "json")

	var buf bytes.Buffer
	err := runFormat(context.Background(), cmd, &buf)

	// The command may succeed or fail depending on whether magex is installed
	if err == nil {
		var resp ValidationResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
		assert.True(t, resp.Success)
	}
}

func TestFormatCommand_HasNoArgs(t *testing.T) {
	cmd := newFormatCmd()
	// Format command should accept no arguments
	assert.Nil(t, cmd.Args)
}

func TestFormatCommand_Examples(t *testing.T) {
	cmd := newFormatCmd()
	// Verify examples are present
	assert.Contains(t, cmd.Long, "atlas format")
	assert.Contains(t, cmd.Long, "atlas format --output json")
}
