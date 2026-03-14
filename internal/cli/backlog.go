// Package cli provides the command-line interface for atlas.
package cli

import (
	"github.com/spf13/cobra"
)

// AddBacklogCommand adds the backlog command group to the root command.
func AddBacklogCommand(root *cobra.Command) {
	backlogCmd := &cobra.Command{
		Use:   "backlog",
		Short: "Manage the work backlog for discovered issues",
		Long: `Commands for managing discoveries in the work backlog.

The backlog captures issues discovered during AI-assisted development that
cannot be addressed in the current task scope. Each discovery is stored as
an individual YAML file in .atlas/backlog/ to enable frictionless capture
and zero merge conflicts.

Examples:
  atlas backlog add "Missing error handling"    # Add a new discovery
  atlas backlog list                            # List pending discoveries
  atlas backlog view disc-abc123                # View discovery details
  atlas backlog promote disc-abc123             # Promote to task
  atlas backlog dismiss disc-abc123 --reason "duplicate"  # Dismiss`,
	}

	backlogCmd.AddCommand(newBacklogAddCmd())
	backlogCmd.AddCommand(newBacklogListCmd())
	backlogCmd.AddCommand(newBacklogViewCmd())
	backlogCmd.AddCommand(newBacklogPromoteCmd())
	backlogCmd.AddCommand(newBacklogDismissCmd())

	root.AddCommand(backlogCmd)
}
