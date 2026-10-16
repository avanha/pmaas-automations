package automations

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/avanha/pmaas-automations/trigger"
	"github.com/avanha/pmaas-spi/automation"
	"github.com/avanha/pmaas-spi/device"
)

func TestNew_MakesAValidAutomationWithTheDefaults(t *testing.T) {
	at := trigger.AtTimeOfDay(17, 0)
	runs := 0

	a := New("lights-on", "Turn on the lights", at, func() (string, error) { runs++; return "done", nil })

	if a.ID() != "lights-on" || a.Name() != "Turn on the lights" || a.Description() != "" || a.Trigger() != at {
		t.Errorf("automation: %v %v %q %v", a.ID(), a.Name(), a.Description(), a.Trigger())
	}

	if a.CatchUp() != DefaultCatchUp || DefaultCatchUp != time.Hour {
		t.Errorf("catch-up: %v", a.CatchUp())
	}

	if note, err := a.Run(); note != "done" || err != nil || runs != 1 {
		t.Errorf("run: %q, %v, %d times", note, err, runs)
	}

	if err := automation.Validate(a); err != nil {
		t.Errorf("not valid: %v", err)
	}
}

func TestNew_Options(t *testing.T) {
	a := New("id", "name", trigger.AtTimeOfDay(1, 0), func() (string, error) { return "", nil },
		WithDescription("more"), WithCatchUp(0))

	if a.Description() != "more" || a.CatchUp() != 0 {
		t.Errorf("description %q, catch-up %v", a.Description(), a.CatchUp())
	}

	b := New("id", "name", trigger.AtTimeOfDay(1, 0), func() (string, error) { return "", nil }, WithCatchUp(6*time.Hour))
	if b.CatchUp() != 6*time.Hour {
		t.Errorf("catch-up %v", b.CatchUp())
	}
}

func TestNew_PanicsForAnAutomationThatCouldNotRun(t *testing.T) {
	run := func() (string, error) { return "", nil }
	at := trigger.AtTimeOfDay(1, 0)

	for name, make := range map[string]func(){
		"no id":            func() { New("", "n", at, run) },
		"no name":          func() { New("i", "", at, run) },
		"no trigger":       func() { New("i", "n", nil, run) },
		"nothing to run":   func() { New("i", "n", at, nil) },
		"negative catchup": func() { New("i", "n", at, run, WithCatchUp(-time.Second)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: didn't panic", name)
				}
			}()

			make()
		}()
	}
}

type fakeSwitch struct {
	data    device.Switch
	err     error
	sets    []device.SwitchState
	changes bool
}

func (f *fakeSwitch) GetSwitchData() device.Switch { return f.data }

func (f *fakeSwitch) SetState(state device.SwitchState) error {
	f.sets = append(f.sets, state)

	if f.err == nil && f.changes {
		f.data.HasData, f.data.State = true, state
	}

	return f.err
}

func sw(name string, state device.SwitchState) *fakeSwitch {
	return &fakeSwitch{data: device.Switch{Name: name, HasData: state != device.SwitchStateUnknown, State: state}, changes: true}
}

func TestTurnOn_TurnsOnWhatIsOffAndLeavesWhatIsOn(t *testing.T) {
	off, on := sw("Porch", device.SwitchStateOff), sw("Garage", device.SwitchStateOn)

	note, err := TurnOn(off, on)()

	if err != nil || note != "turned on Porch; Garage already on" {
		t.Errorf("got %q, %v", note, err)
	}

	if len(off.sets) != 1 || off.sets[0] != device.SwitchStateOn || len(on.sets) != 0 {
		t.Errorf("sets: %v and %v", off.sets, on.sets)
	}
}

func TestTurnOff_IsTheOpposite(t *testing.T) {
	on := sw("Porch", device.SwitchStateOn)

	note, err := TurnOff(on)()

	if err != nil || note != "turned off Porch" || len(on.sets) != 1 || on.sets[0] != device.SwitchStateOff {
		t.Errorf("got %q, %v, sets %v", note, err, on.sets)
	}
}

func TestTurnOn_ASwitchThatHasNotReportedIsToldAnywayBecauseItMightBeEitherWay(t *testing.T) {
	unknown := sw("Porch", device.SwitchStateUnknown)

	note, err := TurnOn(unknown)()

	if err != nil || note != "turned on Porch" || len(unknown.sets) != 1 {
		t.Errorf("got %q, %v, sets %v", note, err, unknown.sets)
	}
}

func TestTurnOn_OneThatCantBeTurnedOnDoesntStopTheOthersAndIsReported(t *testing.T) {
	offline := sw("Porch", device.SwitchStateOff)
	offline.err = errors.New("the device is offline")
	fine := sw("Garage", device.SwitchStateOff)

	note, err := TurnOn(offline, fine)()

	if err == nil || !strings.Contains(err.Error(), "Porch: the device is offline") {
		t.Errorf("error: %v", err)
	}

	if note != "turned on Garage" || len(fine.sets) != 1 {
		t.Errorf("note %q, sets %v", note, fine.sets)
	}
}

func TestTurnOn_RunAgainAfterAFailureFindsTheOthersDone(t *testing.T) {
	flaky := sw("Porch", device.SwitchStateOff)
	flaky.err = errors.New("offline")
	fine := sw("Garage", device.SwitchStateOff)

	run := TurnOn(flaky, fine)

	_, _ = run()
	flaky.err = nil

	note, err := run()

	if err != nil || note != "turned on Porch; Garage already on" || len(fine.sets) != 1 {
		t.Errorf("second run: %q, %v, fine sets %v", note, err, fine.sets)
	}
}

func TestTurnOn_ASwitchWithoutANameIsCalledByItsPlace(t *testing.T) {
	note, _ := TurnOn(sw("", device.SwitchStateOff), sw("Porch", device.SwitchStateOff))()

	if note != "turned on switch 1, Porch" {
		t.Errorf("note %q", note)
	}
}

func TestTurnOn_PanicsWhenThereIsNothingToTurnOn(t *testing.T) {
	for name, make := range map[string]func(){
		"none": func() { TurnOn() },
		"nil":  func() { TurnOff(sw("a", device.SwitchStateOn), nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: didn't panic", name)
				}
			}()

			make()
		}()
	}
}
