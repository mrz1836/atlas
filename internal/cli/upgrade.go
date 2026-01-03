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
	"sync"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
)

// UpgradeFlags holds flags specific to the upgrade command.
type UpgradeFlags struct {
	// Check performs a dry-run, showing updates without installing.
	Check bool
	// Yes skips confirmation prompts.
	Yes bool
	// OutputFormat specifies the output format (text or json).
	OutputFormat string
}

// UpdateInfo contains version information for a single tool.
type UpdateInfo struct {
	// Name is the tool name.
	Name string `json:"name"`
	// CurrentVersion is the currently installed version.
	CurrentVersion string `json:"current_version"`
	// LatestVersion is the latest available version (may be empty if unknown).
	LatestVersion string `json:"latest_version,omitempty"`
	// UpdateAvailable indicates if an update is available.
	UpdateAvailable bool `json:"update_available"`
	// Installed indicates if the tool is currently installed.
	Installed bool `json:"installed"`
	// InstallPath is the go install path for the tool.
	InstallPath string `json:"-"`
}

// UpgradeResult contains the result of upgrading a single tool.
type UpgradeResult struct {
	// Tool is the tool name.
	Tool string `json:"tool"`
	// Success indicates if the upgrade was successful.
	Success bool `json:"success"`
	// Error contains the error message if the upgrade failed.
	Error string `json:"error,omitempty"`
	// OldVersion is the version before upgrade.
	OldVersion string `json:"old_version"`
	// NewVersion is the version after upgrade (if successful).
	NewVersion string `json:"new_version,omitempty"`
	// Warnings contains non-fatal warning messages.
	Warnings []string `json:"warnings,omitempty"`
}

// UpdateCheckResult contains the results of checking for updates.
type UpdateCheckResult struct {
	// UpdatesAvailable indicates if any updates are available.
	UpdatesAvailable bool `json:"updates_available"`
	// Tools contains the update info for each tool.
	Tools []UpdateInfo `json:"tools"`
}

// UpgradeChecker defines the interface for checking tool updates.
type UpgradeChecker interface {
	// CheckAllUpdates checks for updates to all tools.
	CheckAllUpdates(ctx context.Context) (*UpdateCheckResult, error)
	// CheckToolUpdate checks for updates to a specific tool.
	CheckToolUpdate(ctx context.Context, tool string) (*UpdateInfo, error)
}

// UpgradeExecutor defines the interface for executing tool upgrades.
type UpgradeExecutor interface {
	// UpgradeTool upgrades a specific tool.
	UpgradeTool(ctx context.Context, tool string) (*UpgradeResult, error)
	// BackupConstitution backs up the Speckit constitution.md file.
	BackupConstitution() (string, error)
	// RestoreConstitution restores the Speckit constitution.md file from backup.
	RestoreConstitution(originalPath string) error
	// CleanupConstitutionBackup removes the constitution.md backup file.
	CleanupConstitutionBackup(originalPath string) error
}

// upgradeStyles contains styling for the upgrade command output.
type upgradeStyles struct {
	header     lipgloss.Style
	toolName   lipgloss.Style
	version    lipgloss.Style
	updateAvl  lipgloss.Style
	upToDate   lipgloss.Style
	success    lipgloss.Style
	err        lipgloss.Style
	info       lipgloss.Style
	dim        lipgloss.Style
	spinner    lipgloss.Style
	installing lipgloss.Style
}

// newUpgradeStyles creates styles for upgrade command output.
func newUpgradeStyles() *upgradeStyles {
	return &upgradeStyles{
		header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00D7FF")).
			MarginBottom(1),
		toolName: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")),
		version: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")),
		updateAvl: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")), // Yellow for update available
		upToDate: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")), // Green for up to date
		success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		err: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")).
			Bold(true),
		info: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")),
		spinner: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")),
		installing: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")),
	}
}

// upgradeCmd holds the dependencies for the upgrade command.
type upgradeCmd struct {
	flags    *UpgradeFlags
	checker  UpgradeChecker
	executor UpgradeExecutor
	styles   *upgradeStyles
	w        io.Writer
}

// getValidToolNames returns the list of valid tool names for upgrade.
func getValidToolNames() []string {
	return []string{
		constants.ToolAtlas,
		constants.ToolMageX,
		constants.ToolGoPreCommit,
		constants.ToolSpeckit,
	}
}

