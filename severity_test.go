package automations

import (
	"testing"
	"time"

	"github.com/avanha/pmaas-automations/sun"
	"github.com/avanha/pmaas-automations/trigger"
	"github.com/avanha/pmaas-spi/alert"
)

func TestSeverityBetween_CriticalAfterDark(t *testing.T) {
	denver, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Fatal(err)
	}

	home := sun.Location{Latitude: 39.9967, Longitude: -105.0944}
	severity := SeverityBetween(
		trigger.Sun(home, sun.CivilDusk, 0), trigger.Sun(home, sun.CivilDawn, 0),
		alert.SeverityCritical, alert.SeverityWarning)

	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 16, hour, minute, 0, 0, denver) }

	// Civil dawn on 16 October is 6:45am and civil dusk 6:46pm, in daylight saving time.
	for _, c := range []struct {
		when time.Time
		want alert.Severity
	}{
		{at(0, 30), alert.SeverityCritical}, {at(6, 44), alert.SeverityCritical}, {at(6, 46), alert.SeverityWarning},
		{at(12, 0), alert.SeverityWarning}, {at(18, 45), alert.SeverityWarning}, {at(18, 47), alert.SeverityCritical},
		{at(23, 0), alert.SeverityCritical},
	} {
		if got := severity(c.when); got != c.want {
			t.Errorf("%s: %v, want %v", c.when.Format("15:04"), got, c.want)
		}
	}

	// In December dark comes sooner: civil dusk is at 5:09pm standard time.
	december := time.Date(2026, 12, 21, 17, 30, 0, 0, denver)
	if got := severity(december); got != alert.SeverityCritical {
		t.Errorf("5:30pm on the shortest day: %v", got)
	}
}

func TestSeverityBetween_ByClockTimesToo(t *testing.T) {
	severity := SeverityBetween(trigger.AtTimeOfDay(23, 0), trigger.AtTimeOfDay(7, 0), alert.SeverityCritical, alert.SeverityWarning)

	at := func(hour int) time.Time { return time.Date(2026, 10, 16, hour, 0, 0, 0, time.UTC) }

	if severity(at(23)) != alert.SeverityCritical || severity(at(3)) != alert.SeverityCritical ||
		severity(at(7)) != alert.SeverityWarning || severity(at(15)) != alert.SeverityWarning {
		t.Error("wrong severities")
	}
}

func TestSeverityBetween_PanicsForATriggerThatIsNotThere(t *testing.T) {
	for name, make := range map[string]func(){
		"open":  func() { SeverityBetween(nil, trigger.AtTimeOfDay(1, 0), alert.SeverityInfo, alert.SeverityInfo) },
		"close": func() { SeverityBetween(trigger.AtTimeOfDay(1, 0), nil, alert.SeverityInfo, alert.SeverityInfo) },
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
