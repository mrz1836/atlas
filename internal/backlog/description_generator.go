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
		fmt.Fprintf(&sb, " [%s]", strings.ToUpper(string(d.Content.Severity)))
	}

	// Category
	if cfg.IncludeCategory && d.Content.Category != "" {
		fmt.Fprintf(&sb, "\n\nCategory: %s", d.Content.Category)
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
		fmt.Fprintf(&sb, "\n\nLocation: %s", d.Location.File)
		if d.Location.Line > 0 {
			fmt.Fprintf(&sb, ":%d", d.Location.Line)
		}
	}

	// Tags
	if cfg.IncludeTags && len(d.Content.Tags) > 0 {
		fmt.Fprintf(&sb, "\n\nTags: %s", strings.Join(d.Content.Tags, ", "))
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
	maxWorkspaceNameLen = 40
)

// Regex patterns for workspace name generation.
var (
	// nonAlphanumericRegex matches any character that is not a lowercase letter, digit, or hyphen.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9-]+`)
	// multipleHyphensRegex matches consecutive hyphens.
	multipleHyphensRegex = regexp.MustCompile(`-+`)
	// wordSplitRegex matches word boundaries for stop word filtering.
	wordSplitRegex = regexp.MustCompile(`[\s/\\]+`)
)

// getStopWords returns common English words to filter from workspace names.
func getStopWords() map[string]bool {
	return map[string]bool{
		"i": true, "a": true, "an": true, "the": true,
		"is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true,
		"that": true, "this": true, "these": true, "those": true,
		"it": true, "its": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true,
		"and": true, "or": true, "but": true, "not": true,
		"found": true, "there": true, "here": true,
		"two": true, "easy": true, "some": true, "any": true,
	}
}

// SanitizeWorkspaceName sanitizes a string for use as a workspace name.
// It filters common stop words, lowercases the input, replaces spaces with hyphens,
// removes special characters, and truncates to the maximum allowed length.
func SanitizeWorkspaceName(input string) string {
	// Filter out stop words first
	name := removeStopWords(input)

	// Lowercase and replace spaces with hyphens
	name = strings.ToLower(name)
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

// removeStopWords filters common English stop words from the input string.
func removeStopWords(input string) string {
	stopWords := getStopWords()

	// Split on whitespace and path separators
	words := wordSplitRegex.Split(input, -1)

	// Filter out stop words
	filtered := make([]string, 0, len(words))
	for _, word := range words {
		lower := strings.ToLower(word)
		// Remove quotes and other punctuation for stop word check
		cleaned := nonAlphanumericRegex.ReplaceAllString(lower, "")
		if cleaned != "" && !stopWords[cleaned] {
			filtered = append(filtered, word)
		}
	}

	return strings.Join(filtered, " ")
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
