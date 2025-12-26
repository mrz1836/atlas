# ATLAS: AI Task Lifecycle Automation System

- **Version:** 1.1.0-DRAFT
- **Tag:** v1.1-refined
- **Status:** Vision Document

---

## 1. Executive Summary

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

**What ATLAS does:**
- Accepts a task description in natural language
- Coordinates AI agents to analyze, implement, and validate code
- Integrates with Speckit for specification-driven development
- Produces Git branches, commits, and pull requests

**Built with:**
- Pure Go 1.24+ with minimal dependencies
- Direct integration with Claude API (anthropic-sdk-go)
- Charm libraries for beautiful terminal UX (lipgloss, huh, bubbles)
- Git worktrees for parallel workspace isolation

**What ATLAS is not:**
- Not a "virtual employee"—it's a workflow automation tool that requires human oversight
- Not a universal PM integration—GitHub only in v1
- Not language-agnostic—Go projects only in v1
- Not cross-platform—macOS only in v1 (Terminal.app, iTerm2, modern terminals)
- Not magic—AI makes mistakes, validation catches some, humans catch the rest
- Not a learning system—no automatic project rules updates in v1

**Who it's for:**
- Go developers who want to accelerate routine development tasks
- Teams that already use GitHub Issues/PRs for workflow
- Developers comfortable with CLI tools

**Explicit scope (v1):**
- Single-repository Go projects
- GitHub as the sole integration point
- Local execution with Git worktrees for parallel work
- Claude as the AI backend (interface allows future provider additions)
- Integration with Speckit for SDD workflows

---

## 2. Core Principles

### Git is the Backbone

Git is not just version control—it's the audit trail, delivery mechanism, and source of truth. Every ATLAS action produces Git artifacts: branches, commits with machine-parseable trailers, and pull requests. If it's not in Git, it didn't happen.

### Text is Truth

All state is stored as human-readable text files. JSON for structured data, Markdown for prose, YAML for configuration. Templates and workflows are Go code compiled into the binary—type-safe, testable, and dependable. No databases, no binary formats. You can always `cat` your way to understanding what ATLAS did.

### Human Authority at Checkpoints

AI proposes, humans dispose. Validation tasks (lint, test) auto-proceed on success. Code changes always pause for approval. No unsupervised merges, ever.

### Ship Then Iterate

Start with the simplest thing that works. Add complexity only when real usage demands it. If a feature isn't needed for the next task, it doesn't exist yet.

### Transparent State

Every file ATLAS creates is inspectable. No hidden state, no opaque databases. Debug by reading files. Trust by verifying.

---

## 3. Implementation Stack

ATLAS is a pure Go application targeting Go 1.24+.

### Philosophy

- **Pure Go when possible**: Reduce attack surface and dependency rot
- **Best-in-class when needed**: Use proven libraries for complex problems
- **Minimal dependencies**: Every dep must justify its existence

### Core Dependencies

| Purpose | Library | Rationale |
|---------|---------|-----------|
| CLI Framework | `spf13/cobra` | Industry standard for complex CLIs |
| Interactive Forms | `charmbracelet/huh` | Beautiful wizard-style prompts |
| Terminal Styling | `charmbracelet/lipgloss` | Modern terminal UI components |
| Progress/Spinners | `charmbracelet/bubbles` | Animated feedback |
| Configuration | `spf13/viper` | Multi-source config, pairs with Cobra |
| Structured Logging | `rs/zerolog` | Zero-allocation, JSON-native |
| Claude API | `anthropics/anthropic-sdk-go` | Official Claude SDK |
| GitHub API | `google/go-github` | Official GitHub v3/v4 client |

### AI Architecture

```
┌─────────────────────────────────────────────────┐
│                  ATLAS Core                     │
├─────────────────────────────────────────────────┤
│  ModelClient Interface                          │
│  └─ ClaudeClient (anthropic-sdk-go)             │
│      └─ Primary provider                        │
├─────────────────────────────────────────────────┤
│  SDD Framework Integration                      │
│  └─ Speckit (uv tool, /speckit.* commands)      │
└─────────────────────────────────────────────────┘
```

**Why direct SDK integration:**
- Full control over request/response handling
- No framework abstraction overhead
- Easy debugging—read your code, not framework internals
- Type safety from official SDKs

**Interface extensibility:** The `ModelClient` interface allows adding providers (Gemini, etc.) without core changes. Deferred until needed.

### What We Don't Use

- No database (all state is file-based: JSON, YAML, Markdown)
- No web framework (no HTTP server in v1)
- No dependency injection framework (explicit wiring)
- No LangChain/ADK/Genkit (direct API integration is simpler for v1)

---

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              ATLAS CLI                                  │
│                                                                         │
│  atlas init | start | status | approve | reject | resume |  workspace   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────┐  ┌─────────────────────────┐           │
│  │  Worktree: auth-feature     │  │  Worktree: payment-fix  │           │
│  │  ~/projects/repo-auth/      │  │  ~/projects/repo-pay/   │           │
│  │  ┌───────────────────────┐  │  │  ┌───────────────────┐  │           │
│  │  │ Branch: feat/auth     │  │  │  │ Branch: fix/pay   │  │           │
│  │  │ (your code only)      │  │  │  │ (your code only)  │  │           │
│  │  └───────────────────────┘  │  │  └───────────────────┘  │           │
│  └─────────────────────────────┘  └─────────────────────────┘           │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────┐            │
│  │                    ~/.atlas/ (Host)                     │            │
│  │  config.yaml                                            │            │
│  │  workspaces/                                            │            │
│  │    auth/   → tasks/, artifacts/, logs/                  │            │
│  │    payment/ → tasks/, artifacts/, logs/                 │            │
│  └─────────────────────────────────────────────────────────┘            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

