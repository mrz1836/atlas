// Package git provides Git operations for ATLAS.
// This file provides git statistics for live UI display.
package git

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stats holds aggregated git statistics for display.
type Stats struct {
	NewFiles      int // Number of new/untracked files
	ModifiedFiles int // Number of modified files
	Additions     int // Lines added
	Deletions     int // Lines deleted
}

// FormatCompact returns a compact format like "3M +120/-45" for spinner display.
// Returns empty string if there are no changes.
func (s *Stats) FormatCompact() string {
	if s == nil {
		return ""
	}

	var parts []string

	// File count (new + modified)
	fileCount := s.NewFiles + s.ModifiedFiles
	if fileCount > 0 {
		if s.NewFiles > 0 && s.ModifiedFiles > 0 {
			parts = append(parts, strconv.Itoa(s.NewFiles)+"N "+strconv.Itoa(s.ModifiedFiles)+"M")
		} else if s.NewFiles > 0 {
			parts = append(parts, strconv.Itoa(s.NewFiles)+"N")
		} else {
			parts = append(parts, strconv.Itoa(s.ModifiedFiles)+"M")
		}
	}

	// Line counts
	if s.Additions > 0 || s.Deletions > 0 {
		parts = append(parts, "+"+strconv.Itoa(s.Additions)+"/-"+strconv.Itoa(s.Deletions))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

// IsEmpty returns true if there are no changes tracked.
func (s *Stats) IsEmpty() bool {
	if s == nil {
		return true
	}
	return s.NewFiles == 0 && s.ModifiedFiles == 0 && s.Additions == 0 && s.Deletions == 0
}

// StatsProvider calculates git stats with caching and debouncing.
// It's designed for non-blocking access from UI callbacks.
type StatsProvider struct {
	workDir    string
	mu         sync.RWMutex
	cached     *Stats
	debounce   time.Duration
	lastUpdate time.Time
	refreshing bool
}

// NewStatsProvider creates a new stats provider with 500ms debounce.
func NewStatsProvider(workDir string) *StatsProvider {
	return &StatsProvider{
		workDir:  workDir,
		debounce: 500 * time.Millisecond,
	}
}

// GetCachedStats returns the current cached stats (non-blocking).
// Returns nil if no stats have been calculated yet.
func (p *StatsProvider) GetCachedStats() *Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cached
}

// RefreshAsync triggers a background refresh if enough time has passed since the last refresh.
// This is non-blocking and safe to call frequently from UI callbacks.
func (p *StatsProvider) RefreshAsync(ctx context.Context) {
	p.mu.Lock()
	// Check if we should skip due to debounce or already refreshing
	if p.refreshing || time.Since(p.lastUpdate) < p.debounce {
		p.mu.Unlock()
		return
	}
	p.refreshing = true
	p.mu.Unlock()

	// Run refresh in background
	go func() {
		stats := p.calculateStats(ctx)

		p.mu.Lock()
		p.cached = stats
		p.lastUpdate = time.Now()
		p.refreshing = false
		p.mu.Unlock()
	}()
}

// calculateStats runs git commands to calculate current stats.
func (p *StatsProvider) calculateStats(ctx context.Context) *Stats {
	stats := &Stats{}

	// Check context cancellation early
	select {
	case <-ctx.Done():
		return stats
	default:
	}

	// Get status for file counts
	statusOutput, err := RunCommand(ctx, p.workDir, "status", "--porcelain", "-uall")
	if err == nil {
		stats.NewFiles, stats.ModifiedFiles = parseStatusForCounts(statusOutput)
	}

	// Get diff stats for line counts (both staged and unstaged)
	// Use --numstat for easy parsing
	numstatOutput, err := RunCommand(ctx, p.workDir, "diff", "--numstat", "HEAD")
	if err == nil {
		add, del := parseNumstat(numstatOutput)
		stats.Additions = add
		stats.Deletions = del
	}

	return stats
}

// parseStatusForCounts parses git status --porcelain output to count new and modified files.
func parseStatusForCounts(output string) (newFiles, modifiedFiles int) {
	if output == "" {
		return 0, 0
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		// Skip branch info line
		if strings.HasPrefix(line, "## ") {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]

		// Untracked files (new)
		if indexStatus == '?' && workTreeStatus == '?' {
			newFiles++
			continue
		}

		// Added files (staged new)
		if indexStatus == 'A' {
			newFiles++
			continue
		}

		// Modified files
		if indexStatus == 'M' || workTreeStatus == 'M' ||
			indexStatus == 'R' || workTreeStatus == 'R' ||
			indexStatus == 'D' || workTreeStatus == 'D' {
			modifiedFiles++
		}
	}

	return newFiles, modifiedFiles
}

// parseNumstat parses git diff --numstat output to count additions and deletions.
// Each line is: additions\tdeletions\tfilename
// Binary files show "-\t-\tfilename"
func parseNumstat(output string) (additions, deletions int) {
	if output == "" {
		return 0, 0
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}

		// Skip binary files (shown as "-")
		if parts[0] == "-" || parts[1] == "-" {
			continue
		}

		if add, err := strconv.Atoi(parts[0]); err == nil {
			additions += add
		}
		if del, err := strconv.Atoi(parts[1]); err == nil {
			deletions += del
		}
	}

	return additions, deletions
}
