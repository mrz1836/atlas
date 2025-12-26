# ATLAS: AI-Assisted Task Automation for Go Projects

**Version:** 1.0.0-DRAFT
**Tag:** v1-refined
**Status:** Vision Document

---

## 1. Executive Summary

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

**What ATLAS does:**
- Accepts a task description in natural language
- Coordinates AI agents to analyze, implement, and validate code
- Produces Git branches, commits, and pull requests
- Learns from accepted and rejected work to improve over time

**Built with:**
- Pure Go 1.24+ with minimal dependencies
- ADK Go for agent orchestration + Genkit for model abstraction
- Claude (primary) + Gemini (supported) AI backends
- Docker for workspace isolation

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
- Local Docker execution (cloud-ready architecture)
- Parallel feature development via isolated workspaces
- Claude (primary) + Gemini (supported) as AI backends

---

## 2. Core Principles

### Git is the Backbone

Git is not just version control—it's the audit trail, delivery mechanism, and source of truth. Every ATLAS action produces Git artifacts: branches, commits with machine-parseable trailers, and pull requests. If it's not in Git, it didn't happen.

### Text is Truth

All state is stored as human-readable text files. JSON for structured data, Markdown for prose. No databases, no binary formats. You can always `cat` your way to understanding what ATLAS did.

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
| Interactive TUI | `charmbracelet/bubbletea` | Modern terminal UX for wizards |
| Configuration | `spf13/viper` | Multi-source config, pairs with Cobra |
| Agent Orchestration | `google/adk-go` | Multi-agent workflows, cloud-native |
| Model Abstraction | `firebase/genkit` | Unified API for Claude + Gemini |
| Structured Logging | `rs/zerolog` | Zero-allocation, JSON-native |
| Docker SDK | `docker/docker` | Official client for container orchestration |
| GitHub API | `google/go-github` | Official GitHub v3/v4 client |

### AI Architecture (Hybrid Approach)

```
┌─────────────────────────────────────────────────┐
│                  ATLAS Core                      │
├─────────────────────────────────────────────────┤
│  ADK Go (Agent Orchestration)                   │
│  └─ Multi-agent workflows                       │
│  └─ Task delegation & coordination              │
│  └─ Tool/function calling                       │
├─────────────────────────────────────────────────┤
│  Genkit Go (Model Abstraction)                  │
│  └─ Unified API across providers                │
│  └─ Claude (primary) + Gemini (supported)       │
│  └─ One-line provider swapping                  │
└─────────────────────────────────────────────────┘
```

**Why hybrid:**
- ADK provides agent coordination patterns (multi-agent, delegation)
- Genkit provides clean model abstraction (swap Claude ↔ Gemini)
- Both are Google-backed, designed to work together
- Best of both: sophisticated agents + provider flexibility

### What We Don't Use

- No web framework (no HTTP server in v1)
- No dependency injection framework (explicit wiring)
- No LangChain (Genkit provides same abstraction, simpler)

---

