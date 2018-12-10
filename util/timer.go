package util

import "time"

// Timer represents an event that fires with time passage.
// See time.Timer type for intuition on how it works.
type Timer interface {
	// Chan returns a channel which is used to deliver the event.
	Chan() <-chan time.Time
	// Stop prevents the Timer from firing. Returns false if the event has
	// already fired, or the Timer has been stopped.
	Stop() bool
}

// systemTimer is a Timer that uses system time.
type systemTimer struct {
	*time.Timer
}

func (t systemTimer) Chan() <-chan time.Time {
	return t.C
}

// fakeTimer implements Timer interface for testing. Event firing is controlled
// by the FakeTimeSource which creates and owns fakeTimer instances.
type fakeTimer struct {
	ts   *FakeTimeSource
	id   int
	when time.Time
	ch   chan time.Time
	done bool
}

func newFakeTimer(ts *FakeTimeSource, id int, when time.Time) *fakeTimer {
	ch := make(chan time.Time, 1)
	return &fakeTimer{ts: ts, id: id, when: when, ch: ch}
}

func (t *fakeTimer) Chan() <-chan time.Time {
	return t.ch
}

func (t *fakeTimer) Stop() bool {
	return t.ts.unsubscribe(t.id)
}

func (t *fakeTimer) tryFire(now time.Time) bool {
	if t.when.Before(now) || t.when.Equal(now) {
		select {
		case t.ch <- now:
			return true
		default:
		}
	}
	return false
}
