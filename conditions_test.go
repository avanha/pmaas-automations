package automations

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/avanha/pmaas-spi/automation"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

type fakeThermometer struct {
	data environment.WirelessThermometer
}

func (f *fakeThermometer) GetWirelessThermometerData() environment.WirelessThermometer { return f.data }

var noon = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func thermometer(name string, fahrenheit float32, reported time.Time) *fakeThermometer {
	return &fakeThermometer{data: environment.WirelessThermometer{
		Name:       name,
		SensorData: environment.SensorData{HasData: true, Temperature: Fahrenheit(fahrenheit).Celsius(), LastUpdateTime: reported},
	}}
}

func withNow(t *testing.T, at time.Time) {
	t.Helper()

	previous := now
	now = func() time.Time { return at }

	t.Cleanup(func() { now = previous })
}

func expect(t *testing.T, what string, condition Condition, wantResult Result, wantReason string) {
	t.Helper()

	result, reason := condition()
	if result != wantResult || reason != wantReason {
		t.Errorf("%s:\n got  %v %q\n want %v %q", what, result, reason, wantResult, wantReason)
	}
}

func TestTemperature(t *testing.T) {
	for _, c := range []struct {
		name       string
		temp       Temperature
		celsius    float32
		fahrenheit float32
	}{
		{"freezing", Fahrenheit(32), 0, 32},
		{"boiling", Fahrenheit(212), 100, 212},
		{"75F", Fahrenheit(75), 23.8889, 75},
		{"minus 40 is the same in both", Fahrenheit(-40), -40, -40},
		{"from celsius", Celsius(20), 20, 68},
	} {
		if got := c.temp.Celsius(); got < c.celsius-0.001 || got > c.celsius+0.001 {
			t.Errorf("%s: %v°C, want %v", c.name, got, c.celsius)
		}

		if got := c.temp.Fahrenheit(); got < c.fahrenheit-0.001 || got > c.fahrenheit+0.001 {
			t.Errorf("%s: %v°F, want %v", c.name, got, c.fahrenheit)
		}
	}

	if got := Fahrenheit(75).String(); got != "75.0°F (23.9°C)" {
		t.Errorf("string %q", got)
	}
}

func TestSwitchIs(t *testing.T) {
	fan := sw("Fan", device.SwitchStateOn)

	expect(t, "on is on", SwitchIs(fan, device.SwitchStateOn), Met, "Fan is On")
	expect(t, "on is not off", SwitchIs(fan, device.SwitchStateOff), Unmet, "Fan is On, not Off")
	expect(t, "unreported", SwitchIs(sw("Fan", device.SwitchStateUnknown), device.SwitchStateOn), Unknown,
		"Fan has not said whether it is on or off")
	expect(t, "nameless", SwitchIs(sw("", device.SwitchStateOff), device.SwitchStateOn), Unmet, "the switch is Off, not On")
}

func TestTemperatureBelowAndAbove(t *testing.T) {
	withNow(t, noon)

	limit := Fahrenheit(75)
	fresh := noon.Add(-time.Minute)

	expect(t, "below, below", TemperatureBelow(thermometer("Room", 74.9, fresh), limit, 10*time.Minute), Met,
		"Room reads 74.9°F (23.8°C), below 75.0°F (23.9°C)")
	expect(t, "above, below", TemperatureBelow(thermometer("Room", 78.1, fresh), limit, 10*time.Minute), Unmet,
		"Room reads 78.1°F (25.6°C), not below 75.0°F (23.9°C)")
	expect(t, "above, above", TemperatureAbove(thermometer("Room", 78.1, fresh), limit, 10*time.Minute), Met,
		"Room reads 78.1°F (25.6°C), above 75.0°F (23.9°C)")
	expect(t, "below, above", TemperatureAbove(thermometer("Room", 74.9, fresh), limit, 10*time.Minute), Unmet,
		"Room reads 74.9°F (23.8°C), not above 75.0°F (23.9°C)")

	// At the limit is neither below nor above.
	at := thermometer("Room", 0, fresh)
	at.data.SensorData.Temperature = limit.Celsius()

	if result, _ := TemperatureBelow(at, limit, 0)(); result != Unmet {
		t.Error("at the limit counted as below")
	}

	if result, _ := TemperatureAbove(at, limit, 0)(); result != Unmet {
		t.Error("at the limit counted as above")
	}
}

