// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/crypto/native"
	"github.com/mrz1836/atlas/internal/domain"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/git"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/tui"
	"github.com/mrz1836/atlas/internal/workspace"
	"github.com/spf13/cobra"
)

// AddHookCommand adds the hook command group to the root command.
func AddHookCommand(root *cobra.Command) {
	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Manage task recovery hooks",
		Long: `Commands for viewing and managing task recovery hooks.

Hooks provide crash recovery context that allows tasks to resume after
interruptions without losing progress or repeating completed steps.

Examples:
  atlas hook status                    # View current hook state
  atlas hook checkpoints               # List all checkpoints
  atlas hook verify-receipt rcpt-001   # Verify a receipt signature
  atlas hook regenerate                # Regenerate HOOK.md from hook.json
  atlas hook export                    # Export hook state as JSON`,
	}

	hookCmd.AddCommand(newHookStatusCmd())
	hookCmd.AddCommand(newHookCheckpointsCmd())
	hookCmd.AddCommand(newHookInstallCmd())
	hookCmd.AddCommand(newHookVerifyReceiptCmd())
	hookCmd.AddCommand(newHookRegenerateCmd())
	hookCmd.AddCommand(newHookExportCmd())

	root.AddCommand(hookCmd)
}

// newHookStatusCmd creates the hook status command.
func newHookStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Display the current hook state",
		Long: `Display the current hook state for the active workspace.

Shows state, step progress, checkpoints, and validation receipts.

Exit codes:
  0: Success
  1: No active hook found
  2: Hook in error state`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHookStatus(cmd.Context(), cmd, os.Stdout)
		},
	}
}

// newHookCheckpointsCmd creates the hook checkpoints command.
func newHookCheckpointsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "checkpoints",
		Short: "List all checkpoints for the current task",
		Long: `List all checkpoints for the current task.

Checkpoints are created automatically on git commits, validation passes,
step completions, and periodically during long-running steps.

Exit codes:
  0: Success (may have 0 checkpoints)
  1: No active hook found`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHookCheckpoints(cmd.Context(), cmd, os.Stdout)
		},
	}
}

// newHookVerifyReceiptCmd creates the hook verify-receipt command.
func newHookVerifyReceiptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify-receipt <receipt-id>",
		Short: "Verify a validation receipt signature",
		Long: `Verify the cryptographic signature of a validation receipt.

Validation receipts are signed proofs that validation actually ran.
This command verifies the signature using the master key.

Exit codes:
  0: Signature valid
  1: Receipt not found
  2: Signature invalid
  3: Key manager error (missing master key)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookVerifyReceipt(cmd.Context(), cmd, os.Stdout, args[0])
		},
	}
}

// newHookRegenerateCmd creates the hook regenerate command.
func newHookRegenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "regenerate",
		Short: "Regenerate HOOK.md from hook.json",
		Long: `Regenerate the HOOK.md recovery file from hook.json.

Use this if HOOK.md is corrupted or was manually edited incorrectly.
The source of truth is always hook.json.

Exit codes:
  0: Success
  1: No active hook found
  2: Failed to regenerate`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHookRegenerate(cmd.Context(), cmd, os.Stdout)
		},
	}
}

// newHookExportCmd creates the hook export command.
func newHookExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export hook history for debugging",
		Long: `Export the full hook.json content to stdout.

Useful for debugging or preserving hook state for analysis.

Exit codes:
  0: Success
  1: No active hook found`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHookExport(cmd.Context(), cmd, os.Stdout)
		},
	}
}

// newHookInstallCmd creates the hook install command.
func newHookInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Show instructions for installing git hooks",
		Long: `Print the git hook wrapper script and installation instructions.

This command does NOT modify your .git directory. It outputs a script that you
can manually add to your .git/hooks/post-commit (or post-push) file to enable
automatic checkpoints.

Example:
  atlas hook install > .git/hooks/post-commit
  chmod +x .git/hooks/post-commit

