// Package cli provides the command-line interface for atlas.
package cli

import (
	"github.com/spf13/cobra"
)

// newWorkspaceCmd creates the parent workspace command.
func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage ATLAS workspaces",
		Long: `Commands for managing ATLAS workspaces including listing,
destroying, and retiring workspaces.

A workspace represents an isolated development environment with its own
git worktree and task history.`,
		// No RunE - parent command just displays help
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Add subcommands
	addWorkspaceListCmd(cmd)
	addWorkspaceDestroyCmd(cmd)
	addWorkspaceRetireCmd(cmd)
	// Future: addWorkspaceLogsCmd(cmd)

	return cmd
}

// AddWorkspaceCommand adds the workspace command tree to the root command.
func AddWorkspaceCommand(parent *cobra.Command) {
	parent.AddCommand(newWorkspaceCmd())
}
