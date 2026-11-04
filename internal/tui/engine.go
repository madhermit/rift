package tui

import "github.com/madhermit/rift/internal/diff"

// EngineToggle rings the primary diff engine (auto when difftastic is available)
// with its alternates, cycled with `e`: difftastic and git each forced for every
// file. When the primary already is the git engine (difftastic unavailable) there
// is nothing to toggle.
type EngineToggle struct {
	engines []diff.Engine
	idx     int
}

// NewEngineToggle builds the engine ring starting from the given primary engine.
func NewEngineToggle(engine diff.Engine) EngineToggle {
	return EngineToggle{engines: append([]diff.Engine{engine}, diff.Alternates(engine)...)}
}

// Engine is the active engine — what previews are rendered with.
func (e EngineToggle) Engine() diff.Engine { return e.engines[e.idx] }

// Primary is the engine the ring started from. A view built over an existing ring
// is constructed with it and then handed the ring (see ViewSettings), since a view
// seeded with the alternate would conclude there is nothing to toggle to.
func (e EngineToggle) Primary() diff.Engine { return e.engines[0] }

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

// ViewSettings is how previews are rendered: the engine ring and the layout. A
// screen hands them to the views it hosts and takes them back, so `e` and `\\`
// mean the same thing across a lens toggle or a drilldown. The whole ring travels
// (not just the active engine) so a view opened on the alternate can still cycle
// back.
type ViewSettings struct {
	Engines EngineToggle
	Display diff.Display
}

// Same reports whether two settings render identically.
func (s ViewSettings) Same(o ViewSettings) bool {
	return s.Display == o.Display && s.Engines.Name() == o.Engines.Name()
}
