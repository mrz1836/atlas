# ATLAS Quick-Start & CLI Reference

**ATLAS** (AI Task Lifecycle Automation System) is a CLI tool that orchestrates AI-assisted development workflows for Go projects. It automates the cycle of analyzing issues, implementing fixes, validating code, and creating pull requests—while keeping humans in control at every decision point.

<br>

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
   - [atlas hook](#atlas-hook)
      - [atlas hook status](#atlas-hook-status)
      - [atlas hook checkpoints](#atlas-hook-checkpoints)
      - [atlas hook verify-receipt](#atlas-hook-verify-receipt)
      - [atlas hook regenerate](#atlas-hook-regenerate)
      - [atlas hook export](#atlas-hook-export)
   - [atlas checkpoint](#atlas-checkpoint)
   - [atlas backlog](#atlas-backlog)
      - [atlas backlog add](#atlas-backlog-add)
      - [atlas backlog list](#atlas-backlog-list)
      - [atlas backlog view](#atlas-backlog-view)
      - [atlas backlog promote](#atlas-backlog-promote)
      - [atlas backlog dismiss](#atlas-backlog-dismiss)
      - [AI Agent Discovery Protocol](#ai-agent-discovery-protocol)
   - [atlas cleanup](#atlas-cleanup)
   - [atlas upgrade](#atlas-upgrade)
   - [atlas config](#atlas-config)
      - [atlas config show](#atlas-config-show)
      - [atlas config ai](#atlas-config-ai)
      - [atlas config validation](#atlas-config-validation)
      - [atlas config notifications](#atlas-config-notifications)
   - [atlas workspace](#atlas-workspace)
      - [atlas workspace list](#atlas-workspace-list)
      - [atlas workspace destroy](#atlas-workspace-destroy)
      - [atlas workspace close](#atlas-workspace-close)
      - [atlas workspace logs](#atlas-workspace-logs)
   - [atlas completion](#atlas-completion)
6. [Templates](#templates)
   - [Custom Templates](#custom-templates)
7. [Task States](#task-states)
8. [Workflows](#workflows)
   - [Bugfix Workflow](#bugfix-workflow)
   - [Feature Workflow (Speckit SDD)](#feature-workflow-speckit-sdd)
   - [Task Workflow](#task-workflow)
   - [Fix Workflow](#fix-workflow)
   - [Hotfix Workflow](#hotfix-workflow)
   - [Parallel Features](#parallel-features)
   - [Error Recovery Workflow](#error-recovery-workflow)
9. [Configuration](#configuration)
   - [Configuration Locations](#configuration-locations)
   - [Configuration Precedence](#configuration-precedence)
   - [Environment Variables](#environment-variables)
   - [Configuration File Reference](#configuration-file-reference)
10. [File Structure](#file-structure)
   - [ATLAS Home Directory](#atlas-home-directory)
   - [Git Worktree Location](#git-worktree-location)
   - [Browsing Examples](#browsing-examples)
11. [Troubleshooting](#troubleshooting)
   - [Common Issues](#common-issues)
   - [Debugging](#debugging)
   - [Getting Help](#getting-help)

<br>

## Quick Start

Get running in 2 minutes:

```bash
# 1. Check prerequisites
go version        # Need 1.24+
git --version     # Need 2.20+
gh --version      # Need 2.20+
claude --version  # Need 2.0.76+ (if using claude agent)
gemini --version  # Need 0.22.5+ (if using gemini agent)
codex --version   # Need 0.77.0+ (if using codex agent)

# 2. Install ATLAS
go install github.com/mrz1836/atlas@latest

# 3. Initialize (one-time setup)
atlas init

# 4. Run your first task
cd /path/to/your/go/project
atlas start "this is just a test task. say hi!" --template task

# 5. Monitor progress
atlas status --watch

# 6. Approve when ready
atlas approve
```

<br>

## Prerequisites

| Tool | Required Version | Purpose | Installation |
|------|------------------|---------|--------------|
| **Go** | 1.24+ | Runtime | [go.dev](https://go.dev/dl/) or `brew install go` |
| **Git** | 2.20+ | Version control | `brew install git` |
| **GitHub CLI (gh)** | 2.20+ | PR operations | `brew install gh` |
| **Claude CLI** | 2.0.76+ | AI execution (Claude) | [Claude Code](https://docs.anthropic.com/en/docs/claude-code) or `npm install -g @anthropic-ai/claude-code` |
| **Gemini CLI** | 0.22.5+ | AI execution (Gemini) | `npm install -g @google/gemini-cli` |
| **Codex CLI** | 0.77.0+ | AI execution (OpenAI) | `npm install -g @openai/codex` |
| **uv** | 0.5.x | Python tool runner (for Speckit) | `brew install uv` |

**Note:** At least one AI CLI (Claude, Gemini, or Codex) is required. Install based on your preferred AI provider.

ATLAS manages additional tools automatically:
- **mage-x** (magex command) - Standardized build/test toolkit
- **go-pre-commit** - Pre-commit hooks
- **Speckit** - Specification-driven development framework

<br>

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

<br>

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

<br>

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

<br>

### atlas start

Start a new task with the given description.

```bash
# Basic usage - interactive template selection
atlas start "fix null pointer in parseConfig"

# Specify template
atlas start "fix null pointer" --template bugfix

# Custom workspace name & agent + model
atlas start "do this thing..." -t task -w task-workspace --agent claude --model opus

# Use a specific branch as the source (uses remote by default for safety)
atlas start "add logging" -t task --branch develop

# Use local branch explicitly (when you have local changes you want to include)
atlas start "continue work" -t task --branch develop --use-local

# Fix issues on an existing branch (e.g., a branch already in a PR)
atlas start "fix lint errors" --template hotfix --target feat/my-feature

# Enable/disable AI verification
atlas start "simple edit" -t task --verify
atlas start "simple edit" -t task --no-verify

# Non-interactive mode (requires --template)
atlas start "fix bug" -t bugfix --no-interactive

# Dry-run mode - see what would happen without making changes
atlas start "fix null pointer" --template bugfix --dry-run

# Dry-run with JSON output for scripting
atlas start "fix null pointer" --template bugfix --dry-run --output json
```

**Flags:**

| Flag | Short | Description | Values |
|------|-------|-------------|--------|
| `--template` | `-t` | Template to use | `bugfix`, `feature`, `task`, `commit`, `hotfix` |
| `--workspace` | `-w` | Custom workspace name | Any string (sanitized) |
| `--agent` | `-a` | AI agent/CLI to use | `claude`, `gemini`, `codex` |
| `--model` | `-m` | AI model to use | Claude: `sonnet`, `opus`, `haiku`; Gemini: `flash`, `pro`; Codex: `codex`, `max`, `mini` |
| `--branch` | `-b` | Base branch to create workspace from (fetches from remote by default) | Branch name |
| `--target` | | Existing branch to checkout and work on (skips new branch creation, mutually exclusive with `--branch`) | Branch name |
| `--use-local` | | Prefer local branch over remote when both exist | |
| `--verify` | | Enable AI verification step | |
| `--no-verify` | | Disable AI verification step | |
| `--no-interactive` | | Disable interactive prompts | |
| `--dry-run` | | Show what would happen without executing | |

**Dry-Run Mode:**

The `--dry-run` flag shows what would happen without making any changes. It's useful for:
- Previewing the workflow steps before execution
- Validating template and flag combinations
- Scripting and automation with `--output json`

Example output:
```
=== DRY-RUN MODE ===
Showing what would happen without making changes.

[0/9] Workspace Creation
      Name:   fix-null-pointer
      Branch: fix/fix-null-pointer
      Status: WOULD CREATE

[1/9] ai Step: 'analyze'
      Would:
        - Execute AI with model: claude-sonnet-4-20250514
        - Prompt: "fix null pointer in parseConfig"
...

=== Summary ===
Template: bugfix
Steps: 9 total
Side Effects Prevented:
  - Workspace creation (git worktree)
  - AI execution (file modifications)
  - Git commits
  - Git push to remote
  - Pull request creation

Run without --dry-run to execute.
```

**Workspace Naming:**
- Auto-generated from description (lowercase, hyphens, max 50 chars)
- Override with `--workspace <name>`
- Branch format: `<prefix>/<workspace-name>` (e.g., `fix/null-pointer-config`)

<br>

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

<br>

### atlas approve

Approve a completed task awaiting approval.

```bash
# Interactive selection (if multiple pending)
atlas approve

# Approve specific workspace
atlas approve my-workspace

# Skip interactive menu
atlas approve --auto-approve

# Approve and close workspace (removes worktree, keeps history)
atlas approve my-workspace --close

# Custom message for approve+merge operations
atlas approve my-workspace --message "Merged by CI pipeline"

# JSON output for scripting
atlas approve my-workspace --output json
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--auto-approve` | Skip interactive menu, approve directly |
| `--close` | Also close the workspace after approval (removes worktree, preserves history) |
| `--message` | Custom message for approve+merge (overrides `approval.merge_message` config) |

**Interactive Options:**
- Approve and complete
- Approve and close workspace (removes worktree)
- **Approve + Merge + Close** (review PR, squash merge, close workspace)
- View git diff
- View execution logs
- Open PR in browser
- Reject task
- Cancel

The **Approve + Merge + Close** option performs:
1. Adds a PR review (APPROVE) or comment if reviewing own PR
2. Merges the PR using squash merge (with admin bypass if needed)
3. Approves the task in ATLAS
4. Closes the workspace

<br>

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

<br>

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

**Graceful Shutdown (Ctrl+C):**

Both `atlas start` and `atlas resume` support graceful shutdown via Ctrl+C:

```bash
# Start a task
atlas start "fix bug" --template bugfix
# Press Ctrl+C at any time...

# Output:
# ⚠ Interrupt received - saving state...
# ✓ Task state saved
# ─────────────────────────────────────────
# 📁 Workspace:    fix-bug
# 📁 Worktree:     /path/to/worktree
# 📋 Task:         task-550e8400-e29b-41d4-a716-446655440000
# 📊 Status:       interrupted
# ⏸ Stopped at:    Step 3/7 (validate)
#
# ▶ To resume:  atlas resume fix-bug

# Resume later
atlas resume fix-bug
```

When you press Ctrl+C:
- Task state is saved immediately (status changes to `interrupted`)
- Workspace is paused but preserved
- All progress and artifacts are retained
- You can resume exactly where you left off

<br>

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

<br>

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

<br>

### atlas validate

Run the full validation suite.

```bash
# Run all validation commands
atlas validate

# Quiet mode - show only final pass/fail
atlas validate --quiet
```

**Execution Order:**
1. Pre-commit (sequential)
2. Format (sequential)
3. Lint + Test (parallel)

**Default Commands:**
- `go-pre-commit run --all-files`
- `magex format:fix`
- `magex lint`
- `magex test`

<br>

### atlas format

Run code formatters.

```bash
atlas format
```

**Default Command:** `magex format:fix`

<br>

### atlas lint

Run linters.

```bash
atlas lint
```

**Default Command:** `magex lint`

<br>

### atlas test

Run tests.

```bash
atlas test
```

**Default Command:** `magex test`

<br>

### atlas hook

Manage task recovery hooks. The hook system provides crash-resistant context persistence—when Claude Code crashes mid-task, you can resume exactly where you left off.

```bash
# View current hook state
atlas hook status

# List all checkpoints for current task
atlas hook checkpoints

# Verify a validation receipt's cryptographic signature
atlas hook verify-receipt rcpt-00000001

# Regenerate HOOK.md from hook.json (if corrupted)
atlas hook regenerate

# Export full hook state as JSON for debugging
atlas hook export > hook-debug.json

# Show instructions for installing git hooks
atlas hook install
```

#### atlas hook status

Display the current hook state for the active workspace.

```bash
atlas hook status

# Output:
# Hook State: step_running
# Task: task-20260117-143022 (fix-null-pointer)
# Step: implement (3/7), Attempt 2/3
# Last Updated: 2 minutes ago
# Last Checkpoint: ckpt-a1b2c3d4 (git_commit, 5 min ago)
```

**Exit Codes:**
- `0`: Success
- `1`: No active hook found
- `2`: Hook in error state

#### atlas hook checkpoints

List all checkpoints for the current task.

```bash
atlas hook checkpoints

# Output:
# | Time     | Trigger       | Description                      |
# |----------|---------------|----------------------------------|
# | 14:42:15 | git_commit    | Added nil check for Server field |
# | 14:38:22 | step_complete | Plan complete                    |
```

Checkpoints are created automatically on git commits, validation passes, step completions, and periodically during long-running steps.

#### atlas hook verify-receipt

Verify the cryptographic signature of a validation receipt.

```bash
atlas hook verify-receipt rcpt-00000001

# Output:
# Receipt: rcpt-00000001
# Step: analyze
# Command: magex lint
# Exit Code: 0
# Signature: VALID ✓
```

Validation receipts are signed proofs that validation actually ran, preventing scenarios where the AI claims validation passed without running it.

#### atlas hook regenerate

Regenerate HOOK.md from hook.json if the markdown file gets corrupted.

```bash
atlas hook regenerate
```

#### atlas hook export

Export the full hook state as JSON for debugging.

```bash
atlas hook export > hook-debug.json
```

<br>

### atlas checkpoint

Create a manual checkpoint for the current task.

```bash
# Create a checkpoint with description
atlas checkpoint "halfway through refactor"

# Output:
# Created checkpoint ckpt-e5f6g7h8: halfway through refactor

# Specify a trigger type (for automation)
atlas checkpoint --trigger git_commit "Post-commit checkpoint"
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--trigger` | Checkpoint trigger type | `manual` |

**Trigger Types:**
- `manual` - User-initiated checkpoint (default)
- `git_commit` - Triggered by git post-commit hook
- `git_push` - Triggered by git post-push hook
- `pr_created` - Triggered after PR creation
- `validation` - Triggered after validation pass
- `step_complete` - Triggered on step completion
- `interval` - Triggered by interval timer

**Exit Codes:**
- `0`: Checkpoint created successfully
- `1`: No active task found
- `2`: Failed to create checkpoint

<br>

### atlas backlog

Manage the work backlog for capturing issues discovered during AI-assisted development. The backlog provides a lightweight, project-local queue that prevents good observations from getting lost.

**Storage**: Discoveries are stored as individual YAML files in `.atlas/backlog/` directory, enabling zero merge conflicts on concurrent adds.

#### atlas backlog add

Add a new discovery to the backlog.

```bash
# Interactive mode (for humans) - launches guided form
atlas backlog add

# Flag mode (for AI/scripts)
atlas backlog add "Missing error handling" \
  --file main.go \
  --line 47 \
  --category bug \
  --severity high \
  --description "Detailed explanation" \
  --tags "error-handling,config"

# JSON output for scripting
atlas backlog add "Issue title" --category bug --severity low --json
```

**Flags:**

| Flag | Short | Description | Values |
|------|-------|-------------|--------|
| `--file` | `-f` | File path where issue was found | Relative path |
| `--line` | `-l` | Line number in file | Positive integer |
| `--category` | `-c` | Issue category | `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation` |
| `--severity` | `-s` | Priority level | `low`, `medium`, `high`, `critical` |
| `--description` | `-d` | Detailed explanation | String |
| `--tags` | `-t` | Comma-separated labels | String |
| `--json` | | Output created discovery as JSON | Flag |

**Output:**
```
Created discovery: disc-a1b2c3
  Title: Missing error handling
  Category: bug | Severity: high
  Location: main.go:47
```

#### atlas backlog list

List discoveries in the backlog.

```bash
# List all pending discoveries (default)
atlas backlog list

# Filter by status
atlas backlog list --status pending
atlas backlog list --status promoted
atlas backlog list --status dismissed

# Filter by category
atlas backlog list --category bug
atlas backlog list --category security

# Show all (including dismissed)
atlas backlog list --all

# Limit results
atlas backlog list --limit 10

# JSON output for scripting
atlas backlog list --json
```

**Flags:**

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--status` | | Filter by status | `pending` |
| `--category` | `-c` | Filter by category | All |
| `--all` | `-a` | Include dismissed items | `false` |
| `--limit` | `-n` | Maximum items to show | Unlimited |
| `--json` | | Output as JSON array | `false` |

**Output:**
```
ID           TITLE                           CATEGORY  SEVERITY  AGE
disc-a1b2c3  Missing error handling          bug       high      2h
disc-x9y8z7  Potential race condition        bug       critical  1d
disc-p4q5r6  Add test for edge case          testing   medium    3d
```

#### atlas backlog view

View full details of a discovery.

```bash
atlas backlog view disc-a1b2c3

# JSON output
atlas backlog view disc-a1b2c3 --json
```

**Output:**
```
Discovery: disc-a1b2c3
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Title:      Missing error handling
Status:     pending
Category:   bug
Severity:   high

Description:
  The Parse function doesn't handle the case where the config file
  exists but is empty. This causes a nil pointer panic at line 52.

Location:   config/parser.go:47
Tags:       config, error-handling

Discovered: 2026-01-18 14:32:15 UTC
By:         ai:claude-code:claude-sonnet-4
During:     task-20260118-143022
Git:        feat/auth-refactor @ 7b3f1a2
```

#### atlas backlog promote

Promote a discovery to an ATLAS task. Automatically generates task configuration from the discovery's category, severity, and content.

```bash
# Auto-generate task configuration from discovery (deterministic mapping)
atlas backlog promote disc-a1b2c3

# Dry-run: preview what would happen without executing
atlas backlog promote disc-a1b2c3 --dry-run

# With AI-assisted analysis for optimal task configuration
atlas backlog promote disc-a1b2c3 --ai

# Override template selection (bugfix, feature, task, hotfix)
atlas backlog promote disc-a1b2c3 --template feature

# Override AI agent and model
atlas backlog promote disc-a1b2c3 --ai --agent claude --model opus

# Legacy mode: link to existing task ID
atlas backlog promote disc-a1b2c3 --task-id task-20260118-150000

# JSON output for scripting
atlas backlog promote disc-a1b2c3 --dry-run --json
```

**Flags:**

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--template` | `-t` | Override template selection | Auto from category |
| `--ai` | | Use AI-assisted analysis | `false` |
| `--agent` | | AI agent override (claude, gemini, codex) | From config |
| `--model` | | AI model override | From config |
| `--dry-run` | | Preview without executing | `false` |
| `--task-id` | | Legacy: link to existing task ID | - |
| `--json` | | Output as JSON | `false` |

**Category → Template Mapping:**

| Category | Severity | Template |
|----------|----------|----------|
| security | critical | hotfix |
| bug | any | bugfix |
| security | non-critical | bugfix |
| performance | any | task |
| maintainability | any | task |
| testing | any | task |
| documentation | any | task |

**Output (Dry-Run):**
```
Dry-run: Promote discovery disc-a1b2c3
  Title:     Missing error handling in payment processor
  Category:  bug
  Severity:  high

Would create task with:
  Template:   bugfix
  Workspace:  missing-error-handling-payment
  Branch:     fix/missing-error-handling-payment
  Description: Missing error handling in payment processor [HIGH]
               Category: bug
               The API endpoint doesn't handle network failures properly
               Location: cmd/api.go:47
```

**Output (Execute):**
```
Promoted discovery disc-a1b2c3
  Template:   bugfix
  Workspace:  missing-error-handling-payment
  Branch:     fix/missing-error-handling-payment

To start the task: atlas start "..." --workspace missing-error-handling-payment
```

#### atlas backlog dismiss

Dismiss a discovery with a reason.

```bash
# Dismiss with reason
atlas backlog dismiss disc-a1b2c3 --reason "Duplicate of disc-x9y8z7"

# JSON output
atlas backlog dismiss disc-a1b2c3 --reason "Won't fix" --json
```

**Flags:**

| Flag | Description | Required |
|------|-------------|----------|
| `--reason` | Explanation for dismissal | Yes |

**Output:**
```
Dismissed discovery disc-a1b2c3
  Reason: Duplicate of disc-x9y8z7
```

#### AI Agent Discovery Protocol

For AI agents (Claude Code, Gemini, Codex), add to your CLAUDE.md or equivalent:

```markdown
**Discovery Protocol**: If you see an issue outside your current task scope, DO NOT ignore it.
Run `atlas backlog add "<Title>" --file <path> --category <type> --severity <level>` immediately.
Then continue your task.
```

This ensures no good observations get lost during automated development.

<br>

### atlas cleanup

Clean up old task artifacts and hook files based on retention policies.

```bash
# Clean up all old artifacts
atlas cleanup

# Preview what would be deleted (dry run)
atlas cleanup --dry-run

# Only clean up old hook files
atlas cleanup --hooks

# Dry run for hooks only
atlas cleanup --hooks --dry-run
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what would be deleted without removing files |
| `--hooks` | Only clean up hook files (skip other artifact cleanup) |

**Hook Retention Policy:**

| Task State | Retention |
|------------|-----------|
| Completed | 30 days |
| Failed | 7 days |
| Abandoned | 7 days |

**Exit Codes:**
- `0`: Cleanup completed successfully
- `1`: Cleanup failed

<br>

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
atlas upgrade magex
atlas upgrade go-pre-commit
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--check` | `-c` | Dry run - show updates without installing |
| `--yes` | `-y` | Skip confirmation prompts |

**Managed Tools:**
- mage-x (magex command)
- go-pre-commit
- Speckit

<br>

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
- AI agent (claude, gemini, codex)
- Default model (claude: sonnet, opus, haiku; gemini: flash, pro; codex: codex, max, mini)
- API key environment variables per provider
- Timeout duration
- Max agentic turns

#### atlas config validation

Configure validation commands.

```bash
# Interactive configuration
atlas config validation

# Show current values without prompting
atlas config validation --no-interactive
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
# Interactive configuration
atlas config notifications

# Show current values without prompting
atlas config notifications --no-interactive
```

**Configurable Settings:**
- Bell notifications (on/off)
- Events that trigger notifications

<br>

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
- Status (active, paused, closed)
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

#### atlas workspace close

Close a workspace (removes worktree, preserves history).

```bash
# With confirmation prompt
atlas workspace close my-workspace

# Skip confirmation
atlas workspace close my-workspace --force
```

**Use Case:** When done with a workspace but want to keep the history for reference.

#### atlas workspace logs

View workspace task execution logs.

```bash
# View all logs
atlas workspace logs my-workspace

# Stream logs (follow mode)
atlas workspace logs my-workspace --follow

# Filter by step name
atlas workspace logs my-workspace --step validate

# Filter by task ID
atlas workspace logs my-workspace --task task-550e8400-e29b-41d4-a716-446655440001

# Show last N lines
atlas workspace logs my-workspace --tail 50

# Combined filters
atlas workspace logs my-workspace --follow --step validate --tail 100
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--follow` | `-f` | Stream logs as they appear |
| `--step` | | Filter by step name |
| `--task` | | Filter by task ID |
| `--tail` | `-n` | Show last N lines |

**Log Features:**
- Color-coded by log level (info/warn/error/debug)
- JSON-lines format
- Filterable by step and task

<br>

### atlas completion

Generate and install shell completion scripts for atlas commands.

```bash
# Generate completion script for your shell
atlas completion bash
atlas completion zsh
atlas completion fish
atlas completion powershell

# Auto-install completions (detects shell automatically)
atlas completion install

# Install for a specific shell
atlas completion install --shell zsh
atlas completion install --shell bash
atlas completion install --shell fish
```

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `bash` | Generate bash completion script |
| `zsh` | Generate zsh completion script |
| `fish` | Generate fish completion script |
| `powershell` | Generate powershell completion script |
| `install` | Auto-install completions to appropriate location |

**Install Flags:**

| Flag | Description |
|------|-------------|
| `--shell` | Shell to install completions for (zsh, bash, fish) |

**Installation Locations:**
- **zsh**: `~/.zsh/completions/_atlas` (updates `.zshrc` with fpath)
- **bash**: `~/.bash_completion.d/atlas` (updates `.bashrc` with sourcing)
- **fish**: `~/.config/fish/completions/atlas.fish` (auto-loaded)

**Manual Installation:**

```bash
# Zsh - add to ~/.zshrc
source <(atlas completion zsh)

# Bash - add to ~/.bashrc
source <(atlas completion bash)

# Fish
atlas completion fish | source
```

<br>

## Templates

ATLAS provides pre-defined workflow templates:

| Template | Description | Branch Prefix | Use Case |
|----------|-------------|---------------|----------|
| **bugfix** | Analyze → Implement → Validate → Commit → PR | `fix` | Bug fixes requiring analysis |
| **feature** | Speckit SDD: Specify → Plan → Tasks → Implement → Validate → PR | `feat` | New features with specifications |
| **task** | Implement → (Verify) → Validate → Commit → PR | `task` | Simple, well-defined tasks |
| **fix** | Scan → Fix → Validate → Commit → PR | `fix` | Automated issue discovery and fixing |
| **hotfix** | Fix issues on existing branch → Validate → Commit → Push (no PR) | `hotfix` | Quick fixes on branches already in PRs |
| **commit** | Smart commits: Garbage detection, logical grouping, message generation | `chore` | Commit assistance |

**Template Details:**

- **bugfix**: Best for issues requiring investigation. Includes an `analyze` step to understand the problem before implementing.
- **feature**: Full Speckit SDD workflow with specification, planning, and task breakdown. Ideal for complex features.
- **task**: Fastest workflow for straightforward changes. Skips analysis and goes straight to implementation. Add `--verify` for optional AI cross-validation.
- **fix**: Automated issue discovery. Runs validation commands to find lint/format/test issues, then fixes them. Best for codebase maintenance.
- **hotfix**: Designed for fixing issues on branches that are already in PRs. Uses `--target` flag to checkout an existing branch, makes fixes, and pushes directly to that branch. No new branch or PR is created - the existing PR automatically receives the new commits.
- **commit**: Specialized template for creating intelligent commits from existing changes.

**Utility Templates:**
- `format` - Run code formatting only
- `lint` - Run linters only
- `test` - Run tests only
- `validate` - Full validation suite

### Custom Templates

ATLAS supports custom templates defined in YAML or JSON files. Custom templates can extend or override built-in templates.

**Configuring Custom Templates:**

Add custom templates to your `.atlas/config.yaml`:

```yaml
templates:
  default_template: "my-workflow"  # Optional: set your custom template as default
  custom_templates:
    my-workflow: /path/to/my-workflow.yaml
    deploy: ./templates/deploy.json  # JSON also supported
    hotfix: ./templates/hotfix.yml   # Relative paths supported
```

File format is auto-detected from extension (`.yaml`, `.yml`, or `.json`).

**Creating and Running a Custom Template:**

Follow these steps to create and use a custom template:

**Step 1: Create the template file**

Create a new file at `.atlas/templates/my-workflow.yaml`:

```yaml
name: my-workflow
description: Custom workflow with CI monitoring
branch_prefix: custom
default_model: sonnet

steps:
  - name: implement
    type: ai
    description: Implement the requested changes
    required: true
    timeout: 20m

  - name: validate
    type: validation
    required: true
    timeout: 10m

  - name: git_commit
    type: git
    config:
      operation: commit

  - name: git_push
    type: git
    config:
      operation: push

  - name: git_pr
    type: git
    config:
      operation: create_pr

  - name: ci_wait
    type: ci
    description: Wait for CI to pass
    timeout: 30m
    config:
      poll_interval: 2m

  - name: review
    type: human
    description: Human approval checkpoint
```

**Step 2: Register the template**

Add to your `.atlas/config.yaml`:

```yaml
templates:
  custom_templates:
    my-workflow: .atlas/templates/my-workflow.yaml
```

**Step 3: Run with atlas start**

```bash
# Use the custom template
atlas start "add logging to HTTP client" --template my-workflow

# Or set as default and omit --template
# templates:
#   default_template: my-workflow
atlas start "add logging to HTTP client"
```

**Step 4: Monitor and approve**

```bash
atlas status --watch
# Wait for awaiting_approval...
atlas approve
```

**Custom Template File Format (YAML):**

```yaml
name: my-workflow
description: A custom workflow template
branch_prefix: custom
default_model: sonnet

# Optional: Enable/disable verification
verify: false
verify_model: opus  # Model for cross-validation (different family)

# Optional: Template variables
variables:
  ticket_id:
    description: JIRA ticket ID
    required: true
  component:
    description: Component name
    default: core
    required: false

steps:
  - name: implement
    type: ai
    description: Implement the requested changes
    required: true
    timeout: 20m
    retry_count: 3
    config:
      permission_mode: default
      prompt_template: implement_task

  - name: validate
    type: validation
    description: Run format, lint, and test commands
    required: true
    timeout: 10m

  - name: git_commit
    type: git
    required: true
    timeout: 1m
    config:
      operation: commit

  - name: git_push
    type: git
    required: true
    timeout: 2m
    config:
      operation: push

  - name: git_pr
    type: git
    required: true
    timeout: 2m
    config:
      operation: create_pr

  - name: ci_wait
    type: ci
    description: Wait for CI pipeline to complete
    required: true
    timeout: 30m
    config:
      poll_interval: 2m
      workflows: []  # Empty = monitor all workflows

  - name: review
    type: human
    description: Human approval checkpoint
    required: true
```

**Step Types:**

| Type | Purpose |
|------|---------|
| `ai` | AI-powered code generation/modification |
| `validation` | Format, lint, and test execution |
| `git` | Git operations (commit, push, PR, merge, review, comment) |
| `human` | Human approval/intervention |
| `sdd` | Speckit spec-driven development |
| `ci` | CI pipeline monitoring |
| `verify` | AI cross-model verification |
| `loop` | Iterative execution with exit conditions |

**Loop Step Configuration:**

The `loop` step type executes inner steps repeatedly until an exit condition is met. It supports count-based, condition-based, and AI signal-based termination with circuit breakers for safety.

```yaml
steps:
  - name: iterative_fix
    type: loop
    description: Iteratively fix issues until all pass
    timeout: 1h
    config:
      # Iteration control (pick one mode)
      max_iterations: 10          # Hard cap on iterations
      until: "all_tests_pass"     # Exit when condition is true
      until_signal: true          # Exit when AI outputs {"exit": true}

      # Exit conditions (for signal mode - dual-gate pattern)
      exit_conditions:
        - "all tests passing"
        - "no lint errors"

      # Circuit breakers (safety)
      circuit_breaker:
        stagnation_iterations: 3  # Stop if no files changed for 3 iterations
        consecutive_errors: 5     # Stop on repeated failures

      # Context management
      fresh_context: true         # New AI context per iteration
      scratchpad_file: "loop-progress.json"  # Cross-iteration memory

      # Inner steps to execute each iteration
      steps:
        - name: fix
          type: ai
          config:
            prompt_template: analyze_and_fix

        - name: validate
          type: validation
```

| Config Key | Description | Default |
|------------|-------------|---------|
| `max_iterations` | Maximum number of iterations | Required if no other exit |
| `until` | Built-in condition name (`all_tests_pass`, `validation_passed`, `no_changes`) | - |
| `until_signal` | Exit when AI outputs `{"exit": true}` | `false` |
| `exit_conditions` | Patterns that must appear in output for signal exit | `[]` |
| `circuit_breaker.stagnation_iterations` | Stop after N iterations with no file changes | Disabled |
| `circuit_breaker.consecutive_errors` | Stop after N consecutive failures | `5` |
| `fresh_context` | Spawn new AI context per iteration | `false` |
| `scratchpad_file` | JSON file for cross-iteration memory | - |
| `steps` | Inner steps to execute each iteration | Required |

**CI Step Configuration:**

The `ci` step type monitors GitHub Actions workflows and waits for them to complete. It's typically used after creating a PR to ensure CI passes before human review.

| Config Key | Description | Default |
|------------|-------------|---------|
| `poll_interval` | How often to check CI status | `2m` |
| `grace_period` | Initial wait before first poll | `2m` |
| `timeout` | Maximum wait time | `30m` |
| `workflows` | Specific workflows to monitor (empty = all) | `[]` |

**Example ci_wait step:**

```yaml
steps:
  - name: ci_wait
    type: ci
    description: Wait for CI pipeline to pass
    required: true
    timeout: 30m
    config:
      poll_interval: 2m
      grace_period: 2m
      workflows:
        - "CI / lint"
        - "CI / test"
```

**Note:** The CI step requires a PR to have been created. It reads the PR number from task metadata set by a previous `git_pr` step.

**Git Step Operations:**

The `git` step type supports the following operations via the `operation` config key:

| Operation | Description | Config Options |
|-----------|-------------|----------------|
| `commit` | Create a smart commit | — |
| `push` | Push to remote | — |
| `create_pr` | Create a pull request | `base_branch`, `branch` |
| `merge_pr` | Merge a pull request | `pr_number`, `merge_method` (squash/merge/rebase), `admin_bypass`, `delete_branch` |
| `add_pr_review` | Add a PR review | `pr_number`, `event` (APPROVE/REQUEST_CHANGES/COMMENT), `body` |
| `add_pr_comment` | Add a PR comment | `pr_number`, `body` |

**Note:** For `merge_pr`, `add_pr_review`, and `add_pr_comment`, the `pr_number` can be omitted if a previous `create_pr` step stored it in task metadata.

**Example using some git operations:**

```yaml
steps:
  - name: git_pr
    type: git
    config:
      operation: create_pr

  - name: approve_pr
    type: git
    config:
      operation: add_pr_review
      event: APPROVE
      body: "LGTM! Automated approval by ATLAS."

  - name: merge_pr
    type: git
    config:
      operation: merge_pr
      merge_method: squash
      delete_branch: true  # Optional: delete source branch after merge
```

**Template Variables:**

Use `{{variable}}` syntax in descriptions and prompts for dynamic customization:

```yaml
description: "Implement {{ticket_id}}: {{component}} changes"
```

**Override Built-in Templates:**

Custom templates with the same name as built-in templates will override them:

```yaml
# In config.yaml
templates:
  custom_templates:
    bugfix: ./templates/my-bugfix.yaml  # Overrides built-in bugfix
```

<br>

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
     │                      ▼                                       ▼
     │                 gh_failed                          ci_failed / ci_timeout
     │                      │                                       │
     │                      │◄──────── retry ───────────────────────┤
     │                      │                                       │
     │                      └──────── abandon ──────────────────────┴──► abandoned
     │
     ├── Ctrl+C ───────────────────────────────────────────────────►  interrupted
     │                                                                    │
     │                                                                    │
     ▼                                                                    │
  validating ◄────────────────────────── resume ──────────────────────────┘
     │
     ├── pass ────────────────────────────────────────────────────►  awaiting_approval
     │                                                                    │
     ├── Ctrl+C ──────────────────────────────────────────────────►  interrupted
     │                                                                    │
     └── fail ────────────────────────────────────────────────────►  validation_failed

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

  interrupted
     │
     ├── resume ───────────────────────────────────────────────────►  running
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
- `interrupted` - User pressed Ctrl+C (resumable)
- `gh_failed` - GitHub operations failed
- `ci_failed` - CI pipeline failed
- `ci_timeout` - CI exceeded timeout

<br>

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

### Fix Workflow

The **fix** template discovers and fixes validation issues automatically:

```bash
# Scan and fix any issues in the codebase
atlas start "Fix any issues" -t fix

# With specific workspace and branch
atlas start "Fix any issues" -t fix -w test-ws --branch master --verbose

# Monitor progress
atlas status --watch

# Steps:
# 1. scan - Run validations, identify all issues (lint errors, test failures, etc.)
# 2. fix - AI fixes all identified issues
# 3. validate - Confirm fixes work
# 4. git_commit → git_push → git_pr → ci_wait → review

# If no issues found, task completes quickly (no PR created)
```

**When to use Fix vs Bugfix:**
- **Fix**: No known issue - discover problems via validation commands ("Fix any issues", "Clean up the codebase")
- **Bugfix**: Known issue from user description ("fix null pointer in parseConfig")

### Hotfix Workflow

The **hotfix** template is designed for fixing issues on branches that are already in pull requests. Instead of creating a new branch, it checks out the existing branch, makes fixes, and pushes directly to that branch.

```bash
# Branch feat/my-feature has linter issues and already has a PR open
atlas start "fix lint errors" --template hotfix --target feat/my-feature

# Monitor progress
atlas status --watch

# Steps:
# 1. detect - Optional: Run validation to identify issues
# 2. fix - AI fixes the identified issues
# 3. validate - Confirm fixes work
# 4. git_commit - Commit changes to the existing branch
# 5. git_push - Push to origin (the existing PR receives the commits)
#
# NO git_pr, ci_wait, or review steps - the PR already exists!

# After completion, the existing PR automatically shows the new commits
```

**Key Features:**
- **Uses existing branch**: Requires `--target` flag to specify the branch to checkout
- **No new PR**: Pushes directly to the target branch
- **Isolated worktree**: Still uses worktree isolation for safe development
- **Fast workflow**: Skips PR creation, CI wait, and review steps
- **Best for**: Fixing lint errors, test failures, or other issues on branches already in PRs

**When to use Hotfix:**
- Branch already has an open PR with failing CI
- Need to quickly patch lint/format/test issues
- Want commits to go to the same branch (not create a new one)

**Hotfix vs Fix:**
- **Hotfix**: Works on an EXISTING branch with `--target`, no new PR
- **Fix**: Creates a NEW branch from base, opens a new PR

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

<br>

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
  # AI agent/CLI to use: "claude", "gemini", or "codex"
  # Default: "claude"
  agent: claude

  # AI model to use
  # Claude: "sonnet", "opus", or "haiku"
  # Gemini: "flash" or "pro"
  # Codex: "codex", "max", or "mini"
  # Default: "sonnet" for claude, "flash" for gemini, "codex" for codex
  model: sonnet

  # Environment variable names containing API keys per provider
  # You can override the default env var for each provider
  # Defaults: claude=ANTHROPIC_API_KEY, gemini=GEMINI_API_KEY, codex=OPENAI_API_KEY
  api_key_env_vars:
    claude: ANTHROPIC_API_KEY
    gemini: GEMINI_API_KEY
    codex: OPENAI_API_KEY

  # Maximum duration for AI operations (e.g., "30m", "1h")
  # Default: 30m
  timeout: 30m

  # DEPRECATED: Maximum conversation turns (use max_budget_usd instead)
  # Will be removed in v2.0
  # Default: 10
  max_turns: 10

  # Maximum dollar amount for AI operations; 0 = unlimited
  # Default: 0
  max_budget_usd: 0

#------------------------------------------------------------------------------
# Git Configuration
#------------------------------------------------------------------------------
git:
  # Default base branch for creating feature branches
  # Default: "main"
  base_branch: main

  # Enable automatic git operations without user confirmation
  # Default: true
  auto_proceed_git: true

  # Name of the remote repository
  # Default: "origin"
  remote: origin

#------------------------------------------------------------------------------
# Worktree Configuration
#------------------------------------------------------------------------------
worktree:
  # Base directory where worktrees are created; empty = default location
  # Default: ""
  base_dir: ""

  # Suffix appended to worktree directory names
  # Default: ""
  naming_suffix: ""

#------------------------------------------------------------------------------
# Templates Configuration
#------------------------------------------------------------------------------
templates:
  # Name of the default template when none is specified
  # Default: "task"
  default_template: task

  # Map of custom template names to their file paths
  # Example: { "my-template": "/path/to/template.yaml" }
  # Default: {}
  custom_templates: {}

  # Optional: Override branch prefixes for templates
  # branch_prefixes:
  #   bugfix: fix
  #   feature: feat
  #   commit: chore

#------------------------------------------------------------------------------
# CI Configuration
#------------------------------------------------------------------------------
ci:
  # How often to check CI status (e.g., "30s", "2m")
  # Default: 2m
  poll_interval: 2m

  # Initial grace period before starting to poll CI
  # Default: 2m
  grace_period: 2m

  # Maximum duration to wait for CI completion
  # Default: 30m
  timeout: 30m

  # List of CI workflow names that must pass; empty = all workflows
  # Example: ["build", "test", "lint"]
  # Default: []
  required_workflows: []

#------------------------------------------------------------------------------
# Validation Configuration
#------------------------------------------------------------------------------
validation:
  # Maximum duration for each validation command
  # Default: 5m
  timeout: 5m

  # Run validation commands in parallel
  # Default: true
  parallel_execution: true

  # Enable AI-assisted retry when validation fails
  # Default: true
  ai_retry_enabled: true

  # Maximum number of AI retry attempts
  # Default: 3
  max_ai_retry_attempts: 3

  # Validation commands to run at various stages
  commands:
    # Commands that format code
    format:
      - magex format:fix

    # Commands that lint code
    lint:
      - magex lint

    # Commands that run tests
    test:
      - magex test:race

    # Commands run before committing
    pre_commit:
      - go-pre-commit run --all-files

    # Custom commands to run before creating a PR
    custom_pre_pr: []

#------------------------------------------------------------------------------
# Notification Settings
#------------------------------------------------------------------------------
notifications:
  # Enable terminal bell for important events
  # Default: true
  bell: true

  # Events that trigger notifications
  # Available: "awaiting_approval", "validation_failed", "ci_failed", "github_failed"
  # Default: all events
  events:
    - awaiting_approval
    - validation_failed
    - ci_failed
    - github_failed

#------------------------------------------------------------------------------
# Smart Commit Configuration
#------------------------------------------------------------------------------
smart_commit:
  # Agent for commit message generation; empty = uses ai.agent setting
  # Valid values: "claude", "gemini", "codex"
  # Default: "" (uses ai.agent)
  agent: ""

  # Model for commit message generation; empty = uses ai.model setting
  # Default: ""
  model: ""

#------------------------------------------------------------------------------
# PR Description Configuration
#------------------------------------------------------------------------------
pr_description:
  # Agent for PR description generation; empty = uses ai.agent setting
  # Valid values: "claude", "gemini", "codex"
  # Default: "" (uses ai.agent)
  agent: ""

  # Model for PR description generation; empty = uses ai.model setting
  # Default: ""
  model: ""

#------------------------------------------------------------------------------
# Approval Configuration
#------------------------------------------------------------------------------
approval:
  # Default message for approve + merge + close operations
  # Default: "Approved and Merged by ATLAS"
  merge_message: "Approved and Merged by ATLAS"

#------------------------------------------------------------------------------
# Verification Configuration
#------------------------------------------------------------------------------
verify:
  # Permission mode for AI verification steps
  # "plan" = read-only sandbox mode (recommended)
  # "" = full access (not recommended - allows destructive operations)
  # Default: "plan"
  permission_mode: plan

  # Verification checks to run (affects verify step speed)
  # Available checks:
  #   - code_correctness: Does the code address the task? Any bugs? (~10-20s)
  #   - test_coverage: Are there tests for the changes? (~15-30s)
  #   - garbage_files: Any temp/debug files to remove? (~5-10s, redundant with smart_commit)
  #   - security: Any hardcoded secrets or vulnerabilities? (~10-20s)
  # Default: ["code_correctness"] for fast verification
  # Full: ["code_correctness", "test_coverage", "garbage_files", "security"]
  checks:
    - code_correctness
    # - test_coverage
    # - garbage_files
    # - security

#------------------------------------------------------------------------------
# Hook System Configuration (Crash Recovery)
#------------------------------------------------------------------------------
hooks:
  # Maximum number of checkpoints per task (oldest are pruned)
  # Default: 50
  max_checkpoints: 50

  # Interval for periodic checkpoints during long-running steps
  # Set to 0 to disable interval checkpoints
  # Default: 5m
  checkpoint_interval: 5m

  # Time after which a hook is considered stale (potential crash)
  # Default: 5m
  stale_threshold: 5m

  # Retention periods for hook files per terminal state
  retention:
    # Retention for completed task hooks
    # Default: 720h (30 days)
    completed: 720h

    # Retention for failed task hooks
    # Default: 168h (7 days)
    failed: 168h

    # Retention for abandoned task hooks
    # Default: 168h (7 days)
    abandoned: 168h

  # Cryptographic provider configuration for receipt signing
  crypto:
    # Provider type: "native" (Ed25519)
    # Default: "native"
    provider: native

    # Path to master key file
    # Default: "~/.atlas/keys/master.key"
    key_path: ~/.atlas/keys/master.key
```

<br>

## File Structure

### ATLAS Home Directory

```
~/.atlas/
├── config.yaml                           # Global configuration
├── logs/
│   └── atlas.log                         # CLI operations log (rotated)
├── keys/
│   └── master.key                        # Ed25519 signing key for receipts (0600)
├── backups/
│   └── speckit-<timestamp>/              # Speckit upgrade backups
└── workspaces/
    └── <workspace-name>/
        ├── workspace.json                # Workspace metadata + task history
        └── tasks/
            └── task-YYYYMMDD-HHMMSS/     # Task ID (timestamp-based)
                ├── task.json             # Task state & step history
                ├── task.log              # Full execution log (JSON-lines)
                ├── hook.json             # Hook state (crash recovery source of truth)
                ├── HOOK.md               # Human-readable recovery guide
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
ls ~/.atlas/workspaces/auth/tasks/task-550e8400-e29b-41d4-a716-446655440002/artifacts/

# Workspace task history
jq '.tasks' ~/.atlas/workspaces/auth/workspace.json

# Latest task in a workspace (sorts chronologically)
ls ~/.atlas/workspaces/auth/tasks/ | tail -1

# Parse logs with jq
cat ~/.atlas/workspaces/*/tasks/*/task.log | jq 'select(.event=="model_complete")'
```

<br>

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `not in a git repository` | Running outside a git repo | `cd` to your git project root |
| `workspace 'x' exists` | Workspace name conflict | Use `--workspace <new-name>` or `atlas workspace destroy x` |
| `template required` | Non-interactive mode without template | Add `--template bugfix` (or `feature`, `commit`) |
| `invalid model` | Unknown model name | Claude: `sonnet`, `opus`, `haiku`; Gemini: `flash`, `pro`; Codex: `codex`, `max`, `mini` |
| `agent not found` | Unknown agent name | Use `claude`, `gemini`, or `codex` |
| `agent CLI not installed` | AI CLI not available | Install Claude CLI, Gemini CLI, or Codex CLI |
| Validation failed | Code doesn't pass checks | `atlas recover` or fix manually, then `atlas resume` |
| CI timeout | CI taking too long | `atlas recover` → continue waiting or retry |
| GitHub auth failed | gh CLI not authenticated | Run `gh auth login` |
| Claude CLI not found | claude not installed | `npm install -g @anthropic-ai/claude-code` |
| Gemini CLI not found | gemini not installed | `npm install -g @google/gemini-cli` |
| Codex CLI not found | codex not installed | `npm install -g @openai/codex` |
| No issues found (fix template) | Fix template found clean codebase | No action needed - task completes without PR |
| `hook not found` | No active hook for current task | Start a new task or resume existing with `atlas resume` |
| `Hook is stale` | Hook not updated in 5+ minutes (crash) | Run `atlas resume` to enter recovery mode |
| `Signature invalid` | Receipt tampered or key regenerated | Re-run validation to create new receipt |
| `Key manager error` | Master key missing/corrupted | Restore from backup or let ATLAS regenerate |

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

# View hook state and recovery info
atlas hook status
atlas hook export | jq

# List all checkpoints for current task
atlas hook checkpoints

# Verify a validation receipt
atlas hook verify-receipt rcpt-00000001

# View HOOK.md recovery guide
cat ~/.atlas/workspaces/my-workspace/tasks/*/HOOK.md
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

<br>

---

*This document was generated for ATLAS MVP. For detailed architecture information, see [docs/external/vision.md](../external/vision.md).*

---

**Version:** 1.2.2
**Last Updated:** 2026-01-19
