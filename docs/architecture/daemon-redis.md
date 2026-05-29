# Architecture Decision Record: Daemon and Redis Design

**Document:** `docs/architecture/daemon-redis.md`
**Status:** Informational (read-only architecture note; no code changes here)
**Date:** 2026-05-28

---

## Purpose

This document maps the current (as of `origin/master` commit `f1de323`) daemon architecture for the
Atlas project. Every pre-existing audit finding is named in Section 12.

---

## 1. Source-of-Truth Boundaries

### What Redis owns (daemon mode only)

| Resource | Redis representation |
|---|---|
| Queued task metadata | Hash at `<prefix>task:<uuid>` — id, description, template, status, priority, submitted_at, branch, repo_path, agent, model, target_branch, use_local, verify, no_verify |
| Persistent task index | Set at `<prefix>tasks` — all submitted UUIDs, including completed/failed/canceled |
| Active task set | Set at `<prefix>active` — UUIDs of tasks that are not yet terminal |
| Queue (by priority) | Sorted sets at `<prefix>queue:urgent`, `<prefix>queue:normal`, `<prefix>queue:low` — score is microsecond Unix timestamp (see Finding 5) |
| Worker lock | String at `<prefix>lock:task:<uuid>` — owned by the worker goroutine while the task is executing |
| Daemon heartbeat | String at `atlas:daemon:heartbeat` — written with 30s TTL every HeartbeatInterval (default 10s). **Note: hardcoded prefix, not `cfg.Redis.KeyPrefix` — see Finding 8** |
| Daemon state hash | Hash at `atlas:daemon:state` — pid, uptime, version, status, updated_at. **Note: hardcoded prefix — see Finding 8** |
| Log stream | Redis Stream at `<prefix>log:<uuid>` — structured LogEntry JSON under a `data` field, capped at `LogStreamMaxLen` (default 10000) entries |
| Queue notify pub/sub | Channel `<prefix>queue:notify` — lightweight signal to wake the dispatch loop on new submission |
| Task events pub/sub | Channel `atlas:events` — TaskEvent JSON for UI/subscriber consumers. **Note: hardcoded channel name, not prefixed** |

### What the filesystem owns

| Resource | Path |
|---|---|
| Engine task JSON files | `~/.atlas/tasks/<workspace-name>/<task-id>.json` (via `task.FileStore`) |
| Workspace metadata JSON | `~/.atlas/workspaces/<repo-hash>/<workspace-name>.json` (via `workspace.FileStore`) |
| Hook/checkpoint files | `~/.atlas/hooks/<task-id>/` — checkpoint JSON, signed receipt, state transitions |
| Daemon socket | Path from `cfg.Daemon.SocketPath` (default `~/.atlas/daemon.sock`) |
| Daemon PID file | Path from `cfg.Daemon.PIDFile` (default `~/.atlas/daemon.pid`) |
| Daemon log file | Path from `cfg.Daemon.LogFile` (default `~/.atlas/logs/daemon.log`) |
| Activity logs | `~/.atlas/activity/<task-id>-*.log` — per-step AI activity logs |

### The source-of-truth handoff point

During daemon-mode execution the source of truth transfers as follows:

1. **From submit to queue-pop:** Redis is the sole source of truth for task existence. The task hash, persistent task set, active set, and queue entry all exist in Redis. No filesystem record of this task exists yet.

2. **At engine `Start` call:** `DaemonTaskExecutor.start` creates a git worktree (filesystem), calls `eng.Start`, which calls `task.FileStore.Create` to persist the engine task JSON. At this point both Redis (status = `running`, `engine_task_id` written back) and filesystem (engine task JSON) hold task state.

3. **After terminal state:** Redis status field (`completed` / `failed` / `canceled` / `abandoned`) reflects the final outcome. The engine task JSON on disk also reflects the terminal state. The two are written independently and can theoretically diverge if one write succeeds and the other fails — no atomic two-phase commit exists across Redis and filesystem. Recovery (Section 8) is Redis-centric and does not read filesystem state.

**Practical contract:** Redis is authoritative for daemon task lifecycle (queued/running/terminal). The filesystem is authoritative for the engine task content (steps, results, step history) and workspace/workspace-files. For daily-driver use the two must be kept in sync; planned reconciliation tooling (see Finding 9) will detect and report drift.

