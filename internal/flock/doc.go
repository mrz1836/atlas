// Package flock provides cross-platform file locking utilities.
//
// This package consolidates file locking logic that was previously duplicated
// across the workspace and task packages. It provides exclusive, non-blocking
// file locks that work on both Unix and Windows systems.
//
// Usage:
//
//	file, _ := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
//	if err := flock.Exclusive(file.Fd()); err != nil {
//	    // Lock not acquired - file is in use
//	}
//	defer flock.Unlock(file.Fd())
package flock
