<div align="center">

# 🗺️&nbsp;&nbsp;Atlas

**AI handles the tedium. You focus on the craft.**

<br/>

<a href="https://github.com/mrz1836/atlas/releases"><img src="https://img.shields.io/github/release-pre/mrz1836/atlas?include_prereleases&style=flat-square&logo=github&color=black" alt="Release"></a>
<a href="https://golang.org/"><img src="https://img.shields.io/github/go-mod/go-version/mrz1836/atlas?style=flat-square&logo=go&color=00ADD8" alt="Go Version"></a>
<a href="https://github.com/mrz1836/atlas/blob/master/LICENSE"><img src="https://img.shields.io/github/license/mrz1836/atlas?style=flat-square&color=blue&v=1" alt="License"></a>

<br/>

<table align="center" border="0">
  <tr>
    <td align="right">
       <code>CI / CD</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/mrz1836/atlas/actions"><img src="https://img.shields.io/github/actions/workflow/status/mrz1836/atlas/fortress.yml?branch=master&label=build&logo=github&style=flat-square" alt="Build"></a>
       <a href="https://github.com/mrz1836/atlas/actions"><img src="https://img.shields.io/github/last-commit/mrz1836/atlas?style=flat-square&logo=git&logoColor=white&label=last%20update" alt="Last Commit"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Quality</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://goreportcard.com/report/github.com/mrz1836/atlas"><img src="https://goreportcard.com/badge/github.com/mrz1836/atlas?style=flat-square&v=1" alt="Go Report"></a>
       <a href="https://codecov.io/gh/mrz1836/atlas"><img src="https://codecov.io/gh/mrz1836/atlas/branch/master/graph/badge.svg?style=flat-square" alt="Coverage"></a>
    </td>
  </tr>

  <tr>
    <td align="right">
       <code>Security</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://scorecard.dev/viewer/?uri=github.com/mrz1836/atlas"><img src="https://api.scorecard.dev/projects/github.com/mrz1836/atlas/badge?style=flat-square" alt="Scorecard"></a>
       <a href=".github/SECURITY.md"><img src="https://img.shields.io/badge/policy-active-success?style=flat-square&logo=security&logoColor=white" alt="Security"></a>
    </td>
    <td align="right">
       &nbsp;&nbsp;&nbsp;&nbsp; <code>Community</code> &nbsp;&nbsp;
    </td>
    <td align="left">
       <a href="https://github.com/mrz1836/atlas/graphs/contributors"><img src="https://img.shields.io/github/contributors/mrz1836/atlas?style=flat-square&color=orange" alt="Contributors"></a>
       <a href="https://mrz1818.com/"><img src="https://img.shields.io/badge/donate-bitcoin-ff9900?style=flat-square&logo=bitcoin" alt="Bitcoin"></a>
    </td>
  </tr>
</table>

<br/>
<br/>

### <code>Project Navigation</code>

<table align="center">
  <tr>
    <td align="center" width="33%">
       🚀&nbsp;<a href="#-installation"><code>Installation</code></a>
    </td>
    <td align="center" width="33%">
       ⚡&nbsp;<a href="#-quick-start"><code>Quick&nbsp;Start</code></a>
    </td>
    <td align="center" width="33%">
       📚&nbsp;<a href="#-features"><code>Features</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
       📖&nbsp;<a href="#-documentation"><code>Documentation</code></a>
    </td>
    <td align="center">
       🔐&nbsp;<a href="#-security"><code>Security</code></a>
    </td>
    <td align="center">
      🛠️&nbsp;<a href="#-code-standards"><code>Code&nbsp;Standards</code></a>
    </td>
  </tr>
  <tr>
    <td align="center">
      🤖&nbsp;<a href="#-ai-usage--assistant-guidelines"><code>AI&nbsp;Guidelines</code></a>
    </td>
    <td align="center">
       👥&nbsp;<a href="#-maintainers"><code>Maintainers</code></a>
    </td>
    <td align="center">
       🤝&nbsp;<a href="#-contributing"><code>Contributing</code></a>
    </td>
  </tr>
  <tr>
    <td align="center" colspan="3">
       📝&nbsp;<a href="#-license"><code>License</code></a>
    </td>
  </tr>