// newUpgradeCmd creates the upgrade command for updating ATLAS and managed tools.
func newUpgradeCmd(flags *UpgradeFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [tool]",
		Short: "Upgrade ATLAS and managed tools",
		Long: `Upgrade ATLAS and its managed tools to their latest versions.

By default, checks and upgrades all tools:
  - atlas: The ATLAS CLI itself
  - mage-x: Build automation tool
  - go-pre-commit: Git hooks tool
  - speckit: Specification tool

You can upgrade a specific tool by providing its name as an argument.

For Speckit upgrades, constitution.md is automatically backed up and restored.

Examples:
  atlas upgrade              # Check and upgrade all tools
  atlas upgrade --check      # Only check for updates (dry-run)
  atlas upgrade -y           # Upgrade without confirmation
  atlas upgrade speckit      # Upgrade only Speckit
  atlas upgrade atlas        # Upgrade only ATLAS itself
  atlas upgrade --check --output json   # Check for updates in JSON format`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var tool string
			if len(args) > 0 {
				tool = args[0]
				// Validate tool name
				if !isValidTool(tool) {
					validNames := getValidToolNames()
					return fmt.Errorf("%w: %q (valid options: %s)", errors.ErrInvalidToolName, tool, strings.Join(validNames, ", "))
				}
			}
			return runUpgrade(cmd.Context(), cmd.OutOrStdout(), flags, tool)
		},
		SilenceUsage: true,
	}

	// Add flags
	cmd.Flags().BoolVarP(&flags.Check, "check", "c", false, "only check for updates without installing")
	cmd.Flags().BoolVarP(&flags.Yes, "yes", "y", false, "skip confirmation prompt")
	cmd.Flags().StringVarP(&flags.OutputFormat, "output", "o", "text", "output format (text or json)")

	return cmd
}

// AddUpgradeCommand adds the upgrade command to the root command.
func AddUpgradeCommand(rootCmd *cobra.Command) {
	flags := &UpgradeFlags{}
	rootCmd.AddCommand(newUpgradeCmd(flags))
}

// isValidTool checks if the provided tool name is valid.
func isValidTool(tool string) bool {
	for _, valid := range getValidToolNames() {
		if tool == valid {
			return true
		}
	}
	return false
}

// runUpgrade executes the upgrade command with default dependencies.
func runUpgrade(ctx context.Context, w io.Writer, flags *UpgradeFlags, tool string) error {
	executor := &DefaultCommandExecutor{}
	checker := NewDefaultUpgradeChecker(executor)
	upgradeExec := NewDefaultUpgradeExecutor(executor)

	return runUpgradeWithDeps(ctx, w, flags, tool, checker, upgradeExec)
}

// runUpgradeWithDeps executes the upgrade command with custom dependencies.
// This allows for mocking in tests.
func runUpgradeWithDeps(ctx context.Context, w io.Writer, flags *UpgradeFlags, tool string, checker UpgradeChecker, executor UpgradeExecutor) error {
	// Check cancellation at entry
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	cmd := &upgradeCmd{
		flags:    flags,
		checker:  checker,
		executor: executor,
		styles:   newUpgradeStyles(),
		w:        w,
	}

	return cmd.run(ctx, tool)
}

// run executes the upgrade command logic.
func (u *upgradeCmd) run(ctx context.Context, tool string) error {
	// Check for updates
	checkResult, err := u.checkForUpdates(ctx, tool)
	if err != nil {
		return err
	}

	// Handle JSON output mode
	if strings.ToLower(u.flags.OutputFormat) == "json" {
		return u.handleJSONOutput(checkResult)
	}

	// Display update check results (text mode)
	u.displayUpdateTable(checkResult)

	// Handle --check mode (dry-run)
	if u.flags.Check {
		return u.handleCheckMode(checkResult)
	}

	// Get tools that need upgrades
	toolsToUpgrade := u.getToolsToUpgrade(checkResult)
	if len(toolsToUpgrade) == 0 {
		_, _ = fmt.Fprintln(u.w)
		_, _ = fmt.Fprintln(u.w, u.styles.success.Render("✓ All installed tools are up to date."))
		return nil
	}

	// Prompt for confirmation and execute upgrades
	return u.confirmAndExecuteUpgrades(ctx, toolsToUpgrade)
}

