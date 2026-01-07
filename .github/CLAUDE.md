# CLAUDE.md

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects.
It automates: analyze issues -> implement fixes -> validate code -> create PRs.

For generic Go conventions, see `AGENTS.md` and `tech-conventions/`. This file covers ATLAS-specific patterns.

---

## IMPORTANT
- **This project is MVP and unreleased. Make breaking changes freely. Do not add backwards compatibility layers.**
- When adding features or changing CLI commands, update `docs/internal/quick-start.md`.
- When adding features or changing config, update `.atlas/config.yaml` schema and defaults.

---

## Quick Commands

```bash
# Before committing or opening PR
magex format:fix && magex lint && magex test:race && go-pre-commit run --all-files
```

---

## Package Architecture

| Package | Responsibility |
|---------|----------------|
| `internal/ai` | Claude Code CLI integration (`Runner` interface) |
| `internal/task` | Task engine, state machine, lifecycle management |
| `internal/template` | Workflow templates, step definitions |
| `internal/template/steps` | Step executors (ai, validation, git, human, ci, sdd, verify) |
| `internal/workspace` | Workspace lifecycle, git worktree management |
| `internal/git` | Git operations, GitHub API, smart commits, PR creation |
| `internal/validation` | Format/lint/test execution, parallel runner, AI retry |
| `internal/config` | Configuration loading with precedence |
| `internal/cli` | Cobra commands and subcommands |
| `internal/tui` | Terminal UI: styles, menus, progress, spinners |
| `internal/domain` | Shared types: Task, Workspace, Template, AIRequest/Result |
| `internal/errors` | Sentinel errors (use `errors.Is()` to check) |
| `internal/constants` | Timeouts, file names, directory structure, defaults |
| `internal/logging` | Sensitive data filtering for logs |

---

## Key Interfaces

```go
// AI execution - internal/ai/runner.go
type Runner interface {
    Run(ctx context.Context, req *domain.AIRequest) (*domain.AIResult, error)
}

// Task persistence - internal/task/store.go
type Store interface {
    Create(ctx, workspaceName string, task *domain.Task) error
    Get(ctx, workspaceName, taskID string) (*domain.Task, error)
    Update(ctx, workspaceName string, task *domain.Task) error
    List(ctx, workspaceName string) ([]*domain.Task, error)
    AppendLog(ctx, workspaceName, taskID string, entry []byte) error
    SaveArtifact(ctx, workspaceName, taskID, filename string, data []byte) error
}

// Workspace lifecycle - internal/workspace/manager.go
type Manager interface {
    Create(ctx, name, repoPath, branchType, baseBranch string) (*domain.Workspace, error)
    Get(ctx, name string) (*domain.Workspace, error)
    List(ctx) ([]*domain.Workspace, error)
    Destroy(ctx, name string) error
    Close(ctx, name string) error
}

// Git operations - internal/git/runner.go
type Runner interface {
    Status(ctx) (*Status, error)
    Add(ctx, paths []string) error
    Commit(ctx, message string, trailers map[string]string) error
    Push(ctx, remote, branch string, setUpstream bool) error
    Diff(ctx, cached bool) (string, error)
}

// Step execution - internal/template/steps/executor.go
type StepExecutor interface {
    Execute(ctx context.Context, step *domain.Step) (*domain.StepResult, error)
}
```

---

## Task State Machine

```
pending
   |
   | start
   v
running <-----------------------------+
   |                                  |
   | step complete                    | retry (with feedback)
   v                                  |
validating                            |
   |                                  |
   +-- pass --> awaiting_approval ----+
   |                   |
   +-- fail            +-- approve --> completed
   |                   |
   v                   +-- reject --> rejected
validation_failed
   |
   +-- retry --> running
   +-- abandon --> abandoned

External failure states: gh_failed, ci_failed, ci_timeout
  -> Can retry or abandon from these states
```

---

## Error Handling

Use sentinel errors from `internal/errors/errors.go`:

```go
// Check errors
if errors.Is(err, errors.ErrValidationFailed) { ... }
if errors.Is(err, errors.ErrClaudeInvocation) { ... }
if errors.Is(err, errors.ErrGitOperation) { ... }

// Wrap at package boundaries only
return errors.Wrap(err, "context message")

// User-friendly messages
msg := errors.UserMessage(err)
msg, action := errors.Actionable(err)
```

**Exit codes:** 0 = success, 1 = error, 2 = invalid input (use `errors.NewExitCode2Error()`)

---

## Configuration Precedence

1. CLI flags (highest)
2. Environment variables (`ATLAS_*` prefix)
3. Project config (`.atlas/config.yaml`)
4. Global config (`~/.atlas/config.yaml`)
5. Built-in defaults (lowest)

Key config sections: `ai`, `git`, `worktree`, `ci`, `templates`, `validation`, `notifications`

---

## File Locations

```
~/.atlas/
  config.yaml                           # Global config
  workspaces/<name>/
    workspace.json                      # Workspace metadata
    tasks/task-YYYYMMDD-HHMMSS/
      task.json                         # Task state
      task.log                          # Execution log (JSON-lines)
      artifacts/                        # Step outputs

../<repo>-<workspace>/                  # Git worktree (sibling to repo)
```

---

## Step Types

| Type | Purpose | Auto-proceeds? |
|------|---------|----------------|
| `ai` | Claude Code execution | Yes |
| `validation` | magex commands | Yes if pass; pause on fail |
| `git` | Commit, push, PR | Yes (configurable) |
| `human` | User approval | No - always waits |
| `ci` | GitHub Actions polling | Yes if pass; pause on fail/timeout |
| `sdd` | Speckit integration | Yes |
| `verify` | Cross-model verification | Yes |

---

## Templates

Built-in: `bugfix`, `feature`, `task`, `commit`

Template config overrides in `.atlas/config.yaml`:
```yaml
templates:
  bugfix:
    branch_prefix: "fix"
    model: claude-sonnet-4-5-20250916
  feature:
    branch_prefix: "feat"
    model: claude-opus-4-5-20251101
```

---

## Testing Patterns

- Mocks: `Mock<InterfaceName>` convention
- Thread-safe mocks use `sync.Mutex`
- Platform-specific: `*_unix.go`, `*_windows.go`
- Use `t.TempDir()` for temp files
- Use `require.NoError()` for fatal errors, `assert.*` for non-fatal

---

## Logging

Always filter sensitive data:
```go
import "github.com/mrz1836/atlas/internal/logging"
log.Info().Str("config", logging.SafeValue("config", value)).Msg("loaded")
```

---

## Related Documentation

- Original Architecture: `docs/external/vision.md`
- CLI reference: `docs/internal/quick-start.md`
- Project Config: `.atlas/config.yaml`
- Go conventions: `.github/tech-conventions/go-essentials.md`
- Testing: `.github/tech-conventions/testing-standards.md`
- Commits: `.github/tech-conventions/commit-branch-conventions.md`
