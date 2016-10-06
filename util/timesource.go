package util

import (
	"time"
)

// TimeSource can provide the current time, or be replaced by a mock in tests to return
// specific values.
type TimeSource interface {
	// Now returns the current time in real implementations or a suitable value in others
	Now() time.Time
}

// SystemTimeSource provides the current system local time
type SystemTimeSource struct{}

func (s SystemTimeSource) Now() time.Time {
	// Now returns the current local time
	return time.Now()
}

// FakeTimeSource provides a time that can be any arbitrarily set value for use in tests.
// It should not be used in production code.
type FakeTimeSource struct {
	// FakeTime is the value that this fake time source will return. It is public so
	// tests can manipulate it.
	FakeTime time.Time
}

// Now returns the time value this instance contains
func (f FakeTimeSource) Now() time.Time {
	return f.FakeTime
}

// IncrementingFakeTimeSource takes a base time and several increments, which will be applied to
// the base time each time Now() is called. The first call will return the base time + zeroth
// increment. If called more times than provided for then it will panic. Does not require that
// increments increase monotonically.
type IncrementingFakeTimeSource struct {
	BaseTime      time.Time
	Increments    []time.Duration
	NextIncrement int
}

func (a *IncrementingFakeTimeSource) Now() time.Time {
	adjustedTime := a.BaseTime.Add(a.Increments[a.NextIncrement])
	a.NextIncrement++

	return adjustedTime
}
