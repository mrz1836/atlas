# Traceability Matrix & Gap Analysis - Epic 6 User Scenarios

**Epic:** Epic 6 - Git & PR Automation (FR34-FR40)
**Date:** 2025-12-30
**Evaluator:** BMad Master / TEA Agent
**Source Document:** `epic-6-user-scenarios.md`

---

**Note:** This analysis maps user scenario steps to implementing Go code. Gaps identified require new stories before scenarios are achievable.

## PHASE 1: REQUIREMENTS TRACEABILITY

### Coverage Summary

| Priority  | Total Steps | FULL Coverage | Coverage % | Status       |
| --------- | ----------- | ------------- | ---------- | ------------ |
| P0        | 8           | 3             | 37%        | ❌ FAIL      |
| P1        | 12          | 8             | 67%        | ⚠️ WARN      |
| P2        | 6           | 6             | 100%       | ✅ PASS      |
| **Total** | **26**      | **17**        | **65%**    | ⚠️ WARN      |

**Legend:**

- ✅ PASS - Code fully implements scenario step
- ⚠️ WARN - Code exists but not wired into step executor
- ❌ FAIL - Placeholder or missing implementation

---

## Scenario 1: Bugfix Workflow (Stories 6.3-6.7)

### Step-by-Step Traceability

#### Step 3: Workspace & Worktree Creation (P1)

- **Coverage:** FULL ✅
- **Implementation:**
  - `internal/workspace/manager.go:61-119` - `Create()` method
  - `internal/workspace/worktree.go` - Git worktree operations
  - `internal/workspace/store.go` - State persistence

#### Step 4: Analyze (AI Step) (P1)

- **Coverage:** FULL ✅
- **Implementation:**
  - `internal/template/steps/ai.go` - AIExecutor
  - `internal/ai/claude.go` - ClaudeCodeRunner
  - `internal/template/bugfix.go:19-30` - analyze step definition

#### Step 5: Implement (AI Step) (P1)

- **Coverage:** FULL ✅
- **Implementation:**
  - `internal/template/steps/ai.go` - AIExecutor
  - `internal/ai/claude.go` - ClaudeCodeRunner
  - `internal/template/bugfix.go:31-42` - implement step definition

#### Step 6: AI Verification (Optional) (P1)

- **Coverage:** NONE ❌ **[GAP 5]**
- **Implementation:** NOT IMPLEMENTED
- **Required:**
  - Cross-model verification executor
  - Configurable via `--verify` flag
  - `verification-report.md` artifact
- **Recommendation:** Create Story 6.8: "Implement AI Verification Step"

#### Step 7: Validate (P0)

- **Coverage:** FULL ✅
- **Implementation:**
  - `internal/template/steps/validation.go` - ValidationExecutor (fully wired)
  - `internal/validation/runner.go` - Parallel pipeline execution
  - `internal/validation/executor.go` - Command execution
  - `internal/validation/handler.go` - Result handling with bell notification

#### Step 8: Smart Commit (P0)

- **Coverage:** PLACEHOLDER ❌ **[GAP 1 + GAP 4]**
- **Implementation Exists (NOT WIRED):**
  - `internal/git/garbage.go` - GarbageScanner ✅
  - `internal/git/smart_commit.go` - SmartCommitter with grouping ✅
  - `internal/git/pr_description.go` - Commit message generation ✅
- **Placeholder:**
  - `internal/template/steps/git.go:26-27` - "placeholder implementation for Epic 4"
- **Required:**
  1. Wire GitExecutor to call `GarbageScanner.Scan()` before commit
  2. Wire GitExecutor to call `SmartCommitter.Commit()`
- **Recommendation:** Create Story 6.9: "Wire GitExecutor to internal/git package"

#### Step 9: Push (P0)

- **Coverage:** PLACEHOLDER ❌ **[GAP 1]**
- **Implementation Exists (NOT WIRED):**
  - `internal/git/push.go` - Pusher with exponential backoff (3 retries) ✅
- **Placeholder:**
  - `internal/template/steps/git.go:78-79` - Returns fake success
- **Required:** Wire GitExecutor to call `Pusher.Push()`

#### Step 10: Create PR (P0)

- **Coverage:** PLACEHOLDER ❌ **[GAP 1]**
- **Implementation Exists (NOT WIRED):**
  - `internal/git/github.go` - HubRunner.CreatePR() ✅
  - `internal/git/pr_description.go` - PRDescriptionGenerator ✅
- **Placeholder:**
  - `internal/template/steps/git.go:82-83` - Returns fake success
