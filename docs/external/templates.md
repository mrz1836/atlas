# ATLAS Task Templates

> Comprehensive guide to ATLAS template system for automated task orchestration

## Overview

Templates define repeatable workflows for common software engineering tasks. Each template is a YAML file that specifies:

- **Steps**: Ordered sequence of operations (AI analysis, validation, git, human review)
- **SDD Integration**: Which framework to use (Speckit, BMAD, or none)
- **Model Selection**: Which AI model for each step
- **Git Operations**: Commit strategy, PR generation, branch management

Templates encode best practices so you can execute complex workflows with a single command:

```bash
atlas run bugfix --description "Fix nil pointer in user service"
atlas run feature --description "Add OAuth2 authentication"
atlas run feature-bmad --description "Enterprise audit logging system"
```

---

## Template Schema

```yaml
# Template metadata
name: string                    # Unique identifier (e.g., "bugfix", "feature")
version: string                 # Semver (e.g., "1.0.0")
description: string             # Human-readable description

# SDD integration (optional)
sdd:
  framework: speckit | bmad | none
  track: quick | standard | enterprise  # BMAD only

# Default settings
defaults:
  model: claude-sonnet-4-5-5      # Default AI model
  timeout: 300s                 # Default step timeout
  retry: 2                      # Default retry count for validation

# Steps (ordered)
steps:
  - name: string                # Unique step identifier
    type: ai | validation | git | human | sdd

    # Dependencies
    depends_on: [step_names]    # Must complete before this step

    # Type-specific fields (see Step Types below)
```

---

## Step Types

### AI Step

Invokes an AI model with a prompt template. Supports variable interpolation.

```yaml
- name: analyze
  type: ai
  model: claude-sonnet-4-5             # Override default model
  prompt: |
    Analyze this bug and identify root cause:

    Description: {{.Description}}
    Files: {{.Files | join ", "}}

    {{range .Files}}
    ## {{.}}
    ```
    {{file .}}
    ```
    {{end}}

    Provide:
    1. Root cause analysis
    2. Affected code paths
    3. Proposed fix approach
  output: .atlas/artifacts/analysis.md
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | Yes | Prompt template with `{{variables}}` |
| `model` | string | No | Override default model |
| `output` | string | No | Save response to file |

**Template Functions:**
- `{{.Variable}}` - Interpolate variable
- `{{file "path"}}` - Include file contents
- `{{file "path" \| section "Header"}}` - Extract section from file
- `{{.List \| join ", "}}` - Join list with separator
- `{{.List \| bullets}}` - Format as markdown bullets
- `{{range .Items}}...{{end}}` - Iterate over list

### Validation Step

Runs commands and checks for success. Supports retry on failure.

```yaml
- name: validate
  type: validation
  depends_on: [implement]
  commands:
    - magex format:fix
    - magex lint
    - magex test
    - go-pre-commit run --all-files
  retry: 2                         # Retry up to 2 times
  must_pass: true                  # Fail entire task if validation fails
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `commands` | []string | Yes | Commands to execute |
| `retry` | int | No | Retry count (default: from template defaults) |
| `must_pass` | bool | No | Fail task on validation failure (default: true) |

### Git Step

Performs git operations: branching, committing, pushing, PR creation.

```yaml
- name: commit
  type: git
  depends_on: [validate]
  action: commit
  message_template: |
    {{.CommitType}}: {{.ShortDescription}}

    {{.Summary}}

    ATLAS-Task: {{.TaskID}}
    ATLAS-Template: {{.Template}}
```

**Actions:**

| Action | Description | Template Fields |
|--------|-------------|-----------------|
| `branch` | Create/switch to feature branch | `branch_template` |
| `clean` | Remove untracked generated files | - |
| `stage` | Smart staging (exclude generated) | - |
| `commit` | Single commit | `message_template` |
| `smart_commit` | Multiple logical commits | - |
| `push` | Push to remote | - |
| `pr` | Create PR via gh CLI | `title_template`, `body_template` |
| `pr_update` | Update existing PR | `body_template` |

### Human Step

Pauses workflow for human review/approval.

