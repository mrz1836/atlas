# Tasks: Work Backlog for Discovered Issues

**Input**: Design documents from `/specs/003-work-backlog/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/discovery-schema.yaml

**Tests**: Test tasks included in each phase. Target: 90%+ coverage.
**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Based on plan.md project structure:
- Core backlog logic: `internal/backlog/`
- CLI commands: `internal/cli/`
- Error definitions: `internal/errors/`
- Runtime storage: `.atlas/backlog/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and package structure for the backlog feature

- [X] T001 Create backlog package directory structure at internal/backlog/
- [X] T002 [P] Add discovery-specific sentinel errors to internal/errors/errors.go
- [X] T003 Run `magex format:fix && magex lint` and fix any issues

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core types and utilities that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T004 Implement Status, Category, Severity enum types with constants in internal/backlog/types.go
- [X] T005 Implement Discovery, Content, Location, Context, GitContext, Lifecycle structs with YAML tags in internal/backlog/types.go
- [X] T006 Implement Filter struct with Match method in internal/backlog/types.go
- [X] T007 Implement validation methods for Discovery (ValidateID, ValidateTitle, ValidateCategory, ValidateSeverity, ValidateTags, Validate) in internal/backlog/types.go
- [X] T008 Implement GenerateID function using crypto/rand in internal/backlog/id.go
- [X] T009 Implement BacklogManager struct with NewManager constructor in internal/backlog/manager.go
- [X] T010 Implement ensureBacklogDir method to auto-create .atlas/backlog/ with .gitkeep in internal/backlog/manager.go
- [X] T011 [P] Implement createSafe helper function using O_EXCL for collision-proof file creation in internal/backlog/manager.go
- [X] T012 [P] Implement loadFile method to read and parse single discovery YAML in internal/backlog/manager.go
- [X] T013 Create 'atlas backlog' parent command group in internal/cli/backlog.go
- [X] T014 Implement unit tests for types, validation, and storage manager (coverage > 90%) in internal/backlog/
- [X] T015 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - AI Discovers and Records Issue (Priority: P1) 🎯 MVP

**Goal**: Enable AI agents to capture discoveries with flag-based input, creating individual YAML files in `.atlas/backlog/` with git context

**Independent Test**: Run `atlas backlog add "Test issue" --category bug --severity high --file test.go --line 10` and verify a valid YAML file is created in `.atlas/backlog/`

### Implementation for User Story 1

- [X] T016 [US1] Implement Add method on BacklogManager for creating new discoveries in internal/backlog/manager.go
- [X] T017 [US1] Implement git context capture (branch, commit SHA) using internal/git package in internal/backlog/manager.go
- [X] T018 [US1] Create 'atlas backlog add' subcommand with all flags in internal/cli/backlog_add.go
- [X] T019 [US1] Implement flag parsing for --file, --line, --category, --severity, --description, --tags in internal/cli/backlog_add.go
- [X] T020 [US1] Implement discoverer format handling with auto-detection (env vars ATLAS_AGENT/MODEL) in internal/cli/backlog_add.go
- [X] T021 [US1] Implement JSON output mode (--json flag) for add command in internal/cli/backlog_add.go
- [X] T022 [US1] Implement text output formatting showing created discovery details in internal/cli/backlog_add.go
- [X] T023 Implement unit tests for Add command, flags, and format detection (table-driven) in internal/cli/
- [X] T024 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: AI agents can now capture discoveries via CLI - MVP functional

---

## Phase 4: User Story 2 - Human Reviews Backlog (Priority: P2)

**Goal**: Enable developers to list, filter, and view discovery details from the backlog

**Independent Test**: Create sample discovery files in `.atlas/backlog/`, run `atlas backlog list` and `atlas backlog view disc-<id>`, verify correct display

### Implementation for User Story 2

- [X] T025 [US2] Implement List method on BacklogManager with parallel file reads and bounded concurrency in internal/backlog/manager.go
- [X] T026 [US2] Implement Get method on BacklogManager to retrieve single discovery by ID in internal/backlog/manager.go
- [X] T027 [US2] Create 'atlas backlog list' subcommand in internal/cli/backlog_list.go
- [X] T028 [US2] Implement filter flags (--status, --category, --all, --limit) for list command in internal/cli/backlog_list.go
- [X] T029 [US2] Implement table output formatting with ID, title, category, severity, age columns in internal/cli/backlog_list.go
- [X] T030 [US2] Implement JSON output mode for list command in internal/cli/backlog_list.go
- [X] T031 [US2] Create 'atlas backlog view <id>' subcommand in internal/cli/backlog_view.go
- [X] T032 [US2] Implement rich view output using charmbracelet/glamour for markdown rendering in internal/cli/backlog_view.go
- [X] T033 [US2] Implement JSON output mode for view command in internal/cli/backlog_view.go
- [X] T034 Implement unit tests for List/View commands and Filter logic in internal/cli/
- [X] T035 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: Users can review the full backlog and view discovery details

---

## Phase 5: User Story 3 - Human Adds Discovery Interactively (Priority: P3)

