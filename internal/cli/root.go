// Package cli provides the command-line interface for atlas.
package cli

import (
	"context"

	"github.com/spf13/cobra"
)

// newRootCmd creates and returns the root command for the atlas CLI.
// This function-based approach avoids package-level globals, making the
// code more testable and avoiding gochecknoglobals linter warnings.
func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "atlas",
		Short: "ATLAS - AI Task Lifecycle Automation System",
		Long: `ATLAS automates the software development lifecycle with AI-powered task execution,
validation, and delivery through an intuitive CLI interface.`,
	}
}

// Execute runs the root command with the provided context.
func Execute(ctx context.Context) error {
	return newRootCmd().ExecuteContext(ctx)
}
