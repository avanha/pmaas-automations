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

func TestToggle_FlipsAllOfThemEachRunStartingFromTheFirst(t *testing.T) {
	a, b := sw("A", device.SwitchStateOff), sw("B", device.SwitchStateOn)
	toggle := Toggle(a, b)

	for i, want := range []device.SwitchState{
		device.SwitchStateOn, device.SwitchStateOff, device.SwitchStateOn, device.SwitchStateOff,
	} {
		if _, err := toggle(); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}

		if a.data.State != want || b.data.State != want {
			t.Fatalf("run %d: A is %v and B is %v, want both %v", i, a.data.State, b.data.State, want)
		}
	}
}

func TestToggle_BeginsByTurningOffWhatIsOnAndOnWhatHasNotSaid(t *testing.T) {
	on := sw("A", device.SwitchStateOn)

	if _, err := Toggle(on)(); err != nil || on.data.State != device.SwitchStateOff {
		t.Errorf("an on switch: %v %v", on.data.State, err)
	}

	unknown := sw("A", device.SwitchStateUnknown)

	if _, err := Toggle(unknown)(); err != nil || unknown.data.State != device.SwitchStateOn {
		t.Errorf("a switch that has not said: %v %v", unknown.data.State, err)
	}
}

func TestToggle_AFailureIsRetriedToTheSameStateAndThenTheNextRunFlipsThemAll(t *testing.T) {
	a, b := sw("A", device.SwitchStateOff), sw("B", device.SwitchStateOff)
	b.err = errors.New("offline")
	toggle := Toggle(a, b)

	note, err := toggle()
	if err == nil || a.data.State != device.SwitchStateOn || b.data.State != device.SwitchStateOff || note != "turned on A" {
		t.Fatalf("first run: A %v, B %v, note %q, err %v", a.data.State, b.data.State, note, err)
	}

	// The retry puts them in the state the first run meant to; it doesn't flip A back because it is now on.
	b.err = nil

	note, err = toggle()
	if err != nil || a.data.State != device.SwitchStateOn || b.data.State != device.SwitchStateOn || note != "turned on B; A already on" {
		t.Fatalf("retry: A %v, B %v, note %q, err %v", a.data.State, b.data.State, note, err)
	}

	if _, err := toggle(); err != nil || a.data.State != device.SwitchStateOff || b.data.State != device.SwitchStateOff {
		t.Errorf("next run: A %v, B %v, err %v", a.data.State, b.data.State, err)
	}

	if len(a.sets) != 2 {
		t.Errorf("A was sent %v: it should have been turned on once and off once", a.sets)
	}
}

func TestToggle_PutsAllOfThemInTheStateEvenIfSomeoneChangedOneByHand(t *testing.T) {
	a, b := sw("A", device.SwitchStateOff), sw("B", device.SwitchStateOff)
	toggle := Toggle(a, b)

	_, _ = toggle()                      // both on
	b.data.State = device.SwitchStateOff // someone turned B off

	_, _ = toggle() // both off: A is turned off, B was already

	if a.data.State != device.SwitchStateOff || b.data.State != device.SwitchStateOff {
		t.Errorf("A %v, B %v", a.data.State, b.data.State)
	}
}

func TestToggle_PanicsWhenThereIsNothingToToggle(t *testing.T) {
	for name, make := range map[string]func(){
		"none": func() { Toggle() },
		"nil":  func() { Toggle(sw("a", device.SwitchStateOn), nil) },
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

func TestNew_ADescriptionWorkedOutEachTimeItIsAsked(t *testing.T) {
	name := "first"

	a := New("id", "name", trigger.AtTimeOfDay(1, 0), func() (string, error) { return "", nil },
		WithDescription("static"), WithDescriptionFunc(func() string { return "on " + name }))

	if a.Description() != "on first" {
		t.Errorf("description %q", a.Description())
	}

	name = "second"

	if a.Description() != "on second" {
		t.Errorf("description %q", a.Description())
	}
}

func TestSwitchNames(t *testing.T) {
	named := func(name string) device.ISwitch { return &fakeSwitch{data: device.Switch{Name: name}} }

	for name, c := range map[string]struct {
		switches []device.ISwitch
		want     string
	}{
		"none":                  {nil, "no switches"},
		"one":                   {[]device.ISwitch{named("Porch")}, "Porch"},
		"two":                   {[]device.ISwitch{named("Porch"), named("Patio")}, "Porch and Patio"},
		"three":                 {[]device.ISwitch{named("A"), named("B"), named("C")}, "A, B and C"},
		"one that has not said": {[]device.ISwitch{named("A"), named(""), named("C")}, "A, switch 2 and C"},
	} {
		if got := SwitchNames(c.switches...); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}
