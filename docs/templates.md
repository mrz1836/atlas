# ATLAS Task Templates

> Comprehensive guide to ATLAS template system for automated task orchestration

## Overview

Templates define repeatable workflows for common software engineering tasks. Each template is **Go code compiled into the ATLAS binary**, providing:

- **Type safety**: Compile-time validation of step definitions
- **Testability**: Templates can be unit tested like any Go code
- **Mockability**: Step executors can be mocked for testing
- **Reliability**: No YAML parsing errors at runtime
- **IDE support**: Full autocompletion and refactoring support

Templates encode best practices so you can execute complex workflows with a single command:

```bash
atlas start "Fix nil pointer in user service" --template bug
atlas start "Add OAuth2 authentication" --template feature
atlas start "fix lint errors" --template patch --target feat/my-feature
```

---

## Template Architecture

Templates are structured as ordered sequences of steps, where each step has a specific type and behavior:

```
Template
├── Metadata (name, version, description)
├── Defaults (model, timeout, max_turns)
└── Steps[]
    ├── Step 1: AI analysis
    ├── Step 2: Implementation
    ├── Step 3: Validation
    ├── Step 4: Git operations
    └── Step 5: Human review
```

**Key properties:**
- Steps execute in order, with dependency tracking
- Each step type has specific behavior (AI invocation, command execution, git operations, etc.)
- Steps can reference outputs from previous steps
- Validation failures pause for human decision (retry, fix manually, or abandon)

---

## Step Types

### AI Step

Invokes an AI model with a prompt. Supports variable interpolation for dynamic content.

**Capabilities:**
- Model selection (Opus, Sonnet, Haiku, Gemini)
- Extended thinking mode for complex analysis
- Output saved to artifact files
- Access to file contents via template functions

**Common uses:**
- Code analysis and root cause identification
- Implementation generation
- Commit message and PR description generation

### Validation Step

Runs commands and checks for success. Failures pause for human decision.

**Capabilities:**
- Sequential command execution
- Pass/fail determination
- On failure: human chooses to retry (AI tries again), fix manually, or abandon

**Common commands:**
- `magex format:fix` — Auto-format code
- `magex lint` — Run linters
- `magex test` — Execute tests
- `go-pre-commit run --all-files` — Pre-commit hooks

### Git Step

Performs git operations: branching, committing, pushing, PR creation.

**Actions:**
| Action | Description |
|--------|-------------|
| `branch` | Create/switch to feature branch |
| `clean` | Remove untracked generated files |
| `stage` | Smart staging (exclude generated files) |
| `commit` | Single commit with templated message |
| `smart_commit` | Multiple logical commits (groups by package/type) |
| `push` | Push to remote |
| `pr` | Create PR via gh CLI |
| `pr_update` | Update existing PR |

### Human Step

Pauses workflow for human review/approval.

**Capabilities:**
- Display artifacts and summaries for review
- Approve/reject options
- Feedback capture on rejection
- Loop back to previous steps with feedback context

### SDD Step

Integrates with the Speckit framework for specification-driven development.

**Speckit Commands:**
- `constitution` — Set up project constitution
- `specify` — Create specification
- `plan` — Generate implementation plan
- `tasks` — Break into tasks
- `implement` — Execute implementation
- `checklist` — Generate completion checklist

### CI Step

Waits for GitHub Actions workflows to complete and checks their status.

**Capabilities:**
- Polls GitHub Actions API for workflow run status on the PR
- Configurable polling interval (default: 2 minutes)
- Configurable timeout (default: 30 minutes)
- Watches specific workflows or all workflows triggered by the PR
- Fails task if any watched workflow fails

**Configuration:**
```yaml
# .atlas/config.yaml
ci:
  workflows:
    - name: "CI"              # Workflow name to watch
      required: true          # Fail if this workflow fails
    - name: "Lint"
      required: true
    - name: "Security Scan"
      required: false         # Warning only, don't block
  poll_interval: 2m           # Check every 2 minutes
  timeout: 30m                # Give up after 30 minutes
```

