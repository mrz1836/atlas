package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddCompletionCommand verifies that the completion command is added to the root command.
func TestAddCompletionCommand(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddCompletionCommand(rootCmd)

	// Verify completion command was added
	completionCmd, _, err := rootCmd.Find([]string{"completion"})
	require.NoError(t, err)
	assert.NotNil(t, completionCmd)
	assert.Equal(t, "completion", completionCmd.Use)

	// Verify default completion is disabled
	assert.True(t, rootCmd.CompletionOptions.DisableDefaultCmd)

	// Verify subcommands exist
	subcommands := []string{"bash", "zsh", "fish", "powershell", "install"}
	for _, subcmd := range subcommands {
		t.Run("has_"+subcmd+"_subcommand", func(t *testing.T) {
			cmd, _, err := completionCmd.Find([]string{subcmd})
			require.NoError(t, err)
			assert.NotNil(t, cmd)
			assert.Equal(t, subcmd, cmd.Use)
		})
	}
}

// TestBashCompletionCmd tests the bash completion generation command.
func TestBashCompletionCmd(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddCompletionCommand(rootCmd)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"completion", "bash"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "bash completion")
	assert.Contains(t, output, "atlas")
}

// TestZshCompletionCmd tests the zsh completion generation command.
func TestZshCompletionCmd(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddCompletionCommand(rootCmd)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"completion", "zsh"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "#compdef")
	assert.Contains(t, output, "atlas")
}

// TestFishCompletionCmd tests the fish completion generation command.
func TestFishCompletionCmd(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddCompletionCommand(rootCmd)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"completion", "fish"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "atlas")
	assert.Contains(t, output, "complete")
}

// TestPowershellCompletionCmd tests the powershell completion generation command.
func TestPowershellCompletionCmd(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddCompletionCommand(rootCmd)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"completion", "powershell"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Register-ArgumentCompleter")
	assert.Contains(t, output, "atlas")
}

// TestDetectShell tests shell detection from environment variable.
func TestDetectShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shellPath string
		wantShell shellType
		setEnv    bool
	}{
		{
			name:      "detect zsh",
			shellPath: "/bin/zsh",
			wantShell: shellZsh,
			setEnv:    true,
		},
		{
			name:      "detect zsh from usr bin",
			shellPath: "/usr/bin/zsh",
			wantShell: shellZsh,
			setEnv:    true,
		},
		{
			name:      "detect bash",
			shellPath: "/bin/bash",
			wantShell: shellBash,
			setEnv:    true,
		},
		{
			name:      "detect fish",
			shellPath: "/usr/local/bin/fish",
			wantShell: shellFish,
			setEnv:    true,
		},
		{
			name:      "unknown shell",
			shellPath: "/bin/ksh",
			wantShell: shellUnknown,
			setEnv:    true,
		},
		{
			name:      "no shell environment variable",
			wantShell: shellUnknown,
			setEnv:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original SHELL environment variable
			originalShell := os.Getenv("SHELL")
			defer func() {
				if originalShell != "" {
					_ = os.Setenv("SHELL", originalShell)
				} else {
					_ = os.Unsetenv("SHELL")
				}
			}()

			if tt.setEnv {
				_ = os.Setenv("SHELL", tt.shellPath)
			} else {
				_ = os.Unsetenv("SHELL")
			}

			got := detectShell()
			assert.Equal(t, tt.wantShell, got)
		})
	}
}