## 4. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                       ATLAS CLI (Host)                          │
│                                                                 │
│  atlas init | start | status | approve | reject | workspace     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌───────────────────────────┐  ┌───────────────────────────┐   │
│  │  Workspace: auth-feature  │  │  Workspace: payment-fix   │   │
│  │  (Docker container)       │  │  (Docker container)       │   │
│  │  ┌─────────────────────┐  │  │  ┌─────────────────────┐  │   │
│  │  │ Cloned repo @ branch│  │  │  │ Cloned repo @ branch│  │   │
│  │  │ .atlas/tasks/*.json │  │  │  │ .atlas/tasks/*.json │  │   │
│  │  │ Task Engine + Agent │  │  │  │ Task Engine + Agent │  │   │
│  │  │ Go toolchain + Git  │  │  │  │ Go toolchain + Git  │  │   │
│  │  └─────────────────────┘  │  │  └─────────────────────┘  │   │
│  └───────────────────────────┘  └───────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    Shared (Host)                        │    │
│  │  Memory: ~/.atlas/memory/ (read-only mount)             │    │
│  │  Templates: ~/.atlas/templates/*.yaml                   │    │
│  │  Config: ~/.atlas/config.yaml                           │    │
│  │  Credentials: mounted into containers                   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Data flow:**
1. User runs `atlas start "fix the bug" --workspace bugfix-ws`
2. Host CLI creates/reuses Docker workspace container
3. Inside container: repo cloned, branch created, task JSON created
4. Task Engine (in container) runs task chain via ADK Go + Genkit
5. Git push happens from inside container
6. Human approves/rejects via host CLI; outcome stored in shared memory

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
- `atlas start "desc"` — Uses default workspace or creates one
- `atlas start "desc" --workspace feat-x` — Creates/reuses named workspace
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

**Requirements:**
- Go 1.24+
- Docker (for workspace isolation)
- GitHub CLI (`gh`) for PR operations

**First-time setup:**
```bash
atlas init
```

This launches an interactive TUI wizard (powered by Bubbletea) that configures:
- AI provider selection (Claude or Gemini)
- API credentials
- GitHub authentication
- Default workspace settings
- Project-specific templates (optional)

Configuration stored in `~/.atlas/config.yaml`.

### 5.2 Task Engine

Tasks are the atomic units of work. State lives in `.atlas/tasks/` as JSON files.

**Task lifecycle:**
```
pending ─► running ─► validating ─┬─► awaiting_approval ─► completed
                                  │         │
                                  │         └─► rejected ─► (new task or done)
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

**Retry configuration:**
- Default max retries: 3 (configurable via template)
- Retry context includes: original task + validation output + attempt number

**Task types:**
| Type | Executor | Auto-proceeds? |
|------|----------|----------------|
| AI | ADK/Genkit | No (pauses for approval) |
| Validation | golangci-lint, go test | Yes (if passing) |
| Git | Branch, commit, push, PR | No (pauses before PR creation) |
| Human | Developer action | N/A (waits for human) |

**Task JSON structure:**
```json
{
  "id": "task-a1b2c3d4",
  "type": "ai",
  "status": "pending",
  "template": "bugfix",
  "created_at": "2025-12-26T10:00:00Z",
  "input": {
    "description": "Fix null pointer in parseConfig",
    "files": ["pkg/config/parser.go"]
  },
  "output": null,
  "validation": {
    "must_pass": ["lint", "test"]
  },
  "git": {
    "repo": "owner/project",
    "base_branch": "main",
    "work_branch": "fix/null-pointer-parseconfig"
  },
  "links": {
    "depends_on": [],
    "blocks": ["task-review-xyz"]
  },
  "retry": {
    "count": 0,
    "max": 3,
    "last_failure": null
  }
}
```

**Task linking:**
- `depends_on`: This task waits for listed tasks to complete
- `blocks`: Completing this task unblocks listed tasks

#### Retry Loop (Validation Failures)

When validation fails, ATLAS automatically retries with AI assistance:

1. **Capture failure**: Store stdout/stderr from failed validation
2. **Build retry context**:
   ```
   Original task: "Fix null pointer in parseConfig"
   Attempt: 2 of 3
   Previous failure:
   --- golangci-lint output ---
   pkg/config/parser.go:47: unused variable 'cfg'
   --- go test output ---
   FAIL: TestParseConfig (missing nil check on line 52)
   ```
3. **Re-invoke AI**: "Fix the following validation failures: ..."
4. **Re-run validation**: Same commands as before
5. **Loop or exit**:
   - Pass → proceed to `awaiting_approval`
   - Fail + retries remaining → go to step 1
   - Fail + no retries → transition to `failed`, notify human

**Why auto-retry matters:**
- Most validation failures are minor (lint errors, missing edge cases)
- AI can fix these without human intervention
- Humans only see code that passes validation

**Templates:**
Pre-defined task chains for common workflows:

```yaml
# .atlas/templates/bugfix.yaml
name: bugfix
description: Fix a bug with tests
tasks:
  - type: ai
    name: analyze
    prompt: "Analyze the bug and identify root cause"
  - type: ai
    name: implement
    prompt: "Implement fix with test coverage"
    depends_on: [analyze]
  - type: validation
    name: validate
    commands: ["golangci-lint run", "go test ./..."]
    depends_on: [implement]
    auto_retry: 3
  - type: git
    name: commit
    depends_on: [validate]
  - type: git
    name: pr
    depends_on: [commit]
  - type: human
    name: review
    depends_on: [pr]
```

### 5.3 Agent Integration

ATLAS uses a hybrid approach: ADK Go for agent orchestration and Genkit for model abstraction. This provides sophisticated multi-agent workflows with flexible provider support.

**Configuration:**
```yaml
# .atlas/config.yaml
agent:
  provider: claude                    # or: gemini
  model: claude-sonnet-4-20250514     # or: gemini-2.0-flash
  timeout: 300s
  context:
    max_tokens: 100000                # Context budget per invocation
    max_file_lines: 2000              # Truncate files larger than this
```

**Architecture:**
- **ADK Go** handles multi-agent coordination, task delegation, and tool/function calling
- **Genkit** provides a unified model interface across Claude and Gemini
- Provider swapping requires only a config change—no code modifications

**Invocation pattern:**
1. Task Engine prepares context (files, previous outputs, memory)
2. ADK orchestrates agent workflow with appropriate tools
3. Genkit invokes the configured model (Claude or Gemini)
4. Captures output (code changes, analysis)
5. Validates output format
6. Stores result in task JSON

**Context provided to agent:**
- Task description and template context
- Relevant files (explicit in task definition)
- Recent memory entries (decisions, past feedback)
- Validation requirements

#### Context Window Strategy

Large codebases can exceed the model's context window. ATLAS uses a priority-based file selection strategy:

**Selection priority (highest to lowest):**
1. Files explicitly listed in task definition
2. Files in the same package as listed files
3. Files imported by listed files (one level deep)
4. Recent memory entries (last 10 decisions/feedback)
5. Project-level context (coding standards, architecture notes)

**Budget allocation (default 100k tokens):**
| Content Type | Max Allocation |
|-------------|----------------|
| Task + prompt | 5k |
| Explicit files | 50k |
| Related files | 30k |
| Memory | 10k |
| Reserved for output | 5k |

**Truncation rules:**
- Files over 2000 lines: Show first 500 + last 200 lines with `... [truncated N lines] ...`
- If still over budget: Omit lowest-priority content until within limit
- Always include: Task description, explicit files (even if truncated)

**Configuration override:**
```yaml
# Per-task override in template
tasks:
  - type: ai
    name: refactor
    context:
      max_tokens: 150000          # Larger budget for refactoring
      include_imports: true       # Follow import chain
      exclude_patterns:
        - "*_test.go"             # Skip test files
        - "vendor/*"
```

### 5.4 Memory

Memory persists context across tasks and sessions. Stored as Markdown files in `~/.atlas/memory/` (global, shared across all projects).

**Structure:**
```
~/.atlas/memory/
├── decisions/              # Architectural decisions
│   └── 2025-12-use-cobra.md
├── feedback/               # Rejection reasons, learnings
│   └── 2025-12-26-missing-error-handling.md
├── context/                # Project-specific context
│   └── coding-standards.md
└── archive/                # Completed project memories
```

**Memory entry format:**
```markdown
<!-- .atlas/memory/feedback/2025-12-26-null-check.md -->
---
type: feedback
outcome: rejected
task_id: task-a1b2c3d4
files: [pkg/config/parser.go]
created: 2025-12-26T14:30:00Z
---

# Rejection: Missing nil check in parseConfig

## What happened
PR was rejected because the fix didn't handle the case where `cfg.Options` is nil.

## Lesson
Always check nested struct fields for nil before accessing.

## Pattern
When fixing null pointer issues, trace all paths to the nil dereference.
```

**Search:**
Memory is searched via grep. Simple, debuggable, sufficient for hundreds of entries.

```bash
# Find all feedback about error handling
grep -r "error handling" .atlas/memory/
```

**Future:** Semantic search when grep proves insufficient (likely after 1000+ entries).

#### Memory Write Path

**Critical design decision:** Containers do NOT write to memory. Memory is read-only inside containers.

**Who writes memory:**
The host CLI generates memory entries based on user actions:

| User Action | Memory Written |
|-------------|----------------|
| `atlas approve <task>` | Optional success pattern (deferred for v1) |
| `atlas reject <task> "reason"` | Feedback entry with rejection reason |

**Why host-only writes:**
- Simplifies container permissions (read-only mount)
- Human provides rejection context (more valuable than AI guessing)
- Avoids race conditions between parallel workspaces
- Memory stays consistent even if container crashes

**Workflow example:**
```bash
$ atlas reject task-a1b2c3d4 "Missing nil check for nested Options struct"

# Host CLI creates:
# ~/.atlas/memory/feedback/2025-12-26-task-a1b2c3d4.md
# With user's rejection reason as the primary content
```

### 5.5 Git Operations

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

**Trailer fields:**
| Trailer | Purpose |
|---------|---------|
| `ATLAS-Task` | Links commit to task for full context |
| `ATLAS-Template` | Workflow template used |

**Querying ATLAS commits:**
```bash
git log --grep="ATLAS-Task: task-a1b2c3d4"
git log --grep="ATLAS-Template: bugfix" --oneline
```

**Branch naming:**
```
<type>/<short-description>
fix/null-pointer-parseconfig
feat/add-retry-logic
```

**Validation strategy:**

*Internal (before commit):*
- `golangci-lint run` — must pass
- `go test ./...` — must pass
- `go build ./...` — must compile

*External (after PR):*
- GitHub Actions status via `gh run list --branch <branch>`
- Wait for required checks before marking ready for review

**PR creation:**
```bash
gh pr create \
  --title "fix: handle nil config options" \
  --body "$(cat pr-description.md)" \
  --base main \
  --head fix/null-pointer-parseconfig
```

### 5.6 Workspace Isolation (Docker)

Workspaces enable working on multiple features simultaneously on the same repo without interference.

**Why Docker:**
- **Parallel features:** Feature A and Feature B run in separate containers
- **Clean state:** Each workspace starts with a fresh clone
- **Resource isolation:** Container limits prevent runaway processes
- **Cloud path:** Same container image works locally and on K8s/ECS tomorrow

**Workspace lifecycle:**
```bash
# Start two features in parallel
atlas start "add user authentication" --workspace auth-feature
atlas start "fix payment processing" --workspace payment-fix

# Check status of all workspaces
atlas status
# Output:
# WORKSPACE        STATUS              CURRENT TASK
# auth-feature     running             3/7 - Implementing
# payment-fix      awaiting_approval   5/7 - Review PR

# Manage workspaces
atlas workspace list              # List all workspaces
atlas workspace stop auth-feature # Pause a workspace
atlas workspace destroy payment-fix # Clean up after merge
```

**Container contents (base image):**
- Go 1.24+ toolchain
- Git with full capabilities
- ADK Go + Genkit for AI tasks
- golangci-lint for validation
- ATLAS task runner (lightweight)

**Mounts:**
| Host Path | Container Path | Mode |
|-----------|---------------|------|
| `~/.atlas/memory/` | `/atlas/memory/` | read-only |
| `~/.atlas/templates/` | `/atlas/templates/` | read-only |
| Git credentials | `/root/.gitconfig` | read-only |
| Model API keys (Claude/Gemini) | env var | — |

**Communication:**
- Host CLI orchestrates via `docker exec`
- Task state lives in container's `.atlas/tasks/`
- Results streamed via stdout/stderr
- Git push happens from inside container
- New memory entries written by host CLI after task completion

**What this does NOT add:**
- No Temporal (file-based state still)
- No gRPC (just docker exec + stdout)
- No complex orchestration layer
- No multi-node deployment

### 5.7 Host ↔ Container Communication

The host CLI and container communicate via `docker exec` with structured output.

```
┌──────────────────┐                      ┌──────────────────┐
│    Host CLI      │   docker exec        │    Container     │
│                  │ ──────────────────►  │                  │
│  atlas start     │                      │  task-runner     │
│  atlas status    │  ◄────────────────── │                  │
│  atlas approve   │   stdout: JSON       │  stdout: result  │
│                  │   stderr: logs       │  stderr: logs    │
│                  │   exit code          │                  │
└──────────────────┘                      └──────────────────┘
```

**Invocation:**
```bash
# Host CLI runs tasks via docker exec
docker exec -i <workspace-container> atlas-task-runner run <task-id>
```

**Exit codes:**
| Code | Meaning |
|------|---------|
| 0 | Task completed successfully |
| 1 | Task failed (retryable) |
| 2 | Task failed (not retryable, e.g., invalid input) |
| 3 | Validation failed (triggers retry loop) |
| 124 | Timeout exceeded |

**Stdout contract:**
Task runner outputs a single JSON object on completion:
```json
{
  "task_id": "task-a1b2c3d4",
  "status": "completed",
  "output": {
    "summary": "Fixed null pointer in parseConfig",
    "files_modified": ["pkg/config/parser.go"],
    "validation": {
      "lint": {"passed": true},
      "test": {"passed": true}
    }
  },
  "retry_count": 0,
  "duration_ms": 45000
}
```

**Stderr contract:**
- Structured log lines (JSON-lines format)
- Used for debugging, not parsed by host CLI
- Persisted to `~/.atlas/logs/workspaces/<name>/<task-id>.log`

**Status polling:**
```bash
# Check if task is still running
docker exec <container> atlas-task-runner status <task-id>
# Returns: {"status": "running", "step": "validating", "progress": "2/3"}
```

### 5.8 Observability

**Log locations:**
```
~/.atlas/logs/
├── atlas.log                    # Host CLI operations
└── workspaces/
    ├── auth-feature/
    │   ├── task-a1b2c3d4.log   # Full task execution log
    │   └── task-e5f6g7h8.log
    └── payment-fix/
        └── ...
```

**Log format (JSON-lines):**
```json
{"ts":"2025-12-26T10:00:00Z","level":"info","event":"task_start","task_id":"task-a1b2c3d4"}
{"ts":"2025-12-26T10:00:05Z","level":"info","event":"model_invoke","tokens_in":15000}
{"ts":"2025-12-26T10:00:45Z","level":"info","event":"model_complete","tokens_out":2500,"duration_ms":40000}
{"ts":"2025-12-26T10:00:46Z","level":"info","event":"validation_start","command":"golangci-lint run"}
{"ts":"2025-12-26T10:00:52Z","level":"info","event":"validation_complete","passed":true}
```

**What gets logged:**
| Event | Data |
|-------|------|
| `task_start` | Task ID, type, template |
| `model_invoke` | Token count (input), prompt hash |
| `model_complete` | Token count (output), duration, success/error |
| `validation_start` | Command being run |
| `validation_complete` | Pass/fail, duration, output (truncated) |
| `state_transition` | From state, to state, trigger |
| `retry` | Attempt number, failure reason |

**Debugging commands:**
```bash
# What's happening right now?
atlas status --verbose

# Full log for a specific task
cat ~/.atlas/logs/workspaces/auth-feature/task-a1b2c3d4.log

# Tail container logs live
atlas workspace logs auth-feature --follow

# Parse logs with jq
cat ~/.atlas/logs/workspaces/*/task-*.log | jq 'select(.event=="model_complete")'
```

**Retention:**
- Keep logs for 7 days or 100MB, whichever limit is hit first
- Configurable via `~/.atlas/config.yaml`

**NOT in v1:**
- Metrics/telemetry export
- Distributed tracing
- Log aggregation service integration

---

## 6. Workflow Examples

### Bugfix Workflow

```
User: atlas start "fix null pointer panic in parseConfig when options is nil"
  │
  ├─► Task 1: Analyze (AI)
  │   └─► Output: "Root cause: cfg.Options accessed without nil check at line 47"
  │
  ├─► Task 2: Implement (AI)
  │   └─► Output: Code changes + new test case
  │
  ├─► Task 3: Validate (Auto)
  │   ├─► golangci-lint run ✓
  │   ├─► go test ./... ✓
  │   └─► Auto-proceeds (validation passed)
  │
  ├─► Task 4: Commit (Auto)
  │   └─► Creates branch, commits with trailers, pushes
  │
  ├─► Task 5: Open PR (Auto)
  │   └─► gh pr create, monitors CI
  │
  └─► Task 6: Review (Human)
      └─► atlas approve task-xyz OR atlas reject task-xyz "reason"
