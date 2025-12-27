# Story 1.1: Initialize Go Module and Project Structure

Status: ready-for-dev

## Story

As a **developer**,
I want **the ATLAS project initialized with the complete directory structure and Go module**,
So that **I have a consistent foundation for implementing all ATLAS subsystems**.

## Acceptance Criteria

1. **Given** a clean repository **When** I run the initialization commands **Then** the `go.mod` file exists with module path `github.com/mrz1836/atlas`

2. **Given** the Go module is initialized **When** I inspect the project **Then** Go version is set to 1.24+

3. **Given** the project is initialized **When** I list directories **Then** all required directories exist:
   - `cmd/atlas/`
   - `internal/cli/`
   - `internal/config/`
   - `internal/task/`
   - `internal/workspace/`
   - `internal/ai/`
   - `internal/git/`
   - `internal/validation/`
   - `internal/template/`
   - `internal/tui/`
   - `internal/constants/`
   - `internal/errors/`
   - `internal/domain/`
   - `internal/testutil/`

4. **Given** all directories exist **When** I check dependencies **Then** core dependencies are added:
   - `github.com/spf13/cobra` (CLI framework)
   - `github.com/spf13/viper` (configuration management)
   - `github.com/rs/zerolog` (structured JSON logging)
   - `github.com/charmbracelet/huh` (interactive forms)
   - `github.com/charmbracelet/lipgloss` (terminal styling)
   - `github.com/charmbracelet/bubbles` (TUI widgets)
   - `github.com/stretchr/testify` (testing assertions)

5. **Given** dependencies are added **When** I check configuration files **Then** `.mage.yaml` is configured for MAGE-X

6. **Given** MAGE-X is configured **When** I check linter config **Then** `.golangci.yml` is configured with project linting rules

7. **Given** all setup is complete **When** I run `go mod tidy` **Then** it completes without errors

## Tasks / Subtasks