**Behavior:**
1. After PR creation, queries GitHub Actions API for runs on the PR
2. Polls at configured interval until all required workflows complete
3. If all required workflows pass → continue to human review
4. If any required workflow fails → enter `ci_failed` state (human decides)
5. If timeout exceeded → enter `ci_timeout` state (human decides)

**CI failure menu:**
```
? CI workflow "CI" failed. What would you like to do?
  ❯ View workflow logs — Open GitHub Actions in browser
    Retry from implement — AI tries to fix based on CI output
    Fix manually and resume — You fix, then 'atlas resume'
    Abandon task — End task, keep branch for manual work
```

### Gather Step

Optional step for collecting user input before execution. Skipped if CLI provides all required information.

**Capabilities:**
- Interactive prompts for required context
- Template-specific questions (e.g., bugfix asks for reproduction steps)
- Auto-detection of relevant files when possible
- Skip logic based on CLI-provided flags

**Example usage in bugfix template:**
```yaml
steps:
  - name: gather
    type: gather
    optional: true
    questions:
      - id: expected_behavior
        prompt: "Describe the expected behavior"
        required: false
      - id: repro_steps
        prompt: "Steps to reproduce the issue"
        required: false
      - id: files
        prompt: "Relevant file paths"
        auto_detect: true  # ATLAS suggests based on description
```

**When gather is skipped:**
- All required questions already answered via CLI flags
- `--no-prompt` flag passed
- Running in non-interactive mode (CI/automation)

---

## Step Execution

By default, steps execute sequentially. Steps can be grouped for parallel execution using the `parallel_group` attribute.

### Sequential Execution (Default)

Steps without a `parallel_group` run one after another:

```yaml
steps:
  - name: analyze
    type: ai
  - name: implement
    type: ai
  - name: commit
    type: git
```

### Parallel Execution

Steps with the same `parallel_group` value run concurrently:

```yaml
steps:
  - name: format
    type: validation
    # No group — runs first (modifies files)

  - name: lint
    type: validation
    parallel_group: checks    # ─┐
                              #  ├─ Run together
  - name: test                #  │
    type: validation          #  │
    parallel_group: checks    # ─┘
```

**Parallel execution rules:**
- Steps in the same `parallel_group` run concurrently
- Steps without a group run sequentially
- All parallel steps must complete before the next sequential step starts
- If any parallel step fails, others in the group are cancelled

**Common parallel groups:**

| Group | Steps | Rationale |
|-------|-------|-----------|
| `checks` | lint, test | Both read-only, independent |
| `analysis` | static-analysis, security-scan | Independent analyzers |

**What stays sequential:**
- `format` — Must complete before checks (modifies files)
- `commit` — Must wait for all validation
- `human` — Always blocking

---

## Built-in Templates

### bug

Smart bug fix workflow that auto-selects between two modes based on description length.

**With description (>20 chars):** analyze → implement → verify → validate → commit → push → pr → ci_wait → review
**Without description (≤20 chars):** detect → implement → verify → validate → commit → push → pr → ci_wait → review

The `detect` step runs validation commands to find issues, while the `analyze` step analyzes a user-provided bug description. The template automatically skips the irrelevant step based on the description length.

**Best for:** Bug fixes, small patches, quick corrections

**Aliases:** `fix`, `bugfix` (for backward compatibility)

### patch

Fix issues on an existing branch without creating a new PR.

**Steps:** detect → fix → verify → validate → commit → push

**Key difference:** No PR creation, CI wait, or review steps. Designed for use with `--target` flag to push fixes directly to an existing branch that already has a PR.

**Best for:** Quick fixes on existing feature branches, fixing CI failures on PRs

**Aliases:** `hotfix` (for backward compatibility)

**Usage:**
```bash
atlas start "fix lint errors" --template patch --target feat/my-feature
```

### feature

Feature implementation using Speckit for specification-driven development.

**Steps:** specify → review_spec → plan → tasks → implement → validate → checklist → commit → push → pr → ci_wait → review

**Best for:** Small to medium features with clear requirements

### task

Simple task workflow for well-defined work without requiring analysis.

**Steps:** implement → verify (optional) → validate → commit → push → pr → ci_wait → review

