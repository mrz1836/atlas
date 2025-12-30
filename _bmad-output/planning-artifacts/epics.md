---
stepsCompleted: [1, 2, 3, 4]
workflowStatus: complete
completedAt: 2025-12-27
inputDocuments:
  - _bmad-output/planning-artifacts/prd.md
  - _bmad-output/planning-artifacts/architecture.md
  - _bmad-output/planning-artifacts/ux-design-specification.md
  - docs/external/vision.md
totalEpics: 8
totalStories: 66
---

# atlas - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for atlas, decomposing the requirements from the PRD, UX Design, and Architecture requirements into implementable stories.

## Requirements Inventory

### Functional Requirements

**Setup & Configuration (FR1-FR8):**
- FR1: User can initialize ATLAS in a Git repository via setup wizard
- FR2: User can configure AI provider settings (API keys, model selection)
- FR3: User can configure validation commands per project
- FR4: User can configure notification preferences
- FR5: System can auto-detect installed tools (mage-x, go-pre-commit, Speckit, gh CLI)
- FR6: User can override global configuration with project-specific settings
- FR7: User can override configuration via environment variables
- FR8: User can self-upgrade ATLAS and managed tools

**Task Management (FR9-FR13):**
- FR9: User can start a task with natural language description
- FR10: User can select a template for the task (bugfix, feature, commit)
- FR11: User can specify a custom workspace name for the task
- FR12: System can expand task description into structured specification (SDD abstraction)
- FR13: User can run utility commands (format, lint, test, validate) standalone

**Workspace Management (FR14-FR20):**
- FR14: System can create isolated Git worktrees for parallel task execution
- FR15: User can view all active workspaces and their status
- FR16: User can destroy a workspace and clean up its worktree
- FR17: User can retire a completed workspace (archive state, remove worktree)
- FR18: User can view logs for a specific workspace
- FR19: User can view logs for a specific step within a workspace
- FR20: System can manage 3+ parallel workspaces simultaneously

**AI Orchestration (FR21-FR26):**
- FR21: System can invoke Claude Code CLI for task execution
- FR22: System can pass task context and prompts to AI runner
- FR23: System can capture AI runner output and artifacts
- FR24: System can abstract Speckit SDD workflows behind templates
- FR25: System can provide error context to AI for retry attempts
- FR26: User can configure AI model selection per task or globally

**Validation & Quality (FR27-FR33):**
- FR27: System can execute validation commands (lint, test, format)
- FR28: System can detect validation failures and pause for user decision
- FR29: User can retry validation with AI fix attempt
- FR30: User can fix validation issues manually and resume
- FR31: User can abandon task while preserving branch and worktree
- FR32: System can auto-format code before other validations
- FR33: System can run pre-commit hooks as validation step

**Git Operations (FR34-FR40):**
- FR34: System can create feature branches with consistent naming
- FR35: System can stage and commit changes with meaningful messages
- FR36: System can detect and warn about garbage files before commit
- FR37: System can push branches to remote
- FR38: System can create pull requests via gh CLI
- FR39: System can monitor GitHub Actions CI status after PR creation
- FR40: System can detect CI failures and notify user

**Status & Monitoring (FR41-FR46):**
- FR41: User can view real-time status of all workspaces in table format
- FR42: User can enable watch mode for continuous status updates
- FR43: System can emit terminal bell when task needs attention
- FR44: System can display task step progress (e.g., "Step 4/8")
- FR45: System can show clear action indicators (approve, retry, etc.)
- FR46: User can output status in JSON format for scripting

**User Interaction (FR47-FR52):**
- FR47: User can approve completed work and trigger merge-ready state
- FR48: User can reject work with feedback for AI retry
- FR49: System can present interactive menus for error recovery decisions
- FR50: System can display styled output with colors and icons
- FR51: System can show progress spinners for long operations
- FR52: User can run in non-interactive mode with sensible defaults

### NonFunctional Requirements

**Performance (NFR1-NFR4):**
- NFR1: Local operations (status, workspace list) complete in <1 second
- NFR2: UI remains responsive during long-running AI operations (non-blocking)
- NFR3: Timeouts for network operations: 30 seconds default, configurable
- NFR4: Progress indication during AI operations (spinner, step display)

**Security (NFR5-NFR10):**
- NFR5: API keys read from environment variables or secure config
- NFR6: API keys never logged or displayed in output
- NFR7: API keys never committed to Git (warn if detected in worktree)
- NFR8: GitHub auth delegated to gh CLI (no token storage in ATLAS)
- NFR9: No sensitive data in JSON log output
- NFR10: Config files should not contain secrets in plain text (use env var references)

**Reliability (NFR11-NFR21):**
- NFR11: Task state saved after each step completion (safe checkpoint)
- NFR12: On failure or crash, task can resume from last completed step
- NFR13: State files always human-readable (JSON/YAML)
- NFR14: State files can be manually edited if needed
- NFR15: Worktree creation must be atomic (no partial state)
- NFR16: Worktree destruction must be 100% reliable (no orphaned directories)
- NFR17: No orphaned Git branches after workspace cleanup
- NFR18: `atlas workspace destroy` always succeeds, even if state is corrupted
- NFR19: All errors have clear, actionable messages
- NFR20: System never hangs indefinitely (timeouts on all external operations)
- NFR21: Partial failures leave system in recoverable state

**Integration (NFR22-NFR27):**
- NFR22: Claude Code CLI invocation via subprocess
- NFR23: Must handle Claude Code CLI errors gracefully
- NFR24: Fallback strategy defined if primary invocation method fails
- NFR25: All GitHub operations via gh CLI (no direct API calls)
- NFR26: Standard Git operations via subprocess
- NFR27: Speckit invocation via CLI subprocess

**Operational (NFR28-NFR33):**
- NFR28: Structured JSON logs for debugging
- NFR29: Log levels: debug, info, warn, error
- NFR30: Logs stored per-workspace, accessible via `atlas workspace logs`
- NFR31: Clear task step progress (e.g., "Step 4/8: Running validation")
- NFR32: Terminal bell on state changes requiring attention
- NFR33: `--verbose` flag for detailed operation logging

### Additional Requirements

**From Architecture - Project Initialization:**
- ARCH-1: Project uses Manual Setup with Go 1.24+ conventions (no external starter template)
- ARCH-2: Cobra/Viper scaffolding for CLI layer
- ARCH-3: Project structure with 9 major subsystems organized in internal/ packages
- ARCH-4: Single binary entry point via cmd/atlas/main.go

**From Architecture - Foundational Packages:**
- ARCH-5: Constants package (internal/constants) for all shared constants
- ARCH-6: Errors package (internal/errors) for sentinel errors and wrapping utilities
- ARCH-7: Config package (internal/config) for layered configuration with precedence
- ARCH-8: Domain package (internal/domain) for shared types (Task, Workspace, Step, etc.)

**From Architecture - Integration Patterns:**
- ARCH-9: AIRunner interface with ClaudeCodeRunner implementation
- ARCH-10: GitRunner interface for git operations
- ARCH-11: GitHubRunner interface for gh CLI operations
- ARCH-12: State machine with explicit transitions table for task lifecycle

**From Architecture - Implementation Patterns:**
- ARCH-13: Context-first design (ctx as first parameter everywhere)
- ARCH-14: Error wrapping only at package boundaries
- ARCH-15: Atomic writes with file locking (flock) for state files
- ARCH-16: snake_case for all JSON fields in state files
- ARCH-17: Retry configuration with exponential backoff (3x max)

**From UX Design - TUI Framework:**
- UX-1: Charm ecosystem (Bubble Tea + Lip Gloss + Huh + Bubbles) for all TUI components
- UX-2: "ATLAS Command Flow" design direction with dramatic header and progress visualization
- UX-3: OSC 8 hyperlinks for PR numbers in modern terminals

**From UX Design - Visual System:**
- UX-4: Semantic color palette (Primary Blue, Success Green, Warning Yellow, Error Red, Muted Gray)
- UX-5: State iconography: ● running, ✓ ready, ⚠ attention, ✗ failed, ○ pending
- UX-6: AdaptiveColor for light/dark terminal support

**From UX Design - Accessibility:**
- UX-7: NO_COLOR environment variable support
- UX-8: Triple redundancy rule (icon + color + text for all states)
- UX-9: Full keyboard navigation (no mouse required)
- UX-10: Terminal width adaptation (80/120+ column modes)

**From UX Design - Interactive Patterns:**
- UX-11: Action menu bar with single-key shortcuts `[a] Approve [r] Reject [d] Diff [l] Logs [q] Cancel`
- UX-12: Interactive menus using Huh for approval/rejection flows
- UX-13: Progress dashboard with full-width progress bars (▓░ characters)
- UX-14: Auto-density: 2-line mode for ≤5 tasks, 1-line mode for >5 tasks

