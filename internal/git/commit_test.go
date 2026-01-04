package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultTrailers(t *testing.T) {
	// DefaultTrailers is deprecated and now always returns an empty map.
	// Commit messages now include an AI-generated synopsis body instead of trailers.
	tests := []struct {
		name         string
		taskID       string
		templateName string
	}{
		{
			name:         "both values returns empty",
			taskID:       "task-abc-xyz",
			templateName: "bugfix",
		},
		{
			name:         "only task ID returns empty",
			taskID:       "task-abc-xyz",
			templateName: "",
		},
		{
			name:         "only template returns empty",
			taskID:       "",
			templateName: "feature",
		},
		{
			name:         "empty values returns empty",
			taskID:       "",
			templateName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trailers := DefaultTrailers(tt.taskID, tt.templateName)
			assert.Empty(t, trailers)
		})
	}
}

func TestInferCommitType(t *testing.T) {
	tests := []struct {
		name     string
		files    []FileChange
		expected CommitType
	}{
		{
			name: "only test files",
			files: []FileChange{
				{Path: "internal/git/runner_test.go", Status: ChangeModified},
				{Path: "internal/config/config_test.go", Status: ChangeAdded},
			},
			expected: CommitTypeTest,
		},
		{
			name: "only docs",
			files: []FileChange{
				{Path: "README.md", Status: ChangeModified},
				{Path: "docs/architecture.md", Status: ChangeAdded},
			},
			expected: CommitTypeDocs,
		},
		{
			name: "only config",
			files: []FileChange{
				{Path: "config.yaml", Status: ChangeModified},
				{Path: ".golangci.yml", Status: ChangeModified},
			},
			expected: CommitTypeChore,
		},
		{
			name: "source files",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "internal/git/types.go", Status: ChangeAdded},
			},
			expected: CommitTypeFeat,
		},
		{
			name: "mixed source and test",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "internal/git/runner_test.go", Status: ChangeModified},
			},
			expected: CommitTypeFeat,
		},
		{
			name: "mixed docs and config",
			files: []FileChange{
				{Path: "README.md", Status: ChangeModified},
				{Path: "config.yaml", Status: ChangeModified},
			},
			expected: CommitTypeDocs, // docs takes priority over config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferCommitType(tt.files)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"runner_test.go", true},
		{"internal/git/runner_test.go", true},
		{"runner.go", false},
		{"test.go", false}, // Not a test file pattern
		{"testing.go", false},
		{"testdata/file.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isTestFile(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDocFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"README.md", true},
		{"CHANGELOG.md", true},
		{"docs/guide.md", true},
		{"docs/api.txt", true},
		{"internal/README.md", true},
		{"main.go", false},
		{"config.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isDocFile(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsConfigFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"config.yaml", true},
		{"config.yml", true},
		{"package.json", true},
		{"settings.toml", true},
		{"app.ini", true},
		{".golangci.yml", true},
		{"main.go", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isConfigFile(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommitType_Values(t *testing.T) {
	// Verify commit type string values for conventional commits
	assert.Equal(t, CommitTypeFeat, CommitType("feat"))
	assert.Equal(t, CommitTypeFix, CommitType("fix"))
	assert.Equal(t, CommitTypeDocs, CommitType("docs"))
	assert.Equal(t, CommitTypeStyle, CommitType("style"))
	assert.Equal(t, CommitTypeRefactor, CommitType("refactor"))
	assert.Equal(t, CommitTypeTest, CommitType("test"))
	assert.Equal(t, CommitTypeChore, CommitType("chore"))
	assert.Equal(t, CommitTypeBuild, CommitType("build"))
	assert.Equal(t, CommitTypeCI, CommitType("ci"))
}

func TestCommitAnalysis_Fields(t *testing.T) {
	analysis := CommitAnalysis{
		FileGroups: []FileGroup{
			{
				Package: "internal/git",
				Files: []FileChange{
					{Path: "internal/git/runner.go", Status: ChangeModified},
				},
				CommitType: CommitTypeFeat,
			},
		},
		GarbageFiles: []GarbageFile{
			{Path: ".env", Category: GarbageSecrets},
		},
		TotalChanges: 2,
		HasGarbage:   true,
	}

	assert.Len(t, analysis.FileGroups, 1)
	assert.Len(t, analysis.GarbageFiles, 1)
	assert.Equal(t, 2, analysis.TotalChanges)
	assert.True(t, analysis.HasGarbage)
}

func TestCommitOptions_Defaults(t *testing.T) {
	// Test zero-value defaults
	opts := CommitOptions{}
	assert.False(t, opts.SingleCommit)
	assert.False(t, opts.SkipGarbageCheck)
	assert.False(t, opts.IncludeGarbage)
	assert.False(t, opts.DryRun)
	assert.Nil(t, opts.Trailers)
}

func TestCommitResult_Fields(t *testing.T) {
	result := CommitResult{
		Commits: []CommitInfo{
			{
				Hash:       "abc1234",
				Message:    "feat(git): add commit service",
				FileCount:  3,
				Package:    "internal/git",
				CommitType: CommitTypeFeat,
			},
		},
		ArtifactPath: "/path/to/commit-message.md",
		TotalFiles:   3,
	}

	assert.Len(t, result.Commits, 1)
	assert.Equal(t, "/path/to/commit-message.md", result.ArtifactPath)
	assert.Equal(t, 3, result.TotalFiles)
}

func TestCommitInfo_Fields(t *testing.T) {
	// Trailers is deprecated and typically empty in new code.
	// Messages now include a synopsis body instead.
	info := CommitInfo{
		Hash:         "def5678",
		Message:      "fix(config): handle nil pointer\n\nFixed null pointer exception in config parser.",
		FileCount:    1,
		Package:      "internal/config",
		CommitType:   CommitTypeFix,
		Trailers:     map[string]string{}, // Deprecated: always empty
		FilesChanged: []string{"internal/config/parser.go"},
	}

	assert.Equal(t, "def5678", info.Hash)
	assert.Contains(t, info.Message, "fix(config): handle nil pointer")
	assert.Contains(t, info.Message, "Fixed null pointer") // Message now includes body
	assert.Equal(t, 1, info.FileCount)
	assert.Equal(t, "internal/config", info.Package)
	assert.Equal(t, CommitTypeFix, info.CommitType)
	assert.Empty(t, info.Trailers) // Trailers are deprecated
	assert.Len(t, info.FilesChanged, 1)
}

func TestFileGroup_Fields(t *testing.T) {
	group := FileGroup{
		Package: "internal/git",
		Files: []FileChange{
			{Path: "internal/git/runner.go", Status: ChangeModified},
			{Path: "internal/git/runner_test.go", Status: ChangeModified},
		},
		SuggestedMessage: "feat(git): add runner implementation",
		CommitType:       CommitTypeFeat,
	}

	assert.Equal(t, "internal/git", group.Package)
	assert.Len(t, group.Files, 2)
	assert.Equal(t, "feat(git): add runner implementation", group.SuggestedMessage)
	assert.Equal(t, CommitTypeFeat, group.CommitType)
}

func TestCommitArtifact_Fields(t *testing.T) {
	artifact := CommitArtifact{
		TaskID:    "task-abc",
		Template:  "feature",
		Timestamp: "2025-12-30T10:00:00Z",
		Summary:   "Created 2 commits for internal/git changes",
		Commits: []CommitInfo{
			{Hash: "abc1234", Message: "feat(git): add commit service"},
		},
	}

	assert.Equal(t, "task-abc", artifact.TaskID)
	assert.Equal(t, "feature", artifact.Template)
	assert.Equal(t, "2025-12-30T10:00:00Z", artifact.Timestamp)
	assert.Contains(t, artifact.Summary, "2 commits")
	assert.Len(t, artifact.Commits, 1)
}

func TestGroupFilesByPackage(t *testing.T) {
	tests := []struct {
		name           string
		files          []FileChange
		expectedGroups int
		expectedPkgs   []string
	}{
		{
			name: "single package",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "internal/git/types.go", Status: ChangeAdded},
			},
			expectedGroups: 1,
			expectedPkgs:   []string{"internal/git"},
		},
		{
			name: "multiple packages",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "internal/config/config.go", Status: ChangeModified},
			},
			expectedGroups: 2,
			expectedPkgs:   []string{"internal/config", "internal/git"},
		},
		{
			name: "source and test together",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "internal/git/runner_test.go", Status: ChangeModified},
			},
			expectedGroups: 1,
			expectedPkgs:   []string{"internal/git"},
		},
		{
			name: "docs grouped separately",
			files: []FileChange{
				{Path: "internal/git/runner.go", Status: ChangeModified},
				{Path: "README.md", Status: ChangeModified},
				{Path: "docs/guide.md", Status: ChangeAdded},
			},
			expectedGroups: 2,
			expectedPkgs:   []string{"internal/git", "docs"},
		},
		{
			name: "root config files",
			files: []FileChange{
				{Path: "config.yaml", Status: ChangeModified},
				{Path: ".golangci.yml", Status: ChangeModified},
			},
			expectedGroups: 1,
			expectedPkgs:   []string{"config"},
		},
		{
			name: "cmd package",
			files: []FileChange{
				{Path: "cmd/atlas/main.go", Status: ChangeModified},
				{Path: "cmd/atlas/version.go", Status: ChangeAdded},
			},
			expectedGroups: 1,
			expectedPkgs:   []string{"cmd/atlas"},
		},
		{
			name:           "empty input",
			files:          []FileChange{},
			expectedGroups: 0,
			expectedPkgs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := GroupFilesByPackage(tt.files)
			assert.Len(t, groups, tt.expectedGroups)

			for i, g := range groups {
				if i < len(tt.expectedPkgs) {
					assert.Equal(t, tt.expectedPkgs[i], g.Package)
				}
			}
		})
	}
}

