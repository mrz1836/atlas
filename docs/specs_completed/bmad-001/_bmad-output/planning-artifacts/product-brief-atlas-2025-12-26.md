---
stepsCompleted: [1, 2, 3, 4, 5]
inputDocuments:
  - docs/external/vision.md
  - docs/external/templates.md
  - docs/internal/research-agent.md
  - .github/AGENTS.md
  - .github/tech-conventions/go-essentials.md
  - .github/tech-conventions/testing-standards.md
date: 2025-12-26
author: MrZ
---

# Product Brief: atlas

## Executive Summary

ATLAS (AI Task Lifecycle Automation System) is a CLI tool that transforms how Go developers work by orchestrating AI-assisted development workflows. It bridges the gap between powerful AI coding assistants and the structured, repeatable workflows developers need to multiply their output without sacrificing quality.

The core insight: developers today have access to remarkably capable AI agents, but lack the orchestration layer to run them systematically across multiple tasks in parallel. ATLAS provides that layer — accepting light task descriptions, expanding them through specification-driven development workflows, executing validation and git operations automatically, and interrupting the developer only when human judgment is required.

Built for Go developers who work on CLI tools, libraries, and backend services, ATLAS targets a 2-3x productivity multiplier by moving developers from "doing the work" to "reviewing the work."

---

## Core Vision

### Problem Statement

Go developers face a frustrating paradox: AI coding assistants have become remarkably capable, yet daily output hasn't increased proportionally. The bottleneck isn't the AI's ability to write code — it's the cognitive overhead and mechanical friction of orchestrating that AI across the full development lifecycle.

Every bug fix and feature follows predictable steps: analyze, implement, lint, test, commit, push, create PR, wait for CI. Yet developers manually shepherd each task through these steps, context-switch between Claude Code sessions, remember SDD framework commands, run validation tools, and actively monitor progress instead of being notified when input is needed.

### Problem Impact

- **Underutilized potential**: Developers capable of overseeing 3-5 parallel tasks are stuck executing 1 at a time
- **Cognitive drain**: Mental energy spent on process mechanics rather than architectural decisions
- **SDD friction**: Specification-driven development frameworks are powerful but require learning and remembering their workflows
- **Active monitoring burden**: Developers check on tasks rather than being notified when decisions are needed
- **Repetitive ceremony**: The same lint → test → commit → push → PR flow executed manually, repeatedly

### Why Existing Solutions Fall Short

| Solution | Gap |
|----------|-----|
| **Claude Code** | Powerful but stateless — no workflow memory, no parallel orchestration, no automatic validation |
| **Cursor/Copilot** | IDE-bound, single-task focus, no structured SDD workflow |
| **Speckit/SDD Frameworks** | Require developers to learn and remember the steps — no abstraction layer |
| **CI/CD Systems** | Post-commit automation only — don't help with the development workflow itself |

No existing tool combines: template-driven workflow expansion + SDD framework abstraction + parallel workspace management + configurable human checkpoints + notification-based progress tracking.

### Proposed Solution

ATLAS orchestrates the full task lifecycle:

1. **Light input** → Developer provides a brief description ("fix null pointer in config parser")
2. **Template expansion** → ATLAS expands this into a structured specification using configurable templates
3. **SDD workflow** → Automated progression through specify → plan → implement → validate
4. **Parallel execution** → Multiple tasks run simultaneously in isolated Git worktrees
5. **Notification-based checkpoints** → Developer is pinged when decisions are needed, not polling for status
6. **Configurable trust** → Interrupt points adjustable per task (more oversight early, less as trust builds)
7. **PR-ready output** → Validated, committed, pushed, PR created with AI-generated description

The developer stays in "planning mode" — creating specifications and reviewing completed work — while ATLAS handles execution.

### Key Differentiators

| Differentiator | Why It Matters |
|----------------|----------------|
| **Template-to-spec pipeline** | Light input becomes structured specification automatically |
| **SDD abstraction** | Developer doesn't need to learn Speckit commands — ATLAS knows the workflow |
| **Parallel workspace orchestration** | Git worktrees managed automatically; unified status across all tasks |
| **Configurable trust dial** | Start with frequent checkpoints, reduce as confidence grows |
| **Notification-based flow** | Work runs in background; developer notified only when needed |
| **Go-first design** | Optimized for Go development patterns, tooling, and idioms |

---

## Target Users

### Primary Users

#### The Multi-Repo Maintainer

