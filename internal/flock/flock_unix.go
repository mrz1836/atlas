//go:build unix

// Package flock provides cross-platform file locking utilities.
package flock

import "syscall"

// Exclusive acquires an exclusive non-blocking lock on the file descriptor.
// Returns an error if the lock cannot be acquired immediately.
func Exclusive(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB)
}

// Unlock releases the lock on the file descriptor.
func Unlock(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}
