package testutil

import (
	"errors"
	"testing"
)

// errMockWrapped is a static error for testing that non-wrapped errors don't match sentinels.
var errMockWrapped = errors.New("wrapped: network error")

func TestMockErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrMockFileNotFound", ErrMockFileNotFound, "file not found"},
		{"ErrMockGHFailed", ErrMockGHFailed, "gh command failed"},
		{"ErrMockAPIError", ErrMockAPIError, "API error"},
		{"ErrMockNotFound", ErrMockNotFound, "not found"},
		{"ErrMockNetwork", ErrMockNetwork, "network error"},
		{"ErrMockTaskStoreUnavailable", ErrMockTaskStoreUnavailable, "task store unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.want {
				t.Errorf("%s.Error() = %q, want %q", tt.name, tt.err.Error(), tt.want)
			}
		})
	}
}

func TestMockErrorsAreSentinelErrors(t *testing.T) {
	// Verify mock errors work with errors.Is
	// Direct comparison should work
	if !errors.Is(ErrMockNetwork, ErrMockNetwork) {
		t.Error("ErrMockNetwork should be equal to itself")
	}

	// Non-wrapped errors should not match (standard Go error behavior)
	if errors.Is(errMockWrapped, ErrMockNetwork) {
		t.Error("non-wrapped error should not match sentinel")
	}
}
