package ai

import (
	"testing"
)

//nolint:gocognit // Table-driven tests are necessarily verbose
func TestActivityParser_ParseLine(t *testing.T) {
	t.Parallel()

	parser := NewActivityParser()

	tests := []struct {
		name         string
		line         string
		wantNil      bool
		wantType     ActivityType
		wantFile     string
		wantContains string // substring the message should contain
	}{
		// Empty and whitespace
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "whitespace only",
			line:    "   ",
			wantNil: true,
		},

		// Reading patterns
		{
			name:         "reading file",
			line:         "Reading file: internal/ai/runner.go",
			wantNil:      false,
			wantType:     ActivityReading,
			wantFile:     "internal/ai/runner.go",
			wantContains: "Reading",
		},
		{
			name:         "read simple",
			line:         "read internal/config/config.go",
			wantNil:      false,
			wantType:     ActivityReading,
			wantFile:     "internal/config/config.go",
			wantContains: "Reading",
		},
		{
			name:         "read tool pattern",
			line:         "Read tool: src/main.go",
			wantNil:      false,
			wantType:     ActivityReading,
			wantFile:     "src/main.go",
			wantContains: "Reading",
		},

		// Writing patterns
		{
			name:         "writing to",
			line:         "Writing to: output.txt",
			wantNil:      false,
			wantType:     ActivityWriting,
			wantFile:     "output.txt",
			wantContains: "Writing",
		},
		{
			name:         "write simple",
			line:         "write test.go",
			wantNil:      false,
			wantType:     ActivityWriting,
			wantFile:     "test.go",
			wantContains: "Writing",
		},
		{
			name:         "edit file",
			line:         "edit file: internal/ai/base.go",
			wantNil:      false,
			wantType:     ActivityWriting,
			wantFile:     "internal/ai/base.go",
			wantContains: "Editing",
		},
		{
			name:         "editing without file prefix",
			line:         "editing runner.go",
			wantNil:      false,
			wantType:     ActivityWriting,
			wantFile:     "runner.go",
			wantContains: "Editing",
		},

		// Searching patterns
		{
			name:         "searching for",
			line:         "searching for: error handling",
			wantNil:      false,
			wantType:     ActivitySearching,
			wantFile:     "error handling",
			wantContains: "Searching",
		},
		{
			name:         "grep pattern",
			line:         "grep func main",
			wantNil:      false,
			wantType:     ActivitySearching,
			wantContains: "Searching",
		},
		{
			name:         "glob pattern",
			line:         "glob *.go",
			wantNil:      false,
			wantType:     ActivitySearching,
			wantContains: "Searching",
		},

		// Executing patterns
		{
			name:         "executing command",
			line:         "executing: go test ./...",
			wantNil:      false,
			wantType:     ActivityExecuting,
			wantContains: "Executing",
		},
		{
			name:         "running command",
			line:         "running make build",
			wantNil:      false,
			wantType:     ActivityExecuting,
			wantContains: "Executing",
		},
		{
			name:         "bash command",
			line:         "bash: npm install",
			wantNil:      false,
			wantType:     ActivityExecuting,
			wantFile:     "npm install",
			wantContains: "Running",
		},

		// Thinking patterns
		{
			name:         "thinking with dots",
			line:         "Thinking...",
			wantNil:      false,
			wantType:     ActivityThinking,
			wantContains: "Thinking",
		},
		{
			name:         "thinking without dots",
			line:         "thinking",
			wantNil:      false,
			wantType:     ActivityThinking,
			wantContains: "Thinking",
		},
		{
			name:         "processing",
			line:         "processing...",
			wantNil:      false,
			wantType:     ActivityThinking,
			wantContains: "Thinking",
		},

		// Planning patterns
		{
			name:         "planning with details",
			line:         "Planning: implement activity streaming",
			wantNil:      false,
			wantType:     ActivityPlanning,
			wantContains: "implement activity streaming",
		},
		{
			name:         "planning simple",
			line:         "planning",
			wantNil:      false,
			wantType:     ActivityPlanning,
			wantContains: "Planning",
		},

		// Analyzing patterns
		{
			name:         "analyzing with target",
			line:         "Analyzing codebase structure",
			wantNil:      false,
			wantType:     ActivityAnalyzing,
			wantContains: "Analyzing",
		},
		{
			name:         "analyze British spelling",
			line:         "analyse the function", //nolint:misspell // Testing British spelling support
			wantNil:      false,
			wantType:     ActivityAnalyzing,
			wantContains: "Analyzing",
		},

		// Implementing patterns
		{
			name:         "implementing with details",
			line:         "implementing: activity parser",
			wantNil:      false,
			wantType:     ActivityImplementing,
			wantContains: "activity parser",
		},
		{
			name:         "implement simple",
			line:         "implement",
			wantNil:      false,
			wantType:     ActivityImplementing,
			wantContains: "Implementing",
		},

		// Verifying patterns
		{
			name:         "verifying with target",
			line:         "verifying: test results",
			wantNil:      false,
			wantType:     ActivityVerifying,
			wantContains: "test results",
		},
		{
			name:         "checking",
			line:         "checking syntax",
			wantNil:      false,
			wantType:     ActivityVerifying,
			wantContains: "syntax",
		},

		// Generic file mentions (fallback)
		{
			name:         "go file mention",
			line:         "Looking at internal/ai/activity.go for patterns",
			wantNil:      false,
			wantType:     ActivityReading,
			wantFile:     "internal/ai/activity.go",
			wantContains: "activity.go",
		},
		{
			name:         "typescript file mention",
			line:         "Modified src/components/App.tsx",
			wantNil:      false,
			wantType:     ActivityWriting,
			wantFile:     "src/components/App.tsx",
			wantContains: "App.tsx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := parser.ParseLine(tt.line)

			if tt.wantNil {
				if result != nil {
					t.Errorf("ParseLine(%q) = %+v, want nil", tt.line, result)
				}
				return
			}

			if result == nil {
				t.Fatalf("ParseLine(%q) = nil, want non-nil", tt.line)
			}

			if result.Type != tt.wantType {
				t.Errorf("ParseLine(%q).Type = %s, want %s", tt.line, result.Type, tt.wantType)
			}

			if tt.wantFile != "" && result.File != tt.wantFile {
				t.Errorf("ParseLine(%q).File = %q, want %q", tt.line, result.File, tt.wantFile)
			}

			if tt.wantContains != "" {
				if result.Message == "" {
					t.Errorf("ParseLine(%q).Message is empty, want to contain %q", tt.line, tt.wantContains)
				}
			}
		})
	}
}

