package trigger

import (
	"fmt"
	"strings"

	"github.com/avanha/pmaas-spi/automation"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/events"
)

// EventTrigger is what the server asks of every event to know whether an event-driven automation is due. It's defined in the
// SPI, so that the server doesn't have to know about this package.
type EventTrigger = automation.EventTrigger

type eventTrigger struct {
	description func() string
	matches     func(info *events.EventInfo) bool
}

func (t eventTrigger) Matches(info *events.EventInfo) bool { return t.matches(info) }
func (t eventTrigger) Description() string                 { return t.description() }

// OnEvent is due when the function says an event is one. It is asked of every event the server broadcasts, on the event manager's
// goroutine, so it must be quick and never block. The description is what is shown for when it is due. It panics if there is
// neither.
func OnEvent(description string, matches func(info *events.EventInfo) bool) EventTrigger {
	if description == "" || matches == nil {
		panic("trigger.OnEvent: it needs a description and a function")
	}

	return eventTrigger{description: func() string { return description }, matches: matches}
}

// Button is a button an automation can wait for: a device.IButton whose events can be told from another's. The beacon buttons
// of the bluetooth plugin are.
type Button interface {
	device.IButton

	// EntityId is the ID the button is registered under, which its events are from, or empty before it is.
	EntityId() string
}

// ButtonPressed is due when the button is pressed in one of the ways given, or in any way if none are. It is due the moment
// the press is announced.
//
// It panics if there is no button, or a press that is not one of single, double, triple and long, like regexp.MustCompile does
// for a pattern that can't be.
func ButtonPressed(button Button, presses ...device.ButtonPress) EventTrigger {
	if button == nil {
		panic("trigger.ButtonPressed: no button")
	}

	for _, press := range presses {
		if press < device.PressSingle || press > device.PressLong {
			panic(fmt.Sprintf("trigger.ButtonPressed: %d is not a way of pressing a button", int(press)))
		}
	}

	return eventTrigger{
		description: func() string { return describeButton(button.GetButtonData().Name, presses) },
		matches: func(info *events.EventInfo) bool {
			event, ok := info.Event.(device.ButtonPressedEvent)
			if !ok {
				return false
			}

			id := button.EntityId()
			if id == "" || event.Id != id {
				return false
			}

			if len(presses) == 0 {
				return true
			}

			for _, press := range presses {
				if press == event.Press {
					return true
				}
			}

			return false
		},
	}
}

// describeButton says when it is due in words: "when the Couch button is double-clicked, or long-pressed".
func describeButton(name string, presses []device.ButtonPress) string {
	if name == "" {
		name = "the button"
	}

	if len(presses) == 0 {
		return fmt.Sprintf("when %s is pressed", name)
	}

	words := make([]string, len(presses))
	for i, press := range presses {
		switch press {
		case device.PressSingle:
			words[i] = "single-clicked"
		case device.PressDouble:
			words[i] = "double-clicked"
		case device.PressTriple:
			words[i] = "triple-clicked"
		default:
			words[i] = "long-pressed"
		}
	}

	return fmt.Sprintf("when %s is %s", name, strings.Join(words, ", or "))
}