- **Required:** Wire GitExecutor to call `PRDescriptionGenerator` then `HubRunner.CreatePR()`

#### Step 11: CI Wait (P0)

- **Coverage:** PLACEHOLDER ❌ **[GAP 2]**
- **Implementation Exists (NOT WIRED):**
  - `internal/git/github.go:200-350` - `WatchPRChecks()` with polling ✅
  - Configurable poll interval and timeout ✅
  - Bell notification on completion ✅
- **Placeholder:**
  - `internal/template/steps/ci.go:22-24` - "placeholder implementation for Epic 4"
  - Fake polling loop returns success after 3 iterations
- **Required:** Wire CIExecutor to call `HubRunner.WatchPRChecks()`
- **Recommendation:** Create Story 6.10: "Wire CIExecutor to HubRunner.WatchPRChecks"

#### Step 12: Human Review - Final (P1)

- **Coverage:** PARTIAL ⚠️
- **Implementation:**
  - `internal/template/steps/human.go` - Returns `awaiting_approval` status ✅
  - `internal/tui/ci_failure_menu.go` - Menu component exists ✅
- **Gap:**
  - HumanExecutor doesn't display interactive menu (Epic 8 scope)
  - For Epic 6, text-based prompt is acceptable
- **Note:** Full TUI integration is Epic 8 (Story 8.1, 8.5)

---

## Scenario 2: Feature Workflow with Garbage Detection (Story 6.3)

#### Garbage Detection Step (P0)

- **Coverage:** IMPLEMENTED BUT NOT WIRED ⚠️ **[GAP 4]**
- **Implementation Exists:**
  - `internal/git/garbage.go:27-75` - `GarbageScanner.Scan()` ✅
  - Detects: debug files, secrets, build artifacts, temp files ✅
  - Returns `GarbageResult` with categories and file list ✅
- **Gap:**
  - `SmartCommitter` does NOT call `GarbageScanner` before committing
  - No user prompt for garbage removal
- **Required:** Wire garbage detection into smart commit flow
- **Recommendation:** Part of Story 6.9

---

## Scenario 3: PR Creation with Rate Limit (Stories 6.5, 6.7)

#### Rate Limit Handling (P1)

- **Coverage:** FULL ✅ (in git package, not wired to executor)
- **Implementation:**
  - `internal/git/github.go` - Retry with exponential backoff ✅
  - `internal/git/push.go` - 3 retries with backoff ✅
  - Error classification (auth, network, timeout) ✅

#### CI Failure Menu (P1)

- **Coverage:** IMPLEMENTED BUT NOT WIRED ⚠️ **[GAP 3]**
- **Implementation Exists:**
  - `internal/task/ci_failure.go` - CIFailureHandler with 4 actions ✅
  - `internal/tui/ci_failure_menu.go` - Menu rendering ✅
- **Gap:**
  - Task engine doesn't call CIFailureHandler when CI fails
  - No transition to present user options
- **Required:** Wire CIFailureHandler into task engine state machine
- **Recommendation:** Create Story 6.11: "Integrate CIFailureHandler into task engine"

---

## Scenario 4: Multi-File Logical Grouping (Story 6.3)

#### File Grouping (P1)

- **Coverage:** FULL ✅ (in git package, not wired to executor)
- **Implementation:**
  - `internal/git/smart_commit.go:100-180` - `groupFilesByPackage()` ✅
  - Groups files by Go package ✅
  - Generates separate commits per group ✅
  - Adds ATLAS trailers ✅
- **Note:** Wiring needed via Story 6.9

---

## Scenario 5: Feature Workflow with Speckit SDD

#### SDD Steps (specify, plan, tasks, implement, checklist) (P1)

- **Coverage:** FULL ✅
- **Implementation:**
  - `internal/template/steps/sdd.go` - SDDExecutor ✅
  - Invokes Speckit slash commands via Claude Code ✅
  - Saves artifacts with versioning ✅
  - `internal/template/feature.go` - Step definitions ✅

#### Human Review Checkpoints (P2)

- **Coverage:** PARTIAL ⚠️
- **Implementation:**
  - `internal/template/steps/human.go` - Basic awaiting_approval ✅
- **Note:** Full interactive review is Epic 8 scope

---

## Gap Analysis

### Critical Gaps (BLOCKER) ❌

**5 gaps found. Create stories before scenarios are achievable.**

#### GAP 1: GitExecutor Placeholder (P0)