</table>

<br/>

</div>


<br/>

## 🚀 Installation

**ATLAS** requires a [supported release of Go](https://golang.org/doc/devel/release.html#policy).

### Install via go install

```bash
go install github.com/mrz1836/atlas@latest
```

### Build from source

```bash
git clone https://github.com/mrz1836/atlas.git
cd atlas
go build -o bin/atlas .
```

<br/>

## ⚡ Quick Start

ATLAS uses a workspace-based workflow. Each task runs in an isolated Git worktree with its own checkpoint-driven lifecycle.

<br>

### Start a new task

```bash
atlas start "Fix race condition in cache handler" --type bug
```

ATLAS creates a worktree, analyzes the codebase, generates an implementation plan, and waits for your approval.

<br>

### Review and approve the plan

```bash
atlas status
```

Shows all active workspaces and their current states. When a workspace is waiting for approval:

```bash
atlas approve ws-fix-race-condition-20260214-1447
```

ATLAS executes the plan, runs linters and tests, fixes issues automatically, and submits for final review when ready.

<br>

### Work on multiple tasks in parallel

```bash
atlas start "Add metrics endpoint" --type feature
atlas start "Update README examples" --type task
atlas status
```

Each workspace operates independently. You can approve, review, or abandon them in any order.

<br>

### Review and merge

```bash
atlas review ws-fix-race-condition-20260214-1447
```

Opens the workspace for inspection. When satisfied:

```bash
atlas merge ws-fix-race-condition-20260214-1447
```

ATLAS merges the changes back to your main branch and cleans up the worktree.

<br/>

> 📖 **For complete command reference and workflow details, see the [Quick Start Guide →](docs/internal/quick-start.md)**

<br/>

## 📚 Features

### AI-Assisted Workflows

- **Spec-Driven Development (SDD):** Every task starts with analysis and planning
- **Multi-Model Support:** Claude Opus, Sonnet, Haiku, Gemini, GPT-4
- **Context-Aware:** ATLAS analyzes your codebase structure before making changes
- **Iterative Refinement:** Auto-fixes lint and test failures until clean

### Parallel Workspaces

- **Isolated Worktrees:** Each task runs in its own Git worktree
- **Independent Lifecycles:** Start, approve, review, and merge tasks in any order
- **No Branch Pollution:** Clean separation between your main branch and WIP tasks

### Checkpoint Approval System

- **Human-in-the-Loop:** ATLAS stops at critical decision points
- **Plan Review:** Approve implementation plans before execution
- **Final Review:** Inspect completed work before merging
- **Full Transparency:** Every step is logged and reviewable

### Quality Automation

- **Go Validation Protocol:** Automatic pre-commit hooks, formatting, linting, testing
- **Race Detection:** Runs tests with `-race` flag to catch concurrency issues
- **CI Integration:** Validates changes against your existing CI pipeline
- **Coverage Tracking:** Monitors test coverage across changes

### Developer Experience

- **Minimal Configuration:** Works out-of-the-box for Go projects
- **Status Dashboard:** Real-time view of all active workspaces
- **Clean Git History:** Commits are well-formed and descriptive
- **Resume Support:** Pick up interrupted work exactly where you left off

<br/>

## 📖 Documentation

View the comprehensive documentation for ATLAS:

| Document | Description |
|----------|-------------|
| **[vision.md](docs/external/vision.md)** | Full project vision and design philosophy |
| **[templates.md](docs/external/templates.md)** | Template system reference and step types |
| **[quick-start.md](docs/internal/quick-start.md)** | Complete CLI reference (internal) |

<br>

> **Heads up!** ATLAS is experimental software under active development. The MVP focuses on Go projects with a spec-driven workflow. Support for other languages and frameworks is planned.

<br/>

## 🔐 Security

### Important Disclaimer

> ⚠️ **Experimental Software — Use with Caution**
>
> ATLAS is experimental, open-source software provided "AS-IS" without warranty. By using ATLAS, you acknowledge:
>
> - **AI-Generated Code:** All code changes are generated by AI models and should be reviewed carefully
> - **Git Worktrees:** ATLAS creates and manages Git worktrees automatically—ensure you understand worktree behavior
> - **No Formal Audit:** This software has not undergone professional security auditing
> - **Execution Risk:** ATLAS runs commands (linters, tests, formatters) in your repository
> - **API Keys Required:** You must provide your own API keys for AI model access
> - **No Liability:** Authors accept no responsibility for data loss, corrupted repositories, or other damages
>
> **Always review AI-generated code before merging. Always commit your work before running ATLAS.**

For security issues, see our [Security Policy](.github/SECURITY.md) or contact: [atlas@mrz1818.com](mailto:atlas@mrz1818.com)

<br/>

### Additional Documentation & Repository Management

<details>
<summary><strong><code>Development Setup (Getting Started)</code></strong></summary>
<br/>

Install [MAGE-X](https://github.com/mrz1836/go-mage) build tool for development:

```bash
# Install MAGE-X for development and building
go install github.com/magefile/mage@latest
go install github.com/mrz1836/go-mage/magex@latest
magex update:install
```
</details>

<details>
<summary><strong><code>Build Commands</code></strong></summary>
<br/>

View all build commands

```bash script
magex help
```

Common commands:
- `magex build` — Build the binary
- `magex test` — Run test suite
- `magex lint` — Run all linters
- `magex deps:update` — Update dependencies

</details>

<details>
<summary><strong><code>GitHub Workflows</code></strong></summary>
<br/>

ATLAS uses the **Fortress** workflow system for comprehensive CI/CD:

- **fortress-test-suite.yml** — Complete test suite across multiple Go versions
- **fortress-code-quality.yml** — Code quality checks (gofmt, golangci-lint, staticcheck)
- **fortress-security-scans.yml** — Security vulnerability scanning
- **fortress-coverage.yml** — Code coverage reporting to Codecov
- **fortress-release.yml** — Automated binary releases via GoReleaser

See all workflows in [`.github/workflows/`](.github/workflows/).

</details>

<details>
<summary><strong><code>Updating Dependencies</code></strong></summary>
<br/>

To update all dependencies (Go modules, linters, and related tools), run:

```bash
magex deps:update
```

This command ensures all dependencies are brought up to date in a single step, including Go modules and any managed tools. It is the recommended way to keep your development environment and CI in sync with the latest versions.

</details>

<br/>

## 🛠️ Code Standards
Read more about this Go project's [code standards](.github/CODE_STANDARDS.md).

<br/>

## 🤖 AI Usage & Assistant Guidelines
Read the [AI Usage & Assistant Guidelines](.github/CLAUDE.md) for details on how AI is used in this project and how to interact with AI assistants.

<br/>

## 👥 Maintainers
| [<img src="https://github.com/mrz1836.png" height="50" alt="MrZ" />](https://github.com/mrz1836) |
|:------------------------------------------------------------------------------------------------:|
|                                [MrZ](https://github.com/mrz1836)                                 |

<br/>

## 🤝 Contributing
View the [contributing guidelines](.github/CONTRIBUTING.md) and please follow the [code of conduct](.github/CODE_OF_CONDUCT.md).

### How can I help?
All kinds of contributions are welcome :raised_hands:!
The most basic way to show your support is to star :star2: the project, or to raise issues :speech_balloon:.
You can also support this project by [becoming a sponsor on GitHub](https://github.com/sponsors/mrz1836) :clap:
or by making a [**bitcoin donation**](https://mrz1818.com/?tab=tips&utm_source=github&utm_medium=sponsor-link&utm_campaign=atlas&utm_term=atlas&utm_content=atlas) to ensure this journey continues indefinitely! :rocket:


[![Stars](https://img.shields.io/github/stars/mrz1836/atlas?label=Please%20like%20us&style=social)](https://github.com/mrz1836/atlas/stargazers)

<br/>

## 📝 License

[![License](https://img.shields.io/github/license/mrz1836/atlas.svg?style=flat)](LICENSE)
