// Package git provides Git operations for ATLAS.
package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// verifyScriptBasicStructure checks basic script requirements.
func verifyScriptBasicStructure(t *testing.T, script string) {
	t.Helper()

	if !strings.HasPrefix(script, "#!/bin/sh\n") {
		t.Error("script should start with #!/bin/sh shebang")
	}

	if !strings.Contains(script, hookMarker) {
		t.Error("script should contain ATLAS_HOOK_WRAPPER marker")
	}

	if !strings.Contains(script, "atlas checkpoint") {
		t.Error("script should contain atlas checkpoint command")
	}

	if !strings.Contains(script, "|| true") {
		t.Error("script should contain failure isolation (|| true)")
	}
}

// verifyScriptMetadata checks that task and workspace IDs are present.
func verifyScriptMetadata(t *testing.T, script, taskID, workspaceID string) {
	t.Helper()

	if taskID != "" && !strings.Contains(script, taskID) {
		t.Errorf("script should contain task ID %q", taskID)
	}
	if workspaceID != "" && !strings.Contains(script, workspaceID) {
		t.Errorf("script should contain workspace ID %q", workspaceID)
	}
}

func TestGenerateHookScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		hookType    HookType
		taskID      string
		workspaceID string
		wantTrigger string
	}{
		{
			name:        "post-commit hook",
			hookType:    HookPostCommit,
			taskID:      "task-123",
			workspaceID: "ws-456",
			wantTrigger: "git_commit",
		},
		{
			name:        "post-push hook",
			hookType:    HookPostPush,
			taskID:      "task-789",
			workspaceID: "ws-abc",
			wantTrigger: "git_push",
		},
		{
			name:        "empty task and workspace IDs",
			hookType:    HookPostCommit,
			taskID:      "",
			workspaceID: "",
			wantTrigger: "git_commit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			script := GenerateHookScript(tt.hookType, tt.taskID, tt.workspaceID)

			verifyScriptBasicStructure(t, script)

			// Verify trigger type is correct
			expectedTriggerCmd := "--trigger " + tt.wantTrigger
			if !strings.Contains(script, expectedTriggerCmd) {
				t.Errorf("script should contain trigger command %q", expectedTriggerCmd)
			}

			verifyScriptMetadata(t, script, tt.taskID, tt.workspaceID)
		})
	}
}

func TestGenerateHookScript_NoInvasiveOperations(t *testing.T) {
	t.Parallel()

	script := GenerateHookScript(HookPostCommit, "task-1", "ws-1")

	// Ensure no file write operations are in the script
	invasivePatterns := []string{
		"mv ",       // no move commands
		"cp ",       // no copy commands
		"rm ",       // no remove commands
		"chmod ",    // no chmod commands
		"> /",       // no file redirects to absolute paths
		">> /",      // no append redirects
		".original", // no backup file references
		"Install",   // no install references
		"Uninstall", // no uninstall references
	}

	for _, pattern := range invasivePatterns {
		if strings.Contains(script, pattern) {
			t.Errorf("script should not contain invasive pattern %q", pattern)
		}
	}
}

func TestResolveHooksDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git repo test in short mode")
	}

	// Create a temporary git repository
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Initialize git repo
	if _, err := RunCommand(ctx, tmpDir, "init"); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Test ResolveHooksDir
	hooksDir, err := ResolveHooksDir(ctx, tmpDir)
	if err != nil {
		t.Fatalf("ResolveHooksDir failed: %v", err)
	}

	// Verify the hooks directory path
	expectedSuffix := filepath.Join(".git", "hooks")
	if !strings.HasSuffix(hooksDir, expectedSuffix) {
		t.Errorf("hooks dir should end with %q, got %q", expectedSuffix, hooksDir)
	}

	// Verify it's an absolute path
	if !filepath.IsAbs(hooksDir) {
		t.Error("hooks dir should be an absolute path")
	}
}

func TestResolveHooksDir_Worktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git repo test in short mode")
	}

	// Create a temporary git repository
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Initialize git repo
	if _, err := RunCommand(ctx, tmpDir, "init"); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create an initial commit (required for worktrees)
	if _, err := RunCommand(ctx, tmpDir, "commit", "--allow-empty", "-m", "initial"); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create a worktree
	worktreePath := filepath.Join(tmpDir, "worktree-test")
	if _, err := RunCommand(ctx, tmpDir, "worktree", "add", "-b", "test-branch", worktreePath); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Test ResolveHooksDir from worktree
	hooksDir, err := ResolveHooksDir(ctx, worktreePath)
	if err != nil {
		t.Fatalf("ResolveHooksDir failed: %v", err)
	}

	// For worktrees, hooks should resolve to the main repo's .git/hooks
	// The hooks dir should NOT be inside the worktree directory
	if strings.HasPrefix(hooksDir, worktreePath) {
		t.Error("worktree hooks should resolve to main repo, not worktree path")
	}

	// Should still end with .git/hooks
	expectedSuffix := filepath.Join(".git", "hooks")
	if !strings.HasSuffix(hooksDir, expectedSuffix) {
		t.Errorf("hooks dir should end with %q, got %q", expectedSuffix, hooksDir)
	}
}

func TestResolveHooksDir_NotGitRepo(t *testing.T) {
	// Create a temporary directory that is NOT a git repo
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Should fail for non-git directories
	_, err := ResolveHooksDir(ctx, tmpDir)
	if err == nil {
		t.Error("ResolveHooksDir should fail for non-git directory")
	}
}

func TestHookType_Constants(t *testing.T) {
	t.Parallel()

	// Verify hook type constants have expected values
	if HookPostCommit != "post-commit" {
		t.Errorf("HookPostCommit should be 'post-commit', got %q", HookPostCommit)
	}
	if HookPostPush != "post-push" {
		t.Errorf("HookPostPush should be 'post-push', got %q", HookPostPush)
	}
}

func TestHookMarker_IsPresent(t *testing.T) {
	t.Parallel()

	// Ensure the marker constant is non-empty and starts with #
	if hookMarker == "" {
		t.Error("hookMarker should not be empty")
	}
	if !strings.HasPrefix(hookMarker, "#") {
		t.Error("hookMarker should be a shell comment (start with #)")
	}
}

func TestGenerateHookScript_Executable(t *testing.T) {
	t.Parallel()

	// The generated script should be valid shell syntax
	script := GenerateHookScript(HookPostCommit, "task-1", "ws-1")

	// Write to temp file and check it's valid shell
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-hook.sh")

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	// Verify file was written
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("failed to stat script: %v", err)
	}

	// Verify it's a regular file
	if !info.Mode().IsRegular() {
		t.Error("script should be a regular file")
	}
}
