package trigger

import (
	"testing"

	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/events"
)

type fakeButton struct {
	id   string
	name string
}

func (b *fakeButton) GetButtonData() device.Button { return device.Button{Name: b.name} }
func (b *fakeButton) EntityId() string             { return b.id }

func pressed(id string, press device.ButtonPress) *events.EventInfo {
	return &events.EventInfo{Event: device.ButtonPressedEvent{EntityEvent: events.EntityEvent{Id: id}, Press: press}}
}

func TestButtonPressed_AnyPress(t *testing.T) {
	button := &fakeButton{id: "button_1", name: "Couch button"}
	trigger := ButtonPressed(button)

	for _, press := range []device.ButtonPress{device.PressSingle, device.PressDouble, device.PressTriple, device.PressLong} {
		if !trigger.Matches(pressed("button_1", press)) {
			t.Errorf("a %s press did not match", press)
		}
	}

	if trigger.Description() != "when Couch button is pressed" {
		t.Errorf("description %q", trigger.Description())
	}
}

func TestButtonPressed_OnlyTheWaysGiven(t *testing.T) {
	button := &fakeButton{id: "button_1", name: "Couch button"}
	trigger := ButtonPressed(button, device.PressDouble, device.PressLong)

	for press, want := range map[device.ButtonPress]bool{
		device.PressSingle: false, device.PressDouble: true, device.PressTriple: false, device.PressLong: true,
	} {
		if got := trigger.Matches(pressed("button_1", press)); got != want {
			t.Errorf("%s: %v, want %v", press, got, want)
		}
	}

	if trigger.Description() != "when Couch button is double-clicked, or long-pressed" {
		t.Errorf("description %q", trigger.Description())
	}
}

func TestButtonPressed_OnlyThatButtonAndOnlyPresses(t *testing.T) {
	button := &fakeButton{id: "button_1", name: "Couch button"}
	trigger := ButtonPressed(button)

	if trigger.Matches(pressed("button_2", device.PressSingle)) {
		t.Error("another button's press matched")
	}

	for _, other := range []any{"a string", nil, events.EntityStateChangedEvent{EntityEvent: events.EntityEvent{Id: "button_1"}}} {
		if trigger.Matches(&events.EventInfo{Event: other}) {
			t.Errorf("%T matched", other)
		}
	}
}

func TestButtonPressed_ABeforeItIsRegisteredMatchesNothingEvenAnEventWithNoID(t *testing.T) {
	button := &fakeButton{name: "Couch button"}
	trigger := ButtonPressed(button)

	if trigger.Matches(pressed("", device.PressSingle)) {
		t.Error("a button with no ID matched an event with none")
	}

	// And it matches once it has one, which is read at each event.
	button.id = "button_7"

	if !trigger.Matches(pressed("button_7", device.PressSingle)) {
		t.Error("it did not match once the button was registered")
	}
}

func TestButtonPressed_TheNameIsReadWhenAsked(t *testing.T) {
	button := &fakeButton{id: "b"}
	trigger := ButtonPressed(button, device.PressTriple)

	if trigger.Description() != "when the button is triple-clicked" {
		t.Errorf("description %q", trigger.Description())
	}

	button.name = "Kitchen"

	if trigger.Description() != "when Kitchen is triple-clicked" {
		t.Errorf("description %q", trigger.Description())
	}
}

func TestButtonPressed_MistakesPanic(t *testing.T) {
	for name, mistake := range map[string]func(){
		"no button":          func() { ButtonPressed(nil) },
		"an unknown press":   func() { ButtonPressed(&fakeButton{}, device.PressUnknown) },
		"a press that isn't": func() { ButtonPressed(&fakeButton{}, device.ButtonPress(9)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()

			mistake()
		}()
	}
}

func TestOnEvent(t *testing.T) {
	trigger := OnEvent("when a string comes", func(info *events.EventInfo) bool { _, ok := info.Event.(string); return ok })

	if !trigger.Matches(&events.EventInfo{Event: "x"}) || trigger.Matches(&events.EventInfo{Event: 1}) || trigger.Description() != "when a string comes" {
		t.Error("OnEvent")
	}

	for name, mistake := range map[string]func(){
		"no description": func() { OnEvent("", func(*events.EventInfo) bool { return true }) },
		"no function":    func() { OnEvent("x", nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()

			mistake()
		}()
	}
}
