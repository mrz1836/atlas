package backlog

import (
	"fmt"
	"regexp"
	"strings"
)

// DescriptionConfig allows customization of description generation.
type DescriptionConfig struct {
	// IncludeSeverityBadge adds a severity indicator to the title.
	IncludeSeverityBadge bool

	// IncludeDescription adds the discovery description to the output.
	IncludeDescription bool

	// IncludeLocation adds file/line location information.
	IncludeLocation bool

	// IncludeTags adds tags to the output.
	IncludeTags bool

	// IncludeCategory adds the category information.
	IncludeCategory bool

	// MaxDescriptionLength limits description length (0 = unlimited).
	MaxDescriptionLength int
}

// DefaultDescriptionConfig returns the default configuration for description generation.
func DefaultDescriptionConfig() *DescriptionConfig {
	return &DescriptionConfig{
		IncludeSeverityBadge: true,
		IncludeDescription:   true,
		IncludeLocation:      true,
		IncludeTags:          true,
		IncludeCategory:      true,
		MaxDescriptionLength: 0, // No limit by default
	}
}

// GenerateTaskDescription creates a rich task description from a discovery.
// It formats the discovery information into a human-readable format suitable
// for task descriptions.
func GenerateTaskDescription(d *Discovery) string {
	return GenerateTaskDescriptionWithConfig(d, nil)
}

// GenerateTaskDescriptionWithConfig creates a task description with custom configuration.
func GenerateTaskDescriptionWithConfig(d *Discovery, cfg *DescriptionConfig) string {
	if cfg == nil {
		cfg = DefaultDescriptionConfig()
	}

	var sb strings.Builder

	// Title with optional severity badge
	sb.WriteString(d.Title)
	if cfg.IncludeSeverityBadge && d.Content.Severity != "" {
		sb.WriteString(fmt.Sprintf(" [%s]", strings.ToUpper(string(d.Content.Severity))))
	}

	// Category
	if cfg.IncludeCategory && d.Content.Category != "" {
		sb.WriteString(fmt.Sprintf("\n\nCategory: %s", d.Content.Category))
	}

	// Description if available
	if cfg.IncludeDescription && d.Content.Description != "" {
		desc := d.Content.Description
		if cfg.MaxDescriptionLength > 0 && len(desc) > cfg.MaxDescriptionLength {
			desc = desc[:cfg.MaxDescriptionLength] + "..."
		}
		sb.WriteString("\n\n")
		sb.WriteString(desc)
	}

	// Location context
	if cfg.IncludeLocation && d.Location != nil && d.Location.File != "" {
		sb.WriteString(fmt.Sprintf("\n\nLocation: %s", d.Location.File))
		if d.Location.Line > 0 {
			sb.WriteString(fmt.Sprintf(":%d", d.Location.Line))
		}
	}

	// Tags
	if cfg.IncludeTags && len(d.Content.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("\n\nTags: %s", strings.Join(d.Content.Tags, ", ")))
	}

	return sb.String()
}

// GenerateWorkspaceName creates a sanitized workspace name from a discovery title.
// The result is suitable for use as a git branch component or directory name.
func GenerateWorkspaceName(title string) string {
	return SanitizeWorkspaceName(title)
}

// Workspace name generation constants.
const (
	maxWorkspaceNameLen = 50
)

// Regex patterns for workspace name generation.
var (
	// nonAlphanumericRegex matches any character that is not a lowercase letter, digit, or hyphen.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	// multipleHyphensRegex matches consecutive hyphens.
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// SanitizeWorkspaceName sanitizes a string for use as a workspace name.
// It lowercases the input, replaces spaces with hyphens, removes special
// characters, and truncates to the maximum allowed length.
func SanitizeWorkspaceName(input string) string {
	// Lowercase and replace spaces with hyphens
	name := strings.ToLower(input)
	name = strings.ReplaceAll(name, " ", "-")

	// Remove special characters
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	// Collapse multiple hyphens
	name = multipleHyphensRegex.ReplaceAllString(name, "-")

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	// Truncate to max length
	if len(name) > maxWorkspaceNameLen {
		name = name[:maxWorkspaceNameLen]
		// Don't end with a hyphen
		name = strings.TrimRight(name, "-")
	}

	return name
}

// FormatLocation formats a location for display.
// Returns an empty string if the location is nil or has no file.
func FormatLocation(loc *Location) string {
	if loc == nil || loc.File == "" {
		return ""
	}

	if loc.Line > 0 {
		return fmt.Sprintf("%s:%d", loc.File, loc.Line)
	}

	return loc.File
}

// GenerateBranchName creates a branch name from template prefix and workspace name.
// For example, given prefix "fix" and workspace "null-pointer-bug", it returns "fix/null-pointer-bug".
func GenerateBranchName(prefix, workspaceName string) string {
	if prefix == "" {
		return workspaceName
	}
	return fmt.Sprintf("%s/%s", prefix, workspaceName)
}