```yaml
- name: review_spec
  type: human
  depends_on: [specify]
  prompt: |
    Review the specification:

    {{file ".atlas/artifacts/spec.md"}}

    Approve to proceed with implementation, or reject with feedback.
  options:
    - approve
    - reject
  on_reject: specify              # Return to this step with feedback
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | Yes | What to show the user |
| `options` | []string | Yes | Available choices |
| `on_reject` | string | No | Step to return to, or "fail" |

### SDD Step

Invokes an SDD framework command (Speckit or BMAD).

```yaml
# Speckit example
- name: specify
  type: sdd
  command: /speckit.specify
  args:
    input: "{{.Description}}"
    output: .atlas/artifacts/spec.md

# BMAD example
- name: analysis
  type: sdd
  command: "*analyst"
  args:
    task: brainstorm
    input: "{{.Description}}"
  output: .atlas/artifacts/analysis.md
```

**Speckit Commands:**
- `/speckit.constitution` - Set up project constitution
- `/speckit.specify` - Create specification
- `/speckit.plan` - Generate implementation plan
- `/speckit.tasks` - Break into tasks
- `/speckit.implement` - Execute implementation
- `/speckit.checklist` - Generate completion checklist

**BMAD Agents:**
- `*analyst` - Business analysis, brainstorming
- `*pm` - Product management, PRDs
- `*architect` - System design
- `*developer` - Implementation
- `*qa` - Quality assurance
- `*ux` - User experience design

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
- Use `thinking: ultrathink` for Opus 4.5 on architecture decisions
- Budget tokens: 32k+ for complex multi-step reasoning

**Fallback Strategy:**
- Primary: Claude Sonnet 4.5 (best coding)
- Deep thinking: Claude Opus 4.5 + ultrathink
- Fallback: Gemini 3 Pro (when Claude unavailable)
- Fast/cheap: Haiku 4.5 or Gemini 3 Flash

**Supported Models:**

| Provider | Model | Model ID | Use Case |
|----------|-------|----------|----------|
| Claude | Opus 4.5 | `claude-opus-4-5-20251124` | Deep thinking + ultrathink |
| Claude | Sonnet 4.5 | `claude-sonnet-4-5-20250916` | Default, best coding |
| Claude | Haiku 4.5 | `claude-haiku-4-5-20251015` | Fast, cheap |
| Gemini | 3 Pro | `gemini-3-pro-preview` | Complex reasoning fallback |
| Gemini | 3 Flash | `gemini-3-flash-preview` | Fast fallback |
| Gemini | 2.5 Pro | `gemini-2.5-pro` | Stable reasoning |
| Gemini | 2.5 Flash | `gemini-2.5-flash` | Stable balanced |
| Gemini | 2.5 Flash-Lite | `gemini-2.5-flash-lite` | Fastest/cheapest |

---

## Template Examples

### 1. bugfix.yaml

Simple bug fix workflow without SDD overhead.

```yaml
name: bugfix
version: "1.0.0"
description: Fix a bug with validation and PR

sdd:
  framework: none                  # No SDD for bug fixes

defaults:
  model: claude-sonnet-4-5
  timeout: 300s
  retry: 2

steps:
  - name: analyze
    type: ai
    prompt: |
      Analyze this bug and identify the root cause:

      {{.Description}}

      Files to examine:
      {{range .Files}}
      ## {{.}}
      ```go
      {{file .}}
      ```
      {{end}}

      Output a structured analysis:
      1. **Root Cause**: What's causing the bug?
      2. **Affected Code**: Which functions/lines?
      3. **Fix Approach**: How should we fix it?
      4. **Test Strategy**: How to verify the fix?
    output: .atlas/artifacts/analysis.md

  - name: implement
    type: ai
    depends_on: [analyze]
    prompt: |
      Based on this analysis:

      {{file ".atlas/artifacts/analysis.md"}}

      Implement the fix. Requirements:
      - Minimal changes only
      - Add test case that catches the bug
      - Follow existing code style
      - Include comments for complex logic
    output: .atlas/artifacts/implementation.md

  - name: validate
    type: validation
    depends_on: [implement]
    commands:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    retry: 2
    must_pass: true

  - name: git_prepare
    type: git
    depends_on: [validate]
    action: clean

  - name: git_commit
    type: git
    depends_on: [git_prepare]
    action: commit
    message_template: |
      fix: {{.ShortDescription}}

      Root cause: {{file ".atlas/artifacts/analysis.md" | section "Root Cause"}}

      ATLAS-Task: {{.TaskID}}
      ATLAS-Template: bugfix

  - name: git_push
    type: git
    depends_on: [git_commit]
    action: push

  - name: git_pr
    type: git
    depends_on: [git_push]
    action: pr
    title_template: "fix: {{.ShortDescription}}"
    body_template: |
      ## Summary
      {{file ".atlas/artifacts/analysis.md" | section "Root Cause"}}

      ## Changes
      {{.FilesChanged | bullets}}

      ## Testing
      - Added test case for bug scenario
      - All existing tests pass

  - name: review
    type: human
    depends_on: [git_pr]
    prompt: |
      PR created: {{.PRURL}}

      Review the changes and approve or reject.
    options:
      - approve
      - reject
    on_reject: fail
