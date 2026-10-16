package automations

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/avanha/pmaas-spi/alert"
	"github.com/avanha/pmaas-spi/automation"
	"github.com/avanha/pmaas-spi/device"
	"github.com/avanha/pmaas-spi/environment"
)

type recordingBroadcaster struct {
	events []any
	ids    []string
	err    error
}

func (r *recordingBroadcaster) BroadcastEvent(id string, event any) error {
	if r.err != nil {
		return r.err
	}

	r.ids = append(r.ids, id)
	r.events = append(r.events, event)

	return nil
}

func (r *recordingBroadcaster) raised() []alert.RaisedEvent {
	var result []alert.RaisedEvent

	for _, e := range r.events {
		if raised, ok := e.(alert.RaisedEvent); ok {
			result = append(result, raised)
		}
	}

	return result
}

func (r *recordingBroadcaster) cleared() []alert.ClearedEvent {
	var result []alert.ClearedEvent

	for _, e := range r.events {
		if cleared, ok := e.(alert.ClearedEvent); ok {
			result = append(result, cleared)
		}
	}

	return result
}

// scriptedCondition is a condition whose answers are the test's to change.
type scriptedCondition struct {
	result Result
	reason string
}

func (c *scriptedCondition) condition() Condition {
	return func() (Result, string) { return c.result, c.reason }
}

func TestWatchAlert_RaisesWhenMetClearsWhenNotAndLeavesItWhenItCanNotBeTold(t *testing.T) {
	withNow(t, noon)

	alerts := &recordingBroadcaster{}
	c := &scriptedCondition{Unmet, "the door is closed"}
	watch := WatchAlert(alerts, "garage-door-open", "Garage door open", alert.Always(alert.SeverityWarning), c.condition())

	// Nothing to raise, and nothing to clear.
	_, err := watch()
	if reason, ok := automation.NotReadyReason(err); !ok || reason != "no alert: the door is closed" || len(alerts.events) != 0 {
		t.Fatalf("unmet at first: %v %v", err, alerts.events)
	}

	// Raised.
	c.result, c.reason = Met, "the door has been open for 5m"

	note, err := watch()
	if err != nil || note != "raised a warning alert: the door has been open for 5m" {
		t.Fatalf("met: %q %v", note, err)
	}

	raised := alerts.raised()
	if len(raised) != 1 || raised[0].Source != "automations" || raised[0].Key != "watch:garage-door-open" ||
		raised[0].Severity != alert.SeverityWarning || raised[0].Title != "Garage door open" ||
		raised[0].Message != "the door has been open for 5m" {
		t.Fatalf("raised %+v", raised)
	}

	// Still met: nothing more is sent.
	_, err = watch()
	if reason, _ := automation.NotReadyReason(err); reason != "the alert is on, as warning: the door has been open for 5m" || len(alerts.events) != 1 {
		t.Errorf("met again: %v, %d events", err, len(alerts.events))
	}

	// Can't be told: left as it is.
	c.result, c.reason = Unknown, "the sensor is offline"

	_, err = watch()
	if reason, _ := automation.NotReadyReason(err); reason != "leaving the alert as it is: the sensor is offline" || len(alerts.events) != 1 {
		t.Errorf("unknown: %v, %d events", err, len(alerts.events))
	}

	// Closed: cleared, once.
	c.result, c.reason = Unmet, "the door is closed"

	note, err = watch()
	if err != nil || note != "cleared the alert: the door is closed" {
		t.Fatalf("cleared: %q %v", note, err)
	}

	if cleared := alerts.cleared(); len(cleared) != 1 || cleared[0].Source != "automations" || cleared[0].Key != "watch:garage-door-open" {
		t.Errorf("cleared %+v", cleared)
	}

	if _, err = watch(); err == nil || len(alerts.events) != 2 {
		t.Errorf("after clearing: %v, %d events", err, len(alerts.events))
	}
}

