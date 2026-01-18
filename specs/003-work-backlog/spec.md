# Feature Specification: Work Backlog for Discovered Issues

**Feature Branch**: `003-work-backlog`
**Created**: 2026-01-18
**Status**: Draft
**Input**: User description: "Implement the Work Backlog feature for ATLAS to capture discovered issues during AI-assisted development"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - AI Discovers and Records Issue (Priority: P1)

During AI-assisted development, the AI agent notices an issue outside the current task scope (e.g., missing error handling, potential race condition, insufficient test coverage). The AI immediately records this discovery to the project backlog without interrupting its current work.

**Why this priority**: This is the core value proposition - ensuring no good observation gets lost during AI-assisted development. Without frictionless capture, the entire feature fails its purpose.

**Independent Test**: Can be fully tested by running an AI task and verifying that discovered issues are captured as individual YAML files in `.atlas/backlog/` with correct metadata.

**Acceptance Scenarios**:

1. **Given** an active AI task in workspace, **When** the AI agent discovers an issue outside its current scope, **Then** the AI can run a command to record the discovery with title, description, category, severity, and location
2. **Given** a discovery being recorded, **When** the command executes, **Then** a unique YAML file is created in `.atlas/backlog/` containing all discovery metadata including git context (branch, commit SHA)
3. **Given** multiple AI agents working in parallel worktrees, **When** both agents record discoveries simultaneously, **Then** each creates its own distinct file with no conflicts or data loss

---

### User Story 2 - Human Reviews Backlog (Priority: P2)

A developer wants to review all pending discoveries in the project backlog to decide which issues to address. They can list, filter, and view the details of each discovery.

**Why this priority**: Without the ability to review discoveries, captured issues remain invisible and unused. This enables human oversight and decision-making.

**Independent Test**: Can be fully tested by creating sample discovery files and verifying that the list command displays them correctly with appropriate filtering options.

**Acceptance Scenarios**:

1. **Given** discoveries exist in the backlog, **When** a user runs the list command, **Then** all pending discoveries are displayed with title, category, severity, and age
2. **Given** discoveries with various statuses and categories, **When** a user filters by status or category, **Then** only matching discoveries are returned
3. **Given** a specific discovery ID, **When** a user requests details, **Then** the full discovery content including location, description, and git context is displayed

---

### User Story 3 - Human Adds Discovery Interactively (Priority: P3)

A developer manually discovers an issue while reviewing code and wants to add it to the backlog. They can use an interactive form that guides them through providing the necessary information.

**Why this priority**: While AI capture is primary, humans also discover issues. Interactive forms reduce friction for human users who don't want to memorize command flags.

**Independent Test**: Can be fully tested by running the interactive add command and verifying the form flow captures all required fields and creates a valid discovery file.

**Acceptance Scenarios**:

1. **Given** a user runs the add command without arguments, **When** the interactive form appears, **Then** the form collects title, description (optional), category, and severity through guided prompts
2. **Given** a user completes the interactive form, **When** they submit, **Then** a discovery is created with auto-captured git context and a unique ID
3. **Given** a user is in a git repository, **When** they add a discovery, **Then** the current branch and commit SHA are automatically recorded

---

### User Story 4 - Promote Discovery to Task (Priority: P4)

A developer decides a discovered issue should be addressed. They promote the discovery to an ATLAS task, which marks the discovery as promoted and optionally starts a new task workflow.

**Why this priority**: Completing the lifecycle from discovery to action is important but depends on the existing task system integration.

**Independent Test**: Can be fully tested by promoting a discovery and verifying its status changes to "promoted" and the linked task ID is recorded.

**Acceptance Scenarios**:

1. **Given** a pending discovery, **When** a user promotes it, **Then** the discovery status changes to "promoted" and a task ID is recorded in the lifecycle metadata
2. **Given** a promoted discovery, **When** a user views it, **Then** the linked task information is displayed

---

### User Story 5 - Dismiss Discovery (Priority: P5)

A developer reviews a discovery and determines it's not worth addressing (false positive, won't fix, duplicate). They dismiss it with a reason for future reference.

**Why this priority**: Dismissal is a housekeeping feature that keeps the backlog manageable.

**Independent Test**: Can be fully tested by dismissing a discovery with a reason and verifying status and reason are recorded.

**Acceptance Scenarios**:

1. **Given** a pending discovery, **When** a user dismisses it with a reason, **Then** the discovery status changes to "dismissed" and the reason is stored
2. **Given** a dismissed discovery, **When** viewing the backlog, **Then** dismissed items are hidden by default but can be shown with a filter flag

