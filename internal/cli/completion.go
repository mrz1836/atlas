// Package cli provides the command-line interface for atlas.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// shellType represents supported shell types.
type shellType string

// Sentinel errors for completion commands.
var (
	errUnsupportedShell = errors.New("unsupported shell (supported: zsh, bash, fish)")
	errNoShellDetected  = errors.New("could not detect shell from $SHELL environment variable; use --shell flag")
)

const (
	shellZsh     shellType = "zsh"
	shellBash    shellType = "bash"
	shellFish    shellType = "fish"
	shellUnknown shellType = "unknown"
)

// AddCompletionCommand adds the completion command with subcommands to the root command.
// This replaces Cobra's default completion command with a custom one that includes
// an "install" subcommand for easy setup.
func AddCompletionCommand(rootCmd *cobra.Command) {
	// Disable Cobra's default completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completions",
		Long: `Generate shell completion scripts for atlas.

To install completions automatically:
  atlas completion install

To generate completion scripts manually:
  atlas completion bash
  atlas completion zsh
  atlas completion fish
  atlas completion powershell`,
	}

	// Add shell-specific generation subcommands
	completionCmd.AddCommand(newBashCompletionCmd())
	completionCmd.AddCommand(newZshCompletionCmd())
	completionCmd.AddCommand(newFishCompletionCmd())
	completionCmd.AddCommand(newPowershellCompletionCmd())
	completionCmd.AddCommand(newInstallCompletionCmd())

	rootCmd.AddCommand(completionCmd)
}

func newBashCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Long: `Generate bash completion script for atlas.

To load completions in current session:
  source <(atlas completion bash)

To install completions permanently:
  atlas completion install --shell bash`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenBashCompletion(cmd.OutOrStdout())
		},
	}
}

func newZshCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Long: `Generate zsh completion script for atlas.

To load completions in current session:
  source <(atlas completion zsh)

To install completions permanently:
  atlas completion install --shell zsh`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		},
	}
}

func newFishCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Long: `Generate fish completion script for atlas.

To load completions in current session:
  atlas completion fish | source

To install completions permanently:
  atlas completion install --shell fish`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}
}

func newPowershellCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "powershell",
		Short: "Generate powershell completion script",
		Long: `Generate powershell completion script for atlas.

To load completions in current session:
  atlas completion powershell | Out-String | Invoke-Expression`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		},
	}
}

func newInstallCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell completions automatically",
		Long: `Install shell completions for atlas.

This command auto-detects your shell and installs completions to the appropriate location.
You can override the detected shell with the --shell flag.

Supported shells: zsh, bash, fish

Examples:
  atlas completion install              # Auto-detect shell
  atlas completion install --shell zsh  # Force zsh`,
		RunE: runCompletionInstall,
	}

	cmd.Flags().String("shell", "", "Shell to install completions for (zsh, bash, fish)")
	return cmd
}

// runCompletionInstall handles the completion install subcommand.
func runCompletionInstall(cmd *cobra.Command, _ []string) error {
	shellFlag, _ := cmd.Flags().GetString("shell")
	quiet, _ := cmd.Flags().GetBool("quiet")

	// Detect or validate shell
	var shell shellType
	if shellFlag != "" {
		shell = shellType(shellFlag)
		if shell != shellZsh && shell != shellBash && shell != shellFish {
			return fmt.Errorf("%s: %w", shellFlag, errUnsupportedShell)
		}
	} else {
		shell = detectShell()
		if shell == shellUnknown {
			return errNoShellDetected
		}
	}

	if !quiet {
		cmd.Printf("Detected shell: %s\n\n", shell)
		cmd.Println("Installing completions...")
	}

	// Get root command to generate completions
	rootCmd := cmd.Root()

	var err error
	var completionPath string
	var rcUpdated bool

	switch shell {
	case shellZsh:
		completionPath, rcUpdated, err = installZshCompletions(rootCmd, quiet)
	case shellBash:
		completionPath, rcUpdated, err = installBashCompletions(rootCmd, quiet)
	case shellFish:
		completionPath, err = installFishCompletions(rootCmd, quiet)
	case shellUnknown:
		// Already handled above with errNoShellDetected
		return errNoShellDetected
	}

	if err != nil {
		return err
	}

	if !quiet {
		cmd.Printf("  Created %s\n", completionPath)
		if rcUpdated {
			rcFile := getShellRCFile(shell)
			cmd.Printf("  Updated %s\n", rcFile)
		}
		cmd.Println()
		cmd.Printf("Done! Restart your shell or run: source %s\n", getShellRCFile(shell))
	}

	return nil
}

// detectShell detects the user's shell from the $SHELL environment variable.
func detectShell() shellType {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		return shellUnknown
	}

	shellName := filepath.Base(shellPath)
	switch shellName {
	case "zsh":
		return shellZsh
	case "bash":
		return shellBash
	case "fish":
		return shellFish
	default:
		return shellUnknown
	}
}

// getShellRCFile returns the path to the shell's RC file.
func getShellRCFile(shell shellType) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case shellZsh:
		return filepath.Join(home, ".zshrc")
	case shellBash:
		return filepath.Join(home, ".bashrc")
	case shellFish:
		return filepath.Join(home, ".config", "fish", "config.fish")
	case shellUnknown:
		return ""
	}
	return ""
}

// installZshCompletions installs zsh completions to ~/.zsh/completions/_atlas.
func installZshCompletions(rootCmd *cobra.Command, _ bool) (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("could not determine home directory: %w", err)
	}
	return installZshCompletionsToDir(rootCmd, home)
}

