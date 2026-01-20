package prompts

// PromptID identifies a specific prompt template.
type PromptID string

// Prompt identifiers for all AI prompts in ATLAS.
const (
	// Git-related prompts
	CommitMessage PromptID = "git/commit_message"
	PRDescription PromptID = "git/pr_description"

	// Validation-related prompts
	ValidationRetry PromptID = "validation/retry"

	// Backlog/discovery-related prompts
	DiscoveryAnalysis PromptID = "backlog/discovery_analysis"

	// Verification-related prompts
	QuickVerify     PromptID = "verify/quick_check"
	CodeCorrectness PromptID = "verify/code_correctness"
	AutoFix         PromptID = "verify/auto_fix"

	// Task-related prompts
	CIFailure PromptID = "task/ci_failure"
)

// FileChange represents a changed file for commit/PR prompts.
type FileChange struct {
	Path   string
	Status string
}

// CommitMessageData contains input data for commit message generation.
type CommitMessageData struct {
	// Package is the Go package or directory being modified.
	Package string
	// Files are the files being changed in this commit.
	Files []FileChange
	// DiffSummary is a summary of the actual changes (optional).
	DiffSummary string
	// Scope is the conventional commit scope (derived from package).
	Scope string
}

// PRDescriptionData contains input data for PR description generation.
type PRDescriptionData struct {
	// TaskDescription describes what the task is trying to accomplish.
	TaskDescription string
	// CommitMessages are the commit messages for the changes.
	CommitMessages []string
	// FilesChanged lists the files that were modified.
	FilesChanged []PRFileChange
	// DiffSummary is a summary of the changes (optional).
	DiffSummary string
	// ValidationResults are the results of validation commands.
	ValidationResults string
	// TemplateName is the task template name (bugfix, feature, etc.).
	TemplateName string
	// TaskID is the task identifier.
	TaskID string
	// WorkspaceName is the workspace name.
	WorkspaceName string
}

// PRFileChange represents a changed file for PR descriptions.
type PRFileChange struct {
	// Path is the file path relative to repository root.
	Path string
	// Insertions is the number of lines added.
	Insertions int
	// Deletions is the number of lines removed.
	Deletions int
}

// ValidationRetryData contains input data for validation retry prompts.
type ValidationRetryData struct {
	// FailedStep is which step failed (format, lint, test, pre-commit).
	FailedStep string
	// FailedCommands lists the commands that failed.
	FailedCommands []string
	// ErrorOutput is the combined error output (truncated if needed).
	ErrorOutput string
	// AttemptNumber is the current retry attempt (1-indexed).
	AttemptNumber int
	// MaxAttempts is the maximum allowed attempts.
	MaxAttempts int
}

// DiscoveryAnalysisData contains input data for discovery/backlog analysis.
type DiscoveryAnalysisData struct {
	// Title is the discovery title.
	Title string
	// Category is the discovery category (bug, enhancement, etc.).
	Category string
	// Severity is the discovery severity (critical, high, medium, low).
	Severity string
	// Description is the detailed description.
	Description string
	// File is the relevant file path (optional).
	File string
	// Line is the relevant line number (optional).
	Line int
	// Tags are associated tags.
	Tags []string
	// GitBranch is the branch where discovery was found.
	GitBranch string
	// GitCommit is the commit where discovery was found.
	GitCommit string
	// AvailableAgents lists the agents that are detected/installed.
	AvailableAgents []string
	// AvailableTemplates lists valid template names.
	AvailableTemplates []string
}

// AgentModelInfo provides information about an agent's available models.
type AgentModelInfo struct {
	// Name is the agent name (claude, gemini, codex).
	Name string
	// Models are the available model aliases.
	Models []string
	// DefaultModel is the default model for this agent.
	DefaultModel string
}

// QuickVerifyData contains input data for quick verification prompts.
type QuickVerifyData struct {
	// TaskDescription is the task that was implemented.
	TaskDescription string
	// Checks are the verification checks to perform.
	Checks []string
}

// CodeCorrectnessData contains input data for code correctness checking.
type CodeCorrectnessData struct {
	// TaskDescription is the task that was implemented.
	TaskDescription string
	// ChangedFiles are the files that were modified.
	ChangedFiles []ChangedFileInfo
}

// ChangedFileInfo represents a file that was modified during implementation.
type ChangedFileInfo struct {
	// Path is the file path relative to repo root.
	Path string
	// Language is the programming language (inferred from extension).
	Language string
	// Content is the file content (may be diff or full content).
	Content string
}

// CIFailureData contains input data for CI failure analysis prompts.
type CIFailureData struct {
	// FailedChecks are the CI checks that failed.
	FailedChecks []CICheckInfo
	// HasFailures indicates if specific failures were identified.
	HasFailures bool
}

// CICheckInfo represents information about a CI check.
type CICheckInfo struct {
	// Name is the check name.
	Name string
	// Status is the check status (fail, cancel, etc.).
	Status string
	// Workflow is the workflow name (optional).
	Workflow string
	// URL is the link to the check logs (optional).
	URL string
}

// AutoFixData contains input data for verification auto-fix prompts.
type AutoFixData struct {
	// TaskDesc is the task description.
	TaskDesc string
	// TotalIssues is the total number of issues found.
	TotalIssues int
	// ErrorIssues are issues with error severity.
	ErrorIssues []AutoFixIssue
	// WarningIssues are issues with warning severity.
	WarningIssues []AutoFixIssue
	// InfoIssues are issues with info severity.
	InfoIssues []AutoFixIssue
}

// AutoFixIssue represents a single issue to be fixed.
type AutoFixIssue struct {
	// File is the file path where the issue was found.
	File string
	// Line is the line number.
	Line int
	// Message is the issue description.
	Message string
	// Suggestion is an optional fix suggestion.
	Suggestion string
}
