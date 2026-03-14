package dashboard

import (
	"strings"

	"github.com/mrz1836/atlas/internal/daemon"
)

// LogSearch manages search state for the log panel.
// It stores the active query, the list of matching entry indices (into the
// filtered entry slice), and the current match cursor.
//
// Search applies on top of the active level filter: the caller passes the
// already-filtered []daemon.LogEntry to SetQuery so that indices returned by
// Matches() align with what the LogPanel renders.
type LogSearch struct {
	// query is the current search string. Empty string means no active search.
	query string
	// active indicates whether search mode is active (user pressed '/').
	active bool
	// matches holds the indices (into the filtered entry slice) that contain
	// the query string (case-insensitive).
	matches []int
	// currentMatch is the index into matches pointing to the highlighted match.
	currentMatch int
}

// NewLogSearch creates an inactive LogSearch with no query.
func NewLogSearch() *LogSearch {
	return &LogSearch{}
}

// Activate enters search mode (e.g., when the user presses '/').
// The query is cleared and matches are reset.
func (s *LogSearch) Activate() {
	s.active = true
	s.query = ""
	s.matches = nil
	s.currentMatch = 0
}

// Deactivate exits search mode without clearing the query or matches.
// Useful when pressing Esc — the previous highlights remain visible.
func (s *LogSearch) Deactivate() {
	s.active = false
}

// Reset clears all search state. Called when switching to a new task.
func (s *LogSearch) Reset() {
	s.active = false
	s.query = ""
	s.matches = nil
	s.currentMatch = 0
}

// SetQuery updates the search query and rebuilds the match index from entries.
// entries should be the level-filtered slice the LogPanel is currently rendering.
// Matching is case-insensitive substring search.
func (s *LogSearch) SetQuery(query string, entries []daemon.LogEntry) {
	s.query = query
	s.matches = nil
	s.currentMatch = 0

	if query == "" {
		return
	}

	lower := strings.ToLower(query)
	for i, e := range entries {
		if strings.Contains(strings.ToLower(e.Message), lower) {
			s.matches = append(s.matches, i)
		}
	}
}

// NextMatch advances the match cursor forward (wraps around).
// Does nothing if there are no matches.
func (s *LogSearch) NextMatch() {
	if len(s.matches) == 0 {
		return
	}
	s.currentMatch = (s.currentMatch + 1) % len(s.matches)
}

// PrevMatch moves the match cursor backward (wraps around).
// Does nothing if there are no matches.
func (s *LogSearch) PrevMatch() {
	if len(s.matches) == 0 {
		return
	}
	s.currentMatch = (s.currentMatch - 1 + len(s.matches)) % len(s.matches)
}

// CurrentMatchIndex returns the entry index (into the filtered slice) of the
// current match, or -1 if there are no matches.
func (s *LogSearch) CurrentMatchIndex() int {
	if len(s.matches) == 0 {
		return -1
	}
	return s.matches[s.currentMatch]
}

// IsActive returns true when search mode is currently active.
func (s *LogSearch) IsActive() bool { return s.active }

// HasMatches returns true when there is at least one match.
func (s *LogSearch) HasMatches() bool { return len(s.matches) > 0 }

// Query returns the current search query string.
func (s *LogSearch) Query() string { return s.query }

// Matches returns a copy of the match indices slice.
// The indices reference positions in the level-filtered entry slice.
func (s *LogSearch) Matches() []int {
	if s.matches == nil {
		return nil
	}
	out := make([]int, len(s.matches))
	copy(out, s.matches)
	return out
}

// MatchCount returns the number of matching entries.
func (s *LogSearch) MatchCount() int { return len(s.matches) }

// CurrentMatchPosition returns the 1-based position of the current match
// within the total match set, e.g., "3/7". Returns "" when there are no matches.
func (s *LogSearch) CurrentMatchPosition() string {
	if len(s.matches) == 0 {
		return ""
	}
	return formatMatchPos(s.currentMatch+1, len(s.matches))
}

// formatMatchPos formats "current/total" — extracted for testability.
func formatMatchPos(current, total int) string {
	return strings.Join([]string{itoa(current), "/", itoa(total)}, "")
}

// itoa converts an int to a decimal string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// Highlight wraps occurrences of the query in the given line with the dashboard
// search-highlight style. Returns line unchanged if no query is set.
//
// The highlight preserves the original case of the matched text; only the
// visual styling is applied.
func (s *LogSearch) Highlight(line string) string {
	if s.query == "" {
		return line
	}

	styles := GetStyles()
	lower := strings.ToLower(line)
	lowerQ := strings.ToLower(s.query)

	var sb strings.Builder
	remaining := line
	lowerRemaining := lower

	for {
		idx := strings.Index(lowerRemaining, lowerQ)
		if idx < 0 {
			sb.WriteString(remaining)
			break
		}
		// Write everything before the match unchanged.
		sb.WriteString(remaining[:idx])
		// Write the matched portion with highlight style.
		matched := remaining[idx : idx+len(s.query)]
		sb.WriteString(styles.Highlight.Render(matched))
		// Advance past the match.
		remaining = remaining[idx+len(s.query):]
		lowerRemaining = lowerRemaining[idx+len(lowerQ):]
	}

	return sb.String()
}
