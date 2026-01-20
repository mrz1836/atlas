package ai

import (
	"regexp"
	"strings"
	"time"
)

// ActivityParser parses stderr lines from AI CLIs into ActivityEvent structs.
type ActivityParser struct {
	// Compiled patterns for efficient matching
	patterns []activityPattern
}

// activityPattern represents a pattern for matching activity lines.
type activityPattern struct {
	regex    *regexp.Regexp
	actType  ActivityType
	msgFunc  func(matches []string) string
	fileFunc func(matches []string) string
}

// NewActivityParser creates a new ActivityParser with default patterns.
func NewActivityParser() *ActivityParser {
	p := &ActivityParser{}
	p.initPatterns()
	return p
}

// ParseLine attempts to parse a stderr line into an ActivityEvent.
// Returns nil if the line doesn't match any known pattern.
func (p *ActivityParser) ParseLine(line string) *ActivityEvent {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Try each pattern in order
	for _, pat := range p.patterns {
		if matches := pat.regex.FindStringSubmatch(line); len(matches) > 0 {
			event := &ActivityEvent{
				Timestamp: time.Now(),
				Type:      pat.actType,
				Message:   pat.msgFunc(matches),
			}
			if pat.fileFunc != nil {
				event.File = pat.fileFunc(matches)
			}
			return event
		}
	}

	// Check for generic file path mentions (fallback)
	if fileEvent := p.parseGenericFileMention(line); fileEvent != nil {
		return fileEvent
	}

	return nil
}

// AddPattern allows adding custom patterns to the parser.
// This is useful for CLI-specific patterns.
func (p *ActivityParser) AddPattern(pattern string, actType ActivityType, msgFunc, fileFunc func([]string) string) error {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	p.patterns = append([]activityPattern{{
		regex:    regex,
		actType:  actType,
		msgFunc:  msgFunc,
		fileFunc: fileFunc,
	}}, p.patterns...) // Prepend to have higher priority

	return nil
}

