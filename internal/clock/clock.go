// Package clock provides an abstraction for time operations to improve testability.
// Instead of calling time.Now() directly, code can use the Clock interface which
// can be mocked in tests to control time-dependent behavior.
package clock

import "time"

// Clock is an interface for time operations.
// This allows code to be tested with mock clocks.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
}

// RealClock implements Clock using the actual system time.
type RealClock struct{}

// Now returns the current time from the system clock.
func (RealClock) Now() time.Time {
	return time.Now()
}

// Ensure RealClock implements Clock.
var _ Clock = RealClock{}
