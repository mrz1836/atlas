# Research Agent (Deferred)

> **Status:** Deferred until ATLAS tracks 5+ SDD frameworks
> **See also:** [vision-v2.md - What's Deferred](../external/vision-v2.md#7-whats-deferred)

---

## Why This Will Exist (Later)

The SDD and AI-assisted development landscape moves fast. Frameworks release breaking changes, new tools emerge, and APIs evolve. When ATLAS integrates 5+ frameworks, manual monitoring becomes unsustainable.

Until then, manual monitoring is fine. ATLAS v1 integrates only Speckit, so a maintainer periodically checking for updates is sufficient.

---

## Core Concept

When needed, the Research Agent will operate in two modes:

**Tracking Mode** — Monitor integrated frameworks for changes that affect ATLAS:
- Version releases and breaking changes
- CLI signature changes
- Deprecation notices

**Discovery Mode** — Find emerging SDD tools worth evaluating:
- GitHub trending repos in relevant topics
- Community discussions mentioning new approaches
- Evaluate based on activity, adoption, and alignment with ATLAS

---

## What It Might Look Like

If implemented as an `atlas` subcommand (not a separate CLI):

```bash
atlas research status     # Show tracked frameworks and their versions
atlas research check      # Check for updates across all tracked frameworks
```

Output would be simple and actionable:
```
Tracked Frameworks:
  speckit    2.1.0 (current) → 2.2.0 available
  bmad       1.5.0 (current) — up to date
```

---

## Signals to Watch

When tracking a framework:

| Signal | Why It Matters |
|--------|----------------|
| Version releases | New features, bug fixes |
| Breaking changes | Adapter compatibility |
| CLI changes | Command signature updates |
| Deprecation notices | Migration planning |

---

## Implementation Deferred

Per ATLAS v2's "Ship Then Iterate" principle, detailed implementation (automation, data structures, GitHub Actions workflows, etc.) will be designed when the feature is actually needed.

The trigger for revisiting: **ATLAS integrates 5+ SDD frameworks and manual monitoring becomes a burden.**

---

## Original Design Notes

<details>
<summary>Click to expand original detailed design (preserved for future reference)</summary>

### Original Two-Mode System

The original design described a more elaborate system:

**Tracking Mode watched:**
- GitHub Releases API, NPM/PyPI for version releases
- Changelog files and release notes for breaking changes
- CLI help output diffs for command signature changes
- Package manifests for dependency updates

**Discovery Mode scanned:**
- GitHub Trending (topics: `spec-driven`, `ai-coding`, `llm-development`)
- Hacker News, Reddit (r/programming, r/MachineLearning)
- Twitter/X threads from AI dev tooling thought leaders
- Academic papers, conference talks

**Evaluation criteria for discoveries:**
- Activity: Commits in last 30 days, issue response time
- Adoption: Stars, forks, dependents count
- Documentation: README quality, examples, API docs
- Alignment: Does it fit ATLAS architecture?
- Uniqueness: Does it offer capabilities we lack?
- Maintenance: Bus factor, release cadence

### Original CLI Design

The original design proposed a separate `atlas-research` CLI with command groups:
- `atlas-research track list|status|scan|diff|add|remove`
- `atlas-research discover run|digest|evaluate|pending`
- `atlas-research alerts list|show|ack|approve|defer`
- `atlas-research queue list|remove|changelog|release`
- `atlas-research report compatibility|health`

### Original Data Storage

Structured JSON files in `.atlas-internal/research/`:
- `config.json` — Research Agent configuration
- `tracked/*.json` — Per-framework tracking state
- `discoveries/*.json` — Daily discovery results
- `alerts/*.json` — Pending, acknowledged, historical alerts
- `queue/pending.json` — Updates approved for release

### Original Automation

GitHub Actions workflow with:
- Daily tracking check at 06:00 UTC
- Weekly discovery scan on Sundays at 08:00 UTC
- Auto-creation of GitHub issues for alerts
- Commit research data back to repo

### Future Enhancements (from original)

- AI-assisted changelog summarization
- Breaking change detection via code diff analysis
- Migration guide auto-generation
- Trend prediction for emerging tools

</details>

---

*This document captures the concept for future implementation. When the time comes, design the simplest solution that works.*
