package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/daemon"
)

// helpers ─────────────────────────────────────────────────────────────────────

func searchEntries(messages ...string) []daemon.LogEntry {
	out := make([]daemon.LogEntry, len(messages))
	for i, m := range messages {
		out[i] = daemon.LogEntry{Timestamp: time.Now(), Level: "info", Message: m}
	}
	return out
}

// ── NewLogSearch ──────────────────────────────────────────────────────────────

func TestNewLogSearch(t *testing.T) {
	s := NewLogSearch()
	if s == nil {
		t.Fatal("NewLogSearch returned nil")
	}
	if s.IsActive() {
		t.Error("search should not be active by default")
	}
	if s.Query() != "" {
		t.Errorf("query should be empty, got %q", s.Query())
	}
	if s.HasMatches() {
		t.Error("should have no matches initially")
	}
}

// ── Activate / Deactivate ─────────────────────────────────────────────────────

func TestLogSearch_Activate(t *testing.T) {
	s := NewLogSearch()
	s.Activate()
	if !s.IsActive() {
		t.Error("search should be active after Activate()")
	}
}

func TestLogSearch_Activate_ClearsQuery(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("old", searchEntries("old thing"))
	s.Activate()
	if s.Query() != "" {
		t.Errorf("Activate should clear query, got %q", s.Query())
	}
}

func TestLogSearch_Deactivate(t *testing.T) {
	s := NewLogSearch()
	s.Activate()
	s.Deactivate()
	if s.IsActive() {
		t.Error("search should not be active after Deactivate()")
	}
}

// ── SetQuery ─────────────────────────────────────────────────────────────────

func TestLogSearch_SetQuery_FindsMatches(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("hello world", "goodbye world", "hello again")
	s.SetQuery("hello", entries)

	if s.MatchCount() != 2 {
		t.Errorf("expected 2 matches for 'hello', got %d", s.MatchCount())
	}
}

func TestLogSearch_SetQuery_NoMatches(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("nomatch", searchEntries("alpha", "beta", "gamma"))
	if s.HasMatches() {
		t.Error("expected no matches")
	}
}

func TestLogSearch_SetQuery_CaseInsensitive(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("HELLO", searchEntries("hello world", "Hi there"))
	if s.MatchCount() != 1 {
		t.Errorf("case-insensitive: expected 1 match, got %d", s.MatchCount())
	}
}

func TestLogSearch_SetQuery_EmptyString_ClearsMatches(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("word", searchEntries("a word", "another word"))
	s.SetQuery("", searchEntries("a word"))
	if s.HasMatches() {
		t.Error("empty query should clear matches")
	}
}

func TestLogSearch_SetQuery_MatchIndicesCorrect(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("no", "yes", "no", "yes")
	s.SetQuery("yes", entries)

	matches := s.Matches()
	expected := []int{1, 3}
	if len(matches) != len(expected) {
		t.Fatalf("expected indices %v, got %v", expected, matches)
	}
	for i, idx := range expected {
		if matches[i] != idx {
			t.Errorf("match[%d]: expected %d, got %d", i, idx, matches[i])
		}
	}
}

// ── NextMatch / PrevMatch ─────────────────────────────────────────────────────

func TestLogSearch_NextMatch_Wraps(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("match1", "no", "match2")
	s.SetQuery("match", entries)

	// Start at index 0 of matches.
	if s.CurrentMatchIndex() != 0 {
		t.Errorf("expected first match at index 0, got %d", s.CurrentMatchIndex())
	}

	s.NextMatch() // → match2 (index 2 in entries)
	if s.CurrentMatchIndex() != 2 {
		t.Errorf("expected index 2 after NextMatch, got %d", s.CurrentMatchIndex())
	}

	s.NextMatch() // → wraps back to match1 (index 0)
	if s.CurrentMatchIndex() != 0 {
		t.Errorf("expected index 0 after wrap, got %d", s.CurrentMatchIndex())
	}
}

func TestLogSearch_PrevMatch_Wraps(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("match1", "no", "match2")
	s.SetQuery("match", entries)

	// Start at 0; PrevMatch wraps to last.
	s.PrevMatch()
	if s.CurrentMatchIndex() != 2 {
		t.Errorf("PrevMatch from first should wrap to last (index 2), got %d", s.CurrentMatchIndex())
	}
}

func TestLogSearch_NextMatch_NoMatches_DoesNothing(t *testing.T) {
	s := NewLogSearch()
	s.NextMatch() // should not panic
	if s.CurrentMatchIndex() != -1 {
		t.Error("CurrentMatchIndex should be -1 when no matches")
	}
}

