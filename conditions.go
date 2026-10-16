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

// Result is what a Condition makes of things.
type Result int

const (
	// Unmet is that the condition is not true.
	Unmet Result = iota

	// Met is that it is true.
	Met

	// Unknown is that it can't be told, because whatever it looks at hasn't said or has gone quiet. It is neither
	// true nor false: something that acts on a condition should not act on one that is unknown, and certainly not as if it
	// were false.
	Unknown
)

// Condition says whether something is so. Whatever it says, it says why, in words for the page: "the fan is off".
type Condition func() (Result, string)

// When makes something to run, for New, that does run only when every one of the conditions is met. When they aren't,
// it is not ready: the automation stays due and asks again at the next minute, until it acts or its catch-up passes,
// and the page says why it is waiting, naming every condition that isn't met or can't be told.
//
// That makes the catch-up the end of the time in which it may act: set it to how long after the time it is due the
// conditions may be met. An automation that is due at 1:00 AM with a catch-up of 6h59m acts only if the conditions are met
// before 8:00.
func When(run func() (string, error), conditions ...Condition) func() (string, error) {
	if run == nil {
		panic("automations.When: nothing to run")
	}

	check(conditions)

	return func() (string, error) {
		var reasons []string

		for _, condition := range conditions {
			if result, reason := condition(); result != Met {
				reasons = append(reasons, reason)
			}
		}

		if len(reasons) > 0 {
			return "", automation.NotReady("%s", strings.Join(reasons, "; "))
		}

		return run()
	}
}

// AllOf is a condition that is met when every one of the conditions is. If any can't be told it can't be told, whatever
// the others say, since the one that can't be told might have been the one that decided; otherwise, if any is not met
// it is not met. It gives the reasons of those that made it what it is.
func AllOf(conditions ...Condition) Condition {
	check(conditions)

	return func() (Result, string) {
		var met, unmet, unknown []string

		for _, condition := range conditions {
			switch result, reason := condition(); result {
			case Met:
				met = append(met, reason)
			case Unknown:
				unknown = append(unknown, reason)
			default:
				unmet = append(unmet, reason)
			}
		}

		switch {
		case len(unknown) > 0:
			return Unknown, strings.Join(unknown, "; ")
		case len(unmet) > 0:
			return Unmet, strings.Join(unmet, "; ")
		default:
			return Met, strings.Join(met, "; ")
		}
	}
}

func check(conditions []Condition) {
	for i, condition := range conditions {
		if condition == nil {
			panic(fmt.Sprintf("automations: condition %d is nil", i))
		}
	}
}

// SwitchIs is met when the switch is in the state. A switch that has not said what state it is in can't be told.
func SwitchIs(sw device.ISwitch, state device.SwitchState) Condition {
	if sw == nil {
		panic("automations.SwitchIs: no switch")
	}

	return func() (Result, string) {
		data := sw.GetSwitchData()

		name := data.Name
		if name == "" {
			name = "the switch"
		}

		switch {
		case !data.HasData:
			return Unknown, fmt.Sprintf("%s has not said whether it is on or off", name)
		case data.State != state:
			return Unmet, fmt.Sprintf("%s is %s, not %s", name, data.State, state)
		default:
			return Met, fmt.Sprintf("%s is %s", name, state)
		}
	}
}

// reading is what a thermometer reads, and who to call it, or, if it can't be gone by, a reason why.
func reading(thermometer environment.IWirelessThermometer, maxAge time.Duration) (name string, temperature Temperature, problem string) {
	data := thermometer.GetWirelessThermometerData()
	sensor := data.SensorData

	name = data.Name
	if name == "" {
		name = "the thermometer"
	}

	switch {
	case !sensor.HasData:
		return name, 0, fmt.Sprintf("%s has not reported a temperature", name)
	case maxAge > 0 && now().Sub(sensor.LastUpdateTime) > maxAge:
		return name, 0, fmt.Sprintf("%s last reported %s ago, which is too long ago to go by",
			name, now().Sub(sensor.LastUpdateTime).Round(time.Minute))
	default:
		return name, Celsius(sensor.Temperature), ""
	}
}