**Persona: MrZ** — Senior Go developer who maintains 50+ repositories across personal projects, professional work, and contract engagements. Repositories span CLI tools, SDKs, libraries, and backend services, all following consistent Go conventions.

**Context:**
- Personal: 30+ open source Go repos at `github.com/mrz1836`
- Professional: Skyetel (REACH - serverless rating system)
- Contract: BSV Association, managing 20+ repos at `github.com/bsv-blockchain`

**Current Reality:**
- Same workflows repeated across dozens of repos: dependency updates, test coverage improvements, code quality fixes, security patches
- Built custom tooling (go-broadcast) to auto-review Dependabot PRs when status checks pass — but still manually invokes commands
- Spends time being the "mechanical turk in the middle" — executing predictable steps instead of architecting solutions
- Uses Claude Code effectively but lacks workflow orchestration
- Has experimented with Speckit, sees value but wants abstraction layer

**Pain Points:**
- Process multiplication: The same lint → test → commit → PR ceremony, times 50 repos
- Context switching: Jumping between repos loses momentum
- Manual monitoring: Watching CI instead of being notified
- Underutilized capacity: Could oversee 3-5 parallel tasks but stuck on 1 at a time

**Success Vision:**
- Multiple PRs waiting for review each day — meaningful descriptions, passing CI, updated docs, following conventions
- Staying in "creative mode" — planning features, reviewing work, making architectural decisions
- 2-3x output increase with maintained quality
- Minimal AI code review issues because conventions are enforced upfront

**Adoption Posture:** Ready to try on real repos immediately. "Everything is version controlled and it's only going to make a PR — not dangerous."

---

### Secondary Users

#### The Scale Developer

**Persona: Dylan** — Go developer working on massive, complex Go projects (e.g., Teranode — a multi-service distributed system). Deals with large codebases that sometimes have legacy patterns and are difficult to run locally.

**Context:**
- Works on enterprise-scale Go systems (Kubernetes-complexity)
- Codebases may have accumulated technical debt
- Multi-service architectures require careful coordination

**Current Reality:**
- Uses Claude but no workflow automation layer
- Same friction as MrZ: lint, test, pre-commit, CI watching, AI code reviews
- Complex systems mean more validation steps, longer CI runs

**Different Needs:**
- May need higher trust settings initially (more checkpoints until patterns proven)
- Different templates for multi-service changes
- More conservative approach given system complexity

**Why Secondary:** ATLAS v1 optimizes for the majority case (library maintainers, multiple standard repos) rather than the minority (massive multi-service systems). Dylan benefits, but isn't the design center.

---

#### Future Consideration: The Contract Developer

Developers juggling multiple client projects across organizations. Similar to the Multi-Repo Maintainer archetype but with additional context-switching between codebases with different conventions. Deferred for post-MVP consideration.

---

### User Journey

#### Discovery
Low-key, organic. Word of mouth among Go developers who maintain multiple repos. Not seeking broad awareness initially — "for me and my homies." Professional quality, personal scope.

#### First Use
**Day 1:** Install via `go install`, run `atlas init`. Try on a real repo immediately — no toy project phase. "It's only making PRs, nothing dangerous."

**First Task:** A concrete issue — "fix this null pointer" or "add test coverage for this package." Light input, watch the workflow expand it into a spec, see implementation happen, review the PR.

#### Aha Moment
Opening GitHub to find 3 PRs waiting:
- Meaningful descriptions that explain the *why*
- All status checks passing
- Documentation updated inline
- Code follows project conventions
- Few or zero AI code review issues
- Ready to merge with confidence

"I didn't babysit any of this."

#### Daily Usage
- Morning: Queue up tasks across repos with light descriptions
- Throughout day: Notifications when decisions needed (spec approval, PR review)
- End of day: Review and merge completed PRs
- Mental mode: Architect and reviewer, not executor

#### Long-term Value
Managing 50+ repos becomes sustainable. Backlog stays empty. Output multiplies. Creative energy goes to planning and design, not process mechanics. The "mechanical turk in the middle" role is eliminated.

---

## Success Metrics

### User Success Metrics

#### Output Multiplier (2-3x Target)

