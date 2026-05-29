# Lifecycle Service: Package Home Decision (T-228 Phase 1, Task 1.3)

**Date:** 2026-05-28
**Branch:** fix/daemon-daily-driver
**Status:** Decision — ready for Phase 2 implementation

---

## Current State: workflow.Orchestrator

`internal/cli/workflow/orchestrator.go` defines `Orchestrator` with four fields:

| Field | Type | Role |
|---|---|---|
| `services` | `*ServiceFactory` | Creates all runtime dependencies (engine, runners, git services) |
| `initializer` | `*Initializer` | Provisions git worktrees and workspace records |
| `prompter` | `*Prompter` | Interactive CLI prompts (template selection, workspace name) |
| `logger` | `zerolog.Logger` | Structured logging |

**Methods exposed:**

- `Services() *ServiceFactory` — accessor for the service factory
- `Initializer() *Initializer` — accessor for the workspace provisioner
- `Prompter() *Prompter` — accessor for the interactive prompter
- `StartTask(ctx, engine *task.Engine, ws, tmpl, description, fromBacklogID)` — thin wrapper that delegates to `engine.Start()`

**Free functions in the package** (not on Orchestrator but in the `workflow` package):

- `GenerateWorkspaceName`, `SanitizeWorkspaceName` — slug helpers
- `ApplyAgentModelOverrides`, `ApplyVerifyOverrides`, `ApplyDetectOverrides` — template mutation
- `StoreCLIOverrides`, `ApplyCLIOverridesFromTask` — metadata round-trip for resume
- `FindGitRepository` — git root discovery
- `NewDaemonTaskExecutor` — daemon bridge (lives in `daemon_executor.go`)

**Imports / dependencies:**

`orchestrator.go` imports `internal/task` (for `*task.Engine`) and `internal/domain`, `internal/tui`, `internal/constants`. `services.go` (also in package `workflow`) is the heavyweight dependency: it imports `internal/ai`, `internal/config`, `internal/git`, `internal/hook`, `internal/task`, `internal/template/steps`, `internal/tui`, `internal/validation`. `daemon_executor.go` imports `internal/daemon` directly.

**Coupling assessment:**

The `Orchestrator` struct itself is thin and has no direct import of `internal/task/engine` (the concrete engine type). However, its `StartTask` method takes `*task.Engine` as a parameter — a concrete type, not an interface. More significantly, `daemon_executor.go` is already in the `workflow` package and directly imports `internal/daemon`. The package is therefore already doing double duty: it serves the CLI adapter (Orchestrator + Initializer + Prompter) and the daemon adapter (DaemonTaskExecutor). The `ServiceFactory` in `services.go` is the genuine shared core — it wires the engine, AI runners, git services, and validation retry handler identically for both callers.

The `workflow` package is not cleanly separable from the task engine. `ServiceFactory.CreateEngine` returns `*task.Engine` (concrete type) and `RegistryDeps` carries `*task.FileStore` (concrete type). These are deliberate choices that avoid premature interface abstraction, and they mean any attempt to extract a pure lifecycle service into a new package would require either duplicating `ServiceFactory` or making `internal/task` expose interfaces — work that is out of scope for T-228.

---

## Current State: Daemon Execution Path

`runner.go` drives the queue dispatch loop and calls `r.executor.Execute(taskCtx, job)` at `runner.go:496` via the `TaskExecutor` interface (defined in `executor.go`). This is the only abstraction boundary in the daemon execution path.

`TaskExecutor.Execute(ctx, TaskJob)` returns `(engineTaskID, finalStatus string, err error)`. The concrete implementation (`workflow.DaemonTaskExecutor`) does all real work in `daemon_executor.go`:

1. Calls `ServiceFactory.WithRepoPath()` to scope storage
2. Provisions a workspace via `workspace.Manager.Create()`
3. Calls `buildEngine()` to wire AI runner, git services, executor registry, hooks
4. Resolves the template via `template.NewRegistryWithConfig()`
5. Applies agent/model/verify overrides
6. Calls `engine.Start()` (new task) or `engine.Resume()` (resume path)

**Interfaces vs concrete types:**

- `TaskExecutor` in `executor.go` is a clean interface — the only abstraction the daemon knows about.
- Inside the executor, everything is concrete: `*task.Engine`, `*task.FileStore`, `*steps.ExecutorRegistry`, etc.
- There is no `ProgressCallback` wired to a TUI in daemon mode; the executor substitutes a Redis log-stream writer via `makeProgressCallback()`.

**What would change to share a lifecycle service:**

The daemon executor and the CLI `startTaskExecution()` function in `start.go` are already sharing the same underlying `ServiceFactory` methods. The duplication is in the caller layer:
- CLI: `start.go:startTaskExecution()` wires progress callbacks to `tui.Output` spinners, stores a `progressState`, etc.
- Daemon: `daemon_executor.go:buildEngine()` wires progress callbacks to Redis log-stream writes.

