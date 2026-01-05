package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/mrz1836/atlas/internal/config"
)

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.CommandContext(context.Background(), "git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to init git repo")

	// Configure git user for commits
	cmd = exec.CommandContext(context.Background(), "git", "config", "user.email", "test@atlas.local")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to configure git email")

	cmd = exec.CommandContext(context.Background(), "git", "config", "user.name", "ATLAS Test")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "failed to configure git name")

	return tmpDir
}

// setupMainRepoWithWorktree creates a main git repo with a linked worktree.
// Returns paths to both main repo and worktree.
func setupMainRepoWithWorktree(t *testing.T, worktreeName string) (mainPath, worktreePath string) {
	t.Helper()

	// Create main repo
	mainPath = setupTestRepo(t)

	// Create initial commit (required for worktree)
	initialFile := filepath.Join(mainPath, "README.md")
	err := os.WriteFile(initialFile, []byte("# Test Repo"), 0o600)
	require.NoError(t, err, "failed to create initial file")

	cmd := exec.CommandContext(context.Background(), "git", "add", "README.md")
	cmd.Dir = mainPath
	require.NoError(t, cmd.Run(), "failed to add initial file")

	cmd = exec.CommandContext(context.Background(), "git", "commit", "-m", "initial commit")
	cmd.Dir = mainPath
	require.NoError(t, cmd.Run(), "failed to create initial commit")

	// Create worktree in temp directory
	worktreeParent := t.TempDir()
	worktreePath = filepath.Join(worktreeParent, worktreeName)

	cmd = exec.CommandContext(context.Background(), "git", "worktree", "add", worktreePath)
	cmd.Dir = mainPath
	require.NoError(t, cmd.Run(), "failed to create worktree")

	return mainPath, worktreePath
}

// createAtlasConfig creates .atlas/config.yaml in the specified repo path.
func createAtlasConfig(t *testing.T, repoPath string, cfg *config.Config) {
	t.Helper()

	atlasDir := filepath.Join(repoPath, ".atlas")
	err := os.MkdirAll(atlasDir, 0o750)
	require.NoError(t, err, "failed to create .atlas directory")

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err, "failed to marshal config to YAML")

	cfgPath := filepath.Join(atlasDir, "config.yaml")
	err = os.WriteFile(cfgPath, data, 0o600)
	require.NoError(t, err, "failed to write config file")
}