| Metric | Baseline (Today) | Target (With ATLAS) | How to Measure |
|--------|------------------|---------------------|----------------|
| **Substantial PRs merged/week** | ~2-3 | 6-9 | GitHub API: PRs merged, filtered by label or size |
| **Features shipped/month** | Variable | 2-3x current | Count of non-trivial PRs (exclude deps, minor fixes) |
| **Repos actively developed/week** | 1-2 at a time | 3-5 parallel | Count of repos with ATLAS tasks running |
| **Lines of meaningful code/week** | Baseline TBD | 2-3x | Git stats (excluding generated code) |

*"Substantial" = features, significant refactors, meaningful test coverage additions — not Dependabot merges or typo fixes.*

#### Quality Maintenance

| Metric | Target | How to Measure |
|--------|--------|----------------|
| **CI pass rate on first attempt** | >90% | GitHub Actions: first run pass/fail per PR |
| **AI code review severity** | No critical/high issues | Codex/CodeRabbit reports per PR |
| **PR revision rate** | <20% need rework | PRs requiring force-push after initial review |
| **Post-merge issues** | Near zero | Bugs reported within 7 days of merge |

*Quality should not degrade in pursuit of velocity. If CI pass rate drops or code review issues increase, slow down.*

#### Time Reclamation

| Metric | Baseline | Target | How to Measure |
|--------|----------|--------|----------------|
| **Mechanical vs. creative time** | 50-75% mechanical | <25% mechanical | Self-reported weekly check-in |
| **Time for planning sessions** | "Never enough" | Regular planning blocks | Calendar: planning time scheduled/completed |
| **Active monitoring time** | High (watching CI/AI) | Low (notification-based) | Subjective: polling frequency |

*The goal is role transformation: from executor to orchestrator.*

---

### Business Objectives

#### Adoption & Dependency (Personal Project Lens)

| Signal | What It Means | Target |
|--------|---------------|--------|
| **Daily use** | ATLAS is part of the workflow | Using ATLAS most days |
| **Multi-repo adoption** | Works across repo types | Active in 10+ repos |
| **Burnout reduction** | Sustainable pace | Self-reported: feeling less drained |
| **"Mage-X clutch" status** | As essential as core tooling | Can't imagine working without it |

*Success = ATLAS becomes as indispensable as `mage-x` for Go builds.*

#### Role Transformation

| From | To | Indicator |
|------|-----|-----------|
| Developer | Product Manager | Spending time on specs, not implementations |
| Watcher | Reviewer | Notified when PRs ready, not monitoring progress |
| Executor | Orchestrator | Queuing tasks, not running commands |
| 1 task at a time | Parallel oversight | 3-5 workspaces active simultaneously |

*Ultimate success: "I build more features and projects in the same time."*

---

### Key Performance Indicators (MVP Phase)

| KPI | Target | Timeframe |
|-----|--------|-----------|
| **PRs merged via ATLAS** | 10+ | First month |
| **CI first-pass rate** | >85% | First month |
| **Parallel workspaces used** | 3+ simultaneously | Within 2 weeks |
| **Time in planning mode** | >50% of dev time | Within 1 month |
| **Repos with ATLAS active** | 10+ | Within 2 months |
| **Dylan successfully onboarded** | Yes | Within 1 month |

---

## MVP Scope

### Core Features

#### Phase 0: Steel Thread (Week 1)
*Prove the architecture works end-to-end*

| Component | What It Does | Success Criteria |
|-----------|--------------|------------------|
| `atlas init` | Setup wizard: AI provider, GitHub auth, tool detection | Config file created, dependencies verified |
| `atlas start` | Start task with description + template | Worktree created, task JSON written, AI invoked |
| `atlas status` | Show all workspaces and task states | Accurate real-time status across workspaces |
| `atlas approve` | Approve pending work | Task completes, PR created |
| `atlas reject` | Reject with feedback, retry or abandon | Feedback captured, retry works |
| **`bugfix` template** | analyze → implement → validate → commit → push → PR | End-to-end flow works |
| **Git worktrees** | Parallel workspace isolation | 3+ worktrees running simultaneously |
| **Task engine** | State machine, JSON persistence | State transitions work, logs captured |

**Steel Thread Goal:** Run 3 parallel bugfix tasks to completion with PRs created.

---

#### Phase 1: SDD Integration (Week 2)
*Add specification-driven development workflow*

| Component | What It Does | Success Criteria |
|-----------|--------------|------------------|
| **`feature` template** | specify → review_spec → plan → tasks → implement → validate → PR | Full Speckit workflow works |
| **Speckit integration** | Invoke `/speckit.*` commands via Claude | Specs generated, plans created |
| **Human checkpoints** | Pause for spec approval, configurable trust | User can review/approve specs |
| **Template expansion** | Light input → structured specification | "fix auth bug" becomes proper spec |