func TestTemperature_CanNotBeToldFromAReadingThatIsOldOrMissing(t *testing.T) {
	withNow(t, noon)

	limit := Fahrenheit(75)

	old := thermometer("Room", 60, noon.Add(-25*time.Minute))
	expect(t, "old, below", TemperatureBelow(old, limit, 10*time.Minute), Unknown,
		"Room last reported 25m0s ago, which is too long ago to go by")
	expect(t, "old, above", TemperatureAbove(old, limit, 10*time.Minute), Unknown,
		"Room last reported 25m0s ago, which is too long ago to go by")

	// Exactly as old as allowed is still good.
	if result, _ := TemperatureBelow(thermometer("Room", 60, noon.Add(-10*time.Minute)), limit, 10*time.Minute)(); result != Met {
		t.Error("a reading as old as allowed was refused")
	}

	// No limit on the age.
	if result, _ := TemperatureBelow(old, limit, 0)(); result != Met {
		t.Error("a reading was refused although any age is allowed")
	}

	none := &fakeThermometer{data: environment.WirelessThermometer{Name: "Room"}}
	expect(t, "never reported", TemperatureBelow(none, limit, 10*time.Minute), Unknown, "Room has not reported a temperature")
	expect(t, "nameless", TemperatureAbove(&fakeThermometer{}, limit, 0), Unknown, "the thermometer has not reported a temperature")
}

func TestWarmer(t *testing.T) {
	withNow(t, noon)

	fresh := noon.Add(-time.Minute)
	attic := thermometer("Attic", 94, fresh)
	outside := thermometer("Outside", 80.1, fresh)

	expect(t, "warmer", Warmer(attic, outside, 10*time.Minute), Met,
		"Attic reads 94.0°F (34.4°C), warmer than Outside at 80.1°F (26.7°C)")
	expect(t, "cooler", Warmer(outside, attic, 10*time.Minute), Unmet,
		"Outside reads 80.1°F (26.7°C), not warmer than Attic at 94.0°F (34.4°C)")

	same := thermometer("Same", 94, fresh)
	if result, _ := Warmer(attic, same, 0)(); result != Unmet {
		t.Error("the same temperature counted as warmer")
	}

	stale := thermometer("Outside", 70, noon.Add(-time.Hour))
	silent := &fakeThermometer{data: environment.WirelessThermometer{Name: "Attic"}}

	expect(t, "other stale", Warmer(attic, stale, 10*time.Minute), Unknown,
		"Outside last reported 1h0m0s ago, which is too long ago to go by")
	expect(t, "first silent", Warmer(silent, outside, 10*time.Minute), Unknown, "Attic has not reported a temperature")
	expect(t, "both", Warmer(silent, stale, 10*time.Minute), Unknown,
		"Attic has not reported a temperature; Outside last reported 1h0m0s ago, which is too long ago to go by")
}

func TestAllOf(t *testing.T) {
	met := func() (Result, string) { return Met, "a" }
	met2 := func() (Result, string) { return Met, "b" }
	unmet := func() (Result, string) { return Unmet, "c" }
	unknown := func() (Result, string) { return Unknown, "d" }

	expect(t, "all met", AllOf(met, met2), Met, "a; b")
	expect(t, "one unmet", AllOf(met, unmet, met2), Unmet, "c")
	expect(t, "one unknown", AllOf(met, unknown), Unknown, "d")
	expect(t, "unknown outranks unmet", AllOf(unmet, unknown, met), Unknown, "d")
	expect(t, "all the unmet, all the reasons", AllOf(unmet, unmet, met), Unmet, "c; c")
	expect(t, "none", AllOf(), Met, "")
}

