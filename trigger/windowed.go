package trigger

import (
	"fmt"
	"strings"
	"time"
)

// maxWindowSteps is how many times Windowed jumps from one window to the next looking for a time that is due in one,
// about as many as there are days in the search.
const maxWindowSteps = searchDays * 2

// Windowed is due whenever inner is, but only while the time is within a window that opens whenever open is due and
// closes whenever close is due: Windowed(Every(time.Minute), AtTimeOfDay(8, 0), Sun(location, sun.Sunset, time.Hour)) is
// due every minute from 8:00 AM until an hour after sunset, and not at all outside it. The window is open from the
// moment open is due, and closed from the moment close is.
//
// A window can cross midnight (open at 22:00, close at 06:00), and can change length from day to day, as one that ends
// at sunset does. A window that is shorter than the gaps in inner contains none of its times, and so is never due.
//
// It panics if any of the triggers is nil, for the reason AtTimeOfDay does.
func Windowed(inner Trigger, open Trigger, close Trigger) Trigger {
	if inner == nil || open == nil || close == nil {
		panic("trigger.Windowed: a trigger is nil")
	}

	return &windowed{inner: inner, open: open, close: close}
}

type windowed struct {
	inner Trigger
	open  Trigger
	close Trigger
}

func (w *windowed) Description() string {
	return fmt.Sprintf("%s, from %s until %s",
		w.inner.Description(), strings.TrimPrefix(w.open.Description(), "at "), strings.TrimPrefix(w.close.Description(), "at "))
}

// inWindow is whether t is within the window.
func (w *windowed) inWindow(t time.Time) bool {
	return InWindow(w.open, w.close, t)
}

func (w *windowed) Previous(t time.Time) (time.Time, bool) {
	candidate, ok := w.inner.Previous(t)

	for range maxWindowSteps {
		if !ok {
			return time.Time{}, false
		}

		if w.inWindow(candidate) {
			return candidate, true
		}

		// Outside a window: the latest time it was due in one is before the closing that ended the window before it.
		closed, ok := w.close.Previous(candidate)
		if !ok {
			return time.Time{}, false
		}

		candidate, ok = w.inner.Previous(closed.Add(-time.Nanosecond))
		if !ok {
			return time.Time{}, false
		}
	}

	return time.Time{}, false
}

func (w *windowed) Next(t time.Time) (time.Time, bool) {
	candidate, ok := w.inner.Next(t)

	for range maxWindowSteps {
		if !ok {
			return time.Time{}, false
		}

		if w.inWindow(candidate) {
			return candidate, true
		}

		// Outside a window: the earliest time it is due in one is at or after the window's next opening.
		opens, hasOpen := w.open.Next(candidate)
		if !hasOpen {
			return time.Time{}, false
		}

		candidate, ok = w.inner.Next(opens.Add(-time.Nanosecond))
	}

	return time.Time{}, false
}

// InWindow is whether t is within a daily window that opens whenever open is due and closes whenever close is: the latest
// time the window opened, at or before t, is later than the latest time it closed. It is open from the moment it opens
// and closed from the moment it closes, and a window that has opened and never closed is open. It works for windows
// that cross midnight, and for ones whose ends move with the sun: InWindow(Sun(l, sun.CivilDusk, 0), Sun(l, sun.CivilDawn, 0), t)
// is whether it is dark at t.
func InWindow(open Trigger, close Trigger, t time.Time) bool {
	opened, ok := open.Previous(t)
	if !ok {
		return false
	}

	closed, ok := close.Previous(t)

	return !ok || opened.After(closed)
}