func TestResolveExecutionContext(t *testing.T) {
	t.Run("resolves context for main repo without worktree flag", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		// Resolve symlinks (macOS /var -> /private/var)
		resolvedPath, err := filepath.EvalSymlinks(repoPath)
		require.NoError(t, err)

		// Create a config file
		cfg := config.DefaultConfig()
		cfg.Git.BaseBranch = "develop"
		createAtlasConfig(t, repoPath, cfg)

		// Change to repo directory
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(repoPath)
		require.NoError(t, err)

		// Resolve context
		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "")

		require.NoError(t, err)
		assert.Equal(t, resolvedPath, ec.WorkDir, "WorkDir should be repo root")
		assert.Equal(t, resolvedPath, ec.MainRepoPath, "MainRepoPath should be repo root")
		assert.False(t, ec.IsWorktree, "IsWorktree should be false")
		assert.NotNil(t, ec.Config, "Config should be loaded")
		assert.Equal(t, "develop", ec.Config.Git.BaseBranch, "Config should be loaded correctly")
	})

	t.Run("resolves context with valid worktree name", func(t *testing.T) {
		mainPath, worktreePath := setupMainRepoWithWorktree(t, "feature-branch")

		// Resolve symlinks
		resolvedMain, err := filepath.EvalSymlinks(mainPath)
		require.NoError(t, err)
		resolvedWorktree, err := filepath.EvalSymlinks(worktreePath)
		require.NoError(t, err)

		// Create config in main repo
		mainCfg := config.DefaultConfig()
		mainCfg.Git.BaseBranch = "main"
		createAtlasConfig(t, mainPath, mainCfg)

		// Create config in worktree with override
		worktreeCfg := config.DefaultConfig()
		worktreeCfg.Git.BaseBranch = "feature-base"
		createAtlasConfig(t, worktreePath, worktreeCfg)

		// Change to main repo directory
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(mainPath)
		require.NoError(t, err)

		// Resolve context with worktree name
		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "feature-branch")

		require.NoError(t, err)
		assert.Equal(t, resolvedWorktree, ec.WorkDir, "WorkDir should be worktree path")
		assert.Equal(t, resolvedMain, ec.MainRepoPath, "MainRepoPath should be main repo")
		assert.True(t, ec.IsWorktree, "IsWorktree should be true")
		assert.NotNil(t, ec.Config, "Config should be loaded")
		// Worktree config should take precedence
		assert.Equal(t, "feature-base", ec.Config.Git.BaseBranch, "Worktree config should override")
	})

	t.Run("resolves context when current directory is worktree", func(t *testing.T) {
		mainPath, worktreePath := setupMainRepoWithWorktree(t, "my-worktree")

		// Resolve symlinks
		resolvedMain, err := filepath.EvalSymlinks(mainPath)
		require.NoError(t, err)
		resolvedWorktree, err := filepath.EvalSymlinks(worktreePath)
		require.NoError(t, err)

		// Create config
		cfg := config.DefaultConfig()
		createAtlasConfig(t, mainPath, cfg)

		// Change to worktree directory
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(worktreePath)
		require.NoError(t, err)

		// Resolve without worktree flag
		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "")

		require.NoError(t, err)
		assert.Equal(t, resolvedWorktree, ec.WorkDir, "WorkDir should be current worktree")
		assert.Equal(t, resolvedMain, ec.MainRepoPath, "MainRepoPath should be main repo")
		assert.True(t, ec.IsWorktree, "IsWorktree should be true")
	})

	t.Run("handles worktree name with unicode characters", func(t *testing.T) {
		mainPath, worktreePath := setupMainRepoWithWorktree(t, "功能-branch") //nolint:gosmopolitan // testing unicode support

		// Resolve symlinks
		resolvedWorktree, err := filepath.EvalSymlinks(worktreePath)
		require.NoError(t, err)

		cfg := config.DefaultConfig()
		createAtlasConfig(t, mainPath, cfg)

		// Change to main repo
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(mainPath)
		require.NoError(t, err)

		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "功能-branch") //nolint:gosmopolitan // testing unicode support

		require.NoError(t, err)
		assert.Equal(t, resolvedWorktree, ec.WorkDir)
		assert.True(t, ec.IsWorktree)
	})

	t.Run("returns error when not in git repository", func(t *testing.T) {
		tmpDir := t.TempDir()

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(tmpDir)
		require.NoError(t, err)

		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "")

		require.Error(t, err)
		assert.Nil(t, ec)
		assert.Contains(t, err.Error(), "not in a git repository")
	})

	t.Run("returns error when worktree not found", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		cfg := config.DefaultConfig()
		createAtlasConfig(t, repoPath, cfg)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(repoPath)
		require.NoError(t, err)

		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "non-existent-worktree")

		require.Error(t, err)
		assert.Nil(t, ec)
		assert.Contains(t, err.Error(), "worktree")
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error when config loading fails", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		// Create invalid config (malformed YAML)
		atlasDir := filepath.Join(repoPath, ".atlas")
		err := os.MkdirAll(atlasDir, 0o750)
		require.NoError(t, err)
		cfgPath := filepath.Join(atlasDir, "config.yaml")
		err = os.WriteFile(cfgPath, []byte("invalid: yaml: content:\n  bad indent"), 0o600)
		require.NoError(t, err)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(repoPath)
		require.NoError(t, err)

		ctx := context.Background()
		ec, err := ResolveExecutionContext(ctx, "")

		require.Error(t, err)
		assert.Nil(t, ec)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		repoPath := setupTestRepo(t)

		cfg := config.DefaultConfig()
		createAtlasConfig(t, repoPath, cfg)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			require.NoError(t, os.Chdir(originalDir))
		}()
		err = os.Chdir(repoPath)
		require.NoError(t, err)

		// Create canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		ec, err := ResolveExecutionContext(ctx, "")

		// Should return error (either context error or wrapped)
		require.Error(t, err)
		assert.Nil(t, ec)
	})
}