func TestActivityParser_AddPattern(t *testing.T) {
	t.Parallel()

	parser := NewActivityParser()

	// Add a custom pattern
	err := parser.AddPattern(
		`^CUSTOM:\s*(.+)$`,
		ActivityAnalyzing,
		func(m []string) string {
			if len(m) > 1 {
				return "Custom: " + m[1]
			}
			return "Custom"
		},
		nil,
	)
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	// Test custom pattern
	result := parser.ParseLine("CUSTOM: my custom message")
	if result == nil {
		t.Fatal("ParseLine returned nil for custom pattern")
	}

	if result.Type != ActivityAnalyzing {
		t.Errorf("Type = %s, want %s", result.Type, ActivityAnalyzing)
	}

	if result.Message != "Custom: my custom message" {
		t.Errorf("Message = %q, want %q", result.Message, "Custom: my custom message")
	}
}

func TestActivityParser_AddPattern_InvalidRegex(t *testing.T) {
	t.Parallel()

	parser := NewActivityParser()

	err := parser.AddPattern(
		`[invalid(regex`,
		ActivityReading,
		func(_ []string) string { return "test" },
		nil,
	)

	if err == nil {
		t.Error("AddPattern with invalid regex should return error")
	}
}

func TestLooksLikeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected bool
	}{
		{"1.24.0", true},
		{"v2.0.0", true},
		{"1.0", true},
		{"v1.5", true},
		{"internal/ai/runner.go", false},
		{"test.go", false},
		{"1.24.0.go", false}, // Has version-like prefix but file extension
		{"", false},
		{"abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			result := looksLikeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("looksLikeVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestActivityParser_CaseInsensitive(t *testing.T) {
	t.Parallel()

	parser := NewActivityParser()

	cases := []string{
		"reading file.go",
		"Reading file.go",
		"READING file.go",
		"ReAdInG file.go",
	}

	for _, input := range cases {
		result := parser.ParseLine(input)
		if result == nil {
			t.Errorf("ParseLine(%q) = nil, want non-nil", input)
			continue
		}
		if result.Type != ActivityReading {
			t.Errorf("ParseLine(%q).Type = %s, want %s", input, result.Type, ActivityReading)
		}
	}
}

func TestActivityParser_Timestamp(t *testing.T) {
	t.Parallel()

	parser := NewActivityParser()

	result := parser.ParseLine("reading test.go")
	if result == nil {
		t.Fatal("ParseLine returned nil")
	}

	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}
