# ATLAS: AI-Assisted Task Automation for Go Projects

**Version:** 1.0.0-DRAFT
**Status:** Vision Document

---

## 1. Executive Summary

ATLAS is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

**What ATLAS does:**
- Accepts a task description in natural language
- Coordinates AI agents to analyze, implement, and validate code
- Produces Git branches, commits, and pull requests
- Learns from accepted and rejected work to improve over time

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
- Local execution (no cloud infrastructure required)
- Claude as the AI backend

---

## 2. Core Principles

### 1. Git is the Backbone

Git is not just version control—it's the audit trail, delivery mechanism, and source of truth. Every ATLAS action produces Git artifacts: branches, commits with machine-parseable trailers, and pull requests. If it's not in Git, it didn't happen.

### 2. Text is Truth

All state is stored as human-readable text files. JSON for structured data, Markdown for prose. No databases, no binary formats. You can always `cat` your way to understanding what ATLAS did.

### 3. Human Authority at Checkpoints

AI proposes, humans dispose. Validation tasks (lint, test) auto-proceed on success. Code changes always pause for approval. No unsupervised merges, ever.

### 4. Ship Then Iterate

Start with the simplest thing that works. Add complexity only when real usage demands it. If a feature isn't needed for the next task, it doesn't exist yet.

### 5. Transparent State

Every file ATLAS creates is inspectable. No hidden state, no opaque databases. Debug by reading files. Trust by verifying.

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         ATLAS CLI                           │
│                                                             │
│  atlas init | start | status | approve | reject             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │ Task Engine  │───►│  AI Agent    │───►│  Git Ops     │   │
│  │              │    │              │    │              │   │
│  │ JSON files   │    │ Claude CLI   │    │ branch/PR    │   │
│  │ in .atlas/   │    │              │    │ via gh       │   │
│  └──────┬───────┘    └──────────────┘    └──────────────┘   │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────┐    ┌──────────────┐                       │
│  │   Memory     │    │  Templates   │                       │
│  │              │    │              │                       │
│  │ Markdown in  │    │ YAML workflow│                       │
│  │ .atlas/      │    │ definitions  │                       │
│  └──────────────┘    └──────────────┘                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Data flow:**
1. User runs `atlas start "fix the bug"`
2. Task Engine creates task JSON, selects template
3. Template defines task chain (analyze → implement → validate → commit → PR)
4. Each task invokes AI Agent or automated validation
5. Git Ops creates branches, commits (with trailers), and PRs
6. Human approves or rejects; outcome stored in Memory

---

## 4. Components

### 4.1 CLI Interface

Five commands cover 95% of usage:

```bash
atlas init                      # Initialize ATLAS in current repo
atlas start "description"       # Start a task from natural language
atlas status                    # Show running tasks and pending approvals
atlas approve <task-id>         # Approve pending work
atlas reject <task-id> "reason" # Reject with feedback (stored for learning)
```

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

### 4.2 Task Engine

Tasks are the atomic units of work. State lives in `.atlas/tasks/` as JSON files.

**Task lifecycle:**
```
pending → running → validating → awaiting_approval → completed
                 ↘ failed (retry or human intervention)
```

**Task types:**
| Type | Executor | Auto-proceeds? |
|------|----------|----------------|
| AI | Claude | No (pauses for approval) |
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
  }
}
```

**Task linking:**
- `depends_on`: This task waits for listed tasks to complete
- `blocks`: Completing this task unblocks listed tasks

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

### 4.3 Agent Integration

ATLAS uses Claude (via the `claude` CLI) as its AI backend. No abstraction layer—direct integration.

**Configuration:**
```yaml
# .atlas/config.yaml
agent:
  command: claude
  model: claude-sonnet-4-20250514
  timeout: 300s
```

**Invocation pattern:**
1. Task Engine prepares context (files, previous outputs, memory)
2. Invokes `claude` with structured prompt
3. Captures output (code changes, analysis)
4. Validates output format
5. Stores result in task JSON

**Context provided to agent:**
- Task description and template context
- Relevant files (explicit in task definition)
- Recent memory entries (decisions, past feedback)
- Validation requirements

### 4.4 Memory

Memory persists context across tasks and sessions. Stored as Markdown files in `.atlas/memory/`.

**Structure:**
```
.atlas/memory/
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

### 4.5 Git Operations

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

---

## 5. Workflow Examples

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
  ├─► Task 6: Review (Human)
  │   └─► atlas approve task-xyz OR atlas reject task-xyz "reason"
  │
  └─► Task 7: Learn (Auto)
      └─► Stores outcome in .atlas/memory/feedback/
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

---

## 6. What's Deferred

These features are explicitly out of scope for v1. Each has a trigger for when to revisit.

| Feature | Why Deferred | Revisit When |
|---------|--------------|--------------|
| **SDD Frameworks** | Complexity unclear, need usage data | Users request spec-driven workflow |
| **Multiple PM Tools** | GitHub covers target users | Enterprise customers require Jira/Linear |
| **Semantic Search** | Grep works for small memory | Memory exceeds ~1000 entries |
| **Trust Levels** | Need rejection data first | 100+ task completions with outcome data |
| **Multi-Repo** | Enterprise complexity | Users demonstrate concrete need |
| **Temporal/Durable Execution** | File state is sufficient | Workflows exceed file-based limits |
| **Docker Isolation** | Local execution is fine | Security requirements emerge |
| **Other Languages** | Go-first simplifies validation | Go version is stable and adopted |
| **Other AI Backends** | Claude integration is primary | Users require Anthropic alternatives |

**Philosophy:** If it's not blocking the next shipped feature, it doesn't exist yet.

---

## 7. Failure Modes

How ATLAS fails, and how the design mitigates it:

| Failure | Symptom | Mitigation |
|---------|---------|------------|
| **Setup too hard** | Users abandon before first task | One command: `atlas init` with sensible defaults |
| **Unclear state** | "What is it doing?" confusion | All state in readable JSON/Markdown files |
| **Bad output quality** | Rejected PRs, wasted review time | Validation gates (lint, test) before human review |
| **Too slow** | Context switching, lost momentum | Local execution, no network dependencies for core loop |
| **Breaks workflow** | Merge conflicts, CI failures | Additive only—works with existing Git practices |
| **AI makes mistakes** | Incorrect code, missed edge cases | Human approval required for all code changes |

**Design question for every feature:** Does this make ATLAS easier to adopt and harder to abandon?

---

## Appendix: File Structure

```
project-root/
├── .atlas/
│   ├── config.yaml           # Project configuration
│   ├── tasks/
│   │   ├── task-a1b2c3d4.json
│   │   └── task-e5f6g7h8.json
│   ├── memory/
│   │   ├── decisions/
│   │   ├── feedback/
│   │   ├── context/
│   │   └── archive/
│   └── templates/
│       ├── bugfix.yaml
│       └── feature.yaml
└── ... (your code)
```

---

*This document describes ATLAS v1. It will evolve based on real usage.*
