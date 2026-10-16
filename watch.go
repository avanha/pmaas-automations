package automations

import (
	"fmt"
	"strings"

	"github.com/avanha/pmaas-spi/alert"
	"github.com/avanha/pmaas-spi/automation"
)

// AlertSource is who the alerts that WatchAlert raises are from, which is the automations plugin.
const AlertSource = "automations"

// WatchAlert makes something to run, for New, that holds an alert for as long as a condition is met: when it becomes
// met the alert is raised, with the severity for the time, and when it stops being met the alert is cleared. If the
// condition can't be told, the alert is left as it is, since neither raising nor clearing it would be known to be right.
//
// The severity is asked for every time it is run, and if it has changed the alert is raised again with the new one, which
// escalates it, or eases it, as an alert that is one alert. Nothing is sent when nothing has changed, so it can be run
// every minute: use a trigger like Every, since it is the automation's trigger that says when it looks.
//
// The key says which alert it is, among those the automations plugin raises. The title is the headline, and the alert's message is the
// reason the condition gave when it was raised. alerts is where they go, which the automations plugin's configuration
// provides as Alerts.
func WatchAlert(alerts alert.Broadcaster, key string, title string, severity alert.SeverityFunc, condition Condition) func() (string, error) {
	if alerts == nil || key == "" || title == "" || severity == nil || condition == nil {
		panic("automations.WatchAlert: every argument is needed")
	}

	fullKey := "watch:" + key

	// What was last raised, so that the same is not raised again: the plugin's goroutine is the only one to use these.
	raised := false
	var raisedAs alert.Severity

	return func() (string, error) {
		result, reason := condition()
		current := severity(now()).Normalize()

		switch result {
		case Unknown:
			return "", automation.NotReady("leaving the alert as it is: %s", reason)
		case Met:
			if raised && raisedAs == current {
				return "", automation.NotReady("the alert is on, as %s: %s", current, reason)
			}

			if err := alert.Raise(alerts, AlertSource, fullKey, current, title, reason); err != nil {
				return "", fmt.Errorf("unable to raise the alert: %w", err)
			}

			previously := raised && raisedAs != current
			raised, raisedAs = true, current

			if previously {
				return fmt.Sprintf("changed the alert to %s: %s", current, reason), nil
			}

			return fmt.Sprintf("raised a %s alert: %s", current, reason), nil
		default:
			if !raised {
				return "", automation.NotReady("no alert: %s", reason)
			}

			if err := alert.Clear(alerts, AlertSource, fullKey); err != nil {
				return "", fmt.Errorf("unable to clear the alert: %w", err)
			}

			raised = false

			return strings.TrimSpace(fmt.Sprintf("cleared the alert: %s", reason)), nil
		}
	}
}
