# ATLAS: Adaptive Teamwork Layer for AI Systems

## High-Level Vision Document

**Version:** 0.3.0-DRAFT
**Tag:** v0.3-original
**Last Updated:** December 2025
**Status:** Vision & Architecture Definition

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Vision Statement](#vision-statement)
3. [Core Philosophy](#core-philosophy)
   - [Human Authority at Decision Points](#3-human-authority-at-decision-points)
     - [Trust & Autonomy Ladder](#trust--autonomy-ladder)
   - [Git as the Backbone](#6-git-as-the-backbone)
     - [Identity & Commit Model](#identity--commit-model)
   - [Integration & Usability Standards](#7-integration--usability-standards)
     - [Metrics & Observability](#metrics--observability)
   - [UX Philosophy](#8-ux-philosophy-simple-surface-smart-depths)
4. [System Architecture Overview](#system-architecture-overview)
5. [Component Architecture](#component-architecture)
   - [Application Configuration](#1-application-configuration)
   - [Project & Task Engine](#2-project--task-engine)
     - [Context Strategy](#context-strategy)
     - [Multi-Repo Task Coordination](#multi-repo-task-coordination)
     - [Validation Strategy](#validation-strategy)
   - [Project Adapters](#3-project-adapters)
   - [SDD Adapter System](#4-sdd-adapter-system)
     - [Unified Phase Model](#unified-phase-model)
   - [Memory Adapter System](#5-memory-adapter-system)
     - [Session & Memory Lifecycle](#session--memory-lifecycle)
   - [Communication Abstraction](#6-communication-abstraction)
   - [Orchestration Agent](#7-orchestration-agent)
   - [Agent Runtime Environment](#8-agent-runtime-environment)
   - [Research Agent (Internal)](#9-research-agent-internal)
   - [Ecosystem Updates](#10-ecosystem-updates)
   - [Learning & Feedback System](#11-learning--feedback-system)
6. [Integration Patterns](#integration-patterns)
7. [MVP Definition](#mvp-definition)
8. [Failure Modes & Mitigation](#failure-modes--mitigation)
9. [Open Research Areas](#open-research-areas)
10. [Decision Framework](#decision-framework)
11. [Appendix: SDD Framework Landscape](#appendix-sdd-framework-landscape)

---

## Executive Summary

ATLAS is an orchestration system designed to function as a **virtual employee** within development teams. It bridges the gap between project management tools (Asana, Linear, GitHub Projects) and AI-powered development workflows, enabling autonomous task execution while maintaining human oversight at critical decision points.

At its core, ATLAS:

- **Integrates** with existing project management tools while maintaining its own internal task orchestration
- **Abstracts** Spec-Driven Development (SDD) frameworks, allowing flexible adoption without vendor lock-in
- **Maintains** persistent, contextual memory across projects, tasks, and sessions
- **Orchestrates** AI agents and human tasks through a unified workflow engine
- **Produces** production-ready code through branches, commits, and pull requests

ATLAS is not a single monolithic application. It is a **composable ecosystem** of CLI tools, adapters, and agents that can be assembled to match specific team workflows.

---

## Vision Statement

> **Create an orchestration ecosystem that allows teams to integrate AI agents and Spec-Driven Development into real-world workflows, acting as a virtual team member that maintains context, produces quality code, and respects human decision authority.**

### Long-Term Vision

ATLAS evolves into a living ecosystem that:

- Bridges multiple project management tools through a unified internal management interface
- Supports diverse workloads beyond code: research, cron jobs, monitoring, and automated lifecycle management
- Self-improves through experience, learning from successful patterns and failed attempts
- Scales from individual developer workflows to team-wide orchestration

### Near-Term Focus (MVP)

- GitHub and Git as the primary integration surface
- File-based approaches leveraging Git's native capabilities
- Local execution without GUI requirements
- Core plumbing: task execution, status updates, human communication, code delivery via branches and PRs

---

## Core Philosophy

### 1. Decoupled by Design

Every component connects through well-defined interfaces. No component assumes knowledge of another's implementation. This enables:

- Independent evolution of each subsystem
- Flexible composition for different workflows
- Testability at every boundary

### 2. Interfaces Over Implementations

Define **what** components can do, not **how** they do it. Adapters and capabilities drive the architecture:

```
┌─────────────────┐      ┌─────────────────┐
│  Project Tool   │◄────►│ Project Adapter │
│  (Asana, etc.)  │      │   Interface     │
└─────────────────┘      └─────────────────┘
                               │
                               ▼
                    ┌─────────────────┐
                    │  ATLAS Core     │
                    │  Task Engine    │
                    └─────────────────┘
```

### 3. Human Authority at Decision Points

AI agents execute tasks, but humans retain authority at critical junctures:

- Accepting proposed solutions
- Reviewing pull requests
- Approving architectural decisions
- Overriding automated judgments

#### Trust & Autonomy Ladder

Trust is earned over time. ATLAS operates at different autonomy levels based on track record and task type:

| Level | Name | Behavior |
|-------|------|----------|
| 1 | Supervised | Every output requires human approval before action |
| 2 | **Semi-Supervised** (MVP Default) | Validation tasks auto-proceed; code changes need approval |
| 3 | Guided | Routine tasks auto-proceed; novel situations pause |
| 4 | Autonomous | Most tasks proceed; only high-risk actions pause |

**Semi-Supervised Rules (MVP):**
- ✅ **Auto-proceed:** Lint passes, tests pass, pre-commit hooks pass
- ⏸️ **Pause for approval:** Code changes, PR creation, spec approval, architectural decisions
- 🔄 **Auto-retry:** Validation failures (up to N attempts with automatic fixes)

**Trust Signals (Future):**
- Task success rate in this repo/project
- Consecutive successful completions
- Validation pass rate
- No rejected PRs in last N tasks

**Escalation Rules:**
- Human doesn't respond in X hours → reminder notification
- Still no response in Y hours → pause workflow and alert

### 4. Incremental Complexity

Start simple. Add complexity only when real usage demands it. Over-engineering is the primary failure mode to avoid.

### 5. Text as Truth

For memory, configuration, and state: text is the canonical format. Derived artifacts (embeddings, indexes, caches) can always be regenerated from text sources.

### 6. Git as the Backbone

Git is not just version control—it is the communication medium, audit trail, and delivery mechanism. Leverage its full feature set: branches, commits, PRs, reviews, and actions.

#### Identity & Commit Model

ATLAS actions must be traceable. Every commit, branch, and PR links back to internal task state.

**MVP Authentication:**
- Use user's `GITHUB_TOKEN` or `PAT_TOKEN` for all Git operations
- ATLAS operates under the user's identity (no impersonation concerns)
- Future: Dedicated GitHub App/Bot with distinct identity

**Commit Traceability via Git Trailers:**

All ATLAS commits include machine-parseable trailers:

```
fix: handle empty input validation in API handler

Implemented null check before processing request body.
Added unit test for edge case.

ATLAS-Task: task-550e8400-e29b-41d4
ATLAS-Template: bugfix
ATLAS-Project: proj-api-improvements
```

**Trailer Fields:**
| Trailer | Purpose |
|---------|---------|
| `ATLAS-Task` | Links to internal task ID for full context |
| `ATLAS-Template` | Identifies workflow template used |
| `ATLAS-Project` | Parent project for grouping |

**Audit Capabilities:**
- Query all commits by task: `git log --grep="ATLAS-Task: task-123"`
- Trace production issues back to originating task
- Generate reports on ATLAS contributions

### 7. Integration & Usability Standards

Every component in the ATLAS ecosystem follows consistent patterns for integration and usability. These standards ensure components are easy to compose, debug, and extend.

#### Structured JSON Logging

All components emit structured JSON logs with consistent fields:

```json
{
  "timestamp": "2025-12-15T10:30:00.000Z",
  "level": "info",
  "component": "task-engine",
  "correlation_id": "550e8400-e29b-41d4-a716-446655440000",
  "message": "Task execution started",
  "context": {
    "task_id": "task-123",
    "task_type": "ai",
    "template": "bugfix"
  }
}
```

**Log Levels:**
- `debug` - Detailed diagnostic information
- `info` - Normal operational events
- `warn` - Unusual situations that may need attention
- `error` - Failures that require intervention

#### Metrics & Observability

Every task execution captures metrics for debugging, performance tracking, and optimization.

**Task-Level Metrics:**

```json
{
  "task_id": "task-550e8400",
  "metrics": {
    "duration_ms": 45000,
    "tokens": {
      "input": 12500,
      "output": 3200,
      "total": 15700
    },
    "retries": 1,
    "validations": {
      "lint": { "status": "pass", "duration_ms": 2300 },
      "test": { "status": "pass", "duration_ms": 8900 }
    },
    "files": {
      "read": 8,
      "written": 2
    }
  }
}
```

**Project-Level Aggregates:**
- Total tasks executed, success rate
- Average time to completion
- Token efficiency trends
- Common failure patterns

**Debug Mode:**
- Verbose logging with full agent reasoning chains
- Replay capability for failed tasks
- Context snapshot at each decision point

#### Standard I/O Contract

All component inputs and outputs use JSON for machine-readable communication:

- **Inputs:** Accept JSON via stdin, files, or HTTP request bodies
- **Outputs:** Produce JSON to stdout or HTTP response bodies
- **Streaming:** Use NDJSON (newline-delimited JSON) for continuous output

**Standard Error Envelope:**

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Task input validation failed",
    "details": {
      "field": "input.context",
      "reason": "required field missing"
    }
  }
}
```

**Standard Success Envelope:**

```json
{
  "success": true,
  "data": { ... },
  "metadata": {
    "duration_ms": 1234,
    "correlation_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

#### CLI Interface Standards

ATLAS provides a single `atlas` binary with consistent subcommands for all components:

```
atlas <component> <action> [flags]
```

**Universal Flags:**
- `--output json|yaml|table` - Control output format (default: table for TTY, json otherwise)
- `--verbose` - Enable debug logging
- `--quiet` - Suppress non-essential output
- `--correlation-id <id>` - Set correlation ID for request tracing

**Exit Codes:**
- `0` - Success
- `1` - Execution error
- `2` - Invalid input or configuration

**Piping Support:** Commands read from stdin and write to stdout, enabling composition:

```bash
atlas sdd generate --input reqs.json | atlas task create --template feature
```

#### Component Composition

- **Local:** JSON over stdin/stdout for CLI piping
- **Network:** gRPC for inter-service communication
- **Configuration:** Environment variables and YAML config files
- **Health:** Long-running components expose `/health` and `/ready` endpoints

### 8. UX Philosophy: Simple Surface, Smart Depths

The user-facing interface must be minimal and obvious. Complexity lives in adapters and orchestration—never in user interaction.

#### Minimal CLI Surface

The 80% use case requires only these commands:

```bash
atlas init                    # Initialize ATLAS in a repo
atlas start "fix login bug"   # Start a project from natural language
atlas status                  # See what's running
atlas approve <id>            # Approve a pending task
atlas reject <id> "reason"    # Reject with feedback (triggers learning)
atlas upgrade                 # Update ATLAS and all integrations
```

#### Design Principles

- **Progressive Disclosure:** Advanced options exist but aren't required
- **Defaults That Work:** Sensible defaults for everything; override when needed
- **Immediate Feedback:** Every command shows clear status and next steps
- **No Magic:** Users can always inspect state, logs, and artifacts

#### Why This Matters

ATLAS fails if it's:
1. Too complicated to set up
2. Unclear what's happening
3. Hard to understand the output
4. Difficult to get started

The antidote is ruthless simplicity at the surface, with depth available when needed.

---

## System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              ATLAS ECOSYSTEM                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐            │
│  │  Project        │   │  Communication  │   │  Research       │            │
│  │  Adapters       │   │  Abstraction    │   │  Agent          │            │
│  │                 │   │                 │   │                 │            │
│  │ • Asana         │   │ • Email         │   │ • SDD Tracking  │            │
│  │ • Linear        │   │ • Discord       │   │ • Memory R&D    │            │
│  │ • GitHub Proj.  │   │ • Slack         │   │ • Breaking Chg  │            │
│  │ • Self-Hosted   │   │ • GitHub Issues │   │ • New Projects  │            │
│  └────────┬────────┘   └────────┬────────┘   └────────┬────────┘            │
│           │                     │                     │                     │
│           ▼                     ▼                     ▼                     │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      ORCHESTRATION LAYER                            │    │
│  │  ┌───────────────────────────────────────────────────────────────┐  │    │
│  │  │                  Orchestration Agent                          │  │    │
│  │  │           (Temporal-based Workflow Engine)                    │  │    │
│  │  └───────────────────────────────────────────────────────────────┘  │    │
│  │                              │                                      │    │
│  │  ┌───────────────────────────┴───────────────────────────┐          │    │
│  │  │              Project & Task Engine                    │          │    │
│  │  │  • Projects (encapsulation of work)                   │          │    │
│  │  │  • Tasks (atomic units with I/O)                      │          │.   │
│  │  │  • Templates (pre-built workflows)                    │          │    │
│  │  │  • Task Linking (dependencies, blocking)              │          │    │
│  │  └───────────────────────────────────────────────────────┘          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                              │                                              │
│           ┌──────────────────┼──────────────────┐                           │
│           ▼                  ▼                  ▼                           │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐                │
│  │  SDD Adapter    │ │  Memory Adapter │ │  Application    │                │
│  │  System         │ │  System         │ │  Configuration  │                │
│  │                 │ │                 │ │                 │                │
│  │ • Speckit       │ │ • File Backend  │ │ • Model Defs    │                │
│  │ • BMAD          │ │ • Semantic      │ │ • SDD Defaults  │                │
│  │ • OpenSpec      │ │ • Hybrid Search │ │ • API Keys      │                │
│  │ • Kiro          │ │ • Lifecycle Mgmt│ │ • Tool Configs  │                │
│  │ • Conductor     │ │ • Scoped Access │ │ • Profiles      │                │
│  └─────────────────┘ └─────────────────┘ └─────────────────┘                │
│                              │                                              │
│                              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    AGENT RUNTIME LAYER                              │    │
│  │  ┌───────────────────────────────────────────────────────────────┐  │    │
│  │  │              Docker Container Environment                     │  │    │
│  │  │  • Go Runtime + Build Tools (Mage-x)                          │  │    │
│  │  │  • Agent CLI Installation (Claude Code, etc.)                 │  │    │
│  │  │  • SDD Framework Installation                                 │  │    │
│  │  │  • Network Access (controlled)                                │  │    │
│  │  │  • Temporal Workers (Go ADK, Genkit)                          │  │    │
│  │  └───────────────────────────────────────────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Architecture

### 1. Application Configuration

**Purpose:** Centralized defaults and settings for the entire ATLAS ecosystem.

**Responsibilities:**

- Model definitions (which AI models are available, default selections)
- SDD framework defaults (which framework to use when not specified)
- API key management (secure storage and retrieval)
- Tool configurations (Git settings, container defaults)
- Profile management (per-project or per-user overrides)

**Key Design Decisions:**

- Configuration as code (YAML/TOML files under version control)
- Environment-aware layering (base → environment → project → runtime)
- Secret separation (keys never in config files, use environment or vault)

**Configuration Hierarchy:**

```
~/.atlas/config.yaml          # User defaults
./.atlas/config.yaml          # Project overrides
ATLAS_* environment vars      # Runtime overrides
CLI flags                     # Immediate overrides
```

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas config get <key>` | Get configuration value |
| `atlas config set <key> <value>` | Set configuration value |
| `atlas config list` | List all configuration |
| `atlas config validate` | Validate configuration files |

Example:
```bash
atlas config get sdd.default --output json
atlas config set model.default "claude-3-opus" --scope project
```

---

### 2. Project & Task Engine

**Purpose:** The core workflow engine that models, tracks, and executes work.

#### Projects

Projects are encapsulations of larger bodies of work intended to be completed. They:

- Contain one or more tasks
- Have defined start/end states
- Maintain aggregate status
- Link to external project management entities

#### Tasks

Tasks are the atomic units of work. Every task follows a consistent protocol:

```
┌─────────────────────────────────────────────────────────────────┐
│                         TASK PROTOCOL                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  INPUT                          OUTPUT                          │
│  ─────                          ──────                          │
│  • Context (code, docs)         • Artifacts (code, text, file)  │
│  • Previous task results        • New tasks (spawned)           │
│  • Task chain position          • Status transition             │
│                                                                 │
│  EXECUTION                                                      │
│  ─────────                                                      │
│  • Task type (AI / Human)                                       │
│  • Assigned model/agent                                         │
│  • API key context                                              │
│                                                                 │
│  VALIDATION                                                     │
│  ──────────                                                     │
│  • Must-pass requirements                                       │
│  • Quality gates                                                │
│  • Test execution                                               │
│                                                                 │
│  GIT CONTEXT (if applicable)                                    │
│  ───────────                                                    │
│  • Repository (owner/name)                                      │
│  • Source branch                                                │
│  • Target branch                                                │
│                                                                 │
│  OBSERVABILITY                                                  │
│  ─────────────                                                  │
│  • Structured logs (JSON)                                       │
│  • Agent output (debug/history)                                 │
│  • Status transitions                                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Example Task JSON:**

```json
{
  "id": "task-550e8400-e29b-41d4",
  "type": "ai",
  "status": "pending",
  "template": "bugfix",
  "input": {
    "context": {
      "files": ["src/main.go", "src/handler.go"],
      "issue_description": "API returns 500 on empty input"
    },
    "previous_results": []
  },
  "output": null,
  "execution": {
    "model": "claude-3-opus",
    "agent": "claude-code",
    "timeout_ms": 300000
  },
  "validation": {
    "must_pass": ["lint", "test"],
    "quality_gates": ["coverage > 80%"]
  },
  "git": {
    "repo": "owner/project",
    "source_branch": "main",
    "target_branch": "fix/empty-input-500"
  },
  "links": {
    "depends_on": [],
    "blocks": ["task-review-123"]
  }
}
```

#### Task Types

| Type | Executor | Characteristics |
|------|----------|-----------------|
| AI Task | AI Agent | Autonomous execution within constraints |
| Human Task | Team Member | Requires human decision/action |
| Validation Task | Automated | Runs tests, linters, checks |
| Git Task | Git Agent | Branch, commit, PR operations |

#### Task Linking

- **Related:** Informational connection
- **Dependent:** Must complete before this task can start
- **Blocking:** This task's completion unblocks others

#### Context Strategy

AI agents have limited context windows. ATLAS uses explicit context targeting to ensure agents receive relevant information.

**Context Selection in Task Definition:**

```json
{
  "input": {
    "context": {
      "files": ["src/main.go", "src/handler.go"],
      "patterns": ["src/**/*_test.go"],
      "include_tests": true,
      "memory_query": "recent decisions about error handling"
    }
  }
}
```

**Context Fields:**
| Field | Purpose |
|-------|---------|
| `files` | Explicit file paths to include |
| `patterns` | Glob patterns for file discovery |
| `include_tests` | Include corresponding test files |
| `memory_query` | Semantic search against project memory |

**Progressive Context:**
- Start with specified files + recent memory
- Agent can request additional context during execution
- Track what context was actually used (for learning)

**MVP Approach:** Explicit file targeting. Agents work within provided context. Future versions may support dynamic context expansion.

#### Multi-Repo Task Coordination

For work spanning multiple repositories, tasks are decomposed into atomic per-repo units:

```
Project: "Add feature X across services"
├── Task 1: Update API contract (repo: api-contracts) [must complete first]
├── Task 2: Implement in service-a (repo: service-a) [depends on Task 1]
├── Task 3: Implement in service-b (repo: service-b) [depends on Task 1]
└── Task 4: Integration test (repo: e2e-tests) [depends on Tasks 2, 3]
```

**Coordination Rules:**
- Each task executes in its own repo context
- Dependencies enforce ordering (sequential) or allow parallelism
- Cross-repo context passed via task artifacts, not shared state
- Template defines the coordination pattern

#### Templates

Pre-built task configurations and chains for common workflows:

- Bug Fix Template
- Feature Implementation Template
- Code Review Template
- Refactoring Template
- Documentation Template

#### Validation Strategy

Code quality is enforced through layered validation gates.

**Internal Validations (Run Locally Before Commit):**

| Check | Tool | Behavior |
|-------|------|----------|
| Lint | `golangci-lint run` | Must pass |
| Unit Tests | `go test ./...` | Must pass |
| Pre-commit | Project hooks | Must pass |
| Type Check | `go build` | Must compile |

**External Validations (After PR Creation):**

| Check | Source | Behavior |
|-------|--------|----------|
| CI Pipeline | GitHub Actions | Monitor via `gh run list` |
| Required Checks | Branch protection | Wait for completion |
| Security Scan | CodeQL, etc. | Report findings |

**Validation Workflow:**

```
Code Generated
    │
    ▼
┌─────────────────────┐
│ Internal Validation │◄──── Auto-retry with fixes
│ (lint, test, build) │      (up to N attempts)
└─────────┬───────────┘
          │ Pass
          ▼
┌─────────────────────┐
│ Commit & Push       │
│ (with ATLAS trailers│
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ Open PR             │
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ External Validation │◄──── Wait for GitHub Actions
│ (CI, security, etc.)│
└─────────┬───────────┘
          │ Pass
          ▼
┌─────────────────────┐
│ Ready for Review    │
│ (Human checkpoint)  │
└─────────────────────┘
```

**CLI Integration:**

```bash
# Check GitHub Actions status
gh run list --branch fix/my-branch --json status,conclusion

# Wait for checks to complete
gh pr checks <pr-number> --watch
```

**Temporal Integration:**

The Project & Task Engine leverages Temporal for:

- Durable execution guarantees
- Long-running workflow management
- Retry and timeout handling
- Workflow versioning
- Activity coordination

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas task create` | Create a new task |
| `atlas task list` | List tasks with filters |
| `atlas task status <id>` | Get task status |
| `atlas task run <id>` | Execute a task |
| `atlas project create` | Create a new project |
| `atlas project list` | List projects |
| `atlas project status <id>` | Get project status |

Example:
```bash
atlas task create --template bugfix --input task.json --output json
atlas task list --status pending --project my-project
atlas project status proj-123 --include-tasks
```

---

### 3. Project Adapters

**Purpose:** Bridge external project management tools with ATLAS internal task models.

**Adapter Interface Contract:**

```
ProjectAdapter:
  - SyncProject(externalID) → InternalProject
  - SyncTask(externalID) → InternalTask
  - PushStatus(internalTask, status) → void
  - PushComment(internalTask, comment) → void
  - WatchForChanges(callback) → subscription
```

**Planned Adapters:**

| Adapter | Status | Notes |
|---------|--------|-------|
| GitHub Projects | MVP | Native Git integration |
| Asana | MVP | Primary PM tool |
| Linear | Post-MVP | Developer-focused PM |
| Self-Hosted | Post-MVP | File-based, Git-native |

**Synchronization Model:**

- ATLAS maintains internal canonical state
- Adapters sync bidirectionally
- External tools reflect ATLAS status
- Conflict resolution favors most recent change

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas project sync` | Sync with external PM tool |
| `atlas project link <external-id>` | Link external project to ATLAS |
| `atlas project unlink <id>` | Remove external linkage |
| `atlas project adapters` | List available adapters |

Example:
```bash
atlas project sync --adapter github --repo owner/name
atlas project link --adapter asana --external-id 12345 --project my-project
```

---

### 4. SDD Adapter System

**Purpose:** Abstract Spec-Driven Development frameworks to allow flexible adoption without coupling to specific implementations.

**Why Abstraction:**

- SDD landscape is rapidly evolving
- No single framework dominates
- Teams have existing investments
- ATLAS should be framework-agnostic

**Core SDD Capabilities:**

```go
type SDDAdapter interface {
    // Initialize sets up the framework in a project
    Initialize(projectPath string) error

    // GenerateSpec creates a specification from requirements
    GenerateSpec(requirements string) (Specification, error)

    // CreatePlan generates an implementation plan from spec
    CreatePlan(spec Specification) (Plan, error)

    // BreakdownTasks converts plan to executable task list
    BreakdownTasks(plan Plan) ([]Task, error)

    // Validate checks implementation against specification
    Validate(spec Specification, code []File) (ValidationResult, error)
}
```

#### Unified Phase Model

ATLAS abstracts different framework workflows into a common phase model:

```
ATLAS Phase    │ Speckit                │ BMAD Method
───────────────┼────────────────────────┼─────────────────────
Initialize     │ /speckit.constitution  │ *workflow-init
Specify        │ /speckit.specify       │ Planning phase
Plan           │ /speckit.plan          │ Solutioning phase
Tasks          │ /speckit.tasks         │ Story breakdown
Implement      │ /speckit.implement     │ Implementation phase
Validate       │ /speckit.checklist     │ QA agent
```

#### Framework Integration Model

Frameworks integrate as CLI tools installed in the runtime environment:

- **Speckit:** Python CLI (`specify`) installed via uv/pip
- **BMAD:** NPM package (`npx bmad-method`)

ATLAS invokes framework commands and captures structured output:

```bash
# ATLAS internally calls:
specify init .                           # Initialize
specify generate --input reqs.json       # Generate spec
specify plan --spec spec.md             # Create plan
```

#### Artifact Storage

All frameworks store artifacts in a normalized directory structure:

```
.atlas/
├── specs/
│   ├── specification.md    # Current spec (markdown)
│   ├── plan.md            # Implementation plan
│   └── tasks.json         # Machine-readable task list
├── memory/
│   └── ...
└── config.yaml
```

**Framework Categories:**

| Category | Examples | Use Case |
|----------|----------|----------|
| Full Lifecycle | Speckit, BMAD, OpenSpec | End-to-end spec management |
| Spec DSL | Kiro, TypeSpec, Arbiter | Formal specification languages |
| Code Generation | Stainless, Speakeasy | API-first code generation |
| Multi-Agent | Microsoft Amplifier, Shotgun | Agent coordination |
| Minimal | LeanSpec | Small context windows |

**Initial Priority Frameworks:**

1. **Speckit** - GitHub native, well-documented
2. **BMAD Method** - Comprehensive lifecycle
3. **OpenSpec** - Standard specification format
4. **Kiro** - AWS integration, formal specs
5. **Conductor** - Workflow-oriented

**SDD Selection Logic:**

```
Task Request
    │
    ▼
┌───────────────────┐
│ SDD Specified?    │──Yes──► Use Specified Framework
└───────────────────┘
    │ No
    ▼
┌───────────────────┐
│ Project Default?  │──Yes──► Use Project Framework
└───────────────────┘
    │ No
    ▼
┌───────────────────┐
│ Use App Default   │
└───────────────────┘
```

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas sdd generate` | Generate specification from requirements |
| `atlas sdd validate` | Validate a specification |
| `atlas sdd implement` | Generate code from specification |
| `atlas sdd frameworks` | List available SDD frameworks |

Example:
```bash
atlas sdd generate --input requirements.json --framework speckit --output json
atlas sdd validate --spec spec.json --framework bmad
atlas sdd implement --spec spec.json --context ./src
```

---

### 5. Memory Adapter System

**Purpose:** Provide durable, shared, and evolvable memory for the multi-agent ecosystem.

**Core Philosophy (from vision doc):**

> Design for the migrations you will actually make—not theoretical flexibility.

#### Guiding Principles

1. **Text Is the Source of Truth**
   - All memory representable as text
   - Embeddings are derived artifacts
   - Model upgrades are safe
   - Vendor lock-in avoided

2. **Capabilities, Not Backends**
   - Semantic search
   - Keyword search
   - Hybrid retrieval
   - Expiration and decay
   - Scoping and isolation

3. **Files Never Go Away**
   - Permanent escape hatch
   - Human inspection
   - Universal interchange
   - All backends export to files

4. **Memory Has Lifecycle**
   - Recency scoring
   - Importance weighting
   - Decay functions
   - Expiration handling
   - Consolidation

5. **Scope Is Identity**
   - User scope
   - Project scope
   - Session scope
   - Two identical contents in different scopes are different memories

#### Session & Memory Lifecycle

**Session Definition:** A session equals the lifecycle of a project (from creation to completion or abandonment). Sessions span multiple tasks but have defined boundaries.

**Memory Progression:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MEMORY LIFECYCLE                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TASK-SCOPED                    SESSION/PROJECT-SCOPED                      │
│  ────────────                   ──────────────────────                      │
│  • Working memory               • Decisions made                            │
│  • Intermediate state           • Feedback received                         │
│  • Discarded after task         • Context accumulated                       │
│                                 • Lives for project duration                │
│           │                                    │                            │
│           ▼                                    ▼                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     PROJECT COMPLETION                               │    │
│  │                 Consolidation & Promotion                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                        │
│           ┌────────────────────────┴────────────────────────┐               │
│           ▼                                                 ▼               │
│  REPO-SCOPED                                       USER-SCOPED              │
│  ───────────                                       ───────────              │
│  • Permanent learnings                             • Cross-repo patterns    │
│  • Transfer to future projects                     • Personal preferences   │
│  • Style guides, conventions                       • Common mistakes        │
│  • Historical decisions                            • Working style          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Consolidation Points:**
- **Project completion:** Promote key learnings to repo scope
- **Pattern detection:** Promote recurring insights to user scope
- **Explicit save:** User can manually promote important context

#### Canonical Memory Model

```
Memory Entry:
  ├── Identity & Scope
  │   ├── id: UUID
  │   ├── user_id: string
  │   ├── project_id: string?
  │   └── session_id: string?
  │
  ├── Content
  │   └── text: string (canonical)
  │
  ├── Classification
  │   ├── type: enum
  │   ├── tags: []string
  │   └── importance: float
  │
  ├── Temporality
  │   ├── created_at: timestamp
  │   ├── updated_at: timestamp
  │   ├── accessed_at: timestamp
  │   └── expires_at: timestamp?
  │
  ├── Provenance
  │   ├── source: enum
  │   ├── source_id: string?
  │   └── created_by: string
  │
  └── Relationships (optional)
      └── related_ids: []UUID
```

**Example Memory Entry JSON:**

```json
{
  "id": "mem-a1b2c3d4-e5f6-7890",
  "scope": {
    "user_id": "user-123",
    "project_id": "proj-atlas",
    "session_id": null
  },
  "content": {
    "text": "Decision: Use Temporal for workflow orchestration due to durable execution guarantees and native Go support."
  },
  "classification": {
    "type": "decision",
    "tags": ["architecture", "orchestration", "temporal"],
    "importance": 0.9
  },
  "temporality": {
    "created_at": "2025-12-15T10:30:00Z",
    "updated_at": "2025-12-15T10:30:00Z",
    "accessed_at": "2025-12-20T14:00:00Z",
    "expires_at": null
  },
  "provenance": {
    "source": "agent",
    "source_id": "task-design-001",
    "created_by": "orchestration-agent"
  },
  "relationships": {
    "related_ids": ["mem-temporal-research", "mem-workflow-requirements"]
  }
}
```

#### Evolutionary Architecture

| Phase | Focus | Capabilities |
|-------|-------|--------------|
| 1 | File-Based | Zero dependencies, canonical format, debugging |
| 2 | Semantic Search | Embeddings, hybrid queries, meaning-based retrieval |
| 3 | Scaled Backend | Performance optimization when needed |
| 4 | Relationships | Graph traversal if proven necessary |
| 5 | Specialized Routing | Multi-backend orchestration (rare) |

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas memory store` | Store a memory entry |
| `atlas memory query` | Query memories with filters |
| `atlas memory get <id>` | Get specific memory entry |
| `atlas memory export` | Export memories to file |
| `atlas memory import` | Import memories from file |

Example:
```bash
atlas memory store --type decision --scope project --input memory.json
atlas memory query --scope project --type decision --tags "architecture" --output json
atlas memory export --scope project --format ndjson > memories.ndjson
```

---

### 6. Communication Abstraction

**Purpose:** Enable agent-to-agent and agent-to-human communication through unified interfaces.

#### Communication Types

| Type | Description | Examples |
|------|-------------|----------|
| Agent → Agent | Inter-agent messaging | Task handoff, context sharing |
| Agent → Human | Request for input/decision | PR review, approval requests |
| Human → Agent | Commands and feedback | Acceptance, rejection, clarification |
| System → Human | Notifications | Status updates, alerts |

#### Channel Adapters

```
CommunicationAdapter:
  - SendMessage(recipient, message) → receipt
  - RequestInput(recipient, prompt, options) → response
  - Subscribe(patterns, callback) → subscription
  - GetHistory(thread) → []Message
```

**Planned Channels:**

- **GitHub** (MVP) - Issues, PR comments, discussions
- **Email** (MVP) - Async notifications
- **Discord** (Post-MVP) - Real-time team chat
- **Slack** (Post-MVP) - Enterprise integration

#### Human Task Resolution

When a Human Task is created:

1. Notification sent via configured channel(s)
2. Human responds through native interface
3. Response captured and routed back to task
4. Task proceeds based on response

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas notify send` | Send a notification |
| `atlas notify subscribe` | Subscribe to message patterns |
| `atlas notify history` | View message history |
| `atlas notify channels` | List available channels |

Example:
```bash
atlas notify send --channel github --recipient owner/repo --message msg.json
atlas notify history --thread task-123 --output json
atlas notify subscribe --pattern "task.*" --callback http://localhost:8080/webhook
```

---

### 7. Orchestration Agent

**Purpose:** The always-on coordinator that manages task execution and workflow progression.

#### Responsibilities

- Monitor task queue
- Spawn worker agents as needed
- Handle task dependencies
- Manage retries and failures
- Coordinate human touchpoints
- Maintain workflow state

#### Design Considerations

- Implemented with Temporal for durability
- Horizontally scalable (multiple orchestrators)
- Stateless between workflow invocations
- Event-driven rather than polling

#### Orchestration Loop

```
┌─────────────────────────────────────────────────────────────────┐
│                    ORCHESTRATION LOOP                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌─────────┐    ┌─────────────┐    ┌─────────────────┐         │
│   │ Pending │───►│ Ready Queue │───►│ Dispatch to     │         │
│   │ Tasks   │    │ (deps met)  │    │ Agent Runtime   │         │
│   └─────────┘    └─────────────┘    └────────┬────────┘         │
│                                              │                  │
│   ┌──────────────────────────────────────────┘                  │
│   │                                                             │
│   ▼                                                             │
│   ┌─────────────────┐    ┌─────────────────┐                    │
│   │ Execution       │───►│ Result          │                    │
│   │ (AI/Human)      │    │ Processing      │                    │
│   └─────────────────┘    └────────┬────────┘                    │
│                                   │                             │
│                                   ▼                             │
│   ┌─────────────────┐    ┌─────────────────┐                    │
│   │ Validation      │◄───│ Update Status   │                    │
│   │ Gates           │    │ & Dependencies  │                    │
│   └────────┬────────┘    └─────────────────┘                    │
│            │                                                    │
│            ▼                                                    │
│   ┌─────────────────┐                                           │
│   │ Next Task or    │                                           │
│   │ Project Complete│                                           │
│   └─────────────────┘                                           │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas workflow start` | Start a workflow |
| `atlas workflow status <id>` | Get workflow status |
| `atlas workflow cancel <id>` | Cancel a running workflow |
| `atlas workflow list` | List workflows |
| `atlas workflow logs <id>` | Stream workflow logs |

Example:
```bash
atlas workflow start --definition workflow.json --input context.json
atlas workflow status wf-123 --output json
atlas workflow logs wf-123 --follow --format ndjson
```

---

### 8. Agent Runtime Environment

**Purpose:** Isolated, consistent execution environment for AI agents and workers.

#### Container Specification

**Base Image Contents:**

- Go runtime (latest stable)
- Build tools (Mage-x for cross-platform builds)
- Git with full capabilities
- Network access (controlled egress)

**Dynamic Installation:**

- Agent CLIs (Claude Code, etc.) via Mage-x
- SDD frameworks as needed
- Project-specific dependencies

#### Runtime Modes

| Mode | Scope | Use Case |
|------|-------|----------|
| Task | Single task | Isolated, short-lived |
| Task Chain | Related tasks | Shared context, sequential |
| Project | Entire project | Long-lived, full context |

#### Communication Model

```
┌─────────────────┐         ┌─────────────────┐
│  Orchestration  │◄───────►│  Agent Runtime  │
│  Agent          │  gRPC/  │  Container      │
│                 │  HTTP   │                 │
└─────────────────┘         └─────────────────┘
```

#### Worker Implementation

- Pure Go implementation preferred
- Temporal workers for durability
- Go ADK for agent capabilities
- Genkit for AI integration patterns

**CLI Interface:**

| Command | Description |
|---------|-------------|
| `atlas agent run` | Run an agent in a container |
| `atlas agent logs <id>` | Stream agent logs |
| `atlas agent stop <id>` | Stop a running agent |
| `atlas agent list` | List running agents |
| `atlas agent exec <id>` | Execute command in agent container |

Example:
```bash
atlas agent run --config agent.json --task task-123 --output json
atlas agent logs agent-456 --follow --format ndjson
atlas agent exec agent-456 -- git status
```

---

### 9. Research Agent (Internal)

**Purpose:** Internal maintainer tooling that monitors the SDD and memory framework landscape. This is **not a user-facing feature**.

> **For Users:** You don't interact with the Research Agent. Simply run `atlas upgrade` to receive the latest framework integrations, adapter updates, and improvements. The Research Agent works behind the scenes to ensure ATLAS stays current.

The Research Agent enables ATLAS maintainers to:

- Track development of integrated SDD frameworks (Speckit, BMAD, etc.)
- Detect breaking changes before they impact users
- Identify new features worth adopting
- Discover emerging projects in the ecosystem
- Generate compatibility reports for releases

**How it benefits you:** When maintainers run the Research Agent, discoveries are reviewed, tested, and packaged into ATLAS releases. Users receive these updates seamlessly via `atlas upgrade`, with a changelog showing what changed.

**Full Documentation:** See [research-agent.md](../internal/research-agent.md) for maintainer documentation.

---

### 10. Ecosystem Updates

**Purpose:** Keep ATLAS and all integrations current with minimal user effort.

#### User Experience

```bash
atlas upgrade                 # Update everything
atlas upgrade --check         # See what would be updated (no changes)
atlas upgrade --component sdd # Update only SDD frameworks
```

**What happens when you run `atlas upgrade`:**

1. ATLAS checks for updates to core, adapters, and SDD frameworks
2. Downloads and installs compatible versions
3. Displays a clear changelog summary of what changed
4. Verifies installation integrity

#### Upgrade Output

Every upgrade shows exactly what changed in a scannable format:

```
Checking for updates...

┌─────────────────────────────────────────────────────────────────┐
│                     ATLAS UPGRADE SUMMARY                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  CORE                                                           │
│  ────                                                           │
│  atlas-cli: 1.2.0 → 1.3.0                                       │
│    • Added Linear adapter support                               │
│    • Improved task retry logic with exponential backoff         │
│    • Fixed memory leak in long-running workflows                │
│                                                                 │
│  SDD FRAMEWORKS                                                 │
│  ──────────────                                                 │
│  speckit: 2.1.0 → 2.2.0                                         │
│    • New validation rules for API specs                         │
│    • Performance improvements in large codebases                │
│  bmad: 1.5.0 (no update available)                              │
│                                                                 │
│  ADAPTERS                                                       │
│  ────────                                                       │
│  github-adapter: 1.1.0 → 1.1.1                                  │
│    • Bug fix: PR comments now preserve formatting               │
│  asana-adapter: 1.0.0 (no update available)                     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

Upgrade complete in 12.3s

Run `atlas changelog` for full release notes.
Run `atlas rollback` to revert to previous versions.
```

#### Additional Commands

| Command | Description |
|---------|-------------|
| `atlas upgrade` | Update all components to latest compatible versions |
| `atlas upgrade --check` | Preview available updates without installing |
| `atlas upgrade --component <name>` | Update specific component only |
| `atlas changelog` | View full release notes for current versions |
| `atlas rollback` | Revert to previous versions if issues arise |
| `atlas version` | Show current versions of all components |

#### Design Principles

- **Zero configuration:** Updates just work
- **Compatibility guaranteed:** Only compatible versions are installed together
- **Rollback support:** `atlas rollback` if issues arise
- **Offline resilience:** Cached versions work without network
- **Transparency:** Always shows exactly what changed and why

---

### 11. Learning & Feedback System

**Purpose:** Capture task outcomes and adapt behavior based on experience.

#### Core Principle

ATLAS improves by remembering what worked and what didn't. Every task outcome—accepted, rejected, or modified—becomes a learning opportunity.

#### Feedback Capture

When a task completes, capture:
- **Outcome:** accepted, rejected, or modified
- **Reason:** human-provided feedback (especially for rejections)
- **Context:** task type, files touched, template used
- **Metrics:** duration, token usage, retry count

**Example Outcome Memory Entry:**

```json
{
  "type": "task_outcome",
  "scope": {
    "user_id": "user-123",
    "repo": "owner/go-project"
  },
  "content": {
    "text": "PR rejected: Missing error handling for nil pointer in parseConfig function"
  },
  "classification": {
    "outcome": "rejected",
    "reason": "Missing error handling for edge case",
    "task_type": "feature",
    "files_touched": ["pkg/config/parser.go"],
    "importance": 0.8
  }
}
```

#### Learning Integration

Before executing similar tasks, ATLAS queries past outcomes:

1. **Negative Example Surfacing:** "Previously, similar changes were rejected because..."
2. **Pattern Recognition:** "This repo historically requires explicit error handling"
3. **Template Refinement:** Track which templates succeed in which contexts

#### MVP Implementation

Simple but effective:
- Store every task + outcome + feedback as memory entries
- Query past failures before similar task execution
- Include relevant warnings in agent context
- Track success rates per repo/template combination

#### Future Enhancements

- Automated template improvement based on patterns
- Cross-user learning (with consent) for common patterns
- Proactive suggestions based on historical failures

---

## Integration Patterns

This section demonstrates how ATLAS components compose together in real workflows.

### CLI Piping

Components can be chained via stdin/stdout using standard UNIX pipes:

```bash
# Generate spec from requirements, create task, start workflow
atlas sdd generate --input requirements.json | \
  atlas task create --template feature | \
  atlas workflow start --follow

# Query memories and feed into a task
atlas memory query --type context --scope project | \
  atlas task run --id task-123 --context-stdin

# Export workflow logs to memory for future reference
atlas workflow logs wf-456 --format ndjson | \
  atlas memory store --type execution-log --scope project
```

### Error Propagation

When piping commands, errors propagate through the chain:

```json
{
  "success": false,
  "error": {
    "code": "UPSTREAM_FAILURE",
    "message": "Previous command in pipeline failed",
    "details": {
      "upstream_command": "atlas sdd generate",
      "upstream_error": "VALIDATION_FAILED"
    }
  }
}
```

Each component can either handle errors or forward them. The final exit code reflects the overall pipeline success.

### Correlation Tracking

Pass correlation IDs through pipelines for end-to-end tracing:

```bash
CORRELATION_ID=$(uuidgen)

atlas sdd generate --input reqs.json --correlation-id $CORRELATION_ID | \
  atlas task create --correlation-id $CORRELATION_ID | \
  atlas workflow start --correlation-id $CORRELATION_ID

# All logs from all components will include the same correlation_id
atlas logs search --correlation-id $CORRELATION_ID
```

### Batch Processing

Process multiple items using NDJSON streams:

```bash
# Process multiple tasks in sequence
cat tasks.ndjson | while read task; do
  echo "$task" | atlas task run --input-stdin
done

# Or use xargs for parallel processing
cat task-ids.txt | xargs -P4 -I{} atlas task run --id {}
```

### Webhook Integration

Components can notify external systems:

```bash
# Start workflow with webhook callback
atlas workflow start --definition wf.json \
  --on-complete "https://api.example.com/webhook" \
  --on-failure "https://api.example.com/alert"
```

---

## MVP Definition

### Strategic Goals

The MVP must prove the architecture, not be a toy.

**MVP Proves:**

- Task execution works end-to-end
- Human touchpoints function correctly
- Code delivery via Git is reliable
- Memory provides value across sessions
- SDD abstraction enables flexibility
- Code is actually useful, validated and passes lint/tests

### MVP Scope

#### Included

| Component | MVP Capability |
|-----------|----------------|
| Project Adapters | GitHub Projects (read/write) |
| Task Engine | Full task protocol, templates |
| SDD Adapters | Speckit, BMAD (two minimum) |
| Memory | File-based with semantic search |
| Communication | GitHub Issues/PR comments |
| Orchestration | Single-node Temporal |
| Runtime | Local Docker execution |
| Configuration | YAML-based, env overrides |

#### Explicitly Excluded

- Web GUI
- Multi-node deployment
- Enterprise SSO
- Advanced analytics
- Team collaboration features
- Multiple PM tool adapters (beyond GitHub)

### MVP Workflow Example

**The First Real Task: Contributing to an Open-Source Go Project**

This is the concrete workflow ATLAS must execute end-to-end for MVP validation:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        FEATURE IMPLEMENTATION WORKFLOW                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  USER INPUT                                                                 │
│  ──────────                                                                 │
│  • Repo URL: github.com/owner/go-project                                   │
│  • Feature requirements (natural language or issue link)                   │
│  • Target files/packages (optional hints)                                  │
│                                                                             │
│  TASK FLOW                                                                  │
│  ─────────                                                                  │
│                                                                             │
│  ┌──────────────────┐                                                       │
│  │ Task 1: Specify  │ (AI) Generate spec using SDD framework               │
│  │                  │ Output: specification.md                             │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 2: Review   │ (Human) Approve or refine spec                       │
│  │                  │ Output: approved spec or feedback                    │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 3: Plan     │ (AI) Create implementation plan                      │
│  │                  │ Output: tasks.md with checklist                      │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 4: Implement│ (AI) Write code following spec                       │
│  │                  │ Output: code changes                                 │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 5: Validate │ (Auto) Run lint + tests + pre-commit                 │
│  │                  │ Auto-proceeds if passing                             │
│  └────────┬─────────┘                                                       │
│           │ ◄─── Auto-retry with fixes if failing (up to N attempts)       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 6: Commit   │ (AI) Create branch, commit with trailers, push       │
│  │                  │ Output: branch on remote                             │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 7: PR       │ (AI) Open pull request with description              │
│  │                  │ Output: PR URL                                       │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 8: CI Wait  │ (Auto) Monitor GitHub Actions via `gh` CLI           │
│  │                  │ Auto-proceeds when checks pass                       │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 9: Review   │ (Human) Review PR, request changes or approve        │
│  │                  │ Output: approval or feedback                         │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 10: Merge   │ (Auto) Merge PR on approval                          │
│  │                  │ Output: merged commit                                │
│  └────────┬─────────┘                                                       │
│           ▼                                                                 │
│  ┌──────────────────┐                                                       │
│  │ Task 11: Learn   │ (Auto) Store outcome + feedback in memory            │
│  │                  │ Future tasks learn from this experience              │
│  └──────────────────┘                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Parallel Workstreams:**

Users can have 2+ projects running simultaneously:
- Each project operates independently
- Share repo-level and user-level memory
- ATLAS handles context switching between workstreams
- Status command shows all active projects and pending approvals

**Validation Integration:**

Internal validations (run locally before PR):
- `golangci-lint run`
- `go test ./...`
- Pre-commit hooks

External validations (after PR creation):
- GitHub Actions status via `gh run list --branch <branch>`
- Required status checks

---

## Failure Modes & Mitigation

Understanding how ATLAS fails helps prioritize what must work.

### Why People Stop Using ATLAS

| Failure Mode | Symptoms | Impact |
|--------------|----------|--------|
| Too complicated to set up | Abandonment during onboarding | Never starts |
| Unclear what's happening | Confusion, distrust, manual intervention | Partial adoption |
| Output quality not good enough | Rejected PRs, wasted review time | Negative ROI |
| Too slow | Context switching, lost momentum | Workflow disruption |
| Breaks existing workflow | Merge conflicts, CI failures | Active harm |

### Mitigations

| Failure Mode | Mitigation Strategy |
|--------------|---------------------|
| Too complicated | One-command setup (`atlas init`), sensible defaults, zero config for happy path |
| Unclear what's happening | Clear status output, structured logs, real-time progress visibility |
| Output quality | Strong validation gates, learning from rejection, quality templates |
| Too slow | Parallel task execution, efficient context management, warm containers |
| Breaks workflow | Additive integration, works with existing Git workflow, no forced changes |

### Design Implications

Every feature must answer: **Does this make ATLAS easier to adopt and harder to abandon?**

- Prefer invisible automation over manual configuration
- Surface problems immediately, not after wasted work
- Make the right thing the easy thing
- When in doubt, do less but do it well

---

## Open Research Areas

### Questions Requiring Investigation

1. **Container Granularity**
   - Per-task vs per-chain vs per-project containers?
   - Warm container pools for performance?

2. **Temporal Patterns**
   - Optimal workflow decomposition?
   - Activity timeout strategies?
   - Versioning strategy for long-running workflows?

3. **SDD Framework Evaluation**
   - Quantitative comparison criteria?
   - Framework combination strategies?

4. **Memory Retrieval Optimization**
   - Hybrid search weighting?
   - Context window management strategies?

5. **Human Task UX**
   - Optimal notification strategies?
   - Response timeout handling?

### Research Agent Priorities

| Priority | Framework/Project | Reason |
|----------|------------------|--------|
| High | Speckit | Primary SDD candidate |
| High | BMAD | Alternative SDD approach |
| High | Temporal | Core workflow engine |
| Medium | Go ADK | Agent implementation |
| Medium | Memory frameworks | Retrieval patterns |

---

## Decision Framework

### Architecture Decision Criteria

Any addition to ATLAS must answer:

1. **Compatibility** - Does it work with existing interfaces?
2. **Recoverability** - Can state be exported and reimported?
3. **Text Truth** - Does it preserve text as source of truth?
4. **Real Value** - Does it add concrete capability, not theoretical flexibility?
5. **Operational Cost** - Does value justify complexity?

### Technology Selection Criteria

When choosing implementations:

1. **Go First** - Prefer pure Go solutions
2. **Temporal Native** - Leverage Temporal patterns
3. **File Fallback** - Always have file-based alternative
4. **Interface Stable** - Internal interfaces before external dependencies

### Complexity Budget

Before adding any feature, ask:

- Is this needed for MVP?
- What's the simplest implementation?
- Can it be added later without breaking changes?

If uncertain, defer.

---

## Appendix: SDD Framework Landscape

### Tier 1: Primary Candidates (Implement in MVP)

| Framework | Description | Strength |
|-----------|-------------|----------|
| Speckit | GitHub's spec-driven development | Git-native, well-documented |
| BMAD | Business-Model-Application-Data method | Comprehensive lifecycle |

### Tier 2: Secondary Candidates (Post-MVP)

| Framework | Description | Strength |
|-----------|-------------|----------|
| OpenSpec | Standard specification format | Interoperability |
| Kiro | AWS integration for specs | Cloud-native |
| Conductor | Workflow-oriented specs | Process focus |

### Tier 3: Research & Evaluate

| Framework | Description | Notes |
|-----------|-------------|-------|
| cc-sdd | Kiro-style with MCP | MCP integration |
| Shotgun CLI | Multi-agent with tree-sitter | Code-aware |
| Claude CodePro | TDD-enforcing setup | Testing focus |
| LeanSpec | Minimal spec approach | Small contexts |
| Microsoft Amplifier | Research multi-agent | Academic insights |
| TypeSpec | Microsoft API DSL | API-first |

### Tier 4: Commercial/Specialized (Watch)

| Platform | Description | Notes |
|----------|-------------|-------|
| Specific.dev | Spec-as-source | Fully generated code |
| Speakeasy | OpenAPI to SDK | API generation |
| Stainless | AI-first SDK generation | Production quality |

---

## Document History

| Version | Date | Changes |
|---------|------|---------|
| 0.1.0 | December 2025 | Initial vision document |
| 0.2.0 | December 2025 | Added: Trust & Autonomy Ladder, Identity & Commit Model, UX Philosophy, Learning & Feedback System, Metrics & Observability, Context Strategy, Multi-Repo Task Coordination, Validation Strategy, Session & Memory Lifecycle, Failure Modes & Mitigation. Expanded: SDD Framework Integration with unified phase model, MVP Workflow with concrete Go project example. |
| 0.3.0 | December 2025 | Clarified Research Agent as internal maintainer tooling (not user-facing). Added Ecosystem Updates section with `atlas upgrade` command and detailed changelog UX. Research Agent detailed documentation moved to separate maintainer docs. Renumbered Learning & Feedback System to §11. |

---

*This document is a living artifact. Update as the project evolves.*