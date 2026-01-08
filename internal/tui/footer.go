// Package tui provides terminal user interface components for ATLAS.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mrz1836/atlas/internal/constants"
)

// ActionItem represents a single actionable item for the footer.
type ActionItem struct {
	Workspace string
	Action    string // Full command: "atlas approve workspace-name"
	Status    constants.TaskStatus
}

// StatusFooter renders actionable commands for attention-required tasks.
// Shows copy-paste commands for tasks that need user action.
type StatusFooter struct {
	items []ActionItem
}

// NewStatusFooter creates a new StatusFooter from status rows.
// Only includes rows with attention-required statuses.
func NewStatusFooter(rows []StatusRow) *StatusFooter {
	items := make([]ActionItem, 0, len(rows)) // Pre-allocate capacity

	for _, row := range rows {
		if IsAttentionStatus(row.Status) {
			action := SuggestedAction(row.Status)
			if action != "" {
				items = append(items, ActionItem{
					Workspace: row.Workspace,
					Action:    fmt.Sprintf("%s %s", action, row.Workspace),
					Status:    row.Status,
				})
			}
		}
	}

	return &StatusFooter{
		items: items,
	}
}

// HasItems returns true if there are attention items to display.
func (f *StatusFooter) HasItems() bool {
	return len(f.items) > 0
}

// Items returns a copy of the action items.
// Returns nil if there are no items.
func (f *StatusFooter) Items() []ActionItem {
	if len(f.items) == 0 {
		return nil
	}
	result := make([]ActionItem, len(f.items))
	copy(result, f.items)
	return result
}

// FormatSingleAction formats a single action command.
// Returns "Run: atlas approve workspace-name" with optional styling.
func FormatSingleAction(workspace, action string) string {
	return fmt.Sprintf("Run: %s %s", action, workspace)
}

// FormatMultipleActions formats multiple action commands, one per line.
func FormatMultipleActions(items []ActionItem) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = fmt.Sprintf("Run: %s", item.Action)
	}
	return strings.Join(lines, "\n")
}

// Render writes the footer to the writer.
// Outputs nothing if there are no attention items.
// Uses bold styling for the command with optional warning color.
func (f *StatusFooter) Render(w io.Writer) error {
	if !f.HasItems() {
		return nil // No footer to render
	}

	// Blank line before footer
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("write footer separator: %w", err)
	}

	// Render action commands
	if len(f.items) == 1 {
		if _, err := fmt.Fprintln(w, f.renderSingleAction(f.items[0])); err != nil {
			return fmt.Errorf("write action item: %w", err)
		}
		return nil
	}

	// Multiple items
	for _, item := range f.items {
		if _, err := fmt.Fprintln(w, f.renderSingleAction(item)); err != nil {
			return fmt.Errorf("write action item: %w", err)
		}
	}

	return nil
}

// RenderPlain writes the footer without any styling.
// Used for JSON output or testing.
func (f *StatusFooter) RenderPlain(w io.Writer) error {
	if !f.HasItems() {
		return nil
	}

	// Blank line before footer
	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("write footer separator: %w", err)
	}

	for _, item := range f.items {
		if _, err := fmt.Fprintf(w, "Run: %s\n", item.Action); err != nil {
			return fmt.Errorf("write action item: %w", err)
		}
	}

	return nil
}

// ToJSON returns the action items in a format suitable for JSON output.
// Returns nil if there are no items.
func (f *StatusFooter) ToJSON() []map[string]string {
	if !f.HasItems() {
		return nil
	}

	result := make([]map[string]string, len(f.items))
	for i, item := range f.items {
		result[i] = map[string]string{
			"workspace": item.Workspace,
			"action":    item.Action,
		}
	}
	return result
}

// renderSingleAction formats one action item with styling.
// Uses bold for the command portion.
func (f *StatusFooter) renderSingleAction(item ActionItem) string {
	const prefix = "Run: "
	command := item.Action

	if !HasColorSupport() {
		// NO_COLOR mode: plain text
		return fmt.Sprintf("%s%s", prefix, command)
	}

	// Apply bold styling to the command
	boldStyle := lipgloss.NewStyle().Bold(true)
	return fmt.Sprintf("%s%s", prefix, boldStyle.Render(command))
}
