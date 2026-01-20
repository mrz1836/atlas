package prompts

import (
	"errors"
	"strings"
	"testing"
	"text/template"
)

// TestRenderCommitMessage tests the commit message prompt rendering.
func TestRenderCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		data     CommitMessageData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic commit message",
			data: CommitMessageData{
				Package: "internal/git",
				Files: []FileChange{
					{Path: "internal/git/commit.go", Status: "modified"},
					{Path: "internal/git/commit_test.go", Status: "modified"},
				},
				Scope: "git",
			},
			wantErr: false,
			contains: []string{
				"Package: internal/git",
				"internal/git/commit.go (modified)",
				"internal/git/commit_test.go (modified)",
				"conventional commits format",
				"Scope should be: git",
			},
		},
		{
			name: "commit message with diff summary",
			data: CommitMessageData{
				Package: "internal/prompts",
				Files: []FileChange{
					{Path: "internal/prompts/types.go", Status: "added"},
				},
				DiffSummary: "Added new types for prompt data structures",
				Scope:       "prompts",
			},
			wantErr: false,
			contains: []string{
				"Package: internal/prompts",
				"Change summary:",
				"Added new types for prompt data structures",
			},
		},
		{
			name: "commit message empty files",
			data: CommitMessageData{
				Package: "internal/test",
				Files:   []FileChange{},
				Scope:   "test",
			},
			wantErr:  false,
			contains: []string{"Package: internal/test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(CommitMessage, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderPRDescription tests the PR description prompt rendering.
func TestRenderPRDescription(t *testing.T) {
	tests := []struct {
		name     string
		data     PRDescriptionData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic PR description",
			data: PRDescriptionData{
				TaskDescription: "Add user authentication feature",
				CommitMessages:  []string{"feat(auth): add login endpoint", "feat(auth): add logout endpoint"},
				FilesChanged: []PRFileChange{
					{Path: "internal/auth/login.go", Insertions: 50, Deletions: 0},
					{Path: "internal/auth/logout.go", Insertions: 30, Deletions: 0},
				},
				TemplateName:  "feature",
				TaskID:        "task-123",
				WorkspaceName: "auth-feature",
			},
			wantErr: false,
			contains: []string{
				"Add user authentication feature",
				"feat(auth): add login endpoint",
				"feat(auth): add logout endpoint",
				"internal/auth/login.go (+50, -0)",
				"internal/auth/logout.go (+30, -0)",
				"feature",
				"task-123",
				"auth-feature",
			},
		},
		{
			name: "PR description with no task description",
			data: PRDescriptionData{
				CommitMessages: []string{"fix(bug): resolve null pointer"},
				TemplateName:   "bugfix",
			},
			wantErr: false,
			contains: []string{
				"(Not provided)",
				"fix(bug): resolve null pointer",
			},
		},
		{
			name: "PR description with validation results",
			data: PRDescriptionData{
				TaskDescription:   "Fix database connection",
				ValidationResults: "All tests passed\nLint: OK",
				TemplateName:      "fix",
			},
			wantErr: false,
			contains: []string{
				"All tests passed",
				"Lint: OK",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(PRDescription, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderValidationRetry tests the validation retry prompt rendering.
func TestRenderValidationRetry(t *testing.T) {
	tests := []struct {
		name     string
		data     ValidationRetryData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic validation retry",
			data: ValidationRetryData{
				FailedStep:     "lint",
				FailedCommands: []string{"golangci-lint run", "go vet ./..."},
				ErrorOutput:    "main.go:10: unused variable 'x'",
				AttemptNumber:  1,
				MaxAttempts:    3,
			},
			wantErr: false,
			contains: []string{
				"at step: lint",
				"golangci-lint run, go vet ./...",
				"unused variable 'x'",
				"Attempt 1 of 3",
			},
		},
		{
			name: "validation retry without step name",
			data: ValidationRetryData{
				FailedCommands: []string{"go test ./..."},
				ErrorOutput:    "FAIL: TestSomething",
				AttemptNumber:  2,
				MaxAttempts:    3,
			},
			wantErr: false,
			contains: []string{
				"Previous validation failed",
				"go test ./...",
				"FAIL: TestSomething",
				"Attempt 2 of 3",
			},
		},
		{
			name: "validation retry no attempt info",
			data: ValidationRetryData{
				FailedStep:  "test",
				ErrorOutput: "Test failed",
			},
			wantErr:  false,
			contains: []string{"at step: test", "Test failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(ValidationRetry, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderDiscoveryAnalysis tests the discovery analysis prompt rendering.
func TestRenderDiscoveryAnalysis(t *testing.T) {
	tests := []struct {
		name     string
		data     DiscoveryAnalysisData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic discovery analysis",
			data: DiscoveryAnalysisData{
				Title:       "Fix null pointer in auth module",
				Category:    "bug",
				Severity:    "high",
				Description: "Auth module crashes when user is nil",
				File:        "internal/auth/handler.go",
				Line:        42,
				Tags:        []string{"auth", "crash", "urgent"},
			},
			wantErr: false,
			contains: []string{
				"Title: Fix null pointer in auth module",
				"Category: bug",
				"Severity: high",
				"Auth module crashes when user is nil",
				"Location: internal/auth/handler.go:42",
				"auth, crash, urgent",
			},
		},
		{
			name: "discovery with git context",
			data: DiscoveryAnalysisData{
				Title:     "Add caching layer",
				Category:  "enhancement",
				Severity:  "medium",
				GitBranch: "feature/caching",
				GitCommit: "abc1234",
			},
			wantErr: false,
			contains: []string{
				"Title: Add caching layer",
				"Found on branch: feature/caching",
				"Commit: abc1234",
			},
		},
		{
			name: "discovery with available agents",
			data: DiscoveryAnalysisData{
				Title:           "Refactor API",
				Category:        "refactor",
				Severity:        "low",
				AvailableAgents: []string{"claude", "gemini"},
			},
			wantErr: false,
			contains: []string{
				"Title: Refactor API",
				"Claude",
				"Gemini",
				"claude, gemini",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(DiscoveryAnalysis, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderQuickVerify tests the quick verify prompt rendering.
func TestRenderQuickVerify(t *testing.T) {
	tests := []struct {
		name     string
		data     QuickVerifyData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic quick verify",
			data: QuickVerifyData{
				TaskDescription: "Add user validation",
				Checks:          []string{"1. Does the code address the task?", "2. Any obvious bugs?"},
			},
			wantErr: false,
			contains: []string{
				"Add user validation",
				"1. Does the code address the task?",
				"2. Any obvious bugs?",
				"Respond with JSON",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(QuickVerify, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderCodeCorrectness tests the code correctness prompt rendering.
func TestRenderCodeCorrectness(t *testing.T) {
	tests := []struct {
		name     string
		data     CodeCorrectnessData
		wantErr  bool
		contains []string
	}{
		{
			name: "basic code correctness",
			data: CodeCorrectnessData{
				TaskDescription: "Add error handling",
				ChangedFiles: []ChangedFileInfo{
					{
						Path:     "internal/handler/api.go",
						Language: "go",
						Content:  "func Handler() error { return nil }",
					},
				},
			},
			wantErr: false,
			contains: []string{
				"Add error handling",
				"internal/handler/api.go",
				"```go",
				"func Handler() error",
			},
		},
		{
			name: "code correctness without language",
			data: CodeCorrectnessData{
				TaskDescription: "Update config",
				ChangedFiles: []ChangedFileInfo{
					{
						Path:    "config.yaml",
						Content: "key: value",
					},
				},
			},
			wantErr: false,
			contains: []string{
				"config.yaml",
				"key: value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(CodeCorrectness, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render() output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderNotFound tests rendering a non-existent template.
func TestRenderNotFound(t *testing.T) {
	_, err := Render("nonexistent/template", nil)
	if err == nil {
		t.Error("Render() expected error for non-existent template")
	}
	if !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("Render() error = %v, want ErrTemplateNotFound", err)
	}
}

// TestMustRenderPanic tests that MustRender panics on error.
func TestMustRenderPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRender() did not panic for non-existent template")
		}
	}()

	MustRender("nonexistent/template", nil)
}

// TestMustRenderSuccess tests that MustRender returns correct result on success.
func TestMustRenderSuccess(t *testing.T) {
	data := QuickVerifyData{
		TaskDescription: "Test task",
		Checks:          []string{"Check 1"},
	}

	result := MustRender(QuickVerify, data)
	if !strings.Contains(result, "Test task") {
		t.Error("MustRender() did not return expected content")
	}
}

// TestList tests listing all prompt IDs.
func TestList(t *testing.T) {
	ids := List()

	if len(ids) == 0 {
		t.Error("List() returned empty list")
	}

	// Check that known IDs are in the list
	expected := []PromptID{CommitMessage, PRDescription, ValidationRetry, DiscoveryAnalysis, QuickVerify, CodeCorrectness}
	for _, exp := range expected {
		found := false
		for _, id := range ids {
			if id == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing expected ID %s", exp)
		}
	}
}

// TestExists tests checking if a prompt exists.
func TestExists(t *testing.T) {
	tests := []struct {
		id     PromptID
		exists bool
	}{
		{CommitMessage, true},
		{PRDescription, true},
		{ValidationRetry, true},
		{DiscoveryAnalysis, true},
		{QuickVerify, true},
		{CodeCorrectness, true},
		{"nonexistent", false},
		{"also/nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			if got := Exists(tt.id); got != tt.exists {
				t.Errorf("Exists(%s) = %v, want %v", tt.id, got, tt.exists)
			}
		})
	}
}

// TestValidateData tests data validation for each prompt type.
func TestValidateData(t *testing.T) {
	tests := []struct {
		name    string
		id      PromptID
		data    any
		wantErr bool
	}{
		{"valid CommitMessage data", CommitMessage, CommitMessageData{}, false},
		{"invalid CommitMessage data", CommitMessage, "string", true},
		{"valid PRDescription data", PRDescription, PRDescriptionData{}, false},
		{"invalid PRDescription data", PRDescription, 123, true},
		{"valid ValidationRetry data", ValidationRetry, ValidationRetryData{}, false},
		{"invalid ValidationRetry data", ValidationRetry, map[string]string{}, true},
		{"valid DiscoveryAnalysis data", DiscoveryAnalysis, DiscoveryAnalysisData{}, false},
		{"invalid DiscoveryAnalysis data", DiscoveryAnalysis, []byte{}, true},
		{"valid QuickVerify data", QuickVerify, QuickVerifyData{}, false},
		{"invalid QuickVerify data", QuickVerify, nil, true},
		{"valid CodeCorrectness data", CodeCorrectness, CodeCorrectnessData{}, false},
		{"invalid CodeCorrectness data", CodeCorrectness, struct{}{}, true},
		{"unknown prompt ID", "unknown", nil, false}, // unknown IDs don't validate type
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateData(tt.id, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidData) {
				t.Errorf("ValidateData() error = %v, want ErrInvalidData", err)
			}
		})
	}
}

// TestGetTemplate tests retrieving raw template content.
func TestGetTemplate(t *testing.T) {
	// Test existing template
	content, err := GetTemplate(CommitMessage)
	if err != nil {
		t.Errorf("GetTemplate() error = %v", err)
	}
	if content == "" {
		t.Error("GetTemplate() returned empty content")
	}

	// Test non-existent template
	_, err = GetTemplate("nonexistent")
	if err == nil {
		t.Error("GetTemplate() expected error for non-existent template")
	}
}

// TestRenderStringMap tests rendering with a string map.
func TestRenderStringMap(t *testing.T) {
	// This is primarily a convenience wrapper, test basic functionality
	_, err := RenderString(CommitMessage, map[string]string{})
	// This will likely fail because CommitMessageData expects specific fields
	// but it shouldn't panic - it's testing the wrapper works
	if err != nil {
		// Expected - wrong data type
		t.Log("RenderString with wrong type failed as expected:", err)
	}
}

// TestCommonTemplateInclusion tests that common templates are properly included.
func TestCommonTemplateInclusion(t *testing.T) {
	data := CommitMessageData{
		Package: "test",
		Files:   []FileChange{{Path: "test.go", Status: "modified"}},
	}

	result, err := Render(CommitMessage, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check that common template content is included
	if !strings.Contains(result, "conventional commits format") {
		t.Error("Render() missing common template content 'conventional commits format'")
	}
	if !strings.Contains(result, "Do NOT include any AI attribution") {
		t.Error("Render() missing common template content 'Do NOT include any AI attribution'")
	}
}

// TestTemplateFunctionHelpers tests the template helper functions.
func TestTemplateFunctionHelpers(t *testing.T) {
	// Test with data that exercises template functions
	data := CommitMessageData{
		Package:     "test-package",
		Files:       []FileChange{{Path: "file1.go", Status: "added"}, {Path: "file2.go", Status: "deleted"}},
		DiffSummary: "  whitespace only  ",
		Scope:       "test",
	}

	result, err := Render(CommitMessage, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check formatFileChange worked
	if !strings.Contains(result, "- file1.go (added)") {
		t.Error("formatFileChange function not working correctly")
	}
	if !strings.Contains(result, "- file2.go (deleted)") {
		t.Error("formatFileChange function not working correctly for second file")
	}

	// Check hasContent worked (diff summary has content despite whitespace)
	if !strings.Contains(result, "Change summary:") {
		t.Error("hasContent function not detecting content correctly")
	}
}

// TestEmptyDataStructures tests rendering with empty data structures.
func TestEmptyDataStructures(t *testing.T) {
	tests := []struct {
		name string
		id   PromptID
		data any
	}{
		{"empty CommitMessageData", CommitMessage, CommitMessageData{}},
		{"empty PRDescriptionData", PRDescription, PRDescriptionData{}},
		{"empty ValidationRetryData", ValidationRetry, ValidationRetryData{}},
		{"empty DiscoveryAnalysisData", DiscoveryAnalysis, DiscoveryAnalysisData{}},
		{"empty QuickVerifyData", QuickVerify, QuickVerifyData{}},
		{"empty CodeCorrectnessData", CodeCorrectness, CodeCorrectnessData{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Render(tt.id, tt.data)
			if err != nil {
				t.Errorf("Render() with empty data error = %v", err)
			}
			if result == "" {
				t.Error("Render() with empty data returned empty string")
			}
		})
	}
}

// BenchmarkRender benchmarks template rendering.
func BenchmarkRender(b *testing.B) {
	data := CommitMessageData{
		Package:     "internal/prompts",
		Files:       []FileChange{{Path: "prompts.go", Status: "modified"}, {Path: "types.go", Status: "added"}},
		DiffSummary: "Added prompt management system",
		Scope:       "prompts",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Render(CommitMessage, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkList benchmarks listing prompt IDs.
func BenchmarkList(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = List()
	}
}

// TestRegisterCustomFuncs tests registering custom template functions.
func TestRegisterCustomFuncs(t *testing.T) {
	// Register a custom function
	RegisterCustomFuncs(template.FuncMap{
		"testCustomFunc": func(s string) string {
			return "custom:" + s
		},
	})

	// Verify templates still work after registration
	data := CommitMessageData{
		Package: "test",
		Scope:   "test",
	}

	result, err := Render(CommitMessage, data)
	if err != nil {
		t.Errorf("Render() after RegisterCustomFuncs error = %v", err)
	}
	if result == "" {
		t.Error("Render() after RegisterCustomFuncs returned empty string")
	}

	// Verify the registry has been updated (templates are reloaded)
	ids := List()
	if len(ids) == 0 {
		t.Error("List() returned empty after RegisterCustomFuncs")
	}
}

// TestRegisterCustomFuncsMultipleTimes tests multiple registrations don't cause issues.
func TestRegisterCustomFuncsMultipleTimes(t *testing.T) {
	// Register multiple times in sequence (tests for deadlock would timeout)
	for i := 0; i < 3; i++ {
		RegisterCustomFuncs(template.FuncMap{
			"multiFunc": func(s string) string {
				return s
			},
		})
	}

	// Verify templates still work
	data := QuickVerifyData{
		TaskDescription: "test",
		Checks:          []string{"check1"},
	}

	result, err := Render(QuickVerify, data)
	if err != nil {
		t.Errorf("Render() after multiple RegisterCustomFuncs error = %v", err)
	}
	if result == "" {
		t.Error("Render() after multiple RegisterCustomFuncs returned empty string")
	}
}

// TestConcurrentAccess tests thread-safe access to the registry.
//
//nolint:gocognit // complex test intentionally tests multiple concurrent scenarios
func TestConcurrentAccess(t *testing.T) {
	const goroutines = 10
	const iterations = 100

	done := make(chan bool, goroutines*3)

	// Concurrent reads
	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < iterations; i++ {
				_, err := Render(CommitMessage, CommitMessageData{
					Package: "test",
					Scope:   "test",
				})
				if err != nil {
					t.Errorf("concurrent Render() error = %v", err)
				}
			}
			done <- true
		}()
	}

	// Concurrent List()
	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < iterations; i++ {
				ids := List()
				if len(ids) == 0 {
					t.Error("concurrent List() returned empty")
				}
			}
			done <- true
		}()
	}

	// Concurrent Exists()
	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < iterations; i++ {
				if !Exists(CommitMessage) {
					t.Error("concurrent Exists() returned false for existing template")
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines*3; i++ {
		<-done
	}
}

// TestRenderCIFailure tests the CI failure prompt rendering.
func TestRenderCIFailure(t *testing.T) {
	tests := []struct {
		name     string
		data     CIFailureData
		wantErr  bool
		contains []string
	}{
		{
			name: "with failed checks",
			data: CIFailureData{
				HasFailures: true,
				FailedChecks: []CICheckInfo{
					{Name: "Build", Status: "fail", URL: "https://ci.example.com/build/123"},
					{Name: "Test", Status: "fail"},
				},
			},
			wantErr: false,
			contains: []string{
				"CI Failure Analysis",
				"Build",
				"fail",
				"https://ci.example.com/build/123",
				"Test",
				"Please analyze the failures",
			},
		},
		{
			name: "no specific failures",
			data: CIFailureData{
				HasFailures:  false,
				FailedChecks: nil,
			},
			wantErr: false,
			contains: []string{
				"No specific failures identified",
				"overall CI status indicates failure",
			},
		},
		{
			name:     "empty data",
			data:     CIFailureData{},
			wantErr:  false,
			contains: []string{"No specific failures identified"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(CIFailure, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render(CIFailure) error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render(CIFailure) output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestRenderAutoFix tests the auto-fix prompt rendering.
func TestRenderAutoFix(t *testing.T) {
	tests := []struct {
		name     string
		data     AutoFixData
		wantErr  bool
		contains []string
	}{
		{
			name: "with all severity levels",
			data: AutoFixData{
				TaskDesc:    "Implement user authentication",
				TotalIssues: 4,
				ErrorIssues: []AutoFixIssue{
					{File: "auth.go", Line: 42, Message: "undefined variable", Suggestion: "declare the variable"},
				},
				WarningIssues: []AutoFixIssue{
					{File: "handler.go", Line: 15, Message: "unused import"},
				},
				InfoIssues: []AutoFixIssue{
					{File: "main.go", Line: 1, Message: "consider adding comment"},
					{File: "config.go", Line: 5, Message: "magic number"},
				},
			},
			wantErr: false,
			contains: []string{
				"Implement user authentication",
				"Issues Found: 4",
				"Error Issues (1)",
				"auth.go",
				"line 42",
				"undefined variable",
				"declare the variable",
				"Warning Issues (1)",
				"handler.go",
				"Info Issues (2)",
				"Fix each issue",
				"minimal changes",
			},
		},
		{
			name: "errors only",
			data: AutoFixData{
				TaskDesc:    "Fix bug",
				TotalIssues: 1,
				ErrorIssues: []AutoFixIssue{
					{File: "bug.go", Line: 10, Message: "null pointer"},
				},
			},
			wantErr: false,
			contains: []string{
				"Fix bug",
				"Error Issues (1)",
				"bug.go",
			},
		},
		{
			name:     "empty data",
			data:     AutoFixData{},
			wantErr:  false,
			contains: []string{"Issues Found: 0", "Instructions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(AutoFix, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render(AutoFix) error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Render(AutoFix) output missing %q\nGot:\n%s", want, got)
				}
			}
		})
	}
}

// TestValidateDataNewTypes tests data validation for new prompt types.
func TestValidateDataNewTypes(t *testing.T) {
	tests := []struct {
		name    string
		id      PromptID
		data    any
		wantErr bool
	}{
		{"valid CIFailure data", CIFailure, CIFailureData{}, false},
		{"invalid CIFailure data", CIFailure, "string", true},
		{"valid AutoFix data", AutoFix, AutoFixData{}, false},
		{"invalid AutoFix data", AutoFix, 123, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateData(tt.id, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidData) {
				t.Errorf("ValidateData() error = %v, want ErrInvalidData", err)
			}
		})
	}
}

// TestNilSlicesInData tests templates handle nil slices gracefully.
func TestNilSlicesInData(t *testing.T) {
	tests := []struct {
		name string
		id   PromptID
		data any
	}{
		{
			name: "CIFailure with nil FailedChecks",
			id:   CIFailure,
			data: CIFailureData{
				HasFailures:  true,
				FailedChecks: nil, // nil slice
			},
		},
		{
			name: "AutoFix with nil issue slices",
			id:   AutoFix,
			data: AutoFixData{
				TaskDesc:      "test",
				TotalIssues:   0,
				ErrorIssues:   nil,
				WarningIssues: nil,
				InfoIssues:    nil,
			},
		},
		{
			name: "CommitMessage with nil files",
			id:   CommitMessage,
			data: CommitMessageData{
				Package: "test",
				Files:   nil,
				Scope:   "test",
			},
		},
		{
			name: "PRDescription with nil slices",
			id:   PRDescription,
			data: PRDescriptionData{
				CommitMessages: nil,
				FilesChanged:   nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Render(tt.id, tt.data)
			if err != nil {
				t.Errorf("Render() with nil slices error = %v", err)
			}
			if result == "" {
				t.Error("Render() with nil slices returned empty string")
			}
		})
	}
}

// TestGetTemplateReturnsSource tests that GetTemplate returns the actual template source.
func TestGetTemplateReturnsSource(t *testing.T) {
	source, err := GetTemplate(CommitMessage)
	if err != nil {
		t.Errorf("GetTemplate() error = %v", err)
		return
	}

	// Verify it contains template syntax
	if !strings.Contains(source, "{{") {
		t.Error("GetTemplate() source missing template syntax")
	}

	// Verify it contains expected content markers
	if !strings.Contains(source, "Package") {
		t.Error("GetTemplate() source missing expected content")
	}
}

// TestListIncludesNewPrompts tests that List() includes the new prompt IDs.
func TestListIncludesNewPrompts(t *testing.T) {
	ids := List()

	expected := []PromptID{CIFailure, AutoFix}
	for _, exp := range expected {
		found := false
		for _, id := range ids {
			if id == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing expected new ID %s", exp)
		}
	}
}

// TestExistsNewPrompts tests that Exists() returns true for new prompts.
func TestExistsNewPrompts(t *testing.T) {
	newPrompts := []PromptID{CIFailure, AutoFix}
	for _, id := range newPrompts {
		if !Exists(id) {
			t.Errorf("Exists(%s) = false, want true", id)
		}
	}
}
