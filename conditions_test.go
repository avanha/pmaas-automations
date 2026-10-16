package automations

import (
	"errors"
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

func thermometer(celsius float32, reported time.Time) *fakeThermometer {
	return &fakeThermometer{data: environment.WirelessThermometer{
		Name:       "Room",
		SensorData: environment.SensorData{HasData: true, Temperature: celsius, LastUpdateTime: reported},
	}}
}

func withNow(t *testing.T, at time.Time) {
	t.Helper()

	previous := now
	now = func() time.Time { return at }

	t.Cleanup(func() { now = previous })
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

	if met, reason := SwitchIs(fan, device.SwitchStateOn)(); !met || reason != "" {
		t.Errorf("on is on: %v %q", met, reason)
	}

	if met, reason := SwitchIs(fan, device.SwitchStateOff)(); met || reason != "Fan is On, not Off" {
		t.Errorf("on is not off: %v %q", met, reason)
	}

	unreported := sw("Fan", device.SwitchStateUnknown)
	if met, reason := SwitchIs(unreported, device.SwitchStateOn)(); met || reason != "Fan has not said whether it is on or off" {
		t.Errorf("unreported: %v %q", met, reason)
	}

	nameless := sw("", device.SwitchStateOff)
	if _, reason := SwitchIs(nameless, device.SwitchStateOn)(); reason != "the switch is Off, not On" {
		t.Errorf("nameless: %q", reason)
	}
}

func TestTemperatureBelow(t *testing.T) {
	withNow(t, noon)

	limit := Fahrenheit(75)
	fresh := noon.Add(-time.Minute)

	if met, reason := TemperatureBelow(thermometer(Fahrenheit(74.9).Celsius(), fresh), limit, 10*time.Minute)(); !met || reason != "" {
		t.Errorf("below: %v %q", met, reason)
	}

	met, reason := TemperatureBelow(thermometer(Fahrenheit(78.1).Celsius(), fresh), limit, 10*time.Minute)()
	if met || reason != "Room reads 78.1°F (25.6°C), not below 75.0°F (23.9°C)" {
		t.Errorf("above: %v %q", met, reason)
	}

	// At the limit isn't below it.
	if met, _ := TemperatureBelow(thermometer(limit.Celsius(), fresh), limit, 10*time.Minute)(); met {
		t.Error("at the limit counted as below")
	}
}

func TestTemperatureBelow_DoesNotGoByAReadingThatIsOldOrMissing(t *testing.T) {
	withNow(t, noon)

	limit := Fahrenheit(75)
	cold := Fahrenheit(60).Celsius()

	old := thermometer(cold, noon.Add(-25*time.Minute))
	if met, reason := TemperatureBelow(old, limit, 10*time.Minute)(); met || reason != "Room last reported 25m0s ago, which is too long ago to go by" {
		t.Errorf("old: %v %q", met, reason)
	}

	// Exactly as old as allowed is still good.
	edge := thermometer(cold, noon.Add(-10*time.Minute))
	if met, _ := TemperatureBelow(edge, limit, 10*time.Minute)(); !met {
		t.Error("a reading as old as allowed was refused")
	}

	// No limit on the age.
	if met, _ := TemperatureBelow(old, limit, 0)(); !met {
		t.Error("a reading was refused although any age is allowed")
	}

	none := &fakeThermometer{data: environment.WirelessThermometer{Name: "Room"}}
	if met, reason := TemperatureBelow(none, limit, 10*time.Minute)(); met || reason != "Room has not reported a temperature" {
		t.Errorf("never reported: %v %q", met, reason)
	}

	nameless := &fakeThermometer{}
	if _, reason := TemperatureBelow(nameless, limit, 0)(); reason != "the thermometer has not reported a temperature" {
		t.Errorf("nameless: %q", reason)
	}
}

func TestWhen_RunsOnlyWhenEveryConditionIsMetAndOtherwiseIsNotReadyNamingWhatIsNot(t *testing.T) {
	withNow(t, noon)

	fan := sw("Fan", device.SwitchStateOff)
	room := thermometer(Fahrenheit(80).Celsius(), noon)
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

func TestWhen_AnErrorFromWhatItRunsIsAFailureNotAWait(t *testing.T) {
	boom := errors.New("the plug is offline")
	run := When(func() (string, error) { return "", boom }, func() (bool, string) { return true, "" })

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

func TestWhen_PanicsForWhatCouldNotWork(t *testing.T) {
	ok := func() (string, error) { return "", nil }
	for name, make := range map[string]func(){
		"no run":         func() { When(nil) },
		"nil condition":  func() { When(ok, nil) },
		"no switch":      func() { SwitchIs(nil, device.SwitchStateOn) },
		"no thermometer": func() { TemperatureBelow(nil, Fahrenheit(75), 0) },
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
