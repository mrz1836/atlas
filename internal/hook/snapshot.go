package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/mrz1836/atlas/internal/domain"
)

// snapshotFiles captures state of files for debugging at a checkpoint.
// It is fault-tolerant: if a file is missing or unreadable, it records the failure
// rather than aborting the entire operation.
func snapshotFiles(paths []string) []domain.FileSnapshot {
	snapshots := make([]domain.FileSnapshot, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		snapshot := domain.FileSnapshot{Path: path}

		if err != nil {
			snapshot.Exists = false
		} else {
			snapshot.Exists = true
			snapshot.Size = info.Size()
			snapshot.ModTime = info.ModTime().Format(time.RFC3339)

			// Quick hash (first 16 chars of SHA256)
			// We limit the size read to avoid performance hit on large files
			if hash, err := hashFile(path); err == nil {
				snapshot.SHA256 = hash
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

// hashFile computes partial SHA256 hash of a file.
// Returns first 16 characters of the hex string.
func hashFile(path string) (string, error) {
	// Limit read to 10MB to avoid stalling on massive files
	// This is a snapshot for debugging, not a full integrity verification
	const maxReadSize = 10 * 1024 * 1024

	//nolint:gosec // G304: Path is from trusted sources (hook state files)
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()

	// Read up to limit
	b := make([]byte, 4096)
	var total int64
	for {
		n, err := f.Read(b)
		if n > 0 {
			h.Write(b[:n])
			total += int64(n)
		}
		if err != nil {
			break
		}
		if total >= maxReadSize {
			break
		}
	}

	hash := hex.EncodeToString(h.Sum(nil))
	if len(hash) > 16 {
		return hash[:16], nil
	}
	return hash, nil
}