// initPatterns initializes the regex patterns for activity detection.
// Patterns are ordered by specificity - more specific patterns first.
//
//nolint:gocognit // Pattern initialization is necessarily verbose
func (p *ActivityParser) initPatterns() {
	p.patterns = []activityPattern{
		// File reading patterns (more specific patterns first)
		{
			regex:   regexp.MustCompile(`(?i)^read\s+tool:\s*(.+)$`),
			actType: ActivityReading,
			msgFunc: func(_ []string) string { return "Reading" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},
		{
			regex:   regexp.MustCompile(`(?i)^(?:reading|read)\s+(?:file:?\s*)?(.+)$`),
			actType: ActivityReading,
			msgFunc: func(_ []string) string { return "Reading" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},

		// File writing patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:writing|write|wrote)\s+(?:to:?\s*)?(.+)$`),
			actType: ActivityWriting,
			msgFunc: func(_ []string) string { return "Writing" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},
		{
			regex:   regexp.MustCompile(`(?i)^(?:edit|editing)\s+(?:file:?\s*)?(.+)$`),
			actType: ActivityWriting,
			msgFunc: func(_ []string) string { return "Editing" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},

		// Searching patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:searching|search|grep|glob)\s+(?:for:?\s*)?(.+)$`),
			actType: ActivitySearching,
			msgFunc: func(_ []string) string { return "Searching" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},

		// Executing patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:executing|running|exec|run)(?::\s*|\s+)(?:command:?\s*)?(.+)$`),
			actType: ActivityExecuting,
			msgFunc: func(_ []string) string { return "Executing" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},
		{
			regex:   regexp.MustCompile(`(?i)^bash:\s*(.+)$`),
			actType: ActivityExecuting,
			msgFunc: func(_ []string) string { return "Running" },
			fileFunc: func(m []string) string {
				if len(m) > 1 {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},

		// Thinking patterns
		{
			regex:    regexp.MustCompile(`(?i)^(?:thinking|processing)\.{0,3}$`),
			actType:  ActivityThinking,
			msgFunc:  func(_ []string) string { return "Thinking..." },
			fileFunc: func(_ []string) string { return "" },
		},

		// Planning patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:planning|plan)(?::?\s*(.+))?$`),
			actType: ActivityPlanning,
			msgFunc: func(m []string) string {
				if len(m) > 1 && m[1] != "" {
					return "Planning: " + strings.TrimSpace(m[1])
				}
				return "Planning..."
			},
			fileFunc: func(_ []string) string { return "" },
		},

		// Analyzing patterns (supports both British and American spelling)
		{
			regex:   regexp.MustCompile(`(?i)^(?:analyzing|analyse|analyze)(?::?\s*(.+))?$`), //nolint:misspell // Intentionally matches British spelling
			actType: ActivityAnalyzing,
			msgFunc: func(m []string) string {
				if len(m) > 1 && m[1] != "" {
					return "Analyzing " + strings.TrimSpace(m[1])
				}
				return "Analyzing..."
			},
			fileFunc: func(m []string) string {
				if len(m) > 1 && m[1] != "" {
					return strings.TrimSpace(m[1])
				}
				return ""
			},
		},

		// Implementing patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:implementing|implement)(?::?\s*(.+))?$`),
			actType: ActivityImplementing,
			msgFunc: func(m []string) string {
				if len(m) > 1 && m[1] != "" {
					return "Implementing: " + strings.TrimSpace(m[1])
				}
				return "Implementing..."
			},
			fileFunc: func(_ []string) string { return "" },
		},

		// Verifying patterns
		{
			regex:   regexp.MustCompile(`(?i)^(?:verifying|verify|checking|check)(?::?\s*(.+))?$`),
			actType: ActivityVerifying,
			msgFunc: func(m []string) string {
				if len(m) > 1 && m[1] != "" {
					return "Verifying: " + strings.TrimSpace(m[1])
				}
				return "Verifying..."
			},
			fileFunc: func(_ []string) string { return "" },
		},

		// Tool usage patterns (Claude Code specific)
		{
			regex:   regexp.MustCompile(`(?i)^(?:using tool:?\s*)(\w+)(?:\s+on\s+(.+))?$`),
			actType: ActivityReading, // Will be overridden based on tool
			msgFunc: func(m []string) string {
				if len(m) > 1 {
					return "Using " + m[1]
				}
				return "Using tool"
			},
			fileFunc: func(m []string) string {
				if len(m) > 2 {
					return strings.TrimSpace(m[2])
				}
				return ""
			},
		},
	}
}

// parseGenericFileMention looks for file paths in the line.
// This is a fallback for lines that don't match specific patterns.
func (p *ActivityParser) parseGenericFileMention(line string) *ActivityEvent {
	// Look for common file path patterns
	filePatterns := []*regexp.Regexp{
		regexp.MustCompile(`([a-zA-Z0-9_\-./]+\.(go|tsx|ts|jsx|js|py|rb|rs|java|c|cpp|h|hpp|md|yaml|yml|json|toml))`),
		regexp.MustCompile(`(?:^|[\s:])(/[a-zA-Z0-9_\-./]+)`),
		regexp.MustCompile(`(?:^|[\s:])(\./[a-zA-Z0-9_\-./]+)`),
	}

	for _, pat := range filePatterns {
		if matches := pat.FindStringSubmatch(line); len(matches) > 1 {
			filePath := matches[1]

			// Skip if it looks like a version number or timestamp
			if looksLikeVersion(filePath) {
				continue
			}

			// Determine activity type from context
			actType := ActivityReading
			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, "writ") || strings.Contains(lowerLine, "edit") ||
				strings.Contains(lowerLine, "creat") || strings.Contains(lowerLine, "modif") {
				actType = ActivityWriting
			}

			return &ActivityEvent{
				Timestamp: time.Now(),
				Type:      actType,
				Message:   line,
				File:      filePath,
			}
		}
	}

	return nil
}

// looksLikeVersion returns true if the string looks like a version number.
func looksLikeVersion(s string) bool {
	// Skip strings that look like version numbers (e.g., "1.24.0", "v2.0.0")
	versionPattern := regexp.MustCompile(`^v?\d+\.\d+(\.\d+)?$`)
	return versionPattern.MatchString(s)
}