- **Affected Scenarios:** 1, 2, 3, 4
- **Affected Steps:** Smart Commit, Push, Create PR
- **Current State:** `internal/template/steps/git.go` returns fake success
- **Code Exists:** `internal/git/` package fully implemented
- **Required:** Wire executor to git package functions
- **Story:** 6.9 - "Wire GitExecutor to internal/git package"

#### GAP 2: CIExecutor Placeholder (P0)

- **Affected Scenarios:** 1, 3, 5
- **Affected Steps:** CI Wait
- **Current State:** `internal/template/steps/ci.go` fake polling loop
- **Code Exists:** `HubRunner.WatchPRChecks()` fully implemented
- **Required:** Wire executor to call WatchPRChecks
- **Story:** 6.10 - "Wire CIExecutor to HubRunner.WatchPRChecks"

#### GAP 3: CIFailureHandler Not Wired (P0)

- **Affected Scenarios:** 1, 3
- **Affected Steps:** CI failure handling
- **Current State:** Handler exists but task engine doesn't call it
- **Code Exists:** `CIFailureHandler` with all 4 actions
- **Required:** Integrate into task engine state machine
- **Story:** 6.11 - "Integrate CIFailureHandler into task engine"

#### GAP 4: Garbage Detection Not Wired (P1)

- **Affected Scenarios:** 1, 2
- **Affected Steps:** Smart Commit with garbage warning
- **Current State:** `GarbageScanner` exists but not called
- **Code Exists:** `GarbageScanner.Scan()` fully implemented
- **Required:** Call before `SmartCommitter.Commit()`
- **Story:** Part of 6.9 (or separate 6.12)

#### GAP 5: AI Verification Step Missing (P1)

- **Affected Scenarios:** 1, 5
- **Affected Steps:** Step 6 (if --verify), Step 13 in Scenario 5
- **Current State:** Not implemented
- **Code Exists:** None
- **Required:** New cross-model verification executor
- **Story:** 6.8 - "Implement AI Verification Step"

---

### High Priority Gaps (PR BLOCKER) ⚠️

*See Critical Gaps above - all are P0/P1*

---

### Medium Priority Gaps (Epic 7/8 Scope) ℹ️

1. **HumanExecutor TUI Integration** - Epic 8 Story 8.1, 8.5
2. **Interactive Error Recovery Menus** - Epic 8 Story 8.5
3. **Progress Dashboard** - Epic 7 Story 7.8

---

## Coverage by Component

| Component | Status | Notes |
|-----------|--------|-------|
| Workspace Manager | ✅ FULL | Create, retire, destroy working |
| Task Engine | ✅ FULL | State machine, step orchestration |
| AIExecutor | ✅ FULL | Claude Code integration |
| ValidationExecutor | ✅ FULL | Parallel pipeline, retry, artifacts |
| SDDExecutor | ✅ FULL | Speckit slash commands |
| **GitExecutor** | ❌ PLACEHOLDER | Needs wiring (GAP 1, 4) |
| **CIExecutor** | ❌ PLACEHOLDER | Needs wiring (GAP 2) |
| HumanExecutor | ⚠️ PARTIAL | Basic, TUI in Epic 8 |
| GarbageScanner | ✅ FULL | In git/, not wired |
| SmartCommitter | ✅ FULL | In git/, not wired |
| Pusher | ✅ FULL | In git/, not wired |
| HubRunner | ✅ FULL | In git/, not wired |
| CIFailureHandler | ✅ FULL | Not integrated (GAP 3) |
| AI Verification | ❌ MISSING | New implementation (GAP 5) |

---

## PHASE 2: GATE DECISION

**Gate Type:** Epic
**Decision Mode:** Deterministic

---

### Decision Criteria Evaluation

#### P0 Criteria

| Criterion | Threshold | Actual | Status |
|-----------|-----------|--------|--------|
| GitExecutor Wired | Required | PLACEHOLDER | ❌ FAIL |
| CIExecutor Wired | Required | PLACEHOLDER | ❌ FAIL |
| CIFailureHandler Integrated | Required | NOT WIRED | ❌ FAIL |

**P0 Evaluation:** ❌ 0/3 PASS

#### P1 Criteria

| Criterion | Threshold | Actual | Status |
|-----------|-----------|--------|--------|
| Garbage Detection Wired | Required | NOT WIRED | ⚠️ WARN |
| AI Verification Implemented | Required | MISSING | ⚠️ WARN |
| Underlying git/ Package | Complete | 100% | ✅ PASS |
| Step Executor Framework | Complete | 100% | ✅ PASS |

