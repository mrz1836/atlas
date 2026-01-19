// Package git provides Git operations for ATLAS.
// This file implements the SmartCommitService for intelligent commit management.
package git

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
)

// SmartCommitService defines operations for intelligent commit management.
// It analyzes changes, groups files logically, generates commit messages,
// and creates commits with ATLAS trailers.
type SmartCommitService interface {
	// Analyze inspects the current worktree and returns a commit analysis.
	// This includes file groupings, detected garbage, and total changes.
	Analyze(ctx context.Context) (*CommitAnalysis, error)

	// Commit creates one or more commits based on the analysis.
	// If SingleCommit is true in options, all changes are committed together.
	// Otherwise, changes are committed in logical groups.
	Commit(ctx context.Context, opts CommitOptions) (*CommitResult, error)
}

// CommitAnalysis contains the result of analyzing changes in the worktree.
type CommitAnalysis struct {
	FileGroups   []FileGroup   // Files grouped by logical unit (package/directory)
	GarbageFiles []GarbageFile // Detected garbage files that shouldn't be committed
	TotalChanges int           // Total number of changed files
	HasGarbage   bool          // True if any garbage was detected
}

// FileGroup represents a logical grouping of related files.
type FileGroup struct {
	Package          string       // Package or directory name (e.g., "internal/git")
	Files            []FileChange // Files in this group
	SuggestedMessage string       // AI-generated or inferred commit message
	CommitType       CommitType   // Inferred commit type (feat, fix, etc.)
}

// CommitType represents the type of change for conventional commits.
type CommitType string

// Commit type constants for conventional commits format.
const (
	CommitTypeFeat     CommitType = "feat"
	CommitTypeFix      CommitType = "fix"
	CommitTypeDocs     CommitType = "docs"
	CommitTypeStyle    CommitType = "style"
	CommitTypeRefactor CommitType = "refactor"
	CommitTypeTest     CommitType = "test"
	CommitTypeChore    CommitType = "chore"
	CommitTypeBuild    CommitType = "build"
	CommitTypeCI       CommitType = "ci"
)

// ValidCommitTypes contains all valid conventional commit types.
// Use this for validation instead of hardcoding the type strings.
//
//nolint:gochecknoglobals // Read-only list of valid commit types
var ValidCommitTypes = []CommitType{
	CommitTypeFeat,
	CommitTypeFix,
	CommitTypeDocs,
	CommitTypeStyle,
	CommitTypeRefactor,
	CommitTypeTest,
	CommitTypeChore,
	CommitTypeBuild,
	CommitTypeCI,
}

// CommitOptions controls how commits are created.
type CommitOptions struct {
	SingleCommit     bool // If true, create one commit for all changes
	SkipGarbageCheck bool // If true, skip garbage detection
	IncludeGarbage   bool // If true, include garbage files anyway
	DryRun           bool // If true, don't actually create commits
}

// CommitResult contains the result of creating commits.
type CommitResult struct {
	Commits      []CommitInfo // Information about each commit created
	ArtifactPath string       // Path to saved commit-message.md artifact
	TotalFiles   int          // Total files committed across all commits
}

// CommitInfo contains information about a single commit.
type CommitInfo struct {
	Hash         string     // Git commit hash (short form)
	Message      string     // Full commit message (subject + synopsis body)
	FileCount    int        // Number of files in this commit
	Package      string     // Package/directory this commit covers
	CommitType   CommitType // Type of commit (feat, fix, etc.)
	FilesChanged []string   // List of file paths that were committed
}

// CommitArtifact represents the saved artifact for commit messages.
type CommitArtifact struct {
	TaskID    string       // ATLAS task ID if available
	Template  string       // Template name if available
	Commits   []CommitInfo // All commits created
	Timestamp string       // When commits were created
	Summary   string       // Human-readable summary
}

// InferCommitType infers the commit type from file changes.
func InferCommitType(files []FileChange) CommitType {
	hasTest := false
	hasDocs := false
	hasSource := false
	hasConfig := false

	for _, f := range files {
		path := f.Path

		// Check for test files
		if isTestFile(path) {
			hasTest = true
			continue
		}

		// Check for documentation
		if isDocFile(path) {
			hasDocs = true
			continue
		}

		// Check for config files
		if isConfigFile(path) {
			hasConfig = true
			continue
		}

		// Everything else is source
		hasSource = true
	}

	// Priority: if only tests, it's a test commit
	if hasTest && !hasSource && !hasDocs {
		return CommitTypeTest
	}

	// If only docs, it's a docs commit
	if hasDocs && !hasSource && !hasTest {
		return CommitTypeDocs
	}

	// If only config, it's a chore
	if hasConfig && !hasSource && !hasTest && !hasDocs {
		return CommitTypeChore
	}

	// Default to feat for source changes
	// (The AI will refine this based on actual diff content)
	return CommitTypeFeat
}

// isTestFile checks if a file is a test file.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

// isDocFile checks if a file is documentation.
func isDocFile(path string) bool {
	if strings.HasSuffix(path, ".md") {
		return true
	}
	if strings.HasSuffix(path, ".txt") {
		return true
	}
	// Check for docs directory
	if strings.HasPrefix(path, "docs/") {
		return true
	}
	return false
}

