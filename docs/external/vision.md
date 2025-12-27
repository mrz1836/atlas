# ATLAS: AI Task Lifecycle Automation System

- **Status:** Vision Document
- **Version:** (MVP)

---

## 1. Executive Summary

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

**What ATLAS does:**
- Accepts a task description in natural language
- Coordinates AI agents to analyze, implement, and validate code
- Integrates with [Speckit](https://github.com/github/spec-kit) for specification-driven development
- Produces Git branches, commits, and pull requests

**Built with:**
- Pure Go 1.24+ with minimal dependencies
- CLI orchestration via Claude Code (`claude` command)
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
- Integration with [Speckit](https://github.com/github/spec-kit) for SDD workflows

---

## 2. Project Context

### Who This Is For

This project is built by a senior Go engineer who works across:
- **CLI tools** — Command-line applications like ATLAS itself
- **Backend services** — Serverless functions, microservices, REST/GraphQL APIs
- **Libraries** — Reusable Go modules published for community use

### Development Standards

All code follows the conventions documented in [`.github/AGENTS.md`](../../.github/AGENTS.md), which defines:
- Go idioms and patterns
- Testing standards
- Commit and PR conventions
- CI/CD workflows

These standards apply equally to human contributors and AI agents.

### Project Status: MVP

This is an MVP — not production software. Breaking changes are expected and welcome. The goal is rapid iteration toward a tool that actually works, not premature stability.

**What this means:**
- Interfaces may change without deprecation periods
- Features may be added, removed, or completely reimagined
- Feedback drives direction more than roadmaps

### The "Super Powers" Goal

ATLAS exists to multiply developer output while maintaining quality. The ideal workflow:

1. **Stay in planning mode** — Focus on specifications, architecture, and design decisions
2. **Parallel execution** — Run multiple implementations simultaneously across workspaces
3. **Reduced context switching** — ATLAS handles the tedium (lint, test, commit, PR) so you can think strategically
4. **Human authority** — Every decision point pauses for approval; nothing merges unsupervised

The result: more work shipped, more accurately, with less cognitive drain.

### External Resources

| Tool | Purpose | Links |
|------|---------|-------|
| Claude Code | AI execution engine | [GitHub](https://github.com/anthropics/claude-code) · [Docs](https://docs.anthropic.com/en/docs/claude-code/overview) |
| Speckit | SDD framework | [GitHub](https://github.com/github/spec-kit) |
| mage-x | Build automation | [GitHub](https://github.com/mrz1836/mage-x) |
| go-pre-commit | Git hooks | [GitHub](https://github.com/mrz1836/go-pre-commit) |

---

## 3. Core Principles

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

## 4. Implementation Stack

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
| GitHub API | `google/go-github` | Official GitHub v3/v4 client |

**External tools (not Go dependencies):**
- `claude` CLI — AI execution via Claude Code
- `gh` CLI — GitHub PR/issue operations
- `git` — Version control operations

### AI Architecture

```
┌─────────────────────────────────────────────────┐
│                  ATLAS Core                     │
├─────────────────────────────────────────────────┤
│  AIRunner Interface                             │
│  └─ ClaudeCodeRunner (claude CLI)               │
│      └─ [Claude Code](https://docs.anthropic.com/en/docs/claude-code) handles file ops, search    │
├─────────────────────────────────────────────────┤
│  SDD Framework Integration                      │
│  └─ [Speckit](https://github.com/github/spec-kit) (.speckit/ repo + CLI)              │
└─────────────────────────────────────────────────┘
```

**Why AI CLI integration:**
- Leverage mature AI coding tools (Claude Code handles file operations, context, search)
- ATLAS stays focused on orchestration, not AI agent internals
- Easy debugging—inspect CLI invocations and outputs
- Future flexibility—swap CLI tools without core changes

**Interface extensibility:** The `AIRunner` interface allows adding other AI CLI tools (Cursor, Aider, etc.) without core changes. Deferred until needed.

### What We Don't Use

- No database (all state is file-based: JSON, YAML, Markdown)
- No web framework (no HTTP server in v1)
- No dependency injection framework (explicit wiring)
- No LangChain/ADK/Genkit (AI CLI tools handle the complexity)

---

## 5. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              ATLAS CLI                                  │
│                                                                         │
│  atlas init | start | status | approve | reject | resume |  workspace   │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────────────────────┐  ┌─────────────────────────┐           │
│  │  Worktree: auth-feature     │  │  Worktree: payment-fix  │           │
│  │  ../myrepo-auth/            │  │  ../myrepo-payment/     │           │
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
2. ATLAS creates Git worktree as sibling to repo (configurable)
3. Task JSON created in `~/.atlas/workspaces/bugfix-ws/tasks/`
4. Task Engine executes template steps (AI, validation, git, human)
5. Claude invoked via CLI (`claude -p --output-format json`) in worktree directory
6. Git operations happen in worktree directory
7. Human approves/rejects at checkpoints

---

## 6. Components

### 6.1 CLI Interface

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

**Module path:** `github.com/mrz1836/atlas`

ATLAS is installed globally via Go's package manager:

```bash
go install github.com/mrz1836/atlas@latest
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
| [Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-code) | AI execution | 2.0.76+ | No (detect only) |
| [mage-x](https://github.com/mrz1836/mage-x) (`magex` command) | Build automation | v0.3.0 | Yes (install/upgrade) |
| [go-pre-commit](https://github.com/mrz1836/go-pre-commit) | Pre-commit hooks | v0.1.0 | Yes (install/upgrade) |
| [Speckit](https://github.com/github/spec-kit) | SDD framework | 1.0.0 | Yes (install/upgrade) |

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
  claude          ✓ installed 2.1.0       2.0.76+     —
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

Flow: backup constitution → `uv tool upgrade specify-cli` → restore constitution from backup.

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

### 6.2 Task Engine

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
                       │ retry         │ step complete    │ retry
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
                └───────┬─────────┘      └─────────────────┘
                        │                       │
          ┌─────────────┼─────────────┐         │ abandon
          │ approve     │ retry       │ reject  ▼
          ▼             │             ▼    ┌───────────┐
   ┌───────────┐        │      ┌──────────┐│ abandoned │
   │ completed │        └──────┤ rejected │└───────────┘
   └───────────┘               └──────────┘
```

**State transitions:**
| From | To | Trigger |
|------|-----|---------|
| `pending` | `running` | Task starts |
| `running` | `validating` | Step produces output |
| `validating` | `awaiting_approval` | Validation passes |
| `validating` | `validation_failed` | Validation fails |
| `validation_failed` | `running` | Human chooses retry |
| `validation_failed` | `abandoned` | Human chooses abandon |
| `awaiting_approval` | `completed` | Human approves |
| `awaiting_approval` | `running` | Human chooses retry with feedback |
| `awaiting_approval` | `rejected` | Human rejects (done) |
| `running` | `gh_failed` | GitHub operation fails after retries |
| `gh_failed` | `running` | Human resolves and retries |
| `gh_failed` | `abandoned` | Human abandons |
| `running` | `ci_failed` | CI workflow fails |
| `running` | `ci_timeout` | CI polling timeout |
| `ci_failed` | `running` | Human retries |
| `ci_failed` | `abandoned` | Human abandons |
| `ci_timeout` | `running` | Human retries or waits |
| `ci_timeout` | `abandoned` | Human abandons |

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

**CI waiting:**

After creating the PR, ATLAS waits for configured GitHub Actions workflows to complete before requesting human review. This ensures CI passes before the reviewer's time is spent.

```yaml
# .atlas/config.yaml
ci:
  workflows:
    - name: "CI"
      required: true
    - name: "Lint"
      required: true
  poll_interval: 2m
  timeout: 30m
```

ATLAS polls the GitHub Actions API for the PR's check runs every 2 minutes (configurable). If a required workflow fails or timeout is exceeded, the task pauses for human decision:

```
? CI workflow "CI" failed. What would you like to do?
  ❯ View workflow logs — Open GitHub Actions in browser
    Retry from implement — AI tries to fix based on CI output
    Fix manually and resume — You fix, then 'atlas resume'
    Abandon task — End task, keep branch for manual work
```

**Step types:**
| Type | Executor | Auto-proceeds? |
|------|----------|----------------|
| ai | AI Runner (Claude Code CLI) | Yes — proceeds to next step |
| validation | Configured commands | Yes if passing; pauses on failure |
| git | Git CLI operations | Yes (configurable via `auto_proceed_git: false`) |
| ci | GitHub Actions API | Yes if passing; pauses on failure/timeout |
| human | Interactive prompt | No — always waits for human decision |
| sdd | Speckit via AI Runner | Yes — proceeds to next step |

**Human steps:** All human steps are "pause for decision" moments. The UI adapts based on context:
- After AI implementation → shows diff, offers approve/reject/retry
- After specification → shows spec, offers approve/reject/retry
- After validation failure → shows errors, offers retry options

Templates define **when** human steps occur (e.g., `review_spec` after `/speckit.specify`). ATLAS determines **what** to show based on the preceding step's artifacts.

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

### 6.3 AI Runner Layer

ATLAS orchestrates **when** to invoke AI; the AI CLI handles **how** (file reading, editing, search, context management).

```go
type AIRunner interface {
    Run(ctx context.Context, req *AIRequest) (*AIResult, error)
}

type AIRequest struct {
    Prompt    string            // Task description or slash command
    WorkDir   string            // Worktree path
    Mode      string            // "plan" or "implement"
    Context   []string          // Previous step artifact paths
    Model     string            // Model to use (e.g., "claude-sonnet-4-5-20250916")
    Flags     map[string]string // Additional CLI flags
    Timeout   time.Duration     // Per-step timeout
}

type AIResult struct {
    Output       string   // Captured stdout/response
    FilesChanged []string // Files modified (if any)
    SessionID    string   // Session ID for logging
    ExitCode     int
}
```

**Modes:**
| Mode | Purpose | Edits allowed |
|------|---------|---------------|
| `plan` | Explore codebase, propose approach | No |
| `implement` | Execute changes | Yes |

**Implementation (v1):**
- `ClaudeCodeRunner` — Wraps the `claude` CLI

ATLAS invokes `claude -p --output-format json --model <model>` in the worktree directory. Claude Code handles file operations; ATLAS parses the JSON response for logging and artifact extraction.

```bash
# Example invocation
cd /path/to/worktree
claude -p --output-format json --model sonnet --max-turns 10 "<Prompt>"
# Mode "plan" adds --permission-mode plan to restrict edits
```

**Claude CLI response schema** (confirmed via testing):

```json
{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "result": "<output text>",
  "session_id": "b4070e9d-da85-4524-8c13-fa3c78712185",
  "duration_ms": 2551,
  "num_turns": 1,
  "total_cost_usd": 0.04,
  "usage": {
    "input_tokens": 3,
    "output_tokens": 4,
    "cache_read_input_tokens": 12787,
    "cache_creation_input_tokens": 5244
  }
}
```

**Key response fields:**
| Field | Purpose |
|-------|---------|
| `type` | Always `"result"` for completed invocations |
| `subtype` | `"success"` or `"error"` |
| `is_error` | Boolean for error detection |
| `result` | The actual output text |
| `session_id` | UUID for logging and debugging |
| `duration_ms` | Execution time |
| `num_turns` | Agentic turns used |
| `total_cost_usd` | API cost for the invocation |

**Useful CLI flags:**
| Flag | Purpose |
|------|---------|
| `--permission-mode plan` | Restrict to read-only analysis (no file edits) |
| `--max-budget-usd <amount>` | Cap spending per invocation |
| `--append-system-prompt <prompt>` | Inject context without replacing system prompt |
| `--tools <list>` | Restrict available tools (e.g., `"Read,Edit,Write,Bash,Glob,Grep"`) |
| `--model <alias>` | Model selection: `sonnet`, `opus`, or full model ID |

**Execution model:**

Each AI step is a single, atomic CLI invocation. No multi-turn conversations within a step.

| Scenario | Behavior |
|----------|----------|
| Step starts | Fresh invocation with prompt + context artifacts |
| Step completes | Parse JSON response, extract session_id for logging |
| Step retries | Fresh invocation with error/feedback context injected into prompt |

**Why fresh invocations on retry:** When AI produces bad output, continuing the same session carries forward the flawed reasoning. Starting fresh with error context or rejection feedback gives AI a clean slate.

**Step logs:**
```
~/.atlas/workspaces/<ws>/tasks/<task-id>/artifacts/
  implement.log        # Full CLI output captured
```

**Future runners:** Interface supports other AI CLI tools (Cursor, Aider, etc.) by implementing the same interface with tool-specific flag/session mappings.

**Configuration:**

The default model is configurable—not hardcoded. Users set their preference in config:

```yaml
# ~/.atlas/config.yaml
ai:
  runner: claude-code
  default_model: sonnet           # Alias: sonnet, opus, or full model ID
  timeout: 30m
  max_turns: 10                   # Max agentic turns per step
  flags:                          # Default flags passed to CLI
    tools: "Read,Edit,Write,Bash,Glob,Grep"
```

Model aliases: `sonnet` → `claude-sonnet-4-5-20250916`, `opus` → `claude-opus-4-5-20251101`

**Per-template model override:**
```yaml
templates:
  bugfix:
    model: claude-sonnet-4-5-20250916  # Fast for simple fixes
  feature:
    model: claude-opus-4-5-20251101    # Thorough for complex features
```

**Safeguards:**
- `timeout: 30m` — Maximum time for any single AI step
- `max_turns: 10` — Maximum agentic turns per CLI invocation
- Validation failures always pause for human decision (no auto-retry loops)

**Step artifacts:**

AI steps produce artifacts stored in `~/.atlas/workspaces/<ws>/tasks/<task-id>/artifacts/`:
- `analyze.md` — Analysis output
- `implement.log` — Implementation session log
- `spec.md`, `plan.md`, etc. — SDD outputs

Artifacts from previous steps are passed as context to subsequent steps.

### 6.4 SDD Framework Integration

ATLAS integrates with SDD frameworks as external tools, not abstractions. The frameworks do the specification work; ATLAS orchestrates when to invoke them.

#### Speckit Integration

**What is Speckit:** A spec-driven development toolkit providing structured specification, planning, and implementation workflows.

**Two-part installation:**
1. **CLI tool** (global): `uv tool install specify-cli --from git+https://github.com/github/spec-kit.git` — Provides the `specify` command
2. **Project repo** (per-project): Installs Speckit's prompt patterns into `.speckit/` via `specify init`

ATLAS manages both installations. The CLI provides upgrade/management commands; the project repo provides slash command definitions that Claude can access.

**Slash commands:**

| Command | Purpose |
|---------|---------|
| `/speckit.constitution` | Create project governing principles |
| `/speckit.specify` | Define requirements and user stories |
| `/speckit.plan` | Create technical implementation strategy |
| `/speckit.tasks` | Generate actionable task lists |
| `/speckit.implement` | Execute tasks to build features |
| `/speckit.checklist` | Generate quality validation checklists |

**Execution model:** ATLAS invokes the AI Runner with a prompt containing the slash command. Claude Code (with access to `.speckit/` in the worktree) executes the command and produces structured output. ATLAS captures the output as an artifact.

#### When to Use Speckit

| Use Case | Recommended | Rationale |
|----------|-------------|-----------|
| Bug fixes | No SDD | Overkill; just analyze + fix |
| Small features | Speckit | Lightweight, focused specs |
| Large features | Speckit | Full specification + planning |

### 6.5 Workspaces and Git Worktrees

ATLAS uses two related concepts (see Glossary for definitions):
- **Workspace**: ATLAS state (`~/.atlas/workspaces/<name>/`) — tasks, artifacts, logs
- **Worktree**: Git working directory (sibling to repo by default) — your code

This separation enables working on multiple features simultaneously without interference.

**Worktree location** (configurable):
```yaml
# ~/.atlas/config.yaml
worktree:
  base_dir: ""  # Empty = sibling to repo (e.g., ../myrepo-auth)
  # Or explicit: ~/projects/atlas-worktrees/
```

Default: If your repo is at `/Users/me/projects/myrepo`, worktrees are created at `/Users/me/projects/myrepo-<workspace>`.

**Why Git worktrees:**
- **Native Git feature** — No additional dependencies
- **Parallel branches** — Each worktree is a separate working directory
- **Clean isolation** — No file conflicts between features
- **Lightweight** — Shares Git objects with main repo
- **Easy cleanup** — `git worktree remove` handles everything

**Worktree lifecycle:**
```bash
# ATLAS internally runs:
git worktree add ../myrepo-auth feat/auth   # Create (sibling to repo)
git worktree list                           # List
git worktree remove ../myrepo-auth          # Cleanup
```

**State separation:**
- **ATLAS state** lives in `~/.atlas/workspaces/<name>/` (tasks, artifacts, logs)
- **Git worktree** stays clean (just your code + .speckit if using SDD)

This separation means:
- Task state survives accidental worktree deletion
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
- **Worktree path conflict**: If the target worktree path already exists, ATLAS appends a numeric suffix (`-2`, `-3`, etc.)
- **Branch conflict**: If branch `<type>/<name>` already exists, ATLAS appends a timestamp suffix (e.g., `fix/auth-20251226`)
- Both cases display a clear warning message explaining the renamed path/branch

### 6.6 Validation

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

### 6.7 Git Operations

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
Adapted from the `/sc` (smart commit) command. The `git:smart_commit` action:
1. Groups modified files by logical unit (package/directory)
2. Detects and flags uncommittable content (garbage, secrets, debug code)
3. Generates meaningful commit messages via AI
4. Creates atomic commits per logical change

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

### 6.8 Project Rules Update (Post-MVP)

ATLAS can learn from completed work by suggesting updates to project rules files (AGENTS.md, constitution.md, etc.).

**Deferred to post-MVP.** Core concept: after task approval, optionally analyze what was learned and propose updates to project guidance files.

Key ideas preserved for future implementation:
- Explicit, human-controlled process (not automatic)
- Rules stay version-controlled in the project
- AI proposes minimal, targeted updates
- Human reviews diff before applying

### 6.9 Observability

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

### 6.10 User Experience

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

## 7. Workflow Examples

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
  ├─► Step 7: ci_wait (Auto)
  │   └─► Polls GitHub Actions on PR until CI passes ✓
  │
  └─► Step 8: review (Human)
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
  ├─► Step 8: git_commit (Auto)
  │   └─► Creates branch, commits with trailers
  │
  ├─► Step 9: git_push (Auto)
  │   └─► Pushes to remote
  │
  ├─► Step 10: git_pr (Auto)
  │   └─► gh pr create
  │
  ├─► Step 11: ci_wait (Auto)
  │   └─► Polls GitHub Actions on PR until CI passes ✓
  │
  └─► Step 12: review (Human)
```

### Parallel Features

```bash
# Terminal 1
$ atlas start "add user authentication" --workspace auth
Creating workspace 'auth'...
  → Creating worktree at ../myrepo-auth
  → Creating branch: feat/auth
  → Starting task chain...

# Terminal 2
$ atlas start "fix payment timeout" --workspace payment
Creating workspace 'payment'...
  → Creating worktree at ../myrepo-payment
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

## 8. What's Deferred

### Post-MVP Features

| Feature | Why Deferred | Revisit When |
|---------|--------------|--------------|
| **`atlas resume`** | Simplifies MVP; if task dies, re-run | Core workflow proven stable |
| **`refactor` template** | Core templates must prove value first | Bugfix/feature patterns established |
| **`test-coverage` template** | Analyze gaps, implement tests | Test workflow patterns established |
| **`pr-update` template** | Update existing PR descriptions | PR workflow refinement needed |
| **Learn/Rules Update** | Core workflow must be solid first | v1 is stable and useful |
| **[Research Agent](../internal/research-agent.md)** | Manual monitoring is fine for now | Tracking 5+ frameworks |
| **Multi-Repo** | Enterprise complexity | Users demonstrate concrete need |
| **Trust Levels** | Need rejection data first | 100+ task completions |
| **Cloud Execution** | Local first | Need scale-out |
| **Other Languages** | Go-first simplifies validation | Go version is stable |
| **ADK/Genkit** | Direct SDK is simpler for v1 | Multi-agent workflows needed |
| **Additional PM Tools** | GitHub covers target users | Enterprise customers require |
| **Token/Cost Tracking** | Timeout is sufficient guard | Budget concerns arise |
| **Advanced Resume** | Basic checkpoint resume sufficient for v1 | Complex failure scenarios arise |

---

## 9. Failure Modes

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

## 10. Known Obstacles & Risks

### 10.1 Implementation Obstacles

| Obstacle | Impact | Notes |
|----------|--------|-------|
| **Git credential complexity** | High | SSH vs HTTPS, PATs, 2FA. Budget time for edge cases. |
| **SDD framework installation** | Medium | Need uv for Speckit. Auto-install adds complexity. |
| **Worktree branch conflicts** | Medium | Handle existing branches gracefully. |
| **Large repo context** | Medium | File selection heuristics need iteration. |

### 10.2 Accepted Risks (v1)

| Risk | Mitigation | Revisit When |
|------|------------|--------------|
| Worktree left behind | User manually cleans up | Users complain repeatedly |
| No output sanitization | Human reviews all PRs | Security incident |
| API cost runaway | Timeout per task (30m) | Budget exceeded |
| SDD framework breaking changes | Pin versions, test updates | Framework update breaks ATLAS |

### 10.3 Security Acknowledgment

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
../myrepo-auth/                   # Git worktree (sibling to original repo)
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
| **Worktree** | A Git working directory. ATLAS creates worktrees via `git worktree add` for parallel feature development. By default, created as siblings to the original repo. |
| **Task** | A single execution of a template. Workspaces can contain multiple tasks. |
| **Template** | A predefined workflow (bugfix, feature, etc.) compiled into ATLAS as Go code. |
| **SDD** | Specification-Driven Development. A methodology where features are specified before implementation. |
| **Speckit** | An SDD framework providing slash commands for specification, planning, and implementation. |

---

*This document describes ATLAS (MVP). See [templates.md](templates.md) for comprehensive template documentation.*