func TestWhen_RunsOnlyWhenEveryConditionIsMetAndOtherwiseIsNotReadyNamingWhatIsNot(t *testing.T) {
	withNow(t, noon)

	fan := sw("Fan", device.SwitchStateOff)
	room := thermometer("Room", 80, noon)
	runs := 0

	run := When(
		func() (string, error) { runs++; return "turned it off", nil },
		SwitchIs(fan, device.SwitchStateOn),
		TemperatureBelow(room, Fahrenheit(75), 10*time.Minute),
	)

	note, err := run()
	reason, notReady := automation.NotReadyReason(err)

	if !notReady || runs != 0 || note != "" ||
		reason != "Fan is Off, not On; Room reads 80.0°F (26.7°C), not below 75.0°F (23.9°C)" {
		t.Fatalf("both unmet: ready? %v, runs %d, note %q, reason %q", !notReady, runs, note, reason)
	}

	// One met.
	fan.data.State = device.SwitchStateOn

	_, err = run()
	if reason, _ := automation.NotReadyReason(err); runs != 0 || reason != "Room reads 80.0°F (26.7°C), not below 75.0°F (23.9°C)" {
		t.Fatalf("one unmet: runs %d, reason %q", runs, reason)
	}

	// Both.
	room.data.SensorData.Temperature = Fahrenheit(70).Celsius()

	note, err = run()
	if err != nil || note != "turned it off" || runs != 1 {
		t.Errorf("both met: note %q, err %v, runs %d", note, err, runs)
	}
}

func TestWhen_AConditionThatCanNotBeToldIsNotMetEither(t *testing.T) {
	runs := 0
	silent := &fakeThermometer{data: environment.WirelessThermometer{Name: "Room"}}

	run := When(func() (string, error) { runs++; return "", nil }, TemperatureBelow(silent, Fahrenheit(75), 0))

	_, err := run()
	if reason, ok := automation.NotReadyReason(err); !ok || runs != 0 || reason != "Room has not reported a temperature" {
		t.Errorf("reason %q, ok %v, runs %d", reason, ok, runs)
	}
}

func TestWhen_AnErrorFromWhatItRunsIsAFailureNotAWait(t *testing.T) {
	boom := errors.New("the plug is offline")
	run := When(func() (string, error) { return "", boom }, func() (Result, string) { return Met, "" })

	_, err := run()

	if !errors.Is(err, boom) || errors.Is(err, automation.ErrNotReady) {
		t.Errorf("got %v", err)
	}
}

func TestWhen_WithNoConditionsJustRuns(t *testing.T) {
	note, err := When(func() (string, error) { return "ok", nil })()

	if note != "ok" || err != nil {
		t.Errorf("got %q, %v", note, err)
	}
}

func TestWhen_WorksWithNewAndTheActionsHere(t *testing.T) {
	fan := sw("Fan", device.SwitchStateOn)

	a := New("fan-off", "Turn off the fan", scriptedTrigger{}, When(TurnOff(fan), SwitchIs(fan, device.SwitchStateOn)))

	note, err := a.Run()
	if err != nil || note != "turned off Fan" || fan.data.State != device.SwitchStateOff {
		t.Fatalf("note %q, err %v, fan %v", note, err, fan.data.State)
	}

	// It's off now, so the condition isn't met: nothing is sent to a fan that is off.
	_, err = a.Run()
	if _, notReady := automation.NotReadyReason(err); !notReady || len(fan.sets) != 1 {
		t.Errorf("err %v, sets %v", err, fan.sets)
	}
}

type scriptedTrigger struct{}

func (scriptedTrigger) Previous(time.Time) (time.Time, bool) { return time.Time{}, false }
func (scriptedTrigger) Next(time.Time) (time.Time, bool)     { return time.Time{}, false }
func (scriptedTrigger) Description() string                  { return "scripted" }

