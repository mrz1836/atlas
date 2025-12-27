---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]
workflowComplete: true
completedAt: 2025-12-27
inputDocuments:
  - _bmad-output/planning-artifacts/product-brief-atlas-2025-12-26.md
  - docs/external/vision.md
  - docs/external/templates.md
  - docs/internal/research-agent.md
documentCounts:
  briefs: 1
  research: 1
  projectDocs: 2
  brainstorming: 0
workflowType: 'prd'
lastStep: 0
date: 2025-12-27
author: MrZ
---

# Product Requirements Document - atlas

**Author:** MrZ
**Date:** 2025-12-27

## Executive Summary

ATLAS (AI Task Lifecycle Automation System) is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the full task lifecycle—analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

The core insight: developers have access to remarkably capable AI agents, but lack the orchestration layer to run them systematically. ATLAS provides that layer, accepting light task descriptions, expanding them through specification-driven workflows, executing validation and git operations automatically, and interrupting only when human judgment is required.

Built for Go developers who maintain multiple repositories, ATLAS targets a 2-3x productivity multiplier by shifting developers from "doing the work" to "reviewing the work."

### What Makes This Special

ATLAS eliminates the "mechanical turk" role—the repetitive ceremony of lint, test, commit, push, PR that developers execute manually across dozens of repos. Instead of babysitting each task through predictable steps, developers stay in planning mode: creating specifications, making architectural decisions, and reviewing completed work.

Key differentiators:
- **Template-to-spec pipeline**: Light input becomes structured specification automatically
- **SDD abstraction**: Speckit workflows without learning Speckit commands
- **Parallel workspace orchestration**: Multiple tasks run simultaneously via Git worktrees
- **Notification-based flow**: Pinged when decisions needed, not polling for status
- **Go-first design**: Optimized for Go development patterns and tooling

## Project Classification

| Attribute | Value |
|-----------|-------|
| **Technical Type** | CLI Tool |
| **Domain** | Developer Tooling |
| **Complexity** | Low |
| **Project Context** | Greenfield - new implementation |

This PRD defines the MVP implementation as specified in the vision document, building ATLAS from scratch with a focused scope: 7 core commands, 3 primary templates, and Git worktree-based parallel execution.

## Success Criteria

### User Success

**The "Aha" Moment:** Opening GitHub to find 3 PRs waiting—meaningful descriptions, passing CI, ready to merge—without having babysat any of them.

**Output Multiplier (2-3x Target):**
| Metric | Baseline | Target |
|--------|----------|--------|
| Substantial PRs merged/week | 2-3 | 6-9 |
| Repos actively developed/week | 1-2 | 3-5 parallel |
| Features shipped/month | Variable | 2-3x current |

**Quality Maintenance:**
| Metric | Target |
|--------|--------|
| CI pass rate on first attempt | >90% |
| PR revision rate | <20% need rework |
| Post-merge issues | Near zero |

**Time Reclamation:**
| Metric | Target |
|--------|--------|
| Mechanical vs. creative time | <25% mechanical (down from 50-75%) |
| Active monitoring time | Low (notification-based, not polling) |

### Business Success

**Adoption Signals:**
- Daily use: ATLAS is part of the standard workflow
- Multi-repo adoption: Active in 10+ repos within 2 months
- "Mage-X clutch" status: Can't imagine working without it
- Burnout reduction: Sustainable pace, less drained

**Role Transformation:**
| From | To |
|------|-----|
| Developer (doing work) | Orchestrator (reviewing work) |
| Watcher (monitoring progress) | Reviewer (notified when PRs ready) |
| 1 task at a time | 3-5 parallel workspaces |

### Technical Success

**Reliability Metrics:**
| Metric | Target |
|--------|--------|
| Task completion rate | >80% of started tasks reach PR |
| Validation pass rate | >85% on first attempt |
| Worktree cleanup | 100% clean on `workspace destroy` |
| Claude Code invocation success | >95% |

**Operational Health:**
- State files always human-readable (JSON/YAML/Markdown)
- Graceful failure with clear error messages
- No orphaned worktrees or zombie processes
- Logs sufficient for debugging any failure

### Measurable Outcomes