Exit codes:
  0: Success
  1: No active hook found`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHookInstall(cmd.Context(), cmd, os.Stdout)
		},
	}
}

// runHookStatus executes the hook status command.
func runHookStatus(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "status", err.Error())
		}
		return err
	}

	if outputFormat == OutputJSON {
		return out.JSON(h)
	}

	displayHookStatus(out, h)
	return nil
}

// runHookCheckpoints executes the hook checkpoints command.
func runHookCheckpoints(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "checkpoints", err.Error())
		}
		return err
	}

	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"checkpoints": h.Checkpoints,
			"count":       len(h.Checkpoints),
		})
	}

	displayHookCheckpoints(out, h)
	return nil
}

// runHookVerifyReceipt executes the hook verify-receipt command.
func runHookVerifyReceipt(ctx context.Context, cmd *cobra.Command, w io.Writer, receiptID string) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "verify-receipt", err.Error())
		}
		return err
	}

	// Find the receipt
	var receipt *domain.ValidationReceipt
	for i := range h.Receipts {
		if h.Receipts[i].ReceiptID == receiptID {
			receipt = &h.Receipts[i]
			break
		}
	}

	if receipt == nil {
		notFoundErr := fmt.Errorf("%w: %s", atlaserrors.ErrReceiptNotFound, receiptID)
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "verify-receipt", notFoundErr.Error())
		}
		return notFoundErr
	}

	// Get atlas home directory for key file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	keyDir := filepath.Join(homeDir, ".atlas", "keys")

	// Load key manager
	keyMgr := native.NewKeyManager(keyDir)
	if loadErr := keyMgr.Load(ctx); loadErr != nil {
		return fmt.Errorf("failed to load key manager: %w", loadErr)
	}

	// Create receipt signer and verify
	signer, signerErr := hook.NewNativeReceiptSigner(keyMgr)
	if signerErr != nil {
		return fmt.Errorf("failed to create signer: %w", signerErr)
	}
	verifyErr := signer.VerifyReceipt(ctx, receipt)

	result := map[string]any{
		"receipt_id": receipt.ReceiptID,
		"step_name":  receipt.StepName,
		"command":    receipt.Command,
		"exit_code":  receipt.ExitCode,
		"duration":   receipt.Duration,
		"valid":      verifyErr == nil,
	}

	if verifyErr != nil {
		result["error"] = verifyErr.Error()
	}

	if outputFormat == OutputJSON {
		return out.JSON(result)
	}

	displayReceiptVerification(out, receipt, verifyErr)
	return nil
}

// runHookRegenerate executes the hook regenerate command.
func runHookRegenerate(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "regenerate", err.Error())
		}
		return err
	}

	// Generate HOOK.md content
	generator := hook.NewMarkdownGenerator()
	content, err := generator.Generate(h)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "regenerate", fmt.Sprintf("failed to generate: %v", err))
		}
		return fmt.Errorf("failed to generate HOOK.md: %w", err)
	}

	// Get the hook file path
	hookPath, err := getActiveHookPath(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "regenerate", err.Error())
		}
		return err
	}

	// Write HOOK.md alongside hook.json
	mdPath := strings.TrimSuffix(hookPath, "hook.json") + "HOOK.md"
	if err := os.WriteFile(mdPath, content, 0o600); err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "regenerate", fmt.Sprintf("failed to write: %v", err))
		}
		return fmt.Errorf("failed to write HOOK.md: %w", err)
	}

	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"success": true,
			"path":    mdPath,
		})
	}

	out.Success(fmt.Sprintf("Regenerated HOOK.md at %s", mdPath))
	return nil
}

// runHookExport executes the hook export command.
func runHookExport(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	outputFormat := cmd.Flag("output").Value.String()

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "export", err.Error())
		}
		return err
	}

	// Always export as JSON (indented)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(h)
}

// runHookInstall executes the hook install command.
func runHookInstall(ctx context.Context, cmd *cobra.Command, w io.Writer) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	h, err := getActiveHook(ctx)
	if err != nil {
		if outputFormat == OutputJSON {
			return outputHookErrorJSON(w, "install", err.Error())
		}
		return err
	}

	// Generate the script content
	script := git.GenerateHookScript(git.HookPostCommit, h.TaskID, h.WorkspaceID)

	if outputFormat == OutputJSON {
		return out.JSON(map[string]string{
			"script":       script,
			"instructions": "Copy the script to .git/hooks/post-commit and make it executable.",
		})
	}

	// Print script and instructions
	out.Info("# ------------------------------------------------------------------")
	out.Info("# Add the following to your .git/hooks/post-commit file:")
	out.Info("# ------------------------------------------------------------------")
	out.Info("")
	if _, err := fmt.Fprint(w, script); err != nil {
		return fmt.Errorf("failed to print script: %w", err)
	}
	out.Info("")
	out.Info("# ------------------------------------------------------------------")
	out.Info("# Then run: chmod +x .git/hooks/post-commit")
	out.Info("# ------------------------------------------------------------------")

	return nil
}

// getActiveHook finds and returns the active hook for the current workspace.
func getActiveHook(ctx context.Context) (*domain.Hook, error) {
	hookPath, err := getActiveHookPath(ctx)
	if err != nil {
		return nil, err
	}

	// Read and parse the hook file
	data, err := os.ReadFile(hookPath) //nolint:gosec // hookPath is constructed from validated workspace paths
	if err != nil {
		return nil, fmt.Errorf("failed to read hook file: %w", err)
	}

	var h domain.Hook
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("%w: %w", hook.ErrInvalidHook, err)
	}

	return &h, nil
}

// getActiveHookPath finds the path to the active hook.json file.
func getActiveHookPath(ctx context.Context) (string, error) {
	// Get base path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	baseDir := filepath.Join(homeDir, constants.AtlasHome)

	// Get workspace store
	wsStore, err := workspace.NewFileStore("")
	if err != nil {
		return "", fmt.Errorf("failed to create workspace store: %w", err)
	}

	// Find active workspaces
	workspaces, err := wsStore.List(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Look for hook.json in active workspaces
	for _, ws := range workspaces {
		if ws.Status == "closed" {
			continue
		}

		// Check for hook.json in this workspace's tasks
		tasksDir := filepath.Join(baseDir, "workspaces", ws.Name, "tasks")
		entries, err := os.ReadDir(tasksDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			hookPath := filepath.Join(tasksDir, entry.Name(), "hook.json")
			if _, statErr := os.Stat(hookPath); statErr == nil {
				return hookPath, nil
			}
		}
	}

	return "", fmt.Errorf("%w: no active hook found", atlaserrors.ErrHookNotFound)
}

// displayHookStatus displays the hook status in text format.
func displayHookStatus(out tui.Output, h *domain.Hook) {
	out.Info(fmt.Sprintf("Hook State: %s", h.State))
	out.Info(fmt.Sprintf("Task: %s (%s)", h.TaskID, h.WorkspaceID))

	if h.CurrentStep != nil {
		out.Info(fmt.Sprintf("Step: %s (%d), Attempt %d/%d",
			h.CurrentStep.StepName,
			h.CurrentStep.StepIndex+1,
			h.CurrentStep.Attempt,
			h.CurrentStep.MaxAttempts))
	}

	out.Info(fmt.Sprintf("Last Updated: %s", formatRelativeTime(h.UpdatedAt)))

	if len(h.Checkpoints) > 0 {
		latest := h.Checkpoints[len(h.Checkpoints)-1]
		out.Info(fmt.Sprintf("Last Checkpoint: %s (%s, %s)",
			latest.CheckpointID,
			latest.Trigger,
			formatRelativeTime(latest.CreatedAt)))
	}

	validCount := 0
	for _, r := range h.Receipts {
		if r.Signature != "" {
			validCount++
		}
	}
	out.Info(fmt.Sprintf("Receipts: %d (all valid)", validCount))
}

// displayHookCheckpoints displays checkpoints in text format.
func displayHookCheckpoints(out tui.Output, h *domain.Hook) {
	if len(h.Checkpoints) == 0 {
		out.Info("No checkpoints recorded.")
		return
	}

	out.Info(fmt.Sprintf("Checkpoints for %s:", h.TaskID))
	out.Info("")
	out.Info("| Time       | Trigger       | Description                      |")
	out.Info("|------------|---------------|----------------------------------|")

	for _, cp := range h.Checkpoints {
		desc := cp.Description
		if len(desc) > 32 {
			desc = desc[:29] + "..."
		}
		out.Info(fmt.Sprintf("| %-10s | %-13s | %-32s |",
			cp.CreatedAt.Format("15:04:05"),
			cp.Trigger,
			desc))
	}
}

// displayReceiptVerification displays receipt verification result.
func displayReceiptVerification(out tui.Output, receipt *domain.ValidationReceipt, verifyErr error) {
	out.Info(fmt.Sprintf("Receipt: %s", receipt.ReceiptID))
	out.Info(fmt.Sprintf("Step: %s", receipt.StepName))
	out.Info(fmt.Sprintf("Command: %s", receipt.Command))
	out.Info(fmt.Sprintf("Exit Code: %d", receipt.ExitCode))
	out.Info(fmt.Sprintf("Duration: %s", receipt.Duration))
	// KeyPath is available if signed
	if receipt.KeyPath != "" {
		out.Info(fmt.Sprintf("Key Path: %s", receipt.KeyPath))
	}

	if verifyErr == nil {
		out.Success("Signature: VALID")
	} else {
		out.Warning(fmt.Sprintf("Signature: INVALID - %v", verifyErr))
	}
}

// formatRelativeTime formats a time as relative (e.g., "2 minutes ago").
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

// outputHookErrorJSON outputs an error result as JSON for hook commands.
func outputHookErrorJSON(w io.Writer, command, errMsg string) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(map[string]any{
		"success": false,
		"command": command,
		"error":   errMsg,
	}); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return atlaserrors.ErrJSONErrorOutput
}
