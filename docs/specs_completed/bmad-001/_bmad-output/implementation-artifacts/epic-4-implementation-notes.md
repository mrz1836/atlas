# Epic 4 Implementation Notes

**Created:** 2025-12-28
**Source:** Epic 3 Retrospective
**Status:** Pre-work required before Epic 4 stories begin

---

## Critical Pre-Work Items

These changes MUST be implemented before starting Epic 4 stories. They can be done as "Story 4.0" or as pre-work by the first developer.

---

## E4-A1: Add BaseBranch Support to Manager.Create() [P0 - CRITICAL]

### Problem Statement

Users need to create workspaces from branches other than the current HEAD. For example:
- User is on `develop` but wants to start a task from `main`
- User wants to work from a remote branch `origin/feature-x`

Currently, `Manager.Create()` doesn't accept a `baseBranch` parameter, so all worktrees are created from the current HEAD.

### Current Implementation

```go
// internal/workspace/manager.go - Line 21
type Manager interface {
    Create(ctx context.Context, name, repoPath, branchType string) (*Workspace, error)
    // ...
}

// Line 90-94 - BaseBranch is NOT passed
wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
    RepoPath:      repoPath,
    WorkspaceName: name,
    BranchType:    branchType,
    // BaseBranch: NOT SET!
})
```

### Required Changes

#### 1. Update Manager Interface

```go
// internal/workspace/manager.go

// CreateOptions contains options for creating a workspace.
type CreateOptions struct {
    Name       string // Workspace name (required)
    RepoPath   string // Path to the main repository (required)
    BranchType string // Branch type prefix: feat, fix, chore (required)
    BaseBranch string // Branch to create worktree from (optional, default: current HEAD)
}

type Manager interface {
    // Create creates a new workspace with a git worktree.
    // Returns ErrWorkspaceExists if workspace already exists.
    Create(ctx context.Context, opts CreateOptions) (*Workspace, error)
    // ... rest unchanged
}
```

#### 2. Update Manager Implementation

```go
func (m *DefaultManager) Create(ctx context.Context, opts CreateOptions) (*Workspace, error) {
    // ... validation ...

    // Create worktree with BaseBranch
    wtInfo, err := m.worktreeRunner.Create(ctx, WorktreeCreateOptions{
        RepoPath:      opts.RepoPath,
        WorkspaceName: opts.Name,
        BranchType:    opts.BranchType,
        BaseBranch:    opts.BaseBranch, // Now passed through!
    })
    // ...
}
```

#### 3. Update All Callers

Search for `Manager.Create(` and update all callers to use the new `CreateOptions` struct.

#### 4. Update `atlas start` Command (Story 4.7)

Add `--from` flag:

```go
cmd.Flags().StringVar(&fromBranch, "from", "", "Branch to create workspace from (default: current HEAD)")

// In command execution:
workspace, err := manager.Create(ctx, workspace.CreateOptions{
    Name:       workspaceName,
    RepoPath:   repoPath,
    BranchType: branchType,
    BaseBranch: fromBranch,
})
```

#### 5. Consider Remote Branch Handling

If user specifies a remote branch (e.g., `origin/main`), consider fetching first:

```go
if strings.HasPrefix(opts.BaseBranch, "origin/") {
    // Optionally fetch to ensure we have latest
    if err := m.gitRunner.Fetch(ctx); err != nil {
        logger.Warn().Err(err).Msg("failed to fetch, using local state")
    }
}
```

### CLI Usage Examples

```bash
# From current HEAD (default behavior)
atlas start "fix auth bug"

# From local main branch
atlas start "fix auth bug" --from main

# From remote main (latest)
atlas start "fix auth bug" --from origin/main

# From a specific tag
atlas start "fix auth bug" --from v1.2.3
```

### Tests Required

1. Test Create with empty BaseBranch (uses current HEAD)
2. Test Create with local branch BaseBranch
3. Test Create with remote branch BaseBranch
4. Test Create with non-existent BaseBranch (should error)
5. Update all existing Manager tests to use CreateOptions

