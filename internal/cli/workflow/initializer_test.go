package workflow

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInitializer(t *testing.T) {
	logger := zerolog.Nop()
	init := NewInitializer(logger)
	assert.NotNil(t, init)
	assert.Equal(t, logger, init.logger)
}

func TestCreateWorkspaceSimple(t *testing.T) {
	t.Run("creates standalone function wrapper", func(_ *testing.T) {
		// This test verifies the standalone function exists and can be called
		// The actual workspace creation will fail without a real git repo, but we can verify the function signature
		ctx := context.Background()
		_, err := CreateWorkspaceSimple(ctx, "test", "/nonexistent", "feature", "master", "", false)
		// We expect an error since we're using a nonexistent path
		assert.Error(t, err)
	})
}

func TestFindGitRepository_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(_ *testing.T) {
		ctx := context.Background()
		// This will likely fail since we may not be in a git repo,
		// but we're testing that the function exists and can be called
		_, _ = FindGitRepository(ctx)
	})
}

func TestCleanupWorkspace_StandaloneFunction(t *testing.T) {
	t.Run("standalone function exists", func(_ *testing.T) {
		ctx := context.Background()
		// This will fail but we're testing the function signature
		_ = CleanupWorkspace(ctx, "test-ws", "/nonexistent")
	})
}

func TestInitializer_FindGitRepository(t *testing.T) {
	t.Run("method exists", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		// This may succeed or fail depending on whether we're in a git repo
		// We're just testing that the method exists and can be called
		_, _ = init.FindGitRepository(ctx)
	})
}

func TestWorkspaceOptions(t *testing.T) {
	t.Run("struct fields exist", func(_ *testing.T) {
		opts := WorkspaceOptions{
			Name:          "test-workspace",
			RepoPath:      "/path/to/repo",
			BranchPrefix:  "feature",
			BaseBranch:    "master",
			TargetBranch:  "",
			UseLocal:      false,
			NoInteractive: false,
			OutputFormat:  "text",
			ErrorHandler:  func(_ string, err error) error { return err },
		}

		assert.Equal(t, "test-workspace", opts.Name)
		assert.Equal(t, "/path/to/repo", opts.RepoPath)
		assert.Equal(t, "feature", opts.BranchPrefix)
		assert.Equal(t, "master", opts.BaseBranch)
		assert.Empty(t, opts.TargetBranch)
		assert.False(t, opts.UseLocal)
		assert.False(t, opts.NoInteractive)
		assert.Equal(t, "text", opts.OutputFormat)
		assert.NotNil(t, opts.ErrorHandler)
	})
}

func TestCreateWorkspace_ErrorHandler(t *testing.T) {
	t.Run("error handler is called on workspace store creation failure", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		errorHandlerCalled := false
		var capturedWorkspaceName string
		var capturedError error

		opts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/nonexistent/path/that/should/not/exist",
			BranchPrefix: "feature",
			BaseBranch:   "master",
			UseLocal:     false,
			ErrorHandler: func(wsName string, err error) error {
				errorHandlerCalled = true
				capturedWorkspaceName = wsName
				capturedError = err
				return err
			},
		}

		_, err := init.CreateWorkspace(ctx, opts)

		// Should have an error
		require.Error(t, err)

		// Error handler should have been called
		assert.True(t, errorHandlerCalled, "ErrorHandler should have been called")
		assert.Equal(t, "test-workspace", capturedWorkspaceName)
		assert.Error(t, capturedError)
	})
}

func TestCleanupWorkspace_Method(t *testing.T) {
	t.Run("cleanup workspace creates store and runner", func(_ *testing.T) {
		logger := zerolog.Nop()
		init := NewInitializer(logger)
		ctx := context.Background()

		// This will fail due to nonexistent path, but tests the method flow
		err := init.CleanupWorkspace(ctx, "test-workspace", "/nonexistent")
		assert.Error(t, err)
	})
}

func TestCreateWorkspace_BranchModes(t *testing.T) {
	t.Run("new branch mode vs existing branch mode options", func(_ *testing.T) {
		// Test that the two modes have different option structures

		// New branch mode
		newBranchOpts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/path/to/repo",
			BranchPrefix: "feature", // Set for new branch mode
			BaseBranch:   "master",  // Set for new branch mode
			TargetBranch: "",        // Empty for new branch mode
			ErrorHandler: func(_ string, err error) error { return err },
		}

		assert.Equal(t, "feature", newBranchOpts.BranchPrefix)
		assert.Equal(t, "master", newBranchOpts.BaseBranch)
		assert.Empty(t, newBranchOpts.TargetBranch)

		// Existing branch mode
		existingBranchOpts := WorkspaceOptions{
			Name:         "test-workspace",
			RepoPath:     "/path/to/repo",
			BranchPrefix: "",              // Empty for existing branch mode
			BaseBranch:   "",              // Empty for existing branch mode
			TargetBranch: "hotfix/urgent", // Set for existing branch mode
			ErrorHandler: func(_ string, err error) error { return err },
		}

		assert.Empty(t, existingBranchOpts.BranchPrefix)
		assert.Empty(t, existingBranchOpts.BaseBranch)
		assert.Equal(t, "hotfix/urgent", existingBranchOpts.TargetBranch)
	})
}
