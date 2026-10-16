package trigger

import (
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/avanha/pmaas-automations/sun"
)

var (
	denver    = mustLoad("America/Denver")
	lafayette = sun.Location{Latitude: 39.9967, Longitude: -105.0944}
)

func mustLoad(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}

	return location
}

func local(value string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", value, denver)
	if err != nil {
		panic(err)
	}

	return t
}

func format(t time.Time, ok bool) string {
	if !ok {
		return "none"
	}

	return t.In(denver).Format("2006-01-02 15:04 MST")
}

func expectPrevious(t *testing.T, trigger Trigger, from string, want string) {
	t.Helper()

	if got := format(trigger.Previous(local(from))); got != want {
		t.Errorf("Previous(%s) = %s, want %s", from, got, want)
	}
}

func expectNext(t *testing.T, trigger Trigger, from string, want string) {
	t.Helper()

	if got := format(trigger.Next(local(from))); got != want {
		t.Errorf("Next(%s) = %s, want %s", from, got, want)
	}
}

func TestAtTimeOfDay_PreviousAndNext(t *testing.T) {
	five := AtTimeOfDay(17, 0)

	expectPrevious(t, five, "2026-10-16 18:00:00", "2026-10-16 17:00 MDT")
	expectPrevious(t, five, "2026-10-16 16:59:59", "2026-10-15 17:00 MDT")
	expectNext(t, five, "2026-10-16 16:59:59", "2026-10-16 17:00 MDT")
	expectNext(t, five, "2026-10-16 17:00:01", "2026-10-17 17:00 MDT")
}

func TestAtTimeOfDay_DueAtExactlyThatTimeIsDueNowButNotNext(t *testing.T) {
	five := AtTimeOfDay(17, 0)

	expectPrevious(t, five, "2026-10-16 17:00:00", "2026-10-16 17:00 MDT")
	expectNext(t, five, "2026-10-16 17:00:00", "2026-10-17 17:00 MDT")
}

func TestAtTimeOfDay_JustAfterMidnight(t *testing.T) {
	late := AtTimeOfDay(0, 30)

	expectPrevious(t, late, "2026-10-16 00:29:00", "2026-10-15 00:30 MDT")
	expectPrevious(t, late, "2026-10-16 00:30:00", "2026-10-16 00:30 MDT")
	expectNext(t, late, "2026-10-16 23:59:00", "2026-10-17 00:30 MDT")
}

func TestAtTimeOfDay_AcrossDaylightSavingTime(t *testing.T) {
	// Clocks go forward at 2am on 8 March 2026 and back at 2am on 1 November.
	evening := AtTimeOfDay(17, 0)

	expectNext(t, evening, "2026-03-07 18:00:00", "2026-03-08 17:00 MDT")
	expectPrevious(t, evening, "2026-03-08 18:00:00", "2026-03-08 17:00 MDT")
	expectPrevious(t, evening, "2026-03-08 03:00:00", "2026-03-07 17:00 MST")
	expectNext(t, evening, "2026-10-31 18:00:00", "2026-11-01 17:00 MST")
	expectNext(t, evening, "2026-11-01 17:00:01", "2026-11-02 17:00 MST")

	// The night the clocks go back, 1:30 am happens twice; it's due once, at the first.
	small := AtTimeOfDay(1, 30)
	first := time.Date(2026, 11, 1, 1, 30, 0, 0, denver)

	if first.Format("MST") != "MDT" {
		t.Fatalf("expected the first 1:30 to be daylight saving time, got %s", first.Format("MST"))
	}

	if got, ok := small.Next(first.Add(-time.Minute)); !ok || !got.Equal(first) {
		t.Errorf("Next before the first 1:30 = %v, %v", got, ok)
	}

	if got, ok := small.Next(first); !ok || got.Day() != 2 {
		t.Errorf("Next at the first 1:30 = %v, %v: it must not be due again an hour later", got, ok)
	}
}

func TestAtTimeOfDay_DescriptionAndValidation(t *testing.T) {
	if got := AtTimeOfDay(5, 7).Description(); got != "at 05:07" {
		t.Errorf("got %q", got)
	}

	for _, c := range [][2]int{{24, 0}, {-1, 0}, {0, 60}, {12, -1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("AtTimeOfDay(%d, %d) didn't panic", c[0], c[1])
				}
			}()

			AtTimeOfDay(c[0], c[1])
		}()
	}
}

func TestSun_SunsetInLafayette(t *testing.T) {
	sunset := Sun(lafayette, sun.Sunset, 0)

	// Sunset on 16 October is 6:19pm daylight saving time, by the Naval Observatory's tables.
	expectPrevious(t, sunset, "2026-10-16 20:00:00", "2026-10-16 18:19 MDT")
	expectPrevious(t, sunset, "2026-10-16 12:00:00", "2026-10-15 18:21 MDT")
	expectNext(t, sunset, "2026-10-16 12:00:00", "2026-10-16 18:19 MDT")
	expectNext(t, sunset, "2026-10-16 20:00:00", "2026-10-17 18:18 MDT")
}

func TestSun_OffsetsBeforeAndAfter(t *testing.T) {
	before := Sun(lafayette, sun.Sunset, -15*time.Minute)
	after := Sun(lafayette, sun.Sunset, 90*time.Minute)

	expectNext(t, before, "2026-10-16 12:00:00", "2026-10-16 18:04 MDT")
	expectNext(t, after, "2026-10-16 12:00:00", "2026-10-16 19:49 MDT")
}