func TestConditions_PanicForWhatCouldNotWork(t *testing.T) {
	ok := func() (string, error) { return "", nil }
	good := func() (Result, string) { return Met, "" }
	fan := sw("Fan", device.SwitchStateOn)
	room := thermometer("Room", 70, noon)

	for name, make := range map[string]func(){
		"when: no run":           func() { When(nil) },
		"when: nil condition":    func() { When(ok, nil) },
		"allof: nil condition":   func() { AllOf(good, nil) },
		"switchis: no switch":    func() { SwitchIs(nil, device.SwitchStateOn) },
		"below: no thermometer":  func() { TemperatureBelow(nil, Fahrenheit(75), 0) },
		"above: no thermometer":  func() { TemperatureAbove(nil, Fahrenheit(75), 0) },
		"warmer: first missing":  func() { Warmer(nil, room, 0) },
		"warmer: second missing": func() { Warmer(room, nil, 0) },
		"follow: no switch":      func() { Follow(nil, good, time.Minute) },
		"follow: no condition":   func() { Follow(fan, nil, time.Minute) },
		"anyof: nil condition":   func() { AnyOf(good, nil) },
		"not: no condition":      func() { Not(nil) },
		"regulate: no switch":    func() { Regulate(nil, good, good, time.Minute) },
		"regulate: no turn on":   func() { Regulate(fan, nil, good, time.Minute) },
		"regulate: no turn off":  func() { Regulate(fan, good, nil, time.Minute) },
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

// timedSwitch is a switch that notes when it changed, as a real one does.
type timedSwitch struct {
	data device.Switch
	err  error
	sets []device.SwitchState
}

func (s *timedSwitch) GetSwitchData() device.Switch { return s.data }

func (s *timedSwitch) SetState(state device.SwitchState) error {
	s.sets = append(s.sets, state)

	if s.err != nil {
		return s.err
	}

	if s.data.State != state {
		s.data.LastChangeTime = now()
	}

	s.data.HasData, s.data.State = true, state

	return nil
}

func newTimedSwitch(state device.SwitchState, changed time.Time) *timedSwitch {
	return &timedSwitch{data: device.Switch{
		Name: "Attic fan", HasData: state != device.SwitchStateUnknown, State: state, LastChangeTime: changed,
	}}
}

func conditionOf(result Result, reason string) Condition {
	return func() (Result, string) { return result, reason }
}

func notReadyReason(t *testing.T, err error) string {
	t.Helper()

	reason, ok := automation.NotReadyReason(err)
	if !ok {
		t.Fatalf("not a not-ready: %v", err)
	}

	return reason
}

func TestFollow_TurnsTheSwitchOnWhileTheConditionIsMetAndOffWhileItIsNot(t *testing.T) {
	withNow(t, noon)

	attic := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))

	note, err := Follow(attic, conditionOf(Met, "it is hot"), 5*time.Minute)()
	if err != nil || note != "turned on Attic fan: it is hot" || attic.data.State != device.SwitchStateOn {
		t.Fatalf("on: note %q, err %v, state %v", note, err, attic.data.State)
	}

	attic.data.LastChangeTime = noon.Add(-time.Hour)

	note, err = Follow(attic, conditionOf(Unmet, "it is cool"), 5*time.Minute)()
	if err != nil || note != "turned off Attic fan: it is cool" || attic.data.State != device.SwitchStateOff {
		t.Fatalf("off: note %q, err %v, state %v", note, err, attic.data.State)
	}
}

func TestFollow_ASwitchAlreadyAsItShouldBeIsLeftAloneQuietly(t *testing.T) {
	withNow(t, noon)

	on := newTimedSwitch(device.SwitchStateOn, noon.Add(-time.Hour))

	note, err := Follow(on, conditionOf(Met, "it is hot"), 5*time.Minute)()
	if reason := notReadyReason(t, err); note != "" || reason != "Attic fan is On: it is hot" || len(on.sets) != 0 {
		t.Errorf("on: note %q, reason %q, sets %v", note, reason, on.sets)
	}

	off := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))

	_, err = Follow(off, conditionOf(Unmet, "it is cool"), 5*time.Minute)()
	if reason := notReadyReason(t, err); reason != "Attic fan is Off: it is cool" || len(off.sets) != 0 {
		t.Errorf("off: reason %q, sets %v", reason, off.sets)
	}
}

func TestFollow_AConditionThatCanNotBeToldLeavesTheSwitchWhateverItIs(t *testing.T) {
	withNow(t, noon)

	for _, state := range []device.SwitchState{device.SwitchStateOn, device.SwitchStateOff, device.SwitchStateUnknown} {
		fan := newTimedSwitch(state, noon.Add(-time.Hour))

		_, err := Follow(fan, conditionOf(Unknown, "the thermometer went quiet"), 5*time.Minute)()

		if reason := notReadyReason(t, err); reason != "leaving Attic fan as it is: the thermometer went quiet" || len(fan.sets) != 0 {
			t.Errorf("%v: reason %q, sets %v", state, reason, fan.sets)
		}
	}
}