### FR Coverage Map

| FR | Epic | Description |
|----|------|-------------|
| FR1 | Epic 2 | Initialize ATLAS via setup wizard |
| FR2 | Epic 2 | Configure AI provider settings |
| FR3 | Epic 2 | Configure validation commands |
| FR4 | Epic 2 | Configure notification preferences |
| FR5 | Epic 2 | Auto-detect installed tools |
| FR6 | Epic 2 | Project-specific config overrides |
| FR7 | Epic 2 | Environment variable overrides |
| FR8 | Epic 2 | Self-upgrade ATLAS and tools |
| FR9 | Epic 4 | Start task with description |
| FR10 | Epic 4 | Select template for task |
| FR11 | Epic 4 | Specify custom workspace name |
| FR12 | Epic 4 | Expand description to specification |
| FR13 | Epic 4 | Run utility commands standalone |
| FR14 | Epic 3 | Create isolated Git worktrees |
| FR15 | Epic 3 | View all workspaces and status |
| FR16 | Epic 3 | Destroy workspace and worktree |
| FR17 | Epic 3 | Retire completed workspace |
| FR18 | Epic 3 | View workspace logs |
| FR19 | Epic 3 | View step-specific logs |
| FR20 | Epic 3 | Manage 3+ parallel workspaces |
| FR21 | Epic 4 | Invoke Claude Code CLI |
| FR22 | Epic 4 | Pass context to AI runner |
| FR23 | Epic 4 | Capture AI output and artifacts |
| FR24 | Epic 4 | Abstract Speckit workflows |
| FR25 | Epic 4 | Provide error context for retry |
| FR26 | Epic 4 | Configure AI model selection |
| FR27 | Epic 5 | Execute validation commands |
| FR28 | Epic 5 | Detect failures and pause |
| FR29 | Epic 5 | Retry with AI fix attempt |
| FR30 | Epic 5 | Fix manually and resume |
| FR31 | Epic 5 | Abandon preserving branch |
| FR32 | Epic 5 | Auto-format before validation |
| FR33 | Epic 5 | Run pre-commit hooks |
| FR34 | Epic 6 | Create feature branches |
| FR35 | Epic 6 | Commit with meaningful messages |
| FR36 | Epic 6 | Detect garbage files |
| FR37 | Epic 6 | Push to remote |
| FR38 | Epic 6 | Create PRs via gh CLI |
| FR39 | Epic 6 | Monitor CI status |
| FR40 | Epic 6 | Detect CI failures |
| FR41 | Epic 7 | Real-time status table |
| FR42 | Epic 7 | Watch mode with live updates |
| FR43 | Epic 7 | Terminal bell notifications |
| FR44 | Epic 7 | Step progress display |
| FR45 | Epic 7 | Clear action indicators |
| FR46 | Epic 7 | JSON output for scripting |
| FR47 | Epic 8 | Approve completed work |
| FR48 | Epic 8 | Reject with feedback |
| FR49 | Epic 8 | Interactive error recovery menus |
| FR50 | Epic 8 | Styled output with colors/icons |
| FR51 | Epic 8 | Progress spinners |
| FR52 | Epic 8 | Non-interactive mode |

## Epic List

### Epic 1: Project Foundation
**Goal:** Development team can begin building ATLAS with consistent architecture following Go 1.24+ conventions and the documented project structure.

**User Outcome:** All foundational packages exist with proper interfaces, enabling consistent development across all future epics.

**Covers:** ARCH-1 to ARCH-8

---

### Epic 2: CLI Framework & Configuration
**Goal:** Users can install ATLAS, run `atlas init`, and configure their environment with a polished setup wizard.

**User Outcome:** ATLAS is installed and fully configured with AI provider, validation commands, and notification preferences.

**Covers:** FR1-FR8, ARCH-13 to ARCH-17

---

### Epic 3: Workspace Management
**Goal:** Users can create, list, destroy, and retire parallel workspaces backed by Git worktrees.

**User Outcome:** Users can manage multiple isolated workspaces for parallel task execution.

**Covers:** FR14-FR20, NFR11-NFR21

---

### Epic 4: Task Engine & AI Execution
**Goal:** Users can start tasks with `atlas start` and have Claude Code execute them using templates.

**User Outcome:** The core task loop works end-to-end - users queue work, AI executes it.

**Covers:** FR9-FR13, FR21-FR26, ARCH-9 to ARCH-12, NFR22-NFR27

---

### Epic 5: Validation Pipeline
**Goal:** Tasks are automatically validated with format, lint, and test commands, with clear pass/fail feedback.

**User Outcome:** Quality gates ensure AI output meets project standards before delivery.

**Covers:** FR27-FR33, NFR1-NFR4

---

### Epic 6: Git & PR Automation
**Goal:** Completed work automatically becomes branches, commits, and pull requests via gh CLI.

**User Outcome:** The delivery pipeline is fully automated - work becomes PRs without manual ceremony.

**Covers:** FR34-FR40

---

### Epic 7: Status Dashboard & Monitoring
**Goal:** Users can view their "fleet" of workspaces with a beautiful TUI, live updates, and terminal bell notifications.

**User Outcome:** Users experience the "fleet commander" view - glanceable status, notification-driven awareness.

**Covers:** FR41-FR46, UX-1 to UX-14

---

### Epic 8: Interactive Review & Approval
**Goal:** Users can approve, reject, and provide feedback through intuitive interactive flows.

**User Outcome:** The human-in-the-loop cycle is complete with polished approve/reject workflows.

**Covers:** FR47-FR52, NFR28-NFR33

---

## Epic 1: Project Foundation

**Goal:** Development team can begin building ATLAS with consistent architecture following Go 1.24+ conventions and the documented project structure.

### Story 1.1: Initialize Go Module and Project Structure

As a **developer**,
I want **the ATLAS project initialized with the complete directory structure and Go module**,
So that **I have a consistent foundation for implementing all ATLAS subsystems**.

**Acceptance Criteria:**

**Given** a clean repository
**When** I run the initialization commands
**Then** the go.mod file exists with module path `github.com/mrz1836/atlas`
**And** Go version is set to 1.24+
**And** all required directories exist:
- `cmd/atlas/`
- `internal/cli/`
- `internal/config/`
- `internal/task/`
- `internal/workspace/`
- `internal/ai/`
- `internal/git/`
- `internal/validation/`
- `internal/template/`
- `internal/tui/`
- `internal/constants/`
- `internal/errors/`
- `internal/domain/`
- `internal/testutil/`
**And** core dependencies are added (cobra, viper, zerolog, charm libs)
**And** `.golangci.yml` is configured with project linting rules
**And** `go mod tidy` runs without errors

---

### Story 1.2: Create Constants Package

As a **developer**,
I want **a centralized constants package with all shared values**,
So that **I never use magic strings and all AI agents use consistent values**.

**Acceptance Criteria:**

**Given** the project structure exists
**When** I implement `internal/constants/`
**Then** `constants.go` contains:
- File names: `TaskFileName = "task.json"`, `WorkspaceFileName = "workspace.json"`
- Directories: `AtlasHome = ".atlas"`, `WorkspacesDir = "workspaces"`, `TasksDir = "tasks"`, `ArtifactsDir = "artifacts"`
**And** `status.go` contains task and workspace status constants
**And** `paths.go` contains path-related constants
**And** all constants are exported and documented
**And** the package has 100% test coverage for any helper functions
**And** no other package in the codebase defines inline magic strings for these values

---

### Story 1.3: Create Errors Package

As a **developer**,
I want **a centralized errors package with sentinel errors and wrapping utilities**,
So that **error handling is consistent and errors can be categorized programmatically**.

**Acceptance Criteria:**

**Given** the constants package exists
**When** I implement `internal/errors/`
**Then** `errors.go` contains sentinel errors:
- `ErrValidationFailed`
- `ErrClaudeInvocation`
- `ErrGitOperation`
- `ErrGitHubOperation`
- `ErrCIFailed`
- `ErrCITimeout`
- `ErrUserRejected`
- `ErrUserAbandoned`
**And** `wrap.go` contains `Wrap(err error, msg string) error` utility
**And** `user.go` contains user-facing error formatting utilities
**And** errors follow the pattern `errors.New("lower case description")`
**And** the package does not import any other internal packages
**And** tests verify error wrapping preserves the sentinel for `errors.Is()` checks

---

### Story 1.4: Create Domain Types Package

As a **developer**,
I want **a domain package with all shared types**,
So that **type definitions are centralized and consistent across all packages**.

**Acceptance Criteria:**

