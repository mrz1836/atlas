package git

import (
	"testing"
)

func TestStats_FormatCompact(t *testing.T) {
	tests := []struct {
		name     string
		stats    *Stats
		expected string
	}{
		{
			name:     "nil stats",
			stats:    nil,
			expected: "",
		},
		{
			name:     "empty stats",
			stats:    &Stats{},
			expected: "",
		},
		{
			name: "only new files",
			stats: &Stats{
				NewFiles: 3,
			},
			expected: "📄 3",
		},
		{
			name: "only modified files",
			stats: &Stats{
				ModifiedFiles: 5,
			},
			expected: "✏️ 5",
		},
		{
			name: "only deleted files",
			stats: &Stats{
				DeletedFiles: 2,
			},
			expected: "🗑️ 2",
		},
		{
			name: "both new and modified files",
			stats: &Stats{
				NewFiles:      2,
				ModifiedFiles: 3,
			},
			expected: "📄 2  ✏️ 3",
		},
		{
			name: "new modified and deleted files",
			stats: &Stats{
				NewFiles:      1,
				ModifiedFiles: 2,
				DeletedFiles:  3,
			},
			expected: "📄 1  ✏️ 2  🗑️ 3",
		},
		{
			name: "only line counts",
			stats: &Stats{
				Additions: 100,
				Deletions: 50,
			},
			expected: "+100/-50",
		},
		{
			name: "files and line counts",
			stats: &Stats{
				ModifiedFiles: 3,
				Additions:     120,
				Deletions:     45,
			},
			expected: "✏️ 3  +120/-45",
		},
		{
			name: "full stats",
			stats: &Stats{
				NewFiles:      1,
				ModifiedFiles: 2,
				Additions:     200,
				Deletions:     100,
			},
			expected: "📄 1  ✏️ 2  +200/-100",
		},
		{
			name: "full stats with deleted",
			stats: &Stats{
				NewFiles:      1,
				ModifiedFiles: 2,
				DeletedFiles:  3,
				Additions:     200,
				Deletions:     100,
			},
			expected: "📄 1  ✏️ 2  🗑️ 3  +200/-100",
		},
		{
			name: "zero deletions",
			stats: &Stats{
				ModifiedFiles: 1,
				Additions:     50,
				Deletions:     0,
			},
			expected: "✏️ 1  +50/-0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.stats.FormatCompact()
			if result != tt.expected {
				t.Errorf("FormatCompact() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStats_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		stats    *Stats
		expected bool
	}{
		{
			name:     "nil stats",
			stats:    nil,
			expected: true,
		},
		{
			name:     "empty stats",
			stats:    &Stats{},
			expected: true,
		},
		{
			name: "has new files",
			stats: &Stats{
				NewFiles: 1,
			},
			expected: false,
		},
		{
			name: "has modified files",
			stats: &Stats{
				ModifiedFiles: 1,
			},
			expected: false,
		},
		{
			name: "has deleted files",
			stats: &Stats{
				DeletedFiles: 1,
			},
			expected: false,
		},
		{
			name: "has additions",
			stats: &Stats{
				Additions: 10,
			},
			expected: false,
		},
		{
			name: "has deletions",
			stats: &Stats{
				Deletions: 5,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.stats.IsEmpty()
			if result != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseStatusForCounts(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		expectedNew      int
		expectedModified int
		expectedDeleted  int
	}{
		{
			name:             "empty output",
			output:           "",
			expectedNew:      0,
			expectedModified: 0,
			expectedDeleted:  0,
		},
		{
			name:             "untracked files",
			output:           "?? file1.txt\n?? file2.txt\n?? dir/file3.txt",
			expectedNew:      3,
			expectedModified: 0,
			expectedDeleted:  0,
		},
		{
			name:             "staged new file",
			output:           "A  newfile.go",
			expectedNew:      1,
			expectedModified: 0,
			expectedDeleted:  0,
		},
		{
			name:             "modified files",
			output:           " M file1.go\nM  file2.go\nMM file3.go",
			expectedNew:      0,
			expectedModified: 3,
			expectedDeleted:  0,
		},
		{
			name:             "renamed file",
			output:           "R  old.go -> new.go",
			expectedNew:      0,
			expectedModified: 1,
			expectedDeleted:  0,
		},
		{
			name:             "deleted file unstaged",
			output:           " D deleted.go",
			expectedNew:      0,
			expectedModified: 0,
			expectedDeleted:  1,
		},
		{
			name:             "deleted file staged",
			output:           "D  deleted.go",
			expectedNew:      0,
			expectedModified: 0,
			expectedDeleted:  1,
		},
		{
			name:             "multiple deleted files",
			output:           "D  file1.go\n D file2.go\nD  file3.go",
			expectedNew:      0,
			expectedModified: 0,
			expectedDeleted:  3,
		},
		{
			name:             "mixed status",
			output:           "## main...origin/main\n?? untracked.txt\nA  staged_new.go\n M modified.go\nD  deleted.go",
			expectedNew:      2,
			expectedModified: 1,
			expectedDeleted:  1,
		},
		{
			name:             "all file types",
			output:           "?? new.txt\nA  added.go\n M modified.go\nD  deleted.go\nR  old.go -> renamed.go",
			expectedNew:      2,
			expectedModified: 2,
			expectedDeleted:  1,
		},
		{
			name:             "branch line only",
			output:           "## main...origin/main [ahead 1]",
			expectedNew:      0,
			expectedModified: 0,
			expectedDeleted:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newFiles, modifiedFiles, deletedFiles := parseStatusForCounts(tt.output)
			if newFiles != tt.expectedNew {
				t.Errorf("parseStatusForCounts() newFiles = %d, want %d", newFiles, tt.expectedNew)
			}
			if modifiedFiles != tt.expectedModified {
				t.Errorf("parseStatusForCounts() modifiedFiles = %d, want %d", modifiedFiles, tt.expectedModified)
			}
			if deletedFiles != tt.expectedDeleted {
				t.Errorf("parseStatusForCounts() deletedFiles = %d, want %d", deletedFiles, tt.expectedDeleted)
			}
		})
	}
}

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectedAdd int
		expectedDel int
	}{
		{
			name:        "empty output",
			output:      "",
			expectedAdd: 0,
			expectedDel: 0,
		},
		{
			name:        "single file",
			output:      "10\t5\tfile.go",
			expectedAdd: 10,
			expectedDel: 5,
		},
		{
			name:        "multiple files",
			output:      "10\t5\tfile1.go\n20\t10\tfile2.go\n5\t0\tfile3.go",
			expectedAdd: 35,
			expectedDel: 15,
		},
		{
			name:        "binary file",
			output:      "-\t-\timage.png",
			expectedAdd: 0,
			expectedDel: 0,
		},
		{
			name:        "mixed text and binary",
			output:      "10\t5\tfile.go\n-\t-\timage.png\n20\t10\tother.go",
			expectedAdd: 30,
			expectedDel: 15,
		},
		{
			name:        "with empty lines",
			output:      "10\t5\tfile1.go\n\n20\t10\tfile2.go\n",
			expectedAdd: 30,
			expectedDel: 15,
		},
		{
			name:        "zero additions",
			output:      "0\t50\tfile.go",
			expectedAdd: 0,
			expectedDel: 50,
		},
		{
			name:        "zero deletions",
			output:      "100\t0\tnewfile.go",
			expectedAdd: 100,
			expectedDel: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			additions, deletions := parseNumstat(tt.output)
			if additions != tt.expectedAdd {
				t.Errorf("parseNumstat() additions = %d, want %d", additions, tt.expectedAdd)
			}
			if deletions != tt.expectedDel {
				t.Errorf("parseNumstat() deletions = %d, want %d", deletions, tt.expectedDel)
			}
		})
	}
}

func TestNewStatsProvider(t *testing.T) {
	provider := NewStatsProvider("/tmp/test")

	if provider == nil {
		t.Fatal("NewStatsProvider() returned nil")
	}
	if provider.workDir != "/tmp/test" {
		t.Errorf("workDir = %q, want %q", provider.workDir, "/tmp/test")
	}
	if provider.debounce != 500*1000000 { // 500ms in nanoseconds
		t.Errorf("debounce = %v, want 500ms", provider.debounce)
	}
	if provider.cached != nil {
		t.Error("cached should be nil initially")
	}
}

func TestStatsProvider_GetCachedStats_NilInitially(t *testing.T) {
	provider := NewStatsProvider("/tmp/test")
	stats := provider.GetCachedStats()

	if stats != nil {
		t.Errorf("GetCachedStats() = %+v, want nil initially", stats)
	}
}