**MVP Phase KPIs (First Month):**
| KPI | Target |
|-----|--------|
| PRs merged via ATLAS | 10+ |
| CI first-pass rate | >85% |
| Parallel workspaces used | 3+ simultaneously |
| Time in planning mode | >50% of dev time |
| Repos with ATLAS active | 10+ (within 2 months) |

## Product Scope

### MVP - Minimum Viable Product

**Core Commands (7):**
- `atlas init` - Setup wizard
- `atlas start` - Start task with description + template
- `atlas status` - Show all workspaces and tasks
- `atlas approve` - Approve pending work
- `atlas reject` - Reject with feedback
- `atlas workspace` - List, retire, destroy, logs
- `atlas upgrade` - Self-upgrade + managed tools

**Core Templates (3):**
- `bugfix` - Analyze → implement → validate → commit → PR
- `feature` - Speckit SDD workflow → validate → PR
- `commit` - Smart commits with garbage detection

**Utility Templates (4):**
- `format`, `lint`, `test`, `validate`

**Infrastructure:**
- Git worktree-based parallel workspaces
- Claude Code as AI runner (via CLI)
- Speckit for SDD integration
- File-based state (JSON/YAML)

### Growth Features (Post-MVP)

| Feature | Value | Revisit When |
|---------|-------|--------------|
| Trust levels | Auto-approve based on track record | 100+ task completions |
| `refactor` template | Large refactoring with validation | Core templates proven |
| `test-coverage` template | Analyze gaps, implement tests | Test patterns established |
| Token/cost tracking | Budget awareness | Budget concerns arise |
| Aider integration | Alternative AI runner | User requests |

### Vision (Future)

| Capability | What It Enables |
|------------|-----------------|
| Multi-SDD framework support | Speckit, BMAD, others abstracted |
| PM system integration | GitHub Issues, Linear → ATLAS tasks |
| Multi-repo orchestration | Cross-repo features with coordinated PRs |
| Language expansion | TypeScript, Python, Rust |
| Cloud execution | Remote workers for parallel scale-out |

## User Journeys

### Journey 1: MrZ - First-Time Setup

MrZ has been using Claude Code effectively across his 50+ repos, but something's been nagging him. Every bug fix follows the same ritual: analyze, implement, lint, test, commit, push, PR. He's the mechanical turk in the middle, manually shepherding each task through predictable steps. One evening, while waiting for yet another CI run, he thinks: "I could be reviewing 3 PRs right now instead of babysitting one."

He installs ATLAS and runs the setup wizard:

```bash
go install github.com/mrz1836/atlas@latest
atlas init
```

The wizard walks him through configuration: Claude API key (already set), GitHub auth via `gh` CLI (already configured), tool detection (mage-x, go-pre-commit, Speckit—all found). ATLAS is ready in under a minute.

For his first real task, he picks a simple bug in one of his repos—a null pointer panic he's been meaning to fix:

```bash
atlas start "fix null pointer panic in parseConfig when options is nil" --workspace config-fix
```

He watches the first run nervously. ATLAS creates a worktree, invokes Claude, runs validation, commits, pushes, creates a PR. Ten minutes later, he's looking at a clean PR with a meaningful description, passing CI, and a test case he didn't have to write. He merges it and thinks: "Okay, this might actually work."

### Journey 2: MrZ - Daily Use

It's Tuesday morning. MrZ opens his terminal with coffee in hand. Instead of diving into one task, he queues up three:

```bash
atlas start "fix null pointer in config parser" --workspace config-fix
atlas start "add retry logic to HTTP client" --template feature --workspace http-retry
atlas start "update dependencies in go-broadcast" --workspace deps-update
```

He switches to planning mode—reviewing the feature spec for a new SDK he's been thinking about. In another terminal tab, `atlas status --watch` quietly monitors progress.

Twenty minutes later, a soft terminal bell pings. He glances at status:

```
WORKSPACE      BRANCH              STATUS              STEP      ACTION
config-fix     fix/config-null     ✓ awaiting_approval 8/8      approve
http-retry     feat/http-retry     running             5/12     —
deps-update    chore/deps          running             3/8      —
```

He opens GitHub to find a clean PR: meaningful description explaining the nil check fix, CI passing, new test case added. He reviews the diff—three files, exactly what he expected. Approved. Merged. Back to planning.

By lunch, he's merged 3 PRs across 3 different repos and hasn't run `go test` manually once. The mechanical turk is gone. He's an orchestrator now.