func TestFollow_DoesNotChangeTheSwitchMoreOftenThanTheInterval(t *testing.T) {
	fan := newTimedSwitch(device.SwitchStateOff, noon.Add(-2*time.Minute))
	follow := Follow(fan, conditionOf(Met, "hot"), 5*time.Minute)

	withNow(t, noon)

	_, err := follow()
	if reason := notReadyReason(t, err); len(fan.sets) != 0 ||
		reason != "Attic fan should be On (hot), but it changed 2m0s ago, and is not changed again until 5m0s have passed" {
		t.Errorf("reason %q, sets %v", reason, fan.sets)
	}

	// Just short of the interval.
	withNow(t, noon.Add(2*time.Minute+59*time.Second))

	if _, err := follow(); len(fan.sets) != 0 || err == nil {
		t.Errorf("changed after 4m59s: %v, %v", fan.sets, err)
	}

	// Exactly the interval.
	withNow(t, noon.Add(3*time.Minute))

	if note, err := follow(); err != nil || len(fan.sets) != 1 || note == "" {
		t.Errorf("not changed after exactly the interval: %q %v %v", note, err, fan.sets)
	}
}

func TestFollow_ASwitchThatHasNeverChangedOrNeverReportedIsNotHeldBack(t *testing.T) {
	withNow(t, noon)

	neverChanged := newTimedSwitch(device.SwitchStateOff, time.Time{})

	if _, err := Follow(neverChanged, conditionOf(Met, "hot"), time.Hour)(); err != nil || len(neverChanged.sets) != 1 {
		t.Errorf("never changed: %v, sets %v", err, neverChanged.sets)
	}

	// By the SPI's contract a switch that has never reported has never been seen to change.
	unreported := newTimedSwitch(device.SwitchStateUnknown, time.Time{})

	if _, err := Follow(unreported, conditionOf(Unmet, "cool"), time.Hour)(); err != nil || len(unreported.sets) != 1 ||
		unreported.sets[0] != device.SwitchStateOff {
		t.Errorf("never reported: %v, sets %v", err, unreported.sets)
	}
}

func TestFollow_AFailureToChangeTheSwitchIsAFailureNotAWait(t *testing.T) {
	withNow(t, noon)

	boom := errors.New("offline")
	fan := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))
	fan.err = boom

	_, err := Follow(fan, conditionOf(Met, "hot"), time.Minute)()

	if !errors.Is(err, boom) || errors.Is(err, automation.ErrNotReady) || !strings.Contains(err.Error(), "Attic fan") {
		t.Errorf("got %v", err)
	}
}

// Walks an hour in which the attic hovers around the limit and the outside is as warm as the attic for a while, one
// minute at a time, and checks the fan is never changed more often than every five minutes, is on when it should be
// and off when it should be once it is allowed to be, and that holding it off for the interval never leaves it wrong
// for longer than that.
func TestFollow_AnHourOfATemperatureHoveringAroundTheLimit(t *testing.T) {
	start := noon
	attic := thermometer("Attic", 85, start)
	outside := thermometer("Outside", 80, start)
	fan := newTimedSwitch(device.SwitchStateOff, time.Time{})

	// 12:00..12:59, the attic temperature at each minute: it crosses 90 back and forth.
	atticAt := func(minute int) float32 {
		switch {
		case minute < 10:
			return 85
		case minute < 20:
			return 91 + float32(minute%2)*2 // 91, 93, 91, 93 ...
		case minute < 30:
			return 89 + float32(minute%2)*2 // 89, 91, 89, 91 ... flaps around the limit
		case minute < 40:
			return 95
		case minute < 50:
			return 88
		default:
			return 92
		}
	}

	condition := AllOf(
		TemperatureAbove(attic, Fahrenheit(90), 10*time.Minute),
		Warmer(attic, outside, 10*time.Minute),
	)
	follow := Follow(fan, condition, 5*time.Minute)

	var changes []int

	for minute := 0; minute < 60; minute++ {
		at := start.Add(time.Duration(minute) * time.Minute)
		withNow(t, at)

		attic.data.SensorData = environment.SensorData{HasData: true, Temperature: Fahrenheit(atticAt(minute)).Celsius(), LastUpdateTime: at}
		outside.data.SensorData.LastUpdateTime = at

		before := len(fan.sets)

		if _, err := follow(); err != nil {
			if _, notReady := automation.NotReadyReason(err); !notReady {
				t.Fatalf("minute %d: %v", minute, err)
			}
		}

		if len(fan.sets) != before {
			changes = append(changes, minute)
		}
	}

	for i := 1; i < len(changes); i++ {
		if changes[i]-changes[i-1] < 5 {
			t.Errorf("changed at minute %d and again at %d: %v", changes[i-1], changes[i], changes)
		}
	}

	// It never went more than a handful of times in the hour, however much the temperature flapped.
	if len(changes) < 3 || len(changes) > 12 {
		t.Errorf("changed %d times: %v", len(changes), changes)
	}

	// 12:50 on: the attic is 92 and the outside 80, so the fan should be on, and has had time to be.
	if fan.data.State != device.SwitchStateOn {
		t.Errorf("fan ended %v, changes %v", fan.data.State, changes)
	}
}