func TestWatchAlert_ChangesTheSeverityOfTheAlertItHasRaisedWithoutRaisingAnother(t *testing.T) {
	at := noon
	withNow(t, at)

	alerts := &recordingBroadcaster{}
	c := &scriptedCondition{Met, "open"}
	severity := alert.ByTimeOfDay(alert.SeverityWarning, 7, 0, 23, 0, alert.SeverityCritical)
	watch := WatchAlert(alerts, "door", "Door open", severity, c.condition())

	_, _ = watch()

	// An hour before 11pm, then at 11pm: it escalates, once.
	withNow(t, time.Date(2026, 10, 16, 22, 59, 0, 0, time.UTC))

	if _, err := watch(); err == nil {
		t.Fatal("it did something at 10:59pm")
	}

	withNow(t, time.Date(2026, 10, 16, 23, 0, 0, 0, time.UTC))

	note, err := watch()
	if err != nil || note != "changed the alert to critical: open" {
		t.Fatalf("at 11pm: %q %v", note, err)
	}

	withNow(t, time.Date(2026, 10, 16, 23, 30, 0, 0, time.UTC))

	if _, err := watch(); err == nil {
		t.Fatal("it did something at 11:30pm")
	}

	// And eases at 7am.
	withNow(t, time.Date(2026, 10, 17, 7, 0, 0, 0, time.UTC))

	if note, err := watch(); err != nil || note != "changed the alert to warning: open" {
		t.Fatalf("at 7am: %q %v", note, err)
	}

	var severities []alert.Severity
	for _, raised := range alerts.raised() {
		severities = append(severities, raised.Severity)
	}

	if len(severities) != 3 || severities[0] != alert.SeverityWarning || severities[1] != alert.SeverityCritical || severities[2] != alert.SeverityWarning {
		t.Errorf("severities %v", severities)
	}

	if len(alerts.cleared()) != 0 {
		t.Error("the alert was cleared on the way")
	}
}

func TestWatchAlert_ASeverityThatIsNotOneIsAnInfo(t *testing.T) {
	withNow(t, noon)

	alerts := &recordingBroadcaster{}
	c := &scriptedCondition{Met, "open"}

	_, _ = WatchAlert(alerts, "door", "Door open", alert.Always(alert.Severity(99)), c.condition())()

	if raised := alerts.raised(); len(raised) != 1 || raised[0].Severity != alert.SeverityInfo {
		t.Errorf("raised %+v", raised)
	}
}

func TestWatchAlert_AFailureToBroadcastIsAFailureAndItIsTriedAgain(t *testing.T) {
	withNow(t, noon)

	alerts := &recordingBroadcaster{err: errors.New("the event manager is not accepting requests")}
	c := &scriptedCondition{Met, "open"}
	watch := WatchAlert(alerts, "door", "Door open", alert.Always(alert.SeverityWarning), c.condition())

	_, err := watch()
	if err == nil || errors.Is(err, automation.ErrNotReady) || !strings.Contains(err.Error(), "unable to raise") {
		t.Fatalf("got %v", err)
	}

	// It works again: raised now, since it never was.
	alerts.err = nil

	if note, err := watch(); err != nil || !strings.HasPrefix(note, "raised a warning alert") || len(alerts.raised()) != 1 {
		t.Fatalf("retry: %q %v", note, err)
	}

	// And clearing that fails: it is still raised, and cleared when it can be.
	c.result = Unmet
	alerts.err = errors.New("no")

	if _, err := watch(); err == nil || !strings.Contains(err.Error(), "unable to clear") {
		t.Fatalf("clear: %v", err)
	}

	alerts.err = nil

	if note, err := watch(); err != nil || !strings.HasPrefix(note, "cleared the alert") || len(alerts.cleared()) != 1 {
		t.Fatalf("clear retry: %q %v", note, err)
	}
}

