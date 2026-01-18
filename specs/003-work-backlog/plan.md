# Implementation Plan: Work Backlog for Discovered Issues

**Branch**: `003-work-backlog` | **Date**: 2026-01-18 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-work-backlog/spec.md`

## Summary

Implement a project-local work backlog system that captures issues discovered during AI-assisted development. The backlog uses individual YAML files in `.atlas/backlog/` directory, enabling frictionless capture (under 5 seconds), zero merge conflicts on concurrent adds, and full git context preservation. The CLI provides both interactive forms for humans and flag-based input for AI agents.

## Technical Context

**Language/Version**: Go 1.24+ (matches existing ATLAS codebase)
**Primary Dependencies**: spf13/cobra (CLI), charmbracelet/huh (forms), charmbracelet/glamour (markdown), gopkg.in/yaml.v3 (YAML)
**Storage**: Individual YAML files in `.atlas/backlog/` directory (one file per discovery)
**Testing**: Standard Go testing with testify (assert/require), table-driven tests, 90%+ coverage target
**Target Platform**: Cross-platform CLI (macOS, Linux, Windows)
**Project Type**: Single Go CLI application (extends existing ATLAS codebase)
**Performance Goals**: Discovery creation < 5 seconds, List 1000+ files < 2 seconds
**Constraints**: No database, file-based only, atomic writes, zero merge conflicts
**Scale/Scope**: Typical project scale (~100-1000 discoveries over project lifetime)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Requirement | Design Compliance | Status |
|-----------|-------------|-------------------|--------|
| **I. Human Authority at Checkpoints** | Humans review/promote discoveries; AI only captures | AI can add discoveries, humans decide what to promote/dismiss | ✅ PASS |
| **II. Git is the Backbone** | All state produces Git artifacts | Individual YAML files in `.atlas/backlog/`, tracks git branch/commit context | ✅ PASS |
| **III. Text is Truth** | Human-readable text files, no databases | YAML files for discoveries, can `cat` to understand | ✅ PASS |
| **IV. Ship Then Iterate** | MVP mindset, no premature abstraction | MVP scope defined (5 user stories), promotion creates metadata only (no task orchestration) | ✅ PASS |
| **V. Context-First Go** | context.Context first parameter, no global state | All Manager methods take ctx, explicit constructors | ✅ PASS |
| **VI. Validation Before Delivery** | Code must pass validation gates | Tested with race detection, follows existing validation patterns | ✅ PASS |
| **VII. Transparent State** | All files inspectable | Discoveries in `.atlas/backlog/`, human-readable YAML | ✅ PASS |

**Pre-Design Gate**: ✅ All principles pass

## Project Structure

### Documentation (this feature)

```text
specs/003-work-backlog/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── discovery-schema.yaml  # YAML schema definition
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── backlog/                 # NEW: Core backlog functionality
│   ├── manager.go           # BacklogManager - directory scanning, CRUD operations
│   ├── manager_test.go      # Comprehensive unit tests
│   ├── types.go             # Discovery struct and related types
│   ├── types_test.go        # Type validation tests
│   └── id.go                # ID generation utilities
│
├── cli/
│   ├── backlog.go           # NEW: 'atlas backlog' command group
│   ├── backlog_test.go      # Command structure and flag tests
│   ├── backlog_add.go       # NEW: 'atlas backlog add' subcommand
│   ├── backlog_add_test.go  # Add command tests
│   ├── backlog_list.go      # NEW: 'atlas backlog list' subcommand
│   ├── backlog_list_test.go # List command tests
│   ├── backlog_view.go      # NEW: 'atlas backlog view <id>' subcommand
│   ├── backlog_view_test.go # View command tests
│   ├── backlog_promote.go   # NEW: 'atlas backlog promote <id>' subcommand
│   ├── backlog_promote_test.go
│   ├── backlog_dismiss.go   # NEW: 'atlas backlog dismiss <id>' subcommand
│   └── backlog_dismiss_test.go
│
├── domain/
│   └── discovery.go         # NEW: Discovery domain model (if needed separately)
│
└── errors/
    └── errors.go            # ADD: Discovery-specific sentinel errors

.atlas/
└── backlog/                 # Runtime: Discovery storage directory
    ├── .gitkeep             # Ensures directory exists in git
    ├── disc-a1b2c3.yaml     # Individual discovery files
    └── disc-x9y8z7.yaml
```

**Structure Decision**: Follows existing ATLAS patterns - new `internal/backlog/` package for core logic, CLI commands in `internal/cli/backlog*.go`, domain types can stay in the backlog package (simpler than separate domain file for MVP).

## Complexity Tracking

> No constitution violations identified. Design is intentionally minimal.

| Aspect | Chosen Approach | Rationale |
|--------|-----------------|-----------|
| Storage | One YAML file per discovery | Constitution Principle II (Git backbone), avoids merge conflicts |
| No task orchestration | Promotion only records task ID | Constitution Principle IV (Ship Then Iterate), defer integration |
| Storage | One YAML file per discovery | Constitution Principle II (Git backbone), avoids merge conflicts |
| Safety | O_EXCL for creation | "Senior Level" defensive programming against ID collisions |
| UX | Rich Terminal Rendering (glamour) | **Justified Exception**: "Amazing" experience requirement overrides minimal dependency rule (Constitution Tech Stack) |
| No task orchestration | Promotion only records task ID | Constitution Principle IV (Ship Then Iterate), defer integration |
| Types in backlog pkg | Keep Discovery type with Manager | MVP simplicity, can extract later if needed |

---

## Post-Design Constitution Check

*Re-evaluated after Phase 1 design completion.*

| Principle | Requirement | Final Design Verification | Status |
|-----------|-------------|---------------------------|--------|
| **I. Human Authority** | AI proposes, humans dispose | ✓ AI captures discoveries; humans review via `list`, decide via `promote`/`dismiss` | ✅ PASS |
| **II. Git Backbone** | All state in Git artifacts | ✓ YAML files in `.atlas/backlog/` with git context (branch, commit) | ✅ PASS |
| **III. Text is Truth** | Human-readable, no databases | ✓ YAML files, `cat`-able, schema documented | ✅ PASS |
| **IV. Ship Then Iterate** | MVP, no premature abstraction | ✓ 5 commands, no task orchestration, promotion is metadata-only | ✅ PASS |
| **V. Context-First Go** | context.Context threading | ✓ Manager methods take `ctx`, explicit `NewManager()` constructor | ✅ PASS |
| **VI. Validation Before Delivery** | Code passes gates | ✓ Race-tested concurrent writes, table-driven tests, 90%+ coverage | ✅ PASS |
| **VII. Transparent State** | Inspectable files | ✓ Discoveries in project `.atlas/backlog/`, JSON output for scripting | ✅ PASS |

**Post-Design Gate**: ✅ All principles pass

---

## Generated Artifacts

| Artifact | Path | Purpose |
|----------|------|---------|
| Implementation Plan | `specs/003-work-backlog/plan.md` | This file |
| Research | `specs/003-work-backlog/research.md` | Technical decisions and rationale |
| Data Model | `specs/003-work-backlog/data-model.md` | Entity definitions and Go types |
| Schema | `specs/003-work-backlog/contracts/discovery-schema.yaml` | JSON Schema for discovery YAML |
| Quick Start | `specs/003-work-backlog/quickstart.md` | User documentation |

---

## Next Steps

Run `/speckit.tasks` to generate the implementation task breakdown in `specs/003-work-backlog/tasks.md`.