func TestWithExecutionContext(t *testing.T) {
	t.Run("stores and retrieves execution context", func(t *testing.T) {
		originalCtx := context.Background()
		ec := &ExecutionContext{
			WorkDir:      "/test/path",
			MainRepoPath: "/test/main",
			IsWorktree:   true,
			Config:       config.DefaultConfig(),
		}

		// Store context
		ctx := WithExecutionContext(originalCtx, ec)

		// Retrieve context
		retrieved := GetExecutionContext(ctx)

		require.NotNil(t, retrieved)
		assert.Equal(t, ec, retrieved, "should retrieve same ExecutionContext pointer")
		assert.Equal(t, "/test/path", retrieved.WorkDir)
		assert.Equal(t, "/test/main", retrieved.MainRepoPath)
		assert.True(t, retrieved.IsWorktree)
	})

	t.Run("context can be chained", func(t *testing.T) {
		originalCtx := context.Background()
		ec1 := &ExecutionContext{WorkDir: "/first"}
		ec2 := &ExecutionContext{WorkDir: "/second"}

		// Chain contexts
		ctx1 := WithExecutionContext(originalCtx, ec1)
		ctx2 := WithExecutionContext(ctx1, ec2)

		// Last value should win
		retrieved := GetExecutionContext(ctx2)
		require.NotNil(t, retrieved)
		assert.Equal(t, "/second", retrieved.WorkDir)

		// Original context should still have first value
		retrieved1 := GetExecutionContext(ctx1)
		require.NotNil(t, retrieved1)
		assert.Equal(t, "/first", retrieved1.WorkDir)
	})
}

func TestGetExecutionContext(t *testing.T) {
	t.Run("returns nil when no context stored", func(t *testing.T) {
		ctx := context.Background()
		ec := GetExecutionContext(ctx)
		assert.Nil(t, ec, "should return nil when no ExecutionContext is stored")
	})

	t.Run("handles nil context without panic", func(t *testing.T) {
		// This test verifies defensive programming
		//nolint:godox // Intentional note about edge case behavior
		// Note: Calling with nil context is a programming error,
		// but we verify it doesn't panic
		var ctx context.Context

		// Should not panic - this is the bug we're testing for
		assert.NotPanics(t, func() {
			ec := GetExecutionContext(ctx)
			assert.Nil(t, ec)
		})
	})
}

func TestExecutionContextProjectConfigPath(t *testing.T) {
	t.Run("returns worktree config path when in worktree", func(t *testing.T) {
		ec := &ExecutionContext{
			WorkDir:      "/path/to/worktree",
			MainRepoPath: "/path/to/main",
			IsWorktree:   true,
		}

		path := ec.ProjectConfigPath()
		expected := filepath.Join("/path/to/worktree", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})

	t.Run("returns main repo config path when in main repo", func(t *testing.T) {
		ec := &ExecutionContext{
			WorkDir:      "/path/to/main",
			MainRepoPath: "/path/to/main",
			IsWorktree:   false,
		}

		path := ec.ProjectConfigPath()
		expected := filepath.Join("/path/to/main", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})

	t.Run("handles paths with trailing slash", func(t *testing.T) {
		ec := &ExecutionContext{
			WorkDir: "/path/to/repo/",
		}

		path := ec.ProjectConfigPath()
		// filepath.Join handles trailing slashes correctly
		expected := filepath.Join("/path/to/repo/", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})

	t.Run("handles paths with special characters", func(t *testing.T) {
		ec := &ExecutionContext{
			WorkDir: "/path/with spaces/and-dashes",
		}

		path := ec.ProjectConfigPath()
		expected := filepath.Join("/path/with spaces/and-dashes", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})
}

func TestExecutionContextMainRepoConfigPath(t *testing.T) {
	t.Run("always returns main repo path regardless of IsWorktree", func(t *testing.T) {
		ec := &ExecutionContext{
			WorkDir:      "/path/to/worktree",
			MainRepoPath: "/path/to/main",
			IsWorktree:   true,
		}

		path := ec.MainRepoConfigPath()
		expected := filepath.Join("/path/to/main", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})

	t.Run("handles trailing slash in main repo path", func(t *testing.T) {
		ec := &ExecutionContext{
			MainRepoPath: "/path/to/main/",
		}

		path := ec.MainRepoConfigPath()
		expected := filepath.Join("/path/to/main/", ".atlas", "config.yaml")
		assert.Equal(t, expected, path)
	})

	t.Run("handles paths with special characters", func(t *testing.T) {
		ec := &ExecutionContext{
			MainRepoPath: "/path/with unicode/功能", //nolint:gosmopolitan // testing unicode support
		}

		path := ec.MainRepoConfigPath()
		expected := filepath.Join("/path/with unicode/功能", ".atlas", "config.yaml") //nolint:gosmopolitan // testing unicode support
		assert.Equal(t, expected, path)
	})
}