A shared lifecycle service would need to accept an abstract output/progress sink so the same engine-setup code runs regardless of caller. The `ProgressCallback func(task.StepProgressEvent)` field in `EngineDeps` already provides this hook point; the missing piece is a unified entry-point function that accepts a `ProgressSink` interface instead of the current split implementations.

---

## Decision: Chosen Package Home

**Option B — Create new `internal/lifecycle/` package** is NOT chosen.

**Option A — Extend `internal/cli/workflow/`** is the correct home.

**Justification:** `workflow.DaemonTaskExecutor` already lives in `internal/cli/workflow/daemon_executor.go` and directly imports `internal/daemon`. The `ServiceFactory` already serves both callers. Creating `internal/lifecycle/` would require either (a) moving `ServiceFactory` there, which breaks all existing `workflow.ServiceFactory` call sites in `start.go` and `resume.go`, or (b) duplicating it, which defeats the purpose. The `workflow` package is already the de facto lifecycle layer; the correct fix is to extract a shared `Run(ctx, RunRequest, ProgressSink)` entry point within the same package, eliminating the parallel implementations in `start.go:startTaskExecution()` and `daemon_executor.go:buildEngine()`.

The name "workflow" is admittedly CLI-flavored, but the coupling to `internal/daemon` (already present) and the import graph make in-place expansion far less disruptive than a new package.

---

## Interface Design

The lifecycle service will be a function or struct in `internal/cli/workflow/` that both callers invoke. The key methods/entry-points needed:

**`RunRequest`** (input value object)
- `Description string` — human-readable task description
- `Template string` — template name (resolved to `*domain.Template` internally)
- `WorkspaceName string` — pre-generated or empty (lifecycle service generates)
- `RepoPath string` — absolute path to git repo
- `Agent, Model string` — optional overrides
- `BaseBranch, TargetBranch string` — workspace provisioning params
- `Verify, NoVerify bool` — verification flags
- `FromBacklogID string` — optional backlog link
- `EngineTaskID string` — non-empty signals resume path

**`ProgressSink` interface** (output abstraction)
- `OnStepStart(event StepProgressEvent)` — step began
- `OnStepComplete(event StepProgressEvent)` — step finished (success, failure, or awaiting)
- `OnStepProgress(event StepProgressEvent)` — sub-step activity update

**Entry-point: `Submit(ctx, req RunRequest, sink ProgressSink) (engineTaskID, finalStatus string, err error)`**
- Provisions workspace if `req.EngineTaskID == ""`; otherwise locates existing worktree from task metadata
- Builds and wires `*task.Engine` via `ServiceFactory`
- Calls `engine.Start()` or `engine.Resume()` based on `req.EngineTaskID`
- Routes progress events to `sink`
- Returns the engine-assigned task ID, final status string, and any error

**`Status(ctx, repoPath, workspaceName, engineTaskID string) (*domain.Task, error)`**
- Loads the task from the file store; no engine interaction needed

**`Cancel(ctx, repoPath, workspaceName, engineTaskID string) error`**
- Transitions a non-running task to `interrupted`; for running tasks the caller cancels the `context.Context` passed to `Submit`

---

## Adapter Boundaries

**Daemon adapter** (`workflow.DaemonTaskExecutor.Execute`):
- Receives `daemon.TaskJob` via IPC handler -> queue -> `runner.executeTask()`
- Translates `TaskJob` fields into `RunRequest`
- Implements `ProgressSink` by writing to Redis log stream via `daemon.LogWriter`
- Returns `(engineTaskID, finalStatus, err)` to `runner.executeTask()`

**Direct (CLI) adapter** (`cli/start.go:startTaskExecution`, `cli/resume.go`):
- Receives parsed `startOptions` / `resumeOptions` from Cobra command
- Translates options into `RunRequest`
- Implements `ProgressSink` by driving `tui.Output` spinners and writing to `progressState`
- Returns final task state to `displayTaskStatus()`

**IO/output abstraction:**
The `ProgressSink` interface is the seam. Its two implementations — the Redis log-stream writer and the TUI spinner updater — are already nearly complete inside `daemon_executor.go:makeProgressCallback()` and `start.go:createProgressCallback()`. Formalizing `ProgressSink` lets both adapters share the same `Submit()` call path.

---

## Assumption A2 Resolution

Assumption A2 from plan.md is CONFIRMED — **Option A: extend `internal/cli/workflow/`**.

The `workflow` package is already the shared implementation layer for both direct and daemon execution. `DaemonTaskExecutor` and `ServiceFactory` both live there, `internal/daemon` is already imported inside the package, and creating a new `internal/lifecycle/` package would require moving or duplicating `ServiceFactory` with no architectural benefit to justify the migration cost.