---

## 2. Redis Key Inventory

All keys produced by `internal/daemon/*.go`:

| Key Pattern | Type | Owner | Prefixed? |
|---|---|---|---|
| `<prefix>task:<uuid>` | Hash | handlers.go / runner.go | Yes — `d.cfg.Redis.KeyPrefix + "task:" + taskID` |
| `<prefix>tasks` | Set | handlers.go | Yes — `d.cfg.Redis.KeyPrefix + "tasks"` |
| `<prefix>active` | Set | handlers.go / runner.go / recovery.go | Yes — `d.cfg.Redis.KeyPrefix + "active"` |
| `<prefix>queue:urgent` | Sorted Set | queue.go | Yes — `q.keyPrefix + "queue:urgent"` |
| `<prefix>queue:normal` | Sorted Set | queue.go | Yes — `q.keyPrefix + "queue:normal"` |
| `<prefix>queue:low` | Sorted Set | queue.go | Yes — `q.keyPrefix + "queue:low"` |
| `<prefix>queue:notify` | Pub/Sub channel | queue.go | Yes — `q.keyPrefix + "queue:notify"` |
| `<prefix>lock:task:<uuid>` | String (SET NX EX) | runner.go | Yes — `r.cfg.Redis.KeyPrefix + "lock:task:" + taskID` |
| `<prefix>log:<uuid>` | Stream | logstream.go | Yes — `w.prefix + "log:" + taskID` (where `logKeyPrefix = "log:"`) |
| `atlas:daemon:heartbeat` | String (with TTL) | health.go | **HARDCODED — audit finding 8** |
| `atlas:daemon:state` | Hash | health.go | **HARDCODED — audit finding 8** |
| `atlas:events` | Pub/Sub channel | events.go | **HARDCODED — `defaultEventsChannel`** |

**Hardcoded key audit findings:**

- `heartbeatKey = "atlas:daemon:heartbeat"` in `health.go:19` — ignores `cfg.Redis.KeyPrefix`.
- `daemonStateKey = "atlas:daemon:state"` in `health.go:23` — ignores `cfg.Redis.KeyPrefix`.
- `defaultEventsChannel = "atlas:events"` in `events.go:11` — ignores `cfg.Redis.KeyPrefix`.

These are tracked as audit Finding 8.

---

## 3. Daemon Code Path (atlas daemon start → task executing)

```
atlas daemon start            (internal/cli/daemon.go:runDaemonStart)
  ↓
  re-exec self with --daemon flag  (exec.CommandContext with SysProcAttr for detach)
  ↓
  wait up to 2s polling PingSocket (daemon.go:83–91)
  ↓
  [child process: RunDaemonProcess]  (daemon.go:RunDaemonProcess)
    ↓
    config.Load(ctx)
    workflow.NewDaemonTaskExecutor(cfg, logger)
    daemon.New(cfg, logger, daemon.WithExecutor(executor))
    d.Run(ctx)                       (daemon.go:Run)
      ↓
      d.Start(ctx)                   (daemon.go:Start)
        [step 0] expand ~ in socket/PID/log paths
        [step 1] NewRedisClient(ctx, redisCfg)     — redis.go:NewRedisClient
        [step 2] NewRedisQueue(redis, keyPrefix)   — queue.go
                 NewEventPublisher(redis, "")       — events.go
                 inject LogWriter into executor
        [step 3] d.startServer(ctx)  ← AUDIT FINDING 2: socket starts here, before PID/heartbeat/recovery/pool
          NewRouter → setupRouter (registers all handlers)
          NewServer(socketPath, router) → server.Start(ctx)
          → unix socket bound, acceptLoop goroutine started
        [step 4] d.writePIDFile()
        [step 5] d.startHeartbeat(ctx) goroutine
        [step 6] d.RecoverOrphanedTasks(ctx)   — recovery.go
        [step 7] d.runner = NewRunner(...)
                 d.runner.Start(ctx)  → dispatchLoop goroutine
        [step 8] d.events.Publish(ctx, TaskEvent{Type:"daemon.started"})
      ↓
      wait for SIGTERM/SIGINT or ctx.Done or stopCh
```

