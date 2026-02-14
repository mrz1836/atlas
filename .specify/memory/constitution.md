<!--
SYNC IMPACT REPORT
==================
Version Change: N/A → 1.0.0 (Initial ratification)
Added Principles:
  - I. Human Authority at Checkpoints
  - II. Git is the Backbone
  - III. Text is Truth
  - IV. Ship Then Iterate
  - V. Context-First Go
  - VI. Validation Before Delivery
  - VII. Transparent State
Added Sections:
  - Technology Stack (constraints and requirements)
  - Development Workflow (quality gates and process)
Templates Status:
  - ✅ plan-template.md - Constitution Check section aligns with principles
  - ✅ spec-template.md - User scenarios and requirements align with Human Authority
  - ✅ tasks-template.md - Phase structure supports iterative delivery
Follow-up TODOs: None
-->

# ATLAS Constitution

ATLAS (AI Task Lifecycle Automation System) is a CLI tool that orchestrates AI-assisted
development workflows for Go projects. It automates: analyze issues → implement fixes →
validate code → create PRs—while keeping humans in control at every decision point.

## Core Principles

### I. Human Authority at Checkpoints

**AI proposes, humans dispose.** Every significant decision point MUST pause for human
review and approval. ATLAS automates the tedious; humans retain judgment.

- **Validation tasks** (lint, test, format) auto-proceed on success
- **Code changes** ALWAYS pause for human approval before commit
- **PR creation** requires explicit human authorization
- **No unsupervised merges, ever**—humans must approve all code entering the repository
- The `--auto-approve` flag exists for scripting but defaults to interactive review

*Rationale*: AI makes mistakes. Validation catches some; humans catch the rest. Trust is
built through verification, not blind automation.

### II. Git is the Backbone

Git is not just version control—it's the audit trail, delivery mechanism, and source of
truth. Every ATLAS action MUST produce Git artifacts.

- **All code changes** result in branches, commits with machine-parseable trailers, and PRs
- **Commit messages** follow conventional format: `<type>(<scope>): <description>`
- **If it's not in Git, it didn't happen**—no shadow state, no side channels
- **Worktrees** provide parallel workspace isolation without branch conflicts
- **Branch naming** follows `<type>/<workspace-name>` pattern (e.g., `fix/null-pointer`)

*Rationale*: Git provides the complete history that enables debugging, rollback, and
compliance. External state creates drift and confusion.

### III. Text is Truth

All state MUST be stored as human-readable text files. No databases, no binary formats.
Debug by reading files. Trust by verifying.

- **JSON** for structured data (task state, workspace metadata)
- **YAML** for configuration (user preferences, template overrides)
- **Markdown** for prose (artifacts, PR descriptions, specifications)
- **Go code** for templates and workflows—type-safe, testable, compiled into the binary
- You can always `cat` your way to understanding what ATLAS did

*Rationale*: Transparency builds trust. When state is inspectable, debugging is possible
and AI behavior is auditable.

### IV. Ship Then Iterate

Start with the simplest thing that works. Add complexity only when real usage demands it.
If a feature isn't needed for the next task, it doesn't exist yet.

- **MVP mindset**: This project is unreleased—breaking changes are welcome
- **No premature abstraction**: Three similar lines of code beat a speculative helper
- **No backwards compatibility layers**: Delete unused code completely
- **Defer features explicitly**: Track in "Post-MVP" section, not in half-built code
- **User feedback drives direction** more than roadmaps

*Rationale*: Over-engineering kills velocity. Real problems reveal themselves through use;
hypothetical problems waste effort.

### V. Context-First Go

Go is the implementation language. All Go code MUST follow the project's idiomatic
standards, with context.Context threading through every operation.

- **Context as first parameter** for any cancellable or timeout-capable operation
- **Never store context in structs**—pass explicitly through function calls
- **Accept interfaces, return concrete types**—small, focused interfaces at point of use
- **No global state**—use dependency injection and explicit constructors
- **No `init()` functions**—prefer explicit initialization with `NewXxx()` constructors
- **Wrap errors at package boundaries** with `fmt.Errorf("context: %w", err)`

*Rationale*: Idiomatic Go enables AI agents and human developers to read and modify code
with predictable patterns. Context enables cancellation and observability.

### VI. Validation Before Delivery

Code MUST pass all validation gates before being committed or delivered. Validation
commands are configurable but enforcement is not optional.

