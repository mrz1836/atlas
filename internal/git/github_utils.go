// Package git provides Git operations for ATLAS.
// This file contains shared utility functions for GitHub operations.
package git

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// addJitter adds random jitter to a duration to prevent synchronized retry storms.
// The factor determines the jitter range: 0.2 means +/- 20% of the base duration.
func addJitter(d time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return d
	}
	// Generate random value between -factor and +factor
	jitterRatio := (rand.Float64()*2 - 1) * factor //nolint:gosec // Non-cryptographic use for jitter
	jitter := time.Duration(float64(d) * jitterRatio)
	return d + jitter
}

// filterChecks filters checks by required check names with wildcard support.
func filterChecks(checks []CheckResult, required []string) []CheckResult {
	if len(required) == 0 {
		return checks // No filter, return all
	}

	var filtered []CheckResult
	for _, check := range checks {
		if matchesAnyPattern(check.Name, required) {
			filtered = append(filtered, check)
		}
	}
	return filtered
}

// matchesAnyPattern checks if name matches any of the patterns.
// Supports glob-style wildcards: "CI*" matches "CI / lint"
func matchesAnyPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		// Exact match
		if pattern == name {
			return true
		}
		// Prefix matching for patterns ending in *
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

// FormatCIProgressMessage generates a human-readable progress message for CI monitoring.
// Format: "Waiting for CI... (5m elapsed, checking: CI, Lint)"
func FormatCIProgressMessage(elapsed time.Duration, checks []CheckResult) string {
	if len(checks) == 0 {
		return fmt.Sprintf("Waiting for CI... (%s elapsed, no checks found)", formatDuration(elapsed))
	}

	// Collect unique check names (prefer workflow name, fallback to check name)
	names := make([]string, 0, len(checks))
	seen := make(map[string]bool)
	for _, check := range checks {
		name := check.Workflow
		if name == "" {
			name = check.Name
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return fmt.Sprintf("Waiting for CI... (%s elapsed, checking: %s)",
		formatDuration(elapsed), strings.Join(names, ", "))
}

// formatDuration formats a duration in a human-friendly way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		// Show minutes and seconds for better granularity
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