**Audit Finding 2 (socket starts before full readiness):** In `daemon.go:Start`, `d.startServer` is step 3. At that point, the PID file has not been written (step 4), the heartbeat is not running (step 5), orphan recovery has not run (step 6), and the worker pool has not started (step 7). A client can submit work before any of those are complete. This is tracked as Finding 2.

Once the socket is bound, the IPC path for a submitted task:

```
client → Unix socket (newline-delimited JSON-RPC)
  ↓
server.acceptLoop → handleConn → serveRequest
  ↓
router.Dispatch(ctx, &req)    — router.go:Dispatch
  ↓
d.handleTaskSubmit(ctx, params)  — handlers.go:handleTaskSubmit
  (see Section 5 for write sequence)
  ↓
queue.Submit(taskID, priority) writes sorted set + publishes notify
  ↓
dispatchLoop (runner.go) wakes on notify channel OR 500ms poll
  ↓
r.queue.Pop(ctx) → SortedSetPopMin on urgent/normal/low
  ↓
r.sem <- struct{}{} (semaphore, default maxParallelTasks = 3)
go r.executeTask(ctx, taskID)
  ↓
cache.WriteLock(taskCtx, redis, lockKey, workerID, lockTTL)
  ↓ (if locked)
r.markTaskRunning(taskCtx, taskID) → hash set status=running, started_at
  ↓
r.loadTaskJob(taskCtx, taskID) → read all fields from hash
  ↓
r.executor.Execute(taskCtx, job)  ← DaemonTaskExecutor.Execute
  (see Section 4 for the direct engine path from here)
```

---

## 4. Direct Code Path (atlas start → task executing)

```
atlas start "description" [flags]   (internal/cli/start.go:runStart)
  ↓
validateStartFlags(opts)             — flag constraint check
  ↓
workflow.FindGitRepository(ctx)      — discovers repo path
  ↓
if !opts.dryRun:
  tryDaemonSubmit(ctx, cmd, w, description, opts, repoPath)
    → config.Load → tryDaemonClient → c.Call("task.submit")
    if daemon is running: returns *error (non-nil pointer → daemon handled it)
    if daemon not running / no config: returns nil → fall through
  ↓  (nil = daemon unavailable; fall through to direct mode)
signal.NewHandler(ctx)
config.Load → template.NewRegistryWithConfig → orchestrator.Prompter.SelectTemplate
  ↓
workflow.NewOrchestrator(logger, out)
  ↓
executeTask(ctx, ...)                (start.go:executeTask)
  ↓
orchestrator.Initializer().CreateWorkspace(ctx, WorkspaceOptions{...})
  → workspace.NewManager.Create → git worktree created
  ↓
startTaskExecution(ctx, ws, tmpl, ...)  (start.go:startTaskExecution)
  ↓
services := workflow.NewServiceFactory(logger).WithRepoPath(ws.RepoPath)
services.SetupTaskStoreAndConfig(ctx)       → task.FileStore + config.Load
services.CreateHookManager(cfg, logger)
services.CreateNotifiers(cfg)
services.CreateAIRunner(cfg)
services.CreateGitServices(ctx, worktreePath, cfg, aiRunner, ...)
services.CreateExecutorRegistry(RegistryDeps{...})
services.CreateEngine(EngineDeps{...}, cfg)  → *task.Engine
  ↓
engine.Start(ctx, wsName, branch, worktreePath, tmpl, description, backlogID)
  ↓
task.Engine.Start                   (task/engine.go — not read in full)
  → domain.Task created in memory
  → task status transitions: pending → running
  → task.FileStore.Create persists engine task JSON
  → template steps executed sequentially via executor registry
```

**Current behavior of `tryDaemonSubmit`:** `atlas start` currently auto-detects a running daemon and submits to it unless `--dry-run` is set. This is the "auto-prefer daemon" behavior that the direct-first design inverts to "direct-first, daemon opt-in."

