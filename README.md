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