---

### Edge Cases

- What happens when the `.atlas/backlog/` directory doesn't exist? (Auto-create with `.gitkeep`)
- How does the system handle duplicate IDs? (Use UUID-based IDs prefixed with "disc-" to prevent collisions)
- What happens when git context cannot be determined? (Record empty strings, issue a warning)
- How does the system handle concurrent writes from multiple processes? (Each discovery is a separate file - atomic writes prevent conflicts)
- What happens when a discovery file is malformed YAML? (Skip with warning during list, don't corrupt other entries)

## Clarifications

### Session 2026-01-18

- Q: What are the allowed severity levels for discoveries? → A: `low`, `medium`, `high`, `critical` (4-level standard)
- Q: What are the allowed category values for discoveries? → A: `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation`
- Q: What values should the "discoverer" field contain? → A: `ai:<agent>:<model>` or `human:<github-username>` (typed with identifier)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST create individual YAML files in `.atlas/backlog/` directory for each discovery
- **FR-002**: System MUST generate unique discovery IDs using the pattern `disc-<short-hash>` (6 alphanumeric characters)
- **FR-003**: System MUST capture git context (branch name, commit SHA) automatically when creating a discovery
- **FR-004**: System MUST support both interactive form input and flag-based input for adding discoveries
- **FR-005**: System MUST provide a list command that scans the backlog directory and displays discoveries
- **FR-006**: System MUST support filtering discoveries by status (pending, promoted, dismissed) and category
- **FR-007**: System MUST support status transitions: pending → promoted, pending → dismissed
- **FR-008**: System MUST store discovery metadata in YAML format following the schema version "1.0"
- **FR-009**: System MUST auto-create the `.atlas/backlog/` directory with a `.gitkeep` file if it doesn't exist
- **FR-010**: System MUST record timestamps in ISO 8601 format (UTC)
- **FR-011**: System MUST allow specifying file location (path and optional line number) for each discovery
- **FR-012**: System MUST support JSON output format for programmatic access (AI agents, scripts)
- **FR-013**: System MUST handle concurrent writes safely (one file per discovery ensures atomic operations)
- **FR-014**: System MUST preserve all discovery metadata through status transitions
- **FR-015**: System MUST validate required fields (title, category, severity) before creating a discovery
- **FR-016**: System MUST auto-detect AI identity from environment variables where available
- **FR-017**: System MUST prevent data loss from ID collisions via exclusive file creation

### Key Entities

- **Discovery**: A captured issue or observation found during development. Contains title, description, status, content (category, severity, tags), location (file, line), context (timestamp, discoverer, task ID, git info), and lifecycle (promoted task ID, dismissal reason). Severity levels: `low`, `medium`, `high`, `critical`. Categories: `bug`, `security`, `performance`, `maintainability`, `testing`, `documentation`. Discoverer format: `ai:<agent>:<model>` (e.g., `ai:claude-code:claude-sonnet-4`) or `human:<github-username>` (e.g., `human:mrz`).
- **Backlog**: The collection of all discoveries stored as individual YAML files in `.atlas/backlog/` directory. Provides a project-local queue that travels with the codebase.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Discoveries can be recorded in under 5 seconds from command invocation to file creation confirmation
- **SC-002**: Zero merge conflicts occur when multiple developers or worktrees add discoveries simultaneously (verified by concurrent write tests)
- **SC-003**: Interactive form users complete discovery entry with fewer keystrokes than equivalent flag-based commands
- **SC-004**: Git context (branch and commit SHA) is stored with every discovery entry
- **SC-005**: All discoveries round-trip correctly (save → load → verify) with 100% field accuracy including timestamps
- **SC-006**: List command efficiently handles 1000+ discovery files without noticeable delay (under 2 seconds)
- **SC-007**: Filter operations return correct results across all supported criteria (status, category)
- **SC-008**: View command provides "Premium" experience with Markdown rendering and color-coded elements (glamour)

## Assumptions

- The ATLAS CLI infrastructure and command structure already exist (this feature adds new subcommands)
- The Bubble Tea framework (charmbracelet/huh) is available for interactive forms (consistent with existing ATLAS patterns)
- YAML is the preferred configuration format (consistent with `.atlas/config.yaml`)
- Discovery IDs use a short hash (6 chars) which provides sufficient uniqueness for typical project scales
- The AI agent protocol instructions (CLAUDE.md) will be updated separately to encourage backlog usage
- Integration with ATLAS task system for promotion is out of scope for MVP (status recorded, actual task creation handled manually or in future iteration)