**P1 Evaluation:** ⚠️ 2/4 PASS

---

### GATE DECISION: ❌ FAIL

---

### Rationale

**Why FAIL:**

The Epic 6 User Scenarios are **NOT achievable** with the current codebase due to 5 critical gaps:

1. **GitExecutor is a placeholder** - Returns fake success without calling any git operations
2. **CIExecutor is a placeholder** - Fake polling loop doesn't call GitHub Actions API
3. **CIFailureHandler not integrated** - Exists but task engine doesn't use it
4. **Garbage detection not wired** - Scanner exists but not called before commit
5. **AI Verification not implemented** - Required by scenarios but missing

**Why NOT CONCERNS:**

The gaps are not minor threshold misses - they are **fundamental missing wiring** that makes core scenario steps non-functional.

**Positive Findings:**

The `internal/git/` package is **excellently implemented** with:
- Garbage detection with categorization
- Smart commit with package grouping and ATLAS trailers
- Push with exponential backoff and retry
- PR creation via gh CLI
- CI status monitoring with configurable polling
- CI failure handling with 4 action options

The gap is purely **wiring** - connecting step executors to existing implementations.

---

### Required Stories to Achieve PASS

| Story ID | Title | Priority | Effort |
|----------|-------|----------|--------|
| 6.8 | Implement AI Verification Step | P1 | Medium |
| 6.9 | Wire GitExecutor to internal/git package | P0 | Small |
| 6.10 | Wire CIExecutor to HubRunner.WatchPRChecks | P0 | Small |
| 6.11 | Integrate CIFailureHandler into task engine | P0 | Small |

**Note:** GAP 4 (Garbage Detection) can be included in Story 6.9 or as separate Story 6.12.

---

### Next Steps

**Immediate Actions:**

1. Create Stories 6.8, 6.9, 6.10, 6.11 in sprint backlog
2. Prioritize 6.9, 6.10, 6.11 as P0 (blocking)
3. Add 6.8 as P1 (required for full scenario coverage)

**Implementation Order:**

1. Story 6.9 (GitExecutor wiring) - Unblocks Scenarios 1, 2, 4
2. Story 6.10 (CIExecutor wiring) - Unblocks CI monitoring
3. Story 6.11 (CIFailureHandler) - Unblocks CI failure handling
4. Story 6.8 (AI Verification) - Completes optional verification step

**After Stories Complete:**

- Re-run `testarch-trace` workflow
- Verify gate decision is PASS
- Proceed to Epic 7 (Status Dashboard)

---

## Integrated YAML Snippet (CI/CD)

```yaml
epic_6_traceability:
  story_id: "epic-6"
  date: "2025-12-30"
  coverage:
    overall: 65%
    p0: 37%
    p1: 67%
    p2: 100%
  gaps:
    critical: 5
    high: 0
    medium: 3
  gate_decision:
    decision: "FAIL"
    gate_type: "epic"
    blocking_issues:
      - "GAP-1: GitExecutor placeholder"
      - "GAP-2: CIExecutor placeholder"
      - "GAP-3: CIFailureHandler not wired"
      - "GAP-4: Garbage detection not wired"
      - "GAP-5: AI Verification missing"
  required_stories:
    - id: "6.8"
      title: "Implement AI Verification Step"
      priority: "P1"
    - id: "6.9"
      title: "Wire GitExecutor to internal/git package"
      priority: "P0"
    - id: "6.10"
      title: "Wire CIExecutor to HubRunner.WatchPRChecks"
      priority: "P0"
    - id: "6.11"
      title: "Integrate CIFailureHandler into task engine"
      priority: "P0"
```

---

## Related Artifacts

- **User Scenarios:** `_bmad-output/implementation-artifacts/epic-6-user-scenarios.md`
- **Epic Definition:** `_bmad-output/planning-artifacts/epics.md` (Epic 6: Git & PR Automation)
- **Git Package:** `internal/git/`
- **Step Executors:** `internal/template/steps/`
- **Task Engine:** `internal/task/`

---

## Sign-Off

**Traceability Assessment:**

- Overall Coverage: 65%
- P0 Coverage: 37% ❌ FAIL
- P1 Coverage: 67% ⚠️ WARN
- Critical Gaps: 5

**Gate Decision:** ❌ FAIL

**Overall Status:** Scenarios NOT achievable until stories 6.8-6.11 are implemented.

**Generated:** 2025-12-30
**Workflow:** testarch-trace (adapted for scenario-to-code traceability)

---

<!-- Powered by BMAD-CORE™ -->