```

**After human review (Host CLI behavior, not a container task):**
```
atlas approve task-xyz
  └─► Task completed, workspace ready for cleanup

atlas reject task-xyz "Missing nil check for cfg.Options.Nested"
  └─► Host CLI writes feedback to ~/.atlas/memory/feedback/
  └─► Task marked rejected
  └─► User can start new task with context from rejection
```

**What happens on rejection:**
```bash
atlas reject task-xyz "Missing nil check for cfg.Options.Nested"
```
1. Feedback stored in memory
2. Task marked failed
3. New task chain can be started with context: "Previous attempt rejected because..."

### Feature Workflow

Same structure, different template:
```bash
atlas start "add retry logic to HTTP client" --template feature
```

The `feature` template might include additional steps:
- Design review (human checkpoint before implementation)
- Documentation update task
- Changelog entry task

### Parallel Features Workflow

Work on multiple features simultaneously using workspaces:

```
Terminal 1:
$ atlas start "add user authentication" --workspace auth
Creating workspace 'auth'...
  → Cloning repo into container
  → Creating branch: feat/add-user-authentication
  → Starting task chain...

Task 1: Analyze (AI) ─────────────────────────────►│
                                                   │
Terminal 2:                                        │
$ atlas start "fix payment timeout" --workspace pay│
Creating workspace 'pay'...                        │
  → Cloning repo into container                    │
  → Creating branch: fix/payment-timeout           │
  → Starting task chain...                         │
                                                   │