func TestGroupFilesByPackage_Ordering(t *testing.T) {
	// Test that groups are sorted correctly
	files := []FileChange{
		{Path: "docs/guide.md", Status: ChangeModified},
		{Path: "cmd/atlas/main.go", Status: ChangeModified},
		{Path: "internal/git/runner.go", Status: ChangeModified},
		{Path: "internal/config/config.go", Status: ChangeModified},
		{Path: "README.md", Status: ChangeModified},
	}

	groups := GroupFilesByPackage(files)

	// Expected order: internal packages first (alphabetically), then cmd, then docs
	expectedOrder := []string{"internal/config", "internal/git", "cmd/atlas", "docs"}
	assert.Len(t, groups, len(expectedOrder))

	for i, expected := range expectedOrder {
		assert.Equal(t, expected, groups[i].Package, "wrong package at position %d", i)
	}
}

func TestGetGroupKey(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/git/runner.go", "internal/git"},
		{"internal/config/parser/types.go", "internal/config"},
		{"cmd/atlas/main.go", "cmd/atlas"},
		{"README.md", "docs"},
		{"docs/architecture.md", "docs"},
		{"config.yaml", "config"},
		{".golangci.yml", "config"},
		{"main.go", "root"},
		{"Makefile", "root"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := getGroupKey(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDocPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"docs/guide.md", true},
		{"docs/api.txt", true},
		{"README.md", true},
		{"CHANGELOG.md", true},
		{"internal/README.md", false}, // Not root level
		{"internal/git/runner.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isDocPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGroupFilesForSingleCommit(t *testing.T) {
	files := []FileChange{
		{Path: "internal/git/runner.go", Status: ChangeModified},
		{Path: "internal/config/config.go", Status: ChangeModified},
		{Path: "README.md", Status: ChangeModified},
	}

	groups := GroupFilesForSingleCommit(files)
	assert.Len(t, groups, 1)
	assert.Equal(t, "all", groups[0].Package)
	assert.Len(t, groups[0].Files, 3)
}

func TestGroupFilesForSingleCommit_Empty(t *testing.T) {
	groups := GroupFilesForSingleCommit([]FileChange{})
	assert.Nil(t, groups)
}

func TestMergeSmallGroups(t *testing.T) {
	groups := []FileGroup{
		{Package: "internal/git", Files: []FileChange{{Path: "a.go"}}},
		{Package: "internal/config", Files: []FileChange{{Path: "b.go"}}},
		{Package: "internal/task", Files: []FileChange{{Path: "c.go"}, {Path: "d.go"}, {Path: "e.go"}}},
	}

	// Merge groups with fewer than 2 files
	merged := MergeSmallGroups(groups, 2)

	// The first two small groups should be merged, the third stays
	assert.Len(t, merged, 2)
	assert.Equal(t, "internal/git+internal/config", merged[0].Package)
	assert.Len(t, merged[0].Files, 2)
	assert.Equal(t, "internal/task", merged[1].Package)
	assert.Len(t, merged[1].Files, 3)
}

func TestMergeSmallGroups_NoMerge(t *testing.T) {
	groups := []FileGroup{
		{Package: "internal/git", Files: []FileChange{{Path: "a.go"}, {Path: "b.go"}}},
		{Package: "internal/config", Files: []FileChange{{Path: "c.go"}, {Path: "d.go"}}},
	}

	// All groups are large enough
	merged := MergeSmallGroups(groups, 2)
	assert.Len(t, merged, 2)
}

func TestMergeSmallGroups_EdgeCases(t *testing.T) {
	// Single group - no merging
	single := []FileGroup{
		{Package: "pkg", Files: []FileChange{{Path: "a.go"}}},
	}
	assert.Len(t, MergeSmallGroups(single, 2), 1)

	// Empty input
	assert.Empty(t, MergeSmallGroups([]FileGroup{}, 2))

	// Zero minFiles - no merging
	groups := []FileGroup{
		{Package: "a", Files: []FileChange{{Path: "a.go"}}},
		{Package: "b", Files: []FileChange{{Path: "b.go"}}},
	}
	assert.Len(t, MergeSmallGroups(groups, 0), 2)
}

func TestGetFilePaths(t *testing.T) {
	files := []FileChange{
		{Path: "internal/git/runner.go", Status: ChangeModified},
		{Path: "internal/git/types.go", Status: ChangeAdded},
	}

	paths := GetFilePaths(files)
	assert.Equal(t, []string{"internal/git/runner.go", "internal/git/types.go"}, paths)
}

func TestGetFilePaths_Empty(t *testing.T) {
	paths := GetFilePaths([]FileChange{})
	assert.Empty(t, paths)
}

func TestGetScopeFromPackage(t *testing.T) {
	tests := []struct {
		pkg      string
		expected string
	}{
		{"internal/git", "git"},
		{"internal/config", "config"},
		{"cmd/atlas", "atlas"},
		{"internal/task/engine", "engine"},
		{"docs", ""},
		{"root", ""},
		{"config", ""},
		{"all", ""},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			result := GetScopeFromPackage(tt.pkg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