- [ ] Task 1: Initialize Go module (AC: #1, #2)
  - [ ] Run `go mod init github.com/mrz1836/atlas`
  - [ ] Verify go.mod specifies Go 1.24+

- [ ] Task 2: Create complete directory structure (AC: #3)
  - [ ] Create `cmd/atlas/` directory
  - [ ] Create `internal/cli/` directory
  - [ ] Create `internal/config/` directory
  - [ ] Create `internal/task/` directory
  - [ ] Create `internal/workspace/` directory
  - [ ] Create `internal/ai/` directory
  - [ ] Create `internal/git/` directory
  - [ ] Create `internal/validation/` directory
  - [ ] Create `internal/template/` directory
  - [ ] Create `internal/tui/` directory
  - [ ] Create `internal/constants/` directory
  - [ ] Create `internal/errors/` directory
  - [ ] Create `internal/domain/` directory
  - [ ] Create `internal/testutil/` directory
  - [ ] Add `.gitkeep` files to empty directories

- [ ] Task 3: Add core dependencies (AC: #4)
  - [ ] `go get github.com/spf13/cobra`
  - [ ] `go get github.com/spf13/viper`
  - [ ] `go get github.com/rs/zerolog`
  - [ ] `go get github.com/charmbracelet/huh`
  - [ ] `go get github.com/charmbracelet/lipgloss`
  - [ ] `go get github.com/charmbracelet/bubbles`
  - [ ] `go get github.com/stretchr/testify`

- [ ] Task 4: Configure MAGE-X (AC: #5)
  - [ ] Run `magex init` or create `.mage.yaml` manually
  - [ ] Configure standard Go targets (format, lint, test)

- [ ] Task 5: Configure golangci-lint (AC: #6)
  - [ ] Create `.golangci.yml` with project-specific rules
  - [ ] Enable relevant linters (revive, gosec, goimports, etc.)
  - [ ] Configure import rules to match architecture

- [ ] Task 6: Create placeholder main.go (AC: #7)
  - [ ] Create `cmd/atlas/main.go` with minimal entry point
  - [ ] Ensure `go build` and `go mod tidy` succeed

- [ ] Task 7: Verify setup (AC: #7)
  - [ ] Run `go mod tidy`
  - [ ] Run `go build ./...`
  - [ ] Verify no errors

## Dev Notes

### Critical Architecture Requirements

**This is the foundation story - get it right!** All future stories depend on this structure being exactly as documented.

#### Project Structure (MUST MATCH EXACTLY)
```
atlas/
├── cmd/
│   └── atlas/
│       └── main.go           # Entry point, context.Background()
├── internal/
│   ├── cli/                  # Command definitions (Cobra) - one file per command
│   ├── config/               # Configuration management (Viper)
│   ├── task/                 # Task engine & state machine
│   ├── workspace/            # Workspace/worktree management
│   ├── ai/                   # AI runner abstraction
│   ├── git/                  # Git operations layer
│   ├── validation/           # Validation executor
│   ├── template/             # Template system
│   ├── tui/                  # Charm TUI components
│   ├── constants/            # ALL shared constants (CRITICAL)
│   ├── errors/               # ALL sentinel errors (CRITICAL)
│   ├── domain/               # Shared types (Task, Workspace, Step)
│   └── testutil/             # Test fixtures and helpers
├── .mage.yaml                # MAGE-X configuration
├── .golangci.yml             # Linter configuration
└── go.mod
```

#### Package Import Rules (ENFORCE FROM DAY ONE)
- `cmd/atlas` → only imports `internal/cli`
- `internal/cli` → imports task, workspace, tui, config
- `internal/task` → imports ai, git, validation, template, domain
- **All packages** → can import constants, errors, config, domain
- **internal/domain** → MUST NOT import any other internal package
- **internal/constants** → MUST NOT import any other package
- **internal/errors** → MUST NOT import any other internal package (only std lib)

### Technology Stack Specifics

#### Go 1.24+ (Released February 2025)
Key features to leverage:
- Generic type aliases fully supported
- Tool dependencies via `tool` directives in go.mod (can use instead of tools.go)
- Swiss Tables-based map implementation (2-3% faster)
- `testing.B.Loop()` for benchmarks (cleaner than `for range b.N`)
- `os.Root` for filesystem isolation
- `runtime.AddCleanup` for better finalization
- `omitzero` option for JSON tags

**go.mod should specify:**
```go
module github.com/mrz1836/atlas

go 1.24
```

#### Cobra (Latest: v1.8.x)
- Use standard command structure: `APPNAME VERB NOUN --FLAG`
- Flag groups for related flags (`MarkFlagsRequiredTogether`)
- Mutually exclusive flags (`MarkFlagsMutuallyExclusive`)
- Shell completion support built-in

#### Charm Libraries (Use stable v1.x for now)
- **lipgloss** v1.1.0 - Terminal styling (v2 is alpha, not recommended yet)
- **bubbles** v0.20.x - TUI widgets
- **huh** v0.6.x or latest stable - Interactive forms
- **bubbletea** v1.x - TUI framework (v2 is alpha)

**DO NOT use v2 alpha versions** - they are not production ready.

#### Viper Configuration
- YAML as primary config format
- Environment variable prefix: `ATLAS_`
- Layered precedence: CLI > env > project > global > defaults

#### Zerolog
- JSON structured logging
- Use `.Str()`, `.Int()`, `.Dur()` for typed fields
- All field names in `snake_case`
- Never log API keys or secrets

### golangci-lint Configuration

Create `.golangci.yml` with these recommended linters:
```yaml
run:
  go: "1.24"
  timeout: 5m

linters:
  enable:
    - revive
    - gosec
    - goimports
    - govet
    - errcheck
    - staticcheck
    - ineffassign
    - unused
    - misspell
    - gofmt
    - unparam

linters-settings:
  goimports:
    local-prefixes: github.com/mrz1836/atlas
  revive:
    rules:
      - name: context-as-argument
        severity: error
      - name: var-naming
        severity: warning

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
```

### MAGE-X Configuration

Create `.mage.yaml`:
```yaml
version: "1"
targets:
  format:
    description: "Format Go code"
    commands:
      - gofmt -w -s .
      - goimports -w -local github.com/mrz1836/atlas .

  lint:
    description: "Run linters"
    commands:
      - golangci-lint run ./...

  test:
    description: "Run tests"
    commands:
      - go test -race -cover ./...

  build:
    description: "Build binary"
    commands:
      - go build -o bin/atlas ./cmd/atlas
```

### Minimal main.go Template

```go
// cmd/atlas/main.go
package main

import (
	"context"
	"os"

	"github.com/mrz1836/atlas/internal/cli"
)

func main() {
	ctx := context.Background()
	if err := cli.Execute(ctx); err != nil {
		os.Exit(1)
	}
}
```

### Minimal root.go Template

```go
// internal/cli/root.go
package cli

import (
	"context"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "atlas",
	Short: "ATLAS - AI Task Lifecycle Automation System",
	Long: `ATLAS automates the software development lifecycle with AI-powered task execution,
validation, and delivery through an intuitive CLI interface.`,
}

// Execute runs the root command
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
```

### Project Structure Notes

- **Alignment with unified project structure:** Follows Go 1.24+ conventions with `cmd/` for entry points and `internal/` for private packages
- **Architecture compliance:** Structure matches architecture.md exactly
- **Future extensibility:** Template and steps packages ready for Epic 4

### References

- [Source: _bmad-output/planning-artifacts/architecture.md#Project Structure]
- [Source: _bmad-output/planning-artifacts/architecture.md#Complete Project Directory Structure]
- [Source: _bmad-output/planning-artifacts/architecture.md#Package Import Rules]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.1]
- [Source: _bmad-output/project-context.md#Technology Stack & Versions]
- [Source: _bmad-output/project-context.md#Go Language Rules]

### Latest Version Information (Web Research)

**Go 1.24 (February 2025):**
- Swiss Tables map implementation for 2-3% performance improvement
- Generic type aliases fully supported
- `tool` directives in go.mod for executable dependencies
- `testing.B.Loop()` for cleaner benchmarks
- Last version supporting macOS 11 Big Sur

**Cobra (v1.8.x, December 2025):**
- Plugin support for kubectl-like tools
- Custom ShellCompDirective for subcommands
- golangci-lint v2 upgrade

**Charm Libraries (Stable Versions - March 2025):**
- Lipgloss v1.1.0 (stable) - Use this, NOT v2 alpha
- Bubbles v0.20.x (stable)
- Huh v0.6.x (stable)
- Bubble Tea v1.x (stable)

**⚠️ WARNING:** Charm v2 libraries are in alpha. Use stable v1.x versions for production code.

## Dev Agent Record

### Agent Model Used

{{agent_model_name_version}}

### Debug Log References

### Completion Notes List

### File List

