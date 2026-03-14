package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/mrz1836/atlas/internal/daemon"
)

// makeEntry creates a test LogEntry with the given level and message.
func makeEntry(level, msg string) daemon.LogEntry {
	return daemon.LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
	}
}

// ── NewLogBuffer ─────────────────────────────────────────────────────────────

func TestNewLogBuffer(t *testing.T) {
	b := NewLogBuffer()
	if b == nil {
		t.Fatal("NewLogBuffer returned nil")
	}
	if b.Len() != 0 {
		t.Errorf("expected Len 0, got %d", b.Len())
	}
}

// ── Add / Len ─────────────────────────────────────────────────────────────────

func TestLogBuffer_Add_IncreasesLen(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("info", "a"))
	b.Add(makeEntry("warn", "b"))
	if b.Len() != 2 {
		t.Errorf("expected Len 2, got %d", b.Len())
	}
}

// ── Ring buffer overflow ──────────────────────────────────────────────────────

func TestLogBuffer_Overflow_CapsAtMaxCap(t *testing.T) {
	b := NewLogBuffer()
	// Fill beyond cap.
	total := logBufferCap + 500
	for i := 0; i < total; i++ {
		b.Add(makeEntry("info", fmt.Sprintf("msg-%d", i)))
	}

	if b.Len() != logBufferCap {
		t.Errorf("expected Len %d after overflow, got %d", logBufferCap, b.Len())
	}
}

func TestLogBuffer_Overflow_OldestDropped(t *testing.T) {
	b := NewLogBuffer()
	// Fill exactly to cap, then add one more.
	for i := 0; i < logBufferCap; i++ {
		b.Add(makeEntry("info", fmt.Sprintf("msg-%d", i)))
	}
	b.Add(makeEntry("info", "overflow"))

	entries := b.Filter(LogLevelAll)
	// The oldest entry (msg-0) should be gone; the newest should be "overflow".
	if entries[0].Message != "msg-1" {
		t.Errorf("expected oldest to be msg-1 after overflow, got %q", entries[0].Message)
	}
	if entries[len(entries)-1].Message != "overflow" {
		t.Errorf("expected newest to be overflow, got %q", entries[len(entries)-1].Message)
	}
}

// ── Entry ordering ────────────────────────────────────────────────────────────

func TestLogBuffer_Filter_PreservesOrder(t *testing.T) {
	b := NewLogBuffer()
	messages := []string{"first", "second", "third"}
	for _, m := range messages {
		b.Add(makeEntry("info", m))
	}

	got := b.Filter(LogLevelAll)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for i, m := range messages {
		if got[i].Message != m {
			t.Errorf("index %d: expected %q, got %q", i, m, got[i].Message)
		}
	}
}

// ── Level filtering ───────────────────────────────────────────────────────────

func TestLogBuffer_Filter_All(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("debug", "d"))
	b.Add(makeEntry("info", "i"))
	b.Add(makeEntry("warn", "w"))
	b.Add(makeEntry("error", "e"))

	got := b.Filter(LogLevelAll)
	if len(got) != 4 {
		t.Errorf("filter=all: expected 4, got %d", len(got))
	}
}

func TestLogBuffer_Filter_Info(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("debug", "d"))
	b.Add(makeEntry("info", "i"))
	b.Add(makeEntry("warn", "w"))
	b.Add(makeEntry("error", "e"))

	got := b.Filter(LogLevelInfo)
	if len(got) != 3 {
		t.Errorf("filter=info: expected 3, got %d", len(got))
	}
	if got[0].Level != "info" {
		t.Errorf("first entry should be info, got %s", got[0].Level)
	}
}

func TestLogBuffer_Filter_Warn(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("debug", "d"))
	b.Add(makeEntry("info", "i"))
	b.Add(makeEntry("warn", "w"))
	b.Add(makeEntry("error", "e"))

	got := b.Filter(LogLevelWarn)
	if len(got) != 2 {
		t.Errorf("filter=warn: expected 2, got %d", len(got))
	}
	for _, e := range got {
		if e.Level != "warn" && e.Level != "error" {
			t.Errorf("unexpected level %q in warn filter", e.Level)
		}
	}
}

func TestLogBuffer_Filter_Error(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("debug", "d"))
	b.Add(makeEntry("info", "i"))
	b.Add(makeEntry("warn", "w"))
	b.Add(makeEntry("error", "e"))

	got := b.Filter(LogLevelError)
	if len(got) != 1 {
		t.Errorf("filter=error: expected 1, got %d", len(got))
	}
	if got[0].Level != "error" {
		t.Errorf("expected error level, got %s", got[0].Level)
	}
}

func TestLogBuffer_Filter_UnknownLevel_ShowsAll(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("debug", "d"))
	b.Add(makeEntry("info", "i"))

	got := b.Filter("bogus")
	if len(got) != 2 {
		t.Errorf("unknown filter: expected 2, got %d", len(got))
	}
}

// ── Clear ─────────────────────────────────────────────────────────────────────

func TestLogBuffer_Clear(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("info", "a"))
	b.Add(makeEntry("info", "b"))
	b.Clear()

	if b.Len() != 0 {
		t.Errorf("expected Len 0 after Clear, got %d", b.Len())
	}
	if got := b.Filter(LogLevelAll); len(got) != 0 {
		t.Errorf("expected empty slice after Clear, got %d entries", len(got))
	}
}

func TestLogBuffer_AddAfterClear(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("info", "before"))
	b.Clear()
	b.Add(makeEntry("warn", "after"))

	entries := b.Filter(LogLevelAll)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after clear+add, got %d", len(entries))
	}
	if entries[0].Message != "after" {
		t.Errorf("expected 'after', got %q", entries[0].Message)
	}
}

// ── Level filtering with mixed unknown levels ─────────────────────────────────

func TestLogBuffer_Filter_UnknownEntryLevel_PassesAnyFilter(t *testing.T) {
	b := NewLogBuffer()
	b.Add(makeEntry("trace", "t")) // unknown level — rank 0
	b.Add(makeEntry("info", "i"))

	// Even strict filters should pass unknown-level entries (rank 0 >= 0).
	got := b.Filter(LogLevelAll)
	if len(got) != 2 {
		t.Errorf("expected 2 (unknown passes all), got %d", len(got))
	}
	// Warn filter: unknown rank 0 < minRank 2 → should NOT pass
	gotWarn := b.Filter(LogLevelWarn)
	for _, e := range gotWarn {
		if e.Level == "trace" {
			t.Error("trace level should not pass warn filter")
		}
	}
}
