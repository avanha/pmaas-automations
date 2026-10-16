package automations

import (
	"fmt"
	"time"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// DoorOpenFor is met when the door has been open for at least the duration, not met while it is closed or has been open for
// less, and can't be told while the sensor is not reporting a position or can't be reached, since its last position may
// not be the door's now.
//
// How long a door has been open is when the sensor last saw it change. A sensor that has only ever reported the door open,
// say because the server started with it open, has not seen it change, and then the door is taken to have been open since
// the first time this condition looked: no longer than that is known.
//
// The condition remembers that, so make one for each place it is used, and don't call it from more than one goroutine at a
// time, which the automations plugin doesn't.
func DoorOpenFor(door device.IDoorSensor, duration time.Duration) Condition {
	if door == nil {
		panic("automations.DoorOpenFor: no door sensor")
	}

	// When the condition first saw a door open that the sensor had not seen change. Zero while the door is closed.
	var firstSeenOpen time.Time

	return func() (Result, string) {
		data := door.GetDoorSensorData()

		name := data.Name
		if name == "" {
			name = "the door"
		}

		switch {
		case !data.HasData || data.Position == device.DoorPositionUnknown:
			return Unknown, fmt.Sprintf("%s has not reported its position", name)
		case data.Connectivity == environment.ConnectivityOffline:
			return Unknown, fmt.Sprintf("%s's sensor is offline, so it can't be told if the door is open", name)
		case data.Position == device.DoorPositionClosed:
			firstSeenOpen = time.Time{}

			return Unmet, fmt.Sprintf("%s is closed", name)
		}

		since := data.LastChangeTime
		if since.IsZero() {
			if firstSeenOpen.IsZero() {
				firstSeenOpen = now()
			}

			since = firstSeenOpen
		}

		open := now().Sub(since)

		if open < duration {
			return Unmet, fmt.Sprintf("%s has been open for %s, not yet %s", name, open.Truncate(time.Second), duration)
		}

		return Met, fmt.Sprintf("%s has been open for %s", name, open.Truncate(time.Second))
	}
}

// DoorSensorOffline is met when the sensor has been unreachable for at least the duration, and not met while it is
// reachable or has been out of touch for less. It can't be told if the sensor doesn't say whether it is reachable.
func DoorSensorOffline(door device.IDoorSensor, duration time.Duration) Condition {
	if door == nil {
		panic("automations.DoorSensorOffline: no door sensor")
	}

	return func() (Result, string) {
		data := door.GetDoorSensorData()

		name := data.Name
		if name == "" {
			name = "the door"
		}

		switch data.Connectivity {
		case environment.ConnectivityOnline:
			return Unmet, fmt.Sprintf("%s's sensor is online", name)
		case environment.ConnectivityOffline:
			offline := now().Sub(data.OfflineSince)
			if offline < duration {
				return Unmet, fmt.Sprintf("%s's sensor has been offline for %s, not yet %s", name, offline.Truncate(time.Second), duration)
			}

			return Met, fmt.Sprintf("%s's sensor has been offline for %s", name, offline.Truncate(time.Second))
		default:
			return Unknown, fmt.Sprintf("%s's sensor has not said whether it is reachable", name)
		}
	}
}