### Journey 3: MrZ - When Things Go Wrong

MrZ has three workspaces running. He's deep in planning mode, sketching out a new feature architecture. His terminal emits a soft bell—ATLAS needs attention.

```bash
atlas status
```

```
WORKSPACE      BRANCH              STATUS              STEP      ACTION
config-fix     fix/config-null     ⚠ validation_failed 4/8      reject/retry
http-retry     feat/http-retry     running             3/12     —
deps-update    chore/deps          ✓ awaiting_approval 7/8      approve
```

The config fix failed validation—lint errors. He checks what happened:

```bash
atlas workspace logs config-fix
```

Claude added a nil check but forgot to update the test mock. Classic. ATLAS presents clear options:

```
? Validation failed: magex lint returned errors. What would you like to do?
  ❯ Retry — AI tries again with error context
    Fix manually — You fix, then 'atlas resume'
    Abandon — End task, keep branch for manual work
```

He selects "Retry"—ATLAS feeds the lint errors back to Claude with context: "Previous attempt failed because the test mock wasn't updated. Fix the mock to expect the new nil-safe behavior."

Two minutes later, another bell. This time validation passes. The PR is ready. MrZ approves and moves on, never having opened an editor.

### Journey 4: MrZ - The Cascade Failure

Worst case scenario. MrZ queued a feature task before lunch. When he returns, a bell is waiting:

```
WORKSPACE      BRANCH              STATUS              STEP      ACTION
auth-feature   feat/auth           ⚠ ci_failed         9/12     view/retry/abandon
```

CI failed after the PR was created. He checks the logs:

```bash
atlas workspace logs auth-feature --step ci_wait
```

The GitHub Actions workflow failed on an integration test. ATLAS presents options:

```
? CI workflow "Integration Tests" failed. What would you like to do?
  ❯ View logs — Open GitHub Actions in browser
    Retry from implement — AI tries to fix based on CI output
    Fix manually — You fix in worktree, then 'atlas resume'
    Abandon — End task, keep PR as draft
```

MrZ views the logs, realizes it's a flaky test unrelated to his changes. He manually re-runs CI in GitHub, then uses `atlas resume auth-feature` to tell ATLAS to keep watching. Five minutes later—green. PR approved and merged.

No matter what fails—Claude, validation, GitHub, CI—MrZ gets immediate notification, clear status, actionable options, and preserved context.

### Notification System

ATLAS operates on a core principle: **MrZ shouldn't be watching. ATLAS should ping him.**

| Event | Notification |
|-------|--------------|
| Task needs approval | Terminal bell + status shows ACTION |
| Validation failed | Terminal bell + `validation_failed` state |
| Claude invocation failed | Terminal bell + `claude_failed` state |
| GitHub operation failed | Terminal bell + `gh_failed` state |
| CI failed after PR | Terminal bell + `ci_failed` state |

**`atlas status --watch` mode:**
- Refreshes every 2 seconds
- Terminal bell (BEL character) on any state requiring attention
- Designed for a background terminal tab

**Future consideration (post-MVP):**
- macOS notifications via `osascript`
- Slack/Discord webhooks for team visibility
- Email digests for long-running tasks

### Journey Requirements Summary

These journeys reveal the following capability requirements:

| Capability | Revealed By |
|------------|-------------|
| Setup wizard (`atlas init`) | First-Time Setup |
| Parallel workspace management | Daily Use |
| Template selection (`--template`) | Daily Use |
| Real-time status with watch mode | Daily Use |
| Terminal bell notifications | All failure journeys |
| Validation retry with context | When Things Go Wrong |
| Manual fix + resume flow | When Things Go Wrong |
| CI watching and failure handling | Cascade Failure |
| Actionable interactive menus | All journeys |
| Log inspection per step | Failure journeys |

## CLI Tool Specific Requirements

### Design Philosophy

ATLAS is a **human-first CLI** for MVP. The primary interface is a developer at a keyboard, not scripts calling ATLAS. Every interaction should be beautiful, informative, and help the user understand what's happening.

**Core Principles:**
- Sexy, polished terminal UI using Charm libraries (Bubble Tea, Lip Gloss, Huh)
- Information-dense but not overwhelming
- Progressive disclosure: simple by default, detail on demand
- Every state has a clear visual representation
- Errors are helpful, not cryptic

