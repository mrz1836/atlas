//go:build unix

package workspace

import "syscall"

// flockExclusive acquires an exclusive non-blocking lock on the file descriptor.
func flockExclusive(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB)
}

// flockUnlock releases the lock on the file descriptor.
func flockUnlock(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}