---

## E4-A2: Change Worktree Location to ~/.atlas/worktrees/ [P0 - CRITICAL]

### Problem Statement

Current worktrees are created as siblings to the main repo (e.g., `../atlas-auth/`), which:
- Clutters the parent directory
- Is not centralized with other ATLAS state
- May confuse users

### Current Implementation

```go
// internal/workspace/worktree.go - Line 311-318
func siblingPath(repoRoot, workspaceName string) string {
    repoDir := filepath.Dir(repoRoot)
    repoName := filepath.Base(repoRoot)
    return filepath.Join(repoDir, repoName+"-"+workspaceName)
}
```

### Required Changes

#### 1. Add Constant for Worktrees Directory

```go
// internal/constants/paths.go
const WorktreesDir = "worktrees"
```

#### 2. Update GitWorktreeRunner

```go
// internal/workspace/worktree.go

type GitWorktreeRunner struct {
    repoPath  string
    atlasHome string // NEW: Path to ~/.atlas
}

func NewGitWorktreeRunner(repoPath, atlasHome string) (*GitWorktreeRunner, error) {
    root, err := detectRepoRoot(context.Background(), repoPath)
    if err != nil {
        return nil, fmt.Errorf("failed to detect git repository: %w", err)
    }
    return &GitWorktreeRunner{
        repoPath:  root,
        atlasHome: atlasHome,
    }, nil
}
```

#### 3. Update Path Calculation

```go
// Replace siblingPath with worktreePath
func worktreePath(atlasHome, workspaceName string) string {
    return filepath.Join(atlasHome, constants.WorktreesDir, workspaceName)
}

// In Create method:
func (r *GitWorktreeRunner) Create(ctx context.Context, opts WorktreeCreateOptions) (*WorktreeInfo, error) {
    // ...

    // Calculate path under ~/.atlas/worktrees/
    wtPath := worktreePath(r.atlasHome, opts.WorkspaceName)
    wtPath, err := ensureUniquePath(wtPath)
    if err != nil {
        return nil, err
    }

    // Ensure parent directory exists
    if err := os.MkdirAll(filepath.Dir(wtPath), 0o750); err != nil {
        return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
    }

    // ... rest unchanged
}
```

#### 4. Update Public Wrapper

```go
// Remove or deprecate SiblingPath, add WorktreePath
func WorktreePath(atlasHome, workspaceName string) string {
    return worktreePath(atlasHome, workspaceName)
}
```

#### 5. Update All Callers

Search for `NewGitWorktreeRunner(` and update to pass atlasHome:

```go
// Before
wtRunner, err := workspace.NewGitWorktreeRunner(repoPath)

// After
atlasHome := config.AtlasHome() // or use constants.DefaultAtlasHome
wtRunner, err := workspace.NewGitWorktreeRunner(repoPath, atlasHome)
```

### Directory Structure After Change

```
~/.atlas/
├── config.yaml           # Global config
├── logs/                  # Host CLI logs
├── workspaces/           # Workspace METADATA
│   └── auth/
│       ├── workspace.json
│       └── tasks/
│           └── task-20251228-100000/
│               ├── task.json
│               └── task.log
└── worktrees/            # Actual GIT WORKTREES (NEW)
    └── auth/
        ├── .git          # File pointing to main repo's .git
        ├── go.mod
        ├── main.go
        └── (all source files)
```

### Git Compatibility Notes

Git worktrees work correctly regardless of location because:

1. The worktree directory contains a `.git` **file** (not directory) with:
   ```
   gitdir: /path/to/main/repo/.git/worktrees/auth
   ```

2. The main repo's `.git/worktrees/` directory contains metadata for each worktree.

3. All git commands work normally from within the worktree.

### Tests Required

1. Update all worktree tests to pass atlasHome
2. Test worktree creation under ~/.atlas/worktrees/
3. Test that git operations work from new location
4. Test that workspace metadata and worktree are in correct separate locations
5. Test cleanup removes both metadata and worktree

---