**Given** the constants and errors packages exist
**When** I implement `internal/domain/`
**Then** `task.go` contains:
- `Task` struct with JSON tags using snake_case
- `Step` struct
- `StepResult` struct
**And** `workspace.go` contains:
- `Workspace` struct with JSON tags
- `TaskRef` struct
**And** `template.go` contains:
- `Template` struct
- `StepDefinition` struct
**And** `ai.go` contains:
- `AIRequest` struct
- `AIResult` struct
**And** `status.go` contains:
- `TaskStatus` type with constants (Pending, Running, Validating, AwaitingApproval, etc.)
- `WorkspaceStatus` type with constants (Active, Paused, Retired)
**And** all JSON tags use snake_case per Architecture requirement
**And** the package does not import any other internal packages (except constants/errors)
**And** all types have example JSON representations in tests

---

### Story 1.5: Create Configuration Framework

As a **developer**,
I want **a configuration package with layered precedence loading**,
So that **configuration can be overridden at CLI, environment, project, and global levels**.

**Acceptance Criteria:**

**Given** the domain package exists
**When** I implement `internal/config/`
**Then** `config.go` contains the main `Config` struct with nested configs:
- `AIConfig` for AI settings
- `GitConfig` for Git settings
- `WorktreeConfig` for worktree settings
- `CIConfig` for CI settings
- `TemplatesConfig` for template settings
- `ValidationConfig` for validation settings
**And** `load.go` implements `Load(ctx context.Context) (*Config, error)` with precedence:
1. CLI flags (passed in)
2. Environment variables (ATLAS_* prefix)
3. Project config (.atlas/config.yaml)
4. Global config (~/.atlas/config.yaml)
5. Built-in defaults
**And** `validate.go` implements config validation
**And** Viper is integrated for YAML parsing
**And** environment variable mapping works (ATLAS_AI_MODEL → ai.model)
**And** tests verify precedence order is correct

---

### Story 1.6: Create CLI Root Command

As a **developer**,
I want **the CLI entry point and root command implemented with Cobra**,
So that **the `atlas` command runs and displays help information**.

**Acceptance Criteria:**

**Given** the config package exists
**When** I implement `cmd/atlas/main.go` and `internal/cli/root.go`
**Then** `main.go`:
- Creates a root context with `context.Background()`
- Calls the CLI execute function
- Handles exit codes correctly (0 success, 1 error, 2 invalid input)
**And** `root.go`:
- Defines the root Cobra command
- Implements global flags: `--output json|text`, `--verbose`, `--quiet`
- Sets up Viper configuration binding
- Initializes zerolog with appropriate level
**And** `flags.go` contains shared flag definitions
**And** running `go run ./cmd/atlas` displays help text
**And** running `go run ./cmd/atlas --version` displays version info
**And** the command follows Cobra best practices
**And** tests verify flag parsing and help output

---

## Epic 2: CLI Framework & Configuration

**Goal:** Users can install ATLAS, run `atlas init`, and configure their environment with a polished setup wizard.

### Story 2.1: Implement Tool Detection System

As a **user**,
I want **ATLAS to automatically detect installed tools**,
So that **I know which dependencies are available and which need to be installed**.

**Acceptance Criteria:**

**Given** the CLI root command exists
**When** I implement the tool detection system in `internal/config/tools.go`
**Then** the system can detect and report status for:
- Go (version 1.24+, required)
- Git (version 2.20+, required)
- gh CLI (version 2.20+, required)
- uv (version 0.5.x, required)
- claude CLI (version 2.0.76+, required)
- mage-x (managed by ATLAS)
- go-pre-commit (managed by ATLAS)
- Speckit (managed by ATLAS)
**And** each tool reports: installed/missing/outdated status, current version, required version
**And** detection uses `exec.LookPath` and version parsing
**And** missing required tools return clear error with install instructions
**And** the detection completes in under 2 seconds
**And** tests mock command execution for reliable testing

---

### Story 2.2: Implement `atlas init` Setup Wizard

As a **user**,
I want **to run `atlas init` and complete a guided setup wizard**,
So that **ATLAS is configured correctly for my environment**.

**Acceptance Criteria:**

**Given** tool detection is implemented
**When** I run `atlas init`
**Then** the wizard displays the ATLAS header with branding
**And** runs tool detection and displays status table
**And** if required tools are missing, displays error with install instructions and exits
**And** if managed tools are missing/outdated, prompts: "Install/upgrade ATLAS-managed tools? [Y/n]"
**And** proceeds to AI provider configuration step
**And** proceeds to validation commands step
**And** proceeds to notification preferences step
**And** saves configuration to `~/.atlas/config.yaml`
**And** displays success message with suggested next command
**And** the wizard uses Charm Huh for interactive forms
**And** `--no-interactive` flag uses sensible defaults without prompts

---

### Story 2.3: AI Provider Configuration

As a **user**,
I want **to configure my AI provider settings during init**,
So that **ATLAS knows which AI model to use and how to authenticate**.

**Acceptance Criteria:**

**Given** the init wizard is running
**When** I reach the AI provider configuration step
**Then** I can select the default model (sonnet or opus)
**And** I can specify API key environment variable name (default: ANTHROPIC_API_KEY)
**And** the system validates the API key exists in environment
**And** if API key is missing, displays warning but allows continuing
**And** I can configure default timeout (default: 30m)
**And** I can configure max turns per step (default: 10)
**And** settings are saved to config under `ai:` section
**And** API keys are NEVER written to config files (only env var references)

---

### Story 2.4: Validation Commands Configuration

As a **user**,
I want **to configure validation commands during init**,
So that **ATLAS runs the correct lint, test, and format commands for my project**.

**Acceptance Criteria:**

**Given** the init wizard is running
**When** I reach the validation commands step
**Then** the system suggests defaults based on detected tools:
- If mage-x detected: `magex format:fix`, `magex lint`, `magex test`
- If go-pre-commit detected: adds `go-pre-commit run --all-files`
**And** I can customize the command list
**And** I can add custom pre-PR hooks
**And** settings are saved to config under `validation:` section
**And** commands are validated to be executable
**And** the configuration supports per-template overrides

---

### Story 2.5: Notification Preferences Configuration

As a **user**,
I want **to configure notification preferences**,
So that **ATLAS alerts me appropriately when tasks need attention**.

**Acceptance Criteria:**

**Given** the init wizard is running
**When** I reach the notification preferences step
**Then** I can enable/disable terminal bell (default: enabled)
**And** I can configure which events trigger notifications:
- Task awaiting approval
- Validation failed
- CI failed
- GitHub operation failed
**And** settings are saved to config under `notifications:` section
**And** the terminal bell uses the BEL character (\a)

---

### Story 2.6: Configuration Override System

As a **user**,
I want **to override global configuration with project-specific settings and environment variables**,
So that **different projects can have different ATLAS configurations**.

**Acceptance Criteria:**

**Given** global config exists at `~/.atlas/config.yaml`
**When** I create `.atlas/config.yaml` in a project directory
**Then** project settings override global settings
**And** environment variables (ATLAS_*) override both
**And** CLI flags override everything
**And** the precedence order is: CLI > env > project > global > defaults
**And** running `atlas init` in a project creates `.atlas/config.yaml`
**And** the system merges configs correctly (not full replacement)
**And** `atlas config show` displays effective configuration with source annotations

---

### Story 2.7: Implement `atlas upgrade` Command

As a **user**,
I want **to run `atlas upgrade` to update ATLAS and managed tools**,
So that **I always have the latest versions with bug fixes and features**.

**Acceptance Criteria:**

**Given** ATLAS is installed
**When** I run `atlas upgrade`
**Then** the system checks for updates to:
- ATLAS itself (via `go install github.com/mrz1836/atlas@latest`)
- mage-x
- go-pre-commit
- Speckit
**And** displays available updates with version numbers
**And** prompts for confirmation before upgrading
**And** for Speckit upgrades, backs up constitution.md first
**And** restores constitution.md after Speckit upgrade
**And** `atlas upgrade --check` shows available updates without installing
**And** `atlas upgrade speckit` upgrades only Speckit
**And** displays success/failure status for each upgrade
**And** handles upgrade failures gracefully with rollback information

---

## Epic 3: Workspace Management

**Goal:** Users can create, list, destroy, and retire parallel workspaces backed by Git worktrees.

### Story 3.1: Workspace Data Model and Store

As a **developer**,
I want **a workspace persistence layer with atomic operations**,
So that **workspace state is reliably saved and can survive crashes**.

**Acceptance Criteria:**

**Given** the domain types exist
**When** I implement `internal/workspace/store.go`
**Then** the store can:
- Create workspace JSON files at `~/.atlas/workspaces/<name>/workspace.json`
- Read workspace state with proper error handling
- Update workspace state with atomic write (write-then-rename)
- List all workspaces by scanning the workspaces directory
- Delete workspace state files
**And** file operations use flock for concurrent access safety
**And** all JSON uses snake_case field names per Architecture
**And** schema_version is included in all workspace.json files
**And** corrupted JSON files are handled gracefully with clear error messages
**And** tests verify atomic write behavior (no partial writes on failure)

