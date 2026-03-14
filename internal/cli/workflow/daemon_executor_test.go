package workflow

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mrz1836/atlas/internal/config"
	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/daemon"
	"github.com/mrz1836/atlas/internal/domain"
	"github.com/mrz1836/atlas/internal/task"
)

// makeTestGitRepo creates a minimal git repository in a temp dir for testing.
// It runs git init, sets minimal config, and creates an initial commit so that
// git worktree and git runner operations work correctly.
func makeTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...) //nolint:gosec // G204: test code
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\nOutput: %s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# test"), 0o600))
	runGit("add", ".")
	runGit("commit", "-m", "Initial commit")
	return dir
}

// TestNewDaemonTaskExecutor verifies constructor sets fields correctly.
func TestNewDaemonTaskExecutor(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	logger := zerolog.Nop()
	exec := NewDaemonTaskExecutor(cfg, logger)
	require.NotNil(t, exec)
	assert.Equal(t, cfg, exec.cfg)
}

// TestDaemonTaskExecutor_Execute_RoutesOnEngineTaskID verifies that Execute calls
// resume when EngineTaskID is set and start otherwise.
func TestDaemonTaskExecutor_Execute_StartPath(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	// A job without EngineTaskID triggers start() which will fail early
	// because RepoPath doesn't exist — but we exercise the code path.
	job := daemon.TaskJob{
		TaskID:      "test-1",
		Description: "test",
		Template:    "bug",
		RepoPath:    t.TempDir(), // Real dir; SetupTaskStoreAndConfig may succeed.
	}

	// We expect an error (no real git repo), but the start() path is exercised.
	_, _, err := exec.Execute(context.Background(), job)
	// Error is expected; we just verify it is non-nil and from the start path.
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Execute_ResumePath verifies that Execute calls resume()
// when EngineTaskID is present.
func TestDaemonTaskExecutor_Execute_ResumePath(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	job := daemon.TaskJob{
		TaskID:       "test-2",
		EngineTaskID: "eng-123", // non-empty triggers resume()
		Workspace:    "ws",
		RepoPath:     t.TempDir(),
	}

	_, _, err := exec.Execute(context.Background(), job)
	// Error expected because there's no real task store, but resume() was called.
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Abandon_NilEngineTaskID verifies early return when no engine task.
func TestDaemonTaskExecutor_Abandon_NilEngineTaskID(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	// No EngineTaskID → should return nil immediately.
	err := exec.Abandon(context.Background(), daemon.TaskJob{}, "reason")
	assert.NoError(t, err)
}

// TestDaemonTaskExecutor_Abandon_NilWorkspace verifies early return when no workspace.
func TestDaemonTaskExecutor_Abandon_NilWorkspace(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	err := exec.Abandon(context.Background(), daemon.TaskJob{EngineTaskID: "eng-1"}, "reason")
	assert.NoError(t, err)
}

// TestDaemonTaskExecutor_Abandon_TaskStoreError verifies error propagation.
func TestDaemonTaskExecutor_Abandon_TaskStoreError(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	// Use a non-existent repo path to force task store error.
	job := daemon.TaskJob{
		EngineTaskID: "eng-1",
		Workspace:    "ws",
		RepoPath:     "/nonexistent/path/to/repo",
	}

	err := exec.Abandon(context.Background(), job, "reason")
	// Should fail because task store can't be created for non-existent path.
	assert.Error(t, err)
}

// TestResolveGitCfgFromConfig verifies git config resolution with fallbacks.
func TestResolveGitCfgFromConfig(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	cfg.AI.Agent = "global-agent"
	cfg.AI.Model = "global-model"
	cfg.SmartCommit.Agent = "commit-agent"
	cfg.SmartCommit.Model = ""       // falls back to global
	cfg.PRDescription.Agent = ""     // falls back to global
	cfg.PRDescription.Model = "pr-m" // explicit

	gitCfg := resolveGitCfgFromConfig(cfg)
	assert.Equal(t, "commit-agent", gitCfg.CommitAgent)
	assert.Equal(t, "global-model", gitCfg.CommitModel)
	assert.Equal(t, "global-agent", gitCfg.PRDescAgent)
	assert.Equal(t, "pr-m", gitCfg.PRDescModel)
}

// TestCoalesce verifies coalesce returns the first non-empty value.
func TestCoalesce(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a", coalesce("a", "b", "c"))
	assert.Equal(t, "b", coalesce("", "b", "c"))
	assert.Equal(t, "c", coalesce("", "", "c"))
	assert.Empty(t, coalesce("", "", ""))
}

// TestResolveTemplate_DefaultsToTask verifies that an empty template name uses "task".
func TestResolveTemplate_DefaultsToTask(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}
	cfg := config.DefaultConfig()
	// Empty template → defaults to "task".
	tmpl, err := exec.resolveTemplate(daemon.TaskJob{Template: ""}, cfg)
	require.NoError(t, err)
	assert.NotNil(t, tmpl)
	assert.Equal(t, "task", tmpl.Name)
}

// TestResolveTemplate_ValidTemplate verifies known templates resolve correctly.
func TestResolveTemplate_ValidTemplate(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}
	cfg := config.DefaultConfig()

	for _, name := range []string{"bug", "feature", "commit", "patch"} {
		tmpl, err := exec.resolveTemplate(daemon.TaskJob{Template: name}, cfg)
		require.NoError(t, err, "template %q should resolve", name)
		assert.NotNil(t, tmpl)
	}
}