**Best for:** Simple, well-defined tasks with clear requirements (e.g., "add logging", "update dependencies", "update documentation")

**Key Features:**
- **Direct implementation**: Skips analysis phase and goes straight to coding
- **Fast execution**: Uses `sonnet` model by default for speed
- **Optional verification**: Enable with `--verify` flag for AI cross-validation using a different model
- **Branch prefix**: Creates branches with `task/` prefix

**When to use Task vs Bugfix vs Feature:**
- **Task**: Simple, well-defined work with clear requirements
- **Bugfix**: Problem requires investigation and root cause analysis
- **Feature**: Complex changes requiring specification and planning

### test-coverage (Post-MVP)

Add test coverage to existing code.

**Steps:** analyze_coverage → implement_tests → validate → commit → push → pr → review

**Best for:** Improving test coverage, TDD retrofitting

### refactor (Post-MVP)

Incremental refactoring with validation between each step.

**Steps:** analyze → plan → review_plan → implement_step_N → validate_step_N → commit_step_N → ... → push → pr → review

**Best for:** Large refactoring efforts, breaking changes

### learn (Post-MVP)

Analyze completed work and propose updates to project rules files. Spawned after `atlas approve` completes a task.

**Steps:** read_rules → analyze_learnings → propose_updates → review_updates → apply_updates

| Step | Type | Description |
|------|------|-------------|
| `read_rules` | AI | Read configured rules files, understand structure |
| `analyze_learnings` | AI | Review task artifacts, identify patterns worth codifying |
| `propose_updates` | AI | Draft updates respecting each file's format/purpose |
| `review_updates` | Human | Show diff of proposed changes, approve/reject |
| `apply_updates` | Git | Commit rule changes with `ATLAS-Learn` trailer |

**Configured rules files** (in `.atlas/config.yaml`):
```yaml
rules:
  files:
    - path: .speckit/constitution.md
      description: "Core project principles and constraints"
    - path: AGENTS.md
      description: "AI agent behavior guidelines"
    - path: docs/CODING_STANDARDS.md
      description: "Code style and patterns"
```

**Best for:** Capturing learnings from completed work, evolving project standards

---

## Utility Templates

Lightweight, single-purpose templates for common quick tasks.

### commit

Smart commit workflow with garbage detection and logical grouping.

**Steps:** detect_garbage → group_changes → generate_messages → commit

**Features:**
- **Garbage detection**: Warns about files that shouldn't be committed
- **Logical grouping**: Groups files by package, type (source/test/config)
- **Message generation**: AI-generated commit messages matching project conventions

**Garbage patterns detected:**

| Category | Patterns |
|----------|----------|
| Backup files | `.bak`, `.orig`, `~` suffix, `.tmp` |
| Go artifacts | `go.bak`, `__debug_bin*`, `coverage.out`, `*.test` |
| Fuzz artifacts | `testdata/fuzz/**/corpus`, `fuzz_*` temp files |
| IDE/Editor | `.swp`, `.swo`, `*~` |

Future: Patterns configurable in `.atlas/config.yaml` under `commit.garbage_patterns`.

### clean

Detect and remove temporary/garbage files from the working directory.

**Steps:** scan → confirm → remove

**Actions:**
- Scans for garbage patterns (same as commit template)
- Shows files to be removed
- Confirms before deletion (unless `--force`)

### format

Run code formatting only.

**Steps:** format → stage

**Commands:** Runs configured formatters (e.g., `magex format:fix`)

### lint

Run linters only.

**Steps:** lint

**Commands:** Runs configured linters (e.g., `magex lint`)

### test

Run tests only.

**Steps:** test

**Commands:** Runs configured test command (e.g., `magex test`)

### validate

Full validation suite: format, then lint and test in parallel.

**Steps:** format → [lint, test] (parallel)

**Commands:** Runs all validation commands with parallel execution for lint/test.

### pr-update (Post-MVP)

Update an existing PR description based on new changes.

**Steps:** analyze_changes → generate_description → update_pr

**Features:**
- Analyzes commits since PR creation
- Regenerates PR description
- Updates via `gh pr edit`

---

## Model Selection Guide