---

### Story 3.2: Git Worktree Operations

As a **developer**,
I want **a GitRunner implementation for worktree operations**,
So that **ATLAS can create and manage isolated Git working directories**.

**Acceptance Criteria:**

**Given** the workspace store exists
**When** I implement `internal/workspace/worktree.go`
**Then** the system can:
- Create a worktree: `git worktree add <path> -b <branch>`
- List worktrees: `git worktree list --porcelain`
- Remove a worktree: `git worktree remove <path>`
- Prune stale worktrees: `git worktree prune`
**And** worktrees are created as siblings to the repo (e.g., `../myrepo-auth/`)
**And** branch naming follows pattern: `<type>/<workspace-name>`
**And** if worktree path exists, appends numeric suffix (-2, -3, etc.)
**And** if branch already exists, appends timestamp suffix
**And** worktree creation is atomic (no partial state on failure)
**And** errors include actionable recovery suggestions
**And** tests use a temporary git repository for integration testing

---

### Story 3.3: Workspace Manager Service

As a **developer**,
I want **a WorkspaceManager service that orchestrates workspace lifecycle**,
So that **workspace operations are coordinated between state and git worktrees**.

**Acceptance Criteria:**

**Given** the store and worktree operations exist
**When** I implement `internal/workspace/manager.go`
**Then** the manager provides:
- `Create(ctx, name, repoPath, branchType) (*Workspace, error)` - creates workspace + worktree
- `Get(ctx, name) (*Workspace, error)` - retrieves workspace by name
- `List(ctx) ([]*Workspace, error)` - lists all workspaces
- `Destroy(ctx, name) error` - removes workspace state AND worktree
- `Retire(ctx, name) error` - archives state, removes worktree
- `UpdateStatus(ctx, name, status) error` - updates workspace status
**And** Create validates name uniqueness
**And** Destroy succeeds even if state is corrupted (NFR18)
**And** Destroy cleans up orphaned branches
**And** all operations use context for cancellation
**And** operations are logged with workspace context
**And** the manager supports 3+ concurrent workspaces (FR20)

---

### Story 3.4: Implement `atlas workspace list` Command

As a **user**,
I want **to run `atlas workspace list` to see all my workspaces**,
So that **I can see what tasks are running and their current status**.

**Acceptance Criteria:**

**Given** workspaces exist
**When** I run `atlas workspace list`
**Then** a table displays:
```
NAME        BRANCH          STATUS    CREATED         TASKS
auth        feat/auth       active    2 hours ago     2
payment     fix/payment     paused    1 day ago       1
old-feat    feat/old        retired   3 days ago      3
```
**And** the table uses Lip Gloss styling with semantic colors
**And** status shows: active (blue), paused (gray), retired (dim)
**And** `--output json` returns structured JSON array
**And** empty state displays helpful message: "No workspaces. Run 'atlas start' to create one."
**And** the command completes in <1 second (NFR1)

---

### Story 3.5: Implement `atlas workspace destroy` Command

As a **user**,
I want **to run `atlas workspace destroy <name>` to fully clean up a workspace**,
So that **I can remove completed or abandoned work without leaving orphaned files**.

**Acceptance Criteria:**

**Given** a workspace exists
**When** I run `atlas workspace destroy payment`
**Then** the system prompts for confirmation: "Delete workspace 'payment'? This cannot be undone. [y/N]"
**And** if confirmed:
- Removes the git worktree directory
- Deletes the git branch (if not merged)
- Removes `~/.atlas/workspaces/payment/` directory
- Prunes any stale worktree references
**And** displays success: "✓ Workspace 'payment' destroyed"
**And** `--force` skips confirmation prompt
**And** if workspace doesn't exist, displays clear error
**And** if worktree is already gone, continues with state cleanup
**And** if state is corrupted, still removes what it can (NFR18)
**And** no orphaned directories or branches remain (NFR16, NFR17)

---

### Story 3.6: Implement `atlas workspace retire` Command

As a **user**,
I want **to run `atlas workspace retire <name>` to archive a completed workspace**,
So that **I preserve the task history while freeing up disk space**.

**Acceptance Criteria:**

**Given** a workspace exists with status "active" or "paused"
**When** I run `atlas workspace retire auth`
**Then** the system:
- Verifies no tasks are currently running
- Updates workspace status to "retired"
- Removes the git worktree (but keeps the branch)
- Preserves `~/.atlas/workspaces/auth/` with all task history
**And** displays success: "✓ Workspace 'auth' retired. History preserved."
**And** if tasks are running, displays error: "Cannot retire workspace with running tasks"
**And** retired workspaces still appear in `workspace list` with "retired" status
**And** retired workspaces can be referenced for log viewing

---

### Story 3.7: Implement `atlas workspace logs` Command

As a **user**,
I want **to run `atlas workspace logs <name>` to view task execution logs**,
So that **I can debug issues and understand what ATLAS did**.

**Acceptance Criteria:**

**Given** a workspace exists with task history
**When** I run `atlas workspace logs auth`
**Then** displays the most recent task's log file content
**And** logs are displayed with syntax highlighting for JSON-lines format
**And** `--follow` or `-f` streams new log entries in real-time
**And** `--step <name>` filters to a specific step's logs (FR19)
**And** `--task <id>` shows logs for a specific task (not just most recent)
**And** if no logs exist, displays: "No logs found for workspace 'auth'"
**And** log output respects `--output json` flag
**And** large logs are paginated or scrollable
**And** timestamps are displayed in human-readable format

---

## Epic 4: Task Engine & AI Execution

**Goal:** Users can start tasks with `atlas start` and have Claude Code execute them using templates.

### Story 4.1: Task Data Model and Store

As a **developer**,
I want **a task persistence layer with checkpoint support**,
So that **task state is saved after each step and can resume after crashes**.

**Acceptance Criteria:**

**Given** the workspace store exists
**When** I implement `internal/task/store.go`
**Then** the store can:
- Create task directories at `~/.atlas/workspaces/<ws>/tasks/<task-id>/`
- Save task.json with full task state
- Save task.log for execution logs (JSON-lines format)
- Create artifacts/ subdirectory for step outputs
- Read task state with error handling
- List tasks for a workspace
**And** task IDs follow pattern: `task-YYYYMMDD-HHMMSS`
**And** state is saved after each step completion (NFR11)
**And** atomic writes prevent partial state corruption
**And** artifact versioning preserves previous attempts (validation.1.json, validation.2.json)
**And** schema_version is included in task.json

---

### Story 4.2: Task State Machine

As a **developer**,
I want **an explicit state machine for task lifecycle**,
So that **state transitions are validated and auditable**.

**Acceptance Criteria:**

**Given** the task store exists
**When** I implement `internal/task/state.go`
**Then** the state machine enforces valid transitions:
```
Pending → Running
Running → Validating, GHFailed, CIFailed, CITimeout
Validating → AwaitingApproval, ValidationFailed
ValidationFailed → Running, Abandoned
AwaitingApproval → Completed, Running, Rejected
GHFailed → Running, Abandoned
CIFailed → Running, Abandoned
CITimeout → Running, Abandoned
```
**And** invalid transitions return clear errors
**And** each transition is logged with timestamp
**And** `Transition(ctx, task, newStatus) error` validates and applies transition
**And** transition history is stored in task.json
**And** tests verify all valid and invalid transition combinations

---

### Story 4.3: AIRunner Interface and ClaudeCodeRunner

As a **developer**,
I want **an AIRunner interface with ClaudeCodeRunner implementation**,
So that **ATLAS can invoke Claude Code CLI and capture results**.

**Acceptance Criteria:**

**Given** the domain types exist
**When** I implement `internal/ai/runner.go` and `internal/ai/claude.go`
**Then** the AIRunner interface provides:
```go
type AIRunner interface {
    Run(ctx context.Context, req *AIRequest) (*AIResult, error)
}
```
**And** ClaudeCodeRunner:
- Invokes `claude -p --output-format json --model <model>`
- Passes prompts via stdin or -p flag
- Supports `--permission-mode plan` for read-only analysis
- Parses JSON response into AIResult struct
- Captures session_id, duration_ms, num_turns, total_cost_usd
- Handles timeouts via context
- Supports `--max-turns` flag
- Supports `--append-system-prompt` for context injection
**And** errors are wrapped with ErrClaudeInvocation sentinel
**And** retry logic with exponential backoff (3 attempts)
**And** tests mock subprocess execution

---

### Story 4.4: Template Registry and Definitions

As a **developer**,
I want **a template registry with bugfix, feature, and commit templates**,
So that **users can select predefined workflows for common task types**.

