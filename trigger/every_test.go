package trigger

import (
	"testing"
	"time"
)

func TestEvery_PreviousAndNextOnTheBoundariesAndBetween(t *testing.T) {
	five := Every(5 * time.Minute)

	expectPrevious(t, five, "2026-10-16 12:03:00", "2026-10-16 12:00 MDT")
	expectPrevious(t, five, "2026-10-16 12:05:00", "2026-10-16 12:05 MDT")
	expectPrevious(t, five, "2026-10-16 12:09:59", "2026-10-16 12:05 MDT")
	expectNext(t, five, "2026-10-16 12:03:00", "2026-10-16 12:05 MDT")
	expectNext(t, five, "2026-10-16 12:05:00", "2026-10-16 12:10 MDT")
	expectNext(t, five, "2026-10-16 12:04:59", "2026-10-16 12:05 MDT")
}

func TestEvery_TheCountBeginsAgainAtMidnight(t *testing.T) {
	five := Every(5 * time.Minute)

	expectPrevious(t, five, "2026-10-16 00:00:00", "2026-10-16 00:00 MDT")
	expectPrevious(t, five, "2026-10-16 00:02:00", "2026-10-16 00:00 MDT")
	expectNext(t, five, "2026-10-16 23:57:00", "2026-10-17 00:00 MDT")
	expectNext(t, five, "2026-10-16 23:55:00", "2026-10-17 00:00 MDT")
	expectNext(t, five, "2026-10-16 23:54:59", "2026-10-16 23:55 MDT")

	// An interval that doesn't divide the day has a short last stretch, and doesn't carry over.
	seven := Every(7 * time.Hour)

	expectPrevious(t, seven, "2026-10-16 22:00:00", "2026-10-16 21:00 MDT")
	expectNext(t, seven, "2026-10-16 21:00:00", "2026-10-17 00:00 MDT")
	expectNext(t, seven, "2026-10-16 08:00:00", "2026-10-16 14:00 MDT")
}

func TestEvery_ADayThatIsAnHourShortOrLongHasTheStretchesTheClockReallyHas(t *testing.T) {
	hourly := Every(time.Hour)

	// 8 March 2026: clocks go forward at 2am, so there is no 2am, and the day is 23 hours.
	expectNext(t, hourly, "2026-03-08 01:00:00", "2026-03-08 03:00 MDT")
	expectPrevious(t, hourly, "2026-03-08 03:30:00", "2026-03-08 03:00 MDT")
	expectNext(t, hourly, "2026-03-08 22:30:00", "2026-03-08 23:00 MDT")
	expectNext(t, hourly, "2026-03-08 23:30:00", "2026-03-09 00:00 MDT")

	// 1 November: clocks go back at 2am, so 1am happens twice and the day is 25 hours. Each hour is due once.
	first := time.Date(2026, 11, 1, 1, 0, 0, 0, denver)
	second := first.Add(time.Hour)

	if second.Format("15:04 MST") != "01:00 MST" {
		t.Fatalf("the test's times are wrong: %v", second)
	}

	if got, ok := hourly.Next(first); !ok || !got.Equal(second) {
		t.Errorf("Next at the first 1am = %v", got)
	}

	if got, ok := hourly.Previous(second.Add(30 * time.Minute)); !ok || !got.Equal(second) {
		t.Errorf("Previous in the second 1am = %v", got)
	}

	expectNext(t, hourly, "2026-11-01 23:30:00", "2026-11-02 00:00 MST")
}

func TestEvery_EveryMinuteAndEveryDay(t *testing.T) {
	expectNext(t, Every(time.Minute), "2026-10-16 12:03:00", "2026-10-16 12:04 MDT")
	expectPrevious(t, Every(time.Minute), "2026-10-16 12:03:30", "2026-10-16 12:03 MDT")

	daily := Every(24 * time.Hour)

	expectPrevious(t, daily, "2026-10-16 23:00:00", "2026-10-16 00:00 MDT")
	expectNext(t, daily, "2026-10-16 00:00:00", "2026-10-17 00:00 MDT")
}

func TestEvery_NeverGoesBackwardsAndIsAlwaysDueAtLeastOnceAnInterval(t *testing.T) {
	for _, interval := range []time.Duration{time.Minute, 5 * time.Minute, 90 * time.Minute, 7 * time.Hour, 24 * time.Hour} {
		every := Every(interval)
		now := local("2026-03-01 00:00:00")
		var last time.Time

		for step := 0; step < 24*60/7*61; step++ {
			now = now.Add(7 * time.Minute)

			previous, ok := every.Previous(now)
			if !ok || previous.After(now) || previous.Before(last) {
				t.Fatalf("%v at %v: previous %v (last %v)", interval, now, previous, last)
			}

			next, ok := every.Next(now)
			if !ok || !next.After(now) || next.Sub(previous) > interval+time.Hour {
				t.Fatalf("%v at %v: next %v after previous %v", interval, now, next, previous)
			}

			last = previous
		}
	}
}

func TestEvery_Description(t *testing.T) {
	for interval, want := range map[time.Duration]string{
		time.Minute: "every minute", 5 * time.Minute: "every 5 minutes", 90 * time.Minute: "every 1 hour 30 minutes",
		time.Hour: "every hour", 2 * time.Hour: "every 2 hours",
	} {
		if got := Every(interval).Description(); got != want {
			t.Errorf("%v: got %q, want %q", interval, got, want)
		}
	}
}

func TestEvery_RejectsAnIntervalItCanNotKeep(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Minute, 30 * time.Second, 90 * time.Second, 25 * time.Hour} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Every(%v) didn't panic", interval)
				}
			}()

			Every(interval)
		}()
	}
}
