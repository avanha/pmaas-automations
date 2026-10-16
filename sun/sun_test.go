package sun

import (
	"fmt"
	"testing"
	"time"
)

// The times below are from the US Naval Observatory's sun and moon data service
// (aa.usno.navy.mil/api/rstt/oneday), which publishes them to the minute. They're in the zone's standard time, with
// no daylight saving, so that what a time is doesn't depend on what the zone's rules were that day.
//
// Lafayette, Colorado is where the lights are.
var (
	lafayette = Location{Latitude: 39.9967, Longitude: -105.0944}
	mst       = time.FixedZone("MST", -7*3600)
)

type published struct {
	date                                  string
	civilDawn, sunrise, sunset, civilDusk string
}

var lafayetteYear = []published{
	{"2026-01-15", "06:51", "07:20", "17:00", "17:30"},
	{"2026-02-15", "06:25", "06:53", "17:36", "18:04"},
	{"2026-03-08", "05:56", "06:23", "18:00", "18:27"}, // the day daylight saving time begins
	{"2026-03-20", "05:37", "06:04", "18:13", "18:40"},
	{"2026-04-15", "04:54", "05:22", "18:39", "19:07"},
	{"2026-05-15", "04:14", "04:45", "19:09", "19:40"},
	{"2026-06-21", "03:59", "04:32", "19:33", "20:06"}, // the longest day
	{"2026-07-15", "04:12", "04:44", "19:28", "20:00"},
	{"2026-08-15", "04:43", "05:12", "18:57", "19:26"},
	{"2026-09-22", "05:21", "05:48", "17:57", "18:24"},
	{"2026-10-16", "05:45", "06:12", "17:19", "17:46"},
	{"2026-11-01", "06:02", "06:30", "16:58", "17:26"}, // the day it ends
	{"2026-12-21", "06:48", "07:19", "16:38", "17:09"}, // the shortest
	{"2027-03-14", "05:47", "06:14", "18:06", "18:33"},
}

// at is the time of day on a date, in zone.
func at(zone *time.Location, date string, clock string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, zone)
	if err != nil {
		panic(err)
	}

	return t
}

// checkWithinAMinute is that the time we worked out is the published one, to the minute: both have been rounded, so
// they can differ by one when the true time is close to a boundary between two.
func checkWithinAMinute(t *testing.T, what string, got time.Time, ok bool, want time.Time) {
	t.Helper()

	if !ok {
		t.Errorf("%s: no time, want %s", what, want.Format("15:04"))
		return
	}

	rounded := got.Round(time.Minute)
	if diff := rounded.Sub(want); diff < -time.Minute || diff > time.Minute {
		t.Errorf("%s: got %s (%s), want %s", what, rounded.In(want.Location()).Format("15:04"), got.In(want.Location()).Format("15:04:05"), want.Format("15:04"))
	}
}

func TestLocation_MatchesThePublishedTablesForALafayetteYear(t *testing.T) {
	for _, day := range lafayetteYear {
		noon := at(mst, day.date, "12:00")

		for _, c := range []struct {
			event Event
			want  string
		}{
			{CivilDawn, day.civilDawn}, {Sunrise, day.sunrise}, {Sunset, day.sunset}, {CivilDusk, day.civilDusk},
		} {
			got, ok := lafayette.Time(c.event, noon)
			checkWithinAMinute(t, fmt.Sprintf("%s %s", day.date, c.event), got, ok, at(mst, day.date, c.want))
		}
	}
}

func TestLocation_TheSameInstantWhateverZoneTheDayIsGivenIn(t *testing.T) {
	// 15 October at noon in Colorado is the same day in daylight saving time as it is in standard time.
	denver, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}

	inDenver, _ := lafayette.Time(Sunset, time.Date(2026, 10, 16, 12, 0, 0, 0, denver))
	inMST, _ := lafayette.Time(Sunset, time.Date(2026, 10, 16, 12, 0, 0, 0, mst))
	inUTC, _ := lafayette.Time(Sunset, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))

	if !inDenver.Equal(inMST) || !inDenver.Equal(inUTC) {
		t.Errorf("Denver %v, MST %v, UTC %v", inDenver, inMST, inUTC)
	}

	// And it's in the zone that was asked for, which in October is daylight saving time: 18:19, not 17:19.
	if inDenver.Location() != denver || inDenver.Format("15:04") != "18:19" {
		t.Errorf("got %s", inDenver.Format("15:04 MST"))
	}
}