```

---

### 2. feature.yaml (Speckit SDD)

Feature implementation using Speckit for specification-driven development.

```yaml
name: feature
version: "1.0.0"
description: Implement a new feature using Speckit SDD

sdd:
  framework: speckit

defaults:
  model: claude-sonnet-4-5
  timeout: 300s
  retry: 2

steps:
  - name: specify
    type: sdd
    command: /speckit.specify
    args:
      description: "{{.Description}}"
    output: .atlas/artifacts/spec.md

  - name: review_spec
    type: human
    depends_on: [specify]
    prompt: |
      Review the specification:

      {{file ".atlas/artifacts/spec.md"}}

      Approve to proceed, or reject with feedback for refinement.
    options:
      - approve
      - reject
    on_reject: specify

  - name: plan
    type: sdd
    depends_on: [review_spec]
    command: /speckit.plan
    output: .atlas/artifacts/plan.md

  - name: tasks
    type: sdd
    depends_on: [plan]
    command: /speckit.tasks
    output: .atlas/artifacts/tasks.md

  - name: implement
    type: sdd
    depends_on: [tasks]
    command: /speckit.implement

  - name: validate
    type: validation
    depends_on: [implement]
    commands:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    retry: 2
    must_pass: true

  - name: checklist
    type: sdd
    depends_on: [validate]
    command: /speckit.checklist
    output: .atlas/artifacts/checklist.md

  - name: git_smart_commit
    type: git
    depends_on: [checklist]
    action: smart_commit

  - name: git_push
    type: git
    depends_on: [git_smart_commit]
    action: push

  - name: git_pr
    type: git
    depends_on: [git_push]
    action: pr
    title_template: "feat: {{.ShortDescription}}"
    body_template: |
      ## Specification
      {{file ".atlas/artifacts/spec.md" | summary}}

      ## Implementation Plan
      {{file ".atlas/artifacts/plan.md" | summary}}

      ## Checklist
      {{file ".atlas/artifacts/checklist.md"}}

  - name: review
    type: human
    depends_on: [git_pr]
    prompt: "Review the PR and approve or reject."
    options:
      - approve
      - reject
    on_reject: fail
```

---

### 3. feature-bmad.yaml (BMAD Method)

Enterprise feature implementation using BMAD multi-agent workflow.

```yaml
name: feature-bmad
version: "1.0.0"
description: Implement a feature using BMAD Method

sdd:
  framework: bmad
  track: standard                  # quick | standard | enterprise

defaults:
  model: claude-sonnet-4-5

steps:
  - name: analysis
    type: sdd
    command: "*analyst"
    args:
      task: brainstorm
      input: "{{.Description}}"
    output: .atlas/artifacts/analysis.md

  - name: prd
    type: sdd
    depends_on: [analysis]
    command: "*pm"
    args:
      task: create-prd
    output: .atlas/artifacts/prd.md

  - name: review_prd
    type: human
    depends_on: [prd]
    prompt: |
      Review the PRD:
      {{file ".atlas/artifacts/prd.md"}}
    options:
      - approve
      - reject
    on_reject: prd

  - name: architecture
    type: sdd
    depends_on: [review_prd]
    command: "*architect"
    args:
      task: design
    output: .atlas/artifacts/architecture.md

  - name: implementation
    type: sdd
    depends_on: [architecture]
    command: "*developer"
    args:
      task: implement

  - name: validate
    type: validation
    depends_on: [implementation]
    commands:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    retry: 2

  - name: qa
    type: sdd
    depends_on: [validate]
    command: "*qa"
    output: .atlas/artifacts/qa-report.md

  - name: git_commit
    type: git
    depends_on: [qa]
    action: smart_commit

  - name: git_push
    type: git
    depends_on: [git_commit]
    action: push

  - name: git_pr
    type: git
    depends_on: [git_push]
    action: pr
    title_template: "feat: {{.ShortDescription}}"
    body_template: |
      ## PRD
      {{file ".atlas/artifacts/prd.md" | summary}}

      ## Architecture
      {{file ".atlas/artifacts/architecture.md" | summary}}

      ## QA Report
      {{file ".atlas/artifacts/qa-report.md"}}

  - name: review
    type: human
    depends_on: [git_pr]
    prompt: "Review the PR."
    options:
      - approve
      - reject
    on_reject: fail
