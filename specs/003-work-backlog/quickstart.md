# Work Backlog: Quick Start Guide

**Feature Branch**: `003-work-backlog`

The Work Backlog captures issues discovered during AI-assisted development that can't be addressed in the current task. It's a lightweight, project-local queue that prevents good observations from getting lost.

## Quick Reference

```bash
# Add a discovery (interactive)
atlas backlog add

# Add a discovery (AI/script mode)
atlas backlog add "Missing error handling" --file main.go --line 47 --category bug --severity high

# List pending discoveries
atlas backlog list

# View discovery details
atlas backlog view disc-a1b2c3

# Promote to task
atlas backlog promote disc-a1b2c3 --task-id task-20260118-150000

# Dismiss with reason
atlas backlog dismiss disc-a1b2c3 --reason "duplicate of disc-x9y8z7"
```

---

## Commands

### atlas backlog add

Add a new discovery to the backlog.

**Interactive mode** (for humans):
```bash
atlas backlog add
```
Launches a guided form to capture:
- Title (required)
- Description (optional)
- Category (select)
- Severity (select)
- File location (optional)
- Line number (optional)

**Flag mode** (for AI/scripts):
```bash
atlas backlog add "Title of discovery" \
  --file path/to/file.go \
  --line 47 \
  --category bug \
  --severity high \
  --description "Detailed explanation" \
  --tags "error-handling,config" \
  --json
```

| Flag | Short | Description | Values |
|------|-------|-------------|--------|
| `--file` | `-f` | File path where issue was found | Relative path |
| `--line` | `-l` | Line number in file | Positive integer |
| `--category` | `-c` | Issue category | `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation` |
| `--severity` | `-s` | Priority level | `low`, `medium`, `high`, `critical` |
| `--description` | `-d` | Detailed explanation | String |
| `--tags` | `-t` | Comma-separated labels | String |
| `--json` | | Output created discovery as JSON | Flag |

**Output**:
```
Created discovery: disc-a1b2c3
  Title: Missing error handling
  Category: bug | Severity: high
  Location: main.go:47
```

---

### atlas backlog list

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

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--status` | | Filter by status | `pending` |
| `--category` | `-c` | Filter by category | All |
| `--all` | `-a` | Include dismissed items | `false` |
| `--limit` | `-n` | Maximum items to show | Unlimited |
| `--json` | | Output as JSON array | `false` |

**Output**:
```
ID           TITLE                           CATEGORY  SEVERITY  AGE
disc-a1b2c3  Missing error handling          bug       high      2h
disc-x9y8z7  Potential race condition        bug       critical  1d
disc-p4q5r6  Add test for edge case          testing   medium    3d
```

---

### atlas backlog view

View full details of a discovery.

```bash
atlas backlog view disc-a1b2c3

# JSON output
atlas backlog view disc-a1b2c3 --json
```

**Output**:
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

---

### atlas backlog promote

Promote a discovery to an ATLAS task.

```bash
# Promote with task ID
atlas backlog promote disc-a1b2c3 --task-id task-20260118-150000

# JSON output
atlas backlog promote disc-a1b2c3 --task-id task-20260118-150000 --json
```

| Flag | Description | Required |
|------|-------------|----------|
| `--task-id` | ATLAS task ID to link | Yes |

**Output**:
```
Promoted discovery disc-a1b2c3
  Linked to task: task-20260118-150000
```

**Note**: This MVP only records the task ID in the discovery metadata. Full task creation integration is planned for a future release.

---

### atlas backlog dismiss

Dismiss a discovery with a reason.

```bash
# Dismiss with reason
atlas backlog dismiss disc-a1b2c3 --reason "Duplicate of disc-x9y8z7"

# JSON output
atlas backlog dismiss disc-a1b2c3 --reason "Won't fix" --json
```

| Flag | Description | Required |
|------|-------------|----------|
| `--reason` | Explanation for dismissal | Yes |

**Output**:
```
Dismissed discovery disc-a1b2c3
  Reason: Duplicate of disc-x9y8z7
```

---

## Storage Location

Discoveries are stored as individual YAML files in your project:

```
<project-root>/
└── .atlas/
    └── backlog/
        ├── .gitkeep
        ├── disc-a1b2c3.yaml
        ├── disc-x9y8z7.yaml
        └── disc-p4q5r6.yaml
```

Each file is human-readable and can be inspected with `cat`:

```bash
cat .atlas/backlog/disc-a1b2c3.yaml
```

---

## AI Agent Protocol

For AI agents (Claude Code, Gemini, Codex), add to your CLAUDE.md or equivalent:

```markdown
**Discovery Protocol**: If you see an issue outside your current task scope, DO NOT ignore it.
Run `atlas backlog add "<Title>" --file <path> --category <type> --severity <level>` immediately.
Then continue your task.
```

This ensures no good observations get lost during automated development.

---

## Example Workflow

```bash
# 1. During a task, AI discovers an unrelated issue
atlas backlog add "Missing nil check in HTTP handler" \
  --file internal/api/handler.go \
  --line 89 \
  --category bug \
  --severity high \
  --description "The handler doesn't check if request.Body is nil before reading"

# 2. Later, review the backlog
atlas backlog list

# 3. View details of interesting items
atlas backlog view disc-a1b2c3

# 4. Decide to fix it - create a task and promote
atlas start "Fix nil check in HTTP handler" --template bugfix
atlas backlog promote disc-a1b2c3 --task-id task-20260118-153000

# 5. Or dismiss if not needed
atlas backlog dismiss disc-x9y8z7 --reason "Already fixed in PR #123"
```

---

## JSON Output

All commands support `--json` flag for programmatic access:

```bash
# Add and get JSON
atlas backlog add "Issue" --category bug --severity low --json

# List as JSON array
atlas backlog list --json | jq '.[].id'

# View as JSON
atlas backlog view disc-a1b2c3 --json
```

---

## Tips

1. **Quick capture**: Use minimal flags, add details later by editing the YAML directly
2. **Git-friendly**: Each discovery is a separate file - no merge conflicts
3. **Browse with grep**: `grep -r "severity: critical" .atlas/backlog/`
4. **Edit directly**: YAML files are human-editable for corrections
5. **Track patterns**: Use tags to identify recurring issue types