**Acceptance Criteria:**

**Given** the AIRunner exists
**When** I implement `internal/template/`
**Then** `registry.go` provides:
- `Get(name string) (*Template, error)`
- `List() []*Template`
- `Register(template *Template)`
**And** `bugfix.go` defines the bugfix template:
- Steps: analyze → implement → validate → git_commit → git_push → git_pr → ci_wait → review
- Branch prefix: "fix"
- Default model: sonnet
**And** `feature.go` defines the feature template:
- Steps: specify → review_spec → plan → tasks → implement → validate → checklist → git_commit → git_push → git_pr → ci_wait → review
- Branch prefix: "feat"
- Default model: opus
- Integrates Speckit SDD
**And** `commit.go` defines the commit template:
- Steps: analyze_changes → smart_commit → git_push
- Garbage detection, logical grouping
- Branch prefix: "chore"
**And** `variables.go` handles template variable expansion
**And** templates are Go code compiled into the binary (not external files)
**And** template behavior is customizable via config (model, branch_prefix, auto_proceed_git)

---

### Story 4.5: Step Executor Framework

As a **developer**,
I want **a StepExecutor interface with implementations for each step type**,
So that **the task engine can execute different step types uniformly**.

**Acceptance Criteria:**

**Given** the AIRunner and templates exist
**When** I implement `internal/template/steps/`
**Then** the StepExecutor interface provides:
```go
type StepExecutor interface {
    Execute(ctx context.Context, task *Task, step *Step) (*StepResult, error)
    Type() StepType
}
```
**And** `ai.go` implements AIExecutor for AI steps (analyze, implement)
**And** `validation.go` implements ValidationExecutor for validation steps
**And** `git.go` implements GitExecutor for git operations
**And** `human.go` implements HumanExecutor for approval checkpoints
**And** `sdd.go` implements SDDExecutor for Speckit steps
**And** `ci.go` implements CIExecutor for CI waiting
**And** each executor:
- Logs execution start/end
- Saves artifacts to task artifacts directory
- Returns StepResult with output, files_changed, duration
- Handles context cancellation
**And** step results are persisted after each execution

---

### Story 4.6: Task Engine Orchestrator

As a **developer**,
I want **a TaskEngine that orchestrates step execution**,
So that **tasks progress through their template steps automatically**.

**Acceptance Criteria:**

**Given** the step executors exist
**When** I implement `internal/task/engine.go`
**Then** the TaskEngine:
- `Start(ctx, workspace, template, description) (*Task, error)` - creates and starts a task
- `Resume(ctx, task) error` - resumes a task from last checkpoint
- `ExecuteStep(ctx, task, step) (*StepResult, error)` - executes a single step
- `HandleStepResult(ctx, task, result) error` - processes result and transitions state
**And** the engine:
- Creates task with unique ID
- Iterates through template steps
- Saves state after each step (checkpoint)
- Auto-proceeds for passing validation/git steps
- Pauses at human steps
- Handles step failures appropriately
- Provides error context for AI retry (FR25)
**And** context propagation flows through all operations
**And** logging includes task_id, step_name, duration_ms
**And** parallel step groups use sync.WaitGroup + errgroup

---

### Story 4.7: Implement `atlas start` Command

As a **user**,
I want **to run `atlas start "description"` to begin a new task**,
So that **I can queue work for ATLAS to execute**.

**Acceptance Criteria:**