// TemperatureBelow is met when the thermometer reads less than the limit, and not met at the limit. It can't be told if
// the thermometer has not reported, or if its latest reading is more than maxAge old (zero means any age), since a
// temperature that old is not the temperature now, and acting on it is worse than waiting.
func TemperatureBelow(thermometer environment.IWirelessThermometer, limit Temperature, maxAge time.Duration) Condition {
	if thermometer == nil {
		panic("automations.TemperatureBelow: no thermometer")
	}

	return func() (Result, string) {
		name, temperature, problem := reading(thermometer, maxAge)

		switch {
		case problem != "":
			return Unknown, problem
		case temperature < limit:
			return Met, fmt.Sprintf("%s reads %s, below %s", name, temperature, limit)
		default:
			return Unmet, fmt.Sprintf("%s reads %s, not below %s", name, temperature, limit)
		}
	}
}

// TemperatureAbove is met when the thermometer reads more than the limit, and not met at the limit. It can't be told
// when TemperatureBelow can't.
func TemperatureAbove(thermometer environment.IWirelessThermometer, limit Temperature, maxAge time.Duration) Condition {
	if thermometer == nil {
		panic("automations.TemperatureAbove: no thermometer")
	}

	return func() (Result, string) {
		name, temperature, problem := reading(thermometer, maxAge)

		switch {
		case problem != "":
			return Unknown, problem
		case temperature > limit:
			return Met, fmt.Sprintf("%s reads %s, above %s", name, temperature, limit)
		default:
			return Unmet, fmt.Sprintf("%s reads %s, not above %s", name, temperature, limit)
		}
	}
}

// Warmer is met when the first thermometer reads more than the second, and not met if they read the same. It can't be
// told if either can't be gone by, for the reasons of TemperatureBelow.
func Warmer(thermometer environment.IWirelessThermometer, than environment.IWirelessThermometer, maxAge time.Duration) Condition {
	if thermometer == nil || than == nil {
		panic("automations.Warmer: a thermometer is missing")
	}

	return func() (Result, string) {
		name, temperature, problem := reading(thermometer, maxAge)
		otherName, other, otherProblem := reading(than, maxAge)

		switch {
		case problem != "" && otherProblem != "":
			return Unknown, problem + "; " + otherProblem
		case problem != "":
			return Unknown, problem
		case otherProblem != "":
			return Unknown, otherProblem
		case temperature > other:
			return Met, fmt.Sprintf("%s reads %s, warmer than %s at %s", name, temperature, otherName, other)
		default:
			return Unmet, fmt.Sprintf("%s reads %s, not warmer than %s at %s", name, temperature, otherName, other)
		}
	}
}

// Follow makes something to run, for New, that keeps a switch on while the condition is met and off while it is not,
// and changes it no more often than every minInterval.
//
// Each time it is run it looks at the condition and at the switch. If the switch is already as it should be, there is
// nothing to do and it says so by not being ready, which costs nothing and is not logged or kept. If the condition can't
// be told, it leaves the switch as it is. If it needs to change the switch but the switch changed less than minInterval
// ago, it waits, so that a temperature hovering around a limit doesn't have a fan starting and stopping every minute.
// How long ago the switch changed is the switch's own word for it, so it counts a change someone made by hand, and
// it isn't forgotten when the server restarts.
//
// Run it often, with a trigger like Every. It is the automation's job to say when it is looked at. Since that is
// once a minute, a change that has been held back is made at the first look after the interval has passed, which can
// be up to a minute after it.
func Follow(sw device.ISwitch, condition Condition, minInterval time.Duration) func() (string, error) {
	if sw == nil {
		panic("automations.Follow: no switch")
	}

	if condition == nil {
		panic("automations.Follow: no condition")
	}

	return func() (string, error) {
		result, reason := condition()
		data := sw.GetSwitchData()

		name := data.Name
		if name == "" {
			name = "the switch"
		}

		if result == Unknown {
			return "", automation.NotReady("leaving %s as it is: %s", name, reason)
		}

		wanted := device.SwitchStateOff
		if result == Met {
			wanted = device.SwitchStateOn
		}

		// A switch that has not reported is Unknown, which is neither state, so it is told.
		if data.State == wanted {
			return "", automation.NotReady("%s is %s, as it should be: %s", name, wanted, reason)
		}

		// A switch that has never changed has a zero time of changing, which is as long ago as can be.
		if since := now().Sub(data.LastChangeTime); since < minInterval {
			return "", automation.NotReady("%s should be %s (%s), but it changed %s ago, and is not changed again until %s have passed",
				name, wanted, reason, since.Truncate(time.Second), minInterval)
		}

		if err := sw.SetState(wanted); err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}

		return fmt.Sprintf("turned %s %s: %s", strings.ToLower(wanted.String()), name, reason), nil
	}
}