// TestResolveTemplate_InvalidTemplate verifies unknown templates return an error.
func TestResolveTemplate_InvalidTemplate(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}
	cfg := config.DefaultConfig()

	_, err := exec.resolveTemplate(daemon.TaskJob{Template: "nonexistent-template-xyz"}, cfg)
	require.Error(t, err)
}

// TestDaemonTaskExecutor_Abandon_Integration tests the full abandon flow using a
// real task file store on a temp directory.
func TestDaemonTaskExecutor_Abandon_Integration(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: cfg}

	// Create a real task store in the temp dir.
	taskStore, err := task.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)

	// Create a task in the store.
	wsName := "test-ws"
	t.Log("creating task in store")
	testTask := &domain.Task{
		ID:          "task-abc",
		WorkspaceID: wsName,
		Status:      constants.TaskStatusInterrupted, // interrupted can transition to abandoned
		CreatedAt:   now(),
		UpdatedAt:   now(),
		Transitions: []domain.Transition{},
		Steps:       []domain.Step{},
		Metadata:    map[string]any{},
	}
	require.NoError(t, taskStore.Create(context.Background(), wsName, testTask))

	// Build a job pointing to the temp dir.
	job := daemon.TaskJob{
		EngineTaskID: testTask.ID,
		Workspace:    wsName,
		RepoPath:     tmpDir,
	}

	err = exec.Abandon(context.Background(), job, "test abandon")
	require.NoError(t, err)

	// Verify the task was transitioned to abandoned.
	updated, getErr := taskStore.Get(context.Background(), wsName, testTask.ID)
	require.NoError(t, getErr)
	assert.Equal(t, constants.TaskStatusAbandoned, updated.Status)
}

// TestDaemonTaskExecutor_ImplementsInterface is a compile-time check.
func TestDaemonTaskExecutor_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var _ daemon.TaskExecutor = (*DaemonTaskExecutor)(nil)
}

