// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"testing"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationProviderConfig_Defaults(t *testing.T) {
	defaults := ValidationConfigDefaults()

	assert.Empty(t, defaults.FormatCmds)
	assert.Empty(t, defaults.LintCmds)
	assert.Empty(t, defaults.TestCmds)
	assert.Empty(t, defaults.PreCommitCmds)
	assert.Empty(t, defaults.CustomPrePR)
}

func TestSuggestValidationDefaults_WithMageX(t *testing.T) {
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusInstalled},
		},
	}

	defaults := SuggestValidationDefaults(result)

	assert.Equal(t, []string{"magex format:fix"}, defaults.Format)
	assert.Equal(t, []string{"magex lint"}, defaults.Lint)
	assert.Equal(t, []string{"magex test"}, defaults.Test)
	assert.Empty(t, defaults.PreCommit)
}

func TestSuggestValidationDefaults_WithGoPreCommit(t *testing.T) {
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolGoPreCommit, Status: config.ToolStatusInstalled},
		},
	}

	defaults := SuggestValidationDefaults(result)

	// Should fall back to basic go commands for format/lint/test
	assert.Equal(t, []string{"gofmt -w ."}, defaults.Format)
	assert.Equal(t, []string{"go vet ./..."}, defaults.Lint)
	assert.Equal(t, []string{"go test ./..."}, defaults.Test)
	// But should have go-pre-commit
	assert.Equal(t, []string{"go-pre-commit run --all-files"}, defaults.PreCommit)
}

func TestSuggestValidationDefaults_WithBothTools(t *testing.T) {
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusInstalled},
			{Name: constants.ToolGoPreCommit, Status: config.ToolStatusInstalled},
		},
	}

	defaults := SuggestValidationDefaults(result)

	assert.Equal(t, []string{"magex format:fix"}, defaults.Format)
	assert.Equal(t, []string{"magex lint"}, defaults.Lint)
	assert.Equal(t, []string{"magex test"}, defaults.Test)
	assert.Equal(t, []string{"go-pre-commit run --all-files"}, defaults.PreCommit)
}

func TestSuggestValidationDefaults_NoTools(t *testing.T) {
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{},
	}

	defaults := SuggestValidationDefaults(result)

	// Should fall back to basic go commands
	assert.Equal(t, []string{"gofmt -w ."}, defaults.Format)
	assert.Equal(t, []string{"go vet ./..."}, defaults.Lint)
	assert.Equal(t, []string{"go test ./..."}, defaults.Test)
	assert.Empty(t, defaults.PreCommit)
}

func TestSuggestValidationDefaults_MissingToolsIgnored(t *testing.T) {
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusMissing},
			{Name: constants.ToolGoPreCommit, Status: config.ToolStatusMissing},
		},
	}

	defaults := SuggestValidationDefaults(result)

	// Missing tools should not be suggested
	assert.Equal(t, []string{"gofmt -w ."}, defaults.Format)
	assert.Equal(t, []string{"go vet ./..."}, defaults.Lint)
	assert.Equal(t, []string{"go test ./..."}, defaults.Test)
	assert.Empty(t, defaults.PreCommit)
}