// checkForUpdates checks for updates to the specified tool or all tools.
func (u *upgradeCmd) checkForUpdates(ctx context.Context, tool string) (*UpdateCheckResult, error) {
	if tool != "" {
		info, err := u.checker.CheckToolUpdate(ctx, tool)
		if err != nil {
			return nil, fmt.Errorf("failed to check for updates: %w", err)
		}
		return &UpdateCheckResult{
			UpdatesAvailable: info.UpdateAvailable,
			Tools:            []UpdateInfo{*info},
		}, nil
	}

	result, err := u.checker.CheckAllUpdates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	return result, nil
}

// handleJSONOutput outputs results in JSON format and handles exit codes.
func (u *upgradeCmd) handleJSONOutput(checkResult *UpdateCheckResult) error {
	if err := u.outputJSON(checkResult); err != nil {
		return err
	}
	// For --check mode with JSON output, return exit code 1 if updates available (for scripting)
	if u.flags.Check && checkResult.UpdatesAvailable {
		return &ExitCodeError{Code: 1, Err: nil}
	}
	return nil
}

// handleCheckMode handles the --check flag (dry-run mode).
func (u *upgradeCmd) handleCheckMode(checkResult *UpdateCheckResult) error {
	if checkResult.UpdatesAvailable {
		return &ExitCodeError{Code: 1, Err: nil} // Exit code 1 if updates available (for scripting)
	}
	return nil
}

// getToolsToUpgrade returns the list of tools that need upgrades.
func (u *upgradeCmd) getToolsToUpgrade(checkResult *UpdateCheckResult) []UpdateInfo {
	if !checkResult.UpdatesAvailable {
		_, _ = fmt.Fprintln(u.w)
		_, _ = fmt.Fprintln(u.w, u.styles.success.Render("✓ All tools are up to date."))
		return nil
	}

	var toolsToUpgrade []UpdateInfo
	for _, t := range checkResult.Tools {
		if t.UpdateAvailable && t.Installed {
			toolsToUpgrade = append(toolsToUpgrade, t)
		}
	}
	return toolsToUpgrade
}

// confirmAndExecuteUpgrades prompts for confirmation and executes upgrades.
func (u *upgradeCmd) confirmAndExecuteUpgrades(ctx context.Context, toolsToUpgrade []UpdateInfo) error {
	if !u.flags.Yes {
		confirmed, err := u.promptConfirmation(len(toolsToUpgrade))
		if err != nil {
			return fmt.Errorf("failed to prompt for confirmation: %w", err)
		}
		if !confirmed {
			_, _ = fmt.Fprintln(u.w, u.styles.dim.Render("Upgrade canceled."))
			return nil
		}
	}

	results := u.executeUpgrades(ctx, toolsToUpgrade)
	u.displayUpgradeResults(results)
	return nil
}

// displayUpdateTable displays the update check results in a formatted table.
func (u *upgradeCmd) displayUpdateTable(result *UpdateCheckResult) {
	_, _ = fmt.Fprintln(u.w, u.styles.header.Render("ATLAS Upgrade Check"))
	_, _ = fmt.Fprintln(u.w)
	_, _ = fmt.Fprintln(u.w, u.styles.info.Render("Current versions:"))

	for _, tool := range result.Tools {
		statusStr := u.formatToolStatus(tool)
		version := tool.CurrentVersion
		if version == "" {
			version = "(not installed)"
		}

		// Check if version contains multiple lines (e.g., ASCII art from --version)
		if strings.Contains(version, "\n") {
			// Print tool name on its own line
			_, _ = fmt.Fprintf(u.w, "  %s\n", u.styles.toolName.Render(tool.Name))
			// Print multi-line version info
			_, _ = fmt.Fprintln(u.w, u.styles.version.Render(version))
			// Print status
			_, _ = fmt.Fprintln(u.w, statusStr)
		} else {
			// Single-line version: print on same line as tool name
			name := fmt.Sprintf("  %-17s", tool.Name)
			_, _ = fmt.Fprintf(u.w, "%s %s  %s\n",
				u.styles.toolName.Render(name),
				u.styles.version.Render(fmt.Sprintf("%-12s", version)),
				statusStr)
		}
	}
	_, _ = fmt.Fprintln(u.w)
}