**Data flow:**
1. User runs `atlas start "fix the bug" --workspace bugfix-ws`
2. ATLAS creates Git worktree at `~/projects/repo-bugfix-ws/`
3. Task JSON created in `~/.atlas/workspaces/bugfix-ws/tasks/`
4. Task Engine executes template steps (AI, validation, git, human)
5. Claude invoked via SDK for AI steps
6. Git operations happen in worktree directory
7. Human approves/rejects at checkpoints

---

## 5. Components

### 5.1 CLI Interface

Seven commands cover 95% of usage:

```bash
atlas init                              # Initialize ATLAS configuration
atlas start "description" [--workspace] # Start task in workspace
atlas status                            # Show all workspaces and tasks
atlas approve [workspace]               # Approve pending work
atlas reject [workspace]                # Reject with interactive feedback
atlas resume [task-id]                  # Resume interrupted task
atlas workspace <list|retire|destroy|logs>  # Manage workspaces
```

**Workspace-aware behavior:**
- `atlas start "desc"` — Auto-generates workspace name from description
- `atlas start "desc" --workspace feat-x` — Uses explicit workspace name
- `atlas status` — Shows all workspaces and their task states
- `atlas workspace logs <name> [--follow]` — Stream task logs from workspace
- `atlas workspace retire <name>` — Mark workspace as complete (after PR merged)
- `atlas workspace destroy <name>` — Full cleanup: deletes ATLAS state AND git worktree

**Flags (all commands):**
- `--output json|text` — Machine or human output (default: text for TTY)
- `--verbose` — Debug logging
- `--quiet` — Errors only

**Exit codes:**
- `0` — Success
- `1` — Execution error
- `2` — Invalid input

**Piping:**
Commands accept JSON on stdin and produce JSON on stdout, enabling composition:
```bash
echo '{"description": "fix null check"}' | atlas start --output json
```

#### Installation

ATLAS is installed globally via Go's package manager:

```bash
go install github.com/owner/atlas@latest
```

Then run the setup wizard:

```bash
atlas init
```

That's it. ATLAS handles everything else.

#### Dependency Management

ATLAS checks for required tools and manages a small set of ATLAS-owned dependencies.

**Dependencies:**

| Tool | Purpose | Required Version | Managed by ATLAS? |
|------|---------|------------------|-------------------|
| Go | Runtime | 1.24+ | No (detect only) |
| Git | Version control | 2.20+ | No (detect only) |
| GitHub CLI (`gh`) | PR operations | 2.20+ | No (detect only) |
| uv | Python tool runner | 0.5.x | No (detect only) |
| mage-x | Build automation | v0.3.0 | Yes (install/upgrade) |
| go-pre-commit | Pre-commit hooks | v0.1.0 | Yes (install/upgrade) |
| Speckit | SDD framework | 1.0.0 | Yes (install/upgrade) |

**Why this split:**
- **Detect only:** Standard tools users install via their preferred method (brew, apt, etc.)
- **Managed:** ATLAS-ecosystem tools where we control the upgrade experience

**Detection flow:**
```
atlas init
  │
  ├─► Scan: Check required tools
  │   └─► Show status (installed ✓, missing ✗, outdated ⚠)
  │
  ├─► If missing required tools:
  │   └─► Error with install instructions (user installs manually)
  │
  ├─► If managed tools missing/outdated:
  │   └─► Prompt: "Install/upgrade ATLAS tools? [Y/n]"
  │
  └─► Configure: AI providers, GitHub auth, templates
```

**What you'll see:**
```
Checking dependencies...

  TOOL            STATUS      VERSION     REQUIRED    MANAGED
  Go              ✓ installed 1.24.1      1.24+       —
  Git             ✓ installed 2.43.0      2.20+       —
  gh              ✓ installed 2.45.0      2.20+       —
  uv              ✓ installed 0.5.12      0.5.x       —
  mage-x          ⚠ outdated  0.2.1       0.3.0       ATLAS
  go-pre-commit   ✓ installed 0.1.0       0.1.0       ATLAS
  Speckit         ✗ missing   —           1.0.0       ATLAS

Install/upgrade ATLAS-managed tools? [Y/n] y
  Upgrading mage-x...     ✓
  Installing Speckit...   ✓

All dependencies ready.
```

#### Self-Upgrade

ATLAS can upgrade itself and managed tools:

```bash
atlas upgrade              # Upgrade ATLAS + managed tools
atlas upgrade --check      # Show available updates without installing
atlas upgrade speckit      # Upgrade Speckit specifically
```

#### Speckit Upgrades

Speckit upgrades preserve your `constitution.md` (a Speckit-specific file that defines project governance principles and coding standards):

```
Upgrading Speckit 0.9.0 → 1.0.0...
  Backing up constitution.md → ~/.atlas/backups/speckit-<timestamp>/
  Installing Speckit 1.0.0...
  Restoring constitution.md from backup...
  Upgrade complete.
```

Flow: backup constitution → `uv tool upgrade speckit` → restore constitution from backup.

**First-time setup wizard** (launched by `atlas init`):

The interactive wizard (powered by Charm huh) configures:
- AI provider selection and API credentials
- GitHub authentication
- Default template selection
- SDD framework preferences