**Goal**: Enable developers to add discoveries via interactive form with guided prompts

**Independent Test**: Run `atlas backlog add` without arguments, complete the form, verify discovery file is created correctly

### Implementation for User Story 3

- [X] T036 [US3] Implement interactive form flow using charmbracelet/huh in internal/cli/backlog_add.go
- [X] T037 [US3] Create form fields for title (required), description (optional), category (select), severity (select) in internal/cli/backlog_add.go
- [X] T038 [US3] Create optional form fields for file path and line number in internal/cli/backlog_add.go
- [X] T039 [US3] Implement form validation matching schema requirements in internal/cli/backlog_add.go
- [X] T040 [US3] Implement automatic discoverer detection (human:<git-username> from git config) in internal/cli/backlog_add.go
- [X] T041 Implement unit tests for interactive form helpers and input validation in internal/cli/
- [X] T042 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: Humans can add discoveries interactively with full form guidance

---

## Phase 6: User Story 4 - Promote Discovery to Task (Priority: P4)

**Goal**: Enable developers to promote a discovery to a task, recording the task ID

**Independent Test**: Run `atlas backlog promote disc-<id> --task-id task-xxx`, verify status changes to "promoted" and task ID is stored

### Implementation for User Story 4

- [X] T043 [US4] Implement Promote method on BacklogManager for status transition in internal/backlog/manager.go
- [X] T044 [US4] Implement status transition validation (only pending → promoted allowed) in internal/backlog/manager.go
- [X] T045 [US4] Create 'atlas backlog promote <id>' subcommand in internal/cli/backlog_promote.go
- [X] T046 [US4] Implement --task-id flag (required) for promote command in internal/cli/backlog_promote.go
- [X] T047 [US4] Implement text and JSON output modes for promote command in internal/cli/backlog_promote.go
- [X] T048 Implement unit tests for Promote command and status transitions in internal/cli/
- [X] T049 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: Discoveries can be promoted to tasks with proper linking

---

## Phase 7: User Story 5 - Dismiss Discovery (Priority: P5)

**Goal**: Enable developers to dismiss a discovery with a reason for housekeeping

**Independent Test**: Run `atlas backlog dismiss disc-<id> --reason "duplicate"`, verify status changes to "dismissed" and reason is stored

### Implementation for User Story 5

- [X] T050 [US5] Implement Dismiss method on BacklogManager for status transition in internal/backlog/manager.go
- [X] T051 [US5] Implement status transition validation (only pending → dismissed allowed) in internal/backlog/manager.go
- [X] T052 [US5] Create 'atlas backlog dismiss <id>' subcommand in internal/cli/backlog_dismiss.go
- [X] T053 [US5] Implement --reason flag (required) for dismiss command in internal/cli/backlog_dismiss.go
- [X] T054 [US5] Implement text and JSON output modes for dismiss command in internal/cli/backlog_dismiss.go
- [X] T055 Implement unit tests for Dismiss command and status transitions in internal/cli/
- [X] T056 Run `magex format:fix && magex lint` and fix any issues

**Checkpoint**: Discoveries can be dismissed with documented reasons

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Error handling refinement, edge cases, and final validation

- [X] T057 Handle edge case: malformed YAML files during list (skip with warning, don't break) in internal/backlog/manager.go
- [X] T058 Handle edge case: missing git context gracefully (empty strings, log warning) in internal/backlog/manager.go
- [X] T059 [P] Add human-readable age calculation for list output (e.g., "2h", "1d", "3d") in internal/cli/backlog_list.go
- [X] T060 [P] Ensure consistent exit codes: 0 (success), 1 (general error), 2 (invalid input) across all commands
- [X] T061 Verify 90%+ test coverage across all new packages and cover edge cases (missing files, permissions, etc.)
- [X] T062 Run `magex format:fix && magex lint` and fix any issues
- [X] T063 Update `docs/internal/quick-start.md` with new commands and backlog functionality
- [X] T064 Run quickstart.md validation - verify all documented commands work as specified

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phases 3-7)**: All depend on Foundational phase completion
  - User stories can proceed in priority order (P1 → P2 → P3 → P4 → P5)
  - Or in parallel with multiple developers
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational - No dependencies on other stories
- **User Story 3 (P3)**: Depends on US1 (extends add command) - Must implement after US1
- **User Story 4 (P4)**: Can start after Foundational - No dependencies on other stories
- **User Story 5 (P5)**: Can start after Foundational - No dependencies on other stories

### Within Each User Story

- BacklogManager methods before CLI commands
- Core functionality before output formatting
- Text output before JSON output
- **Tests MUST be implemented before moving to next phase**
- **Validation MUST run before moving to next phase**

### Parallel Opportunities

- T001, T002 can run in parallel (Setup phase)
- T011, T012 can run in parallel (file utilities)
- T059, T060 can run in parallel (Polish phase)
- After Foundational: US1, US2, US4, US5 can run in parallel (different files)
- US3 must wait for US1 completion (extends same file)