When the daemon handles the submit, the executor path is:
```
DaemonTaskExecutor.Execute(taskCtx, job)       (workflow/daemon_executor.go:Execute)
  if job.EngineTaskID != "": → resume(ctx, job)
  else: → start(ctx, job)
    provisionWorkspace(ctx, job, wsName, cfg)  → workspace.Manager.Create
    buildEngine(ctx, services, worktreePath, taskStore, cfg, job.TaskID)
    resolveTemplate(job, cfg)
    ApplyAgentModelOverrides / ApplyVerifyOverrides
    eng.Start(ctx, wsName, branch, worktreePath, tmpl, job.Description, "")
```

---

## 5. Submit State Sequence

`handleTaskSubmit` in `internal/daemon/handlers.go` performs the following Redis writes in order:

1. **Hash write** (`handlers.go:150`) — `cache.HashMapSet(ctx, redis, "<prefix>task:<uuid>", pairs)` — writes id, description, template, status="queued", priority, submitted_at, and all optional fields (workspace, branch, repo_path, agent, model, target_branch, use_local, verify, no_verify).

2. **Persistent tasks set add** (`handlers.go:157`) — `cache.SetAdd(ctx, redis, "<prefix>tasks", taskID)` — makes the task visible in listings including after terminal state.

3. **Active set add** (`handlers.go:163`) — `cache.SetAdd(ctx, redis, "<prefix>active", taskID)` — makes the task visible in the active-task count.

4. **Queue submit** (`handlers.go:168`) — `d.queue.Submit(ctx, taskID, priority)` — writes to the sorted set and publishes `<prefix>queue:notify`.

5. **Event publish** (`handlers.go:174`) — `d.events.Publish(ctx, TaskEvent{Type:"task.submitted"})` — non-fatal if Redis is down.

