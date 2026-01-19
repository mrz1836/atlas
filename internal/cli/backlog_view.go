package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"

	"github.com/mrz1836/atlas/internal/backlog"
	"github.com/mrz1836/atlas/internal/tui"
)

var (
	glamourRenderer     *glamour.TermRenderer //nolint:gochecknoglobals // cached renderer for performance
	glamourRendererOnce sync.Once             //nolint:gochecknoglobals // sync.Once for renderer initialization
)

// getGlamourRenderer returns a cached glamour renderer for markdown rendering.
// The renderer is initialized once and reused across all calls.
func getGlamourRenderer() *glamour.TermRenderer {
	glamourRendererOnce.Do(func() {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err == nil {
			glamourRenderer = r
		}
	})
	return glamourRenderer
}

// newBacklogViewCmd creates the backlog view command.
func newBacklogViewCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "View full details of a discovery",
		Long: `View full details of a discovery by ID.

Displays all information about a discovery including description,
location, context, and lifecycle information.

Examples:
  atlas backlog view disc-abc123        # View discovery details
  atlas backlog view disc-abc123 --json # Output as JSON

Exit codes:
  0: Success
  1: Discovery not found or error`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBacklogView(cmd.Context(), cmd, os.Stdout, args[0], jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

// runBacklogView executes the backlog view command.
func runBacklogView(ctx context.Context, cmd *cobra.Command, w io.Writer, id string, jsonOutput bool) error {
	outputFormat := getOutputFormat(cmd, jsonOutput)
	out := tui.NewOutput(w, outputFormat)

	// Create manager
	mgr, err := backlog.NewManager("")
	if err != nil {
		return outputBacklogError(w, outputFormat, "view", err)
	}

	// Get discovery
	d, err := mgr.Get(ctx, id)
	if err != nil {
		return outputBacklogError(w, outputFormat, "view", err)
	}

	// Output results
	if outputFormat == OutputJSON {
		return out.JSON(d)
	}

	displayBacklogView(out, w, d)
	return nil
}

// displayBacklogView displays a discovery in rich detail format.
func displayBacklogView(out tui.Output, w io.Writer, d *backlog.Discovery) {
	// Header
	out.Info(fmt.Sprintf("Discovery: %s", d.ID))
	out.Info(strings.Repeat("━", 50))
	out.Info("")

	// Basic info
	out.Info(fmt.Sprintf("Title:      %s", d.Title))
	out.Info(fmt.Sprintf("Status:     %s", d.Status))
	out.Info(fmt.Sprintf("Category:   %s", d.Content.Category))
	out.Info(fmt.Sprintf("Severity:   %s", d.Content.Severity))
	out.Info("")

	// Description with markdown rendering
	if d.Content.Description != "" {
		out.Info("Description:")
		renderDescription(w, d.Content.Description)
		out.Info("")
	}

	// Location
	if d.Location != nil && d.Location.File != "" {
		if d.Location.Line > 0 {
			out.Info(fmt.Sprintf("Location:   %s:%d", d.Location.File, d.Location.Line))
		} else {
			out.Info(fmt.Sprintf("Location:   %s", d.Location.File))
		}
	}

	// Tags
	if len(d.Content.Tags) > 0 {
		out.Info(fmt.Sprintf("Tags:       %s", strings.Join(d.Content.Tags, ", ")))
	}

	out.Info("")

	// Context
	out.Info(fmt.Sprintf("Discovered: %s", d.Context.DiscoveredAt.Format("2006-01-02 15:04:05 MST")))
	out.Info(fmt.Sprintf("By:         %s", d.Context.DiscoveredBy))

	if d.Context.DuringTask != "" {
		out.Info(fmt.Sprintf("During:     %s", d.Context.DuringTask))
	}

	if d.Context.Git != nil {
		if d.Context.Git.Branch != "" && d.Context.Git.Commit != "" {
			out.Info(fmt.Sprintf("Git:        %s @ %s", d.Context.Git.Branch, d.Context.Git.Commit))
		} else if d.Context.Git.Branch != "" {
			out.Info(fmt.Sprintf("Git:        %s", d.Context.Git.Branch))
		}
	}

	// Lifecycle info
	if d.Status == backlog.StatusPromoted && d.Lifecycle.PromotedToTask != "" {
		out.Info("")
		out.Info(fmt.Sprintf("Promoted to task: %s", d.Lifecycle.PromotedToTask))
	}

	if d.Status == backlog.StatusDismissed && d.Lifecycle.DismissedReason != "" {
		out.Info("")
		out.Info(fmt.Sprintf("Dismissed reason: %s", d.Lifecycle.DismissedReason))
	}
}

// renderDescription renders markdown description using glamour.
func renderDescription(w io.Writer, description string) {
	if renderer := getGlamourRenderer(); renderer != nil {
		if rendered, err := renderer.Render(description); err == nil {
			// Indent the rendered output
			for _, line := range strings.Split(rendered, "\n") {
				_, _ = fmt.Fprintf(w, "  %s\n", line)
			}
			return
		}
	}
	// Fallback to plain text
	_, _ = fmt.Fprintf(w, "  %s\n", description)
}