**Given** the task engine exists
**When** I run `atlas start "fix null pointer in parseConfig"`
**Then** ATLAS:
- Auto-generates workspace name from description (e.g., "fix-null-pointer")
- Presents template selection menu if `--template` not specified
- Creates workspace with git worktree
- Creates task with the selected template
- Begins executing template steps
- Displays progress with spinner/step indicator
**And** `--template bugfix|feature|commit` selects template directly
**And** `--workspace <name>` specifies custom workspace name (FR11)
**And** `--model <model>` overrides default model (FR26)
**And** if workspace already exists, offers to resume or create new
**And** displays initial status after task starts
**And** returns quickly after task starts (doesn't block until completion)

---

### Story 4.8: Implement Utility Commands

As a **user**,
I want **to run `atlas format`, `atlas lint`, `atlas test`, and `atlas validate` standalone**,
So that **I can run validation commands without a full task workflow**.

**Acceptance Criteria:**

**Given** the validation executor exists
**When** I run `atlas validate`
**Then** executes the full validation suite:
1. Format (magex format:fix)
2. Lint (magex lint) — parallel with test
3. Test (magex test) — parallel with lint
4. Pre-commit (go-pre-commit run --all-files)
**And** displays progress with step names and status
**And** `atlas format` runs only formatters
**And** `atlas lint` runs only linters
**And** `atlas test` runs only tests
**And** each command respects project config for command customization
**And** `--output json` returns structured results
**And** exit code reflects pass/fail status
**And** these commands work without a workspace (run in current directory)

---

### Story 4.9: Speckit SDD Integration

As a **developer**,
I want **Speckit SDD workflows abstracted behind the feature template**,
So that **users get spec-driven development without learning Speckit commands**.

**Acceptance Criteria:**

**Given** the SDDExecutor exists
**When** the feature template executes SDD steps
**Then** the SDDExecutor invokes Speckit via Claude Code:
- `/speckit.specify` → generates spec.md artifact
- `/speckit.plan` → generates plan.md artifact
- `/speckit.tasks` → generates tasks.md artifact
- `/speckit.implement` → executes implementation
- `/speckit.checklist` → generates checklist.md artifact
**And** each step captures output as artifact in task directory
**And** spec is presented for human review after specify step (FR12)
**And** if Speckit is not installed, displays clear error with install instructions
**And** Speckit invocation uses the worktree's .speckit/ directory
**And** errors from Speckit are wrapped appropriately

---

## Epic 5: Validation Pipeline

**Goal:** Tasks are automatically validated with format, lint, and test commands, with clear pass/fail feedback.

### Story 5.1: Validation Command Executor

As a **developer**,
I want **a validation executor that runs configured commands**,
So that **tasks are validated against project quality standards**.

**Acceptance Criteria:**

**Given** validation commands are configured
**When** I implement `internal/validation/executor.go`
**Then** the executor can:
- Run any configured shell command in the worktree directory
- Capture stdout, stderr, and exit code
- Timeout after configured duration (default 5 minutes per command)
- Return structured ValidationResult with pass/fail, output, duration
**And** commands are executed via `exec.CommandContext`
**And** environment variables are inherited from parent process
**And** working directory is set to the worktree path
**And** command output is logged to task.log
**And** errors include the command that failed and its output
**And** tests mock command execution for reliable testing

---

### Story 5.2: Parallel Validation Runner

As a **developer**,
I want **validation commands to run in optimal order with parallelization**,
So that **validation completes as quickly as possible**.

**Acceptance Criteria:**

**Given** the command executor exists
**When** I implement `internal/validation/parallel.go`
**Then** the validation pipeline runs in this order:
1. **Format** (sequential, first) - `magex format:fix`
2. **Lint + Test** (parallel) - `magex lint` and `magex test` simultaneously
3. **Pre-commit** (sequential, last) - `go-pre-commit run --all-files`
**And** parallel execution uses errgroup for error collection
**And** if format fails, subsequent steps are skipped
**And** if any parallel step fails, all results are collected before returning
**And** total validation result aggregates all step results
**And** progress is reported for each step (starting, completed, failed)
**And** validation completes efficiently (parallel steps run concurrently)

---

### Story 5.3: Validation Result Handling

As a **developer**,
I want **validation failures to pause the task and present options**,
So that **users can decide how to proceed when validation fails**.

**Acceptance Criteria:**

**Given** validation has run
**When** validation fails
**Then** the task transitions to `validation_failed` state
**And** the ValidationResult is saved as artifact (validation.json)
**And** previous validation attempts are preserved (validation.1.json, etc.)
**And** the system emits terminal bell notification (FR28)
**And** `atlas status` shows the task as needing attention
**And** the validation output clearly shows:
- Which command(s) failed
- The error output from each failure
- Suggested next actions
**And** when validation passes:
- Task auto-proceeds to next step
- ValidationResult is saved as artifact
- No user intervention required

---

### Story 5.4: Validation Retry with AI Context

As a **user**,
I want **to retry validation with AI attempting to fix the issues**,
So that **ATLAS can automatically resolve validation errors**.

**Acceptance Criteria:**

**Given** a task is in `validation_failed` state
**When** I select "Retry with AI fix" option
**Then** the system:
- Extracts error messages from validation output
- Constructs a prompt including the errors as context
- Invokes Claude Code with: "Previous validation failed with these errors: [errors]. Please fix the issues."
- AI attempts to fix the code
- Re-runs validation after AI changes
**And** the error context is appended to the AI prompt (FR25)
**And** the retry is logged with attempt number
**And** if retry succeeds, task proceeds normally
**And** if retry fails again, returns to `validation_failed` state
**And** maximum retry attempts can be configured (default: 3)

---

### Story 5.5: Manual Fix and Resume Flow

As a **user**,
I want **to fix validation issues manually and then resume the task**,
So that **I can handle cases where AI cannot fix the problem**.

**Acceptance Criteria:**

**Given** a task is in `validation_failed` state
**When** I select "Fix manually" option
**Then** the system displays:
- The worktree path where I can make changes
- The specific errors to fix
- Instructions: "Make your changes, then run 'atlas resume <workspace>'"
**And** the task remains in `validation_failed` state
**And** I can navigate to the worktree and edit files directly
**And** `atlas resume <workspace>` re-runs validation from the current step
**And** if validation now passes, task proceeds
**And** if validation still fails, returns to `validation_failed` with new errors
**And** manual changes are attributed correctly in git history

---

### Story 5.6: Task Abandonment Flow

As a **user**,
I want **to abandon a task while preserving the branch and worktree**,
So that **I can take over manually or revisit later**.

**Acceptance Criteria:**

**Given** a task is in `validation_failed` state (or any error state)
**When** I select "Abandon task" option
**Then** the system:
- Transitions task to `abandoned` state
- Preserves the git branch with all commits
- Preserves the worktree directory
- Preserves all task artifacts and logs
**And** displays: "Task abandoned. Branch '<branch>' preserved at '<worktree-path>'"
**And** the workspace remains in `paused` state (not retired)
**And** I can start a new task in the same workspace if desired
**And** abandoned tasks appear in task history
**And** `atlas workspace destroy` can still clean up if needed later

---

### Story 5.7: Pre-commit Hook Integration

As a **user**,
I want **pre-commit hooks to run as part of validation**,
So that **all project quality checks pass before committing**.

**Acceptance Criteria:**

**Given** go-pre-commit is installed
**When** validation reaches the pre-commit step
**Then** the system runs `go-pre-commit run --all-files`
**And** pre-commit runs after format, lint, and test
**And** pre-commit failures are handled like other validation failures
**And** if pre-commit modifies files (auto-fix), those changes are staged
**And** if go-pre-commit is not installed, the step is skipped with warning
**And** custom pre-commit commands can be configured
**And** pre-commit output is captured in validation results

---

### Story 5.8: Validation Progress and Feedback

As a **user**,
I want **clear progress indication during validation**,
So that **I know what's happening and how long it might take**.

**Acceptance Criteria:**

**Given** validation is running
**When** I observe the terminal output
**Then** I see:
- Current step name: "Running format..." → "Running lint..." → "Running test..."
- Spinner animation during execution
- Step completion status: ✓ or ✗
- Duration for each completed step
- Overall progress: "Step 2/4"
**And** progress updates don't spam the terminal (reasonable update frequency)
**And** UI remains responsive during long-running tests (NFR2)
**And** `--verbose` shows command output in real-time
**And** `--quiet` shows only final pass/fail result

---

## Epic 6: Git & PR Automation

**Goal:** Completed work automatically becomes branches, commits, and pull requests via gh CLI.

### Story 6.1: GitRunner Implementation

As a **developer**,
I want **a GitRunner that wraps git CLI operations**,
So that **ATLAS can perform git operations reliably with proper error handling**.

**Acceptance Criteria:**

**Given** git is installed
**When** I implement `internal/git/runner.go`
**Then** the GitRunner provides:
- `Status(ctx) (*GitStatus, error)` - get working tree status
- `Add(ctx, paths []string) error` - stage files
- `Commit(ctx, message string, trailers map[string]string) error` - commit with trailers
- `Push(ctx, remote, branch string) error` - push to remote
- `CurrentBranch(ctx) (string, error)` - get current branch name
- `CreateBranch(ctx, name string) error` - create and checkout branch
- `Diff(ctx, cached bool) (string, error)` - get diff output
**And** all operations run in specified working directory
**And** errors are wrapped with ErrGitOperation sentinel
**And** output is captured for logging and debugging
**And** operations use context for cancellation
**And** tests use temporary git repositories

---

### Story 6.2: Branch Creation and Naming

As a **user**,
I want **ATLAS to create feature branches with consistent naming**,
So that **my branches follow project conventions and are easy to identify**.

**Acceptance Criteria:**

**Given** a task is starting in a workspace
**When** the system creates a branch
**Then** the branch name follows pattern: `<type>/<workspace-name>`
- bugfix template → `fix/<workspace-name>`
- feature template → `feat/<workspace-name>`
- commit template → `chore/<workspace-name>`
**And** workspace names are sanitized (lowercase, hyphens, no special chars)
**And** if branch already exists, appends timestamp: `fix/auth-20251227`
**And** the branch is created from the configured base branch (default: main)
**And** branch creation is logged with source and target
**And** branch naming patterns are configurable per-template in config

---

### Story 6.3: Smart Commit System

As a **user**,
I want **ATLAS to create meaningful commits with garbage detection**,
So that **my git history is clean and commits are logically grouped**.

**Acceptance Criteria:**

**Given** changes exist in the worktree
**When** the git_commit step executes
**Then** the system:
1. Analyzes all modified/added files
2. Detects garbage files and warns:
   - Debug files (console.log, print statements)
   - Secrets (.env, credentials, API keys)
   - Build artifacts (node_modules, .DS_Store, *.exe)
   - Temporary files (*.tmp, *.bak)
3. Groups files by logical unit (package/directory)
4. Generates commit message via AI based on changes
**And** if garbage detected, pauses with warning and options:
- Remove garbage and continue
- Include anyway (with confirmation)
- Abort and fix manually
**And** commit messages follow conventional commits format
**And** commit includes ATLAS trailers:
```
ATLAS-Task: task-20251226-100000
ATLAS-Template: bugfix
```
**And** commit message is saved as artifact (commit-message.md)

---

### Story 6.4: Push to Remote

As a **user**,
I want **ATLAS to push my branch to remote automatically**,
So that **my work is backed up and ready for PR creation**.

**Acceptance Criteria:**

**Given** commits exist on the local branch
**When** the git_push step executes
**Then** the system runs `git push -u origin <branch>`
**And** the upstream tracking is set
**And** if push fails due to auth, transitions to `gh_failed` state
**And** retry logic with exponential backoff (3 attempts) for transient failures
**And** network timeouts are handled gracefully
**And** push progress is displayed (if available)
**And** `auto_proceed_git: false` in config pauses for confirmation before push

---

### Story 6.5: GitHubRunner and PR Creation

As a **user**,
I want **ATLAS to create pull requests via gh CLI**,
So that **my completed work is ready for review without manual PR creation**.

**Acceptance Criteria:**

**Given** the branch is pushed to remote
**When** the git_pr step executes
**Then** the system:
1. Generates PR description via AI based on:
   - Task description
   - Commit messages
   - Files changed
   - Diff summary
2. Saves description as artifact (pr-description.md)
3. Creates PR via `gh pr create`:
```bash
gh pr create \
  --title "<type>: <summary>" \
  --body "$(cat pr-description.md)" \
  --base main \
  --head <branch>
```
**And** PR title follows conventional commits format
**And** PR body includes:
- Summary of changes
- Files modified with brief descriptions
- Test plan / validation results
- Link to task artifacts (if applicable)
**And** PR URL is captured and displayed
**And** if gh CLI fails, transitions to `gh_failed` state with actionable error
**And** retry logic handles rate limits and transient failures

---

### Story 6.6: CI Status Monitoring

As a **user**,
I want **ATLAS to wait for CI to pass after creating the PR**,
So that **I only review PRs that have passed automated checks**.

**Acceptance Criteria:**

**Given** a PR has been created
**When** the ci_wait step executes
**Then** the system:
1. Polls GitHub Actions API via `gh api` or `gh run list`
2. Checks status of configured required workflows
3. Waits until all required workflows complete
**And** polling interval is configurable (default: 2 minutes)
**And** timeout is configurable (default: 30 minutes)
**And** progress shows: "Waiting for CI... (5m elapsed, checking: CI, Lint)"
**And** if all workflows pass, task proceeds to review step
**And** if any workflow fails, transitions to `ci_failed` state
**And** if timeout exceeded, transitions to `ci_timeout` state
**And** terminal bell notifies when CI completes (pass or fail)

---

### Story 6.7: CI Failure Handling

As a **user**,
I want **clear options when CI fails**,
So that **I can decide whether to fix, retry, or abandon**.

**Acceptance Criteria:**

**Given** CI has failed or timed out
**When** the task is in `ci_failed` or `ci_timeout` state
**Then** the system presents options:
```
? CI workflow "CI" failed. What would you like to do?
  ❯ View workflow logs — Open GitHub Actions in browser
    Retry from implement — AI tries to fix based on CI output
    Fix manually — You fix in worktree, then resume
    Abandon task — End task, keep PR as draft
```
**And** "View workflow logs" opens the GitHub Actions URL in browser
**And** "Retry from implement" extracts CI error output and provides to AI
**And** "Fix manually" preserves state for manual intervention
**And** "Abandon task" keeps the PR (as draft if possible) and branch
**And** CI failure details are saved as artifact (ci-result.json)
**And** the user can re-trigger CI manually in GitHub and use `atlas resume`

---

### Story 6.8: AI Verification Step

As a **user**,
I want **an optional AI verification step that uses a different model to review my implementation**,
So that **I can catch issues before committing, with cross-model validation for higher confidence**.

**Acceptance Criteria:**

**Given** the `--verify` flag is passed to `atlas start`
**When** implementation completes
**Then** the system invokes a secondary AI model to review the implementation
**And** the verifier checks: code correctness, test coverage, garbage files, security issues
**And** if issues found, presents options: auto-fix, manual fix, ignore, view report
**And** `--no-verify` flag disables verification regardless of template default
**And** bugfix template has verification OFF by default
**And** feature template has verification ON by default
**And** verification model is configurable via `verify_model` in config
**And** verification report is saved as artifact (verification-report.md)

**Source:** epic-6-user-scenarios.md - Scenario 1 Step 6, Scenario 5 Step 13

---

### Story 6.9: Wire GitExecutor to internal/git Package

As a **user**,
I want **the git operations in my workflow (commit, push, PR) to actually execute**,
So that **my changes are committed, pushed, and a PR is created automatically**.

**Acceptance Criteria:**

**Given** a task reaches the `git_commit` step
**When** the step executes
**Then** the system runs garbage detection via `GarbageScanner.Scan()`
**And** if garbage found, presents warning with options (remove, include, abort)
**And** executes smart commit via `SmartCommitter.Commit()` with file grouping
**And** for `git_push` step, calls `Pusher.Push()` with retry logic
**And** for `git_pr` step, generates description and calls `HubRunner.CreatePR()`
**And** ATLAS trailers are included in all commits
**And** push/PR failures transition to `gh_failed` state

**Source:** epic-6-traceability-matrix.md - GAP 1, GAP 4

---

### Story 6.10: Wire CIExecutor to HubRunner.WatchPRChecks

As a **user**,
I want **the CI wait step to actually poll GitHub Actions**,
So that **I know when my CI checks pass or fail and can take appropriate action**.

**Acceptance Criteria:**

**Given** a task reaches the `ci_wait` step
**When** the step executes
**Then** the system calls `HubRunner.WatchPRChecks()` to poll GitHub Actions API
**And** polls at configurable intervals (default: 2 minutes)
**And** emits bell notification when checks complete
**And** on success, transitions to next step and saves ci-result.json
**And** on failure, transitions to `ci_failed` state and invokes `CIFailureHandler`
**And** on timeout, transitions to `ci_timeout` state with continue/retry options

**Source:** epic-6-traceability-matrix.md - GAP 2

---

### Story 6.11: Integrate CIFailureHandler into Task Engine

As a **user**,
I want **the task engine to automatically invoke the CI failure handler when CI fails**,
So that **I am presented with options to view logs, retry, fix manually, or abandon**.

**Acceptance Criteria:**

**Given** a step returns `ci_failed` failure type
**When** the task engine processes the result
**Then** the engine invokes `CIFailureHandler` to present options
**And** "View logs" opens GitHub Actions URL in browser
**And** "Retry from implement" resumes from implement step with error context
**And** "Fix manually" shows worktree path and `atlas resume` instructions
**And** "Abandon task" converts PR to draft and transitions to abandoned
**And** `gh_failed` failures (push/PR) present similar options
**And** `ci_timeout` presents additional "Continue waiting" option
**And** `atlas resume` after manual fix continues from ci_wait step

**Source:** epic-6-traceability-matrix.md - GAP 3

---

## Epic 7: Status Dashboard & Monitoring

**Goal:** Users can view their "fleet" of workspaces with a beautiful TUI, live updates, and terminal bell notifications.

### Story 7.1: TUI Style System

As a **developer**,
I want **a centralized style system using Lip Gloss**,
So that **all TUI components have consistent styling**.

**Acceptance Criteria:**

**Given** the Charm libraries are installed
**When** I implement `internal/tui/styles.go`
**Then** the style system provides:
- **Semantic colors** with AdaptiveColor for light/dark terminals:
  - Primary (Blue): `#0087AF` / `#00D7FF`
  - Success (Green): `#008700` / `#00FF87`
  - Warning (Yellow): `#AF8700` / `#FFD700`
  - Error (Red): `#AF0000` / `#FF5F5F`
  - Muted (Gray): `#585858` / `#6C6C6C`
- **State icons** with color mapping:
  - Running: `●` Blue
  - Awaiting Approval: `✓` Green
  - Needs Attention: `⚠` Yellow
  - Failed: `✗` Red
  - Completed: `✓` Dim
  - Pending: `○` Gray
- **Typography styles**: Bold, Dim, Underline, Reverse
**And** NO_COLOR environment variable disables colors (UX-7)
**And** triple redundancy: icon + color + text for all states (UX-8)
**And** styles are exported for use across all TUI components

---

### Story 7.2: Output Interface

As a **developer**,
I want **an Output interface that handles TTY vs JSON output**,
So that **commands can output human-friendly or machine-readable formats**.

**Acceptance Criteria:**

**Given** the style system exists
**When** I implement `internal/tui/output.go`
**Then** the Output interface provides:
```go
type Output interface {
    Success(msg string)
    Error(err error)
    Warning(msg string)
    Info(msg string)
    Table(headers []string, rows [][]string)
    JSON(v interface{})
    Spinner(msg string) Spinner
}
```
**And** `NewOutput(w io.Writer, format string)` creates appropriate implementation
**And** TTY output uses Lip Gloss styling
**And** JSON output produces structured JSON
**And** auto-detection: TTY gets human output, pipe gets JSON
**And** `--output json` flag forces JSON output
**And** `--output text` flag forces human output

---

### Story 7.3: Status Table Component

As a **developer**,
I want **a reusable status table component**,
So that **workspace status is displayed consistently**.

**Acceptance Criteria:**

**Given** the output interface exists
**When** I implement `internal/tui/table.go`
**Then** the table component:
- Renders headers with bold styling
- Aligns columns appropriately (left for text, right for numbers)
- Applies semantic colors to status cells
- Supports variable column widths
- Adapts to terminal width (80/120+ column modes)
**And** the status table displays columns:
```
WORKSPACE   BRANCH          STATUS              STEP    ACTION
auth        feat/auth       ● running           3/7     —
payment     fix/payment     ⚠ awaiting_approval 6/7     approve
```
**And** STATUS column shows icon + colored state
**And** ACTION column shows command to run or `—`
**And** narrow terminals (< 80 cols) use abbreviated headers

---

### Story 7.4: Implement `atlas status` Command

As a **user**,
I want **to run `atlas status` to see all my workspaces**,
So that **I can understand the state of my task fleet at a glance**.

**Acceptance Criteria:**

**Given** workspaces exist with tasks
**When** I run `atlas status`
**Then** the display shows status table with header and footer
**And** workspaces are sorted by status priority (attention first, then running, then others)
**And** footer shows summary and actionable command
**And** `--output json` returns structured JSON (FR46)
**And** empty state shows: "No workspaces. Run 'atlas start' to create one."
**And** command completes in < 1 second (NFR1)

---

### Story 7.5: Watch Mode with Live Updates

As a **user**,
I want **to run `atlas status --watch` for live updates**,
So that **I can monitor my tasks without repeatedly running commands**.

**Acceptance Criteria:**

**Given** workspaces exist
**When** I run `atlas status --watch`
**Then** the display:
- Refreshes every 2 seconds (configurable)
- Clears and redraws the status table
- Shows last update timestamp
- Continues until Ctrl+C
**And** Bubble Tea is used for the TUI application
**And** the UI remains responsive during updates
**And** terminal resize is handled gracefully
**And** `--interval <duration>` configures refresh rate
**And** watch mode works in tmux, iTerm2, Terminal.app, VS Code terminal

---

### Story 7.6: Terminal Bell Notifications

As a **user**,
I want **ATLAS to emit a terminal bell when tasks need attention**,
So that **I'm notified without constantly watching the terminal**.

**Acceptance Criteria:**

**Given** watch mode is running (or task state changes)
**When** any task transitions to an attention-required state:
- `awaiting_approval`
- `validation_failed`
- `gh_failed`
- `ci_failed`
- `ci_timeout`
**Then** the system emits the terminal bell character (`\a` / BEL)
**And** the notification is emitted once per state transition (not on every refresh)
**And** bell is configurable: `notifications.bell: true/false`
**And** bell works in background terminal tabs
**And** bell is suppressed if `--quiet` flag is used

---

### Story 7.7: ATLAS Header Component

As a **user**,
I want **a dramatic ATLAS header on status screens**,
So that **the tool feels polished and professional**.

**Acceptance Criteria:**

**Given** terminal width is 80+ columns
**When** displaying the status screen
**Then** the header shows ASCII art logo with cyan/gradient coloring
**And** header is centered in terminal
**And** narrow terminals (< 80 cols) show simple text: `═══ ATLAS ═══`
**And** header is reusable across status, init, and approval screens

---

### Story 7.8: Progress Dashboard Component

As a **user**,
I want **visual progress bars for active tasks**,
So that **I can see at a glance how far along each task is**.

**Acceptance Criteria:**

**Given** tasks are running
**When** displaying the status dashboard
**Then** active tasks show progress visualization with progress bars
**And** progress bar width adapts to terminal width
**And** step count shows current/total (FR44)
**And** current step name can be shown on hover or with flag
**And** auto-density mode:
  - ≤5 tasks: 2-line mode (progress + details)
  - >5 tasks: 1-line mode (progress + name + step)
**And** progress updates smoothly in watch mode

---

### Story 7.9: Action Indicators

As a **user**,
I want **clear action indicators showing what command to run**,
So that **I never have to guess what to do next**.

**Acceptance Criteria:**

**Given** tasks are in various states
**When** displaying status
**Then** ACTION column shows appropriate action for each state
**And** footer shows copy-paste command: `Run: atlas approve payment`
**And** multiple attention items list all commands
**And** action text is styled appropriately (warning color for attention states)

---

## Epic 8: Interactive Review & Approval

**Goal:** Users can approve, reject, and provide feedback through intuitive interactive flows.

### Story 8.1: Interactive Menu System

As a **developer**,
I want **a reusable interactive menu system using Charm Huh**,
So that **all user decision points have consistent, intuitive interfaces**.

**Acceptance Criteria:**

**Given** Charm Huh is installed
**When** I implement `internal/tui/menus.go`
**Then** the menu system provides:
- `Select(title string, options []Option) (string, error)` - single selection
- `Confirm(message string, defaultYes bool) (bool, error)` - yes/no confirmation
- `Input(prompt string, defaultValue string) (string, error)` - text input
- `TextArea(prompt string, placeholder string) (string, error)` - multi-line input
**And** menus use the established style system (colors, icons)
**And** keyboard navigation: arrow keys, enter to select, q/esc to cancel
**And** menus display action hints: `[↑↓] Navigate  [enter] Select  [q] Cancel`
**And** menus respect terminal width
**And** menus work in tmux, iTerm2, Terminal.app, VS Code terminal

---

### Story 8.2: Approval Summary Component

As a **developer**,
I want **an approval summary component showing task context**,
So that **users have complete information before approving**.

**Acceptance Criteria:**

**Given** a task is awaiting approval
**When** displaying the approval screen
**Then** the summary shows task details including status, step, summary, files changed, validation status, and PR URL
**And** PR URL is clickable (OSC 8 hyperlink) in modern terminals (UX-3)
**And** file changes show insertions/deletions
**And** validation status shows pass/fail summary
**And** summary is generated from task artifacts

---

### Story 8.3: Implement `atlas approve` Command

As a **user**,
I want **to run `atlas approve` to approve completed work**,
So that **I can confirm the task is complete and ready to merge**.

**Acceptance Criteria:**

**Given** tasks are awaiting approval
**When** I run `atlas approve`
**Then** if multiple tasks pending, shows selection menu
**And** after selection (or if only one task), shows approval summary
**And** presents action menu with options: Approve, View diff, View logs, Open PR, Reject, Cancel
**And** "Approve and continue" transitions task to `completed` state
**And** "View diff" shows full git diff in pager
**And** "View logs" shows task execution log
**And** "Open PR in browser" opens PR URL via `open` command
**And** `atlas approve <workspace>` skips selection menu
**And** displays success: "✓ Task approved. PR ready for merge."

---

### Story 8.4: Implement `atlas reject` Command

As a **user**,
I want **to run `atlas reject` to reject work with feedback**,
So that **ATLAS can retry with my guidance**.

**Acceptance Criteria:**

**Given** a task is awaiting approval
**When** I run `atlas reject payment`
**Then** the system presents decision flow with retry or done options
**And** if "Reject and retry" selected, shows feedback form and step selection
**And** feedback is saved to task artifacts
**And** task returns to `running` state at specified step
**And** AI receives feedback as context for retry
**And** if "Reject (done)" selected, task transitions to `rejected` state
**And** rejected tasks preserve branch and worktree for manual work

---

### Story 8.5: Error Recovery Menus

As a **user**,
I want **interactive menus when errors occur**,
So that **I can decide how to proceed without memorizing commands**.

**Acceptance Criteria:**

**Given** a task is in an error state
**When** the error state is displayed or I run `atlas status`
**Then** error-specific menus are presented for validation failed, GitHub operation failed, and CI failed states
**And** all menus follow consistent styling (UX-11, UX-12)
**And** escape routes are always available (Cancel, Abandon)

---

### Story 8.6: Progress Spinners

As a **user**,
I want **progress spinners during long operations**,
So that **I know the system is working and not frozen**.

**Acceptance Criteria:**

**Given** a long operation is running
**When** the operation executes
**Then** a spinner is displayed with current action
**And** spinners use Bubbles spinner component
**And** spinner animation runs at appropriate speed (not too fast/slow)
**And** spinner message updates to reflect current activity
**And** elapsed time is shown for operations > 30 seconds
**And** spinners work correctly in watch mode
**And** `--quiet` mode suppresses spinners (shows only final result)
**And** spinners don't interfere with log output in `--verbose` mode

---

### Story 8.7: Non-Interactive Mode

As a **user**,
I want **to run ATLAS in non-interactive mode**,
So that **I can use it in scripts and CI pipelines**.

**Acceptance Criteria:**

**Given** ATLAS is running in a non-TTY environment or with `--no-interactive`
**When** a decision point is reached
**Then** sensible defaults are applied:
- Template selection: Error (must specify `--template`)
- Workspace name: Auto-generated from description
- Approval: Error (must use `--auto-approve` flag)
- Retry on failure: No retry (fail immediately)
- Destructive actions: Error (must use `--force`)
**And** `--auto-approve` flag auto-approves awaiting tasks
**And** `--force` flag skips confirmation prompts
**And** all prompts that would block return errors with clear messages
**And** exit codes reflect success/failure appropriately
**And** JSON output works correctly in non-interactive mode

---

### Story 8.8: Structured Logging System

As a **developer**,
I want **structured JSON logging for all operations**,
So that **debugging and auditing are straightforward**.

**Acceptance Criteria:**

**Given** zerolog is configured
**When** operations execute
**Then** logs are written to:
- `~/.atlas/logs/atlas.log` for host CLI operations
- `~/.atlas/workspaces/<ws>/tasks/<task-id>/task.log` for task execution
**And** log format is JSON-lines with standard fields: ts, level, event, task_id, workspace_name, step_name, duration_ms
**And** log levels: debug, info, warn, error (NFR29)
**And** sensitive data (API keys, tokens) is NEVER logged (NFR9)
**And** `--verbose` flag sets log level to debug
**And** logs are rotated or capped to prevent disk exhaustion
**And** `atlas workspace logs` can parse and display these logs

---

### Story 8.9: Styled Output System

As a **user**,
I want **all output to be styled with colors and icons**,
So that **information is easy to scan and understand**.

**Acceptance Criteria:**

**Given** the style system exists
**When** any command produces output
**Then** output follows styling conventions:
- Success messages: `✓` green
- Error messages: `✗` red with actionable suggestion
- Warning messages: `⚠` yellow
- Info messages: `ℹ` dim
- Progress: spinner with description
**And** all output uses the centralized style system
**And** output adapts to terminal width
**And** NO_COLOR is respected
**And** `--output json` produces unstyled structured JSON
**And** output is visually consistent across all commands

---

## Summary

| Epic | Title | Stories |
|------|-------|---------|
| 1 | Project Foundation | 6 |
| 2 | CLI Framework & Configuration | 7 |
| 3 | Workspace Management | 7 |
| 4 | Task Engine & AI Execution | 9 |
| 5 | Validation Pipeline | 8 |
| 6 | Git & PR Automation | 11 |
| 7 | Status Dashboard & Monitoring | 9 |
| 8 | Interactive Review & Approval | 9 |

**Grand Total: 66 stories across 8 epics covering all 52 FRs, 33 NFRs, and 31 additional requirements**