| Step Type | Recommended Model | Rationale |
|-----------|------------------|-----------|
| Deep Analysis | `claude-opus-4-5` + ultrathink | Complex architecture, critical decisions |
| Analysis | `claude-sonnet-4-5` | Good reasoning, cost-effective |
| Specification | `claude-sonnet-4-5` | Creativity + precision |
| Planning | `claude-sonnet-4-5` | Strategic thinking |
| Implementation | `claude-sonnet-4-5` | Best coding model |
| Commit messages | `claude-haiku-4-5` or `gemini-3-flash` | Simple task, speed |
| PR descriptions | `claude-sonnet-4-5` | Good summarization |

**Extended Thinking (Ultrathink):**
- Use for Opus 4.5 on architecture decisions
- Budget tokens: 32k+ for complex multi-step reasoning

**Fallback Strategy:**
- Primary: Claude Sonnet 4.5 (best coding)
- Deep thinking: Claude Opus 4.5 + ultrathink
- Fallback: Gemini 3 Pro (when Claude unavailable)
- Fast/cheap: Haiku 4.5 or Gemini 3 Flash

**Supported Models:**

| Provider | Model | Model ID | Use Case |
|----------|-------|----------|----------|
| Claude | Opus 4.5 | `claude-opus-4-5-20251101` | Deep thinking + ultrathink |
| Claude | Sonnet 4.5 | `claude-sonnet-4-5-20250916` | Default, best coding |
| Claude | Haiku 4.5 | `claude-haiku-4-5-20251015` | Fast, cheap |
| Gemini | 3 Pro | `gemini-3-pro-preview` | Complex reasoning fallback |
| Gemini | 3 Flash | `gemini-3-flash-preview` | Fast fallback |
| Gemini | 2.5 Pro | `gemini-2.5-pro` | Stable reasoning |
| Gemini | 2.5 Flash | `gemini-2.5-flash` | Stable balanced |
| Gemini | 2.5 Flash-Lite | `gemini-2.5-flash-lite` | Fastest/cheapest |

---

## Template Variables

Templates support runtime variable interpolation for dynamic content:

| Variable | Description |
|----------|-------------|
| `{{.Description}}` | Task description from CLI |
| `{{.ShortDescription}}` | First line of description |
| `{{.Files}}` | List of relevant files |
| `{{.TaskID}}` | Unique task identifier |
| `{{.Template}}` | Template name |
| `{{.Workspace}}` | Worktree path |
| `{{.Branch}}` | Current branch name |
| `{{.BaseBranch}}` | Base branch (main/master) |
| `{{.PRURL}}` | PR URL after creation |
| `{{.FilesChanged}}` | List of changed files |
| `{{.Feedback}}` | Human feedback (after rejection) |

**Template Functions:**
- `{{file "path"}}` — Include file contents
- `{{file "path" | section "Header"}}` — Extract section from file
- `{{.List | join ", "}}` — Join list with separator
- `{{.List | bullets}}` — Format as markdown bullets
- `{{range .Items}}...{{end}}` — Iterate over list
- `{{file "path" | summary}}` — Summarize file contents

---

## Customization

Users customize template behavior via CLI flags and configuration files—no code changes required.

### CLI Flags

```bash
# Select template
atlas start "description" --template bugfix

# Override model for all AI steps
atlas start "description" --model claude-opus-4-5

# Skip human approval (for CI/automation)
atlas start "description" --auto-approve

# Specify workspace name
atlas start "description" --workspace my-feature
```

### Configuration File

Project-level customization in `.atlas/config.yaml`:

```yaml
# Validation commands (override template defaults)
validation:
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
    feature:
      - magex format:fix
      - magex lint
      - magex test
      - magex integration-test

  # Custom hooks
  hooks:
    pre_pr:
      - magex integration-test
```

Global configuration in `~/.atlas/config.yaml`:

```yaml
model:
  primary:
    provider: claude
    model: claude-sonnet-4-5-20250916
    api_key_env: ANTHROPIC_API_KEY
  fallback:
    provider: gemini
    model: gemini-3-pro-preview
    api_key_env: GOOGLE_API_KEY
  timeout: 300s
  max_tokens: 100000
```
