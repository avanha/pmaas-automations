package trigger

import (
	"testing"
	"time"

	"github.com/avanha/pmaas-automations/sun"
)

func daytime() Trigger {
	return Windowed(Every(5*time.Minute), AtTimeOfDay(8, 0), AtTimeOfDay(20, 0))
}

func TestWindowed_DueOnlyWhileTheWindowIsOpen(t *testing.T) {
	w := daytime()

	// Inside.
	expectPrevious(t, w, "2026-10-16 12:03:00", "2026-10-16 12:00 MDT")
	expectNext(t, w, "2026-10-16 12:03:00", "2026-10-16 12:05 MDT")

	// The first time it's due is the moment the window opens, and the last is the slot before it closes.
	expectNext(t, w, "2026-10-16 07:59:00", "2026-10-16 08:00 MDT")
	expectPrevious(t, w, "2026-10-16 08:00:00", "2026-10-16 08:00 MDT")
	expectPrevious(t, w, "2026-10-16 07:59:00", "2026-10-15 19:55 MDT")
	expectNext(t, w, "2026-10-16 19:55:00", "2026-10-17 08:00 MDT")
	expectPrevious(t, w, "2026-10-16 20:00:00", "2026-10-16 19:55 MDT")
	expectPrevious(t, w, "2026-10-16 23:00:00", "2026-10-16 19:55 MDT")

	// Not at all at night.
	expectNext(t, w, "2026-10-16 21:00:00", "2026-10-17 08:00 MDT")
	expectNext(t, w, "2026-10-16 03:00:00", "2026-10-16 08:00 MDT")
	expectPrevious(t, w, "2026-10-16 03:00:00", "2026-10-15 19:55 MDT")
}

func TestWindowed_TheWindowIsOpenFromTheMomentItOpensAndClosedFromTheMomentItCloses(t *testing.T) {
	// Every minute, so a time exactly at each end is a time it could be due.
	w := Windowed(Every(time.Minute), AtTimeOfDay(8, 0), AtTimeOfDay(20, 0))

	expectNext(t, w, "2026-10-16 07:59:00", "2026-10-16 08:00 MDT")
	expectNext(t, w, "2026-10-16 19:58:00", "2026-10-16 19:59 MDT")
	expectNext(t, w, "2026-10-16 19:59:00", "2026-10-17 08:00 MDT") // 20:00 is closed
	expectPrevious(t, w, "2026-10-16 20:00:00", "2026-10-16 19:59 MDT")
	expectPrevious(t, w, "2026-10-16 08:00:00", "2026-10-16 08:00 MDT")
}

func TestWindowed_AWindowCanCrossMidnight(t *testing.T) {
	night := Windowed(Every(time.Hour), AtTimeOfDay(22, 0), AtTimeOfDay(6, 0))

	expectNext(t, night, "2026-10-16 12:00:00", "2026-10-16 22:00 MDT")
	expectNext(t, night, "2026-10-16 22:00:00", "2026-10-16 23:00 MDT")
	expectNext(t, night, "2026-10-16 23:00:00", "2026-10-17 00:00 MDT")
	expectNext(t, night, "2026-10-17 04:00:00", "2026-10-17 05:00 MDT")
	expectNext(t, night, "2026-10-17 05:00:00", "2026-10-17 22:00 MDT") // 06:00 is closed
	expectPrevious(t, night, "2026-10-17 12:00:00", "2026-10-17 05:00 MDT")
	expectPrevious(t, night, "2026-10-17 02:30:00", "2026-10-17 02:00 MDT")
}

func TestWindowed_AWindowThatEndsAtSunsetPlusAnHourFollowsTheSun(t *testing.T) {
	w := Windowed(Every(time.Minute), AtTimeOfDay(8, 0), Sun(lafayette, sun.Sunset, time.Hour))

	// 16 October: sunset 6:19pm, so the window closes at 7:19pm and the last minute it's due is 7:18.
	expectPrevious(t, w, "2026-10-16 23:00:00", "2026-10-16 19:18 MDT")
	expectNext(t, w, "2026-10-16 19:18:00", "2026-10-17 08:00 MDT")

	// 21 December: sunset 4:38pm standard time, so it closes at 5:38.
	expectPrevious(t, w, "2026-12-21 23:00:00", "2026-12-21 17:37 MST")
}

func TestWindowed_AWindowShorterThanTheGapsInWhatItWrapsIsNeverDue(t *testing.T) {
	never := Windowed(Every(6*time.Hour), AtTimeOfDay(8, 0), AtTimeOfDay(9, 0))

	if got, ok := never.Next(local("2026-10-16 12:00:00")); ok {
		t.Errorf("Next = %v", got)
	}

	if got, ok := never.Previous(local("2026-10-16 12:00:00")); ok {
		t.Errorf("Previous = %v", got)
	}
}

func TestWindowed_AWindowThatNeverOpensIsNeverDue(t *testing.T) {
	arctic := sun.Location{Latitude: 69.6492, Longitude: 18.9553}
	oslo := time.FixedZone("CEST", 2*3600)
	midsummer := time.Date(2026, 6, 21, 12, 0, 0, 0, oslo)

	// There's no sunset that day to close the window, so it's open from the morning on, and due.
	open := Windowed(Every(time.Hour), AtTimeOfDay(8, 0), Sun(arctic, sun.Sunset, 0))

	if got, ok := open.Previous(midsummer); !ok || got.Hour() != 12 {
		t.Errorf("a window with no closing: %v, %v", got, ok)
	}
}