func TestSun_AnOffsetPastMidnightBelongsToTheDayItWasFor(t *testing.T) {
	// 7 hours after the 16th's sunset (6:19pm) is the 17th at 1:19am. Asked at midnight on the 17th, that has not
	// happened yet, so the latest it was due is the 15th's sunset (6:21pm) plus that: the 16th at 1:21am.
	late := Sun(lafayette, sun.Sunset, 7*time.Hour)

	expectPrevious(t, late, "2026-10-17 00:00:00", "2026-10-16 01:21 MDT")
	expectNext(t, late, "2026-10-17 00:00:00", "2026-10-17 01:19 MDT")
	expectPrevious(t, late, "2026-10-17 01:19:00", "2026-10-17 01:19 MDT")
}

func TestSun_AnOffsetBeforeMidnightBelongsToTheDayItWasFor(t *testing.T) {
	// 7 hours before the 16th's sunrise (6:12 am standard time, 7:12 daylight) is the 16th at 0:12.
	early := Sun(lafayette, sun.Sunrise, -7*time.Hour)

	expectNext(t, early, "2026-10-15 23:00:00", "2026-10-16 00:12 MDT")
	expectPrevious(t, early, "2026-10-16 00:12:00", "2026-10-16 00:12 MDT")
}

func TestSun_AnOffsetOfSeveralDaysStillFindsTheDay(t *testing.T) {
	lag := Sun(lafayette, sun.Sunset, 100*time.Hour)

	// The 12th's sunset plus 100 hours is the 16th at about 22:20.
	got, ok := lag.Previous(local("2026-10-16 23:00:00"))
	if !ok {
		t.Fatal("none")
	}

	if got.In(denver).Format("2006-01-02") != "2026-10-16" || got.In(denver).Hour() != 22 {
		t.Errorf("got %s", format(got, ok))
	}

	next, ok := lag.Next(local("2026-10-16 23:00:00"))
	if !ok || next.In(denver).Format("2006-01-02") != "2026-10-17" {
		t.Errorf("next: %s", format(next, ok))
	}
}

func TestSun_EachMinuteIsDueAtMostOncePerDayOverAYear(t *testing.T) {
	// Walk a year of minutes-of-interest: Previous must never go backwards as the time moves forward, and Next must
	// always be after the time asked about.
	sunset := Sun(lafayette, sun.Sunset, -30*time.Minute)
	t0 := local("2026-01-01 00:00:00")
	var last time.Time

	for step := 0; step < 365*8; step++ {
		now := t0.Add(time.Duration(step) * 3 * time.Hour)

		previous, ok := sunset.Previous(now)
		if !ok || previous.After(now) || previous.Before(last) {
			t.Fatalf("at %v: previous %v (last %v)", now, previous, last)
		}

		next, ok := sunset.Next(now)
		if !ok || !next.After(now) || next.Sub(previous) < 23*time.Hour || next.Sub(previous) > 25*time.Hour {
			t.Fatalf("at %v: next %v after previous %v", now, next, previous)
		}

		last = previous
	}
}

func TestSun_NoSunsetInTheArcticSummerFindsTheNextOneWhenThereIsOne(t *testing.T) {
	tromso := sun.Location{Latitude: 69.6492, Longitude: 18.9553}
	oslo := time.FixedZone("CEST", 2*3600)
	sunset := Sun(tromso, sun.Sunset, 0)

	midsummer := time.Date(2026, 6, 21, 12, 0, 0, 0, oslo)

	next, ok := sunset.Next(midsummer)
	if !ok || next.Month() != time.July || next.Day() < 15 || next.Day() > 31 {
		t.Errorf("next sunset after midsummer in Tromsø: %v, %v", next, ok)
	}

	previous, ok := sunset.Previous(midsummer)
	if !ok || previous.Month() != time.May {
		t.Errorf("previous sunset before midsummer in Tromsø: %v, %v", previous, ok)
	}
}

func TestDaily_NeverDueMeansNone(t *testing.T) {
	never := &daily{occurrence: func(time.Time) (time.Time, bool) { return time.Time{}, false }}
	now := local("2026-10-16 12:00:00")

	if got, ok := never.Next(now); ok {
		t.Errorf("Next = %v", got)
	}

	if got, ok := never.Previous(now); ok {
		t.Errorf("Previous = %v", got)
	}
}

func TestSun_Description(t *testing.T) {
	for _, c := range []struct {
		trigger Trigger
		want    string
	}{
		{Sun(lafayette, sun.Sunset, 0), "at sunset"},
		{Sun(lafayette, sun.Sunset, -15*time.Minute), "15 minutes before sunset"},
		{Sun(lafayette, sun.Sunrise, time.Minute), "1 minute after sunrise"},
		{Sun(lafayette, sun.CivilDusk, 90*time.Minute), "1 hour 30 minutes after civil dusk"},
		{Sun(lafayette, sun.AstronomicalDawn, -2*time.Hour), "2 hours before astronomical dawn"},
	} {
		if got := c.trigger.Description(); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestSun_RejectsAnEventThatIsNotOne(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("didn't panic")
		}
	}()

	Sun(lafayette, sun.Event(42), 0)
}