func TestFollow_AnOutsideThatIsAsWarmAsTheAtticKeepsTheFanOff(t *testing.T) {
	withNow(t, noon)

	attic := thermometer("Attic", 95, noon)
	outside := thermometer("Outside", 97, noon)
	fan := newTimedSwitch(device.SwitchStateOff, time.Time{})

	follow := Follow(fan, AllOf(TemperatureAbove(attic, Fahrenheit(90), 0), Warmer(attic, outside, 0)), 5*time.Minute)

	_, err := follow()
	if reason := notReadyReason(t, err); len(fan.sets) != 0 || !strings.Contains(reason, "Attic fan is Off") ||
		!strings.Contains(reason, "not warmer than Outside") {
		t.Errorf("reason %q, sets %v", reason, fan.sets)
	}

	// The outside cools below the attic.
	outside.data.SensorData.Temperature = Fahrenheit(80).Celsius()

	if _, err := follow(); err != nil || len(fan.sets) != 1 || fan.sets[0] != device.SwitchStateOn {
		t.Errorf("err %v, sets %v", err, fan.sets)
	}
}

func TestNot(t *testing.T) {
	expect(t, "met", Not(conditionOf(Met, "hot")), Unmet, "hot")
	expect(t, "unmet", Not(conditionOf(Unmet, "cool")), Met, "cool")
	expect(t, "unknown", Not(conditionOf(Unknown, "quiet")), Unknown, "quiet")
}

func TestAnyOf(t *testing.T) {
	met := conditionOf(Met, "a")
	met2 := conditionOf(Met, "b")
	unmet := conditionOf(Unmet, "c")
	unknown := conditionOf(Unknown, "d")

	expect(t, "one met", AnyOf(unmet, met), Met, "a")
	expect(t, "all the met, all the reasons", AnyOf(met, unmet, met2), Met, "a; b")
	expect(t, "met outranks unknown", AnyOf(unknown, met), Met, "a")
	expect(t, "unknown outranks unmet", AnyOf(unmet, unknown), Unknown, "d")
	expect(t, "none met", AnyOf(unmet, unmet), Unmet, "c; c")
	expect(t, "none at all", AnyOf(), Unmet, "")
}

func TestRegulate_StaysAsItIsBetweenTheTwoConditionsWhicheverWayItIs(t *testing.T) {
	withNow(t, noon)

	// Neither is met: the temperature is between the two limits.
	onWhenHot := conditionOf(Unmet, "89 is not above 90")
	offWhenCool := conditionOf(Unmet, "89 is not below 88")

	for _, state := range []device.SwitchState{device.SwitchStateOn, device.SwitchStateOff} {
		fan := newTimedSwitch(state, noon.Add(-time.Hour))

		_, err := Regulate(fan, onWhenHot, offWhenCool, 5*time.Minute)()
		reason := notReadyReason(t, err)

		want := "Attic fan is On: 89 is not below 88"
		if state == device.SwitchStateOff {
			want = "Attic fan is Off: 89 is not above 90"
		}

		if reason != want || len(fan.sets) != 0 {
			t.Errorf("%v: reason %q, sets %v", state, reason, fan.sets)
		}
	}
}