// isConfigFile checks if a file is a configuration file.
func isConfigFile(path string) bool {
	configExtensions := []string{".yaml", ".yml", ".json", ".toml", ".ini"}
	for _, ext := range configExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// GroupFilesByPackage groups files by their logical package or directory.
// Files are grouped by directory, with special handling for:
// - Source and test files stay together (parser.go + parser_test.go)
// - Documentation files are grouped separately (docs/, *.md)
// - Renamed files are grouped with their destination directory
func GroupFilesByPackage(files []FileChange) []FileGroup {
	groups := make(map[string]*FileGroup)

	for _, f := range files {
		// Normalize path separators
		path := filepath.ToSlash(f.Path)

		// Determine the group key (package/directory)
		groupKey := getGroupKey(path)

		// Create or update group
		if g, ok := groups[groupKey]; ok {
			g.Files = append(g.Files, f)
		} else {
			groups[groupKey] = &FileGroup{
				Package: groupKey,
				Files:   []FileChange{f},
			}
		}
	}

	// Convert map to sorted slice
	result := make([]FileGroup, 0, len(groups))
	for _, g := range groups {
		// Set commit type based on files
		g.CommitType = InferCommitType(g.Files)
		result = append(result, *g)
	}

	// Sort groups for deterministic ordering
	sortGroups(result)

	return result
}

// getGroupKey determines which group a file belongs to.
func getGroupKey(path string) string {
	// Special handling for documentation
	if isDocPath(path) {
		return "docs"
	}

	// Special handling for root-level files
	dir := filepath.Dir(path)
	if dir == "." {
		// Root files get grouped by type
		if isConfigFile(path) {
			return "config"
		}
		return "root"
	}

	// For nested paths, use the first two levels for internal packages
	// e.g., internal/git/runner.go -> internal/git
	parts := strings.Split(dir, "/")
	if len(parts) >= 2 && parts[0] == "internal" {
		return strings.Join(parts[:2], "/")
	}

	// For cmd packages
	if len(parts) >= 2 && parts[0] == "cmd" {
		return strings.Join(parts[:2], "/")
	}

	// Otherwise use the full directory
	return dir
}

// isDocPath checks if a path should be grouped as documentation.
func isDocPath(path string) bool {
	// Files in docs/ directory
	if strings.HasPrefix(path, "docs/") {
		return true
	}

	// Root markdown files
	dir := filepath.Dir(path)
	if dir == "." && strings.HasSuffix(path, ".md") {
		return true
	}

	return false
}

// sortGroups sorts file groups for deterministic ordering.
// Priority: internal packages first, then cmd, then others, then docs.
func sortGroups(groups []FileGroup) {
	sort.Slice(groups, func(i, j int) bool {
		pi := getGroupPriority(groups[i].Package)
		pj := getGroupPriority(groups[j].Package)
		if pi != pj {
			return pi < pj
		}
		return groups[i].Package < groups[j].Package
	})
}

// getGroupPriority returns the sort priority for a group.
func getGroupPriority(pkg string) int {
	switch {
	case strings.HasPrefix(pkg, "internal/"):
		return 1
	case strings.HasPrefix(pkg, "cmd/"):
		return 2
	case pkg == "docs":
		return 5
	case pkg == "config":
		return 4
	case pkg == "root":
		return 3
	default:
		return 3
	}
}

// GroupFilesForSingleCommit returns a single group containing all files.
func GroupFilesForSingleCommit(files []FileChange) []FileGroup {
	if len(files) == 0 {
		return nil
	}

	return []FileGroup{
		{
			Package:    "all",
			Files:      files,
			CommitType: InferCommitType(files),
		},
	}
}

// MergeSmallGroups merges groups with fewer than minFiles into adjacent groups.
// This prevents creating too many tiny commits.
func MergeSmallGroups(groups []FileGroup, minFiles int) []FileGroup {
	if len(groups) <= 1 || minFiles <= 0 {
		return groups
	}

	result := make([]FileGroup, 0, len(groups))
	var pending *FileGroup

	for _, g := range groups {
		if len(g.Files) >= minFiles {
			result = flushPending(result, pending)
			pending = nil
			result = append(result, g)
			continue
		}

		// Small group - merge with pending
		pending = mergeIntoPending(pending, g)
	}

	// Flush remaining pending
	return flushPending(result, pending)
}

// flushPending adds a pending group to results if it exists.
func flushPending(result []FileGroup, pending *FileGroup) []FileGroup {
	if pending != nil {
		pending.CommitType = InferCommitType(pending.Files)
		result = append(result, *pending)
	}
	return result
}

// mergeIntoPending merges a group into the pending group.
func mergeIntoPending(pending *FileGroup, g FileGroup) *FileGroup {
	if pending == nil {
		return &FileGroup{
			Package: g.Package,
			Files:   append([]FileChange{}, g.Files...),
		}
	}
	pending.Files = append(pending.Files, g.Files...)
	pending.Package = pending.Package + "+" + g.Package
	return pending
}

// GetFilePaths extracts file paths from a list of FileChange.
func GetFilePaths(files []FileChange) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}

// GetScopeFromPackage extracts a short scope name from a package path.
// e.g., "internal/git" -> "git", "cmd/atlas" -> "atlas"
func GetScopeFromPackage(pkg string) string {
	if pkg == "docs" {
		return ""
	}
	if pkg == "root" || pkg == "config" || pkg == "all" {
		return ""
	}

	// Get the last component
	parts := strings.Split(pkg, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkg
}
