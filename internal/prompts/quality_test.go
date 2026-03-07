package prompts

import (
	"strings"
	"testing"
)

func sampleQualityData() QualityAnalysisData {
	return QualityAnalysisData{
		Files: []SourceFile{
			{
				Path:     "pkg/example/main.go",
				Language: "go",
				Content:  "package main\n\nfunc main() {}\n",
			},
		},
		GoVersion:      "1.24",
		ProjectContext: "Example project for testing",
	}
}

// TestQualityPrompts tests all 7 quality prompt templates.
func TestQualityPrompts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		promptID PromptID
		data     QualityAnalysisData
		wantErr  bool
		contains string
	}{
		{
			name:     "dedup with sample files",
			promptID: Deduplication,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "duplication",
		},
		{
			name:     "goroutine leak with sample files",
			promptID: GoroutineLeak,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "goroutine",
		},
		{
			name:     "jr to sr with sample files",
			promptID: JrToSr,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "junior",
		},
		{
			name:     "constant hunter with sample files",
			promptID: ConstantHunter,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "magic",
		},
		{
			name:     "config hunter with project context",
			promptID: ConfigHunter,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "configuration",
		},
		{
			name:     "config hunter without project context",
			promptID: ConfigHunter,
			data: QualityAnalysisData{
				Files:          sampleQualityData().Files,
				ProjectContext: "",
			},
			wantErr:  false,
			contains: "os.Getenv",
		},
		{
			name:     "go optimizer interpolates version",
			promptID: GoOptimize,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "1.24",
		},
		{
			name:     "test creator with sample files",
			promptID: TestCreator,
			data:     sampleQualityData(),
			wantErr:  false,
			contains: "test",
		},
		{
			name:     "dedup with empty file list",
			promptID: Deduplication,
			data: QualityAnalysisData{
				Files:     []SourceFile{},
				GoVersion: "1.24",
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := Render(tc.promptID, tc.data)
			if (err != nil) != tc.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				if result == "" {
					t.Error("expected non-empty result")
				}
				if tc.contains != "" && !strings.Contains(result, tc.contains) {
					t.Errorf("expected output to contain %q, got:\n%s", tc.contains, result)
				}
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
// Quality prompts are not yet in the ValidateData type-switch, so any data type is accepted;
// this test verifies the expected zero-value input returns no error.
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

// TestQualityPromptsEmptyData tests that quality prompts handle empty data gracefully.
func TestQualityPromptsEmptyData(t *testing.T) {
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
			result, err := Render(id, QualityAnalysisData{})
			if err != nil {
				t.Errorf("Render(%s, empty) error = %v", id, err)
			}
			if result == "" {
				t.Errorf("Render(%s, empty) returned empty string", id)
			}
		})
	}
}
