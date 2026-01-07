// Package git provides Git operations for ATLAS.
// This file implements garbage detection for files that shouldn't be committed.
package git

import (
	"path/filepath"
	"strings"
)

// =============================================================================
// GARBAGE FILE PATTERNS
// =============================================================================
// The following patterns define files that should not be committed.
// Patterns use glob syntax (* matches any characters, / indicates directories).
//
// DEBUG FILES:
//   __debug_bin*         - Delve debugger binaries
//
// SECRET/CREDENTIAL FILES:
//   .env                 - Environment variables
//   .env.*               - Environment variants (.env.local, .env.production)
//   credentials*         - Credential files (credentials.json, etc.)
//   *.key                - Private key files
//   *.pem                - PEM certificate/key files
//   *.p12                - PKCS#12 keystore files
//
// BUILD ARTIFACTS:
//   coverage.out         - Go test coverage output
//   coverage.html        - Coverage HTML report
//   vendor/              - Go vendor directory
//   node_modules/        - Node.js dependencies
//   dist/                - Distribution/build output
//   build/               - Build output directory
//   .DS_Store            - macOS Finder metadata
//   *.exe                - Windows executables
//   *.dll                - Windows dynamic libraries
//   *.so                 - Linux/Unix shared objects
//   *.dylib              - macOS dynamic libraries
//
// TEMPORARY FILES:
//   *.tmp                - Temporary files (e.g., something.go.tmp)
//   *.bak                - Backup files
//   *.swp                - Vim swap files
//   *~                   - Editor backup files (Emacs, etc.)
//   *.orig               - Original files from merge conflicts
// =============================================================================

// GarbageCategory represents the category of garbage file detected.
type GarbageCategory string

// Garbage category constants.
const (
	GarbageDebug         GarbageCategory = "debug"
	GarbageSecrets       GarbageCategory = "secrets"
	GarbageBuildArtifact GarbageCategory = "build_artifact"
	GarbageTempFile      GarbageCategory = "temp_file"
)

// GarbageFile represents a file identified as garbage.
type GarbageFile struct {
	Path     string          // File path relative to repo root
	Category GarbageCategory // Type of garbage
	Reason   string          // Human-readable reason for detection
}

// GarbageConfig holds configurable patterns for garbage detection.
type GarbageConfig struct {
	DebugPatterns    []string // Patterns for debug files
	SecretPatterns   []string // Patterns for secrets/credentials
	BuildPatterns    []string // Patterns for build artifacts
	TempFilePatterns []string // Patterns for temporary files
}

// DefaultGarbageConfig returns the default garbage detection patterns for Go projects.
func DefaultGarbageConfig() *GarbageConfig {
	return &GarbageConfig{
		DebugPatterns: []string{
			"__debug_bin*",
		},
		SecretPatterns: []string{
			".env",
			".env.*",
			"credentials*",
			"*.key",
			"*.pem",
			"*.p12",
		},
		BuildPatterns: []string{
			"coverage.out",
			"coverage.html",
			"vendor/",
			"node_modules/",
			"dist/",
			"build/",
			".DS_Store",
			"*.exe",
			"*.dll",
			"*.so",
			"*.dylib",
		},
		TempFilePatterns: []string{
			"*.tmp",
			"*.bak",
			"*.swp",
			"*~",
			"*.orig",
		},
	}
}

// GarbageDetector detects garbage files that shouldn't be committed.
type GarbageDetector struct {
	config *GarbageConfig
}

// NewGarbageDetector creates a new GarbageDetector with the given config.
// If config is nil, uses DefaultGarbageConfig.
func NewGarbageDetector(config *GarbageConfig) *GarbageDetector {
	if config == nil {
		config = DefaultGarbageConfig()
	}
	return &GarbageDetector{config: config}
}

// DetectGarbage analyzes a list of file paths and returns any garbage files found.
func (d *GarbageDetector) DetectGarbage(files []string) []GarbageFile {
	var garbage []GarbageFile

	for _, file := range files {
		if gf := d.checkFile(file); gf != nil {
			garbage = append(garbage, *gf)
		}
	}

	return garbage
}