```

---

### 4. test-coverage.yaml

Add test coverage to existing code.

```yaml
name: test-coverage
version: "1.0.0"
description: Add test coverage to existing code

sdd:
  framework: none

defaults:
  model: claude-sonnet-4-5

steps:
  - name: analyze_coverage
    type: ai
    prompt: |
      Analyze test coverage gaps for:
      {{range .Files}}
      ## {{.}}
      ```go
      {{file .}}
      ```
      {{end}}

      Identify:
      1. Functions/methods without tests
      2. Edge cases not covered
      3. Error paths not tested

      Prioritize by risk (what breaks if untested?).
    output: .atlas/artifacts/coverage-analysis.md

  - name: implement_tests
    type: ai
    depends_on: [analyze_coverage]
    prompt: |
      Based on this analysis:
      {{file ".atlas/artifacts/coverage-analysis.md"}}

      Write comprehensive tests:
      - Table-driven tests where appropriate
      - Edge cases and error conditions
      - Clear test names that document behavior
      - Use testify/require for assertions
    output: .atlas/artifacts/test-implementation.md

  - name: validate
    type: validation
    depends_on: [implement_tests]
    commands:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    retry: 2

  - name: git_commit
    type: git
    depends_on: [validate]
    action: commit
    message_template: |
      test: add coverage for {{.ShortDescription}}

      ATLAS-Task: {{.TaskID}}
      ATLAS-Template: test-coverage

  - name: git_push
    type: git
    depends_on: [git_commit]
    action: push

  - name: git_pr
    type: git
    depends_on: [git_push]
    action: pr
    title_template: "test: add coverage for {{.ShortDescription}}"
    body_template: |
      ## Coverage Analysis
      {{file ".atlas/artifacts/coverage-analysis.md" | summary}}

      ## Tests Added
      {{.FilesChanged | bullets}}

  - name: review
    type: human
    depends_on: [git_pr]
    prompt: "Review the test coverage PR."
    options:
      - approve
      - reject
    on_reject: fail
```

---

### 5. refactor.yaml

Incremental refactoring with validation between each step.

```yaml
name: refactor
version: "1.0.0"
description: Refactor code incrementally with validation between steps

sdd:
  framework: speckit

defaults:
  model: claude-sonnet-4-5

steps:
  - name: analyze
    type: ai
    prompt: |
      Analyze this code for refactoring opportunities:
      {{range .Files}}
      ## {{.}}
      ```go
      {{file .}}
      ```
      {{end}}

      Identify:
      1. Code smells
      2. Duplication
      3. Complexity hotspots
      4. Naming issues
    output: .atlas/artifacts/refactor-analysis.md

  - name: plan
    type: ai
    depends_on: [analyze]
    prompt: |
      Based on this analysis:
      {{file ".atlas/artifacts/refactor-analysis.md"}}

      Create an incremental refactoring plan:
      - Each step should be independently valid
      - Order by dependency (foundational changes first)
      - Each step should pass all tests

      Output as numbered list of discrete changes.
    output: .atlas/artifacts/refactor-plan.md

  - name: review_plan
    type: human
    depends_on: [plan]
    prompt: |
      Review the refactoring plan:
      {{file ".atlas/artifacts/refactor-plan.md"}}

      Approve to proceed with incremental refactoring.
    options:
      - approve
      - reject
    on_reject: plan

  - name: implement_step_1
    type: ai
    depends_on: [review_plan]
    prompt: |
      Implement step 1 of the refactoring plan:
      {{file ".atlas/artifacts/refactor-plan.md" | section "1."}}

  - name: validate_step_1
    type: validation
    depends_on: [implement_step_1]
    commands:
      - magex format:fix
      - magex lint
      - magex test
      - go-pre-commit run --all-files
    retry: 2

  - name: commit_step_1
    type: git
    depends_on: [validate_step_1]
    action: commit
    message_template: "refactor: step 1 - {{description}}"

  # Additional steps follow same pattern...
  # In practice, step count is determined dynamically

  - name: git_push
    type: git
    depends_on: [commit_step_1]
    action: push

  - name: git_pr
    type: git
    depends_on: [git_push]
    action: pr
    title_template: "refactor: {{.ShortDescription}}"
    body_template: |
      ## Analysis
      {{file ".atlas/artifacts/refactor-analysis.md" | summary}}

      ## Refactoring Plan
      {{file ".atlas/artifacts/refactor-plan.md"}}

      ## Changes
      {{.FilesChanged | bullets}}

  - name: review
    type: human
    depends_on: [git_pr]
    prompt: "Review the refactoring PR."
    options:
      - approve
      - reject
    on_reject: fail