## E4-A3: Print Worktree Path After Creation [P1]

### Problem Statement

Since worktrees are now in `~/.atlas/worktrees/`, users need clear guidance on where to find them.

### Required Changes

Update `atlas start` command to print path info:

```go
// After successful workspace creation
fmt.Fprintf(w, "%s Workspace '%s' created\n",
    styles.Success.Render("✓"),
    workspace.Name)
fmt.Fprintf(w, "  Branch: %s\n", workspace.Branch)
fmt.Fprintf(w, "  Path:   %s\n", workspace.WorktreePath)
fmt.Fprintln(w)
fmt.Fprintf(w, "  To enter: cd %s\n", workspace.WorktreePath)
```

**Example Output:**
```
✓ Workspace 'auth' created
  Branch: feat/auth
  Path:   /Users/mrz/.atlas/worktrees/auth

  To enter: cd /Users/mrz/.atlas/worktrees/auth
```

---

## E4-A4: Add `atlas workspace path <name>` Command [P2]

### Implementation

```go
// internal/cli/workspace_path.go

func addWorkspacePathCmd(parent *cobra.Command) {
    cmd := &cobra.Command{
        Use:   "path <name>",
        Short: "Print the worktree path for a workspace",
        Long: `Print the filesystem path to the workspace's git worktree.

Useful for scripting or quick navigation:
  cd $(atlas workspace path auth)`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            return runWorkspacePath(cmd.Context(), cmd, os.Stdout, args[0])
        },
    }
    parent.AddCommand(cmd)
}

func runWorkspacePath(ctx context.Context, cmd *cobra.Command, w io.Writer, name string) error {
    store, err := workspace.NewFileStore("")
    if err != nil {
        return err
    }

    ws, err := store.Get(ctx, name)
    if err != nil {
        return fmt.Errorf("workspace '%s' not found", name)
    }

    // For active workspaces, print worktree path
    if ws.WorktreePath != "" {
        fmt.Fprintln(w, ws.WorktreePath)
        return nil
    }

    // For retired workspaces, explain
    return fmt.Errorf("workspace '%s' is retired (no worktree)", name)
}
```

### Usage

```bash
atlas workspace path auth
# Output: /Users/mrz/.atlas/worktrees/auth

# Shell integration
cd $(atlas workspace path auth)

# In scripts
WORKSPACE_PATH=$(atlas workspace path auth)
```

---

## E4-A5: Shell Helper for Directory Change [P3 - FUTURE]

Since a subprocess cannot change the parent shell's directory, document a shell function workaround:

```bash
# Add to ~/.bashrc or ~/.zshrc
atlas-cd() {
    local path
    path=$(atlas workspace path "$1" 2>/dev/null)
    if [ -n "$path" ]; then
        cd "$path" || return 1
    else
        echo "Workspace '$1' not found or has no worktree" >&2
        return 1
    fi
}

# Usage:
# atlas-cd auth
```

This is P3 (future enhancement) - document in README but don't implement as a command.

---

## Validation Checklist

Before marking Epic 4 pre-work as complete:

- [ ] E4-A1: Manager.Create() accepts CreateOptions with BaseBranch
- [ ] E4-A1: WorktreeRunner.Create() uses BaseBranch when provided
- [ ] E4-A1: All existing Manager tests updated and passing
- [ ] E4-A2: GitWorktreeRunner accepts atlasHome parameter
- [ ] E4-A2: Worktrees created under ~/.atlas/worktrees/
- [ ] E4-A2: Worktrees directory created automatically if missing
- [ ] E4-A2: All existing worktree tests updated and passing
- [ ] E4-A2: Git operations work from new worktree location
- [ ] All validation commands pass: `magex format:fix && magex lint && magex test:race`

---

## References

- Epic 3 Retrospective: `_bmad-output/implementation-artifacts/epic-3-retro-2025-12-28.md`
- Worktree implementation: `internal/workspace/worktree.go`
- Manager implementation: `internal/workspace/manager.go`
- Constants: `internal/constants/paths.go`