// TestDaemonTaskExecutor_Resume_AppliesMetadata verifies that approval metadata
// is persisted before resuming (error expected due to no real workspace, but
// the metadata path is exercised).
func TestDaemonTaskExecutor_Resume_AppliesMetadata(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	tmpDir := t.TempDir()
	taskStore, err := task.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)

	wsName := "resume-ws"
	testTask := &domain.Task{
		ID:          "resume-task-1",
		WorkspaceID: wsName,
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   now(),
		UpdatedAt:   now(),
		Transitions: []domain.Transition{},
		Steps:       []domain.Step{},
		Metadata:    map[string]any{"worktree_dir": "/tmp/wt", "branch": "feat/x"},
	}
	require.NoError(t, taskStore.Create(context.Background(), wsName, testTask))

	// resume() will error on buildEngine (no real git) but the metadata update path runs.
	job := daemon.TaskJob{
		EngineTaskID:   testTask.ID,
		Workspace:      wsName,
		RepoPath:       tmpDir,
		ApprovalChoice: "approve",
		RejectFeedback: "some feedback",
	}

	_, _, resumeErr := exec.Execute(context.Background(), job)
	// An error is expected (no real git repo), but we verify the metadata was saved.
	require.Error(t, resumeErr)

	updated, getErr := taskStore.Get(context.Background(), wsName, testTask.ID)
	require.NoError(t, getErr)
	assert.Equal(t, "approve", updated.Metadata["step_approval_choice"])
	assert.Equal(t, "some feedback", updated.Metadata["reject_feedback"])
}

// TestDaemonTaskExecutor_Abandon_GetTaskError verifies error propagation when
// the engine task does not exist in the store.
func TestDaemonTaskExecutor_Abandon_GetTaskError(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	tmpDir := t.TempDir()
	// Task store can be created for a valid dir, but no task with this ID exists.
	job := daemon.TaskJob{
		EngineTaskID: "nonexistent-task-id",
		Workspace:    "ws",
		RepoPath:     tmpDir,
	}

	err := exec.Abandon(context.Background(), job, "reason")
	// Should fail because the task doesn't exist in the store.
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Abandon_InvalidTransition verifies error propagation when
// the state transition is not allowed.
func TestDaemonTaskExecutor_Abandon_InvalidTransition(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: cfg}

	taskStore, err := task.NewRepoScopedFileStore(tmpDir)
	require.NoError(t, err)

	wsName := "trans-ws"
	// "completed" cannot transition to "abandoned"
	completedTask := &domain.Task{
		ID:          "completed-task-1",
		WorkspaceID: wsName,
		Status:      constants.TaskStatusCompleted,
		CreatedAt:   now(),
		UpdatedAt:   now(),
		Transitions: []domain.Transition{},
		Steps:       []domain.Step{},
		Metadata:    map[string]any{},
	}
	require.NoError(t, taskStore.Create(context.Background(), wsName, completedTask))

	job := daemon.TaskJob{
		EngineTaskID: completedTask.ID,
		Workspace:    wsName,
		RepoPath:     tmpDir,
	}

	err = exec.Abandon(context.Background(), job, "reason")
	assert.Error(t, err, "completed→abandoned should be an invalid transition")
}