**Audit Finding 3 (Redis-first, filesystem-later):** The task hash, tasks set, active set, and queue entry all exist in Redis before any workspace or engine task file exists on disk. The workspace and engine task file are created by `DaemonTaskExecutor.start` when a worker actually picks up the task. This is architecturally valid (Redis is the queue's source of truth) but the contract must be explicit and documented.

**Audit Finding 4 (partial rollback):** On queue submit failure (step 4), the code rolls back only the active-set entry (`cache.SetRemoveMember(ctx, redis, activeKey, taskID)`). It does NOT roll back the hash (step 1) or the persistent tasks set (step 2). Those orphaned entries will appear in `task.list` responses with status "queued" but with no queue entry. A restart or status check will find a "queued" task that the worker will never pick up. Full rollback is planned (see Finding 4 in Section 12).

---

## 6. Socket / PID / Log File Ownership

### Socket path

- **Derived from:** `cfg.Daemon.SocketPath` (default `~/.atlas/daemon.sock`), `~` expanded via `daemon.ExpandSocketPath` at daemon start (`daemon.go:83`).
- **Created by:** `server.Start(ctx)` in `server.go:Start` — removes any stale socket file with `os.Remove`, then binds with `net.ListenConfig.Listen(ctx, "unix", socketPath)`.
- **Removed by:** `server.Stop()` in `server.go:Stop` — calls `os.Remove(s.socketPath)` as a best-effort cleanup.
- **CLI usage:** `daemon.DialFromConfigContext(ctx, cfg.Daemon.SocketPath)` in `daemon_client.go` and `daemon.go:runDaemonStop/Status/Ping`.

### PID file path

- **Derived from:** `cfg.Daemon.PIDFile` (default `~/.atlas/daemon.pid`), `~` expanded at daemon start (`daemon.go:84`).
- **Written by:** `d.writePIDFile()` in `daemon.go:281` — writes `strconv.Itoa(os.Getpid()) + "\n"` with mode 0o600. Called at step 4 of `Start`, after the socket is bound (see Finding 2).
- **Read by:** `daemon.IsRunning(pidFile)` in `daemon.go:249` — reads PID, uses `os.FindProcess` + `Signal(0)` for liveness check. Used by `newDaemonStartCmd` to detect an already-running daemon.
- **Removed by:** `d.removePIDFile()` in `daemon.go:303` — called during `Stop`.

### Log file path

- **Derived from:** `cfg.Daemon.LogFile` (default `~/.atlas/logs/daemon.log`), `~` expanded at daemon start (`daemon.go:85`).
- **Owned by:** The daemon process — the zerolog logger writes to this file when initialized with `InitLogger` in `cli/daemon.go:RunDaemonProcess`. The CLI parent (`newDaemonStartCmd`) does not read or write this file; it only prints the path in error diagnostics (when implemented).
- **Note:** The child daemon process currently has `daemonCmd.Stdout = nil` and `daemonCmd.Stderr = nil` (`daemon.go:76–77`). Log visibility depends entirely on the daemon having initialized its file logger before any error. If Redis fails before logger init, the error is silently dropped.

### Heartbeat key

- **Written by:** `d.refreshHeartbeat(ctx)` called immediately on start, then every `HeartbeatInterval` (default 10s).
- **Key:** `atlas:daemon:heartbeat` (hardcoded, not prefixed).
- **TTL:** 30 seconds (`heartbeatTTL` constant in `health.go:16`).
- **Content:** The current UTC time in RFC3339 format (a liveness timestamp).
- **State hash:** `atlas:daemon:state` hash holds pid, uptime, version, status="running", updated_at (also hardcoded, also updated on every heartbeat refresh).

---

## 7. UI / Log Streaming

### How `atlas ui` connects

`internal/cli/ui.go:runUI` performs two independent connections:

1. **Daemon socket** — `connectDaemonClient(ctx, cfg)` dials `cfg.Daemon.SocketPath` and sends a `daemon.ping` RPC. Used for task listing, status queries, and interactive actions (approve/reject/pause/resume/abandon/destroy).

2. **Redis** — `connectRedisClient(ctx, cfg)` creates a `cache.Client` and calls `PingRedis`. Used exclusively for log streaming. The `keyPrefix` defaults to `"atlas:"` if `cfg.Redis.KeyPrefix` is empty.

### Dashboard model selection

- Both daemon and Redis available: `dashboard.NewWithCacheClient(daemonClient, redisClient)` + `model.SetCacheClientWithPrefix(redisClient, keyPrefix)` — full-featured UI.
- Daemon only, Redis unavailable: `dashboard.NewWithClient(daemonClient)` — task monitoring works; log streaming disabled. A startup error banner is shown: `"Redis unavailable — log streaming disabled"`.
- Neither available: `dashboard.New()` — empty dashboard with startup error showing `atlas daemon start` guidance.

### What happens when Redis is down

UI/log streaming degrades gracefully (accepted design decision; see Decision D5 in Section 11). Task monitoring and all interactive actions still work via the daemon socket. Log streaming is silent. A banner is displayed to the user. This is the intended degraded behavior; no additional Redis resilience is planned.

### Redis data types backing log streaming

- **Log storage:** Redis Stream at `<prefix>log:<uuid>`. Entries written by `LogWriter.Write` (`logstream.go:Write`) via `cache.StreamAddCapped` with MAXLEN cap.
- **Log reading:** `LogReader.Read` uses `cache.StreamRead` (non-blocking). `LogReader.Tail` uses `cache.StreamReadBlock` (blocking, for live streaming).
- **Events:** Redis Pub/Sub on `atlas:events`. `EventSubscriber.Start` (subscriber.go) calls `cache.Subscribe` and forwards deserialized `TaskEvent` JSON to a typed channel.

### subscriber.go / logstream.go roles

- `subscriber.go` — `EventSubscriber` wraps a `cache.Subscription` on `atlas:events`, deserializes `TaskEvent` JSON, and delivers events on a typed channel. Used by the dashboard TUI to receive task state changes in real time.
- `logstream.go` — `LogWriter` appends structured `LogEntry` JSON to a Redis Stream. `LogReader` reads entries non-blocking or blocking. The `DaemonTaskExecutor` injects a `LogWriter` via `SetLogWriter`; the executor wraps it in `logStreamWriter` (an `io.Writer` adapter) to forward engine progress and validation output to Redis.

---

## 8. Recovery Model

`internal/daemon/recovery.go:RecoverOrphanedTasks` is called once at daemon startup, after the worker pool starts (daemon.go step 6 of Start, though before the socket is up — see Section 3).

### What it scans

1. Reads all members of `<prefix>active` (the active set).
2. For each member, validates it is a UUID (rejects malformed IDs to prevent Redis key injection).
3. Reads the task hash fields: `status`, `retry_count`, `priority`.

### Decision logic per task

| Condition | Decision |
|---|---|
| `status != "running"` | Skip — task is in a non-orphaned state (queued, paused, awaiting_approval, completed, failed, etc.) |
| `status == "running"` AND lock key `<prefix>lock:task:<uuid>` exists | Skip — worker is still alive |
| `status == "running"` AND lock key does not exist AND `retry_count >= 3 (maxRetryCount)` | Mark `status=failed`, set `error="max retries exceeded"` |
| `status == "running"` AND lock key does not exist AND `retry_count < 3` | Increment `retry_count`, reset `status=queued`, re-submit to queue at original priority |

### What it does NOT reconcile

- **Filesystem task files:** `RecoverOrphanedTasks` does not read `~/.atlas/tasks/*/` or `~/.atlas/workspaces/*/`. If the Redis active set says a task was running but no corresponding filesystem engine task JSON exists (e.g., the worktree was never created), the task is re-queued. The next execution attempt will try to provision a workspace again.
- **Tasks not in the active set:** Terminal-state tasks (completed, failed, canceled, abandoned) are not in `<prefix>active` and are therefore never inspected by recovery.
- **Partial submit state:** Orphaned task hashes left by a failed submit (Finding 4) with status="queued" and no queue entry are not detected; they show up in `task.list` but are never executed and are never cleaned up.
- **Hook/checkpoint state:** Recovery does not inspect `~/.atlas/hooks/`. Hook continuity across daemon restart is not reconciled.

**Audit Finding 9 (Redis-centric recovery):** This is by design — Redis is the daemon source-of-truth. However, no reconciliation path exists to detect Redis ↔ filesystem drift. Planned reconciliation tooling (`atlas daemon status --reconcile`) will surface drift.

---

## 9. Release Flow

### How `fortress-release.yml` triggers

The workflow is a **reusable workflow** (`workflow_call`), called from the orchestrator (`fortress.yml`). It is triggered when the orchestrator runs after a semantic-version tag push. The workflow:

1. Validates the tag matches semver pattern `v[0-9]+.[0-9]+.[0-9]+` (with optional pre-release suffix).
2. Sets up Go and caching.
3. Sets up `magex` (MAGE-X) and GoReleaser.
4. Validates GoReleaser config (`magex release:validate`).
5. Runs `magex release` (or `magex release godocs` if `ENABLE_GODOCS_PUBLISHING=true`) which invokes GoReleaser.

### What it builds

GoReleaser is configured in `.goreleaser.yml`. The `magex` build uses `cmd/atlas/main.go` as the entry point with ldflags injecting `-X main.version={{.Version}}`, `-X main.commit={{.Commit}}`, `-X main.buildDate={{.Date}}`.

### Where the version constant lives

`cmd/atlas/main.go:16`:
```go
version = "dev"
```
This is a package-level variable, not a `const`. GoReleaser's ldflags inject the actual version string at build time via `-X main.version=...`. At runtime `daemonVersion` in `health.go:25` is also hardcoded to `"dev"` and is not injected — it reports `"dev"` in all environments including production builds. That is a separate issue.

**Current released tag:** `v0.20.10` (as of 2026-05-28).

---

## 10. magex test:race

**Confirmed: the target `test:race` exists.**

From `.github/workflows/fortress-setup-config.yml:426`, the expected magex targets include `"test:race"` in the target list. From `fortress-test-matrix.yml:254–263`, when `race-detection-enabled=true` and `code-coverage-enabled=false`, the matrix invokes `magex test:race` with a `TEST_TIMEOUT` (default 30m).

The `.mage.yaml` file specifies `timeout: 30m` for the `test` target (the base timeout that race variants inherit). The actual magex command dispatched is `magex test:race -timeout <TEST_TIMEOUT>`.

**`magex test:race` confirmed:** it runs the test suite with the `-race` flag. A documented fallback is `go test -race ./...` if magex is unavailable.

---

## 11. Design Decisions (D1–D9)

These design decisions must not be reversed without project-owner sign-off.

- **D1 (direct-first default):** `atlas start` defaults to direct (foreground) mode. Daemon mode is opt-in via `--daemon` flag or `config.Daemon.Default = true`. The current auto-prefer-daemon behavior (`tryDaemonSubmit` at `start.go:213`) is inverted to direct-first (see Section 4).

- **D2 (worktree-path exclusivity):** Exclusivity locking is keyed on the canonical absolute worktree path, not on the upstream git origin URL. Daemon mode uses a Redis lock `<prefix>lock:worktree:<sha256(abs_path)>`. Direct mode uses a filesystem lock. Multiple worktrees of the same origin can run in parallel.

- **D3 (Redis stays):** Redis remains as the daemon queue backend. No embedded alternative (e.g., SQLite, BoltDB) is introduced in this design. Redis is an explicit daemon-mode prerequisite with first-class diagnostics.

- **D4 (auto-resume + verbose recovery):** On daemon restart, orphaned tasks are automatically re-queued (up to `maxRetryCount = 3`). `RecoverOrphanedTasks` emits a structured per-task recovery event to daemon logs and `atlas daemon status` output.

- **D5 (UI/log streaming may degrade):** When Redis is unavailable, `atlas ui` shows a degraded banner but task monitoring via the daemon socket continues. No additional Redis resilience work is in scope.

- **D6 (hook failure = degraded state, not hard failure):** When `CreateHook` or `ReadyHook` fails, the daemon submit succeeds and the task hash gets `crash_recovery: degraded`. This is visible in `atlas status` and `atlas daemon status`. A `atlas hook retry <id>` command re-attempts init.

- **D7 (submit partial failure = full rollback):** On any Redis write failure during `handleTaskSubmit`, all prior writes are rolled back. The intended post-rollback state is identical to pre-submit state (see Finding 4).

- **D8 (direct mode permanently first-class):** Direct mode is not a temporary fallback. It remains a supported, tested, production execution path for all future Atlas versions. It is not deprecated.

---

## 12. Audit Finding Map

Map of all 16 pre-existing audit findings and their remediation status.

| Finding | Description (brief) | Remediation |
|---|---|---|
| 1 | Daemon child fails silently — parent prints "not yet responding" instead of real error | Readiness contract + child error surfacing |
| 2 | Socket starts before full readiness (PID/heartbeat/recovery/pool not yet done) | Reorder Start to: Redis → PID → heartbeat → recovery → pool → socket |
| 3 | Submit is Redis-first, filesystem-later (workspace/engine task created later by executor) | Explicit transactional contract; documented in Section 5 |
| 4 | Submit rollback is partial — only active-set rolled back; hash and tasks-set orphaned | Full rollback with defer-rollback pattern |
| 5 | Queue comment says nanosecond, code uses `UnixMicro()` — FIFO stability unclear | Pick nanosecond precision; update code and tests |
| 6 | `Queue.MaxSize` config field not enforced in `RedisQueue.Submit` | Enforce or remove with `ErrQueueFull` |
| 7 | Cancel/abandon may leave queue entries until popped; status/queue-depth UX confusion | Clarify UX contract; verify queue depth does not count canceled tasks as actionable |
| 8 | `heartbeatKey` and `daemonStateKey` hardcoded as `atlas:*`, not using `cfg.Redis.KeyPrefix` | Thread prefix through health.go |
| 9 | Recovery is Redis-centric; does not reconcile from filesystem task/workspace state | `atlas daemon status --reconcile` detects Redis ↔ filesystem drift |
| 10 | `DaemonTaskExecutor` is heavy and requires real AI/GitHub/git for any test | Testfakes harness (fake AI runner, fake git, miniredis) |
| 11 | `atlas ui` intentionally degrades when Redis is down; needs explicit tests and user copy | Degraded UI banner tested explicitly |
| 12 | Engine persistence ordering: `store.Create` may fail after in-memory task is `running` | Characterization tests; lock down intended behavior |
| 13 | Hooks/checkpoints are best-effort; `CreateHook`/`ReadyHook` failure is silent warning | Degrade to `crash_recovery: degraded` state visible in status |
| 14 | Hook checkpoint loop is in-memory + task-path keyed; daemon restart behavior unverified | Add tests for checkpoint loop stop/start on restart |
| 15 | Direct workflow `StartTask` returns partial task on failure; may not be resumable | Lifecycle vocabulary unification; CLI preserves task ID in error output |
| 16 | Local checkout drift is a process risk before implementation | Create fresh worktree from `origin/master`; already done |
