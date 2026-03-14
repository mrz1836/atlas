// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/hook"
	"github.com/mrz1836/atlas/internal/tui"
)

// Cleanup retention defaults (overridden by config).
const (
	defaultCompletedRetention = 30 * 24 * time.Hour // 30 days
	defaultFailedRetention    = 7 * 24 * time.Hour  // 7 days
	defaultAbandonedRetention = 7 * 24 * time.Hour  // 7 days
)

// AddCleanupCommand adds the cleanup command to the root command.
func AddCleanupCommand(root *cobra.Command) {
	root.AddCommand(newCleanupCmd())
}

// newCleanupCmd creates the cleanup command for removing old artifacts.
func newCleanupCmd() *cobra.Command {
	var dryRun bool
	var hooksOnly bool

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up old task artifacts and hook files",
		Long: `Clean up old task artifacts and hook files based on retention policies.

By default, hooks are retained based on their terminal state:
- Completed hooks: 30 days
- Failed hooks: 7 days
- Abandoned hooks: 7 days

Use --dry-run to preview what would be deleted without actually removing files.
Use --hooks to only clean up hook files (skip other artifact cleanup).

Examples:
  atlas cleanup              # Clean up all old artifacts
  atlas cleanup --dry-run    # Preview what would be deleted
  atlas cleanup --hooks      # Only clean up old hooks

Exit codes:
  0: Cleanup completed successfully
  1: Cleanup failed`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCleanup(cmd.Context(), cmd, os.Stdout, dryRun, hooksOnly)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be deleted without removing files")
	cmd.Flags().BoolVar(&hooksOnly, "hooks", false, "Only clean up hook files")

	return cmd
}

// runCleanup executes the cleanup command.
func runCleanup(ctx context.Context, cmd *cobra.Command, w io.Writer, dryRun, _ bool) error {
	outputFormat := cmd.Flag("output").Value.String()
	out := tui.NewOutput(w, outputFormat)

	// Load config and get hook store
	hookStore, cfg, err := setupCleanup(ctx)
	if err != nil {
		return err
	}

	// Collect hooks to clean up
	toDelete, stats, err := collectStaleHooks(ctx, hookStore, cfg)
	if err != nil {
		return err
	}

	// Handle empty result
	if len(toDelete) == 0 {
		return handleNoHooksToClean(out, outputFormat, dryRun)
	}

	// Handle dry run
	if dryRun {
		return outputDryRunResults(out, outputFormat, toDelete, stats)
	}

	// Perform actual deletion
	return performCleanup(ctx, hookStore, out, outputFormat, toDelete, stats)
}

// setupCleanup initializes the hook store and loads configuration.
func setupCleanup(ctx context.Context) (*hook.FileStore, *config.Config, error) {
	// Load config for retention settings
	cfg, err := config.Load(ctx)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Get base path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	baseDir := filepath.Join(homeDir, constants.AtlasHome)

	// Create hook store
	hookStore := hook.NewFileStore(baseDir)

	return hookStore, cfg, nil
}

// collectStaleHooks collects all hooks that are eligible for cleanup based on retention policies.
func collectStaleHooks(ctx context.Context, hookStore *hook.FileStore, cfg *config.Config) ([]*domain.Hook, cleanupStats, error) {
	var toDelete []*domain.Hook
	var stats cleanupStats

	// Get retention durations from config or use defaults
	completedRetention := getRetentionDuration(cfg.Hooks.Retention.Completed, defaultCompletedRetention)
	failedRetention := getRetentionDuration(cfg.Hooks.Retention.Failed, defaultFailedRetention)
	abandonedRetention := getRetentionDuration(cfg.Hooks.Retention.Abandoned, defaultAbandonedRetention)

	// Find stale hooks for each terminal state
	if err := collectStaleHooksByState(ctx, hookStore, completedRetention, domain.HookStateCompleted, &toDelete, &stats.completed); err != nil {
		return nil, stats, fmt.Errorf("failed to list stale completed hooks: %w", err)
	}

	if err := collectStaleHooksByState(ctx, hookStore, failedRetention, domain.HookStateFailed, &toDelete, &stats.failed); err != nil {
		return nil, stats, fmt.Errorf("failed to list stale failed hooks: %w", err)
	}

	if err := collectStaleHooksByState(ctx, hookStore, abandonedRetention, domain.HookStateAbandoned, &toDelete, &stats.abandoned); err != nil {
		return nil, stats, fmt.Errorf("failed to list stale abandoned hooks: %w", err)
	}

	return toDelete, stats, nil
}

