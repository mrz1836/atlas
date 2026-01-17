# Tasks: Hook System for Crash Recovery & Context Persistence

**Input**: Design documents from `/specs/002-hook-system-mvp/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**Tests**: Testing is REQUIRED per spec.md (TR-001 through TR-010). Tests are included for each component.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Project type**: Go CLI extending existing ATLAS codebase
- **Paths**: `internal/` for packages, `~/.atlas/` for runtime data

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, dependencies, and constants

- [X] T001 Add `github.com/bsv-blockchain/go-sdk` dependency to go.mod
- [X] T002 Create hook constants in internal/constants/hook.go (HookFileName, HookMarkdownFileName, HookSchemaVersion, MaxLastOutputLength)
- [X] T003 [P] Create HookConfig, KeyDerivationConfig, RetentionConfig structs in internal/config/config.go
- [X] T004 [P] Create test fixtures directory structure at internal/hook/testdata/hooks/

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Domain types and core infrastructure that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Domain Types (internal/domain/hook.go)

- [X] T005 Define HookState enum with all 9 states (initializing, step_pending, step_running, step_validating, awaiting_human, recovering, completed, failed, abandoned) in internal/domain/hook.go
- [X] T006 Define CheckpointTrigger enum with all 7 triggers (manual, git_commit, git_push, pr_created, validation, step_complete, interval) in internal/domain/hook.go
- [X] T007 Define StepContext struct in internal/domain/hook.go
- [X] T008 Define StepCheckpoint struct with FileSnapshot embedded in internal/domain/hook.go
- [X] T009 Define FileSnapshot struct in internal/domain/hook.go
- [X] T010 Define HookEvent struct in internal/domain/hook.go
- [X] T011 Define RecoveryContext struct in internal/domain/hook.go
- [X] T012 Define ValidationReceipt struct in internal/domain/hook.go
- [X] T013 Define Hook struct (primary entity) in internal/domain/hook.go
- [X] T014 Define ValidHookTransitions map in internal/domain/hook.go
- [X] T015 Add IsTerminalState helper function in internal/domain/hook.go
- [X] T016 [P] Write domain type tests (serialization round-trip, enum validation) in internal/domain/hook_test.go

### Shared Crypto Interfaces (internal/crypto/)

- [X] T017a Define Signer interface (Sign, Verify methods) in internal/crypto/signer.go
- [X] T017b Define Verifier interface (Verify method only, for read-only consumers) in internal/crypto/signer.go
- [X] T017c [P] Write crypto interface contract tests in internal/crypto/signer_test.go

### Hook-Specific Interfaces (internal/hook/)

- [X] T017 Define ReceiptSigner interface (embeds crypto.Signer, adds KeyPath) in internal/hook/signer.go
- [X] T018 Define KeyManager interface in internal/hook/signer.go
- [X] T019 [P] Create MockSigner implementation for testing in internal/hook/signer_mock.go
- [X] T020 [P] Write signer interface contract tests in internal/hook/signer_test.go

### Hook Store (internal/hook/store.go)

- [X] T021 Define HookStore interface in internal/hook/store.go
- [X] T022 Implement FileStore struct with atomic write pattern (temp+rename) in internal/hook/store.go
- [X] T023 Implement FileStore.Create method in internal/hook/store.go
- [X] T024 Implement FileStore.Get method in internal/hook/store.go
- [X] T025 Implement FileStore.Save method with file locking in internal/hook/store.go
- [X] T026 Implement FileStore.Delete method in internal/hook/store.go
- [X] T027 Implement FileStore.Exists method in internal/hook/store.go
- [X] T028 Implement FileStore.ListStale method in internal/hook/store.go
- [X] T029 Write store tests (CRUD, atomic writes, concurrent access, corrupted file handling) in internal/hook/store_test.go

### State Machine (internal/hook/state.go)

- [X] T030 Implement HookTransitioner with Transition method in internal/hook/state.go
- [X] T031 Implement IsValidTransition method in internal/hook/state.go
- [X] T032 Add helper to append HookEvent to history on transition in internal/hook/state.go
- [X] T033 Write state machine tests (all valid transitions, all invalid transitions, terminal state rejection, history append) in internal/hook/state_test.go

### Test Fixtures

- [X] T034 [P] Create valid_initializing.json fixture in internal/hook/testdata/hooks/
- [X] T035 [P] Create valid_step_running.json fixture in internal/hook/testdata/hooks/
- [X] T036 [P] Create valid_completed.json fixture in internal/hook/testdata/hooks/
- [X] T037 [P] Create with_checkpoints.json fixture in internal/hook/testdata/hooks/
- [X] T038 [P] Create corrupted.json fixture in internal/hook/testdata/hooks/
- [X] T039 [P] Create stale.json fixture in internal/hook/testdata/hooks/

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Resume After Crash (Priority: P1) 🎯 MVP

**Goal**: Enable developers to resume interrupted tasks without losing work or repeating completed steps

**Independent Test**: Start a multi-step task, simulate a crash mid-step, verify AI resumes from correct position with accurate context

### Tests for User Story 1

- [X] T040 [US1] Write recovery detection tests (stale hook detection, terminal state exclusion) in internal/hook/recovery_test.go
- [X] T041 [US1] Write recovery recommendation tests (retry_step for idempotent, manual for implement, retry_from_checkpoint) in internal/hook/recovery_test.go
  - Note: T040 and T041 target same file - run sequentially

### Implementation for User Story 1

- [X] T042 [US1] Implement RecoveryDetector.DetectRecoveryNeeded in internal/hook/recovery.go
- [X] T043 [US1] Implement RecoveryDetector.DiagnoseAndRecommend with decision tree logic in internal/hook/recovery.go
- [X] T044 [US1] Implement isStaleHook helper using config threshold in internal/hook/recovery.go
- [X] T045 [US1] Add idempotent step detection (analyze, plan, validate vs implement, commit, pr) in internal/hook/recovery.go
- [X] T046 [US1] Modify atlas start command to create hook on task start in internal/cli/start.go
- [X] T047 [US1] Modify atlas resume command to check for HOOK.md and show recovery context in internal/cli/resume.go
- [X] T048 [US1] Add recovery mode prompt "Continue? [Y/n]" in resume command in internal/cli/resume.go
- [ ] T048a [US1] Implement FilesTouched tracking: update StepContext.FilesTouched during step execution in internal/task/engine.go (FR-015)

**Checkpoint**: User Story 1 complete - crash recovery detection and resume works independently

---

## Phase 4: User Story 2 - Automatic Progress Checkpoints (Priority: P1) 🎯 MVP

**Goal**: Create checkpoints automatically at key moments (git commits, validation passes, step completions)

**Independent Test**: Run a task, make commits, verify checkpoints are created with appropriate metadata

### Tests for User Story 2

- [X] T049 [P] [US2] Write checkpoint creation tests (each trigger type, git state capture, file snapshot) in internal/hook/checkpoint_test.go
- [X] T050 [P] [US2] Write checkpoint pruning tests (50 limit, oldest removed) in internal/hook/checkpoint_test.go
- [X] T051 [P] [US2] Write interval checkpointer tests (start/stop, ticker, state check) in internal/hook/checkpoint_test.go
- [ ] T052 [P] [US2] Write git hook wrapper tests in internal/git/hooks_test.go:
  - Fresh install (no existing hooks) creates wrapper
  - **Existing hook preserved**: pre-existing `post-commit` renamed to `post-commit.original`
  - **Chaining works**: wrapper executes both ATLAS checkpoint AND original hook
  - **Failure isolation**: if ATLAS checkpoint fails, original hook still runs
  - **Uninstall restores**: `post-commit.original` renamed back, wrapper removed
  - Hook permissions preserved (executable bit)

### Implementation for User Story 2

- [X] T053 [US2] Implement Checkpointer.CreateCheckpoint in internal/hook/checkpoint.go
- [X] T054 [US2] Implement checkpoint ID generation (ckpt-{uuid8}) in internal/hook/checkpoint.go
- [X] T055 [US2] Implement git state capture (branch, commit, dirty status) in internal/hook/checkpoint.go
- [X] T056 [US2] Implement file snapshot capture (path, size, modtime, sha256 prefix) in internal/hook/checkpoint.go
- [X] T057 [US2] Implement checkpoint pruning (FIFO when exceeds max_checkpoints config) in internal/hook/checkpoint.go
- [X] T058 [US2] Implement Checkpointer.GetLatestCheckpoint in internal/hook/checkpoint.go
- [X] T059 [US2] Implement Checkpointer.GetCheckpointByID in internal/hook/checkpoint.go
- [X] T060 [US2] Implement IntervalCheckpointer with cancellable goroutine in internal/hook/checkpoint.go
- [ ] T061 [US2] Implement GitHookInstaller.Install (wrapper approach, chain existing) in internal/git/hooks.go
- [ ] T062 [US2] Implement GitHookInstaller.Uninstall (restore .original) in internal/git/hooks.go
- [ ] T063 [US2] Implement GitHookInstaller.IsInstalled in internal/git/hooks.go
- [ ] T064 [US2] Create post-commit hook wrapper script template in internal/git/hooks.go
- [ ] T065 [US2] Integrate git hook installation into atlas start command in internal/cli/start.go
- [ ] T066 [US2] Integrate interval checkpointer start into task engine step execution in internal/task/engine.go
  - Note: T066, T048a, and T091 all modify engine.go - run sequentially (T048a → T066 → T091)

**Checkpoint**: User Story 2 complete - automatic checkpoints work independently

---

## Phase 5: User Story 3 - Human-Readable Recovery Context (Priority: P1) 🎯 MVP

**Goal**: Generate clear HOOK.md recovery file that explains current state and exactly what to do next

**Independent Test**: Generate a recovery file and verify both human and AI can correctly interpret the next actions

### Tests for User Story 3

- [X] T067 [US3] Write HOOK.md generation tests (fresh task, mid-step crash, completed steps, with checkpoints) in internal/hook/markdown_test.go
- [X] T068 [US3] Write regeneration test (overwrite from hook.json) in internal/hook/markdown_test.go
  - Note: T067 and T068 target same file - run sequentially

### Implementation for User Story 3

- [X] T069 [US3] Define HOOK.md template structure (header, current state, what to do now, do not, completed steps, timeline) in internal/hook/markdown.go
- [X] T070 [US3] Implement MarkdownGenerator.Generate in internal/hook/markdown.go
- [X] T071 [US3] Implement "What To Do Now" section generation based on recovery recommendations in internal/hook/markdown.go
- [X] T072 [US3] Implement "DO NOT REPEAT" section with completed steps table in internal/hook/markdown.go
- [X] T073 [US3] Implement checkpoint timeline section in internal/hook/markdown.go
- [X] T074 [US3] Implement validation receipts table section in internal/hook/markdown.go
- [X] T075 [US3] Integrate HOOK.md regeneration into FileStore.Save in internal/hook/store.go
- [X] T076 [US3] Implement atlas hook regenerate command in internal/cli/hook.go

**Checkpoint**: User Story 3 complete - human-readable recovery files work independently

---

## Phase 6: User Story 4 - Validation Receipt Integrity (Priority: P2)

**Goal**: Cryptographically sign validation receipts to prove validation actually ran

**Independent Test**: Generate receipts, verify signatures, confirm tampered receipts fail verification

### Tests for User Story 4

- [X] T077 [P] [US4] Write HD key derivation tests (path format, config-driven path) in internal/crypto/hd/signer_test.go
- [X] T078 [P] [US4] Write key file permission tests (0600) in internal/crypto/hd/signer_test.go
- [X] T079 [P] [US4] Write signing determinism tests (same input = same signature) in internal/hook/signer_hd_test.go
- [X] T080 [P] [US4] Write verification tests (valid passes, tampered fails) in internal/hook/signer_hd_test.go
- [X] T081 [P] [US4] Write missing key handling tests (graceful error) in internal/hook/signer_hd_test.go

### Implementation for User Story 4

- [X] T082 [US4] Implement FileKeyManager with key generation and 0600 permissions in internal/crypto/hd/signer.go
- [X] T083 [US4] Implement FileKeyManager.Load (generate if not exists) in internal/crypto/hd/signer.go
- [X] T084 [US4] Implement FileKeyManager.Exists in internal/crypto/hd/signer.go
- [X] T085 [US4] Implement FileKeyManager.NewSigner in internal/crypto/hd/signer.go
- [X] T086 [US4] Implement HDReceiptSigner using go-sdk in internal/hook/signer_hd.go
- [X] T087 [US4] Implement HDReceiptSigner.Sign with HD key derivation in internal/hook/signer_hd.go
- [X] T088 [US4] Implement HDReceiptSigner.Verify in internal/hook/signer_hd.go
- [X] T089 [US4] Implement HDReceiptSigner.KeyPath in internal/hook/signer_hd.go
- [X] T090 [US4] Implement signature message format (pipe-delimited) in internal/hook/signer_hd.go
- [ ] T091 [US4] Create validation receipt on validation pass in task engine in internal/task/engine.go
  - Note: Depends on T048a and T066 completing first (same file constraint)
- [X] T092 [US4] Implement atlas hook verify-receipt command in internal/cli/hook.go

**Checkpoint**: User Story 4 complete - validation receipts are cryptographically verifiable

---

## Phase 7: User Story 5 - Task State Visibility (Priority: P2)

**Goal**: Allow developers to see current task state, step progress, and checkpoint history

**Independent Test**: Run status commands during various task states and verify accurate information

### Tests for User Story 5

- [ ] T093 [P] [US5] Write hook status command tests (text output, JSON output, no hook, error state) in internal/cli/hook_test.go
- [ ] T094 [P] [US5] Write hook checkpoints command tests (list, empty, JSON format) in internal/cli/hook_test.go
- [ ] T095 [P] [US5] Write hook export command tests in internal/cli/hook_test.go

### Implementation for User Story 5

- [X] T096 [US5] Implement atlas hook status command (text and JSON output) in internal/cli/hook.go
- [X] T097 [US5] Implement atlas hook checkpoints command (table and JSON output) in internal/cli/hook.go
- [X] T098 [US5] Implement atlas hook export command in internal/cli/hook.go
- [X] T099 [US5] Add relative time formatting helper ("2 minutes ago") in internal/cli/hook.go

**Checkpoint**: User Story 5 complete - task state visibility works independently

---

## Phase 8: User Story 6 - Manual Checkpoint Creation (Priority: P3)

**Goal**: Allow developers to manually create checkpoints with descriptions

**Independent Test**: Create a manual checkpoint and verify it appears in checkpoint history

### Tests for User Story 6

- [ ] T100 [P] [US6] Write manual checkpoint command tests (success, not running, description saved) in internal/cli/checkpoint_test.go

### Implementation for User Story 6

- [X] T101 [US6] Implement atlas checkpoint "description" command in internal/cli/checkpoint.go
- [X] T102 [US6] Validate task is in step_running state before creating checkpoint in internal/cli/checkpoint.go

**Checkpoint**: User Story 6 complete - manual checkpoints work independently

---

## Phase 9: User Story 7 - Hook File Cleanup (Priority: P3)

**Goal**: Automatically clean up old hook files based on retention policies

**Independent Test**: Create old hook files and run cleanup, verify correct files are removed

### Tests for User Story 7

- [ ] T103 [P] [US7] Write cleanup command tests (retention policy, dry-run, per-state retention) in internal/cli/cleanup_test.go

### Implementation for User Story 7

- [X] T104 [US7] Add --hooks flag to atlas cleanup command in internal/cli/cleanup.go
- [X] T105 [US7] Implement retention policy logic (completed: 30d, failed: 7d, abandoned: 7d) in internal/cli/cleanup.go
- [X] T106 [US7] Implement --dry-run support for hooks cleanup in internal/cli/cleanup.go
- [X] T107 [US7] Use HookStore.ListStale and Delete for cleanup implementation in internal/cli/cleanup.go

**Checkpoint**: User Story 7 complete - hook cleanup works independently

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Integration testing, documentation, and final validation

- [ ] T108 [P] Write integration test: happy path recovery (start → run 2 steps → crash → resume at step 3) in internal/hook/integration_test.go
- [ ] T109 [P] Write integration test: checkpoint recovery (start → checkpoint → crash → resume with checkpoint info) in internal/hook/integration_test.go
- [ ] T110 [P] Write integration test: validation receipt chain (run validation → verify signature) in internal/hook/integration_test.go
- [ ] T111 [P] Write integration test: stale detection (start → wait 6 min → check status) in internal/hook/integration_test.go
- [ ] T112 [P] Write integration test: HOOK.md accuracy (various states → generate → verify content) in internal/hook/integration_test.go
- [ ] T113 Run `magex test:race` to verify all tests pass with race detection
- [ ] T114 Run `go-pre-commit run --all-files` to verify gitleaks compliance
- [ ] T115 Verify minimum 80% line coverage for internal/hook/ package
- [ ] T116 Run quickstart.md CLI commands to validate documented behavior

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phases 3-9)**: All depend on Foundational phase completion
  - US1, US2, US3 (all P1) should be done first as MVP
  - US4, US5 (P2) can follow
  - US6, US7 (P3) are lowest priority
- **Polish (Phase 10)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - Needs recovery.go
- **User Story 2 (P1)**: Can start after Foundational - Needs checkpoint.go, git/hooks.go
- **User Story 3 (P1)**: Can start after Foundational - Needs markdown.go
- **User Story 4 (P2)**: Can start after Foundational - Needs crypto/hd/, signer_hd.go
- **User Story 5 (P2)**: Can start after Foundational - Needs CLI commands
- **User Story 6 (P3)**: Depends on US2 (checkpoint infrastructure)
- **User Story 7 (P3)**: Can start after Foundational - Needs cleanup.go

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Domain types before interfaces
- Interfaces before implementations
- Core implementation before CLI integration

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All test fixture tasks (T034-T039) can run in parallel
- All domain type tasks (T005-T015) are sequential (same file)
- Shared crypto interfaces (T017a-T017c) can run in parallel with domain types
- Hook interfaces (T017-T020) depend on T017a-T017b (ReceiptSigner embeds crypto.Signer)
- Store tests (T029) and state tests (T033) can run in parallel
- Within each user story, test tasks marked [P] can run in parallel
- **Same-file constraints**: T040/T041, T067/T068 removed [P] - must run sequentially
- **engine.go constraint**: T048a → T066 → T091 must run sequentially (all modify internal/task/engine.go)

---

## Parallel Example: Foundational Phase

```bash
# Crypto interfaces can run in parallel with domain types:
Task: "Define Signer interface in internal/crypto/signer.go" (T017a)
Task: "Define Verifier interface in internal/crypto/signer.go" (T017b)
# Then crypto tests:
Task: "Write crypto interface contract tests" (T017c)

