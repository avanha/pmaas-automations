# pmaas-automations
Re-usable automations for PMAAS.

These are plain Go, not plugins: they depend on interfaces (a switch, a time), never on a running server, so each one
can be written and tested once, with fakes, and used by any assembly. An assembly builds them in its code ("config as
code") and hands them to the automations plugin (`pmaas-plugin-automations`), which runs them and shows their status.

## Packages

- `sun`: when the sun rises, sets and passes the angles that dawn and dusk are counted by (civil, nautical and
  astronomical twilight), for a place and a day. Checked against the US Naval Observatory's tables.
- `trigger`: when something is due, once a day: at a time of day, or at an event of the sun with an offset ("15
  minutes before sunset"). A trigger is a pure function of the time, with `Previous` and `Next`, so that running on
  time, catching up after a pause and catching up after a restart are all the same question: when was this last due,
  and has it been done since?
- `automations` (this package): `New` makes an automation from a trigger and something to run. `TurnOn`, `TurnOff` and
  `Toggle` are things to run on switches. `When` runs something only when conditions are met (`SwitchIs`,
  `TemperatureBelow`), and otherwise says it is not ready and why, so the automation waits for them until its catch-up
  passes. `Fahrenheit` and `Celsius` make temperatures.

```go
automations.New("fan-off", "Turn off the fan once it cools after 1 AM",
	trigger.AtTimeOfDay(1, 0),
	automations.When(automations.TurnOff(fan),
		automations.SwitchIs(fan, device.SwitchStateOn),
		automations.TemperatureBelow(room, automations.Fahrenheit(75), 10*time.Minute)),
	automations.WithCatchUp(6*time.Hour+59*time.Minute)) // acts only before 8:00
```

### Keeping a switch in step with a condition

`Follow` keeps a switch on while a condition is met and off while it is not, changing it no more often than an interval
(counting from when the switch last changed, so a change by hand counts too), and leaving it alone when the condition
can't be told, such as when a thermometer has gone quiet.

`Regulate` is the same with two conditions, one for turning the switch on and one for turning it off, so that there can be
a gap between them: a fan that comes on above 90°F and goes off below 88°F doesn't start and stop as the temperature
wobbles across 90. Between the two it stays as it is. Run either often, in a window:

```go
automations.New("attic-fan", "Run the attic fan when the attic is hot",
	trigger.Windowed(trigger.Every(time.Minute), trigger.AtTimeOfDay(8, 0), trigger.Sun(lafayette, sun.Sunset, time.Hour)),
	automations.Regulate(atticFan,
		// On when the attic is over 90 and warmer than outside...
		automations.AllOf(
			automations.TemperatureAbove(attic, automations.Fahrenheit(90), 10*time.Minute),
			automations.Warmer(attic, outside, 10*time.Minute)),
		// ...and off when it is under 88 or outside is warmer.
		automations.AnyOf(
			automations.TemperatureBelow(attic, automations.Fahrenheit(88), 10*time.Minute),
			automations.Not(automations.Warmer(attic, outside, 10*time.Minute))),
		5*time.Minute),
	automations.WithCatchUp(0)) // only in the minute it is due: never run after the window has closed
```

A condition has three answers: met, unmet, and unknown. `When` acts only on met; `Follow` and `Regulate` hold on unknown.
`AllOf` is met when all are (and unknown if any is, since that one might have decided it); `AnyOf` is met when any is, whatever
the others can tell; `Not` turns met into unmet and back.

### Alerts that follow a condition

`WatchAlert` holds an alert for as long as a condition is met, raising it when it becomes met and clearing it when it
stops, and re-raising it with a new severity when the severity changes (an alert that escalates is still one alert). The
severity is a function of the time, so an alert can be a warning in the day and critical after dark:

```go
conf.Add(automations.New("garage-door-open", "Alert when the garage door is left open",
	trigger.Every(time.Minute),
	automations.WatchAlert(conf.Alerts(), "garage-door-open", "Garage door open",
		automations.SeverityBetween(
			trigger.Sun(home, sun.CivilDusk, 0), trigger.Sun(home, sun.CivilDawn, 0),
			alert.SeverityCritical, alert.SeverityWarning),
		automations.DoorOpenFor(garageDoor, 5*time.Minute)),
	automations.WithCatchUp(0)))
```

A condition that can't be told, like a door whose sensor is offline, leaves the alert as it is. An alert about the sensor
itself being offline isn't an automation: it is a setting on the device (`alert.WhenOffline`, in pmaas-spi), for the plugin
that looks after it.