**Phase 1 Goal:** Complete a feature using SDD workflow from light description to merged PR.

---

#### Phase 2: Full MVP (Week 3)
*Complete the minimum viable product*

| Component | What It Does | Success Criteria |
|-----------|--------------|------------------|
| `atlas workspace` | list, retire, destroy, logs | Full workspace lifecycle management |
| `atlas upgrade` | Self-upgrade + managed tools (mage-x, go-pre-commit, Speckit) | Clean upgrade experience |
| **`commit` template** | Smart commits: garbage detection, logical grouping | Multi-commit with good messages |
| **CI waiting** | Poll GitHub Actions, pause on failure | Tasks wait for CI, notify on completion |
| **Notification flow** | Terminal bell, status --watch | Know when action needed without polling |
| **Utility templates** | format, lint, test, validate | Quick single-purpose workflows |

**MVP Goal:** ATLAS is daily-driver ready for multi-repo maintenance.

---

### Out of Scope for MVP

| Feature | Why Deferred | Revisit When |
|---------|--------------|--------------|
| **Research Agent** | Manual monitoring is fine for 1 SDD framework | Tracking 5+ frameworks |
| **`atlas config`** | Manual config files are sufficient | User feedback indicates need |
| **`atlas resume`** | If task dies, re-run (worktree preserved) | Complex failure scenarios arise |
| **Multi-repo orchestration** | Single-repo focus simplifies MVP | Enterprise users require |
| **Other languages** | Go-first validates the concept | Go version is stable |
| **Token/cost tracking** | Timeout per task (30m) is sufficient guard | Budget concerns arise |
| **Trust levels** | Need rejection data first | 100+ task completions |
| **UI dashboard** | CLI is sufficient for power users | User feedback indicates need |
| **Learn/Rules Update** | Core workflow must be solid first | v1 is stable and useful |
| **Custom user templates** | Built-in templates cover primary use cases | Template patterns proven |
| **`pr-update` template** | Update existing PRs | PR workflow refinement needed |
| **`refactor` template** | Large refactoring with validation steps | Core templates proven |
| **`test-coverage` template** | Analyze gaps, implement tests | Test workflow patterns established |

---

### MVP Success Criteria

#### Phase 0 Exit Criteria (End of Week 1)
- [ ] `atlas init` completes successfully on a real repo
- [ ] 3 parallel bugfix tasks run to completion
- [ ] PRs created with meaningful descriptions
- [ ] CI passes on first attempt (>85%)
- [ ] Worktrees clean up properly

#### Phase 1 Exit Criteria (End of Week 2)
- [ ] Feature template completes full SDD workflow
- [ ] Speckit commands invoke correctly
- [ ] Spec approval checkpoint works
- [ ] Light input expands to structured spec
- [ ] Dylan can run a successful task

#### MVP Exit Criteria (End of Week 3)
- [ ] All 7 core commands working
- [ ] All core templates working (bugfix, feature, commit)
- [ ] Utility templates working (format, lint, test, validate)
- [ ] CI waiting + notification flow working
- [ ] 10+ repos with ATLAS active
- [ ] Using ATLAS daily without major friction

---

### Future Vision

#### Post-MVP Enhancements (v1.x)

| Enhancement | Value | Complexity |
|-------------|-------|------------|
| **Aider integration** | Alternative AI runner | Medium |
| **`refactor` template** | Large refactoring with validation per step | Medium |
| **`test-coverage` template** | Analyze gaps, implement tests | Medium |
| **Trust levels** | Auto-approve based on track record | Medium |
| **Token/cost tracking** | Budget awareness | Low |
| **`atlas config`** | Interactive config editor | Low |

#### Long-term Vision (v2+)

| Capability | What It Enables |
|------------|-----------------|
| **Multi-SDD framework support** | Speckit, BMAD, others — abstracted for user |
| **PM system integration** | GitHub Issues, Linear, Jira → ATLAS tasks |
| **Multi-repo orchestration** | Cross-repo features with coordinated PRs |
| **Cloud execution** | Remote workers for parallel scale-out |
| **Language expansion** | TypeScript, Python, Rust |
| **Team features** | Shared workspaces, approval workflows |

**North Star:** ATLAS becomes the orchestration layer between product vision and shipped code. You describe what you want; ATLAS handles how it gets built, validated, and delivered.
