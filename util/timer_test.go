package util

import (
	"fmt"
	"testing"
	"time"
)

func checkNotFiring(t *testing.T, timer Timer) {
	t.Helper()
	select {
	case time := <-timer.Chan():
		t.Errorf("Timer unexpectedly fired at %v", time)
	case <-time.After(10 * time.Millisecond): // Give some real time to pass.
	}
}

func TestFakeTimerFiresOnce(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 00, 00, 00, time.UTC)
	ts := NewFakeTimeSource(base)
	timer := ts.NewTimer(10 * time.Millisecond)

	checkNotFiring(t, timer)
	newTime := base.Add(9 * time.Millisecond)
	ts.Set(newTime)
	checkNotFiring(t, timer)

	newTime = newTime.Add(1 * time.Millisecond)
	ts.Set(newTime)
	got := <-timer.Chan() // Now it should fire.
	if !got.Equal(newTime) {
		t.Errorf("Timer fired at %v, want %v", got, newTime)
	}

	checkNotFiring(t, timer) // Shouldn't fire any more.
}

func TestFakeTimerStopBeforeFire(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 00, 00, 00, time.UTC)
	ts := NewFakeTimeSource(base)
	timer := ts.NewTimer(10 * time.Millisecond)

	checkNotFiring(t, timer)
	if !timer.Stop() {
		t.Error("Stop() returns true, want false")
	}

	newTime := base.Add(20 * time.Millisecond)
	ts.Set(newTime)
	checkNotFiring(t, timer) // Shouldn't fire because it was stopped.
}

func TestFakeTimerStopAfterFire(t *testing.T) {
	for _, drain := range []bool{false, true} {
		t.Run(fmt.Sprintf("drain:%v", drain), func(t *testing.T) {
			base := time.Date(2018, 12, 12, 18, 00, 00, 00, time.UTC)
			ts := NewFakeTimeSource(base)
			timer := ts.NewTimer(10 * time.Millisecond)
			ts.Set(base.Add(20 * time.Millisecond)) // Triggers the event.
			if drain {
				<-timer.Chan()
			}
			if timer.Stop() {
				t.Error("Stop() returns false, want true")
			}
		})
	}
}

func TestManyFakeTimers(t *testing.T) {
	base := time.Date(2018, 12, 12, 18, 00, 00, 00, time.UTC)
	ts := NewFakeTimeSource(base)
	var timers []Timer
	var times []time.Time
	for i := 1; i <= 10; i++ {
		d := time.Duration(i) * time.Second
		timers = append(timers, ts.NewTimer(d))
		times = append(times, base.Add(d))
	}
	check := func(fire int, want time.Time) {
		for i, timer := range timers {
			if i != fire {
				checkNotFiring(t, timer)
				continue
			}
			got := <-timer.Chan()
			if !got.Equal(want) {
				t.Errorf("Timer %d fired at %v, want %v", i, got, want)
			}
		}
	}
	check(-1, time.Time{}) // No firing before.
	for i, time := range times {
		ts.Set(time)
		check(i, time)
	}
	check(-1, time.Time{}) // No firing after.
}
