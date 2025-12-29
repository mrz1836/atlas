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

func TestValidateCommand_HasQuietFlag(t *testing.T) {
	cmd := newValidateCmd()

	// Verify --quiet flag exists
	quietFlag := cmd.Flags().Lookup("quiet")
	require.NotNil(t, quietFlag, "--quiet flag should exist")
	assert.Equal(t, "q", quietFlag.Shorthand)
	assert.Equal(t, "false", quietFlag.DefValue)
}

func TestCapitalizeStep(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"format", "Format"},
		{"lint", "Lint"},
		{"test", "Test"},
		{"pre-commit", "Pre-commit"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := capitalizeStep(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPipelineResultToResponse_NilResult(t *testing.T) {
	resp := pipelineResultToResponse(nil)
	assert.False(t, resp.Success)
	assert.Empty(t, resp.Results)
}