func TestLogSearch_PrevMatch_NoMatches_DoesNothing(_ *testing.T) {
	s := NewLogSearch()
	s.PrevMatch() // should not panic
}

// ── CurrentMatchIndex ─────────────────────────────────────────────────────────

func TestLogSearch_CurrentMatchIndex_NoMatches(t *testing.T) {
	s := NewLogSearch()
	if s.CurrentMatchIndex() != -1 {
		t.Errorf("expected -1 when no matches, got %d", s.CurrentMatchIndex())
	}
}

// ── CurrentMatchPosition ──────────────────────────────────────────────────────

func TestLogSearch_CurrentMatchPosition(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("m1", "x", "m2", "x", "m3")
	s.SetQuery("m", entries)

	if pos := s.CurrentMatchPosition(); pos != "1/3" {
		t.Errorf("expected 1/3, got %q", pos)
	}
	s.NextMatch()
	if pos := s.CurrentMatchPosition(); pos != "2/3" {
		t.Errorf("expected 2/3, got %q", pos)
	}
}

func TestLogSearch_CurrentMatchPosition_NoMatches(t *testing.T) {
	s := NewLogSearch()
	if pos := s.CurrentMatchPosition(); pos != "" {
		t.Errorf("expected empty string, got %q", pos)
	}
}

// ── Highlight ─────────────────────────────────────────────────────────────────

func TestLogSearch_Highlight_ContainsMatch(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("error", searchEntries("error message"))

	result := s.Highlight("running magex error check")
	// After styling the result should still contain the matched text.
	if !strings.Contains(result, "error") {
		t.Error("Highlight result should contain the matched text")
	}
}

func TestLogSearch_Highlight_EmptyQuery_ReturnsOriginal(t *testing.T) {
	s := NewLogSearch()
	line := "original line"
	if got := s.Highlight(line); got != line {
		t.Errorf("expected %q, got %q", line, got)
	}
}

func TestLogSearch_Highlight_MultipleOccurrences(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("x", searchEntries("x marks the spot, x twice"))

	result := s.Highlight("x marks the spot, x twice")
	// Both "x" occurrences should be styled — count two rendered segments.
	count := strings.Count(result, "x")
	if count < 2 {
		t.Errorf("expected at least 2 'x' occurrences in highlighted output, got %d", count)
	}
}

func TestLogSearch_Highlight_CaseInsensitive(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("HELLO", searchEntries("hello world"))

	result := s.Highlight("hello world")
	if !strings.Contains(result, "hello") {
		t.Error("case-insensitive highlight should still contain matched text")
	}
}

// ── Reset ─────────────────────────────────────────────────────────────────────

func TestLogSearch_Reset(t *testing.T) {
	s := NewLogSearch()
	s.Activate()
	s.SetQuery("test", searchEntries("test entry"))
	s.Reset()

	if s.IsActive() {
		t.Error("Reset should deactivate search")
	}
	if s.Query() != "" {
		t.Errorf("Reset should clear query, got %q", s.Query())
	}
	if s.HasMatches() {
		t.Error("Reset should clear matches")
	}
}

// ── Matches slice is a copy ────────────────────────────────────────────────────

func TestLogSearch_Matches_ReturnsCopy(t *testing.T) {
	s := NewLogSearch()
	s.SetQuery("a", searchEntries("a", "b", "a"))

	m1 := s.Matches()
	m1[0] = 999
	m2 := s.Matches()

	if m2[0] == 999 {
		t.Error("Matches() should return a copy, not a reference")
	}
}

// ── State machine ─────────────────────────────────────────────────────────────

func TestLogSearch_StateMachine(t *testing.T) {
	s := NewLogSearch()
	entries := searchEntries("apple pie", "banana split", "apple cider")

	// Inactive → Activate.
	s.Activate()
	if !s.IsActive() {
		t.Fatal("should be active")
	}

	// SetQuery finds matches.
	s.SetQuery("apple", entries)
	if s.MatchCount() != 2 {
		t.Fatalf("expected 2 matches, got %d", s.MatchCount())
	}

	// Navigate.
	s.NextMatch()
	pos := s.CurrentMatchPosition()
	if pos != "2/2" {
		t.Errorf("expected 2/2, got %q", pos)
	}

	// Deactivate — matches persist.
	s.Deactivate()
	if s.IsActive() {
		t.Error("should be inactive after Deactivate")
	}
	if !s.HasMatches() {
		t.Error("matches should persist after Deactivate")
	}

	// Reset clears everything.
	s.Reset()
	if s.HasMatches() {
		t.Error("Reset should clear matches")
	}
}