func TestWindowed_ADayWhenTheClocksChange(t *testing.T) {
	w := Windowed(Every(time.Hour), AtTimeOfDay(0, 30), AtTimeOfDay(5, 0))

	// 8 March: 2am doesn't exist. The hours in the window are 1:00 MST, then 3:00 and 4:00 MDT, and 5:00 is closed.
	expectNext(t, w, "2026-03-08 00:00:00", "2026-03-08 01:00 MST")
	expectNext(t, w, "2026-03-08 01:00:00", "2026-03-08 03:00 MDT")
	expectNext(t, w, "2026-03-08 03:00:00", "2026-03-08 04:00 MDT")
	expectNext(t, w, "2026-03-08 04:00:00", "2026-03-09 01:00 MDT")
	expectPrevious(t, w, "2026-03-08 12:00:00", "2026-03-08 04:00 MDT")
}

func TestWindowed_NeverGoesBackwardsAndOnlyEverFallsInsideTheWindow(t *testing.T) {
	w := Windowed(Every(5*time.Minute), AtTimeOfDay(8, 0), Sun(lafayette, sun.Sunset, time.Hour)).(*windowed)

	now := local("2026-01-01 00:00:00")
	var last time.Time

	for step := 0; step < 365*24*60/37; step++ {
		now = now.Add(37 * time.Minute)

		previous, ok := w.Previous(now)
		if !ok || previous.After(now) || previous.Before(last) || !w.inWindow(previous) || previous.Sub(now) < -24*time.Hour {
			t.Fatalf("at %v: previous %v (last %v)", now, previous, last)
		}

		next, ok := w.Next(now)
		if !ok || !next.After(now) || !w.inWindow(next) || next.Sub(now) > 24*time.Hour {
			t.Fatalf("at %v: next %v", now, next)
		}

		// Nothing is due between the two.
		if again, _ := w.Next(previous); !again.Equal(next) && again.After(now) {
			t.Fatalf("at %v: next after previous %v is %v, not %v", now, previous, again, next)
		}

		last = previous
	}
}

func TestWindowed_Description(t *testing.T) {
	got := Windowed(Every(time.Minute), AtTimeOfDay(8, 0), Sun(lafayette, sun.Sunset, time.Hour)).Description()

	if got != "every minute, from 08:00 until 1 hour after sunset" {
		t.Errorf("got %q", got)
	}

	got = Windowed(Every(5*time.Minute), Sun(lafayette, sun.Sunrise, 0), Sun(lafayette, sun.Sunset, 0)).Description()

	if got != "every 5 minutes, from sunrise until sunset" {
		t.Errorf("got %q", got)
	}
}

func TestWindowed_PanicsForATriggerThatIsNotThere(t *testing.T) {
	for name, make := range map[string]func(){
		"inner": func() { Windowed(nil, AtTimeOfDay(1, 0), AtTimeOfDay(2, 0)) },
		"open":  func() { Windowed(Every(time.Minute), nil, AtTimeOfDay(2, 0)) },
		"close": func() { Windowed(Every(time.Minute), AtTimeOfDay(1, 0), nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: didn't panic", name)
				}
			}()

			make()
		}()
	}
}

func TestWindowed_AWindowThatOpensAndClosesAtTheSameMomentIsNeverOpen(t *testing.T) {
	never := Windowed(Every(time.Minute), AtTimeOfDay(8, 0), AtTimeOfDay(8, 0))
	now := local("2026-10-16 12:00:00")

	if got, ok := never.Previous(now); ok {
		t.Errorf("Previous = %v", got)
	}

	if got, ok := never.Next(now); ok {
		t.Errorf("Next = %v", got)
	}
}

func TestWindowed_WithNothingToOpenOrCloseItIsNeverDue(t *testing.T) {
	nothing := &daily{occurrence: func(time.Time) (time.Time, bool) { return time.Time{}, false }}
	now := local("2026-10-16 12:00:00")

	for name, w := range map[string]Trigger{
		"neither opens nor closes":  Windowed(Every(time.Minute), nothing, nothing),
		"never opens":               Windowed(Every(time.Minute), nothing, AtTimeOfDay(20, 0)),
		"never closes, never opens": Windowed(Every(time.Minute), nothing, nothing),
	} {
		if got, ok := w.Previous(now); ok {
			t.Errorf("%s: Previous = %v", name, got)
		}

		if got, ok := w.Next(now); ok {
			t.Errorf("%s: Next = %v", name, got)
		}
	}
}

func TestWindowed_AWindowThatOpensAndNeverClosesIsOpenFromThenOn(t *testing.T) {
	nothing := &daily{occurrence: func(time.Time) (time.Time, bool) { return time.Time{}, false }}
	always := Windowed(Every(time.Hour), AtTimeOfDay(8, 0), nothing)

	expectPrevious(t, always, "2026-10-16 23:30:00", "2026-10-16 23:00 MDT")
	expectNext(t, always, "2026-10-16 05:30:00", "2026-10-16 06:00 MDT")
}
