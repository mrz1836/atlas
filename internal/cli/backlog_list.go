package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/backlog"
	atlaserrors "github.com/mrz1836/atlas/internal/errors"
	"github.com/mrz1836/atlas/internal/tui"
)

// backlogListFlags holds the flags for the list command.
type backlogListFlags struct {
	status   string
	category string
	all      bool
	limit    int
	json     bool
}

// newBacklogListCmd creates the backlog list command.
func newBacklogListCmd() *cobra.Command {
	flags := &backlogListFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discoveries in the backlog",
		Long: `List discoveries in the work backlog.

By default, shows only pending discoveries. Use --all to include
promoted and dismissed discoveries.

Examples:
  atlas backlog list                    # List pending discoveries
  atlas backlog list --status pending   # Same as default
  atlas backlog list --status promoted  # List promoted discoveries
  atlas backlog list --all              # Include all statuses
  atlas backlog list --category bug     # Filter by category
  atlas backlog list --limit 10         # Limit to 10 results
  atlas backlog list --json             # Output as JSON

Exit codes:
  0: Success
  1: General error`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBacklogList(cmd.Context(), cmd, os.Stdout, flags)
		},
	}

	cmd.Flags().StringVar(&flags.status, "status", "pending", "Filter by status (pending, promoted, dismissed)")
	cmd.Flags().StringVarP(&flags.category, "category", "c", "", "Filter by category")
	cmd.Flags().BoolVarP(&flags.all, "all", "a", false, "Include all statuses (overrides --status)")
	cmd.Flags().IntVarP(&flags.limit, "limit", "n", 0, "Maximum items to show (0 = unlimited)")
	cmd.Flags().BoolVar(&flags.json, "json", false, "Output as JSON array")

	return cmd
}

// runBacklogList executes the backlog list command.
func runBacklogList(ctx context.Context, cmd *cobra.Command, w io.Writer, flags *backlogListFlags) error {
	outputFormat := cmd.Flag("output").Value.String()
	if flags.json {
		outputFormat = OutputJSON
	}
	out := tui.NewOutput(w, outputFormat)

	// Create manager
	mgr, err := backlog.NewManager("")
	if err != nil {
		return outputBacklogError(w, outputFormat, "list", err)
	}

	// Build filter
	filter := backlog.Filter{
		Limit: flags.limit,
	}

	// Status filter (unless --all is set)
	if !flags.all {
		status := backlog.Status(flags.status)
		if !status.IsValid() {
			return outputBacklogError(w, outputFormat, "list",
				fmt.Errorf("%w: %q is not valid, must be one of: %v", atlaserrors.ErrInvalidDiscoveryStatus, flags.status, backlog.ValidStatuses()))
		}
		filter.Status = &status
	}

	// Category filter
	if flags.category != "" {
		category := backlog.Category(flags.category)
		if !category.IsValid() {
			return outputBacklogError(w, outputFormat, "list",
				fmt.Errorf("%w: %q is not valid, must be one of: %v", atlaserrors.ErrInvalidArgument, flags.category, backlog.ValidCategories()))
		}
		filter.Category = &category
	}

	// List discoveries
	discoveries, err := mgr.List(ctx, filter)
	if err != nil {
		return outputBacklogError(w, outputFormat, "list", err)
	}

	// Output results
	if outputFormat == OutputJSON {
		return out.JSON(discoveries)
	}

	displayBacklogList(out, discoveries)
	return nil
}

// displayBacklogList displays the list of discoveries in table format.
func displayBacklogList(out tui.Output, discoveries []*backlog.Discovery) {
	if len(discoveries) == 0 {
		out.Info("No discoveries found.")
		return
	}

	// Print header
	out.Info(fmt.Sprintf("%-12s  %-40s  %-15s  %-8s  %s",
		"ID", "TITLE", "CATEGORY", "SEVERITY", "AGE"))

	// Print each discovery
	for _, d := range discoveries {
		title := d.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		age := formatAge(d.Context.DiscoveredAt)
		out.Info(fmt.Sprintf("%-12s  %-40s  %-15s  %-8s  %s",
			d.ID, title, d.Content.Category, d.Content.Severity, age))
	}
}

// formatAge formats a time as a human-readable age string.
func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1m"
		}
		return fmt.Sprintf("%dm", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1h"
		}
		return fmt.Sprintf("%dh", h)
	}

	days := int(d.Hours() / 24)
	if days == 1 {
		return "1d"
	}
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}

	weeks := days / 7
	if weeks == 1 {
		return "1w"
	}
	if weeks < 4 {
		return fmt.Sprintf("%dw", weeks)
	}

	months := days / 30
	if months == 1 {
		return "1mo"
	}
	return fmt.Sprintf("%dmo", months)
}
