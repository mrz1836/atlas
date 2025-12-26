# Research Agent: Maintainer Documentation

> **Important:** This is internal tooling for ATLAS maintainers. End users should never interact with the Research Agent directly—they simply run `atlas upgrade` to receive updates.

## Overview

The Research Agent is the intelligence layer that keeps ATLAS current with the rapidly evolving SDD ecosystem. It operates in two modes:

1. **Tracking Mode** — Monitor integrated frameworks for changes
2. **Discovery Mode** — Find emerging tools and approaches

All discoveries flow through a maintainer review process before being packaged into ATLAS releases.

---

## Why This Exists

The SDD and AI-assisted development landscape moves fast:

- Frameworks release breaking changes
- New tools emerge that solve problems better than existing integrations
- APIs and CLI interfaces evolve
- Dependencies have security vulnerabilities

Without automated tracking, ATLAS would:
- Break silently when upstream frameworks change
- Miss valuable new tools
- Require manual monitoring of dozens of projects
- Lag behind the ecosystem

The Research Agent automates this vigilance.

---

## Two Operating Modes

### 1. Tracking Mode (Known Frameworks)

Monitor integrated frameworks for changes that affect ATLAS.

#### What It Watches

| Signal | Data Source | Why It Matters |
|--------|-------------|----------------|
| Version releases | GitHub Releases API, NPM/PyPI | New features, bug fixes |
| Breaking changes | Changelog files, release notes | Adapter compatibility |
| CLI changes | Help output diff, docs | Command signature changes |
| Dependency updates | Package manifests | Transitive security issues |
| Deprecation notices | Issues, docs, release notes | Migration planning |

#### Tracked Frameworks (MVP)

| Framework | Package Manager | Repository |
|-----------|-----------------|------------|
| Speckit | pip (uv) | github.com/speckit/speckit |
| BMAD | npm | github.com/bmad-method/bmad |

#### Tracking Output

For each tracked framework, the Research Agent produces:

```json
{
  "framework": "speckit",
  "current_integrated_version": "2.1.0",
  "latest_version": "2.2.0",
  "update_available": true,
  "breaking_changes": false,
  "changelog_summary": [
    "New validation rules for API specs",
    "Performance improvements in large codebases"
  ],
  "cli_changes": [],
  "security_advisories": [],
  "last_checked": "2025-12-26T10:30:00Z",
  "recommended_action": "safe_to_update"
}
```

### 2. Discovery Mode (New Frameworks)

Proactively find emerging SDD tools and approaches before they become mainstream.

#### Discovery Signals

| Source | What To Look For |
|--------|------------------|
| GitHub Trending | Repos in topics: `spec-driven`, `ai-coding`, `llm-development` |
| Hacker News | Posts mentioning specification-driven, AI coding assistants |
| Reddit | r/programming, r/MachineLearning posts on AI dev tools |
| Twitter/X | Threads from AI dev tooling thought leaders |
| Academic | Papers on specification-driven development, formal methods |
| Conferences | Talks from AI Engineer Summit, Strange Loop |

#### Evaluation Criteria

When a new project is discovered, it's evaluated on:

| Criterion | Weight | Measurement |
|-----------|--------|-------------|
| Activity | High | Commits in last 30 days, issue response time |
| Adoption | High | Stars, forks, dependents count |
| Documentation | Medium | README quality, examples, API docs |
| Alignment | High | Does it fit ATLAS architecture? |
| Uniqueness | Medium | Does it offer capabilities we lack? |
| Maintenance | High | Bus factor, release cadence |

#### Discovery Output

```json
{
  "project": "new-sdd-framework",
  "url": "github.com/org/new-sdd-framework",
  "discovered_via": "github_trending",
  "discovered_at": "2025-12-26T08:00:00Z",
  "evaluation": {
    "stars": 1250,
    "stars_growth_30d": 450,
    "last_commit": "2025-12-25",
    "open_issues": 23,
    "contributors": 8,
    "documentation_score": 0.85,
    "alignment_notes": "Uses similar phase model to ATLAS SDD adapters"
  },
  "recommendation": "evaluate_for_integration",
  "summary": "Promising new SDD framework with strong adoption trajectory. Focus on API-first development with built-in mock generation."
}
```

---

