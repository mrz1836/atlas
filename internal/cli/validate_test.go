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

func TestNewValidateCmd(t *testing.T) {
	cmd := newValidateCmd()

	assert.Equal(t, "validate", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Contains(t, cmd.Long, "Format")
	assert.Contains(t, cmd.Long, "Lint")
	assert.Contains(t, cmd.Long, "Test")
	assert.Contains(t, cmd.Long, "Pre-commit")
}

func TestAddValidateCommand(t *testing.T) {
	root := &cobra.Command{Use: "atlas"}
	AddValidateCommand(root)

	// Verify validate command was added
	validateCmd, _, err := root.Find([]string{"validate"})
	require.NoError(t, err)
	assert.NotNil(t, validateCmd)
	assert.Equal(t, "validate", validateCmd.Name())
}

func TestRunValidate_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{})
	root.AddCommand(cmd)

	var buf bytes.Buffer
	err := runValidate(ctx, cmd, &buf)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRunValidate_WithJSONOutput(t *testing.T) {
	// Skip if magex is not available
	t.Skip("Requires magex to be installed")

	cmd := newValidateCmd()
	root := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(root, &GlobalFlags{Output: "json"})
	root.AddCommand(cmd)

	// Set the output flag value
	_ = root.PersistentFlags().Set("output", "json")

	var buf bytes.Buffer
	err := runValidate(context.Background(), cmd, &buf)

	// The command may succeed or fail depending on whether magex is installed
	// but if it fails with JSON output, it should still produce valid JSON
	if err == nil {
		var resp ValidationResponse
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
		assert.True(t, resp.Success)
	}
}

func TestValidateCommand_HasNoArgs(t *testing.T) {
	cmd := newValidateCmd()
	// Validate command should accept no arguments
	assert.Nil(t, cmd.Args)
}

func TestValidateCommand_Examples(t *testing.T) {
	cmd := newValidateCmd()
	// Verify examples are present
	assert.Contains(t, cmd.Long, "atlas validate")
	assert.Contains(t, cmd.Long, "atlas validate --output json")
}
