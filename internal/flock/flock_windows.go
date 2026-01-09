//go:build windows

// Package flock provides cross-platform file locking utilities.
package flock

import "golang.org/x/sys/windows"

// Windows LockFileEx/UnlockFileEx API parameters.
// See: https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex
const (
	lockReserved  = 0 // Reserved parameter, must be zero
	lockBytesLow  = 1 // Low-order 32 bits of byte range to lock (1 byte = entire file)
	lockBytesHigh = 0 // High-order 32 bits of byte range to lock
)

// Exclusive acquires an exclusive non-blocking lock on the file descriptor.
// Returns an error if the lock cannot be acquired immediately.
func Exclusive(fd uintptr) error {
	return windows.LockFileEx(
		windows.Handle(fd),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		lockReserved,
		lockBytesLow,
		lockBytesHigh,
		&windows.Overlapped{},
	)
}

// Unlock releases the lock on the file descriptor.
func Unlock(fd uintptr) error {
	return windows.UnlockFileEx(
		windows.Handle(fd),
		lockReserved,
		lockBytesLow,
		lockBytesHigh,
		&windows.Overlapped{},
	)
}
