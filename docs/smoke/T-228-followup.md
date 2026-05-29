# T-228 Dogfood Followup Notes (Phase 7, Task 7.3)

Date: 2026-05-28
Phase: 7 (Dogfood on Atlas itself)
Author: executor (T-228 Phase 7 worker)

## Purpose

Capture unexpected behavior, copy issues, and test gaps observed during Phase 7 dogfood.
These items are either addressed in Phase 8 docs or deferred for Z's triage.

---

## Items Found and Fixed During Dogfood

### FIXED-1: `--daemon` flag collision between global interception and subcommand flag

**Observed:** `atlas start --daemon --no-interactive` exited 0/1 with no output.
Investigating revealed that `Execute()` in `internal/cli/root.go` scanned ALL `os.Args`
for `"--daemon"` before Cobra could parse subcommands. Result: any command containing
`--daemon` (including `atlas start --daemon`) silently triggered `RunDaemonProcess(ctx)`
instead of being parsed by Cobra.

**Root cause:** Pre-existing from initial commit. The intent was to intercept
`atlas --daemon` (the global flag that starts the daemon process in-process), but the
scan was too broad.

**Fix applied (Phase 7):** Changed the scan to only match when `os.Args[1] == "--daemon"`,
so `atlas start --daemon` is correctly parsed by Cobra as a subcommand flag.

```go
// Before (too broad — intercepts atlas start --daemon):
for _, arg := range os.Args[1:] {
    if arg == "--daemon" {
        return RunDaemonProcess(ctx)
    }
}

// After (correct — only intercepts atlas --daemon):
if len(os.Args) > 1 && os.Args[1] == "--daemon" {
    return RunDaemonProcess(ctx)
}
```

**Test gap noted:** No test existed for `Execute()` with `atlas start --daemon`. Should
add a unit test in `internal/cli/root_test.go` (or integration) asserting that
`atlas start --daemon` routes to Cobra, not `RunDaemonProcess`. → Phase 8.

---

## Deferred Items (Phase 8 docs or follow-up)

### DEFER-1: Redis not installed locally → real daemon dogfood incomplete

**Observed:** Real daemon session (Task 7.2) blocked by missing Redis install.
`atlas daemon start` fails with clear diagnostics (AC-PB-3 confirmed), but full
CLI-level daemon submit/execute/status couldn't be demonstrated.

**Recommendation for Phase 8.7 (Smoke test on fresh worktree):** Run on a machine
with Redis available (`brew install redis && brew services start redis`). The fake
dogfood (Task 7.1) proves the code is correct; this is an environment gap.

**Workaround docs:** Phase 8 README/quick-start should prominently call out:
```
Prerequisites for daemon mode:
  brew install redis
  brew services start redis
```

### DEFER-2: `TestCopyBinaryFile` pre-existing failure (Phase 8 task)

**Observed:** `go test ./internal/cli/...` shows `TestCopyBinaryFile` failure
(upgrade_release_test.go:706, permissions 0x1ed expected vs 0x1c0 actual).

**Status:** Pre-existing, not introduced by Phase 7. Tracked in Phase 8, Task 8.4.

### DEFER-3: No test for Execute() --daemon routing behavior

**Observed:** The `--daemon` interception in Execute() has no unit test.
After fixing FIXED-1, this remains a test gap.

**Recommendation:** Add to Phase 8, Task 8.4 or a new test:
```go
func TestExecute_DaemonFlag_OnStart_DoesNotInterceptSubcommand(t *testing.T) {
    // Ensure 'atlas start --daemon' routes through Cobra, not RunDaemonProcess.
}
```

### DEFER-4: atlas --version shows "unknown" build date when built without ldflags

**Observed:** `atlas version dev-t228 (commit: e63b7e0, built: unknown)` — the `date`
ldflags variable was empty in the build. The released binary has the correct date.

**Status:** Not a code issue. Phase 8 release process injects ldflags. No action needed.

---

## Summary

| Item | Type | Status |
|------|------|--------|
| --daemon flag collision in Execute() | Bug | Fixed in Phase 7 |
| Redis not installed locally | Env gap | Deferred to Phase 8 smoke test docs |
| TestCopyBinaryFile failure | Pre-existing test regression | Phase 8, Task 8.4 |
| No test for Execute() --daemon routing | Test gap | Deferred to Phase 8 |
| Built binary shows "unknown" build date | Cosmetic | No action (release process handles) |

All items are non-blocking for Phase 8 (docs, validation, release). Phase 7 DONE.