### Command Structure

**Core Commands (7):**

| Command | Purpose | Interactive Elements |
|---------|---------|---------------------|
| `atlas init` | Setup wizard | Multi-step form, tool detection, validation |
| `atlas start` | Begin task | Template selection, workspace naming |
| `atlas status` | View all workspaces | Table view, `--watch` mode with live updates |
| `atlas approve` | Approve pending work | Confirmation prompt, optional message |
| `atlas reject` | Reject with feedback | Feedback input, retry options |
| `atlas workspace` | Manage workspaces | Subcommands: list, logs, destroy, retire |
| `atlas upgrade` | Self-update | Progress display, changelog summary |

**Utility Commands:**

| Command | Purpose |
|---------|---------|
| `atlas validate` | Run full validation suite |
| `atlas format` | Run formatters only |
| `atlas lint` | Run linters only |
| `atlas test` | Run tests only |

### Output Formats

**Human-Readable (Default for TTY):**
- Rich table formatting with colors and icons
- Markdown-style output where appropriate
- Status indicators: ✓ ⚠ ✗ ● (running)
- Progress spinners for long operations

**Machine-Readable (`--output json`):**
- Structured JSON for all commands
- Consistent envelope: `{ "success": bool, "data": {...}, "error": {...} }`
- NDJSON for streaming output (logs, watch mode)
- Exit codes: 0 (success), 1 (error), 2 (invalid input)

**Examples:**

```bash
# Human-readable (default)
atlas status
WORKSPACE      BRANCH              STATUS              STEP      ACTION
config-fix     fix/config-null     ✓ awaiting_approval 8/8      approve

# Machine-readable
atlas status --output json
{"success": true, "data": {"workspaces": [...]}}
```

### Configuration Schema

**Layered Configuration (highest priority wins):**

```
CLI flags              ← Immediate overrides
ATLAS_* env vars       ← Runtime/CI overrides
.atlas/config.yaml     ← Project-specific
~/.atlas/config.yaml   ← User defaults
Built-in defaults      ← Sensible starting point
```

**Key Configuration Areas:**

| Area | Example Keys |
|------|--------------|
| AI Provider | `ai.provider`, `ai.model`, `ai.api_key_env` |
| Validation | `validation.commands`, `validation.timeout` |
| Git | `git.default_branch`, `git.commit_style` |
| Notifications | `notifications.bell`, `notifications.sound` |
| Templates | `templates.default`, `templates.custom_path` |

**Environment Variables (namespaced):**
- `ATLAS_AI_PROVIDER` - Override AI provider
- `ATLAS_AI_MODEL` - Override model selection
- `ATLAS_LOG_LEVEL` - Debug, info, warn, error
- `ATLAS_NO_COLOR` - Disable colored output

### Scripting Support

While MVP focuses on human interaction, scripting basics are included:

- All commands accept `--output json` for parseable output
- Consistent exit codes for automation
- `--quiet` flag suppresses non-essential output
- `--no-interactive` skips prompts (uses defaults or fails)

**Deferred for post-MVP:**
- Shell completion (bash/zsh/fish)
- `--batch` mode for processing multiple tasks
- Webhook callbacks for external integrations

### Terminal UI Components

**Using Charm ecosystem:**

| Component | Library | Usage |
|-----------|---------|-------|
| Interactive forms | Huh | `atlas init` wizard, feedback input |
| Live status | Bubble Tea | `atlas status --watch` |
| Styled output | Lip Gloss | Tables, status indicators, errors |
| Spinners/Progress | Bubble Tea | Long operations, AI invocation |

**Visual Language:**

| State | Icon | Color |
|-------|------|-------|
| Running | ● | Blue |
| Awaiting approval | ✓ | Green |
| Validation failed | ⚠ | Yellow |
| Error/Failed | ✗ | Red |
| Completed | ✓ | Dim |

## Project Scoping & Phased Development

### MVP Strategy & Philosophy

**MVP Approach:** Problem-Solving MVP
Solve the core problem (eliminate mechanical turk role) with minimal but polished features. The user is the developer - if it's not easy and intuitive, it won't get used.

**Resource Requirements:**
- Solo developer (MrZ) + AI assistance
- Leverage Claude for implementation acceleration
- Focus on depth over breadth