```

---

## Git Operations Deep Dive

### Smart Commit Strategy

The `git:smart_commit` action automatically:

1. **Analyzes changes** - Groups modified files by:
   - Package/directory
   - Type (source, test, docs, config)
   - Semantic relationship

2. **Generates commit messages** - For each group:
   - Infers change type (feat, fix, refactor, test, docs)
   - Summarizes what changed
   - Adds ATLAS trailers for traceability

3. **Creates commits** - One commit per logical unit

### Example Smart Commit Output

```
# Given changes to:
# - internal/user/service.go
# - internal/user/service_test.go
# - internal/auth/token.go
# - README.md

git commit -m "feat(user): add email validation to user service"
git commit -m "test(user): add tests for email validation"
git commit -m "feat(auth): extend token expiry configuration"
git commit -m "docs: update README with new auth features"
```

### PR Generation

PR body supports rich templates:

```yaml
body_template: |
  ## Summary
  {{.Summary}}

  ## Changes
  {{.FilesChanged | bullets}}

  ## Testing
  {{if .HasTests}}
  - Tests added/updated
  {{end}}
  - All existing tests pass

  ## Artifacts
  {{if file ".atlas/artifacts/spec.md"}}
  <details>
  <summary>Specification</summary>

  {{file ".atlas/artifacts/spec.md"}}
  </details>
  {{end}}
```

---

## Human Checkpoints

### Approval Flow

When a human step is reached:

1. ATLAS displays the prompt with interpolated content
2. User selects from options (typically approve/reject)
3. On approve: workflow continues to next step
4. On reject: workflow returns to `on_reject` step with feedback

### Rejection Handling

```yaml
- name: review_spec
  type: human
  prompt: "Review specification..."
  options:
    - approve
    - reject
  on_reject: specify    # Go back to specify step
```

When rejected:
- User provides feedback text
- Feedback is available as `{{.Feedback}}` in the target step
- Step re-executes with feedback context

### Skip Human Steps

For automation/CI, human steps can be auto-approved:

```bash
atlas run feature --auto-approve
```

---

## Custom Templates

### Creating Your Own

1. Create template file in `~/.atlas/templates/`:

```yaml
# ~/.atlas/templates/my-workflow.yaml
name: my-workflow
version: "1.0.0"
description: My custom workflow

steps:
  # Your steps here
```

2. Use it:

```bash
atlas run my-workflow --description "Do the thing"
```

### Template Locations

Templates are loaded from (in order):
1. `.atlas/templates/` (project-specific)
2. `~/.atlas/templates/` (user-specific)
3. Embedded defaults (built-in)

Later definitions override earlier ones.

### Template Variables

Available variables in templates:

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

---

## Configurable Validation

Validation commands are configurable per-project in `.atlas/config.yaml`:

```yaml
# .atlas/config.yaml
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

---

## SDD Framework Selection

| Use Case | Framework | Rationale |
|----------|-----------|-----------|
| Bug fixes | None | Overkill; just analyze + fix |
| Small features | Speckit | Lightweight, focused specs |
| Medium features | Speckit | Full specification + planning |
| Large features | BMAD Standard | Multi-agent coordination |
| Enterprise | BMAD Enterprise | Governance, compliance |

### When to Use Speckit
- Clear, bounded requirements
- Individual developer workflow
- Fast iteration cycles
- Specification-first approach

### When to Use BMAD
- Complex features requiring multiple perspectives
- Team coordination needed
- Enterprise requirements (compliance, audit)
- Need PRDs, architecture docs, QA reports
