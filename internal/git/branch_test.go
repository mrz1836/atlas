package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase",
			input:    "myfeature",
			expected: "myfeature",
		},
		{
			name:     "uppercase converted",
			input:    "MyFeature",
			expected: "myfeature",
		},
		{
			name:     "spaces replaced with hyphens",
			input:    "my feature",
			expected: "my-feature",
		},
		{
			name:     "special characters replaced",
			input:    "My Feature!",
			expected: "my-feature",
		},
		{
			name:     "multiple special characters",
			input:    "My@#$Feature!!!",
			expected: "my-feature",
		},
		{
			name:     "leading trailing hyphens trimmed",
			input:    "---my-feature---",
			expected: "my-feature",
		},
		{
			name:     "consecutive hyphens collapsed",
			input:    "my---feature",
			expected: "my-feature",
		},
		{
			name:     "underscores replaced",
			input:    "my_feature_name",
			expected: "my-feature-name",
		},
		{
			name:     "mixed case and special chars",
			input:    "User Auth v2.0!",
			expected: "user-auth-v2-0",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "!!!@@@###",
			expected: "",
		},
		{
			name:     "numbers preserved",
			input:    "feature123",
			expected: "feature123",
		},
		{
			name:     "hyphens preserved",
			input:    "my-existing-feature",
			expected: "my-existing-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeBranchName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name          string
		branchType    string
		workspaceName string
		expected      string
	}{
		{
			name:          "simple feature branch",
			branchType:    "feat",
			workspaceName: "auth",
			expected:      "feat/auth",
		},
		{
			name:          "fix branch with spaces",
			branchType:    "fix",
			workspaceName: "user auth bug",
			expected:      "fix/user-auth-bug",
		},
		{
			name:          "chore branch with special chars",
			branchType:    "chore",
			workspaceName: "Update CI/CD!",
			expected:      "chore/update-ci-cd",
		},
		{
			name:          "empty workspace name",
			branchType:    "feat",
			workspaceName: "",
			expected:      "feat/unnamed",
		},
		{
			name:          "workspace name only special chars",
			branchType:    "fix",
			workspaceName: "!!!",
			expected:      "fix/unnamed",
		},
		{
			name:          "uppercase branch type preserved",
			branchType:    "FEAT",
			workspaceName: "test",
			expected:      "FEAT/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateBranchName(tt.branchType, tt.workspaceName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveBranchPrefix(t *testing.T) {
	tests := []struct {
		name         string
		templateType string
		expected     string
	}{
		{
			name:         "bugfix maps to fix",
			templateType: "bugfix",
			expected:     "fix",
		},
		{
			name:         "feature maps to feat",
			templateType: "feature",
			expected:     "feat",
		},
		{
			name:         "commit maps to chore",
			templateType: "commit",
			expected:     "chore",
		},
		{
			name:         "fix stays fix",
			templateType: "fix",
			expected:     "fix",
		},
		{
			name:         "feat stays feat",
			templateType: "feat",
			expected:     "feat",
		},
		{
			name:         "chore stays chore",
			templateType: "chore",
			expected:     "chore",
		},
		{
			name:         "uppercase normalized",
			templateType: "BUGFIX",
			expected:     "fix",
		},
		{
			name:         "mixed case normalized",
			templateType: "Feature",
			expected:     "feat",
		},
		{
			name:         "unknown type returned as-is lowercase",
			templateType: "Custom",
			expected:     "custom",
		},
		{
			name:         "empty type returned as-is",
			templateType: "",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveBranchPrefix(tt.templateType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultBranchPrefixes(t *testing.T) {
	// Verify expected mappings exist
	assert.Equal(t, "fix", DefaultBranchPrefixes["bugfix"])
	assert.Equal(t, "feat", DefaultBranchPrefixes["feature"])
	assert.Equal(t, "chore", DefaultBranchPrefixes["commit"])
	assert.Equal(t, "fix", DefaultBranchPrefixes["fix"])
	assert.Equal(t, "feat", DefaultBranchPrefixes["feat"])
	assert.Equal(t, "chore", DefaultBranchPrefixes["chore"])
}

func TestResolveBranchPrefixWithConfig(t *testing.T) {
	tests := []struct {
		name           string
		templateType   string
		customPrefixes map[string]string
		expected       string
	}{
		{
			name:           "nil config uses defaults",
			templateType:   "bugfix",
			customPrefixes: nil,
			expected:       "fix",
		},
		{
			name:           "empty config uses defaults",
			templateType:   "feature",
			customPrefixes: map[string]string{},
			expected:       "feat",
		},
		{
			name:         "custom prefix overrides default",
			templateType: "bugfix",
			customPrefixes: map[string]string{
				"bugfix": "hotfix",
			},
			expected: "hotfix",
		},
		{
			name:         "custom prefix for new type",
			templateType: "refactor",
			customPrefixes: map[string]string{
				"refactor": "refactor",
			},
			expected: "refactor",
		},
		{
			name:         "falls back to defaults for unmatched type",
			templateType: "commit",
			customPrefixes: map[string]string{
				"bugfix": "hotfix",
			},
			expected: "chore",
		},
		{
			name:         "case insensitive custom lookup",
			templateType: "BUGFIX",
			customPrefixes: map[string]string{
				"bugfix": "hotfix",
			},
			expected: "hotfix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveBranchPrefixWithConfig(tt.templateType, tt.customPrefixes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBranchCreator_GenerateUniqueBranchName(t *testing.T) {
	// Create temp directory for test repos
	tempDir := t.TempDir()

	t.Run("returns base name when branch does not exist", func(t *testing.T) {
		// Setup a git repo
		repoDir := filepath.Join(tempDir, "repo-unique-base")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		// Initialize git repo
		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		// Create initial commit
		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		// Test
		name, err := creator.GenerateUniqueBranchName(context.Background(), "feat/new-feature")
		require.NoError(t, err)
		assert.Equal(t, "feat/new-feature", name)
	})

	t.Run("appends timestamp when branch exists", func(t *testing.T) {
		// Setup a git repo
		repoDir := filepath.Join(tempDir, "repo-unique-exists")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		// Initialize git repo with initial commit
		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		// Create the branch that will cause collision
		_, err = RunCommand(context.Background(), repoDir, "branch", "feat/existing")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		// Test
		name, err := creator.GenerateUniqueBranchName(context.Background(), "feat/existing")
		require.NoError(t, err)

		// Should have timestamp suffix
		assert.Contains(t, name, "feat/existing-")
		assert.Regexp(t, `feat/existing-\d{8}-\d{6}`, name)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-unique-cancel")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = creator.GenerateUniqueBranchName(ctx, "feat/test")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestBranchCreator_Create(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("creates branch with correct naming", func(t *testing.T) {
		// Setup git repo
		repoDir := filepath.Join(tempDir, "repo-create-branch")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		// Create branch (empty BaseBranch creates from HEAD)
		info, err := creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "User Auth Feature",
			BranchType:    "feature",
			BaseBranch:    "", // Empty = create from current HEAD
		})

		require.NoError(t, err)
		assert.Equal(t, "feat/user-auth-feature", info.Name)
		assert.Empty(t, info.BaseBranch)
		assert.WithinDuration(t, time.Now(), info.CreatedAt, time.Second)

		// Verify branch exists
		exists, err := runner.BranchExists(context.Background(), "feat/user-auth-feature")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("creates bugfix branch with fix prefix", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-create-bugfix")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		info, err := creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "null-pointer-bug",
			BranchType:    "bugfix",
		})

		require.NoError(t, err)
		assert.Equal(t, "fix/null-pointer-bug", info.Name)
	})

	t.Run("creates chore branch for commit template", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-create-chore")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		info, err := creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "update-deps",
			BranchType:    "commit",
		})

		require.NoError(t, err)
		assert.Equal(t, "chore/update-deps", info.Name)
	})

	t.Run("error with empty workspace name", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-empty-workspace")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		_, err = creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "",
			BranchType:    "feat",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace name cannot be empty")
	})

	t.Run("error with empty branch type", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-empty-branchtype")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		_, err = creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "my-feature",
			BranchType:    "",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "branch type cannot be empty")
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-cancel")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		creator := NewBranchCreator(runner)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = creator.Create(ctx, BranchCreateOptions{
			WorkspaceName: "test",
			BranchType:    "feat",
		})

		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestBranchInfo(t *testing.T) {
	// Test that BranchInfo struct works correctly
	now := time.Now()
	info := BranchInfo{
		Name:       "feat/test",
		BaseBranch: "main",
		CreatedAt:  now,
	}

	assert.Equal(t, "feat/test", info.Name)
	assert.Equal(t, "main", info.BaseBranch)
	assert.Equal(t, now, info.CreatedAt)
}

