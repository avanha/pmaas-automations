// Package automations makes automations out of a trigger and something to do, and has the things to do that most
// need doing: turning switches on and off.
//
// The automations it makes are the SPI's automation.Automation, which the automations plugin runs.
package automations

import (
	"fmt"
	"time"

	"github.com/avanha/pmaas-spi/automation"
)

// DefaultCatchUp is how long after an automation was due it is still run, if it is not told otherwise. An hour is
// long enough to ride out a restart, and short enough that the lights don't come on in the middle of the night
// because the server was down all evening.
const DefaultCatchUp = time.Hour

// Option adjusts an automation that is being made.
type Option func(*made)

// WithDescription adds a longer description than the name, for the page.
func WithDescription(description string) Option {
	return func(m *made) { m.description = description }
}

// WithDescriptionFunc is WithDescription for a description that is worked out each time the page is made, such as one
// that names devices, whose names aren't known until they have connected (see SwitchNames). It is called on the goroutine
// the automations are run on, so it may look at the devices the automation uses, and it should be quick.
func WithDescriptionFunc(description func() string) Option {
	return func(m *made) { m.describe = description }
}

// WithCatchUp sets how long after it was due the automation is still run. Zero means only in the minute it is due.
// See automation.Automation.
func WithCatchUp(catchUp time.Duration) Option {
	return func(m *made) { m.catchUp = catchUp }
}

// New makes an automation that does run when trigger says it is due.
//
// The id is what the server remembers when the automation last ran under: choose one and keep it, even if the name
// changes. The name is what it does, as a person would say it: "Turn on the holiday lights".
//
// run should leave things right however many times it is called (see automation.Automation).
//
// It panics if the automation is not valid, like regexp.MustCompile does for a pattern that can't be: automations are
// written in the program, and a mistake in one should stop the program starting, not leave it quietly not doing it.
func New(id string, name string, trigger automation.Trigger, run func() (string, error), options ...Option) automation.Automation {
	m := &made{id: id, name: name, trigger: trigger, run: run, catchUp: DefaultCatchUp}

	for _, option := range options {
		option(m)
	}

	if err := automation.Validate(m); err != nil {
		panic(fmt.Sprintf("automations.New: %v", err))
	}

	if run == nil {
		panic(fmt.Sprintf("automations.New: automation %q has nothing to run", id))
	}

	return m
}

type made struct {
	id          string
	name        string
	description string
	describe    func() string
	trigger     automation.Trigger
	catchUp     time.Duration
	run         func() (string, error)
}

func (m *made) ID() string   { return m.id }
func (m *made) Name() string { return m.name }
func (m *made) Description() string {
	if m.describe != nil {
		return m.describe()
	}

	return m.description
}

func (m *made) Trigger() automation.Trigger { return m.trigger }
func (m *made) CatchUp() time.Duration      { return m.catchUp }
func (m *made) Run() (string, error)        { return m.run() }
