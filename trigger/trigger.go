// Package trigger has the triggers that say when an automation is due: at a time of day, or at an event of the sun.
//
// A trigger is a pure function of the time (see automation.Trigger), which is what makes it easy to test.
package trigger

import (
	"fmt"
	"strings"
	"time"

	"github.com/avanha/pmaas-automations/sun"
	"github.com/avanha/pmaas-spi/automation"
)

// searchDays is how far from the time asked about a trigger looks for when it's due, a year and a bit. A trigger
// that isn't due at all in that long, like sunset north of the Arctic Circle in summer, is not due.
const searchDays = 400

// Trigger is what the server asks when an automation is due. It's defined in the SPI, so that the server doesn't
// have to know about this package; everything here returns it.
type Trigger = automation.Trigger

// Times are in the zone of the time the trigger is asked about, which is the server's local time when it's asked
// about the time now. Each is to the minute, because that is how often anything checks.

// AtTimeOfDay is due every day at the given time. On a day when daylight saving time skips that time, it is due at
// whatever the zone says it was then; on a day when it comes round twice, it is due once.
//
// It panics if there is no such time of day, like regexp.MustCompile does for a pattern that can't be: the time is
// written in the program, and a typo in it should stop the program starting, not make something happen at a time
// nobody meant.
func AtTimeOfDay(hour int, minute int) Trigger {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		panic(fmt.Sprintf("trigger.AtTimeOfDay(%d, %d): not a time of day", hour, minute))
	}

	return &daily{
		description: fmt.Sprintf("at %02d:%02d", hour, minute),
		occurrence: func(day time.Time) (time.Time, bool) {
			year, month, date := day.Date()

			return time.Date(year, month, date, hour, minute, 0, 0, day.Location()), true
		},
	}
}

// Sun is due every day at an event of the sun, as it is at the location, and offset after it; make the offset
// negative for before. It panics if the event is not one of the sun package's, for the reason AtTimeOfDay does.
func Sun(location sun.Location, event sun.Event, offset time.Duration) Trigger {
	if !event.Valid() {
		panic(fmt.Sprintf("trigger.Sun: %d is not a sun event", int(event)))
	}

	return &daily{
		description: describeSun(event, offset),
		occurrence: func(day time.Time) (time.Time, bool) {
			at, ok := location.Time(event, day)
			if !ok {
				return time.Time{}, false
			}

			return at.Round(time.Minute).Add(offset.Round(time.Minute)), true
		},
		// An offset of more than a day puts it more than a day from the day it's for.
		slackDays: int(offset.Abs() / (24 * time.Hour)),
	}
}

// daily is a trigger that is due at most once on each calendar day, at a time that is worked out from the day.
type daily struct {
	description string

	// occurrence is when it's due for the day, which it's given as a time in that day, and the zone to work in.
	occurrence func(day time.Time) (time.Time, bool)

	// slackDays is how many more days than usual to look at: how much further from its day it can be.
	slackDays int
}

func (d *daily) Description() string {
	return d.description
}

// reach is how many days from the one asked about an occurrence for a day can be: an occurrence can be a day away
// from its own day, if the offset or a time zone puts it there, and more if the offset is large.
func (d *daily) reach() int {
	return 2 + d.slackDays
}

func (d *daily) Previous(t time.Time) (time.Time, bool) {
	// Start with the days after, which can be due before t when they are shifted back, and work back. Occurrences
	// are in the order of their days, so the first that is not after t is the latest that isn't.
	for offset := d.reach(); offset >= -searchDays; offset-- {
		if at, ok := d.occurrence(dayAt(t, offset)); ok && !at.After(t) {
			return at, true
		}
	}

	return time.Time{}, false
}

func (d *daily) Next(t time.Time) (time.Time, bool) {
	for offset := -d.reach(); offset <= searchDays; offset++ {
		if at, ok := d.occurrence(dayAt(t, offset)); ok && at.After(t) {
			return at, true
		}
	}

	return time.Time{}, false
}

// dayAt is the middle of the day that is days from the one t is in, in t's zone. The middle, so that it is well inside
// the day whatever the zone does at its edges.
func dayAt(t time.Time, days int) time.Time {
	year, month, date := t.Date()

	return time.Date(year, month, date+days, 12, 0, 0, 0, t.Location())
}

func describeSun(event sun.Event, offset time.Duration) string {
	offset = offset.Round(time.Minute)

	if offset == 0 {
		return "at " + event.String()
	}

	direction := "after"
	if offset < 0 {
		direction = "before"
		offset = -offset
	}

	return fmt.Sprintf("%s %s %s", durationInWords(offset), direction, event)
}

// durationInWords is a whole number of minutes the way a person would say it.
func durationInWords(d time.Duration) string {
	hours := int(d / time.Hour)
	minutes := int(d % time.Hour / time.Minute)

	var parts []string
	if hours > 0 {
		parts = append(parts, plural(hours, "hour"))
	}

	if minutes > 0 {
		parts = append(parts, plural(minutes, "minute"))
	}

	return strings.Join(parts, " ")
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}

	return fmt.Sprintf("%d %ss", n, unit)
}
