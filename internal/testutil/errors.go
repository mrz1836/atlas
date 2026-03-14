// Package testutil provides testing utilities for ATLAS.
//
// This package contains mock errors and test helpers used across test files.
// It should only be imported by test files (*_test.go).
package testutil

import "errors"

// Mock errors for testing purposes.
// These errors are used to simulate various failure scenarios in tests.
var (
	// ErrMockFileNotFound indicates a mock file was not found (used in tests).
	ErrMockFileNotFound = errors.New("file not found")

	// ErrMockGHFailed indicates a mock gh command failed (used in tests).
	ErrMockGHFailed = errors.New("gh command failed")

	// ErrMockAPIError indicates a mock API error occurred (used in tests).
	ErrMockAPIError = errors.New("API error")

	// ErrMockNotFound indicates a mock resource was not found (used in tests).
	ErrMockNotFound = errors.New("not found")

	// ErrMockNetwork indicates a mock network error occurred (used in tests).
	ErrMockNetwork = errors.New("network error")

	// ErrMockTaskStoreUnavailable indicates a mock task store is unavailable (used in tests).
	ErrMockTaskStoreUnavailable = errors.New("task store unavailable")
)
