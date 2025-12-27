// Package constants provides centralized constant values used throughout ATLAS.
// This file contains tool-related constants for the tool detection system.
package constants

import "time"

// Tool detection timeout configuration.
const (
	// ToolDetectionTimeout is the maximum duration for detecting all tools.
	// Detection runs in parallel but must complete within this timeout.
	ToolDetectionTimeout = 2 * time.Second
)

// Tool names used by the tool detection system.
const (
	// ToolGo is the Go programming language runtime.
	ToolGo = "go"

	// ToolGit is the Git version control system.
	ToolGit = "git"

	// ToolGH is the GitHub CLI tool.
	ToolGH = "gh"

	// ToolUV is the uv Python package manager.
	ToolUV = "uv"

	// ToolClaude is the Claude Code CLI.
	ToolClaude = "claude"

	// ToolMageX is the mage-x build automation tool.
	ToolMageX = "magex"

	// ToolGoPreCommit is the go-pre-commit Git hooks tool.
	ToolGoPreCommit = "go-pre-commit"

	// ToolSpeckit is the Speckit specification tool.
	ToolSpeckit = "specify"
)

// Minimum version requirements for required tools.
const (
	// MinVersionGo is the minimum required Go version.
	MinVersionGo = "1.24.0"

	// MinVersionGit is the minimum required Git version.
	MinVersionGit = "2.20.0"

	// MinVersionGH is the minimum required GitHub CLI version.
	MinVersionGH = "2.20.0"

	// MinVersionUV is the minimum required uv version.
	MinVersionUV = "0.5.0"

	// MinVersionClaude is the minimum required Claude Code version.
	MinVersionClaude = "2.0.76"
)

// Tool version command arguments.
const (
	// VersionFlagGo is the version argument for Go.
	VersionFlagGo = "version"

	// VersionFlagStandard is the standard version flag used by most tools.
	VersionFlagStandard = "--version"
)