Task 1: Analyze (AI) ──────► │                     │ (running in parallel)
Task 2: Implement (AI) ─────►│                     │
                             │                     │
$ atlas status               │                     │
┌──────────────────────────────────────────────────┐
│ WORKSPACE   STATUS              TASK             │
│ auth        running             2/7 Implement    │
│ pay         awaiting_approval   5/7 Review PR    │
└──────────────────────────────────────────────────┘

$ atlas approve pay-task-xyz
  → PR merged
  → Memory updated with outcome
  → Workspace 'pay' ready for cleanup

$ atlas workspace destroy pay
  → Container removed
```

**Key points:**
- Each workspace is completely isolated
- No merge conflicts between parallel work
- Memory is shared (learnings from `pay` available to `auth`)
- Host CLI manages all workspaces from single terminal

---

## 7. What's Deferred

These features are explicitly out of scope for v1. Each has a trigger for when to revisit.

| Feature | Why Deferred | Revisit When |
|---------|--------------|--------------|
| **SDD Frameworks** | Complexity unclear, need usage data | Users request spec-driven workflow |
| **Multiple PM Tools** | GitHub covers target users | Enterprise customers require Jira/Linear |
| **Semantic Search** | Grep works for small memory | Memory exceeds ~1000 entries |
| **Trust Levels** | Need rejection data first | 100+ task completions with outcome data |
| **Multi-Repo** | Enterprise complexity | Users demonstrate concrete need |
| **Temporal/Durable Execution** | File state is sufficient | Workflows exceed file-based limits |
| **Cloud Execution** | Docker locally first | Local CPU exhausted, need scale-out |
| **Other Languages** | Go-first simplifies validation | Go version is stable and adopted |
| **Additional AI Providers** | Claude + Gemini covers most needs | Users require OpenAI, Mistral, etc. |

**Philosophy:** If it's not blocking the next shipped feature, it doesn't exist yet.

---

## 8. Failure Modes

How ATLAS fails, and how the design mitigates it:

| Failure | Symptom | Mitigation |
|---------|---------|------------|
| **Setup too hard** | Users abandon before first task | One command: `atlas init` with sensible defaults |
| **Unclear state** | "What is it doing?" confusion | All state in readable JSON/Markdown files |
| **Bad output quality** | Rejected PRs, wasted review time | Validation gates (lint, test) before human review |
| **Too slow** | Context switching, lost momentum | Local execution, no network dependencies for core loop |
| **Breaks workflow** | Merge conflicts, CI failures | Additive only—works with existing Git practices |
| **AI makes mistakes** | Incorrect code, missed edge cases | Human approval required for all code changes |
| **Container crash** | Task disappears, no output | Accept for v1. User restarts. State is cheap. |
| **Context overflow** | AI produces garbage or refuses task | File selection strategy + truncation (see 5.3) |
| **Retry loop stuck** | Same failure 3 times in a row | Fail task, surface to human with full context |
| **Model API changes** | Task hangs or errors unexpectedly | Pin ADK/Genkit versions, test with provider updates |

**Design question for every feature:** Does this make ATLAS easier to adopt and harder to abandon?

---

## 9. Known Obstacles & Risks

This section documents implementation challenges and accepted risks for v1.

### 9.1 Implementation Obstacles

| Obstacle | Impact | Notes |
|----------|--------|-------|
| **Git credential complexity** | High | SSH vs HTTPS, personal access tokens, 2FA, enterprise SSO. Budget significant time for edge cases. |
| **Docker image distribution** | Medium | Need strategy: Dockerfile in repo (user builds), published image (we host), or bundled binary. Each has trade-offs. |
| **Headless AI invocation** | Medium | ADK/Genkit run headless by design. Verify container can reach model APIs. |
| **Large repo context** | Medium | File selection heuristics will need iteration based on real usage. First implementation will be naive. |
| **Cross-platform Docker** | Medium | Docker Desktop licensing, Docker Engine on Linux, Colima on Mac. Different behavior per platform. |

### 9.2 Accepted Risks (v1)

These are known risks we're explicitly accepting for v1 to ship faster:

| Risk | Mitigation | Revisit When |
|------|------------|--------------|
| Container crash = task loss | User restarts task. Simple for v1. | Users complain about lost work repeatedly |
| No output sanitization | Human reviews all PRs. Trust but verify. | Security incident or enterprise customer requires |
| API cost runaway | Timeout per task (300s). No loop detection. | Budget exceeded unexpectedly |
| Memory race conditions | Last-write-wins. Unlikely with human-speed approvals. | Automated batch processing is added |
| No secret detection | Human review catches secrets in PRs | Secrets leaked to PR (pre-commit hook recommended) |

### 9.3 Security Acknowledgment

The container has access to:
- **Model API keys** (Claude/Gemini): Can incur costs, potential for prompt injection
- **Git push credentials**: Can push code to any branch (except protected)
- **Network access**: Model APIs, GitHub API, Go module proxies

**v1 security stance:**
Human approval is the security boundary. All code is reviewed before merge. Detailed threat modeling deferred until v1 is validated with real users.

**Recommendations for users:**
- Use branch protection rules on main/master
- Require PR reviews before merge
- Set up API budget alerts with your AI provider (Anthropic/Google)
- Consider using GitHub App tokens with minimal scopes instead of PATs

---

## Appendix A: File Structure

**Host (~/.atlas/):**
```
~/.atlas/
├── config.yaml               # Global configuration
├── memory/                   # Shared across all workspaces (read-only mount)
│   ├── decisions/
│   ├── feedback/
│   ├── context/
│   └── archive/
├── templates/                # Shared workflow templates
│   ├── bugfix.yaml
│   └── feature.yaml
├── workspaces/               # Metadata about active workspaces
│   ├── auth-feature.json
│   └── payment-fix.json
└── logs/                     # Task execution logs
    ├── atlas.log             # Host CLI operations
    └── workspaces/
        ├── auth-feature/
        │   └── task-a1b2c3d4.log
        └── payment-fix/
            └── task-e5f6g7h8.log
