//go:build unix

// Package flock provides cross-platform file locking utilities.
package flock

import "syscall"

// Exclusive acquires an exclusive non-blocking lock on the file descriptor.
// Returns an error if the lock cannot be acquired immediately.
func Exclusive(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // G115: uintptr->int for syscall, file descriptors fit in int on all supported platforms
}

// Unlock releases the lock on the file descriptor.
func Unlock(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN) //nolint:gosec // G115: uintptr->int for syscall, file descriptors fit in int on all supported platforms
}
