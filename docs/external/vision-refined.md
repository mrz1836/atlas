# ATLAS: AI-Assisted Task Automation for Go Projects

- **Version:** 1.1.0-DRAFT
- **Tag:** v1.1-refined
- **Status:** Vision Document

---

## 1. Executive Summary

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

**What ATLAS does:**
- Accepts a task description in natural language
- Coordinates AI agents to analyze, implement, and validate code
- Integrates with SDD frameworks (Speckit, BMAD) for specification-driven development
- Produces Git branches, commits, and pull requests
- Learns from accepted and rejected work to improve over time

**Built with:**
- Pure Go 1.24+ with minimal dependencies
- Direct integration with Claude and Gemini APIs (anthropic-sdk-go, google-genai)
- Charm libraries for beautiful terminal UX (lipgloss, huh, bubbles)
- Git worktrees for parallel workspace isolation

**What ATLAS is not:**
- Not a "virtual employee"—it's a workflow automation tool that requires human oversight
- Not a universal PM integration—GitHub only in v1
- Not language-agnostic—Go projects only in v1
- Not magic—AI makes mistakes, validation catches some, humans catch the rest

**Who it's for:**
- Go developers who want to accelerate routine development tasks
- Teams that already use GitHub Issues/PRs for workflow
- Developers comfortable with CLI tools

**Explicit scope (v1):**
- Single-repository Go projects
- GitHub as the sole integration point
- Local execution with Git worktrees for parallel work
- Claude (primary) + Gemini (fallback) as AI backends
- Integration with Speckit and BMAD for SDD workflows

---

## 2. Core Principles

### Git is the Backbone

Git is not just version control—it's the audit trail, delivery mechanism, and source of truth. Every ATLAS action produces Git artifacts: branches, commits with machine-parseable trailers, and pull requests. If it's not in Git, it didn't happen.

### Text is Truth

All state is stored as human-readable text files. JSON for structured data, Markdown for prose, YAML for configuration. No databases, no binary formats. You can always `cat` your way to understanding what ATLAS did.

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
| Gemini API | `google/generative-ai-go` | Official Gemini SDK |
| GitHub API | `google/go-github` | Official GitHub v3/v4 client |

### AI Architecture

```
┌─────────────────────────────────────────────────┐
│                  ATLAS Core                     │
├─────────────────────────────────────────────────┤
│  ModelClient Interface                          │
│  ├─ ClaudeClient (anthropic-sdk-go)             │
│  │   └─ Primary provider, best code quality     │
│  └─ GeminiClient (google-genai)                 │
│      └─ Fallback when Claude unavailable        │
├─────────────────────────────────────────────────┤
│  SDD Framework Integration                      │
│  ├─ Speckit (uv tool, /speckit.* commands)      │
│  └─ BMAD (npm, *agent commands)                 │
└─────────────────────────────────────────────────┘
```

**Why direct SDK integration:**
- Full control over request/response handling
- No framework abstraction overhead
- Easy debugging—read your code, not framework internals
- Type safety from official SDKs

**Future consideration:** ADK Go and Genkit for multi-agent orchestration when workflows become more complex.

### What We Don't Use

- No database (all state is file-based: JSON, YAML, Markdown)
- No web framework (no HTTP server in v1)
- No dependency injection framework (explicit wiring)
- No LangChain/ADK/Genkit (direct API integration is simpler for v1)