// TestGetShellRCFile tests RC file path retrieval.
func TestGetShellRCFile(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		shell    shellType
		expected string
	}{
		{
			name:     "zsh rc file",
			shell:    shellZsh,
			expected: filepath.Join(home, ".zshrc"),
		},
		{
			name:     "bash rc file",
			shell:    shellBash,
			expected: filepath.Join(home, ".bashrc"),
		},
		{
			name:     "fish rc file",
			shell:    shellFish,
			expected: filepath.Join(home, ".config", "fish", "config.fish"),
		},
		{
			name:     "unknown shell",
			shell:    shellUnknown,
			expected: "",
		},
		{
			name:     "invalid shell type",
			shell:    shellType("invalid"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := getShellRCFile(tt.shell)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestRunCompletionInstall_Errors tests error cases in runCompletionInstall.
func TestRunCompletionInstall_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		shellFlag   string
		setShellEnv bool
		shellEnv    string
		expectedErr error
	}{
		{
			name:        "unsupported shell flag",
			shellFlag:   "cmd",
			expectedErr: errUnsupportedShell,
		},
		{
			name:        "no shell detected",
			setShellEnv: false,
			expectedErr: errNoShellDetected,
		},
		{
			name:        "unknown shell in environment",
			setShellEnv: true,
			shellEnv:    "/bin/ksh",
			expectedErr: errNoShellDetected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original SHELL environment variable
			originalShell := os.Getenv("SHELL")
			defer func() {
				if originalShell != "" {
					_ = os.Setenv("SHELL", originalShell)
				} else {
					_ = os.Unsetenv("SHELL")
				}
			}()

			if tt.setShellEnv {
				_ = os.Setenv("SHELL", tt.shellEnv)
			} else {
				_ = os.Unsetenv("SHELL")
			}

			rootCmd := &cobra.Command{Use: "atlas"}
			AddCompletionCommand(rootCmd)

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)

			args := []string{"completion", "install"}
			if tt.shellFlag != "" {
				args = append(args, "--shell", tt.shellFlag)
			}
			rootCmd.SetArgs(args)

			err := rootCmd.Execute()
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

// TestInstallZshCompletions tests zsh completion installation.
func TestInstallZshCompletions(t *testing.T) {
	t.Parallel()

	// Create a temporary home directory for testing
	tmpHome := t.TempDir()

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, rcUpdated, err := installZshCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Verify completion file was created
	assert.FileExists(t, completionPath)
	expectedPath := filepath.Join(tmpHome, ".zsh", "completions", "_atlas")
	assert.Equal(t, expectedPath, completionPath)

	// Verify completion content
	content, err := os.ReadFile(completionPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	assert.Contains(t, string(content), "#compdef atlas")

	// Verify .zshrc was updated
	assert.True(t, rcUpdated)
	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	assert.FileExists(t, zshrcPath)

	zshrcContent, err := os.ReadFile(zshrcPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	zshrcStr := string(zshrcContent)
	assert.Contains(t, zshrcStr, "fpath=")
	assert.Contains(t, zshrcStr, "compinit")
	assert.Contains(t, zshrcStr, "Atlas shell completions")
}

// TestInstallZshCompletions_ExistingRC tests zsh installation with existing .zshrc.
func TestInstallZshCompletions_ExistingRC(t *testing.T) {
	t.Parallel()

	tmpHome := t.TempDir()

	// Create existing .zshrc with compinit
	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	existingContent := "# Existing config\nautoload -U compinit && compinit\n"
	err := os.WriteFile(zshrcPath, []byte(existingContent), 0o600)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, rcUpdated, err := installZshCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Should create completion file
	assert.FileExists(t, completionPath)

	// Should update .zshrc (to add fpath)
	assert.True(t, rcUpdated)

	// Verify existing content is preserved
	newContent, err := os.ReadFile(zshrcPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	assert.Contains(t, string(newContent), "# Existing config")
	assert.Contains(t, string(newContent), "fpath=")
}

// TestInstallZshCompletions_FullyConfigured tests when .zshrc already has everything.
func TestInstallZshCompletions_FullyConfigured(t *testing.T) {
	t.Parallel()

	tmpHome := t.TempDir()

	completionsDir := filepath.Join(tmpHome, ".zsh", "completions")

	// Create existing .zshrc with both fpath and compinit
	zshrcPath := filepath.Join(tmpHome, ".zshrc")
	existingContent := "# Existing config\nfpath=(" + completionsDir + " $fpath)\nautoload -U compinit && compinit\n"
	err := os.WriteFile(zshrcPath, []byte(existingContent), 0o600)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, rcUpdated, err := installZshCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Should create completion file
	assert.FileExists(t, completionPath)

	// Should NOT update .zshrc (already configured)
	assert.False(t, rcUpdated)
}

// TestInstallBashCompletions tests bash completion installation.
func TestInstallBashCompletions(t *testing.T) {
	t.Parallel()

	tmpHome := t.TempDir()

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, rcUpdated, err := installBashCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Verify completion file was created
	assert.FileExists(t, completionPath)
	expectedPath := filepath.Join(tmpHome, ".bash_completion.d", "atlas")
	assert.Equal(t, expectedPath, completionPath)

	// Verify completion content
	content, err := os.ReadFile(completionPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	assert.Contains(t, string(content), "bash completion")

	// Verify .bashrc was updated
	assert.True(t, rcUpdated)
	bashrcPath := filepath.Join(tmpHome, ".bashrc")
	assert.FileExists(t, bashrcPath)

	bashrcContent, err := os.ReadFile(bashrcPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	bashrcStr := string(bashrcContent)
	assert.Contains(t, bashrcStr, ".bash_completion.d")
	assert.Contains(t, bashrcStr, "Atlas shell completions")
}

// TestInstallBashCompletions_ExistingRC tests bash installation with existing .bashrc.
func TestInstallBashCompletions_ExistingRC(t *testing.T) {
	t.Parallel()

	tmpHome := t.TempDir()

	// Create existing .bashrc with bash_completion.d already sourced
	bashrcPath := filepath.Join(tmpHome, ".bashrc")
	existingContent := "# Existing config\nfor f in ~/.bash_completion.d/*; do source \"$f\"; done\n"
	err := os.WriteFile(bashrcPath, []byte(existingContent), 0o600)
	require.NoError(t, err)

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, rcUpdated, err := installBashCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Should create completion file
	assert.FileExists(t, completionPath)

	// Should NOT update .bashrc (already configured)
	assert.False(t, rcUpdated)
}

// TestInstallFishCompletions tests fish completion installation.
func TestInstallFishCompletions(t *testing.T) {
	t.Parallel()

	tmpHome := t.TempDir()

	rootCmd := &cobra.Command{Use: "atlas"}

	completionPath, err := installFishCompletionsToDir(rootCmd, tmpHome)
	require.NoError(t, err)

	// Verify completion file was created
	assert.FileExists(t, completionPath)
	expectedPath := filepath.Join(tmpHome, ".config", "fish", "completions", "atlas.fish")
	assert.Equal(t, expectedPath, completionPath)

	// Verify completion content
	content, err := os.ReadFile(completionPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
	require.NoError(t, err)
	assert.Contains(t, string(content), "atlas")
	assert.Contains(t, string(content), "complete")
}

// TestUpdateZshRC tests .zshrc update logic.
func TestUpdateZshRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		existingContent string
		completionsDir  string
		expectUpdated   bool
		expectFpath     bool
		expectCompinit  bool
	}{
		{
			name:            "new zshrc file",
			existingContent: "",
			completionsDir:  "/home/user/.zsh/completions",
			expectUpdated:   true,
			expectFpath:     true,
			expectCompinit:  true,
		},
		{
			name:            "missing fpath",
			existingContent: "autoload -U compinit && compinit\n",
			completionsDir:  "/home/user/.zsh/completions",
			expectUpdated:   true,
			expectFpath:     true,
			expectCompinit:  false,
		},
		{
			name:            "missing compinit",
			existingContent: "fpath=(/home/user/.zsh/completions $fpath)\n",
			completionsDir:  "/home/user/.zsh/completions",
			expectUpdated:   true,
			expectFpath:     false,
			expectCompinit:  true,
		},
		{
			name:            "fully configured",
			existingContent: "fpath=(/home/user/.zsh/completions $fpath)\nautoload -U compinit && compinit\n",
			completionsDir:  "/home/user/.zsh/completions",
			expectUpdated:   false,
			expectFpath:     false,
			expectCompinit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := t.TempDir()

			// Create .zshrc if needed
			zshrcPath := filepath.Join(tmpHome, ".zshrc")
			if tt.existingContent != "" {
				err := os.WriteFile(zshrcPath, []byte(tt.existingContent), 0o600)
				require.NoError(t, err)
			}

			updated, err := updateZshRC(tmpHome, tt.completionsDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expectUpdated, updated)

			if tt.expectUpdated {
				content, err := os.ReadFile(zshrcPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
				require.NoError(t, err)
				contentStr := string(content)

				if tt.expectFpath {
					assert.Contains(t, contentStr, "fpath=")
				}
				if tt.expectCompinit {
					assert.Contains(t, contentStr, "compinit")
				}
			}
		})
	}
}

// TestUpdateBashRC tests .bashrc update logic.
func TestUpdateBashRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		existingContent string
		completionsDir  string
		expectUpdated   bool
	}{
		{
			name:           "new bashrc file",
			completionsDir: "/home/user/.bash_completion.d",
			expectUpdated:  true,
		},
		{
			name:            "already configured",
			existingContent: "for f in ~/.bash_completion.d/*; do source \"$f\"; done\n",
			completionsDir:  "/home/user/.bash_completion.d",
			expectUpdated:   false,
		},
		{
			name:            "partial match",
			existingContent: "# Some other config\n",
			completionsDir:  "/home/user/.bash_completion.d",
			expectUpdated:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome := t.TempDir()

			// Create .bashrc if needed
			bashrcPath := filepath.Join(tmpHome, ".bashrc")
			if tt.existingContent != "" {
				err := os.WriteFile(bashrcPath, []byte(tt.existingContent), 0o600)
				require.NoError(t, err)
			}

			updated, err := updateBashRC(tmpHome, tt.completionsDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expectUpdated, updated)

			if tt.expectUpdated {
				content, err := os.ReadFile(bashrcPath) // #nosec G304 -- test uses temporary directory created by t.TempDir()
				require.NoError(t, err)
				assert.Contains(t, string(content), ".bash_completion.d")
			}
		})
	}
}

// TestRunCompletionInstall_QuietFlagRecognized tests that the quiet flag is properly recognized.
// Full quiet mode behavior is tested indirectly through other integration tests.
// We can't fully test the quiet flag without modifying the user's home directory since
// os.UserHomeDir() doesn't respect HOME env var override on all platforms.
func TestRunCompletionInstall_QuietFlagRecognized(t *testing.T) {
	t.Parallel()

	rootCmd := &cobra.Command{Use: "atlas"}
	AddGlobalFlags(rootCmd, &GlobalFlags{})
	AddCompletionCommand(rootCmd)

	// Find the install command
	installCmd, _, err := rootCmd.Find([]string{"completion", "install"})
	require.NoError(t, err)
	require.NotNil(t, installCmd)

	// Verify that the quiet flag is available (from persistent flags)
	quietFlag := installCmd.Flags().Lookup("quiet")
	if quietFlag == nil {
		// Try persistent flags
		quietFlag = rootCmd.PersistentFlags().Lookup("quiet")
	}
	assert.NotNil(t, quietFlag, "quiet flag should be available")
}

// TestRunCompletionInstall_WithShellFlag tests explicit shell selection via flag.
// Since os.UserHomeDir() doesn't respect HOME environment variable override on all platforms,
// this test only verifies that the command executes without error and produces appropriate output.
func TestRunCompletionInstall_WithShellFlag(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping test that modifies home directory in CI")
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name              string
		shellFlag         string
		expectedFile      string
		expectedShellType shellType
	}{
		{
			name:              "force zsh",
			shellFlag:         "zsh",
			expectedFile:      filepath.Join(home, ".zsh", "completions", "_atlas"),
			expectedShellType: shellZsh,
		},
		{
			name:              "force bash",
			shellFlag:         "bash",
			expectedFile:      filepath.Join(home, ".bash_completion.d", "atlas"),
			expectedShellType: shellBash,
		},
		{
			name:              "force fish",
			shellFlag:         "fish",
			expectedFile:      filepath.Join(home, ".config", "fish", "completions", "atlas.fish"),
			expectedShellType: shellFish,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up after test
			defer func() {
				_ = os.Remove(tt.expectedFile)
			}()

			rootCmd := &cobra.Command{Use: "atlas"}
			AddGlobalFlags(rootCmd, &GlobalFlags{})
			AddCompletionCommand(rootCmd)

			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetErr(buf)
			rootCmd.SetArgs([]string{"completion", "install", "--shell", tt.shellFlag})

			err := rootCmd.Execute()
			require.NoError(t, err)

			// Verify output mentions the correct shell
			output := buf.String()
			assert.Contains(t, output, string(tt.expectedShellType))

			// Verify file was created
			assert.FileExists(t, tt.expectedFile)
		})
	}
}

// TestShellTypeConstants verifies the shell type constants.
func TestShellTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, shellZsh, shellType("zsh"))
	assert.Equal(t, shellBash, shellType("bash"))
	assert.Equal(t, shellFish, shellType("fish"))
	assert.Equal(t, shellUnknown, shellType("unknown"))
}

