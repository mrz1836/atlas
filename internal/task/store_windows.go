//go:build windows

package task

import "golang.org/x/sys/windows"

// flockExclusive acquires an exclusive non-blocking lock on the file descriptor.
func flockExclusive(fd uintptr) error {
	// LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY
	return windows.LockFileEx(
		windows.Handle(fd),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&windows.Overlapped{},
	)
}

// flockUnlock releases the lock on the file descriptor.
func flockUnlock(fd uintptr) error {
	return windows.UnlockFileEx(
		windows.Handle(fd),
		0,
		1,
		0,
		&windows.Overlapped{},
	)
}