func TestRegulate_OnlyTheConditionForTheWayTheSwitchCouldGoMatters(t *testing.T) {
	withNow(t, noon)

	met, unmet, unknown := conditionOf(Met, "m"), conditionOf(Unmet, "u"), conditionOf(Unknown, "k")

	// Off: turning off being met is nothing to do, and turning off being unknown doesn't matter.
	off := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))

	for _, turnOff := range []Condition{met, unknown} {
		if _, err := Regulate(off, unmet, turnOff, 5*time.Minute)(); notReadyReason(t, err) != "Attic fan is Off: u" || len(off.sets) != 0 {
			t.Errorf("an off switch: %v %v", err, off.sets)
		}
	}

	// On: turning on being met is nothing to do, and turning on being unknown doesn't matter.
	on := newTimedSwitch(device.SwitchStateOn, noon.Add(-time.Hour))

	for _, turnOn := range []Condition{met, unknown} {
		if _, err := Regulate(on, turnOn, unmet, 5*time.Minute)(); notReadyReason(t, err) != "Attic fan is On: u" || len(on.sets) != 0 {
			t.Errorf("an on switch: %v %v", err, on.sets)
		}
	}

	// And the one that matters being met is enough, whatever the other one can or can't tell.
	turned := newTimedSwitch(device.SwitchStateOn, noon.Add(-time.Hour))

	if _, err := Regulate(turned, unknown, met, 5*time.Minute)(); err != nil || len(turned.sets) != 1 || turned.sets[0] != device.SwitchStateOff {
		t.Errorf("on, turn off met, turn on unknown: %v %v", err, turned.sets)
	}

	turnedOn := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))

	if _, err := Regulate(turnedOn, met, unknown, 5*time.Minute)(); err != nil || len(turnedOn.sets) != 1 || turnedOn.sets[0] != device.SwitchStateOn {
		t.Errorf("off, turn on met, turn off unknown: %v %v", err, turnedOn.sets)
	}

	// But the one that matters, unknown, holds it.
	if _, err := Regulate(off, unknown, met, 5*time.Minute)(); notReadyReason(t, err) != "leaving Attic fan as it is: k" || len(off.sets) != 0 {
		t.Errorf("off, turn on unknown: %v %v", err, off.sets)
	}

	if _, err := Regulate(on, met, unknown, 5*time.Minute)(); notReadyReason(t, err) != "leaving Attic fan as it is: k" || len(on.sets) != 0 {
		t.Errorf("on, turn off unknown: %v %v", err, on.sets)
	}
}

func TestRegulate_TurnsOnWhenTurnOnIsMetAndOffWhenTurnOffIsMet(t *testing.T) {
	withNow(t, noon)

	fan := newTimedSwitch(device.SwitchStateOff, noon.Add(-time.Hour))

	note, err := Regulate(fan, conditionOf(Met, "hot"), conditionOf(Unmet, "x"), 5*time.Minute)()
	if err != nil || note != "turned on Attic fan: hot" || fan.data.State != device.SwitchStateOn {
		t.Fatalf("on: %q %v %v", note, err, fan.data.State)
	}

	fan.data.LastChangeTime = noon.Add(-time.Hour)

	note, err = Regulate(fan, conditionOf(Unmet, "x"), conditionOf(Met, "cool"), 5*time.Minute)()
	if err != nil || note != "turned off Attic fan: cool" || fan.data.State != device.SwitchStateOff {
		t.Fatalf("off: %q %v %v", note, err, fan.data.State)
	}
}

func TestRegulate_ASwitchThatHasNotReportedIsTurnedOnIfThatIsCalledForAndOtherwiseOffIfThatIs(t *testing.T) {
	withNow(t, noon)

	met, unmet, unknown := conditionOf(Met, "m"), conditionOf(Unmet, "u"), conditionOf(Unknown, "k")

	for name, c := range map[string]struct {
		turnOn, turnOff Condition
		want            []device.SwitchState
		reason          string
	}{
		"turn on met":                              {met, unmet, []device.SwitchState{device.SwitchStateOn}, ""},
		"both met, on first":                       {met, met, []device.SwitchState{device.SwitchStateOn}, ""},
		"only turn off met":                        {unmet, met, []device.SwitchState{device.SwitchStateOff}, ""},
		"turn on unknown":                          {unknown, unmet, nil, "leaving Attic fan as it is: k"},
		"turn off unknown":                         {unmet, unknown, nil, "leaving Attic fan as it is: k"},
		"neither met, both known":                  {unmet, unmet, nil, "Attic fan has not said whether it is on or off, and nothing says to change it: u; u"},
		"turn off met outranks an unknown turn on": {unknown, met, []device.SwitchState{device.SwitchStateOff}, ""},
	} {
		fan := newTimedSwitch(device.SwitchStateUnknown, time.Time{})

		_, err := Regulate(fan, c.turnOn, c.turnOff, 5*time.Minute)()

		if c.want == nil {
			if reason := notReadyReason(t, err); reason != c.reason || len(fan.sets) != 0 {
				t.Errorf("%s: reason %q, sets %v", name, reason, fan.sets)
			}

			continue
		}

		if err != nil || len(fan.sets) != 1 || fan.sets[0] != c.want[0] {
			t.Errorf("%s: err %v, sets %v", name, err, fan.sets)
		}
	}
}

