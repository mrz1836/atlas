package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_Now(t *testing.T) {
	c := RealClock{}

	before := time.Now()
	got := c.Now()
	after := time.Now()

	assert.False(t, got.Before(before), "clock.Now() should not return time before actual time.Now()")
	assert.False(t, got.After(after), "clock.Now() should not return time after actual time.Now()")
}

// MockClock is a Clock implementation for testing that returns a fixed time.
type MockClock struct {
	FixedTime time.Time
}

// Now returns the fixed time.
func (m MockClock) Now() time.Time {
	return m.FixedTime
}

func TestMockClock_Now(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	c := MockClock{FixedTime: fixedTime}

	assert.Equal(t, fixedTime, c.Now())

	// Multiple calls return the same time
	assert.Equal(t, fixedTime, c.Now())
	assert.Equal(t, fixedTime, c.Now())
}