## Maintainer Workflow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         RESEARCH AGENT WORKFLOW                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  AUTOMATED (Scheduled via GitHub Actions / Cron)                            │
│  ────────────────────────────────────────────────                           │
│                                                                             │
│  Daily @ 06:00 UTC                                                          │
│  ├── Check all tracked framework releases                                   │
│  ├── Diff CLI outputs for signature changes                                 │
│  └── Generate alerts for any changes                                        │
│                                                                             │
│  Weekly (Sundays @ 08:00 UTC)                                               │
│  ├── Run full discovery scan                                                │
│  ├── Evaluate new candidates                                                │
│  └── Generate weekly digest                                                 │
│                                                                             │
│                              │                                              │
│                              ▼                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                    MAINTAINER REVIEW DASHBOARD                         │  │
│  │                                                                        │  │
│  │  ALERTS (Require Action)                                               │  │
│  │  ────────────────────────                                              │  │
│  │  🔴 speckit 2.1.0 → 2.2.0 — CLI change detected                        │  │
│  │  🟡 bmad 1.5.0 → 1.5.1 — Bug fix release                               │  │
│  │                                                                        │  │
│  │  DISCOVERIES (For Review)                                              │  │
│  │  ──────────────────────────                                            │  │
│  │  🆕 new-sdd-framework — 1.2k stars, growing fast                       │  │
│  │  🆕 spec-validator-x — Unique validation approach                      │  │
│  │                                                                        │  │
│  │  UPDATE QUEUE (Approved)                                               │  │
│  │  ───────────────────────                                               │  │
│  │  ✅ github-adapter 1.1.0 → 1.1.1 — Ready for release                   │  │
│  │                                                                        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│                              │                                              │
│                              ▼                                              │
│  MAINTAINER ACTIONS                                                         │
│  ──────────────────                                                         │
│                                                                             │
│  For Alerts:                                                                │
│  ├── Acknowledge — Mark as reviewed, no action needed                       │
│  ├── Investigate — Need more info before deciding                           │
│  ├── Approve — Add to update queue for next release                         │
│  └── Defer — Not now, revisit later                                         │
│                                                                             │
│  For Discoveries:                                                           │
│  ├── Dismiss — Not relevant to ATLAS                                        │
│  ├── Watch — Add to tracking without integration                            │
│  ├── Evaluate — Deep-dive to assess integration feasibility                 │
│  └── Integrate — Create task to build adapter                               │
│                                                                             │
│                              │                                              │
│                              ▼                                              │
│  RELEASE PIPELINE                                                           │
│  ────────────────                                                           │
│                                                                             │
│  1. Approved updates collected                                              │
│  2. Compatibility tests run                                                 │
│  3. Changelog auto-generated from alerts                                    │
│  4. Release tagged and published                                            │
│  5. Users receive via `atlas upgrade`                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## CLI Interface

The Research Agent uses a separate CLI (`atlas-research`) to avoid polluting the user-facing `atlas` command namespace.

### Tracking Commands

```bash
# List all tracked frameworks and their status
atlas-research track list

# Check for updates across all tracked frameworks
atlas-research track status

# Deep scan a specific framework
atlas-research track scan speckit

# Show changes since last check
atlas-research track diff speckit

# Add a new framework to track
atlas-research track add <name> --repo <github-url> --package-manager <npm|pip>

# Remove a framework from tracking
atlas-research track remove <name>
```

### Discovery Commands

```bash
# Run a discovery scan
atlas-research discover run

# Generate weekly digest of discoveries
atlas-research discover digest

# Evaluate a specific project URL
atlas-research discover evaluate https://github.com/org/project

# List pending discoveries awaiting review
atlas-research discover pending
```

### Alert Management

```bash
# List all pending alerts
atlas-research alerts list

# Show details for a specific alert
atlas-research alerts show <alert-id>

# Acknowledge an alert (mark as reviewed)
atlas-research alerts ack <alert-id>

# Approve an alert for the update queue
atlas-research alerts approve <alert-id>

# Defer an alert for later
atlas-research alerts defer <alert-id> --until 2025-01-15
```

### Update Queue

```bash
# List approved updates ready for release
atlas-research queue list

# Remove an item from the queue
atlas-research queue remove <item-id>

# Generate changelog from queue items
atlas-research queue changelog

# Tag a release with queued items
atlas-research queue release --version 1.3.0
```

### Reports

```bash
# Framework compatibility matrix
atlas-research report compatibility

# Ecosystem health summary
atlas-research report health

# Export all data for backup
atlas-research export --output research-backup.json
```

---

## Data Storage

All research data is stored as structured JSON files, designed for Git-friendliness and easy inspection.