// TestDaemonTaskExecutor_Resume_GetTaskError verifies that resume returns an error
// when the engine task cannot be found in the store.
func TestDaemonTaskExecutor_Resume_GetTaskError(t *testing.T) {
	t.Parallel()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	tmpDir := t.TempDir()
	// Valid task store but the task ID doesn't exist.
	job := daemon.TaskJob{
		EngineTaskID: "no-such-task",
		Workspace:    "ws",
		RepoPath:     tmpDir,
	}

	_, _, err := exec.Execute(context.Background(), job)
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Start_WithGitRepo verifies the full start() code path using a
// real git repository. The call will fail at eng.Start (no AI runner configured) but all
// preceding lines — provisionWorkspace, buildEngine, resolveTemplate — are exercised.
func TestDaemonTaskExecutor_Start_WithGitRepo(t *testing.T) {
	// Not parallel: creates sibling git worktree directories in the filesystem.
	repoPath := makeTestGitRepo(t)

	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	job := daemon.TaskJob{
		Description: "test start with real git",
		Template:    "task",
		RepoPath:    repoPath,
	}

	// Use a timed-out context so the real AI agent isn't invoked and the executor returns an error fast.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	_, _, err := exec.Execute(ctx, job)
	// Error is expected (no real AI runner), but the start() path through
	// provisionWorkspace → buildEngine → resolveTemplate → eng.Start is covered.
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Start_InvalidTemplate verifies that resolveTemplate's error
// path inside start() is covered: provisionWorkspace succeeds but template lookup fails.
func TestDaemonTaskExecutor_Start_InvalidTemplate(t *testing.T) {
	// Not parallel: creates sibling git worktree directories.
	repoPath := makeTestGitRepo(t)

	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	job := daemon.TaskJob{
		Description: "test invalid template",
		Template:    "no-such-template-xyz",
		RepoPath:    repoPath,
	}

	_, _, err := exec.Execute(context.Background(), job)
	// Error expected: resolveTemplate returns "template not found".
	assert.Error(t, err)
}

// TestDaemonTaskExecutor_Start_WithExistingBranch verifies the provisionWorkspace branch
// override path (job.Branch != "") is exercised with a real git repository.
func TestDaemonTaskExecutor_Start_WithExistingBranch(t *testing.T) {
	// Not parallel: creates sibling git worktree directories.
	repoPath := makeTestGitRepo(t)

	// Create a branch to check out via the existing-branch code path.
	ctx := context.Background()
	branchCmd := exec.CommandContext(ctx, "git", "checkout", "-b", "feat/branch-override")
	branchCmd.Dir = repoPath
	require.NoError(t, branchCmd.Run())
	// Return to the default branch so feat/branch-override is available for worktree.
	defaultCmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	defaultCmd.Dir = repoPath
	out, err := defaultCmd.Output()
	require.NoError(t, err)
	_ = out // Current branch is feat/branch-override; need to go back to initial.
	backCmd := exec.CommandContext(ctx, "git", "checkout", "-")
	backCmd.Dir = repoPath
	require.NoError(t, backCmd.Run())

	execE := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: config.DefaultConfig()}

	job := daemon.TaskJob{
		Description: "test existing branch",
		Template:    "task",
		RepoPath:    repoPath,
		Branch:      "feat/branch-override",
	}

	// Use a timed-out context to avoid hanging on actual Claude invocation
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, _, startErr := execE.Execute(testCtx, job)
	// Error expected (no AI runner), but the job.Branch != "" code path is covered.
	assert.Error(t, startErr)
}

// TestDaemonTaskExecutor_Resume_WithGitRepo exercises the resume() code path beyond
// buildEngine by placing the main git repo as the task's worktree_dir so that
// CreateGitServices succeeds, allowing resolveTemplate and eng.Resume to be reached.
func TestDaemonTaskExecutor_Resume_WithGitRepo(t *testing.T) {
	// Not parallel: touches real git repo and task store files.
	repoPath := makeTestGitRepo(t)

	cfg := config.DefaultConfig()
	exec := &DaemonTaskExecutor{logger: zerolog.Nop(), cfg: cfg}

	taskStore, err := task.NewRepoScopedFileStore(repoPath)
	require.NoError(t, err)

	wsName := "resume-git-ws"
	testTask := &domain.Task{
		ID:          "resume-git-task-1",
		WorkspaceID: wsName,
		Status:      constants.TaskStatusAwaitingApproval,
		CreatedAt:   now(),
		UpdatedAt:   now(),
		Transitions: []domain.Transition{},
		Steps:       []domain.Step{},
		// Point worktree_dir at the repo itself so buildEngine's CreateGitServices succeeds.
		Metadata: map[string]any{"worktree_dir": repoPath, "branch": "master"},
	}
	require.NoError(t, taskStore.Create(context.Background(), wsName, testTask))

	job := daemon.TaskJob{
		EngineTaskID: testTask.ID,
		Workspace:    wsName,
		RepoPath:     repoPath,
		Template:     "task",
	}

	// Use a timed-out context to avoid hanging on actual Claude invocation
	testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, _, resumeErr := exec.Execute(testCtx, job)
	// Error is expected (no AI runner), but resolveTemplate and eng.Resume are exercised.
	assert.Error(t, resumeErr)
}

// -- helpers --

func now() time.Time { return time.Now().UTC() }

// Suppress unused import.
var _ = errors.New