func TestBranchConfig(t *testing.T) {
	// Test that BranchConfig struct works correctly
	config := BranchConfig{
		Type:       "feat",
		BaseBranch: "main",
		Pattern:    "{type}/{name}",
	}

	assert.Equal(t, "feat", config.Type)
	assert.Equal(t, "main", config.BaseBranch)
	assert.Equal(t, "{type}/{name}", config.Pattern)
}

func TestBranchCreateOptions(t *testing.T) {
	// Test that BranchCreateOptions struct works correctly
	opts := BranchCreateOptions{
		WorkspaceName: "auth-feature",
		BranchType:    "feature",
		BaseBranch:    "develop",
	}

	assert.Equal(t, "auth-feature", opts.WorkspaceName)
	assert.Equal(t, "feature", opts.BranchType)
	assert.Equal(t, "develop", opts.BaseBranch)
}

func TestBranchCreator_CreateWithCustomConfig(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("uses custom prefix from config", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-custom-config")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		// Create with custom prefix configuration
		customPrefixes := map[string]string{
			"bugfix": "hotfix",
		}
		creator := NewBranchCreatorWithConfig(runner, customPrefixes)

		info, err := creator.Create(context.Background(), BranchCreateOptions{
			WorkspaceName: "critical-issue",
			BranchType:    "bugfix",
		})

		require.NoError(t, err)
		// Should use custom "hotfix" prefix instead of default "fix"
		assert.Equal(t, "hotfix/critical-issue", info.Name)
	})
}

