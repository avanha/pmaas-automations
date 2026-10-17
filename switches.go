package automations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/avanha/pmaas-spi/device"
)

// TurnOn is something for New to run that turns every one of the switches on. A switch that is already on is left
// alone. If one can't be turned on, because its device is offline, say, the others still are, and the error says which
// weren't; the automation is then run again, and finds those that were done already done.
func TurnOn(switches ...device.ISwitch) func() (string, error) {
	return setSwitches(device.SwitchStateOn, switches)
}

// TurnOff is TurnOn's opposite.
func TurnOff(switches ...device.ISwitch) func() (string, error) {
	return setSwitches(device.SwitchStateOff, switches)
}

func setSwitches(state device.SwitchState, switches []device.ISwitch) func() (string, error) {
	checkSwitches("turn "+state.String(), switches)

	return func() (string, error) {
		return applyState(state, switches)
	}
}

// Toggle is something for New to run that flips the switches: on the first run it turns them all on if the first is off
// (or has not said), and off if it is on, and on each run after it turns them all the opposite of the time before.
//
// Flipping isn't something that can safely be done twice, which a retry after a failure would do, so it doesn't flip
// what it finds: it decides which state this run is to put them all in, and keeps deciding the same until the run
// has succeeded. If one switch can't be reached, the others are set again, to the same state, at the retry, and then
// the next run takes them all the other way. Switches that were changed by hand since the last run are put in the state
// regardless. The state it expects is not kept across a restart: after one it begins again from the first switch.
func Toggle(switches ...device.ISwitch) func() (string, error) {
	checkSwitches("toggle", switches)

	next := device.SwitchStateUnknown

	return func() (string, error) {
		if next == device.SwitchStateUnknown {
			next = device.SwitchStateOn

			if switches[0].GetSwitchData().State == device.SwitchStateOn {
				next = device.SwitchStateOff
			}
		}

		note, err := applyState(next, switches)
		if err != nil {
			return note, err
		}

		if next == device.SwitchStateOn {
			next = device.SwitchStateOff
		} else {
			next = device.SwitchStateOn
		}

		return note, nil
	}
}

// checkSwitches panics if there is nothing to act on, as the constructors do, saying what for.
func checkSwitches(what string, switches []device.ISwitch) {
	if len(switches) == 0 {
		panic(fmt.Sprintf("automations: nothing to %s", strings.ToLower(what)))
	}

	for i, sw := range switches {
		if sw == nil {
			panic(fmt.Sprintf("automations: switch %d to %s is nil", i, strings.ToLower(what)))
		}
	}
}

// SwitchNames lists the switches by name, as a person would: "Kitchen Relay", "Kitchen Relay and Porch Relay", "A, B and
// C". A switch that hasn't said its name, because its device hasn't connected yet, is "switch 2", by its place in the list.
// It is for a description that names what an automation acts on (see WithDescriptionFunc).
func SwitchNames(switches ...device.ISwitch) string {
	names := make([]string, len(switches))

	for i, sw := range switches {
		names[i] = sw.GetSwitchData().Name

		if names[i] == "" {
			names[i] = fmt.Sprintf("switch %d", i+1)
		}
	}

	switch len(names) {
	case 0:
		return "no switches"
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// applyState puts every switch in the state, leaving alone those already in it, and says what it did. Every switch is
// tried, whatever happens to the others, and the error says which could not be.
func applyState(state device.SwitchState, switches []device.ISwitch) (string, error) {
	var changed, already []string
	var failures []error

	for i, sw := range switches {
		data := sw.GetSwitchData()
		name := data.Name

		if name == "" {
			name = fmt.Sprintf("switch %d", i+1)
		}

		// One that has not reported anything is Unknown, which is neither state, so it is told.
		if data.State == state {
			already = append(already, name)
			continue
		}

		if err := sw.SetState(state); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			continue
		}

		changed = append(changed, name)
	}

	return describe(state, changed, already), errors.Join(failures...)
}

// describe is what was done, for the page: "turned on A, B; C was already on".
func describe(state device.SwitchState, changed []string, already []string) string {
	verb := strings.ToLower(state.String())

	var parts []string

	if len(changed) > 0 {
		parts = append(parts, fmt.Sprintf("turned %s %s", verb, strings.Join(changed, ", ")))
	}

	if len(already) > 0 {
		parts = append(parts, fmt.Sprintf("%s already %s", strings.Join(already, ", "), verb))
	}

	return strings.Join(parts, "; ")
}