func TestLocation_TheTimeOfDayDoesntMatter(t *testing.T) {
	early, _ := lafayette.Time(Sunset, time.Date(2026, 10, 16, 0, 0, 0, 0, mst))
	late, _ := lafayette.Time(Sunset, time.Date(2026, 10, 16, 23, 59, 59, 0, mst))

	if !early.Equal(late) {
		t.Errorf("%v, %v", early, late)
	}
}

func TestLocation_EachDaysTwilightsAndSunEventsComeInOrder(t *testing.T) {
	for _, day := range lafayetteYear {
		noon := at(mst, day.date, "12:00")
		previous := time.Time{}

		for _, event := range []Event{
			AstronomicalDawn, NauticalDawn, CivilDawn, Sunrise, Sunset, CivilDusk, NauticalDusk, AstronomicalDusk,
		} {
			got, ok := lafayette.Time(event, noon)
			if !ok {
				t.Fatalf("%s %s: no time", day.date, event)
			}

			if !got.After(previous) {
				t.Errorf("%s: %s at %s isn't after the one before at %s", day.date, event, got.Format("15:04:05"), previous.Format("15:04:05"))
			}

			if got.Sub(noon) > 13*time.Hour || noon.Sub(got) > 13*time.Hour {
				t.Errorf("%s %s: %s isn't within the day", day.date, event, got.Format("2006-01-02 15:04"))
			}

			previous = got
		}
	}
}

func TestLocation_TheGapBetweenTwilightsIsRightForThatLatitude(t *testing.T) {
	// On the March equinox at 40 degrees, civil twilight lasts about 27 minutes, nautical adds about 33 and
	// astronomical adds about 36 (it varies by minutes with the latitude).
	noon := at(mst, "2026-03-20", "12:00")
	sunrise, _ := lafayette.Time(Sunrise, noon)
	civil, _ := lafayette.Time(CivilDawn, noon)
	nautical, _ := lafayette.Time(NauticalDawn, noon)
	astronomical, _ := lafayette.Time(AstronomicalDawn, noon)

	for name, c := range map[string]struct {
		got, min, max time.Duration
	}{
		"sunrise-civil":         {sunrise.Sub(civil), 24 * time.Minute, 30 * time.Minute},
		"civil-nautical":        {civil.Sub(nautical), 28 * time.Minute, 38 * time.Minute},
		"nautical-astronomical": {nautical.Sub(astronomical), 30 * time.Minute, 42 * time.Minute},
	} {
		if c.got < c.min || c.got > c.max {
			t.Errorf("%s: %v, want between %v and %v", name, c.got, c.min, c.max)
		}
	}
}

// otherPlaces checks the parts of the arithmetic that Colorado never exercises: the southern hemisphere, east
// longitudes, where a calendar day's noon falls on the UTC day before, a place on the prime meridian, and one with
// a long polar night.
func TestLocation_OtherPlaces(t *testing.T) {
	for _, c := range []struct {
		name     string
		location Location
		zone     *time.Location
		date     string
		published
	}{
		{"Sydney, December", Location{-33.8688, 151.2093}, time.FixedZone("AEST", 10*3600), "2026-12-21",
			published{"", "04:11", "04:41", "19:05", "19:35"}},
		{"Sydney, June", Location{-33.8688, 151.2093}, time.FixedZone("AEST", 10*3600), "2026-06-21",
			published{"", "06:32", "07:00", "16:54", "17:22"}},
		{"Tokyo, March", Location{35.6895, 139.6917}, time.FixedZone("JST", 9*3600), "2026-03-20",
			published{"", "05:20", "05:46", "17:53", "18:18"}},
		{"Tokyo, September", Location{35.6895, 139.6917}, time.FixedZone("JST", 9*3600), "2026-09-23",
			published{"", "05:04", "05:30", "17:37", "18:03"}},
		{"London, June", Location{51.5074, -0.1278}, time.UTC, "2026-06-21",
			published{"", "02:55", "03:43", "20:22", "21:09"}},
		// A zone that is a long way from its longitude: Kiritimati is a day ahead of where the sun says, so its noon
		// falls on the UTC day before its calendar day.
		{"Kiritimati, March", Location{1.8721, -157.4278}, time.FixedZone("LINT", 14*3600), "2026-03-20",
			published{"", "06:13", "06:34", "18:41", "19:01"}},
		{"Kiritimati, September", Location{1.8721, -157.4278}, time.FixedZone("LINT", 14*3600), "2026-09-23",
			published{"", "05:58", "06:19", "18:26", "18:46"}},
		{"Apia, December", Location{-13.8333, -171.75}, time.FixedZone("WSST", 13*3600), "2026-12-21",
			published{"", "05:33", "05:56", "18:53", "19:17"}},
		{"Fairbanks, December", Location{64.8378, -147.7164}, time.FixedZone("AKST", -9*3600), "2026-12-21",
			published{"", "09:33", "10:58", "14:40", "16:05"}},
	} {
		noon := at(c.zone, c.date, "12:00")

		for _, e := range []struct {
			event Event
			want  string
		}{
			{CivilDawn, c.civilDawn}, {Sunrise, c.sunrise}, {Sunset, c.sunset}, {CivilDusk, c.civilDusk},
		} {
			got, ok := c.location.Time(e.event, noon)
			checkWithinAMinute(t, fmt.Sprintf("%s %s", c.name, e.event), got, ok, at(c.zone, c.date, e.want))
		}
	}
}