# After crypto interfaces, hook interfaces (depends on T017a-T017b):
Task: "Define ReceiptSigner interface in internal/hook/signer.go" (T017)
Task: "Create MockSigner implementation in internal/hook/signer_mock.go" (T019)
Task: "Write signer interface contract tests in internal/hook/signer_test.go" (T020)

# Launch all test fixtures in parallel:
Task: "Create valid_initializing.json fixture"
Task: "Create valid_step_running.json fixture"
Task: "Create valid_completed.json fixture"
Task: "Create with_checkpoints.json fixture"
Task: "Create corrupted.json fixture"
Task: "Create stale.json fixture"
```

---

## Parallel Example: User Story 1

```bash
# T040 and T041 target same file (recovery_test.go) - run sequentially:
Task: "Write recovery detection tests in internal/hook/recovery_test.go" (T040)
# Wait for T040 to complete
Task: "Write recovery recommendation tests in internal/hook/recovery_test.go" (T041)
```

---

## Implementation Strategy

### MVP First (User Stories 1, 2, 3)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (Resume After Crash)
4. Complete Phase 4: User Story 2 (Automatic Checkpoints)
5. Complete Phase 5: User Story 3 (Human-Readable Recovery)
6. **STOP and VALIDATE**: Test all three MVP stories work together
7. Deploy/demo crash recovery MVP

### Incremental Delivery

1. MVP (US1 + US2 + US3) → Core crash recovery working
2. Add US4 (Validation Receipts) → Cryptographic proof of validation
3. Add US5 (Task Visibility) → CLI status commands
4. Add US6 (Manual Checkpoints) → User control
5. Add US7 (Cleanup) → Disk space management

### Critical Files by Story

| User Story | Primary Files |
|------------|---------------|
| Foundation | internal/crypto/signer.go (shared interfaces), internal/domain/hook.go |
| US1 | internal/hook/recovery.go, internal/cli/resume.go, internal/task/engine.go (T048a) |
| US2 | internal/hook/checkpoint.go, internal/git/hooks.go, internal/task/engine.go (T066) |
| US3 | internal/hook/markdown.go |
| US4 | internal/crypto/hd/signer.go, internal/hook/signer_hd.go, internal/task/engine.go (T091) |
| US5 | internal/cli/hook.go |
| US6 | internal/cli/checkpoint.go |
| US7 | internal/cli/cleanup.go |

**Note**: `internal/task/engine.go` is modified by US1, US2, and US4. Tasks must run in order: T048a → T066 → T091.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- All test keys MUST be generated at runtime using `t.TempDir()` - never commit keys
- Use `// gitleaks:allow` inline comments sparingly with clear justification