**Non-Negotiables:**
- Beautiful, intuitive TUI (Charm ecosystem)
- Effortless UX - reduces friction, not adds it
- Reliable core loop (start → validate → PR)

### MVP Feature Set (Phase 1)

**Core User Journeys Supported:**
- First-time setup (streamlined `atlas init`)
- Daily parallel task execution (3+ workspaces)
- Error recovery with clear options (retry, fix manually, abandon)
- CI watching and notification

**Must-Have Capabilities:**

| Capability | Rationale |
|------------|-----------|
| 7 core commands | Complete workflow coverage |
| `bugfix` template | Most common task type |
| `feature` template | Speckit SDD integration |
| `commit` template | Smart commits with garbage detection |
| Parallel workspaces | Core value proposition |
| Terminal bell notifications | Don't poll, get pinged |
| `--watch` mode | Background monitoring |
| Validation retry with context | AI learns from errors |
| Beautiful TUI | Non-negotiable UX |

### Technical Risks & Mitigation

**Critical Risk: Claude Code CLI Invocation**

| Risk | Impact | Mitigation |
|------|--------|------------|
| Claude Code CLI may not support custom slash commands via `-p` | Breaks SDD abstraction, forces manual Speckit invocation | **Early investigation required** - test Claude Code CLI capabilities before deep implementation |
| Claude Code invocation might be unreliable | Task failures, user frustration | Retry logic with exponential backoff, clear error states |

**Mitigation Strategy:**
1. **Week 1 Spike:** Test Claude Code CLI with `-p` flag and custom prompts
2. **Fallback Plan:** If `-p` doesn't work, evaluate:
   - Direct Anthropic API calls (bypass Claude Code)
   - Claude Code `--continue` patterns
   - Alternative AI runner (Aider, etc.)
3. **Design for abstraction:** AIRunner interface allows swapping implementations

**Low Risks:**
- Speckit integration (user has 10-15 uses, familiar)
- Git worktree management (well-documented Go libraries)
- Charm TUI (mature ecosystem, good docs)

### Post-MVP Features (Phase 2)

| Feature | Value | Trigger |
|---------|-------|---------|
| Trust levels | Auto-approve based on track record | 100+ successful tasks |
| `refactor` template | Large refactoring workflows | Core templates proven |
| `test-coverage` template | Analyze gaps, implement tests | Test patterns established |
| Token/cost tracking | Budget awareness | If costs become a concern |
| macOS notifications | System-level pings | If terminal bell isn't enough |
| Shell completion | bash/zsh/fish | If UX needs it |

### Vision Features (Phase 3+)

| Capability | What It Enables |
|------------|-----------------|
| Multi-SDD framework support | Speckit, BMAD, others abstracted |
| Alternative AI runners | Aider, direct API, future tools |
| PM system integration | GitHub Issues → ATLAS tasks |
| Multi-repo orchestration | Cross-repo features |
| Language expansion | TypeScript, Python, Rust |

### Implementation Strategy Note

*Out of scope for PRD, but noted: The goal is to leverage AI (Claude) effectively throughout implementation - not just as the runtime engine, but as a development accelerator. Build fast, iterate, ship.*

## Functional Requirements

### Setup & Configuration

- FR1: User can initialize ATLAS in a Git repository via setup wizard
- FR2: User can configure AI provider settings (API keys, model selection)
- FR3: User can configure validation commands per project
- FR4: User can configure notification preferences
- FR5: System can auto-detect installed tools (mage-x, go-pre-commit, Speckit, gh CLI)
- FR6: User can override global configuration with project-specific settings
- FR7: User can override configuration via environment variables
- FR8: User can self-upgrade ATLAS and managed tools

### Task Management

- FR9: User can start a task with natural language description
- FR10: User can select a template for the task (bugfix, feature, commit)
- FR11: User can specify a custom workspace name for the task
- FR12: System can expand task description into structured specification (SDD abstraction)
- FR13: User can run utility commands (format, lint, test, validate) standalone

### Workspace Management

- FR14: System can create isolated Git worktrees for parallel task execution
- FR15: User can view all active workspaces and their status
- FR16: User can destroy a workspace and clean up its worktree
- FR17: User can retire a completed workspace (archive state, remove worktree)
- FR18: User can view logs for a specific workspace
- FR19: User can view logs for a specific step within a workspace
- FR20: System can manage 3+ parallel workspaces simultaneously

