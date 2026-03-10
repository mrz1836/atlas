package dashboard

import "github.com/mrz1836/atlas/internal/daemon"

const (
	// logBufferCap is the maximum number of log entries stored per task.
	// Entries beyond this limit overwrite the oldest (ring buffer behavior).
	logBufferCap = 10_000

	// LogLevelAll shows all levels (debug, info, warn, error).
	LogLevelAll = "all"
	// LogLevelInfo shows info and above (info, warn, error).
	LogLevelInfo = "info"
	// LogLevelWarn shows warn and above (warn, error).
	LogLevelWarn = "warn"
	// LogLevelError shows error only.
	LogLevelError = "error"
)

// levelRank maps level strings to integers for ordered comparison.
// Higher rank = higher severity.
//
//nolint:gochecknoglobals // Intentional package-level constants for log level ordering
var levelRank = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
}

// minRankForFilter maps filter names to the minimum log level rank that should
// be displayed. Entries with rank >= the minimum pass the filter.
//
//nolint:gochecknoglobals // Intentional package-level constants for filter thresholds
var minRankForFilter = map[string]int{
	LogLevelAll:   0, // debug and above
	LogLevelInfo:  1, // info and above
	LogLevelWarn:  2, // warn and above
	LogLevelError: 3, // error only
}

// LogBuffer is a ring buffer of daemon.LogEntry values capped at logBufferCap.
// It is intentionally not thread-safe: Bubble Tea's single-goroutine model
// means all mutations happen in the Update loop without concurrent access.
type LogBuffer struct {
	// entries is the underlying fixed-capacity slice.
	entries [logBufferCap]daemon.LogEntry
	// head is the index of the oldest entry (read position).
	head int
	// tail is the index where the next write will go.
	tail int
	// count is the number of valid entries currently stored.
	count int
}

// NewLogBuffer creates an empty LogBuffer ready for use.
func NewLogBuffer() *LogBuffer {
	return &LogBuffer{}
}

// Add appends a log entry to the buffer.
// When the buffer is full the oldest entry is overwritten (ring behavior).
func (b *LogBuffer) Add(entry daemon.LogEntry) {
	b.entries[b.tail] = entry
	b.tail = (b.tail + 1) % logBufferCap

	if b.count < logBufferCap {
		b.count++
	} else {
		// Buffer full: advance head to drop the oldest entry.
		b.head = (b.head + 1) % logBufferCap
	}
}

// Filter returns a slice of entries that match the given level filter.
// Entries are returned in insertion order (oldest first).
//
// Supported filter values:
//   - "all"   → every entry
//   - "info"  → info, warn, error
//   - "warn"  → warn, error
//   - "error" → error only
//
// An unrecognized filter value is treated as "all".
func (b *LogBuffer) Filter(level string) []daemon.LogEntry {
	minRank, ok := minRankForFilter[level]
	if !ok {
		minRank = 0 // default: show everything
	}

	out := make([]daemon.LogEntry, 0, b.count)
	for i := 0; i < b.count; i++ {
		idx := (b.head + i) % logBufferCap
		entry := b.entries[idx]
		rank, rankOK := levelRank[entry.Level]
		if !rankOK {
			rank = 0 // unknown levels pass any filter
		}
		if rank >= minRank {
			out = append(out, entry)
		}
	}
	return out
}

// Len returns the number of entries currently stored in the buffer.
func (b *LogBuffer) Len() int {
	return b.count
}

// Clear resets the buffer to an empty state, discarding all entries.
func (b *LogBuffer) Clear() {
	b.head = 0
	b.tail = 0
	b.count = 0
}