// formatToolStatus returns a styled status string for a tool.
func (u *upgradeCmd) formatToolStatus(tool UpdateInfo) string {
	if !tool.Installed {
		return u.styles.dim.Render("(not installed)")
	}
	if tool.UpdateAvailable {
		if tool.LatestVersion != "" {
			return u.styles.updateAvl.Render(fmt.Sprintf("→ %s (update available)", tool.LatestVersion))
		}
		// MVP: We don't fetch latest version from registry, upgrade will determine if update occurred
		return u.styles.info.Render("(installed, upgrade to check)")
	}
	return u.styles.upToDate.Render("✓ latest")
}

// outputJSON outputs the update check result in JSON format.
func (u *upgradeCmd) outputJSON(result *UpdateCheckResult) error {
	encoder := json.NewEncoder(u.w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// promptConfirmation prompts the user to confirm the upgrade.
func (u *upgradeCmd) promptConfirmation(count int) (bool, error) {
	_, _ = fmt.Fprintln(u.w)

	var confirm bool
	title := fmt.Sprintf("Upgrade %d tool(s)?", count)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Affirmative("Yes").
				Negative("No").
				Value(&confirm),
		),
	).WithTheme(huh.ThemeCharm())

	if err := form.Run(); err != nil {
		return false, err
	}

	return confirm, nil
}

// executeUpgrades performs the actual upgrades for the specified tools.
func (u *upgradeCmd) executeUpgrades(ctx context.Context, tools []UpdateInfo) []UpgradeResult {
	results := make([]UpgradeResult, 0, len(tools))
	_, _ = fmt.Fprintln(u.w)

	for _, tool := range tools {
		// Check context cancellation
		select {
		case <-ctx.Done():
			results = append(results, UpgradeResult{
				Tool:       tool.Name,
				Success:    false,
				Error:      "canceled",
				OldVersion: tool.CurrentVersion,
			})
			continue
		default:
		}

		_, _ = fmt.Fprintf(u.w, "%s %s...\n",
			u.styles.installing.Render("Upgrading"),
			u.styles.toolName.Render(tool.Name))

		result, err := u.executor.UpgradeTool(ctx, tool.Name)
		if err != nil {
			results = append(results, UpgradeResult{
				Tool:       tool.Name,
				Success:    false,
				Error:      err.Error(),
				OldVersion: tool.CurrentVersion,
			})
			_, _ = fmt.Fprintf(u.w, "  %s\n", u.styles.err.Render("✗ Failed: "+err.Error()))
		} else {
			result.OldVersion = tool.CurrentVersion
			results = append(results, *result)
			_, _ = fmt.Fprintf(u.w, "  %s\n", u.styles.success.Render("✓ Success"))
			// Display any warnings
			for _, warning := range result.Warnings {
				_, _ = fmt.Fprintf(u.w, "  %s\n", u.styles.dim.Render(warning))
			}
		}
	}

	return results
}

// displayUpgradeResults displays the final upgrade results.
func (u *upgradeCmd) displayUpgradeResults(results []UpgradeResult) {
	_, _ = fmt.Fprintln(u.w)

	successCount := 0
	failCount := 0
	var failedTools []string
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
			failedTools = append(failedTools, r.Tool)
		}
	}

	if failCount == 0 {
		_, _ = fmt.Fprintln(u.w, u.styles.success.Render("✓ All upgrades completed successfully."))
	} else if successCount == 0 {
		_, _ = fmt.Fprintln(u.w, u.styles.err.Render("✗ All upgrades failed."))
		u.displayRollbackInfo(failedTools)
	} else {
		_, _ = fmt.Fprintf(u.w, "%s %d succeeded, %d failed.\n",
			u.styles.info.Render("Upgrade results:"),
			successCount, failCount)
		u.displayRollbackInfo(failedTools)
	}
}

// displayRollbackInfo shows recovery guidance when upgrades fail.
func (u *upgradeCmd) displayRollbackInfo(failedTools []string) {
	_, _ = fmt.Fprintln(u.w)
	_, _ = fmt.Fprintln(u.w, u.styles.info.Render("Recovery options:"))
	_, _ = fmt.Fprintln(u.w, u.styles.dim.Render("  • Retry failed upgrades: atlas upgrade <tool-name>"))
	_, _ = fmt.Fprintln(u.w, u.styles.dim.Render("  • Check network connectivity and Go installation"))
	_, _ = fmt.Fprintln(u.w, u.styles.dim.Render("  • Manual install: go install <package>@latest"))
	if len(failedTools) > 0 {
		_, _ = fmt.Fprintf(u.w, "%s %s\n",
			u.styles.dim.Render("  • Failed tools:"),
			u.styles.err.Render(strings.Join(failedTools, ", ")))
	}
}