Configuration stored in `~/.atlas/config.yaml`.

**Config Precedence (highest to lowest):**
1. CLI flags
2. Project config (`.atlas/config.yaml`)
3. Global config (`~/.atlas/config.yaml`)
4. Template defaults (compiled into binary)

### 5.2 Task Engine

Tasks are the atomic units of work. State lives in `~/.atlas/workspaces/<name>/tasks/` as JSON files.

**Task lifecycle:**

```
                                ┌──────────────┐
                                │   pending    │
                                └──────┬───────┘
                                       │ start
                                       ▼
                                ┌──────────────┐
                       ┌───────►│   running    │◄─────────┐
                       │        └──────┬───────┘          │
                       │               │ ai complete      │ retry
                       │               ▼                  │
                       │        ┌──────────────┐          │
                       │        │  validating  │──────────┤
                       │        └──────┬───────┘          │
                       │               │                  │
                       │     ┌─────────┴─────────┐        │
                       │     │ pass              │ fail   │
                       │     ▼                   ▼        │
                ┌──────┴──────────┐      ┌────────────────┴┐
                │awaiting_approval│      │validation_failed│
                └──────┬──────────┘      └─────────────────┘
                       │                        │
             ┌─────────┴─────────┐              │ abandon
             │ approve           │ reject       ▼
             ▼                   ▼         ┌───────────┐
      ┌───────────┐         ┌──────────┐   │ abandoned │
      │ completed │         │ rejected │   └───────────┘
      └───────────┘         └──────────┘
```

**Key concepts:**
- **Validation failures** (lint/test errors) → pause for human decision
- **Task retry** = catastrophic failure only (API down, crash, network timeout)
- **Resume** = continue interrupted task from last checkpoint

**Validation failed menu (interactive):**
```
? Validation failed. What would you like to do?
  ❯ Retry this step — AI tries again with error context
    Retry from earlier step — Go back to analyze/implement
    Fix manually and resume — You fix, then 'atlas resume'
    Abandon task — End task, keep branch for manual work
```

**State transitions:**
| From | To | Trigger |
|------|-----|---------|
| `pending` | `running` | Task execution starts |
| `running` | `validating` | AI produces output |
| `validating` | `awaiting_approval` | All validations pass |
| `validating` | `validation_failed` | Validation fails (lint/test errors) |
| `validation_failed` | `running` | Human chooses retry |
| `validation_failed` | `abandoned` | Human chooses abandon |
| `awaiting_approval` | `completed` | Human runs `atlas approve` |
| `awaiting_approval` | `rejected` | Human runs `atlas reject` |
| `running` | `gh_failed` | GitHub operation fails after retries |
| `gh_failed` | `running` | Human resolves issue and retries |
| `gh_failed` | `abandoned` | Human chooses abandon |

**GitHub failure handling:**

GitHub operations (`gh pr create`, `git push`) automatically retry 3x with exponential backoff for transient errors. After exhausting retries, the task enters `gh_failed` state:

```
? GitHub operation failed: gh pr create returned "rate limit exceeded"
  ❯ Retry now — Try the operation again
    Fix and retry — You fix the issue, then retry
    Abandon task — End task, keep branch for manual work
```

**Triggers for human intervention:**
- Authentication failures (expired token, missing permissions)
- Rate limits exceeded
- Protected branch rejection
- Network timeouts after retries

**Resume capability:**
```bash
atlas resume <task-id>     # Continue interrupted task
atlas resume               # Resume most recent task in workspace
```

Tasks checkpoint after each step, enabling resume after crashes or interruptions.

**Step types:**
| Type | Executor | Auto-proceeds? | Configurable? |
|------|----------|----------------|---------------|
| ai | Claude SDK | No — pauses for approval after AI steps | No |
| validation | Configured commands | Yes if passing; pauses on failure | No |
| git | Git CLI operations | Default: Yes (configurable via `auto_proceed_git`) | Yes |
| human | Developer action | N/A — always waits for human | No |
| sdd | Speckit slash commands | No — output requires review | No |

**Note:** "Auto-proceeds" means the task continues without pausing for human input. Git operations (commit, push, PR) auto-proceed by default but can be configured to pause via `auto_proceed_git: false` in template config.

**Task JSON structure:**
```json
{
  "id": "task-20251226-100000",
  "template": "bugfix",
  "status": "running",
  "workspace": "fix-null-pointer",
  "created_at": "2025-12-26T10:00:00Z",
  "input": {
    "description": "Fix null pointer in parseConfig",
    "files": ["pkg/config/parser.go"]
  },
  "current_step": 1,
  "steps": [
    {"name": "analyze", "status": "completed", "output": "..."},
    {"name": "implement", "status": "running"},
    {"name": "validate", "status": "pending"}
  ],
  "git": {
    "repo": "owner/project",
    "base_branch": "main",
    "work_branch": "fix/null-pointer-parseconfig"
  }
}
```

**Location:** `~/.atlas/workspaces/fix-null-pointer/tasks/task-20251226-100000/task.json`

**Task ID format:** `task-YYYYMMDD-HHMMSS` — Timestamp-based, human-readable, sorts chronologically.

**State File Integrity:**

All JSON state files include safety mechanisms:
- **Schema versioning:** Every file includes `"schema_version": 1` for forward compatibility
- **Atomic writes:** Write to temp file, then rename (atomic on POSIX)
- **File locking:** Use `flock` for concurrent workspace access

