# ATLAS Work Backlog: Discovered Issues Tracking

## Technical Specification v1.1

**Author:** Z (Refactored by Senior Agent)
**Date:** January 2026
**Status:** Design Specification
**Target:** ATLAS AI Task Lifecycle Automation System

---

## Executive Summary

The Work Backlog captures issues discovered during AI-assisted development that can't be addressed in the current task. It's a lightweight, project-local queue that prevents good observations from getting lost.

**Core Principle:** The AI should never silently pass over problems it notices. Every discovery gets recorded, nothing falls through the cracks.

---

## Constitution Alignment

| Principle | Alignment | Status |
|-----------|-----------|--------|
| I. Human Authority at Checkpoints | Human reviews/promotes discoveries; AI only captures observations | ✅ PASS |
| II. Git is the Backbone | Individual YAML files in `.atlas/backlog/` allow atomic commits and zero merge conflicts | ✅ PASS |
| III. Text is Truth | YAML format, human-editable, `cat`-able, file-based | ✅ PASS |
| IV. Ship Then Iterate | MVP scope defined; directory structure scales naturally | ✅ PASS |
| V. Context-First Go | `context.Context` in all Manager operations | ✅ PASS |
| VI. Validation Before Delivery | Backlog operations tested with race detection | ✅ PASS |
| VII. Transparent State | Backlog visible in project `.atlas/` directory | ✅ PASS |

---

## Problem Statement

### The Discovery Problem

During implementation, the AI frequently notices things:
- "This function has no error handling"
- "There's a potential race condition here"
- "This test file is missing coverage for edge cases"

Currently, these observations are either **Lost**, **Scope creep**, or require **Manual tracking**.

### The Storage Problem (Addressed in v1.1)

A single backlog file creates **Git Merge Conflicts** when multiple developers (or worktrees) add items simultaneously. This friction discourages usage.

**Solution:** Dictionary-based storage (One file per discovery).

### Success Criteria

| ID | Criterion | Measurement |
|----|-----------|-------------|
| SC-001 | Discovery captured in < 5 seconds | Time from command to confirmation |
| SC-002 | Zero merge conflicts on concurrent adds | Git merge behavior verification |
| SC-003 | Human "Delight" factor | Interactive form usage vs flags |
| SC-004 | Context preservation | Git sha/branch stored with every item |

---

## Design Philosophy

### Principles

1.  **Frictionless Capture**: Recording a discovery should be effortless. For AI -> Flags. For Humans -> Interactive Forms.
2.  **Git-Native Storage**: One file per item (`.atlas/backlog/<id>.yaml`). Atomic commits.
3.  **Context is King**: Always record *where* and *when* something was found (commit, branch, author).
4.  **Project-Local**: Travels with the code.

### Development Context

**ATLAS is unreleased software.** We prioritize the *right* design over backwards compatibility.

---

## Data Model

### Storage Structure

```text
<project-root>/
├── .atlas/
│   ├── config.yaml
│   └── backlog/              # Directory container
│       ├── .gitkeep
│       ├── disc-a1b2c3.yaml  # Individual discovery
│       ├── disc-x9y8z7.yaml
│       └── ...
```

### Discovery Schema (YAML)

Each file follows this schema:

```yaml
# .atlas/backlog/disc-a1b2c3.yaml
schema_version: "1.0"
id: "disc-a1b2c3"
title: "Missing error handling in config.Parse"
status: "pending" # pending, promoted, dismissed

# Core Content
content:
  description: |
    The Parse function doesn't handle the case where the config file
    exists but is empty.
  category: "bug"          # bug, tech-debt, security, etc.
  severity: "medium"       # critical, high, medium, low
  tags: ["config", "error-handling"]

# precise location context
location:
  file: "config/parser.go"
  line: 47

# Historical Context (The "When" and "Who")
context:
  discovered_at: "2026-01-17T14:32:15Z"
  discovered_during_task: "task-20260117-143022"
  discovered_by: "claude-sonnet" # or "human-username"
  git:
    branch: "feat/auth-refactor"
    commit: "7b3f1a2" # SHA at time of discovery

# Lifecycle
lifecycle:
  promoted_to_task: ""    # Task ID if promoted
  dismissed_reason: ""    # Reason if dismissed
```