---

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                       ATLAS CLI                                 │
│                                                                 │
│  atlas init | start | status | approve | reject | workspace     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────┐  ┌─────────────────────────┐   │
│  │  Worktree: auth-feature     │  │  Worktree: payment-fix  │   │
│  │  ~/projects/repo-auth/      │  │  ~/projects/repo-pay/   │   │
│  │  ┌───────────────────────┐  │  │  ┌───────────────────┐  │   │
│  │  │ Branch: feat/auth     │  │  │  │ Branch: fix/pay   │  │   │
│  │  │ .atlas/tasks/*.json   │  │  │  │ .atlas/tasks/...  │  │   │
│  │  │ .atlas/artifacts/     │  │  │  │ .atlas/artifacts/ │  │   │
│  │  └───────────────────────┘  │  │  └───────────────────┘  │   │
│  └─────────────────────────────┘  └─────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    Shared (Host)                        │    │
│  │  Memory: ~/.atlas/memory/                               │    │
│  │  Templates: ~/.atlas/templates/*.yaml                   │    │
│  │  Config: ~/.atlas/config.yaml                           │    │
│  │  Logs: ~/.atlas/logs/                                   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Data flow:**
1. User runs `atlas start "fix the bug" --workspace bugfix-ws`
2. ATLAS creates Git worktree at `~/projects/repo-bugfix-ws/`
3. Task JSON created in worktree's `.atlas/tasks/`
4. Task Engine executes template steps (AI, validation, git, human)
5. Claude/Gemini invoked via SDK for AI steps
6. Git operations happen in worktree directory
7. Human approves/rejects; outcome stored in shared memory

---

## 5. Components

### 5.1 CLI Interface

Six commands cover 95% of usage:

```bash
atlas init                              # Initialize ATLAS configuration
atlas start "description" [--workspace] # Start task in workspace
atlas status                            # Show all workspaces and tasks
atlas approve <task-id>                 # Approve pending work
atlas reject <task-id> "reason"         # Reject with feedback
atlas workspace <list|stop|destroy|logs> # Manage workspaces
```

**Workspace-aware behavior:**
- `atlas start "desc"` — Auto-generates workspace name from description
- `atlas start "desc" --workspace feat-x` — Uses explicit workspace name
- `atlas status` — Shows all workspaces and their task states
- `atlas workspace logs <name> [--follow]` — Stream task logs from workspace

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

ATLAS manages all required tools automatically. On first run (and periodically thereafter), it detects, installs, and upgrades dependencies.

**Managed dependencies:**

| Tool | Purpose | Pinned Version | Auto-Install |
|------|---------|----------------|--------------|
| Go | Runtime | 1.24+ | Detected only |
| Git | Version control | 2.20+ | Detected only |
| GitHub CLI (`gh`) | PR operations | Latest | ✓ |
| mage-x | Build automation | v0.3.0 | ✓ |
| go-pre-commit | Pre-commit hooks | v0.1.0 | ✓ |
| uv | Speckit runtime | 0.5.x | ✓ |
| npm | BMAD runtime | 10.x | Detected only |
| Speckit | SDD framework | 1.0.0 | ✓ |
| BMAD | SDD framework | alpha | ✓ |

**Detection flow:**
```
atlas init
  │
  ├─► Scan: Detect installed tools and versions
  │   └─► Show status table (installed ✓, missing ✗, outdated ⚠)
  │
  ├─► Prompt: "Install missing dependencies? [Y/n]"
  │   └─► One-command install for all missing tools
  │
  └─► Configure: AI providers, GitHub auth, templates
```

**What you'll see:**
```
Checking dependencies...

  TOOL            STATUS      VERSION     REQUIRED
  Go              ✓ installed 1.24.1      1.24+
  Git             ✓ installed 2.43.0      2.20+
  gh              ✗ missing   —           latest
  mage-x          ⚠ outdated  0.2.1       0.3.0
  go-pre-commit   ✓ installed 0.1.0       0.1.0
  uv              ✓ installed 0.5.12      0.5.x
  Speckit         ✗ missing   —           1.0.0

Install missing/outdated tools? [Y/n] y
  Installing gh...        ✓
  Upgrading mage-x...     ✓
  Installing Speckit...   ✓

All dependencies ready.
```

#### Self-Upgrade

ATLAS can upgrade itself and all managed dependencies:

```bash
atlas upgrade              # Upgrade ATLAS + all tools
atlas upgrade --self       # Upgrade ATLAS only
atlas upgrade --tools      # Upgrade managed tools only
atlas upgrade --check      # Show available updates without installing
```

#### SDD Framework Upgrades

SDD frameworks (Speckit, BMAD) require special handling to preserve your customizations.

**Speckit upgrades:**
```bash
atlas upgrade speckit
```

ATLAS handles Speckit upgrades intelligently:
- Preserves your `constitution.md` and custom templates
- Shows diff of what will change before applying
- Backs up existing files to `.atlas/backups/`
- Merges new features without overwriting customizations

```
Upgrading Speckit 0.9.0 → 1.0.0...

  Files to update:
    .speckit/prompts/specify.md    (new version available)
    .speckit/prompts/plan.md       (new version available)

  Files preserved (your customizations):
    .speckit/constitution.md       (keeping yours)
    .speckit/templates/custom.md   (keeping yours)

  Apply upgrade? [Y/n] y
  Backup created: .atlas/backups/speckit-0.9.0-20251226/
  Upgrade complete.
```

**BMAD upgrades:**
```bash
atlas upgrade bmad
```

Same intelligent handling for BMAD configurations.

**First-time setup wizard** (launched by `atlas init`):

The interactive wizard (powered by Charm huh) configures:
- AI provider selection and API credentials
- GitHub authentication
- Default template selection
- SDD framework preferences

Configuration stored in `~/.atlas/config.yaml`.

### 5.2 Task Engine

Tasks are the atomic units of work. State lives in `.atlas/tasks/` as JSON files.

**Task lifecycle:**
```
pending ─► running ─► validating ─┬─► awaiting_approval ─► completed
                                  │         │
                                  │         └─► rejected ─► (feedback stored)
                                  │
                                  └─► validation_failed ─► running (retry loop)
                                              │
                                              └─► failed (max retries exceeded)
```

**State transitions:**
| From | To | Trigger |
|------|-----|---------|
| `pending` | `running` | Task execution starts |
| `running` | `validating` | AI produces output |
| `validating` | `awaiting_approval` | All validations pass |
| `validating` | `validation_failed` | Any validation fails |
| `validation_failed` | `running` | Retry with failure context (retry < max) |
| `validation_failed` | `failed` | Max retries exceeded |
| `awaiting_approval` | `completed` | Human runs `atlas approve` |
| `awaiting_approval` | `rejected` | Human runs `atlas reject` |

**Step types:**
| Type | Executor | Auto-proceeds? |
|------|----------|----------------|
| ai | Claude/Gemini SDK | No (pauses for approval after all AI steps) |
| validation | Configured commands | Yes (if passing) |
| git | Git CLI operations | No (pauses before PR creation) |
| human | Developer action | N/A (waits for human) |
| sdd | Speckit/BMAD CLI | Varies by command |

**Task JSON structure:**
```json
{
  "id": "task-a1b2c3d4",
  "template": "bugfix",
  "status": "pending",
  "workspace": "fix-null-pointer",
  "created_at": "2025-12-26T10:00:00Z",
  "input": {
    "description": "Fix null pointer in parseConfig",
    "files": ["pkg/config/parser.go"]
  },
  "current_step": 0,
  "steps": [
    {"name": "analyze", "status": "completed", "output": "..."},
    {"name": "implement", "status": "running"},
    {"name": "validate", "status": "pending"}
  ],
  "git": {
    "repo": "owner/project",
    "base_branch": "main",
    "work_branch": "fix/null-pointer-parseconfig"
  },
  "retry": {
    "count": 0,
    "max": 3
  }
}
```

**Templates:**
Pre-defined task chains for common workflows. See [templates.md](templates.md) for comprehensive documentation.

Built-in templates:
- `bugfix` — Analyze, implement, validate, commit, PR
- `feature` — Speckit SDD: specify, plan, implement, validate, PR
- `feature-bmad` — BMAD: analysis, PRD, architecture, implement, QA, PR
- `test-coverage` — Analyze gaps, implement tests, validate, PR
- `refactor` — Incremental refactoring with validation between steps

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
    Model     string  // Optional override
}

type CompletionResponse struct {
    Content    string
    TokensIn   int
    TokensOut  int
    StopReason string
}
```

**Implementations:**
- `ClaudeClient` — Uses anthropic-sdk-go, primary provider
- `GeminiClient` — Uses google-genai, fallback provider

**Fallback pattern:**
```go
func (e *Engine) invokeModel(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    resp, err := e.primary.Complete(ctx, req)
    if err != nil && e.fallback != nil {
        log.Warn("Primary model failed, trying fallback", "error", err)
        return e.fallback.Complete(ctx, req)
    }
    return resp, err
}
```

**Configuration:**
```yaml
# ~/.atlas/config.yaml
model:
  primary:
    provider: claude
    model: claude-sonnet-4-5-20250916
    api_key_env: ANTHROPIC_API_KEY
  deep_thinking:
    provider: claude
    model: claude-opus-4-5-20251124
    api_key_env: ANTHROPIC_API_KEY
    thinking:
      enabled: true
      budget_tokens: 32000  # ultrathink
  fallback:
    provider: gemini
    model: gemini-3-pro-preview
    api_key_env: GOOGLE_API_KEY
  fast:
    provider: claude
    model: claude-haiku-4-5-20251015
    api_key_env: ANTHROPIC_API_KEY
  timeout: 300s
  max_tokens: 100000
```

**Model selection per step:**
Templates can specify different models for different steps:
```yaml
steps:
  - name: architecture_review
    type: ai
    model: claude-opus-4-5
    thinking: ultrathink        # Enable 32k+ thinking budget
  - name: analyze
    type: ai
    model: claude-sonnet-4-5    # Best coding model
  - name: commit_message
    type: ai
    model: claude-haiku-4-5     # Fast, cheap for simple tasks
```

### 5.4 SDD Framework Integration

ATLAS integrates with SDD frameworks as external tools, not abstractions. The frameworks do the specification work; ATLAS orchestrates when to invoke them.

#### Speckit Integration

**What is Speckit:** GitHub's spec-driven development toolkit providing structured specification, planning, and implementation workflows.

**Installation:** ATLAS auto-installs via uv:
```bash
uv tool install specify-cli --from git+https://github.com/github/spec-kit.git
```

**Commands available to templates:**
| Command | Purpose |
|---------|---------|
| `/speckit.constitution` | Create project governing principles |
| `/speckit.specify` | Define requirements and user stories |
| `/speckit.plan` | Create technical implementation strategy |
| `/speckit.tasks` | Generate actionable task lists |
| `/speckit.implement` | Execute tasks to build features |
| `/speckit.checklist` | Generate quality validation checklists |

**Template usage:**
```yaml
steps:
  - name: specify
    type: sdd
    framework: speckit
    command: /speckit.specify
    args:
      description: "{{.Description}}"
    output: .atlas/artifacts/spec.md
```

#### BMAD Integration

**What is BMAD:** Breakthrough Method for Agile AI-Driven Development—a multi-agent framework with specialized roles.

**Installation:** ATLAS auto-installs via npm:
```bash
npx bmad-method@alpha install
```

**Agents available to templates:**
| Agent | Role |
|-------|------|
| `*analyst` | Brainstorming and analysis |
| `*pm` | PRD creation |
| `*architect` | Technical architecture |
| `*developer` | Implementation |
| `*qa` | Quality assurance |

**Workflow tracks:**
| Track | Best For | Planning Depth |
|-------|----------|----------------|
| Quick Flow | Bug fixes, small features | Tech spec only |
| Standard | Products, platforms | PRD + Architecture |
| Enterprise | Compliance, scalability | Full governance |

**Template usage:**
```yaml
sdd:
  framework: bmad
  track: standard

steps:
  - name: prd
    type: sdd
    command: "*pm"
    args:
      task: create-prd
    output: .atlas/artifacts/prd.md
```

#### When to Use Which

| Use Case | Recommended | Rationale |
|----------|-------------|-----------|
| Bug fixes | No SDD | Overkill; just analyze + fix |
| Small features | Speckit | Lightweight, focused specs |
| Large features | Speckit or BMAD | Full specification + planning |
| Enterprise/compliance | BMAD Enterprise | Governance, full docs |

### 5.5 Workspace Isolation (Git Worktrees)

Workspaces enable working on multiple features simultaneously without interference.

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

**Workspace manager:**
```go
type Workspace struct {
    Name         string    `json:"name"`
    RepoPath     string    `json:"repo_path"`      // Original repo
    WorktreePath string    `json:"worktree_path"`  // Created worktree
    Branch       string    `json:"branch"`
    TaskID       string    `json:"task_id"`
    CreatedAt    time.Time `json:"created_at"`
    Status       string    `json:"status"`         // active, paused, completed
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
│ Run Validations     │◄──── Auto-retry with AI fixes
│ (configurable)      │      (up to N attempts)
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

ATLAS-Task: task-a1b2c3d4
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
```bash
gh pr create \
  --title "fix: handle nil config options" \
  --body "$(cat .atlas/artifacts/pr-description.md)" \
  --base main \
  --head fix/null-pointer-parseconfig
```

### 5.8 Memory

Memory persists context across tasks and sessions. Stored as Markdown files in `~/.atlas/memory/` (global, shared across all workspaces).

**Structure:**
```
~/.atlas/memory/
├── feedback/               # Rejection reasons, learnings
│   └── 2025-12-26-task-a1b2c3d4.md
├── context/                # Project-specific context
│   └── coding-standards.md
└── decisions/              # Architectural decisions
    └── 2025-12-use-cobra.md
```

**Memory write path:**
The host CLI writes memory based on user actions:

| User Action | Memory Written |
|-------------|----------------|
| `atlas reject <task> "reason"` | Feedback entry with rejection reason |
| `atlas approve <task>` | Success logged (optional) |

**Search:**
Memory is searched via grep. Simple, debuggable, sufficient for hundreds of entries.

```bash
grep -r "error handling" ~/.atlas/memory/
```

### 5.9 Observability

**Log locations:**
```
~/.atlas/logs/
├── atlas.log                    # Host CLI operations
└── workspaces/
    ├── auth/
    │   ├── task-a1b2c3d4.log   # Full task execution log
    │   └── task-e5f6g7h8.log
    └── payment/
        └── ...
```

**Log format (JSON-lines):**
```json
{"ts":"2025-12-26T10:00:00Z","level":"info","event":"task_start","task_id":"task-a1b2c3d4"}
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
cat ~/.atlas/logs/workspaces/auth/task-a1b2c3d4.log

# Tail workspace logs live
atlas workspace logs auth --follow

# Parse logs with jq
cat ~/.atlas/logs/workspaces/*/task-*.log | jq 'select(.event=="model_complete")'
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

