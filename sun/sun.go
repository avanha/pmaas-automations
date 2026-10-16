// Package sun works out when the sun rises, sets, and passes the angles below the horizon that dawn and dusk are
// counted by, for a place and a day.
//
// It uses the algorithm of NOAA's solar calculator, which is Jean Meeus's Astronomical Algorithms in a form simple
// enough to carry around. It's good to within a minute or so at the latitudes people live at, which is as good as the
// published tables are, and they round to the minute.
package sun

import (
	"math"
	"time"
)

// Location is a place on the earth.
type Location struct {
	// Latitude is in degrees, positive to the north.
	Latitude float64

	// Longitude is in degrees, positive to the east: Colorado is a negative number.
	Longitude float64
}

// Event is something the sun does each day.
type Event int

const (
	Sunrise Event = iota
	Sunset

	// The next six are when the middle of the sun is 6, 12 and 18 degrees below the horizon: the beginning and end
	// of civil, nautical and astronomical twilight. Dawn is before sunrise and dusk is after sunset.
	CivilDawn
	CivilDusk
	NauticalDawn
	NauticalDusk
	AstronomicalDawn
	AstronomicalDusk
)

// Valid is whether e is one of the events above.
func (e Event) Valid() bool {
	return e >= Sunrise && e <= AstronomicalDusk
}

func (e Event) String() string {
	switch e {
	case Sunrise:
		return "sunrise"
	case Sunset:
		return "sunset"
	case CivilDawn:
		return "civil dawn"
	case CivilDusk:
		return "civil dusk"
	case NauticalDawn:
		return "nautical dawn"
	case NauticalDusk:
		return "nautical dusk"
	case AstronomicalDawn:
		return "astronomical dawn"
	case AstronomicalDusk:
		return "astronomical dusk"
	default:
		return "unknown sun event"
	}
}

// zenith is the angle from straight up at which the event happens, in degrees. Sunrise and sunset are when the top
// edge of the sun meets the horizon, 90 degrees and 50 arc minutes from straight up: 16 minutes for the sun's
// radius and 34 for what the atmosphere bends its light by.
func (e Event) zenith() float64 {
	switch e {
	case Sunrise, Sunset:
		return 90 + 50.0/60
	case CivilDawn, CivilDusk:
		return 96
	case NauticalDawn, NauticalDusk:
		return 102
	default:
		return 108
	}
}

func (e Event) isMorning() bool {
	return e == Sunrise || e == CivilDawn || e == NauticalDawn || e == AstronomicalDawn
}

// Time is when the event happens on the calendar day that day falls on, in day's own time zone. It reports false if
// there is no such event that day, which happens near the poles: in a midsummer when the sun never gets low enough
// for the end of astronomical twilight, or a midwinter when it never gets up.
//
// The time has seconds in it. Round it if a minute will do.
func (l Location) Time(event Event, day time.Time) (time.Time, bool) {
	zone := day.Location()
	year, month, date := day.Date()

	utcMidnight := l.utcDayOfSolarNoon(year, month, date, zone)
	estimate := utcMidnight.Add(time.Duration((720 - 4*l.Longitude) * float64(time.Minute)))

	// Work it out for the sun as it is at the middle of the day, then again for the sun as it is when the event
	// happens, which is hours away, and a little different. The second is as exact as the formulas are.
	for range 2 {
		position := sunAt(estimate)

		hourAngle, ok := l.hourAngle(event, position)
		if !ok {
			return time.Time{}, false
		}

		noon := utcMidnight.Add(time.Duration((720 - 4*l.Longitude - position.equationOfTime) * float64(time.Minute)))
		offset := time.Duration(hourAngle * 4 * float64(time.Minute))

		if event.isMorning() {
			offset = -offset
		}

		estimate = noon.Add(offset)
	}

	return estimate.In(zone), true
}