```json
{
  "schema_version": 1,
  "id": "task-20251226-100000",
  ...
}
```

**Templates:**

Pre-defined task chains for common workflows, implemented as Go code compiled into the ATLAS binary. This approach provides type safety, compile-time validation, testability, and IDE support. Users customize template behavior through configuration files (`~/.atlas/config.yaml` and `.atlas/config.yaml`), not by modifying templates directly. See [templates.md](templates.md) for comprehensive documentation.

Built-in templates:
- `bugfix` — Analyze, implement, validate, commit, PR
- `feature` — Speckit SDD: specify, plan, implement, validate, PR
- `commit` — Smart commits: garbage detection, logical grouping, message generation

Utility templates (lightweight, single-purpose):
- `format` — Run code formatting only
- `lint` — Run linters only
- `test` — Run tests only
- `validate` — Full validation suite (format, then lint+test in parallel)

**Parallel step execution:** Steps can be grouped with `parallel_group` for concurrent execution (e.g., lint and test run together after format completes).

**Template customization (via config, not code):**
```yaml
# .atlas/config.yaml
templates:
  bugfix:
    auto_proceed_git: true      # Git operations don't pause for approval
    model: claude-sonnet-4-5-20250916
  feature:
    auto_proceed_git: false     # Pause before PR creation
    model: claude-opus-4-5-20251101
```

Users customize validation commands, model selection, and auto-proceed behavior via configuration files. Templates themselves are immutable Go code.

### 5.3 Model Client Layer

ATLAS uses a simple interface for AI model integration:

```go
type ModelClient interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}

type CompletionRequest struct {
    System    string
    Messages  []Message
    MaxTokens int
}

type CompletionResponse struct {
    Content    string
    StopReason string
}
```

**Implementation:**
- `ClaudeClient` — Uses anthropic-sdk-go

The interface allows adding providers (Gemini, OpenAI, etc.) in the future without core changes.

**Configuration:**
```yaml
# ~/.atlas/config.yaml
model:
  provider: claude
  model: claude-sonnet-4-5-20250916
  api_key_env: ANTHROPIC_API_KEY
  timeout: 30m  # Long-running tasks need room
```

**Safeguards:**
- `timeout: 30m` — Maximum time for any single AI step
- `max_ai_retries: 3` — AI step failures before breaking to human intervention
- `max_validation_loops: 5` — Validation retry cycles before forcing human intervention

No token counting or cost tracking in v1—these safeguards prevent runaway execution.

**AI Output Schema:**

All AI steps produce structured JSON output, validated before proceeding:

```json
{
  "schema_version": 1,
  "step": "analyze",
  "output": {
    "summary": "Root cause: cfg.Options accessed without nil check",
    "root_cause": "Missing nil check in parseConfig",
    "files_to_modify": ["pkg/config/parser.go"],
    "approach": "Add nil check before accessing Options field"
  }
}
```

- AI prompted to return JSON matching step-specific schema
- ATLAS validates output against schema before proceeding
- Parse/validation failure → `validation_failed` state (human decides)

### 5.4 SDD Framework Integration

ATLAS integrates with SDD frameworks as external tools, not abstractions. The frameworks do the specification work; ATLAS orchestrates when to invoke them.

#### Speckit Integration

**What is Speckit:** GitHub's spec-driven development toolkit providing structured specification, planning, and implementation workflows.