Then shows interactive review screen:
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
│  [a]pprove  [r]eject  [d]iff  [l]ogs  [o]pen PR  [q]uit            │
└─────────────────────────────────────────────────────────────────────┘
```

**Key bindings:**
| Key | Action |
|-----|--------|
| `a` | Approve and continue workflow |
| `r` | Reject (prompts for feedback) |
| `d` | Show git diff in pager |
| `l` | Show recent task logs |
| `o` | Open PR in browser (`gh pr view --web`) |
| `q` | Quit without action |

#### Rejection Flow

**`atlas reject [workspace]` — Structured Feedback**

Interactive prompt for rejection reason:
```
? Why are you rejecting this task?
  ❯ Code quality issues
    Missing tests
    Wrong approach
    Incomplete implementation
    Other (provide details)

Additional feedback (optional):
> The error handling doesn't cover the timeout case
```

Feedback is stored in memory (`~/.atlas/memory/feedback/`) for learning and improvement.

#### Dashboard (Post-MVP)

**`atlas dashboard` — Split-Pane Multi-Workspace View**

Full TUI dashboard for monitoring multiple workspaces simultaneously:
```
┌─────────────────────────────────────────────────────────────────────────┐
│  ATLAS Dashboard                                    [q]uit [?]help      │
├────────────────────────────────────┬────────────────────────────────────┤
│  auth (feat/auth) - running 3/7   │  payment (fix/payment) ⚠ APPROVE   │
├────────────────────────────────────┼────────────────────────────────────┤
│  [12:34:01] Implementing...       │  [12:33:45] PR created             │
│  [12:34:12] Running validation... │  [12:33:46] Awaiting approval      │
│  [12:34:18] ✓ Validation passed   │                                    │
│  [12:34:19] Running tests...      │  Press [Enter] to approve          │
│  █████████░░░░░░░░░ 45%           │                                    │
└────────────────────────────────────┴────────────────────────────────────┘
```

Features:
- Split panes, one per active workspace
- Real-time log streaming in each pane
- Arrow keys to navigate between panes
- Enter on highlighted pane to approve/interact
- Resize panes with +/-

*Note: Dashboard is a post-MVP enhancement. MVP focuses on `status`, `approve`, and `reject` commands.*

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
| **Multi-Repo** | Enterprise complexity | Users demonstrate concrete need |
| **Semantic Search** | Grep works for small memory | Memory exceeds ~1000 entries |
| **Trust Levels** | Need rejection data first | 100+ task completions |
| **Cloud Execution** | Local first | Need scale-out |
| **Other Languages** | Go-first simplifies validation | Go version is stable |
| **ADK/Genkit** | Direct SDK is simpler for v1 | Multi-agent workflows needed |
| **Additional PM Tools** | GitHub covers target users | Enterprise customers require |

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
| **SDD framework issues** | Speckit/BMAD failures | Graceful fallback, show framework output |

---

## 9. Known Obstacles & Risks

### 9.1 Implementation Obstacles

| Obstacle | Impact | Notes |
|----------|--------|-------|
| **Git credential complexity** | High | SSH vs HTTPS, PATs, 2FA. Budget time for edge cases. |
| **SDD framework installation** | Medium | Need uv for Speckit, npm for BMAD. Auto-install adds complexity. |
| **Worktree branch conflicts** | Medium | Handle existing branches gracefully. |
| **Large repo context** | Medium | File selection heuristics need iteration. |

### 9.2 Accepted Risks (v1)

| Risk | Mitigation | Revisit When |
|------|------------|--------------|
| Worktree left behind | User manually cleans up | Users complain repeatedly |
| No output sanitization | Human reviews all PRs | Security incident |
| API cost runaway | Timeout per task (300s) | Budget exceeded |
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

**Host (~/.atlas/):**
```
~/.atlas/
├── config.yaml               # Global configuration
├── memory/                   # Shared across all workspaces
│   ├── feedback/
│   ├── context/
│   └── decisions/
├── templates/                # User template overrides
│   └── custom.yaml
├── workspaces/               # Metadata about active workspaces
│   ├── auth.json
│   └── payment.json
└── logs/
    ├── atlas.log             # Host CLI operations
    └── workspaces/
        ├── auth/
        │   └── task-a1b2c3d4.log
        └── payment/
```

**Inside each worktree:**
```
~/projects/myrepo-auth/       # Git worktree
├── .atlas/
│   ├── tasks/                # Task state for this workspace
│   │   └── task-a1b2c3d4.json
│   └── artifacts/            # Generated artifacts (specs, plans, etc.)
│       ├── analysis.md
│       ├── spec.md
│       └── plan.md
├── .speckit/                 # Speckit config (if using)
├── .bmad/                    # BMAD config (if using)
└── ... (your code)
```

---

## Appendix B: Task Output Schema

```json
{
  "$schema": "atlas-task-output-v1",
  "task_id": "task-a1b2c3d4",
  "status": "completed",
  "workspace": "fix-null-pointer",
  "output": {
    "summary": "Fixed null pointer in parseConfig by adding nil check",
    "files_modified": [
      "pkg/config/parser.go",
      "pkg/config/parser_test.go"
    ],
    "artifacts": [
      ".atlas/artifacts/analysis.md",
      ".atlas/artifacts/implementation.md"
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

*This document describes ATLAS v1.1. See [templates.md](templates.md) for comprehensive template documentation.*