func TestRegulate_DoesNotChangeTheSwitchEitherWayMoreOftenThanTheInterval(t *testing.T) {
	for _, state := range []device.SwitchState{device.SwitchStateOn, device.SwitchStateOff} {
		withNow(t, noon)

		fan := newTimedSwitch(state, noon.Add(-time.Minute))

		turnOn, turnOff := conditionOf(Met, "hot"), conditionOf(Met, "cool")
		regulate := Regulate(fan, turnOn, turnOff, 5*time.Minute)

		if _, err := regulate(); notReadyReason(t, err) == "" || len(fan.sets) != 0 {
			t.Fatalf("%v: changed after a minute: %v", state, fan.sets)
		}

		// It changed a minute before noon, so four minutes less a second after noon is a second short of five.
		withNow(t, noon.Add(4*time.Minute-time.Second))

		if _, err := regulate(); len(fan.sets) != 0 || err == nil {
			t.Fatalf("%v: changed a second short of the interval: %v", state, fan.sets)
		}

		withNow(t, noon.Add(4*time.Minute))

		if _, err := regulate(); err != nil || len(fan.sets) != 1 {
			t.Fatalf("%v: not changed at the interval: %v %v", state, err, fan.sets)
		}
	}
}

// The attic hovers a degree and a half either side of 90 for an hour. With one limit the fan would be changed as often as
// the debounce allows; with a gap between turning on at 90 and turning off at 88 it comes on once and stays on.
func TestRegulate_AWobbleAcrossTheLimitDoesNotMakeTheFanCycleWhenThereIsAGap(t *testing.T) {
	start := noon

	run := func(withGap bool) int {
		attic := thermometer("Attic", 85, start)
		fan := newTimedSwitch(device.SwitchStateOff, time.Time{})

		turnOn := TemperatureAbove(attic, Fahrenheit(90), 10*time.Minute)

		turnOff := Not(turnOn)
		if withGap {
			turnOff = TemperatureBelow(attic, Fahrenheit(88), 10*time.Minute)
		}

		regulate := Regulate(fan, turnOn, turnOff, 5*time.Minute)

		for minute := 0; minute < 60; minute++ {
			at := start.Add(time.Duration(minute) * time.Minute)
			withNow(t, at)

			reading := float32(88.5) // below 90 but not below 88
			if minute%2 == 1 || minute == 0 {
				reading = 91.5
			}

			attic.data.SensorData = environment.SensorData{HasData: true, Temperature: Fahrenheit(reading).Celsius(), LastUpdateTime: at}

			_, _ = regulate()
		}

		return len(fan.sets)
	}

	if without := run(false); without < 8 {
		t.Fatalf("with one limit the fan was changed %d times in the hour, which is not the cycling this test is about", without)
	}

	if with := run(true); with != 1 {
		t.Errorf("with a gap the fan was changed %d times, want once", with)
	}
}

func TestRegulate_ARealCycleThroughTheGapChangesTheFanAtTheLimitsOnly(t *testing.T) {
	start := noon
	attic := thermometer("Attic", 85, start)
	fan := newTimedSwitch(device.SwitchStateOff, time.Time{})

	regulate := Regulate(fan,
		TemperatureAbove(attic, Fahrenheit(90), 10*time.Minute),
		TemperatureBelow(attic, Fahrenheit(88), 10*time.Minute),
		5*time.Minute)

	// A reading every ten minutes: heating through the gap, then cooling back through it.
	var states []string

	for step, reading := range []float32{85, 89, 91, 89.5, 88.5, 87, 89, 91, 92, 89, 87.5} {
		at := start.Add(time.Duration(step) * 10 * time.Minute)
		withNow(t, at)

		attic.data.SensorData = environment.SensorData{HasData: true, Temperature: Fahrenheit(reading).Celsius(), LastUpdateTime: at}

		_, _ = regulate()

		states = append(states, fan.data.State.String())
	}

	want := "Off Off On On On Off Off On On On Off"
	if got := strings.Join(states, " "); got != want {
		t.Errorf("fan states\n got  %s\n want %s", got, want)
	}
}
