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
	if len(switches) == 0 {
		panic(fmt.Sprintf("automations: nothing to turn %s", strings.ToLower(state.String())))
	}

	for i, sw := range switches {
		if sw == nil {
			panic(fmt.Sprintf("automations: switch %d to turn %s is nil", i, strings.ToLower(state.String())))
		}
	}

	return func() (string, error) {
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