```

**Inside each workspace container:**
```
/workspace/
├── <cloned-repo>/            # Full repo clone at feature branch
│   ├── .atlas/
│   │   └── tasks/            # Task state for this workspace
│   │       ├── task-a1b2c3d4.json
│   │       └── task-e5f6g7h8.json
│   └── ... (your code)
├── /atlas/memory/            # Read-only mount from host
└── /atlas/templates/         # Read-only mount from host
```

---

## Appendix B: Task Output Schema

The task runner outputs this JSON structure on stdout when a task completes:

```json
{
  "$schema": "atlas-task-output-v1",
  "task_id": "task-a1b2c3d4",
  "status": "completed",
  "exit_code": 0,
  "output": {
    "summary": "Fixed null pointer in parseConfig by adding nil check",
    "files_modified": [
      "pkg/config/parser.go",
      "pkg/config/parser_test.go"
    ],
    "validation_results": {
      "lint": {
        "passed": true,
        "command": "golangci-lint run",
        "duration_ms": 3200,
        "output": ""
      },
      "test": {
        "passed": true,
        "command": "go test ./...",
        "duration_ms": 8500,
        "output": "ok  \tgithub.com/user/repo/pkg/config\t0.045s"
      }
    },
    "git": {
      "branch": "fix/null-pointer-parseconfig",
      "commit": "abc1234",
      "pr_url": "https://github.com/user/repo/pull/42"
    }
  },
  "error": null,
  "retry_count": 0,
  "duration_ms": 45000,
  "tokens": {
    "input": 15000,
    "output": 2500
  }
}
```

**Status values:**
| Status | Meaning |
|--------|---------|
| `completed` | Task finished successfully, ready for human review |
| `failed` | Task failed permanently (not retryable) |
| `validation_failed` | Validation failed, retry limit reached |

**Error structure (when status is `failed`):**
```json
{
  "error": {
    "code": "MODEL_TIMEOUT",
    "message": "Model API did not respond within 300s",
    "details": { "timeout_ms": 300000 }
  }
}
```

---

*This document describes ATLAS v1. It will evolve based on real usage.*
