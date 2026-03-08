package prompts

import (
	"strings"
	"testing"
)

// TestQualityPrompts verifies all 7 quality prompt templates render correctly with nil data.
// Templates are static — Claude Code CLI reads files autonomously, so no input data is needed.
func TestQualityPrompts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		promptID PromptID
		contains string
	}{
		{
			name:     "dedup contains domain keyword",
			promptID: Deduplication,
			contains: "duplication",
		},
		{
			name:     "goroutine leak contains domain keyword",
			promptID: GoroutineLeak,
			contains: "goroutine",
		},
		{
			name:     "jr to sr contains domain keyword",
			promptID: JrToSr,
			contains: "junior",
		},
		{
			name:     "constant hunter contains domain keyword",
			promptID: ConstantHunter,
			contains: "magic",
		},
		{
			name:     "config hunter contains configuration keyword",
			promptID: ConfigHunter,
			contains: "configuration",
		},
		{
			name:     "config hunter contains os.Getenv reference",
			promptID: ConfigHunter,
			contains: "os.Getenv",
		},
		{
			name:     "go optimizer contains workspace artifact path",
			promptID: GoOptimize,
			contains: "workspace/",
		},
		{
			name:     "test creator contains domain keyword",
			promptID: TestCreator,
			contains: "test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := Render(tc.promptID, nil)
			if err != nil {
				t.Errorf("Render(%s, nil) error = %v", tc.promptID, err)
				return
			}
			if result == "" {
				t.Errorf("Render(%s, nil) returned empty string", tc.promptID)
				return
			}
			if tc.contains != "" && !strings.Contains(result, tc.contains) {
				t.Errorf("Render(%s, nil) missing %q\nGot:\n%s", tc.promptID, tc.contains, result)
			}
		})
	}
}

// TestQualityPromptsExistAndList tests that all quality prompt IDs are registered.
func TestQualityPromptsExistAndList(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			if !Exists(id) {
				t.Errorf("Exists(%s) = false, want true", id)
			}
		})
	}

	ids := List()
	for _, exp := range qualityPrompts {
		found := false
		for _, id := range ids {
			if id == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing quality prompt ID %s", exp)
		}
	}
}

// TestQualityPromptsValidateData tests that ValidateData accepts QualityAnalysisData for quality prompts.
// Quality prompts accept QualityAnalysisData for backward-compatible API usage even though
// the templates themselves are now static and render correctly with nil.
func TestQualityPromptsValidateData(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			if err := ValidateData(id, QualityAnalysisData{}); err != nil {
				t.Errorf("ValidateData(%s, QualityAnalysisData{}) = %v, want nil", id, err)
			}
		})
	}
}

// TestQualityPromptsRenderWithNilData confirms all quality templates render successfully
// with nil data and produce the workspace artifact path that the fix step depends on.
func TestQualityPromptsRenderWithNilData(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()

			result, err := Render(id, nil)
			if err != nil {
				t.Errorf("Render(%s, nil) error = %v", id, err)
				return
			}
			if result == "" {
				t.Errorf("Render(%s, nil) returned empty string", id)
				return
			}
			// Every quality template must reference its workspace artifact so the fix step
			// knows where to find the analysis plan.
			if !strings.Contains(result, "workspace/") {
				t.Errorf("Render(%s, nil) missing workspace artifact path", id)
			}
		})
	}
}

// TestQualityPromptsStructure verifies all quality templates contain the
// required 7-section structure: EVIDENCE RULES, BEFORE WRITING, Findings
// output format, numbered detection rules, and workspace artifact path.
func TestQualityPromptsStructure(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	requiredSections := []struct {
		label   string
		content string
	}{
		{"EVIDENCE RULES section", "# EVIDENCE RULES"},
		{"BEFORE WRITING section", "# BEFORE WRITING"},
		{"Findings output format", "## Findings"},
		{"Numbered detection rule", "1. **"},
		{"ROLE section", "# ROLE"},
		{"SCOPE section", "# SCOPE"},
		{"DETECTION RULES section", "# DETECTION RULES"},
		{"DO NOT FLAG section", "# DO NOT FLAG"},
		{"OUTPUT section", "# OUTPUT"},
		{"workspace artifact path", "workspace/"},
		{"Scope exclusion for vendor", "vendor/"},
		{"Scope exclusion for generated code", "Code generated"},
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()

			result, err := Render(id, nil)
			if err != nil {
				t.Fatalf("Render(%s, nil) error = %v", id, err)
			}

			for _, sec := range requiredSections {
				if !strings.Contains(result, sec.content) {
					t.Errorf("Render(%s, nil) missing %s (expected %q)", id, sec.label, sec.content)
				}
			}
		})
	}
}

// TestQualityPromptsEvidenceRules verifies the standardized anti-hallucination
// guardrails are present in every quality template.
func TestQualityPromptsEvidenceRules(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	evidenceKeyPhrases := []string{
		"directly verified by reading",
		"Zero false positives",
		"No issues found",
		"verbatim",
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()

			result, err := Render(id, nil)
			if err != nil {
				t.Fatalf("Render(%s, nil) error = %v", id, err)
			}

			for _, phrase := range evidenceKeyPhrases {
				if !strings.Contains(result, phrase) {
					t.Errorf("Render(%s, nil) EVIDENCE RULES missing key phrase %q", id, phrase)
				}
			}
		})
	}
}

// TestQualityPromptsVerificationChecklist verifies the standardized self-verification
// checklist is present in every quality template.
func TestQualityPromptsVerificationChecklist(t *testing.T) {
	t.Parallel()

	qualityPrompts := []PromptID{
		Deduplication,
		GoroutineLeak,
		JrToSr,
		ConstantHunter,
		ConfigHunter,
		GoOptimize,
		TestCreator,
	}

	checklistItems := []string{
		"File path exists",
		"Line numbers match",
		"Original snippet is verbatim",
		"Finding matches a numbered detection rule",
	}

	for _, id := range qualityPrompts {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()

			result, err := Render(id, nil)
			if err != nil {
				t.Fatalf("Render(%s, nil) error = %v", id, err)
			}

			for _, item := range checklistItems {
				if !strings.Contains(result, item) {
					t.Errorf("Render(%s, nil) BEFORE WRITING missing checklist item %q", id, item)
				}
			}
		})
	}
}