```
.atlas-internal/
├── research/
│   ├── config.json              # Research Agent configuration
│   ├── tracked/
│   │   ├── speckit.json         # Tracking state for Speckit
│   │   ├── bmad.json            # Tracking state for BMAD
│   │   └── ...
│   ├── discoveries/
│   │   ├── 2025-12-26.json      # Daily discovery results
│   │   ├── 2025-12-25.json
│   │   └── ...
│   ├── alerts/
│   │   ├── pending.json         # Alerts awaiting review
│   │   ├── acknowledged.json    # Reviewed alerts
│   │   └── history.json         # All past alerts
│   ├── queue/
│   │   └── pending.json         # Updates approved for release
│   └── reports/
│       ├── compatibility.json   # Latest compatibility matrix
│       └── health.json          # Latest health report
```

### Example: Tracked Framework State

```json
{
  "name": "speckit",
  "repository": "github.com/speckit/speckit",
  "package_manager": "pip",
  "integrated_version": "2.1.0",
  "latest_known_version": "2.2.0",
  "tracking_since": "2025-11-01T00:00:00Z",
  "last_check": "2025-12-26T06:00:00Z",
  "check_history": [
    {
      "timestamp": "2025-12-26T06:00:00Z",
      "version_found": "2.2.0",
      "alerts_generated": ["alert-speckit-220"]
    }
  ],
  "cli_signature_hash": "sha256:abc123...",
  "notes": "Primary SDD framework for ATLAS"
}
```

---

## Automation Setup

### GitHub Actions Workflow

```yaml
# .github/workflows/research-agent.yml
name: Research Agent

on:
  schedule:
    # Daily tracking check at 06:00 UTC
    - cron: '0 6 * * *'
    # Weekly discovery scan on Sundays at 08:00 UTC
    - cron: '0 8 * * 0'
  workflow_dispatch:
    inputs:
      mode:
        description: 'Run mode'
        required: true
        type: choice
        options:
          - tracking
          - discovery
          - full

jobs:
  research:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run Research Agent
        run: |
          if [ "${{ github.event_name }}" == "schedule" ]; then
            # Determine mode from schedule
            HOUR=$(date -u +%H)
            if [ "$HOUR" == "06" ]; then
              atlas-research track status --output alerts.json
            else
              atlas-research discover run --output discoveries.json
            fi
          else
            # Manual trigger
            atlas-research ${{ inputs.mode }}
          fi

      - name: Create Issue for Alerts
        if: steps.research.outputs.has_alerts == 'true'
        uses: actions/github-script@v7
        with:
          script: |
            // Create GitHub issue with alert summary
            // Assign to maintainers

      - name: Commit Research Data
        run: |
          git config user.name "ATLAS Research Agent"
          git config user.email "atlas-research@noreply.github.com"
          git add .atlas-internal/research/
          git commit -m "research: daily tracking update" || echo "No changes"
          git push
```

---

## Future Enhancements

### AI-Assisted Analysis

The Research Agent can leverage AI to:

- **Changelog Summarization** — Extract relevant changes from verbose release notes
- **Breaking Change Detection** — Analyze code diffs to identify API changes
- **Integration Feasibility** — Assess how difficult it would be to build an adapter
- **Migration Guide Generation** — Auto-generate upgrade guides for breaking changes
- **Trend Prediction** — Identify which emerging tools are likely to gain traction

### Community Signals

- Monitor GitHub Discussions for ATLAS for feature requests related to new frameworks
- Track which SDD frameworks are mentioned in ATLAS issues
- Survey users about desired integrations

### Cross-Project Learning

- Share discovery data with other orchestration projects (with consent)
- Contribute to a shared SDD framework registry
- Participate in ecosystem health monitoring

---

## Troubleshooting

### Common Issues

**Problem:** Discovery scan returns too many low-quality results
**Solution:** Adjust minimum star threshold in `config.json`, increase `documentation_score` requirement

**Problem:** Tracking alerts generated for patch versions
**Solution:** Configure alert thresholds to only trigger on minor/major versions

**Problem:** CLI signature hash changed but no actual breaking change
**Solution:** Update the stored hash after manual verification

### Debug Mode

```bash
# Enable verbose logging
atlas-research --verbose track status

# Dry run without persisting changes
atlas-research --dry-run discover run

# Output raw API responses
atlas-research --debug track scan speckit
```

---

## Contributing

To add a new framework to tracking:

1. Open an issue with the framework details
2. Verify the framework meets evaluation criteria
3. Add tracking configuration via PR
4. Update the compatibility matrix

To improve discovery signals:

1. Propose new data sources via issue
2. Implement scraper/API integration
3. Add evaluation criteria if needed
4. Test with historical data