func TestWatchAlert_PanicsForWhatCouldNotWork(t *testing.T) {
	alerts := &recordingBroadcaster{}
	condition := func() (Result, string) { return Met, "" }
	severity := alert.Always(alert.SeverityInfo)

	for name, make := range map[string]func(){
		"no broadcaster": func() { WatchAlert(nil, "k", "t", severity, condition) },
		"no key":         func() { WatchAlert(alerts, "", "t", severity, condition) },
		"no title":       func() { WatchAlert(alerts, "k", "", severity, condition) },
		"no severity":    func() { WatchAlert(alerts, "k", "t", nil, condition) },
		"no condition":   func() { WatchAlert(alerts, "k", "t", severity, nil) },
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

type fakeDoor struct {
	data device.DoorSensor
}

func (f *fakeDoor) GetDoorSensorData() device.DoorSensor { return f.data }

func door(position device.DoorPosition, changed time.Time) *fakeDoor {
	return &fakeDoor{data: device.DoorSensor{
		Name: "Garage door", HasData: position != device.DoorPositionUnknown, Position: position,
		LastChangeTime: changed, Connectivity: environment.ConnectivityOnline,
	}}
}

func TestDoorOpenFor(t *testing.T) {
	withNow(t, noon)

	open := door(device.DoorPositionOpen, noon.Add(-7*time.Minute))
	expect(t, "open long enough", DoorOpenFor(open, 5*time.Minute), Met, "Garage door has been open for 7m0s")

	open.data.LastChangeTime = noon.Add(-2 * time.Minute)
	expect(t, "open not long enough", DoorOpenFor(open, 5*time.Minute), Unmet, "Garage door has been open for 2m0s, not yet 5m0s")

	open.data.LastChangeTime = noon.Add(-5 * time.Minute)
	expect(t, "open exactly long enough", DoorOpenFor(open, 5*time.Minute), Met, "Garage door has been open for 5m0s")

	closed := door(device.DoorPositionClosed, noon.Add(-time.Hour))
	expect(t, "closed", DoorOpenFor(closed, 5*time.Minute), Unmet, "Garage door is closed")

	unreported := door(device.DoorPositionUnknown, time.Time{})
	expect(t, "unreported", DoorOpenFor(unreported, 5*time.Minute), Unknown, "Garage door has not reported its position")

	offline := door(device.DoorPositionOpen, noon.Add(-time.Hour))
	offline.data.Connectivity = environment.ConnectivityOffline
	expect(t, "sensor offline", DoorOpenFor(offline, 5*time.Minute), Unknown, "Garage door's sensor is offline, so it can't be told if the door is open")

	nameless := door(device.DoorPositionClosed, time.Time{})
	nameless.data.Name = ""
	expect(t, "nameless", DoorOpenFor(nameless, time.Minute), Unmet, "the door is closed")
}

func TestDoorOpenFor_ADoorFoundOpenCountsFromTheFirstTimeItWasSeenOpen(t *testing.T) {
	// The sensor has only ever reported the door open: it has never seen it change.
	d := door(device.DoorPositionOpen, time.Time{})
	condition := DoorOpenFor(d, 5*time.Minute)

	withNow(t, noon)
	expect(t, "first look", condition, Unmet, "Garage door has been open for 0s, not yet 5m0s")

	withNow(t, noon.Add(3*time.Minute))
	expect(t, "three minutes later", condition, Unmet, "Garage door has been open for 3m0s, not yet 5m0s")

	withNow(t, noon.Add(5*time.Minute))
	expect(t, "five minutes later", condition, Met, "Garage door has been open for 5m0s")

	// It closes, and opens again later: counted from then.
	d.data.Position = device.DoorPositionClosed
	withNow(t, noon.Add(6*time.Minute))
	expect(t, "closed", condition, Unmet, "Garage door is closed")

	d.data.Position = device.DoorPositionOpen
	withNow(t, noon.Add(20*time.Minute))
	expect(t, "reopened, first look", condition, Unmet, "Garage door has been open for 0s, not yet 5m0s")
}

func TestDoorConditions_PanicWithoutASensor(t *testing.T) {
	for name, make := range map[string]func(){
		"open": func() { DoorOpenFor(nil, time.Minute) },
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