// collectStaleHooksByState collects stale hooks for a specific state.
func collectStaleHooksByState(ctx context.Context, hookStore *hook.FileStore, retention time.Duration, state domain.HookState, toDelete *[]*domain.Hook, count *int) error {
	hooks, err := hookStore.ListStale(ctx, retention)
	if err != nil {
		return err
	}

	for _, h := range hooks {
		if h.State == state {
			*toDelete = append(*toDelete, h)
			*count++
		}
	}

	return nil
}

// handleNoHooksToClean handles the case where no hooks are eligible for cleanup.
func handleNoHooksToClean(out tui.Output, outputFormat string, dryRun bool) error {
	if outputFormat == OutputJSON {
		return out.JSON(map[string]any{
			"success": true,
			"dry_run": dryRun,
			"deleted": 0,
			"message": "No hooks eligible for cleanup",
		})
	}
	out.Info("No hooks eligible for cleanup.")
	return nil
}

// performCleanup performs the actual deletion of hooks and outputs results.
func performCleanup(ctx context.Context, hookStore *hook.FileStore, out tui.Output, outputFormat string, toDelete []*domain.Hook, stats cleanupStats) error {
	var deleteErrors []string
	deleted := 0

	for _, h := range toDelete {
		if err := hookStore.Delete(ctx, h.TaskID); err != nil {
			deleteErrors = append(deleteErrors, fmt.Sprintf("%s: %v", h.TaskID, err))
		} else {
			deleted++
		}
	}

	return outputCleanupResults(out, outputFormat, deleted, stats, deleteErrors)
}

// outputCleanupResults outputs the cleanup results in the appropriate format.
func outputCleanupResults(out tui.Output, outputFormat string, deleted int, stats cleanupStats, deleteErrors []string) error {
	if outputFormat == OutputJSON {
		result := map[string]any{
			"success":   len(deleteErrors) == 0,
			"dry_run":   false,
			"deleted":   deleted,
			"completed": stats.completed,
			"failed":    stats.failed,
			"abandoned": stats.abandoned,
		}
		if len(deleteErrors) > 0 {
			result["errors"] = deleteErrors
		}
		return out.JSON(result)
	}

	out.Success(fmt.Sprintf("Cleaned up %d hook files", deleted))
	out.Info(fmt.Sprintf("  Completed: %d", stats.completed))
	out.Info(fmt.Sprintf("  Failed: %d", stats.failed))
	out.Info(fmt.Sprintf("  Abandoned: %d", stats.abandoned))

	if len(deleteErrors) > 0 {
		out.Warning(fmt.Sprintf("Failed to delete %d hooks:", len(deleteErrors)))
		for _, errMsg := range deleteErrors {
			out.Info(fmt.Sprintf("  - %s", errMsg))
		}
	}

	return nil
}

// cleanupStats tracks cleanup statistics.
type cleanupStats struct {
	completed int
	failed    int
	abandoned int
}

// getRetentionDuration returns the config duration or default if zero.
func getRetentionDuration(configured, defaultDuration time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return defaultDuration
}

// outputDryRunResults outputs dry-run results.
func outputDryRunResults(out tui.Output, outputFormat string, toDelete []*domain.Hook, stats cleanupStats) error {
	if outputFormat == OutputJSON {
		hooks := make([]map[string]any, len(toDelete))
		for i, h := range toDelete {
			hooks[i] = map[string]any{
				"task_id":      h.TaskID,
				"workspace_id": h.WorkspaceID,
				"state":        h.State,
				"updated_at":   h.UpdatedAt.Format(time.RFC3339),
			}
		}
		return out.JSON(map[string]any{
			"success":      true,
			"dry_run":      true,
			"would_delete": len(toDelete),
			"completed":    stats.completed,
			"failed":       stats.failed,
			"abandoned":    stats.abandoned,
			"hooks":        hooks,
		})
	}

	out.Info(fmt.Sprintf("Would delete %d hook files (dry-run):", len(toDelete)))
	out.Info(fmt.Sprintf("  Completed: %d", stats.completed))
	out.Info(fmt.Sprintf("  Failed: %d", stats.failed))
	out.Info(fmt.Sprintf("  Abandoned: %d", stats.abandoned))
	out.Info("")
	out.Info("Files that would be deleted:")
	for _, h := range toDelete {
		out.Info(fmt.Sprintf("  - %s (%s, %s)", h.TaskID, h.State, h.UpdatedAt.Format("2006-01-02")))
	}
	return nil
}
