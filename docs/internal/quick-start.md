# ATLAS Quick-Start & CLI Reference

**ATLAS** (AI Task Lifecycle Automation System) is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [Prerequisites](#prerequisites)
3. [Installation](#installation)
4. [Global Flags](#global-flags)
5. [CLI Commands Reference](#cli-commands-reference)
   - [atlas init](#atlas-init)
   - [atlas start](#atlas-start)
   - [atlas status](#atlas-status)
   - [atlas approve](#atlas-approve)
   - [atlas reject](#atlas-reject)
   - [atlas resume](#atlas-resume)
   - [atlas abandon](#atlas-abandon)
   - [atlas recover](#atlas-recover)
   - [atlas validate](#atlas-validate)
   - [atlas format](#atlas-format)
   - [atlas lint](#atlas-lint)
   - [atlas test](#atlas-test)
   - [atlas upgrade](#atlas-upgrade)
   - [atlas config](#atlas-config)
   - [atlas workspace](#atlas-workspace)
6. [Templates](#templates)
7. [Task States](#task-states)
8. [Workflows](#workflows)
9. [Configuration](#configuration)
10. [File Structure](#file-structure)
11. [Troubleshooting](#troubleshooting)

---

## Quick Start

Get running in 2 minutes:

```bash
# 1. Check prerequisites
go version        # Need 1.24+
git --version     # Need 2.20+
gh --version      # Need 2.20+
claude --version  # Need 2.0.76+

# 2. Install ATLAS
go install github.com/mrz1836/atlas@latest

# 3. Initialize (one-time setup)
atlas init

# 4. Run your first task
cd /path/to/your/go/project
atlas start "fix the null pointer in parseConfig" --template bugfix

# 5. Monitor progress
atlas status --watch

# 6. Approve when ready
atlas approve
```

---

## Prerequisites

| Tool | Required Version | Purpose | Installation |
|------|------------------|---------|--------------|
| **Go** | 1.24+ | Runtime | [go.dev](https://go.dev/dl/) |
| **Git** | 2.20+ | Version control | `brew install git` |
| **GitHub CLI (gh)** | 2.20+ | PR operations | `brew install gh` |
| **Claude CLI** | 2.0.76+ | AI execution engine | [Claude Code](https://docs.anthropic.com/en/docs/claude-code) |
| **uv** | 0.5.x | Python tool runner (for Speckit) | `brew install uv` |

ATLAS manages additional tools automatically:
- **mage-x** (magex command) - Standardized build/test toolkit
- **go-pre-commit** - Pre-commit hooks
- **Speckit** - Specification-driven development framework

---

## Installation

```bash
# Install ATLAS globally
go install github.com/mrz1836/atlas@latest

# Run the setup wizard
atlas init

# Verify installation
atlas --version
```

The `atlas init` wizard will:
1. Check for required tools
2. Install/upgrade ATLAS-managed tools
3. Configure AI provider settings
4. Set up default templates

---

## Global Flags

These flags work with all commands:

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output format (`text` or `json`) | `text` |
| `--verbose` | `-v` | Enable debug-level logging | `false` |
| `--quiet` | `-q` | Suppress non-essential output | `false` |

**Note:** `--verbose` and `--quiet` are mutually exclusive.

**Exit Codes:**
- `0` - Success
- `1` - Execution error
- `2` - Invalid input (bad flags, missing arguments)

**Environment Variables:**

Use the `ATLAS_` prefix to set flags via environment:
```bash
export ATLAS_OUTPUT=json
export ATLAS_VERBOSE=true
```

---

## CLI Commands Reference

### atlas init

Initialize ATLAS configuration.

```bash
# Interactive setup wizard
atlas init

# Non-interactive with defaults
atlas init --no-interactive

# Save to global config only
atlas init --global

# Save to project config only
atlas init --project
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--no-interactive` | Skip all prompts, use defaults |
| `--global` | Save to `~/.atlas/config.yaml` only |
| `--project` | Save to `.atlas/config.yaml` only |

**Configuration sections:**
- AI provider settings (model, API key env var, timeout, max turns)
- Validation commands (format, lint, test, pre-commit)
- Notification preferences (bell, events)

---

### atlas start

Start a new task with the given description.

```bash
# Basic usage - interactive template selection
atlas start "fix null pointer in parseConfig"

# Specify template
atlas start "fix null pointer" --template bugfix

# Custom workspace name
atlas start "Update the quick start guide in docs/internal for the new task template" -t task -w task-workspace --model opus

# Use a specific model
atlas start "complex refactor" --model opus

# Enable/disable AI verification
atlas start "quick fix" --verify
atlas start "quick fix" --no-verify

# Non-interactive mode (requires --template)
atlas start "fix bug" -t bugfix --no-interactive
```

**Flags:**

| Flag | Short | Description | Values |
|------|-------|-------------|--------|
| `--template` | `-t` | Template to use | `bugfix`, `feature`, `task`, `commit` |
| `--workspace` | `-w` | Custom workspace name | Any string (sanitized) |
| `--model` | `-m` | AI model to use | `sonnet`, `opus`, `haiku` |
| `--verify` | | Enable AI verification step | |
| `--no-verify` | | Disable AI verification step | |
| `--no-interactive` | | Disable interactive prompts | |

**Workspace Naming:**
- Auto-generated from description (lowercase, hyphens, max 50 chars)
- Override with `--workspace <name>`
- Branch format: `<prefix>/<workspace-name>` (e.g., `fix/null-pointer-config`)

---

### atlas status

Show workspace status dashboard.

```bash
# One-time status snapshot
atlas status

# Live updating mode (refreshes every 2s)
atlas status --watch

# Custom refresh interval
atlas status --watch --interval 5s

# Show visual progress bars
atlas status --watch --progress
```

**Flags:**

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--watch` | `-w` | Enable live updating mode | `false` |
| `--interval` | | Refresh interval (min 500ms) | `2s` |
| `--progress` | `-p` | Show visual progress bars | `false` |

**Output Columns:**
- `WORKSPACE` - Workspace name
- `BRANCH` - Git branch
- `STATUS` - Task status
- `STEP` - Current step (e.g., `3/7`)
- `ACTION` - Suggested next command

**Watch Mode Features:**
- Automatic refresh
- Terminal bell on `awaiting_approval`
- Timestamp of last update
- Ctrl+C to exit

---

### atlas approve

Approve a completed task awaiting approval.

```bash
# Interactive selection (if multiple pending)
atlas approve

# Approve specific workspace
atlas approve my-workspace

# Skip interactive menu
atlas approve --auto-approve

# JSON output for scripting
atlas approve my-workspace --output json
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--auto-approve` | Skip interactive menu, approve directly |

**Interactive Options:**
- Approve and complete
- View git diff
- View execution logs
- Open PR in browser
- Reject task
- Cancel

---

### atlas reject

Reject work with feedback.

```bash
# Interactive rejection flow
atlas reject my-workspace

# JSON mode: Retry with feedback
atlas reject --output json --retry --feedback "fix the tests" --step 3

# JSON mode: Reject and be done
atlas reject --output json --done
```

**Interactive Modes:**

1. **Reject and Retry** - AI tries again with your feedback
   - Select reason (code quality, missing tests, wrong approach, etc.)
   - Choose which step to retry from
   - Provide additional guidance

2. **Reject (Done)** - End task, keep code for manual work
   - Branch preserved with current changes
   - Feedback stored for reference

**Flags (JSON mode only):**

| Flag | Description |
|------|-------------|
| `--retry` | Retry with feedback |
| `--feedback <text>` | Feedback for AI |
| `--step <N>` | Step number to resume from |
| `--done` | Reject and end task |

---

### atlas resume

Resume a paused or failed task.

```bash
# Resume with current state
atlas resume my-workspace

# Resume with AI attempting to fix errors
atlas resume my-workspace --ai-fix
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--ai-fix` | Retry with AI attempting to fix errors |

---

### atlas abandon

Abandon a failed task (preserves branch and worktree).

```bash
# Interactive confirmation
atlas abandon my-workspace

# Skip confirmation
atlas abandon my-workspace --force
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Skip confirmation prompt |

**Applicable States:**
- `validation_failed`
- `gh_failed`
- `ci_failed`
- `ci_timeout`

---

### atlas recover

Recover from task error states with guided options.

```bash
# Interactive recovery menu
atlas recover my-workspace

# JSON mode: Retry with AI fix
atlas recover --output json --retry

# JSON mode: Get manual fix instructions
atlas recover --output json --manual

# JSON mode: Abandon task
atlas recover --output json --abandon

# JSON mode: Continue waiting (ci_timeout only)
atlas recover --output json --continue
```

**Interactive Options:**
- Retry with AI fix
- Fix manually (instructions provided)
- View errors/logs
- Abandon task

**Flags (JSON mode only):**

| Flag | Description |
|------|-------------|
| `--retry` | Retry with AI attempting to fix |
| `--manual` | Get manual fix instructions |
| `--abandon` | Abandon task |
| `--continue` | Continue waiting (ci_timeout only) |

**Applicable States:**
- `validation_failed`
- `gh_failed`
- `ci_failed`
- `ci_timeout`

---

### atlas validate

Run the full validation suite.

```bash
# Run all validation commands
atlas validate

# Quiet mode - show only final pass/fail
atlas validate --quiet
```

**Execution Order:**
1. Format (sequential)
2. Lint + Test (parallel)
3. Pre-commit (sequential)

**Default Commands:**
- `magex format:fix`
- `magex lint`
- `magex test`
- `go-pre-commit run --all-files`

---

### atlas format

Run code formatters.

```bash
atlas format
```

**Default Command:** `magex format:fix`

---

### atlas lint

Run linters.

```bash
atlas lint
```

**Default Command:** `magex lint`

---

### atlas test

Run tests.

```bash
atlas test
```

**Default Command:** `magex test`

---

### atlas upgrade

Check and install tool updates for ATLAS and managed tools.

```bash
# Upgrade ATLAS and managed tools
atlas upgrade

# Check for updates without installing
atlas upgrade --check

# Skip confirmation prompts
atlas upgrade --yes

# Upgrade specific tool
atlas upgrade speckit
atlas upgrade mage-x
atlas upgrade go-pre-commit
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--check` | Dry run - show updates without installing |
| `--yes` | Skip confirmation prompts |

**Managed Tools:**
- mage-x (magex command)
- go-pre-commit
- Speckit

---

### atlas config

Manage ATLAS configuration.

#### atlas config show

Display effective configuration with sources.

```bash
# YAML output (default)
atlas config show

# JSON output
atlas config show --output json
```

**Flags:**

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output format (`yaml` or `json`) | `yaml` |

**Features:**
- Shows config values with source annotations (default/global/project/env)
- Masks sensitive values (API keys, tokens)

#### atlas config ai

Configure AI provider settings interactively.

```bash
# Interactive configuration
atlas config ai

# Show current values without prompting
atlas config ai --no-interactive
```

**Configurable Settings:**
- Default model (sonnet, opus, haiku)
- API key environment variable
- Timeout duration
- Max agentic turns

#### atlas config validation

Configure validation commands.

```bash
atlas config validation
```

**Configurable Commands:**
- Format command
- Lint command
- Test command
- Pre-commit command
- Custom pre-PR hooks

#### atlas config notifications

Configure notification preferences.

```bash
atlas config notifications
```

**Configurable Settings:**
- Bell notifications (on/off)
- Events that trigger notifications

---

### atlas workspace

Manage ATLAS workspaces.

#### atlas workspace list

List all workspaces with status, branch, creation time, and task count.

```bash
# List all workspaces
atlas workspace list

# Short alias
atlas workspace ls
```

**Output Columns:**
- Workspace name
- Branch
- Status (active, paused, retired)
- Created time
- Task count

#### atlas workspace destroy

Completely remove a workspace, worktree, branch, and state.

```bash
# With confirmation prompt
atlas workspace destroy my-workspace

# Skip confirmation
atlas workspace destroy my-workspace --force
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Skip confirmation prompt |

**Warning:** This cannot be undone. Deletes:
- ATLAS workspace state
- Git worktree
- Local branch

#### atlas workspace retire

Archive a workspace (removes worktree, preserves history).

```bash
# With confirmation prompt
atlas workspace retire my-workspace

# Skip confirmation
atlas workspace retire my-workspace --force
```

**Use Case:** When done with a workspace but want to keep the history for reference.

#### atlas workspace logs

View workspace task execution logs.

```bash
# View all logs
atlas workspace logs my-workspace

# Stream new logs (follow mode)
atlas workspace logs my-workspace --follow

# Filter by step name
atlas workspace logs my-workspace --step validate

# Filter by task ID
atlas workspace logs my-workspace --task-id task-20251226-100000

# Show last N lines
atlas workspace logs my-workspace --tail 50

# Combined filters
atlas workspace logs my-workspace --follow --step validate --tail 100
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--follow` | `-f` | Stream new logs as they appear |
| `--step` | | Filter by step name |
| `--task-id` | | Filter by task ID |
| `--tail` | | Show last N lines |

**Log Features:**
- Color-coded by log level (info/warn/error/debug)
- JSON-lines format
- Filterable by step and task

---

## Templates

ATLAS provides pre-defined workflow templates:

| Template | Description | Branch Prefix | Use Case |
|----------|-------------|---------------|----------|
| **bugfix** | Analyze → Implement → Validate → Commit → PR | `fix` | Bug fixes requiring analysis |
| **feature** | Speckit SDD: Specify → Plan → Tasks → Implement → Validate → PR | `feat` | New features with specifications |
| **task** | Implement → (Verify) → Validate → Commit → PR | `task` | Simple, well-defined tasks |
| **commit** | Smart commits: Garbage detection, logical grouping, message generation | `chore` | Commit assistance |

**Template Details:**

- **bugfix**: Best for issues requiring investigation. Includes an `analyze` step to understand the problem before implementing.
- **feature**: Full Speckit SDD workflow with specification, planning, and task breakdown. Ideal for complex features.
- **task**: Fastest workflow for straightforward changes. Skips analysis and goes straight to implementation. Add `--verify` for optional AI cross-validation.
- **commit**: Specialized template for creating intelligent commits from existing changes.

**Utility Templates:**
- `format` - Run code formatting only
- `lint` - Run linters only
- `test` - Run tests only
- `validate` - Full validation suite

---

## Task States

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ATLAS Task State Machine                          │
└─────────────────────────────────────────────────────────────────────────────┘

  pending
     │
     │ start
     ▼
  running ──────────────────┬───────────────────────────────────────┐
     │                      │                                       │
     │ step complete        │ GitHub fails                          │ CI fails
     ▼                      ▼                                       ▼
  validating           gh_failed                          ci_failed / ci_timeout
     │                      │                                       │
     ├── pass ─────────────►│◄──────── retry ───────────────────────┤
     │                      │                                       │
     │                      └──────── abandon ──────────────────────┴──► abandoned
     ▼
  awaiting_approval
     │
     ├── approve ──────────────────────────────────────────────────►  completed
     │
     ├── retry with feedback ──────────────────────────────────────►  running
     │
     └── reject ───────────────────────────────────────────────────►  rejected

  validation_failed
     │
     ├── retry ────────────────────────────────────────────────────►  running
     │
     └── abandon ──────────────────────────────────────────────────►  abandoned
```

**Status Values:**
- `pending` - Queued, not yet started
- `running` - AI agent actively executing
- `validating` - Undergoing validation checks
- `validation_failed` - Validation failed, needs human decision
- `awaiting_approval` - Validation passed, waiting for human approval
- `completed` - Successfully finished and approved
- `rejected` - User rejected during approval
- `abandoned` - Manually abandoned by user
- `gh_failed` - GitHub operations failed
- `ci_failed` - CI pipeline failed
- `ci_timeout` - CI exceeded timeout

---

## Workflows

### Bugfix Workflow

```bash
# 1. Start the task
atlas start "fix null pointer in parseConfig when options is nil" --template bugfix

# 2. Monitor progress
atlas status --watch

# 3. Wait for awaiting_approval status...

# 4. Review and approve
atlas approve my-workspace
```

**Steps:**
1. `analyze` - AI analyzes the problem
2. `implement` - AI implements the fix
3. `validate` - Run format, lint, test, pre-commit
4. `git_commit` - Create branch and commit
5. `git_push` - Push to remote
6. `git_pr` - Create pull request
7. `ci_wait` - Wait for CI to pass
8. `review` - Human approval

### Feature Workflow (Speckit SDD)

```bash
# Start with feature template
atlas start "add retry logic to HTTP client" --template feature

# Steps include:
# 1. specify - /speckit.specify creates spec.md
# 2. review_spec - Human reviews specification
# 3. plan - /speckit.plan creates plan.md
# 4. tasks - /speckit.tasks creates tasks.md
# 5. implement - /speckit.implement executes
# 6. validate - Run validation suite
# 7. checklist - /speckit.checklist for quality
# 8. git_commit, git_push, git_pr, ci_wait, review
```

### Task Workflow

The **task** template is ideal for simple, well-defined work where you know exactly what needs to be done. It skips the analysis phase and goes straight to implementation, making it the fastest workflow for straightforward changes.

```bash
# Start a simple task
atlas start "add logging to HTTP client" --template task

# Monitor progress
atlas status --watch

# Steps:
# 1. implement - AI implements the task directly
# 2. verify - Optional AI verification (enable with --verify)
# 3. validate - Run format, lint, test, pre-commit
# 4. git_commit - Create branch and commit
# 5. git_push - Push to remote
# 6. git_pr - Create pull request
# 7. ci_wait - Wait for CI to pass
# 8. review - Human approval

# Enable verification for more complex tasks
atlas start "refactor HTTP client" --template task --verify
```

**Key Features:**
- **Direct implementation**: Skips analysis, goes straight to coding
- **Optional verification**: Add `--verify` flag for AI cross-validation using a different model
- **Branch prefix**: Creates `task` branches
- **Default model**: Uses `sonnet` for speed
- **Best for**: Documentation updates, simple refactors, adding straightforward features

**When to use Task vs Bugfix vs Feature:**
- **Task**: Simple, well-defined work with clear requirements (e.g., "add logging", "update dependencies", "rename function", "update documentation")
- **Bugfix**: Problem analysis needed (e.g., "fix null pointer", "resolve race condition")
- **Feature**: Complex changes requiring specification (e.g., "add authentication", "implement caching layer")

### Parallel Features

```bash
# Terminal 1 - Start first feature
atlas start "add user authentication" --workspace auth

# Terminal 2 - Start second feature
atlas start "fix payment processing" --workspace payment

# Monitor both workspaces
atlas status
# Output:
# WORKSPACE   BRANCH         STATUS              STEP    ACTION
# auth        feat/auth      running             3/7     —
# payment     fix/payment    awaiting_approval   6/7     approve

# Approve the ready one
atlas approve payment

# Cleanup after PR merge
atlas workspace destroy payment
```

### Error Recovery Workflow

```bash
# When validation fails
atlas status
# Shows: validation_failed

# Option 1: Let AI fix it
atlas resume my-workspace --ai-fix

# Option 2: Guided recovery
atlas recover my-workspace
# Interactive menu with options:
# - Retry with AI fix
# - Fix manually
# - View errors/logs
# - Abandon task

# Option 3: Fix manually then continue
# (make manual fixes in worktree)
atlas resume my-workspace
```

---

## Configuration

### Configuration Locations

| Location | Scope | Path |
|----------|-------|------|
| Global | User-wide defaults | `~/.atlas/config.yaml` |
| Project | Repository-specific | `.atlas/config.yaml` |

### Configuration Precedence

1. CLI flags (highest priority)
2. Project config (`.atlas/config.yaml`)
3. Global config (`~/.atlas/config.yaml`)
4. Built-in defaults (lowest priority)

### Environment Variables

Use the `ATLAS_` prefix:
```bash
export ATLAS_OUTPUT=json
export ATLAS_VERBOSE=true
export ATLAS_QUIET=false
```

### Configuration File Reference

```yaml
# ~/.atlas/config.yaml or .atlas/config.yaml

#------------------------------------------------------------------------------
# AI Configuration
#------------------------------------------------------------------------------
ai:
  # Default model for AI operations
  # Options: sonnet, opus, haiku (or full model ID)
  default_model: sonnet

  # Maximum duration for AI operations
  timeout: 30m

  # Maximum agentic turns per invocation
  max_turns: 10

  # Environment variable containing API key
  api_key_env: ANTHROPIC_API_KEY

#------------------------------------------------------------------------------
# Validation Commands
#------------------------------------------------------------------------------
validation:
  # Code formatting command
  format: magex format:fix

  # Linting command
  lint: magex lint

  # Test command
  test: magex test

  # Pre-commit hooks command
  pre_commit: go-pre-commit run --all-files

  # Custom pre-PR hook (optional)
  # custom_pre_pr: magex integration-test

#------------------------------------------------------------------------------
# Notification Settings
#------------------------------------------------------------------------------
notifications:
  # Enable terminal bell on attention-required states
  bell: true

  # Events that trigger notifications
  events:
    - awaiting_approval
    - validation_failed
    - gh_failed
    - ci_failed
    - ci_timeout

#------------------------------------------------------------------------------
# Template Overrides
#------------------------------------------------------------------------------
templates:
  bugfix:
    # AI model for bugfix tasks
    model: sonnet
    # Branch prefix for bugfix branches
    branch_prefix: fix
    # Auto-proceed on git operations
    auto_proceed_git: true

  feature:
    model: opus
    branch_prefix: feat
    auto_proceed_git: false

  task:
    model: sonnet
    branch_prefix: task
    auto_proceed_git: true

  commit:
    branch_prefix: chore

#------------------------------------------------------------------------------
# CI Configuration
#------------------------------------------------------------------------------
ci:
  # GitHub Actions workflows to wait for
  workflows:
    - name: "CI"
      required: true
    - name: "Lint"
      required: true

  # How often to poll CI status
  poll_interval: 2m

  # Maximum time to wait for CI
  timeout: 30m

#------------------------------------------------------------------------------
# Worktree Configuration
#------------------------------------------------------------------------------
worktree:
  # Base directory for worktrees
  # Empty = sibling to repo (e.g., ../myrepo-auth)
  base_dir: ""
```

---

## File Structure

### ATLAS Home Directory

```
~/.atlas/
├── config.yaml                           # Global configuration
├── logs/
│   └── atlas.log                         # CLI operations log (rotated)
├── backups/
│   └── speckit-<timestamp>/              # Speckit upgrade backups
└── workspaces/
    └── <workspace-name>/
        ├── workspace.json                # Workspace metadata + task history
        └── tasks/
            └── task-YYYYMMDD-HHMMSS/     # Task ID (timestamp-based)
                ├── task.json             # Task state & step history
                ├── task.log              # Full execution log (JSON-lines)
                └── artifacts/
                    ├── analyze.md        # Analysis output
                    ├── spec.md           # Specification (feature template)
                    ├── plan.md           # Implementation plan
                    ├── tasks.md          # Task breakdown
                    ├── checklist.md      # Quality checklist
                    ├── validation.json   # Validation results
                    ├── validation.1.json # Previous attempt (on retry)
                    └── pr-description.md # Generated PR description
```

### Git Worktree Location

By default, worktrees are created as siblings to your repository:

```
/Users/me/projects/
├── myrepo/                    # Original repository
├── myrepo-auth/               # Worktree for 'auth' workspace
├── myrepo-payment/            # Worktree for 'payment' workspace
└── myrepo-feature-x/          # Worktree for 'feature-x' workspace
```

### Browsing Examples

```bash
# All PR descriptions ever created
cat ~/.atlas/workspaces/*/tasks/*/artifacts/pr-description.md

# All artifacts for a specific task
ls ~/.atlas/workspaces/auth/tasks/task-20251226-143022/artifacts/

# Workspace task history
jq '.tasks' ~/.atlas/workspaces/auth/workspace.json

# Latest task in a workspace (sorts chronologically)
ls ~/.atlas/workspaces/auth/tasks/ | tail -1

# Parse logs with jq
cat ~/.atlas/workspaces/*/tasks/*/task.log | jq 'select(.event=="model_complete")'
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `not in a git repository` | Running outside a git repo | `cd` to your git project root |
| `workspace 'x' exists` | Workspace name conflict | Use `--workspace <new-name>` or `atlas workspace destroy x` |
| `template required` | Non-interactive mode without template | Add `--template bugfix` (or `feature`, `commit`) |
| `invalid model` | Unknown model name | Use `sonnet`, `opus`, or `haiku` |
| Validation failed | Code doesn't pass checks | `atlas recover` or fix manually, then `atlas resume` |
| CI timeout | CI taking too long | `atlas recover` → continue waiting or retry |
| GitHub auth failed | gh CLI not authenticated | Run `gh auth login` |
| Claude CLI not found | claude not installed | Install [Claude Code](https://docs.anthropic.com/en/docs/claude-code) |

### Debugging

```bash
# Enable verbose logging
atlas --verbose start "description" -t bugfix

# Check specific workspace logs
atlas workspace logs my-workspace --follow

# View task state
cat ~/.atlas/workspaces/my-workspace/tasks/*/task.json | jq

# View full task execution log
cat ~/.atlas/workspaces/my-workspace/tasks/*/task.log

# Check configuration sources
atlas config show
```

### Getting Help

```bash
# General help
atlas --help

# Command-specific help
atlas start --help
atlas workspace --help
atlas config --help
```

---

*This document was generated for ATLAS MVP. For detailed architecture information, see [docs/external/vision.md](../external/vision.md).*