func TestParseMultilineInput_Extended(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "multiple lines",
			input: "cmd1\ncmd2\ncmd3",
			want:  []string{"cmd1", "cmd2", "cmd3"},
		},
		{
			name:  "empty lines filtered",
			input: "cmd1\n\ncmd2",
			want:  []string{"cmd1", "cmd2"},
		},
		{
			name:  "whitespace trimmed",
			input: "  cmd1  \n  cmd2  ",
			want:  []string{"cmd1", "cmd2"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "only whitespace",
			input: "   \n   \n   ",
			want:  nil,
		},
		{
			name:  "single command",
			input: "magex lint",
			want:  []string{"magex lint"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMultilineInput(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectValidationConfigInteractive_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := &ValidationProviderConfig{}
	err := CollectValidationConfigInteractive(ctx, cfg, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewValidationConfigForm(t *testing.T) {
	cfg := &ValidationProviderConfig{
		FormatCmds:    "magex format:fix",
		LintCmds:      "magex lint",
		TestCmds:      "magex test",
		PreCommitCmds: "go-pre-commit run --all-files",
		CustomPrePR:   "custom-hook",
	}

	form := NewValidationConfigForm(cfg)
	require.NotNil(t, form)
}

func TestPopulateValidationConfigDefaults(t *testing.T) {
	cfg := &ValidationProviderConfig{}
	result := &config.ToolDetectionResult{
		Tools: []config.Tool{
			{Name: constants.ToolMageX, Status: config.ToolStatusInstalled},
		},
	}

	PopulateValidationConfigDefaults(cfg, result)

	assert.Equal(t, "magex format:fix", cfg.FormatCmds)
	assert.Equal(t, "magex lint", cfg.LintCmds)
	assert.Equal(t, "magex test", cfg.TestCmds)
	assert.Empty(t, cfg.PreCommitCmds)
}

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		wantOK bool
	}{
		{
			name:   "go exists",
			cmd:    "go version",
			wantOK: true,
		},
		{
			name:   "git exists",
			cmd:    "git status",
			wantOK: true,
		},
		{
			name:   "nonexistent command",
			cmd:    "nonexistent-cmd-xyz --help",
			wantOK: false,
		},
		{
			name:   "empty string",
			cmd:    "",
			wantOK: false,
		},
		{
			name:   "whitespace only",
			cmd:    "   ",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, warning := ValidateCommand(tt.cmd)
			assert.Equal(t, tt.wantOK, exists)
			if !tt.wantOK {
				assert.NotEmpty(t, warning)
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}

func TestValidateCommands(t *testing.T) {
	// Valid commands - should have no warnings
	warnings := ValidateCommands("go version\ngit --version")
	assert.Empty(t, warnings)

	// Invalid command - should have warning
	warnings = ValidateCommands("nonexistent-xyz")
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "nonexistent-xyz")

	// Mixed valid and invalid
	warnings = ValidateCommands("go version\nnonexistent-xyz\ngit status")
	assert.Len(t, warnings, 1)
}

func TestValidateAllConfigCommands(t *testing.T) {
	cfg := &ValidationProviderConfig{
		FormatCmds:    "go fmt ./...",
		LintCmds:      "go vet ./...",
		TestCmds:      "nonexistent-test-cmd",
		PreCommitCmds: "nonexistent-precommit",
		CustomPrePR:   "",
	}

	result := ValidateAllConfigCommands(cfg)

	// Format and Lint should have no warnings (go exists)
	assert.Empty(t, result["Format"])
	assert.Empty(t, result["Lint"])

	// Test and Pre-commit should have warnings
	assert.Len(t, result["Test"], 1)
	assert.Len(t, result["Pre-commit"], 1)

	// Custom Pre-PR is empty, no warnings
	assert.Empty(t, result["Custom Pre-PR"])
}

func TestToValidationCommands(t *testing.T) {
	cfg := &ValidationProviderConfig{
		FormatCmds:    "cmd1\ncmd2",
		LintCmds:      "lint1",
		TestCmds:      "test1\ntest2\ntest3",
		PreCommitCmds: "",
	}

	cmds := cfg.ToValidationCommands()

	assert.Equal(t, []string{"cmd1", "cmd2"}, cmds.Format)
	assert.Equal(t, []string{"lint1"}, cmds.Lint)
	assert.Equal(t, []string{"test1", "test2", "test3"}, cmds.Test)
	assert.Nil(t, cmds.PreCommit)
}

func TestToValidationConfig(t *testing.T) {
	cfg := &ValidationProviderConfig{
		FormatCmds: "magex format:fix",
		LintCmds:   "magex lint",
		TestCmds:   "magex test",
	}

	valCfg := cfg.ToValidationConfig()

	assert.Equal(t, []string{"magex format:fix"}, valCfg.Commands.Format)
	assert.Equal(t, []string{"magex lint"}, valCfg.Commands.Lint)
	assert.Equal(t, []string{"magex test"}, valCfg.Commands.Test)
}

func TestToValidationCommands_IncludesCustomPrePR(t *testing.T) {
	cfg := &ValidationProviderConfig{
		FormatCmds:  "gofmt -w .",
		LintCmds:    "go vet ./...",
		TestCmds:    "go test ./...",
		CustomPrePR: "custom-script\nanother-script",
	}

	cmds := cfg.ToValidationCommands()

	assert.Equal(t, []string{"gofmt -w ."}, cmds.Format)
	assert.Equal(t, []string{"custom-script", "another-script"}, cmds.CustomPrePR)
}
