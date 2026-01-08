//go:build windows

// Package flock provides cross-platform file locking utilities.
package flock

import "golang.org/x/sys/windows"

// Exclusive acquires an exclusive non-blocking lock on the file descriptor.
// Returns an error if the lock cannot be acquired immediately.
func Exclusive(fd uintptr) error {
	return windows.LockFileEx(
		windows.Handle(fd),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&windows.Overlapped{},
	)
}

// Unlock releases the lock on the file descriptor.
func Unlock(fd uintptr) error {
	return windows.UnlockFileEx(
		windows.Handle(fd),
		0,
		1,
		0,
		&windows.Overlapped{},
	)
}