// TestCompletionCmdStructure verifies the structure of completion commands.
func TestCompletionCmdStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cmdFunc        func() *cobra.Command
		expectedUse    string
		expectedShort  string
		shouldHaveLong bool
	}{
		{
			name:           "bash command",
			cmdFunc:        newBashCompletionCmd,
			expectedUse:    "bash",
			expectedShort:  "Generate bash completion script",
			shouldHaveLong: true,
		},
		{
			name:           "zsh command",
			cmdFunc:        newZshCompletionCmd,
			expectedUse:    "zsh",
			expectedShort:  "Generate zsh completion script",
			shouldHaveLong: true,
		},
		{
			name:           "fish command",
			cmdFunc:        newFishCompletionCmd,
			expectedUse:    "fish",
			expectedShort:  "Generate fish completion script",
			shouldHaveLong: true,
		},
		{
			name:           "powershell command",
			cmdFunc:        newPowershellCompletionCmd,
			expectedUse:    "powershell",
			expectedShort:  "Generate powershell completion script",
			shouldHaveLong: true,
		},
		{
			name:           "install command",
			cmdFunc:        newInstallCompletionCmd,
			expectedUse:    "install",
			expectedShort:  "Install shell completions automatically",
			shouldHaveLong: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := tt.cmdFunc()
			assert.Equal(t, tt.expectedUse, cmd.Use)
			assert.Equal(t, tt.expectedShort, cmd.Short)
			if tt.shouldHaveLong {
				assert.NotEmpty(t, cmd.Long)
			}
		})
	}
}

// TestErrorMessages verifies error message content.
func TestErrorMessages(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unsupported shell (supported: zsh, bash, fish)", errUnsupportedShell.Error())
	assert.Equal(t, "could not detect shell from $SHELL environment variable; use --shell flag", errNoShellDetected.Error())
}
