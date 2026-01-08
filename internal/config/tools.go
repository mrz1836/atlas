// Package config provides configuration management for ATLAS.
// This file implements the tool detection system for checking external tool availability.
package config

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/mrz1836/atlas/internal/constants"
)

// Pre-compiled regexes for version parsing (compiled once at package init).
//
//nolint:gochecknoglobals // Package-level compiled regexes are a Go best practice for performance
var (
	goVersionRe      = regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)
	gitVersionRe     = regexp.MustCompile(`git version (\d+\.\d+(?:\.\d+)?)`)
	ghVersionRe      = regexp.MustCompile(`gh version (\d+\.\d+(?:\.\d+)?)`)
	uvVersionRe      = regexp.MustCompile(`uv (\d+\.\d+(?:\.\d+)?)`)
	mageVersionRe    = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)?)`)
	genericVersionRe = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)?)`)

	// Claude version patterns (from most specific to most general)
	claudeVersionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)claude[- ]?code[- ]?v?(\d+\.\d+(?:\.\d+)?)`),
		regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	}

	// Gemini version patterns (from most specific to most general)
	geminiVersionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)gemini[- ]?(?:cli)?[- ]?v?(\d+\.\d+(?:\.\d+)?)`),
		regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	}

	// Codex version patterns (from most specific to most general)
	codexVersionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)codex[- ]?(?:cli)?[- ]?v?(\d+\.\d+(?:\.\d+)?)`),
		regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	}
)

// ToolStatus represents the installation status of an external tool.
//
//nolint:recvcheck // UnmarshalJSON requires pointer receiver per json.Unmarshaler interface
type ToolStatus int

const (
	// ToolStatusMissing indicates the tool is not installed.
	ToolStatusMissing ToolStatus = iota

	// ToolStatusInstalled indicates the tool is installed and meets version requirements.
	ToolStatusInstalled

	// ToolStatusOutdated indicates the tool is installed but below the minimum version.
	ToolStatusOutdated
)

// maxVersionSegments is the number of segments in a semantic version (major.minor.patch).
const maxVersionSegments = 3

// String returns a human-readable representation of the tool status.
// Uses value receiver since ToolStatus is an immutable int type.
func (s ToolStatus) String() string {
	switch s {
	case ToolStatusInstalled:
		return "installed"
	case ToolStatusMissing:
		return "missing"
	case ToolStatusOutdated:
		return "outdated"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for human-readable JSON output.
// Uses value receiver for consistency with String() and because marshaling doesn't mutate.
func (s ToolStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler for parsing JSON status strings.
// Uses pointer receiver as required by json.Unmarshaler interface to modify the value.
func (s *ToolStatus) UnmarshalJSON(data []byte) error {
	str := string(data)
	// Remove quotes
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	switch str {
	case "installed":
		*s = ToolStatusInstalled
	case "missing":
		*s = ToolStatusMissing
	case "outdated":
		*s = ToolStatusOutdated
	default:
		*s = ToolStatusMissing
	}
	return nil
}

// Tool represents an external tool that ATLAS depends on.
type Tool struct {
	// Name is the tool identifier (e.g., "go", "git").
	Name string `json:"name"`

	// Required indicates if the tool is mandatory for ATLAS to function.
	Required bool `json:"required"`

	// Managed indicates if the tool is installed/managed by ATLAS.
	Managed bool `json:"managed"`

	// MinVersion is the minimum required version (semver format).
	MinVersion string `json:"min_version"`

	// CurrentVersion is the detected installed version.
	CurrentVersion string `json:"current_version"`

	// Status is the current installation status.
	Status ToolStatus `json:"status"`

	// InstallHint provides installation instructions for missing tools.
	InstallHint string `json:"install_hint"`
}

// ToolDetectionResult holds the results of detecting all tools.
type ToolDetectionResult struct {
	// Tools contains the detection result for each tool.
	Tools []Tool `json:"tools"`

	// HasMissingRequired indicates if any required tools are missing or outdated.
	HasMissingRequired bool `json:"has_missing_required"`
}

// MissingRequiredTools returns a list of required tools that are missing or outdated.
func (r *ToolDetectionResult) MissingRequiredTools() []Tool {
	var missing []Tool
	for _, tool := range r.Tools {
		if tool.Required && (tool.Status == ToolStatusMissing || tool.Status == ToolStatusOutdated) {
			missing = append(missing, tool)
		}
	}
	return missing
}

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	// LookPath searches for an executable named file in the PATH.
	LookPath(file string) (string, error)

	// Run executes a command and returns its combined output.
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// DefaultCommandExecutor implements CommandExecutor using os/exec.
type DefaultCommandExecutor struct{}

// LookPath searches for an executable in the PATH.
func (e *DefaultCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Run executes a command and returns its output.
func (e *DefaultCommandExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// Ensure output is captured and not printed to terminal
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// ToolDetector detects the installation status of external tools.
type ToolDetector interface {
	// Detect checks all configured tools and returns their status.
	Detect(ctx context.Context) (*ToolDetectionResult, error)
}

// DefaultToolDetector implements ToolDetector.
type DefaultToolDetector struct {
	executor CommandExecutor
}

// NewToolDetector creates a new DefaultToolDetector with the default executor.
func NewToolDetector() *DefaultToolDetector {
	return &DefaultToolDetector{
		executor: &DefaultCommandExecutor{},
	}
}

// NewToolDetectorWithExecutor creates a new DefaultToolDetector with a custom executor.
func NewToolDetectorWithExecutor(executor CommandExecutor) *DefaultToolDetector {
	return &DefaultToolDetector{
		executor: executor,
	}
}

// toolConfig holds the configuration for detecting a specific tool.
type toolConfig struct {
	name        string
	command     string
	versionFlag string
	minVersion  string
	required    bool
	managed     bool
	installHint string
	parseFunc   func(output string) string
}

// getToolConfigs returns the configuration for all tools to detect.
func getToolConfigs() []toolConfig {
	return []toolConfig{
		{
			name:        constants.ToolGo,
			command:     constants.ToolGo,
			versionFlag: constants.VersionFlagGo,
			minVersion:  constants.MinVersionGo,
			required:    true,
			managed:     false,
			installHint: "Install Go from https://go.dev/dl/ (version 1.24+)",
			parseFunc:   parseGoVersion,
		},
		{
			name:        constants.ToolGit,
			command:     constants.ToolGit,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionGit,
			required:    true,
			managed:     false,
			installHint: "Install Git from https://git-scm.com/downloads (version 2.20+)",
			parseFunc:   parseGitVersion,
		},
		{
			name:        constants.ToolGH,
			command:     constants.ToolGH,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionGH,
			required:    true,
			managed:     false,
			installHint: "Install GitHub CLI: brew install gh (version 2.20+)",
			parseFunc:   parseGHVersion,
		},
		{
			name:        constants.ToolUV,
			command:     constants.ToolUV,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionUV,
			required:    true,
			managed:     false,
			installHint: "Install uv: curl -LsSf https://astral.sh/uv/install.sh | sh",
			parseFunc:   parseUVVersion,
		},
		{
			name:        constants.ToolClaude,
			command:     constants.ToolClaude,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionClaude,
			required:    false, // Lazy validation - checked when agent is used
			managed:     false,
			installHint: "Install Claude CLI: npm install -g @anthropic-ai/claude-code",
			parseFunc:   parseClaudeVersion,
		},
		{
			name:        constants.ToolGemini,
			command:     constants.ToolGemini,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionGemini,
			required:    false, // Lazy validation - checked when agent is used
			managed:     false,
			installHint: "Install Gemini CLI: npm install -g @google/gemini-cli",
			parseFunc:   parseGeminiVersion,
		},
		{
			name:        constants.ToolCodex,
			command:     constants.ToolCodex,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  constants.MinVersionCodex,
			required:    false, // Lazy validation - checked when agent is used
			managed:     false,
			installHint: "Install Codex CLI: npm install -g @openai/codex",
			parseFunc:   parseCodexVersion,
		},
		{
			name:        constants.ToolMageX,
			command:     constants.ToolMageX,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  "",
			required:    false,
			managed:     true,
			installHint: "Install with: go install github.com/mage-x/magex@latest",
			parseFunc:   parseMageXVersion,
		},
		{
			name:        constants.ToolGoPreCommit,
			command:     constants.ToolGoPreCommit,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  "",
			required:    false,
			managed:     true,
			installHint: "Install with: go install github.com/mrz1836/go-pre-commit@latest",
			parseFunc:   parseGenericVersion,
		},
		{
			name:        constants.ToolSpeckit,
			command:     constants.ToolSpeckit,
			versionFlag: constants.VersionFlagStandard,
			minVersion:  "",
			required:    false,
			managed:     true,
			installHint: "Install Speckit following https://github.com/speckit/speckit",
			parseFunc:   parseGenericVersion,
		},
	}
}

// Detect checks all configured tools and returns their status.
func (d *DefaultToolDetector) Detect(ctx context.Context) (*ToolDetectionResult, error) {
	// Check for cancellation at entry
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Apply timeout for detection
	detectCtx, cancel := context.WithTimeout(ctx, constants.ToolDetectionTimeout)
	defer cancel()

	configs := getToolConfigs()
	result := &ToolDetectionResult{
		Tools: make([]Tool, 0, len(configs)),
	}
	var resultMu sync.Mutex

	g, gCtx := errgroup.WithContext(detectCtx)

	for _, cfg := range configs {
		g.Go(func() error {
			tool := d.detectTool(gCtx, cfg)
			resultMu.Lock()
			result.Tools = append(result.Tools, tool)
			resultMu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed to detect tools: %w", err)
	}

	// Check if any required tools are missing
	for _, tool := range result.Tools {
		if tool.Required && (tool.Status == ToolStatusMissing || tool.Status == ToolStatusOutdated) {
			result.HasMissingRequired = true
			break
		}
	}

	return result, nil
}

// detectTool detects a single tool's status.
func (d *DefaultToolDetector) detectTool(ctx context.Context, cfg toolConfig) Tool {
	tool := Tool{
		Name:        cfg.name,
		Required:    cfg.required,
		Managed:     cfg.managed,
		MinVersion:  cfg.minVersion,
		InstallHint: cfg.installHint,
		Status:      ToolStatusMissing,
	}

	// Check if tool exists in PATH
	_, err := d.executor.LookPath(cfg.command)
	if err != nil {
		return tool
	}

	// Get version
	output, err := d.executor.Run(ctx, cfg.command, cfg.versionFlag)
	if err != nil {
		// Tool exists but version command failed - treat as installed without version info
		tool.Status = ToolStatusInstalled
		tool.CurrentVersion = "unknown"
		return tool
	}

	// Parse version
	tool.CurrentVersion = cfg.parseFunc(output)
	if tool.CurrentVersion == "" {
		tool.CurrentVersion = "unknown"
		tool.Status = ToolStatusInstalled
		return tool
	}

	// Compare versions if minimum is specified
	if cfg.minVersion != "" {
		cmp := CompareVersions(tool.CurrentVersion, cfg.minVersion)
		if cmp < 0 {
			tool.Status = ToolStatusOutdated
		} else {
			tool.Status = ToolStatusInstalled
		}
	} else {
		// No minimum version, just needs to be present
		tool.Status = ToolStatusInstalled
	}

	return tool
}

// Version parsing functions for each tool.
// All functions use pre-compiled regexes defined at package level for performance.

// parseGoVersion parses "go version go1.24.2 darwin/arm64" → "1.24.2"
func parseGoVersion(output string) string {
	if matches := goVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseGitVersion parses "git version 2.39.0" → "2.39.0"
func parseGitVersion(output string) string {
	if matches := gitVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseGHVersion parses "gh version 2.62.0 (2024-11-06)" → "2.62.0"
func parseGHVersion(output string) string {
	if matches := ghVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseUVVersion parses "uv 0.5.14 (bb7af57b8 2025-01-03)" → "0.5.14"
func parseUVVersion(output string) string {
	if matches := uvVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseClaudeVersion parses various Claude version formats.
// Examples: "Claude Code 2.0.76", "claude-code 2.0.76", "2.0.76"
func parseClaudeVersion(output string) string {
	for _, re := range claudeVersionPatterns {
		if matches := re.FindStringSubmatch(output); len(matches) >= 2 {
			return matches[1]
		}
	}
	return ""
}

// parseMageXVersion parses mage-x version output.
func parseMageXVersion(output string) string {
	if matches := mageVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseGeminiVersion parses various Gemini CLI version formats.
// Examples: "gemini 0.22.5", "gemini-cli 0.22.5", "0.22.5"
func parseGeminiVersion(output string) string {
	for _, re := range geminiVersionPatterns {
		if matches := re.FindStringSubmatch(output); len(matches) >= 2 {
			return matches[1]
		}
	}
	return ""
}

// parseCodexVersion parses various Codex CLI version formats.
// Examples: "codex 0.77.0", "Codex CLI v0.77.0", "0.77.0"
func parseCodexVersion(output string) string {
	for _, re := range codexVersionPatterns {
		if matches := re.FindStringSubmatch(output); len(matches) >= 2 {
			return matches[1]
		}
	}
	return ""
}

// parseGenericVersion extracts a version number from generic output.
func parseGenericVersion(output string) string {
	if matches := genericVersionRe.FindStringSubmatch(output); len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// CompareVersions compares two semantic versions.
// Returns:
//
//	-1 if current < required
//	 0 if current == required
//	 1 if current > required
func CompareVersions(current, required string) int {
	// Normalize versions by removing 'v' prefix
	current = strings.TrimPrefix(current, "v")
	required = strings.TrimPrefix(required, "v")

	currentParts := parseVersionParts(current)
	requiredParts := parseVersionParts(required)

	// Compare each part
	for i := 0; i < maxVersionSegments; i++ {
		if currentParts[i] < requiredParts[i] {
			return -1
		}
		if currentParts[i] > requiredParts[i] {
			return 1
		}
	}

	return 0
}

// parseVersionParts parses a version string into [major, minor, patch].
func parseVersionParts(version string) [maxVersionSegments]int {
	var parts [maxVersionSegments]int
	segments := strings.Split(version, ".")

	for i := 0; i < len(segments) && i < maxVersionSegments; i++ {
		// Extract only numeric portion (handle formats like "0.5.x")
		numStr := segments[i]
		for j, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:j]
				break
			}
		}
		if numStr != "" {
			parts[i], _ = strconv.Atoi(numStr)
		}
	}

	return parts
}

// IsGoPreCommitInstalled checks if go-pre-commit is installed and returns its version.
// This is a convenience function for the validation runner to quickly check tool availability
// without running full tool detection.
//
// Returns:
//   - installed: true if go-pre-commit is found in PATH
//   - version: the detected version string, or "unknown" if version check fails
//   - err: only set for unexpected errors (not for "tool not found")
func IsGoPreCommitInstalled(ctx context.Context) (installed bool, version string, err error) {
	return IsGoPreCommitInstalledWithExecutor(ctx, &DefaultCommandExecutor{})
}

// IsGoPreCommitInstalledWithExecutor is the testable version of IsGoPreCommitInstalled.
func IsGoPreCommitInstalledWithExecutor(ctx context.Context, executor CommandExecutor) (installed bool, version string, err error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, "", ctx.Err()
	default:
	}

	// Check if tool exists in PATH
	_, lookErr := executor.LookPath(constants.ToolGoPreCommit)
	if lookErr != nil {
		// Tool not found - not an error condition, just not installed
		// Return nil error because "not installed" is a valid response state
		return false, "", nil //nolint:nilerr // intentional: not found is not an error
	}

	// Get version
	output, runErr := executor.Run(ctx, constants.ToolGoPreCommit, constants.VersionFlagStandard)
	if runErr != nil {
		// Tool exists but version command failed - treat as installed without version info
		// Return nil error because version detection failure shouldn't fail the overall check
		return true, "unknown", nil //nolint:nilerr // intentional: version error is non-fatal
	}

	// Parse version using the same function as full tool detection
	parsedVersion := parseGenericVersion(output)
	if parsedVersion == "" {
		return true, "unknown", nil
	}

	return true, parsedVersion, nil
}

// FormatMissingToolsError creates a formatted error message for missing tools.
func FormatMissingToolsError(missing []Tool) string {
	if len(missing) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Missing required tools:\n\n")

	for _, tool := range missing {
		status := "missing"
		if tool.Status == ToolStatusOutdated {
			status = fmt.Sprintf("outdated (have %s, need %s)", tool.CurrentVersion, tool.MinVersion)
		}
		sb.WriteString(fmt.Sprintf("  • %s: %s\n", tool.Name, status))
		sb.WriteString(fmt.Sprintf("    Install: %s\n\n", tool.InstallHint))
	}

	return sb.String()
}