**Prerequisites:** uv must be installed (ATLAS detects but doesn't install it).

**Installation:** ATLAS manages Speckit installation and upgrades:
```bash
# ATLAS internally runs:
uv tool install speckit
```

**Slash commands (passed to Claude):**

Slash commands are prompt-based actions. ATLAS passes them to Claude along with user-provided context, then captures the structured output as artifacts.

| Command | Purpose |
|---------|---------|
| `/speckit.constitution` | Create project governing principles |
| `/speckit.specify` | Define requirements and user stories |
| `/speckit.plan` | Create technical implementation strategy |
| `/speckit.tasks` | Generate actionable task lists |
| `/speckit.implement` | Execute tasks to build features |
| `/speckit.checklist` | Generate quality validation checklists |

**Execution model:** When a template step specifies an SDD command, ATLAS constructs a prompt containing the slash command and any user context, sends it to Claude, validates the response matches the expected schema, and stores the output as an artifact (e.g., `~/.atlas/workspaces/<name>/artifacts/spec.md`).

#### When to Use Speckit

| Use Case | Recommended | Rationale |
|----------|-------------|-----------|
| Bug fixes | No SDD | Overkill; just analyze + fix |
| Small features | Speckit | Lightweight, focused specs |
| Large features | Speckit | Full specification + planning |

### 5.5 Workspaces and Git Worktrees

ATLAS uses two related concepts (see Glossary for definitions):
- **Workspace**: ATLAS state (`~/.atlas/workspaces/<name>/`) — tasks, artifacts, logs
- **Worktree**: Git working directory (`~/projects/<repo>-<name>/`) — your code

This separation enables working on multiple features simultaneously without interference.

**Why Git worktrees:**
- **Native Git feature** — No additional dependencies
- **Parallel branches** — Each worktree is a separate working directory
- **Clean isolation** — No file conflicts between features
- **Lightweight** — Shares Git objects with main repo
- **Easy cleanup** — `git worktree remove` handles everything

**Worktree lifecycle:**
```bash
# ATLAS internally runs:
git worktree add ~/projects/myrepo-auth feat/auth  # Create
git worktree list                                   # List
git worktree remove ~/projects/myrepo-auth         # Cleanup
```

**State separation:**
- **ATLAS state** lives in `~/.atlas/workspaces/<name>/` (tasks, artifacts, logs)
- **Git worktree** stays clean (just your code + .speckit if using SDD)

This separation means:
- Task state survives accidental worktree deletion
- Resume works after crashes
- No `.atlas/` pollution in your repo

**Workspace lifecycle:**

A workspace can contain multiple tasks. This happens when:
- **Rejection + retry**: User rejects a task, starts fresh approach in same workspace
- **Abandonment + restart**: Task abandoned, new task with different parameters
- **Iterative work**: Analysis task, then implementation task

Workspaces have three states:
- `active` — Work in progress
- `paused` — No running tasks, can resume later
- `retired` — Work complete (PR merged), preserved for reference

Workspaces are retired manually with `atlas workspace retire <name>` after verifying the PR was merged. Use `atlas workspace destroy <name>` for full cleanup (deletes both ATLAS state and the git worktree).

**Workspace JSON (workspace.json):**
```json
{
  "name": "auth-feature",
  "repo_path": "/Users/me/projects/myrepo",
  "worktree_path": "/Users/me/projects/myrepo-auth",
  "branch": "feat/auth",
  "status": "active",
  "created_at": "2025-12-26T10:00:00Z",
  "tasks": [
    {"id": "task-20251226-100000", "status": "rejected", "template": "feature"},
    {"id": "task-20251226-143022", "status": "running", "template": "feature"}
  ]
}
```

**Workspace manager:**
```go
type Workspace struct {
    Name         string       `json:"name"`
    RepoPath     string       `json:"repo_path"`      // Original repo
    WorktreePath string       `json:"worktree_path"`  // Created worktree
    Branch       string       `json:"branch"`
    CreatedAt    time.Time    `json:"created_at"`
    Status       string       `json:"status"`         // active, paused, retired
    Tasks        []TaskRef    `json:"tasks"`          // Task history
}

type TaskRef struct {
    ID       string `json:"id"`
    Status   string `json:"status"`
    Template string `json:"template"`
}
```

**CLI commands:**
```bash
# Start two features in parallel
atlas start "add user authentication" --workspace auth
atlas start "fix payment processing" --workspace payment

# Check status
atlas status
# Output:
# WORKSPACE   BRANCH                STATUS              STEP
# auth        feat/add-auth         running             3/7 Implementing
# payment     fix/payment-timeout   awaiting_approval   6/7 Review PR

# Manage workspaces
atlas workspace list
atlas workspace logs auth --follow
atlas workspace destroy payment  # After merge
```

**Naming convention:**
- Auto-generated: `fix-null-pointer-config` (from description)
- Override: `--workspace my-custom-name`
- Branch: `<type>/<workspace-name>` (e.g., `fix/null-pointer-config`)

**Conflict detection:**
- **Worktree path conflict**: If `~/projects/<repo>-<name>/` already exists, ATLAS appends a numeric suffix (`-2`, `-3`, etc.)
- **Branch conflict**: If branch `<type>/<name>` already exists, ATLAS appends a timestamp suffix (e.g., `fix/auth-20251226`)
- Both cases display a clear warning message explaining the renamed path/branch

### 5.6 Validation

Validation commands are configurable per-project.

**Configuration:**
```yaml
# .atlas/config.yaml (project-level)
validation:
  # Default commands for all templates
  default:
    - magex format:fix
    - magex lint
    - magex test
    - go-pre-commit run --all-files

  # Template-specific overrides
  templates:
    bugfix:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    feature:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files

  # Custom hooks
  hooks:
    pre_pr:
      - magex integration-test
```

**Validation workflow:**
```
Code Generated
    │
    ▼
┌─────────────────────┐
│ Run Validations     │──── Fail ──► validation_failed (human decides)
│ (configurable)      │
└─────────┬───────────┘
          │ Pass
          ▼
┌─────────────────────┐
│ Run pre_commit hooks│
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ Commit & Push       │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ Run pre_pr hooks    │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ Open PR             │
└─────────────────────┘
```

### 5.7 Git Operations

All code delivery happens through Git. ATLAS never modifies files without creating commits.

**Commit trailers:**
Every ATLAS commit includes machine-parseable trailers:

```
fix: handle nil config options in parseConfig

Added nil check before accessing cfg.Options.
Added test case for nil options scenario.

ATLAS-Task: task-20251226-100000
ATLAS-Template: bugfix
```

**Smart commit grouping:**
The `git:smart_commit` action:
1. Groups modified files by package/directory
2. Separates source, test, and config changes
3. Creates one commit per logical unit
4. Generates meaningful commit messages with AI assistance

**Branch naming:**
```
<type>/<workspace-name>
fix/null-pointer-parseconfig
feat/add-user-authentication
```

**PR creation:**
Uses the diff to generate a detailed PR description with AI help, stored in the task's artifacts folder.
```bash
gh pr create \
  --title "fix: handle nil config options" \
  --body "$(cat ~/.atlas/workspaces/<ws>/tasks/<task-id>/artifacts/pr-description.md)" \
  --base main \
  --head fix/null-pointer-parseconfig
```

### 5.8 Project Rules Update (Post-MVP)

ATLAS can learn from completed work by suggesting updates to project rules files (AGENTS.md, constitution.md, etc.).

**Deferred to post-MVP.** Core concept: after task approval, optionally analyze what was learned and propose updates to project guidance files.

Key ideas preserved for future implementation:
- Explicit, human-controlled process (not automatic)
- Rules stay version-controlled in the project
- AI proposes minimal, targeted updates
- Human reviews diff before applying

### 5.9 Observability

**Log locations:**
```
~/.atlas/
├── logs/
│   └── atlas.log                              # Host CLI operations
└── workspaces/
    └── auth/
        └── tasks/
            ├── task-20251226-100000/
            │   └── task.log                   # Full task execution log
            └── task-20251226-143022/
                └── task.log
```

**Log format (JSON-lines):**
```json
{"ts":"2025-12-26T10:00:00Z","level":"info","event":"task_start","task_id":"task-20251226-100000"}
{"ts":"2025-12-26T10:00:05Z","level":"info","event":"model_invoke","provider":"claude","tokens_in":15000}
{"ts":"2025-12-26T10:00:45Z","level":"info","event":"model_complete","tokens_out":2500,"duration_ms":40000}
{"ts":"2025-12-26T10:00:46Z","level":"info","event":"validation_start","command":"golangci-lint run"}
{"ts":"2025-12-26T10:00:52Z","level":"info","event":"validation_complete","passed":true}
```

**Debugging commands:**
```bash
# What's happening right now?
atlas status --verbose

# Full log for a specific task
cat ~/.atlas/workspaces/auth/tasks/task-20251226-100000/task.log

# Tail workspace logs live
atlas workspace logs auth --follow

# Parse logs with jq
cat ~/.atlas/workspaces/*/tasks/*/task.log | jq 'select(.event=="model_complete")'
```

### 5.10 User Experience

ATLAS prioritizes clear, actionable feedback at every step. The CLI is designed so you always know what's happening, when action is needed, and how to respond.

#### Status Display

**`atlas status` — Snapshot with Action Hints**

```
┌──────────────────────────────────────────────────────────────────────┐
│  ATLAS Status                                                        │
├──────────────────────────────────────────────────────────────────────┤
│  WORKSPACE   BRANCH         STATUS              STEP    ACTION       │
│  auth        feat/auth      running             3/7     —            │
│  payment     fix/payment    ⚠ awaiting_approval 6/7     approve      │
└──────────────────────────────────────────────────────────────────────┘

⚠ 1 task needs your attention. Run: atlas approve payment
Tip: Use 'atlas status --watch' for live updates
```

Key elements:
- **ACTION column** — Shows exact command to run when action needed
- **Visual indicator (⚠)** — Highlights tasks requiring attention
- **Actionable footer** — Copy-paste command to proceed

**`atlas status --watch` — Live Mode**

- Refreshes every 2 seconds
- Terminal bell (BEL character) when any task transitions to `awaiting_approval`
- Shows timestamp of last update
- Ctrl+C to exit

#### Approval Flow

**`atlas approve [workspace]` — Interactive Review**

If no workspace specified and multiple tasks pending, interactive selection:
```
? Select task to approve:
  ❯ payment (fix/payment) - Review PR
    auth (feat/auth) - Review specification
```

Then shows task summary and interactive menu:
```
┌─────────────────────────────────────────────────────────────────────┐
│  Task: payment                                         fix/payment  │
├─────────────────────────────────────────────────────────────────────┤
│  Status: awaiting_approval                                          │
│  Step: 6/7 - Review PR                                              │
│                                                                     │
│  Summary:                                                           │
│    Fixed null pointer in parseConfig by adding nil check.           │
│                                                                     │
│  Files changed:                                                     │
│    • pkg/config/parser.go (+12, -3)                                 │
│    • pkg/config/parser_test.go (+45, -0)                            │
│                                                                     │
│  PR: https://github.com/user/repo/pull/42                           │
├─────────────────────────────────────────────────────────────────────┤
│  ? What would you like to do?                                       │
│    ❯ Approve and continue                                           │
│      Reject (with feedback)                                         │
│      View diff                                                      │
│      View logs                                                      │
│      Open PR in browser                                             │
│      Cancel                                                         │
└─────────────────────────────────────────────────────────────────────┘
```

Interactive menus (powered by charmbracelet/huh) guide you through every decision. No keyboard shortcuts to memorize—just arrow keys and enter.

#### Rejection Flow

**`atlas reject [workspace]` — Interactive Decision Flow**

When rejecting, ATLAS presents an interactive decision flow:

```
┌─────────────────────────────────────────────────────────────────────┐
│  Rejecting: payment                                    fix/payment  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ? What would you like to do?                                       │
│    ❯ Reject and retry — AI tries again with your feedback           │
│      Reject (done) — End task, keep code for manual work            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Path A: Reject and Retry**

If "Reject and retry" is selected:
```
? What needs to change?
  ❯ Code quality issues
    Missing tests
    Wrong approach
    Incomplete implementation
    Other (provide details)

? Which step should retry?
  ❯ implement — Re-implement with your feedback (Recommended)
    analyze — Re-analyze the problem first
    Full restart — Start from the beginning

Additional guidance for AI:
> Focus on the timeout case - the current implementation doesn't handle
> network timeouts properly. See pkg/http/client.go:245 for context.

Retrying task with feedback...
  → Returning to 'implement' step
  → AI will receive your feedback as context
```

The task returns to `running` state with feedback context injected into the AI prompt.

**Path B: Reject (Done)**

If "Reject (done)" is selected:
```
? Why are you rejecting this task?
  ❯ Code quality issues
    Missing tests
    Wrong approach
    Incomplete implementation
    Other (provide details)

Additional feedback (optional):
> The approach is fundamentally wrong for our architecture.

Task rejected.
  → Branch 'fix/payment' preserved with current changes
  → Feedback stored for future learning
  → Run 'atlas start "..." --workspace payment' to try fresh approach
```

The task ends. Branch and code remain for manual intervention. Feedback is stored in the task's JSON file for future reference.

---

## 6. Workflow Examples

### Bugfix Workflow

```
User: atlas start "fix null pointer panic in parseConfig when options is nil"
  │
  ├─► Step 1: analyze (AI)
  │   └─► Output: "Root cause: cfg.Options accessed without nil check"
  │
  ├─► Step 2: implement (AI)
  │   └─► Output: Code changes + new test case
  │
  ├─► Step 3: validate (Auto)
  │   ├─► magex format:fix ✓
  │   ├─► magex lint ✓
  │   ├─► magex test ✓
  │   ├─► go-pre-commit run --all-files ✓
  │   └─► Auto-proceeds (validation passed)
  │
  ├─► Step 4: git_commit (Auto)
  │   └─► Creates branch, commits with trailers
  │
  ├─► Step 5: git_push (Auto)
  │   └─► Pushes to remote
  │
  ├─► Step 6: git_pr (Auto)
  │   └─► gh pr create
  │
  └─► Step 7: review (Human)
      └─► atlas approve OR atlas reject "reason"
```

### Feature Workflow (Speckit SDD)

```
User: atlas start "add retry logic to HTTP client" --template feature
  │
  ├─► Step 1: specify (SDD - Speckit)
  │   └─► /speckit.specify → spec.md
  │
  ├─► Step 2: review_spec (Human)
  │   └─► Approve or reject with feedback
  │
  ├─► Step 3: plan (SDD - Speckit)
  │   └─► /speckit.plan → plan.md
  │
  ├─► Step 4: tasks (SDD - Speckit)
  │   └─► /speckit.tasks → tasks.md
  │
  ├─► Step 5: implement (SDD - Speckit)
  │   └─► /speckit.implement → code changes
  │
  ├─► Step 6: validate (Auto)
  │   └─► Validation commands from config
  │
  ├─► Step 7: checklist (SDD - Speckit)
  │   └─► /speckit.checklist → checklist.md
  │
  ├─► Step 8-10: git operations
  │
  └─► Step 11: review (Human)
```

### Parallel Features

```bash
# Terminal 1
$ atlas start "add user authentication" --workspace auth
Creating workspace 'auth'...
  → Creating worktree at ~/projects/myrepo-auth
  → Creating branch: feat/auth
  → Starting task chain...

# Terminal 2
$ atlas start "fix payment timeout" --workspace payment
Creating workspace 'payment'...
  → Creating worktree at ~/projects/myrepo-payment
  → Creating branch: fix/payment
  → Starting task chain...

# Check status
$ atlas status
┌─────────────────────────────────────────────────────────────┐
│  ATLAS Status                                               │
├─────────────────────────────────────────────────────────────┤
│  WORKSPACE   BRANCH         STATUS              STEP        │
│  auth        feat/auth      running             3/7         │
│  payment     fix/payment    awaiting_approval   6/7         │
└─────────────────────────────────────────────────────────────┘

# Approve and cleanup
$ atlas approve payment-task-xyz
$ atlas workspace destroy payment
```

---

## 7. What's Deferred

| Feature | Why Deferred | Revisit When |
|---------|--------------|--------------|
| **`refactor` template** | Core templates must prove value first | Bugfix/feature patterns established |
| **`test-coverage` template** | Analyze gaps, implement tests | Test workflow patterns established |
| **`pr-update` template** | Update existing PR descriptions | PR workflow refinement needed |
| **Learn/Rules Update** | Core workflow must be solid first | v1 is stable and useful |
| **Research Agent** | Manual monitoring is fine for now | Tracking 5+ frameworks |
| **Multi-Repo** | Enterprise complexity | Users demonstrate concrete need |
| **Trust Levels** | Need rejection data first | 100+ task completions |
| **Cloud Execution** | Local first | Need scale-out |
| **Other Languages** | Go-first simplifies validation | Go version is stable |
| **ADK/Genkit** | Direct SDK is simpler for v1 | Multi-agent workflows needed |
| **Additional PM Tools** | GitHub covers target users | Enterprise customers require |
| **Token/Cost Tracking** | Timeout is sufficient guard | Budget concerns arise |
| **Advanced Resume** | Basic checkpoint resume sufficient for v1 | Complex failure scenarios arise |

---

## 8. Failure Modes

| Failure | Symptom | Mitigation |
|---------|---------|------------|
| **Setup too hard** | Users abandon before first task | One command: `atlas init` with wizard |
| **Unclear state** | "What is it doing?" confusion | All state in readable JSON/YAML/MD files |
| **Bad output quality** | Rejected PRs | Validation gates + human approval |
| **Too slow** | Context switching | Local execution, no containers |
| **Breaks workflow** | Merge conflicts | Additive only—works with existing Git |
| **AI mistakes** | Incorrect code | Human approval required |
| **Worktree conflicts** | Branch already exists | Clear error message, suggest cleanup |
| **SDD framework issues** | Speckit failures | Graceful fallback, show framework output |
| **GitHub failures** | PR creation fails, push rejected | Auto-retry (3x), then `gh_failed` state for human intervention |

---

## 9. Known Obstacles & Risks

### 9.1 Implementation Obstacles

| Obstacle | Impact | Notes |
|----------|--------|-------|
| **Git credential complexity** | High | SSH vs HTTPS, PATs, 2FA. Budget time for edge cases. |
| **SDD framework installation** | Medium | Need uv for Speckit. Auto-install adds complexity. |
| **Worktree branch conflicts** | Medium | Handle existing branches gracefully. |
| **Large repo context** | Medium | File selection heuristics need iteration. |

### 9.2 Accepted Risks (v1)

| Risk | Mitigation | Revisit When |
|------|------------|--------------|
| Worktree left behind | User manually cleans up | Users complain repeatedly |
| No output sanitization | Human reviews all PRs | Security incident |
| API cost runaway | Timeout per task (30m) | Budget exceeded |
| SDD framework breaking changes | Pin versions, test updates | Framework update breaks ATLAS |

### 9.3 Security Acknowledgment

The execution environment has access to:
- **Model API keys**: Can incur costs, potential for prompt injection
- **Git push credentials**: Can push code to any branch (except protected)
- **Local filesystem**: Full access to worktree directory

**v1 security stance:**
Human approval is the security boundary. All code is reviewed before merge.

**Recommendations:**
- Use branch protection rules
- Require PR reviews before merge
- Set up API budget alerts
- Consider GitHub App tokens with minimal scopes

---

## Appendix A: File Structure

**ATLAS home (~/.atlas/):**
```
~/.atlas/
├── config.yaml                            # Global configuration
├── logs/
│   └── atlas.log                          # Host CLI operations
├── backups/
│   └── speckit-<timestamp>/               # Speckit upgrade backups
└── workspaces/
    └── auth/
        ├── workspace.json                 # Workspace metadata + task history
        └── tasks/
            └── task-20251226-143022/      # Timestamp-based task ID
                ├── task.json              # Task state & step history
                ├── task.log               # Full execution log (JSON-lines)
                └── artifacts/
                    ├── analyze.md
                    ├── spec.md            # (Speckit templates)
                    ├── plan.md            # (Speckit templates)
                    ├── tasks.md           # (Speckit templates)
                    ├── checklist.md       # (Speckit templates)
                    ├── validation.json
                    ├── validation.1.json  # First attempt (on retry)
                    ├── validation.2.json  # Second attempt (on retry)
                    └── pr-description.md
```

**Artifact versioning:** When a step retries (e.g., validation fails, AI tries again), previous artifacts are preserved with numeric suffixes (`validation.1.json`, `validation.2.json`). The current/latest is always the base name (`validation.json`).

**Browsing use cases:**
```bash
# All PR descriptions ever
cat ~/.atlas/workspaces/*/tasks/*/artifacts/pr-description.md

# All artifacts for a specific task
ls ~/.atlas/workspaces/auth/tasks/task-20251226-143022/artifacts/

# Workspace task history
jq '.tasks' ~/.atlas/workspaces/auth/workspace.json

# Latest task in a workspace (sorts chronologically)
ls ~/.atlas/workspaces/auth/tasks/ | tail -1
```

**Git worktree (stays clean):**
```
~/projects/myrepo-auth/           # Git worktree
├── .speckit/                     # Speckit config (if using SDD)
└── ... (your code)
```

ATLAS state is completely separated from your repository. The worktree contains only your code and optional SDD configuration.

---

## Appendix B: Task Output Schema

```json
{
  "$schema": "atlas-task-output-v1",
  "task_id": "task-20251226-100000",
  "status": "completed",
  "workspace": "fix-null-pointer",
  "output": {
    "summary": "Fixed null pointer in parseConfig by adding nil check",
    "files_modified": [
      "pkg/config/parser.go",
      "pkg/config/parser_test.go"
    ],
    "artifacts": [
      "~/.atlas/workspaces/fix-null-pointer/tasks/task-20251226-100000/artifacts/analyze.md",
      "~/.atlas/workspaces/fix-null-pointer/tasks/task-20251226-100000/artifacts/pr-description.md"
    ],
    "validation_results": {
      "lint": {"passed": true, "duration_ms": 3200},
      "test": {"passed": true, "duration_ms": 8500}
    },
    "git": {
      "branch": "fix/null-pointer-parseconfig",
      "commits": ["abc1234"],
      "pr_url": "https://github.com/user/repo/pull/42"
    }
  },
  "metrics": {
    "duration_ms": 45000,
    "tokens_in": 15000,
    "tokens_out": 2500,
    "retries": 0
  }
}
```

---

## Appendix C: Glossary

| Term | Definition |
|------|------------|
| **Workspace** | ATLAS's logical unit of work. Contains task state, artifacts, and logs. Stored in `~/.atlas/workspaces/<name>/`. |
| **Worktree** | A Git working directory. ATLAS creates worktrees via `git worktree add` for parallel feature development. Located at `~/projects/<repo>-<name>/`. |
| **Task** | A single execution of a template. Workspaces can contain multiple tasks. |
| **Template** | A predefined workflow (bugfix, feature, etc.) compiled into ATLAS as Go code. |
| **SDD** | Specification-Driven Development. A methodology where features are specified before implementation. |
| **Speckit** | An SDD framework providing slash commands for specification, planning, and implementation. |

---

*This document describes ATLAS v1.1. See [templates.md](templates.md) for comprehensive template documentation.*