// utcDayOfSolarNoon is the start of the UTC day on which the sun is highest at the longitude, during the given
// calendar day in zone. For most of the world that isn't the UTC day with the same date: in Colorado it's the same,
// because noon is in the evening in UTC, but a calendar day in Japan has its noon on the UTC day before.
func (l Location) utcDayOfSolarNoon(year int, month time.Month, date int, zone *time.Location) time.Time {
	utcMidnight := time.Date(year, month, date, 0, 0, 0, 0, time.UTC)

	for range 3 {
		noon := utcMidnight.Add(time.Duration((720 - 4*l.Longitude) * float64(time.Minute)))
		y, m, d := noon.In(zone).Date()

		switch want, got := dayNumber(year, month, date), dayNumber(y, m, d); {
		case got == want:
			return utcMidnight
		case got < want:
			utcMidnight = utcMidnight.Add(24 * time.Hour)
		default:
			utcMidnight = utcMidnight.Add(-24 * time.Hour)
		}
	}

	return utcMidnight
}

// dayNumber puts calendar days in order.
func dayNumber(year int, month time.Month, date int) int64 {
	return time.Date(year, month, date, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// hourAngle is how far round the sky the sun is from its highest point, in degrees, when the event happens.
func (l Location) hourAngle(event Event, position position) (float64, bool) {
	latitude := radians(l.Latitude)

	cosine := math.Cos(radians(event.zenith()))/(math.Cos(latitude)*math.Cos(position.declination)) -
		math.Tan(latitude)*math.Tan(position.declination)

	if cosine < -1 || cosine > 1 {
		return 0, false
	}

	return degrees(math.Acos(cosine)), true
}

// position is what the algorithm needs to know about where the sun is on a day.
type position struct {
	// declination is how far north of the equator the sun is overhead, in radians.
	declination float64

	// equationOfTime is how far the sun is ahead of a clock that keeps perfect time, in minutes.
	equationOfTime float64
}

func radians(d float64) float64 { return d * math.Pi / 180 }
func degrees(r float64) float64 { return r * 180 / math.Pi }

// sunAt is the sun's position at an instant. The formulas are NOAA's, in the order of their spreadsheet.
func sunAt(t time.Time) position {
	// Julian century: 36525 days from noon on 1 January 2000.
	julianDay := float64(t.UnixMilli())/86400000 + 2440587.5
	c := (julianDay - 2451545) / 36525

	meanLongitude := math.Mod(280.46646+c*(36000.76983+c*0.0003032), 360)
	meanAnomaly := 357.52911 + c*(35999.05029-0.0001537*c)
	eccentricity := 0.016708634 - c*(0.000042037+0.0000001267*c)

	anomaly := radians(meanAnomaly)
	equationOfCentre := math.Sin(anomaly)*(1.914602-c*(0.004817+0.000014*c)) +
		math.Sin(2*anomaly)*(0.019993-0.000101*c) +
		math.Sin(3*anomaly)*0.000289

	trueLongitude := meanLongitude + equationOfCentre
	apparentLongitude := trueLongitude - 0.00569 - 0.00478*math.Sin(radians(125.04-1934.136*c))

	meanObliquity := 23 + (26+(21.448-c*(46.815+c*(0.00059-c*0.001813)))/60)/60
	obliquity := radians(meanObliquity + 0.00256*math.Cos(radians(125.04-1934.136*c)))

	declination := math.Asin(math.Sin(obliquity) * math.Sin(radians(apparentLongitude)))

	y := math.Tan(obliquity/2) * math.Tan(obliquity/2)
	l0 := radians(meanLongitude)
	equation := y*math.Sin(2*l0) -
		2*eccentricity*math.Sin(anomaly) +
		4*eccentricity*y*math.Sin(anomaly)*math.Cos(2*l0) -
		0.5*y*y*math.Sin(4*l0) -
		1.25*eccentricity*eccentricity*math.Sin(2*anomaly)

	return position{declination: declination, equationOfTime: 4 * degrees(equation)}
}