func TestLocation_NoEventWhenTheSunDoesntDoIt(t *testing.T) {
	tromso := Location{Latitude: 69.6492, Longitude: 18.9553}
	cet := time.FixedZone("CET", 3600)

	for _, c := range []struct {
		date  string
		event Event
		ok    bool
	}{
		{"2026-06-21", Sunset, false}, // midnight sun
		{"2026-06-21", Sunrise, false},
		{"2026-06-21", AstronomicalDusk, false}, // and it never gets dark
		{"2026-12-21", Sunrise, false},          // polar night
		{"2026-12-21", Sunset, false},
		{"2026-12-21", CivilDawn, true}, // though there is some twilight at noon
		{"2026-03-20", Sunrise, true},   // and an ordinary day at the equinox
	} {
		_, ok := tromso.Time(c.event, at(cet, c.date, "12:00"))
		if ok != c.ok {
			t.Errorf("Tromsø %s %s: ok is %v, want %v", c.date, c.event, ok, c.ok)
		}
	}
}

func TestEvent_String(t *testing.T) {
	for event, want := range map[Event]string{
		Sunrise: "sunrise", Sunset: "sunset", CivilDawn: "civil dawn", CivilDusk: "civil dusk",
		NauticalDawn: "nautical dawn", NauticalDusk: "nautical dusk",
		AstronomicalDawn: "astronomical dawn", AstronomicalDusk: "astronomical dusk", Event(99): "unknown sun event",
	} {
		if got := event.String(); got != want {
			t.Errorf("%d: got %q, want %q", event, got, want)
		}
	}
}

func TestLocation_AsExactAsTheTablesAreAbleToSay(t *testing.T) {
	// The tables are rounded to the minute, so they can be 30 seconds from the truth by themselves. Anything beyond
	// that is ours, and the refinement of the second pass is a few seconds of it.
	for _, day := range lafayetteYear {
		noon := at(mst, day.date, "12:00")

		for _, c := range []struct {
			event Event
			want  string
		}{
			{CivilDawn, day.civilDawn}, {Sunrise, day.sunrise}, {Sunset, day.sunset}, {CivilDusk, day.civilDusk},
		} {
			got, _ := lafayette.Time(c.event, noon)

			if diff := got.Sub(at(mst, day.date, c.want)).Abs(); diff > 34*time.Second {
				t.Errorf("%s %s: %v from the published time", day.date, c.event, diff)
			}
		}
	}
}

func TestLocation_TheTimeIsAFixedPointOfTheCalculation(t *testing.T) {
	// The sun is not where it was at noon when the event happens, so the time is worked out again for where it is
	// then. Working it out once more from the answer must give the answer.
	for _, event := range []Event{Sunrise, Sunset, CivilDusk, AstronomicalDawn} {
		noon := at(mst, "2026-10-16", "12:00")
		got, _ := lafayette.Time(event, noon)

		position := sunAt(got)
		hourAngle, _ := lafayette.hourAngle(event, position)
		utcDay := lafayette.utcDayOfSolarNoon(2026, 10, 16, mst)
		solarNoon := utcDay.Add(time.Duration((720 - 4*lafayette.Longitude - position.equationOfTime) * float64(time.Minute)))

		offset := time.Duration(hourAngle * 4 * float64(time.Minute))
		if event.isMorning() {
			offset = -offset
		}

		if diff := solarNoon.Add(offset).Sub(got).Abs(); diff > 500*time.Millisecond {
			t.Errorf("%s: working it out again moves it by %v", event, diff)
		}
	}
}