- **Format first**: `magex format:fix` ensures consistent style
- **Lint always**: `magex lint` catches issues early
- **Test rigorously**: `magex test:race` with race detection enabled
- **Pre-commit hooks**: `go-pre-commit run --all-files` for final verification
- **Parallel execution**: Lint and test run concurrently after format completes
- **AI retry on failure**: ATLAS can invoke AI to fix validation failures (configurable)

*Rationale*: Quality gates prevent broken code from entering the repository. Automation
catches mistakes that humans miss under fatigue.

### VII. Transparent State

Every file ATLAS creates MUST be inspectable. No hidden state, no opaque databases. Users
can always understand what happened by reading files.

- **Task state** in `~/.atlas/workspaces/<name>/tasks/<id>/task.json`
- **Execution logs** in JSON-lines format at `task.log`
- **Artifacts** (analysis, specs, PR descriptions) in `artifacts/` directory
- **Workspace metadata** in `workspace.json` with full task history
- **Configuration sources** visible via `atlas config show`

*Rationale*: When users can inspect state, they can debug issues, recover from failures,
and trust the system's behavior.

## Technology Stack

ATLAS is a pure Go application targeting Go 1.24+ with minimal dependencies. Every
dependency MUST justify its existence.

### Core Dependencies

| Purpose | Library | Non-Negotiable? |
|---------|---------|-----------------|
| CLI Framework | `spf13/cobra` | Yes |
| Interactive Forms | `charmbracelet/huh` | Yes |
| Terminal Styling | `charmbracelet/lipgloss` | Yes |
| Progress/Spinners | `charmbracelet/bubbles` | Yes |
| Configuration | `spf13/viper` | Yes |
| Structured Logging | `rs/zerolog` | Yes |

### External Tools (Required)

| Tool | Purpose | Version |
|------|---------|---------|
| `claude` CLI | AI execution (primary) | 2.0.76+ |
| `gh` CLI | GitHub operations | 2.20+ |
| `git` | Version control | 2.20+ |

### What We Don't Use

- **No database**—all state is file-based
- **No web framework**—no HTTP server in MVP
- **No dependency injection framework**—explicit wiring only
- **No LangChain/ADK/Genkit**—AI CLI tools handle complexity

## Development Workflow

### Quality Gates

All code MUST pass these gates before merge:

```bash
magex format:fix && magex lint && magex test:race && go-pre-commit run --all-files
```

### Testing Standards

- **Table-driven tests** with descriptive names: `TestFunctionNameScenarioDescription`
- **Use `testify`**: `require` for fatal checks, `assert` for non-fatal
- **Mock external dependencies**—tests must be fast and deterministic
- **Thread-safe mocks** use `sync.Mutex`
- **Use `t.TempDir()`** for temporary files
- **Target 90%+ coverage** for new code

### Commit Conventions

- **Format**: `<type>(<scope>): <imperative description>`
- **Types**: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `build`, `ci`
- **Atomic commits**: Each commit represents one logical change
- **Reference issues**: Include `Closes #123` when applicable

### Documentation Updates

When changing CLI commands or features:
1. Update `docs/quick-start.md` with new/changed commands
2. Update `.atlas/config.yaml` schema if configuration changed
3. Update relevant `internal/` package docs if architecture changed

## Governance

This constitution supersedes all other practices within the ATLAS repository. Amendments
require:

1. **Documented rationale** explaining the change
2. **PR approval** from repository maintainers
3. **Migration plan** if change affects existing workflows
4. **Version increment** following semantic versioning:
   - MAJOR: Backward-incompatible principle removals or redefinitions
   - MINOR: New principles added or materially expanded guidance
   - PATCH: Clarifications, wording fixes, non-semantic refinements

All PRs and code reviews MUST verify compliance with these principles. Complexity
deviating from these standards MUST be explicitly justified in the "Complexity Tracking"
section of implementation plans.

For runtime development guidance, see:
- `.github/AGENTS.md` — Technical conventions index
- `.github/CLAUDE.md` — ATLAS-specific patterns and architecture
- `.github/tech-conventions/` — Detailed standards by topic (for this project and other projects)
- `docs/quick-start.md` — User-facing documentation (Keep this updated!)

**Version**: 1.0.0 | **Ratified**: 2026-01-17 | **Last Amended**: 2026-01-17