### AI Orchestration

- FR21: System can invoke Claude Code CLI for task execution
- FR22: System can pass task context and prompts to AI runner
- FR23: System can capture AI runner output and artifacts
- FR24: System can abstract Speckit SDD workflows behind templates
- FR25: System can provide error context to AI for retry attempts
- FR26: User can configure AI model selection per task or globally

### Validation & Quality

- FR27: System can execute validation commands (lint, test, format)
- FR28: System can detect validation failures and pause for user decision
- FR29: User can retry validation with AI fix attempt
- FR30: User can fix validation issues manually and resume
- FR31: User can abandon task while preserving branch and worktree
- FR32: System can auto-format code before other validations
- FR33: System can run pre-commit hooks as validation step

### Git Operations

- FR34: System can create feature branches with consistent naming
- FR35: System can stage and commit changes with meaningful messages
- FR36: System can detect and warn about garbage files before commit
- FR37: System can push branches to remote
- FR38: System can create pull requests via gh CLI
- FR39: System can monitor GitHub Actions CI status after PR creation
- FR40: System can detect CI failures and notify user

### Status & Monitoring

- FR41: User can view real-time status of all workspaces in table format
- FR42: User can enable watch mode for continuous status updates
- FR43: System can emit terminal bell when task needs attention
- FR44: System can display task step progress (e.g., "Step 4/8")
- FR45: System can show clear action indicators (approve, retry, etc.)
- FR46: User can output status in JSON format for scripting

### User Interaction

- FR47: User can approve completed work and trigger merge-ready state
- FR48: User can reject work with feedback for AI retry
- FR49: System can present interactive menus for error recovery decisions
- FR50: System can display styled output with colors and icons
- FR51: System can show progress spinners for long operations
- FR52: User can run in non-interactive mode with sensible defaults

## Non-Functional Requirements

### Performance

**CLI Responsiveness:**
- Local operations (status, workspace list) complete in <1 second
- UI remains responsive during long-running AI operations (non-blocking)
- Timeouts for network operations: 30 seconds default, configurable

**AI Operations:**
- AI invocation latency is expected and acceptable (minutes, not seconds)
- Progress indication during AI operations (spinner, step display)
- No performance targets for AI completion time

### Security

**API Key Handling:**
- API keys read from environment variables or secure config
- API keys never logged or displayed in output
- API keys never committed to Git (warn if detected in worktree)

**Credential Safety:**
- GitHub auth delegated to gh CLI (no token storage in ATLAS)
- No sensitive data in JSON log output
- Config files should not contain secrets in plain text (use env var references)

### Reliability

**State & Checkpoints:**
- Task state saved after each step completion (safe checkpoint)
- On failure or crash, task can resume from last completed step
- State files always human-readable (JSON/YAML)
- State files can be manually edited if needed

**Worktree Management:**
- Worktree creation must be atomic (no partial state)
- Worktree destruction must be 100% reliable (no orphaned directories)
- No orphaned Git branches after workspace cleanup
- `atlas workspace destroy` always succeeds, even if state is corrupted

**Error Handling:**
- All errors have clear, actionable messages
- System never hangs indefinitely (timeouts on all external operations)
- Partial failures leave system in recoverable state

### Integration

**Claude Code CLI:**
- Invocation via CLI subprocess
- Must handle Claude Code CLI errors gracefully
- **Risk noted:** Custom slash commands via `-p` flag may not work - early validation required
- Fallback strategy defined if primary invocation method fails

**GitHub (gh CLI):**
- All GitHub operations via gh CLI (no direct API calls)
- Requires gh CLI authenticated (`gh auth status`)
- PR creation, CI status checking, branch operations

**Git:**
- Standard Git operations via subprocess
- Worktree management via `git worktree` commands
- Branch naming follows configurable convention

**Speckit:**
- SDD workflows abstracted behind templates
- Speckit invocation via CLI subprocess
- User doesn't need to know Speckit commands

### Operational

**Logging:**
- Structured JSON logs for debugging
- Log levels: debug, info, warn, error
- Logs stored per-workspace, accessible via `atlas workspace logs`

**Observability:**
- Clear task step progress (e.g., "Step 4/8: Running validation")
- Terminal bell on state changes requiring attention
- `--verbose` flag for detailed operation logging