// DefaultUpgradeChecker implements UpgradeChecker using the tool detector.
type DefaultUpgradeChecker struct {
	executor config.CommandExecutor
}

// NewDefaultUpgradeChecker creates a new DefaultUpgradeChecker.
func NewDefaultUpgradeChecker(executor config.CommandExecutor) *DefaultUpgradeChecker {
	return &DefaultUpgradeChecker{executor: executor}
}

// CheckAllUpdates checks for updates to all tools in parallel.
func (c *DefaultUpgradeChecker) CheckAllUpdates(ctx context.Context) (*UpdateCheckResult, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	tools := getUpgradableTools()
	result := &UpdateCheckResult{
		Tools: make([]UpdateInfo, 0, len(tools)),
	}
	var resultMu sync.Mutex

	g, gCtx := errgroup.WithContext(ctx)

	for _, tool := range tools {
		g.Go(func() error {
			info := c.checkToolUpdate(gCtx, tool)
			resultMu.Lock()
			result.Tools = append(result.Tools, info)
			if info.UpdateAvailable {
				result.UpdatesAvailable = true
			}
			resultMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	return result, nil
}

// CheckToolUpdate checks for updates to a specific tool.
func (c *DefaultUpgradeChecker) CheckToolUpdate(ctx context.Context, tool string) (*UpdateInfo, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	toolConfig := getToolConfig(tool)
	if toolConfig == nil {
		return nil, fmt.Errorf("%w: %s", errors.ErrUnknownTool, tool)
	}

	info := c.checkToolUpdate(ctx, *toolConfig)
	return &info, nil
}

// upgradableToolConfig holds configuration for an upgradable tool.
type upgradableToolConfig struct {
	name        string
	command     string
	versionFlag string
	installPath string
}

// getUpgradableTools returns the list of tools that can be upgraded.
func getUpgradableTools() []upgradableToolConfig {
	return []upgradableToolConfig{
		{
			name:        constants.ToolAtlas,
			command:     "atlas",
			versionFlag: "--version",
			installPath: constants.InstallPathAtlas,
		},
		{
			name:        constants.ToolMageX,
			command:     constants.ToolMageX,
			versionFlag: constants.VersionFlagStandard,
			installPath: constants.InstallPathMageX,
		},
		{
			name:        constants.ToolGoPreCommit,
			command:     constants.ToolGoPreCommit,
			versionFlag: constants.VersionFlagStandard,
			installPath: constants.InstallPathGoPreCommit,
		},
		{
			name:        constants.ToolSpeckit,
			command:     constants.ToolSpeckit,
			versionFlag: constants.VersionFlagSpeckit,
			installPath: constants.InstallPathSpeckit,
		},
	}
}

// getToolConfig returns the configuration for a specific tool.
func getToolConfig(name string) *upgradableToolConfig {
	for _, t := range getUpgradableTools() {
		if t.name == name {
			return &t
		}
	}
	return nil
}

// checkToolUpdate checks for updates to a single tool.
func (c *DefaultUpgradeChecker) checkToolUpdate(ctx context.Context, tool upgradableToolConfig) UpdateInfo {
	info := UpdateInfo{
		Name:        tool.name,
		InstallPath: tool.installPath,
	}

	// Check if tool exists in PATH
	_, err := c.executor.LookPath(tool.command)
	if err != nil {
		info.Installed = false
		info.UpdateAvailable = true // Can be installed
		return info
	}
	info.Installed = true

	// Get current version
	output, err := c.executor.Run(ctx, tool.command, tool.versionFlag)
	if err != nil {
		// Tool exists but version command failed
		info.CurrentVersion = "unknown"
		info.UpdateAvailable = true // Assume update is needed
		return info
	}

	info.CurrentVersion = parseVersionFromOutput(tool.name, output)

	// For MVP, we assume an update is potentially available if the tool is installed
	// A more sophisticated approach would query the package registry
	// For now, we'll mark it as "check available" and let the actual upgrade determine if there was a change
	info.UpdateAvailable = true

	return info
}

// parseVersionFromOutput extracts a version string from command output.
func parseVersionFromOutput(tool, output string) string {
	output = strings.TrimSpace(output)
	switch tool {
	case constants.ToolAtlas:
		return parseAtlasVersion(output)
	case constants.ToolSpeckit:
		if version := parseSpeckitVersion(output); version != "" {
			return version
		}
		// Fallthrough to generic parser if format changes
		return parseGenericVersion(output)
	default:
		return parseGenericVersion(output)
	}
}

// parseAtlasVersion extracts version from atlas version output.
func parseAtlasVersion(output string) string {
	// Parse "atlas version X.Y.Z (commit: abc, built: 2024-01-01)" → "X.Y.Z"
	if strings.HasPrefix(output, "atlas version ") {
		rest := strings.TrimPrefix(output, "atlas version ")
		if idx := strings.Index(rest, " "); idx > 0 {
			return rest[:idx]
		}
		return rest
	}
	// Fallback: first word
	if idx := strings.Index(output, " "); idx > 0 {
		return output[:idx]
	}
	return output
}

// parseSpeckitVersion extracts version from speckit version output.
func parseSpeckitVersion(output string) string {
	// Parse specify version output
	// Example: "CLI Version    1.0.0"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "cli version") {
			// Extract version from "CLI Version    1.0.0"
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[2] // Return "1.0.0"
			}
		}
	}
	return ""
}

