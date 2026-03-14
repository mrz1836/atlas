package constants

// Log file names and patterns.
const (
	// AILogFileName is the name of the log file that captures AI agent output.
	AILogFileName = "ai.log"

	// ValidationLogFileName is the name of the log file that captures validation output.
	ValidationLogFileName = "validation.log"

	// TaskLogFileName is the name of the log file that captures general task execution output.
	TaskLogFileName = "task.log"

	// CLILogFileName is the name of the global CLI log file for host operations.
	// This file is located in ~/.atlas/logs/atlas.log
	CLILogFileName = "atlas.log"
)

// Configuration file names.
const (
	// GlobalConfigName is the name of the global ATLAS configuration file.
	// This file is located in the ATLAS home directory.
	GlobalConfigName = "config.yaml"

	// ProjectConfigName is the name of the project-specific ATLAS configuration file.
	// This file is located in the project root directory.
	ProjectConfigName = ".atlas.yaml"
)

// Branch prefix patterns used for Git branch naming.
const (
	// BranchPrefixFix is the prefix for bug fix branches.
	BranchPrefixFix = "fix/"

	// BranchPrefixFeat is the prefix for feature branches.
	BranchPrefixFeat = "feat/"

	// BranchPrefixChore is the prefix for maintenance/chore branches.
	BranchPrefixChore = "chore/"

	// BranchPrefixRefactor is the prefix for refactoring branches.
	BranchPrefixRefactor = "refactor/"

	// BranchPrefixDocs is the prefix for documentation branches.
	BranchPrefixDocs = "docs/"

	// BranchPrefixTest is the prefix for test-related branches.
	BranchPrefixTest = "test/"
)