// checkFile checks a single file against all garbage patterns.
// Returns nil if the file is not garbage.
func (d *GarbageDetector) checkFile(path string) *GarbageFile {
	// Normalize path separators
	path = filepath.ToSlash(path)
	filename := filepath.Base(path)

	// Check debug patterns
	if reason := d.matchPatterns(filename, path, d.config.DebugPatterns); reason != "" {
		return &GarbageFile{
			Path:     path,
			Category: GarbageDebug,
			Reason:   reason,
		}
	}

	// Check secret patterns (but exclude public keys)
	if reason := d.matchSecretPatterns(filename); reason != "" {
		return &GarbageFile{
			Path:     path,
			Category: GarbageSecrets,
			Reason:   reason,
		}
	}

	// Check build artifact patterns
	if reason := d.matchPatterns(filename, path, d.config.BuildPatterns); reason != "" {
		return &GarbageFile{
			Path:     path,
			Category: GarbageBuildArtifact,
			Reason:   reason,
		}
	}

	// Check temp file patterns
	if reason := d.matchPatterns(filename, path, d.config.TempFilePatterns); reason != "" {
		return &GarbageFile{
			Path:     path,
			Category: GarbageTempFile,
			Reason:   reason,
		}
	}

	return nil
}

// matchPatterns checks if the file matches any of the given patterns.
// Returns the matched pattern as the reason, or empty string if no match.
func (d *GarbageDetector) matchPatterns(filename, fullPath string, patterns []string) string {
	for _, pattern := range patterns {
		// Handle directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			// Check if path contains this directory
			if strings.Contains(fullPath, dirPattern+"/") || strings.HasPrefix(fullPath, dirPattern+"/") {
				return "matches directory pattern: " + pattern
			}
			continue
		}

		// Use filepath.Match for glob patterns on filename
		if matched, _ := filepath.Match(pattern, filename); matched {
			return "matches pattern: " + pattern
		}

		// Also try matching against full path for nested patterns
		if matched, _ := filepath.Match(pattern, fullPath); matched {
			return "matches pattern: " + pattern
		}
	}
	return ""
}

// matchSecretPatterns checks if the file matches secret patterns.
// Excludes public keys and certificates that are safe to commit.
func (d *GarbageDetector) matchSecretPatterns(filename string) string {
	// Public keys are generally safe to commit
	if strings.HasSuffix(filename, ".pub") {
		return ""
	}

	// Check against secret patterns
	for _, pattern := range d.config.SecretPatterns {
		// Handle patterns with dots specially (like .env, .env.*)
		if strings.HasPrefix(pattern, ".env") {
			// Exact match for .env
			if pattern == ".env" && filename == ".env" {
				return "matches pattern: " + pattern
			}
			// .env.* pattern matches .env.local, .env.production, etc.
			if pattern == ".env.*" && strings.HasPrefix(filename, ".env.") {
				return "matches pattern: " + pattern
			}
			continue
		}

		// Use filepath.Match for glob patterns
		if matched, _ := filepath.Match(pattern, filename); matched {
			return "matches pattern: " + pattern
		}
	}
	return ""
}

// HasGarbage returns true if any garbage files were detected.
func HasGarbage(garbage []GarbageFile) bool {
	return len(garbage) > 0
}

// FilterByCategory returns garbage files of a specific category.
func FilterByCategory(garbage []GarbageFile, category GarbageCategory) []GarbageFile {
	var filtered []GarbageFile
	for _, g := range garbage {
		if g.Category == category {
			filtered = append(filtered, g)
		}
	}
	return filtered
}

// GarbageSummary returns a human-readable summary of detected garbage.
func GarbageSummary(garbage []GarbageFile) map[GarbageCategory]int {
	summary := make(map[GarbageCategory]int)
	for _, g := range garbage {
		summary[g.Category]++
	}
	return summary
}
