package automations

import (
	"time"

	"github.com/avanha/pmaas-automations/trigger"
	"github.com/avanha/pmaas-spi/alert"
)

// SeverityBetween is a severity that is inside from the moment open is due until the moment close is due, and outside the
// rest of the time. The ends are triggers, so they can be times of day or moments of the sun's: critical after dark and
// a warning in the day is
//
//	SeverityBetween(trigger.Sun(home, sun.CivilDusk, 0), trigger.Sun(home, sun.CivilDawn, 0), alert.SeverityCritical, alert.SeverityWarning)
//
// For clock times alone, alert.ByTimeOfDay does the same without this package.
func SeverityBetween(open trigger.Trigger, close trigger.Trigger, inside alert.Severity, outside alert.Severity) alert.SeverityFunc {
	if open == nil || close == nil {
		panic("automations.SeverityBetween: a trigger is nil")
	}

	return func(at time.Time) alert.Severity {
		if trigger.InWindow(open, close, at) {
			return inside
		}

		return outside
	}
}