// parseGenericVersion extracts version pattern from generic output.
func parseGenericVersion(output string) string {
	// Extract version pattern from output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for version patterns like "v1.2.3" or "1.2.3"
		for _, word := range strings.Fields(line) {
			word = strings.TrimPrefix(word, "v")
			if len(word) > 0 && word[0] >= '0' && word[0] <= '9' {
				// Check if it looks like a version
				if strings.Contains(word, ".") {
					return word
				}
			}
		}
	}
	return output
}

// DefaultUpgradeExecutor implements UpgradeExecutor.
type DefaultUpgradeExecutor struct {
	executor config.CommandExecutor
}

// NewDefaultUpgradeExecutor creates a new DefaultUpgradeExecutor.
func NewDefaultUpgradeExecutor(executor config.CommandExecutor) *DefaultUpgradeExecutor {
	return &DefaultUpgradeExecutor{executor: executor}
}

// UpgradeTool upgrades a specific tool.
func (e *DefaultUpgradeExecutor) UpgradeTool(ctx context.Context, tool string) (*UpgradeResult, error) {
	toolConfig := getToolConfig(tool)
	if toolConfig == nil {
		return nil, fmt.Errorf("%w: %s", errors.ErrUnknownTool, tool)
	}

	result := &UpgradeResult{
		Tool: tool,
	}

	// Handle Speckit constitution.md backup
	constitutionPath, backupWarning := e.handleSpeckitBackup(tool)
	if backupWarning != "" {
		result.Warnings = append(result.Warnings, backupWarning)
	}
	if constitutionPath != "" {
		defer e.handleSpeckitRestore(result, constitutionPath)
	}

	// Try tool-specific upgrade methods first
	if e.tryToolSpecificUpgrade(ctx, tool, toolConfig, result) {
		return result, nil
	}

	// Execute the upgrade via go install
	e.upgradeViaGoInstall(ctx, tool, toolConfig, result)
	return result, nil
}

// BackupConstitution backs up the Speckit constitution.md file.
func (e *DefaultUpgradeExecutor) BackupConstitution() (string, error) {
	// Check both possible locations for constitution.md
	locations := getConstitutionLocations()

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			backupPath := loc + ".backup"
			if err := copyFile(loc, backupPath); err != nil {
				return "", fmt.Errorf("failed to backup constitution.md: %w", err)
			}
			return loc, nil
		}
	}

	// No constitution.md found, nothing to backup
	return "", nil
}

// RestoreConstitution restores the Speckit constitution.md file from backup.
func (e *DefaultUpgradeExecutor) RestoreConstitution(originalPath string) error {
	if originalPath == "" {
		return nil
	}

	backupPath := originalPath + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		// No backup file exists - this is not an error, just nothing to restore
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return copyFile(backupPath, originalPath)
}

// CleanupConstitutionBackup removes the constitution.md backup file.
func (e *DefaultUpgradeExecutor) CleanupConstitutionBackup(originalPath string) error {
	if originalPath == "" {
		return nil
	}

	backupPath := originalPath + ".backup"
	return os.Remove(backupPath)
}