func TestNewBranchCreatorWithConfig(t *testing.T) {
	// Mock runner for testing
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo-test-config")
	err := os.MkdirAll(repoDir, 0o750)
	require.NoError(t, err)

	_, err = RunCommand(context.Background(), repoDir, "init")
	require.NoError(t, err)

	runner, err := NewRunner(context.Background(), repoDir)
	require.NoError(t, err)

	customPrefixes := map[string]string{
		"bugfix": "hotfix",
		"docs":   "docs",
	}

	creator := NewBranchCreatorWithConfig(runner, customPrefixes)
	require.NotNil(t, creator)
	assert.Equal(t, customPrefixes, creator.customPrefixes)
}

func TestCLIRunner_BranchExists(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("returns true for existing branch", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-exists-true")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		// Create test branch
		_, err = RunCommand(context.Background(), repoDir, "branch", "test-branch")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "test-branch")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for non-existing branch", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-exists-false")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.email", "test@example.com")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "config", "user.name", "Test User")
		require.NoError(t, err)

		testFile := filepath.Join(repoDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o600)
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "add", ".")
		require.NoError(t, err)
		_, err = RunCommand(context.Background(), repoDir, "commit", "-m", "initial commit")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		exists, err := runner.BranchExists(context.Background(), "nonexistent-branch")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("context cancellation", func(t *testing.T) {
		repoDir := filepath.Join(tempDir, "repo-exists-cancel")
		err := os.MkdirAll(repoDir, 0o750)
		require.NoError(t, err)

		_, err = RunCommand(context.Background(), repoDir, "init")
		require.NoError(t, err)

		runner, err := NewRunner(context.Background(), repoDir)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = runner.BranchExists(ctx, "test-branch")
		assert.ErrorIs(t, err, context.Canceled)
	})
}