---

## Go Implementation

### Types

```go
// internal/backlog/types.go

type Status string
const (
    StatusPending   Status = "pending"
    StatusPromoted  Status = "promoted"
    StatusDismissed Status = "dismissed"
)

type Discovery struct {
    SchemaVersion string           `yaml:"schema_version"`
    ID            string           `yaml:"id"`
    Title         string           `yaml:"title"`
    Status        Status           `yaml:"status"`
    Content       DiscoveryContent `yaml:"content"`
    Location      Location         `yaml:"location"`
    Context       Context          `yaml:"context"`
    Lifecycle     Lifecycle        `yaml:"lifecycle"`
}

type DiscoveryContent struct {
    Description string   `yaml:"description"`
    Category    string   `yaml:"category"`
    Severity    string   `yaml:"severity"`
    Tags        []string `yaml:"tags,omitempty"`
}

type Location struct {
    File string `yaml:"file,omitempty"`
    Line int    `yaml:"line,omitempty"`
}

type Context struct {
    DiscoveredAt   time.Time  `yaml:"discovered_at"`
    DuringTask     string     `yaml:"discovered_during_task,omitempty"`
    By             string     `yaml:"discovered_by"`
    Git            GitContext `yaml:"git"`
}

type GitContext struct {
    Branch string `yaml:"branch"`
    Commit string `yaml:"commit"`
}
```

### Manager Logic

The `Manager` no longer holds a big struct in memory. It scans the directory.

```go
// internal/backlog/manager.go

// List scans the .atlas/backlog directory
func (m *Manager) List(ctx context.Context, filter Filter) ([]Discovery, error) {
     pattern := filepath.Join(m.rootDir, ".atlas", "backlog", "*.yaml")
     matches, _ := filepath.Glob(pattern)

     var results []Discovery
     for _, file := range matches {
         // Load file...
         // Apply filter...
     }
     return results, nil
}

// Save writes a single file atomically
func (m *Manager) Save(d *Discovery) error {
    filename := filepath.Join(m.rootDir, ".atlas", "backlog", d.ID+".yaml")
    // Marshal to YAML
    // WriteFile
    return nil
}
```

---

## CLI & User Experience

### 1. Interactive Add (Human Mode)

When a user runs `atlas backlog add` without arguments, we launch a **Bubble Tea** form (`charmbracelet/huh`).

```bash
atlas backlog add
```

**UX Flow:**

```text
? What did you find?
  > [ Text Input for Title ]

? Details (Optional):
  > [ Text Area for Description ]

? Category:
  > Bug
    Tech Debt
    Security
    Tests
    Docs

? Severity:
  > Medium
    High
    Critical
    Low

[ Submit ]
```

### 2. Fast Add (AI/Script Mode)

Full flag support for headless operation.

```bash
atlas backlog add "Title" \
  --file "main.go" \
  --severity high \
  --json
```

---

## Testing Plan

### Use Case Coverage

1.  **Concurrent Writes**: Verify that 10 goroutines adding discoveries simultaneously result in 10 distinct files and 0 errors.
2.  **Git Context**: Mock the git environment and verify `Save()` captures the current sha/branch correctly.
3.  **Filtering**: Create 50 dummy files with different metadata, verify `List()` filters efficiently.
4.  **Round Trip**: Save -> Load -> Verify every field exactly matches (including timestamps).

---

## AI Protocols (`CLAUDE.md`)

Update instructions to emphasize the new capability:

> **Discovery Protocol**: If you see an issue outside your current task scope, DO NOT ignore it.
> Run `atlas backlog add "<Title>" --file <path> ...` immediately.
> Then continue your task.