// tryToolSpecificUpgrade attempts to upgrade using tool-specific commands.
// Returns true if a tool-specific upgrade was attempted (regardless of success).
func (e *DefaultUpgradeExecutor) tryToolSpecificUpgrade(ctx context.Context, tool string, toolConfig *upgradableToolConfig, result *UpgradeResult) bool {
	switch tool {
	case constants.ToolMageX:
		if _, lookErr := e.executor.LookPath(constants.ToolMageX); lookErr == nil {
			e.upgradeMagex(ctx, tool, toolConfig, result)
			return true
		}
	case constants.ToolGoPreCommit:
		if _, lookErr := e.executor.LookPath(constants.ToolGoPreCommit); lookErr == nil {
			e.upgradeGoPreCommit(ctx, tool, toolConfig, result)
			return true
		}
	}
	return false
}

// upgradeMagex upgrades magex using its built-in update command.
func (e *DefaultUpgradeExecutor) upgradeMagex(ctx context.Context, tool string, toolConfig *upgradableToolConfig, result *UpgradeResult) {
	_, err := e.executor.Run(ctx, constants.ToolMageX, "update:install")
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return
	}
	result.Success = true
	e.updateResultVersion(ctx, tool, toolConfig, result)
}

// upgradeGoPreCommit upgrades go-pre-commit using its built-in upgrade command.
func (e *DefaultUpgradeExecutor) upgradeGoPreCommit(ctx context.Context, tool string, toolConfig *upgradableToolConfig, result *UpgradeResult) {
	_, err := e.executor.Run(ctx, constants.ToolGoPreCommit, "upgrade", "--force")
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return
	}
	result.Success = true
	e.updateResultVersion(ctx, tool, toolConfig, result)
}

// upgradeViaGoInstall upgrades a tool using go install.
func (e *DefaultUpgradeExecutor) upgradeViaGoInstall(ctx context.Context, tool string, toolConfig *upgradableToolConfig, result *UpgradeResult) {
	_, err := e.executor.Run(ctx, "go", "install", toolConfig.installPath)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return
	}
	result.Success = true
	e.updateResultVersion(ctx, tool, toolConfig, result)
}

// updateResultVersion updates the result with the new version after upgrade.
func (e *DefaultUpgradeExecutor) updateResultVersion(ctx context.Context, tool string, toolConfig *upgradableToolConfig, result *UpgradeResult) {
	output, err := e.executor.Run(ctx, toolConfig.command, toolConfig.versionFlag)
	if err == nil {
		result.NewVersion = parseVersionFromOutput(tool, output)
	}
}

// handleSpeckitBackup backs up constitution.md for Speckit upgrades.
// Returns the backup path and a warning message if backup failed.
func (e *DefaultUpgradeExecutor) handleSpeckitBackup(tool string) (string, string) {
	if tool != constants.ToolSpeckit {
		return "", ""
	}
	backupPath, err := e.BackupConstitution()
	if err != nil {
		// Return warning but continue - backup is best effort
		return "", fmt.Sprintf("Warning: failed to backup constitution.md: %v", err)
	}
	return backupPath, ""
}

// handleSpeckitRestore restores constitution.md after a successful Speckit upgrade.
func (e *DefaultUpgradeExecutor) handleSpeckitRestore(result *UpgradeResult, constitutionPath string) {
	if result.Success {
		if err := e.RestoreConstitution(constitutionPath); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Warning: failed to restore constitution.md: %v (backup at %s.backup)", err, constitutionPath))
			return // Don't cleanup backup if restore failed
		}
		if err := e.CleanupConstitutionBackup(constitutionPath); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Warning: failed to cleanup backup: %v", err))
		}
	}
}

// getConstitutionLocations returns the possible locations for constitution.md.
func getConstitutionLocations() []string {
	locations := []string{
		filepath.Join(".", ".speckit", "constitution.md"),
	}

	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations, filepath.Join(home, ".speckit", "constitution.md"))
	}

	return locations
}

// ExitCodeError represents an error with a specific exit code.
type ExitCodeError struct {
	Code int
	Err  error
}

// Error implements the error interface.
func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

// DefaultCommandExecutor wraps config.DefaultCommandExecutor for use in upgrade.
type DefaultCommandExecutor struct {
	config.DefaultCommandExecutor
}