// installZshCompletionsToDir installs zsh completions to a specific home directory.
// This function is extracted for testability.
func installZshCompletionsToDir(rootCmd *cobra.Command, home string) (string, bool, error) {
	// Create completions directory
	completionsDir := filepath.Join(home, ".zsh", "completions")
	if err := os.MkdirAll(completionsDir, 0o750); err != nil {
		return "", false, fmt.Errorf("could not create %s: %w", completionsDir, err)
	}

	// Generate and write completion script
	completionPath := filepath.Join(completionsDir, "_atlas")
	var buf bytes.Buffer
	if err := rootCmd.GenZshCompletion(&buf); err != nil {
		return "", false, fmt.Errorf("could not generate zsh completions: %w", err)
	}

	if err := os.WriteFile(completionPath, buf.Bytes(), 0o600); err != nil {
		return "", false, fmt.Errorf("could not write %s: %w", completionPath, err)
	}

	// Update .zshrc if needed
	rcUpdated, err := updateZshRC(home, completionsDir)
	if err != nil {
		return completionPath, false, fmt.Errorf("could not update .zshrc: %w", err)
	}

	return completionPath, rcUpdated, nil
}

// updateZshRC ensures fpath and compinit are configured in .zshrc.
func updateZshRC(home, completionsDir string) (bool, error) {
	rcPath := filepath.Clean(filepath.Join(home, ".zshrc"))

	// Read existing content
	content, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	contentStr := string(content)
	var additions []string

	// Check for fpath configuration
	fpathLine := fmt.Sprintf("fpath=(%s $fpath)", completionsDir)
	if !strings.Contains(contentStr, completionsDir) {
		additions = append(additions, fpathLine)
	}

	// Check for compinit
	if !strings.Contains(contentStr, "compinit") {
		additions = append(additions, "autoload -U compinit && compinit")
	}

	if len(additions) == 0 {
		return false, nil
	}

	// Append to .zshrc
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	// Add a newline and comment before our additions
	toWrite := "\n# Atlas shell completions\n" + strings.Join(additions, "\n") + "\n"
	if _, err = f.WriteString(toWrite); err != nil {
		return false, err
	}

	return true, nil
}

// installBashCompletions installs bash completions to ~/.bash_completion.d/atlas.
func installBashCompletions(rootCmd *cobra.Command, _ bool) (string, bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("could not determine home directory: %w", err)
	}
	return installBashCompletionsToDir(rootCmd, home)
}

// installBashCompletionsToDir installs bash completions to a specific home directory.
// This function is extracted for testability.
func installBashCompletionsToDir(rootCmd *cobra.Command, home string) (string, bool, error) {
	// Create completions directory
	completionsDir := filepath.Join(home, ".bash_completion.d")
	if err := os.MkdirAll(completionsDir, 0o750); err != nil {
		return "", false, fmt.Errorf("could not create %s: %w", completionsDir, err)
	}

	// Generate and write completion script
	completionPath := filepath.Join(completionsDir, "atlas")
	var buf bytes.Buffer
	if err := rootCmd.GenBashCompletion(&buf); err != nil {
		return "", false, fmt.Errorf("could not generate bash completions: %w", err)
	}

	if err := os.WriteFile(completionPath, buf.Bytes(), 0o600); err != nil {
		return "", false, fmt.Errorf("could not write %s: %w", completionPath, err)
	}

	// Update .bashrc if needed
	rcUpdated, err := updateBashRC(home, completionsDir)
	if err != nil {
		return completionPath, false, fmt.Errorf("could not update .bashrc: %w", err)
	}

	return completionPath, rcUpdated, nil
}

// updateBashRC ensures completion sourcing is configured in .bashrc.
func updateBashRC(home, completionsDir string) (bool, error) {
	rcPath := filepath.Clean(filepath.Join(home, ".bashrc"))

	// Read existing content
	content, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	contentStr := string(content)

	// Check if our completion directory is already sourced
	if strings.Contains(contentStr, ".bash_completion.d") {
		return false, nil
	}

	// Append sourcing loop to .bashrc
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	sourceLine := fmt.Sprintf(`
# Atlas shell completions
for f in %s/*; do
  [ -f "$f" ] && source "$f"
done
`, completionsDir)

	if _, err = f.WriteString(sourceLine); err != nil {
		return false, err
	}

	return true, nil
}

// installFishCompletions installs fish completions to ~/.config/fish/completions/atlas.fish.
func installFishCompletions(rootCmd *cobra.Command, _ bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return installFishCompletionsToDir(rootCmd, home)
}

// installFishCompletionsToDir installs fish completions to a specific home directory.
// This function is extracted for testability.
func installFishCompletionsToDir(rootCmd *cobra.Command, home string) (string, error) {
	// Create completions directory
	completionsDir := filepath.Join(home, ".config", "fish", "completions")
	if err := os.MkdirAll(completionsDir, 0o750); err != nil {
		return "", fmt.Errorf("could not create %s: %w", completionsDir, err)
	}

	// Generate and write completion script
	completionPath := filepath.Join(completionsDir, "atlas.fish")
	var buf bytes.Buffer
	if err := rootCmd.GenFishCompletion(&buf, true); err != nil {
		return "", fmt.Errorf("could not generate fish completions: %w", err)
	}

	if err := os.WriteFile(completionPath, buf.Bytes(), 0o600); err != nil {
		return "", fmt.Errorf("could not write %s: %w", completionPath, err)
	}

	// Fish auto-loads from this directory, no RC update needed
	return completionPath, nil
}
