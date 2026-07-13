package tui

import "github.com/madhermit/rift/internal/diff"

// EngineToggle pairs the primary diff engine (difftastic when available) with
// git's own diff as the alternate, cycled with `e`. When the primary already
// is the git engine (difftastic unavailable) there is nothing to toggle.
type EngineToggle struct {
	engines []diff.Engine
	idx     int
}

// NewEngineToggle builds the engine ring starting from the given primary
// engine. An alternate matching the primary isn't added twice, so a ring
// seeded with an already-cycled engine (a log drilldown passes the active
// engine) stays duplicate-free.
func NewEngineToggle(engine diff.Engine) EngineToggle {
	engines := []diff.Engine{engine}
	if alt := diff.NewPlainEngine(); alt.Name() != engine.Name() {
		engines = append(engines, alt)
	}
	return EngineToggle{engines: engines}
}

// Engine is the active engine — what previews are rendered with.
func (e EngineToggle) Engine() diff.Engine { return e.engines[e.idx] }

// Name is the active engine's name, used as the header context label.
func (e EngineToggle) Name() string { return e.Engine().Name() }

// CanToggle reports whether there is more than one engine to cycle through
// (false without difftastic — the hint is then hidden).
func (e EngineToggle) CanToggle() bool { return len(e.engines) > 1 }

// Toggle advances to the next engine in the ring, returning the new state; a
// no-op when there's nothing to cycle to.
func (e EngineToggle) Toggle() EngineToggle {
	if len(e.engines) > 1 {
		e.idx = (e.idx + 1) % len(e.engines)
	}
	return e
}
