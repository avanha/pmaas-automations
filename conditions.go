package automations

import (
	"fmt"
	"strings"
	"time"

	"github.com/avanha/pmaas-spi/automation"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

// now is the time, which the tests replace.
var now = time.Now

// Temperature is a temperature. Entities report theirs in Celsius, so that is what it keeps; make one in the unit you
// think in, with Celsius or Fahrenheit.
type Temperature float32

// Celsius is a temperature in degrees Celsius.
func Celsius(degrees float32) Temperature {
	return Temperature(degrees)
}

// Fahrenheit is a temperature in degrees Fahrenheit.
func Fahrenheit(degrees float32) Temperature {
	return Temperature((degrees - 32) * 5 / 9)
}

// Celsius is the temperature in degrees Celsius.
func (t Temperature) Celsius() float32 {
	return float32(t)
}

// Fahrenheit is the temperature in degrees Fahrenheit.
func (t Temperature) Fahrenheit() float32 {
	return float32(t)*9/5 + 32
}

// String is the temperature in both units: "75.0°F (23.9°C)".
func (t Temperature) String() string {
	return fmt.Sprintf("%.1f°F (%.1f°C)", t.Fahrenheit(), t.Celsius())
}

// Condition says whether it is time to act. If it isn't, it says why, in words for the page: "the fan is off".
type Condition func() (met bool, reason string)

// When makes something to run, for New, that does run only when every one of the conditions is met. When they aren't,
// it is not ready: the automation stays due and asks again at the next minute, until it acts or its catch-up passes,
// and the page says why it is waiting, naming every condition that isn't met.
//
// That makes the catch-up the end of the time in which it may act: set it to how long after the time it is due the
// conditions may be met. An automation that is due at 1:00 AM with a catch-up of 6h59m acts only if the conditions are met
// before 8:00.
func When(run func() (string, error), conditions ...Condition) func() (string, error) {
	if run == nil {
		panic("automations.When: nothing to run")
	}

	for i, condition := range conditions {
		if condition == nil {
			panic(fmt.Sprintf("automations.When: condition %d is nil", i))
		}
	}

	return func() (string, error) {
		var reasons []string

		for _, condition := range conditions {
			if met, reason := condition(); !met {
				reasons = append(reasons, reason)
			}
		}

		if len(reasons) > 0 {
			return "", automation.NotReady("%s", strings.Join(reasons, "; "))
		}

		return run()
	}
}

// SwitchIs is met when the switch is in the state. A switch that has not said what state it is in is not in either.
func SwitchIs(sw device.ISwitch, state device.SwitchState) Condition {
	if sw == nil {
		panic("automations.SwitchIs: no switch")
	}

	return func() (bool, string) {
		data := sw.GetSwitchData()

		name := data.Name
		if name == "" {
			name = "the switch"
		}

		switch {
		case !data.HasData:
			return false, fmt.Sprintf("%s has not said whether it is on or off", name)
		case data.State != state:
			return false, fmt.Sprintf("%s is %s, not %s", name, data.State, state)
		default:
			return true, ""
		}
	}
}

// TemperatureBelow is met when the thermometer reads less than the limit. It is not met if the thermometer has not
// reported, or if its latest reading is more than maxAge old (zero means any age), since a temperature that old is not
// the temperature now, and acting on it is worse than waiting.
func TemperatureBelow(thermometer environment.IWirelessThermometer, limit Temperature, maxAge time.Duration) Condition {
	if thermometer == nil {
		panic("automations.TemperatureBelow: no thermometer")
	}

	return func() (bool, string) {
		data := thermometer.GetWirelessThermometerData()
		reading := data.SensorData

		name := data.Name
		if name == "" {
			name = "the thermometer"
		}

		switch {
		case !reading.HasData:
			return false, fmt.Sprintf("%s has not reported a temperature", name)
		case maxAge > 0 && now().Sub(reading.LastUpdateTime) > maxAge:
			return false, fmt.Sprintf("%s last reported %s ago, which is too long ago to go by",
				name, now().Sub(reading.LastUpdateTime).Round(time.Minute))
		case Celsius(reading.Temperature) >= limit:
			return false, fmt.Sprintf("%s reads %s, not below %s", name, Celsius(reading.Temperature), limit)
		default:
			return true, ""
		}
	}
}
